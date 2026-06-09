#!/usr/bin/env bash
set -euo pipefail

if [[ -n "${LLM_PROXY_LIVE_MODEL:-}" && -z "${LLM_PROXY_LIVE_GEMINI_MODEL:-}" ]]; then
  export LLM_PROXY_LIVE_GEMINI_MODEL="${LLM_PROXY_LIVE_MODEL}"
fi

export LLM_PROXY_LIVE_PROVIDERS="${LLM_PROXY_LIVE_PROVIDERS:-gemini}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "${SCRIPT_DIR}/test_live_providers.sh" "$@"
