# Current OpenAI GPT-6 routes

F103 adds five exact OpenAI routes: `gpt-6-sol`, `gpt-6.1-sol`, `gpt-6.1-terra`, `gpt-6.2-astra` and `gpt-6.2-sol`. It also verifies the existing `gpt-6-astra` and `gpt-6-luna` declarations against current official references. These seven general-purpose models accept text and images and return text through the existing OpenAI Responses transport. GPT-5.6 and GPT-6 offerings remain selectable. The approved GPT-5.6 cutoff below replaces saved selections for seven older routes.

Each route supports strict JSON Schema output, caller function tools and web search through the current proxy interfaces. The `openai_responses_reasoning_tools` profile omits sampling controls and retains the existing stored background-response lifecycle. Model discovery uses catalog registration time for `created`, as specified by the [client contract](client-protocols.md).

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

For requests above 272,000 input tokens, input and cache-read prices double and output prices increase by 50 percent for the complete request. The catalog retains both tiers. New declarations use `2026-11-21T00:00:00Z` as local price eligibility; that timestamp does not claim a provider publication date.

The newly added routes leave cache-write price selection unavailable. The current catalog cache classes cannot select the separately priced retention periods documented for GPT-6.1 Terra and GPT-6.2. Existing Astra and Luna cache-write declarations remain unchanged. Standard rates do not describe Batch, Flex, Fast, Ultrafast, regional premiums, discounted background completion windows or negotiated account charges.

## GPT-5.6 cutoff and stored selections

The current catalog removes these seven OpenAI exact models and their offerings. Explicit requests for a removed route fail before upstream dispatch, and discovery lists only retained routes. The bounded startup cutover uses this approved tier mapping:

| Retired selection | Replacement |
| --- | --- |
| `gpt-4o-mini`, `gpt-5-mini` | `gpt-6-luna` |
| `gpt-4o`, `gpt-4.1`, `gpt-5`, `gpt-5.5` | `gpt-6.2-sol` |
| `gpt-5.5-pro` | `gpt-6.2-astra` |

The global, OpenAI provider and new-tenant text defaults select `gpt-6.2-sol`. OpenAI connection verification uses the same route. The `gpt-image-2` Responses orchestration parent is also `gpt-6.2-sol`, using the existing `image_generation` tool payload documented by [OpenAI](https://developers.openai.com/api/docs/guides/tools-image-generation).

Before any database write, startup reads affected tenant defaults and OpenAI provider profiles and validates each replacement against its inherited tenant reasoning effort. It preserves omission and every supported explicit effort. `none` remains valid for mini selections moving to Luna. A `none` selection moving to Sol or Astra stops startup with `provider_reasoning_decision_required`, tenant, profile/default scope, source, target and effort. The operator must resolve that specific effort decision before activation.

The version-18 transaction changes affected model columns only. It preserves efforts, timestamps, credentials, prompts, connection assignments and historical usage identities. Existing account-connection databases apply it before current-route validation. Retained predecessor schemas first complete their existing normalization; validation projects the final route during that bounded transfer. Fresh databases retain the versionless current schema. Repeated startup performs no selection update once the cutover has completed and rejects reintroduced retired selections.

Accepted structured requests retain their original provider, model, intent and result authority. Readback and reconciliation use the original accepted execution; the cutoff creates no replacement inference. A dispatched request recovered after restart becomes `uncertain` under the ordinary structured-request contract. Retain its idempotency key while reconciling the original outcome.

Before production activation, use an ORM read-only metadata inventory to record affected defaults, profiles and inherited efforts, historical usage counts, and unsettled hosted, media and native structured-request identities. Compare those records after activation and verify tenant discovery and an explicit retained route. Keep provider credentials and request/result content outside this inventory.

## Validation and activation

Table-driven public-contract tests cover all seven exact routes: authenticated and public discovery, every documented reasoning effort, images with strict schema, token-usage headers, unsupported effort and excessive output rejection, and the 272,000/272,001 price boundary. They use a local provider protocol implementation and establish proxy serialization and admission behavior. The catalog suite checks aggregate public media inventory and verified source dates. Startup tests cover current and predecessor inputs, supported and omitted efforts, zero-write incompatible-effort rejection, transaction rollback, repeated startup and unchanged historical usage. Public cutoff tests verify all seven removed routes, default dispatch and original-request reconciliation with zero new upstream calls.

After deployment, verify the tenant's model list through authenticated discovery. Qualify the exact model and reasoning effort through the existing tenant connection with a representative image and strict JSON Schema. Use the original idempotency key when reconciling uncertain requests.
