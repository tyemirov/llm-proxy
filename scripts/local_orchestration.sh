#!/usr/bin/env bash

LOCAL_ORCHESTRATION_SCRIPT_DIRECTORY="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOCAL_ORCHESTRATION_REPOSITORY_ROOT="$(cd "${LOCAL_ORCHESTRATION_SCRIPT_DIRECTORY}/.." && pwd)"
LOCAL_ORCHESTRATION_COMPOSE_FILE="${LOCAL_ORCHESTRATION_REPOSITORY_ROOT}/docker-compose.local.yml"
LOCAL_ORCHESTRATION_COMPOSE_PROJECT="llm-proxy-local"

readonly LOCAL_ORCHESTRATION_SCRIPT_DIRECTORY
readonly LOCAL_ORCHESTRATION_REPOSITORY_ROOT
readonly LOCAL_ORCHESTRATION_COMPOSE_FILE
readonly LOCAL_ORCHESTRATION_COMPOSE_PROJECT

local_orchestration_compose() {
  local_orchestration_compose_for_project "${LOCAL_ORCHESTRATION_COMPOSE_PROJECT}" "$@"
}

local_orchestration_compose_for_project() {
  local compose_project="$1"
  local site_artifact_directory
  shift

  site_artifact_directory="${LLM_PROXY_LOCAL_SITE_ARTIFACT_DIRECTORY:-${TMPDIR:-/tmp}}"
  LLM_PROXY_LOCAL_SITE_ARTIFACT_DIRECTORY="${site_artifact_directory}" \
    docker compose \
      --project-name "${compose_project}" \
      --file "${LOCAL_ORCHESTRATION_COMPOSE_FILE}" \
      "$@"
}

local_orchestration_allocate_loopback_port() {
  python3 -c '
import socket

with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
    listener.bind(("127.0.0.1", 0))
    print(listener.getsockname()[1])
'
}

local_orchestration_environment_value() {
  local source_environment_path="$1"
  local variable_name="$2"
  awk -v requested_name="${variable_name}" '
    index($0, requested_name "=") == 1 {
      print substr($0, length(requested_name) + 2)
      exit
    }
  ' "${source_environment_path}"
}

local_orchestration_replace_environment_value() {
  local environment_path="$1"
  local variable_name="$2"
  local replacement_value="$3"
  local temporary_environment_path

  [[ -n "$(local_orchestration_environment_value "${environment_path}" "${variable_name}")" ]] || {
    echo "error: ${environment_path} must define ${variable_name}" >&2
    return 1
  }
  temporary_environment_path="$(mktemp "${environment_path}.XXXXXX")"
  awk -v requested_name="${variable_name}" -v replacement_value="${replacement_value}" '
    index($0, requested_name "=") == 1 {
      print requested_name "=" replacement_value
      next
    }
    { print }
  ' "${environment_path}" >"${temporary_environment_path}"
  mv "${temporary_environment_path}" "${environment_path}"
  chmod 600 "${environment_path}"
}

local_orchestration_ensure_generated_value() {
  local source_environment_path="$1"
  local variable_name="$2"
  local placeholder_value="$3"
  local random_byte_count="$4"
  local configured_value
  local generated_value
  local temporary_environment_path

  configured_value="$(local_orchestration_environment_value "${source_environment_path}" "${variable_name}")"
  [[ -n "${configured_value}" ]] || {
    echo "error: ${source_environment_path} must define ${variable_name}" >&2
    return 1
  }
  if [[ "${configured_value}" != "${placeholder_value}" ]]; then
    return
  fi

  generated_value="$(openssl rand -base64 "${random_byte_count}")"
  [[ -n "${generated_value}" ]] || {
    echo "error: openssl did not generate ${variable_name}" >&2
    return 1
  }
  temporary_environment_path="$(mktemp "${source_environment_path}.XXXXXX")"
  awk -v requested_name="${variable_name}" -v replacement_value="${generated_value}" '
    index($0, requested_name "=") == 1 {
      print requested_name "=" replacement_value
      next
    }
    { print }
  ' "${source_environment_path}" >"${temporary_environment_path}"
  mv "${temporary_environment_path}" "${source_environment_path}"
  chmod 600 "${source_environment_path}"
  echo "Generated ${variable_name} for the local profile."
}

local_orchestration_write_scoped_environment() {
  local source_path="$1"
  local destination_path="$2"
  local temporary_environment_path
  shift 2

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
  ' "${source_path}" "$@" >"${temporary_environment_path}"; then
    rm -f "${temporary_environment_path}"
    return 1
  fi
  mv "${temporary_environment_path}" "${destination_path}"
  chmod 600 "${destination_path}"
}

local_orchestration_prepare_environment() {
  local source_environment_path="$1"
  local scoped_frontend_environment_path="$2"
  local scoped_api_environment_path="$3"
  local scoped_tauth_environment_path="$4"

  chmod 600 "${source_environment_path}"
  local_orchestration_ensure_generated_value "${source_environment_path}" "LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY" "__GENERATE_ON_FIRST_MAKE_UP__" "48"
  local_orchestration_ensure_generated_value "${source_environment_path}" "LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY" "__GENERATE_ON_FIRST_MAKE_UP__" "32"

  local_orchestration_write_scoped_environment "${source_environment_path}" "${scoped_frontend_environment_path}" \
    "GHTTP_SERVE_PORT" \
    "GHTTP_SERVE_DIRECTORY" \
    "GHTTP_SERVE_NO_MARKDOWN"
  local_orchestration_write_scoped_environment "${source_environment_path}" "${scoped_api_environment_path}" \
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
  local_orchestration_write_scoped_environment "${source_environment_path}" "${scoped_tauth_environment_path}" \
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

local_orchestration_ensure_services_running() {
  local compose_project="$1"
  local running_services
  local expected_services=$'api\nfrontend\nschema\ntauth'
  running_services="$(local_orchestration_compose_for_project "${compose_project}" ps --status running --services | LC_ALL=C sort)"
  [[ "${running_services}" == "${expected_services}" ]] || {
    echo "error: local orchestration services are not running; expected api, frontend, schema, tauth; got ${running_services:-none}" >&2
    return 1
  }
}

local_orchestration_wait_for_http_status() {
  local compose_project="$1"
  local boundary_name="$2"
  local expected_status="$3"
  local readiness_url="$4"
  local attempt
  local readiness_status
  shift 4

  for attempt in {1..150}; do
    readiness_status="$(curl --silent --show-error --max-time 1 --output /dev/null --write-out '%{http_code}' "$@" "${readiness_url}" 2>/dev/null || true)"
    if [[ "${readiness_status}" == "${expected_status}" ]]; then
      local_orchestration_ensure_services_running "${compose_project}"
      return
    fi
    local_orchestration_ensure_services_running "${compose_project}"
    sleep 0.2
  done
  echo "error: ${boundary_name} did not become ready at ${readiness_url}; expected HTTP ${expected_status}, got ${readiness_status:-connection_failure}" >&2
  return 1
}

local_orchestration_verify_capability_catalog() {
  local frontend_origin="$1"
  local landing_page

  landing_page="$(curl --silent --show-error --max-time 5 "${frontend_origin}/")" || {
    echo "error: ghttp did not return the rendered landing page" >&2
    return 1
  }
  [[ "${landing_page}" == *'class="routing-tree"'* ]] || {
    echo "error: ghttp landing page omitted the generated routing tree" >&2
    return 1
  }
  [[ "${landing_page}" == *'class="catalog-table"'* ]] || {
    echo "error: ghttp landing page omitted the generated capability matrix" >&2
    return 1
  }
  [[ "${landing_page}" != *"llm-proxy-routing-tree"* ]] || {
    echo "error: ghttp landing page retained the unrendered routing marker" >&2
    return 1
  }
  [[ "${landing_page}" != *"llm-proxy-capability-catalog"* ]] || {
    echo "error: ghttp landing page retained the unrendered capability marker" >&2
    return 1
  }
  [[ "${landing_page}" != *"api_key"* && "${landing_page}" != *"base_url"* ]] || {
    echo "error: ghttp landing page exposed private provider configuration" >&2
    return 1
  }
}

local_orchestration_verify_ready() {
  local compose_project="$1"
  local frontend_origin="$2"
  local api_origin="$3"
  local tauth_tenant_id="$4"

  local_orchestration_wait_for_http_status "${compose_project}" "ghttp static frontend" "200" "${frontend_origin}/"
  local_orchestration_verify_capability_catalog "${frontend_origin}"
  local_orchestration_wait_for_http_status "${compose_project}" "ghttp canonical OpenAPI schema" "200" "${frontend_origin}/openapi.yaml"
  local_orchestration_wait_for_http_status "${compose_project}" "ghttp runtime configuration" "200" "${frontend_origin}/config-ui.yaml"
  local_orchestration_wait_for_http_status "${compose_project}" "LLM Proxy public capabilities" "200" "${api_origin}/api/public/capabilities"
  local_orchestration_wait_for_http_status "${compose_project}" "LLM Proxy API boundary" "403" "${api_origin}/?prompt=ready"
  local_orchestration_wait_for_http_status "${compose_project}" "TAuth session through ghttp" "204" "${frontend_origin}/auth/session" --header "X-TAuth-Tenant: ${tauth_tenant_id}"
  local_orchestration_wait_for_http_status "${compose_project}" "TAuth nonce through ghttp" "200" "${frontend_origin}/auth/nonce" --request POST --header "Origin: ${frontend_origin}" --header "Content-Type: application/json" --header "X-Requested-With: XMLHttpRequest" --header "X-TAuth-Tenant: ${tauth_tenant_id}"
  local_orchestration_wait_for_http_status "${compose_project}" "LLM Proxy management API boundary" "401" "${api_origin}/api/management/account" --header "Origin: ${frontend_origin}"
}
