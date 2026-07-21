#!/usr/bin/env bash
set -euo pipefail

usage() {
  builtin printf '%s\n' 'Usage:
  resolve_container_manifest_digest.sh <image-reference>

Reads a published OCI manifest through Docker Buildx and prints its canonical
digest. It waits only while the registry has not made the manifest readable.'
}

if [[ $# -eq 1 && "$1" == "--help" || $# -eq 1 && "$1" == "-h" ]]; then
  usage
  exit 0
fi

[[ $# -eq 1 ]] || { echo "error: exactly one image reference is required" >&2; usage >&2; exit 1; }

image_reference="$1"
attempts="${CONTAINER_REGISTRY_VERIFY_ATTEMPTS:-12}"
delay_seconds="${CONTAINER_REGISTRY_VERIFY_DELAY_SECONDS:-5}"

require_positive_integer() {
  local name="$1"
  local value="$2"
  [[ "${value}" =~ ^[1-9][0-9]*$ ]] || {
    echo "error: ${name} must be a positive integer (got: ${value})" >&2
    exit 1
  }
}

manifest_digest_from_inspection() {
  local inspection="$1"
  local inspection_line=""
  local candidate_digest=""
  local manifest_digest=""
  while IFS= read -r inspection_line || [[ -n "${inspection_line}" ]]; do
    case "${inspection_line}" in
      "Digest: "*)
        [[ "${inspection_line}" =~ ^Digest:[[:space:]]+(sha256:[a-f0-9]{64})[[:space:]]*$ ]] || return 1
        candidate_digest="${BASH_REMATCH[1]}"
        if [[ -n "${manifest_digest}" && "${manifest_digest}" != "${candidate_digest}" ]]; then
          return 1
        fi
        manifest_digest="${candidate_digest}"
        ;;
    esac
  done <<<"${inspection}"
  [[ -n "${manifest_digest}" ]] || return 1
  printf '%s\n' "${manifest_digest}"
}

command -v docker >/dev/null 2>&1 || { echo "error: docker is required" >&2; exit 1; }
command -v sleep >/dev/null 2>&1 || { echo "error: sleep is required" >&2; exit 1; }
require_positive_integer "CONTAINER_REGISTRY_VERIFY_ATTEMPTS" "${attempts}"
require_positive_integer "CONTAINER_REGISTRY_VERIFY_DELAY_SECONDS" "${delay_seconds}"

for ((attempt = 1; attempt <= attempts; attempt += 1)); do
  inspection=""
  if inspection="$(docker buildx imagetools inspect "${image_reference}")"; then
    if manifest_digest="$(manifest_digest_from_inspection "${inspection}")"; then
      printf '%s\n' "${manifest_digest}"
      exit 0
    fi
    echo "error: Docker manifest inspection returned no canonical digest for ${image_reference}" >&2
    printf '%s\n' "${inspection}" >&2
    exit 1
  else
    docker_exit_status=$?
  fi
  if (( attempt < attempts )); then
    echo "==> [registry] Waiting for ${image_reference} manifest (attempt ${attempt}/${attempts}; Docker exit ${docker_exit_status})." >&2
    sleep "${delay_seconds}"
  fi
done

echo "error: container manifest did not become readable for ${image_reference} after ${attempts} attempts" >&2
exit 1
