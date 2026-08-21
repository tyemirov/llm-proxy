# Dictation Endpoint Contract (`/dictate`)

Status: current implementation contract.

## Purpose

LLM Proxy exposes one authenticated dictation endpoint for supported audio transcription providers.
Client applications use one tenant key. LLM Proxy owns provider credentials,
provider transports, the provider catalog, routing, and error normalization.

## Public API Contract

- Use `POST /dictate?key=TENANT_KEY` with `multipart/form-data`.
- Send the audio file in the required `audio` form part.
- Use optional `provider` and `model` query parameters to select an exact configured route.
- Omit both selectors to use the tenant dictation default.
- Use optional `X-LLM-Proxy-Request-Timeout-Seconds` for the bounded proxy work budget.
- Keep the audio payload within `server.max_input_audio_bytes`.
- Receive HTTP `200` with JSON `{ "text": "transcription" }` after successful transcription.

The canonical OpenAPI artifact defines request validation and response status details.
The endpoint returns `400`, `403`, `413`, `429`, `499`, `502`, `503`, or `504` when the documented condition occurs.

## Provider Contract

| Provider selector | Models | Provider transport |
|---|---|---|
| `openai` | `gpt-4o-mini-transcribe`, `gpt-4o-transcribe` | OpenAI `dictation` transport |
| `siliconflow` | `sensevoice-small` | SiliconFlow `dictation` transport |
| `zai` | `glm-asr-2512` | Z.AI `dictation` transport |
| `xai` | `xai-stt` | xAI `dictation` transport |

OpenAI is the default dictation provider. Its default model is `gpt-4o-mini-transcribe`.
The selected provider offering references one transport in
[`configs/providers.yml`](../../configs/providers.yml). The transport supplies
the endpoint and authentication field. The tenant provider connection supplies
the credential value.

OpenAI, SiliconFlow, and Z.AI receive the selected model in the upstream multipart request.
xAI uses its configured STT endpoint and does not receive an upstream multipart model field.

The adapter accepts nonblank upstream `text`, `transcript`, or `output_text` JSON fields.
It also accepts a nonblank plain-text upstream response. Empty or malformed success payloads return a sanitized provider failure.

## Security And Privacy

- Authenticate each request with the tenant key in the `key` query parameter.
- Reject provider credential fields in query parameters and multipart fields.
- Keep upstream provider keys and authorization headers inside the backend runtime.
- Keep query strings, request bodies, cookies, authorization headers, and raw provider bodies out of request logs.
- Record only normalized, content-free managed usage data.

## Validation

- Exercise `/dictate` through the public HTTP router.
- Verify tenant authentication, provider and model resolution, multipart validation, and the audio-size limit.
- Verify successful provider routing, timeout handling, rate-limit handling, sanitized failures, and managed usage recording.
- Run `timeout -k 350s -s SIGKILL 350s make ci` after a source, test, config, dependency, or build change.
