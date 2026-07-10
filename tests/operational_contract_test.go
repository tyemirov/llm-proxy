package tests_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	operationalScriptsDirectory     = "scripts"
	operationalReleaseToolsRelative = "tools/gitrelease"
)

func TestOperationalReleaseWrapperUsesRepositoryOwnedTools(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	fixtureRoot := testingInstance.TempDir()
	copyOperationalFile(testingInstance, filepath.Join(repositoryRoot, operationalScriptsDirectory, "release.sh"), filepath.Join(fixtureRoot, operationalScriptsDirectory, "release.sh"))
	copyOperationalDirectory(testingInstance, filepath.Join(repositoryRoot, operationalReleaseToolsRelative), filepath.Join(fixtureRoot, operationalReleaseToolsRelative))
	runOperationalCommand(testingInstance, fixtureRoot, nil, "git", "init")

	command := exec.Command(filepath.Join(fixtureRoot, operationalScriptsDirectory, "release.sh"), "--help")
	command.Dir = fixtureRoot
	output, commandError := command.CombinedOutput()
	if commandError != nil {
		testingInstance.Fatalf("repository-owned release wrapper failed: %v\n%s", commandError, output)
	}
	if !strings.Contains(string(output), "Prepares a release entirely from local repository state") {
		testingInstance.Fatalf("unexpected release help output: %s", output)
	}
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
		"#!/usr/bin/env bash\nset -euo pipefail\nprintf '%s\\t%s\\n' \"$*\" \"${DEPLOY_PAGES_ARGS:-}\" >>\"${MAKE_CAPTURE}\"\nif [[ \"$*\" == *pages-deploy* && \"${DEPLOY_PAGES_ARGS:-}\" == *--verify-only* ]]; then exit 42; fi\nif [[ \"${1:-}\" == \"-C\" ]]; then : >\"${GATEWAY_SENTINEL}\"; fi\n",
		0o755,
	)
	gatewayDirectory := filepath.Join(testingInstance.TempDir(), "gateway")
	if directoryError := os.MkdirAll(gatewayDirectory, 0o755); directoryError != nil {
		testingInstance.Fatalf("create gateway fixture: %v", directoryError)
	}
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
	if directoryError := os.MkdirAll(gatewayDirectory, 0o755); directoryError != nil {
		testingInstance.Fatalf("create gateway fixture: %v", directoryError)
	}
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
	writeOperationalFile(testingInstance, environmentFile, "MODEL_API_KEY=test-meta-key\nLLM_PROXY_MANAGEMENT_ENABLED=true\nLLM_PROXY_MANAGEMENT_UI_DESCRIPTION=LLM Proxy\n", 0o600)
	environment := append(
		os.Environ(),
		"LIVE_ENV_FILE="+environmentFile,
		"LLM_PROXY_LIVE_PROVIDERS=meta",
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

func operationalRepositoryRoot(testingInstance *testing.T) string {
	testingInstance.Helper()
	repositoryRoot, absoluteError := filepath.Abs("..")
	if absoluteError != nil {
		testingInstance.Fatalf("resolve repository root: %v", absoluteError)
	}
	return repositoryRoot
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
