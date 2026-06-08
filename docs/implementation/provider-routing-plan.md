# Provider Routing Implementation Plan

Issue: `.mprl/ISSUES.md` -> `B001`

## Goal

Extend `llm-proxy` from an OpenAI-only proxy into an explicit multi-provider proxy while preserving current OpenAI defaults for existing clients.

## Request Contract

- `provider` is an optional query parameter on `GET /`, `POST /`, `POST /v2`, and `POST /dictate`.
- Omitted `provider` means the authenticated tenant's default provider.
- `model` keeps its current meaning; omitted `model` means the authenticated tenant's default model when set, otherwise the selected provider's native default.
- Compatibility JSON `POST /` accepts exactly one text input shape: `prompt` for a single user prompt or `messages[]` for an OpenRouter/OpenAI-compatible chat transcript.
- Canonical JSON `POST /v2` accepts only `messages[]` as the text input shape; request-body `prompt` and `system_prompt` are invalid.
- `messages[]` items contain `role` and string `content`. Supported roles are `system`, `user`, and `assistant`; at least one `user` message is required.
- `messages[].order` is optional. When any submitted message includes `order`, every submitted message must include a unique non-negative integer `order`; the proxy sorts submitted messages by ascending `order` before adding a request or tenant system prompt and before routing upstream.
- With `messages[]` on `POST /`, body `system_prompt` is prepended as a system message only when the transcript does not already contain a `system` message. A body containing both `system_prompt` and a system message is invalid. With `POST /v2`, callers send system instructions as `system` role messages.
- `max_tokens` is an optional positive integer on `GET /` query strings and JSON `POST /` bodies.
- Provided `max_tokens` maps to OpenAI Responses `max_output_tokens`, OpenAI-compatible chat completions `max_tokens`, Anthropic Messages `max_tokens`, and Gemini `generationConfig.maxOutputTokens`.
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
| `openai` | none | OpenAI Responses API | OpenAI audio transcription | Supported by existing OpenAI model table |
| `deepseek` | none | OpenAI-compatible chat completions | Not supported | Not supported |
| `dashscope` | `qwen` | OpenAI-compatible chat completions | Not supported | Not supported |
| `moonshot` | `kimi` | OpenAI-compatible chat completions | Not supported | Not supported |
| `siliconflow` | none | OpenAI-compatible chat completions | OpenAI-compatible audio transcription | Not supported |
| `zhipu` | `glm` | OpenAI-compatible chat completions | Not supported | Not supported |
| `gemini` | none | Native Gemini generateContent | Not supported | Not supported |
| `anthropic` | `claude` | Native Anthropic Messages | Not supported | Not supported |
| `grok` | `xai` | xAI OpenAI-compatible chat completions | Not supported | Not supported |

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
- `providers.anthropic.api_key`, `providers.anthropic.base_url`
- `providers.grok.api_key`, `providers.grok.base_url`

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
- Anthropic uses a native Messages adapter, translating proxy `system` messages to the top-level Anthropic `system` parameter and `user`/`assistant` messages to Anthropic `messages[]`.
- Gemini uses a native generateContent adapter against `GEMINI_BASE_URL`.
- Grok uses the shared OpenAI-compatible Chat Completions adapter against `GROK_BASE_URL`, defaulting to `https://api.x.ai/v1`.
- OpenAI-compatible chat providers receive validated and sorted `messages[]` as provider-supported `role` and `content` items.
- Gemini receives user messages as native `contents`, assistant messages as `model` contents, and system messages as `systemInstruction`.
- OpenAI Responses receives single-prompt requests unchanged and multi-message requests as a deterministic role-labelled transcript.
- Dictation routing reuses the multipart transcription adapter with provider-specific URLs.
- Response formatting keeps existing text/XML/CSV bodies and existing JSON `request`, `response`, and normalized `usage` fields. JSON responses also include OpenRouter-style `object`, `model`, and `choices[].message.content` metadata, plus caller-visible request `messages` with provided `order` values. Server-injected tenant default system prompts are sent upstream but not echoed in response metadata.

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
