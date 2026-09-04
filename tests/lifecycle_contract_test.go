package tests_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
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

func TestOperationalRepositoryOwnsVersionlessLifecycle(testingInstance *testing.T) {
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
	if owner, ownerAvailable := resourcesDocument["owner"].(string); !ownerAvailable || owner != "llm-proxy" {
		testingInstance.Fatalf("unexpected lifecycle owner: %#v", resourcesDocument["owner"])
	}
	release, releaseAvailable := resourcesDocument["release"].(map[string]any)
	if !releaseAvailable || len(release) != 2 || release["scheme"] != "semver" || release["fixed_major"] != 1 {
		testingInstance.Fatalf("unexpected lifecycle release policy: %#v", resourcesDocument["release"])
	}
	if _, dependenciesAvailable := resourcesDocument["dependencies"]; dependenciesAvailable {
		testingInstance.Fatalf("lifecycle manifest must not declare top-level dependencies: %#v", resourcesDocument["dependencies"])
	}
	resourceKeys := make([]string, 0, len(resourcesDocument))
	for key := range resourcesDocument {
		resourceKeys = append(resourceKeys, key)
	}
	slices.Sort(resourceKeys)
	if !slices.Equal(resourceKeys, []string{"owner", "release", "resources"}) {
		testingInstance.Fatalf("lifecycle manifest must contain only owner, release, and resources: %#v", resourcesDocument)
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
		"health_check/api-health",
		"health_check/website-health",
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
	expectedRuntimeAssets := []any{
		map[string]any{
			"source": "configs/config.yml",
			"target": "/app/config.yml",
			"mode":   "0444",
		},
		map[string]any{
			"source": "configs/providers.yml",
			"target": "/app/providers.yml",
			"mode":   "0444",
		},
	}
	if !reflect.DeepEqual(runtimeService["assets"], expectedRuntimeAssets) {
		testingInstance.Fatalf("unexpected runtime assets: %#v", runtimeService["assets"])
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
		"name: mprlab-nginx-gateway_llm-proxy-data",
		"name: llm-proxy.http",
		"alias: llm-proxy",
		"hostname: llm-proxy-api.mprlab.com",
		"url: https://llm-proxy-api.mprlab.com/healthz",
		"url: https://llm-proxy.mprlab.com/healthz",
		"dockerfile: docker/pages/Dockerfile",
		"capability: tauth.tenants",
	} {
		if !strings.Contains(manifestText, requiredContract) {
			testingInstance.Errorf("lifecycle manifest is missing %q", requiredContract)
		}
	}
	for _, forbiddenContract := range []string{
		"schema_version:",
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

func TestOperationalProductionLifecycleKeepsPolicyInAppAndDelegatesToSiblingGateway(testingInstance *testing.T) {
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
	validatorPath := filepath.Join(repositoryRoot, "scripts", "validate-release-decision")
	validatorMetadata, statError := os.Stat(validatorPath)
	if statError != nil || !validatorMetadata.Mode().IsRegular() || validatorMetadata.Mode().Perm() != 0o755 {
		testingInstance.Fatalf("release decision validator is not a mode-0755 regular file: %v", statError)
	}
}

func TestOperationalReleaseDecisionMustMatchRepositoryVersion(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	repositoryVersion := operationalRepositoryReleaseVersion(testingInstance, repositoryRoot)
	repositoryTag := "v" + repositoryVersion
	nextVersion := nextOperationalRepositoryReleaseVersion(testingInstance, repositoryVersion)
	nextTag := "v" + nextVersion

	testCases := []struct {
		name       string
		output     string
		wantStatus bool
		wantText   string
	}{
		{
			name:       "exact repository version",
			output:     `{"contract":"mprlab.version-decision/v2","policy":{"scheme":"semver","fixed_major":1},"next_version":"` + repositoryTag + `"}`,
			wantStatus: true,
			wantText:   "LLM_PROXY_RELEASE_POLICY_OK version=" + repositoryTag,
		},
		{
			name:     "different major one version",
			output:   `{"contract":"mprlab.version-decision/v2","policy":{"scheme":"semver","fixed_major":1},"next_version":"` + nextTag + `"}`,
			wantText: "llm_proxy.release_version_invalid: release version must match repository version " + repositoryTag,
		},
		{
			name:     "higher major",
			output:   `{"contract":"mprlab.version-decision/v2","policy":{"scheme":"semver","fixed_major":1},"next_version":"v2.0.0"}`,
			wantText: "llm_proxy.release_version_invalid: release version must match repository version " + repositoryTag,
		},
		{
			name:     "missing fixed major",
			output:   `{"contract":"mprlab.version-decision/v2","policy":{"scheme":"semver"},"next_version":"` + repositoryTag + `"}`,
			wantText: "llm_proxy.release_policy_invalid: expected SemVer decision with fixed major 1",
		},
		{
			name:     "different fixed major",
			output:   `{"contract":"mprlab.version-decision/v2","policy":{"scheme":"semver","fixed_major":2},"next_version":"` + repositoryTag + `"}`,
			wantText: "llm_proxy.release_policy_invalid: expected SemVer decision with fixed major 1",
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

	testingInstance.Run("repository version drift", func(testingInstance *testing.T) {
		fixtureRoot := newOperationalReleaseVersionFixture(testingInstance, repositoryRoot)
		projectPath := filepath.Join(fixtureRoot, "python", "pyproject.toml")
		projectBytes, readError := os.ReadFile(projectPath)
		if readError != nil {
			testingInstance.Fatalf("read fixture Python project: %v", readError)
		}
		writeOperationalFile(
			testingInstance,
			projectPath,
			strings.Replace(
				string(projectBytes),
				`version = "`+repositoryVersion+`"`,
				`version = "`+nextVersion+`"`,
				1,
			),
			0o644,
		)
		command := exec.Command(filepath.Join(fixtureRoot, "scripts", "validate-release-decision"))
		command.Dir = fixtureRoot
		command.Stdin = strings.NewReader(`{"contract":"mprlab.version-decision/v2","policy":{"scheme":"semver","fixed_major":1},"next_version":"` + repositoryTag + `"}`)
		output, runError := command.CombinedOutput()
		if runError == nil || !strings.Contains(string(output), "llm_proxy.release_policy_invalid: Python project version must match repository version "+repositoryVersion) {
			testingInstance.Fatalf("repository version drift error=%v output=%s", runError, output)
		}
	})
}

func TestOperationalRepositoryReleaseVersionCommand(testingInstance *testing.T) {
	repositoryRoot := operationalRepositoryRoot(testingInstance)
	repositoryVersion := operationalRepositoryReleaseVersion(testingInstance, repositoryRoot)
	nextVersion := nextOperationalRepositoryReleaseVersion(testingInstance, repositoryVersion)
	checkCommand := exec.Command("make", "check-release-version")
	checkCommand.Dir = repositoryRoot
	checkOutput, checkError := checkCommand.CombinedOutput()
	if checkError != nil || !strings.Contains(string(checkOutput), "LLM_PROXY_RELEASE_VERSION_OK version=v"+repositoryVersion) {
		testingInstance.Fatalf("repository release version check error=%v output=%s", checkError, checkOutput)
	}

	testingInstance.Run("updates all explicit versions", func(testingInstance *testing.T) {
		fixtureRoot := newOperationalReleaseVersionFixture(testingInstance, repositoryRoot)
		setOutput, setError := runOperationalReleaseVersionMakeCommand(testingInstance, fixtureRoot, "set-release-version", nextVersion)
		if setError != nil || !strings.Contains(setOutput, "LLM_PROXY_RELEASE_VERSION_UPDATED version=v"+nextVersion) {
			testingInstance.Fatalf("set repository release version error=%v output=%s", setError, setOutput)
		}
		checkOutput, checkError := runOperationalReleaseVersionMakeCommand(testingInstance, fixtureRoot, "check-release-version", "")
		if checkError != nil || !strings.Contains(checkOutput, "LLM_PROXY_RELEASE_VERSION_OK version=v"+nextVersion) {
			testingInstance.Fatalf("check updated repository release version error=%v output=%s", checkError, checkOutput)
		}
		decisionCommand := exec.Command(filepath.Join(fixtureRoot, "scripts", "validate-release-decision"))
		decisionCommand.Dir = fixtureRoot
		decisionCommand.Stdin = strings.NewReader(`{"contract":"mprlab.version-decision/v2","policy":{"scheme":"semver","fixed_major":1},"next_version":"v` + nextVersion + `"}`)
		decisionOutput, decisionError := decisionCommand.CombinedOutput()
		if decisionError != nil || !strings.Contains(string(decisionOutput), "LLM_PROXY_RELEASE_POLICY_OK version=v"+nextVersion) {
			testingInstance.Fatalf("validate updated repository release decision error=%v output=%s", decisionError, decisionOutput)
		}
		for relativePath, expectedText := range map[string]string{
			"VERSION":               nextVersion + "\n",
			"python/pyproject.toml": `version = "` + nextVersion + `"`,
			"python/uv.lock":        `version = "` + nextVersion + `"`,
		} {
			fileBytes, readError := os.ReadFile(filepath.Join(fixtureRoot, filepath.FromSlash(relativePath)))
			if readError != nil || !strings.Contains(string(fileBytes), expectedText) {
				testingInstance.Errorf("updated %s error=%v contents=%s", relativePath, readError, fileBytes)
			}
		}
	})

	for _, testCase := range []struct {
		name            string
		version         string
		preparedVersion string
		want            string
	}{
		{name: "rejects malformed value", version: "v" + nextVersion, want: "target version must use canonical major version 1"},
		{name: "rejects major two", version: "2.0.0", want: "target version must use canonical major version 1"},
		{
			name:            "rejects decrease",
			version:         repositoryVersion,
			preparedVersion: nextVersion,
			want:            "repository version cannot decrease from " + nextVersion + " to " + repositoryVersion,
		},
	} {
		testingInstance.Run(testCase.name, func(testingInstance *testing.T) {
			fixtureRoot := newOperationalReleaseVersionFixture(testingInstance, repositoryRoot)
			if testCase.preparedVersion != "" {
				output, runError := runOperationalReleaseVersionMakeCommand(
					testingInstance,
					fixtureRoot,
					"set-release-version",
					testCase.preparedVersion,
				)
				if runError != nil {
					testingInstance.Fatalf("prepare repository version error=%v output=%s", runError, output)
				}
			}
			paths := []string{"VERSION", "python/pyproject.toml", "python/uv.lock"}
			before := make(map[string]string, len(paths))
			for _, relativePath := range paths {
				fileBytes, readError := os.ReadFile(filepath.Join(fixtureRoot, filepath.FromSlash(relativePath)))
				if readError != nil {
					testingInstance.Fatalf("read %s before rejected update: %v", relativePath, readError)
				}
				before[relativePath] = string(fileBytes)
			}
			output, runError := runOperationalReleaseVersionMakeCommand(testingInstance, fixtureRoot, "set-release-version", testCase.version)
			if runError == nil || !strings.Contains(output, testCase.want) {
				testingInstance.Fatalf("rejected version error=%v output=%s", runError, output)
			}
			for _, relativePath := range paths {
				fileBytes, readError := os.ReadFile(filepath.Join(fixtureRoot, filepath.FromSlash(relativePath)))
				if readError != nil || string(fileBytes) != before[relativePath] {
					testingInstance.Errorf("rejected update changed %s error=%v", relativePath, readError)
				}
			}
		})
	}
}

func newOperationalReleaseVersionFixture(testingInstance *testing.T, repositoryRoot string) string {
	testingInstance.Helper()
	fixtureRoot := testingInstance.TempDir()
	for _, relativePath := range []string{
		"Makefile",
		"VERSION",
		"python/pyproject.toml",
		"python/uv.lock",
		"scripts/release_version.py",
		"scripts/validate-release-decision",
		"scripts/validate_release_policy.py",
	} {
		copyOperationalFile(
			testingInstance,
			filepath.Join(repositoryRoot, filepath.FromSlash(relativePath)),
			filepath.Join(fixtureRoot, filepath.FromSlash(relativePath)),
		)
	}
	return fixtureRoot
}

func runOperationalReleaseVersionMakeCommand(testingInstance *testing.T, fixtureRoot string, target string, version string) (string, error) {
	testingInstance.Helper()
	commandArguments := []string{"--no-print-directory", target}
	if version != "" {
		commandArguments = append(commandArguments, "RELEASE_VERSION="+version)
	}
	command := exec.Command("make", commandArguments...)
	command.Dir = fixtureRoot
	output, runError := command.CombinedOutput()
	return string(output), runError
}

func operationalRepositoryReleaseVersion(testingInstance *testing.T, repositoryRoot string) string {
	testingInstance.Helper()
	versionBytes, readError := os.ReadFile(filepath.Join(repositoryRoot, "VERSION"))
	if readError != nil {
		testingInstance.Fatalf("read repository release version: %v", readError)
	}
	return strings.TrimSuffix(string(versionBytes), "\n")
}

func nextOperationalRepositoryReleaseVersion(testingInstance *testing.T, version string) string {
	testingInstance.Helper()
	components := strings.Split(version, ".")
	if len(components) != 3 || components[0] != "1" {
		testingInstance.Fatalf("unexpected repository release version %q", version)
	}
	patchVersion, parseError := strconv.Atoi(components[2])
	if parseError != nil {
		testingInstance.Fatalf("parse repository release version %q: %v", version, parseError)
	}
	return strings.Join([]string{components[0], components[1], strconv.Itoa(patchVersion + 1)}, ".")
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
	if !strings.Contains(workflowText, "      - 'VERSION'\n") {
		testingInstance.Fatal("hosted CI does not run for canonical repository version changes")
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
