# Provider Routing Implementation Plan

Status: implemented provider-routing contract notes retained from the retired provider-routing backlog.

## Goal

Extend `llm-proxy` from an OpenAI-only proxy into an explicit multi-provider proxy while preserving current OpenAI defaults for existing clients.

## Request Contract

- `provider` is an optional query parameter on `GET /`, `POST /`, `POST /v2`, and `POST /dictate`.
- Omitted `provider` means the authenticated tenant's default provider.
- `model` keeps its current meaning; omitted `model` means the authenticated tenant's default model when set, otherwise the selected provider's configured default model.
- A provider with an API key configured must have a configured default text model so provider-selected requests can omit `model` consistently.
- Compatibility JSON `POST /` accepts exactly one text input shape: `prompt` for a single user prompt or `messages[]` for an OpenRouter/OpenAI-compatible chat transcript.
- Canonical JSON `POST /v2` accepts only `messages[]` as the text input shape; request-body `prompt` and `system_prompt` are invalid.
- `messages[]` items contain `role` and string `content`. Supported roles are `system`, `user`, and `assistant`; at least one `user` message is required.
- `messages[].order` is optional. When any submitted message includes `order`, every submitted message must include a unique non-negative integer `order`; the proxy sorts submitted messages by ascending `order` before adding a request or tenant system prompt and before routing upstream.
- With `messages[]` on `POST /`, body `system_prompt` is prepended as a system message only when the transcript does not already contain a `system` message. A body containing both `system_prompt` and a system message is invalid. With `POST /v2`, callers send system instructions as `system` role messages.
- `max_tokens` is an optional positive integer on `GET /` query strings and JSON `POST /` bodies.
- Provided `max_tokens` maps to OpenAI Responses `max_output_tokens`, Meta Chat Completions `max_completion_tokens`, other OpenAI-compatible chat completions `max_tokens`, Anthropic Messages `max_tokens`, and Gemini `generationConfig.maxOutputTokens`.
- Omitted `max_tokens` means the proxy omits provider max-token fields and lets the selected provider/model default apply, except Anthropic Messages where the upstream API requires `max_tokens` and the proxy sends the selected model's configured synchronous output limit.
- Known provider-specific output-token ceilings are validated before upstream calls; Gemini text models reject `max_tokens` above `65536` with `400 Bad Request`, and Claude models reject values above their configured synchronous Messages output limit.
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
| `moonshot` | `kimi` | OpenAI-compatible chat completions | Not supported | Not supported |
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

Provider credentials and base URLs:

- `providers.openai.api_key`, `providers.openai.base_url`, `providers.openai.transcriptions_url`
- `providers.meta.api_key`, `providers.meta.base_url`
- `providers.deepseek.api_key`, `providers.deepseek.base_url`
- `providers.dashscope.api_key`, `providers.dashscope.base_url`
- `providers.moonshot.api_key`, `providers.moonshot.base_url`
- `providers.siliconflow.api_key`, `providers.siliconflow.base_url`, `providers.siliconflow.transcriptions_url`
- `providers.zhipu.api_key`, `providers.zhipu.base_url`, `providers.zhipu.transcriptions_url`
- `providers.gemini.api_key`, `providers.gemini.base_url`
- `providers.anthropic.api_key`, `providers.anthropic.base_url`
- `providers.grok.api_key`, `providers.grok.base_url`, `providers.grok.transcriptions_url`

Provider model catalogs:

- `providers.<provider>.text.default_model`
- `providers.<provider>.text.models[].id`
- `providers.<provider>.text.models[].output_token_limit`
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
dictation model ids, model-specific web-search enablement, and known
provider-side output-token limits.

OpenAI `request_profile` values select stable payload shapes:

- `openai_responses_base`
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
The server keeps `GET /` and compatibility JSON `POST /` available for direct
REST callers.

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
proves authenticated routing without an upstream call.

Startup validates configured tenants, rejects duplicate tenant ids and duplicate secrets, requires API keys for each configured static tenant's default text and dictation providers when management mode is disabled, allows non-default provider API keys to be blank so those providers are disabled until configured, requires every configured provider base URL, requires transcription URLs for dictation-capable providers, requires text model catalogs for every provider, requires dictation model catalogs for dictation-capable providers, rejects blank or duplicate model ids, rejects defaults not listed in their model catalog, rejects `web_search` outside OpenAI text model entries, validates OpenAI request profiles, validates each configured static tenant's default text provider/model, and validates endpoint/credential support for each configured static tenant's default dictation provider/model. When `management.enabled` is false, at least one static tenant is required. When `management.enabled` is true, static tenants and nonblank config-level provider API keys are rejected because managed tokens and provider credentials are user-owned database state.

The repository owns the immutable release implementation under
`tools/gitrelease` and uses one canonical `vMAJOR.MINOR.PATCH` SemVer contract.
`make release` builds container inputs from the tracked `git archive HEAD`
snapshot, packages a Pages marker for that source commit, and creates the
changelog-only release commit. Before Pages branch mutation, `make deploy`
validates the archive, its source marker, and the remote release tag, then
verifies the public marker after backend rollout.

The management UI is served as a static GitHub Pages app from `site/` on `https://llm-proxy.mprlab.com`; the Go backend does not serve management HTML or assets. `make release` renders and validates the Pages archive locally, `make publish` uploads that immutable archive without changing the live site, and `make deploy` activates it on `gh-pages` after the backend rollout. The backend serves one public browser config file at `/config-ui.yaml` from the already-loaded management config, with credentialed CORS restricted to `management.public_origin`. The static app owns the normal MPR UI contract: API-served `config-ui.yaml`, pinned `mpr-ui` assets, `mpr-ui-config.js`, `<mpr-header data-config-url="...">`, a pinned bundle marker, `<mpr-user>`, and `<mpr-footer>`. The Pages artifact contains no static `config-ui.yaml` or `llm-proxy-config.json`; release rendering writes the profile-owned `PAGES_CONFIG_URL` into the declarative header attribute, and `mpr-ui-config.js` applies that single backend-served YAML before loading the bundle. Application JavaScript listens to documented `mpr-ui:auth:*` events, waits for MPR UI auto-orchestration before releasing the shared auth transition, and does not read MPR UI internal DOM state. The YAML points browser management API calls, generated proxy examples, and MPR UI/TAuth at the configured origins. DNS must leave `llm-proxy.mprlab.com` pointed at GitHub Pages and point `llm-proxy-api.mprlab.com` at the MPR gateway; the gateway route for `llm-proxy.mprlab.com` must be removed or moved so the backend only owns the API hostname. Management APIs under `/api/management` validate the configured TAuth session cookie locally with issuer `tauth` unless `management.jwt_issuer` overrides it. Provider API keys are accepted only through authenticated management endpoints, encrypted at rest with AES-GCM using the required `management.provider_key_encryption_key`, stored server-side, and returned only as masked status. Each provider-key record also stores that provider's selected text model and provider-specific system prompt. Managed text requests that select a provider and omit `model` use the saved provider text model, and provider-selected requests without request-level system instructions use the saved provider system prompt. Existing plaintext provider-key rows are encrypted and cleared during management startup, and existing provider-key rows without a text model are normalized to the current configured provider default model. This is an encrypted-at-rest guarantee for database dumps, backups, and direct storage access, not a user-only decryption or zero-knowledge guarantee. Management mode requires `management.database_dialect`, `management.database_dsn`, and `management.provider_key_encryption_key`; supported GORM dialects are `postgres` and `sqlite`, with SQLite using a pure-Go GORM driver so `CGO_ENABLED=0` release builds remain valid. Administrators are config-owned through exact `management.admin_emails` entries; public config populates those entries from the plural `${LLM_PROXY_MANAGEMENT_ADMIN_EMAILS}` YAML flow sequence placeholder so personal admin addresses stay out of the repository while allowing multiple admins. An admin session gets `user.is_admin: true`, an `Admin` avatar-menu item, and `GET /api/management/admin/users` access to all managed users' tenant facts and 30-day usage summaries without provider API keys, masked key values, generated secrets, secret digests, prompts, responses, audio names, or transcripts. Non-admin sessions receive `403` from admin APIs. The packaged management config uses strict expandable `LLM_PROXY_MANAGEMENT_*` placeholders, so local and hosted profiles must define explicit values in the runtime environment or `configs/.env`. Signup state, enabled providers, defaults, and generated-secret digests are persisted through GORM and are never stored by mutating the runtime config file. The old startup importer is retired, and startup removes its obsolete marker table through GORM. If a prior release left exactly one `static-config:<tenant-id>` row, startup requires an exact `management.legacy_token_migration` tenant id and deployment-owned target email; the unowned token returns `403` until that verified email signs in. Drain every old service instance before that email signs in. The first matching management session atomically rekeys the tenant to the TAuth subject, preserves its token digest, defaults, creation time, and all usage events, and re-encrypts provider keys for the new owner id. A destination conflict returns `409` without partial writes. After production verification, remove the temporary migration config. Server/runtime settings, backend auth validation, browser-facing MPR UI/TAuth settings, provider base URLs, transcription URLs, and model catalogs remain config-file-owned. Database access must use GORM model APIs without raw SQL. Generated llm-proxy client secrets are returned once and stored as SHA-256 digests. Managed tenants authenticate the same public proxy endpoints with `key=<generated secret>` and use only their own saved provider credentials.

On startup, the pinned MPR UI shell restores the browser session through TAuth `/auth/session` before LLM Proxy treats a management-profile `401` as anonymous; a valid refresh cookie rotates into a new access cookie without exposing the signed-out panel. Ordinary reloads never clear either cookie. The explicit user-menu **Sign out** action is the only application flow that calls `/auth/logout` and clears the session.

The backend consumes TAuth's published Go `pkg/sessionvalidator` for cookie/JWT validation and adds only llm-proxy's tenant, required-expiry, and principal invariants; no application-owned JWT parser or claims schema exists. The gateway `llm-proxy` target stages both services' runtime inputs, restarts `tauth-api` and `llm-proxy`, and verifies both health checks before Pages activation so signing-key, cookie-name, and cookie-domain changes cannot leave the two runtimes split.

The authenticated management landing view is usage-focused. `GET /api/management/usage` returns 30-day aggregate, daily, provider, model, and status-code usage for the signed-in user's managed tenant. Managed proxy requests record endpoint, provider, model, status, success flag, latency, and normalized token counts only; prompts, audio, transcripts, responses, tenant secrets, and provider API keys are excluded from usage events. Client access, generated secrets, routing defaults, copyable request examples, and provider key controls live in a large Settings modal opened from the shared `<mpr-user>` avatar dropdown, where the `Settings` item is inserted before `Sign out`. Request examples include copyable default text, v2, and dictation commands plus copyable selected-provider text and v2 commands; dictation-capable selected providers also show a provider-specific dictation command. Provider key controls use one selected-provider editor with API key, text model, and system prompt fields because those settings are part of the provider-owned managed routing contract.

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
