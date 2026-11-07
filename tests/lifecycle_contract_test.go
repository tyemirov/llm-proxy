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
	"time"

	"gopkg.in/yaml.v3"
)

const lifecycleManifestRelativePath = ".mprlab/deploy/resources.yml"

func TestOperationalRepositoryOwnsVersionlessLifecycle(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	manifestBytes, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(lifecycleManifestRelativePath)))
	if err != nil {
		testingInstance.Fatal(err)
	}
	manifest := currentManifestContract(testingInstance, manifestBytes)
	requireCurrentContractYAML(testingInstance, "owner", manifest["owner"], `llm-proxy`)
	requireCurrentContractYAML(testingInstance, "release", manifest["release"], `{scheme: semver, fixed_major: 1}`)
	environment := currentContractMap(testingInstance, manifest["environment"])
	requireCurrentContractYAML(testingInstance, "environment.source", environment["source"], `.mprlab/deploy/.env`)
	inputs := currentContractIndex(testingInstance, environment["private_inputs"], "variable")
	expectedInputNames := []string{"LLM_PROXY_MANAGEMENT_ADMIN_EMAILS", "LLM_PROXY_MANAGEMENT_GOOGLE_CLIENT_ID", "LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY", "LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY", "DEPLOY_SUDO_PASSWORD_TUTOSH", "MPRLAB_TAUTH_MANAGEMENT_URL", "MPRLAB_TAUTH_PROVISIONING_CREDENTIALS"}
	if len(inputs) != len(expectedInputNames) {
		testingInstance.Fatalf("unexpected private inputs: %#v", inputs)
	}
	for _, name := range expectedInputNames {
		if inputs[name] == nil {
			testingInstance.Fatalf("missing declared input %s", name)
		}
	}
	targets := currentContractMap(testingInstance, manifest["targets"])
	requireCurrentContractYAML(testingInstance, "targets.access", targets["access"], `hosts: [{host: tutosh, elevation_password: {input: deploy-sudo-password-tutosh}}]`)
	lifecycle := currentContractMap(testingInstance, manifest["lifecycle"])
	requireCurrentContractYAML(testingInstance, "lifecycle.ci", lifecycle["ci"], `enabled: true
command: [make, ci]`)
	defaults := currentContractMap(testingInstance, lifecycle["defaults"])
	timeouts := currentContractMap(testingInstance, defaults["timeouts"])
	if len(defaults) != 1 || len(timeouts) != 5 {
		testingInstance.Fatalf("unexpected timeouts: %#v", defaults)
	}
	for _, name := range []string{"startup", "completion", "readiness", "request", "shutdown"} {
		value, err := time.ParseDuration(lifecycleStringField(testingInstance, timeouts, name))
		if err != nil || value <= 0 {
			testingInstance.Fatalf("%s requires a positive finite duration", name)
		}
	}
	operations := currentContractIndex(testingInstance, lifecycle["operations"], "name")
	if len(operations) != 4 {
		testingInstance.Fatalf("unexpected operations: %#v", operations)
	}
	for _, name := range []string{"app_lifecycle.repository_read", "app-release", "app-publish", "app-deploy"} {
		operation := operations[name]
		polling := currentContractMap(testingInstance, operation["polling"])
		if len(operation) != 2 || len(polling) != 5 {
			testingInstance.Fatalf("unexpected %s observation fields", name)
		}
		initial, initialErr := time.ParseDuration(lifecycleStringField(testingInstance, polling, "initial"))
		maximum, maximumErr := time.ParseDuration(lifecycleStringField(testingInstance, polling, "maximum"))
		increment, incrementErr := time.ParseDuration(lifecycleStringField(testingInstance, polling, "increment"))
		if initialErr != nil || maximumErr != nil || incrementErr != nil || initial <= 0 || maximum < initial || increment < 0 || polling["multiplier"].(int) < 1 || polling["attempt_limit"].(int) < 1 {
			testingInstance.Fatalf("invalid %s polling", name)
		}
	}
	compose := currentContractMap(testingInstance, targets["docker_compose"])
	projects := currentContractIndex(testingInstance, compose["projects"], "id")
	if len(projects) != 1 {
		testingInstance.Fatalf("unexpected runtime projects: %#v", projects)
	}
	project := projects["runtime"]
	requireCurrentContractYAML(testingInstance, "runtime.retired_services", project["retired_services"], `[{project: mprlab-nginx-gateway, service: llm-proxy}]`)
	services := currentContractIndex(testingInstance, project["services"], "id")
	if len(services) != 1 || services["llm-proxy"] == nil {
		testingInstance.Fatalf("unexpected services: %#v", services)
	}
	service := services["llm-proxy"]
	requireCurrentContractYAML(testingInstance, "llm-proxy.ports", service["ports"], `[{container_port: 8080, host_port: 18083, scope: private_lan}]`)
	endpoints := currentContractIndex(testingInstance, service["endpoints"], "id")
	if len(endpoints) != 2 || endpoints["http"] == nil || endpoints["http-private-lan"] == nil {
		testingInstance.Fatal("unexpected native endpoints")
	}
	requireCurrentContractYAML(testingInstance, "readiness-reference", service["readiness"], `endpoint: http`)
	requireCurrentContractYAML(testingInstance, "http-private-lan", endpoints["http-private-lan"], `{id: http-private-lan, name: llm-proxy.http.private-lan, version: 1, endpoint: {scope: private_lan, scheme: http, port: 18083}, health: {protocol: http, path: /healthz, expected_status: 200}}`)
	requireCurrentContractYAML(testingInstance, "http.endpoint", endpoints["http"]["endpoint"], `{scope: same_host, scheme: http, alias: llm-proxy, port: 8080}`)
	requireCurrentContractYAML(testingInstance, "http.health", endpoints["http"]["health"], `{protocol: http, path: /healthz, expected_status: 200}`)
	requireCurrentContractYAML(testingInstance, "llm-proxy.assets", service["assets"], `[{source: configs/config.yml, target: /app/config.yml}, {source: configs/providers.yml, target: /app/providers.yml}]`)
	if _, exists := service["environment_files"]; exists {
		testingInstance.Fatal("runtime retains environment_files")
	}
	runtimeEnvironment := currentContractIndex(testingInstance, service["environment"], "name")
	expectedEnvironmentNames := []string{"LLM_PROXY_MANAGEMENT_ADMIN_EMAILS", "LLM_PROXY_MANAGEMENT_API_ORIGIN", "LLM_PROXY_MANAGEMENT_DATABASE_PATH", "LLM_PROXY_MANAGEMENT_GOOGLE_CLIENT_ID", "LLM_PROXY_MANAGEMENT_JWT_ISSUER", "LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY", "LLM_PROXY_MANAGEMENT_LOCALHOST_ORIGIN", "LLM_PROXY_MANAGEMENT_LOOPBACK_ORIGIN", "LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY", "LLM_PROXY_MANAGEMENT_PROXY_ORIGIN", "LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN", "LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME", "LLM_PROXY_MANAGEMENT_TAUTH_LOGIN_PATH", "LLM_PROXY_MANAGEMENT_TAUTH_LOGOUT_PATH", "LLM_PROXY_MANAGEMENT_TAUTH_NONCE_PATH", "LLM_PROXY_MANAGEMENT_TAUTH_SESSION_PATH", "LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID", "LLM_PROXY_MANAGEMENT_TAUTH_URL", "LLM_PROXY_MANAGEMENT_UI_DESCRIPTION"}
	if len(runtimeEnvironment) != len(expectedEnvironmentNames) {
		testingInstance.Fatalf("unexpected runtime environment: %#v", runtimeEnvironment)
	}
	for _, name := range expectedEnvironmentNames {
		if runtimeEnvironment[name] == nil {
			testingInstance.Fatalf("missing runtime environment %s", name)
		}
	}
	for name, expected := range map[string]string{
		"LLM_PROXY_MANAGEMENT_ADMIN_EMAILS":                `{name: LLM_PROXY_MANAGEMENT_ADMIN_EMAILS, input: llm-proxy-management-admin-emails}`,
		"LLM_PROXY_MANAGEMENT_API_ORIGIN":                  `{name: LLM_PROXY_MANAGEMENT_API_ORIGIN, value: https://llm-proxy-api.mprlab.com}`,
		"LLM_PROXY_MANAGEMENT_GOOGLE_CLIENT_ID":            `{name: LLM_PROXY_MANAGEMENT_GOOGLE_CLIENT_ID, input: llm-proxy-management-google-client-id}`,
		"LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY":             `{name: LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY, input: llm-proxy-management-jwt-signing-key}`,
		"LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY": `{name: LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY, input: llm-proxy-management-provider-key-encryption-key}`,
		"LLM_PROXY_MANAGEMENT_PROXY_ORIGIN":                `{name: LLM_PROXY_MANAGEMENT_PROXY_ORIGIN, value: https://llm-proxy-api.mprlab.com}`,
		"LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN":               `{name: LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN, target: website, output: origin}`,
		"LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME":         `{name: LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME, value: app_session_llm_proxy}`,
		"LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID":             `{name: LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID, value: llm-proxy}`,
		"LLM_PROXY_MANAGEMENT_TAUTH_URL":                   `{name: LLM_PROXY_MANAGEMENT_TAUTH_URL, value: https://tauth-api.mprlab.com}`,
	} {
		requireCurrentContractYAML(testingInstance, name, runtimeEnvironment[name], expected)
	}
	caddy := currentContractMap(testingInstance, currentContractMap(testingInstance, targets["ingress"])["caddy"])
	routes := currentContractIndex(testingInstance, caddy["routes"], "id")
	if len(routes) != 1 || routes["public-api"] == nil {
		testingInstance.Fatal("unexpected API routes")
	}
	pages := currentContractIndex(testingInstance, currentContractMap(testingInstance, targets["github_pages"])["sites"], "id")
	if len(pages) != 1 || pages["website"] == nil {
		testingInstance.Fatal("unexpected Pages targets")
	}
	observations := currentContractIndex(testingInstance, currentContractMap(testingInstance, lifecycle["observations"])["http"], "id")
	if len(observations) != 2 || observations["api-health"] == nil || observations["website-health"] == nil {
		testingInstance.Fatal("unexpected public health observations")
	}
	publications := currentContractMap(testingInstance, manifest["publications"])
	for _, image := range currentContractIndex(testingInstance, currentContractMap(testingInstance, publications["ghcr"])["images"], "id") {
		if _, exists := image["visibility"]; exists {
			testingInstance.Fatal("GHCR visibility must remain provider-owned")
		}
	}
	manifestText := string(manifestBytes)
	for _, required := range []string{"name: mprlab-nginx-gateway_llm-proxy-data", "name: llm-proxy.http", "alias: llm-proxy", "hostname: llm-proxy-api.mprlab.com", "url: https://llm-proxy-api.mprlab.com/healthz", "url: https://llm-proxy.mprlab.com/healthz", "dockerfile: docker/pages/Dockerfile"} {
		if !strings.Contains(manifestText, required) {
			testingInstance.Errorf("manifest lacks %q", required)
		}
	}
	for _, forbidden := range []string{"schema_version:", "environment_files:", "profiles:", "secret:", "make_workflow", "ansible_task_bundle", "container_name:", "compose_template:", "npm_package", "mobile_application", "target: pages-deploy"} {
		if strings.Contains(manifestText, forbidden) {
			testingInstance.Errorf("manifest retains %q", forbidden)
		}
	}
}

func TestOperationalProductionLifecycleKeepsPolicyInAppAndDelegatesToInstalledGateway(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	makefileBytes, readError := os.ReadFile(filepath.Join(repositoryRoot, "Makefile"))
	if readError != nil {
		testingInstance.Fatalf("read Makefile: %v", readError)
	}
	makefileText := string(makefileBytes)
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

func TestOperationalReleaseDecisionUsesGixVersion(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	repositoryTag := "v1.4.1"
	nextTag := "v1.4.2"

	testCases := []struct {
		name       string
		output     string
		wantStatus bool
		wantText   string
	}{
		{
			name:       "Gix reuses a release version",
			output:     `{"contract":"mprlab.version-decision/v3","policy":{"scheme":"semver","fixed_major":1},"next_version":"` + repositoryTag + `"}`,
			wantStatus: true,
			wantText:   "LLM_PROXY_RELEASE_POLICY_OK version=" + repositoryTag,
		},
		{
			name:       "Gix selects the next version",
			wantStatus: true,
			output:     `{"contract":"mprlab.version-decision/v3","policy":{"scheme":"semver","fixed_major":1},"next_version":"` + nextTag + `"}`,
			wantText:   "LLM_PROXY_RELEASE_POLICY_OK version=" + nextTag,
		},
		{
			name:       "Gix selects a minor release",
			output:     `{"contract":"mprlab.version-decision/v3","policy":{"scheme":"semver","fixed_major":1},"next_version":"v1.5.0"}`,
			wantStatus: true,
			wantText:   "LLM_PROXY_RELEASE_POLICY_OK version=v1.5.0",
		},
		{
			name:     "malformed version",
			output:   `{"contract":"mprlab.version-decision/v3","policy":{"scheme":"semver","fixed_major":1},"next_version":"v1.05.0"}`,
			wantText: "llm_proxy.release_version_invalid: expected a major version 1 SemVer release",
		},
		{
			name:     "missing version",
			output:   `{"contract":"mprlab.version-decision/v3","policy":{"scheme":"semver","fixed_major":1}}`,
			wantText: "llm_proxy.release_version_invalid: expected a major version 1 SemVer release",
		},
		{
			name:     "higher major",
			output:   `{"contract":"mprlab.version-decision/v3","policy":{"scheme":"semver","fixed_major":1},"next_version":"v2.0.0"}`,
			wantText: "llm_proxy.release_version_invalid: expected a major version 1 SemVer release",
		},
		{
			name:     "missing fixed major",
			output:   `{"contract":"mprlab.version-decision/v3","policy":{"scheme":"semver"},"next_version":"` + repositoryTag + `"}`,
			wantText: "llm_proxy.release_policy_invalid: expected SemVer decision with fixed major 1",
		},
		{
			name:     "different fixed major",
			output:   `{"contract":"mprlab.version-decision/v3","policy":{"scheme":"semver","fixed_major":2},"next_version":"` + repositoryTag + `"}`,
			wantText: "llm_proxy.release_policy_invalid: expected SemVer decision with fixed major 1",
		},
		{
			name:     "retired decision contract",
			output:   `{"contract":"mprlab.version-decision/v2","policy":{"scheme":"semver","fixed_major":1},"next_version":"` + repositoryTag + `"}`,
			wantText: "llm_proxy.release_policy_invalid: expected one release decision document",
		},
		{
			name:     "missing decision",
			output:   `not a version decision`,
			wantText: "llm_proxy.release_policy_invalid: expected one release decision document",
		},
	}

	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(testingInstance *testing.T) {
			command := exec.Command(filepath.Join(repositoryRoot, "scripts", "validate-release-decision"))
			command.Dir = repositoryRoot
			command.Stdin = strings.NewReader(testCase.output)
			output, runError := command.CombinedOutput()
			if (runError == nil) != testCase.wantStatus || !strings.Contains(string(output), testCase.wantText) {
				testingInstance.Fatalf("release policy status=%v error=%v output=%s", testCase.wantStatus, runError, output)
			}
		})
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
	if !strings.Contains(workflowText, "fetch-depth: 0") {
		testingInstance.Fatal("hosted CI does not fetch release tags for Python package metadata")
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

func currentContractMap(t *testing.T, value any) map[string]any {
	t.Helper()
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected native declaration mapping, got %#v", value)
	}
	return result
}

func currentManifestContract(t *testing.T, content []byte) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := yaml.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("parse selected manifest: %v", err)
	}
	if len(envelope) != 1 {
		t.Fatalf("unexpected manifest envelope: %#v", envelope)
	}
	actual := currentContractMap(t, envelope["mprlab_resources"])
	keys := make([]string, 0, len(actual))
	for key := range actual {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	expected := []string{"artifacts", "environment", "lifecycle", "owner", "publications", "release", "targets"}
	if !reflect.DeepEqual(keys, expected) {
		t.Fatalf("selected manifest root keys=%v want=%v", keys, expected)
	}
	return actual
}

func currentContractIndex(t *testing.T, value any, identityKey string) map[string]map[string]any {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("expected native declaration list, got %#v", value)
	}
	result := make(map[string]map[string]any, len(items))
	for _, item := range items {
		declaration := currentContractMap(t, item)
		identity, ok := declaration[identityKey].(string)
		if !ok || identity == "" || result[identity] != nil {
			t.Fatalf("invalid or duplicate %s in %#v", identityKey, value)
		}
		result[identity] = declaration
	}
	return result
}

func requireCurrentContractYAML(t *testing.T, path string, actual any, expectedText string) {
	t.Helper()
	var expected any
	if err := yaml.Unmarshal([]byte(expectedText), &expected); err != nil {
		t.Fatalf("parse expected %s: %v", path, err)
	}
	requireCurrentContractValue(t, path, actual, expected)
}

func requireCurrentContractValue(t *testing.T, path string, actual, expected any) {
	t.Helper()
	switch wanted := expected.(type) {
	case map[string]any:
		mapping, ok := actual.(map[string]any)
		if !ok {
			t.Fatalf("%s requires a mapping, got %#v", path, actual)
		}
		if !strings.HasSuffix(path, ".assets[]") && len(mapping) != len(wanted) {
			t.Fatalf("%s has unexpected fields: %#v", path, actual)
		}
		for key, value := range wanted {
			observed, exists := mapping[key]
			if !exists {
				t.Fatalf("%s lacks %s", path, key)
			}
			requireCurrentContractValue(t, path+"."+key, observed, value)
		}
	case []any:
		items, ok := actual.([]any)
		if !ok || len(items) != len(wanted) {
			t.Fatalf("%s list=%#v want=%#v", path, actual, wanted)
		}
		for index, value := range wanted {
			observed := items[index]
			if declaration, isMapping := value.(map[string]any); isMapping {
				identityKey := "id"
				if _, exists := declaration[identityKey]; !exists {
					identityKey = "name"
				}
				if identity, exists := declaration[identityKey]; exists {
					matches := 0
					for _, candidate := range items {
						if entry, valid := candidate.(map[string]any); valid && entry[identityKey] == identity {
							observed = candidate
							matches++
						}
					}
					if matches != 1 {
						t.Fatalf("%s requires one %s=%v, got %d", path, identityKey, identity, matches)
					}
				}
			}
			requireCurrentContractValue(t, path+"[]", observed, value)
		}
	default:
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("%s=%#v want=%#v", path, actual, expected)
		}
	}
}
