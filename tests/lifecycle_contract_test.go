package tests_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const lifecycleManifestRelativePath = ".mprlab/deploy/resources.yml"

var expectedSiblingGatewayWrapper = strings.Join([]string{
	".PHONY: release publish deploy",
	"",
	"release publish deploy:",
	"\t@application_root=\"$$(git rev-parse --show-toplevel)\"; \\",
	"\tgateway_root=\"$$(dirname \"$${application_root}\")/mprlab-gateway\"; \\",
	"\tif [ ! -d \"$${gateway_root}\" ]; then \\",
	"\t\tprintf \"required sibling gateway is missing: %s; clone mprlab-gateway at exactly %s\\n\" \\",
	"\t\t\t\"$${gateway_root}\" \"$${gateway_root}\" >&2; \\",
	"\t\texit 2; \\",
	"\tfi; \\",
	"\t$(MAKE) --no-print-directory -C \"$${gateway_root}\" \"app-$@\" \\",
	"\t\tMPRLAB_APP_ROOT=\"$${application_root}\"",
}, "\n")

func TestOperationalRepositoryOwnsSchemaV2Lifecycle(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	manifestPath := filepath.Join(repositoryRoot, filepath.FromSlash(lifecycleManifestRelativePath))
	manifestBytes, readError := os.ReadFile(manifestPath)
	if readError != nil {
		testingInstance.Fatalf("read lifecycle manifest: %v", readError)
	}

	var document map[string]any
	if unmarshalError := yaml.Unmarshal(manifestBytes, &document); unmarshalError != nil {
		testingInstance.Fatalf("decode lifecycle manifest: %v", unmarshalError)
	}
	if len(document) != 1 {
		testingInstance.Fatalf("lifecycle manifest must have one root: %#v", document)
	}
	resourcesDocument, available := document["mprlab_resources"].(map[string]any)
	if !available {
		testingInstance.Fatalf("lifecycle manifest has no mprlab_resources mapping: %#v", document)
	}
	if schemaVersion, schemaAvailable := resourcesDocument["schema_version"].(int); !schemaAvailable || schemaVersion != 2 {
		testingInstance.Fatalf("unexpected lifecycle schema version: %#v", resourcesDocument["schema_version"])
	}
	if owner, ownerAvailable := resourcesDocument["owner"].(string); !ownerAvailable || owner != "llm-proxy" {
		testingInstance.Fatalf("unexpected lifecycle owner: %#v", resourcesDocument["owner"])
	}
	dependencies, dependenciesAvailable := resourcesDocument["dependencies"].([]any)
	if !dependenciesAvailable || len(dependencies) != 0 {
		testingInstance.Fatalf("llm-proxy must declare no runtime dependencies: %#v", resourcesDocument["dependencies"])
	}

	resources, resourcesAvailable := resourcesDocument["resources"].([]any)
	if !resourcesAvailable {
		testingInstance.Fatalf("lifecycle manifest has no resources list: %#v", resourcesDocument["resources"])
	}
	resourceIdentities := make([]string, 0, len(resources))
	for _, resourceValue := range resources {
		resource, resourceAvailable := resourceValue.(map[string]any)
		if !resourceAvailable {
			testingInstance.Fatalf("lifecycle resource is not a mapping: %#v", resourceValue)
		}
		resourceIdentities = append(
			resourceIdentities,
			lifecycleStringField(testingInstance, resource, "kind")+"/"+lifecycleStringField(testingInstance, resource, "id"),
		)
	}
	slices.Sort(resourceIdentities)
	expectedResourceIdentities := []string{
		"caddy_route/public-api",
		"compose_project/runtime",
		"github_pages/website",
		"health_check/api-auth-boundary",
		"health_check/management-config",
		"runtime_capability/http",
		"tauth_tenant/authentication",
	}
	if !slices.Equal(resourceIdentities, expectedResourceIdentities) {
		testingInstance.Fatalf("unexpected llm-proxy lifecycle resources: %#v", resourceIdentities)
	}

	manifestText := string(manifestBytes)
	for _, requiredContract := range []string{
		"secret: llm-proxy.runtime-environment",
		"source: configs/config.yml",
		"target: /app/config.yml",
		"name: mprlab-nginx-gateway_llm-proxy-data",
		"name: llm-proxy.http",
		"alias: llm-proxy",
		"hostname: llm-proxy-api.mprlab.com",
		"url: https://llm-proxy-api.mprlab.com/",
		"url: https://llm-proxy-api.mprlab.com/config-ui.yaml",
		"dockerfile: docker/pages/Dockerfile",
		"capability: tauth.tenants",
	} {
		if !strings.Contains(manifestText, requiredContract) {
			testingInstance.Errorf("lifecycle manifest is missing %q", requiredContract)
		}
	}
	for _, forbiddenContract := range []string{
		"schema_version: 1",
		"make_workflow",
		"ansible_task_bundle",
		"container_name:",
		"compose_template:",
		"npm_package",
		"mobile_application",
		"target: pages-deploy",
	} {
		if strings.Contains(manifestText, forbiddenContract) {
			testingInstance.Errorf("lifecycle manifest retains forbidden contract %q", forbiddenContract)
		}
	}
}

func TestOperationalProductionLifecycleDelegatesOnlyToSiblingGateway(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	makefileBytes, readError := os.ReadFile(filepath.Join(repositoryRoot, "Makefile"))
	if readError != nil {
		testingInstance.Fatalf("read Makefile: %v", readError)
	}
	makefileText := string(makefileBytes)
	if !strings.Contains(makefileText, expectedSiblingGatewayWrapper) {
		testingInstance.Fatal("Makefile does not expose the exact sibling-gateway lifecycle wrapper")
	}
	for _, obsoleteTarget := range []string{
		"\ncontainer-artifacts:",
		"\npages-artifact:",
		"\npublish-release:",
		"\npages-deploy:",
		"\ndeploy-dry-run:",
		"\ndeploy-syntax:",
	} {
		if strings.Contains(makefileText, obsoleteTarget) {
			testingInstance.Errorf("Makefile retains obsolete production target %q", obsoleteTarget)
		}
	}

	for _, forbiddenPath := range []string{
		"scripts/release.sh",
		"scripts/publish-release.sh",
		"scripts/build-container-artifact.sh",
		"scripts/build-pages-artifact.sh",
		"tools/gitrelease",
	} {
		_, statError := os.Stat(filepath.Join(repositoryRoot, filepath.FromSlash(forbiddenPath)))
		if !errors.Is(statError, os.ErrNotExist) {
			testingInstance.Errorf("application repository still owns production lifecycle path %s", forbiddenPath)
		}
	}

	trackedDeployCommand := exec.Command("git", "ls-files", ".mprlab/deploy")
	trackedDeployCommand.Dir = repositoryRoot
	trackedDeployOutput, trackedDeployError := trackedDeployCommand.Output()
	if trackedDeployError != nil {
		testingInstance.Fatalf("list tracked deployment files: %v", trackedDeployError)
	}
	trackedDeployFiles := make([]string, 0, 1)
	for _, trackedDeployFile := range strings.Fields(string(trackedDeployOutput)) {
		_, statError := os.Stat(filepath.Join(repositoryRoot, filepath.FromSlash(trackedDeployFile)))
		if statError == nil {
			trackedDeployFiles = append(trackedDeployFiles, trackedDeployFile)
			continue
		}
		if !errors.Is(statError, os.ErrNotExist) {
			testingInstance.Fatalf("inspect tracked deployment file %s: %v", trackedDeployFile, statError)
		}
	}
	if !slices.Equal(trackedDeployFiles, []string{lifecycleManifestRelativePath}) {
		testingInstance.Fatalf("unexpected tracked deployment files: %#v", trackedDeployFiles)
	}
}

func TestOperationalPagesArtifactUsesGoWithoutNode(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	dockerfileBytes, readError := os.ReadFile(filepath.Join(repositoryRoot, "docker", "pages", "Dockerfile"))
	if readError != nil {
		testingInstance.Fatalf("read Pages Dockerfile: %v", readError)
	}
	dockerfileText := string(dockerfileBytes)
	for _, requiredContract := range []string{
		"FROM golang:1.26.5-bookworm AS renderer",
		"--render-site-output /pages",
		"rm /pages/CNAME",
		"FROM scratch AS pages",
		"COPY --from=renderer /pages/ /",
	} {
		if !strings.Contains(dockerfileText, requiredContract) {
			testingInstance.Errorf("Pages Dockerfile is missing %q", requiredContract)
		}
	}
	for _, forbiddenRuntime := range []string{"FROM node", "npm ", "npx ", "yarn ", "pnpm "} {
		if strings.Contains(dockerfileText, forbiddenRuntime) {
			testingInstance.Errorf("Pages Dockerfile introduces Node tooling %q", forbiddenRuntime)
		}
	}
}

func lifecycleStringField(testingInstance *testing.T, document map[string]any, fieldName string) string {
	testingInstance.Helper()
	value, available := document[fieldName].(string)
	if !available || value == "" {
		testingInstance.Fatalf("lifecycle field %s is not a non-empty string: %#v", fieldName, document[fieldName])
	}
	return value
}
