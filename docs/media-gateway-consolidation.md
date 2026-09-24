# Media Gateway Consolidation

## Decision And Status

LLM Proxy becomes the shared gateway for text, images, video, speech, and other media capabilities.
All media-provider API access moves from MediaOps to LLM Proxy.
The current target separates the TelePrompter website from the single MediaOps backend.
MediaOps owns its API, workflows, application data, and processing. TelePrompter calls the MediaOps API.
Frame Picker, thumbnails, and hover zoom stay in MediaOps.
FamilyHome uses the gateway through its backend.
Dictator remains a private runtime behind the gateway.

P011 records this implementation plan on 2026-09-06.
The 2026-09-08 revision adds provider catalog, protocol adapter, and second-provider acceptance requirements.
MediaOps P006 records the original provider gateway plan.
I286 establishes the provider boundary. MediaOps I092 owns final cross-product acceptance. Provider capability issues keep their separate delivery boundaries.
The [model-access boundary](mediaops-model-access-boundary.md) records ownership and the provider issue sequence.
F022 delivered the common durable media service through controlled provider
protocols. It establishes local runtime acceptance for the shared lifecycle.
Live provider acceptance remains with each provider capability issue.

The product has one gateway API, tenant model, credential lifecycle, provider catalog, and usage view.
The gateway can contain separate adapters and workers within the same repository.
The first release extends the existing Go service and its database.
Dictator continues to run in its existing process.

```mermaid
flowchart LR
    F[FamilyHome backend] --> G[LLM Proxy gateway]
    T[TelePrompter website] --> M[MediaOps API and processing service]
    C[MediaOps CLI and MCP] --> M
    M --> G
    G --> P[Cloud providers]
    G --> D[Private Dictator runtime]
```

## Ownership And Authentication

| Concern | Owner |
| --- | --- |
| Parent login, family access, calendars, child interface, product budgets | FamilyHome |
| Timeline and prompt website | TelePrompter |
| Product API, user access, jobs, projects, assets, and client contracts | MediaOps |
| Narration orchestration, local composition, inspection, validation, review, and YouTube workflows | MediaOps |
| Frame Picker upload, storage, FFmpeg execution, download, thumbnails, and hover zoom | MediaOps |
| Tenant authentication, provider connections, routes, execution, usage | LLM Proxy |
| Speech engines, native jobs, GPU execution | Dictator |

Use an existing managed tenant and client key for each calling backend.
Use separate tenants where environments need independent credentials and usage.
An operator can manage multiple tenants through the existing management interface.
FamilyHome stores its gateway key in backend configuration.
MediaOps uses a gateway tenant for model requests through its backend and shared provider adapter.
The MediaOps service uses the official gateway client. Browser, CLI, and MCP clients use the MediaOps API.
The parent does not configure provider services.

Reuse the existing bearer authentication adapter for the new media routes.
Asset upload, metadata, content, deletion, and official client calls use that
same authentication contract.
Use the same managed key identity and replacement behavior as existing gateway clients.
Authorize every operation, asset, voice, and history read against that tenant.
Return `404` for a resource owned by another tenant.
Keep provider credentials in managed provider connections or private runtime configuration.

FamilyHome maps its own product job to a gateway operation.
It checks family access before it returns status or bytes to Android.
A gateway tenant represents a calling backend. It does not replace family or project authorization.
MediaOps keeps composition tasks and their resource authorization.
MediaOps I098 verifies local preview and export after model-generated inputs use the gateway.
I286 does not create a gateway composition API.

## Verified Source And Reuse

The LLM Proxy source baseline is `2da5d87b6b536817bf8100391feb48a3d32208a7`.
Separate, uncommitted provider-audit changes are outside this plan's delivery.
The following paths define the existing foundation or the source to move.

| Source | Required use |
| --- | --- |
| `internal/proxy/management_store.go` | Reuse managed tenants, provider connections, SQLite access, and usage ownership. |
| `internal/proxy/client_protocols.go` | Reuse bearer authentication against managed tenant keys. |
| `internal/proxy/assets.go` | Extend the existing asset identity and byte store with output reads and active references. |
| `internal/proxy/structured_requests.go` | Preserve current text behavior through integration tests before any shared extraction. |
| `internal/proxy/provider_transport.go` | Integrate media network work with I046 capacity limits. |
| `configs/providers.yml` | Extend the canonical catalog with qualified media routes and typed controls. |
| `pkg/llmproxycontract`, `docs/openapi.yaml` | Define the canonical media resources and error shapes. |
| `pkg/llmproxyclient` | Add media methods to the existing official Go client. |
| MediaOps `internal/media/provider` | Move provider request translation, native recovery, and provider-specific tests by capability. |
| MediaOps `internal/media/mcp` | Keep tool contracts in the client. Execute workflows through the MediaOps API. |
| MediaOps `internal/mediajobs/api` | Keep application jobs and local execution in MediaOps. Replace model-provider calls only. |
| MediaOps `internal/timelineexport` | Keep composition and export execution in MediaOps. |

The asset store provides authenticated upload, metadata, content, and deletion.
Durable active references prevent deletion while an operation owns an input or
output.
Its metadata uses files. Managed tenant data already uses SQLite.
The structured text store also uses files and a process-local mutex.
Its terminal cleanup and failed-request retry behavior remain separate from paid
media operations. The media operation store owns paid-operation recovery.

## Provider Catalog And Protocol Adapters

This contract applies to every migrated capability and each later provider addition.
The [provider catalog contract](provider-catalog.md) defines the current provider data and the procedure for a compatible provider addition.
The media migration extends that same catalog and registry.
The schema validates provider definitions and transport-component composition.
Executable components implement the supported API behavior.

| Layer | Owner and responsibility |
| --- | --- |
| Provider catalog | `configs/providers.yml` owns provider fields, transports, offerings, operations, controls, limits, prices, and transport-component selection. |
| Request and response codecs | Reusable code owns native request serialization, response parsing, errors, continuation, and usage translation. |
| Authentication components | Reusable code owns credential injection. Credential values remain in managed connection or private runtime storage. |
| Execution components | Reusable code owns synchronous completion, submission, observation, cancellation, and provider recovery. |
| Shared media service | F022 owns tenant authorization, operation storage, worker claims, duplicate prevention, assets, retention, and usage delivery. I046 owns network capacity. |
| Consumer services | MediaOps, FamilyHome, and other active consumers use official gateway clients for model operations. |

Keep credential values and tenant settings in their existing stores outside the catalog.
Use catalog projections for provider discovery, connection forms, capability validation, routing, and price metadata.
Compose executable routes from the declared request codec, response codec,
authentication, and execution lifecycle.
Keep provider identity as route data in the shared service.
Keep native API behavior inside reusable transport components.

Before each capability slice, record its provider offering, component
composition, supported controls, and required assets.
Classify each addition with the following table.

| Condition | Required change |
| --- | --- |
| Existing components implement the complete contract. | Add catalog data and connection values. Keep production code unchanged. Complete the second-provider acceptance procedure below. |
| The provider needs an unsupported codec variation. | Add or extend the reusable request or response codec and its strict schema contract. Then add the provider definition. |
| The provider needs a new capability or lifecycle. | Add the typed capability or shared execution lifecycle first. Then add its codec and catalog records. |

Compare authentication, request fields, response fields, controls, errors, usage, and execution lifecycle before selecting an adapter.
A shared endpoint name or an OpenAI compatibility claim does not establish that complete match.
Current catalog mappings must match implemented component contracts.
Reject unsupported component declarations and incompatible controls at the applicable startup or request boundary.
Add new behavior through typed code and its schema contract, rather than executable expressions in provider data.

### Acceptance Through A Second Provider

F022 provides the reusable acceptance harness and the shared service boundary.
Each capability issue owns this acceptance for every transport component that it adds or extends.
F024 supplies the first image proof. F039 through F043 and F025 through F027 apply the same requirement to their slices.

1. Start the real service with its YAML loader, SQLite database, filesystem, and official client.
2. Exercise one provider through a controlled implementation of the selected external protocol.
3. Add a second provider identity with the same codecs and lifecycle.
   Use a different supported authentication configuration.
4. Give the second definition distinct connection fields, endpoint values, and upstream model identifiers where the protocol permits them.
5. Supply test credentials through the existing connection contract.
6. Reload the catalog through normal service startup. Use the same service executable, clients, and component implementations for both definitions.
7. Use public HTTP and browser assertions for provider discovery, generated connection forms, and capability metadata.
8. Use public assertions for routing, accepted controls, tenant isolation, artifact downloads, and one execution usage event per operation.
9. Exercise duplicate requests, process restart, cancellation, and result recovery according to the declared protocol contract.
10. Make sure the result records uncertainty or unsupported cancellation when the remote outcome cannot be established.
11. Make sure an invalid adapter declaration stops startup. Make sure an unsupported request causes zero provider dispatch.
12. Record the catalog changes, test configuration, unchanged executable, and public test results in the slice's acceptance evidence.

The second provider requires only catalog data, connection values, and controlled test infrastructure after the components exist.
If that addition requires production changes, complete the missing shared contract before accepting the slice.
Keep fictional provider definitions in test fixtures only.
This procedure proves architectural reuse. Qualify each actual provider separately through authorized live acceptance.

## First API Contract

The first consumer submits one image request, reads its operation, and downloads its result.
Operation creation validates the request and records the selected route before dispatch.
That stored request is the execution contract.
A separate public plan resource is outside the first release.
Exact price estimates are optional catalog evidence, independent from acceptance.

All new media resources use the existing `/model/v1` namespace.
The table specifies planned additions and the asset authorization change.
F022 updated the server, OpenAPI, contract types, and official client together.

| Method and resource | Result |
| --- | --- |
| `GET /model/v1/capabilities` | Tenant-available capabilities, routes, supported controls, and limits. |
| `POST /model/v1/assets` | Existing upload identity with canonical bearer authentication and streaming input. |
| `GET /model/v1/assets/{asset_id}` | Tenant-owned metadata, state, MIME type, byte count, and expiry. |
| `GET /model/v1/assets/{asset_id}/content` | Bounded output or input byte stream. |
| `DELETE /model/v1/assets/{asset_id}` | Existing deletion with active-reference protection. |
| `POST /model/v1/operations` | `202`, an opaque operation identifier, and `Location`. Require `Idempotency-Key`. |
| `GET /model/v1/operations/{operation_id}` | Current state, output asset references, timestamps, and typed error evidence. |
| `PUT /model/v1/operations/{operation_id}/cancellation` | Idempotent cancellation request with its observed outcome. |

Use the existing asset identifier for each generated output.
An artifact is an output asset attached to an operation.
Keep ordered output references in the operation result.
Keep native provider identifiers and byte digests in private records.
The service verifies byte integrity. The client verifies the authenticated transfer and expected byte count.

Use typed capability inputs, including provider, model, prompt, controls, and asset references where required.
Reject unsupported controls before acceptance.
Return the original operation with `200` for the same tenant, key, and request intent.
Return `409` for a different intent under an accepted key.
Resolve accepted requests before current catalog validation, so later catalog changes cannot break retrieval or authorize duplicate work.
Return explicit expiry evidence when output bytes are gone.
Keep the accepted key bound to that operation.

Add typed Go methods for capability discovery, creation, status, cancellation, upload, and download.
A wait timeout must return the accepted operation identifier.
A caller disconnect must stop waiting without cancelling accepted work.
An explicit cancellation request controls cancellation.

## Persistence And Execution Decisions

F022 uses operation, claim, asset-reference, usage-delivery, and tombstone tables
in the existing managed SQLite database.
The first deployment has one API process with bounded in-process workers and the existing persistent asset volume.
The service periodically reads outstanding operations from SQLite. It queues
undispatched work without a live claim. It recovers dispatched work through the
media operation adapter after an expired claim.
Additional process replicas require their own qualification before activation.

Create tables for operations, worker claims, input references, output references, and usage delivery records.
Use a unique tenant-and-key constraint and transactional compare-and-set updates.
Keep the normalized request, selected route, catalog revision, and credential-version reference in the operation record.
Keep credential values outside that record.
A revoked credential prevents a new dispatch through that connection.
Existing provider jobs require an authorized connection to the same provider account for recovery.
The worker compares the current connection identifier and version with the accepted credential reference before dispatch or recovery.
A changed reference prevents adapter execution. Dispatched work remains uncertain when its accepted authority is unavailable.
The adapter receives the checked credential reference.
The service permits one acceptance transaction at a time.
The transaction reads capacity before it inserts the operation.

Persist acceptance and input references before dispatch.
Extend asset deletion and cleanup to consult those durable references.
Coordinate reference creation with byte publication and deletion.
Reconcile interrupted file publication through restart tests with the real database and filesystem.
Keep active inputs and staged outputs until the operation reaches its documented completion boundary.
Active input references prevent upload expiry during reads, timer cleanup, and startup cleanup.
Uncertain operations keep their active inputs. Completion or confirmed cancellation releases input references.
After release, the normal upload expiry applies. Cleanup checks retained expired assets each minute.

Persist dispatch intent before the external call.
Give each worker claim a generation number and expiry.
Require that generation for every state change.
A replacement worker can resume only work that has no dispatch intent.
After dispatch intent, use recorded provider evidence to recover the result.
A stale worker cannot authorize another attempt.

Preserve the provider execution states `not_dispatched`, `dispatched`, `succeeded`, `failed`, and `uncertain` in private evidence.
Expose operation states `queued`, `running`, `succeeded`, `failed`, `cancelled`, and `uncertain` through one closed schema.
Keep cancellation observations separate from provider execution evidence.
Confirm `cancelled` only when work stopped before dispatch or the provider confirmed cancellation.
Keep an unsupported cancellation explicit and continue to observe the provider outcome.

If a response is lost after submission, recover through the provider's documented handle or idempotency mechanism.
If the provider cannot resolve that outcome, keep the operation `uncertain`.
A repeated client request returns that same operation.
A new paid attempt requires a new product action and key.
This contract prevents automatic duplicate submissions. It cannot guarantee provider-side exactly-once execution.

Use these initial values in explicit configuration and deterministic tests:

| Setting | Initial implementation value |
| --- | --- |
| First image operation lifetime | 900 seconds from acceptance |
| Worker claim lifetime and renewal | 60 seconds and 20 seconds |
| Terminal output retention | 172800 seconds, consistent with the existing asset retention |
| Idempotency tombstone retention | Tenant lifetime |
| Accepted media operations | 32 globally and 4 per tenant |
| Active provider HTTP requests | Existing global limit of 4, with at least 1 place reserved for interactive text |

Discard terminal prompts and request bodies after the retention boundary.
Keep only the tenant, key digest, intent digest, operation identifier, and terminal classification in each tombstone.
Keep unresolved operations and their necessary evidence until reconciliation or explicit disposition.
An operation deadline ends local execution authority. It does not prove that a remote provider stopped.
Validate the selected deployment values before activation.

I046 owns bounded queues, tenant fairness, provider-origin limits, and shared provider-account limits.
Distinguish accepted operations, active HTTP requests, polls, and byte transfers.
Release active HTTP capacity between polls.
Reserve bounded progress for text even when text and images use the same upstream origin.
All network work still obeys the configured global ceiling.

Record one execution usage event per accepted operation through a durable delivery record.
Use the operation identifier to deduplicate usage writes after restart.
Status reads and downloads do not create generation charges.
Keep unknown provider cost explicit.

## Delivery Sequence

Use I274 for gateway execution and the [ownership contract](mediaops-model-access-boundary.md) for exact consumer handoffs.

1. Complete I286 and MediaOps I009 for the current boundary and common client.
2. Complete MediaOps I095, B019, F025, I096, and I099 for the single backend and its public contracts.
3. Use completed F024 and F039 for MediaOps I084 OpenAI image acceptance.
4. Complete MediaOps I097, I027, and I098 for the first TelePrompter prompt-to-export workflow.
5. Complete F072 and F042, then MediaOps I087 for Dictator.
6. Use the MediaOps I089 inventory for F043, then complete F040, MediaOps I089 acceptance, and I085.
7. Use completed F041 for MediaOps I086. Require staging only where the selected route needs it.
8. Complete F025 and MediaOps I010 in provider order: Runway, Vertex, FAL, Kling, then xAI.
9. Complete F026, then MediaOps I011 and B024 for ElevenLabs.
10. Complete F027, then MediaOps I012 for HeyGen and Kling resources.
11. Complete MediaOps B307 and I088, then I244 and I274.
12. Close MediaOps I093 and I094, then verify final acceptance under MediaOps I092.

FamilyHome owns its independent consumer acceptance. It is not a prerequisite for MediaOps integration.
I287 groups later provider additions. P008 and P014 remain planning.
Record publication, deployment, and paid acceptance separately from source completion.

## First Consumer Acceptance

FamilyHome P003 still describes Portal-to-MediaOps execution as of this review.
Revise P003 before implementing the image feature.
The current target is Android to FamilyHome backend to LLM Proxy to OpenAI.
FamilyHome retains its separate drawing activity and image-generation product decisions.

Use `provider=openai`, `model=gpt-image-2`, and `quality=low` for the first qualification.
Keep dimensions, format, prompt choices, family budgets, and simultaneous-job limits in FamilyHome configuration.
Close those P003 product decisions before Android acceptance.
Create the corresponding FamilyHome implementation issue after completion of the product plan.

1. Authenticate the parent through FamilyHome's selected family-access contract.
2. Create a FamilyHome image job with one stable product identifier.
3. Submit through the official Go client with that identifier as the gateway idempotency key.
4. Keep the returned operation identifier before the backend returns to Android.
5. Repeat the same request after a simulated lost response and verify one accepted gateway operation.
6. Retrieve the result through the FamilyHome backend after checking family access.
7. Verify that Android displays and saves the image under the selected product contract.
8. Verify rejection with a replaced key and isolation from another tenant and another family.
9. Verify one execution usage event and explicit failure or uncertainty evidence.
10. Record the service revision, client release, backend revision, device build, and provider result separately.

The complete flow proves the first product capability.
API tests alone do not establish Android or production acceptance.

## Data Migration And Source Removal

I009 starts with a read-only inventory of actual consumer workspaces and deployed stores.
Record capability, record count, schema, provider account, resource owner, target tenant, and recovery requirement.
Record retained voices, response chains, provider jobs, history references, and accepted local artifacts separately.
An empty inventory produces a zero-count receipt.

Create a bounded import tool only for records that must remain recoverable.
Require explicit tenant and provider-account mappings.
Preserve native handles without a new paid submission.
Reject incomplete mappings and conflicting ownership.
Map accepted product records to gateway resources before deleting their direct provider path.
Keep completed local artifacts in the product when further provider access is unnecessary.

Each switch produces a receipt with imported, excluded, rejected, and remaining counts.
Record source digests and gateway identifiers in private operator evidence.
I088 reconciles all receipts and proves zero remaining direct provider execution.
I244 removes only the import tools actually introduced by these migrations.
Remove obsolete adapters, provider secrets, recovery code, catalog copies, and provider tests after their new owner passes acceptance.
Move provider protocol tests with their adapters. Keep all application interaction and local-processing tests in MediaOps.

## Validation And Release Gates

F022 must prove concurrent duplicate convergence, intent conflicts, tenant isolation, and zero dispatch for invalid input or credentials.
Use the real HTTP listener, SQLite database, filesystem, and official client.
Use controlled external-provider protocols for repeatable failure tests.
Apply the [second-provider acceptance procedure](#acceptance-through-a-second-provider) to the shared harness and each affected protocol adapter.

Exercise process death before dispatch, after dispatch intent, after provider acceptance, during output transfer, and during usage delivery.
Prove stale-worker rejection and explicit uncertainty where recovery is unavailable.
Exercise byte expiry, active-reference deletion, interrupted downloads, cancellation races, and tombstone retries.
Use controlled clocks and injected boundary failures to make these cases deterministic.

Each slice has four separate evidence records: source CI, client release, service activation, and consumer acceptance.
Qualify actual providers and the Dictator runtime separately from local protocol fixtures.
An operator performs production release and deployment through the existing repository lifecycle.
Record a failed acceptance against its owning slice before the next capability switch.

## Product Position And Remaining Decisions

Position the combined service as one gateway for AI capabilities.
MediaOps keeps application data and processing. TelePrompter owns its website. I286 establishes the provider boundary.
Keep the existing LLM Proxy name through the first consumer release.
A broader brand is a later product decision.

Before external gateway positioning, compare fal, Replicate, and Eden AI on the same representative workloads.
Measure supported controls, result quality, latency, failure recovery, operating cost, and integration effort.
Evaluate an aggregator as an upstream provider where it improves those outcomes.
A unified endpoint by itself is insufficient evidence of a product advantage.
Internal consolidation has an immediate purpose: one maintained tenant and provider boundary for our applications.

The remaining decisions have explicit owners:

- FamilyHome P003: image format, dimensions, input method, save behavior, family limits, and its implementation issue.
- F043: exact staging routes, Google credential mode, and provider-readable URL lifetime.
- F042: capability schemas and the disposition of existing transcription routes before its public release.
- I286: final model-access boundary and affected caller inventory.
- MediaOps I098: local preview and export acceptance after model-provider cutovers.
- Each activation: credentials, runtime values, retained-data inventory, and measured provider acceptance.

F028, F029, and F030 cover later AvatarV, MiniMax, and Speechify integrations.
MediaOps F022, F023, and F024 keep their product exposure requirements. Provider capability delivery belongs to LLM Proxy.
MediaOps I027, I030, and I031 remain creator workflow work in MediaOps. P003 and P005 remain planning work.
I286 checks that provider cutovers leave these product responsibilities in MediaOps.
MediaOps I089 supplies acceptance evidence for provider input staging under F043. It does not move local media execution.
