# Current OpenAI GPT-6 routes

F103 adds five exact OpenAI routes: `gpt-6-sol`, `gpt-6.1-sol`, `gpt-6.1-terra`, `gpt-6.2-astra` and `gpt-6.2-sol`. It also verifies the existing `gpt-6-astra` and `gpt-6-luna` declarations against current official references. These seven general-purpose models accept text and images and return text through the existing OpenAI Responses transport. The existing GPT-5.6 offerings and saved provider defaults remain unchanged.

Each route supports strict JSON Schema output, caller function tools and web search through the current proxy interfaces. The `openai_responses_reasoning_tools` profile omits sampling controls and retains the existing stored background-response lifecycle. Model discovery uses catalogue registration time for `created`, as specified by the [client contract](client-protocols.md).

## Reasoning and token limits

| Exact model and official reference | Reasoning efforts | Context tokens | Output tokens |
| --- | --- | ---: | ---: |
| [gpt-6-astra](https://developers.openai.com/api/docs/models/gpt-6-astra) | low, medium, high, xhigh, max | 1,050,000 | 128,000 |
| [gpt-6-sol](https://developers.openai.com/api/docs/models/gpt-6-sol) | none, low, medium, high, xhigh, max | 1,050,000 | 128,000 |
| [gpt-6-luna](https://developers.openai.com/api/docs/models/gpt-6-luna) | none, low, medium, high, xhigh, max | 1,050,000 | 128,000 |
| [gpt-6.1-sol](https://developers.openai.com/api/docs/models/gpt-6.1-sol) | low, medium, high, xhigh, max | 1,050,000 | 128,000 |
| [gpt-6.1-terra](https://developers.openai.com/api/docs/models/gpt-6.1-terra) | none, low, medium, high, xhigh, max | 1,050,000 | 128,000 |
| [gpt-6.2-astra](https://developers.openai.com/api/docs/models/gpt-6.2-astra) | low, medium, high, xhigh, max | 1,050,000 | 128,000 |
| [gpt-6.2-sol](https://developers.openai.com/api/docs/models/gpt-6.2-sol) | low, medium, high, xhigh, max | 1,050,000 | 128,000 |

The GPT-6.2 model references explicitly publish a 922,000-token maximum input. The older references publish context and output limits without a separate maximum-input value. F103 therefore leaves that value unspecified for the newly added GPT-6 Sol, GPT-6.1 Sol and GPT-6.1 Terra offerings. The existing GPT-6 Astra declaration retains its conservative 922,000-token proxy input cap; the current page does not independently establish that separate cap.

The [image guide](https://developers.openai.com/api/docs/guides/images-vision) supplies the request-payload and image-count bounds. The native proxy's smaller encoded-body bound still applies before provider dispatch.

## Standard prices

Prices are USD per million tokens, verified November 21, 2026, against each exact-model reference above.

| Exact model | Input | Output | Cache read |
| --- | ---: | ---: | ---: |
| gpt-6-astra | 10 | 50 | 1.00 |
| gpt-6-sol | 2 | 10 | 0.20 |
| gpt-6-luna | 0.10 | 0.50 | 0.01 |
| gpt-6.1-sol | 2 | 10 | 0.10 |
| gpt-6.1-terra | 1 | 5 | 0.05 |
| gpt-6.2-astra | 8 | 40 | 0.40 |
| gpt-6.2-sol | 2 | 10 | 0.10 |

For requests above 272,000 input tokens, input and cache-read prices double and output prices increase by 50 percent for the complete request. The catalogue retains both tiers. New declarations use `2026-11-21T00:00:00Z` as local price eligibility; that timestamp does not claim a provider publication date.

The newly added routes leave cache-write price selection unavailable. The current catalogue cache classes cannot select the separately priced retention periods documented for GPT-6.1 Terra and GPT-6.2. Existing Astra and Luna cache-write declarations remain unchanged. Standard rates do not describe Batch, Flex, Fast, Ultrafast, regional premiums, discounted background completion windows or negotiated account charges.

## Validation and activation

Table-driven public-contract tests cover all seven exact routes: authenticated and public discovery, every documented reasoning effort, images with strict schema, token-usage headers, unsupported effort and excessive output rejection, and the 272,000/272,001 price boundary. They use a local provider protocol implementation and establish proxy serialization and admission behavior. The catalogue suite checks aggregate public media inventory and verified source dates.

The primary agent owns the final full CI gate and the authorized release, publication and deployment sequence. The current checkout includes unrelated pending changes, so the release requires the curated F103 patch. After activation, read authenticated tenant discovery and qualify the exact route through the existing tenant connection before the benchmark arm. Retain the original idempotency key and use ordinary reconciliation for uncertain requests.

Source preparation makes no live provider request or tenant change. Actual provider access and production activation remain separate from local protocol qualification.
