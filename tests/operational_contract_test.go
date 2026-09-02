package tests_test

import (
	"bytes"
	"context"
	"encoding/base64"
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

	"gopkg.in/yaml.v3"
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
		{name: "live-providers", path: filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_providers.sh"), expectedFragment: "scripts/test_live_providers.sh [--gemini-candidates | --media | --preflight | --write-config <path>] [--existing-local-origin <origin>]"},
		{name: "local-live-providers", path: filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_local.sh"), expectedFragment: "Usage: make test-live-local-providers"},
		{name: "live-gemini-candidates", path: filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_gemini_candidates.sh"), expectedFragment: "Runs paid Google Interactions acceptance"},
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

	testingInstance.Run("cleanup-failure", func(testingInstance *testing.T) {
		targetLogPath := filepath.Join(fixtureRoot, "cleanup-failure-targets")
		bashEnvironmentPath := filepath.Join(fixtureRoot, "cleanup-failure.bash")
		writeOperationalFile(testingInstance, bashEnvironmentPath, `rm() {
  command rm "$@"
  return 29
}
`, 0o600)
		command := exec.Command(runnerPath)
		command.Dir = repositoryRoot
		command.Env = append(
			os.Environ(),
			"MAKE_BIN="+fakeMakePath,
			"GO="+fakeGoPath,
			"CI_TARGET_LOG="+targetLogPath,
			"BASH_ENV="+bashEnvironmentPath,
		)
		output, commandError := command.CombinedOutput()
		exitError, isExitError := commandError.(*exec.ExitError)
		if !isExitError || exitError.ExitCode() != 1 {
			testingInstance.Fatalf("cleanup failure exit=%v, want 1\n%s", commandError, output)
		}
		outputText := string(output)
		if !strings.Contains(outputText, "CI FAILED: stopped during temporary CI state cleanup (exit 1).") {
			testingInstance.Fatalf("cleanup failure omitted active stage:\n%s", outputText)
		}
		for _, forbiddenFragment := range []string{"CI summary", "CI PASSED"} {
			if strings.Contains(outputText, forbiddenFragment) {
				testingInstance.Fatalf("cleanup failure printed %q:\n%s", forbiddenFragment, outputText)
			}
		}
		targetLogBytes, readError := os.ReadFile(targetLogPath)
		if readError != nil {
			testingInstance.Fatalf("read cleanup-failure target log: %v", readError)
		}
		if string(targetLogBytes) != expectedTargets {
			testingInstance.Fatalf("cleanup failure did not follow every target:\n%s", targetLogBytes)
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

func TestOperationalManagedModelMigrationsStayCatalogOwned(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	catalogDocument, readCatalogError := os.ReadFile(filepath.Join(repositoryRoot, "configs", "providers.yml"))
	if readCatalogError != nil {
		testingInstance.Fatalf("read provider catalog: %v", readCatalogError)
	}
	var catalog struct {
		ModelMigrations []struct {
			SourceModel string `yaml:"source_model"`
			TargetModel string `yaml:"target_model"`
		} `yaml:"model_migrations"`
	}
	if decodeError := yaml.Unmarshal(catalogDocument, &catalog); decodeError != nil {
		testingInstance.Fatalf("decode provider catalog model migrations: %v", decodeError)
	}
	if len(catalog.ModelMigrations) == 0 {
		testingInstance.Fatal("provider catalog has no managed model migrations")
	}
	storeDocument, readStoreError := os.ReadFile(filepath.Join(repositoryRoot, "internal", "proxy", "management_store.go"))
	if readStoreError != nil {
		testingInstance.Fatalf("read managed store: %v", readStoreError)
	}
	storeSource := string(storeDocument)
	if strings.Contains(storeSource, "ModelName") {
		testingInstance.Fatal("managed store contains a compiled model identifier constant")
	}
	for migrationIndex, migration := range catalog.ModelMigrations {
		for field, model := range map[string]string{"source_model": migration.SourceModel, "target_model": migration.TargetModel} {
			if model != "" && strings.Contains(storeSource, model) {
				testingInstance.Fatalf("managed store contains catalog model migration %d %s=%s", migrationIndex, field, model)
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
		filepath.Join(operationalScriptsDirectory, "local_orchestration.sh"),
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

func TestOperationalMakeDownStopsLocalWebOrchestration(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	for _, relativePath := range []string{
		"Makefile",
		"docker-compose.local.yml",
		filepath.Join(operationalScriptsDirectory, "down.sh"),
		filepath.Join(operationalScriptsDirectory, "local_orchestration.sh"),
	} {
		copyOperationalFile(testingInstance, filepath.Join(repositoryRoot, relativePath), filepath.Join(fixtureRoot, relativePath))
	}
	writeOperationalFile(testingInstance, filepath.Join(fixtureRoot, "down"), "phony target guard\n", 0o600)

	toolDirectory := filepath.Join(fixtureRoot, "tools")
	temporaryDirectory := filepath.Join(fixtureRoot, "temporary")
	if createTemporaryDirectoryError := os.MkdirAll(temporaryDirectory, 0o700); createTemporaryDirectoryError != nil {
		testingInstance.Fatalf("create make down temporary directory: %v", createTemporaryDirectoryError)
	}
	composeArgumentsPath := filepath.Join(fixtureRoot, "compose-arguments")
	writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "docker"), `#!/usr/bin/env bash
set -euo pipefail

builtin printf '%s\n' "$*" >>"${DOCKER_ARGUMENT_CAPTURE:?}"
[[ "${1:?}" == "compose" ]]
if [[ "${2:?}" == "version" ]]; then
  [[ "$#" -eq 2 ]]
  exit 0
fi
[[ "${LLM_PROXY_LOCAL_SITE_ARTIFACT_DIRECTORY:?}" == "${EXPECTED_SITE_ARTIFACT_DIRECTORY:?}" ]]
`, 0o755)

	command := exec.Command("make", "down")
	command.Dir = fixtureRoot
	command.Env = append(
		os.Environ(),
		"PATH="+toolDirectory+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TMPDIR="+temporaryDirectory,
		"DOCKER_ARGUMENT_CAPTURE="+composeArgumentsPath,
		"EXPECTED_SITE_ARTIFACT_DIRECTORY="+temporaryDirectory,
	)
	output, commandError := command.CombinedOutput()
	if commandError != nil {
		testingInstance.Fatalf("make down failed: %v\n%s", commandError, output)
	}
	if !strings.Contains(string(output), "LLM Proxy local orchestration stopped.") {
		testingInstance.Fatalf("make down omitted its shutdown receipt: %s", output)
	}

	composeArguments, readComposeArgumentsError := os.ReadFile(composeArgumentsPath)
	if readComposeArgumentsError != nil {
		testingInstance.Fatalf("read make down Compose arguments: %v", readComposeArgumentsError)
	}
	expectedComposeFilePath, resolveComposeFileError := filepath.EvalSymlinks(filepath.Join(fixtureRoot, "docker-compose.local.yml"))
	if resolveComposeFileError != nil {
		testingInstance.Fatalf("resolve make down Compose file: %v", resolveComposeFileError)
	}
	expectedComposeArguments := strings.Join([]string{
		"compose version",
		"compose --project-name llm-proxy-local --file " + expectedComposeFilePath + " down --remove-orphans",
	}, "\n") + "\n"
	if string(composeArguments) != expectedComposeArguments {
		testingInstance.Fatalf("make down used unexpected Compose arguments:\n%s", composeArguments)
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
		filepath.Join(operationalScriptsDirectory, "local_orchestration.sh"),
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
	localSiteArtifactPath := filepath.Join(fixtureRoot, "local-site-artifact-path")
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
    [[ -d "${LLM_PROXY_LOCAL_SITE_ARTIFACT_DIRECTORY:?}" ]]
    builtin printf '%s\n' "${LLM_PROXY_LOCAL_SITE_ARTIFACT_DIRECTORY}" >"${LOCAL_SITE_ARTIFACT_CAPTURE:?}"
    sleep 0.1
    builtin printf '%s\n' started >"${COMPOSE_STARTED_CAPTURE:?}"
    exit 0
    ;;
  ps)
    builtin printf '%s\n' api frontend schema tauth
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
if [[ "${arguments}" != *"--write-out"* ]]; then
  [[ "${arguments}" == *"http://localhost:4179/"* ]]
  builtin printf '%s' '<routing-tree class="routing-tree"></routing-tree><table class="catalog-table"><tr><td>validated public route</td></tr></table>'
  exit 0
fi
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
  *"http://localhost:8080/api/public/capabilities"*)
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
		"LOCAL_SITE_ARTIFACT_CAPTURE="+localSiteArtifactPath,
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

	localSiteArtifactBytes, readLocalSiteArtifactError := os.ReadFile(localSiteArtifactPath)
	if readLocalSiteArtifactError != nil {
		testingInstance.Fatalf("read local site artifact path: %v", readLocalSiteArtifactError)
	}
	localSiteArtifactDirectory := strings.TrimSpace(string(localSiteArtifactBytes))
	if _, statError := os.Stat(localSiteArtifactDirectory); !os.IsNotExist(statError) {
		testingInstance.Fatalf("make up retained temporary local site artifact %s: %v", localSiteArtifactDirectory, statError)
	}

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
		"http://localhost:4179/openapi.yaml",
		"http://localhost:4179/config-ui.yaml",
		"http://localhost:4179/auth/session",
		"http://localhost:4179/auth/nonce",
		"http://localhost:8080/api/public/capabilities",
		"http://localhost:8080/?prompt=ready",
		"http://localhost:8080/api/management/account",
	} {
		if !strings.Contains(string(curlArguments), expectedURL) {
			testingInstance.Fatalf("make up did not verify %s: %s", expectedURL, curlArguments)
		}
	}
	if !strings.Contains(string(curlArguments), "--header X-TAuth-Tenant: llm-proxy-test http://localhost:4179/auth/session") {
		testingInstance.Fatalf("make up did not verify TAuth client session restoration through the same-origin frontend: %s", curlArguments)
	}
	if strings.Contains(string(curlArguments), "Origin: http://localhost:4179 --header X-TAuth-Tenant: llm-proxy-test http://localhost:4179/auth/session") {
		testingInstance.Fatalf("make up hid the same-origin TAuth session request shape behind a synthetic Origin: %s", curlArguments)
	}
	if !strings.Contains(string(curlArguments), "--request POST --header Origin: http://localhost:4179 --header Content-Type: application/json --header X-Requested-With: XMLHttpRequest --header X-TAuth-Tenant: llm-proxy-test http://localhost:4179/auth/nonce") {
		testingInstance.Fatalf("make up did not verify TAuth client nonce issuance through the same-origin frontend: %s", curlArguments)
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
				"GHTTP_SERVE_PROXIES: \"/openapi.yaml=http://schema:4179,/config-ui.yaml=http://api:8080,/auth=http://tauth:8080,/me=http://tauth:8080\"",
				"GHTTP_SERVE_RESPONSE_HEADERS: \"/=Cache-Control:no-store\"",
				"GHTTP_SERVE_DIRECTORY: \"/app/render/site\"",
				"LLM_PROXY_MANAGEMENT_TAUTH_URL: \"http://localhost:4179\"",
				"127.0.0.1:${LLM_PROXY_LOCAL_FRONTEND_HOST_PORT:-4179}:4179",
				"127.0.0.1:${LLM_PROXY_LOCAL_API_HOST_PORT:-8080}:8080",
				"site-builder:",
				"image: node:22-bookworm-slim",
				"/app/scripts/render_public_site.mjs",
				"http://api:8080/api/public/capabilities",
				"condition: service_completed_successfully",
				"./site:/app/site:ro",
				"LLM_PROXY_LOCAL_SITE_ARTIFACT_DIRECTORY",
				"condition: service_healthy",
				"curl --fail --silent http://127.0.0.1:8080/api/public/capabilities",
				"./configs/config.yml:/app/configs/config.yml:ro",
				"./configs/providers.yml:/app/configs/providers.yml:ro",
				"GHTTP_SERVE_DIRECTORY: \"/app/docs\"",
				"./docs:/app/docs:ro",
			},
		},
		{
			path: filepath.Join(fixtureRoot, "configs", "tauth.local.yml"),
			expectedFragments: []string{
				"enable_tenant_header_override: true",
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
	if strings.Contains(string(composeConfiguration), "DASHSCOPE_BASE_URL") {
		testingInstance.Fatalf("local Compose retains a DashScope URL input: %s", composeConfiguration)
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

func TestOperationalLocalComposeLiveAcceptanceOwnsDisposableProject(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	for _, testCase := range []struct {
		name          string
		harnessStatus string
		expectSuccess bool
	}{
		{name: "success", harnessStatus: "0", expectSuccess: true},
		{name: "provider failure", harnessStatus: "37", expectSuccess: false},
	} {
		testingInstance.Run(testCase.name, func(testingInstance *testing.T) {
			fixtureRoot := testingInstance.TempDir()
			for _, relativePath := range []string{
				"Makefile",
				"docker-compose.local.yml",
				filepath.Join(operationalScriptsDirectory, "local_orchestration.sh"),
				filepath.Join(operationalScriptsDirectory, "test_live_local.sh"),
				filepath.Join("configs", "config.yml"),
				filepath.Join("configs", "tauth.local.yml"),
			} {
				copyOperationalFile(testingInstance, filepath.Join(repositoryRoot, relativePath), filepath.Join(fixtureRoot, relativePath))
			}
			writeOperationalLocalEnvironment(testingInstance, fixtureRoot)
			liveEnvironmentPath := filepath.Join(fixtureRoot, "provider.env")
			writeOperationalFile(testingInstance, liveEnvironmentPath, "OPENAI_API_KEY=local-compose-provider-key\n", 0o600)

			toolDirectory := filepath.Join(fixtureRoot, "tools")
			composeArgumentsPath := filepath.Join(fixtureRoot, "compose-arguments")
			composeStartedPath := filepath.Join(fixtureRoot, "compose-started")
			composeDownPath := filepath.Join(fixtureRoot, "compose-down")
			composeProjectPath := filepath.Join(fixtureRoot, "compose-project")
			artifactPathCapture := filepath.Join(fixtureRoot, "artifact-path")
			harnessCapture := filepath.Join(fixtureRoot, "harness-arguments")
			writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "docker"), `#!/usr/bin/env bash
set -euo pipefail

builtin printf '%s\n' "$*" >>"${DOCKER_ARGUMENT_CAPTURE:?}"
[[ "${1:?}" == "compose" ]]
if [[ "${2:?}" == "version" ]]; then
  exit 0
fi
shift
compose_project=""
while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --project-name)
      compose_project="$2"
      shift 2
      ;;
    --file)
      shift 2
      ;;
    *)
      break
      ;;
  esac
done
if [[ -n "${compose_project}" ]]; then
  [[ "${compose_project}" != "${STALE_COMPOSE_PROJECT:?}" ]]
fi
case "${1:?}" in
  ps)
    if [[ "${compose_project}" == llm-proxy-live-test-* && -f "${COMPOSE_STARTED_CAPTURE:?}" && " $* " == *" --status running "* ]]; then
      builtin printf '%s\n' api frontend schema tauth
    fi
    ;;
  up)
    [[ "${compose_project}" == llm-proxy-live-test-* ]]
    [[ -d "${LLM_PROXY_LOCAL_SITE_ARTIFACT_DIRECTORY:?}" ]]
    builtin printf '%s\n' "${LLM_PROXY_LOCAL_SITE_ARTIFACT_DIRECTORY}" >"${ARTIFACT_PATH_CAPTURE:?}"
    builtin printf '%s\n' "${compose_project}" >"${COMPOSE_PROJECT_CAPTURE:?}"
    builtin printf '%s\n' started >"${COMPOSE_STARTED_CAPTURE:?}"
    ;;
  down)
    [[ "${compose_project}" == "$(<"${COMPOSE_PROJECT_CAPTURE:?}")" ]]
    [[ " $* " == *" --volumes "* ]]
    builtin printf '%s\n' down >"${COMPOSE_DOWN_CAPTURE:?}"
    ;;
  *)
    exit 1
    ;;
esac
`, 0o755)
			writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "curl"), `#!/usr/bin/env bash
set -euo pipefail

arguments="$*"
if [[ "${arguments}" != *"--write-out"* ]]; then
  builtin printf '%s' '<routing-tree class="routing-tree"></routing-tree><table class="catalog-table"><tr><td>validated public route</td></tr></table>'
  exit 0
fi
case "${arguments}" in
  *"http://127.0.0.1:${LLM_PROXY_LOCAL_FRONTEND_HOST_PORT:?}/auth/session"*) status=204 ;;
  *"http://127.0.0.1:${LLM_PROXY_LOCAL_FRONTEND_HOST_PORT}/auth/nonce"*) status=200 ;;
  *"http://127.0.0.1:${LLM_PROXY_LOCAL_FRONTEND_HOST_PORT}/"*) status=200 ;;
  *"http://127.0.0.1:${LLM_PROXY_LOCAL_API_HOST_PORT:?}/api/public/capabilities"*) status=200 ;;
  *"http://127.0.0.1:${LLM_PROXY_LOCAL_API_HOST_PORT}/?prompt=ready"*) status=403 ;;
  *"http://127.0.0.1:${LLM_PROXY_LOCAL_API_HOST_PORT}/api/management/account"*) status=401 ;;
  *) exit 1 ;;
esac
builtin printf '%s' "${status}"
`, 0o755)
			writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "openssl"), `#!/usr/bin/env bash
set -euo pipefail
[[ "${1:?}" == "rand" ]]
case "${2:?}" in
  -base64)
    builtin printf '%s' generated-local-live-value
    ;;
  -hex)
    [[ "${3:?}" == "16" ]]
    builtin printf '%s' 0123456789abcdef0123456789abcdef
    ;;
  *)
    exit 1
    ;;
esac
`, 0o755)
			writeOperationalFile(testingInstance, filepath.Join(fixtureRoot, operationalScriptsDirectory, "test_live_providers.sh"), `#!/usr/bin/env bash
set -euo pipefail
builtin printf 'arguments=%s\norigin_env=%s\nproviders=%s\n' "$*" "${LLM_PROXY_LIVE_MANAGEMENT_ENV_FILE:?}" "${LLM_PROXY_LIVE_PROVIDERS:?}" >"${LIVE_HARNESS_CAPTURE:?}"
exit "${LIVE_HARNESS_STATUS:?}"
`, 0o755)

			command := exec.Command("make", "test-live-local-providers")
			command.Dir = fixtureRoot
			command.Env = append(
				os.Environ(),
				"PATH="+toolDirectory+string(os.PathListSeparator)+os.Getenv("PATH"),
				"LIVE_ENV_FILE="+liveEnvironmentPath,
				"LLM_PROXY_LIVE_PROVIDERS=openai",
				"DOCKER_ARGUMENT_CAPTURE="+composeArgumentsPath,
				"COMPOSE_STARTED_CAPTURE="+composeStartedPath,
				"COMPOSE_DOWN_CAPTURE="+composeDownPath,
				"COMPOSE_PROJECT_CAPTURE="+composeProjectPath,
				"STALE_COMPOSE_PROJECT=llm-proxy-live-test",
				"ARTIFACT_PATH_CAPTURE="+artifactPathCapture,
				"LIVE_HARNESS_CAPTURE="+harnessCapture,
				"LIVE_HARNESS_STATUS="+testCase.harnessStatus,
			)
			output, commandError := command.CombinedOutput()
			if testCase.expectSuccess && commandError != nil {
				testingInstance.Fatalf("local Compose live acceptance failed: %v\n%s", commandError, output)
			}
			if !testCase.expectSuccess && commandError == nil {
				testingInstance.Fatalf("local Compose live acceptance ignored provider failure: %s", output)
			}
			if _, readError := os.ReadFile(composeDownPath); readError != nil {
				testingInstance.Fatalf("local Compose live acceptance did not clean up: %v\n%s", readError, output)
			}
			for environmentPath, expectedOrigins := range map[string][]string{
				filepath.Join(fixtureRoot, "configs", ".env.api.local"): {
					"LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN=http://localhost:4179",
					"LLM_PROXY_MANAGEMENT_API_ORIGIN=http://localhost:8080",
				},
				filepath.Join(fixtureRoot, "configs", ".env.tauth.local"): {
					"LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN=http://localhost:4179",
				},
			} {
				environmentBytes, environmentReadError := os.ReadFile(environmentPath)
				if environmentReadError != nil {
					testingInstance.Fatalf("read restored local environment %s: %v", environmentPath, environmentReadError)
				}
				for _, expectedOrigin := range expectedOrigins {
					if !strings.Contains(string(environmentBytes), expectedOrigin) {
						testingInstance.Fatalf("local Compose live acceptance did not restore %q in %s: %s", expectedOrigin, environmentPath, environmentBytes)
					}
				}
			}
			composeArguments, readError := os.ReadFile(composeArgumentsPath)
			if readError != nil {
				testingInstance.Fatalf("read local live Compose arguments: %v", readError)
			}
			composeProjectBytes, projectReadError := os.ReadFile(composeProjectPath)
			if projectReadError != nil {
				testingInstance.Fatalf("read local live Compose project: %v", projectReadError)
			}
			composeProject := strings.TrimSpace(string(composeProjectBytes))
			if !regexp.MustCompile(`^llm-proxy-live-test-[0-9a-f]{32}$`).MatchString(composeProject) {
				testingInstance.Fatalf("local live Compose project is not unique: %q", composeProject)
			}
			for _, expectedFragment := range []string{
				"--project-name " + composeProject,
				"up --build --remove-orphans --wait",
				"down --remove-orphans --volumes",
			} {
				if !strings.Contains(string(composeArguments), expectedFragment) {
					testingInstance.Fatalf("local live Compose arguments omitted %q: %s", expectedFragment, composeArguments)
				}
			}
			harnessArguments, readError := os.ReadFile(harnessCapture)
			if readError != nil {
				testingInstance.Fatalf("read local live harness arguments: %v", readError)
			}
			resolvedFixtureRoot, resolveError := filepath.EvalSymlinks(fixtureRoot)
			if resolveError != nil {
				testingInstance.Fatalf("resolve local live fixture root: %v", resolveError)
			}
			for _, expectedFragment := range []string{
				"arguments=--existing-local-origin http://127.0.0.1:",
				"origin_env=" + filepath.Join(resolvedFixtureRoot, "configs", ".env.api.local"),
				"providers=openai",
			} {
				if !strings.Contains(string(harnessArguments), expectedFragment) {
					testingInstance.Fatalf("local live harness omitted %q: %s", expectedFragment, harnessArguments)
				}
			}
			artifactPathBytes, readError := os.ReadFile(artifactPathCapture)
			if readError != nil {
				testingInstance.Fatalf("read local live artifact path: %v", readError)
			}
			artifactPath := strings.TrimSpace(string(artifactPathBytes))
			if _, statError := os.Stat(artifactPath); !os.IsNotExist(statError) {
				testingInstance.Fatalf("local live acceptance retained temporary artifact %s: %v", artifactPath, statError)
			}
			if testCase.expectSuccess && !strings.Contains(string(output), "local Compose live-provider acceptance passed: providers=openai") {
				testingInstance.Fatalf("local Compose live acceptance omitted success receipt: %s", output)
			}
		})
	}
}

func TestOperationalMakeUpRetainsSiteArtifactWhenAutomaticShutdownFails(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	for _, relativePath := range []string{
		"Makefile",
		"docker-compose.local.yml",
		filepath.Join(operationalScriptsDirectory, "up.sh"),
		filepath.Join(operationalScriptsDirectory, "local_orchestration.sh"),
	} {
		copyOperationalFile(testingInstance, filepath.Join(repositoryRoot, relativePath), filepath.Join(fixtureRoot, relativePath))
	}
	writeOperationalLocalEnvironment(testingInstance, fixtureRoot)

	toolDirectory := filepath.Join(fixtureRoot, "tools")
	temporaryDirectory := filepath.Join(fixtureRoot, "temporary")
	if createTemporaryDirectoryError := os.MkdirAll(temporaryDirectory, 0o700); createTemporaryDirectoryError != nil {
		testingInstance.Fatalf("create failed-shutdown temporary directory: %v", createTemporaryDirectoryError)
	}
	localSiteArtifactPath := filepath.Join(fixtureRoot, "local-site-artifact-path")
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
  version|logs)
    exit 0
    ;;
  up)
    [[ -d "${LLM_PROXY_LOCAL_SITE_ARTIFACT_DIRECTORY:?}" ]]
    builtin printf '%s\n' "${LLM_PROXY_LOCAL_SITE_ARTIFACT_DIRECTORY}" >"${LOCAL_SITE_ARTIFACT_CAPTURE:?}"
    ;;
  ps)
    builtin printf '%s\n' api frontend schema tauth
    ;;
  down)
    exit 41
    ;;
  *)
    exit 1
    ;;
esac
`, 0o755)
	writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "curl"), `#!/usr/bin/env bash
set -euo pipefail

arguments="$*"
if [[ "${arguments}" != *"--write-out"* ]]; then
  builtin printf '%s' '<routing-tree class="routing-tree"></routing-tree><table class="catalog-table"><tr><td>validated public route</td></tr></table>'
  exit 0
fi
case "${arguments}" in
  *"http://localhost:4179/auth/session"*)
    status=204
    ;;
  *"http://localhost:4179/auth/nonce"*|*"http://localhost:4179/"*)
    status=200
    ;;
  *"http://localhost:8080/api/public/capabilities"*)
    status=200
    ;;
  *"http://localhost:8080/?prompt=ready"*)
    status=403
    ;;
  *"http://localhost:8080/api/management/account"*)
    status=401
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

	command := exec.Command("make", "up")
	command.Dir = fixtureRoot
	command.Env = append(
		os.Environ(),
		"PATH="+toolDirectory+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TMPDIR="+temporaryDirectory,
		"LOCAL_SITE_ARTIFACT_CAPTURE="+localSiteArtifactPath,
	)
	output, commandError := command.CombinedOutput()
	if _, isExitError := commandError.(*exec.ExitError); !isExitError {
		testingInstance.Fatalf("failed automatic shutdown exit=%v, want nonzero\n%s", commandError, output)
	}
	if !strings.Contains(string(output), "error: local orchestration cleanup failed") {
		testingInstance.Fatalf("failed automatic shutdown omitted cleanup error: %s", output)
	}
	localSiteArtifactBytes, readLocalSiteArtifactError := os.ReadFile(localSiteArtifactPath)
	if readLocalSiteArtifactError != nil {
		testingInstance.Fatalf("read retained local site artifact path: %v", readLocalSiteArtifactError)
	}
	localSiteArtifactDirectory := strings.TrimSpace(string(localSiteArtifactBytes))
	if _, statError := os.Stat(localSiteArtifactDirectory); statError != nil {
		testingInstance.Fatalf("failed automatic shutdown removed mounted artifact %s: %v", localSiteArtifactDirectory, statError)
	}
}

func TestOperationalMakeUpRejectsAnotherProcessReadinessResponse(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	for _, relativePath := range []string{
		"Makefile",
		"docker-compose.local.yml",
		filepath.Join(operationalScriptsDirectory, "up.sh"),
		filepath.Join(operationalScriptsDirectory, "local_orchestration.sh"),
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

if [[ "${request_url}" == "https://llm-proxy-api.mprlab.com/v2/identity?key="* ]]; then
  builtin printf 'HTTP/1.1 200 OK\r\nX-LLM-Proxy-Request-ID: AAAAAAAAAAAAAAAAAAAAAAAAAA\r\n\r\n' >"${headers_path}"
  builtin printf '{"tenant_id":"%s"}' "${FAKE_TENANT_ID:?}" >"${response_path}"
  builtin printf '%s' '200'
  exit 0
fi

[[ -n "${request_timeout_seconds}" ]]

response_marker="LLM_PROXY_LIVE_ECHO_OK"
if [[ "${request_body}" == *LLM_PROXY_LIVE_COMPLEX_OK* ]]; then
  response_marker="LLM_PROXY_LIVE_COMPLEX_OK"
fi
builtin printf 'HTTP/1.1 200 OK\r\nX-LLM-Proxy-Request-Timeout-Seconds: %s\r\nX-LLM-Proxy-Request-ID: AAAAAAAAAAAAAAAAAAAAAAAAAA\r\n\r\n' "${request_timeout_seconds}" >"${headers_path}"
builtin printf '%s' "${response_marker}" >"${response_path}"
builtin printf '%s' '200'
`, 0o755)
	defaultTenantSecret := "default-tenant-client-secret"
	defaultTenantID := "managed-11111111111111111111111111111111"
	environment := append(
		os.Environ(),
		"PATH="+toolDirectory+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CURL_CAPTURE_DIRECTORY="+captureDirectory,
		"LLM_PROXY_DEFAULT_TENANT_KEY="+defaultTenantSecret,
		"LLM_PROXY_EXPECTED_TENANT_ID="+defaultTenantID,
		"FAKE_TENANT_ID="+defaultTenantID,
	)
	output := runOperationalCommand(testingInstance, fixtureRoot, environment, "make", "live-test")
	if strings.Contains(output, defaultTenantSecret) {
		testingInstance.Fatalf("production live-test output exposed the tenant secret: %s", output)
	}
	if !strings.Contains(output, "live test passed: total_cases=9") {
		testingInstance.Fatalf("production live-test did not report all cases: %s", output)
	}
	if !strings.Contains(output, "live test identity preflight passed: tenant=expected status=200") {
		testingInstance.Fatalf("production live-test omitted the tenant identity preflight: %s", output)
	}
	if strings.Count(output, "request_id=AAAAAAAAAAAAAAAAAAAAAAAAAA") != 10 {
		testingInstance.Fatalf("production live-test did not report one validated request id per case: %s", output)
	}
	preflightURLBytes, preflightURLError := os.ReadFile(filepath.Join(captureDirectory, "url-1"))
	if preflightURLError != nil {
		testingInstance.Fatalf("read production live-test preflight URL: %v", preflightURLError)
	}
	preflightURL, preflightParseError := url.Parse(string(preflightURLBytes))
	if preflightParseError != nil || preflightURL.Path != "/v2/identity" || preflightURL.Query().Get("key") != defaultTenantSecret || preflightURL.Query().Has("provider") {
		testingInstance.Fatalf("production live-test used invalid preflight URL: %s error=%v", preflightURLBytes, preflightParseError)
	}
	for _, expectedCase := range []string{
		"case=openai-background-polling provider=openai status=200",
		"case=anthropic-long-completion provider=anthropic status=200",
		"case=meta-long-completion provider=meta status=200",
		"case=gemini-background-polling provider=gemini status=200",
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
	expectedProviders := []string{"openai", "anthropic", "meta", "gemini", "moonshot", "openai", "anthropic", "meta", "gemini"}
	for callIndex, expectedProvider := range expectedProviders {
		captureIndex := callIndex + 2
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
		if captureIndex == 10 {
			if query.Get("model") != "gemini-3.5-flash" {
				testingInstance.Fatalf("production Gemini background call used model=%q", query.Get("model"))
			}
		} else if query.Has("model") {
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
		expectedMaxTokens := 512
		if expectedProvider == "gemini" {
			expectedMaxTokens = 32768
		}
		if len(requestBody) < 16384 || len(payload.Messages) != 2 || payload.MaxTokens == nil || *payload.MaxTokens != expectedMaxTokens || !strings.Contains(payload.Messages[1].Content, "LLM_PROXY_LIVE_COMPLEX_OK") || !strings.Contains(payload.Messages[1].Content, "all 120 normalized lines") {
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
	if !strings.Contains(string(output), "LLM_PROXY_DEFAULT_TENANT_KEY must contain the Default-tenant client secret") {
		testingInstance.Fatalf("missing-secret error omitted the Default-tenant contract: %s", output)
	}

	command = exec.Command("make", "live-test")
	command.Dir = fixtureRoot
	command.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"LLM_PROXY_DEFAULT_TENANT_KEY=default-tenant-client-secret",
	}
	output, commandError = command.CombinedOutput()
	if commandError == nil {
		testingInstance.Fatalf("make live-test accepted a missing Default-tenant identifier: %s", output)
	}
	if !strings.Contains(string(output), "LLM_PROXY_EXPECTED_TENANT_ID must contain the Default-tenant identifier") {
		testingInstance.Fatalf("missing-identifier error omitted the Default-tenant contract: %s", output)
	}
}

func TestOperationalProductionLiveTestStopsWhenClientKeyIsRejected(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	for _, relativePath := range []string{
		"Makefile",
		filepath.Join(operationalScriptsDirectory, "live_test.sh"),
	} {
		copyOperationalFile(testingInstance, filepath.Join(repositoryRoot, relativePath), filepath.Join(fixtureRoot, relativePath))
	}

	toolDirectory := filepath.Join(fixtureRoot, "tools")
	callCountPath := filepath.Join(fixtureRoot, "curl-call-count")
	writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "curl"), `#!/usr/bin/env bash
set -euo pipefail

headers_path=""
response_path=""
while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --dump-header)
      headers_path="$2"
      shift 2
      ;;
    --output)
      response_path="$2"
      shift 2
      ;;
    --config|--request|--header|--connect-timeout|--max-time|--write-out|--data-binary)
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
[[ -n "${headers_path}" ]]
[[ -n "${response_path}" ]]
cat >/dev/null
call_count=0
if [[ -f "${CURL_CALL_COUNT_PATH:?}" ]]; then
  call_count="$(<"${CURL_CALL_COUNT_PATH}")"
fi
builtin printf '%s' "$((call_count + 1))" >"${CURL_CALL_COUNT_PATH}"
builtin printf 'HTTP/1.1 403 Forbidden\r\nX-LLM-Proxy-Request-ID: BBBBBBBBBBBBBBBBBBBBBBBBBB\r\n\r\n' >"${headers_path}"
builtin printf '%s' 'forbidden' >"${response_path}"
builtin printf '%s' '403'
`, 0o755)

	rejectedTenantSecret := "rejected-default-tenant-secret"
	command := exec.Command("make", "live-test")
	command.Dir = fixtureRoot
	command.Env = append(
		os.Environ(),
		"PATH="+toolDirectory+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CURL_CALL_COUNT_PATH="+callCountPath,
		"LLM_PROXY_DEFAULT_TENANT_KEY="+rejectedTenantSecret,
		"LLM_PROXY_EXPECTED_TENANT_ID=managed-11111111111111111111111111111111",
	)
	output, commandError := command.CombinedOutput()
	if commandError == nil {
		testingInstance.Fatalf("make live-test accepted a rejected client key: %s", output)
	}
	outputText := string(output)
	if !strings.Contains(outputText, "live test identity preflight failed: client_key=rejected status=403") {
		testingInstance.Fatalf("make live-test did not report the rejected client key: %s", outputText)
	}
	callCount, readCallCountError := os.ReadFile(callCountPath)
	if readCallCountError != nil || string(callCount) != "1" {
		testingInstance.Fatalf("make live-test made provider calls after rejected client key: count=%s error=%v output=%s", callCount, readCallCountError, outputText)
	}
	for _, forbiddenValue := range []string{rejectedTenantSecret, "case=", "LLM_PROXY_LIVE_ECHO_OK", "LLM_PROXY_LIVE_COMPLEX_OK"} {
		if strings.Contains(outputText, forbiddenValue) {
			testingInstance.Fatalf("make live-test rejected-key failure exposed or executed %q: %s", forbiddenValue, outputText)
		}
	}
}

func TestOperationalProductionLiveTestStopsForUnexpectedTenant(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	for _, relativePath := range []string{
		"Makefile",
		filepath.Join(operationalScriptsDirectory, "live_test.sh"),
	} {
		copyOperationalFile(testingInstance, filepath.Join(repositoryRoot, relativePath), filepath.Join(fixtureRoot, relativePath))
	}

	toolDirectory := filepath.Join(fixtureRoot, "tools")
	callCountPath := filepath.Join(fixtureRoot, "curl-call-count")
	writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "curl"), `#!/usr/bin/env bash
set -euo pipefail

headers_path=""
response_path=""
while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --dump-header)
      headers_path="$2"
      shift 2
      ;;
    --output)
      response_path="$2"
      shift 2
      ;;
    --config|--request|--connect-timeout|--max-time|--write-out)
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
[[ -n "${headers_path}" ]]
[[ -n "${response_path}" ]]
call_count=0
if [[ -f "${CURL_CALL_COUNT_PATH:?}" ]]; then
  call_count="$(<"${CURL_CALL_COUNT_PATH}")"
fi
builtin printf '%s' "$((call_count + 1))" >"${CURL_CALL_COUNT_PATH}"
builtin printf 'HTTP/1.1 200 OK\r\nX-LLM-Proxy-Request-ID: CCCCCCCCCCCCCCCCCCCCCCCCCC\r\n\r\n' >"${headers_path}"
builtin printf '%s' '{"tenant_id":"managed-22222222222222222222222222222222"}' >"${response_path}"
builtin printf '%s' '200'
`, 0o755)

	unexpectedTenantSecret := "unexpected-tenant-secret"
	command := exec.Command("make", "live-test")
	command.Dir = fixtureRoot
	command.Env = append(
		os.Environ(),
		"PATH="+toolDirectory+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CURL_CALL_COUNT_PATH="+callCountPath,
		"LLM_PROXY_DEFAULT_TENANT_KEY="+unexpectedTenantSecret,
		"LLM_PROXY_EXPECTED_TENANT_ID=managed-11111111111111111111111111111111",
	)
	output, commandError := command.CombinedOutput()
	if commandError == nil {
		testingInstance.Fatalf("make live-test accepted an unexpected tenant: %s", output)
	}
	outputText := string(output)
	if !strings.Contains(outputText, "live test identity preflight failed: client_key=unexpected_tenant status=200") {
		testingInstance.Fatalf("make live-test did not report the unexpected tenant: %s", outputText)
	}
	callCount, readCallCountError := os.ReadFile(callCountPath)
	if readCallCountError != nil || string(callCount) != "1" {
		testingInstance.Fatalf("make live-test made provider calls for an unexpected tenant: count=%s error=%v output=%s", callCount, readCallCountError, outputText)
	}
	for _, forbiddenValue := range []string{unexpectedTenantSecret, "managed-11111111111111111111111111111111", "managed-22222222222222222222222222222222", "case=", "LLM_PROXY_LIVE_ECHO_OK", "LLM_PROXY_LIVE_COMPLEX_OK"} {
		if strings.Contains(outputText, forbiddenValue) {
			testingInstance.Fatalf("make live-test unexpected-tenant failure exposed or executed %q: %s", forbiddenValue, outputText)
		}
	}
}

func TestOperationalProductionLiveTestDoesNotInventRequestIDForTransportFailure(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	for _, relativePath := range []string{
		"Makefile",
		filepath.Join(operationalScriptsDirectory, "live_test.sh"),
	} {
		copyOperationalFile(testingInstance, filepath.Join(repositoryRoot, relativePath), filepath.Join(fixtureRoot, relativePath))
	}

	toolDirectory := filepath.Join(fixtureRoot, "tools")
	writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "curl"), `#!/usr/bin/env bash
exit 7
`, 0o755)
	defaultTenantSecret := "transport-failure-tenant-secret"
	command := exec.Command("make", "live-test")
	command.Dir = fixtureRoot
	command.Env = append(
		os.Environ(),
		"PATH="+toolDirectory+string(os.PathListSeparator)+os.Getenv("PATH"),
		"LLM_PROXY_DEFAULT_TENANT_KEY="+defaultTenantSecret,
		"LLM_PROXY_EXPECTED_TENANT_ID=managed-11111111111111111111111111111111",
	)
	output, commandError := command.CombinedOutput()
	if commandError == nil {
		testingInstance.Fatalf("make live-test accepted transport failures: %s", output)
	}
	outputText := string(output)
	if strings.Count(outputText, "transport_error") != 1 || !strings.Contains(outputText, "live test identity preflight failed: client_key=transport_error") {
		testingInstance.Fatalf("make live-test did not stop at the tenant identity preflight: %s", outputText)
	}
	for _, forbiddenValue := range []string{"request_id=", defaultTenantSecret, "LLM_PROXY_LIVE_ECHO_OK", "LLM_PROXY_LIVE_COMPLEX_OK"} {
		if strings.Contains(outputText, forbiddenValue) {
			testingInstance.Fatalf("make live-test transport failure exposed or invented %q: %s", forbiddenValue, outputText)
		}
	}
}

func TestOperationalLiveConfigRequiresManagementAndSafelyLoadsDotenv(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	environmentFile := filepath.Join(fixtureRoot, "live.env")
	configurationOutput := filepath.Join(fixtureRoot, "live-config.yml")
	writeOperationalFile(testingInstance, environmentFile, "DASHSCOPE_API_KEY=test-dashscope-key\nDASHSCOPE_BASE_URL=https://dashscope.example\nMINIMAX_API_KEY=test-minimax-key\nLLM_PROXY_MANAGEMENT_UI_DESCRIPTION=LLM Proxy\n", 0o600)
	environment := append(
		os.Environ(),
		"LIVE_ENV_FILE="+environmentFile,
		"LLM_PROXY_LIVE_PROVIDERS=dashscope,minimax",
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
	if !strings.Contains(configuration, "management:\n  public_origin:") {
		testingInstance.Fatalf("generated live config omitted mandatory management: %s", configuration)
	}
	for _, forbiddenFragment := range []string{"enabled:", "tenants:", "api_key:", "test-dashscope-key", "test-minimax-key"} {
		if strings.Contains(configuration, forbiddenFragment) {
			testingInstance.Fatalf("generated live config retained obsolete or secret-bearing %q: %s", forbiddenFragment, configuration)
		}
	}
}

func TestOperationalGeminiCandidateHarnessExercisesExactAcceptanceMatrix(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	toolDirectory := filepath.Join(fixtureRoot, "tools")
	environmentFile := filepath.Join(fixtureRoot, "live.env")
	capturePath := filepath.Join(fixtureRoot, "requests.log")
	stateDirectory := filepath.Join(fixtureRoot, "state")
	const providerCredential = "private-gemini-candidate-credential"
	writeOperationalFile(testingInstance, environmentFile, "GEMINI_API_KEY="+providerCredential+"\n", 0o600)
	writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "sleep"), `#!/usr/bin/env bash
exit 0
`, 0o755)
	writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "curl"), `#!/usr/bin/env python3
import json
import os
import pathlib
import sys

arguments = sys.argv[1:]
method = arguments[arguments.index("-X") + 1]
output_path = pathlib.Path(arguments[arguments.index("-o") + 1])
url = arguments[-1]
headers = [arguments[index + 1] for index, value in enumerate(arguments) if value == "-H"]
if not any(value.startswith("x-goog-api-key: ") for value in headers):
    raise SystemExit(41)
if "Api-Revision: 2026-05-20" not in headers:
    raise SystemExit(42)
payload = {}
if "--data-binary" in arguments:
    request_reference = arguments[arguments.index("--data-binary") + 1]
    payload = json.loads(pathlib.Path(request_reference.removeprefix("@")).read_text(encoding="utf-8"))

state_directory = pathlib.Path(os.environ["GEMINI_FAKE_STATE"])
state_directory.mkdir(parents=True, exist_ok=True)
model = payload.get("model", "")
thinking_level = payload.get("generation_config", {}).get("thinking_level", "omitted")
background = payload.get("background", "")
lifecycle = ""
if background is True:
    lifecycle = "completion" if payload["generation_config"]["max_output_tokens"] == 4096 else "cancellation"
    interaction_id = model + "-" + lifecycle
else:
    interaction_id = "sync"

with pathlib.Path(os.environ["GEMINI_FAKE_CAPTURE"]).open("a", encoding="utf-8") as capture:
    capture.write(f"{method} {url} model={model} background={background} thinking_level={thinking_level} lifecycle={lifecycle}\n")

status_code = "200"
response = {}
base_url = "https://generativelanguage.googleapis.com/v1beta/interactions"
if method == "POST" and url == base_url and background is False:
    response = {"id": interaction_id, "status": "completed", "steps": [{"type": "model_output", "content": [{"type": "text", "text": "OK"}]}]}
elif method == "POST" and url == base_url and background is True:
    response = {"id": interaction_id, "status": "in_progress"}
elif method == "GET" and url.startswith(base_url + "/"):
    interaction_id = url.rsplit("/", 1)[-1]
    if interaction_id.endswith("-completion"):
        count_path = state_directory / (interaction_id + ".count")
        count = int(count_path.read_text(encoding="utf-8")) if count_path.exists() else 0
        count_path.write_text(str(count + 1), encoding="utf-8")
        if count == 0:
            response = {"id": interaction_id, "status": "in_progress"}
        else:
            response = {"id": interaction_id, "status": "completed", "steps": [{"type": "model_output", "content": [{"type": "text", "text": "OK"}]}]}
    else:
        response = {"id": interaction_id, "status": "in_progress"}
elif method == "POST" and url.endswith("/cancel"):
    interaction_id = url.removesuffix("/cancel").rsplit("/", 1)[-1]
    response = {"id": interaction_id, "status": "cancelled"}
elif method == "DELETE" and url.startswith(base_url + "/"):
    status_code = "204"
else:
    raise SystemExit(43)

output_path.write_text(json.dumps(response), encoding="utf-8")
sys.stdout.write(status_code)
`, 0o755)

	environment := append(
		os.Environ(),
		"PATH="+toolDirectory+string(os.PathListSeparator)+os.Getenv("PATH"),
		"LIVE_ENV_FILE="+environmentFile,
		"GEMINI_FAKE_CAPTURE="+capturePath,
		"GEMINI_FAKE_STATE="+stateDirectory,
	)
	output := runOperationalCommand(
		testingInstance,
		repositoryRoot,
		environment,
		filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_providers.sh"),
		"--gemini-candidates",
	)
	for _, expectedFragment := range []string{
		"model=gemini-3.6-flash reasoning_effort=minimal status=200",
		"model=gemini-3.7-flash reasoning_effort=high status=200",
		"completion lifecycle passed: model=gemini-3.6-flash status=completed",
		"cancellation lifecycle passed: model=gemini-3.7-flash status=cancelled",
		"live Gemini candidate acceptance passed: models=gemini-3.6-flash,gemini-3.7-flash",
	} {
		if !strings.Contains(output, expectedFragment) {
			testingInstance.Fatalf("candidate output omitted %q:\n%s", expectedFragment, output)
		}
	}
	captureBytes, readError := os.ReadFile(capturePath)
	if readError != nil {
		testingInstance.Fatalf("read Gemini candidate request capture: %v", readError)
	}
	capture := string(captureBytes)
	for _, exactCase := range []string{
		"model=gemini-3.6-flash background=False thinking_level=omitted",
		"model=gemini-3.6-flash background=False thinking_level=minimal",
		"model=gemini-3.6-flash background=False thinking_level=low",
		"model=gemini-3.6-flash background=False thinking_level=medium",
		"model=gemini-3.6-flash background=False thinking_level=high",
		"model=gemini-3.7-flash background=False thinking_level=omitted",
		"model=gemini-3.7-flash background=False thinking_level=low",
		"model=gemini-3.7-flash background=False thinking_level=medium",
		"model=gemini-3.7-flash background=False thinking_level=high",
	} {
		if !strings.Contains(capture, exactCase) {
			testingInstance.Fatalf("candidate request capture omitted %q:\n%s", exactCase, capture)
		}
	}
	if strings.Contains(capture, "model=gemini-3.7-flash background=False thinking_level=minimal") {
		testingInstance.Fatalf("candidate request capture sent unsupported Gemini 3.7 minimal thinking:\n%s", capture)
	}
	if strings.Count(capture, " POST ") != 0 || strings.Count(capture, "POST ") != 15 || strings.Count(capture, "GET ") != 6 || strings.Count(capture, "DELETE ") != 4 {
		testingInstance.Fatalf("candidate request capture has an unexpected lifecycle matrix:\n%s", capture)
	}
	for _, privateValue := range []string{providerCredential, "x-goog-api-key"} {
		if strings.Contains(output, privateValue) || strings.Contains(capture, privateValue) {
			testingInstance.Fatalf("candidate harness exposed private value %q", privateValue)
		}
	}
}

func TestOperationalGeminiCandidateHarnessPreservesExplicitReasoningMatrixOverride(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	scriptDirectory := filepath.Join(fixtureRoot, operationalScriptsDirectory)
	capturePath := filepath.Join(fixtureRoot, "reasoning-matrix.log")
	copyOperationalFile(
		testingInstance,
		filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_providers.sh"),
		filepath.Join(scriptDirectory, "test_live_providers.sh"),
	)
	writeOperationalFile(testingInstance, filepath.Join(scriptDirectory, "test_live_gemini_candidates.sh"), `#!/usr/bin/env bash
set -euo pipefail
builtin printf '%s\n' "${LLM_PROXY_LIVE_REASONING_MATRIX:?}" >"${GEMINI_MATRIX_CAPTURE:?}"
`, 0o755)
	runOperationalCommand(
		testingInstance,
		fixtureRoot,
		append(
			os.Environ(),
			"GEMINI_MATRIX_CAPTURE="+capturePath,
			"LLM_PROXY_LIVE_REASONING_MATRIX=false",
		),
		filepath.Join(scriptDirectory, "test_live_providers.sh"),
		"--gemini-candidates",
	)
	captureBytes, readError := os.ReadFile(capturePath)
	if readError != nil {
		testingInstance.Fatalf("read Gemini reasoning-matrix override: %v", readError)
	}
	if string(captureBytes) != "false\n" {
		testingInstance.Fatalf("Gemini reasoning-matrix override=%q, want false", captureBytes)
	}
}

func TestOperationalGeminiWrapperRunsCandidatesBeforeRegisteredRoutes(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	scriptDirectory := filepath.Join(fixtureRoot, "scripts")
	capturePath := filepath.Join(fixtureRoot, "wrapper.log")
	copyOperationalFile(
		testingInstance,
		filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_gemini.sh"),
		filepath.Join(scriptDirectory, "test_live_gemini.sh"),
	)
	writeOperationalFile(testingInstance, filepath.Join(scriptDirectory, "test_live_providers.sh"), `#!/usr/bin/env bash
set -euo pipefail
builtin printf '%s\n' "$*" >>"${GEMINI_WRAPPER_CAPTURE:?}"
`, 0o755)
	runOperationalCommand(
		testingInstance,
		fixtureRoot,
		append(os.Environ(), "GEMINI_WRAPPER_CAPTURE="+capturePath),
		filepath.Join(scriptDirectory, "test_live_gemini.sh"),
	)
	captureBytes, readError := os.ReadFile(capturePath)
	if readError != nil {
		testingInstance.Fatalf("read Gemini wrapper capture: %v", readError)
	}
	if string(captureBytes) != "--gemini-candidates\n\n" {
		testingInstance.Fatalf("Gemini wrapper invocation order=%q, want candidate then registered routes", captureBytes)
	}
}

func TestOperationalLiveConfigRejectsDuplicateDotenvNames(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	environmentFile := filepath.Join(fixtureRoot, "live.env")
	configurationOutput := filepath.Join(fixtureRoot, "live-config.yml")
	const firstCredential = "first-provider-credential"
	const secondCredential = "second-provider-credential"
	writeOperationalFile(
		testingInstance,
		environmentFile,
		"OPENAI_API_KEY="+firstCredential+"\nOPENAI_API_KEY="+secondCredential+"\n",
		0o600,
	)
	command := exec.Command(
		filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_providers.sh"),
		"--write-config",
		configurationOutput,
	)
	command.Dir = repositoryRoot
	command.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"LIVE_ENV_FILE=" + environmentFile,
		"LLM_PROXY_LIVE_PORT=18181",
	}
	output, commandError := command.CombinedOutput()
	if commandError == nil {
		testingInstance.Fatalf("live provider harness accepted a duplicate dotenv name: %s", output)
	}
	outputText := string(output)
	if !strings.Contains(outputText, "duplicate dotenv name: "+environmentFile+":2: OPENAI_API_KEY") {
		testingInstance.Fatalf("live provider harness did not identify the duplicate dotenv name: %s", outputText)
	}
	for _, credential := range []string{firstCredential, secondCredential} {
		if strings.Contains(outputText, credential) {
			testingInstance.Fatalf("live provider harness exposed a duplicate credential: %s", outputText)
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
	if !strings.Contains(configuration, "management:\n  public_origin:") {
		testingInstance.Fatalf("generated live config omitted mandatory management: %s", configuration)
	}
	for _, forbiddenFragment := range []string{"enabled:", "tenants:", "api_key:"} {
		if strings.Contains(configuration, forbiddenFragment) {
			testingInstance.Fatalf("generated live config retained obsolete %q: %s", forbiddenFragment, configuration)
		}
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
	const (
		providerKey          = "test-live-openai-key"
		inheritedProviderKey = "stale-inherited-openai-key"
	)
	writeOperationalFile(testingInstance, environmentFile, "OPENAI_API_KEY="+providerKey+"\n", 0o600)
	environment := []string{
		"PATH=" + fixture.toolDirectory + string(os.PathListSeparator) + os.Getenv("PATH"),
		"GO=" + filepath.Join(fixture.toolDirectory, "go"),
		"LLM_PROXY_LIVE_PORT=" + strconv.Itoa(operationalLoopbackPort(testingInstance)),
		"PROXY_PID_CAPTURE=" + fixture.proxyPIDPath,
		"LIVE_ENV_FILE=" + environmentFile,
		"OPENAI_API_KEY=" + inheritedProviderKey,
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
	if strings.Contains(outputText, providerKey) || strings.Contains(outputText, inheritedProviderKey) || strings.Contains(outputText, "live-generated-secret") {
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
	expectedPayload := `payload {"fields":{"api_key":"` + providerKey + `"},"text_model":"gpt-4.1","system_prompt":""}`
	if !strings.Contains(capture, expectedPayload) {
		testingInstance.Fatalf("live verification payload mismatch: %s", capture)
	}
	assertOperationalProxyChildStopped(testingInstance, fixture.proxyPIDPath)
}

func TestOperationalLiveHarnessRejectsInheritedProviderCredentialMissingFromFile(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixture := newOperationalLiveHarnessFixture(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	environmentFile := filepath.Join(fixtureRoot, "live.env")
	writeOperationalFile(testingInstance, environmentFile, "UNRELATED_LIVE_VALUE=set\n", 0o600)
	command := exec.Command(filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_providers.sh"))
	command.Dir = repositoryRoot
	command.Env = []string{
		"PATH=" + fixture.toolDirectory + string(os.PathListSeparator) + os.Getenv("PATH"),
		"GO=" + filepath.Join(fixture.toolDirectory, "go"),
		"LLM_PROXY_LIVE_PORT=" + strconv.Itoa(operationalLoopbackPort(testingInstance)),
		"PROXY_PID_CAPTURE=" + fixture.proxyPIDPath,
		"LIVE_ENV_FILE=" + environmentFile,
		"OPENAI_API_KEY=stale-inherited-openai-key",
		"LLM_PROXY_LIVE_PROVIDERS=openai",
	}
	output, commandError := command.CombinedOutput()
	if commandError == nil {
		testingInstance.Fatalf("live provider harness accepted an inherited provider key: %s", output)
	}
	if !strings.Contains(string(output), "openai requested but required catalog environment is not set: OPENAI_API_KEY") {
		testingInstance.Fatalf("live provider harness reported the wrong missing-file error: %s", output)
	}
	if _, statError := os.Stat(fixture.proxyPIDPath); !os.IsNotExist(statError) {
		testingInstance.Fatalf("live provider harness started a proxy with an inherited provider key: %v", statError)
	}
}

func TestOperationalLiveHarnessRoutesThroughExistingLocalOriginWithoutStartingProxy(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixture := newOperationalLiveHarnessFixture(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	environmentFile := filepath.Join(fixtureRoot, "live.env")
	managementEnvironmentFile := filepath.Join(fixtureRoot, "management.env")
	operationCapture := filepath.Join(fixtureRoot, "operations.log")
	const (
		providerKey          = "test-existing-local-openai-key"
		inheritedProviderKey = "stale-existing-local-openai-key"
		managementSigningKey = "existing-local-signing-key"
		managementIssuer     = "tauth"
		managementCookieName = "existing_local_session"
		managementTenantID   = "existing-local-tenant"
	)
	writeOperationalFile(testingInstance, environmentFile, strings.Join([]string{
		"OPENAI_API_KEY=" + providerKey,
		"LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY=stale-file-signing-key",
		"LLM_PROXY_MANAGEMENT_JWT_ISSUER=stale-file-issuer",
		"LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME=stale_file_session",
		"LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID=stale-file-tenant",
	}, "\n")+"\n", 0o600)
	writeOperationalFile(testingInstance, managementEnvironmentFile, strings.Join([]string{
		"LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY=" + managementSigningKey,
		"LLM_PROXY_MANAGEMENT_JWT_ISSUER=" + managementIssuer,
		"LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME=" + managementCookieName,
		"LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID=" + managementTenantID,
	}, "\n")+"\n", 0o600)
	localOrigin := "http://127.0.0.1:" + strconv.Itoa(operationalLoopbackPort(testingInstance))
	command := exec.Command(
		filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_providers.sh"),
		"--existing-local-origin",
		localOrigin,
	)
	command.Dir = repositoryRoot
	command.Env = []string{
		"PATH=" + fixture.toolDirectory + string(os.PathListSeparator) + os.Getenv("PATH"),
		"GO=" + filepath.Join(fixture.toolDirectory, "go"),
		"PROXY_PID_CAPTURE=" + fixture.proxyPIDPath,
		"ALLOW_EXISTING_LOCAL_ORIGIN=true",
		"LIVE_ENV_FILE=" + environmentFile,
		"OPENAI_API_KEY=" + inheritedProviderKey,
		"LLM_PROXY_LIVE_MANAGEMENT_ENV_FILE=" + managementEnvironmentFile,
		"LLM_PROXY_LIVE_PROVIDERS=openai",
		"LIVE_OPERATION_CAPTURE=" + operationCapture,
		"EXPECTED_MANAGEMENT_SIGNING_KEY=" + managementSigningKey,
		"EXPECTED_MANAGEMENT_ISSUER=" + managementIssuer,
		"EXPECTED_MANAGEMENT_COOKIE_NAME=" + managementCookieName,
		"EXPECTED_MANAGEMENT_TENANT_ID=" + managementTenantID,
	}
	output, commandError := command.CombinedOutput()
	if commandError != nil {
		testingInstance.Fatalf("existing local-origin harness failed: %v\n%s", commandError, output)
	}
	if _, statError := os.Stat(fixture.proxyPIDPath); !os.IsNotExist(statError) {
		testingInstance.Fatalf("existing local-origin harness started a host proxy: %v", statError)
	}
	outputText := string(output)
	for _, expectedFragment := range []string{
		"live provider verification passed: provider=openai model=gpt-4.1 status=200",
		"live provider smoke passed: provider=openai model=omitted status=200",
	} {
		if !strings.Contains(outputText, expectedFragment) {
			testingInstance.Fatalf("existing local-origin output omitted %q: %s", expectedFragment, outputText)
		}
	}
	if strings.Contains(outputText, providerKey) || strings.Contains(outputText, inheritedProviderKey) || strings.Contains(outputText, "existing-local-signing-key") {
		testingInstance.Fatalf("existing local-origin output exposed private values: %s", outputText)
	}
	captureBytes, readError := os.ReadFile(operationCapture)
	if readError != nil {
		testingInstance.Fatalf("read existing local-origin operation capture: %v", readError)
	}
	if !strings.Contains(string(captureBytes), localOrigin+"/api/management/tenants/tenant-live/provider-connections/openai") ||
		!strings.Contains(string(captureBytes), localOrigin+"/v2?provider=openai") {
		testingInstance.Fatalf("existing local-origin requests did not use %s: %s", localOrigin, captureBytes)
	}
}

func TestOperationalLiveHarnessRejectsNonLoopbackExistingOrigin(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	command := exec.Command(
		filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_providers.sh"),
		"--existing-local-origin",
		"https://llm-proxy-api.mprlab.com",
	)
	command.Dir = repositoryRoot
	command.Env = []string{"PATH=" + os.Getenv("PATH")}
	output, commandError := command.CombinedOutput()
	if commandError == nil {
		testingInstance.Fatalf("live harness accepted a non-loopback existing origin: %s", output)
	}
	if !strings.Contains(string(output), "--existing-local-origin must be an HTTP 127.0.0.1 origin with an explicit port") {
		testingInstance.Fatalf("non-loopback origin failure was not specific: %s", output)
	}
}

func TestOperationalLiveHarnessDiscoversCatalogOnlyProviderFields(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixture := newOperationalLiveHarnessFixture(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	environmentFile := filepath.Join(fixtureRoot, "live.env")
	operationCapture := filepath.Join(fixtureRoot, "operations.log")
	const (
		providerKey = "test-live-catalog-token"
		providerURL = "https://catalog-test.example/v1"
		providerID  = "catalog-test"
		modelID     = "catalog-test-model"
	)
	writeOperationalFile(
		testingInstance,
		environmentFile,
		"CATALOG_TEST_TOKEN="+providerKey+"\nCATALOG_TEST_URL="+providerURL+"\n",
		0o600,
	)
	discoveryFixture := `{"schema_version":1,"providers":[{"id":"catalog-test","fields":[{"id":"access_token","kind":"credential","required":true,"environment":"CATALOG_TEST_TOKEN"},{"id":"gateway_url","kind":"setting","required":true,"environment":"CATALOG_TEST_URL"}]}]}`
	command := exec.Command(filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_providers.sh"))
	command.Dir = repositoryRoot
	command.Env = []string{
		"PATH=" + fixture.toolDirectory + string(os.PathListSeparator) + os.Getenv("PATH"),
		"GO=" + filepath.Join(fixture.toolDirectory, "go"),
		"LLM_PROXY_LIVE_PORT=" + strconv.Itoa(operationalLoopbackPort(testingInstance)),
		"PROXY_PID_CAPTURE=" + fixture.proxyPIDPath,
		"LIVE_ENV_FILE=" + environmentFile,
		"LLM_PROXY_LIVE_PROVIDERS=" + providerID,
		"LLM_PROXY_LIVE_CATALOG_TEST_MODEL=" + modelID,
		"PROVIDER_DISCOVERY_FIXTURE=" + discoveryFixture,
		"LIVE_OPERATION_CAPTURE=" + operationCapture,
	}
	output, commandError := command.CombinedOutput()
	if commandError != nil {
		testingInstance.Fatalf("catalog-only live provider harness failed: %v\n%s", commandError, output)
	}
	outputText := string(output)
	for _, expected := range []string{
		"live provider verification passed: provider=" + providerID + " model=" + modelID + " status=200",
		"live provider smoke passed: provider=" + providerID + " model=" + modelID + " status=200",
	} {
		if !strings.Contains(outputText, expected) {
			testingInstance.Fatalf("catalog-only live provider output omitted %q: %s", expected, output)
		}
	}
	if strings.Contains(outputText, providerKey) || strings.Contains(outputText, "live-generated-secret") {
		testingInstance.Fatalf("catalog-only live provider output exposed credential material: %s", output)
	}
	captureBytes, readError := os.ReadFile(operationCapture)
	if readError != nil {
		testingInstance.Fatalf("read catalog-only live provider capture: %v", readError)
	}
	capture := string(captureBytes)
	expectedPayload := `payload {"fields":{"access_token":"` + providerKey + `","gateway_url":"` + providerURL + `"},"text_model":"` + modelID + `","system_prompt":""}`
	if !strings.Contains(capture, "provider-connections/"+providerID) || !strings.Contains(capture, expectedPayload) {
		testingInstance.Fatalf("catalog-only live provider did not use discovered fields: %s", capture)
	}
	assertOperationalProxyChildStopped(testingInstance, fixture.proxyPIDPath)
}

func TestOperationalLiveHarnessDiscoversEverySelectedProviderTextModel(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixture := newOperationalLiveHarnessFixture(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	environmentFile := filepath.Join(fixtureRoot, "live.env")
	operationCapture := filepath.Join(fixtureRoot, "operations.log")
	const dashScopeProviderKey = "test-live-dashscope-key"
	const miniMaxProviderKey = "test-live-minimax-key"
	writeOperationalFile(testingInstance, environmentFile, "DASHSCOPE_API_KEY="+dashScopeProviderKey+"\nDASHSCOPE_BASE_URL=https://tenant-workspace.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1\nMINIMAX_API_KEY="+miniMaxProviderKey+"\n", 0o600)
	command := exec.Command(filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_providers.sh"))
	command.Dir = repositoryRoot
	command.Env = []string{
		"PATH=" + fixture.toolDirectory + string(os.PathListSeparator) + os.Getenv("PATH"),
		"GO=" + filepath.Join(fixture.toolDirectory, "go"),
		"LLM_PROXY_LIVE_PORT=" + strconv.Itoa(operationalLoopbackPort(testingInstance)),
		"PROXY_PID_CAPTURE=" + fixture.proxyPIDPath,
		"LIVE_ENV_FILE=" + environmentFile,
		"LLM_PROXY_LIVE_PROVIDERS=dashscope,minimax",
		"LLM_PROXY_LIVE_ALL_MODELS=true",
		"LIVE_OPERATION_CAPTURE=" + operationCapture,
	}
	output, commandError := command.CombinedOutput()
	if commandError != nil {
		testingInstance.Fatalf("live provider all-model harness failed: %v\n%s", commandError, output)
	}
	outputText := string(output)
	models := []struct {
		provider string
		model    string
	}{
		{provider: "dashscope", model: "qwen-plus"},
		{provider: "dashscope", model: "qwen3.6-flash"},
		{provider: "dashscope", model: "qwen3.7-max"},
		{provider: "dashscope", model: "qwen3.7-plus"},
		{provider: "minimax", model: "minimax-m2"},
		{provider: "minimax", model: "minimax-m2.1"},
		{provider: "minimax", model: "minimax-m2.1-highspeed"},
		{provider: "minimax", model: "minimax-m2.5"},
		{provider: "minimax", model: "minimax-m2.5-highspeed"},
		{provider: "minimax", model: "minimax-m2.7"},
		{provider: "minimax", model: "minimax-m2.7-highspeed"},
	}
	for _, route := range models {
		for _, expected := range []string{
			"live provider verification passed: provider=" + route.provider + " model=" + route.model + " status=200",
			"live provider smoke passed: provider=" + route.provider + " model=" + route.model + " status=200",
		} {
			if !strings.Contains(outputText, expected) {
				testingInstance.Fatalf("live all-model output missing %q: %s", expected, output)
			}
		}
	}
	if strings.Contains(outputText, dashScopeProviderKey) || strings.Contains(outputText, miniMaxProviderKey) || strings.Contains(outputText, "live-generated-secret") {
		testingInstance.Fatalf("live all-model output exposed credential material: %s", output)
	}
	captureBytes, readError := os.ReadFile(operationCapture)
	if readError != nil {
		testingInstance.Fatalf("read live all-model capture: %v", readError)
	}
	capture := string(captureBytes)
	if strings.Count(capture, "verify PUT ") != len(models) || strings.Count(capture, "smoke POST ") != len(models) {
		testingInstance.Fatalf("live all-model operation count mismatch: %s", capture)
	}
	for _, route := range models {
		if !strings.Contains(capture, `"text_model":"`+route.model+`"`) || !strings.Contains(capture, `"model":"`+route.model+`"`) {
			testingInstance.Fatalf("live all-model capture missing provider=%s model=%s: %s", route.provider, route.model, capture)
		}
	}
	assertOperationalProxyChildStopped(testingInstance, fixture.proxyPIDPath)
}

func TestOperationalLiveHarnessRunsCatalogSelectedImageMatrixAfterVerification(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixture := newOperationalLiveHarnessFixture(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	environmentFile := filepath.Join(fixtureRoot, "live.env")
	operationCapture := filepath.Join(fixtureRoot, "operations.log")
	providerKeys := map[string]string{
		"OPENAI_API_KEY":    "test-live-openai-key",
		"ANTHROPIC_API_KEY": "test-live-anthropic-key",
		"GEMINI_API_KEY":    "test-live-gemini-key",
		"MOONSHOT_API_KEY":  "test-live-moonshot-key",
		"XAI_API_KEY":       "test-live-xai-key",
	}
	environmentContents := ""
	for variableName, variableValue := range providerKeys {
		environmentContents += variableName + "=" + variableValue + "\n"
	}
	writeOperationalFile(testingInstance, environmentFile, environmentContents, 0o600)
	environment := []string{
		"PATH=" + fixture.toolDirectory + string(os.PathListSeparator) + os.Getenv("PATH"),
		"GO=" + filepath.Join(fixture.toolDirectory, "go"),
		"LLM_PROXY_LIVE_PORT=" + strconv.Itoa(operationalLoopbackPort(testingInstance)),
		"PROXY_PID_CAPTURE=" + fixture.proxyPIDPath,
		"LIVE_ENV_FILE=" + environmentFile,
		"LIVE_OPERATION_CAPTURE=" + operationCapture,
		"LLM_PROXY_LIVE_ALL_MODELS=true",
	}
	command := exec.Command(filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_providers.sh"), "--media")
	command.Dir = repositoryRoot
	command.Env = environment
	output, commandError := command.CombinedOutput()
	if commandError != nil {
		testingInstance.Fatalf("live provider image matrix failed: %v\n%s", commandError, output)
	}
	outputText := string(output)
	expectedModels := []string{"gpt-4.1", "kimi-k2.6", "kimi-k2.7-code", "kimi-k2.7-code-highspeed", "kimi-k3", "gemini-3.5-flash", "claude-sonnet-4-6", "grok-4.5"}
	expectedProviders := []string{"openai", "moonshot", "moonshot", "moonshot", "moonshot", "gemini", "anthropic", "xai"}
	for index, provider := range expectedProviders {
		expectedVerification := "live provider verification passed: provider=" + provider + " model=" + expectedModels[index] + " status=200"
		expectedSuccess := "live provider image smoke passed: provider=" + provider + " model=" + expectedModels[index] + " status=200"
		if !strings.Contains(outputText, expectedVerification) || !strings.Contains(outputText, expectedSuccess) {
			testingInstance.Fatalf("live provider image matrix omitted provider=%s proof: %s", provider, output)
		}
	}
	for _, forbiddenValue := range append([]string{"live-generated-secret", "iVBOR"}, mapValues(providerKeys)...) {
		if strings.Contains(outputText, forbiddenValue) {
			testingInstance.Fatalf("live provider image matrix output exposed credential or image material: %s", output)
		}
	}
	captureBytes, readError := os.ReadFile(operationCapture)
	if readError != nil {
		testingInstance.Fatalf("read live image operation capture: %v", readError)
	}
	capture := string(captureBytes)
	lastVerificationOffset := strings.LastIndex(capture, "verify PUT ")
	firstImageOffset := strings.Index(capture, "image POST ")
	if strings.Count(capture, "verify PUT ") != 8 || strings.Count(capture, "image POST ") != 8 || firstImageOffset <= lastVerificationOffset {
		testingInstance.Fatalf("live image operations were not eight verifications followed by eight images: %s", capture)
	}
	imagePayloadLines := []string{}
	for _, line := range strings.Split(capture, "\n") {
		if strings.HasPrefix(line, "image-payload ") {
			imagePayloadLines = append(imagePayloadLines, strings.TrimPrefix(line, "image-payload "))
		}
	}
	if len(imagePayloadLines) != len(expectedModels) {
		testingInstance.Fatalf("live image payload count=%d capture=%s", len(imagePayloadLines), capture)
	}
	for index, payloadLine := range imagePayloadLines {
		assertOperationalImageSmokePayload(testingInstance, payloadLine, expectedModels[index])
	}
	assertOperationalProxyChildStopped(testingInstance, fixture.proxyPIDPath)
}

func TestOperationalLiveHarnessRejectsProviderWithoutCatalogImageBeforeVerification(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixture := newOperationalLiveHarnessFixture(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	environmentFile := filepath.Join(fixtureRoot, "live.env")
	operationCapture := filepath.Join(fixtureRoot, "operations.log")
	const providerKey = "test-live-deepseek-key"
	writeOperationalFile(testingInstance, environmentFile, "DEEPSEEK_API_KEY="+providerKey+"\n", 0o600)
	command := exec.Command(filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_providers.sh"), "--media")
	command.Dir = repositoryRoot
	command.Env = []string{
		"PATH=" + fixture.toolDirectory + string(os.PathListSeparator) + os.Getenv("PATH"),
		"GO=" + filepath.Join(fixture.toolDirectory, "go"),
		"LLM_PROXY_LIVE_PORT=" + strconv.Itoa(operationalLoopbackPort(testingInstance)),
		"PROXY_PID_CAPTURE=" + fixture.proxyPIDPath,
		"LIVE_ENV_FILE=" + environmentFile,
		"LLM_PROXY_LIVE_PROVIDERS=deepseek",
		"LIVE_OPERATION_CAPTURE=" + operationCapture,
	}
	output, commandError := command.CombinedOutput()
	if commandError == nil {
		testingInstance.Fatalf("live provider image matrix accepted a provider without an image route: %s", output)
	}
	outputText := string(output)
	if !strings.Contains(outputText, "live provider image model is unavailable or ambiguous: provider=deepseek") || strings.Contains(outputText, providerKey) {
		testingInstance.Fatalf("live provider image rejection was not safe: %s", output)
	}
	if captureBytes, readError := os.ReadFile(operationCapture); readError == nil && len(captureBytes) != 0 {
		testingInstance.Fatalf("live provider image rejection performed paid work: %s", captureBytes)
	} else if readError != nil && !os.IsNotExist(readError) {
		testingInstance.Fatalf("inspect rejected image operations: %v", readError)
	}
	assertOperationalProxyChildStopped(testingInstance, fixture.proxyPIDPath)
}

func assertOperationalImageSmokePayload(testingInstance *testing.T, payloadText string, expectedModel string) {
	testingInstance.Helper()
	var payload struct {
		Messages []struct {
			Role        string `json:"role"`
			Content     string `json:"content"`
			Attachments []struct {
				Type     string `json:"type"`
				MIMEType string `json:"mime_type"`
				Data     string `json:"data"`
			} `json:"attachments"`
		} `json:"messages"`
		Model     string `json:"model"`
		WebSearch bool   `json:"web_search"`
	}
	decoder := json.NewDecoder(strings.NewReader(payloadText))
	decoder.DisallowUnknownFields()
	if decodeError := decoder.Decode(&payload); decodeError != nil {
		testingInstance.Fatalf("decode live image payload: %v", decodeError)
	}
	if payload.Model != expectedModel || payload.WebSearch || len(payload.Messages) != 1 || payload.Messages[0].Role != "user" || len(payload.Messages[0].Attachments) != 1 {
		testingInstance.Fatalf("live image payload shape=%+v", payload)
	}
	attachment := payload.Messages[0].Attachments[0]
	imageBytes, decodeError := base64.StdEncoding.DecodeString(attachment.Data)
	if decodeError != nil {
		testingInstance.Fatalf("decode live image data: %v", decodeError)
	}
	if attachment.Type != "image" || attachment.MIMEType != "image/png" || !bytes.HasPrefix(imageBytes, []byte("\x89PNG\r\n\x1a\n")) {
		testingInstance.Fatalf("live image attachment=%+v bytes=%d", attachment, len(imageBytes))
	}
}

func mapValues(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
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
  'if [[ " $* " == *" --provider-catalog-only "* ]]; then' \
  '  if [[ -n "${PROVIDER_DISCOVERY_FIXTURE:-}" ]]; then' \
  '    builtin printf "%s\n" "${PROVIDER_DISCOVERY_FIXTURE}"' \
  '  else' \
  '    builtin printf "%s\n" '\''{"schema_version":1,"providers":[{"id":"openai","fields":[{"id":"api_key","kind":"credential","required":true,"environment":"OPENAI_API_KEY"}]},{"id":"deepseek","fields":[{"id":"api_key","kind":"credential","required":true,"environment":"DEEPSEEK_API_KEY"}]},{"id":"dashscope","fields":[{"id":"api_key","kind":"credential","required":true,"environment":"DASHSCOPE_API_KEY"},{"id":"base_url","kind":"setting","required":true,"environment":"DASHSCOPE_BASE_URL"}]},{"id":"moonshot","fields":[{"id":"api_key","kind":"credential","required":true,"environment":"MOONSHOT_API_KEY"}]},{"id":"minimax","fields":[{"id":"api_key","kind":"credential","required":true,"environment":"MINIMAX_API_KEY"}]},{"id":"siliconflow","fields":[{"id":"api_key","kind":"credential","required":true,"environment":"SILICONFLOW_API_KEY"}]},{"id":"zai","fields":[{"id":"api_key","kind":"credential","required":true,"environment":"ZAI_API_KEY"}]},{"id":"gemini","fields":[{"id":"api_key","kind":"credential","required":true,"environment":"GEMINI_API_KEY"}]},{"id":"anthropic","fields":[{"id":"api_key","kind":"credential","required":true,"environment":"ANTHROPIC_API_KEY"}]},{"id":"meta","fields":[{"id":"api_key","kind":"credential","required":true,"environment":"MODEL_API_KEY"}]},{"id":"xai","fields":[{"id":"api_key","kind":"credential","required":true,"environment":"XAI_API_KEY"}]}]}'\''' \
  '  fi' \
  '  exit 0' \
  'fi' \
  'builtin printf "%s\n" "$$" >"${PROXY_PID_CAPTURE:?}"' \
  'exec sleep 60' >"${output_path}"
chmod +x "${output_path}"
`, 0o755)
	writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "curl"), `#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f "${PROXY_PID_CAPTURE:?}" && "${ALLOW_EXISTING_LOCAL_ORIGIN:-false}" != "true" ]]; then
  exit 1
fi
if [[ -n "${EXPECTED_MANAGEMENT_SIGNING_KEY:-}" ]]; then
  [[ "${LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY:-}" == "${EXPECTED_MANAGEMENT_SIGNING_KEY}" ]]
  [[ "${LLM_PROXY_MANAGEMENT_JWT_ISSUER:-}" == "${EXPECTED_MANAGEMENT_ISSUER:?}" ]]
  [[ "${LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME:-}" == "${EXPECTED_MANAGEMENT_COOKIE_NAME:?}" ]]
  [[ "${LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID:-}" == "${EXPECTED_MANAGEMENT_TENANT_ID:?}" ]]
fi

output_path=""
response_headers_path=""
request_body_path=""
request_body=""
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
    -D)
      response_headers_path="$2"
      shift 2
      ;;
    --data-binary)
      request_body_path="${2#@}"
      shift 2
      ;;
    --data)
      request_body="$2"
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

write_response_headers() {
  if [[ -n "${response_headers_path}" ]]; then
    builtin printf '%s\r\n' 'HTTP/1.1 200 OK' 'X-LLM-Proxy-Request-ID: ABCDEFGHIJKLMNOPQRSTUVWXYZ234567' '' >"${response_headers_path}"
  fi
}

case "${request_url}" in
  */api/public/capabilities)
    builtin printf '%s' '{"offerings":[{"provider":"openai","model":"gpt-4.1","capabilities":["image_input","text"]},{"provider":"anthropic","model":"claude-sonnet-4-6","capabilities":["image_input","text"]},{"provider":"gemini","model":"gemini-3.5-flash","capabilities":["audio_input","image_input","text"]},{"provider":"moonshot","model":"kimi-k2.6","capabilities":["image_input","text"]},{"provider":"moonshot","model":"kimi-k2.7-code","capabilities":["image_input","text"]},{"provider":"moonshot","model":"kimi-k2.7-code-highspeed","capabilities":["image_input","text"]},{"provider":"moonshot","model":"kimi-k3","capabilities":["image_input","text"]},{"provider":"xai","model":"grok-4.5","capabilities":["image_input","text"]},{"provider":"dashscope","model":"qwen-plus","capabilities":["text"]},{"provider":"dashscope","model":"qwen3.6-flash","capabilities":["text"]},{"provider":"dashscope","model":"qwen3.7-max","capabilities":["text"]},{"provider":"dashscope","model":"qwen3.7-plus","capabilities":["text"]},{"provider":"minimax","model":"minimax-m2","capabilities":["text"]},{"provider":"minimax","model":"minimax-m2.1","capabilities":["text"]},{"provider":"minimax","model":"minimax-m2.1-highspeed","capabilities":["text"]},{"provider":"minimax","model":"minimax-m2.5","capabilities":["text"]},{"provider":"minimax","model":"minimax-m2.5-highspeed","capabilities":["text"]},{"provider":"minimax","model":"minimax-m2.7","capabilities":["text"]},{"provider":"minimax","model":"minimax-m2.7-highspeed","capabilities":["text"]}]}' >"${output_path}"
    builtin printf '%s' 200
    ;;
  */api/management/account)
    builtin printf '%s' '{"tenants":[{"id":"tenant-live"}]}' >"${output_path}"
    builtin printf '%s' 200
    ;;
  */api/management/tenants/tenant-live/secrets)
    builtin printf '%s' '{"secret":"live-generated-secret","profile":{"providers":[{"id":"openai","text_default_model":"gpt-4.1"},{"id":"anthropic","text_default_model":"claude-sonnet-4-6"},{"id":"gemini","text_default_model":"gemini-3.5-flash"},{"id":"moonshot","text_default_model":"kimi-k2.6"},{"id":"minimax","text_default_model":"minimax-m2.7"},{"id":"xai","text_default_model":"grok-4.3"},{"id":"deepseek","text_default_model":"deepseek-v4-flash"}]}}' >"${output_path}"
    builtin printf '%s' 200
    ;;
  */api/management/tenants/tenant-live/provider-connections/*)
    if [[ -n "${PREFLIGHT_PROVIDER_URL:-}" ]]; then
      if [[ -n "${CURL_PREFLIGHT_BLOCK_PATH:-}" ]]; then
        builtin printf '%s\n' ready >"${CURL_PREFLIGHT_BLOCK_PATH}"
        sleep "${CURL_PREFLIGHT_BLOCK_SECONDS:-1}"
      fi
      python3 -c '
import json
import os
import pathlib
import sys
import urllib.request

candidate = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
payload = json.dumps({
    "model": candidate["text_model"],
    "input": "Verify this provider credential.",
    "max_output_tokens": 16,
}).encode("utf-8")
request = urllib.request.Request(
    os.environ["PREFLIGHT_PROVIDER_URL"] + "/responses",
    data=payload,
    headers={
        "Authorization": "Bearer " + os.environ["PREFLIGHT_PROVIDER_API_KEY"],
        "Content-Type": "application/json",
    },
    method="POST",
)
with urllib.request.urlopen(request, timeout=5) as response:
    response.read()
' "${request_body_path}"
    fi
    if [[ -n "${LIVE_OPERATION_CAPTURE:-}" ]]; then
      builtin printf 'verify %s %s\n' "${request_method}" "${request_url}" >>"${LIVE_OPERATION_CAPTURE}"
      builtin printf 'payload ' >>"${LIVE_OPERATION_CAPTURE}"
      command cat "${request_body_path}" >>"${LIVE_OPERATION_CAPTURE}"
      builtin printf '\n' >>"${LIVE_OPERATION_CAPTURE}"
    fi
    builtin printf '%s' '{}' >"${output_path}"
    builtin printf '%s' 200
    ;;
  */v2?provider=*)
    if [[ -n "${request_body_path}" ]]; then
      if [[ -n "${LIVE_OPERATION_CAPTURE:-}" ]]; then
        builtin printf 'image %s %s\n' "${request_method}" "${request_url}" >>"${LIVE_OPERATION_CAPTURE}"
        builtin printf 'image-payload ' >>"${LIVE_OPERATION_CAPTURE}"
        command cat "${request_body_path}" >>"${LIVE_OPERATION_CAPTURE}"
        builtin printf '\n' >>"${LIVE_OPERATION_CAPTURE}"
      fi
      write_response_headers
      builtin printf '%s' RED >"${output_path}"
    else
      if [[ -n "${PREFLIGHT_PROVIDER_URL:-}" ]]; then
        python3 -c '
import json
import os
import urllib.request

payload = json.dumps({
    "model": "gpt-4.1",
    "input": "Reply with exactly OK and no punctuation.",
}).encode("utf-8")
request = urllib.request.Request(
    os.environ["PREFLIGHT_PROVIDER_URL"] + "/responses",
    data=payload,
    headers={
        "Authorization": "Bearer " + os.environ["PREFLIGHT_PROVIDER_API_KEY"],
        "Content-Type": "application/json",
    },
    method="POST",
)
with urllib.request.urlopen(request, timeout=5) as response:
    response.read()
'
      fi
      if [[ -n "${LIVE_OPERATION_CAPTURE:-}" ]]; then
        builtin printf 'smoke %s %s\n' "${request_method}" "${request_url}" >>"${LIVE_OPERATION_CAPTURE}"
        builtin printf 'smoke-payload %s\n' "${request_body}" >>"${LIVE_OPERATION_CAPTURE}"
      fi
      builtin printf '%s' OK >"${output_path}"
    fi
    builtin printf '%s' 200
    ;;
  *provider=*)
    if [[ -n "${PREFLIGHT_PROVIDER_URL:-}" ]]; then
      python3 -c '
import json
import os
import urllib.request

payload = json.dumps({
    "model": "gpt-4.1",
    "input": "Reply with exactly OK and no punctuation.",
}).encode("utf-8")
request = urllib.request.Request(
    os.environ["PREFLIGHT_PROVIDER_URL"] + "/responses",
    data=payload,
    headers={
        "Authorization": "Bearer " + os.environ["PREFLIGHT_PROVIDER_API_KEY"],
        "Content-Type": "application/json",
    },
    method="POST",
)
with urllib.request.urlopen(request, timeout=5) as response:
    response.read()
'
    fi
    if [[ -n "${LIVE_OPERATION_CAPTURE:-}" ]]; then
      builtin printf 'smoke %s %s\n' "${request_method}" "${request_url}" >>"${LIVE_OPERATION_CAPTURE}"
      builtin printf 'smoke-payload %s\n' "${request_body}" >>"${LIVE_OPERATION_CAPTURE}"
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
		fileBytes, readError := os.ReadFile(path)
		if readError == nil && len(fileBytes) > 0 {
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
	writeOperationalFile(testingInstance, filepath.Join(fixtureRoot, "configs", ".env.local"), `LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN=http://localhost:4179
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
