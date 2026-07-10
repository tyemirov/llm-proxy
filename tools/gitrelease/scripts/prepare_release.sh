#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  prepare_release.sh [options]

Prepares a release entirely from local repository state. The command validates
the worktree, runs make ci, creates the changelog commit and annotated tag, and
writes the release manifest and notes under .git/mprlab-release.

It never fetches, pushes, calls GitHub, publishes an image/store build, updates
GitHub Pages, or deploys production.

Options:
  --bump <patch|minor|major>  SemVer bump when no exact version is supplied. Default: patch
  --version <value>           Exact local release tag/version to prepare
  --dry-run                   Validate and report the selected version without changing files
  --help                      Show this help text
USAGE
}

if [[ -v RELEASE_HELPER ]]; then
  helper="${RELEASE_HELPER}"
else
  helper=""
fi
if [[ -v RELEASE_BUMP ]] && [[ -n "${RELEASE_BUMP}" ]]; then
  bump="${RELEASE_BUMP}"
else
  bump="patch"
fi
if [[ -v RELEASE_VERSION ]]; then
  version="${RELEASE_VERSION}"
else
  version=""
fi
if [[ -v RELEASE_CI_TIMEOUT_SECONDS ]] && [[ -n "${RELEASE_CI_TIMEOUT_SECONDS}" ]]; then
  ci_timeout="${RELEASE_CI_TIMEOUT_SECONDS}"
elif [[ -v LLM_PROXY_CI_TIMEOUT_SECONDS ]] && [[ -n "${LLM_PROXY_CI_TIMEOUT_SECONDS}" ]]; then
  ci_timeout="${LLM_PROXY_CI_TIMEOUT_SECONDS}"
else
  ci_timeout="350"
fi
dry_run="false"
if [[ -v RELEASE_ARTIFACT_TARGETS ]]; then
  artifact_targets="${RELEASE_ARTIFACT_TARGETS}"
else
  artifact_targets=""
fi

while [[ $# -gt 0 ]]; do
  case "$1" in
    --bump)
      [[ $# -ge 2 ]] || { echo "error: --bump requires a value" >&2; exit 1; }
      bump="$2"
      shift 2
      ;;
    --version)
      [[ $# -ge 2 ]] || { echo "error: --version requires a value" >&2; exit 1; }
      version="$2"
      shift 2
      ;;
    --dry-run)
      dry_run="true"
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

case "${bump}" in
  patch|minor|major) ;;
  *) echo "error: --bump must be patch, minor, or major" >&2; exit 1 ;;
esac
[[ "${ci_timeout}" =~ ^[1-9][0-9]*$ ]] || { echo "error: RELEASE_CI_TIMEOUT_SECONDS must be a positive integer" >&2; exit 1; }

command -v git >/dev/null 2>&1 || { echo "error: git is required" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo "error: python3 is required" >&2; exit 1; }

repo_root="$(git rev-parse --show-toplevel)"
cd "${repo_root}"

if [[ -z "${helper}" ]]; then
  helper="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/release_helper.py"
fi
[[ -x "${helper}" ]] || { echo "error: release helper is not executable: ${helper}" >&2; exit 1; }

json_value() {
  python3 - "$1" "$2" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    value = json.load(handle)
for part in sys.argv[2].split("."):
    value = value.get(part) if isinstance(value, dict) else None
print("" if value is None else value)
PY
}

select_release() {
  python3 - "$1" "${version}" "${bump}" <<'PY'
import json
import re
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    data = json.load(handle)
explicit_version, bump = sys.argv[2], sys.argv[3]
info = data.get("version_info") or {}

semver_pattern = re.compile(
    r"^v(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)"
    r"(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?$"
)
def validate_selected_version(value):
    if re.fullmatch(r"[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}", value) is None:
        raise SystemExit("release version must satisfy Git and container tag constraints")
    if semver_pattern.fullmatch(value) is None:
        raise SystemExit("release version must use canonical vMAJOR.MINOR.PATCH SemVer")

def semver_bump(latest):
    if not latest:
        return "v1.0.0"
    match = re.match(r"^(v?)(\d+)\.(\d+)\.(\d+)(?:[-+].*)?$", latest)
    if not match:
        raise SystemExit(f"latest SemVer tag is invalid: {latest}")
    prefix, major, minor, patch = match.groups()
    major, minor, patch = int(major), int(minor), int(patch)
    if bump == "major":
        major, minor, patch = major + 1, 0, 0
    elif bump == "minor":
        minor, patch = minor + 1, 0
    else:
        patch += 1
    return f"{prefix or 'v'}{major}.{minor}.{patch}"

if explicit_version:
    selected = explicit_version
else:
    selected = semver_bump(info.get("latest_semver_tag") or "")
boundary = info.get("latest_semver_tag") or ""

if not selected:
    raise SystemExit("release version selection returned an empty version")
validate_selected_version(selected)
print(selected)
print(boundary)
print("semver")
PY
}

preflight_json="$(mktemp)"
notes_file="$(mktemp)"
cleanup() {
  rm -f "${preflight_json}" "${notes_file}"
}
trap cleanup EXIT

release_timestamp="$(date +%Y-%m-%dT%H:%M:%S%z)"
release_date="${release_timestamp%%T*}"

run_local_preflight() {
  if ! "${helper}" preflight --local --release-timestamp "${release_timestamp}" >"${preflight_json}"; then
    cat "${preflight_json}"
    echo "error: local release preflight failed" >&2
    exit 1
  fi
  cat "${preflight_json}"
}

echo "==> [release] Checking local release state"
run_local_preflight
default_branch="$(json_value "${preflight_json}" "default_branch")"
source_commit="$(git rev-parse HEAD)"
selection="$(select_release "${preflight_json}")"
next_version="$(sed -n '1p' <<<"${selection}")"
boundary_tag="$(sed -n '2p' <<<"${selection}")"
effective_scheme="$(sed -n '3p' <<<"${selection}")"

if [[ "${dry_run}" == "true" ]]; then
  echo "release_dry_run=true"
  echo "release_scope=local"
  echo "default_branch=${default_branch}"
  echo "version_scheme=${effective_scheme}"
  echo "next_version=${next_version}"
  echo "changelog_boundary=${boundary_tag:-<none>}"
  exit 0
fi

echo "==> [release] Running make ci"
timeout -k "${ci_timeout}s" -s SIGKILL "${ci_timeout}s" make ci

echo "==> [release] Rechecking local state after CI"
run_local_preflight
[[ "$(git rev-parse HEAD)" == "${source_commit}" ]] || { echo "error: HEAD changed while make ci was running" >&2; exit 1; }
selection="$(select_release "${preflight_json}")"
next_version="$(sed -n '1p' <<<"${selection}")"
boundary_tag="$(sed -n '2p' <<<"${selection}")"
effective_scheme="$(sed -n '3p' <<<"${selection}")"

"${helper}" initialize-release-artifact \
  --version "${next_version}" \
  --source-commit "${source_commit}" \
  --release-timestamp "${release_timestamp}"
artifact_dir="$(git rev-parse --git-path mprlab-release)"
if [[ "${artifact_dir}" != /* ]]; then
  artifact_dir="${repo_root}/${artifact_dir}"
fi

if [[ -n "${artifact_targets}" ]]; then
  read -r -a artifact_target_list <<<"${artifact_targets}"
  echo "==> [release] Preparing local artifacts: ${artifact_targets}"
    RELEASE_VERSION="${next_version}" \
    RELEASE_TIMESTAMP="${release_timestamp}" \
    MOBILE_RELEASE_TIMESTAMP="${release_timestamp}" \
    RELEASE_ARTIFACT_DIR="${artifact_dir}" \
    make --no-print-directory "${artifact_target_list[@]}"
  echo "==> [release] Rechecking local state after artifact preparation"
  run_local_preflight
  [[ "$(git rev-parse HEAD)" == "${source_commit}" ]] || { echo "error: HEAD changed while preparing release artifacts" >&2; exit 1; }
fi

echo "==> [release] Preparing ${next_version} from local Git history"
notes_args=(generate-notes --version "${next_version}" --release-date "${release_date}")
if [[ -n "${boundary_tag}" ]]; then
  notes_args+=(--since-tag "${boundary_tag}")
fi
"${helper}" "${notes_args[@]}" | tee "${notes_file}"
"${helper}" insert-changelog --notes-file "${notes_file}"

git add CHANGELOG.md
if git diff --cached --quiet -- CHANGELOG.md; then
  echo "error: CHANGELOG.md has no staged release changes" >&2
  exit 1
fi
staged_files="$(git diff --cached --name-only)"
if [[ "${staged_files}" != "CHANGELOG.md" ]]; then
  echo "error: release commit may contain only CHANGELOG.md" >&2
  printf '%s\n' "${staged_files}" >&2
  exit 1
fi

git commit -m "Release ${next_version}"
release_commit="$(git rev-parse HEAD)"
git tag -a "${next_version}" -m "Release ${next_version}" "${release_commit}"
"${helper}" write-release-artifact \
  --version "${next_version}" \
  --source-commit "${source_commit}" \
  --release-commit "${release_commit}" \
  --notes-file "${notes_file}" \
  --default-branch "${default_branch}" \
  --release-timestamp "${release_timestamp}"

echo "Prepared ${next_version} at ${release_commit}. Run make publish to publish it."
