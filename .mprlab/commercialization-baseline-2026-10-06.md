# Evaluation Baseline: October 6, 2026

## Source identity

The primary checkout is `/Users/tyemirov/Development/llm-proxy`.
Its origin is `git@github.com:tyemirov/llm-proxy.git`.
The branch is `master`. The commit is `91ba581ba5763ce99c38a31f42a1148a06e8966f`.
The checkout has concurrent provider, client, test, and document changes.
The pre-existing tracker edit concerns F027 HeyGen work. This evaluation preserved that edit.

The recorded baseline covered 669 source files, including untracked source files.
The combined source fingerprint is `641432ba9e52fbedd238ab0d5fb4b4d201aa79c6ca732d90e60737773e93d57e`.
The before and after hashes matched. This result identifies the tested working state, rather than the commit alone.
The [source manifest](evidence/commercialization-2026-10-06/source-before.json) contains individual hashes.
The [run manifest](evidence/commercialization-2026-10-06/baseline-manifest.json) records the fingerprint and capture time.
Private environment files and credentials are outside the fingerprint.

## Checks executed

The recorded run used these environment settings:

```bash
GOCACHE=/tmp/llm-proxy-evaluation-go-cache GOPROXY=off GOSUMDB=off
```

The tests used existing local protocol fixtures and temporary databases. External model calls and expenditure were zero.
The [test output](evidence/commercialization-2026-10-06/baseline-tests.txt) contains the executed commands and package results.

| Existing target | Observed result | Evidence limit |
| --- | --- | --- |
| `make test-mcp` | Passed, proxy package 12.408 seconds | Tests discovery, generation, OAuth boundaries, errors, cancellation, capacity, media, and usage with controlled dependencies. |
| `make test-account-connections` | Passed, proxy package 3.822 seconds | Tests connection ownership, routing, persistence, and failure boundaries. It does not prove customer provider onboarding. |
| `make test-completion-measurements` | Passed, proxy 0.357 seconds, Go client 0.099 seconds | Tests successful token measurements and absence of invented measurements on failure. |
| Focused `make test-hosted-access` selection | Passed, proxy package 1.732 seconds | Its pattern selected existing BYOK loss, failure pagination, cursor scope, and tenant-isolation tests. It did not select hosted financial acceptance. |
| `make test-hosted-runtime`, earlier local run | Passed, proxy 39.399 seconds, CLI 1.167 seconds | Controlled hosted runtime checks passed. This earlier run has no before/after fingerprint and remains separate supporting evidence. |

The focused pattern was:

```text
^Test(ManagedUsageWriterKeepsPublicResponsesIndependentFromPersistence|ManagementUsageFailuresRejectsInvalidQueriesAndEnforcesTenantOwnership|ManagementAccountUsageRejectionsAggregateOwnedTenantsAndBindCursorsToScope|ManagementUsageFailuresExposeSafeCanonicalRowsWithStableSnapshotPagination|ManagementTenantConfigurationSecretAndUsageIsolation)
```

Use `HOSTED_TEST_PATTERN` with `make test-hosted-access` to select these existing tests.
The target name is broader than this selection. The recorded command shows its actual test scope.

The first sandbox attempt failed before meaningful acceptance:

```text
TestMCPDictatorWorkflow: listen tcp 127.0.0.1:0: bind: operation not permitted
httptest: listen tcp6 [::1]:0: bind: operation not permitted
make: *** [test-mcp] Error 1
```

The approved retry allowed temporary local listeners. The subsequent tests passed.
No product change or longer test timeout corrected this environment restriction.

## Source and document findings

- `mcp.go`, `mcp_generation.go`, and `mcp_media.go` register five tools. They do not register a usage-inspection tool.
- `management_api.go` registers account and tenant usage, failure, and rejection reads.
- `management_usage_writer.go` drops new events when its queue is full. It attempts each database insert once.
- `management_usage.go` copies token quantities only when usage exists. `managedUsageEventRecord` retains integer defaults without measurement presence.
- `management_api.go` exposes token totals without measured/unknown request counts.
- `README.md` explicitly describes process-local, at-most-once telemetry before database commit.
- `docs/hosted-billing.md` reports F065 through F070 development acceptance on September 26, 2026.
- That document reports complete hosted tests and stack CI. This task did not rerun those complete lanes.
- Hosted activation and live financial qualification remain outside this evaluation. No deployed-runtime acceptance was established.

The source findings extend the earlier audit. They do not establish live provider reliability or complete usage coverage.
The operational summary cannot establish token measurement completeness from its current integer totals.

## Planning and validation state

The task read root guidance, policy, planning, documentation, terminology, issue format, runtime, and relevant product documents.
No repository `.agents/skills` directory exists. The user-level `.agents/skills` inventory contained only the unrelated Hugging Face CLI skill.
The task used `mprlab-governor` from the local Codex skills directory.
Local Codex memories supplied checkout history and existing F021 context. Current source took precedence over historical records.

The official ASD-STE100 Issue 9 PDF passed the reference hash check.
Its SHA-256 was `d1f4ea9e7cd6e46b47aa9057209f99e78c0e9cfc4e27a5b07895b05c1a166431`.
The first script invocation failed because the default uv cache was outside the write boundary.
The direct Python invocation used the existing verified reference without cache writes.

The Governor dry run found six pre-existing managed-content differences:

- `.gitignore`
- `.mprlab/AGENTS.DOCKER.md`
- `.mprlab/AGENTS.FRONTEND.md`
- `.mprlab/AGENTS.GO.md`
- `.mprlab/PLANNING.md`
- `.mprlab/POLICY.md`

The initial uv attempt could not retrieve PyYAML because sandbox DNS access failed.
The approved read-only check used an existing local Python environment and the public issue-format endpoint.
The issue-format source SHA-256 was `adc2ef25b029445f719358cbfda8e6425dd60869a831efb50d6c7142e492c2a2`.
The final Governor check retained the same six differences. Its exit status was 1 for existing drift.
The [Governor result](evidence/commercialization-2026-10-06/governor-check.json) records these differences.
No normalization writes occurred.

Changed prose received a scoped review against Part 1 rules and Part 2 word meanings.
Mechanical checks cover both new documents, the terminology additions, and P015 through P017.
The unchanged tracker text retains 72 mechanical findings. The terminology document has no mechanical findings.
This task makes no full-document compliance claim for the existing documents.
The first `git diff --check` reported `new blank line at EOF` in the added tracker tail.
The task removed its extra newline. The final whitespace check passed.
The active tracker and archive contain 578 unique identifiers with correct section letters.
The temporary execution plan was removed after review.

The required architect review used native agent `evaluation_architect` with requested assignment `gpt-6-astra/high`.
The tool did not expose independent confirmation of the actual model and effort.
The charter includes the review findings on workflow selection, source identity, fair comparison, telemetry coverage, and decision thresholds.
The final independent review found no blocking defects. It refined diagnosis timers and equivalent ownership effort.

## Pilot blockers and next decision

1. Select the real workflow, owner, request fields, and two routes under P015.
2. Approve evaluation thresholds and resolve time and budget decisions before dependent work.
3. Execute the direct/proxy/LiteLLM comparison under P016. Relative value is currently unknown.
4. Resolve absent-versus-zero token measurements for any offer that promises honest aggregate token coverage.
5. Obtain owner acceptance of event loss. Reject invoice-grade requirements for ordinary BYOK telemetry.
6. Record demand, willingness to pay, support ownership, and external-operation authority before a pilot decision under P017.

The smallest conditional engineering candidate preserves measurement presence and exposes measured/unknown counts through existing usage summaries.
It needs a separate approved implementation issue. MCP access does not correct this data limitation.
This task completed week-one planning and the local baseline. The six-week evaluation remains open.

## Supplied discovery research

The follow-up research identified Postiz and Parseur as discovery candidates. Neither has verified purchasing intent or a validated recurring problem.
The charter records their narrow workflow candidates and cheaper direct remedies. The commercial gate remains unmet.
P016 compares the smallest direct provider/configuration patch before additional product work.

## Authorized week-one continuation

The user selected synthetic structured extraction after this baseline.
The [LP-C01-S1 record](commercialization-benchmark-LP-C01-S1.md) contains 24 local observations and the retained comparison harness.
Both executable alternatives passed the extraction contract on both selected routes.
Persisted token values collapse missing usage and measured zero. Immediate responses preserve that distinction.
This limitation does not block the selected extraction workflow. It limits historical token-coverage claims.
Customer qualification and the full comparison remain open. No commercial gate passed.
