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
BACKGROUND_PIDS=()
BACKGROUND_LOGS=()

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
  local child_status=0
  local child_index
  trap - EXIT INT TERM

  for child_index in "${!BACKGROUND_PIDS[@]}"; do
    if wait "${BACKGROUND_PIDS[$child_index]}"; then
      child_status=0
    else
      child_status=$?
    fi
    cat "${BACKGROUND_LOGS[$child_index]}" || cleanup_failed=1
    if [[ "$exit_status" -eq 0 && "$child_status" -ne 0 ]]; then
      exit_status="$child_status"
      CURRENT_STAGE="${STAGE_NAMES[$child_index]}"
    fi
  done

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
  "Protocol acceptance"
  "Upstream admission race tests"
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
  "test-protocol-acceptance"
  "test-upstream-admission-race"
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
  completed_stage_index=$((stage_number - 1))
  COMPLETED_STAGE_NAMES[$completed_stage_index]="$stage_name"
  COMPLETED_STAGE_EVIDENCE[$completed_stage_index]="${stage_elapsed_seconds}s"
  printf '[%d/%d] PASS %s (%ds)\n' "$stage_number" "$TOTAL_TARGET_STAGES" "$stage_name" "$stage_elapsed_seconds"
}

start_background_stage() {
  local child_index="$1"
  local child_started_seconds=$SECONDS
  BACKGROUND_LOGS[$child_index]="$RUN_DIRECTORY/stage-${child_index}.log"
  printf '\n[%d/%d] %s\n' "$((child_index + 1))" "$TOTAL_TARGET_STAGES" "${STAGE_NAMES[$child_index]}"
  (
    trap - EXIT INT TERM
    "$MAKE_BIN" --no-print-directory "${STAGE_TARGETS[$child_index]}"
    printf '%s\n' "$((SECONDS - child_started_seconds))" >"$RUN_DIRECTORY/stage-${child_index}.seconds"
  ) >"${BACKGROUND_LOGS[$child_index]}" 2>&1 &
  BACKGROUND_PIDS[$child_index]=$!
}

finish_background_stage() {
  local child_index="$1"
  local child_status=0
  local child_elapsed_seconds
  CURRENT_STAGE="${STAGE_NAMES[$child_index]}"
  if wait "${BACKGROUND_PIDS[$child_index]}"; then
    child_status=0
  else
    child_status=$?
  fi
  unset 'BACKGROUND_PIDS[child_index]'
  cat "${BACKGROUND_LOGS[$child_index]}"
  if [[ "$child_status" -ne 0 ]]; then
    return "$child_status"
  fi
  read -r child_elapsed_seconds <"$RUN_DIRECTORY/stage-${child_index}.seconds"
  COMPLETED_STAGE_NAMES[$child_index]="$CURRENT_STAGE"
  COMPLETED_STAGE_EVIDENCE[$child_index]="${child_elapsed_seconds}s"
  printf '[%d/%d] PASS %s (%ds)\n' "$((child_index + 1))" "$TOTAL_TARGET_STAGES" "$CURRENT_STAGE" "$child_elapsed_seconds"
}

for ((stage_index = 0; stage_index < TOTAL_TARGET_STAGES; stage_index++)); do
  if [[ "${STAGE_TARGETS[$stage_index]}" == "test-upstream-admission-race" ]]; then
    # These checks use independent temporary state and do not change source files.
    start_background_stage "$stage_index"
    start_background_stage "$((stage_index + 2))"
    run_stage "${STAGE_NAMES[$((stage_index + 1))]}" "${STAGE_TARGETS[$((stage_index + 1))]}" "$((stage_index + 2))"
    finish_background_stage "$stage_index"
    finish_background_stage "$((stage_index + 2))"
    stage_index=$((stage_index + 2))
    continue
  fi
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
