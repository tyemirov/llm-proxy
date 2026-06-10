# ISSUES

Entries record newly discovered requests or changes.

Read @AGENTS.md (Workflow section), @POLICY.md, and relevant stack guides before implementing changes.

Format: `- [ ] [B042] (P1) {I007} Title`

- `[ ]` open, `[!]` blocked, `[x]` closed.
- Blocked issues (`[!]`) must include a `Blocked:` line in the body.

## BugFixes

- [x] [B001] (P1) Gemini POST responses can return thought or partial text as successful output.
  ### Summary
  A production-comparable Russian semantic-stress QA run sent the full prompt through `POST /?provider=gemini` with `model=gemini-3.5-flash`, but the client received a non-JSON response and failed before materialization. The same prompt contract succeeds only when the proxy returns the model's answer text as parseable JSON or returns a structured proxy/provider error.
  ### Impact
  This blocks using Gemini as an alternate semantic reviewer for long JSON-only prompts. Downstream clients cannot distinguish model formatting drift, Gemini thought-part leakage, truncation, and proxy/provider errors; they only see invalid text even though the HTTP request path appears successful.
  ### Reproduction
  From the sibling Russian-language/Camu workflow, run the full pipeline rather than calling the proxy directly:
  ```bash
  /Users/tyemirov/Development/Smith/russian-language/russian_language/pipeline_runner.py \
    --output-dir /Users/tyemirov/Documents/Projects/Camu/fairy-tales/runs/russian-language-puzyr-vanilla-gemini35flash-20260604T052031Z \
    --llm-proxy-base-url "https://llm-proxy.mprlab.com/?provider=gemini" \
    --llm-proxy-model gemini-3.5-flash \
    --llm-proxy-timeout-seconds 300 \
    --llm-proxy-single-request \
    /Users/tyemirov/Documents/Projects/Camu/fairy-tales/puzyr-solominka-i-lapot/source/narration.txt
  ```
  The semantic QA stage sends a POST body equivalent to:
  ```json
  {
    "prompt": "<large semantic stress QA prompt>",
    "model": "gemini-3.5-flash",
    "web_search": false
  }
  ```
  ### Observed
  The full pipeline completed deterministic format, yofication, yofication QA, RuAccent stressor, and stressor QA, then failed in `llm-proxy` with:
  ```text
  semantic review response is not valid JSON and --llm-proxy-single-request forbids retry
  ```
  Earlier Gemini evidence for the same story captured a 1278-byte response that started with `thought`, then a fenced JSON block, and cut off mid-string:
  ```text
  /Users/tyemirov/Documents/Projects/Camu/fairy-tales/runs/russian-language-puzyr-vanilla-gemini35flash-20260604T052031Z/direct-semantic-evidence/puzyr-gemini35flash.llm_response.txt
  ```
  ### Suspected proxy gaps
  `internal/proxy/gemini.go` currently parses only `candidates[].content.parts[].text`, concatenates every non-empty text part, and does not model Gemini response fields such as part-level `thought` or candidate-level `finishReason`. That can make the proxy return internal/thought text or a non-final answer as HTTP 200 plain text instead of returning only the final answer text or a provider error.
  ### Expected
  For Gemini text generation, `llm-proxy` should:
  1. Return only final, user-visible answer text from Gemini response parts; thought/internal parts must not be concatenated into the client-visible response.
  2. Treat non-terminal or truncated Gemini candidates as a proxy/provider error rather than returning partial text as success.
  3. Preserve the existing plain-text response contract when the provider returns a complete final answer.
  ### Acceptance Criteria
  1. Add black-box/integration-style tests for `POST /?provider=gemini` with a fake Gemini upstream response containing a thought part plus an answer part; the proxy returns only the answer text.
  2. Add a fake Gemini upstream response with a truncation/non-final `finishReason`; the proxy maps it to a failure status instead of returning partial text.
  3. Rerun the Russian semantic-stress QA prompt through the full pipeline and verify the client receives either parseable JSON or a structured proxy error, not a thought-prefixed or truncated successful text body.
  ### Resolution
  Added black-box Gemini POST coverage for thought-marked parts returning only final answer text, non-final `finishReason` values mapping to `502`, and missing `finishReason` mapping to `502`. `internal/proxy/gemini.go` now models `parts[].thought` and candidate `finishReason`, returns only non-thought text for completed `STOP` candidates, and treats missing or non-`STOP` finish reasons as provider API errors instead of successful partial output. Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 350s -s SIGKILL 350s make test` (total coverage 100.0%), `timeout -k 350s -s SIGKILL 350s make lint`, and `timeout -k 350s -s SIGKILL 350s make ci` (total coverage 100.0%).
- [ ] [B002] (P1) Long semantic-review POSTs fail transport while small requests pass.
  ### Summary
  Camu Russian semantic-stress QA can still fail before materialization on long full-story prompts even though a small `llm-proxy` smoke request succeeds. On 2026-06-09, `po-schuchemu-velenyu` text prep failed three times at the `llm-proxy` stage: one read timeout, then two SSL record-layer failures. A tiny Russian pipeline smoke test using the same Russian-language `pipeline_runner.py` reached `semantic-stress-qa` and `materialize` successfully, so the service is not fully down; the failure appears tied to long-running or larger semantic-review POSTs and/or chunk retry transport handling.
  ### Impact
  Downstream production workflows cannot safely materialize corrected TTS for long stories. In the observed Camu run, this blocked applying an occurrence-scoped pronunciation correction for `Слез` versus `Слёз`; regenerating audio from the existing stale `tts.txt` would reproduce the old pronunciation.
  ### Reproduction
  From the Camu checkout, run the long story materialization:
  ```bash
  tools/camu_audio.py prepare-text \
    fairy-tales/po-schuchemu-velenyu/source/narration.txt \
    --output fairy-tales/po-schuchemu-velenyu/pronunciation \
    --skip-speech-performance
  ```
  Retried variants also failed:
  ```bash
  tools/camu_audio.py prepare-text \
    fairy-tales/po-schuchemu-velenyu/source/narration.txt \
    --output fairy-tales/po-schuchemu-velenyu/pronunciation \
    --skip-speech-performance \
    --llm-proxy-timeout-seconds 240 \
    --llm-proxy-chunk-chars 3000
  LLM_PROXY_FORCE_CHUNKED=1 tools/camu_audio.py prepare-text \
    fairy-tales/po-schuchemu-velenyu/source/narration.txt \
    --output fairy-tales/po-schuchemu-velenyu/pronunciation \
    --skip-speech-performance \
    --llm-proxy-model gpt-5-mini \
    --llm-proxy-timeout-seconds 240 \
    --llm-proxy-chunk-chars 1800
  ```
  As a control, this tiny pipeline passed through `semantic-stress-qa` and `materialize`:
  ```bash
  mkdir -p /tmp/camu-llm-proxy-smoke
  printf 'Жил-был кот. Он любил тёплое молоко.\n' | \
    /Users/tyemirov/Development/Smith/russian-language/russian_language/pipeline_runner.py \
      --output-dir /tmp/camu-llm-proxy-smoke \
      --basename smoke \
      --to materialize \
      --quiet
  ```
  ### Observed
  The long story runs completed deterministic Russian-language stages and then failed at `llm-proxy` with:
  ```text
  llm_proxy_client_transport_failure: The read operation timed out
  [SSL] record layer failure (_ssl.c:2658)
  ```
  The forced-chunked `gpt-5-mini` retry also failed with the same SSL record-layer error before materialization. The small control request reported `semantic-stress-qa` passed with `transport: post`, `invocationMode: single`, and `materialize` passed.
  ### Expected
  Long semantic-review POSTs should either complete successfully or return a structured proxy/provider error that the client can classify and retry. The proxy should not leave clients with opaque transport failures after the upstream request may still be in progress or recoverable.
  ### Acceptance Criteria
  1. Add production-comparable black-box coverage for a long POST body where the upstream completes after the normal client wait; the proxy either streams/polls/waits correctly or returns a structured timeout error with retry guidance.
  2. Add coverage for chunked semantic-review retry behavior so chunk transport failures are reported with chunk index, provider, model, timeout, and upstream status when available.
  3. Verify the Russian-language `po-schuchemu-velenyu` materialization path reaches `materialize` or returns a structured proxy error instead of `read operation timed out` or SSL record-layer failure.
  4. Preserve the existing successful behavior for small POST requests.

  ### Resolution
  OpenAI Responses text requests now run in background mode by sending `background: true` and `store: true` on initial, continuation, and synthesis payloads, then polling the returned response id instead of holding long semantic-review generations on one synchronous upstream HTTP read. Added black-box integration coverage for a long semantic-review JSON POST that returns a queued background response, polls by response id, and returns the completed body. Python client HTTP and transport failures now include non-secret provider, model, and timeout context, and raw OS/SSL-style failures are typed as `LLMProxyTransportError`. B003 supersedes the remaining manual tuning gap with the final one-shot REST contract.

  Validation for the initial background-mode fix passed with `timeout -k 350s -s SIGKILL 350s make go-test` (Go total coverage 100.0%), `timeout -k 350s -s SIGKILL 350s make python-test` (27 passed), `timeout -k 350s -s SIGKILL 350s make python-lint`, `timeout -k 350s -s SIGKILL 350s make go-lint`, and `timeout -k 350s -s SIGKILL 350s make ci` (Go total coverage 100.0%, Python 27 passed). The exact reported forced-chunked semantic-review payload was reconstructed from the failed Camu pipeline state (`chunkIndex=1`, `chunkChars=1800`, request SHA-256 `ba8737a7dd4b81dc0d52e68d9260756fdb0f32e6e12f3be2527d8be55df1a0cd`) and proved that OpenAI could complete the response in about 171 seconds, motivating B003.

- [x] [B003] (P1) OpenAI background semantic-review calls require manual timeout tuning
  ### Summary
  B002 moved long OpenAI Responses calls into background mode, but the successful manual replay still required hand-editing timeout knobs. A normal llm-proxy REST call should not require operators, downstream workflows, or client libraries to guess provider polling behavior for a semantic-review payload.

  ### Impact
  Downstream callers can still fail a viable OpenAI background response, then retry from scratch or require caller-specific timeout overrides. This keeps long semantic review fragile even though the provider can complete the response.

  ### Reproduction
  Reuse the reported `po-schuchemu-velenyu` forced-chunked semantic-review request reconstructed from the failed Camu pipeline state:

  ```text
  /tmp/llm-proxy-b002-direct/po-schuchemu-velenyu.chunk-0001.llm-proxy-request.json
  ```

  Before this fix, the default proxy returned a classified 504 after about 60 seconds. With a temporary larger timeout, the same request returned HTTP 200 after about 171 seconds and validated as a full semantic-review response.

  ### Expected
  The proxy should own long OpenAI background response completion. A simple REST caller should issue one `GET /`, `POST /`, or `POST /v2` request through llm-proxy and receive the final answer without streaming, client-side polling, or a resume-token protocol.

  ### Acceptance Criteria
  1. OpenAI background response polling is server-side and bounded only by `server.request_timeout_seconds`.
  2. The public `GET /`, `POST /`, and `POST /v2` contract does not require streaming, client-side polling, or `/responses` resume calls.
  3. The obsolete poll-timeout configuration is removed from the public static config surface.
  4. Bundled Go and Python clients remain simple one-request REST transports; they do not implement OpenAI background polling.
  5. Black-box integration tests cover a long background semantic-review POST returning HTTP 200 in the original request after multiple server-side polls.

  ### Resolution
  OpenAI background response polling is now owned by llm-proxy for the normal REST request lifecycle. Initial, continuation, and synthesis OpenAI Responses payloads use `background: true` and `store: true`; when OpenAI returns a non-terminal response id, llm-proxy polls that stored response server-side until completion or the normal `server.request_timeout_seconds` deadline. The public `/responses` resume endpoint and the Go/Python client resume loops were removed, and `upstream_poll_timeout_seconds` was removed from the static config surface. Default `request_timeout_seconds` is now 240 seconds, while the packaged Go CLI and Python client default to a 260-second transport timeout so one normal REST request has room to complete.

  Validation passed with `timeout -k 350s -s SIGKILL 350s make go-test` (Go total coverage 100.0%), `timeout -k 350s -s SIGKILL 350s make python-test` (Python 27 passed), `timeout -k 350s -s SIGKILL 350s make ci` (Go/Python lint clean, Go total coverage 100.0%, Python 27 passed), and `timeout -k 30s -s SIGKILL 30s git diff --check`. Live OpenAI smoke passed with configured-default model and HTTP 200. Live exact-payload replay of `/tmp/llm-proxy-b002-direct/po-schuchemu-velenyu.chunk-0001.llm-proxy-request.json` through the local current-branch proxy returned HTTP 200 from one `POST /?provider=openai&format=text/plain` in 150.394s with no `X-LLM-Proxy-Resume-*` or raw upstream response-id headers. The final body was valid JSON with 23 `targetReviews`, `needsHumanReview=true`, and 2 human-review items for `проруби`/`прорубь` stress confirmation.

## Improvements

- [x] [I001] (P1) Make missing placeholder handling field-aware.
  ### Summary
  Review found that allowing missing `${...}` placeholders globally can silently mutate non-key configuration values. Keep the new disabled-provider behavior for missing provider API-key placeholders, but fail startup for missing placeholders everywhere else and for partial API-key placeholders.
  ### Acceptance Criteria
  1. Missing placeholders outside provider `api_key` fields fail startup with `config_placeholder_missing`.
  2. A provider `api_key` value that is exactly a missing `${...}` placeholder expands to an empty key so a non-default provider can be disabled.
  3. Missing placeholders embedded inside longer provider `api_key` values fail startup rather than creating malformed credentials.
  4. README and provider-routing docs describe the field-aware placeholder behavior.
  ### Resolution
  Config placeholder expansion now allows a missing placeholder only when it is the whole provider `api_key` value; missing placeholders in tenant secrets, URLs, and partial API-key strings fail with `config_placeholder_missing`. Added black-box CLI coverage for missing non-key placeholders, exact optional provider API-key placeholders, partial API-key placeholders, and missing default-provider credentials. README and provider-routing docs now describe the field-aware placeholder rule. Validation passed with `timeout -k 180s -s SIGKILL 180s go test -count=1 ./cmd/cli -run 'TestRootCommand(RejectsMissingTenantSecretPlaceholder|AllowsMissingNonDefaultProviderKey|RejectsPartialMissingProviderKeyPlaceholder|RejectsMissingDefaultDictationProviderKey|RejectsMissingDefaultTextProviderKeys)'` and `timeout -k 350s -s SIGKILL 350s make ci`.
- [x] [I002] (P1) Require API keys only for tenant default providers.
  ### Summary
  Provider configuration should treat supported providers symmetrically: every provider keeps explicit URLs and model catalogs, but API credentials are only startup-required when a tenant uses that provider as a default text or dictation provider. Non-default providers with missing credentials should remain selectable by name but return a clear `provider not configured` response to that tenant instead of preventing service startup.
  ### Acceptance Criteria
  1. Static config loading no longer fails solely because a non-default provider API-key placeholder is missing or expands to blank.
  2. Static config loading fails startup when any tenant default text provider lacks its provider API key.
  3. Static config loading fails startup when any tenant default dictation provider lacks its provider API key.
  4. Requests that explicitly select a configured non-default provider without an API key return `503 provider not configured`.
  5. README and provider-routing docs describe provider keys as optional for non-default providers and required for tenant defaults.
  ### Resolution
  Static config placeholder expansion now turns missing `${...}` values into empty strings, allowing non-default provider API keys to be omitted without blocking startup. Config validation now requires API keys only for tenant default text providers and tenant default dictation providers that support dictation; unknown or unsupported defaults continue through the runtime provider/model validation path. Explicit requests for configured non-default providers without API keys return `503 provider not configured` with provider and endpoint detail. README and provider-routing docs now document optional non-default provider keys, disabled-provider request behavior, and startup-fatal default provider credentials. Validation passed with `timeout -k 180s -s SIGKILL 180s go test -count=1 ./cmd/cli`, `timeout -k 240s -s SIGKILL 240s go test -count=1 ./internal/proxy`, `timeout -k 240s -s SIGKILL 240s go test -count=1 ./tests/...`, `timeout -k 30s -s SIGKILL 30s git diff --check`, and `timeout -k 350s -s SIGKILL 350s make ci` (Go total coverage 100.0%, Python 26 passed).
- [x] [I003] (P1) Address provider config review followups.
  ### Summary
  Review found three gaps in the explicit provider config branch: non-OpenAI model catalogs can advertise `web_search` even though only OpenAI consumes it, programmatic runtime configuration still silently falls back to a hardcoded model catalog, and live provider smoke tests duplicate default model ids instead of exercising the configured provider defaults.
  ### Acceptance Criteria
  1. Static config validation rejects `web_search: true` for any non-OpenAI text model catalog entry and for dictation catalogs.
  2. Runtime `proxy.Configuration` no longer injects a hardcoded provider model catalog when `ProviderModels` is omitted.
  3. Tests that build routers/configuration programmatically pass explicit provider model catalogs through test fixtures rather than relying on runtime fallback.
  4. `make test-live-providers` omits the `model` field by default and sends a model only when a per-provider override is set.
  5. README documents that live smoke defaults come from `configs/config.yml` and override variables are optional.
  ### Resolution
  Static provider model catalog validation now rejects `web_search: true` outside OpenAI text model entries, and the CLI config test matrix covers non-OpenAI text and dictation catalog failures. Runtime configuration no longer injects a hardcoded model catalog fallback; programmatic tests load explicit provider model catalogs from `configs/config.yml` through test fixtures, preserving custom catalog tests. The live provider smoke runner now omits `model` by default so configured provider defaults are exercised, and only sends `model` when a per-provider override variable is set. Because the new configured-default live run exposed Gemini `gemini-3.5-flash` returning provider 503 while `gemini-2.5-flash` passed, the Gemini default in `configs/config.yml`, README, and representative CLI test fixtures was changed to `gemini-2.5-flash`. Validation passed with `timeout -k 120s -s SIGKILL 120s go test -count=1 ./cmd/cli -run TestRootCommandRejectsIncompleteStaticProviderConfig`, `timeout -k 180s -s SIGKILL 180s go test -count=1 ./internal/proxy`, `timeout -k 240s -s SIGKILL 240s go test -count=1 ./tests/...`, `bash -n scripts/test_live_providers.sh scripts/test_live_gemini.sh`, `scripts/test_live_providers.sh --help`, no-key skip via `env -i PATH="$PATH" HOME="$HOME" TMPDIR="${TMPDIR:-/tmp}" GOENV=off scripts/test_live_providers.sh`, explicit missing-key failure via `env -i PATH="$PATH" HOME="$HOME" TMPDIR="${TMPDIR:-/tmp}" LLM_PROXY_LIVE_PROVIDERS=openai GOENV=off scripts/test_live_providers.sh`, targeted Gemini override proof via `timeout -k 180s -s SIGKILL 180s env LLM_PROXY_LIVE_PROVIDERS=gemini LLM_PROXY_LIVE_GEMINI_MODEL=gemini-2.5-flash make test-live-providers LIVE_ENV_FILE=configs/.env`, dynamic live smoke via `timeout -k 180s -s SIGKILL 180s make test-live-providers LIVE_ENV_FILE=configs/.env` (OpenAI and Gemini passed with configured defaults), `timeout -k 350s -s SIGKILL 350s make ci`, and `timeout -k 30s -s SIGKILL 30s git diff --check`.
- [x] [I004] (P1) Add dynamic live provider smoke tests.
  ### Summary
  The repository has a stale Gemini-only live smoke target that now fails against the complete static config contract, and it does not exercise OpenAI even when `OPENAI_API_KEY` is present. Add a provider-aware live smoke target that builds a complete temporary config, runs text smoke tests for providers with available API keys, and keeps targeted runs explicit for debugging.
  ### Acceptance Criteria
  1. A canonical Makefile target runs live text smoke tests for every supported text provider whose API key is present in the environment or loaded `LIVE_ENV_FILE`.
  2. Missing provider keys do not fail the dynamic target unless the provider is explicitly requested.
  3. The temporary live config satisfies the complete static config contract without requiring real keys for providers that are not being tested.
  4. OpenAI is covered when `OPENAI_API_KEY` is present.
  5. The old Gemini live target still works as a Gemini-focused wrapper.
  6. README documents dynamic provider discovery, targeted provider selection, per-provider model overrides, and that live tests are not part of normal CI.
  ### Resolution
  Added `scripts/test_live_providers.sh` and `make test-live-providers` as the canonical live text smoke path. The runner sources optional `LIVE_ENV_FILE`, discovers provider keys, fills unused provider placeholders with dummy values so the complete static config contract is satisfied, builds a temporary binary/config, and calls only providers with real keys unless `LLM_PROXY_LIVE_PROVIDERS` explicitly selects a provider. OpenAI now runs when `OPENAI_API_KEY` is present. `scripts/test_live_gemini.sh` remains as a Gemini-focused compatibility wrapper and maps the old `LLM_PROXY_LIVE_MODEL` override to `LLM_PROXY_LIVE_GEMINI_MODEL`. README now documents dynamic discovery, targeted provider selection, model overrides, and that live tests are not part of normal CI. Validation passed with `scripts/test_live_providers.sh --help`, no-key skip via `env -i PATH="$PATH" HOME="$HOME" TMPDIR="${TMPDIR:-/tmp}" GOENV=off scripts/test_live_providers.sh`, explicit missing-key failure via `env -i PATH="$PATH" HOME="$HOME" TMPDIR="${TMPDIR:-/tmp}" LLM_PROXY_LIVE_PROVIDERS=openai GOENV=off scripts/test_live_providers.sh`, `timeout -k 180s -s SIGKILL 180s make test-live-providers LIVE_ENV_FILE=configs/.env` (live OpenAI and Gemini passed), `timeout -k 180s -s SIGKILL 180s make test-live-gemini LIVE_ENV_FILE=configs/.env` (live Gemini passed), `bash -n scripts/test_live_providers.sh scripts/test_live_gemini.sh`, `timeout -k 30s -s SIGKILL 30s git diff --check`, and `timeout -k 350s -s SIGKILL 350s make ci`.
- [x] [I005] (P1) Move provider model catalogs into config.yml.
  ### Summary
  Provider model ids, provider default models, dictation model ids, web-search support, request payload profiles, and provider-side output-token limits currently live in Go constants and provider registry tables. Move those changing model catalogs into `config.yml` so runtime model support changes are made through the authoritative YAML config, while code retains only stable provider transports and known request-profile implementations.
  ### Acceptance Criteria
  1. `configs/config.yml` declares each provider's text default model and allowed text models.
  2. Dictation-capable providers declare their dictation default model and allowed dictation models in `configs/config.yml`.
  3. Runtime static config loading rejects missing provider model catalogs, blank model ids, duplicate model ids, and defaults not present in the configured model list.
  4. Provider routing validates and routes text/dictation requests using configured model catalogs rather than hardcoded per-provider model sets.
  5. OpenAI model request-shape behavior is selected from configured stable request profiles, and model-specific web-search support/output-token limits come from model metadata.
  6. README and provider-routing docs document the model catalog schema and distinguish configurable model data from code-owned provider transports.
  7. Black-box tests prove adding a provider model in config routes upstream without code changes and invalid configured model catalogs fail startup.
  ### Resolution
  `configs/config.yml` now declares text catalogs for every supported provider and dictation catalogs for OpenAI, SiliconFlow, Zhipu, and Grok/xAI. Runtime configuration carries those catalogs through `ProviderModels`, validates missing/blank/duplicate/default-missing catalog data at startup, and builds provider routing from configured text and dictation models. OpenAI request payload shape now comes from configured stable `request_profile` values, while model-specific `web_search` support and output-token limits come from model metadata. README and `docs/implementation/provider-routing-plan.md` document the model catalog schema, default support matrix, dictation catalogs, and the split between config-owned model data and code-owned provider transports. Black-box CLI/config tests cover invalid catalog startup failures, and router/integration tests prove a configured model routes upstream without code changes and unsupported model-specific web search fails at the request edge. Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 60s -s SIGKILL 60s go test -count=1 ./cmd/cli -run TestRootCommandRejectsIncompleteStaticProviderConfig`, `timeout -k 60s -s SIGKILL 60s go test -count=1 ./internal/proxy -run 'TestProviderRouting(UsesConfiguredTextModelCatalog|RejectsMissingConfiguredProviderCatalog)'`, `timeout -k 180s -s SIGKILL 180s go test -count=1 ./tests/integration`, `timeout -k 350s -s SIGKILL 350s make test` (Go total coverage 100.0%, Python 26 passed), `timeout -k 350s -s SIGKILL 350s make lint`, `timeout -k 350s -s SIGKILL 350s make ci`, and `timeout -k 30s -s SIGKILL 30s git diff --check`.
- [x] [I006] (P1) Add Grok/xAI and Zhipu dictation support.
  ### Summary
  Upstream xAI and Z.AI currently expose speech-to-text APIs, but the proxy only supports `/dictate` for OpenAI and SiliconFlow. Add explicit Grok/xAI and Zhipu transcription URL configuration and wire `/dictate` to xAI STT and Zhipu GLM-ASR, while leaving text-only providers without dictation config fields.
  ### Acceptance Criteria
  1. `configs/config.yml` includes `providers.zhipu.transcriptions_url` and `providers.grok.transcriptions_url`.
  2. Runtime config loading accepts and requires Grok and Zhipu transcription URLs.
  3. `/dictate?provider=zhipu` sends multipart audio to the configured Zhipu transcription URL with model `glm-asr-2512`.
  4. `/dictate?provider=grok` sends multipart audio to the configured xAI STT URL without a model form field.
  5. README and provider-routing docs distinguish proxy-supported dictation from upstream-only capabilities and list OpenAI, SiliconFlow, Zhipu, and Grok/xAI as proxy-supported dictation providers.
  6. Black-box tests cover Grok and Zhipu dictation routing plus required transcription URL config validation.
  ### Resolution
  `configs/config.yml` now includes explicit `providers.zhipu.transcriptions_url` and `providers.grok.transcriptions_url` entries, and static config validation requires those fields alongside the provider API keys and base URLs. Runtime provider routing now supports `/dictate` for Zhipu through Z.AI GLM-ASR with `model=glm-asr-2512`, and Grok/xAI through xAI STT without sending a multipart `model` field. README and `docs/implementation/provider-routing-plan.md` now distinguish proxy-supported dictation from upstream-only speech APIs and list OpenAI, SiliconFlow, Zhipu, and Grok/xAI as dictation-capable providers. Black-box CLI coverage validates required Zhipu and Grok transcription URLs, and black-box router coverage verifies the multipart request shape and credential routing for both providers. Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 120s -s SIGKILL 120s go test -count=1 ./cmd/cli -run 'TestRootCommand(RunsConfiguredProxyFromConfigFile|RunsProductionLoggerFromConfigFile|RejectsIncompleteStaticProviderConfig)'`, `timeout -k 120s -s SIGKILL 120s go test -count=1 ./internal/proxy -run 'TestProviderRouting(SupportsZhipuAndGrokDictation|RejectsInvalidDefaultDictationProvider|RejectsAnthropicAndGrokUnsupportedCapabilities)'`, `timeout -k 350s -s SIGKILL 350s make test` (Go total coverage 100.0%, Python 26 passed), `timeout -k 350s -s SIGKILL 350s make lint`, `timeout -k 350s -s SIGKILL 350s make ci`, and `timeout -k 30s -s SIGKILL 30s git diff --check`.
- [x] [I007] (P1) Make OpenAI dictation URL explicit in static provider config.
  ### Summary
  Only OpenAI and SiliconFlow currently support `/dictate`. SiliconFlow already exposes `providers.siliconflow.transcriptions_url`, while OpenAI dictation still derives `/audio/transcriptions` from `providers.openai.base_url`. Add an explicit `providers.openai.transcriptions_url` so every dictation-capable provider declares the actual dictation endpoint URL in `config.yml`, without adding unsupported dictation fields to text-only providers.
  ### Acceptance Criteria
  1. `configs/config.yml` includes `providers.openai.transcriptions_url: "https://api.openai.com/v1/audio/transcriptions"`.
  2. Runtime config loading accepts and requires `providers.openai.transcriptions_url`.
  3. OpenAI dictation uses the configured transcription URL when no test endpoint override is supplied.
  4. README and provider-routing docs state that only dictation-capable providers have transcription URL fields.
  5. Black-box tests cover OpenAI transcription URL loading, required config validation, and dictation routing.
  ### Resolution
  `providers.openai.transcriptions_url` is now part of the static OpenAI provider config and is required alongside `providers.openai.api_key` and `providers.openai.base_url`. Runtime configuration carries it as `OpenAITranscriptionsURL`, and the OpenAI endpoint bundle now derives Responses and Models from `providers.openai.base_url` while routing `/dictate` through the configured transcription URL. `configs/config.yml`, README, and `docs/implementation/provider-routing-plan.md` now document that only dictation-capable providers expose `transcriptions_url` fields: OpenAI and SiliconFlow. Black-box config tests cover OpenAI transcription URL loading and startup rejection when it is blank, and router coverage proves OpenAI text and dictation can route through distinct configured URLs. Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 120s -s SIGKILL 120s go test -count=1 ./cmd/cli -run 'TestRootCommand(RunsConfiguredProxyFromConfigFile|RunsProductionLoggerFromConfigFile|RejectsIncompleteStaticProviderConfig)'`, `timeout -k 120s -s SIGKILL 120s go test -count=1 ./internal/proxy -run TestProviderRoutingUsesConfiguredOpenAIURLsForTextAndDictation`, `timeout -k 350s -s SIGKILL 350s make test` (Go total coverage 100.0%, Python 26 passed), `timeout -k 350s -s SIGKILL 350s make lint`, `timeout -k 350s -s SIGKILL 350s make ci`, and `timeout -k 30s -s SIGKILL 30s git diff --check`.
- [x] [I008] (P1) Add OpenAI base URL to explicit provider config.
  ### Summary
  OpenAI still uses internal fixed endpoint URLs while other providers expose `base_url` in static `config.yml`. Add `providers.openai.base_url` and derive OpenAI Responses and Models endpoints from it so every provider has an explicit configured text URL surface. OpenAI dictation URL configuration is handled separately by I003.
  ### Acceptance Criteria
  1. `configs/config.yml` includes `providers.openai.base_url: "https://api.openai.com/v1"`.
  2. Runtime config loading accepts and requires `providers.openai.base_url`.
  3. OpenAI Responses and Models endpoints are derived from the configured base URL when no test endpoint override is supplied.
  4. README and provider-routing docs document OpenAI `base_url` consistently with other providers.
  5. Black-box tests cover OpenAI base URL routing.
  ### Resolution
  `configs/config.yml` now includes `providers.openai.base_url: "https://api.openai.com/v1"` alongside the required `${OPENAI_API_KEY}` placeholder. Static config loading now requires `providers.openai.base_url`, runtime configuration carries it through `OpenAIBaseURL`, and OpenAI Responses and Models endpoints are derived from that base URL when tests do not inject endpoint overrides. README and `docs/implementation/provider-routing-plan.md` now document OpenAI with the same explicit provider shape as the other upstreams. Black-box CLI config coverage asserts OpenAI base URL loading and required-provider validation, and black-box router coverage verifies OpenAI text requests use the configured base URL. Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 120s -s SIGKILL 120s go test -count=1 ./cmd/cli -run 'TestRootCommand(RunsConfiguredProxyFromConfigFile|RunsProductionLoggerFromConfigFile|RejectsIncompleteStaticProviderConfig)'`, `timeout -k 120s -s SIGKILL 120s go test -count=1 ./internal/proxy -run TestProviderRoutingUsesConfiguredOpenAIURLsForTextAndDictation`, `timeout -k 350s -s SIGKILL 350s make test` (Go total coverage 100.0%, Python 26 passed), `timeout -k 350s -s SIGKILL 350s make lint`, `timeout -k 350s -s SIGKILL 350s make ci`, and `timeout -k 30s -s SIGKILL 30s git diff --check`.
- [x] [I009] (P1) Make static provider configuration explicit and key-complete.
  ### Summary
  The static `config.yml` surface should list concrete provider base URLs instead of relying on blank values plus runtime defaults, and every supported provider key should be required through config placeholders so missing provider credentials fail at startup instead of later per request.
  ### Acceptance Criteria
  1. `configs/config.yml` contains concrete base URLs for every provider with a `base_url` field and concrete SiliconFlow transcription URL.
  2. `configs/config.yml` uses `${...}` placeholders for every supported upstream provider API key.
  3. Runtime config validation rejects missing provider API keys for all supported providers at startup.
  4. README and provider-routing docs describe provider API keys as required and base URLs as explicit config values.
  5. Black-box CLI/config tests cover missing required provider credentials.
  ### Resolution
  `configs/config.yml` now uses `${...}` placeholders for every supported provider API key and explicit URLs for every provider `base_url` plus SiliconFlow `transcriptions_url`. The config-file loader now rejects blank provider API keys, blank provider base URLs, and blank SiliconFlow transcription URLs before building runtime configuration. README and provider-routing docs now describe provider keys as required and provider URLs as explicit config values. Black-box CLI config tests cover complete startup config, missing provider key, missing provider base URL, missing SiliconFlow transcription URL, and omitted tenant defaults. Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 120s -s SIGKILL 120s go test -count=1 ./cmd/cli -run 'TestRootCommand(RunsConfiguredProxyFromConfigFile|RunsProductionLoggerFromConfigFile|RejectsIncompleteStaticProviderConfig)'`, `timeout -k 350s -s SIGKILL 350s make test` (Go total coverage 100.0%, Python 26 passed), `timeout -k 350s -s SIGKILL 350s make lint`, and `timeout -k 350s -s SIGKILL 350s make ci`.
- [ ] [I010] (P0) Limit upstream HTTP call rate in shared HTTP client for text and dictation, without provider‑specific logic.
  Goal:
  Ensure the shared HTTP client layer enforces a consistent, configurable limit on upstream HTTP calls across both text and dictation flows, so that provider rate limits and system resource usage are controlled without duplicating provider-specific throttling logic in multiple places.
  
  Requirements:
  - Apply limiting at the shared HTTP client abstraction used by text and dictation, not in provider-specific integrations.
  - Support configuration of call limits (e.g., max requests per unit time) via existing config mechanisms or a new, clearly documented one.
  - Avoid changing existing provider-specific semantics beyond the addition of call limiting; existing error handling, retries, and timeouts should remain compatible.
  - Ensure limits can be tuned independently per upstream host or service if needed, without hard-coding provider identities.
  - Preserve observability: logging and/or metrics should make it possible to understand when and how often calls are limited.
  - Do not introduce breaking API changes for current callers of the shared HTTP client without a migration path.
  - Handle concurrent requests safely to avoid race conditions or inconsistent enforcement of limits.
  
  Deliverables:
  - Updated shared HTTP client implementation that enforces upstream HTTP call limits for all consumers, including text and dictation paths.
  - Configuration options (and defaults) for call limiting behavior, with inline code docs describing their usage.
  - Logging/metrics hooks that surface when requests are delayed, denied, or otherwise affected by the limiting behavior.
  - Brief developer documentation or comments explaining where the limiting occurs, how to configure it, and how it interacts with existing retry/timeout behavior.
  - Tests covering at least: respecting configured limits, behavior under concurrent load, interaction with retries, and no regression for existing non-limited behavior when limits are effectively disabled.
  
  Validation:
  - Run automated tests demonstrating that the shared HTTP client enforces configured call limits under concurrent traffic for both text and dictation code paths.
  - Confirm that no provider-specific modules contain duplicated or custom call-limiting logic for this purpose after the change, or that any remaining logic is clearly outside this shared concern.
  - Verify through logs/metrics that limiting events are emitted as expected when thresholds are reached.
  - Validate that typical workloads for text and dictation still succeed without unexpected errors when limits are set to realistic production values.
  - Perform a targeted regression check to ensure existing integrations using the shared HTTP client behave as before when limits are configured to be non-restrictive (e.g., effectively off).


## Maintenance

- [x] [M001] (P1) Consolidate repository runbook documents under `.mprlab/`.
  ### Summary
  The repository had duplicate runbook and issue-tracker documents under `issues.md/`, `.mprl/`, and `.mprlab/`. Keep the active tracker and relevant recurring procedures under `.mprlab/`, then remove the old duplicate locations.
  ### Resolution
  Consolidated the current policy, planning, issue-format, and stack-guide documents under `.mprlab/`; kept `.mprlab/ISSUES.md` as the active tracker; carried forward recurring housekeeping runbooks from `issues.md/ISSUES.md`; updated stale runbook path references; and removed the duplicate `issues.md/` and `.mprl/` directories.
- [ ] [M002] (P2) Backlog housekeeping.
  1. Validate `.mprlab/ISSUES.md` matches `.mprlab/issues-md-format.md`.
  2. Confirm user-facing consequences of recently closed work are documented in README, ARCHITECTURE, or PRD.
  3. Prune closed entries once documented.
  4. Merge duplicates and delete irrelevant issues.
- [ ] [M003] (P2) Polish open issues.
  1. For each open issue, add missing context, dependencies, repro steps, acceptance criteria, and test expectations.
  2. Ensure each issue has a clear priority and concrete deliverables.
- [ ] [M004] (P2) Architecture and policy review.
  1. Review the codebase against `.mprlab/POLICY.md` and stack guides.
  2. Record findings as new Maintenance issues, or close as "no action" if already covered.


## Features

- [ ] [F001] (P1) Add authenticated self-service API key and tenant secret management UI.
  ### Summary
  Users need an authenticated browser UI where they sign in through the MPR/TAuth login surface, ask llm-proxy to create a new client key for them, bring their own upstream provider API keys for any supported provider, choose tenant defaults, and then use the service with the generated llm-proxy tenant secret. This should turn llm-proxy from an operator-provisioned static-tenant service into a self-service tenant onboarding surface without changing the public proxy request contract: clients still call `GET /`, JSON `POST /`, `POST /v2`, and `POST /dictate` with `key=<tenant secret>`, while upstream provider API keys stay server-side.
  ### MPR UI and authentication contract
  The UI must follow the `mpr-integration` `mpr-ui` and TAuth contracts:
  1. Serve `/config-ui.yaml` as the only browser-facing MPR UI auth config surface.
  2. Load a pinned `mpr-ui.css`, Google Identity Services, `js-yaml`, pinned `mpr-ui-config.js`, and a bundle marker with `data-mpr-ui-bundle-src`.
  3. Render the shared shell through `<mpr-header data-config-url="/config-ui.yaml">`, `<mpr-user>`, and `<mpr-footer>` rather than direct `tauth.js` loading or manual `tauth-*` attributes.
  4. Treat successful TAuth login as the gate before showing tenant, provider-key, or generated-secret controls.
  5. React to documented `mpr-ui:auth:authenticated`, `mpr-ui:auth:unauthenticated`, `mpr-ui:auth:status-change`, and `mpr-ui:auth:error` events as needed; use `auth-transition` only when the authenticated settings surface has a visible loading gap.
  6. Protect all key-management APIs with TAuth session-cookie validation. Unauthenticated requests return `401`, not a rendered management page or partial tenant data.
  7. Use exact profile values for frontend origin, backend/API origin, cookie names, issuer, tenant id, OAuth callback, TLS/cookie policy, CORS credentials, DNS, reverse proxy, and service port. Do not infer hosted values from localhost.
  ### Product contract
  1. An authenticated user sees a primary "Create a new key" flow that creates or selects their llm-proxy tenant and generates a client tenant secret.
  2. Tenant ownership is bound to the authenticated TAuth subject; users can manage only their own tenant secrets, provider keys, defaults, and usage examples.
  3. The user can add, update, or remove their own API keys for every provider supported by llm-proxy, then configure text/dictation defaults from those available providers.
  4. Provider API keys are accepted only through the authenticated management UI/API, stored server-side, masked after save, never returned raw, and never logged.
  5. Public proxy requests never accept upstream provider API keys in query parameters or JSON/multipart bodies.
  6. The tenant secret is generated by the server from a cryptographically strong source, is unique, is shown only once after generation, and can be revoked or regenerated from the authenticated UI.
  7. Generated secrets authenticate existing proxy endpoints through the current `key` query parameter and select that tenant's provider credentials and defaults.
  8. Missing, invalid, or revoked tenant secrets continue to return `403` on proxy endpoints without exposing whether a tenant exists.
  9. Tenant/provider configuration has one authoritative runtime store and loader. Do not preserve legacy static-only fallback reads or old single-secret schema interpretation for this feature unless a separate compatibility issue explicitly asks for it.
  ### Acceptance Criteria
  1. Add the MPR-authenticated browser shell and management pages using the declarative `/config-ui.yaml` + `mpr-ui-config.js` path; key-management controls are visible only after TAuth login.
  2. Add backend management endpoints for authenticated tenant profile, provider-key save/update/removal, default provider/model settings, tenant secret generation, revoke, and regenerate.
  3. Add a "create new key" flow that creates the tenant-owned client secret and presents copyable usage examples for text, `/v2`, and dictation calls.
  4. Add a validated domain model for tenant-owned provider credentials and generated client secrets with edge-only validation and no zero-but-invalid exported state.
  5. Store provider API keys and generated tenant secrets securely; raw provider keys and raw historical secrets are not readable through the UI/API after save.
  6. Route proxy text and dictation requests using the generated tenant secret and tenant-owned provider credentials/defaults.
  7. Document the self-service setup flow, generated-secret usage examples, provider-key security model, and hosted profile values required for production deployment.
  8. Add black-box HTTP tests for unauthenticated management `401`, authenticated key save/masking, cross-user tenant isolation, secret generation, generated-secret proxy success, revoked-secret `403`, and rejection of client-supplied provider keys on public proxy endpoints.
  9. Add Playwright browser coverage for the MPR shell rendering, login/session recovery, "create new key" behavior, provider-key form behavior, one-time secret display, revoke/regenerate flow, and copied usage example using the generated secret.


## Planning
*do not implement yet*

