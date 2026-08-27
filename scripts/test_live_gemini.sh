#!/usr/bin/env bash
set -euo pipefail

if [[ -n "${LLM_PROXY_LIVE_MODEL:-}" && -z "${LLM_PROXY_LIVE_GEMINI_MODEL:-}" ]]; then
  export LLM_PROXY_LIVE_GEMINI_MODEL="${LLM_PROXY_LIVE_MODEL}"
fi

export LLM_PROXY_LIVE_PROVIDERS="${LLM_PROXY_LIVE_PROVIDERS:-gemini}"
export LLM_PROXY_LIVE_REASONING_MATRIX="${LLM_PROXY_LIVE_REASONING_MATRIX:-true}"
LLM_PROXY_LIVE_GEMINI_CANDIDATES="${LLM_PROXY_LIVE_GEMINI_CANDIDATES:-true}"
if [[ "${LLM_PROXY_LIVE_GEMINI_CANDIDATES}" != "true" && "${LLM_PROXY_LIVE_GEMINI_CANDIDATES}" != "false" ]]; then
  echo "error: LLM_PROXY_LIVE_GEMINI_CANDIDATES must be true or false" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ "${LLM_PROXY_LIVE_GEMINI_CANDIDATES}" == "true" && $# -eq 0 ]]; then
  "${SCRIPT_DIR}/test_live_providers.sh" --gemini-candidates
fi
exec "${SCRIPT_DIR}/test_live_providers.sh" "$@"
