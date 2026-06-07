# Provider Routing Implementation Plan

Issue: `.mprl/ISSUES.md` -> `B001`

## Goal

Extend `llm-proxy` from an OpenAI-only proxy into an explicit multi-provider proxy while preserving current OpenAI defaults for existing clients.

## Request Contract

- `provider` is an optional query parameter on `GET /`, `POST /`, and `POST /dictate`.
- Omitted `provider` means the authenticated tenant's default provider.
- `model` keeps its current meaning; omitted `model` means the authenticated tenant's default model when set, otherwise the selected provider's native default.
- `max_tokens` is an optional positive integer on `GET /` query strings and JSON `POST /` bodies.
- Omitted `max_tokens` means the proxy omits provider max-token fields and lets the selected provider/model default apply.
- Provided `max_tokens` maps to OpenAI Responses `max_output_tokens`, OpenAI-compatible chat completions `max_tokens`, and Gemini `generationConfig.maxOutputTokens`.
- Known provider-specific output-token ceilings are validated before upstream calls; Gemini text models reject `max_tokens` above `65536` with `400 Bad Request`.
- For JSON `POST /`, query `model` may override the body only when the body omits `model` or provides the same value.
- Conflicting query/body `model` values return `400 Bad Request`.
- Upstream provider API keys are never accepted from client requests.

## Providers

| Provider | Aliases | Text | Dictation | Web Search |
|----------|---------|------|-----------|------------|
| `openai` | none | OpenAI Responses API | OpenAI audio transcription | Supported by existing OpenAI model table |
| `deepseek` | none | OpenAI-compatible chat completions | Not supported | Not supported |
| `dashscope` | `qwen` | OpenAI-compatible chat completions | Not supported | Not supported |
| `moonshot` | `kimi` | OpenAI-compatible chat completions | Not supported | Not supported |
| `siliconflow` | none | OpenAI-compatible chat completions | OpenAI-compatible audio transcription | Not supported |
| `zhipu` | `glm` | OpenAI-compatible chat completions | Not supported | Not supported |
| `gemini` | none | Native Gemini generateContent | Not supported | Not supported |

## Configuration

Runtime service configuration comes from `config.yml`; env vars and `.env`
files are interpolation inputs only for `${NAME}` placeholders in that YAML.
The loader rejects unknown keys and missing placeholders before the proxy starts.

Shared config fields:

- `server.port`
- `server.log_level`
- `server.workers`
- `server.queue_size`
- `server.request_timeout_seconds`
- `server.upstream_poll_timeout_seconds`
- `server.max_prompt_bytes`
- `server.max_input_audio_bytes`
- `tenants[].id`
- `tenants[].secret`
- `tenants[].defaults.provider`
- `tenants[].defaults.model`
- `tenants[].defaults.dictation_provider`
- `tenants[].defaults.dictation_model`
- `tenants[].defaults.system_prompt`

Provider credentials and base URLs:

- `providers.openai.api_key`
- `providers.deepseek.api_key`, `providers.deepseek.base_url`
- `providers.dashscope.api_key`, `providers.dashscope.base_url`
- `providers.moonshot.api_key`, `providers.moonshot.base_url`
- `providers.siliconflow.api_key`, `providers.siliconflow.base_url`, `providers.siliconflow.transcriptions_url`
- `providers.zhipu.api_key`, `providers.zhipu.base_url`
- `providers.gemini.api_key`, `providers.gemini.base_url`

Startup validates configured tenants, rejects duplicate tenant ids and duplicate secrets, validates the credential required by each tenant's default text provider, and validates endpoint/credential support for each tenant's default dictation provider. Credentials for providers not used by tenant defaults are validated when a request selects that provider.

## Error Contract

- `400`: unknown provider, unknown model, unsupported capability, unsupported endpoint, conflicting model parameters.
- `403`: missing or invalid client `key`.
- `413`: prompt or audio payload too large.
- `429`: upstream provider rate limiting.
- `503`: registered provider is missing its server-side credential.
- `504`: upstream timeout.
- `502`: other upstream provider failure.

## Implementation Notes

- Provider/model validation happens at the HTTP edge through a provider registry.
- OpenAI keeps the existing Responses API adapter.
- Non-OpenAI text providers use a shared OpenAI-compatible Chat Completions adapter.
- Gemini uses a native generateContent adapter against `GEMINI_BASE_URL`.
- Dictation routing reuses the multipart transcription adapter with provider-specific URLs.
- Response formatting remains unchanged.

## Test Strategy

Black-box router tests cover:

- OpenAI omitted-provider regression.
- Explicit DeepSeek chat-completions routing.
- Unsupported `web_search` for DeepSeek.
- Known provider without credential.
- Invalid default dictation provider configuration.
- Conflicting JSON body/query models.
- SiliconFlow dictation routing.
- Existing OpenAI dictation and response-format tests.
