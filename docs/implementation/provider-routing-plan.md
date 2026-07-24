# Provider Routing Implementation Plan

Status: implemented provider-routing contract notes retained from the retired provider-routing backlog.

## Goal

Extend `llm-proxy` from an OpenAI-only proxy into an explicit multi-provider proxy while preserving current OpenAI defaults for existing clients.

## Request Contract

- `provider` is an optional query parameter on `GET /`, `POST /`, `POST /v2`, and `POST /dictate`.
- Omitted `provider` means the authenticated tenant's default provider.
- `model` keeps its current meaning; omitted `model` means the authenticated tenant's default model when set, otherwise the selected provider's configured default model.
- A provider with an API key configured must have a configured default text model so provider-selected requests can omit `model` consistently.
- Managed tenants persist complete canonical text and dictation provider/model pairs. A request that omits the routing fields uses the exact saved text pair; management persistence never substitutes a different provider/model pair at runtime.
- Compatibility JSON `POST /` accepts exactly one text input shape: `prompt` for a single user prompt or `messages[]` for an OpenRouter/OpenAI-compatible chat transcript.
- Canonical JSON `POST /v2` accepts only `messages[]` as the text input shape; request-body `prompt` and `system_prompt` are invalid.
- `messages[]` items contain `role` and string `content`. Supported roles are `system`, `user`, and `assistant`; at least one `user` message is required.
- `messages[].order` is optional. When any submitted message includes `order`, every submitted message must include a unique non-negative integer `order`; the proxy sorts submitted messages by ascending `order` before adding a request or tenant system prompt and before routing upstream.
- With `messages[]` on `POST /`, body `system_prompt` is prepended as a system message only when the transcript does not already contain a `system` message. A body containing both `system_prompt` and a system message is invalid. With `POST /v2`, callers send system instructions as `system` role messages.
- `max_tokens` is an optional positive integer on `GET /` query strings and JSON `POST /` bodies.
- Provided `max_tokens` maps to OpenAI Responses `max_output_tokens`, Meta, Moonshot, and MiniMax Chat Completions `max_completion_tokens`, other OpenAI-compatible chat completions `max_tokens`, Anthropic Messages `max_tokens`, and Gemini `generationConfig.maxOutputTokens`.
- Omitted `max_tokens` means the proxy omits provider max-token fields and lets the selected provider/model default apply, except Anthropic Messages where the upstream API requires `max_tokens` and the proxy sends the selected model's configured synchronous output limit.
- Known provider-specific output-token ceilings are validated before upstream calls; MiniMax M2.7 rejects `max_tokens` above `2048`, Gemini text models reject values above `65536`, and Claude models reject values above their configured synchronous Messages output limits with `400 Bad Request`.
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
- `management.database_dialect`
- `management.database_dsn`
- `management.provider_key_encryption_key`
- `management.management_api_origin`
- `management.proxy_origin`
- `management.legacy_token_migration.tenant_id`
- `management.legacy_token_migration.owner_email`
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
`reasoning_effort` controls are not part of the public proxy request contract.
The distinct tenant routing-default `reasoning_effort` can be forwarded only
through an explicit catalog capability mapping. The current mapping is the
OpenAI Responses reasoning adapter; it does not enable GLM or generic
OpenAI-compatible routes.

OpenAI `request_profile` values select stable payload shapes:

- `openai_responses_temperature`
- `openai_responses_temperature_tools`
- `openai_responses_reasoning_tools`

Every OpenAI Responses text request includes `background: true` and
`store: true`; the proxy polls the returned response id server-side until a
terminal status or `server.request_timeout_seconds`. Callers use one normal
`GET /`, `POST /`, or `POST /v2` request and receive the final formatted answer;
there is no streaming or client-side polling contract.

Bundled clients intentionally expose only the canonical `POST /v2` text
contract. The installable Go CLI maps prompt flags or stdin into v2 `system` and
`user` messages, while the reusable Go and Python packages expose only
messages-request constructors and `PostMessages`/`post_messages` send methods.
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

## Managed Routing Defaults

`PUT /api/management/defaults` requires both a text `provider`/`model` pair, a
dictation `dictation_provider`/`dictation_model` pair, and an explicit
`reasoning_effort` value. Empty is the explicit unset value; a nonempty value
must be in the selected exact text provider/model capability list. The handler
resolves the text pair before validating effort and constructs all defaults
before the database write, so a blank, unknown, unsupported, cross-provider
model, or incompatible effort fails with `400 managed_routing_defaults_invalid`
and leaves the prior defaults unchanged. The profile response exposes
capabilities only through `providers[].text_models[]`; it has no provider-level
capability or global option list. A malformed profile is a workspace-integrity
failure in the browser, not a UI repair opportunity.

Startup owns a single versioned, transactional migration for previously stored
management defaults. Version 3 retains an effort only when it is valid for its
stored text route and converts an invalid nonempty value to the explicit unset
value; it never derives an effort from a model, provider, profile, or web-search
state. Before the version marker is written, it repairs only a blank
model or a model configured for a different provider at the same endpoint by
selecting the saved provider's configured endpoint default. It rejects unknown
models and unknown or dictation-unsupported providers with contextual
tenant, endpoint, provider, and model errors. After the marker exists, all
persisted pairs must be canonical and valid; startup fails rather than retaining
a fallback, compatibility read, or runtime repair path.

`server.workers` limits concurrent upstream provider HTTP operations, not whole
client request lifecycles. `server.queue_size` limits the number of additional
upstream HTTP operations waiting for that shared worker limit. OpenAI
background-response sleeps between polls do not occupy worker capacity; only the
actual upstream create, poll, continuation, synthesis, chat, native-provider, or
dictation HTTP operation does.

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
execution, discovers selected provider keys, and writes a disposable static
tenant config with management disabled and placeholder values for unused
providers. It therefore cannot open or mutate the configured management
database. The `--write-config` option exposes that generated contract without
building the proxy or making a paid provider call; `--preflight` starts it and
proves authenticated routing without an upstream call. Each run allocates a
fresh loopback port unless `LLM_PROXY_LIVE_PORT` explicitly provides one, and
cleanup terminates only the proxy child started by the harness rather than a
process discovered through a shared port.

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

Provider API keys are accepted only through authenticated management endpoints and encrypted at rest with AES-GCM using the required `management.provider_key_encryption_key`. Normal save, profile, and administrator responses return only masked status. The sole raw-key response is the explicit owner-authenticated `POST /api/management/provider-keys/:provider/reveal` action, which requires the configured management origin and returns `Cache-Control: no-store`. Each provider-key record also stores that provider's selected text model and provider-specific system prompt. Managed text requests that select a provider and omit `model` use the saved provider text model, and provider-selected requests without request-level system instructions use the saved provider system prompt. Existing plaintext provider-key rows are encrypted and cleared during management startup, and existing provider-key rows without a text model are normalized to the current configured provider default model. This is an encrypted-at-rest guarantee for database dumps, backups, and direct storage access, not a user-only decryption or zero-knowledge guarantee.

Management mode requires `management.database_dialect`, `management.database_dsn`, and `management.provider_key_encryption_key`; supported GORM dialects are `postgres` and `sqlite`, with SQLite using a pure-Go GORM driver so `CGO_ENABLED=0` release builds remain valid. Administrators are config-owned through exact `management.admin_emails` entries; public config populates those entries from the plural `${LLM_PROXY_MANAGEMENT_ADMIN_EMAILS}` YAML flow sequence placeholder so personal admin addresses stay out of the repository while allowing multiple admins. An admin session gets `user.is_admin: true`, an `Admin` avatar-menu item, and `GET /api/management/admin/users` access to all managed users' tenant facts and 30-day usage summaries without provider API keys, masked key values, generated secrets, secret digests, prompts, responses, audio names, or transcripts. Non-admin sessions receive `403` from admin APIs. The packaged management config uses strict expandable `LLM_PROXY_MANAGEMENT_*` placeholders, so local and hosted profiles must define explicit values in the runtime environment or `configs/.env`.

Signup state, enabled providers, defaults, and generated-secret digests are persisted through GORM and are never stored by mutating the runtime config file. The old startup importer is retired, and startup removes its obsolete marker table through GORM. If a prior release left exactly one `static-config:<tenant-id>` row, startup requires an exact `management.legacy_token_migration` tenant id and deployment-owned target email; the unowned token returns `403` until that verified email signs in. Drain every old service instance before that email signs in. The first matching management session atomically rekeys the tenant to the TAuth subject, preserves its token digest, defaults, creation time, and all usage events, and re-encrypts provider keys for the new owner id. An empty destination account created by an earlier sign-in is removed inside that transaction before the rekey; a destination with a generated secret, provider settings, or usage returns `409` without partial writes. After production verification, remove the temporary migration config.

Server/runtime settings, backend auth validation, browser-facing MPR UI/TAuth settings, provider base URLs, transcription URLs, and model catalogs remain config-file-owned. Database access must use GORM model APIs without raw SQL. Generated llm-proxy client secrets are returned once and stored as SHA-256 digests. Managed tenants authenticate the same public proxy endpoints with `key=<generated secret>` and use only their own saved provider credentials.

The shared header has one application-owned notification region followed by the MPR-owned identity control in the `aux` slot. Scoped flex ordering keeps every visible notice immediately left of the avatar or Sign in control, and the application clears each notice after 10 seconds; MPR UI remains the only owner of sign-in, session, and avatar-menu behavior.

On startup, the `mpr-ui@latest` shell restores the browser session through TAuth `/auth/session` and reports the resulting lifecycle state before LLM Proxy requests protected workspace data. A valid refresh cookie rotates into a new access cookie without exposing the signed-out panel. Ordinary reloads never clear either cookie. The explicit user-menu **Sign out** action is the only application flow that calls `/auth/logout` and clears the session.

The backend consumes TAuth's published Go `pkg/sessionvalidator` for cookie/JWT validation and adds only llm-proxy's tenant, required-expiry, and principal invariants; no application-owned JWT parser or claims schema exists. The gateway `llm-proxy` target stages both services' runtime inputs, restarts `tauth-api` and `llm-proxy`, and verifies both health checks before Pages activation so signing-key, cookie-name, and cookie-domain changes cannot leave the two runtimes split.

The authenticated management landing view is usage-focused. `GET /api/management/usage?interval=all|30d|7d|1d` requires exactly one recognized interval and returns `400` for missing, repeated, or unknown values. The user response is the current `interval`, `bucket_unit`, aggregate `totals`, ordered generic `buckets`, and provider, model, and status-code usage for the signed-in user's managed tenant; it contains no fixed-period `period_days` or `daily` fields. `1d` uses 24 hourly buckets, `7d` and `30d` use exact trailing-duration daily buckets, and `all` uses UTC daily buckets from the earliest retained tenant event through the one captured server timestamp. An empty all-time result has no buckets. The UI orders its controls as `ALL`, `30 days`, `7 days`, and `1 day`, defaults to `30 days`, replaces every dashboard surface from the selected response, retains the selection on refresh, disables interval/refresh controls while loading, discards stale out-of-order responses, and clears the selected snapshot on failure. Each database query applies the authenticated user id and the selected time boundary; the admin API remains a distinct 30-day daily contract. Managed proxy requests record endpoint, provider, model, status, success flag, latency, and normalized token counts only; prompts, audio, transcripts, responses, tenant secrets, and provider API keys are excluded from usage events. Client access, generated secrets, routing defaults, copyable request examples, and provider key controls live in a large Settings modal opened from the shared `<mpr-user>` avatar dropdown, where the `Settings` item is inserted before `Sign out`. The routing-default form keeps Text provider, Text model, and the selected model's Reasoning effort control on one desktop row. It clears an incompatible effort on a model change, exposes `Not supported` when the route has no capability, and autosaves provider/model/effort selections immediately plus the tenant system prompt on field exit. Settings serializes every mutation that returns a complete management profile and locks its controls while a close request waits for in-flight work. A client key created or replaced during that wait keeps Settings open for an explicit later close so its one-time value remains available to copy; revoking the client key or removing the last provider key re-enforces mandatory setup. Failed edits remain available for retry. Request examples include copyable default text, v2, and dictation commands plus copyable selected-provider text and v2 commands; dictation-capable selected providers also show a provider-specific dictation command. Provider key controls use one selected-provider editor with API key, text model, and system prompt fields because those settings are part of the provider-owned managed routing contract.

## Error Contract

- `400`: unknown provider, unknown model, unsupported capability, unsupported endpoint, conflicting model parameters, or client-supplied provider API key fields on public proxy requests.
- `403`: missing or invalid client `key`.
- `413`: prompt or audio payload too large.
- `429`: upstream provider rate limiting.
- `503`: registered non-default provider credential is unavailable, so the selected provider is disabled until its API key is configured.
- `504`: the overall proxy request timed out before the selected upstream provider returned a final result.
- `502`: other upstream provider failure.

## Implementation Notes

- Provider/model validation happens at the HTTP edge through a provider registry built from the configured model catalogs.
- OpenAI keeps the existing Responses API adapter and derives Responses and Models URLs from `providers.openai.base_url`; audio transcription uses `providers.openai.transcriptions_url`.
- Non-OpenAI text providers use a shared OpenAI-compatible Chat Completions adapter.
- Meta uses the shared OpenAI-compatible Chat Completions adapter against `providers.meta.base_url`; its proxy contract is text-only and has no Responses fallback.
- Anthropic uses a native Messages adapter, translating proxy `system` messages to the top-level Anthropic `system` parameter and `user`/`assistant` messages to Anthropic `messages[]`.
- Gemini uses a native generateContent adapter against `providers.gemini.base_url`.
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
