#!/usr/bin/env bash
set -euo pipefail

usage() {
  builtin printf '%s\n' 'Usage:
  deploy_pages_artifact.sh --url <public-url> [options]

Downloads manifest.json and pages.tar.gz from a published GitHub Release,
verifies them against the remote tag, and replaces the configured Pages branch.

Options:
  --remote <name>       Git remote. Default: origin
  --branch <name>       Pages branch. Default: gh-pages
  --version <tag>       Published release tag. Default: exact v* tag at HEAD
  --url <url>           Public Pages URL used for post-deploy verification
  --skip-configure      Do not create/update the Pages branch source setting
  --skip-verify         Do not verify the public release marker
  --verify-only         Validate the published artifact without mutating Pages
  --help                Show this help text'
}

remote="origin"
branch="gh-pages"
version=""
url=""
configure="true"
verify="true"
verify_only="false"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --remote) [[ $# -ge 2 ]] || { echo "error: --remote requires a value" >&2; exit 1; }; remote="$2"; shift 2 ;;
    --branch) [[ $# -ge 2 ]] || { echo "error: --branch requires a value" >&2; exit 1; }; branch="$2"; shift 2 ;;
    --version) [[ $# -ge 2 ]] || { echo "error: --version requires a value" >&2; exit 1; }; version="$2"; shift 2 ;;
    --url) [[ $# -ge 2 ]] || { echo "error: --url requires a value" >&2; exit 1; }; url="$2"; shift 2 ;;
    --skip-configure) configure="false"; shift ;;
    --skip-verify) verify="false"; shift ;;
    --verify-only) verify_only="true"; shift ;;
    --help|-h) usage; exit 0 ;;
    *) echo "error: unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

[[ -n "${url}" || "${verify}" == "false" || "${verify_only}" == "true" ]] || { echo "error: --url is required unless --skip-verify or --verify-only is set" >&2; exit 1; }
required_commands=(git gh python3 tar)
if [[ "${verify}" == "true" && "${verify_only}" != "true" ]]; then required_commands+=(curl sleep); fi
for command_name in "${required_commands[@]}"; do command -v "${command_name}" >/dev/null 2>&1 || { echo "error: ${command_name} is required" >&2; exit 1; }; done
script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
pages_helper="${script_directory}/pages_artifact_helper.py"
[[ -f "${pages_helper}" ]] || { echo "error: Pages artifact helper is missing: ${pages_helper}" >&2; exit 1; }
release_helper="${script_directory}/release_helper.py"
[[ -f "${release_helper}" ]] || { echo "error: release helper is missing: ${release_helper}" >&2; exit 1; }

repo_root="$(git rev-parse --show-toplevel)"
temporary_directory="$(mktemp -d)"
trap 'rm -rf "${temporary_directory}"' EXIT
if ! remote_config_url="$(git config --get "remote.${remote}.url")"; then
  echo "error: selected Git remote does not exist: ${remote}" >&2
  exit 1
fi
remote_url="$(git remote get-url "${remote}")"
remote_push_url="$(git remote get-url --push "${remote}")"

github_identity_for_url() {
  local remote_url="$1"
  local hostname=""
  local repository_path=""
  local owner=""
  local name=""
  local extra=""
  local selector=""
  if [[ "${remote_url}" =~ ^[A-Za-z][A-Za-z0-9+.-]*://([^/@]+@)?([^/:]+)(:[0-9]+)?/(.+)$ ]]; then
    hostname="${BASH_REMATCH[2]}"
    repository_path="${BASH_REMATCH[4]}"
  elif [[ "${remote_url}" =~ ^([^@/]+@)?([^:/]+):(.+)$ ]]; then
    hostname="${BASH_REMATCH[2]}"
    repository_path="${BASH_REMATCH[3]}"
  fi
  [[ -n "${hostname}" ]] || return 1
  repository_path="${repository_path#/}"
  repository_path="${repository_path%/}"
  repository_path="${repository_path%.git}"
  IFS='/' read -r owner name extra <<<"${repository_path}"
  [[ -n "${owner}" && -n "${name}" && -z "${extra:-}" ]] || return 1
  selector="${owner}/${name}"
  if [[ "${hostname,,}" != "github.com" ]]; then selector="${hostname}/${selector}"; fi
  printf '%s|%s/%s|%s\n' "${selector}" "${owner}" "${name}" "${hostname,,}"
}

github_identity_for_selector() {
  local selector="$1"
  local hostname=""
  local owner=""
  local name=""
  local extra=""
  [[ "${selector}" =~ ^([A-Za-z0-9.-]+/)?[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || return 1
  IFS='/' read -r owner name extra <<<"${selector}"
  if [[ -n "${extra:-}" ]]; then hostname="${owner}"; owner="${name}"; name="${extra}"; fi
  printf '%s|%s/%s|%s\n' "${selector}" "${owner}" "${name}" "${hostname,,}"
}

require_matching_push_repository() {
  local push_url="$1"
  local equivalent_url="$2"
  local expected_repository="$3"
  local push_identity=""
  local push_repository=""
  if push_identity="$(github_identity_for_url "${push_url}")"; then
    IFS='|' read -r push_repository _ _ <<<"${push_identity}"
    [[ "${push_repository}" == "${expected_repository}" ]] || {
      echo "error: selected Git remote fetch and push URLs identify different GitHub repositories" >&2
      exit 1
    }
  elif [[ "${push_url}" != "${equivalent_url}" ]]; then
    echo "error: selected Git remote fetch and push URLs identify different GitHub repositories" >&2
    exit 1
  fi
}

if ! github_identity="$(github_identity_for_url "${remote_config_url}")"; then
  if ! github_identity="$(github_identity_for_selector "${GH_REPO:-}")"; then
    echo "error: selected Git remote cannot scope GitHub operations; set GH_REPO to [HOST/]OWNER/REPO" >&2
    exit 1
  fi
fi
IFS='|' read -r github_repository github_api_repository github_hostname <<<"${github_identity}"
require_matching_push_repository "${remote_push_url}" "${remote_url}" "${github_repository}"
github_repository_args=()
if [[ -n "${github_repository}" ]]; then github_repository_args=(--repo "${github_repository}"); fi
github_api_repository="${github_api_repository:-\{owner\}/\{repo\}}"
github_api_args=()
if [[ -n "${github_hostname}" ]]; then github_api_args=(--hostname "${github_hostname}"); fi
if [[ -z "${version}" ]]; then
  version="$(git tag --points-at HEAD --list 'v*' --sort=-version:refname | head -n 1)"
fi
[[ -n "${version}" ]] || { echo "error: no exact release tag at HEAD; pass --version after make publish" >&2; exit 1; }
python3 "${release_helper}" validate-version --version "${version}" >/dev/null
attempts="${PAGES_VERIFY_ATTEMPTS:-12}"
delay_seconds="${PAGES_VERIFY_DELAY_SECONDS:-5}"
build_attempts="${PAGES_BUILD_VERIFY_ATTEMPTS:-36}"
build_delay_seconds="${PAGES_BUILD_VERIFY_DELAY_SECONDS:-5}"
if [[ "${verify}" == "true" ]]; then
  [[ "${attempts}" =~ ^[1-9][0-9]*$ ]] || { echo "error: PAGES_VERIFY_ATTEMPTS must be a positive integer" >&2; exit 1; }
  [[ "${delay_seconds}" =~ ^[1-9][0-9]*$ ]] || { echo "error: PAGES_VERIFY_DELAY_SECONDS must be a positive integer" >&2; exit 1; }
  [[ "${build_attempts}" =~ ^[1-9][0-9]*$ ]] || { echo "error: PAGES_BUILD_VERIFY_ATTEMPTS must be a positive integer" >&2; exit 1; }
  [[ "${build_delay_seconds}" =~ ^[1-9][0-9]*$ ]] || { echo "error: PAGES_BUILD_VERIFY_DELAY_SECONDS must be a positive integer" >&2; exit 1; }
fi

download_directory="${temporary_directory}/download"
site_directory="${temporary_directory}/site"
checkout_directory="${temporary_directory}/checkout"
mkdir -p "${download_directory}" "${site_directory}"
gh release download "${version}" --pattern manifest.json --pattern pages.tar.gz --dir "${download_directory}" "${github_repository_args[@]}"
archive="${download_directory}/pages.tar.gz"
release_values_output="$(python3 "${pages_helper}" manifest-values --manifest "${download_directory}/manifest.json" --version "${version}")"
readarray -t release_values <<<"${release_values_output}"
[[ "${#release_values[@]}" -eq 3 ]] || { echo "error: Pages manifest validation returned incomplete values" >&2; exit 1; }
release_commit="${release_values[0]}"
source_commit="${release_values[1]}"
expected_sha256="${release_values[2]}"
remote_tag_commit="$(git ls-remote --tags "${remote}" "refs/tags/${version}^{}" | awk 'NR == 1 {print $1}')"
if [[ -z "${remote_tag_commit}" ]]; then
  remote_tag_commit="$(git ls-remote --tags "${remote}" "refs/tags/${version}" | awk 'NR == 1 {print $1}')"
fi
[[ "${remote_tag_commit}" == "${release_commit}" ]] || { echo "error: published release manifest does not match remote tag ${version}" >&2; exit 1; }
actual_sha256="$(shasum -a 256 "${archive}" | awk '{print $1}')"
[[ "${actual_sha256}" == "${expected_sha256}" ]] || { echo "error: published Pages asset does not match make release" >&2; exit 1; }
python3 "${pages_helper}" validate-archive --archive "${archive}"
tar -xzf "${archive}" -C "${site_directory}"
python3 "${pages_helper}" validate-marker \
  --marker "${site_directory}/.mprlab-release.json" \
  --source-commit "${source_commit}" \
  --version "${version}"

if [[ "${verify_only}" == "true" ]]; then
  echo "Verified published Pages artifact ${version} at source ${source_commit}."
  exit 0
fi

git clone --no-checkout "${remote_url}" "${checkout_directory}" >/dev/null
git -C "${checkout_directory}" remote set-url --push origin "${remote_push_url}"
checkout_push_url="$(git -C "${checkout_directory}" remote get-url --push origin)"
require_matching_push_repository "${checkout_push_url}" "${remote_push_url}" "${github_repository}"
if git -C "${checkout_directory}" show-ref --verify --quiet "refs/remotes/origin/${branch}"; then
  git -C "${checkout_directory}" checkout -B "${branch}" "origin/${branch}" >/dev/null
else
  git -C "${checkout_directory}" checkout --orphan "${branch}" >/dev/null
fi
find "${checkout_directory}" -mindepth 1 -maxdepth 1 ! -name .git -exec rm -rf {} +
cp -R "${site_directory}"/. "${checkout_directory}/"
git -C "${checkout_directory}" add -A
if ! git -C "${checkout_directory}" diff --cached --quiet; then
  git -C "${checkout_directory}" -c user.name="MPR Lab Pages Deployer" -c user.email="pages-deployer@mprlab.invalid" commit -m "Deploy Pages for ${version}" >/dev/null
  git -C "${checkout_directory}" push origin "HEAD:refs/heads/${branch}"
else
  echo "Pages branch already contains ${version}."
fi
pages_branch_commit="$(git -C "${checkout_directory}" rev-parse HEAD)"

if [[ "${configure}" == "true" ]]; then
  pages_site_path="${temporary_directory}/pages-site.json"
  if gh api "${github_api_args[@]}" "repos/${github_api_repository}/pages" >"${pages_site_path}"; then
    if ! python3 "${pages_helper}" pages-site-matches --site "${pages_site_path}" --branch "${branch}"; then
      gh api "${github_api_args[@]}" --method PUT "repos/${github_api_repository}/pages" -f build_type=legacy -f "source[branch]=${branch}" -f 'source[path]=/' -F https_enforced=true >/dev/null
    fi
  else
    gh api "${github_api_args[@]}" --method POST "repos/${github_api_repository}/pages" -f build_type=legacy -f "source[branch]=${branch}" -f 'source[path]=/' -F https_enforced=true >/dev/null
  fi
fi

if [[ "${verify}" == "true" ]]; then
  pages_builds_path="${temporary_directory}/pages-builds.json"
  for ((attempt = 1; attempt <= build_attempts; attempt += 1)); do
    if ! gh api "${github_api_args[@]}" "repos/${github_api_repository}/pages/builds?per_page=100" >"${pages_builds_path}"; then
      echo "error: could not read GitHub Pages build state for branch commit ${pages_branch_commit}" >&2
      exit 1
    fi
    if ! pages_build_state_output="$(python3 "${pages_helper}" pages-build-state --builds "${pages_builds_path}" --commit "${pages_branch_commit}")"; then
      echo "error: GitHub Pages returned an invalid build-state response for branch commit ${pages_branch_commit}" >&2
      exit 1
    fi
    readarray -t pages_build_state_values <<<"${pages_build_state_output}"
    [[ "${#pages_build_state_values[@]}" -ge 1 ]] || { echo "error: GitHub Pages build-state response is empty for branch commit ${pages_branch_commit}" >&2; exit 1; }
    case "${pages_build_state_values[0]}" in
      built)
        break
        ;;
      waiting)
        if (( attempt < build_attempts )); then
          echo "==> [pages] Waiting for GitHub Pages build ${pages_branch_commit} (attempt ${attempt}/${build_attempts})."
          sleep "${build_delay_seconds}"
          continue
        fi
        echo "error: GitHub Pages build did not reach built state for branch commit ${pages_branch_commit} after ${build_attempts} attempts" >&2
        exit 1
        ;;
      errored)
        pages_build_error="${pages_build_state_values[1]:-no error message reported}"
        echo "error: GitHub Pages build failed for branch commit ${pages_branch_commit}: ${pages_build_error}" >&2
        exit 1
        ;;
      *)
        echo "error: GitHub Pages build-state response has unknown state '${pages_build_state_values[0]}' for branch commit ${pages_branch_commit}" >&2
        exit 1
        ;;
    esac
  done

  marker_url="${url%/}/.mprlab-release.json?source_commit=${source_commit}"
  public_marker_path="${temporary_directory}/public-marker.json"
  for ((attempt = 1; attempt <= attempts; attempt += 1)); do
    marker="$(curl --fail --silent --show-error "${marker_url}" || true)"
    printf '%s' "${marker}" >"${public_marker_path}"
    if python3 "${pages_helper}" validate-public-marker \
      --marker "${public_marker_path}" \
      --source-commit "${source_commit}" \
      --version "${version}" >/dev/null 2>&1
    then
      echo "Verified ${url} at source ${source_commit}."
      exit 0
    fi
    if (( attempt < attempts )); then
      echo "==> [pages] Waiting for public marker at source ${source_commit} (attempt ${attempt}/${attempts})."
      sleep "${delay_seconds}"
    fi
  done
  echo "error: Pages marker did not reach source ${source_commit}: ${marker_url}" >&2
  exit 1
fi

echo "Deployed Pages release ${version}."
