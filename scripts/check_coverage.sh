#!/usr/bin/env bash
set -euo pipefail

GO_BIN="${GO:-go}"
COVERPKG="./cmd/cli,./internal/apperrors,./internal/constants,./internal/proxy,./internal/utils"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

cd "$ROOT_DIR"

"$GO_BIN" test -count=1 ./... -covermode=count -coverpkg="$COVERPKG" -coverprofile="$TMP_DIR/go-test.coverprofile"
"$GO_BIN" build -cover -covermode=count -coverpkg="$COVERPKG" -o "$TMP_DIR/llm-proxy.cover" ./cmd/cli

mkdir -p "$TMP_DIR/cov-help" "$TMP_DIR/cov-empty-env" "$TMP_DIR/cov-missing-openai"
env -i GOCOVERDIR="$TMP_DIR/cov-help" "$TMP_DIR/llm-proxy.cover" --help >/dev/null 2>/dev/null || true
env -i GOCOVERDIR="$TMP_DIR/cov-empty-env" "$TMP_DIR/llm-proxy.cover" >/dev/null 2>/dev/null || true
env -i SERVICE_SECRET=" service-secret " GOCOVERDIR="$TMP_DIR/cov-missing-openai" "$TMP_DIR/llm-proxy.cover" >/dev/null 2>/dev/null || true

"$GO_BIN" tool covdata textfmt -i="$TMP_DIR/cov-help" -o="$TMP_DIR/bin-help.coverprofile"
"$GO_BIN" tool covdata textfmt -i="$TMP_DIR/cov-empty-env" -o="$TMP_DIR/bin-empty-env.coverprofile"
"$GO_BIN" tool covdata textfmt -i="$TMP_DIR/cov-missing-openai" -o="$TMP_DIR/bin-missing-openai.coverprofile"

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
' "$TMP_DIR/go-test.coverprofile" "$TMP_DIR/bin-help.coverprofile" "$TMP_DIR/bin-empty-env.coverprofile" "$TMP_DIR/bin-missing-openai.coverprofile" > coverage.out

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
