#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  scripts/release.sh [options]

Cuts an llm-proxy release from master:
  1. validates the clean local branch matches origin/master
  2. selects the next v* SemVer tag
  3. runs make ci
  4. inserts a CHANGELOG.md section when missing
  5. commits the changelog when changed
  6. creates and pushes an annotated tag

The tag push triggers the GitHub release workflow, which builds and publishes
release artifacts.

Options:
  --bump <patch|minor|major>  SemVer bump when RELEASE_VERSION is not set. Default: patch
  --version <value>           Exact release tag/version to use, e.g. v1.2.3
  --dry-run                   Report the selected version without changing files
  --skip-ci                   Skip the local make ci release gate
  --remote <value>            Git remote. Default: $RELEASE_REMOTE or origin
  --branch <value>            Release branch. Default: $RELEASE_BRANCH or master
  --help                      Show this help text
USAGE
}

BUMP="${RELEASE_BUMP:-patch}"
VERSION="${RELEASE_VERSION:-}"
DRY_RUN="false"
SKIP_CI="false"
RELEASE_REMOTE="${RELEASE_REMOTE:-origin}"
RELEASE_BRANCH="${RELEASE_BRANCH:-master}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --bump)
      [[ $# -ge 2 ]] || { echo "error: --bump requires a value" >&2; exit 1; }
      BUMP="$2"
      shift 2
      ;;
    --version)
      [[ $# -ge 2 ]] || { echo "error: --version requires a value" >&2; exit 1; }
      VERSION="$2"
      shift 2
      ;;
    --dry-run)
      DRY_RUN="true"
      shift
      ;;
    --skip-ci)
      SKIP_CI="true"
      shift
      ;;
    --remote)
      [[ $# -ge 2 ]] || { echo "error: --remote requires a value" >&2; exit 1; }
      RELEASE_REMOTE="$2"
      shift 2
      ;;
    --branch)
      [[ $# -ge 2 ]] || { echo "error: --branch requires a value" >&2; exit 1; }
      RELEASE_BRANCH="$2"
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

case "${BUMP}" in
  patch|minor|major) ;;
  *) echo "error: --bump must be patch, minor, or major" >&2; exit 1 ;;
esac

command -v git >/dev/null 2>&1 || { echo "error: git is required" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo "error: python3 is required" >&2; exit 1; }

repo_root="$(git rev-parse --show-toplevel)"
cd "${repo_root}"

timeout -k 30s -s SIGKILL 30s git fetch "${RELEASE_REMOTE}" "${RELEASE_BRANCH}" --tags --prune

current_branch="$(git rev-parse --abbrev-ref HEAD)"
if [[ "${current_branch}" != "${RELEASE_BRANCH}" ]]; then
  echo "error: release is allowed only from branch '${RELEASE_BRANCH}' (current: '${current_branch}')" >&2
  exit 1
fi

if [[ -n "$(git status --porcelain)" ]]; then
  echo "error: working tree is dirty; commit or stash changes before releasing" >&2
  exit 1
fi

head_sha="$(git rev-parse HEAD)"
remote_branch_sha="$(git rev-parse "${RELEASE_REMOTE}/${RELEASE_BRANCH}")"
if [[ "${head_sha}" != "${remote_branch_sha}" ]]; then
  echo "error: local ${RELEASE_BRANCH} is not at ${RELEASE_REMOTE}/${RELEASE_BRANCH}; push or pull first" >&2
  exit 1
fi

latest_tag="$(git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-version:refname | head -n 1)"
if [[ -n "${VERSION}" ]]; then
  next_version="${VERSION}"
else
  next_version="$(python3 - "${latest_tag}" "${BUMP}" <<'PY'
import re
import sys

latest_tag = sys.argv[1].strip()
bump = sys.argv[2]
if not latest_tag:
    print("v1.0.0")
    raise SystemExit

match = re.match(r"^v(\d+)\.(\d+)\.(\d+)$", latest_tag)
if not match:
    raise SystemExit(f"latest SemVer tag is invalid: {latest_tag}")

major, minor, patch = (int(part) for part in match.groups())
if bump == "major":
    major, minor, patch = major + 1, 0, 0
elif bump == "minor":
    minor, patch = minor + 1, 0
else:
    patch += 1
print(f"v{major}.{minor}.{patch}")
PY
)"
fi

if [[ ! "${next_version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "error: release version must look like vX.Y.Z (got: ${next_version})" >&2
  exit 1
fi

if git rev-parse -q --verify "refs/tags/${next_version}" >/dev/null; then
  echo "error: tag already exists: ${next_version}" >&2
  exit 1
fi

if [[ "${DRY_RUN}" == "true" ]]; then
  echo "release_dry_run=true"
  echo "release_branch=${RELEASE_BRANCH}"
  echo "latest_tag=${latest_tag:-<none>}"
  echo "next_version=${next_version}"
  exit 0
fi

if [[ "${SKIP_CI}" != "true" ]]; then
  echo "==> [release] Running make ci"
  timeout -k 350s -s SIGKILL 350s make ci
fi

release_date="$(date +%Y-%m-%d)"
if ! grep -Eq "^## \\[${next_version}\\]" CHANGELOG.md; then
  notes_file="$(mktemp)"
  changelog_file="$(mktemp)"
  cleanup() {
    rm -f "${notes_file}" "${changelog_file}"
  }
  trap cleanup EXIT

  if [[ -n "${latest_tag}" ]]; then
    git log --format='- %s' "${latest_tag}..HEAD" > "${notes_file}"
  else
    git log --format='- %s' > "${notes_file}"
  fi
  if [[ ! -s "${notes_file}" ]]; then
    printf '%s\n' "- Maintenance release." > "${notes_file}"
  fi

  python3 - CHANGELOG.md "${changelog_file}" "${next_version}" "${release_date}" "${notes_file}" <<'PY'
import pathlib
import sys

source = pathlib.Path(sys.argv[1])
target = pathlib.Path(sys.argv[2])
version = sys.argv[3]
release_date = sys.argv[4]
notes = pathlib.Path(sys.argv[5]).read_text(encoding="utf-8").strip()
text = source.read_text(encoding="utf-8")
section = f"## [{version}] - {release_date}\n\n### Changes\n{notes}\n\n"
marker = "\n## ["
index = text.find(marker)
if index == -1:
    updated = text.rstrip() + "\n\n" + section
else:
    updated = text[: index + 1] + section + text[index + 1 :]
target.write_text(updated, encoding="utf-8")
PY
  mv "${changelog_file}" CHANGELOG.md
fi

git add CHANGELOG.md
if git diff --cached --quiet -- CHANGELOG.md; then
  echo "==> [release] CHANGELOG.md already contains ${next_version}"
else
  git commit -m "Release ${next_version}"
fi

git tag -a "${next_version}" -m "Release ${next_version}"
git push "${RELEASE_REMOTE}" "${RELEASE_BRANCH}"
git push "${RELEASE_REMOTE}" "${next_version}"

echo "Released ${next_version}"
