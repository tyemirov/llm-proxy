# Operational Token Measurement Evidence

## Current contract

Operational usage summaries preserve measurement evidence for each token quantity.
Each aggregate includes `token_coverage` for `request_tokens`, `response_tokens`, and `total_tokens`.
The same contract applies to account, tenant, time, provider, model, and administrator aggregates.

| Coverage field | Meaning |
| --- | --- |
| `measured_requests` | The quantity is known for all contributing logical generations in the retained execution event. |
| `partial_requests` | The retained quantity is a subtotal with at least one unknown contribution. |
| `unknown_requests` | The execution has no measurement evidence for this quantity. |
| `historical_requests` | The retained predecessor record has quantities without measurement evidence. |

These four counts sum to the aggregate execution count for each quantity.
They do not include rejected requests or lost queue events.
They do not prove that every provider attempt has complete measurements.
Retries, queue loss, and financial evidence remain separate concerns.
This contract does not change the process-local telemetry queue or activate hosted billing.

The existing numeric fields remain retained subtotals. They can contain historical quantities.
Coverage must accompany a subtotal when an operation depends on measurement completeness.
A reported zero remains a measured zero. An absent field remains unknown.
Vertex thought tokens without candidate tokens remain a partial output subtotal.
They cannot supply a complete total when the total field is absent.
A missing total is calculated only when both input and output counts are known.
An explicit total of zero is preserved.

A continuation combines quantities from distinct generations.
An unmeasured contribution makes the affected quantity partial.
Repeated poll snapshots replace observations of the same generation. They do not add another execution.
If the latest snapshot omits a quantity, the earlier quantity remains a partial subtotal.

## Public completion and display

The existing public completion usage object requires all three complete quantities.
Incomplete operational measurements produce `usage: null` in `/v1` and MCP.
The `/v2` JSON response omits incomplete usage. All three paths omit token headers.
The summary still retains available partial quantities and their coverage.
This rule keeps partial measurements from becoming invented complete zeros.
MCP retains its existing tools and output shape.

The dashboard shows numeric zero for measured zero and `Unknown` for wholly unknown totals.
It labels partial or mixed quantities as subtotals with incomplete request counts.
It labels wholly historical quantities as historical subtotals with unknown measurement.
If an API response omits required coverage, the display rejects the response.
The unavailable token card shows `Unknown` and the page shows its existing error.
Token chart labels retain the same distinction. Unknown points do not become measured-zero markers.
Administrator token values use the same formatter.

## Persistence and upgrade

The startup transaction adds the nullable `measurement_evidence` column once.
The packed value records unknown, partial, or complete evidence for each quantity.
A null column means historical evidence. Startup does not infer presence from numeric values.
Predecessor quantities, event identities, authorization, routing, and timestamps remain unchanged.
A failed startup transaction rolls back the column addition.
A later startup validates the evidence values without another data transfer.

Saved completion metadata retains the same operational evidence across replay.
Earlier saved completions without evidence remain unverified.
This metadata does not supply financial quantities or replace the financial journal.

## Release and recovery

B305 owns implementation and repository acceptance.
Release, publication, deployment, and production checks need separate evidence.
The established lifecycle requires the clean primary default branch and an exact published release.
The user selects the current provider work and B305 for one combined release.

The Gateway lifecycle uses forward recovery and rejects deployment of an older release.
A recovery release must retain the evidence column and current saved-completion shape.
Do not delete the column or replace a production database to reverse this display change.
Keep quantities and coverage intact in a corrective release.
Production identity, publication receipts, live data compatibility, and health checks remain rollout prerequisites.
