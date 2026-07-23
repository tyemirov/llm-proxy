package tests_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
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
	operationalScriptsDirectory     = "scripts"
	operationalReleaseToolsRelative = "tools/gitrelease"
	operationalHelpTimeout          = 5 * time.Second
	operationalHelpWaitDelay        = time.Second
	constrainedPipeHelpCommand      = `ulimit -p 1 2>/dev/null || true
exec "$@"`
)

func TestOperationalReleaseWrapperUsesRepositoryOwnedTools(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	copyOperationalFile(testingInstance, filepath.Join(repositoryRoot, operationalScriptsDirectory, "release.sh"), filepath.Join(fixtureRoot, operationalScriptsDirectory, "release.sh"))
	copyOperationalDirectory(testingInstance, filepath.Join(repositoryRoot, operationalReleaseToolsRelative), filepath.Join(fixtureRoot, operationalReleaseToolsRelative))
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "init")

	output := runOperationalHelpCommand(
		testingInstance,
		fixtureRoot,
		filepath.Join(fixtureRoot, operationalScriptsDirectory, "release.sh"),
		"--help",
		nil,
	)
	if !strings.Contains(string(output), "Prepares a release entirely from local repository state") {
		testingInstance.Fatalf("unexpected release help output: %s", output)
	}
}

func TestOperationalHelpCommandsUseBuiltinOutput(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	restrictedPath := testingInstance.TempDir()
	helpCommands := []struct {
		name             string
		path             string
		expectedFragment string
	}{
		{name: "deploy", path: filepath.Join(repositoryRoot, operationalScriptsDirectory, "deploy.sh"), expectedFragment: "scripts/deploy.sh [options]"},
		{name: "live-providers", path: filepath.Join(repositoryRoot, operationalScriptsDirectory, "test_live_providers.sh"), expectedFragment: "scripts/test_live_providers.sh [--preflight | --write-config <path>]"},
		{name: "deploy-pages", path: filepath.Join(repositoryRoot, operationalReleaseToolsRelative, "scripts", "deploy_pages_artifact.sh"), expectedFragment: "deploy_pages_artifact.sh --url <public-url> [options]"},
		{name: "prepare-container", path: filepath.Join(repositoryRoot, operationalReleaseToolsRelative, "scripts", "prepare_container_artifact.sh"), expectedFragment: "prepare_container_artifact.sh --name <name> --image <registry/repository> [options]"},
		{name: "prepare-pages", path: filepath.Join(repositoryRoot, operationalReleaseToolsRelative, "scripts", "prepare_pages_artifact.sh"), expectedFragment: "prepare_pages_artifact.sh --source <directory> [options]"},
		{name: "prepare-release", path: filepath.Join(repositoryRoot, operationalReleaseToolsRelative, "scripts", "prepare_release.sh"), expectedFragment: "prepare_release.sh [options]"},
		{name: "publish-container", path: filepath.Join(repositoryRoot, operationalReleaseToolsRelative, "scripts", "publish_container_artifacts.sh"), expectedFragment: "publish_container_artifacts.sh"},
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

func TestOperationalMakeUpStartsLocalWebOrchestration(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	for _, relativePath := range []string{
		"Makefile",
		".dockerignore",
		"docker-compose.local.yml",
		filepath.Join(operationalScriptsDirectory, "up.sh"),
		filepath.Join("configs", "config.yml"),
		filepath.Join("configs", ".env.sample"),
		filepath.Join("configs", ".env.local.example"),
		filepath.Join("configs", "tauth.local.yml"),
	} {
		copyOperationalFile(testingInstance, filepath.Join(repositoryRoot, relativePath), filepath.Join(fixtureRoot, relativePath))
	}

	toolDirectory := filepath.Join(fixtureRoot, "tools")
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
  *"http://127.0.0.1:4179/config-ui.yaml"*)
    status=200
    ;;
  *"http://127.0.0.1:4179/"*)
    status=200
    ;;
  *"http://127.0.0.1:8080/?prompt=ready"*)
    status=403
    ;;
  *"http://127.0.0.1:8082/auth/session"*)
    status=204
    ;;
  *"http://127.0.0.1:8080/api/management/profile"*)
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

	command := exec.Command("make", "up")
	command.Dir = fixtureRoot
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Env = append(
		os.Environ(),
		"PATH="+toolDirectory+string(os.PathListSeparator)+os.Getenv("PATH"),
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
	waitForOperationalFile(testingInstance, curlReadyPath)
	waitForOperationalOutput(testingInstance, &output, "LLM Proxy local orchestration is ready.")
	waitForOperationalFile(testingInstance, composePIDPath)
	if signalError := syscall.Kill(-command.Process.Pid, syscall.SIGINT); signalError != nil {
		testingInstance.Fatalf("interrupt make up: %v", signalError)
	}
	waitForOperationalCommand(testingInstance, command)
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

	curlArguments, readCurlArgumentsError := os.ReadFile(curlArgumentsPath)
	if readCurlArgumentsError != nil {
		testingInstance.Fatalf("read curl arguments: %v", readCurlArgumentsError)
	}
	for _, expectedURL := range []string{
		"http://127.0.0.1:4179/",
		"http://127.0.0.1:4179/config-ui.yaml",
		"http://127.0.0.1:8080/?prompt=ready",
		"http://127.0.0.1:8082/auth/session",
		"http://127.0.0.1:8080/api/management/profile",
	} {
		if !strings.Contains(string(curlArguments), expectedURL) {
			testingInstance.Fatalf("make up did not verify %s: %s", expectedURL, curlArguments)
		}
	}
	if !strings.Contains(string(curlArguments), "Origin: http://localhost:4179") {
		testingInstance.Fatalf("make up did not verify browser-origin authentication boundaries: %s", curlArguments)
	}

	localEnvironment, readLocalEnvironmentError := os.ReadFile(filepath.Join(fixtureRoot, "configs", ".env.local"))
	if readLocalEnvironmentError != nil {
		testingInstance.Fatalf("read generated local environment: %v", readLocalEnvironmentError)
	}
	for _, expectedFragment := range []string{
		"LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN=http://localhost:4179",
		"LLM_PROXY_MANAGEMENT_TAUTH_URL=http://localhost:8082",
		"LLM_PROXY_MANAGEMENT_API_ORIGIN=http://localhost:8080",
		"GHTTP_SERVE_DIRECTORY=/app/site",
		"GHTTP_SERVE_PROXIES=/config-ui.yaml=http://api:8080",
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
		"GHTTP_SERVE_PROXIES",
	})
	assertOperationalEnvironmentKeys(testingInstance, filepath.Join(fixtureRoot, "configs", ".env.api.local"), []string{
		"LLM_PROXY_MANAGEMENT_ENABLED",
		"LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN",
		"LLM_PROXY_MANAGEMENT_LOOPBACK_ORIGIN",
		"LLM_PROXY_MANAGEMENT_LOCALHOST_ORIGIN",
		"LLM_PROXY_MANAGEMENT_UI_DESCRIPTION",
		"LLM_PROXY_MANAGEMENT_ADMIN_EMAILS",
		"LLM_PROXY_MANAGEMENT_TAUTH_URL",
		"LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID",
		"LLM_PROXY_MANAGEMENT_GOOGLE_CLIENT_ID",
		"LLM_PROXY_MANAGEMENT_TAUTH_LOGIN_PATH",
		"LLM_PROXY_MANAGEMENT_TAUTH_LOGOUT_PATH",
		"LLM_PROXY_MANAGEMENT_TAUTH_NONCE_PATH",
		"LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY",
		"LLM_PROXY_MANAGEMENT_JWT_ISSUER",
		"LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME",
		"LLM_PROXY_MANAGEMENT_DATABASE_DIALECT",
		"LLM_PROXY_MANAGEMENT_DATABASE_DSN",
		"LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY",
		"LLM_PROXY_MANAGEMENT_API_ORIGIN",
		"LLM_PROXY_MANAGEMENT_PROXY_ORIGIN",
		"LLM_PROXY_MANAGEMENT_LEGACY_TOKEN_OWNER_EMAIL",
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
				"127.0.0.1:4179:4179",
				"127.0.0.1:8080:8080",
				"127.0.0.1:8082:8080",
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
		filepath.Join("configs", ".env.sample"),
		filepath.Join("configs", ".env.local.example"),
	} {
		copyOperationalFile(testingInstance, filepath.Join(repositoryRoot, relativePath), filepath.Join(fixtureRoot, relativePath))
	}

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

func waitForOperationalOutput(testingInstance *testing.T, output *synchronizedOperationalOutput, expectedFragment string) {
	testingInstance.Helper()
	deadline := time.Now().Add(operationalHelpTimeout)
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
		key, _, hasValue := strings.Cut(line, "=")
		if !hasValue || key == "" {
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
	for _, relativeRoot := range []string{operationalScriptsDirectory, filepath.Join(operationalReleaseToolsRelative, "scripts")} {
		walkError := filepath.Walk(filepath.Join(repositoryRoot, relativeRoot), func(path string, fileInfo os.FileInfo, pathError error) error {
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
			testingInstance.Fatalf("scan shell scripts under %s: %v", relativeRoot, walkError)
		}
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

func TestOperationalContainerArtifactUsesTrackedSnapshot(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	writeOperationalFile(testingInstance, filepath.Join(fixtureRoot, "Dockerfile"), "FROM scratch\nCOPY . /app\n", 0o644)
	writeOperationalFile(testingInstance, filepath.Join(fixtureRoot, "tracked.txt"), "tracked\n", 0o644)
	writeOperationalFile(testingInstance, filepath.Join(fixtureRoot, ".gitignore"), "configs/.env\n", 0o644)
	writeOperationalFile(testingInstance, filepath.Join(fixtureRoot, "configs", ".env"), "MODEL_API_KEY=must-not-enter-build-context\n", 0o600)
	copyOperationalFile(testingInstance, filepath.Join(repositoryRoot, operationalScriptsDirectory, "build-container-artifact.sh"), filepath.Join(fixtureRoot, operationalScriptsDirectory, "build-container-artifact.sh"))
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "init")
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "config", "user.name", "Operational Test")
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "config", "user.email", "operational-test@example.invalid")
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "add", "Dockerfile", "tracked.txt", ".gitignore", operationalScriptsDirectory)
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "commit", "-m", "Fixture")

	toolDirectory := filepath.Join(fixtureRoot, "fake-release-tools")
	fakeTool := `#!/usr/bin/env bash
set -euo pipefail
context=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "--context" ]]; then context="$2"; shift 2; else shift; fi
done
[[ -n "${context}" ]]
[[ -f "${context}/tracked.txt" ]]
[[ ! -e "${context}/configs/.env" ]]
[[ ! -e "${context}/.git" ]]
`
	writeOperationalFile(testingInstance, filepath.Join(toolDirectory, "prepare_container_artifact.sh"), fakeTool, 0o755)
	environment := append(os.Environ(), "RELEASE_TOOL_DIR="+toolDirectory)
	runOperationalCommand(testingInstance, fixtureRoot, environment, filepath.Join(fixtureRoot, operationalScriptsDirectory, "build-container-artifact.sh"))
}

func TestOperationalContainerArtifactWritesCanonicalDescriptor(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	artifactDirectory := filepath.Join(fixtureRoot, "release-artifact")
	writeOperationalFile(testingInstance, filepath.Join(fixtureRoot, "Dockerfile"), "FROM scratch\n", 0o644)
	writeOperationalFile(testingInstance, filepath.Join(artifactDirectory, "staging.json"), "{}\n", 0o644)
	fakeBinaryDirectory := filepath.Join(fixtureRoot, "bin")
	fakeDocker := `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "buildx" && "$2" == "version" ]]; then exit 0; fi
if [[ "$1" == "buildx" && "$2" == "build" ]]; then exit 0; fi
if [[ "$1" == "image" && "$2" == "inspect" ]]; then builtin printf '%s\n' 'sha256:fixture-image'; exit 0; fi
if [[ "$1" == "save" ]]; then
  output=""
  shift
  while [[ $# -gt 0 ]]; do
    if [[ "$1" == "--output" ]]; then output="$2"; shift 2; else shift; fi
  done
  [[ -n "${output}" ]]
  builtin printf '%s\n' 'fixture-archive' >"${output}"
  exit 0
fi
echo "unsupported fake Docker command: $*" >&2
exit 1
`
	writeOperationalFile(testingInstance, filepath.Join(fakeBinaryDirectory, "docker"), fakeDocker, 0o755)
	environment := append(
		os.Environ(),
		"PATH="+fakeBinaryDirectory+string(os.PathListSeparator)+os.Getenv("PATH"),
		"RELEASE_VERSION=v1.2.3",
		"RELEASE_ARTIFACT_DIR="+artifactDirectory,
		"RELEASE_CONTAINER_BUILD_TIMEOUT_SECONDS=5",
		"RELEASE_CONTAINER_SAVE_TIMEOUT_SECONDS=5",
	)
	runOperationalCommand(
		testingInstance,
		fixtureRoot,
		environment,
		filepath.Join(repositoryRoot, operationalReleaseToolsRelative, "scripts", "prepare_container_artifact.sh"),
		"--name", "fixture",
		"--image", "example/fixture",
		"--file", "Dockerfile",
		"--context", ".",
		"--platforms", "linux/amd64",
	)
	descriptorPath := filepath.Join(artifactDirectory, "payloads", "containers", "fixture", "container.json")
	descriptorBytes, readError := os.ReadFile(descriptorPath)
	if readError != nil {
		testingInstance.Fatalf("read container descriptor: %v", readError)
	}
	var descriptor struct {
		SchemaVersion int    `json:"schema_version"`
		ArtifactKind  string `json:"artifact_kind"`
		Name          string `json:"name"`
		Image         string `json:"image"`
		Version       string `json:"version"`
		Platforms     []struct {
			Platform string `json:"platform"`
			Token    string `json:"token"`
			LocalRef string `json:"local_ref"`
			ImageID  string `json:"image_id"`
			Archive  string `json:"archive"`
			SHA256   string `json:"sha256"`
		} `json:"platforms"`
	}
	if unmarshalError := json.Unmarshal(descriptorBytes, &descriptor); unmarshalError != nil {
		testingInstance.Fatalf("parse container descriptor: %v", unmarshalError)
	}
	if descriptor.SchemaVersion != 1 || descriptor.ArtifactKind != "mprlab.container" {
		testingInstance.Fatalf("unexpected container descriptor contract: %s", descriptorBytes)
	}
	if descriptor.Name != "fixture" || descriptor.Image != "example/fixture" || descriptor.Version != "v1.2.3" {
		testingInstance.Fatalf("unexpected container descriptor identity: %s", descriptorBytes)
	}
	if len(descriptor.Platforms) != 1 {
		testingInstance.Fatalf("unexpected container descriptor platforms: %s", descriptorBytes)
	}
	platform := descriptor.Platforms[0]
	if platform.Platform != "linux/amd64" || platform.Token != "linux-amd64" || platform.LocalRef != "mprlab-release.local/fixture:v1.2.3-linux-amd64" || platform.ImageID != "sha256:fixture-image" || platform.Archive != "payloads/containers/fixture/linux-amd64.tar" || platform.SHA256 == "" {
		testingInstance.Fatalf("unexpected container platform descriptor: %s", descriptorBytes)
	}
}

func TestOperationalDeployNoopMatchesGatewayVerifier(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	environment := append(os.Environ(), "GATEWAY_DIR="+filepath.Join(testingInstance.TempDir(), "missing-gateway"))
	output := runOperationalCommand(
		testingInstance,
		repositoryRoot,
		environment,
		filepath.Join(repositoryRoot, operationalScriptsDirectory, "deploy.sh"),
		"--tag", "v0.0.0",
		"--skip-ci",
		"--skip-image-verify",
		"--skip-gateway",
	)
	if !strings.Contains(output, "llm-proxy deploy complete") {
		testingInstance.Fatalf("unexpected deploy no-op output: %s", output)
	}
}

func TestOperationalDeployRejectsNoncanonicalTag(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	command := exec.Command(
		filepath.Join(repositoryRoot, operationalScriptsDirectory, "deploy.sh"),
		"--tag", "v1.0.0-01",
		"--skip-ci",
		"--skip-image-verify",
		"--skip-gateway",
	)
	command.Dir = repositoryRoot
	output, commandError := command.CombinedOutput()
	if commandError == nil {
		testingInstance.Fatalf("deploy accepted a noncanonical tag: %s", output)
	}
	if !strings.Contains(string(output), "canonical vMAJOR.MINOR.PATCH SemVer") {
		testingInstance.Fatalf("unexpected invalid-tag error: %s", output)
	}
}

func TestOperationalDeployPreflightsPagesBeforeGatewayMutation(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	copyOperationalFile(testingInstance, filepath.Join(repositoryRoot, operationalScriptsDirectory, "deploy.sh"), filepath.Join(fixtureRoot, operationalScriptsDirectory, "deploy.sh"))
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "init")
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "config", "user.name", "Operational Test")
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "config", "user.email", "operational-test@example.invalid")
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "add", operationalScriptsDirectory)
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "commit", "-m", "Fixture")
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "branch", "-M", "master")
	remoteRoot := filepath.Join(testingInstance.TempDir(), "origin.git")
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "init", "--bare", remoteRoot)
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "remote", "add", "origin", remoteRoot)
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "push", "-u", "origin", "master")
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "tag", "v1.0.0")
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "push", "origin", "v1.0.0")

	toolDirectory := filepath.Join(testingInstance.TempDir(), "bin")
	gatewaySentinel := filepath.Join(testingInstance.TempDir(), "gateway-mutated")
	makeCapture := filepath.Join(testingInstance.TempDir(), "make-capture.log")
	writeOperationalFile(
		testingInstance,
		filepath.Join(toolDirectory, "make"),
		"#!/usr/bin/env bash\nset -euo pipefail\nprintf '%s\\t%s\\n' \"$*\" \"${DEPLOY_PAGES_ARGS:-}\" >>\"${MAKE_CAPTURE}\"\nif [[ \"$*\" == *pages-deploy* && \"${DEPLOY_PAGES_ARGS:-}\" == *--verify-only* ]]; then exit 42; fi\nif [[ \"${1:-}\" == \"-C\" && \"$*\" == *deploy-llm-proxy-backend* ]]; then : >\"${GATEWAY_SENTINEL}\"; fi\n",
		0o755,
	)
	gatewayDirectory := filepath.Join(testingInstance.TempDir(), "gateway")
	initializeOperationalGatewayCheckout(testingInstance, gatewayDirectory, "origin")
	environment := append(
		os.Environ(),
		"PATH="+toolDirectory+string(os.PathListSeparator)+os.Getenv("PATH"),
		"MAKE_CAPTURE="+makeCapture,
		"GATEWAY_SENTINEL="+gatewaySentinel,
		"GATEWAY_DIR="+gatewayDirectory,
		"DEPLOY_REMOTE=origin",
		"DEPLOY_BRANCH=master",
		"RELEASE_HELPER="+filepath.Join(repositoryRoot, operationalReleaseToolsRelative, "scripts", "release_helper.py"),
	)
	command := exec.Command(
		filepath.Join(fixtureRoot, operationalScriptsDirectory, "deploy.sh"),
		"--tag", "v1.0.0",
		"--skip-ci",
		"--skip-image-verify",
		"--pages-url", "https://pages.example.invalid/",
	)
	command.Dir = fixtureRoot
	command.Env = environment
	output, commandError := command.CombinedOutput()
	if commandError == nil {
		testingInstance.Fatalf("deploy continued after Pages preflight failure: %s", output)
	}
	if _, statError := os.Stat(gatewaySentinel); !os.IsNotExist(statError) {
		testingInstance.Fatalf("gateway mutated before Pages preflight succeeded: %v", statError)
	}
	captureBytes, readError := os.ReadFile(makeCapture)
	if readError != nil {
		testingInstance.Fatalf("read make preflight capture: %v", readError)
	}
	if !strings.Contains(string(captureBytes), "pages-deploy\t--verify-only") {
		testingInstance.Fatalf("Pages verify-only preflight was not invoked: %s", captureBytes)
	}
}

func TestOperationalDeployForwardsSelectedRemoteToPages(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	copyOperationalFile(testingInstance, filepath.Join(repositoryRoot, operationalScriptsDirectory, "deploy.sh"), filepath.Join(fixtureRoot, operationalScriptsDirectory, "deploy.sh"))
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "init")
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "config", "user.name", "Operational Test")
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "config", "user.email", "operational-test@example.invalid")
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "add", operationalScriptsDirectory)
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "commit", "-m", "Fixture")
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "branch", "-M", "master")
	remoteRoot := filepath.Join(testingInstance.TempDir(), "upstream.git")
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "init", "--bare", remoteRoot)
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "remote", "add", "upstream", remoteRoot)
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "push", "-u", "upstream", "master")
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "tag", "v1.0.0")
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "push", "upstream", "v1.0.0")

	toolDirectory := filepath.Join(testingInstance.TempDir(), "bin")
	makeCapture := filepath.Join(testingInstance.TempDir(), "make-capture.log")
	writeOperationalFile(
		testingInstance,
		filepath.Join(toolDirectory, "make"),
		"#!/usr/bin/env bash\nset -euo pipefail\nprintf '%s\\t%s\\n' \"$*\" \"${PUBLISH_REMOTE:-}\" >>\"${MAKE_CAPTURE}\"\n",
		0o755,
	)
	gatewayDirectory := filepath.Join(testingInstance.TempDir(), "gateway")
	initializeOperationalGatewayCheckout(testingInstance, gatewayDirectory, "origin")
	environment := append(
		os.Environ(),
		"PATH="+toolDirectory+string(os.PathListSeparator)+os.Getenv("PATH"),
		"MAKE_CAPTURE="+makeCapture,
		"GATEWAY_DIR="+gatewayDirectory,
		"DEPLOY_REMOTE=upstream",
		"DEPLOY_BRANCH=master",
		"RELEASE_HELPER="+filepath.Join(repositoryRoot, operationalReleaseToolsRelative, "scripts", "release_helper.py"),
	)
	runOperationalCommand(
		testingInstance,
		fixtureRoot,
		environment,
		filepath.Join(fixtureRoot, operationalScriptsDirectory, "deploy.sh"),
		"--tag", "v1.0.0",
		"--skip-ci",
		"--skip-image-verify",
		"--pages-url", "https://pages.example.invalid/",
	)
	captureBytes, readError := os.ReadFile(makeCapture)
	if readError != nil {
		testingInstance.Fatalf("read make invocation capture: %v", readError)
	}
	if !strings.Contains(string(captureBytes), "--no-print-directory pages-deploy\tupstream") {
		testingInstance.Fatalf("Pages deployment did not receive selected remote: %s", captureBytes)
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
	waitForOperationalFile(testingInstance, preflightBlockPath)
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

for argument in "$@"; do
  case "${argument}" in
    *provider=unsupported-live-preflight*)
      if [[ -n "${CURL_PREFLIGHT_BLOCK_PATH:-}" ]]; then
        builtin printf '%s\n' ready >"${CURL_PREFLIGHT_BLOCK_PATH}"
        sleep "${CURL_PREFLIGHT_BLOCK_SECONDS:-1}"
      fi
      builtin printf '%s' 400
      exit 0
      ;;
  esac
done
builtin printf '%s' 403
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

func waitForOperationalFile(testingInstance *testing.T, path string) {
	testingInstance.Helper()
	deadline := time.Now().Add(operationalHelpTimeout)
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

func waitForOperationalCommand(testingInstance *testing.T, command *exec.Cmd) {
	testingInstance.Helper()
	completed := make(chan error, 1)
	go func() {
		completed <- command.Wait()
	}()
	select {
	case <-completed:
	case <-time.After(operationalHelpTimeout):
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

func initializeOperationalGatewayCheckout(testingInstance *testing.T, gatewayDirectory string, remoteName string) {
	testingInstance.Helper()
	writeOperationalFile(testingInstance, filepath.Join(gatewayDirectory, "deployment-contract.txt"), "coupled llm-proxy and TAuth\n", 0o644)
	runOperationalCommand(testingInstance, gatewayDirectory, nil, "git", "init")
	runOperationalCommand(testingInstance, gatewayDirectory, nil, "git", "config", "user.name", "Operational Test")
	runOperationalCommand(testingInstance, gatewayDirectory, nil, "git", "config", "user.email", "operational-test@example.invalid")
	runOperationalCommand(testingInstance, gatewayDirectory, nil, "git", "add", "deployment-contract.txt")
	runOperationalCommand(testingInstance, gatewayDirectory, nil, "git", "commit", "-m", "Gateway fixture")
	runOperationalCommand(testingInstance, gatewayDirectory, nil, "git", "branch", "-M", "master")
	remoteDirectory := filepath.Join(testingInstance.TempDir(), remoteName+"-gateway.git")
	runOperationalCommand(testingInstance, gatewayDirectory, nil, "git", "init", "--bare", remoteDirectory)
	runOperationalCommand(testingInstance, gatewayDirectory, nil, "git", "remote", "add", remoteName, remoteDirectory)
	runOperationalCommand(testingInstance, gatewayDirectory, nil, "git", "push", "-u", remoteName, "master")
}

func copyOperationalDirectory(testingInstance *testing.T, sourceDirectory string, targetDirectory string) {
	testingInstance.Helper()
	entries, readError := os.ReadDir(sourceDirectory)
	if readError != nil {
		testingInstance.Fatalf("read operational directory %s: %v", sourceDirectory, readError)
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(sourceDirectory, entry.Name())
		targetPath := filepath.Join(targetDirectory, entry.Name())
		if entry.IsDir() {
			copyOperationalDirectory(testingInstance, sourcePath, targetPath)
			continue
		}
		copyOperationalFile(testingInstance, sourcePath, targetPath)
	}
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
