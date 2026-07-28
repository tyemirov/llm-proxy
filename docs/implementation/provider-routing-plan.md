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
- Canonical JSON `POST /v2` accepts only `messages[]` as the text input shape; request-body `prompt` and `system_prompt` are invalid.
- `messages[]` items contain `role` and string `content`. Supported roles are `system`, `user`, and `assistant`; at least one `user` message is required.
- `messages[].order` is optional. When any submitted message includes `order`, every submitted message must include a unique non-negative integer `order`; the proxy sorts submitted messages by ascending `order` before adding a request or tenant system prompt and before routing upstream.
- With `messages[]` on `POST /`, body `system_prompt` is prepended as a system message only when the transcript does not already contain a `system` message. A body containing both `system_prompt` and a system message is invalid. With `POST /v2`, callers send system instructions as `system` role messages.
- `max_tokens` is an optional positive integer on `GET /` query strings and JSON `POST /` bodies. It is the initial per-attempt output budget and is reused for missing-suffix attempts.
- Provided `max_tokens` maps to OpenAI Responses `max_output_tokens`, Meta, Moonshot, and MiniMax Chat Completions `max_completion_tokens`, other OpenAI-compatible chat completions `max_tokens`, Anthropic Messages `max_tokens`, and Gemini `generationConfig.maxOutputTokens`.
- Omitted `max_tokens` means the proxy omits provider max-token fields and lets the selected provider/model default apply, except Anthropic Messages where the upstream API requires `max_tokens` and the proxy sends the selected model's configured synchronous output limit. After an output-budget stop with no visible progress, a configured model output limit becomes the generic ceiling for increasing the next attempt.
- Known provider-specific output-token ceilings are validated before upstream calls; MiniMax M2.7 rejects `max_tokens` above `2048`, Gemini text models reject values above `65536`, and Claude models reject values above their configured synchronous Messages output limits with `400 Bad Request`.
- `reasoning_effort` is optional on `GET /` as a query parameter and on JSON `POST /` and `POST /v2` as a body field. Omission retains the resolved tenant default. A supplied value must be nonblank and supported by the exact resolved text provider/model route; blank, `null`, or unsupported values return `400 Bad Request` before a provider call.
- `X-LLM-Proxy-Request-Timeout-Seconds` is an optional positive whole-number
  header on `GET /`, `POST /`, `POST /v2`, and `POST /dictate`. Omission uses
  `server.request_timeout_seconds`; a supplied value must be in the inclusive
  range `1..server.max_request_timeout_seconds`.
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
- JSON `POST /v2` bodies that provide `prompt`, body `system_prompt`, missing or empty messages, unsupported roles, empty content, a missing user message, partially specified `order`, duplicate `order`, negative `order`, or unknown JSON fields return `400 Bad Request`.
- Upstream provider API keys are never accepted from client requests.

## Providers

| Provider | Aliases | Text | Dictation | Web Search |
|----------|---------|------|-----------|------------|
| `openai` | none | OpenAI Responses API | OpenAI audio transcription | Supported by configured OpenAI model entries with `web_search: true` |
| `meta` | none | Meta Model API OpenAI-compatible chat completions | Not supported | Not supported |
| `deepseek` | none | OpenAI-compatible chat completions | Not supported | Not supported |
| `dashscope` | `qwen` | OpenAI-compatible chat completions | Not supported | Not supported |
| `qwencloud` | none | Qwen Cloud Token Plan OpenAI-compatible chat completions | Not supported | Not supported |
| `moonshot` | `kimi` | OpenAI-compatible chat completions | Not supported | Not supported |
| `minimax` | none | MiniMax OpenAI-compatible chat completions | Not supported | Not supported |
| `siliconflow` | none | OpenAI-compatible chat completions | OpenAI-compatible audio transcription | Not supported |
| `zhipu` | `glm` | OpenAI-compatible chat completions | Z.AI GLM-ASR transcription | Not supported |
| `gemini` | none | Native Gemini generateContent | Not supported | Not supported |
| `anthropic` | `claude` | Native Anthropic Messages | Not supported | Not supported |
| `grok` | `xai` | xAI OpenAI-compatible chat completions | xAI STT | Not supported |

This matrix describes capabilities wired through `llm-proxy`. Upstream products
can expose speech APIs that are not yet proxy adapters; do not mark them
supported for `/dictate` until the provider registry and black-box routing tests
cover that path.

The canonical Meta contract uses selector `meta` with no aliases,
`https://api.meta.ai/v1`, `${MODEL_API_KEY}`, and model
`muse-spark-1.1`. llm-proxy sends Meta requests only through the shared
OpenAI-compatible Chat Completions adapter on the text endpoints and maps the
public `max_tokens` input to Meta's current `max_completion_tokens` field. It
does not expose Meta dictation, `web_search`, tools, multimodal inputs, or a Responses
API fallback. Meta documents Muse Spark 1.1 as a public preview for U.S.
developers with a 1,048,576-token context window. Current upstream details live
in Meta's [Muse Spark guide](https://developer.meta.com/ai/resources/blog/build-with-muse-spark/),
[model reference](https://dev.meta.ai/docs/getting-started/models),
[Chat Completions reference](https://dev.meta.ai/docs/features/chat-completion),
and [pricing and rate-limit documentation](https://dev.meta.ai/docs/getting-started/pricing-rate-limits).

Qwen Cloud Token Plan is a distinct text-only provider: use canonical selector
`qwencloud`, model `qwen3.8-max-preview`, its Token Plan endpoint
`https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1`, and
the dedicated `${QWEN_CLOUD_TOKEN_PLAN_API_KEY}` credential. It has no alias;
the existing `qwen` alias remains DashScope-only because the two services do
not share API keys or base URLs. Qwen Cloud requests retain the shared
compatible `max_tokens` field and do not add Qwen-specific thinking, tools, or
multimodal controls.

MiniMax is a distinct text-only provider with canonical selector `minimax`,
model `MiniMax-M2.7`, endpoint `https://api.minimax.io/v1`, and
`${MINIMAX_API_KEY}`. The shared adapter maps public `max_tokens` to MiniMax
`max_completion_tokens`; its configured `2048` output limit is rejected at the
proxy edge before an upstream call. The proxy does not add MiniMax-specific
reasoning, tools, streaming, or multimodal controls.

## Configuration

Runtime service configuration comes from `config.yml`; env vars and `.env`
files are interpolation inputs only for `${NAME}` placeholders in that YAML.
The loader rejects unknown keys and missing placeholders before the proxy
starts, except when a provider `api_key` value is exactly one missing
placeholder. That exact missing provider credential expands to an empty string
so non-default providers can stay disabled; missing placeholders in other
fields or embedded inside longer values fail startup.

Shared config fields:

- `server.port`
- `server.log_level`
- `server.workers`
- `server.queue_size`
- `server.request_timeout_seconds`
- `server.max_request_timeout_seconds`
- `server.max_prompt_bytes`
- `server.max_input_audio_bytes`
- `server.upstream_rate_limits[].origin`
- `server.upstream_rate_limits[].max_requests`
- `server.upstream_rate_limits[].interval`
- `management.enabled`
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
- `tenants[].id`
- `tenants[].secret`
- `tenants[].defaults.provider`
- `tenants[].defaults.model`
- `tenants[].defaults.dictation_provider`
- `tenants[].defaults.dictation_model`
- `tenants[].defaults.system_prompt`
- `tenants[].defaults.reasoning_effort`

Provider credentials and base URLs:

- `providers.openai.api_key`, `providers.openai.base_url`, `providers.openai.transcriptions_url`
- `providers.meta.api_key`, `providers.meta.base_url`
- `providers.deepseek.api_key`, `providers.deepseek.base_url`
- `providers.dashscope.api_key`, `providers.dashscope.base_url`
- `providers.qwencloud.api_key`, `providers.qwencloud.base_url`
- `providers.moonshot.api_key`, `providers.moonshot.base_url`
- `providers.minimax.api_key`, `providers.minimax.base_url`
- `providers.siliconflow.api_key`, `providers.siliconflow.base_url`, `providers.siliconflow.transcriptions_url`
- `providers.zhipu.api_key`, `providers.zhipu.base_url`, `providers.zhipu.transcriptions_url`
- `providers.gemini.api_key`, `providers.gemini.base_url`
- `providers.anthropic.api_key`, `providers.anthropic.base_url`
- `providers.grok.api_key`, `providers.grok.base_url`, `providers.grok.transcriptions_url`

Provider model catalogs:

- `providers.<provider>.text.default_model`
- `providers.<provider>.text.models[].id`
- `providers.<provider>.text.models[].output_token_limit`
- `providers.<provider>.text.models[].reasoning_effort`
- `providers.openai.text.models[].request_profile`
- `providers.openai.text.models[].web_search`
- `providers.openai.dictation.default_model`
- `providers.openai.dictation.models[].id`
- `providers.siliconflow.dictation.default_model`
- `providers.siliconflow.dictation.models[].id`
- `providers.zhipu.dictation.default_model`
- `providers.zhipu.dictation.models[].id`
- `providers.grok.dictation.default_model`
- `providers.grok.dictation.models[].id`

The model catalog is runtime config data. Code owns provider selectors,
aliases, transports, endpoint shapes, and stable OpenAI request-profile
implementations. `config.yml` owns provider model ids, provider default models,
dictation model ids, model-specific web-search enablement, provider/model
reasoning-effort capability declarations, and known provider-side output-token
limits.

The README model-capability table mirrors `config.yml`; refresh those two
catalog representations together and do not hardcode model ids in provider
transports. Moonshot's current Kimi Chat Completions route receives
`max_completion_tokens` when a caller supplies the proxy `max_tokens` value.
It deliberately omits sampling controls because Kimi K3 fixes those values
upstream.
Qwen Cloud Token Plan is separate from DashScope at both the selector and
credential boundary. MiniMax M2.7 maps public `max_tokens` to
`max_completion_tokens` and carries a configured 2048-token output ceiling.
GLM-5.2 remains on the existing BigModel/Zhipu Chat Completions endpoint. Its
128K output maximum is catalog metadata; optional `thinking` and
provider-native `reasoning_effort` controls are not exposed directly. The
canonical proxy `reasoning_effort` request field is validated against the exact
resolved provider/model capability and translated only through its configured
adapter. A supplied value must be nonblank and exact and takes precedence over
the tenant routing default; an omitted value uses that default. The current
mapping is the OpenAI Responses reasoning adapter, so a blank or unsupported
effort fails closed for GLM and generic OpenAI-compatible routes instead of
being ignored or leaked downstream.

OpenAI `request_profile` values select stable payload shapes:

- `openai_responses_temperature`
- `openai_responses_temperature_tools`
- `openai_responses_reasoning_tools`

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

One completion coordinator owns output-budget recovery for all transports.
Adapters normalize only their exact recoverable signal: OpenAI
`incomplete/max_output_tokens`, Chat Completions `length`, Gemini
`MAX_TOKENS`, and Anthropic `max_tokens`. The coordinator preserves the
original messages, appends all accumulated assistant text plus one
missing-suffix user instruction, repeats the selected provider call, and joins
the returned suffixes until OpenAI reports `completed`, Chat reports `stop`,
Gemini reports `STOP`, or Anthropic reports `end_turn`/`stop_sequence`. The
overall request deadline is the hard bound. Safety/filter, refusal,
tool/intermediate, failed/cancelled, malformed, missing, and unknown signals
return the canonical failure without exposing partial text. Provider-specific
deferred, batch, and asynchronous APIs remain separate transports; an
arbitrary response `id` never activates polling.

The caller's `max_tokens` value is the initial per-attempt budget and is reused
for suffix attempts. When an output-budget response has no visible text and
the model catalog declares an output limit, the next attempt increases the
budget toward that limit. Each provider call independently passes through the
shared worker, queue, and upstream-rate-limit controls; the coordinator's wait
does not retain a worker. Distinct attempt usage is summed into one final
managed event, while successive OpenAI snapshots for one response id replace
one another. A recovered request records one success and no failure; a request
deadline records one `504` with all usage observed before the deadline.

Bundled clients intentionally expose only the canonical `POST /v2` text
contract. The installable Go CLI maps prompt flags or stdin into v2 `system` and
`user` messages, while the reusable Go and Python packages expose only
messages-request constructors and `PostMessages`/`post_messages` send methods.
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
REST callers.

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

Every nonempty `api_key` submitted to
`PUT /api/management/tenants/:tenant_id/provider-keys/:provider` is an
unverified new or replacement credential. After ownership, provider, and exact
selected text-model resolution, the handler performs one fixed,
non-user-content inference through the provider's canonical text transport:

- OpenAI Responses uses one synchronous, non-stored `POST /responses` with
  `background: false`, `store: false`, and a 16-token output limit.
- DeepSeek, DashScope, Qwen Cloud, Moonshot, MiniMax, SiliconFlow, Zhipu, Meta,
  and Grok use one authenticated OpenAI-compatible `POST /chat/completions`
  with the provider's declared token-limit parameter.
- Gemini uses one `generateContent` request with `x-goog-api-key` and
  `maxOutputTokens: 16`.
- Anthropic uses one Messages request with `x-api-key`,
  `anthropic-version: 2023-06-01`, and `max_tokens: 16`.

The verifier uses the management request context and the same shared upstream
worker, queue, and origin-rate-limit boundary as proxy traffic. It performs no
retry, alternate endpoint, polling, continuation, background work, or managed
usage recording. A transport success counts only when its canonical response
envelope is structurally valid. Provider `4xx` credential/model rejection maps
to `422 provider_key_rejected` except `408` timeout and `429` rate limit;
upstream `504` also maps to timeout. Transport cancellation/deadline, outage,
and malformed success map to the documented provider-neutral `504`, `503`, or
`429` error. Candidate keys, fixed probe content, authenticated URLs, and raw
upstream bodies never enter logs, management responses, profiles, or usage
rows.

Only successful verification enters the existing atomic provider-key
transaction, which encrypts the credential, saves the submitted model and
system prompt, reconciles routing defaults, and returns the complete profile.
Every failure leaves a new provider unkeyed or preserves the prior verified
replacement credential, settings, and defaults unchanged. An empty `api_key`
is the canonical retain-existing-key settings update and bypasses verification.

Settings starts this operation automatically when a paste updates the selected
provider key. It announces `Verifying key`, leaves only that input available
for a newer paste, and locks tenant, provider, model, reveal, remove, routing,
and close actions until settlement. Per-candidate abort ownership plus the
existing workspace/editor versions prevent newer paste, authentication, tenant,
provider, model, or editor context changes from applying a stale result.
Success clears the raw draft and restores the masked key. A safe failure keeps
the draft only in that editor, distinguishes an unsaved first key from an active
prior key, and exposes an explicit retry.

## Managed Routing Defaults

`PUT /api/management/tenants/:tenant_id/defaults` requires every field and
accepts only complete text and dictation provider/model pairs. Each nonempty
pair must name a provider with a saved tenant key; dictation also requires that
provider's declared dictation capability. The text pair is both empty only when
the tenant has no provider key, and the dictation pair is both empty only when
none of its keyed providers supports dictation. `reasoning_effort` is explicit;
empty means unset and a nonempty value must be in the selected exact text
provider/model capability list. The handler resolves the text pair before
validating effort and constructs all defaults before the database write, so a
partial, unkeyed, unknown, unsupported, cross-provider pair or incompatible
effort fails with `400 managed_routing_defaults_invalid` and leaves the prior
defaults unchanged. The profile response exposes key eligibility through
`providers[].has_key` and capabilities through `providers[].text_models[]`; it
has no provider-level reasoning capability or global option list. A malformed
profile is a workspace-integrity failure in the browser, not a UI repair
opportunity.

Saving or removing a provider key reconciles routing defaults in the same
database transaction. An eligible current pair is preserved. Otherwise the
first keyed provider by canonical provider id becomes the text default using
its saved text model, and the first keyed dictation-capable provider becomes the
dictation default using its configured dictation model. A missing eligible
provider clears that pair and also clears reasoning effort when text is unset.

Startup requires all persisted pairs to be canonical, catalog-valid, and backed
by saved tenant keys; it never retains a fallback, compatibility read, or
runtime repair path. The bounded schema-version-3 migration applies the same
deterministic reconciliation once, preserves tenant timestamps, verifies every
row, and records the version atomically. Unknown models, corrupt keys, and
unknown or dictation-unsupported providers fail with contextual owner, tenant,
endpoint, provider, and model errors.

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

The live-provider harness parses `LIVE_ENV_FILE` as dotenv data without shell
execution and discovers selected provider keys. A paid run starts a disposable
management database with ephemeral encryption/session material, creates one
local managed tenant and client secret, verifies each available key through the
same provider-settings operation above, and runs that provider's smoke request
only after the verification returns a keyed profile. Verification payloads,
session material, and provider/proxy response bodies remain in the private
temporary directory and are removed at exit. `--write-config` and `--preflight`
retain the non-paid static-tenant configuration with management disabled and
make no verification or upstream call. Each mode allocates a fresh loopback
port unless `LLM_PROXY_LIVE_PORT` explicitly provides one, and cleanup
terminates only the proxy child started by the harness rather than a process
discovered through a shared port.

`make live-test` is deliberately a different boundary: it calls only the
production API origin with `LLM_PROXY_SECRET`, the Default tenant client
secret. It never loads a dotenv file or local provider credential. The command
uses canonical `POST /v2` calls with explicit OpenAI, Anthropic, Meta, Gemini,
and Moonshot providers and no explicit model, so managed production provider
settings remain authoritative. Five echo-marker requests verify those routes;
matching deterministic requests larger than 16 KiB target OpenAI, Anthropic,
and Meta with a 900-second request budget and a required normalized line for
each portfolio record before the final marker. OpenAI keeps one blocking request
open while the proxy owns its Responses background polling. Anthropic and Meta
exercise their canonical synchronous completion paths, including shared
output-continuation work when needed. The client validates only the final
marker, status, and resolved timeout header. This paid check remains outside
`make ci`, runs all eight cases even after an earlier failure, and never prints
a tenant secret or response body.

Startup validates configured tenants, rejects duplicate tenant ids and duplicate secrets, requires API keys for each configured static tenant's default text and dictation providers when management mode is disabled, allows non-default provider API keys to be blank so those providers are disabled until configured, requires every configured provider base URL, requires transcription URLs for dictation-capable providers, requires text model catalogs for every provider, requires dictation model catalogs for dictation-capable providers, rejects blank or duplicate model ids, rejects defaults not listed in their model catalog, rejects `web_search` outside OpenAI text model entries, validates OpenAI request profiles, validates exact model-owned reasoning-effort lists, validates each configured static tenant's default text provider/model and effort, and validates endpoint/credential support for each configured static tenant's default dictation provider/model. When `management.enabled` is false, at least one static tenant is required. When `management.enabled` is true, static tenants and nonblank config-level provider API keys are rejected because managed tokens and provider credentials are user-owned database state.

The repository owns the immutable release implementation under
`tools/gitrelease` and uses one canonical `vMAJOR.MINOR.PATCH` SemVer contract.
`make release` builds container inputs from the tracked `git archive HEAD`
snapshot, packages a Pages marker for that source commit, and creates the
changelog-only release commit. Before Pages branch mutation, `make deploy`
validates the archive, its source marker, and the remote release tag. After
activation it matches GitHub Pages build state to the pushed branch commit,
then verifies a cache-distinct public marker after backend rollout.

The management UI is served as a static GitHub Pages app from `site/` on `https://llm-proxy.mprlab.com`; the Go backend does not serve management HTML or assets. `make release` renders and validates the Pages archive locally, `make publish` uploads that immutable archive without changing the live site, and `make deploy` activates it on `gh-pages` after the backend rollout. The backend serves one public browser config file at `/config-ui.yaml` from the already-loaded management config, with credentialed CORS restricted to `management.public_origin`.

The static app owns the canonical MPR UI contract: API-served `config-ui.yaml`, literal `mpr-ui@latest` assets, `mpr-ui-config.js`, `<mpr-header data-config-url="...">`, the `@latest` bundle marker, `<mpr-user>`, and `<mpr-footer>`. The config declares the current `/auth/session` path and keeps login-button presentation in static MPR UI markup; obsolete `authButton` payloads are not emitted. The Pages artifact contains no static `config-ui.yaml` or `llm-proxy-config.json`; release rendering writes the profile-owned `PAGES_CONFIG_URL` into the declarative header attribute, and `mpr-ui-config.js` applies that single backend-served YAML before loading the bundle.

The shared bundle registers `mpr-legal-document`; P005 remains the sole owner of legal-page routes and document rendering.

MPR UI is the sole browser authentication authority. Application JavaScript listens to documented `mpr-ui:auth:authenticated` and `mpr-ui:auth:unauthenticated` events, uses the documented header `data-mpr-auth-status` only to reconcile lifecycle state that settled before application startup, and never reads TAuth cookies, storage, tokens, claims, or private MPR UI DOM state. The app makes no protected management request until MPR UI reports `authenticated`; after that boundary, a management API failure is a workspace error and never an application-owned authentication downgrade. The YAML points browser management API calls, generated proxy examples, and MPR UI/TAuth at the configured origins.

DNS must leave `llm-proxy.mprlab.com` pointed at GitHub Pages and point `llm-proxy-api.mprlab.com` at the MPR gateway; the gateway route for `llm-proxy.mprlab.com` must be removed or moved so the backend only owns the API hostname. Management APIs under `/api/management` validate the configured TAuth session cookie locally with issuer `tauth` unless `management.jwt_issuer` overrides it.

Provider API keys are accepted only through authenticated, workspace-scoped management endpoints. Every nonempty new or replacement credential is operationally verified against its exact provider and selected text model before it is encrypted at rest with AES-GCM using the required `management.provider_key_encryption_key`. Normal save, workspace-profile, and administrator responses return only masked status. The sole raw-key response is the explicit owner-authenticated `POST /api/management/tenants/:tenant_id/provider-keys/:provider/reveal` action, which requires the configured management origin and returns `Cache-Control: no-store`. Each provider-key record also stores that provider's selected text model and provider-specific system prompt. Managed text requests that select a provider and omit `model` use the saved provider text model, and provider-selected requests without request-level system instructions use the saved provider system prompt. The bounded F014 migration rejects plaintext or corrupt legacy rows and re-encrypts valid keys with the preserved opaque tenant id as AES-GCM associated data. This is an encrypted-at-rest guarantee for database dumps, backups, and direct storage access, not a user-only decryption or zero-knowledge guarantee.

Management mode requires `management.database_path` and `management.provider_key_encryption_key`; management persistence uses the pure-Go GORM SQLite driver so `CGO_ENABLED=0` release builds remain valid. Administrators are config-owned through exact `management.admin_emails` entries; public config populates those entries from the plural `${LLM_PROXY_MANAGEMENT_ADMIN_EMAILS}` YAML flow sequence placeholder so personal admin addresses stay out of the repository while allowing multiple admins. An admin session gets `user.is_admin: true`, an `Admin` avatar-menu item, and `GET /api/management/admin/users` access to all managed users' tenant facts and 30-day usage summaries without provider API keys, masked key values, generated secrets, secret digests, prompts, responses, audio names, or transcripts. Non-admin sessions receive `403` from admin APIs. The packaged management config uses strict expandable `LLM_PROXY_MANAGEMENT_*` placeholders, so local and hosted profiles must define explicit values in the runtime environment or `configs/.env`.

Account state, workspace names, enabled providers, defaults, and generated-secret digests are persisted through GORM and are never stored by mutating the runtime config file. One TAuth subject owns one or more personal workspaces; there is no membership, invitation, role, or shared-workspace model. F014 upgrades the previous one-row-per-user shape as one startup transaction: it preflights all legacy tenant, provider, and usage rows; rejects unclaimed static owners, duplicates, invalid routing or secrets, plaintext/corrupt keys, and orphan rows; renames the two colliding legacy GORM indexes and the old tables; creates explicit user and workspace records; preserves opaque tenant ids and usage; rebinds provider-key encryption to tenant ids; verifies counts, values, and decryption; writes schema version 1; and drops only its bounded legacy tables. Every failed stage rolls back to the untouched legacy schema and original index names and prevents startup.

Schema version 2 gives every managed usage event one nonblank outcome code chosen at the request/error boundary: `success`, `invalid_request`, `payload_too_large`, `rate_limited`, `service_unavailable`, `request_timeout`, or `upstream_error`. The bounded upgrade maps historical successful rows to `success` and exact `400`, `413`, `429`, `499`, `502`, `503`, and `504` statuses to their canonical failure codes before adding the tenant/success/time/id page index; caller cancellation `499` and proxy-budget expiry `504` both become `request_timeout`. An unsupported historical status rejects startup before mutation. Neither current recording nor migration persists or reconstructs raw provider bodies or free-form error messages.

Managed usage persistence is deliberately asynchronous and bounded. After the selected proxy response is written and flushed, the request handler constructs one immutable content-free usage record and performs a non-blocking send to the management runtime's FIFO channel. `management.usage_queue_size` is positive and defaults to `1024`; it is independent from the upstream `server.queue_size`. The first accepted event starts the runtime's sole usage writer goroutine. That writer serializes accepted usage inserts in FIFO order, attempts each insert once under the existing detached five-second budget, and never creates per-event goroutines or retries. A dedicated context-aware database-write gate sequences usage inserts with management mutations without taking the management mutation mutex. Authentication bypasses both and remains an independent caller-scoped read transaction; runtime SQLite WAL journaling and the bounded busy timeout allow that reader to proceed alongside the writer.

If the usage channel is full, the newest event is dropped while previously accepted events remain queued, the public response is unchanged, and one safe warning carries the stable `managed_usage_queue_full` error. Accepted entries are process-local at-most-once work until their insert commits: queued entries are not crash-durable, and an insert failure or process exit can lose uncommitted telemetry. Usage summaries are therefore eventually consistent with completed responses. This contract is appropriate only because managed usage is operational telemetry, not billing, accounting, or a provider-job ledger. Queued records and their warnings exclude prompts, responses, audio, transcripts, tenant secrets, provider keys, raw provider bodies, and free-form upstream errors.

Schema version 3 makes saved tenant keys the hard eligibility boundary for
managed routing defaults. Its bounded upgrade decrypts and validates every
provider record, preserves eligible defaults, deterministically replaces
ineligible pairs, clears text or dictation when no provider is eligible,
preserves tenant timestamps, verifies the result, and records version 3 in one
transaction. Current-version startup rejects any later drift.

Server/runtime settings, backend auth validation, browser-facing MPR UI/TAuth settings, provider base URLs, transcription URLs, and model catalogs remain config-file-owned. Database access must use GORM model APIs without raw SQL. Generated llm-proxy client secrets are returned once and stored as SHA-256 digests. Managed tenants authenticate the same public proxy endpoints with `key=<generated secret>` and use only their own saved provider credentials.

The shared header has one application-owned notification region followed by the MPR-owned identity control in the `aux` slot. Scoped flex ordering keeps every visible notice immediately left of the avatar or Sign in control, and the application clears each notice after 10 seconds; MPR UI remains the only owner of sign-in, session, and avatar-menu behavior.

On startup, the `mpr-ui@latest` shell restores the browser session through TAuth `/auth/session` and reports the resulting lifecycle state before LLM Proxy requests protected workspace data. A valid refresh cookie rotates into a new access cookie without exposing the signed-out panel. Ordinary reloads never clear either cookie. The explicit user-menu **Sign out** action is the only application flow that calls `/auth/logout` and clears the session.

The LLM Proxy application startup guard covers the complete versioned first-party module graph and the pinned Alpine module. A module-link failure or browser-side rejection of the one canonical CDN dependency renders the application-owned recovery surface, completes `llm-proxy:management-ready`, and makes no protected management request; it never tries a mirror, bundled copy, retry, or alternate authentication path. The local ghttp surface sends `Cache-Control: no-store`, while one bounded module-graph revision evicts files cached before that policy. MPR UI continues to own the authenticated session and shared transition, while LLM Proxy owns whether its application runtime mounted successfully.

The backend consumes TAuth's published Go `pkg/sessionvalidator` for cookie/JWT validation and adds only llm-proxy's tenant, required-expiry, and principal invariants; no application-owned JWT parser or claims schema exists. The gateway `llm-proxy` target stages both services' runtime inputs, restarts `tauth-api` and `llm-proxy`, and verifies both health checks before Pages activation so signing-key, cookie-name, and cookie-domain changes cannot leave the two runtimes split.

The authenticated management landing view is usage-focused. The browser first loads `GET /api/management/account`; every returned tenant remains operational through its own generated secret, and the browser has no global active-tenant or URL/history selection contract. The independent `Usage tenant` control defaults to `All tenants` immediately before the ordered `ALL`, `30 days`, `7 days`, and `1 day` controls, while the interval defaults to `30 days`. The all-tenant selection calls `GET /api/management/usage?interval=all|30d|7d|1d`; an explicit tenant calls `GET /api/management/tenants/:tenant_id/usage?interval=all|30d|7d|1d`. Both operations require exactly one recognized interval, return `400` for missing, repeated, or unknown values, and carry `Cache-Control: no-store`. The response is the current `interval`, `bucket_unit`, aggregate `totals`, ordered generic `buckets`, and provider, model, and status-code usage for the selected scope; it contains no fixed-period `period_days` or `daily` fields. `1d` uses 24 hourly buckets, `7d` and `30d` use exact trailing-duration daily buckets, and `all` uses UTC daily buckets from the earliest retained event through one captured server timestamp. An empty all-time result has no buckets. Account-wide aggregation runs once at the database boundary across every owned tenant and calculates totals and average latency from the complete event set; the browser never fans out per-tenant summaries. Refresh and interval changes retain the Usage tenant selection, Settings changes do not affect it, loading disables the Usage controls, and request identity prevents a stale scope or interval response from replacing the selected snapshot. The admin API remains a distinct 30-day daily contract. Managed proxy requests record endpoint, provider, model, status, success flag, latency, and normalized token counts only; prompts, audio, transcripts, responses, tenant secrets, and provider API keys are excluded from usage events. Tenant lifecycle, client access, generated secrets, routing defaults, copyable request examples, and provider key controls live in the Settings modal opened from the shared `<mpr-user>` avatar dropdown, where the `Settings` item is inserted before `Sign out`. One compact `Tenant access` row combines the `Tenant` selector, modal Rename, client-key state and one-time reveal/copy actions, confirmed Replace key, confirmed Delete tenant, and Create tenant. The selected tenant is only the Settings editor context. Switching that selector with an unsaved draft requires explicit discard confirmation, clears any raw one-time secret or revealed provider key from browser state, and never changes the Usage tenant. The routing-default form lists only providers with saved tenant keys; its dictation controls are disabled and show `Not configured` when none of those providers supports dictation. It keeps Text provider, Text model, and the selected model's Reasoning effort control on one desktop row, clears an incompatible effort on a model change, exposes `Not supported` when the route has no reasoning capability, and autosaves provider/model/effort selections immediately plus the tenant system prompt on field exit. Tenant-wide and provider-specific system-prompt fields start collapsed behind semantic `System prompt` disclosures with a visible `Hidden` indicator; they expand through pointer or keyboard activation and collapse again when Settings opens or their tenant/provider context changes. Settings serializes every mutation that returns a complete tenant profile and locks its controls while a close request waits for in-flight work. A client key created or replaced during that wait keeps Settings open for an explicit later close so its one-time value remains available to copy; removing the last provider key re-enforces mandatory setup; client keys can only be replaced or removed with their owning non-final tenant. Failed edits remain available for retry. Request examples include copyable default text and v2 commands only when a keyed text default exists, and a default dictation command only when a keyed dictation default exists, plus copyable selected-provider text and v2 commands; dictation-capable selected providers also show a provider-specific dictation command. Provider key controls use one selected-provider editor with API key, text model, and system prompt fields because those settings are part of the provider-owned managed routing contract.

When the selected Usage snapshot contains failures, the success-rate card exposes an **N failed requests** action. Its semantic dialog keeps the selected interval and the summary's non-success status breakdown. `GET /api/management/usage/failures` pages newest-first failures across all owned tenants and adds each row's safe tenant id and current name; `GET /api/management/tenants/:tenant_id/usage/failures` retains the tenant-less row shape for one explicitly selected owned tenant. Both operations require one `interval`, accept one `limit` from 1 through 100 and one opaque `cursor`, return `Cache-Control: no-store`, and paginate against one opaque snapshot using stable event-time/id order. Each cursor is bound to its exact all-tenant or tenant scope and is rejected in every other scope. Apart from the account-wide tenant context, rows contain only event time, endpoint, provider, model, status, canonical outcome code, and latency. Dialog load failures stay local; Usage tenant or interval changes abort and invalidate the request; the admin surface remains aggregate-only.

## Error Contract

- `400`: unknown provider, unknown model, unsupported capability, unsupported endpoint, conflicting model parameters, or client-supplied provider API key fields on public proxy requests.
- `403`: missing or invalid client `key`.
- `413`: prompt or audio payload too large.
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

- Provider/model validation happens at the HTTP edge through a provider registry built from the configured model catalogs.
- OpenAI keeps the existing Responses API adapter and derives Responses and Models URLs from `providers.openai.base_url`; audio transcription uses `providers.openai.transcriptions_url`. The adapter polls documented pending states, normalizes only `incomplete/max_output_tokens` for shared continuation, and rejects failed, cancelled, other incomplete, missing, or unknown states.
- Non-OpenAI compatible text providers use a shared Chat Completions adapter. It normalizes `finish_reason=length` for shared continuation and requires `finish_reason=stop` to complete content or reasoning text.
- Meta uses the shared OpenAI-compatible Chat Completions adapter against `providers.meta.base_url`; its proxy contract is text-only and has no Responses fallback.
- Anthropic uses a native Messages adapter, translating proxy `system` messages to the top-level Anthropic `system` parameter and `user`/`assistant` messages to Anthropic `messages[]`; `max_tokens` continues through the shared coordinator, while `end_turn` and `stop_sequence` are complete text stops.
- Gemini uses a native generateContent adapter against `providers.gemini.base_url`; `MAX_TOKENS` continues through the shared coordinator, while `STOP` is the complete text finish reason.
- Grok uses the shared OpenAI-compatible Chat Completions adapter against `providers.grok.base_url`.
- OpenAI-compatible chat providers receive validated and sorted `messages[]` as provider-supported `role` and `content` items.
- OpenAI Responses payload shape comes from the selected configured model's stable `request_profile`; model-specific web-search support comes from the selected model catalog entry. OpenAI Responses text calls run in background mode with stored responses so long provider work can be polled by llm-proxy while the caller waits on one REST request.
- Gemini receives user messages as native `contents`, assistant messages as `model` contents, and system messages as `systemInstruction`.
- OpenAI Responses receives single-prompt requests unchanged and multi-message requests as a deterministic role-labelled transcript.
- Dictation routing reuses the multipart transcription adapter with provider-specific URLs. OpenAI, SiliconFlow, and Zhipu send a multipart `model` field; Grok/xAI uses xAI STT and omits the multipart `model` field. Only providers that support `/dictate` expose transcription URL config fields.
- Response formatting keeps existing text/XML/CSV bodies and existing JSON `request`, `response`, and normalized `usage` fields. JSON responses also include OpenRouter-style `object`, `model`, and `choices[].message.content` metadata, plus caller-visible request `messages` with provided `order` values. Server-injected tenant default system prompts are sent upstream but not echoed in response metadata.

## Test Strategy

Black-box router tests cover:

- OpenAI omitted-provider regression.
- OpenAI output-budget recovery through the public `POST /v2` handler,
  including pending-response snapshots, repeated suffix attempts, exact text
  assembly, aggregated usage, and one successful managed event.
- OpenAI polling only for documented pending states, with missing and unknown
  states rejected without another provider call.
- Every configured provider through its public text route, proving the same
  shared continuation transcript, completion order, suffix assembly, and usage
  aggregation for Chat Completions, Gemini, Anthropic, and OpenAI.
- Deadline exhaustion and nonrecoverable safety, refusal, tool, malformed,
  missing, and unknown signals, proving partial text is never exposed as a
  failure response.
- Explicit Meta Muse Spark 1.1 routing through `GET /`, compatibility `POST /`, and canonical `POST /v2`.
- Unsupported Meta `web_search` and dictation paths.
- Explicit DeepSeek chat-completions routing.
- Unsupported `web_search` for DeepSeek.
- Known provider without credential.
- Invalid default dictation provider configuration.
- Conflicting JSON body/query models.
- SiliconFlow dictation routing.
- Configured text model routing without code changes.
- Invalid configured model catalog startup failures.
- Existing OpenAI dictation and response-format tests.
