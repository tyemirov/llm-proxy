#!/usr/bin/env bash
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
temporary_directory="$(mktemp -d)"
capability_api_pid=""

cleanup() {
  if [[ -n "${capability_api_pid}" ]] && kill -0 "${capability_api_pid}" 2>/dev/null; then
    kill "${capability_api_pid}"
    wait "${capability_api_pid}" 2>/dev/null || true
  fi
  rm -rf "${temporary_directory}"
}
trap cleanup EXIT

cd "${repository_root}"
[[ ! -e site/openapi.yaml ]] || {
  echo "error: site/openapi.yaml is a forbidden duplicate schema source" >&2
  exit 1
}

capability_config_path="${temporary_directory}/capabilities.yml"
capability_port="$(node scripts/create_public_capability_test_config.mjs configs/config.yml "${capability_config_path}")"
capabilities_url="http://127.0.0.1:${capability_port}/api/public/capabilities"
capability_api_binary="${temporary_directory}/llm-proxy"
go build -o "${capability_api_binary}" ./cmd/cli
"${capability_api_binary}" --config "${capability_config_path}" --public-capabilities-only \
  >"${temporary_directory}/capability-api.log" 2>&1 &
capability_api_pid="$!"
capability_api_ready="false"
for ((attempt = 0; attempt < 300; attempt++)); do
  if ! kill -0 "${capability_api_pid}" 2>/dev/null; then
    cat "${temporary_directory}/capability-api.log" >&2
    exit 1
  fi
  if CAPABILITIES_URL="${capabilities_url}" node --input-type=module --eval \
    'const response = await fetch(process.env.CAPABILITIES_URL); process.exit(response.ok ? 0 : 1);' \
    >/dev/null 2>&1; then
    if ! kill -0 "${capability_api_pid}" 2>/dev/null; then
      cat "${temporary_directory}/capability-api.log" >&2
      exit 1
    fi
    capability_api_ready="true"
    break
  fi
  sleep 0.1
done
if [[ "${capability_api_ready}" != "true" ]]; then
  cat "${temporary_directory}/capability-api.log" >&2
  exit 1
fi
node scripts/render_public_site.mjs \
  --source site \
  --output "${temporary_directory}/site" \
  --config-url https://llm-proxy-api.mprlab.com/config-ui.yaml \
  --capabilities-url "${capabilities_url}"
kill "${capability_api_pid}"
wait "${capability_api_pid}" 2>/dev/null || true
capability_api_pid=""
if ! CAPABILITIES_URL="${capabilities_url}" node --input-type=module --eval \
  'try { await fetch(process.env.CAPABILITIES_URL, { signal: AbortSignal.timeout(1000) }); } catch (requestError) { if (requestError instanceof TypeError) process.exit(0); throw requestError; } throw new Error(`public_capability_server_listener_open: ${process.env.CAPABILITIES_URL}`);' \
  >/dev/null 2>&1; then
  echo "error: public capability server listener remained open after shutdown" >&2
  exit 1
fi
./scripts/stage-openapi-publication.sh docs/openapi.yaml "${temporary_directory}/site"
cmp -s docs/openapi.yaml "${temporary_directory}/site/openapi.yaml"
test -f "${temporary_directory}/site/docs/index.html"

printf '\n' >>"${temporary_directory}/site/openapi.yaml"
if ./scripts/verify-openapi-publication.sh docs/openapi.yaml "${temporary_directory}/site" >/dev/null 2>&1; then
  echo "error: publication verification accepted a modified OpenAPI artifact" >&2
  exit 1
fi
