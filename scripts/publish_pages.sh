#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  scripts/publish_pages.sh [options]

Renders and publishes the GitHub Pages static frontend without GitHub Actions.
The published artifact contains only static shell files; browser configuration is
loaded at runtime from the llm-proxy backend /config-ui.yaml endpoint.

Options:
  --remote <value>       Git remote to publish to. Default: $PAGES_REMOTE or origin
  --branch <value>       Pages branch to push. Default: $PAGES_BRANCH or gh-pages
  --site-source <path>   Static site source directory. Default: $PAGES_SITE_SOURCE or site
  --domain <value>       Pages custom domain. Default: $PAGES_DOMAIN or llm-proxy.mprlab.com
  --skip-configure       Do not configure the repository Pages source through the GitHub API
  --dry-run              Render and validate without pushing or configuring Pages
  --help                 Show this help text
USAGE
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

repo_slug_from_remote_url() {
  local remote_url="$1"
  local slug=""
  case "${remote_url}" in
    git@github.com:*)
      slug="${remote_url#git@github.com:}"
      ;;
    https://github.com/*)
      slug="${remote_url#https://github.com/}"
      ;;
    ssh://git@github.com/*)
      slug="${remote_url#ssh://git@github.com/}"
      ;;
    *)
      echo "error: GitHub remote URL is required for Pages configuration: ${remote_url}" >&2
      exit 1
      ;;
  esac
  slug="${slug%.git}"
  if [[ ! "${slug}" =~ ^[^/]+/[^/]+$ ]]; then
    echo "error: could not derive owner/repo from remote URL: ${remote_url}" >&2
    exit 1
  fi
  printf "%s\n" "${slug}"
}

configure_pages_source() {
  local repo_slug="$1"
  local pages_branch="$2"
  local pages_domain="$3"
  local api_arguments=(
    -f "build_type=legacy"
    -f "source[branch]=${pages_branch}"
    -f "source[path]=/"
    -f "cname=${pages_domain}"
    -F "https_enforced=true"
  )

  if gh api --method PUT "/repos/${repo_slug}/pages" "${api_arguments[@]}" >/dev/null; then
    return
  fi
  gh api --method POST "/repos/${repo_slug}/pages" "${api_arguments[@]}" >/dev/null
}

PAGES_REMOTE="$(env_or_default PAGES_REMOTE origin)"
PAGES_BRANCH="$(env_or_default PAGES_BRANCH gh-pages)"
PAGES_SITE_SOURCE="$(env_or_default PAGES_SITE_SOURCE site)"
PAGES_DOMAIN="$(env_or_default PAGES_DOMAIN llm-proxy.mprlab.com)"
DRY_RUN="$(env_or_default PAGES_DRY_RUN false)"
CONFIGURE_PAGES="$(env_or_default PAGES_CONFIGURE_SOURCE true)"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --remote)
      [[ $# -ge 2 ]] || { echo "error: --remote requires a value" >&2; exit 1; }
      PAGES_REMOTE="$2"
      shift 2
      ;;
    --branch)
      [[ $# -ge 2 ]] || { echo "error: --branch requires a value" >&2; exit 1; }
      PAGES_BRANCH="$2"
      shift 2
      ;;
    --site-source)
      [[ $# -ge 2 ]] || { echo "error: --site-source requires a value" >&2; exit 1; }
      PAGES_SITE_SOURCE="$2"
      shift 2
      ;;
    --domain)
      [[ $# -ge 2 ]] || { echo "error: --domain requires a value" >&2; exit 1; }
      PAGES_DOMAIN="$2"
      shift 2
      ;;
    --skip-configure)
      CONFIGURE_PAGES="false"
      shift
      ;;
    --dry-run)
      DRY_RUN="true"
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
if [[ "${CONFIGURE_PAGES}" == "true" && "${DRY_RUN}" != "true" && "${DRY_RUN}" != "1" ]]; then
  command -v gh >/dev/null 2>&1 || { echo "error: gh is required to configure GitHub Pages" >&2; exit 1; }
fi

repo_root="$(git rev-parse --show-toplevel)"
cd "${repo_root}"

remote_url="$(git remote get-url "${PAGES_REMOTE}")"
render_directory="$(mktemp -d)"
publish_directory="$(mktemp -d)"
cleanup() {
  rm -rf "${render_directory}" "${publish_directory}"
}
trap cleanup EXIT

echo "==> [pages] Rendering static Pages shell"
go run ./cmd/cli --site-source "${PAGES_SITE_SOURCE}" --render-site-output "${render_directory}/site"

for forbidden_file in config-ui.yaml llm-proxy-config.json; do
  if find "${render_directory}/site" -name "${forbidden_file}" -print -quit | grep -q .; then
    echo "error: rendered Pages artifact contains forbidden static config file: ${forbidden_file}" >&2
    exit 1
  fi
done

if grep -R -n -e "__LLM_PROXY_CONFIG_URL__" "${render_directory}/site" >/dev/null; then
  echo "error: rendered Pages artifact contains the retired config URL placeholder" >&2
  exit 1
fi

if grep -n -e "data-config-url" "${render_directory}/site/index.html" >/dev/null; then
  echo "error: rendered Pages index.html contains a static config URL attribute" >&2
  exit 1
fi

if [[ "${DRY_RUN}" == "true" || "${DRY_RUN}" == "1" ]]; then
  echo "pages_publish_dry_run=true"
  echo "pages_branch=${PAGES_BRANCH}"
  echo "pages_domain=${PAGES_DOMAIN}"
  echo "pages_source=${PAGES_SITE_SOURCE}"
  exit 0
fi

git -C "${publish_directory}" init >/dev/null
git -C "${publish_directory}" remote add "${PAGES_REMOTE}" "${remote_url}"

if git -C "${publish_directory}" fetch --depth=1 "${PAGES_REMOTE}" "refs/heads/${PAGES_BRANCH}:refs/remotes/${PAGES_REMOTE}/${PAGES_BRANCH}" >/dev/null 2>&1; then
  git -C "${publish_directory}" checkout -B "${PAGES_BRANCH}" "${PAGES_REMOTE}/${PAGES_BRANCH}" >/dev/null
else
  git -C "${publish_directory}" checkout --orphan "${PAGES_BRANCH}" >/dev/null
fi

git_user_name="$(git config user.name || true)"
git_user_email="$(git config user.email || true)"
git -C "${publish_directory}" config user.name "${git_user_name:-llm-proxy pages publisher}"
git -C "${publish_directory}" config user.email "${git_user_email:-pages-publisher@example.invalid}"

find "${publish_directory}" -mindepth 1 -maxdepth 1 ! -name .git -exec rm -rf {} +
cp -R "${render_directory}/site"/. "${publish_directory}/"
git -C "${publish_directory}" add -A

if git -C "${publish_directory}" diff --cached --quiet; then
  echo "==> [pages] ${PAGES_BRANCH} already matches the rendered site"
else
  source_sha="$(git rev-parse --short HEAD)"
  git -C "${publish_directory}" commit -m "Publish llm-proxy Pages for ${source_sha}" >/dev/null
  git -C "${publish_directory}" push "${PAGES_REMOTE}" "HEAD:${PAGES_BRANCH}"
  echo "Published Pages branch: ${PAGES_BRANCH}"
fi

if [[ "${CONFIGURE_PAGES}" == "true" ]]; then
  repo_slug="$(repo_slug_from_remote_url "${remote_url}")"
  echo "==> [pages] Configuring GitHub Pages branch source for ${repo_slug}"
  configure_pages_source "${repo_slug}" "${PAGES_BRANCH}" "${PAGES_DOMAIN}"
fi

echo "llm-proxy Pages publish complete"
