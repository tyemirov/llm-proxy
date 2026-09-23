# Hosted Billing

F070 defines the prepaid hosted service. F065 through F069 own its implementation in that order.
This document records the shared contract and current decisions.
The service is not complete. Hosted activation remains disabled until F070 acceptance passes.

## Product Decisions

On 2026-09-22, the operator confirmed a 30% markup on provider prices.
The operator selected all providers and their supported operations.
The implementation must use one shared billing contract across providers.

The minimum funding amount is USD 5. Customers can spend their balance down to USD 0.
The service does not require an unspent USD 5 reserve.
The operator selected Paddle for payments. The service must obey the applicable Paddle financial policies.

The operator required reuse of the existing PoodleScanner and Hecate integrations and implementation approaches.
LLM Proxy has a browser application. Native mobile applications are outside this scope.

On 2026-09-23, the operator selected the existing Ledger account model and balance conservation for acceptance.
Use the existing append-only journal and verify each account balance against its recorded financial effects.
Include exact remainders, active reservations, and payment reversal holds in these checks.
F069 must extend conservation checks to payment credits, refunds, reversals, and recovery.

```text
customer_price = provider_price * 1.30
               = provider_price * 13 / 10
```

For example, a provider cost of USD 1 produces a customer price of USD 1.30 before monetary rounding.
The markup calculation uses provider prices before payment fees, taxes, or other platform expenses.
The service must retain those amounts separately from provider costs and customer usage charges.

F067 must retain the exact calculation until its specified monetary rounding boundary.
Each accepted request must retain its provider rates and customer rates in one immutable price snapshot.
Later catalog changes must not change an accepted price snapshot.
Unavailable provider rates must remain unavailable for hosted admission.
This restriction is an admission requirement, not a reduction of provider scope.
Each supported provider and operation requires metering, bounded pricing, and qualification evidence.
Missing evidence remains an explicit implementation or qualification gap.
Provider additions must use the same financial contract.

## Exact Rating and Ledger Amounts

Provider rates use exact decimal strings. Customer rates use the exact multiplier `13/10`.
The rating result retains reduced rational amounts for provider costs and customer charges.
A minute has 60 seconds. An hour has 3600 seconds.
The catalog uses `USD/1M_tokens` for one million tokens.
Cache storage uses `USD/1M_token_hours` and measured token-seconds.

A priced child replaces its part of an inclusive parent quantity.
Reasoning tokens that remain part of output tokens do not create another charge.
Unknown required quantities prevent settlement. They do not become zero quantities.
A provider minimum applies before the customer markup.

Ledger entries use integer USD cents. The maximum entry is `9223372036854775807` cents.
Settlement adds the retained account remainder before conversion to cents.
Settlement rounds down once and retains the exact remainder, which is less than one cent.
F068 must commit the remainder and the ledger entry in one transaction.
An exact usage credit reduces this remainder before it credits whole cents.
The conversion preserves the same fractional accounting boundary:

```text
residual = previous_remainder - exact_credit
credited_cents = max(0, ceil(-residual * 100))
next_remainder = residual + credited_cents / 100
```

The credit transaction retains the previous remainder, next remainder, and credited cents.
The next remainder stays below one cent.
Original provider costs, charges, and settlement records remain unchanged.

The reservation estimate rounds up to cents and includes each authorized attempt.
An upper bound for cached tokens does not establish a cache discount.
An unknown component bound or an amount above the ledger limit prevents authorization.

Text admission uses the catalog input bound and output-token limit for each authorized attempt.
The input bound is the smallest fixed input-token or context-token limit.
Input tokens are part of the total context, so both limits apply.
The output bound includes increases that a continuation can make after an empty response.
Each cache quantity uses the input-token limit as its upper bound.
Without a fixed input or context limit, admission fails. Missing output limits also prevent admission.
Provider search requires a fixed call limit, an exact call price, and a protocol that enforces the limit.
An exhausted attempt limit returns HTTP 409 with `usage_journal_conflict` before another provider call.

The calculation tests use the public Go package interface and local catalog fixtures.
The HTTP tests use local SQLite storage and the management API.

### Retained Prices and Charges

The admission transaction stores the selected provider rates, customer rates, markup, component bounds, and attempt limit.
The price snapshot retains its catalog revision and source evidence.
Later catalog or markup changes cannot replace accepted prices.
The rating worker reads retained prices without a current catalog dependency.
Each component retains all active input-token tiers in its `rates` array.
Measured input selects the tier at settlement. The reservation covers every tier within the accepted input bound.
A gap in that range prevents admission. Later price expiry does not invalidate an accepted snapshot.

Text price bindings use the existing native protocol meters and catalog components.
Responses and Chat Completions subtract priced cached input from inclusive input tokens.
Anthropic cache reads and writes remain separate from ordinary input.
Its five-minute and one-hour cache writes use their separate catalog rates.
The `additional_dimension` field identifies a separate native quantity that contributes to the same rate.
The calculation adds only disjoint quantities with the same unit. It rejects quantities already included in each other.
Missing contributions prevent settlement. The original journal quantities remain unchanged.

Google output prices include thinking tokens, which its native usage reports separately from visible output.
The Google bindings add `reasoning_tokens` to `output_tokens` before the output charge calculation.
See the [Google pricing contract](https://ai.google.dev/gemini-api/docs/pricing) and [native usage fields](https://ai.google.dev/api/generate-content#UsageMetadata).
The current Google input price components use the native input total, with priced cached tokens removed once.

Gemini Interactions supports implicit caching without explicit cache resources.
See the [Google cache contract](https://ai.google.dev/gemini-api/docs/caching).
The current Google adapters do not allocate or reference explicit cache resources.
Their snapshots retain `cache_storage` under `excluded_components` instead of inventing a zero storage measurement.
The `zero_dimensions` field requires reported zero tool input until provider-tool pricing is qualified.
Missing or nonzero tool input prevents settlement. These conditions also apply after restart.

OpenAI Responses search uses the native `web_search_calls` quantity and a selected `USD/call` rate.
The catalog must declare a fixed `web_search_calls` limit in `calls`.
The transport reads that accepted bound before each generation and sends it as `max_tool_calls`.
This also applies to continuations. Polling does not start a new generation.
The [OpenAI request contract](https://developers.openai.com/api/reference/cli/resources/responses/methods/create) defines this limit across built-in tools.
The reservation covers the initial input pass and one additional full input pass per authorized tool call.
It includes call charges and the output bound for each authorized attempt.
Missing search evidence prevents settlement. Usage above the accepted bound remains unresolved.
When search is disabled, the snapshot records `web_search_calls` under `excluded_components`.

Search content has a token cost in addition to the tool call cost.
See [OpenAI tool prices](https://developers.openai.com/api/docs/pricing).
Model qualification must verify the native token totals and any fixed-block search charges.
The current search acceptance tests use controlled prices and protocol responses.
Production search rates and model-specific token rules still require qualification.

Media bindings use the native quantity and an explicit catalog conversion rate.
Dictation duration uses seconds with the selected per-second, per-minute, or per-hour rate.
Token prices require native token measurements. The calculation never converts tokens into an estimated duration.
Image prices use separate input and output text and image token quantities.
ElevenLabs `character_cost` and FAL `billable_units` require an explicit `USD/provider_unit` rate.
Provider units have no implicit currency or duration value.
Dictator speech can use measured output duration with a catalog time rate.
Missing measurements prevent settlement. An unsupported component prevents price admission.
Controlled HTTP tests cover dictation, ElevenLabs speech, and Images charges.
Image tests also verify duplicate delivery and usage above the accepted bound.
Media admission constructs bounds from fixed catalog limits for each priced native dimension.
Limit identifiers match native dimensions, such as `audio_seconds`, `input_image_tokens`, or `character_cost`.
Catalog units are `seconds`, `tokens`, and `provider_units` for those examples.
A missing limit, an account-dependent limit, or an incompatible unit prevents admission before provider dispatch.
The media API returns HTTP 422 with `media_operation_unavailable` in these cases.
Billing limits do not replace the capability limits that validate request controls.
The retained snapshot contains all selected bounds and the authorized attempt count.
These tests use fixture prices and limits. They do not qualify production prices or provider limit enforcement.
Publish only verified provider ceilings or enforced request ceilings as billing limits.
A desired spending budget does not establish a usage ceiling.

The journal delivery transaction records one charge per attempt and observation.
The charge, settlement effect, and delivery acknowledgment share one transaction.
A settlement failure rolls back the charge and leaves the observation pending.
The accepted attempt limit prevents another attempt from being prepared or dispatched.

Customer usage credits use separate adjustment records. The original rating and incurred provider cost remain unchanged.
Each adjustment has an account-scoped event identity, a positive exact credit, a reason code, and a creation time.
A repeated event has no additional effect. A changed event with the same identity fails.
The account lock serializes concurrent credits. Total credits cannot exceed the original customer charge.
Only a resolved charge can receive a usage credit.

The adjustment record and its settlement callback share one transaction.
A failed callback rolls back the adjustment. F068 connects this callback to the shared Ledger service.
The command is an internal financial interface. The customer HTTP resources remain read-only.
Paddle funding reversals remain separate F069 operations.

Charge responses retain `customer_charge` as the original charge.
They also expose `customer_adjustments` and `net_customer_charge` after those credits.
An unresolved charge has no net amount. A full credit leaves a zero net amount and retains the provider cost.
HTTP tests verify partial and full credits, event replay, restart recovery, account isolation, settlement failure, and concurrent over-credit rejection.

Each request has a charge summary across all its attempts.
The summary reads the request, attempts, charges, and credits from one database snapshot.
It adds exact amounts without rounding. Customer credits do not change the provider cost or original customer charge.

The `pending` state has no final totals.
It covers active work, unpublished results, and charges that await delivery.
The `unresolved` state has no customer totals. It retains the complete provider cost when that cost is known.
Failed work, uncertain results, missing usage, and unresolved charge policies prevent a final customer total.
The `rated` state reports provider cost, original customer charges, customer credits, and net customer charges.

The account owner can read these resources:

| Method | Resource | Result |
| --- | --- | --- |
| `GET` | `/api/management/billing-accounts/{billing_account_id}/charges` | A page of at most 100 charges. |
| `GET` | `/api/management/billing-accounts/{billing_account_id}/charges/{charge_id}` | One charge with itemized calculations and evidence identifiers. |
| `GET` | `/api/management/billing-accounts/{billing_account_id}/requests/{request_id}/charge-summary` | Exact totals across the request attempts and credits. |
| `GET` | `/api/management/billing-accounts/{billing_account_id}/price-snapshots/{price_snapshot_id}` | The prices and bounds accepted for a request. |

The charge page uses ascending identifiers and a cursor.
The responses omit provider request identifiers, credentials, and request content.
Exact amounts use rational strings. The `reserved_cents` value is also a string.

Only the `rated` state permits a customer charge and a settlement call.
The `usage_unresolved` state retains unknown required quantities without a zero-cost assumption.
The `policy_unresolved` state retains known provider costs when failed work requires a customer policy decision.
The `limit_unresolved` state retains costs above an accepted component or attempt bound.
Unresolved states expose a null `customer_charge` and do not call settlement.
Their `rating` field retains the available calculations under the accepted prices.

Typed price conditions now supply token ranges, cache classes, service tiers, regions, and effective intervals.
F067 constructs native quantity bounds from fixed catalog limits and retains them with accepted prices.
Production qualification must verify each offering's prices, measurements, and enforced limits before hosted activation.
F068 connects the admission and settlement callbacks to shared Ledger transactions.

## Paddle Payment Contract

The following references were verified on 2026-09-22.
Paddle supports prepaid credit purchases with balances maintained by the application.
F069 must use one-time purchases for funding. F068 remains authoritative for spendable funds.
See [Paddle for AI companies](https://developer.paddle.com/get-started/how-paddle-works/ai-companies/).

Paddle acts as merchant of record for customer purchases.
Its financial records supply transaction amounts, tax, fees, and payout evidence.
The platform must retain the actual Paddle account and environment identity.
See [Paddle financial responsibilities](https://www.paddle.com/resources/paddle-heavy-lifting-finance).

Credit a funding order only after verified `transaction.completed` evidence matches its account, amount, and currency.
Bind each transaction to one funding order before crediting the account.
Retain tax, Paddle fees, customer credit, and payout amounts separately.
See [completed transactions](https://developer.paddle.com/webhooks/transactions/transaction-completed/).

Verify `Paddle-Signature` against the raw request bytes before JSON parsing.
Persist verified events before returning successful acceptance.
Deduplicate `event_id` and each transaction effect independently.
Use `occurred_at` and verified entity state when events arrive out of order.
See [signature verification](https://developer.paddle.com/webhooks/about/signature-verification/) and [webhook delivery](https://developer.paddle.com/webhooks/about/how-webhooks-work/).

Refund eligibility must obey the Paddle Buyer Terms and Refund Policy applicable to the purchase.
The customer interface must link to Paddle support and the applicable refund terms.
Do not promise unconditional refunds or reject mandatory refund rights because credits were consumed.
See [Paddle Refund Policy](https://www.paddle.com/legal/refund-policy).

Use Paddle adjustment resources for full refunds, partial refunds, and chargebacks.
Keep `pending_approval`, `approved`, `rejected`, and `reversed` distinct from local funding states.
A pending refund must hold the affected funds against new usage.
An approved adjustment must produce one compensating ledger effect.
A rejected refund must release its unused hold without recording a completed refund.
Chargeback warnings, chargebacks, and their reversals require distinct durable effects without duplicate deductions.
See [transaction adjustments](https://developer.paddle.com/build/transactions/create-transaction-adjustments/) and [adjustment events](https://developer.paddle.com/webhooks/adjustments/adjustment-created/).

Paddle customer credit balances serve transaction adjustments and subscription changes.
They do not replace the platform usage balance.
See [Paddle credit balances](https://developer.paddle.com/build/customers/get-customer-credit-balances/).

Retain financial records when customer access ends. Archive Paddle resources where its API requires retention.
Paddle reports five-year retention for certain transaction data after a buyer deletion request.
Its privacy policy also accounts for applicable legal periods and ongoing disputes.
These sources do not define a universal deletion date for the platform usage journal.
Specify the platform retention schedule and dispute holds before production activation.
See [entity retention](https://developer.paddle.com/api-reference/about/delete-entities/), [buyer deletion](https://www.paddle.com/help/manage/your-customers/requesting-buyer-data-deletion), and [data retention](https://www.paddle.com/legal/privacy).

## Component Ownership

### F065 Resource State

The development API has billing account, platform connection, hosted access grant, and tenant assignment resources.
The browser supports account creation, grant selection, and tenant API access without customer provider credentials.
Hosted execution remains disabled. Financial admission and complete acceptance remain incomplete.

Each grant binds one tenant to its billing account and one platform connection.
The grant names exact models or explicit provider services and their permitted operations.
Each service entry omits `model`. This omission does not grant access to all models.
The API rejects empty, null, blank, or non-string model values.
The current provider catalog controls which operations belong to a model or a service.
Grant creation requires the current catalog revision and an idempotency key.
The retained creation response remains available after catalog changes and grant changes.

Only operators can create grants or change their state.
Each state change requires the current revision and an operator reason.
The grant state and audit record change in one database transaction.
A suspended grant can return to active state. A revoked grant cannot return to another state.
Customers can read only their own grants. Their responses omit platform connection identifiers and operator audit details.
Tenant deletion returns a conflict when the tenant has hosted grant history, including revoked grants.
This conflict preserves tenant identity and its audit records.

The grant does not authorize execution by itself.
Admission must also check credential qualification, pricing, metering, funds, and the hosted activation gate.

Each tenant assignment selects `account_connection` or `hosted_access_grant` with its `resource_id`.
Database constraints reject simultaneous assignments for the same tenant and provider.
The customer must detach the current resource before selecting another resource.
Hosted profiles retain customer model and prompt settings without platform credentials.
The browser shows grant state, permitted offerings, and the saved assignment.
Suspension and revocation do not erase the assignment or its history.

### F066 Journal State

The development API has account-owned request collection and detail resources.
The collection uses a cursor and ascending request identifiers.
The browser client reads these resources through the authenticated management API.
The responses omit platform connection identifiers, worker claims, request content, and private provider identifiers.

Journal admission retains one request for each tenant and idempotency key.
Changed request intent produces a conflict.
Admission selects the active grant and qualified credential version before it records the request.
The reservation operation and request creation use the same database transaction.
A reservation failure rolls back both effects.

Each attempt requires a current worker claim and an active, assigned grant before dispatch.
The journal records dispatch intent before the caller can send provider work.
A second dispatch for the same attempt produces a conflict.
Recovery marks expired, unobserved dispatches as uncertain and creates a reconciliation case.
Recovery does not change requests with live worker claims.

Usage observations retain decimal quantities, source fields, adapter revisions, and inclusion relations.
Missing quantities have an explicit unknown reason. They are not measured zero.
Unknown usage prevents a new continuation attempt.
The observation and its delivery record use the same database transaction.
The accounting effect and completed delivery also use one database transaction.
Database tests verify rollback, repeated delivery, and retained evidence after the database connection closes and reopens.

An admitted text execution records provider attempts through the shared HTTP transport.
Controlled HTTP tests supply the financial authorization operation.
Each continuation and synthesis call has its own attempt.
Provider polling keeps the existing attempt and its private provider identifier.
File staging and cleanup do not create generation attempts.

A lost response leaves an uncertain attempt and a reconciliation case.
Invalid JSON produces unknown usage.
A grant denial during synthesis produces HTTP `403` and prevents another provider call.
These tests do not prove financial admission or live provider qualification.
Financial admission, usage extraction for all operations, execution recovery, and financial settlement remain incomplete.

### Customer Journal Views

The browser dashboard reads hosted request history from the owned billing account.
Each request has separate collections for attempts, usage observations, and reconciliation cases.
The collections use ascending opaque identifiers and cursor pagination with at most 100 records per response.
The browser loads each additional page only when the customer requests it.

The account and request ownership checks apply to every collection read.
Journal GET requests do not change retained evidence. Responses use `Cache-Control: no-store`.
Provider identifiers, source fields, adapter details, and private resolution notes stay private.
Invalid retained evidence produces `usage_journal_unavailable` with HTTP `500`.

Usage values remain exact decimal strings through the API and browser.
Unknown measurements retain their reason and have no numeric value.
The browser shows inclusion relations to distinguish a quantity from its inclusive parent.
Execution completion and complete usage do not establish a final customer charge.

The request children are `attempts`, `observations`, and `reconciliation-cases` under the current account request resource.
The browser shows open and resolved cases through safe identifiers and reasons.
The public case reasons include unknown usage, unknown dispatch outcome, unknown execution outcome, and missing execution result.
The missing-result reason is `execution_result_unknown`. It identifies completed provider work whose result was not durably published.
This case remains visible after recovery and does not authorize another paid execution.

HTTP acceptance verifies account isolation, pagination, private-field omission, exact quantities, and unchanged journal records.
Browser acceptance covers desktop and phone widths, additional pages, failed reads, and rejection of numeric measurement values.
F066 remains open. Hosted execution remains disabled.

### Hosted Text Identity

The development text coordinator uses database admission and the existing text request store.
Native text, MCP, Chat Completions, and Responses use one normalized request intent.
Hosted text requires one tenant-scoped `Idempotency-Key`.
The intent includes resolved messages, media content digests, tools, schema, and generation controls.
The journal keeps the execution identifier, catalog revision, grant revision, and credential version selected at admission.

Only the current journal worker can dispatch the accepted execution.
Concurrent duplicates return HTTP `202` with the original execution identifier.
Identical completed requests return the saved result without provider dispatch.
Changed intent returns HTTP `409`.
Client result identifiers and timestamps use the accepted execution receipt during replay.
Structured JSON remains the validated JSON result.

The existing `GET /v2/requests` resource reads hosted state through the journal.
A saved hosted completion contains `text`, optional `tool_calls`, and optional operational `usage`.
This usage field does not supply financial evidence.
An uncertain journal receipt produces HTTP `409` even when the response store recorded a transport failure.
After result expiry, HTTP `410` reports `hosted_result_expired`. The journal identity remains and prevents another execution.
Grant revocation prevents new work but does not remove a saved result.
Native text and dictation replays use the durable failure envelope for retained HTTP 502 failures.
Client protocol replays use their existing error envelope. Neither replay sends another provider request.

The production router does not install the financial authorization operation yet.
It denies hosted text with HTTP `403` after request validation.
MCP uses the `idempotency_key` tool input for the same tenant-scoped identity.
Concurrent duplicates return the retained execution identifier with `state: dispatched`.
Uncertain outcomes return the retained identifier and an explicit tool error.
Go, Python, and the CLI accept a request key without a structured-output schema.
The clients preserve pending states and recognized hosted error codes.
Media admission now uses the existing operation identity in controlled HTTP tests.
Controlled acceptance covers all 67 active text offerings through their declared provider codecs.
Each route retains exact usage and one execution across replay.
Request timeout and client disconnect after dispatch retain an uncertain journal outcome.
Replay cannot repeat this unresolved provider work.
Image editing acceptance covers both Images and Responses through the existing adapter.
Financial admission and settlement remain with F067 and F068.

### Reported Provider Cost

The xAI Responses meter retains `usage.cost_in_usd_ticks` as `provider_cost`, with the unit `usd_tick`.
The [xAI cost contract](https://docs.x.ai/developers/cost-tracking) defines 10000000000 ticks per USD.
This reported amount includes applicable discounts and provider tool costs.
The journal keeps the exact integer separately from token measurements. It does not round or calculate customer charges.
F067 must distinguish reported provider cost from calculated provider cost and apply the selected customer price schedule.

Missing cost fields remain unknown. Negative, fractional, and nonnumeric values produce `invalid_quantity`.
HTTP tests verify exact large values, zero, missing and invalid costs, and replay without another provider request.
Reported cost remains available after provider rejection or contradictory token measurements.

### Cache Lifetime Evidence

The Anthropic Messages meter retains separate five-minute and one-hour cache writes as exact token quantities.
The dimensions are `cache_write_5m_tokens` and `cache_write_1h_tokens`. Both belong to the inclusive `cache_write_tokens` total.
Cache writes and cache reads remain separate from ordinary input tokens.
The [Anthropic cache contract](https://platform.claude.com/docs/en/build-with-claude/prompt-caching) defines these inclusion rules and response fields.
F067 must select lifetime-specific rates without charging both a subdivision and its inclusive total.

The shared token meter validates inclusive counts and complete partitions before it records usage.
Contradictory counts become unknown quantities with `invalid_quantity`. Numeric source evidence remains available for reconciliation.
Missing lifetime fields remain unknown, including when the provider reports a cache-write total.
HTTP tests verify exact values, zero, missing subdivisions, contradictory sums, private-field omission, and replay without repeated dispatch.

### Google Modality Evidence

Gemini Interactions and Vertex GenerateContent use one shared reader for provider token arrays.
The reader retains exact counts for text, image, audio, video, and document inputs and outputs.
Cache and tool input subdivisions retain their respective inclusive totals.
The [Interactions response contract](https://ai.google.dev/api/interactions-api) defines the lowercase modality names and usage arrays.
The [Vertex modality contract](https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/rest/v1/ModalityTokenCount) defines uppercase names and the default text modality.

Missing or partial arrays remain unknown when their reported total is positive.
A reported zero total does not require a subdivision. The meter does not replace missing measurements with zero.
Duplicate modalities, invalid quantities, and subdivisions above their total produce unknown evidence.
Cache subdivisions cannot exceed the corresponding input subdivision.
Numeric source fields remain available for reconciliation. Private labels and response content do not enter source evidence.
HTTP tests verify both protocols, exact values, zero, malformed arrays, contradictory counts, and replay without repeated dispatch.

### Web Search Evidence

Accepted web search intent enables tool metering for each provider attempt, including polling and continuations.
The [official OpenAI documentation](https://developers.openai.com/api/docs/guides/tools-web-search) distinguishes search actions from page opens and page searches.
The meter records completed search actions as `web_search_calls`, with the unit `call`.
It counts actions rather than query strings. Page opens and page searches do not increase this quantity.
The numeric source path `derived.output.web_search.search_count` identifies a count derived from the provider output list.
The financial evidence excludes tool identifiers, queries, URLs, page contents, and caller tool arguments.

An explicit empty output list establishes zero search actions. Missing output remains unknown.
Incomplete or failed tool calls remain unknown because their charge cannot be established from that response.
Duplicate identifiers and invalid shapes produce `invalid_quantity`. Unknown action types produce `unsupported_meter`.
Polling records the terminal observation only. Each continuation has its own attempt and tool count.
Unknown usage prevents another paid attempt. A failed evidence transaction cannot publish a successful result.
HTTP tests verify these rules, safe failure replay, and one retained accounting delivery per observation.

### Hosted Media Admission

The media operation, journal request, and reservation operation use one database transaction.
The journal retains the operation identifier and the accepted grant and credential version.
Concurrent requests with the same key return one operation and create one reservation.
Changed intent or a key already used for text produces HTTP `409`.

Adapter validation uses the selected credential reference.
The shared admission boundary also supplies the selected hosted authority to the existing validator.
Dictator speech validation verifies voice ownership and credential authority before admission.
Validation does not submit provider work or reserve funds.

Admission checks that reference again under the database writer lock.
Credential rotation or grant revocation during validation prevents admission.
A failed reservation or operation write rolls back all admission records.
An accepted operation remains readable after grant revocation.

Admission tests use the existing image adapter and an injected reservation operation.
These admission tests do not start media workers or move customer funds.
Production media admission remains disabled without the financial operation.
Text startup recovery does not change media journal records.

### Hosted Media Execution

The media worker generation and journal claim change in one transaction.
Claim renewal updates both records together.
Attempt authorization, journal dispatch intent, and media dispatch intent also use one transaction.
The shared credential reader loads the platform version selected at admission.

Provider submission requires the current worker claim and an active grant.
HTTP and gRPC calls use the same dispatch guard and permit one generation per execution attempt.
The generated Dictator SDK uses explicit roles for job submission, status, cancellation, upload, and download.
Unknown SDK methods have no hosted execution authority.
Artifact uploads require the active grant. Artifact streams verify worker authority during transfer.

Status reads and transfers require the current worker claim.
Result and preview publication hold the database writer lock against claim replacement.
An obsolete worker cannot submit provider work or publish an operation result.

The adapter supplies separate recovery data and an explicit provider request identifier.
The operation retains the recovery data. The journal retains only the supplied identifier.
Both records use one transaction before polling starts.
An absent identifier stays absent. Recovery URLs, dictionary content, and adapter metadata do not become journal identifiers.

A replacement worker recovers the accepted provider job without another submission or reservation.
Grant revocation prevents new provider work but permits recovery through the accepted credential version.
Unknown provider outcomes remain uncertain and require reconciliation.

Provider observations and their delivery records use one transaction.
The terminal operation and journal outcome use one transaction.
Unavailable usage extraction produces `unsupported_meter` evidence and unknown usage.
It does not produce a measured zero or a settled customer charge.
Confirmed cancellation records journal state `failed` with failure code `operation_cancelled`.
F068 remains responsible for the financial effect of cancellation.

Controlled HTTP tests exercise image submission, result publication, rotation, revocation, worker replacement, provider recovery, and cancellation.
These tests do not prove live provider qualification or usage extraction for every operation.

Hosted Dictator speech tests use a retained tenant-owned voice and the existing gRPC adapter.
They verify voice isolation, accepted credential versions, admission races, and retained request identity.
Hosted gRPC speech tests cover dispatch fencing, recovery, artifact transfer, and cancellation.
Recovery cannot submit another job, even when the grant remains active.
Cancellation requires a current cancellation request and can proceed after grant revocation.
Hosted voice discovery reuses the existing HTTP and gRPC adapters with current grant and credential checks.
Voice pages and previews preserve tenant isolation without creating billable attempts.
Hosted ElevenLabs discovery exposes default premade voices. Hosted Dictator discovery includes tenant-owned extracted voices.
Metadata permission cannot submit provider work. Obsolete grant and credential versions invalidate hosted page cursors.
See [Provider Voices](provider-voices.md) for the resource and privacy contract.
The dictation section records current acceptance and its remaining boundaries.
The remaining operations still require complete hosted acceptance.

### Hosted Provider Services

Hosted admission uses the existing model-free media operation contract.
The journal omits `model` for these service requests.
Grant matching requires the exact provider and service operation.
A model grant does not authorize a model-free service.
The browser displays these grant entries as `Provider services` with their selected operations.

Controlled HTTP tests verify dictionary creation and audio alignment through existing adapters and accepted platform credentials.
Admission and dispatch each require financial authorization through the shared transaction callback.
Duplicate requests retain one operation and one provider submission.
Changed intent conflicts. Grant revocation rejects new work and preserves accepted results.
Missing service meters retain unknown usage.
Complete service metering and live provider qualification remain open.

### Media Worker Failure Reports

The configured service logger records media worker persistence failures.
The worker preserves the database cause when claim, dispatch, or final result writes fail.
Adapter callbacks report usage, provider handle, preview, and claim renewal failures before returning the error.
Output, voice, and dictionary write failures also produce reports.
Reports add operation context without request content, credentials, or provider response bodies.

| Log event | Context fields |
| --- | --- |
| `media operation worker failed` | `worker_id`, `operation_id`, `error` |
| `media operation persistence failed` | `operation_id`, `phase`, `error` |
| `media operation maintenance failed` | `operation_id`, `phase`, `error` |

These reports do not authorize another provider submission.
The retained operation, claim, and journal state control recovery.
HTTP tests verify the reports, caller-visible states, and duplicate request identity.
Maintenance reports identify queue selection, recovery scans, usage scans, retention scans, usage delivery, and terminal deletion.
Scan reports have an empty operation identifier because the failed query selects a batch.
Transaction failures preserve the operation for the next maintenance pass.
HTTP tests verify rollback, repeated maintenance, and financial journal retention after operational deletion.

### Speech Usage Evidence

The existing ElevenLabs generation and conversion adapters share one cost reader before audio validation or publication.
The [ElevenLabs API introduction](https://elevenlabs.io/docs/api-reference/introduction#tracking-generation-costs) documents the `character-cost` and `request-id` headers.
The journal retains the exact decimal value as `character_cost`, with the unit `provider_unit`.
The numeric source path is `headers.character_cost`. The provider request identifier remains private.
This evidence does not establish a currency amount or a conversion to text length.
F067 must select a verified price mapping before it can calculate a charge.

Missing headers produce unknown usage. Invalid or duplicate values produce `invalid_quantity`.
Reported zero remains measured zero. Provider failures and invalid audio do not erase reported usage.
Observation and delivery writes commit together. Failed writes prevent audio publication and return `usage_journal_unavailable`.
HTTP tests cover all current generation and conversion offerings, including multipart audio and timestamped generation responses.
Tests also verify exact decimals, failures, duplicate requests, transaction rollback, and repeated worker execution.

### Dictator Duration Evidence

The hosted gRPC boundary records terminal synthesis duration before the existing Dictator adapter transfers artifacts.
The journal retains `output_audio_seconds`, with the unit `second`, from `audio_duration_seconds` in the native synthesis job response.
The decimal representation restores the same value when read as a native double.
Negative and nonfinite values produce `invalid_quantity`.
The protobuf field has no presence marker. Its zero default remains unknown and does not establish measured zero usage.

Queued and running job responses do not create duration observations.
Succeeded, failed, and canceled jobs retain reported positive duration.
Artifact transfer failures do not erase this evidence. Journal write failures prevent artifact transfer and publication.
HTTP tests use the existing adapter and a real gRPC server with controlled native responses.
The tests verify duration precision, unknown values, terminal states, transaction rollback, artifact loss, and request replay.

Transcription, diarization, alignment, subtitle creation, and voice extraction retain unknown `input_audio_seconds` with `unsupported_meter`.
The native job responses do not report processed input duration.
Upload metadata, word timestamps, and job timestamps do not establish this quantity.
Voice extraction also retains `output_audio_seconds` from `sample_artifact.audio_metadata.duration_seconds`.
This output measurement does not resolve the unknown input quantity.
The same duration rules apply to synthesis and extracted samples.

The gRPC boundary records terminal evidence before result parsing, artifact transfer, or voice publication.
Controlled HTTP tests cover all current Dictator audio input offerings.
These tests verify explicit unknown usage, reported sample duration, terminal states, invalid results, failed writes, and replay.
No duration measurement establishes a provider price or customer charge.

### FAL Queue Usage Evidence

The existing queue image adapter records `X-Fal-Billable-Units` before result decoding or artifact download.
The [FAL header contract](https://fal.ai/docs/documentation/model-apis/common-parameters#response-headers) defines these charged units.
The shared decimal header reader also serves the ElevenLabs adapters.
The journal retains `billable_units` with the unit `provider_unit` and the numeric source path `headers.x_fal_billable_units`.
F067 must bind these units to the selected endpoint price. Output count does not establish billed units.

Failed submissions, terminal status failures, and result failures retain reported units.
Missing, invalid, and duplicate headers produce unknown evidence.
Journal write failures prevent artifact download and publication.
Recovery reads the original queue job under its retained authority. It does not submit another generation request.
HTTP tests verify exact decimals, zero, unknown units, failure evidence, transaction rollback, and recovery after grant revocation.

### Image Usage Evidence

The existing Images API adapter records usage before image validation or asset publication.
JSON responses and completed stream events use the same journal boundary.
Preview events do not create billable observations.
The journal retains exact integer token counts, numeric source fields, and inclusion relations.
Text and image subdivisions remain part of their input or output total.
Contradictory totals produce invalid measurements while the journal retains the numeric source evidence.

The [OpenAI Images API reference](https://developers.openai.com/api/reference/resources/images/methods/generate) defines aggregate tokens and text and image subdivisions.
It does not establish separate cached text and image measurements.
The image meter records these cache quantities as `unsupported_meter`.
Absent documented fields remain `not_reported`. Invalid fields remain `invalid_quantity`.
Known counts do not establish complete billable usage when required cache measurements remain unknown.

An evidence write failure prevents result publication and retains an uncertain operation.
The operation exposes `usage_journal_unavailable` through the existing error representation.
Duplicate requests retain this operation without another provider submission.
An obsolete worker cannot record new observations or publish its delayed result.
Controlled HTTP tests cover these boundaries with local provider responses.
Live image meter qualification remains open.

The Responses image adapter retains terminal usage from JSON, polling, and stream responses before image decoding and publication.
The meter uses `responses_` quantity names for the response model and separate quantities for its image tool.
Response totals include input and output tokens. Cache counters remain part of input tokens, and reasoning remains part of output tokens.
The journal keeps exact reported values, including zero, without converting them to floating-point numbers.

The [OpenAI Responses reference](https://developers.openai.com/api/reference/resources/responses/methods/create) defines response usage but no separate image tool token counters.
Image tool quantities remain `unsupported_meter` until qualified provider evidence supplies them.
The adapter does not interpret undeclared tool fields as measured usage.
Partial and queued responses do not produce usage observations.
Failed, incomplete, and cancelled responses retain reported quantities without establishing charge eligibility.

Controlled HTTP tests verify retained quantities, separate cost layers, missing usage, inconsistent totals, replay, and evidence write failures.
Complete image cost measurement and live Responses qualification remain open.

### Hosted Dictation

Hosted dictation reuses the completion admission, pinned credentials, response store, and recovery code used by hosted text.
The existing provider adapters continue to own transcription requests and response parsing.
Both `/dictate` and `/v1/audio/transcriptions` require one tenant-scoped `Idempotency-Key` for hosted work.
The request intent includes the audio digest, byte count, file extension, provider, model, and operation.
The journal does not retain the audio or transcript as financial evidence.

Identical requests share one execution across both interfaces and service instances.
Concurrent requests return HTTP `202`. Changed audio returns HTTP `409`.
A revoked grant prevents new work but preserves replay of an accepted result.
Expired results return HTTP `410` without another provider dispatch.
`GET /v2/requests` reads the saved transcript through the existing completion representation.

The [OpenAI transcription reference](https://developers.openai.com/api/reference/resources/audio/subresources/transcriptions/methods/create) defines duration and token usage variants.
The multipart meter retains `usage.seconds` as exact decimal seconds when `usage.type` is `duration`.
The token variant retains totals and audio and text subdivisions from `input_token_details`.
Invalid or contradictory measurements remain unknown. Missing usage never becomes measured zero.
The Meta protocol has no qualified duration meter. Its duration remains `unsupported_meter`.
Gemini and Vertex dictation use their existing response token meters.

Controlled HTTP tests verify cross-interface replay, concurrent requests, restart identity, revocation, expiry, and exact duration and token evidence.
Lost responses and evidence write failures retain uncertain requests and prevent another paid call.
Catalog-driven tests exercise every current dictation offering through its existing provider adapter and both HTTP interfaces.
Process-interruption tests verify all five completion boundaries for text and dictation.
Live meter qualification remains open.

Provider file requests use the accepted credential version and current worker claim.
Each upload request also requires the active grant. Grant revocation stops further file transmission and generation.
The current worker can still read or delete an accepted provider file after grant revocation.
An expired or replaced worker cannot send provider file requests or poll provider completion status.
File staging and cleanup do not create model generation observations.

Controlled upload tests verify complete execution, revocation during staging, claim expiry, cleanup failure, and request replay.
A replacement worker can resume before generation dispatch while preserving the accepted execution identity.
Cleanup failure prevents transcript publication and preserves measured generation usage.

### Completion Recovery

An expired worker claim permits another worker to take accepted work only before the first dispatch.
The new worker keeps the accepted request, execution, attempt, catalog revision, and credential version.
This transition does not create another initial funds reservation.
An obsolete worker cannot dispatch or change the response store.

An interrupted dispatch remains uncertain when the provider outcome is unknown.
An interruption after observation retains the measured usage and an uncertain execution outcome.
Neither state permits another paid call without reconciliation.
The journal keeps a reconciliation case for each unresolved outcome.

Response publication holds the database writer lock against worker replacement and startup recovery.
The response file and its database publication receipt remain separate durable effects.
After an interruption, recovery can repair a missing receipt from the saved result.
When no result exists, recovery records an uncertain result and preserves the known usage.
Response expiry cannot remove a saved result before its publication receipt exists.

Startup recovery checks expired completed requests that lack a publication receipt.
An identical POST also checks an expired completed request after a result write failure.
Status GET requests do not change the journal.
Process interruption tests verify these boundaries through authenticated HTTP requests and a controlled provider.
These tests do not prove funds settlement, provider reconciliation, or recovery for every operation.

### Funds Admission and Settlement

F068 uses the shared Ledger service in the managed database transaction.
Admission locks the billing account before it reads funds or creates a reservation.
The transaction retains the request, accepted price, and reservation together.
An unsuccessful transaction retains none of these effects.
All tenants of the account use the same Ledger balance.

The reservation covers the accepted maximum cost, rounded up to cents.
It has no automatic expiry.
An identical request reuses its reservation and execution identity.
Insufficient funds produce `402` before provider dispatch.
A suspended financial account produces `403`.
A financial admission failure produces `503`.
Media admission uses these same errors before it creates queued work.

Usage delivery retains each exact charge before settlement.
Settlement waits for request completion and a resolved charge for every attempt.
The transaction releases the hold, posts the charge in cents, and retains the exact account remainder.
It also retains a settlement record and acknowledges usage delivery.
A failed write rolls back all these effects and leaves delivery pending.
A repeated delivery cannot post another charge or release.

Unknown usage, unresolved charge policy, and usage above the accepted bound retain the hold for reconciliation.
The account API exposes the reason through the request's reconciliation cases.
Controlled acceptance uses local HTTP providers and reopened database connections.
Production activation remains disabled.
Decisions for uncertain costs and complete service acceptance remain open under F068.

An audited usage credit uses the existing charge adjustment transaction and its account lock.
A compensating Ledger grant restores whole cents when required.
This supports credits for several fractional charges that shared one settled cent.
A separate credit record links the Ledger effect to the original adjustment and request.
Repeated adjustment identifiers cannot change funds twice.
Before settlement, a credit reduces the pending charge. The settlement record retains the identifiers of those credits.

Controlled tests verify fractional credits, concurrent adjustments, duplicate events, and rollback after a failed financial write.
The public calculation tests verify that exact credits reverse cent settlements, including the maximum Ledger amount.
F069 owns payment refunds and funding reversals through Paddle.

Hosted completion startup reconciles retained funds after journal and result recovery.
It delivers pending funded observations and reviews held reservations in bounded batches.
Each financial effect has one transaction. A later run resumes pending records without a process-local checkpoint.

The HTTP service reconciles funds before it accepts requests and then once per second.
Its lifecycle owns the financial worker and HTTP listener.
A reconciliation failure stops the listener and returns the financial error to the process owner.
An interrupt or termination signal cancels the worker and active requests.
HTTP shutdown has a ten-second limit.

Settlement requires a published result and final charges for all attempts.
A completed request with no publication receipt retains its reservation.
A later reconciliation pass can settle the request after publication.

A failed request with no dispatch evidence releases its reservation.
For an expired undispatched request, recovery closes the journal request before it releases funds.
This transition prevents a stale worker from dispatching with released funds.
Recovery leaves active worker claims unchanged.
Dispatched work with an uncertain outcome retains its hold and requires reconciliation.

A failed recovery write stops hosted startup and leaves the financial effect pending.
Controlled tests interrupt real service processes before dispatch, after dispatch, and during settlement.
The settlement interruption occurs after Ledger release and spend operations, before the caller commits the transaction.
Restart tests verify rollback, retained holds, exact remainders, and one financial effect per accepted request.
Two concurrent service processes share one funded balance and reject excess reservations before provider work.
Two running service processes also settle new completed work through the shared database without restart.
Runtime tests verify failed settlement rollback, listener closure, restart recovery, and request cancellation during shutdown.

### Platform Exposure

Reservation details include `known_provider_cost`, `known_platform_exposure`, and `provider_cost_complete`.
Known costs include only attempts with complete retained pricing results.
An incomplete cost response does not establish a zero total cost.
Known exposure is the positive difference between known provider costs and the exact request authorization.
It is a lower bound when provider costs remain incomplete.

Usage delivery retains positive exposure with a `platform_exposure` reconciliation case in the same financial transaction.
A failed write leaves usage delivery pending and keeps the hold.
Repeated delivery and recovery retain one exposure record and case for the request.
Customer credits and financial waivers do not erase provider costs or exposure.
The browser journal shows the exposure case. Reservation details expose the exact amounts to the owner and operator.

### Audited Financial Resolution

An operator can resolve a reservation in `reconciliation_required` after the request reaches a final execution state.
The decision records a final net customer charge, reason, evidence reference, operator identity, and time.
The charge cannot exceed the exact authorized maximum, even when the rounded hold permits a larger amount.
When all provider costs are known, the accepted customer pricing can reduce this upper limit.
Prior customer credits also reduce the limit.
Reservation details expose the resulting `resolution_charge_limit`.
A zero charge waives the request charge and releases the hold.
No default failure-charge policy applies through this command.

The financial transaction commits the decision, settlement receipt, Ledger effects, account remainder, and tenant total together.
A failed audit write rolls back all these effects.
An identical repeat returns the retained decision. A different decision for the same request produces `409`.
Later credits cannot exceed the amount actually settled for that request.

Original usage, provider costs, and charges remain unchanged.
The financial decision does not establish missing provider evidence or close its usage cases.
The account owner can read the financial receipt. Private operator identity and evidence references remain in the audit record.

Use this procedure for an approved decision:

1. Read `GET /api/management/billing-accounts/{billing_account_id}/reservations/{request_id}` with an operator session.
2. Review the reservation state, current revision, exact authorization, and retained financial evidence.
3. Prepare the final net USD amount as `customer_charge`, including prior credits.
4. Set `reason` to the approved reason code and `evidence_reference` to the retained review identifier.
5. Submit these fields and the reservation `revision` to `PUT /api/management/billing-accounts/{billing_account_id}/requests/{request_id}/funds-resolution`.
6. If the response is `409`, review the current reservation and decision before another submission.
7. Read the same resolution resource with `GET` to verify the retained receipt.

The command rejects customer sessions and active execution.
It does not create a funding credit or change the payment processor records.

An operator can credit a settled request through `PUT /api/management/billing-accounts/{billing_account_id}/requests/{request_id}/funds-credits/{credit_id}`.
This resource also supports a settled decision with unresolved provider usage.
The request body contains a positive exact `credit`, a `reason`, and an `evidence_reference`.
The account owner or an operator can read the receipt with `GET` at the same resource.
The audit record retains the operator identity and evidence reference. Customer responses omit these private fields.

Each request credit has one immutable identifier. A changed repeat produces `409`.
The account lock serializes request credits and usage credits against the same settled amount.
The transaction retains the credit, Ledger effect, exact account remainder, and tenant total together.
The original settlement, financial decision, charges, and provider evidence remain unchanged.
These credits correct customer usage charges. F069 owns Paddle payment refunds and funding reversals.

### Financial Backup and Restore

`make snapshot-managed-database` copies the complete managed SQLite database through the SQLite snapshot API.
The copy includes journal records, price snapshots, charges, Ledger entries, reservations, credits, tenant totals, financial decisions, exposure records, and payment inbox records.
It also includes all other tables in that database. The command does not select a subset of financial records.
Committed WAL data forms part of the snapshot. Uncommitted writes do not.

The command opens the source for reads only and checks the copied database for integrity and foreign key errors.
It publishes the complete image at a new destination and refuses to replace an existing path.
The JSON receipt contains the source, destination, byte count, SHA-256, and completion time.
A failed check returns a nonzero exit status without publishing the invalid image.

Use the configured `management.database_path` as the source:

```bash
SNAPSHOT_SOURCE=/srv/llm-proxy/managed.db \
SNAPSHOT_DESTINATION=/srv/backups/llm-proxy-2026-09-23.db \
make --silent snapshot-managed-database
```

Create the destination directory before this command. Use a new path for each backup.
Retain the JSON receipt with the backup and verify its SHA-256 before restoration.
The database snapshot is consistent while the service runs.

For a complete application backup, stop all service instances before the database and asset copies.
Retain the matching `server.asset_store_path` directory, including stored request results.
Retain the configuration and credential encryption key through the existing credential storage procedure.
The database command does not copy these external files.

Use this restore procedure:

1. Stop all instances that use the affected database.
2. Verify the backup against its retained SHA-256 receipt.
3. Copy the backup to a new database path with the same snapshot command.

```bash
SNAPSHOT_SOURCE=/srv/backups/llm-proxy-2026-09-23.db \
SNAPSHOT_DESTINATION=/srv/llm-proxy/restored-2026-09-23.db \
make --silent snapshot-managed-database
```

4. Set `management.database_path` to the restored path and restore the matching asset directory and credential configuration.
5. Start one instance and verify the schema and financial recovery results before more instances start.
6. Compare account balances, reservations, charges, usage evidence, and financial decisions with the retained backup records.
7. Reconcile external payments and provider work after the backup time before hosted admission resumes.

An older backup cannot contain financial effects that occurred after its snapshot.
Do not copy only the main file of an active WAL database or replace individual financial tables.
Do not start old and restored instances against different financial copies for the same customers.

`make test-managed-database-snapshot` verifies the CLI and restores financial fixtures through the management HTTP API.
The test retains an uncertain hold and compares financial responses after two recovery runs.
Restore acceptance includes verified processor events, their private bodies, and replay after restart.
F069 acceptance must also add payment receipts and financial processor effects to this fixture.

### Tenant Spending Limits

An account owner can set an optional USD limit for each tenant.
The default has no tenant limit. The account balance still limits all paid work.
The tenant limit covers total net charges and active reservations. It does not reset automatically.
Credits reduce net charges. A lower limit does not cancel existing reservations.

Admission checks the tenant limit under the same account lock as the Ledger reservation.
Settlement and usage credits update exact tenant totals in their financial transactions.
Fractional charges reduce the remaining allowance before they form a whole cent.
The tenant total does not replace the shared Ledger balance.
Free requests with a zero maximum remain available when the allowance is zero.

`GET /api/management/billing-accounts/{billing_account_id}/tenant-limits/{tenant_id}` reads the limit and exact net usage.
The read creates no financial records and prohibits caching.
`PUT` sets `limit_cents` to a nonnegative decimal string, or `null` to remove the limit.
The body requires the current `revision`. A conflicting revision produces `409`.
A repeat of the last identical change returns the same result.
Both methods require ownership of the account and tenant.

The balance panel shows the selected tenant's limit and remaining allowance.
An account owner can save or remove the limit in this panel.
After a revision conflict, the panel shows the current value for review before another change.

### Account Balance Resource

`GET /api/management/billing-accounts/{billing_account_id}/balance` requires the account owner's management session.
The response uses one database snapshot and prohibits caching.
The read does not create a Ledger account or financial records.
An unfunded account returns zero amounts.

All cent amounts use decimal strings to preserve integer precision in browsers.
`posted_cents` contains net posted credits and debits.
`reserved_cents` contains all active Ledger holds.

`available_cents` equals posted cents minus reserved cents.
`spent_cents` contains cumulative settled usage debits before compensating credits.
`pending_cents` identifies reservations that require financial reconciliation and forms part of reserved cents.

`unsettled_fraction` retains an exact USD charge below one cent for later settlement.
The account state identifies active, suspended, or reconciliation-required financial access.
Financial read failures return an error without partial balance data.

The account resource also provides `GET /reservations` and `GET /ledger-entries` as child collections.
Both collections use ascending identifiers, bounded pages, and `next_cursor`.
Entries with the same timestamp remain distinct across pages.
New records can appear before an existing cursor. Start a new read to obtain those records.

A reservation uses its request identifier and retains its currency, maximum cents, state, revision, and timestamps.
Ledger responses retain entry identifiers, types, signed cent amounts, timestamps, and reservation or refund references.
Private metadata and idempotency keys remain outside these responses.

The existing dashboard shows available, reserved, spent, pending, and posted amounts for its billing account.
The browser formats integer cents without conversion to floating point.
It shows the USD 5 funding minimum and the zero balance floor.
Financial history provides reservation and Ledger pages with separate continuation controls.

A failed balance refresh removes prior financial values and provides an error with a retry control.
Malformed financial responses do not produce displayed balances.
The usage journal accepts financial reconciliation reasons for unresolved usage, policy, and authorized limits.
Browser acceptance covers desktop and narrow widths with the real local management stack and controlled financial responses.

### Funding Orders

The account owner can read `funding-offers` and create `funding-orders` under the billing account resource.
The creation request contains one `offer_code` and an `Idempotency-Key` header.
The server selects the Paddle price and USD funding amount from its configured offers.
Each offer requires at least 500 cents. The browser cannot supply an amount, currency, account identity, or return URL.

Order creation retains the offer, price, amount, supplier, processor account, and environment in one immutable snapshot.
The order and its checkout delivery intent commit in one database transaction.
A failed delivery write rolls back the order. Creation does not change the Ledger balance.
The account and creation key identify one order across service instances and restarts.
A repeated request retains the original snapshot when the offer changes or leaves the current catalog.
A changed offer code, supplier, processor account, or environment returns `409` for that key.

The owner can read one order or its paginated order history.
The API orders history by ascending identifier and returns `next_cursor` for the next page.
Order responses exclude private processor configuration and the creation key digest.
Existing order reads remain available when new funding is disabled.

Controlled HTTP tests configure funding offers explicitly. The normal runtime uses the optional `payments` configuration.
The snapshot fixture verifies restoration and replay of orders, customer associations, checkout delivery, payment receipts, and Ledger credits.

### Checkout Delivery

The delivery worker uses the shared Paddle commerce client from `utils/billing` v0.19.0.
A durable customer association binds the processor customer to one billing account and environment.
The worker records its dispatch intent before it creates the processor transaction.
It verifies the returned transaction against the retained order, customer, price, quantity, currency, and server metadata.
The owner can read the verified transaction through the order's `checkout` resource.
Checkout creation does not grant funds.

A lost response or receipt write leaves the outcome unresolved.
Recovery reads processor transactions for the retained customer and order reference.
The worker requires one matching transaction before it completes delivery.
It does not repeat transaction creation after an uncertain dispatch.
Concurrent workers use durable claims and reject obsolete claim tokens.

### Completed Payment Credits

The processor worker reads signed events from the durable inbox.
A funding credit requires a completed event and the current processor transaction for the retained checkout.
Both sources must match the order's customer, price, quantity, amount, currency, and server metadata.
Captured payment amounts must explain the transaction total.
Conflicting, unavailable, or unmatched evidence remains unresolved with a reason and retry time.

The receipt, Ledger credit, paid order state, and applied event state commit in one database transaction.
The order identifies the financial effect independently of the event identifier.
Duplicate events and concurrent workers produce one receipt and one credit.
A failed receipt, order, or event write rolls back the Ledger credit.
Restart recovery retries the retained event.
A delayed payment collection event cannot repeat a credit or change a paid order to pending.

The receipt retains gross payment, tax, processor fees, earnings, customer credit, and payout currency separately.
Unknown processor fees remain null. Payout amounts retain their own currency.
Controlled fixtures use the retained offer amount as the customer credit and keep processor fees separate.
These fixtures do not decide the production fee allocation or tax policy.
Owned receipt resources expose customer amounts without private processor evidence.

### Payment State Evidence

The worker verifies lifecycle events against the current processor transaction and the retained checkout.
A verified canceled transaction changes an unpaid order to `failed` without a credit.
A failed payment attempt leaves the order `pending` so the customer can retry checkout.
A completed transaction without its completed event remains unresolved with `completion_event_required`.
Lifecycle events cannot reverse a retained receipt. Paddle adjustments control financial reversals.

Each accepted processor observation retains its timestamp, complete evidence, and digest.
The observation, order state, and event state commit together under the account writer lock.
Funding credits and adjustments use the same observation check within their financial transaction.
An older processor snapshot cannot replace a newer retained observation.
Different evidence at the same processor timestamp requires reconciliation.
The processor snapshot must not precede the signed event that requires its verification.

Controlled HTTP tests cover cancellation, retryable failure, reordered events, concurrent workers, and failed writes.
The backup fixture restores observations with payment receipts and Ledger effects.
The browser shows a failed payment without a change to the existing balance.

### Receipts And Processor Portal

`GET /api/management/billing-accounts/{billing_account_id}/funding-orders/{order_id}/receipt` reads one verified payment receipt.
The response includes original and adjusted gross amounts, taxes, customer credit, credit reversals, and pending refund allocations.
It also includes the invoice number, payment time, environment, and current order state.
The database supplies one consistent snapshot. An unpaid order has no receipt.
Private processor fees, payout evidence, customer identifiers, and event bodies do not enter this response.

`POST /api/management/billing-accounts/{billing_account_id}/payment-portal-sessions` accepts an empty JSON object.
The server selects the owned processor customer in its configured environment and returns a temporary Paddle URL.
The response uses `201`, `Location`, and `Cache-Control: no-store`.
Each request creates a new session without a financial effect.
The service does not store or cache the URL.
The [Paddle customer portal](https://developer.paddle.com/api-reference/customer-portals/create-customer-portal-session/) supplies transaction history and invoice downloads.
The browser must open this URL directly, without an iframe.

### Browser Payment History

The billing dashboard shows funding orders and verified receipts for the owned account.
The customer can refresh payment records and request each additional page.
Each order shows its state, credit amount, environment, identifier, and creation time.
Pending orders have no receipt. A browser return URL cannot grant funds.

The receipt shows original and current payment amounts, taxes, customer credit, reversals, and pending refund allocations.
The browser keeps cent amounts as decimal strings and formats them with integer arithmetic.
It rejects invalid amounts, currencies, and receipt identities before display.
A failed refresh removes the affected records and shows a retry message.
Financial history accepts opaque Ledger reservation identifiers, including payment refund holds.

The invoice button creates a temporary Paddle portal session and opens it in a separate tab.
The browser does not retain the session URL. A failed session request closes the empty tab.
Disconnecting the view cancels its pending requests and closes an unfinished portal tab.

`tests/blackbox/hosted-payments.spec.js` runs the normal CLI, authentication service, SQLite database, and browser.
A controlled Paddle HTTP protocol supplies transactions, signed events, adjustments, and portal sessions.
The test verifies delayed funding, receipts, pending holds, partial refunds, pagination, failure recovery, and desktop and narrow widths.
These checks do not qualify a live Paddle environment.
Full reconciliation remains open under F069.

### Browser Checkout

The browser adapts the PoodleScanner transaction checkout approach for one-time funding.
The owned `funding-offers` response supplies the provider, environment, public client token, and available credit amounts.
The response excludes the API key and webhook secret.
The customer selects a server offer. The browser sends its code with a creation key.

The browser retains the creation key and order reference in account-specific session storage.
A lost response retains the same key for retry after reload.
After order creation, the browser reads that order until checkout delivery finishes.
The browser can also resume a retained order from funding history.
These reads do not create another processor transaction.

The adapter loads Paddle.js from its official CDN and initializes it once per page.
A token or environment change requires a page reload.
It opens the verified server transaction with customer changes and discount entry disabled.
See [Paddle transaction checkout](https://developer.paddle.com/build/transactions/pass-transaction-checkout/).

Paddle checkout events must match the active transaction identifier.
A completion event starts server status reads. It does not grant funds.
Verified server state refreshes the account balance and payment history.
A closed checkout retains the order for later use.
Automatic status reads stop after 30 attempts. The customer can request another status check.
Disconnecting the component cancels its requests and timers and closes its active checkout.

The funding view links to the Paddle Buyer Terms, Refund Policy, and buyer support.
The controlled browser tests cover response loss, reload, duplicate events, delayed confirmation, SDK failure, and checkout closure.
The tests also verify recovery from funding history without a retained browser intent.
Actual Paddle connectivity remains a separate qualification step.

### Payment Runtime Configuration

The optional `payments` object in the server configuration enables payment processing.
Omit this object to disable checkout creation, portal sessions, and the webhook route.
Owned historical records remain readable when payment processing is disabled.
The tracked server configuration omits this object. Production payments remain disabled.

The following configuration shows the field contract. It does not authorize live payment qualification.

```yaml
payments:
  environment: sandbox
  client_token: "${PADDLE_CLIENT_TOKEN}"
  processor_account_id: selected-paddle-account
  supplier_id: selected-platform-supplier
  api_key: "${PADDLE_API_KEY}"
  webhook_secret: "${PADDLE_WEBHOOK_SECRET}"
  offers:
    - code: five
      price_id: pri_00000000000000000000000000
      funding_cents: 500
```

The normal CLI reads configured values through its existing environment interpolation.
The required public client token starts with `test_` for sandbox or `live_` for production.
The server rejects a token from the wrong environment before database access.
Create this token in the selected Paddle account under Developer tools, Authentication.
The [Paddle.js initialization contract](https://developer.paddle.com/paddle-js/methods/paddle-initialize/) uses this token instead of the API key.
Configure and obtain approval for the default payment link before actual Paddle qualification.
The [transaction checkout prerequisites](https://developer.paddle.com/build/transactions/pass-transaction-checkout/) define this processor setup requirement.
The shared client selects the Paddle API origin for the configured environment.
`api_base_url` can select an explicit HTTPS origin. A sandbox loopback protocol can use HTTP.
Offer amounts must be at least 500 cents. The server rejects incomplete processor identities and secrets at startup.
The selected price must satisfy the retained funding amount and currency before the service credits funds.

The normal service runs checkout delivery and event processing before HTTP startup and once per second during operation.
The worker retains uncertain processor outcomes with a reason and retry time.
A financial database failure stops HTTP admission. A restart retries retained work without another funding effect.
Shutdown cancels active processor requests through the shared client.
The payment database binds to one environment. A different environment requires a separate database.

### Payment Adjustments

F069 reads current transaction totals and all transaction adjustments through the shared Paddle client in utils v0.19.0.
Each adjustment must match the retained transaction, customer, currency, and funding item.
The worker checks approved amounts against the current adjusted totals before a financial change.
Separate [Paddle adjustment records](https://developer.paddle.com/api-reference/adjustments/list-adjustments/) retain refund status, tax, fees, and payout evidence.

Pending refunds reserve affected funds through Ledger. Rejected refunds release the hold.
Approved refunds produce cumulative compensating entries. Replayed events cannot repeat the same deduction.
Chargeback warnings and chargebacks cannot deduct more than the original customer credit for one order.
Reversed original adjustments and separate reversal records cannot restore the same funds twice.
The worker retains exact evidence and each applied revision in the same transaction as the Ledger effects.
Funding credits and current adjustments commit together, including refunds that precede the initial funding event.

Development fixtures allocate customer credit in proportion to the original payment subtotal.
The cumulative debit rounds down to whole cents. Pending holds round up to whole cents.
Each revision retains the exact fraction. Full reversal removes the complete original credit.
Taxes and processor fees remain separate from the customer debit.
This development calculation does not select the production tax or fee policy.

A mandatory reversal can produce a negative account balance.
The service rejects new hosted work when available funds are negative or a pending refund lacks its complete hold.
This payment restriction does not replace an operator suspension.
New funding and admission checks use available funds to complete retained refund holds.
Controlled tests cover refund approval, rejection, concurrent processing, chargeback replay, reversal, and failed financial writes.
Full reconciliation and processor sandbox qualification remain open under F069.

### Payment Event Inbox

F069 adds `POST /api/payments/paddle/events` through the shared Paddle signature verifier from `github.com/tyemirov/utils/billing`.
The package resolves from `@latest` to v0.19.0.
The receiver verifies `Paddle-Signature` against the raw body before JSON parsing.
The configured secret binds each receiver to one processor account and environment.
The receiver rejects duplicate signature headers and bodies above one MiB.

The inbox retains the verified body, event identifier, entity identifier, event time, receipt time, and processor identity.
These records are private financial evidence. They are not customer response fields or log messages.
The database key includes the environment, processor account, and event identifier.
An identical event has one record across service instances and restarts.
Notification identifiers, JSON property order, and equivalent timestamp offsets do not change event identity.
Different content for the same event produces `409`.

The receiver returns `200` only after the inbox transaction commits. A database failure produces `503`.
Inbox acceptance does not grant funds or establish that an event matches a funding order.
The event starts as `pending` for the payment processor worker.
Paddle distinguishes completed transaction processing from initial payment collection.
F069 funding requires verified `transaction.completed` evidence and a matching order.
See [Paddle transaction completion](https://developer.paddle.com/webhooks/transactions/transaction-completed/).

`make test-hosted-payments` tests the real HTTP receiver with the shared verifier and SQLite storage.
The current component has controlled development integration only.
Full reconciliation and processor sandbox qualification remain open under F069.
Production payments remain disabled.

### Issue Responsibilities

| Issue | Resources and responsibilities |
| --- | --- |
| F065 | Billing accounts, platform connections, hosted access grants, tenant assignments, and customer onboarding |
| F066 | Request identities, billable attempts, usage observations, usage journal, and recovery |
| F067 | Price snapshots, provider costs, customer charges, and maximum authorized costs |
| F068 | Credit ledger, funds reservations, admission, settlement, and account holds |
| F069 | Funding orders, payment receipts, payment inbox, delivery outbox, and reconciliation |
| F070 | Shared decisions, complete service acceptance, and launch requirements |

F036 owns public price comparison. F067 must use the same catalog condition model.
P006 owns proposed service commitments. This document does not establish a service commitment.

## Existing Integration Reuse

The source review on 2026-09-22 identified the following implementation sources.
These are source findings, not evidence of deployed payment acceptance.

| Source | Reuse in LLM Proxy |
| --- | --- |
| `utils/billing` | Shared Paddle provider, signature verifier, checkout, portal, and webhook interfaces |
| `PoodleScanner/internal/billing` | Direct Paddle integration, catalog verification, and credit grant adapter |
| `PoodleScanner/web/site/js/core/billingCheckoutClient.js` | Browser checkout initialized from server configuration and an existing transaction ID |
| `PoodleScanner/test/web/tests/acceptance/billing-reconciliation-boundary.spec.js` | Checkout reconciliation and account isolation test approach |
| `Hecate/backend/internal/crosswordapi/billing_service.go` | Durable event records, transaction identity, retry state, and idempotent grants |
| `ledger/pkg/ledger` and `ledger/api/credit/v1/credit.proto` | Shared grant, reserve, capture, release, refund, and account isolation contracts |

Paths in this table are relative to the sibling repositories under the common development directory.
PoodleScanner currently imports `github.com/tyemirov/utils v0.17.1`.
Its billing integration uses `github.com/tyemirov/utils/billing`.
The shared package exposes `NewPaddleProvider`, `NewPaddleWebhookVerifier`, and `WebhookProcessor`.
Use the shared package instead of creating another Paddle transport or signature implementation.
Verify the selected release API before adding its dependency.

PoodleScanner creates Paddle transactions on the server before opening browser checkout.
Reuse this sequence and its separate sandbox and production configuration.
Browser completion must trigger a server status read, not a credit grant.
The selected account and funding order must control each transaction association.

Hecate currently routes purchases through RevenueCat and grants credits through Ledger.
Reuse its durable event and retry approach for direct Paddle funding.
LLM Proxy does not require RevenueCat, App Store, or Google Play integration.
Desktop and mobile browser widths remain part of browser acceptance.

The source products have different commercial rules.
PoodleScanner requires an active subscription for its top-up packs.
Hecate retains previously granted credits after purchase refunds.
LLM Proxy requires prepaid funding without a subscription and compensating entries for payment reversals.
Adapt these product rules explicitly while retaining the shared integration code.

### Shared Ledger Assessment

Ledger supports `Reserve`, `Capture`, `Release`, `Refund`, and atomic batches within one Ledger account.
Its current amounts use integer cents.
Its reservation expiry makes held funds spendable and prevents later capture.
Its refund operation credits a prior usage debit. It does not reverse a funding grant.

F067 must define exact fractional accounting without interpreting `amount_cents` as another unit.
F068 must preserve holds for uncertain provider work. It must not apply automatic Ledger expiry to those holds.
F069 must define payment reversals separately from refunds of usage debits.

The Ledger integration guide also describes an embedded Go service and a public `Store` interface.
Ledger F004 exposes the existing GORM adapter as `pkg/gormstore` in [merged PR 102](https://github.com/tyemirov/ledger/pull/102).
LLM Proxy uses this package from the published Ledger v1.1.0 module.
Its public integration tests prove that application records and Ledger effects share the caller's transaction.
They also verify permanent holds after restart and atomic settlement below the reserved amount.
The operator authorized publication on 2026-09-23. Release CI and publication completed for commit `7ba6162535652bcac01a08cd1bb6bf97085878d8`.
Use shared Ledger domain code or the existing service contract wherever it satisfies that boundary.
Keep one authority for balances. Do not add a competing local balance ledger.
Implement missing shared capabilities in their owning package instead of creating incompatible financial behavior in LLM Proxy.

A remote Ledger transaction cannot commit atomically with the LLM Proxy database.
F068 requires request admission, reservation, and ledger effects in one database transaction.
The integration must preserve that requirement through a shared storage contract before implementation proceeds.
The acceptance tests must prove transaction recovery and rejection before unauthorized provider dispatch.

## Current Integration Boundaries

The management API uses TAuth sessions and an explicit administrator identity.
`management_api.go` registers account resources and applies authentication and mutation middleware.
The current administrator boundary can restrict platform connection mutations.

`management_store.go` owns the managed database and encrypted provider credentials.
`account_connections_store.go` owns customer connections and tenant assignments.
Current assignments select either a customer connection or a hosted access grant.
The service must not substitute platform credentials when a customer connection fails.

`catalog_service.go` owns exact provider price selection.
Its current condition model includes provider billing conditions.
F067 must extend that contract where hosted pricing requires additional conditions.

`management_usage_writer.go` records operational telemetry through a memory queue.
These records do not establish customer funds or financial history.
F066 must write financial evidence durably before dispatch and before settlement.
HTTP acceptance fills this queue before a hosted request.
The request retains exact usage and one delivery record while the telemetry writer drops its records.
Replay returns the saved result without another provider call.

`media_operations.go` retains operation identities, catalog revisions, and credential references.
The development admission path binds these identities to the usage journal in the same transaction.
The worker connects dispatch and recovery to that identity.
The shared journal records native usage or explicit unknown quantities through the supported execution adapters.
Credential rotation must preserve recovery references for accepted work.
Revocation must prevent new provider work while permitting authorized result recovery.

`upstream_admission.go` controls provider capacity.
F068 must enforce financial admission separately and before each paid dispatch.
Native HTTP, client protocols, MCP, dictation, and media workers require the same financial authority.

## Open Decisions

- Identify the platform supplier and its Paddle account.
- Specify whether checkout adds tax to the selected funding amount or includes tax within it.
- Specify whether Paddle fees reduce platform proceeds or customer credit.
- Specify the maximum account exposure.
- Specify charges for failed work, cancellations, and provider continuations.
- Specify the platform retention schedule and authorized deletion procedures consistent with Paddle requirements.
- Confirm commercial authorization and paid capacity for each hosted offering.

## Development Acceptance

Complete each child issue before F070 acceptance.
Use controlled provider and processor protocols for repeatable development tests.
Record processor sandbox qualification separately from local protocol tests.
Use real HTTP entry points and automated browsers at desktop and mobile widths.
Restore one consistent database backup that contains all financial resources.
Prove account isolation, exact charges, bounded spending, and recovery through the public interfaces.

Production activation and live financial qualification require explicit operator authorization.
Development completion does not establish deployment, real payment acceptance, or provider invoice reconciliation.
