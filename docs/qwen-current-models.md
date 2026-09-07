# Qwen 3.8 Candidates

F050 adds five disabled models to the Singapore DashScope provider.
The current `qwen-plus` default remains unchanged.
The catalog creation date for these records is September 5, 2026.
That date identifies registration in this repository.

## Models and prices

Prices are Singapore International list prices in USD per million tokens.
The source was verified on September 5, 2026.
Prices apply to thinking and non-thinking requests with at most 1,000,000 input tokens.

| Exact model | Proxy input | Input price | Output price |
|---|---|---:|---:|
| `qwen3.8-max` | Text and images | 2.00 | 6.00 |
| `qwen3.8-max-0902` | Text and images | 2.00 | 6.00 |
| `qwen3.8-flash` | Text and images | 0.15 | 0.47 |
| `qwen3.8-2.4t-a95b` | Text | 2.00 | 6.00 |
| `qwen3.8-27b` | Text and images | 0.50 | 3.00 |

The two open models use the `qwen3-8` family with `open_weights` metadata.
The other three offerings use the existing Qwen family.
Alibaba's hosted model page declares text input for `qwen3.8-2.4t-a95b`.
The proxy rejects image input for that exact offering.

## Request contract

Requests use the tenant's Singapore workspace URL and synchronous `POST /responses`.
The codec sends `store: false` and preserves the ordered conversation.
Explicit reasoning effort uses `reasoning.effort`.
The four distinct values are `none`, `low`, `medium`, and `xhigh`.
Omission preserves the provider's `xhigh` default.
The proxy rejects `minimal`, `high`, and `max` before dispatch.
Alibaba maps those other names to existing levels.
F050 exposes only the distinct values.

Public `max_tokens` maps to `max_output_tokens`.
The accepted range is 16 through 131,072 tokens.
For Qwen 3.8, this limit includes reasoning and visible output together.
The context window is 1,000,000 tokens.
The model pages also list these input limits:

- Non-thinking input: 991,808 tokens.
- Thinking input: 983,616 tokens.

Those limits depend on the combination of API parameters.
The provider validates the combined input and output budget.
The catalog exposes the context window and the protocol output control.

An error-free terminal `incomplete` response starts a new stateless continuation.
The continuation includes the visible answer and the missing-suffix instruction.
Provider reasoning remains private.
Native usage totals include reasoning tokens without duplicate addition.
The route preserves the current text and image capability scope.
Caller tools, structured output, video, and provider storage require separate contracts.

## Image limits

The four image offerings use ordered `input_image` parts with complete Data URIs.
Each request can contain at most 250 images.
Each complete image Data URI can contain at most 20,000,000 bytes.
The encoded size includes the URI prefix, MIME type, and Base64 content.
The total request bound remains unknown in the catalog.
The existing proxy request and asset limits also apply.
B195 defines this shared admission contract.

## Qualification

Local HTTP tests verify all five exact identifiers, omission, every distinct effort, continuation, usage, and output limits.
Image tests verify ordered PNG and JPEG input for each declared image offering.
They verify image-count rejection and rejection of images for the text-only model.
Catalog tests verify model limits, current prices, reasoning controls, and candidate exclusion from public discovery.
These tests use a local provider protocol implementation.
Final CI passes all 12 gates and 95 browser tests with 100.0% Go statement coverage.

Live qualification requires `DASHSCOPE_API_KEY` and `DASHSCOPE_BASE_URL`.
Both values are absent from the process and all six private repository environment files.
All five text checks and four image checks stopped before provider dispatch with the missing-input error.
No live Qwen 3.8 request ran.
The base URL must identify the tenant's Singapore workspace.
After the operator configures both values, run this command for each candidate:

```sh
LLM_PROXY_LIVE_REASONING_MATRIX=true make test-live-provider-candidate \
  LIVE_CANDIDATE_MODEL=dashscope/qwen3.8-max LIVE_ENV_FILE=configs/.env
```

Replace the model identifier with each exact identifier in the table.
For each image offering, also run:

```sh
make test-live-provider-candidate-media \
  LIVE_CANDIDATE_MODEL=dashscope/qwen3.8-max LIVE_ENV_FILE=configs/.env
```

Keep each model disabled until its key, text, reasoning, and declared image checks pass.
Production deployment belongs to the operator.

## Sources

- [Responses protocol and reasoning levels](https://www.alibabacloud.com/help/en/model-studio/qwen-api-via-openai-responses)
- [Singapore list prices](https://www.alibabacloud.com/help/en/model-studio/model-pricing)
- [Qwen3.8-Max and September 2 snapshot](https://help.aliyun.com/en/model-studio/qwen3-8-max)
- [Qwen3.8-Flash limits and capabilities](https://help.aliyun.com/en/model-studio/qwen3-8-flash)
- [Qwen3.8-2.4T-A95B hosted limits and text input](https://help.aliyun.com/en/model-studio/qwen3-8-2-4t-a95b)
- [Qwen3.8-27B hosted limits and image input](https://help.aliyun.com/en/model-studio/qwen3-8-27b)
- [Base64 image limits](https://www.alibabacloud.com/help/en/model-studio/vision)
