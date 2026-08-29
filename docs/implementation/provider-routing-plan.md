# Provider Routing Implementation Plan

Status: implemented provider-routing contract notes retained from the retired provider-routing backlog.

## Goal

Extend `llm-proxy` from an OpenAI-only proxy into an explicit multi-provider proxy while preserving current OpenAI defaults for existing clients.

## Request Contract

- `provider` is an optional query parameter on `GET /`, `POST /`, `POST /v2`, and `POST /dictate`.
- Omitted `provider` means the authenticated tenant's default provider.
- `model` keeps its current meaning; omitted `model` means the authenticated tenant's default model when set, otherwise the selected provider's configured default model.
- A provider with an API key configured must have a configured default text model so provider-selected requests can omit `model` consistently.
- Managed tenants persist complete canonical provider/model pairs chosen only
  from providers for which that tenant has a saved API key. The text pair is
  empty only when no provider key exists; the dictation pair is empty when no
  keyed provider supports dictation. A request that omits routing fields uses
  the exact persisted pair, and read-time routing never substitutes another
  provider or model.
- Compatibility JSON `POST /` accepts exactly one text input shape: `prompt` for a single user prompt or `messages[]` for an OpenRouter/OpenAI-compatible chat transcript.
- Canonical JSON `POST /v2` accepts only `messages[]`; request-body `prompt`
  and `system_prompt` are invalid. A user message may additionally contain an
  ordered `attachments[]` array. Compatibility `GET /`, JSON `POST /`, and
  `/dictate` never accept message attachments.
- `messages[]` items contain `role` and nonblank string `content`. Supported
  roles are `system`, `user`, and `assistant`; at least one `user` message is
  required. `attachments[]` is allowed only on a user message and each item
  uses one exact union variant. Inline media contains `type`, `mime_type`,
  canonical padded base64 `data`, and the matching lowercase hexadecimal
  `sha256`. Asset media contains `type`, `asset_id`, `mime_type`, and the
  matching lowercase hexadecimal `sha256`. Image accepts `image/jpeg`,
  `image/png`, and `image/webp`. Audio accepts `audio/m4a`, `audio/mpeg`, and
  `audio/wav`.
- `POST /model/v1/assets` stores exact image or audio bytes for the
  authenticated tenant. The request supplies the exact media content type and
  `X-LLM-Proxy-Asset-SHA256`. The response supplies one opaque asset id and its
  hash-bound metadata. `DELETE /model/v1/assets/{asset_id}` marks the asset as
  deleted and removes its stored bytes.
- `server.max_prompt_bytes` limits compatibility `POST /`. Canonical
  `POST /v2` accepts an encoded JSON envelope of 8 MiB or less. A smaller
  catalog-derived limit applies when its text and inline media limits have a
  smaller sum. Larger media uses the asset endpoint and an asset reference.
  The proxy uses the resolved provider offering's media limits. It validates
  asset ownership, state, expiry, MIME type, size, and SHA-256 before provider
  dispatch.
- `messages[].order` is optional. When any submitted message includes `order`, every submitted message must include a unique non-negative integer `order`; the proxy sorts submitted messages by ascending `order` before adding a request or tenant system prompt and before routing upstream.
- With `messages[]` on `POST /`, body `system_prompt` is prepended as a system message only when the transcript does not already contain a `system` message. A body containing both `system_prompt` and a system message is invalid. With `POST /v2`, callers send system instructions as `system` role messages.
- `max_tokens` is an optional positive integer on `GET /` query strings and JSON `POST /` bodies. It is the initial per-attempt output budget and is reused for missing-suffix attempts.
- Provided `max_tokens` maps to OpenAI Responses `max_output_tokens`, Meta, Moonshot, and MiniMax Chat Completions `max_completion_tokens`, other OpenAI-compatible chat completions `max_tokens`, Anthropic Messages `max_tokens`, and Gemini Interactions `generation_config.max_output_tokens`.
- Omitted `max_tokens` means the proxy omits provider max-token fields and lets the selected provider/model default apply, except Anthropic Messages where the upstream API requires `max_tokens` and the proxy sends the selected model's configured synchronous output limit. After an output-budget stop with no visible progress, a configured model output limit becomes the generic ceiling for increasing the next attempt.
- Known provider-specific output-token ceilings are validated before upstream calls. MiniMax M2 routes reject `max_tokens` above `204800`. Gemini text models reject values above `65536`. Claude models reject values above their configured synchronous Messages output limits with `400 Bad Request`.
- `reasoning_effort` is optional on `GET /` as a query parameter and on JSON `POST /` and `POST /v2` as a body field. Omission retains the resolved tenant default. A supplied value must be nonblank and supported by the exact resolved text provider/model route; blank, `null`, or unsupported values return `400 Bad Request` before a provider call.
- `structured_output.schema` is optional on canonical `POST /v2`. It requires
  one valid `Idempotency-Key` header. The header is invalid without the field.
- The proxy compiles the caller JSON Schema before provider dispatch. It maps
  the schema only to supported provider protocols and validates the final JSON.
- Structured requests do not use output continuation or repair inference.
  Unsupported routes and invalid provider JSON fail without a second call.
- A structured request uses a private tenant-bound record with
  `not_dispatched`, `dispatched`, `succeeded`, `failed`, or `uncertain` state.
  Authenticated `GET /v2/requests` reconciles the same idempotency key.
- The request intent contains the resolved route and canonical request body.
  Request-timeout and transport settings do not change that identity.
- A restart changes an interrupted `dispatched` record to `uncertain`. An
  uncertain record never permits automatic or duplicate provider dispatch.
- The record stores tenant, key, and intent hashes instead of their plaintext
  values. It does not store prompts, schemas, credentials, or provider bodies.
- `X-LLM-Proxy-Request-Timeout-Seconds` is an optional positive whole-number
  header on `GET /`, `POST /`, `POST /v2`, `POST /model/v1/assets`, and
  `POST /dictate`. Omission uses `server.request_timeout_seconds`; a supplied
  value must be in the inclusive range
  `1..server.max_request_timeout_seconds`.
- The effective request budget begins at authenticated ingress before body
  parsing and covers validation, queue admission, provider work, OpenAI
  background polling, and response construction. Provider adapters propagate
  the ingress context and never start a replacement deadline.
- Accepted responses echo the effective timeout header. Invalid values return
  the canonical `400 invalid_request_timeout` JSON envelope before upstream
  admission, and budget expiry returns the canonical `504 request_timeout`
  JSON envelope. Caller cancellation remains independent and may end the
  request sooner without a response.
- For JSON `POST /`, query `model` may override the body only when the body omits `model` or provides the same value.
- Conflicting query/body `model` values return `400 Bad Request`.
- JSON `POST /` bodies that provide both `prompt` and `messages`, neither field, empty messages, unsupported roles, empty content, a missing user message, partially specified `order`, duplicate `order`, or negative `order` return `400 Bad Request`.
- JSON `POST /v2` bodies that provide `prompt`, body `system_prompt`, missing
  or empty messages, unsupported roles, empty content, a missing user message,
  partially specified `order`, duplicate `order`, negative `order`, media on a
  non-user message, `null` or empty attachments, malformed base64, mismatched
  digests, unsupported media types, or unknown JSON fields return
  `400 Bad Request`. Media unsupported by the exact resolved model also returns
  `400` before any provider call.
- Upstream provider API keys are never accepted from client requests.

## Providers

| Provider | Aliases | Wire contract | Execution lifecycle | Dictation | Web Search |
|----------|---------|---------------|---------------------|-----------|------------|
| `openai` | none | `openai_responses` | `pollable_resource` | OpenAI audio transcription | Supported by configured OpenAI model entries with `web_search: true` |
| `meta` | none | `openai_chat_completions` | `synchronous_completion` | Not supported | Not supported |
| `deepseek` | none | `openai_chat_completions` | `synchronous_completion` | Not supported | Not supported |
| `dashscope` | `qwen` | `openai_chat_completions` | `synchronous_completion` | Not supported | Not supported |
| `moonshot` | `kimi` | `openai_chat_completions` | `synchronous_completion` | Not supported | Not supported |
| `minimax` | none | `openai_chat_completions` | `synchronous_completion` | Not supported | Not supported |
| `siliconflow` | none | `openai_chat_completions` | `synchronous_completion` | OpenAI-compatible audio transcription | Not supported |
| `zai` | none | `openai_chat_completions` | `synchronous_completion` | Z.AI GLM-ASR transcription | Not supported |
| `gemini` | none | `gemini_interactions` | `pollable_resource` | Not supported | Not supported |
| `anthropic` | `claude` | `anthropic_messages` | `synchronous_completion` | Not supported | Not supported |
| `xai` | none | Model-specific: `grok-4.5` uses `openai_responses`, and other text models use `openai_chat_completions` | `synchronous_completion` | xAI STT | Not supported |

This matrix describes capabilities wired through `llm-proxy`. Upstream products
can expose speech APIs that are not yet proxy adapters; do not mark them
supported for `/dictate` until the provider registry and black-box routing tests
cover that path.

Structured requests use `openai_responses`, `gemini_interactions`, or
`anthropic_messages`. Each other protocol rejects a structured request before
provider dispatch.

The canonical Meta contract uses selector `meta` with no aliases and
`https://api.meta.ai/v1`. Each tenant supplies one Meta API key. Muse Spark 1.1
remains the default. Muse Spark 1.2 is an additional Standard-tier text model.
llm-proxy sends both models through the shared Chat Completions adapter. It
maps public `max_tokens` to Meta's `max_completion_tokens` field. Meta describes
Muse Spark 1.2 as coding-focused. This focus does not change the provider
transport or add agent orchestration. The proxy does not expose Meta dictation,
`web_search`, tools, multimodal input, or a Responses API fallback. Current
upstream details live in Meta's
[Muse Spark 1.2 announcement](https://research.meta.ai/blog/introducing-muse-code-and-muse-spark-1-2),
[model reference](https://dev.meta.ai/docs/models),
[Chat Completions reference](https://dev.meta.ai/docs/protocols/chat-completions),
and [pricing and rate-limit documentation](https://dev.meta.ai/docs/pricing-rate-limits).

DashScope is the only Alibaba provider. Each managed tenant saves its Singapore
Model Studio workspace URL with the matching regional API key. Verification and
routed requests use that saved tenant URL. The `qwen` alias resolves to DashScope. `qwen-plus`
remains the exact default. `qwen3.7-max`, `qwen3.7-plus`, and `qwen3.6-flash`
are additional text-only Chat Completions routes. Each has a 1,000,000-token
context record and a `65536` output boundary, and maps public `max_tokens` to
the documented upstream field. Singapore availability, workspace routing,
limits, and list pricing were verified from Alibaba's official model,
compatibility, and pricing references on 2026-08-13.

MiniMax is a distinct text-only provider with canonical selector `minimax`,
endpoint `https://api.minimax.io/v1`, and a tenant-managed API key. The seven M2
routes use exact lowercase canonical model ids and exact provider-native model
ids. `minimax-m2.7` remains the default. The shared adapter maps public
`max_tokens` to MiniMax `max_completion_tokens`. Each route has the documented
204,800-token context and output boundary. MiniMax's supported-model, API,
limit, and standard PAYG price records were verified on 2026-08-13. The proxy
does not add MiniMax-specific reasoning, tools, streaming, or media controls.

## Configuration

Runtime service configuration comes from `config.yml`. The provider catalog
comes from the sibling `providers.yml`. The loader validates the provider
catalog before it validates service configuration.

Environment variables and `.env` files supply placeholder values for
`config.yml`. They also supply optional provider field values through catalog
environment bindings. Client keys and tenant routing defaults remain
tenant-owned management state.

Shared config fields:

- `server.port`
- `server.log_level`
- `server.workers`
- `server.queue_size`
- `server.request_timeout_seconds`
- `server.max_request_timeout_seconds`
- `server.max_prompt_bytes`
- `server.max_asset_bytes`
- `server.asset_retention_seconds`
- `server.asset_store_path`
- `server.max_input_audio_bytes`
- `server.upstream_rate_limits[].origin`
- `server.upstream_rate_limits[].max_requests`
- `server.upstream_rate_limits[].interval`
- `management.public_origin`
- `management.ui_description`
- `management.ui_origins`
- `management.admin_emails`
- `management.tauth_url`
- `management.tauth_tenant_id`
- `management.google_client_id`
- `management.login_path`
- `management.logout_path`
- `management.nonce_path`
- `management.session_path`
- `management.jwt_signing_key`
- `management.jwt_issuer`
- `management.session_cookie_name`
- `management.database_path`
- `management.provider_key_encryption_key`
- `management.management_api_origin`
- `management.proxy_origin`

The provider catalog owns provider identities, fields, transports, models,
offerings, limits, controls, and prices. Static endpoint bases and paths also
belong to the provider catalog. The managed DashScope `base_url` provider field
supplies its tenant-specific workspace URL.

See [Provider Catalog](../provider-catalog.md) for the complete field mapping,
adapter mapping, validation contract, and one-file provider procedure.

The README model-capability table describes the same provider catalog.
Moonshot's current Kimi Chat Completions routes receive
`max_completion_tokens` when a caller supplies the proxy `max_tokens` value.
Kimi K3 maps exact proxy reasoning efforts `low`, `high`, and `max` to the
top-level provider field. Omission keeps Moonshot's default. Kimi K2.6 and
both Kimi K2.7 Code routes receive no `thinking` field, which keeps their
provider default. All four routes serialize ordered canonical JPEG, PNG, and
WebP inputs as Data URL Chat Completions blocks before message text. Only
visible answer content leaves the adapter. For output continuation, the
adapter sends the complete truncated assistant message to the next Kimi
request. This private message includes `reasoning_content` and never enters a
public response or log.
All seven MiniMax M2 routes map public `max_tokens` to
`max_completion_tokens` and carry a configured 204800-token output ceiling.
GLM-5.2 uses the international Z.AI Chat Completions endpoint. Its
128K output maximum is catalog metadata. The proxy does not expose optional
`thinking` or provider-native `reasoning_effort` controls. The
canonical proxy `reasoning_effort` request field is validated against the exact
resolved provider/model capability and translated only through its configured
adapter. A supplied value must be nonblank and exact. It takes precedence over
the tenant routing default. An omitted value uses that default. The current
mappings are OpenAI Responses, OpenAI-compatible Chat Completions, and Gemini
Interactions. A blank or unsupported effort stops before an upstream request.

OpenAI `request_profile` values select stable payload shapes:

- `openai_responses_temperature`
- `openai_responses_temperature_tools`
- `openai_responses_reasoning_tools`

Every text model must explicitly declare one current `wire_contract` and one
`execution_lifecycle`; the loader does not infer either capability from the
provider, base URL, request profile, or presence of an upstream identifier.
The closed wire values are `openai_responses`, `openai_chat_completions`,
`gemini_interactions`, and `anthropic_messages`. The closed lifecycle
values are `synchronous_completion` and `pollable_resource`. Startup rejects
missing, unknown, contradictory, or provider-incompatible pairs and rejects
these text-only fields on dictation entries. A future one-read deferred result
must introduce a distinct lifecycle value.

The shared `pollable_resource` lifecycle owns post-create observation. Each
shared pollable text transport declares a bounded visibility retry policy in
the provider catalog. The lifecycle reads the created resource immediately and
applies the declared statuses, interval, and retry limit without
provider-specific control flow. The caller context bounds every wait. Protocol
adapters continue to own resource wire formats and provider status parsing.

Every OpenAI Responses text request includes `background: true` and
`store: true`. A nonblank response id is polled server-side only for the
documented `queued` and `in_progress` pending states or for a proxy-initiated
stored background synthesis. Documented terminal states are resolved
immediately, and missing or unknown states are rejected rather than guessed.
Only `status=completed` is a successful terminal response.
`status=incomplete` with `reason=max_output_tokens` is normalized into the
same provider-neutral missing-suffix lifecycle used by every text transport.
Other incomplete reasons are canonical `502` upstream failures. Usage objects
observed while polling one response id are cumulative snapshots, so the newest
nonempty snapshot replaces earlier observations. Usage is summed across
distinct missing-suffix attempts and completed-response synthesis requests.
Callers use one normal `GET /`, `POST /`, or `POST /v2` request and receive a
complete formatted answer or a non-2xx response; there is no streaming,
client-side polling, durable provider-job queue, or later resume contract.

Every Gemini text request uses `POST /interactions`, `x-goog-api-key`, and
`Api-Revision: 2026-05-20`. The configured base URL is the `v1beta` API because
that revision exposes the full configured model catalog. Gemini 3.x creates a
stored background interaction; a nonblank interaction id is polled only while
its status is `queued` or `in_progress`. `completed` with visible text is
success, `incomplete` is the
provider-neutral output-limit signal, and `failed`, `cancelled`,
`budget_exceeded`, `requires_action`, malformed, missing, or unknown states
fail closed. The newest nonempty usage snapshot replaces earlier observations
for one interaction; input, output, and total counts are taken from
`total_input_tokens`, `total_output_tokens`, and `total_tokens`, so
provider-counted thought tokens remain represented. Active-resource cancel and
delete operations use independent bounded contexts, so cancel exhaustion cannot
prevent the delete request from starting.

Each Gemini Interactions route rejects assistant history before provider
dispatch. The public request cannot carry the provider state or thought
signatures that a `model_output` step requires.

Gemini offering media limits are part of the validated catalog and public
capability resource. The inline request limit is 20,000,000 encoded request
bytes. The image-count limit is 3,600 files for one request. The published
audio-count limit is `unknown`. Each image or audio Files API upload is limited
to 2,000,000,000 bytes. The adapter builds the complete inline interaction and
uses inline `data` when its encoded size is within the limit. A larger request
streams each exact attachment to the Gemini Files API and uses the returned
`uri` in the same attachment order. The adapter verifies the provider file's
MIME type, byte count, SHA-256, URI, and active state. It deletes every uploaded
provider file when the interaction ends. The catalog records Google's
[file input methods](https://ai.google.dev/gemini-api/docs/file-input-methods),
[image input](https://ai.google.dev/gemini-api/docs/image-understanding),
[audio input](https://ai.google.dev/gemini-api/docs/audio), and
[Files API](https://ai.google.dev/gemini-api/docs/files) guides as the verified
sources.

OpenAI, Anthropic, and xAI image limits are also part of the validated catalog.
The OpenAI Responses adapter sends image Data URLs with `detail: auto`.
The Anthropic Messages adapter sends base64 image blocks before the message text.
The xAI `grok-4.5` adapter sends image Data URLs with `detail: high` and `store: false`.
Each adapter applies its provider offering limits before provider dispatch.

OpenAI background Responses are stored upstream. llm-proxy keeps their ids only
in memory for the active request and never returns or persists them, but the
current adapter does not cancel or delete the stored resource after completion,
failure, timeout, or caller cancellation; OpenAI account retention policy
therefore applies. Background Gemini interaction ids are likewise request-local
and never returned or persisted. The proxy cancels every still-active Gemini
interaction and then deletes it; terminal interactions are deleted directly. A
cancel failure never skips deletion, and a deletion failure prevents a
successful or output-limit result from escaping as success. Gemini
Interactions declares no continuation actions. An `incomplete` interaction is
deleted and returns a provider error without a second create request.

One completion coordinator owns output-budget recovery for all transports.
Adapters normalize only their exact recoverable signal: OpenAI
`incomplete/max_output_tokens`, Chat Completions `length`, Gemini Interactions
`incomplete`, and Anthropic `max_tokens`. The coordinator starts replay only
when the selected transport declares continuation actions. For these
transports, it appends the accumulated assistant text and one missing-suffix
instruction. It then joins the returned suffixes until the provider completes.
The overall request deadline is the hard bound. Other terminal signals return
the canonical failure without partial text. An arbitrary response `id` never
activates polling.

The caller's `max_tokens` value is the initial per-attempt budget and is reused
for suffix attempts. When an output-budget response has no visible text and
the model catalog declares an output limit, the next attempt increases the
budget toward that limit. Each provider call independently passes through the
shared worker, queue, and upstream-rate-limit controls; the coordinator's wait
does not retain a worker. Distinct attempt usage is summed into one final
managed event, while successive OpenAI snapshots for one response id replace
one another. A recovered request records one success and no failure; a request
deadline records one `504` with all usage observed before the deadline.

Bundled clients intentionally expose only the canonical `POST /v2` message
contract. The installable Go CLI maps prompt flags or stdin into v2 `system`
and `user` messages. The reusable Go package additionally exposes closed
image/audio attachment constructors that copy bytes, derive the canonical
base64 and digest representations, and attach only to user messages. The
Python package and CLI remain text-only callers of the same endpoint.
Their optional `reasoning_effort` input serializes the same canonical field;
the clients reject only blank local input and leave exact model-capability
validation to the proxy edge.
Their optional request-timeout input serializes
`X-LLM-Proxy-Request-Timeout-Seconds` on each messages request. The Go CLI uses
`--request-timeout-seconds`; the Go and Python packages expose
`RequestTimeoutSeconds` and `request_timeout_seconds` on their request types.
Omission delegates to the server default. Bundled transports add no unrelated
total-response timeout; Go contexts and explicitly injected transports remain
independent caller-owned cancellation mechanisms.
When a bundled-client request omits `model`, it deliberately sends no model
field and delegates selection to the authenticated tenant or selected provider.
The server keeps `GET /` and compatibility JSON `POST /` available for direct
REST callers. Direct `GET /` accepts an optional `web_search` query value only
as exact `true` or `false`; omission means false, and every other supplied
spelling returns `400` before route resolution. JSON `POST /` and `POST /v2`
use a native boolean body field. The Go package, Go CLI, and Python package
remain v2-only, remove `web_search` inherited from a configured base URL, and
serialize that body field as a JSON boolean. The Python request constructor
also rejects non-boolean runtime values before HTTP.

This permits a managed-tenant owner to change the tenant's routing default in
the LLM Proxy Settings UI and have subsequent model-omitting client requests
use that saved value without an application deployment. Application end-user
model selection is a separate, client-owned contract: the Go library, Go CLI,
and Python client all accept one application-owned JSON model-profile path per
client instance. Its complete document is exactly nonblank `provider` and
`model` string fields, contains no secret or TAuth material, and is reread for
every outbound v2 request. An application atomically replaces a user's profile
after their selection changes, so the next request from the existing client
uses the new pair without a rebuild, restart, or deployment.

Profile mode is mutually exclusive with a request model, configured provider,
or base-URL `provider`/`model` query value. An unreadable, malformed,
incomplete, or competing profile fails before HTTP and never reuses a previous
profile or tenant/provider default. The proxy remains the authority for whether
the resulting provider/model pair is valid. Without a profile path, model
omission keeps the existing tenant/provider-default behavior.

## Managed Provider Credential Verification

`PUT /api/management/tenants/:tenant_id/provider-connections/:provider` accepts
an exact map of catalog-defined provider fields. A nonempty credential value is
an unverified new or replacement credential. A changed setting can require
verification with the retained credential. The handler uses the selected
provider transport for one fixed, non-user-content probe:

- OpenAI Responses uses one synchronous, non-stored `POST /responses` with
  `background: false`, `store: false`, and a 16-token output limit.
- xAI synchronous Responses uses one `POST /responses` at the selected xAI
  base URL. It sends `store: false` and a 16-token output limit.
- DeepSeek, DashScope, Moonshot, MiniMax, SiliconFlow, Z.AI, Meta, and xAI
  Chat Completions models use one authenticated `POST /chat/completions`.
  The request uses the provider's declared token-limit parameter.
- A Gemini model creates one stored background interaction. It then
  observes the interaction through the shared `pollable_resource` lifecycle.
  It cancels active work and deletes the stored interaction before it accepts
  the credential.
- Anthropic uses one Messages request with `x-api-key`,
  `anthropic-version: 2023-06-01`, and `max_tokens: 16`.

The verifier uses the management request context and the shared upstream
worker, queue, and origin-rate-limit boundary. It applies the selected
transport's catalog-owned visibility retry policy after creation. It does not
retry a create, an undeclared provider status, cancel, or delete. It does not
use an alternative endpoint, start a continuation, or record managed usage. A
pollable Gemini probe uses the production adapter's create, retrieve, cancel,
and delete operations. A transport success counts only when its canonical
response is valid. Provider `4xx` credential or model rejection maps to
`422 provider_key_rejected`, except `408` and `429`. Upstream `504` also maps
to timeout. Transport cancellation, deadline, outage, and malformed success
map to the documented provider-neutral error. Candidate keys, probe content,
authenticated URLs, and raw upstream bodies never enter logs, responses,
profiles, or usage rows.

Only successful verification enters the provider connection transaction. The
transaction encrypts each secret provider field. It saves nonsecret provider
fields and the separate provider profile. It reconciles routing defaults and
returns the complete tenant profile.

Every failure preserves the prior connection, profile, and defaults. A blank
saved secret field retains the current encrypted value.

Settings starts this operation automatically when a paste updates the selected
provider key. It announces `Verifying key`, leaves only that input available
for a newer paste, and locks tenant, provider, model, reveal, remove, routing,
and close actions until settlement. Per-candidate abort ownership plus the
existing app/editor versions prevent newer paste, authentication, tenant,
provider, model, or editor context changes from applying a stale result.
Success clears the raw draft and restores the masked key. A safe failure keeps
the draft only in that editor, distinguishes an unsaved first key from an active
prior key, and exposes an explicit retry.

## Managed Routing Defaults

`PUT /api/management/tenants/:tenant_id/defaults` requires every field and
accepts only complete text and dictation provider/model pairs. Each nonempty
pair must name a provider with a configured connection. Dictation also requires that
provider's declared dictation capability. The text pair is both empty only when
the tenant has no provider connection. The dictation pair is both empty only when
none of its configured providers supports dictation. `reasoning_effort` is explicit;
empty means unset and a nonempty value must be in the selected exact text
provider/model capability list. The handler resolves the text pair before
validating effort and constructs all defaults before the database write, so a
partial, unkeyed, unknown, unsupported, cross-provider pair or incompatible
effort fails with `400 managed_routing_defaults_invalid` and leaves the prior
defaults unchanged. The profile response exposes eligibility through
`providers[].configured` and capabilities through `providers[].text_models[]`. It
has no provider-level reasoning capability or global option list. A malformed
profile is an app-integrity failure in the browser, not a UI repair
opportunity.

Saving or removing a provider connection reconciles routing defaults in the same
database transaction. The transaction preserves an eligible current pair.
Otherwise, the first configured provider becomes the text default with its
saved text model. The first configured dictation provider becomes the dictation
default with its configured model. A missing eligible provider clears that
pair. A missing text provider also clears reasoning effort.

Startup requires all persisted pairs to be canonical, catalog-valid, and backed
by saved tenant keys; it never retains a fallback, compatibility read, or
runtime repair path. The bounded schema-version-3 migration applies the same
deterministic reconciliation once, preserves tenant timestamps, verifies every
row, and records the version atomically. Unknown models, corrupt keys, and
unknown or dictation-unsupported providers fail with contextual owner, tenant,
endpoint, provider, and model errors.

The bounded schema-version-4 migration deletes retired `qwencloud` provider
settings. If an affected tenant still has provider keys, its text default moves
to the first keyed provider by canonical identifier. The default uses that
provider's stored text model. Otherwise the migration clears the text pair and
reasoning effort. Settings is then mandatory. Tenant timestamps and historical
usage identifiers are preserved. Verification of every changed row and retained
usage record occurs inside the transaction. The transaction records version 4
after verification. Current-version startup rejects any retired provider
setting or default.

The bounded schema-version-5 migration converts stored provider-native model
values to canonical exact model identifiers. It updates affected provider
settings and tenant defaults. It preserves tenant timestamps and historical
usage records. Current-version startup rejects invalid canonical routes.

The bounded schema-version-6 migration replaces the retired `grok` provider
identity with `xai`. The bounded schema-version-7 migration adds the managed
provider base URL field and removes incomplete DashScope settings that have no
tenant-owned Singapore workspace URL. Each transaction preserves tenant
timestamps and historical usage records.

The bounded schema-version-8 migration replaces the retired `zhipu` provider
identity with `zai`. Preflight decrypts every affected key with its existing
tenant and `zhipu` identity. It validates the projected canonical routes. It
rejects conflicts, corrupt ciphertext, noncanonical identifiers, and retired
base URLs. One transaction re-encrypts each key with the same tenant and `zai`
identity. The transaction updates provider settings and current routing
defaults. It verifies timestamps and historical usage records before it records
the new schema version.
Current-version startup rejects `zhipu`, `glm`, and other noncanonical routes.

The bounded schema-version-9 migration replaces provider-key rows with provider
connection records and provider profile records. It maps predecessor columns
through each current provider definition. It re-encrypts credentials with the
provider field identity as associated data. It verifies all values and
timestamps before it removes the predecessor table.

`server.workers` limits concurrent upstream provider HTTP operations, not whole
client request lifecycles. `server.queue_size` limits the number of additional
upstream HTTP operations waiting for that shared worker limit. OpenAI
background-response sleeps between polls do not occupy worker capacity; only the
actual upstream create, poll, completed-response synthesis, chat,
native-provider, or dictation HTTP operation does. The admission queue does not
persist provider job ids and does not implement retry or resume semantics.

`server.upstream_rate_limits` is enforced by the same shared HTTP client for
text and dictation. Each rule is a strict rolling-window budget keyed by exact
normalized upstream origin (`scheme://host[:port]`), never by provider name.
Calls waiting for a rate window retain bounded queue admission without
occupying a worker. A rate slot is reserved only after worker capacity is
available; a caller that finds the rolling window full releases the worker
before waiting. Each retry is a new upstream call and therefore consumes a new
slot. Empty configuration disables rate limiting. Origins with paths,
queries, fragments, or user info, non-positive maxima or intervals, invalid Go
duration strings, and duplicate normalized origins fail startup. Delayed calls
and context-canceled waits emit structured shared-client logs.

One request-scoped monotonic accumulator correlates the complete routed proxy
lifecycle with the proxy-owned request id. The terminal
`proxy request phase summary` event always includes query-free endpoint,
canonical provider/model, effective request budget, status, outcome, total
latency, and explicit millisecond totals for authentication, upstream
admission, configured origin-rate-limit waiting, observed provider HTTP work
through response-body close, provider poll waiting, shared-continuation
waiting, response formatting, and managed-usage enqueue. Unused phases remain
present with zero. Detached managed-usage persistence is outside the enqueue
phase. These values describe only proxy-observed boundaries; they are neither
provider execution/billing measurements nor a basis for inferring unclassified
time by subtraction.

The `proxy provider progress` event records each OpenAI create/poll observation
and each provider-neutral continuation attempt under the same request id. It
uses an attempt or poll count, normalized provider state or completion signal,
elapsed milliseconds, current output bytes, and accumulated output bytes.
Every visibility error that starts a catalog-declared retry uses `pending`.
An undeclared visibility error or the error that exhausts the retry limit uses
`failure` because it stops the lifecycle.
Progress and terminal events never include provider resource ids, prompts,
messages, generated text, provider bodies, credentials, cookies, or tenant
secrets. The telemetry stays in structured logs; managed usage persistence,
public response/OpenAPI contracts, and bundled clients retain their existing
shapes.

The live-provider harness parses `LIVE_ENV_FILE` as dotenv data without shell
execution. It clears every catalog provider field before it loads the file.
The file is the only provider-value source when it is set. The harness then
discovers selected provider keys. A paid run starts a disposable
management database with ephemeral encryption/session material, creates one
local managed tenant and client secret, verifies each available key through the
same provider-settings operation above, and runs that provider's smoke request
only after the verification returns a configured tenant profile. Verification payloads,
session material, and provider/proxy response bodies remain in the private
temporary directory and are removed at exit. `--write-config` and `--preflight`
retain the non-paid managed-only configuration and make no provider
verification or upstream call. The preflight creates a disposable managed
user, tenant, and client key, then proves authenticated routing. Each mode allocates a fresh loopback
port unless `LLM_PROXY_LIVE_PORT` explicitly provides one, and cleanup
terminates only the proxy child started by the harness rather than a process
discovered through a shared port.

Local Compose live acceptance uses the current `docker-compose.local.yml`
contract. Each project name starts with `llm-proxy-live-test-` and has a unique
random suffix. The suffix prevents reuse of containers, networks, or volumes
from an earlier run. Each run uses allocated loopback ports, builds the API
image from the current checkout, and creates new management and TAuth volumes.
The runner verifies the complete local readiness contract before provider
configuration. It gives the harness only the existing
allocated API origin and its scoped management environment. The
harness accepts only an HTTP `127.0.0.1` origin with an explicit port. It does
not start a host proxy process in this mode. Each text smoke uses canonical
`POST /v2`. Cleanup removes the complete test Compose project and its volumes
after success or failure. Paid local Compose acceptance remains outside
`make ci` and does not provide production acceptance.

The explicit `--media` mode selects OpenAI, Anthropic, Gemini, Moonshot, and
xAI by default. It verifies all selected provider keys before image requests.
Each case selects its exact image model from the validated public provider
catalog. It uses the verified model when that route supports image input.
Otherwise, it requires one exact provider image route. Key verification uses
the selected image model. Each request sends the same deterministic inline PNG
through canonical `POST /v2`. The case requires HTTP `200`, the exact response
marker, and one valid proxy request identifier. The paid media mode remains
outside `make ci`. `LLM_PROXY_LIVE_ALL_MODELS=true` expands media mode to
every image route for each selected provider, including all four Kimi routes.

`make live-test` is deliberately a different boundary. It calls only the
production API origin. It requires `LLM_PROXY_SECRET`, the Default tenant
client secret, and `LLM_PROXY_EXPECTED_TENANT_ID`, the exact Default tenant
identifier. It never loads a dotenv file or local provider credential. The
command first calls authenticated `GET /v2/identity`. It compares the returned
server-resolved tenant identifier to the expected tenant identifier. A
missing, rejected, unreachable, or unexpected tenant stops the command before
any paid provider call. The command does not store the client secret in a
temporary file. It then uses canonical `POST /v2` calls with explicit
OpenAI, Anthropic, Meta, Gemini, and Moonshot providers. Requests have no
explicit model, so managed production provider settings remain authoritative.
Five echo-marker requests verify those routes. Matching deterministic requests
larger than 16 KiB target OpenAI, Anthropic, Meta, and Gemini. Each request uses
a 900-second budget and requires a normalized line for each portfolio record
before the final marker. The Gemini long case selects
`gemini-3.5-flash`, while the Gemini echo retains the saved provider model.
OpenAI and Gemini 3.5 keep one blocking request open while the proxy owns their
resource polling. Anthropic and Meta exercise their canonical synchronous completion paths, including shared
output-continuation work when needed. The client validates the final marker,
status, resolved timeout header, and proxy request-id header. Each HTTP result
prints the validated request ID for structured-log correlation. A transport
failure without a response prints none. This paid check remains outside
`make ci`, runs all nine cases even after an earlier failure, and never prints
a tenant secret or response body.

Startup validates the mandatory management configuration and provider catalog.
Catalog validation rejects invalid identities,
references, route pairs, defaults, and capabilities. Obsolete
`management.enabled`, top-level `tenants`, and provider `api_key` fields are
unknown configuration keys. Managed tenants own client keys, provider
credentials, and routing defaults in the database.

The repository owns the permanent versionless production declaration in
`.mprlab/deploy/resources.yml`. The repository also owns the standard
`make release`, `make publish`, and `make deploy` entrypoints. These targets use
the exact `../mprlab-gateway` sibling. The gateway owns SemVer selection, schema
validation, artifact sealing, publication, Ansible inventory, reconciliation,
and production verification. The selected transaction reads only this
application checkout and the references in its typed resources. It does not
inspect unrelated repositories. Publication and deployment use the exact
sealed release. They do not run CI or build artifacts again.

The ignored `.mprlab/deploy/.env` file is the selected application's private
deployment input. The manifest binds exact dotenv keys to closed private
outputs. Deployment generates one service environment from private, TAuth,
Caddy, Pages, and capability outputs. Release and publication do not read the
private input.

The management UI is served as a static GitHub Pages app from `site/` on `https://llm-proxy.mprlab.com`; the Go backend does not serve management HTML or assets. The backend owns the secret-free `/api/public/capabilities` REST resource and the public browser config file at `/config-ui.yaml`; credentialed config CORS remains restricted to `management.public_origin`. The declared `docker/pages/Dockerfile` starts the backend's public-capabilities-only surface during the build, then the frontend-owned Node renderer fetches that resource and validates the static Pages archive. The final Pages image contains only static files. The gateway publishes that immutable archive without changing the live site and deploy activates it on `gh-pages` after the backend rollout.

The static app owns the canonical MPR UI contract: API-served `config-ui.yaml`, literal `mpr-ui@latest` assets, `mpr-ui-config.js`, `<mpr-header data-config-url="...">`, the `@latest` bundle marker, `<mpr-user>`, and `<mpr-footer>`. The config declares the current `/auth/session` path and keeps login-button presentation in static MPR UI markup; obsolete `authButton` payloads are not emitted. The Pages artifact contains no static `config-ui.yaml` or `llm-proxy-config.json`; the declared Pages container writes the canonical `https://llm-proxy-api.mprlab.com/config-ui.yaml` URL into the declarative header attribute, and `mpr-ui-config.js` applies that single backend-served YAML before loading the bundle.

The shared bundle registers `mpr-legal-document`; P005 remains the sole owner of legal-page routes and document rendering.

MPR UI is the sole browser authentication authority. Application JavaScript listens to documented `mpr-ui:auth:authenticated` and `mpr-ui:auth:unauthenticated` events, uses the documented header `data-mpr-auth-status` only to reconcile lifecycle state that settled before application startup, and never reads TAuth cookies, storage, tokens, claims, or private MPR UI DOM state. The app makes no protected management request until MPR UI reports `authenticated`; after that boundary, a management API failure is an app error and never an application-owned authentication downgrade. The YAML points browser management API calls, generated proxy examples, and MPR UI/TAuth at the configured origins.

DNS must leave `llm-proxy.mprlab.com` pointed at GitHub Pages and point `llm-proxy-api.mprlab.com` at the MPR gateway; the gateway route for `llm-proxy.mprlab.com` must be removed or moved so the backend only owns the API hostname. Management APIs under `/api/management` validate the configured TAuth session cookie locally with issuer `tauth` unless `management.jwt_issuer` overrides it.

Provider connection values are accepted only through authenticated,
tenant-scoped management endpoints. The service verifies each new credential
with its exact provider transport and selected text model. It encrypts every
secret provider field with `management.provider_key_encryption_key`.

Normal responses return masked secret state. The owner can reveal one secret
through
`POST /api/management/tenants/:tenant_id/provider-connections/:provider/fields/:field/reveal`.
That action requires the configured management origin and returns
`Cache-Control: no-store`.

Provider profile records store the selected text model and provider system
prompt. Provider connection records store only catalog field values. Current
schema reads use only these two generic record types.

Management requires `management.database_path` and
`management.provider_key_encryption_key`. Persistence uses the pure-Go GORM
SQLite driver, so `CGO_ENABLED=0` release builds remain valid. Exact
`management.admin_emails` entries configure administrators. Public config
populates those entries from the plural
`${LLM_PROXY_MANAGEMENT_ADMIN_EMAILS}` YAML flow sequence placeholder. Thus,
personal admin addresses stay out of the repository and multiple admins remain
possible. An admin session gets `user.is_admin: true`, an `Admin` avatar-menu
item, and access to `GET /api/management/admin/users`. The operation returns
all managed users' tenant facts and 30-day usage summaries. It excludes
provider API keys, masked key values, generated secrets, secret digests,
prompts, responses, audio names, and transcripts. Non-admin sessions receive
`403` from admin APIs. The packaged management config uses strict expandable
`LLM_PROXY_MANAGEMENT_*` placeholders. Local and hosted profiles must define
explicit values in the runtime environment or `configs/.env`.

Account state, tenant names, enabled providers, defaults, and generated-secret digests are persisted through GORM and are never stored by mutating the runtime config file. One TAuth subject owns one or more personal tenants; there is no membership, invitation, role, or shared-tenant model. F014 upgrades the previous one-row-per-user shape as one startup transaction: it preflights all legacy tenant, provider, and usage rows; rejects unclaimed static owners, duplicates, invalid routing or secrets, plaintext/corrupt keys, and orphan rows; renames the two colliding legacy GORM indexes and the old tables; creates explicit user and tenant records; preserves opaque tenant ids and usage; rebinds provider-key encryption to tenant ids; verifies counts, values, and decryption; writes schema version 1; and drops only its bounded legacy tables. Every failed stage rolls back to the untouched legacy schema and original index names and prevents startup.

Schema version 2 gives every managed usage event one nonblank outcome code chosen at the request/error boundary: `success`, `invalid_request`, `payload_too_large`, `rate_limited`, `service_unavailable`, `request_timeout`, or `upstream_error`. The bounded upgrade maps historical successful rows to `success` and exact `400`, `413`, `429`, `499`, `502`, `503`, and `504` statuses to their canonical failure codes before adding the tenant/success/time/id page index; caller cancellation `499` and proxy-budget expiry `504` both become `request_timeout`. An unsupported historical status rejects startup before mutation. Neither current recording nor migration persists or reconstructs raw provider bodies or free-form error messages.

Managed usage persistence is deliberately asynchronous and bounded. After the selected proxy response is written and flushed, the request handler constructs one immutable content-free usage record and performs a non-blocking send to the management runtime's FIFO channel. `management.usage_queue_size` is positive and defaults to `1024`; it is independent from the upstream `server.queue_size`. The first accepted event starts the runtime's sole usage writer goroutine. That writer serializes accepted usage inserts in FIFO order, attempts each insert once under the existing detached five-second budget, and never creates per-event goroutines or retries. A dedicated context-aware database-write gate sequences usage inserts with management mutations without taking the management mutation mutex. Authentication bypasses both and remains an independent caller-scoped read transaction; runtime SQLite WAL journaling and the bounded busy timeout allow that reader to proceed alongside the writer.

If the usage channel is full, the newest event is dropped while previously accepted events remain queued, the public response is unchanged, and one safe warning carries the stable `managed_usage_queue_full` error. Accepted entries are process-local at-most-once work until their insert commits: queued entries are not crash-durable, and an insert failure or process exit can lose uncommitted telemetry. Usage summaries are therefore eventually consistent with completed responses. This contract is appropriate only because managed usage is operational telemetry, not billing, accounting, or a provider-job ledger. Queued records and their warnings exclude prompts, responses, audio, transcripts, tenant secrets, provider keys, raw provider bodies, and free-form upstream errors.

Schema version 3 makes saved tenant keys the hard eligibility boundary for
managed routing defaults. Its bounded upgrade decrypts and validates every
provider record, preserves eligible defaults, deterministically replaces
ineligible pairs, clears text or dictation when no provider is eligible,
preserves tenant timestamps, verifies the result, and records version 3 in one
transaction. Current-version startup rejects any later drift.

Schema version 4 removes retired `qwencloud` settings and reconciles affected
text defaults against remaining keyed providers. It clears text and reasoning
when no key remains. It preserves tenant timestamps and historical usage
provider and model identifiers. The same transaction verifies the result and
records version 4. Current startup rejects retired settings and defaults.

Schema version 5 converts affected provider-native model values to canonical
exact model identifiers. It preserves tenant timestamps and historical usage
records. Current startup rejects invalid canonical routes.

Server and management settings remain in `config.yml`. Provider definitions,
static endpoints, exact models, and offerings remain in `providers.yml`.
Database access uses GORM model APIs without raw SQL. Generated client secrets
are returned once and stored as SHA-256 digests.

The shared header has one application-owned notification region followed by the MPR-owned identity control in the `aux` slot. Scoped flex ordering keeps every visible notice immediately left of the avatar or Sign in control, and the application clears each notice after 10 seconds; MPR UI remains the only owner of sign-in, session, and avatar-menu behavior.

On startup, the `mpr-ui@latest` shell restores the browser session through TAuth `/auth/session` and reports the resulting lifecycle state before LLM Proxy requests protected account data. The anonymous-only `/` landing route reacts to the documented authenticated lifecycle by replacing itself with `/app/`; it does not inspect TAuth state or issue an authentication request. A valid refresh cookie rotates into a new access cookie without exposing the signed-out panel. Ordinary reloads never clear either cookie. The explicit user-menu **Sign out** action is the only application flow that calls `/auth/logout` and clears the session.

The LLM Proxy application startup guard covers the complete versioned first-party module graph and the pinned Alpine module. A module-link failure or browser-side rejection of the one canonical CDN dependency renders the application-owned recovery surface, completes `llm-proxy:management-ready`, and makes no protected management request; it never tries a mirror, bundled copy, retry, or alternate authentication path. The local ghttp surface sends `Cache-Control: no-store`, while one bounded module-graph revision evicts files cached before that policy. MPR UI continues to own the authenticated session and shared transition, while LLM Proxy owns whether its application runtime mounted successfully.

The backend consumes TAuth's published Go `pkg/sessionvalidator` for cookie/JWT validation and adds only llm-proxy's tenant, required-expiry, and principal invariants; no application-owned JWT parser or claims schema exists. The deployment manifest declares the tenant and route as data. The sibling gateway's Ansible transaction stages the selected service inputs and reconciles the declared TAuth capability and Caddy route before Pages activation; application runtime code contains no TAuth or Caddy deployment orchestration.

The authenticated management landing view is usage-focused. The browser first loads `GET /api/management/account`; every returned tenant remains operational through its own generated secret, and the browser has no global active-tenant or URL/history selection contract. The independent `Usage tenant` control defaults to `All tenants` immediately before the ordered `ALL`, `30 days`, `7 days`, and `1 day` controls, while the interval defaults to `30 days`. The all-tenant selection calls `GET /api/management/usage?interval=all|30d|7d|1d`; an explicit tenant calls `GET /api/management/tenants/:tenant_id/usage?interval=all|30d|7d|1d`. Both operations require exactly one recognized interval, return `400` for missing, repeated, or unknown values, and carry `Cache-Control: no-store`. The response is the current `interval`, `bucket_unit`, aggregate `totals`, ordered generic `buckets`, and provider, model, and status-code usage for the selected scope; it contains no fixed-period `period_days` or `daily` fields. `1d` uses 24 hourly buckets, `7d` and `30d` use exact trailing-duration daily buckets, and `all` uses UTC daily buckets from the earliest retained event through one captured server timestamp. An empty all-time result has no buckets. Account-wide aggregation runs once at the database boundary across every owned tenant and calculates totals and average latency from the complete event set; the browser never fans out per-tenant summaries. Refresh and interval changes retain the Usage tenant selection, Settings changes do not affect it, loading disables the Usage controls, and request identity prevents a stale scope or interval response from replacing the selected snapshot. The admin API remains a distinct 30-day daily contract. Managed proxy requests record endpoint, provider, model, status, success flag, latency, and normalized token counts only; prompts, audio, transcripts, responses, tenant secrets, and provider API keys are excluded from usage events. Tenant lifecycle, client access, generated secrets, routing defaults, copyable request examples, and provider key controls live in the Settings modal opened from the shared `<mpr-user>` avatar dropdown, where the `Settings` item is inserted before `Sign out`. One compact `Tenant access` row combines the `Tenant` selector, modal Rename, client-key state and one-time reveal/copy actions, confirmed Replace key, confirmed Delete tenant, and Create tenant. The selected tenant is only the Settings editor context. Switching that selector with an unsaved draft requires explicit discard confirmation, clears any raw one-time secret or revealed provider key from browser state, and never changes the Usage tenant. The routing-default form lists only providers with saved tenant keys; its dictation controls are disabled and show `Not configured` when none of those providers supports dictation. It keeps Text provider, Text model, and the selected model's Reasoning effort control on one desktop row, clears an incompatible effort on a model change, exposes `Not supported` when the route has no reasoning capability, and autosaves provider/model/effort selections immediately plus the tenant system prompt on field exit. Tenant-wide and provider-specific system-prompt fields start collapsed behind semantic `System prompt` disclosures with a visible `Hidden` indicator; they expand through pointer or keyboard activation and collapse again when Settings opens or their tenant/provider context changes. Settings serializes every mutation that returns a complete tenant profile and locks its controls while a close request waits for in-flight work. A client key created or replaced during that wait keeps Settings open for an explicit later close so its one-time value remains available to copy; removing the last provider key re-enforces mandatory setup; client keys can only be replaced or removed with their owning non-final tenant. Failed edits remain available for retry. Request examples include copyable default text and v2 commands only when a keyed text default exists, and a default dictation command only when a keyed dictation default exists, plus copyable selected-provider text and v2 commands; dictation-capable selected providers also show a provider-specific dictation command. Provider key controls use one selected-provider editor with API key, text model, and system prompt fields because those settings are part of the provider-owned managed routing contract.

When the selected Usage snapshot contains failures, the success-rate card exposes an **N failed requests** action. Its semantic dialog keeps the selected interval and the summary's non-success status breakdown. `GET /api/management/usage/failures` pages newest-first failures across all owned tenants and adds each row's safe tenant id and current name; `GET /api/management/tenants/:tenant_id/usage/failures` retains the tenant-less row shape for one explicitly selected owned tenant. Both operations require one `interval`, accept one `limit` from 1 through 100 and one opaque `cursor`, return `Cache-Control: no-store`, and paginate against one opaque snapshot using stable event-time/id order. Each cursor is bound to its exact all-tenant or tenant scope and is rejected in every other scope. Apart from the account-wide tenant context, rows contain only event time, endpoint, provider, model, status, canonical outcome code, and latency. Dialog load failures stay local; Usage tenant or interval changes abort and invalidate the request; the admin surface remains aggregate-only.

## Error Contract

- `400`: unknown provider, unknown model, unsupported capability, unsupported endpoint, conflicting model parameters, or client-supplied provider API key fields on public proxy requests.
- `403`: missing or invalid client `key`.
- `413`: compatibility prompt, dictation audio, tenant asset, or selected
  provider media limit exceeded. Provider media failures use the stable
  `provider_media_limit_exceeded` JSON code.
- `429`: upstream provider rate limiting.
- `503`: registered non-default provider credential is unavailable, so the selected provider is disabled until its API key is configured.
- `504`: the overall proxy request timed out before the selected upstream provider returned a final result.
- `502`: upstream provider failure, including non-budget OpenAI incomplete
  reasons, Chat Completions reasons other than `stop` or `length`, Gemini
  reasons other than `STOP` or `MAX_TOKENS`, and Anthropic reasons other than
  `end_turn`, `stop_sequence`, or `max_tokens`; partial provider text is never
  returned.

Provider-originated `429` and `502` responses use the canonical six-field JSON
envelope documented in `docs/openapi.yaml`: error code, canonical provider,
nullable exact upstream status, retryability, proxy-owned request id, and
nullable validated `Retry-After`. The public proxy status and upstream status
remain distinct. Raw provider bodies, messages, and partial output never enter
that response or structured provider-failure logs.

## Implementation Notes

- Provider/model validation happens at the HTTP edge through a registry built from the provider catalog.
- OpenAI selects Responses, Models, and dictation endpoints from catalog-defined provider transports. The Responses adapter polls documented pending states.
- Non-OpenAI compatible text providers use a shared Chat Completions adapter. It normalizes `finish_reason=length` for shared continuation and requires `finish_reason=stop` to complete content or reasoning text.
- Meta uses the shared OpenAI-compatible Chat Completions adapter and its catalog-defined endpoint. Its proxy contract is text-only.
- Anthropic uses a native Messages adapter. It maps `system` messages to the top-level `system` parameter. It maps other messages to `messages[]`.
- Anthropic sends declared image inputs as ordered base64 content blocks. `max_tokens` continues through the shared coordinator.
- Gemini uses a native Interactions adapter against its catalog-defined
  endpoint. `incomplete` continues through the shared
  coordinator as a new interaction. `completed` with visible model text
  succeeds. Gemini 3.x uses stored background polling. For exact models whose catalog declares the
  capability, ordered image and audio attachments become typed interaction
  content after the message text. The adapter selects inline `data` or Files
  API `uri` content from the exact provider offering limits.
- xAI uses the shared Chat Completions adapter for text-only models. The `grok-4.5` image route uses synchronous Responses.
- OpenAI-compatible chat providers receive validated and sorted `messages[]` as provider-supported `role` and `content` items.
- OpenAI Responses payload shape comes from the selected configured model's stable `request_profile`; model-specific web-search support comes from the selected model catalog entry. OpenAI Responses text calls run in background mode with stored responses so long provider work can be polled by llm-proxy while the caller waits on one REST request.
- Gemini receives user messages as `user_input` steps and system messages as
  `system_instruction`. The resolved route rejects each assistant message.
- OpenAI-compatible Chat Completions adapters remain text-only and reject media declarations at startup.
- OpenAI Responses receives text-only single prompts unchanged. Requests with images use role-preserving typed content blocks.
- Dictation routing uses catalog-defined transports and the multipart transcription adapter. OpenAI, SiliconFlow, and Z.AI send a multipart `model` field. xAI STT omits it.
- Response formatting keeps existing text/XML/CSV bodies and existing JSON `request`, `response`, and normalized `usage` fields. JSON responses also include OpenRouter-style `object`, `model`, and `choices[].message.content` metadata, plus caller-visible request `messages` with provided `order` values. Server-injected tenant default system prompts are sent upstream but not echoed in response metadata.

## Test Strategy

Black-box router tests cover:

- OpenAI omitted-provider regression.
- OpenAI output-budget recovery through the public `POST /v2` handler,
  including pending-response snapshots, repeated suffix attempts, exact text
  assembly, aggregated usage, and one successful managed event.
- OpenAI polling only for documented pending states, with missing and unknown
  states rejected without another provider call.
- Each transport with continuation actions through its public text route. These
  tests prove transcript order, suffix assembly, and usage aggregation.
- Gemini incomplete responses through the public text route. These tests prove
  one create request, interaction deletion, and no partial-text response.
- Ordered images through OpenAI Responses, Anthropic Messages, xAI Responses,
  and Gemini Interactions through canonical `POST /v2`.
- Ordered Gemini audio through inline `data` or Files API `uri`, with no media echo in response metadata.
- Inline and asset-backed media admission, exact-limit and one-unit-above
  provider boundaries, tenant isolation, asset expiry and deletion, and
  provider file cleanup.
- Malformed, digest-mismatched, misplaced, unsupported-model, and
  compatibility-route media rejection before any upstream request.
- Deadline exhaustion and nonrecoverable safety, refusal, tool, malformed,
  missing, and unknown signals, proving partial text is never exposed as a
  failure response.
- Explicit Meta Muse Spark 1.1 and 1.2 routing through `GET /`, compatibility `POST /`, and canonical `POST /v2`.
- Unsupported Meta `web_search` and dictation paths.
- Explicit DeepSeek chat-completions routing.
- Unsupported `web_search` for DeepSeek.
- Known provider without credential.
- Invalid default dictation provider configuration.
- Conflicting JSON body/query models.
- SiliconFlow dictation routing.
- Configured text model routing without code changes.
- Invalid configured model catalog startup failures, including noncanonical,
  duplicate, and adapter-incompatible `media_inputs` or `media_limits`.
- Existing OpenAI dictation and response-format tests.
