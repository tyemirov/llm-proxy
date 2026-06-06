#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  scripts/test_live_gemini.sh

Runs the current llm-proxy binary against the live Gemini generateContent API.

Required environment:
  GEMINI_API_KEY
  SERVICE_SECRET

Optional environment:
  LIVE_ENV_FILE           Path to an env file to source before validation and config interpolation.
  LLM_PROXY_LIVE_PORT     Local port for the temporary proxy. Default: 18080.
  LLM_PROXY_LIVE_MODEL    Gemini model to test. Default: gemini-3.5-flash.
  GO                      Go binary. Default: go.
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

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  usage
  exit 0
fi

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

[[ -n "${GEMINI_API_KEY:-}" ]] || { echo "error: GEMINI_API_KEY is required for live Gemini test" >&2; exit 1; }
[[ -n "${SERVICE_SECRET:-}" ]] || { echo "error: SERVICE_SECRET is required for live Gemini test" >&2; exit 1; }

GO_BIN="$(env_or_default GO go)"
PORT="$(env_or_default LLM_PROXY_LIVE_PORT 18080)"
MODEL="$(env_or_default LLM_PROXY_LIVE_MODEL gemini-3.5-flash)"
BINARY_PATH="${TMP_DIR}/llm-proxy-live"
CONFIG_PATH="${TMP_DIR}/config.yml"
LOG_PATH="${TMP_DIR}/llm-proxy.log"
RESPONSE_PATH="${TMP_DIR}/response.txt"
export LLM_PROXY_LIVE_PORT="${PORT}"
export OPENAI_API_KEY="${OPENAI_API_KEY:-unused-openai-key-for-gemini-live-smoke}"

if command -v lsof >/dev/null 2>&1 && lsof -tiTCP:"${PORT}" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "error: port ${PORT} is already in use; set LLM_PROXY_LIVE_PORT to a free port" >&2
  exit 1
fi

cd "${ROOT_DIR}"
GOEXPERIMENT= CGO_ENABLED=0 "${GO_BIN}" build -o "${BINARY_PATH}" ./cmd/cli

cat > "${CONFIG_PATH}" <<'CONFIG'
server:
  port: ${LLM_PROXY_LIVE_PORT}
  log_level: info
tenants:
  - id: gemini-live
    secret: "${SERVICE_SECRET}"
    defaults:
      provider: gemini
      model: gemini-3.5-flash
      dictation_provider: openai
providers:
  openai:
    api_key: "${OPENAI_API_KEY}"
  gemini:
    api_key: "${GEMINI_API_KEY}"
CONFIG

GOEXPERIMENT= "${BINARY_PATH}" --config "${CONFIG_PATH}" >"${LOG_PATH}" 2>&1 &
PROXY_PID="$!"

READY="false"
for _ in {1..50}; do
  readiness_status="$(curl -sS --max-time 1 -o /dev/null -w "%{http_code}" "http://127.0.0.1:${PORT}/?prompt=ready" 2>/dev/null || true)"
  if [[ "${readiness_status}" == "403" ]]; then
    READY="true"
    break
  fi
  if ! kill -0 "${PROXY_PID}" >/dev/null 2>&1; then
    echo "error: live proxy exited before readiness" >&2
    sed -E 's/(key=)[^& ]+/\1<redacted>/g' "${LOG_PATH}" >&2 || true
    exit 1
  fi
  sleep 0.1
done
if [[ "${READY}" != "true" ]]; then
  echo "error: live proxy did not become ready on port ${PORT}" >&2
  sed -E 's/(key=)[^& ]+/\1<redacted>/g' "${LOG_PATH}" >&2 || true
  exit 1
fi

HTTP_STATUS="$(
  curl -sS --max-time 30 \
    -X POST \
    -H "Content-Type: application/json" \
    --data "{\"prompt\":\"Return only OK.\",\"model\":\"${MODEL}\",\"web_search\":false}" \
    -o "${RESPONSE_PATH}" \
    -w "%{http_code}" \
    "http://127.0.0.1:${PORT}/?provider=gemini&format=text/plain&key=${SERVICE_SECRET}"
)"

RESPONSE_TEXT="$(tr -d '\r\n' < "${RESPONSE_PATH}")"
if [[ "${HTTP_STATUS}" != "200" || "${RESPONSE_TEXT}" != "OK" ]]; then
  echo "error: live Gemini smoke failed: status=${HTTP_STATUS} response=${RESPONSE_TEXT}" >&2
  sed -E 's/(key=)[^& ]+/\1<redacted>/g' "${LOG_PATH}" >&2 || true
  exit 1
fi

echo "live Gemini smoke passed: model=${MODEL} status=${HTTP_STATUS} response=${RESPONSE_TEXT}"
