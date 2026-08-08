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
expected_running_services=$'api\nfrontend\nschema\ntauth'

fail() {
  echo "error: $*" >&2
  exit 1
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

write_scoped_local_environment() {
  local destination_path="$1"
  local temporary_environment_path
  shift

  temporary_environment_path="$(mktemp "${destination_path}.XXXXXX")"
  if ! awk '
    BEGIN {
      environment_path = ARGV[1]
      requested_count = 0
      for (argument_index = 2; argument_index < ARGC; argument_index++) {
        requested_count++
        requested_names[requested_count] = ARGV[argument_index]
        requested_lookup[ARGV[argument_index]] = 1
        ARGV[argument_index] = ""
      }
    }
    {
      separator_index = index($0, "=")
      if (separator_index == 0) {
        next
      }
      variable_name = substr($0, 1, separator_index - 1)
      if ((variable_name in requested_lookup) && !(variable_name in environment_values)) {
        environment_values[variable_name] = substr($0, separator_index + 1)
      }
    }
    END {
      for (requested_index = 1; requested_index <= requested_count; requested_index++) {
        variable_name = requested_names[requested_index]
        if (!(variable_name in environment_values) || environment_values[variable_name] == "") {
          printf "error: %s must define %s\n", environment_path, variable_name >"/dev/stderr"
          exit 1
        }
        printf "%s=%s\n", variable_name, environment_values[variable_name]
      }
    }
  ' "${local_environment_path}" "$@" >"${temporary_environment_path}"; then
    rm -f "${temporary_environment_path}"
    return 1
  fi
  mv "${temporary_environment_path}" "${destination_path}"
  chmod 600 "${destination_path}"
}

require_private_local_environment() {
  [[ -f "${local_environment_path}" ]] ||
    fail "missing private local environment: ${local_environment_path}; create the ignored real file explicitly with mode 0600 (tracked env examples are documentation only)"
}

prepare_local_environment() {
  chmod 600 "${local_environment_path}"
  ensure_generated_local_value "LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY" "__GENERATE_ON_FIRST_MAKE_UP__" "48"
  ensure_generated_local_value "LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY" "__GENERATE_ON_FIRST_MAKE_UP__" "32"

  write_scoped_local_environment "${frontend_environment_path}" \
    "GHTTP_SERVE_PORT" \
    "GHTTP_SERVE_DIRECTORY" \
    "GHTTP_SERVE_NO_MARKDOWN"
  write_scoped_local_environment "${api_environment_path}" \
    "LLM_PROXY_MANAGEMENT_ENABLED" \
    "LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN" \
    "LLM_PROXY_MANAGEMENT_LOOPBACK_ORIGIN" \
    "LLM_PROXY_MANAGEMENT_LOCALHOST_ORIGIN" \
    "LLM_PROXY_MANAGEMENT_UI_DESCRIPTION" \
    "LLM_PROXY_MANAGEMENT_ADMIN_EMAILS" \
    "LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID" \
    "LLM_PROXY_MANAGEMENT_GOOGLE_CLIENT_ID" \
    "LLM_PROXY_MANAGEMENT_TAUTH_LOGIN_PATH" \
    "LLM_PROXY_MANAGEMENT_TAUTH_LOGOUT_PATH" \
    "LLM_PROXY_MANAGEMENT_TAUTH_NONCE_PATH" \
    "LLM_PROXY_MANAGEMENT_TAUTH_SESSION_PATH" \
    "LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY" \
    "LLM_PROXY_MANAGEMENT_JWT_ISSUER" \
    "LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME" \
    "LLM_PROXY_MANAGEMENT_DATABASE_PATH" \
    "LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY" \
    "LLM_PROXY_MANAGEMENT_API_ORIGIN" \
    "LLM_PROXY_MANAGEMENT_PROXY_ORIGIN"
  write_scoped_local_environment "${tauth_environment_path}" \
    "TAUTH_CONFIG_FILE" \
    "TAUTH_LISTEN_ADDR" \
    "TAUTH_DATABASE_URL" \
    "TAUTH_ENABLE_CORS" \
    "TAUTH_CORS_EXCEPTION_1" \
    "TAUTH_ALLOW_INSECURE_HTTP" \
    "LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN" \
    "LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID" \
    "LLM_PROXY_MANAGEMENT_GOOGLE_CLIENT_ID" \
    "LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY" \
    "LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME" \
    "LLM_PROXY_LOCAL_TAUTH_REFRESH_COOKIE_NAME"
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

verify_local_capability_catalog() {
  local landing_page

  landing_page="$(curl --silent --show-error --max-time 5 "${local_frontend_origin}/")" ||
    fail "ghttp did not return the rendered landing page"
  [[ "${landing_page}" == *'class="catalog-table"'* ]] ||
    fail "ghttp landing page omitted the generated capability matrix"
  [[ "${landing_page}" != *"llm-proxy-capability-catalog"* ]] ||
    fail "ghttp landing page retained the unrendered capability marker"
  [[ "${landing_page}" != *"api_key"* && "${landing_page}" != *"base_url"* ]] ||
    fail "ghttp landing page exposed private provider configuration"
}

ensure_compose_services_running() {
  local running_services
  running_services="$(local_orchestration_compose ps --status running --services | LC_ALL=C sort)"
  [[ "${running_services}" == "${expected_running_services}" ]] || fail "local orchestration services are not running; expected api, frontend, schema, tauth; got ${running_services:-none}"
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
      ensure_compose_services_running
      return
    fi
    ensure_compose_services_running
    sleep 0.2
  done
  fail "${boundary_name} did not become ready at ${readiness_url}; expected HTTP ${expected_status}, got ${readiness_status:-connection_failure}"
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
  ensure_compose_services_running
else
  compose_exit_status=$?
  fail "local orchestration failed to start with status ${compose_exit_status}"
fi

wait_for_http_status "ghttp static frontend" "200" "${local_frontend_origin}/"
verify_local_capability_catalog
wait_for_http_status "ghttp canonical OpenAPI schema" "200" "${local_frontend_origin}/openapi.yaml"
wait_for_http_status "ghttp runtime configuration" "200" "${local_frontend_origin}/config-ui.yaml"
wait_for_http_status "LLM Proxy API boundary" "403" "${local_api_origin}/?prompt=ready"
wait_for_http_status "TAuth session through ghttp" "204" "${local_frontend_origin}/auth/session" --header "X-TAuth-Tenant: ${local_tauth_tenant_id}"
wait_for_http_status "TAuth nonce through ghttp" "200" "${local_frontend_origin}/auth/nonce" --request POST --header "Origin: ${local_frontend_origin}" --header "Content-Type: application/json" --header "X-Requested-With: XMLHttpRequest" --header "X-TAuth-Tenant: ${local_tauth_tenant_id}"
wait_for_http_status "LLM Proxy management API boundary" "401" "${local_api_origin}/api/management/account" --header "Origin: ${local_frontend_origin}"
local_stack_ready="1"

echo
echo "LLM Proxy local orchestration is ready."
echo "Static UI: ${local_frontend_origin}/"
echo "Capability catalog: rendered from the current validated provider registry."
echo "OpenAPI schema: ${local_frontend_origin}/openapi.yaml (canonical read-only source)"
echo "API: ${local_api_origin}/"
echo "TAuth: ${local_frontend_origin}/auth/ (ghttp to TAuth)"
echo "Runtime config: ${local_frontend_origin}/config-ui.yaml (ghttp to API)"
echo "Readiness contracts: static=200, OpenAPI schema=200, config=200, API=403 without a key, same-origin TAuth session=204 and nonce=200, management API=401 without a session."

local_orchestration_compose logs --follow --no-color
