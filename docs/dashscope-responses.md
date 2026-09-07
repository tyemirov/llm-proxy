# DashScope Responses

All four active Qwen offerings use `dashscope_responses` with
`synchronous_completion`. The default remains `qwen-plus`.

| Model | Input |
|---|---|
| `qwen-plus` | Text |
| `qwen3.7-max` | Text |
| `qwen3.7-plus` | Text and images |
| `qwen3.6-flash` | Text and images |

The private catalog also contains five disabled [Qwen 3.8 candidates](qwen-current-models.md).
Those models add explicit reasoning control through the same Responses codec.

The tenant saves its Singapore workspace URL and matching regional API key.
Generation and credential verification append `/responses` to that URL.
Model selectors, tenant defaults, credentials, and historical usage retain
their existing identities.

The codec preserves message order. It sends `store: false` and maps public
`max_tokens` to `max_output_tokens`. A supplied limit must be at least 16.
The public API rejects a smaller limit before provider work. Image parts use
`input_image` and a Data URL. The request includes only documented fields.

Each request can contain at most 250 Base64 images.
Each complete image Data URI can contain at most 20,000,000 bytes.
The `attachment_data_uri_bytes` scope includes the MIME type, URI prefix, and Base64 content.
This encoded limit also keeps each raw image below the provider's 20 MB raw limit.
The total request bound remains unknown in the catalog.
B195 applies these limits to both existing DashScope image offerings.

The codec reads assistant `output_text` content and native usage totals.
Reasoning details remain private. Reasoning tokens are part of the output
breakdown and are not added again. An error-free terminal `incomplete` result
starts the shared missing-suffix continuation. Each continuation is a new
stateless POST. Nonterminal results and provider errors return safe failures.

## Acceptance

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

## Sources

Alibaba's [Responses reference](https://www.alibabacloud.com/help/en/model-studio/qwen-api-via-openai-responses),
checked on 2026-09-05, documents the three newer Qwen selectors and basic
compatibility for other Model Studio text models, including `qwen-plus`.
The [workspace URL reference](https://www.alibabacloud.com/help/en/model-studio/base-url)
defines the Singapore connection boundary.
The [vision guide](https://www.alibabacloud.com/help/en/model-studio/vision) defines the Base64 count and complete Data URI limits.
