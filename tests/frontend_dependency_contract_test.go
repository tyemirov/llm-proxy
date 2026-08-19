package tests_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const frontendDependencyContractTimeout = 10 * time.Second

func TestOperationalFrontendValidationPreparesPinnedDependencies(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	makePath, lookupError := exec.LookPath("make")
	if lookupError != nil {
		testingInstance.Fatalf("resolve Make executable: %v", lookupError)
	}
	for _, environmentVariable := range []string{
		"GNUMAKEFLAGS",
		"MAKEFLAGS",
		"MFLAGS",
		"PLAYWRIGHT_INSTALL_FLAGS",
	} {
		testingInstance.Setenv(environmentVariable, "PLAYWRIGHT_INSTALL_FLAGS=")
	}

	scenarios := []struct {
		name             string
		target           string
		makeArguments    []string
		expectedCommands []string
	}{
		{
			name:   "frontend-lint",
			target: "frontend-lint",
			expectedCommands: []string{
				"ci",
				"playwright install --with-deps chromium",
				"run frontend:lint",
			},
		},
		{
			name:   "frontend-test",
			target: "frontend-test",
			expectedCommands: []string{
				"ci",
				"playwright install --with-deps chromium",
				"run frontend:test",
			},
		},
		{
			name:   "management-black-box",
			target: "test-management-auth-blackbox",
			expectedCommands: []string{
				"ci",
				"playwright install --with-deps chromium",
				"run frontend:test:blackbox",
			},
		},
		{
			name:   "test",
			target: "test",
			expectedCommands: []string{
				"ci",
				"playwright install --with-deps chromium",
				"run frontend:test",
				"run frontend:test:blackbox",
			},
		},
		{
			name:   "ci",
			target: "ci",
			expectedCommands: []string{
				"ci",
				"playwright install --with-deps chromium",
				"run frontend:lint",
				"run frontend:test",
				"run frontend:test:blackbox",
			},
		},
		{
			name:          "hosted-ci",
			target:        "ci",
			makeArguments: []string{"PLAYWRIGHT_INSTALL_FLAGS="},
			expectedCommands: []string{
				"ci",
				"playwright install chromium",
				"run frontend:lint",
				"run frontend:test",
				"run frontend:test:blackbox",
			},
		},
	}

	for _, scenario := range scenarios {
		testingInstance.Run(scenario.name, func(testingInstance *testing.T) {
			fixtureRoot := testingInstance.TempDir()
			prepareFrontendDependencyFixture(testingInstance, repositoryRoot, fixtureRoot)
			npmLogPath := filepath.Join(fixtureRoot, "npm.log")
			canonicalFixtureRoot, evaluateError := filepath.EvalSymlinks(fixtureRoot)
			if evaluateError != nil {
				testingInstance.Fatalf("resolve fixture path: %v", evaluateError)
			}
			browserPath := filepath.Join(canonicalFixtureRoot, "node_modules", ".cache", "ms-playwright")

			commandContext, cancelCommand := context.WithTimeout(context.Background(), frontendDependencyContractTimeout)
			defer cancelCommand()
			makeArguments := []string{
				"--no-print-directory",
				scenario.target,
				"NPM=" + filepath.Join(fixtureRoot, "npm"),
				"GO=" + filepath.Join(fixtureRoot, "go"),
				"GOFMT=true",
				"UV=true",
			}
			makeArguments = append(makeArguments, scenario.makeArguments...)
			command := exec.CommandContext(commandContext, makePath, makeArguments...)
			command.Dir = fixtureRoot
			command.Env = append(
				frontendDependencyFixtureEnvironment(),
				"FRONTEND_NPM_LOG="+npmLogPath,
				"FRONTEND_PLAYWRIGHT_FIXTURE="+filepath.Join(fixtureRoot, "playwright"),
			)
			output, commandError := command.CombinedOutput()
			if commandContext.Err() != nil {
				testingInstance.Fatalf("make %s timed out: %v\n%s", scenario.target, commandContext.Err(), output)
			}
			if commandError != nil {
				testingInstance.Fatalf("make %s failed: %v\n%s", scenario.target, commandError, output)
			}

			npmLogBytes, readError := os.ReadFile(npmLogPath)
			if readError != nil {
				testingInstance.Fatalf("read npm command log: %v", readError)
			}
			expectedLines := make([]string, 0, len(scenario.expectedCommands))
			for _, expectedCommand := range scenario.expectedCommands {
				expectedLines = append(expectedLines, expectedCommand+"|"+browserPath)
			}
			expectedLog := strings.Join(expectedLines, "\n") + "\n"
			if string(npmLogBytes) != expectedLog {
				testingInstance.Fatalf("unexpected npm command order for make %s:\n%s", scenario.target, npmLogBytes)
			}
		})
	}

	workflowBytes, readError := os.ReadFile(filepath.Join(repositoryRoot, ".github", "workflows", "test.yml"))
	if readError != nil {
		testingInstance.Fatalf("read hosted CI workflow: %v", readError)
	}
	workflow := string(workflowBytes)
	for _, duplicateSetup := range []string{"run: npm ci", "run: npx playwright install"} {
		if strings.Contains(workflow, duplicateSetup) {
			testingInstance.Fatalf("hosted CI duplicates Make-owned frontend setup %q", duplicateSetup)
		}
	}
	if !strings.Contains(workflow, "run: timeout -k 350s -s SIGKILL 350s make ci PLAYWRIGHT_INSTALL_FLAGS=") {
		testingInstance.Fatal("hosted CI does not declare its preinstalled Playwright OS packages")
	}
}

func frontendDependencyFixtureEnvironment() []string {
	environment := make([]string, 0, len(os.Environ()))
	for _, assignment := range os.Environ() {
		name, _, found := strings.Cut(assignment, "=")
		if !found {
			continue
		}
		switch name {
		case "GNUMAKEFLAGS", "MAKEFLAGS", "MFLAGS", "PLAYWRIGHT_INSTALL_FLAGS":
			continue
		}
		environment = append(environment, assignment)
	}
	return environment
}

func prepareFrontendDependencyFixture(testingInstance *testing.T, repositoryRoot string, fixtureRoot string) {
	testingInstance.Helper()
	makefileBytes, readError := os.ReadFile(filepath.Join(repositoryRoot, "Makefile"))
	if readError != nil {
		testingInstance.Fatalf("read Makefile: %v", readError)
	}
	writeOperationalFile(
		testingInstance,
		filepath.Join(fixtureRoot, "Makefile"),
		string(makefileBytes)+`\ncheck-format go-lint python-lint python-test test-openapi-pages-artifact test-live-provider-harness:
	@:

go-test:
	@if [ -n "$${COVERAGE_FILE:-}" ]; then \
		printf '%s\n' 'mode: count' 'fixture.go:1.1,1.2 1 1' >"$$COVERAGE_FILE"; \
	fi
`,
		0o644,
	)
	for _, relativePath := range []string{
		"package.json",
		"package-lock.json",
		filepath.Join("scripts", "run_ci.sh"),
	} {
		copyOperationalFile(
			testingInstance,
			filepath.Join(repositoryRoot, relativePath),
			filepath.Join(fixtureRoot, relativePath),
		)
	}
	writeOperationalFile(testingInstance, filepath.Join(fixtureRoot, "npm"), `#!/usr/bin/env bash
set -euo pipefail

builtin printf '%s|%s\n' "$*" "${PLAYWRIGHT_BROWSERS_PATH:?}" >>"${FRONTEND_NPM_LOG:?}"
case "${1:?}" in
  ci)
    [[ "$#" -eq 1 ]]
    mkdir -p node_modules/.bin
    cp "${FRONTEND_PLAYWRIGHT_FIXTURE:?}" node_modules/.bin/playwright
    chmod 755 node_modules/.bin/playwright
    ;;
  run)
    [[ -f "${PLAYWRIGHT_BROWSERS_PATH}/.llm-proxy-frontend-dependencies" ]]
    ;;
  *)
    exit 31
    ;;
esac
`, 0o755)
	writeOperationalFile(testingInstance, filepath.Join(fixtureRoot, "playwright"), `#!/usr/bin/env bash
set -euo pipefail

builtin printf 'playwright %s|%s\n' "$*" "${PLAYWRIGHT_BROWSERS_PATH:?}" >>"${FRONTEND_NPM_LOG:?}"
[[ "$*" == "install --with-deps chromium" || "$*" == "install chromium" ]]
mkdir -p "${PLAYWRIGHT_BROWSERS_PATH}"
`, 0o755)
	writeOperationalFile(testingInstance, filepath.Join(fixtureRoot, "go"), `#!/usr/bin/env bash
set -euo pipefail

[[ "$#" -eq 3 ]]
[[ "$1" == "tool" ]]
[[ "$2" == "cover" ]]
[[ -s "${3#-func=}" ]]
builtin printf 'total:\t(statements)\t100.0%%\n'
`, 0o755)
}
