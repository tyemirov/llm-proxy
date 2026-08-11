#!/usr/bin/env bash
set -euo pipefail

usage() {
  builtin printf '%s\n' 'Usage:
  scripts/test_live_providers.sh [--preflight | --write-config <path>]

Builds the current llm-proxy binary, verifies each available provider key
through the authenticated management operation, and only then runs its live
text smoke test. The preflight mode builds a temporary static configuration and
verifies authenticated routing without making an upstream provider call.

Required environment:
  At least one provider API key, unless no-op skip behavior is desired.

Provider key variables:
  OPENAI_API_KEY
  DEEPSEEK_API_KEY
  DASHSCOPE_API_KEY
  MOONSHOT_API_KEY
  MINIMAX_API_KEY
  SILICONFLOW_API_KEY
  ZHIPU_API_KEY
  GEMINI_API_KEY
  ANTHROPIC_API_KEY
  MODEL_API_KEY
  XAI_API_KEY

Optional environment:
  DASHSCOPE_BASE_URL          Required workspace URL when DashScope is selected.
  LIVE_ENV_FILE              Path to a dotenv file to parse before discovery.
  LLM_PROXY_LIVE_PROVIDERS   Comma or space separated provider list. If set,
                             every listed provider must have its key.
  LLM_PROXY_LIVE_PORT        Local port for the temporary proxy. Default: a
                             freshly allocated loopback port.
  LLM_PROXY_LIVE_TIMEOUT     Per-request curl timeout in seconds. Default: 45.
  GO                         Go binary. Default: go.

Options:
  --preflight                Verify the disposable static config without an
                             upstream provider call.
  --write-config <path>      Write the disposable static config and exit
                             without building the proxy or calling providers.

Per-provider model overrides:
  LLM_PROXY_LIVE_OPENAI_MODEL
  LLM_PROXY_LIVE_DEEPSEEK_MODEL
  LLM_PROXY_LIVE_DASHSCOPE_MODEL
  LLM_PROXY_LIVE_MOONSHOT_MODEL
  LLM_PROXY_LIVE_MINIMAX_MODEL
  LLM_PROXY_LIVE_SILICONFLOW_MODEL
  LLM_PROXY_LIVE_ZHIPU_MODEL
  LLM_PROXY_LIVE_GEMINI_MODEL
  LLM_PROXY_LIVE_ANTHROPIC_MODEL
  LLM_PROXY_LIVE_META_MODEL
  LLM_PROXY_LIVE_XAI_MODEL'
}

env_or_default() {
  local name="$1"
  local fallback="$2"
  local value=""
  if declare -p "${name}" >/dev/null 2>&1; then
    value="${!name}"
  fi
  if [[ -n "${value}" ]]; then
    printf "%s\n" "${value}"
  else
    printf "%s\n" "${fallback}"
  fi
}

allocate_loopback_port() {
  command -v python3 >/dev/null 2>&1 || { echo "error: python3 is required to allocate a live-provider harness port" >&2; exit 1; }
  python3 -c '
import socket

listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
listener.bind(("127.0.0.1", 0))
print(listener.getsockname()[1])
listener.close()
'
}

load_env_file() {
  local env_path="$1"
  local parsed_path="${TMP_DIR}/dotenv-values"
  command -v python3 >/dev/null 2>&1 || { echo "error: python3 is required to load LIVE_ENV_FILE" >&2; exit 1; }
  python3 -c '
import ast
import pathlib
import re
import sys

path = pathlib.Path(sys.argv[1])
for line_number, raw_line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
    line = raw_line.strip()
    if not line or line.startswith("#"):
        continue
    if line.startswith("export "):
        line = line.removeprefix("export ").lstrip()
    if "=" not in line:
        raise SystemExit(f"invalid dotenv entry: {path}:{line_number}")
    name, raw_value = line.split("=", 1)
    name = name.strip()
    if re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", name) is None:
        raise SystemExit(f"invalid dotenv name: {path}:{line_number}")
    value = raw_value.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in {chr(39), chr(34)}:
        parsed_value = ast.literal_eval(value)
        if not isinstance(parsed_value, str):
            raise SystemExit(f"invalid dotenv value: {path}:{line_number}")
        value = parsed_value
    sys.stdout.buffer.write(name.encode("utf-8") + b"\0" + value.encode("utf-8") + b"\0")
' "${env_path}" >"${parsed_path}"

  local variable_name
  local variable_value
  while IFS= read -r -d '' variable_name; do
    IFS= read -r -d '' variable_value || { echo "error: invalid parsed dotenv output" >&2; exit 1; }
    if declare -p "${variable_name}" >/dev/null 2>&1; then
      continue
    fi
    printf -v "${variable_name}" '%s' "${variable_value}"
    export "${variable_name}"
  done <"${parsed_path}"
}

provider_key_variable() {
  case "$1" in
    openai) printf "%s\n" "OPENAI_API_KEY" ;;
    deepseek) printf "%s\n" "DEEPSEEK_API_KEY" ;;
    dashscope) printf "%s\n" "DASHSCOPE_API_KEY" ;;
    moonshot) printf "%s\n" "MOONSHOT_API_KEY" ;;
    minimax) printf "%s\n" "MINIMAX_API_KEY" ;;
    siliconflow) printf "%s\n" "SILICONFLOW_API_KEY" ;;
    zhipu) printf "%s\n" "ZHIPU_API_KEY" ;;
    gemini) printf "%s\n" "GEMINI_API_KEY" ;;
    anthropic) printf "%s\n" "ANTHROPIC_API_KEY" ;;
    meta) printf "%s\n" "MODEL_API_KEY" ;;
    xai) printf "%s\n" "XAI_API_KEY" ;;
    *) return 1 ;;
  esac
}

provider_model_override() {
  case "$1" in
    openai) env_or_default LLM_PROXY_LIVE_OPENAI_MODEL "" ;;
    deepseek) env_or_default LLM_PROXY_LIVE_DEEPSEEK_MODEL "" ;;
    dashscope) env_or_default LLM_PROXY_LIVE_DASHSCOPE_MODEL "" ;;
    moonshot) env_or_default LLM_PROXY_LIVE_MOONSHOT_MODEL "" ;;
    minimax) env_or_default LLM_PROXY_LIVE_MINIMAX_MODEL "" ;;
    siliconflow) env_or_default LLM_PROXY_LIVE_SILICONFLOW_MODEL "" ;;
    zhipu) env_or_default LLM_PROXY_LIVE_ZHIPU_MODEL "" ;;
    gemini) env_or_default LLM_PROXY_LIVE_GEMINI_MODEL "" ;;
    anthropic) env_or_default LLM_PROXY_LIVE_ANTHROPIC_MODEL "" ;;
    meta) env_or_default LLM_PROXY_LIVE_META_MODEL "" ;;
    xai) env_or_default LLM_PROXY_LIVE_XAI_MODEL "" ;;
    *) return 1 ;;
  esac
}

provider_default_model() {
  local provider="$1"
  python3 -c '
import json
import pathlib
import sys

profile = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")).get("profile", {})
providers = profile.get("providers", [])
matches = [candidate for candidate in providers if candidate.get("id") == sys.argv[2]]
if len(matches) != 1:
    raise SystemExit(1)
model = matches[0].get("text_default_model")
if not isinstance(model, str) or not model:
    raise SystemExit(1)
print(model)
' "${MANAGEMENT_SECRET_RESPONSE_PATH}" "${provider}"
}

provider_model() {
  local provider="$1"
  local override
  override="$(provider_model_override "${provider}")"
  if [[ -n "${override}" ]]; then
    printf "%s\n" "${override}"
    return
  fi
  provider_default_model "${provider}"
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
    MINIMAX_API_KEY \
    SILICONFLOW_API_KEY \
    ZHIPU_API_KEY \
    GEMINI_API_KEY \
    ANTHROPIC_API_KEY \
    MODEL_API_KEY \
    XAI_API_KEY; do
    if [[ -z "${!key_variable:-}" ]]; then
      export "${key_variable}=unused-${key_variable}-for-live-smoke"
    fi
  done
}

redact_log() {
  sed -E 's/(key=)[^& ]+/\1<redacted>/g; s/(api_key: ).+/\1<redacted>/g' "${LOG_PATH}" >&2 || true
}

write_static_live_config() {
  awk -v port="${PORT}" '
    BEGIN {
      provider_keys["openai"] = "OPENAI_API_KEY"
      provider_keys["deepseek"] = "DEEPSEEK_API_KEY"
      provider_keys["dashscope"] = "DASHSCOPE_API_KEY"
      provider_keys["moonshot"] = "MOONSHOT_API_KEY"
      provider_keys["minimax"] = "MINIMAX_API_KEY"
      provider_keys["siliconflow"] = "SILICONFLOW_API_KEY"
      provider_keys["zhipu"] = "ZHIPU_API_KEY"
      provider_keys["gemini"] = "GEMINI_API_KEY"
      provider_keys["anthropic"] = "ANTHROPIC_API_KEY"
      provider_keys["meta"] = "MODEL_API_KEY"
      provider_keys["xai"] = "XAI_API_KEY"
    }
    /^  port: / && replaced == 0 {
      print "  port: " port
      replaced = 1
      next
    }
    /^management:$/ {
      print "management:"
      print "  enabled: false"
      print "tenants:"
      print "  - id: live-smoke"
      print "    secret: \"${SERVICE_SECRET}\""
      print "    defaults:"
      print "      provider: openai"
      print "      model: gpt-4.1"
      print "      dictation_provider: openai"
      print "      dictation_model: gpt-4o-mini-transcribe"
      print "      system_prompt: \"\""
      in_management = 1
      next
    }
    /^providers:$/ {
      in_management = 0
      print
      next
    }
    in_management == 1 { next }
    {
      print
      if ($0 ~ /^  [[:alnum:]_]+:$/) {
        provider = $1
        sub(/:$/, "", provider)
        if (provider in provider_keys) {
          print "    api_key: \"${" provider_keys[provider] "}\""
          if (provider == "dashscope") {
            print "    base_url: \"${DASHSCOPE_BASE_URL}\""
          }
        }
      }
    }
  ' "${ROOT_DIR}/configs/config.yml" > "${CONFIG_PATH}"
}

write_managed_live_config() {
  awk -v port="${PORT}" '
    /^  port: / && replaced == 0 {
      print "  port: " port
      replaced = 1
      next
    }
    { print }
  ' "${ROOT_DIR}/configs/config.yml" >"${CONFIG_PATH}"
}

configure_live_management_environment() {
  local live_origin="http://127.0.0.1:${PORT}"
  export LLM_PROXY_MANAGEMENT_ENABLED=true
  export LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN="${live_origin}"
  export LLM_PROXY_MANAGEMENT_LOOPBACK_ORIGIN="${live_origin}"
  export LLM_PROXY_MANAGEMENT_LOCALHOST_ORIGIN="http://localhost:${PORT}"
  export LLM_PROXY_MANAGEMENT_UI_DESCRIPTION="LLM Proxy live provider verification"
  export LLM_PROXY_MANAGEMENT_ADMIN_EMAILS="[]"
  export LLM_PROXY_MANAGEMENT_TAUTH_URL="${live_origin}"
  export LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID="live-provider-verification"
  export LLM_PROXY_MANAGEMENT_GOOGLE_CLIENT_ID="live-provider-verification.apps.googleusercontent.com"
  export LLM_PROXY_MANAGEMENT_TAUTH_LOGIN_PATH="/auth/google"
  export LLM_PROXY_MANAGEMENT_TAUTH_LOGOUT_PATH="/auth/logout"
  export LLM_PROXY_MANAGEMENT_TAUTH_NONCE_PATH="/auth/nonce"
  export LLM_PROXY_MANAGEMENT_TAUTH_SESSION_PATH="/auth/session"
  export LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY
  LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY="$(python3 -c 'import secrets; print(secrets.token_urlsafe(32))')"
  export LLM_PROXY_MANAGEMENT_JWT_ISSUER="tauth"
  export LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME="llm_proxy_live_provider_session"
  export LLM_PROXY_MANAGEMENT_DATABASE_PATH="${TMP_DIR}/management.sqlite"
  export LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY
  LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY="$(python3 -c 'import base64, secrets; print(base64.b64encode(secrets.token_bytes(32)).decode("ascii"))')"
  export LLM_PROXY_MANAGEMENT_API_ORIGIN="${live_origin}"
  export LLM_PROXY_MANAGEMENT_PROXY_ORIGIN="${live_origin}"
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

initialize_live_management() {
  local account_response_path="${TMP_DIR}/management-account.json"
  local account_status
  local secret_status
  SESSION_COOKIE_PATH="${TMP_DIR}/management-cookie.txt"
  MANAGEMENT_SECRET_RESPONSE_PATH="${TMP_DIR}/management-secret.json"
  python3 -c '
import base64
import hashlib
import hmac
import json
import os
import pathlib
import sys
import time

def encoded(value):
    return base64.urlsafe_b64encode(value).rstrip(b"=")

issued_at = int(time.time())
header = encoded(json.dumps({"alg": "HS256", "typ": "JWT"}, separators=(",", ":")).encode("utf-8"))
payload = encoded(json.dumps({
    "iss": os.environ["LLM_PROXY_MANAGEMENT_JWT_ISSUER"],
    "tenant_id": os.environ["LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID"],
    "user_id": "live-provider-verification",
    "user_email": "live-provider-verification@example.invalid",
    "user_display_name": "Live provider verification",
    "iat": issued_at,
    "exp": issued_at + 3600,
}, separators=(",", ":")).encode("utf-8"))
signing_input = header + b"." + payload
signature = encoded(hmac.new(
    os.environ["LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY"].encode("utf-8"),
    signing_input,
    hashlib.sha256,
).digest())
token = (signing_input + b"." + signature).decode("ascii")
cookie_name = os.environ["LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME"]
cookie_path = pathlib.Path(sys.argv[1])
cookie_path.write_text(
    "# Netscape HTTP Cookie File\n"
    f"127.0.0.1\tFALSE\t/\tFALSE\t{issued_at + 3600}\t"
    f"{cookie_name}\t{token}\n",
    encoding="utf-8",
)
cookie_path.chmod(0o600)
' "${SESSION_COOKIE_PATH}"

  account_status="$(
    curl -sS --max-time 5 \
      --cookie "${SESSION_COOKIE_PATH}" \
      -o "${account_response_path}" \
      -w "%{http_code}" \
      "http://127.0.0.1:${PORT}/api/management/account"
  )"
  if [[ "${account_status}" != "200" ]]; then
    echo "error: live provider management account setup failed: status=${account_status}" >&2
    redact_log
    exit 1
  fi
  if ! TENANT_ID="$(python3 -c '
import json
import pathlib
import sys

account = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
tenants = account.get("tenants", [])
if len(tenants) != 1 or not isinstance(tenants[0].get("id"), str) or not tenants[0]["id"]:
    raise SystemExit(1)
print(tenants[0]["id"])
' "${account_response_path}" 2>/dev/null)"; then
    echo "error: live provider management account response was invalid" >&2
    exit 1
  fi

  secret_status="$(
    curl -sS --max-time 5 \
      --cookie "${SESSION_COOKIE_PATH}" \
      -X POST \
      -H "Content-Type: application/json" \
      --data '{}' \
      -o "${MANAGEMENT_SECRET_RESPONSE_PATH}" \
      -w "%{http_code}" \
      "http://127.0.0.1:${PORT}/api/management/tenants/${TENANT_ID}/secrets"
  )"
  if [[ "${secret_status}" != "200" ]]; then
    echo "error: live provider tenant-key setup failed: status=${secret_status}" >&2
    redact_log
    exit 1
  fi
  if ! SERVICE_SECRET="$(python3 -c '
import json
import pathlib
import sys

secret = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")).get("secret")
if not isinstance(secret, str) or not secret:
    raise SystemExit(1)
print(secret)
' "${MANAGEMENT_SECRET_RESPONSE_PATH}" 2>/dev/null)"; then
    echo "error: live provider tenant-key response was invalid" >&2
    exit 1
  fi
}

verification_failure_code() {
  local response_path="$1"
  local response_code
  response_code="$(tr -d '\r\n' <"${response_path}")"
  case "${response_code}" in
    provider_key_rejected|provider_key_verification_rate_limited|provider_key_verification_timed_out|provider_key_verification_unavailable)
      printf "%s\n" "${response_code}"
      ;;
    *)
      printf "%s\n" "provider_key_verification_unconfirmed"
      ;;
  esac
}

verify_provider_key() {
  local provider="$1"
  local key_variable
  local model
  local request_path
  local response_path
  local http_status
  key_variable="$(provider_key_variable "${provider}")"
  model="$(provider_model "${provider}")"
  request_path="${TMP_DIR}/${provider}-verification-request.json"
  response_path="${TMP_DIR}/${provider}-verification-response.json"
  python3 -c '
import json
import os
import pathlib
import sys

base_url = os.environ["DASHSCOPE_BASE_URL"] if sys.argv[4] == "dashscope" else ""
pathlib.Path(sys.argv[3]).write_text(
    json.dumps({
        "api_key": os.environ[sys.argv[1]],
        "base_url": base_url,
        "text_model": sys.argv[2],
        "system_prompt": "",
    }, separators=(",", ":")),
    encoding="utf-8",
)
' "${key_variable}" "${model}" "${request_path}" "${provider}"
  chmod 600 "${request_path}"

  http_status="$(
    curl -sS --max-time "${LIVE_TIMEOUT}" \
      --cookie "${SESSION_COOKIE_PATH}" \
      -X PUT \
      -H "Content-Type: application/json" \
      --data-binary "@${request_path}" \
      -o "${response_path}" \
      -w "%{http_code}" \
      "http://127.0.0.1:${PORT}/api/management/tenants/${TENANT_ID}/provider-keys/${provider}"
  )"
  if [[ "${http_status}" != "200" ]]; then
    echo "error: live provider verification failed: provider=${provider} model=${model} status=${http_status} error=$(verification_failure_code "${response_path}")" >&2
    redact_log
    exit 1
  fi
  echo "live provider verification passed: provider=${provider} model=${model} status=${http_status}"
}

run_text_smoke() {
  local provider="$1"
  local model
  local response_path
  local request_body
  local http_status
  local response_text
  local request_model_label
  model="$(provider_model_override "${provider}")"
  response_path="${TMP_DIR}/${provider}-response.txt"
  if [[ -n "${model}" ]]; then
    request_body="$(printf '{"prompt":"Reply with exactly OK and no punctuation.","model":"%s","web_search":false}' "${model}")"
    request_model_label="${model}"
  else
    request_body='{"prompt":"Reply with exactly OK and no punctuation.","web_search":false}'
    request_model_label="omitted"
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
    echo "error: live ${provider} smoke failed: model=${request_model_label} status=${http_status}" >&2
    redact_log
    exit 1
  fi
  echo "live provider smoke passed: provider=${provider} model=${request_model_label} status=${http_status}"
}

run_static_config_preflight() {
  local response_path="${TMP_DIR}/preflight-response.txt"
  local http_status
  http_status="$(
    curl -sS --max-time 5 \
      -o "${response_path}" \
      -w "%{http_code}" \
      "http://127.0.0.1:${PORT}/?provider=unsupported-live-preflight&prompt=ready&key=${SERVICE_SECRET}"
  )"
  if [[ "${http_status}" != "400" ]]; then
    echo "error: live provider harness preflight failed: status=${http_status}" >&2
    redact_log
    exit 1
  fi
  echo "live provider harness preflight passed: static tenant authenticated and routing rejected the unknown provider"
}

PREFLIGHT_ONLY=false
WRITE_CONFIG_PATH=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --preflight)
      PREFLIGHT_ONLY=true
      shift
      ;;
    --write-config)
      [[ $# -ge 2 ]] || { echo "error: --write-config requires a path" >&2; exit 1; }
      WRITE_CONFIG_PATH="$2"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done
if [[ "${PREFLIGHT_ONLY}" == "true" && -n "${WRITE_CONFIG_PATH}" ]]; then
  echo "error: --preflight and --write-config are mutually exclusive" >&2
  exit 1
fi

SUPPORTED_PROVIDERS=(openai deepseek dashscope moonshot minimax siliconflow zhipu gemini anthropic meta xai)
LIVE_PROVIDERS=()
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
PROXY_PID=""

cleanup() {
  local exit_status=$?
  trap - EXIT HUP INT TERM
  if [[ -n "${PROXY_PID}" ]] && kill -0 "${PROXY_PID}" >/dev/null 2>&1; then
    kill -TERM "${PROXY_PID}" >/dev/null 2>&1 || true
    wait "${PROXY_PID}" >/dev/null 2>&1 || true
  fi
  rm -rf "${TMP_DIR}"
  return "${exit_status}"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

if [[ -n "${LIVE_ENV_FILE:-}" ]]; then
  [[ -f "${LIVE_ENV_FILE}" ]] || { echo "error: LIVE_ENV_FILE not found: ${LIVE_ENV_FILE}" >&2; exit 1; }
  load_env_file "${LIVE_ENV_FILE}"
fi

if [[ "${PREFLIGHT_ONLY}" != "true" && -z "${WRITE_CONFIG_PATH}" ]]; then
  discover_live_providers
  if [[ "${#LIVE_PROVIDERS[@]}" -eq 0 ]]; then
    echo "live provider smoke skipped: no provider API keys found"
    exit 0
  fi
fi
if [[ " ${LIVE_PROVIDERS[*]} " == *" dashscope "* && -z "${DASHSCOPE_BASE_URL:-}" ]]; then
  echo "error: DASHSCOPE_BASE_URL is required when DashScope is selected" >&2
  exit 1
fi
export DASHSCOPE_BASE_URL="${DASHSCOPE_BASE_URL:-https://dashscope.invalid}"

if [[ -n "${LLM_PROXY_LIVE_PORT:-}" ]]; then
  PORT="${LLM_PROXY_LIVE_PORT}"
else
  PORT="$(allocate_loopback_port)"
fi
LIVE_TIMEOUT="$(env_or_default LLM_PROXY_LIVE_TIMEOUT 45)"
if [[ -n "${WRITE_CONFIG_PATH}" ]]; then
  mkdir -p "$(dirname "${WRITE_CONFIG_PATH}")"
  CONFIG_PATH="$(cd "$(dirname "${WRITE_CONFIG_PATH}")" && pwd)/$(basename "${WRITE_CONFIG_PATH}")"
else
  CONFIG_PATH="${TMP_DIR}/config.yml"
fi
LOG_PATH="${TMP_DIR}/llm-proxy.log"
export LLM_PROXY_LIVE_PORT="${PORT}"
if [[ "${PREFLIGHT_ONLY}" == "true" || -n "${WRITE_CONFIG_PATH}" ]]; then
  export SERVICE_SECRET="${SERVICE_SECRET:-live-service-secret}"
  export_unused_provider_placeholders
  write_static_live_config
else
  configure_live_management_environment
  write_managed_live_config
fi

if [[ -n "${WRITE_CONFIG_PATH}" ]]; then
  echo "isolated live provider config written: ${CONFIG_PATH}"
  exit 0
fi

GO_BIN="$(env_or_default GO go)"
BINARY_PATH="${TMP_DIR}/llm-proxy-live"

if command -v lsof >/dev/null 2>&1 && lsof -tiTCP:"${PORT}" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "error: port ${PORT} is already in use; set LLM_PROXY_LIVE_PORT to a free port" >&2
  exit 1
fi

cd "${ROOT_DIR}"
GOEXPERIMENT= CGO_ENABLED=0 "${GO_BIN}" build -o "${BINARY_PATH}" ./cmd/cli

GOEXPERIMENT= "${BINARY_PATH}" --config "${CONFIG_PATH}" >"${LOG_PATH}" 2>&1 &
PROXY_PID="$!"
wait_for_proxy

if [[ "${PREFLIGHT_ONLY}" == "true" ]]; then
  run_static_config_preflight
  exit 0
fi

initialize_live_management
for live_provider in "${LIVE_PROVIDERS[@]}"; do
  verify_provider_key "${live_provider}"
  run_text_smoke "${live_provider}"
done
