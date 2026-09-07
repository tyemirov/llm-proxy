# Gemini Customer Connections

P012 assesses independent customer connections as of September 7, 2026.
This document records a proposed support decision and its acceptance gaps.
P001 owns the shared provider connection interface.

## Proposed Decision

Continue Vertex qualification at the selected concurrency of 10 requests.
Retain Gemini Flash 3.8 as a separate candidate with its own customer key.
Resolve the Gemini Pro request quota and the Flash-Lite response failure before accepting those offerings at the tested load.
Permit the same exact model through both providers when each offering passes all required checks.
Require explicit provider selection and separate credential records.

F060 proved the Vertex transport with an operator-managed service account.
That result does not satisfy P012's independent customer setup requirement.
Both API-key routes passed the direct comparison with the selected existing account.
The next customer step is independent setup and public proxy acceptance.
Further authentication changes require the completed P012 decision and separate implementation approval.

## Customer Flows

### AI Studio

1. Open the official [AI Studio key page](https://aistudio.google.com/api-keys).
2. Sign in with an eligible Google account and accept the applicable terms.
3. Create a project or import a project that the customer controls.
4. Create a current authorization API key for the Gemini Developer API.
5. Open the project's billing setup and complete the assigned payment plan.
6. Verify the paid tier, available credits, and exact model quota.
7. Return to the proxy and select the AI Studio provider and exact model.
8. Paste the key and validate the selected route through the proxy.
9. Save the connection and selected default through the P001 operation when implemented.
10. To disconnect, remove the saved connection and verify that it can no longer dispatch requests.
11. To revoke the key itself, delete it in Google's credential interface.

Google creates new AI Studio keys as authorization keys.
Its documentation announces rejection of standard keys in September 2026.
Project permissions can prevent key creation. [Key requirements](https://ai.google.dev/gemini-api/docs/api-key)
AI Studio requires an eligible region and an adult account. [Access requirements](https://ai.google.dev/gemini-api/docs/available-regions)
New paid accounts can require a minimum $5 prepayment. Existing accounts can have a different assigned payment plan.
A billed project alone does not prove available prepaid credits. [Billing requirements](https://ai.google.dev/gemini-api/docs/billing)

Customer benefit: the flow fits the existing pasted-key contract and retains direct control of Google billing and revocation.
Complete setup through a separate customer account remains unverified.

### Vertex Express

1. Open Google's [Vertex API-key guide](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/start/api-keys).
2. Select its Express key-acquisition link and sign in with an eligible customer account.
3. Complete Google's setup and select the required billing tier.
4. Obtain the generated key and verify that it authorizes Vertex requests.
5. Verify access to each exact model through the Express endpoint.
6. After adapter approval and implementation, paste the key into the Vertex connection.
7. Complete the same proxy validation, selection, disconnect, and revocation checks as AI Studio.

Google documents automated API setup and a generated key for Express.
The overview describes eligibility for Gmail accounts and a limited trial for new Cloud users.
Existing Cloud users use paid access. Express remains a preview offering.
The model table lists Pro 3.1 Preview, but does not list Flash 3.8 or Flash-Lite 3.5.
The same page uses Flash 3.5 in an example. These sources require live model verification.
[Express requirements and models](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/start/express-mode/overview)

The Express endpoint omits project and location segments:

```text
POST https://aiplatform.googleapis.com/v1/publishers/google/models/{model}:generateContent
```

The existing F060 route uses a different URL and OAuth credentials.
[Express API contract](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/start/express-mode/vertex-ai-express-mode-api-reference)

Customer benefit: a second independently selected Google offering can provide different model access and capacity.
That benefit remains conditional on exact-model and customer acceptance.

## Excluded Scenarios

| Scenario | Failed customer requirement |
|---|---|
| F060 operator credential profile | Requires server configuration and operator identity provisioning. |
| ADC or service-account JSON installation or upload | Requires a credential-file workflow that P012 excludes. |
| Reuse of an AI Studio key restricted to the Developer API | Does not establish authorization for Vertex. |
| Standard key presented as a Vertex authorization key | Does not establish the required service-account identity. |
| Custom customer OAuth | Lacks a demonstrated customer consent and token lifecycle. |
| Flash-Lite background interaction | The observed provider operation is unsupported. |

Google distinguishes standard keys from authorization keys bound to a service account.
Generic Vertex authorization-key creation can require an organization policy change.
The documented policy procedure requires an organization resource.
Keys restricted solely to the Gemini Developer API have a documented exception.
The managed Express acquisition flow requires its own practical verification.
[Google key types and policy](https://docs.cloud.google.com/docs/authentication/api-keys)

An OAuth proposal requires proof of consent, scopes, application verification, renewal, reconnection, revocation, and tenant isolation.
A separate customer must complete that flow before implementation approval.

## Retained Evidence

These results come from September 7 records.
The initial assessment used retained evidence. The subsequent verification used the existing billed-project key, as recorded below.

| Route and identity | Result | Meaning |
|---|---|---|
| Developer API, original dotenv key | HTTP 429 with zero free-tier quota | Credential/project capacity failure. |
| Developer API, different billed-project key | Six direct high-reasoning requests passed | Billing and credential selection affect the result. |
| Flash-Lite background interaction | HTTP 400 | Operation mismatch, independent of quota. |
| Vertex Express, original key | HTTP 401 | Authentication failed. |
| Vertex Express, Developer API restricted key | HTTP 403, `API_KEY_SERVICE_BLOCKED` | Service restriction prevented access. |
| Vertex, local user OAuth | 22/22 passed | Direct transport qualification. |
| Vertex proxy, operator service account | 28/28 passed | Proxy transport qualification, not customer setup. |

P012 retains the key-comparison evidence. I251 records the original quota diagnosis.
The [I252 evidence](evidence/vertex-gemini-2026-09-07.json) and [F060 evidence](evidence/vertex-proxy-2026-09-07.json) retain successful request results.

| F060 exact model | Passed | Observed latency |
|---|---:|---:|
| `gemini-3.1-pro-preview` | 7/7 | 2.391–4.250 seconds |
| `gemini-3.8-flash` | 7/7 | 1.523–7.922 seconds |
| `gemini-3.5-flash-lite` | 8/8 | 0.452–1.178 seconds |
| `gemini-3.5-flash` | 4/4 | 1.302–3.732 seconds |
| `gemini-3.5-transcribe-preview` | 2/2 | 0.654–0.756 seconds |

These small sequential samples do not establish sustained capacity or a reliability ranking.
The API-key comparison below records current list prices and estimated token charges.
Actual customer charges and sustained capacity remain unmeasured.
Classify future failures by quota, billing, credential restriction, model availability, operation support, or transient provider error.

## Repeated Acceptance Plan

The following thresholds are proposed release gates, not provider guarantees.

1. Use a separate customer account that has no proxy-server access.
2. Record each key-acquisition step, required role, payment step, and elapsed setup time.
3. Require zero proxy-operator actions during customer setup and reconnection.
4. Record the credential type, service restriction, billing tier, quota, and exact model availability without secret values.
5. Compare Pro 3.1 Preview, Flash 3.8, and Flash-Lite 3.5 with identical inputs.
6. Use a 45-second deadline and a 4,096-token output limit for text comparisons.
7. Cover omitted reasoning and every level that the exact offering declares.
8. Cover JSON Schema output, ordered image input, audio input, and public output-limit errors.
9. Repeat each case ten times in each of three sessions across at least 24 hours.
10. Require every functional case to pass before activation.
11. Measure capacity at concurrency one, then at the declared customer target within available quota.
12. Count each first-attempt failure in the success rate. Record retries separately.
13. Require at least 99% first-attempt success and p95 latency below 45 seconds at the declared target.
14. Record sample counts, p50/p95/maximum latency, throughput, failures, token usage, and actual charges by offering.
15. Import exact provider prices and compare cost per accepted request with equal fixtures.
16. Repeat connection replacement, disconnect, Google revocation, and cross-tenant isolation through public management APIs.

Do a synchronous test of Flash-Lite.
For any required background offering, verify creation, retrieval, completion, cancellation, and deletion independently.
Include Flash 3.5 and both public dictation endpoints before proposing migration of those existing routes.
The distinct dictation IDs require separate offering records and acceptance.

The user selected 10 concurrent requests on September 7, 2026.
The steady request rate remains an open decision. Record account limits for each run.
[Developer API rate limits](https://ai.google.dev/gemini-api/docs/rate-limits)

## Proposed Repository Changes

P012 authorizes this assessment only. The following changes require implementation approval after customer acceptance passes.

- Reuse P001's catalog-derived acquisition link, encrypted API-key storage, and tenant connection operation.
- Keep `gemini` and `vertex` as distinct provider identities with independent offering activation.
- Reuse the native generation codec where the accepted wire contracts agree.
- Give the selected Vertex key route one explicit endpoint and API-key authentication contract.
- Replace F060's customer-facing credential-profile requirement with the accepted key contract.
- Remove the excluded customer profile path during that implementation instead of adding automatic credential selection.
- Qualify each provider/model pair independently and import its own limits and prices.
- Keep F043 responsible for media staging. Require a separately approved ownership contract for storage credentials.
- Keep inline customer completion independent of operator-managed media storage.

The existing F060 branch changes remain transport evidence pending reconciliation.
Its operator cutover procedure does not satisfy the customer acceptance gate.

## Open Decisions And Next Evidence

The first capacity session favors Vertex for repeated bursts at the selected concurrency.
Gemini Flash 3.8 also passed its complete matrix.
Neither provider has complete separate-customer acceptance in the retained evidence.
P012 remains blocked on separate-customer setup, public proxy acceptance, and repeated capacity and actual cost evidence.
The user selected the primary Google account for the current verification.
Continue the repeated API-key comparison with that account's existing project and billing setup.
Record separate first-use acceptance as a distinct outstanding result.
Any required account-owner sign-in or payment action belongs to that customer.
The assessment must then select accepted offerings and present the bounded F060 revision for approval.

## Existing Account Verification

The user selected the primary Google account on September 7, 2026.
AI Studio showed the LLM Proxy project on Tier 1 Prepay with a positive credit balance.
The project already had an authorization API key restricted to `generativelanguage.googleapis.com`.
The verification process supplied that existing key in memory to the repository's standard acceptance commands.

| Existing-key check | Result |
|---|---|
| Pro 3.1 Preview, direct reasoning | Omitted, low, medium, and high passed. |
| Flash 3.8, direct reasoning | Omitted, low, medium, and high passed. |
| Flash-Lite 3.5, direct reasoning | Omitted, minimal, low, medium, and high passed. |
| Pro and Flash background lifecycle | Completion and cancellation suites passed. |
| Flash and Flash-Lite through the proxy | All declared reasoning cases and both image cases passed. |
| Original dotenv key control | Verification passed. Proxy completion timed out after 45 seconds. |

All three `make test-live-gemini-candidate` runs passed.
Both model selections passed `make test-live-provider-candidate` and `make test-live-provider-candidate-media`.
The original-key `make test-live-providers` run exited with code 2 after `curl` error 28.
The [sanitized results](evidence/gemini-customer-existing-account-2026-09-07.json) record these outcomes and their limits.
These earlier runs exclude structured output, audio, sustained capacity, and cost comparison.
The later direct API-key comparison below adds structured output, audio, and estimated token charges.

The existing key's service restriction does not permit Vertex access.
The Cloud console accepted a proposed Vertex key form with the existing Vertex service account.
The user approved creation of `P012 Vertex acceptance` with that identity and API restriction.
The browser connection failed, so the Google CLI created the key.
The CLI included the initial key value in diagnostic output. That key was immediately revoked.
Replacement creation captured all raw output. The replacement has the approved identity and Vertex-only restriction.
Its value was supplied to acceptance requests in memory.
The effective organization policy read failed because `orgpolicy.googleapis.com` was disabled.
Key creation succeeded despite that separate policy-read failure.

A separate account reached AI Studio's first-use terms page before the user selected the primary account.
Google's Express link sent that separate account to a Cloud payment-profile form.
The primary account's Express link opened its existing billing overview.
These observations do not establish a completed independent Express signup.

## Direct API-Key Comparison

The existing Gemini key and the new Vertex key used the same project on September 7, 2026.
Each request used the same input, a 45-second deadline, and a 4,096-token output limit.
Text cases covered omitted reasoning and each declared level.
Each model also received one JSON Schema request, one image, and one audio clip.
The [comparison evidence](evidence/gemini-vertex-api-key-comparison-2026-09-07.json) retains all initial results and corrected requests.

| Exact model | Gemini passed | Vertex passed | Gemini median / maximum | Vertex median / maximum |
|---|---:|---:|---:|---:|
| `gemini-3.1-pro-preview` | 7/7 | 7/7 | 3.205 / 6.053 seconds | 2.690 / 4.265 seconds |
| `gemini-3.8-flash` | 7/7 | 7/7 | 1.328 / 5.831 seconds | 1.721 / 2.003 seconds |
| `gemini-3.5-flash-lite` | 8/8 | 8/8 | 0.762 / 1.065 seconds | 0.744 / 1.048 seconds |

The initial run passed 41 of 44 requests.
Three Gemini schema requests returned HTTP 400 because the diagnostic script supplied an array instead of an object.
The corrected requests all passed. The complete comparison used 47 requests for 44 logical cases.
The correction changed only the diagnostic request format. Runtime code did not change.
The Gemini contract defines `generationConfig.responseFormat` as an object. [Generation API reference](https://ai.google.dev/api/generate-content#ResponseFormatConfig)

No comparison request returned a quota error or a transient provider error.
These results establish direct API-key access to all three exact models through both providers.
They do not establish public proxy acceptance of the Vertex key or sustained reliability.
The original zero-quota key and Flash-Lite background operation remain separate failure causes.
The billed Gemini key resolves the access failure in these tests without a provider change.

### Comparable List Prices

Both providers list the same Standard token rates for these inputs as of September 7, 2026.
The Vertex rates apply to the global endpoint. Pro inputs in this table have at most 200,000 tokens.
Output prices include reasoning tokens.
[Gemini pricing](https://ai.google.dev/gemini-api/docs/pricing) and [Vertex pricing](https://cloud.google.com/gemini-enterprise-agent-platform/generative-ai/pricing) specify these rates.

| Exact model | Input USD per million tokens | Output USD per million tokens |
|---|---:|---:|
| `gemini-3.1-pro-preview` | 2.00 | 12.00 |
| `gemini-3.8-flash` | 0.75 | 3.75 |
| `gemini-3.5-flash-lite` | 0.30 | 2.50 |

The Flash 3.8 rates apply through December 31, 2026.
Its listed rates increase to USD 1.50 input and USD 7.50 output on January 1, 2027.
The 22 accepted requests cost an estimated USD 0.02214920 through Gemini and USD 0.01991535 through Vertex.
These estimates use returned token counts. They exclude earlier probes and rejected requests.
Actual invoice charges remain unverified. Output token variation prevents a cost ranking from this single sample.

### Remaining Acceptance

1. Complete independent key acquisition with a separate customer account.
2. Complete P012's repeated capacity comparison and record actual charges at the agreed customer load.
3. Approve the bounded F060 revision after the customer evidence satisfies the P012 gate.
4. Replace the customer credential-profile contract with one Vertex API-key field and the tested endpoint.
5. Verify public connection creation, model selection, replacement, disconnect, revocation, and tenant isolation.
6. Complete proxy reasoning, structured output, ordered media, output-limit, and required operation acceptance.

P001 retains ownership of the shared connection interface.
The Vertex key remains available in the selected Cloud project for the next accepted test.
The existing runtime authentication and production configuration remain unchanged by this comparison.

## Saved Key And First Capacity Session

The user supplied the billed key and requested storage in `configs/.env` as `AI_STUDIO_API_KEY`.
The key matched the previously qualified billed key.
The local `GEMINI_API_KEY` assignment now selects that same value through the existing provider environment contract.
Other environment assignments remain unchanged. Production configuration remains unchanged.

The [saved-key evidence](evidence/gemini-saved-key-acceptance-2026-09-07.json) records the native acceptance commands.
The Gemini 3.5 Flash default passed proxy key verification, text completion, and image input.
Flash 3.8 and Flash-Lite passed proxy reasoning and image acceptance.
All three direct model suites passed. Pro and Flash also passed their supported background lifecycle checks.

The first Flash 3.8 proxy verification returned HTTP 422 with `provider_key_rejected`.
Its repeat passed. Exact direct verification requests and a 10-request verification burst also passed.
The original upstream cause remains unknown. The evidence retains the initial failure separately.
Pro 3.1 has no Gemini provider offering in the current catalog, so its proxy candidate command stopped before a provider request.

### Capacity Results

The first session used synchronized batches of 10 direct requests without an explicit request-rate limit.
Each complete matrix repeats every reasoning, schema, image, and audio case ten times.
The [capacity evidence](evidence/gemini-vertex-capacity-2026-09-07-session-1.json) records all 370 requests, including ten failures.

| Provider | Exact model | Passed / attempted | Result |
|---|---|---:|---|
| Gemini | `gemini-3.1-pro-preview` | 21/30 | Nine HTTP 429 responses stopped this offering. |
| Gemini | `gemini-3.8-flash` | 70/70 | Complete matrix passed. |
| Gemini | `gemini-3.5-flash-lite` | 29/30 | One malformed response stopped this offering. |
| Vertex | `gemini-3.1-pro-preview` | 90/90 | Complete matrix and 20 initial requests passed. |
| Vertex | `gemini-3.8-flash` | 70/70 | Complete matrix passed. |
| Vertex | `gemini-3.5-flash-lite` | 80/80 | Complete matrix passed. |

Vertex passed its complete 220-request matrix and all 20 additional requests from the interrupted comparison.
Successful Vertex requests had a maximum latency of 5.580 seconds.
The sample does not establish sustained reliability across 24 hours.

Gemini Pro reported `generativelanguage.googleapis.com/generate_requests_per_model` with a limit of 25 requests per minute.
That paid-project limit differs from the original zero free-tier quota.
Ten simultaneous requests passed before repeated batches exhausted the shared quota window.
Earlier functional requests and other project traffic can consume the same quota.
Google applies these limits per project and model. [Rate-limit requirements](https://ai.google.dev/gemini-api/docs/rate-limits)

Gemini Flash-Lite returned HTTP 200 with `MALFORMED_RESPONSE` at low reasoning.
It returned no accepted answer. This is an output failure, not a credential or quota rejection.
The remaining Flash-Lite cases did not run in that session.

The estimated token charge for all 370 requests is USD 0.28469825.
This estimate includes the malformed HTTP 200 response and uses the listed rates above.
Actual invoice charges remain unverified.

### Follow-Up Work

Two automated sessions will run at 12-hour intervals and retain their results separately.
They test Vertex Pro, Flash, and Flash-Lite, plus Gemini Flash and Flash-Lite, at concurrency 10.
Gemini Pro remains excluded until its quota or the intended request rate is resolved.
The automation pauses after the third session.
Its local runner is `tmp/p012/capacity-session.py`, with a retained audio fixture in the same ignored directory.

For Gemini Pro, select an intended request rate within the current quota or obtain a higher project quota.
For Vertex, complete the remaining time-separated sessions and independent customer setup before the bounded F060 API-key revision.
Keep the original failures in the support decision and all aggregate success counts.
