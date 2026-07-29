#!/usr/bin/env bash
set -euo pipefail

[[ $# -eq 0 ]] || {
  printf '%s\n' 'error: llm-proxy deployment accepts no arguments' >&2
  exit 2
}

deployment_mode="${LLM_PROXY_DEPLOY_MODE:-}"
[[ "${deployment_mode}" == "dry-run" || "${deployment_mode}" == "deploy" ]] || {
  printf '%s\n' 'error: LLM_PROXY_DEPLOY_MODE must be dry-run or deploy' >&2
  exit 2
}

image_repository="${DOCKER_IMAGE:-ghcr.io/tyemirov/llm-proxy}"
[[ "${image_repository}" == "ghcr.io/tyemirov/llm-proxy" ]] || {
  printf '%s\n' 'error: deployment requires the canonical ghcr.io/tyemirov/llm-proxy repository' >&2
  exit 2
}
publish_remote="${PUBLISH_REMOTE:-origin}"
publish_branch="${PUBLISH_BRANCH:-master}"
pages_branch="${PAGES_BRANCH:-gh-pages}"
pages_url="${PAGES_URL:-https://llm-proxy.mprlab.com/}"

command -v git >/dev/null 2>&1 || {
  printf '%s\n' 'error: git is required' >&2
  exit 2
}
command -v python3 >/dev/null 2>&1 || {
  printf '%s\n' 'error: python3 is required' >&2
  exit 2
}
command -v docker >/dev/null 2>&1 || {
  printf '%s\n' 'error: docker is required for image verification' >&2
  exit 2
}

repo_root="$(git rev-parse --show-toplevel)"
cd "${repo_root}"
release_helper="${RELEASE_HELPER:-${repo_root}/tools/gitrelease/scripts/release_helper.py}"
manifest_digest_helper="${repo_root}/tools/gitrelease/scripts/resolve_container_manifest_digest.sh"
app_deploy_runner="${repo_root}/scripts/run-app-ansible-deploy.sh"
for required_file in "${release_helper}" "${manifest_digest_helper}" "${app_deploy_runner}"; do
  [[ -f "${required_file}" && ! -L "${required_file}" ]] || {
    printf 'error: deployment input must be a regular file: %s\n' "${required_file}" >&2
    exit 2
  }
done

release_state_json="$(mktemp)"
cleanup() {
  rm -f -- "${release_state_json}"
}
trap cleanup EXIT

if ! python3 "${release_helper}" local-release-state \
  --default-branch "${publish_branch}" \
  >"${release_state_json}"
then
  cat "${release_state_json}" >&2
  printf '%s\n' 'error: make deploy requires the exact sealed release created by make release' >&2
  exit 1
fi
release_state_summary="$(
  python3 -c '
import json
import sys

with open(sys.argv[1], encoding="utf-8") as release_state_handle:
    release_state = json.load(release_state_handle)
if release_state.get("ok") is not True:
    raise SystemExit("release helper did not confirm local release state")
state = release_state.get("state")
version = release_state.get("version", "")
release_commit = release_state.get("release_commit", "")
if (
    not isinstance(state, str)
    or not isinstance(version, str)
    or not isinstance(release_commit, str)
):
    raise SystemExit("release helper returned an invalid local release identity")
print(state)
print(version)
print(release_commit)
' "${release_state_json}"
)" || {
  printf '%s\n' 'error: release helper returned an invalid local release state' >&2
  exit 1
}
release_state="$(sed -n '1p' <<<"${release_state_summary}")"
release_tag="$(sed -n '2p' <<<"${release_state_summary}")"
sealed_release_commit="$(sed -n '3p' <<<"${release_state_summary}")"
if [[ "${release_state}" != "sealed" ]]; then
  printf '%s\n' 'error: make deploy requires the exact sealed release created by make release' >&2
  exit 1
fi
if ! version_validation="$(
  python3 "${release_helper}" validate-version --version "${release_tag}" 2>&1
)"; then
  printf '%s\n' "${version_validation}" >&2
  exit 1
fi

head_commit="$(git rev-parse HEAD)"
if [[ -z "${sealed_release_commit}" || "${sealed_release_commit}" != "${head_commit}" ]]; then
  printf 'error: sealed release commit %s does not match deploy HEAD %s\n' "${sealed_release_commit:-<missing>}" "${head_commit}" >&2
  exit 1
fi
head_release_tag="$(git tag --points-at HEAD --list 'v*' --sort=-version:refname | head -n 1)"
if [[ -z "${head_release_tag}" ]]; then
  printf '%s\n' 'error: make deploy requires the exact sealed release tag created by make release' >&2
  exit 1
fi
if [[ "${release_tag}" != "${head_release_tag}" ]]; then
  printf 'error: sealed release version %s does not match deploy tag %s\n' "${release_tag}" "${head_release_tag}" >&2
  exit 1
fi
tag_commit="$(git rev-list -n 1 "${release_tag}" 2>/dev/null || true)"
if [[ "${tag_commit}" != "${head_commit}" ]]; then
  printf 'error: deploy tag %s does not point at HEAD\n' "${release_tag}" >&2
  exit 1
fi

current_branch="$(git rev-parse --abbrev-ref HEAD)"
if [[ "${current_branch}" != "${publish_branch}" ]]; then
  printf "error: deployment is allowed only from branch '%s' (current: '%s')\n" "${publish_branch}" "${current_branch}" >&2
  exit 1
fi
if [[ -n "$(git status --porcelain)" ]]; then
  printf '%s\n' 'error: working tree is dirty; commit or stash changes before deploying' >&2
  exit 1
fi
timeout -k 30s -s SIGKILL 30s \
  git fetch "${publish_remote}" "${publish_branch}" --tags --prune
remote_branch_commit="$(git rev-parse "${publish_remote}/${publish_branch}")"
if [[ "${head_commit}" != "${remote_branch_commit}" ]]; then
  printf 'error: local %s is not at %s/%s; push or pull first\n' "${publish_branch}" "${publish_remote}" "${publish_branch}" >&2
  exit 1
fi

printf '==> [deploy] Verifying %s:latest matches %s\n' "${image_repository}" "${release_tag}"
release_digest="$(bash "${manifest_digest_helper}" "${image_repository}:${release_tag}")"
latest_digest="$(bash "${manifest_digest_helper}" "${image_repository}:latest")"
[[ "${release_digest}" =~ ^sha256:[0-9a-f]{64}$ ]] || {
  printf 'error: could not resolve canonical digest for %s:%s\n' "${image_repository}" "${release_tag}" >&2
  exit 1
}
[[ "${latest_digest}" =~ ^sha256:[0-9a-f]{64}$ ]] || {
  printf 'error: could not resolve canonical digest for %s:latest\n' "${image_repository}" >&2
  exit 1
}
if [[ "${release_digest}" != "${latest_digest}" ]]; then
  printf 'error: %s:latest digest %s does not match %s digest %s; run make publish first\n' \
    "${image_repository}" "${latest_digest}" "${release_tag}" "${release_digest}" >&2
  exit 1
fi
image_ref="${image_repository}@${release_digest}"

if [[ "${deployment_mode}" == "dry-run" ]]; then
  printf '==> [deploy] Validating the pinned LLM Proxy transaction for %s\n' "${image_ref}"
  MPRLAB_LLM_PROXY_IMAGE_REF="${image_ref}" \
    "${app_deploy_runner}" --mode dry-run
  printf '%s\n' 'llm-proxy deploy dry-run complete; production state was not changed'
  exit 0
fi

printf '==> [deploy] Validating the published Pages artifact for %s\n' "${release_tag}"
PUBLISH_REMOTE="${publish_remote}" \
PAGES_BRANCH="${pages_branch}" \
PAGES_URL="${pages_url}" \
PAGES_VERSION="${release_tag}" \
DEPLOY_PAGES_ARGS="--verify-only" \
  make --no-print-directory pages-deploy

printf '==> [deploy] Converging the exact LLM Proxy backend %s\n' "${image_ref}"
MPRLAB_LLM_PROXY_IMAGE_REF="${image_ref}" \
  "${app_deploy_runner}" --mode deploy

printf '==> [deploy] Activating the published Pages artifact for %s\n' "${release_tag}"
PUBLISH_REMOTE="${publish_remote}" \
PAGES_BRANCH="${pages_branch}" \
PAGES_URL="${pages_url}" \
PAGES_VERSION="${release_tag}" \
DEPLOY_PAGES_ARGS="" \
  make --no-print-directory pages-deploy

printf '%s\n' 'llm-proxy deploy complete'
