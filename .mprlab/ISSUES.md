# ISSUES

Entries record newly discovered requests or changes.

Read @AGENTS.md (Workflow section), @POLICY.md, and relevant stack guides before implementing changes.

Format: `- [ ] [B042] (P1) {I007} Title`

- `[ ]` open, `[-]` taken, `[!]` blocked, `[x]` closed.
- Blocked issues (`[!]`) must include a `Blocked:` line in the body.

Resolved history: `.mprlab/ISSUES-ARCHIVE.md`; the complete original issue
bodies, resolution notes, and validation records remain in `v0.2.43`.

Triage, 2026-07-24: B069 is the immediate active regression. F014 has no
remaining active prerequisites and unblocks I027 and P001. I029 is the
remaining contract predecessor for I031; I031 also waits for F014. M019 is now
independently ready because M018 is complete. M013 then M012 resolve the
product-context governance path. Planning proceeds P002 -> P003 -> P004 ->
P005, with M020 already satisfied; recurring maintenance remains scheduled
work.

## BugFixes

- [ ] [B069] (P1) Restore the long v2 semantic-review completion contract after request-level effort rollout.
  ### Summary
  The redeployed production endpoint accepts the canonical optional `reasoning_effort` field, but a production-sized F001 source-world request still outlives the caller without returning either the final response or a structured proxy timeout. A small request using the same endpoint, credentials, model, and explicit `high` effort succeeds, so this is the long-request boundary previously covered by B016, B017, and B018 rather than request-shape rejection or total service unavailability.

  ### Impact
  Kamu F001 cannot begin Bulgarian Creative Director source-world review. The failed one-shot request produced no durable response or safe completion receipt, so the caller cannot distinguish a still-running provider operation from a failed operation and must not retry it.

  ### Production evidence
  - On 2026-07-24, after the effort-capable deployment, a sanitized `POST /v2` control using `gpt-5.5`, `reasoning_effort: "high"`, `max_tokens: 16`, and `Reply exactly: OK` returned `OK` in about 2.6 seconds.
  - Creative Director was then upgraded to released `github.com/tyemirov/llm-proxy v0.2.42`. No-spend HTTP coverage proves the F001 delivery-profile value is serialized as canonical `reasoning_effort: "high"` and that omitted configuration remains absent.
  - The exact F001 profile passed Creative Director validation at SHA-256 `eec521a661b9f64886bfb7bbc232752f0947c6067ffa33dddec15d5a5966e581`. The selected `neblagodarnost` canonical source is 15,845 bytes with SHA-256 `45da648ff5dd21d417c65ad8e022919e8f761d52ad628d017192bf5095f6d58d`.
  - The canonical pipeline request started with inherited `LLM_PROXY_SECRET`, `LLM_PROXY_BASE_URL`, and `LLM_PROXY_MODEL` unset so the project-owned dotenv configuration was authoritative:
    ```bash
    env -u LLM_PROXY_SECRET -u LLM_PROXY_BASE_URL -u LLM_PROXY_MODEL \
      /Users/tyemirov/Development/Smith/creative-director/bin/creative-director pipeline \
      --project-profile /Users/tyemirov/Documents/Projects/Kamu/configs/creative-director/kamu-tales.json \
      --story-id neblagodarnost \
      --mode storyboard \
      --delivery-profile kamu-storyboard \
      --execute \
      --from source-world \
      --to source-world \
      --confirm-provider-spend \
      --json
    ```
  - At `2026-07-24T21:51:55Z`, after the configured 300-second caller deadline, the pipeline stopped with stable code `source_world_review_llm_proxy_http_error` and the sanitized error:
    ```text
    llm_proxy_client_http_failure: post request: Post "[redacted-url]": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
    ```
  - No source-world prompt, review, or manifest was promoted by the failed transaction. The pipeline wrote only its blocked execution state and did not retry the provider request.
  - Public verification after the failure resolved `llm-proxy-api.mprlab.com` to the expected gateway and returned the expected Caddy-served `403` health response from `/` plus `200` from `/config-ui.yaml`; the service was not generally unavailable.
  - Read-only inspection of the files staged on the gateway host confirms `request_timeout_seconds: 360`, `response_header_timeout 420s`, and `read_timeout 420s`. The running container image digest could not be inspected without the gateway sudo boundary, and no sudo or deployment command was attempted.
  - The signed-in one-day usage dashboard records three `openai / gpt-5.5` requests but exposes only aggregate counts, so it cannot identify whether the abandoned long request later completed or reached the proxy deadline.
  - Released `v0.2.43` contains only client-authentication documentation/site work and does not address this issue.

  ### Confirmed boundary defect and remaining investigation
  B016 and B017 established a one-request REST contract in which LLM Proxy owns OpenAI background polling, the proxy runtime owns the terminal or structured timeout, packaged clients wait longer than the server, and the gateway waits longer than clients. The deployed files preserve the 360-second server and 420-second gateway deadlines, but this existing product profile supplies a 300-second client deadline. That ordering guarantees the caller can abandon a valid in-flight proxy request before LLM Proxy owns completion or returns its structured timeout.

  The reusable Go package does not currently expose the documented 390-second supported-client deadline as a public constant or default. `llmproxyclient.ConfigInput` requires every application to supply a positive duration, while `390 * time.Second` exists only as a private `llm-proxy-client` command constant and a README example. Creative Director therefore cannot consume one owner-defined timeout contract; its package default is 120 seconds and the Kamu profile duplicates a 300-second product value.

  Fix this contract defect first. Production-safe correlation must then establish whether the explicit-effort workload also reveals a provider/background-lifecycle regression after a supported client remains connected through the proxy deadline. Possible remaining failures are:
  1. the deployed effort-capable route no longer completing the background polling lifecycle for this workload;
  2. runtime/gateway timeout ordering drifting from the released contract; or
  3. the provider legitimately reaching the proxy-owned deadline and LLM Proxy returning a structured timeout.

  The investigation must establish the terminal boundary from production-safe correlation evidence. Do not assume that merely raising a product-local timeout proves or repairs the owning defect.

  ### Requirements
  1. Reproduce a production-comparable `POST /v2` request with an approximately 16 KB source-world prompt, `gpt-5.5`, and explicit `reasoning_effort: "high"` through the released public boundary.
  2. Prove from safe request correlation that LLM Proxy received `high`, resolved the expected provider/model, entered the OpenAI background lifecycle, and either reached a terminal response or its own configured deadline.
  3. Restore deterministic timeout ordering so the proxy returns the final body or a structured proxy/provider error before any supported caller or gateway closes the connection. Expose one canonical supported-client deadline from the Go client package so Creative Director can consume the owner contract and Kamu can remove its product-local magic number.
  4. Preserve the one-shot public REST contract from B017. Do not add product-side retries, prompt chunking, tenant-default mutation, provider-specific fields, direct OpenAI calls, or a Kamu-only timeout exception.
  5. Add production-comparable black-box coverage for the explicit-effort long-request path and retain the existing small-request behavior.

  ### Acceptance criteria
  - The exact F001 `neblagodarnost` source-world canary returns a final review body or a structured, attributable proxy error; it never ends as an opaque client `awaiting headers` timeout.
  - A small explicit-`high` v2 control continues to return HTTP 200.
  - The effective server, supported-client, and gateway deadlines are verified at the deployed boundary.
  - `make ci` passes and the released/deployed artifact is verified before F001 retries the request.

## Improvements

- [ ] [I031] (P1) {I029,F014} Add tenant-scoped failure details to the usage dashboard.
  Goal:
  Let a signed-in tenant owner open a selected-period **failed requests** link
  from the success-rate metric and inspect safe, individual failure metadata.
  A 55% success rate over 22 requests means 10 failed requests, but that is not
  necessarily 10 provider failures: client validation and upstream/runtime
  failures must remain distinguishable.

  Evidence:
  - Managed usage rows already retain event time, endpoint, provider, model,
    HTTP status, success, and latency, while the current usage response already
    aggregates `status_codes`; neither surface retains or renders a per-event
    failure reason.
  - F014 replaces the current singular management/usage API with canonical
    tenant-scoped routes, and I029 makes OpenAPI the sole HTTP contract source.
    A new unscoped endpoint now would be immediately obsolete.

  Requirements:
  - Implement only after I029 and F014. Use F014's canonical tenant-scoped
    management contract; do not add an unscoped endpoint, alias, compatibility
    response, dual read, or client fallback.
  - Add exactly one owner-only operation:
    `GET /api/management/tenants/:tenant_id/usage/failures`.
    It requires exactly one `interval=all|30d|7d|1d`, accepts optional single
    `limit` (default 25, inclusive range 1-100) and opaque `cursor`, and
    rejects missing, repeated, malformed, or unknown query fields with `400`.
    Scope every query by both authenticated owner and tenant id; a missing or
    foreign tenant returns the same `404` used by F014.
  - Return newest-first failure rows with a stable `(created_at, id)` cursor and
    an opaque snapshot boundary. Each row contains only `occurred_at`,
    `endpoint`, `provider`, `model`, `status_code`, `outcome_code`, and
    `latency_ms`; do not expose row ids, tenant/user ids, prompts, responses,
    audio, transcripts, client secrets, provider keys, raw upstream bodies, or
    free-form error text.
  - Introduce one nonblank canonical `outcome_code` for every usage event:
    `success`, `invalid_request`, `payload_too_large`, `rate_limited`,
    `service_unavailable`, `request_timeout`, or `upstream_error`. Construct it
    at the request/error boundary alongside the HTTP status, persist it once,
    and use it for the details response. Never persist `error.Error()` or any
    provider response text.
  - Add one bounded, versioned GORM migration after F014's tenant migration.
    Populate every historical usage row from its status: successful rows become
    `success`; `400`, `413`, `429`, `503`, `504`, and `502` map to their exact
    canonical failure codes. Preflight and reject a row with any other status
    rather than leaving a blank/unknown value. Keep one current schema and add
    the tenant/success/time/id index needed for the failure-page query.
  - Refactor managed validation and upstream-error recording so every
    authenticated managed proxy outcome has an explicit code before it is
    written. Keep the caller's current response behavior intact, but do not use
    its message as dashboard telemetry.
  - In the user dashboard, keep the existing selected interval and render a
    visible, keyboard-operable **N failed requests** action inside the success
    metric only when failures exist. It opens a semantic, focus-managed failure
    details dialog with the existing non-success status-code breakdown, safe
    code/status labels, rows, loading/empty/error states, and a **Load more**
    action. A failed details request clears only that dialog's state and cannot
    replace a current tenant's main dashboard data.
  - Derive user-facing labels only from centralized frontend copy/constants or
    backend payload values. Preserve F014 tenant request identity/cancellation
    rules so an out-of-order details response cannot appear for a different
    tenant or interval. Keep the admin surface aggregate-only; it must not
    expose another tenant's failure rows.
  - Update `docs/openapi.yaml`, management types/client, README, provider
    routing documentation, and any generator-owned public usage documentation
    from the final single API contract. Document that historical rows receive
    normalized status-derived codes, not reconstructed raw error messages.

  Deliverables:
  - Tenant-scoped failure-event query, cursor domain type, outcome-code domain
    type, indexed current-schema migration, and safe request-boundary recording.
  - The failure-details dialog, central copy/types/client methods, responsive
    styles, and selected-interval/status-summary presentation.
  - Canonical OpenAPI and product documentation describing the operation,
    response, privacy boundary, and safe diagnostic vocabulary.

  Validation:
  - Add black-box HTTP coverage through the real management router for interval
    validation, owner/tenant isolation, newest-first stable pagination, all
    supported outcome codes, zero failures, and absence of every prohibited
    sensitive field.
  - Exercise public managed proxy requests that yield validation, payload-size,
    rate-limit, unavailable, timeout, and upstream failures; prove each stores
    the exact safe code and a success stores `success`, without persisting a raw
    error message.
  - Run the real SQLite and PostgreSQL migration paths with historical events;
    prove exact code backfill, index creation, totals preservation, contextual
    rejection/rollback for an unsupported status, and no obsolete nullable or
    compatibility path.
  - Add Playwright coverage for 10 failures from 22 requests, zero-failure link
    absence, interval changes, tenant switching, pagination, loading/error and
    stale-response states, keyboard/focus behavior, mobile layout, and no
    secret-bearing or raw-error text in rendered DOM or browser storage.
  - Prove the exact new operation and exchanges conform to `docs/openapi.yaml`,
    then run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair, with the final run after the
    last code edit.

- [ ] [I029] (P1) Publish one canonical OpenAPI contract and enforce server/client conformance.
  Goal:
  Make one committed OpenAPI 3.1 document the sole canonical HTTP wire
  contract for every llm-proxy-owned endpoint, publish that exact artifact on
  the public site, and make CI reject drift in handlers, bundled clients, or
  human-facing API documentation.

  Evidence:
  - The repository currently has no OpenAPI or Swagger artifact and no
    contract-conformance gate.
  - The public site and API origin return `404` for the conventional OpenAPI
    and interactive-documentation paths.
  - The published canonical-v2 prose page omits the request-level
    `reasoning_effort` field delivered by B068, demonstrating that independently
    maintained prose can drift from the live wire contract.

  Requirements:
  - Add exactly one hand-maintained canonical contract at
    `docs/openapi.yaml`. Do not add aliases, fallback schema locations,
    generated source copies, legacy operations, compatibility fields, or a
    second independently editable contract.
  - Describe every llm-proxy-owned public proxy, configuration, and management
    operation, including methods, paths, query parameters, headers, request
    bodies, multipart parts, response bodies, response headers, content types,
    authentication, and every intentionally returned status code. Exclude
    TAuth-owned endpoints from the llm-proxy contract.
  - Define the current `/v2` request precisely, including `model`, `messages`,
    `web_search`, `max_tokens`, and `reasoning_effort`; preserve the B068
    distinction between omission and an explicit non-blank value, and document
    route capability validation and the canonical error response.
  - Define security at the actual boundary: the tenant client key query
    parameter for proxy operations and the TAuth session cookie for management
    operations. Examples and fixtures must never contain real credentials,
    tenant identifiers, or user data.
  - Treat a server or client wire-contract change as incomplete unless the same
    change updates `docs/openapi.yaml`. CI must fail when a registered
    llm-proxy route is absent from the contract, a contract operation has no
    registered handler, or an exercised request/response/status/header/content
    type violates the contract.
  - Enforce conformance through black-box tests at the public HTTP boundary.
    Load the real router and handlers, compare their operation inventory with
    the OpenAPI operations, and validate representative success and error
    exchanges against the schemas. Permit only explicitly documented protocol
    handling such as `OPTIONS`; do not substitute isolated schema unit tests.
  - Make the bundled Go package, Python package, and Go CLI prove compliance
    with the same artifact. Their real serialized `/v2` requests and parsed
    success/error responses must cover tenant-key placement, provider
    selection, query stripping, model, messages, web search, token limits, and
    reasoning effort. CI must reject an undocumented field or a missing current
    field; do not generate compatibility clients or preserve obsolete shapes.
  - Publish the byte-equivalent canonical artifact at
    `https://llm-proxy.mprlab.com/openapi.yaml` and publish a human-readable
    reference at `https://llm-proxy.mprlab.com/docs/` derived from that
    artifact. The contract's server URL must identify
    `https://llm-proxy-api.mprlab.com`; the API origin must not become a second
    schema source.
  - Build publication from the committed artifact, record enough provenance to
    prove the deployed file came from the release source, and fail release
    validation when the Pages artifact differs. Link both forms from the site
    navigation/resource index and include the documentation page in the
    sitemap.
  - Remove independently maintained endpoint and field inventories from prose
    API resource pages, or derive and verify them from the OpenAPI artifact.
    In particular, the canonical-v2 documentation must expose
    `reasoning_effort` without creating another source of truth.

  Deliverables:
  - The canonical `docs/openapi.yaml` contract and documented ownership/update
    rule.
  - Server-route and HTTP-exchange conformance checks wired into `make ci`.
  - Go, Python, and CLI client conformance coverage wired into `make ci`.
  - The exact published schema, derived human-readable reference, navigation,
    sitemap, and release/publication verification.
  - Updated repository and site documentation that points contributors and
    clients to the canonical contract.

  Validation:
  - Run the required baseline `make ci` before the first implementation edit
    and the required final `make ci` after the last edit.
  - Prove the route inventory is bidirectionally complete and representative
    real-handler exchanges validate against `docs/openapi.yaml`.
  - Prove all three bundled clients serialize and consume the current `/v2`
    contract, including omitted and explicit `reasoning_effort`.
  - Prove the generated Pages artifact contains the byte-equivalent canonical
    schema and derived documentation, with no independently generated copy.
  - After the user-owned production deployment, capture live `200` responses
    for `/openapi.yaml` and `/docs/` and verify the published schema matches the
    released source artifact.

- [ ] [I027] (P1) {F014} Redesign the user dashboard around connected-provider widgets.
  Goal:
  Make the authenticated dashboard answer, at a glance, which upstream
  providers the current tenant has connected. Preserve usage reporting as a
  separate measure of activity so an unused connected provider remains visible
  and historical traffic never implies that a provider is still connected.

  Dependencies:
  - F014 replaces the singular profile and usage contracts with selected-tenant
    APIs. Build the widgets against that canonical tenant-scoped boundary rather
    than implementing and immediately replacing a singleton-profile join.

  Requirements:
  - Define a connected provider solely as an entry in the current authenticated
    management profile whose canonical `has_key` value is `true`. Do not infer
    connection from catalog membership, aliases, routing defaults, local
    environment credentials, or a provider's presence in historical usage.
  - Add a prominent `Connected providers` section to the user usage dashboard
    and render exactly one widget for each connected provider, in the
    deterministic order returned by the management profile. Do not hard-code
    provider names or duplicate provider-registration state in the browser.
  - Give each widget a concise, consistent summary: the profile label,
    `Connected` status, saved text model, declared text/dictation capabilities,
    and current-period request and token totals matched by exact canonical
    provider ID. A connected provider with no usage in the period must still
    render with zero activity; a usage-load failure must render as unavailable,
    not as a false zero or a disconnected provider.
  - Add a provider-specific `Manage` action that opens Settings with that exact
    provider selected. It must not reveal a key, invoke the key-reveal endpoint,
    or alter provider/default settings merely by opening the editor.
  - Replace the ambiguous usage-derived `Providers` summary metric with a
    `Connected providers` count derived from the same `has_key` projection.
    Keep provider/model usage breakdowns explicitly labeled as activity for the
    selected reporting period, including historical rows for providers that are
    no longer connected.
  - Render a purposeful empty state when no providers are connected, with one
    action that opens Settings. The state must coexist with mandatory onboarding
    and must not create a path around its persisted-key requirements.
  - Keep the widgets synchronized with the profile: a successful provider-key
    autosave adds its widget, a successful removal removes it, failed mutations
    leave the current projection unchanged, and dashboard refresh reloads both
    current profile state and usage. Never let an out-of-order response restore
    stale connection state.
  - Treat widgets as non-secret metadata. Never render provider API keys,
    masked-key suffixes, client keys, system prompts, or credential-bearing
    values in widget text, attributes, accessible names, or browser storage.
  - Use semantic headings and per-provider articles, unique accessible action
    names such as `Manage OpenAI`, full keyboard operation, and a responsive
    grid that remains aligned without horizontal overflow on narrow screens.
    Keep the provider widgets confined to the current user's dashboard; the
    admin dashboard must not project another tenant's provider credentials or
    connection state.
  - Consume the existing management-profile and usage contracts unless a
    demonstrated missing field requires one canonical contract change. Do not
    add a parallel provider-registration endpoint, cached shadow state,
    compatibility aliases, or fallback matching.
  - Update dashboard and self-service documentation so `connected provider` and
    `active provider` have explicit, non-overlapping meanings.

  Deliverables:
  - Add the connected-provider widget grid, connected count, provider-specific
    Settings navigation, empty/error states, and responsive styling to the user
    dashboard.
  - Add one derived presentation model that joins profile providers to usage by
    exact canonical ID while keeping registration authoritative to `has_key`.
  - Update first-party frontend types, copy, documentation, and rendered-browser
    coverage for the final dashboard contract.

  Validation:
  - Add Playwright scenarios for zero, one, and multiple connected providers;
    deterministic widget order; a connected provider with zero activity; an
    unconnected provider with historical activity; exact model/capability and
    usage rendering; and the connected-provider count.
  - Prove successful key autosave/removal and dashboard refresh update the
    widgets, while rejected or out-of-order requests do not mutate the visible
    projection and usage failure leaves connection state intact with activity
    marked unavailable.
  - Prove each `Manage` action selects the intended provider without a reveal or
    mutation request, no secret-bearing value reaches the rendered dashboard or
    browser storage, and admin/user dashboard switching preserves isolation.
  - Cover keyboard navigation, accessible names, and desktop/narrow viewport
    layout without overlap or horizontal overflow.
  - Run the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci`
    pair for the implementation, with the final run after the last code edit.

## Maintenance

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
  Last run: 2026-07-24. Archived 119 resolved one-off issue blocks in
  `.mprlab/ISSUES-ARCHIVE.md`, preserving their exact bodies, resolutions, and
  validation records at `v0.2.43`. Historical `M009R` was reclassified as the
  completed one-off `M009`; all active, blocked, Planning, and recurring entries
  remain in this tracker. Removed satisfied archived dependencies from I029,
  M019, F014, and P005.
- [ ] [M002R] (P2) Polish open issues.
  Goal:
  Keep unresolved work executable by making each open issue concrete, ordered, and testable.
  Requirements:
  - Cadence: run weekly during active development and before handing a repo to automated execution.
  - Review every unresolved non-recurring issue for missing context, dependencies, repro steps, acceptance criteria, and validation expectations.
  - Make priorities concrete and ensure each open issue has actionable deliverables.
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
  Last run: 2026-07-23. Polished 14 non-recurring unresolved entries. B063 is
  explicitly blocked on an operator-owned deployment; M018 is P0 after a
  pinned-toolchain scan found reachable GO-2026-5970; F013, F014, and I027 now
  have their implementation order recorded. Planning entries remain open but
  deferred under the repository workflow.
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
  Last run: 2026-07-23 triage follow-up. Release `v0.2.39` and GHCR `latest`
  are current, while the public Pages marker remains at `v0.2.38`; B063 records
  the exact activation boundary. The live API management boundaries responded
  as expected, but no production deployment was performed.
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
  Last run: 2026-07-23 triage follow-up. The live API returned the expected
  anonymous proxy (`403`), configuration (`200`), and management (`401`)
  boundaries. The public Pages marker is still `v0.2.38` while release `v0.2.39`
  is published; B063 is blocked on the operator-owned deployment flow.
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
  - Check links, command names, paths, and public contract descriptions touched by the pass.
  - Confirm docs describe the current canonical path only.
  - Confirm issue archive and active tracker references remain consistent.
  Last run: 2026-07-24. Reviewed README, implementation notes, deployment
  manifest, CHANGELOG, release docs, marketing report, and generated public
  resource sources. Corrected the B068 request-level `reasoning_effort`
  contract across REST, Go, Python, CLI, and public-resource documentation;
  recorded the current Google Analytics and LoopAware public-page telemetry in
  the README and marketing report; and added the required legal-disclosure scope
  to P005. `PRD.md` and `ARCHITECTURE.md` remain absent, so M013 is the canonical
  product-context follow-up.
- [ ] [M012] (P2) {M013} Reconcile repository governance with the MPR Lab normalizer.
  Goal:
  Make the governance normalizer check pass without deleting repository-owned binding contracts.
  Requirements:
  - Resolve M013's product-context document decision first so the normalizer
    works from the final repository-owned root guidance.
  - Inspect the normalizer differences reported for root `AGENTS.md` and every managed `.mprlab/` guide.
  - Preserve the M011 pre-change and post-change CI requirement and all other current repository-owned rules.
  - Update the appropriate managed templates, boundaries, or repository documents as one canonical forward-only contract rather than applying a destructive bulk rewrite.
  Deliverables:
  - A reviewed governance normalization change with no unrelated product or runtime edits.
  Validation:
  - Run the MPR Lab governor in `--dry-run` and `--check` modes and require no pending managed-file changes.
- [ ] [M013] (P2) Resolve missing product-context document references.
  Goal:
  Keep the root governance entrypoint limited to product-context documents that exist and represent the current contract.
  Requirements:
  - Decide whether current `PRD.md` and `ARCHITECTURE.md` documents are required or whether their references are stale.
  - Add current canonical documents or remove the obsolete references; do not add placeholders or compatibility documents.
  Deliverables:
  - Root governance references that resolve to current product-context files.
  Validation:
  - Verify every product-context path named by root `AGENTS.md` exists and contains current repository guidance.

- [ ] [M019] (P2) Refresh non-security direct dependency pins.
  Goal:
  Bring direct Go, frontend, and Python development dependencies to their current supported releases after the security graph is stable.
  Requirements:
  - Evaluate the observed direct-version drift for Gin, JWT, Viper, TAuth, Zap, GORM, Alpine, js-yaml, mypy, and pytest against their current contracts.
  - Upgrade compatible current releases in one canonical lockfile/module state; do not preserve stale dependency aliases or parallel versions.
  - Keep generated client, browser, and release behavior covered through their real repository entry points.
  Deliverables:
  - Updated Go module graph, npm lockfile, and Python lockfile only where the selected current contract requires them.
  - A concise compatibility note for any package intentionally left at its current supported version.
  Validation:
  - Run `go mod verify`, `npm audit --json`, the locked Python audit, and the required baseline/final `timeout -k 350s -s SIGKILL 350s make ci` pair.

## Features

- [ ] [F014] (P1) Support multiple isolated tenants per managed user.
  Goal:
  Let one authenticated TAuth user create, select, rename, and delete multiple independently configured LLM Proxy tenants. Each tenant owns its own generated client secret, provider credentials and settings, routing defaults, request examples, and usage history. This feature is one-user-to-many-tenants; shared tenants, invitations, memberships, and team roles are outside scope.
  Current contract:
  - `managedTenantRecord.UserID` is currently the primary key, `managedTenantID` is derived only from that user id, and profile hydration finds or creates exactly one tenant by user id.
  - Provider-key records and AES-GCM associated data are keyed by user id plus provider id, while usage summaries are queried by user id even though usage rows also carry a tenant id.
  - `/api/management/profile`, defaults, secrets, provider-key operations, usage, administrator responses, frontend state, and the Settings `Client access` section all assume one singular tenant.
  Requirements:
  - Separate authenticated account identity from tenant state. Persist one managed user keyed by the validated TAuth subject and any number of managed tenants keyed by stable opaque tenant ids with an owner-user foreign key.
  - Give every tenant a required editable display name. Normalize and validate names once at the HTTP/database edge, allow 1-80 visible characters after trimming, and enforce case-insensitive uniqueness within one owner's tenants. Different users may use the same name.
  - Generate unpredictable immutable ids for new tenants with bounded collision handling; do not derive a new tenant id solely from the owner id. Preserve every existing tenant id during migration so deployed client secrets and operational references keep their identity.
  - Maintain the invariant that every managed user has at least one tenant. Create one tenant named `Default` on first authenticated access for a new user, and reject deletion of an owner's final tenant with `409 Conflict`.
  - Make tenant id, rather than user id, the ownership key for secret digests, provider-key rows, provider-specific models/system prompts, routing defaults, and usage events. Bind provider-key AES-GCM associated data to tenant id plus provider id so ciphertext cannot be moved between two tenants owned by the same user.
  - Keep non-empty generated-secret digests globally unique and mapped to exactly one tenant. Public proxy requests continue to authenticate only with `key=<generated secret>` and require no tenant parameter; resolving the secret selects that tenant's credentials, defaults, and usage owner.
  - Preserve strict tenant isolation at the database query boundary. Every tenant-scoped management query and mutation must constrain both authenticated owner user id and tenant id. Return the same `404 Not Found` for a missing tenant and another user's tenant so identifiers cannot be enumerated.
  - Keep administrators read-only with respect to tenant ownership. Replace the singular admin tenant shape with an ordered `tenants` collection and tenant count per user; show each tenant's facts and existing 30-day usage summary without exposing provider keys, masked key material, secret digests, generated secrets, prompts, responses, audio names, or transcripts.
  Migration:
  - Add one bounded, versioned, all-or-nothing GORM migration for both supported SQLite and PostgreSQL management databases. Do not add raw-SQL persistence, dual reads/writes, a runtime fallback to the old schema, or a compatibility response shape.
  - Preflight the complete current dataset before mutation: require unique nonblank user and tenant ids, valid B036 provider/model pairs, matching tenant ids on usage rows, no orphan provider or usage rows, and successful decryption of every provider key. Reject any remaining `static-config:<tenant-id>` owner with a contextual instruction to complete the F011 ownership claim first.
  - Create one managed-user row for every current authenticated owner and one owned tenant row for that user's existing record. Name each migrated tenant `Default`; preserve tenant id, secret digest, defaults, provider settings, creation/update timestamps, and all usage fields and timestamps exactly.
  - Move provider records from the user foreign key to the preserved tenant id and decrypt/re-encrypt each key from the old user/provider associated data to the new tenant/provider associated data inside the migration boundary. Move usage ownership to its existing tenant id and remove user id as an independent usage-partition key.
  - Verify source/destination row counts, ownership, referential integrity, secret digests, defaults, decrypted provider values, and per-tenant usage totals before committing. Any failure must roll back the whole migration with operation, table, user, tenant, and provider context as applicable.
  - After the migration is verified, drop the obsolete one-to-one columns/tables and delete the old user-keyed store operations and temporary migration bridge. The running application must understand only the new schema and tenant-scoped contract.
  Management API:
  - Replace singular bootstrap with `GET /api/management/account`, returning the authenticated user plus a stable creation-ordered list of tenant summaries. Delete `/api/management/profile`; do not retain it as an alias.
  - Add canonical owner-only tenant lifecycle endpoints: `POST /api/management/tenants`, `GET /api/management/tenants/:tenant_id`, `PUT /api/management/tenants/:tenant_id`, and `DELETE /api/management/tenants/:tenant_id` for create, hydrate, rename, and delete.
  - Move every tenant operation under that resource: usage, defaults, secret creation/revocation, provider-key save/remove, and I025 provider-key reveal must use `/api/management/tenants/:tenant_id/...`. Delete the former unscoped endpoints instead of keeping compatibility routes.
  - Treat tenant creation, rename, deletion, provider mutation, default mutation, and secret mutation as transactional operations with validated tenant-id/name domain types and explicit stable errors. Tenant deletion must cascade its digest, provider settings, and usage only after explicit confirmation at the UI boundary.
  - Keep F013's exact interval query and bucket semantics on the tenant-scoped usage endpoint. A user aggregate is not a substitute for the selected tenant dashboard, and concurrent tabs selecting different tenants must not share server-side active-tenant state.
  UI and interaction:
  - Add one compact, keyboard-operable tenant switcher directly below the shared MPR header and above the authenticated dashboard so the active tenant is always visible outside Settings. Show the display name as the primary label and the immutable tenant id as secondary context.
  - Store the active tenant in the page URL as `tenant=<tenant-id>` so reload, browser history, bookmarks, and independent tabs preserve explicit context. When the parameter is absent, select the oldest tenant returned by the account bootstrap and write it with `history.replaceState`; when a supplied id is invalid or unauthorized, show an explicit workspace error rather than silently choosing another tenant.
  - Add an accessible create-tenant dialog with focused name input and inline validation. Select the new tenant after creation and update the URL without reloading the MPR authentication shell.
  - Put tenant rename and deletion controls in the Settings `Client access` section. Require a destructive confirmation containing the tenant display name, explain why the last tenant cannot be deleted, and after deleting the active tenant select the oldest remaining tenant and replace the URL.
  - Switching tenants must atomically replace dashboard usage, secret status, defaults, request examples, and provider settings. Clear any one-time generated secret and any I025 revealed provider key immediately; if Settings contains unsaved edits, require an explicit discard decision before switching.
  - Use request identity/cancellation so a late account, tenant, usage, reveal, save, create, rename, or delete response cannot overwrite the newly selected tenant. A failed hydration must clear prior tenant data and render the existing explicit workspace error state rather than displaying stale cross-tenant values.
  - Preserve the compact MPR visual language, shared header/footer/modal stacking contracts, visible focus, screen-reader labels/status announcements, and unclipped switcher/dialog layouts at desktop and mobile widths.
  Deliverables:
  - Add the account, tenant, tenant-name/id domain types, tenant-scoped repository interfaces, relational GORM models/indexes, secret lookup, provider encryption binding, usage partitioning, and administrator projection.
  - Add and document the bounded migration plus a disposable-database verification and rollback runbook. Production backup, migration apply, and deployment remain operator-owned and must not be performed by the implementation agent.
  - Replace the singular management routes and frontend client/types/state with the account bootstrap and tenant-scoped APIs; remove obsolete one-to-one code and response types.
  - Add the tenant switcher, create dialog, Settings rename/delete controls, URL selection, race handling, responsive styles, and centralized user-facing copy.
  - Update README and implementation documentation with the account-to-many-tenants model, exact API paths, migration ordering, encryption associated-data change, isolation rules, deletion semantics, and explicit exclusion of shared/team tenancy.
  Validation:
  - Add black-box management HTTP scenarios where one authenticated user creates two tenants with different keys for the same provider, defaults, secrets, and usage; prove each tenant round-trips independently and another authenticated user receives indistinguishable `404` responses for both reads and mutations.
  - Prove through public proxy endpoints that each tenant's generated secret selects only that tenant's provider key/defaults, records usage only for that tenant, and that revoking or deleting one tenant never changes another tenant's authentication or history.
  - Exercise I025 reveal and F013 intervals through the tenant-scoped endpoints, including cross-tenant denial, response non-caching, concurrent requests, and the absence of raw secrets/keys from account, tenant-summary, usage, and admin payloads.
  - Add disposable pre-migration database fixtures for SQLite and PostgreSQL containing multiple current users, encrypted provider keys, generated-secret digests, defaults, and usage. Run the real migration entrypoint and prove exact preservation, ciphertext re-binding, old-schema removal, idempotent version rejection, rollback on corrupted/orphaned rows, and unchanged client-secret routing.
  - Add Playwright coverage for first-user bootstrap, create, switch, URL reload/history, independent tabs, rename, guarded final-tenant deletion, confirmed deletion, unsaved-edit handling, one-time secret/key cleanup, response-order races, explicit invalid-URL errors, admin tenant lists, keyboard use, and desktop/mobile geometry.
  - Extend the real local TAuth black-box path to create and use two tenants for one verified session and prove a second verified user cannot access either tenant.
  - Run the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci` pair for the implementation, with the final run occurring after the last code edit.

## Planning
*do not implement yet*

- [ ] [P001] (P1) {F014} Design a tenant-scoped provider, model, and key-acquisition onboarding flow.
  Goal:
  Let a signed-in managed user complete one clear text-routing setup: select a
  supported provider, select one of that provider's supported text models, and
  either paste an existing provider API key or open that provider's official
  key-acquisition page in a new window before returning to paste it. A completed
  setup must make the chosen provider/model the active tenant's usable text
  route without asking the user to reconcile separate provider, default, and
  client-secret forms.

  Requirements:
  - Build the flow on the canonical F014 active-tenant context. It must read and
    write only the selected tenant; another tenant or user must never inherit a
    provider key, model choice, in-progress form value, or completion state.
  - Serve provider labels, text-model choices, capabilities, and the verified
    official credential-acquisition URL from one validated provider catalog.
    Do not hard-code provider/model lists or provider registration URLs in the
    browser. The public/management catalog build must reject a self-service
    provider without a canonical HTTPS credential URL rather than render a
    guessed link.
  - Make provider selection the first step and expose only that provider's text
    models in the next step. Explain whether the provider already has a saved
    key, but never show the raw key or make a model from another provider
    selectable.
  - When the user has no key, render a descriptive provider-specific anchor
    that opens the official acquisition page with `target="_blank"` and
    `rel="noopener noreferrer"`. Do not send tenant IDs, TAuth data, proxy
    secrets, provider keys, or tracking query values to the external site, and
    do not attempt to detect registration completion.
  - Keep selection local while the external page is open. On return, require a
    manually pasted key and make one atomic authenticated operation that saves
    the encrypted provider key, the selected provider text model, and the
    tenant's text defaults. A failure leaves no partial routing state and shows
    an explicit error; it must not reuse a prior model or key as a fallback.
  - Preserve existing security boundaries: public proxy requests still reject
    upstream provider keys; management responses and generated examples never
    return them; saved keys remain encrypted at rest and masked after save.
  - Keep the generated client-secret step visibly separate but adjacent to
    completion, including one-time secret display and copyable route examples.
    Do not create a second client-authentication or provider-key storage path.

  Deliverables:
  - Add a validated, sanitized provider catalog projection containing the
    provider identity, label, text models, capability metadata, and official
    credential-acquisition URL; use it for the management API and browser UI.
  - Replace the disconnected Settings controls with a tenant-scoped onboarding
    surface and one canonical management mutation for completed provider/model/
    key setup.
  - Update typed frontend contracts, management API documentation, examples,
    and accessibility copy to describe the exact sequence and no-key path.
  - Do not add provider aliases, hidden default selection, a browser-maintained
    catalog, a compatibility endpoint, a key-import shortcut, or a best-effort
    retry/fallback path.

  Validation:
  - Add black-box configuration and management API coverage for invalid/missing
    credential URLs, provider/model mismatches, atomic rollback, tenant/user
    isolation, masked responses, and the absence of provider keys in profile,
    example, and public-proxy payloads.
  - Add Playwright coverage for a first-time user choosing a provider, seeing
    only its models, opening the correctly protected official link in a new
    page, returning to save a key, receiving the selected default route, and
    generating/copying a client secret. Cover keyboard, screen-reader labels,
    narrow layouts, saved-key updates, and explicit failure states.
  - Run the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci`
    pair for the implementation, with the final run after the last code edit.

- [ ] [P002] (P1) Create a canonical public landing page and generated capability catalog.
  Goal:
  Make the Pages root an indexable, useful LLM Proxy landing page that accurately
  explains what the service does, who it is for, how it is used, and every
  currently supported provider/model capability. Move the existing authenticated
  management workspace to the one canonical `/manage/` route so public product
  discovery and key-management workspaces are not competing root pages.

  Requirements:
  - Serve a public, useful `https://llm-proxy.mprlab.com/` landing page without
    requiring a management session. Keep the separate API origin's `GET /`,
    `POST /`, `/v2`, and `/dictate` contracts unchanged; only the Pages
    information architecture changes.
  - Move the current MPR UI/TAuth management shell, its rendered
    `data-config-url`, header navigation, logout destination, browser tests,
    and release renderer to `/manage/`. `/manage/` is a private workspace
    entry, uses `noindex`, and is absent from the public sitemap; do not leave
    a duplicate root workspace, JavaScript/meta-refresh redirect, or legacy
    management route.
  - Generate a sanitized public capability catalog from the same validated
    provider registry used for request validation and management profiles. The
    landing matrix must enumerate every supported text and dictation provider
    and model, defaults, dictation availability, web-search availability, and
    known proxy output limits without exposing provider keys, tenant state,
    configured base URLs, or non-public deployment data.
  - Do not maintain a second hand-written provider/model table. A catalog change
    must update the landing matrix deterministically or fail the site build,
    including missing/duplicate providers or models and capabilities that cannot
    be represented publicly.
  - Describe the full current capability set with evidence-backed language:
    tenant-secret authenticated text and canonical `/v2` messages, native and
    compatible provider routing, dictation, constrained OpenAI web search,
    response formats and normalized usage metadata, request limits/clear error
    behavior, self-service encrypted provider-key management, generated-secret
    rotation, usage visibility, and Go/Python/CLI integration options. State
    model/provider limitations rather than implying universal feature parity.
  - Provide clear crawlable calls to action for `/manage/`, the resource hub,
    and current integration documentation. Use semantic HTML, visible focus,
    accessible tables/filters, concise unique metadata, canonical root URLs,
    and structured data that describes only visible landing-page content.

  Deliverables:
  - Add the canonical catalog projection/build contract and a static public
    landing page with capability sections, provider/model matrix, limitations,
    and conversion paths.
  - Relocate and render the management application at `/manage/`, update all
    root/resource/header/footer links, and document the new public-vs-private
    Pages route contract in README and deployment/site-render guidance.
  - Update the resource hub and shared site shell so public navigation points to
    the landing page while management calls to action point only to `/manage/`.
  - Do not duplicate catalogs in HTML/JavaScript/docs, make availability claims
    based on whether a particular user has a key, expose secrets, or preserve a
    second root management implementation.

  Validation:
  - Add black-box build/render coverage proving the public matrix exactly
    reflects the validated catalog, has no secret-bearing fields, and rejects
    catalog/render drift.
  - Add Playwright coverage for an anonymous public landing, its accessible
    provider/model matrix and CTAs, navigation to `/manage/`, and the full
    existing authenticated management lifecycle at that new route.
  - Verify root canonical, Open Graph, JSON-LD, sitemap, and resource links use
    the final public URL form, while `/manage/` is noindex and excluded from
    sitemap output.
  - Run the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci`
    pair for the implementation, with the final run after the last code edit.

- [ ] [P003] (P1) {P002} Re-audit and expand the SEO/use-case resource system from verified product contracts.
  Goal:
  Refresh LLM Proxy's search and resource strategy from the current repository
  contract so prospective users can discover concrete, supported ways to use
  the service without creating duplicate doorway pages or claiming roadmap work
  as shipped functionality.

  Requirements:
  - Produce a new repo-grounded SEO report before changing public copy. It must
    inventory current capabilities, limits, public routes, existing resource
    pages, claim evidence, unsupported claims, the final landing/`/manage/`
    separation, and every current provider/model capability from P002's
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
    testimonials, compliance, provider performance, or support for F014/F015
    roadmap behavior before it is implemented.
  - Replace the generator's arbitrary page-count quota and fixed modified-date
    snapshot with an evidence-backed content manifest. Compute `lastmod` only
    from maintainable source/build data or omit it; never publish stale dates.
    Keep every model/provider assertion tied to the generated public catalog.
  - Enforce the complete indexing contract: canonical, sitemap, Open Graph,
    JSON-LD, and crawlable internal links use one final trailing-slash URL;
    root and the resource hub link to all public content; `/manage/`, private
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
    representative pages from every use-case family, `/manage/` exclusion, and
    crawlable navigation from landing page to hub to resource page.
  - Require an evaluation result of at least 4/5 for repo grounding, use-case
    specificity, doorway safety, metadata, conversion clarity, duplicate-risk,
    site integration, and indexing readiness, and exactly 5/5 for factual
    integrity before publication.
  - Run the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci`
    pair for the implementation, with the final run after the last code edit.

- [ ] [P004] (P1) {P002,P003} Make Resources an always-available footer surface and enforce the resource-page shell.
  Goal:
  Make the public Resources entry point continuously discoverable from the
  shared footer, and make every public resource page use one unambiguous
  document order: header, resource content, then footer.

  Requirements:
  - Render a semantic `Resources` navigation section in the shared public
    footer on the landing page, the resource hub, and every generated public
    resource page. It must contain a descriptive, crawlable anchor to the
    canonical `/resources/` hub; it must not depend on JavaScript interaction,
    a sitemap, or an authenticated `/manage/` page to discover the resources.
  - Treat the footer as an always-rendered part of the public document shell,
    rather than an optional resource-hub-only fragment. The Resources entry
    must remain available in normal document flow at every supported viewport
    without covering page content or creating a duplicate navigation surface.
  - Give each generated resource document exactly one shared shell in this
    order: the canonical public header, one `main` element containing all
    resource-specific visible content, and the canonical public footer. No
    resource article, related-link group, CTA, or generated navigation may sit
    before the header, after the footer, or outside the page's `main` region.
  - Generate the footer Resources link and the resource-page shell from the
    same deterministic site manifest/template contract as the hub, pages,
    canonical URLs, and sitemap. Do not hand-maintain duplicate footer links,
    retain the current hub-only footer, or create a legacy layout path.
  - Preserve P002's public-root versus private-`/manage/` separation and
    P003's canonical trailing-slash, accessibility, and indexing contracts.
    The footer must never expose tenant data, secrets, private API routes, or
    noindex management URLs as public resource navigation.

  Deliverables:
  - Extend the generated public site shell with the canonical footer Resources
    navigation and apply it consistently to the landing page, resource hub,
    and every generated public resource page.
  - Update the resource generator and any site-rendering documentation so the
    header-main-footer ordering and footer-based resource discovery are explicit
    invariants.
  - Add black-box static-site and Playwright coverage that checks the footer
    Resources link on every public route family, verifies its target is the
    canonical hub URL, and proves generated resource pages place all visible
    resource content between the shared header and footer at desktop and narrow
    widths.

  Validation:
  - Make generation fail when a public resource page omits the canonical
    header, `main`, footer, or footer Resources anchor; when those elements are
    out of order; when resource content escapes `main`; or when the footer link
    is not the canonical public hub URL.
  - Extend the public-site link/canonical audit to prove footer discovery uses
    a normal crawlable anchor and keeps `/manage/`, APIs, secrets, redirects,
    and noindex pages out of resource navigation.
  - Run the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci`
    pair for the implementation, with the final run after the last code edit.

- [ ] [P005] (P1) {P002,P004} Normalize public Privacy and Terms pages using PoodleScanner's legal-page contract as the structural reference.
  Goal:
  Give LLM Proxy one coherent, public legal-page experience: canonical Privacy
  and Terms pages with LLM Proxy-specific, evidence-backed content, a readable
  no-JavaScript fallback, and consistent legal links in the shared footer.

  Requirements:
  - Establish `/privacy/` and `/terms/` as the only canonical public legal
    routes. Use those exact trailing-slash URLs in page metadata, Open Graph,
    sitemap, shared footer links, and all internal links; do not add `/tos`,
    slashless, duplicate, or compatibility routes.
  - Use PoodleScanner's current `web/site/privacy/index.html`,
    `web/site/tos/index.html`, and `test/web/tests/footer-legal-links.spec.js`
    as a structural and test-design reference only. Do not copy PoodleScanner's
    product-specific clauses, dates, contact details, YouTube sections, refund
    policy, branding, or external-link assertions into LLM Proxy.
  - Render each legal page through the canonical public shell established by
    P002 and P004: one shared header, one `main` element containing the legal
    document, and the shared footer. The footer must expose descriptive,
    crawlable `Privacy` and `Terms` links on the landing page, `/manage/`, the
    resource hub, every resource page, and both legal pages themselves.
  - Follow the PoodleScanner pattern of a semantic `mpr-legal-document` for
    `privacy` and `terms`, with a fully readable static fallback inside the
    document when the component cannot render. M020 records the verified
    v3.11.3 component surface and moves the app to the canonical literal
    `@latest` integration contract. Do not add another version pin, a second
    legal renderer, or a compatibility path.
  - Source policy statements only from verified LLM Proxy behavior and an
    approved legal-content input. Privacy content must accurately distinguish
    MPR UI/TAuth session handling from LLM Proxy persistence; describe
    tenant-owned provider keys as encrypted at rest, generated secrets as
    digest-only storage, and usage records as excluding prompts, audio,
    transcripts, responses, raw provider keys, and raw tenant secrets. Terms
    content must state the documented proxy/provider limitations and user
    responsibility for submitted data and upstream-provider use without
    inventing privacy, retention, compliance, deletion, uptime, payment,
    refund, jurisdiction, or legal-rights claims.
  - Privacy content must disclose the current Google Analytics and LoopAware
    public-page telemetry. State only verified implementation facts; do not
    claim collection, retention, consent, or opt-out behavior beyond approved
    legal or provider documentation.
  - Give each page a specific title, description, canonical URL, Open Graph
    values, visible H1, effective date, and last-updated date derived from one
    maintained source. Keep the legal pages indexable only if the final legal
    policy authorizes that public status; they must otherwise be handled by an
    explicit site-indexing decision, never silently omitted or disguised.
  - Keep the legal-page source, footer links, sitemap inclusion decision, and
    rendered fallback synchronized from one canonical site contract. Do not
    hand-maintain divergent footer fragments, duplicate legal copy, or a
    legacy management-only footer path.

  Deliverables:
  - Add a deterministic, canonical legal-page source/template and render the
    public `/privacy/` and `/terms/` pages from it with the current MPR UI
    legal-document component and accessible static fallback content.
  - Extend the shared footer contract with the canonical Privacy and Terms
    anchors across public and management surfaces, alongside P004's Resources
    navigation.
  - Update public-site and deployment documentation with the final legal URLs,
    policy-content ownership, effective/modified-date source, and indexability
    decision.

  Validation:
  - Add black-box static-site and Playwright coverage that requests both legal
    routes, verifies their public metadata, visible headings, canonical URLs,
    footer links, and keyboard-accessible navigation, and proves every required
    site route exposes the same canonical Privacy and Terms anchors.
  - Verify that static fallback legal content remains readable when the custom
    element is unavailable, and that an initialized element does not duplicate
    visible legal copy, obscure the footer, or put legal content outside
    `main` at desktop and narrow widths.
  - Make rendering fail on missing approved legal content, invalid/missing
    effective or modified dates, route/metadata/footer disagreement, duplicate
    legal pages, unsupported MPR UI legal-component use, or policy claims that
    are not tied to an approved source.
  - Run the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci`
    pair for the implementation, with the final run after the last code edit.
