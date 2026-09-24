# ISSUES

[repo:MediaOps]: https://github.com/MarcoPoloResearchLab/MediaOps

Entries record newly discovered requests or changes.

Read @AGENTS.md (Workflow section), @POLICY.md, and relevant stack guides before implementing changes.

For cross-repository dependencies, read [Dependency References](DEPENDENCY-REFERENCES.md).

Format: `- [ ] [B042] (P1) {I007} Title`

- `[ ]` open, `[-]` taken, `[!]` blocked, `[x]` closed.
- Blocked issues (`[!]`) must include a `Blocked:` line in the body.

Issue work tracks the development SDLC. Development completion requires the
specified repository changes and repository validation.

Production state is outside development completion. An issue can include
production state only when its goal explicitly specifies an activation issue
or production acceptance. Keep repository validation separate from production
acceptance. An activation issue depends only on unresolved development work.

Resolved history is in `.mprlab/ISSUES-ARCHIVE.md`. The archive contains the
initial `v0.2.43` index and complete entries from later archive passes.

Current dependencies name unresolved prerequisites only. Archived issue bodies
retain satisfied historical dependencies.

Migration scope (2026-09-23):
LLM Proxy owns all media-provider API access. MediaOps provides one API and processing service.
TelePrompter is the separate timeline and prompt website. Frame Picker and other retained interfaces stay in MediaOps.
Umbrellas coordinate executable dependencies. Complete dependency issues before closing their umbrella.
Start with I274. MediaOps I093 owns its backend, I094 owns TelePrompter, and I092 owns final acceptance.

## BugFixes

- [x] [B276] (P1) Preserve pending response state after a journal completion failure.
  Evidence:
  Controlled HTTP checks failed with `status=409 want=200` after storage recovered for an accepted request with zero provider calls.
  A failed attempt write and a failed terminal journal write left the journal accepted but the response file failed.
  The response file then rejected the current worker after the journal authorized recovery.
  Requirements:
  - Publish a terminal failure only after the journal records a terminal state.
  - Preserve accepted or executing response state when the terminal journal write fails.
  - Permit one dispatch after recovery of accepted work that has no prior provider call.
  - Preserve uncertainty, financial resources, and evidence for work that already reached the provider.
  - Verify storage failure and recovery through HTTP, then run focused regression, race, lint, and formatting checks.
  - Keep B266 coverage and the final F070 CI checkpoint open.
  Resolution:
  Response publication now preserves pending state when the terminal journal write fails.
  Eight HTTP scenarios verify failed recovery, retained financial and journal resources, restored admission, and restart replay.
  The new checks passed in 4.968 seconds. Broad regression passed in 108.118 seconds.
  The new scenarios passed with race detection in 72.123 seconds. Go lint and formatting passed.
  No public API or event contract changed. B266 and final stack CI remain open.


- [x] [B275] (P1) Restore aggregate Go validation after the billing acceptance expansion.
  Evidence:
  The B274 `make go-test` run exhausted the Go package timeout after 601.301 seconds.
  It reported `panic: test timed out after 10m0s` with `TestMCPDictatorWorkflow (2s)` as the active test.
  The log contains no preceding assertion failure. The coverage script removed its temporary profile after the aborted run.
  The stack dump also retains media maintenance workers from earlier tests after their database cleanup.
  Requirements:
  - Diagnose aggregate test duration and retained workers through the canonical validation target.
  - Correct the owning test or runtime lifecycle without weakening assertions or the coverage threshold.
  - Complete the Go test and executable coverage phases with a current aggregate profile.
  Validation:
  - Use `/tmp/llm-proxy-b274-go-test.log` as the initial failure evidence.
  - Preserve B266 coverage requirements and the final F070 CI checkpoint.
  Progress:
  HTTP shutdown acceptance reproduced six later maintenance reads and an active provider request after the listener stopped.
  The application now starts media workers after initial financial reconciliation and waits for them during shutdown.
  Embedded callers receive a closable router. Shared fixtures stop its workers before database cleanup.
  Interrupted execution retains uncertainty and held funds with the public `worker_shutdown` code.
  Failed financial startup leaves accepted media queued. Restart executes once after the failure is removed.
  A delayed transport proved that adapter cleanup could outlive service shutdown. Shutdown now also waits for adapter goroutines.
  Final lifecycle and recovery checks passed in 12.590 seconds. Their race checks passed in 96.067 seconds.
  Go lint, format, OpenAPI contract, and generated artifact checks passed.
  The corrected worker lifecycle reduced the timeout dump from 663 maintenance loops and 1952 worker loops to one current service.
  The aggregate invocation still exhausted ten minutes without an assertion failure. Its active test reported zero elapsed seconds.
  Canonical coverage now runs hosted tests and all remaining tests in separate passes across every package.
  Both profiles and executable probes contribute to the unchanged strict coverage gate. Each pass retains the existing timeout.
  The runner regression verifies both test groups, shared coverage counts, and the explicit client probe input.
  That regression passed in 2.215 seconds.
  Resolution:
  Both complete Go test passes and all executable probes finished. The proxy passes took 452.809 and 274.878 seconds.
  The current aggregate profile has 438 uncovered statements across 21172 statements.
  The command then failed the unchanged gate with `coverage total 97.9%, want 100.0%`.
  B266 retains that coverage work. Final stack CI and hosted CI remain open.
  Evidence: `/tmp/llm-proxy-b275-go-test.log` and `/tmp/llm-proxy-b275-coverage.out`.

- [x] [B274] (P1) Attach hosted financial admission before media worker startup.
  Evidence:
  A normal-runtime restart rejects funded queued media while its accepted offering scope remains enabled.
  A controlled storage delay exposes `state=failed`, `media_operation_unavailable`, and zero provider calls.
  The media constructor starts workers before the router assigns hosted admission.
  Requirements:
  - Attach validated financial admission before workers resume accepted operations.
  - Preserve rejection when hosted configuration or the accepted scope is removed.
  - Preserve exact settlement, operation identity, and replay without duplicate provider work.
  Validation:
  - Run public HTTP restart acceptance, related media and runtime checks, race checks, lint, and formatting.
  - Keep B266 and final stack CI requirements open.
  Resolution:
  The media constructor now attaches validated financial admission before worker startup and queued recovery.
  The router no longer assigns this dependency after workers start.
  Retained and removed-scope restart checks passed in 2.825 seconds. Their race run passed in 39.399 seconds.
  Media and runtime regression passed in 71.957 seconds. Go lint and format checks passed.
  B275 records the subsequent aggregate package timeout. B266 and final stack CI remain open.

- [x] [B273] (P1) Reject explicit empty payment configuration.
  Evidence:
  The root command accepts `payments: {}` and reaches service startup without an error.
  Viper drops the empty object during typed decoding and disables payment processing.
  The new root-command scenario reports `started=true error=<nil>`.
  Goal:
  Preserve explicit payment configuration for validation before service startup or database access.
  Requirements:
  - Reject incomplete explicit payment configuration through the existing payment validator.
  - Preserve omitted payment configuration as the disabled state.
  - Preserve valid payment configuration and existing environment validation.
  Validation:
  - Pass root-command checks for omitted, empty, incomplete, and valid payment configuration.
  - Pass CLI regression, race, lint, and format checks.
  Resolution:
  The configuration decoder preserves explicit payment presence for the existing payment validator.
  Empty and incomplete objects are rejected before startup. Omitted configuration keeps payments disabled.
  CLI regression passed in 8.578 seconds. All 87 selected CLI race scenarios passed in 95.615 seconds.
  Go lint and format checks passed. B266 retains the aggregate coverage and final CI requirements.

- [x] [B272] (P1) Reject explicit hosted configuration without offering scopes.
  Evidence:
  The root command accepts `hosted: {}` and reaches service startup without an error.
  The configuration decoder drops the empty object and treats hosted configuration as omitted.
  The committed `missing-list` scenario reports `started=true error=<nil>`.
  Goal:
  Reject incomplete explicit hosted configuration before service startup or database creation.
  Requirements:
  - Validate explicit hosted scope presence at the configuration file boundary.
  - Preserve omitted hosted configuration as the disabled state.
  - Preserve valid scopes and existing condition validation.
  Validation:
  - Pass root-command rejection and omitted-configuration checks.
  - Pass runtime regression, CLI checks, Go lint, and format checks.
  Resolution:
  Explicit hosted configuration now requires an offering list before typed decoding.
  Omitted hosted configuration still reaches service startup with hosted execution disabled.
  Runtime HTTP checks passed in 36.644 seconds. Final CLI configuration checks passed in 7.389 seconds.
  Before the final lookup refactor, all 82 selected CLI race scenarios passed in 81.180 seconds.
  Go lint and format checks passed after the final change. B266 retains the aggregate coverage and final CI requirements.

- [x] [B271] (P1) Reject incomplete retained completion results.
  Evidence:
  An identical funded POST returns HTTP 200 with empty text when its retained result contains JSON `null`.
  Recovery also records a publication receipt for that invalid result.
  The status GET returns HTTP 200 with JSON `null` or null text from the same file.
  Goal:
  Reject invalid completion data before result replay, status reads, or publication recovery.
  Requirements:
  - Require a JSON object with a non-null text string at the stored-result boundary.
  - Preserve valid empty text and optional tool calls and usage.
  - Return the existing service error for invalid stored results.
  - Preserve funds and pending evidence without another provider call.
  Validation:
  - Exercise result corruption through HTTP before and after publication.
  - Restore the original result and verify replay and one financial settlement after repeated restart.
  - Run focused result, text, funds, management, race, lint, and format checks.
  Resolution:
  The shared decoder requires a text string before recovery, replay, and status publication.
  Invalid files return the existing errors without a successful result or a new publication receipt.
  Seventeen HTTP scenarios verify storage failures, corrupt results, financial preservation, and repeated restart.
  Valid tool-only results retain empty text, tool calls, usage, and one provider call.
  Text, funds, rating, and financial regression checks passed in 176.130 seconds.
  All 17 focused race scenarios passed in 157.728 seconds. Management tests passed in 17.515 seconds.
  Go lint and format checks passed. B266 retains the open aggregate coverage and final CI requirements.

- [x] [B270] (P1) Reject corrupt retained financial credit receipts.
  Evidence:
  Authenticated credit reads return HTTP 200 with invalid, negative, or zero credit amounts and invalid denominators.
  The same reads expose invalid reason codes and missing receipt timestamps.
  Goal:
  Validate retained credit receipts before publication through the financial API.
  Requirements:
  - Validate the stored amount, reason, and timestamp at the database boundary.
  - Return the existing service error without partial financial data or private details.
  - Preserve balances, Ledger history, tenant usage, and the original financial decision.
  Validation:
  - Verify corrupt receipts through owner and operator HTTP sessions.
  - Restore the original record and verify identical reads and idempotent command replay.
  - Run credit, management, race, lint, and format checks.
  Resolution:
  Retained credit receipts now validate the amount, reason code, and timestamp before publication.
  Invalid records return HTTP 500 with `billing_account_store_failed` and no partial receipt.
  All 18 scenarios passed, including storage failures and corrupt usage credits. Restoration preserves the original receipt and financial effects.
  Funds, rating, and financial-read tests passed in 145.282 seconds. Management tests passed in 18.359 seconds.
  The new credit-read race suite passed in 34.426 seconds. Go lint and format checks pass.


- [x] [B267] (P1) Reject trailing JSON before management writes.
  Evidence:
  The funds-resolution HTTP test sent a valid decision followed by a second JSON object.
  The endpoint returned `200` and settled one cent instead of rejecting the malformed request.
  The shared management decoder read only the first JSON value.
  Requirements:
  - Require one complete JSON value at the shared management boundary.
  - Reject additional values and trailing non-whitespace bytes before any state change.
  - Continue to accept trailing whitespace.
  Validation:
  - Verify unchanged funds, holds, Ledger history, and audit records after rejection.
  - Run the management and funds regression tests.
  Resolution:
  The shared decoder now requires the end of the request after one JSON value.
  HTTP tests reject another object, trailing text, and trailing null before financial changes.
  Valid decisions with trailing whitespace still succeed. Restart replay preserves one settlement and its Ledger effects.
  Management tests passed in 16.798 seconds. Funds tests passed in 67.332 seconds.
  Funds-resolution race tests passed in 247.812 seconds. Go lint and format checks pass.

- [x] [B268] (P1) Classify corrupt retained prices as service failures.
  Evidence:
  A valid price-snapshot GET returns HTTP 400 with `usage_journal_invalid` when the stored usage bound is not numeric.
  The integration test reports `status=400 expected=500`.
  Goal:
  Keep stored price corruption separate from malformed customer requests.
  Requirements:
  - Use the existing internal-service error response for retained price validation failures.
  - Preserve the original error cause without exposing private details.
  - Preserve pending usage delivery and customer funds until the original price is restored.
  Validation:
  - Verify the public price resource and startup settlement with corrupt retained prices.
  - Verify recovery settles once with the original accepted prices.
  Resolution:
  Retained price validation failures now return HTTP 500 with `usage_journal_unavailable`.
  The wrapped error preserves its cause. The public response excludes private validation details.
  The 32 scenarios verify price reads, unchanged funds after failure, and one settlement after restoration.
  Rating tests passed in 25.214 seconds. Management tests passed in 17.905 seconds.
  Price integrity race tests passed in 199.411 seconds. Go lint and format checks pass.

- [x] [B269] (P1) Reject invalid stored fractions from balance reads.
  Evidence:
  A valid balance GET returns HTTP 200 when the stored remainder has a zero denominator, invalid text, or a negative numerator.
  It also returns a remainder of USD 1 although the account fraction must stay below one cent.
  Goal:
  Apply the same remainder contract to public balances and financial transactions.
  Requirements:
  - Validate the stored remainder at the database boundary.
  - Use the existing service error response without private data.
  - Share remainder validation with settlement and credit calculations.
  - Preserve pending delivery and customer funds until the original data is restored.
  Validation:
  - Reject each corrupt balance through the authenticated HTTP resource.
  - Verify one settlement after restoration and restart.
  Resolution:
  Balance reads now reject invalid stored fractions with HTTP 500 and `billing_account_store_failed`.
  The public response excludes private data. Settlement, credits, and balance reads share one remainder validator.
  All 20 recovery scenarios passed. Funds and money tests passed in 103.746 seconds.
  Management tests passed in 18.131 seconds. The new race suite passed in 128.605 seconds.
  Go lint and format checks pass. The public error schema is unchanged.

- [ ] [B266] (P1) Restore the required coverage gate for the hosted billing stack.
  Evidence:
  The corrected stack CI run passed all Go tests but reported `coverage total 95.3%, want 100.0%`.
  The function report identifies incomplete coverage in 288 functions across 79 files.
  Progress:
  - Payment checks cover rejected evidence, refund history, tax-inclusive rounding, financial rollback, and unchanged receipts and balances after failure.
  - Authority checks cover atomic writes, failed reads, random-source failures, provider qualification, pagination, and concurrent creation and rotation.
  - Payment reconciliation checks cover exact differences, checkpoint recovery, concurrency, and immutable reports without new Ledger entries.
  - Provider reconciliation checks cover invalid imports, failed reads and writes, retained evidence, and unchanged customer charges.
  - Funds-resolution checks cover financial rollback, completed receipt reads, restart replay, and one monetary effect.
  - B267 fixes the shared decoder defect found by these checks. Trailing JSON cannot trigger management writes.
  - Startup checks preserve pending delivery and financial resources after failure. Restart does not repeat provider work.
  - Admission checks reject before dispatch and preserve funds without partial request, price, or reservation records.
  - Added 20 adjustment scenarios for refund-hold refresh, failed reads, corrupt records, processor outages, and retry checkpoint failures.
  - Refund-hold refresh reserves the pending refund before later usage. Failed refresh leaves no admitted request or provider call.
  - Unapplied adjustments preserve balances, holds, receipts, and Ledger history. Restart and repeated evidence have one refund effect.
  - Added 17 checkout scenarios for failed reads and writes, processor outages, and customer creation recovery.
  - Checkout restart creates one transaction. Repeated payment events credit USD 5 once without additional Ledger entries.
  - Added 22 text execution scenarios for failed reads and writes, truncated responses, and uncertainty recovery.
  - Restart releases undispatched holds and preserves holds for unresolved usage. Replays do not repeat provider work.
  - Added 25 funded media scenarios for failed claims, dispatch, evidence reads, and completion checkpoints.
  - Failed media completion leaves outputs unpublished. Recovery preserves operation identity and prevents repeated provider work.
  - Added 28 credit scenarios for failed reads and writes, corrupt retained amounts, and invalid commands.
  - Failed credits preserve balances, remainders, tenant usage, and settlement records. Recovery applies one credit through the same HTTP resource.
  - Added 32 retained-price scenarios for corrupt data, invalid bounds, and prices that do not match the accepted request.
  - Invalid prices preserve pending delivery and customer funds. Restoration settles once without another provider call.
  - B268 fixes the stored-price error response found by these checks.
  - Added 20 settlement scenarios for later read failures, corrupt amounts, and reservation conflicts after Ledger effects.
  - Failed settlement preserves pending delivery, holds, and funds. Restoration settles once without another provider call.
  - B269 fixes the invalid balance response found by these checks and shares the remainder validator across financial boundaries.
  - Added 17 held-reservation scenarios for failed reads and writes before and after provider dispatch.
  - Failed recovery preserves financial resources, attempts, and reconciliation cases. Restart releases only undispatched holds.
  - Dispatched work keeps its hold for reconciliation. Repeated restart has no new Ledger effect or provider call.
  - Added seven delayed-accounting scenarios after a waiver or partial operator charge.
  - Later evidence updates provider costs and exposure without changing the decision, balance, remainder, or tenant usage.
  - Failed delivery preserves the earlier exposure record and pending evidence. Restart retains one exposure record and one case.
  - Added 76 financial-read scenarios for storage failures, corrupt charges and prices, invalid queries, missing resources, and account isolation.
  - Failed reads return no partial financial data. Restored storage returns the original resources without new provider work.
  - A new billing account reads zero funds without creating Ledger or financial account records.
  - Added 18 credit-read scenarios for corrupt receipts, corrupt usage credits, storage failures, and restoration.
  - B270 fixes corrupt credit receipt publication. Restored records preserve financial resources and idempotent command replay.
  - Added nine account-creation scenarios for storage failures, entropy failures, cancellation, and competing service instances.
  - Failed creation retains no account or financial effects. Restart recovers one account for the accepted intent.
  - Competing instances share one account for the same intent and reject a different intent.
  - Added 29 financial-signal scenarios for unavailable storage, corrupt amounts, unreadable queue timestamps, and inconsistent comparison evidence.
  - Failed CLI reads publish no partial report and preserve durable database bytes. Restoration returns the original signals.
  - Funded checks preserve balances, charges, pending delivery, and provider call counts.
  - Balance reads now retain numeric cents and validated fractions until output. Financial signals no longer parse validated API strings again.
  - Database validation and public response fields remain unchanged. The refactor removes two unreachable parsing errors.
  - Characterization verifies two maximum int64 account balances and their exact aggregate through HTTP and public signal reads.
  - Added 17 result-publication scenarios for storage failures, corrupt completion data, and valid tool-only results.
  - B271 rejects incomplete saved completions before recovery, replay, and status output.
  - Failed recovery preserves held funds and pending evidence. Restoration permits one settlement without another provider call.
  - Added 20 inbox scenarios for malformed HTTP inputs, incomplete bodies, replay read failures, and exact large-number identity.
  - Rejected inputs preserve the original event, receipt, balance, and Ledger history. Restart does not repeat an applied credit.
  - Signed event replay preserves exact JSON numbers and rejects a one-unit difference above the exact integer range of float64.
  - Added 16 startup scenarios for failed environment reads and writes, mixed environments, incomplete schemas, and invalid processor URLs.
  - The public service rejects invalid startup before HTTP admission. Restoration permits two normal starts with unchanged financial resources.
  - Added 12 funding-credit scenarios for account locks, Ledger writes, receipt reads, refund records, and balance reads.
  - Failed credit stops HTTP startup and preserves the pending event without partial financial records.
  - Restoration and completion replay retain one USD 5 credit, one receipt, and unchanged Ledger history.
  - Payment worker assembly now consumes the validated database, catalog, and shared client without duplicate dependency checks.
  - The refactor removes two duplicate checks and their unreachable caller error paths. External configuration and database errors remain unchanged.
  - Added 10 funding-order scenarios for failed admission, replay reads, authorization, invalid queries, and restart recovery.
  - Failed admission preserves existing orders, delivery records, receipts, and funds. Recovery retains one order and delivery for the request.
  - Added 10 payment-state scenarios for failed storage, malformed events, processor outages, incomplete evidence, and cancellation after verified funding.
  - Failed startup closes the listener and preserves the pending event. Recovery applies cancellation once without partial financial records.
  - Cancellation cannot reverse verified funding. Conflicting evidence remains in reconciliation until matching completed-payment evidence resolves it.
  - Hosted scopes now retain validated conditions, attempt limits, and transport from the immutable catalog.
  - Media admission no longer repeats service or model resolution. Startup validation and request-specific pricing remain unchanged.
  - Eight root-command scenarios reject empty, duplicate, unknown-service, and invalid-condition scopes before startup or database creation.
  - B272 fixes the empty hosted-object defect found by these checks. Omitted hosted configuration remains disabled.
  - B273 preserves explicit payment configuration for validation. Empty objects no longer silently disable payments.
  - Payment presence and CLI regression passed in 8.578 seconds. All 87 selected CLI race scenarios passed in 95.615 seconds.
  - Runtime HTTP checks passed in 36.644 seconds. Final CLI configuration checks passed in 7.389 seconds.
  - All 82 selected CLI race scenarios passed before the final lookup refactor. Final CLI, lint, and format checks passed afterward.
  - Earlier increments retain their focused regression and race results in PR 344 and its commits.
  - Public catalog checks reject fractional tokens, excessive quantities, incompatible units, incomplete selections, and invalid imported conditions.
  - Missing supplier prices and tier gaps remain unavailable. Rejected inputs and caller mutations cannot change accepted prices.
  - Catalog regression passed in 1.624 seconds. All 43 selected catalog race scenarios passed in 17.570 seconds.
  - Go lint and format checks passed after the final test change. No production code changed in this increment.
  - Ten public startup scenarios reject incomplete funds storage, incorrect Ledger indexes, and unavailable metadata.
  - Rejected startup preserves the schema and financial resources. Restored storage permits two normal starts without repeated financial effects.
  - All ten startup scenarios passed in 4.752 seconds. Their race run passed in 68.107 seconds. Lint and format checks passed.
  - Five funded media scenarios cover failed claim renewal and failed authorization after dispatch intent.
  - Rejected provider calls and interrupted work retain funds without partial output. Restart cannot repeat the provider submission.
  - Media recovery and the new scenarios passed in 22.151 seconds. No production code changed.
  - All five new race scenarios passed in 57.154 seconds. Go lint and format checks passed.
  - Reservation calculation now retains the exact numeric charge instead of parsing its generated output representation.
  - External monetary inputs still pass boundary validation. Integer-cent overflow and rounding remain unchanged.
  - Three characterization cases verify minimum charges, fractional cents, overflow, repeated calculations, and immutable snapshots.
  - Rating and funds regression passed in 21.556 seconds. Hosted rating regression passed in 25.954 seconds.
  - All 48 selected race scenarios passed in 28.643 seconds. Go lint and format checks passed.
  - Two normal-runtime restart scenarios reject queued media after hosted configuration or the accepted offering scope is removed.
  - Rejected work causes no provider call or output. Reconciliation releases unused funds, and restored authorization cannot repeat the operation.
  - Both scenarios passed in 2.850 seconds. Their race run passed in 31.603 seconds. Go lint and format checks passed.
  - B274 fixes hosted admission assignment after media worker startup. Retained-scope recovery and removed-scope rejection pass through normal HTTP.
  - The B274 component run exhausted the package timeout. B275 records the aggregate validation failure.
  - B275 restored complete Go execution with owned media shutdown and two disjoint test passes.
  - Both test passes and executable probes finished. The current aggregate profile has 438 uncovered statements across 21172 statements.
  - The command failed the unchanged gate with `coverage total 97.9%, want 100.0%`.
  - Use `/tmp/llm-proxy-b275-coverage.out` for current coverage. Earlier focused diagnostics are superseded.
  - Added 14 funded media cancellation scenarios for failed storage before and after provider dispatch.
  - Failed queued cancellation preserves the operation and funds. Recovery releases unused funds once without provider calls.
  - Failed or unsupported cancellation after dispatch preserves the reservation. Uncertain provider results cannot trigger another submission after restart.
  - The new checks passed in 11.929 seconds. Their race run passed in 166.327 seconds. Go lint and formatting passed.
  - The combined diagnostic has 434 uncovered statements across 21172 statements. No production code changed in this increment.
  - Use `/tmp/llm-proxy-b266-media-cancellation-diagnostic.coverprofile` for the updated diagnostic. It does not replace aggregate CI.
  - Extended the normal catalog runtime fixture with ordered image, audio, and mixed input acceptance.
  - Image checks cover 37 offerings across seven providers. Audio and mixed checks each cover eight offerings across two providers.
  - The text regression retains all 67 offerings across 13 providers and four HTTP interfaces.
  - Each media case verifies exact charges, settlement, replay, and rejection of changed media without another provider call.
  - The complete matrix passed in 28.715 seconds. Media input race checks passed in 61.995 seconds. Go lint and formatting passed.
  - The combined diagnostic now has 433 uncovered statements across 21172 statements. Production code is unchanged.
  - Use `/tmp/llm-proxy-b266-multimodal-diagnostic.coverprofile` for the latest diagnostic. Earlier totals are superseded.
  - Added 12 journal admission scenarios for failed tenant locks, request writes, authority reads, and identifier generation.
  - Malformed grant scope and absent or future credential qualification reject admission before reservation.
  - Rejected requests preserve funds without journal, price, reservation, or provider effects. Restored dependencies admit once and preserve restart replay.
  - The new checks passed in 6.143 seconds. Their race run passed in 77.700 seconds. Go lint and formatting passed.
  - The latest combined diagnostic has 422 uncovered statements across 21172 statements. Production code is unchanged.
  - Use `/tmp/llm-proxy-b266-journal-admission-diagnostic.coverprofile` for the latest diagnostic. It does not replace aggregate CI.
  - Added 14 result replay scenarios for saved identity conflicts and incomplete publication before claim expiry.
  - Conflicts preserve financial resources. Missing results remain pending before expiry and become uncertain after recovery without another provider call.
  - Replay and status reuse all six identity checks at the filesystem boundary. Duplicate checks after that boundary are removed.
  - Characterization passed before the refactor in 9.677 seconds. Broad regression passed in 101.679 seconds.
  - Failed-completion replay passed in 1.033 seconds. The new scenarios passed with race detection in 151.960 seconds.
  - Go lint and formatting passed. No public API or event contract changed.
  - The current diagnostic has 414 uncovered statements across 21170 statements. Changed production files use only current coverage counts.
  - Use `/tmp/llm-proxy-b266-result-replay-diagnostic.coverprofile` for the latest diagnostic. It does not replace aggregate CI.
  - Added eight interrupted journal scenarios for failed recovery of accepted, dispatched, and observed work.
  - Failed reads and writes preserve funds, attempts, observations, and reconciliation cases through public HTTP resources.
  - B276 corrects premature terminal response publication found by these checks. Restored accepted work dispatches once.
  - Previously dispatched work remains uncertain and retains held funds without another provider call.
  - Focused checks passed in 4.968 seconds, broad regression in 108.118 seconds, and race checks in 72.123 seconds.
  - Go lint and formatting passed. The current diagnostic has 406 uncovered statements across 21172 statements.
  - Use `/tmp/llm-proxy-b276-diagnostic.coverprofile`. The changed production file uses only current coverage counts.
  - Added 12 normal application startup scenarios for interrupted dispatch, unpublished result, and funds recovery failures.
  - Failed construction preserves financial and journal resources. Restored storage permits HTTP replay without another provider call.
  - A later funds failure retains the recovered result receipt. Subsequent startup settles once.
  - All 12 scenarios passed in 7.069 seconds. Race checks passed in separate groups of 11 and one scenario.
  - The race groups passed in 88.835 and 13.080 seconds. Go lint and formatting passed after formatting the new test file.
  - The diagnostic has 396 uncovered statements across 21172 statements. No production code changed.
  - Use `/tmp/llm-proxy-b266-completion-startup-diagnostic.coverprofile`. Complete aggregate CI remains open.
  - Added eight payment scenarios for malformed events, unknown checkouts, supplier changes, missing adjustment evidence, and refund timing.
  - Unverified events retain reconciliation reasons without credits. Valid completion and restart retain one funding credit and one refund effect.
  - A pending refund cannot hold more than the remaining payment principal. Rejection releases that hold without reversing an approved refund.
  - Focused checks passed in 3.509 seconds and race checks in 46.507 seconds. Go lint and formatting passed.
  - No production code changed. The current diagnostic has 389 uncovered statements across 21172 statements.
  - Use `/tmp/llm-proxy-b266-deferred-payments-diagnostic.coverprofile`. This diagnostic does not replace aggregate CI.
  - Added five tenant budget scenarios for failed account locks, failed limit writes, corrupt admission usage, and inconsistent credit usage.
  - Failed writes preserve limits, revisions, and funds. Invalid usage rejects admission before dispatch and rejects credits before negative totals.
  - Restored storage permits one admission or credit. HTTP replay and restart preserve exact usage and financial effects.
  - Tenant regression passed in 5.521 seconds. New scenario race checks passed in 39.614 seconds. Go lint and formatting passed.
  - No production code changed. The current diagnostic has 385 uncovered statements across 21172 statements.
  - Use `/tmp/llm-proxy-b266-tenant-recovery-diagnostic.coverprofile`. Complete aggregate CI remains open.
  - The Governor check and a direct retry returned HTTP 404 from `https://issues-api.mprlab.com/api/contracts/issue-format`.
  - Changed prose and `git diff --check` passed. Preserve the local issue format until its authoritative source is available.
  - The required coverage gate, complete F070 acceptance, and final stack CI remain open.
  - Added four search limit scenarios for failed and corrupt price reads before initial dispatch and continuation.
  - Initial rejection causes zero provider calls. Continuation rejection preserves the prior call and its exact costs.
  - Restart releases undispatched holds and retains reconciliation holds after prior dispatch. Repeated replay cannot repeat financial effects.
  - Search regression passed in 7.743 seconds and race checks in 35.912 seconds. Go lint and formatting passed.
  - No production code changed. No public API or event contract changed.
  - The diagnostic has 382 uncovered statements across 21172 statements. Use `/tmp/llm-proxy-b266-tool-limit-diagnostic.coverprofile`.
  - Added six assignment mutation scenarios and two collection read scenarios through authenticated HTTP.
  - Failed storage operations preserve tenant profiles, selected grants, and balances. Restoration applies one assignment across restart and replay.
  - A separate scenario verifies ordered hosted and customer-owned assignments across providers. A conflicting grant cannot change the original selection.
  - Assignment regression passed in 4.031 seconds and race checks in 41.997 seconds. Go lint and formatting passed.
  - No production code or public contract changed.
  - The diagnostic has 373 uncovered statements across 21172 statements. Use `/tmp/llm-proxy-b266-assignment-diagnostic.coverprofile`.
  - Added four signed webhook scenarios for older and conflicting transaction and adjustment revisions.
  - Conflicts preserve the refund hold and retained evidence. Valid evidence after restart applies one refund without repeated financial effects.
  - Transaction revision checks remain at the processor observation boundary. The duplicate timestamp check in adjustment comparison is removed.
  - Separate adjustment identity and revision checks remain. Hold refresh reuses its saved evidence without a processor read.
  - Characterization passed before the refactor in 2.294 seconds. Full payment regression passed in 121.207 seconds.
  - Race checks passed in 29.844 seconds. Admission and balance checks passed in 5.056 and 0.961 seconds.
  - Go lint and formatting passed. No public API or event contract changed.
  - The diagnostic has 372 uncovered statements across 21170 statements. The changed production file uses only current coverage counts.
  - Use `/tmp/llm-proxy-b266-payment-revisions-diagnostic.coverprofile`. Complete aggregate CI and F070 acceptance remain open.
  - Net charge calculation now returns its exact rational after validation of retained charges and credits.
  - Settlement consumes that rational directly. Charge responses encode numerator and denominator strings at the HTTP boundary.
  - Stored amount validation and rounding are unchanged. No public API or event contract changed.
  - Existing characterization passed before the refactor in 3.181 seconds. Financial regression passed in 149.702 seconds.
  - Race checks passed in 36.926 seconds. Go lint and formatting passed.
  - The diagnostic has 371 uncovered statements across 21168 statements. Changed production files use only current financial regression counts.
  - Use `/tmp/llm-proxy-b266-net-charge-diagnostic.coverprofile`. Complete aggregate CI and F070 acceptance remain open.
  - Added three grant transition scenarios for a failed read after grant and audit writes inside the transaction.
  - Suspension, reactivation, and revocation failures preserve grant history, assignments, and funds through authenticated HTTP.
  - Restoration applies one transition after restart. Stale revision retries cannot repeat the transition or change prior audit records.
  - Grant and authority regression passed in 4.519 seconds. Race checks passed in 18.680 seconds. Go lint and formatting passed.
  - Initial test errors concerned typed grant states and owner sessions. The corrected fixture uses the existing authorization contract.
  - No production code or public contract changed. The diagnostic has 370 uncovered statements across 21168 statements.
  - Use `/tmp/llm-proxy-b266-grant-transition-diagnostic.coverprofile`. Complete aggregate CI and F070 acceptance remain open.
  Requirements:
  - Cover missing public behaviors and financial failure boundaries with the real service components.
  - Preserve the required coverage threshold and the current provider scope.
  - Remove unreachable or obsolete paths only when the current contract does not require them.
  Validation:
  - Run focused component checks during correction.
  - Pass the existing Go coverage gate and final stack CI.

- [x] [B265] (P1) Reject incomplete cost bounds for Responses image requests.
  Evidence:
  The funded image fixture accepts a Responses request with only an Images price snapshot and reservation.
  The response model can incur additional costs outside that bound.
  Requirements:
  - Reject hosted requests before dispatch when the selected snapshot cannot bound all paid components.
  - Keep customer funds unchanged after rejection.
  - Keep complete Responses billing in F070 scope.
  Validation:
  - Verify rejection through HTTP for image generation and editing, with no provider calls or financial effects.
  - Run image financial and normal runtime regression checks.
  Resolution:
  The integration test first returned `202` instead of `503` for generation and editing.
  Media financial admission now rejects the incomplete Responses cost bound before dispatch.
  The HTTP checks verify unchanged funds, zero charges, and zero provider calls after repeated rejection.
  Related image, rating, admission, worker, and normal runtime checks passed in 10.932 seconds. Go lint passed.
  Complete Responses billing remains open under F070. B266 owns the separate coverage failure.

- [x] [B264] (P1) Correct OpenAPI payment route and authentication fixtures.
  Evidence:
  Stack CI compares the complete route contract with a fixture that disables payments.
  The authentication test expects `TAuthSession` for the Paddle webhook, which requires `PaddleSignature`.
  Requirements:
  - Configure controlled payments in the complete route inventory fixture.
  - Verify the Paddle signature header scheme and webhook authentication requirement.
  - Preserve disabled payment behavior.
  Validation:
  - Run focused OpenAPI and payment checks, then the final stack CI checkpoint under F070.
  Status:
  The complete route fixture now configures controlled payments. Authentication expectations now include the Paddle signature scheme.
  `make test-openapi-contract` passed in 5.467 seconds. Format checks passed.
  The subsequent stack run passed all Go tests, including both OpenAPI checks. B266 owns its separate coverage failure.

- [x] [B263] (P1) Correct the CLI catalog rejection fixture.
  Evidence:
  Stack CI rejects catalog schema version 7, but the test expects an error for version 6.
  The production loader reports the submitted invalid value correctly.
  The fixture now derives its values from the canonical version. Focused catalog checks and format checks pass.
  The subsequent stack run passed all Go tests, including CLI catalog rejection. B266 owns its separate coverage failure.
  Requirements:
  - Derive the invalid schema value and expected error from the canonical version.
  - Include CLI catalog rejection tests in the existing provider catalog target.
  Validation:
  - Run the focused provider catalog checks and the final stack CI checkpoint.

- [x] [B262] (P1) Admit and settle priced dictionary services without a model.
  Goal:
  Use the shared financial contract for the existing dictionary creation service.
  Evidence:
  Normal HTTP admission returns `financial_admission_unavailable` with HTTP 503 instead of HTTP 402 for an unfunded, explicitly priced service.
  Financial admission resolves only model offerings. Dictionary execution does not retain a measured call quantity.
  Requirements:
  - Resolve declared services without an invented model.
  - Bound each dictionary attempt to its single native creation call.
  - Retain a successful call from a validated provider receipt, including receipt recovery.
  - Require an explicit catalog rate per call before financial admission.
  - Preserve unknown financial outcomes when no valid receipt exists.
  Validation:
  - Verify exact provider costs, customer charges, funds release, and replay through normal HTTP execution.
  - Reject unfunded requests without provider work.
  - Keep controlled fixture rates separate from supplier rate qualification.
  Resolution:
  Service admission now resolves the declared service and uses the adapter's single-call bound.
  Dictionary receipts supply exact call evidence through the existing usage journal, including receipt recovery.
  Normal HTTP acceptance passed exact markup, automatic settlement, replay, zero balance, and rejection without provider work.
  Recovery checks passed after failed usage and terminal writes. Invalid receipts remain uncertain without measured usage.
  Related service, media rating, and normal runtime checks passed. Go lint passed.

- [x] [B261] (P1) Include provider service prices in the shared pricing index.
  Goal:
  Use declared service prices through the same exact pricing API as model offerings.
  Evidence:
  Public catalog tests returned `price_not_cataloged` for declared alignment and dictionary service rates.
  The same lookup also lost the reason and source for unavailable service prices.
  Requirements:
  - Index validated service prices by provider and operation with an empty model.
  - Keep the service declaration as the only price source.
  - Preserve exact markup, charge bounds, snapshot immutability, and unavailable price evidence.
  - Reject invented model identifiers and retain unknown usage without a charge.
  Validation:
  - Verify both existing provider services through the public catalog API.
  - Run catalog and hosted rating regression checks.
  - Keep actual provider rate and metering qualification separate from controlled calculation tests.
  Resolution:
  The shared price index now includes validated service declarations without an invented model.
  Public API tests passed exact markup, charge bounds, immutable snapshots, unknown usage, and unavailable price evidence for both services.
  Catalog, provider service, hosted rating, and runtime regression checks passed. Go lint also passed.

- [x] [B260] (P1) Accept explicit routing defaults within assigned hosted grants.
  Goal:
  Let a customer select a model from an assigned hosted grant without customer-owned provider credentials.
  Evidence:
  The funded browser test assigned an active OpenAI grant and selected `gpt-4.1`.
  The defaults resource returned HTTP 400 with `managed_routing_defaults_invalid: reason=provider_key_ineligible`.
  Requirements:
  - Validate hosted defaults against the assigned grant model and operation scope.
  - Preserve explicit model selection through reload and restart.
  - Keep provider credentials private and enforce execution authority at admission.
  - Reject defaults outside the grant scope without a default route substitution.
  Validation:
  - Save and read an allowed default through authenticated HTTP and the browser.
  - Reject another model, another operation, and another account.
  - Verify suspended grants retain the saved selection but cannot execute.
  Resolution:
  The server validates hosted defaults against the assigned grant scope at request time and startup.
  Browser profiles identify assigned hosted providers without exposing credentials.
  Authenticated HTTP checks passed for allowed defaults, rejected scope, account isolation, restart, and connection conflicts.
  The funded browser test passed saved selection, suspension, and zero dispatch after suspension.
  Routing, startup, account connection, hosted browser, Go lint, and frontend lint checks passed.

- [x] [B259] (P1) Reject recurring Paddle prices before prepaid checkout.
  Goal:
  Enforce the F069 one-time funding contract before the service creates or exposes a checkout.
  Evidence:
  A controlled recurring price returns a checkout through the authenticated funding resource.
  The public HTTP test reports `status=200 want=404` and retains a processor transaction.
  The shared transaction representation does not retain the price billing cycle.
  Resolution:
  The worker now reads the price through the existing shared client before transaction creation or uncertain recovery.
  It rejects recurring prices and incompatible amounts without a checkout. Unavailable reads remain retryable without dispatch intent.
  Payment, checkout, browser, Go lint, frontend lint, and formatting checks passed.
  The checkout suite passed in 6.271 seconds. Both payment browser scenarios passed in 22.6 seconds.
  Requirements:
  - Read the selected price through the existing shared Paddle commerce client.
  - Require the retained price identity, funding amount, and a nonrecurring billing cycle before transaction creation.
  - Keep unavailable price reads retryable without a transaction dispatch intent.
  - Preserve uncertain checkout recovery without another transaction creation request.
  Validation:
  - Reject the recurring price through the real authenticated checkout resource with zero processor transaction creations.
  - Verify normal checkout, payment transitions, lost responses, and browser funding through controlled protocols.

- [ ] [B258] (P1) Reject unsupported Dictator alignment languages and preserve typed failure evidence.
  Goal:
  Make language discovery, request validation, and provider failures obey the actual alignment contract.
  Evidence (2026-09-23):
  - Creative Director B073 requested Bulgarian source alignment after successful original-audio review for Kamu F001.
  - Gateway operation `mop_d70826f8327790380bcbbf2dab6d31f2` used `audio.align`, `dictator`, `whisper-medium`, language `bg`, and `remove_punctuation=true`.
  - Input asset: `ast_c6b8bc61647a663a30a5a8b1f5b24b3d`.
  - The gateway accepted the operation at `2026-09-23T19:22:39.805937619Z` and reported terminal `provider_error` without outputs.
  - Retained gateway logs identify native job `4e4cd4a7b0b448fd898d04d2a4bc2d17` and native input `24aabf6a77994eb2ab0f6ceb52ecbf85`.
  - The native job records `dictator.alignment.input.invalid_language` with `unsupported language: 'bg'`.
  - The native job failed approximately 16 milliseconds after execution started, before model loading or alignment.
  - Deployed Dictator alignment languages exclude `bg`. Both installed WhisperX default model maps also exclude `bg`.
  - Successful diarization `mop_51b12ff8a363772f37d8ee988391adfb` does not establish forced-alignment language support.
  Failing boundary:
  - `dictator_adapter.go` accepts any nonempty alignment language.
  - `dictator_grpc_protocol.go` forwards the language to `SubmitAlignTranscriptJob`.
  - `dictatorExecutionResult` reduces the native terminal failure to `provider_error` without the specific validation reason.
  Requirements:
  - Declare supported alignment languages through the current provider capability contract.
  - Validate the selected language before operation acceptance, provider upload, and native job submission.
  - Reject unsupported languages with a stable, actionable public error and zero provider dispatch.
  - Preserve safe typed provider failure evidence when a native job still rejects an accepted request.
  - Keep native handles, credentials, private endpoints, and raw provider details outside public responses.
  - Keep transcription, diarization, and forced-alignment language sets distinct.
  - Preserve this terminal operation, input asset, original audio, checked transcript, and prior accepted recognition.
  Deliverables:
  - Capability metadata, edge validation, safe error mapping, official client documentation, and public HTTP regression coverage.
  Validation:
  - Prove `bg` rejection against the currently unsupported route before any native upload or submission.
  - Prove a declared supported language still reaches alignment.
  - Prove a native invalid-language failure produces retained typed evidence without another submission.
  - Prove terminal reads preserve the original operation and asset references.
  Provider follow-up:
  - Dictator F003 owns new Bulgarian forced-alignment support and exact model qualification.
  - B258 can close with truthful rejection before F003. Bulgarian production acceptance requires F003 and a qualified gateway route.
  Diagnosis scope:
  - Inspection used retained logs, the native job file, and installed source. It submitted no new operation.
  - No release, deployment, artifact replacement, or accepted-content change occurred.


- [x] [B257] (P1) Document retained native completion failures.
  Resolution: The shared native HTTP 502 schema now includes the current durable failure envelope.
  Text, dictation, and client protocol replay tests pass without repeated provider work.
  Generated API pages, artifact validation, shared regression, race checks, lint, and formatting pass.
  Evidence: A hosted continuation failure replays as HTTP 502 with `structured_request_failed`.
  The shared native response schema rejects this current durable failure representation.
  Requirements:
  - Include retained completion failures in the shared HTTP 502 response contract.
  - Preserve the existing failure identifier, safe cause, and replay behavior.
  - Verify native text, dictation, and client protocol failures through HTTP.
  - Regenerate the API pages and run the artifact check.

- [x] [B256] (P1) Report media maintenance persistence failures.
  Resolution: All six maintenance phases report database failures through the configured logger.
  HTTP tests verify rollback, retry, duplicate suppression, and retained financial evidence after operational deletion.
  Media regression, focused race checks, Go lint, and formatting pass.
  Evidence: Controlled database failures leave no operator report in six maintenance paths after HTTP operation admission.
  Requirements:
  - Report failures during queue selection, recovery scans, usage delivery, and terminal retention.
  - Include the maintenance phase and operation identifier when available.
  - Preserve transaction rollback and retry through the next maintenance pass.
  - Keep financial journal records independent of operational retention.
  Validation:
  - Exercise admitted HTTP operations with controlled database read and transaction failures.
  - Verify safe reports, unchanged public state after failure, successful recovery, and duplicate suppression.
  - Run media regression and applicable checks.

- [x] [B255] (P1) Stop hosted file staging after authority loss.
  Evidence: A hosted dictation request completes its file upload after grant revocation during upload initialization.
  Evidence: The generation guard rejects the later model call, but the upload bypasses the authority check.
  Requirements:
  - Assign a staging role to provider file uploads.
  - Require the current worker claim and active grant before each staging request.
  - Preserve read and cleanup operations for accepted work after grant revocation.
  - Reject provider transfers from an obsolete worker.
  - Preserve safe authority error codes through the existing adapter.
  Validation:
  - Exercise upload initialization, finalization, generation, cleanup, and duplicate requests through hosted HTTP dictation.
  - Verify grant revocation and claim expiry before file transmission.
  - Run shared completion regression and applicable checks.
  Resolution: Shared completion authority now guards each provider upload, file read, cleanup, and completion poll.
  Resolution: Staging requires an active grant. Cleanup remains available to the current worker after grant revocation.
  Validation: Hosted upload and process-interruption tests, shared adapter regression, race checks, Go lint, and formatting pass.
  Validation: Final stack CI remains the F070 completion checkpoint.

- [x] [B254] (P1) Expose reconciliation cases for missing hosted results.
  Evidence: A real process restart creates `execution_result_unknown`, but the customer journal returns HTTP 500 for that case.
  Evidence: The browser rejects the same case reason and removes the request evidence view.
  Requirements:
  - Keep all canonical reconciliation reasons in the journal contract.
  - Accept result-recovery cases in the API, OpenAPI schema, and browser client.
  - Preserve private resolution data and rejection of unrecognized reasons.
  Validation:
  - Read the case created after process interruption and result recovery through HTTP.
  - Verify the complete case contract and browser rendering at desktop and phone widths.
  - Run focused regression and applicable checks before final stack CI.
  Resolution: The journal, API schema, and browser accept `execution_result_unknown` and preserve private evidence.
  Validation: Process recovery, journal HTTP tests, browser acceptance, lint, formatting, and generated API checks pass.
  Validation: Final stack CI remains the F070 completion checkpoint.

- [x] [B253] (P1) Enforce hosted worker authority at the gRPC transport boundary.
  Evidence: Controlled gRPC tests submit speech after grant revocation and worker claim expiry.
  Requirements:
  - Use the shared media dispatch guard for HTTP and gRPC provider calls.
  - Reject new work after authority loss and during recovery.
  - Preserve authorized status reads, artifact transfers, and cancellation.
  Validation:
  - Exercise the generated Dictator SDK through hosted HTTP operations.
  - Verify rejection before provider submission and retention of accepted request identity.
  - Run focused regression and race checks before final stack CI.
  Resolution: HTTP and generated Dictator SDK calls use the same media dispatch guard.
  Resolution: The SDK wrapper uses explicit method roles and checks authority for artifact streams.
  Validation: Hosted HTTP and gRPC tests reject revoked or expired authority before submission and prevent submission during recovery.
  Validation: Recovery and cancellation after revocation retain one provider submission and the accepted request identity.
  Validation: Broad regression tests, focused race checks, Go lint, and formatting pass. Final stack CI remains the F070 completion checkpoint.

- [x] [B252] (P1) Use hosted authority during speech admission validation.
  Evidence: A hosted Dictator speech request returns HTTP 400 because validation reads an absent customer connection.
  Requirements:
  - Supply the selected hosted authority to the existing adapter validator.
  - Keep voice ownership and credential checks in their current adapters.
  - Recheck grant and credential authority in the admission transaction.
  Validation:
  - Exercise hosted speech through HTTP and the current Dictator gRPC adapter.
  - Verify accepted credential versions, voice isolation, replay, rotation, and revocation.
  - Run focused regression and race checks before final stack CI.
  Resolution: The shared admission boundary supplies hosted authority to the existing adapter validator.
  Validation: HTTP and gRPC tests verify voice isolation, accepted credential versions, replay, and rejection after admission authority changes.
  Validation: Broad regression tests, focused race checks, Go lint, and formatting pass. Final stack CI remains the F070 completion checkpoint.

- [x] [B251] (P1) Keep provider recovery content outside the usage journal.
  Evidence: Hosted dictionary HTTP tests retain the dictionary name and description in the journal provider identifier.
  Requirements:
  - Give each adapter separate recovery data and provider request identifier fields.
  - Keep the recovery data in the media operation.
  - Commit the recovery data and explicit provider identifier in one transaction.
  - Preserve an absent provider identifier without a substitute value.
  Validation:
  - Verify journal contents after real hosted service requests with present and absent provider identifiers.
  - Verify recovery, worker fencing, and transaction rollback through existing integration tests.
  - Run focused regression and race checks before the final stack CI checkpoint.
  Resolution: Adapters supply a typed receipt with separate recovery data and provider request identifiers.
  Resolution: The worker stores recovery data with the operation and explicit identifiers with the journal in one transaction.
  Validation: HTTP tests verify present and absent identifiers, private dictionary content, and rollback after an identifier write failure.
  Validation: Broad adapter regression, focused race checks, Go lint, and formatting pass. Final stack CI remains the F070 completion checkpoint.

- [x] [B250] (P1) Report media worker persistence failures.
  Evidence: HTTP tests in F066 reproduced missing operator reports after claim, dispatch, and final result write failures.
  Requirements:
  - Return transaction errors to the worker boundary.
  - Preserve the original database cause with the operation and worker identifiers.
  - Report failures from usage, provider handle, preview, output, voice, dictionary, and claim renewal writes.
  - Preserve retained request identity and prevent duplicate provider work after uncertain outcomes.
  Validation:
  - Inject database failures through real HTTP operation flows and the configured logger.
  - Verify operator reports and caller-visible states through the official client.
  - Run focused regression and race checks before the final stack CI checkpoint.
  Resolution: The worker returns transaction failures and records their cause through the configured logger.
  Resolution: Adapter persistence failures include the operation identifier and execution phase.
  Validation: HTTP reporting tests and broad hosted and media regression tests pass. Focused race checks, Go lint, and formatting pass.
  Validation: Final stack CI remains the F070 completion checkpoint.

- [x] [B249] (P1) Keep browser screenshot output outside tracked files.
  Evidence: Release CI changes five tracked PNG files in `artifacts/` and fails with `app_release.source_drift`.
  Requirements: Save generated screenshots in ignored Playwright output and attach them to their test results.
  Requirements: Preserve the failed release screenshots before restoration of the tracked copies.
  Validation: Confirm the initial output failure through browser tests. Verify that final CI leaves tracked file contents unchanged.
  Evidence: `/tmp/browser-output-b249-initial.log` records two missing screenshot failures in the test output directory.
  Resolution: The five screenshot writes use Playwright output paths and test attachments. Both browser stages have separate ignored directories.
  Evidence: Focused frontend and authentication checks pass. All five tracked screenshots retain their original contents.
  Evidence: Final `make ci` passes all 14 gates, including 154 frontend tests and 7 authentication tests. Go statement coverage is 100.0%.
  Evidence: `/tmp/browser-output-b249-integrity.log` confirms that CI changed none of the 621 tracked files. All five screenshot attachments remain available.

- [x] [B247] (P2) Show the necessary text input for Vision and Listen.
  Evidence: Task details mark text as optional, but `/v2` rejects blank message content with attachments.
  Requirements: Show text and the attachment type as required inputs for Vision and Listen.
  Validation: Confirm the browser failure before the metadata correction. Run focused browser checks and final CI.
  Evidence: `/tmp/task-inputs-b247-initial.log` records both incorrect Optional input labels.
  Resolution: Task details now show text and the respective attachment type as necessary inputs for Vision and Listen.
  Validation: Final CI passes all 14 gates, including 154 frontend tests, seven integration browser tests, and 100.0 percent Go coverage.
  Evidence: `/tmp/task-inputs-ci.log`. Event contracts are unchanged.


- [x] [B248] (P2) Include the required transcript in voice extraction task inputs.
  Evidence: Dictator rejects voice extraction without a transcript, but task details list only audio.
  Evidence: The Text input filter incorrectly excludes voice extraction offerings.
  Requirements: Include text in the supported and required inputs for voice extraction.
  Validation: Check task details and Text input selection through the browser. Run final CI with B247.
  Evidence: `/tmp/task-inputs-b248-initial.log` records the missing required Text input.
  Resolution: Voice extraction declares text and audio as supported and required inputs.
  Evidence: `/tmp/task-inputs-focused.log` records three successful browser checks, including Text input selection.
  Validation: Final CI passes all 14 gates, including 154 frontend tests, seven integration browser tests, and 100.0 percent Go coverage.
  Evidence: `/tmp/task-inputs-ci.log`. Event contracts are unchanged.


- [x] [B246] (P2) Keep model capability icons independent of task filters.
  Evidence: Task filters remove supported capability icons from model cards.
  Requirements: Show all supported tasks on dashboard cards and public explorer cards, independent of search filters.
  Validation: Check model icons before and after filter changes through the browser. Run final CI.
  Evidence: `/tmp/model-cards-b246-initial.log` records the missing Generate text icon after Vision selection.
  Evidence: Dashboard and explorer checks pass in `/tmp/model-cards-b246-focused.log`.
  Resolution: Dashboard and explorer cards show all supported task icons, independent of search filters.
  Validation: Final CI passes all 14 gates, including 151 frontend tests, seven integration browser tests, and 100.0 percent Go coverage.
  Evidence: `/tmp/model-filters-ci-final.log`. Event contracts are unchanged.


- [x] [B245] (P2) Keep model drafts when the final task filter stays pressed.
  Evidence: Activating the final pressed task clears the selected model and removes the unsaved system prompt.
  Requirements: Keep the selected model, form values, and focus when the task selection does not change.
  Validation: Use browser coverage for mouse, Enter, and Space activation. Run final CI.
  Evidence: `/tmp/b245-initial.log` records the expected browser failure because the prompt field disappears.
  Resolution: The dashboard keeps model details and form values when the task selection stays unchanged.
  Validation: Mouse, Enter, and Space browser checks pass. Final CI passes all 14 gates, including 151 browser tests and 100.0 percent Go coverage.
  Evidence: `/tmp/b245-focused.log` and `/tmp/b245-ci.log`.
  Event contracts: No event contract changed.

- [x] [B244] (P2) Correct the asset metadata response header declaration.
  Evidence: The public metadata endpoint returns no request-timeout header. Its OpenAPI response requires the upload timeout header.
  Requirements: Declare the metadata response separately from the upload response. Preserve the upload timeout requirement.
  Validation: Validate real upload and metadata responses against OpenAPI. Complete final CI with B243.
  Evidence: `/tmp/llm-proxy-b243-assets-initial.log` records the missing-header schema error for every asset type.

  Resolution: The public asset contract tests and final CI pass. All 14 gates pass with 100.0 percent Go coverage.
  Evidence: `/tmp/llm-proxy-b243-b244-ci.log`. No event contract changed.

- [x] [B243] (P2) Preserve supported asset types in the Python client and API schema.
  Evidence: The gateway and Go client accept FLAC, Ogg, video, JSON, and subtitle assets.
  Evidence: Python `upload_asset` rejects these types because it uses the narrower message attachment list.
  Evidence: OpenAPI omits JSON and subtitle assets from upload, download, and metadata declarations.
  Requirements: Keep asset support consistent across the gateway, official clients, and API schema.
  Requirements: Preserve the separate message attachment constraints.
  Validation: Upload every supported asset type through public clients and HTTP. Verify exact bytes and schema conformance.
  Validation: Preserve the initial failure and complete final CI after the correction.
  Evidence: `/tmp/llm-proxy-b243-python-initial.log` records 12 rejected upload cases across six supported types.
  Evidence: `/tmp/llm-proxy-b243-assets-initial.log` records the JSON and subtitle schema failures.

  Resolution: The public asset contract tests and final CI pass. All 14 gates pass with 100.0 percent Go coverage.
  Evidence: `/tmp/llm-proxy-b243-b244-ci.log`. No event contract changed.

- [x] [B242] (P2) Open the Vertex API Keys page from connection setup.
  Evidence: The catalog link opens the general Vertex page instead of the API Keys page.
  Requirements: Use the direct Google API Keys URL without a fixed account or project.
  Requirements: Document project selection and the Google controls for key creation and retrieval.
  Validation: Chrome shows **Create API Key** and **Show key** at the new destination.
  Validation: The browser test fails with the old URL and passes with the new URL.
  Progress: Updated the catalog URL and the Vertex setup procedure. No event contract changed.
  Validation: All 147 frontend browser tests pass. Frontend static analysis and `git diff --check` pass.
  Resolution: The operation-count assertion is corrected. Final CI passes all 14 gates with 100.0 percent Go coverage.
  Evidence: `/tmp/llm-proxy-f026-dictionaries-final-ci.log` records the successful gate.
  Validation: `make ci` stops at `TestPublicCapabilityCatalogProjectsValidatedRuntimeRegistry` in `tests/capabilities_test.go:137`.
  Evidence: The unchanged test expects 11 operations. The catalog contains 12 operations. This URL change preserves operation counts.
  Evidence: `vertex-ci.log` in the task work directory records the failure.
  Validation: The changed prose passes the mechanical language check. Existing tracker text has 72 findings.
  Validation: Governor reports existing template differences in `AGENTS.DOCKER.md`, `PLANNING.md`, and `POLICY.md`.

- [x] [B241] (P2) Reserve executable capacity for voice preview downloads.
  Evidence: The new preview origin has one active slot and an interactive reserve of one slot.
  Evidence: The public preview test returns `502` when it uses this production allocation.
  Requirements: Allocate an active transfer slot without exceeding the global admission budget.
  Validation: Use the production allocation in the public preview test. Preserve the initial failure and final CI result.
  Evidence: `/tmp/llm-proxy-f026-voices-capacity-initial.log` records the initial failure.
  Progress: The corrected allocation passes the public preview test. Total origin admission is 508 of 512 slots.
  Resolution: Allocated two active preview slots within the unchanged global budget.
  Validation: Final CI passes all 14 gates with 100.0 percent Go coverage.
  Evidence: `/tmp/llm-proxy-f026-voices-verified-ci.log` records the final 347-second run.
  Files: `configs/config.yml` and `internal/proxy/elevenlabs_voices_test.go`.
  Contracts: No new event contract.

- [x] [B240] (P2) Reject explicit empty and null media models at the CLI boundary.
  Evidence: CLI JSON decoding converts an empty or null model to the same Go value as an omitted model.
  Evidence: The client then omits that field and can submit a model-free service request.
  Requirements: Preserve model presence during CLI decoding. Reject null, empty, whitespace-only, and non-string values before HTTP submission.
  Requirements: Preserve valid exact model values and omitted service models.
  Validation: Use the real CLI entry point with a local HTTP server. Verify invalid input sends no request.
  Validation: The initial public CLI test reported `invalid model sent: exit=0 calls=1` for null and empty models.
  Resolution: The CLI preserves model presence and rejects invalid values before HTTP submission.
  Files: `llm-proxy-client/media.go`, `llm-proxy-client/media_test.go`, and `docs/provider-services.md`.
  Contract: The CLI enforces the existing media model contract. No new event contract is required.
  Validation: Focused CLI tests and all 14 CI gates pass with 100.0 percent Go coverage.
  Evidence: `/tmp/llm-proxy-b240-ci.log` records the final result.

- [x] [B239] (P2) Preserve the global capacity budget in the local provider preflight.
  Evidence: The preflight adds an origin allocation to the unchanged production allocation list.
  Evidence: With the ElevenLabs origin, startup fails with `invalid_upstream_capacity: field=global.admitted reason=origin_allocations_exceed_global`.
  Requirements: Divide the existing OpenAI allocation between its native origin and the local preflight origin.
  Requirements: Keep the total origin allocation and production configuration unchanged.
  Validation: `make test-live-provider-harness` and `make test-operational-live-contracts` pass.
  Files: `scripts/test_live_providers.sh` and `README.md`.
  Resolution: The preflight preserves the capacity budget and verifies saved credentials through the local provider.
  Final CI: All 14 gates pass with 100.0 percent Go coverage. The log is `/tmp/llm-proxy-b239-f026-ci-final.log`.


- [x] [B238] (P1) Reject fractional bounds on integer catalog controls.
  Evidence: Public catalog tests accepted integer controls with minimum `0.7` and maximum `1.2`.
  Cause: YAML decoding converted fractional values into integer fields before validation.
  Requirements: Preserve numeric bounds at the YAML boundary. Reject fractional, negative, or non-finite integer bounds.
  Validation: Verify the public catalog parser rejects these declarations and preserves valid integer ranges.
  Resolution: Numeric fields preserve YAML decimals. Integer validation rejects fractional and non-finite bounds before registry construction.
  Validation: Public rejection tests and final CI passed with F078.

- [x] [B237] (P1) Wait for document startup before shared UI readiness.
  Evidence: Two full CI runs reported an authenticated header with `data-auth-state="error"` on the application.
  Evidence: The trace contains no account request. The failure occurs before authenticated data loading.
  Cause: Document state `interactive` can precede `DOMContentLoaded`, which starts shared UI orchestration.
  Requirements: Wait for the document startup event before reading the shared UI readiness promise.
  Requirements: Keep the shared UI loader as the authentication owner. Do not add retries or increase timeouts.
  Validation: Run management browser tests, the local TAuth black-box suite, and final `make ci`.
  Progress: A delayed-document browser test reproduced the failure before the code change and passed after the change.
  Resolution: The application captures document readiness before asynchronous configuration reads. It then awaits shared UI orchestration.
  Validation: Final `make ci` passed all 14 gates in 342 seconds with 100.0 percent Go statement coverage.
  Files: `site/assets/llm-proxy/js/core/mprShell.js` and `tests/e2e/management-ui.spec.js`.
  Event contracts: No changes.

- [x] [B236] (P1) Transfer retained capability defaults before startup validation.
  Evidence: Local Compose exits with `missing_default_column=default_transcription_provider` when the database has the previous dictation columns.
  Goal: Start the current application with retained tenant data after the capability schema change.
  Requirements: Use a bounded GORM transfer in the startup transaction before current schema validation.
  Requirements: Keep saved transcription routes, text defaults, credentials, assignments, access keys, timestamps, and usage records.
  Requirements: Initialize speech defaults as empty and remove the previous dictation columns.
  Requirements: Reject incomplete or mixed schemas without data changes. Keep the current schema versionless.
  Validation: Run public startup, HTTP defaults, restart, and transaction rollback tests for retained databases.
  Validation: Verify a disposable copy of the local database and run `make ci`.
  Scope: Production deployment and production data changes require separate authorization.
  Progress: The initial integration tests reproduced `missing_default_column=default_transcription_provider` and `obsolete_default_column=default_dictation_provider`.
  Progress: Focused capability tests pass after the bounded transfer. Invalid data and injected failures leave the original schema and records unchanged.
  Progress: A Linux binary passed HTTP health on first startup and restart with a disposable copy of the local Compose database.
  Progress: Initial CI passed its assertions but found one uncovered obsolete-column rejection after the new preflight made that check redundant.
  Progress: The duplicate check was removed. Public rejection tests still pass.
  Files: `internal/proxy/management_capability_migration.go`, `internal/proxy/management_store.go`, `internal/proxy/account_connections_store.go`, `internal/proxy/management_capability_migration_test.go`, `docs/tenant-connections.md`, `docs/managed-schema-transition.md`.
  Event contracts: No changes.
  Resolution: Startup transfers the exact previous capability columns once through GORM, then validates the current schema before commit.
  Validation: Final `make ci` passed all 14 gates in 352 seconds with 100.0 percent Go statement coverage.
  Validation: Both starts of the final Linux binary passed HTTP health with the local database copy. The original database remained unchanged.
  Validation: Changed prose has no mechanical findings. Governor reports existing drift in three unchanged policy files.

- [x] [B234] (P2) Reject media-only models at dictation endpoints.
  Evidence: A saved `dictator/whisper-base` default causes HTTP 502 from `/dictate` without a gRPC submission.
  Requirements: Reject offerings without the `dictation` operation before provider execution.
  Requirements: Keep Dictator transcription discovery and defaults available for the media API.
  Validation: Exercise both dictation endpoints through HTTP with a local Dictator server.
  Progress: The initial tests produced HTTP 502 for all three requests.
  Progress: The capability-default tests and `make test-dictator` pass after the operation check.
  Files: `internal/proxy/provider_types.go`, `internal/proxy/provider_registry.go`, `internal/proxy/management_capability_defaults_test.go`, `docs/speech-workflows.md`.
  Event contracts: No changes.
  Resolution: Both dictation endpoints reject media-only models with HTTP 400 before provider execution. Dictator remains available through the media API.
  Final `make ci` passes all 14 gates with 100.0 percent Go coverage.

- [x] [B235] (P2) Distinguish image admission rejection from an unknown provider outcome.
  Evidence: Local capacity rejection produces `uncertain` with zero provider calls.
  Requirements: Set a definite failure when local admission rejects an image submission.
  Requirements: Keep unknown provider outcomes separate from requests without provider execution.
  Validation: Exercise both image surfaces, idempotency, and input asset release through the public API.
  Progress: The initial tests produced `uncertain` for all four rejected requests without another provider call.
  Progress: `make test-image-generation` and `make test-upstream-admission` pass after the error classification change.
  Files: `internal/proxy/upstream_admission_http.go`, `internal/proxy/image_generation.go`, `internal/proxy/image_responses.go`, `internal/proxy/image_admission_e2e_test.go`, `README.md`.
  Event contracts: No changes.
  Resolution: Local image admission rejection produces `failed` with `media_operation_unavailable` and releases input assets. Repeated idempotency keys identify the same failed operation.
  Resolution: Requests with an unknown provider outcome remain `uncertain` after dispatch.
  Final `make ci` passes all 14 gates with 100.0 percent Go coverage.

- [x] [B229] (P1) Send the selected Whisper size to Dictator.
  Evidence: Transcription and subtitle requests omit the model size.
  Requirements: Send the selected size for each Whisper model.
  Validation: Run the public HTTP and gRPC integration tests for all five sizes.
  Progress: All ten model and operation combinations pass through a real gateway and a local gRPC server.
  Files: `internal/proxy/dictator_grpc_protocol.go`, `internal/proxy/dictator_grpc_e2e_test.go`, `docs/speech-workflows.md`.
  Resolution: All five Whisper sizes reach the upstream transcription and subtitle requests. Final CI passes.

- [x] [B230] (P1) Reject speech voices with an incompatible synthesis engine.
  Evidence: A Qwen3 request accepts a Silero voice at admission.
  Requirements: Reject the incompatible voice before upstream submission.
  Validation: Run the public speech admission tests.
  Progress: Admission rejects Silero presets for Qwen3 and extracted Qwen3 voices for Silero.
  Files: `internal/proxy/dictator_grpc_protocol.go`, `internal/proxy/dictator_grpc_e2e_test.go`, `internal/proxy/dictator_grpc_internal_test.go`.
  Resolution: Both incompatible voice types fail before submission. Valid preset and extracted voices pass. Final CI passes.

- [x] [B231] (P1) Restrict speech defaults to synthesis models.
  Evidence: The speech model list includes voice extraction models without synthesis support.
  Requirements: Keep extraction models in discovery and exclude them from synthesis defaults.
  Validation: Run the management API tests with an assigned Dictator connection.
  Progress: The API exposes only Qwen3 and Silero synthesis defaults and rejects Whisper as a speech default.
  Files: `internal/proxy/provider_registry.go`, `internal/proxy/management_capability_defaults_test.go`, `docs/openapi.yaml`, `site/docs/index.html`.
  Resolution: Synthesis defaults exclude Whisper and persist after restart. The schema describes the current list. Final CI passes.

- [x] [B232] (P2) Include image inputs in public capability domains.
  Evidence: GPT-4.1 has image inputs but only the text domain.
  Requirements: Include the image domain for models and offerings with image inputs.
  Validation: Run the public catalog tests.
  Progress: Public models and offerings include image inputs in their domain lists.
  Files: `internal/proxy/public_capabilities.go`, `internal/proxy/public_capability_domains_test.go`.
  Resolution: The public catalog tests prove the image domain for models and offerings. Final CI passes.

- [x] [B233] (P1) Use capability domains in the connection dashboard.
  Evidence: The dashboard has obsolete Dictation and Media tabs and no speech default form.
  Requirements: Use the five public domains and preserve defaults across domain changes.
  Validation: Run desktop and mobile browser tests through the management application.
  Progress: All 133 frontend tests pass. The real-server flow passes at 320, 390, and 1440 pixels.
  Files: `connectionDashboard.js`, `backendClient.js`, `types.d.js`, `styles.css`, `tests/e2e/management-ui.spec.js`, `tests/blackbox/connection-dashboard.spec.js`, `docs/tenant-connections.md`.
  Event contracts: The connection context event keeps its current name and payload fields.
  Resolution: All five domains work on desktop and mobile. Explicit saves preserve other defaults.
  Final `make ci` passes all 14 gates with 100.0 percent Go coverage.
  The result includes 133 frontend tests and seven real-server browser tests. The log is `/tmp/x9c2-ci-complete.log`.

- [x] [B228] (P1) Repair capability-domain routing defaults across the current application.
  Goal:
  Restore the current backend build and preserve the separate text, transcription, and speech defaults introduced by F073.
  Evidence:
  - The current branch fails compilation at commit `6aee2ae366d6b46c06924164a23bd8f3b46980e0`.
  - `TenantDefaults` defines transcription and speech fields, but the store and callers still use removed dictation fields.
  - `make test-upstream-admission` cannot execute the I046 regression because these fields do not compile.
  Requirements:
  - Use the current domain fields throughout request handling, persistence, and test fixtures.
  - Preserve all selected capability defaults through reads, writes, and connection removal.
  - Reject obsolete public fields. Do not restore aliases for removed fields.
  - Keep the OpenAPI schema consistent with the current capability domains.
  Validation:
  - Prove domain-default persistence through the public management API.
  - Run the applicable backend tests before the I046 regression.
  - Run final repository validation at the stack completion checkpoint.
  Progress:
  - The backend now uses separate transcription and speech defaults in the current database schema.
  - Public management tests prove persistence after restart and selective removal of connection defaults.
  - Startup rejects missing current columns and obsolete dictation columns.
  - Management, account-connection, Dictator, and client-contract tests pass with local HTTP, gRPC, and SQLite fixtures.
  - The OpenAPI schema now accepts the capability domains that the server publishes.
  - Final CI found an obsolete `dictator-speech` family in the icon manifest. The current catalog requires `whisper`, `qwen3`, and `silero`.
  - The icon manifest and site renderer now accept the current catalog. Invalid domain lists cause a build error.
  - The management browser uses current transcription fields and preserves separate speech defaults through text and transcription changes.
  - The current catalog counts and browser fixtures pass their focused checks.
  Resolution:
  - Current defaults persist through the API, database, and management browser.
  - Final `make ci` passes all 14 gates with 100.0 percent Go coverage.
  - Validation uses local HTTP, gRPC, SQLite, and Chromium. The full result is in `/tmp/i046-ci-fifth.log`.

- [!] [B210] (P1) Restore local startup with the current TAuth image contract.
  Goal: Start local TAuth with the current OAuth configuration.
  Observed: `ghcr.io/tyemirov/tauth:latest` rejected both OAuth blocks with `field oauth not found` and exited with status 1.
  The browser tests use TAuth `v1.2.7` from `go.mod`. The registry supplies `ghcr.io/tyemirov/tauth:1.2.7` for ARM64 and AMD64.
  Requirements: Use `ghcr.io/tyemirov/tauth:latest` in local Compose.
  Keep the current OAuth configuration.
  Validation: Run `make up`. Check TAuth session and OAuth discovery responses through the frontend. Run `make ci`.
  Investigation: The temporary `1.2.7` image passed local startup, session, OAuth discovery, and all 13 CI gates.
  The user rejected the version pin. Compose again uses `latest`, and the README pin instruction was removed.
  A fresh registry pull returned digest `sha256:bfe6b0ec4e7f3bb5e3934756d456266f603ae4871969e0091025725f527dcc85`.
  The image creation date is July 29, 2026. TAuth added OAuth support in commit `40806aa` on August 8, 2026.
  The current TAuth source and README still require the two OAuth blocks. The package manager resolves `@latest` to `v1.2.7`.
  Removal of these blocks would remove the authorization server required by MCP.
  Validation after pin removal: A fresh `latest` image still rejected both OAuth blocks with status 1.
  `make ci` passed all 13 gates in 320 seconds with 100.0 percent Go statement coverage.
  CI does not establish startup acceptance for the published `latest` image.
  The changed prose has no mechanical findings. Existing guide differences and 71 findings in unchanged tracker text remain.
  Blocked: TAuth publication must supply an image with the current OAuth contract through the `latest` tag.

- [!] [B207] (P1) {I260} Restore production login with matching shared UI assets.
  Goal: Restore Google login on the production website.
  Observed: On September 10, 2026, the live API supplies `auth.providers.google.clientId` with a nonempty Google client ID.
  The published `mpr-ui@latest/mpr-ui-config.js` response identifies version `3.11.11` and requires `auth.googleClientId`.
  Its public `MPRUI.loadYamlConfig` entry point rejects the live YAML with `config-ui.yaml missing auth.googleClientId`.
  The website release marker identifies `v1.9.1` at `60068442d823af78d07bb0de5405485da263c99c`.
  Requirements:
  - Complete the shared publication and cache procedure in mpr-ui I009.
  - Verify that the published loader accepts the live provider map.
  - Verify Google login, session restoration, protected requests, and logout in the production browser.
  Validation:
  - The published loader reproduced the reported config error through its public entry point with the live API response.
  - `make test-shared-ui-config` passed for the current repository config producers.
  - The CDN response declares `max-age=604800, s-maxage=43200`.
  Blocked: The operator must publish the qualified shared UI assets and complete cache convergence under mpr-ui I009.

## Improvements

- [ ] [I288] (P0) Simplify installation of the official Python client.
  Goal: Make the official client easy to install in external applications.
  Requirements:
  - Use the PAL integration evidence in https://github.com/BeehiveInnovations/pal-mcp-server/pull/483.
  - Replace the required Git checkout and GitHub release lookup with a standard Python package installation.
  - Supply wheel and source packages through the repository release process.
  - State the supported Python versions and the current release selection policy.
  - Keep the client optional when a host supports older Python versions or other providers.
  - Coordinate client installation examples with I292 and the Node.js package scope in F016.
  Deliverables:
  - Package metadata, release automation, installation instructions, and package tests.
  - A separate operational record for package publication and package registry access.
  Validation:
  - Install the built package in a clean environment without Git or GitHub CLI.
  - Send a request through the installed official client to a local protocol server.
  - Verify host startup without the optional client.
  - Run the applicable repository checks and record package publication separately.

- [ ] [I289] (P0) Preserve completion metadata through official client results.
  Goal: Let integrations report the actual route, usage, and completion status.
  Requirements:
  - Use the PAL integration evidence in https://github.com/BeehiveInnovations/pal-mcp-server/pull/483.
  - Replace text-only information loss with one typed result contract for the official clients.
  - Include output text, request identifier, actual provider and model, completion status, and available token usage.
  - Represent unavailable usage explicitly. Keep measured usage separate from estimates and calculated cost.
  - Reuse the canonical result from F038 and attribution semantics from F083.
  - Keep durable connection history in F083 and charge calculation in F067.
  - Keep provider credentials and native response bodies outside public results.
  - Update the server, OpenAPI, applicable clients, and examples as one current contract.
  Deliverables:
  - Typed completion results and a reference adapter that uses their metadata.
  Validation:
  - Verify explicit routes, tenant-selected routes, available usage, absent usage, and provider termination through public interfaces.
  - Verify that the reference adapter reports actual results without invented token counts.
  - Run the applicable repository checks.

- [ ] [I290] (P0) Make executable text capabilities usable through official clients.
  Goal: Remove duplicate model catalogs from external integrations.
  Requirements:
  - Use the PAL integration evidence in https://github.com/BeehiveInnovations/pal-mcp-server/pull/483.
  - Inspect existing discovery from F038 and the authoritative provider catalog before changing public interfaces.
  - Expose tenant-authorized executable text offerings through the official clients.
  - Include exact model identity, route eligibility, context limits, output limits, supported controls, and allowed reasoning values.
  - Keep account-visible provider observations distinct from executable tenant offerings.
  - Keep host aliases and subjective model scores in the host application.
  - Preserve one catalog authority and one documented public discovery contract.
  Deliverables:
  - SDK discovery methods, capability types, current API documentation, and a reference adapter example.
  Validation:
  - Enumerate tenant offerings through a real public interface with controlled provider connections.
  - Verify tenant isolation and exclusion of unavailable offerings.
  - Configure valid requests from discovered capabilities without a copied provider catalog.
  - Run the applicable repository checks.

- [ ] [I291] (P0) Unify tenant authentication across official client operations.
  Goal: Use one header-based credential contract for text and media requests.
  Requirements:
  - Use the PAL integration evidence in https://github.com/BeehiveInnovations/pal-mcp-server/pull/483.
  - Replace text query credentials with the canonical bearer key contract already used by media and F038 interfaces.
  - Update the server, OpenAPI, official clients, CLI, examples, and affected consumers together.
  - Remove obsolete query credential handling from the changed interfaces.
  - Preserve tenant authorization and the server-owned provider credential boundary.
  - Keep tenant key creation and retrieval in F081.
  - Remove raw credentials from generated URLs, errors, traces, and access logs.
  Deliverables:
  - One current authentication contract, consumer update instructions, and public boundary tests.
  Validation:
  - Verify text and media authorization with valid, invalid, missing, and cross-tenant credentials.
  - Verify rejection of obsolete credential inputs on the changed interfaces.
  - Inspect captured request URLs and diagnostic output for test credential values.
  - Run the applicable repository checks.

- [ ] [I292] (P0) Make integration guidance fit each language and host.
  Goal: Let external applications use the official clients within their existing architecture.
  Requirements:
  - Use the PAL integration evidence in https://github.com/BeehiveInnovations/pal-mcp-server/pull/483.
  - Separate shared security and validation requirements from language-specific configuration instructions.
  - Supply Go application and Python plugin examples with each host's canonical configuration surface.
  - Preserve PAL environment settings and JSON model metadata without a second YAML hierarchy.
  - Distinguish configured defaults from model and reasoning choices owned by an operation.
  - Define package installation and release selection for each supported language.
  - Align the mpr-integration skill, client guides, and repository examples with the same contract.
  - Coordinate installation instructions with I288 and Node.js documentation with F016.
  - Keep website presentation work in I218.
  Deliverables:
  - Revised integration contracts, skill instructions, and executable examples.
  Validation:
  - Run Go and Python examples through official clients against local protocol servers.
  - Verify startup validation, secret ownership, explicit routing, tenant defaults, and request budgets.
  - Verify that examples use only the host's existing configuration hierarchy.
  - Run the applicable repository and documentation checks.

- [ ] [I293] (P0) Clarify request recovery and retry ownership for integrations.
  Goal: Let callers recover accepted work without duplicate provider execution.
  Requirements:
  - Use the PAL integration evidence in https://github.com/BeehiveInnovations/pal-mcp-server/pull/483.
  - Inspect I229 recovery and current hosted request behavior before extending ordinary text recovery.
  - Define server work budgets, client deadlines, cancellation, request identity, and result retrieval in one contract.
  - Supply typed errors for validation, authentication, throttling, transport failure, and uncertain execution.
  - State when a caller can safely repeat a request and when it must retrieve existing work.
  - Preserve uncertainty when provider execution cannot be established.
  - Reuse F082 configuration error codes rather than creating competing error meanings.
  - Apply the same recovery semantics to all applicable official clients.
  Deliverables:
  - Current recovery documentation, typed client results and errors, and deterministic protocol fixtures.
  Validation:
  - Verify failures before dispatch and lost responses after dispatch through public interfaces.
  - Verify server restart, result retrieval, repeated request identity, and conflicting request content.
  - Prove one provider execution for recoverable repeated requests.
  - Verify the distinction between the server work budget and client transport deadline.
  - Run the applicable repository checks.

- [ ] [I294] (P0) {I288,I289,I290,I291,I292,I293} Qualify official clients through external reference integrations.
  Goal: Detect client integration failures before consumers adopt a release.
  Requirements:
  - Use the PAL integration evidence in https://github.com/BeehiveInnovations/pal-mcp-server/pull/483.
  - Maintain a small reference adapter and shared conformance fixtures for each official client.
  - Extend the I029 conformance checks instead of creating another API authority.
  - Verify installation from built packages and declared dependency ranges in clean supported environments.
  - Cover optional client absence, authentication, route selection, capabilities, result metadata, limits, and recovery.
  - Do a test of the actual client transport against local protocol servers.
  - Keep external model quality checks and production acceptance separate from deterministic integration checks.
  - Record PAL's MCP and Black dependency failures as examples of clean-install qualification requirements.
  Deliverables:
  - Reference integrations, reusable protocol fixtures, runtime matrices, and release qualification checks.
  Validation:
  - Run all applicable client examples from packaged artifacts in clean environments.
  - Verify enabled and disabled optional integrations.
  - Prove that incompatible dependencies or contract changes fail with actionable diagnostics.
  - Run the applicable repository checks and record each tested client version.

- [ ] [I287] (P2) {F028,F029,F030} Umbrella: deliver additional media providers after the retained-provider migration.
  Goal:
  Keep approved provider extensions separate from migration of existing capabilities.
  Execution:
  - Complete F028, F029, and F030 after their retained-provider prerequisites.
  - Hand F028 to MediaOps F022, F029 to MediaOps F023, and F030 to MediaOps F024.
  - Deliver each provider contract and official client before its product integration.
  Validation:
  - Verify each exact provider offering through the public gateway and then its MediaOps consumer.
  - Record provider acceptance and product acceptance separately.
  Scope:
  - This umbrella does not block I274 or MediaOps I092.

- [ ] [I286] (P1) Establish provider ownership for the MediaOps and TelePrompter split.
  Goal:
  Make the gateway boundary executable before provider cutovers.
  Requirements:
  - Own every media-provider API call, credential, native request, upload, resource, history, and recovery path in LLM Proxy.
  - Keep one provider catalog and reuse account connections across text and media offerings.
  - Keep product workflows, projects, assets, authorization, and media processing in the single MediaOps backend.
  - Keep the TelePrompter timeline and prompt website connected to MediaOps.
  - Keep Frame Picker, thumbnails, hover zoom, and other retained MediaOps interfaces outside LLM Proxy.
  - Keep Dictator as a private provider runtime. Keep provider metrics in LLM Proxy.
  - Map each source API method to an executable capability issue and its official client contract.
  Deliverables:
  - Current ownership documents, provider-call inventory, and capability handoff criteria for I274.
  Validation:
  - Compare the inventory with current MediaOps source and public interfaces.
  - Prove that each provider call has one destination owner and one consumer cutover issue.
  Classification:
  - Reclassified from F071. The earlier all-applications-in-MediaOps scope is historical.
  - The proposed gateway Frame Picker was removed before publication. This work preserves that boundary.

- [x] [I285] (P2) Retain browser screenshots only for failed tests.
  Goal: Remove routine screenshot storage after visual review.
  Requirements: Remove manual screenshot captures and the five obsolete tracked PNG files.
  Requirements: Configure both browser suites to retain screenshots only for failed tests in ignored output directories.
  Validation: Verify screenshot output with passing and failing browser tests. Run final CI and check tracked file contents.
  Resolution: Removed manual captures and five tracked PNG files. Both suites use `screenshot: "only-on-failure"` in separate ignored directories.
  Evidence: Temporary browser checks retained one screenshot for each deliberate failure and none for successful tests in both suites.
  Evidence: Final `make ci` passed all 14 gates, including 154 frontend tests and 7 authentication tests. Go statement coverage is 100.0%.
  Evidence: `/tmp/screenshot-i285-integrity.json` records zero screenshots after successful CI and no changes to 621 tracked paths.

- [x] [I284] (P2) Use icons for model task filters.
  Requirements: Replace task labels with icons in one row. Keep accessible names, hover descriptions, and keyboard controls.
  Validation: Check desktop and narrow browser layouts. Run final CI.
  Evidence: `/tmp/model-filters-i284-initial.log` records visible Text content where the test expects an icon.
  Resolution: Icon buttons retain accessible names and hover descriptions. The row scrolls when space is limited.
  Evidence: A focused check found a three-pixel header offset. The corrected checks pass in `/tmp/model-filters-i284-focused.log`.
  Resolution: Shared task filters use one row of icons with accessible names and keyboard controls.
  Validation: Final CI passes all 14 gates, including 151 frontend tests, seven integration browser tests, and 100.0 percent Go coverage.
  Evidence: `/tmp/model-filters-ci-final.log`. Event contracts are unchanged.


- [x] [I283] (P2) Show task icons on model cards.
  Requirements: Use the agreed task icons and descriptions on dashboard and explorer model cards.
  Requirements: Keep compact selector labels unchanged. Show descriptions on hover and keyboard focus.
  Requirements: Keep input and output directions in model details.
  Validation: Check browser rendering, accessible names, and filter behavior. Run final CI.
  Resolution: Dashboard and explorer cards use task icons with hover text and accessible descriptions. Model details retain input/output directions.
  Evidence: `/tmp/i283-initial.log` records the initial missing-icon failures. Focused browser tests pass in `/tmp/i283-focused.log`.
  Validation: The first CI run failed during temporary-directory cleanup in `TestGeminiCurrentModelsVertexAPIKeyConnection`. The isolated test passed without changes.
  Validation: Final CI passes all 14 gates, including 151 browser tests and 100.0 percent Go coverage.
  Evidence: `/tmp/i283-go-recheck.log` and `/tmp/i283-ci-final.log`. Event contracts are unchanged.

- [x] [I282] (P2) Place task selectors beside the Models title.
  Requirements: Put compact task buttons in the Models header. Align the group right and keep the title on its first row.
  Requirements: Wrap buttons within the available header space. Keep labels, hover descriptions, and filter behavior.
  Validation: Check header geometry and desktop and mobile browser renders. Run final CI.
  Resolution: Task selectors sit beside the Models title and wrap within the right side of its header.
  Validation: Header geometry, desktop, mobile, and filter browser checks pass. Final CI passes all 14 gates with 151 browser tests.
  Evidence: `/tmp/i282-initial.log` records the initial alignment failure. `/tmp/i282-ci.log` records final CI and 100.0 percent Go coverage.
  Event contracts: No event contract changed.

- [x] [I281] (P2) Make task filter buttons compact and rectangular.
  Requirements: Use short labels without visible explanations. Keep descriptions in hover text and accessible names.
  Requirements: Use small rectangular buttons with clear pressed states and wrapping at narrow widths.
  Validation: Check desktop and mobile browser renders. Keep combined selection behavior and run final CI.
  Evidence: The initial browser test found visible explanations. The first CI run found an obsolete visible-description assertion.
  Evidence: Updated label and hover-description checks pass in `/tmp/i281-corrected.log`.
  Resolution: Shared task controls use rectangular buttons with short labels, hover descriptions, and accessible names.
  Validation: Desktop and mobile browser checks pass. The refreshed local application shows the compact controls.
  Validation: Final CI passes all 14 gates, including 151 browser tests and 100.0 percent Go coverage.
  Evidence: `/tmp/i281-ci-final.log`. Event contracts are unchanged.

- [ ] [I280] Buy `untzr.ai` and migrate llm-proxy to the new domain.
  Goal: Complete the domain purchase and production migration, with `untzr.ai` as the website and `api.untzr.ai` as the API hostname.
  Requirements:
  - Confirm domain availability, purchase cost, renewal cost, and registrar account with the owner before purchase.
  - Purchase `untzr.ai` through the approved account and record domain ownership and renewal settings.
  - Configure DNS and HTTPS for `untzr.ai` and `api.untzr.ai`.
  - Publish the website at `https://untzr.ai` through GitHub Pages.
  - Route `https://api.untzr.ai` to the production llm-proxy API through the deployment gateway.
  - Update deployment resources, website API configuration, authentication origins, callback URLs, cookies, and CORS settings as applicable.
  - Update supported clients, examples, documentation, and runbooks to use the new canonical URLs.
  - Preserve tenant data, credentials, and access controls through the migration.
  - Remove obsolete domain configuration after the migration.
  - Obtain owner approval for the purchase and production activation.
  Deliverables:
  - Record the purchase, DNS configuration, deployment changes, and production acceptance evidence.
  Validation:
  - Run the applicable repository checks and browser integration tests for the domain changes.
  - Verify public DNS resolution and valid HTTPS certificates for both hostnames.
  - Verify the website, sign-in flow, and authenticated API requests through the new public URLs.
  - Verify website publication through `/.mprlab-release.json`.
  - Record repository validation and production acceptance separately.

- [x] [I279] (P2) Clarify model tasks with compact labels and input/output icons.
  Goal: Distinguish model tasks, accepted inputs, and produced outputs through a compact visual language.
  The current Image tab includes models that accept images and produce text.
  This issue refines the existing model discovery UI and the capability separation from F073.
  Its task selector replaces the tab navigation specified in F073.
  Requirements:
  - Replace the dashboard capability tabs with one compact Task selector.
  - Keep a text label in the selector and show a full task description in its menu.
  - Use these compact labels and descriptions:
    - `Text`: Generate text.
    - `Vision`: Understand images.
    - `Images`: Generate images.
    - `Editing`: Edit images.
    - `Transcribe`: Transcribe audio.
    - `Speech`: Generate speech.
    - `Video analysis`: Understand video.
    - `Video`: Generate video.
  - Determine task eligibility from the exact provider offering and supported operation.
  - Let a model appear under each task that its provider offering supports.
  - Define supported inputs and outputs for each task explicitly.
  - Keep image editing and other tasks distinct even when their input and output types match.
  - Limit available tasks to operations supported through LLM Proxy.
  - Show an input-to-output icon strip below the model name in the same position on each card.
  - Use consistent text-line, picture, waveform, and filmstrip icons for text, image, audio, and video.
  - Use an arrow to show the input-to-output direction.
  - Show only the inputs and outputs relevant to the selected task on each card.
  - Show every supported task and its respective inputs and outputs in model details.
  - Explain required and optional inputs in model details.
  - Use neutral modality icons and reserve teal for selection and connection state.
  - Give each icon an accessible name, such as `Image input` or `Text output`.
  - Show icon explanations on hover and keyboard focus, and make details available by touch.
  - Keep task meaning available through text and accessible names independently of icon shape or color.
  - Preserve the existing tenant, connection, and model default actions under the new task selector.
  - Use the same task vocabulary and icon meanings in the public model explorer.
  - Provide separate Input and Output filters in the public model explorer.
  Deliverables:
  - Produce reviewable views of the current dashboard, expanded task menu, and model details before implementation.
  - Update the shared task definitions, dashboard, model explorer, and affected capability contracts.
  - Update browser tests and the current model discovery documentation.
  Validation:
  - Verify that Kimi vision offerings appear under `Vision` with text and image inputs producing text.
  - Verify that image generation results include only offerings with an image output operation.
  - Verify generation, editing, transcription, speech, and video task mappings against supported provider offerings.
  - Verify that a multimodal card shows the selected task without implying unsupported input/output combinations.
  - Verify that model details show all supported tasks and identify required and optional inputs.
  - Verify combined Input and Output filters in the public model explorer.
  - Verify keyboard navigation, focus explanations, accessible names, touch access, and readable desktop and mobile layouts.
  - Verify tenant selection, connection assignment, and applicable model default actions through real browser flows.
  - Run the applicable repository validation after the final implementation change.
  Resolution: 2026-09-21.
  - Added shared task definitions, directional modality icons, and required/optional input details in `modelTasks.js`.
  - Replaced dashboard tabs with a compact task selector and preserved explicit model default saves.
  - Added task, Input, and Output filters to the public explorer with exact offering matching.
  - Updated dashboard and explorer browser coverage, documentation, styles, and rendered acceptance images.
  - `make ci` passed all 14 gates, including 149 frontend browser tests and 100.0% Go statement coverage.
  - All seven TAuth management browser tests passed with local services and controlled provider responses.
  - Changed prose and `git diff --check` passed.
  - Existing Governor differences remain in `AGENTS.DOCKER.md`, `PLANNING.md`, and `POLICY.md`.
  - Event contracts: The existing connection context event and public API contracts remain unchanged.


- [ ] [I278] (P1) {I277,F081,F082,F083} Verify the complete tenant API access flow.
  Goal: Provide durable tenant API access with clear configuration errors and historical connection attribution.
  Requirements:
  - Execute the work in this order: I277, F081, F082, F083, then I278 acceptance.
  - Use I277 for the tenant name and optional existing connection selector.
  - Use F081 for automatic key creation, encrypted storage, later retrieval, and key replacement.
  - Use F082 for distinct authentication and configuration errors.
  - Use F083 for saved request attribution and connection reporting.
  - Keep implementation requirements in their respective issues.
  - Close this umbrella only after its dependencies and combined acceptance pass.
  Deliverables:
  - Record the completed sequence and combined acceptance evidence in this issue.
  Validation:
  - Create a tenant with an existing connection and copy its automatically created key.
  - Close the dialog, reload, and copy the same key through the tenant API access action.
  - Repeat retrieval in a new authenticated browser session.
  - Create a tenant without a connection and verify its key is available immediately.
  - Send a generation request and verify the configuration error from F082.
  - Assign a connection and save the required model default, then repeat the request successfully.
  - Verify the actual connection in request details and usage reporting.
  - Change the tenant assignment and verify that previous request attribution remains intact.
  - Use automated browsers and real public API entry points for combined acceptance.

- [ ] [I277] (P2) Simplify the tenant creation dialog and connection selection.
  Goal: Create a tenant with an existing connection by default, with an explicit option to leave the connection blank.
  The supplied screenshot shows a `Next step` selector instead of a selector for a specific connection.
  The current form also offers `Create a new connection` and opens a separate connection form after tenant creation.
  Requirements:
  - Show the tenant name and an optional connection selector in the tenant creation dialog.
  - List the account's existing connections by name and provider.
  - Show the default connection as the initial selection when it is available.
  - Let the user select another existing connection or explicitly select no connection.
  - Assign the selected connection when the user creates the tenant.
  - If the selection is blank, create the tenant without a connection assignment.
  - If the account has no connections, permit tenant creation with a blank selection.
  - Remove the new connection option and its automatic form transition from tenant creation.
  - Keep connection creation in the separate connection management action.
  - Show the saved tenant and its actual connection assignment after creation.
  Open Decisions:
  - Define which existing connection is the default when the account has multiple connections.
  - Define the initial selection when connections exist but no default connection is available.
  Deliverables:
  - Update the tenant dialog, browser tests, and `docs/tenant-connections.md`.
  Validation:
  - Use automated browser tests with the real management API.
  - Verify the default selection, another existing connection, an explicit blank selection, and an account without connections.
  - Reload the page and verify the saved tenant assignment for each case.
  - Verify that tenant creation neither creates a connection nor opens a connection creation form.
  - Verify keyboard operation and the rendered dialog at desktop and mobile viewport sizes.

- [x] [I276] (P1) Separate hosted backend gates to complete within the job limit.
  Evidence: PR #335 run `35567984432` cancels the backend job at its ten-minute limit.
  Evidence: Admission race tests pass. The full Go suite starts after five minutes and cannot finish before cancellation.
  Goal: Start coverage independently from the other backend gates.
  Requirements: Put the other backend gates in a separate job. Preserve each local CI gate exactly once across hosted jobs.
  Requirements: Give coverage fifteen minutes for setup, compilation, and the existing ten-minute Go test limit. Keep other qualification jobs at ten minutes.
  Requirements: Require all qualification jobs to succeed before the required test check passes.
  Evidence: Run `35570475157` passes frontend and backend supporting checks. Coverage still exceeds the ten-minute job limit.
  Resolution: Go coverage starts in its own job with a fifteen-minute budget. A separate job runs the other backend gates.
  Validation: The public Make test passes. All 125 aggregate result combinations pass. Each local gate occurs exactly once across hosted jobs.
  Validation: Final local CI passes all 14 gates in 386 seconds with 100.0 percent Go coverage.
  Evidence: `/tmp/llm-proxy-i276-budget-initial.log`, `/tmp/llm-proxy-i276-budget-focused.log`, and `/tmp/llm-proxy-i276-final-ci.log` record local validation.
  Delivery: GitHub checks on PR #335 record hosted validation after the push.
  Files: `.github/workflows/test.yml`, `Makefile`, `README.md`, and the hosted CI contract tests. Public API and event contracts remain unchanged.

- [x] [I275] (P1) Make queue saturation acceptance independent of request timing.
  Evidence: PR #335 run `35556331424` fails in `TestIntegrationHighLoadQueue` under the race detector.
  Evidence: `high_load_queue_test.go:116` reports `queue-full response was not observed`. The frontend job passes.
  Cause: A 25 ms request deadline can expire before admission. Concurrent request launch does not prove queue occupancy.
  Evidence: The queued request returns HTTP 499 after 25 ms. The active request then reaches its one-second timeout.
  Requirements: Wait for active and queued admission before the excess request. Preserve real HTTP requests and the production scheduler.
  Requirements: Remove saturation-specific request deadlines. Keep bounded assertion waits and release blocked requests during failure cleanup.
  Validation: Verify one HTTP 503, two successful admitted requests, and repeated race execution with different processor counts.
  Validation: Run final local CI. Report GitHub validation separately for the updated PR commit.
  Resolution: The test waits for active and queued admission, verifies one HTTP 503, and releases both successful requests.
  Resolution: Failure cleanup releases blocked requests. The race target accepts optional stress-test arguments.
  Validation: All 60 race runs pass with one, two, and four processors. Final local CI passes all 14 gates.
  Validation: Go coverage remains 100.0 percent. All 148 frontend tests and seven service-backed browser tests pass.
  Evidence: `/tmp/llm-proxy-pr335-queue-stress.log` and `/tmp/llm-proxy-pr335-i275-ci.log` record the local checks.
  Delivery: Commit `47d24baa` is on PR #335. Its hosted race tests pass in run `35567984432`. I276 addresses the later job cancellation.
  Files: `tests/integration/high_load_queue_test.go`, `Makefile`. Public API and event contracts remain unchanged.

- [x] [I273] (P1) Show all catalog families before route filters are selected.
  Goal: Let visitors discover text and media models from the same initial view.
  Evidence: The route explorer selects Text and Proprietary at first load. Media families remain hidden.
  Requirements: Select all capabilities and both weight access types at first load.
  Requirements: Derive every family, exact model, and provider offering from the validated catalog.
  Requirements: Keep explicit capability filters and show distinct image generation and image input labels.
  Validation: Verify initial visibility, filter changes, media route selection, and the page without JavaScript through browser tests.
  Resolution: The initial view selects all capabilities and both weight access types. Explicit filters remain available.
  Validation: Final `make ci` passed all 14 gates, including 135 frontend tests and seven local TAuth browser tests.
  Files: `scripts/render_public_site.mjs`, `site/assets/llm-proxy/js/constants.js`, `site/assets/llm-proxy/js/ui/routingTree.js`, browser tests, and `README.md`.
  Event contracts: No changes.

- [ ] [I274] (P1) {I286,F072,F042,F043,F040,F025,F026,F027,I244} Umbrella: migrate all retained media-provider APIs.
  Goal:
  Deliver every retained media-provider capability through the gateway and official clients.
  Source:
  - docs/media-provider-completeness.md records the nine-provider inventory.
  - docs/mediaops-model-access-boundary.md records ownership and cross-repository execution order.
  Execution:
  - Complete I286 before provider cutovers.
  - Reuse completed F022, I046, I216, F024, F039, and F041 foundations. Verify their current contract before reuse.
  - Complete F072 and F042 for Dictator, then F043 and F040 for staging and Vertex images.
  - Complete F025, F026, and F027 sequentially for video, ElevenLabs, and avatar or account operations.
  - Hand each accepted capability to its named MediaOps cutover issue without waiting for this umbrella to close.
  - After MediaOps I088 supplies its final receipt, complete I244.
  Requirements:
  - Include OpenAI, Vertex, FAL, Runway, xAI, ElevenLabs, HeyGen, Kling, and Dictator.
  - Include every retained model, public provider method, supported control, and recovery path.
  - Include discovery, voices, account resources, histories, dictionaries, uploads, quotas, and mutations.
  - Keep configs/providers.yml as the single provider and model inventory.
  - Reuse one provider identity and account connection across its capability branches.
  - Deliver public API, official clients, provider protocol tests, and documentation in each capability issue.
  - Preserve MediaOps application data and processing. Keep TelePrompter connected to the MediaOps API.
  Validation:
  - Prove public provider contracts, tenant isolation, capacity, recovery, cancellation, and artifact integrity.
  - Record each source capability as accepted or still open. Close only after every retained capability has evidence.
  - Use the MediaOps I088 receipt for source retirement and I244 for import-tool removal.
  Delivery:
  - Record source validation, client publication, runtime activation, and paid acceptance separately.
  - Additional providers under I287 and local inference planning under P008 do not block this migration.

- [x] [I272] (P1) Run Python and Go CI checks at the same time.
  Goal:
  Complete the full current validation suite within the 350-second command limit.
  Resolution (2026-09-19):
  - Python client, Go integration, and admission race checks now run at the same time.
  - Runner integration tests verify child failures, temporary file retention, and completion evidence.
  - Final `make ci` passed all 14 gates in 334 seconds, with 100.0% Go statement coverage.
  Evidence:
  - F039 validation reached the authentication browser suite before the command terminated at 350 seconds.
  - Go coverage, 60 Python checks, and 129 browser tests passed before termination.
  Requirements:
  - Run Python client checks at the same time as Go integration and admission race checks.
  - Preserve all gates, failure statuses, output, temporary file removal, and completion receipts.
  - Wait for each child before removal of temporary files or completion.
  Validation:
  - Add a failing runner test for checks that start at the same time.
  - Verify that child failures prevent a success receipt.
  - Run `make test-ci-runner` and final `make ci`.

- [x] [I270] (P2) Auto-refresh dashboard summaries and show the tenant label.
  Goal: Keep dashboard summaries current without a manual refresh control.
  Requirements:
  - Show `Tenant` beside the usage scope selector on desktop and mobile.
  - Refresh the active usage or administrator summary every 30 seconds while authenticated.
  - Remove the manual `Refresh` control.
  - Preserve the selected tenant and interval during automatic refresh.
  Deliverables: Shared management markup, styling, refresh lifecycle, and browser coverage.
  Validation: Run the focused management browser tests, frontend lint, and final `make ci`.
  Progress: B228 repairs the compiler failure. The refresh test now resets its response fixture for each viewport.
  The header test checks geometry with a visible error notice. Both focused browser tests and final CI now pass.
  Resolution: Desktop and mobile browser tests verify the tenant label and automatic refresh. Final CI passes all 14 gates.

- [x] [I269] (P2) Isolate temporary Python repositories from retained uv Git caches.
  Observed: Three selected-version installation tests returned `0.0.post1.dev2` versions during I268 validation.
  The cached `v1.5.0` tag pointed to commit `be24384`. The temporary source repository had that tag on commit `ef298f1`.
  Git could not describe the installed commit from the cached tags. All five package tests passed on an unchanged rerun.
  Goal: Give each temporary source repository an independent cache identity.
  Deliverables: `tests/python_package_contract_test.py`.
  Validation: Reproduce installation with retained Git cache data. Run `make python-package-install-test` and final `make ci`.
  Evidence: `/tmp/llm-proxy-i268-ci.log` and `/tmp/llm-proxy-i268-package-recheck.log`.
  Progress: The media migration CI run reproduced all three selected-release failures in `/tmp/i046-ci-second.log`.
  Each temporary source repository now has a unique Git URL. The retained uv cache remains available.
  Resolution: All five package installation tests pass with the retained cache. Final `make ci` passes all 14 gates.

- [!] [I271] (P0) Remove completed migration paths and initialize only the current schema.
  Goal:
  Remove obsolete database migrations, predecessor records, and migration-only provider rules after their data transfer is completed.
  Keep one current schema for normal application startup and all new databases.
  The user requested P0 removal on 2026-09-13 after the F064 investigation found a Singapore-only migration rule.
  Evidence:
  - `internal/proxy/management_store.go` contains `managedProviderBaseURL` and `dashScopeWorkspaceHostSuffix` with a Singapore-only URL rule.
  - `managedProviderSettingsFromRecordsForSchema` calls this validator for predecessor provider-key records.
  - Current account connections use catalog field validation in `internal/proxy/account_connections_store.go`.
  - The current `managedAccountConnectionsSchemaVersion` is 17.
  - `initializeManagedTenantSchema` calls `initializeManagedTenantSchemaRecords` and `migrateAccountConnections` for versions below 17.
  - A fresh database enters that branch, creates intermediate tables, records version 16, and then enters the account-connection migration.
  - The predecessor initializer also retains a version switch and provider retirement migrations for older databases.
  - These paths remain reachable in source. Their presence alone does not prove that retained production databases still require them.
  - F063 records development completion. It does not establish migration completion for every retained database.
  Requirements:
  - Trace production callers for each predecessor initializer, migration, record type, validator, constant, and catalog migration entry.
  - Separate fresh-database dependencies from data transfers that remain necessary for a retained database.
  - Record each retained database's actual shape and remaining predecessor records before removal of its required transfer path.
  - Treat historical version records as inventory evidence only for the predecessor transfer.
  - Use a versionless current schema. Extend its declared shape when the product adds fields or tables.
  - Validate current tables and records instead of a schema number.
  - Use authorized read-only evidence or an existing operator receipt for that inventory.
  - If a transfer remains necessary, specify its exact input, owner, completion receipt, and bounded execution procedure.
  - Complete each necessary one-off transfer before removal of its bridge.
  - Create fresh databases directly in the current schema without intermediate tables or historical migration execution.
  - Make current-schema restart validate and use the existing records without replay of completed transfers.
  - Reject obsolete persisted shapes at the database boundary after removal of their transfer paths.
  - Remove completed migration functions and their exclusive callers, record types, constants, fixtures, and configuration.
  - Remove the Singapore-only migration validator when its last required caller is removed.
  - Keep current provider URL validation in the canonical provider catalog.
  - Preserve current account connections, tenant assignments, encrypted credentials, access keys, defaults, prompts, timestamps, and usage history.
  - Preserve historical usage identities that the current product still requires.
  - Remove historical model-migration declarations only after analysis of their remaining runtime consumers.
  - Replace migration-only test expectations with public acceptance of fresh startup, current restart, and obsolete-schema rejection.
  - Update the current schema and operator documentation in `README.md` and `docs/tenant-connections.md`.
  - Keep I244 responsible for the separate MediaOps import bridge.
  Deliverables:
  - Record the removed migration paths and the evidence that permits each removal.
  - Record each temporarily required transfer with its exact remaining prerequisite and removal condition.
  - Implement direct current-schema initialization and remove completed predecessor code.
  - Update the affected tests, catalog declarations, and current documentation.
  Validation:
  - Add public integration scenarios for fresh database creation and restart with current account and tenant data.
  - Confirm fresh startup creates only current tables without a migration-version table.
  - Confirm restart preserves credentials, ownership, assignments, defaults, and usage through public APIs.
  - Confirm obsolete or unsupported schemas fail with a contextual error before data changes.
  - Confirm transactional failure preserves the original database during any remaining authorized one-off transfer.
  - Search production code and tests for removed symbols and obsolete record shapes.
  - Run `make test-account-connections` and the applicable `make test-management-contracts` scenarios.
  - Run `make ci` after the last application change under the repository validation policy.
  - Report code removal, repository validation, data-transfer completion, and production acceptance as separate results.
  Progress 2026-09-13:
  - The user selected a versionless schema during implementation.
  - Empty databases now enter direct current-schema creation.
  - Current account-connection databases enter record validation without a version query or historical transfer.
  - Current-schema acceptance rejects predecessor credential and temporary transfer tables without removal of their data.
  - Public regression coverage checks startup, restart, schema creation failure, and retained data after rejection.
  - The production inventory found one account, three tenants, ten credential-field records, ten profiles, and 4,684 usage records.
  - The production database still uses predecessor tenant connections. Its last historical version record is 16.
  - The ignored local database has one unclaimed static owner, two provider-key records, and one usage record.
  - The local Compose volume already has current account connections for two tenants.
  - Removed the historical fresh-initialization branch and its create-and-drop expectation.
  - Remaining transfer tests now create explicit predecessor fixtures.
  - Read-only inventory and the remaining transfer procedure are in [the schema transition record](../docs/managed-schema-transition.md).
  Validation 2026-09-13:
  - The initial fresh-start regression returned `schema versions=[16 17], want only [17]` before the versionless decision.
  - The versionless regression then rejected the unwanted migration-version table.
  - The initial unrecognized-database regression returned `startup error=<nil>`.
  - Initial CI found three obsolete fixture expectations for version creation, predecessor-table deletion, and numeric current-schema rejection.
  - The corrected Go suite passed its assertions but reported two uncovered predecessor failure paths.
  - Public startup tests now verify rollback after predecessor-query and post-transfer validation failures.
  - Final `make ci` passed all 13 gates in 302 seconds with 100.0 percent Go statement coverage.
  - Evidence: `/tmp/llm-proxy-i261-ci-final.log`.
  - Scoped prose checks, issue-ID checks, and `git diff --check` passed.
  - Governor reported existing managed-content differences in `.mprlab/POLICY.md` and `.mprlab/AGENTS.DOCKER.md`.
  - No production transfer, deployment, or retained-data deletion occurred. Public HTTP and event contracts remain unchanged.
  Changed files:
  - `internal/proxy/management_store.go` and `internal/proxy/account_connections_store.go`.
  - `internal/proxy/account_connections_schema_test.go` and `internal/proxy/account_connections_test.go`.
  - `internal/proxy/account_connections_migration_internal_test.go` and `internal/proxy/account_connections_migration_failures_internal_test.go`.
  - `internal/proxy/management_gorm_internal_test.go`, `internal/proxy/management_provider_connections_migration_internal_test.go`, and `internal/proxy/management_tenants_internal_test.go`.
  - `README.md`, `docs/tenant-connections.md`, `docs/managed-schema-transition.md`, and `.mprlab/TERMINOLOGY.md`.
  Blocked: Complete the production connection transfer and decide ownership or disposal of the local predecessor database before bridge removal.

- [!] [I260] (P1) Prepare the current shared UI migration.
  Goal:
  Use the current shared authentication config, footer menu, and protected request transport throughout the browser frontend.
  Requirements:
  - Convert the API producer and static config to `auth.providers` with explicit session paths.
  - Preserve environment-owned Google identifiers and supported origins.
  - Keep every shared asset URL on literal `@latest`.
  - Convert the shared footer generator and its generated pages together.
  - Use shared session recovery for protected management requests.
  - Preserve authorization before domain mutations.
  - Qualify the same final shared candidate as the other mpr-ui I009 consumers.
  Deliverables:
  - Update producers, consumers, tests, generated pages, and current API documentation together.
  - Record candidate digests, CI results, public observations, and publication dependencies.
  Validation:
  - Verify actual YAML through HTTP and the static config entry point.
  - Verify login, restoration, read recovery, mutation recovery, logout, and footer use through the real application.
  - Use controlled external provider responses for local qualification.
  - Preserve repository coverage requirements.
  - Keep user-owned publication and real Google acceptance as separate gates.

  Progress:
  - Both real HTTP cases failed against the flat producers before the source changes.
  - The API and static config now pass the nested provider-map regression.
  - The logs are `/tmp/llm-proxy-i260-config-red.log` and `/tmp/llm-proxy-i260-config-green.log`.
  - Both producers, protected transport, and all 52 generated footers now use the current shared contract.
  - The browser checks use verified candidate `768f25936497c5aabd426197d21c2100b6e5d9a1`.
  - Final native CI passed all 12 gates with 100 percent Go coverage, 114 browser tests, and six TAuth/MCP scenarios.
  - The final log is `/tmp/llm-proxy-i260-ci-final3.log`.
  - The migration record contains asset digests, public observations, and activation instructions.
  Blocked: Complete central mpr-ui I009 final-candidate qualification, coordinated publication, cache convergence, and real Google acceptance.

- [ ] [I259] (P2) Use one Responses codec with explicit protocol variations.
  Goal:
  Reduce repeated request construction and result parsing across OpenAI, xAI, and DashScope Responses routes.
  This refactor follows the catalog and transport improvements. It is not a prerequisite for F022.
  Requirements:
  - Compare the current Responses implementations against provider documentation and I256 characterization evidence.
  - Extract equal request, output, tool, error, and usage behavior into a shared Responses codec.
  - Represent each required difference as a closed protocol variation with one documented meaning.
  - Keep OpenAI background execution and its resource lifecycle.
  - Keep synchronous xAI and DashScope requests with explicit `store:false` and omission of `background`.
  - Keep image detail fields, caller tools, structured output support, reasoning controls, and native usage interpretation by route.
  - Keep xAI output-limit reason checks and DashScope terminal incomplete behavior.
  - Keep unsupported output rejection and provider-private reasoning protection explicit.
  - Remove replaced implementations, duplicate schema facts, and obsolete registrations after equivalent behavior passes.
  - Keep current provider identifiers and tenant records unchanged.
  - Update I038 and I041 implementation descriptions to reference the shared codec and their required protocol variations.
  - Keep their separate live acceptance requirements and recorded evidence.
  Deliverables:
  - A shared Responses implementation, typed variations, catalog registrations, and current provider documentation.
  Validation:
  - Run I256 and existing provider HTTP tests before refactoring.
  - Compare requests, results, errors, continuation, polling, and usage for every affected active offering.
  - Exercise each supported variation through the same shared public test suite.
  - Prove that another provider with an existing variation needs only catalog data and connection values.
  - Run focused Responses tests and applicable repository checks. Record live-provider acceptance separately.

- [ ] [I255] (P1) Distinguish Gemini model-operation failures from rejected credentials.
  Goal:
  Improve the existing connection verification error classification.
  Requirements:
  - Preserve bounded upstream error details at the Gemini verification boundary.
  - Distinguish explicit credential rejection from an unsupported model operation.
  - Keep provider secrets and raw provider responses out of public errors and logs.
  - Preserve the current atomic connection-save contract after failed verification.
  - Use explicit error classification without another provider or operation attempt.
  Evidence:
  P012 captured HTTP 400 when Flash 3.8 rejected a background interaction with the required API revision.
  The verification code currently discards error bodies and maps all upstream HTTP 400 responses to `provider_key_rejected`.
  The original uninstrumented HTTP 422 has no captured upstream response. Its exact cause remains unproven.
  Deliverables:
  Bounded error classification, public integration tests, and current error documentation.
  Validation:
  Prove the different outcomes for invalid credentials and unsupported operations through public management HTTP requests.
  Prove that both failures preserve the previous connection and its settings.

- [ ] [I244] (P1) {F025,F026,F027,F040,F042,F043,I088@MediaOps} Remove the completed MediaOps operation-import bridge.
  Current execution contract:
  - Require the final MediaOps I088 receipt, including accepted zero-count provider families.
  - Keep I274 and MediaOps I092 after this cleanup. They must not be prerequisites for the receipt.
  - Preserve application project and asset records in MediaOps.
  Goal:
  Leave only the canonical model-operation contract after migration of every
  selected MediaOps provider record.
  Cross-repository prerequisite:
  - MediaOps I088 must produce the operator-held final per-tenant migration
    receipt and prove that every eligible legacy record is migrated or
    explicitly terminal and locally complete.
  Requirements:
  - Reconcile the MediaOps receipt with gateway operation IDs, source-record
    digests, provider families, terminal classifications, and rejection counts.
  - Remove the operator-only import command, manifest schemas, provider-family
    import registrations, migration-only configuration, and bridge docs.
  - Keep imported rows only in the current canonical operation schema; remove
    legacy discriminators and source-record shapes after receipt verification.
  - Prove the public service exposes no import endpoint and every new operation
    enters through idempotent operation creation.
  Validation:
  - Run static contract checks and public black-box tests proving no migration
    entrypoint or legacy record shape remains and migrated operations retain
    status, tenant isolation, recovery, and artifact behavior.
  - Start with the required failing integration test. Complete validation under the current repository policy.
  Classification: Reclassified from M021. Implementation remains open.
  Inventory rule:
  - Accept an explicit zero-count receipt for a family with no recoverable source records.
  - Remove only import tooling actually introduced by the selected capability migrations.

- [ ] [I241] (P1) Show provider requests over time on each provider card.
  Goal:
  Each provider card shows request activity across the selected Usage time
  span. The current `Request volume` meter compares one provider total with
  the largest provider total. It does not show when the requests occurred.
  The replacement is a miniature `Requests over time` chart.
  Requirements:
  - Use the current Usage tenant scope and interval for each provider chart.
  - Update each chart from the accepted Usage summary response.
  - Do not make a separate browser request for a provider chart.
  - Keep account-wide aggregation in one database operation.
  - Use the same captured server time for summary and provider buckets.
  - Extend authenticated Usage provider entries with required
    `request_buckets` data.
  - Define each request bucket with one RFC 3339 `start` and one nonnegative
    integer `requests` value.
  - Align each provider request bucket with the corresponding top-level Usage
    bucket.
  - Include one provider request bucket for each top-level bucket.
  - Preserve each zero-valued provider request bucket.
  - Keep provider aggregate totals in the existing `data` object.
  - Define a separate OpenAPI provider-series schema for authenticated Usage
    summaries.
  - Keep the aggregate administrator Usage response unchanged.
  - Validate the new response data once in the browser backend adapter.
  - Reject missing, repeated, unordered, or misaligned provider bucket data.
  - Reject a provider request sum that differs from its aggregate request
    total.
  - Build provider bucket data during the existing usage-record aggregation
    pass.
  - Do not add a presentation-specific endpoint or persisted chart data.
  - Remove the comparison with the provider that has the largest total.
  - Remove the old percentage, track, fill, copy, markup, and CSS contract.
  - Label the replacement chart `Requests over time`.
  - Show the active interval label in the chart header.
  - Use the accepted summary bucket order without interpolation or smoothing.
  - Use a compact semantic SVG line chart with a restrained area fill.
  - Keep the SVG plot within the current `2.25rem` graph height.
  - Use current chart, surface, and border tokens in each theme.
  - Do not use color as the only activity indicator.
  - Use a provider-local Y scale that starts at zero.
  - Keep a zero series on the baseline without a false nonzero range.
  - Build a zero series from the top-level buckets for a catalog provider with
    no provider aggregate.
  - Show the existing unavailable state when the Usage summary is unavailable.
  - Show an empty time-span state when an all-time summary has no buckets.
  - Do not show visible axes or tick labels in the miniature chart.
  - Keep the exact request total in the existing provider activity row.
  - Give each chart an accessible provider, scope, interval, and metric name.
  - Make each exact UTC bucket start and request count available to assistive
    technology.
  - Do not require pointer hover to get an exact bucket value.
  - Reuse the canonical UTC bucket-label function.
  - Keep all chart copy in the centralized frontend copy contract.
  - Replace a chart only after the selected scope and interval response wins
    the current request identity check.
  - Preserve the selected Usage tenant during an interval change.
  - Preserve the selected interval during a Usage tenant change.
  - Keep each provider card size and the responsive provider grid unchanged.
  - Keep the miniature chart inside the card at every supported viewport.
  - Preserve the provider settings, card flip, and catalog membership
    contracts.
  - Update the OpenAPI contract and the managed usage implementation document.
  - Update generated public usage content through its owning generator when
    that content describes provider cards.
  - Do not add a compatibility response, optional legacy shape, or UI fallback.
  Deliverables:
  - Add typed provider request buckets to authenticated Usage summaries.
  - Add one shared miniature time-series presentation transform.
  - Replace the provider request meter with the compact semantic SVG chart.
  - Remove all obsolete request-meter code and styles.
  - Add public API, real-store, and browser regression coverage.
  - Update current contract documents and applicable generated resources.
  Validation:
  - Prove that `1d` uses the same 24 starts in top-level and provider series.
  - Prove that other intervals align top-level and provider daily starts.
  - Prove that account and tenant scopes return their exact provider values.
  - Prove that each provider series sum equals its aggregate request total.
  - Prove that zero, flat, and single-spike series produce valid SVG geometry.
  - Prove that a provider without usage shows an exact zero series.
  - Prove that an empty all-time result does not show false activity.
  - Prove that a scope change updates every provider chart without page reload.
  - Prove that an interval change updates every provider chart without an
    additional HTTP request.
  - Prove that a stale response cannot replace the current provider charts.
  - Prove that the UI contains `Requests over time` and no `Request volume`.
  - Prove that assistive technology can read each exact bucket value.
  - Prove that provider cards do not overflow desktop or narrow viewports.
  - Prove that the administrator Usage response retains its current schema.
  - Validate the updated OpenAPI document against real HTTP responses.
  - Run `make ci` after the last application change.
- [!] [I234] (P1) Restore Gemini 3.1 Pro Preview after live acceptance.
  Goal:
  Restore the exact upstream route only after its current Google contract
  passes.
  Evidence:
  - Google publishes `gemini-3.1-pro-preview` as the only Gemini 3.1 Pro API
    model. Google does not publish a stable `gemini-3.1-pro` model ID.
  - I232 removed the preview route after two provider verification requests
    returned HTTP 429.
  Requirements:
  - Do a test of the omitted, `low`, `medium`, and `high` thinking levels.
  - Prove background completion, active retrieval, cancellation, and deletion.
  - Restore only the exact `gemini-3.1-pro-preview` model ID after all live
    checks pass.
  - Do not add a `gemini-3.1-pro` alias or change the Gemini default model.
  - Keep the completed schema-version-11 migration unchanged.
  - Restore the catalog, public capabilities, route constants, tests, and
    current documentation together.
  Validation:
  - Run the exact paid candidate acceptance before source changes.
  - Run `make ci` after the last application change.
  Blocked: The omitted-thinking acceptance request returned HTTP 429. The test
  stopped before the other thinking levels and background lifecycle. The
  provider catalog remains unchanged.
  Progress (2026-09-05):
  The standard candidate harness now accepts `gemini-3.1-pro-preview` as an explicit model selection.
  A new CLI case first failed with `unsupported Gemini candidate model: gemini-3.1-pro-preview`.
  The corrected harness passed the local omitted, `low`, `medium`, `high`, completion, active retrieval, cancellation, and deletion matrix.
  The paid check used `configs/.env` through the standard credential loader.
  The first omitted-effort request again returned HTTP 429. The remaining live checks did not run.
  The route remains absent, and the default and schema-version-11 migration remain unchanged.
  Evidence: `/tmp/llm-proxy-i234-candidate-green.log` and `/tmp/llm-proxy-i234-live-current.log`.
  Final CI passed all 12 gates, 97 browser tests, and 100.0% Go statement coverage in 235 seconds.
  Evidence: `/tmp/llm-proxy-i234-ci.log`. The harness and documentation changes introduce no event contract.
  Root cause diagnosis (2026-09-06, I251):
  The provider reports zero free-tier input-token quota and zero free-tier request quota for the `gemini-3.1-pro` quota group.
  The requested exact API model remains `gemini-3.1-pro-preview`.
  The process has no Gemini key. The repository's two populated environment inputs contain one distinct key.
  The operator must supply a project with nonzero quota before the full candidate acceptance can run.
  Evidence: `/tmp/llm-proxy-i234-diagnosis.log`.
- [!] [I228] (P1) Add current MiniMax text model offerings.
  Goal:
  Give managed tenants access to the current MiniMax M2 text models through
  the existing direct MiniMax provider connection.
  Evidence:
  - The current catalog contains only exact model `minimax-m2.7`.
  - MiniMax documents these OpenAI-compatible model identifiers:
    `MiniMax-M2.7`, `MiniMax-M2.7-highspeed`, `MiniMax-M2.5`,
    `MiniMax-M2.5-highspeed`, `MiniMax-M2.1`, `MiniMax-M2.1-highspeed`, and
    `MiniMax-M2`:
    https://platform.minimax.io/docs/api-reference/text-openai-api
  - MiniMax documents `https://api.minimax.io/v1/chat/completions` as the
    OpenAI-compatible text endpoint:
    https://platform.minimax.io/docs/api-reference/text-chat-openai
  - The current Chat Completions reference gives M2 models a 204,800-token
    completion maximum. The PAYG page publishes standard rates for all seven:
    https://platform.minimax.io/docs/guides/pricing-paygo
  Requirements:
  - Keep `minimax-m2.7` as the MiniMax default text model.
  - Add canonical exact models for the six other documented identifiers.
  - Keep each canonical identifier lowercase and provider independent.
  - Store each exact MiniMax identifier in its provider offering.
  - Use `openai_chat_completions` and `synchronous_completion` for each route.
  - Map the public `max_tokens` value to `max_completion_tokens`.
  - Apply the documented 204,800-token limit to each compatible route.
  - Record one current price for each new text offering.
  - Record the official price source and verification date.
  - Preserve existing tenant defaults and saved `minimax-m2.7` selections.
  - Expose each new model through management profiles and routing selectors.
  - Expose each new model through public capabilities and the route explorer.
  - Derive live-test discovery from the provider catalog.
  - Update constants, configuration, examples, documentation, and fixtures.
  Deliverables:
  - Add six exact MiniMax models and six direct provider offerings.
  - Add complete capability, limit, and price records for each offering.
  - Add management, public catalog, browser, and live-test coverage.
  Validation:
  - Prove each exact model selects the documented upstream identifier.
  - Prove each route sends `max_completion_tokens` when the caller supplies a limit.
  - Prove each route rejects a value above 204,800 before provider dispatch.
  - Prove existing `minimax-m2.7` defaults remain unchanged.
  - Prove each model appears once in every generated model surface.
  - Run one paid key verification and text request with a selected new model.
  - Keep credentials, prompts, and response bodies out of test output.
  - Run `make ci` after the last application change.
  Blocked: A MiniMax credential and paid-call authorization are not available
  in this workspace. The operator must supply `MINIMAX_API_KEY` and authorize
  the 14 paid live calls for seven key checks and seven text requests.
- [!] [I227] (P1) Add Kimi reasoning and image route capabilities.
  Goal:
  Expose verified Kimi reasoning and image capabilities through the current
  Moonshot provider and canonical message contract.
  Evidence:
  - The current Kimi offerings declare text generation only.
  - Kimi documents image input for Kimi K3, K2.7 Code, and K2.6:
    https://platform.kimi.ai/docs/overview
  - Kimi K3 accepts exact `reasoning_effort` values `low`, `high`, and `max`.
  - Kimi K2.6 uses a separate binary `thinking` object:
    https://platform.kimi.ai/docs/api/models-overview
  Requirements:
  - Add a Moonshot K3 reasoning adapter for the public `reasoning_effort` field.
  - Accept only `low`, `high`, and `max` on the K3 route.
  - Send each explicit value unchanged in the top-level provider field.
  - Omit the provider field when the public value is absent.
  - Keep K2.6 binary thinking under its documented provider default.
  - Add image input to K3, K2.7 Code, K2.7 Code Highspeed, and K2.6.
  - Serialize ordered canonical images as documented Chat Completions content blocks.
  - Preserve image bytes, MIME type, and order.
  - Record each verified image limit with its source and verification date.
  - Use `unknown` for an official limit that the provider does not publish.
  - Return only visible answer text through the canonical response.
  - Keep provider reasoning content out of responses, logs, and usage records.
  - Publish route-specific reasoning and image capabilities through public data.
  - Render the capabilities in management and public browser surfaces.
  - Extend the paid provider image matrix with Moonshot.
  - Keep video input in a separate typed attachment issue.
  - Update OpenAPI, official clients, configuration, documentation, and fixtures.
  Deliverables:
  - Add one exact K3 reasoning adapter and four Kimi image routes.
  - Add provider serialization, route validation, limits, and safe output handling.
  - Add management, public catalog, client, browser, and paid-harness coverage.
  Validation:
  - Prove each K3 reasoning value reaches the exact provider field.
  - Prove an omitted value leaves the provider field absent.
  - Prove unsupported values fail before provider dispatch.
  - Prove each image route preserves exact ordered image data.
  - Prove unsupported MIME types and provider limits fail before dispatch.
  - Prove public capability data matches the implemented routes.
  - Run one paid image request through each enabled Moonshot image model.
  - Keep credentials, image data, and response bodies out of test output.
  - Run `make ci` after the last application change.
  Blocked: A Moonshot credential and paid-call authorization are not available
  in this workspace. The operator must supply `MOONSHOT_API_KEY` and authorize
  the eight paid live calls for four key checks and four image requests.
- [!] [I226] (P1) {I038} Add current Qwen text models to DashScope.
  Goal:
  Let managed tenants select current Qwen flagship and cost-efficient models
  through their existing Alibaba Model Studio workspace connection.
  Evidence:
  - The current DashScope catalog contains only exact model `qwen-plus`.
  - Alibaba lists `qwen3.7-max`, `qwen3.7-plus`, and `qwen3.6-flash` as current
    recommended text models:
    https://www.alibabacloud.com/help/en/model-studio/text-generation-model
  - Alibaba lists the three models for the Singapore deployment scope:
    https://www.alibabacloud.com/help/en/model-studio/models
  - Alibaba documents workspace-specific Singapore endpoints for production:
    https://www.alibabacloud.com/help/en/model-studio/base-url
  Requirements:
  - Add exact models `qwen3.7-max`, `qwen3.7-plus`, and `qwen3.6-flash`.
  - Keep `qwen-plus` as the DashScope default text model.
  - Preserve existing saved `qwen-plus` settings and tenant defaults.
  - Use each tenant's saved Singapore workspace URL and matching API key.
  - Verify each exact model in the Singapore deployment scope before registration.
  - Select each route protocol from Alibaba's current documented contract.
  - Use I038's synchronous Responses adapter when a model requires Responses.
  - Keep each route on `synchronous_completion` unless Alibaba documents another lifecycle.
  - Record each verified context limit, output limit, and request control.
  - Record one current price for each new text offering.
  - Record each official source and verification date.
  - Keep this issue on text input and text output.
  - Expose each model through key verification and provider settings.
  - Expose each model through public capabilities and the route explorer.
  - Derive live-test discovery from the provider catalog.
  - Update constants, configuration, documentation, examples, and fixtures.
  Deliverables:
  - Add three exact Qwen models and three direct DashScope offerings.
  - Add protocol, lifecycle, control, limit, and price records.
  - Add management, public catalog, browser, and live-test coverage.
  Validation:
  - Prove each model uses the tenant's exact workspace URL.
  - Prove each route sends the documented request and parses the documented response.
  - Prove each configured limit fails at the public boundary.
  - Prove existing `qwen-plus` selections remain valid and unchanged.
  - Prove each new model appears once in every generated model surface.
  - Run one paid verification and text request for each new model.
  - Keep credentials, prompts, and response bodies out of test output.
  - Run `make ci` after the last application change.
  Blocked: A DashScope credential and tenant Singapore workspace URL are not
  available in this workspace. The operator must supply `DASHSCOPE_API_KEY`
  and `DASHSCOPE_BASE_URL` and authorize the six paid live calls.
- [!] [I225] (P1) Move GLM routes to the international Z.AI API.
  Goal:
  Use one canonical `zai` provider for international Z.AI text and dictation
  routes. Accept API keys from the international Z.AI platform.
  Evidence:
  - The current `zhipu` text route uses
    `https://open.bigmodel.cn/api/paas/v4`.
  - The current dictation route uses
    `https://api.z.ai/api/paas/v4/audio/transcriptions`.
  - Z.AI documents `https://api.z.ai/api/paas/v4` as its general API endpoint:
    https://docs.z.ai/api-reference/introduction
  - Z.AI documents GLM Chat Completions with bearer API-key authentication.
  Requirements:
  - Replace provider identifier `zhipu` and alias `glm` with canonical `zai`.
  - Use label `Z.AI` and declare no provider aliases.
  - Use `https://api.z.ai/api/paas/v4` for text requests.
  - Use `https://api.z.ai/api/paas/v4/audio/transcriptions` for dictation.
  - Keep GLM-5.1, GLM-5.2, and GLM-ASR-2512 as exact models.
  - Keep Chat Completions and multipart transcription as the protocol adapters.
  - Keep each current route on `synchronous_completion`.
  - Rename runtime configuration to `providers.zai`.
  - Rename the live credential binding to `ZAI_API_KEY`.
  - Remove current `zhipu` and `glm` request, profile, and configuration values.
  - Preflight each stored `zhipu` provider key before database mutation.
  - Decrypt each valid key with its existing tenant and provider identity.
  - Re-encrypt each key with the same tenant and canonical `zai` identity.
  - Update provider settings and routing defaults in the same transaction.
  - Reject conflicting, corrupt, or noncanonical migration input.
  - Preserve tenant timestamps and historical usage records.
  - Reject retired provider values after the migration.
  - Use the general API endpoint for application traffic.
  - Keep the Coding Plan endpoint in tool-specific integrations.
  - Update the catalog, constants, management API, UI, OpenAPI, and clients.
  - Update environment examples, provider documentation, and live-test discovery.
  Deliverables:
  - Add one canonical international `zai` provider definition.
  - Add one bounded provider-identity and encrypted-key migration.
  - Remove all current `zhipu` and `glm` integration surfaces.
  - Add complete static, managed, public, browser, and live-test coverage.
  Validation:
  - Prove text requests use the documented Z.AI general endpoint.
  - Prove dictation requests use the documented Z.AI transcription endpoint.
  - Prove managed key verification succeeds only through Z.AI.
  - Prove migration re-encrypts keys and updates each current routing field.
  - Prove migration preserves timestamps and historical usage values.
  - Prove current-schema startup rejects each retired provider shape.
  - Prove profiles, public data, examples, and clients expose only `zai`.
  - Run one paid Z.AI key verification and one small text request.
  - Keep the API key, prompt, and response body out of test output.
  - Run `make ci` after the last application change.
  Blocked: A Z.AI credential is not available in this workspace. The operator
  must supply `ZAI_API_KEY` and authorize the two paid live calls.
- [!] [I041] (P1) Migrate xAI text routes to Responses without OpenAI background assumptions.
  Goal:
  Move Grok models off xAI's deprecated Chat Completions surface while
  preserving xAI's actual synchronous Responses behavior.
  Evidence:
  - xAI calls Responses its preferred API and Chat Completions deprecated:
    https://docs.x.ai/developers/model-capabilities/text/comparison
  - xAI Responses supports typed output and optional stored conversation state,
    but its `background` field is currently compatibility-only and unused:
    https://docs.x.ai/developers/rest-api-reference/inference/chat
  - xAI separately exposes Deferred Chat Completions with `202` polling and a
    final result retrievable exactly once, but that operation belongs to the
    deprecated Chat family:
    https://docs.x.ai/developers/advanced-api-usage/deferred-chat-completions
  Requirements:
  - Verify Responses support for every configured Grok model and migrate each
    eligible model to an xAI-owned Responses codec. Do not reuse OpenAI's
    request builder or terminal-state parser merely because the endpoint path
    and typed output resemble OpenAI.
  - Omit `background` and register the lifecycle as synchronous. Parse xAI
    output, reasoning, usage, errors, storage controls, and output limits from
    xAI's schema.
  - Keep proxy requests stateless by default and document any approved use of
    xAI's 30-day stored response state. Do not retrieve a completed stored
    response as if it were an in-progress job.
  - Do not adopt Deferred Chat merely to manufacture polling. If xAI later
    offers deferred execution on its current Responses contract, audit that
    lifecycle in a separate issue.
  Validation:
  - Public fixtures prove xAI Responses request/response mapping, synchronous
    continuation, storage policy, usage, safe errors, and absence of
    `background` polling. Existing xAI speech routing remains independent.
  Progress:
  - All ten configured text offerings now use the xAI-owned `xai_responses` codec.
  - Requests use `store:false`, ordered messages, and the native output limit field.
    They omit background execution and stored response identifiers.
  - The codec preserves visible output, image inputs, and usage across synchronous continuation requests.
    B191 rejects invalid success responses and provider-private content.
  - Key verification, Go client validation, public discovery, and browser fixtures use the current protocol.
  - `docs/xai-responses.md` records provider sources and the exact live acceptance command.
  - On 2026-09-05, `make ci` passed all 12 gates in 203 seconds.
    Go statement coverage was 100%. All 95 frontend browser tests passed.
  Blocked: `XAI_API_KEY` is absent from the process environment and all six repository private environment files.
  Live key verification and text requests remain required for all ten routes before production activation.
- [ ] [I218] (P1) Expand the product node into integration routes.
  Goal:
  Make the product-to-proxy side of the public routing tree as actionable as
  its model-to-provider-offering side. Expand `Your product` into exact
  supported integration routes and route-specific instructions.
  Requirements:
  - Make the complete `Your product` box toggle the integration fan through
    pointer activation. The plus/minus is a visual element inside that box, not
    a separate control.
  - Expand HTTP, Go, Python, and CLI nodes to the left of `Your product`, draw
    measured Bezier connectors into the product node, and expose one selected
    integration at a time.
  - Show exactly one instruction panel for the selected integration and keep
    integration labels, links, commands, and instruction copy in one
    frontend-owned definition consumed by the graph and existing integration
    surface.
  - Extend the single routing graph positioned by F031 without duplicating the
    graph or restoring a second landing-page copy.
  - Preserve model publisher, exact model, and provider offering selection,
    semantic no-JavaScript access, reduced-motion behavior, and responsive
    containment without horizontal page overflow.
  Deliverables:
  - Add the product disclosure, four integration nodes, selected-route state,
    instruction panel, connector drawing, and current module revision.
  - Document the integration-fan interaction and accessibility contract.
  Validation:
  - Prove exactly four generated integration routes, whole-box pointer
    disclosure including the visual plus/minus, one selected instruction panel,
    and selected connector endpoints through the public entry point.
  - Prove model and provider offering interactions remain unchanged. Inspect
    connector geometry and containment at 1280-, 900-, and 390-pixel widths.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
- [!] [I210] (P1) Add Meta Muse Spark 1.2 as a selectable Standard-tier model.
  Goal:
  Add Meta's current Muse Spark 1.2 checkpoint to the existing `meta` text
  offering through the repository's exact model-owned routing contract.
  Evidence:
  - Meta announced Muse Spark 1.2 on 2026-08-05 and states that it is available
    in Meta Model API with expanded global access:
    https://research.meta.ai/blog/introducing-muse-code-and-muse-spark-1-2
  - Meta's current model catalog publishes exact Standard-tier model id
    `muse-spark-1.2` alongside `muse-spark-1.1`, with text, image, video, audio,
    and PDF input, text output, and a 1,048,576-token context window:
    https://dev.meta.ai/docs/models
  - Meta's Chat Completions guide uses `muse-spark-1.2` directly through the
    OpenAI-compatible `https://api.meta.ai/v1` API. It documents
    `max_completion_tokens` as the current output-budget field and synchronous
    non-streaming completion responses:
    https://dev.meta.ai/docs/protocols/chat-completions
  - Standard-tier Muse Spark 1.1 and 1.2 share the same pricing, data-use, and
    team rate-limit contract. The separate `muse-spark-1.2-contributor` tier
    permits Meta to use prompts and completions for training and has distinct
    pricing and rate limits:
    https://developer.meta.com/ai/models/muse-spark/
    https://dev.meta.ai/docs/pricing-rate-limits
  Requirements:
  - Register exact model id `muse-spark-1.2` under provider `meta` with
    `openai_chat_completions` and `synchronous_completion`, reusing
    `https://api.meta.ai/v1`, the existing Meta credential, and the shared
    Chat Completions adapter.
  - Keep `muse-spark-1.1` as the configured Meta default and current selectable
    Standard-tier model. This issue adds 1.2 selection without rewriting saved
    tenant routing or provider settings.
  - Scope the addition to Standard-tier `muse-spark-1.2`. Treat Contributor as
    a separate explicit opt-in data-use, billing, and rate-limit contract.
  - Preserve the current Meta text surface and upstream-default reasoning:
    send the selected model, ordered text messages, and optional
    `max_completion_tokens`; keep Meta media, tools, search grounding,
    streaming, configurable reasoning effort, and Responses API work in their
    own route-capability issues.
  - Expose 1.2 through the management profile, provider-model selectors,
    provider-key verification, public capability catalog, and explicit public
    request routing. Update the canonical constant, checked-in configuration,
    README model tables/examples, provider-routing documentation, generated
    public artifacts, and affected black-box fixtures together.
  Validation:
  - Startup and public-boundary scenarios prove the 1.2 catalog entry, exact
    upstream model id, `max_completion_tokens` mapping, synchronous completion
    and output-length continuation through `GET /`, compatibility `POST /`,
    and canonical `POST /v2`.
  - Management and browser scenarios prove 1.2 appears under Meta, can be
    verified and saved with an existing Meta key, can become a tenant routing
    default, and leaves existing 1.1 selections valid.
  - Public-site rendering proves the generated capability matrix publishes
    both exact Standard-tier Meta model ids without Contributor or unsupported
    proxy capabilities.
  - Authenticated branch acceptance confirms `GET /v1/models` contains
    `muse-spark-1.2`, then runs one small paid Meta verification and text
    request with `LLM_PROXY_LIVE_META_MODEL=muse-spark-1.2`. Run the required
    baseline and final `timeout -k 350s -s SIGKILL 350s make ci` pair; deployment
    and production acceptance remain operator-owned.
  Blocked: Repository work is complete. Both required `make ci` runs pass.
  The existing key is available as `MUSE_API_KEY` in `configs/.env`.
  Complete exact Muse Spark 1.2 discovery, key verification, and live text acceptance with that file.
- [x] [I046] (P1) Make upstream admission fair across provider origins.
  Goal:
  Keep upstream work globally bounded while preventing one slow or throttled
  origin from consuming the active and queued capacity needed by unrelated
  origins.
  Evidence:
  - `limitedHTTPDoer` owns one global active channel sized by `server.workers`
    and one global admission channel sized by
    `server.workers + server.queue_size`. With the checked-in `4` and `32`
    values, the fifth simultaneous upstream operation waits even when it targets
    an unrelated origin, and the thirty-seventh is rejected.
  - A call waiting for its origin's rolling rate-limit window correctly
    releases its active worker, but retains its global admission token. A
    throttled origin can therefore fill all 36 admissions and make another
    origin receive `request queue full` while active workers are idle.
  - I042 and I043 removed managed-database authentication and usage-write
    serialization. They do not alter this shared upstream active/admission
    contract.
  Requirements:
  - Replace the global-only worker and admission channels with one canonical
    origin-aware capacity contract. Key ownership by the exact normalized
    upstream origin used by the existing rate limiter, so provider transports
    that intentionally share an origin also share its capacity.
  - Keep explicit positive global ceilings for active and admitted work and
    explicit positive per-origin active and queued limits. Validate the complete
    contract at startup, reject duplicate, missing, unknown, or contradictory
    origin rules, and remove the obsolete global-only configuration in the same
    forward-only change rather than retaining aliases or dual scheduling paths.
  - Schedule ready work fairly across origins. Continuous traffic from one
    origin must not starve a queued operation from another origin when capacity
    becomes available, and one origin may not consume another origin's bounded
    queue allocation.
  - A call delayed by an origin rate limit may retain only that origin's bounded
    admission. It must not occupy active global capacity or unrelated-origin
    admission while sleeping. Cancellation or deadline expiry must remove the
    waiter and release every owned capacity token exactly once.
  - Preserve the current response-body ownership rule: an active operation
    retains its worker until the upstream body is closed. Keep all queues and
    schedulers bounded, use no per-waiter background goroutine, and preserve the
    public overload and request-timeout mappings.
  - Correlate admission decisions, waits, and rejections through I045's safe
    request telemetry. Update runtime configuration, README, provider-routing
    guidance, tracked deployment inputs, and configuration examples as one
    current contract.
  Deliverables:
  - One bounded origin-aware scheduler with explicit global and per-origin
    capacity ownership, fair ready-origin selection, cancellation-safe token
    release, and no legacy global-only path.
  - Strict configuration parsing and documentation for every configured
    upstream origin.
  - Deterministic public-boundary concurrency coverage and a Makefile-owned
    race-detector gate for the concurrency path.
  Validation:
  - Use controlled upstream servers on at least two origins. Saturate one
    origin with active, queued, and rate-limited calls and prove an admissible
    request to the other origin begins while its own and global capacity are
    available.
  - Under continuous contention, prove both origins make bounded progress,
    per-origin and global maxima are never exceeded, queue-full rejection is
    isolated to the exhausted capacity, and response-body close releases the
    exact active slot.
  - Cover cancellation and request-budget expiry before admission, during
    ordinary worker wait, during rate-limit wait, and after upstream response
    acquisition without leaked slots, duplicate release, blocked shutdown, or
    goroutine growth.
  - Add a repository Makefile target that runs the public concurrency coverage
    with Go's race detector and include it in `make ci`.
  - Run the final `make ci` target under the current repository validation policy.
  Media expansion:
  - Prove bounded interactive text progress while image or video traffic saturates the same upstream origin.
  - Define separate capacity ownership for accepted jobs, active HTTP requests, status reads, and artifact transfers.
  - Release active HTTP capacity between remote-job polls.
  - Include tenant fairness and shared provider-account limits in the chosen capacity contract.
  - Make this acceptance part of F024 before general media availability.
  Progress:
  - The public HTTP regression reproduces HTTP 503 for an independent origin during saturation.
  - The new scheduler core passes local HTTP and race tests for capacity ownership, cancellation, and fair selection.
  - The router now uses explicit origin capacity. The obsolete worker settings and limiter are removed.
  - Public HTTP tests prove text progress during media submission, status, and transfer saturation on the same origin and account.
  - The expanded race tests pass. Public HTTP tests prove tenant turns and shared-account limits.
  - The obsolete fixtures and CI receipt expectations are corrected. Their focused tests and Go static analysis pass.
  - Saved connections now require declared origins at startup. Public tests verify this rejection and request-correlated admission events.
  - Management verification shares the saved connection's account capacity and request correlation. Its public HTTP regression passes.
  - All tests pass in the final full Go run with 100.0 percent statement coverage.
  - Additional contract tests pass for invalid speech defaults, cancellation before dispatch, and invalid startup inputs.
  - B228 repairs the obsolete family icon manifest and current browser contracts.
  - The CI runner executes the admission race tests and full Go tests concurrently. Both results are required.
  - The local provider preflight declares its temporary origin in the capacity configuration.
  Resolution:
  - Explicit capacity now bounds each origin, tenant, account, and work class. The obsolete worker settings are removed.
  - The `upstream HTTP admission` event records safe request or operation correlation and bounded capacity counts.
  - Final `make ci` passes all 14 gates in 340 seconds, including 100.0 percent Go coverage and race checks.
  - All 127 browser tests and seven TAuth browser tests pass. The full result is in `/tmp/i046-ci-fifth.log`.
- [!] [I038] (P2) Adopt DashScope's synchronous Responses API without background mode.
  Goal:
  Move eligible DashScope Qwen models from Chat Completions to Alibaba's newer
  Responses wire format while retaining its explicitly synchronous lifecycle.
  Evidence:
  - Alibaba documents an OpenAI-compatible Responses endpoint with typed output,
    tools, `previous_response_id`, and storage controls:
    https://www.alibabacloud.com/help/en/model-studio/qwen-api-via-openai-responses
  - The same reference states that `background` is unsupported and that only
    synchronous calls are processed. Unlisted OpenAI fields may be ignored.
  Requirements:
  - Verify the Responses support matrix for every configured DashScope model.
    Migrate supported models to a dedicated DashScope Responses wire adapter.
    Leave any unsupported model on one explicitly registered current contract
    rather than trying Responses and falling back at runtime.
  - Send only Alibaba-documented fields and omit `background`. Parse typed
    output items, incomplete status, Qwen reasoning usage, and provider errors
    from the Alibaba schema rather than the OpenAI schema by assumption.
  - Keep public proxy calls stateless unless a separately approved retention
    contract requires stored provider state. Do not adopt
    `previous_response_id`, conversations, built-in tools, or default storage
    merely because the fields exist.
  - Use output-limit continuation only after a terminal incomplete result.
    Never issue `GET /responses/{id}` as a progress poll.
  Validation:
  - Public black-box tests prove the eligible-model request shape, typed text
    extraction, synchronous incomplete continuation, usage, safe errors, and
    rejection of accidental `background` or unsupported OpenAI-only fields.
  Development:
  - All four Qwen text routes use the dedicated `dashscope_responses` codec.
    Requests use the saved Singapore workspace URL and explicit `store: false`.
  - Public output limits below 16 are rejected before provider work. Typed
    visible text and native usage feed the existing stateless continuation.
  - Workspace verification, image serialization, discovery, and catalog search
    use the new protocol. Model identities and the default remain unchanged.
  - Focused public HTTP tests pass. Final CI passed all 12 gates in 181 seconds,
    with 100% Go statement coverage and 95 browser tests. Evidence:
    `/tmp/llm-proxy-i038-ci.log`.
  - Current contract and live command: `docs/dashscope-responses.md`.
  Blocked:
  - On 2026-09-05, `DASHSCOPE_API_KEY` and `DASHSCOPE_BASE_URL` were absent from
    the process and all six authorized repository input files. All four models
    require live workspace acceptance. I226 retains that acceptance gate.
- [ ] [I035] (P2) Persist each user's selected Usage interval across sessions.
  Goal:
  Make the Usage Overview reopen with the last interval the authenticated user
  successfully selected. A user who selects `7 days` must start the next login
  at `7 days`; after changing to `1 day`, subsequent logins must start at
  `1 day`.
  Evidence:
  - The frontend initializes `selectedUsageInterval` from the hard-coded
    `30d` default. Selecting another interval changes only the mounted Alpine
    component and requests that interval's usage summary.
  - Authentication reset explicitly restores `30d`, and a full page reload
    constructs the same default before the authenticated workspace loads.
  - The managed user record and `GET /api/management/account` response contain
    no dashboard preference, so the current selection cannot survive logout,
    reload, session restoration, or another browser/device.
  Requirements:
  - Persist exactly one canonical account-owned `usage_interval` preference for
    each authenticated managed user. Accepted values are `all`, `30d`, `7d`,
    and `1d`; a newly created user defaults to `30d`.
  - Keep this preference independent of the `Usage tenant` filter and Settings
    tenant. The same saved interval initializes both account-wide and explicitly
    tenant-filtered Usage Overview queries. Do not persist the Usage tenant,
    dashboard view, admin view, failure-dialog state, or any other local UI
    state as part of this issue.
  - Extend the canonical `GET /api/management/account` response with a required
    `preferences` object containing the exact saved `usage_interval`. Add one
    owner-only `PUT /api/management/account/preferences` operation whose strict
    request and response contain that same complete preference object. Reject
    a missing, blank, unknown, or additional field with `400`; never normalize,
    infer, or silently replace an invalid value.
  - Store the preference on the managed user through the existing GORM database
    boundary. Add one bounded, all-or-nothing schema migration that initializes
    every existing user to `30d`, verifies the migrated rows, and records the
    new current schema version. After migration, keep only the current schema
    and reject invalid persisted values at startup without a read-time fallback,
    nullable legacy shape, dual read/write, or compatibility response.
  - Apply the account response's saved interval before issuing the initial
    Usage Overview request so login, session restoration, and full reload make
    exactly the saved-interval request without first rendering or requesting
    `30d`.
  - On interval selection, persist the exact new value before treating it as
    the confirmed selection and loading its usage summary. Keep interval
    controls blocked through the preference mutation and selected-interval load.
    A failed preference mutation must retain the prior confirmed interval and
    snapshot, show the existing explicit request-failure treatment, and never
    imply that an unsaved choice will survive the next login.
  - Preserve request identity and authentication isolation. A late preference
    or usage response cannot overwrite a newer interval, authentication reset,
    another user, Usage tenant change, or dashboard-view change.
  - Keep the preference server-side. Do not add localStorage, sessionStorage,
    cookies, URL/history state, a tenant field, a client-library preference
    file, or a browser-only fallback. The fixed administrator dashboard remains
    a separate 30-day contract and does not read or mutate this preference.
  - Update the canonical OpenAPI source, generated API reference, frontend
    types, README, CHANGELOG.md, and
    `docs/implementation/provider-routing-plan.md` in the same implementation.
  Deliverables:
  - One typed Usage-interval preference contract, forward-only managed-user
    schema migration, owner-isolated read/update store path, canonical account
    response and preference update operation, and race-safe frontend hydration
    and mutation flow.
  - Updated canonical and generated documentation describing the account-owned
    persistence boundary and the unchanged local-only state outside this
    preference.
  Validation:
  - Exercise the real management router and a disposable SQLite database to
    prove a new user starts at `30d`, can save each supported interval, retains
    the latest value after database restart and a new authenticated session,
    and cannot read or change another user's preference.
  - Prove the bounded migration initializes existing users once, preserves all
    account/tenant/provider/usage data, and rejects an invalid current-schema
    preference at startup without mutation or fallback.
  - Add OpenAPI conformance coverage for the required account preference and
    strict authenticated update operation, including invalid bodies,
    authorization, owner isolation, and stable error responses.
  - Add Playwright coverage showing `7 days` selected after a full reload and
    later login, then `1 day` after the next successful change and login.
    Prove the first usage request uses only the saved interval, failed saves
    retain the prior confirmed view, rapid/stale responses cannot regress it,
    and no preference is written to browser storage.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair for the implementation, with
    the final run after the last code edit.

## Maintenance

- [ ] [M004R] (P1) Dependency and security audit.
  Goal:
  Keep third-party dependencies, runtime versions, and security-sensitive configuration within the current supported contract.
  Requirements:
  - Cadence: run weekly for active apps and before each release cut.
  - Inspect package managers, lockfiles, language toolchains, container bases, and generated clients for known vulnerabilities or stale direct dependencies.
  - Review auth, secret, CORS, CSP, SQL, network, and permission-sensitive configuration for drift from the current contract.
  - Prefer current supported dependencies; do not add compatibility shims for obsolete dependency behavior.
  - File separate Maintenance or BugFix issues for each actionable vulnerability, unsupported runtime, or security-contract gap.
  Deliverables:
  - Documented audit commands or data sources used for the pass.
  - Updated issues for each actionable dependency or security finding.
  - A short `Last run:` note with clean result or follow-up issue IDs.
  Validation:
  - Rerun the repository-native audit, lint, or dependency checks used for the pass.
  - Confirm every finding is either filed, fixed under a separate issue, or explicitly marked not applicable with evidence.
  - Confirm no secrets or private payloads were written into the tracker.
  Last run: 2026-07-20. Ran `go mod verify`, `go run golang.org/x/vuln/cmd/govulncheck@latest -show verbose ./...`, `npm audit --json`, and a locked Python `pip-audit` export; npm and Python audits were clean, while Go findings are filed in M014 through M018. Reviewed tracked configuration, ignored runtime-input boundaries, container-base refresh behavior, management auth/CORS/encryption/GORM paths, and request logging; the logging privacy gap is filed in B039. M019 records non-security direct dependency freshness. No secrets or private payloads were added to this tracker.
- [ ] [M005R] (P1) CI, release, and artifact health.
  Goal:
  Keep the repository's validation, release, publication, and generated artifact surfaces trustworthy.
  Requirements:
  - Cadence: run before every release, publish, or deploy, and weekly for critical services.
  - Verify repository-native CI, lint, format, coverage, release, publish, Docker image, Pages, and artifact workflows still match the documented contract.
  - Check generated artifacts, release tags, published images, and Pages outputs for source-to-public drift.
  - File concrete follow-up issues for failing gates, stale artifacts, missing release prerequisites, or undocumented workflow changes.
  - Do not perform production deployment from this runbook unless the operator explicitly requests that deployment.
  Deliverables:
  - Recorded gate status and artifact surfaces inspected.
  - Follow-up issues for each reproducible CI, release, publish, or artifact drift problem.
  - A short `Last run:` note with commands run and any skipped surfaces.
  Validation:
  - Use repository-native `make` targets or documented release helpers for checks.
  - Confirm release and deployment ownership boundaries remain separate.
  - Confirm public or published artifacts match the intended source revision when that surface is inspected.
  Last run: 2026-09-01 after release `v1.3.0`. The release and publication
  receipts select application commit `7a61fc55f36786086cb887a6f9b91f4450119e8d`.
  GitHub, Go Proxy, GHCR, Pages, and the installed Python client use `v1.3.0`.
  The Pages build and public marker select the exact published commit.
- [ ] [M006R] (P1) Code contract and static hygiene.
  Goal:
  Keep source contracts explicit, current, and statically guarded against policy drift.
  Requirements:
  - Cadence: run monthly and before large refactors.
  - Scan for dead code, unused exports, duplicated literals, silent fallbacks, legacy aliases, compatibility reads, and zero-but-invalid domain states.
  - Check static analysis, coverage, schema, and contract guards that are supposed to prevent drift.
  - File focused Maintenance issues for each concrete violation instead of broad cleanup placeholders.
  - Keep the current canonical contract only; do not preserve obsolete behavior unless a product requirement explicitly says so.
  Deliverables:
  - Issue entries for each actionable static hygiene or contract violation.
  - Notes on static tools, searches, and contract guards used during the pass.
  - A short `Last run:` note with clean result or follow-up issue IDs.
  Validation:
  - Rerun the relevant static checks, contract tests, or repository searches used to identify drift.
  - Confirm every finding has a narrow follow-up issue and does not duplicate existing backlog work.
  - Confirm no implementation changes were mixed into the audit unless separately requested.
- [ ] [M007R] (P1) Production drift and health.
  Goal:
  Detect when production, public, or scheduled runtime state has drifted from the intended repository contract.
  Requirements:
  - Cadence: run weekly for deployed services and after each publish or deploy.
  - Compare current source, runtime configuration, published images, public routes, scheduled jobs, and health checks for drift.
  - Inspect real operator-facing surfaces rather than assuming merged source is deployed.
  - File follow-up issues for stale images, stale Pages output, missing routes, failed monitors, invalid production config, or undocumented runtime differences.
  - Stop before production deploy or destructive operator actions unless the operator explicitly requests them.
  Deliverables:
  - Recorded source revision, public artifact, route, image, or health surfaces inspected.
  - Follow-up issues for each source-to-runtime drift finding.
  - A short `Last run:` note with evidence links or commands used.
  Validation:
  - Verify inspected production or public surfaces directly where access is available.
  - Confirm any deploy-required finding is filed with the exact publish/deploy boundary and owner.
  - Confirm no production state was changed by the audit unless explicitly requested.
  Last run: 2026-09-01 after deployment `v1.3.0`. State revision 180 selects
  LLM Proxy generation 14 and application commit
  `7a61fc55f36786086cb887a6f9b91f4450119e8d`. All eight resources, Caddy, and
  TAuth have current verified observations. The runtime uses the published
  image digest. The API returned the required `403` and `200` responses. The
  Pages marker reports `v1.3.0` and the exact application commit. The canonical
  Default tenant key passed identity verification and all nine production
  provider cases. B128 and B160 are resolved.
- [ ] [M001R] (P2) Backlog hygiene and archive.
  Goal:
  Keep the issue tracker reliable, readable, and focused on active work while preserving resolved history in the appropriate archive.
  Requirements:
  - Cadence: run weekly during active development and before each release cut.
  - Validate section names, identifier prefixes, recurrence suffixes, priority markers, dependencies, and duplicate IDs against the current `issues-md-format.md`.
  - Reconcile stale statuses, duplicate issues, broken references, obsolete instructions, and entries filed under the wrong section.
  - Move completed non-recurring history to the repository issue archive or durable documentation when the active tracker becomes noisy.
  - Keep active, blocked, planning, and recurring entries visible in `ISSUES.md`.
  Deliverables:
  - Normalized `ISSUES.md` structure and statuses.
  - Updated issue archive or docs when completed entries are removed from the active tracker.
  - A short `Last run:` note summarizing the cleanup and any follow-up issues filed.
  Validation:
  - Re-read `ISSUES.md` after edits and confirm every issue is under the right section with a unique section-aware ID.
  - Confirm recurring entries remain open and keep the `R` suffix.
  - Confirm no active, blocked, recurring, or planning work was archived.
  Last run: 2026-09-18. Audited the active tracker and archive after the
  durable-documentation review. Moved 41 resolved non-recurring entries to the
  archive: 22 BugFixes, 12 Improvements, two Features, and five Planning
  entries. Renamed the blocked schema issue to I271 and archived the identifier
  repair as I263. Kept active, blocked, planning, and recurring entries in the
  active tracker. No duplicate issue identifiers remain.
- [ ] [M002R] (P2) Polish open issues.
  Goal:
  Keep unresolved work executable by making each open issue concrete, ordered, and testable.
  Requirements:
  - Cadence: run weekly during active development and before handing a repo to automated execution.
  - Review every unresolved non-recurring issue for missing context, dependencies, repro steps, acceptance criteria, and validation expectations.
  - Make priorities concrete. Make sure each open issue has actionable deliverables.
  - Merge duplicate open issues or add explicit dependency links when separate entries must remain.
  - Do not close or implement issues as part of this polish pass unless that work is separately requested.
  Deliverables:
  - Open issues with enough detail for a person or agent to execute without rediscovery.
  - New or updated dependency markers where ordering matters.
  - A short `Last run:` note listing the number of issues polished and any blockers found.
  Validation:
  - Sample the open entries after the pass and confirm each has clear next actions and validation expectations.
  - Confirm no recurring runbook was marked complete.
  - Confirm duplicates were merged or explicitly cross-referenced.
  Last run: 2026-08-10. Audited all 46 unresolved entries from the archive
  pass. Archived B099 because its merged contract is enforced. Archived B077
  because its historical release contract is verified. Archived F018 because
  I027 supersedes it. Archived I205 because it belongs to the ISSUES.md editor
  repository. Added B126 for current release activation and P007 for the
  Alibaba provider decision.
  Added 11 dependency tokens, removed stale prerequisite prose, and demoted six
  downstream or planning items to P2. The tracker now has 44 current entries:
  42 open and two blocked. B126 and F021 each name one exact external action or
  issue in their `Blocked:` line.
- [ ] [M003R] (P2) Architecture and policy review.
  Goal:
  Catch architecture, policy, and workflow drift before it becomes hidden maintenance debt.
  Requirements:
  - Cadence: run monthly, before large refactors, and after major framework or runtime changes.
  - Review the codebase, docs, and workflow against `AGENTS.md`, `POLICY.md`, stack guides, and the current architecture notes.
  - Look for drift from forward-only contracts, edge-validation boundaries, smart-constructor usage, testing policy, and module ownership.
  - Record findings as new Maintenance issues with concrete scope, priority, and validation.
  - Close the pass with a no-action note only when the review finds no actionable drift.
  Deliverables:
  - New Maintenance issues for each actionable architecture or policy drift finding.
  - Updated notes on areas reviewed and areas intentionally left unchanged.
  - A short `Last run:` note with the review scope and outcome.
  Validation:
  - Confirm every finding is represented as an issue with owner-readable context and validation criteria.
  - Confirm no implementation changes were mixed into the review runbook unless separately requested.
  - Confirm all recurring runbooks remain open.
- [ ] [M008R] (P2) Documentation and runbook hygiene.
  Goal:
  Keep durable documentation and runbooks aligned with the current behavior users and operators actually rely on.
  Requirements:
  - Cadence: run before release cuts and after merge bursts that change user-facing or operator-facing behavior.
  - Review README, ARCHITECTURE, PRD, CHANGELOG, docs, runbooks, setup guides, and local workflow notes for stale behavior or missing new contracts.
  - Update docs when closed issues changed durable behavior, public APIs, operator workflows, release semantics, or deployment expectations.
  - Remove or rewrite stale instructions instead of preserving obsolete alternatives.
  - File separate issues for documentation gaps that require product or implementation decisions.
  Deliverables:
  - Updated documentation or filed follow-up issues for each gap.
  - A short `Last run:` note listing docs inspected and changes made.
  - Cross-references from archived issue history to durable docs when useful.
  Validation:
  - Examine links, command names, paths, and public contract descriptions touched by the pass.
  - Confirm docs describe the current canonical path only.
  - Confirm issue archive and active tracker references remain consistent.
  Last run: 2026-09-18. Reviewed README, docs/openapi.yaml,
  docs/client-protocols.md, docs/mcp.md, docs/media-gateway-consolidation.md,
  docs/tenant-connections.md, docs/provider-catalog.md,
  docs/provider-model-icons.md, docs/managed-schema-transition.md,
  docs/dictator-live-acceptance.md, and docs/dictator-migration-audit.md.
  Updated schema-transition references from I261 to I271. The current
  documents match the 41 archived contracts. No further documentation update
  was required.

## Features

- [ ] [F087] (P1) Configure and qualify the LLM Proxy Paddle sandbox.
  Status:
  - On 2026-09-23, the operator moved sandbox setup and actual qualification to this separate follow-up issue.
  - This issue does not block F069 or F070 development completion.
  Evidence:
  - Read-only Paddle sandbox requests succeeded with the API key from `/Users/tyemirov/Development/PoodleScanner/configs/.env.ps`.
  - The file also contains sandbox client-token and webhook-secret inputs. Their values were not copied or disclosed.
  - The account has active PoodleScanner, Hecate, and Crossword products. It has no active LLM Proxy product or USD 5 funding price.
  - No listed notification destination targets LLM Proxy. The existing destinations belong to other application flows.
  - Hecate uses RevenueCat for browser Paddle commerce. F069 uses the existing direct Paddle approach from PoodleScanner.
  - The API checks did not establish the platform supplier or the canonical Paddle account identity.
  Goal:
  Configure separate LLM Proxy sandbox resources and retain evidence from actual processor scenarios.
  Requirements:
  - Confirm the platform supplier and Paddle account identity before configuration.
  - Reuse the existing authorized sandbox account and shared Paddle client.
  - Create a distinct LLM Proxy product and an active one-time USD 5 funding price.
  - Keep the USD 5 funding minimum and permit spending down to USD 0 without a subscription.
  - Configure the LLM Proxy checkout origin and a notification destination for `/api/payments/paddle/events`.
  - Use the secret for that notification destination. Do not reuse another application's webhook secret or product identifiers.
  - Select a sandbox client token from the same account and an isolated managed database.
  - Record the private configuration path and non-secret account, product, price, and notification identifiers.
  - Keep sandbox records separate from production. Keep production payments disabled.
  - Use the actual sandbox procedure in `docs/hosted-billing.md`.
  Validation:
  - Complete checkout, signed notification, delayed confirmation, replay, refund, reversal, and reconciliation scenarios.
  - Retain the source revision, scenario results, processor references, and expected financial totals without credentials.
  - Run `make qualify-paddle-sandbox` and distinguish actual results from controlled protocol evidence.
  - Preserve the existing F069 and F070 controlled acceptance and CI requirements.

- [ ] [F086] (P1) Compare models on tenant tasks before a model change.
  Goal:
  Help tenants evaluate a candidate model with their own requests before they change the selected model.
  Requirements:
  - Place a `Compare models` action beside the model selection controls in the tenant dashboard.
  - Start with the current model and one candidate model.
  - Identify each exact model and provider offering separately.
  - Let the tenant supply sample requests and define the expected results.
  - Execute the same requests through both selected routes with explicit, supported settings.
  - Show the two answers beside each other for each request.
  - Show task results, response time, request errors, and available cost evidence.
  - Distinguish tenant judgments from automatic checks and provider execution failures.
  - Show cases where the candidate fails a check that the current model passes.
  - Record request settings, route identities, execution dates, sample counts, and individual results.
  - Label estimated cost separately from measured usage and confirmed charges.
  - Show unavailable cost evidence explicitly.
  - Use the canonical catalog and applicable price conditions for cost estimates.
  - Keep the public price display owned by F036 separate from this tenant workflow.
  - Save each result under the tenant in a dedicated `Comparisons` page.
  - Let the tenant reopen results and run a new evaluation after a model update.
  - Retain the original dated results when the tenant runs a new evaluation.
  - Require an explicit tenant action before the selected model changes.
  - Start with a results table and individual answers.
  - Show summary charts only for comparable measurements with visible sample counts, settings, units, and scoring rules.
  - Add a small public demonstration that links to the working tenant workflow after implementation.
  Deliverables:
  - Implement the dashboard action, execution workflow, result view, and saved results page.
  - Define task scoring, request limits, repeat counts, and result retention before implementation.
  - Protect tenant requests and results with the existing tenant access controls.
  - Document the workflow and the limits of each metric.
  Validation:
  - Do a test of both routes with identical sample inputs through the public application interfaces.
  - Do a test of supported settings, invalid inputs, provider errors, unavailable costs, and failed task checks.
  - Verify saved results survive reload and remain inaccessible to other tenants.
  - Verify the selected model changes only after the tenant chooses that action.
  - Verify keyboard use, readable results, and no horizontal page overflow at desktop and mobile widths.
  - Verify charts and tables agree with their individual results and sample counts.
  - Run the applicable integration tests and `make ci` after application changes.
  Context:
  The user requested P1 on September 22, 2026.
  A Reddit response requested task checks when providers change. This is one feedback signal, not measured demand.
  Source: https://www.reddit.com/user/MarcoPoloResearchLab/comments/1wmhonq/comment/pbchgot/
  Presentation reference: https://artificialanalysis.ai/


- [x] [F085] (P2) Show all connection models without a task selection.
  Requirements: Start the dashboard with no task filter selected. Show all models that the selected connection supports.
  Requirements: Show only filters for tasks in the complete connection inventory. Permit removal of the final selection.
  Requirements: Preserve available selections on connection changes. Show all models when no selection remains.
  Requirements: Keep AND matching for selected tasks. Derive default actions from the selected model capabilities.
  Validation: Check selection, search, connection changes, and default actions through the browser. Run final CI.
  Evidence: `/tmp/model-filters-f085-initial.log` records one initial task selection where the test expects zero.
  Evidence: `/tmp/model-filters-f085-focused.log` records five successful browser checks.
  Validation: Initial CI passed 151 browser tests but found an obsolete automatic-selection assertion in the dashboard integration test.
  Evidence: `/tmp/model-filters-ci.log` records expected `true` and actual `false` for the Transcribe button.
  Validation: The corrected test passes in `/tmp/model-filters-blackbox.log`.
  Resolution: The dashboard shows all connection models without a task selection. Filters use the complete connection inventory.
  Validation: Final CI passes all 14 gates, including 151 frontend tests, seven integration browser tests, and 100.0 percent Go coverage.
  Evidence: `/tmp/model-filters-ci-final.log`. Event contracts are unchanged.


- [x] [F084] (P2) {I279} Filter model tasks with independently selectable buttons.
  Goal: Select one or more tasks through compact buttons and show models that support every selected task.
  This change replaces the single-task dropdown introduced by I279.
  Requirements:
  - Replace the task dropdown with a visible group of compact toggle buttons in the dashboard and public explorer.
  - Reuse the task vocabulary and modality icons from I279.
  - Keep Text generation, Image generation, and Image understanding distinct in labels and accessible descriptions.
  - Show a task button only when the selected connection has at least one model that supports that task.
  - Determine button visibility from the connection's complete model inventory before applying task intersections or search filters.
  - Start with Text as the only pressed task button when Text is available.
  - If Text is unavailable, select the first available task in the canonical task order.
  - Keep at least one task button pressed whenever task buttons are available.
  - If the connection has no model tasks, show an empty state without task buttons.
  - Let each button toggle independently while preserving at least one selection.
  - If the user activates the final pressed button, keep that button pressed.
  - Update the model list immediately after each selection change, without an Apply action or page reload.
  - Use AND matching across selected tasks.
  - With Text and Images pressed, show only offerings that support both text generation and image generation.
  - Match all selected tasks on the same exact model and provider offering.
  - Combine task selections with the existing search, provider, input, output, and other applicable filters.
  - Keep input and output matching within one task on the same offering, as defined by I279.
  - Preserve selected task filters when their combination has no results.
  - When the connection changes, remove unavailable task selections and preserve the remaining selections.
  - If no selection remains, select Text when available, otherwise select the first available task.
  - Update the visible buttons, pressed states, and model list together after a connection change.
  - Show an explicit empty result and retain controls that let the user revise the selection.
  - Keep filter changes separate from tenant assignments and saved model defaults.
  - Show each selected task's input/output direction separately on matching cards or their details.
  - Preserve task-specific default controls when multiple tasks are selected.
  - Use `aria-pressed`, keyboard activation, visible focus, and accessible descriptions for each toggle button.
  - Make the button group wrap cleanly at desktop and mobile viewport widths.
  Deliverables:
  - Update shared filter state, dashboard controls, public explorer controls, and current discovery documentation.
  - Add browser coverage for individual toggles, combined selections, the final pressed button, and empty results.
  Validation:
  - Verify initial Text selection, Images alone, and Text plus Images.
  - Select Images with Text pressed and verify the model list immediately shows their intersection.
  - Release Text with Images pressed and verify the model list immediately shows image generation offerings.
  - Activate Images as the final pressed button and verify that its pressed state and results remain unchanged.
  - Verify the same selection rules with mouse, keyboard, and touch input.
  - Exclude a model with text generation on one provider and image generation only on another provider.
  - Exclude vision-only offerings from a Text plus Images selection.
  - Verify combined task and input/output filters without combining incompatible task directions.
  - Verify that an empty intersection retains the selected buttons until the user changes them.
  - Verify that unsupported task buttons are absent rather than disabled.
  - Switch between text-only, image-only, multimodal, and resource-only connections and verify the available buttons.
  - Verify that connection changes preserve available selections and remove unavailable selections.
  - Verify Text selection when available and first-task selection when Text is unavailable.
  - Verify that search results and empty task intersections do not remove otherwise supported buttons.
  - Verify that a connection without model tasks shows an empty state without task buttons.
  - Verify keyboard and touch operation, pressed-state announcements, and readable layouts at narrow widths.
  - Verify that filtering causes no assignment or model default mutation.
  - Run the applicable repository validation after the final implementation change.
  Resolution: 2026-09-22.
  - Replaced the single-task dropdown with toggle buttons in `modelTasks.js`, the dashboard, and the public explorer.
  - Selections use AND matching on the same offering with Text pressed first and one pressed button kept.
  - Connection changes preserve available selections and remove unavailable selections.
  - Updated dashboard and explorer browser coverage, documentation, and styles.
  - `make ci` passed all 14 gates, including 150 frontend browser tests and 100.0% Go statement coverage.
  - Changed prose and `git diff --check` passed.
  - Event contracts: The existing connection context event and public API contracts remain unchanged.

- [ ] [F081] (P1) {I277} Create and retain tenant API keys for later retrieval.
  Goal: Make API access available when a tenant is created and through a permanent tenant action.
  Requirements:
  - Create the tenant and its initial API key in one durable operation.
  - Make creation retries return the same tenant and key without duplicate resources.
  - Store the recoverable key encrypted on the server with the current credential encryption facilities.
  - Authorize key retrieval through the authenticated tenant owner's management session.
  - Show a tenant creation result with `Copy API key`, connection status, and an example request.
  - Keep API access available when the tenant has no connection or model default.
  - Provide permanent `Reveal`, `Copy`, and `Replace key` actions in tenant API access.
  - Retrieve the same key after dialog closure, page reload, and a new authenticated browser session.
  - Keep the key masked until an explicit reveal or copy action.
  - Clear revealed keys from browser memory when the access view closes or the session ends.
  - Exclude raw keys from browser persistent storage, logs, traces, and usage records.
  - Make key replacement explicit and revoke the replaced key when the replacement succeeds.
  - Report creation, retrieval, and replacement failures with actionable messages.
  Open Decisions:
  - Define the bounded transition for existing digest-only keys, whose original values cannot be recovered.
  - Preserve existing access until the owner explicitly replaces an existing key.
  Deliverables:
  - Update tenant creation, key storage, management endpoints, browser controls, and public contract tests.
  - Update the OpenAPI contract, applicable clients, and tenant API access documentation.
  Validation:
  - Verify atomic creation and retry behavior through the management API.
  - Verify retrieval after server restart and through a new authenticated browser session.
  - Verify owner access and rejection of another account or an expired session.
  - Verify key replacement, old-key rejection, and continued access with the replacement key.
  - Verify the flow with and without a connection or model default.
  - Verify that persisted ciphertext and captured logs contain no raw key.
  - Run the applicable repository validation after the final change.

- [ ] [F082] (P1) {F081} Return distinct tenant configuration errors for generation requests.
  Goal: Distinguish valid tenant access from missing generation configuration.
  Requirements:
  - Accept a valid tenant key independently of connection and model readiness.
  - Return `tenant_connection_required` when a generation request has no required tenant connection.
  - Give the error an actionable message that directs the owner to assign a connection.
  - Define distinct errors for a missing model default and rejected provider credentials.
  - Keep invalid tenant authentication distinct from tenant configuration errors.
  - Define HTTP statuses, error precedence, and response fields in the canonical API contract.
  - Apply the configuration errors consistently across applicable generation entry points and official clients.
  - Reject missing configuration before provider dispatch.
  - Preserve successful explicit provider and model requests when no model default is needed.
  Deliverables:
  - Update request validation, error responses, OpenAPI, applicable clients, and tenant configuration documentation.
  Validation:
  - Verify invalid keys, no connections, an unassigned requested provider, and missing required defaults.
  - Verify rejected provider credentials through a controlled upstream response.
  - Verify that missing configuration causes zero provider requests.
  - Assign a connection and the required default, then verify success with the same tenant key.
  - Verify successful explicit routes without a saved default.
  - Run the applicable repository validation after the final change.

- [ ] [F083] (P1) {F082} Retain actual connection attribution for request details and reporting.
  Goal: Show which connection each request actually used, including after tenant assignments change.
  Requirements:
  - Record the request ID, tenant ID, connection ID, connection version, provider, model, outcome, and usage.
  - Capture the resolved connection when execution selects its route.
  - Retain the connection identity for each provider attempt when one request has multiple attempts.
  - Keep historical attribution unchanged after connection rename, replacement, reassignment, or deletion.
  - Record rejected requests with their request ID, tenant identity when known, and error code.
  - Leave connection attribution empty when rejection occurs before connection selection.
  - Exclude tenant keys and provider credentials from attribution records.
  - Show the actual connection in request details and support connection-based usage reporting.
  - Restrict request details and reports to the authorized account and tenant scope.
  Deliverables:
  - Update request tracing, durable usage records, reporting contracts, and application request details.
  - Document connection identity, version semantics, and historical reporting behavior.
  Validation:
  - Send requests through two connections and verify their separate attribution and usage totals.
  - Change the tenant assignment and verify that earlier requests retain their original connection identity.
  - Rename or replace a connection and verify historical attribution remains intact after server restart.
  - Verify concurrent requests, failed attempts, and configuration rejections.
  - Verify account isolation and the absence of raw credentials in traces and reports.
  - Verify rendered request details and connection reporting through automated browser tests.
  - Run the applicable repository validation after the final change.

- [ ] [F080] (P1) Add Kimi web search through the current Moonshot search APIs.
  Goal: Let callers use `web_search=true` with supported Moonshot model offerings.
  Evidence (2026-09-20): `README.md` defines search as an optional Boolean request field. Only selected OpenAI offerings support it.
  Evidence: `configs/providers.yml` declares four Kimi offerings without `web_search: true`.
  Evidence: `internal/proxy/provider_registry.go` rejects search requests when the selected offering has no search capability.
  Evidence: `internal/proxy/openai_compatible_chat.go` returns caller function calls. It has no search execution path.
  Evidence: This investigation checked source code and official documentation. It made no live Moonshot API calls.
  Evidence: The current Kimi guide describes separate REST endpoints for Web Search Basic, Web Search Pro, and URL Fetch.
  Evidence: The built-in search guide gives October 20, 2026 as the retirement date for `$web_search`.
  Sources:
  - https://platform.kimi.ai/docs/guide/best-practices-for-web-search
  - https://platform.kimi.ai/docs/guide/use-web-search
  - https://platform.kimi.ai/docs/pricing/websearch
  Requirements:
  - Keep `web_search` as the canonical Boolean request field. Keep omission and `false` disabled.
  - Use the current standalone REST APIs under the existing Moonshot provider connection.
  - Select one search execution contract before implementation. Use `/v1/tools/search_pro` as the proposed default for answers with sources.
  - Define how the proxy produces search queries and supplies results to the selected Kimi model.
  - Declare transports, credentials, limits, and supported offerings in the provider catalog.
  - Reuse the model-free service contract from F079 when the search endpoints need service declarations.
  - Keep provider search execution separate from caller tools. Retain caller ownership of caller function execution.
  - Define request controls for result count, timeout, sites, and date range where the selected endpoint supports them.
  - Define total call and context limits. Keep source titles and URLs in the model input and final answer.
  - Define empty-result, malformed-response, authentication, rate-limit, timeout, and cancellation behavior.
  - Record search charges separately from model token usage across all calls for one request.
  - Use current REST contracts only. Exclude the retiring built-in tool and alternative compatibility paths.
  - Update capability discovery, request documentation, and client examples together.
  - Define supported combinations with caller tools, structured output, and image input before capability activation.
  Open Decisions:
  - Confirm the search endpoint, query policy, public controls, and supported Kimi offerings during implementation design.
  - Decide whether URL Fetch is necessary for the initial answer workflow.
  Cost evidence: Basic costs USD 0.002 per successful call with results. Pro costs USD 0.003 under the same condition.
  Cost evidence: Fetch costs USD 0.002 per successful call with nonempty content. Failed or empty responses have no search charge.
  Cost evidence: Model token charges remain separate. Examine current provider prices before implementation.
  Deliverables: Catalog declarations, search execution, usage records, public documentation, and integration tests.
  Validation:
  - Start with failing HTTP integration tests through `POST /v2` and a local Moonshot protocol server.
  - Examine enabled search, disabled search, unsupported offerings, source links, error responses, cancellation, and usage totals.
  - Make sure that disabled search causes no search API calls.
  - Make sure that search execution obeys call limits and preserves caller tools and reasoning messages.
  - Run `make ci` after the final application change.
  - Record live provider qualification separately when paid calls are authorized. Keep source acceptance separate from deployment.

- [x] [F079] (P1) Declare model-free provider services in the shared catalog.
  Goal: Execute provider services that do not select a model through the current durable operation API.
  Evidence: MediaOps alignment, dictionary creation, composition upload, stem separation, and account mutations include requests without a model.
  Evidence: The current operation API requires a model and resolves only model offerings.
  Requirements: Add typed service bindings within the existing provider definition in `configs/providers.yml`.
  Requirements: Reuse provider fields, connection authority, transports, controls, limits, admission, artifacts, and recovery.
  Requirements: Select a declared service only when the request omits a model. Reject undeclared routes and invalid model combinations.
  Requirements: Do not add fake model identifiers or a second provider definition.
  Requirements: Define exact operation, input, output, and price contracts before each native adapter.
  Requirements: Include available service bindings in public, tenant, and management discovery without adding model families.
  Requirements: Update both official clients, OpenAPI, and the browser resource presentation.
  Deliverables: Implement ElevenLabs forced alignment as the first native service through the existing provider connection.
  Resolution: Added model-free service bindings under the existing provider hierarchy and native ElevenLabs forced alignment.
  Resolution: Public, tenant, and management discovery, both clients, MCP, and browser views use this catalog contract.
  Resolution: Alignment preserves timing and loss artifacts, shared account authority, provider usage, cancellation state, and recovery without resubmission.
  Validation: Initial tests rejected omitted models and missing service discovery. HTTP, MCP, client, and browser corrections pass.
  Validation: CI exposed adapter-map mutation during router construction and one uncovered decoder rejection. Both corrections pass.
  Validation: Final CI passes all 14 gates in 345 seconds with 100.0 percent Go coverage, 146 frontend tests, and seven real-server browser tests.
  Evidence: `/tmp/llm-proxy-f079-ci-complete.log` records final source validation.
  Files: `configs/providers.yml`, provider service and alignment modules, durable operation and usage modules, both clients, OpenAPI, browser views, and public tests.
  Contract: An omitted operation model selects only a declared service. Discovery includes `services`. Model routes retain exact model selection.
  Contract: This source result does not establish deployment, client publication, native provider qualification, or MediaOps consumer acceptance.
  Validation: Start with failing public HTTP and browser tests.
  Validation: Prove one connection across model offerings and services, a second catalog identity, exact dispatch, and no retry of uncertain mutations.
  Validation: Complete local CI and preserve separate native provider and consumer acceptance records.


- [x] [F078] (P1) Add decimal controls to the provider catalog.
  Goal: Describe bounded voice settings in the same YAML contract as integer, Boolean, and enum controls.
  Requirements: Add the `number` control kind with finite minimum and maximum values.
  Requirements: Preserve integer constraints for integer controls. Reject invalid ranges before runtime construction.
  Requirements: Project exact decimal bounds through public discovery, tenant discovery, and the website build.
  Validation: Start with failing public catalog and website tests. Verify the generated OpenAPI contract and final CI.
  Deliverables: Typed control bounds, catalog validation, public contract, renderer, and tests.
  Initial results: The catalog and website rejected the new number kind. Integer controls silently truncated decimal bounds under B238.
  Progress: Focused HTTP, tenant-client, immutable-snapshot, and browser tests now pass.
  Initial results: CI required Go formatting in two files. Formatting is corrected.
  Resolution: One control schema now supports decimal ranges through YAML, HTTP discovery, the tenant client, and the website build.
  Validation: Final `make ci` passed all 14 gates in 354 seconds with 100.0 percent Go statement coverage.
  Validation: All 145 frontend tests and seven local browser tests passed. No new event contract was added.

- [x] [F077] (P1) Add catalog bindings for provider resources.
  Goal: Expose provider resources through the same provider definition and account connection as model operations.
  Requirements: Add typed resource bindings for voices, history, dictionaries, metadata, quotas, and reusable elements.
  Requirements: Reference provider transports and shared fields. Reject dangling references and unsupported compositions at YAML load time.
  Requirements: Keep model offerings separate from provider resources. Do not invent model identifiers for account operations.
  Requirements: Define each public resource schema before its adapter implementation under F026 or F027.
  Validation: Verify discovery, tenant isolation, credential replacement, and a second provider identity through public entry points.
  Deliverables: Typed YAML bindings, registry projection, public resource contracts, and integration tests.
  Progress: Typed resource bindings now share provider transports and connection fields. Resource-only providers need no model offering.
  Progress: Voice discovery uses the explicit binding. Public and tenant catalogs expose declared resource kinds.
  Progress: Public tests prove connection replacement, detachment, tenant isolation, and a second provider identity.
  Progress: OpenAPI and both official clients use the current discovery shape. Resource, catalog, Dictator, media, and client-contract tests pass.
  Initial results: Two CLI fixtures omitted the new resource array. Corrected fixtures pass the CLI component.
  Initial results: The website parser rejected the resource array. Updated parsing passes 140 frontend tests.
  Initial results: A resource-only browser fixture exposed a model-family requirement in connection setup.
  Progress: The corrected form saves a resource-only connection, shows Voices, and preserves it after reload without fake models.
  Resolution: Resource bindings use shared provider transports and connections. Discovery and connection setup support resources without model offerings.
  Validation: Final `make ci` passed all 14 gates in 347 seconds with 100.0 percent Go statement coverage.
  Validation: All 140 frontend tests and seven local browser tests passed. No new event contract was added.

- [x] [F076] (P1) Add catalog verification for media-only providers.
  Goal: Support media-only account connections without duplicate providers or fake text models.
  Evidence: The account verifier selects a text route or the separate Dictator protocol case.
  Requirements: Add one typed verification binding inside each existing provider definition.
  Requirements: Reference declared transports and shared connection fields. Reject dangling references and unsupported compositions at YAML load time.
  Requirements: Use read-only upstream verification where available. Keep paid generation outside routine connection verification.
  Requirements: Keep provider identity independent of capability type. Do not create separate media provider aliases.
  Requirements: Keep model offerings separate from provider resources. Do not invent model identifiers for account operations.
  Validation: Exercise account creation, credential replacement, revocation, tenant isolation, and browser discovery through public entry points.
  Validation: Prove a second provider identity through catalog data and connection values without production code changes.
  Deliverables: Typed YAML schema, shared registry projection, connection verification, and public integration tests.
  Progress: Schema version 5 requires an explicit verification binding for every provider. Version 4 is rejected.
  Progress: The `json_resource` codec uses authenticated GET with bounded response size and request duration.
  Progress: Public HTTP tests prove two media-only provider identities, credential rotation, rejection, and owner isolation.
  Progress: A real browser creates and reloads a media-only connection through the catalog-defined field and verification route.
  Initial results: CI exposed stale protocol, Gemini, and image fixture bindings. The corrected fixtures pass their component targets.
  Initial results: A later CI run lost a Go standard-library cache file. The MCP component and final CI passed on repeat.
  Resolution: Explicit verification now shares each provider definition, credential fields, and connection lifecycle.
  Validation: Final `make ci` passed all gates with 100.0 percent Go statement coverage, 135 frontend tests, and seven local browser tests.
  Files: Provider catalog schema, registry, protocol definitions, account verifier, shared fixtures, HTTP tests, browser tests, and catalog documentation.
  Event contracts: No changes.

- [ ] [F075] (P1) Expose provider routes through the Anthropic Messages client protocol.
  Goal:
  Let a customer use an Anthropic-compatible client with any catalog provider route.
  Make the public Anthropic interface a protocol adapter over the existing provider-neutral completion coordinator.
  Keep this issue independent from F074. This issue does not add Jev or change the Jev typed-decision contract.
  Research:
  - The current public client registry exposes OpenAI Chat Completions, OpenAI Responses, model listing, and transcription routes.
  - The current public registry does not expose `POST /v1/messages`.
  - The current `anthropic_messages` implementation is an upstream provider codec.
  - An upstream codec does not provide an Anthropic-shaped public client interface.
  - The public adapter must translate one Anthropic request into the canonical completion request.
  - The public adapter must translate one canonical result into an Anthropic Message response.
  - The Anthropic Messages API uses `model`, `max_tokens`, and `messages` as core request fields.
  - The Anthropic Messages API returns `id`, `type`, `role`, `content`, `model`, `stop_reason`, and `usage` fields.
  - The Messages API is stateless. The caller sends the conversation history with each request.
  - The official Messages documentation defines content blocks, tools, streaming events, and error behavior.
  - Source reference: `https://platform.claude.com/docs/en/build-with-claude/working-with-messages`.
  Requirements:
  - Add a public `POST /v1/messages` route.
  - Register the route through the existing `ClientProtocolAdapter` registry.
  - Add a dedicated Anthropic client protocol adapter near the current public client adapters.
  - Keep public protocol translation separate from upstream provider transport selection.
  - Reuse the existing tenant authentication, request identifiers, limits, timeout, usage, and accounting flow.
  - Accept the authentication headers required by the selected Anthropic client contract.
  - Map public Anthropic authentication to the existing tenant authentication policy.
  - Never use a customer authentication value as an upstream provider credential.
  - Parse the request into a typed Anthropic request model.
  - Require `model`, `max_tokens`, and a non-empty `messages` list.
  - Use the current provider and model selection contract without adding provider-specific selection branches.
  - Support user and assistant messages with string or typed content blocks.
  - Support text blocks in the first release.
  - Support image, tool-use, and tool-result blocks only when the canonical route declares each capability.
  - Support the top-level `system` field only when the selected route accepts system instructions.
  - Map `temperature`, `top_p`, `top_k`, and `stop_sequences` through explicit canonical fields.
  - Reject an unsupported request field before provider dispatch.
  - Do not silently drop an Anthropic request field.
  - Map `tools` and `tool_choice` to the canonical tool contract.
  - Return tool calls as Anthropic `tool_use` content blocks.
  - Accept caller tool results as Anthropic `tool_result` content blocks.
  - Keep tool execution in the caller. The proxy only transports and normalizes tool calls.
  - Map structured output controls only when the canonical route declares structured output support.
  - Map reasoning controls only when the canonical route declares reasoning support.
  - Reject media, tools, structured output, reasoning, and other controls that the route does not declare.
  - Use provider catalog capabilities as the single feature support matrix.
  - Dispatch the canonical request through the existing completion coordinator.
  - Keep the existing upstream `anthropic_messages` codec separate from the public adapter.
  - Return the canonical Anthropic Message shape for `stream: false`.
  - Include a proxy-owned message identifier and the selected model identifier.
  - Preserve content block order and stable tool-use identifiers.
  - Normalize available input and output token usage into the Anthropic usage shape.
  - Define one result for routes that cannot report a required usage field.
  - Return the proxy request identifier in the response headers.
  - Encode failures as the canonical Anthropic error envelope.
  - Map validation, authentication, authorization, model, rate-limit, provider, and timeout failures to Anthropic error types.
  - Keep provider credentials, raw provider payloads, and internal routing data out of customer responses.
  - Keep provider credentials, raw provider payloads, and internal routing data out of request logs.
  - Support `stream: true` with the documented Anthropic server-sent event shape.
  - Emit the documented `message_start`, content-block, `message_delta`, and `message_stop` events in order.
  - Encode text deltas and tool-input JSON deltas with the documented event fields.
  - Emit an Anthropic error event when a stream fails after headers are sent.
  - Use the existing buffered-SSE behavior for routes without native upstream streaming.
  - Document buffered streaming so clients do not expect token-level latency from every route.
  - Return an `Allow` header and the canonical Anthropic error for unsupported methods.
  - Keep the existing OpenAI client routes unchanged.
  - Do not add a legacy `/v1/complete` route, compatibility alias, or provider fallback.
  - Keep model discovery on the existing model-list route unless a separate Anthropic model-list contract is approved.
  Deliverables:
  - A typed Anthropic client request, content block, tool, response, stream event, and error model.
  - A public Anthropic protocol adapter that uses the canonical completion coordinator.
  - A registered `POST /v1/messages` route with method and error handling.
  - OpenAPI schemas for the request, response, stream events, and error envelope.
  - Documentation for Anthropic SDK configuration, authentication, model selection, supported fields, and buffered streaming.
  - A capability table that maps Anthropic request controls to catalog capabilities.
  - Integration fixtures for each active upstream provider protocol family.
  - Integration tests that send the same Anthropic request through multiple provider routes.
  - Integration tests for text, tools, media, structured output, reasoning, streaming, and error paths.
  - Tests that prove unsupported capabilities fail before an upstream request is sent.
  - Tests that prove the existing OpenAI client routes remain operational.
  - A provider qualification procedure for every enabled route that uses this client protocol.
  Validation:
  - Run the repository test target with the public HTTP server and real route registry.
  - Run the repository lint target.
  - Run the repository CI target.
  - Send a non-streaming Anthropic request through each active provider protocol family.
  - Verify the response has the Anthropic Message type, role, content, stop reason, and usage fields.
  - Send a streaming Anthropic request through each route that declares streaming support.
  - Verify the event order and final usage in the Anthropic stream.
  - Send a tool request and verify the returned `tool_use` block.
  - Send the matching tool result and verify the next completion includes the caller result.
  - Send an unsupported capability and verify no upstream request occurs.
  - Send invalid, unauthenticated, unauthorized, unknown-model, rate-limited, and provider-failure requests.
  - Verify each failure has the Anthropic status and error shape.
  - Verify customer credentials and raw provider payloads do not appear in logs or responses.
  - Verify OpenAI Chat Completions and OpenAI Responses acceptance tests still pass.
  - Run provider qualification against the live provider routes required by the deployment policy.
  Open Decisions:
  - Select the first supported Anthropic API version and the accepted `anthropic-version` and beta headers.
  - Select the first-release content-block set beyond text, image, tool-use, and tool-result blocks.
  - Select whether structured output uses the current Anthropic format or a later separate contract.
  - Select the usage result when an upstream route cannot report one required Anthropic usage field.
  - Confirm that buffered SSE is the first-release behavior for routes without native streaming.
  - Confirm that Files, Batches, token counting, prompt caching, citations, and computer-use endpoints stay out of scope.

- [ ] [F074] (P1) Add Jev typed-decision offerings with provider protocol adapters.
  Goal:
  Add Jev as a provider-independent model family with one canonical typed-decision contract and explicit provider transports.
  Preserve Jev's typed Choice, Score, and Noul semantics when a tenant selects the direct TypeSafe route or a broker route.
  Do not present a general text model or a model-backed System One emulator as Jev.
  Research:
  - The TypeSafe HTTP contract uses `POST https://api.typesafe.ai/v1/systemone` with a bearer key, `state`, `model`, and typed `questions`.
  - The TypeSafe quickstart creates the key in `https://console.typesafe.ai/settings/keys` after account access.
  - Vercel exposes Jev through `experimental_evaluate` with model `typesafe-ai/jev`.
  - OpenRouter lists Jev as `~typesafe/jev-latest` through its OpenAI-compatible API at `https://openrouter.ai/api/v1`.
  - The official TypeSafe System One Adapter keeps the typed `system_one` interface while executing against OpenAI or Anthropic.
  - The System One Adapter is a separate model-backed evaluator. It is not the Jev model and must not be registered as a Jev provider offering.
  - Source references: `https://docs.typesafe.ai/introduction/quickstart`, `https://vercel.com/ai-gateway/models/jev`, `https://openrouter.ai/typesafe`, and `https://github.com/typesafe-ai/system-one-adapter-python`.
  Requirements:
  - Add one closed provider-catalog operation for typed decisions. Use one canonical internal name for the operation and document its relation to TypeSafe's `system_one` protocol.
  - Define one canonical request type with an exact model, one structured `state` value, and named questions.
  - Define a closed question union for Choice, Score, and Noul. Reject unknown question kinds and unknown fields at the public boundary.
  - Validate question identifiers, required instructions, Choice labels, Score criteria, and Noul criteria before provider dispatch.
  - Define one canonical response type with the exact model and answers keyed by question identifier.
  - Include typed answer values, provider probabilities or confidence data, and normalized usage.
  - Keep typed answers as typed values. Do not parse a free-text completion to reconstruct a Jev answer in the canonical direct route.
  - Add a narrow decision protocol adapter interface.
  - Make the interface accept the canonical request and return the canonical response or a typed provider error.
  - Add a TypeSafe transport for synchronous bearer-authenticated `POST /v1/systemone` requests with the `jev-latest` upstream model.
  - Add an OpenRouter transport only after a request and response fixture proves the `~typesafe/jev-latest` route.
  - Use the fixture to verify canonical typed-decision semantics through the OpenAI-compatible endpoint.
  - Add a Vercel transport only after the raw gateway request, response, authentication, usage, and error contract is verified. Do not infer a Go transport from the Vercel SDK example alone.
  - Keep the TypeSafe, OpenRouter, and Vercel routes as separate provider definitions.
  - Give each route separate credentials, endpoints, terms, prices, usage mapping, and provider identity.
  - Keep provider identity as catalog data. Select adapter code from the declared provider transport and protocol components.
  - Do not route Jev through the current text route adapter or structured-output text contract.
  - Use a generic OpenAI Chat Completions composition only after an exact fixture proves that composition is verified.
  - Do not add an automatic provider fallback, a legacy Jev alias, or a compatibility read for an obsolete request shape.
  - Add one canonical resource-oriented REST representation to `docs/openapi.yaml` and the native client contract. Do not overload chat message paths with Jev question objects.
  - Return typed errors for invalid questions, unsupported operations, authentication failures, provider failures, malformed answers, and missing required usage fields.
  - Keep provider secrets and raw provider responses out of tenant responses, logs, and usage views.
  - Keep private broker metadata out of public capability projections.
  - Add provider connection fields and key-acquisition links for each route without storing credential values in `configs/providers.yml`.
  - Register the TypeSafe publisher, Jev model family, exact model records, provider offerings, operation defaults, limits, and prices.
  - Register each route only after its transport contract is verified.
  - Treat official and community SDKs as client libraries. Do not create provider definitions for SDK implementations.
  Deliverables:
  - Canonical typed-decision request, response, question, answer, usage, and error types.
  - Decision protocol adapters and provider transports for the qualified TypeSafe route and each approved broker route.
  - Provider catalog definitions in `configs/providers.yml` and startup composition support in `internal/proxy/provider_catalog_schema.go`, `internal/proxy/catalog_service.go`, and `internal/proxy/provider_protocol_definitions.go`.
  - Decision routing integration separate from `textRouteAdapter` in `internal/proxy/provider_router.go` and the current text request types.
  - OpenAPI schemas, native Go and Python client support, model discovery, public capability output, management connection forms, and provider setup documentation.
  - A provider-specific Jev/System One integration document.
  - Record account registration, credential fields, endpoint ownership, upstream model identifiers, usage fields, error mapping, and live-qualification evidence.
  - Explicit documentation for the boundary between direct Jev, brokered Jev, and the non-Jev System One Adapter.
  Validation:
  - Add public-entrypoint integration tests before production changes. Use injected upstream HTTP servers for deterministic protocol tests.
  - Verify that valid Choice, Score, and Noul requests produce the canonical typed response through the TypeSafe adapter fixture.
  - Verify that invalid question kinds, unknown fields, invalid criteria, unsupported models, and unsupported provider operations fail before any upstream request.
  - Verify request serialization, bearer authentication, model selection, and response parsing for each adapter.
  - Verify usage normalization, provider error mapping, malformed-answer handling, and request identifiers for each adapter.
  - Verify equivalent typed answers through each qualified route for identical canonical requests.
  - Preserve distinct provider and upstream model identity in each result.
  - Verify that a provider failure does not call another provider and does not invoke the System One Adapter.
  - Verify tenant credential isolation, management connection creation, and management connection update.
  - Verify key verification, route discovery, and public capability output for every registered provider.
  - Verify the native REST contract, client behavior, status codes, and typed errors through a real HTTP listener.
  - Verify generated or checked OpenAPI artifacts through the same listener.
  - Run `make test-provider-catalog` and `make test-protocol-acceptance` after catalog and adapter changes.
  - Run one separate authorized live qualification for each route with its own account and key. Record the exact model, endpoint, request, response, usage, and provider evidence.
  - Run final `make ci` after the last source, catalog, client, documentation, and test change.
  Open Decisions:
  - Select the canonical operation identifier and public REST path before implementation.
  - Use `typed_decision` to describe the capability. Use `system_one` to match the TypeSafe protocol name.
  - Confirm whether the first release includes Vercel and OpenRouter, or only direct TypeSafe access.
  - Confirm whether `state` accepts arbitrary JSON or one closed state shape in the public contract.
  - Confirm whether broker routes must expose provider-selected upstream metadata to tenants or only retain it in private usage records.
  - Open a separate feature if the product needs the TypeSafe System One Adapter.
  - Define that feature as a model-backed evaluator over OpenAI or Anthropic.

- [ ] [F072] (P1) Expose granular Dictator models for Whisper transcription and Qwen3 and Silero synthesis.
  Current handoff:
  - Supply exact model and capability evidence to F042 and MediaOps I087.
  - Keep engine routing in the gateway. MediaOps and TelePrompter use the declared public capability contract.
  - Verify existing source before implementing missing work.
  Goal:
  Replace the monolithic `dictator-speech-v1` catalog entry with distinct, purpose-built models for Dictator's underlying Whisper transcription and Qwen3/Silero speech synthesis engines.
  Requirements:
  - Remove the umbrella `dictator-speech-v1` model definition from `configs/providers.yml` under forward-only contract discipline.
  - Register distinct Whisper transcription models by size: `whisper-tiny`, `whisper-base`, `whisper-small`, `whisper-medium`, and `whisper-large-v3` under publisher `openai` and family `whisper`.
  - Bind each Whisper model offering to Dictator's speech transport supporting operations `audio_transcription`, `audio_diarization`, `audio_alignment`, `subtitle_creation`, and `voice_extraction`.
  - Register distinct speech synthesis models: `qwen3-tts` (family `qwen3`) and `silero-ru` (family `silero`) under Dictator's speech transport supporting `speech_generation`.
  - Update `internal/proxy/dictator_grpc_protocol.go` and `dictator_adapter.go` to derive `model_size` directly from the selected Whisper model identifier without requiring out-of-band control parameters.
  - Update synthesis requests in the Dictator gRPC protocol to automatically set `SynthesisEngine_SYNTHESIS_ENGINE_QWEN3` for `qwen3-tts` and `SynthesisEngine_SYNTHESIS_ENGINE_SILERO_RU` for `silero-ru`.
  - Update official Go and Python client tests and contract fixtures to exercise the granular models.
  - Coordinate downstream caller migration in `CreativeDirector` to pass the canonical granular model (e.g. `whisper-large-v3` or `whisper-base`) instead of `dictator-speech-v1`.
  Deliverables:
  - Updated `configs/providers.yml` catalog models and provider offerings.
  - Updated Dictator protocol adapter in `internal/proxy/dictator_grpc_protocol.go` and `dictator_adapter.go`.
  - Updated test fixtures, OpenAPI schemas, and documentation in `docs/speech-workflows.md` and `docs/tenant-connections.md`.
  Validation:
  - `make test-dictator` passes for all granular Whisper and synthesis routes.
  - Public HTTP integration tests verify job submission, cancellation, and artifact delivery for each granular model.
  - Live acceptance tests pass through `make test-dictator-live`.
  - Final local `make ci` passes with 100% statement coverage.

- [ ] [F073] (P1) {F072} Deconstruct umbrella Media capability into specific Transcription and Speech taxonomy.
  Current ownership:
  - This issue changes the LLM Proxy management interface and capability discovery.
  - MediaOps projects those capabilities through its API. TelePrompter owns the creative product controls.
  - Reconcile the existing task filters before further interface changes.
  - This management UI refinement is outside the retained-provider migration gate.
  Goal:
  Replace the catch-all "Media" capability in the management dashboard and public capabilities catalog with first-class domain capabilities for Transcription, Speech synthesis, and visual media.
  Requirements:
  - Split the umbrella `media` capability group into explicit, domain-specific capability buckets: `text`, `transcription` (unifying `dictation`, `audio_transcription`, `audio_diarization`, `audio_alignment`, and `subtitle_creation`), `speech` (unifying `speech_generation` and `voice_extraction`), `image`, and `video`.
  - Unify transcription models across all providers (`whisper-*`, `gpt-transcribe`, `gemini-3.5-transcribe`, `xai-stt`, `sensevoice-small`) under the `Transcription` capability tab in `connectionDashboard.js`.
  - Surface speech synthesis models (`qwen3-tts`, `silero-ru`) under a dedicated `Speech` (TTS) capability tab.
  - Implement dynamic capability tab fallback: when a tenant or connection is selected, if the currently active tab has no models for that connection, automatically select the first capability tab that contains available models.
  - Document and enforce canonical model card visual states:
    - Default state: `.cw-node.selected` with solid teal border, teal background (`#18302d`), `★ Default model` badge (`.cw-connected`), and solid teal SVG Bezier wire from the active connection card.
    - Preview state: `.cw-node.preview` with dashed amber border (`#e8b75e`), dashed amber SVG Bezier wire (`stroke-dasharray: 5 4`), and footer route text reflecting `Preview: <model-id>`.
    - Unselected state: `.cw-node` with brand icon (`brand-icon`) and monospace code identifier (`<code>`).
    - Disabled state: Model buttons disabled when `this.busy` is true or when the active connection is not ready.
  - Document and enforce model card click and preview interaction lifecycle:
    - Clicking an unselected model card puts it into preview mode and renders the capability-specific configuration form (`[data-default-form]`) in the details panel (`[data-details]`).
    - Model selection remains in preview until confirmed via explicit save button (`Save text default`, `Save transcription default`, `Save speech default`).
    - Switching tenants or connections immediately clears unsaved model card previews and form drafts without mutating tenant defaults.
    - Preserving other capability defaults: saving a default in one capability domain (e.g. Transcription) must preserve the existing saved defaults of other domains (e.g. Text or Speech).
  - Implement domain-specific configuration forms (`[data-default-form]`) upon model card selection:
    - Text: system prompt textarea, reasoning effort selector (when supported by model), and `Save text default` button.
    - Transcription: language selection and timestamp controls (when supported), and `Save transcription default` button persisting `defaults.transcription_provider` and `defaults.transcription_model`.
    - Speech (TTS): voice preset selection (Silero presets vs Qwen3 reference voices), sample rate / format controls, and `Save speech default` button persisting `defaults.speech_provider` and `defaults.speech_model`.
    - Visual media (Image / Video): model offering details and capability badges.
  - Enforce model column empty states:
    - `"Connect to choose models. Use a Connect button in the middle column."` when connection is unattached.
    - `"Add credentials to choose models. Edit this connection to complete its setup."` when connection credentials are missing.
    - `"No models for this capability."` when the connection has no offerings for the active capability tab.
  - Update `site/assets/llm-proxy/js/constants.js` and `types.d.js` to define the canonical capability domain constants and labels.
  - Update public capability catalog projection in `internal/proxy/public_capabilities.go` to publish normalized capability domain classifications.
  Deliverables:
  - Updated capability catalog projection in `internal/proxy/public_capabilities.go`.
  - Updated dashboard component in `site/assets/llm-proxy/js/ui/connectionDashboard.js` and capability constants in `constants.js`.
  - Playwright integration tests covering capability tab rendering, automatic tab fallback, and model card lifecycle (preview, save default, wire rendering, domain forms) across providers.
  Validation:
  - `npx playwright test tests/e2e/management-ui.spec.js -g "dashboard|connection"` passes.
  - `npm run frontend:lint` passes without type or syntax errors.
  - Final local `make ci` passes all quality and coverage gates.

- [ ] [F065] (P1) Add platform connections and explicit hosted access grants.
  Goal:
  Let customers use approved provider offerings through platform credentials after account creation.
  Give each customer separate tenant access, usage attribution, and billing ownership.
  Billing account, platform connection, and grant resources are implemented in the development branch.
  Grant tests cover account isolation, concurrent revisions, audit rollback, and recovery after restart or catalog changes.
  Explicit assignments, database exclusion constraints, and browser hosted setup are implemented.
  HTTP tests reject unavailable hosted execution across native, client, MCP, dictation, and media interfaces without upstream calls.
  Browser tests verify hosted setup and grant suspension at desktop, 390px, and 320px widths.
  The resource foundation is in [PR 339](https://github.com/tyemirov/llm-proxy/pull/339).
  Hosted CI run 35804548155 passed frontend and backend-checks jobs.
  The backend coverage gate reported `coverage total 99.0%, want 100.0%`.
  Financial admission and accepted-work recovery are implemented in the stack.
  Final CI and complete F070 acceptance remain open.
  Requirements:
  - Use F070 as the shared hosted service contract.
  - Extend the ownership rules in `docs/tenant-connections.md` and the account connection store.
  - Create one billing account for each management account through an explicit mutation.
  - Keep account identity, tenant identity, platform connection identity, and provider credential version separate.
  - Define platform connections as operator resources with encrypted credentials and explicit provider settings.
  - Add hosted access grants with account, tenant, offering, platform connection, state, and revision fields.
  - Define grant states as `active`, `suspended`, and `revoked`.
  - Select either an account connection or a hosted access grant for each tenant provider assignment.
  - Represent this selection with a closed resource type and an explicit identifier.
  - Preserve customer-owned connections as an explicit product choice with separate usage attribution.
  - Require an explicit assignment change between customer credentials and platform credentials.
  - Keep platform credentials and private provider handles outside customer responses, exports, and logs.
  - Add account reads and grant resources under `/api/management/billing-accounts` and `/api/management/hosted-access-grants`.
  - Restrict platform connection mutations to the existing authenticated operator boundary or a documented operator CLI.
  - Require idempotency keys for resource creation and revision preconditions for grant changes.
  - Resolve the exact grant, tenant, connection version, offering, and catalog revision before admission.
  - Recheck grant authority before each new upstream attempt or continuation.
  - Preserve accepted credential references for recovery after credential rotation.
  - Define revocation behavior separately for queued, dispatched, and recoverable work.
  - Keep result recovery possible without authorizing new provider work after revocation.
  - Make hosted eligibility depend on credential qualification, pricing, metering, and an active grant.
  - Keep public catalog availability separate from paid customer eligibility.
  - Apply the same authority contract to native HTTP, client protocols, MCP, dictation, and media operations.
  - Require the completed F068 admission check before production requests use platform credentials.
  - Keep hosted execution disabled until F070 development acceptance passes.
  Deliverables:
  - Billing account, platform connection, grant, and assignment schemas with database constraints.
  - Operator provisioning interface, customer entitlement views, and audited grant transitions.
  - Onboarding flow that creates tenant access without requesting provider credentials.
  - OpenAPI, applicable clients, README, and tenant connection documentation updates.
  Validation:
  - Exercise account creation, grant creation, and provider dispatch through real authenticated HTTP entry points.
  - Prove one customer's key cannot use another customer's grant, resources, or results.
  - Prove customers cannot read or change platform credentials.
  - Prove duplicate creation, revision conflicts, rotation, revocation, and restart behavior.
  - Prove an unavailable grant causes zero upstream dispatches across every supported entry point.
  - Prove existing customer-owned assignments remain explicit and independently attributed.
  - Verify onboarding through the real browser and controlled provider endpoints.
  - Run `make ci` after the last application change.

- [ ] [F066] (P1) {F065} Add a durable usage journal for customer billing.
  Goal:
  Record every hosted request and upstream attempt with enough evidence to explain its financial outcome after a restart.
  Account-owned journal reads and browser client methods are implemented in the development branch.
  Database tests cover concurrent admission, reservation rollback, worker claims, dispatch recovery, exact quantities, and accounting delivery.
  Controlled HTTP tests cover text continuations, synthesis, polling, lost responses, invalid JSON, and grant denial.
  Hosted text identity tests cover concurrent database instances, result replay, response expiry, and native and client protocols.
  Text recovery tests interrupt the service before dispatch, during provider work, after observation, and before and after result publication.
  HTTP tests verify worker replacement, retained usage, publication recovery, and rejection of obsolete worker writes.
  Authenticated MCP tests verify shared native identity, concurrent replay, changed intent, account isolation, result expiry, and uncertain outcomes.
  Go, Python, and CLI acceptance verifies shared hosted keys, pending states, and result expiry through the service.
  Controlled media HTTP tests verify atomic admission, shared operation identity, concurrent replay, authority changes, and rollback.
  Text startup recovery leaves media records for the media worker.
  Hosted image worker tests cover accepted credential versions, dispatch authority, obsolete workers, provider recovery, and cancellation.
  Media outcomes and journal outcomes use one transaction. Observations and delivery records also commit together.
  Missing media meters retain unknown usage.
  The Images API adapter records exact token evidence before image validation and asset publication.
  HTTP tests cover completed streams, contradictory totals, missing quantities, evidence write failures, and delayed obsolete responses.
  Unknown cache measurements prevent complete image usage.
  The Responses image adapter retains exact terminal response usage from JSON, polling, and streams before result publication.
  Separate response quantities preserve cache and reasoning inclusion. Unqualified image tool counters remain unknown.
  Controlled HTTP tests verify failed outcomes, missing usage, replay, and evidence write failures across all three response paths.
  B250 resolved missing operator reports for media worker persistence failures.
  B251 prevents adapter recovery content from becoming journal provider identifiers.
  B252 supplies hosted authority during existing speech validation and preserves the final admission check.
  B253 applies shared dispatch authority to HTTP and gRPC, including recovery and cancellation.
  Hosted voice discovery and previews now use current speech grants and qualified platform credentials through existing adapters.
  HTTP and gRPC tests verify private voice isolation, retained tenant voices, obsolete cursors, revocation, and separate metadata permission.
  Hosted grants and journal records now support explicit provider services without model identifiers.
  HTTP tests verify dictionary creation, audio alignment, retained identity, exact service authority, and revocation through existing adapters.
  The browser displays and assigns service grants at desktop and phone widths.
  Customer journal views now expose bounded attempts, exact usage observations, and safe reconciliation cases through the existing dashboard.
  HTTP tests verify account isolation, pagination, private-field omission, and unchanged journal records.
  Browser tests verify exact values, unknown quantities, additional pages, failed reads, and desktop and phone layouts.
  Hosted dictation now uses shared completion admission, pinned credentials, replay, and recovery.
  HTTP tests verify cross-interface identity, concurrent requests, revocation, expiry, exact measurements, and uncertain outcomes after failures.
  Catalog-driven HTTP tests verify each current dictation offering through its existing adapter and both transcription interfaces.
  Dictation recovery tests verify all five process-interruption boundaries with retained usage and execution identity.
  Hosted file tests verify staging authority, cleanup after revocation, claim expiry, and replay without repeated generation.
  B256 verifies maintenance failure reports, transaction rollback, repeated recovery, and financial evidence after operational retention.
  The existing ElevenLabs generation and conversion adapters retain exact provider cost units before audio publication.
  HTTP tests cover every current offering for both protocols, unknown usage, failures, multipart conversion, and replay.
  Anthropic cache evidence now retains exact five-minute and one-hour subdivisions with explicit inclusion rules.
  Shared token validation preserves contradictory numeric source evidence while marking the corresponding quantities as unknown.
  Hosted web search retains per-attempt search action counts without queries, URLs, or tool identifiers.
  HTTP tests verify unknown tool usage, polling, continuations, transaction rollback, and failure replay without repeated work.
  Google text and dictation meters now retain exact modality arrays with explicit inclusion rules.
  HTTP tests verify unknown subdivisions, invalid counts, cache boundaries, and replay through both Google protocols.
  xAI Responses evidence now retains exact provider cost in USD ticks separately from token measurements.
  FAL queue results now retain exact billed units through the shared decimal header reader.
  HTTP tests verify failed results, artifact failures, transaction rollback, and recovery without another submission.
  The hosted Dictator gRPC boundary now retains terminal synthesis duration before artifact transfer.
  HTTP and gRPC tests verify native precision, unknown duration, terminal states, failed writes, and artifact loss.
  The same gRPC boundary now records unknown input duration for all five Dictator audio input operations.
  Voice extraction retains reported sample duration separately from unknown input duration.
  HTTP acceptance covers each current audio input offering and rejects duplicate work on replay.
  A full operational telemetry queue does not remove hosted usage or its delivery record.
  HTTP acceptance verifies this separation and one provider call across replay.
  Controlled HTTP acceptance covers all 67 active text offerings, both image editing surfaces, and both Dictator synthesis offerings.
  Timeout and client disconnect acceptance preserve uncertain dispatches and prevent repeated provider work.
  The journal implementation passes component acceptance. F067 and F068 connect rating and funds settlement to its delivery transaction.
  Final stack CI remains the F070 completion checkpoint.
  Requirements:
  - Use F070 as the shared hosted service contract.
  - Replace the billing dependency on `management_usage_writer.go` with a durable journal in the managed database.
  - Retain operational telemetry as a separate, explicitly nonfinancial projection.
  - Create request, attempt, usage observation, and reconciliation records with stable opaque identifiers.
  - Bind each request to its billing account, tenant, grant, offering, connection version, and catalog revision.
  - Bind the selected price snapshot and funds reservation when F067 and F068 provide them.
  - Require one tenant-scoped idempotency key for every hosted generation request.
  - Reuse current structured request and media operation identities instead of creating competing execution records.
  - Define native, client protocol, and MCP representations for the same hosted idempotency contract.
  - Return the retained result or operation for identical intent and reject changed intent with `409`.
  - Record admission and dispatch intent before each upstream attempt starts.
  - Assign separate attempt identifiers to continuations and explicit retries.
  - Use database uniqueness constraints for request identities, attempt numbers, observations, and final accounting effects.
  - Define request states as `accepted`, `executing`, `completed`, `failed`, and `uncertain`.
  - Define attempt states as `prepared`, `dispatched`, `observed`, and `uncertain`.
  - Keep execution outcomes separate from usage completeness and customer charge eligibility.
  - Record provider request identifiers privately when the provider supplies them.
  - Record input, output, cache-read, cache-write, and other independently billed token quantities when applicable.
  - Record audio duration, generated media quantities, tool calls, and provider-specific billing units when applicable.
  - Preserve the provider's inclusion rules to prevent duplicate counting of reasoning or cached tokens.
  - Represent missing quantities as unknown with a reason, never as measured zero.
  - Retain normalized evidence, source fields, adapter revision, timestamps, and evidence digests without prompt or credential content.
  - Commit observations and accounting delivery records together before financial settlement.
  - Recover pending deliveries after restart and deduplicate their effects at the destination.
  - Reconcile uncertain dispatches through provider evidence before any new paid attempt.
  - Retain an unresolved state when the provider cannot establish the outcome.
  - Preserve financial evidence independently of tenant deletion, response expiry, and asset retention.
  - Define authorized deletion and retention procedures before production acceptance.
  - Expose account-scoped, cursor-paginated journal reads with safe request and reconciliation identifiers.
  Deliverables:
  - Durable journal tables, transaction boundaries, restart recovery, and accounting delivery integration.
  - Usage extraction in every supported provider adapter and operation.
  - Safe journal API, applicable client updates, and retention documentation.
  Validation:
  - Interrupt the real service before dispatch, after dispatch, after observation, and before settlement.
  - Prove each restart preserves known usage and exposes unresolved provider outcomes.
  - Prove concurrent duplicate requests cause one logical execution and one final accounting effect.
  - Exercise provider failure, timeout, client disconnect, continuation, and missing usage through controlled upstream protocols.
  - Prove unknown usage cannot become a zero-cost settled request.
  - Prove telemetry queue saturation cannot lose financial evidence.
  - Prove journal attribution and access remain isolated across accounts and tenants.
  - Run `make ci` after the last application change.

- [ ] [F067] (P1) {F066} Calculate exact provider costs and customer charges.
  Goal:
  Convert measured usage into reproducible provider costs and customer charges with the price selected at request acceptance.
  Evidence:
  The catalog now uses exact decimal strings for rates and minimum charges.
  Public HTTP tests retain large values and small fractions without floating-point conversion.
  The calculation service copies selected rates and applies the approved multiplier `13/10`.
  Public package tests cover cache inclusion, reasoning inclusion, minimum charges, unknown quantities, fractional remainders, and authorization bounds.
  Admission retains prices and bounds in the journal transaction. The attempt limit rejects excess work before dispatch.
  Journal delivery records charges in the settlement transaction. Unresolved charges do not call settlement.
  Account-owned HTTP resources expose charges and accepted prices under OpenAPI.
  HTTP tests cover restart recovery, immutable prices, duplicate delivery, rollback, ownership, pagination, and corrupt records.
  The catalog now has typed token ranges, cache classes, service tiers, regions, and effective intervals.
  The loader rejects overlapping known conditions. Admission rejects expired prices and unresolved token boundaries.
  Price snapshots retain all active input-token tiers. HTTP tests prove correct tier selection after restart and price expiry.
  Native text bindings cover Responses, Chat Completions, DashScope, xAI, and Anthropic cache lifetimes.
  Text admission constructs bounds from catalog token limits for each authorized attempt.
  HTTP tests prove that an exhausted attempt limit prevents dispatch and returns a request conflict.
  Compound quantity rules retain separate thinking tokens under the output rate without changing journal measurements.
  Gemini HTTP tests prove cache-storage exclusion and unresolved settlement for missing thoughts or unexpected tool input.
  HTTP tests verify dictation time rates, explicit speech provider-unit rates, and separate image modality charges.
  Image retries produce one charge. Missing measurements and usage above accepted bounds do not settle.
  Media admission constructs native quantity bounds from fixed catalog limits.
  HTTP tests reject missing, account-dependent, and incompatible bounds before provider dispatch.
  Catalog validation retains billing limits separately from capability control limits.
  Text admission uses the smallest fixed input or context limit for its input bound.
  The catalog ends the published Gemini Flash discounts at the local January 2027 eligibility boundary.
  Search admission includes call prices and input passes in its reservation.
  The transport sends the accepted tool limit on each generation, including continuations.
  HTTP tests verify search charges, missing evidence, exceeded bounds, and disabled-search exclusion.
  Separate customer-credit records preserve the original provider cost and exact charge.
  HTTP tests verify partial and full credits, duplicate events, restart recovery, account isolation, settlement rollback, and concurrent credit limits.
  The adjustment and Ledger callback share one transaction. Shared Ledger integration remains under F068.
  Request summaries add exact amounts across all attempts and customer credits without rounding.
  One database snapshot prevents mixed totals when another instance commits a credit during the read.
  HTTP tests cover multiple pages, pending delivery, unpublished results, unknown usage, failed work, and account ownership.
  Unresolved customer totals remain null. Known provider costs remain separate from customer credits.
  The browser reads itemized charges and request totals through the existing account-owned APIs.
  Browser checks passed for customer credits, exact fractions, large amounts, pending totals, pagination, failed reads, and phone widths.
  Complete provider price and limit qualification remains incomplete.
  Shared Ledger admission and settlement remain under F068.
  Component validation passes for hosted execution, exact rating, provider catalogs, Go lint, client contracts, and 39 browser tests.
  Final stack CI remains the F070 completion checkpoint. Production qualification and activation remain separate.
  Requirements:
  - Use F070 as the shared hosted service contract.
  - Extend `catalog_service.go` and the canonical provider catalog instead of copying provider rates into billing code.
  - Use one shared price condition model for F036 comparison and hosted rating.
  - Coordinate typed condition changes with F036 and preserve its separate public comparison scope.
  - Define exact component units, currencies, token ranges, cache classes, service tiers, regions, and effective intervals.
  - Reject overlapping conditions, missing components, incompatible units, and expired rate intervals during paid eligibility validation.
  - Preserve unavailable prices as unavailable and exclude their routes from hosted admission.
  - Import rate evidence from official provider sources with verification dates and explicit effective times.
  - Store immutable price snapshots for accepted requests independently of later catalog changes.
  - Keep historical snapshots as financial evidence under the current schema, without obsolete catalog readers or compatibility paths.
  - Record provider rates and customer rates separately under one accepted snapshot identifier.
  - Apply the F070 customer price schedule: provider price multiplied by `1.30`.
  - Require explicit approval of customer rates and fees before production activation.
  - Use exact decimal or rational arithmetic for rates and integer monetary units for ledger amounts.
  - Specify monetary precision, overflow bounds, rounding direction, and the single rounding boundary.
  - Preserve fractional remainders when aggregation requires finer precision than the ledger unit.
  - Define component inclusion rules for cache, reasoning, audio, images, tools, and minimum charges.
  - Calculate each billable attempt separately and aggregate attempts under the customer request.
  - Define customer treatment of continuations, failed work, cancellations, and provider charges without a successful result.
  - Preserve incurred provider cost when a customer adjustment removes the customer charge.
  - Return a typed unresolved result when quantities or required rate conditions remain unknown.
  - Produce a maximum authorized cost estimate before dispatch for F068 reservations.
  - Include output limits, continuation limits, and optional tool costs in that estimate.
  - Reject hosted work when its cost cannot be bounded under the selected policy.
  - Require a new authorized reservation before work exceeds the accepted maximum.
  - Expose account-scoped charge resources with quantities, rates, units, totals, and price snapshot identifiers.
  - Keep tax amounts, processor fees, provider costs, and customer charges separately attributable.
  Deliverables:
  - Exact rating service, immutable price snapshots, and maximum-cost calculation.
  - Charge records and itemized customer representations with safe evidence references.
  - Canonical catalog, OpenAPI, client, pricing, and operator documentation updates.
  Validation:
  - Verify calculations against hand-calculated fixtures for every supported provider billing component.
  - Prove cached and reasoning quantities follow provider inclusion rules without duplicate counting.
  - Prove repeated rating of one immutable observation produces the same charge.
  - Prove historical charges remain unchanged after catalog, margin, or currency configuration changes.
  - Exercise tier boundaries, minimum charges, fractional amounts, large values, and rounding through public APIs.
  - Prove unknown quantities and unmatched conditions prevent final settlement.
  - Prove every permitted execution path stays within its declared reservation bound.
  - Run `make ci` after the last application change.

- [ ] [F068] (P1) {F067} Enforce prepaid balances with atomic funds reservations.
  Goal:
  Bound hosted provider spending by each customer's available funds across concurrent requests and process restarts.
  Evidence:
  The operator selected the existing Ledger model and balance conservation for acceptance on 2026-09-23.
  The funds suite passes in 20.456 seconds after the decision update.
  Ledger PR 102 is merged. Release CI and publication passed for Ledger v1.1.0 on 2026-09-23.
  LLM Proxy uses the published public GORM adapter without a local module replacement.
  Journal admission, accepted prices, Ledger reservations, and settlement effects share the application transaction.
  Exact account remainders preserve fractional usage and credits. Uncertain work retains its funds reservation.
  Native HTTP, Go and Python clients, MCP, dictation, and media tests cover financial admission.
  Real service processes verify competing reservations and interruption before dispatch, after dispatch, and during settlement.
  The service reconciles funded usage and retained holds at startup and during operation.
  Account resources expose balances, reservations, Ledger entries, tenant limits, and audited financial decisions.
  The browser shows exact amounts, financial history, and tenant limits at desktop and narrow widths.
  Operator decisions preserve provider evidence and cannot exceed authorization, known pricing, or the remaining charge.
  Audited request credits and usage credits share one settlement limit, account lock, and Ledger credit implementation.
  Duplicate credits have one effect. A failed audit write rolls back the Ledger effect and exact remainder.
  Usage delivery retains known platform exposure and reconciliation cases. Incomplete provider costs remain explicit.
  Snapshot and restore tests preserve journal records, prices, charges, Ledger entries, holds, credits, decisions, and exposure.
  The funds suite passes in 22.330 seconds. Credit race checks pass in 36.013 seconds. Go lint and formatting pass.
  With the published dependency, hosted regression passes in 121.302 seconds and browser acceptance passes in 11.6 seconds.
  Go lint and formatting pass. Final stack CI remains the F070 completion gate.
  F069 verifies payment credits, reversals, and payment records in the restore fixture.
  F070 connects service admission and settlement and verifies browser funding errors.
  Final CI, commercial decisions, and complete F070 acceptance remain open.
  Requirements:
  - Use F070 as the shared hosted service contract.
  - Reuse the Ledger integration and domain code used by PoodleScanner and Hecate.
  - Resolve the transaction boundary, monetary precision, and uncertain holds before financial persistence implementation.
  - Keep one balance authority and extend the owning shared component for missing capabilities.
  - Use the existing Ledger account model and append-only credit ledger.
  - Verify balance conservation for each account and currency, including exact remainders and holds.
  - Use one billing account balance across all customer tenants, with optional lower tenant spending limits.
  - Define available funds as posted credits minus settled charges, active reservations, and payment reversal holds.
  - Store ledger amounts in the precision defined by F067.
  - Create reservation records with request, account, currency, maximum amount, state, and revision.
  - Define reservation states as `held`, `settled`, `released`, and `reconciliation_required`.
  - Commit request admission, funds reservation, and ledger effects in one database transaction.
  - Enforce sufficient funds with database constraints or conditional updates across all service instances.
  - Reserve the F067 maximum before each initial dispatch or authorized extension.
  - Return `402` with `insufficient_funds` before dispatch when available funds cannot cover the reservation.
  - Return `403` for suspended hosted authority and `503` when financial admission is unavailable.
  - Settle known charges and release unused funds in one transaction with a unique request settlement identifier.
  - Release reservations for requests proven never dispatched.
  - Keep reservations for uncertain dispatched work until reconciliation establishes the required financial disposition.
  - Use expiry to trigger recovery review, not automatic release of potentially incurred costs.
  - Require an explicit audited adjustment when policy resolves an outcome without complete provider evidence.
  - Stop further paid attempts when the reservation cannot cover additional work.
  - Record any provider cost above the authorized bound as platform exposure with a reconciliation case.
  - Apply admission through native HTTP, client protocols, MCP, dictation, and media execution.
  - Reuse the same accounting identity for retries, result reads, and media polling.
  - Keep status reads and existing result downloads outside generation reservations.
  - Accept payment credits and reversals only through verified, idempotent F069 ledger commands.
  - Preserve original entries and use compensating entries for refunds, disputes, and corrections.
  - Retain financial records when account access ends, under the approved retention policy.
  - Add balance, reservation, and ledger reads under the account management API.
  - Show available, reserved, spent, and pending amounts separately in the dashboard.
  Deliverables:
  - Credit ledger, funds reservations, atomic admission, settlement, and reconciliation procedures.
  - Stable funding errors and account balance views across supported client surfaces.
  - Financial backup, restore, and recovery verification through documented operator commands.
  Validation:
  - Submit concurrent requests whose combined maximum exceeds the funded balance through multiple real service processes.
  - Prove admitted reservations never exceed available funds and rejected requests cause zero provider work.
  - Prove balance conservation after settlement, refund, reversal, and recovery through the existing Ledger model.
  - Prove duplicate settlements, credits, and releases change balances once.
  - Interrupt execution across each transaction boundary and verify recovery against the real database.
  - Prove uncertain work retains its hold and known undispatched work releases its hold.
  - Prove database failure prevents new hosted dispatch.
  - Verify browser balances, tenant limits, and insufficient-funds messages through real public entry points.
  - Run `make ci` after the last application change.

- [ ] [F069] (P1) {F068} Add prepaid payments, receipts, and financial reconciliation.
  Status:
  - On 2026-09-22, the operator selected Paddle. The service must obey the applicable Paddle financial policies.
  - F070 requires a USD 5 funding minimum. Customers can spend their balance down to USD 0.
  - The current payment and refund contract is in `docs/hosted-billing.md`.
  Evidence:
  The HTTP inbox uses the published shared Paddle verifier from `utils/billing` v0.18.0, resolved through `@latest`.
  It verifies raw bytes before JSON parsing and acknowledges only a committed inbox record.
  Processor account, environment, and event identity control deduplication across service instances and restarts.
  Conflicting content is rejected. Signed events do not grant funds before financial verification.
  SQL diagnostics retain database failures without the private payment body.
  Inbox and restore acceptance passes in 3.657 seconds. Race checks pass in 15.969 seconds.
  Go lint, format checks, frontend lint, and generated API checks pass.
  Authenticated funding orders retain a server offer of at least 500 cents and its supplier, processor, and environment identities.
  Orders and checkout delivery intents commit together. Concurrent retries retain one order and preserve its original price.
  HTTP tests verify ownership, invalid financial inputs, pagination, disabled funding, rollback, and processor isolation.
  Payment and restore checks pass in 5.118 seconds. Payment race checks pass in 40.265 seconds.
  The funds regression passes in 21.591 seconds.
  The snapshot fixture preserves order and delivery records and verifies replay after restoration.
  Go lint, formatting, frontend lint, and generated API checks pass.
  Checkout delivery binds one processor transaction to the retained order through the released shared commerce client.
  A lost response or local write failure requires processor reconciliation without another transaction creation request.
  Completed events and current processor evidence must match before the service grants funds.
  The receipt, Ledger credit, order state, and event state commit together.
  Different events for one transaction and concurrent workers produce one credit.
  Receipt, order, and event write failures roll back the complete financial change.
  Payment and funds checks pass in 7.308 and 20.928 seconds against the released dependency.
  The complete backup fixture retains payment receipts and verifies replay without another credit after restoration.
  Receipt restoration passes in 5.012 seconds. Payment race checks pass in 103.389 seconds.
  Go and frontend lint pass.
  Utils PR 47 is merged and published as v0.19.0 with matching remote artifact hashes.
  The application uses the released shared adjustment client without a local dependency replacement.
  Current processor evidence controls pending holds, cumulative reversals, and restored funds.
  Adjustment revisions and Ledger effects commit together, including adjustments before initial funding.
  Derived payment restrictions reject new work after a deficit without changing an operator suspension.
  Controlled tests cover approval, rejection, chargeback replay, concurrent workers, restart, and failed financial writes.
  Payment and funds checks pass in 8.213 and 23.260 seconds against released utils v0.19.0.
  The complete backup test restores adjustment evidence and Ledger effects without another deduction after replay.
  The payment race check, Go lint, and format checks pass.
  Owned receipt resources expose original and adjusted customer amounts without private processor evidence.
  Temporary Paddle portal sessions use the owned processor customer and remain uncached.
  The normal CLI configures payment processing through its existing environment interpolation.
  The normal service runs checkout delivery and payment processing with HTTP admission.
  Financial write failure stops admission. Restart recovery applies the retained payment once.
  The database binds to one payment environment and preserves that binding through backup restoration.
  Payment, CLI, funds, restore, race, lint, and generated API checks pass against released utils v0.19.0.
  The browser shows funding history, verified receipts, and temporary Paddle invoice links.
  Signed payment and adjustment events drive the normal runtime, database, and rendered financial records in browser tests.
  The checks cover delayed funding, pending holds, partial refunds, pagination, invalid responses, and failed portal requests.
  Payment and onboarding browser checks pass in 16.5 seconds at desktop and narrow widths.
  Browser checkout uses the server transaction and an environment-specific public Paddle token.
  Lost responses retain one creation key through reload. Checkout can resume from browser state or retained funding history.
  Browser completion events cannot grant funds. Verified server state refreshes the balance and payment history.
  Payment, CLI, format, Go lint, frontend lint, and generated API checks pass.
  All three payment and onboarding browser checks pass in 23.5 seconds with controlled Paddle protocols.
  Verified cancellation changes an unpaid order to failed without a credit. Declined payment attempts remain retryable.
  Processor observations commit with order and event changes. Funding and adjustments use the same timestamp check.
  Older snapshots and conflicting evidence remain unresolved without a financial change.
  HTTP tests verify rollback, concurrency, restart, and restoration of retained observations.
  Payment, CLI, backup, race, browser, lint, format, and generated API checks pass.
  The operator command retains payment reconciliation runs, fixed order sets, processor evidence, and atomic checkpoints.
  Reports compare processor state, receipts, refund projections, and Ledger entries without monetary effects.
  Completed runs return their retained report. Unfinished runs resume the same order set.
  Public tests detect currency, discount, fee, receipt, Ledger, duplicate-credit, and pending-refund differences.
  Payment, CLI, backup, race, funds, browser, lint, and format checks pass.
  Provider cost imports bind original source bytes and normalized amounts to a credential version and UTC period.
  The comparison uses immutable ratings and separates incomplete usage, currencies, discounts, fees, and duplicate provider identifiers.
  Imported evidence and reports commit together without customer charge changes.
  Public tests verify exact cost comparison, source identity, scope filters, concurrent imports, rollback, replay, and unchanged customer charges.
  The normal CLI reads normalized evidence and the original source file. Backup restoration preserves both evidence and reports.
  Payment, CLI, backup, provider scope, race, lint, and format checks pass.
  Approved corrections reuse the F068 administrator-only financial resolution and request credit resources.
  Integrated HTTP acceptance combines Paddle funding, metered usage, a provider report, an approved customer credit, and a later refund.
  Customer approval fails. The operator identity and report reference remain in the audit record.
  Replay preserves one credit, original costs remain unchanged, and the final payment report confirms the expected Ledger effects.
  The combined hosted billing target passed hosted HTTP, exact rating, payments, official clients, and database restoration.
  All three authenticated browser scenarios passed in 32.3 seconds. The approved correction race and Go lint checks passed.
  The separate Paddle sandbox procedure specifies isolated configuration, required scenarios, and retained evidence.
  The separate native sandbox target verifies expected order and receipt amounts against the existing Paddle and Ledger reconciliation.
  It rejects production settings, protocol overrides, empty expectations, missing orders, and reused qualification reports.
  Controlled input checks and payment tests pass.
  B259 rejects recurring prices before checkout and preserves retries after temporary price failures through the existing shared client.
  Payment, checkout, browser, formatting, and lint checks pass after B259.
  The corrected stack CI run passed all Go tests, Python checks, and upstream race checks.
  Its coverage gate failed with `coverage total 95.3%, want 100.0%`. B266 owns that failure.
  B265 now rejects Responses image requests whose accepted snapshot cannot bound both paid components.
  The related HTTP regression and Go lint passed. F087 owns actual sandbox evidence. Complete F070 development acceptance remains open.
  `make test-hosted-billing` passed after B265, including clients, backup restoration, and four browser scenarios in 37.1 seconds.
  Goal:
  Convert verified customer payments into account funds and explain differences between local records and external financial evidence.
  Requirements:
  - Use F070 as the shared hosted service contract.
  - Use Paddle Checkout with one-time purchases for prepaid funding.
  - Reuse `github.com/tyemirov/utils/billing` and the direct Paddle integration in PoodleScanner.
  - Reuse Hecate event persistence and idempotent grant patterns without its native-store or RevenueCat purchase flow.
  - Permit prepaid funding without an active subscription.
  - Use Paddle as merchant of record and retain the platform supplier, Paddle account, and environment identities.
  - Require a minimum funding amount of USD 5 without an unspent balance floor.
  - Obey the Paddle Buyer Terms and Refund Policy applicable to each purchase.
  - Keep the credit ledger authoritative for immediate admission and customer usage deductions.
  - Use payment processing for account funding, receipts, refunds, and payment reversals.
  - Keep metered invoices outside the initial prepaid contract to prevent duplicate collection for consumed credits.
  - Map one processor customer to each billing account within the selected processor environment.
  - Add account-owned funding orders and checkout sessions through authenticated resource creation.
  - Require idempotency keys and derive amounts, currency, account identity, and return URLs on the server.
  - Define funding states as `created`, `pending`, `paid`, `failed`, `partially_refunded`, `refunded`, and `disputed`.
  - Keep payment receipts separate from credit ledger entries and link both through a unique funding order.
  - Verify webhook signatures against raw request bytes and validate the processor environment.
  - Verify the `Paddle-Signature` header before JSON parsing.
  - Persist verified events in a payment inbox before acknowledging durable acceptance.
  - Deduplicate processor event identifiers and financial effects by payment object and effect type.
  - Handle duplicate and out-of-order events with verified payment state and explicit transition rules.
  - Credit funds only after verified payment success with matching account, amount, and currency.
  - Use verified `transaction.completed` evidence for the funding credit.
  - Keep browser return pages informational until the server confirms the payment state.
  - Use a delivery outbox for retryable external requests with stable idempotency identifiers.
  - Retry transient transport failures while retaining uncertain outcomes for provider reconciliation.
  - Apply partial refunds, disputes, and chargebacks through compensating ledger entries and explicit account holds.
  - Process Paddle adjustments with separate pending approval, approved, rejected, and reversed states.
  - Deduplicate chargeback warnings, chargebacks, and reversals without duplicate deductions.
  - Prevent new spending from funds allocated to a pending refund or reversal.
  - Define account suspension and deficit treatment when a reversal exceeds remaining funds.
  - Keep gross payment, tax, processor fee, net settlement, customer credit, and refund amounts separate.
  - Add itemized funding history, usage charges, receipt links, and pending payment states to the dashboard.
  - Add an idempotent reconciliation command with a durable run identifier, checkpoint, and result report.
  - Compare payment receipts with processor records and customer credits with ledger entries.
  - Compare provider costs with provider usage exports or invoices where those sources are available.
  - Separate timing, currency, discount, fee, missing usage, and duplicate-effect differences in reconciliation cases.
  - Require explicit approval and an audit record for corrective monetary entries.
  - Align tax, receipt, retention, refund, and dispute behavior with the verified Paddle contract.
  - Record platform retention periods and dispute holds before production activation.
  - Keep sandbox and production identifiers, secrets, and records isolated.
  Deliverables:
  - Checkout integration, verified webhook endpoint, payment inbox, delivery outbox, and receipt resources.
  - Funding and refund state machines, account holds, and browser payment history.
  - Reconciliation command, scheduled execution contract, difference reports, and operator recovery procedures.
  - OpenAPI, applicable client, payment setup, and operational documentation updates.
  Validation:
  - Exercise the real service with controlled processor protocols for each financial transition and failure.
  - Track actual checkout, webhook, delayed payment, and refund qualification in F087 without blocking development completion.
  - Prove duplicate events and different events for one payment create one customer credit.
  - Prove invalid signatures, mismatched amounts, and another account's payment cannot create funds.
  - Prove a browser success URL cannot create funds without verified payment evidence.
  - Exercise crashes before and after inbox persistence, ledger posting, and external response receipt.
  - Prove partial refunds, disputes, delayed success, and event reordering preserve ledger invariants.
  - Prove reconciliation reruns preserve existing effects and expose seeded discrepancies.
  - Verify the complete funding and receipt flow through the real browser.
  - Run `make ci` after the last application change and record sandbox acceptance separately.

- [ ] [F070] (P1) {F065,F066,F067,F068,F069} Deliver the unified prepaid hosted service.
  Status:
  - On 2026-09-22, the operator confirmed a 30% markup: `customer_price = provider_price * 1.30`.
  - The operator selected all providers and their supported operations under one shared billing contract.
  - The minimum funding amount is USD 5. Customers can spend their balance down to USD 0.
  - Paddle supplies payments and merchant-of-record services. The service must obey the applicable Paddle financial policies.
  - Reuse PoodleScanner and Hecate integrations and shared components. LLM Proxy has no native mobile application.
  - Shared decisions and current integration boundaries are in `docs/hosted-billing.md`.
  - Normal server construction now connects hosted completion and media admission to the existing price, journal, and Ledger components.
  - `make test-hosted-runtime` passed CLI configuration checks and funded text and image execution through the normal HTTP listener.
  - These tests verify automatic settlement, exact remainders, idempotent replay, and zero provider calls after financial rejection.
  - After runtime integration, `make test-hosted-billing` passed the billing, client, backup, and three browser tests. Go lint also passed.
  - The usage journal now shows request totals, itemized charges, and customer credits through the existing APIs.
  - Frontend lint and browser checks passed for exact amounts, pending and unresolved states, pagination, invalid responses, and narrow layouts.
  - The funded browser flow passed model selection, USD 5 funding, exact charges, replay, receipts, isolation, and grant suspension.
  - Four hosted browser tests passed at desktop and narrow widths with controlled Paddle and provider responses.
  - The operational command reads queue ages, comparison differences, and exact financial totals without financial writes or external calls.
  - `make test-hosted-signals` passed CLI and financial snapshot checks. Go lint passed.
  - B262 added normal HTTP acceptance for dictionary service charges, exact balance exhaustion, replay, and receipt recovery.
  - The text acceptance matrix passed every enabled catalog offering through all four public HTTP interfaces.
  - It verified platform authority, native model selection, exact costs and charges, automatic settlement, and replay across interfaces.
  - The same runtime fixture now verifies current image and audio inputs through `/v2`, including exact charges and media identity.
  - Image acceptance passed for 37 offerings across seven providers. Audio and mixed inputs each passed for eight offerings across two providers.
  - Changed bytes, removed media, and reversed attachments return HTTP 409 without additional provider work. Media input race checks passed.
  - `make test-hosted-runtime` and Go lint passed after the matrix change.
  - Speech financial tests passed each current ElevenLabs generation and conversion offering, including timestamped speech.
  - The tests check exact charges, account remainders, reservations, replay, and zero provider calls after unfunded rejection.
  - Missing usage, provider errors, and uncertain results keep funds for reconciliation without an automatic debit.
  - Related usage, rating, settlement, and exposure checks passed. Go lint passed after the test changes.
  - Dictation financial acceptance passed every enabled offering through both public HTTP interfaces.
  - It checks native model selection, duration and token rates, exact settlement, account remainders, and replay across interfaces.
  - Missing usage keeps the funds reservation and has no customer charge. Related dictation tests and Go lint passed.
  - Image financial checks passed generation, editing, JSON, stream events, exact charges, and account remainders.
  - FAL queue checks passed settlement, unresolved outcomes, excess cost, and recovery without duplicate provider work.
  - Funded queue recovery preserves one settlement after worker replacement and grant revocation.
  - Responses images keep funds for reconciliation because the image-tool meter remains unqualified.
  - Dictator synthesis tests passed real funds admission, exact duration charges, and Ledger settlement for both current synthesis models.
  - Cancellation, invalid durations, artifact loss, and usage write errors keep funds for reconciliation.
  - Related Dictator and speech regression checks passed. Go lint passed.
  - Actual Paddle sandbox inputs are absent from the process environment and all six repository private environment files.
  - Read-only sandbox requests succeeded with the existing PoodleScanner local API key. No active LLM Proxy product or price exists there.
  - F087 owns sandbox product, price, webhook, account identity, and actual qualification work without blocking development completion.
  - Complete provider-operation qualification and final CI remain open. F069 has ready PR 344.
  - A 2026-09-24 source review confirmed missing image-tool usage, alignment billed duration, and Dictator processed input duration.
  - The current dependency evidence is in the hosted billing runbook. These provider measurements remain in F070 scope.
  - The remaining commercial decisions and F065 through F069 implementation remain open.
  - The corrected stack CI run passed all Go tests, Python checks, and upstream race checks.
  - B266 records the remaining gate failure: `coverage total 95.3%, want 100.0%`.
  - B265 rejects an incomplete Responses image cost bound before dispatch. Complete Responses billing remains required.
  - `make test-hosted-billing` passed after B265, including clients, backup restoration, and four browser scenarios in 37.1 seconds.
  Goal:
  Give a customer one account, one funded balance, and immediate access to approved services without provider account setup.
  Govern implementation and development acceptance across the five connected billing capabilities.
  Requirements:
  - Implement F065, F066, F067, F068, and F069 sequentially in dependency order.
  - Use this umbrella for shared decisions, interface ownership, and complete service acceptance.
  - Keep implementation details and component acceptance in the owning child issue.
  - Use a prepaid USD account balance with a USD 5 funding minimum and no unspent balance floor.
  - Cover all providers and their supported operations through one shared financial contract.
  - Calculate customer rates from provider rates with a 30% markup.
  - Record scope decisions before implementation depends on them.
  - Record every provider, supported operation, customer rate, fee, funding minimum, and maximum account exposure.
  - Use Paddle for payments and obey its applicable financial and refund policies.
  - Record the platform supplier, Paddle account, failure-charge policy, and financial retention policy.
  - Confirm provider commercial authorization and paid capacity before hosted activation for each offering.
  - Keep unresolved commercial choices visible without inventing rates, tax rules, or provider rights.
  - Use F065 for billing identity, platform credentials, grants, and customer onboarding.
  - Use F066 for request identities, billable attempts, usage evidence, and restart recovery.
  - Use F067 for immutable price snapshots, provider costs, customer charges, and maximum authorized costs.
  - Use F068 for funds reservations, ledger entries, admission, settlement, and financial account holds.
  - Use F069 for payment receipts, processor events, funding credits, reversals, and external reconciliation.
  - Reuse the current managed database, authentication boundary, provider catalog, and execution coordinators.
  - Reuse existing Paddle and Ledger code and approaches from PoodleScanner and Hecate.
  - Use shared component contracts for payment transport, signature verification, and financial operations.
  - Resolve shared component gaps in the owning package instead of creating another billing implementation.
  - Deliver browser flows without native mobile applications or native-store purchase integrations.
  - Add current schema resources through explicit migrations only when an actual stored-data inventory requires a transfer.
  - Keep billing schema extensions separate from historical telemetry conversion.
  - Start customer financial history from verified funding and journal records, not reconstructed telemetry totals.
  - Keep current customer-owned provider access as an explicit service option.
  - Keep each hosted provider assignment explicit and prevent automatic credential substitution.
  - Make sure every public route that can incur provider costs uses the same financial admission contract.
  - Include continuations, tool execution, disconnected clients, and durable media workers in the cost boundary.
  - Activate each hosted offering only after metering, bounded pricing, funding enforcement, and provider qualification pass.
  - Preserve uncertain financial outcomes until an audited reconciliation decision resolves them.
  - Give customers readable amounts, itemized charges, funding history, and actionable funding errors.
  - Coordinate catalog condition ownership with F036 and service commitment decisions with P006.
  - Keep new provider implementations with their existing capability issues.
  - Document the hosted architecture, state transitions, failure recovery, and component ownership in one current source document.
  - Add repository-native targets for component integration, complete billing acceptance, and processor sandbox qualification.
  - Require database backup and restore evidence for journal, ledger, reservations, and payment records together.
  - Add operational signals for unresolved attempts, settlement delay, reconciliation differences, and funded exposure.
  - Keep development completion separate from deployment, real payments, and provider invoice acceptance.
  - Obtain explicit operator authorization for production activation and any live financial qualification.
  - File reproducible defects discovered during acceptance as separate BugFix issues.
  Deliverables:
  - Completed F065 through F069 with linked validation evidence and resolved shared product decisions.
  - Hosted service architecture document, OpenAPI resources, customer onboarding, and billing dashboard.
  - Controlled end-to-end acceptance suite and a separate F087 record for actual processor sandbox qualification.
  - Operator launch checklist with provider qualification, rate evidence, recovery evidence, and remaining activation decisions.
  Validation:
  - Create a fresh account, complete funding with controlled processor responses, and use platform credentials through the customer interface.
  - Verify the exact customer charge, provider cost, funds release, and remaining balance for every supported provider operation.
  - Exhaust the available balance and prove subsequent rejected requests cause zero upstream work.
  - Exercise concurrent requests across tenants, idempotent retries, failed providers, client disconnects, and restarts.
  - Exercise payment duplication, delayed confirmation, refund, reversal, and uncertain provider outcomes.
  - Restore a consistent backup and prove retained financial records explain all accepted requests and payments.
  - Verify one customer's resources and financial evidence remain inaccessible to another customer.
  - Verify the rendered login, onboarding, model selection, funding, usage, and receipt flows on desktop and mobile widths.
  - Compare controlled payment receipts, usage journal entries, charge calculations, and ledger totals with expected fixtures.
  - Run `make ci` after the last application change and record the complete acceptance target result.
  - Mark this issue complete only after its development deliverables and acceptance requirements pass.

- [ ] [F064] (P1) Add Alibaba Model Studio US East access and its complete model inventory.
  Goal:
  Let users connect an Alibaba Cloud Model Studio workspace in US (Virginia), `us-east-1`.
  Make all models available through that workspace part of the integration scope, including third-party models.
  Keep access region and service deployment scope distinct.
  This issue records future implementation work. The 2026-09-13 investigation changes documentation only.
  Requirements:
  - Extend the current `dashscope` provider with US East workspace support.
  - Use the Alibaba Cloud labels and English credential setup link defined by I262.
  - Keep `configs/providers.yml` as the only runtime provider catalog.
  - Keep the account-owned provider connection and explicit tenant assignment from F063.
  - Use a pay-as-you-go API key from the same region and workspace as the API host.
  - Use `https://{WorkspaceId}.us-east-1.maas.aliyuncs.com/compatible-mode/v1` as the US East base URL.
  - Keep Singapore and US East available as explicit current regions.
  - Keep each upstream model selector exact, including each documented `-us` suffix.
  - Show whether each US East offering uses Global or US service deployment scope.
  - Treat US East access as distinct from a guarantee of inference in Virginia.
  - Preserve model publishers such as Alibaba, DeepSeek, Moonshot, and Z.ai when Alibaba supplies the provider offering.
  - Keep each unqualified offering disabled until its required live checks pass.
  - Account for every inventory model with a supported route or an explicit implementation gap.
  - File separate Feature issues for public operations or protocol adapters that the current product does not support.
  Evidence checked on 2026-09-13:
  - Alibaba documents US East workspace endpoints for both Responses and Chat Completions.
  - The regions page distinguishes US East access from Global and US service deployment scopes.
  - The documented `qwen-plus-us` selector restricts inference to the US.
  - The Base URL page still lists `dashscope-us.aliyuncs.com`, but the newer regions page marks shared US access unsupported.
  - Both pages document the workspace endpoint selected above.
  - The US pricing tables include `qwen-plus-us`, `qwen3.6-flash-us`, `qwen-flash-us`, `deepseek-v4-pro-us`, and `deepseek-v4-flash-us`.
  - The same tables include Global offerings such as `qwen3.7-plus`, `deepseek-v4-pro`, `deepseek-v4-flash`, and `kimi-k3`.
  - These examples establish regional model availability in official documentation. They are not the complete workspace inventory.
  - No authenticated inventory request or live generation request ran during this investigation.
  Inventory procedure:
  - Call `GET https://{WorkspaceId}.us-east-1.maas.aliyuncs.com/api/v1/models` with `Authorization: Bearer {API_KEY}`.
  - Start with `page_no=1` and `page_size=20`.
  - Retrieve all pages through `output.total` without publisher, name, capability, or deployment-scope filters.
  - Read `output.models` and examine `success`, page counts, duplicate identifiers, and incomplete results.
  - Run separate queries with `service_site=global` and `service_site=united-states` to establish scope membership.
  - Record `provider`, `inference_provider`, modalities, features, token limits, prices, and `equivalent_snapshot` when present.
  - Separate models available for inference from models that require a separate deployment or external provider connection.
  - Preserve the response date, request ID, source host, and total count as inventory evidence.
  - Keep credentials out of inventory files and logs.
  - Compare the inventory with the US East console model list and each selected model's API reference.
  Current implementation and gaps:
  - `configs/providers.yml` has one `dashscope` provider, four active Qwen offerings, and five disabled Qwen 3.8 candidates.
  - Its `base_url` pattern permits only `ap-southeast-1` workspace hosts.
  - `internal/proxy/account_connections_store.go` validates current connection fields through `validatedProviderFieldValue` and the catalog pattern.
  - Its current connection path does not require a new provider-specific authentication implementation.
  - `internal/proxy/management_store.go` also contains `managedProviderBaseURL` and `dashScopeWorkspaceHostSuffix` with a Singapore-only rule.
  - That rule applies to older persisted records. Examine its migration callers before deciding whether US East needs a code change there.
  - `internal/proxy/dashscope_responses.go` receives the resolved endpoint and upstream selector without a region check.
  - The existing text transport appends `/responses`, uses bearer authentication, and selects `dashscope_responses` with `synchronous_completion`.
  - Alibaba documents this protocol in US East. Basic text connectivity can reuse the existing adapter, subject to live qualification.
  - The codec supports text, image input, token limits, reasoning effort, usage, and terminal output handling.
  - Its output parser rejects unsupported output items. An OpenAI-compatible label does not establish support for every model capability.
  - The catalog schema and route selection have no access-region or service-deployment-scope restriction for an offering.
  - A wider URL pattern alone would expose one combined offering list to both regions.
  - `internal/proxy/provider_catalog_schema.go`, `internal/proxy/model_catalog.go`, and `internal/proxy/catalog_service.go` need a regional contract review.
  - No Alibaba model inventory importer or `/api/v1/models` upstream request exists in the inspected application and scripts.
  - Current discovery reads the validated YAML snapshot. It does not retrieve new models when a connection is saved.
  - Image generation, video, speech, embeddings, rerank, and realtime models require a per-operation adapter and public-contract assessment.
  - F022 supplies the durable media foundation. F024, F025, F026, and I046 contain related capability work.
  Deliverables:
  - Add a dated complete US East model inventory with exact selectors and scope membership.
  - Add a repeatable model inventory importer through a documented Makefile target.
  - Make the importer produce a proposed catalog change for review before runtime activation.
  - Add typed regional restrictions to catalog validation, connection selection, route admission, and applicable discovery responses.
  - Use the selected connection's region to prevent a route that is unavailable in that region.
  - Import supported offerings, limits, controls, prices, currencies, and price conditions from the regional sources.
  - Record unknown metadata explicitly where the current schema permits it.
  - Give every remaining model an exact missing adapter, public operation, qualification gate, or external prerequisite.
  - Add each required dependency issue before the affected model enters implementation.
  - Update `docs/provider-catalog.md`, `docs/dashscope-responses.md`, and the relevant README connection and live-test instructions.
  - Record the US East extension of P007's Singapore-only endpoint decision in current product documentation.
  - Keep I038, I226, and F050 qualification evidence separate from US East acceptance.
  Open Decisions:
  - The requested scope includes models available through US East, with both Global and US inference identified.
  - A US-only default for tenant selection requires a product decision. Do not infer it from the access-region name.
  - Automatic runtime inventory refresh is not specified. The initial proposal uses an operator importer and reviewed YAML changes.
  Validation:
  - First add public integration scenarios that fail for US East connection creation and unavailable regional model selection.
  - Examine connection creation, credential verification, tenant assignment, saved defaults, reload, and browser model selection.
  - Examine Singapore and US East connections together with distinct credentials and exact model selectors.
  - Examine malformed hosts, region mismatches, missing models, incomplete inventory pages, and safe provider errors.
  - Examine inventory completeness against the recorded upstream total and explicit disposition of every unique model.
  - Examine text, image input, usage, output limits, and reasoning controls for each advertised combination.
  - Run `make test-account-connections`, `make test-provider-catalog`, and `make test-dashscope-responses` for their affected contracts.
  - Run `make test-dashscope-media-limits` when image offerings change.
  - Add a focused Makefile target for inventory and regional route scenarios.
  - Run `make ci` after the last application change, as required by the repository policy.
  - Run the exact US East model matrix with the regional key and base URL through the live-provider harness.
  - Extend the live harness when its current inputs cannot select the complete regional matrix.
  - Record each model, scope, operation, response status, request ID, and result without credentials or private content.
  - Report repository validation, live-provider qualification, release, and production acceptance as separate gates.
  Sources:
  - [Regions and endpoints](https://www.alibabacloud.com/help/en/model-studio/regions).
  - [Base URL overview](https://www.alibabacloud.com/help/en/model-studio/base-url).
  - [List models API](https://www.alibabacloud.com/help/en/model-studio/list-models).
  - [Responses API](https://www.alibabacloud.com/help/en/model-studio/qwen-api-via-openai-responses).
  - [DeepSeek API and US East Chat Completions](https://help.aliyun.com/en/model-studio/deepseek-api).
  - [Regional model pricing](https://help.aliyun.com/en/model-studio/model-pricing).

- [!] [F061] (P1) Add GPT-6 Astra through the existing OpenAI provider.
  Goal:
  Add the requested exact `gpt-6-astra` model to current public text interfaces.
  Requirements:
  - Use the existing Responses transport and retain the current provider default.
  - Support the five documented reasoning levels from `low` through `max`.
  - Preserve image input, structured output, caller tools, web search, and current usage accounting.
  - Record verified context, input, output, media limits, and standard prices for both context tiers.
  - Reject unsupported effort and excessive requested output before provider dispatch.
  - Qualify the candidate with the existing authorized OpenAI key before activation.
  - Update management, discovery, clients, documentation, and browser acceptance together.
  Validation:
  - Start with failing public HTTP tests and cover the current Responses lifecycle.
  - Run live text, reasoning, image, structured-output, and caller-tool checks.
  - Run final CI after the last application change.
  Evidence:
  On 2026-09-07, the existing key returned HTTP 200 for exact model discovery, with `created=1787853604`.
  The catalog now enables Astra with five reasoning levels, image input, tools, web search, limits, and both standard price tiers.
  Public HTTP tests and both focused browser tests pass.
  Live checks passed for omitted effort, all five explicit efforts, image input, structured output, caller functions, and web search.
  Blocked:
  Final CI stopped at `Go files require formatting` for files in the active F060 Vertex change.
  Complete F060 formatting and rerun `make ci` before resolving F061.
  Production deployment belongs to the operator.
  Sources:
  - https://developers.openai.com/api/docs/models/gpt-6-astra
  - https://developers.openai.com/api/docs/pricing
- [!] [F060] (P1) {P012} Connect Gemini completion routes through Vertex API keys.
  Goal:
  Provide Vertex Gemini completion through the existing tenant API-key connection contract.
  Requirements:
  - Use the tested global `v1 generateContent` endpoint and exact qualified model IDs.
  - Accept one secret `api_key` field and send it through `x-goog-api-key`.
  - Use the key-only model path without a project or location segment.
  - Store each customer key in its encrypted tenant connection record.
  - Reject obsolete credential-profile fields and configuration declarations.
  - Keep inline requests independent of the separate F043 media staging work.
  - Map text, structured output, reasoning, media, finish reasons, and usage through the native Vertex contract.
  - Preserve public deadlines, output-limit errors, media order, and tenant isolation.
  - Qualify each affected offering, including both public dictation protocols where applicable.
  - Complete customer setup and public proxy acceptance before customer activation.
  Deliverables:
  API-key transport, catalog mappings, explicit operator-profile transition, public integration tests, and customer documentation.
  Validation:
  - Start with failing public HTTP tests for API-key connections and the new endpoint.
  - Verify key replacement, disconnect, tenant isolation, provider errors, cancellation, structured output, media, and exact usage.
  - Run Flash 3.8 proxy acceptance at concurrency 10 and complete final CI.
  - Record production acceptance after the separately approved cutover.
  Approval (2026-09-08):
  The user approved replacement of the operator credential-profile input with the customer API-key contract.
  This approval supersedes the initial OAuth requirements. Production deployment remains a separate action.
  Source:
  `docs/vertex-gemini-qualification.md` records 22 successful direct Vertex requests on September 7, 2026.
  Implementation (2026-09-07):
  - Added the native Vertex transport and tenant-bound Google OAuth profiles.
  - Added an explicit provider and offering activation contract.
  - Enabled four qualified Vertex text models and `gemini-3.5-transcribe-preview` for dictation.
  - Kept the unqualified Developer API offerings for Flash 3.8 and Flash-Lite disabled.
  - Created the dedicated service account in `llm-proxy-499919` and granted `roles/aiplatform.user`.
  - All 28 live proxy requests passed with that service account.
  - Public HTTP tests verify token refresh, cancellation, management verification, tenant isolation, media, structured output, usage, and provider failures.
  - Browser rendering accepts the Google credential profile contract.
  - Added `docs/vertex-gemini.md` with the explicit tenant cutover procedure.
  - Evidence: `docs/evidence/vertex-proxy-2026-09-07.json`.
  Validation:
  The first CI run stopped because the generated OpenAPI page was stale.
  The page was regenerated. The next CI run passed Go coverage at 100% and found five stale browser assertions.
  The corrected browser cases and candidate-harness test pass.
  Final `make ci` passed all 12 gates in 252 seconds with 100.0% Go statement coverage and 99 frontend browser tests.
  Evidence: `/tmp/llm-proxy-f060-ci-complete.log`.
  Blocked:
  P012 requires independent customer setup through the accepted API-key flow.
  The implemented operator credential profiles do not satisfy that requirement.
  The API-key revision has explicit user approval. Complete its proxy acceptance and the remaining P012 customer gates.
  The earlier operator procedure remains historical evidence.
  Exact Vertex prices remain unavailable until their import.
  API-key implementation (2026-09-08):
  - Replaced the operator profile with the tenant `api_key` field and `VERTEX_API_KEY` local binding.
  - Reused catalog authentication to send `x-goog-api-key` to the key-only model endpoint.
  - Removed the Google profile loader, OAuth dependency, and public profile credential kind.
  - Public HTTP tests cover valid replacement, rejected replacement, tenant isolation, disconnect, and obsolete-field rejection.
  - The initial API-key test failed with HTTP 400 before an upstream request. The corrected focused tests passed.
  - Flash 3.8 passed 70/70 public requests at concurrency 10 and all 15 connection and tenant checks.
  - Public p95 latency was 5.451 seconds. The maximum was 6.486 seconds with a 45-second deadline.
  - All 28 additional live offering checks passed, including both dictation endpoints.
  - The first live trial failed ten structured requests locally. The corrected trial used its own temporary asset store.
  - `docs/evidence/vertex-api-key-acceptance-2026-09-08-summary.json` retains the acceptance results and remaining gates.
  - Local `configs/.env` contains the approved Vertex key. Production configuration and catalog activation remain unchanged.
  - The OpenAPI artifact, browser validator, catalog guide, and Vertex runbook describe the current API-key contract.
  - Final `make ci` passed all 12 gates in 284 seconds with 100.0 percent Go statement coverage.
  - Governor and changed-prose checks passed. The tracker retains 79 language findings outside this change.
  - Changed contracts: Vertex uses `api_key`, `VERTEX_API_KEY`, and header authentication. The public profile credential kind was removed.
  - Changed files: the Vertex transport and tests, catalog and configuration validators, OpenAPI artifact, browser validator, dependencies, and runbooks.
  - Independent customer setup, live provider-key replacement, Google revocation, and production acceptance remain open.
  Reconciliation (2026-09-07):
  `docs/gemini-customer-connections.md` records the proposed F060 revision and customer acceptance gates.
  Retain the 28 successful requests as transport evidence only.
  Initial changed contracts (2026-09-07):
  The public credential kind adds `google_credential_profile`.
  The provider catalog adds `vertex_generate_content`, `google_credentials`, and provider/offerings `enabled` flags.
  Existing public request paths and event schemas are unchanged.
  Initial changed files (2026-09-07):
  - `configs/providers.yml`, `cmd/cli/config_file.go`, `go.mod`, and `go.sum`.
  - The Vertex codec, Google profiles, routing, catalog, management verification, media, and schema files in `internal/proxy/`.
  - `pkg/llmproxyclient/capabilities.go`, `scripts/render_public_site.mjs`, and `scripts/test_live_providers.sh`.
  - The related Go fixtures, public HTTP tests, CLI tests, operational tests, and browser tests.
  - `docs/vertex-gemini.md`, Vertex evidence, the provider guide, Gemini guides, OpenAPI, generated API page, README, and terminology.
- [!] [F054] (P1) Add Meta file dictation through the current public endpoints.
  Goal:
  Implement the file dictation part of P009, approved on 2026-09-06.
  Requirements:
  - Add `muse-voice-transcribe-1.0` with a native Meta ASR codec and synchronous dictation transport.
  - Send JSON settings in the multipart `request` part and WAV bytes in the `audio` part.
  - Use `PUSH_TO_TALK`, bearer authentication, and buffered JSON output.
  - Validate RIFF/WAVE structure, mono 16-bit PCM, 16 kHz or 24 kHz, and the ten-minute duration bound.
  - Bound the complete multipart request and preserve the smaller configured public upload limit.
  - Return the complete transcript through both current dictation endpoints.
  - Keep provider identifiers and error bodies private.
  - Preserve tenant defaults, management selection, client discovery, and usage classification.
  - Register a disabled candidate until provider access, retention evidence, and paid acceptance pass.
  - Record the published audio price and the exact request-bound evidence.
  Deliverables:
  Native codec, candidate catalog, public integration tests, and operator documentation.
  Validation:
  - Start with failing HTTP tests for both public dictation endpoints.
  - Prove valid audio, malformed audio, duration bounds, provider errors, cancellation, and tenant selection.
  - Run final CI and paid acceptance at both sample rates.
  - Resolve the file content-retention policy and exact 32 MB interpretation before activation.
  Source:
  `docs/meta-media-assessment.md`.
  Development (2026-09-06):
  The native `meta_transcription` codec and disabled candidate are implemented.
  Both public endpoints pass at 16 kHz and 24 kHz with complete transcript output.
  Public tests cover WAV validation, duration, request bounds, provider errors, cancellation, tenant defaults, usage, and client discovery.
  `make test-meta-transcription` and `make test-provider-catalog` pass.
  Final CI passed all 12 gates in 249 seconds with 100% Go coverage.
  Blocked:
  The existing key is available as `MUSE_API_KEY` in `configs/.env`.
  File retention evidence, the exact provider request bound, and paid acceptance at both sample rates remain required before activation.
- [ ] [F055] (P1) {F054} Add Meta file transcription controls and speaker results.
  Goal:
  Complete the structured file transcription scope approved under P009 on 2026-09-06.
  Requirements:
  - Expose `PUSH_TO_TALK`, `ENDPOINTING`, and `DIARIZATION` through typed media operation inputs.
  - Support keyword and language bias with provider-verified limits.
  - Preserve complete transcripts, turn identifiers, timestamps, and session-local speaker labels in typed results.
  - Keep text-only dictation as the existing F054 projection.
  - Support the documented file event stream through canonical operation events and final results.
  - Define partial replacement, delta assembly, terminal failure, and disconnect behavior explicitly.
  - Reuse tenant assets, retained results, capacity, cancellation observations, and usage from F022 and I046.
  - Update OpenAPI and the official client with the server contract.
  Validation:
  - Prove every mode and control through public HTTP and official client integration tests.
  - Prove ordered turns, overlap, speaker changes, partial revisions, tenant isolation, and restart behavior.
  - Qualify real speech for all declared modes before activation.
  Source:
  `docs/meta-media-assessment.md`.
- [ ] [F056] (P1) {I046} Add tenant-owned Meta realtime transcription sessions.
  Goal:
  Implement the full realtime transcription scope approved under P009 on 2026-09-06.
  Requirements:
  - Define authenticated session resources and a typed audio and event transport under the current gateway namespace.
  - Keep the provider key in the backend handshake and the native session identifier private.
  - Support the three transcription modes, keyword bias, language bias, partial modes, and audio progress.
  - Validate mono 16-bit PCM at 16 kHz or 24 kHz and enforce documented pacing and session limits.
  - Map final transcripts, turn boundaries, speaker labels, errors, and close codes to one canonical event schema.
  - Define explicit end-of-input, cancellation, disconnect, and session-expiry behavior.
  - Keep new connections distinct because Meta provides no resume token.
  - Enforce tenant fairness and shared provider-account concurrency and session-start limits.
  - Define content retention, metadata-only provider logging, and processed-audio usage before activation.
  - Update the server, OpenAPI, official client, and session example together.
  Validation:
  - Start with public session tests against a controlled WebSocket provider.
  - Prove handshake timeout, pacing errors, partial revisions, overlapping turns, cancellation, and resource cleanup.
  - Qualify both sample rates, all modes, terminal events, and provider usage with real audio.
  Source:
  `docs/meta-media-assessment.md`.
- [ ] [F057] (P1) {I046} Add Meta image generation and edit operations.
  Goal:
  Implement the full image endpoint scope approved under P009 on 2026-09-06.
  Requirements:
  - Add `muse-image-1.0` generation and edits through durable tenant-owned media operations.
  - Preserve ordered input assets and implement native JSON and required multipart request forms.
  - Expose verified output count, aspect ratio, file format, reasoning, moderation, and internal tool controls.
  - Support completed image events with truthful completion and cancellation observations.
  - Transfer provider output into authenticated tenant assets before operation success.
  - Keep provider URLs and file identifiers private and implement required staging cleanup.
  - Reconcile uncertain submissions through F022 evidence without automatic duplicate generation.
  - Verify endpoint-specific input bounds, output URL lifetime, retention, and safety error behavior.
  - Record per-image cost and shared account limits separately from informational token counts.
  - Update the server, OpenAPI, official client, discovery, and examples together.
  Validation:
  - Prove generation, edits, controls, ordered inputs, output integrity, isolation, and uncertain dispatch through public integration tests.
  - Qualify each declared operation and format, required cleanup, and usage with the Meta account before activation.
  Source:
  `docs/meta-media-assessment.md`, with P011 as the current shared operation contract.
- [ ] [F058] (P1) {F057} Add Meta conversational image operations.
  Goal:
  Complete the Responses image conversation scope approved under P009 on 2026-09-06.
  Requirements:
  - Model image conversation state as tenant-owned resources linked to durable operations and assets.
  - Support successive generation and edit turns with ordered references and all verified image controls.
  - Keep signed image identifiers, provider response identifiers, and native replay items private.
  - Define one canonical retention and replay policy from verified Meta contracts.
  - Validate input ownership, replay order, expiry, deletion, and provider history failures at their boundaries.
  - Preserve immutable accepted intent, idempotency, dispatch evidence, and usage for each turn.
  - Update OpenAPI, official client methods, and a complete conversation example.
  Validation:
  - Prove successive edits, cross-tenant rejection, expired references, interrupted turns, cleanup, and duplicate convergence.
  - Verify real provider replay and deletion behavior before activation.
  Source:
  `docs/meta-media-assessment.md`.
- [ ] [F059] (P1) Add all six approved SiliconFlow provider offerings.
  Goal:
  Implement the full P010 expansion approved on 2026-09-06.
  Requirements:
  - Add GLM-5.3, dated DeepSeek V4 Pro and Flash, Kimi K3, Hy3, and LongCat 2.0.
  - Use the exact upstream selectors recorded in `docs/siliconflow-expansion.md`.
  - Verify each provider-specific reasoning control, context, output limit, media capability, and price.
  - Resolve the conflicting Kimi K3 and LongCat output limits before canonical registration.
  - Include Kimi image input after its provider-specific formats and limits are verified.
  - Keep existing provider defaults and tenant selections unchanged.
  - Add missing model publishers and families with verified weight-access metadata.
  - Qualify complete candidate catalogs independently of shared exact-model activation.
  - Keep unqualified offerings outside the canonical catalog when their shared model is already enabled.
  - Update management, public discovery, clients, examples, and generated artifacts with qualified offerings.
  Validation:
  - Add failing public integration tests before each implementation change.
  - Prove exact dispatch, reasoning, usage, continuation, invalid inputs, media, and saved defaults.
  - Verify authenticated model discovery and each declared capability through the disposable proxy.
  - Run final CI and preserve provider acceptance separately from production activation.
  Source:
  `docs/siliconflow-expansion.md`.
- [!] [F051] (P1) Add GLM 5.3 and GLM 5.3 Flash text routes.
  Goal:
  Add both current Z.AI models through the international general API.
  Requirements:
  - Register exact models `glm-5.3` and `glm-5.3-flash` as disabled text candidates.
  - Use synchronous Chat Completions with `max_tokens` and enabled thinking.
  - Accept only `low`, `high`, and `max` reasoning effort.
  - Preserve provider-default effort when the caller omits the field.
  - Record the 1M-token context, 131,072-token output maximum, and current prices.
  - Identify the Flash promotion expiration separately from its list prices.
  - Use the existing `glm-5` family with verified open weights.
  - Keep the current Z.AI default and existing model identities.
  - Keep both candidates disabled until live provider qualification passes.
  Validation:
  - Start with failing public HTTP tests for both models.
  - Verify all public text endpoints, each effort, continuation, private reasoning, and usage.
  - Verify rejection of unsupported effort, media, structured output, and excessive output budgets.
  - Verify catalog metadata and candidate exclusion from public discovery.
  - Run final CI and both disposable live candidate commands.
  Sources:
  - https://docs.z.ai/guides/llm/glm-5.3
  - https://docs.z.ai/guides/vlm/glm-5.3-flash
  - https://docs.z.ai/api-reference/llm/chat-completion
  - https://docs.z.ai/guides/overview/pricing
  - https://huggingface.co/zai-org/GLM-5.3
  - https://huggingface.co/zai-org/GLM-5.3-Flash
  Implementation (2026-09-05):
  The private catalog contains both disabled text candidates with exact upstream identifiers, reasoning levels, output limits, and prices.
  The existing thinking adapter sends `thinking.type: enabled` and each explicit effort.
  The Flash record separates list prices from its promotion, which ends at 2026-09-09 16:00 UTC.
  The initial HTTP tests failed with `unknown model` for both identifiers.
  The focused target now passes 24 text requests and 24 continuations across both models.
  It verifies visible output, private reasoning, usage totals, unsupported inputs, and output budget rejection.
  Public catalog and browser tests verify candidate exclusion from discovery.
  Final CI passes all 12 gates and 95 browser tests with 100.0% Go statement coverage in 222 seconds.
  Evidence: `/tmp/llm-proxy-f051-red.log`, `/tmp/llm-proxy-f051-focused.log`, and `/tmp/llm-proxy-f051-ci.log`.
  Changed files:
  `configs/providers.yml`, `internal/proxy/zai_current_models_test.go`, `tests/e2e/management-ui.spec.js`, `Makefile`,
  `README.md`, `docs/provider-catalog.md`, and `docs/zai-current-models.md`.
  Public event schemas did not change.
  Blocked:
  `ZAI_API_KEY` is absent from the process and all six private repository environment files.
  Both disposable live commands stopped before dispatch with `required catalog environment is not set: ZAI_API_KEY`.
  No live GLM 5.3 or Flash request ran.
  The operator must configure the key and complete both candidate qualifications before activation.
  Evidence: `/tmp/llm-proxy-f051-glm-5.3-live.log` and `/tmp/llm-proxy-f051-glm-5.3-flash-live.log`.
- [!] [F052] (P2) Add image input for GLM 5.3 Flash.
  Goal:
  Expose the current Flash image contract after F051 text registration.
  Requirements:
  - Verify the exact image byte unit, strict size boundary, dimensions, and image count limit.
  - Add JPEG and PNG input through `image_url` content blocks on the existing Chat Completions route.
  - Enforce the documented image dimensions through an explicit media admission contract.
  - Expose current limits and their sources in the public catalog.
  - Keep the candidate disabled until live image qualification passes.
  Validation:
  - Start with failing public HTTP tests for ordered images and every published admission limit.
  - Verify private reasoning and text continuation with image input.
  - Run final CI and the disposable live candidate media command.
  Evidence (2026-09-05):
  The Flash model guide documents image URLs and Base64 Data URLs.
  The current OpenAPI image schema specifies JPEG and PNG, images under 5M, and dimensions at most 6000 by 6000.
  Its 150-image limit names older models and does not explicitly name GLM 5.3 Flash.
  Before F052, the proxy media schema supported byte and count limits but had no image dimension limit.
  Sources:
  - https://docs.z.ai/guides/vlm/glm-5.3-flash
  - https://docs.z.ai/openapi.json
  Implementation (2026-09-05):
  The disabled Flash candidate accepts ordered JPEG and PNG attachments through the existing Chat Completions adapter.
  The catalog and public offering expose explicit `image_mime_types`.
  Image dimensions use `image_width_pixels` and `image_height_pixels`, with 6000-pixel bounds for each attachment.
  Header checks apply to inline images and tenant assets before dispatch.
  Oversized dimensions return HTTP 413. Malformed images, MIME mismatches, and unsupported formats return HTTP 400.
  Tests verify image order, private reasoning, continuation, usage, asset reader integrity, catalog validation, and Go client discovery.
  The renderer accepts valid format and dimension metadata and rejects invalid combinations.
  The initial HTTP test failed with HTTP 400 `unsupported provider capability` for Flash image input.
  Focused image and text tests pass.
  Final CI passes all 12 gates and 96 browser tests with 100.0% Go statement coverage in 225 seconds.
  Evidence: `/tmp/llm-proxy-f052-red.log`, `/tmp/llm-proxy-f052-focused.log`, and `/tmp/llm-proxy-f052-ci.log`.
  Changed files:
  `configs/providers.yml`, `internal/proxy/provider_catalog_schema.go`, `internal/proxy/model_catalog.go`,
  `internal/proxy/provider_types.go`, `internal/proxy/provider_registry.go`, `internal/proxy/catalog_service.go`,
  `internal/proxy/public_capabilities.go`, `internal/proxy/message_media.go`, `internal/proxy/media_limits.go`,
  `internal/proxy/image_dimensions.go`, `internal/proxy/image_dimensions_internal_test.go`,
  `internal/proxy/zai_image_test.go`, `internal/proxy/zai_current_models_test.go`, `pkg/llmproxyclient/capabilities.go`,
  `scripts/render_public_site.mjs`, `tests/e2e/public-site-renderer.spec.js`, `Makefile`, `docs/openapi.yaml`,
  `site/docs/index.html`, `README.md`, `docs/provider-catalog.md`, and `docs/zai-current-models.md`.
  Public catalog changes: optional offering field `image_mime_types` and media-limit unit `pixels`.
  Public event schemas did not change.
  Blocked:
  The provider's image-size rule says under 5M but does not define the byte unit.
  Its image-count rule names older models and does not explicitly include Flash.
  The catalog records the exact image-byte, image-count, and total request bounds as unknown.
  Provider clarification or equivalent authoritative evidence must establish these bounds before activation.
  `ZAI_API_KEY` is absent from the process and all six private repository environment files.
  The disposable media command stopped before dispatch with `required catalog environment is not set: ZAI_API_KEY`.
  No live Flash image request ran. Live image qualification requires the key.
  Evidence: `/tmp/llm-proxy-f052-live.log`.
- [!] [F050] (P1) Add current hosted Qwen 3.8 models.
  Goal:
  Add the current Qwen 3.8 models through the Singapore DashScope workspace connection.
  Requirements:
  - Register `qwen3.8-max`, `qwen3.8-max-0902`, `qwen3.8-flash`, `qwen3.8-2.4t-a95b`, and `qwen3.8-27b`.
  - Verify each model's hosted limits and image contract before registration.
  - Use synchronous Responses requests with `store: false`.
  - Expose the distinct `none`, `low`, `medium`, and `xhigh` reasoning levels.
  - Send explicit effort in `reasoning.effort` and preserve omission.
  - Record current Singapore prices and exact verified limits.
  - Keep candidates disabled until live text, reasoning, and declared media checks pass.
  - Preserve the current default and existing model identities.
  Validation:
  - Start with failing public HTTP integration tests.
  - Verify exact dispatch, effort, continuation, visible output, usage, and declared image input.
  - Verify catalog metadata and candidate exclusion from public discovery.
  - Run final CI and each disposable live candidate command.
  Sources:
  - https://www.alibabacloud.com/help/en/model-studio/qwen-api-via-openai-responses
  - https://www.alibabacloud.com/help/en/model-studio/text-generation-model
  - https://www.alibabacloud.com/help/en/model-studio/vision-model
  - https://www.alibabacloud.com/help/en/model-studio/model-pricing
  Evidence (2026-09-05):
  The Responses reference lists all five identifiers.
  The pricing reference lists all five for Singapore.
  `DASHSCOPE_API_KEY` and `DASHSCOPE_BASE_URL` are absent from the process and six private repository environment files.
  The hosted model pages confirm a 1,000,000-token context and 131,072-token output maximum for all five identifiers.
  The hosted `qwen3.8-2.4t-a95b` route accepts text only. The other four accept image input.
  Additional sources:
  - https://help.aliyun.com/en/model-studio/qwen3-8-max
  - https://help.aliyun.com/en/model-studio/qwen3-8-flash
  - https://help.aliyun.com/en/model-studio/qwen3-8-2-4t-a95b
  - https://help.aliyun.com/en/model-studio/qwen3-8-27b
  Implementation (2026-09-05):
  The private catalog contains all five disabled candidates with current Singapore prices and verified limits.
  Four candidates accept image input. The hosted 2.4T offering accepts text only.
  The two open models use the `qwen3-8` family with `open_weights` metadata.
  The DashScope codec sends each explicit effort in `reasoning.effort` and preserves omission.
  Public tests cover all five models, 25 text requests, 25 continuations, usage, output limits, and declared image behavior.
  Public metadata and browser tests verify candidate exclusion from discovery.
  Final CI passes all 12 gates and 95 browser tests with 100.0% Go statement coverage in 211 seconds.
  Evidence: `/tmp/llm-proxy-f050-focused.log` and `/tmp/llm-proxy-f050-ci.log`.
  Changed files:
  `configs/providers.yml`, `internal/proxy/provider_types.go`, `internal/proxy/model_catalog.go`,
  `internal/proxy/provider_router.go`, `internal/proxy/dashscope_responses.go`, `internal/proxy/provider_media_edges_internal_test.go`,
  `internal/proxy/qwen_current_models_test.go`, `tests/e2e/management-ui.spec.js`, `Makefile`, `README.md`,
  `docs/provider-catalog.md`, `docs/dashscope-responses.md`, and `docs/qwen-current-models.md`.
  Public event schemas did not change.
  Blocked:
  `DASHSCOPE_API_KEY` and `DASHSCOPE_BASE_URL` are absent from the process and all six private repository environment files.
  All five text commands and four media commands stopped before provider dispatch because both inputs are missing.
  No live Qwen 3.8 request ran.
  The operator must configure the Singapore workspace connection and complete each candidate's qualification before activation.
  Evidence: `/tmp/llm-proxy-f050-qwen3.8-*-live.log`.
- [!] [F049] (P1) Add Muse Spark 1.3.
  Goal:
  Add the current Standard-tier Meta text model with explicit reasoning controls.
  Requirements:
  - Register exact model `muse-spark-1.3` as disabled until live qualification passes.
  - Use the existing synchronous Chat Completions transport and `max_completion_tokens`.
  - Support `minimal`, `low`, `medium`, `high`, `xhigh`, and `max` reasoning effort.
  - Preserve provider-default reasoning when the caller omits the effort.
  - Record the 1,048,576-token context window and current Standard prices.
  - Preserve the current Meta default and existing offerings.
  Validation:
  - Start with failing public HTTP integration tests.
  - Verify exact dispatch, reasoning, visible continuation, usage, and rejection of unsupported structured output.
  - Verify catalog metadata and candidate exclusion from public discovery.
  - Run final CI and the disposable live candidate command.
  Sources:
  - https://research.meta.ai/blog/introducing-muse-spark-1-3
  - https://dev.meta.ai/docs/models
  - https://dev.meta.ai/docs/reasoning
  - https://dev.meta.ai/docs/protocols/chat-completions
  - https://dev.meta.ai/docs/pricing-rate-limits
  Scope:
  This addition uses Standard-tier text through the current Meta adapter.
  Media, structured output, caller tools, and a Meta Responses transport require separate contracts.
  The current model and reasoning guides confirm `max` for Standard-tier Muse Spark 1.3.
  The protocol parameter table has not added `max` to its general list.
  Live qualification must verify each explicit effort before activation.
  Implementation (2026-09-05):
  The private catalog contains the disabled Standard-tier model, context window, and three price components.
  The shared Chat Completions effort vocabulary now includes the six declared Meta values.
  Each offering still restricts its own accepted effort values.
  Tests verify all three public text endpoints, omission, six efforts, continuation, usage, and catalog metadata.
  Tests also verify rejection of unsupported structured output and candidate exclusion from public discovery.
  Final CI passes all 12 gates with 100.0% Go statement coverage in 197 seconds.
  Evidence: `/tmp/llm-proxy-f049-ci.log` and `/tmp/llm-proxy-f049-focused-final.log`.
  Changed files:
  `configs/providers.yml`, `internal/proxy/provider_types.go`, `internal/proxy/meta_current_models_test.go`,
  `Makefile`, `tests/e2e/management-ui.spec.js`, `README.md`, `docs/provider-catalog.md`,
  `docs/implementation/provider-routing-plan.md`, and `docs/meta-current-model.md`.
  Public event schemas did not change.
  Blocked:
  Final CI stopped with `Go files require formatting: ./internal/proxy/vertex_test.go` during the active F060 change.
  Browser setup stopped with `public_capabilities_invalid: catalog.providers[10].credential_kinds`.
  Complete the shared validation after F060 resolves these failures.
  Acceptance (2026-09-07):
  The existing Meta key passed verification, omitted effort, and all six explicit efforts with HTTP 200.
  All live Meta tests use `MUSE_API_KEY`, as required by the user.
  After the final name change, live key verification and a Muse Spark 1.3 text request passed with HTTP 200.
  The catalog environment binding and the entry in `configs/.env` use that exact name.
  The key value is unchanged.
  A CLI regression assertion requires this exact binding in live discovery.
  Live verification and all seven reasoning cases passed again through this binding.
  Evidence: `/tmp/llm-proxy-muse11-live.log`.
  The catalog now enables Muse Spark 1.3.
  Focused public HTTP tests pass, and discovery and browser expectations include the enabled model.
  Production deployment belongs to the operator.
  Evidence: `/tmp/llm-proxy-f049-qualified-live.log`, `/tmp/llm-proxy-f049-activation-green.log`, `/tmp/llm-proxy-f049-browser.log`, and `/tmp/llm-proxy-f049-activation-ci.log`.
- [!] [F048] (P1) Add Grok 4.6.
  Goal:
  Expose the exact current xAI model after live qualification.
  Requirements:
  - Add `grok-4.6` as a disabled candidate in the existing Grok family.
  - Declare its 500,000-token context, JPEG/PNG inputs, structured output, and function calls.
  - Support `low`, `medium`, `high`, and `xhigh` through `reasoning.effort`.
  - Preserve the provider default when effort is omitted.
  - Record standard prices below 200,000 input tokens and at or above that threshold.
  - Preserve synchronous Responses requests with `store:false` and private reasoning output.
  - Keep `grok-4.3` as the provider default.
  - Qualify the exact model through the provider account before activation.
  Validation:
  - Start with failing public HTTP tests.
  - Verify all efforts, images, structured output, function calls, and disabled discovery.
  - Run final CI after the last implementation change.
  Sources:
  - https://docs.x.ai/developers/models/grok-4.6
  - https://docs.x.ai/developers/model-capabilities/text/reasoning
  - https://docs.x.ai/developers/model-capabilities/images/understanding
  - https://docs.x.ai/developers/pricing
  Implementation:
  The disabled offering includes the current context, image limits, reasoning controls, caller functions, and both standard price tiers.
  The `xai_responses` reasoning adapter sends `reasoning.effort` and preserves omission.
  Public HTTP tests verify every effort, private reasoning, usage, image bytes, structured output, and a complete function-call round trip.
  Browser discovery excludes the candidate. The xAI default remains `grok-4.3`.
  Validation:
  Focused HTTP tests pass. Final CI passes all 12 gates and 95 browser tests with 100.0% Go statement coverage in 185 seconds.
  The Governor check passes. Changed technical prose has no STE findings.
  Blocked:
  `XAI_API_KEY` is absent from the process and all six private repository environment files.
  The candidate harness stops with `required catalog environment is not set: XAI_API_KEY` before provider dispatch.
  Live key, reasoning, image, structured-output, and function-call qualification remain incomplete.
  Keep the model disabled until these checks pass.
- [!] [F047] (P1) Add Gemini 3.8 Flash and 3.5 Flash-Lite.
  Goal:
  Expose both current Gemini models through the existing text and media contract after live qualification.
  Requirements:
  - Add exact IDs `gemini-3.8-flash` and `gemini-3.5-flash-lite` as disabled candidates.
  - Declare their 1,048,576-token input context and 65,536-token output limit.
  - Support text, image, audio, and the existing structured output contract.
  - Declare `low`, `medium`, and `high` for Gemini 3.8 Flash.
  - Also declare `minimal` for Gemini 3.5 Flash-Lite.
  - Preserve provider defaults when the caller omits effort.
  - Record current standard prices and Gemini 3.8 Flash's introductory price period.
  - Reuse the current Gemini Interactions transport and provider default.
  - Qualify reasoning, media, and each model's declared completion lifecycle before activation.
  - For Gemini 3.8 Flash, also qualify active retrieval, cancellation, and deletion.
  - For Flash-Lite, qualify synchronous key verification and completion without storage.
  - Keep candidates disabled when any required provider check fails.
  Validation:
  - Start with failing public HTTP and candidate CLI tests.
  - Verify exact metadata, request payloads, errors, and public discovery.
  - Run each exact model through the disposable provider harness.
  - Run final CI after the last implementation change.
  Implementation:
  - Both exact offerings are disabled. They declare current efforts, context, output, standard prices, and the common 20 MB inline media bound.
  - Public HTTP tests cover reasoning, structured output, media order, Files API routing, cleanup, and catalog metadata.
  - The candidate CLI selects each exact model and rejects unknown selections before dispatch.
  - Browser discovery excludes both candidates. The default remains `gemini-3.5-flash`.
  Blocked:
  - Both Models API requests passed and confirmed the declared token limits.
  - Gemini 3.8 Flash returned HTTP 429 with a free-tier request limit of 5.
  - A later lifecycle probe timed out before its first response. Its stored lifecycle remains unqualified.
  - After B197, Flash-Lite passed the direct reasoning matrix, proxy key verification, omitted-effort text, and image input.
  - Flash-Lite's full proxy reasoning matrix timed out on its next request.
  - Live audio remains unqualified for both models. Both candidates remain disabled until all applicable checks pass.
  Local validation:
  - Focused HTTP, candidate CLI, and browser checks pass.
  - Initial CI rejected a migration-test target that selected a disabled candidate: `reason=dangling_reference`.
  - The fixture now selects from enabled offerings. The Go suite passes with 100.0% statement coverage.
  - Final CI passes all 12 gates, including 95 browser tests, in 185 seconds.
  Sources:
  - https://ai.google.dev/gemini-api/docs/models/gemini-3.8-flash
  - https://ai.google.dev/gemini-api/docs/models/gemini-3.5-flash-lite
  - https://ai.google.dev/gemini-api/docs/thinking
  - https://ai.google.dev/gemini-api/docs/pricing
  Acceptance refresh (2026-09-05):
  Independent runs with the repository credential reproduced both provider blockers.
  Gemini 3.8 Flash timed out after 45,005 milliseconds with no response bytes at the first omitted-effort request.
  Flash-Lite passed omitted effort and all four explicit efforts. Its background completion creation returned HTTP 400.
  Neither run reached the required full stored lifecycle. Both candidates remain disabled.
  Evidence: `/tmp/llm-proxy-f047-gemini38-recheck.log` and `/tmp/llm-proxy-f047-lite-recheck.log`.
  The unchanged candidate harness passed all 12 CI gates for I234 before these paid runs.
  Latest acceptance refresh (2026-09-05):
  Gemini 3.8 Flash passed omitted, `low`, and `medium` effort with HTTP 200.
  Its `high` effort request returned HTTP 429. The run stopped before the stored lifecycle checks.
  The candidate remains disabled until all required acceptance checks pass.
  Evidence: `/tmp/llm-proxy-f047-gemini38-goal-refresh.log`.
  Root cause diagnosis (2026-09-06, I251 and B197):
  Gemini 3.8 Flash's HTTP 429 reports `generate_content_free_tier_requests, limit: 5` and a retry delay of approximately 41 seconds.
  Flash-Lite's HTTP 400 states: `Model 'gemini-3.5-flash-lite' does not support background interactions.`
  B197 corrects its catalog lifecycle and candidate checks to synchronous completion.
  The direct candidate matrix then passed omitted, minimal, low, medium, and high effort with HTTP 200.
  Flash-Lite also had a transient HTTP 500 high-demand response during diagnosis.
  Evidence: `/tmp/llm-proxy-f047-gemini38-diagnosis.log`, `/tmp/llm-proxy-f047-lite-background-diagnosis.log`, and `/tmp/llm-proxy-b197-lite-live.log`.
  Flash-Lite's corrected proxy route passed key verification, omitted-effort text, and the image smoke test with HTTP 200.
  Its full proxy reasoning matrix stopped on the next request after a 45,003-millisecond timeout.
  A separate Gemini 3.8 Flash lifecycle probe also timed out before its first response.
  Both candidates remain disabled. Full proxy reasoning, applicable stored lifecycle checks, and live audio still require acceptance.
  Evidence: `/tmp/llm-proxy-b197-lite-proxy-live.log`, `/tmp/llm-proxy-b197-lite-media-declared-env.log`, and `/tmp/llm-proxy-f047-flash-lifecycle-diagnosis.log`.
  Vertex alternative (2026-09-07):
  F060 qualified both exact models through the public Vertex proxy with the dedicated service account.
  These Vertex offerings are enabled. The Developer API offerings remain disabled.
  See `docs/vertex-gemini.md` for the tenant cutover.
- [!] [F045] (P1) Add MiniMax M3 to the current text and image routes.
  Goal:
  Add the current M3 model through the existing MiniMax tenant connection.
  Requirements:
  - Add exact model `minimax-m3` and upstream selector `MiniMax-M3`.
  - Declare a separate open-weight MiniMax M3 family.
  - Use the existing Chat Completions transport and MiniMax request profile.
  - Record a 1,000,000-token context and 524,288-token output maximum.
  - Preserve the `minimax-m2.7` default and existing tenant selections.
  - Support the current canonical JPEG, PNG, and WebP image inputs.
  - Record documented image and request limits with current sources.
  - Record standard input, output, and cache-read prices for both context tiers.
  - Keep M3 disabled until live text and image qualification passes.
  - Add an explicit candidate mode to the disposable live harness.
  - Enable the selected candidate only in the copied catalog for that run.
  - Preserve standard discovery and reject invalid candidate selections.
  Sources:
  - https://platform.minimax.io/docs/api-reference/text-openai-api
  - https://platform.minimax.io/docs/api-reference/text-chat-openai
  - https://platform.minimax.io/docs/guides/pricing-paygo
  - https://huggingface.co/MiniMaxAI/MiniMax-M3
  Validation:
  - Prove exact routing, usage, reasoning separation, and output-limit rejection.
  - Prove image serialization and media rejection through public HTTP requests.
  - Prove disabled discovery and candidate-only activation in a copied catalog.
  - Prove candidate prices and capabilities through HTTP.
  - Prove the browser omits the disabled candidate.
  - Run focused tests, final CI, and live text and image qualification.
  Progress (2026-09-05):
  - Added the disabled M3 model, its provider offering, limits, and both standard price tiers.
  - Public HTTP tests prove routing, image input, output limits, usage, prices, and key verification.
  - The initial route test returned HTTP `400` because M3 was absent.
  - Added `--candidate-model` to enable one disabled model only in the disposable catalog.
  - CLI tests prove exact copied activation and rejection of invalid candidate selectors.
  - The initial candidate test failed with an unknown argument error.
  - `make test-minimax-m3` and `make test-live-candidate-contract` passed.
  - The first CI run found duplicate activation fields in the catalog test fixture.
  - Corrected the fixture to accept both activation states. `make test-provider-catalog` passed after the correction.
  - Browser validation exposed B193 when the disabled candidate left an empty public model family.
  - B193 now excludes unused runtime families. I247 completes the retirement test before database cleanup.
  - Final CI passed all 12 gates in 181 seconds with 100% Go statement coverage and 95 browser tests.
  - Evidence: `/tmp/llm-proxy-f045-b193-i247-ci.log`. Public event schemas did not change.
  - Added the M3 runbook and updated current catalog and route documents.
  Blocked:
  `MINIMAX_API_KEY` is absent from the process and all six authorized private environment files.
  `make test-live-minimax-m3 LIVE_ENV_FILE=configs/.env` stopped before a provider call with this error:
  `error: minimax requested but required catalog environment is not set: MINIMAX_API_KEY`.
  Live text and image acceptance remains open. M3 remains disabled until both checks pass.
- [x] [F039] (P1) {F024,I272} Complete OpenAI image editing and progressive output.
  Goal:
  Preserve the current MediaOps OpenAI image contract before its complete provider cutover.
  Progress:
  - Added Images editing through ordered tenant assets, PNG masks, catalog transport references, and the official Go client.
  - Public HTTP tests verify two provider definitions, output bytes, duplicate convergence, changed-intent rejection, and invalid or foreign input rejection.
  - Added durable progressive assets for Images generation and editing. Public tests verify partial order, duplicate events, result integrity, tenant isolation, restart, worker replacement, and retention.
  - The browser shows editing through the existing connection. The Go and Python clients expose typed progressive assets.
  - Added Responses generation, editing, private chain handles, route and connection binding, background retrieval, cancellation, and verified previews.
  - Public tests cover two providers, foreign or invalid parents, restart recovery, stream interruption, parent retention, and corrupt provider output.
  - The bounded source audit found zero native handles in 561 image records and 713 provider snapshots. It retained all ten uncertain source records without a provider call.
  Resolution (2026-09-19):
  - Completed source implementation, official clients, OpenAPI, examples, and source recovery evidence.
  - Final `make ci` passed all 14 gates in 334 seconds, with 100.0% Go statement coverage.
  - Validation included 60 Python checks, 129 frontend browser tests, and seven authentication browser tests.
  - Protocol fixtures verified storage failures, stream limits, authority changes, and private response chains without another provider submission.
  - Changed prose has no new mechanical findings in the reviewed scope.
  - Client publication, service activation, and MediaOps I084 consumer acceptance remain separate pending work.
  Requirements:
  - Apply the provider catalog and protocol adapter contract in P011 before each capability release.
  - Add image editing, ordered multi-image inputs, masks, and the current Images and Responses controls.
  - Preserve output counts, formats, quality, transparency, compression, and supported progressive-image behavior.
  - Represent response chains through tenant-owned gateway resources. Keep native response identifiers private.
  - Reuse F022 operation, cancellation, asset, artifact, and retry contracts.
  - Audit recoverable records before adding an importer. Record a zero-count receipt when none require migration.
  - Coordinate the released contract with MediaOps I084.
  Deliverables:
  - Typed controls, provider translation, official-client methods, capability data, and recovery evidence.
  Validation:
  - Complete the P011 second-provider acceptance procedure for each protocol adapter added or changed in this slice.
  - Prove current OpenAI generate/edit parity through real handlers and controlled provider fixtures.
  - Prove tenant-isolated response chains, partial output order, cancellation, and final artifact integrity.
  - Run current repository validation and separately record explicitly authorized live acceptance.
- [ ] [F040] (P1) {F043} Add Vertex image operations through tenant provider connections.
  Current ownership and handoff:
  - Close this capability issue after destination source validation.
  - Record consumer cutover under its MediaOps issue and final retirement under I244.
  - LLM Proxy owns native provider calls, credentials, resource APIs, uploads, and provider recovery for this capability.
  - Deliver the public contract and official client methods before MediaOps I085 switches its service integration.
  - Keep product jobs, application assets, and local media processing in one MediaOps backend.
  - Keep TelePrompter browser controls connected to MediaOps.
  - Preserve detailed requirements and dated implementation evidence below.
  - Record source qualification separately from consumer activation and paid acceptance.
  Goal:
  Move the current Vertex image provider contract behind the shared gateway.
  Requirements:
  - Apply the provider catalog and protocol adapter contract in P011 before each capability release.
  - Add the current Vertex generation and editing routes with their exact supported controls.
  - Use the F043 Google credential and staging contract where the selected route requires it.
  - Keep project, location, credential references, and provider-native identifiers inside validated server configuration.
  - Reuse the common catalog, operations, assets, artifacts, and official-client types.
  - Inventory recoverable records before implementing route-specific migration.
  - Coordinate the released contract with MediaOps I085.
  Deliverables:
  - Vertex image adapters, catalog entries, client types, and bounded recovery or migration support.
  Validation:
  - Complete the P011 second-provider acceptance procedure for each protocol adapter added or changed in this slice.
  - Prove exact route controls, tenant isolation, staging ownership, duplicate prevention, and artifact integrity.
  - Run current repository validation and separately record explicitly authorized live acceptance.

- [x] [F041] (P1) Add FAL image operations and queue recovery.
  Progress: One FAL provider now declares shared credentials, read-only verification, and the Reve image transport.
  Progress: Public tests cover a second provider identity, ordered artifacts, uncertain outcomes, queue recovery, and cancellation acknowledgements.
  Initial results: Catalog tests assumed 14 providers and a final Dictator entry. Updated fixtures select the intended identity.
  Initial results: The Go discovery client rejected the queue codec. Its current contract now includes that codec.
  Progress: A browser saves and reloads the FAL connection and shows Reve.
  Initial results: Corrected a fixture compile error and added queue authority rejection tests for complete statement coverage.
  Initial results: CI passed Go and Python checks, then found stale browser catalog totals. Updated assertions include Reve and FAL.
  Inventory: A bounded audit found no FAL generation record or native handle in 1,233 operation records and 667 sidecar files.
  Inventory: One FAL catalog result requires no import. The private receipt is `.git/f041-fal-source-inventory.json`.
  Resolution: Development completion includes the queue adapter, recovery, catalog, official clients, API contract, browser discovery, and bounded source inventory.
  Validation: Final `make ci` passed all 14 gates in 341 seconds with 100.0 percent Go statement coverage.
  Validation: All 140 frontend tests and seven local browser tests passed. No new event contract was added.
  Remaining: Client publication, service activation, and MediaOps I086 consumer acceptance remain separate.
  Goal:
  Move current FAL image generation and recoverable queue state behind the shared gateway.
  Requirements:
  - Apply the provider catalog and protocol adapter contract in P011 before each capability release.
  - Add current FAL image routes with tenant provider connections and provider-readable staging where required.
  - Require F043 only when the selected route needs staged inputs. Current prompt-only generation has no staging dependency.
  - Persist queue handles privately and recover the original operation without provider resubmission.
  - Reuse common operation, catalog, asset, artifact, and official-client contracts.
  - Inventory retained FAL records before adding exact tenant-bound import validators.
  - Verify destination provider-account ownership before a migration uses a native recovery handle.
  - Coordinate the released contract with MediaOps I086.
  Deliverables:
  - FAL image adapter, queue recovery, catalog entries, client types, and evidence-based migration tools.
  Validation:
  - Complete the P011 second-provider acceptance procedure for each protocol adapter added or changed in this slice.
  - Prove queue recovery after restart, uncertain transport behavior, exact controls, and ordered verified artifacts.
  - Prove staging fetch and cleanup through real files and an HTTP serving boundary.
  - Run current repository validation and separately record explicitly authorized live acceptance.
- [ ] [F042] (P1) {F072,B258} Expose Dictator media capabilities through the tenant gateway.
  Current acceptance boundary:
  - Require F072 model selection evidence and verify all six retained speech capabilities through the public gateway.
  - Preserve voice discovery, extraction duration, aligned SRT, and private native identifiers.
  - MediaOps I087 owns its service cutover. Creative Director owns its own product state and semantic output acceptance.
  - A consumer transcript-quality defect does not prove a gateway transport defect.
  - Recheck dated inventory and publication claims below before a new cutover.
  Current ownership and handoff:
  - Close this capability issue after destination source validation.
  - Record consumer cutover under its MediaOps issue and final retirement under I244.
  - LLM Proxy owns native provider calls, credentials, resource APIs, uploads, and provider recovery for this capability.
  - Deliver the public contract and official client methods before MediaOps I087 switches its service integration.
  - Keep product jobs, application assets, and local media processing in one MediaOps backend.
  - Keep TelePrompter browser controls connected to MediaOps.
  - Preserve detailed requirements and dated implementation evidence below.
  - Record source qualification separately from consumer activation and paid acceptance.
  Current execution: Positive extraction duration, media CLI commands, and durable MCP tools are implemented.
  Validation: Public CLI and MCP tests passed. All six live capabilities passed in 5.21 seconds. Final `make ci` passed 13 gates in 324 seconds.
  Initial result: CI reported four uncovered blocks. Public rejection and store-failure tests cleared all four blocks.
  Source files: `internal/proxy/dictator_adapter.go`, `dictator_grpc_protocol.go`, `mcp.go`, `mcp_media.go`, `router.go`, and `llm-proxy-client/media.go`.
  Contracts: `duration_seconds`, three media MCP tools, and eight media CLI commands. Existing event contracts did not change.
  Test and operator files: Public speech and MCP tests, CLI tests, `Makefile`, OpenAPI, generated API pages, and `docs/speech-workflows.md`.
  Production inventory: Both deployed MediaOps volumes are empty. The account owns the dedicated MediaOps tenant and its assigned Dictator connection.
  Remaining: Verify the current provider contract and hand its acceptance evidence to MediaOps I087.
  Reconciliation (2026-09-16):
  - The MediaOps tenant has Dictator connection `connection-ef115fb73594e0158a626d610393aec1` and a client key.
  - The deployed capability API returned all six speech routes for that key.
  - Five uncertain records have established failure dispositions. The user directed deletion of the dispatch without terminal evidence.
  - The private receipt maps 23 Dictator records and 13 workspace roots to the selected account, tenant, and connection.
  - Deleted `mediaops-20260821T030050-000001` as directed. The other 22 source digests still match. No replacement work was submitted.
  - Creative Director I013 owns the consumer change. Its live adapter test passed in 12.67 seconds.
  - Final consumer CI passed with 100.0 percent statement coverage. The qualified CLI is installed and its public startup checks pass.
  - The 2026-09-16 Creative Director I014 checkpoint reported 184 retained-state validation errors.
  - MediaOps I088 owns the final source-retirement receipt. Verify current consumer evidence before reporting an external blocker.
  Goal:
  Make Dictator a private media provider behind LLM Proxy's public API.
  SDK evidence (2026-09-14):
  - SDK `v1.11.0` is published at `sdk/go/dictatorspeechv1/v1.11.0` on commit `2f0c83c67dbd6f8093bb6f56359fdb818255fcbd`.
  - Dictator application release `v2.0.3` contains the SDK publication receipt.
  - `make verify-released-sdk` passed with `preset_speaker` and `text_format` through a separate consumer and fresh module cache.
  Current boundary:
  - MediaOps CLI, MCP, browser jobs, and diagnostics call Dictator through its internal adapter.
  - WriterBlock is paused and is not an active project. Its caller replacement is outside F042.
  - After consolidation, LLM Proxy owns that adapter and the public tenant authorization boundary.
  Requirements:
  - Apply the provider catalog and protocol adapter contract in P011 before each capability release.
  - Inventory retained capabilities against the current Dictator gRPC contract and MediaOps capability matrix.
  - Cover transcription, diarization, subtitles, alignment, synthesis, voice extraction, and discovery where currently supported.
  - Define tenant-owned voices, assets, artifacts, and operation resources for retained capabilities.
  - Keep native voice, job, and artifact identifiers inside private provider records.
  - Map progress, cancellation, failures, and restart recovery through the common operation contract.
  - Use an account-owned Dictator connection for each assigned tenant.
  - Keep speech engines and native workers in Dictator.
  - Use the existing qualified runtime when it satisfies the retained capability.
  - Treat P008 hardware or controller changes as dependencies only when measured requirements make them necessary.
  - Extend official Go and Python clients and current public caller interfaces in the selected capability release.
  - Inventory retained voices and jobs before activation. Require explicit tenant and provider-account ownership mapping.
  - Coordinate MediaOps I087 source migration and each actual consumer in one bounded cutover.
  - Keep MediaOps processing and application data in its backend. Extract the TelePrompter website under MediaOps I094.
  - Do not require a replacement MediaOps metrics client. Keep tenant metrics in LLM Proxy.
  - Remove obsolete public runtime routing and direct caller credentials after the cutover acceptance.
  Deliverables:
  - Private Dictator adapter, tenant media resources, official clients, caller migration inventory, and activation plan.
  Retained contract (2026-09-10):
  - Use `dictator` as the canonical provider identity.
  - Expose `audio.transcribe`, `audio.diarize`, `audio.align`, `subtitles.create`, `audio.speech.generate`, and `audio.voice.extract` through durable media operations.
  - Publish transcript, diarization, alignment, subtitle, audio, and timeline results only as tenant assets.
  - Expose synthesis voice discovery through tenant-owned gateway voice resources. Retain preset voices and extracted voices under opaque gateway identifiers.
  - Do not add a Dictator history resource. MediaOps has no retained Dictator history capability. F026 owns ElevenLabs history.
  - Keep Dictator jobs, artifacts, preset identifiers, extracted-speaker artifacts, engine selection, and connection values in private gateway records.
  - Use the current async Dictator routes for transcription, diarization, subtitles, alignment, synthesis, and extraction. Do not retain the synchronous legacy RPCs as a second execution contract.
  - The operator selected account-owned Dictator servers on 2026-09-14. This requirement replaces deployment-owned Dictator access.
  - Include Dictator in the provider selector. Store the server address, bearer token, and TLS setting in the account connection.
  - Encrypt the token through the current account credential store. Keep each tenant assignment explicit.
  - Bind execution, voice discovery, recovery, and cancellation to the assigned connection. Reject obsolete connection authority.
  - Show speech capabilities in the dashboard without text-model defaults or provider prompts.
  - The released Dictator Go SDK must include the retained preset-voice and text-format fields before the production adapter dependency is added. Do not copy protobuf definitions or use an unreleased pseudo-version as a bridge.
  - Production resource counts are not verified. Inventory extracted voices and active jobs at activation. Produce a zero-count receipt only when verified.
  Validation:
  - Complete the P011 second-provider acceptance procedure for each protocol adapter added or changed in this slice.
  - Start with real public API and persistence tests against a controlled Dictator protocol boundary.
  - Prove tenant isolation for voices, operations, and artifacts.
  - Prove exact speech behavior, native identifier privacy, restart recovery, and truthful cancellation.
  - Prove Dictator unavailability does not prevent admitted cloud operations.
  - Run repository validation and record real-runtime and consumer acceptance separately.
  Implementation evidence (2026-09-14):
  - The production adapter uses released SDK `v1.11.0` and the assigned account connection.
  - The browser supports the server address, token, TLS selection, tenant assignment, and the Media tab.
  - Public HTTP tests use real gRPC servers for all six capabilities, extracted voices, and separate account servers.
  - The tests cover native cancellation and startup recovery without another submission.
  - Cross-account voice, operation, and asset reads return HTTP 404.
  - Credential rejection, rate limits, timeouts, and server failure do not save connections.
  - SDK boundary tests reject damaged artifacts and foreign job or voice references.
  - Diarization uses a closed result schema. The public HTTP regression rejects native resource fields without publishing an asset.
  - Initial failures identified rejected connection creation and text-model validation for saved speech connections. Both paths now pass.
  - Published LLM Proxy module `v1.9.1` does not contain the media-operation or voice client methods. I087 requires a new client release.
  - Before the alignment SRT change, `make ci` passed all 13 gates in 337 seconds, with 100 percent Go statement coverage.
  - MediaOps browser alignment requires SRT. The public HTTP test first failed with one output, then passed with two outputs.
  - `make test-dictator` passed after the adapter published exact aligned SRT bytes as the second tenant asset.
  - Final `make ci` passed all 13 gates in 331 seconds after the SRT change. Go coverage is 100 percent.
  - Validation includes 59 Python checks, 117 frontend browser tests, and the real TAuth management browser suite.
  - The caller inventory is in `docs/media-gateway-consolidation.md`. Active scope includes the MediaOps provider cutover and all affected product flows.
  - The operator replaced server metrics with tenant metrics. MediaOps must not collect or expose server totals.
  - `GET /model/v1/provider-diagnostics/dictator` returns retained tenant operation counts without a provider call.
  - Counts exclude other tenants and providers. Terminal detail expiry removes its count.
  - The server, official Go client, Python client, and OpenAPI schema use one closed tenant response.
  - Public HTTP tests prove tenant and provider isolation, retained state counts, detachment, and zero native metrics calls.
  - The first revision CI mixed source snapshots during coverage collection and reported 99.8 percent. Its result was discarded.
  - Final `make ci` passed all 13 gates in 330 seconds against the fixed source. Go coverage is 100 percent.
  - MediaOps removed native metrics collection and public metric fields. Its focused tests and final CI passed.
  - The operator removed the proposed MediaOps tenant count integration from scope. It is not a publication or completion blocker.
  - MCP resources belong to caller-selected workspaces. The backend YAML does not establish the complete retained-resource inventory.
  - Remaining gates: destination migration, required consumer integration, client publication for those consumers, retained-resource inventory, and separate live acceptance.
  Audit evidence (2026-09-15):
  - `docs/dictator-migration-audit.md` records the implementation, caller inventory, resource locations, and next source changes.
  - Published Go module `v1.10.0` contains the required client methods. Its client and contract files match commit `d7c7dab0b530d2294f3b347595c45e52a445b2a0`.
  - The Go-client publication blocker is cleared. Publication does not prove deployed service or consumer acceptance.
  - `make test-dictator` and `make test-client-contracts` passed against the current source.
  - MediaOps retains direct shared execution. The paused WriterBlock caller is excluded from active migration requirements.
  - B222 corrected catalog-selected registration and connection fields. The second provider passes the shared public acceptance flow and browser checks.
  - The inspected MediaOps MCP store has 164 records and no Dictator provider records. Other workspaces and deployed stores remain unverified.
  - Live gateway acceptance passed all six capabilities. Production owner mappings, retained-resource transfer, deployment, and consumer acceptance remain open.
  Protocol and live acceptance (2026-09-15):
  - The second provider uses distinct model identity, connection fields, endpoint values, and credentials through the existing protocol components.
  - Public tests cover duplicate requests, restart recovery, cancellation, tenant isolation, and one usage event per completed operation.
  - The browser test creates both account connections and displays the selected model capabilities.
  - `make test-dictator-live` passed transcription, diarization, alignment, subtitles, extraction, and synthesis in 22.94 seconds.
  - `docs/dictator-live-acceptance.md` records required inputs, fixture duration, and the live result.
  - Final `make ci` passed 13 gates in 333 seconds, with 100.0 percent Go coverage. B222 and I264 are resolved.
  - MediaOps provider-call replacement and production resource ownership remain open. WriterBlock remains excluded.
  - MediaOps extraction exposes a duration control. Keep this control when its provider call moves.
  Shared workflows and ownership (2026-09-15):
  - Destination CLI and MCP tools now use the durable media operation lifecycle. Extraction preserves the positive duration control and default.
  - Public tests and live acceptance passed all six speech capabilities. Final `make ci` passed 13 gates with 100.0 percent Go coverage.
  - The operator requested a MediaOps tenant. The management API created `managed-39cb2d58f065d06646f487cc82b98d4f` and verified its account ownership.
  - The tenant has no provider connection or client key. Connection assignment and deployed consumer acceptance remain open.
  - Both deployed MediaOps volumes contain zero files. Creative Director uses an additional MediaOps store in `/Users/tyemirov/Development/Kamu/.mediaops`.
  - That store contains 1,065 operation records, including 23 Dictator records. Six diarization records remain unresolved: five `uncertain` and one `dispatched`.
  - Records reference 13 absent historical workspace roots. Corresponding directories exist under the current Kamu root and require explicit migration mapping.
  - Reconcile retained evidence and migrate Creative Director's shared speech calls before source retirement. Do not infer a complete zero-record inventory.
  - No record transfer, provider work, publication, or deployment occurred during tenant creation and the external workspace inventory.
  Scope correction (2026-09-15):
  - The operator confirmed that WriterBlock is paused and is not an active project.
  - Its public homepage already displays the pause notice. Its README and product document record the same status.
  - Exclude WriterBlock code, credentials, resource transfer, and consumer acceptance from this migration.
  - WriterBlock does not gate F042 acceptance, publication, or deployment.

- [ ] [F043] (P1) Add provider-readable media staging and Google credential profiles.
  Current ownership and handoff:
  - Close this capability issue after destination source validation.
  - Record consumer cutover under its MediaOps issue and final retirement under I244.
  - LLM Proxy owns native provider calls, credentials, resource APIs, uploads, and provider recovery for this capability.
  - Deliver the public contract and official client methods before MediaOps I089 switches its service integration.
  - Keep product jobs, application assets, and local media processing in one MediaOps backend.
  - Keep TelePrompter browser controls connected to MediaOps.
  - Preserve detailed requirements and dated implementation evidence below.
  - Record source qualification separately from consumer activation and paid acceptance.
  Goal:
  Let gateway providers read tenant media through the exact storage and credential contracts they require.
  Requirements:
  - Apply the provider catalog and protocol adapter contract in P011 before each capability release.
  - Extend the existing tenant asset store rather than create another public upload identity.
  - Add explicit GCS staging where Vertex routes require it.
  - Support bounded provider-readable HTTP staging for routes that accept that transport.
  - Reuse the verified MediaOps static HTTP and GCS behavior as source evidence.
  - Add typed Google workload-identity or service-account references with exact project and location metadata where required.
  - Keep provider staging references private and bound to one tenant asset and operation.
  - Retain staged inputs while accepted work references them. Clean up after the declared retention boundary.
  - Keep arbitrary caller paths and provider credentials outside public payloads.
  - Keep this work independent of F024's first OpenAI generation release.
  Deliverables:
  - Store adapters, typed credential profiles, staging lifecycle, and provider integration fixtures.
  Open decisions:
  - Select deployment values, provider-readable URL lifetime, and Google credential mode for each consuming route.
  P012 reconciliation (2026-09-07):
  Keep media staging separate from independent customer completion connections.
  Operator credential profiles do not satisfy P012's customer setup requirement.
  Prepare the separate storage ownership contract as the first F043 deliverable.
  Obtain its approval before implementing storage credentials.
  Bind private storage credentials to the authorized gateway account and its assigned tenant routes.
  Keep customer completion authentication under P012, independent of provider staging.
  Reuse existing credential storage and declare exact route requirements before adapter implementation.
  `docs/gemini-customer-connections.md` records the proposed boundary.
  F060 API-key revision (2026-09-08):
  F060 removed the completion route's operator profile loader after explicit user approval.
  Future storage credentials require the separate F043 ownership contract.
  Validation:
  - Complete the P011 second-provider acceptance procedure for each protocol adapter added or changed in this slice.
  - Use real files and HTTP serving to prove exact fetched bytes, digest, expiry, and cleanup.
  - Prove tenant isolation, active reference retention, failed staging, and secret-free public output.
  - Run repository validation and record storage acceptance separately from paid generation acceptance.

- [ ] [F036] (P1) Add public provider-offering price comparison.
  Goal:
  Let landing-page visitors compare published prices and workload estimates for
  compatible provider offerings. Use the authoritative provider catalog for
  each rate, condition, source, and verification date.
  Evidence:
  - The public capability resource already contains one price descriptor for
    each provider-offering operation.
  - The public site renderer validates the complete price representation but
    does not render it.
  - Catalog revision `2026-08-13.i228.1` has 67 price records. Eleven records
    contain available exact rates.
  - Ten available records cover text. One available record covers video
    generation.
  - The other 56 records identify unavailable prices and provide official
    source URLs.
  - The landing page contains a route explorer and model matrix but no price
    comparison.
  Requirements:
  - Read prices from the same immutable catalog snapshot that owns routing and
    public capabilities.
  - Do not add page-owned rates, copied price files, runtime scraping, or
    browser requests to provider price pages.
  - Compare provider offerings. Do not merge different provider routes under
    one exact model result.
  - Show the catalog revision, official source, verification date, rate units,
    conditions, and formula for each result.
  - Keep currency, billing region, billing mode, and price units explicit.
  - Replace combined free-text condition values with typed catalog fields
    before a calculation uses them.
  - Define typed token ranges, resolution, duration, quality, quantity, cache
    state, and media state when they apply.
  - Reject overlapping condition ranges and ambiguous price selections during
    catalog validation.
  - Treat `available: false` as unavailable. Never treat an unavailable price
    as zero.
  - Keep unavailable offerings visible in a separate evidence list with their
    exact unavailable reason.
  - Start in a published-rate view. Do not use a hidden default workload.
  - Let a visitor enter workload values only for components used by the
    selected operation.
  - For text, accept input, output, cache-read, and cache-write token amounts
    when applicable.
  - For dictation, accept audio duration only when the selected rates use a
    compatible duration unit.
  - For video, accept output duration, resolution, input-image quantity, and
    output quantity when applicable.
  - Let the visitor select a per-request, per-thousand-request, or monthly
    volume multiplier.
  - Calculate each component separately. Show the exact sum and formula.
  - Rank offerings only when all required components, units, and conditions
    have exact matches.
  - Do not rank prices with different units or incompatible conditions.
  - Show a ranked chart only when two or more offerings are comparable.
  - Show one eligible offering as rate evidence without a cheapest-price
    claim.
  - Filter by operation, provider, model family, weight access, and declared
    capability.
  - Preserve the current selected route when it remains eligible.
  - Highlight a route-explorer selection in the price comparison when that
    offering has price evidence.
  - Keep comparison filters independent after the initial route highlight.
  - Place a `Pricing` navigation link and a `#pricing` section before the model
    matrix.
  - Use a compact ranked horizontal bar chart on wide screens.
  - Use stacked bar segments for separate price components.
  - Put an exact currency value beside each bar. Do not require visual length
    to communicate the value.
  - Use a compact ranked list on narrow screens without horizontal page
    overflow.
  - Use the existing MPR color, border, type, spacing, and motion tokens.
  - Use accent colors only to identify components, selection, freshness, and
    unavailable state.
  - Render a semantic price table and complete evidence without JavaScript.
  - Use JavaScript only for filtering, calculations, chart updates, and route
    selection synchronization.
  - Keep keyboard operation, visible focus, live result announcements, and
    reduced-motion behavior.
  - Mark a price as `Review due` when `last_verified` is more than 30 days old.
  - Exclude a review-due price from rankings. Keep its source and rates visible
    as stale evidence.
  - Add a repository command that reports current, due-soon, review-due, and
    unavailable price records.
  - Add a weekly repository workflow that runs the price review command.
  - Report provider, model, operation, source, verification date, and remaining
    freshness days for each record.
  - Require reviewed official-source evidence before a rate or verification
    date changes.
  - Keep one current price contract. Use Git history for previous price
    versions.
  - Use this implementation sequence:
    1. Normalize price conditions and import each available official rate.
    2. Extend the static renderer with the semantic price section and evidence.
    3. Add one checked custom element for comparison controls and calculations.
    4. Add responsive chart presentation and route-selection synchronization.
    5. Add public-contract, no-JavaScript, browser, and calculation coverage.
    6. Add the price review command, weekly workflow, and operator guidance.
  Deliverables:
  - Add typed, machine-comparable price conditions to the provider catalog.
  - Add each available official rate that the current provider contract can
    represent exactly.
  - Add generated semantic price markup to the public site renderer.
  - Add the checked price comparison custom element and current module
    revision.
  - Add compact responsive styles to the landing-page stylesheet.
  - Add the `Pricing` navigation link and route-selection integration.
  - Add the price review command and weekly repository workflow.
  - Update OpenAPI, README, provider catalog documentation, and generated site
    documentation when their contracts change.
  Validation:
  - Prove every displayed rate matches the public catalog snapshot exactly.
  - Prove each result shows its source, verification date, conditions, units,
    catalog revision, and formula.
  - Prove text calculations for available Dashscope and MiniMax offerings.
  - Prove video calculations for the available xAI offering without a
    cheapest-price claim.
  - Prove unavailable, condition-mismatch, incompatible-unit, and review-due
    records never enter a ranking.
  - Prove the chart appears only for two or more comparable offerings.
  - Prove operation, provider, family, weight-access, and capability filters.
  - Prove route selection highlights the matching price evidence.
  - Prove complete price content and source links without JavaScript.
  - Prove keyboard operation, accessible names, live results, and visible
    focus through the public browser entry point.
  - Inspect the section at 1280, 900, and 390 pixels.
  - Prove the page has no horizontal overflow at each inspected width.
  - Prove reduced motion removes nonessential chart movement.
  - Prove the price review command classifies each freshness state.
  - Run `make ci` after the last application change.
- [x] [F024] (P1) Deliver the first OpenAI image-generation capability.
  Resolution:
  - Added terminal `image.generate` through the catalog-selected `openai_images` codec and the existing OpenAI connection.
  - Preserved explicit generation controls and verified ordered PNG, JPEG, and WebP artifacts through private integrity metadata.
  - Added `CreateImageGeneration`, its official Go example, API schemas, and browser discovery.
  - Proved the same codec with a second catalog provider, distinct authentication fields, and explicit zero compression.
  - Verified duplicate convergence, tenant isolation, restart uncertainty, cancellation, rejected controls, and text progress during image saturation.
  - Source validation: Final `make ci` passed all 14 gates in 342 seconds with 100.0 percent Go coverage.
  - Browser validation: 129 frontend tests and seven TAuth tests passed in local Chromium.
  - Python validation: 54 client tests and five package installation tests passed.
  - Initial validation found obsolete catalog counts, a duplicate initializer, and asynchronous test cleanup. All corrections passed final CI.
  - Client publication: The new typed method awaits its release. Independent consumer integration still requires that release.
  - Service activation: Not performed for this slice.
  - Live acceptance: Not performed. Controlled local providers supplied the protocol evidence.
  - Source files: `internal/proxy/image_generation*.go`, `media_operations.go`, catalog and protocol definitions, and `configs/providers.yml`.
  - Client and interface files: `pkg/llmproxyclient/image_generation.go`, `pkg/llmproxycontract/contract.go`, `examples/image-generation/main.go`, and `docs/openapi.yaml`.
  - Discovery and documentation: Public catalog projection, site renderer, browser capability metadata, browser tests, README, and consolidation contract.
  - Event contracts: Reused the existing durable operation and tenant asset contracts. No new event type was added.
  Goal:
  Let a backend tenant generate an image and retrieve verified bytes through the official LLM Proxy client.
  FamilyHome is the first independent consumer. It uses its existing tenant credential.
  Requirements:
  - Apply the provider catalog and protocol adapter contract in P011 before each capability release.
  - Add terminal `image.generate` requests through the existing OpenAI tenant provider connection.
  - Preserve explicit model, prompt, quality, size, background, format, compression, and output-count controls for qualified routes.
  - Qualify `gpt-image-2` with `quality=low` for the FamilyHome acceptance case.
  - Keep FamilyHome dimensions, format, prompts, and product budgets in FamilyHome configuration.
  - Execute through the F022 operation lifecycle and existing authoritative catalog.
  - Materialize ordered outputs as tenant-owned artifacts with opaque identifiers, MIME types, byte counts, and lifecycle metadata.
  - Verify stored bytes through private integrity metadata. Preserve the hashless public media contract established by B163.
  - Keep unresolved provider outcomes explicit when the provider cannot recover a lost response.
  - Keep OpenAI editing, response-chain controls, and progressive image output in F039.
  - Keep Vertex and FAL image adapters in F040 and F041.
  - Release the official Go client before the independent consumer integration.
  - F039 preserves the complete OpenAI image contract. MediaOps I084 supplies the provider cutover and acceptance through affected MediaOps callers.
  Deliverables:
  - OpenAI image adapter, catalog route, operation schema, artifact results, and official-client example.
  - A backend-to-gateway acceptance fixture with the same tenant model as text calls.
  Validation:
  - Complete the first image proof through the P011 second-provider acceptance harness.
  - Start with a failing real-service integration test and controlled OpenAI protocol fixture.
  - Prove exact selected controls, duplicate convergence, lost-response recovery, output order, and artifact integrity.
  - Prove cross-tenant reads fail and invalid credentials cause zero provider dispatch.
  - Prove interactive text retains bounded capacity during image saturation, including the same provider origin.
  - Run the current repository checks after implementation.
  - Record published-client, deployed-service, and explicitly authorized live-provider acceptance separately.
- [ ] [F025] (P1) {F043} Add durable video generation to model operations.
  Current ownership and handoff:
  - Close this capability issue after destination source validation.
  - Record consumer cutover under its MediaOps issue and final retirement under I244.
  - LLM Proxy owns native provider calls, credentials, resource APIs, uploads, and provider recovery for this capability.
  - Deliver the public contract and official client methods before MediaOps I010 switches its service integration.
  - Keep product jobs, application assets, and local media processing in one MediaOps backend.
  - Keep TelePrompter browser controls connected to MediaOps.
  - Preserve detailed requirements and dated implementation evidence below.
  - Record source qualification separately from consumer activation and paid acceptance.
  Goal:
  Make LLM Proxy the sole provider boundary for Vertex Veo, Vertex Gemini Omni,
  Runway, FAL, Kling, and xAI video generation.
  Cross-repository sequence:
  - MediaOps I010 owns the provider cutover and acceptance through affected MediaOps callers for this video slice.
  Requirements:
  - Apply the provider catalog and protocol adapter contract in P011 before each capability release.
  - Add a typed `video.generate` contract for prompt, start/end frame, source
    video, ordered image/video/audio references, reusable provider assets,
    duration, aspect, resolution, audio, extension, seed, moderation, and
    provider-supported controls.
  - Use gateway asset identifiers for every local input and typed external
    references only where the selected provider accepts them directly.
  - Implement Vertex Veo and Gemini Omni operations, Runway tasks, FAL queues,
    Kling tasks, and xAI generation/private-file lifecycles as durable provider
    operations with exact provider-handle recovery.
  - Preserve GCS/object-store staging, xAI retained-file authentication and
    cleanup evidence, observed usage, and every current recoverable artifact.
  - Make `xai` the only xAI selector supplied by the canonical catalog after
    the schema-v6 xAI persisted-route migration.
  - Register exact import validators for selected recoverable Vertex, Gemini
    Omni, Runway, FAL, Kling, and xAI video records. Imported records must enter
    the canonical operation schema without submitting new provider work.
  Deliverables:
  - Add all video adapters, operation schemas, capability/price entries,
    recovery paths, official-client types, fixture servers, and docs.
  Validation:
  - Complete the P011 second-provider acceptance procedure for each protocol adapter added or changed in this slice.
  - Prove every text, keyframe, reference, extension, and source-video mode,
    long polling across restart, uncertain transport recovery, output hash and
    MIME validation, and provider resource cleanup.
  - Run the existing minimal opt-in live canaries through LLM Proxy after the
    fake provider suite and repository CI pass.
  - Start with the required failing integration test. Complete validation under the current repository policy.
  Delivery boundary:
  - Execute provider slices in this order: Runway, Vertex, FAL, Kling, then xAI.
  - F043 gates only routes that require staging. Accept independent routes before the complete F025 scope closes.
  - Implement and release provider slices with explicit acceptance and removal receipts.
  - Keep each current capability available under its single owner until its verified cutover.
  - Add import validators only for records identified by the migration inventory.
  - Use the current repository validation policy instead of older baseline CI instructions.

- [ ] [F026] (P1) Add ElevenLabs speech, music, and alignment operations.
  Current ownership and handoff:
  - Close this capability issue after destination source validation.
  - Record consumer cutover under its MediaOps issue and final retirement under I244.
  - LLM Proxy owns native provider calls, credentials, resource APIs, uploads, and provider recovery for this capability.
  - Deliver the public contract and official client methods before MediaOps I011 switches its service integration.
  - Keep product jobs, application assets, and local media processing in one MediaOps backend.
  - Keep TelePrompter browser controls connected to MediaOps.
  - Preserve detailed requirements and dated implementation evidence below.
  - Record source qualification separately from consumer activation and paid acceptance.
  Progress (2026-09-20): One YAML provider now defines shared credentials, account verification, model metadata, and subscription quotas.
  Progress: The gateway and both official clients expose typed account resources through the assigned connection.
  Validation: Public HTTP tests cover a second provider identity, account capacity, credential replacement, tenant separation, and malformed native responses.
  Validation: The initial test reported `catalog must contain one ElevenLabs provider, found 0`.
  Validation: The client test rejected the initial decoder because incomplete model metadata was accepted. The corrected resource tests pass.
  Validation: CI exposed stale authentication and documentation assertions, plus the capacity defect in B239. All corrections pass.
  Validation: Final CI passes all 14 gates with 100.0 percent Go coverage, 145 frontend tests, and seven real-server browser tests.
  Evidence: `/tmp/llm-proxy-b239-f026-ci-final.log` records the account resource checkpoint. It does not close the remaining F026 scope.
  Progress: F079 completes the native forced-alignment service through the same provider and account connection.
  Progress: Voice discovery now preserves native pages, filters, metadata, and private account-bound preview access.
  Validation: Voice discovery initially returned `400 media_voice_invalid`. The new public page and preview tests pass.
  Progress: The common page contract preserves Dictator discovery and retained extracted voices.
  Validation: The voice checkpoint passes all 14 CI gates with 100.0 percent Go coverage, 146 frontend tests, and seven service-backed browser tests.
  Evidence: `/tmp/llm-proxy-f026-voices-verified-ci.log` records the final voice checkpoint.
  Contracts: Voice collections use typed queries and page results. Owned voice previews use the common voice resource.
  Progress: Dictionary creation now uses the shared service branch with alias and phoneme rules, private references, and saved native recovery evidence.
  Validation: Public dictionary tests cover both provider identities, storage failures, cancellation, restart, and operation expiry.
  Validation: The dictionary checkpoint passes all 14 CI gates, 147 frontend tests, and seven service-backed browser tests.
  Evidence: `/tmp/llm-proxy-f026-dictionaries-final-ci.log` records 100.0 percent Go coverage.
  Evidence: `/tmp/llm-proxy-f026-dictionaries-initial.log` records the initial `400 media_operation_invalid` result.
  Progress: Both source conversion models now use the existing provider, connection, and operation gateway.
  Progress: The YAML declares all 28 source formats and eight conversion controls. Raw audio includes a separate interpretation artifact.
  Validation: Public tests verify both models, a renamed provider, exact multipart requests, account authority, cancellation, and restart behavior.
  Evidence: `/tmp/llm-proxy-f026-conversion-initial.log` records the initial `400 media_operation_invalid` result.
  Validation: Initial CI rejected the new discovery codec and a fixture that removed existing model offerings. Both focused corrections pass.
  Progress: All four source speech models now use the same provider and typed native-speed or text-pacing transports.
  Progress: Speech requests preserve owned dictionary and continuity references, seed, normalization, voice settings, formats, and timestamps.
  Validation: Native limits reject a Multilingual v2 language override and Eleven v3 similarity or speaker boost. The catalog corrects those source flags.
  Validation: Public tests cover both provider identities, all source pacing intervals, invalid context, malformed timestamps, cancellation, and uncertain recovery.
  Evidence: `/tmp/llm-proxy-f026-generation-initial.log` records the initial `422 media_operation_unavailable` result.
  Evidence: `/tmp/llm-proxy-f026-generation-openapi-initial.log` records the missing public request schema.
  Validation: The conversion Go checkpoint reached 100.0 percent coverage. Corrected browser checks now select all six source models.
  Validation: Final combined speech and conversion CI passes all 14 gates with 100.0 percent Go coverage.
  Validation: All 148 frontend tests and seven service-backed browser tests pass. Python passes 116 client and five packaging tests.
  Validation: The account browser test expected zero ElevenLabs models. The corrected test checks all six models and a saved speech default.
  Evidence: `/tmp/llm-proxy-f026-speech-verified-ci.log` records the final combined checkpoint.
  Contracts: Both speech capabilities use the existing media operation API. Native audio, format descriptions, and optional timestamps use owned artifacts.
  Remaining: Voice-library operations, history, all seven music methods, imports, and consumer acceptance remain open.
  Files: `configs/providers.yml`, `internal/proxy/provider_metadata.go`, `pkg/llmproxycontract/provider_metadata.go`, both official clients, OpenAPI, and public integration tests.
  Contract: `GET /model/v1/provider-resources/{provider}/{kind}` adds typed `metadata` and `quotas` resources.
  Scope clarification (2026-09-20): Include every source method listed in `docs/media-provider-completeness.md`.
  Requirements: Include pronunciation dictionary creation, model and subscription metadata, voice-library search, and voice-library import.
  Requirements: Include all four speech models, both conversion models, and all seven `music_v1` operations.
  Requirements: Use canonical provider connections and the shared YAML catalog for all supported routes and resources.
  Goal:
  Move ElevenLabs provider access into LLM Proxy. Keep narration plans and local audio assembly in MediaOps.
  Cross-repository sequence:
  - MediaOps I011 owns the provider cutover and acceptance through affected MediaOps callers for this audio slice.
  Requirements:
  - Apply the provider catalog and protocol adapter contract in P011 before each capability release.
  - Add typed operations for speech generation, speech conversion, voice
    discovery, history listing/download, prompt music, composition plans,
    detailed and streamed composition, composition upload, video-to-music,
    stem separation, and forced alignment.
  - Preserve exact voice/model settings, pronunciation dictionaries,
    continuity context, timestamps, seed, normalization, pacing/speed
    translation, formats, provider concurrency, and history identifiers.
  - Keep render plans, narrative cadence, chunk reuse, stitching, and final composite validation in MediaOps.
  - I286 defines the model access boundary. Keep workflow execution in MediaOps and timeline controls in TelePrompter.
  - Represent each provider request as one durable gateway operation.
  - Materialize provider audio and JSON outputs as typed artifacts and retain
    history or song identifiers as internal recovery evidence.
  - F042 owns Dictator as a private gateway provider. This issue owns ElevenLabs capabilities.
  - Register exact import validators for selected ElevenLabs history and
    provider-operation records and return canonical operation mappings without
    replaying generation or mutation.
  Deliverables:
  - Add the ElevenLabs credential profile, adapters, catalogs, prices,
    official-client types, recovery handlers, fake upstream fixtures, and docs.
  Validation:
  - Complete the P011 second-provider acceptance procedure for each protocol adapter added or changed in this slice.
  - Prove all read-only, paid, upload, streaming, and recovery paths; bounded
    concurrency; cancellation; exact output order; and artifact integrity.
  - Run explicitly enabled minimal ElevenLabs live acceptance only after the
    fake provider suite and repository CI pass.
  - Start with the required failing integration test. Complete validation under the current repository policy.
  Delivery boundary:
  - Separate generation, retained-resource operations, and music into bounded implementation slices when their contracts permit it.
  - Keep each migrated capability on one execution path and preserve current consumer behavior.
  - Add import validators only for actual recoverable records.
  - Use the current repository validation policy instead of older baseline CI instructions.

- [ ] [F027] (P1) Add provider account mutations, avatars, translation, and lip-sync.
  Current ownership and handoff:
  - Close this capability issue after destination source validation.
  - Record consumer cutover under its MediaOps issue and final retirement under I244.
  - LLM Proxy owns native provider calls, credentials, resource APIs, uploads, and provider recovery for this capability.
  - Deliver the public contract and official client methods before MediaOps I012 switches its service integration.
  - Keep product jobs, application assets, and local media processing in one MediaOps backend.
  - Keep TelePrompter browser controls connected to MediaOps.
  - Preserve detailed requirements and dated implementation evidence below.
  - Record source qualification separately from consumer activation and paid acceptance.
  Scope clarification (2026-09-20): Include every HeyGen and Kling source capability listed in `docs/media-provider-completeness.md`.
  Goal:
  Complete gateway ownership of external media-provider credentials and
  provider-native task recovery for HeyGen and Kling account operations.
  Cross-repository sequence:
  - MediaOps I012 owns the provider cutover and acceptance through affected MediaOps callers for this account-operation slice.
  Requirements:
  - Apply the provider catalog and protocol adapter contract in P011 before each capability release.
  - Add typed operations for HeyGen translation, existing-video lip-sync,
    photo-avatar creation, motion enhancement, avatar-video generation, quota
    reads, and Kling/HeyGen lip-sync where currently supported.
  - Add typed Kling reusable-asset create/list/get/delete operations with exact
    mutation and destructive classifications.
  - Require immutable accepted requests and idempotency for paid and mutating work. Preserve
    provider task ids internally and expose only gateway operation or asset ids.
  - Use gateway assets for all uploaded image, audio, video, and character
    inputs and materialize terminal video outputs through the artifact contract.
  - Preserve provider quota and observed-usage evidence separately from the
    published price catalog.
  - Register exact import validators for selected HeyGen and Kling task and
    reusable-asset records. Import must preserve ownership and terminal state
    without replaying paid, mutating, or destructive work.
  Deliverables:
  - Add HeyGen and Kling operation adapters, catalogs, prices, recovery,
    official-client types, provider fixtures, and docs.
  Validation:
  - Complete the P011 second-provider acceptance procedure for each protocol adapter added or changed in this slice.
  - Prove read-only versus paid versus mutating versus destructive behavior,
    duplicate idempotency, restart recovery, asset ownership, quota reporting,
    and sanitized errors through public black-box tests.
  - Run explicitly enabled minimal live acceptance only after the fake provider
    suite and repository CI pass.
  - Start with the required failing integration test. Complete validation under the current repository policy.
  Delivery boundary:
  - Release provider and resource slices with exact mutation authority and consumer cutover evidence.
  - Use F043 staging only for provider routes that require it.
  - Add import validators only for actual recoverable records.
  - Use the current repository validation policy instead of older baseline CI instructions.

- [ ] [F017] (P1) Add shared MPR UI inactivity warning and automatic logout.
  Goal:
  Make an authenticated browser session warn and sign out explicitly after
  bounded user inactivity, before its TAuth session can expire behind a stale
  application snapshot. Implement the behavior once in MPR UI and consume that
  same current contract from llm-proxy and LoopAware.
  Evidence:
  - An unattended llm-proxy Usage view can retain MPR UI's last authenticated
    state and the last accepted workspace data after the server session has
    expired. As a result, returning to the tab presents stale data. A full
    reload then restores the authoritative signed-out first screen.
  - llm-proxy currently refreshes Usage only during authenticated-workspace
    startup, scope/interval changes, or explicit Refresh. It receives MPR UI
    authentication events but has no independent authority to inspect,
    refresh, or terminate the TAuth session.
  - LoopAware already proves the desired user flow: its dashboard warns after
    60 seconds of inactivity, signs out after 120 seconds, responds to activity,
    renders theme-aware controls, and has browser coverage for warning,
    dismissal, forced logout, persistence, layout, and logout failure.
  - The LoopAware implementation is application-owned inline JavaScript and
    Bootstrap markup. It directly calls TAuth logout/refresh endpoints, stores
    per-user timeout preferences, uses high-frequency interval and pointer
    activity, and has no cross-tab coordinator. Copying it into llm-proxy would
    create a second browser authentication owner and violate the shared
    MPR UI/TAuth integration contract.
  - I033 proposed 60-second foreground Usage polling for the stale-snapshot
    symptom. Product direction now selects explicit inactivity warning/logout.
    Active users retain the existing manual Refresh path.
  Requirements:
  - Implement the inactivity state machine, warning surface, and logout
    transaction in MPR UI first. Publish it through the canonical literal
    `mpr-ui@latest` asset and adopt that same implementation in llm-proxy and
    LoopAware. Neither application may copy the manager, call TAuth
    login/session/refresh/logout endpoints, or infer authentication from
    management API failures.
  - Extend the strict browser configuration contract with optional
    `auth.autoLogout`. Presence enables the feature and requires exactly two
    positive integer fields: `promptAfterSeconds` and `logoutAfterSeconds`,
    with logout strictly later than the prompt. Reject missing, unknown,
    non-integer, non-positive, or misordered values; do not default, clamp,
    alias, or infer them.
  - Configure both applications with the current LoopAware policy: warn at 60
    seconds and attempt logout at 120 seconds. Keep that logout deadline below
    every environment's TAuth session TTL. Any later policy change must update
    the explicit runtime configuration rather than browser storage.
  - Run the manager only while MPR UI is authoritatively authenticated. Start
    one lifecycle after authentication, and remove every listener, scheduled
    deadline, warning surface, coordinator, and pending callback on logout,
    authentication reset, controller teardown, or configuration failure.
  - Calculate prompt and logout behavior from one last-activity timestamp and
    scheduled deadlines rather than a polling interval. Count only trusted,
    intentional user input; throttle noisy input. Synthetic events,
    timer ticks, network responses, focus, and visibility changes are not
    activity. A return to visibility must immediately reconcile the existing
    deadline so browser timer suspension cannot extend the policy.
  - Coordinate activity, warning dismissal, and logout across same-origin tabs
    for the configured TAuth tenant. Store or broadcast only the minimum
    non-secret timing/coordination state; never include identity, tokens,
    cookies, session material, profile data, or application payloads. One user
    action in any participating tab renews the shared deadline, and a deadline
    produces one deduplicated logout transaction and one terminal transition
    across tabs.
  - Render one MPR UI-owned, theme-aware, responsive, keyboard-operable warning
    with a semantic countdown and the actions `Stay signed in` and `Sign out
    now`. Move focus intentionally, restore it when the user stays signed in,
    announce state changes without repeated timer spam, and honor reduced
    motion. Applications may not supply Bootstrap-specific warning markup,
    CSS, copy, or overlay behavior.
  - Route manual and inactivity logout through one MPR UI operation. Deduplicate
    concurrent requests. On successful TAuth logout, clear the restore hint and
    profile, emit the canonical status and unauthenticated events with stable
    reason `inactivity` for this path, remove protected UI, and redirect once
    through the configured login path.
  - If the logout request reports that the TAuth session is already absent,
    reconcile through MPR UI's canonical session operation and complete the
    unauthenticated transition only when that operation confirms it. For a
    transport/server failure while the session remains authenticated or cannot
    be authoritatively classified, keep the authenticated state, show a
    persistent retry/error action, and do not redirect or emit a false
    unauthenticated event.
  - Project the exact configuration through llm-proxy's generated
    `/config-ui.yaml`. Continue using the existing MPR UI unauthenticated event
    path to cancel/invalidate pending workspace requests and clear tenants,
    providers, generated credentials, usage state, notices, and protected DOM
    content. Do not add a Usage polling scheduler, last-updated contract, or
    application-owned session timer.
  - Replace LoopAware's implementation forward-only: delete its inline session
    timeout manager, warning/banner/overlay ownership, direct browser auth
    requests, settings toggle and duration fields, per-email localStorage
    preference and migration, test globals, and feature-specific CSS after the
    shared MPR UI behavior is adopted. Do not retain a compatibility read or
    dual path.
  - Retire I033 before implementation and preserve explicit Refresh as the
    active-session Usage freshness action. If foreground freshness remains a
    demonstrated need after F017 is deployed, record it as a new problem with
    fresh evidence rather than reviving the superseded polling specification.
  - Update MPR UI integration/configuration documentation plus llm-proxy and
    LoopAware authentication/user documentation. State the exact policy,
    activity semantics, cross-tab behavior, failure behavior, and distinction
    between inactivity logout and the authoritative TAuth session TTL.
  Deliverables:
  - One MPR UI inactivity controller and accessible warning surface, one strict
    `auth.autoLogout` configuration contract, one cross-tab coordination
    contract, and one shared manual/automatic logout transaction.
  - Exact llm-proxy runtime configuration and complete protected-state cleanup
    on the canonical unauthenticated event.
  - Exact LoopAware runtime configuration with the obsolete app-owned
    inactivity/authentication implementation and persisted preferences removed.
  - Updated OpenAPI/browser-config, MPR UI event, and repository documentation;
    no compatibility shim, application-specific auth helper, or production
    deployment.
  Validation:
  - In MPR UI, use controlled time and visibility to cover authentication
    start/reset, activity before/after warning, countdown, stay-signed-in,
    sign-out-now, automatic deadline, background timer suspension, teardown,
    reduced motion, focus restoration, keyboard/screen-reader semantics, and
    strict configuration rejection.
  - Cover multiple tabs proving shared activity and dismissal, one logout
    transaction, one redirect/unauthenticated transition, tenant/origin
    isolation, stale coordinator recovery, and absence of identity/session
    material from coordination state.
  - Cover logout success, already-expired reconciliation, unauthorized session,
    transport/server failure, retry, and concurrent manual/automatic logout
    without false success, duplicate request, redirect, or event emission.
  - In both applications, run browser black-box scenarios with real MPR UI and
    TAuth boundaries. Prove warning and logout at 60/120 seconds, protected
    state removal, no post-logout stale response mutation, successful
    reauthentication, and absence of app-owned auth requests, timers,
    preferences, and obsolete warning DOM.
  - For each code-changing repository, run its required baseline `make ci`
    immediately before the first edit and final `make ci` after the last edit.
    Do not contact or deploy production as part of implementation acceptance.
- [ ] [F016] (P1) Add an installable Node.js client for canonical v2 messages.
  Goal:
  Let server-side Node.js applications install `llm-proxy-client`, create one
  validated client from an application-supplied LLM Proxy base URL and tenant
  secret, and await canonical `/v2` message requests without duplicating
  authentication, URL, model-profile, timeout, or error-handling logic.
  Evidence:
  - `pkg/llmproxyclient` is the reusable Go client, `python/llm_proxy_client` is
    the installable Python package, and `llm-proxy-client` is the standalone Go
    CLI. Their public tests exercise the same v2-only messages transport.
  - The root npm project is private frontend/test tooling. The repository has
    no installable Node.js package, Node import surface, package declaration,
    or packed-consumer validation.
  - `docs/openapi.yaml` is the sole public HTTP contract, and repository CI
    already uses Node.js 22. The Node package can therefore use the built-in
    Fetch API without adding a runtime dependency or another wire schema.
  Requirements:
  - Add exactly one package project under `node/`, named
    `llm-proxy-client`, initially versioned `0.1.0`, with
    `"type": "module"`, a strict public `exports` map, and
    `"engines": {"node": ">=22"}`. Keep the root frontend package private and
    separate; do not add alternate package names, scopes, entrypoints, or
    registry fallbacks.
  - Author the runtime directly as vanilla ESM JavaScript with `// @ts-check`,
    complete JSDoc, descriptive identifiers, immutable validated values, and
    no runtime dependencies. Generate one TypeScript declaration surface from
    that source for consumers. Do not add a CommonJS build, dual-package
    condition, transpiled runtime copy, browser bundle, provider SDK, or legacy
    prompt/GET client.
  - Export only the canonical public surface: `Client`, `ClientConfig`,
    `ClientMessage`, `ClientMessagesRequest`, `LLMProxyClientError`,
    `LLMProxyModelProfileError`, `LLMProxyHTTPError`, and
    `LLMProxyTransportError`. `Client.postMessages(request, {signal})` returns
    `Promise<string>`; the constructor accepts an explicitly injected Fetch
    implementation or uses Node's built-in `globalThis.fetch`.
  - Validate configuration and request input exactly once at their constructors.
    Require an absolute HTTP(S) base URL and nonblank tenant secret; accept an
    optional provider. Require at least one message and one `user` message,
    allow only `system`, `user`, and `assistant` roles with nonempty content,
    and require optional `order` values to be all-or-none, unique,
    non-negative integers. Accept only optional `model`, `webSearch`,
    `maxTokens`, `reasoningEffort`, and `requestTimeoutSeconds` values matching
    the canonical v2 contract; a request timeout is a positive whole number.
  - Build the request with the standard `URL` API. Append `/v2` exactly once,
    replace `key` and `format`, preserve unrelated query values, preserve a
    base-URL provider unless the validated config explicitly overrides it, and
    remove body-owned query fields. Send `format=text/plain`,
    `Accept: text/plain`, `Content-Type: application/json; charset=utf-8`, and
    the exact canonical JSON body. Omit `model`, `max_tokens`, and
    `reasoning_effort` when not selected; serialize `web_search` as a boolean.
  - Serialize `requestTimeoutSeconds` only as
    `X-LLM-Proxy-Request-Timeout-Seconds`. Add no client-owned total-response
    deadline, retry, polling, streaming, or fallback transport. A supplied
    `AbortSignal` is the caller's independent cancellation authority.
  - Match the current application-user model-profile contract. A configured
    `modelProfilePath` requires one application-injected asynchronous text
    reader; reread and strictly decode its complete JSON document before every
    request. Accept exactly one nonblank `provider` and `model` string, reject
    unknown or duplicate fields, and reject profile mode combined with a
    configured/base-URL provider, base-URL model, or request model. A read,
    decode, validation, or conflict failure must stop before Fetch and must
    never reuse a prior profile, tenant default, or alternate source.
  - Return successful response text unchanged. Map every non-2xx response to
    `LLMProxyHTTPError` with status, response body, status text, and bounded
    provider/model/request-timeout context. Map network failures and caller
    cancellation to `LLMProxyTransportError` while retaining the original
    error as `cause`. No error name, message, field, cause wrapper, log, or
    package example may expose the tenant secret, authenticated URL, request
    messages, or response text from another request.
  - Keep the package server-side only. Do not read environment variables,
    dotenv files, service `config.yml`, browser storage, TAuth state, or
    upstream provider keys. Applications supply configuration and secrets
    explicitly; the package never sends a provider API key.
  - Add repository-owned package lint, declaration, black-box test, pack, and
    temporary-consumer install targets to the root Makefile and `make ci`.
    Extend CI path filters for `node/**`. Package only the ESM runtime,
    declarations, package README, and MIT license through an explicit `files`
    allowlist; exclude tests, coverage, repository tooling, and local files.
  - Produce one deterministic `npm pack` tarball and an explicit
    operator-owned npm publication command. Validate package name, version,
    contents, registry target, and an unpublished version before mutation.
    CI and implementation work use `npm publish --dry-run` only; no PR,
    `make ci`, deploy, or implicit release step may publish externally, and
    there is no alternate registry/package-name fallback.
  - Add Node.js installation and ESM/TypeScript usage to the package README and
    root README. Update `CHANGELOG.md`,
    `docs/implementation/provider-routing-plan.md`, the client-authentication
    documentation, and the generated Clients resource family with one
    Node.js-client page and examples. Do not change the public HTTP contract to
    accommodate the client; prove the package conforms to the existing
    `docs/openapi.yaml`.
  Deliverables:
  - One installable zero-runtime-dependency Node.js ESM package with generated
    declarations, validated public request/config types, injectable Fetch
    transport, model-profile support, and stable typed errors.
  - Root Makefile/CI integration, packed-consumer validation, deterministic
    package artifact, and explicit operator-owned publication path.
  - Updated product, client-authentication, provider-routing, package, and
    generated public documentation for the Node.js integration.
  Validation:
  - Pack the package, install only that tarball into disposable JavaScript and
    TypeScript consumer projects, import only its public export, compile the
    typed example, and make real requests through a loopback HTTP server.
    Never validate by importing unpublished source paths.
  - Through the installed public client, prove exact method, `/v2` path,
    authentication/format/provider query behavior, unrelated-query
    preservation, body-field stripping, headers, Unicode messages, explicit
    ordering, optional-field omission, response text, and conformance with the
    canonical OpenAPI request and documented response statuses.
  - Cover every configuration/message/request invariant; provider
    preserve/override behavior; profile reload after atomic replacement;
    malformed, duplicate, incomplete, unreadable, and conflicting profiles;
    timeout-header omission/presence; caller abort; transport failure; every
    documented non-2xx status; and proof that one call occurs with no hidden
    retry or client deadline.
  - Assert HTTP and transport errors preserve their typed fields and cause
    while their string/object representations exclude the tenant secret,
    authenticated URL, request content, and unrelated response state.
  - Verify `npm pack --dry-run` and `npm publish --dry-run` contain only the
    allowlisted files, use exact `llm-proxy-client@0.1.0` metadata, require
    Node.js 22 or newer, expose only ESM plus declarations, and leave no packed,
    installed, credential, or coverage artifacts in the worktree.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair for the implementation, with
    the final run after the last code edit.
- [ ] [F021] (P1) Add OAuth-authenticated MCP access with tenant discovery.
  Goal:
  An authenticated user can connect one remote MCP client to all owned tenants.
  The client obtains the tenant list and selects one tenant for each operation.
  The server uses the selected tenant's saved provider credentials, routing defaults,
  and standard text-generation lifecycle for each MCP request.
  Current contract:
  - Public proxy requests use a generated tenant secret in `key=...`.
  - TAuth browser sessions authorize management operations.
  - `GET /api/management/account` provides the user and summaries of all owned tenants.
  - Each summary contains `id`, `name`, `has_secret`, `created_at`, and `updated_at`.
  - The account handler can create a user and a default tenant during account setup.
  - Provider credentials remain on the server and belong to one tenant.
  - TAuth provides the OAuth authorization-server contract that a remote MCP
    client requires.
  - The gateway generates the root OAuth block and the tenant OAuth policy.
  - The deployment manifest declares the protected API origin and the
    `llm-proxy:use` scope.
  Requirements:
  - Serve MCP revisions `2026-07-28`, `2025-11-25`, `2025-06-18`, and `2025-03-26` at the exact resource URL `https://llm-proxy-api.mprlab.com/mcp`.
  - Accept the Codex `2025-06-18` initialization flow and use SDK protocol negotiation.
  - Maintain a dated agent protocol map with client versions, evidence types, and source links.
  - Use the official Go MCP SDK in stateless Streamable HTTP mode. Return JSON responses.
  - Reject unsupported explicit protocol headers. Negotiate unsupported handshake offers with a supported revision.
  - Keep the endpoint stateless across both protocol families. Use Streamable HTTP without a standalone SSE endpoint.
  - Use `https://llm-proxy-api.mprlab.com` as the OAuth protected-resource
    identifier and access-token audience.
  - Publish path-specific OAuth Protected Resource Metadata at
    `/.well-known/oauth-protected-resource/mcp`.
  - Return an RFC-compliant Bearer challenge for an unauthenticated MCP
    request. Include the protected-resource metadata URL in the challenge.
  - Validate each JWT access token signature, issuer, subject, exact audience,
    expiry, and required `llm-proxy:use` scope at the HTTP edge.
  - Use the TAuth subject as the account identity for tenant discovery and ownership checks.
  - Permit the OAuth grant to access all tenants that the account currently owns.
  - Confirm current ownership before each operation that selects a tenant.
  - Give the same sanitized error for a missing tenant and a foreign tenant.
    For tool calls, use `isError: true` with the canonical not-found classification.
    For resource reads, use the same MCP resource-not-found error for both cases.
  - Require an explicit `tenant_id` for generation and route discovery.
  - Keep tenant selection in each request. Do not store an active tenant in an MCP connection.
  - Keep provider credentials on the server. Never return provider credentials,
    tenant secrets, access tokens, refresh tokens, or session data through MCP.
  - Add `llm_proxy.list_tenants` with an empty input object and a structured `tenants` array.
  - Include all owned tenants, including tenants without provider configuration.
  - Use the same summary fields and ownership query as `GET /api/management/account`.
  - Extract a shared read operation for tenant summaries. Keep account setup outside MCP discovery.
  - If the account has no tenants, provide an empty array without account or tenant creation.
  - Mark tenant discovery as read-only, not destructive, idempotent, and closed-world.
  - Keep the existing REST account endpoint under TAuth session authentication.
  - Require only the OAuth bearer token and `llm-proxy:use` scope for MCP discovery.
  - Add `llm_proxy.generate_text`. Require `tenant_id` and `messages`.
    Accept optional `provider`, `model`, `web_search`, `max_tokens`,
    `reasoning_effort`, and `request_timeout_seconds` inputs.
  - Use the canonical `/v2` message and attachment contract for the tool.
    Preserve ordered image and audio attachments on user messages.
  - Return generated text and structured `request_id`, `provider`, `model`,
    `usage`, and `request_timeout_seconds` fields from a successful tool call.
  - Mark the generation tool as not read-only, not destructive, not idempotent, and
    open-world because one call can create provider charges.
  - Return a sanitized MCP tool error with `isError: true` for an accepted tool
    call that fails. Preserve the canonical proxy error classification without
    exposing an upstream body, provider message, prompt, response, or secret.
  - Add the resource template `llm-proxy://tenants/{tenant_id}/routes` for route discovery.
  - Include only the selected tenant's configured text routes, defaults, and declared capabilities.
  - Keep tenant mutations, credential management, and dictation outside this first MCP contract.
  - Extract one transport-neutral text-generation service from the current
    HTTP handlers. Make `/v2` and MCP use the same routing, admission, queue,
    rate-limit, timeout, cancellation, continuation, error, and usage logic.
  - Record MCP usage with endpoint value `mcp`. Record the logical proxy result
    status when an MCP tool error uses a successful HTTP transport response.
  - Add one `Copy MCP URL` action for the authenticated account.
  - Permit discovery before provider setup. Explain that generation requires a configured text route in the selected tenant.
  - Use only `/mcp`. Do not implement `/mcp/{tenant_id}` or the previous `llm-proxy://routes` resource.
  - Document the MCP URL, OAuth flow, tool schema, resource schema, client
    configuration, security boundary, and provider-key requirement.
  - Add the MCP route and OAuth metadata to OpenAPI, runtime configuration, and
    the repository deployment capability contract.
  - Keep local acceptance separate from live deployment acceptance. Do not
    contact or change production during implementation.
  Deliverables:
  - Add the stateless MCP transport and the path-specific protected-resource
    metadata endpoint.
  - Add strict OAuth bearer-token validation and tenant ownership checks.
  - Add the shared tenant query and `llm_proxy.list_tenants` tool.
  - Add the shared text-generation service and `llm_proxy.generate_text` tool with explicit tenant selection.
  - Add the resource template `llm-proxy://tenants/{tenant_id}/routes`.
  - Add account UI copy, configuration, OpenAPI, deployment, and user-guide
    updates for the MCP connection contract.
  - Add black-box integration and browser coverage through public entry points.
  Validation:
  - Run an official MCP client against the real local HTTP route, real local
    TAuth OAuth flow, and a fake upstream provider.
  - Verify successful authorization-code and PKCE login, token refresh, token
    revocation, and consent behavior.
  - Verify missing, malformed, expired, wrong-issuer, wrong-audience, and
    wrong-scope tokens. Verify missing and foreign tenant identifiers.
  - Compare MCP tenant summaries with the REST account response for the same existing account.
  - Verify discovery with two owned tenants and another account's tenant.
  - Verify that discovery includes tenants without configured routes and excludes every foreign tenant.
  - Verify that an empty result creates no account, tenant, secret, provider request, or generation usage event.
  - Use one connection and OAuth grant to generate text through each owned tenant.
  - Verify each tenant's credentials, defaults, route resource, and usage attribution.
  - Verify that concurrent calls to different tenants cannot change another call's tenant selection.
  - Verify missing `tenant_id`, tenant deletion after discovery, and direct access to a foreign tenant.
  - Verify that repeated discovery reflects tenant creation, renaming, and deletion through the management API.
  - Verify MCP discovery without a browser cookie or tenant secret.
  - Verify the account copy action before provider setup and after tenant creation.
  - Verify exact tool discovery, input-schema validation, route selection,
    defaults, media inputs, structured success output, and route-resource data.
  - Verify queue rejection, provider rate limits, request timeouts, caller
    cancellation, sanitized tool errors, and managed usage records.
  - Verify that logs and errors contain no provider credential, tenant secret, token, prompt, or generated response.
  - Verify that MCP results contain no credentials or tokens. Permit generated text only in successful generation results.
  - Verify that OAuth tokens appear only in the authorized token response and client storage.
  - Run `/v2` regression scenarios to prove one shared execution lifecycle and
    unchanged tenant-secret authentication for the REST contract.
  - Use MCP Inspector and one supported remote MCP client for manual local
    acceptance. Record live-host acceptance as a separate deployment result.
  - Obey `.mprlab/POLICY.md` for focused validation and the final `make ci` checkpoint.
  Scope update: 2026-09-07 — Tenant discovery uses one account connection and explicit tenant selection for each operation.
  Scope update: 2026-09-08 — The operator authorized multiple MCP revisions, including Codex support for `2025-06-18`.
  This requirement replaces the previous single-version restriction for this endpoint.
  A live retry after TAuth B077 completed Google login and OAuth authorization.
  Codex 0.153.4 then received HTTP `400` on its `initialize` request.
  A local capture confirmed its `2025-06-18` offer without a protocol header.
  The new handshake tests first failed with `initialize version="": HTTP 400 want 200` for all three supported handshake revisions.
  The SDK then replaced `no-store` with `no-cache`, which failed the response-policy assertion.
  The response writer now preserves `no-store` across both protocol families.
  An initial Codex harness assertion expected `ready`. The actual successful client state is `connected`.
  The corrected harness passed tenant discovery, route reads, and generation with a local TAuth-issued token.
  OpenCode 1.18.28 now passes local connection and tool discovery.
  The agent protocol map distinguishes measured offers, installed SDK declarations, and vendor documentation.
  Documentation update: 2026-09-08 — Expanded the client map with Antigravity, Grok, Z.AI, and other researched clients.
  Added the server support table, source links, result limits, and required capture procedure.
  Current March 2025 client demand remains unverified. The documented server implementation still includes that revision.
  Documentation validation: Governor checks, local Markdown links, and whitespace checks passed.
  Reviewed the changed prose. The two MCP documents have no mechanical language findings.
  The 79 findings in unchanged tracker text remain outside this documentation update.
  The first full CI run reported one uncovered block at `mcp.go:155` in an unused response-writer method.
  Removed that method. Protocol and cache behavior remain covered through HTTP requests.
  Final validation for expanded support: `make ci` passed all 12 gates in 339 seconds with 100% Go coverage.
  The browser suite passed 114 tests. Both local TAuth black-box tests passed.
  The separate `make test-mcp-codex` run passed after the final source change.
  Governor normalization passed. Changed prose has no mechanical findings. The 318 findings in unchanged text remain outside this change.
  CI correction: 2026-09-08 — GitHub run 34286668186 rejected four uncovered completion-client error paths.
  Added public-client cases for structured requests, missing model profiles, missing resolved models, and duplicate token headers.
  The focused completion tests passed. Final `make ci` passed all 12 gates in 309 seconds with no uncovered blocks.
  The browser suite passed 114 tests. Both TAuth black-box tests passed.
  Production deployment and live acceptance of this expanded protocol support remain pending.
  Implementation: 2026-09-08 — Added the account MCP endpoint, tenant discovery,
  generation tool, route resource, OAuth validation, and shared text service.
  Added the account copy action, local TAuth configuration, and API documentation.
  MCP usage preserves its tenant, logical status, and route after database reopen.
  Local checks passed with the official Go SDK v1.7.0 and MCP Inspector 2.5.0.
  The real TAuth flow passed PKCE login, consent denial and approval, refresh, and revocation checks.
  Public tests passed tenant isolation, concurrent selection, media, queue rejection,
  provider rate limits, timeout, cancellation, body limits, and sanitized failures.
  Go coverage reached 100% with no uncovered blocks.
  Earlier validation before the protocol scope update: `make ci` passed all 12 gates in 282 seconds.
  The browser suite passed 114 tests. Local OAuth and Inspector checks passed.
  Earlier acceptance before the scope update: OpenCode 1.18.28 reported `SSE error: Non-200 status code (405)`.
  The expanded protocol support passed local OpenCode and Codex acceptance.
  F021 remains open for deployment and live acceptance of the expanded support.
  I254 resolved the issue-format drift. The changed prose has no mechanical findings.
  Dependency handoff: 2026-08-15 — gateway F001 and both application manifests
  passed local contract validation. Production activation remains separate.
- [ ] [F028] (P2) {F027} Add HeyGen Avatar V as a gateway-owned avatar engine.
  Current ownership:
  - Implement the provider API and exact native specification in LLM Proxy.
  - MediaOps F022 owns backend consumption and TelePrompter product controls.
  - Keep this additional capability under I287, outside I274 and the retained-provider migration gate.
  Goal:
  Add the current Avatar V engine to the gateway HeyGen avatar contract for actual gateway consumers, including required TelePrompter flows.
  Cross-repository sequence:
  - MediaOps F022 qualifies required TelePrompter exposure after its I012 base HeyGen/Kling cutover.
  Requirements:
  - Add exact engine values `avatar_iv` and `avatar_v` to the HeyGen avatar-video
    operation and capability catalog.
  - Resolve the selected look through `GET /v3/avatars/looks/{look_id}` and
    require `supported_api_engines` to contain `avatar_v` before submitting an
    Avatar V operation.
  - Reject Avatar IV-only `motion_prompt` and `expressiveness` controls for an
    Avatar V plan and preserve the selected engine in the immutable intent.
  - Use the existing tenant-owned HeyGen credential profile, durable operation
    state, gateway assets, price evidence, and artifact recovery.
  Deliverables:
  - Add the provider adapter fields, catalog metadata, official-client types,
    HeyGen fixtures, docs, and an explicitly enabled paid live target.
  Validation:
  - Prove eligible success, ineligible pre-dispatch rejection, engine-specific
    control rejection, terminal artifact download, and restart recovery.
  - Start with the required failing integration test. Complete validation under the current repository policy.

- [ ] [F029] (P2) {F025} Add MiniMax H3 V2 video generation to model operations.
  Current ownership:
  - Implement the provider API and exact native specification in LLM Proxy.
  - MediaOps F023 owns backend consumption and TelePrompter product controls.
  - Keep this additional capability under I287, outside I274 and the retained-provider migration gate.
  Goal:
  Add the provider-qualified MiniMax H3 V2 route to the gateway for actual gateway consumers, including required TelePrompter flows.
  Cross-repository sequence:
  - MediaOps F023 qualifies required TelePrompter exposure after its I010 base video cutover.
  Requirements:
  - Add canonical provider `minimax`, exact model `MiniMax-H3`, and only the
    documented V2 create/query and `video_generation_input` upload contracts.
  - Support text, first/last frame, and reference image/video/audio roles with
    the documented mutual exclusions, media limits, 768P/2K resolutions,
    4..15-second duration, and exact aspect behavior.
  - Store provider task and upload ids in the durable operation, poll only the
    V2 task resource, materialize the terminal MP4 as a gateway artifact, and
    recover through the gateway operation id.
  - Publish the H3 price as unavailable until MiniMax publishes an exact rate.
    Preserve returned duration and media counts as observed usage.
  - Use one tenant-owned MiniMax credential profile and catalog-owned account
    concurrency.
  Deliverables:
  - Add the MiniMax adapter, catalog, official-client types, provider fixtures,
    recovery, docs, and an explicitly enabled minimal live target.
  Validation:
  - Prove exact payload roles, upload URI mapping, polling states, restart and
    uncertain recovery, input limits, artifact integrity, and absence of Hailuo
    V1 behavior through public black-box tests.
  - Start with the required failing integration test. Complete validation under the current repository policy.

- [ ] [F030] (P2) {F026} Add Speechify text-to-speech and voice discovery to model operations.
  Current ownership:
  - Implement the provider API and exact native specification in LLM Proxy.
  - MediaOps F024 owns backend consumption and TelePrompter product controls.
  - Keep this additional capability under I287, outside I274 and the retained-provider migration gate.
  Goal:
  Add the current Speechify complete-response speech and voice-discovery
  contracts to the gateway for actual consumers, including required TelePrompter narration flows.
  Cross-repository sequence:
  - MediaOps F024 qualifies required TelePrompter exposure after its I011 base audio cutover.
  Requirements:
  - Add canonical provider `speechify`, live `GET /v1/audio/models` and
    `GET /v1/voices` discovery, and complete-response `POST /v1/audio/speech`.
  - Admit only non-deprecated models that declare the speech endpoint and
    preserve exact model, language, voice, output format, billable character,
    request-id, and speech-mark metadata.
  - Apply the documented input and pagination limits, account concurrency,
    `Retry-After`, and provider error classification before returning a typed
    result.
  - Decode and validate returned audio into a gateway artifact and expose
    provider-tagged speech marks as a JSON artifact. Mark provider recovery
    unavailable when no durable retrieval handle exists.
  - Use one tenant-owned Speechify credential profile. Publish price
    unavailable until an exact official rate is cataloged.
  Deliverables:
  - Add Speechify adapters, live discovery projection, catalog entries,
    official-client types, fixtures, docs, and an explicitly enabled live
    discovery/minimal speech target.
  Validation:
  - Prove discovery, voice pagination, exact speech payload, audio decoding,
    speech marks, rate/concurrency handling, transport uncertainty, secret
    safety, and artifact integrity through public black-box tests.
  - Start with the required failing integration test. Complete validation under the current repository policy.

- [ ] [F020] (P2) {F016} Add route-validated sampling controls to the canonical v3 messages contract.
  Goal:
  Let a caller set low-level sampling controls only when the selected provider
  offering declares exact support for them.
  Requirements:
  - Use the authoritative catalog as the source for each provider offering's supported
    controls, numeric bounds, defaults, and incompatible control combinations.
  - Add one optional `sampling` object with exact fields `temperature`,
    `top_p`, `presence_penalty`, and `frequency_penalty`. Reject unknown
    fields, non-finite numbers, and values outside the selected route's bounds.
  - Make `POST /v3` the canonical messages operation. Update every first-party
    client, example, generated contract, and caller in the same forward-only
    change, then remove the obsolete `/v2` route and schema.
  - Resolve the provider and model route before validation. Reject a supplied
    control that the resolved offering does not support before an upstream
    request. Omit only controls that the caller did not supply.
  - Define the exact interaction between `sampling` and `reasoning_effort`
    for each offering in the capability catalog. Reject unsupported
    combinations without translation, clamping, or silent removal.
  - Map each accepted field through the provider-owned adapter and documented
    provider-native field. Keep provider-specific names and defaults at that
    adapter boundary.
  - Update the Go, Python, Node.js, and CLI clients to expose the same typed
    sampling object and canonical `/v3` request.
  Deliverables:
  - Add the strict `/v3` request schema, route-capability validation, provider
    mappings, official-client types, CLI flags, OpenAPI contract, and current
    documentation.
  - Remove the obsolete `/v2` route, schemas, examples, and first-party client
    calls after the forward migration.
  Validation:
  - Exercise the real public router against fake upstream providers. Prove each
    accepted control maps to the exact provider request and every invalid or
    unsupported control fails before upstream dispatch.
  - Prove absent controls remain absent, accepted zero values remain explicit,
    and supported sampling plus reasoning combinations preserve exact values.
  - Exercise all official clients and the CLI through `/v3`. Prove they expose
    the same typed failures and do not retain a `/v2` request path.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.

## Planning

- [ ] [P014] (P2) Assess the native provider contract for Seedance and HeyGen avatar use.
  Goal:
  Determine whether the proposed workflow has a supported provider API.
  Requirements:
  - Verify the exact model name and provider route from current official sources.
  - Verify whether HeyGen permits the proposed external video engine or requires a separate composition workflow.
  - Record supported controls, account access, resource ownership, and concrete limitations.
  - Supply the feasibility result to MediaOps P007.
  Deliverables:
  - A supported gateway contract proposal or a documented rejection of the unsupported integration.
  Validation:
  - Link current official evidence for each proposed native operation.
  Scope:
  - This Planning issue does not authorize implementation or block I274.

*do not implement yet*

- [!] [P012] (P1) Plan reliable Gemini access through independent customer connections.
  Goal:
  Eliminate impractical connection scenarios and select the best supported Gemini access for customers who connect and manage their providers independently.
  The result can support the same exact model through both AI Studio and Vertex AI as separate provider offerings.
  Requirements:
  - Assess the Gemini Developer API through AI Studio and Vertex AI against the same customer acceptance criteria.
  - Permit both providers when each offers reliable access through a practical customer connection flow.
  - Represent the same exact model through separate provider offerings, with explicit provider selection and independent qualification.
  - Use the existing API-key connection flow as the initial customer contract.
  - Specify how customers obtain a suitable key, connect a provider, validate access, select a model, and disconnect.
  - Require each customer to complete setup without credential-file installation or identity configuration by the proxy operator.
  - Exclude custom connection flows that require Application Default Credentials (ADC) JSON files, service-account JSON files, or credential-file uploads.
  - Exclude custom operator provisioning, server configuration edits, and operator-managed credential profiles from customer connection requirements.
  - Verify the complete Google key-acquisition process, including billing and access requirements, before accepting an API-key flow as practical.
  - Keep provider OAuth outside implementation scope unless research demonstrates a supported customer connection flow.
  - For any OAuth proposal, verify Google consent, required scopes, application verification, token renewal, reconnection, revocation, and tenant isolation.
  - Demonstrate the complete OAuth flow with a separate customer account before requesting implementation approval.
  - Treat operator-managed credential profiles as insufficient evidence of a customer OAuth flow.
  - Compare exact model IDs with equivalent inputs, reasoning levels, deadlines, and output limits.
  - Verify text, structured output, required media inputs, and the lifecycle of each required operation.
  - Separate quota, billing, credential restrictions, model availability, unsupported operations, and transient provider failures.
  - Define repeated acceptance runs and report success rates, latency, capacity limits, cost, and customer setup requirements.
  - Use current official sources and live evidence to recommend the supported provider offerings for the required Gemini capabilities.
  - Explain the customer benefit and connection requirements of each recommended offering.
  - Record excluded scenarios and the customer requirement that each scenario fails.
  - Specify the bounded schema and adapter changes that the recommended offerings require.
  - Reconcile F060 and the relevant F043 credential requirements with this decision before further authentication or migration implementation.
  - Keep P001 responsible for the shared provider connection interface.
  Evidence (2026-09-07):
  - The local and deployment dotenv files contained the same Gemini key.
  - That key returned HTTP 429 with zero free-tier quota for Gemini 3.1 Pro Preview.
  - The existing billed project had a different key, restricted to the Gemini Developer API.
  - That project key passed six direct high-reasoning requests across generateContent and Interactions.
  - The models were `gemini-3.1-pro-preview`, `gemini-3.8-flash`, and `gemini-3.5-flash-lite`.
  - Flash-Lite returned HTTP 400 for a background interaction because the model does not support that operation.
  - Vertex Express returned HTTP 401 for the local key and HTTP 403, `API_KEY_SERVICE_BLOCKED`, for the project key.
  - A suitable Vertex authorization key was absent during the initial probes. The later comparison below used an approved replacement key.
  - I252 passed direct Vertex requests with local user OAuth. Those results do not qualify the customer API-key flow.
  - These small direct probes do not establish sustained reliability, public proxy acceptance, or production acceptance.
  Deliverables:
  - A provider support decision that permits AI Studio, Vertex AI, or both according to customer acceptance evidence.
  - The customer flow, exact models, capability limits, and acceptance evidence for each recommended offering.
  - An explicit list of excluded connection scenarios and their failed customer requirements.
  - A repeatable qualification plan with explicit success criteria and unresolved external dependencies.
  - A proposed revision of F060 and any separate implementation issues justified by the decision.
  Validation:
  - Specify independent customer setup and public proxy acceptance for every recommended provider offering.
  - Require evidence from a separate customer account without proxy-server access or operator credential provisioning.
  - Record any missing credential or provider approval as an explicit unresolved comparison result.
  - Keep implementation approval separate from this planning issue.
  Assessment (2026-09-07):
  - `docs/gemini-customer-connections.md` compares customer setup, exact routes, excluded scenarios, and retained evidence.
  - Qualify the billed AI Studio key flow first. Assess Vertex Express independently.
  - Current Google documentation specifies authorization keys and distinct service restrictions.
  - The Express model table does not establish access to every required exact model.
  - F060 passed 28 proxy requests with an operator service account. This result does not satisfy independent customer setup.
  - The proposed F060 revision uses the accepted API-key contract and retains P001 as the shared interface owner.
  - The plan defines repeated functional, lifecycle, capacity, latency, cost, and tenant-isolation acceptance.
  - The initial assessment changed planning documents only. Later acceptance runs are recorded separately below.
  - The Governor check passed without drift. Changed prose passed the language review and mechanical checks.
  - `git diff --check` passed, and the issue ID review found no duplicates.
  Existing account verification (2026-09-07):
  - The user selected the primary Google account and its existing billed project for continued acceptance.
  - AI Studio showed Tier 1 Prepay and a positive credit balance.
  - The existing authorization key passed all three direct model suites, including the supported background lifecycle checks.
  - Flash 3.8 and Flash-Lite 3.5 passed proxy reasoning and image acceptance.
  - The original dotenv key passed verification, but proxy completion failed with `curl` error 28 after 45 seconds.
  - `docs/evidence/gemini-customer-existing-account-2026-09-07.json` records the sanitized results.
  - The Vertex API-key form uses the existing Vertex service account and restricts access to Agent Platform API.
  API-key comparison (2026-09-07):
  - The user approved Vertex key creation in the existing project with the existing Vertex service account.
  - The Google CLI created the key after the browser connection failed.
  - The initial key was revoked because the CLI included its value in diagnostic output.
  - Replacement creation captured all raw output. The replacement restricts access to `aiplatform.googleapis.com`.
  - Both API-key routes passed 22 direct cases across Pro 3.1 Preview, Flash 3.8, and Flash-Lite 3.5.
  - Cases covered declared reasoning levels, structured output, one image, and one audio clip.
  - The initial comparison passed 41 of 44 requests. Three Gemini schema requests used an incorrect array format.
  - All three corrected Gemini requests passed. These diagnostic corrections do not indicate a provider capacity failure.
  - `docs/evidence/gemini-vertex-api-key-comparison-2026-09-07.json` retains all 47 request results and token estimates.
  - Both providers list the same Standard token rates for these models and inputs.
  - Direct key access passed. Independent first-use setup and Vertex key acceptance through the proxy remain unverified.
  - Runtime authentication and production configuration did not change during this comparison.
  - The Governor check and changed-prose checks passed. `git diff --check` passed.
  Saved key and capacity session (2026-09-07):
  - The user supplied the existing billed key as `AI_STUDIO_API_KEY` in `configs/.env`.
  - The local `GEMINI_API_KEY` assignment now selects the same verified value.
  - Gemini 3.5 Flash passed default proxy text and image acceptance.
  - Flash 3.8 and Flash-Lite passed proxy reasoning and image checks. All three direct model suites passed.
  - Flash 3.8 initially returned HTTP 422 during proxy key verification. The repeat and ten concurrent direct verification requests passed.
  - The initial verification rejection has no established upstream cause and remains in the evidence.
  - Pro 3.1 has no Gemini provider offering in the current catalog. Its proxy candidate command stopped before a provider request.
  - The user selected 10 concurrent requests. The steady request rate remains unspecified.
  - Vertex passed the complete 220-request capacity matrix and 20 additional initial requests.
  - Gemini Flash passed 70/70. Gemini Pro passed 21/30 and hit its paid-project quota of 25 requests per minute.
  - Gemini Flash-Lite passed 29/30 before HTTP 200 with `MALFORMED_RESPONSE` stopped that offering.
  - The evidence retains all 370 attempts and ten failures. Estimated token charges total USD 0.28469825.
  - `docs/evidence/gemini-saved-key-acceptance-2026-09-07.json` records local configuration and functional results.
  - `docs/evidence/gemini-vertex-capacity-2026-09-07-session-1.json` records the first capacity session.
  - Two automated follow-up sessions run at 12-hour intervals. Gemini Pro remains excluded pending quota or request-rate resolution.
  - The current evidence favors Vertex for repeated bursts. The support decision remains provisional.
  Capacity session 2 (2026-09-08 UTC):
  - The scheduled run began more than 12 hours after session 1, with 10 concurrent requests.
  - Vertex Pro and Flash each passed 70/70. Gemini Flash passed 70/70 and Flash-Lite passed 80/80.
  - Vertex Flash-Lite passed 29/30 before one HTTP 200 response lacked a finish reason and failed answer acceptance.
  - The runner stopped the remaining 50 Vertex Flash-Lite cases. The failure cause remains unknown.
  - The session retained 320 attempts, 319 accepted results, and one failure. No request was retried or replaced.
  - Estimated token charges total USD 0.227892 with the retained September 7 rates. Actual charges remain unverified.
  - Each Flash-Lite offering now has one failure among 110 attempts across both sessions.
  - `docs/evidence/gemini-vertex-capacity-2026-09-08-session-2.json` retains the separate results and cumulative counts.
  - Session 3 remains scheduled. The initial quota and output failures remain in the support decision.
  Capacity session 3 and aggregate (2026-09-08 UTC):
  - Session 3 started 24.388 hours after session 1 at concurrency 10.
  - It retained 250 attempts, 238 accepted results, and 12 failures. No request was retried or replaced.
  - Ten Vertex Pro audio requests reached the client read deadline. Their upstream outcomes remain unknown.
  - Vertex Flash-Lite had one timeout. Gemini Flash-Lite returned another malformed response at low reasoning.
  - Both Flash 3.8 offerings passed 210/210 across three complete direct matrices.
  - Vertex Pro passed 220/230. Gemini Pro retains 21/30 and its original quota blocker.
  - Gemini Flash-Lite passed 138/140. Vertex Flash-Lite passed 118/120. Both remain below 99 percent observed success.
  - The aggregate retains 940 attempts, 917 accepted results, and 23 failures across three finite burst sessions.
  - Returned token usage gives an estimated USD 0.69257860. Eleven timeout requests have unknown usage and charges.
  - `docs/evidence/gemini-vertex-capacity-2026-09-08-session-3.json` records the separate final session.
  - `docs/evidence/gemini-vertex-capacity-2026-09-08-summary.json` records aggregate results and the remaining gates.
  - The automation is paused after the two authorized follow-up sessions. Runtime and production configuration remain unchanged.
  Public proxy diagnostics (2026-09-08):
  - The first Flash 3.8 connection returned HTTP 422. Its upstream cause remains unproven.
  - A corrected recorder retained the required API revision and captured an intermittent background-operation rejection from Google.
  - Flash 3.8 text generation passed 9/10 at concurrency 10. Google returned HTTP 400 for the other creation.
  - All 15 connection and tenant checks passed. Ten separate concurrent connection saves also passed.
  - The capacity runner stopped before the remaining reasoning, schema, image, and audio cases.
  - The initial recorder omitted the API revision. Its results remain excluded from acceptance.
  - The separate model diagnostic passed 43/44. Gemini Flash-Lite returned another malformed response at low reasoning.
  - Vertex Pro audio passed 11/11. Vertex Flash-Lite passed 22/22. The earlier failures remain in the qualification evidence.
  - `docs/evidence/gemini-customer-proxy-diagnostics-2026-09-08-summary.json` records the results and instrumentation limits.
  - I255 records the verification error-classification work. The user approved the bounded F060 API-key change.
  Blocked:
  Resolve the Gemini Interactions background rejection or qualify the approved Vertex API-key route.
  Complete key acquisition and public proxy acceptance through a separate customer account without proxy-server access.
  Diagnose Vertex Pro audio timeouts and both Flash-Lite failure sets. Resolve the Gemini Pro request-rate decision and record actual costs.
  Complete the approved F060 API-key revision and its public proxy acceptance.
  Keep the support decision provisional until this evidence is available.
  Sources:
  - https://docs.cloud.google.com/vertex-ai/generative-ai/docs/start/api-keys
  - https://docs.cloud.google.com/docs/authentication/api-keys
  - https://docs.cloud.google.com/vertex-ai/generative-ai/docs/start/express-mode/vertex-ai-express-mode-api-reference
- [ ] [P008] (P1) Plan unified local inference and `computercat` GPU control.
  Goal:
  Make LLM Proxy the sole public inference API for cloud and local operations.
  Plan a forward-only migration that makes Qwen3.8-27B Q4_K_M, SAM 3.1
  still-image segmentation, and all current Dictator operations available
  through LLM Proxy. Keep model execution on `computercat`, load only the
  required GPU runtime, and stop an idle runtime. This planning issue does not
  authorize implementation.
  Requirements:
  - Use these confirmed architecture constraints:
    - Keep the public LLM Proxy service on the gateway host. A failure or
      restart on `computercat` must not stop cloud provider routes.
    - Make LLM Proxy own public authentication, tenant authorization, the
      provider catalog, request validation, asset ownership, public operation
      state, usage records, rate limits, and normalized errors.
    - Keep Dictator as a private speech runtime. Move its public capabilities
      behind LLM Proxy. Do not move its speech engines and artifact workers
      into the public LLM Proxy process.
    - Run one private node controller on `computercat`. Make it own GPU request
      admission, runtime selection, container lifecycle, readiness checks,
      draining, idle timers, and crash reconciliation.
    - Use Docker Compose and one small controller. Do not introduce Kubernetes
      for this single-node runtime.
    - Treat `computercat` as deployment placement, not as a public provider or
      model identity.
    - Use one exclusive GPU group. Do not promise simultaneous residency or
      low cold-start latency for different runtimes on one GPU. Record a
      second GPU as a capacity requirement if the service needs that promise.
    - Return an explicit local-offering unavailable error when the private
      node is unavailable. Do not send the request to a cloud model or another
      local model.
    - Replace the former Gemma proposal with Qwen3.8. Do not add Gemma to this
      plan.
    - Limit the first SAM 3.1 scope to still images. Do not add video tracking
      or segmentation sessions.
    - Complete one bounded Dictator cutover. Remove the direct public Dictator
      route and direct first-party callers after the cutover. Do not keep a
      compatibility route, dual writes, or an indefinite dual-read period.
  - Define a private control contract between LLM Proxy and the node controller:
    - Accept only declared runtime profile identifiers. Never accept an
      arbitrary image, command, path, port, mount, or environment value from a
      public request.
    - Authenticate and authorize the gateway-to-node connection. Keep the
      controller and all model endpoints off the public network.
    - Do not give the public LLM Proxy container direct access to the Docker
      socket. If the controller runs in a container, use a restricted socket
      boundary that exposes only the required lifecycle operations.
    - Return typed states for queued, starting, ready, busy, draining,
      stopped, failed, and unavailable work. Include a bounded wait estimate
      or retry time where the controller can calculate it.
    - Issue a GPU lease only after the selected runtime is ready. Release the
      lease after a synchronous request ends or an asynchronous operation
      reaches a terminal state.
    - Close admission before a runtime enters the draining state. Wait for its
      active leases and background operations to end. Do not stop a container
      that owns active work.
    - Stop the complete runtime container to unload a model. Verify that CUDA
      memory is released before another runtime starts. Do not use an in-process
      cache-clear call as the unload contract.
    - Start the idle timer only when the runtime has no active lease, queued
      request, or background operation. Permit an exact idle limit for each
      runtime profile.
    - Use a bounded queue and a documented fairness rule. Continuous Qwen
      traffic must not prevent admitted segmentation or speech work from
      running.
    - Reconcile a controller or container restart from declared state. Fail or
      resume public operations only according to the selected operation-state
      contract. Never infer success from an incomplete record.
  - Define immutable runtime profiles and artifact storage:
    - Pin each container image, model revision, quantization, tokenizer,
      processor, and checksum. Reject a profile whose required artifact is not
      present or does not match its checksum.
    - Download gated and large model artifacts during an explicit preparation
      step. Do not download model files on the first public request.
    - Keep model files and caches outside disposable containers. Mount them
      read-only where the runtime does not need to write.
    - Give each profile exact GPU, host-memory, temporary-storage, concurrency,
      startup, request, drain, and idle limits.
    - Record cold-start time, warm latency, peak GPU memory, peak host memory,
      and shutdown time for every qualified profile.
  - Qualify the Qwen text profile before it enters the public catalog:
    - Use canonical model ID `qwen3.8-27b-q4-k-m` for the exact Qwen3.8-27B
      Q4_K_M artifact. Pin its source revision and checksum.
    - Evaluate `llama.cpp` `llama-server` as the first GGUF runtime. Record a
      different runtime only if measured results or a required request feature
      rejects this choice.
    - Start with one inference slot, full supported CUDA offload, a 16K context
      limit, and text-only messages. Test 32K context before it is advertised.
    - Map the canonical messages contract, usage values, finish reasons,
      cancellation, timeouts, structured output, tools, and reasoning effort
      only after each item passes an exact runtime test. Reject a capability
      that the profile does not declare.
    - Keep private reasoning text out of public responses, usage records, and
      logs.
  - Qualify the SAM image profile before it enters the public catalog:
    - Select the official SAM 3.1 still-image processor and checkpoint after
      license, access, revision, and checksum review.
    - Accept the input image through the tenant-owned LLM Proxy asset system.
      Define exact text, box, positive-point, and negative-point prompt shapes.
      Define one coordinate system and reject mixed or out-of-range values.
    - Return ordered instances with an exact score, bounding box, and
      tenant-owned mask artifact. Decide whether the canonical mask format is
      PNG, run-length encoding, or one exact combination before implementation.
    - Use a public operation resource when queue and cold-start time can exceed
      the synchronous request budget. Define cancellation, expiry, retention,
      and asset-deletion behavior.
  - Qualify Dictator runtime profiles against the F042 public capability contract:
    - Reuse the F042 capability inventory, public resources, private adapter, and caller migration ownership.
    - Verify asset transfer, progress, cancellation, failures, and restart reconciliation through the selected runtime profile.
    - Keep a Dictator runtime resident while one of its accepted background
      jobs is active. Add an exact drain and activity signal before the node
      controller can stop it.
    - Decide whether one Dictator container profile can satisfy measured GPU
      limits. Split transcription, analysis, and synthesis into separate
      profiles when one process keeps incompatible models resident.
    - Use the transcription route decision owned by F042.
  - Coordinate each selected local capability through its existing public contract:
    - Add local operations and offerings to the normalized provider catalog.
      Keep runtime placement and private endpoint data out of its public
      projection.
    - Route Qwen through the canonical messages version selected by F020. Do
      not add a new local text endpoint or preserve `/v2` after `/v3` becomes
      current.
    - Define typed segmentation and retained speech resources, operation
      states, error codes, cancellation, limits, prices or internal-cost
      records, usage units, and retention rules.
    - Update the server, OpenAPI document, Go client, Python client, Node.js
      client, CLI, examples, and black-box fixtures in the same contract
      change. First-party applications must use an official LLM Proxy client.
    - Keep provider credentials, gated-model tokens, node credentials, and
      private runtime addresses on the server. Redact prompts, media content,
      reasoning text, tokens, and private runtime errors from logs.
    - F042 owns the Dictator API and caller migration. F026 owns the separate ElevenLabs provider migration.
  - Resolve these open decisions before implementation issues are approved:
    - Select the canonical provider identity for MPR Lab local offerings and
      the exact model and operation IDs. Do not use a deployment host name as
      the provider ID.
    - Select private HTTP or gRPC for the node control contract. Specify mutual
      authentication, authorization, timeouts, retry rules, and request IDs.
    - Select a host service or a restricted container for the node controller.
      Record its deployment owner and the owner of the `computercat` Compose
      project.
    - Set per-profile idle limits, queue limits, admission priorities, fairness
      rules, maximum cold-start waits, and maintenance behavior from measured
      data.
    - Reuse F022 operation storage and restart behavior. Specify the controller-owned runtime and lease state transitions.
    - Select the exact SAM prompt set, coordinate system, mask format, maximum
      image size, maximum instance count, and retention limits.
    - Select the Dictator runtime profile split and verify the F042 artifact transfer contract under GPU scheduling.
    - Decide whether local usage has a billable price, an internal cost only,
      or no price. Keep the decision explicit in catalog and usage contracts.
  - Use this implementation sequence after the architecture is approved:
    1. Qualify the hardware and each pinned runtime on `computercat`. Measure
       Qwen at 16K and 32K, SAM prompt and output cases, each Dictator profile,
       repeated runtime switches, and complete GPU-memory release.
    2. Approve public API ownership, catalog identities, asset ownership,
       operation state, private control, deployment, and security contracts.
       Update the planning records for F020, F022, and F042 where required.
    3. Build the node controller against fake runtime containers. Prove its
       state machine, GPU leases, bounded queue, fairness, draining, idle stop,
       failed startup, cancellation, and restart reconciliation.
    4. Prepare private Dictator profiles. Add exact readiness, activity, drain,
       and cancellation signals. Prove that an accepted background job prevents
       unload until it reaches a terminal state.
    5. Add the local provider adapter and Qwen offering to LLM Proxy. Exercise
       the public router and each official client against a fake node before a
       live `computercat` acceptance run.
    6. Add the SAM segmentation resource, image-asset validation, private
       adapter, mask artifacts, and asynchronous operation behavior.
    7. Verify F042 resources through the selected Dictator runtime profile. Reuse its public ownership contract.
    8. Deploy the private controller and runtime profiles on `computercat`.
       Remove public runtime ports and routes. Install only the credentials and
       model artifacts that each component requires.
    9. Verify the F042 caller migration receipt before activating a replacement Dictator runtime profile.
       Remove only obsolete runtime configuration introduced or replaced by this controller migration.
    10. Run production acceptance through Qwen, SAM, Dictator, and Qwen again.
        Confirm queue behavior, no active-work preemption, no GPU-memory growth,
        cancellation, restart recovery, idle unload, logs, metrics, and cloud
        route availability while `computercat` is unavailable.
  Deliverables:
  - Add an approved architecture decision that assigns public API, node
    control, GPU lifecycle, asset, operation, catalog, security, deployment,
    and observability ownership.
  - Add a measured runtime qualification report for every pinned profile and
    the repeated Qwen-to-SAM-to-Dictator-to-Qwen switch sequence.
  - Add exact public and private API schemas, state diagrams, timeout budgets,
    queue rules, idle policies, deployment topology, and threat boundaries.
  - Add a cross-repository migration table for LLM Proxy, Dictator, deployment
    configuration, official clients, and each first-party caller. Give each
    forward migration and deletion an ordered implementation issue.
  - Add an acceptance matrix for fake-runtime tests, live-node tests, failure
    injection, resource limits, security, observability, and production
    receipts.
  Validation:
  - Confirm each open decision has one approved answer and one owner. Confirm
    each implementation step has an ordered issue and explicit dependency.
  - Review the plan against the current LLM Proxy API, official clients,
    Dictator gRPC contract, Dictator callers, and deployment resources. Resolve
    all contract conflicts before the first implementation issue starts.
  - Confirm the acceptance matrix proves exact routing, tenant isolation,
    server-side secrets, bounded admission, fair scheduling, safe draining,
    complete unload, crash reconciliation, public error normalization, and no
    cloud fallback for unavailable local work.
  - Reference the F042 caller migration receipt. Record separate deletion evidence for runtime profiles replaced by the controller migration.
  Scope after gateway consolidation:
  - Keep this issue as planning for additional local models, GPU control, and runtime qualification.
  - F042 independently owns the public Dictator capability migration through the existing qualified runtime.
  - Qwen, SAM, and GPU residency work are not blanket prerequisites for F042 or cloud media operations.
  - Reuse F022 operation storage and F042 speech resource contracts instead of defining competing public lifecycles.
  - Require measured evidence before making a controller change a prerequisite for a retained Dictator capability.
- [ ] [P006] (P2) Define provider lifecycle, model onboarding, and hosted service SLA terms.
  Goal:
  Turn the proposed long-term provider support, model-addition timing, and
  hosted availability promises into measurable service commitments before they
  appear in public marketing copy.
  Requirements:
  - Define separate provider lifecycle, model-onboarding SLO, and hosted uptime
    SLA scopes, including eligibility, measurement windows, exclusions,
    deprecation notice, incident communication, and remedies where applicable.
  - Decide which commitments apply to the open-source integration contract,
    managed provider onboarding, and a hosted service. Do not collapse them into
    one ambiguous guarantee.
  - Identify the operational evidence, ownership, monitoring, support channel,
    and approval needed to publish each commitment.
  Deliverables:
  - An approved support-policy and SLA contract suitable for public-site copy,
    with implementation issues for any missing operational controls.
  Validation:
  - Legal, product, and service owners approve each published metric and the
    production evidence path can calculate it without manual interpretation.
- [ ] [P003] (P2) {I218} Re-audit and expand the SEO/use-case resource system from verified product contracts.
  Goal:
  Refresh LLM Proxy's search and resource strategy from the current repository
  contract so prospective users can discover concrete, supported ways to use
  the service without creating duplicate doorway pages or claiming roadmap work
  as shipped functionality.
  Requirements:
  - Produce a new repo-grounded SEO report before changing public copy. It must
    inventory current capabilities, limits, public routes, existing resource
    pages, claim evidence, unsupported claims, the final landing/`/app/`
    separation, and every current provider/model capability from the normalized
    generated catalog.
  - Audit and cover distinct user jobs including: self-service bring-your-own
    provider-key onboarding; multi-provider and model routing; provider/default
    model selection; `/v2` messages and direct REST integration; Go, Python,
    and CLI clients; text response formats and usage headers; dictation;
    supported OpenAI web search; generated-secret lifecycle; tenant/admin usage
    visibility without prompt or key exposure; native/compatible provider
    adapters; runtime configuration; and queue, rate-limit, timeout, and error
    handling. Merge or reject a page unless it has at least three independent
    distinctions such as audience, job, workflow, feature set, example,
    objection, FAQ, CTA, or internal-link path.
  - For every approved page, record audience, problem, search intent, primary
    and secondary keyword candidates, product evidence, allowed and forbidden
    claims, differentiating examples, internal-link path, and doorway-page
    risk. Do not claim search volume, rankings, pricing, benchmarks,
    testimonials, compliance, provider performance, or roadmap behavior before
    it is implemented.
  - Replace the generator's arbitrary page-count quota and fixed modified-date
    snapshot with an evidence-backed content manifest. Compute `lastmod` only
    from maintainable source/build data or omit it; never publish stale dates.
    Keep every model/provider assertion tied to the generated public catalog.
  - Enforce the complete indexing contract: canonical, sitemap, Open Graph,
    JSON-LD, and crawlable internal links use one final trailing-slash URL;
    root and the resource hub link to all public content; `/app/`, private
    API pages, token pages, redirects, and noindex pages stay out of the
    sitemap. Schema must match visible content, and article-like pages need
    visible maintainer attribution and a verifiable publication/modification
    policy.
  - Preserve or improve useful existing resources rather than regenerating
    generic copy. Each indexable page must have a concrete repository-derived
    command/configuration example, problem-specific FAQ, limitation section,
    meaningful CTA, and accessible/lazy-loaded presentation where applicable.
  Deliverables:
  - Update `docs/marketing/seo-resource-cluster-report.md` with the fresh repo
    analysis, use-case opportunity list, recommended generation order,
    rejected/merged ideas, claim audit, indexing audit, and explicit evaluation
    scores.
  - Replace the static SEO source/generator with a deterministic evidence-backed
    manifest, refreshed resource hub/pages, contextual related links, sitemap,
    robots, and landing-page discovery paths.
  - Add a release-verification checklist covering final URL responses,
    canonical/sitemap alignment, JSON-LD validity, internal-link crawlability,
    Google Search Console URL Inspection, and Rich Results Test where the
    visible schema qualifies.
  - Do not manufacture pages merely to reach a count, rely on sitemap-only
    discoverability, repeat a generic FAQ across a cluster, or retain stale
    provider/model and roadmap claims as marketing copy.
  Validation:
  - Make generation fail on missing evidence, duplicate or orphaned pages,
    unsupported claims, stale date metadata, incompatible canonical URLs,
    sitemap entries that are not public `200` pages, invalid JSON-LD, or a page
    that does not meet the documented specificity/doorway thresholds.
  - Add black-box static-site/browser coverage for the public root, hub,
    representative pages from every use-case family, `/app/` exclusion, and
    crawlable navigation from landing page to hub to resource page.
  - Require an evaluation result of at least 4/5 for repo grounding, use-case
    specificity, doorway safety, metadata, conversion clarity, duplicate-risk,
    site integration, and indexing readiness, and exactly 5/5 for factual
    integrity before publication.
  - Run the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci`
    pair for the implementation, with the final run after the last code edit.
