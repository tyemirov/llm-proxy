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
