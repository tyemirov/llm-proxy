# LLM Proxy

LLM Proxy is a lightweight HTTP service that forwards user prompts to OpenAI's
Responses API, OpenAI-compatible chat providers, Anthropic's native Messages
API, Google Gemini's native generateContent API, and audio transcription APIs.
It exposes protected HTTP endpoints that require a tenant secret and simplify
integrating provider capabilities without embedding API credentials in each
client.

## Features

- Minimal HTTP server whose complete owned operation surface is defined by the
  [canonical OpenAPI contract](docs/openapi.yaml)
- Choose the provider per request via `provider=...`; omitted provider uses the authenticated tenant default
- Choose the model per request via `model=...`; omitted model uses the tenant default when `provider` is omitted, otherwise the selected provider's configured default
- Set optional nonblank `reasoning_effort=...` on `GET /`, or in a JSON body for `POST /` and `POST /v2`, to select a capability-supported reasoning level for that exact resolved route. An explicit value overrides the tenant default; an omitted value retains it. Blank or unsupported values fail before an upstream call.
- Choose the dictation model per request via `model=...` on `/dictate`; omitted model uses the tenant default when `provider` is omitted, otherwise the selected provider's configured default
- Optional per-request web search via `web_search=1|true|yes` when the selected provider/model is configured to support it
- Optional logging at `debug` or `info` levels
- Forwards requests using server-side provider API keys, loaded from the database in management mode
- Optional TAuth-protected self-service UI where signed-in users automatically receive an llm-proxy client key and their provider settings plus routing defaults autosave
- Supports plain text, JSON, XML, or CSV responses

## REST Contract

llm-proxy exposes a blocking REST contract for text generation. A caller sends
one authenticated `GET /`, `POST /`, or `POST /v2` request and receives the final
formatted answer in that same HTTP response.

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
serve another schema location.

The caller does not stream tokens, poll a job endpoint, or follow a resume
token. OpenAI Responses is the only current text adapter with a pollable
upstream lifecycle: llm-proxy sends stored background requests and keeps the
client request open while it polls a nonblank response id reported with
`status=queued` or `status=in_progress`, or returned by a proxy-initiated
stored background synthesis. A documented terminal status is resolved
immediately, and a missing or unknown status is rejected rather than polled.
The response id remains in the active request lifecycle; llm-proxy has no
durable provider-job queue or later resume endpoint.

The current Chat Completions, Gemini `generateContent`, and Anthropic Messages
routes are synchronous. Their text is successful only when the same response
reports a complete provider-specific stop signal: Chat
`finish_reason=stop`, Gemini `finishReason=STOP`, or Anthropic
`stop_reason=end_turn|stop_sequence`. Missing, truncated, tool/intermediate, or
unknown stop signals return `502` without partial text.

A `504 Gateway Timeout` means the overall proxy request deadline expired before
the selected upstream provider produced a final answer. It is not a prompt for
the client to poll llm-proxy.

### Request work budgets

Every authenticated operation that can start upstream work accepts one optional
header:

```text
X-LLM-Proxy-Request-Timeout-Seconds: N
```

`N` is the positive whole-number wall-clock budget, in seconds, for that
request. The budget begins before body parsing and covers validation, queue
admission, every provider call, OpenAI background polling, and response
construction for `GET /`, `POST /`, `POST /v2`, and `POST /dictate`.

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

For managed tenants, usage persistence begins only after the selected response
status and body have been written and the response writer has been flushed.
The store lock and database calls run in a detached, five-second persistence
budget so a selected timeout or caller cancellation does not erase its terminal
usage event. Persistence still happens after response selection and never
changes an already selected response.

Internally, `server.workers` limits concurrent upstream provider HTTP
operations and `server.queue_size` limits upstream HTTP operations waiting for a
worker. Long OpenAI background-response poll sleeps do not occupy a worker slot;
only the actual upstream HTTP request or poll does. This admission queue stores
no provider job ids and provides no durable retry or resume behavior.

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
  max_request_timeout_seconds: 3600
  max_prompt_bytes: 4194304
  max_input_audio_bytes: 26214400
  upstream_rate_limits: []
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
  session_path: "${LLM_PROXY_MANAGEMENT_TAUTH_SESSION_PATH}"
  jwt_signing_key: "${LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY}"
  jwt_issuer: "${LLM_PROXY_MANAGEMENT_JWT_ISSUER}"
  session_cookie_name: "${LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME}"
  database_path: "${LLM_PROXY_MANAGEMENT_DATABASE_PATH}"
  provider_key_encryption_key: "${LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY}"
  management_api_origin: "${LLM_PROXY_MANAGEMENT_API_ORIGIN}"
  proxy_origin: "${LLM_PROXY_MANAGEMENT_PROXY_ORIGIN}"
providers:
  openai:
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
          request_profile: "openai_responses_reasoning_tools"
          reasoning_effort:
            adapter: "openai_responses"
            efforts: ["minimal", "low", "medium", "high"]
        - id: "gpt-5"
          request_profile: "openai_responses_reasoning_tools"
          web_search: true
          reasoning_effort:
            adapter: "openai_responses"
            efforts: ["minimal", "low", "medium", "high"]
        - id: "gpt-5.5"
          request_profile: "openai_responses_reasoning_tools"
          web_search: true
          reasoning_effort:
            adapter: "openai_responses"
            efforts: ["none", "low", "medium", "high", "xhigh"]
        - id: "gpt-5.5-pro"
          request_profile: "openai_responses_reasoning_tools"
          web_search: true
          reasoning_effort:
            adapter: "openai_responses"
            efforts: ["medium", "high", "xhigh"]
        - id: "gpt-5.6"
          request_profile: "openai_responses_reasoning_tools"
          web_search: true
          reasoning_effort:
            adapter: "openai_responses"
            efforts: ["none", "low", "medium", "high", "xhigh", "max"]
        - id: "gpt-5.6-sol"
          request_profile: "openai_responses_reasoning_tools"
          web_search: true
          reasoning_effort:
            adapter: "openai_responses"
            efforts: ["none", "low", "medium", "high", "xhigh", "max"]
        - id: "gpt-5.6-terra"
          request_profile: "openai_responses_reasoning_tools"
          web_search: true
          reasoning_effort:
            adapter: "openai_responses"
            efforts: ["none", "low", "medium", "high", "xhigh", "max"]
        - id: "gpt-5.6-luna"
          request_profile: "openai_responses_reasoning_tools"
          web_search: true
          reasoning_effort:
            adapter: "openai_responses"
            efforts: ["none", "low", "medium", "high", "xhigh", "max"]
    dictation:
      default_model: "gpt-4o-mini-transcribe"
      models:
        - id: "gpt-4o-mini-transcribe"
        - id: "gpt-4o-transcribe"
  meta:
    base_url: "https://api.meta.ai/v1"
    text:
      default_model: "muse-spark-1.1"
      models:
        - id: "muse-spark-1.1"
  deepseek:
    base_url: "https://api.deepseek.com"
    text:
      default_model: "deepseek-v4-flash"
      models:
        - id: "deepseek-v4-flash"
        - id: "deepseek-v4-pro"
        - id: "deepseek-chat"
        - id: "deepseek-reasoner"
  dashscope:
    base_url: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
    text:
      default_model: "qwen-plus"
      models:
        - id: "qwen-plus"
  qwencloud:
    base_url: "https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1"
    text:
      default_model: "qwen3.8-max-preview"
      models:
        - id: "qwen3.8-max-preview"
  moonshot:
    base_url: "https://api.moonshot.ai/v1"
    text:
      default_model: "kimi-k2-0905-preview"
      models:
        - id: "kimi-k2-0905-preview"
        - id: "kimi-k3"
        - id: "kimi-k2.7-code"
        - id: "kimi-k2.7-code-highspeed"
        - id: "kimi-k2.6"
  minimax:
    base_url: "https://api.minimax.io/v1"
    text:
      default_model: "MiniMax-M2.7"
      models:
        - id: "MiniMax-M2.7"
          output_token_limit: 2048
  siliconflow:
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
    base_url: "https://open.bigmodel.cn/api/paas/v4"
    transcriptions_url: "https://api.z.ai/api/paas/v4/audio/transcriptions"
    text:
      default_model: "glm-5.1"
      models:
        - id: "glm-5.1"
        - id: "glm-5.2"
          output_token_limit: 131072
    dictation:
      default_model: "glm-asr-2512"
      models:
        - id: "glm-asr-2512"
  gemini:
    base_url: "https://generativelanguage.googleapis.com/v1"
    text:
      default_model: "gemini-2.5-flash"
      models:
        - id: "gemini-3.5-flash"
          output_token_limit: 65536
        - id: "gemini-3.1-pro-preview"
          output_token_limit: 65536
        - id: "gemini-3-flash-preview"
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
    base_url: "https://api.anthropic.com"
    text:
      default_model: "claude-sonnet-4-6"
      models:
        - id: "claude-fable-5"
          output_token_limit: 128000
        - id: "claude-sonnet-5"
          output_token_limit: 128000
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
    base_url: "https://api.x.ai/v1"
    transcriptions_url: "https://api.x.ai/v1/stt"
    text:
      default_model: "grok-4.3"
      models:
        - id: "grok-4.3"
        - id: "grok-4.3-latest"
        - id: "grok-4.5"
        - id: "grok-4.20-0309-reasoning"
        - id: "grok-4.20-0309-non-reasoning"
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
`provider` is omitted; otherwise they use the selected provider's configured
default text model. This table describes capabilities currently wired through
`llm-proxy` and the defaults shipped in [configs/config.yml](configs/config.yml).
Upstream providers may expose additional speech APIs that need separate proxy
adapters before they are available through `/dictate`.

| Provider selector | Aliases | Text API | Configured default text model | Credential field | Default base URL | Dictation | Web search |
|-------------------|---------|----------|-------------------------------|------------------|------------------|-----------|------------|
| `openai` | none | OpenAI Responses | `gpt-4.1` | `providers.openai.api_key` | `https://api.openai.com/v1` | Yes: `gpt-4o-mini-transcribe`, `gpt-4o-transcribe` | Yes, on marked OpenAI models |
| `meta` | none | Meta Model API OpenAI-compatible chat completions | `muse-spark-1.1` | `providers.meta.api_key` | `https://api.meta.ai/v1` | No | No |
| `deepseek` | none | OpenAI-compatible chat completions | `deepseek-v4-flash` | `providers.deepseek.api_key` | `https://api.deepseek.com` | No | No |
| `dashscope` | `qwen` | OpenAI-compatible chat completions | `qwen-plus` | `providers.dashscope.api_key` | `https://dashscope-intl.aliyuncs.com/compatible-mode/v1` | No | No |
| `qwencloud` | none | Qwen Cloud Token Plan OpenAI-compatible chat completions | `qwen3.8-max-preview` | `providers.qwencloud.api_key` | `https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1` | No | No |
| `moonshot` | `kimi` | OpenAI-compatible chat completions | `kimi-k2-0905-preview` | `providers.moonshot.api_key` | `https://api.moonshot.ai/v1` | No | No |
| `minimax` | none | MiniMax OpenAI-compatible chat completions | `MiniMax-M2.7` | `providers.minimax.api_key` | `https://api.minimax.io/v1` | No | No |
| `siliconflow` | none | OpenAI-compatible chat completions | `deepseek-ai/DeepSeek-R1` | `providers.siliconflow.api_key` | `https://api.siliconflow.com/v1` | Yes: `FunAudioLLM/SenseVoiceSmall` | No |
| `zhipu` | `glm` | OpenAI-compatible chat completions | `glm-5.1` | `providers.zhipu.api_key` | `https://open.bigmodel.cn/api/paas/v4` | Yes: `glm-asr-2512` | No |
| `gemini` | none | Gemini native `generateContent` | `gemini-2.5-flash` | `providers.gemini.api_key` | `https://generativelanguage.googleapis.com/v1` | No | No |
| `anthropic` | `claude` | Anthropic native Messages | `claude-sonnet-4-6` | `providers.anthropic.api_key` | `https://api.anthropic.com` | No | No |
| `grok` | `xai` | xAI OpenAI-compatible chat completions | `grok-4.3` | `providers.grok.api_key` | `https://api.x.ai/v1` | Yes: `xai-stt` | No |

All upstream provider credentials are server-side only. Client requests must
never send OpenAI, Meta, Anthropic, xAI, Gemini, or other upstream API keys.

### Model catalog schema

Model ids and per-model metadata are runtime config data. To add, remove, or
replace provider models, update the selected `config.yml` and restart the
service; provider transports stay code-owned.

The model-capability table below mirrors the checked-in catalog. Refresh that
table and `config.yml` together; provider transports remain code-owned.
Moonshot's current Kimi route receives Chat Completions
`max_completion_tokens` when callers set the proxy `max_tokens` value.
The transport deliberately omits sampling controls because Kimi K3 fixes those
values upstream.
GLM-5.2 uses the existing BigModel/Zhipu Chat Completions endpoint with a
configured 131072-token output cap; its optional `thinking` and
provider-native `reasoning_effort` controls are not exposed directly. The
proxy's provider-neutral request-level `reasoning_effort` is accepted only when
the exact resolved route declares a capability mapping. Blank values and
supplied values for GLM or generic compatible-provider routes fail before an
upstream call; an omitted field retains the supported tenant default.
Qwen Cloud Token Plan is separate from DashScope: select `qwencloud` with a
dedicated `${QWEN_CLOUD_TOKEN_PLAN_API_KEY}` and its token-plan base URL; the
existing `qwen` alias remains DashScope-only. MiniMax M2.7 uses
`max_completion_tokens`, and the proxy rejects `max_tokens` values above the
documented 2048-token completion maximum before it calls MiniMax.

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
          reasoning_effort:
            adapter: "openai_responses"
            efforts: ["minimal", "low", "medium", "high"]
```

`server.request_timeout_seconds` and `server.max_request_timeout_seconds` must
both be positive, and the default must not exceed the maximum. Invalid explicit
values fail startup, including YAML `null` and an explicitly empty YAML value;
omitting either field selects its compiled default. The maximum is
operator-owned service capacity and must remain strictly below the
response-header and read deadlines of the outer gateway.

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

`reasoning_effort` is an optional text-model capability declaration. It appears
only under `providers.<provider>.text.models[]` and belongs to that exact
provider/model route; provider-level and catalog-wide declarations are rejected.
Each declaration uses the `openai_responses` adapter and a nonempty,
duplicate-free ordered list of values that adapter supports, and only an OpenAI
`openai_responses_reasoning_tools` route may declare it. The capability limits
the tenant default that can be persisted for that route and validates any
supplied public request value.

`request_profile` is currently required only for OpenAI text models. It selects
the stable proxy payload shape for that OpenAI model and must be one of:

| Request profile | Payload behavior |
|-----------------|------------------|
| `openai_responses_temperature` | Adds `temperature`. |
| `openai_responses_temperature_tools` | Adds `temperature`; includes web-search tools only when both the request and model catalog enable web search. |
| `openai_responses_reasoning_tools` | Adds reasoning/text controls; includes web-search tools only when both the request and model catalog enable web search. The resolved request-level reasoning effort is sent only when this route declares the capability. |

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

Provider-specific details:

* OpenAI is the only provider currently exposed with `web_search` support, and
  only for OpenAI model catalog entries with `web_search: true`. OpenAI
  dictation uses the same `providers.openai.api_key` value. OpenAI Responses
  and Models endpoint URLs are derived from `providers.openai.base_url`;
  dictation uses `providers.openai.transcriptions_url`.
* OpenAI-compatible text providers send chat completion requests with
  `Authorization: Bearer <api_key>` and the selected provider base URL. The
  shared adapter accepts text only with `finish_reason=stop`; `length`,
  `content_filter`, `tool_calls`, missing, and provider-specific non-stop
  reasons are upstream failures.
* Qwen Cloud Token Plan uses selector `qwencloud`, exact model
  `qwen3.8-max-preview`, `${QWEN_CLOUD_TOKEN_PLAN_API_KEY}`, and
  `https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1`.
  It is deliberately separate from DashScope because their API keys and base
  URLs are not interchangeable. The proxy exposes text generation only and
  sends the public `max_tokens` value through the compatible Chat Completions
  field without adding Qwen-specific thinking, tool, or multimodal controls.
* MiniMax uses selector `minimax`, exact model `MiniMax-M2.7`,
  `${MINIMAX_API_KEY}`, and `https://api.minimax.io/v1`. The shared compatible
  Chat Completions adapter maps public `max_tokens` to upstream
  `max_completion_tokens`; the catalog enforces MiniMax's documented 2048-token
  completion maximum. The proxy does not expose MiniMax-specific reasoning,
  tool, streaming, or multimodal controls.
* Meta Model API requests use that shared Chat Completions adapter with the
  exact `meta` selector, `https://api.meta.ai/v1` base URL,
  `${MODEL_API_KEY}` credential, and `muse-spark-1.1` model. llm-proxy exposes
  the public `max_tokens` input upstream as Meta's current
  `max_completion_tokens` field rather than Meta's deprecated `max_tokens` field.
  The proxy exposes Muse Spark 1.1 only as text generation through `GET /`,
  `POST /`, and `POST /v2`: there is no Meta dictation or `web_search`, no
  proxy tool or multimodal input contract, and no fallback to Meta's Responses API. Meta
  documents Muse Spark 1.1 as a public preview for U.S. developers with a
  1,048,576-token context window. See Meta's
  [Muse Spark guide](https://developer.meta.com/ai/resources/blog/build-with-muse-spark/),
  [model reference](https://dev.meta.ai/docs/getting-started/models),
  [Chat Completions reference](https://dev.meta.ai/docs/features/chat-completion),
  and [pricing and rate-limit documentation](https://dev.meta.ai/docs/getting-started/pricing-rate-limits).
* Only dictation-capable providers expose `transcriptions_url` fields:
  OpenAI uses `providers.openai.transcriptions_url`, SiliconFlow uses
  `providers.siliconflow.transcriptions_url`, Zhipu uses
  `providers.zhipu.transcriptions_url`, and Grok/xAI uses
  `providers.grok.transcriptions_url`.
* Gemini text requests use the native `generateContent` route and normalize
  Gemini usage metadata into the same response headers and JSON `usage` object
  used by the other text providers. Only `finishReason=STOP` can return text.
* Anthropic text requests use `POST /v1/messages` with `x-api-key` and
  `anthropic-version: 2023-06-01`. System messages are translated to
  Anthropic's top-level `system` field. Anthropic requires `max_tokens`, so
  when the client omits it the proxy sends the selected Claude model's
  configured output limit. Only `stop_reason=end_turn` or `stop_sequence` can
  return text; truncation, tool use, paused turns, refusals, and unknown reasons
  are upstream failures because this adapter exposes no continuation or tool
  loop.
* Zhipu dictation uses Z.AI GLM-ASR through
  `providers.zhipu.transcriptions_url` with the selected configured dictation
  model.
* Grok text requests use xAI's OpenAI-compatible `/chat/completions` API at
  `https://api.x.ai/v1`. Grok/xAI dictation uses xAI STT through
  `providers.grok.transcriptions_url`; the upstream STT endpoint does not
  receive a `model` multipart field.

When management is disabled, provider API keys are optional until a configured
static tenant uses that provider as a default. If a non-default provider key is
blank or its whole `api_key` value is a missing `${...}` placeholder, startup
continues and explicit requests for that provider return `503 provider not
configured`. Missing placeholders in other fields, or embedded inside a longer
`api_key` value, fail startup. If a static tenant's default text or dictation
provider lacks its API key, startup fails before the server listens. Provider
`base_url` values are explicit config values; leave them at
the documented URLs unless routing through a test server, proxy, or compatible
gateway. Dictation-capable provider
`transcriptions_url` values are also explicit config values and are required for
OpenAI, SiliconFlow, Zhipu, and Grok/xAI. Text model catalogs are required for
every supported provider, and dictation model catalogs are required for OpenAI,
SiliconFlow, Zhipu, and Grok/xAI. When `management.enabled` is false, startup
validates that `tenants` includes at least one unique `id` and unique `secret`.
When management is enabled, `tenants` and nonblank provider `api_key` fields are
invalid: all client tokens and provider credentials are user-owned database
state.
Unknown YAML keys fail startup.

### Self-service management UI

Set `management.enabled: true` to enable TAuth-protected management APIs under
`/api/management`. The browser UI is static and lives in `site/`, which is
packaged by `make release`, uploaded as an immutable GitHub Release asset by
`make publish`, and activated on `gh-pages` by `make deploy`. GitHub Actions is not used for Pages deployment. The backend does
not serve management HTML or assets; `GET /` remains a proxy endpoint and
returns `403` without a tenant `key`. The backend does serve public
`/config-ui.yaml` from the loaded management config so the GitHub Pages frontend
can consume the current llm-proxy runtime, MPR UI, and TAuth bootstrap values
from `llm-proxy-api`.

The static UI uses the shared MPR shell through API-served `config-ui.yaml`,
literal `mpr-ui@latest` assets, `mpr-ui-config.js`,
`<mpr-header data-config-url="...">`, the `@latest` bundle marker, `<mpr-user>`,
and `<mpr-footer>`. It does not load `tauth.js` directly or apply MPR UI config
from application JavaScript. The Pages artifact contains no static
`config-ui.yaml` or `llm-proxy-config.json`; release rendering writes the
profile-owned `PAGES_CONFIG_URL` into the declarative header attribute. That
single API-served YAML points browser management API calls, generated usage
examples, and MPR UI/TAuth at the configured origins. Browser-facing values are
projected from the already-loaded backend `config.yml`; there is no second
environment expansion path for Pages.

Release rendering also copies the exact committed `docs/openapi.yaml` bytes to
the Pages artifact root and verifies the SHA-256 provenance embedded in the
derived `site/docs/index.html`. `site/openapi.yaml` is intentionally forbidden:
there is no independently editable or generated schema copy in site source.

The consumed shared bundle registers `mpr-legal-document`; legal-page routes
and document rendering remain owned by P005 and are not duplicated here.

MPR UI is the sole browser authentication authority. LLM Proxy registers the
documented `mpr-ui:auth:authenticated` and `mpr-ui:auth:unauthenticated`
lifecycle listeners, uses the header's documented `data-mpr-auth-status` only
to reconcile the current state after startup, and does not request
`/api/management/account` until MPR UI reports `authenticated`. LLM Proxy does
not inspect TAuth cookies, storage, tokens, or claims and does not call TAuth
authentication endpoints. After MPR UI reports authentication, a management
API failure renders an explicit workspace error; it does not reinterpret the
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
| `management.database_path` | Required SQLite database location for tenant-owned provider keys, defaults, generated-secret digests, and usage events. Persistence uses the pure-Go GORM SQLite driver so `CGO_ENABLED=0` builds remain valid. |
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
that client key and at least one persisted managed provider key. Only
`tenant.has_secret` and `providers[].has_key` satisfy this setup gate; a typed
provider-key draft or a credential in local dotenv configuration does not.
The selected provider's API key, default model, and system prompt autosave when
the user leaves a changed field, switches providers, or closes Settings. A
successful first provider-key autosave unlocks Settings and atomically
establishes eligible routing defaults. Settings remains open until the user
closes it explicitly.
Text and dictation provider/model defaults plus reasoning effort autosave on
selection, while the tenant system prompt autosaves when the user leaves the
changed field. Settings serializes every mutation that returns a complete
management profile, including provider and routing-default autosaves, provider
removal, and client-key creation or replacement. A close request
locks the controls and waits for the mutations already in progress. If a client
key is created or replaced during that wait, Settings stays open so the one-time
value can be copied before a second explicit close. A failed save retains the
edited values for retry. Removing the last managed provider key makes Settings
mandatory again, while a failed automatic client-key request remains retryable
through Create key.

Signed-in users also choose each provider's text model and provider-specific
system prompt, choose routing defaults, and replace llm-proxy client keys after
confirming that the prior value stops working immediately. A client key cannot
be deleted independently; access is rotated through replacement or removed
with the owning non-final tenant. Management mode requires
`management.database_path` so signups, enabled
providers, defaults, generated secret digests, and usage events survive restarts
in a GORM-managed SQLite database at the configured location. The packaged management config uses
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
through authenticated management endpoints and are encrypted at rest with AES-GCM
before database persistence. Normal save, profile, and administrator responses
return only masked key status. The sole raw-key response is the explicit
owner-authenticated
`POST /api/management/tenants/:tenant_id/provider-keys/:provider/reveal`
management action, which requires the configured management origin and returns
`Cache-Control: no-store`. Provider-key records also store the selected text model and
provider-specific system prompt for that provider. Managed text requests that
select a provider and omit `model` use the saved provider text model; when
request-level system instructions are omitted, the provider-specific system
prompt is injected before routing upstream. The F014 ownership migration accepts
only already-encrypted legacy provider-key rows, decrypts them with their prior
user binding, and re-encrypts them with the preserved opaque workspace id as
AES-GCM associated data. Plaintext, corrupt, orphaned, or non-canonical rows
fail startup before the migration transaction begins. The backend decrypts
provider keys only inside the runtime
path that routes requests to upstream providers and the explicit owner reveal action,
so this protects database dumps, backups, and direct storage access; it is not a user-only decryption or
zero-knowledge guarantee. Generated tenant secrets are returned once and the
database retains only their SHA-256 digest. Replacing a generated secret
immediately makes future public proxy requests with the prior value return
`403`. Deleting a non-final tenant removes its secret digest with the rest of
the tenant-owned state.

Managed routing defaults contain complete canonical provider/model pairs plus a
route-bound `reasoning_effort`. A provider is eligible only while that tenant
has a saved API key for it. The text pair is both empty only when no provider
key is saved. The dictation pair is both empty when none of the keyed providers
supports dictation; in that state the Settings controls are disabled and no
default dictation example is shown. Saving or removing a provider key preserves
an eligible current default and otherwise selects the first eligible provider
by canonical provider id, using that provider's saved text model or configured
dictation default model. The key mutation and both reconciled routing pairs are
one database transaction, so a profile never exposes a default whose key was
removed.

`PUT /api/management/tenants/:tenant_id/defaults` accepts only these eligible
complete pairs and resolves the supplied text pair before validating the
effort. Empty is the explicit unset effort value; a nonempty value must be in
that exact route's declared list. A partial pair, unkeyed or unknown provider,
unsupported dictation provider, cross-provider model, or incompatible effort
returns `400 managed_routing_defaults_invalid` before any default is persisted.

The profile exposes key eligibility through `providers[].has_key` and
capability data only as
`providers[].text_models[].reasoning_effort`; it has no global option list or
provider-level reasoning capability. The Settings routing selectors contain
only keyed providers; dictation additionally requires declared dictation
support. The form keeps Text provider, Text model, and Reasoning effort in one
desktop row, clears an incompatible saved value on a model change, reports `Not
supported` for routes without a declaration, and autosaves every
routing-default change without a separate action. The browser rejects malformed
profile data instead of repairing it. Public
`GET /` accepts optional query `reasoning_effort`; JSON `POST /` and `POST /v2`
accept the same optional field in their bodies. When omitted, the saved tenant
default remains authoritative. An explicit value must be nonblank and exactly
supported by the resolved provider/model route, otherwise the proxy returns
`400` before an upstream call.

Management startup requires every persisted routing field to be canonical and
catalog-valid and every nonempty provider default to have the tenant's saved
key. It never infers or repairs a provider, model, or reasoning effort at read
time. The bounded schema-version-3 migration performs the one-time reconciliation
of older managed defaults against saved provider keys, preserves tenant
timestamps, verifies the result, and records the new version in one
transaction. Invalid keys, models, or routing data stop startup with the owner,
workspace, endpoint, provider, and model context.

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

Usage events are recorded only for managed workspaces when they call the public
proxy endpoints with a generated secret. Account-wide usage queries apply the
authenticated owner and all owned tenant ids at the database boundary;
tenant-scoped queries additionally require the explicit tenant id. Every query
uses one captured time boundary. Stored usage
metadata includes endpoint, provider, model, status code, success flag, one
canonical outcome code, latency, and normalized request/response/total token
counts. Outcome codes are exactly `success`, `invalid_request`,
`payload_too_large`, `rate_limited`, `service_unavailable`, `request_timeout`,
or `upstream_error`. They are selected at the request/error boundary; prompts,
audio, transcripts, responses, tenant secrets, provider API keys, raw upstream
bodies, and free-form error text are not stored in usage events.

Management mode no longer imports config tenants or global provider keys.
TAuth subjects own personal workspaces directly; there is no shared-workspace,
membership, role, invitation, or team-tenancy contract.

F014 upgrades the previous one-workspace-per-user database to schema version 1
as one bounded startup transaction:

1. Drain every old llm-proxy instance and take an operator-owned database
   backup. Never run the old and new binaries against the same database during
   this migration.
2. Exercise the exact SQLite migration on a disposable database through the
   repository `make ci` gate.
3. Start one new instance. Preflight reads all legacy tenant, provider-key, and
   usage rows before opening the mutation transaction. It rejects missing
   tables, unclaimed `static-config:` owners, blank or duplicate owners and
   workspace ids, duplicate or malformed secret digests, orphan provider or
   usage rows, plaintext or corrupt provider keys, and non-canonical routing
   data.
4. The transaction renames the two colliding legacy GORM indexes and the three
   legacy tables, creates explicit user and workspace tables, preserves every
   opaque tenant id, moves secret digests and routing data, rebinds encrypted
   provider keys from the prior user id to the workspace id, copies usage rows,
   verifies counts and values including decryption, writes schema version 1,
   and removes the bounded legacy tables.
5. Verify account, workspace, provider, secret, routing, and usage behavior
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
| `tauth-api.mprlab.com` | TAuth backend | Google login, nonce, logout, `/auth/session`, and session-cookie issuance. |

Add these DNS records:

1. `CNAME llm-proxy.mprlab.com -> tyemirov.github.io`
2. Point `llm-proxy-api.mprlab.com` at the MPR gateway public endpoint. Use a
   `CNAME` when the gateway has a hostname, or `A`/`AAAA` records when it is
   addressed by public IP.

Then configure GitHub Pages for this repository:

1. Use branch publishing from `gh-pages` at `/`.
2. Set the Pages custom domain to `llm-proxy.mprlab.com`.
3. Run `make release` to render and validate the Pages archive, `make publish`
   to upload that immutable archive, and `make deploy` to activate it on
   `gh-pages`. Deployment configures the repository Pages source and verifies
   the matching GitHub Pages build before fetching a cache-distinct
   `/.mprlab-release.json` marker at the public origin.
4. Configure real backend deployment secrets outside the Pages artifact:
   `LLM_PROXY_MANAGEMENT_ADMIN_EMAILS`, `LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY`,
   `LLM_PROXY_MANAGEMENT_DATABASE_PATH`,
   and `LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY`.
5. Do not store browser runtime config in the Pages branch. Production browser
   config is served only by `https://llm-proxy-api.mprlab.com/config-ui.yaml`
   from the running backend's loaded management config. `make release` writes
   that URL into `mpr-header[data-config-url]` through `PAGES_CONFIG_URL` and
   validates the declarative mpr-ui bundle marker.

Configure TAuth for tenant `llm-proxy` with:

- allowed tenant origin `https://llm-proxy.mprlab.com`
- browser-facing API origin `https://tauth-api.mprlab.com`
- session cookie name matching `management.session_cookie_name`
- cookie domain `.mprlab.com`
- HTTPS-only cookies
- JWT signing key matching `management.jwt_signing_key`

The gateway `llm-proxy` deployment profile treats this as one runtime contract:
it stages the TAuth and llm-proxy env/config inputs, restarts both `tauth-api`
and `llm-proxy`, and verifies both public health checks before Pages activation.
This prevents a newly deployed backend from validating sessions against stale
TAuth cookie or signing configuration.

The same boundary is executable locally without Google OAuth or deployed
services:

```bash
make test-management-auth-blackbox
```

The target builds the TAuth version pinned in `go.mod` and the current
llm-proxy binary, starts both on disposable local ports, and opens the real
static management app in Playwright. The page signs in through TAuth's seeded
password-login endpoint with a credentialed cross-origin browser request, so
the test enforces TAuth login CORS and receives the configured HttpOnly access
and refresh cookies. It then drives the mounted header through the documented
`MPRUI.testing.authenticate` adapter, which emits the normal authenticated
lifecycle event and persists MPR UI's session-restore hint. The test proves the
anonymous/authorized behavior of `/api/management/account`, proves the browser
makes no protected account or tenant request before MPR UI authentication, then
hydrates the initial tenant selected in Settings and account-wide Usage view afterward. It
creates two tenants for one real TAuth subject, proves both secrets remain
independently routable, proves the default account-wide usage and safe
tenant-attributed failure page include both, and signs in a second real subject
to prove foreign tenant ids return `404` without disclosure. It waits for the
`mpr-ui@latest` shell plus the
dashboard to report the authenticated state, then proves an ordinary reload
stays authenticated, removes only the access cookie and proves `/auth/session`
recovers it from the refresh cookie without rendering the signed-out panel, and
uses the visible **Sign out** action to prove `/auth/logout` clears both cookies
and returns TAuth plus the management API to anonymous responses. The MPR UI
application assets are loaded through their literal `@latest` CDN contract;
TAuth and management API routes are never mocked.

Normal navigation, page refreshes, and access-cookie expiration do not sign the
user out. The `mpr-ui@latest` shell silently restores the TAuth session while its
rotating refresh cookie remains valid. Only the explicit **Sign out** action
calls TAuth logout and clears the browser session; LLM Proxy does not own a
second session store or an automatic logout path.

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
known. MiniMax M2.7 rejects `max_tokens` above `2048`; Gemini text models
currently reject values above `65536`; Claude models reject values above the
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

Before the first run, explicitly create the ignored private
`configs/.env.local`, populate it with real local values, and set mode `0600`.
The tracked `configs/.env.local.example` and `configs/.env.sample` files are
field-name documentation with deliberately unrealistic values; never copy or
source them as runtime configuration. `make up` fails before contacting Docker
when the private file is absent. When the real file uses the explicit
`__GENERATE_ON_FIRST_MAKE_UP__` marker for its local TAuth signing key or
provider-key encryption key, `make up` generates that value once. It then
writes ignored, service-scoped environment projections for ghttp, llm-proxy,
and TAuth.

ghttp receives only its `GHTTP_*` inputs. TAuth receives only its server and
tenant inputs, including the signing key it shares with the API. Only llm-proxy
receives the provider-key encryption configuration; aggregate dotenv files and
live provider smoke-test credentials are not injected into auxiliary
containers. The API image is built from the current source and runs the
canonical `configs/config.yml` configuration. The stack has two explicit
browser-facing endpoints:

- Static UI: `http://localhost:4179/`, served from `site/` by ghttp.
- Backend API: `http://localhost:8080/`, including the proxy and
  `/api/management/*` endpoints.

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

Compose first completes image pulls/builds and reports all three services
running through `docker compose up --wait`; only then does the bounded HTTP
readiness budget begin. Readiness proves static content (`200`), the
ghttp-served runtime config (`200`), the unauthenticated API boundary (`403`),
the same-origin TAuth session (`204`) and nonce (`200`) boundaries, and the
unauthenticated management API boundary (`401`). It does not call a paid
provider. After readiness, Compose logs remain attached in the foreground. Use
`Ctrl-C` to stop the containers and network; the named local data volumes keep
local TAuth and management state for the next run.

Browser startup additionally loads the pinned Alpine 3.13.5 module from
`https://cdn.jsdelivr.net`. `make up` cannot override a Chrome extension,
privacy filter, or browser policy that blocks that client-side request. If the
page reports **Unable to open LLM Proxy**, allow `cdn.jsdelivr.net` for
`http://localhost:4179` in the blocking browser control and select **Reload LLM
Proxy**. The same failure screen replaces an incoherent or rejected first-party
module graph. LLM Proxy does not try another CDN or a bundled fallback; the
failure screen completes the shared MPR transition without making a protected
management request.

With `management.enabled: false`, set a static tenant's default text
provider/model to route omitted-provider requests to DeepSeek. Static tenant
blocks are invalid in management mode, where every token is owned by an
authenticated user:

```yaml
tenants:
  - id: deepseek
    secret: "${SERVICE_SECRET}"
    defaults:
      provider: deepseek
      model: deepseek-v4-flash
```

For a static tenant, `reasoning_effort` is a route-bound default. Set it only
when the exact configured provider/model declares the value. It applies when a
caller omits the optional per-request field; a supplied supported value
overrides it. For example, a supported OpenAI route can use `high`:

```yaml
tenants:
  - id: openai-reasoning
    secret: "${SERVICE_SECRET}"
    defaults:
      provider: openai
      model: gpt-5
      reasoning_effort: high
```

The allowed values are the selected route's configured list; omit the field or
use an empty value to leave it explicitly unset. The proxy rejects an
incompatible static default at startup and forwards an effort only when the
resolved route declares that exact value.

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

Set Meta Muse Spark 1.1 as the default text provider:

```yaml
tenants:
  - id: meta
    secret: "${SERVICE_SECRET}"
    defaults:
      provider: meta
      model: muse-spark-1.1
```

## Local Automation

This repository exposes the standard local targets used by MPR app repos:

| Command | Purpose |
|---------|---------|
| `npm ci` | Install pinned frontend validation dependencies before running local frontend checks. |
| `make up` | Require the ignored private `configs/.env.local`, then build and run the complete local browser orchestration: ghttp static UI and same-origin TAuth routes on `localhost:4179`, plus the API on `localhost:8080`. It waits for Compose startup before verifying the static/config/auth/API boundaries and reporting ready. |
| `make ci` | Run format checks, Go lint (`go vet`, `staticcheck`, `ineffassign`), Python strict mypy, frontend syntax checks, the 100% coverage-gated Go test suite, Python pytest, Playwright browser tests, repository-owned release integration tests, and the non-paid live-harness preflight. |
| `make test-live-provider-harness` | Generate the temporary static-mode live-test config and verify authenticated routing without an upstream call. |
| `make test-live-providers` | Generate a complete temporary static-mode config and run live text smoke tests for every provider whose API key is present; use `LIVE_ENV_FILE=/path/to/env` to load interpolation values. |
| `make test-live-gemini` | Compatibility wrapper for `make test-live-providers` with `LLM_PROXY_LIVE_PROVIDERS=gemini`. |
| `make release` | Run CI and prepare the local tag, container archives, and validated Pages archive under `.git/mprlab-release` without remote writes. |
| `make publish` | Publish the exact prepared Git refs, GitHub Release assets, and container archives without rebuilding or deploying; wait for each GHCR manifest to become readable. |
| `make deploy` | Verify and deploy the published backend through the sibling gateway, then activate the Pages archive and verify the matching Pages build and public marker. |

Live provider smoke tests are intentionally not part of `make ci`; they call
paid upstream APIs and depend on local or CI secret availability. The dynamic
target discovers these provider keys after loading `LIVE_ENV_FILE`. By default,
smoke requests omit `model`, so each provider uses the default configured in
the selected `configs/config.yml`; set a model override only when debugging a
specific provider/model pair.

| Provider | Key variable | Model override |
|----------|--------------|----------------|
| OpenAI | `OPENAI_API_KEY` | `LLM_PROXY_LIVE_OPENAI_MODEL` |
| Meta Muse Spark | `MODEL_API_KEY` | `LLM_PROXY_LIVE_META_MODEL` |
| DeepSeek | `DEEPSEEK_API_KEY` | `LLM_PROXY_LIVE_DEEPSEEK_MODEL` |
| DashScope/Qwen | `DASHSCOPE_API_KEY` | `LLM_PROXY_LIVE_DASHSCOPE_MODEL` |
| Qwen Cloud Token Plan | `QWEN_CLOUD_TOKEN_PLAN_API_KEY` | `LLM_PROXY_LIVE_QWEN_CLOUD_MODEL` |
| Moonshot/Kimi | `MOONSHOT_API_KEY` | `LLM_PROXY_LIVE_MOONSHOT_MODEL` |
| MiniMax | `MINIMAX_API_KEY` | `LLM_PROXY_LIVE_MINIMAX_MODEL` |
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

The live harness parses `LIVE_ENV_FILE` as dotenv data without executing it as
shell code. It always writes a disposable static-mode config with management
disabled, a temporary tenant, and placeholder values for unused provider keys,
so live checks never open the configured management database or reuse its
migration state. Inspect that generated contract without building or calling a
paid provider with `./scripts/test_live_providers.sh --write-config
/tmp/llm-proxy-live.yml`. Unless `LLM_PROXY_LIVE_PORT` explicitly selects a
port, each harness run allocates a fresh loopback port. It removes only the
temporary proxy child it started and never terminates an unrelated local
listener.

`make release` and `make deploy` run the local `make ci` gate with the standard
350-second timeout. Override both with
`LLM_PROXY_CI_TIMEOUT_SECONDS=<seconds>`, or use the command-specific
`RELEASE_CI_TIMEOUT_SECONDS` and `DEPLOY_CI_TIMEOUT_SECONDS` variables.
`make publish` verifies and uploads only the already-prepared immutable
artifacts; it does not rebuild or rerun CI. GHCR manifest readiness is bounded
by `CONTAINER_REGISTRY_VERIFY_ATTEMPTS` (default `12`) and
`CONTAINER_REGISTRY_VERIFY_DELAY_SECONDS` (default `5`), with every Docker
inspection bounded by `CONTAINER_REGISTRY_VERIFY_ATTEMPT_TIMEOUT_SECONDS`
(default `30`). Pages build readiness is bounded by
`PAGES_BUILD_VERIFY_ATTEMPTS` (default `36`) and
`PAGES_BUILD_VERIFY_DELAY_SECONDS` (default `5`); the final public marker check
uses `PAGES_VERIFY_ATTEMPTS` and `PAGES_VERIFY_DELAY_SECONDS` (defaults `12`
and `5`). Each wait reports its observed external readiness boundary rather
than treating a completed push as immediate public availability.

`llm-proxy` is a gateway-local service in `mprlab-gateway`, so `make deploy`
uses the sole gateway `deploy-llm-proxy-backend` target after the gateway-owned
`verify-llm-proxy-deployment-contract` preflight proves the coupled TAuth
service, runtime assets, and health checks. The gateway discovers this
canonical checkout and reads `server.max_request_timeout_seconds` through the
committed app-owned capacity reader; the app deploy script passes no capacity
flag or environment variable. Deployment fails unless the gateway-owned
response-header, upstream-read, and client-write guards are all strictly
greater. Neither repository carries a fallback copy of the other's capacity.
Override only the checkout with
`GATEWAY_DIR=/path/to/mprlab-gateway`; the selected gateway checkout must be a
clean, synchronized `origin/master`. Override Pages preparation and activation
with `PAGES_DOMAIN=<domain>`, `PAGES_CONFIG_URL=<https-config-url>`,
`PAGES_BRANCH=<branch>`, `PAGES_URL=<url>`, or
`DEPLOY_PAGES_ARGS="--skip-configure"`.

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

Public proxy calls authenticate with the tenant secret in `key=...`. The
optional MPR UI/TAuth session instead authorizes management actions such as
creating a client key or saving a provider key; it does not authenticate a
direct `POST /v2` request. Upstream provider API keys stay in server-side
configuration or authenticated management storage and must never be sent by a
client.

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
  --secret "$SERVICE_SECRET" \
  --prompt "Summarize this"
```

Or read configuration and prompt text from environment/stdin:

```shell
export LLM_PROXY_BASE_URL="http://localhost:8080/"
export LLM_PROXY_SECRET="$SERVICE_SECRET"
printf 'large prompt...\n' | llm-proxy-client --max-tokens 4096
```

For a route whose catalog declares the value, override the tenant default for
one request with `--reasoning-effort`:

```shell
llm-proxy-client \
  --base-url "http://localhost:8080/?provider=openai" \
  --secret "$SERVICE_SECRET" \
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

The request value controls only the proxy budget header. `ctx` remains the Go
caller's independent cancellation authority, and the injected `HTTPDoer` may
have its own explicitly selected transport policy. The package does not add a
total-response timeout.

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

Changing `providers.<provider>.text.default_model` in the service config affects
only requests that resolve through that provider catalog default. It does not
rewrite a managed tenant's saved routing default or a saved provider text model.

#### Application-user model profiles

For application-owned, per-user selection, configure one client instance with
that user's JSON model-profile path. The document contains exactly these two
nonblank string fields and never contains credentials or TAuth material:

```json
{
  "provider": "gemini",
  "model": "gemini-2.5-flash"
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
  --secret "$SERVICE_SECRET" \
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
        max_tokens=512,
        request_timeout_seconds=900,
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

Meta Muse Spark 1.1 text generation:

```shell
curl --get \
  --data-urlencode "prompt=Summarize this with Muse Spark" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "provider=meta" \
  --data-urlencode "model=muse-spark-1.1" \
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
MiniMax M2.7 `max_tokens` values above `2048`, Gemini values above `65536`, and
Anthropic values above the configured Claude model output limit return `400 Bad
Request` before the proxy calls the selected provider.

`POST /v2` is the canonical chat endpoint. Its exact accepted fields come from
the OpenAPI schema, including the omission-versus-explicit-value contract for
`reasoning_effort`. It rejects `prompt` and body `system_prompt`; send a
`system` role message instead. The tenant default system prompt is still
prepended when the submitted messages do not include a system message.

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

## Canonical endpoint reference

Use the derived [API reference](https://llm-proxy.mprlab.com/docs/) for the
complete operation inventory and the exact query, header, JSON, multipart,
authentication, media-type, response-header, and status contracts. Use the
[committed OpenAPI artifact](docs/openapi.yaml) for tooling and review.

The request examples in [Usage](#usage) demonstrate common calls without
duplicating that inventory. In particular, dictation has one canonical incoming
multipart file part, `audio`; the obsolete `file` alias is rejected.

## Model catalog

The default model catalog in [configs/config.yml](configs/config.yml)
declares the LLM endpoint models below. The `/dictate` endpoint defaults to
OpenAI's audio transcriptions API and also supports SiliconFlow, Zhipu, and
Grok/xAI through their provider selectors. Not all configured models support
tools; use a model marked `Yes` below for web search. A dash in the proxy
`max_tokens` limit column means the proxy validates only that `max_tokens` is
positive and lets the upstream provider enforce any provider-side model limit.

### OpenAI reasoning-effort capabilities

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
| `deepseek-v4-flash` | DeepSeek | Yes | - | No |
| `deepseek-v4-pro` | DeepSeek | No | - | No |
| `deepseek-chat` | DeepSeek | No | - | No |
| `deepseek-reasoner` | DeepSeek | No | - | No |
| `qwen-plus` | DashScope/Qwen | Yes | - | No |
| `qwen3.8-max-preview` | Qwen Cloud Token Plan | Yes | - | No |
| `kimi-k2-0905-preview` | Moonshot/Kimi | Yes | - | No |
| `kimi-k3` | Moonshot/Kimi | No | - | No |
| `kimi-k2.7-code` | Moonshot/Kimi | No | - | No |
| `kimi-k2.7-code-highspeed` | Moonshot/Kimi | No | - | No |
| `kimi-k2.6` | Moonshot/Kimi | No | - | No |
| `MiniMax-M2.7` | MiniMax | Yes | `2048` | No |
| `deepseek-ai/DeepSeek-R1` | SiliconFlow | Yes | - | No |
| `glm-5.1` | Zhipu/GLM | Yes | - | No |
| `glm-5.2` | Zhipu/GLM | No | `131072` | No |
| `gemini-3.5-flash` | Gemini | No | `65536` | No |
| `gemini-3.1-pro-preview` | Gemini | No | `65536` | No |
| `gemini-3-flash-preview` | Gemini | No | `65536` | No |
| `gemini-3.1-flash-lite` | Gemini | No | `65536` | No |
| `gemini-2.5-flash` | Gemini | Yes | `65536` | No |
| `gemini-2.5-flash-lite` | Gemini | No | `65536` | No |
| `gemini-2.5-pro` | Gemini | No | `65536` | No |
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

### Dictation capabilities

| Provider selector | Models | Credential field | Transcription URL field | Notes |
|-------------------|--------|------------------|-------------------------|-------|
| `openai` | `gpt-4o-mini-transcribe`, `gpt-4o-transcribe` | `providers.openai.api_key` | `providers.openai.transcriptions_url` | Default dictation provider and default model `gpt-4o-mini-transcribe`. |
| `siliconflow` | `FunAudioLLM/SenseVoiceSmall` | `providers.siliconflow.api_key` | `providers.siliconflow.transcriptions_url` | OpenAI-compatible audio transcription. |
| `zhipu` / `glm` | `glm-asr-2512` | `providers.zhipu.api_key` | `providers.zhipu.transcriptions_url` | Z.AI GLM-ASR; sends `model=glm-asr-2512`. |
| `grok` / `xai` | `xai-stt` | `providers.grok.api_key` | `providers.grok.transcriptions_url` | xAI STT; the proxy model name selects the provider but is not sent as a multipart `model` field. |

### Status codes

* `200 OK` - success
* `400 Bad Request` - missing/invalid parameters, invalid request timeout, invalid multipart audio form, unknown provider/model, or unsupported provider capability. Invalid timeout headers return `{"error":{"code":"invalid_request_timeout","max_request_timeout_seconds":M}}`.
* `403 Forbidden` - missing or invalid `key`
* `413 Payload Too Large` - JSON prompt body exceeds `max_prompt_bytes`, or
  dictation audio exceeds `max_input_audio_bytes`
* `429 Too Many Requests` - upstream provider rate limit
* `503 Service Unavailable` - selected provider credential is unavailable because that non-default provider is disabled or missing its API key
* `504 Gateway Timeout` - the accepted proxy work budget expired; the response is `{"error":{"code":"request_timeout","request_timeout_seconds":N}}`
* `502 Bad Gateway` - upstream provider API returned an error

## Security

* All requests must include a configured tenant secret via `key=...`.
* Client requests must not include upstream provider API keys; public proxy endpoints reject provider-key-like query, JSON, and multipart form fields.
* Request logs record only the query-free path plus method, status, latency, client IP, and tenant metadata; they do not record query strings, request bodies, cookies, or authorization headers.
* Self-service provider API keys are accepted only through TAuth-protected management endpoints. Autosave responses return masked status; raw retrieval requires the explicit owner-authenticated reveal action.
* Public static pages load Google Analytics and LoopAware page-view scripts. This repository makes no claim about their collection, retention, consent, or opt-out behavior; the public Privacy page must use approved legal and provider content. Do not put tenant secrets or other sensitive values in public-page URLs.
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

Use `make release` from a clean local `master` branch. It runs `make ci`, builds
the multi-platform container archives and static Pages archive under
`.git/mprlab-release`, updates `CHANGELOG.md`, and creates only a local release
commit and tag. The release implementation is repository-owned under
`tools/gitrelease`, uses the single canonical `vMAJOR.MINOR.PATCH` SemVer
contract, and builds containers from `git archive HEAD` so ignored credentials,
`.git`, and local artifacts never enter BuildKit. It performs no remote writes.

Use `make publish` to push the prepared Git refs, GitHub Release assets, and
container archives without rebuilding. It uses the standard Docker client to
wait until the exact published manifests are readable. Use `make deploy` only
after publish; it verifies the published image, deploys the backend through the
gateway, then activates the exact `pages.tar.gz` asset on the live Pages branch,
waits for the matching GitHub Pages build, and verifies its cache-distinct
public marker.

## License

This project is licensed under the MIT License. See [LICENSE](MIT-LICENSE) for
details.
