#!/usr/bin/env bash
set -euo pipefail

[[ -n "${RELEASE_TOOL_DIR:-}" ]] || { echo "error: RELEASE_TOOL_DIR is required" >&2; exit 1; }
for command_name in git tar; do
  command -v "${command_name}" >/dev/null 2>&1 || { echo "error: ${command_name} is required" >&2; exit 1; }
done

repo_root="$(git rev-parse --show-toplevel)"
source_directory="$(mktemp -d)"
trap 'rm -rf "${source_directory}"' EXIT

git -C "${repo_root}" archive --format=tar HEAD | tar -xf - -C "${source_directory}"
"${RELEASE_TOOL_DIR}/prepare_container_artifact.sh" \
  --name llm-proxy \
  --image "${DOCKER_IMAGE:-ghcr.io/tyemirov/llm-proxy}" \
  --file "${source_directory}/Dockerfile" \
  --context "${source_directory}" \
  --platforms "${PUBLISH_PLATFORMS:-linux/amd64,linux/arm64}" \
  --pull
