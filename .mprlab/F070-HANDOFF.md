# F070 Handoff

## Latest Handoff: 2026-09-24

Read this section first. It supersedes all earlier checkout states and next steps below.
The user requested a handoff because the session has few tokens left.
After the handoff request, the active goal resumed with the B273 configuration correction.

### Verified Checkout

- Repository: `/Users/tyemirov/Development/llm-proxy`.
- Branch: `feature/F069-prepaid-payments`.
- Verified parent of the B273 increment: `ae2eb21c90bc20cf09d2235487d59931ffb0e3da`.
- PR 344: https://github.com/tyemirov/llm-proxy/pull/344.
- PR 344 is open and ready for review. Its base is `feature/F068-prepaid-balances`.
- The checkout was clean before the handoff update. The subsequent B273 changes are recorded below.
- The operator committed the earlier CLI test, handoff, tracker, and media documents. Preserve those commits.
- Consult the current B273 evidence below for validation and publication state.

The F070 objective remains incomplete. Do not mark it complete or paused because of this handoff.
Keep F065 through F070 in scope. Do not start unrelated I274 media work from the tracker instruction.
Keep production activation disabled. Application merge is not authorized.

### Current Payment Correction

B273 confirmed the empty payment configuration defect through the real root command.
The failing `empty` scenario reported `started=true error=<nil>`.
The source correction preserves explicit payment presence before typed decoding.
The existing payment validator now rejects the incomplete object. Omitted configuration still disables payments.
The new tests are in `cmd/cli/payment_configuration_test.go`.

CLI regression passed in 8.578 seconds. Go lint and format checks passed.
All 87 selected CLI race scenarios passed in 95.615 seconds. No validation process remains active.
Evidence files use `/tmp/llm-proxy-b273-` with suffixes `before.log`, `cli.log`, `race.log`, and `checks.log`.
The fresh coverage diagnostic has 464 uncovered statements across 21145 statements.
Use `/tmp/llm-proxy-b273-diagnostic.coverprofile` and `/tmp/llm-proxy-b273-merge.py`.
The aggregate coverage gate and final CI remain open.

B273 is recorded in the commit that contains this section. Verify its remote publication before the next edit.
Continue B266 and the remaining provider qualification.
Keep the full F070 scope below. Do not infer completion from this configuration correction.

### Latest Completed Increment

B272 is resolved in `ae2eb21c`. It rejects `hosted: {}` before service startup or database creation.
Omitted hosted configuration still permits startup with hosted execution disabled.
The B272 execution plan was removed. Keep `.mprlab/B266-PLAN.md` while B266 remains open.
Its latest section, `Hosted Scope CLI Rejection Increment`, records the completed increment.

Current evidence:

| Validation | Result | Evidence |
| --- | --- | --- |
| Hosted runtime | Passed, proxy 36.644 seconds and CLI 2.679 seconds | `/tmp/llm-proxy-b272-runtime.log` |
| Final CLI configuration | Passed, 7.389 seconds | `/tmp/llm-proxy-b272-cli-final.log` |
| CLI race checks | 82 scenarios passed, 81.180 seconds | `/tmp/llm-proxy-b272-cli-race.log` |
| Final lint and format | Passed | `/tmp/llm-proxy-b272-checks-final.log` |

The race run preceded the final single-lookup refactor. Final CLI, lint, and format checks followed that change.
These results do not establish aggregate CI success or hosted CI success.

The current diagnostic has 464 uncovered statements across 21143 statements.
Use `/tmp/llm-proxy-b272-diagnostic.coverprofile` or its copy, `/tmp/llm-proxy-b266-diagnostic.coverprofile`.
The current merge script is `/tmp/llm-proxy-b272-merge.py`.
It discards old counts for `cmd/cli/config_file.go` and uses the final CLI profile for that file.
Do not merge stale source coordinates after another production edit.

The last aggregate CI run failed with `coverage total 95.3%, want 100.0%`.
B266 and final `make ci` remain open. Do not lower the coverage threshold.
Responses image billing, Dictator duration billing, and ElevenLabs alignment qualification remain incomplete.
See `Remaining F070 Work` below for the complete known scope and open commercial decisions.

### Preserved Decisions And Sandbox Follow-Up

Use provider cost multiplied by 1.30, all current providers, and a USD 5 funding minimum.
Customers can spend down to zero. Use existing Ledger and verify balance conservation.
Reuse existing integrations. Deliver the browser application without a native mobile application.

F087 already records the separate Paddle sandbox setup. It does not block development completion.
The verified private source path is `/Users/tyemirov/Development/PoodleScanner/configs/.env.ps`.
Prior read-only authentication passed. No secret values belong in this handoff or another repository.
The prior account check found no LLM Proxy product or USD 5 price.
F087 owns distinct product and price creation, identity confirmation, notification setup, and actual sandbox qualification.
Do not reuse another application's product or webhook secret.

### Resume Commands

Use the repository Make targets. Relevant focused commands are:

```bash
make test-hosted-runtime
make test-upstream-admission ADMISSION_TEST_PATTERN='^Test(RootCommand|HostedRuntimeCLI|HostedRuntimeConfiguration)'
make go-lint check-format
```

Use `GOFLAGS` for race or coverage options when needed.
Run final `make ci` at the stack completion checkpoint after corrections.
Update PR 344 only after applicable validation. Preserve the remaining-scope and F087 paragraphs in its description.

## Resume Update: 2026-09-24

This section supersedes the original snapshot below.
The operator committed the handoff, CLI rejection tests, and media documentation after that snapshot.
The resumed work started from clean commit `fde0714eb4bacb1ba13c30cd765a519127138bb2` on the same F069 branch.

B272 now rejects `hosted: {}` at the configuration file boundary, before service startup or database creation.
Omitted hosted configuration still reaches startup with hosted execution disabled.
The correction changes `cmd/cli/config_file.go` and adds an omitted-configuration check in `cmd/cli/hosted_runtime_test.go`.
The durable configuration contract is updated in `docs/hosted-billing.md`.

Runtime HTTP checks passed in 36.644 seconds. Final CLI configuration checks passed in 7.389 seconds.
All 82 selected CLI race scenarios passed in 81.180 seconds before the final lookup refactor.
Final CLI, lint, and format checks passed afterward.
Evidence uses `/tmp/llm-proxy-b272-` with suffixes `runtime`, `cli-final`, `cli-race`, and `checks-final`.
The current diagnostic is `/tmp/llm-proxy-b272-diagnostic.coverprofile`, also copied to `/tmp/llm-proxy-b266-diagnostic.coverprofile`.
It has 464 uncovered statements across 21143 statements. The decoder uses only current validation counts.
The merge script is `/tmp/llm-proxy-b272-merge.py`.

Continue B266 and the remaining F070 provider qualification. The aggregate coverage gate and final CI remain open.
The original failing CLI test no longer needs implementation. B272 records its resolution.

## Original Handoff Snapshot

## Objective And Current State

Implement F070 in full scope. Deliver F065, F066, F067, F068, and F069 as a ready PR stack.
The goal remains active. The user requested this handoff before further implementation.
Do not mark the goal complete or paused from this handoff.

Repository: `/Users/tyemirov/Development/llm-proxy`
Branch: `feature/F069-prepaid-payments`
Verified local and remote HEAD: `a1301f5700b26d8dc05c62a456406f293b43a5a8`
Current PR: https://github.com/tyemirov/llm-proxy/pull/344
PR 344 is open and ready. Its base is `feature/F068-prepaid-balances`.
These facts were verified when this note was written.

The prior implementation turns made progress. No external condition currently prevents further development.
All test and publication processes from these turns have terminated. No live tool session needs recovery.

## Immediate Unfinished Work

The current uncommitted change is in `cmd/cli/hosted_runtime_test.go`.
It adds `TestHostedRuntimeCLIRejectsInvalidScopesBeforeServiceStartup` with eight scenarios.
The test executes the real root command. A controlled service dependency detects unexpected startup.
Each rejected configuration must leave its database absent.

The scenarios cover:

- An empty offering list.
- An empty hosted object.
- Duplicate offering scopes.
- Duplicate scopes with different conditions.
- An unknown provider service.
- A cache condition in deployment configuration.
- A token range in deployment configuration.
- An invalid region.

Seven scenarios passed. The empty object found a reproducible defect:

```text
--- FAIL: TestHostedRuntimeCLIRejectsInvalidScopesBeforeServiceStartup/missing-list
hosted_runtime_test.go:44: invalid hosted scope reached startup:
started=true error=<nil> want=explicit offering scopes are required
```

Input `hosted: {}` reaches service startup without an error.
Viper drops the empty object during `UnmarshalExact`. The runtime then treats hosted configuration as omitted.
Explicit nonempty objects reach `newHostedRuntimeSettings`, which rejects missing offering scopes.
Omitted hosted configuration must continue to disable hosted execution.

Failing command:

```bash
GOFLAGS='-coverpkg=./internal/proxy,./cmd/cli -coverprofile=/tmp/llm-proxy-b266-scope-rejections.coverprofile' make test-hosted-runtime
```

Evidence: `/tmp/llm-proxy-b266-scope-rejections.log`.
The proxy package passed in 37.121 seconds. The CLI package failed in 4.343 seconds.
Do not treat this profile or this run as passing validation.

No production fix exists. No separate bug entry exists yet.
B272 was absent from the active tracker and archive at handoff. Recheck before allocation.

Next steps:

1. Record the defect as a separate BugFix and create its untracked execution plan.
2. Preserve the existing failing test as evidence.
3. Fix explicit hosted-object validation at the configuration boundary.
4. Preserve valid omitted configuration and existing startup errors.
5. Run the focused runtime and CLI checks, then applicable lint and format checks.
6. Update B266, the new bug, and PR 344 after validation passes.

Relevant code:

- `cmd/cli/config_file.go`: `loadRuntimeConfiguration`, `validateExplicitPositiveIntegerConfiguration`, and `fileConfiguration.Hosted`.
- `internal/proxy/hosted_runtime.go`: `newHostedRuntimeSettings`.
- `cmd/cli/root_test.go`: `executeRootCommand`, `withServeProxy`, `writeTestConfig`, and `completeManagementYAML`.
- `cmd/cli/root.go`: root command configuration loading and service startup.

Viper v1.21.0 source is locally available under `/Users/tyemirov/go/pkg/mod/github.com/spf13/viper@v1.21.0/`.
`InConfig` tests whether a path lookup returns a non-nil value. Explicit null needs separate analysis if relevant.
Empty payment configuration might have the same decoder behavior. This possibility is unverified.
Do not report that possibility as a confirmed defect.

## Checkout Preservation

The index is empty. These unrelated changes predate the current work:

- `.mprlab/ISSUES.md`: approximately 420 changed lines for separate media work.
- `docs/media-gateway-consolidation.md`.
- `docs/mediaops-model-access-boundary.md`.

Preserve all three files. Do not stage their unrelated changes.
The only current application change is the new CLI test above.
This handoff is a new local document. Do not include it in the application PR unless requested.

The ignored execution plan is `.mprlab/B266-PLAN.md`.
Its latest section is `Hosted Scope CLI Rejection Increment` and records the failing result.
Keep the plan while B266 remains open.

For tracker staging, construct a patch from the index version with only the selected issue entries replaced.
Use `git apply --cached` for that patch, then stage explicit application files.
Verify `git diff --cached --check` and the staged file list before committing.
Never stage the whole tracker during this goal.

## User Decisions And Authorization

- Customer price equals provider price multiplied by 1.30. This is markup, not gross margin.
- Cover all current providers and their supported operations.
- Use a USD 5 funding minimum. Permit spending down to zero without a retained floor.
- Use Paddle for payments and merchant-of-record services.
- Reuse existing PoodleScanner, Hecate, Ledger, and shared-library integrations.
- Use existing Ledger journal semantics and verify balance conservation. Do not add double-entry accounting.
- Deliver the browser application. Do not add a native mobile application.
- Keep missing sandbox setup separate from development completion.

Approved shared releases were completed earlier:

- Ledger PR 102: v1.1.0.
- utils PR 45: v0.18.0, including the approved publication-gate correction.
- utils PR 47: v0.19.0 with shared Paddle adjustment reads.

The goal authorizes ready PRs and updates to the existing PR branches.
Do not create drafts, worktrees, or rewritten history.
Application merge and production activation are not authorized.
Production hosted payments remain disabled.

The PR stack was previously verified as:

| Issue | PR | Base |
| --- | --- | --- |
| F065 | 339 | master |
| F066 | 340 | F065 branch |
| F067 | 341 | F066 branch |
| F068 | 342 | F067 branch |
| F069 | 344 | F068 branch |

PR 343 is unrelated onboarding work.
Only PR 344 was refreshed at this handoff. Recheck other PRs before claiming their current state.
Hosted workflows run for PRs targeting master. Do not claim hosted CI success for the stacked PRs.

## Coverage And Validation Boundary

B266 remains open. The last aggregate stack run failed with:

```text
coverage total 95.3%, want 100.0%
```

That run passed Go tests, Python tests, and upstream race checks before the coverage gate.
Stages after the coverage gate did not run.
Evidence: `/tmp/llm-proxy-f070-b266-coverage.log` and `/tmp/llm-proxy-f070-b266-coverage.out`.
The repository-root `coverage.out` contains older results. Do not use it as current evidence.

Current passing diagnostic baseline:

- `/tmp/llm-proxy-b266-scope-diagnostic.coverprofile`.
- `/tmp/llm-proxy-b266-diagnostic.coverprofile`.
- 468 uncovered statements across 21140 statements.

This diagnostic combines focused results. It does not prove aggregate CI success.
Never lower the threshold, exclude production files, or test invalid core states just to increase coverage.
Cover real boundary failures through public entry points.
Remove unreachable code only when the canonical contract proves it unnecessary.

When production source changes, discard old counts for each changed file before merging fresh profiles.
Retain prior counts only for unchanged executable blocks.
Profiles from multiple packages can contain duplicate blocks. Combine their counts instead of overwriting them.
Use the maximum count when constructing the diagnostic set profile.
The canonical coverage script already combines counts across package and executable profiles.

Latest merge script: `/tmp/llm-proxy-b266-merge-scope.py`.
It resets `internal/proxy/hosted_runtime.go` and combines current runtime, media, and management profiles.
Do not reuse it unchanged after another production edit.

Use repository Makefile targets. Run component checks during correction.
Run final `make ci` after the last stack correction at the documented completion checkpoint.
Do not run full CI for every small increment.

## Recent Published Work

### a1301f57: Validated Offering Scopes

`internal/proxy/hosted_runtime.go` now stores an internal `hostedOfferingScope`.
It retains validated conditions, maximum attempts, and the resolved transport.
Media admission reuses the startup result from the immutable catalog.
The change removes repeated service or model resolution and two unreachable error paths.
Startup validation and request-specific pricing remain handled.

Characterization passed before the production edit:

- Runtime proxy tests: 38.781 seconds.
- Runtime CLI tests: 2.215 seconds.

Post-change checks passed:

- Runtime proxy tests: 42.490 seconds. CLI tests: 1.915 seconds.
- Media financial tests: 74.709 seconds.
- Two runtime race scenarios: 19.718 seconds.
- Management tests: 24.410 seconds.
- Go lint and format checks.

Logs and profiles use `/tmp/llm-proxy-b266-scope-` with suffixes `runtime`, `media`, `race`, `management`, and `checks`.
Diagnostic coverage changed from 470 to 468 uncovered statements.

### 663f87f5: Payment State Recovery

Added `internal/proxy/hosted_payments_state_recovery_internal_test.go`.
Ten scenarios cover storage failures, malformed events, processor outages, incomplete completion evidence, and cancellation after verified funding.
Failed startup closes the listener and retains the pending event without partial financial effects.
Restoration applies cancellation once. A lifecycle cancellation cannot reverse a verified funding credit.
The common checkout fixture now owns application construction and startup-failure helpers.

Payment regression passed in 124.414 seconds. All 22 state and credit race scenarios passed in 139.001 seconds.
Management, lint, and formatting passed. Diagnostic coverage changed from 476 to 470.
Evidence uses `/tmp/llm-proxy-b266-state-`.

### 11a26d11: Funding Order Admission Recovery

Added `internal/proxy/hosted_payments_order_recovery_internal_test.go` with ten scenarios.
Failed account locks, order reads, and order writes preserve existing funds and delivery records.
Recovery and replay through two service instances retain one order and delivery for the request.
Payment regression passed in 105.422 seconds. Ten race scenarios passed in 43.520 seconds.
Management, lint, and formatting passed. Diagnostic coverage changed from 484 to 476.
Evidence uses `/tmp/llm-proxy-b266-order-`.

### fa4ff925: Payment Worker Construction

Private checkout and processor constructors consume validated dependencies directly.
Configuration, shared-client, signature-verifier, and database-binding errors remain handled.
Payment regression, management, lint, format, and 13 targeted race scenarios passed.
Diagnostic coverage changed from 488 to 484.

Earlier fixes B267 through B271 remain resolved and documented in PR 344.
They cover trailing JSON rejection, corrupt retained prices, invalid balance fractions, corrupt credit receipts, and incomplete saved completions.

## Remaining F070 Work

F065 through F070 remain open. B266 and the final aggregate CI gate remain open.
The current ready PR stack is not proof of full completion.

Provider qualification still includes these unresolved areas:

- Responses image requests need a complete bound and billing for the response model and image tool together.
- B265 rejects incomplete Responses image bounds before dispatch. Rejection does not complete Responses image billing.
- Dictator input-operation duration must support complete financial qualification. Existing journal tests alone do not prove this.
- ElevenLabs alignment metering and price qualification remain incomplete.

Keep current providers and supported operations in scope. Native video F025 is separate unfinished work.
Do not infer financial acceptance from usage-journal tests or narrower protocol tests.

Keep unresolved commercial choices visible in `docs/hosted-billing.md`:

- Platform supplier and canonical Paddle account identity.
- Tax presentation and fee allocation.
- Maximum account exposure.
- Failed, canceled, and continuation charge policy.
- Financial retention policy.
- Provider commercial rights and paid capacity before activation.

Do not invent these decisions. Preserve development fixtures and disabled production activation.

## F087 Sandbox Follow-Up

F087 is already committed and explicitly does not block F069 or F070 development completion.
Private source path: `/Users/tyemirov/Development/PoodleScanner/configs/.env.ps`.
Read-only Paddle requests previously verified the sandbox API key from that file.
No secret values were copied or disclosed.

The account has PoodleScanner, Hecate, and Crossword products, but no active LLM Proxy product or USD 5 funding price.
F087 owns identity confirmation, a distinct product, a one-time USD 5 price, checkout origin, notification destination, and actual sandbox qualification.
Use `/api/payments/paddle/events` with its own destination secret and an isolated managed database.
Do not reuse another application's product or webhook secret.
Hecate uses RevenueCat for browser Paddle commerce. LLM Proxy uses the direct shared Paddle integration from PoodleScanner.

## Governance And Reporting

Read applicable repository guidance before edits. Do not edit AGENTS.md.
Use `.mprlab/POLICY.md`, `.mprlab/PLANNING.md`, relevant stack guides, and `docs/hosted-billing.md`.
Keep new reproducible defects in separate BugFix entries.
Keep temporary execution plans untracked. Never rewrite history to remove a plan.
Do not parallelize issues or spawn agents without explicit authorization.
Do not require physical devices. Do not inspect or change file permission modes.
Do not write persistent memory without an explicit request.

The integration, deployment, and Governor skills were already applied earlier in this goal.
Governor tools are under `/Users/tyemirov/.codex-work/skills/mprlab-governor/scripts/`.
The verified STE reference is `/Users/tyemirov/.cache/mprlab-governor/asd-ste100/issue-9/ASD-STE100_ISSUE9.pdf`.
Run `prepare-ste-reference --json` before new prose, then review changed prose and run `check-ste`.

The tracker retains 72 pre-existing mechanical prose findings.
The Governor check retains five pre-existing differences: `.gitignore`, both Go and Docker guides, PLANNING.md, and POLICY.md.
Do not normalize those unrelated files during this task.

PR body working file: `/tmp/llm-proxy-f069-pr-body.md`.
Update the current B266 paragraph after each validated increment. Preserve the other fixes and remaining-scope paragraphs.
Use `gh pr edit 344 --body-file` and verify the remote head against local HEAD after pushing.
Report local validation, hosted CI, publication, deployment, and production acceptance separately.
