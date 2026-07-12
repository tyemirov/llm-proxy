#!/usr/bin/env bash
set -euo pipefail

[[ -n "${RELEASE_TOOL_DIR:-}" ]] || { echo "error: RELEASE_TOOL_DIR is required" >&2; exit 1; }
site_source="${PAGES_SITE_SOURCE:-site}"
pages_domain="${PAGES_DOMAIN:-llm-proxy.mprlab.com}"
pages_config_url="${PAGES_CONFIG_URL:?PAGES_CONFIG_URL is required}"
render_directory="$(mktemp -d)"
trap 'rm -rf "${render_directory}"' EXIT

echo "==> [release] Rendering the llm-proxy Pages shell"
go run ./cmd/cli --site-source "${site_source}" --site-config-url "${pages_config_url}" --render-site-output "${render_directory}/site"

for forbidden_file in config-ui.yaml llm-proxy-config.json; do
  if find "${render_directory}/site" -name "${forbidden_file}" -print -quit | grep -q .; then
    echo "error: rendered Pages artifact contains forbidden static config file: ${forbidden_file}" >&2
    exit 1
  fi
done
if ! grep -F -q "data-config-url=\"${pages_config_url}\"" "${render_directory}/site/index.html"; then
  echo "error: rendered Pages index.html is missing the production config URL" >&2
  exit 1
fi
if ! grep -F -q "data-mpr-ui-bundle-src=" "${render_directory}/site/index.html"; then
  echo "error: rendered Pages index.html is missing the mpr-ui bundle marker" >&2
  exit 1
fi
if grep -F -q "tauth.js" "${render_directory}/site/index.html"; then
  echo "error: rendered Pages index.html contains a direct tauth.js integration" >&2
  exit 1
fi

"${RELEASE_TOOL_DIR}/prepare_pages_artifact.sh" --source "${render_directory}/site" --domain "${pages_domain}"
