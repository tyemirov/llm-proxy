#!/usr/bin/env bash
set -euo pipefail

usage() {
  builtin printf '%s\n' 'Usage:
  scripts/deploy.sh [options]

Deploys llm-proxy after verifying the release image and Pages archive were
published. The backend deploy runs through mprlab-gateway, then the exact
published Pages archive replaces the live branch.

Options:
  --gateway-dir <path>  Gateway checkout. Default: $GATEWAY_DIR or sibling ../mprlab-gateway
  --image <value>       Image repository. Default: $DOCKER_IMAGE or ghcr.io/tyemirov/llm-proxy
  --tag <value>         Release tag. Default: v* tag pointing at HEAD
  --skip-ci             Skip the local make ci deployment gate
  --skip-image-verify   Skip release/latest image digest verification
  --skip-pages          Skip GitHub Pages activation
  --pages-branch <value> Pages branch to publish. Default: $PAGES_BRANCH or gh-pages
  --pages-url <value>   Public Pages URL. Default: $PAGES_URL or https://llm-proxy.mprlab.com/
  --skip-gateway        Skip gateway deployment
  --help                Show this help text

Environment:
  DEPLOY_CI_TIMEOUT_SECONDS  make ci timeout in seconds. Default: $LLM_PROXY_CI_TIMEOUT_SECONDS or 350'
}

require_positive_integer() {
  local name="$1"
  local value="$2"
  if [[ ! "${value}" =~ ^[1-9][0-9]*$ ]]; then
    echo "error: ${name} must be a positive integer number of seconds (got: ${value})" >&2
    exit 1
  fi
}

env_or_default() {
  local name="$1"
  local fallback="$2"
  local value=""
  if [[ -v "${name}" ]]; then
    value="${!name}"
  fi
  if [[ -n "${value}" ]]; then
    printf "%s\n" "${value}"
  else
    printf "%s\n" "${fallback}"
  fi
}

GATEWAY_DIR="$(env_or_default GATEWAY_DIR "")"
GATEWAY_TARGET="deploy-llm-proxy-backend"
GATEWAY_CONTRACT_TARGET="verify-llm-proxy-deployment-contract"
GATEWAY_DEPLOY_REMOTE="origin"
GATEWAY_DEPLOY_BRANCH="master"
IMAGE_REPOSITORY="$(env_or_default DOCKER_IMAGE ghcr.io/tyemirov/llm-proxy)"
TAG="$(env_or_default DEPLOY_TAG "")"
SKIP_CI="false"
SKIP_IMAGE_VERIFY="false"
SKIP_GATEWAY="false"
SKIP_PAGES="$(env_or_default DEPLOY_SKIP_PAGES false)"
PAGES_BRANCH="$(env_or_default PAGES_BRANCH gh-pages)"
PAGES_URL="$(env_or_default PAGES_URL https://llm-proxy.mprlab.com/)"
DEPLOY_BRANCH="$(env_or_default DEPLOY_BRANCH master)"
DEPLOY_REMOTE="$(env_or_default DEPLOY_REMOTE origin)"
LLM_PROXY_CI_TIMEOUT_SECONDS_EFFECTIVE="$(env_or_default LLM_PROXY_CI_TIMEOUT_SECONDS 350)"
CI_TIMEOUT_SECONDS="$(env_or_default DEPLOY_CI_TIMEOUT_SECONDS "${LLM_PROXY_CI_TIMEOUT_SECONDS_EFFECTIVE}")"

resolve_release_tag() {
  if [[ -n "${TAG}" ]]; then
    printf '%s\n' "${TAG}"
    return
  fi
  git tag --points-at HEAD --list 'v*' --sort=-version:refname | head -n 1
}

image_digest() {
  local image_ref="$1"
  docker buildx imagetools inspect "$image_ref" | awk '/^Digest:/ { print $2; exit }'
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --gateway-dir)
      [[ $# -ge 2 ]] || { echo "error: --gateway-dir requires a value" >&2; exit 1; }
      GATEWAY_DIR="$2"
      shift 2
      ;;
    --image)
      [[ $# -ge 2 ]] || { echo "error: --image requires a value" >&2; exit 1; }
      IMAGE_REPOSITORY="$2"
      shift 2
      ;;
    --tag)
      [[ $# -ge 2 ]] || { echo "error: --tag requires a value" >&2; exit 1; }
      TAG="$2"
      shift 2
      ;;
    --skip-ci)
      SKIP_CI="true"
      shift
      ;;
    --skip-image-verify)
      SKIP_IMAGE_VERIFY="true"
      shift
      ;;
    --skip-pages)
      SKIP_PAGES="true"
      shift
      ;;
    --pages-branch)
      [[ $# -ge 2 ]] || { echo "error: --pages-branch requires a value" >&2; exit 1; }
      PAGES_BRANCH="$2"
      shift 2
      ;;
    --pages-url)
      [[ $# -ge 2 ]] || { echo "error: --pages-url requires a value" >&2; exit 1; }
      PAGES_URL="$2"
      shift 2
      ;;
    --skip-gateway)
      SKIP_GATEWAY="true"
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

command -v git >/dev/null 2>&1 || { echo "error: git is required" >&2; exit 1; }
require_positive_integer "DEPLOY_CI_TIMEOUT_SECONDS" "${CI_TIMEOUT_SECONDS}"

repo_root="$(git rev-parse --show-toplevel)"
cd "${repo_root}"
release_helper="${RELEASE_HELPER:-${repo_root}/tools/gitrelease/scripts/release_helper.py}"
[[ -f "${release_helper}" ]] || { echo "error: release helper is missing: ${release_helper}" >&2; exit 1; }

resolve_gateway_dir() {
  if [[ -n "${GATEWAY_DIR}" ]]; then
    printf "%s\n" "${GATEWAY_DIR}"
    return
  fi
  printf "%s\n" "${repo_root}/../mprlab-gateway"
}

if [[ "${SKIP_GATEWAY}" != "true" ]]; then
  GATEWAY_DIR="$(resolve_gateway_dir)"
  [[ -n "${GATEWAY_DIR}" ]] || { echo "error: gateway checkout not found; set GATEWAY_DIR=/path/to/mprlab-gateway or pass --gateway-dir" >&2; exit 1; }
  [[ -d "${GATEWAY_DIR}" ]] || { echo "error: gateway checkout not found: ${GATEWAY_DIR}" >&2; exit 1; }

  git -C "${GATEWAY_DIR}" rev-parse --show-toplevel >/dev/null 2>&1 || { echo "error: gateway checkout is not a git worktree: ${GATEWAY_DIR}" >&2; exit 1; }
  gateway_branch="$(git -C "${GATEWAY_DIR}" rev-parse --abbrev-ref HEAD)"
  if [[ "${gateway_branch}" != "${GATEWAY_DEPLOY_BRANCH}" ]]; then
    echo "error: gateway deployment is allowed only from branch '${GATEWAY_DEPLOY_BRANCH}' (current: '${gateway_branch}')" >&2
    exit 1
  fi
  if [[ -n "$(git -C "${GATEWAY_DIR}" status --porcelain)" ]]; then
    echo "error: gateway working tree is dirty; commit or stash changes before deploying" >&2
    exit 1
  fi

  echo "==> [deploy] Verifying coupled llm-proxy/TAuth gateway contract"
  if ! timeout -k 180s -s SIGKILL 180s make -C "${GATEWAY_DIR}" "${GATEWAY_CONTRACT_TARGET}"; then
    echo "error: gateway checkout does not satisfy the coupled llm-proxy/TAuth deployment contract" >&2
    exit 1
  fi

  timeout -k 30s -s SIGKILL 30s git -C "${GATEWAY_DIR}" fetch "${GATEWAY_DEPLOY_REMOTE}" "${GATEWAY_DEPLOY_BRANCH}" --tags --prune
  gateway_head_sha="$(git -C "${GATEWAY_DIR}" rev-parse HEAD)"
  gateway_remote_branch_sha="$(git -C "${GATEWAY_DIR}" rev-parse "${GATEWAY_DEPLOY_REMOTE}/${GATEWAY_DEPLOY_BRANCH}")"
  if [[ "${gateway_head_sha}" != "${gateway_remote_branch_sha}" ]]; then
    echo "error: gateway ${GATEWAY_DEPLOY_BRANCH} is not at ${GATEWAY_DEPLOY_REMOTE}/${GATEWAY_DEPLOY_BRANCH}; push or pull first" >&2
    exit 1
  fi

  timeout -k 30s -s SIGKILL 30s git fetch "${DEPLOY_REMOTE}" "${DEPLOY_BRANCH}" --tags --prune

  current_branch="$(git rev-parse --abbrev-ref HEAD)"
  if [[ "${current_branch}" != "${DEPLOY_BRANCH}" ]]; then
    echo "error: deployment is allowed only from branch '${DEPLOY_BRANCH}' (current: '${current_branch}')" >&2
    exit 1
  fi

  if [[ -n "$(git status --porcelain)" ]]; then
    echo "error: working tree is dirty; commit or stash changes before deploying" >&2
    exit 1
  fi

  head_sha="$(git rev-parse HEAD)"
  remote_branch_sha="$(git rev-parse "${DEPLOY_REMOTE}/${DEPLOY_BRANCH}")"
  if [[ "${head_sha}" != "${remote_branch_sha}" ]]; then
    echo "error: local ${DEPLOY_BRANCH} is not at ${DEPLOY_REMOTE}/${DEPLOY_BRANCH}; push or pull first" >&2
    exit 1
  fi

  release_tag="$(resolve_release_tag)"
  [[ -n "${release_tag}" ]] || { echo "error: no v* release tag points at HEAD; run make release first or pass --tag" >&2; exit 1; }
  tag_sha="$(git rev-list -n 1 "${release_tag}" 2>/dev/null || true)"
  if [[ "${tag_sha}" != "${head_sha}" ]]; then
    echo "error: deploy tag ${release_tag} does not point at HEAD" >&2
    exit 1
  fi
else
  release_tag="$(resolve_release_tag)"
fi

if [[ -n "${release_tag}" ]]; then
  command -v python3 >/dev/null 2>&1 || { echo "error: python3 is required" >&2; exit 1; }
  if ! version_validation="$(python3 "${release_helper}" validate-version --version "${release_tag}" 2>&1)"; then
    printf '%s\n' "${version_validation}" >&2
    exit 1
  fi
fi

if [[ "${SKIP_CI}" != "true" && "${SKIP_GATEWAY}" != "true" ]]; then
  echo "==> [deploy] Running make ci before deployment (timeout ${CI_TIMEOUT_SECONDS}s)"
  timeout -k "${CI_TIMEOUT_SECONDS}s" -s SIGKILL "${CI_TIMEOUT_SECONDS}s" make ci
fi

if [[ "${SKIP_IMAGE_VERIFY}" != "true" && "${SKIP_GATEWAY}" != "true" ]]; then
  command -v docker >/dev/null 2>&1 || { echo "error: docker is required for image verification" >&2; exit 1; }
  docker buildx version >/dev/null 2>&1 || { echo "error: docker buildx is required for image verification" >&2; exit 1; }
  echo "==> [deploy] Verifying ${IMAGE_REPOSITORY}:latest matches ${release_tag}"
  release_digest="$(image_digest "${IMAGE_REPOSITORY}:${release_tag}")"
  latest_digest="$(image_digest "${IMAGE_REPOSITORY}:latest")"
  [[ -n "${release_digest}" ]] || { echo "error: could not resolve digest for ${IMAGE_REPOSITORY}:${release_tag}" >&2; exit 1; }
  [[ -n "${latest_digest}" ]] || { echo "error: could not resolve digest for ${IMAGE_REPOSITORY}:latest" >&2; exit 1; }
  if [[ "${release_digest}" != "${latest_digest}" ]]; then
    echo "error: ${IMAGE_REPOSITORY}:latest digest ${latest_digest} does not match ${release_tag} digest ${release_digest}; run make publish first" >&2
    exit 1
  fi
fi

if [[ "${SKIP_PAGES}" != "true" && "${SKIP_GATEWAY}" != "true" ]]; then
  [[ -n "${release_tag}" ]] || { echo "error: no release tag selected; run make publish first" >&2; exit 1; }
  echo "==> [deploy] Validating the published Pages artifact for ${release_tag}"
  PUBLISH_REMOTE="${DEPLOY_REMOTE}" PAGES_BRANCH="${PAGES_BRANCH}" PAGES_URL="${PAGES_URL}" PAGES_VERSION="${release_tag}" DEPLOY_PAGES_ARGS="--verify-only" make --no-print-directory pages-deploy
fi

if [[ "${SKIP_GATEWAY}" != "true" ]]; then
  echo "==> [deploy] Deploying llm-proxy through mprlab-gateway target ${GATEWAY_TARGET}"
  timeout --foreground -k 1200s -s SIGKILL 1200s make -C "${GATEWAY_DIR}" "${GATEWAY_TARGET}"
fi

if [[ "${SKIP_PAGES}" != "true" && "${SKIP_GATEWAY}" != "true" ]]; then
  [[ -n "${release_tag}" ]] || { echo "error: no release tag selected; run make publish first" >&2; exit 1; }
  echo "==> [deploy] Activating the published Pages artifact for ${release_tag}"
  PUBLISH_REMOTE="${DEPLOY_REMOTE}" PAGES_BRANCH="${PAGES_BRANCH}" PAGES_URL="${PAGES_URL}" PAGES_VERSION="${release_tag}" make --no-print-directory pages-deploy
fi

echo "llm-proxy deploy complete"
