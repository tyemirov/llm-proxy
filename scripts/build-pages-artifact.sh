#!/usr/bin/env bash
set -euo pipefail

[[ -n "${RELEASE_TOOL_DIR:-}" ]] || { echo "error: RELEASE_TOOL_DIR is required" >&2; exit 1; }
site_source="${PAGES_SITE_SOURCE:-site}"
pages_domain="${PAGES_DOMAIN:-llm-proxy.mprlab.com}"
render_directory="$(mktemp -d)"
trap 'rm -rf "${render_directory}"' EXIT

echo "==> [release] Rendering the llm-proxy Pages shell"
go run ./cmd/cli --site-source "${site_source}" --render-site-output "${render_directory}/site"

for forbidden_file in config-ui.yaml llm-proxy-config.json; do
  if find "${render_directory}/site" -name "${forbidden_file}" -print -quit | grep -q .; then
    echo "error: rendered Pages artifact contains forbidden static config file: ${forbidden_file}" >&2
    exit 1
  fi
done
if grep -R -n -e "__LLM_PROXY_CONFIG_URL__" "${render_directory}/site" >/dev/null; then
  echo "error: rendered Pages artifact contains the retired config URL placeholder" >&2
  exit 1
fi
if grep -n -e "data-config-url" "${render_directory}/site/index.html" >/dev/null; then
  echo "error: rendered Pages index.html contains a static config URL attribute" >&2
  exit 1
fi

"${RELEASE_TOOL_DIR}/prepare_pages_artifact.sh" --source "${render_directory}/site" --domain "${pages_domain}"
