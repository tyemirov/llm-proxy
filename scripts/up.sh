#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repository_root="$(cd "${script_directory}/.." && pwd)"
config_path="${repository_root}/configs/config.yml"
proxy_binary="${LLM_PROXY_UP_BINARY:?LLM_PROXY_UP_BINARY must be set by make up}"
proxy_pid=""
local_service_ready="0"

fail() {
  echo "error: $*" >&2
  exit 1
}

stop_proxy() {
  if [[ -z "${proxy_pid}" ]]; then
    return
  fi
  if kill -0 "${proxy_pid}" >/dev/null 2>&1; then
    kill -TERM "${proxy_pid}" >/dev/null 2>&1 || true
  fi
  wait "${proxy_pid}" >/dev/null 2>&1 || true
  proxy_pid=""
}

cleanup() {
  local exit_status=$?
  trap - EXIT HUP INT TERM
  stop_proxy
  exit "${exit_status}"
}

handle_operator_interrupt() {
  if [[ "${local_service_ready}" == "1" ]]; then
    stop_proxy
    trap - EXIT HUP INT TERM
    echo
    echo "LLM Proxy localhost service stopped."
    exit 0
  fi
  exit 130
}

configured_port() {
  awk '
    /^server:[[:space:]]*$/ { inside_server = 1; next }
    inside_server && /^[^[:space:]]/ { exit }
    inside_server && /^[[:space:]]+port:[[:space:]]*[0-9]+[[:space:]]*$/ {
      port = $0
      sub(/^[[:space:]]+port:[[:space:]]*/, "", port)
      sub(/[[:space:]]*$/, "", port)
      print port
      exit
    }
  ' "${config_path}"
}

wait_for_local_service() {
  local readiness_status
  local attempt
  local proxy_exit_status
  for attempt in {1..100}; do
    readiness_status="$(curl --silent --show-error --max-time 1 --output /dev/null --write-out '%{http_code}' "${readiness_url}" 2>/dev/null || true)"
    if [[ "${readiness_status}" == "403" ]]; then
      return 0
    fi
    if ! kill -0 "${proxy_pid}" >/dev/null 2>&1; then
      if wait "${proxy_pid}"; then
        proxy_pid=""
        fail "local proxy exited before readiness"
      else
        proxy_exit_status=$?
        proxy_pid=""
        fail "local proxy exited before readiness with status ${proxy_exit_status}"
      fi
    fi
    sleep 0.1
  done
  fail "local proxy did not become ready at ${readiness_url}; expected HTTP 403 without a tenant key"
}

trap cleanup EXIT
trap 'exit 143' HUP TERM
trap handle_operator_interrupt INT

[[ -f "${config_path}" ]] || fail "missing canonical local configuration: ${config_path}"
[[ -x "${proxy_binary}" ]] || fail "missing local proxy binary: ${proxy_binary}"
command -v curl >/dev/null 2>&1 || fail "curl is required to verify local startup"

local_port="$(configured_port)"
[[ "${local_port}" =~ ^[0-9]+$ ]] || fail "configs/config.yml must declare a numeric server.port"
readiness_url="http://127.0.0.1:${local_port}/?prompt=ready"

cd "${repository_root}"
"${proxy_binary}" --config "${config_path}" &
proxy_pid="$!"
wait_for_local_service
local_service_ready="1"

echo
echo "LLM Proxy localhost service is ready."
echo "Proxy URL: http://localhost:${local_port}/"
echo "Readiness contract: GET /?prompt=ready without a tenant key returns 403."

wait "${proxy_pid}"
