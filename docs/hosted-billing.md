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
Its GORM adapter resides under `internal/store/gormstore` and cannot be imported by LLM Proxy.
Before integration, verify the published storage interface and the required transaction boundary.
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
