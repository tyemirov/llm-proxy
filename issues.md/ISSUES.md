# ISSUES

Working backlog for this repository. Keep it current and small. Use @issues-md-format.md for the canonical format.

- Status markers: `[ ]` open, `[!]` blocked (must include a `Blocked:` line), `[x]` closed.
- Hygiene: once a closed issue's consequences are reflected in code/tests and in user-facing docs, remove the entry from this file. Git history remains the record. (Recurring runbooks below are the exception: keep them open.)

## BugFixes

- [x] [B412] (P0) Stop sending Gemini response-only `thought` fields in generateContent requests.
  Live Gemini calls proved that `gemini-3.5-flash` accepts the proxy's model and prompt, but rejects request parts containing `thought:false` with `400 INVALID_ARGUMENT`. The proxy introduced `parts[].thought` to filter Gemini response thoughts, then reused that struct for outbound request serialization, causing selected Gemini requests to fail as proxy `502` with `provider API error: status=400`.
  Resolution: Split Gemini request and response part types so outbound `generateContent` payloads serialize only request-valid `text` fields, while response parsing still reads `parts[].thought` and filters thought text. Added black-box provider-routing assertions that Gemini GET and JSON POST request payloads omit `thought` from user contents and system instructions. Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 350s -s SIGKILL 350s make test` (total coverage 100.0%), `timeout -k 350s -s SIGKILL 350s make lint`, `timeout -k 350s -s SIGKILL 350s make ci` (total coverage 100.0%), and a patched local proxy smoke test returning `OK`/`200` from live Gemini for `model=gemini-3.5-flash`.

- [x] [B407] (P0) Cancel upstream text generation when downstream requests time out.
  Handler timeout and client/gateway disconnect contexts must flow through queued text work, provider routing, OpenAI Responses requests, OpenAI-compatible chat requests, continuation creation, and polling. After the proxy sends a timeout response, upstream work must not keep running long enough to produce a usable late OpenAI response.
  Acceptance criteria:
  1. `requestTask` carries a request context derived from the HTTP request and app timeout.
  2. OpenAI Responses, OpenAI-compatible chat, continuation, and polling HTTP requests derive from that request context instead of `context.Background()`.
  3. An integration test proves a gateway-style timed-out request causes the upstream request context to be canceled.
  4. The integration test proves no late usable OpenAI API response is accepted after the proxy has sent `504 Gateway Timeout`.
  Resolution: Threaded the handler-derived request context through queued text tasks, provider routing, OpenAI Responses, OpenAI-compatible chat, continuation creation, and response polling. Added integration coverage that fails on a late usable OpenAI response after a proxy `504`, plus poll-path coverage for fetch-timeout, poll-sleep timeout, and poll-sleep cancellation behavior. Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 350s -s SIGKILL 350s make test` (total coverage 100.0%), and `timeout -k 350s -s SIGKILL 350s make lint`.

- [x] [B406] (P1) Document request timeout knobs for gateway alignment.
  Public gateway routes need their upstream transport timeout to stay aligned with llm-proxy's app-side request timeout; otherwise long-running LLM calls can be cut off by the gateway before the proxy returns its own response.
  Resolution: Documented `LLM_PROXY_REQUEST_TIMEOUT_SECONDS` and `LLM_PROXY_UPSTREAM_POLL_TIMEOUT_SECONDS` in the README so operators can keep gateway transport settings aligned with the proxy's request and poll windows. Validation passed with `git diff --check`.

- [x] [B405] (P0) Fix large semantic review POSTs for heavy GPT-5.5-family models.
  Full semantic stress review requests can be around 31 KB and need a response budget large enough to return a complete reviewed transcript. The public `POST /?key=...` JSON body path must support those review jobs without forcing callers into chunked review, because chunked model responses can mutate source text or drop required stress coverage.
  Acceptance criteria:
  1. Add an end-to-end integration test that sends a large semantic-review JSON body through the public HTTP route with a heavier GPT-5.5-family model.
  2. The test must fail before the implementation fix by reproducing the low-output-budget/incomplete-response path as a client-visible `502`.
  3. The proxy must forward a sufficiently large output budget for the large full-review request and return a normal text response from the upstream stub.
  4. The fix must not weaken downstream semantic validation expectations; chunked review remains a retry transport strategy, not an acceptance path.
  Resolution: Added a black-box integration test for a 31 KB semantic-review JSON POST using `gpt-5.5-pro`; the pre-fix failure reproduced `502 OpenAI API error` with a low `max_output_tokens` value and an incomplete/max-output upstream path. Large full-review requests are covered through the public JSON POST contract, low-budget continuation coverage remains explicit, and `make ci` passes with total coverage at 100.0%.

- [x] [B404] (P0) Fix GPT-5.5 JSON body model requests returning 502.
  Reproduce and repair the `POST /?key=...` JSON request path where clients specify `"model": "gpt-5.5"` in the body and expect a successful OpenAI Responses API reply instead of a proxy-level `502 OpenAI API error`.
  Acceptance criteria:
  1. A JSON body request with `prompt`, `model: "gpt-5.5"`, and `web_search: false` reaches the upstream Responses API with the requested model.
  2. The proxy returns a normal text response when upstream returns a completed GPT-5.5 response.
  3. The failure mode is documented if a live upstream credential or model access check blocks verification.
  Resolution: The 502 came from GPT-5.5 Responses returning `status: "incomplete"` after spending the output budget on reasoning/web-search work; the proxy then called the unsupported `/v1/responses/{id}/continue` endpoint. Incomplete max-token responses now continue through a new Responses request with `previous_response_id`, preserving the body model and web-search settings. A patched live proxy run with `model: "gpt-5.5"` in the JSON body returned `200 OK`, and `make ci` passes with total coverage at 100.0%.

## Improvements

- [x] [I413] (P0) Add a live Gemini smoke gate for the current binary.
  The Gemini request-shape regression passed fake-upstream tests because the fake server accepted response-only fields that live Gemini rejects. The repository needs an explicit opt-in live provider test that builds the current binary, runs it locally, sends a JSON POST through the public proxy route with the model in the body, and verifies live Gemini returns `OK` with HTTP `200`.
  Resolution: Added `scripts/test_live_gemini.sh` and `make test-live-gemini`. The target requires `GEMINI_API_KEY` and `SERVICE_SECRET`, supports `LIVE_ENV_FILE=/path/to/env`, starts a temporary local proxy with `LLM_PROXY_DEFAULT_PROVIDER=gemini`, and asserts a real `POST /?provider=gemini` call using `model=gemini-3.5-flash` succeeds against live Gemini. README documents the target. Validation passed with `LIVE_ENV_FILE=/Users/tyemirov/Development/mprlab-gateway/configs/.env.llm-proxy timeout -k 120s -s SIGKILL 120s make test-live-gemini`.

- [x] [I408] (P1) Surface token usage metadata in LLM responses.
  Upstream text providers may return token usage counts, but the proxy currently returns only generated text to response formatters. Normalize provider usage into request, response, and total token counts, surface it through non-breaking response headers, and include it in JSON-format LLM responses when upstream usage is present.
  Acceptance criteria:
  1. OpenAI Responses API `usage.input_tokens`, `usage.output_tokens`, and `usage.total_tokens` are normalized.
  2. OpenAI-compatible chat `usage.prompt_tokens`, `usage.completion_tokens`, and `usage.total_tokens` are normalized.
  3. Plain text/XML/CSV response bodies remain unchanged.
  4. Usage metadata is visible in response headers when upstream usage is available.
  5. JSON-format LLM responses include a `usage` object when upstream usage is available.
  Resolution: Added normalized token usage propagation for OpenAI Responses and OpenAI-compatible chat completions, surfaced counts through `X-LLM-Proxy-Request-Tokens`, `X-LLM-Proxy-Response-Tokens`, and `X-LLM-Proxy-Total-Tokens`, and included `usage.request_tokens`, `usage.response_tokens`, and `usage.total_tokens` in JSON-format LLM responses when upstream usage is present. Empty or mismatched upstream `usage` objects are treated as absent metadata. Plain text, XML, and CSV bodies remain unchanged. Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 350s -s SIGKILL 350s make test` (total coverage 100.0%), and `timeout -k 350s -s SIGKILL 350s make lint`.

## Maintenance

- [x] [M403] (P1) Enforce 100% Go test coverage
  Require repository tests to prove 100% coverage without production code knowing about tests. Cover gaps through black-box HTTP/CLI entry points and use dependency injection only around hard-to-control external effects.
  Resolution: `make test` now enforces the merged package plus CLI binary coverage gate, and `make ci` passes with total coverage at 100.0%.

### Recurring (runbooks; keep open)

These entries are always-available procedures. Keep them `[ ]` so they remain runnable; when you run one, update a short `Last run:` line in the body (and optionally link the PR/commit).

- [ ] [M400] (P2) Backlog housekeeping
  1. Validate `issues.md/ISSUES.md` matches `issues-md-format.md`.
  2. Confirm user-facing consequences of recently closed work are documented (README/ARCHITECTURE/PRD).
  3. Prune closed entries once documented.
  4. Merge duplicates and delete irrelevant issues.

- [ ] [M401] (P2) Polish open issues
  1. For each open issue, add missing context (dependencies, repro steps, acceptance criteria, and test expectations).
  2. Ensure each issue has a clear priority and concrete deliverables.

- [ ] [M402] (P2) Architecture and policy review
  1. Review the codebase against POLICY.md and stack guides.
  2. Record findings as new Maintenance issues (or close as "no action" if already covered).

## Features

- [x] [F410] (P1) Replace global output-token configuration with request `max_tokens`.
  Remove the server-wide text output token cap configuration because providers do not require it and a single global budget has different semantics across provider APIs. Clients may request an explicit cap per call with `max_tokens`, and the proxy translates that public request parameter to each provider's native field.
  Acceptance criteria:
  1. `LLM_PROXY_MAX_OUTPUT_TOKENS`, `--max_output_tokens`, `Configuration.MaxOutputTokens`, and the default output-token cap are removed from runtime configuration.
  2. When no client `max_tokens` is supplied, text provider requests omit OpenAI `max_output_tokens`, OpenAI-compatible `max_tokens`, and Gemini `generationConfig.maxOutputTokens`.
  3. `GET /?...&max_tokens=N` and JSON `POST /?key=...` bodies with `"max_tokens": N` validate positive integer values and forward them as OpenAI `max_output_tokens`, OpenAI-compatible `max_tokens`, and Gemini `generationConfig.maxOutputTokens`.
  4. Invalid, zero, or negative `max_tokens` values are rejected at the API edge with `400`.
  5. README and provider-routing docs describe the request-level `max_tokens` contract and no longer document the removed environment variable.
  6. Black-box HTTP and CLI tests cover the removed config surface and provider-specific request translations, and repository validation passes.
  Resolution: Removed the server-wide max output token configuration surface from CLI flags, env bindings, runtime configuration, provider clients, and defaults. Added request-level `max_tokens` for GET query parameters and JSON POST bodies; absent values now omit provider max-token fields, while positive values map to OpenAI Responses `max_output_tokens`, OpenAI-compatible chat `max_tokens`, and Gemini `generationConfig.maxOutputTokens`. Invalid query/body values return `400` before upstream calls. README and provider-routing docs were updated, the large semantic-review integration test now uses request `max_tokens`, and validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 350s -s SIGKILL 350s make test` (total coverage 100.0%), `timeout -k 350s -s SIGKILL 350s make lint`, and `timeout -k 350s -s SIGKILL 350s make ci` (total coverage 100.0%).

- [x] [F409] (P1) Add Gemini as a native text provider.
  Add Google Gemini support to the authenticated LLM endpoint using Gemini's native `generateContent` API. The provider should be selected with `provider=gemini`, should use server-side `GEMINI_API_KEY`, should default to `gemini-3.5-flash`, and should support only text generation initially. `/dictate` and `web_search` must return existing unsupported endpoint/capability errors for Gemini until those capabilities are explicitly designed.
  Acceptance criteria:
  1. `GET /` and JSON `POST /` route Gemini requests to the native Gemini API with the configured Gemini API key.
  2. Known Gemini models validate through the provider registry, unknown Gemini models return `400`, and omitted Gemini model values use `gemini-3.5-flash`.
  3. Missing Gemini credentials return `503` for selected Gemini requests and fail startup when Gemini is configured as the default provider.
  4. Gemini dictation and Gemini `web_search` requests return the existing unsupported endpoint/capability errors.
  5. README and implementation docs describe Gemini configuration, usage, supported models, and capability limits.
  6. Black-box HTTP and CLI tests cover success and failure paths, and repository validation passes.
  Resolution: Added native Gemini text routing through `provider=gemini` using Google's generateContent API, `GEMINI_API_KEY`, optional `GEMINI_BASE_URL`, and `gemini-3.5-flash` as the provider default model. Supported Gemini text models are `gemini-3.5-flash`, `gemini-3.1-flash-lite`, `gemini-2.5-flash`, `gemini-2.5-flash-lite`, and `gemini-2.5-pro`. Gemini token usage metadata is normalized into the existing usage headers and JSON response shape when upstream returns it. Gemini `/dictate`, `web_search`, unknown model, missing credential, upstream status, transport, malformed response, no-text response, and invalid usage cases are covered by black-box tests. README and provider-routing docs were updated. Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 350s -s SIGKILL 350s make test` (total coverage 100.0%), `timeout -k 350s -s SIGKILL 350s make lint`, `timeout -k 350s -s SIGKILL 350s make ci`, and `git diff --check`.

## Planning

- [ ] [P411] (P1) {F410} Make `config.yml` the sole service configuration source.
  Plan and implement a configuration refactor where the running service receives all service configuration from `config.yml`. Environment variables and `.env` files may only supply interpolation values for placeholders in that YAML file; the rest of the program must never read environment variables as configuration.
  Acceptance criteria:
  1. `config.yml` has a documented strict schema for server, defaults, provider credentials/base URLs, timeouts, prompt/body/audio limits, and logging.
  2. The CLI stops binding service configuration flags and environment variables; only help and config-file path selection remain outside `config.yml`.
  3. The config loader expands `${NAME}` placeholders from a single expansion map built at the config-loading boundary, fails on missing placeholders, and does not mutate process environment.
  4. Runtime code receives a validated `proxy.Configuration` value produced by a smart constructor; router, providers, middleware, and clients never call Viper, gotenv, or OS env APIs.
  5. Startup, README, provider-routing docs, Docker guidance, and coverage helper flows describe `config.yml` as authoritative and `.env` as interpolation input only.
  6. Black-box CLI and HTTP tests cover config-file loading, `.env` expansion, missing placeholders, unknown YAML keys, removed env/flag config surfaces, and provider credential validation.
