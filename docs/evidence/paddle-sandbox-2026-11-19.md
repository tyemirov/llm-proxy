# Paddle sandbox qualification — November 19, 2026

F087 is complete. Actual sandbox qualification and final repository CI passed. Account verification, checkout configuration, a new unpaid order, actual payment and signed delivery, delayed confirmation, replay, partial and full refunds, isolation, reconciliation, and financial backup restoration passed. Production activation is separate.

## Scope and identity

The operator authorized existing-account verification, dedicated sandbox setup, and actual qualification on November 19, 2026. The primary checkout was used, preserving unrelated work. Source commit: `4caf6d705e0b18af45e445684ba3f1683cd9c226`, with working-tree billing corrections B334, B335, B336, and B337. The private operation record includes source hashes and the selected provider catalog.

Paddle's authenticated API root identifies sandbox account `slr_01khmcbqn0v8mvmtqf51612cme` as Marco Polo Research Lab. Client token `ctkn_01khmchvjrs47h9g337m8rfsph` is active and matches the existing documented private input. The account has product, price, transaction, adjustment, notification, and client-token permissions needed for qualification. Its API key lacks the newer account-details and checkout-settings permissions. The operator's authenticated Paddle dashboard independently confirmed Marco Polo Research Lab, website `https://mprlab.com`, and seller ID `48660`. The existing in-app browser session supplied those permissions and saved the checkout setting; no replacement credential was required.

| Resource | Sandbox identity |
| --- | --- |
| Product | `pro_01m7sdxwabqecenh0ytyf3zcv2` — LLM Proxy prepaid usage |
| One-time USD 5 price | `pri_01m7sdxwkftrvf2cpezzrnm1j8` |
| Notification destination | `ntfset_01m7sdz3afaerqtenbvfy6ryva` |
| Funding order | `funding-093711f0709a93ae618f361a28cfb272` |
| Transaction | `txn_01m7se638xj3fjwx2ge41fzgmp` |
| Completion event | `evt_01m7se9pst2keahgmkd7j2kbwm` |
| Original completion notification | `ntf_01m7se9q5qp86drzft3s9ss34q` |
| Post-credit replay notification | `ntf_01m7sep9h0mpqsdr3n2k0bxqtn` |
| Partial refund | `adj_01m7senfqge4svqcphbg8c4hvt` |
| Remaining refund | `adj_01m7sfejz9rq1ccpwkf9e01nmt` |
| Checkout-configuration order | `funding-4ec1fec856ec6bb6f740d0fbdae15a43` |
| Checkout-configuration transaction | `txn_01m7tjf564xey4sw27yd2q7h1j` |
| Post-configuration replay | `ntf_01m7tjwaf73hmjkz56xy2q1qny` |

Existing products and destinations were preserved. The dedicated destination uses its own secret and real sandbox platform events. Existing API and client-token credentials remain in their original private input; the operation launcher supplies only the documented values to the process environment.

## Actual acceptance

The normal LLM Proxy binary runs with an isolated managed database and actual Paddle sandbox API selection. The browser uses Chrome with the current frontend, real local TAuth, and the existing controlled Google exchange adapter. Authentication is a local test fixture; payment processing is actual Paddle sandbox execution. Hosted provider execution is disabled, and no model-provider request was dispatched.

Browser origin is `http://localhost:18880`; API origin is `http://localhost:18881`. A temporary HTTPS tunnel forwards payment webhook POST requests to the normal receiver and serves a read-only sandbox checkout fixture at `/checkout/`. The fixture loads actual Paddle.js with the public sandbox token and opens the server-created `_ptxn`. Local authentication and management interfaces stay on loopback.

The dashboard saved and retained `https://1d95-47-155-12-131.ngrok-free.app/checkout/` as the sandbox default payment link. The dashboard rejected the initially planned localhost URL as invalid. A new application-created order returned that HTTPS URL with its transaction identifier. Opening it rendered actual Paddle Checkout in TEST MODE for LLM Proxy prepaid usage, quantity one, USD 5. The new order remains pending, has no receipt, and granted no funds. The private screenshot is `checkout-configured.jpg`.

| Scenario | Observed result |
| --- | --- |
| Checkout | A browser-created order completed with Paddle's documented test card. Gross payment 544 cents, tax 44 cents, application credit 500 cents. |
| Delayed confirmation | While webhook delivery returned HTTP 503, Paddle showed completed while the application order remained pending and posted/available funds remained zero. |
| Invalid signature | The public receiver returned HTTP 401 with `payment_signature_invalid`; no funding effect occurred. |
| Signed delivery | The original completion notification succeeded on its third delivery attempt after the hold was removed. |
| Replay and restart | Actual Paddle replay succeeded. Restart recovered the original event after B334. The ledger retained one 500-cent grant. A subsequent restart preserved the partial-refund hold. |
| Partial refund pending | A 200-cent principal refund plus 18 cents tax produced a 200-cent hold: posted 500, available 300, reserved 200. |
| Partial refund approved | After B335 and B336, posted and available funds were 300, reserved zero, and reversed credit 200. Actual reconciliation had no differences. |
| Remaining refund pending | Paddle accepted a full adjustment of the remaining 326 cents, including 300 cents principal and 26 cents tax. The application held the remaining 300 cents. |
| Full refund approved | Actual approval was observed at 08:50:19 UTC. The order is refunded, reversed credit is 500 cents, and posted, available, and reserved funds are all zero. Original gross and tax remain 544 and 44 cents; adjusted gross and tax are zero. |
| Balance conservation | One 500-cent grant and two deductions of 200 and 300 cents explain the zero balance. No payment inbox events or checkout deliveries remain unresolved. The new unpaid order has a completed checkout-delivery job and remains pending. |
| Checkout configuration | Dashboard setting survived reload. The new order generated the configured link, and actual Paddle Checkout rendered the correct USD 5 item. No additional payment was submitted. |
| Post-configuration replay | The original completion notification was replayed and delivered after the setting change; one funding grant, zero balance, and zero unresolved events remain. |
| Backup restoration | The canonical snapshot command produced an integrity-checked financial backup. A separate restored database passed actual processor reconciliation with the same final receipt and zero differences. |
| Account isolation | A second real local TAuth subject received HTTP 404 for the owned order, receipt, and balance, including after the receipt existed. |
| Browser receipt | The rendered receipt shows original payment 5.44 USD, tax 0.44 USD, credit 5.00 USD, and the pending partial reversal. It remains visible without horizontal overflow at emulated widths 390 and 320 pixels. The browser subsequently displayed the fully refunded receipt with zero current payment and pending reversal. Temporary viewport overrides were reset. |
| Reconciliation replay | Reusing `f087-partial-pending-20261119-0828` returned its original retained report unchanged. |

The price uses external tax for these sandbox scenarios. Processor fees reduce proceeds rather than customer credit. These test settings do not settle the production commercial-policy decisions.

## Native financial checkpoints

Each checkpoint uses `make qualify-paddle-sandbox`, the actual sandbox configuration without an API-origin override, and independently prepared expected amounts. All run identifiers below are retained in the isolated database.

| Run identifier | Result |
| --- | --- |
| `f087-paid-recovered-20261119-0828` | Passed: 500-cent credit, 544 gross, 44 tax, zero differences. |
| `f087-partial-pending-20261119-0828` | Passed: pending reversal 200, zero differences. |
| `f087-partial-approved-verified-20261119-0842` | Passed: reversed credit 200, zero differences. |
| `f087-remaining-pending-20261119-0842` | Passed: reversed credit 200 and pending reversal 300, zero differences. |
| `f087-full-refunded-20261119-085019` | Passed: reversed credit 500, zero pending reversal, zero differences. Its retained report also replayed successfully. |
| `f087-restored-20261119-0851` | Passed: independent restored financial database, reversed credit 500 and zero differences. |
| `f087-checkout-configured-verified-20261119` | Passed: original fully refunded order and new unpaid order, two orders completed, zero differences. |
| `f087-restored-checkout-configured-20261119` | Passed: canonical updated backup restored independently, both orders and exact receipts verified, zero differences. |

Earlier paid and partial-approved checkpoint attempts failed before their corresponding corrections or before the scheduled event retry. Their logs are preserved separately from the passing results.

## Corrections and validation

B334 compares webhook and API revision timestamps at microsecond precision. Actual webhook timestamp `2026-11-19T08:20:47.02610139Z` and API timestamp `2026-11-19T08:20:47.026101Z` otherwise caused permanent stale-snapshot rejection. Original timestamps and exact financial digests remain intact. An earlier conflicting lifecycle status is still rejected. This precision rule is supported by actual sandbox evidence; Paddle documents RFC 3339 timestamps without promising a fixed precision.

B335 accepts canonical signed integers only for the adjusted transaction retained-fee field. Actual partial approval reports `-31` there and `31` on the adjustment. Customer principal, tax, and individual adjustment amount validation remain unsigned.

B336 compares equal transaction revisions without their computed adjusted totals. Paddle changes those aggregates without changing the transaction revision. Complete retained evidence and digests remain unchanged. Original transaction evidence and independently ordered adjustment records remain checked. Reconciliation reports `adjustment_history_conflict` for missing, older, or conflicting adjustment history.

B337 excludes generated `checkout.url` values from transaction-revision equality. Changing the account default link also changes historical transaction URLs without advancing their revision. Checkout presence, original money, identity, status, captures, and independent adjustment history remain strict. Retained observations and reconciliation evidence preserve the complete original URLs and valid digests. Paid and refunded orders, signed lifecycle events, replay, restart, and combined URL/financial conflicts have focused coverage.

Independent architect review covered the design and final corrections and found no remaining actionable findings. Focused race checks cover timestamp precision, malformed fee evidence, fixed transaction revisions, original-evidence conflicts, adjustment-history conflicts, corruption, replay, and restart.

Initial reproductions returned pending instead of paid/canceled for B334, retained 500 cents instead of reversing 200 for B335, and preserved a pending hold after approval for B336. The corrected focused regressions pass. Complete `make test-hosted-payments` passed after the final correction: proxy package 67.418 seconds and CLI package 0.995 seconds. Selected `make check-format` and `git diff --check` passed. An existing conflicting-transaction test was updated to change original billing evidence at an equal revision; computed aggregate changes are valid under the current contract.

Complete focused payments passed again after B337 in 63.718 seconds. The final corrupt-refund reconciliation regression passed with race detection in 2.576 seconds and verifies failure propagation without a balance change. Independent final review found no actionable findings. Final repository CI passed; see the final result below.

The first resumed backup check failed because files disappeared from the shared Go build cache (`could not import ... no such file or directory`). Repeating the same native check with a private `GOCACHE` passed. Full CI uses that private cache.

Repository-wide `make go-lint` initially found an unrelated incomplete generated benchmark snapshot: `budget_model_qualification_xa1d_test.go:107:19: undefined: buildRouterWithCatalogs`. Its 12 files were preserved in the private operation directory and its original location now links to them. Go package discovery excludes the linked directory; consumers of the original path retain access. Static analysis then passed. `snapshot-relocation.txt` records this reversible relocation. An initial Governor check reported drift in AGENTS.GIT.md, PLANNING.md, and POLICY.md, plus missing issue-format provenance. This task preserved those instruction files. All issue identifiers introduced for this work are unique across the active tracker and archive.

The first final `make ci` passed installed Gateway qualification, release policy, formatting, Go analysis, and Python analysis, then failed frontend analysis with `openapi_docs_out_of_date`. B338 regenerated `site/docs/index.html` from the existing canonical OpenAPI output-allowance edits. The diff contains only those descriptions and generated source digests. `make frontend-lint test-openapi-pages-artifact` then passed. The next final CI passed all hosted Go groups but failed `TestDashScopeMediaCatalog` with `image offerings=4`. The current catalog already enables `dashscope/qwen3.8-27b`; B339 updates the exact expected image-model set and includes that model in the existing real HTTP image-count and data-URI boundary checks. All those focused checks pass with race detection (proxy 2.508 seconds). Admission race tests and Python tests also passed in that run. Subsequent corrections are recorded below.

The following final CI attempt passed seven Go groups, then exposed the same activated-model omission in `TestManagementProfileListsCurrentCatalogModels`. Independent review found two related expectations: DashScope family labels and public media offering counts. B339 now covers all of them. B340 updates the Gemini polling fixture to expect only the selected 65,536-token allowance, preserving its polling and deletion checks. The changed catalog/routing cases passed with race detection in 4.147 seconds.

A broader focused remaining-group run then timed out at `TestQualificationBudgetXA1DConcurrentCapAndTenantIsolation`. Its explicit output allowance was already 128. The current Cerebras context and prices require `ceil(131072 × 0.99) + ceil(128 × 1.49) = 129953` microUSD, exceeding the old 70000 test cap. B341 raises only that test cap to 140000: one reservation leaves 10047, a second is rejected, and actual seven-microUSD settlement leaves 139993. The test now reports premature HTTP completion and releases blocked handlers during cleanup. Two related OpenAI/Gemini tests verify that catalog defaults exceeding a private allowance reject before dispatch, followed by explicit-128 success. Original failures and timeout traces are retained privately. All XA1D qualification-budget tests passed with race detection in 74.535 seconds. Independent final review of B339 through B341 found no actionable findings. The corrected remaining-group run completed in 138.819 seconds and exposed only an old excessive-input fixture value of 65537, now within the current context. B341 makes that value the frozen input ceiling plus one. All measurement-boundary scenarios then passed with race detection in 7.496 seconds; independent final review found no actionable findings. All identified corrections and non-CI work are complete. Final CI passed; see the final result below.

The subsequent full CI passed all nine Go groups, admission race checks, and Python tests. Exact coverage still rejected three obsolete blocks despite a rounded total of 100.0%: a second continuation default and private wire-cap injection. I323 removes those duplicate defaults after independent architect approval. Qualification now requires a serialized positive output allowance within its private maximum; missing or malformed Gemini configuration or output fields reject before reservation or dispatch. The selected router allowance remains unchanged.

Five controlled boundary reproductions previously dispatched and retained reservations for missing wire allowances. After correction they require qualification failure, zero upstream calls, zero attempts, unchanged allowance, and no suspension. Qualification, routing, automatic sizing, and selected-ceiling race checks passed in 148.369 seconds; wire preparation has 100.0% statement coverage. The existing public unknown-ceiling continuation case passed with race detection in 2.389 seconds. Independent final review found no actionable findings. These corrections passed final full CI as recorded below.

The full run after I323 passed all backend gates and exact 100.0% coverage, then passed 187 of 188 frontend cases. The sole failure was B339's Flash-selection case still expecting the activated 27B offering to be absent. The assertion now requires one active 27B offering without the default label while retaining Flash selection, save, reload, and disabled-candidate checks. The focused browser case passed in 5.0 seconds. Independent review is clear. The final canonical `make ci` receipt below includes this test correction.

## Final validation

Final local `make ci` passed all 15 gates in 799 seconds, with exact 100.0% Go statement coverage, 188 frontend browser cases, and all 13 authenticated browser cases.

Command: `GOCACHE=/Users/tyemirov/.codex/operations/2026-11-19-llm-proxy-paddle-sandbox/go-cache make ci`.
The complete terminal receipt is retained privately in `final-ci-terminal.log`; exit status was zero.
The live-provider harness gate used its local provider fixture. Actual payment-provider qualification is established separately by the sandbox scenarios and processor checkpoints above.
F087, B334 through B341, and I323 are resolved. Changes remain in the primary checkout; publication and production activation are separate.

## Changed files and contracts

Payment implementation changes are in `internal/proxy/hosted_payments_processing.go`, `hosted_payments_adjustments.go`, `hosted_payments_states.go`, and `hosted_payments_reconciliation_compare.go` in that directory. Existing webhook and customer response schemas are preserved. Reconciliation adds the `adjustment_history_conflict` difference code.

Payment regressions in that directory are `hosted_payments_timestamp_precision_internal_test.go`, `hosted_payments_retained_fee_internal_test.go`, `hosted_payments_adjustment_revision_internal_test.go`, and `hosted_payments_checkout_revision_internal_test.go`. Existing `hosted_payments_processing_internal_test.go` and `hosted_payments_revision_recovery_internal_test.go` fixtures reflect the current processor contract.

Incidental final-gate corrections update `internal/proxy/dashscope_media_limits_test.go`, `internal/proxy/management_test.go`, `internal/proxy/provider_catalog_end_to_end_test.go`, `internal/proxy/provider_routing_test.go`, `internal/proxy/qualification_budget_xa1d_test.go`, `internal/proxy/qualification_budget_transport_internal_test.go`, `tests/capabilities_test.go`, `tests/e2e/management-ui.spec.js`, and generated `site/docs/index.html`. I323 also removes the obsolete continuation branch in `internal/proxy/provider_router.go`, replaces private cap injection with validation in `internal/proxy/qualification_budget_transport.go`, and documents that rule in `docs/qualification-budgets.md`. Existing catalog and automatic-sizing changes from other work remain intact.

Operator results are recorded here and in `docs/hosted-billing.md`; F087 and B334 through B341 retain their evidence in the repository tracker. No other event contract changes are introduced.

## Resume and paid-pilot prerequisites

The private operation directory is `/Users/tyemirov/.codex/operations/2026-11-19-llm-proxy-paddle-sandbox/`. Its `HANDOFF.md` identifies the owner, exact browser/account, source inputs, active resources, expected checkpoints, and recovery steps. Raw payment evidence, the database, and secrets stay private.

The dedicated product and price remain active. The saved default payment link names the temporary HTTPS qualification host. After acceptance, the owned notification destination was paused, the tunnel and local runtimes stopped, and all owned browser tabs closed. Ports 18880 through 18883 were verified closed. No dashboard sign-in handoff remains.

For another sandbox run, use the private launchers and isolated configuration, start the local runtime and a new HTTPS tunnel, update this destination and the dashboard default payment link to that tunnel, activate the destination, and verify a fresh unpaid order before new financial scenarios. Preserve the existing orders and independently reconcile them. Use a durable checkout host and webhook origin for a continuing pilot.

Recreate the required resources in the live account and complete the operator launch checklist before actual customer payment acceptance. This sandbox record does not establish live account approval, provider commercial qualification, deployment, or paid-pilot activation.
