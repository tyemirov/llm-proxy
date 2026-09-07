# GPT-6 Astra

F061 adds the enabled `openai/gpt-6-astra` offering.
Select this exact model in a tenant profile or a public request.
The existing OpenAI key and Responses transport apply to this model.
The OpenAI provider default remains `gpt-4.1`.

## Request contract

The route accepts text and images and returns text.
It supports structured output, caller function tools, and web search through the current public interfaces.
Supported reasoning efforts are `low`, `medium`, `high`, `xhigh`, and `max`.
Omit the effort to use the provider default.
The proxy rejects `none`, `minimal`, and a requested output above 128,000 tokens before provider dispatch.

Public `max_tokens` maps to Responses `max_output_tokens`.
The route uses `background: true` and `store: true` with the existing response lifecycle.
The output budget includes reasoning tokens.
Usage totals include these tokens, but visible output excludes reasoning summaries.

| Limit | Value |
|---|---:|
| Context | 1,050,000 tokens |
| Input | 922,000 tokens |
| Output | 128,000 tokens |
| Images per request | 1,500 |
| Encoded image request | 512,000,000 bytes |

The proxy accepts JPEG, PNG, and WebP images through its current image contract.
The catalog leaves the individual image byte limit unspecified.

## Standard prices

Prices are USD per million tokens, verified on September 7, 2026.
The input token count selects the price tier for the complete request.

| Component | Input at most 272,000 tokens | Input above 272,000 tokens |
|---|---:|---:|
| Input | 10 | 20 |
| Output | 50 | 75 |
| Cache read | 1 | 2 |
| Cache write | 12.5 | 25 |

## Acceptance

On September 7, 2026, the existing OpenAI key passed exact model verification with HTTP 200.
Live requests through a disposable proxy passed with omitted effort and all five explicit efforts.
Separate live checks passed for image input, structured output, caller function calls, and web search.
These results permit catalog activation.
Production deployment belongs to the operator.

Local HTTP tests verify routing, controls, image order, structured output, function continuation, usage, and discovery.
Browser tests verify model search and the exact reasoning control options.
Use these commands for focused checks:

```sh
make test-astra
LLM_PROXY_LIVE_REASONING_MATRIX=true make test-live-provider-candidate \
  LIVE_CANDIDATE_MODEL=openai/gpt-6-astra
make test-live-provider-candidate-media LIVE_CANDIDATE_MODEL=openai/gpt-6-astra
make test-live-astra-capabilities
```

The live commands use the existing `OPENAI_API_KEY` and incur provider charges.

## Sources

- [Astra model contract](https://developers.openai.com/api/docs/models/gpt-6-astra)
- [Current model guide](https://developers.openai.com/api/docs/guides/latest-model)
- [Standard pricing](https://developers.openai.com/api/docs/pricing)
- [Image input limits](https://developers.openai.com/api/docs/guides/images-vision)
