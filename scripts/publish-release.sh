#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"

if [[ -v PUBLISH_RELEASE_PIPELINE ]] && [[ -n "${PUBLISH_RELEASE_PIPELINE}" ]]; then
  pipeline="${PUBLISH_RELEASE_PIPELINE}"
else
  pipeline="${repo_root}/../agentSkills/gitrelease/scripts/publish_release.sh"
fi
[[ -x "${pipeline}" ]] || {
  echo "error: prepared-release publish pipeline not found; set PUBLISH_RELEASE_PIPELINE=/path/to/publish_release.sh" >&2
  exit 1
}

exec "${pipeline}" "$@"

