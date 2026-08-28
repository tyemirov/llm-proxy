#!/usr/bin/env bash

set -euo pipefail

usage() {
  builtin printf '%s\n' 'Usage: make test-live-local-providers LIVE_ENV_FILE=/path/to/env LLM_PROXY_LIVE_PROVIDERS=provider[,provider]

Builds the current local Compose services with disposable data volumes. The
command saves each selected provider connection through the local management
API and sends paid smoke requests through the Dockerized API. Cleanup removes
the complete test Compose project, including its volumes.

Use make test-live-local-gemini LIVE_ENV_FILE=/path/to/env for the complete
registered Gemini model and reasoning matrix.'
}

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  usage
  exit 0
fi

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${script_directory}/local_orchestration.sh"

readonly live_compose_project="llm-proxy-live-test"
readonly local_environment_path="${LOCAL_ORCHESTRATION_REPOSITORY_ROOT}/configs/.env.local"
readonly frontend_environment_path="${LOCAL_ORCHESTRATION_REPOSITORY_ROOT}/configs/.env.frontend.local"
readonly api_environment_path="${LOCAL_ORCHESTRATION_REPOSITORY_ROOT}/configs/.env.api.local"
readonly tauth_environment_path="${LOCAL_ORCHESTRATION_REPOSITORY_ROOT}/configs/.env.tauth.local"
local_frontend_origin=""
local_api_origin=""

local_stack_started="0"
local_environment_scoped="0"
local_site_artifact_directory=""

fail() {
  echo "error: $*" >&2
  exit 1
}

remove_local_site_artifact_directory() {
  local artifact_directory="${local_site_artifact_directory}"
  if [[ -z "${artifact_directory}" ]]; then
    return
  fi
  local_site_artifact_directory=""
  unset LLM_PROXY_LOCAL_SITE_ARTIFACT_DIRECTORY
  rm -rf -- "${artifact_directory}"
}

stop_live_stack() {
  local stop_status="0"
  if [[ "${local_stack_started}" != "1" ]]; then
    return
  fi
  if ! local_orchestration_compose_for_project "${live_compose_project}" down --remove-orphans --volumes; then
    stop_status="1"
  fi
  local_stack_started="0"
  return "${stop_status}"
}

restore_local_environment() {
  if [[ "${local_environment_scoped}" != "1" ]]; then
    return
  fi
  local_environment_scoped="0"
  local_orchestration_prepare_environment \
    "${local_environment_path}" \
    "${frontend_environment_path}" \
    "${api_environment_path}" \
    "${tauth_environment_path}"
}

cleanup() {
  local exit_status=$?
  local cleanup_status="0"
  trap - EXIT HUP INT TERM
  if ! stop_live_stack; then
    echo "error: local live-test orchestration cleanup failed" >&2
    cleanup_status="1"
  fi
  if ! restore_local_environment; then
    echo "error: local live-test environment restoration failed" >&2
    cleanup_status="1"
  fi
  if [[ "${cleanup_status}" == "1" ]]; then
    exit 1
  fi
  remove_local_site_artifact_directory
  exit "${exit_status}"
}

if [[ $# -ne 0 ]]; then
  fail "test-live-local-providers accepts no arguments"
fi

trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

[[ -n "${LIVE_ENV_FILE:-}" ]] || fail "LIVE_ENV_FILE is required"
[[ -f "${LIVE_ENV_FILE}" ]] || fail "LIVE_ENV_FILE not found: ${LIVE_ENV_FILE}"
[[ -n "${LLM_PROXY_LIVE_PROVIDERS:-}" ]] || fail "LLM_PROXY_LIVE_PROVIDERS is required"
[[ -f "${local_environment_path}" ]] ||
  fail "missing private local environment: ${local_environment_path}; create the ignored real file explicitly with mode 0600"
command -v docker >/dev/null 2>&1 || fail "Docker Compose is required"
docker compose version >/dev/null 2>&1 || fail "Docker Compose v2 is required"
command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v openssl >/dev/null 2>&1 || fail "openssl is required"
command -v python3 >/dev/null 2>&1 || fail "python3 is required to allocate isolated host ports"

if [[ -n "$(local_orchestration_compose_for_project "${LOCAL_ORCHESTRATION_COMPOSE_PROJECT}" ps --status running --services)" ]]; then
  fail "make up is already using the local HTTP ports"
fi
if [[ -n "$(local_orchestration_compose_for_project "${live_compose_project}" ps --all --quiet)" ]]; then
  fail "the ${live_compose_project} Compose project already contains state"
fi

LLM_PROXY_LOCAL_FRONTEND_HOST_PORT="$(local_orchestration_allocate_loopback_port)"
LLM_PROXY_LOCAL_API_HOST_PORT="$(local_orchestration_allocate_loopback_port)"
while [[ "${LLM_PROXY_LOCAL_API_HOST_PORT}" == "${LLM_PROXY_LOCAL_FRONTEND_HOST_PORT}" ]]; do
  LLM_PROXY_LOCAL_API_HOST_PORT="$(local_orchestration_allocate_loopback_port)"
done
export LLM_PROXY_LOCAL_FRONTEND_HOST_PORT LLM_PROXY_LOCAL_API_HOST_PORT
local_frontend_origin="http://127.0.0.1:${LLM_PROXY_LOCAL_FRONTEND_HOST_PORT}"
local_api_origin="http://127.0.0.1:${LLM_PROXY_LOCAL_API_HOST_PORT}"

local_orchestration_prepare_environment \
  "${local_environment_path}" \
  "${frontend_environment_path}" \
  "${api_environment_path}" \
  "${tauth_environment_path}"
local_environment_scoped="1"
for origin_name in \
  LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN \
  LLM_PROXY_MANAGEMENT_LOOPBACK_ORIGIN \
  LLM_PROXY_MANAGEMENT_LOCALHOST_ORIGIN; do
  local_orchestration_replace_environment_value "${api_environment_path}" "${origin_name}" "${local_frontend_origin}"
done
for origin_name in \
  LLM_PROXY_MANAGEMENT_API_ORIGIN \
  LLM_PROXY_MANAGEMENT_PROXY_ORIGIN; do
  local_orchestration_replace_environment_value "${api_environment_path}" "${origin_name}" "${local_api_origin}"
done
local_orchestration_replace_environment_value \
  "${tauth_environment_path}" \
  "LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN" \
  "${local_frontend_origin}"

local_tauth_tenant_id="$(local_orchestration_environment_value "${api_environment_path}" "LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID")"
[[ -n "${local_tauth_tenant_id}" ]] || fail "${api_environment_path} must define LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID"

temporary_root="${TMPDIR:-/tmp}"
local_site_artifact_directory="$(mktemp -d "${temporary_root%/}/llm-proxy-live-site.XXXXXX")"
export LLM_PROXY_LOCAL_SITE_ARTIFACT_DIRECTORY="${local_site_artifact_directory}"

cd "${LOCAL_ORCHESTRATION_REPOSITORY_ROOT}"
local_stack_started="1"
local_orchestration_compose_for_project "${live_compose_project}" up --build --remove-orphans --wait
local_orchestration_ensure_services_running "${live_compose_project}"
local_orchestration_verify_ready \
  "${live_compose_project}" \
  "${local_frontend_origin}" \
  "${local_api_origin}" \
  "${local_tauth_tenant_id}"

LLM_PROXY_LIVE_MANAGEMENT_ENV_FILE="${api_environment_path}" \
  "${script_directory}/test_live_providers.sh" \
    --existing-local-origin "${local_api_origin}"

echo "local Compose live-provider acceptance passed: providers=${LLM_PROXY_LIVE_PROVIDERS}"
