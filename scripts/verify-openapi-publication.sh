#!/usr/bin/env bash
set -euo pipefail

canonical_source="${1:?canonical OpenAPI source path is required}"
pages_root="${2:?rendered Pages root is required}"
published_schema="${pages_root}/openapi.yaml"
published_reference="${pages_root}/docs/index.html"

[[ -f "${canonical_source}" ]] || { echo "error: canonical OpenAPI source is missing: ${canonical_source}" >&2; exit 1; }
[[ ! -L "${canonical_source}" ]] || { echo "error: canonical OpenAPI source must not be a symlink: ${canonical_source}" >&2; exit 1; }
[[ -f "${published_schema}" ]] || { echo "error: published OpenAPI schema is missing: ${published_schema}" >&2; exit 1; }
[[ -f "${published_reference}" ]] || { echo "error: derived API reference is missing: ${published_reference}" >&2; exit 1; }

cmp -s "${canonical_source}" "${published_schema}" || {
  echo "error: published OpenAPI schema differs from ${canonical_source}" >&2
  exit 1
}

source_digest="$(shasum -a 256 "${canonical_source}" | awk '{print $1}')"
grep -F -q "data-openapi-source-sha256=\"${source_digest}\"" "${published_reference}" || {
  echo "error: derived API reference provenance differs from ${canonical_source}" >&2
  exit 1
}
grep -F -q 'href="/openapi.yaml"' "${published_reference}" || {
  echo "error: derived API reference does not link the published schema" >&2
  exit 1
}
