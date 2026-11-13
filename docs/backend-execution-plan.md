# Back-End Execution Plan

## Decision and scope

The review date is November 13, 2026. P019 records this review.
The review covers all 99 entries that were unresolved or taken at the start of this task.
The tables include mixed tasks and identify tasks outside the back-end scope.
Issue status is separate from plan readiness.
Recorded past test results require confirmation before issue closure.

Alibaba Model Studio access is the first provider priority.
The user selected Qwen 3.8 and Qwen 4 as the target generations after the initial review.
F050 records the completed repository implementation. Qwen 3.8 Flash passed qualification through the configured Singapore workspace.
F095 records production activation separately from the repository implementation.
F094 records Qwen 4 access and production readiness. F093 is a deferred cost alternative.
Optimize price within the requested generations. F064 supplies the regional inventory work.
I038, I226, and F095 retain their separate live qualification requirements.
The current `qwen-plus` tenant default stays unchanged until an explicit model selection.

Singapore already has the transport and connection implementation needed for qualification.
US East offers lower listed Global rates but still requires regional route work and workspace evidence.
This sequence minimizes the implementation required before the first qualified low-cost route.

## Production readiness on November 13, 2026

| Target | Verified evidence | Remaining production work |
| --- | --- | --- |
| Qwen 3.8 Flash | Account verification, omitted and four explicit reasoning cases, ordered PNG/JPEG input, and measured usage passed. Flash is enabled in source; final CI passed all 15 gates at 100.0% Go coverage and independent review is clear. | Commit and publish the selected release, deploy, and verify the TellTale production route under F095. |
| Other Qwen 3.8 offerings | Four disabled catalog candidates have local protocol coverage. | Obtain the required model grants and complete exact-model qualification before activation. |
| Qwen 4 | Alibaba documents `qwen4-max-preview` through Token Plan only. It is absent from our catalog and pay-as-you-go inventory. | Establish a provider-supported production access contract, eligible credentials, implementation, qualification, and production acceptance under F094. |

The public production catalog returned HTTP 200 and no Qwen 3.8 or Qwen 4 entries during this check.
Source: [production capabilities](https://llm-proxy-api.mprlab.com/api/public/capabilities).
The source catalog now enables Flash and retains the other four disabled candidates.
Local qualification and source activation establish separate evidence from production model availability.
The [production runbook](qwen-flash-production.md) records the selected TellTale tenant and remaining operations.

Alibaba's current Qwen 4 offering is `qwen4-max-preview`.
It requires Token Plan credentials and a separate endpoint. The existing pay-as-you-go key does not supply this access.
Alibaba schedules the preview to retire after its November 25, 2026 preview period.
The service usage contract must support the intended application backend before integration or production selection.
Sources: [Qwen 4 model guide](https://www.alibabacloud.com/help/en/model-studio/qwen4-max-preview)
and [Token Plan overview](https://www.alibabacloud.com/help/en/model-studio/token-plan-overview).

## Provider recommendation on November 13, 2026

The user selected Alibaba for production on November 13, 2026.
Qwen 3.8 Flash qualification passed on the configured workspace.
The remaining production work does not establish an Alibaba availability failure.
Alibaba recommends the workspace endpoint for production and publishes a 99.9% SLA.
The inference SLA applies to eligible paid inference and excludes Token Plan subscriptions.
Sources: [regional endpoint comparison](https://www.alibabacloud.com/help/en/model-studio/regions)
and [model inference SLA](https://www.alibabacloud.com/help/en/legal/latest/alibaba-cloud-model-studio-model-inference-service-level-agreement).

Together is a future alternative to evaluate for the exact Qwen 3.8 Flash offering.
Its published serverless selector is `Qwen/Qwen3.8-Flash`, with a 1M-token context and $0.15 input/$0.47 output per million tokens.
These rates match the configured Alibaba Singapore offering before discounts.
Novita also lists `qwen/qwen3.8-flash` at those token rates.
Neither listing establishes a higher availability commitment or an independent inference failure domain.
Confirm upstream independence, applicable availability terms, quotas, and support before treating either as a resilience improvement.
Sources: [Together Flash](https://www.together.ai/models/qwen3-8-flash)
and [Novita Flash](https://novita.ai/models/model-detail/qwen-qwen3.8-flash).

Fireworks is a separate option when reserved throughput or dedicated capacity is required.
It offers `accounts/fireworks/models/qwen3p8-max` with dedicated deployment support and links the open `Qwen3.8-2.4T-A95B` checkpoint.
That offering requires its own qualification; the listing does not establish equivalence with Alibaba's proprietary Qwen 3.8 Max.
Fireworks publishes 99.9% uptime terms for reserved throughput and dedicated deployments.
Its serverless service has no latency or availability SLA.
Obtain the exact eligible deployment terms and total capacity price before selecting this option.
Sources: [Fireworks model](https://fireworks.ai/models/fireworks/qwen3p8-max),
[enterprise terms](https://fireworks.ai/solutions/for-enterprise),
and [serverless service levels](https://docs.fireworks.ai/faq-new/deployment-infrastructure/is-latency-guaranteed-for-serverless-models).

Qualify any additional route against the same text, reasoning, media, usage, and task-quality matrix.
Measure errors, rate limits, p50 and p95 latency, and actual cost under representative concurrency before changing the selected provider.
The recommendation proposes explicitly selected provider offerings. It does not authorize a provider purchase or model activation.
The providers inspected did not establish a durable Qwen 4 production route; F094 retains that external prerequisite.

## Initial cost comparison

The initial review proposed Qwen 3.7 Flash from its lower list cost.
The user subsequently selected Qwen 3.8 and Qwen 4 as the production targets.
The following comparison remains price evidence for the deferred F093 task.

The following USD rates are per million tokens, without cache discounts or free quota.
Alibaba's official price page was checked on November 13, 2026.
The example uses 10,000 input tokens and 1,000 billed output tokens.

| Exact model | Access region and scope | Input tier | Input rate | Output rate | Example cost |
| --- | --- | --- | ---: | ---: | ---: |
| `qwen3.7-flash` | Singapore, International | At most 32K input tokens | $0.030 | $0.130 | $0.00043 |
| `qwen3.7-flash` | US East, Global | At most 32K input tokens | $0.028 | $0.110 | $0.00039 |
| `qwen3.8-flash` | Singapore, International | At most 1M input tokens | $0.150 | $0.470 | $0.00197 |
| `qwen3.8-flash` | US East, Global | At most 1M input tokens | $0.113 | $0.382 | $0.001512 |

Singapore `qwen3.7-flash` costs $0.10/$0.40 above 32K through 256K input tokens.
It costs $0.20/$0.80 above 256K through 1M input tokens.
Its first-tier example costs approximately 4.6 times less than Singapore `qwen3.8-flash`.
Prices alone do not establish task quality or account access.
This comparison does not establish the lowest price across every provider or workload.

Sources: [regional prices](https://www.alibabacloud.com/help/en/model-studio/model-pricing),
[Qwen 3.7 Flash](https://www.alibabacloud.com/help/en/model-studio/qwen3-7-flash),
and [Responses API](https://www.alibabacloud.com/help/en/model-studio/qwen-api-via-openai-responses).
SiliconFlow's [price list](https://www.siliconflow.com/pricing) does not establish an equivalent Flash offering for this comparison.

## Execution order

1. F050 repository implementation, Flash qualification, final CI, and independent review are complete.
2. Complete committed-source release, publication, operator deployment, and TellTale production acceptance under F095 through the Flash runbook.
3. Obtain model grants and qualify the other requested Qwen 3.8 offerings before their activation.
4. Establish Qwen 4 production access, eligibility, and model lifecycle under F094.
5. Implement and qualify the supported Qwen 4 contract after those prerequisites are met.
6. Compare quality, measured usage, and conditional cost within the requested model generations.
7. Complete F064 regional restrictions and US East acceptance, then F036 price comparison.
8. Resolve remaining back-end defects and continue client, tenant, and provider work in dependency order.
9. Return to F093 if the user selects Qwen 3.7 as an additional cost alternative.

The new user priority controls this provider sequence.
The media migration retains its internal I274 dependency order.
Financial defects B288 through B291 remain release acceptance work for the affected financial paths.
Each implementation issue requires its own failing public test before production edits.
The final stack checkpoint requires `make ci` after the last application change.

## F064 implementation milestones

| Milestone | Required result | State at review |
| --- | --- | --- |
| Inventory export | Separate complete inference and deployment queries, with exact upstream metadata | Independent review and all 15 final CI gates passed. |
| Workspace evidence | Authenticated inventory, account identity, access region, and service deployment scope | Singapore account and inventory verified. US East and scope evidence remain incomplete. |
| Catalog proposal | Each model has a supported route or an explicit implementation gap | Pending workspace evidence |
| Regional routes | Typed restrictions across connections, admission, discovery, and saved defaults | Detailed issue plan exists |
| Live qualification | Exact model, protocol, controls, usage, and declared media evidence | Qwen 3.8 Flash text, all distinct reasoning levels, ordered PNG/JPEG input, and measured usage passed. Other model qualification remains unresolved. |

The command is `llm-proxy inventory-dashscope`.
It reads `DASHSCOPE_API_KEY` and `DASHSCOPE_BASE_URL` from the process.
It accepts the canonical Singapore or US East workspace URL.
It collects separate `supports=inference` and `supports=deploy` inventories.
It emits one JSON document after both collections pass validation.
The export preserves upstream price strings and unknown metadata.

The workspace service deployment scope remains unknown until separately established.
The export does not change the runtime catalog.

Current [regional guidance](https://www.alibabacloud.com/help/en/model-studio/regions) uses workspaces to separate US deployment scopes.
The [Models API page](https://www.alibabacloud.com/help/en/model-studio/list-models) still shows a shared US hostname.
The command selects the workspace host from the regional contract.
US East inventory acceptance requires a live result from that host.
Current workspace evidence must determine model identifiers, including any regional suffix.
F064's September investigation remains historical evidence for those identifiers.

## Back-end issue readiness

Ready means that the issue specifies executable deliverables and public acceptance checks.
Conditional means that the plan requires the named input, dependency, or decision.
Decision means that the issue requires further analysis before implementation.
Past implementation results do not establish the current checkout's acceptance.

### Bug fixes

| Issues | Plan state | Required next result |
| --- | --- | --- |
| B288, B289, B290, B291 | Ready | Confirm financial receipts, retry boundaries, media recovery, and image usage through public tests. |
| B292, B293 | Ready | Confirm fixture corrections and complete the shared stack checkpoint. |
| B258 | Ready | Reject unsupported Dictator languages and preserve typed failure evidence. |
| B311 | Conditional | Capture the intermittent Python build stall before a cause-specific correction. |
| B316 | Conditional | Apply the readiness correction to the synchronized production source before production acceptance. |
| B210 | Conditional | Obtain the published TAuth image with the current OAuth configuration contract. |
| B207 | Outside scope | Shared UI publication and production login acceptance belong to the UI work. |

### Improvements

| Issues | Plan state | Required next result |
| --- | --- | --- |
| I038, I226 | Conditional | Complete the route matrix with additional model grants. The approved key covers three Qwen models. |
| I288, I289, I290, I291, I292, I293 | Ready | Complete each official-client contract and its public acceptance. |
| I294 | Conditional | Complete I288 through I293 before external reference acceptance. |
| I296 | Ready | Execute Goose qualification with the specified real client build. |
| I295 | Conditional | Recheck the recorded Governor and deployment blockers before integration publication. |
| I314 | Conditional | Apply and verify the retained pilot recovery, then remove its temporary source bridge. |
| I286 | Ready | Establish provider ownership for the MediaOps and TelePrompter split. |
| I274, I287 | Conditional | Complete their explicit media capability dependencies in order. |
| I244 | Conditional | Obtain the MediaOps I088 migration receipt before removal of the import bridge. |
| I278 | Conditional | Complete I277 and F081 through F083 before combined tenant acceptance. |
| I271 | Conditional | Record production connection transfer and the local predecessor database disposition. |
| I255 | Ready | Separate Gemini model-operation errors from credential rejection. |
| I035 | Ready | Persist the account-owned Usage interval and confirm account isolation and UI hydration. |
| I241 | Ready | Add request-bucket aggregation, its API contract, and provider chart acceptance. |
| I234 | Conditional | Establish nonzero provider quota before the remaining Gemini live checks. |
| I228, I227, I225 | Conditional | Supply the relevant provider access and complete the declared live matrices. |
| I041 | Conditional | Complete live acceptance for the recorded xAI Responses routes. |
| I210 | Conditional | Confirm current Meta inputs and complete live discovery and text acceptance. |
| I260, I277, I218 | Outside scope | Shared UI, tenant dialog, and public resource pages have separate interface work. |
| I280 | Outside scope | Domain purchase and migration are separate operations. |

### Features

| Issues | Plan state | Required next result |
| --- | --- | --- |
| F064 | Conditional | Complete the available inventory tooling before authenticated regional qualification. |
| F050 | Complete | Five models are implemented; Flash is qualified and enabled in source. F095 records remaining operational work. |
| F036 | Ready | Implement typed price conditions, freshness checks, and comparable public results. |
| F059 | Conditional | Establish SiliconFlow access and resolve conflicting Kimi and LongCat limits. |
| F090 | Decision | Select the initial Bedrock models, regions, verification charges, and applicable provider terms. |
| F086 | Decision | Select task fixtures, the comparison workflow, and acceptance thresholds. |
| F080 | Decision | Resolve the exact Moonshot search API and public response contract. |
| F075 | Decision | Resolve the listed Anthropic protocol and route decisions. |
| F087 | Conditional | Establish the Paddle sandbox account and processor qualification inputs. |
| F081 | Decision | Complete I277 and select the transition from digest-only keys to retained secrets. |
| F082, F083 | Conditional | Complete F081 before configuration errors and connection attribution in dependency order. |
| F072, F073 | Ready | Expose granular Dictator models before the capability taxonomy change. |
| F061 | Conditional | Complete the recorded F060 formatting correction and rerun final CI. |
| F060 | Conditional | Complete the documented Vertex credential, quota, and acceptance investigations. |
| F051, F052, F049, F048, F047, F045 | Conditional | Complete each exact provider and model qualification matrix. |
| F054 | Conditional | Confirm Meta provider access before native transcription qualification. |
| F055, F056, F057, F058 | Conditional | Complete their capability dependencies and provider acceptance. |
| F040, F042, F043 | Conditional | Complete staging and Dictator dependencies, then confirm recorded progress through current acceptance. |
| F025, F026, F027 | Conditional | Complete their explicit staging, ownership, and provider requirements. |
| F028, F029, F030 | Conditional | Complete F027, F025, and F026 respectively before additional provider acceptance. |
| F016, F021 | Ready | Implement the specified Node client and OAuth MCP contracts. |
| F020 | Conditional | Complete F016 and the coordinated canonical v3 client changes. |
| F017 | Outside scope | Shared UI inactivity and logout belong to the interface work. |

### Maintenance and planning

| Issues | Plan state | Required next result |
| --- | --- | --- |
| M023R, M004R, M005R, M006R, M007R | Recurring | Execute bounded integration, dependency, CI, source, and production checks when selected. |
| M001R, M002R, M003R, M008R | Recurring | Complete selected backlog, architecture, and documentation reviews without closing recurring records. |
| P008 | Decision | Select the private controller contract, runtime ownership, resource policies, and SAM interface. |
| P012 | Conditional | Complete customer Gemini access and quota investigations. |
| P006 | Decision | Obtain product, service, and legal decisions for provider lifecycle and SLA terms. |
| P014 | Ready for research | Complete the bounded Seedance and HeyGen assessment. |
| P015, P016, P017 | Conditional | Select evaluation ownership and thresholds, then complete comparisons before the product decision. |
| P003 | Outside scope | SEO resource strategy has a separate analysis scope. |

Planning entries authorize their analysis deliverables only.
Their implementation requires a separate implementation issue or explicit user instruction.

## Decision preparation

The following procedures complete the missing decisions before the affected implementation starts.
They retain each issue's current capability scope.

### F090: Bedrock connection

1. Record official Gemma 4 model identifiers, supported AWS regions, prices, and provider terms.
2. Select the least expensive eligible model for the declared task and account region.
3. Record the MPR Lab principal, customer role, external ID, and minimum inference permissions.
4. Separate role verification from any billable model request in the connection procedure.
5. Obtain independent review of tenant isolation, role trust, temporary credentials, and revocation.
6. Add failing public connection and inference scenarios before the catalog and authentication implementation.

### F086: Tenant comparison

1. Define tenant-supplied examples and expected results for one task.
2. Record automatic checks, tenant judgments, request limits, repeat counts, and result retention.
3. Establish the task acceptance threshold before execution.
4. Compare the current route with one candidate through identical supported settings.
5. Record billed tokens, errors, response times, and conditional cost estimates separately from confirmed charges.
6. Recommend the least expensive route that meets the declared task threshold.

### F080: Moonshot search

1. Examine the current official search API and its provider-specific charges.
2. Record the exact request, response, citation, error, and usage fields.
3. Define the public tool interaction and per-request search limits.
4. Add local HTTP scenarios for enabled search, disabled search, errors, and cancellation.
5. Implement the selected contract and complete provider qualification separately.

### F075: Anthropic client protocol

1. Select one supported `anthropic-version` and an explicit beta-header policy.
2. Select the first content-block set and structured-output representation.
3. Define the public result when required upstream usage is unavailable.
4. Confirm buffered SSE for routes without native streaming.
5. Record the first-release endpoint scope in OpenAPI and the client guide.
6. Obtain independent contract review before public protocol tests and implementation.

### F081 and I277: Tenant keys

1. Select the tenant dialog's explicit connection and default-model behavior under I277.
2. Record the retained-secret encryption and account authorization contract under F081.
3. Define replacement of existing digest-only keys, which cannot supply the original secret.
4. Specify the bounded data transition and remove its bridge after completion.
5. Obtain independent security-boundary review before implementation.
6. Prove key retrieval, replacement, account isolation, reload, and safe errors through public tests.

### P008 and P006: Architecture and service policy

1. For P008, assign the controller owner and select one private control protocol.
2. Define runtime profiles, queue fairness, idle policy, and the SAM public operation.
3. Confirm software acceptance through emulators, local protocol servers, and automated tests.
4. For P006, assign product, service, and legal decision owners.
5. Define measurable provider qualification, retirement, availability, support, and service-credit terms.
6. Record approved decisions before creating the affected implementation issues.

## Account preparation and resume point

The initial check covered the process, `configs/.env`, and `configs/.env.local` for the declared provider fields.
`DASHSCOPE_API_KEY`, `DASHSCOPE_BASE_URL`, and `SILICONFLOW_API_KEY` were absent at the start of the review.
No upstream request ran during that initial input check.
Existing TypeSafe changes in the primary checkout belong to separate work.

The user later signed in and approved a scoped Alibaba key on November 13, 2026.
The private `configs/.env` now contains the replacement key and Singapore workspace URL.
The key permits `qwen-plus`, `qwen3.7-flash`, and `qwen3.8-flash` from any IPv4 source.
Qwen Plus supplies the proxy's current connection-verification route.
The account and workspace identifiers are in `.mprlab/evidence/F064-inventory-20261113.json`.

The authenticated export contains 186 inference models across 10 pages and 33 deployment models across two pages.
Its raw metadata is in the ignored `_scratch/F064-dashscope-inventory-20261113.json` file.
Inventory membership does not establish permission to generate through every returned model.
The first proxy smoke stopped before dispatch because the workspace origin lacked an explicit admission allocation.
The reviewed configuration adds four active slots and 24 queued slots for that exact origin.

Global admitted capacity is 708. The global active limit remains 32.

The repeated proxy verification and Qwen 3.8 Flash text smoke both returned HTTP 200.
The live command used `make test-live-provider-candidate` with `LIVE_CANDIDATE_MODEL=dashscope/qwen3.8-flash`.
It loaded the approved private input through `LIVE_ENV_FILE=configs/.env`.
This result establishes a text smoke through the disposable proxy.
The later F050 continuation passed reasoning, ordered PNG/JPEG input, and measured usage qualification, then enabled Flash in source.
The [Flash record](../.mprlab/evidence/F050-flash-20261113.json) retains that acceptance scope.
Production publication, deployment, connection assignment, and tenant acceptance retain their own requirements.
The temporary Chrome tab was closed after the saved-key record was verified.

Before an authentication handoff, identify the Alibaba account owner, workspace ID, access region, and service deployment scope.
Confirm the account's Model Studio permissions and any required MFA session.
Use the [English API-key guide](https://www.alibabacloud.com/help/en/model-studio/get-api-key) for the selected regional workspace.
Keep the key in the approved private input source.

For another inventory export, supply the two DashScope process inputs and run `make inventory-dashscope`.
Save the JSON output in the approved private evidence location.
Compare its complete inference inventory with the selected workspace console.
Resolve the service deployment scope before regional route activation.

For `qwen3.7-flash`, run the candidate harness after F093 supplies the disabled model and its exact acceptance matrix.
Save model identifiers, request IDs, account identifiers, and results without secret values or private content.
If a query fails, correct its reported input or provider error before another run.

F064 remains open until its full catalog, regional route, and qualification requirements pass.
