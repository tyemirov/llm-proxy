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
The grant names its exact models and their permitted operations.
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

`media_operations.go` retains operation identities, catalog revisions, and credential references.
F066 must bind these identities to the usage journal.
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
