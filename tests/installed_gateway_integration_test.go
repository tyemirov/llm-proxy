//go:build installed_gateway

package tests_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const lifecycleInstalledPlanFixture = `set -euo pipefail
export CI=1
for git_variable in "${!GIT_@}"; do
  unset "$git_variable"
done
export GIT_TERMINAL_PROMPT=0
export GIT_CONFIG_NOSYSTEM=1
export GIT_CONFIG_SYSTEM=/dev/null
export GIT_CONFIG_GLOBAL=/dev/null
export GIT_CONFIG_COUNT=0
exec </dev/null
repository_root="$1"
gateway_command="${MPRLAB_GATEWAY_EXECUTABLE:-mprlab-gateway}"
fixture_root="$(mktemp -d)"
trap 'rm -rf "$fixture_root"' EXIT
fixture_root="$(cd "$fixture_root" && pwd -P)"
application_root="$fixture_root/application"
mkdir -p "$application_root"
while IFS= read -r -d '' relative_path; do
  mkdir -p "$application_root/$(dirname "$relative_path")"
  cp -P "$repository_root/$relative_path" "$application_root/$relative_path"
done < <(git -C "$repository_root" ls-files -z)
git -C "$application_root" init --quiet --initial-branch=master
unset LLM_PROXY_MANAGEMENT_ADMIN_EMAILS LLM_PROXY_MANAGEMENT_GOOGLE_CLIENT_ID \
  LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY \
  DEPLOY_SUDO_PASSWORD_TUTOSH MPRLAB_TAUTH_MANAGEMENT_URL MPRLAB_TAUTH_PROVISIONING_CREDENTIALS
cat >"$application_root/.mprlab/deploy/.env" <<'ENVIRONMENT'
LLM_PROXY_MANAGEMENT_ADMIN_EMAILS=installed-gateway@example.invalid
LLM_PROXY_MANAGEMENT_GOOGLE_CLIENT_ID=installed-gateway.apps.googleusercontent.com
LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY=synthetic-installed-gateway-signing-key
LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY=synthetic-installed-gateway-encryption-key
DEPLOY_SUDO_PASSWORD_TUTOSH=synthetic-installed-gateway-sudo-password
MPRLAB_TAUTH_MANAGEMENT_URL=http://127.0.0.1:9
MPRLAB_TAUTH_PROVISIONING_CREDENTIALS={"llm-proxy":{"authentication":"tauthp_installed_gateway_fixture"}}
ENVIRONMENT
git -C "$application_root" check-ignore --quiet .mprlab/deploy/.env
git -C "$application_root" add --all
git -C "$application_root" -c core.hooksPath=/dev/null -c commit.gpgsign=false \
  -c user.name='Gateway integration test' -c user.email='gateway-test@example.invalid' \
  commit --quiet -m 'Selected manifest integration fixture'
git clone --quiet --bare "$application_root" "$fixture_root/origin.git"
cat >"$fixture_root/ssh-transport.sh" <<EOF
set -eu
if [ "\$#" -ne 2 ] || [ "\$1" != 'git@github.com' ] || [ "\$2" != "git-upload-pack 'tyemirov/llm-proxy.git'" ]; then
  printf 'unexpected fixture Git transport arguments\\n' >&2
  exit 2
fi
exec git upload-pack '$fixture_root/origin.git'
EOF
git -C "$application_root" remote add origin git@github.com:tyemirov/llm-proxy.git
export GIT_SSH_COMMAND="sh '$fixture_root/ssh-transport.sh'"
export GIT_SSH_VARIANT=simple
git -C "$application_root" config branch.master.remote origin
git -C "$application_root" config branch.master.merge refs/heads/master
git -C "$application_root" fetch --quiet origin
git -C "$application_root" remote set-head origin master
"$gateway_command" app-plan-release --app-root "$application_root" --operator-root "$fixture_root/operator" --ansible-inventory localhost,
`

func TestOperationalInstalledGatewayPlansCurrentTrackedManifest(testingInstance *testing.T) {
	trapPath := filepath.Join(testingInstance.TempDir(), "inherited-transport.sh")
	globalConfigPath := filepath.Join(testingInstance.TempDir(), "inherited-gitconfig")
	for path, content := range map[string]string{
		trapPath:         "printf 'inherited Git transport escaped fixture isolation\\n' >&2\nexit 91\n",
		globalConfigPath: "[url \"ssh://global.fixture.invalid/\"]\n\tinsteadOf = git@github.com:\n",
	} {
		file, createError := os.Create(path)
		if createError != nil {
			testingInstance.Fatalf("create competing Git input: %v", createError)
		}
		if _, writeError := file.WriteString(content); writeError != nil {
			testingInstance.Fatalf("write competing Git input: %v", writeError)
		}
		if closeError := file.Close(); closeError != nil {
			testingInstance.Fatalf("close competing Git input: %v", closeError)
		}
	}
	command := exec.Command("bash", "-c", lifecycleInstalledPlanFixture, "installed-gateway-plan", operationalRepositoryRoot(testingInstance))
	command.Env = append(os.Environ(), "CI=1", "GIT_TERMINAL_PROMPT=0",
		"GIT_SSH_COMMAND=sh '"+strings.ReplaceAll(trapPath, "'", "'\\''")+"'",
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=url.ssh://fixture.invalid/.insteadOf",
		"GIT_CONFIG_VALUE_0=git@github.com:", "GIT_CONFIG_GLOBAL="+globalConfigPath,
	)
	output, commandError := command.CombinedOutput()
	if commandError != nil {
		testingInstance.Fatalf("installed Gateway selected-manifest plan: %v\n%s", commandError, output)
	}
}
