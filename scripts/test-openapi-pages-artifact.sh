#!/usr/bin/env bash
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
temporary_directory="$(mktemp -d)"
trap 'rm -rf "${temporary_directory}"' EXIT

cd "${repository_root}"
[[ ! -e site/openapi.yaml ]] || {
  echo "error: site/openapi.yaml is a forbidden duplicate schema source" >&2
  exit 1
}

go run ./cmd/cli \
  --config configs/config.yml \
  --site-source site \
  --site-config-url https://llm-proxy-api.mprlab.com/config-ui.yaml \
  --render-site-output "${temporary_directory}/site"
./scripts/stage-openapi-publication.sh docs/openapi.yaml "${temporary_directory}/site"
cmp -s docs/openapi.yaml "${temporary_directory}/site/openapi.yaml"
test -f "${temporary_directory}/site/docs/index.html"

printf '\n' >>"${temporary_directory}/site/openapi.yaml"
if ./scripts/verify-openapi-publication.sh docs/openapi.yaml "${temporary_directory}/site" >/dev/null 2>&1; then
  echo "error: publication verification accepted a modified OpenAPI artifact" >&2
  exit 1
fi
