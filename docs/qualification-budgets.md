# Operator qualification budgets

`qualification_budget` is an optional server policy for bounded text-model
qualification on one existing tenant. The canonical inactive value is `null`.
The selected deployment input, `LLM_PROXY_QUALIFICATION_BUDGET`, supplies the
policy as JSON. Provider credentials remain in account-owned connections.

The policy declares a unique budget ID, tenant ID, UTC expiry, remaining
microUSD allowance, receipt database path, paid-attempt limit per logical
request, exact provider/model routes, and output caps. An optional owner ID
asserts ownership; the server derives the owner from the existing tenant.
Each offering selects its catalog `price_region` explicitly when the rates are
regional; an omitted region selects only catalog rates with no region. Verify
this selection against the assigned connection before dispatch.
Named verification connections attribute a new connection's paid verification
before tenant assignment. Names use the account connection's canonical key.
Existing connections can be attached without creating another credential.

## Accounting and dispatch

The receipt database shares the management database's persistent directory.
Initialization freezes the policy, catalog revision, token ceilings, and list
rates. A registry in the management database prevents allowance resets after
receipt loss, policy changes, removal, or reuse of a closed budget ID. One
process holds the journal's lifetime lock. Startup validates existing receipts.

Before each paid upstream POST, the Proxy atomically reserves the catalog-priced
input ceiling plus the enforced output maximum and persists dispatch intent.
Retries and continuations share the logical request's attempt limit. Selected
attempt receipts retain the originating Proxy request ID for reconciliation
with the client's `X-LLM-Proxy-Request-ID` response header. Selected
requests retain normal capacity admission and use a fresh HTTP/1 connection
with redirects rejected. Other tenants retain their ordinary transport.
Polls correlate with the existing reservation and settle it once.

Complete usage settles an uncached list-cost estimate, rounding each component
upward to a microUSD. Reasoning is counted once; separately reported Gemini
thinking is added to visible output. Cache input is conservatively priced as
uncached input. This estimates sustainable token cost; it does not establish
provider invoices, credits, or Proxy customer charges. Tools, media, explicit
cache controls, and service-tier overrides are outside this qualification policy.

Lost or malformed responses, contradictory usage, abandoned pending work, and
journal failures after dispatch retain reservations and suspend further paid
dispatch. Admission or persistence failures before dispatch stop the request
before network execution. When admission proves an initial generation was not
sent, its receipt settles at zero cost and the full reservation returns to the
allowance without suspension. A rejected status poll retains the existing
reservation because the generation already started. A restart marks interrupted
attempts inactive and preserves the suspension.
The allowance and expiry fail closed. Keep the active operation on a Gateway
release that implements this policy; database schema versioning alone does not
enforce that deployment floor.

## Operator inspection and reconciliation

Run the deployed CLI against its existing management database. Inspection
emits metadata and cost evidence without prompts, answers, or credentials.
Each inspection validates accounting and collects its output within one database
snapshot, so concurrent reservations and settlements cannot mix receipt states:

```sh
llm-proxy qualification-budget inspect \
  --management-database /data/llm-proxy-management.sqlite --budget-id XA1D
```

When an inactive attempt's usage remains unknown, consume its entire retained
maximum explicitly:

```sh
llm-proxy qualification-budget reconcile-max --attempt-id 1 \
  --management-database /data/llm-proxy-management.sqlite --budget-id XA1D
```

This operation is idempotent, refunds nothing, and preserves the unknown-usage
classification. Inspection reports observed estimates and conservative unknown
maxima separately. Suspension clears only after every unresolved attempt has
been reconciled. An active dispatch cannot be reconciled.

After all attempts are settled or conservatively reconciled, close the budget:

```sh
llm-proxy qualification-budget close \
  --management-database /data/llm-proxy-management.sqlite --budget-id XA1D
```

Closure first persists in the journal, then in the registry. Retrying closure
completes an interrupted operation. Retain these receipts and closed identities.
Set the deployment policy to `null` through the normal selected lifecycle after
closure. A later authorized qualification uses a new budget ID and receipt path.

The [TellTale operation](telltale-tenant-benchmark-activation.md) records its
remaining allowance, exact tenant, candidates, and validation sequence.
