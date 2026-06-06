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

- [x] [I415] (P1) Add an importable Python llm-proxy client package.
  The American-language and Russian-language skill surfaces are Python-based callers of the llm-proxy JSON POST contract. Add a first-class Python package in this repository so those surfaces can share transport behavior without each owning a local proxy helper.
  Acceptance criteria:
  1. `python/llm_proxy_client` exposes validated config/request dataclasses and a client that sends JSON `POST /?key=...&format=text/plain`.
  2. The client preserves non-body query parameters such as `provider`, strips body-owned query parameters such as `prompt`, `model`, `web_search`, `system_prompt`, and `max_tokens`, and keeps model IDs unchanged.
  3. HTTP errors and transport errors are surfaced as typed Python exceptions without logging or printing secrets.
  4. Pytest coverage uses a local HTTP server for the public client contract plus validation/error scenarios.
  5. The repository Makefile runs Python mypy and pytest as part of `make lint`, `make test`, and `make ci`.
  Resolution: Added `python/llm_proxy_client` with validated `ClientConfig` and `ClientRequest` dataclasses, typed HTTP/transport exceptions, and a transport-only `Client.post` that preserves non-body query fields, strips body-owned fields, and sends the JSON POST contract without model alias rewriting. Added `python/pyproject.toml`, `python/uv.lock`, local-server pytest coverage, README import docs, and Makefile Python lint/test integration. Post-review timeout coverage now proves stalled urllib reads raise `LLMProxyTransportError` instead of leaking raw `TimeoutError`. Root-level packaging and a Makefile import smoke check now prove `llm_proxy_client` installs from the repository root. Validation passed with `make python-lint`, `make python-test`, editable install smoke via `uv pip install -e .`, exact local `go install github.com/tyemirov/llm-proxy/llm-proxy-client`, and final `timeout -k 1200s -s SIGKILL 1200s make ci` with Go total coverage 100.0%, Python pytest 12 passed, and the root import smoke passing.

- [x] [I414] (P1) Add an installable llm-proxy client command and reusable client library.
  Port the JSON POST client contract used by the Russian-language semantic QA workflow into reusable Go client code under `llm-proxy` that compiles through `go install github.com/tyemirov/llm-proxy/llm-proxy-client`.
  Acceptance criteria:
  1. The reusable client preserves the proxy contract: `POST /?key=...` with prompt/model/web_search in JSON body and non-payload query parameters such as `provider` preserved.
  2. The command reads prompts from `--prompt`, `--prompt-file`, or stdin, authenticates with `--secret`/`LLM_PROXY_SECRET`, and accepts `--base-url`, `--model`, `--provider`, `--web-search`, `--system-prompt`, `--max-tokens`, and `--timeout`.
  3. The command is installable at `github.com/tyemirov/llm-proxy/llm-proxy-client`.
  4. Black-box CLI tests cover successful JSON POST behavior and edge validation failures.
  Resolution: Added the reusable Go client package, root-level installable `llm-proxy-client` command, README usage docs, and black-box CLI tests for JSON POST request shaping, env/stdin/file prompt paths, validation failures, and proxy/IO errors. Corrected the module/import path to `github.com/tyemirov/llm-proxy` so the requested install path compiles. The Russian-language QA script keeps its local urllib transport helper; no reusable Python client was added. Post-review fixes removed client-side shorthand model rewriting so model IDs pass through unchanged, and added `llm-proxy-client` plus `pkg/llmproxyclient` to the 100% coverage gate. Validation passed with `timeout -k 120s -s SIGKILL 120s go test ./llm-proxy-client ./pkg/llmproxyclient`, exact local `go install github.com/tyemirov/llm-proxy/llm-proxy-client`, `timeout -k 120s -s SIGKILL 120s /Users/tyemirov/Development/Smith/russian-language/tests/test_semantic_stress_qa.py -k llm_proxy`, full `timeout -k 350s -s SIGKILL 350s /Users/tyemirov/Development/Smith/russian-language/tests/test_semantic_stress_qa.py`, `timeout -k 350s -s SIGKILL 350s make lint`, original `timeout -k 350s -s SIGKILL 350s make ci` with total coverage 100.0%, post-review `timeout -k 350s -s SIGKILL 350s make test` plus `timeout -k 350s -s SIGKILL 350s make lint`, and final post-review `timeout -k 350s -s SIGKILL 350s make ci` with total coverage 100.0%.

- [x] [I413] (P0) Add a live Gemini smoke gate for the current binary.
  The Gemini request-shape regression passed fake-upstream tests because the fake server accepted response-only fields that live Gemini rejects. The repository needs an explicit opt-in live provider test that builds the current binary, runs it locally, sends a JSON POST through the public proxy route with the model in the body, and verifies live Gemini returns `OK` with HTTP `200`.
  Resolution: Added `scripts/test_live_gemini.sh` and `make test-live-gemini`. The target requires `GEMINI_API_KEY` and `SERVICE_SECRET`, supports `LIVE_ENV_FILE=/path/to/env`, starts a temporary local proxy with `LLM_PROXY_DEFAULT_PROVIDER=gemini`, supplies an unused placeholder OpenAI key for startup-only dictation validation, and asserts a real `POST /?provider=gemini` call using `model=gemini-3.5-flash` succeeds against live Gemini. README documents the target. Validation passed with only the documented required live credentials and with `LIVE_ENV_FILE=/Users/tyemirov/Development/mprlab-gateway/configs/.env.llm-proxy timeout -k 120s -s SIGKILL 120s make test-live-gemini`.

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

- [x] [M417] (P1) Refactor slow release-gate tests instead of extending the CI timeout.
  `make release` still timed out after the release lifecycle timeout was extended to 1200 seconds because the Go integration suite can spend long periods in fixed sleeps and retry waits before the coverage summary and Python tests run. The release gate should be made deterministic by refactoring the slow test fixtures, not by extending the default release timeout.
  Acceptance criteria:
  1. Release, publish, and deploy local `make ci` wrappers default back to the original 350-second gate while retaining explicit operator overrides.
  2. Long-running integration tests stop sleeping for fixed multi-second or 30-second intervals when a channel-controlled upstream fixture can prove the same black-box route behavior.
  3. The integration package runtime drops materially while preserving the 100% Go coverage gate.
  4. Focused Go integration coverage and full `make ci` pass locally.
  Resolution: Restored the release, publish, and deploy CI wrappers to the standard 350-second default while keeping shared and command-specific timeout overrides for exceptional diagnostics. Refactored slow Go fixtures to use channel-controlled upstream release/cancellation, one-slot queue saturation, short request contexts for retry-mapping checks, and injected deadline errors for dictation timeout coverage. Request cancellation and deadline errors now stop retry loops immediately. The post-test coverage binary probes also redirect stdin from `/dev/null` and fail explicitly on timeout so interactive `make ci` cannot block inside `llm-proxy-client.cover`. Validation passed with `timeout -k 120s -s SIGKILL 120s go test -count=1 ./internal/utils ./internal/proxy ./tests/integration`, `timeout -k 120s -s SIGKILL 120s make go-test` from a TTY (total coverage 100.0%, real 0m8.946s), and `timeout -k 350s -s SIGKILL 350s make ci` (real 0m13.283s).

- [x] [M416] (P1) Keep release lifecycle CI wrappers aligned with the expanded Makefile gate.
  `make release` still wrapped `make ci` in a hard-coded 350-second timeout after the Makefile gate expanded to include Python mypy and pytest. Release validation can finish the Go coverage gate and then be killed by the wrapper before the remaining `make ci` steps complete. `make publish` and `make deploy` use the same local CI wrapper and should share the same operator timeout contract.
  Acceptance criteria:
  1. `make release`, `make publish`, and `make deploy` keep running local `make ci` by default.
  2. The local CI timeout is long enough for the combined Go and Python gate and is configurable without editing scripts.
  3. README documents the timeout override and the full `make ci` contents.
  4. Shell syntax and the full CI gate pass locally.
  Resolution: `scripts/release.sh`, `scripts/publish.sh`, and `scripts/deploy.sh` now run their local `make ci` gates with a 1200-second default timeout, support `LLM_PROXY_CI_TIMEOUT_SECONDS` plus command-specific overrides, validate positive integer timeout values, and print the active timeout in operator output. Generated Python bytecode and egg-info metadata are no longer tracked and are ignored so CI does not dirty release worktrees. README documents the expanded `make ci` contents and timeout override. Validation passed with `bash -n scripts/release.sh scripts/publish.sh scripts/deploy.sh`, `git diff --check`, and `timeout -k 1200s -s SIGKILL 1200s make ci`.

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

- [x] [F418] (P1) Add tenant-authenticated defaults.
  Replace the single shared `server.service_secret` plus global `defaults` contract with an explicit tenant list where each tenant has its own client secret and default text/dictation settings. Requests continue to authenticate with the `key` query parameter, but the accepted key selects tenant defaults when provider/model/system prompt values are omitted.
  Acceptance criteria:
  1. `config.yml` defines `tenants[]` with unique non-empty `id` and `secret` values plus tenant-local `defaults.provider`, `defaults.model`, `defaults.dictation_provider`, `defaults.dictation_model`, and `defaults.system_prompt`.
  2. `server.service_secret` and top-level `defaults` are removed from the canonical service schema; unknown legacy keys fail startup through the strict config loader.
  3. Runtime configuration exposes a validated tenant registry instead of raw scalar auth/default fields.
  4. `GET /`, JSON `POST /`, and `POST /dictate` use the authenticated tenant defaults when request parameters omit provider, model, dictation provider/model, or system prompt.
  5. Explicit request provider/model/system prompt parameters keep their existing override semantics.
  6. Missing or invalid `key` still returns `403` without logging raw secrets.
  7. README, provider-routing docs, and mounted gateway config examples document the tenant schema.
  8. Black-box CLI and HTTP tests cover tenant config loading, duplicate tenant validation, token-selected defaults, request overrides, dictation defaults, and invalid keys.
  Resolution: Replaced the canonical single-secret/global-default schema with `tenants[]`, where each tenant owns a unique `id`, unique `secret`, and text/dictation defaults. Runtime startup now validates tenant identity, secret uniqueness, and each tenant's default provider/model contract. Auth middleware resolves `key` to a tenant and routes GET, JSON POST, and `/dictate` omitted provider/model/system prompt values through that tenant; explicit request parameters keep their override behavior. README, provider-routing docs, live Gemini smoke config, coverage probe config, and the mounted gateway config were updated to the tenant schema. Added black-box coverage for config loading, duplicate/missing tenant validation, token-selected defaults, overrides, dictation defaults, and invalid keys. Validation passed with `make fmt`, `make test` (Go total coverage 100.0%, Python pytest 12 passed), `make lint`, and `make ci` (Go total coverage 100.0%, Python pytest 12 passed).

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

- [x] [P411] (P1) {F410} Make `config.yml` the sole service configuration source.
  Plan and implement a configuration refactor where the running service receives all service configuration from `config.yml`. Environment variables and `.env` files may only supply interpolation values for placeholders in that YAML file; the rest of the program must never read environment variables as configuration.
  Acceptance criteria:
  1. `config.yml` has a documented strict schema for server, defaults, provider credentials/base URLs, timeouts, prompt/body/audio limits, and logging.
  2. The CLI stops binding service configuration flags and environment variables; only help and config-file path selection remain outside `config.yml`.
  3. The config loader expands `${NAME}` placeholders from a single expansion map built at the config-loading boundary, fails on missing placeholders, and does not mutate process environment.
  4. Runtime code receives a validated `proxy.Configuration` value produced by a smart constructor; router, providers, middleware, and clients never call Viper, gotenv, or OS env APIs.
  5. Startup, README, provider-routing docs, Docker guidance, and coverage helper flows describe `config.yml` as authoritative and `.env` as interpolation input only.
  6. Black-box CLI and HTTP tests cover config-file loading, `.env` expansion, missing placeholders, unknown YAML keys, removed env/flag config surfaces, and provider credential validation.
  Resolution: Added strict `config.yml` loading at the CLI edge with `${NAME}` expansion from process env plus same-directory `.env` values, removed service configuration flags/env bindings and direct `.env` mutation, and routed runtime setup through `proxy.NewConfiguration`. Updated README, provider-routing docs, coverage startup probes, and live Gemini smoke setup to treat config files as authoritative. Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 350s -s SIGKILL 350s make test` (total coverage 100.0%), `timeout -k 350s -s SIGKILL 350s make lint`, and `timeout -k 350s -s SIGKILL 350s make ci` (total coverage 100.0%).
