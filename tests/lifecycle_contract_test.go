package tests_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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

func TestOperationalRepositoryOwnsSchemaV4Lifecycle(testingInstance *testing.T) {
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
	if schemaVersion, schemaAvailable := resourcesDocument["schema_version"].(int); !schemaAvailable || schemaVersion != 4 {
		testingInstance.Fatalf("unexpected lifecycle schema version: %#v", resourcesDocument["schema_version"])
	}
	if owner, ownerAvailable := resourcesDocument["owner"].(string); !ownerAvailable || owner != "llm-proxy" {
		testingInstance.Fatalf("unexpected lifecycle owner: %#v", resourcesDocument["owner"])
	}
	release, releaseAvailable := resourcesDocument["release"].(map[string]any)
	if !releaseAvailable || len(release) != 1 || release["scheme"] != "semver" {
		testingInstance.Fatalf("unexpected lifecycle release policy: %#v", resourcesDocument["release"])
	}
	if _, dependenciesAvailable := resourcesDocument["dependencies"]; dependenciesAvailable {
		testingInstance.Fatalf("lifecycle manifest must not declare top-level dependencies: %#v", resourcesDocument["dependencies"])
	}
	if len(resourcesDocument) != 4 {
		testingInstance.Fatalf("lifecycle manifest must contain only schema_version, owner, release, and resources: %#v", resourcesDocument)
	}

	resources, resourcesAvailable := resourcesDocument["resources"].([]any)
	if !resourcesAvailable {
		testingInstance.Fatalf("lifecycle manifest has no resources list: %#v", resourcesDocument["resources"])
	}
	resourceIdentities := make([]string, 0, len(resources))
	resourcesByIdentity := make(map[string]map[string]any, len(resources))
	composeProjectFound := false
	for _, resourceValue := range resources {
		resource, resourceAvailable := resourceValue.(map[string]any)
		if !resourceAvailable {
			testingInstance.Fatalf("lifecycle resource is not a mapping: %#v", resourceValue)
		}
		resourceKind := lifecycleStringField(testingInstance, resource, "kind")
		resourceID := lifecycleStringField(testingInstance, resource, "id")
		resourceIdentity := resourceKind + "/" + resourceID
		resourceIdentities = append(resourceIdentities, resourceIdentity)
		resourcesByIdentity[resourceIdentity] = resource
		if resourceKind != "compose_project" || resourceID != "runtime" {
			continue
		}
		for _, imageValue := range resource["images"].([]any) {
			image := imageValue.(map[string]any)
			if _, visibilityAvailable := image["visibility"]; visibilityAvailable {
				testingInstance.Fatalf("GHCR visibility must remain provider-owned: %#v", image)
			}
		}
		composeProjectFound = true
		retiredServices, retiredServicesAvailable := resource["retired_services"].([]any)
		if !retiredServicesAvailable || len(retiredServices) != 1 {
			testingInstance.Fatalf("unexpected retired runtime services: %#v", resource["retired_services"])
		}
		retiredService, retiredServiceAvailable := retiredServices[0].(map[string]any)
		if !retiredServiceAvailable {
			testingInstance.Fatalf("retired runtime service is not a mapping: %#v", retiredServices[0])
		}
		if project := lifecycleStringField(testingInstance, retiredService, "project"); project != "mprlab-nginx-gateway" {
			testingInstance.Fatalf("unexpected retired runtime project: %q", project)
		}
		if service := lifecycleStringField(testingInstance, retiredService, "service"); service != "llm-proxy" {
			testingInstance.Fatalf("unexpected retired runtime service: %q", service)
		}
	}
	if !composeProjectFound {
		testingInstance.Fatal("lifecycle manifest has no runtime compose project")
	}
	slices.Sort(resourceIdentities)
	expectedResourceIdentities := []string{
		"caddy_route/public-api",
		"compose_project/runtime",
		"github_pages/website",
		"health_check/api-auth-boundary",
		"health_check/management-config",
		"private_values/private",
		"runtime_capability/http",
		"tauth_tenant/authentication",
	}
	if !slices.Equal(resourceIdentities, expectedResourceIdentities) {
		testingInstance.Fatalf("unexpected llm-proxy lifecycle resources: %#v", resourceIdentities)
	}

	privateBindings := resourcesByIdentity["private_values/private"]["bindings"]
	expectedPrivateBindings := map[string]any{
		"admin-emails":                "LLM_PROXY_MANAGEMENT_ADMIN_EMAILS",
		"google-web-client-id":        "LLM_PROXY_MANAGEMENT_GOOGLE_CLIENT_ID",
		"jwt-signing-key":             "LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY",
		"provider-key-encryption-key": "LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY",
	}
	if !reflect.DeepEqual(privateBindings, expectedPrivateBindings) {
		testingInstance.Fatalf("unexpected private-value bindings: %#v", privateBindings)
	}

	authenticationTenant := resourcesByIdentity["tauth_tenant/authentication"]["tenant"].(map[string]any)
	expectedOAuthPolicy := map[string]any{
		"access_token_ttl":                "5m",
		"refresh_token_ttl":               "720h",
		"consent_ttl":                     "720h",
		"allow_client_metadata_documents": true,
		"resources": []any{map[string]any{
			"identifier":   "https://llm-proxy-api.mprlab.com",
			"display_name": "LLM Proxy API",
			"scopes": []any{map[string]any{
				"identifier":   "llm-proxy:use",
				"display_name": "Use LLM Proxy",
				"description":  "Use the LLM Proxy API.",
			}},
		}},
		"clients": []any{},
	}
	if !reflect.DeepEqual(authenticationTenant["oauth"], expectedOAuthPolicy) {
		testingInstance.Fatalf("unexpected TAuth tenant OAuth policy: %#v", authenticationTenant["oauth"])
	}

	runtimeServices, servicesAvailable := resourcesByIdentity["compose_project/runtime"]["services"].([]any)
	if !servicesAvailable || len(runtimeServices) != 1 {
		testingInstance.Fatalf("unexpected runtime services: %#v", resourcesByIdentity["compose_project/runtime"]["services"])
	}
	runtimeService, runtimeServiceAvailable := runtimeServices[0].(map[string]any)
	if !runtimeServiceAvailable {
		testingInstance.Fatalf("runtime service is not a mapping: %#v", runtimeServices[0])
	}
	if _, environmentFilesAvailable := runtimeService["environment_files"]; environmentFilesAvailable {
		testingInstance.Fatalf("runtime service retains environment_files: %#v", runtimeService["environment_files"])
	}
	runtimeEnvironment, environmentAvailable := runtimeService["environment"].(map[string]any)
	if !environmentAvailable {
		testingInstance.Fatalf("runtime service has no typed environment: %#v", runtimeService["environment"])
	}
	runtimeEnvironmentNames := make([]string, 0, len(runtimeEnvironment))
	for environmentName := range runtimeEnvironment {
		runtimeEnvironmentNames = append(runtimeEnvironmentNames, environmentName)
	}
	slices.Sort(runtimeEnvironmentNames)
	expectedRuntimeEnvironmentNames := []string{
		"LLM_PROXY_MANAGEMENT_ADMIN_EMAILS",
		"LLM_PROXY_MANAGEMENT_API_ORIGIN",
		"LLM_PROXY_MANAGEMENT_DATABASE_PATH",
		"LLM_PROXY_MANAGEMENT_GOOGLE_CLIENT_ID",
		"LLM_PROXY_MANAGEMENT_JWT_ISSUER",
		"LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY",
		"LLM_PROXY_MANAGEMENT_LOCALHOST_ORIGIN",
		"LLM_PROXY_MANAGEMENT_LOOPBACK_ORIGIN",
		"LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY",
		"LLM_PROXY_MANAGEMENT_PROXY_ORIGIN",
		"LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN",
		"LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME",
		"LLM_PROXY_MANAGEMENT_TAUTH_LOGIN_PATH",
		"LLM_PROXY_MANAGEMENT_TAUTH_LOGOUT_PATH",
		"LLM_PROXY_MANAGEMENT_TAUTH_NONCE_PATH",
		"LLM_PROXY_MANAGEMENT_TAUTH_SESSION_PATH",
		"LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID",
		"LLM_PROXY_MANAGEMENT_TAUTH_URL",
		"LLM_PROXY_MANAGEMENT_UI_DESCRIPTION",
	}
	if !slices.Equal(runtimeEnvironmentNames, expectedRuntimeEnvironmentNames) {
		testingInstance.Fatalf("unexpected runtime environment keys: %#v", runtimeEnvironmentNames)
	}
	for environmentName, expectedBinding := range map[string]map[string]any{
		"LLM_PROXY_MANAGEMENT_ADMIN_EMAILS":                {"resource": "private", "output": "admin-emails"},
		"LLM_PROXY_MANAGEMENT_API_ORIGIN":                  {"resource": "public-api", "output": "origin"},
		"LLM_PROXY_MANAGEMENT_GOOGLE_CLIENT_ID":            {"resource": "authentication", "output": "google-web-client-id"},
		"LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY":             {"resource": "authentication", "output": "jwt-signing-key"},
		"LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY": {"resource": "private", "output": "provider-key-encryption-key"},
		"LLM_PROXY_MANAGEMENT_PROXY_ORIGIN":                {"resource": "public-api", "output": "origin"},
		"LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN":               {"resource": "website", "output": "origin"},
		"LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME":         {"resource": "authentication", "output": "session-cookie-name"},
		"LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID":             {"resource": "authentication", "output": "tenant-id"},
		"LLM_PROXY_MANAGEMENT_TAUTH_URL":                   {"value": "https://tauth-api.mprlab.com"},
	} {
		if !reflect.DeepEqual(runtimeEnvironment[environmentName], expectedBinding) {
			testingInstance.Errorf("unexpected %s binding: %#v", environmentName, runtimeEnvironment[environmentName])
		}
	}

	manifestText := string(manifestBytes)
	for _, requiredContract := range []string{
		"kind: private_values",
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
		"environment_files:",
		"profiles:",
		"secret:",
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
		"site/.nojekyll",
		"site/CNAME",
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

func TestOperationalPagesArtifactUsesFrontendRendererWithBackendRESTData(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	dockerfileBytes, readError := os.ReadFile(filepath.Join(repositoryRoot, "docker", "pages", "Dockerfile"))
	if readError != nil {
		testingInstance.Fatalf("read Pages Dockerfile: %v", readError)
	}
	dockerfileText := string(dockerfileBytes)
	for _, requiredContract := range []string{
		"FROM golang:1.26.5-bookworm AS backend-builder",
		"FROM node:22-bookworm-slim AS renderer",
		"--public-capabilities-only",
		"node scripts/render_public_site.mjs",
		"--capabilities-url",
		"FROM scratch AS pages",
		"COPY --from=renderer /pages/ /",
	} {
		if !strings.Contains(dockerfileText, requiredContract) {
			testingInstance.Errorf("Pages Dockerfile is missing %q", requiredContract)
		}
	}
	for _, forbiddenRenderer := range []string{
		"--render-site-output",
		"--site-source",
		"--site-config-url",
		"/pages/.nojekyll",
		"/pages/CNAME",
		"/pages/.mprlab-release.json",
	} {
		if strings.Contains(dockerfileText, forbiddenRenderer) {
			testingInstance.Errorf("Pages Dockerfile retains Go renderer contract %q", forbiddenRenderer)
		}
	}
}

func TestOperationalHostedCIUsesModuleGoToolchain(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	workflowBytes, readError := os.ReadFile(filepath.Join(repositoryRoot, ".github", "workflows", "test.yml"))
	if readError != nil {
		testingInstance.Fatalf("read hosted CI workflow: %v", readError)
	}
	workflowText := string(workflowBytes)
	if !strings.Contains(workflowText, "go-version-file: go.mod") {
		testingInstance.Fatal("hosted CI does not select the canonical go.mod toolchain")
	}
	if strings.Contains(workflowText, "GO_VERSION:") || strings.Contains(workflowText, "go-version:") {
		testingInstance.Fatal("hosted CI retains a second Go version declaration")
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
