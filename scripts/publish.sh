#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  scripts/publish.sh [options]

Publishes the llm-proxy Docker image from the release branch by:
  1. validating the clean local branch matches its remote
  2. validating a pushed v* release tag points at HEAD
  3. running make ci
  4. building and pushing the multi-arch Docker image

Options:
  --image <value>       Full image name without tag. Default: $DOCKER_IMAGE or ghcr.io/tyemirov/llm-proxy
  --platforms <value>   Docker platforms. Default: $PUBLISH_PLATFORMS or linux/amd64,linux/arm64
  --tag <value>         Release tag to publish. Default: v* tag pointing at HEAD
  --remote <value>      Git remote to validate from. Default: $PUBLISH_REMOTE or origin
  --branch <value>      Release branch to publish from. Default: $PUBLISH_BRANCH or master
  --no-latest           Do not push :latest
  --dry-run             Run source checks and make ci without pushing images
  --skip-checks         Skip the local make ci gate
  --username <value>    Registry username. Default: gh auth user login
  --token <value>       Registry token/password. Default: $GHCR_TOKEN or $GITHUB_TOKEN or $GH_TOKEN or gh auth token
  --help                Show this help text
USAGE
}

IMAGE="${DOCKER_IMAGE:-ghcr.io/tyemirov/llm-proxy}"
PLATFORMS="${PUBLISH_PLATFORMS:-linux/amd64,linux/arm64}"
TAG="${PUBLISH_TAG:-}"
PUSH_LATEST="true"
DRY_RUN="${PUBLISH_DRY_RUN:-false}"
SKIP_CHECKS="false"
USERNAME="${GHCR_USERNAME:-}"
TOKEN="${GHCR_TOKEN:-${GITHUB_TOKEN:-${GH_TOKEN:-}}}"
PUBLISH_BRANCH="${PUBLISH_BRANCH:-master}"
PUBLISH_REMOTE="${PUBLISH_REMOTE:-origin}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --image)
      [[ $# -ge 2 ]] || { echo "error: --image requires a value" >&2; exit 1; }
      IMAGE="$2"
      shift 2
      ;;
    --platforms)
      [[ $# -ge 2 ]] || { echo "error: --platforms requires a value" >&2; exit 1; }
      PLATFORMS="$2"
      shift 2
      ;;
    --tag)
      [[ $# -ge 2 ]] || { echo "error: --tag requires a value" >&2; exit 1; }
      TAG="$2"
      shift 2
      ;;
    --remote)
      [[ $# -ge 2 ]] || { echo "error: --remote requires a value" >&2; exit 1; }
      PUBLISH_REMOTE="$2"
      shift 2
      ;;
    --branch)
      [[ $# -ge 2 ]] || { echo "error: --branch requires a value" >&2; exit 1; }
      PUBLISH_BRANCH="$2"
      shift 2
      ;;
    --no-latest)
      PUSH_LATEST="false"
      shift
      ;;
    --dry-run)
      DRY_RUN="true"
      shift
      ;;
    --skip-checks)
      SKIP_CHECKS="true"
      shift
      ;;
    --username)
      [[ $# -ge 2 ]] || { echo "error: --username requires a value" >&2; exit 1; }
      USERNAME="$2"
      shift 2
      ;;
    --token)
      [[ $# -ge 2 ]] || { echo "error: --token requires a value" >&2; exit 1; }
      TOKEN="$2"
      shift 2
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

command -v docker >/dev/null 2>&1 || { echo "error: docker is required" >&2; exit 1; }
command -v gh >/dev/null 2>&1 || { echo "error: gh is required" >&2; exit 1; }

timeout -k 30s -s SIGKILL 30s git fetch "${PUBLISH_REMOTE}" "${PUBLISH_BRANCH}" --tags --prune

current_branch="$(git rev-parse --abbrev-ref HEAD)"
if [[ "${current_branch}" != "${PUBLISH_BRANCH}" ]]; then
  echo "error: publishing is allowed only from branch '${PUBLISH_BRANCH}' (current: '${current_branch}')" >&2
  exit 1
fi

if [[ -n "$(git status --porcelain)" ]]; then
  echo "error: working tree is dirty; commit or stash changes before publishing" >&2
  exit 1
fi

head_sha="$(git rev-parse HEAD)"
remote_branch_sha="$(git rev-parse "${PUBLISH_REMOTE}/${PUBLISH_BRANCH}")"
if [[ "${head_sha}" != "${remote_branch_sha}" ]]; then
  echo "error: local ${PUBLISH_BRANCH} is not at ${PUBLISH_REMOTE}/${PUBLISH_BRANCH}; push or pull first" >&2
  exit 1
fi

if [[ -z "${TAG}" ]]; then
  TAG="$(git tag --points-at HEAD --list 'v*' --sort=-version:refname | head -n 1)"
fi
[[ -n "${TAG}" ]] || { echo "error: no v* release tag points at HEAD; run make release first or pass --tag" >&2; exit 1; }

tag_sha="$(git rev-list -n 1 "${TAG}" 2>/dev/null || true)"
if [[ "${tag_sha}" != "${head_sha}" ]]; then
  echo "error: release tag ${TAG} does not point at HEAD" >&2
  exit 1
fi

remote_tag_refs="$(git ls-remote --tags "${PUBLISH_REMOTE}" "refs/tags/${TAG}" "refs/tags/${TAG}^{}")"
remote_tag_sha="$(awk '$2 ~ /\^\{\}$/ { peeled = $1 } $2 !~ /\^\{\}$/ { direct = $1 } END { if (peeled != "") print peeled; else print direct }' <<<"${remote_tag_refs}")"
if [[ "${remote_tag_sha}" != "${head_sha}" ]]; then
  echo "error: release tag ${TAG} is not pushed to ${PUBLISH_REMOTE} at HEAD" >&2
  exit 1
fi

if [[ "${SKIP_CHECKS}" != "true" ]]; then
  echo "==> [publish] Running make ci before publishing"
  timeout -k 350s -s SIGKILL 350s make ci
fi

registry_host="$(printf '%s' "${IMAGE}" | cut -d'/' -f1)"
[[ -n "${registry_host}" ]] || { echo "error: unable to determine registry host from image: ${IMAGE}" >&2; exit 1; }

if [[ "${DRY_RUN}" == "true" || "${DRY_RUN}" == "1" ]]; then
  echo "publish_dry_run=true"
  echo "release_branch=${PUBLISH_BRANCH}"
  echo "release_tag=${TAG}"
  echo "image=${IMAGE}:${TAG}"
  if [[ "${PUSH_LATEST}" == "true" ]]; then
    echo "image=${IMAGE}:latest"
  fi
  exit 0
fi

if [[ -z "${TOKEN}" ]]; then
  TOKEN="$(gh auth token 2>/dev/null || true)"
fi
[[ -n "${TOKEN}" ]] || { echo "error: registry token is required (use --token or GHCR_TOKEN/GITHUB_TOKEN/GH_TOKEN)" >&2; exit 1; }

if [[ -z "${USERNAME}" ]]; then
  USERNAME="$(gh api user --jq '.login')"
fi
[[ -n "${USERNAME}" ]] || { echo "error: could not infer registry username; use --username or GHCR_USERNAME" >&2; exit 1; }

echo "==> [publish] Logging in to ${registry_host}"
echo "${TOKEN}" | timeout -k 30s -s SIGKILL 30s docker login "${registry_host}" -u "${USERNAME}" --password-stdin

build_args=(
  timeout -k 1200s -s SIGKILL 1200s
  docker buildx build
  --pull
  --platform "${PLATFORMS}"
  --push
  -f Dockerfile
  -t "${IMAGE}:${TAG}"
)

if [[ "${PUSH_LATEST}" == "true" ]]; then
  build_args+=(-t "${IMAGE}:latest")
fi

build_args+=(.)
"${build_args[@]}"

echo "Published image: ${IMAGE}:${TAG}"
if [[ "${PUSH_LATEST}" == "true" ]]; then
  echo "Published image: ${IMAGE}:latest"
fi
