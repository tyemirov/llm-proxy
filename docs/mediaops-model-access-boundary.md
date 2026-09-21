# MediaOps Model Access Boundary

## Confirmed Scope

On 2026-09-19, the operator limited this migration to model and provider access.
This decision replaces the earlier proposal to move MediaOps applications into LLM Proxy.

MediaOps keeps all applications, browser interfaces, CLI and MCP product operations, local processing, and application data.
LLM Proxy owns model access, provider connections, provider requests, provider operation recovery, and provider artifacts.
Dictator remains a private provider runtime behind LLM Proxy.

Frame Picker stays entirely in MediaOps.
Its thumbnail strips, hover zoom, uploads, storage, FFmpeg worker, and downloads are outside this migration.
The proposed LLM Proxy Frame Picker implementation was removed before publication or activation.

## Ownership

| Function | MediaOps responsibility | LLM Proxy responsibility |
| --- | --- | --- |
| TelePrompter | Editing, project state, user access, task orchestration, preview, export | Model calls required by prompt operations |
| Tube and YouTube | Channel access, OAuth, catalog, staged changes, uploads, playlists | Model calls used for text assistance |
| Subtitles | Browser workflow, job state, local audio extraction, result presentation | Dictator transcription, alignment, and subtitle provider calls |
| Text Video | Browser workflow, local render jobs, composition, output validation | Model calls required by that workflow |
| Audio QC | Projects, review, corrections, repair plans, stitching, promotion, export | Model calls required for speech or alignment |
| Frame Picker | Entire application, storage, and frame extraction | None |
| CLI and MCP | Public product commands, authorization, dry runs, local paths, orchestration | Provider execution through the official client |
| Narration and composition | Plans, chunk order, cadence, assembly, manifests, local validation | Speech, music, and other provider requests |

MediaOps keeps YouTube credentials and application authentication.
Only model-provider credentials move to the gateway after their final direct consumer passes acceptance.
Tenant metrics remain in LLM Proxy. A MediaOps metrics client is outside this task.

## Issue Sequence

The canonical provider sequence is in [Media Gateway Consolidation](media-gateway-consolidation.md#delivery-sequence).

| Order | LLM Proxy | MediaOps | Result |
| --- | --- | --- | --- |
| 1 | F022, I046, F024 | I009 foundation | Durable model operations, capacity, and official client integration |
| 2 | F039 | I084 | Complete OpenAI image access |
| 3 | F042 | I087 | Dictator access through the gateway |
| 4 | F043, F040 | I089, I085 | Required provider staging and Vertex image access |
| 5 | F041 | I086 | FAL image access |
| 6 | F025 | I010 | Video access: Runway, Vertex, FAL, Kling, then xAI |
| 7 | F026 | I011 | ElevenLabs speech, music, alignment, voices, and history |
| 8 | F027 | I012 | HeyGen and remaining Kling provider resources and mutations |
| 9 | I244, F071 | I088, I092 | Final provider boundary and receipt audit |

F071 and I092 confirm this ownership boundary. They do not move applications.
MediaOps F021 keeps preview and export on the existing local composition path after provider cutover.
MediaOps I027, I030, and I031 remain MediaOps workflow work.
P003 and P005 remain Planning items and are outside implementation.

## Acceptance And Data

Use each retained MediaOps public entry point to verify its gateway integration.
Keep user authorization, explicit spend authority, dry-run behavior, local path containment, and product intent in MediaOps.
Use the released official client for provider requests.
Keep provider protocol tests in LLM Proxy and product behavior tests in MediaOps.

Inventory provider records before each bounded import.
Map each eligible provider handle to its source owner, destination tenant, and provider account.
Keep application projects, drafts, local jobs, and local artifacts in MediaOps.
Replace provider references with gateway references where the capability cutover requires them.
Keep uncertain work available for recovery without another paid submission.

The existing source inventory records 1,233 local operation records and 18 uncertain operations.
These counts are historical evidence with explicit scope limits, not proof of an empty current runtime.
MediaOps `docs/media-gateway-source-inventory.md` and the private receipts contain the detailed evidence.

Remove only direct provider paths and obsolete provider credentials after acceptance.
Keep Frame Picker and other local workers available throughout this migration.
Record source CI, client publication, service activation, and consumer acceptance separately.
