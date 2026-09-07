# Current Z.AI Candidates

F051 adds disabled text offerings for `glm-5.3` and `glm-5.3-flash`.
F052 adds image input for the disabled Flash candidate.
They use the existing `glm-5` family with open weights.
The provider default remains `glm-5.1`.
Z.AI lists release dates of August 18 and August 26, 2026, respectively.
The catalog creation timestamps use midnight UTC on those dates.

## Text contract

Both models use synchronous `POST /api/paas/v4/chat/completions` on `https://api.z.ai`.
The existing `chat_completions_thinking` adapter sends `thinking.type: enabled`.
It sends explicit effort through top-level `reasoning_effort`.
The accepted values are `low`, `high`, and `max`.
Omission preserves the provider default of `max`.
The proxy rejects other values before dispatch.

Public `max_tokens` maps directly to the same upstream field.
The maximum output budget is 131,072 tokens.
The model guides publish a 1M-token context window.
The catalog records this as 1,000,000 tokens.
The provider checks the combined input and output budget.

The existing continuation path returns private `reasoning_content` to the provider when output stops at its limit.
It returns only visible text to the caller and combines usage across requests.
The adapter keeps the general API default for `clear_thinking`.
The separate preserved-thinking mode is outside this addition.

The GLM 5.3 endpoint table lists the Coding Plan URL.
Its request examples and the API reference use the general API URL.
This implementation follows the general API examples and the existing I225 routing contract.
Live qualification must confirm both models on that endpoint.

GLM 5.3 accepts text input and output.
Flash also accepts ordered JPEG and PNG attachments under F052.
The adapter sends image blocks before the message text and preserves image order.
The catalog declares Flash formats through `image_mime_types`.

Caller tools and structured output require separate acceptance contracts.

## Flash image admission

The public catalog exposes `image_width_pixels` and `image_height_pixels`.
Both limits use `pixels`, the `attachment` scope, and a maximum value of 6000.
The proxy reads each image header before dispatch.
It rejects dimensions above either maximum with HTTP 413.
It rejects malformed headers, MIME mismatches, and unsupported formats with HTTP 400.
The same checks apply to inline attachments and tenant assets.
Header inspection preserves the complete asset bytes for serialization.

The provider schema specifies images under 5M without a precise byte unit.
Its image-count limits name older models and do not explicitly include Flash.
The candidate records its image byte limit, image count, and total request limit as `unknown`.
These states describe unresolved bounds and do not mean unlimited provider capacity.
Qualification must establish those numeric limits before activation.

## Prices

These prices use USD per million tokens, verified on September 5, 2026.

| Model and rate | Input | Output | Cache read |
|---|---:|---:|---:|
| GLM 5.3 list | 1.40 | 4.40 | 0.26 |
| GLM 5.3 Flash list | 0.15 | 0.50 | 0.03 |
| GLM 5.3 Flash promotion | 0.075 | 0.25 | 0.015 |

The Flash promotion ends at midnight after September 9 in Singapore.
This instant is September 9, 2026, at 16:00 UTC.
The promotional condition excludes that instant and all later times.
The catalog retains both list and promotional rates with separate conditions.
These records describe provider prices and do not select a rate automatically.
Z.AI describes cache storage as temporarily free without an end date.
The catalog omits that unspecified storage rate.

## Qualification

Public HTTP tests cover both models, all three text endpoints, omission, each accepted effort, continuation, and usage totals.
They also verify rejection of unsupported input and excessive output budgets.
Catalog and browser tests verify candidate exclusion from runtime discovery.
The tests use a local provider protocol implementation.
Final F052 CI passes all 12 gates and 96 browser tests with 100.0% Go statement coverage.

`ZAI_API_KEY` is absent from the process and all six private repository environment files.
Both disposable live commands stopped before dispatch with `required catalog environment is not set: ZAI_API_KEY`.
The Flash media command stopped with the same missing-key error.
No live request ran.
Live qualification requires that key.
After the operator configures it, run both commands:

```sh
LLM_PROXY_LIVE_REASONING_MATRIX=true make test-live-provider-candidate \
  LIVE_CANDIDATE_MODEL=zai/glm-5.3 LIVE_ENV_FILE=configs/.env
LLM_PROXY_LIVE_REASONING_MATRIX=true make test-live-provider-candidate \
  LIVE_CANDIDATE_MODEL=zai/glm-5.3-flash LIVE_ENV_FILE=configs/.env
```

For Flash image qualification, also run:

```sh
make test-live-provider-candidate-media \
  LIVE_CANDIDATE_MODEL=zai/glm-5.3-flash LIVE_ENV_FILE=configs/.env
```

Keep each candidate disabled until its live qualification passes.
Production deployment belongs to the operator.

## Sources

- [GLM 5.3 model guide](https://docs.z.ai/guides/llm/glm-5.3)
- [Flash model guide](https://docs.z.ai/guides/vlm/glm-5.3-flash)
- [Release dates](https://docs.z.ai/release-notes/new-released)
- [Chat Completions](https://docs.z.ai/api-reference/llm/chat-completion)
- [Thinking modes](https://docs.z.ai/guides/capabilities/thinking-mode)
- [Image schema](https://docs.z.ai/openapi.json)
- [Prices](https://docs.z.ai/guides/overview/pricing)
- [GLM 5.3 weights](https://huggingface.co/zai-org/GLM-5.3)
- [Flash weights](https://huggingface.co/zai-org/GLM-5.3-Flash)
