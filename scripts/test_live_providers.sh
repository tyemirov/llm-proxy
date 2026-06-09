#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  scripts/test_live_providers.sh

Builds the current llm-proxy binary and runs live text smoke tests for providers
whose API keys are present.

Required environment:
  At least one provider API key, unless no-op skip behavior is desired.

Provider key variables:
  OPENAI_API_KEY
  DEEPSEEK_API_KEY
  DASHSCOPE_API_KEY
  MOONSHOT_API_KEY
  SILICONFLOW_API_KEY
  ZHIPU_API_KEY
  GEMINI_API_KEY
  ANTHROPIC_API_KEY
  XAI_API_KEY

Optional environment:
  LIVE_ENV_FILE              Path to an env file to source before discovery.
  LLM_PROXY_LIVE_PROVIDERS   Comma or space separated provider list. If set,
                             every listed provider must have its key.
  LLM_PROXY_LIVE_PORT        Local port for the temporary proxy. Default: 18080.
  LLM_PROXY_LIVE_TIMEOUT     Per-request curl timeout in seconds. Default: 45.
  SERVICE_SECRET             Tenant secret. Generated when omitted.
  GO                         Go binary. Default: go.

Per-provider model overrides:
  LLM_PROXY_LIVE_OPENAI_MODEL
  LLM_PROXY_LIVE_DEEPSEEK_MODEL
  LLM_PROXY_LIVE_DASHSCOPE_MODEL
  LLM_PROXY_LIVE_MOONSHOT_MODEL
  LLM_PROXY_LIVE_SILICONFLOW_MODEL
  LLM_PROXY_LIVE_ZHIPU_MODEL
  LLM_PROXY_LIVE_GEMINI_MODEL
  LLM_PROXY_LIVE_ANTHROPIC_MODEL
  LLM_PROXY_LIVE_GROK_MODEL
USAGE
}

env_or_default() {
  local name="$1"
  local fallback="$2"
  local value=""
  if [[ -v "${name}" ]]; then
    value="${!name}"
  fi
  if [[ -n "${value}" ]]; then
    printf "%s\n" "${value}"
  else
    printf "%s\n" "${fallback}"
  fi
}

provider_key_variable() {
  case "$1" in
    openai) printf "%s\n" "OPENAI_API_KEY" ;;
    deepseek) printf "%s\n" "DEEPSEEK_API_KEY" ;;
    dashscope) printf "%s\n" "DASHSCOPE_API_KEY" ;;
    moonshot) printf "%s\n" "MOONSHOT_API_KEY" ;;
    siliconflow) printf "%s\n" "SILICONFLOW_API_KEY" ;;
    zhipu) printf "%s\n" "ZHIPU_API_KEY" ;;
    gemini) printf "%s\n" "GEMINI_API_KEY" ;;
    anthropic) printf "%s\n" "ANTHROPIC_API_KEY" ;;
    grok) printf "%s\n" "XAI_API_KEY" ;;
    *) return 1 ;;
  esac
}

provider_model_override() {
  case "$1" in
    openai) env_or_default LLM_PROXY_LIVE_OPENAI_MODEL "" ;;
    deepseek) env_or_default LLM_PROXY_LIVE_DEEPSEEK_MODEL "" ;;
    dashscope) env_or_default LLM_PROXY_LIVE_DASHSCOPE_MODEL "" ;;
    moonshot) env_or_default LLM_PROXY_LIVE_MOONSHOT_MODEL "" ;;
    siliconflow) env_or_default LLM_PROXY_LIVE_SILICONFLOW_MODEL "" ;;
    zhipu) env_or_default LLM_PROXY_LIVE_ZHIPU_MODEL "" ;;
    gemini) env_or_default LLM_PROXY_LIVE_GEMINI_MODEL "" ;;
    anthropic) env_or_default LLM_PROXY_LIVE_ANTHROPIC_MODEL "" ;;
    grok) env_or_default LLM_PROXY_LIVE_GROK_MODEL "" ;;
    *) return 1 ;;
  esac
}

validate_provider_name() {
  provider_key_variable "$1" >/dev/null
}

provider_has_key() {
  local provider="$1"
  local key_variable
  key_variable="$(provider_key_variable "${provider}")"
  [[ -n "${!key_variable:-}" ]]
}

discover_live_providers() {
  local selected_provider
  if [[ -n "${LLM_PROXY_LIVE_PROVIDERS:-}" ]]; then
    for selected_provider in ${LLM_PROXY_LIVE_PROVIDERS//,/ }; do
      [[ -n "${selected_provider}" ]] || continue
      validate_provider_name "${selected_provider}" || {
        echo "error: unknown live provider: ${selected_provider}" >&2
        exit 1
      }
      if ! provider_has_key "${selected_provider}"; then
        echo "error: ${selected_provider} requested but $(provider_key_variable "${selected_provider}") is not set" >&2
        exit 1
      fi
      LIVE_PROVIDERS+=("${selected_provider}")
    done
    return
  fi

  for selected_provider in "${SUPPORTED_PROVIDERS[@]}"; do
    if provider_has_key "${selected_provider}"; then
      LIVE_PROVIDERS+=("${selected_provider}")
    fi
  done
}

export_unused_provider_placeholders() {
  local key_variable
  for key_variable in \
    OPENAI_API_KEY \
    DEEPSEEK_API_KEY \
    DASHSCOPE_API_KEY \
    MOONSHOT_API_KEY \
    SILICONFLOW_API_KEY \
    ZHIPU_API_KEY \
    GEMINI_API_KEY \
    ANTHROPIC_API_KEY \
    XAI_API_KEY; do
    if [[ -z "${!key_variable:-}" ]]; then
      export "${key_variable}=unused-${key_variable}-for-live-smoke"
    fi
  done
}

redact_log() {
  sed -E 's/(key=)[^& ]+/\1<redacted>/g; s/(api_key: ).+/\1<redacted>/g' "${LOG_PATH}" >&2 || true
}

write_live_config() {
  awk -v port="${PORT}" '
    /^  port: / && replaced == 0 {
      print "  port: " port
      replaced = 1
      next
    }
    { print }
  ' "${ROOT_DIR}/configs/config.yml" > "${CONFIG_PATH}"
}

wait_for_proxy() {
  local readiness_status
  for _ in {1..50}; do
    readiness_status="$(curl -sS --max-time 1 -o /dev/null -w "%{http_code}" "http://127.0.0.1:${PORT}/?prompt=ready" 2>/dev/null || true)"
    if [[ "${readiness_status}" == "403" ]]; then
      return 0
    fi
    if ! kill -0 "${PROXY_PID}" >/dev/null 2>&1; then
      echo "error: live proxy exited before readiness" >&2
      redact_log
      exit 1
    fi
    sleep 0.1
  done
  echo "error: live proxy did not become ready on port ${PORT}" >&2
  redact_log
  exit 1
}

run_text_smoke() {
  local provider="$1"
  local model
  local response_path
  local request_body
  local http_status
  local response_text
  model="$(provider_model_override "${provider}")"
  response_path="${TMP_DIR}/${provider}-response.txt"
  if [[ -n "${model}" ]]; then
    request_body="$(printf '{"prompt":"Reply with exactly OK and no punctuation.","model":"%s","web_search":false}' "${model}")"
  else
    request_body='{"prompt":"Reply with exactly OK and no punctuation.","web_search":false}'
  fi

  http_status="$(
    curl -sS --max-time "${LIVE_TIMEOUT}" \
      -X POST \
      -H "Content-Type: application/json" \
      --data "${request_body}" \
      -o "${response_path}" \
      -w "%{http_code}" \
      "http://127.0.0.1:${PORT}/?provider=${provider}&format=text/plain&key=${SERVICE_SECRET}"
  )"

  response_text="$(tr -d '\r\n' < "${response_path}" | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//')"
  if [[ "${http_status}" != "200" || "${response_text}" != "OK" ]]; then
    echo "error: live ${provider} smoke failed: model=${model:-configured-default} status=${http_status} response=${response_text}" >&2
    redact_log
    exit 1
  fi
  echo "live provider smoke passed: provider=${provider} model=${model:-configured-default} status=${http_status}"
}

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  usage
  exit 0
fi

SUPPORTED_PROVIDERS=(openai deepseek dashscope moonshot siliconflow zhipu gemini anthropic grok)
LIVE_PROVIDERS=()
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
PROXY_PID=""

cleanup() {
  if [[ -n "${PROXY_PID}" ]] && kill -0 "${PROXY_PID}" >/dev/null 2>&1; then
    kill "${PROXY_PID}" >/dev/null 2>&1 || true
    wait "${PROXY_PID}" >/dev/null 2>&1 || true
  fi
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

if [[ -n "${LIVE_ENV_FILE:-}" ]]; then
  [[ -f "${LIVE_ENV_FILE}" ]] || { echo "error: LIVE_ENV_FILE not found: ${LIVE_ENV_FILE}" >&2; exit 1; }
  set -a
  # shellcheck source=/dev/null
  . "${LIVE_ENV_FILE}"
  set +a
fi

discover_live_providers
if [[ "${#LIVE_PROVIDERS[@]}" -eq 0 ]]; then
  echo "live provider smoke skipped: no provider API keys found"
  exit 0
fi

GO_BIN="$(env_or_default GO go)"
PORT="$(env_or_default LLM_PROXY_LIVE_PORT 18080)"
LIVE_TIMEOUT="$(env_or_default LLM_PROXY_LIVE_TIMEOUT 45)"
BINARY_PATH="${TMP_DIR}/llm-proxy-live"
CONFIG_PATH="${TMP_DIR}/config.yml"
LOG_PATH="${TMP_DIR}/llm-proxy.log"
export SERVICE_SECRET="${SERVICE_SECRET:-live-service-secret}"
export LLM_PROXY_LIVE_PORT="${PORT}"
export_unused_provider_placeholders

if command -v lsof >/dev/null 2>&1 && lsof -tiTCP:"${PORT}" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "error: port ${PORT} is already in use; set LLM_PROXY_LIVE_PORT to a free port" >&2
  exit 1
fi

cd "${ROOT_DIR}"
GOEXPERIMENT= CGO_ENABLED=0 "${GO_BIN}" build -o "${BINARY_PATH}" ./cmd/cli
write_live_config

GOEXPERIMENT= "${BINARY_PATH}" --config "${CONFIG_PATH}" >"${LOG_PATH}" 2>&1 &
PROXY_PID="$!"
wait_for_proxy

for live_provider in "${LIVE_PROVIDERS[@]}"; do
  run_text_smoke "${live_provider}"
done
