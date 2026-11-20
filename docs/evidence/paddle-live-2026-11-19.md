# Live Paddle paid-pilot preparation — November 19, 2026

F101 is an activation and qualification operation. F087 sandbox qualification remains complete and separate.
This record establishes live account and resource preparation. It does not establish deployed checkout or a completed live payment.

## Verified account

- Intended login: vadym@tyemirov.net. The user signed into Paddle live and approved dedicated credential and webhook creation.
- Account: MPR Lab, seller 287367, `slr_01kgtaq3a0qyht2skncvt4hkem`.
- Paddle's account API reports legal name Marco Polo Research Lab, website https://mprlab.com, and support contact support@mprlab.com.
- Live UI and `/verifications?verification_type=onboarding` report all onboarding steps completed. Business verification was completed October 2.
- Bank-transfer payout settings are populated. There is no completed payout in the dashboard history; receipt of a payout was not tested.
- Account balance currency is USD with zero active automatic conversions. Settings were preserved.
- The exact `llm-proxy.mprlab.com` checkout domain is absent from the approved domain list. Domain approval remains required. Paddle explicitly requires separate subdomain approval; the approved `mprlab.com` parent does not cover this checkout host. See [domain review](https://www.paddle.com/help/start/account-verification/what-is-domain-verification) and the [go-live checklist](https://developer.paddle.com/build/go-live-checklist/), rechecked November 19.
- The shared account default remains `https://poodlescanner.com/pricing`. LLM Proxy uses its own per-transaction URL.

## Dedicated production resources

| Resource | Verified configuration |
| --- | --- |
| Product | `pro_01m7tvgx960ptsks35fjqcws1e`, LLM Proxy prepaid usage, active, standard digital goods |
| Price | `pri_01m7tvjahkr1qcax59g9cn2e7j`, 500 cents (USD 5), one-time, tax external, quantity 1 |
| Server key | LLM Proxy production, active, rotatable, expires February 17, 2027; 11 read and 3 write permissions |
| Public client token | Dedicated LLM Proxy production token, active; saved privately |
| Webhook | `ntfset_01m7tw0kqx1ehna0eg98b6fj7q`, API version 1, platform events only, sensitive fields disabled, inactive until deployment |
| Destination | `https://llm-proxy-api.mprlab.com/api/payments/paddle/events` |
| Application checkout | `https://llm-proxy.mprlab.com/app/` |

The key reads account, adjustments, checkout domain/settings, customers, notifications/settings, prices, products, transactions, and verifications.
Its write permissions are limited to customers, transactions, and customer portal sessions.
Paddle permissions apply across the seller account. Other applications' credentials and destinations were preserved.
The live API root verified this key against the production environment and the expected account.

The destination subscribes to transaction billed, canceled, completed, created, paid, past_due, payment_failed, ready, updated, and revised events, plus adjustment created and updated events.
The dashboard creates destinations active; this destination was immediately deactivated and its final inactive state verified through the UI and API.
Its generated secret and the two application credentials are saved only in the selected private input `.mprlab/deploy/.env`.

## Available validation

Actual live API reads verified identity, onboarding, domains, default checkout URL, product, price, and inactive destination.
No live transaction or payment was created by the pricing checks.

| Preview location | Currency | Principal | Tax | Total |
| --- | --- | --- | --- | --- |
| U.S., sample ZIP 10001 | USD | 500 | 44 | 544 |
| U.K. | USD | 500 | 100 | 600 |
| Germany | USD | 500 | 95 | 595 |

Each location was checked with explicit USD and omitted currency. Both produced the same USD principal and totals.
These previews establish current price calculation. They do not establish browser checkout behavior after country changes.

Production configuration and manifest inputs now reference the dedicated resources.
Public pricing and refund-support pages and payment disclosures are prepared locally for review and publication.
F102 covers owned-order continuation. Initial focused tests used an ignored workspace; the app now adopts published utils v0.20.0 through normal Go resolution. The owned workspace was removed. The required MCP SDK upgrade to v1.8.0 passed independent source/contract review. Payment checks against published utils v0.20.0 passed (64.567s backend,0.866s CLI); MCP/hosted-MCP checks passed with workspace overrides disabled and the race detector (31.718s backend). The first application CI attempt stopped in the installed Gateway fixture: `undefined environment reference PADDLE_API_KEY`. The fixture now clears inherited payment inputs and supplies controlled values for all three; its focused check passed in 25.389s. The second application CI attempt passed that gate, then stopped at a stale static-page module revision assertion. Updated the Go assertion and the remaining shared-UI dynamic import to revision 20261119f102, including the early continuation bootstrap assertion. The focused Go check passed (0.477s backend). The shared-UI browser fixture also inherited production payments; ordinary management scenarios now explicitly disable payments, while payment suites keep their local processor configuration. Four shared-UI browser cases passed (12.2s); the OpenAPI Pages artifact check passed. Independent read-only review found no actionable issue in these fixture/assertion corrections. The third final CI attempt then exposed one packaged CLI fixture missing the three payment inputs and two provider-harness output checks retaining production payment configuration. The CLI fixture now supplies controlled synthetic inputs and asserts resolved production configuration; its focused check passed (0.348s). B343 owns explicit disabled-payment configuration for provider qualification, local Compose, and the tagged non-payment recovery fixture. B343 now uses one typed stdlib renderer to produce explicit `payments: null` for provider qualification and local Compose; the tagged recovery fixture explicitly disables payments and its unused qualification budget. Renderer/provider regressions pass (0.708s), local startup/shutdown/regeneration/rejection checks pass (3.526s), all provider config cases pass (0.582s), controlled provider preflight passes, and the tagged recovery CLI passes (2.051s). Strict Python lint, formatting, shell syntax and diff checks pass. Independent review is clear for the current canonical layout and startup failure propagation. The fourth application CI attempt passed backend integration (616s), race tests, Python tests, all 190 frontend browser tests, and the Pages artifact. One authenticated payment test failed while 18 passed: it assumed the paid order appeared on the first history page, after another test had created more than 50 shared orders. Source and the worker's captured-state observation show paid continuation completed and the uncertain key remained; the original screenshot/trace were replaced by the focused Playwright run, so the retained CI log and source are the durable failure evidence. Independent diagnosis identified that pagination assumption and a related capped-count assertion gap. F102 now corrects test synchronization and compares complete order ID sets across pages. A deterministic real-API one-item history page reproduced the same failure after direct paid status and key preservation succeeded. The corrected tests use terminal continuation status, UI pagination for historical actions, and complete paginated ID-set comparisons. Review also found an initial-load race in the pagination helper; a gated first response now covers its initialization wait. After the last correction, all nine payment/service cases passed (48.7s), frontend lint and diff checks passed, and independent rereview found no remaining actionable findings. Red/green/final artifacts were preserved privately and the owned scratch directory was removed. Governor check initially required the new local-config ignore rule before the managed block; that ordering was corrected and structural check plus diff check passed. Final application `make ci` then passed all 15 gates in 958 seconds with exact 100.0% Go statement coverage, all 190 frontend browser tests and all 19 authenticated browser cases. Content fingerprints for 1,107 source files were unchanged during the gate. The complete log and fingerprint/result receipts are retained privately. F102, B342 and B343 development acceptance is complete.
Focused browser checks cover purchase support links, static pages without external scripts, and shared-shell behavior at desktop and narrow widths.
B342 corrects the static footer's narrow-screen width calculation.
Frontend lint, the lifecycle contract check, and the Governor structural check passed during preparation.

## Activation and acceptance handoff

1. Independent review completed without remaining actionable findings. Five final checkout recovery regressions passed after the last correction; four purchase/legal/shared-shell checks passed after the final public-page changes. Focused payment, lint, lifecycle and Governor checks also passed. Selected full CI passed all 15 gates in 958 seconds with 100.0% Go coverage, 190 frontend browser cases and 19 authenticated cases.
2. The operator approved a timing exception: final SDK/app CI precedes publication/deployment, and actual live validation follows deployment. SDK `make ci` passed all 18 package 100% coverage gates. The reviewed SDK source commit `c596535` and release commit `95610ea` are published as [v0.20.0](https://github.com/tyemirov/utils/releases/tag/v0.20.0). Published assets and the module proxy origin were verified. The app adopted that release; focused published-dependency payment and MCP race checks passed before application final CI.
3. SDK publication and consumer adoption are complete. Application validation is complete. Deliver its source and publication through the normal workflow.
4. Obtain approval for the prepared public purchase disclosures and publish the website. Verify pricing, refunds, terms, privacy, and their links on the exact public domain before requesting domain review.
5. Submit `llm-proxy.mprlab.com` for Paddle approval and verify approval before live checkout. Keep the other apps' default link unchanged.
   The operator selected unrestricted funding on November 19: any authenticated billing-account owner can create funding orders. Hosted-access grants separately authorize model use. No pilot allowlist is required for funding.
6. Activate the reviewed production runtime and then enable the dedicated destination. Verify the webhook rejects an invalid signature without a financial effect.
7. Create one application funding order. Confirm the generated checkout URL retains the order query and opens the same owned transaction. Check USD 5 principal after country changes before payment.
8. Have the payer approve the displayed real charge and complete required card or 3DS steps. Verify no early grant, signed completion, one 500-cent credit, receipt, replay, isolation, reconciliation, and financial backup restoration.

No live charge, production rollout, live webhook delivery, or live financial-conservation acceptance is claimed.
Commercial failure-charge decisions, account exposure limits, and the platform financial-retention schedule remain the existing operator decisions recorded in the hosted-billing design. Set those policies before admitting paid pilot usage.

## Source and publication boundary

Application source base: `4caf6d705e0b18af45e445684ba3f1683cd9c226`. Shared utils source base: `5cb885b8b7ed7f2ad9543d636274a1106800dd4b`; published feature commit `c596535c0aaa46c13bb9e3cf002e4d36b7db0f6b`, release commit `95610eabd76f179a6ae982f69a48a5e85c32d742`. Both checkouts use their verified remote default branch, `master`. The app has uncommitted work; SDK feature/release commits are published. Installed Gateway is v9.3.6, lifecycle contract 4.

Published utils v0.20.0 includes `CheckoutURL`. The app uses that version without a replacement or workspace. The SDK's pre-existing governance edits were temporarily set aside with explicit approval and all 13 files were restored and content-verified after publication.

## Private resume evidence

The operation owner is this Codex chat. Private metadata and screenshots are in
`/Users/tyemirov/.codex/operations/2026-11-19-llm-proxy-paddle-production/`.
The account, price, webhook, and currency preview JSON records contain no credential values.
All run-owned browser tabs were closed and closure was verified. An authenticated session can be reused by reopening the live dashboard; sign-in is not requested again unless Paddle requires it.

Primary references: [account identity](https://developer.paddle.com/api-reference/account/get-account/), [go-live checklist](https://developer.paddle.com/build/go-live-checklist/), [per-transaction checkout URL](https://developer.paddle.com/api-reference/transactions/create-transaction/), [transaction checkout](https://developer.paddle.com/build/transactions/pass-transaction-checkout/), [transaction preview](https://developer.paddle.com/api-reference/transactions/preview-transaction-create/), [domain review](https://www.paddle.com/help/start/account-verification/what-is-domain-verification), and [Paddle refund policy](https://www.paddle.com/legal/refund-policy).
