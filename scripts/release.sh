#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"

if [[ -v RELEASE_PIPELINE ]] && [[ -n "${RELEASE_PIPELINE}" ]]; then
  pipeline="${RELEASE_PIPELINE}"
else
  pipeline="${repo_root}/../agentSkills/gitrelease/scripts/prepare_release.sh"
fi
[[ -x "${pipeline}" ]] || {
  echo "error: local release pipeline not found; set RELEASE_PIPELINE=/path/to/prepare_release.sh" >&2
  exit 1
}

exec "${pipeline}" "$@"

