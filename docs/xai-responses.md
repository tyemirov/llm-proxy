# xAI Responses

All configured xAI text offerings use `xai_responses` with `synchronous_completion`.
The default remains `grok-4.3`. Provider keys and saved model selections retain their existing identities.

The codec sends one `POST /v1/responses` with ordered role messages and `store:false`.
It omits `background`, `previous_response_id`, and requests for encrypted reasoning.
The public `max_tokens` field becomes `max_output_tokens`.
The codec returns visible assistant `output_text` items and valid function calls.
It rejects error envelopes, refusals, unknown output types, and nonterminal synchronous results.
Provider reasoning is not a public result.

An `incomplete` result with reason `max_output_tokens` starts the common continuation loop.
The next request includes the original messages, visible partial output, and the missing-suffix instruction.
Each continuation remains stateless. The proxy sums usage across requests.
It maps `input_tokens`, `output_tokens`, and `total_tokens` without adding reasoning tokens a second time.

Grok 4.5 keeps its current JPEG and PNG image support and media limits.
The disabled [Grok 4.6 candidate](grok-current-model.md) adds four reasoning levels, image input, structured output, and caller functions.
Speech-to-text keeps its separate multipart transcription transport.

## Provider evidence

The official sources were reviewed on 2026-09-05.
The [migration guide](https://docs.x.ai/developers/model-capabilities/text/comparison) specifies Responses as the current text API.
The [REST reference](https://docs.x.ai/developers/rest-api-reference/inference/chat) defines response states, typed output, usage, and storage.
Stored responses have a 30-day retention period. This integration sets `store:false`.

| Configured model identifiers | Model record |
|---|---|
| `grok-4.3`, `grok-4.3-latest` | [Grok 4.3](https://docs.x.ai/developers/models/grok-4.3) |
| `grok-4.5` | [Grok 4.5](https://docs.x.ai/developers/models/grok-4.5) |
| `grok-4.20-0309-reasoning` | [Grok 4.20 reasoning](https://docs.x.ai/developers/models/grok-4.20-0309-reasoning) |
| `grok-4.20-0309-non-reasoning` | [Grok 4.20 non-reasoning](https://docs.x.ai/developers/models/grok-4.20-0309-non-reasoning) |
| `grok-build-0.1`, `grok-code-fast`, `grok-code-fast-1`, `grok-code-fast-1-0825` | [Grok Build and its published aliases](https://docs.x.ai/developers/models/grok-build-0.1) |
| `grok-latest` | Existing catalog selection. Exact current target requires live verification. |

Local HTTP tests prove serialization and parsing for all ten configured text offerings.
These tests do not prove model access through the provider account.

## Live acceptance

Use the existing repository environment file with a valid `XAI_API_KEY`.
Run the catalog-derived matrix before production activation:

```bash
LLM_PROXY_LIVE_PROVIDERS=xai LLM_PROXY_LIVE_ALL_MODELS=true \
  make test-live-providers LIVE_ENV_FILE=configs/.env
```

The matrix performs a key verification and a text request for each configured text model.
Record failures by exact model identifier. Keep I041 open until every route passes.
Production activation belongs to the operator.
