# B305 Execution Record

## Result

The local change preserves unknown, partial, complete, and historical token evidence.
The summaries and displays distinguish measured zero from absent usage.
The user authorizes the scoped fix and deployment through the established lifecycle.
The user selects a combined primary-checkout release. The final combined source passes repository CI.
Release and production results are recorded in [the combined execution record](combined-release-2026-10-06.md).
The implementation contract is in [token measurement evidence](../docs/token-measurement-evidence.md).

## Checks run

- The public integration regression failed before the application change. See `evidence/B305/initial-regression.txt`.
- Focused backend checks passed in 18.915 seconds. They cover presence, continuation, polls, scope, restart, and saved completions.
- Six account and administrator browser cases passed in 9.0 seconds. They cover zero, unknown, partial, historical, mixed, and invalid API coverage.
- Six Meta route cases passed in 3.490 seconds. Their unmeasured first generation makes the combined usage partial.
- Frontend static checks passed.
- Full `make ci` passed all 14 gates in 684 seconds. Go statement coverage is 100.0 percent with no uncovered blocks.
- The independent final review found no unresolved defects.
- The first two full CI attempts found old complete-usage expectations for unmeasured contributions. The corrected assertions preserve routing and provider checks.

The first browser failure had an interval fixture error. It is not reliable evidence of the product defect.
The backend regression is the initial failure evidence.
All model responses came from local test doubles. Paid external model calls: zero.
Hosted billing remains inactive.
The [validation data](evidence/B305/validation.json) records the tested application hashes and exact CI receipt.
The evidence directory retains final command output and failure excerpts.
The validation file records the complete earlier CI log paths and hashes.

## Benchmark

LP-C01-S1-B305 passed 24 observations in 10.497 seconds after harness formatting.
The caller accepted 12 extraction outputs and rejected four invalid outputs.
OpenAI and Anthropic routes retain one unknown event for missing usage and one measured event for explicit zero.
The available numeric quantities remain zero in both cases. Coverage supplies the distinction.
All 12 retained proxy execution rows have current measurement evidence.

The original LP-C01-S1 evidence remains unchanged.
The repeated harness is in `evidence/B305/harness`. From that directory, run `make benchmark` with Go 1.26.5 and cached dependencies.
The harness rejects non-loopback HTTP destinations and disables module downloads.
No latency, model quality, or commercial advantage follows from this deterministic run.
The direct endpoint and authorization qualification remain open. LiteLLM execution remains unrun.
P015, P016, and P017 remain open for the six-week evaluation.

## Data and release limits

Temporary SQLite fixtures verify the nullable column addition, transaction rollback, restart, historical retention, and invalid evidence rejection.
Saved-completion replay checks preserve private evidence and the existing public response shape.
No production database was inspected, migrated, restored, or replaced.
The deployment retains `mprlab-nginx-gateway_llm-proxy-data` and `/data/llm-proxy-management.sqlite`.
A corrective release must preserve the evidence column and saved-completion metadata.
The Gateway contract requires forward recovery through a newer release.
An older binary is not a qualified recovery path for the new saved-completion shape.

The public API health read returned `{"status":"ok"}`.
The website marker reports `v1.12.0` at `2fda9020e776da3e82bdd50b28b2a6c87e988842`.
That marker identifies the website. It does not establish the API backend revision.
These reads are observations of the existing service. They do not verify deployment of B305.

## Release recovery

The installed Gateway is v5.0.10 at source `1d186901e4c5c4481cb1a40015a74a0a6d831522`.
Its parser converts resource maps to internal lists. The selected resource maps are valid.
The first release plan failed because the manifest had no `ci` field.
The next checks required lifecycle timeouts and observation limits.
The correction declares `make ci`, five positive timeouts, and finite repository and phase observation limits.
The completion limit is 30 minutes. The last application CI took 684 seconds.
Provider request settings, resource identities, private bindings, mounts, and retained volumes remain unchanged.
The native planner now accepts those fields and requires clean `master` synchronized with `origin/master`.
The read-only remote check found `master` at the recorded base commit.

The scoped patch applies to the clean base commit without the concurrent provider changes.
The API document was generated from the clean contract in a document fixture.
The fixture has no Git repository or application checkout.
The read-only patch check passed. The Git index remains empty.
The final Governor check reports the same six preexisting differences and no warnings.
The installed Gateway and release-policy tests passed after the final manifest change.
The native plan qualifies the final manifest only through the source guard.
The 14-gate CI result predates the manifest correction and includes the existing provider work.
The clean release candidate requires its own full CI and artifact qualification.

## Superseded isolation decision

The primary checkout contains provider work associated with F027.
The task inventory and process check did not identify an active owner.
The checks do not prove that no other process can write the checkout.
The preservation check found no unexpected changes in 917 preexisting files.
No user changes were overwritten, staged, stashed, or discarded.

`.mprlab/AGENTS.GIT.md` requires the existing primary checkout and explicit user permission for another checkout.
Git Release requires clean `master`. The occupied checkout cannot satisfy both requirements for this scoped release.
An independent release checkout needs a user exception before creation or use.
A second worktree on the same `master` branch shares Git references and can affect the primary checkout.
No callable execution-chain interface was found. Manual Git finalization also needs a scoped workflow exception.
The smallest decision is permission for one independent release checkout and normal Git finalization for B305.
The primary checkout and F027 changes must remain intact.
Deployment authorization already exists. This decision concerns checkout and workflow restrictions.

The next execution must verify release identity, publication receipts, retained-data compatibility, and the live backend revision.
The API and frontend artifacts must both identify the qualified release.
The public health response alone cannot prove that the new summaries and displays are active.
The prepared patch and source manifests are in the task workspace under `token-fix`.

## Combined release authorization

The user requests release of all current LLM Proxy changes together.
The combined source includes B305 and the current HeyGen v3 work in the primary checkout.
This decision removes the separate-checkout requirement.
The current execution must validate the exact combined source before release.
The earlier scoped patch remains recovery evidence. It is not the selected release input.
Hosted billing remains inactive. Paid provider acceptance and MediaOps cutover remain separate work.

## Final combined source acceptance

The combined source passes all 14 CI gates after the last source correction.
Go statement coverage is 100.0 percent with no uncovered blocks.
New media usage rows now record unknown evidence for each unmeasured quantity.
The public integration regression verifies scoped coverage before and after a database reopen.
The final review found no unresolved source findings.
The earlier clean-patch proposal and its release blockers are superseded by the combined release decision.
Use the combined execution record for current release and production status.
