# Combined release execution record

The user selects all current LLM Proxy changes for one release in the primary checkout.
The source includes B305, the current HeyGen v3 work, and the six-week evaluation records.
The review found no active writer. Source hashes remained unchanged through the final CI run.
No user changes were discarded.

## Repository acceptance

The final `make ci` passed all 14 gates.
Go statement coverage is 100.0 percent with no uncovered blocks.
The independent source review found no unresolved findings.
The [validation data](evidence/combined-release-2026-10-06/validation.json) identifies the tested source and retained command output.

A public integration test exposed new media usage rows without current token evidence.
The writer now records unknown evidence for each unmeasured quantity.
The test verifies tenant, provider, account, time, and administrator summaries before and after a database reopen.
Historical rows keep their null evidence.

An earlier full CI run failed at the obsolete three-field manifest assertion.
The corrected test checks the declared CI command, five finite timeouts, and four finite observation policies.
The resource, authentication, and retained-volume checks remain intact.
Two earlier CI runs stopped before the final regression assertions. They are not acceptance results.

## Release boundary

Release preparation, publication, and deployment remain separate results.
The native lifecycle must seal and publish the exact committed source before activation.
The deployment retains `mprlab-nginx-gateway_llm-proxy-data`.
Before activation, create a consistent SQLite snapshot on the production host.
Record its hash and integrity result without database contents or credentials.

Recovery requires a newer corrective release that retains the evidence column and saved-completion metadata.
An older binary is not a qualified recovery path.
Keep the current release receipt and the database snapshot available for recovery.

Hosted billing remains inactive. Paid provider acceptance and MediaOps I012 remain open.
P015, P016, and P017 remain open.
The local benchmark does not establish an advantage over direct integration or LiteLLM.
Budget and time ceilings for the commercial evaluation remain unapproved.

## Native release result

The combined application source is `5976c92ef7f17060cdcd686c070e2c8f29d05388`.
The normal push to `master` completed. The primary checkout was clean and synchronized before release.
The native `make release` CI passed all 14 gates in 676 seconds.
Go statement coverage is 100.0 percent with no uncovered blocks.
All 172 frontend browser cases and 11 authentication browser cases passed.

Release preparation then failed at `Execute Go-owned lifecycle receipt operation` with exit status 2.
The native error is `publication authority exists without a selected active release`.
Gateway v5.0.10 uses source `1d186901e4c5c4481cb1a40015a74a0a6d831522` and lifecycle contract 4.
Its bounded conversion requires current, selected, seal, or active staging authority before it can consume publication state.
The existing local state has the publication selection but none of those release authority records.
The independent review found no supported recovery command in that runtime.

No candidate artifact was assembled or sealed. Publication and deployment did not start.
No production database snapshot or migration occurred.
The public API health read still returned `{"status":"ok"}`.
The website still reports `v1.12.0` at `2fda9020e776da3e82bdd50b28b2a6c87e988842`.
That marker identifies the website. It does not establish the backend source revision.

P018 records the required recovery decision.
Preserve the existing lifecycle records.
Use a qualified Gateway repair to restore authoritative state through its supported lifecycle.
Then retry `make release && make publish && make deploy`.
The selected application command is NOT READY until that repair completes.
