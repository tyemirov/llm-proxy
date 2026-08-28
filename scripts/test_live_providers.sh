#!/usr/bin/env bash
set -euo pipefail

usage() {
  builtin printf '%s\n' 'Usage:
  scripts/test_live_providers.sh [--gemini-candidates | --media | --preflight | --write-config <path>] [--existing-local-origin <origin>]

Builds the current llm-proxy binary, verifies each available provider key
through the authenticated management operation, and only then runs its live
text smoke test. Provider identifiers, field environment bindings, and image
routes come from configs/providers.yml. The preflight mode builds a temporary
managed configuration and verifies authenticated routing through a local provider connection.

Required environment:
  At least one provider API key, unless no-op skip behavior is desired.

Optional environment:
  LIVE_ENV_FILE              Path to a dotenv file to parse before discovery.
  LLM_PROXY_LIVE_PROVIDERS   Comma or space separated provider list. If set,
                             every listed provider must have its key.
  LLM_PROXY_LIVE_ALL_MODELS  Exact true or false. When true, verify and smoke
                             every selected provider text model discovered from
                             the public catalog. Default: false.
  LLM_PROXY_LIVE_REASONING_MATRIX
                             Exact true or false. When true, send one omitted
                             reasoning-effort request and one request for each
                             effort published by the selected exact route.
                             Default: false. Gemini candidate default: true.
  LLM_PROXY_LIVE_PORT        Local port for the temporary proxy. Default: a
                             freshly allocated loopback port.
  LLM_PROXY_LIVE_MANAGEMENT_ENV_FILE
                             Scoped management environment for an existing
                             local origin. Required with --existing-local-origin.
  LLM_PROXY_LIVE_TIMEOUT     Per-request curl timeout in seconds. Default: 45.
  GO                         Go binary. Default: go.

Options:
  --gemini-candidates        Run paid direct acceptance for Gemini 3.6 Flash
                             and Gemini 3.7 Flash. This mode does not register
                             either candidate in the public provider catalog.

  --media                    Run paid image routes from the public catalog.
                             LLM_PROXY_LIVE_PROVIDERS can select a subset.
                             LLM_PROXY_LIVE_ALL_MODELS=true runs every selected
                             provider image route.

  --preflight                Verify the disposable managed config without an
                             external provider call.
  --write-config <path>      Write the disposable managed config and exit
                             without building the proxy or calling providers.
  --existing-local-origin <origin>
                             Use an existing HTTP loopback API. Do not start a
                             host proxy process. The local orchestration owner
                             must remove its disposable management state.

Provider fields use the environment bindings declared in configs/providers.yml.
Per-provider model overrides use LLM_PROXY_LIVE_<PROVIDER_ID>_MODEL, with the
provider identifier converted to uppercase and hyphens converted to underscores.'
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
  local allowed_names_path="${2:-}"
  local parsed_path="${TMP_DIR}/dotenv-values"
  command -v python3 >/dev/null 2>&1 || { echo "error: python3 is required to load LIVE_ENV_FILE" >&2; exit 1; }
  python3 -c '
import ast
import pathlib
import re
import sys

path = pathlib.Path(sys.argv[1])
allowed_names = None
if sys.argv[2]:
    allowed_names = set(pathlib.Path(sys.argv[2]).read_text(encoding="utf-8").splitlines())
seen_names = set()
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
    if name in seen_names:
        raise SystemExit(f"duplicate dotenv name: {path}:{line_number}: {name}")
    seen_names.add(name)
    if allowed_names is not None and name not in allowed_names:
        continue
    value = raw_value.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in {chr(39), chr(34)}:
        parsed_value = ast.literal_eval(value)
        if not isinstance(parsed_value, str):
            raise SystemExit(f"invalid dotenv value: {path}:{line_number}")
        value = parsed_value
    sys.stdout.buffer.write(name.encode("utf-8") + b"\0" + value.encode("utf-8") + b"\0")
' "${env_path}" "${allowed_names_path}" >"${parsed_path}"

  local variable_name
  local variable_value
  while IFS= read -r -d '' variable_name; do
    IFS= read -r -d '' variable_value || { echo "error: invalid parsed dotenv output" >&2; exit 1; }
    printf -v "${variable_name}" '%s' "${variable_value}"
    export "${variable_name}"
  done <"${parsed_path}"
}

provider_model_override() {
  local provider_name
  local override_name
  provider_name="$(printf '%s' "$1" | tr '[:lower:]-' '[:upper:]_')"
  override_name="LLM_PROXY_LIVE_${provider_name}_MODEL"
  env_or_default "${override_name}" ""
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

provider_image_model() {
  local provider="$1"
  local verified_model
  verified_model="$(provider_model "${provider}")"
  python3 -c '
import json
import pathlib
import sys

catalog = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
offerings = catalog.get("offerings", [])
candidates = sorted({
    offering.get("model")
    for offering in offerings
    if offering.get("provider") == sys.argv[2]
    and "image_input" in offering.get("capabilities", [])
    and isinstance(offering.get("model"), str)
    and offering.get("model")
})
if sys.argv[3] in candidates:
    print(sys.argv[3])
elif len(candidates) == 1:
    print(candidates[0])
else:
    raise SystemExit(1)
' "${PUBLIC_CAPABILITIES_RESPONSE_PATH}" "${provider}" "${verified_model}"
}

provider_catalog_text_models() {
  local provider="$1"
  python3 -c '
import json
import pathlib
import sys

catalog = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
models = sorted({
    offering.get("model")
    for offering in catalog.get("offerings", [])
    if offering.get("provider") == sys.argv[2]
    and "text" in offering.get("capabilities", [])
    and isinstance(offering.get("model"), str)
    and offering.get("model")
})
if not models:
    raise SystemExit(1)
print("\n".join(models))
' "${PUBLIC_CAPABILITIES_RESPONSE_PATH}" "${provider}"
}

provider_catalog_reasoning_efforts() {
  local provider="$1"
  local model="$2"
  python3 -c '
import json
import pathlib
import sys

catalog = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
matches = [
    offering
    for offering in catalog.get("offerings", [])
    if offering.get("provider") == sys.argv[2]
    and offering.get("model") == sys.argv[3]
    and "text" in offering.get("capabilities", [])
]
if len(matches) != 1:
    raise SystemExit(1)
efforts = matches[0].get("reasoning_efforts", [])
if not isinstance(efforts, list) or any(not isinstance(effort, str) or not effort for effort in efforts):
    raise SystemExit(1)
print("\n".join(efforts))
' "${PUBLIC_CAPABILITIES_RESPONSE_PATH}" "${provider}" "${model}"
}

provider_catalog_image_models() {
	local provider="$1"
	python3 -c '
import json
import pathlib
import sys

catalog = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
models = sorted({
    offering.get("model")
    for offering in catalog.get("offerings", [])
    if offering.get("provider") == sys.argv[2]
    and "image_input" in offering.get("capabilities", [])
    and isinstance(offering.get("model"), str)
    and offering.get("model")
})
if not models:
    raise SystemExit(1)
print("\n".join(models))
' "${PUBLIC_CAPABILITIES_RESPONSE_PATH}" "${provider}"
}

validate_provider_name() {
  python3 -c '
import json
import pathlib
import sys

discovery = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
matches = [provider for provider in discovery.get("providers", []) if provider.get("id") == sys.argv[2]]
raise SystemExit(0 if len(matches) == 1 else 1)
' "${PROVIDER_DISCOVERY_PATH}" "$1"
}

provider_missing_environment() {
  local provider="$1"
  python3 -c '
import json
import os
import pathlib
import sys

discovery = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
matches = [candidate for candidate in discovery.get("providers", []) if candidate.get("id") == sys.argv[2]]
if len(matches) != 1:
    print("catalog_provider_invalid")
    raise SystemExit(0)
fields = matches[0].get("fields")
if not isinstance(fields, list) or not fields:
    print("catalog_fields_invalid")
    raise SystemExit(0)
for field in fields:
    if not isinstance(field, dict) or not isinstance(field.get("required"), bool):
        print("catalog_field_invalid")
        continue
    if field["required"] is not True:
        continue
    identifier = field.get("id")
    environment = field.get("environment")
    if not isinstance(identifier, str) or not identifier or not isinstance(environment, str) or not environment:
        print("catalog_field_binding_invalid")
    elif not os.environ.get(environment):
        print(environment)
' "${PROVIDER_DISCOVERY_PATH}" "${provider}"
}

provider_has_connection() {
  [[ -z "$(provider_missing_environment "$1")" ]]
}

catalog_provider_ids() {
  python3 -c '
import json
import pathlib
import sys

discovery = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
providers = discovery.get("providers", [])
if not isinstance(providers, list) or not providers:
    raise SystemExit(1)
for provider in providers:
    identifier = provider.get("id")
    if not isinstance(identifier, str) or not identifier:
        raise SystemExit(1)
    print(identifier)
' "${PROVIDER_DISCOVERY_PATH}"
}

catalog_provider_environment_names() {
  python3 -c '
import json
import pathlib
import sys

discovery = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
providers = discovery.get("providers")
if not isinstance(providers, list) or not providers:
    raise SystemExit(1)
environment_names = set()
for provider in providers:
    fields = provider.get("fields") if isinstance(provider, dict) else None
    if not isinstance(fields, list) or not fields:
        raise SystemExit(1)
    for field in fields:
        environment = field.get("environment") if isinstance(field, dict) else None
        if not isinstance(environment, str) or not environment:
            raise SystemExit(1)
        environment_names.add(environment)
print("\n".join(sorted(environment_names)))
' "${PROVIDER_DISCOVERY_PATH}"
}

load_catalog_provider_environment() {
  local environment_names_path="${TMP_DIR}/catalog-provider-environment-names"
  local environment_name
  catalog_provider_environment_names >"${environment_names_path}"
  while IFS= read -r environment_name; do
    [[ -n "${environment_name}" ]] || continue
    unset "${environment_name}"
  done <"${environment_names_path}"
  load_env_file "${LIVE_ENV_FILE}" "${environment_names_path}"
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
      if ! provider_has_connection "${selected_provider}"; then
        echo "error: ${selected_provider} requested but required catalog environment is not set: $(provider_missing_environment "${selected_provider}")" >&2
        exit 1
      fi
      LIVE_PROVIDERS+=("${selected_provider}")
    done
    return
  fi

  while IFS= read -r selected_provider; do
    [[ -n "${selected_provider}" ]] || continue
    if provider_has_connection "${selected_provider}"; then
      LIVE_PROVIDERS+=("${selected_provider}")
    fi
  done < <(catalog_provider_ids)
}

redact_log() {
  sed -E 's/(key=)[^& ]+/\1<redacted>/g; s/(api_key: ).+/\1<redacted>/g' "${LOG_PATH}" >&2 || true
  if [[ -n "${PREFLIGHT_PROVIDER_LOG_PATH:-}" && -f "${PREFLIGHT_PROVIDER_LOG_PATH}" ]]; then
    sed -E 's/(Bearer )[A-Za-z0-9._~-]+/\1<redacted>/g' "${PREFLIGHT_PROVIDER_LOG_PATH}" >&2 || true
  fi
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
  cp "${ROOT_DIR}/configs/providers.yml" "${PROVIDER_CATALOG_PATH}"
  if [[ -n "${PREFLIGHT_PROVIDER_URL:-}" ]]; then
    python3 -c '
import pathlib
import sys

catalog_path = pathlib.Path(sys.argv[1])
document = catalog_path.read_text(encoding="utf-8")
needle = "            default_base_url: https://api.openai.com/v1\n            path: /responses"
replacement = f"            default_base_url: {sys.argv[2]}\n            path: /responses"
if document.count(needle) != 1:
    raise SystemExit("OpenAI preflight transport is not unique")
catalog_path.write_text(document.replace(needle, replacement, 1), encoding="utf-8")
' "${PROVIDER_CATALOG_PATH}" "${PREFLIGHT_PROVIDER_URL}"
  fi
}

start_managed_preflight_provider() {
  local ready_path="${TMP_DIR}/managed-preflight-provider.ready"
  PREFLIGHT_PROVIDER_LOG_PATH="${TMP_DIR}/managed-preflight-provider.log"
  export PREFLIGHT_PROVIDER_API_KEY
  PREFLIGHT_PROVIDER_API_KEY="$(python3 -c 'import secrets; print(secrets.token_urlsafe(32))')"
  python3 -c '
from http.server import BaseHTTPRequestHandler, HTTPServer
import json
import os
import pathlib
import sys

expected_prompts = (
    "Verify this provider credential.",
    "Reply with exactly OK and no punctuation.",
)
expected_key = os.environ["PREFLIGHT_PROVIDER_API_KEY"]
observed_model = None
request_count = 0
request_failed = False

class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        global observed_model, request_count, request_failed
        response_status = 200
        response_body = {"id": "managed-preflight-verification", "status": "completed"}
        try:
            content_length = int(self.headers.get("Content-Length", ""))
            payload = json.loads(self.rfile.read(content_length))
            payload_text = json.dumps(payload, separators=(",", ":"))
            model = payload.get("model")
            request_valid = (
                request_count < len(expected_prompts)
                and self.path == "/responses"
                and self.headers.get("Authorization") == f"Bearer {expected_key}"
                and self.headers.get_content_type() == "application/json"
                and isinstance(model, str)
                and model != ""
                and expected_prompts[request_count] in payload_text
                and (observed_model is None or model == observed_model)
            )
        except (json.JSONDecodeError, TypeError, ValueError):
            request_valid = False
            model = None
        if not request_valid:
            request_failed = True
            response_status = 400
            response_body = {"error": "invalid managed preflight provider request"}
        elif request_count == 0:
            observed_model = model
        else:
            response_body = {
                "id": "managed-preflight-route",
                "status": "completed",
                "output": [{
                    "type": "message",
                    "role": "assistant",
                    "content": [{"type": "output_text", "text": "OK"}],
                }],
            }
        request_count += 1
        response_bytes = json.dumps(response_body, separators=(",", ":")).encode("utf-8")
        self.send_response(response_status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(response_bytes)))
        self.end_headers()
        self.wfile.write(response_bytes)

    def log_message(self, _format, *_arguments):
        return

server = HTTPServer(("127.0.0.1", int(sys.argv[1])), Handler)
pathlib.Path(sys.argv[2]).touch()
for _ in expected_prompts:
    server.handle_request()
    if request_failed:
        raise SystemExit("managed preflight provider rejected a request")
server.server_close()
if request_count != len(expected_prompts):
    raise SystemExit("managed preflight provider received an invalid request count")
' "${PREFLIGHT_PROVIDER_PORT}" "${ready_path}" >"${PREFLIGHT_PROVIDER_LOG_PATH}" 2>&1 &
  PREFLIGHT_PROVIDER_PID="$!"

  for _ in {1..50}; do
    if [[ -f "${ready_path}" ]]; then
      return
    fi
    if ! kill -0 "${PREFLIGHT_PROVIDER_PID}" >/dev/null 2>&1; then
      echo "error: managed preflight provider exited before readiness" >&2
      redact_log
      exit 1
    fi
    sleep 0.02
  done
  echo "error: managed preflight provider did not become ready" >&2
  redact_log
  exit 1
}

configure_live_management_environment() {
  export LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN="${LIVE_ORIGIN}"
  export LLM_PROXY_MANAGEMENT_LOOPBACK_ORIGIN="${LIVE_ORIGIN}"
  export LLM_PROXY_MANAGEMENT_LOCALHOST_ORIGIN="http://localhost:${PORT}"
  export LLM_PROXY_MANAGEMENT_UI_DESCRIPTION="LLM Proxy live provider verification"
  export LLM_PROXY_MANAGEMENT_ADMIN_EMAILS="[]"
  export LLM_PROXY_MANAGEMENT_TAUTH_URL="${LIVE_ORIGIN}"
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
  export LLM_PROXY_MANAGEMENT_API_ORIGIN="${LIVE_ORIGIN}"
  export LLM_PROXY_MANAGEMENT_PROXY_ORIGIN="${LIVE_ORIGIN}"
}

existing_local_origin_port() {
  python3 -c '
import sys
import urllib.parse

origin = urllib.parse.urlsplit(sys.argv[1])
try:
    port = origin.port
except ValueError:
    raise SystemExit(1)
if (
    origin.scheme != "http"
    or origin.hostname != "127.0.0.1"
    or port is None
    or origin.username is not None
    or origin.password is not None
    or origin.path not in {"", "/"}
    or origin.query
    or origin.fragment
):
    raise SystemExit(1)
print(port)
' "$1"
}

load_existing_management_environment() {
  local management_environment_path="${LLM_PROXY_LIVE_MANAGEMENT_ENV_FILE:-}"
  [[ -n "${management_environment_path}" ]] || {
    echo "error: LLM_PROXY_LIVE_MANAGEMENT_ENV_FILE is required with --existing-local-origin" >&2
    exit 1
  }
  [[ -f "${management_environment_path}" ]] || {
    echo "error: local management environment not found: ${management_environment_path}" >&2
    exit 1
  }
  unset \
    LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY \
    LLM_PROXY_MANAGEMENT_JWT_ISSUER \
    LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME \
    LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID
  load_env_file "${management_environment_path}"
  for required_name in \
    LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY \
    LLM_PROXY_MANAGEMENT_JWT_ISSUER \
    LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME \
    LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID; do
    [[ -n "${!required_name:-}" ]] || {
      echo "error: ${management_environment_path} must define ${required_name}" >&2
      exit 1
    }
  done
}

wait_for_proxy() {
  local readiness_status
  for _ in {1..50}; do
    readiness_status="$(curl -sS --max-time 1 -o /dev/null -w "%{http_code}" "${LIVE_ORIGIN}/?prompt=ready" 2>/dev/null || true)"
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

wait_for_existing_local_origin() {
  local readiness_status
  readiness_status="$(curl -sS --max-time 5 -o /dev/null -w "%{http_code}" "${LIVE_ORIGIN}/?prompt=ready" 2>/dev/null || true)"
  if [[ "${readiness_status}" != "403" ]]; then
    echo "error: existing local API is not ready: origin=${LIVE_ORIGIN} expected_status=403 status=${readiness_status:-transport_error}" >&2
    exit 1
  fi
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
      "${LIVE_ORIGIN}/api/management/account"
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
      "${LIVE_ORIGIN}/api/management/tenants/${TENANT_ID}/secrets"
  )"
  if [[ "${secret_status}" != "200" ]]; then
    echo "error: live provider tenant-key setup failed: status=${secret_status}" >&2
    redact_log
    exit 1
  fi
  if ! LLM_PROXY_SECRET="$(python3 -c '
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
  local requested_model="${2:-}"
  local model
  local request_path
  local response_path
  local http_status
  if [[ -n "${requested_model}" ]]; then
    model="${requested_model}"
  else
    model="$(provider_model "${provider}")"
  fi
  request_path="${TMP_DIR}/${provider}-verification-request.json"
  response_path="${TMP_DIR}/${provider}-verification-response.json"
  python3 -c '
import json
import os
import pathlib
import sys

discovery = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
matches = [provider for provider in discovery.get("providers", []) if provider.get("id") == sys.argv[2]]
if len(matches) != 1:
    raise SystemExit(1)
fields = {}
for field in matches[0].get("fields", []):
    environment = field.get("environment")
    value = os.environ.get(environment, "") if isinstance(environment, str) and environment else ""
    if field.get("required") is True and not value:
        raise SystemExit(1)
    if value:
        fields[field["id"]] = value
pathlib.Path(sys.argv[3]).write_text(
    json.dumps({
        "fields": fields,
        "text_model": sys.argv[4],
        "system_prompt": "",
    }, separators=(",", ":")),
    encoding="utf-8",
)
' "${PROVIDER_DISCOVERY_PATH}" "${provider}" "${request_path}" "${model}"
  chmod 600 "${request_path}"

  http_status="$(
    curl -sS --max-time "${LIVE_TIMEOUT}" \
      --cookie "${SESSION_COOKIE_PATH}" \
      -X PUT \
      -H "Content-Type: application/json" \
      --data-binary "@${request_path}" \
      -o "${response_path}" \
      -w "%{http_code}" \
      "${LIVE_ORIGIN}/api/management/tenants/${TENANT_ID}/provider-connections/${provider}"
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
  local requested_model="${2:-}"
  local reasoning_effort="${3:-}"
  local model
  local response_path
  local request_body
  local http_status
  local response_text
  local request_model_label
  local reasoning_effort_label="omitted"
  if [[ -n "${requested_model}" ]]; then
    model="${requested_model}"
  else
    model="$(provider_model_override "${provider}")"
  fi
  response_path="${TMP_DIR}/${provider}-response.txt"
  if [[ -n "${model}" ]]; then
    if [[ -n "${reasoning_effort}" ]]; then
      request_body="$(printf '{"messages":[{"role":"user","content":"Reply with exactly OK and no punctuation."}],"model":"%s","reasoning_effort":"%s","web_search":false}' "${model}" "${reasoning_effort}")"
      reasoning_effort_label="${reasoning_effort}"
    else
      request_body="$(printf '{"messages":[{"role":"user","content":"Reply with exactly OK and no punctuation."}],"model":"%s","web_search":false}' "${model}")"
    fi
    request_model_label="${model}"
  else
    request_body='{"messages":[{"role":"user","content":"Reply with exactly OK and no punctuation."}],"web_search":false}'
    request_model_label="omitted"
  fi

  http_status="$(
    curl -sS --max-time "${LIVE_TIMEOUT}" \
      -X POST \
      -H "Content-Type: application/json" \
      --data "${request_body}" \
      -o "${response_path}" \
      -w "%{http_code}" \
      "${LIVE_ORIGIN}/v2?provider=${provider}&format=text/plain&key=${LLM_PROXY_SECRET}"
  )"

  response_text="$(tr -d '\r\n' < "${response_path}" | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//')"
  if [[ "${http_status}" != "200" || "${response_text}" != "OK" ]]; then
    echo "error: live ${provider} smoke failed: model=${request_model_label} status=${http_status}" >&2
    redact_log
    exit 1
  fi
  echo "live provider smoke passed: provider=${provider} model=${request_model_label} status=${http_status} reasoning_effort=${reasoning_effort_label}"
}

run_text_smoke_matrix() {
  local provider="$1"
  local requested_model="${2:-}"
  local model
  local reasoning_efforts
  local reasoning_effort
  if [[ -n "${requested_model}" ]]; then
    model="${requested_model}"
  else
    model="$(provider_model "${provider}")"
  fi
  run_text_smoke "${provider}" "${model}"
  if ! reasoning_efforts="$(provider_catalog_reasoning_efforts "${provider}" "${model}")"; then
    echo "error: live provider reasoning capabilities are unavailable: provider=${provider} model=${model}" >&2
    exit 1
  fi
  while IFS= read -r reasoning_effort; do
    [[ -n "${reasoning_effort}" ]] || continue
    run_text_smoke "${provider}" "${model}" "${reasoning_effort}"
  done <<<"${reasoning_efforts}"
}

fetch_public_capabilities() {
  local http_status
  http_status="$(
    curl -sS --max-time 5 \
      -o "${PUBLIC_CAPABILITIES_RESPONSE_PATH}" \
      -w "%{http_code}" \
      "${LIVE_ORIGIN}/api/public/capabilities"
  )"
  if [[ "${http_status}" != "200" ]] || ! python3 -c '
import json
import pathlib
import sys

catalog = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
if not isinstance(catalog.get("offerings"), list):
    raise SystemExit(1)
' "${PUBLIC_CAPABILITIES_RESPONSE_PATH}"; then
    echo "error: live provider capability discovery failed: status=${http_status}" >&2
    redact_log
    exit 1
  fi
}

write_image_smoke_request() {
  local model="$1"
  local request_path="$2"
  python3 -c '
import base64
import binascii
import hashlib
import json
import pathlib
import struct
import sys
import zlib

def png_chunk(chunk_type, payload):
    return struct.pack(">I", len(payload)) + chunk_type + payload + struct.pack(
        ">I", binascii.crc32(chunk_type + payload) & 0xFFFFFFFF
    )

width = 256
height = 256
scanlines = b"".join(b"\x00" + (b"\xff\x00\x00" * width) for _ in range(height))
image = (
    b"\x89PNG\r\n\x1a\n"
    + png_chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0))
    + png_chunk(b"IDAT", zlib.compress(scanlines, level=9))
    + png_chunk(b"IEND", b"")
)
payload = {
    "messages": [{
        "role": "user",
        "content": "Identify the solid color in this image. Reply with exactly RED and no punctuation.",
        "attachments": [{
            "type": "image",
            "mime_type": "image/png",
            "data": base64.b64encode(image).decode("ascii"),
            "sha256": hashlib.sha256(image).hexdigest(),
        }],
    }],
    "model": sys.argv[1],
    "web_search": False,
}
pathlib.Path(sys.argv[2]).write_text(
    json.dumps(payload, separators=(",", ":")),
    encoding="utf-8",
)
' "${model}" "${request_path}"
}

validated_response_request_id() {
  local headers_path="$1"
  python3 -c '
import re
import sys

with open(sys.argv[1], encoding="utf-8", errors="strict") as header_file:
    values = [
        line.split(":", 1)[1].strip()
        for line in header_file
        if line.lower().startswith("x-llm-proxy-request-id:")
    ]
if len(values) != 1 or re.fullmatch(r"[A-Z2-7]{26,}", values[0]) is None:
    raise SystemExit(1)
' "${headers_path}"
}

run_image_smoke() {
  local provider="$1"
  local model="$2"
  local request_path="${TMP_DIR}/${provider}-image-request.json"
  local headers_path="${TMP_DIR}/${provider}-image-headers.txt"
  local response_path="${TMP_DIR}/${provider}-image-response.txt"
  local http_status
  local response_text
  write_image_smoke_request "${model}" "${request_path}"

  http_status="$(
    curl -sS --max-time "${LIVE_TIMEOUT}" \
      -X POST \
      -D "${headers_path}" \
      -H "Content-Type: application/json" \
      --data-binary "@${request_path}" \
      -o "${response_path}" \
      -w "%{http_code}" \
      "${LIVE_ORIGIN}/v2?provider=${provider}&format=text/plain&key=${LLM_PROXY_SECRET}"
  )"

  response_text="$(tr -d '\r\n' <"${response_path}" | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//')"
  if [[ "${http_status}" != "200" || "${response_text}" != "RED" ]] || ! validated_response_request_id "${headers_path}"; then
    echo "error: live provider image smoke failed: provider=${provider} model=${model} status=${http_status}" >&2
    redact_log
    exit 1
  fi
  echo "live provider image smoke passed: provider=${provider} model=${model} status=${http_status}"
}

run_managed_config_preflight() {
  OPENAI_API_KEY="${PREFLIGHT_PROVIDER_API_KEY}"
  export OPENAI_API_KEY
  verify_provider_key openai
  run_text_smoke openai
  if ! wait "${PREFLIGHT_PROVIDER_PID}"; then
    echo "error: managed preflight provider did not observe the required requests" >&2
    redact_log
    exit 1
  fi
  PREFLIGHT_PROVIDER_PID=""
  echo "live provider harness preflight passed: saved managed provider key reloaded and routed through the local provider"
}

PREFLIGHT_ONLY=false
MEDIA_ONLY=false
GEMINI_CANDIDATES_ONLY=false
WRITE_CONFIG_PATH=""
EXISTING_LOCAL_ORIGIN=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --gemini-candidates)
      GEMINI_CANDIDATES_ONLY=true
      shift
      ;;
    --media)
      MEDIA_ONLY=true
      shift
      ;;
    --preflight)
      PREFLIGHT_ONLY=true
      shift
      ;;
    --write-config)
      [[ $# -ge 2 ]] || { echo "error: --write-config requires a path" >&2; exit 1; }
      WRITE_CONFIG_PATH="$2"
      shift 2
      ;;
    --existing-local-origin)
      [[ $# -ge 2 ]] || { echo "error: --existing-local-origin requires an origin" >&2; exit 1; }
      EXISTING_LOCAL_ORIGIN="$2"
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
if [[ "${MEDIA_ONLY}" == "true" && ( "${PREFLIGHT_ONLY}" == "true" || "${GEMINI_CANDIDATES_ONLY}" == "true" || -n "${WRITE_CONFIG_PATH}" ) ]] ||
  [[ "${PREFLIGHT_ONLY}" == "true" && ( "${GEMINI_CANDIDATES_ONLY}" == "true" || -n "${WRITE_CONFIG_PATH}" ) ]] ||
  [[ "${GEMINI_CANDIDATES_ONLY}" == "true" && -n "${WRITE_CONFIG_PATH}" ]] ||
  [[ -n "${EXISTING_LOCAL_ORIGIN}" && ( "${PREFLIGHT_ONLY}" == "true" || "${GEMINI_CANDIDATES_ONLY}" == "true" || -n "${WRITE_CONFIG_PATH}" ) ]]; then
  echo "error: --gemini-candidates, --media, --preflight, and --write-config are mutually exclusive" >&2
  exit 1
fi

LIVE_PROVIDERS=()
IMAGE_PROVIDERS=()
IMAGE_MODELS=()
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
PROXY_PID=""
PREFLIGHT_PROVIDER_PID=""
PREFLIGHT_PROVIDER_LOG_PATH=""
PREFLIGHT_PROVIDER_URL=""
PREFLIGHT_PROVIDER_API_KEY=""
PROVIDER_DISCOVERY_PATH="${TMP_DIR}/provider-discovery.json"

cleanup() {
  local exit_status=$?
  trap - EXIT HUP INT TERM
  if [[ -n "${PROXY_PID}" ]] && kill -0 "${PROXY_PID}" >/dev/null 2>&1; then
    kill -TERM "${PROXY_PID}" >/dev/null 2>&1 || true
    wait "${PROXY_PID}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${PREFLIGHT_PROVIDER_PID}" ]] && kill -0 "${PREFLIGHT_PROVIDER_PID}" >/dev/null 2>&1; then
    kill -TERM "${PREFLIGHT_PROVIDER_PID}" >/dev/null 2>&1 || true
    wait "${PREFLIGHT_PROVIDER_PID}" >/dev/null 2>&1 || true
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
  LIVE_ENV_FILE="$(cd "$(dirname "${LIVE_ENV_FILE}")" && pwd)/$(basename "${LIVE_ENV_FILE}")"
  load_env_file "${LIVE_ENV_FILE}"
fi

LIVE_ALL_MODELS="$(env_or_default LLM_PROXY_LIVE_ALL_MODELS false)"
if [[ "${LIVE_ALL_MODELS}" != "true" && "${LIVE_ALL_MODELS}" != "false" ]]; then
  echo "error: LLM_PROXY_LIVE_ALL_MODELS must be true or false" >&2
  exit 1
fi
LIVE_REASONING_MATRIX_DEFAULT=false
if [[ "${GEMINI_CANDIDATES_ONLY}" == "true" ]]; then
  LIVE_REASONING_MATRIX_DEFAULT=true
fi
LIVE_REASONING_MATRIX="$(env_or_default LLM_PROXY_LIVE_REASONING_MATRIX "${LIVE_REASONING_MATRIX_DEFAULT}")"
if [[ "${LIVE_REASONING_MATRIX}" != "true" && "${LIVE_REASONING_MATRIX}" != "false" ]]; then
  echo "error: LLM_PROXY_LIVE_REASONING_MATRIX must be true or false" >&2
  exit 1
fi
LIVE_TIMEOUT="$(env_or_default LLM_PROXY_LIVE_TIMEOUT 45)"
if [[ "${GEMINI_CANDIDATES_ONLY}" == "true" ]]; then
  if [[ -n "${LIVE_ENV_FILE:-}" ]]; then
    candidate_environment_names_path="${TMP_DIR}/candidate-provider-environment-names"
    printf '%s\n' GEMINI_API_KEY >"${candidate_environment_names_path}"
    unset GEMINI_API_KEY
    load_env_file "${LIVE_ENV_FILE}" "${candidate_environment_names_path}"
  fi
  LLM_PROXY_LIVE_REASONING_MATRIX="${LIVE_REASONING_MATRIX}" \
    LLM_PROXY_LIVE_TIMEOUT="${LIVE_TIMEOUT}" \
    "${ROOT_DIR}/scripts/test_live_gemini_candidates.sh"
  exit 0
fi

if [[ -n "${EXISTING_LOCAL_ORIGIN}" ]]; then
  if ! PORT="$(existing_local_origin_port "${EXISTING_LOCAL_ORIGIN}")"; then
    echo "error: --existing-local-origin must be an HTTP 127.0.0.1 origin with an explicit port" >&2
    exit 1
  fi
  LIVE_ORIGIN="${EXISTING_LOCAL_ORIGIN%/}"
  load_existing_management_environment
elif [[ -n "${LLM_PROXY_LIVE_PORT:-}" ]]; then
  PORT="${LLM_PROXY_LIVE_PORT}"
  LIVE_ORIGIN="http://127.0.0.1:${PORT}"
else
  PORT="$(allocate_loopback_port)"
  LIVE_ORIGIN="http://127.0.0.1:${PORT}"
fi
if [[ -n "${WRITE_CONFIG_PATH}" ]]; then
  mkdir -p "$(dirname "${WRITE_CONFIG_PATH}")"
  CONFIG_PATH="$(cd "$(dirname "${WRITE_CONFIG_PATH}")" && pwd)/$(basename "${WRITE_CONFIG_PATH}")"
  PROVIDER_CATALOG_PATH="$(dirname "${CONFIG_PATH}")/providers.yml"
else
  CONFIG_PATH="${TMP_DIR}/config.yml"
  PROVIDER_CATALOG_PATH="${TMP_DIR}/providers.yml"
fi
LOG_PATH="${TMP_DIR}/llm-proxy.log"
PUBLIC_CAPABILITIES_RESPONSE_PATH="${TMP_DIR}/public-capabilities.json"
export LLM_PROXY_LIVE_PORT="${PORT}"
if [[ -z "${WRITE_CONFIG_PATH}" && -z "${EXISTING_LOCAL_ORIGIN}" ]]; then
  configure_live_management_environment
fi
if [[ "${PREFLIGHT_ONLY}" == "true" ]]; then
  PREFLIGHT_PROVIDER_PORT="$(allocate_loopback_port)"
  while [[ "${PREFLIGHT_PROVIDER_PORT}" == "${PORT}" ]]; do
    PREFLIGHT_PROVIDER_PORT="$(allocate_loopback_port)"
  done
  export PREFLIGHT_PROVIDER_URL
  PREFLIGHT_PROVIDER_URL="http://127.0.0.1:${PREFLIGHT_PROVIDER_PORT}"
  start_managed_preflight_provider
fi
write_managed_live_config

if [[ -n "${WRITE_CONFIG_PATH}" ]]; then
  echo "isolated live provider config written: ${CONFIG_PATH}"
  exit 0
fi

GO_BIN="$(env_or_default GO go)"
BINARY_PATH="${TMP_DIR}/llm-proxy-live"

if [[ -z "${EXISTING_LOCAL_ORIGIN}" ]] && command -v lsof >/dev/null 2>&1 && lsof -tiTCP:"${PORT}" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "error: port ${PORT} is already in use; set LLM_PROXY_LIVE_PORT to a free port" >&2
  exit 1
fi

cd "${ROOT_DIR}"
GOEXPERIMENT= CGO_ENABLED=0 "${GO_BIN}" build -o "${BINARY_PATH}" ./cmd/cli

if ! GOEXPERIMENT= "${BINARY_PATH}" --config "${CONFIG_PATH}" --provider-catalog-only >"${PROVIDER_DISCOVERY_PATH}"; then
  echo "error: live provider catalog discovery failed" >&2
  exit 1
fi
if [[ -n "${LIVE_ENV_FILE:-}" ]]; then
  load_catalog_provider_environment
fi
if [[ "${PREFLIGHT_ONLY}" != "true" ]]; then
  discover_live_providers
  if [[ "${#LIVE_PROVIDERS[@]}" -eq 0 ]]; then
    echo "live provider smoke skipped: no complete provider environment bindings found"
    exit 0
  fi
fi

if [[ -n "${EXISTING_LOCAL_ORIGIN}" ]]; then
  wait_for_existing_local_origin
else
  GOEXPERIMENT= "${BINARY_PATH}" --config "${CONFIG_PATH}" >"${LOG_PATH}" 2>&1 &
  PROXY_PID="$!"
  wait_for_proxy
fi

initialize_live_management
if [[ "${PREFLIGHT_ONLY}" == "true" ]]; then
  run_managed_config_preflight
  exit 0
fi
if [[ "${MEDIA_ONLY}" == "true" ]]; then
  fetch_public_capabilities
  if [[ -z "${LLM_PROXY_LIVE_PROVIDERS:-}" ]]; then
    catalog_image_providers=()
    for live_provider in "${LIVE_PROVIDERS[@]}"; do
      if provider_catalog_image_models "${live_provider}" >/dev/null 2>&1; then
        catalog_image_providers+=("${live_provider}")
      fi
    done
    LIVE_PROVIDERS=("${catalog_image_providers[@]}")
    if [[ "${#LIVE_PROVIDERS[@]}" -eq 0 ]]; then
      echo "live provider image smoke skipped: no catalog image provider has complete environment bindings"
      exit 0
    fi
  fi
  for live_provider in "${LIVE_PROVIDERS[@]}"; do
    if [[ "${LIVE_ALL_MODELS}" == "true" ]]; then
      if ! live_models="$(provider_catalog_image_models "${live_provider}")"; then
        echo "error: live provider image models are unavailable: provider=${live_provider}" >&2
        exit 1
      fi
      while IFS= read -r image_model; do
        [[ -n "${image_model}" ]] || continue
        IMAGE_PROVIDERS+=("${live_provider}")
        IMAGE_MODELS+=("${image_model}")
      done <<<"${live_models}"
    else
      if ! image_model="$(provider_image_model "${live_provider}")"; then
        echo "error: live provider image model is unavailable or ambiguous: provider=${live_provider}" >&2
        exit 1
      fi
      IMAGE_PROVIDERS+=("${live_provider}")
      IMAGE_MODELS+=("${image_model}")
    fi
  done
  for live_provider_index in "${!IMAGE_PROVIDERS[@]}"; do
    verify_provider_key "${IMAGE_PROVIDERS[${live_provider_index}]}" "${IMAGE_MODELS[${live_provider_index}]}"
  done
  for live_provider_index in "${!IMAGE_PROVIDERS[@]}"; do
    run_image_smoke "${IMAGE_PROVIDERS[${live_provider_index}]}" "${IMAGE_MODELS[${live_provider_index}]}"
  done
else
  if [[ "${LIVE_REASONING_MATRIX}" == "true" && "${LIVE_ALL_MODELS}" != "true" ]]; then
    fetch_public_capabilities
  fi
  if [[ "${LIVE_ALL_MODELS}" == "true" ]]; then
    fetch_public_capabilities
    for live_provider in "${LIVE_PROVIDERS[@]}"; do
      if ! live_models="$(provider_catalog_text_models "${live_provider}")"; then
        echo "error: live provider text models are unavailable: provider=${live_provider}" >&2
        exit 1
      fi
      while IFS= read -r live_model; do
        [[ -n "${live_model}" ]] || continue
        verify_provider_key "${live_provider}" "${live_model}"
        if [[ "${LIVE_REASONING_MATRIX}" == "true" ]]; then
          run_text_smoke_matrix "${live_provider}" "${live_model}"
        else
          run_text_smoke "${live_provider}" "${live_model}"
        fi
      done <<<"${live_models}"
    done
  else
    for live_provider in "${LIVE_PROVIDERS[@]}"; do
      verify_provider_key "${live_provider}"
      if [[ "${LIVE_REASONING_MATRIX}" == "true" ]]; then
        run_text_smoke_matrix "${live_provider}"
      else
        run_text_smoke "${live_provider}"
      fi
    done
  fi
fi
