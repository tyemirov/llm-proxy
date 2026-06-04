# ISSUES

Working backlog for this repository. Keep it current and small. Use @issues-md-format.md for the canonical format.

- Status markers: `[ ]` open, `[!]` blocked (must include a `Blocked:` line), `[x]` closed.
- Hygiene: once a closed issue's consequences are reflected in code/tests and in user-facing docs, remove the entry from this file. Git history remains the record. (Recurring runbooks below are the exception: keep them open.)

## BugFixes

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
  Resolution: Added a black-box integration test for a 31 KB semantic-review JSON POST using `gpt-5.5-pro`; the pre-fix failure reproduced `502 OpenAI API error` with `max_output_tokens=1024` and an incomplete/max-output upstream path. The proxy now defaults text `max_output_tokens` to `8192`, README documents the `LLM_PROXY_MAX_OUTPUT_TOKENS` knob, low-budget continuation coverage remains explicit, and `make ci` passes with total coverage at 100.0%.

- [x] [B404] (P0) Fix GPT-5.5 JSON body model requests returning 502.
  Reproduce and repair the `POST /?key=...` JSON request path where clients specify `"model": "gpt-5.5"` in the body and expect a successful OpenAI Responses API reply instead of a proxy-level `502 OpenAI API error`.
  Acceptance criteria:
  1. A JSON body request with `prompt`, `model: "gpt-5.5"`, and `web_search: false` reaches the upstream Responses API with the requested model.
  2. The proxy returns a normal text response when upstream returns a completed GPT-5.5 response.
  3. The failure mode is documented if a live upstream credential or model access check blocks verification.
  Resolution: The 502 came from GPT-5.5 Responses returning `status: "incomplete"` after spending the output budget on reasoning/web-search work; the proxy then called the unsupported `/v1/responses/{id}/continue` endpoint. Incomplete max-token responses now continue through a new Responses request with `previous_response_id`, preserving the body model and web-search settings. A patched live proxy run with `model: "gpt-5.5"` in the JSON body returned `200 OK`, and `make ci` passes with total coverage at 100.0%.

## Improvements

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
