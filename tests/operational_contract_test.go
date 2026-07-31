package tests_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const (
	operationalScriptsDirectory                     = "scripts"
	operationalHelpTimeout                          = 5 * time.Second
	operationalOrchestrationTimeout                 = 30 * time.Second
	operationalHelpWaitDelay                        = time.Second
	operationalScopedEnvironmentMaximumAWKProcesses = 7
	constrainedPipeHelpCommand                      = `ulimit -p 1 2>/dev/null || true
exec "$@"`
)

func TestOperationalHelpCommandsUseBuiltinOutput(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	restrictedPath := testingInstance.TempDir()
	helpCommands := []struct {
		name             string
		path             string
		expectedFragment string
	}{
		{name: "live-providers", path: filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_providers.sh"), expectedFragment: "scripts/test_live_providers.sh [--preflight | --write-config <path>]"},
		{name: "production-live-test", path: filepath.Join(repositoryRoot, operationalScriptsDirectory, "live_test.sh"), expectedFragment: "Usage: make live-test"},
	}
	for _, helpCommand := range helpCommands {
		for _, helpArgument := range []string{"--help", "-h"} {
			testingInstance.Run(helpCommand.name+"/"+helpArgument, func(testingInstance *testing.T) {
				output := runOperationalHelpCommand(
					testingInstance,
					repositoryRoot,
					helpCommand.path,
					helpArgument,
					[]string{"PATH=" + restrictedPath},
				)
				if !strings.Contains(output, helpCommand.expectedFragment) {
					testingInstance.Fatalf("unexpected help output for %s: %s", helpCommand.path, output)
				}
			})
		}
	}
}

func TestOperationalCIRunnerRequiresCurrentCompletionEvidence(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	fakeMakePath := filepath.Join(fixtureRoot, "make")
	fakeGoPath := filepath.Join(fixtureRoot, "go")
	runnerPath := filepath.Join(repositoryRoot, operationalScriptsDirectory, "run_ci.sh")

	writeOperationalFile(testingInstance, fakeMakePath, `#!/usr/bin/env bash
set -euo pipefail

target=""
for argument in "$@"; do
  target="${argument}"
done
builtin printf '%s\n' "${target}" >>"${CI_TARGET_LOG:?}"
if [[ "${CI_FAIL_TARGET:-}" == "${target}" ]]; then
  exit "${CI_FAIL_STATUS:-23}"
fi
if [[ "${target}" == "go-test" && "${CI_SKIP_COVERAGE:-0}" != "1" ]]; then
  builtin printf '%s\n' "mode: count" "fixture.go:1.1,1.2 1 1" >"${COVERAGE_FILE:?}"
fi
`, 0o755)
	writeOperationalFile(testingInstance, fakeGoPath, `#!/usr/bin/env bash
set -euo pipefail

[[ "$#" -eq 3 ]]
[[ "$1" == "tool" ]]
[[ "$2" == "cover" ]]
coverage_file="${3#-func=}"
if [[ ! -s "${coverage_file}" ]]; then
  builtin printf 'current coverage evidence is missing: %s\n' "${coverage_file}" >&2
  exit 24
fi
builtin printf 'total:\t(statements)\t%s\n' "${CI_COVERAGE_TOTAL:-100.0%}"
`, 0o755)

	expectedTargets := strings.Join([]string{
		"check-format",
		"go-lint",
		"python-lint",
		"frontend-lint",
		"go-test",
		"python-test",
		"frontend-test",
		"test-openapi-pages-artifact",
		"test-management-auth-blackbox",
		"test-live-provider-harness",
	}, "\n") + "\n"

	testingInstance.Run("complete", func(testingInstance *testing.T) {
		targetLogPath := filepath.Join(fixtureRoot, "complete-targets")
		command := exec.Command(runnerPath)
		command.Dir = repositoryRoot
		command.Env = append(
			os.Environ(),
			"MAKE_BIN="+fakeMakePath,
			"GO="+fakeGoPath,
			"CI_TARGET_LOG="+targetLogPath,
		)
		output, commandError := command.CombinedOutput()
		if commandError != nil {
			testingInstance.Fatalf("complete CI fixture failed: %v\n%s", commandError, output)
		}
		outputText := string(output)
		for _, expectedFragment := range []string{
			"CI summary",
			"Go coverage verification",
			"100.0%",
			"CI PASSED: all 11 gates completed; Go statement coverage 100.0%.",
		} {
			if !strings.Contains(outputText, expectedFragment) {
				testingInstance.Fatalf("complete CI output omitted %q:\n%s", expectedFragment, outputText)
			}
		}
		targetLogBytes, readError := os.ReadFile(targetLogPath)
		if readError != nil {
			testingInstance.Fatalf("read complete CI target log: %v", readError)
		}
		if string(targetLogBytes) != expectedTargets {
			testingInstance.Fatalf("unexpected CI target order:\n%s", targetLogBytes)
		}
	})

	testingInstance.Run("child-failure", func(testingInstance *testing.T) {
		targetLogPath := filepath.Join(fixtureRoot, "failure-targets")
		command := exec.Command(runnerPath)
		command.Dir = repositoryRoot
		command.Env = append(
			os.Environ(),
			"MAKE_BIN="+fakeMakePath,
			"GO="+fakeGoPath,
			"CI_TARGET_LOG="+targetLogPath,
			"CI_FAIL_TARGET=frontend-test",
			"CI_FAIL_STATUS=23",
		)
		output, commandError := command.CombinedOutput()
		exitError, isExitError := commandError.(*exec.ExitError)
		if !isExitError || exitError.ExitCode() != 23 {
			testingInstance.Fatalf("child failure exit=%v, want 23\n%s", commandError, output)
		}
		outputText := string(output)
		if !strings.Contains(outputText, "CI FAILED: stopped during Frontend browser tests (exit 23).") {
			testingInstance.Fatalf("child failure omitted active stage:\n%s", outputText)
		}
		if strings.Contains(outputText, "CI PASSED") {
			testingInstance.Fatalf("child failure printed a success receipt:\n%s", outputText)
		}
	})

	testingInstance.Run("zero-exit-without-coverage", func(testingInstance *testing.T) {
		targetLogPath := filepath.Join(fixtureRoot, "missing-coverage-targets")
		command := exec.Command(runnerPath)
		command.Dir = repositoryRoot
		command.Env = append(
			os.Environ(),
			"MAKE_BIN="+fakeMakePath,
			"GO="+fakeGoPath,
			"CI_TARGET_LOG="+targetLogPath,
			"CI_SKIP_COVERAGE=1",
		)
		output, commandError := command.CombinedOutput()
		exitError, isExitError := commandError.(*exec.ExitError)
		if !isExitError || exitError.ExitCode() != 24 {
			testingInstance.Fatalf("missing coverage exit=%v, want 24\n%s", commandError, output)
		}
		outputText := string(output)
		if !strings.Contains(outputText, "current coverage evidence is missing:") ||
			!strings.Contains(outputText, "CI FAILED: stopped during Go coverage verification (exit 24).") {
			testingInstance.Fatalf("missing coverage was not reported as incomplete CI:\n%s", outputText)
		}
		if strings.Contains(outputText, "CI PASSED") {
			testingInstance.Fatalf("missing coverage printed a success receipt:\n%s", outputText)
		}
		targetLogBytes, readError := os.ReadFile(targetLogPath)
		if readError != nil {
			testingInstance.Fatalf("read missing-coverage target log: %v", readError)
		}
		if string(targetLogBytes) != expectedTargets {
			testingInstance.Fatalf("missing-coverage fixture did not return zero from every target:\n%s", targetLogBytes)
		}
	})
}

func TestOperationalEnvironmentExamplesStayDocumentationOnly(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	for _, relativePath := range []string{
		filepath.Join("configs", ".env.sample"),
		filepath.Join("configs", ".env.local.example"),
	} {
		environmentBytes, readError := os.ReadFile(filepath.Join(repositoryRoot, relativePath))
		if readError != nil {
			testingInstance.Fatalf("read environment documentation %s: %v", relativePath, readError)
		}
		environmentDocumentation := string(environmentBytes)
		for _, expectedFragment := range []string{
			"Documentation only: never source, copy, or use this file as runtime configuration.",
			"Values are deliberately goofy and non-operational.",
			".invalid",
		} {
			if !strings.Contains(environmentDocumentation, expectedFragment) {
				testingInstance.Fatalf("environment documentation %s omitted %q: %s", relativePath, expectedFragment, environmentDocumentation)
			}
		}
		for _, forbiddenFragment := range []string{
			"LLM_PROXY_MANAGEMENT_ENABLED=true",
			"__GENERATE_ON_FIRST_MAKE_UP__",
			"localhost",
			"llm-proxy.mprlab.com",
		} {
			if strings.Contains(environmentDocumentation, forbiddenFragment) {
				testingInstance.Fatalf("environment documentation %s contains runnable value %q: %s", relativePath, forbiddenFragment, environmentDocumentation)
			}
		}
	}
}

func TestOperationalMakeUpRequiresPrivateLocalEnvironment(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	for _, relativePath := range []string{
		"Makefile",
		filepath.Join(operationalScriptsDirectory, "up.sh"),
	} {
		copyOperationalFile(testingInstance, filepath.Join(repositoryRoot, relativePath), filepath.Join(fixtureRoot, relativePath))
	}

	command := exec.Command("make", "up")
	command.Dir = fixtureRoot
	output, commandError := command.CombinedOutput()
	if commandError == nil {
		testingInstance.Fatalf("make up accepted a missing private environment: %s", output)
	}
	localEnvironmentPath := filepath.Join(fixtureRoot, "configs", ".env.local")
	for _, expectedFragment := range []string{
		"missing private local environment: " + localEnvironmentPath,
		"create the ignored real file explicitly with mode 0600",
		"tracked env examples are documentation only",
	} {
		if !strings.Contains(string(output), expectedFragment) {
			testingInstance.Fatalf("missing-private-env failure omitted %q: %s", expectedFragment, output)
		}
	}
	if _, statError := os.Stat(localEnvironmentPath); !os.IsNotExist(statError) {
		testingInstance.Fatalf("make up created the private environment instead of rejecting its absence: %v", statError)
	}
}

func TestOperationalMakeUpStartsLocalWebOrchestration(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	for _, relativePath := range []string{
		"Makefile",
		".dockerignore",
		"docker-compose.local.yml",
		filepath.Join(operationalScriptsDirectory, "up.sh"),
		filepath.Join("configs", "config.yml"),
		filepath.Join("configs", "tauth.local.yml"),
	} {
		copyOperationalFile(testingInstance, filepath.Join(repositoryRoot, relativePath), filepath.Join(fixtureRoot, relativePath))
	}
	writeOperationalLocalEnvironment(testingInstance, fixtureRoot)

	toolDirectory := filepath.Join(fixtureRoot, "tools")
	realAWKPath, lookupAWKError := exec.LookPath("awk")
	if lookupAWKError != nil {
		testingInstance.Fatalf("resolve awk executable: %v", lookupAWKError)
	}
	awkInvocationsPath := filepath.Join(fixtureRoot, "awk-invocations")
	composePIDPath := filepath.Join(fixtureRoot, "compose.pid")
	composeArgumentsPath := filepath.Join(fixtureRoot, "compose-arguments")
	composeDownPath := filepath.Join(fixtureRoot, "compose-down")
	composeStartedPath := filepath.Join(fixtureRoot, "compose-started")
	curlArgumentsPath := filepath.Join(fixtureRoot, "curl-arguments")
	curlEarlyPath := filepath.Join(fixtureRoot, "curl-early")
	curlReadyPath := filepath.Join(fixtureRoot, "curl-ready")
	writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "docker"), `#!/usr/bin/env bash
set -euo pipefail

builtin printf '%s\n' "$*" >>"${DOCKER_ARGUMENT_CAPTURE:?}"
[[ "${1:?}" == "compose" ]]
shift
while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --project-name|--file)
      shift 2
      ;;
    *)
      break
      ;;
  esac
done
case "${1:?}" in
  version)
    exit 0
    ;;
  up)
    sleep 0.1
    builtin printf '%s\n' started >"${COMPOSE_STARTED_CAPTURE:?}"
    exit 0
    ;;
  ps)
    builtin printf '%s\n' api frontend tauth
    ;;
  logs)
    builtin printf '%s\n' "$$" >"${COMPOSE_PID_CAPTURE:?}"
    trap 'exit 0' INT TERM
    while :; do sleep 1; done
    ;;
  down)
    builtin printf '%s\n' down >"${COMPOSE_DOWN_CAPTURE:?}"
    exit 0
    ;;
  *)
    exit 1
    ;;
esac
`, 0o755)
	writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "curl"), `#!/usr/bin/env bash
set -euo pipefail

arguments="$*"
builtin printf '%s\n' "${arguments}" >>"${CURL_ARGUMENT_CAPTURE:?}"
if [[ ! -f "${COMPOSE_STARTED_CAPTURE:?}" ]]; then
  builtin printf '%s\n' early >"${CURL_EARLY_CAPTURE:?}"
  exit 1
fi
[[ ! -f "${CURL_EARLY_CAPTURE:?}" ]]
case "${arguments}" in
  *"http://localhost:4179/config-ui.yaml"*)
    status=200
    ;;
  *"http://localhost:4179/auth/session"*)
    status=204
    ;;
  *"http://localhost:4179/auth/nonce"*)
    status=200
    ;;
  *"http://localhost:4179/"*)
    status=200
    ;;
  *"http://localhost:8080/?prompt=ready"*)
    status=403
    ;;
  *"http://localhost:8080/api/management/account"*)
    status=401
    builtin printf '%s\n' ready >"${CURL_READY_CAPTURE:?}"
    ;;
  *)
    exit 1
    ;;
esac
builtin printf '%s' "${status}"
`, 0o755)
	writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "openssl"), `#!/usr/bin/env bash
set -euo pipefail

[[ "${1:?}" == "rand" ]]
[[ "${2:?}" == "-base64" ]]
builtin printf '%s' generated-local-value
`, 0o755)
	writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "awk"), `#!/usr/bin/env bash
set -euo pipefail

builtin printf '%s\n' invoked >>"${AWK_INVOCATION_CAPTURE:?}"
exec "${REAL_AWK_PATH:?}" "$@"
`, 0o755)

	command := exec.Command("make", "up")
	command.Dir = fixtureRoot
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Env = append(
		os.Environ(),
		"PATH="+toolDirectory+string(os.PathListSeparator)+os.Getenv("PATH"),
		"AWK_INVOCATION_CAPTURE="+awkInvocationsPath,
		"REAL_AWK_PATH="+realAWKPath,
		"DOCKER_ARGUMENT_CAPTURE="+composeArgumentsPath,
		"COMPOSE_PID_CAPTURE="+composePIDPath,
		"COMPOSE_DOWN_CAPTURE="+composeDownPath,
		"COMPOSE_STARTED_CAPTURE="+composeStartedPath,
		"CURL_ARGUMENT_CAPTURE="+curlArgumentsPath,
		"CURL_EARLY_CAPTURE="+curlEarlyPath,
		"CURL_READY_CAPTURE="+curlReadyPath,
	)
	var output synchronizedOperationalOutput
	command.Stdout = &output
	command.Stderr = &output
	if startError := command.Start(); startError != nil {
		testingInstance.Fatalf("start make up: %v", startError)
	}
	waitForOperationalFile(testingInstance, curlReadyPath, operationalOrchestrationTimeout)
	waitForOperationalOutput(testingInstance, &output, "LLM Proxy local orchestration is ready.", operationalOrchestrationTimeout)
	waitForOperationalFile(testingInstance, composePIDPath, operationalOrchestrationTimeout)
	if signalError := syscall.Kill(-command.Process.Pid, syscall.SIGINT); signalError != nil {
		testingInstance.Fatalf("interrupt make up: %v", signalError)
	}
	waitForOperationalCommand(testingInstance, command, operationalOrchestrationTimeout)
	assertOperationalProxyChildStopped(testingInstance, composePIDPath)

	composeArguments, readComposeArgumentsError := os.ReadFile(composeArgumentsPath)
	if readComposeArgumentsError != nil {
		testingInstance.Fatalf("read Compose arguments: %v", readComposeArgumentsError)
	}
	expectedComposeFilePath, resolveComposeFileError := filepath.EvalSymlinks(filepath.Join(fixtureRoot, "docker-compose.local.yml"))
	if resolveComposeFileError != nil {
		testingInstance.Fatalf("resolve local Compose file: %v", resolveComposeFileError)
	}
	for _, expectedFragment := range []string{
		"--project-name llm-proxy-local",
		"--file " + expectedComposeFilePath,
		"up --build --remove-orphans --wait",
		"ps --status running --services",
		"logs --follow --no-color",
		"down --remove-orphans",
	} {
		if !strings.Contains(string(composeArguments), expectedFragment) {
			testingInstance.Fatalf("make up did not use the local Compose contract %q: %s", expectedFragment, composeArguments)
		}
	}
	if _, downReadError := os.ReadFile(composeDownPath); downReadError != nil {
		testingInstance.Fatalf("make up did not stop the local Compose stack: %v", downReadError)
	}
	if _, earlyReadError := os.ReadFile(curlEarlyPath); !os.IsNotExist(earlyReadError) {
		testingInstance.Fatalf("make up started HTTP readiness before Compose reported startup: %v", earlyReadError)
	}
	awkInvocations, readAWKInvocationsError := os.ReadFile(awkInvocationsPath)
	if readAWKInvocationsError != nil {
		testingInstance.Fatalf("read awk invocations: %v", readAWKInvocationsError)
	}
	awkProcessCount := len(strings.Fields(string(awkInvocations)))
	if awkProcessCount == 0 || awkProcessCount > operationalScopedEnvironmentMaximumAWKProcesses {
		testingInstance.Fatalf(
			"make up used %d awk processes while preparing scoped environments; want 1..%d",
			awkProcessCount,
			operationalScopedEnvironmentMaximumAWKProcesses,
		)
	}

	curlArguments, readCurlArgumentsError := os.ReadFile(curlArgumentsPath)
	if readCurlArgumentsError != nil {
		testingInstance.Fatalf("read curl arguments: %v", readCurlArgumentsError)
	}
	for _, expectedURL := range []string{
		"http://localhost:4179/",
		"http://localhost:4179/config-ui.yaml",
		"http://localhost:4179/auth/session",
		"http://localhost:4179/auth/nonce",
		"http://localhost:8080/?prompt=ready",
		"http://localhost:8080/api/management/account",
	} {
		if !strings.Contains(string(curlArguments), expectedURL) {
			testingInstance.Fatalf("make up did not verify %s: %s", expectedURL, curlArguments)
		}
	}
	if !strings.Contains(string(curlArguments), "Origin: http://localhost:4179") {
		testingInstance.Fatalf("make up did not verify browser-origin authentication boundaries: %s", curlArguments)
	}
	if !strings.Contains(string(curlArguments), "--request POST --header Origin: http://localhost:4179 --header X-Requested-With: XMLHttpRequest http://localhost:4179/auth/nonce") {
		testingInstance.Fatalf("make up did not verify browser nonce issuance through the same-origin frontend: %s", curlArguments)
	}

	localEnvironment, readLocalEnvironmentError := os.ReadFile(filepath.Join(fixtureRoot, "configs", ".env.local"))
	if readLocalEnvironmentError != nil {
		testingInstance.Fatalf("read generated local environment: %v", readLocalEnvironmentError)
	}
	for _, expectedFragment := range []string{
		"LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN=http://localhost:4179",
		"LLM_PROXY_MANAGEMENT_API_ORIGIN=http://localhost:8080",
		"GHTTP_SERVE_DIRECTORY=/app/site",
		"LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY=generated-local-value",
		"LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY=generated-local-value",
	} {
		if !strings.Contains(string(localEnvironment), expectedFragment) {
			testingInstance.Fatalf("local environment omitted %q: %s", expectedFragment, localEnvironment)
		}
	}
	if strings.Contains(string(localEnvironment), "__GENERATE_ON_FIRST_MAKE_UP__") {
		testingInstance.Fatalf("make up left generated local secrets unresolved: %s", localEnvironment)
	}
	assertOperationalEnvironmentKeys(testingInstance, filepath.Join(fixtureRoot, "configs", ".env.frontend.local"), []string{
		"GHTTP_SERVE_PORT",
		"GHTTP_SERVE_DIRECTORY",
		"GHTTP_SERVE_NO_MARKDOWN",
	})
	assertOperationalEnvironmentKeys(testingInstance, filepath.Join(fixtureRoot, "configs", ".env.api.local"), []string{
		"LLM_PROXY_MANAGEMENT_ENABLED",
		"LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN",
		"LLM_PROXY_MANAGEMENT_LOOPBACK_ORIGIN",
		"LLM_PROXY_MANAGEMENT_LOCALHOST_ORIGIN",
		"LLM_PROXY_MANAGEMENT_UI_DESCRIPTION",
		"LLM_PROXY_MANAGEMENT_ADMIN_EMAILS",
		"LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID",
		"LLM_PROXY_MANAGEMENT_GOOGLE_CLIENT_ID",
		"LLM_PROXY_MANAGEMENT_TAUTH_LOGIN_PATH",
		"LLM_PROXY_MANAGEMENT_TAUTH_LOGOUT_PATH",
		"LLM_PROXY_MANAGEMENT_TAUTH_NONCE_PATH",
		"LLM_PROXY_MANAGEMENT_TAUTH_SESSION_PATH",
		"LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY",
		"LLM_PROXY_MANAGEMENT_JWT_ISSUER",
		"LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME",
		"LLM_PROXY_MANAGEMENT_DATABASE_PATH",
		"LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY",
		"LLM_PROXY_MANAGEMENT_API_ORIGIN",
		"LLM_PROXY_MANAGEMENT_PROXY_ORIGIN",
	})
	assertOperationalEnvironmentKeys(testingInstance, filepath.Join(fixtureRoot, "configs", ".env.tauth.local"), []string{
		"TAUTH_CONFIG_FILE",
		"TAUTH_LISTEN_ADDR",
		"TAUTH_DATABASE_URL",
		"TAUTH_ENABLE_CORS",
		"TAUTH_CORS_EXCEPTION_1",
		"TAUTH_ALLOW_INSECURE_HTTP",
		"LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN",
		"LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID",
		"LLM_PROXY_MANAGEMENT_GOOGLE_CLIENT_ID",
		"LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY",
		"LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME",
		"LLM_PROXY_LOCAL_TAUTH_REFRESH_COOKIE_NAME",
	})

	for _, configurationContract := range []struct {
		path              string
		expectedFragments []string
	}{
		{
			path: filepath.Join(fixtureRoot, "docker-compose.local.yml"),
			expectedFragments: []string{
				"image: ghcr.io/tyemirov/ghttp:latest",
				"./configs/.env.frontend.local",
				"./configs/.env.api.local",
				"./configs/.env.tauth.local",
				"GHTTP_SERVE_PROXIES: \"/config-ui.yaml=http://api:8080,/auth=http://tauth:8080,/me=http://tauth:8080\"",
				"GHTTP_SERVE_RESPONSE_HEADERS: \"/=Cache-Control:no-store\"",
				"LLM_PROXY_MANAGEMENT_TAUTH_URL: \"http://localhost:4179\"",
				"127.0.0.1:4179:4179",
				"127.0.0.1:8080:8080",
				"./site:/app/site:ro",
			},
		},
		{
			path: filepath.Join(fixtureRoot, "configs", "tauth.local.yml"),
			expectedFragments: []string{
				"id: \"${LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID}\"",
				"jwt_signing_key: \"${LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY}\"",
				"session_cookie_name: \"${LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME}\"",
				"refresh_cookie_name: \"${LLM_PROXY_LOCAL_TAUTH_REFRESH_COOKIE_NAME}\"",
			},
		},
		{
			path: filepath.Join(fixtureRoot, ".dockerignore"),
			expectedFragments: []string{
				"configs/.env",
				"configs/.env.local",
				"configs/.env.*.local",
			},
		},
	} {
		configurationBytes, readConfigurationError := os.ReadFile(configurationContract.path)
		if readConfigurationError != nil {
			testingInstance.Fatalf("read local orchestration configuration %s: %v", configurationContract.path, readConfigurationError)
		}
		for _, expectedFragment := range configurationContract.expectedFragments {
			if !strings.Contains(string(configurationBytes), expectedFragment) {
				testingInstance.Fatalf("local orchestration configuration %s omitted %q: %s", configurationContract.path, expectedFragment, configurationBytes)
			}
		}
	}
	composeConfiguration, readComposeConfigurationError := os.ReadFile(filepath.Join(fixtureRoot, "docker-compose.local.yml"))
	if readComposeConfigurationError != nil {
		testingInstance.Fatalf("read local Compose configuration: %v", readComposeConfigurationError)
	}
	for _, forbiddenEnvFile := range []string{"- ./configs/.env\n", "- ./configs/.env.local\n"} {
		if strings.Contains(string(composeConfiguration), forbiddenEnvFile) {
			testingInstance.Fatalf("local Compose injects aggregate environment file %q: %s", forbiddenEnvFile, composeConfiguration)
		}
	}
	if strings.Contains(string(composeConfiguration), "127.0.0.1:8082:8080") {
		testingInstance.Fatalf("local Compose exposes TAuth outside the same-origin frontend: %s", composeConfiguration)
	}
	if !strings.Contains(output.String(), "LLM Proxy local orchestration stopped.") {
		testingInstance.Fatalf("make up did not report local stack shutdown: %s", output.String())
	}
}

func TestOperationalMakeUpRejectsAnotherProcessReadinessResponse(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	for _, relativePath := range []string{
		"Makefile",
		"docker-compose.local.yml",
		filepath.Join(operationalScriptsDirectory, "up.sh"),
	} {
		copyOperationalFile(testingInstance, filepath.Join(repositoryRoot, relativePath), filepath.Join(fixtureRoot, relativePath))
	}
	writeOperationalLocalEnvironment(testingInstance, fixtureRoot)

	toolDirectory := filepath.Join(fixtureRoot, "tools")
	writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "docker"), `#!/usr/bin/env bash
set -euo pipefail

[[ "${1:?}" == "compose" ]]
shift
while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --project-name|--file)
      shift 2
      ;;
    *)
      break
      ;;
  esac
done
case "${1:?}" in
  version|down)
    exit 0
    ;;
  up)
    exit 1
    ;;
  *)
    exit 1
    ;;
esac
`, 0o755)
	writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "curl"), `#!/usr/bin/env bash
set -euo pipefail

builtin printf '%s' 200
`, 0o755)
	writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "openssl"), `#!/usr/bin/env bash
set -euo pipefail

[[ "${1:?}" == "rand" ]]
[[ "${2:?}" == "-base64" ]]
builtin printf '%s' generated-local-value
`, 0o755)

	commandContext, cancelCommand := context.WithTimeout(context.Background(), operationalHelpTimeout)
	defer cancelCommand()
	command := exec.CommandContext(commandContext, "make", "up")
	command.Dir = fixtureRoot
	command.Env = append(
		os.Environ(),
		"PATH="+toolDirectory+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, commandError := command.CombinedOutput()
	if commandContext.Err() == context.DeadlineExceeded {
		testingInstance.Fatal("make up did not fail after the owned Compose process exited")
	}
	if commandError == nil {
		testingInstance.Fatalf("make up accepted readiness from another process: %s", output)
	}
	if strings.Contains(string(output), "LLM Proxy local orchestration is ready.") {
		testingInstance.Fatalf("make up reported readiness for an exited Compose process: %s", output)
	}
	if !strings.Contains(string(output), "local orchestration failed to start with status 1") {
		testingInstance.Fatalf("make up did not report the owned Compose failure: %s", output)
	}
}

type synchronizedOperationalOutput struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (output *synchronizedOperationalOutput) Write(payload []byte) (int, error) {
	output.mutex.Lock()
	defer output.mutex.Unlock()
	return output.buffer.Write(payload)
}

func (output *synchronizedOperationalOutput) String() string {
	output.mutex.Lock()
	defer output.mutex.Unlock()
	return output.buffer.String()
}

func waitForOperationalOutput(testingInstance *testing.T, output *synchronizedOperationalOutput, expectedFragment string, timeout time.Duration) {
	testingInstance.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if strings.Contains(output.String(), expectedFragment) {
			return
		}
		if time.Now().After(deadline) {
			testingInstance.Fatalf("timed out waiting for operational output %q: %s", expectedFragment, output.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func assertOperationalEnvironmentKeys(testingInstance *testing.T, environmentPath string, expectedKeys []string) {
	testingInstance.Helper()
	environmentBytes, readEnvironmentError := os.ReadFile(environmentPath)
	if readEnvironmentError != nil {
		testingInstance.Fatalf("read scoped environment %s: %v", environmentPath, readEnvironmentError)
	}
	lines := strings.Split(strings.TrimSpace(string(environmentBytes)), "\n")
	actualKeys := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		key, value, hasValue := strings.Cut(line, "=")
		if !hasValue || key == "" || value == "" {
			testingInstance.Fatalf("invalid scoped environment line in %s: %q", environmentPath, line)
		}
		actualKeys[key] = struct{}{}
	}
	if len(actualKeys) != len(expectedKeys) || len(lines) != len(expectedKeys) {
		testingInstance.Fatalf("unexpected scoped environment keys in %s: %s", environmentPath, environmentBytes)
	}
	for _, expectedKey := range expectedKeys {
		if _, present := actualKeys[expectedKey]; !present {
			testingInstance.Fatalf("scoped environment %s omitted %s: %s", environmentPath, expectedKey, environmentBytes)
		}
	}
}

func TestOperationalShellScriptsDoNotUseHeredocs(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	heredocPattern := regexp.MustCompile(`<<-?[[:space:]]*['"]?[A-Za-z_][A-Za-z0-9_]*['"]?`)
	offendingScripts := []string{}
	walkError := filepath.Walk(filepath.Join(repositoryRoot, operationalScriptsDirectory), func(path string, fileInfo os.FileInfo, pathError error) error {
		if pathError != nil {
			return pathError
		}
		if fileInfo.IsDir() || filepath.Ext(path) != ".sh" {
			return nil
		}
		fileBytes, readError := os.ReadFile(path)
		if readError != nil {
			return readError
		}
		if heredocPattern.Match(fileBytes) {
			offendingScripts = append(offendingScripts, path)
		}
		return nil
	})
	if walkError != nil {
		testingInstance.Fatalf("scan shell scripts under %s: %v", operationalScriptsDirectory, walkError)
	}
	if len(offendingScripts) != 0 {
		testingInstance.Fatalf("shell scripts feed external commands through heredocs: %s", strings.Join(offendingScripts, ", "))
	}
}

func TestOperationalCoverageClientProbeUsesExplicitPrompt(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	coverageScriptPath := filepath.Join(fixtureRoot, operationalScriptsDirectory, "check_coverage.sh")
	copyOperationalFile(testingInstance, filepath.Join(repositoryRoot, operationalScriptsDirectory, "check_coverage.sh"), coverageScriptPath)

	fakeGoPath := filepath.Join(fixtureRoot, "fake-go")
	writeOperationalFile(testingInstance, fakeGoPath, `#!/usr/bin/env bash
set -euo pipefail

command_name="${1:?}"
shift

case "${command_name}" in
  test)
    coverage_profile=""
    for argument in "$@"; do
      case "${argument}" in
        -coverprofile=*)
          coverage_profile="${argument#-coverprofile=}"
          ;;
      esac
    done
    [[ -n "${coverage_profile}" ]]
    builtin printf '%s\n' 'mode: count' 'fake.go:1.1,1.2 1 1' >"${coverage_profile}"
    ;;
  build)
    output_path=""
    while [[ "$#" -gt 0 ]]; do
      case "$1" in
        -o)
          output_path="$2"
          shift 2
          ;;
        *)
          shift
          ;;
      esac
    done
    [[ -n "${output_path}" ]]
    builtin printf '%s\n' \
      '#!/bin/bash' \
      'set -euo pipefail' \
      'binary_name="${0##*/}"' \
      'if [[ "${binary_name}" == "llm-proxy-client.cover" && "${1:-}" != "--prompt" ]]; then' \
      '  exit 124' \
      'fi' \
      'exit 0' >"${output_path}"
    chmod +x "${output_path}"
    ;;
  tool)
    tool_name="${1:?}"
    shift
    case "${tool_name}" in
      covdata)
        coverage_profile=""
        for argument in "$@"; do
          case "${argument}" in
            -o=*)
              coverage_profile="${argument#-o=}"
              ;;
          esac
        done
        [[ -n "${coverage_profile}" ]]
        builtin printf '%s\n' 'mode: count' 'fake.go:1.1,1.2 1 1' >"${coverage_profile}"
        ;;
      cover)
        builtin printf '%s\n' 'total: (statements) 100.0%'
        ;;
      *)
        exit 1
        ;;
    esac
    ;;
  *)
    exit 1
    ;;
esac
`, 0o755)

	runOperationalCommand(testingInstance, fixtureRoot, append(os.Environ(), "GO="+fakeGoPath), coverageScriptPath)
}

func TestOperationalProductionLiveTestUsesDefaultTenantSecretOnly(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	for _, relativePath := range []string{
		"Makefile",
		filepath.Join(operationalScriptsDirectory, "live_test.sh"),
	} {
		copyOperationalFile(testingInstance, filepath.Join(repositoryRoot, relativePath), filepath.Join(fixtureRoot, relativePath))
	}

	liveTestScript, readScriptError := os.ReadFile(filepath.Join(fixtureRoot, operationalScriptsDirectory, "live_test.sh"))
	if readScriptError != nil {
		testingInstance.Fatalf("read production live-test script: %v", readScriptError)
	}
	for _, forbiddenFragment := range []string{"LIVE_ENV_FILE", "API_KEY", "SERVICE_SECRET", "configs/.env"} {
		if strings.Contains(string(liveTestScript), forbiddenFragment) {
			testingInstance.Fatalf("production live-test script reads forbidden local credential source %q", forbiddenFragment)
		}
	}

	toolDirectory := filepath.Join(fixtureRoot, "tools")
	captureDirectory := filepath.Join(fixtureRoot, "curl-capture")
	if createCaptureDirectoryError := os.MkdirAll(captureDirectory, 0o755); createCaptureDirectoryError != nil {
		testingInstance.Fatalf("create curl capture directory: %v", createCaptureDirectoryError)
	}
	writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "curl"), `#!/usr/bin/env bash
set -euo pipefail

curl_config_path=""
headers_path=""
response_path=""
request_timeout_seconds=""
while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --config)
      curl_config_path="$2"
      shift 2
      ;;
    --dump-header)
      headers_path="$2"
      shift 2
      ;;
    --output)
      response_path="$2"
      shift 2
      ;;
    --header)
      case "$2" in
        X-LLM-Proxy-Request-Timeout-Seconds:*)
          request_timeout_seconds="${2#X-LLM-Proxy-Request-Timeout-Seconds: }"
          ;;
      esac
      shift 2
      ;;
    --request|--connect-timeout|--max-time|--write-out|--data-binary)
      shift 2
      ;;
    --silent|--show-error)
      shift
      ;;
    *)
      exit 2
      ;;
  esac
done
[[ -n "${curl_config_path}" ]]
[[ -n "${headers_path}" ]]
[[ -n "${response_path}" ]]
[[ -n "${request_timeout_seconds}" ]]

request_url=""
while IFS= read -r config_line; do
  if [[ "${config_line}" == url\ =\ \"* ]]; then
    request_url="${config_line#url = \"}"
    request_url="${request_url%\"}"
  fi
done <"${curl_config_path}"
[[ -n "${request_url}" ]]
request_body="$(< /dev/stdin)"

call_count_path="${CURL_CAPTURE_DIRECTORY:?}/call-count"
call_index=0
if [[ -f "${call_count_path}" ]]; then
  call_index="$(<"${call_count_path}")"
fi
call_index=$((call_index + 1))
builtin printf '%s' "${call_index}" >"${call_count_path}"
builtin printf '%s' "${request_url}" >"${CURL_CAPTURE_DIRECTORY}/url-${call_index}"
builtin printf '%s' "${request_body}" >"${CURL_CAPTURE_DIRECTORY}/body-${call_index}"
builtin printf '%s' "${request_timeout_seconds}" >"${CURL_CAPTURE_DIRECTORY}/timeout-${call_index}"

response_marker="LLM_PROXY_LIVE_ECHO_OK"
if [[ "${request_body}" == *LLM_PROXY_LIVE_COMPLEX_OK* ]]; then
  response_marker="LLM_PROXY_LIVE_COMPLEX_OK"
fi
builtin printf 'HTTP/1.1 200 OK\r\nX-LLM-Proxy-Request-Timeout-Seconds: %s\r\n\r\n' "${request_timeout_seconds}" >"${headers_path}"
builtin printf '%s' "${response_marker}" >"${response_path}"
builtin printf '%s' '200'
`, 0o755)
	defaultTenantSecret := "default-tenant-client-secret"
	environment := append(
		os.Environ(),
		"PATH="+toolDirectory+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CURL_CAPTURE_DIRECTORY="+captureDirectory,
		"LLM_PROXY_SECRET="+defaultTenantSecret,
	)
	output := runOperationalCommand(testingInstance, fixtureRoot, environment, "make", "live-test")
	if strings.Contains(output, defaultTenantSecret) {
		testingInstance.Fatalf("production live-test output exposed the tenant secret: %s", output)
	}
	if !strings.Contains(output, "live test passed: total_cases=8") {
		testingInstance.Fatalf("production live-test did not report all cases: %s", output)
	}
	for _, expectedCase := range []string{
		"case=openai-background-polling provider=openai status=200",
		"case=anthropic-long-completion provider=anthropic status=200",
		"case=meta-long-completion provider=meta status=200",
	} {
		if !strings.Contains(output, expectedCase) {
			testingInstance.Fatalf("production live-test omitted long-completion result %q: %s", expectedCase, output)
		}
	}

	type liveTestMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type liveTestPayload struct {
		Messages  []liveTestMessage `json:"messages"`
		MaxTokens *int              `json:"max_tokens"`
	}
	expectedProviders := []string{"openai", "anthropic", "meta", "gemini", "moonshot", "openai", "anthropic", "meta"}
	for callIndex, expectedProvider := range expectedProviders {
		captureIndex := callIndex + 1
		requestURLBytes, readURLError := os.ReadFile(filepath.Join(captureDirectory, "url-"+strconv.Itoa(captureIndex)))
		if readURLError != nil {
			testingInstance.Fatalf("read production live-test URL for call %d: %v", captureIndex, readURLError)
		}
		requestURL, parseURLError := url.Parse(string(requestURLBytes))
		if parseURLError != nil {
			testingInstance.Fatalf("parse production live-test URL for call %d: %v", captureIndex, parseURLError)
		}
		if requestURL.Scheme != "https" || requestURL.Host != "llm-proxy-api.mprlab.com" || requestURL.Path != "/v2" {
			testingInstance.Fatalf("production live-test call %d used non-production endpoint: %s", captureIndex, requestURL)
		}
		query := requestURL.Query()
		if query.Get("key") != defaultTenantSecret || query.Get("provider") != expectedProvider || query.Get("format") != "text/plain" {
			testingInstance.Fatalf("production live-test call %d used unexpected tenant or route query: %s", captureIndex, requestURL)
		}
		if query.Has("model") {
			testingInstance.Fatalf("production live-test call %d bypassed the saved provider default model: %s", captureIndex, requestURL)
		}

		requestBody, readBodyError := os.ReadFile(filepath.Join(captureDirectory, "body-"+strconv.Itoa(captureIndex)))
		if readBodyError != nil {
			testingInstance.Fatalf("read production live-test body for call %d: %v", captureIndex, readBodyError)
		}
		var payload liveTestPayload
		if decodeError := json.Unmarshal(requestBody, &payload); decodeError != nil {
			testingInstance.Fatalf("decode production live-test body for call %d: %v", captureIndex, decodeError)
		}

		timeoutBytes, readTimeoutError := os.ReadFile(filepath.Join(captureDirectory, "timeout-"+strconv.Itoa(captureIndex)))
		if readTimeoutError != nil {
			testingInstance.Fatalf("read production live-test timeout for call %d: %v", captureIndex, readTimeoutError)
		}
		if callIndex < 5 {
			if len(payload.Messages) != 1 || payload.MaxTokens != nil || !strings.Contains(payload.Messages[0].Content, "LLM_PROXY_LIVE_ECHO_OK") {
				testingInstance.Fatalf("echo call %d did not preserve the simple marker request: %s", captureIndex, requestBody)
			}
			if string(timeoutBytes) != "90" {
				testingInstance.Fatalf("echo call %d used unexpected request budget: %s", captureIndex, timeoutBytes)
			}
			continue
		}
		if len(requestBody) < 16384 || len(payload.Messages) != 2 || payload.MaxTokens == nil || *payload.MaxTokens != 512 || !strings.Contains(payload.Messages[1].Content, "LLM_PROXY_LIVE_COMPLEX_OK") || !strings.Contains(payload.Messages[1].Content, "all 120 normalized lines") {
			testingInstance.Fatalf("long completion call %d did not preserve the large complex request contract: bytes=%d payload=%s", captureIndex, len(requestBody), requestBody)
		}
		if string(timeoutBytes) != "900" {
			testingInstance.Fatalf("long completion call %d used unexpected request budget: %s", captureIndex, timeoutBytes)
		}
	}
}

func TestOperationalProductionLiveTestRequiresDefaultTenantSecret(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	for _, relativePath := range []string{
		"Makefile",
		filepath.Join(operationalScriptsDirectory, "live_test.sh"),
	} {
		copyOperationalFile(testingInstance, filepath.Join(repositoryRoot, relativePath), filepath.Join(fixtureRoot, relativePath))
	}

	command := exec.Command("make", "live-test")
	command.Dir = fixtureRoot
	command.Env = []string{"PATH=" + os.Getenv("PATH")}
	output, commandError := command.CombinedOutput()
	if commandError == nil {
		testingInstance.Fatalf("make live-test accepted a missing Default-tenant secret: %s", output)
	}
	if !strings.Contains(string(output), "LLM_PROXY_SECRET must contain the Default-tenant client secret") {
		testingInstance.Fatalf("missing-secret error omitted the Default-tenant contract: %s", output)
	}
}

func TestOperationalLiveConfigDisablesManagementAndSafelyLoadsDotenv(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	environmentFile := filepath.Join(fixtureRoot, "live.env")
	configurationOutput := filepath.Join(fixtureRoot, "live-config.yml")
	writeOperationalFile(testingInstance, environmentFile, "QWEN_CLOUD_TOKEN_PLAN_API_KEY=test-qwencloud-key\nMINIMAX_API_KEY=test-minimax-key\nLLM_PROXY_MANAGEMENT_ENABLED=true\nLLM_PROXY_MANAGEMENT_UI_DESCRIPTION=LLM Proxy\n", 0o600)
	environment := append(
		os.Environ(),
		"LIVE_ENV_FILE="+environmentFile,
		"LLM_PROXY_LIVE_PROVIDERS=qwencloud,minimax",
		"LLM_PROXY_LIVE_PORT=18181",
		"GO=/does/not/exist",
	)
	runOperationalCommand(
		testingInstance,
		repositoryRoot,
		environment,
		filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_providers.sh"),
		"--write-config", configurationOutput,
	)
	configurationBytes, readError := os.ReadFile(configurationOutput)
	if readError != nil {
		testingInstance.Fatalf("read generated live config: %v", readError)
	}
	configuration := string(configurationBytes)
	if !strings.Contains(configuration, "  port: 18181") {
		testingInstance.Fatalf("generated live config did not set the requested port: %s", configuration)
	}
	if !strings.Contains(configuration, "management:\n  enabled: false") {
		testingInstance.Fatalf("generated live config did not disable management: %s", configuration)
	}
	for _, expectedFragment := range []string{"api_key: \"${QWEN_CLOUD_TOKEN_PLAN_API_KEY}\"", "api_key: \"${MINIMAX_API_KEY}\""} {
		if !strings.Contains(configuration, expectedFragment) {
			testingInstance.Fatalf("generated live config missing %q: %s", expectedFragment, configuration)
		}
	}
}

func TestOperationalLiveConfigWritesWithoutProviderKeys(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	configurationOutput := filepath.Join(testingInstance.TempDir(), "live-config.yml")
	environment := []string{
		"PATH=" + os.Getenv("PATH"),
		"LLM_PROXY_LIVE_PORT=18182",
	}
	runOperationalCommand(
		testingInstance,
		repositoryRoot,
		environment,
		filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_providers.sh"),
		"--write-config", configurationOutput,
	)
	configurationBytes, readError := os.ReadFile(configurationOutput)
	if readError != nil {
		testingInstance.Fatalf("read generated live config without provider keys: %v", readError)
	}
	configuration := string(configurationBytes)
	if !strings.Contains(configuration, "  port: 18182") {
		testingInstance.Fatalf("generated live config did not set the requested port: %s", configuration)
	}
	if !strings.Contains(configuration, "management:\n  enabled: false") {
		testingInstance.Fatalf("generated live config did not disable management: %s", configuration)
	}
}

func TestOperationalLiveConfigAllocatesDefaultHarnessPort(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	configurationOutput := filepath.Join(testingInstance.TempDir(), "live-config.yml")
	environment := append(
		os.Environ(),
		"LLM_PROXY_LIVE_PORT=",
		"GO=/does/not/exist",
	)
	runOperationalCommand(
		testingInstance,
		repositoryRoot,
		environment,
		filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_providers.sh"),
		"--write-config", configurationOutput,
	)
	configurationBytes, readError := os.ReadFile(configurationOutput)
	if readError != nil {
		testingInstance.Fatalf("read generated default-port live config: %v", readError)
	}
	allocatedPort := operationalLiveConfigPort(testingInstance, string(configurationBytes))
	if allocatedPort == 18080 {
		testingInstance.Fatalf("default live config retained shared port 18080: %s", configurationBytes)
	}
	if allocatedPort < 1024 {
		testingInstance.Fatalf("default live config did not allocate an unprivileged port: %d", allocatedPort)
	}
}

func TestOperationalLiveHarnessVerifiesEachKeyBeforeItsSmokeRequest(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixture := newOperationalLiveHarnessFixture(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	environmentFile := filepath.Join(fixtureRoot, "live.env")
	operationCapture := filepath.Join(fixtureRoot, "operations.log")
	const providerKey = "test-live-openai-key"
	writeOperationalFile(testingInstance, environmentFile, "OPENAI_API_KEY="+providerKey+"\n", 0o600)
	environment := []string{
		"PATH=" + fixture.toolDirectory + string(os.PathListSeparator) + os.Getenv("PATH"),
		"GO=" + filepath.Join(fixture.toolDirectory, "go"),
		"LLM_PROXY_LIVE_PORT=" + strconv.Itoa(operationalLoopbackPort(testingInstance)),
		"PROXY_PID_CAPTURE=" + fixture.proxyPIDPath,
		"LIVE_ENV_FILE=" + environmentFile,
		"LLM_PROXY_LIVE_PROVIDERS=openai",
		"LIVE_OPERATION_CAPTURE=" + operationCapture,
	}
	command := exec.Command(filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_providers.sh"))
	command.Dir = repositoryRoot
	command.Env = environment
	output, commandError := command.CombinedOutput()
	if commandError != nil {
		testingInstance.Fatalf("live provider harness failed: %v\n%s", commandError, output)
	}
	outputText := string(output)
	if !strings.Contains(outputText, "live provider verification passed: provider=openai model=gpt-4.1 status=200") ||
		!strings.Contains(outputText, "live provider smoke passed: provider=openai model=omitted status=200") {
		testingInstance.Fatalf("live provider harness omitted verification or smoke success: %s", output)
	}
	if strings.Contains(outputText, providerKey) || strings.Contains(outputText, "live-generated-secret") {
		testingInstance.Fatalf("live provider harness output exposed credential material: %s", output)
	}
	captureBytes, readError := os.ReadFile(operationCapture)
	if readError != nil {
		testingInstance.Fatalf("read live operation capture: %v", readError)
	}
	capture := string(captureBytes)
	verificationOffset := strings.Index(capture, "verify PUT ")
	smokeOffset := strings.Index(capture, "smoke POST ")
	if verificationOffset < 0 || smokeOffset <= verificationOffset {
		testingInstance.Fatalf("live operations were not verification then smoke: %s", capture)
	}
	expectedPayload := `payload {"api_key":"` + providerKey + `","text_model":"gpt-4.1","system_prompt":""}`
	if !strings.Contains(capture, expectedPayload) {
		testingInstance.Fatalf("live verification payload mismatch: %s", capture)
	}
	assertOperationalProxyChildStopped(testingInstance, fixture.proxyPIDPath)
}

func TestOperationalLiveHarnessReapsOwnedProxyChild(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	reservedPort := operationalLoopbackPort(testingInstance)
	fixture := newOperationalLiveHarnessFixture(testingInstance)
	environment := fixture.environment(reservedPort)
	runOperationalCommand(
		testingInstance,
		repositoryRoot,
		environment,
		filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_providers.sh"),
		"--preflight",
	)
	assertOperationalProxyChildStopped(testingInstance, fixture.proxyPIDPath)
}

func TestOperationalLiveHarnessReapsOwnedProxyChildAfterTermination(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixture := newOperationalLiveHarnessFixture(testingInstance)
	preflightBlockPath := filepath.Join(testingInstance.TempDir(), "preflight-blocked")
	command := exec.Command(filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_providers.sh"), "--preflight")
	command.Dir = repositoryRoot
	command.Env = fixture.environment(
		operationalLoopbackPort(testingInstance),
		"CURL_PREFLIGHT_BLOCK_PATH="+preflightBlockPath,
		"CURL_PREFLIGHT_BLOCK_SECONDS=2",
	)
	if startError := command.Start(); startError != nil {
		testingInstance.Fatalf("start live harness: %v", startError)
	}
	waitForOperationalFile(testingInstance, preflightBlockPath, operationalHelpTimeout)
	if signalError := command.Process.Signal(syscall.SIGTERM); signalError != nil {
		testingInstance.Fatalf("terminate live harness: %v", signalError)
	}
	if waitError := command.Wait(); waitError == nil {
		testingInstance.Fatal("live harness succeeded after termination")
	}
	assertOperationalProxyChildStopped(testingInstance, fixture.proxyPIDPath)
}

func operationalLiveConfigPort(testingInstance *testing.T, configuration string) int {
	testingInstance.Helper()
	portMatch := regexp.MustCompile(`(?m)^  port: ([0-9]+)$`).FindStringSubmatch(configuration)
	if len(portMatch) != 2 {
		testingInstance.Fatalf("generated live config omitted port: %s", configuration)
	}
	port, parseError := strconv.Atoi(portMatch[1])
	if parseError != nil {
		testingInstance.Fatalf("parse generated live config port: %v", parseError)
	}
	return port
}

type operationalLiveHarnessFixture struct {
	proxyPIDPath  string
	toolDirectory string
}

func newOperationalLiveHarnessFixture(testingInstance *testing.T) operationalLiveHarnessFixture {
	testingInstance.Helper()
	fixtureRoot := testingInstance.TempDir()
	toolDirectory := filepath.Join(fixtureRoot, "tools")
	proxyPIDPath := filepath.Join(fixtureRoot, "proxy.pid")
	writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "go"), `#!/usr/bin/env bash
set -euo pipefail

[[ "${1:?}" == "build" ]]
shift
output_path=""
while [[ "$#" -gt 0 ]]; do
  case "$1" in
    -o)
      output_path="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
[[ -n "${output_path}" ]]
builtin printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  'builtin printf "%s\n" "$$" >"${PROXY_PID_CAPTURE:?}"' \
  'exec sleep 60' >"${output_path}"
chmod +x "${output_path}"
`, 0o755)
	writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "curl"), `#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f "${PROXY_PID_CAPTURE:?}" ]]; then
  exit 1
fi

output_path=""
request_body_path=""
request_method="GET"
request_url=""
while [[ "$#" -gt 0 ]]; do
  case "$1" in
    -o)
      output_path="$2"
      shift 2
      ;;
    -X)
      request_method="$2"
      shift 2
      ;;
    --data-binary)
      request_body_path="${2#@}"
      shift 2
      ;;
    http://*)
      request_url="$1"
      shift
      ;;
    *)
      shift
      ;;
  esac
done

case "${request_url}" in
  */api/management/account)
    builtin printf '%s' '{"tenants":[{"id":"tenant-live"}]}' >"${output_path}"
    builtin printf '%s' 200
    ;;
  */api/management/tenants/tenant-live/secrets)
    builtin printf '%s' '{"secret":"live-generated-secret","profile":{"providers":[{"id":"openai","text_default_model":"gpt-4.1"}]}}' >"${output_path}"
    builtin printf '%s' 200
    ;;
  */api/management/tenants/tenant-live/provider-keys/*)
    if [[ -n "${LIVE_OPERATION_CAPTURE:-}" ]]; then
      builtin printf 'verify %s %s\n' "${request_method}" "${request_url}" >>"${LIVE_OPERATION_CAPTURE}"
      builtin printf 'payload ' >>"${LIVE_OPERATION_CAPTURE}"
      command cat "${request_body_path}" >>"${LIVE_OPERATION_CAPTURE}"
      builtin printf '\n' >>"${LIVE_OPERATION_CAPTURE}"
    fi
    builtin printf '%s' '{}' >"${output_path}"
    builtin printf '%s' 200
    ;;
  *provider=unsupported-live-preflight*)
    if [[ -n "${CURL_PREFLIGHT_BLOCK_PATH:-}" ]]; then
      builtin printf '%s\n' ready >"${CURL_PREFLIGHT_BLOCK_PATH}"
      sleep "${CURL_PREFLIGHT_BLOCK_SECONDS:-1}"
    fi
    builtin printf '%s' 400
    ;;
  *provider=*)
    if [[ -n "${LIVE_OPERATION_CAPTURE:-}" ]]; then
      builtin printf 'smoke %s %s\n' "${request_method}" "${request_url}" >>"${LIVE_OPERATION_CAPTURE}"
    fi
    builtin printf '%s' OK >"${output_path}"
    builtin printf '%s' 200
    ;;
  *)
    builtin printf '%s' 403
    ;;
esac
`, 0o755)
	return operationalLiveHarnessFixture{
		proxyPIDPath:  proxyPIDPath,
		toolDirectory: toolDirectory,
	}
}

func (fixture operationalLiveHarnessFixture) environment(port int, extraEnvironment ...string) []string {
	environment := append(
		os.Environ(),
		"PATH="+fixture.toolDirectory+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GO="+filepath.Join(fixture.toolDirectory, "go"),
		"LLM_PROXY_LIVE_PORT="+strconv.Itoa(port),
		"PROXY_PID_CAPTURE="+fixture.proxyPIDPath,
	)
	return append(environment, extraEnvironment...)
}

func operationalLoopbackPort(testingInstance *testing.T) int {
	testingInstance.Helper()
	listener, listenError := net.Listen("tcp", "127.0.0.1:0")
	if listenError != nil {
		testingInstance.Fatalf("reserve loopback port: %v", listenError)
	}
	reservedAddress, addressOK := listener.Addr().(*net.TCPAddr)
	if !addressOK {
		testingInstance.Fatalf("reserved address type=%T", listener.Addr())
	}
	if closeError := listener.Close(); closeError != nil {
		testingInstance.Fatalf("release loopback port: %v", closeError)
	}
	return reservedAddress.Port
}

func waitForOperationalFile(testingInstance *testing.T, path string, timeout time.Duration) {
	testingInstance.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if _, statError := os.Stat(path); statError == nil {
			return
		}
		if time.Now().After(deadline) {
			testingInstance.Fatalf("timed out waiting for operational file: %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForOperationalCommand(testingInstance *testing.T, command *exec.Cmd, timeout time.Duration) {
	testingInstance.Helper()
	completed := make(chan error, 1)
	go func() {
		completed <- command.Wait()
	}()
	select {
	case <-completed:
	case <-time.After(timeout):
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		<-completed
		testingInstance.Fatal("make up did not stop after interruption")
	}
}

func assertOperationalProxyChildStopped(testingInstance *testing.T, proxyPIDPath string) {
	testingInstance.Helper()
	proxyPIDBytes, readError := os.ReadFile(proxyPIDPath)
	if readError != nil {
		testingInstance.Fatalf("read proxy pid: %v", readError)
	}
	proxyPID, parseError := strconv.Atoi(strings.TrimSpace(string(proxyPIDBytes)))
	if parseError != nil {
		testingInstance.Fatalf("parse proxy pid: %v", parseError)
	}
	if killError := exec.Command("kill", "-0", strconv.Itoa(proxyPID)).Run(); killError == nil {
		_ = exec.Command("kill", "-TERM", strconv.Itoa(proxyPID)).Run()
		testingInstance.Fatalf("live harness left proxy child running: pid=%d", proxyPID)
	}
}

func operationalRepositoryRoot(testingInstance *testing.T) string {
	testingInstance.Helper()
	repositoryRoot, absoluteError := filepath.Abs("..")
	if absoluteError != nil {
		testingInstance.Fatalf("resolve repository root: %v", absoluteError)
	}
	return repositoryRoot
}

func writeOperationalLocalEnvironment(testingInstance *testing.T, fixtureRoot string) {
	testingInstance.Helper()
	writeOperationalFile(testingInstance, filepath.Join(fixtureRoot, "configs", ".env.local"), `LLM_PROXY_MANAGEMENT_ENABLED=true
LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN=http://localhost:4179
LLM_PROXY_MANAGEMENT_LOOPBACK_ORIGIN=http://localhost:4179
LLM_PROXY_MANAGEMENT_LOCALHOST_ORIGIN=http://localhost:4179
LLM_PROXY_MANAGEMENT_UI_DESCRIPTION=LLM Proxy test
LLM_PROXY_MANAGEMENT_ADMIN_EMAILS=[]
LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID=llm-proxy-test
LLM_PROXY_MANAGEMENT_GOOGLE_CLIENT_ID=424242424242-bananahelmet.apps.googleusercontent.com
LLM_PROXY_MANAGEMENT_TAUTH_LOGIN_PATH=/auth/google
LLM_PROXY_MANAGEMENT_TAUTH_LOGOUT_PATH=/auth/logout
LLM_PROXY_MANAGEMENT_TAUTH_NONCE_PATH=/auth/nonce
LLM_PROXY_MANAGEMENT_TAUTH_SESSION_PATH=/auth/session
LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY=__GENERATE_ON_FIRST_MAKE_UP__
LLM_PROXY_MANAGEMENT_JWT_ISSUER=tauth
LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME=app_session_llm_proxy_test
LLM_PROXY_MANAGEMENT_DATABASE_PATH=/data/llm-proxy-management.sqlite
LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY=__GENERATE_ON_FIRST_MAKE_UP__
LLM_PROXY_MANAGEMENT_API_ORIGIN=http://localhost:8080
LLM_PROXY_MANAGEMENT_PROXY_ORIGIN=http://localhost:8080
TAUTH_CONFIG_FILE=/config/tauth.local.yml
TAUTH_LISTEN_ADDR=:8080
TAUTH_DATABASE_URL=sqlite:///data/tauth.sqlite
TAUTH_ENABLE_CORS=true
TAUTH_CORS_EXCEPTION_1=https://accounts.google.com
TAUTH_ALLOW_INSECURE_HTTP=true
LLM_PROXY_LOCAL_TAUTH_REFRESH_COOKIE_NAME=app_refresh_llm_proxy_test
GHTTP_SERVE_PORT=4179
GHTTP_SERVE_DIRECTORY=/app/site
GHTTP_SERVE_NO_MARKDOWN=true
`, 0o600)
}

func copyOperationalFile(testingInstance *testing.T, sourcePath string, targetPath string) {
	testingInstance.Helper()
	fileBytes, readError := os.ReadFile(sourcePath)
	if readError != nil {
		testingInstance.Fatalf("read operational file %s: %v", sourcePath, readError)
	}
	fileInfo, statError := os.Stat(sourcePath)
	if statError != nil {
		testingInstance.Fatalf("stat operational file %s: %v", sourcePath, statError)
	}
	writeOperationalFile(testingInstance, targetPath, string(fileBytes), fileInfo.Mode().Perm())
}

func writeOperationalFile(testingInstance *testing.T, path string, contents string, permissions os.FileMode) {
	testingInstance.Helper()
	if directoryError := os.MkdirAll(filepath.Dir(path), 0o755); directoryError != nil {
		testingInstance.Fatalf("create operational directory: %v", directoryError)
	}
	if writeError := os.WriteFile(path, []byte(contents), permissions); writeError != nil {
		testingInstance.Fatalf("write operational file %s: %v", path, writeError)
	}
}

func runOperationalHelpCommand(
	testingInstance *testing.T,
	directory string,
	scriptPath string,
	helpArgument string,
	environment []string,
) string {
	testingInstance.Helper()
	bashPath, lookupError := exec.LookPath("bash")
	if lookupError != nil {
		testingInstance.Fatalf("resolve Bash executable: %v", lookupError)
	}
	commandContext, cancelCommand := context.WithTimeout(context.Background(), operationalHelpTimeout)
	defer cancelCommand()
	command := exec.CommandContext(
		commandContext,
		bashPath,
		"-c",
		constrainedPipeHelpCommand,
		"operational-help",
		bashPath,
		scriptPath,
		helpArgument,
	)
	command.Dir = directory
	command.WaitDelay = operationalHelpWaitDelay
	if environment != nil {
		command.Env = environment
	}
	output, commandError := command.CombinedOutput()
	if commandContext.Err() == context.DeadlineExceeded {
		testingInstance.Fatalf("operational help command timed out: %s %s", scriptPath, helpArgument)
	}
	if commandError != nil {
		testingInstance.Fatalf("operational help command failed: %s %s: %v\n%s", scriptPath, helpArgument, commandError, output)
	}
	return string(output)
}

func runOperationalCommand(testingInstance *testing.T, directory string, environment []string, commandName string, arguments ...string) string {
	testingInstance.Helper()
	command := exec.Command(commandName, arguments...)
	command.Dir = directory
	if environment != nil {
		command.Env = environment
	}
	output, commandError := command.CombinedOutput()
	if commandError != nil {
		testingInstance.Fatalf("operational command failed: %s %s: %v\n%s", commandName, strings.Join(arguments, " "), commandError, output)
	}
	return string(output)
}
