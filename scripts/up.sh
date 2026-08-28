#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${script_directory}/local_orchestration.sh"
repository_root="${LOCAL_ORCHESTRATION_REPOSITORY_ROOT}"
local_environment_path="${repository_root}/configs/.env.local"
frontend_environment_path="${repository_root}/configs/.env.frontend.local"
api_environment_path="${repository_root}/configs/.env.api.local"
tauth_environment_path="${repository_root}/configs/.env.tauth.local"
local_stack_started="0"
local_stack_ready="0"
local_site_artifact_directory=""
local_frontend_origin="http://localhost:4179"
local_api_origin="http://localhost:8080"
local_tauth_tenant_id=""

fail() {
  echo "error: $*" >&2
  exit 1
}

require_private_local_environment() {
  [[ -f "${local_environment_path}" ]] ||
    fail "missing private local environment: ${local_environment_path}; create the ignored real file explicitly with mode 0600 (tracked env examples are documentation only)"
}

prepare_local_environment() {
  local_orchestration_prepare_environment \
    "${local_environment_path}" \
    "${frontend_environment_path}" \
    "${api_environment_path}" \
    "${tauth_environment_path}"
}

stop_local_stack() {
  local stop_status="0"
  if [[ "${local_stack_started}" != "1" ]]; then
    return
  fi
  if ! local_orchestration_compose down --remove-orphans; then
    stop_status="1"
  fi
  local_stack_started="0"
  return "${stop_status}"
}

prepare_local_site_artifact_directory() {
  local temporary_root

  temporary_root="${TMPDIR:-/tmp}"
  local_site_artifact_directory="$(mktemp -d "${temporary_root%/}/llm-proxy-local-site.XXXXXX")"
  [[ -d "${local_site_artifact_directory}" ]] || fail "mktemp did not create the local site artifact directory"
  export LLM_PROXY_LOCAL_SITE_ARTIFACT_DIRECTORY="${local_site_artifact_directory}"
}

remove_local_site_artifact_directory() {
  local artifact_directory

  artifact_directory="${local_site_artifact_directory}"
  if [[ -z "${artifact_directory}" ]]; then
    return
  fi
  local_site_artifact_directory=""
  unset LLM_PROXY_LOCAL_SITE_ARTIFACT_DIRECTORY
  rm -rf -- "${artifact_directory}"
}

cleanup() {
  local exit_status=$?
  trap - EXIT HUP INT TERM
  if ! stop_local_stack; then
    echo "error: local orchestration cleanup failed" >&2
    exit 1
  fi
  remove_local_site_artifact_directory
  exit "${exit_status}"
}

handle_operator_interrupt() {
  trap - EXIT HUP INT TERM
  if ! stop_local_stack; then
    echo "error: local orchestration cleanup failed" >&2
    exit 1
  fi
  remove_local_site_artifact_directory
  if [[ "${local_stack_ready}" == "1" ]]; then
    echo
    echo "LLM Proxy local orchestration stopped."
    exit 0
  fi
  exit 130
}

trap cleanup EXIT
trap 'exit 143' HUP TERM
trap handle_operator_interrupt INT

require_private_local_environment
command -v docker >/dev/null 2>&1 || fail "Docker Compose is required for make up"
docker compose version >/dev/null 2>&1 || fail "Docker Compose v2 is required for make up"
command -v curl >/dev/null 2>&1 || fail "curl is required to verify local startup"
command -v openssl >/dev/null 2>&1 || fail "openssl is required to generate local secrets"
[[ -f "${LOCAL_ORCHESTRATION_COMPOSE_FILE}" ]] || fail "missing local orchestration: ${LOCAL_ORCHESTRATION_COMPOSE_FILE}"

prepare_local_environment
while IFS='=' read -r scoped_variable_name scoped_variable_value; do
  if [[ "${scoped_variable_name}" == "LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID" ]]; then
    local_tauth_tenant_id="${scoped_variable_value}"
    break
  fi
done <"${api_environment_path}"
[[ -n "${local_tauth_tenant_id}" ]] || fail "${local_environment_path} must define LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID"
prepare_local_site_artifact_directory

cd "${repository_root}"
local_stack_started="1"
if local_orchestration_compose up --build --remove-orphans --wait; then
  local_orchestration_ensure_services_running "${LOCAL_ORCHESTRATION_COMPOSE_PROJECT}"
else
  compose_exit_status=$?
  fail "local orchestration failed to start with status ${compose_exit_status}"
fi

local_orchestration_verify_ready \
  "${LOCAL_ORCHESTRATION_COMPOSE_PROJECT}" \
  "${local_frontend_origin}" \
  "${local_api_origin}" \
  "${local_tauth_tenant_id}"
local_stack_ready="1"

echo
echo "LLM Proxy local orchestration is ready."
echo "Static UI: ${local_frontend_origin}/"
echo "Capability catalog: rendered from the current validated provider registry."
echo "OpenAPI schema: ${local_frontend_origin}/openapi.yaml (canonical read-only source)"
echo "API: ${local_api_origin}/"
echo "Public capabilities: ${local_api_origin}/api/public/capabilities"
echo "TAuth: ${local_frontend_origin}/auth/ (ghttp to TAuth)"
echo "Runtime config: ${local_frontend_origin}/config-ui.yaml (ghttp to API)"
echo "Readiness contracts: static=200, OpenAPI schema=200, config=200, public capabilities=200, API=403 without a key, same-origin TAuth session=204 and nonce=200, management API=401 without a session."

local_orchestration_compose logs --follow --no-color
