# Grok 4.6 Candidate

F048 adds `grok-4.6` as a disabled xAI offering in the existing Grok family.
The provider default remains `grok-4.3`.
The model was released on August 12, 2026.

## Request contract

The route uses synchronous `POST /v1/responses` with `store:false`.
The adapter sends explicit reasoning effort in `reasoning.effort`.
Supported efforts are `low`, `medium`, `high`, and `xhigh`.
Omission preserves the provider's `high` default.
The proxy rejects `none`, `minimal`, and `max` before provider dispatch.
Provider reasoning summaries remain private.

The model accepts text, JPEG, and PNG input.
It supports the current structured output and caller function contracts.
The context window is 500,000 tokens.
The reviewed model page does not specify a separate maximum output size.
The catalog leaves that output limit unspecified.

Each image can contain at most 20 MiB of raw data.
The image count is unbounded.
The total inline request bound is unknown in the provider documentation.
The proxy retains its own request and asset admission limits.

## Standard prices

Prices are USD per million tokens, verified on September 5, 2026.

| Input-token count | Input | Output | Cache read |
|---|---:|---:|---:|
| Below 200,000 | 2.00 | 6.00 | 0.50 |
| At least 200,000 | 4.00 | 12.00 | 1.00 |

The threshold applies to all tokens in the request.
The catalog records both price tiers with explicit input-token conditions.

## Qualification

Local HTTP tests verify exact model dispatch, effort, images, structured output, usage, private reasoning, and function-call continuation.
Public discovery excludes this candidate.
These tests use a local provider protocol implementation.

`XAI_API_KEY` is absent from the process and all six private repository environment files.
The disposable candidate command stops with `required catalog environment is not set: XAI_API_KEY`.
No live Grok 4.6 request ran.

After the operator configures the key, run these checks:

```sh
LLM_PROXY_LIVE_REASONING_MATRIX=true make test-live-provider-candidate \
  LIVE_CANDIDATE_MODEL=xai/grok-4.6 LIVE_ENV_FILE=configs/.env
make test-live-provider-candidate-media \
  LIVE_CANDIDATE_MODEL=xai/grok-4.6 LIVE_ENV_FILE=configs/.env
```

The first command verifies the key and all catalog efforts in a disposable proxy.
The second command verifies image input.
Live structured output and a complete function-call round trip also require acceptance evidence before activation.
Keep the model disabled until all provider checks pass.
Production deployment belongs to the operator.

## Sources

- [Model contract](https://docs.x.ai/developers/models/grok-4.6)
- [Release announcement](https://x.ai/news/grok-4-6)
- [Reasoning levels and payload](https://docs.x.ai/developers/model-capabilities/text/reasoning)
- [Image formats and limits](https://docs.x.ai/developers/model-capabilities/images/understanding)
- [Standard and long-context prices](https://docs.x.ai/developers/pricing)
