# Media Provider Completeness

## Scope

The 2026-09-20 request requires all media providers currently implemented in MediaOps.
The source inventory includes nine providers.
It includes generation, discovery, account resources, recovery, and provider mutations.
The model-access boundary in `mediaops-model-access-boundary.md` still applies.

`configs/providers.yml` remains the only runtime provider catalog.
This document records migration scope and evidence. It is not another runtime inventory.
Provider identity, exact models, transports, controls, limits, and prices belong in that YAML file.
The API, connection forms, route explorer, and model matrix must use its validated registry.
The route explorer must show all supported families before a visitor selects filters.

## Provider Hierarchy

Each upstream provider has one provider definition, regardless of its number of capability types.
Existing `openai`, `vertex`, and `xai` definitions receive additional transports and offerings.
Separate providers such as `openai-media` or `openai-images` are prohibited.
All capability branches use the provider's existing connection fields and tenant assignments.
Additional fields are permitted only when the upstream capability requires distinct settings or credentials.

The existing YAML hierarchy supplies the required structure:

```text
providers[]
  id: openai
  fields[]             shared connection inputs
  verification         explicit credential check
  transports[]         text, transcription, images, image editing
  resources[]          typed account resource bindings
  services[]           operations without model selection
  offerings[]          exact model routes and capability controls
```

The same exact model can have offerings from different providers.
Its model identity remains independent of the provider identity.
The global model and family records use references from provider offerings without duplicate model definitions.
Protocol components select execution behavior. Provider names do not select separate service implementations.

1. Extend an existing provider definition when its identity already exists.
2. Add a provider definition only for an absent upstream provider.
3. Reject duplicate provider identifiers during YAML validation.
4. Verify all capability branches through the same provider connection and tenant assignment.
5. Verify one provider entry in discovery and connection setup after each capability addition.
6. Verify each model offering selects its declared transport, controls, limits, and prices.

## Source Evidence

The source is the primary MediaOps checkout under `internal/media/provider/`.
The inspected source commit is `a90a3941fc53be5ecdd8e1c1492467d5ebea1912`.
Model declarations also occur in `internal/media/image/capabilities/`, `internal/media/video/capabilities/`, and `internal/media/audio/`.
The provider methods and source catalogs define the required migration scope.
Published upstream capabilities without a MediaOps implementation are separate feature work.

| Provider definition | Required catalog change | Shared connection rule |
| --- | --- | --- |
| `openai` | Extend the existing definition when a required image control is absent. | Text, transcription, Images, and Responses use the same API-key field. |
| `vertex` | Add image and video transports to the existing definition. | Reuse current connection authority. Add explicit storage settings only when required. |
| `xai` | Extend the existing video offering and its execution component. | Text, transcription, and video use the same account connection. |
| `dictator` | Preserve the existing definition and current exact models. | All speech operations use the same gRPC connection. |
| `fal` | Add one provider for image and video transports. | Both capability types use the same FAL key. |
| `runway` | Add one provider for all seven video model offerings. | All transports use the same Runway account connection. |
| `elevenlabs` | Add one provider for speech, music, alignment, and provider resources. | All capability types use the same ElevenLabs key. |
| `heygen` | Add one provider for translation, avatars, lip-sync, and account resources. | All transports use the same HeyGen account connection. |
| `kling` | Add one provider for video, lip-sync, uploads, and reusable elements. | All transports use the same Kling account connection. |

Resource discovery must not create fake model offerings for histories, dictionaries, quotas, or reusable elements.
Those resource contracts must reference transports and fields within the same provider definition.
Each required new resource schema belongs to F026 or F027 before its adapter implementation.
Storage ownership under F043 remains distinct from provider identity and model ownership.

## Shared Prerequisites

Schema version 5 requires an explicit verification transport within each provider definition.
Media-only providers can use authenticated read-only checks without a text offering.
F076 owns connection verification. F077 owns provider resource declarations.
Provider capability issues remain responsible for their native request and response codecs.

1. Add a typed verification binding within the existing provider definition.
2. Reference the provider's declared transport and existing credential fields.
3. Use a read-only provider request when the upstream contract supplies one.
4. Do not require a fake text offering to save a media-only connection.
5. Declare provider resource bindings for voices, history, dictionaries, metadata, quotas, and reusable elements as required.
6. Validate every resource and verification reference at YAML load time.
7. Expose supported provider resources through the same validated registry.
8. Keep private upstream identifiers inside the resource adapter and stored gateway references.
9. Verify connection creation, replacement, revocation, tenant isolation, and browser discovery through public entry points.

The exact resource schemas must precede their provider implementation.
The declarations select typed code. They must not contain executable expressions or arbitrary request templates.

F079 owns the required service bindings for durable requests without a model.
Those bindings belong within the existing provider definition and use its shared connection.
An absent model selects only an explicitly declared service. It does not select a default model.
Service discovery must remain separate from model families and exact models.
F079 includes ElevenLabs forced alignment as the first native service.
F026 and F027 retain the other native service adapters and complete source coverage.

| Provider | Required current capabilities | Implementation issue | Current gateway evidence |
| --- | --- | --- | --- |
| OpenAI | Image generation, editing, masks, ordered references, progressive results, Responses chains, recovery, cancellation | F024, F039 | Implemented in source with public tests. Publication and consumer acceptance remain separate. |
| Dictator | Speech, voice extraction and discovery, transcription, diarization, alignment, subtitles, diagnostics | F042 | Implemented in source. Final consumer and migration acceptance remain open. |
| Vertex | Image generation and editing, Veo generation and extension, Gemini Omni video, Google credentials, GCS transfers, recovery | F040, F025, F043 | Media adapters remain open. Completion credentials do not prove media support. |
| FAL | Reve images, both Seedance video routes, queue status and recovery, ordered artifacts | F041, F025 | Reve image code and local acceptance tests are implemented. Video, publication, and consumer acceptance remain open. |
| Runway | Text, image, and video inputs, task status and recovery, all seven declared models | F025 | Media adapters remain open. |
| xAI | Video generation, extension and editing controls, task recovery, private file access and cleanup | F025 | Catalog presence does not establish a complete durable media adapter. |
| ElevenLabs | Speech generation and conversion, dictionaries, voices, metadata, history, alignment, all seven music operations | F026 | One provider defines account metadata, quotas, voice discovery, forced alignment, and dictionary creation. All six speech and conversion models use the same provider. Voice-library operations, history, and music remain open. |
| HeyGen | Translation, video lip-sync, avatar creation, motion, avatar video, uploads, quota, task recovery | F027 | Provider definition and adapters are absent. |
| Kling | Video generation, lip-sync, uploads, element create/list/get/delete, task recovery | F025, F027 | Provider definition and adapters are absent. |

## Exact Model Coverage

The implementation must account for every active source model and every supported input mode.
Native identifiers below are source evidence, not permission to add runtime aliases.

| Source family | Source model identifiers |
| --- | --- |
| OpenAI image | `gpt-image-2` |
| Vertex image | `gemini-3-pro-image` |
| FAL image | `reve/2.1/text-to-image` |
| Vertex video | `veo-3.1-generate-001`, `veo-3.1-fast-generate-001`, `gemini-omni-flash-preview` |
| Runway video | `gen4.5`, `gen4_turbo`, `gen4_aleph`, `veo3`, `veo3.1`, `veo3.1_fast`, `seedance2_5` |
| FAL video | `bytedance/seedance-2.0/reference-to-video`, `bytedance/seedance-2.0/fast/reference-to-video` |
| Kling video | `kling-v3`, `kling-v3-omni` |
| xAI video | `grok-imagine-video-1.5` |
| ElevenLabs speech | `eleven_multilingual_v2`, `eleven_flash_v2_5`, `eleven_turbo_v2_5`, `eleven_v3` |
| ElevenLabs conversion | `eleven_english_sts_v2`, `eleven_multilingual_sts_v2` |
| ElevenLabs music | `music_v1` |
| Dictator speech | Source `qwen3` and `silero_ru` map to canonical `qwen3-tts` and `silero-ru`. |
| Dictator transcription | `whisper-tiny`, `whisper-base`, `whisper-small`, `whisper-medium`, `whisper-large-v3` |

The source marks `gpt-image-1` as deprecated.
The forward-only contract uses `gpt-image-2` for current OpenAI image capability.
Deprecated records do not justify runtime compatibility paths.
HeyGen, voice libraries, histories, dictionaries, and reusable elements also require resource contracts without invented model identifiers.

## ElevenLabs Coverage

F026 must include these source methods:

- Speech `Generate` and `Convert`.
- `CreatePronunciationDictionaryFromRules`.
- `LoadMetadata`, including models and subscription information.
- `ListVoices`, `SearchVoiceLibrary`, and `ImportVoiceLibrary`.
- `ListHistory` and `DownloadHistory`.
- Forced alignment `Align`.
- Music `Generate` and `CreateCompositionPlan`.
- `ComposeDetailed` and `ComposeStream`.
- `UploadComposition`, `VideoToMusic`, and `SeparateStems`.

Narration plans, chunk assembly, local path checks, and product authorization stay in MediaOps.
Each provider request uses the shared gateway operation or resource contract.

## Execution And Acceptance

1. Complete I273 for initial catalog visibility.
2. Implement F076 before new media-only provider connections. Complete F077 before provider resource adapters.
3. Use the existing delivery sequence for F042, F043, F040, F041, F025, F026, and F027.
4. Add a failing public integration test before each behavior change.
5. Implement shared typed contracts and reusable protocol components before their provider declarations.
6. Verify a second provider identity with catalog changes only for each new protocol component.
7. Verify exact controls, connection ownership, artifacts, idempotency, restart, cancellation, and recovery.
8. Verify discovery through the API and browser for each new provider and capability.
9. Verify the retained MediaOps entry points through the official released client.
10. Record every source method and model as accepted or still open.
11. Keep I274 open while any required provider capability remains absent.

I244 and F071 retain their final import and ownership requirements.
The source inventory and prior receipts remain authoritative for existing provider records.
Missing owner mappings or artifacts do not authorize deletion or another paid submission.

## Release State

The investigation observed website release `v1.11.1` and catalog revision `sha256-31e420dd859dd49b682145bc7891e3b18f2246c01eb293abf94e77b0220c627a`.
That API and website exposed the same 72 exact models.
The website uses a catalog snapshot during its build.
A source change alone does not change the deployed API or website.
The release procedure must publish the official client and activate the service and website from the accepted catalog.
Production activation and paid acceptance remain separate from local implementation tests.

F079 adds the declared service contract and native ElevenLabs forced alignment.
See [Provider services](provider-services.md) for its request, output, recovery, and acceptance limits.
The remaining F026 methods are still open.

F026 voice discovery uses the common paginated voice resource for ElevenLabs and Dictator.
See [Provider voices](provider-voices.md) for metadata, preview access, authority, and client contracts.

F026 dictionary creation uses the declared service branch and shared account connection.
See [Provider dictionaries](provider-dictionaries.md) for private references, durable creation evidence, and recovery.
The dictionary checkpoint passes all 14 CI gates with 100.0 percent Go coverage.

F026 speech conversion adds both exact source models under the existing ElevenLabs provider.
The shared catalog declares all 28 source formats and eight conversion controls.
The gateway preserves native bytes and publishes a separate audio description artifact.
See [provider speech](provider-speech.md) for the current request and recovery contract.

F026 speech generation adds all four source models with owned dictionaries, continuity references, timestamps, and catalog-selected pacing.
The current native contract rejects a Multilingual v2 language override. This corrects the older source capability flag.
