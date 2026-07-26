#!/usr/bin/env bash
set -euo pipefail

canonical_source="${1:?canonical OpenAPI source path is required}"
pages_root="${2:?rendered Pages root is required}"
published_schema="${pages_root}/openapi.yaml"

[[ -f "${canonical_source}" ]] || { echo "error: canonical OpenAPI source is missing: ${canonical_source}" >&2; exit 1; }
[[ ! -L "${canonical_source}" ]] || { echo "error: canonical OpenAPI source must not be a symlink: ${canonical_source}" >&2; exit 1; }
[[ -d "${pages_root}" ]] || { echo "error: rendered Pages root is missing: ${pages_root}" >&2; exit 1; }

cp "${canonical_source}" "${published_schema}"
./scripts/verify-openapi-publication.sh "${canonical_source}" "${pages_root}"
