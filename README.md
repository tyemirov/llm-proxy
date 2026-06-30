# LLM Proxy

LLM Proxy is a lightweight HTTP service that forwards user prompts to OpenAI's
Responses API, OpenAI-compatible chat providers, Anthropic's native Messages
API, Google Gemini's native generateContent API, and audio transcription APIs.
It exposes protected HTTP endpoints that require a tenant secret and simplify
integrating provider capabilities without embedding API credentials in each
client.

## Features

- Minimal HTTP server that accepts:
  - `GET /?prompt=...&key=...[&provider=...]` for LLM responses
  - `POST /?key=...[&provider=...]` for large JSON prompt bodies
  - `POST /v2?key=...[&provider=...]` for ordered chat-message JSON bodies
  - `POST /dictate?key=...[&provider=...]` for audio transcription
- Choose the provider per request via `provider=...`; omitted provider uses the authenticated tenant default
- Choose the model per request via `model=...`; omitted model uses the tenant default when `provider` is omitted, otherwise the selected provider's configured default
- Choose the dictation model per request via `model=...` on `/dictate`; omitted model uses the tenant default when `provider` is omitted, otherwise the selected provider's configured default
- Optional per-request web search via `web_search=1|true|yes` when the selected provider/model is configured to support it
- Optional logging at `debug` or `info` levels
- Forwards requests using server-side provider API keys, loaded from the database in management mode
- Optional TAuth-protected self-service UI where signed-in users save their own provider API keys and generate llm-proxy tenant secrets
- Supports plain text, JSON, XML, or CSV responses

## REST Contract

llm-proxy exposes a blocking REST contract for text generation. A caller sends
one authenticated `GET /`, `POST /`, or `POST /v2` request and receives the final
formatted answer in that same HTTP response.

The caller does not stream tokens, poll a job endpoint, follow a resume token, or
know whether the selected upstream provider uses synchronous responses,
background responses, or provider-specific polling internally. For OpenAI
Responses, llm-proxy always owns the background-response lifecycle: it sends
stored background requests upstream and polls OpenAI server-side until the answer
is terminal or `server.request_timeout_seconds` expires.

A `504 Gateway Timeout` means the overall proxy request deadline expired before
the selected upstream provider produced a final answer. It is not a prompt for
the client to poll llm-proxy.

Internally, `server.workers` limits concurrent upstream provider HTTP
operations and `server.queue_size` limits upstream HTTP operations waiting for a
worker. Long OpenAI background-response poll sleeps do not occupy a worker slot;
only the actual upstream HTTP request or poll does.

## Configuration

The service reads service configuration from `config.yml`. The default path is
`config.yml` in the current working directory; use `--config /path/config.yml`
only to select a different file. Command-line flags and environment variables
are not service configuration sources.

Before parsing YAML, the loader expands `${NAME}` placeholders from process
environment variables and from an optional `.env` file in the same directory as
the selected config file. Process environment values override `.env` values.
Missing placeholders fail startup except when an `api_key` value is exactly one
missing placeholder; that exact missing provider credential expands to an empty
string so non-default providers can stay disabled. The loader does not mutate
process environment, and all runtime code receives only the validated config
value.

```yaml
server:
  port: 8080
  log_level: info
  workers: 4
  queue_size: 100
  request_timeout_seconds: 360
  max_prompt_bytes: 4194304
  max_input_audio_bytes: 26214400
management:
  enabled: ${LLM_PROXY_MANAGEMENT_ENABLED}
  public_origin: "${LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN}"
  ui_description: "${LLM_PROXY_MANAGEMENT_UI_DESCRIPTION}"
  ui_origins:
    - "${LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN}"
    - "${LLM_PROXY_MANAGEMENT_LOOPBACK_ORIGIN}"
    - "${LLM_PROXY_MANAGEMENT_LOCALHOST_ORIGIN}"
  admin_emails: ${LLM_PROXY_MANAGEMENT_ADMIN_EMAILS}
  tauth_url: "${LLM_PROXY_MANAGEMENT_TAUTH_URL}"
  tauth_tenant_id: "${LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID}"
  google_client_id: "${LLM_PROXY_MANAGEMENT_GOOGLE_CLIENT_ID}"
  login_path: "${LLM_PROXY_MANAGEMENT_TAUTH_LOGIN_PATH}"
  logout_path: "${LLM_PROXY_MANAGEMENT_TAUTH_LOGOUT_PATH}"
  nonce_path: "${LLM_PROXY_MANAGEMENT_TAUTH_NONCE_PATH}"
  jwt_signing_key: "${LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY}"
  jwt_issuer: "${LLM_PROXY_MANAGEMENT_JWT_ISSUER}"
  session_cookie_name: "${LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME}"
  database_dialect: "${LLM_PROXY_MANAGEMENT_DATABASE_DIALECT}"
  database_dsn: "${LLM_PROXY_MANAGEMENT_DATABASE_DSN}"
  provider_key_encryption_key: "${LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY}"
  management_api_origin: "${LLM_PROXY_MANAGEMENT_API_ORIGIN}"
  proxy_origin: "${LLM_PROXY_MANAGEMENT_PROXY_ORIGIN}"
tenants:
  - id: default
    secret: "${SERVICE_SECRET}"
    defaults:
      provider: openai
      model: gpt-4.1
      dictation_provider: openai
      dictation_model: gpt-4o-mini-transcribe
      system_prompt: ""
providers:
  openai:
    api_key: "${OPENAI_API_KEY}"
    base_url: "https://api.openai.com/v1"
    transcriptions_url: "https://api.openai.com/v1/audio/transcriptions"
    text:
      default_model: "gpt-4.1"
      models:
        - id: "gpt-4o-mini"
          request_profile: "openai_responses_temperature"
        - id: "gpt-4o"
          request_profile: "openai_responses_temperature_tools"
          web_search: true
        - id: "gpt-4.1"
          request_profile: "openai_responses_temperature_tools"
          web_search: true
        - id: "gpt-5-mini"
          request_profile: "openai_responses_base"
        - id: "gpt-5"
          request_profile: "openai_responses_reasoning_tools"
          web_search: true
        - id: "gpt-5.5"
          request_profile: "openai_responses_reasoning_tools"
          web_search: true
        - id: "gpt-5.5-pro"
          request_profile: "openai_responses_reasoning_tools"
          web_search: true
    dictation:
      default_model: "gpt-4o-mini-transcribe"
      models:
        - id: "gpt-4o-mini-transcribe"
        - id: "gpt-4o-transcribe"
  deepseek:
    api_key: "${DEEPSEEK_API_KEY}"
    base_url: "https://api.deepseek.com"
    text:
      default_model: "deepseek-v4-flash"
      models:
        - id: "deepseek-v4-flash"
        - id: "deepseek-v4-pro"
        - id: "deepseek-chat"
        - id: "deepseek-reasoner"
  dashscope:
    api_key: "${DASHSCOPE_API_KEY}"
    base_url: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
    text:
      default_model: "qwen-plus"
      models:
        - id: "qwen-plus"
  moonshot:
    api_key: "${MOONSHOT_API_KEY}"
    base_url: "https://api.moonshot.ai/v1"
    text:
      default_model: "kimi-k2-0905-preview"
      models:
        - id: "kimi-k2-0905-preview"
  siliconflow:
    api_key: "${SILICONFLOW_API_KEY}"
    base_url: "https://api.siliconflow.com/v1"
    transcriptions_url: "https://api.siliconflow.com/v1/audio/transcriptions"
    text:
      default_model: "deepseek-ai/DeepSeek-R1"
      models:
        - id: "deepseek-ai/DeepSeek-R1"
    dictation:
      default_model: "FunAudioLLM/SenseVoiceSmall"
      models:
        - id: "FunAudioLLM/SenseVoiceSmall"
  zhipu:
    api_key: "${ZHIPU_API_KEY}"
    base_url: "https://open.bigmodel.cn/api/paas/v4"
    transcriptions_url: "https://api.z.ai/api/paas/v4/audio/transcriptions"
    text:
      default_model: "glm-5.1"
      models:
        - id: "glm-5.1"
    dictation:
      default_model: "glm-asr-2512"
      models:
        - id: "glm-asr-2512"
  gemini:
    api_key: "${GEMINI_API_KEY}"
    base_url: "https://generativelanguage.googleapis.com/v1"
    text:
      default_model: "gemini-2.5-flash"
      models:
        - id: "gemini-3.5-flash"
          output_token_limit: 65536
        - id: "gemini-3.1-flash-lite"
          output_token_limit: 65536
        - id: "gemini-2.5-flash"
          output_token_limit: 65536
        - id: "gemini-2.5-flash-lite"
          output_token_limit: 65536
        - id: "gemini-2.5-pro"
          output_token_limit: 65536
  anthropic:
    api_key: "${ANTHROPIC_API_KEY}"
    base_url: "https://api.anthropic.com"
    text:
      default_model: "claude-sonnet-4-6"
      models:
        - id: "claude-opus-4-8"
          output_token_limit: 128000
        - id: "claude-sonnet-4-6"
          output_token_limit: 64000
        - id: "claude-haiku-4-5-20251001"
          output_token_limit: 64000
        - id: "claude-haiku-4-5"
          output_token_limit: 64000
        - id: "claude-sonnet-4-5-20250929"
          output_token_limit: 64000
        - id: "claude-sonnet-4-5"
          output_token_limit: 64000
        - id: "claude-opus-4-1-20250805"
          output_token_limit: 32000
        - id: "claude-opus-4-1"
          output_token_limit: 32000
  grok:
    api_key: "${XAI_API_KEY}"
    base_url: "https://api.x.ai/v1"
    transcriptions_url: "https://api.x.ai/v1/stt"
    text:
      default_model: "grok-4.3"
      models:
        - id: "grok-4.3"
        - id: "grok-4.3-latest"
        - id: "grok-latest"
        - id: "grok-build-0.1"
        - id: "grok-code-fast"
        - id: "grok-code-fast-1"
        - id: "grok-code-fast-1-0825"
    dictation:
      default_model: "xai-stt"
      models:
        - id: "xai-stt"
```

`server.workers` is not the number of client requests that may be connected at
once. It is the upstream provider HTTP concurrency limit shared by text
generation and dictation. `server.queue_size` is the number of additional
upstream HTTP operations that may wait for that shared limit before the proxy
returns `503 request queue full`.

### Provider support matrix

Provider selectors and aliases are accepted anywhere the public API accepts
`provider`. Omitted text models use the authenticated tenant default when
`provider` is omitted; otherwise they use the selected provider's configured
default text model. This table describes capabilities currently wired through
`llm-proxy` and the defaults shipped in [configs/config.yml](configs/config.yml).
Upstream providers may expose additional speech APIs that need separate proxy
adapters before they are available through `/dictate`.

| Provider selector | Aliases | Text API | Configured default text model | Credential field | Default base URL | Dictation | Web search |
|-------------------|---------|----------|-------------------------------|------------------|------------------|-----------|------------|
| `openai` | none | OpenAI Responses | `gpt-4.1` | `providers.openai.api_key` | `https://api.openai.com/v1` | Yes: `gpt-4o-mini-transcribe`, `gpt-4o-transcribe` | Yes, on marked OpenAI models |
| `deepseek` | none | OpenAI-compatible chat completions | `deepseek-v4-flash` | `providers.deepseek.api_key` | `https://api.deepseek.com` | No | No |
| `dashscope` | `qwen` | OpenAI-compatible chat completions | `qwen-plus` | `providers.dashscope.api_key` | `https://dashscope-intl.aliyuncs.com/compatible-mode/v1` | No | No |
| `moonshot` | `kimi` | OpenAI-compatible chat completions | `kimi-k2-0905-preview` | `providers.moonshot.api_key` | `https://api.moonshot.ai/v1` | No | No |
| `siliconflow` | none | OpenAI-compatible chat completions | `deepseek-ai/DeepSeek-R1` | `providers.siliconflow.api_key` | `https://api.siliconflow.com/v1` | Yes: `FunAudioLLM/SenseVoiceSmall` | No |
| `zhipu` | `glm` | OpenAI-compatible chat completions | `glm-5.1` | `providers.zhipu.api_key` | `https://open.bigmodel.cn/api/paas/v4` | Yes: `glm-asr-2512` | No |
| `gemini` | none | Gemini native `generateContent` | `gemini-2.5-flash` | `providers.gemini.api_key` | `https://generativelanguage.googleapis.com/v1` | No | No |
| `anthropic` | `claude` | Anthropic native Messages | `claude-sonnet-4-6` | `providers.anthropic.api_key` | `https://api.anthropic.com` | No | No |
| `grok` | `xai` | xAI OpenAI-compatible chat completions | `grok-4.3` | `providers.grok.api_key` | `https://api.x.ai/v1` | Yes: `xai-stt` | No |

All upstream provider credentials are server-side only. Client requests must
never send OpenAI, Anthropic, xAI, Gemini, or other upstream API keys.

### Model catalog schema

Model ids and per-model metadata are runtime config data. To add, remove, or
replace provider models, update the selected `config.yml` and restart the
service; provider transports stay code-owned.

Each provider must declare a text catalog. A provider with an `api_key`
configured must have a valid text `default_model`; that default is used when a
request selects the provider and omits `model`.

```yaml
providers:
  provider_name:
    text:
      default_model: "provider-default-model"
      models:
        - id: "provider-model-id"
          request_profile: "openai_responses_temperature_tools"
          web_search: true
          output_token_limit: 65536
```

Dictation-capable providers must also declare a dictation catalog:

```yaml
providers:
  provider_name:
    dictation:
      default_model: "provider-default-dictation-model"
      models:
        - id: "provider-dictation-model-id"
```

Catalog validation fails startup when a provider text catalog is missing, a
dictation-capable provider dictation catalog is missing, a model id is blank or
duplicated, `default_model` is not present in the corresponding `models` list,
or `web_search: true` appears outside an OpenAI text model entry.
`output_token_limit` is optional for most providers; when set, it is used as a
proxy-side maximum for `max_tokens`. Anthropic text models require
`output_token_limit` because Anthropic Messages requires `max_tokens` even when
the client omits it.

`request_profile` is currently required only for OpenAI text models. It selects
the stable proxy payload shape for that OpenAI model and must be one of:

| Request profile | Payload behavior |
|-----------------|------------------|
| `openai_responses_base` | Sends `model` and `input`; omits temperature, tools, and reasoning controls. |
| `openai_responses_temperature` | Adds `temperature`. |
| `openai_responses_temperature_tools` | Adds `temperature`; includes web-search tools only when both the request and model catalog enable web search. |
| `openai_responses_reasoning_tools` | Adds reasoning/text controls; includes web-search tools only when both the request and model catalog enable web search. |

All OpenAI Responses text requests also send `background: true` and
`store: true`. llm-proxy polls the stored OpenAI response server-side until it
reaches a terminal state or the normal `server.request_timeout_seconds` deadline
expires. Plain REST callers use one `GET /`, `POST /`, or `POST /v2` request and
receive the final formatted answer; they do not stream, poll, or follow a
separate resume endpoint.

Provider-specific details:

* OpenAI is the only provider currently exposed with `web_search` support, and
  only for OpenAI model catalog entries with `web_search: true`. OpenAI
  dictation uses the same `providers.openai.api_key` value. OpenAI Responses
  and Models endpoint URLs are derived from `providers.openai.base_url`;
  dictation uses `providers.openai.transcriptions_url`.
* OpenAI-compatible text providers send chat completion requests with
  `Authorization: Bearer <api_key>` and the selected provider base URL.
* Only dictation-capable providers expose `transcriptions_url` fields:
  OpenAI uses `providers.openai.transcriptions_url`, SiliconFlow uses
  `providers.siliconflow.transcriptions_url`, Zhipu uses
  `providers.zhipu.transcriptions_url`, and Grok/xAI uses
  `providers.grok.transcriptions_url`.
* Gemini text requests use the native `generateContent` route and normalize
  Gemini usage metadata into the same response headers and JSON `usage` object
  used by the other text providers.
* Anthropic text requests use `POST /v1/messages` with `x-api-key` and
  `anthropic-version: 2023-06-01`. System messages are translated to
  Anthropic's top-level `system` field. Anthropic requires `max_tokens`, so
  when the client omits it the proxy sends the selected Claude model's
  configured output limit.
* Zhipu dictation uses Z.AI GLM-ASR through
  `providers.zhipu.transcriptions_url` with the selected configured dictation
  model.
* Grok text requests use xAI's OpenAI-compatible `/chat/completions` API at
  `https://api.x.ai/v1`. Grok/xAI dictation uses xAI STT through
  `providers.grok.transcriptions_url`; the upstream STT endpoint does not
  receive a `model` multipart field.

Provider API keys are optional until a tenant uses that provider as a default.
If a non-default provider key is blank or its whole `api_key` value is a missing
`${...}` placeholder, startup continues and explicit requests for that provider
return `503 provider not configured`. Missing placeholders in other fields, or
embedded inside a longer `api_key` value, fail startup. If a tenant's default
text or dictation provider lacks its API key, startup fails before the server
listens. Provider `base_url` values are explicit config values; leave them at
the documented URLs unless routing through a test server, proxy, or compatible
gateway. Dictation-capable provider
`transcriptions_url` values are also explicit config values and are required for
OpenAI, SiliconFlow, Zhipu, and Grok/xAI. Text model catalogs are required for
every supported provider, and dictation model catalogs are required for OpenAI,
SiliconFlow, Zhipu, and Grok/xAI. When `management.enabled` is false, startup
validates that `tenants` includes at least one unique `id` and unique `secret`.
When management is enabled, static tenants are optional migration seed data
because signed-in users can create managed tenants through the UI, and
DB-authoritative runtime authentication does not validate static tenant
provider credentials.
Unknown YAML keys fail startup.

### Self-service management UI

Set `management.enabled: true` to enable TAuth-protected management APIs under
`/api/management`. The browser UI is static and lives in `site/`, which is
published to the `gh-pages` branch by `make publish-pages`, `make publish`, or
`make deploy`. GitHub Actions is not used for Pages deployment. The backend does
not serve management HTML or assets; `GET /` remains a proxy endpoint and
returns `403` without a tenant `key`. The backend does serve public
`/config-ui.yaml` from the loaded management config so the GitHub Pages frontend
can consume the current llm-proxy runtime, MPR UI, and TAuth bootstrap values
from `llm-proxy-api`.

The static UI uses the shared MPR shell through API-served `config-ui.yaml`,
pinned `mpr-ui` assets, `<mpr-header>`, `<mpr-user>`, and `<mpr-footer>`. It
does not load `tauth.js` directly. The Pages artifact contains no static
`config-ui.yaml`, no `llm-proxy-config.json`, and no rendered config URL in
`index.html`; the frontend fetches the backend `/config-ui.yaml` endpoint at
runtime. That single API-served YAML points browser management API calls,
generated usage examples, and MPR UI/TAuth at the configured origins.
Browser-facing values are projected from the already-loaded backend
`config.yml`; there is no second environment expansion path for Pages.

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
| `management.jwt_signing_key` | Internal signing key used to validate the TAuth session cookie. |
| `management.jwt_issuer` | JWT issuer, normally `tauth`. |
| `management.session_cookie_name` | Exact app/environment TAuth session cookie name. |
| `management.database_dialect` | Required GORM SQL dialect for management persistence. Supported values are `postgres` and `sqlite`; SQLite uses the pure-Go GORM driver so `CGO_ENABLED=0` builds remain valid. |
| `management.database_dsn` | Required DSN passed to the selected GORM dialect for tenant-owned provider keys, defaults, generated-secret digests, and usage events. |
| `management.provider_key_encryption_key` | Required base64-encoded 32-byte key used for AES-GCM encryption of tenant-owned provider API keys at rest. Generate with `openssl rand -base64 32` and store it with backend deployment secrets. |
| `management.management_api_origin` | Browser-facing management API origin served from `/config-ui.yaml` under `llmProxy.managementApiOrigin`. |
| `management.proxy_origin` | Browser-facing public proxy origin served from `/config-ui.yaml` under `llmProxy.proxyOrigin` for generated examples. |

Signed-in users save provider API keys for any supported provider, choose
routing defaults, and generate llm-proxy secrets. Management mode requires
`management.database_dialect` and `management.database_dsn` so signups, enabled
providers, defaults, generated secret digests, and usage events survive restarts
in a GORM-managed database. `postgres` uses a Postgres DSN, while `sqlite` uses
a SQLite database path or SQLite DSN. The packaged management config uses
strict expandable placeholders for the hosted profile values; define every
`LLM_PROXY_MANAGEMENT_*` key in the runtime environment or local `configs/.env`.
Placeholders without matching values fail startup. The runtime config file is
never mutated for user signup, provider enablement, or usage tracking, and
database access must stay on GORM model APIs without raw SQL. Generated secrets
continue to authenticate the public proxy endpoints with the same
`key=<tenant secret>` query parameter. Provider API keys are accepted only
through authenticated management endpoints, are encrypted at rest with AES-GCM
before database persistence, and after save the UI/API returns only masked key
status. Existing plaintext provider-key rows are encrypted and cleared during
management startup. The backend decrypts provider keys only inside the runtime
path that routes requests to upstream providers, so this protects database
dumps, backups, and direct storage access; it is not a user-only decryption or
zero-knowledge guarantee. Generated tenant secrets are returned once and the
database retains only their SHA-256 digest. Revoking a generated secret
immediately makes future public proxy requests with that secret return `403`.

The authenticated landing screen is a usage dashboard. It shows 30-day request
and token graphs, total request and token counts, success rate, and provider and
model breakdowns for the signed-in user's managed tenant. The prior dashboard
controls now live in a large Settings modal opened from the avatar dropdown;
the `Settings` menu item is inserted before `Sign out` through the shared
`<mpr-user>` menu contract. The modal contains client access, generated secret,
routing defaults, request examples, and provider key management.

Administrators are configured only through `management.admin_emails`; use the
plural `${LLM_PROXY_MANAGEMENT_ADMIN_EMAILS}` placeholder in public config files
and define the real value as a YAML flow sequence in the runtime environment or
ignored `configs/.env`. When the validated TAuth
session email matches that list, the profile response includes
`user.is_admin: true`, the shared avatar menu gets an `Admin` item, and
`GET /api/management/admin/users` returns all managed users with tenant facts
and 30-day usage summaries. Admin responses never include provider API keys,
masked provider-key strings, generated tenant secrets, secret digests, prompts,
audio names, transcripts, or model responses. Authenticated non-admin users get
`403 Forbidden` from admin-only APIs.

`GET /api/management/usage` returns the dashboard data for the authenticated
user. Usage events are recorded only for managed tenants when they call the
public proxy endpoints with a generated secret. Stored usage metadata includes
endpoint, provider, model, status code, success flag, latency, and normalized
request/response/total token counts. Prompts, audio, transcripts, responses,
tenant secrets, and provider API keys are not stored in usage events.

On the first management-mode startup, llm-proxy runs a one-time migration from
legacy config tenants and nonblank configured provider API keys into the
database, then records a migration marker. After that marker exists, config-file
tenants and provider API key fields are ignored by runtime authentication and
routing, even if stale values remain in YAML. Remove migrated `tenants` and
provider `api_key` values from config after confirming the DB migration.
Server/runtime settings, backend auth validation settings, provider base URLs,
transcription URLs, model catalogs, and browser-facing MPR UI/TAuth bootstrap
settings remain config-file-owned. The GitHub Pages artifact is only the static
shell; API-served browser config endpoints are projections of backend
`config.yml`, not independent configuration sources.

### Hosted split-origin setup

Production is split-origin:

| Hostname | Owner | Purpose |
|----------|-------|---------|
| `llm-proxy.mprlab.com` | GitHub Pages | Static self-service frontend from `site/`. |
| `llm-proxy-api.mprlab.com` | MPR gateway/backend | llm-proxy API, management API, `/`, `/v2`, and `/dictate`. |
| `tauth-api.mprlab.com` | TAuth backend | Google login, nonce, logout, `/me`, and session-cookie issuance. |

Add these DNS records:

1. `CNAME llm-proxy.mprlab.com -> tyemirov.github.io`
2. Point `llm-proxy-api.mprlab.com` at the MPR gateway public endpoint. Use a
   `CNAME` when the gateway has a hostname, or `A`/`AAAA` records when it is
   addressed by public IP.

Then configure GitHub Pages for this repository:

1. Use branch publishing from `gh-pages` at `/`.
2. Set the Pages custom domain to `llm-proxy.mprlab.com`.
3. Publish Pages from this repository with `make publish-pages`, or as part of
   `make publish` and `make deploy`. The publisher renders `site/`, verifies no
   static browser config files or rendered config URL are present, pushes the
   `gh-pages` branch, and configures the repository Pages source through the
   GitHub API unless `--skip-configure` is passed.
4. Configure real backend deployment secrets outside the Pages artifact:
   `SERVICE_SECRET`, `LLM_PROXY_MANAGEMENT_ADMIN_EMAILS`,
   `LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY`,
   `LLM_PROXY_MANAGEMENT_DATABASE_DSN`,
   `LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY`.
5. Do not store browser runtime config in the Pages branch. Production browser
   config is served only by `https://llm-proxy-api.mprlab.com/config-ui.yaml`
   from the running backend's loaded management config.

Configure TAuth for tenant `llm-proxy` with:

- allowed tenant origin `https://llm-proxy.mprlab.com`
- browser-facing API origin `https://tauth-api.mprlab.com`
- session cookie name matching `management.session_cookie_name`
- cookie domain `.mprlab.com`
- HTTPS-only cookies
- JWT signing key matching `management.jwt_signing_key`

Configure the gateway/backend route for `llm-proxy-api.mprlab.com` to the
llm-proxy service, and remove any backend route that still claims
`llm-proxy.mprlab.com`; that hostname is now owned by GitHub Pages. The backend
must run with `management.public_origin: "https://llm-proxy.mprlab.com"` so
`/config-ui.yaml` and `/api/management/*` return credentialed CORS headers only
to the static frontend.

Web search is per request and currently supported only on OpenAI models that
support the OpenAI web search tool.
Text output length is also per request: pass `max_tokens` when a client wants
to cap one generation. When omitted, the proxy does not send a provider
max-token field, except Anthropic Messages where `max_tokens` is required
upstream and the proxy sends the selected model's configured output limit.
Provider-specific output-token limits are enforced at the request edge when
known. Gemini text models currently reject `max_tokens` above `65536`; Claude
models reject values above the configured synchronous Messages output limit.
Those errors return `400 Bad Request` before any upstream provider call.

## Running

Generate a secret:

```shell
openssl rand -hex 32
```

Run the service with OpenAI defaults:

```shell
./llm-proxy --config config.yml
```

In static config mode, set a tenant's default text provider/model to route
omitted-provider requests to DeepSeek. In management mode these tenant blocks
are one-time migration seed data only; after the migration marker exists, manage
tenant defaults in the UI/DB instead:

```yaml
tenants:
  - id: deepseek
    secret: "${SERVICE_SECRET}"
    defaults:
      provider: deepseek
      model: deepseek-v4-flash
```

Set Gemini as the default text provider:

```yaml
tenants:
  - id: gemini
    secret: "${SERVICE_SECRET}"
    defaults:
      provider: gemini
      model: gemini-2.5-flash
```

Set Anthropic as the default text provider:

```yaml
tenants:
  - id: anthropic
    secret: "${SERVICE_SECRET}"
    defaults:
      provider: anthropic
      model: claude-sonnet-4-6
```

Set Grok as the default text provider:

```yaml
tenants:
  - id: grok
    secret: "${SERVICE_SECRET}"
    defaults:
      provider: grok
      model: grok-4.3
```

## Local Automation

This repository exposes the standard local targets used by MPR app repos:

| Command | Purpose |
|---------|---------|
| `npm ci` | Install pinned frontend validation dependencies before running local frontend checks. |
| `make ci` | Run format checks, Go lint (`go vet`, `staticcheck`, `ineffassign`), Python strict mypy, frontend syntax checks, the 100% coverage-gated Go test suite, Python pytest, and Playwright browser tests. |
| `make test-live-providers` | Generate a complete temporary config and run live text smoke tests for every provider whose API key is present; use `LIVE_ENV_FILE=/path/to/env` to load interpolation values. |
| `make test-live-gemini` | Compatibility wrapper for `make test-live-providers` with `LLM_PROXY_LIVE_PROVIDERS=gemini`. |
| `make release` | Cut a `v*` release from `master`, update `CHANGELOG.md` when needed, and push the release tag. |
| `make publish-pages` | Render `site/`, verify it has no static browser config, push the `gh-pages` branch, and configure branch-based GitHub Pages without Actions. |
| `make publish` | Validate the release source, publish `ghcr.io/tyemirov/llm-proxy:<tag>` plus `:latest`, and publish the Pages branch. |
| `make deploy` | Verify the published image, publish the Pages branch, and deploy the backend through the sibling `../mprlab-gateway` checkout. |

Live provider smoke tests are intentionally not part of `make ci`; they call
paid upstream APIs and depend on local or CI secret availability. The dynamic
target discovers these provider keys after loading `LIVE_ENV_FILE`. By default,
smoke requests omit `model`, so each provider uses the default configured in
the selected `configs/config.yml`; set a model override only when debugging a
specific provider/model pair.

| Provider | Key variable | Model override |
|----------|--------------|----------------|
| OpenAI | `OPENAI_API_KEY` | `LLM_PROXY_LIVE_OPENAI_MODEL` |
| DeepSeek | `DEEPSEEK_API_KEY` | `LLM_PROXY_LIVE_DEEPSEEK_MODEL` |
| DashScope/Qwen | `DASHSCOPE_API_KEY` | `LLM_PROXY_LIVE_DASHSCOPE_MODEL` |
| Moonshot/Kimi | `MOONSHOT_API_KEY` | `LLM_PROXY_LIVE_MOONSHOT_MODEL` |
| SiliconFlow | `SILICONFLOW_API_KEY` | `LLM_PROXY_LIVE_SILICONFLOW_MODEL` |
| Zhipu/GLM | `ZHIPU_API_KEY` | `LLM_PROXY_LIVE_ZHIPU_MODEL` |
| Gemini | `GEMINI_API_KEY` | `LLM_PROXY_LIVE_GEMINI_MODEL` |
| Anthropic/Claude | `ANTHROPIC_API_KEY` | `LLM_PROXY_LIVE_ANTHROPIC_MODEL` |
| Grok/xAI | `XAI_API_KEY` | `LLM_PROXY_LIVE_GROK_MODEL` |

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

The release lifecycle commands wrap their local `make ci` gate with the
standard 350-second timeout by default. For exceptional local diagnostics,
override all three with `LLM_PROXY_CI_TIMEOUT_SECONDS=<seconds>`, or use the
command-specific `RELEASE_CI_TIMEOUT_SECONDS`, `PUBLISH_CI_TIMEOUT_SECONDS`,
and `DEPLOY_CI_TIMEOUT_SECONDS` variables.

`llm-proxy` is a gateway-local service in `mprlab-gateway`, so `make deploy`
defaults to the gateway `deploy-llm-proxy-backend` target. Override the
checkout or target with `GATEWAY_DIR=/path/to/mprlab-gateway` or
`GATEWAY_DEPLOY_TARGET=<target>`. Override Pages publishing with
`PAGES_BRANCH=<branch>`, `PAGES_DOMAIN=<domain>`, or
`PUBLISH_PAGES_ARGS="--skip-configure"` when an operator must separate the
GitHub API source configuration from the branch push.

## Usage

### Installable prompt client

Install the reusable JSON POST client:

```shell
go install github.com/tyemirov/llm-proxy/llm-proxy-client@latest
```

Use it with explicit flags:

```shell
llm-proxy-client \
  --base-url "http://localhost:8080/?provider=gemini" \
  --secret "$SERVICE_SECRET" \
  --model gemini-2.5-flash \
  --prompt "Summarize this"
```

Or read configuration and prompt text from environment/stdin:

```shell
export LLM_PROXY_BASE_URL="http://localhost:8080/"
export LLM_PROXY_SECRET="$SERVICE_SECRET"
printf 'large prompt...\n' | llm-proxy-client --model gpt-5.5 --max-tokens 4096
```

The client always uses canonical `POST /v2?key=...` with a JSON body. It keeps
non-payload query parameters such as `provider`, strips body-owned query fields
such as `prompt` and `model`, and sends the prompt as a v2 `user` message.
`--system-prompt` becomes a v2 `system` message. Optional `model`,
`web_search`, and `max_tokens` values remain body fields. When `--model` is
omitted, the body omits `model` so llm-proxy uses the selected provider's
configured default model.

The reusable Go package under `pkg/llmproxyclient` is v2-only: construct a
`MessagesRequest` with `NewMessagesRequest` and send it with
`Client.PostMessages`.

### Python client package

The same transport contract is available as an importable Python package:

```shell
uv pip install "llm-proxy-client @ git+https://github.com/tyemirov/llm-proxy.git"
```

```python
from llm_proxy_client import Client, ClientConfig, ClientMessagesRequest, ClientMessage

client = Client(
    ClientConfig(
        base_url="http://localhost:8080/?provider=gemini",
        secret="mysecret",
    )
)

text = client.post_messages(
    ClientMessagesRequest(
        messages=(ClientMessage(role="user", content="Summarize this"),),
        model="gemini-2.5-flash",
        max_tokens=512,
    )
)
```

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

The optional `order` field is for callers that do not want to rely on array
position. When any message includes `order`, every submitted message must
include a unique non-negative integer `order`; the proxy sorts ascending before
provider routing and echoes provided order values in JSON responses.

For local development:

```shell
uv pip install -e .
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

Gemini text generation:

```shell
curl --get \
  --data-urlencode "prompt=Summarize this with Gemini" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "provider=gemini" \
  --data-urlencode "model=gemini-2.5-flash" \
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

Grok text generation:

```shell
curl --get \
  --data-urlencode "prompt=Summarize this with Grok" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "provider=grok" \
  --data-urlencode "model=grok-4.3" \
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
  --data '{"prompt":"large text...","model":"gpt-5.5","web_search":false,"system_prompt":"optional","max_tokens":4096}' \
  "http://localhost:8080/?key=mysecret"
```

Compatibility chat transcript on `/`:

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

JSON body fields:

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `prompt` | Yes, unless `messages` is provided | none | Full text to send to the LLM. Use this body field for large or non-ASCII prompts. |
| `messages` | Yes, unless `prompt` is provided | none | Chat messages using `role` and string `content`. Supported roles are `system`, `user`, and `assistant`; at least one `user` message is required. Each item may include numeric `order`; if any item includes it, every submitted item must include a unique non-negative `order`, and messages are sorted ascending before routing. |
| `model` | No | tenant or configured provider default | Model identifier from the selected provider's configured model list. Omitted model uses the tenant default when `provider` is omitted; otherwise it uses the selected provider's configured default. |
| `web_search` | No | `false` | Enables OpenAI web search when the selected provider/model supports it. |
| `system_prompt` | No | authenticated tenant default | Per-request system prompt override. With `messages`, it is prepended as a system message only when the body does not already contain a system message. |
| `max_tokens` | No | provider default | Positive integer output-token cap for this request. The proxy maps it to OpenAI `max_output_tokens`, OpenAI-compatible `max_tokens`, Anthropic `max_tokens`, or Gemini `generationConfig.maxOutputTokens`. |

For `POST /`, `provider` remains a query parameter. Query `model` may override
the JSON body only when the body omits `model` or provides the same value;
conflicting values return `400 Bad Request`.
Bodies that provide both `prompt` and `messages`, empty `messages`, unsupported
message roles, empty message content, partially specified `order`, duplicate
or negative `order`, or both `system_prompt` and a system message return
`400 Bad Request` before any upstream call.
Gemini `max_tokens` values above `65536` return `400 Bad Request` before the
proxy calls Gemini. Anthropic `max_tokens` values above the configured Claude
model output limit return `400 Bad Request` before the proxy calls Anthropic.

`POST /v2` is the canonical chat endpoint. It accepts the same `messages`,
`model`, `web_search`, and `max_tokens` body fields, but rejects `prompt` and
body `system_prompt`; send a `system` role message instead. The tenant default
system prompt is still prepended when the submitted messages do not include a
system message.

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
  --data-urlencode "web_search=1" \
  "http://localhost:8080/"
```

You can enable web search with GPT-5 by specifying the model:

```shell
curl --get \
  --data-urlencode "prompt=Latest research on quantum gravity" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "model=gpt-5" \
  --data-urlencode "web_search=1" \
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

## Endpoint

### LLM endpoint

```text
GET /
  ?prompt=STRING            # required
  &key=SERVICE_SECRET       # required
  &provider=PROVIDER        # optional; tenant default
  &model=MODEL_NAME         # optional; tenant or configured provider default
  &web_search=1|true|yes    # optional; requires configured model support
  &max_tokens=N             # optional positive integer per-request cap
  &format=CONTENT_TYPE      # optional; or use Accept header
```

```text
POST /
  ?key=SERVICE_SECRET       # required
  &provider=PROVIDER        # optional; tenant default
  &model=MODEL_NAME         # optional; overrides JSON body if absent or equal
  &format=CONTENT_TYPE      # optional; or use Accept header
Content-Type: application/json
{
  "prompt": "STRING",       # required unless messages is provided
  "messages": [             # required unless prompt is provided
    {"role": "user", "content": "STRING", "order": 1}
  ],
  "model": "MODEL_NAME",    # optional; tenant or configured provider default
  "web_search": false,      # optional; defaults to false
  "system_prompt": "STRING",# optional; tenant default
  "max_tokens": 512         # optional positive integer per-request cap
}
```

```text
POST /v2
  ?key=SERVICE_SECRET       # required
  &provider=PROVIDER        # optional; tenant default
  &model=MODEL_NAME         # optional; overrides JSON body if absent or equal
  &format=CONTENT_TYPE      # optional; or use Accept header
Content-Type: application/json
{
  "messages": [             # required
    {"role": "user", "content": "STRING", "order": 1}
  ],
  "model": "MODEL_NAME",    # optional; tenant or configured provider default
  "web_search": false,      # optional; defaults to false
  "max_tokens": 512         # optional positive integer per-request cap
}
```

The POST JSON body carries only LLM request parameters. The tenant secret
remains in the `key` query parameter, and upstream provider API keys are
never accepted from client requests.

### Dictation endpoint

```text
POST /dictate
  ?key=SERVICE_SECRET       # required
  &provider=PROVIDER        # optional; tenant default
  &model=MODEL_NAME         # optional; tenant or configured provider default
Content-Type: multipart/form-data
  audio=<file>              # required (alias: file)
```

Success response:

```json
{ "text": "..." }
```

The default model catalog in [configs/config.yml](configs/config.yml)
declares the LLM endpoint models below. The `/dictate` endpoint defaults to
OpenAI's audio transcriptions API and also supports SiliconFlow, Zhipu, and
Grok/xAI through their provider selectors. Not all configured models support
tools; use a model marked `Yes` below for web search. A dash in the proxy
`max_tokens` limit column means the proxy validates only that `max_tokens` is
positive and lets the upstream provider enforce any provider-side model limit.

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
| `deepseek-v4-flash` | DeepSeek | Yes | - | No |
| `deepseek-v4-pro` | DeepSeek | No | - | No |
| `deepseek-chat` | DeepSeek | No | - | No |
| `deepseek-reasoner` | DeepSeek | No | - | No |
| `qwen-plus` | DashScope/Qwen | Yes | - | No |
| `kimi-k2-0905-preview` | Moonshot/Kimi | Yes | - | No |
| `deepseek-ai/DeepSeek-R1` | SiliconFlow | Yes | - | No |
| `glm-5.1` | Zhipu/GLM | Yes | - | No |
| `gemini-3.5-flash` | Gemini | No | `65536` | No |
| `gemini-3.1-flash-lite` | Gemini | No | `65536` | No |
| `gemini-2.5-flash` | Gemini | Yes | `65536` | No |
| `gemini-2.5-flash-lite` | Gemini | No | `65536` | No |
| `gemini-2.5-pro` | Gemini | No | `65536` | No |
| `claude-opus-4-8` | Anthropic/Claude | No | `128000` | No |
| `claude-sonnet-4-6` | Anthropic/Claude | Yes | `64000` | No |
| `claude-haiku-4-5-20251001` | Anthropic/Claude | No | `64000` | No |
| `claude-haiku-4-5` | Anthropic/Claude | No | `64000` | No |
| `claude-sonnet-4-5-20250929` | Anthropic/Claude | No | `64000` | No |
| `claude-sonnet-4-5` | Anthropic/Claude | No | `64000` | No |
| `claude-opus-4-1-20250805` | Anthropic/Claude | No | `32000` | No |
| `claude-opus-4-1` | Anthropic/Claude | No | `32000` | No |
| `grok-4.3` | Grok/xAI | Yes | - | No |
| `grok-4.3-latest` | Grok/xAI | No | - | No |
| `grok-latest` | Grok/xAI | No | - | No |
| `grok-build-0.1` | Grok/xAI | No | - | No |
| `grok-code-fast` | Grok/xAI | No | - | No |
| `grok-code-fast-1` | Grok/xAI | No | - | No |
| `grok-code-fast-1-0825` | Grok/xAI | No | - | No |

### Dictation capabilities

| Provider selector | Models | Credential field | Transcription URL field | Notes |
|-------------------|--------|------------------|-------------------------|-------|
| `openai` | `gpt-4o-mini-transcribe`, `gpt-4o-transcribe` | `providers.openai.api_key` | `providers.openai.transcriptions_url` | Default dictation provider and default model `gpt-4o-mini-transcribe`. |
| `siliconflow` | `FunAudioLLM/SenseVoiceSmall` | `providers.siliconflow.api_key` | `providers.siliconflow.transcriptions_url` | OpenAI-compatible audio transcription. |
| `zhipu` / `glm` | `glm-asr-2512` | `providers.zhipu.api_key` | `providers.zhipu.transcriptions_url` | Z.AI GLM-ASR; sends `model=glm-asr-2512`. |
| `grok` / `xai` | `xai-stt` | `providers.grok.api_key` | `providers.grok.transcriptions_url` | xAI STT; the proxy model name selects the provider but is not sent as a multipart `model` field. |

### Status codes

* `200 OK` - success
* `400 Bad Request` - missing/invalid parameters, invalid multipart audio form, unknown provider/model, or unsupported provider capability
* `403 Forbidden` - missing or invalid `key`
* `413 Payload Too Large` - JSON prompt body exceeds `max_prompt_bytes`
* `429 Too Many Requests` - upstream provider rate limit
* `503 Service Unavailable` - selected provider credential is unavailable because that non-default provider is disabled or missing its API key
* `504 Gateway Timeout` - the overall proxy request timed out before the selected upstream provider returned a final result
* `502 Bad Gateway` - upstream provider API returned an error

## Security

* All requests must include a configured tenant secret via `key=...`.
* Client requests must not include upstream provider API keys; public proxy endpoints reject provider-key-like query, JSON, and multipart form fields.
* Self-service provider API keys are accepted only through TAuth-protected management endpoints and are not returned raw after save.
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

Use `make release` from a clean, up-to-date `master` branch. It runs `make ci`,
updates `CHANGELOG.md` if the selected version is missing, creates the release
commit when needed, and pushes the `v*` tag. Tags that begin with `v` trigger
the release workflow, which builds and publishes release artifacts and uses the
matching changelog section as release notes.

Use `make publish` when you need to manually publish the release image and
refresh the GitHub Pages branch from the same source revision. Use
`make publish-pages` only for a Pages-only refresh. Use `make deploy` after the
release image is published and `:latest` points at the same digest as the
release tag; deploy republishes Pages before the gateway backend target. Pages
deployment is intentionally owned by these Makefile commands, not by GitHub
Actions.

## License

This project is licensed under the MIT License. See [LICENSE](MIT-LICENSE) for
details.
