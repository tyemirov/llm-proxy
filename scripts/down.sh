#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${script_directory}/local_orchestration.sh"

fail() {
  echo "error: $*" >&2
  exit 1
}

command -v docker >/dev/null 2>&1 || fail "Docker Compose is required for make down"
docker compose version >/dev/null 2>&1 || fail "Docker Compose v2 is required for make down"
[[ -f "${LOCAL_ORCHESTRATION_COMPOSE_FILE}" ]] || fail "missing local orchestration: ${LOCAL_ORCHESTRATION_COMPOSE_FILE}"

cd "${LOCAL_ORCHESTRATION_REPOSITORY_ROOT}"
local_orchestration_compose down --remove-orphans

echo "LLM Proxy local orchestration stopped."
