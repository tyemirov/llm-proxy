#!/usr/bin/env bash
set -euo pipefail

readonly PRODUCTION_API_ORIGIN="https://llm-proxy-api.mprlab.com"
readonly ECHO_REQUEST_TIMEOUT_SECONDS=90
readonly ECHO_CURL_TIMEOUT_SECONDS=120
readonly LONG_COMPLETION_REQUEST_TIMEOUT_SECONDS=900
readonly LONG_COMPLETION_CURL_TIMEOUT_SECONDS=930
readonly CURL_CONNECT_TIMEOUT_SECONDS=15
readonly ECHO_RESPONSE_MARKER="LLM_PROXY_LIVE_ECHO_OK"
readonly LONG_COMPLETION_RESPONSE_MARKER="LLM_PROXY_LIVE_COMPLEX_OK"
readonly LONG_COMPLETION_MAX_TOKENS=512
readonly LONG_COMPLETION_MINIMUM_REQUEST_BYTES=16384

LIVE_TEST_PROVIDERS=(openai anthropic meta gemini moonshot)
LONG_COMPLETION_PROVIDERS=(openai anthropic meta gemini)
TEMPORARY_DIRECTORY=""
ENCODED_TENANT_SECRET=""

usage() {
  builtin printf '%s\n' \
    'Usage: make live-test' \
    '' \
    'Runs paid production requests against the Default tenant at https://llm-proxy-api.mprlab.com.' \
    'Requires only LLM_PROXY_SECRET, the Default-tenant client secret.' \
    'It never loads dotenv files or local upstream-provider credentials.' \
    '' \
    'The command sends one echo-marker request to OpenAI, Anthropic, Meta, Gemini, and Moonshot,' \
    'then sends matching large-completion requests through OpenAI, Anthropic, Meta, and Gemini.' \
    'The Gemini long case selects gemini-3.5-flash so OpenAI and Gemini prove server-owned background polling.'
}

fail() {
  builtin printf 'error: %s\n' "$1" >&2
  exit 1
}

cleanup() {
  local exit_status=$?
  trap - EXIT HUP INT TERM
  if [[ -n "${TEMPORARY_DIRECTORY}" && -d "${TEMPORARY_DIRECTORY}" ]]; then
    rm -rf "${TEMPORARY_DIRECTORY}"
  fi
  exit "${exit_status}"
}

encode_tenant_secret() {
  printf '%s' "${LLM_PROXY_SECRET}" | python3 -c 'import sys; from urllib.parse import quote; print(quote(sys.stdin.read(), safe=""), end="")'
}

build_echo_request() {
  builtin printf '%s' '{"messages":[{"role":"user","content":"Reply with exactly LLM_PROXY_LIVE_ECHO_OK and no other text."}]}'
}

build_long_completion_request() {
  python3 -c '
import json
import sys

marker = "LLM_PROXY_LIVE_COMPLEX_OK"
regions = ("north", "south", "east", "west")
channels = ("direct", "partner", "self-serve", "enterprise")
records = []
for record_index in range(1, 121):
    records.append(
        "Portfolio {index:03d}: region={region}; channel={channel}; baseline={baseline}; "
        "forecast={forecast}; risk={risk}; dependency={dependency}; required_control={control}.".format(
            index=record_index,
            region=regions[record_index % len(regions)],
            channel=channels[record_index % len(channels)],
            baseline=120 + ((record_index * 37) % 880),
            forecast=140 + ((record_index * 53) % 1040),
            risk=("low", "medium", "high")[record_index % 3],
            dependency=("legal", "security", "data", "operations")[record_index % 4],
            control=("two-person review", "rollback plan", "access review", "budget cap")[record_index % 4],
        )
    )
prompt = "\n".join(
    (
        "Inspect every record in the complete fictional portfolio below.",
        "For every record, emit one normalized line containing its portfolio number, region, channel, baseline,",
        "forecast, risk, dependency, and required control. Do not omit, combine, or summarize any record.",
        "After all 120 normalized lines, calculate the total forecast uplift for records where region is north and",
        "risk is high, then count records where dependency is security and forecast is below 600. End with exactly " +
        marker + " followed by the two integer results separated by a single comma. Do not include any other summary.",
        "",
        "\n".join(records),
    )
)
payload = {
    "messages": [
        {
            "role": "system",
            "content": "You are a meticulous program analyst. Follow the user response-format instruction exactly.",
        },
        {"role": "user", "content": prompt},
    ],
    "max_tokens": int(sys.argv[1]),
}
print(json.dumps(payload, separators=(",", ":")))
' "${LONG_COMPLETION_MAX_TOKENS}"
}

write_curl_config() {
  local provider_identifier="$1"
  local model_identifier="$2"
  local curl_config_path="$3"
  local model_query=""
  if [[ -n "${model_identifier}" ]]; then
    model_query="&model=${model_identifier}"
  fi
  builtin printf 'url = "%s/v2?provider=%s&format=text%%2Fplain&key=%s%s"\n' \
    "${PRODUCTION_API_ORIGIN}" \
    "${provider_identifier}" \
    "${ENCODED_TENANT_SECRET}" \
    "${model_query}" >"${curl_config_path}"
}

response_size() {
  local response_path="$1"
  if [[ ! -f "${response_path}" ]]; then
    builtin printf '%s' '0'
    return
  fi
  wc -c <"${response_path}" | tr -d '[:space:]'
}

has_expected_timeout_header() {
  local headers_path="$1"
  local request_timeout_seconds="$2"
  [[ -f "${headers_path}" ]] || return 1
  tr -d '\r' <"${headers_path}" | grep -Eiq "^X-LLM-Proxy-Request-Timeout-Seconds:[[:space:]]*${request_timeout_seconds}[[:space:]]*$"
}

validated_response_request_id() {
  local headers_path="$1"
  [[ -f "${headers_path}" ]] || return 1
  python3 -c '
import re
import sys

header_path = sys.argv[1]
with open(header_path, encoding="utf-8", errors="strict") as header_file:
    values = [
        line.split(":", 1)[1].strip()
        for line in header_file
        if line.lower().startswith("x-llm-proxy-request-id:")
    ]
if len(values) != 1 or re.fullmatch(r"[A-Z2-7]{26,}", values[0]) is None:
    raise SystemExit(1)
print(values[0], end="")
' "${headers_path}"
}

run_live_case() {
  local case_identifier="$1"
  local provider_identifier="$2"
  local request_body="$3"
  local expected_marker="$4"
  local request_timeout_seconds="$5"
  local curl_timeout_seconds="$6"
  local model_identifier="$7"
  local curl_config_path="${TEMPORARY_DIRECTORY}/${case_identifier}.curl"
  local headers_path="${TEMPORARY_DIRECTORY}/${case_identifier}.headers"
  local response_path="${TEMPORARY_DIRECTORY}/${case_identifier}.response"
  local http_status=""
  local response_bytes
  local response_request_id=""

  write_curl_config "${provider_identifier}" "${model_identifier}" "${curl_config_path}"
  if ! http_status="$(
    printf '%s' "${request_body}" | curl \
      --silent \
      --show-error \
      --config "${curl_config_path}" \
      --request POST \
      --header 'Content-Type: application/json' \
      --header "X-LLM-Proxy-Request-Timeout-Seconds: ${request_timeout_seconds}" \
      --connect-timeout "${CURL_CONNECT_TIMEOUT_SECONDS}" \
      --max-time "${curl_timeout_seconds}" \
      --dump-header "${headers_path}" \
      --output "${response_path}" \
      --write-out '%{http_code}' \
      --data-binary @-
  )"; then
    response_bytes="$(response_size "${response_path}")"
    if response_request_id="$(validated_response_request_id "${headers_path}")"; then
      builtin printf 'live test failed: case=%s provider=%s transport_error response_bytes=%s request_id=%s\n' \
        "${case_identifier}" "${provider_identifier}" "${response_bytes}" "${response_request_id}" >&2
    else
      builtin printf 'live test failed: case=%s provider=%s transport_error response_bytes=%s\n' \
        "${case_identifier}" "${provider_identifier}" "${response_bytes}" >&2
    fi
    return 1
  fi

  response_bytes="$(response_size "${response_path}")"
  if ! response_request_id="$(validated_response_request_id "${headers_path}")"; then
    builtin printf 'live test failed: case=%s provider=%s status=%s response_bytes=%s invalid_request_id_header\n' \
      "${case_identifier}" "${provider_identifier}" "${http_status}" "${response_bytes}" >&2
    return 1
  fi
  if [[ "${http_status}" != '200' ]]; then
    builtin printf 'live test failed: case=%s provider=%s status=%s response_bytes=%s request_id=%s\n' \
      "${case_identifier}" "${provider_identifier}" "${http_status}" "${response_bytes}" "${response_request_id}" >&2
    return 1
  fi
  if ! has_expected_timeout_header "${headers_path}" "${request_timeout_seconds}"; then
    builtin printf 'live test failed: case=%s provider=%s status=%s response_bytes=%s request_id=%s missing_timeout_header\n' \
      "${case_identifier}" "${provider_identifier}" "${http_status}" "${response_bytes}" "${response_request_id}" >&2
    return 1
  fi
  if [[ ! -f "${response_path}" ]] || ! grep -Fq "${expected_marker}" "${response_path}"; then
    builtin printf 'live test failed: case=%s provider=%s status=%s response_bytes=%s request_id=%s missing_response_marker\n' \
      "${case_identifier}" "${provider_identifier}" "${http_status}" "${response_bytes}" "${response_request_id}" >&2
    return 1
  fi

  builtin printf 'live test passed: case=%s provider=%s status=%s response_bytes=%s request_id=%s\n' \
    "${case_identifier}" "${provider_identifier}" "${http_status}" "${response_bytes}" "${response_request_id}"
}

if [[ "${1:-}" == '--help' || "${1:-}" == '-h' ]]; then
  usage
  exit 0
fi
if [[ $# -ne 0 ]]; then
  usage >&2
  fail 'make live-test accepts no arguments'
fi
if [[ -z "${LLM_PROXY_SECRET:-}" ]]; then
  fail 'LLM_PROXY_SECRET must contain the Default-tenant client secret'
fi
command -v curl >/dev/null 2>&1 || fail 'curl is required for make live-test'
command -v python3 >/dev/null 2>&1 || fail 'python3 is required for make live-test'

umask 077
TEMPORARY_DIRECTORY="$(mktemp -d)"
trap cleanup EXIT HUP INT TERM
ENCODED_TENANT_SECRET="$(encode_tenant_secret)"

echo_request_body="$(build_echo_request)"
long_completion_request_body="$(build_long_completion_request)"
if [[ "$(printf '%s' "${long_completion_request_body}" | wc -c | tr -d '[:space:]')" -lt "${LONG_COMPLETION_MINIMUM_REQUEST_BYTES}" ]]; then
  fail 'large completion request did not meet the minimum request size'
fi

failed_case_count=0
for provider_identifier in "${LIVE_TEST_PROVIDERS[@]}"; do
  if ! run_live_case \
    "${provider_identifier}-echo" \
    "${provider_identifier}" \
    "${echo_request_body}" \
    "${ECHO_RESPONSE_MARKER}" \
    "${ECHO_REQUEST_TIMEOUT_SECONDS}" \
    "${ECHO_CURL_TIMEOUT_SECONDS}" \
    ""; then
    failed_case_count=$((failed_case_count + 1))
  fi
done
for provider_identifier in "${LONG_COMPLETION_PROVIDERS[@]}"; do
  case_identifier="${provider_identifier}-long-completion"
  if [[ "${provider_identifier}" == 'openai' ]]; then
    case_identifier='openai-background-polling'
  fi
  if [[ "${provider_identifier}" == 'gemini' ]]; then
    case_identifier='gemini-background-polling'
    model_identifier='gemini-3.5-flash'
  else
    model_identifier=''
  fi
  if ! run_live_case \
    "${case_identifier}" \
    "${provider_identifier}" \
    "${long_completion_request_body}" \
    "${LONG_COMPLETION_RESPONSE_MARKER}" \
    "${LONG_COMPLETION_REQUEST_TIMEOUT_SECONDS}" \
    "${LONG_COMPLETION_CURL_TIMEOUT_SECONDS}" \
    "${model_identifier}"; then
    failed_case_count=$((failed_case_count + 1))
  fi
done

if [[ "${failed_case_count}" -ne 0 ]]; then
  builtin printf 'live test failed: failed_cases=%s total_cases=%s\n' \
    "${failed_case_count}" \
    "$(( ${#LIVE_TEST_PROVIDERS[@]} + ${#LONG_COMPLETION_PROVIDERS[@]} ))" >&2
  exit 1
fi
builtin printf 'live test passed: total_cases=%s\n' \
  "$(( ${#LIVE_TEST_PROVIDERS[@]} + ${#LONG_COMPLETION_PROVIDERS[@]} ))"
