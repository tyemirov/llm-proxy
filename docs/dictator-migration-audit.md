# Dictator Migration Audit

## Scope And Result

This audit records F042 source and migration evidence on 2026-09-15.
The [media consolidation contract](media-gateway-consolidation.md) defines the required ownership and acceptance rules.
F042 remains open. MediaOps I087 owns the source migration. F071 owns the other application migrations.

The operator confirmed WriterBlock's paused status after this audit on 2026-09-15.
WriterBlock is not an active project. Its source observations below are historical evidence only.
WriterBlock code, credentials, resources, and acceptance are excluded from the active migration requirements.
The [public homepage](https://writer.mprlab.com/) already displays the pause notice and inactive project status.

The production adapter and account connection interface exist.
The released Go client contains the required media methods.
The remaining work includes shared workflow migration, resource ownership, and deployed consumer acceptance.
The initial audit changed documentation only. The subsequent B222 execution changes the protocol registration and acceptance tests.
No service activation or record transfer occurred.

| Repository | Inspected commit | Checkout state before the audit |
| --- | --- | --- |
| LLM Proxy | `d7c7dab0b530d2294f3b347595c45e52a445b2a0` | Clean |
| MediaOps | `547234ddecd7bee8daffa2dfa6ab980029c8bacd` | Existing source and documentation changes |
| WriterBlock | `b6d731411ed8135f13f43c1543d71c5a0bc860a3` | Existing interface and governance changes |

The MediaOps and WriterBlock observations include their current uncommitted files.
Neither checkout changed during this audit.

## Implementation Evidence

| Requirement | Current source evidence | Acceptance status |
| --- | --- | --- |
| Released Dictator SDK | `go.mod` selects `sdk/go/dictatorspeechv1 v1.11.0`. | The production adapter imports this SDK. |
| Account connection and tenant assignment | `internal/proxy/dictator_grpc.go` and `dictator_account.go` use the saved address, token, TLS setting, and assignment. | Public HTTP tests exercise authenticated gRPC and account isolation. |
| Six speech capabilities | `internal/proxy/dictator_grpc_protocol.go` submits transcription, diarization, alignment, subtitles, synthesis, and extraction jobs. | `TestDictatorAccountConnectionUsesAuthenticatedGRPC` exercises all six through the official client. |
| Assets and voices | `dictator_grpc_protocol.go` verifies artifact bytes. `media_voices.go` stores tenant voices and private provider references. | Tests cover voice extraction, private identifiers, exact SRT, and synthesis timelines. |
| Cancellation and recovery | `dictator_account.go` binds each operation to its connection. `dictator_adapter.go` preserves the native job handle. | The focused suite covers cancellation, restart recovery, and changed connection authority. |
| Separate worker capacity | `internal/proxy/media_operations.go` registers a separate Dictator queue. | The focused suite includes Dictator worker isolation. |
| Browser provider selection | `tests/blackbox/connection-dashboard.spec.js` creates and assigns an account connection through the browser. | The real TAuth browser test passed for both catalog providers. |
| Go and Python clients | `pkg/llmproxyclient/media_operations.go` and `python/llm_proxy_client/client.py` expose operations and voices. | Go publication verified below. Python publication was not checked. |
| Second provider identity | B222 derives registration and account binding from the catalog. The second provider uses distinct field names, credentials, endpoint values, and model identity. | The same public acceptance flow passes for both providers. Browser forms and metadata also pass. |

The initial second-provider test returned HTTP 400 for voice discovery.
Distinct connection fields then returned HTTP 503 during credential verification.
B222 corrected both reproduced failures. No provider-name branches were added.

## Released Go Client

The [public module record](https://proxy.golang.org/github.com/tyemirov/llm-proxy/@v/v1.10.0.info) identifies `v1.10.0` at the inspected LLM Proxy commit.
The module record reports `Time: 2026-09-15T19:05:25Z`.
The module archive contains the required operation, voice, and asset methods.
At the initial audit, all 20 Go files in `pkg/llmproxyclient` matched the checkout bytes.
The Go contract file and `go.mod` also matched.
I264 subsequently adds one rejection scenario to `client_test.go`. Production client files still match the release.

| Released file | SHA-256 |
| --- | --- |
| `pkg/llmproxyclient/media_operations.go` | `877c5c1b347a364935adc1832a39e077d8bd31e3bd7f89e318789f6b9053181a` |
| `pkg/llmproxyclient/assets.go` | `dad96f8648721daf0b9189c3def1a572362dc2e7895e9acecb302b84706bf7cf` |

The earlier Go-client publication blocker is cleared.
The archive comparison does not prove service deployment or consumer acceptance.

## Caller Inventory

Paths in this table belong to the named source repository.

| Source caller | Evidence | Destination and required acceptance |
| --- | --- | --- |
| MediaOps speech CLI and MCP | `internal/media/cli/audio_speech.go`, `audio_align.go`, and `internal/media/mcp/service_adapters.go` | Move required shared workflows into LLM Proxy under F042 and I087. Preserve controls, outputs, cancellation, and recovery. |
| MediaOps voices | `internal/media/audio/voice_resource.go` and `internal/media/mcp/voice_resources.go` | Map retained extracted voices to tenant voices. Keep preset discovery in LLM Proxy. Preserve product references to retained voices. |
| MediaOps subtitles and browser jobs | `internal/webapp/mediajobs_runtime.go`, `internal/mediajobs/api/dictator_speech_service.go`, and `subtitles_create.go` | Move shared execution into LLM Proxy. Preserve word timings and exact aligned SRT. F071 owns the remaining application interface migration. |
| MediaOps narration and assembly | `internal/media/cli/audio_speech_render_plan.go` and `internal/media/mcp/service_adapters.go` | Move shared execution and artifact recovery. Keep TelePrompter project behavior in MediaOps. |
| MediaOps diagnostic | `internal/media/doctor/dictator/service.go` and `internal/media/mcp/dictator_diagnostics.go` | Retire the obsolete diagnostic with its migrated callers. Do not add a MediaOps metrics client. |
| TelePrompter preview and export | MediaOps F021 and LLM Proxy F071 define the destination composition contract. | Identify each required project flow before its client integration. General composition delivery remains with F071. |
| WriterBlock dictation, excluded | `internal/app/app.go`, `handleDictate`, and `transcribeWithDictator` | The project is paused. No client replacement is required by F042. |

WriterBlock still imports Dictator SDK `v1.10.0`.
Its `configs/config.yml` contains `llm.dictation.url` and `llm.dictation.auth_token`.
Its dictation handler calls the synchronous native `TranscribeAudio` method.
The source does not use the F042 operation lifecycle.

These WriterBlock source observations do not create migration work or acceptance requirements.
WriterBlock does not gate F042 acceptance, publication, or deployment.

## Resource Inventory

This inventory separates source contracts from observed records.
No private transcript, credential, native identifier, or artifact content is included.

| Resource | Source location and shape | Required destination evidence |
| --- | --- | --- |
| Extracted and preset voices | `<workspace_root>/.mediaops/voices/*.json`, schema `mediaops.voice.v1` | Explicit tenant and provider connection. Private mapping of voice references, source artifacts, and reference transcripts. |
| MCP operations | `<workspace_root>/.mediaops/mcp/operations/*.json`, `operationRecord` in `internal/media/mcp/types.go` | Counts by provider and state. Native recovery evidence for accepted work. Product references and destination operation identities. |
| Subtitle jobs | `<ReelDataDir>/jobs.json` and each job directory | Job state, input bytes, aligned SRT, owner, and target application resource. |
| Text Video jobs | `<ReelDataDir>/text-video/jobs.json` and each job directory | The same ownership and artifact evidence for the shared execution migration. |
| Published artifacts | MCP artifact links, recovery metadata, and job output paths | Verified bytes and source digests. A retained local result can remain without another provider submission. |
| Runtime credentials | MediaOps Dictator configuration | Account connection and tenant assignment. Removal of the old credentials after their final active consumer moves. |

The inspected MediaOps repository root has no `.mediaops/voices` directory.
Its MCP store contains 164 readable JSON records and no records with provider `dictator`.
The records comprise 104 OpenAI operations, 57 stream inspections, and three help operations.
Five OpenAI records have state `uncertain`. They are outside F042 and must remain unchanged.

The inspected WriterBlock repository root has no `.mediaops/voices` directory.
Its MCP directory contains no JSON records.
The MediaOps local `data/audio_to_text_backend` paths contain neither inspected `jobs.json` file.
These observations apply only to those exact local paths.

MediaOps production configuration selects `/data/audio_to_text_backend` on the `mediajobs-data` volume.
A subsequent read-only production inventory inspected that deployed volume.
MCP accepts a selected workspace root. The repository root does not establish the complete workspace set.
The complete MCP workspace set remains unknown.
The receipt below applies only to the two deployed volumes.

No MediaOps voice or operation import command was found in LLM Proxy `cmd`, `scripts`, `internal`, or `pkg`.
An import is necessary only if the complete inventory finds records that require transfer.
Current gateway recovery tests do not prove import of a MediaOps record.

## Next Implementation Steps

1. Move the shared Dictator workflows and behavior tests into LLM Proxy under F042 and MediaOps I087.
2. Record every active workspace and deployed store before the runtime switch.
3. If retained records require transfer, implement a bounded import with explicit owner mappings and recovery evidence.
4. Verify the deployed destination and each active consumer with its assigned tenant connection.
5. Remove obsolete direct callers and credentials after acceptance. Record transferred, excluded, rejected, and remaining counts.

MediaOps voice extraction exposes `DurationSeconds`.
The gateway now accepts positive `duration_seconds` values and selects 20 seconds when the control is absent.
Public tests verify fractional values, omission, and rejection of nonpositive values.
The live harness passed with the original short fixture and an explicit 0.5-second extraction duration.

The second-provider test and live gateway acceptance now pass.
Shared workflow source changes can proceed before the complete production inventory exists.
Consumer activation requires the deployed gateway contract, account connection, and tenant assignment.
Release, publication, deployment, and live acceptance remain separate evidence records.
F071 application work outside the Dictator slice is not an F042 completion prerequisite.

## Validation

- B222 reproduced both registration failures before their corrections.
- `make test-dictator` passed for both providers and the local live-test harness.
- The public tests cover six capabilities, unsupported controls, tenant isolation, downloads, duplicate requests, restart, cancellation, and usage totals.
- The real TAuth browser test passed with distinct connection fields and model metadata for the second provider.
- `make test-dictator-live` passed all six capabilities in 22.94 seconds. See the [live acceptance record](dictator-live-acceptance.md).
- The first CI run passed all Go tests but failed the complete coverage gate.
- I264 adds the missing public client case for a JSON object with a mismatched delimiter.
- Final `make ci` passed all 13 gates in 333 seconds, with 100.0 percent Go coverage.
- Validation includes 117 frontend browser tests and seven real TAuth browser tests.
- Changed prose has no mechanical language findings. Existing findings and governance drift remain unchanged.
- Issue checks preserve all completed identifiers. `git diff --check` passed. Event contracts did not change.
- The earlier client archive comparison remains valid. No production client file changed. I264 adds one client rejection test.
- Governor reported existing differences in `.mprlab/AGENTS.DOCKER.md`, `.mprlab/PLANNING.md`, and `.mprlab/POLICY.md`.
- The existing duplicate I261 remains tracked by I263. No completed issue identifier changed.

## Open Decisions

- Creative Director uses an external MediaOps store in Kamu. Its retained records require reconciliation before transfer.
- The operator selected the MediaOps tenant. Each retained record needs a provider connection before transfer.
- Required TelePrompter composition flows remain with F021 and F071.

## Production Inventory: 2026-09-15

The operator selected their existing account as the resource owner.
The production account query matched the supplied former email address.
At the initial query, its stable Google subject owned three tenants: Default, Social Threader, and FamilyHome.
No separate account matched the supplied current email address.
The account has no Dictator connection or Dictator tenant assignment.
The operator then requested a dedicated MediaOps tenant. The authenticated management API created this tenant and verified its account ownership.

| Resource | Verified value |
| --- | --- |
| Account subject | `google:111357980452034959148` |
| Tenant name | `MediaOps` |
| Tenant ID | `managed-39cb2d58f065d06646f487cc82b98d4f` |
| Creation time | `2026-09-16T01:51:54Z` |
| Provider connection and client key | Not assigned. |

The creation receipt is `/tmp/f042-mediaops-tenant.json`.
The private local receipt is `/tmp/f042-production-ownership.log`.
It contains the stable subject and tenant IDs, without credentials or resource content.

Read-only SSH and Ansible queries inspected the live `tutosh` host.
Ansible reported zero changed tasks.

| Store | Observed result |
| --- | --- |
| `mprlab-nginx-gateway_mediajobs-data`, mounted at `/data/audio_to_text_backend` | Zero files, zero job indexes, and zero nested voice or MCP stores. |
| `mprlab-nginx-gateway_mediaops-data`, mounted at `/data/mediaops` | Zero files, zero job indexes, and zero nested voice or MCP stores. |
| LLM Proxy production `media_voice_records` | Zero records. |
| LLM Proxy production `media_operation_records` | Zero records. |

These MediaOps volumes require no record transfer at the inspected time.
This receipt does not cover external MCP workspace roots.
The Creative Director inventory below identifies an external store with retained records.
The authorized tenant creation changed production state. No record transfer, connection assignment, or deployment occurred.

The destination now provides media CLI commands and three durable MCP tools.
The [speech workflow contract](speech-workflows.md) defines their inputs and recovery behavior.
F042 remains open for the complete source switch, tenant assignment, and deployed consumer acceptance.

## Creative Director And Kamu Inventory

The operator requested inspection of the installed Creative Director skill.
The skill delegates media production to the Creative Director application.
Creative Director starts MediaOps MCP with a caller-selected `--workspace-root`.
Its stage executor also supplies story roots in tool arguments.

The Kamu profile at `configs/creative-director/kamu-tales.json` selects `/Users/tyemirov/Development/Kamu/fairy-tales` as its creative project root.
Kamu contains the installed Creative Director application under `.local/creative-director`.
The external MediaOps store is `/Users/tyemirov/Development/Kamu/.mediaops`.
The inventory found 1,065 readable MCP operation records in that store.

| Dictator record group | Count |
| --- | --- |
| Successful capability queries | 10 |
| Successful diarization operations | 4 |
| Diarization dry runs | 3 |
| Diarization operations with `uncertain` state | 5 |
| Diarization operations with `dispatched` state | 1 |
| Total records tagged for Dictator | 23 |

No voice records were found in the inspected MediaOps voice store.
The other 1,042 operation records are outside the Dictator migration slice.
The six unresolved diarization records contain no explicit job or recovery-handle fields.
Five contain result fields and report references. The dispatched record contains a report reference but no result.
These observations do not establish the provider outcome.

The records contain 13 workspace roots under the absent prefix `/Users/tyemirov/Documents/Projects/Kamu`.
Each relative directory exists under `/Users/tyemirov/Development/Kamu`.
These current directories are migration candidates. No saved path was changed.

| Recorded path relative to the Kamu root | Records |
| --- | --- |
| Repository root | 558 |
| `fairy-tales/zhivotni-vyv-vodenitsa` | 97 |
| `intro/intro-prikazki-ot-kamu-bg` | 1 |
| `ending/kamu-subscribe-outro-bg` | 1 |
| `fairy-tales/umna-moma-i-tsar` | 32 |
| `fairy-tales/zlatna-jabylka` | 48 |
| `fairy-tales/dvama-bratja` | 26 |
| `fairy-tales/neblagodarnost` | 64 |
| `fairy-tales/tsarska-dyshterja-i-shivach` | 72 |
| `fairy-tales/umna-moma` | 64 |
| `fairy-tales/siromah-i-tsarska-dyshterja` | 84 |
| `fairy-tales/svirach-na-tambura` | 1 |
| `fairy-tales/chovek-i-zmija` | 17 |

The local receipt `/tmp/f042-creative-director-inventory.json` contains counts, paths, and field-presence metadata.
It contains no credentials, transcripts, or provider identifiers.
This inspection did not start Creative Director or submit provider work.

Before source retirement:

1. Reconcile the six unresolved records with their reports and retained provider evidence.
2. Establish the current paths and transfer the required resources into the MediaOps tenant through one bounded migration.
3. Assign the Dictator connection and switch Creative Director's shared speech calls to LLM Proxy.
4. Verify deployed consumer acceptance before source retirement.

Preserve uncertain outcomes until evidence establishes their state. Do not submit duplicate work to replace missing recovery evidence.

## Workflow Validation

- Public tests first rejected the missing extraction duration field.
- Both provider registrations then passed fractional duration, default duration, and nonpositive duration checks.
- Explicit `null` initially passed validation. The corrected API rejects it before provider submission.
- B223 corrected a process crash from an authenticated unknown MCP tool call.
- Public MCP tests cover all six speech capabilities, tenant ownership, duplicate requests, downloads, and sanitized store failures.
- The compiled CLI passed extraction submission, a separate wait, and download against the real local gateway.
- The final live run passed all six capabilities in 5.21 seconds.
- Initial CI reported four uncovered blocks. Additional public requests exercised all four blocks.
- The coverage target passed with no uncovered blocks.
- Final `make ci` passed all 13 gates in 324 seconds with 100.0 percent Go coverage.
- Changed prose has no mechanical language findings. Existing tracker and README findings remain unchanged.
- The existing duplicate I261 remains unchanged. B223 has one unique identifier.
- No publication, deployment, or production resource transfer occurred.

## Reconciliation And Connection: 2026-09-16

The account now owns Dictator connection `connection-ef115fb73594e0158a626d610393aec1`.
The connection is assigned to MediaOps tenant `managed-39cb2d58f065d06646f487cc82b98d4f`.
The management API verified the assignment. The tenant client key returned all six routes from `GET /model/v1/capabilities`.
The private Creative Director environment contains the new key as `LLM_PROXY_MEDIA_SECRET`.
The existing text credential remains separate.

The first connection request returned `HTTP 400 managed_connection_invalid`.
The corrected request used the required idempotency header and canonical TLS value.
Connection creation then returned HTTP 201. Assignment returned HTTP 200.

| Source operation | Established outcome | Evidence |
| --- | --- | --- |
| `mediaops-20260821T025702-000001` | Failed before job creation | The saved error reports rejection of the synchronous diarization RPC. |
| `mediaops-20260821T030050-000001` | Deleted by user decision | The outcome is unknown. No terminal report or recovery handle exists in the inspected records. |
| `mediaops-20260828T214305-000006` | Failed before diarization submission | The saved error reports HTTP 502 during artifact upload. |
| `mediaops-20260828T214335-000007` | Failed before diarization submission | The saved error reports HTTP 502 during artifact upload. |
| `mediaops-20260828T225803-000017` | Failed before diarization submission | The saved error reports HTTP 502 during artifact upload. |
| `mediaops-20260829T020941-000003` | Failed before job creation | The saved error reports rejection of the synchronous diarization RPC. |

The Dictator handler rejects the synchronous RPC before job creation.
The MediaOps adapter completes artifact upload before it submits diarization.
These code paths support the five failure dispositions. Upload failure does not establish whether the provider retained partial input bytes.
The five failure records remain unchanged. The private migration receipt contains their digests and established outcomes.
No replacement provider request was submitted for these records.

All 13 relative workspace paths in the inventory now have explicit mappings under `/Users/tyemirov/Development/Kamu`.
All 23 Dictator records map to the account, tenant, and connection above.
The other 1,042 records remain outside this migration.
The receipt is `.git/f042-migration-receipt.json` in the LLM Proxy checkout.
It contains paths, source digests, dispositions, and ownership. It contains no credentials or transcript text.

Two successful records have exact retained output matches.
The `neblagodarnost` output file is absent. Its successful record retains result data.
Two successful `umna-moma` records share one output path. Its current bytes match only the later record.
The uncertain dispatch also names that path. A later successful output does not establish the earlier dispatch outcome.
No historical result was promoted into Creative Director current state or imported as a new gateway operation.

Creative Director I013 owns the current consumer change.
The user-directed deletion below supplies the unresolved dispatch disposition.
Source retirement still requires consumer acceptance.

## Consumer And Asset Acceptance: 2026-09-16

Creative Director's live speech adapter passed against the assigned production tenant.
Operation `mop_51949cd730217e9255a3196c02c39ebf` returned the expected transcript.
The test verified upload, terminal success, metadata, download, local evidence, and receipt reuse in 12.67 seconds.
This result proves adapter connectivity. It does not prove an installed product delivery.

The first upload failed with `HTTP 500 asset_store_error`.
B224 found 139 stored asset records with an obsolete metadata shape.
All 139 retained blobs matched their saved sizes and digests. All records were expired.
The bounded migration changed only `version: 1` to `version: 2` and `sha256` to `content_sha256`.
Original metadata and the migration receipt remain under `migrations/B224-20260916` in the gateway data volume.
The migration program was removed after acceptance.

The next live request returned `HTTP 400 media_operation_invalid`.
Creative Director sent output-selection flags that the current gateway does not accept.
The corrected controls select automatic language detection and the medium model.
The gateway returns all diarization output collections without those flags.

## Retained Consumer State

Creative Director I014 owns the retained `neblagodarnost` checkpoint migration.
The checkpoint contains four copies of a MediaOps speech handoff.
It lacks semantic operation, audio-artifact, and text-artifact identities.
The story has no current artifact registry.
The checkpoint remains unchanged. A tool-name replacement cannot establish current identity or recovery.
Source retirement also requires this owner migration and installed product acceptance.

## User-Directed Deletion

The user directed deletion of `mediaops-20260821T030050-000001`.
The source record was deleted on 2026-09-16. The private receipt records the decision and source digest.
The operation outcome remains unknown. No replacement provider request was submitted.
All 22 other Dictator source records still match their recorded digests.
The initial ownership mapping covers 23 records. The current source store retains 22.
The unresolved-dispatch disposition no longer blocks retirement.
Creative Director I014 and installed product acceptance remain open.

## Final Consumer Validation

Creative Director's final `make ci TEST_FLAGS=-count=1` passed with 100.0 percent statement coverage.
The qualified build is installed in Kamu's local Creative Director prefix.
Installed help and artifact-impact checks passed.
The installed persisted-story validator reported 184 errors for `neblagodarnost`.
Its first error was `obsolete semantic review snapshots are not permitted`.
Creative Director I014 owns that retained-state migration and installed delivery acceptance.
Creative Director I015 records 19 existing analyzer findings outside the speech change.
Existing event contracts did not change.

The consumer changes cover `cmd/creative-director/deliver.go`, `internal/mediaopsclient/gateway_speech.go`, and `internal/appconfig/config.go`.
They also cover the handoff and recovery schemas, configuration, official-client dependency, tests, Makefile, and operator documents.
This checkout adds the reconciliation audit, private mapping receipt, and B224 resolution.
MediaOps execution remains active until the retained-state and product-acceptance requirements pass.

## Preserved Check Findings

The Governor check still reports existing managed-content differences.
LLM Proxy has differences in `.mprlab/AGENTS.DOCKER.md`, `.mprlab/PLANNING.md`, and `.mprlab/POLICY.md`.
Creative Director has differences in `.mprlab/PLANNING.md` and `.mprlab/POLICY.md`.
These documents remain unchanged by this execution.
The existing duplicate I261 remains under I263. No new issue ID is duplicated.
Changed-prose review and both whitespace checks passed.

## Consumer File Summary

These Creative Director files contain the consumer change and its validation:

```text
.mprlab/ISSUES.md
Makefile
README.md
cmd/creative-director/b011_transition_pipeline_test.go
cmd/creative-director/current_flow_coverage_test.go
cmd/creative-director/deliver.go
cmd/creative-director/gateway_speech_test.go
configs/config.yml
docs/FLOW.md
docs/GATEWAY-SPEECH.md
docs/INSTALLATION.md
docs/INTERNAL-FLOW.md
docs/OWNERSHIP.md
go.mod
go.sum
internal/appconfig/config.go
internal/mediaopsclient/client_test.go
internal/mediaopsclient/gateway_speech.go
internal/mediaopsclient/gateway_speech_test.go
internal/schema/execution_operation.go
internal/schema/execution_operation_test.go
internal/schema/gateway_speech_test.go
internal/semanticqa/coverage_test.go
internal/semanticqa/semanticqa_test.go
internal/stageexecutor/language_alignment_contract_test.go
internal/stageexecutor/language_storyboard_coverage_test.go
internal/stageexecutor/workflows.go
schemas/catalog.go
schemas/execution-operation.schema.json
schemas/execution-receipt.schema.json
schemas/executor-handoff.schema.json
schemas/gateway-speech.schema.json
schemas/schema-catalog.json
```
