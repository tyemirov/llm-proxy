package tests_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func assertHostedCIWorkflow(testingInstance *testing.T, repositoryRoot string, document []byte) {
	testingInstance.Helper()
	type workflowStep struct {
		Run             string
		If              string
		ContinueOnError bool `yaml:"continue-on-error"`
		Env             map[string]string
	}
	type workflowJob struct {
		Needs           []string
		If              string
		TimeoutMinutes  int  `yaml:"timeout-minutes"`
		ContinueOnError bool `yaml:"continue-on-error"`
		Steps           []workflowStep
	}
	var workflow struct {
		Jobs map[string]workflowJob
	}
	if err := yaml.Unmarshal(document, &workflow); err != nil {
		testingInstance.Fatalf("decode hosted CI workflow: %v", err)
	}
	if len(workflow.Jobs) != 4 {
		testingInstance.Fatal("hosted CI must have independent backend, backend-checks, and frontend jobs plus the required test check")
	}
	for _, name := range []string{"backend", "backend-checks", "frontend"} {
		job, exists := workflow.Jobs[name]
		expectedDeadline := 10
		if name == "backend" {
			expectedDeadline = 15
		}
		if !exists || len(job.Needs) != 0 || job.If != "" || job.ContinueOnError || job.TimeoutMinutes != expectedDeadline {
			testingInstance.Fatalf("hosted %s job must run independently with its %d-minute deadline", name, expectedDeadline)
		}
		qualificationSteps := 0
		for _, step := range job.Steps {
			if step.If != "" || step.ContinueOnError {
				testingInstance.Fatalf("hosted %s qualification must not skip or ignore a failed step", name)
			}
			if step.Run == "make ci-"+name+" PLAYWRIGHT_INSTALL_FLAGS=" {
				qualificationSteps++
			}
			if strings.Contains(step.Run, "timeout ") || strings.Contains(step.Run, "go install ") {
				testingInstance.Fatalf("hosted %s job duplicates Make-owned tooling or adds a shell deadline", name)
			}
		}
		if qualificationSteps != 1 {
			testingInstance.Fatalf("hosted %s job must run its public Make target exactly once", name)
		}
	}

	makefile, err := os.ReadFile(filepath.Join(repositoryRoot, "Makefile"))
	if err != nil {
		testingInstance.Fatal(err)
	}
	runner, err := os.ReadFile(filepath.Join(repositoryRoot, "scripts", "run_ci.sh"))
	if err != nil {
		testingInstance.Fatal(err)
	}
	stageBlock := regexp.MustCompile(`(?s)STAGE_TARGETS=\(\n(.*?)\n\)`).FindSubmatch(runner)
	if stageBlock == nil {
		testingInstance.Fatal("canonical local CI stage list is absent")
	}
	canonicalTargets := make(map[string]bool)
	for _, line := range strings.Split(string(stageBlock[1]), "\n") {
		canonicalTargets[strings.Trim(line, " \t\"")] = true
	}
	hostedTargets := make(map[string]bool)
	for _, name := range []string{"backend", "backend-checks", "frontend"} {
		match := regexp.MustCompile(`(?m)^ci-` + name + `: ([^\n]+)$`).FindSubmatch(makefile)
		if match == nil {
			testingInstance.Fatalf("public ci-%s target is absent", name)
		}
		if name == "backend" && string(match[1]) != "go-test" {
			testingInstance.Fatal("hosted coverage must start independently from all other backend gates")
		}
		for _, target := range strings.Fields(string(match[1])) {
			if !canonicalTargets[target] || hostedTargets[target] {
				testingInstance.Fatalf("hosted CI has unknown or repeated gate %s", target)
			}
			hostedTargets[target] = true
		}
	}
	if len(hostedTargets) != len(canonicalTargets) {
		testingInstance.Fatal("hosted jobs omit canonical local CI gates")
	}

	aggregate, exists := workflow.Jobs["test"]
	if !exists || strings.Join(aggregate.Needs, ",") != "backend,backend-checks,frontend" || aggregate.If != "${{ always() }}" || aggregate.ContinueOnError || aggregate.TimeoutMinutes != 1 || len(aggregate.Steps) != 1 {
		testingInstance.Fatal("required test check must evaluate all completed job results even after failure or cancellation")
	}
	step := aggregate.Steps[0]
	if step.If != "" || step.ContinueOnError || step.Env["BACKEND_RESULT"] != "${{ needs.backend.result }}" || step.Env["BACKEND_CHECKS_RESULT"] != "${{ needs.backend-checks.result }}" || step.Env["FRONTEND_RESULT"] != "${{ needs.frontend.result }}" {
		testingInstance.Fatal("required test check does not consume the exact qualification results")
	}
	for _, backend := range []string{"success", "failure", "cancelled", "skipped", ""} {
		for _, checks := range []string{"success", "failure", "cancelled", "skipped", ""} {
			for _, frontend := range []string{"success", "failure", "cancelled", "skipped", ""} {
				command := exec.Command("bash", "-e", "-c", step.Run)
				command.Env = append(os.Environ(), "BACKEND_RESULT="+backend, "BACKEND_CHECKS_RESULT="+checks, "FRONTEND_RESULT="+frontend)
				output, commandError := command.CombinedOutput()
				wantSuccess := backend == "success" && checks == "success" && frontend == "success"
				if (commandError == nil) != wantSuccess {
					testingInstance.Fatalf("required test result is wrong for backend=%q checks=%q frontend=%q: %v\n%s", backend, checks, frontend, commandError, output)
				}
			}
		}
	}
}
