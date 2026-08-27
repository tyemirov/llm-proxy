#!/usr/bin/env bash
set -euo pipefail

usage() {
  builtin printf '%s\n' 'Usage: scripts/test_live_gemini_candidates.sh

Runs paid Google Interactions acceptance for the exact Gemini 3.6 Flash and
Gemini 3.7 Flash candidate models. The test covers each supported thinking
level, one omitted level, background completion, active retrieval,
cancellation, and deletion.

Required environment:
  GEMINI_API_KEY

Optional environment:
  LLM_PROXY_LIVE_REASONING_MATRIX  Exact true or false. Default: true.
  LLM_PROXY_LIVE_TIMEOUT           Per-request curl timeout. Default: 45.'
}

if [[ $# -gt 0 ]]; then
  case "$1" in
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
fi

command -v curl >/dev/null 2>&1 || { echo "error: curl is required for Gemini candidate acceptance" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo "error: python3 is required for Gemini candidate acceptance" >&2; exit 1; }
[[ -n "${GEMINI_API_KEY:-}" ]] || { echo "error: GEMINI_API_KEY is required for Gemini candidate acceptance" >&2; exit 1; }

REASONING_MATRIX="${LLM_PROXY_LIVE_REASONING_MATRIX:-true}"
if [[ "${REASONING_MATRIX}" != "true" && "${REASONING_MATRIX}" != "false" ]]; then
  echo "error: LLM_PROXY_LIVE_REASONING_MATRIX must be true or false" >&2
  exit 1
fi
REQUEST_TIMEOUT="${LLM_PROXY_LIVE_TIMEOUT:-45}"
if [[ ! "${REQUEST_TIMEOUT}" =~ ^[1-9][0-9]*$ ]]; then
  echo "error: LLM_PROXY_LIVE_TIMEOUT must be a positive integer" >&2
  exit 1
fi

readonly GEMINI_INTERACTIONS_URL="https://generativelanguage.googleapis.com/v1beta/interactions"
readonly GEMINI_API_REVISION="2026-05-20"
readonly VISIBILITY_RETRY_LIMIT=6
readonly POLL_LIMIT=12
readonly POLL_INTERVAL_SECONDS=5

TMP_DIR="$(mktemp -d)"
chmod 700 "${TMP_DIR}"
INTERACTION_IDS=()
CREATED_INTERACTION_ID=""

gemini_request() {
  local method="$1"
  local request_url="$2"
  local response_path="$3"
  local request_path="${4:-}"
  local curl_arguments=(
    -sS
    --max-time "${REQUEST_TIMEOUT}"
    -X "${method}"
    -H "x-goog-api-key: ${GEMINI_API_KEY}"
    -H "Api-Revision: ${GEMINI_API_REVISION}"
    -o "${response_path}"
    -w "%{http_code}"
  )
  if [[ -n "${request_path}" ]]; then
    curl_arguments+=(
      -H "Content-Type: application/json"
      --data-binary "@${request_path}"
    )
  fi
  curl "${curl_arguments[@]}" "${request_url}"
}

delete_interaction() {
  local interaction_id="$1"
  local response_path="${TMP_DIR}/delete-response.json"
  local http_status
  http_status="$(gemini_request DELETE "${GEMINI_INTERACTIONS_URL}/${interaction_id}" "${response_path}")"
  if [[ "${http_status}" != "200" && "${http_status}" != "204" ]]; then
    echo "error: Gemini candidate deletion failed: status=${http_status}" >&2
    return 1
  fi
  local remaining_ids=()
  local candidate_id
  for candidate_id in "${INTERACTION_IDS[@]}"; do
    if [[ "${candidate_id}" != "${interaction_id}" ]]; then
      remaining_ids+=("${candidate_id}")
    fi
  done
  INTERACTION_IDS=("${remaining_ids[@]}")
}

cleanup() {
  local exit_status=$?
  local interaction_id
  trap - EXIT HUP INT TERM
  for interaction_id in "${INTERACTION_IDS[@]}"; do
    gemini_request DELETE "${GEMINI_INTERACTIONS_URL}/${interaction_id}" "${TMP_DIR}/cleanup-response.json" >/dev/null 2>&1 || true
  done
  rm -rf "${TMP_DIR}"
  return "${exit_status}"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

write_reasoning_request() {
  local model="$1"
  local reasoning_effort="$2"
  local request_path="$3"
  python3 -c '
import json
import pathlib
import sys

payload = {
    "model": sys.argv[1],
    "input": "Reply with exactly OK and no punctuation.",
    "background": False,
    "store": False,
}
if sys.argv[2]:
    payload["generation_config"] = {"thinking_level": sys.argv[2]}
pathlib.Path(sys.argv[3]).write_text(
    json.dumps(payload, separators=(",", ":")),
    encoding="utf-8",
)
' "${model}" "${reasoning_effort}" "${request_path}"
  chmod 600 "${request_path}"
}

write_background_request() {
  local model="$1"
  local lifecycle="$2"
  local request_path="$3"
  python3 -c '
import json
import pathlib
import sys

if sys.argv[2] == "completion":
    prompt = "Reply with exactly OK and no punctuation."
    output_limit = 4096
else:
    prompt = "Write the integers from 1 through 10000 in ascending order."
    output_limit = 65536
payload = {
    "model": sys.argv[1],
    "input": prompt,
    "background": True,
    "store": True,
    "generation_config": {
        "max_output_tokens": output_limit,
        "thinking_level": "high",
    },
}
pathlib.Path(sys.argv[3]).write_text(
    json.dumps(payload, separators=(",", ":")),
    encoding="utf-8",
)
' "${model}" "${lifecycle}" "${request_path}"
  chmod 600 "${request_path}"
}

interaction_summary() {
  local response_path="$1"
  python3 -c '
import json
import pathlib
import sys

interaction = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
identifier = interaction.get("id", "")
status = interaction.get("status", "")
texts = []
for step in interaction.get("steps", []):
    if isinstance(step, dict) and step.get("type") == "model_output":
        for content in step.get("content", []):
            if isinstance(content, dict) and isinstance(content.get("text"), str):
                texts.append(content["text"])
print(json.dumps({
    "id": identifier,
    "status": status,
    "text": "".join(texts).strip(),
}, separators=(",", ":")))
' "${response_path}"
}

summary_field() {
  local summary="$1"
  local field="$2"
  python3 -c '
import json
import sys

value = json.loads(sys.argv[1]).get(sys.argv[2], "")
if not isinstance(value, str):
    raise SystemExit(1)
print(value)
' "${summary}" "${field}"
}

run_reasoning_request() {
  local model="$1"
  local reasoning_effort="$2"
  local effort_label="${reasoning_effort:-omitted}"
  local request_path="${TMP_DIR}/reasoning-${model}-${effort_label}.json"
  local response_path="${TMP_DIR}/reasoning-${model}-${effort_label}-response.json"
  local http_status
  local summary
  write_reasoning_request "${model}" "${reasoning_effort}" "${request_path}"
  http_status="$(gemini_request POST "${GEMINI_INTERACTIONS_URL}" "${response_path}" "${request_path}")"
  if [[ "${http_status}" != "200" ]]; then
    echo "error: Gemini candidate reasoning failed: model=${model} reasoning_effort=${effort_label} status=${http_status}" >&2
    exit 1
  fi
  summary="$(interaction_summary "${response_path}")"
  if [[ "$(summary_field "${summary}" status)" != "completed" ||
    "$(summary_field "${summary}" text)" != "OK" ]]; then
    echo "error: Gemini candidate reasoning returned an invalid response: model=${model} reasoning_effort=${effort_label}" >&2
    exit 1
  fi
  echo "live Gemini candidate reasoning passed: model=${model} reasoning_effort=${effort_label} status=200"
}

create_background_interaction() {
  local model="$1"
  local lifecycle="$2"
  local request_path="${TMP_DIR}/${lifecycle}-${model}.json"
  local response_path="${TMP_DIR}/${lifecycle}-${model}-create-response.json"
  local http_status
  local summary
  local interaction_id
  local interaction_status
  write_background_request "${model}" "${lifecycle}" "${request_path}"
  http_status="$(gemini_request POST "${GEMINI_INTERACTIONS_URL}" "${response_path}" "${request_path}")"
  if [[ "${http_status}" != "200" ]]; then
    echo "error: Gemini candidate background create failed: model=${model} lifecycle=${lifecycle} status=${http_status}" >&2
    exit 1
  fi
  summary="$(interaction_summary "${response_path}")"
  interaction_id="$(summary_field "${summary}" id)"
  interaction_status="$(summary_field "${summary}" status)"
  if [[ ! "${interaction_id}" =~ ^[A-Za-z0-9._~-]+$ ||
    ( "${interaction_status}" != "queued" && "${interaction_status}" != "in_progress" ) ]]; then
    echo "error: Gemini candidate background create returned an invalid resource: model=${model} lifecycle=${lifecycle}" >&2
    exit 1
  fi
  INTERACTION_IDS+=("${interaction_id}")
  CREATED_INTERACTION_ID="${interaction_id}"
}

retrieve_active_interaction() {
  local model="$1"
  local lifecycle="$2"
  local interaction_id="$3"
  local response_path="${TMP_DIR}/${lifecycle}-${model}-active-response.json"
  local attempt
  local http_status
  local summary
  local interaction_status
  for ((attempt = 0; attempt <= VISIBILITY_RETRY_LIMIT; attempt += 1)); do
    http_status="$(gemini_request GET "${GEMINI_INTERACTIONS_URL}/${interaction_id}" "${response_path}")"
    if [[ "${http_status}" == "200" ]]; then
      summary="$(interaction_summary "${response_path}")"
      interaction_status="$(summary_field "${summary}" status)"
      if [[ "$(summary_field "${summary}" id)" != "${interaction_id}" ||
        ( "${interaction_status}" != "queued" && "${interaction_status}" != "in_progress" ) ]]; then
        echo "error: Gemini candidate active retrieval returned an invalid resource: model=${model} lifecycle=${lifecycle} status=${interaction_status}" >&2
        exit 1
      fi
      return
    fi
    if [[ ( "${http_status}" != "400" && "${http_status}" != "403" && "${http_status}" != "404" ) || "${attempt}" -eq "${VISIBILITY_RETRY_LIMIT}" ]]; then
      echo "error: Gemini candidate active retrieval failed: model=${model} lifecycle=${lifecycle} status=${http_status}" >&2
      exit 1
    fi
    sleep "${POLL_INTERVAL_SECONDS}"
  done
}

complete_background_interaction() {
  local model="$1"
  local interaction_id="$2"
  local response_path="${TMP_DIR}/completion-${model}-poll-response.json"
  local attempt
  local http_status
  local summary
  local interaction_status
  for ((attempt = 0; attempt < POLL_LIMIT; attempt += 1)); do
    sleep "${POLL_INTERVAL_SECONDS}"
    http_status="$(gemini_request GET "${GEMINI_INTERACTIONS_URL}/${interaction_id}" "${response_path}")"
    if [[ "${http_status}" != "200" ]]; then
      echo "error: Gemini candidate completion retrieval failed: model=${model} status=${http_status}" >&2
      exit 1
    fi
    summary="$(interaction_summary "${response_path}")"
    interaction_status="$(summary_field "${summary}" status)"
    if [[ "${interaction_status}" == "queued" || "${interaction_status}" == "in_progress" ]]; then
      continue
    fi
    if [[ "${interaction_status}" != "completed" ||
      "$(summary_field "${summary}" id)" != "${interaction_id}" ||
      "$(summary_field "${summary}" text)" != "OK" ]]; then
      echo "error: Gemini candidate completion returned an invalid resource: model=${model} status=${interaction_status}" >&2
      exit 1
    fi
    delete_interaction "${interaction_id}"
    echo "live Gemini candidate completion lifecycle passed: model=${model} status=completed"
    return
  done
  echo "error: Gemini candidate completion did not reach a terminal state: model=${model}" >&2
  exit 1
}

cancel_background_interaction() {
  local model="$1"
  local interaction_id="$2"
  local response_path="${TMP_DIR}/cancellation-${model}-response.json"
  local http_status
  local summary
  http_status="$(gemini_request POST "${GEMINI_INTERACTIONS_URL}/${interaction_id}/cancel" "${response_path}")"
  if [[ "${http_status}" != "200" ]]; then
    echo "error: Gemini candidate cancellation failed: model=${model} status=${http_status}" >&2
    exit 1
  fi
  summary="$(interaction_summary "${response_path}")"
  if [[ "$(summary_field "${summary}" id)" != "${interaction_id}" ||
    "$(summary_field "${summary}" status)" != "cancelled" ]]; then
    echo "error: Gemini candidate cancellation returned an invalid resource: model=${model}" >&2
    exit 1
  fi
  delete_interaction "${interaction_id}"
  echo "live Gemini candidate cancellation lifecycle passed: model=${model} status=cancelled"
}

run_candidate_model() {
  local model="$1"
  shift
  local reasoning_effort
  local completion_id
  local cancellation_id
  run_reasoning_request "${model}" ""
  if [[ "${REASONING_MATRIX}" == "true" ]]; then
    for reasoning_effort in "$@"; do
      run_reasoning_request "${model}" "${reasoning_effort}"
    done
  fi
  create_background_interaction "${model}" completion
  completion_id="${CREATED_INTERACTION_ID}"
  complete_background_interaction "${model}" "${completion_id}"
  create_background_interaction "${model}" cancellation
  cancellation_id="${CREATED_INTERACTION_ID}"
  retrieve_active_interaction "${model}" cancellation "${cancellation_id}"
  cancel_background_interaction "${model}" "${cancellation_id}"
}

run_candidate_model gemini-3.6-flash minimal low medium high
run_candidate_model gemini-3.7-flash low medium high
echo "live Gemini candidate acceptance passed: models=gemini-3.6-flash,gemini-3.7-flash"
