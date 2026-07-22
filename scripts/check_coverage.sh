#!/usr/bin/env bash
set -euo pipefail

GO_BIN="${GO:-go}"
RUNTIME_COVERPKG="./cmd/cli,./internal/apperrors,./internal/constants,./internal/proxy,./internal/utils"
CLIENT_COVERPKG="./llm-proxy-client,./pkg/llmproxyclient"
COVERPKG="$RUNTIME_COVERPKG,$CLIENT_COVERPKG"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
COVERAGE_PROBE_TIMEOUT_SECONDS=5
CLIENT_COVERAGE_PROBE_PROMPT="coverage probe"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

cd "$ROOT_DIR"

run_coverage_probe() {
  local cover_dir="$1"
  shift
  local probe_status=0
  timeout -k "${COVERAGE_PROBE_TIMEOUT_SECONDS}s" -s SIGKILL "${COVERAGE_PROBE_TIMEOUT_SECONDS}s" env -i GOCOVERDIR="$cover_dir" "$@" </dev/null >/dev/null 2>/dev/null || probe_status=$?
  if [[ "$probe_status" -eq 124 || "$probe_status" -eq 137 ]]; then
    printf 'coverage probe timed out: %s\n' "$*" >&2
    return "$probe_status"
  fi
}

"$GO_BIN" test -count=1 ./... -covermode=count -coverpkg="$COVERPKG" -coverprofile="$TMP_DIR/go-test.coverprofile"
"$GO_BIN" build -cover -covermode=count -coverpkg="$RUNTIME_COVERPKG" -o "$TMP_DIR/llm-proxy.cover" ./cmd/cli
"$GO_BIN" build -cover -covermode=count -coverpkg="$CLIENT_COVERPKG" -o "$TMP_DIR/llm-proxy-client.cover" ./llm-proxy-client

builtin printf '%s\n' 'tenants:
  - id: coverage
    secret: "service-secret"
providers:
  openai: {}' >"$TMP_DIR/missing-openai.yml"

mkdir -p "$TMP_DIR/cov-help" "$TMP_DIR/cov-missing-config" "$TMP_DIR/cov-missing-openai" "$TMP_DIR/cov-client-missing-config"
run_coverage_probe "$TMP_DIR/cov-help" "$TMP_DIR/llm-proxy.cover" --help
run_coverage_probe "$TMP_DIR/cov-missing-config" "$TMP_DIR/llm-proxy.cover" --config "$TMP_DIR/missing.yml"
run_coverage_probe "$TMP_DIR/cov-missing-openai" "$TMP_DIR/llm-proxy.cover" --config "$TMP_DIR/missing-openai.yml"
run_coverage_probe "$TMP_DIR/cov-client-missing-config" "$TMP_DIR/llm-proxy-client.cover" --prompt "$CLIENT_COVERAGE_PROBE_PROMPT"

"$GO_BIN" tool covdata textfmt -i="$TMP_DIR/cov-help" -o="$TMP_DIR/bin-help.coverprofile"
"$GO_BIN" tool covdata textfmt -i="$TMP_DIR/cov-missing-config" -o="$TMP_DIR/bin-missing-config.coverprofile"
"$GO_BIN" tool covdata textfmt -i="$TMP_DIR/cov-missing-openai" -o="$TMP_DIR/bin-missing-openai.coverprofile"
"$GO_BIN" tool covdata textfmt -i="$TMP_DIR/cov-client-missing-config" -o="$TMP_DIR/bin-client-missing-config.coverprofile"

awk '
  FNR == 1 { next }
  {
    split($0, fields, " ")
    block = fields[1]
    statements[block] = fields[2]
    counts[block] += fields[3]
  }
  END {
    print "mode: count"
    for (block in statements) {
      print block, statements[block], counts[block]
    }
  }
' "$TMP_DIR/go-test.coverprofile" "$TMP_DIR/bin-help.coverprofile" "$TMP_DIR/bin-missing-config.coverprofile" "$TMP_DIR/bin-missing-openai.coverprofile" "$TMP_DIR/bin-client-missing-config.coverprofile" > coverage.out

coverage_output="$("$GO_BIN" tool cover -func=coverage.out)"
printf '%s\n' "$coverage_output"

total_coverage="$(printf '%s\n' "$coverage_output" | awk '/^total:/ {print $3}')"
if [[ "$total_coverage" != "100.0%" ]]; then
  printf 'coverage total %s, want 100.0%%\n' "$total_coverage" >&2
  exit 1
fi

uncovered_blocks="$(awk 'FNR > 1 { split($0, fields, " "); if (fields[3] == 0) print fields[1] }' coverage.out)"
if [[ -n "$uncovered_blocks" ]]; then
  printf 'uncovered coverage blocks remain:\n%s\n' "$uncovered_blocks" >&2
  exit 1
fi
