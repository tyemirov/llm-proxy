# F070 Handoff

## Authoritative Resume State

The user requested this handoff because the session has few tokens left.
This section replaces all resume instructions in the historical sections below.
F070 remains incomplete. This handoff does not complete or pause the goal.

B275 is published. The latest increment adds B266 journal admission acceptance. Verify publication, then continue the remaining B266 and F070 work.
Do not expand the product scope.

### Checkout And Publication

- Repository: `/Users/tyemirov/Development/llm-proxy`.
- Branch: `feature/F069-prepaid-payments`.
- Published B275 commit: `5bcd86d96050c51c94995a12fd4e5d373759b48b`.
- Published cancellation increment: `a3e72246c647b89c2543cb48e2daf81aeeb6237f`.
- Published media input increment: `bcdbee132afe38afc5ac73deaf31695630f77f4a`.
- The commit that contains this section adds B266 journal admission tests. Verify its remote publication before further edits.
- PR 344: https://github.com/tyemirov/llm-proxy/pull/344.
- Last verified PR state: open, ready for review, base `feature/F068-prepaid-balances`.
- PR stack: 339 (F065), 340 (F066), 341 (F067), 342 (F068), 344 (F069).
- Each component uses the preceding component as its base. PR 339 targets `master`.
- PR 343 is unrelated onboarding work.
- Final stack CI and complete acceptance remain open. Keep F065 through F070 open.

This section describes the B275 increment. Application merge, release, and deployment remain outside its scope.
The current production changes have focused validation. Aggregate validation remains incomplete.

### Latest Journal Admission Acceptance

The latest increment adds `internal/proxy/hosted_journal_admission_recovery_internal_test.go`.
Its 12 scenarios exercise failed tenant locks, journal writes, authority reads, and identifier generation through real HTTP.
The tests also inject malformed grant scope and absent or future credential qualification at the database read boundary.
Rejection preserves funds without partial journal, price, or reservation records and causes zero provider calls.
Restored dependencies admit the same key once and preserve replay across restart.

The new checks passed in 6.143 seconds. Their race run passed in 77.700 seconds.
Go lint and formatting passed. No production code or public contract changed.
The first fixture interrupted authentication. The corrected fixture selects the hosted request context and active database transaction.
Logs and profiles use `/tmp/llm-proxy-b266-journal-admission` as their prefix.
No test process remains active.

The latest diagnostic is `/tmp/llm-proxy-b266-journal-admission-diagnostic.coverprofile`.
It has 422 uncovered statements across 21172 statements and retains the B275 aggregate baseline.
Later focused profiles use unchanged production source coordinates. This diagnostic does not establish aggregate CI success.
Discard old counts for each production file that changes before another profile combination.

### Previous Media Input Acceptance

The latest increment extends `internal/proxy/hosted_runtime_text_catalog_internal_test.go`.
The shared normal-runtime fixture now tests image and audio inputs alongside the existing text matrix.
All 67 text offerings across 13 providers retain their four-interface acceptance.
Image cases cover 37 offerings across seven providers through `/v2`.
Audio and mixed image/audio cases each cover eight offerings across two providers.

Each case verifies ordered media bytes, platform credentials, native models, exact charges, settlement, and account remainders.
Removed or changed media and reversed attachments return HTTP 409 under the accepted key without another provider call.
Unfunded requests cause no provider work. Responses do not expose input media.
The controlled Google responses report media token quantities within the exact input total.

The original matrix passed in 22.016 seconds before fixture changes. The extended matrix passed in 28.715 seconds.
Media input race checks passed in 61.995 seconds. Go lint and formatting passed.
No production code or public contract changed. No test process remains active.
Logs and profiles use `/tmp/llm-proxy-b266-multimodal` as their prefix.
That increment produced `/tmp/llm-proxy-b266-multimodal-diagnostic.coverprofile`.
It has 433 uncovered statements across 21172 statements. It does not establish aggregate CI success.

### Previous Cancellation Acceptance

The latest increment adds `internal/proxy/hosted_media_cancellation_recovery_internal_test.go`.
It has ten queued cancellation scenarios and four running cancellation scenarios through real HTTP endpoints.
Storage failures preserve financial resources. Queued recovery releases unused funds once without provider work.
Failed or unsupported cancellation after dispatch keeps funds reserved. Restart cannot repeat unresolved provider work.

All 14 scenarios passed in 11.929 seconds. Their race run passed in 166.327 seconds.
Go lint and formatting passed. No production code or public contract changed.
The initial test used an incorrect response field. It now reads the canonical `cancellation_state` field.
Logs use `/tmp/llm-proxy-b266-media-cancellation` as their prefix.
That increment produced `/tmp/llm-proxy-b266-media-cancellation-diagnostic.coverprofile`.
It combines the B275 aggregate profile with the new focused profile over unchanged production source.
It has 434 uncovered statements across 21172 statements. It does not establish aggregate CI success.
No test process remains active.

### Completed Validation Process

Unified exec session `92981` finished with exit code 2. No validation process remains active.
Its command was:

```sh
make go-test COVERAGE_FILE=/tmp/llm-proxy-b275-coverage.out > /tmp/llm-proxy-b275-go-test.log 2>&1
```

Both complete test passes and all four executable probes finished.
The hosted proxy pass took 452.809 seconds. The remaining proxy pass took 274.878 seconds.
The command then failed with `coverage total 97.9%, want 100.0%`.
The aggregate profile has 438 uncovered statements across 21172 statements, in 425 uncovered blocks.
B275 is resolved. B266 retains the coverage failure and final CI requirements.
The uncovered block list is `/tmp/llm-proxy-b275-uncovered.txt`.

The coverage runner executes two disjoint passes across all packages:

```sh
go test -count=1 ./... -run='^TestHosted' -covermode=count ...
go test -count=1 ./... -skip='^TestHosted' -covermode=count ...
```

The actual commands and profile paths are in `scripts/check_coverage.sh`.
Both passes retain the existing ten-minute timeout. Their profiles combine with all four executable probes.
No tests or production files are excluded. Required coverage remains 100.0%, with no uncovered blocks.
The hosted coverage job retains its 15-minute limit. Hosted execution remains unverified for this increment.

### B275 Implementation In The Checkout

The initial aggregate failure retained 663 maintenance loops and 1952 workers from previous fixtures.
Normal HTTP shutdown tests reproduced six later database reads and an active provider request after service return.
A delayed transport also reproduced `service returned before adapter cleanup: <nil>`.

| Files | Current change |
| --- | --- |
| `internal/proxy/media_operations.go` | Own worker cancellation and completion. Wait for adapter goroutines. Preserve uncertainty on shutdown. |
| `internal/proxy/application.go` | Start media after initial financial reconciliation. Stop media on shutdown and startup failure. |
| `internal/proxy/router.go` | Return a closable `Router` from `BuildRouter`. Separate construction from worker startup. |
| `pkg/llmproxycontract/contract.go` | Add `ErrorCodeMediaWorkerShutdown` with value `worker_shutdown`. |
| `docs/openapi.yaml`, `site/docs/index.html` | Add the error code and regenerate the API page. |
| `internal/proxy/router_lifecycle_fixture_test.go` | Add internal fixtures that close routers before database cleanup. |
| `internal/testfixtures/managed_router.go` | Give shared and bootstrap routers explicit cleanup ownership. |
| Router callers in proxy, CLI, and integration tests | Use the lifecycle fixtures. Preserve test behavior. |
| `internal/proxy/hosted_media_lifecycle_internal_test.go` | Verify startup failure, idle shutdown, provider cancellation, and delayed adapter cleanup. |
| `internal/proxy/hosted_media_configuration_recovery_internal_test.go` | Verify queued work remains idle during construction and executes once after startup. |
| `internal/proxy/media_operations_edges_internal_test.go` | Use real deadline cancellation and release controlled adapter goroutines. |
| `scripts/check_coverage.sh` | Run both test groups and combine their coverage with executable probes. |
| `tests/operational_contract_test.go`, `Makefile` | Verify both groups, unchanged timeouts, profile combination, and the explicit client prompt. |
| `README.md`, `docs/hosted-billing.md` | Record coverage execution and media lifecycle ownership. |
| Tracker, this handoff, and B275 plan | Record progress and remaining validation. |

The new shutdown result is uncertain with `worker_shutdown`.
The account retains 500 posted cents and 461 available cents while the 39-cent reservation remains unresolved.
A restart cannot submit the uncertain operation again.
The constructor establishes hosted authorization before any worker starts.
The application starts media only after initial funds and Paddle reconciliation succeed.
Embedded router callers must stop HTTP service, then call `Router.Close()`.
The shared fixtures register that cleanup before database cleanup.

### B275 Validation Evidence

| Validation | Result | Evidence |
| --- | --- | --- |
| Final lifecycle, recovery, and media edge checks | Passed, 12.590 seconds | `/tmp/llm-proxy-b275-recovery-final.log` |
| Same selected checks with race detection | Passed, 96.067 seconds | `/tmp/llm-proxy-b275-race-final.log` |
| Go lint and formatting after runner changes | Passed | `/tmp/llm-proxy-b275-checks-complete.log` |
| OpenAPI contract and generated page checks | Passed | `/tmp/llm-proxy-b275-openapi.log` |
| API page generation | Passed | `/tmp/llm-proxy-b275-api-docs.log` |
| Coverage runner contract | Passed, 2.215 seconds | `/tmp/llm-proxy-b275-coverage-contract.log` |
| Complete Go component with both passes | Tests and probes passed. Coverage gate failed at 97.9% | `/tmp/llm-proxy-b275-go-test.log` |

The initial runner regression failed with `coverage test group missing`.
Its log is `/tmp/llm-proxy-b275-coverage-contract-before.log`.
The initial lifecycle failures are in `/tmp/llm-proxy-b275-before.log` and `/tmp/llm-proxy-b275-adapter-before.log`.
The error-code regression is in `/tmp/llm-proxy-b275-interruption-before.log`.

An earlier canonical run was stopped deliberately when the delayed adapter test exposed another defect.
Its log is `/tmp/llm-proxy-b275-go-test-before-adapter.log`. Do not treat it as a completed run.
After the adapter correction, the single-pass run still timed out at 601.142 seconds without an assertion failure.
Its log is `/tmp/llm-proxy-b275-go-test-single-pass.log`.
That stack contained only one maintenance loop and three workers, which belonged to the active service.
The remaining timeout caused the two-pass runner change.

### Resume Procedure

1. Inspect the checkout and verify the latest commit in ready PR 344.
2. Continue B266 from `/tmp/llm-proxy-b266-journal-admission-diagnostic.coverprofile`.
3. Cover missing financial behavior through public entry points without invalid core states.
4. Discard old coverage coordinates for each production file that changes.
5. Keep all remaining provider-operation acceptance requirements in scope.
6. Run complete controlled acceptance and final `make ci` after the last stack correction.

Preserve `.mprlab/B266-PLAN.md` and `.mprlab/F070-PLAN.md` while their work remains open.
The completed B275 plan is removed. Plans are untracked.
Do not discard unrelated checkout changes.
Prepare PR descriptions in `/tmp/llm-proxy-f069-pr-body.md` before each update.
Use `gh pr edit 344 --body-file` with that file.
Do not claim hosted CI success from local checks. PR 344 has no hosted checks at its latest inspection.

### B266 And Remaining F070 Scope

The B275 aggregate profile has 438 uncovered statements across 21172 statements.
The latest journal admission diagnostic reduces that count to 422 without production changes.
Do not combine stale source coordinates with new profiles.
The repository-root `coverage.out` is also stale.
The last full stack CI failed with `coverage total 95.3%, want 100.0%`.
Its evidence uses `/tmp/llm-proxy-f070-b266-coverage` as the prefix.
Do not lower coverage requirements, exclude production code, or construct invalid core states to increase coverage.

Complete provider-operation acceptance remains in scope. Consult `Remaining Provider Measurement Contracts` in `docs/hosted-billing.md`.
OpenAI Responses image-tool usage and its combined model/image price bound remain incomplete.
B265 rejects an incomplete bound before dispatch. This rejection does not complete billing support.
ElevenLabs alignment lacks established billed duration and exact account pricing.
Dictator SDK 1.11.0 input-duration operations lack native processed-input duration evidence.
Do not infer billed duration from word timestamps, upload metadata, or output duration.
Keep all selected providers and supported operations in scope.
Do not add unrelated provider features, native video F025, native mobile work, or I274 work.

### Confirmed Commercial Decisions And Authority

- Use provider cost multiplied by 1.30. This is a markup.
- Include all current providers and supported operations under the shared financial contract.
- Require USD 5 minimum funding. Permit customers to spend down to zero.
- Use Paddle and its applicable financial policies.
- Reuse the existing Ledger journal. Verify balance conservation without adding double-entry accounting.
- Reuse existing integrations and shared code. Deliver the browser application without a native mobile application.
- Keep production activation disabled. Application merge and paid provider calls are not authorized.
- Ledger PR 102 was released as v1.1.0. Approved utils PRs 45 and 47 were released as v0.18.0 and v0.19.0.
- Do not infer authorization for another shared release from those approvals.

Account and supplier identity, tax presentation, fee allocation, account exposure, charge policies, and retention remain open before activation.
Commercial rights and provider capacity also require activation decisions.
Keep unspecified decisions explicit. Controlled fixtures do not establish live provider or payment qualification.

### Separate Paddle Follow-Up

F087 already records the separate Paddle setup and actual sandbox qualification.
It must not block F069 or F070 development completion.
Prior read-only authentication used `/Users/tyemirov/Development/PoodleScanner/configs/.env.ps`.
The account had no LLM Proxy product, USD 5 price, or notification setup.
F087 owns distinct product and price resources, identity, checkout origin, webhook configuration, isolated storage, and actual qualification.
Do not copy secrets or reuse another application's product or webhook secret.
PoodleScanner supplies the direct Paddle pattern. Hecate uses RevenueCat for browser commerce.

### Reusable Evidence And Constraints

B274 is published as `a9258ec3`. It corrected hosted authorization before media execution.
Earlier published increments include `bc21175d` for removed authorization and `40082a4f` for exact reservation arithmetic.
Further published increments include `8b52f058` for renewal recovery and `c51ddf6d` for financial schema recovery.
B272 and B273 reject explicit empty hosted and payment configuration.

Reuse `fundedMediaRecoveryFixture` in `internal/proxy/hosted_media_recovery_internal_test.go`.
Its HTTP admission starts with 500 posted cents and 461 available cents after a 39-cent reservation.
Its recovery checks use real financial endpoints and controlled provider responses.
`restartConfiguration` supplies a fresh store over the same database and controlled provider endpoints.
Successful execution settles balances to 498 posted and available cents. Uncertain execution retains the reservation.

Use repository Make targets. Keep focused checks, aggregate CI, publication, and production acceptance distinct.
Preserve unrelated changes. Do not create draft PRs, worktrees, history rewrites, or persistent memory updates.
Do not use subagents, require physical devices, or examine file permission modes.
Do not parallelize issues. Keep user progress updates within 60 seconds during resumed work.
Prior document checks found five existing Governor differences and 72 existing tracker language findings.
Do not normalize unrelated governance files.

Earlier handoff snapshots remain in Git history. Their resume instructions and coverage totals are superseded by this document.
