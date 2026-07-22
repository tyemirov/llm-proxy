#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repository_root="$(cd "${script_directory}/.." && pwd)"
base_environment_path="${repository_root}/configs/.env"
base_environment_example_path="${repository_root}/configs/.env.sample"
local_environment_path="${repository_root}/configs/.env.local"
local_environment_example_path="${repository_root}/configs/.env.local.example"
compose_file="${repository_root}/docker-compose.local.yml"
compose_project="llm-proxy-local"
compose_pid=""
local_stack_started="0"
local_stack_ready="0"
local_frontend_origin="http://localhost:4179"

fail() {
  echo "error: $*" >&2
  exit 1
}

compose() {
  docker compose --project-name "${compose_project}" --file "${compose_file}" "$@"
}

local_environment_value() {
  local variable_name="$1"
  awk -v requested_name="${variable_name}" '
    index($0, requested_name "=") == 1 {
      print substr($0, length(requested_name) + 2)
      exit
    }
  ' "${local_environment_path}"
}

ensure_generated_local_value() {
  local variable_name="$1"
  local placeholder_value="$2"
  local random_byte_count="$3"
  local configured_value
  local generated_value
  local temporary_environment_path

  configured_value="$(local_environment_value "${variable_name}")"
  [[ -n "${configured_value}" ]] || fail "${local_environment_path} must define ${variable_name}"
  if [[ "${configured_value}" != "${placeholder_value}" ]]; then
    return
  fi

  generated_value="$(openssl rand -base64 "${random_byte_count}")"
  [[ -n "${generated_value}" ]] || fail "openssl did not generate ${variable_name}"
  temporary_environment_path="$(mktemp "${local_environment_path}.XXXXXX")"
  awk -v requested_name="${variable_name}" -v replacement_value="${generated_value}" '
    index($0, requested_name "=") == 1 {
      print requested_name "=" replacement_value
      next
    }
    { print }
  ' "${local_environment_path}" >"${temporary_environment_path}"
  mv "${temporary_environment_path}" "${local_environment_path}"
  chmod 600 "${local_environment_path}"
  echo "Generated ${variable_name} for the local profile."
}

prepare_local_environment() {
  if [[ ! -f "${base_environment_path}" ]]; then
    [[ -f "${base_environment_example_path}" ]] || fail "missing base environment example: ${base_environment_example_path}"
    cp "${base_environment_example_path}" "${base_environment_path}"
    chmod 600 "${base_environment_path}"
    echo "Created ${base_environment_path} from the tracked example."
  fi
  if [[ ! -f "${local_environment_path}" ]]; then
    [[ -f "${local_environment_example_path}" ]] || fail "missing local environment example: ${local_environment_example_path}"
    cp "${local_environment_example_path}" "${local_environment_path}"
    chmod 600 "${local_environment_path}"
    echo "Created ${local_environment_path} from the tracked example."
  fi
  ensure_generated_local_value "LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY" "__GENERATE_ON_FIRST_MAKE_UP__" "48"
  ensure_generated_local_value "LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY" "__GENERATE_ON_FIRST_MAKE_UP__" "32"
}

stop_local_stack() {
  local stop_status="0"
  if [[ "${local_stack_started}" != "1" ]]; then
    return
  fi
  if ! compose down --remove-orphans; then
    stop_status="1"
  fi
  if [[ -n "${compose_pid}" ]] && kill -0 "${compose_pid}" >/dev/null 2>&1; then
    kill -TERM "${compose_pid}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${compose_pid}" ]]; then
    wait "${compose_pid}" >/dev/null 2>&1 || true
  fi
  compose_pid=""
  local_stack_started="0"
  return "${stop_status}"
}

cleanup() {
  local exit_status=$?
  trap - EXIT HUP INT TERM
  stop_local_stack || true
  exit "${exit_status}"
}

handle_operator_interrupt() {
  trap - EXIT HUP INT TERM
  if ! stop_local_stack; then
    echo "error: local orchestration cleanup failed" >&2
    exit 1
  fi
  if [[ "${local_stack_ready}" == "1" ]]; then
    echo
    echo "LLM Proxy local orchestration stopped."
    exit 0
  fi
  exit 130
}

ensure_compose_running() {
  local compose_exit_status
  if [[ -n "${compose_pid}" ]] && kill -0 "${compose_pid}" >/dev/null 2>&1; then
    return
  fi
  if [[ -n "${compose_pid}" ]] && wait "${compose_pid}"; then
    compose_exit_status="0"
  else
    compose_exit_status=$?
  fi
  compose_pid=""
  fail "local orchestration exited before readiness with status ${compose_exit_status}"
}

wait_for_http_status() {
  local boundary_name="$1"
  local expected_status="$2"
  local readiness_url="$3"
  local attempt
  local readiness_status
  shift 3

  for attempt in {1..150}; do
    readiness_status="$(curl --silent --show-error --max-time 1 --output /dev/null --write-out '%{http_code}' "$@" "${readiness_url}" 2>/dev/null || true)"
    if [[ "${readiness_status}" == "${expected_status}" ]]; then
      ensure_compose_running
      return
    fi
    ensure_compose_running
    sleep 0.2
  done
  fail "${boundary_name} did not become ready at ${readiness_url}; expected HTTP ${expected_status}, got ${readiness_status:-connection_failure}"
}

trap cleanup EXIT
trap 'exit 143' HUP TERM
trap handle_operator_interrupt INT

command -v docker >/dev/null 2>&1 || fail "Docker Compose is required for make up"
docker compose version >/dev/null 2>&1 || fail "Docker Compose v2 is required for make up"
command -v curl >/dev/null 2>&1 || fail "curl is required to verify local startup"
command -v openssl >/dev/null 2>&1 || fail "openssl is required to generate local secrets"
[[ -f "${compose_file}" ]] || fail "missing local orchestration: ${compose_file}"

prepare_local_environment

cd "${repository_root}"
docker compose --project-name "${compose_project}" --file "${compose_file}" up --build --remove-orphans &
compose_pid="$!"
local_stack_started="1"

wait_for_http_status "ghttp static frontend" "200" "http://127.0.0.1:4179/"
wait_for_http_status "ghttp runtime configuration" "200" "http://127.0.0.1:4179/config-ui.yaml"
wait_for_http_status "LLM Proxy API boundary" "403" "http://127.0.0.1:8080/?prompt=ready"
wait_for_http_status "TAuth session boundary" "204" "http://127.0.0.1:8082/auth/session" --header "Origin: ${local_frontend_origin}" --header "X-Requested-With: XMLHttpRequest"
wait_for_http_status "LLM Proxy management API boundary" "401" "http://127.0.0.1:8080/api/management/profile" --header "Origin: ${local_frontend_origin}"
local_stack_ready="1"

echo
echo "LLM Proxy local orchestration is ready."
echo "Static UI: ${local_frontend_origin}/"
echo "API: http://localhost:8080/"
echo "TAuth: http://localhost:8082/"
echo "Runtime config: ${local_frontend_origin}/config-ui.yaml (ghttp to API)"
echo "Readiness contracts: static=200, config=200, API=403 without a key, TAuth session=204, management API=401 without a session."

wait "${compose_pid}"
compose_pid=""
