# LLM Proxy

LLM Proxy is a lightweight HTTP service that forwards user prompts to OpenAI's
Responses API, OpenAI-compatible chat providers, Anthropic's native Messages
API, Google Gemini's native Interactions API, and audio transcription APIs.
It exposes protected HTTP endpoints that require a tenant secret and simplify
integrating provider capabilities without embedding API credentials in each
client. Canonical `POST /v2` user messages can also carry provider-neutral,
ordered image and audio attachments when the exact configured model declares
that input capability.

The public [LLM Proxy landing page](https://llm-proxy.mprlab.com/) explains the
current provider, model, dictation, web-search, request-limit, and integration
surface. Its route explorer filters model families by weight access and
provider-offering capabilities. It then selects a model family, an exact model,
and a provider offering. Its model matrix has one row for each exact model and
shows all current provider offerings for that model. Both interfaces use the
validated runtime catalog. A catalog change appears on the landing page without
a second inventory. The authenticated
management app opens at [`/app/`](https://llm-proxy.mprlab.com/app/) only after
the public **Log In** action authenticates the user through MPR UI and TAuth.

## Features

- Minimal HTTP server whose complete owned operation surface is defined by the
  [canonical OpenAPI contract](docs/openapi.yaml)
- Choose the provider per request via `provider=...`; omitted provider uses the authenticated tenant default
- Choose the model per request via `model=...`; omitted model uses the tenant default when `provider` is omitted, otherwise the selected provider's configured default
- Set optional nonblank `reasoning_effort=...` on `GET /`, or in a JSON body for `POST /` and `POST /v2`, to select a capability-supported reasoning level for that exact resolved route. An explicit value overrides the tenant default; an omitted value retains it. Blank or unsupported values fail before an upstream call.
- Choose the dictation model per request via `model=...` on `/dictate`; omitted model uses the tenant default when `provider` is omitted, otherwise the selected provider's configured default
- Optional per-request web search via exact `web_search=true`; `false` or omission keeps it disabled
- Optional exact inline or tenant-asset-backed image and audio attachments on canonical `POST /v2` user messages
- Provider-enforced JSON Schema output and durable request reconciliation for supported canonical `POST /v2` routes
- Optional logging at `debug` or `info` levels
- Forwards requests with tenant-managed provider API keys loaded from the management database
- TAuth-protected self-service UI where signed-in users automatically receive an llm-proxy client key and their provider settings plus routing defaults autosave
- Supports plain text, JSON, XML, or CSV responses

## REST Contract

llm-proxy exposes a blocking REST contract for text generation. Canonical
`POST /v2` also accepts optional media evidence and structured requests. An
ordinary request returns its final formatted answer in the same HTTP response.
The structured request contract also provides durable reconciliation.

### Canonical OpenAPI ownership

[`docs/openapi.yaml`](docs/openapi.yaml) is the sole hand-maintained HTTP wire
contract for every llm-proxy-owned proxy, browser-configuration, and management
operation. It defines exact paths, methods, authentication, parameters, bodies,
multipart parts, response headers, media types, schemas, and intentional status
codes. TAuth-owned operations are deliberately excluded.

Every server or bundled-client wire change must update that artifact in the
same change. CI compares the real router inventory bidirectionally with it and
validates representative real-handler, Go package, Python package, and Go CLI
exchanges against it. Prose and command examples in this README are explanatory,
not a second contract.

The Pages release copies those committed bytes to
[`/openapi.yaml`](https://llm-proxy.mprlab.com/openapi.yaml) and publishes a
human-readable reference derived from them at
[`/docs/`](https://llm-proxy.mprlab.com/docs/). The contract names
`https://llm-proxy-api.mprlab.com` as its API server; the API origin does not
serve another schema location. The public OpenAPI navigation opens the schema
actions on `/docs/`: visitors can view the complete YAML in the generated
reference or download the same canonical bytes as `llm-proxy-openapi.yaml`.

### `web_search` boolean migration

Direct `GET /` callers must send the optional query parameter as exact
`web_search=true` or `web_search=false`. Omission means false. Former spellings
such as `1`, `0`, `t`, `f`, `y`, `n`, `yes`, and `no`, along with blank,
uppercase, or whitespace-padded values, now return HTTP 400 instead of being
coerced or silently treated as false.

JSON callers continue to send a native boolean body field on `POST /` or
`POST /v2`. The official Go package exposes `MessagesRequestInput.WebSearch`,
the Go CLI exposes the boolean `--web-search` flag, and the Python package
exposes `ClientMessagesRequest.web_search: bool`. All bundled clients use
`POST /v2`, remove any `web_search` query value inherited from a configured
base URL, and serialize the request body as `"web_search": true` or
`"web_search": false`.

An ordinary request does not stream tokens, poll a job endpoint, or use a
resume token.

OpenAI Responses and Gemini 3-series Interactions use pollable upstream
lifecycles. The proxy sends stored background requests and holds the client
connection. It polls a nonblank resource id through `status=queued` and
`status=in_progress`. The shared `pollable_resource` lifecycle makes the first
resource read immediately. The selected transport declares the retry statuses,
interval, and limit for a resource that is not visible yet. OpenAI permits one
retry after two seconds for `403` or `404`. Gemini permits six retries after
five-second intervals for `400`, `403`, or `404`. The lifecycle releases the
upstream worker during each wait. The request context bounds each wait.
Provider resource ids remain in the active request lifecycle. The proxy does
not keep a durable provider job or expose a provider resource id.

Every current text route uses one provider-neutral completion coordinator.
The coordinator starts a new request only when the selected transport declares
continuation actions. OpenAI Responses `status=incomplete` with
`reason=max_output_tokens`, Chat Completions `finish_reason=length`, and
Anthropic `stop_reason=max_tokens` use those actions. The coordinator retains
the original messages and appends the visible output and one missing-suffix
instruction. Gemini declares no continuation actions. A Gemini
`status=incomplete` result causes a provider error after interaction cleanup.
Safety filters, refusals, tool/intermediate states, malformed responses, and
missing or unknown signals remain `502` failures and never trigger this loop.

A `504 Gateway Timeout` means the proxy request budget expired before a final
answer. For a structured request, reconcile the idempotency key before a new
submission.

### Structured JSON output and reconciliation

Canonical `POST /v2` accepts an optional `structured_output.schema` object.
The object contains the caller-owned JSON Schema. A structured request requires
exactly one valid `Idempotency-Key` header. The proxy rejects either input when
the other input is absent.

The proxy compiles the schema and validates the selected provider subset before
provider dispatch. It maps the schema to OpenAI Responses `text.format`, Gemini
Interactions `response_format`, or Anthropic Messages `output_config.format`.
The proxy validates the final JSON against the same schema before durable
success. Other provider protocols reject structured requests before provider
work.

A structured request does not use output continuation or a repair inference.
Invalid provider JSON causes one terminal provider failure. This rule prevents
a malformed response from causing a second paid inference.

The idempotency key binds one request intent to one authenticated tenant. The
intent contains the resolved provider, model, and canonical request body. A
different intent with the same key returns `409 Conflict`. Request-timeout and
transport settings are not part of the request intent.

The durable record has these states:

- `not_dispatched`: The proxy saved the request intent before provider work.
- `dispatched`: The proxy started provider work.
- `succeeded`: The proxy saved the validated JSON result.
- `failed`: The proxy saved a known provider failure.
- `uncertain`: The proxy cannot prove the result after provider dispatch.

Use authenticated `GET /v2/requests` with the same `Idempotency-Key` header.
A successful record returns the exact saved JSON with status `200`. An active
record returns safe timestamps and elapsed time with status `202`. A failed
record returns its safe failure with the recorded status. An uncertain record
returns `409 structured_request_outcome_unknown`. Every reconciliation response
sets `Cache-Control: no-store` because its content depends on the tenant and key.

An identical `POST /v2` submission reuses a succeeded or active record. A
failed record permits an explicit new attempt with the same request intent.
An uncertain record never starts provider work again. A process restart changes
each interrupted `dispatched` record to `uncertain`.

The proxy stores private records under
`server.asset_store_path/structured-requests`. Directories use mode `0700`, and
records use mode `0600`. A record contains tenant, key, and intent hashes. It
also contains routing, timing, state, safe failure, and successful result data.
It does not contain prompts, schemas, credentials, or raw provider bodies.

Terminal records use `server.asset_retention_seconds`. Uncertain records remain
available because automatic removal would erase the no-resubmit decision. A
structured record does not contain a provider resource id and cannot restart
uncertain provider work.

### Request work budgets

Every authenticated operation that can run long work accepts one optional
header:

```text
X-LLM-Proxy-Request-Timeout-Seconds: N
```

`N` is the positive whole-number wall-clock budget, in seconds, for that
request. The budget begins before body parsing and covers validation, asset
upload, queue admission, every provider call, OpenAI background polling, and
response construction for `GET /`, `POST /`, `POST /v2`,
`POST /model/v1/assets`, and `POST /dictate`.

If the header is omitted, `server.request_timeout_seconds` is the effective
budget. A supplied value must be in the inclusive range
`1..server.max_request_timeout_seconds`; the proxy never rounds, clamps, or
replaces it. A blank, repeated, signed, fractional, nonnumeric, zero, negative,
or over-limit value returns this exact `400 application/json` response before
queue admission:

```json
{"error":{"code":"invalid_request_timeout","max_request_timeout_seconds":3600}}
```

Every accepted response, including errors, echoes the effective value in
`X-LLM-Proxy-Request-Timeout-Seconds`. If that budget expires, the proxy cancels
the remaining queued, provider, or polling work and returns:

```json
{"error":{"code":"request_timeout","request_timeout_seconds":900}}
```

with `504 application/json`. The value shown is the effective budget for that
request.

This server work budget is not a client transport timeout. A caller may still
cancel sooner with a Go context, a process signal, or an explicitly configured
HTTP transport policy, and no response is guaranteed after caller cancellation.
The bundled clients impose no separate total-response deadline by default.

Accepted upstream requests emit safe terminal evidence with the effective
budget and one of `validation_failure`, `success`, `proxy_timeout`,
`proxy_overload`, `provider_failure`, or `caller_cancelled`. Queue-capacity
rejection is proxy overload, not a provider failure.

Every routed proxy request also emits one `proxy request phase summary`
structured event keyed by the same `request_id` returned in
`X-LLM-Proxy-Request-ID`. It carries the query-free `endpoint`, canonical
`provider` and `model`, `request_timeout_seconds`, terminal `status` and
`outcome`, `total_latency_ms`, and these explicit phase totals:

- `authentication_ms`: static or managed tenant authentication.
- `upstream_admission_ms`: bounded queue admission and worker acquisition.
- `upstream_rate_limit_wait_ms`: time waiting for configured origin windows.
- `provider_http_ms`: observed provider HTTP work through response-body close.
- `provider_poll_wait_ms`: proxy-owned sleeps between provider resource polls.
- `continuation_wait_ms`: proxy-owned sleeps between missing-suffix attempts.
- `response_formatting_ms`: construction and writing of the selected proxy response.
- `managed_usage_enqueue_ms`: synchronous construction and non-blocking enqueue
  of a managed usage event; detached persistence is outside the request.

An unused phase is present with zero. The totals use one request-scoped
monotonic accumulator. They describe proxy-observed boundaries, not provider
execution time or billing, and unclassified orchestration time must not be
derived by subtracting phase totals from total latency.

OpenAI create and poll observations and every provider-neutral continuation
attempt emit `proxy provider progress` under the same request ID. These events
contain canonical provider and model values, `progress_kind`, a count,
`completion_signal`, and `elapsed_ms`. OpenAI progress also contains
`provider_state`, `current_output_bytes`, and `accumulated_output_bytes`.
Gemini create, poll, cancel, and delete operations use the same event. Gemini
progress contains `provider_state` and validated `provider_error_codes` for a
terminal fault. It does not contain a provider message or interaction ID. The
event contains no prompt, message, generated text, provider body, credential,
cookie, or tenant secret. Phase fields stay in structured logs. They do not
extend responses, OpenAPI, bundled clients, or managed usage persistence.

For managed tenants, the proxy writes and flushes the selected response before
it attempts a non-blocking usage enqueue. Each management runtime has one FIFO
channel bounded by `management.usage_queue_size` and starts one writer goroutine
when the first event is accepted. The writer attempts each accepted insert once
under a detached five-second budget, in acceptance order, without per-event
goroutines or retries. A dedicated database-write gate sequences usage inserts
with management mutations without acquiring the management mutation mutex.
Authentication bypasses both, so database latency neither retains the request
handler nor serializes authentication for another request, and it cannot change
an already selected response.

When the channel is full, the newest event is dropped, previously accepted work
stays queued, and the proxy emits one `managed_usage_queue_full` warning with
safe request metadata. Accepted events are process-local, at-most-once work
until their database insert commits. Queue contents are not crash-durable, and
an insert failure or process termination can lose an uncommitted event. Managed
usage is operational telemetry, not a billing, accounting, or provider-job
ledger; summaries may briefly lag completed proxy responses.

Internally, `server.workers` limits concurrent upstream provider HTTP
operations and `server.queue_size` limits upstream HTTP operations waiting for a
worker. Long OpenAI background-response poll sleeps do not occupy a worker slot;
only the actual upstream HTTP request or poll does. This admission queue stores
no provider job ids and provides no durable retry or resume behavior.

## Configuration

The service reads service configuration from `config.yml`. The default path is
`config.yml` in the current working directory. Use `--config /path/config.yml`
to select a different file.

The loader also reads `providers.yml` from the selected configuration
directory. That file is the only source for supported providers, exact models,
provider offerings, operations, controls, limits, and prices.

Before parsing YAML, the loader expands `${NAME}` placeholders from process
environment variables and from an optional `.env` file in the same directory as
the selected config file. Process environment values override `.env` values.
Missing placeholders in `config.yml` fail startup. The loader does not mutate
the process environment. Runtime code receives only validated configuration.

The complete service configuration is in
[configs/config.yml](configs/config.yml). The complete provider catalog is in
[configs/providers.yml](configs/providers.yml). See the
[provider catalog reference](docs/provider-catalog.md) for its field mapping
and provider procedure.

`server.workers` is not the number of client requests that may be connected at
once. It is the upstream provider HTTP concurrency limit shared by text
generation and dictation. `server.queue_size` is the number of additional
upstream HTTP operations that may wait for that shared limit before the proxy
returns `503 request queue full`.

`server.upstream_rate_limits` applies strict rolling-window call limits in that
same shared HTTP layer. Rules match an exact normalized upstream origin
(`scheme://host[:port]`), so providers that use the same origin share one
budget while different origins are independent. A delayed call remains in the
bounded upstream queue but does not occupy a worker. Every upstream attempt,
including transport retries and OpenAI response retries, consumes one call.
The shared client reserves the slot only after worker capacity is available;
if the rolling window is still full, it releases that worker before waiting.
An absent or empty list disables rate limiting; invalid and duplicate rules
fail startup.

```yaml
server:
  upstream_rate_limits:
    - origin: "https://api.openai.com"
      max_requests: 60
      interval: "1m"
```

`origin` accepts only an exact `http` or `https` origin without user info,
path, query, or fragment. `max_requests` must be positive, and `interval` must
be a positive Go duration such as `500ms`, `1s`, or `1m`. When a call must wait,
the shared client emits a structured info log with the origin, limit, interval,
and wait duration; context cancellation during the wait emits a warning and
keeps the existing request-timeout error mapping.

### Provider support matrix

Provider selectors and aliases are accepted anywhere the public API accepts
`provider`. Omitted text models use the authenticated tenant default when
`provider` is omitted. Otherwise, they use the selected provider offering that
declares the text default. This table describes current runtime capabilities
through `llm-proxy` and the defaults in [configs/providers.yml](configs/providers.yml).
Upstream providers may expose additional speech APIs that need separate proxy
adapters before they are available through `/dictate`.

| Provider selector | Aliases | Wire contract | Execution lifecycle | Configured default text model | Credential field | Default base URL | Dictation | Web search |
|-------------------|---------|---------------|---------------------|-------------------------------|------------------|------------------|-----------|------------|
| `openai` | none | `openai_responses` | `pollable_resource` | `gpt-4.1` | Tenant-managed API key | `https://api.openai.com/v1` | Yes: `gpt-4o-mini-transcribe`, `gpt-4o-transcribe` | Yes, on marked OpenAI models |
| `meta` | none | `openai_chat_completions` | `synchronous_completion` | `muse-spark-1.1` | Tenant-managed API key | `https://api.meta.ai/v1` | No | No |
| `deepseek` | none | `openai_chat_completions` | `synchronous_completion` | `deepseek-v4-flash` | Tenant-managed API key | `https://api.deepseek.com` | No | No |
| `dashscope` | `qwen` | `openai_chat_completions` | `synchronous_completion` | `qwen-plus` | Tenant-managed API key | Tenant-managed Singapore workspace URL | No | No |
| `moonshot` | `kimi` | `openai_chat_completions` | `synchronous_completion` | `kimi-k2.6` | Tenant-managed API key | `https://api.moonshot.ai/v1` | No | No |
| `minimax` | none | `openai_chat_completions` | `synchronous_completion` | `minimax-m2.7` | Tenant-managed API key | `https://api.minimax.io/v1` | No | No |
| `siliconflow` | none | `openai_chat_completions` | `synchronous_completion` | `deepseek-reasoner` | Tenant-managed API key | `https://api.siliconflow.com/v1` | Yes: `sensevoice-small` | No |
| `zai` | none | `openai_chat_completions` | `synchronous_completion` | `glm-5.1` | Tenant-managed API key | `https://api.z.ai/api/paas/v4` | Yes: `glm-asr-2512` | No |
| `gemini` | none | `gemini_interactions` | `pollable_resource` | `gemini-3.5-flash` | Tenant-managed API key | `https://generativelanguage.googleapis.com/v1beta` | No | No |
| `anthropic` | `claude` | `anthropic_messages` | `synchronous_completion` | `claude-sonnet-4-6` | Tenant-managed API key | `https://api.anthropic.com` | No | No |
| `xai` | none | Model-specific: `grok-4.5` uses `openai_responses`, and other text models use `openai_chat_completions` | `synchronous_completion` | `grok-4.3` | Tenant-managed API key | `https://api.x.ai/v1` | Yes: `xai-stt` | No |

All upstream provider credentials are server-side only. Client requests must
never send OpenAI, Meta, Anthropic, xAI, Gemini, or other upstream API keys.
Media input is model-scoped. The checked-in catalog declares media input only
when the provider offering has a code-owned transport.

| Exact model set | Provider | Standard-message media inputs |
|-----------------|----------|-------------------------------|
| All 11 OpenAI text models in the GPT-4 and GPT-5 families | OpenAI | `image` |
| All 10 Claude text models in the Fable, Sonnet, Opus, and Haiku families | Anthropic | `image` |
| Both Gemini text models | Gemini | `image`, `audio` |
| `grok-4.5` | xAI | `image` |
| Every other configured model | Its configured provider | None |

### Provider catalog

The versioned [provider catalog](docs/provider-catalog.md) has root records for
operations, publishers, families, exact models, and provider definitions. Each
provider definition owns its fields, transports, provider offerings, and
prices.

The loader rejects unknown fields and unsupported schema versions. It also
rejects invalid identities, references, defaults, protocols, capabilities,
limits, and prices.

The current snapshot contains 11 providers, 62 exact models, 63 provider
offerings, and 63 price records. The application compiles one immutable
registry from this snapshot.

Each provider offering references one exact model and one provider transport.
The transport selects one reusable protocol adapter. Provider identifiers do
not select protocol code.

Public resources omit credential fields, setting fields, authentication data,
environment names, and upstream model identifiers. Management resources return
safe field definitions and masked connection state.

OpenAI offerings use `request_profile` to select a stable payload shape:

| Request profile | Payload behavior |
|-----------------|------------------|
| `openai_responses_temperature` | Adds `temperature`. |
| `openai_responses_temperature_tools` | Adds `temperature` and enabled web-search tools. |
| `openai_responses_reasoning_tools` | Adds reasoning controls and enabled web-search tools. |

All OpenAI Responses text requests also send `background: true` and
`store: true`. llm-proxy polls the stored OpenAI response server-side until it
reaches a documented terminal state or the request's effective work budget
expires. Only `queued` and `in_progress` are pending states; unknown states do
not trigger polling. Only `completed` can produce a successful response.
Plain REST callers use one `GET /`, `POST /`, or `POST /v2` request and receive
the final formatted answer; they do not stream, poll, or follow a separate
resume endpoint. Separately published provider-specific deferred, batch, or
asynchronous APIs are not implicit variants of these routes and are not
activated by an arbitrary response `id`.

The OpenAI adapter requests provider storage because background Responses need
`store: true`. The response identifier exists only in memory for the active
proxy call and is never returned to the caller or persisted by llm-proxy. The
current adapter stops observing the resource when the caller cancels or the
proxy budget expires; it does not issue an upstream cancel or delete after
success, failure, timeout, or cancellation, so OpenAI account retention policy
continues to govern that stored resource.

Gemini 3.x Interactions requires `store: true` for background execution. The
proxy sends `Api-Revision: 2026-05-20`, polls only `queued` and `in_progress`,
and never exposes or persists an interaction id. On every exit it cancels an
interaction that is still active and then deletes the resource; a terminal
interaction is deleted directly. Cancel and delete each receive an independent
bounded cleanup context, so a stalled or failed cancel cannot consume the
delete attempt. A failed deletion prevents a successful or output-limit result
from escaping as success.
When a transport declares continuation actions, output-limit continuation
starts a distinct inference request. It never treats an arbitrary upstream
identifier as pollable state. Each Gemini Interactions route rejects a request
that contains an `assistant` message.

Provider-specific details:

* OpenAI is the only provider currently exposed with `web_search` support.
  This support applies only to catalog entries with `web_search: true`.
  OpenAI derives each endpoint from its provider transports in `providers.yml`.
  Text and dictation use the same catalog-defined credential field.
  Each catalog model with image input sends ordered Data URLs through OpenAI Responses.
  The adapter uses `detail: auto` for each image.
* OpenAI-compatible text providers send chat completion requests with
  `Authorization: Bearer <api_key>` and the selected provider base URL. The
  shared adapter normalizes `finish_reason=length` into the common
  missing-suffix loop and accepts the assembled text only after
  `finish_reason=stop`; `content_filter`, `tool_calls`, missing, and
  provider-specific non-stop reasons are upstream failures.
* Moonshot Kimi K3 accepts exact public `reasoning_effort` values `low`,
  `high`, and `max`. The adapter sends the selected value as the top-level
  provider field and omits it when no value is selected. Kimi K2.6 and both
  Kimi K2.7 Code routes keep Moonshot's thinking default because the proxy does
  not send a `thinking` field. Kimi K3, K2.6, K2.7 Code, and K2.7 Code
  Highspeed accept ordered JPEG, PNG, or WebP inputs through canonical
  `POST /v2` messages. The adapter sends exact bytes as ordered Data URL blocks
  before message text. Only visible answer `content` enters the proxy response
  and usage path. For output continuation, the adapter stores the complete
  truncated assistant message as private state. It sends that message,
  including `reasoning_content`, only to the next upstream Kimi request.
  Provider `reasoning_content` stays absent from public responses and logs. See
  Moonshot's
  [Kimi K3 guide](https://platform.kimi.ai/docs/guide/kimi-k3-quickstart),
  [reasoning-effort guide](https://platform.kimi.ai/docs/guide/use-reasoning-effort),
  [vision guide](https://platform.kimi.ai/docs/guide/use-kimi-vision-model),
  and [Chat Completions reference](https://platform.kimi.ai/docs/api/chat).
* MiniMax uses selector `minimax`, a tenant-managed API key, and
  `https://api.minimax.io/v1`. `minimax-m2.7` remains the default. The catalog
  also includes M2.7 Highspeed, M2.5, M2.5 Highspeed, M2.1, M2.1 Highspeed,
  and M2. The shared Chat Completions adapter maps public `max_tokens` to
  upstream `max_completion_tokens`. All seven routes enforce the documented
  204,800-token completion maximum. The proxy does not expose MiniMax-specific
  reasoning, tool, streaming, or multimodal controls. See the official
  [OpenAI SDK guide](https://platform.minimax.io/docs/api-reference/text-openai-api),
  [Chat Completions reference](https://platform.minimax.io/docs/api-reference/text-chat-openai),
  and [PAYG prices](https://platform.minimax.io/docs/guides/pricing-paygo).
* Meta Model API requests use that shared Chat Completions adapter with the
  exact `meta` selector and `https://api.meta.ai/v1` base URL. The provider
  offerings are `muse-spark-1.1` and `muse-spark-1.2`. Muse Spark 1.1 remains
  the Meta default. llm-proxy exposes the public `max_tokens` input upstream as Meta's current
  `max_completion_tokens` field rather than Meta's deprecated `max_tokens` field.
  The proxy exposes both models only as text generation through `GET /`,
  `POST /`, and `POST /v2`. Meta describes Muse Spark 1.2 as coding-focused.
  This focus does not change the provider transport or add agent orchestration.
  The proxy does not expose Meta dictation, `web_search`, tools, multimodal
  input, or a Responses API fallback. See Meta's
  [Muse Spark 1.2 announcement](https://research.meta.ai/blog/introducing-muse-code-and-muse-spark-1-2),
  [model reference](https://dev.meta.ai/docs/models),
  [Chat Completions reference](https://dev.meta.ai/docs/protocols/chat-completions),
  and [pricing and rate-limit documentation](https://dev.meta.ai/docs/pricing-rate-limits).
* Each dictation-capable provider has a `multipart_transcription` transport in
  `providers.yml`. That transport owns the exact endpoint and model-field rule.
* Gemini text requests use native `POST /interactions` against the configured
  `v1beta` base URL with `x-goog-api-key` and
  `Api-Revision: 2026-05-20`. Every configured Gemini 3.x route sends
  `background: true` and `store: true`. The proxy polls its `queued` and
  `in_progress` states. User messages become `user_input` steps. System
  messages become the top-level `system_instruction`. Assistant history is
  invalid because the public request cannot carry provider interaction state or
  thought signatures. Only `completed` with visible model text succeeds.
  `incomplete` returns a provider error after interaction deletion. A `failed`
  response can include diagnostic errors. The adapter accepts each code only
  when it is a canonical HTTPS URI of 2,048 characters or fewer. The URI must
  contain a host and a clean path. It must not contain user information, a
  query, a fragment, or an escaped path. The final path segment must be a
  1-to-64-character `snake_case` value. The adapter discards each provider
  message. A malformed URI is a response protocol failure. A failed
  Interaction is retryable only when every URI has a retryable final path
  segment. The retryable segments are `aborted`, `api_error`,
  `deadline_exceeded`, `rate_limit_exceeded`, `service_unavailable`, and
  `too_many_requests`. An empty list or an unknown segment fails closed.
  `cancelled`, `requires_action`, malformed, missing, and unknown states are
  safe upstream failures. Usage
  totals map from `total_input_tokens`, `total_output_tokens`, and
  `total_tokens`, preserving provider-counted thought tokens. For exact models
  whose catalog declares `media_inputs`, ordered image and audio attachments
  become native typed interaction content after the message text. The adapter
  sends an inline request when the complete encoded request is at most the
  offering's inline limit. It streams exact media bytes through the Gemini
  Files API when the encoded request is larger, then deletes each provider
  file after the interaction ends. See Google's
  [Interactions overview](https://ai.google.dev/gemini-api/docs/interactions-overview),
  [file input methods](https://ai.google.dev/gemini-api/docs/file-input-methods),
  [Files API guide](https://ai.google.dev/gemini-api/docs/files),
  [background execution guide](https://ai.google.dev/gemini-api/docs/background-execution),
  and [Interactions API reference](https://ai.google.dev/api/interactions-api).
* Anthropic text requests use `POST /v1/messages` with `x-api-key` and
  `anthropic-version: 2023-06-01`. The proxy maps system messages to Anthropic's
  top-level `system` field. Anthropic requires `max_tokens`.
  When the client omits it, the proxy sends the selected model output limit.
  `stop_reason=max_tokens` starts the common missing-suffix loop.
  `end_turn` or `stop_sequence` completes the assembled answer.
  Tool use, paused turns, refusals, and unknown reasons remain upstream failures.
  Each catalog model with image input sends ordered base64 image blocks before the message text.
* Z.AI dictation uses the catalog-defined GLM-ASR transport with the selected
  exact dictation model.
* Most xAI text models use xAI's OpenAI-compatible `/chat/completions` API at
  `https://api.x.ai/v1`. The `grok-4.5` route uses synchronous `/responses`
  with `store: false`. Its image blocks use `detail: high`. Grok/xAI dictation
  uses the catalog-defined xAI STT transport. The upstream STT endpoint does
  not receive a `model` multipart field.

All client keys, provider API keys, and tenant defaults are tenant-owned
management state. An explicit request for a provider without a saved tenant
credential returns `503 provider not configured`. An omitted provider resolves
through the authenticated tenant's saved default and requires a saved
credential for that route. Static provider base URLs and paths belong to
`providers.yml`. DashScope instead references a tenant-owned `base_url` provider
field. The provider catalog must contain every supported provider and route.
The `management` configuration is mandatory. Provider blocks in `config.yml`
are unknown YAML keys and fail startup.

### Self-service management UI

The mandatory management configuration exposes TAuth-protected management APIs
under `/api/management`. The browser site is static and lives in `site/`: `/` is the
anonymous-only product landing page and `/app/` is the only authenticated app
route. The landing page replaces itself with `/app/` whenever MPR UI reports
its documented authenticated lifecycle, including restored and refreshed TAuth
sessions. Every other public header owns the same declarative **Log In** action,
and successful interactive authentication redirects to `/app/`. An anonymous
direct visit to `/app/` returns to `/` after MPR UI reports its unauthenticated
lifecycle state.
There is no second signed-out application screen.
The site is declared as a `github_pages` resource in
`.mprlab/deploy/resources.yml`.
`make release`, `make publish`, and `make deploy` delegate that resource to the
exact sibling `../mprlab-gateway`; GitHub Actions is not used for Pages
deployment. The backend does not serve management HTML or assets; `GET /`
remains a proxy endpoint and returns `403` without a tenant `key`. The backend
serves public `/config-ui.yaml` from the loaded management config and
`/api/public/capabilities` from the validated provider registry. The GitHub
Pages frontend consumes these REST resources from `llm-proxy-api`; neither
resource exposes provider credentials, tenant state, or management secrets.

The `/app/` UI uses the shared MPR shell through API-served `config-ui.yaml`,
literal `mpr-ui@latest` assets, `mpr-ui-config.js`,
`<mpr-header data-config-url="...">`, the `@latest` bundle marker, `<mpr-user>`,
and the shared compact `<mpr-footer>`. MPR UI owns browser authentication,
session restoration, refresh, logout, and all communication with TAuth;
application JavaScript does not load a second TAuth client, implement those
operations, or apply MPR UI config. The Pages artifact contains no static
`config-ui.yaml` or `llm-proxy-config.json`. The declared Pages build starts the
backend's public-capabilities-only REST surface, then the frontend-owned Node
renderer fetches `/api/public/capabilities`. The renderer writes
`https://llm-proxy-api.mprlab.com/config-ui.yaml` into every auth-aware header,
replaces the public landing's capability marker with the returned model-centric
catalog. It replaces the routing marker with a five-stage route from the
product through LLM Proxy, model family, exact model, and provider offering.
Every route item remains in semantic HTML without JavaScript. Browser
enhancement supplies a multi-choice weight-access filter and a single-choice
capability filter in the route title row. Proprietary and Text are the default
filters. Browser enhancement also supplies family, model, and provider
selection and the final route display. The capability catalog supplies
one all-characteristics search surface, disclosed match-all capability filters,
sortable table headers, a live result count, and reset. Node exists only in the
Pages build stage; the published artifact is static and has no runtime renderer
or environment-expansion path. A missing or invalid provider catalog, management config
attribute, or landing catalog marker fails the site build. That single
API-served YAML points browser management API
calls, generated usage examples, and MPR UI/TAuth at the configured origins.
Browser-facing values are projected from the already-loaded backend
`config.yml`.

Every public HTML route also uses the MPR component shell. `/`, `/docs/`,
`/resources/`, every generated resource article, `/privacy/`, and `/terms/`
render the same canonical `mpr-header` and compact sticky `mpr-footer` from
`scripts/public_site_shell.mjs`, in that exact order around one `main` region.
The authenticated app uses the same footer. The MPR footer keeps its host in
flow while its hydrated surface stays fixed, so the end of `main` remains
accessible. It contains crawlable Resources, Privacy, Terms, and GitHub links
plus the active **Built by Marco Polo Research Lab** project-catalog drop-up. A
semantic fallback inside the component keeps
those anchors and the native project drop-up usable when JavaScript is
unavailable; MPR UI replaces that fallback when the component hydrates. The
shell validator and every public-page generator reject a missing, duplicated,
misordered, or divergent header/main/footer contract, content outside `main`,
and any public anchor that bypasses authentication by linking directly to
`/app/`.

Release rendering also copies the exact committed `docs/openapi.yaml` bytes to
the Pages artifact root and verifies the SHA-256 provenance embedded in the
derived `site/docs/index.html`. `site/openapi.yaml` is intentionally forbidden:
there is no independently editable or generated schema copy in site source.

`scripts/generate_legal_pages.mjs` is the single maintained source for the
canonical `/privacy/` and `/terms/` pages. It owns their route metadata,
indexable sitemap entries, effective and updated dates, exact
`mpr-legal-document` section data, and semantic no-JavaScript fallback. The
current policy source dates both documents `2026-08-08`. Privacy statements are
grounded in the repository's MPR UI/TAuth session boundary, encrypted provider
credentials, digest-only client secrets, content-free usage records, and
Google Analytics plus LoopAware public-page telemetry. Terms describe the
current proxy/provider and user-responsibility boundaries without introducing
an unsupported payment or availability contract.

MPR UI owns the browser authentication presentation, lifecycle, and all browser
communication with TAuth. Public headers use MPR UI's documented sign-in label
and successful-authentication redirect attributes.
The app registers the documented `mpr-ui:auth:authenticated` and
`mpr-ui:auth:unauthenticated` lifecycle listeners, uses the header's documented
`data-mpr-auth-status` only to reconcile the current state after startup, and
does not request
`/api/management/account` until MPR UI reports `authenticated`. LLM Proxy does
not inspect TAuth cookies, storage, tokens, or claims and does not call TAuth
authentication endpoints. After MPR UI reports authentication, a management
API failure renders an explicit app error; it does not reinterpret the
MPR UI session as signed out.

The Go backend consumes TAuth's published `pkg/sessionvalidator` for the
configured session cookie. It does not maintain a second JWT parser or claims
schema; llm-proxy adds only its product-owned tenant, required-expiry, and
principal checks after TAuth validation. Authentication rejections are logged
only as stable categories such as `missing_cookie`, `expired`,
`invalid_issuer`, or `wrong_tenant`; session values and identity claims are
never logged.

Required hosted values are profile-specific:

| Field | Purpose |
|-------|---------|
| `management.public_origin` | Static frontend origin allowed for credentialed management CORS, for example `https://llm-proxy.mprlab.com`. |
| `management.ui_description` | Browser-facing MPR UI environment description. |
| `management.ui_origins` | Browser-facing MPR UI allowed origins served from `/config-ui.yaml`. |
| `management.admin_emails` | Exact administrator email addresses. In public config, populate this from `${LLM_PROXY_MANAGEMENT_ADMIN_EMAILS}` as a YAML flow sequence such as `["admin@example.invalid","ops@example.invalid"]` so personal admin addresses stay out of the repository. |
| `management.tauth_url` | Browser-facing TAuth API origin served from `/config-ui.yaml`. |
| `management.tauth_tenant_id` | TAuth tenant id that issues accepted sessions. |
| `management.google_client_id` | Browser-facing Google OAuth web client id for the `llm-proxy` TAuth tenant. |
| `management.login_path` | Browser-facing TAuth Google login path. |
| `management.logout_path` | Browser-facing TAuth logout path. |
| `management.nonce_path` | Browser-facing TAuth nonce path. |
| `management.session_path` | Browser-facing TAuth session restore path, normally `/auth/session`. |
| `management.jwt_signing_key` | Internal signing key used to validate the TAuth session cookie. |
| `management.jwt_issuer` | JWT issuer, normally `tauth`. |
| `management.session_cookie_name` | Exact app/environment TAuth session cookie name. |
| `management.database_path` | Required SQLite location for provider connections, provider profiles, defaults, generated-secret digests, and usage events. |
| `management.usage_queue_size` | Positive capacity of the process-local FIFO for asynchronous managed usage persistence. Defaults to `1024`; this queue is independent from `server.queue_size`. |
| `management.provider_key_encryption_key` | Required base64-encoded 32-byte key used for AES-GCM encryption of tenant-owned provider API keys at rest. Generate with `openssl rand -base64 32` and store it with backend deployment secrets. |
| `management.management_api_origin` | Browser-facing management API origin served from `/config-ui.yaml` under `llmProxy.managementApiOrigin`. |
| `management.proxy_origin` | Browser-facing public proxy origin served from `/config-ui.yaml` under `llmProxy.proxyOrigin` for generated examples. |

After the shared `mpr-ui` shell reports authentication, the frontend loads the
account through `GET /api/management/account`. A new TAuth subject receives one
`Default` tenant. Each account may create, rename, select, and delete its own
tenants; deleting the final tenant returns `409 Conflict`. Every owned tenant is
operational at the same time: each generated secret independently selects that
tenant's credentials, defaults, and usage owner. The browser has no global
active-tenant state, activation flag, or tenant URL parameter.

Tenant lifecycle and configuration live in Settings. One compact `Tenant
access` row contains the `Tenant` selector, modal Rename, client-key state and
one-time reveal/copy controls, confirmed Replace key, confirmed Delete tenant,
and Create tenant. The selected tenant is only the current Settings editor
context; it is not an activation state. Switching it while the current editor
contains unsaved input requires an explicit discard confirmation and clears
one-time generated secrets and revealed provider credentials from browser
state. It does not change the independent `Usage tenant` filter. If the
selected tenant has no llm-proxy client key, the frontend creates one through
`POST /api/management/tenants/:tenant_id/secrets` and presents the one-time
value masked in the read-only Key field with explicit Show and Copy actions.
Settings opens automatically and cannot be dismissed until the profile has both
that client key and at least one configured provider connection. Only
`tenant.has_secret` and `providers[].configured` satisfy this setup gate. A
typed credential draft or a static environment value does not satisfy it.
DashScope also requires the tenant's exact Singapore Model Studio workspace
URL. Pasting into a credential provider field immediately starts one server-side
operational verification. The operation uses the exact provider, text model,
and submitted provider fields. It does not wait for blur, provider
switching, Settings close, or a separate action. While the attempt is active,
Settings announces `Verifying key` and keeps the key input available. It locks
tenant, provider, model, reveal, remove, routing, and close actions. A newer paste or a
tenant, provider, model, editor, or authentication context change cancels or
invalidates the prior request. Other provider field edits still autosave through
the same verify-before-persist operation when the user leaves the field,
switches providers, or closes Settings.

The verifier uses the selected route's exact lifecycle for one fixed,
non-user-content probe. Synchronous routes make one provider request. A
pollable Gemini route creates one stored background interaction and observes it
through the shared `pollable_resource` lifecycle. The verifier cancels an
active interaction and deletes every stored interaction before it accepts the
credential. Every request uses the shared upstream worker, queue,
origin-rate-limit, and management request boundaries. The verifier applies the
selected transport's catalog-owned visibility policy after creation. It does
not retry a create, an undeclared response, cancel, or delete. It does not start
a continuation or record managed usage.
Only an accepted provider connection and model enter the provider connection
transaction. That transaction encrypts each secret field and saves each setting
field. It also saves the provider profile, reconciles routing defaults, and
returns the complete tenant profile. When the
saved provider text model changes, that transaction can update the active text
route. It does this only when the same provider owns that route. It also clears
a reasoning effort when the new model does not support it. A different active
provider remains unchanged. The browser then clears the raw draft and returns
to the masked presentation. A successful first key unlocks mandatory Settings.

Credential/model rejection returns `422 provider_key_rejected`; an unconfirmed
provider rate limit, timeout/cancellation, or outage/malformed response returns
the documented `429`, `504`, or `503` provider-neutral verification error.
None saves the candidate. A first failure leaves the provider unkeyed, while a
failed replacement leaves the previously verified encrypted key, provider
settings, and routing defaults active. The current editor retains only the
rejected draft for correction or explicit retry and states which of those two
outcomes applies. An empty saved secret field retains its existing value. A
DashScope workspace URL change verifies the retained key
against the new URL before persistence. Settings remains open until the user
closes it explicitly.
Text and dictation provider/model defaults plus reasoning effort autosave on
selection, while the tenant system prompt autosaves when the user leaves the
changed field. Settings serializes every mutation that returns a complete
management profile, including provider and routing-default autosaves, provider
removal, and client-key creation or replacement. A close request
locks the controls and waits for the mutations already in progress. If a client
key is created or replaced during that wait, Settings stays open so the one-time
value can be copied before a second explicit close. A failed save retains the
edited values for retry. Feedback caused by Settings activity appears in the
Settings title row; page-level activity feedback remains in the MPR header.
Removing the last configured provider connection makes Settings
mandatory again, while a failed automatic client-key request remains retryable
through Create key.

Signed-in users also choose each provider's text model and provider-specific
system prompt, choose routing defaults, and replace llm-proxy client keys after
confirming that the prior value stops working immediately. A client key cannot
be deleted independently; access is rotated through replacement or removed
with the owning non-final tenant. Management requires
`management.database_path` so signups, enabled
providers, defaults, generated secret digests, and committed usage events
survive restarts in a GORM-managed SQLite database at the configured location.
SQLite is the sole runtime source of truth; there is no application
authentication cache, replica, dual read, or invalidation path. Runtime
connections use WAL journaling and a five-second busy timeout. Managed
authentication uses the caller context and one read-only GORM transaction to
load the tenant, provider connection, and provider profile records from one
consistent SQLite snapshot.
Authentication and single usage-event inserts do not acquire the process-wide
management mutation lock; management flows retain that lock where they
coordinate state transitions, while their existing GORM transactions own
multi-statement database atomicity.
The packaged management config uses
strict expandable placeholders for the hosted profile values; define every
`LLM_PROXY_MANAGEMENT_*` key in the API runtime environment. Local `make up`
projects those values from `configs/.env.local` into the ignored, API-scoped
`configs/.env.api.local`. Both files are ignored; tracked environment examples
are documentation only and never participate in runtime configuration.
Placeholders without matching values fail startup.
The runtime config file is never mutated for user signup, provider enablement,
or usage tracking, and database access must stay on GORM model APIs without raw
SQL. Generated secrets continue to authenticate the public proxy endpoints with the same
`key=<tenant secret>` query parameter. Provider API keys are accepted only
through authenticated management endpoints. Every nonempty new or replacement
key is operationally verified for its exact provider and selected text model
before it is encrypted at rest with AES-GCM and persisted. Normal save,
profile, and administrator responses return only masked key status. The sole
raw-key response is the explicit
owner-authenticated
`POST /api/management/tenants/:tenant_id/provider-connections/:provider/fields/:field/reveal`
management action, which requires the configured management origin and returns
`Cache-Control: no-store`. Provider connection records store catalog field
values. Separate provider profile records store the selected text model and
provider-specific system prompt. Managed text requests that
select a provider and omit `model` use the saved provider text model; when
request-level system instructions are omitted, the provider-specific system
prompt is injected before routing upstream. The F014 ownership migration accepts
only already-encrypted legacy provider-key rows, decrypts them with their prior
user binding, and re-encrypts them with the preserved opaque tenant id as
AES-GCM associated data. Plaintext, corrupt, orphaned, or non-canonical rows
fail startup before the migration transaction begins. The backend decrypts
secret provider fields only in the provider request path and the explicit owner
reveal action. This protects database dumps, backups, and direct storage access.
It is not a user-only decryption or zero-knowledge guarantee. Generated tenant
secrets are returned once and the
database retains only their SHA-256 digest. Replacing a generated secret
immediately makes future public proxy requests with the prior value return
`403`. Deleting a non-final tenant removes its secret digest with the rest of
the tenant-owned state.

Managed routing defaults contain complete canonical provider/model pairs plus a
route-bound `reasoning_effort`. A provider is eligible only while that tenant
has all required provider connection fields. A provider default text model applies when a request
names that provider and omits a model. The tenant text routing pair applies when
a request omits both provider and model. Settings explains both scopes through
help tooltips. Choosing a text routing provider initializes its routing model
from that provider's saved default, after which the routing model can be changed
independently. The text pair is both empty only when no provider connection exists.
The dictation pair is both empty when none of the keyed providers supports
dictation; in that state the Settings controls are disabled and no default
dictation example is shown. Saving provider settings preserves an
eligible current provider, while a changed provider text model also updates the
active same-provider text default and clears an incompatible reasoning effort.
A different active provider remains unchanged. Removing a provider connection
preserves an eligible current default and otherwise selects the first eligible
provider by canonical provider id, using that provider's saved text model or
configured dictation default model. The provider mutation and both reconciled
routing pairs are one database transaction, so a profile never exposes a
default whose key was removed or an active provider-model change that was not
applied.

`PUT /api/management/tenants/:tenant_id/defaults` accepts only these eligible
complete pairs and resolves the supplied text pair before validating the
effort. Empty is the explicit unset effort value; a nonempty value must be in
that exact route's declared list. A partial pair, unkeyed or unknown provider,
unsupported dictation provider, cross-provider model, or incompatible effort
returns `400 managed_routing_defaults_invalid` before any default is persisted.

The profile exposes connection eligibility through `providers[].configured`.
It exposes capability data only through
`providers[].text_models[].reasoning_effort`. It has no global option list or
provider-level reasoning capability. The Settings routing selectors contain
only configured providers. Dictation also requires declared dictation support.
The form keeps Text provider, Text model, and Reasoning effort in one desktop
row. It clears an incompatible value after a model change. It reports `Not
supported` for a route without a declaration. It autosaves each routing-default
change without a separate action. The browser rejects malformed profile data.
It does not repair that data. Public `GET /` accepts the optional
`reasoning_effort` query. JSON `POST /` and `POST /v2` accept the same optional
body field. When omitted, the saved tenant default remains authoritative. An
explicit value must match the resolved provider offering. Otherwise, the proxy
returns `400` before an upstream call.

Management startup requires every persisted routing field to be canonical and
catalog-valid and every nonempty provider default to have the tenant's saved
key. It never infers or repairs a provider, model, or reasoning effort at read
time. The bounded schema-version-3 migration performs the one-time
reconciliation of older managed defaults against saved provider keys. The
bounded schema-version-4 migration then removes retired `qwencloud` provider
settings and reconciles affected text defaults to the first remaining keyed
provider. The schema-version-5 migration converts affected provider-native
model values to canonical exact model identifiers. These migrations preserve
tenant timestamps and verify their result before recording the version.
Invalid keys, models, or routing data stop startup with
the owner, tenant, endpoint, provider, and model context.

Configured authenticated users land on Usage Overview. An independent `Usage
tenant` selector sits immediately before the ordered `ALL`, `30 days`, `7
days`, and `1 day` controls. It defaults to `All tenants`, while the interval
independently defaults to `30 days`. The account-wide selection aggregates
requests, tokens, success rate, buckets, status codes, providers, and models
across every owned tenant. Choosing one tenant narrows the same dashboard
surfaces to that tenant. `Refresh` and interval changes retain the Usage tenant
selection, and changes to the Tenant control in Settings do not affect it. Users whose
client/provider setup is incomplete enter the mandatory Settings modal instead;
after setup, the modal remains available from the avatar dropdown. The
success-rate metric renders an **N failed requests** action only when the selected
snapshot contains failures. It opens a keyboard- and focus-managed dialog with
the current non-success status breakdown and newest-first safe failure metadata.
The dialog retains the active interval, paginates within one opaque snapshot,
and discards any response made stale by an interval or Usage tenant change. An
account-wide failure row includes the owning tenant's safe ID and current
display name; a tenant-scoped row retains the tenant-less safe shape. A
details error stays inside the dialog and never replaces aggregate dashboard
data. The
`Settings` menu item is inserted before `Sign out` through the shared
`<mpr-user>` menu contract. The modal contains client access, generated secret,
routing defaults, copyable default request examples, copyable selected-provider
request examples, and one selected-provider editor for API key, provider text
model, and provider system prompt settings. The routing-default form exposes
Reasoning effort only for the exact selected text route, clears an incompatible
value when that route changes, and shows `Not supported` when the route has no
declaration. Its provider/model/effort selections autosave immediately and its
system prompt autosaves on field exit. Default examples omit `provider`;
selected-provider examples include the current provider selector and text model.

Administrators are configured only through `management.admin_emails`; use the
plural `${LLM_PROXY_MANAGEMENT_ADMIN_EMAILS}` placeholder in public config files
and define the real value as a YAML flow sequence in the runtime environment or
ignored `configs/.env`. When the validated TAuth
session email matches that list, the account response includes
`user.is_admin: true`, the shared avatar menu gets an `Admin` item, and
`GET /api/management/admin/users` returns all managed users with tenant facts
and 30-day usage summaries. Admin responses never include provider API keys,
masked provider-key strings, generated tenant secrets, secret digests, prompts,
audio names, transcripts, or model responses. Authenticated non-admin users get
`403 Forbidden` from admin-only APIs.

`GET /api/management/usage?interval=all|30d|7d|1d` returns one summary across
every tenant owned by the authenticated TAuth subject.
`GET /api/management/tenants/:tenant_id/usage?interval=all|30d|7d|1d` returns
the same summary shape for one explicitly selected owned tenant. These are
distinct canonical scopes; neither is an alias or browser-side fan-out.
`interval` is required exactly once; a missing, repeated, or unknown value
returns `400`. Both responses carry `Cache-Control: no-store` and contain
the selected `interval`, its `bucket_unit`, `totals`, ordered generic `buckets`,
and provider, model, and status-code breakdowns; the user endpoint has no
`period_days` or `daily` fields. `1d` uses 24 hourly buckets, `7d` and `30d` use
7 and 30 daily buckets, and each finite interval is an exact trailing duration
ending at one captured server timestamp. `all` includes retained tenant events
through that timestamp in UTC daily buckets from the earliest event through
today, or an empty bucket list when the selected scope has no events. Account
totals and average latency are calculated from the complete owned event set,
not from per-tenant summaries. The administrator
endpoint remains a separate fixed 30-day daily contract.

```text
GET /api/management/usage?interval=30d
GET /api/management/tenants/:tenant_id/usage?interval=30d
```

`GET /api/management/usage/failures?interval=all|30d|7d|1d`
is the account-wide failure operation. It uses one stable newest-first snapshot
across all owned tenants and adds only `tenant_id` and `tenant_name` to each safe
row. Its cursor is bound to the account-wide scope.
`GET /api/management/tenants/:tenant_id/usage/failures?interval=all|30d|7d|1d`
is the corresponding operation for one explicitly selected tenant; its rows do
not repeat tenant identity. Both operations require exactly one `interval`,
accept one optional `limit` from 1 through 100 (default 25) and one optional
opaque `cursor`, and reject missing, repeated, malformed, or unknown query
fields with `400`. Missing and foreign tenant ids both return `404`. Pages are
newest first under a stable `(created_at, id)` position and an opaque snapshot
boundary; a cursor from one tenant or account-wide scope is rejected in every
other scope. Safe failure metadata is limited to `occurred_at`, `endpoint`,
`provider`, `model`, `status_code`, `outcome_code`, and `latency_ms`, plus the
account-wide tenant fields described above. Neither operation returns row or
user ids; prompts; responses; audio; transcripts; client secrets; provider
keys; raw upstream bodies; or free-form errors. The administrator surface
remains aggregate-only and cannot fetch another owner's rows.

Usage events are recorded only for managed tenants when they call the public
proxy endpoints with a generated secret. Account-wide usage queries apply the
authenticated owner and all owned tenant ids at the database boundary;
tenant-scoped queries additionally require the explicit tenant id. Every query
uses one captured time boundary. Because proxy responses enqueue usage
asynchronously, a summary or failure query can temporarily omit accepted work
that has not committed yet. Stored usage
metadata includes endpoint, provider, model, status code, success flag, one
canonical outcome code, latency, and normalized request/response/total token
counts. Outcome codes are exactly `success`, `invalid_request`,
`payload_too_large`, `rate_limited`, `service_unavailable`, `request_timeout`,
or `upstream_error`. They are selected at the request/error boundary; prompts,
audio, transcripts, responses, tenant secrets, provider API keys, raw upstream
bodies, and free-form error text are not stored in usage events.

The runtime never imports config tenants or global provider keys.
TAuth subjects own personal tenants directly. The contract has no shared
tenant, membership, role, invitation, or team tenancy.

F014 upgrades the previous one-tenant-per-user database to schema version 1
as one bounded startup transaction:

1. Drain every old llm-proxy instance and take an operator-owned database
   backup. Never run the old and new binaries against the same database during
   this migration.
2. Exercise the exact SQLite migration on a disposable database through the
   repository `make ci` gate.
3. Start one new instance. Preflight reads all legacy tenant, provider-key, and
   usage rows before opening the mutation transaction. It rejects missing
   tables, unclaimed `static-config:` owners, blank or duplicate owners and
   tenant ids, duplicate or malformed secret digests, orphan provider or
   usage rows, plaintext or corrupt provider keys, and non-canonical routing
   data.
4. The transaction renames the two colliding legacy GORM indexes and the three
   legacy tables, creates explicit user and tenant tables, preserves every
   opaque tenant id, moves secret digests and routing data, rebinds encrypted
   provider keys from the prior user id to the tenant id, copies usage rows,
   verifies counts and values including decryption, writes schema version 1,
   and removes the bounded legacy tables.
5. Verify account, tenant, provider, secret, routing, and usage behavior
   before adding capacity. A failed stage rolls the transaction back to the
   untouched legacy schema and prevents startup. Correct the source data or
   restore the backup; do not hand-edit a partially migrated shape.

The subsequent bounded schema-version-2 migration preflights every schema-1
usage row, maps successful rows to `success`, and maps historical `400`, `413`,
`429`, `499`, `502`, `503`, and `504` statuses to their exact canonical failure
codes. Caller cancellation `499` and proxy-budget expiry `504` both become
`request_timeout`. It rejects any other historical failure status before
mutation, then writes the non-null outcome field and the
tenant/success/time/id failure-page index in one transaction. Historical
diagnostics therefore contain normalized, status-derived codes, never
reconstructed raw error messages.

The bounded schema-version-3 migration preflights every managed provider key
and canonical routing pair. For each tenant it preserves currently eligible
defaults, otherwise selects the first keyed text provider and first keyed
dictation-capable provider by canonical provider id, and clears the corresponding
pair when no provider is eligible. It writes the reconciled defaults without
changing tenant timestamps, verifies every row against its decrypted saved
keys, and records schema version 3 in the same transaction. Reopening a
version-3 database validates this invariant and rejects drift instead of
repairing it at read time.

The bounded schema-version-4 migration removes every stored `qwencloud` key,
selected model, and provider system prompt. Affected text defaults move to the
first remaining keyed provider by canonical identifier. The default uses that
provider's stored text model. If no key remains, the migration clears the text
route and reasoning effort. Settings becomes mandatory. Tenant timestamps and
historical usage provider/model identifiers remain unchanged. The transaction
verifies deleted settings, reconciled defaults, decrypted remaining keys,
timestamps, and usage rows before recording version 4. Current-version startup
rejects retired provider settings or routing defaults instead of repairing them.

The bounded schema-version-5 migration converts stored provider-native model
values to canonical exact model identifiers. It updates affected provider
settings and tenant defaults in one transaction. It preserves tenant timestamps
and all historical usage records. Current-version startup rejects an invalid
or dangling canonical route.

The bounded schema-version-6 migration replaces the retired `grok` provider
identity with `xai`. The bounded schema-version-7 migration adds the provider
base URL field to managed provider settings. Existing DashScope records do not
contain a workspace URL. The migration removes those incomplete records and
reconciles affected text defaults. It preserves tenant timestamps and
historical usage. The owner must then save the complete DashScope key and URL
pair. The bounded schema-version-8 migration replaces the retired `zhipu`
provider identity with `zai`. It preflights all provider settings and defaults,
decrypts each affected key with `zhipu` associated data, and re-encrypts it with
  `zai` associated data. The transaction updates keys and current routing
defaults, preserves tenant timestamps and historical usage, and rejects
conflicting, corrupt, or noncanonical stored values before mutation.

The bounded schema-version-9 migration replaces each provider-key row with
provider connection records and one provider profile record. It maps every
predecessor value through the current provider definition. It re-encrypts the
credential with the provider field identity as associated data. The transaction
verifies every value and timestamp before it removes the predecessor table.
Before current-catalog validation, startup atomically replaces each predecessor
model that a schema-version-10 catalog record identifies. It also replaces the
matching tenant default.
Current-schema reads use only provider connection records and provider profile
records.

The bounded schema-version-10 migration replaces each stored Gemini 2.5 text
model and same-provider tenant default with `gemini-3.5-flash`. It updates the
provider profile and current default in one transaction. It preserves tenant
and profile timestamps and all historical usage model identifiers. Current
startup rejects retired Gemini routes instead of accepting an alias or fallback.

The bounded schema-version-11 migration replaces stored
`gemini-3.1-flash-lite` and `gemini-3.1-pro-preview` selections with
`gemini-3.5-flash`. It updates the provider profile and same-provider tenant
default in one transaction. It preserves provider connections, system prompts,
timestamps, and historical usage model identifiers.

Server settings and browser-facing MPR UI/TAuth bootstrap settings remain in
`config.yml`. Provider definitions and static endpoints remain in
`providers.yml`. Each managed DashScope workspace URL is tenant-owned and is
stored as a provider connection value. The selected model and system prompt use
a provider profile record. The GitHub Pages artifact is only the static shell.
API-served browser config
endpoints are projections of backend `config.yml`, not independent
configuration sources.

### Hosted split-origin setup

Production is split-origin:

| Hostname | Owner | Purpose |
|----------|-------|---------|
| `llm-proxy.mprlab.com` | GitHub Pages | Public landing at `/`; noindex self-service app at `/app/`. |
| `llm-proxy-api.mprlab.com` | MPR gateway/backend | llm-proxy API, management API, `/`, `/v2`, and `/dictate`. |
| `tauth-api.mprlab.com` | TAuth backend | Google login, nonce, logout, `/auth/session`, and session-cookie issuance. |

Add these DNS records:

1. `CNAME llm-proxy.mprlab.com -> tyemirov.github.io`
2. Point `llm-proxy-api.mprlab.com` at the MPR gateway public endpoint. Use a
   `CNAME` when the gateway has a hostname, or `A`/`AAAA` records when it is
   addressed by public IP.

Then configure GitHub Pages for this repository:

1. Use branch publishing from `gh-pages` at `/`.
2. Set the Pages custom domain to `llm-proxy.mprlab.com`.
3. Use the standard `make release`, `make publish`, and `make deploy` lifecycle.
   Those commands delegate to `../mprlab-gateway`, which renders the declared
   frontend-built Pages container, publishes its immutable artifact, activates it on
   `gh-pages`, and verifies the matching Pages build and cache-distinct
   `/.mprlab-release.json` marker.
4. Configure real backend deployment secrets outside the Pages artifact:
   `LLM_PROXY_MANAGEMENT_ADMIN_EMAILS`, `LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY`,
   `LLM_PROXY_MANAGEMENT_DATABASE_PATH`,
   and `LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY`.
5. Do not store browser runtime config in the Pages branch. Production browser
   config is served only by `https://llm-proxy-api.mprlab.com/config-ui.yaml`
   from the running backend's loaded management config. The declared Pages
   container writes that canonical URL into `mpr-header[data-config-url]` and
   validates the declarative mpr-ui bundle marker.

Configure TAuth for tenant `llm-proxy` with:

- allowed tenant origin `https://llm-proxy.mprlab.com`
- browser-facing API origin `https://tauth-api.mprlab.com`
- session cookie name matching `management.session_cookie_name`
- cookie domain `.mprlab.com`
- HTTPS-only cookies
- JWT signing key matching `management.jwt_signing_key`

The sibling `mprlab-gateway` Ansible orchestrator treats this declaration as one
runtime contract: it resolves the `tauth.tenants` capability, stages TAuth and
llm-proxy inputs, reconciles changed services, and verifies the declared public
health checks before Pages activation. This prevents a newly deployed backend
from validating sessions against stale TAuth cookie or signing configuration.

The same boundary is executable locally without Google OAuth or deployed
services:

```bash
make test-management-auth-blackbox
```

The target builds the TAuth version pinned in `go.mod` and the current
llm-proxy binary, starts both on disposable local ports, and opens the real
static management app in Playwright. A Google Identity test adapter activates
the visible MPR UI control and maps its provider exchange to TAuth's seeded
password fixture via the same-origin frontend proxy. TAuth issues the configured
HttpOnly access and refresh cookies, and MPR UI drives the authenticated
lifecycle and documented redirect without an application-owned auth script or
manual auth event. The test proves the
public **Log In** control is owned by MPR UI, proves an anonymous direct
`/app/` visit returns to `/`, and proves the anonymous/authorized behavior of
`/api/management/account`. The browser makes no protected account or tenant
request before MPR UI authentication, restores the real TAuth session on
`/app/`, then hydrates the initial tenant selected in Settings and account-wide
Usage view. It
creates two tenants for one real TAuth subject, proves both secrets remain
independently routable, proves the default account-wide usage and safe
tenant-attributed failure page include both, and signs in a second real subject
to prove foreign tenant ids return `404` without disclosure. It waits for the
`mpr-ui@latest` shell plus the
dashboard to report the authenticated state, then proves an ordinary reload
stays authenticated, removes only the access cookie and proves `/auth/session`
recovers it from the refresh cookie without returning to the public page, and
uses the visible **Sign out** action to prove `/auth/logout` clears both cookies
and returns TAuth plus the management API to anonymous responses. The MPR UI
application assets are loaded through their literal `@latest` CDN contract;
MPR UI's shell and lifecycle, TAuth's session/refresh/logout routes, and the
management API routes are exercised as real boundaries. Only the external
Google provider exchange is replaced by the local seeded-credential adapter.

Normal navigation, page refreshes, and access-cookie expiration do not sign the
user out. MPR UI silently restores the TAuth session while its rotating refresh
cookie remains valid and reports the resulting lifecycle. Only the explicit
**Sign out** action asks MPR UI to clear the browser session; LLM Proxy does not
own a second session store or an automatic logout path.

Configure the gateway/backend route for `llm-proxy-api.mprlab.com` to the
llm-proxy service, and remove any backend route that still claims
`llm-proxy.mprlab.com`; that hostname is now owned by GitHub Pages. The backend
must run with `management.public_origin: "https://llm-proxy.mprlab.com"` so
`/config-ui.yaml` and `/api/management/*` return credentialed CORS headers only
to the static frontend.

Web search is per request and currently supported only on OpenAI models that
support the OpenAI web search tool.
Text output length is per upstream attempt: pass `max_tokens` to set the
initial attempt's budget, which is reused for missing-suffix attempts. If an
output-budget stop contains no visible progress and the model has a configured
output limit, the coordinator increases the next attempt's budget toward that
limit. When omitted, the proxy does not send a provider max-token field, except
Anthropic Messages where `max_tokens` is required upstream and the proxy sends
the selected model's configured output limit.
Provider-specific output-token limits are enforced at the request edge when
known. MiniMax M2 routes reject `max_tokens` above `204800`. Gemini text models
currently reject values above `65536`. Claude models reject values above the
configured synchronous Messages output limit. Those errors return `400 Bad
Request` before any upstream provider call.

## Running

Generate a secret:

```shell
openssl rand -hex 32
```

Run the canonical local browser stack:

```shell
make up
```

Stop that local browser stack from another terminal:

```shell
make down
```

Before the first run, explicitly create the ignored private
`configs/.env.local`, populate it with real local values, and set mode `0600`.
The tracked `configs/.env.local.example` file documents local fields.
The tracked `configs/.env.sample` file documents direct runtime fields.
Both files contain deliberately unrealistic values. Never copy or source them
as runtime configuration. `make up` fails before contacting Docker when the
private file is absent. When the real file uses the explicit
`__GENERATE_ON_FIRST_MAKE_UP__` marker for its local TAuth signing key or
provider-key encryption key, `make up` generates that value once. It then
writes ignored, service-scoped environment projections for ghttp, llm-proxy,
and TAuth.

ghttp receives only its `GHTTP_*` inputs. TAuth receives only its server and
tenant inputs, including the signing key it shares with the API. Only llm-proxy
receives the provider-key encryption configuration; aggregate dotenv files and
live provider smoke-test credentials are not injected into auxiliary
containers. The API image is built from the current source and runs the
canonical `configs/config.yml` configuration.

Local and production orchestration do not bind a DashScope URL. Each tenant
supplies its Singapore Model Studio workspace URL with its DashScope API key in
Settings.
The stack has these explicit browser-facing endpoints:

- Public landing: `http://localhost:4179/`, served by ghttp from the rendered
  local site artifact.
- Management UI: `http://localhost:4179/app/`, served from the same static artifact.
- API reference: `http://localhost:4179/docs/`, generated from the canonical
  schema with explicit raw-view and exact-YAML download actions.
- OpenAPI schema: `http://localhost:4179/openapi.yaml`, served through the
  frontend from the canonical read-only `docs/openapi.yaml` mount.
- Backend API: `http://localhost:8080/`, including the proxy and
  `/api/management/*` endpoints.

`make up` starts the API, verifies its public capability resource, and runs a
one-shot frontend site-builder. That builder fetches
`/api/public/capabilities`, renders the validated provider registry into the
public routing tree and capability matrix, and writes an isolated static
artifact for ghttp to serve read-only. Startup rejects a failed API resource or
site build, and shutdown removes the temporary artifact.

ghttp proxies `http://localhost:4179/config-ui.yaml` to the API and the
same-origin `/auth/*` and `/me` routes to the internal TAuth service. The
browser receives `http://localhost:4179` as its TAuth origin and the direct API
origin from that one runtime configuration. Production keeps its explicit
split-origin topology; local authentication stays on the front door so another
host process cannot intercept a TAuth port through a different `localhost`
address family. Use the `localhost` UI URL rather than `127.0.0.1`: TAuth's
insecure local HTTP cookie profile is intentionally scoped to the single
`localhost` host. The local ghttp front door sends `Cache-Control: no-store` so
an ordinary reload reads one current set of mounted HTML, CSS, and ES modules
instead of combining files cached from different working-tree states.

Compose first completes image pulls/builds and reports all four services
running through `docker compose up --wait`; only then does the bounded HTTP
readiness budget begin. Readiness proves static content (`200`), the canonical
OpenAPI mount (`200`), the ghttp-served runtime config (`200`), the
unauthenticated API boundary (`403`), the same-origin TAuth session (`204`) and
nonce (`200`) boundaries, and the unauthenticated management API boundary
(`401`). It does not call a paid provider. After readiness, Compose logs remain
attached in the foreground. Use `Ctrl-C` there or run `make down` from another
terminal to stop the same local containers, project network, and orphaned
services. Both shutdown paths retain the named local data volumes so local
TAuth and management state remain available for the next run.

Browser startup additionally loads the pinned Alpine 3.13.5 module from
`https://cdn.jsdelivr.net`. `make up` cannot override a Chrome extension,
privacy filter, or browser policy that blocks that client-side request. If the
page reports **Unable to open LLM Proxy**, allow `cdn.jsdelivr.net` for
`http://localhost:4179` in the blocking browser control and select **Reload LLM
Proxy**. The same failure screen replaces an incoherent or rejected first-party
module graph. LLM Proxy does not try another CDN or a bundled fallback; the
failure screen completes the shared MPR transition without making a protected
management request.

Use **Settings** in `/app/` to save each provider API key and select the
Default tenant's text and dictation routes. The same settings store the
provider-specific default model. Supported routes can also save a
`reasoning_effort` default. A supported per-request value overrides that saved
default for one request. The proxy rejects a default that the selected
provider/model route does not declare.

Create or replace the tenant's client key separately in **Settings**. That
generated client key is the value that applications store as
`LLM_PROXY_SECRET`. It is not an upstream provider credential.

## Local Automation

This repository exposes the standard local targets used by MPR app repos:

| Command | Purpose |
|---------|---------|
| `make frontend-dependencies` | Install the pinned npm graph and Chromium into ignored project-local state. Focused frontend validation, `make lint`, `make test`, and `make ci` invoke this target automatically. |
| `make up` | Require the ignored private `configs/.env.local`, then build and run the complete local browser orchestration: ghttp static UI and same-origin TAuth routes on `localhost:4179`, plus the API on `localhost:8080`. It waits for Compose startup before verifying the static/config/auth/API boundaries and reporting ready. |
| `make down` | Stop the exact local Compose project started by `make up`, including orphaned services and its project network, while retaining the named local TAuth and management data volumes. |
| `make ci` | Prepare pinned frontend dependencies, then run format checks, Go lint (`go vet`, `staticcheck`, `ineffassign`), Python strict mypy, frontend syntax checks, the 100% coverage-gated Go test suite, Python pytest, Playwright browser tests, the app lifecycle contract test, and the non-paid live-harness preflight. A successful run ends with a per-gate table, current-run coverage, and an explicit `CI PASSED` receipt. |
| `make test-live-provider-harness` | Save a temporary managed OpenAI connection and route one request through a loopback provider. |
| `make test-live-providers` | Start a disposable managed tenant, verify every available provider key through the canonical management operation, and run that provider's live text smoke only after verification succeeds; use `LIVE_ENV_FILE=/path/to/env` to load key values. |
| `make test-live-provider-media` | Verify OpenAI, Anthropic, Gemini, Moonshot, and xAI keys, then send one paid canonical image request through each provider. |
| `make test-live-gemini` | Run direct acceptance for the exact Gemini 3.6 Flash and Gemini 3.7 Flash candidates, then run registered Gemini routes through `make test-live-providers`. |
| `make test-live-local-providers` | Build an isolated local Compose project, verify selected provider keys, and send paid `POST /v2` requests through its Dockerized API. |
| `make test-live-local-gemini` | Run every registered Gemini model and reasoning level through the isolated local Compose API. |
| `make live-test` | Send paid production `POST /v2` requests through the Default tenant using only `LLM_PROXY_SECRET`: echo checks for OpenAI, Anthropic, Meta, Gemini, and Moonshot, plus large completion cases for OpenAI, Anthropic, Meta, and Gemini. |
| `make release` | Delegate the clean checkout to the exact sibling `../mprlab-gateway`. The transaction validates its exact prepared or reused decision before CI. |
| `make publish` | Delegate publication of the exact sealed release to `../mprlab-gateway`; it does not rebuild or deploy. |
| `make deploy` | Delegate convergence of only this app's declared runtime, route, health, Pages, and TAuth resources to `../mprlab-gateway`. |

Live provider smoke tests are intentionally not part of `make ci`; they call
paid upstream APIs and depend on local or CI secret availability. The dynamic
target clears catalog provider fields before it loads `LIVE_ENV_FILE`. The
file is the only provider-value source when it is set. The target discovers
these provider keys after loading the file. It verifies
each key against the provider's configured default model, or the exact model
override below, before making that provider's smoke request. By default, the
subsequent smoke omits `model` and proves that the newly saved managed provider
default is operational; an override is included in both verification and smoke.
Set `LLM_PROXY_LIVE_ALL_MODELS=true` to discover every text model for each
selected provider. The harness uses the validated public catalog and runs one
verification and one explicit-model smoke request for each model.

The local Compose live test uses the current `docker-compose.local.yml` file.
It builds the API image from the current checkout. Each project name starts
with `llm-proxy-live-test-` and has a unique random suffix. The suffix prevents
reuse of resources from an earlier run. The test uses allocated loopback ports
and temporary management and TAuth volumes. The command verifies all local
readiness boundaries before it saves a provider
connection. Each text smoke uses canonical `POST /v2` through
the allocated API origin. The harness does not start a host proxy process for
this mode. Cleanup removes the test containers, network, volumes, and temporary
site files after success or failure. The command stops if `make up` is active.

Run selected providers through the isolated local Compose API:

```shell
LLM_PROXY_LIVE_PROVIDERS=openai,gemini \
  make test-live-local-providers LIVE_ENV_FILE=/path/to/provider.env
```

Run every registered Gemini text model and reasoning level through local
Compose:

```shell
make test-live-local-gemini LIVE_ENV_FILE=/path/to/provider.env
```

The Gemini wrapper first sends direct Interactions requests for the exact
`gemini-3.6-flash` and `gemini-3.7-flash` candidates. It sends one request with
the thinking level omitted and one request for every thinking level supported
by the exact model. It then proves active retrieval, completion, cancellation,
and deletion for stored background interactions. Google documents both exact
models as stable Interactions models and documents the background resource
lifecycle in its
[Gemini 3.6 Flash](https://ai.google.dev/gemini-api/docs/models/gemini-3.6-flash),
[Gemini 3.7 Flash](https://ai.google.dev/gemini-api/docs/models/gemini-3.7-flash),
and [background execution](https://ai.google.dev/gemini-api/docs/background-execution)
guides. These candidates remain outside the public provider catalog until the
complete lifecycle succeeds. Set
`LLM_PROXY_LIVE_GEMINI_CANDIDATES=false` only when a run must exercise the
registered Gemini routes without candidate acceptance.

`make ci` runs each declared gate sequentially through one top-level runner.
Coverage is written to a fresh private artifact for that invocation and
verified again after the final test gate. If orchestration exits before the
terminal receipt, the command returns nonzero and identifies the active stage;
an ignored coverage artifact from an earlier run cannot satisfy completion.

| Provider | Key variable | Model override |
|----------|--------------|----------------|
| OpenAI | `OPENAI_API_KEY` | `LLM_PROXY_LIVE_OPENAI_MODEL` |
| Meta Muse Spark | `MODEL_API_KEY` | `LLM_PROXY_LIVE_META_MODEL` |
| DeepSeek | `DEEPSEEK_API_KEY` | `LLM_PROXY_LIVE_DEEPSEEK_MODEL` |
| DashScope/Qwen | `DASHSCOPE_API_KEY` | `LLM_PROXY_LIVE_DASHSCOPE_MODEL` |
| Moonshot/Kimi | `MOONSHOT_API_KEY` | `LLM_PROXY_LIVE_MOONSHOT_MODEL` |
| MiniMax | `MINIMAX_API_KEY` | `LLM_PROXY_LIVE_MINIMAX_MODEL` |
| SiliconFlow | `SILICONFLOW_API_KEY` | `LLM_PROXY_LIVE_SILICONFLOW_MODEL` |
| Z.AI | `ZAI_API_KEY` | `LLM_PROXY_LIVE_ZAI_MODEL` |
| Gemini | `GEMINI_API_KEY` | `LLM_PROXY_LIVE_GEMINI_MODEL` |
| Anthropic/Claude | `ANTHROPIC_API_KEY` | `LLM_PROXY_LIVE_ANTHROPIC_MODEL` |
| xAI/Grok | `XAI_API_KEY` | `LLM_PROXY_LIVE_XAI_MODEL` |

Run every provider with an available key:

```shell
make test-live-providers LIVE_ENV_FILE=configs/.env
```

Run only selected providers. When `LLM_PROXY_LIVE_PROVIDERS` is set, every
listed provider must have its key:

```shell
LLM_PROXY_LIVE_PROVIDERS=openai,gemini \
  make test-live-providers LIVE_ENV_FILE=configs/.env
```

Run the complete current Qwen model matrix through the saved Singapore
workspace URL:

```shell
LLM_PROXY_LIVE_PROVIDERS=dashscope \
  LLM_PROXY_LIVE_ALL_MODELS=true \
  make test-live-providers LIVE_ENV_FILE=configs/.env
```

Run the complete MiniMax M2 model matrix:

```shell
LLM_PROXY_LIVE_PROVIDERS=minimax \
  LLM_PROXY_LIVE_ALL_MODELS=true \
  make test-live-providers LIVE_ENV_FILE=configs/.env
```

The live harness parses `LIVE_ENV_FILE` as dotenv data without executing it as
shell code. A paid run creates a disposable management database, encryption
key, signed local session, tenant, and client secret under its private temporary
directory. It submits each candidate once to
`PUT /api/management/tenants/:tenant_id/provider-connections/:provider`, requires the
safe `200` tenant-profile result, and only then sends that provider's smoke
request. Candidate payloads, session material, provider responses, and proxy
responses are never printed, and the temporary state is removed at exit.

The paid image matrix uses OpenAI, Anthropic, Gemini, Moonshot, and xAI by
default. It requires all five provider keys. Set `LLM_PROXY_LIVE_PROVIDERS` to
run a selected subset. The harness selects each image model from the validated
public provider catalog. It uses the catalog default when that model supports
image input. Otherwise, it requires one exact image model for that provider.
The key verification uses the selected image model.

Set `LLM_PROXY_LIVE_ALL_MODELS=true` with media mode to verify and call every
image route for each selected provider. This command runs the complete current
Moonshot image matrix:

```shell
LLM_PROXY_LIVE_PROVIDERS=moonshot \
  LLM_PROXY_LIVE_ALL_MODELS=true \
  make test-live-provider-media LIVE_ENV_FILE=configs/.env
```

Each image case creates the same deterministic 256 by 256 red PNG. The request
sends canonical padded base64 data through `POST /v2`.
The case requires HTTP `200`, an exact `RED` response, and a valid proxy request
identifier. The harness does not print the image, response body, or credentials.

Run the complete paid image matrix:

```shell
make test-live-provider-media LIVE_ENV_FILE=configs/.env
```

Run a selected paid image subset:

```shell
LLM_PROXY_LIVE_PROVIDERS=openai,gemini \
  make test-live-provider-media LIVE_ENV_FILE=configs/.env
```

The non-paid `--preflight` mode creates an isolated managed user, tenant, and
client key. It saves a generated OpenAI key through the management API. It then
reloads the encrypted provider connection and routes one prompt through a loopback
Responses server. The `--write-config` mode writes the managed-only service
configuration without building or starting the proxy. Inspect that config with
`./scripts/test_live_providers.sh --write-config
/tmp/llm-proxy-live.yml`. Unless `LLM_PROXY_LIVE_PORT` explicitly selects a
port, each harness run allocates a fresh loopback port. Cleanup removes only the
temporary proxy and provider children that it starts.

### Production Default-tenant live test

`make live-test` is a separate paid production check. It calls only
`https://llm-proxy-api.mprlab.com` and requires exactly one local credential:
`LLM_PROXY_SECRET`, the Default tenant's generated client secret. It neither
loads a dotenv file nor reads, accepts, or sends a local upstream-provider key.
The saved provider credentials and per-provider default models remain entirely
on the production tenant.

Before any paid provider call, the command sends an intentionally incomplete
`POST /v2` request with `LLM_PROXY_SECRET`. A valid client key must reach normal
request validation and return `400` with a proxy request ID. A missing,
rejected, or unreachable client key stops the command before the provider
matrix starts.

The command then sends canonical `POST /v2` requests with an explicit provider.
Its echo cases omit `model`, so they exercise each saved Default-tenant provider
model. It runs echo markers for OpenAI, Anthropic, Meta, Gemini, and Moonshot.
It sends the same deterministic request larger than 16 KiB through OpenAI,
Anthropic, Meta, and Gemini. The long Gemini case explicitly selects
`gemini-3.5-flash`. This selection proves the default background Interactions
path. The long request requires normalized output for every portfolio record before its
final marker and uses a 900-second request budget. OpenAI and Gemini 3.5 keep
the blocking caller request open while their resource adapters own background
polling. Anthropic and Meta use their canonical
synchronous completion paths, including shared output-continuation work when
needed. The test client never polls a provider or llm-proxy itself.
Each case verifies HTTP `200`, the echoed request budget, a validated proxy
request ID, and a completion marker. Its result line prints that request ID for
log correlation without printing the response body or tenant secret. A
transport failure with no response prints no request ID. The command runs all
nine cases before returning nonzero for any failed case.

Set the Default-tenant client secret and its exact tenant identifier before
you invoke the target:

```shell
export LLM_PROXY_SECRET='...'
export LLM_PROXY_EXPECTED_TENANT_ID='managed-...'
make live-test
```

The target first calls authenticated `GET /v2/identity`. The server returns
the tenant identifier that it resolved from the client secret. The target
compares that value to `LLM_PROXY_EXPECTED_TENANT_ID`. It stops before any paid
provider request if authentication fails or the tenant does not match. It does
not store the client secret in a temporary file.

This target is intentionally outside `make ci`: it has a real production cost
and is expected to fail honestly for a disabled, rate-limited, or failing
provider.

The complete production lifecycle is:

```shell
make release
make publish
make deploy
```

Each target resolves this checkout and the exact sibling `../mprlab-gateway`,
then invokes the corresponding gateway transaction with this checkout as the
selected app. The gateway runs the release gate and seals immutable artifacts;
publish and deploy consume that exact release without rerunning CI or
rebuilding. Retries reuse exact state and reject conflicting immutable state.
The selected app transaction reads no unrelated app repository.

This repository owns one production declaration:
`.mprlab/deploy/resources.yml`. It declares the container image and service,
retained data volume, `llm-proxy.http` capability, Caddy route, public health
checks, GitHub Pages artifact, TAuth tenant, and private-value bindings. The
ignored `.mprlab/deploy/.env` file is the canonical private deployment input.
The gateway reads it only during deployment and generates one mode-`0600`
service environment. TAuth, Pages, Caddy, and capability resources supply
their owned outputs without duplicate values in the private file or service
declaration.

The gateway owns schema validation, resource-output resolution, artifact
sealing and publication, Ansible reconciliation, inventory, and production
verification. Application private values do not use gateway inventory secret
maps. There is no application-owned production Ansible, Compose, Caddy,
release, publish, or deploy implementation.

The runtime declaration identifies the exact legacy
`mprlab-nginx-gateway/llm-proxy` Compose service. During the first gateway-managed
deployment, the gateway verifies that service belongs to llm-proxy before
removing only its old container. The retained
`mprlab-nginx-gateway_llm-proxy-data` volume is not removed.

The Pages declaration uses `docker/pages/Dockerfile`. A compiled Go backend
serves the secret-free `/api/public/capabilities` resource during the build.
The Node frontend renderer fetches that resource. It renders model families,
exact models, provider offerings, and route filters in the route explorer. It
keeps publishers in the capability matrix. It also renders
request limits and injects the browser configuration URL into
every generated auth-aware HTML page. The final Pages image contains only the
static artifact. Application runtime code has no Caddy deployment knowledge, and
its TAuth knowledge remains limited to the published client/session
integration.

## Usage

### Client authentication and configuration boundary

The installable `llm-proxy-client` command does not discover a user-level or
system-level YAML configuration file. It accepts `--base-url` and `--secret`,
with `LLM_PROXY_BASE_URL` and `LLM_PROXY_SECRET` as their environment
counterparts; the Go and Python libraries accept the same values through
application-supplied configuration. The only optional file-based client
configuration input is an application-owned JSON model profile for per-user
provider/model selection. Service `config.yml` remains server-side operator
configuration and is never loaded by a bundled client.

Public proxy calls authenticate with the tenant secret in `key=...`. The MPR
UI/TAuth session instead authorizes management actions such as
creating a client key or saving a provider key. It does not authenticate a
direct `POST /v2` request. Upstream provider API keys stay only in
authenticated tenant management storage. A client must never send them.

For an end-to-end first request and the boundary between these credentials, see
the [client authentication guide](https://llm-proxy.mprlab.com/resources/llm-proxy-client-authentication/).

### Installable prompt client

Install the reusable JSON POST client:

```shell
go install github.com/tyemirov/llm-proxy/llm-proxy-client@latest
```

Use it with explicit flags:

```shell
llm-proxy-client \
  --base-url "http://localhost:8080/?provider=gemini" \
  --secret "$LLM_PROXY_SECRET" \
  --prompt "Summarize this"
```

Or read configuration and prompt text from environment/stdin:

```shell
export LLM_PROXY_BASE_URL="http://localhost:8080/"
export LLM_PROXY_SECRET="..."
printf 'large prompt...\n' | llm-proxy-client --max-tokens 4096
```

For a route whose catalog declares the value, override the tenant default for
one request with `--reasoning-effort`:

```shell
llm-proxy-client \
  --base-url "http://localhost:8080/?provider=openai" \
  --secret "$LLM_PROXY_SECRET" \
  --model gpt-5.5 \
  --reasoning-effort high \
  --request-timeout-seconds 900 \
  --prompt "Summarize this"
```

The client always uses canonical `POST /v2?key=...` with a JSON body. It keeps
non-payload query parameters such as `provider`, strips body-owned query fields
such as `prompt` and `model`, and sends the prompt as a v2 `user` message.
`--system-prompt` becomes a v2 `system` message. Optional `model`,
`web_search`, `max_tokens`, and `reasoning_effort` values remain body fields.
When `--model` is omitted, the body omits `model` so llm-proxy uses the selected
provider's configured default model. `--request-timeout-seconds` is instead
serialized as `X-LLM-Proxy-Request-Timeout-Seconds`; omitting the flag omits the
header and selects the server default. The obsolete `--timeout` flag is not an
alias and is rejected.

The reusable Go package under `pkg/llmproxyclient` is v2-only: construct a
`MessagesRequest` with `NewMessagesRequest` and send it with
`Client.PostMessages`. `MessagesRequestInput.ReasoningEffort` is an optional
nonblank request override; the proxy validates it against the exact resolved
provider/model capability before it calls upstream.

Use `Client.GetPublicCapabilities` to read provider offerings and media limits.
The client owns the public resource path, response boundary, and typed offering
contract. Applications must not reproduce that HTTP operation.

Set `MessagesRequestInput.RequestTimeoutSeconds` when one request needs a
specific proxy work budget:

```go
requestTimeoutSeconds := 900
request, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{
    Messages: []llmproxyclient.MessageInput{
        {Role: "user", Content: "Summarize this"},
    },
    RequestTimeoutSeconds: &requestTimeoutSeconds,
})
if err != nil {
    return err
}
text, err := client.PostMessages(ctx, request)
```

For a route whose model catalog declares image input, construct media through
the official client and attach it to a user message:

```go
frameBytes, err := os.ReadFile("frame.png")
if err != nil {
    return err
}
frame, err := llmproxyclient.NewImageAttachment(llmproxyclient.ImageAttachmentInput{
    MIMEType: "image/png",
    Data:     frameBytes,
})
if err != nil {
    return err
}
request, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{
    Messages: []llmproxyclient.MessageInput{{
        Role:        "user",
        Content:     "Inspect this exact frame.",
        Attachments: []llmproxyclient.MessageAttachment{frame},
    }},
    Model: "gemini-3.5-flash",
})
if err != nil {
    return err
}
text, err := client.PostMessages(ctx, request)
```

`NewImageAttachment` accepts `image/jpeg`, `image/png`, or `image/webp`.
`NewAudioAttachment` accepts `audio/m4a`, `audio/mpeg`, or `audio/wav`.
Both constructors copy the supplied nonempty bytes and serialize canonical
base64. Attachment values cannot be constructed directly. The proxy decodes
the bytes at the HTTP edge and preserves attachment order. It rejects media on
non-user messages or unsupported model routes before upstream work. It never
echoes media bytes in response metadata.

Use `UploadAsset` when the application needs a reusable tenant asset. The
returned record contains the opaque asset id, MIME type, byte count, state,
and expiry. Construct the attachment from that exact record:

```go
asset, err := client.UploadAsset(ctx, llmproxyclient.AssetUploadInput{
    MIMEType: "image/png",
    Data:     frameBytes,
})
if err != nil {
    return err
}
frame, err := llmproxyclient.NewImageAssetAttachment(
    llmproxyclient.ImageAssetAttachmentInput{
        AssetID:  asset.AssetID,
        MIMEType: asset.MIMEType,
    },
)
```

`NewAudioAssetAttachment` provides the corresponding audio constructor. Both
asset constructors serialize `asset_id` instead of `data` and preserve the
same attachment union.

The request value controls only the proxy budget header. `ctx` remains the Go
caller's independent cancellation authority, and the injected `HTTPDoer` may
have its own explicitly selected transport policy. The package does not add a
total-response timeout.

Every completed non-2xx response returns an `*llmproxyclient.HTTPFailure`.
Use `errors.As` to read its `StatusCode()` and recognized stable
`ProxyErrorCode()` values; `errors.Is(err,
llmproxyclient.ErrClientHTTPFailure)` also remains true. Raw response bodies
are never included in or exposed by the returned error. Transport and response
read failures preserve `ErrClientHTTPFailure` without fabricating a completed
HTTP status.

#### Update from the prior media contract

This media contract change requires one coordinated server and client update.
The prior server rejects current media clients. The current server rejects prior
media clients.

For text-only applications, this media change does not require a source change.
Inline media helpers keep their function signatures, but their serialized form
changes. Complete these steps before you resume media traffic:

1. Stop media requests to the prior server.
2. Update each Go application to `llmproxyclient@latest`.
3. Remove `SHA256` from `ImageAssetAttachmentInput` and
   `AudioAssetAttachmentInput` values.
4. Stop reading `Asset.SHA256` from asset upload results.
5. Update each Python application to this repository release.
6. Remove the third argument from `image_asset_attachment` and
   `audio_asset_attachment` calls.
7. Stop reading `ClientAsset.sha256` from asset upload results.
8. For direct HTTP integrations, remove `sha256` from each attachment.
9. Remove `X-LLM-Proxy-Asset-SHA256` from each direct asset upload.
10. Remove `sha256` from each expected asset response.
11. Update fake server fixtures to use the exact current request and response
    shapes.
12. Run each application's fake server tests against the updated official
    client.
13. Stop the prior server.
14. Move root-level `ast_*.json` and `ast_*.data` files from
    `server.asset_store_path` to a private backup directory.
15. Keep the `structured-requests` directory unchanged.
16. Start the current server and the updated applications.
17. Upload retained source media again and replace each stored prior `asset_id`.
18. Resume media traffic after the server and all applications pass acceptance
    checks.

Do not deploy a current media client against the prior server. Do not deploy
the current server while a prior media client remains active. Do not add a
dual-shape client or server bridge.

Applications that use `GetPublicCapabilities` must update their fake responses.
Each response must contain nonempty offerings with `wire_contract`,
`execution_lifecycle`, and all required media limit records. The client returns
`ErrClientHTTPFailure` for an incomplete successful response.

To upgrade the Go package and CLI:

```shell
go get github.com/tyemirov/llm-proxy/pkg/llmproxyclient@latest
go install github.com/tyemirov/llm-proxy/llm-proxy-client@latest
```

Remove `ConfigInput.Timeout` from existing Go integrations and move the desired
proxy budget to `MessagesRequestInput.RequestTimeoutSeconds`. Use a caller
context only when the application intentionally needs an independent,
potentially shorter cancellation deadline.

#### Model selection without application redeployment

Every bundled client deliberately leaves `model` out of a request when the
caller does not set it. This is the correct integration when LLM Proxy owns
model selection: a managed-tenant owner can change that tenant's routing
default in the LLM Proxy Settings UI, and the next model-omitting request uses
the saved default without an application code or deployment change. An explicit
`--model` or request `model` pins that one request and does not follow a tenant
default.

Changing an offering's `default_operations` in service configuration affects
only requests that resolve through that provider default. It does not rewrite
a managed tenant's saved routing default or saved provider setting.

#### Application-user model profiles

For application-owned, per-user selection, configure one client instance with
that user's JSON model-profile path. The document contains exactly these two
nonblank string fields and never contains credentials or TAuth material:

```json
{
  "provider": "gemini",
  "model": "gemini-3.5-flash"
}
```

The client reads this file for every outbound v2 request. An application can
write a replacement in the same filesystem and atomically rename it onto the
user's profile path; the next request from the existing client instance then
uses the new provider/model pair. The application continues to own the user
identity, authorization, storage, and atomic publication of that file.

Use the profile directly from the installable CLI:

```shell
llm-proxy-client \
  --base-url "http://localhost:8080/" \
  --secret "$LLM_PROXY_SECRET" \
  --model-profile "/var/lib/my-app/users/42/model.json" \
  --prompt "Summarize this"
```

For the Go package, inject the application's file reader when creating the
validated config once:

```go
config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
    BaseURL:            "http://localhost:8080/",
    Secret:             serviceSecret,
    ModelProfilePath:   userModelProfilePath,
    ModelProfileReader: os.ReadFile,
})
if err != nil {
    return err
}
client, err := llmproxyclient.NewClient(config, http.DefaultClient)
if err != nil {
    return err
}
```

`Config.MessagesPostURL` also resolves the current profile and therefore returns
`(string, error)` in profile-capable client versions.

The profile is the sole provider/model source in this mode. Do not combine it
with `--model`, a request `model`, `--provider`, `ConfigInput.Provider`, or a
`provider` or `model` query parameter on the base URL. The clients reject those
competing inputs; they never choose a winner or retain a previous parsed
profile. A missing, unreadable, malformed, incomplete, or unsupported profile
also fails that request before HTTP with `ErrInvalidModelProfile` (Go) or
`LLMProxyModelProfileError` (Python). The proxy remains responsible for
validating whether the resulting provider/model pair is supported.

Without a profile path, the model-omitting tenant/provider-default path above
remains the separate normal contract.

### Python client package

The same transport contract is available as an importable Python package:

```shell
uv pip install --upgrade "llm-proxy-client @ git+https://github.com/tyemirov/llm-proxy.git@master#subdirectory=python"
```

For reproducible application builds, replace `master` with the desired released
repository tag.

```python
from llm_proxy_client import (
    Client,
    ClientConfig,
    ClientMessage,
    ClientMessagesRequest,
    image_asset_attachment,
)

client = Client(
    ClientConfig(
        base_url="http://localhost:8080/?provider=gemini",
        secret="mysecret",
    )
)

text = client.post_messages(
    ClientMessagesRequest(
        messages=(ClientMessage(role="user", content="Summarize this"),),
        max_tokens=512,
        request_timeout_seconds=900,
    )
)
```

The Python client has the same asset contract:

```python
asset = client.upload_asset(frame_bytes, "image/png")
frame = image_asset_attachment(asset.asset_id, asset.mime_type)
text = client.post_messages(
    ClientMessagesRequest(
        messages=(
            ClientMessage(
                role="user",
                content="Inspect this exact frame.",
                attachments=(frame,),
            ),
        ),
        model="gemini-3.5-flash",
    )
)
```

`ClientMessagesRequest.reasoning_effort` is the same optional per-request
override. Supply a nonblank value only for a resolved provider/model route that
declares it; omit the field to retain the tenant default.
`ClientMessagesRequest.request_timeout_seconds` serializes the canonical proxy
work-budget header. Omit it to use the server default.

Python client `0.2.0` removes `ClientConfig.timeout_seconds`. Move that value to
each `ClientMessagesRequest` that needs it. The default urllib transport is
called without a total-response timeout; applications that intentionally need
an independent transport deadline can continue to inject an opener that owns
that policy.

The Python package is v2-only. For chat-transcript callers, send the same
`post_messages` request with multiple messages:

```python
chat_text = client.post_messages(
    ClientMessagesRequest(
        messages=(
            ClientMessage(role="user", content="Summarize this", order=2),
            ClientMessage(role="system", content="Be concise.", order=1),
        ),
        model="deepseek-v4-flash",
    )
)
```

Pass `model` only when an application intentionally pins one request instead
of using the tenant or selected-provider default.

To give one application user a reloadable model choice, configure that user's
profile path and reader once. The client does not cache its parsed contents:

```python
from pathlib import Path


def read_model_profile(path: str) -> str:
    return Path(path).read_text(encoding="utf-8")


user_client = Client(
    ClientConfig(
        base_url="http://localhost:8080/",
        secret="mysecret",
        model_profile_path="/var/lib/my-app/users/42/model.json",
        model_profile_reader=read_model_profile,
    )
)
```

Publish a complete replacement JSON document atomically at that path after the
user selects a new provider/model pair. Do not set `provider` on the config or
base URL, or `model` on the request, when this profile is configured.

The optional `order` field is for callers that do not want to rely on array
position. When any message includes `order`, every submitted message must
include a unique non-negative integer `order`; the proxy sorts ascending before
provider routing and echoes provided order values in JSON responses.

For local development from the repository root, target the canonical `python/`
package project explicitly:

```shell
uv pip install -e ./python
make python-test
make python-lint
```

### Basic request (default provider and model, no web search)

```shell
curl --get \
  --data-urlencode "prompt=Hello, how are you?" \
  --data-urlencode "key=mysecret" \
  "http://localhost:8080/"
```

### Choose a provider

```shell
curl --get \
  --data-urlencode "prompt=Summarize this cheaply" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "provider=deepseek" \
  --data-urlencode "model=deepseek-v4-flash" \
  "http://localhost:8080/"
```

Current Qwen text generation through the tenant's saved DashScope workspace:

```shell
curl --get \
  --data-urlencode "prompt=Summarize this with Qwen" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "provider=dashscope" \
  --data-urlencode "model=qwen3.7-plus" \
  --data-urlencode "max_tokens=512" \
  "http://localhost:8080/"
```

Gemini text generation:

```shell
curl --get \
  --data-urlencode "prompt=Summarize this with Gemini" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "provider=gemini" \
  --data-urlencode "model=gemini-3.5-flash" \
  --data-urlencode "max_tokens=512" \
  "http://localhost:8080/"
```

Anthropic Claude text generation:

```shell
curl --get \
  --data-urlencode "prompt=Summarize this with Claude" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "provider=anthropic" \
  --data-urlencode "model=claude-sonnet-4-6" \
  --data-urlencode "max_tokens=512" \
  "http://localhost:8080/"
```

xAI Grok text generation:

```shell
curl --get \
  --data-urlencode "prompt=Summarize this with Grok" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "provider=xai" \
  --data-urlencode "model=grok-4.3" \
  --data-urlencode "max_tokens=512" \
  "http://localhost:8080/"
```

Meta Muse Spark 1.2 text generation:

```shell
curl --get \
  --data-urlencode "prompt=Summarize this with Muse Spark" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "provider=meta" \
  --data-urlencode "model=muse-spark-1.2" \
  --data-urlencode "max_tokens=512" \
  "http://localhost:8080/"
```

### Large text request

Use `POST /` with a JSON body when the prompt is too large for a URL query
parameter or when the caller already has a chat transcript. Authentication
still uses the `key` query parameter, which is the configured tenant secret.
Provider selection also stays in the query parameter. Do not send upstream
provider secrets in the request body; the proxy reads them from server-side
configuration. The JSON body is capped by `server.max_prompt_bytes`.

```shell
curl -X POST \
  -H "Content-Type: application/json" \
  --data '{"prompt":"large text...","model":"gpt-5.5","web_search":false,"system_prompt":"optional","max_tokens":4096,"reasoning_effort":"high"}' \
  "http://localhost:8080/?key=mysecret"
```

Chat transcript on `POST /`:

```shell
curl -X POST \
  -H "Content-Type: application/json" \
  --data '{"messages":[{"role":"user","content":"Summarize this","order":2},{"role":"system","content":"Be concise.","order":1}],"model":"deepseek-v4-flash","max_tokens":4096}' \
  "http://localhost:8080/?key=mysecret&provider=deepseek"
```

Canonical v2 chat transcript:

```shell
curl -X POST \
  -H "Content-Type: application/json" \
  --data '{"messages":[{"role":"user","content":"Summarize this","order":2},{"role":"system","content":"Be concise.","order":1}],"model":"deepseek-v4-flash","max_tokens":4096}' \
  "http://localhost:8080/v2?key=mysecret&provider=deepseek"
```

The authoritative body-field list, required/optional distinction, nested message
shape, and response schemas are rendered directly from OpenAPI in the
[`POST /` API reference](https://llm-proxy.mprlab.com/docs/#operation-postText)
and
[`POST /v2` API reference](https://llm-proxy.mprlab.com/docs/#operation-postV2Messages).
The examples above are intentionally illustrative rather than a separately
maintained field inventory.

For `POST /`, `provider` remains a query parameter. Query `model` may override
the JSON body only when the body omits `model` or provides the same value;
conflicting values return `400 Bad Request`.
Bodies that provide both `prompt` and `messages`, empty `messages`, unsupported
message roles, empty message content, partially specified `order`, duplicate
or negative `order`, or both `system_prompt` and a system message return
`400 Bad Request` before any upstream call.
The proxy returns `400 Bad Request` before upstream dispatch for MiniMax M2
values above `204800` and current Qwen or Gemini values above `65536`. The same
response applies to values above the configured Claude model output limit.

`POST /v2` is the canonical chat endpoint. Its exact accepted fields come from
the OpenAPI schema, including the omission-versus-explicit-value contract for
`reasoning_effort`. It rejects `prompt` and body `system_prompt`; send a
`system` role message instead. The tenant default system prompt is still
prepended when the submitted messages do not include a system message.
Only `POST /v2` user messages may include `attachments`; compatibility
`POST /`, `GET /`, and `/dictate` do not accept that field. Each attachment
is one exact union variant. An inline attachment contains `type`, `mime_type`,
and canonical padded base64 `data`. An asset attachment contains `type`,
`asset_id`, and `mime_type`. The proxy validates tenant ownership, asset state,
expiry, MIME type, byte count, and stored-byte integrity before provider
dispatch. The proxy calculates private checksums for integrity validation.
These checksums are not public asset identity or response fields.
`server.max_prompt_bytes` applies to compatibility `POST /`. Canonical
`POST /v2` accepts an encoded JSON envelope of 8 MiB or less. A smaller
catalog-derived limit applies when its configured text and inline media limits
have a smaller sum. The proxy applies the selected provider offering's
published media limits and transport rules.
Upload larger media through `POST /model/v1/assets` and send its asset
reference through `/v2`.

Upload an asset with the exact media content type:

```shell
curl -X POST \
  -H "Content-Type: image/png" \
  --data-binary @./frame.png \
  "http://localhost:8080/model/v1/assets?key=mysecret"
```

Use the returned asset record in the canonical message:

```json
{
  "messages": [
    {
      "role": "user",
      "content": "Inspect this exact frame.",
      "attachments": [
        {
          "type": "image",
          "asset_id": "ast_0123456789abcdef0123456789abcdef",
          "mime_type": "image/png"
        }
      ]
    }
  ],
  "model": "gemini-3.5-flash"
}
```

`DELETE /model/v1/assets/{asset_id}?key=...` deletes the tenant asset. The
default store retains an available asset for 48 hours.

### Choose an OpenAI model

```shell
curl --get \
  --data-urlencode "prompt=Summarize quantum error correction" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "model=gpt-4o" \
  "http://localhost:8080/"
```

### Enable web search

```shell
curl --get \
  --data-urlencode "prompt=What changed in the 2025 child tax credit?" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "web_search=true" \
  "http://localhost:8080/"
```

You can enable web search with GPT-5 by specifying the model:

```shell
curl --get \
  --data-urlencode "prompt=Latest research on quantum gravity" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "model=gpt-5" \
  --data-urlencode "web_search=true" \
  "http://localhost:8080/"
```

### Dictation request

```shell
curl -X POST \
  -F "audio=@./recording.webm" \
  "http://localhost:8080/dictate?key=mysecret"
```

SiliconFlow dictation:

```shell
curl -X POST \
  -F "audio=@./recording.webm" \
  "http://localhost:8080/dictate?key=mysecret&provider=siliconflow"
```

Optional model override:

```shell
curl -X POST \
  -F "audio=@./recording.webm" \
  "http://localhost:8080/dictate?key=mysecret&model=gpt-4o-mini-transcribe"
```

### Response formats

You can request alternative formats using either the `format` query parameter or
the `Accept` header. Supported values are:

* `text/csv` - the reply as a single CSV cell with internal quotes doubled
  and a trailing newline
* `application/json` - JSON object containing `request` and `response` fields,
  plus `usage` when upstream token usage is available
* `application/xml` - XML document `<response request="...">...</response>`

If no supported value is provided, `text/plain` is returned.

When upstream text providers return token usage, the proxy also sets these
response headers without changing the plain text, XML, or CSV response bodies:

| Header | Description |
|--------|-------------|
| `X-LLM-Proxy-Request-Tokens` | Normalized request/input token count |
| `X-LLM-Proxy-Response-Tokens` | Normalized response/output token count |
| `X-LLM-Proxy-Total-Tokens` | Normalized total token count |

JSON-format LLM responses include the same normalized counts:

```json
{
  "request": "Hello",
  "response": "Hi",
  "object": "chat.completion",
  "model": "gpt-4.1",
  "choices": [
    {
      "index": 0,
      "finish_reason": "stop",
      "message": {
        "role": "assistant",
        "content": "Hi"
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Hello"
    }
  ],
  "usage": {
    "request_tokens": 1,
    "response_tokens": 1,
    "total_tokens": 2
  }
}
```

The response `messages` field echoes only caller-visible request messages.
Server-injected tenant default system prompts are sent upstream when applicable,
but are not returned in response metadata.

## Canonical endpoint reference

Use the derived [API reference](https://llm-proxy.mprlab.com/docs/) for the
complete operation inventory and the exact query, header, JSON, multipart,
authentication, media-type, response-header, and status contracts. Use the
[committed OpenAPI artifact](docs/openapi.yaml) for tooling and review.

The request examples in [Usage](#usage) demonstrate common calls without
duplicating that inventory. In particular, dictation has one canonical incoming
multipart file part, `audio`; the obsolete `file` alias is rejected.

## Model catalog

The provider catalog in [configs/providers.yml](configs/providers.yml)
declares the LLM endpoint models below. The `/dictate` endpoint defaults to
OpenAI's audio transcriptions API. It also supports SiliconFlow, Z.AI, and
Grok/xAI through their provider selectors. Not all configured models support
tools. Use a model marked `Yes` below for web search. A dash in the proxy
`max_tokens` column means that the proxy only requires a positive value. The
upstream provider enforces its model limit.

### Reasoning-effort capabilities

The checked-in OpenAI catalog follows the current model documentation and keeps
each model's list separate. GPT-4.1 is explicitly a non-reasoning model and
does not accept a configurable effort; GPT-5 mini is part of the reasoning GPT-5
API family and accepts the same four original GPT-5 effort values:

| Model | Allowed `reasoning_effort` values |
|-------|-----------------------------------|
| `gpt-4.1` | Not supported |
| `gpt-5-mini` | `minimal`, `low`, `medium`, `high` |
| `gpt-5` | `minimal`, `low`, `medium`, `high` |
| `gpt-5.5` | `none`, `low`, `medium`, `high`, `xhigh` |
| `gpt-5.5-pro` | `medium`, `high`, `xhigh` |
| `gpt-5.6`, `gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna` | `none`, `low`, `medium`, `high`, `xhigh`, `max` |

See OpenAI's [GPT-4.1 model reference](https://developers.openai.com/api/docs/models/gpt-4.1),
[GPT-5 API launch contract](https://openai.com/index/introducing-gpt-5-for-developers/),
[GPT-5 model reference](https://developers.openai.com/api/docs/models/gpt-5),
[GPT-5.5 model reference](https://developers.openai.com/api/docs/models/gpt-5.5),
[GPT-5.5 Pro model reference](https://developers.openai.com/api/docs/models/gpt-5.5-pro),
and [latest-model guide](https://developers.openai.com/api/docs/guides/latest-model).

Kimi K3 accepts `low`, `high`, and `max`. Omission keeps Moonshot's provider
default. Kimi K2.6 and the Kimi K2.7 Code routes do not expose a selectable
effort through the proxy.

### Model capabilities

| Model | Provider | Provider default | Proxy `max_tokens` limit | Web search |
|-------|----------|------------------|--------------------------|------------|
| `gpt-4.1` | OpenAI | Yes | - | Yes |
| `gpt-4o` | OpenAI | No | - | Yes |
| `gpt-4o-mini` | OpenAI | No | - | No |
| `gpt-5` | OpenAI | No | - | Yes |
| `gpt-5-mini` | OpenAI | No | - | No |
| `gpt-5.5` | OpenAI | No | - | Yes |
| `gpt-5.5-pro` | OpenAI | No | - | Yes |
| `gpt-5.6` | OpenAI | No | - | Yes |
| `gpt-5.6-sol` | OpenAI | No | - | Yes |
| `gpt-5.6-terra` | OpenAI | No | - | Yes |
| `gpt-5.6-luna` | OpenAI | No | - | Yes |
| `muse-spark-1.1` | Meta | Yes | - | No |
| `muse-spark-1.2` | Meta | No | - | No |
| `deepseek-v4-flash` | DeepSeek | Yes | - | No |
| `deepseek-v4-pro` | DeepSeek | No | - | No |
| `deepseek-chat` | DeepSeek | No | - | No |
| `deepseek-reasoner` | DeepSeek, SiliconFlow | SiliconFlow | - | No |
| `qwen-plus` | DashScope/Qwen | Yes | - | No |
| `qwen3.7-max` | DashScope/Qwen | No | `65536` | No |
| `qwen3.7-plus` | DashScope/Qwen | No | `65536` | No |
| `qwen3.6-flash` | DashScope/Qwen | No | `65536` | No |
| `kimi-k2.6` | Moonshot/Kimi | Yes | - | No |
| `kimi-k3` | Moonshot/Kimi | No | - | No |
| `kimi-k2.7-code` | Moonshot/Kimi | No | - | No |
| `kimi-k2.7-code-highspeed` | Moonshot/Kimi | No | - | No |
| `minimax-m2.7` | MiniMax | Yes | `204800` | No |
| `minimax-m2.7-highspeed` | MiniMax | No | `204800` | No |
| `minimax-m2.5` | MiniMax | No | `204800` | No |
| `minimax-m2.5-highspeed` | MiniMax | No | `204800` | No |
| `minimax-m2.1` | MiniMax | No | `204800` | No |
| `minimax-m2.1-highspeed` | MiniMax | No | `204800` | No |
| `minimax-m2` | MiniMax | No | `204800` | No |
| `glm-5.1` | Z.AI | Yes | - | No |
| `glm-5.2` | Z.AI | No | `131072` | No |
| `gemini-3.5-flash` | Gemini | Yes | `65536` | No |
| `gemini-3-flash-preview` | Gemini | No | `65536` | No |
| `claude-opus-4-8` | Anthropic/Claude | No | `128000` | No |
| `claude-fable-5` | Anthropic/Claude | No | `128000` | No |
| `claude-sonnet-5` | Anthropic/Claude | No | `128000` | No |
| `claude-sonnet-4-6` | Anthropic/Claude | Yes | `64000` | No |
| `claude-haiku-4-5-20251001` | Anthropic/Claude | No | `64000` | No |
| `claude-haiku-4-5` | Anthropic/Claude | No | `64000` | No |
| `claude-sonnet-4-5-20250929` | Anthropic/Claude | No | `64000` | No |
| `claude-sonnet-4-5` | Anthropic/Claude | No | `64000` | No |
| `claude-opus-4-1-20250805` | Anthropic/Claude | No | `32000` | No |
| `claude-opus-4-1` | Anthropic/Claude | No | `32000` | No |
| `grok-4.3` | Grok/xAI | Yes | - | No |
| `grok-4.3-latest` | Grok/xAI | No | - | No |
| `grok-4.5` | Grok/xAI | No | - | No |
| `grok-4.20-0309-reasoning` | Grok/xAI | No | - | No |
| `grok-4.20-0309-non-reasoning` | Grok/xAI | No | - | No |
| `grok-latest` | Grok/xAI | No | - | No |
| `grok-build-0.1` | Grok/xAI | No | - | No |
| `grok-code-fast` | Grok/xAI | No | - | No |
| `grok-code-fast-1` | Grok/xAI | No | - | No |
| `grok-code-fast-1-0825` | Grok/xAI | No | - | No |

The three current Qwen routes are text-only proxy offerings. Each uses the
tenant's exact Singapore workspace URL and the OpenAI-compatible Chat
Completions endpoint. `max_tokens` is the request control and has an inclusive
`1..65536` proxy boundary. Each model has a 1,000,000-token context window.
The catalog records Singapore pay-as-you-go list rates for the lowest published
input tier. Qwen 3.7 Max costs $2.50 for input and $7.50 for output per million
tokens. Qwen 3.7 Plus costs $0.40 for input and $1.60 for output. Qwen 3.6 Flash
costs $0.25 for input and $1.50 for output. The limits, prices, Singapore
availability, and workspace URL were verified on 2026-08-13 against Alibaba's
[text model guide](https://www.alibabacloud.com/help/en/model-studio/text-generation-model),
[model catalog](https://www.alibabacloud.com/help/en/model-studio/models),
[OpenAI compatibility guide](https://www.alibabacloud.com/help/en/model-studio/compatibility-with-openai-responses-api),
and [pricing page](https://www.alibabacloud.com/help/en/model-studio/model-pricing).

All seven MiniMax M2 routes are text-only Chat Completions offerings with a
204,800-token context record and output boundary. M2.7 input and output cost
$0.30 and $1.20 per million tokens. M2.7 Highspeed costs $0.60 and $2.40.
The M2.5, M2.1, and M2 routes use the same standard input and output rates as
M2.7. Their Highspeed variants use the M2.7 Highspeed rates. The catalog also
records the published prompt-cache read and write rates. MiniMax now lists
M2.5, M2.1, and M2 under legacy prices, but still includes them in the supported
OpenAI-compatible model list. Limits, availability, and standard PAYG prices
were verified on 2026-08-13 against MiniMax's official references above.

### Dictation capabilities

| Provider selector | Models | Credential field | Provider transport | Notes |
|-------------------|--------|------------------|--------------------|-------|
| `openai` | `gpt-4o-mini-transcribe`, `gpt-4o-transcribe` | Tenant-managed API key | Catalog `dictation` transport | Default dictation provider and default model `gpt-4o-mini-transcribe`. |
| `siliconflow` | `sensevoice-small` | Tenant-managed API key | Catalog `dictation` transport | OpenAI-compatible audio transcription. |
| `zai` | `glm-asr-2512` | Tenant-managed API key | Catalog `dictation` transport | Z.AI GLM-ASR sends `model=glm-asr-2512`. |
| `xai` | `xai-stt` | Tenant-managed API key | Catalog `dictation` transport | The upstream request omits the multipart `model` field. |

### Status codes

* `200 OK` - success
* `400 Bad Request` - missing/invalid parameters, invalid request timeout, invalid multipart audio form, unknown provider/model, or unsupported provider capability. Invalid timeout headers return `{"error":{"code":"invalid_request_timeout","max_request_timeout_seconds":M}}`.
* `403 Forbidden` - missing or invalid `key`
* `413 Payload Too Large` - compatibility JSON exceeds `max_prompt_bytes`, a
  `/v2` JSON envelope exceeds its service or catalog-derived ingress limit,
  dictation audio exceeds `max_input_audio_bytes`, an asset upload exceeds
  `max_asset_bytes`, or media exceeds the selected provider offering's
  transport limit. Provider media failures use
  `provider_media_limit_exceeded`.
* `429 Too Many Requests` - upstream provider rate limit; returns the sanitized `provider_rate_limited` JSON contract
* `503 Service Unavailable` - selected provider credential is unavailable because that non-default provider is disabled or missing its API key
* `504 Gateway Timeout` - the accepted proxy work budget expired; the response is `{"error":{"code":"request_timeout","request_timeout_seconds":N}}`
* `502 Bad Gateway` - upstream provider API or response-protocol failure; returns the sanitized `provider_error` JSON contract

Provider failures use one stable response shape:

```json
{
  "error": {
    "code": "provider_error",
    "provider": "gemini",
    "upstream_status": 503,
    "retryable": true,
    "request_id": "PROXY_GENERATED_REQUEST_ID",
    "retry_after": "120"
  }
}
```

All six fields are present. `upstream_status` is the exact provider HTTP status,
not the proxy status. It is `null` when no usable unsuccessful upstream HTTP
response exists. The proxy preserves upstream `429` as public `429` and maps
other provider failures to public `502`. `retryable` is true for upstream HTTP
`408`, `425`, `429`, `500`, `502`, `503`, and `504`. It is also true for a
failed Gemini Interaction when every URI code has a retryable final path
segment. An unknown final segment sets `retryable` to false. The value
classifies the provider condition. It does not make an LLM request idempotent
or remove duplicate-work and billing risk. `retry_after` is `null` unless the
provider supplied a valid delta-seconds or HTTP-date value. The proxy also
returns that normalized value in the standard `Retry-After` header.

`request_id` is generated by the proxy, returned in
`X-LLM-Proxy-Request-ID`, and recorded in structured proxy logs. Provider
failure responses and logs never include the provider's raw response body or
error message. For a failed Gemini Interaction, structured logs can include
validated `provider_error_codes`. Public responses do not include these codes.

## Security

* All requests must include a configured tenant secret via `key=...`.
* Client requests must not include upstream provider API keys; public proxy endpoints reject provider-key-like query, JSON, and multipart form fields.
* Request logs record only the query-free path plus method, status, latency, client IP, proxy request ID, and tenant metadata; they do not record query strings, request bodies, cookies, or authorization headers.
* Self-service provider API keys are accepted only through TAuth-protected management endpoints. Autosave responses return masked status; raw retrieval requires the explicit owner-authenticated reveal action.
* Public static pages load Google Analytics and LoopAware page-view scripts. The canonical `/privacy/` policy discloses those integrations without making unsupported collection, retention, consent, or opt-out claims. Do not put tenant secrets or other sensitive values in public-page URLs.
* Do not expose this service to the public internet without appropriate network controls.

## Implementation Plans

Current scoped implementation plans are tracked under `docs/implementation/`.

## MPR Integration Verification

For Marco Polo Research Lab integration workflows, use the Codex
`mpr-integration` skill when a change needs contract/profile/task-based
black-box verification against an MPR app or fixture. Keep app-specific
hostnames, cookie names, ports, OAuth callbacks, and environment literals in
the selected integration profile or deployment docs, not in this README.

## Releasing

Use `make release`, `make publish`, and `make deploy` from the selected clean
checkout. These are deliberately thin entrypoints into the exact sibling
`../mprlab-gateway`. This repository owns its permanent major version `1`
release policy. The gateway runs the committed
`scripts/validate-release-decision` program against the exact decision that
the transaction prepares or reuses. The validator requires that decision to
match the Python client version.

The generic gateway transaction validates this app, builds the declared
multi-platform container and frontend-rendered Pages artifact from committed source,
and seals the canonical SemVer release. Publication creates only missing remote
state from that exact release and rejects conflicts. Deployment uses the
gateway-owned Ansible inventory and transaction to reconcile only this app's
declared resources, then verifies its backend and Pages boundaries. Publish and
deploy do not rerun CI or rebuild artifacts, and all three stages are retry-safe.

The gateway does not own this application's version policy, version history,
artifact names, migration, cutover, or cleanup sequence. Keep each such rule in
this repository.

## License

This project is licensed under the MIT License. See [LICENSE](MIT-LICENSE) for
details.
