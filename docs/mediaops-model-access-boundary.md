# MediaOps And TelePrompter Provider Boundary

## Current Decision

LLM Proxy owns all media-provider API access.
MediaOps provides one backend for its public API, workflows, application data, and processing.
TelePrompter is a separate timeline and prompt website connected to MediaOps.
This decision replaces the 2026-09-19 requirement to keep the TelePrompter website inside MediaOps.

Frame Picker stays in MediaOps, including thumbnails, hover zoom, storage, and frame extraction.
Other retained MediaOps interfaces and YouTube authorization stay with MediaOps.
Dictator remains a private provider runtime behind LLM Proxy.

## Responsibility

| Responsibility | Owner |
| --- | --- |
| Provider credentials, catalog, native requests, resource APIs, uploads, and recovery | LLM Proxy |
| Provider staging, durable provider operations, tenant assets, and usage | LLM Proxy |
| Projects, application assets, authorization, product jobs, and spend authority | MediaOps service |
| Narration, composition, FFmpeg, inspection, preview, export, and validation | MediaOps service |
| Timeline editing, prompt controls, and presentation | TelePrompter website |
| Frame Picker and retained auxiliary interfaces | MediaOps |

Construct the official gateway client in the MediaOps backend.
Keep gateway secrets outside browser, CLI, and MCP payloads.
Make those product clients use the MediaOps API for workflow execution.
Keep application asset retention independent of gateway temporary assets.
Keep provider metrics in LLM Proxy and route health in MediaOps.

## Executable Umbrellas

- I274 owns the retained-provider migration through its executable dependencies.
- I286 establishes the current boundary before provider cutovers. It replaces F071.
- MediaOps I093 owns the single backend and provider consumer changes.
- MediaOps I094 owns the TelePrompter website extraction.
- MediaOps I092 owns final cross-product acceptance.

The full [execution table](https://github.com/MarcoPoloResearchLab/MediaOps/blob/tyemirov/media-migration-backlog/docs/public-sdk-contract.md) records the coordinated order.
The local [provider inventory](media-provider-completeness.md) defines retained capability coverage.
The [consolidation contract](media-gateway-consolidation.md) defines gateway lifecycle and provider protocol requirements.

| Gateway contract | MediaOps consumer | Required result |
| --- | --- | --- |
| F024, F039 | I084 | OpenAI image generation, editing, chains, and progressive output. |
| F072, F042 | I087 | Dictator speech, discovery, voices, recovery, and exact artifacts. |
| F043, F040 | I089, I085 | Provider staging and Vertex images. |
| F041, plus F043 where required | I086 | FAL images. |
| F025 | I010 | Runway, Vertex, FAL, Kling, and xAI video. |
| F026 | I011, B024 | ElevenLabs speech, music, alignment, voices, dictionaries, history, and account resources. |
| F027 | I012 | HeyGen and Kling mutations, avatars, translation, lip-sync, and resources. |
| I244 | I088 receipt is the prerequisite | Remove bounded import tooling after source reconciliation. |

Hand accepted capability contracts to MediaOps before I274 closes.
Require MediaOps I088 before I244, then close I274 before final MediaOps I092 acceptance.
Keep I274 out of the prerequisites for MediaOps provider cutovers.
This ordering prevents a cross-repository dependency cycle.

## Evidence And Boundaries

Preserve dated implementation receipts. Verify current source before implementing missing work.
Use public gateway tests for provider protocols and MediaOps tests for product workflows.
Use TelePrompter automated browser tests for the website.
Record source qualification, official client publication, runtime activation, and paid acceptance separately.

Keep application records and accepted assets in MediaOps.
Inventory provider records and explicit owner mappings before each bounded import.
Preserve uncertain operations for recovery without another paid submission.
MediaOps I088 supplies per-tenant receipts, including verified zero-count families.
I244 removes only migration tooling introduced for those records.

I287 groups later Avatar V, MiniMax H3, and Speechify additions.
P014 owns provider feasibility for the former MediaOps I071 proposal, now MediaOps P007.
P008 owns later local inference planning.
These issues do not block the retained-provider migration.
