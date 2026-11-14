# Alibaba Cloud Responses

## Shared codec

I259 uses the shared Responses codec with the DashScope protocol variation.
The catalog selects this variation through `dashscope_responses`.
The shared code owns common message serialization, response items, and usage translation.
The DashScope variation omits image detail and caller tools.
An error-free terminal `incomplete` result starts continuation for ordinary text.
Structured generation fails at an incomplete result and never continues partial JSON.

Its execution remains synchronous. Requests use `store:false` and omit `background`.
Local protocol tests establish code behavior. The live acceptance requirements below remain separate.

All five active Qwen offerings in the source catalog use `dashscope_responses` with
`synchronous_completion`. The default remains `qwen-plus`.

| Model | Input |
|---|---|
| `qwen-plus` | Text |
| `qwen3.7-max` | Text |
| `qwen3.7-plus` | Text and images |
| `qwen3.6-flash` | Text and images |
| `qwen3.8-flash` | Text and images |

The private catalog also contains four disabled [Qwen 3.8 candidates](qwen-current-models.md).
Those models add explicit reasoning control through the same Responses codec.

The tenant assigns an Alibaba Cloud connection with a Singapore workspace URL and the matching regional API key.
Use the English [API-key instructions](https://www.alibabacloud.com/help/en/model-studio/get-api-key) to create the key.
Generation and credential verification append `/responses` to that URL.
Model selectors, tenant defaults, credentials, and historical usage retain
their existing identities.

The codec preserves message order. It sends `store: false` and maps public
`max_tokens` to `max_output_tokens`. A supplied limit must be at least 16.
The public API rejects a smaller limit before provider work. Image parts use
`input_image` and a Data URL. The request includes only documented fields.
The nominal context window differs from preserved input context: Responses reserves approximately 20% of the model window and automatically truncates excess input.
The provider documents no truncation-disable control or exact preserved input threshold.
See the [Flash production contract](qwen-flash-production.md) before selecting long-context workloads.

Each request can contain at most 250 Base64 images.
Each complete image Data URI can contain at most 20,000,000 bytes.
The `attachment_data_uri_bytes` scope includes the MIME type, URI prefix, and Base64 content.
This encoded limit also keeps each raw image below the provider's 20 MB raw limit.
The total request bound remains unknown in the catalog.
B195 applies these limits to all three active DashScope image offerings.

The codec reads assistant `output_text` content and native usage totals.
Reasoning details remain private. Reasoning tokens are part of the output
breakdown and are not added again. An error-free terminal `incomplete` result
starts the shared missing-suffix continuation. Each continuation is a new
stateless POST. Nonterminal results and provider errors return safe failures.

## Structured generation

Only the exact `qwen3.8-flash` offering declares strict JSON Schema support.
The native structured request sends `text.format` with `type: json_schema`,
`strict: true`, and the supplied supported schema through the same Responses codec.
The proxy validates the completed JSON against that schema before returning success.
Unsupported schema keywords and external references fail before provider work.
The conservative string-format allowlist contains `email` and `date`, the documented
examples. This proxy policy does not claim that Alibaba rejects every other format.
Structured request identity, replay, and reconciliation use the existing request resource.

Schema-constrained requests must contain only text.
The proxy rejects image inputs with a schema because Alibaba documents a downgrade
from JSON Schema to JSON object mode for multimodal input.
This restriction also applies to compatible client protocols and stored media assets.
Ordinary Flash image requests retain their existing contract.

Use explicit reasoning effort `none` for the cost-oriented TellTale route.
The provider output limit includes reasoning and visible tokens; incomplete structured
responses fail even when the visible prefix happens to be valid JSON.
Local protocol coverage and live application qualification are separate checks.
Sources: [Responses parameters](https://docs.modelstudio.console.alibabacloud.com/en/model-studio/qwen-api-via-openai-responses)
and [supported schema keywords](https://docs.modelstudio.console.alibabacloud.com/en/model-studio/qwen-structured-output).

## Acceptance

### Inventory export

F064 adds `llm-proxy inventory-dashscope` for operator review.
Set `DASHSCOPE_API_KEY` and `DASHSCOPE_BASE_URL` in the process before execution.
Use the matching Singapore or US East workspace URL with the `/compatible-mode/v1` path.

```sh
make inventory-dashscope > /tmp/dashscope-inventory.json
```

The command retrieves separate complete `supports=inference` and `supports=deploy` inventories.
It emits one JSON document after both queries pass validation.
It preserves model metadata, exact price strings, request IDs, and page counts.
The export identifies the access region. The workspace service deployment scope remains unknown.
The command does not change the runtime catalog or activate provider offerings.

The operation has a two-minute limit. Each HTTP request has a 30-second limit.
Each collection permits at most 20,000 models. Each response page permits at most 2 MiB.
The complete operation permits at most 32 MiB of response data.
An incomplete or excessive inventory returns an error without an export.

`make test-dashscope-inventory` exercises the CLI through a local provider protocol server.

The current regional contract selects workspace hosts.
The Models API page still shows a shared US hostname.
Live qualification must confirm the US workspace inventory endpoint before US East acceptance.
See the [back-end execution plan](backend-execution-plan.md) for account preparation and remaining F064 milestones.

### Route qualification

`make test-dashscope-responses` checks the public route contract.
`make test-dashscope-media-limits` checks count and complete URI admission.
The full suite checks workspace credential verification, image serialization,
client discovery, and browser catalog search. On 2026-09-05, final CI passed
all 12 gates with 100% Go statement coverage and 95 browser tests.

Live acceptance requires `DASHSCOPE_API_KEY` and `DASHSCOPE_BASE_URL`:

```bash
LLM_PROXY_LIVE_PROVIDERS=dashscope LLM_PROXY_LIVE_ALL_MODELS=true \
  make test-live-providers LIVE_ENV_FILE=configs/.env
```

On 2026-09-05 both values were absent from the process and authorized repository
input files. Provider acceptance remains open for all four models.

On 2026-11-13, the approved Singapore connection passed account verification.
Qwen 3.8 Flash passed omitted and all four explicit reasoning cases plus ordered PNG/JPEG input through the disposable proxy.
All six generation requests reported positive, consistent token usage.
Flash is enabled in source; [production activation](qwen-flash-production.md) remains separate.
The complete active-route matrix needs the remaining I226 model grants and live checks.
See the [execution plan](backend-execution-plan.md) for the current scope and next model priority.

## Sources

Alibaba's [Responses reference](https://www.alibabacloud.com/help/en/model-studio/qwen-api-via-openai-responses),
checked on 2026-09-05, documents the three newer Qwen selectors and basic
compatibility for other Model Studio text models, including `qwen-plus`.
The [workspace URL reference](https://www.alibabacloud.com/help/en/model-studio/base-url)
defines the Singapore connection boundary.
The [vision guide](https://www.alibabacloud.com/help/en/model-studio/vision) defines the Base64 count and complete Data URI limits.
