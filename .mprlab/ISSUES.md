# ISSUES

Working backlog for this repository. Keep entries in the canonical ISSUES.md format.

## BugFixes

- [x] [B001] (P1) Gemini POST responses can return thought or partial text as successful output
  ### Summary
  A production-comparable Russian semantic-stress QA run sent the full prompt through `POST /?provider=gemini` with `model=gemini-3.5-flash` and a large `max_tokens` body value, but the client received a non-JSON response and failed before materialization. The same prompt contract succeeds only when the proxy returns the model's answer text as parseable JSON or returns a structured proxy/provider error.

  ### Impact
  This blocks using Gemini as an alternate semantic reviewer for long JSON-only prompts. Downstream clients cannot distinguish model formatting drift, Gemini thought-part leakage, truncation, and proxy/provider errors; they only see invalid text even though the HTTP request path appears successful.

  ### Reproduction
  From the sibling Russian-language/Camu workflow, run the full pipeline rather than calling the proxy directly:

  ```bash
  /Users/tyemirov/Development/Smith/russian-language/russian_language/pipeline_runner.py \
    --output-dir /Users/tyemirov/Documents/Projects/Camu/fairy-tales/runs/russian-language-puzyr-gemini35flash-256k-body-20260604T055725Z \
    --llm-proxy-base-url "https://llm-proxy.mprlab.com/?provider=gemini" \
    --llm-proxy-model gemini-3.5-flash \
    --llm-proxy-timeout-seconds 300 \
    --llm-proxy-max-tokens 262144 \
    --llm-proxy-single-request \
    /Users/tyemirov/Documents/Projects/Camu/fairy-tales/puzyr-solominka-i-lapot/source/narration.txt
  ```

  The semantic QA stage sends a POST body equivalent to:

  ```json
  {
    "prompt": "<large semantic stress QA prompt>",
    "model": "gemini-3.5-flash",
    "web_search": false,
    "max_tokens": 262144
  }
  ```

  ### Observed
  The full pipeline completed deterministic format, yofication, yofication QA, RuAccent stressor, and stressor QA, then failed in `llm-proxy` with:

  ```text
  semantic review response is not valid JSON and --llm-proxy-single-request forbids retry
  ```

  Latest compact state:

  ```text
  /Users/tyemirov/Documents/Projects/Camu/fairy-tales/runs/russian-language-puzyr-gemini35flash-256k-body-20260604T055725Z/pipeline-state-20260604T055725Z.json
  ```

  Earlier Gemini evidence for the same story captured a 1278-byte response that started with `thought`, then a fenced JSON block, and cut off mid-string:

  ```text
  /Users/tyemirov/Documents/Projects/Camu/fairy-tales/runs/russian-language-puzyr-vanilla-gemini35flash-20260604T052031Z/direct-semantic-evidence/puzyr-gemini35flash.llm_response.txt
  ```

  ### Suspected proxy gaps
  `internal/proxy/gemini.go` currently parses only `candidates[].content.parts[].text`, concatenates every non-empty text part, and does not model Gemini response fields such as part-level `thought` or candidate-level `finishReason`. That can make the proxy return internal/thought text or a MAX_TOKENS-truncated answer as HTTP 200 plain text instead of returning only the final answer text or a provider error.

  ### Expected
  For Gemini text generation, `llm-proxy` should:

  1. Forward `max_tokens` from POST JSON as `generationConfig.maxOutputTokens`.
  2. Return only final, user-visible answer text from Gemini response parts; thought/internal parts must not be concatenated into the client-visible response.
  3. Treat non-terminal or truncated Gemini candidates, including `finishReason` values that imply partial output such as MAX_TOKENS, as a proxy/provider error rather than returning partial text as success.
  4. Preserve the existing plain-text response contract when the provider returns a complete final answer.

  ### Acceptance Criteria
  1. Add black-box/integration-style tests for `POST /?provider=gemini` with a fake Gemini upstream response containing a thought part plus an answer part; the proxy returns only the answer text.
  2. Add a fake Gemini upstream response with a truncation/non-final `finishReason`; the proxy maps it to a failure status instead of returning partial text.
  3. Add or keep coverage proving POST-body `max_tokens` maps to Gemini `generationConfig.maxOutputTokens`.
  4. Rerun the Russian semantic-stress QA prompt through the full pipeline and verify the client receives either parseable JSON or a structured proxy error, not a thought-prefixed or truncated successful text body.

  ### Resolution
  Added black-box Gemini POST coverage for thought-marked parts returning only final answer text, non-final `finishReason` values such as `MAX_TOKENS` mapping to `502`, missing `finishReason` mapping to `502`, and POST/GET `max_tokens` values above Gemini's `65536` output-token limit returning `400` before upstream calls. The existing POST-body `max_tokens` to `generationConfig.maxOutputTokens` mapping remains covered for valid caps. `internal/proxy/gemini.go` now models `parts[].thought` and candidate `finishReason`, returns only non-thought text for completed `STOP` candidates, and treats missing or non-`STOP` finish reasons as provider API errors instead of successful partial output. `internal/proxy/router.go` validates known provider/model output-token ceilings at the HTTP edge so the reproduction's `max_tokens=262144` is classified as invalid client input instead of an upstream `502`. Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 350s -s SIGKILL 350s make test` (total coverage 100.0%), `timeout -k 350s -s SIGKILL 350s make lint`, and `timeout -k 350s -s SIGKILL 350s make ci` (total coverage 100.0%). The full Russian pipeline was rerun against the local fixed proxy at `http://127.0.0.1:18080/?provider=gemini`; it reached the semantic QA stage and received `HTTP Error 400: Bad Request` from request-edge validation, with the proxy log showing status `400` and zero upstream latency. State file: `/Users/tyemirov/Documents/Projects/Camu/fairy-tales/runs/russian-language-puzyr-gemini35flash-local-fixed-B001-max-token-400-20260604T062605Z/pipeline-state-20260604T062605Z.json`.

## Improvements

## Maintenance

## Features

## Planning
*do not implement yet*
