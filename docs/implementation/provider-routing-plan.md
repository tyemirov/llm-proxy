# Provider Routing Implementation Plan

Issue: `.mprl/ISSUES.md` -> `B001`

## Goal

Extend `llm-proxy` from an OpenAI-only proxy into an explicit multi-provider proxy while preserving current OpenAI defaults for existing clients.

## Request Contract

- `provider` is an optional query parameter on `GET /`, `POST /`, and `POST /dictate`.
- Omitted `provider` means `openai`.
- `model` keeps its current meaning.
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

## Configuration

Shared:

- `SERVICE_SECRET`
- `LLM_PROXY_DEFAULT_PROVIDER`
- `LLM_PROXY_DEFAULT_MODEL`
- `LLM_PROXY_DEFAULT_DICTATION_PROVIDER`
- `LLM_PROXY_DICTATION_MODEL`

Provider credentials and base URLs:

- `OPENAI_API_KEY`
- `DEEPSEEK_API_KEY`, `DEEPSEEK_BASE_URL`
- `DASHSCOPE_API_KEY`, `DASHSCOPE_BASE_URL`
- `MOONSHOT_API_KEY`, `MOONSHOT_BASE_URL`
- `SILICONFLOW_API_KEY`, `SILICONFLOW_BASE_URL`, `SILICONFLOW_TRANSCRIPTIONS_URL`
- `ZHIPU_API_KEY`, `ZHIPU_BASE_URL`

Startup validates `SERVICE_SECRET`, the credential required by the configured default text provider, and the endpoint/credential required by the configured default dictation provider. Credentials for non-default providers are validated when a request selects that provider.

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
