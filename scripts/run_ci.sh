#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MAKE_BIN="${MAKE_BIN:-make}"
GO_BIN="${GO:-go}"
RUN_DIRECTORY=""
CURRENT_STAGE="CI initialization"
CI_COMPLETE=0
RUN_STARTED_SECONDS=$SECONDS
COMPLETED_STAGE_NAMES=()
COMPLETED_STAGE_EVIDENCE=()

print_ci_success() {
  local stage_index
  local total_elapsed_seconds=$((SECONDS - RUN_STARTED_SECONDS))

  printf '\nCI summary\n' || return $?
  printf '%-3s %-42s %-8s %s\n' "#" "Gate" "Result" "Evidence" || return $?
  for ((stage_index = 0; stage_index < ${#COMPLETED_STAGE_NAMES[@]}; stage_index++)); do
    printf '%-3d %-42s %-8s %s\n' \
      "$((stage_index + 1))" \
      "${COMPLETED_STAGE_NAMES[$stage_index]}" \
      "PASS" \
      "${COMPLETED_STAGE_EVIDENCE[$stage_index]}" || return $?
  done
  printf '%-3s %-42s %-8s %s\n' \
    "" \
    "Continuous integration" \
    "PASS" \
    "${total_elapsed_seconds}s" || return $?
  printf '\nCI PASSED: all %d gates completed; Go statement coverage %s.\n' \
    "${#COMPLETED_STAGE_NAMES[@]}" \
    "$coverage_total"
}

finish_ci_run() {
  local exit_status=$?
  local cleanup_failed=0
  local receipt_status=0
  trap - EXIT INT TERM

  if [[ -n "$RUN_DIRECTORY" && -d "$RUN_DIRECTORY" ]]; then
    rm -rf -- "$RUN_DIRECTORY" || cleanup_failed=1
  fi
  if [[ "$cleanup_failed" -ne 0 && "$exit_status" -eq 0 ]]; then
    exit_status=1
    CI_COMPLETE=0
    CURRENT_STAGE="temporary CI state cleanup"
  fi
  if [[ "$CI_COMPLETE" -eq 1 && "$exit_status" -eq 0 ]]; then
    CURRENT_STAGE="terminal completion receipt"
    print_ci_success || receipt_status=$?
    if [[ "$receipt_status" -ne 0 ]]; then
      exit_status="$receipt_status"
      CI_COMPLETE=0
    fi
  fi
  if [[ "$CI_COMPLETE" -ne 1 || "$exit_status" -ne 0 ]]; then
    if [[ "$exit_status" -eq 0 ]]; then
      exit_status=1
    fi
    printf '\nCI FAILED: stopped during %s (exit %d).\n' "$CURRENT_STAGE" "$exit_status" >&2
  fi
  exit "$exit_status"
}

trap finish_ci_run EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

RUN_DIRECTORY="$(mktemp -d "${TMPDIR:-/tmp}/llm-proxy-ci.XXXXXX")"
export COVERAGE_FILE="$RUN_DIRECTORY/coverage.out"
cd "$ROOT_DIR"

STAGE_NAMES=(
  "Release version contract"
  "Go formatting"
  "Go static analysis"
  "Python static analysis"
  "Frontend static analysis"
  "Go integration tests"
  "Python client tests"
  "Frontend browser tests"
  "OpenAPI Pages artifact"
  "TAuth management browser black box"
  "Live-provider harness preflight"
)
STAGE_TARGETS=(
  "test-release-policy"
  "check-format"
  "go-lint"
  "python-lint"
  "frontend-lint"
  "go-test"
  "python-test"
  "frontend-test"
  "test-openapi-pages-artifact"
  "test-management-auth-blackbox"
  "test-live-provider-harness"
)
TOTAL_TARGET_STAGES=${#STAGE_TARGETS[@]}

if [[ "${#STAGE_NAMES[@]}" -ne "$TOTAL_TARGET_STAGES" ]]; then
  printf 'CI stage names and targets differ in length\n' >&2
  exit 1
fi

run_stage() {
  local stage_name="$1"
  local stage_target="$2"
  local stage_number="$3"
  local stage_started_seconds=$SECONDS
  local stage_elapsed_seconds
  local completed_stage_index

  CURRENT_STAGE="$stage_name"
  printf '\n[%d/%d] %s\n' "$stage_number" "$TOTAL_TARGET_STAGES" "$stage_name"
  "$MAKE_BIN" --no-print-directory "$stage_target"
  stage_elapsed_seconds=$((SECONDS - stage_started_seconds))
  completed_stage_index=${#COMPLETED_STAGE_NAMES[@]}
  COMPLETED_STAGE_NAMES[$completed_stage_index]="$stage_name"
  COMPLETED_STAGE_EVIDENCE[$completed_stage_index]="${stage_elapsed_seconds}s"
  printf '[%d/%d] PASS %s (%ds)\n' "$stage_number" "$TOTAL_TARGET_STAGES" "$stage_name" "$stage_elapsed_seconds"
}

for ((stage_index = 0; stage_index < TOTAL_TARGET_STAGES; stage_index++)); do
  run_stage \
    "${STAGE_NAMES[$stage_index]}" \
    "${STAGE_TARGETS[$stage_index]}" \
    "$((stage_index + 1))"
done

CURRENT_STAGE="Go coverage verification"
coverage_output="$("$GO_BIN" tool cover -func="$COVERAGE_FILE")"
coverage_total="$(printf '%s\n' "$coverage_output" | awk '$1 == "total:" { print $3 }')"
if [[ "$coverage_total" != "100.0%" ]]; then
  printf 'coverage total %s, want 100.0%%\n' "${coverage_total:-missing}" >&2
  exit 1
fi
completed_stage_index=${#COMPLETED_STAGE_NAMES[@]}
COMPLETED_STAGE_NAMES[$completed_stage_index]="$CURRENT_STAGE"
COMPLETED_STAGE_EVIDENCE[$completed_stage_index]="$coverage_total"

CURRENT_STAGE="terminal completion receipt"
CI_COMPLETE=1
