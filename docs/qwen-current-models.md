# Hosted Qwen 3.8 Models

F050 adds five models to the Singapore DashScope provider.
Qwen 3.8 Flash passed live qualification on November 13, 2026 and is enabled in the source catalog.
The other four offerings remain disabled pending their model grants and qualification.
Release, deployment, and tenant acceptance are tracked in the [production runbook](qwen-flash-production.md).
The current `qwen-plus` default remains unchanged.
The catalog creation date for these records is September 5, 2026.
That date identifies registration in this repository.

## Models and prices

Prices are Singapore International list prices in USD per million tokens.
Flash prices were verified on November 13, 2026. The other price records retain their September 5 verification date.
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
That is the nominal model window. Alibaba's Responses reference states that effective input context is approximately 80% of the model window and excess input is automatically truncated.
It documents no control to disable this truncation or require overflow rejection.
The nominal window does not guarantee preservation of a 1,000,000-token input.
The approximate percentage does not establish an exact safe token threshold.
The model pages also list these input limits:

- Non-thinking input: 991,808 tokens.
- Thinking input: 983,616 tokens.

Those model-level limits do not override the Responses-specific restriction.
The catalog exposes the context window and the protocol output control.

For ordinary text, an error-free terminal `incomplete` response starts a new stateless continuation.
The continuation includes the visible answer and the missing-suffix instruction.
Provider reasoning remains private.
Native usage totals include reasoning tokens without duplicate addition.
The output limit applies to each provider response. A stateless continuation can add provider requests and charges.
Flash additionally declares [strict text-only JSON Schema generation](dashscope-responses.md#structured-generation).
Structured incomplete responses fail without continuation; schema-plus-image requests fail before dispatch.
The other Qwen offerings retain their text and declared image scope.
Caller tools, video, and provider storage require separate contracts.

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
The September 5 implementation CI passed all 12 gates and 95 browser tests with 100.0% Go statement coverage.
The November 13 Flash continuation passed all 15 current CI gates with 100.0% Go statement coverage.
That run includes 177 frontend browser tests and 13 authentication browser tests.

Live qualification requires `DASHSCOPE_API_KEY` and `DASHSCOPE_BASE_URL`.
The initial September 5 checks found both values absent from the authorized inputs.
All five text checks and four image checks stopped before provider dispatch at that time.

On November 13, 2026, the user approved a scoped Singapore key and private workspace configuration.
The authenticated inventory reports the published context and output limits for `qwen3.8-flash`.
Account connection verification, five text requests, and one ordered image request passed through the real disposable proxy and saved connection.
The text matrix covered omitted effort, `none`, `low`, `medium`, and `xhigh`.
The image request contained a red PNG followed by a blue JPEG and returned exactly `RED BLUE`.
All six generation requests returned HTTP 200, valid request IDs, positive measured usage, and consistent input/output/total counts.
Observed request times ranged from 809 to 2,593 milliseconds. This sample establishes the qualification cases, not sustained load capacity.
Public HTTP tests cover Flash discovery, explicit dispatch, retained `qwen-plus` defaults, 250/251-image admission, and complete Data URI bounds.
An automated Chromium test verifies explicit Flash selection and persistence after page reload.
The other four candidates require model grants and their complete live qualification.

The [Flash execution record](../.mprlab/evidence/F050-flash-20261113.json) retains safe qualification evidence.
The earlier [inventory record](../.mprlab/evidence/F064-inventory-20261113.json) retains the account and workspace milestone.

To requalify the enabled Flash offering:

```sh
LLM_PROXY_LIVE_PROVIDERS=dashscope LLM_PROXY_LIVE_DASHSCOPE_MODEL=qwen3.8-flash \
  LLM_PROXY_LIVE_QUALIFICATION=true LLM_PROXY_LIVE_REASONING_MATRIX=true \
  make test-live-providers LIVE_ENV_FILE=configs/.env

LLM_PROXY_LIVE_PROVIDERS=dashscope LLM_PROXY_LIVE_DASHSCOPE_MODEL=qwen3.8-flash \
  LLM_PROXY_LIVE_QUALIFICATION=true \
  make test-live-provider-media LIVE_ENV_FILE=configs/.env
```

Qualification caps each provider response at 4,096 tokens. Continuation can add calls; this is not a total spending cap.

The base URL must identify the tenant's Singapore workspace.
For each remaining disabled candidate, run:

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
