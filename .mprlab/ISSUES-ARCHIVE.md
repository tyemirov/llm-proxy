# Resolved Issue Archive

This is historical context, not an active backlog. `.mprlab/ISSUES.md`
contains only current active, blocked, planning, and recurring work.

Archive passes:

- 2026-07-24: Indexed 119 resolved issues from the `v0.2.43` release snapshot.
  Read their complete original entries with
  `git show v0.2.43:.mprlab/ISSUES.md`.
- 2026-08-10: Moved 84 resolved or retired entries from the active tracker
  after the durable-documentation and open-issue audits.
- 2026-08-10: Archived P007 after approval of the Alibaba backend provider
  decision.
- 2026-08-10: Archived I222 after validation of the CalVer release decision.
- 2026-08-10: Archived I039 after retiring Qwen Cloud Token Plan and migrating
  managed data to schema version 4.
- 2026-08-10: Archived I221 after normalizing model identity and provider
  offerings.
- 2026-08-10: Archived I216 after publishing the authoritative model-operation
  capability and pricing catalog.
- 2026-08-30: Moved 45 resolved non-recurring entries from the active
  tracker: 34 BugFixes, seven Improvements, one Maintenance, and three
  Features.
- 2026-09-08: Moved 71 resolved non-recurring entries from the active
  tracker: 44 BugFixes, 18 Improvements, three Maintenance, and six
  Features. Kept all Planning entries in the active tracker.

`CHANGELOG.md` remains the release-level history. This index keeps completed
issue titles discoverable without making the active tracker noisy.

## Archive conventions

- Resolved non-recurring issues remain under their original tracker section.
- The 2026-07-24 entries are a title index for the retained release snapshot.
- Dated complete-entry sections preserve their complete bodies and resolution records.
- `M009` was historically entered as `M009R`, but it was a completed one-off
  consolidation with no recurring cadence and is archived under its canonical
  non-recurring identifier.

## BugFixes

- [x] [B068] Let text callers select a capability-validated reasoning effort.
- [x] [B001] Make management request examples copyable and provider-specific.
- [x] [B002] Present provider settings through a selected-provider editor.
- [x] [B003] Store text model and system prompt with each managed provider.
- [x] [B004] Populate management request examples before secret generation.
- [x] [B005] Move Pages deployment out of GitHub Actions and keep browser config backend-owned.
- [x] [B006] Make management admin configuration plural and deployable.
- [x] [B007] Make llm-proxy-client invalid-input tests immune to ambient client env.
- [x] [B008] Published production image accepts the current management config.
- [x] [B063] Activate the released v0.2.39 Pages artifact.
- [x] [B009] Validate management migration seed tenant defaults at startup.
- [x] [B010] Require expiration on management session JWTs.
- [x] [B011] Remove unsupported no-dictation default option from management UI.
- [x] [B012] GitHub Pages frontend remains unavailable until the workflow fix reaches master.
- [x] [B013] Fix F007 review issues in usage loading and usage queries.
- [x] [B014] Fix F007 usage dashboard follow-up review findings.
- [x] [B015] Gemini POST responses can return thought or partial text as successful output.
- [x] [B016] Long semantic-review POSTs fail transport while small requests pass.
- [x] [B017] OpenAI background semantic-review calls require manual timeout tuning.
- [x] [B018] Polled OpenAI terminal responses skip continuation and synthesis handling.
- [x] [B019] PR merge CI drops limiter coverage below 100%.
- [x] [B020] Adjust settings modal layout relative to header and footer.
- [x] [B021] Resolve Meta, upstream-rate, and release review regressions.
- [x] [B022] Validate the effective Pages push repository before deployment.
- [x] [B023] Preserve Pages release markers under branch publishing.
- [x] [B024] Prevent shell help deadlocks under constrained pipe limits.
- [x] [B025] Restore release pipeline tests after prepare release exits 2.
- [x] [B026] Retry management profile after MPR UI refreshes authentication.
- [x] [B027] Make TAuth session validation and deployment one canonical contract.
- [x] [B028] Present a direct LLM Proxy sign-in experience.
- [x] [B029] Exercise the real local TAuth and LLM Proxy session boundary in browser tests.
- [x] [B030] Keep the authenticated session until explicit sign-out.
- [x] [B031] Drive real-stack sign-in through the browser lifecycle.
- [x] [B032] Preserve sign-in button contrast on hover.
- [x] [B033] Let the verified legacy-token owner reach the dashboard after earlier sign-in created an empty account.
- [x] [B034] Hydrate the dashboard only from the canonical MPR UI authentication lifecycle.
- [x] [B035] Move workspace notifications above the footer.
- [x] [B036] Keep routing default provider/model pairs valid.
- [x] [B037] Declare the app-owned orchestration manifest completely.
- [x] [B038] Keep the DashScope catalog valid for the default endpoint.
- [x] [B039] Remove user query content from proxy request logs.
- [x] [B040] Keep invalid web-search query values out of structured logs.
- [x] [B041] {B020,B035} Render management notifications in the header immediately left of the avatar.
- [x] [B042] {B041,I014,I015} Place the LLM Proxy logo directly left of its shared-header title.
- [x] [B043] {B001} Replace the generated-secret bracket glyph with a standard copy icon.
- [x] [B044] Make GHCR and GitHub Pages publication verification wait on authoritative readiness.
- [x] [B045] {I026,F012} Make routing reasoning effort provider/model-specific and co-locate it with the text route.
- [x] [B046] {B041} Restore management notifications immediately left of the avatar or Sign in control.
- [x] [B047] {B041,B046} Auto-dismiss management notifications after the configured 10 seconds.
- [x] [B048] {M005R} Make the Go coverage client probe independent of stdin EOF.
- [x] [B049] {M005R,B048} Isolate the disposable live-provider harness from unrelated local listeners.
- [x] [B050] Compact selected-provider settings and inline key visibility.
- [x] [B051] Present Client access as a compact tenant/key row.
- [x] [B052] Add the standard `make up` local service command.
- [x] [B053] Make `make up` run the complete local browser orchestration.
- [x] [B054] Compact and align the Settings controls.
- [x] [B055] Simplify the Settings key surface.
- [x] [B056] Scope provider-key editor state to the selected provider.
- [x] [B057] Require complete client and provider key setup after authentication.
- [x] [B058] Autosave selected-provider settings and clarify retained client-key state.
- [x] [B059] Expose GPT-5 mini reasoning effort from the current OpenAI contract.
- [x] [B060] Autosave routing defaults and remove their manual save action.
- [x] [B061] Serialize Settings mutations and isolate local orchestration secrets.
- [x] [B062] Make signed-out notice expiry coverage deterministic.
- [x] [B064] Make live-harness proxy ownership coverage deterministic.
- [x] [B065] {F013} Improve selected usage-interval contrast.
- [x] [B066] {F013} Clarify the client-key replacement action.
- [x] [B067] Make local environment projection readiness deterministic.


### Complete entries archived 2026-08-10

- [x] [B129] (P2) Correct I045 cancellation and managed response-flush telemetry.
  Goal:
  Make I045 progress events and phase totals agree with the request outcome.
  Evidence:
  - OpenAI progress mapped `context.Canceled` and `context.DeadlineExceeded` to
    `completion_signal=failure`. The terminal summary mapped these errors to a
    canceled or timed-out request outcome.
  - Managed usage flushed the response before the response-formatting timer and
    the managed-usage timer. A slow flush appeared only in total request time.
  Requirements:
  - Map OpenAI context cancellation and deadline errors to
    `completion_signal=canceled` before the generic failure case.
  - Measure a managed response flush as response formatting. Start the
    managed-usage enqueue phase after the flush finishes.
  - Preserve all request budgets, provider lifecycles, public payloads, and
    managed usage data.
  Validation:
  - Drive the public handler through canceled OpenAI create and poll requests.
    Prove each progress event uses `completion_signal=canceled`.
  - Use a managed public request with a delayed response flush. Prove the delay
    enters `response_formatting_ms` and not `managed_usage_enqueue_ms`.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - OpenAI transport cancellation and deadline errors now emit the canceled
    completion signal for create and poll progress events, consistent with the
    continuation event and terminal request outcome.
  - Managed response flushing now contributes to response formatting before
    managed usage enqueue timing begins. Public-handler coverage delays the
    flush and verifies both phase totals.
  - The required pre-change and final `make ci` runs passed all 11 gates with
    100.0% Go statement coverage. The final run completed in 118 seconds.

- [x] [B097] (P1) Return typed sanitized failures from the official Go client.
  Goal:
  Let server-side callers classify authentication, rate-limit, proxy
  work-budget, and upstream-availability failures without parsing formatted
  error strings or retaining raw response bodies.
  Evidence:
  - `pkg/llmproxyclient` returns only `ErrClientHTTPFailure` plus a formatted
    `status=<n> body=<raw>` string for every non-2xx response.
  - The public proxy publishes four stable JSON error codes for invalid request
    timeouts, provider failures, provider rate limits, and proxy work-budget
    expiry.
  Requirements:
  - Return one typed official-client failure for every completed non-2xx HTTP
    response, with read-only HTTP status and recognized stable proxy error-code
    accessors.
  - Preserve `errors.Is(error, ErrClientHTTPFailure)` while supporting
    `errors.As` for the typed failure.
  - Exclude raw response bodies from the returned error.
  - Keep transport and read failures distinct from completed HTTP responses.
  Validation:
  - Exercise the public Go client through a fake HTTP server for structured and
    unstructured non-2xx responses.
  - Prove typed status and code values, sentinel identity, sanitized error text,
    and rejection of unknown or malformed error codes.
  - Run final `timeout -k 350s -s SIGKILL 350s make ci`.
  Resolution:
  - Completed non-2xx responses now return a sanitized `HTTPFailure` with
    read-only status and recognized proxy error-code accessors while preserving
    `ErrClientHTTPFailure` identity. Raw bodies remain outside the returned
    error, and transport and response-read failures retain their distinct path.
  - Fake-server coverage passes for all four published codes and for
    unstructured, malformed, and unknown bodies. The focused Go gate passed at
    100.0% statement coverage, and final `make ci` passed all 11 gates in 102
    seconds with 100.0% Go statement coverage.

- [x] [B125] (P1) {I219} Isolate and stop public-capability test servers.
  Goal:
  Keep frontend and Pages artifact validation deterministic and free of
  background capability-server processes.
  Evidence:
  - The Playwright renderer setup starts the capability server through
    `go run`, then stops only the Go driver while its compiled CLI child keeps
    listening after the temporary test directory is removed.
  - The Pages artifact gate always starts and probes port 8080, so a running
    local stack can satisfy readiness or prevent the test-owned server from
    binding.
  Requirements:
  - Build a temporary CLI binary and run that binary directly in both test
    launchers so the recorded process is the actual server process.
  - Give the Pages artifact gate a temporary capability configuration with an
    ephemeral loopback port instead of the local runtime port.
  - Stop and wait for every test-owned server before removing its temporary
    files, and prove its HTTP listener is closed.
  - Keep validation on the real `--public-capabilities-only` CLI and
    `/api/public/capabilities` HTTP boundaries.
  Validation:
  - Focused Playwright coverage and the Pages artifact target pass without
    leaving a capability-server process or listener.
  - Run final `timeout -k 350s -s SIGKILL 350s make ci` after the last code
    edit; reuse the current exact-code passing result as the baseline.
  Resolution:
  - One frontend-owned helper now creates private temporary capability
    configurations on ephemeral loopback ports for both launchers. Each
    launcher builds and starts the actual temporary CLI binary, stops that
    exact process, waits for exit, and rejects a listener that remains open.
  - The focused Pages artifact target passed while an unrelated HTTP service
    held port 8080. The complete 89-scenario Playwright target passed with no
    matching capability-server process before or after the run. Four orphan
    servers left by the prior harness were stopped.
  - Final `make ci` passed all 11 gates in 120 seconds with 89 frontend browser
    scenarios and 100.0% Go statement coverage.

- [x] [B124] (P1) Anchor the Settings close control in the title row.
  Goal:
  Keep the Settings close control at the right edge of the title row and
  vertically centered with the title at every supported width.
  Evidence:
  - The title, conditional notification region, and close control rely on
    automatic three-column grid placement.
  - When the notification region is hidden, it leaves grid layout and the close
    control moves into the middle column beside `Settings` instead of remaining
    at the right edge.
  Requirements:
  - Give the title, notification region, and close control explicit positions
    in the Settings header grid.
  - Align the close control to the header's right content edge and center it on
    the title's vertical center whether the notification is hidden or visible.
  - Preserve notification containment, close behavior, pending-state locking,
    focus behavior, and narrow-screen layout.
  Validation:
  - Browser coverage proves right-edge and title-center geometry with the
    notification hidden and visible at desktop, compact, and mobile widths.
  - Run the required final
    `timeout -k 350s -s SIGKILL 350s make ci` after the last code edit; reuse the
    current exact-code passing result as the baseline.
  Resolution:
  - Named header grid areas now keep the title, conditional notification, and
    close control in fixed columns. The close control is aligned to the right
    content edge and centered on the title regardless of notification state.
  - The browser geometry regression failed against the prior hidden-notification
    layout with an 836.6875-pixel right-edge difference, then passed for hidden
    and visible notifications at 1280x720, 480x780, and 390x780 viewports.
  - Desktop, compact, and mobile screenshots confirm the close control remains
    in the title row's upper-right corner without overlap or overflow.
  - Final `make ci` passed all 11 gates in 121 seconds with 89 frontend browser
    scenarios and 100.0% Go statement coverage.

- [x] [B123] (P1) {B121} Initialize routing from a pending provider default.
  Goal:
  Preserve the newly selected provider default model when the same provider is
  chosen for routing before its provider-settings autosave returns.
  Evidence:
  - Provider model edits update only the provider editor while their serialized
    profile mutation is pending.
  - Routing-provider selection reads `providers[].text_model` from the last
    applied profile, so it can queue the previous model behind the provider save
    and persist that stale model as an explicit routing override.
  Requirements:
  - Wait for a matching pending provider autosave before initializing the
    routing provider/model pair from the returned current profile.
  - Keep unrelated provider and routing mutations independently editable and
    preserve the existing serialized whole-profile mutation contract.
  - Reject a delayed initialization after the tenant, authentication, Settings,
    or routing-selection context changes.
  Validation:
  - Browser coverage holds a provider-model save open, selects that provider as
    the routing default, and proves the queued defaults mutation uses the saved
    new model.
  - Run the required final
    `timeout -k 350s -s SIGKILL 350s make ci`.
  Resolution:
  - Matching routing-provider selection now waits for the provider autosave,
    rereads the current profile model, and rejects stale tenant,
    authentication, Settings, and routing-selection completions.
  - The new browser regression failed against the previous behavior with
    `gpt-4.1` instead of `gpt-5-mini`, then passed after the fix.
  - Final `make ci` passed all 11 gates in 117 seconds with 100.0% Go statement
    coverage and 87 frontend browser scenarios.

- [x] [B122] (P1) Show Settings activity notifications in the Settings title.
  Goal:
  Keep feedback from Settings actions visible while the modal obscures and
  de-emphasizes the application header.
  Evidence:
  - The application has one notification region in the MPR header.
  - The Settings overlay sits above that header, so Settings save and failure
    notices are not visibly associated with the active window.
  Requirements:
  - Render notifications caused by Settings activities in the Settings title
    row while Settings is open.
  - Keep notifications caused by page activities in the MPR header.
  - Preserve the existing live-region semantics and automatic dismissal.
  - Keep both placements usable at supported desktop and narrow widths.
  Validation:
  - Browser coverage proves a Settings success and failure appear in the
    Settings title rather than the MPR header.
  - Browser coverage proves page activity continues to use the MPR header.
  - Run the required final
    `timeout -k 350s -s SIGKILL 350s make ci`.
  Resolution:
  - Added an explicit page-versus-Settings notification surface contract and a
    live notification region inside the Settings title row.
  - Settings success and failure feedback now replaces the obscured header
    notice while page activity continues to use the MPR header.
  - Browser coverage verifies placement, live-region semantics, automatic
    dismissal, and desktop and narrow-screen containment.
  - `timeout -k 350s -s SIGKILL 350s make ci` passed all 11 gates with 100.0%
    Go statement coverage and 86 passing frontend browser tests.

- [x] [B121] (P1) Clarify and synchronize provider and routing defaults.
  Goal:
  Make a saved provider default-model change immediately update the tenant's
  active text routing model when that tenant currently routes through the same
  provider, while keeping the two default scopes explicit and independently
  editable.
  Evidence:
  - Settings can show OpenAI provider default `gpt-5.6-terra` while Routing
    defaults still shows OpenAI model `gpt-4.1`.
  - The provider-settings transaction uses eligibility-only reconciliation,
    so it preserves any model while the active provider still has a saved key.
  Requirements:
  - Add accessible help tooltips that explain when provider defaults and
    routing defaults apply.
  - Update the active same-provider routing model in the provider-settings
    database transaction and return the synchronized profile.
  - Preserve a routing default owned by another provider.
  - Preserve a compatible reasoning effort and clear an incompatible effort
    when the active route's model changes.
  - Keep explicit routing-default edits as the canonical override operation;
    do not add a second request, compatibility path, or read-time repair.
  Validation:
  - Black-box management API coverage proves same-provider synchronization,
    other-provider preservation, and incompatible-effort clearing.
  - Browser coverage proves the returned provider-save profile updates the
    visible Routing defaults model, both help tooltips expose their canonical
    explanations, and a user can then override the routing model independently.
  - Run the required final
    `timeout -k 350s -s SIGKILL 350s make ci`.
  Resolution:
  - Added keyboard- and hover-accessible help tooltips for both default scopes,
    including narrow-screen containment coverage.
  - A changed provider default model now updates the active same-provider route
    atomically, preserves compatible reasoning effort, clears incompatible
    effort, and leaves inactive-provider routes and later route overrides
    unchanged.
  - `timeout -k 350s -s SIGKILL 350s make ci` passed all 11 gates with 100.0%
    Go statement coverage and 86 passing frontend browser tests.

- [x] [B120] (P0) {B114} Keep production browser authentication on the public TAuth origin.
  Goal:
  Make the hosted MPR UI login, nonce, session restore, refresh, and logout
  requests use the browser-reachable HTTPS TAuth API.
  Evidence:
  - Production `https://llm-proxy-api.mprlab.com/config-ui.yaml` currently
    exposes `tauthUrl: "http://tauth-api:8080"`.
  - The browser blocks `/auth/nonce` as mixed content, and `tauth-api` is a
    Docker-only service alias that public clients cannot resolve.
  - Gateway capability component `url` intentionally renders the provider's
    runtime endpoint as `scheme://host:port`; `tauth.http` is therefore the
    correct container-to-container capability and the wrong hosted browser
    profile value.
  Requirements:
  - Bind `LLM_PROXY_MANAGEMENT_TAUTH_URL` to the canonical public TAuth origin
    `https://tauth-api.mprlab.com` in the selected production manifest.
  - Keep MPR UI and TAuth as the sole browser authentication owners; do not add
    an application-side auth request, proxy fallback, alias, or compatibility
    path.
  - Preserve the internal `tauth.http` capability for runtime consumers that
    actually need container-to-container traffic; do not change gateway
    capability semantics.
  Validation:
  - Lifecycle black-box coverage rejects any non-public binding for the
    browser-facing TAuth URL.
  - Run focused lifecycle validation and the required final
    `timeout -k 350s -s SIGKILL 350s make ci`.
  - Recheck the hosted config and public TAuth TLS boundary without claiming
    the source correction is deployed before the operator lifecycle completes.
  Resolution:
  - The production manifest now injects `https://tauth-api.mprlab.com` as the
    browser-facing TAuth origin while leaving the internal `tauth.http`
    capability unchanged for container consumers.
  - The lifecycle regression failed first on the internal capability binding
    and now locks the hosted profile to the canonical public HTTPS origin.
  - Focused Go validation passes with exact 100% statement coverage. The live
    v0.2.59 profile remains unchanged until the operator-owned release,
    publication, and deployment lifecycle activates this source correction.

- [x] [B119] (P1) {B118,I215} Keep selected models in one readable column.
  Goal:
  Make every selected provider's exact model versions read as one ordered
  vertical branch on the routing diagram.
  Requirements:
  - Render the selected provider's model leaves in one column at desktop,
    tablet, and mobile widths.
  - Preserve the compact desktop ingress, narrow provider leaves, generated
    catalog order, selected connector endpoints, and responsive containment.
  Validation:
  - Browser black-box coverage proves the OpenAI model group has exactly one
    rendered grid column at the desktop acceptance width.
  - Rebuild and visually inspect a provider with many models, then run the
    applicable validation from `.mprlab/POLICY.md`.
  Resolution:
  - Model groups now render as one 280-pixel-wide column at every supported
    width, preserving the compact provider stage and exact generated model
    order.
  - Increased the desktop diagram's reserved height by 40 pixels so OpenAI's
    11-row list fits without changing provider positions when a shorter model
    list is selected.
  - The browser regression failed first with two rendered model columns. It
    then exposed the 19.8-pixel provider-branch shift caused by the taller
    OpenAI list; after reserving the complete list height, all 86 focused
    browser scenarios passed with stable provider and connector endpoints.
  - Headed browser inspection verified the generated OpenAI route with all 11
    model versions in one narrow column. The final run passed all 11 CI gates
    in 122 seconds with the real TAuth black-box scenario, live-provider
    preflight, and exact 100% Go statement coverage.

- [x] [B118] (P1) {B117,I215} Compact and center the desktop routing fork.
  Goal:
  Make the full-width desktop routing map read as one compact left-to-right
  Product to Proxy to Providers to Models flow.
  Requirements:
  - Shorten the visible Product-to-Proxy connection to at most 24 pixels at the
    desktop acceptance width.
  - Reduce provider leaves from 300 pixels to at most 220 pixels so the desktop
    map reserves more width for exact model leaves.
  - Vertically center the Product and Proxy nodes against the complete provider
    branch list while keeping the selected provider in its catalog position.
  - Preserve the generated catalog, selected connector endpoints, tablet and
    mobile stacking, keyboard interaction, and responsive containment.
  Validation:
  - Browser black-box coverage proves the four desktop stages are ordered left
    to right, the Product-to-Proxy gap and provider width stay within their
    limits, and the Product and Proxy centers align with the provider-list
    center within one CSS pixel.
  - Rebuild and visually inspect the desktop routing map, then run the
    applicable validation from `.mprlab/POLICY.md`.
  Resolution:
  - Replaced the desktop two-row map with a compact left-to-right ingress,
    provider, and model grid. The Product-to-Proxy gap is 20 pixels, provider
    leaves are at most 220 pixels wide, and both ingress nodes align with the
    full provider-list center within one CSS pixel.
  - The connector canvas now selects horizontal proxy-to-provider Bézier curves
    for the desktop stage order while retaining the stacked tablet curve path;
    provider selection remains in catalog order and the highlighted
    provider-to-model curve still touches both selected leaves.
  - The browser regression failed first with 300-pixel provider leaves, a
    208.8-pixel Product-to-Proxy gap, and 322.4-pixel center offsets. All 86
    focused browser scenarios then passed after the layout change.
  - Headed browser inspection selected DeepSeek and verified the compact
    Product-to-Proxy ingress plus both highlighted Bézier stages. The
    satisfactory 129-second CI result was reused as the unchanged-input
    baseline; the final run passed all 11 gates in 117 seconds with 86 browser
    tests, the real TAuth black-box scenario, live-provider preflight, and exact
    100% Go statement coverage.

- [x] [B117] (P1) {I215} Keep the selected provider in place and draw its model fan.
  Goal:
  Preserve the approved routing fork so selecting any provider visibly draws
  that provider's outgoing Bézier fan to its supported model leaves.
  Evidence:
  - The selected provider receives `order: -1`, so every selection moves to the
    top of the provider list instead of keeping the generated catalog order.
  - Existing browser coverage proves only that the canvas is nonempty; it does
    not prove the selected provider remains in place or that an accent curve
    leaves its model-facing edge.
  Requirements:
  - Keep provider leaves in their generated catalog order across pointer and
    keyboard selection.
  - Draw the selected provider-to-model fan from the selected provider's
    model-facing edge, matching the approved working visualization.
  - Preserve dynamic catalog generation, exact model selection, responsive
    behavior, and compact provider and model leaves.
  Validation:
  - Add browser black-box coverage that first fails with the current selected
    provider reordering, then proves multiple provider selections retain their
    positions and expose accent pixels at the selected provider and model
    connector endpoints.
  - Run the applicable validation from `.mprlab/POLICY.md`.
  Resolution:
  - Removed the selected-provider order override, so every generated provider
    remains in its catalog position while selection changes only its visual
    state and active model group.
  - The browser regression failed first by proving Moonshot swapped positions
    with OpenAI. It now proves keyboard and pointer selections keep all provider
    positions fixed and samples the rendered canvas for accent pixels at both
    the selected provider's outgoing edge and the selected model's incoming
    edge.
  - Headed browser inspection selected Zhipu in its original final position and
    verified the highlighted proxy-to-provider curve plus the outgoing
    provider-to-`glm-5.1` Bézier fan.
  - The satisfactory 119-second CI result was reused as the unchanged-input
    baseline. The final run after the last source and test edits passed all 11
    gates in 129 seconds with 86 browser tests, 36 Python tests, the real TAuth
    black-box scenario, live-provider preflight, and exact 100% Go statement
    coverage.

- [x] [B116] (P1) {I214,B114} Keep the real mobile sticky footer compact.
  Goal:
  Preserve the canonical sticky footer without letting its hydrated MPR UI
  surface consume excess mobile viewport height.
  Evidence:
  - At 390 by 780 pixels, the real `mpr-ui@latest` footer is fixed and reserves
    its in-flow footprint, but its controls wrap into a 78.6-pixel surface while
    the canonical compact-footer contract allows at most 56 pixels.
  - The application Playwright fixture renders a 48-pixel hand-built footer, so
    its compact-height assertion does not exercise the deployed component path.
  Requirements:
  - Retain the supported MPR footer attributes, all canonical links, the exact
    `Built by Marco Polo Research Lab` project-catalog label, theme control,
    semantic no-JavaScript fallback, and sticky spacer behavior.
  - Keep the hydrated footer at or below 56 pixels without horizontal overflow
    at 390 pixels wide, and preserve desktop geometry.
  - Verify compact geometry through the real MPR UI black-box browser path; do
    not claim deployed footer correctness through the application-owned mock.
  Validation:
  - Rebuild and inspect representative routes through `http://localhost:4179/`
    at desktop and 390-pixel widths.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - The supported public wrapper now keeps every footer control on one compact
    row while retaining the full project label and existing MPR UI inputs.
  - The real TAuth/MPR UI browser test waits for hydration, then asserts fixed
    positioning, all viewport anchors, the shared 56-pixel limit, and no footer
    overflow at 390 pixels wide.
  - The canonical landing and docs routes at `http://localhost:4179/` rendered
    54.2-pixel footers at 390 by 780 pixels; every visible control remained
    inside the viewport, and the project drop-up opened with its full catalog.
  - The required baseline passed all 11 gates in 110 seconds. The final run
    passed all 11 gates in 113 seconds with 85 browser tests, 36 Python tests,
    the real TAuth black-box scenario, live-provider preflight, and exact 100%
    Go statement coverage.

- [x] [B115] (P1) {F019,I213} Align public capability-catalog table rows.
  Goal:
  Keep every provider, model, and capabilities cell on the same visual row
  boundary in the generated public catalog.
  Evidence:
  - At the canonical local landing page, catalog rows are about 67 pixels tall
    while the Model cell computes as a 42-pixel `display: flex` box, so its
    bottom border ends above the Provider and Capabilities cell borders.
  Requirements:
  - Preserve the semantic table and server-rendered no-JavaScript catalog.
  - Keep each `td` in the table formatting context and move model identifier
    and default-badge wrapping and spacing into an inner content wrapper.
  - Preserve the responsive model-to-default-badge gap at desktop and mobile
    widths.
  Validation:
  - Browser black-box coverage proves that all three cells retain table-cell
    display and share their row's top and bottom boundaries at desktop and
    mobile widths.
  - Rebuild and inspect the catalog through `http://localhost:4179/`.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - The generated Model cell remains a semantic table cell, while the new
    inner `catalog-model__content` wrapper owns badge wrapping and spacing.
  - Browser coverage now checks every catalog cell at desktop and mobile
    widths for table-cell display and exact row-boundary alignment while
    retaining the eight-pixel default-badge gap.
  - The rebuilt canonical local page at `http://localhost:4179/` rendered all
    58 rows and 174 cells with zero top or bottom boundary deviation at 1210
    and 390 pixels wide; the desktop visual inspection showed continuous rules.
  - The required baseline passed all 11 gates in 112 seconds. The final run
    passed all 11 gates in 101 seconds with 85 browser tests, 36 Python tests,
    the TAuth black-box scenario, live-provider preflight, and exact 100% Go
    statement coverage.

- [x] [B114] (P1) {B111,B112,B113} Restore the single MPR UI authentication path and correct the shared public contract.
  Goal:
  Keep browser authentication fully owned by MPR UI while preserving the
  authenticated landing redirect, shared responsive footer, and accurate
  privacy disclosure.
  Evidence:
  - The generated shell loads `tauth.js` directly before `mpr-ui-config.js`,
    creating an application-owned authentication bootstrap outside the
    canonical `/config-ui.yaml` contract.
  - The real browser test invokes the exposed TAuth credential-exchange global
    instead of exercising the shared MPR UI login control.
  - Narrow-screen footer CSS targets private `.mpr-footer__*` markup and
    `data-mpr-footer` internals that are not part of the MPR UI DSL.
  - The Privacy page says LLM Proxy cannot read HttpOnly authentication
    cookies, although the backend receives and validates the configured session
    cookie through TAuth's published `sessionvalidator`.
  Requirements:
  - Remove the direct TAuth browser script and every generated/static assertion
    that requires or invokes its global API. Keep `/config-ui.yaml`,
    `mpr-ui-config.js`, declarative MPR UI markup, and documented auth lifecycle
    events as the only browser authentication path.
  - Exercise interactive authentication through the visible MPR UI login
    surface and retain real session restoration, refresh-cookie recovery,
    authenticated landing replacement, and explicit logout coverage.
  - Remove host styling and test selectors that depend on private MPR UI footer
    markup. Configure the shared footer only through supported component
    attributes and verify observable accessibility and geometry.
  - State accurately that browser JavaScript cannot read HttpOnly cookies while
    the backend receives and validates the session cookie only to authorize
    protected LLM Proxy resources.
  Validation:
  - Static generation rejects direct `tauth.js` loading and private MPR UI
    footer selectors across every generated page.
  - Browser black-box coverage authenticates through the visible shared control
    and proves the existing login, restore, refresh, route, and logout outcomes.
  - Regenerated legal pages contain the corrected cookie boundary.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - All public and application pages now load only the canonical MPR UI
    configuration and runtime. Shared-shell generation and Go/browser static
    coverage reject any application-authored `tauth.js` bootstrap.
  - The real browser black box activates the visible MPR UI login control,
    receives real TAuth HttpOnly cookies through the seeded provider adapter,
    opens `/app/`, restores and refreshes the session across navigation, and
    signs out through the visible shared control without manual auth events or
    application-owned TAuth calls.
  - The footer uses MPR UI's supported `wrapper-class` attribute. Host CSS and
    geometry assertions no longer depend on private MPR UI markup or data
    attributes.
  - The generated Privacy page now distinguishes the browser JavaScript cookie
    boundary from backend authorization through TAuth's published validator,
    and integration documentation describes MPR UI as the sole browser-auth
    owner.
  - The required baseline passed before implementation. The final CI run passed
    all 11 gates in 99 seconds with 85 browser tests, 36 Python tests, the real
    TAuth management black box, live-provider preflight, and exact 100% Go
    statement coverage.

- [x] [B113] (P1) {B111,B112,F019} Prevent authenticated sessions from rendering the anonymous landing page.
  Goal:
  Make `/` an anonymous-only route and `/app/` the only authenticated
  application route, including when MPR UI restores an existing TAuth session.
  Evidence:
  - MPR UI restores the session and renders the authenticated user profile on
    `/`, but `sign-in-redirect-url` intentionally runs only after an interactive
    sign-in, so the public landing remains visible.
  - Existing browser coverage proves interactive login and authenticated reload
    only after the browser is already on `/app/`; it never opens `/` with an
    authenticated or refresh-cookie-backed session.
  Requirements:
  - Register the landing route policy before the shared MPR UI bootstrap can
    emit its authenticated lifecycle and replace `/` with `/app/` for every
    documented `mpr-ui:auth:authenticated` event.
  - Keep login, session restoration, refresh, cookies, and logout fully owned by
    MPR UI and its internal TAuth integration. Do not add TAuth requests, cookie or
    storage inspection, protected-API probes, or an application authentication
    state machine.
  - Keep interactive login from documentation, resource, privacy, and terms
    pages on MPR UI's documented `sign-in-redirect-url` contract; authenticated
    users may continue to read those public routes.
  - Use history replacement for the authenticated landing transition so browser
    Back and the authenticated brand link cannot reintroduce `/`.
  Validation:
  - Browser integration coverage proves anonymous `/` remains public,
    interactive login replaces it with `/app/`, and a restored authenticated
    visit to `/` cannot render or remain on the landing page.
  - The real MPR UI/TAuth black box proves both access-cookie restoration and
    refresh-cookie recovery from `/` replace the route with `/app/`, while
    explicit sign out remains on anonymous `/`.
  - Static coverage proves only the landing route owns the authenticated route
    guard and all other public routes preserve the canonical interactive login.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - The generated landing header now registers one route-specific guard before
    MPR UI bootstrap. It consumes the documented authenticated lifecycle and
    replaces `/` with `/app/`; it performs no TAuth request, cookie inspection,
    storage inspection, or session management.
  - Documentation, resource, privacy, and terms routes retain MPR UI's
    canonical `sign-in-redirect-url` for interactive login, while the landing
    route has one authenticated redirect owner.
  - Browser coverage proves anonymous landing access, history-replacing
    interactive login, restored authenticated landing replacement, anonymous
    application rejection, and shared-shell route ownership. The real
    MPR UI/TAuth black box proves access-cookie restoration, refresh-cookie
    recovery from `/`, and explicit logout back to the anonymous landing.
  - Backend authorization remains on TAuth's published Go `sessionvalidator`;
    no TAuth communication or validator was added to the LLM Proxy contract.
  - The required baseline and final CI runs passed all 11 gates with 85 browser
    tests, 36 Python tests, the real TAuth management black box, live-provider
    preflight, and exact 100% Go statement coverage.

- [x] [B112] (P1) {B111,F019} Restore the authenticated dashboard through the canonical MPR UI and TAuth session.
  Goal:
  Make successful public authentication open `/app/` and keep the user
  authenticated across ordinary page refreshes while all browser-side TAuth
  communication remains delegated to MPR UI.
  Evidence:
  - A real Google credential exchange returns `200` and the shared header shows
    the authenticated profile, but the page remains on `/`.
  - Browser `GET /auth/session` requests return `403`; the same endpoint returns
    `204` only when a synthetic `Origin` header is added.
  - Browser same-origin `GET` requests do not send `Origin`. MPR UI's TAuth
    integration sends the configured `X-TAuth-Tenant` header, but the local TAuth
    tenant-header override is disabled, so session restore cannot resolve the
    tenant.
  Requirements:
  - Load the canonical MPR UI configuration and runtime on every auth-aware
    public and application page. LLM Proxy must not load a separate TAuth
    browser client or implement auth endpoint requests, credential exchange,
    cookie handling, session restoration, refresh, logout, or auth redirect
    recovery.
  - Keep `/auth` and `/me` behind the local same-origin frontend proxy and make
    the local TAuth profile resolve the explicit TAuth client tenant header.
  - Verify startup with the exact MPR UI session request headers,
    without a synthetic `Origin`, and keep nonce verification aligned with the
    MPR UI request contract.
  - Exercise the black-box browser flow through the same-origin auth proxy,
    invoke credential exchange through the visible MPR UI control, prove the
    shared-component post-auth redirect to `/app/`, and prove ordinary reload
    plus refresh-cookie recovery remain authenticated.
  - Keep backend request authorization on TAuth's published Go
    `sessionvalidator`; LLM Proxy may enforce only its resource-specific
    management invariants after validation.
  Validation:
  - Operational coverage rejects a local TAuth profile without explicit
    tenant-header resolution and rejects readiness probes that hide the
    browser request shape.
  - The real MPR UI/TAuth black-box test uses MPR UI for login, session
    restoration, refresh, and logout through the frontend
    origin, verifies the session request headers, opens `/app/` after the
    authenticated lifecycle, and survives refresh.
  - Backend coverage exercises the published TAuth validator through the
    protected management API boundary.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - Every auth-aware page now loads the canonical MPR UI configuration and
    runtime. LLM Proxy application modules contain no separate TAuth client,
    TAuth endpoint, cookie, storage, credential-exchange, restore, refresh,
    logout, or redirect implementation.
  - Local TAuth resolves MPR UI's explicit tenant header for same-origin
    session requests, and startup verifies the exact client session and nonce
    request shapes through the frontend proxy.
  - The real browser black box now exchanges a seeded credential through the
    visible MPR UI control, follows MPR UI's authenticated redirect to `/app/`,
    restores the session after reload, recovers with the refresh cookie, and
    signs out through the visible shared control.
  - Backend authorization remains on TAuth's published Go `sessionvalidator`;
    no second token parser or TAuth protocol was added to LLM Proxy.
  - The required baseline and final CI runs passed. The final run passed all
    11 gates in 97 seconds with 84 browser tests, 36 Python tests, the real
    TAuth management black box, live-provider preflight, and exact 100% Go
    statement coverage.

- [x] [B111] (P1) {F019,P004,P005} Make public Log In authenticate directly and unify the site footer.
  Goal:
  Present LLM Proxy as one application by starting the canonical MPR UI/TAuth
  login from every public page, opening `/app/` only after authentication, and
  publishing one compact site footer across public, legal, and app surfaces.
  Evidence:
  - Public `Log In` controls are plain links to `/app/`, so an anonymous user
    reaches a second screen that asks them to sign in again.
  - The anonymous `/app/` panel duplicates the header authentication control
    and makes the public site and authenticated workspace appear unrelated.
  - Public pages hide the legal link while `/app/` renders a different footer;
    `/privacy/` and `/terms/` are not published.
  Requirements:
  - Make the shared public MPR header the declarative `/config-ui.yaml` owner,
    label its authentication action `Log In`, and redirect successful
    interactive authentication to `/app/` through the documented MPR UI
    contract.
  - Redirect an anonymous direct `/app/` visit to `/` after the documented MPR
    UI unauthenticated lifecycle event. Remove the anonymous app panel and do
    not add application-owned session, token, cookie, or TAuth requests.
  - Render one compact non-sticky footer on public pages, `/app/`, `/privacy/`,
    and `/terms/`. Include Resources, Privacy, Terms, GitHub, and an active
    `Built by Marco Polo Research Lab` drop-up with the maintained MPR project
    catalog.
  - Publish deterministic `/privacy/` and `/terms/` documents from one
    repository-owned legal-page source with semantic static content and the
    MPR legal-document component.
  - Rewrite every generated auth-aware header to the absolute production API
    config URL while preserving the local same-origin `/config-ui.yaml`
    contract.
  Validation:
  - Browser integration coverage proves public `Log In` owns the interactive
    authentication flow and redirects to `/app/` only after success.
  - Browser coverage proves an anonymous direct `/app/` visit returns to `/`,
    the obsolete anonymous panel is absent, and authenticated app startup still
    waits for the documented MPR UI lifecycle.
  - Static and hydrated coverage proves all published HTML surfaces use the
    canonical header/footer, legal routes are present in the sitemap, and the
    portfolio drop-up is keyboard accessible at desktop and mobile widths.
  - The site-render contract proves every auth-aware generated page uses the
    production API config URL.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - The canonical MPR header now owns public authentication and redirects to
    `/app/` only after success. Anonymous direct app visits return to `/`, so
    the duplicate sign-in screen and app-owned authentication path are gone;
    static coverage rejects direct public `/app/` anchors.
  - One generated non-sticky footer now publishes Resources, Privacy, Terms,
    GitHub, the theme control, and the keyboard-accessible MPR project drop-up
    across public, legal, resource, and authenticated app surfaces, with a
    crawlable native fallback when JavaScript is unavailable.
  - Deterministic Privacy and Terms pages now provide semantic fallback copy,
    the MPR legal-document component, canonical metadata, and sitemap entries;
    rendered auth config URLs are rewritten across every generated HTML page.
  - The required baseline passed all 11 gates immediately before the final
    correction. The final run passed all 11 gates in 100 seconds with 84
    browser tests, 36 Python tests, the TAuth
    black-box scenario, live-provider preflight, and exact 100% Go statement
    coverage.

- [x] [B110] (P1) {F019,B105,B109} Resolve the remaining public-site review correctness findings.
  Goal:
  Make the public-site validation and local cleanup contracts fail visibly when
  their authoritative inputs or lifecycle state are invalid.
  Requirements:
  - Type-check every production browser module in the binding frontend lint
    gate and resolve the complete module graph without weakening diagnostics.
  - Reject capability catalogs containing an identifier that has no public
    presentation definition.
  - Remove the temporary local-site artifact only after Compose shutdown
    succeeds, and return a failure when automatic shutdown fails.
  Validation:
  - Static frontend validation covers every production browser JavaScript file.
  - Go integration coverage proves unknown capability identifiers fail the site
    render contract.
  - Operational coverage proves failed shutdown retains the mounted artifact
    and exits unsuccessfully.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - Frontend lint now discovers every generator and production browser module,
    normalizes browser-only cache query specifiers in a temporary mirror, and
    runs `tsc --noEmit` across the complete tree. The resulting JSDoc fixes are
    published with coherent application revision `20260806b110`.
  - Site rendering now rejects every capability identifier without exactly one
    public presentation definition and reports the provider and model context
    through `site_render_failed`.
  - Automatic local shutdown now retains the mounted site artifact and exits
    unsuccessfully when Compose shutdown fails; black-box operational coverage
    proves both the retained directory and visible failure receipt.
  - The required baseline passed all 11 gates in 104 seconds. The first
    post-edit run found one stale `b109` cache assertion; after correcting that
    assertion, the final run passed all 11 gates in 101 seconds with 82 browser
    tests, 36 Python tests, the TAuth black-box scenario, live-provider
    preflight, and exact 100% Go statement coverage.
- [x] [B109] (P1) {F019,I209} Resolve the public-site review validation findings.
  Goal:
  Keep cached authenticated-app module graphs coherent, enforce JSDoc
  type-checking in the frontend lint gate, and publish current documentation
  freshness metadata.
  Requirements:
  - Bump one revision across both authenticated-app entrypoints and the complete
    first-party ES-module graph after the renamed integrity-error export.
  - Run `tsc --noEmit` from the binding frontend lint command for edited browser
    JavaScript and add `// @ts-check` to the edited syntax checker.
  - Set the generated `/docs/` sitemap `lastmod` to the current significant
    update date and regenerate the sitemap.
  Validation:
  - Static browser coverage rejects stale or inconsistent authenticated-app
    module revisions.
  - The frontend lint gate passes TypeScript checking and generated-resource
    drift checks.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - Both authenticated-app entrypoints and every first-party runtime import now
    use revision `20260806b109`; static browser coverage rejects an unversioned
    or stale module-graph edge.
  - Frontend lint now runs TypeScript 7 `tsc --noEmit` against the edited
    production browser and generator modules. The syntax checker has
    `// @ts-check`, JSDoc sort unions are explicit, and generator data is
    narrowed before use.
  - The canonical contract-documentation date is `2026-08-06`; regenerated
    resource metadata and `site/sitemap.xml` publish that date for `/docs/`.
  - The required baseline passed all 11 gates in 97 seconds. After correcting
    two stale date assertions found by the first post-edit run, the final run
    passed all 11 gates in 95 seconds with 82 browser tests, 36 Python tests,
    the TAuth black-box scenario, live-provider preflight, and exact 100% Go
    statement coverage.
- [x] [B108] (P1) {F019,I209} Toggle capability filters from the catalog search control.
  Goal:
  Match Kamu's reversible search disclosure so the magnifying-glass control can
  both reveal and collapse the advanced capability filters.
  Requirements:
  - Make consecutive magnifying-glass activations alternate the filter panel
    between visible and hidden states with matching `aria-expanded` state.
  - Preserve automatic disclosure when search input begins or receives focus,
    Escape collapse, Enter disclosure, capability-badge activation, selected
    filters, search results, and sorting state.
  - Keep the complete no-JavaScript catalog and compact responsive layout.
  Validation:
  - Playwright exercises repeated pointer activation, keyboard disclosure and
    collapse, accessibility state, filtering, and narrow-screen containment
    through the rendered public site.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - The magnifying-glass control now alternates the advanced capability panel
    between visible and hidden states and keeps `aria-expanded` synchronized.
  - Search focus, typing, and Enter still disclose filters; Escape collapses
    them. Selected capabilities, filtered results, and table sorting survive a
    collapse-and-reopen cycle, and the full catalog remains available without
    JavaScript.
  - Playwright covers pointer toggling, keyboard behavior, accessibility state,
    preserved filter state, and mobile containment. A headed Chromium check
    confirmed the rendered panel and accessibility state on both clicks.
  - The required baseline passed all 11 gates in 95 seconds. The final
    post-edit run passed all 11 gates in 96 seconds with 82 browser tests, 36
    Python tests, the TAuth black-box scenario, live-provider preflight, and
    exact 100% Go statement coverage.
- [x] [B107] (P1) Add the standard `make down` local service command.
  Goal:
  Provide a symmetric public shutdown command for every local Compose resource
  started by `make up`.
  Requirements:
  - Declare `down` as a phony Make target and route it through the exact local
    Compose project and file owned by `make up`.
  - Stop the local containers, project network, and orphaned services through
    `docker compose down --remove-orphans` while retaining the named local data
    volumes.
  - Keep the Compose identity in one canonical declaration shared by startup
    and shutdown, and fail visibly when Docker Compose or shutdown fails.
  - Let shutdown run independently from private local-environment preparation.
  Validation:
  - Exercise the real `make down` boundary with a fake Docker edge and prove the
    target remains phony, selects the exact local project and Compose file,
    removes orphans, and does not delete named volumes.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - `make down` is a phony public target that stops the exact
    `llm-proxy-local` Compose project with orphan cleanup and retains its named
    TAuth and management data volumes.
  - Startup and shutdown consume one shared Compose project and file identity.
    Shutdown validates Docker Compose, runs independently from private local
    environment preparation, propagates failures, and prints a terminal
    shutdown receipt on success.
  - Black-box Make coverage proves the target runs even when a `down` file is
    present and invokes the exact project/file command without a volume-removal
    option. The required baseline and final `make ci` runs each passed all 11
    gates in 95 seconds with 82 browser tests, 36 Python tests, and exact 100%
    Go statement coverage.
- [x] [B106] (P1) {F019} Remove workspace terminology from the web site.
  Goal:
  Present one public product site and one authenticated LLM Proxy app without
  exposing a separate workspace concept anywhere in browser-served content.
  Evidence:
  - The shared public shell already links to `/app/` as `Log In`, but landing,
    app lifecycle, metadata, and generated resource copy still say workspace.
  - One generated resource URL and several frontend identifiers also publish
    the obsolete term in static site bytes.
  Requirements:
  - Use `Log In` for public navigation to `/app/`, `App` for application
    lifecycle copy, `account` for signed-in ownership, and `tenant` for exact
    technical isolation or persistence contracts.
  - Remove the obsolete term from every browser-served HTML, JavaScript, CSS,
    XML, metadata, structured-data, and URL artifact under `site/`.
  - Update canonical generators and product documentation, regenerate the
    resource cluster, and delete the obsolete resource URL without an alias.
  Validation:
  - Add a static publication guard and browser coverage that fail on any
    case-insensitive occurrence in the served site.
  - Verify the public shell and authenticated app copy in Chromium.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - Every browser-served artifact now uses `Log In` for `/app/`, `App` for
    lifecycle copy, `account` for signed-in ownership, and `tenant` for exact
    isolation and persistence contracts. The obsolete term has zero
    case-insensitive occurrences under `site/`.
  - Canonical generators and product documentation were updated. The resource
    URL is now `/resources/multi-tenant-ownership-migration/` without an alias;
    its evidence, metadata, sitemap date, publication brief, and migration
    runbook CTA passed the independent SEO evaluation.
  - Static publication validation and the 80-test Playwright suite reject any
    recurrence. Chromium verified the landing `Log In` navigation and the
    authenticated app sign-in state.
  - The required baseline passed all 11 gates in 91 seconds. The final run
    passed all 11 gates in 90 seconds with 80 browser tests, 36 Python tests,
    the TAuth black-box scenario, exact OpenAPI Pages publication,
    live-provider preflight, and exact 100% Go coverage.
- [x] [B105] (P1) {F019} Populate the local landing capability matrix.
  Goal:
  Make the localhost landing page publish the current validated provider and
  model capability catalog instead of an empty section.
  Evidence:
  - Release rendering replaces `<!-- llm-proxy-capability-catalog -->` with the
    sanitized catalog projected from `configs/config.yml`.
  - `make up` mounts the unrendered `site/` source directly into ghttp, so the
    local `/` page retains only the marker and displays no matrix.
  Requirements:
  - Keep `proxy.NewPublicCapabilityCatalog` and the Go site renderer as the
    single validated catalog path.
  - Render a temporary local site artifact from the active configuration before
    ghttp starts, and serve that artifact through `http://localhost:4179/`.
  - Remove the temporary artifact when local orchestration stops.
  Validation:
  - Exercise the local orchestration contract and prove its served landing page
    contains the generated provider/model matrix without private provider data.
  - Verify the populated matrix in Chromium.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - `make up` now renders an isolated temporary public-site artifact from the
    active `configs/config.yml` through the existing validated Go catalog
    renderer. The API health gate rejects a missing or unrendered matrix before
    ghttp serves the artifact read-only, and shutdown removes the artifact.
  - The orchestration contract verifies generated matrix content, rejects
    private provider configuration, and proves cleanup after interruption.
    Chromium verified the live localhost catalog with 12 text providers, 53
    text routes, 4 dictation providers, and 5 dictation routes.
  - The required baseline passed all 11 gates in 93 seconds. The final run after
    the last code edit passed all 11 gates in 97 seconds with 79 browser tests,
    36 Python tests, the TAuth black-box scenario, exact OpenAPI Pages
    publication, live-provider preflight, and exact 100% Go coverage.
- [x] [B104] (P1) {F019} Expose explicit OpenAPI view and download actions.
  Goal:
  Let a visitor inspect the canonical OpenAPI manifest in the browser or
  download its exact YAML bytes from the existing human-readable reference.
  Evidence:
  - `/docs/` is already generated from `docs/openapi.yaml`, but its
    `Download exact schema` link has no `download` contract and opens the raw
    YAML instead.
  - The shared footer links directly to `/openapi.yaml`, bypassing the
    human-readable reference and leaving no explicit view-versus-download
    choice.
  Requirements:
  - Keep `docs/openapi.yaml` as the only hand-maintained schema source.
  - Use `/docs/` as the schema viewer and expose separate raw-view and YAML
    download actions backed by `/openapi.yaml`.
  - Route the shared public OpenAPI navigation to the viewer actions.
  Validation:
  - Prove the generated viewer and downloaded file equal the canonical source
    byte for byte and use an explicit download filename.
  - Keep the generated documentation provenance and drift checks current.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - `/docs/#openapi-schema` is the public schema action surface. Its generated
    reference offers `View YAML`, a bounded inline viewer containing the full
    canonical manifest, and `Download YAML`, which downloads `/openapi.yaml`
    as `llm-proxy-openapi.yaml`.
  - The viewer content, human-readable operations, source digest, and download
    all derive from `docs/openapi.yaml`; no second editable schema or external
    viewer was introduced. The shared public footer now opens these actions.
  - Playwright verifies the inline viewer text and downloaded bytes equal the
    canonical source exactly, including the explicit filename. The required
    baseline passed all 11 gates in 92 seconds, and the final run after the
    last code edit passed all 11 gates in 96 seconds with 79 browser scenarios,
    the TAuth black-box scenario, exact OpenAPI Pages publication,
    live-provider preflight, and exact 100% Go coverage.
- [x] [B103] (P1) {F019} Use the shared MPR header and footer on every public page.
  Goal:
  Give `/docs/` and every public page the same declarative MPR shell.
  Evidence:
  - `/docs/` and generated resource pages use custom native header and footer
    wrappers instead of `mpr-header` and `mpr-footer`.
  - The landing page uses `mpr-footer` but still owns a separate native header.
  Requirements:
  - Define one canonical public `mpr-header` and compact `mpr-footer` contract.
  - Apply it to `/`, `/docs/`, `/resources/`, and every generated resource page.
  - Load the MPR UI stylesheet and bundle on every public route family.
  - Preserve exactly one header, one main region, and one footer in document
    order without changing the authenticated `/app/` shell.
  Validation:
  - Prove the generated HTML contains only the shared components and no custom
    public shell wrappers.
  - Exercise component hydration, navigation, document order, and compact
    responsive geometry through Playwright.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - One shared renderer now publishes the exact same compact `mpr-header` and
    `mpr-footer` on the landing page, `/docs/`, `/resources/`, and all 46
    generated resource articles. Every public route loads the MPR UI assets,
    and the page-specific native shell wrappers and their CSS are removed.
  - The landing-shell and OpenAPI generators reject drift from the canonical
    renderer. Playwright verifies byte-identical shell markup across all 49
    sitemap HTML pages and hydrated desktop and mobile behavior for every
    public route family, including header-main-footer order and compact footer
    geometry. The authenticated `/app/` shell remains unchanged.
  - The required baseline and post-change `make ci` runs pass. The final run
    followed the last code edit and passed all 11 gates in 90 seconds: exact
    100% Go coverage, 36 Python tests, 78 browser scenarios, the TAuth
    black-box scenario, exact OpenAPI Pages publication, and live-provider
    preflight.
- [x] [B102] (P1) {F019} Publish the authenticated web application only at `/app/`.
  Goal:
  Make `/app/` the single canonical route for the authenticated web application.
  Evidence:
  - The authenticated site source currently lives under a management-named
    directory, and generated landing, API, and resource links use that route.
  Requirements:
  - Move the authenticated site source and release renderer to `/app/`.
  - Update canonical metadata, generators, documentation, and public links to
    use `/app/`.
  - Do not retain a second application route, redirect, alias, or compatibility
    path.
  - Keep the authenticated application out of the public sitemap and retain its
    `noindex` metadata.
  Validation:
  - Prove `/app/` renders the authenticated application and the removed route
    returns `404`.
  - Prove generated OpenAPI and resource pages link only to `/app/`.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - The authenticated source and rendered release artifact now live only at
    `site/app/index.html` and `/app/`; the removed route has no source,
    redirect, alias, or compatibility handler and returns `404`.
  - Landing, OpenAPI, resource, README, canonical metadata, and generated SEO
    references now point to `/app/`. The application remains `noindex` and is
    excluded from the public sitemap.
  - Renderer, static-site, Playwright, and TAuth black-box coverage exercise
    `/app/` and explicitly reject the removed route where applicable.
  - The required baseline and post-change `make ci` runs pass. After formatting
    was applied, the final run followed the last code edit and passed all 11
    gates in 90 seconds: exact 100% Go coverage, 36 Python tests, 76 browser
    scenarios, the TAuth black-box scenario, exact OpenAPI Pages publication,
    and live-provider preflight.
- [x] [B101] (P1) {I029,F019} Serve the canonical OpenAPI schema from local ghttp.
  Goal:
  Make `http://localhost:4179/openapi.yaml` serve the same current OpenAPI file
  used by release publication without introducing a second schema source.
  Evidence:
  - Local ghttp mounts only `site/`, where `openapi.yaml` is intentionally
    absent, so the landing-page OpenAPI links return `404` under `make up`.
  - Release rendering already stages exact `docs/openapi.yaml` bytes and CI
    rejects both publication drift and a tracked `site/openapi.yaml` duplicate.
  Requirements:
  - Mount `docs/` read-only in a schema-only ghttp service and proxy the exact
    `/openapi.yaml` path through the local frontend.
  - Keep `site/openapi.yaml` forbidden and retain `docs/openapi.yaml` as the
    only hand-maintained schema.
  - Make local startup verify the schema route and make browser coverage
    exercise the rendered artifact rather than a test-only schema handler.
  Validation:
  - Prove the local Compose mount and startup probe through the operational
    black-box test.
  - Prove the rendered public schema remains byte-equivalent to the canonical
    source through Playwright and the Pages artifact gate.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - Local Compose now mounts `docs/` read-only in a schema-only ghttp service
    and proxies only `/openapi.yaml` through the public local frontend, keeping
    `docs/openapi.yaml` as the sole editable contract.
  - `make up` now requires the schema service and a successful public schema
    probe before reporting ready. A real-stack acceptance returned `200`, and
    the response matched `docs/openapi.yaml` byte-for-byte.
  - Operational coverage proves the mount, proxy, service, and readiness
    contracts. Playwright stages the real Pages artifact and serves its schema
    from disk, while the existing publication gate continues to reject drift
    and any tracked `site/openapi.yaml` duplicate.
  - The required baseline and post-change `make ci` runs pass. The final run
    followed the last code edit and passed all 11 gates in 89 seconds: exact
    100% Go coverage, 36 Python tests, 76 browser scenarios, the TAuth black-box
    scenario, exact OpenAPI Pages publication, and live-provider preflight.
- [x] [B100] (P0) Make declared frontend validation self-contained in a clean checkout.
  Goal:
  Make the public `make test` and `make ci` contracts install their exact pinned
  frontend dependencies and Chromium before invoking Playwright.
  Evidence:
  - Agentic execution of recurring issue M001R at exact remote revision
    `95457e317a93e85b6b225babd9064284089dab1d` passed Go and Python baseline
    validation, then failed with `playwright: not found` and zero provider
    requests.
  - The GitHub workflow installs npm dependencies and Chromium before calling
    `make ci`, while the repository Make targets assume that untracked state
    already exists.
  Requirements:
  - Give Make one canonical dependency-preparation target using the pinned npm
    lock and the declared Chromium browser.
  - Make clean `make test`, `make lint`, focused frontend targets, and `make ci`
    cross that preparation boundary before frontend validation.
  - Remove workflow-only duplicate setup so hosted and Agentic execution use
    the same public contract.
  - Keep dependency state untracked and do not add an alternate validation
    path or fallback browser.
  Validation:
  - Add a black-box Make regression that invokes the public targets with an
    empty dependency state and records exact npm preparation/test ordering.
  - Run the focused regression and the required final
    `timeout -k 350s -s SIGKILL 350s make ci`.
  Resolution:
  - Make now installs the exact lockfile graph and invokes its pinned Playwright
    binary to install Chromium into ignored project-local state before any
    frontend validation target runs.
  - The preparation stamp makes recursive `make ci` stages reuse that exact
    state, while a changed package manifest or lockfile requires preparation
    again.
  - Black-box Make fixtures prove clean focused frontend, `make test`, and
    `make ci` executions prepare dependencies exactly once and in the required
    order. Hosted CI now delegates the same setup to `make ci` instead of
    maintaining a second workflow-only path.
  - The focused dependency-contract target, real pinned dependency and browser
    preparation, 75 frontend browser tests, and one TAuth browser black-box
    test passed. The required final `make ci` returned zero with all 11 gates
    complete and exact 100% Go statement coverage.
- [x] [B098] (P0) Make canonical CI completion fail closed and visible.
  Goal:
  Make `make ci` prove that every declared gate completed in the current run,
  show the enforced coverage at the terminal tail, and return nonzero whenever
  orchestration stops before that proof is complete.
  Evidence:
  - A captured successful baseline returned zero after all current gates, but
    its `total: ... 100.0%` coverage line appeared at line 533 while the final
    line 650 was only the live-provider harness preflight message.
  - The dependency-only `ci` target has no start/end receipt, active-stage
    failure report, fresh run identity, or terminal success assertion.
  - A future summary that reads the repository-level ignored `coverage.out`
    could accept stale evidence when a coverage command exits zero without
    producing a current artifact.
  - Hosted CI selects Go `1.25.12` independently while `go.mod` and both
    production builders require Go `1.26.5`.
  Requirements:
  - Run the canonical gates sequentially through one top-level runner even when
    the caller supplies parallel Make flags.
  - Treat every exit before terminal completion as failure, including an
    accidental zero exit from the runner, and identify the active stage.
  - Require a fresh run-scoped coverage artifact and independently verify exact
    100% Go statement coverage after all test gates.
  - Print one terminal table containing every completed gate, the coverage
    result, elapsed time, and an unambiguous `CI PASSED` line only after the
    complete contract succeeds.
  - Select the hosted Go toolchain from `go.mod` instead of a second version
    declaration.
  Validation:
  - Add black-box runner scenarios for complete success, a nonzero child gate,
    and a child that returns zero without producing current coverage evidence.
  - Prove failure output names the interrupted stage and never prints the
    success receipt.
  - Run the required final
    `timeout -k 350s -s SIGKILL 350s make ci`.
  Resolution:
  - `make ci` now owns one sequential ten-stage runner with an exit trap that
    converts every incomplete exit into failure and names the active gate.
  - Go coverage is written to a private artifact created for that invocation,
    verified at its producer and again after the final test stage, then removed.
    A zero-exit stage without current coverage evidence fails before completion.
  - The terminal output now contains per-gate receipts, elapsed time, exact Go
    coverage, and an explicit `CI PASSED` line emitted only after private
    run-state cleanup succeeds. Cleanup failure remains a named failing stage
    and cannot print the terminal summary or success receipt.
  - Black-box process coverage proves complete success, exact propagation of a
    child exit 23, rejection of a zero-exit test sequence missing its current
    coverage artifact, and rejection of cleanup failure after every declared
    gate completes. Hosted CI now selects its Go version directly from
    `go.mod`.
  - The required final `timeout -k 350s -s SIGKILL 350s make ci` returned zero
    after the last code edit with exact 100% Go statement coverage, 33 Python
    tests, 75 browser tests, one TAuth browser black-box test, the OpenAPI Pages
    artifact check, and the live-provider harness preflight. Its terminal table
    reported all 11 gates passed in 86 seconds.
- [x] [B096] (P0) Make deployment self-contained in the application repository.
  Goal:
  Preserve the exact `make release`, `make publish`, `make deploy` operator
  surface without requiring an installed MPRLab controller, an
  `mprlab-gateway` source checkout, or any sibling repository.
  Evidence:
  - The merged B095 implementation reduced `make deploy` to
    `mprlab-deploy`, and a clean operator invocation failed with
    `make: mprlab-deploy: No such file or directory`.
  - Installing a content-addressed executable under `~/.local/bin` repaired
    that machine but made application deployment depend on hidden,
    machine-global MPRLab state.
  - The release gate also intermittently failed its asynchronous `make up`
    orchestration fixture because the test reused a five-second help-command
    deadline for complete Compose startup and shutdown.
  - A fresh CI run rewrote tracked generated Python `*.egg-info` metadata from
    version `0.1.0` to the canonical project version `0.2.0`, dirtying the
    worktree that release admission requires to remain clean.
  Requirements:
  - Execute only tracked files from this repository plus ordinary documented
    tools such as Git, Docker, Python/uv, Ansible, SSH, and GitHub CLI.
  - Keep deployment playbooks, tasks, inventory documentation, and resource
    declarations under `.mprlab/deploy`; do not locate, download, install, or
    execute an MPRLab binary, gateway checkout, bundle, or sibling repository.
  - Preserve exact sealed-release and published-artifact admission before any
    remote mutation.
  - Keep retries convergent and conflicts fail-closed.
  - Give asynchronous orchestration acceptance its own bounded timeout so
    machine load cannot make an unchanged release nondeterministic.
  - Keep generated Python build metadata untracked.
  - Keep production deployment user-owned.
  Validation:
  - Add black-box Make scenarios proving dry-run and deployment delegation use
    only the current repository's tracked Ansible entrypoint.
  - Prove an absent `mprlab-deploy`, absent gateway checkout, and malformed
    sibling repository cannot affect the transaction.
  - Run the required final
    `timeout -k 350s -s SIGKILL 350s make ci`.
  Resolution:
  - `make deploy`, `make deploy-dry-run`, and `make deploy-syntax` now execute
    the complete tracked `.mprlab/deploy/ansible` transaction through pinned
    `ansible-core`; no MPRLab-specific executable, controller bundle, gateway
    checkout, repository-parent selector, or sibling repository is resolved.
  - Admission proves clean `master`, the exact sealed release commit and
    annotated tag, the published branch, equal version/`latest` image digests,
    the published Pages artifact, and regular mode-`0600` private inputs before
    remote mutation.
  - The convergent transaction replaces only llm-proxy's TAuth tenant and
    owned origins, validates the complete Caddy route set before activation,
    pulls only a missing immutable image, converges one Compose service,
    verifies both declared public boundaries, and activates Pages last.
  - Black-box coverage runs the real playbook twice against an isolated target,
    preserves unrelated TAuth/Caddy state, proves one image pull/restart/reload,
    and remains successful with a malformed sibling plus obsolete external
    selector variables.
  - The local-orchestration acceptance test now has its own bounded 30-second
    deadline, and generated Python `*.egg-info` files are no longer tracked, so
    ordinary CI load and package metadata generation do not dirty or randomly
    fail the release gate.
  - The ignored operator inventory was migrated in place at mode `0600`; the
    rejected orphaned controller cache was moved to Trash. No production
    deployment command was run.
  - The required final `timeout -k 350s -s SIGKILL 350s make ci` passes with
    exact 100% Go statement coverage, 33 Python tests, 75 browser tests, one
    TAuth browser black-box test, 57 release/deployment tests, and the
    live-provider harness preflight.
- [x] [B095] (P0) Keep deployment declarations app-owned and execution platform-owned.
  Goal:
  Preserve the standard `make release`, `make publish`, `make deploy` lifecycle
  while making every retry convergent and removing deployment-resource
  orchestration from llm-proxy.
  Evidence:
  - The prior deploy flow selected sibling checkouts and failed on unrelated
    app manifests.
  - A proposed app-bundled gateway archive moved platform implementation into
    llm-proxy and introduced direct TAuth/Caddy awareness instead of removing
    the coupling.
  Requirements:
  - Keep only declarative deployment resource and Ansible inventory YAML in
    `.mprlab/deploy`.
  - Make `make deploy` invoke one installed neutral controller with no
    selectors, gateway paths, image arguments, or resource-specific flags.
  - Resolve exactly the current Git repository; never scan or validate sibling
    checkouts during this app deployment.
  - Verify immutable release and publication state before mutation, reuse exact
    matches, reject conflicts, and converge when any lifecycle command is
    repeated.
  - Keep TAuth runtime integration in the published client/session boundary and
    keep TAuth/Caddy deployment reconciliation outside application code.
  Validation:
  - Add black-box repeated zero-argument Make delegation and declaration-only
    repository contracts.
  - Validate the target-neutral gateway controller and its idempotent installer
    without production contact.
  - Run the required final
    `timeout -k 350s -s SIGKILL 350s make ci`.
  Resolution:
  - `make deploy` is now a zero-argument handoff to the installed
    `mprlab-deploy` controller. It carries no product selector, gateway
    checkout, image override, or repository-parent input.
  - The app-owned deployment surface is limited to the committed resource
    manifest and conventional Ansible inventory YAML. App-bundled controller
    archives, locks, playbooks, shell/Python orchestration, and capacity
    readers were removed.
  - Black-box contracts prove repeated delegation remains exact, obsolete
    environment inputs cannot change the handoff, controller failures
    propagate, and only the declaration files remain tracked.
  - The gateway controller and installer gates pass without production
    contact. The required final repository `make ci` passes.
- [x] [B094] (P1) Run the full CI suite once per release lifecycle.
  Goal:
  Make `make release` the sole full-suite validation stage while requiring
  `make publish` and `make deploy` to prove they continue that exact sealed
  release.
  Evidence:
  - `tools/gitrelease/scripts/prepare_release.sh` runs `make ci` before it
    seals `.git/mprlab-release`.
  - `make publish` already consumes and validates that sealed manifest.
  - `scripts/deploy.sh` reruns `make ci` before deployment and exposes
    `--skip-ci` plus a deploy-only CI timeout, even though it separately checks
    the release tag, published image, and Pages artifact.
  Requirements:
  - Keep the complete `make ci` gate in `make release`.
  - Remove the deploy-time CI run, skip flag, and deploy-only CI timeout.
  - Make deployment fail before gateway, registry, or Pages work unless the
    local sealed manifest identifies the exact release tag and commit being
    deployed.
  - Keep publication and deployment artifact verification intact; do not add a
    fallback, compatibility path, or manually asserted success marker.
  Validation:
  - Add black-box lifecycle scenarios for a missing sealed release, a sealed
    version mismatch, and a valid continuation that reaches the gateway
    without invoking the fixture `ci` target.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  - Run the gateway `make verify-app-workflows` cross-repository contract
    without production contact.
  Resolution:
  - `make release` remains the lifecycle's sole full `make ci` gate.
    Deployment no longer has a CI invocation, CI timeout, or `--skip-ci`
    bypass.
  - `make deploy` now requires `local-release-state` to report `sealed` and
    requires its version and release commit to match the selected annotated
    tag and deploy `HEAD` before any gateway, registry, or Pages work.
  - Black-box coverage rejects a missing seal, mismatched version, mismatched
    commit, the removed CI bypass, and a noncanonical tag; the valid sealed
    continuation reaches the gateway without invoking the fixture CI target.
  - The required baseline and final repository `make ci` runs pass. The final
    run includes exact 100% Go statement coverage, 33 Python tests, 75 browser
    tests, the authentication black-box test, 58 release-tool tests, and the
    live-provider harness preflight.
  - Gateway `make verify-app-workflows` reports llm-proxy ready and passes the
    cross-repository lifecycle contract without production contact.
- [x] [B093] (P0) Publish prepared OCI platform indexes without rejecting attestations.
  Goal:
  Make `make publish` converge when current Docker Buildx publishes each
  prepared platform image as an OCI index containing the runnable image
  manifest and its provenance attestation.
  Evidence:
  - The real `make release` completed for `v0.2.50`, including all CI gates,
    both platform archives, the Pages archive, the changelog-only release
    commit, annotated tag, and sealed manifest.
  - The first `make publish` pushed `master`, `v0.2.50`, the GitHub Release,
    `manifest.json`, `pages.tar.gz`, and the prepared amd64 platform tag before
    failing with `published platform tag is not a single immutable image
    manifest`.
  - Standard `docker buildx imagetools inspect --raw` proves the platform tag
    is an OCI index whose digest exactly equals the prepared local image ID. It
    contains one `linux/amd64` image manifest and one `unknown/unknown` SLSA
    attestation manifest that references the runnable manifest.
  - The publisher assumes a bare image manifest and compares its config digest
    with the local image ID. With the current Docker containerd image store,
    that local ID identifies the complete platform index instead.
  Requirements:
  - Treat one OCI index with exactly one declared Linux platform image and its
    matching provenance attestation as the canonical platform artifact.
  - Require the remote platform-index digest to equal the prepared local image
    ID before reusing it.
  - Validate a version index as the exact union of the descriptors from every
    prepared platform index, including their attestations.
  - Preserve partial publication and reuse exact remote state; do not rebuild,
    replace, delete, or retag an immutable published object.
  - Continue using only standard Docker CLI publication and inspection
    boundaries.
  Validation:
  - Cover fresh publication, exact retry, missing version/latest recovery,
    uncertain inspection, and immutable conflict through the black-box release
    suite.
  - Run the required final
    `timeout -k 350s -s SIGKILL 350s make ci` after the last code edit.
  - Merge through a ready PR with hosted CI, then verify the forward release
    with exact `make release && make publish` retries on clean `master`.
  Resolution:
  - Platform publication now accepts only the canonical OCI index containing
    exactly one runnable Linux image descriptor and its matching provenance
    attestation, and requires the remote index digest to equal the prepared
    image ID.
  - Version publication now validates the exact descriptor union from every
    prepared platform index, preserving immutable partial state across retries.
  - The 54-scenario black-box release suite, direct standard-Docker inspection
    of the published platform index and composed version index, and the final
    repository `make ci` all pass.
- [x] [B092] (P0) Make the container inspection-bound test deterministic.
  Goal:
  Verify that every registry inspection is independently bounded without
  coupling the release gate to nested wall-clock timers.
  Evidence:
  - B091's final `make ci` proved the repaired operational test under a
    22.777-second Go package run, then failed in
    `test_container_manifest_digest_bounds_each_inspection_attempt`.
  - The container test expected two fake Docker attempts but observed one
    during a 151.503-second release-suite run.
  - The fixture wraps two real one-second `timeout` calls and a real one-second
    delay inside an outer five-second timeout. Host scheduling can consume the
    outer budget before the second process starts, so the assertion measures
    scheduler timing rather than the script's per-attempt command contract.
  Requirements:
  - Replace real nested timers with fake `timeout` and `sleep` process
    boundaries that capture and validate the exact arguments.
  - Require two independently bounded Docker inspection commands and one
    configured inter-attempt delay.
  - Do not increase a timeout or change the production registry-readiness
    script.
  Validation:
  - Run the focused release suite and the required final
    `timeout -k 350s -s SIGKILL 350s make ci`.
  - Do not run `make release`, `make publish`, or `make deploy`.
  Resolution:
  - Replaced the nested real timers with fake `timeout` and `sleep`
    executables that capture the public command boundary.
  - The test now requires two exact one-second Docker inspection bounds, one
    configured inter-attempt delay, Docker exit `124` reporting, and the final
    unreadable-manifest error without depending on scheduler timing.
  - All 54 repository-owned release tests passed in 87.971 seconds under the
    same contended host conditions.
- [x] [B091] (P0) Remove synthetic local-orchestration latency from the release gate.
  Goal:
  Keep the black-box `make up` contract deterministic when `make release` runs
  the complete CI suite under host contention.
  Evidence:
  - The user-run release gate failed after five seconds waiting for the fake
    management-boundary `curl-ready` file even though the proxy tests around
    it continued to pass.
  - Ten isolated runs of
    `TestOperationalMakeUpStartsLocalWebOrchestration` passed in
    1.95–2.08 seconds, while the same Go package took 25.444 seconds in the
    failing release gate.
  - The fixture deliberately sleeps 150 ms in every fake `awk` invocation,
    even though it already reads the invocation capture and rejects more than
    seven processes. The sleep adds no contract evidence and turns unrelated
    host scheduling into readiness behavior.
  - The required pre-change `make ci` passed, but showed the same host
    contention: the Go `tests` package took 16.562 seconds and the release
    suite took 106.887 seconds.
  Requirements:
  - Remove the fake `awk` sleep and its environment control. Do not increase
    the five-second diagnostic guard.
  - Retain the real `make up` entrypoint, fake Docker/HTTP boundaries, exact
    readiness assertions, and the process-count guard that detects
    per-variable environment projection.
  - Do not change production `scripts/up.sh`; its batched environment
    projection and readiness sequence are not the failing contract.
  Validation:
  - Run the focused black-box operational test repeatedly.
  - Run the required final
    `timeout -k 350s -s SIGKILL 350s make ci` after the last code edit.
  - Do not run `make release`, `make publish`, or `make deploy`.
  Resolution:
  - Removed the fake `awk` delay and its environment control without changing
    the five-second diagnostic guard or production `scripts/up.sh`.
  - The test still exercises the real `make up` entrypoint and asserts the
    exact Compose, HTTP readiness, scoped-environment, cleanup, and maximum
    process-count contracts.
  - Thirty consecutive focused black-box runs passed in 36.283 seconds.
- [x] [B090] (P0) Make release, publication, and deployment retries converge on one sealed release.
  Goal:
  Make the canonical `make release && make publish && make deploy` lifecycle
  safe to rerun after any completed or partially completed phase.
  Evidence:
  - Local and refreshed `origin/master` both point at release commit
    `2b8202753f4e2022a0d58d47c575cf6a3472fae8`, and the annotated `v0.2.49`
    tag resolves to that same commit.
  - A second `make release` selected `v0.2.50`, initialized a replacement
    staging area, rebuilt at least the Pages payload, and then failed because
    `v0.2.49..HEAD` contains no commits. The retry erased the sealed local
    `v0.2.49` manifest, notes, and container payloads instead of recognizing
    the exact release at `HEAD`.
  - GitHub already has the non-draft `v0.2.49` release with matching
    `manifest.json` and `pages.tar.gz`. GHCR has readable `v0.2.49` and
    `latest` manifests at the same digest. The public Pages marker still names
    `v0.2.48`, so the lifecycle is legitimately between publication and Pages
    activation.
  - Publication currently edits an existing GitHub Release, uploads assets
    with `--clobber`, and republishes every container platform and manifest on
    every retry. These are mutable replays rather than exact-state reuse and
    immutable-conflict rejection.
  Requirements:
  - Treat a validated release manifest and its exact tag/release/source commit
    relationship as the authoritative sealed local release. An exact retry at
    that release commit must return success without selecting a new version,
    rerunning CI, rebuilding payloads, changing Git, or replacing artifacts.
  - Prepare a new release outside the last sealed artifact directory and
    activate it only after its manifest, notes, payload inventory, release
    commit, and annotated tag are complete. Preserve the prior sealed artifact
    after any failed preparation.
  - Make publication resumable and immutable: skip exact GitHub Release
    metadata/assets and exact platform/version manifests, publish only missing
    state, update `latest` only when needed, and reject any existing immutable
    tag, release metadata, asset, platform, or version-manifest conflict.
  - Keep deployment convergent: reuse an exact Pages branch and matching
    configuration, wait on an existing queued/building build, and request
    exactly one replacement build when the matching commit has no build or its
    newest build failed. Never create duplicate work while a matching build is
    active.
  - Keep the backend deployment on the app-owned gateway target. Reapplying
    the exact published image remains an idempotent desired-state operation;
    do not add an alternate deployment route, mutable source checkout, or
    manual recovery command.
  Validation:
  - Add black-box lifecycle tests for an exact `make release` retry, failed
    preparation preserving the prior sealed artifact, interrupted-phase
    recovery, exact and conflicting GitHub Release assets, exact and
    conflicting container manifests, partial publication resume, repeated
    Pages activation, active Pages builds, and one failed/missing build retry.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair. Do not run production
    release, publish, Pages activation, or gateway deployment commands.
  Resolution:
  - Exact release commits now reuse their validated sealed manifest, while new
    payloads are prepared separately and atomically activated only after the
    changelog-only commit, annotated tag, notes, and payload inventory agree.
    An interrupted release commit resumes from its prepared payloads and a
    failed new preparation leaves the prior sealed release unchanged.
  - GitHub Release metadata/assets and GHCR platform/version manifests now
    reuse exact existing state, publish only confirmed-missing state, and
    reject immutable conflicts. Ambiguous registry reads fail closed and
    `latest` changes only when its digest differs.
  - Pages activation now reuses an exact branch/configuration and built or
    active matching build, with one bounded replacement request for a missing
    or failed matching build.
  - The repository-owned black-box release suite covers exact retries,
    interrupted/partial resumes, conflict rejection, fail-closed registry
    inspection, and repeated Pages activation.
- [x] [B089] (P1) Return sanitized, correlated provider errors at the public proxy boundary.
  Goal:
  Let clients distinguish a proxy status from the exact upstream provider
  condition without exposing provider-controlled error bodies or messages.
  Requirements:
  - Replace provider-originated plaintext `429` and `502` bodies with one
    canonical JSON envelope containing `code`, canonical `provider`,
    `upstream_status`, `retryable`, proxy-owned `request_id`, and
    `retry_after`.
  - Keep every field present. Use `null` for `upstream_status` when no usable
    unsuccessful provider HTTP response exists and for `retry_after` when the
    provider omitted it or supplied an invalid value.
  - Preserve upstream `429` as public `429`; continue mapping every other
    provider failure to public `502`, with the exact received status carried
    separately in `upstream_status`.
  - Set `retryable` only for upstream HTTP `408`, `425`, `429`, `500`, `502`,
    `503`, and `504`. Document that this classification does not make LLM
    requests idempotent or remove duplicate-work and billing risk.
  - Accept only unsigned delta seconds or parseable HTTP dates from an
    upstream `Retry-After`, normalize the value, and return it in both JSON and
    the response header. Drop malformed values.
  - Generate the request ID inside the proxy, return it in
    `X-LLM-Proxy-Request-ID`, and record the same value with sanitized
    provider metadata in structured logs.
  - Apply the contract to OpenAI Responses and dictation,
    OpenAI-compatible providers, Gemini, and Anthropic. Never retain a raw
    provider error body in the public response or provider-failure log.
  Validation:
  - Exercise the public router against controlled OpenAI, Anthropic, Meta,
    Gemini, Moonshot, and dictation failures. Assert exact proxy and upstream
    statuses, retryability, normalized and rejected `Retry-After` values,
    request-ID correlation, JSON schema, and raw-body non-disclosure.
  - Validate the canonical OpenAPI document and generated reference against
    the new `429` and `502` contract.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - Added typed provider HTTP metadata across every provider adapter and a
    single public error writer that returns the six-field sanitized envelope.
  - Added proxy-owned request IDs to response headers and structured request,
    response, authentication-failure, and provider-failure logs.
  - Removed OpenAI raw response-body and response-text logging and retained
    only validated `Retry-After` data for provider failures.
  - Added public-boundary coverage for all five live-test providers plus
    dictation and response-protocol failures, and updated README/OpenAPI
    documentation and generated API reference.
- [x] [B086] (P1) Make Default-tenant production live tests repeatable.
  Goal:
  Provide one paid, production-boundary command that proves the Default tenant
  can route text through its saved provider credentials without putting any
  upstream credential in the local test environment.
  Requirements:
  - Add `make live-test`, separate from the disposable local provider harness
    and excluded from `make ci`.
  - Require only the canonical `LLM_PROXY_DEFAULT_TENANT_KEY` tenant client secret. The
    command must call `https://llm-proxy-api.mprlab.com`, must not load a dotenv
    file, and must never read, accept, or send local provider API keys.
  - Use that secret to select the Default tenant and test exactly OpenAI,
    Anthropic, Meta, Gemini, and Moonshot through canonical `POST /v2` calls
    with each provider's saved Default-tenant model.
  - For every listed provider, send one short echo-marker request. Also send
    the same deterministic, large completion request through OpenAI,
    Anthropic, and Meta. The OpenAI Responses case must remain open through
    the server-owned background polling lifecycle before returning its final
    marker; the Anthropic and Meta cases must wait for their canonical
    synchronous provider completions.
  - Run every case even after a failure, redact all credentials and response
    bodies from output, and return nonzero when any case does not return the
    expected completed response.
  Validation:
  - Add black-box operational coverage for `make live-test` using a fake curl
    boundary. Prove all eight canonical requests use the production origin,
    Default-tenant secret query authentication, exact provider selection, no
    explicit model, and all three large-completion request shapes.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair, then execute the paid
    `make live-test` command and record its exact safe provider outcomes.
  Resolution:
  - Extended the production-only harness to eight cases: five echo requests
    plus the identical large completion request through OpenAI, Anthropic, and
    Meta. The OpenAI result retains its explicit background-polling case name;
    Anthropic and Meta retain explicit long-completion case names.
  - The fake-curl operational boundary proves all eight calls use only the
    production origin, Default-tenant client secret, saved provider default
    model, required request budget, and final completion marker.
  - The required baseline and final `make ci` checks passed. The paid run
    returned `200` for the OpenAI, Anthropic, and Meta echoes; Gemini echo
    returned `502`, Moonshot echo returned `429`, OpenAI long completion
    returned `504`, and Anthropic and Meta long completions returned `502`.
    The harness completed all eight cases, redacted their bodies, and correctly
    returned nonzero. The historical echo failures are resolved by this issue;
    the long-completion failures remain tracked in B088.
- [x] [B087] (P1) Restore Default-tenant Gemini and Moonshot production routing.
  Goal:
  Restore successful text generation for the Default tenant's saved Gemini and
  Moonshot provider routes without weakening the production live-test contract.
  Evidence:
  - The current eight-case `make live-test` production run returned `200` for
    OpenAI, Anthropic, and Meta echo cases using the same Default-tenant client
    secret. Gemini echo returned safe HTTP `502`; Moonshot echo returned safe
    HTTP `429`. The independent long-completion failures are tracked in B088.
  - The I036 disposable managed-key run authenticated the supplied Kimi
    credential against Moonshot's model catalog (`200`), where the configured
    former default was absent. The same credential verified and completed its
    smoke request with cataloged `kimi-k2.6` (`200`/`200`), isolating the
    credential from the existing default-route repair.
  - An authenticated catalog recheck on 2026-07-28 again returned `200`,
    confirmed the former default remains absent, and confirmed `kimi-k2.6` is
    present. The checked-in catalog now removes the obsolete model and promotes
    `kimi-k2.6`. The disposable managed-key harness then verified that new
    default and completed its smoke request (`200`/`200`); the production
    Default-tenant saved route remains unverified.
  Requirements:
  - Diagnose the exact Default-tenant Gemini `502` and Moonshot `429` at the
    public proxy/provider boundary without exposing secrets, prompts, response
    bodies, or client credentials.
  - Restore the affected provider routes through their canonical saved tenant
    credentials and provider configuration; do not add a local provider-key
    path, fallback provider, retry loop, or test-only bypass.
  - Preserve `make live-test` as an honest production boundary: it must retain
    all five providers, the short marker requests, the OpenAI polling case, and
    the Anthropic and Meta long-completion cases.
  Validation:
  - Run `make live-test` with the Default-tenant client secret and prove the
    Gemini and Moonshot echo cases return HTTP `200` with their required
    completion markers while retaining the complete eight-case matrix.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair for any code change.
  Resolved 2026-08-04:
  - The paid production `make live-test` run returned HTTP `200` with the
    required echo marker for both the Default-tenant Gemini and Moonshot
    routes. It retained the complete eight-case matrix without printing
    response bodies or credentials.
  - The independent B088 cases remain open: Anthropic long completion passed,
    while OpenAI returned HTTP `200` without the required completion marker and
    Meta returned HTTP `504`.
- [x] [B085] (P1) {B080} Complete truncated provider output through one shared coordinator.
  Goal:
  Make every configured text provider recover from output-budget truncation
  inside the caller's existing blocking request, using one provider-neutral
  completion lifecycle instead of recording a recoverable partial result as an
  upstream failure.
  Evidence:
  - The production Meta Muse Spark path returns HTTP `200` with
    `finish_reason=length` when the caller selects a 256-token completion
    budget, while the same prompt completes with `finish_reason=stop` at 1200
    tokens. B080 currently converts the recoverable first response into a
    public `502` and a failed managed usage event.
  - The configured provider catalog contains 12 text providers implemented by
    four transports: OpenAI Responses; shared OpenAI-compatible Chat
    Completions for Meta, DeepSeek, DashScope, Qwen Cloud, Moonshot, MiniMax,
    SiliconFlow, Zhipu, and Grok; Gemini `generateContent`; and Anthropic
    Messages.
  - Each transport reports output-budget exhaustion explicitly:
    OpenAI `status=incomplete` with `reason=max_output_tokens`, Chat
    `finish_reason=length`, Gemini `finishReason=MAX_TOKENS`, and Anthropic
    `stop_reason=max_tokens`.
  - The current adapters classify those four signals independently as terminal
    errors. OpenAI pending-response polling is also embedded in its adapter, so
    there is no shared owner for completion state, continuation accounting, or
    the request deadline.
  Requirements:
  - Introduce one provider-neutral completion coordinator used by every
    configured text provider. It must keep the original public request open and
    continue provider work until the normalized state is complete, the request
    context ends, or a non-recoverable provider state occurs.
  - Normalize only exact output-budget exhaustion as recoverable:
    OpenAI `incomplete/max_output_tokens`, Chat `length`, Gemini `MAX_TOKENS`,
    and Anthropic `max_tokens`. Safety/content filtering, refusals, tool calls,
    context-window exhaustion, missing or unknown states, failed/cancelled
    work, malformed responses, and provider HTTP failures remain canonical
    upstream failures.
  - Use one continuation transcript contract for every transport: retain the
    original messages, append any accumulated assistant output, and request
    only the missing suffix. Provider adapters may translate that canonical
    transcript to their wire format but must not own separate retry loops or
    provider-name-specific continuation policy.
  - Treat public `max_tokens` as the initial per-attempt output budget for this
    completion lifecycle, not permission to return a truncated answer. Reuse
    it for suffix-producing attempts. When an incomplete attempt produces no
    visible progress, increase the next attempt budget generically, bounded by
    the configured model output limit when one is known and by integer safety;
    the overall request timeout remains the hard lifecycle bound.
  - Keep upstream worker admission and configured origin rate limits on every
    provider operation. Waiting between explicit incomplete observations must
    not occupy a worker.
  - Aggregate token usage across distinct continuation attempts while retaining
    cumulative-snapshot replacement for repeated observations of one OpenAI
    response id. A recovered lifecycle produces one successful managed usage
    event and no failure row. A request that exhausts its overall deadline
    produces one canonical `504 request_timeout` event with usage accumulated
    before the deadline.
  - Never expose an intermediate partial response, raw provider body, provider
    error, prompt, response, or credential through the public error or managed
    failure-detail contract.
  - Supersede B080's terminal-incomplete and no-hidden-continuation decision in
    README, the canonical OpenAPI contract, the generated API reference, and
    provider-routing documentation. Keep the client-facing operation blocking;
    do not add a client polling endpoint, durable job queue, compatibility
    path, or provider-specific public option.
  Validation:
  - Exercise every canonical text provider selector through the public
    `POST /v2` boundary with an output-budget truncation followed by a complete
    suffix, and prove each returns one complete HTTP `200` result.
  - Prove the four transport families use the same coordinator contract,
    preserve ordered/system/user/assistant messages, concatenate suffixes
    without returning an intermediate response, and aggregate exact usage.
  - Reproduce the Meta no-visible-output case and prove the generic progress
    budget increases before the next attempt without exceeding a configured
    model limit.
  - Prove repeated incomplete observations continue until an explicit complete
    signal, while deadline expiry returns the canonical `504` and a safety,
    refusal, tool, context-window, missing, or unknown state still returns
    `502` without another continuation request.
  - Prove recovered managed requests add only one successful usage event and do
    not appear in account-wide or tenant failure details.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - Resolved 2026-07-27: one provider-neutral coordinator now continues exact
    output-budget signals across all 12 configured providers and all four
    transports until canonical completion or the public request deadline.
    Adapters only normalize native provider state and translate the shared
    continuation transcript.
  - A recovered lifecycle records one success with usage aggregated across
    distinct attempts; repeated OpenAI snapshots for one response id replace
    prior snapshots. Deadline expiry records one canonical `504` with usage
    accumulated before the deadline. Non-recoverable states remain `502`
    failures and never expose partial text.
  - The required baseline and post-change `make ci` runs passed. Public
    black-box coverage exercises every configured provider, repeated and
    zero-progress continuations, transcript and suffix assembly, exact usage
    accounting, deadline expiry, and representative non-recoverable states.
- [x] [B084] (P1) {I029} Restore the generated API reference after OpenAPI contract merges.
  Goal:
  Keep the committed human-readable API reference derived from the exact
  canonical OpenAPI source after forward-only branch merges.
  Evidence:
  - Merge commit `39869d3` combined OpenAPI changes from both parents into
    `docs/openapi.yaml` but retained `site/docs/index.html` from a parent.
  - The canonical contract SHA-256 is
    `796cff4216584bde8fb94cdadee195a0e715d3590a43f407c0f7ba60708b5c78`,
    while the committed reference records
    `5a6683d01dc04a10d6e045df3c3c265cd6b66aa43d9f39518ec9ecfa47c39b88`.
  - The required baseline `make ci` passes Go and Python static checks, then
    fails frontend lint with `openapi_docs_out_of_date`.
  Requirements:
  - Regenerate `site/docs/index.html` from the current `docs/openapi.yaml`
    through the canonical generator.
  - Do not change the canonical contract, generator, runtime behavior, or add
    another schema or documentation source.
  Validation:
  - Verify the reference records the exact canonical source digest.
  - Run the required final
    `timeout -k 350s -s SIGKILL 350s make ci` after the last code edit.
  Resolved 2026-07-27:
  - Regenerated `site/docs/index.html` from the unchanged canonical
    `docs/openapi.yaml`; its three provenance fields now record the exact
    `796cff4216584bde8fb94cdadee195a0e715d3590a43f407c0f7ba60708b5c78`
    source digest.
  - The full final `make ci` passes after the generated-document check, exact
    100% Go coverage, Python, rendered-browser, TAuth black-box, release, and
    live-provider harness gates.
- [x] [B083] (P1) Keep tracked environment examples out of runtime use.
  Goal:
  Preserve sample environment files as deliberately unrealistic documentation
  while requiring real runtime values only from ignored private dotenv files.
  Evidence:
  - `configs/.env.local.example` and `configs/.env.sample` now identify
    themselves as documentation-only and contain non-operational values.
  - The prior local startup contract copied the tracked local example into
    `configs/.env.local`, which allowed documentation to become runtime
    configuration and contradicted the private-env boundary.
  - The runtime and orchestration tests now use an explicitly created
    `configs/.env.local`, but README still documents the obsolete copy behavior.
  Requirements:
  - Never source, copy, or infer runtime configuration from a tracked sample.
  - Require the operator to create the ignored real `configs/.env.local`
    explicitly with private values and mode `0600`.
  - Fail before Docker startup when the private file is absent.
  - Keep `configs/.env`, `configs/.env.local`, and generated service-scoped
    dotenv files ignored and excluded from container build context.
  Validation:
  - Exercise the missing-private-env failure and the complete local
    orchestration path through the public `make up` boundary.
  - Verify both tracked sample files retain the documentation-only banner and
    deliberately unrealistic values.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair, with the final run after the
    last tracked edit.
  Resolved 2026-07-26:
  - Both tracked examples now remain visibly documentation-only with
    deliberately unrealistic values and cannot seed local runtime state.
  - `make up` requires the ignored real `configs/.env.local` before checking
    Docker, enforces mode `0600`, and creates only ignored service projections.
  - Operational coverage verifies the sample-file boundary, missing-file
    failure, generated local secrets, scoped projections, and complete
    orchestration flow.
- [x] [B082] (P1) {F014} Restore persisted-routing and dictation-size enforcement.
  Goal:
  Make management startup reject catalog-invalid persisted state and make the
  public dictation endpoint enforce its published upload limit.
  Evidence:
  - Schema-version-2 startup verifies the usage outcome column and index, then
    returns without validating persisted tenant routing defaults against the
    active provider/model catalogs.
  - F014 preflight resolves legacy provider aliases and models but persists the
    original values, so a noncanonical provider row can survive migration under
    an identifier the runtime does not use.
  - `/dictate` maps `http.MaxBytesReader` failures to `400` and does not reject
    an audio part that exceeds `server.max_input_audio_bytes` while the complete
    multipart body remains inside its separate overhead allowance, despite the
    canonical OpenAPI `413` response.
  Requirements:
  - Validate every current-schema tenant routing default at startup and fail
    with owner, tenant, endpoint, provider, and model context; never repair or
    infer persisted routing state.
  - Reject noncanonical legacy provider and text-model values before the F014
    migration transaction; persist only exact canonical provider/model
    identifiers.
  - Return `413 Payload Too Large` when either the audio part exceeds
    `server.max_input_audio_bytes` or the bounded multipart reader overflows,
    and do not call an upstream transcription provider.
  Validation:
  - Exercise restart through the real router construction boundary with an
    invalid current-schema tenant, legacy preflight with alias/case variants,
    and `/dictate` with both audio-part and total-body overflow.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair, with the final run after the
    last tracked edit.
  Resolved 2026-07-26:
  - Current-schema startup now rejects catalog-invalid routing rows, legacy
    preflight rejects noncanonical provider/model values, and `/dictate`
    returns `413` for either audio-part or bounded-body overflow.
  - Router-boundary, migration, and public dictation scenarios cover the
    corrected contracts.
- [x] [B081] (P1) {F014,B079} Keep managed routing defaults on providers with saved tenant keys.
  Goal:
  Ensure every managed routing default is immediately usable by the owning
  tenant instead of retaining a catalog default for a provider whose API key is
  absent.
  Requirements:
  - Treat a saved tenant provider key as a hard eligibility boundary for
    managed routing defaults. Preserve an existing text or dictation default
    only while its exact provider remains keyed and supports that endpoint.
  - When a provider-key save or removal invalidates a default, reconcile it
    atomically to a deterministic eligible keyed provider. Use the saved
    provider text model for an automatically selected text route and the
    catalog dictation default model for an automatically selected dictation
    route.
  - Represent an endpoint with no eligible keyed provider as one canonical
    unset provider/model pair. In particular, disable managed dictation when
    none of the tenant's keyed providers supports dictation; never retain an
    unkeyed or unsupported dictation provider as a placeholder.
  - Restrict the Settings routing selectors to keyed eligible providers and
    show the unset dictation state explicitly. Do not special-case any provider
    name, infer credentials from global configuration, or add a read-time
    fallback.
  - Migrate existing managed tenants once into the keyed-default contract and
    reject noncanonical persisted state after migration.
  - Keep static-configuration tenant defaults unchanged; this contract applies
    only to managed tenants and their tenant-owned provider keys.
  Validation:
  - Exercise the real management API and public proxy with arbitrary keyed
    providers, including a sole text-only provider, multiple eligible
    providers, removal of the active default, and restart after migration.
  - Prove Settings exposes only keyed routing candidates, disables dictation
    with no eligible keyed provider, and re-enables it from the complete
    profile returned by a successful provider-key mutation.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair, with the final run after the
    last tracked edit.
  Resolved 2026-07-26:
  - Managed defaults now remain on deterministic keyed providers, reconcile
    atomically with provider-key mutations, and persist one canonical unset
    pair when no eligible provider exists.
  - Settings exposes only keyed routing candidates and disables dictation when
    no keyed provider supports it; provider-agnostic API/runtime, migration,
    restart, and rendered-browser coverage verifies the contract.
- [x] [B080] (P1) Reject incomplete OpenAI responses that contain partial text.
  Goal:
  Make every successful text-provider result complete so callers never receive
  a provider-truncated prefix or intermediate result as an HTTP 200 response,
  while keeping asynchronous job handling explicit to each supported adapter.
  Evidence:
  - Fruits of the Quill Story Plan generation failed in production at
    `2026-07-27 00:14:16 UTC` after its strict JSON decoder reported
    `unexpected EOF`.
  - The matching LLM Proxy usage row recorded `response_tokens=2048`,
    `total_tokens=3647`, `status_code=200`, and `success=1`. The caller's exact
    2048-token output budget was exhausted, but LLM Proxy classified the
    incomplete result as successful.
  - `resolveIncompleteOpenAIResponse` currently returns
    `responseSnapshot.generation()` whenever an incomplete response contains
    nonblank text, bypassing the existing incomplete-response error path.
  - Multiple production rows reached the same exact 2048-token cap, so this is
    a repeatable transport-contract defect rather than an isolated malformed
    model response.
  - The shared OpenAI-compatible Chat Completions adapter does not inspect
    `finish_reason`, so `length` and other non-complete choices can currently
    return partial text for DeepSeek, DashScope, Qwen Cloud, Moonshot, MiniMax,
    SiliconFlow, Zhipu, Meta, and Grok.
  - The Anthropic Messages adapter does not inspect `stop_reason`, so
    `max_tokens`, `pause_turn`, and other non-complete results can currently
    return text. Gemini already requires `finishReason=STOP` but discards
    reported usage when rejecting another reason.
  - Only the OpenAI Responses adapter uses a pollable lifecycle. The shared
    `server.queue_size` facility is HTTP-operation admission control, not a
    durable provider-job-id queue, and the configured synchronous provider
    routes do not imply each provider's separate deferred or batch API.
  - This issue is the upstream owner for the dependent Story Service correction
    tracked as `story-generator` B007.
  Requirements:
  - Define HTTP success for the OpenAI Responses adapter as a provider response
    whose terminal status is complete. An upstream `status=incomplete` must
    never return partial text, an HTTP 2xx response, or a successful usage
    event merely because text is nonblank.
  - Poll an OpenAI response id only from the adapter's explicit background
    lifecycle and documented `queued` or `in_progress` states. Reject missing
    and unknown states instead of treating every non-terminal value as pending.
  - Treat usage reported across observations of one OpenAI response id as
    cumulative snapshots: retain the newest nonempty snapshot instead of
    summing it with earlier observations. Sum usage only across a genuinely
    distinct synthesis response id.
  - Define complete success for the shared Chat Completions adapter as
    `finish_reason=stop`, Gemini as `finishReason=STOP`, and Anthropic Messages
    as `stop_reason=end_turn` or `stop_sequence`. Reject missing, truncated,
    tool/intermediate, refused, and unknown reasons without returning their
    text.
  - Do not invent generic polling from an arbitrary `id`. A provider-specific
    deferred, batch, or asynchronous API requires its own explicit transport
    and lifecycle contract.
  - Map every incomplete terminal response, including
    `incomplete_details.reason=max_output_tokens`, to the canonical upstream
    failure response and status. Do not expose the provider body or partial
    generated text.
  - Remove hidden continuation or synthesis of incomplete output. The caller's
    explicit `max_tokens` value remains the upper bound for that request and
    must not trigger an undisclosed additional paid generation.
  - Record the request as unsuccessful with the canonical upstream failure
    outcome while retaining safe normalized metadata and available token usage;
    never persist prompts, responses, or raw provider errors.
  - Update the canonical OpenAPI and provider-routing documentation in the same
    change so HTTP 200 continues to mean a complete response at every public
    text endpoint.
  - Include B080 in the next immutable release prepared by B077. Production
    activation remains operator-owned.
  Validation:
  - Exercise the real public `POST /v2` handler against a fake OpenAI endpoint
    that returns HTTP 200, `status=incomplete`, nonempty partial output,
    `reason=max_output_tokens`, and exact token usage.
  - Prove the proxy returns the canonical non-2xx upstream failure, emits no
    partial generated text, and records a failed normalized usage event with
    the provider-reported token counts.
  - Prove a terminal completed OpenAI response still returns its exact text and
    successful usage record.
  - Exercise the shared Chat Completions, Gemini, and Anthropic adapters through
    public `POST /v2` with partial/intermediate stop signals. Prove each returns
    `502`, exposes no partial text or token headers, and retains exact
    provider-reported token counts in failed managed usage.
  - Prove unknown OpenAI states with ids do not trigger polling and that
    documented `queued` and `in_progress` states still poll to completion.
  - Prove repeated usage snapshots for one polled OpenAI response id retain the
    latest exact counts on both success and managed failure instead of
    double-counting earlier snapshots.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - OpenAI Responses now accepts only exact `status=completed` as success,
    polls only explicit `queued` or `in_progress` work, and rejects incomplete,
    missing, unknown, failed, or cancelled states without leaking partial text
    or starting a hidden continuation.
  - The shared Chat Completions adapter requires exact
    `finish_reason=stop`, Gemini requires exact `finishReason=STOP`, and
    Anthropic requires exact `stop_reason=end_turn` or `stop_sequence`.
    Truncated, tool/intermediate, refused, missing, and unknown reasons return
    the canonical upstream failure.
  - Polling remains owned by the OpenAI Responses adapter and the active client
    request. `server.queue_size` remains HTTP-operation admission control; no
    generic or durable provider-job queue was invented for synchronous routes.
  - Failed managed usage retains available provider-reported token counts
    without exposing token headers, prompts, responses, or raw provider bodies.
    Repeated observations of one OpenAI response id replace its cumulative usage
    snapshot; only a separately created synthesis response is additive.
  - Public `POST /v2` coverage proves the exact OpenAI
    `1599/2048/3647` incomplete case and representative Chat, Gemini, and
    Anthropic partial cases return `502`; completed responses and documented
    OpenAI pending-state polling remain successful.
  - The README, canonical OpenAPI, generated API reference, and provider-routing
    documentation describe the same completion and async-ownership contract.
  - The managed-routing public proxy fixture now returns the required
    `finish_reason=stop` completed Chat Completions response, and the full final
    `make ci` passes.
- [x] [B079] (P1) {B074,B076,F014} Consolidate tenant and client-key lifecycle into one Settings row.
  Goal:
  Make the selected Settings tenant and its client key one compact, logical
  control surface without exposing a standalone key-deletion state.
  Problem:
  Settings currently repeats the selected tenant across a selector row and a
  separate identity row, then separates the same tenant's client key into a
  third row. Rename expands inline, key replacement happens without an
  explicit invalidation confirmation, and the client key has a revoke action
  even though the intended lifecycle is replacement or deletion of the owning
  tenant.
  Requirements:
  - Replace the Settings-tenant selector, tenant identity row, and client-key
    row with one semantic compact row containing the selected tenant dropdown,
    Rename, key state and one-time reveal/copy controls, confirmed Replace key,
    confirmed Delete tenant, and Create tenant.
  - Treat the selected dropdown value as the current Settings editor context;
    do not add another active-tenant state, activation flag, URL parameter,
    immutable-id banner, or compatibility selection path. Preserve the
    independent Usage tenant filter and simultaneous operability of every
    tenant.
  - Move rename into a keyboard- and focus-managed modal with canonical
    validation and conflict feedback. Preserve the unsaved-Settings discard
    decision when switching tenants.
  - Require confirmation before replacing an existing client key. State that
    the prior key stops working immediately, then preserve the one-time masked
    reveal, Show, Copy, close guard, stale-response isolation, and retryable
    Create key state after an automatic creation failure.
  - Remove standalone client-key revocation from the browser, management
    router, OpenAPI contract, persistence surface, documentation, and tests.
    Provider API-key removal remains a separate provider-settings contract.
  - Keep final-tenant deletion disabled with an accessible explanation and
    retain the named deletion confirmation, complete tenant cleanup, Usage
    filter reset, and deterministic next-tenant selection.
  - Keep the row on one line at desktop width and use one bounded responsive
    wrap on narrow screens. Preserve semantic custom elements, centralized
    copy, visible focus, keyboard operation, and unclipped modal geometry.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair, with the final run after the
    last tracked edit.
  Validation:
  - Exercise selection independence, modal rename success/conflict/cancel,
    confirmed key replacement and cancellation, one-time key reveal/copy,
    missing-key retry, final-tenant protection, confirmed tenant deletion, and
    stale-response isolation through the rendered Playwright interface.
  - Prove the real management router and canonical OpenAPI inventory no longer
    expose `DELETE /api/management/tenants/{tenant_id}/secrets`.
  - Prove the combined row remains contained and ordered across desktop,
    compact, and mobile viewports.
  Resolved 2026-07-26:
  - Replaced the repeated tenant selector, identity, and key rows with one
    semantic `Tenant access` row. Desktop stays on one line; narrow screens
    preserve the same control order in one bounded two-line wrap.
  - Moved rename and client-key replacement into focus-managed dialogs,
    centralized nested Escape handling, retained one-time reveal/copy and
    missing-key retry behavior, and kept final-tenant deletion protected.
  - Removed standalone client-key revocation from the browser, management
    router/store, canonical OpenAPI inventory, generated API reference,
    current documentation/resources, mocks, and tests. Replacement now proves
    immediate invalidation of the prior key; tenant deletion remains the only
    other client-key removal path.
  - Extended rendered Playwright coverage for rename validation/conflict,
    replacement confirmation/cancellation/pending state, selection
    independence, lifecycle cleanup, and desktop/compact/mobile geometry.
  - Refined the final tenant-lifecycle action to a large icon-only plus beside
    the trash action at every viewport while retaining `Create tenant` as its
    accessible name and tooltip.
  - Standardized the generated client-key Copy control on the Material Symbols
    `content_copy` glyph and removed its custom inline SVG while retaining the
    existing accessible clipboard behavior.
- [x] [B078] (P1) {F014,B075} Fail visibly when the management application runtime is blocked.
  Goal:
  Make local browser startup terminate in either the management application or
  an actionable error instead of leaving the MPR authentication transition
  visible forever.
  Problem:
  The local ghttp frontend publishes no cache policy and every application
  module has an unversioned URL. Chrome can therefore combine modules cached
  before B076 with current files from the mounted working tree. The observed
  page loaded current `keyManagement.js` with an older `backendClient.js` that
  did not export `fetchAccountUsageFailures`, so module linking failed before
  the application entrypoint evaluated. MPR UI authenticated independently,
  but LLM Proxy never mounted Alpine, requested the management account, or
  dispatched `llm-proxy:management-ready`; the user remained on
  `Opening LLM Proxy` indefinitely.
  Requirements:
  - Revise the complete first-party module graph once so browser copies cached
    before the local cache policy cannot mix with current files.
  - Serve the local browser surface with `Cache-Control: no-store` so an
    ordinary reload cannot combine stale and current ES modules while `make up`
    is mounted to the working tree. Revise the entry-module URL once to evict
    browser copies cached before that policy existed.
  - Keep the pinned Alpine 3.13.5 jsDelivr module as the single canonical
    dependency. Do not add a fallback CDN, compatibility loader, bundled copy,
    retry loop, or timeout.
  - Guard application-module linking and Alpine loading so a rejected runtime
    renders a semantic error surface with the exact allow-and-reload recovery
    action.
  - Complete the MPR UI transition after the failure surface is ready, without
    issuing a protected management request or reinterpreting the MPR UI
    authentication state.
  - Preserve the current authenticated, unauthenticated, and already-settled
    authentication lifecycle when Alpine loads.
  - Add black-box rendered-browser coverage for an incompatible cached
    first-party module and a rejected Alpine request. Prove both show the
    recovery state and emit the management-ready completion event.
  - Document that `make up` service readiness cannot override a browser-side
    block and identify the exact CDN origin that local Chrome must allow.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair, with the final run after the
    last tracked edit.
  Resolved 2026-07-26:
  The local frontend now disables browser caching and uses one revised URL
  across the complete first-party ES-module graph, preventing the observed
  current-`keyManagement.js`/stale-`backendClient.js` link failure. A separate
  startup guard owns application-link and pinned-Alpine failures, renders the
  semantic allow-and-reload surface, and completes the existing MPR UI
  transition without a protected management request. The 65-scenario rendered
  browser suite passes, including both failure boundaries, and clean reloads in
  real Chrome and the in-app browser reach the signed-out application with
  `data-llm-proxy-application="ready"` and no startup error.
- [x] [B076] (P1) {F014,I029,I031} Separate active tenants from Settings and Usage selection.
  Goal:
  Keep every owned tenant simultaneously operational while making Settings the
  sole tenant-management surface and making Usage Overview an account-wide
  report by default with an independent tenant filter.
  Problem:
  F014 introduced one global `active tenant` that simultaneously chooses the
  Settings profile, Usage Overview scope, URL workspace, and tenant lifecycle
  target. The toolbar therefore implies that only one tenant is active, exposes
  tenant management outside Settings, and makes the default usage report show
  only the oldest tenant. The product contract is instead that every tenant's
  secret remains active independently; UI selection chooses only what the user
  is editing or reporting.
  Requirements:
  - Delete the global active-tenant toolbar, `Active tenant` copy, immutable-id
    banner, create action, and URL/history-owned workspace contract. Do not add
    an activation flag, status, server-side selected tenant, compatibility
    query, or fallback selection path. Every tenant remains independently
    routable through its own generated secret.
  - Put tenant selection and `Create tenant` inside Settings with the existing
    rename, guarded delete, client key, provider settings, defaults, and request
    examples. The Settings tenant is an editor context only. Switching it must
    preserve the unsaved-edit decision, clear one-time/revealed credentials,
    and never change Usage Overview's filter.
  - Add one accessible Usage tenant selector immediately left of the ordered
    `ALL`, `30 days`, `7 days`, and `1 day` interval controls. Its first option
    is `All tenants`, it defaults to that option on every authenticated
    workspace load, and the interval independently continues to default to
    `30 days`. Refresh and interval changes retain the Usage tenant selection.
  - Add canonical owner-only `GET /api/management/usage` and
    `GET /api/management/usage/failures` operations for all owned tenants.
    Preserve the existing tenant-scoped operations for an explicitly selected
    tenant; these are distinct canonical scopes, not aliases or fallback
    reads. Require the same exact interval and failure pagination query
    contracts and return `Cache-Control: no-store`.
  - Compute owner-wide usage in the database/store boundary from every usage
    row whose tenant belongs to the authenticated owner, using one captured
    server timestamp. Preserve exact bucket, provider, model, status, token,
    success, and request aggregation; calculate average latency from the
    complete event set rather than averaging tenant averages. Never fetch each
    tenant from the browser and combine partial responses.
  - Make the existing failed-request action follow the Usage selector.
    Owner-wide pages use one stable newest-first snapshot/cursor across all
    owned tenants and add only the owning tenant's safe id and display name to
    each row. Tenant-scoped pages retain their current safe shape. Bind cursors
    to their exact owner-wide or tenant scope so they cannot cross scopes.
  - Keep Settings and Usage request identity/cancellation independent. A late
    profile, usage, failure, create, rename, or delete response cannot overwrite
    another Settings tenant or Usage scope. Deleting the tenant currently used
    by the Usage filter resets that filter to `All tenants` and refreshes the
    owner-wide snapshot; other deletions retain the current filter.
  - Preserve the account/tenant persistence model, owner isolation, final
    tenant guard, admin aggregate-only boundary, mandatory setup behavior,
    credential secrecy, semantic components, centralized copy, focus handling,
    and unclipped desktop/mobile layout.
  - Update the canonical OpenAPI source, README, implementation documentation,
    generator-owned public usage resource, and unresolved F014-dependent issue
    wording to distinguish `Settings tenant`, `Usage tenant`, and `All tenants`
    from operational tenant activity. Do not hand-edit a generated artifact.
  Validation:
  - Add real management-router and OpenAPI scenarios proving exact owner-wide
    totals/buckets/breakdowns, weighted latency, all-time bounds, empty usage,
    strict queries, no-store headers, owner isolation, distinct tenant scope,
    stable scope-bound failure pagination, safe tenant context, and absence of
    credentials, prompts, responses, raw errors, or other owners' data.
  - Add Playwright scenarios proving the missing global toolbar, Settings-only
    lifecycle controls, default `All tenants` plus `30 days`, selector placement
    immediately before `ALL`, scope-preserving Refresh/interval behavior,
    independent Settings and Usage selections, creation/deletion behavior,
    stale-response rejection, failure drill-down scope, keyboard/screen-reader
    semantics, and desktop/mobile geometry.
  - Extend the real TAuth black-box flow to prove the default owner-wide
    dashboard includes two independently routable tenants while Settings can
    manage either without changing the report.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair, with the final run after the
    last code edit.
  Resolved 2026-07-26:
  The global active-tenant toolbar and URL/history workspace state are removed.
  Tenant selection and creation now live in Settings as an editor-only context,
  while Usage Overview independently defaults to `All tenants` and `30 days`
  with its tenant selector immediately before the interval controls. Every
  tenant secret remains independently routable.
  Canonical owner-wide usage and failure operations now aggregate every owned
  tenant at the database boundary under one captured time/snapshot contract.
  Tenant-scoped operations remain distinct; all-tenant failure rows add only
  safe tenant attribution, and opaque cursors cannot cross scopes. OpenAPI,
  README, implementation guidance, generated public resources, and dependent
  backlog wording now describe the same forward-only contract.
  The required pre-change and post-change `make ci` runs pass. Exact-coverage Go
  scenarios, 63 Playwright scenarios, and the real TAuth black-box prove
  aggregation, isolation, simultaneous routing, independent Settings and Usage
  selection, stale-response rejection, failure pagination, accessibility, and
  desktop/mobile layout.
- [x] [B075] (P1) {F014} Keep local browser authentication on the ghttp front door.
  Goal:
  Make the canonical `make up` browser login path reach the TAuth container
  regardless of unrelated host processes.
  Problem:
  The local runtime config publishes `http://localhost:8082` while Compose
  binds TAuth only on IPv4 `127.0.0.1:8082`. An unrelated process can own the
  IPv6 `localhost` listener on that port. The browser then reaches that process,
  receives no TAuth CORS headers, and cannot restore a session or request a
  nonce even though `make up` reports ready because readiness checks IPv4
  directly.
  Requirements:
  - Keep production's explicit split-origin contract unchanged.
  - Publish the ghttp `http://localhost:4179` front door as the local TAuth URL
    and proxy `/auth/*` plus `/me` to TAuth on the Compose network.
  - Remove the host TAuth port from the canonical local topology so it cannot
    conflict with unrelated IPv4 or IPv6 listeners.
  - Verify static, config, API, same-origin session, nonce, and management API
    boundaries through the exact `localhost` origins used by the browser.
  - Keep aggregate local environment files out of containers and retain
    service-scoped secret ownership.
  - Prove the operational contract, the real Compose stack, and the browser
    login bootstrap, then pass the required final `make ci`.
  Resolved 2026-07-26:
  Local browser authentication now stays on the ghttp front door. Compose owns
  the canonical `/auth/*` and `/me` proxy mappings plus the browser-facing
  `tauthUrl`, so stale ignored local environment entries cannot restore the
  obsolete direct-port topology. TAuth remains internal to the Compose network
  and production's split-origin configuration is unchanged.
  `make up` now verifies the exact `localhost` URLs used by the browser,
  including `GET /auth/session` and `POST /auth/nonce`. The real stack reached
  ready with session `204`, nonce `200`, anonymous management `401`, and no
  host TAuth port. Reloading the real local UI and activating **Sign in**
  produced a same-origin nonce request in TAuth without the prior CORS or
  `:8082` browser requests.
  The required pre-change and post-change `make ci` runs pass. The final run
  follows the last tracked edit and passes static analysis, exact 100% Go
  coverage, 33 Python tests, package installation, 63 browser scenarios, the
  TAuth black-box test, 47 release tests, and live-provider preflight.
- [x] [B074] (P1) {F014} Make tenant context and lifecycle controls compact.
  Goal:
  Make multi-tenant context immediately legible without presenting the active
  tenant as a permanently open edit form.
  Problem:
  The active-tenant toolbar stacks its label above the selector and separates
  the immutable id into a wide second column. In Settings, the tenant name is
  always rendered as a full-width input with save/delete actions and two help
  lines. This gives stable tenant identity the visual weight of a pending form,
  pushes the client key away from its owning context, and makes the final-tenant
  guard read like an error during ordinary use.
  Requirements:
  - Keep the canonical F014 account, URL, isolation, and tenant lifecycle
    behavior unchanged; this is one frontend presentation and interaction fix.
  - Render the global active-tenant label, selector, immutable id, and create
    action as one dense MPR toolbar on desktop with a bounded two-row mobile
    layout.
  - Render Settings tenant identity as a compact idle row with the display name
    primary and immutable id secondary. Enter an inline name editor only after
    an explicit Rename action, focus it deterministically, and return focus
    after save or cancel.
  - Keep deletion adjacent to the tenant it affects. Use a compact destructive
    action, retain the named confirmation, and present the undeletable final
    tenant as concise protected state with an accessible explanation.
  - Keep the client-key row separate and preserve one-time reveal, copy,
    replacement, revocation, mandatory-setup, and credential-clearing behavior.
  - Preserve centralized copy, semantic custom elements, visible focus,
    keyboard operation, screen-reader naming, and unclipped desktop/mobile
    geometry.
  - Extend Playwright coverage through the real rendered interface and pass the
    required final `make ci` after the last code edit.
  Resolved 2026-07-26:
  The global tenant selector, immutable id, and create action now share one
  dense context toolbar. Settings renders the active tenant as a compact
  name/id row with an `Only tenant` protection chip and colocated destructive
  action; the name input appears only after Rename and returns focus after save
  or cancel.
  The client-key contract remains a separate row with unchanged one-time
  reveal, copy, replace, revoke, and mandatory-setup behavior. Its narrow
  layout now gives key status the full row before placing the create/replace
  action below it, avoiding the prior squeezed vertical copy.
  Browser coverage proves lifecycle behavior, stale-response isolation, focus,
  final-tenant protection, and bounded desktop/compact/mobile geometry. The
  required pre-change and post-change `make ci` runs pass. The final run
  followed the last code edit and passed static analysis, exact 100% Go
  coverage, 33 Python tests, package installation, 63 browser scenarios, the
  TAuth black-box test, 47 release tests, and live-provider preflight.
- [x] [B073] (P1) {B070,F014,I031} Migrate persisted caller-cancellation usage outcomes.
  Goal:
  Let existing management databases containing valid caller-cancellation
  usage rows start and migrate to the canonical outcome schema.
  Problem:
  Runtime usage recording already persists caller cancellation as status `499`
  with outcome `request_timeout`, but the bounded historical mapper accepts
  only status `504` for that outcome. A persisted schema-1 or pre-F014 `499`
  row therefore fails migration preflight and prevents router startup.
  Requirements:
  - Map historical status `499` to `request_timeout` through the existing
    canonical caller-cancellation status constant.
  - Exercise persisted `499` rows through both the schema-1-to-2 and pre-F014
    SQLite startup migration paths while retaining rejection of unknown
    historical statuses.
  - Document that caller cancellation `499` and proxy-budget expiry `504`
    share the normalized `request_timeout` outcome.
  - Pass the required post-edit `make ci`.
  Resolved 2026-07-26:
  The historical outcome mapper now uses the canonical caller-cancellation
  status constant to map persisted `499` rows to `request_timeout`, matching
  current runtime recording and presentation. Unknown historical statuses
  remain rejected before mutation.
  The schema-1-to-2 and pre-F014 disposable SQLite startup migrations both
  contain caller-cancellation rows and prove their status and normalized
  outcome survive the bounded upgrade. README and implementation runbooks now
  state that `499` caller cancellation and `504` proxy-budget expiry share the
  canonical outcome without conflating their HTTP meanings.
  The required pre-change and post-change `make ci` runs pass. The final run
  followed the last code edit and passed static analysis, exact 100% Go
  coverage, 33 Python tests, package installation, 63 browser scenarios, the
  TAuth black-box test, 47 release tests, and live-provider preflight.
- [x] [B072] (P1) {F014,B071} Preserve legacy SQLite index names through the ownership migration.
  Goal:
  Let `make up` start against an existing pre-F014 SQLite volume without
  deleting tenant, provider-key, or usage data.
  Problem:
  The deployed one-workspace schema contains
  `idx_managed_tenant_records_secret_digest` and
  `idx_managed_usage_created_at`. SQLite keeps those global index names when
  GORM renames the legacy tables, so creating the current tables fails with
  `index ... already exists`. The disposable migration fixture omitted the
  historical index tags and therefore did not reproduce the real volume.
  Requirements:
  - Make the disposable legacy SQLite fixture reproduce the exact historical
    tenant and usage indexes from the pre-F014 GORM models.
  - Inside the existing all-or-nothing migration transaction, rename only the
    two colliding legacy indexes through the GORM Migrator before renaming
    tables and creating the current schema. Do not add raw SQL or delete data.
  - Prove the current schema receives its canonical indexes and every injected
    migration failure restores the legacy tables and original index names.
  - Update the migration runbook, pass the required final `make ci`, then run
    `make up` against the preserved named volume and verify every readiness
    boundary.
  Resolved 2026-07-26:
  The ownership migration now renames the two colliding historical indexes
  through the GORM Migrator inside its existing transaction before it renames
  the legacy tables. No runtime raw SQL, alternate database, compatibility
  path, or data deletion was added.
  The disposable legacy fixture now recreates the exact pre-F014 GORM indexes.
  Migration coverage proves the current tables receive their canonical index
  names and every injected failure restores the original legacy table and
  index shape.
  The pre-change `make ci` baseline reached two existing local-orchestration
  timing failures after the immediately preceding full gate had passed. The
  required post-change `make ci` passed static analysis, exact 100% Go
  coverage, 33 Python tests, package installation, 63 browser scenarios, the
  TAuth black-box test, 47 release tests, and live-provider preflight.
  `make up` then migrated the preserved named SQLite volume and passed static,
  runtime-config, API, TAuth-session, and management-session readiness. A
  normal interrupt removed the containers and network while preserving
  `llm-proxy-local_llm_proxy_local_data`.
- [x] [B071] (P1) {F014,I029,I031} Restore the SQLite-only GORM management database contract.
  Goal:
  Select the management database by its SQLite location and keep all runtime
  persistence behind GORM model APIs, without a PostgreSQL or raw-SQL path.
  Requirements:
  - Replace `management.database_dialect` and `management.database_dsn` with
    one required `management.database_path` field and the matching
    `LLM_PROXY_MANAGEMENT_DATABASE_PATH` placeholder.
  - Open the configured SQLite location with the pure-Go GORM dialector. Keep
    injected GORM dialectors only as test boundaries; do not expose another
    runtime database selector.
  - Delete the PostgreSQL driver, raw sequence SQL, disposable PostgreSQL
    harness, workflow service, and database-specific tests.
  - Update current repository, operator, generated public, and issue
    documentation to the forward-only SQLite contract without aliases or
    compatibility reads.
  - Preserve the real SQLite migration and management API coverage, then pass
    the required post-edit `make ci`.
  Resolved 2026-07-26:
  Replaced the dialect/DSN pair with the required `management.database_path`
  and `LLM_PROXY_MANAGEMENT_DATABASE_PATH` contract. Runtime persistence now
  opens that SQLite location through GORM, while injected GORM dialectors
  remain test-only boundaries.
  Removed the PostgreSQL driver and transitive dependencies, raw identity
  sequence SQL, PostgreSQL workflow service and environment, disposable
  PostgreSQL harness, target, and database-specific tests. Updated the current
  configuration examples, local and deployment environment files, repository
  guidance, migration runbooks, issue records, and generated public resources
  to the SQLite-only contract without an alias or fallback.
  The required pre-change `make ci` established a failing exact-coverage
  baseline at two `management_usage.go` branches. After the final code edit,
  `make ci` passed static analysis, exact 100% Go coverage, 33 Python tests,
  package installation, 63 browser scenarios, the TAuth black-box test, 47
  release tests, and live-provider preflight.
- [x] [B070] (P1) {F014,I029,I031} Correct review-discovered management presentation and OpenAPI gaps.
  Goal:
  Keep canonical failure presentation and OpenAPI conformance aligned with the
  released multi-tenant management contract.
  Requirements:
  - Present the persisted caller-cancellation status `499` through the same
    strict status-label and tenant-scoped failure-detail paths as every other
    canonical failure.
  - Document and validate the existing empty `api_key` request that retains a
    stored credential while updating its provider model or system prompt.
  - Enforce declared integer `maximum` values in the canonical OpenAPI
    conformance validator.
  - Cover each correction through the real-router, OpenAPI, and Playwright
    integration boundaries, then pass the required post-edit `make ci`.
  Resolved 2026-07-26:
  Persisted caller cancellation now renders as canonical status `499 Client
  closed request` in both usage breakdowns and tenant-scoped failure details.
  The real management router proves cancellation persists only the normalized
  `request_timeout` outcome, while Playwright proves the complete response
  remains usable and secret-safe.
  The canonical OpenAPI request describes empty `api_key` as the existing
  retain-credential operation, its generated reference is synchronized, and a
  real provider-settings request passes both schema and server validation.
  Integer `maximum` values are enforced, with the documented failure-page
  limit rejecting `101` in both the conformance gate and real handler.
  The required pre-change and post-change `make ci` runs pass. The final run
  followed the last code edit and passed exact 100% Go coverage, 33 Python
  tests, 63 management-browser tests, TAuth black-box coverage, release
  checks, and live-provider preflight.
- [x] [B069] (P1) Make upstream request timeouts an explicit, bounded client-to-proxy contract.
  Goal:
  Let each client choose the exact bounded amount of time LLM Proxy may spend
  on one upstream request, while keeping caller cancellation, provider work,
  and gateway protection under distinct owners.
  Problem:
  A production-sized F001 `POST /v2` request reached the caller's independent
  300-second HTTP deadline before LLM Proxy returned either a result or its own
  360-second timeout. The owned gateway was staged with 420-second outer
  deadlines. The Go CLI and Python package separately use 390 seconds, but that
  number has no product, protocol, provider, or deployment significance.
  The defect is not that one of these constants is too small. The defect is
  that the caller, proxy, provider adapters, and gateway each own an implicit
  clock, so a client cannot ask LLM Proxy for the work budget its request needs
  and cannot know which layer ended the request.
  Consequence:
  Kamu F001 cannot safely retry its Bulgarian source-world review. The failed
  one-shot request produced no response or durable completion receipt, and the
  available aggregate usage count cannot establish whether provider work later
  completed or the proxy eventually reached its deadline.
  Evidence:
  - A small `gpt-5.5` request with explicit `reasoning_effort: "high"` returned
    `200` in about 2.6 seconds, so the released field, route, credentials, and
    basic service path work.
  - The approximately 16 KB `neblagodarnost` source-world request ended after
    300 seconds with `Client.Timeout exceeded while awaiting headers`; no
    source-world artifact was promoted and the caller did not retry.
  - The current runtime uses `server.request_timeout_seconds: 360`; staged
    gateway `response_header_timeout` and `read_timeout` values are 420
    seconds. The Go CLI, README example, and Python client introduce a separate
    390-second convention, while the reusable Go package requires every caller
    to invent its own positive HTTP timeout.
  - LLM Proxy currently constructs a global timeout for provider clients,
    creates another deadline in the text handler, and lets dictation rely on
    provider-local deadlines. There is no single request-ingress deadline
    shared consistently by every upstream operation.
  Contract:
  Define one optional request header for every public operation that can start
  upstream work:
  ```text
  X-LLM-Proxy-Request-Timeout-Seconds: <positive whole number>
  ```
  It applies to `GET /`, `POST /`, `POST /v2`, and `POST /dictate`; management
  and static-site operations are out of scope.
  The header is the maximum wall-clock budget LLM Proxy may spend on the
  authenticated request. It begins before request-body parsing and includes
  validation, admission/queue wait, the provider call, OpenAI background
  polling, and response construction. It is a ceiling, not a promise that the
  operation will run for that long: validation, queue saturation, provider
  failure, or explicit caller cancellation may finish it earlier.
  - If the header is omitted, use `server.request_timeout_seconds` as the
    effective budget.
  - Add `server.max_request_timeout_seconds` as the operator-owned capacity
    limit. Accept a requested value exactly when it is within the inclusive
    range `1..max`; never round, clamp, replace, or silently fall back.
  - Reject a blank, repeated, signed, fractional, nonnumeric, zero, negative,
    or over-limit value with `400` before queue admission or any provider call.
    Return `application/json` with the exact safe envelope
    `{"error":{"code":"invalid_request_timeout","max_request_timeout_seconds":M}}`.
  - Return the effective value on every accepted response, including errors,
    in `X-LLM-Proxy-Request-Timeout-Seconds`.
  - When the accepted budget expires, cancel queued/provider/polling work and
    return `504 application/json` with the exact safe envelope
    `{"error":{"code":"request_timeout","request_timeout_seconds":N}}`, where
    `N` is the effective timeout. Do not report it as a provider failure.
  - Create the deadline once at authenticated request ingress and propagate
    that context unchanged. Provider adapters must not start a fresh timeout or
    extend the remaining budget.
  Caller cancellation is a separate concern. A Go context, process signal, or
  explicitly configured transport policy may cancel a request sooner, in which
  case no response is guaranteed. Bundled clients must not hide such a policy
  behind the server-budget setting or impose an unrelated total-response
  deadline by default.
  The owned gateway is the final outer guard. Its response-header and read
  deadlines must be strictly greater than
  `server.max_request_timeout_seconds`, with the relationship validated from
  deployment configuration. The gateway must not use a client-specific magic
  number.
  Requirements:
  1. Validate the server default and maximum at startup: both effective values
     are positive and the default does not exceed the maximum. An explicitly
     invalid value fails startup rather than being reset.
  2. Enforce the header and one ingress-owned context identically across all
     four upstream public operations. Preserve the one-shot REST lifecycle:
     no async receipt, retry, prompt chunking, provider-specific timeout field,
     tenant mutation, direct provider call, or product-only exception.
  3. Move timeout selection into each Go and Python messages request and
     serialize it as the canonical header. Replace the CLI's ambiguous
     `--timeout` with `--request-timeout-seconds`. Remove the 390-second
     constants and config-level total-response timeout, and replace the
     390-second README example. Continue to honor the Go caller's context and
     injected transports as separate cancellation mechanisms.
  4. Record the effective timeout and terminal outcome in safe structured
     request evidence without prompts, audio, credentials, provider response
     bodies, or free-form error text. Production correlation must distinguish
     success, proxy deadline, provider failure, and caller cancellation.
  5. Update the owning documentation in the same change:
     - `README.md` defines the header, default/max behavior, client examples,
       status codes, and the distinction between work budget and cancellation.
     - `CHANGELOG.md` records the externally visible header, errors, client API
       change, and removal of the arbitrary supported-client deadline.
     - `configs/config.yml` declares the current operator-selected default and
       maximum. The accepted request budget is at most the proxy maximum, which
       remains strictly below the owned gateway's outer guard; caller
       cancellation is independent of that ordering.
     - This repository currently has no `PRD.md` or `ARCHITECTURE.md`. Do not
       create partial timeout-only placeholders. M013 owns the product-context
       document decision and must carry this final behavior into any canonical
       documents it introduces. I029 will subsequently freeze the wire
       contract in canonical OpenAPI.
  Deployment dependency:
  The current gateway schema-v1 `caddy_route` declaration cannot express
  transport timeout values and rejects unknown fields. Do not invent an
  llm-proxy-only manifest field or extend that obsolete shape. Production
  activation requires a companion `mprlab-gateway` change or the forward-only
  I204 `caddy_fragment` migration to make the edge guard greater than the
  configured proxy maximum, plus non-deploying validation of the assembled
  Caddy configuration. Production deployment itself remains user-owned.
  Validation:
  - Add black-box tests proving omission uses the server default; a shorter
    accepted value times out and cancels work; a value longer than the default
    can succeed after the default would have expired; and malformed, repeated,
    or over-limit values return `400` without an upstream request.
  - Exercise `GET /`, both JSON POST routes, and `/dictate`; prove queue time and
    OpenAI background polling consume the same non-resetting budget.
  - Exercise the Go package, Python package, and CLI against a real test server;
    prove each sends the requested header, omits it when not requested, receives
    the server's effective response header, and has no hidden 390-second
    deadline.
  - Validate startup invariants and reject a deployment whose gateway outer
    deadline cannot outwait the configured server maximum.
  - Re-run a production-comparable approximately 16 KB `gpt-5.5`,
    explicit-`high` request with a deliberately selected budget above the
    deployed default and within the deployed maximum. Correlate it through a
    final body or the canonical proxy `504`; it must not end at an opaque
    bundled-client `awaiting headers` timeout. Retain the small explicit-`high`
    `200` control.
  - Run the required baseline and final `make ci` pair, with the final run after
    the last code edit. Verify the released artifact and deployed timeout
    relationship before F001 retries the request.
  Implemented 2026-07-24:
  LLM Proxy now accepts the canonical request-scoped header on all four
  upstream routes, validates the configured default and maximum, starts one
  cause-preserving deadline before body parsing, echoes the effective budget,
  and returns exact safe `400` and `504` envelopes. Queue wait, provider work,
  dictation, and OpenAI background polling share that deadline without adapter
  resets. Safe terminal evidence distinguishes validation failure, success,
  proxy timeout, proxy overload, provider failure, and caller cancellation.
  The Go package, Go CLI, and Python package now serialize the budget per
  request and impose no hidden total-response deadline. Go integrations move
  `ConfigInput.Timeout` to
  `MessagesRequestInput.RequestTimeoutSeconds`; Python client `0.2.0` moves
  `ClientConfig.timeout_seconds` to
  `ClientMessagesRequest.request_timeout_seconds`; the CLI uses only
  `--request-timeout-seconds`. README, changelog, implementation plans,
  generated public client guides, and upgrade commands document the migration.
  The app deployment preflight reads its tracked maximum and supplies it to the
  companion gateway verifier. The gateway parses only its own Caddy
  configuration, rejects outer guards that are not strictly greater, and sets
  the response-header, upstream-read, and client-write guards to 3660 seconds
  for the current 3600-second service capacity. The connection-idle policy
  remains independent. Focused Go coverage is 100%, Python checks pass, the
  complete gateway test suite and pinned-container Caddy validation pass, and
  no production deployment command was run.
  The required pre-change and post-change `make ci` runs pass. The final run
  followed the last code edit and passed exact 100% Go coverage, 34 Python
  tests, 51 management-browser tests, the black-box authentication test,
  release-contract checks, and the live-provider harness preflight.
  Review follow-up 2026-07-24:
  Response bodies are now fully constructed and checked against the request
  context before success is selected. Managed-usage persistence runs only
  after the selected response is written and flushed; its store-lock
  acquisition and GORM operations use the same request context. Cancellation
  can therefore leave terminal log evidence without a managed-usage row, which
  is documented in README, but persistence can no longer outwait the accepted
  request budget or change the selected response.
  Explicit YAML `null` and empty timeout values now fail startup instead of
  selecting defaults, while true omission still selects the compiled default.
  Queue saturation records `proxy_overload` rather than
  `provider_failure`. The canonical Python package metadata reports client
  version `0.2.0`, with a package-metadata regression test. The required
  review-follow-up baseline and final `make ci` runs pass; the final run follows
  the last tracked edit.
  Packaging contract cleanup 2026-07-24:
  `python/pyproject.toml` is the sole Python distribution definition. The
  root-level package-install check stages that project in a temporary directory,
  installs it as a normal package, and verifies both its public import surface
  and installed version against the canonical metadata. Setuptools and coverage
  outputs remain generated, ignored local artifacts rather than tracked sources.
  Gateway review correction 2026-07-24:
  The final `deploy-llm-proxy-backend` invocation now receives the same
  app-owned maximum that was read once from the tracked configuration and
  supplied to the early verifier. The gateway target itself requires that
  value, so direct or aggregate gateway deployment cannot bypass the
  request-capacity contract. The public deployment fixture and final
  post-edit `make ci` pass without production contact.
  Flag-free gateway correction 2026-07-24:
  The app deploy script no longer reads or forwards the capacity. Gateway now
  discovers the canonical LLM Proxy checkout, verifies the exact committed
  reader and tracked config, and derives the maximum inside its sealed
  ready-fleet plan. The app invokes the verifier and deployment target without
  `LLM_PROXY_MAX_REQUEST_TIMEOUT_SECONDS`; focused operational coverage rejects
  any reintroduction of that environment handoff.
  Resolved 2026-07-25:
  - The coordinated app and gateway changes were released and deployed by the operator; released client `v0.2.46` exposes the request-level budget without a hidden total-response deadline.
  - A live `gpt-5.6-terra` / `max` control returned `OK` with a 900-second request budget.
  - A production-sized Bulgarian source-world request using the same model, effort, and budget remained connected beyond the former 300-second caller deadline and the 360-second proxy default, then returned a complete passing review after roughly 6.5 minutes.
  - Kamu F001 can therefore resume with the declared 900-second budget; no retry, direct-provider path, prompt chunking, or tenant mutation is required.


- [x] [B099] (P0) Retire the exact legacy llm-proxy Compose service.
  Goal:
  Make the first schema-v2 deployment remove the obsolete llm-proxy container
  from the shared legacy Compose project without deleting its retained data
  volume or affecting another application.
  Evidence:
  - The current production service is still identified as
    `mprlab-nginx-gateway/llm-proxy`.
  - The schema-v2 manifest owns the replacement runtime and retains the
    existing `mprlab-nginx-gateway_llm-proxy-data` volume, but did not declare
    the old service that the gateway must retire.
  Requirements:
  - Declare exactly the legacy project `mprlab-nginx-gateway` and service
    `llm-proxy` on the selected runtime resource.
  - Keep the existing data volume retained.
  - Prove the declaration structurally and document the bounded first-deploy
    transition.
  Validation:
  - Run the required final
    `timeout -k 350s -s SIGKILL 350s make ci`.
  Resolved 2026-08-10:
  - Pull request 243 merged commit
    `44303b3d3d7cdfc7f2c045ede0ef114d87799417`.
  - The current schema-v3 manifest retains the exact retired service and data
    volume. The lifecycle integration test enforces both declarations.

- [x] [B077] (P1) Publish and activate the merged LLM Proxy contract.
  Goal:
  Make the production API and management UI run the same canonical contract
  that is merged and marked resolved in this repository, with immutable release
  provenance that can be verified before dependent provider work resumes.
  Evidence:
  - The production `umna-moma-i-tsar` Creative Director canary selected
    `gpt-5.6-terra`, reasoning effort `max`, and the explicit 900-second request
    timeout, then received `HTTP 504` with an empty response body at the
    source-world review boundary on 2026-07-26.
  - The current production gateway configuration and deployed Caddy global
    timeout guards are already 3660 seconds, so the earlier gateway-timeout
    diagnosis is disproved.
  - On 2026-07-26, both
    `ghcr.io/tyemirov/llm-proxy:v0.2.46` and
    `ghcr.io/tyemirov/llm-proxy:latest` resolve to
    `sha256:f2593f7a55e6e7bde5f37fe36edaa027fcee96700c48cdcb19c1e3e718b9009c`.
    That release predates the 2026-07-26 merged F014/I029/I031 management
    contract and its failure-details implementation.
  - The live tenant-scoped
    `GET /api/management/tenants/{tenant_id}/usage/failures` path returns a
    router-level `404 page not found`, while the merged canonical API defines
    that operation. The refreshed production management UI likewise lacks the
    merged failed-request inspection action.
  - The missing live failure-details operation prevents retrieval of the
    normalized failure record needed to distinguish an LLM Proxy timeout from
    an upstream provider or transport 504 without exposing prompts, responses,
    or raw provider errors.
  Requirements:
  - Prepare a clean immutable release from committed source that contains the
    resolved B069, F014, I029, and I031 contracts. Do not publish from the
    current dirty worktree or fold unresolved B076 work into the release.
  - Publish the versioned multi-platform container, move `latest` to that exact
    manifest, and publish the matching generated Pages artifact through the
    repository-owned release workflow. Record the release tag, source commit,
    manifest digest, and Pages version as one provenance tuple.
  - Have the production operator activate that exact release without replacing
    or reinitializing the existing management SQLite volume. Apply only the
    forward migrations owned by the released binary.
  - Verify the backend and Pages surfaces independently. A successful image
    pull or container recreation is not proof that the API route and browser UI
    are current.
  - Do not work around the failure by inflating another timeout, bypassing the
    public LLM Proxy API, querying private provider state, or retrying the paid
    F001 canary before release activation is proven.
  Deliverables:
  - A versioned release and matching `latest` image containing the merged
    request-timeout, tenant-management, OpenAPI, and normalized failure-details
    contracts.
  - A matching published management UI and a production activation receipt
    containing the immutable source/tag/digest tuple.
  - Read-only post-deploy evidence for the canonical health, timeout-header,
    tenant failure-details, and management UI boundaries.
  Validation:
  - Prove the version tag and `latest` resolve to the same newly published
    manifest and that the manifest was built from the recorded source commit.
  - Prove an unauthenticated request to the tenant failure-details operation
    reaches the authentication boundary rather than returning router-level 404,
    then prove an authenticated owner can read the safe normalized failure page.
  - Prove the production UI exposes the failed-request action and reads the same
    canonical operation without credentials, prompts, responses, transcripts,
    or raw provider errors entering the DOM.
  - Re-run one Creative Director source-world canary with Terra/max and the
    explicit 900-second request timeout. Use its normalized usage/failure record
    to classify any subsequent provider failure before resuming the full F001
    batch.
  Verified 2026-07-27:
  - Release `v0.2.47` points to release commit
    `943a9b5b582534c11526a5242c145a2d234f6f09`; its immutable source commit is
    `63763e70c20db0dad311e95d654aa67a6f076e13`.
  - `ghcr.io/tyemirov/llm-proxy:v0.2.47` and `:latest` both resolve through the
    standard Docker client to
    `sha256:986fb7cb1a3dc50d49d53678121452e28acb503458e70d107f45f98f3dfa4121`.
  - The public Pages marker reports `release_version=v0.2.47` and source
    `63763e70c20db0dad311e95d654aa67a6f076e13`. The deployed UI assets contain
    the failed-request action and account/tenant failure clients.
  - Unauthenticated requests to both canonical failure-details operations now
    return `401`, proving the live backend reaches the authentication boundary
    instead of the obsolete router-level `404`.
  - The dominant active Meta failure path is not evidence of stale B077
    activation.
    One repository-owned live smoke without an explicit output ceiling
    completed successfully. A controlled `muse-spark-1.1` request with the
    caller's explicit 256-token ceiling returned provider HTTP `200`,
    `finish_reason=length`, and no visible answer; the same request at 1200
    tokens returned `finish_reason=stop` with visible text. The public
    production proxy reproduced the same split: 256 returned safe `502`
    `chat completion finish_reason=length`, while 1200 returned exact success.
    B080 correctly maps the incomplete 256-token result without leaking partial
    output. The current Gix commit-message path owns that 256-token ceiling and
    fails over to its lower-priority OpenAI connection.
  Blocked: direct inspection of the running production container image still
  requires the gateway operator's sudo authority, and the required post-release
  Terra/max 900-second Creative Director canary has not been rerun. Keep B077
  blocked until the operator records the running container's image ID and
  matching repo digest, then completes the single normalized Terra canary.
  Resolved 2026-08-10:
  - The issue's verified v0.2.47 release, Pages marker, UI assets, and API
    authentication boundaries satisfy its historical activation contract.
  - B126 now owns current-release activation provenance. B088 owns the
    unresolved OpenAI and Meta long-completion routes after I045.

### Complete entries archived 2026-08-30

- [x] [B162] (P1) Preserve Gemini terminal error codes during background polling.
  Goal:
  A failed Gemini background interaction must retain safe provider failure
  evidence before resource cleanup removes the interaction.
  Evidence:
  - Expected: I045 identifies the Gemini lifecycle operation, terminal state,
    and sanitized provider error code for each failed request.
  - Actual: The proxy returns HTTP `502` with `provider_error`,
    `retryable: false`, `upstream_status: null`, and no terminal error code.
  - Production release `v6.1.0` uses application commit
    `975053c675dcb1437e22c611433a8f0b46a42834`.
  - The expected Default tenant passed `/v2/identity` before each production
    run sent paid traffic.
  - Eight production live cases passed, including `gemini-echo`.
  - `gemini-background-polling` failed with request ID
    `KEQJO2TXJLI4QJ3HYGPNWJO227` and a 157-byte response.
  - The exact scoped retry failed with request ID
    `OEPWJEUOLKNNK45VUDDFEYOQT2` and the same 157-byte response.
  - The second failure occurred after the identity check returned HTTP `200`.
  - The response proves that no unsuccessful provider HTTP status reached the
    public provider error mapper.
  - The response does not distinguish a terminal Interaction fault, a response
    protocol fault, or a transport fault without a response.
  - `geminiInteractionResponse` decodes `id`, `status`, `steps`, and `usage`.
    It does not decode the provider `errors` array.
  - The current Google Interaction resource defines each `errors[].code` as a
    URI. It defines `message` as human-readable text.
  - Google defines `failed` as a terminal status that can include a tool fault
    or a rate limit.
  - `geminiInteractionSnapshot.resolve` converts every `failed` status to bare
    `ErrProviderAPI` and discards the provider error code.
  - `providerHTTPMetadata` cannot classify bare `ErrProviderAPI` as an upstream
    HTTP response or a retryable provider condition.
  - The Gemini observation callback records usage and cleanup mode only. It
    does not record the observed state or provider error code in I045.
  - The terminal failure test supplies no `errors` array. It asserts only the
    generic public provider error.
  - The continuation event records only `failure`. It cannot identify the
    failed Gemini lifecycle operation.
  - The adapter deletes the provider resource on exit. The discarded provider
    error code cannot be recovered after cleanup.
  - The current provider contracts are documented at
    `https://ai.google.dev/api/interactions-api-v1` and
    `https://ai.google.dev/gemini-api/docs/background-execution`.
  Requirements:
  - Decode the Gemini Interaction `errors` array once at the provider response
    boundary.
  - Validate each provider error code as a canonical URI at the response edge.
  - Represent the terminal status and safe error codes in one typed provider
    failure.
  - Keep `upstream_status` null when no unsuccessful provider HTTP response
    exists.
  - Do not invent an HTTP status from an Interaction error code.
  - Define an explicit retry classification for supported terminal error codes.
  - Fail closed for an unknown terminal error code.
  - Record the safe terminal code in structured provider failure telemetry.
  - Record Gemini create, observation, cancellation, and deletion outcomes in
    I045.
  - Distinguish an Interaction terminal fault from a cleanup fault.
  - Keep provider messages, raw bodies, interaction identifiers, prompts,
    responses, and credentials out of public responses and logs.
  - Preserve cancellation and deletion for every stored interaction.
  - Preserve the shared `pollable_resource` lifecycle and its visibility rule.
  - Do not add another provider attempt, fallback, or timeout increase.
  Deliverables:
  - Add a typed Gemini terminal error model at the response boundary.
  - Add safe Gemini lifecycle fields to the existing provider progress event.
  - Update the provider error contract and OpenAPI artifacts when public fields
    or retry classification change.
  - Update B128 production evidence after the corrected request identifies the
    terminal provider condition.
  Validation:
  - Return one controlled `failed` Interaction with a safe error code through
    the real public router.
  - Prove the public error and I045 events retain only the approved code.
  - Prove provider messages and interaction identifiers do not escape.
  - Prove retryable and non-retryable URI codes get different classifications.
  - Prove an unknown URI path segment fails closed without a fallback.
  - Prove terminal Interaction and cleanup faults produce different telemetry.
  - Prove cancellation and deletion still occur in the required order.
  - Rerun only `gemini-background-polling` with the expected Default tenant.
  - Correlate its request ID with Gemini progress and one terminal I045 summary.
  - Run `make ci` after the last application change.
  Resolution:
  - The Gemini response decoder validates each provider error code as a
    canonical HTTPS URI. It does not retain provider messages.
  - Each URI has a safe final `snake_case` path segment for classification.
  - One typed terminal failure owns the status and validated codes.
  - A failed Interaction is retryable only when every URI has a retryable final
    path segment. An empty list or an unknown segment fails closed.
  - `upstream_status` remains null without an unsuccessful provider HTTP
    response. The public provider error keeps its six-field shape.
  - I045 records Gemini create, poll, cancel, and delete operations. Terminal
    progress and provider failure logs include only validated error codes.
  - Real-router tests cover retryable, non-retryable, unknown, and malformed URI
    codes. They also cover terminal faults, cleanup faults, redaction, and cleanup.
  - The README, provider routing guide, OpenAPI contract, and generated API
    reference describe the current retry and telemetry contracts.
  - `make go-test` passed with 100.0% Go statement coverage after the URI fix.
  - `make ci` passed all 11 gates with 100.0% Go statement coverage after the
    URI fix.
  - This implementation run did not send production traffic. B128 owns the
    release, exact paid background case, and production I045 correlation.

- [x] [B161] (P2) Reject undeclared tenant identity input.
  Goal:
  The tenant identity handler must match its canonical OpenAPI request contract.
  Evidence:
  - Expected: `GET /v2/identity` accepts only the tenant client key and no body.
  - Actual: The handler returns HTTP `200` with an undeclared query field or a
    request body.
  Requirements:
  - Reject each undeclared query field with HTTP `400`.
  - Reject a request body with HTTP `400`.
  - Keep valid authenticated identity reads unchanged.
  - Validate the request once at the HTTP boundary.
  Validation:
  - Send requests through the real router.
  - Prove that valid input returns the authenticated tenant identifier.
  - Prove that an extra query field returns HTTP `400`.
  - Prove that a request body returns HTTP `400`.
  - Prove that rejected requests send no provider request.
  - Run `make ci` after the last application change.
  Resolution:
  - The authenticated handler accepts exactly one `key` query field and no
    request body. It returns HTTP `400` for all other request shapes.
  - The canonical OpenAPI contract and generated reference declare the HTTP
    `400` response.
  - Real-router tests cover valid identity reads, undeclared query input,
    request bodies, and the zero-provider-request boundary.
  - `make test` passed.
  - `make ci` passed all 11 gates with 100.0% Go statement coverage.

- [x] [B159] (P0) Restore LoopAware telemetry after the database reset.
  Goal:
  Public pages send telemetry to the current LLM Proxy site record.
  Evidence:
  - Expected: Each public page uses site `543d2796-d616-4080-99e7-0720ae438440`.
  - Actual: Generated pages use a site record that the reset removed.
  Requirements:
  - Define the production pixel URL in the shared public shell module.
  - Generate each legal page and resource page from that URL.
  - Reject a public page that does not use the exact production URL.
  Validation:
  - Run the public-page browser test.
  - Run `make ci` after the last repository change.
  Resolution:
  - The shared public shell exports the production LoopAware pixel URL.
  - The landing page and each generator use the current site record.
  - The all-pages browser test rejects a missing or incorrect pixel URL.
  - The final `make ci` passed all 11 gates in 149 seconds.
  - Go statement coverage remained at 100.0 percent.

- [x] [B158] (P2) Preserve the Gemini candidate reasoning matrix default.
  Goal:
  The Gemini candidate command tests all supported thinking levels by default.
  Evidence:
  - Expected: `test_live_providers.sh --gemini-candidates` uses the candidate
    script default.
  - Actual: The parent script supplies `false` and skips the supported thinking
    levels.
  Requirements:
  - Use `true` as the candidate mode default.
  - Preserve an explicit `false` value from the operator.
  - Keep the registered provider mode default unchanged.
  Validation:
  - Prove that candidate mode sends all supported thinking levels without an
    environment override.
  - Prove that an explicit `false` value reaches the candidate script unchanged.
  - Run `make ci` after the last repository change.
  Resolution:
  - Gemini candidate mode now uses `true` as its reasoning matrix default.
  - An explicit `false` value remains authoritative.
  - Registered provider mode keeps its `false` default.
  - The operational test proves both candidate behaviors through the public
    parent script.
  - `make go-test` passed with 100.0 percent Go statement coverage.
  - The final `make ci` passed all 11 gates in 138 seconds with 100.0 percent
    Go statement coverage.

- [x] [B157] (P1) Apply pending model replacements before route validation.
  Goal:
  A schema-version-9 database reaches the current schema with a retired Gemini
  selection.
  Evidence:
  - Expected: Startup applies the schema-version-10 and schema-version-11 model
    replacements before current route validation.
  - Actual: The schema-version-10 migration validates the retired Gemini 3.1
    selection and stops startup.
  Requirements:
  - Apply all pending model replacements in one database transaction.
  - Validate the final routing state after all replacements.
  - Record each pending schema version only after successful validation.
  - Roll back all replacements and version records after a migration error.
  - Preserve provider connections, profiles, tenant defaults, timestamps, and
    historical usage records.
  Validation:
  - Prove the schema-version-9 to schema-version-11 migration for each retired
    Gemini 3.1 selection.
  - Prove transaction rollback after a pending migration error.
  - Run `make ci` after the last repository change.
  Resolution:
  - Startup applies all pending model replacements in one transaction.
  - Startup validates routing after the final replacement and then records each
    schema version.
  - The schema-version-9 test covers each retired Gemini 3.1 selection and a
    missing schema-version-11 policy.
  - `make go-test` passed with 100.0 percent Go statement coverage.
  - The final `make ci` passed all 11 gates in 140 seconds with 100.0 percent
    Go statement coverage.

- [x] [B156] (P0) Mount the provider catalog in local orchestration.
  Goal:
  The local API starts with the canonical provider catalog.
  Evidence:
  - Expected: `providers.yml` exists beside the selected `config.yml` in the
    API container.
  - Actual: `make up` mounts only `config.yml`, and the API exits with
    `provider_catalog_read_failed`.
  Requirements:
  - Mount `configs/providers.yml` beside the selected API configuration.
  - Keep both runtime assets read-only.
  - Require the exact local runtime asset pair in the operational test.
  Validation:
  - Prove the local API starts with the current provider catalog.
  - Run `make ci` after the last repository change.
  Resolution:
  - The local API mounts `config.yml` and `providers.yml` as read-only runtime
    assets in the same directory.
  - The operational test requires both exact local runtime asset mounts.
  - `make go-test` passed with 100.0 percent Go statement coverage.
  - The final `make ci` passed all 11 gates in 134 seconds with 100.0 percent
    Go statement coverage.
  - `make up` passed all local readiness checks and stopped cleanly.

- [x] [B155] (P1) {I231} Migrate retired Gemini selections before provider validation.
  Goal:
  Preserve each stored Gemini configuration during the schema-version-10
  migration.
  Evidence:
  - Expected: Startup replaces each stored Gemini 2.5 selection before
    current-catalog validation.
  - Actual: Startup validates a schema-version-8 selection during the
    schema-version-9 migration and rejects the retired model.
  Requirements:
  - Use the schema-version-10 catalog records to replace each predecessor
    model.
  - Replace the selected model in each affected provider profile and
    same-provider tenant default.
  - Preserve provider credentials, system prompts, timestamps, and historical
    usage records.
  - Reject retired Gemini selections after the schema-version-10 migration.
  Validation:
  - Prove startup migration from schema version 8 for each retired Gemini
    model.
  - Prove preservation of credentials, prompts, timestamps, and historical
    usage records.
  - Run `make ci` after the last repository change.
  Resolution:
  - Predecessor migrations use the schema-version-10 catalog records before
    current-catalog validation.
  - Schema-version-8 startup preserves Gemini credentials, prompts,
    timestamps, and historical usage records while it reaches the current
    schema.
  - Schema-version-8 startup tests cover each retired Gemini model and the
    transaction failure paths.
  - The final `make ci` passed all 11 gates with 100.0 percent Go statement
    coverage.

- [x] [B154] (P1) Move model migration policy into the provider catalog.
  Goal:
  Keep each persisted model identifier in the provider catalog.
  Evidence:
  - Expected: `configs/providers.yml` owns current, upstream, and retired model
    identifiers.
  - Actual: `management_store.go` defines retired and upstream model
    identifiers for database migrations.
  Requirements:
  - Add a strict model migration record to the provider catalog schema.
  - Declare the schema version, provider, operation, source model, and target
    model in each record.
  - Permit an empty target only when the record retires an absent provider.
  - Make each managed database migration read the validated catalog records.
  - Remove each model identifier literal and model constant from
    `management_store.go`.
  - Preserve atomic selection updates and unchanged historical usage records.
  - Update the provider catalog documentation.
  Validation:
  - Prove that a changed catalog record changes the database migration result.
  - Prove rejection of invalid and duplicate model migration records.
  - Prove that managed store code contains no model identifier.
  - Run `make ci` after the last repository change.
  Resolution:
  - The provider catalog owns seven strict database model migration records.
  - Catalog validation rejects invalid versions, operations, sources, targets,
    references, and duplicate records.
  - Managed database migrations use the immutable registry projection and keep
    historical usage unchanged.
  - `management_store.go` contains no model identifier or model constant.
  - Focused migration and operational tests passed. `make go-test` passed with
    100.0 percent Go statement coverage.

- [x] [B153] (P1) Reject unsupported Gemini assistant replay.
  Goal:
  Keep unsupported assistant history out of Gemini Interactions requests.
  Evidence:
  - Expected: Gemini requests contain only input that the public proxy contract
    can represent completely.
  - Actual: The proxy sends assistant messages as `model_output` steps without
    provider state or thought signatures.
  - An output-limit response also starts this unsupported replay automatically.
  Requirements:
  - Reject each assistant message for a Gemini Interactions route before an
    upstream request.
  - Declare no replay continuation for the Gemini Interactions transport.
  - Return a provider error after an incomplete Gemini response.
  - Delete the incomplete interaction without a second create request.
  - Keep replay for each transport that declares continuation rules.
  Validation:
  - Prove assistant-history rejection through the public request route.
  - Prove incomplete-response cleanup through the public request route.
  - Run `make ci` after the last application change.
  Resolution:
  - Both public JSON message routes reject Gemini assistant history before an
    upstream request.
  - The Gemini transport declares no continuation actions. An incomplete
    interaction returns a provider error after cleanup and does not start a
    second create request.
  - The generic completion coordinator continues only when the selected
    transport declares continuation actions.
  - The public routing tests and `make go-test` passed with 100.0 percent Go
    statement and block coverage.

- [x] [B152] (P1) Reconcile Gemini interaction visibility to a readable state.
  Goal:
  Keep a created Gemini interaction under the proxy lifecycle until Google
  exposes its stored resource.
  Evidence:
  - Expected: A stored background create becomes readable before request
    execution treats an observation error as terminal.
  - Actual: Google returned `403 permission_denied`, transient
    `400 invalid_request` responses, and then `200 completed` after 20 seconds
    for its documented Gemini 3.6 Flash background example.
  - The shared lifecycle stopped at the first transient `400` after two
    seconds, so the proxy returned HTTP 502 and logged cleanup failure.
  Requirements:
  - Keep post-create visibility reconciliation in the shared pollable-resource
    lifecycle.
  - Read retry statuses, interval, and limit from each pollable provider
    transport in `providers.yml`.
  - Configure Gemini for the observed `400`, `403`, and `404` sequence.
  - Configure OpenAI with its existing first-read visibility contract.
  - Release upstream capacity during every wait.
  - Preserve caller cancellation and request-budget authority.
  Validation:
  - Prove the complete transient sequence through public request routing.
  - Prove exhaustion returns the last provider error.
  - Run `make ci` after the last application change.
  Resolution:
  - The shared lifecycle reads a bounded visibility policy from the selected
    provider transport without provider-specific retry control flow.
  - OpenAI declares one retry after two seconds for `403` or `404`. Gemini
    declares six retries at five-second intervals for `400`, `403`, or `404`.
  - Public routing proves transient visibility recovery, exhaustion, caller
    cancellation, cancel, and delete behavior.
  - The intended key passed verification and smoke tests for all six Gemini
    3.x candidates. Four models remain registered.
  - `make ci` passed all 11 gates with 100.0 percent Go statement coverage.

- [x] [B151] (P1) Reject duplicate live-test dotenv names.
  Goal:
  Make each live provider test use one explicit credential value.
  Evidence:
  - Expected: A dotenv name identifies one value for the live provider test.
  - Actual: The loader silently kept the first of two `GEMINI_API_KEY`
    entries, so it tested a retired credential instead of the replacement.
  Requirements:
  - Reject a duplicate name before provider discovery or an upstream request.
  - Identify the duplicate name and line without exposing either value.
  - Keep explicit process environment values authoritative over dotenv values.
  Validation:
  - Prove duplicate rejection through the live harness entry point.
  - Run the applicable repository validation after the last change.
  Resolution:
  - The live harness rejects the second occurrence of a dotenv name before
    provider discovery.
  - The error identifies only the duplicate name and line number.
  - The compiled operational test proves rejection without exposing either
    credential value.
  - `make go-test` passed with 100.0 percent Go statement coverage.

- [x] [B150] (P2) Record terminal visibility errors as failures.
  Goal:
  OpenAI poll progress events agree with terminal provider failure summaries.
  Evidence:
  - Expected: Only a visibility error that starts reconciliation records
    `pending`.
  - Actual: Reconciliation and later poll `403` or `404` errors also record
    `pending`.
  Requirements:
  - Make the shared lifecycle identify only the visibility error that starts
    reconciliation.
  - Record all terminal OpenAI visibility errors as `failure`.
  - Keep the B149 observation and provider error contracts.
  Validation:
  - Make sure that a repeated visibility error records `pending` and then
    `failure` through the public request route.
  - Run `make ci` after the last application change.
  Resolution:
  - The shared lifecycle now identifies the first visibility error that starts
    reconciliation.
  - OpenAI records that retried observation as `pending`.
  - OpenAI records reconciliation and later poll visibility errors as
    `failure`.
  - Public request tests include terminal reconciliation `403` and later poll
    `404` responses.
  - `make go-test` passed with 100.0 percent Go statement coverage.
  - The final `make ci` passed all 11 gates in 175 seconds with 100.0 percent
    Go statement coverage.

- [x] [B149] (P1) Stabilize the first read of a pollable resource.
  Goal:
  Read each created provider resource after its visibility boundary.
  Evidence:
  - Expected: A successful create makes the same resource readable with the
    same credential.
  - Actual: The first read can return HTTP `403` before a later read returns
    HTTP `200` for the same resource and credential.
  - Two Gemini 3.5 Flash checks became readable after the initial observation
    interval and completed without a permission change.
  Requirements:
  - Own post-create observation in the shared `pollable_resource` lifecycle.
  - Apply the lifecycle to request execution and provider connection
    verification.
  - Keep provider adapters responsible for wire formats and provider status
    parsing.
  - Bound all observation work with the caller context.
  - Release the upstream worker during each observation interval.
  - Return the provider error when the resource does not cross the visibility
    boundary.
  - Preserve cleanup, safe errors, and secret redaction.
  Validation:
  - Prove delayed visibility through public request and management routes.
  - Prove each implemented pollable transport adapter uses the shared
    lifecycle.
  - Prove synchronous transports do not use the lifecycle.
  - Prove cancellation stops the observation phase.
  - Run `make ci` after the last application change.
  Resolution:
  - Added one provider-neutral lifecycle for observation of a created resource.
  - The lifecycle reads immediately. It reconciles one first-read HTTP `403`
    or `404` after a two-second, context-bound interval.
  - OpenAI Responses, Gemini Interactions, and pollable connection verification
    now use the shared lifecycle. Synchronous transports remain unchanged.
  - Public request tests cover delayed visibility through OpenAI and Gemini.
    Management tests cover transient and persistent retrieval failures.
  - Cancellation prevents a reconciliation read after the request context ends.
  - `make go-test` passed with 100.0 percent Go statement coverage.
  - The final `make ci` passed all 11 gates in 167 seconds with 100.0 percent
    Go statement coverage.
  - `make test-live-gemini` passed provider verification and text smoke checks
    for `gemini-3.5-flash` with the existing Authorization key.
  - Release, deployment, credential rotation, and production acceptance remain
    in B128.

- [x] [B148] (P0) Deploy the provider catalog with the service.
  Goal:
  The deployed service starts with its canonical provider catalog.
  Evidence:
  - Expected: `providers.yml` exists beside `/app/config.yml`.
  - Actual: Production generation 6 mounted only `/app/config.yml`.
  - Actual: The container restarted with `provider_catalog_read_failed`.
  Requirements:
  - Mount `configs/providers.yml` at `/app/providers.yml`.
  - Keep both runtime assets read-only.
  - Require the exact runtime asset pair in the public lifecycle test.
  Validation:
  - Use the current successful CI result as the initial result.
  - Run `make go-test`.
  - Run `make ci` after the last change.
  Resolution:
  - Added the canonical provider catalog as `/app/providers.yml`.
  - Kept the configuration and provider catalog assets read-only.
  - The public lifecycle test now requires the exact runtime asset pair.
  - `make go-test` passed with 100.0 percent Go statement coverage.
  - The final `make ci` passed all 11 gates in 130 seconds.
  - Did not run release, publication, or deployment.

- [x] [B146] (P1) Bound the complete hosted CI job.
  Goal:
  Hosted CI completes the current canonical gate on GitHub-hosted runners.
  Evidence:
  - Expected: Hosted CI completes all 11 gates and prints the success receipt.
  - Actual: Runs `32437201144` and `32448302006` reached the final gate.
    The shell watchdog then killed `make ci` at exactly 350 seconds.
  - The completed gates used 333 seconds in run `32448302006` attempt 3.
    The final gate then used 17 seconds to build the current proxy binary.
  Requirements:
  - Keep one execution limit in the hosted workflow.
  - Apply the execution limit to the complete GitHub Actions job.
  - Set the job execution limit to 10 minutes.
  - Keep the canonical `make ci` command and all 11 gates.
  - Keep the hosted Playwright package declaration.
  Validation:
  - Prove the exact hosted workflow contract through the public Make test.
  - Run `make ci` after the last workflow or test change.
  - Prove one hosted run completes all 11 gates and prints the success receipt.

  Resolution:
  - The workflow now applies one 10-minute limit to the complete hosted job.
  - The CI step invokes `make ci` without a second shell execution limit.
  - The focused Make contract test passed.
  - A local `make ci` passed all 11 gates in 169 seconds with 100.0%
    Go statement coverage.
  - Hosted run `32450200399` passed all 11 gates. The canonical gate used 350
    seconds, and the complete job used 6 minutes and 46 seconds.

- [x] [B147] (P0) Restore the permanent selected application manifest.
  Goal:
  The release command accepts the selected application manifest.
  Evidence:
  - Expected: The manifest contains only `owner`, `release`, and `resources`.
  - Actual: Commit `f349171` added `schema_version` on the `owner` line.
  - Actual: Commit `97215af` made the public lifecycle test invalid Go code.
  Requirements:
  - Restore the permanent manifest contract from I230.
  - Keep all resource shapes unchanged.
  - Restore the public lifecycle contract test for the permanent manifest.
  Validation:
  - Run the focused lifecycle contract test.
  - Run `make ci` after the last change.
  Resolution:
  - Restored the permanent manifest without `schema_version`.
  - Kept all resource shapes unchanged.
  - Restored the public lifecycle contract test and its exact root-key check.
  - The initial CI run stopped during Go static analysis on the invalid test.
  - `make go-test` passed with 100.0 percent Go statement coverage.
  - The final `make ci` passed all 11 gates in 138 seconds.
  - Did not run release, publication, or deployment.

- [x] [B145] (P1) Keep failed request usage on one model route.
  Goal:
  Usage reports show the model route that the request contract selects.
  Evidence:
  - Expected: An explicit provider with no model uses its saved model or its
    catalog default.
  - Actual: A failed Gemini request can use the tenant-wide OpenAI model in
    usage data.
  - The dashboard then shows an invalid pair such as
    `gemini / gpt-5.6-terra`.
  - A registered provider without saved settings can record an empty model.
  Requirements:
  - Derive the failed request provider and model from one route default.
  - Use the same provider-specific default that request validation uses.
  - Use the provider catalog default when no saved provider model exists.
  - Do not change successful request usage or public response behavior.
  Validation:
  - Prove failed `GET /`, `POST /`, and `POST /v2` requests use the selected
    provider's saved model.
  - Prove usage summaries contain no tenant-wide model under that provider.
  - Prove the documented unconfigured-provider `503` records the catalog
    default model.
  - Run `make ci` after the last application change.

  Resolution:
  - An explicit registered provider now selects its catalog default before
    request validation.
  - A saved managed-provider model replaces that catalog default.
  - Failed `GET /`, `POST /`, and `POST /v2` requests now record the selected
    provider model.
  - The final `make ci` passed all 11 gates with 94 browser scenarios and
    100.0% Go statement coverage.

- [x] [B144] (P1) Use preinstalled Playwright OS packages in hosted CI.
  Goal:
  Hosted CI completes the canonical gate within its 350-second watchdog.
  Evidence:
  - Expected: Hosted CI completes all 11 gates and prints the success receipt.
  - Actual: Run `32207613268` reached gate 9 before the watchdog killed
    `make ci` at exactly 350 seconds.
  - Playwright spent 93 seconds on optional font packages from the Ubuntu
    package mirror. The browser suite then passed all 94 scenarios.
  - Run `32208690338` reduced frontend setup to 11 seconds. The outer Make flag
    changed the default-path fixture cases through inherited `MAKEFLAGS`.
  Requirements:
  - Keep Make as the only frontend dependency owner.
  - Keep the default clean-checkout `--with-deps` behavior.
  - Let hosted CI declare its preinstalled OS packages.
  - Install the exact pinned Chromium browser through the same Make target.
  - Isolate fixture Make processes from parent command variables.
  - Do not increase the watchdog duration.
  Validation:
  - Prove the default and hosted Make command sequences through the public
    target.
  - Run the final `make ci` gate.

  Resolution:
  - Make keeps `--with-deps` as the default Playwright installation flag.
  - Hosted CI supplies an empty installation flag. Make installs the pinned
    Chromium browser without a second OS package operation.
  - Fixture Make processes reject inherited Make command variables. The
    black-box test proves both exact command sequences under contaminated
    parent state.
  - The final `make ci` passed all 11 gates in 125 seconds with 100.0% Go
    statement coverage.

- [x] [B143] (P1) Keep route filters clear of the sticky public header.
  Goal:
  Visitors can click each route filter after the browser scrolls the route
  explorer.
  Evidence:
  - Expected: Each visible and enabled filter accepts pointer input.
  - Actual: GitHub CI positioned the Text filter under the sticky `mpr-header`.
    The header intercepted pointer input until the test timed out.
  Requirements:
  - Preserve the sticky public header and all route explorer selection behavior.
  - Keep scrolled route controls outside the header hit area.
  - Do not use forced clicks or longer timeouts.
  Validation:
  - Repeat the affected browser scenario with the CI browser configuration.
  - Run the final `make ci` gate.

  Resolution:
  - The landing page now uses one 72-pixel scroll clearance for the document
    and section anchors.
  - The browser test scrolls the Text filter to the start position. It verifies
    that the filter is below the header and owns its center hit point.
  - The affected scenario passed 20 serial repetitions.
  - The final `make ci` passed all 11 gates with 94 browser scenarios and
    100.0% Go statement coverage.

- [x] [B142] (P1) {I229} Publish the structured-request client contract.
  Goal:
  Let Creative Director consume the declared structured-output and durable
  request API from an official released Go module.
  Evidence:
  - Before this change, the current released module was `v1.0.0`.
  - That release has no structured-output request fields and no durable request
    reconciliation method.
  - Product release `v3.1.0` contains the client at commit `fe48b5b`, but Go
    rejects that tag because the module path does not end in `/v3`.
  Requirements:
  - Keep structured request construction and reconciliation in the official Go
    client.
  - Remove provider, model, and format query values from reconciliation calls.
  - Publish commit `fe48b5b` as Go module `v1.1.0` for the unchanged module
    path `github.com/tyemirov/llm-proxy`.
  - Do not use a pseudo-version, a replace directive, or a local workspace as
    released dependency evidence.
  - Update Creative Director to use `v1.1.0`.
  Validation:
  - Prove the official client sends the structured schema and idempotency key.
  - Prove reconciliation sends no provider request selection.
  - Prove that `go list -m -versions github.com/tyemirov/llm-proxy` includes
    `v1.1.0`.
  - Run the LLM Proxy CI gate.
  - Run the Creative Director CI gate without a Go workspace or replace
    directive.
  Completion evidence:
  - Tag `v1.1.0` points to commit
    `fe48b5b2259b022eca2156b81be3697465453d8a`.
  - The public Go module graph lists `v1.1.0`.
  - The exact release tree passed all 11 CI gates with 100.0% Go statement
    coverage.
  - Creative Director selects `v1.1.0` and passes CI without a workspace.

- [x] [B140] (P1) Remove gateway-owned Pages markers.
  Goal:
  The LLM Proxy Pages image contains only application-owned site files.
  Evidence:
  - The release gate rejected the image because `site/.nojekyll` reached the
    final Pages image.
  - The source also owns `site/CNAME`, and the Dockerfile deletes that file.
  Requirements:
  - Remove `.nojekyll` and `CNAME` from the application source.
  - Remove the application renderer and Dockerfile contracts for `CNAME`.
  - Keep all reserved Pages marker ownership in the gateway.
  Validation:
  - Prove the repository and Pages Dockerfile do not own reserved markers.
  - Run the renderer integration test.
  - Run `make ci` after the last application change.
  Resolution:
  - Removed `.nojekyll` and `CNAME` from the application source.
  - Removed the renderer requirement and Dockerfile deletion for `CNAME`.
  - Added lifecycle coverage that rejects all reserved marker paths.
  - `make ci` passed all 11 gates with 94 browser tests and 100.0% Go
    statement coverage.

- [x] [B138] (P1) Preserve Kimi reasoning during output continuation.
  Goal:
  Preserve each private Kimi reasoning field when the proxy requests a missing
  output suffix.
  Evidence:
  - Kimi K3 and K2.7 require complete assistant messages in later requests.
  - The current adapter discards `reasoning_content` before a continuation
    request.
  Requirements:
  - Retain `reasoning_content` only as private adapter state.
  - Send each complete assistant message only to its Moonshot continuation.
  - Return only visible content through public responses, logs, and usage.
  Validation:
  - Prove K3 and K2.7 continuations contain exact prior reasoning and content.
  - Prove public responses do not contain private reasoning.
  - Run `make ci` after the last application change.
  Resolution:
  - Stored truncated reasoning only in private Chat Completions state.
  - Sent each complete assistant message to the next upstream Kimi request.
  - Proved K3 and both K2.7 routes return only assembled visible content.
  - `make ci` passed all 11 gates with 94 browser tests and 100.0% Go
    statement coverage.

- [x] [B139] (P2) Reject retired Z.AI credential fields.
  Goal:
  Reject retired `zhipu_api_key` and `glm_api_key` inputs at each public
  credential boundary.
  Evidence:
  - Query and multipart boundaries ignore unknown fields.
  - The rejection set contains `zai_api_key` but omits both retired fields.
  Requirements:
  - Add both retired fields only to the credential rejection set.
  - Keep provider and configuration contracts limited to `zai`.
  - Do not restore retired provider aliases.
  Validation:
  - Prove query and `/dictate` multipart inputs return the client credential
    error.
  - Prove the request does not use the stored provider key.
  - Run `make ci` after the last application change.
  Resolution:
  - Added both retired field names only to the credential rejection set.
  - Proved query and multipart inputs return the client credential error.
  - Proved rejected inputs do not dispatch with the stored provider key.
  - `make ci` passed all 11 gates with 94 browser tests and 100.0% Go
    statement coverage.

- [x] [B130] (P1) Store each DashScope workspace URL with its tenant settings.
  Goal:
  Keep provider configuration in the management domain and keep local and
  production orchestration independent of provider-specific URLs.
  Evidence:
  - `make up` stops before Docker when `configs/.env.local` does not define
    `DASHSCOPE_BASE_URL`.
  - A DashScope API key is valid only for its matching workspace URL.
  - The management store saves a tenant API key, model, and system prompt but
    does not save the tenant workspace URL.
  Requirements:
  - Save the DashScope workspace URL with the tenant provider settings.
  - Require the URL when management verifies and saves a DashScope key.
  - Use the saved URL for tenant requests and provider-key verification.
  - Remove the global DashScope URL from local and production orchestration.
  - Advance incomplete stored DashScope settings through one bounded migration
    into the current canonical schema.
  - Keep public catalog projections free of provider endpoint details.
  Validation:
  - Exercise the management API and routed provider request with a tenant-owned
    DashScope workspace URL.
  - Run `make up` without a local DashScope base URL.
  - Run `make ci` after the last application change.
  Resolution:
  - Local and deployment orchestration no longer bind a DashScope URL.
  - Managed settings now verify, encrypt, and save each tenant's DashScope key
    with its matching Singapore workspace URL.
  - Schema version 7 removes incomplete prior DashScope settings, reconciles
    affected defaults, and preserves tenant timestamps and historical usage.
  - `make up` passed without a local DashScope URL and passed all local
    orchestration readiness checks.
  - `make ci` passed all 11 gates with 91 browser tests and 100.0% Go statement
    coverage.

- [x] [B131] (P1) Restore the five-stage model route.
  Goal:
  Restore one continuous route graph from the product to the selected provider
  offering. The graph uses model family and exact model stages.
  Evidence:
  - The current route explorer separates publisher and model selection from
    the lower route graph.
  - The lower graph combines publisher and exact model in one node.
  - The model family appears only as a filter and model label.
  Requirements:
  - Render exactly five desktop stages: Product, LLM Proxy, Model Family,
    Model, and Provider.
  - Keep this stage order at each supported desktop width.
  - Generate each model family, exact model, and provider offering from the
    normalized public catalog.
  - Select a model family before an exact model.
  - Show only the selected family's exact models in the model stage.
  - Show only the selected exact model's provider offerings in the provider
    stage.
  - Remove the publisher picker and route filters from the route explorer.
  - Keep publisher data in the normalized catalog and capability table.
  - Preserve complete semantic HTML without JavaScript.
  - Preserve responsive containment and the selected provider/model route
    output.
  Validation:
  - Prove the five-stage order and connector endpoints in a real browser.
  - Prove family, model, and provider selection updates the explicit route.
  - Prove all families, exact models, and provider offerings exist without
    JavaScript.
  - Run `make ci` after the last application change.
  Resolution:
  - Restored one continuous Product, LLM Proxy, Model Family, Model, and
    Provider route graph.
  - Removed publisher selection and route filters from the route explorer.
    The capability matrix continues to show publisher data and its filters.
  - Added browser proofs for the five-stage order, connector endpoints,
    selection updates, complete semantic HTML, and responsive layouts.
  - `make ci` passed all 11 gates with 92 browser tests and 100.0% Go statement
    coverage.

- [x] [B132] (P1) Route xAI Responses verification to xAI.
  Goal:
  Verify an xAI Responses model through the selected xAI provider endpoint.
  Requirements:
  - Use the exact protocol adapter and execution lifecycle for verification.
  - Send synchronous Responses verification to the selected provider base URL.
  - Keep OpenAI Responses verification on the configured OpenAI endpoint.
  Validation:
  - Prove management saves a verified `grok-4.5` key after an xAI request.
  Resolution:
  - Keyed verification request builders by wire contract and execution
    lifecycle.
  - Added management proof that `grok-4.5` verification calls xAI and saves
    the key.

- [x] [B133] (P1) Reject large media before provider serialization.
  Goal:
  Reject media that exceeds a provider limit before the proxy reads an asset.
  Requirements:
  - Check media counts and attachment sizes from validated metadata.
  - Check a bounded encoded request minimum before media serialization.
  - Preserve the exact serialized request-size check.
  Validation:
  - Prove each inline-only adapter rejects an oversized closed asset.
  - Require a media limit error for each rejection.
  Resolution:
  - Split metadata admission from the exact serialized request-size check.
  - Proved OpenAI, xAI, and Anthropic reject an oversized closed asset before
    an asset read.

- [x] [B134] (P1) Bound buffered canonical request bodies.
  Goal:
  Keep each buffered `POST /v2` body within a safe service limit.
  Requirements:
  - Apply one service body limit below large provider request limits.
  - Keep tenant assets as the transport for larger media.
  - Return HTTP `413` before JSON decoding for a body above the limit.
  Validation:
  - Prove the service limit overrides a larger provider catalog limit.
  Resolution:
  - Limited buffered `/v2` bodies to 8 MiB or the smaller catalog-derived
    value.
  - Removed the JSON body string copy and added public HTTP `413` proof.

- [x] [B135] (P2) Bind media declarations to exact protocol adapters.
  Goal:
  Accept a media declaration only when the exact route adapter serializes it.
  Requirements:
  - Validate media input against the wire contract and execution lifecycle.
  - Reject media on every Chat Completions route during startup.
  Validation:
  - Prove an xAI Chat Completions media declaration stops catalog startup.
  Resolution:
  - Keyed media support by wire contract and execution lifecycle.
  - Proved an xAI Chat Completions media declaration stops catalog startup.

- [x] [B136] (P2) Require each adapter media transport limit.
  Goal:
  Require the media limit for the transport that each exact adapter uses.
  Requirements:
  - Require inline attachment limits for inline-only adapters.
  - Require file attachment limits for Gemini Interactions adapters.
  - Reject limits that declare only an unused transport.
  Validation:
  - Prove startup rejects file-only limits for an inline adapter.
  - Prove startup rejects a Gemini adapter without file limits.
  Resolution:
  - Required inline attachment limits for inline adapters and file attachment
    limits for Gemini adapters.
  - Added startup rejection proofs for each wrong transport declaration.

- [x] [B137] (P2) Close assets after route media rejection.
  Goal:
  Close each resolved asset when exact route media validation rejects a request.
  Requirements:
  - Close all constructed message media before the handler returns an error.
  - Preserve the provider-specific MIME rejection response.
  Validation:
  - Prove xAI WebP asset rejection closes the opened asset reader.
  Resolution:
  - Closed constructed message media before an exact route MIME rejection
    returns.
  - Proved xAI WebP asset rejection closes the asset reader.
  - `make ci` passed all 11 gates with 93 browser tests and 100.0% Go statement
    coverage.

- [x] [B126] (P1) Reconcile the stale v0.4.0 production activation blocker.
  Resolution:
  - The forward-only production lifecycle superseded `v0.4.0` with `v3.1.1`.
  - The `v3.1.1` release receipt binds source commit
    `264fb18379eef07d65e90ff7b47ae7e621869e0a` to container manifest
    `sha256:f7e145af76763655557e5739ca964ab8713058c53c33c5f017b5999ceb54946e`.
  - The running service uses that manifest and image ID
    `sha256:74890559bf309ea2362f47f47f6733574c05330a6a0a6e4d87e6d2c1160da19f`.
    The Pages marker reports the same version and source commit.
  - The public configuration route returned HTTP `200`. The proxy root returned
    its expected HTTP `403` authentication boundary.

- [x] [B127] (P1) Activate I045 telemetry for B088 production acceptance.
  Resolution:
  - Production `v3.1.1` contains I045 and uses the recorded immutable source and
    container manifest from B126.
  - Request `EM73QSJWEYKXDL4FTHXYGJ5TSF` correlated one content-free Gemini
    provider-progress event with one terminal phase summary. The summary
    recorded the 900-second request budget, provider HTTP time, poll wait, and
    zero proxy rate-limit wait.
  - The live harness validated the matching response request ID without printing
    the response body or a credential.

- [x] [B088] (P1) Restore Default-tenant long completion routing for OpenAI and Meta.
  Goal:
  Make the Default tenant complete deterministic production live-test requests
  through OpenAI and Meta. Do not use local provider credentials, fallback
  providers, or client-side polling. Keep the repaired Anthropic case as a
  required regression check.
  Completion boundary:
  - Repository changes and repository validation prove development completion.
  - Production acceptance is an explicit completion condition for this issue.
  Evidence:
  - The initial expanded `make live-test` run returned HTTP `200` for all three
    providers' short echo requests. OpenAI's background-polling case exhausted
    its 900-second budget with a safe HTTP `504`. Anthropic and Meta long
    completion cases returned safe HTTP `502`.
  - After the shared continuation coordinator shipped in release `v0.2.48`,
    Anthropic long completion returned HTTP `200` with 18,098 response bytes.
    OpenAI still exhausted the full budget with HTTP `504`, while Meta moved
    from the immediate `502` to a full-budget `504`. As a result, Anthropic is
    no longer an unresolved route. OpenAI and Meta still need diagnosis.
  - The harness sent the same request larger than 16 KiB to all three cases,
    required normalized output for all 120 fictional portfolio records before
    the final marker. It printed no response body or credential. It continued
    through the then-current eight-case matrix. The current harness contains
    five echo cases and four long-completion cases.
  - B089 supplies a safe proxy request id and provider failure metadata. I045
    now prints the validated response request id in the live harness and adds a
    correlated proxy phase and provider-progress timeline.
  - On 2026-08-10, the current nine-case paid run passed all five echo cases.
    OpenAI background polling exhausted 900 seconds with HTTP `504`. Its request
    id was `SYAQENJCFBOD5QE7RNB62IKNDD`. Meta long completion exhausted 900
    seconds with HTTP `504`. Its request id was
    `KW5SYFPYZP2FPSCOI4WCMTJKTW`. Anthropic long completion remained healthy
    with HTTP `200`, 10,836 response bytes, and its final marker. Its request id
    was `2GTUJCLSMIOO7CIJUW6XYLWTM3`.
  - The same run proved both target routes accept the saved Default-tenant
    credential and model through their echo cases. Production `v3.1.1` now
    contains I045, and B127 records the correlated telemetry acceptance.
  - The long-completion harness gives each provider a 512-token initial output
    budget.
  - B085 requires a larger next budget when an incomplete attempt has no
    visible output.
  - The current coordinator keeps the same budget when the model has no
    configured output limit.
  - The Terra and Muse Spark catalog records have no configured output limit.
  - The existing zero-progress regression uses a model with a configured output
    limit.
  - The Meta regression always returns visible partial output. It does not test
    the zero-progress case.
  - The last B088 paid run occurred before I045 reached production. B127 proves
    I045 with one Gemini request only.
  Development evidence:
  - On 2026-08-27, two new public route tests reproduced the defect. Both
    continuations sent 512 tokens again instead of 1024.
  - The repair keeps one shared coordinator. It doubles an explicit budget only
    when the latest attempt returned no visible text.
  - The repair caps known model limits. It uses the platform integer limit when
    the catalog does not declare a model limit.
  - Public tests cover OpenAI and Meta zero-progress continuations. They also
    cover visible progress, configured limits, omitted budgets, and integer
    overflow.
  - The first unchanged baseline run stopped during Go integration tests with
    no named failing test. An immediate focused run passed.
  - The unchanged baseline rerun passed all 11 CI gates in 139 seconds with
    100.0 percent Go statement coverage.
  - After the repair, `make go-test` passed with 100.0 percent Go statement
    coverage. The required final `make ci` passed all 11 gates in 134 seconds.
  - No telemetry event contract changed. The existing I045 progress events and
    terminal phase summary remain the production evidence source.
  Production evidence:
  - Release `v6.0.0` uses application commit
    `0d74ab6ca055ea614b52e93abd4b620018796b7d`.
  - Publication produced container index
    `sha256:6ff3dc29b4918019887fa587c5a132fec83fe6bcb1ed5958007e2f0837cbbd17`.
  - Deployment generation 8 converged with zero failed or unreachable hosts.
  - The public API returned its expected HTTP `403` authentication response.
  - The public configuration and Pages routes returned HTTP `200`.
  - The first paid run proved the repaired OpenAI long case returned HTTP
    `200` with 6,572 bytes and its final marker.
  - I045 classified the other HTTP `503` results as missing tenant provider
    connections before provider execution.
  - The authenticated management API verified and saved Anthropic
    `claude-sonnet-4-6` and Meta `muse-spark-1.1`.
  - The final paid run returned HTTP `200` for the OpenAI, Anthropic, and Meta
    echo cases.
  - Their echo request IDs were `A2UVHOC6IRNE74WWYP4V7LRD3J`,
    `BB3R57HGZIA4W3FR5WTTAQQFIV`, and `OKKJCY6EYLGR4BGZT3MR7UELEM`.
  - OpenAI long completion returned 8,292 bytes with request ID
    `7YMWQOSNOW4JIL6KQWR67JWWKQ`.
  - Anthropic long completion returned 18,630 bytes with request ID
    `Y2BE7WXGQC4TBJUSLUZPXXE4DD`.
  - Meta long completion returned 15,303 bytes with request ID
    `ZRJOPH4DTE4DF5UTGAHJFXZTGM`.
  - I045 recorded one terminal success summary and provider progress for each
    of the six requests.
  - The Meta long request returned no visible output for five attempts. Its
    sixth attempt completed with all 15,303 bytes.
  - The isolated Creative Director canary used `gpt-5.6-terra`, `max`, and a
    900-second request budget.
  - The canary passed on its first provider attempt with nine passing
    assertions and zero blocking items.
  - Canary request `JKP4WTL34KFKITJ522EZB4ZTK7` returned HTTP `200` after
    613,657 milliseconds with 41,715 output bytes.
  - The exact live-test command used a Social Threader client secret. Its
    documentation identified the Default tenant as its target. B160 tracks that
    identity defect.
  Requirements:
  - Diagnose and restore the exact OpenAI and Meta production routes through
    the saved Default-tenant provider configuration. Retain OpenAI's
    server-owned Responses polling and Meta's canonical blocking request
    contract.
  - Increase the next attempt budget after an output-limit result with no
    visible text.
  - Cap the next budget at the configured model output limit when that limit
    exists.
  - Use an overflow-safe increase when the model has no configured output
    limit.
  - Keep the initial budget after an output-limit result with visible text.
  - Keep one shared continuation coordinator for all provider transports.
  - Do not add provider-specific growth rules, retries, or timeout changes.
  - Keep the repaired Anthropic long-completion case in the production matrix
    and treat any regression from HTTP `200` as a new failure of this issue's
    acceptance gate.
  - Do not weaken, skip, shorten, special-case, retry, or replace the
    large-completion live-test cases to conceal a provider, continuation, or
    request-deadline failure.
  - Do not add local provider keys, a client polling endpoint, a fallback
    provider, or an unbounded timeout. Keep request and response data redacted
    from user-facing failures and issue evidence.
  Validation:
  - Run `make live-test` with only the Default-tenant client secret and prove
    the named OpenAI background-polling and Meta long-completion cases return
    HTTP `200` with their final marker while Anthropic long completion remains
    HTTP `200`.
  - Run one normalized Terra/max Creative Director source-world canary with
    the explicit 900-second request budget. Use I045 phase evidence to classify
    any failure before another paid run.
  - Prove an OpenAI zero-progress continuation increases its next output budget
    without a configured model limit.
  - Prove a Meta zero-progress continuation increases its next output budget
    without a configured model limit.
  - Prove visible progress keeps the caller's initial per-attempt budget.
  - Prove a configured model output limit caps the next output budget.
  - Prove an unknown model output limit cannot cause integer overflow.
  - Correlate each production case request ID with I045 progress events and one
    terminal phase summary.
  - For any source change, run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - The shared coordinator increases zero-output continuation budgets without
    a provider-specific rule.
  - Release `v6.0.0` is published, deployed, and active on the public routes.
  - OpenAI, Anthropic, and Meta passed their echo and long-completion contracts.
  - I045 correlated each accepted request with provider progress and one
    terminal phase summary.
  - The Creative Director Terra/max source-world canary passed within its
    explicit 900-second request budget.

### Complete entries archived 2026-09-08

- [x] [B203] (P2) Prepare the capability binary before browser test hooks.
  Evidence: Run `34160268019` passed backend qualification but failed frontend setup at the 30-second hook limit.
  The independent frontend job compiled Go code with an empty build cache inside `beforeAll`.
  Requirements:
  - Build the capability binary before Playwright starts test workers.
  - Keep the browser test and hosted job limits unchanged.
  - Remove temporary build output after success or failure.
  Validation: Run browser tests with an empty Go build cache, then run final local CI.
  Results:
  - The hosted frontend setup log confirmed a Go cache miss.
  - The local five-second probe reproduced the hook timeout with an empty Go build cache.
  - Both selected browser tests passed the same probe after the build moved into global setup.
  - The full frontend lane passed with an empty Go build cache in 105 seconds.
  - All 112 browser tests, the Pages artifact check, and the authentication test passed with their normal limits.
  - An injected compiler failure stopped qualification before browser hooks and preserved the Go error.
  - The failure check confirmed removal of temporary build output.
  - Final local `make ci` passed all 12 gates in 272 seconds, with 100.0% Go statement coverage.
  - Governor and changed-prose checks passed. Existing language findings remain outside this change.
  - Changed files: Playwright configuration, global setup, management browser tests, and README.
  - No application API or event contract changed.
  Resolution: Local qualification passed. Hosted confirmation requires source sync and a new PR run.

- [x] [B202] (P2) Complete hosted CI within independent job budgets.
  Evidence: Run `34158361621` reached the ten-minute job deadline during authentication test startup.
  Go integration tests passed in 345 seconds. All 112 browser tests passed in 119 seconds.
  Requirements:
  - Run backend and frontend qualification in independent jobs with ten-minute limits.
  - Keep every canonical CI gate and the 100% Go coverage requirement.
  - Require both jobs to succeed before the existing `test` check passes.
  - Reject failed, cancelled, skipped, or missing job results.
  - Keep `make ci` as the complete local qualification command.
  Validation: Add failing public Make and workflow command tests. Run focused checks and final local CI.
  Results:
  - Initial regressions found missing `ci-backend` and `ci-frontend` targets and the absent aggregate check.
  - The hosted jobs now run the backend and frontend gate groups independently.
  - Both groups retain ten-minute limits. The aggregate check retains the `test` name.
  - Removed duplicate Go lint tool installation from the workflow. Make continues to own Go analysis.
  - The dependency contract target passed with both new public Make entry points.
  - The workflow command passed all 25 combinations of successful, failed, cancelled, skipped, and missing dependency results.
  - The contract test requires the hosted groups to cover every local CI gate exactly once.
  - Final local `make ci` passed all 12 gates in 317 seconds, with 100.0% Go statement coverage.
  - All 112 browser tests and the management authentication test passed.
  - Governor and changed-prose checks passed. Existing language findings remain outside this change.
  - Changed files: workflow, Makefile, dependency tests, hosted CI tests, and README.
  - No application API or event contract changed.
  Resolution: Source changes passed local validation. The changes remain uncommitted. Hosted confirmation requires a new PR run after source sync.

- [x] [B201] (P2) Preserve Kimi logo contrast in light themes.
  Evidence: The white Kimi symbol has no background and disappears on light surfaces.
  Requirements:
  - Give the Kimi artwork a contrasting surface on management and public pages.
  - Set both theme and palette attributes in browser tests.
  - Verify the actual page colors before icon contrast assertions.
  Validation: Confirm failing browser regressions, then run focused checks and final `make ci`.
  Results:
  - Both browser regressions first failed because Kimi images had transparent backgrounds on confirmed light pages.
  - The manifest now selects a dark surface for the unchanged Kimi SVG.
  - Shared CSS and asset validation support the dark surface.
  - Tests set both theme and palette attributes and confirm actual page colors.
  - Both surfaces passed checks in the default light, sunrise light, and default dark palettes.
  - All 13 focused icon tests passed. Screenshots confirm visible Kimi symbols on light surfaces.
  - Final `make ci` passed all 12 gates with 100.0% Go statement coverage and 112 browser tests.
  - Changed files: brand manifest, brand stylesheet, asset validator, management browser tests, and provider icon documentation.
  - No API or event contract changed.
  Resolution: Kimi artwork keeps its contrast in supported light and dark palettes. The changes remain local and uncommitted.

- [x] [B199] (P2) Remove models without active provider offerings from runtime discovery.
  Evidence: A disabled Vertex provider leaves public models with no capabilities or provider offerings.
  Requirements:
  - Remove models when their last active provider offering is disabled.
  - Remove families without runtime models.
  - Keep models available through another active provider offering.
  - Keep the private catalog metadata.
  Validation: Use public HTTP tests and final CI after B200.
  Implementation: Runtime models now require an active provider offering. Runtime families use these models.
  The private catalog retains all metadata. Shared models remain available through active providers.
  The initial HTTP test found four Vertex models with empty capabilities and provider offerings.
  Resolution (2026-09-07): `make test-provider-catalog` and final `make ci` passed.
  CI passed all 12 gates with 100.0% Go statement coverage.
  Changed files: `internal/proxy/provider_catalog_schema.go`, `internal/proxy/model_activation_test.go`, and `docs/provider-catalog.md`.
  No event contract changed.

- [x] [B200] (P2) Keep reported Qianfan token usage when response policy rejects content.
  Evidence: A blocked response with valid token usage produces a failed request with zero recorded tokens.
  Requirements:
  - Parse valid token usage before response policy checks.
  - Keep reported usage on failed requests without exposing rejected text.
  Validation: Use public HTTP tests and final CI.
  Implementation: The parser now keeps valid usage before the Qianfan policy checks.
  Failed requests retain reported tokens without exposing rejected text.
  The initial accounting test recorded zero tokens instead of two input tokens and three output tokens.
  Resolution (2026-09-07): `make test-baidu` and final `make ci` passed.
  CI passed all 12 gates with 100.0% Go statement coverage in 254 seconds.
  Changed files: `internal/proxy/openai_compatible_chat.go`, `internal/proxy/baidu_usage_test.go`, and `docs/baidu-qianfan.md`.
  No event contract changed.

- [x] [B198] (P1) Permit a disabled candidate for a new provider operation.
  Evidence:
  F054 adds Meta dictation as a disabled candidate.
  Catalog loading fails with `provider=meta operation=dictation default_count=0`.
  A default on that disabled model fails with `reason=disabled_default`.
  Requirements:
  - Validate all retained candidate metadata.
  - Require one default for each enabled provider operation.
  - Reject defaults on disabled models.
  - Prove candidate discovery exclusion and explicit activation through HTTP tests.
  Validation:
  `make test-meta-transcription` reproduces the catalog failure before this correction.
  Resolution (2026-09-06):
  Candidate metadata validation and enabled-operation default validation are separate.
  Public HTTP and constructor tests pass. Final CI passed all 12 gates with 100% Go coverage.

- [x] [B197] (P1) Use synchronous Gemini interactions for Flash-Lite.
  Goal:
  Correct the Flash-Lite candidate transport to match the provider contract.
  Evidence:
  Google returned HTTP 400 with `Model 'gemini-3.5-flash-lite' does not support background interactions.`
  Requirements:
  - Use the existing synchronous Gemini execution path with `background: false` and `store: false`.
  - Preserve reasoning controls, media, structured output, continuation, and candidate activation gates.
  - Qualify the declared synchronous lifecycle instead of unsupported background operations.
  Validation:
  - Start with a failing public HTTP test for exact request and lifecycle behavior.
  - Verify public capability decoding, the candidate harness, live acceptance, and final CI.
  Implementation:
  - Flash-Lite now uses `text_synchronous` with the existing Gemini adapter.
  - Text requests and key verification use `background: false` and `store: false`.
  - Shared media admission and public capability decoding accept the synchronous Gemini route.
  - Public HTTP tests cover reasoning, incomplete results, provider errors, media, and key verification without stored resources.
  - The direct live matrix passed all five effort cases. Proxy key verification, omitted-effort text, and image input passed.
  - Full proxy reasoning stopped after a later timeout. Live audio remains a separate F047 acceptance requirement.
  Validation:
  - Initial public HTTP and CLI tests reproduced unsupported background requests.
  - Focused HTTP and CLI tests pass.
  - CI exposed an uncovered catalog default-validation branch in concurrent work.
  - A public catalog test now verifies that Gemini candidates require one text default.
  - The focused default test passes. Final CI passed all 12 gates with 100.0% Go coverage in 245 seconds.
  - Evidence: `/tmp/llm-proxy-b197-http-final.log`, `/tmp/llm-proxy-i251-contract-final.log`, and `/tmp/llm-proxy-b197-lite-live.log`.
  - The correction introduces no event contract. F047 controls model activation.
  Changed files:
  - `configs/providers.yml`, `internal/proxy/provider_types.go`, and `internal/proxy/provider_router.go`.
  - `internal/proxy/message_media.go`, `internal/proxy/provider_key_verifier.go`, and `pkg/llmproxyclient/capabilities.go`.
  - `internal/proxy/gemini_current_models_test.go`, `scripts/test_live_gemini_candidates.sh`, and `tests/operational_contract_test.go`.
  - `Makefile`, `docs/gemini-current-models.md`, and `docs/provider-catalog.md`.
  Resolution (2026-09-07): Source changes and required validation passed.
  Evidence: `/tmp/llm-proxy-b197-ci-verified.log`.

- [x] [B196] (P1) Use catalog field defaults in live-provider qualification.
  Goal:
  Permit a live provider connection with one API key and a catalog URL default.
  Evidence:
  F032 adds a valid required setting with a default URL and no environment binding.
  `make test-live-providers` exits before dispatch because environment discovery rejects this field.
  Requirements:
  - Include each field default in the CLI provider-discovery contract.
  - Use catalog defaults when a field has no supplied environment value.
  - Inspect and clear only declared environment bindings.
  - Keep required credentials mandatory and keep their values out of command output.
  Validation:
  - Prove verification and text dispatch through the real harness with a local connection fixture.
  - Verify final CI at the F032 stack checkpoint.
  Resolution (2026-09-05):
  CLI provider discovery now includes `fields[].default` from the validated catalog.
  The live harness uses a supplied environment value or the canonical field default.
  It clears only declared environment bindings before it imports a selected environment file.
  Required credential fields retain empty defaults.
  The initial command test failed with `catalog_field_binding_invalid`.
  The corrected test verifies the connection and text route with one key and the catalog URL default.
  It also proves that the selected file replaces a stale process key.
  Baidu live verification and text generation both returned HTTP 200.
  Final CI passed all 12 gates with 100.0% Go statement coverage.
  Evidence: `/tmp/llm-proxy-b196-red.log`, `/tmp/llm-proxy-b196-focused.log`, and `/tmp/llm-proxy-f032-ci-final.log`.
  Changed files:
  `cmd/cli/provider_catalog_discovery.go`, `cmd/cli/root_test.go`, `scripts/test_live_providers.sh`,
  `tests/operational_contract_test.go`, `Makefile`, and `docs/provider-catalog.md`.
  The CLI discovery contract adds `default`. Public event schemas did not change.

- [x] [B195] (P1) Correct DashScope Base64 image admission.
  Goal:
  Apply the documented image count and complete Data URI limit to the existing DashScope routes.
  Evidence:
  The catalog uses URL image counts of 2,048 and 256 for a route that sends Base64 Data URIs.
  Alibaba limits Base64 requests to 250 images and each complete image Data URI to 20 MB.
  Requirements:
  - Set the image count to 250 for both existing DashScope image offerings.
  - Count the complete Data URI against a 20,000,000-byte per-image limit.
  - Keep the unknown total request bound explicit.
  - Preserve raw-byte and Base64-byte limits for other provider contracts.
  Validation:
  - Prove count rejection through public HTTP requests before provider dispatch.
  - Prove exact URI boundaries for PNG and JPEG images.
  - Verify current catalog values, units, scopes, sources, and dates.
  - Run final CI after the last implementation change.
  Source:
  - https://www.alibabacloud.com/help/en/model-studio/vision
  Resolution (2026-09-05):
  Both existing DashScope image routes enforce 250 images and 20,000,000 bytes per complete Data URI.
  The new `attachment_data_uri_bytes` scope includes the prefix, MIME type, and Base64 content.
  The public Go client, site renderer, OpenAPI schema, and generated API page accept that scope.
  Public HTTP tests prove exact count and URI boundaries for both models.
  The initial CI run found stale generated API documentation.
  `make generate-api-docs` corrected the page, and `make frontend-lint` passed.
  Final CI passed all 12 gates and 95 browser tests with 100.0% Go statement coverage in 196 seconds.
  Evidence: `/tmp/llm-proxy-b195-red.log`, `/tmp/llm-proxy-b195-focused.log`, and `/tmp/llm-proxy-b195-ci-final.log`.
  Changed files:
  `configs/providers.yml`, `internal/proxy/media_limits.go`, `internal/proxy/message_media.go`,
  `internal/proxy/dashscope_media_limits_test.go`, `pkg/llmproxyclient/capabilities.go`, `tests/capabilities_test.go`,
  `scripts/render_public_site.mjs`, `Makefile`, `docs/openapi.yaml`, `site/docs/index.html`,
  `docs/provider-catalog.md`, and `docs/dashscope-responses.md`.
  The public media-limit contract adds one scope value. Public event schemas did not change.

- [x] [B194] (P1) Correct Gemini candidate image request limits.
  Goal:
  Apply the documented 20 MB media request bound to all Gemini offerings.
  Evidence:
  F047 declares a 100 MB general bound and a separate 20 MB audio bound.
  Google's image guide also states a 20 MB total inline request limit.
  The general file guide states that limits can vary by media type and model.
  Requirements:
  - Set the common inline request bound to 20,000,000 encoded bytes.
  - Remove the unused audio-specific descriptor and adapter branch.
  - Verify current source evidence for all four Gemini offerings.
  - Preserve exact Files API uploads and cleanup above the bound.
  - Correct the F047 documentation and the I249 audit premise.
  Validation:
  - Start with a failing public HTTP image request above 20 MB encoded size.
  - Verify image and audio uploads for each exact model.
  - Verify inline boundaries and catalog metadata through public entry points.
  - Run final CI after the last implementation change.
  Sources:
  - https://ai.google.dev/gemini-api/docs/image-understanding
  - https://ai.google.dev/gemini-api/docs/audio
  - https://ai.google.dev/gemini-api/docs/file-input-methods
  Resolution (2026-09-05):
  All four Gemini offerings use the media-specific 20,000,000-byte request bound.
  The unused audio override is removed. Source references and verification dates are current.
  Public HTTP tests upload 16,000,000-byte image and audio assets for each model.
  Each request exceeds the encoded inline bound and verifies exact Files API bytes, URI dispatch, and deletion.
  Inline boundary tests and public metadata tests pass. Current documentation explains the source distinction.
  Final CI passes all 12 gates and 95 browser tests with 100.0% Go statement coverage in 181 seconds.

- [x] [B193] (P1) Exclude inactive families from runtime discovery.
  Goal:
  Preserve the route explorer when a model family has no enabled models.
  Evidence:
  The disabled M3 model leaves an empty family in public capabilities.
  The browser stops route initialization with `routing_tree_buttons_missing: selector=[data-route-model]`.
  The connector test repeatedly detects missing downstream lines.
  Requirements:
  - Compile runtime families from enabled model references.
  - Retain all family metadata in the private catalog.
  - Restore the family when one of its models becomes enabled.
  - Preserve the current model and offering counts.
  Validation:
  - Prove family activation through the public capability resource.
  - Verify the route explorer and model search through browser tests.
  - Run focused catalog tests and final CI.
  Resolution (2026-09-05):
  Runtime catalog compilation now retains only families referenced by enabled models.
  The private catalog retains complete family metadata.
  Public HTTP tests prove family removal and restoration through model activation.
  Focused browser tests prove model selection, connector output, and candidate exclusion from search.
  Final CI passed all 12 gates with 100% Go statement coverage and 95 browser tests.
  Evidence: `/tmp/llm-proxy-f045-b193-i247-ci.log`.
  Public event schemas did not change.

- [x] [B192] (P1) Separate MiniMax reasoning from visible answers.
  Goal:
  Return only the visible answer from existing MiniMax text routes.
  Evidence:
  - The current MiniMax contract puts reasoning in content by default.
    Its documented `reasoning_split: true` request separates that reasoning:
    https://platform.minimax.io/docs/api-reference/text-openai-api
  - Public HTTP tests on 2026-09-05 returned HTTP 200 with private test reasoning
    in the answer and visible continuation history for all seven M2 routes.
  Requirements:
  - Select a MiniMax Chat Completions request profile through catalog metadata.
  - Send `reasoning_split: true` for generation and credential verification.
  - Apply the profile to all seven current MiniMax offerings.
  - Preserve visible continuation, native usage totals, and existing defaults.
  - Keep reasoning out of public answers and logs.
  - Keep other providers on their declared request contracts.
  Validation:
  - Prove separation through public HTTP requests and continuation.
  - Prove key verification uses the selected model profile.
  - Prove the catalog rejects the profile on another wire contract.
  - Run focused tests and final CI.
  Resolution:
  All seven MiniMax offerings select `minimax_chat_completions`. Generation and
  key verification send `reasoning_split: true`. Public continuation tests
  preserve visible text and usage without exposing test reasoning.
  Final CI passed all 12 gates in 185 seconds with 100% Go statement coverage
  and 95 browser tests. Evidence: `/tmp/llm-proxy-b192-ci.log`.

- [x] [B191] (P1) Reject invalid xAI Responses success payloads.
  Goal:
  Return success only for a valid xAI result with no provider error.
  Evidence:
  - Public tests returned HTTP 200 for a response with only top-level `output_text`.
  - Public tests returned HTTP 200 for a response with a non-null `error` object.
  Requirements:
  - Parse the documented xAI output items through an xAI-owned codec.
  - Return only visible assistant text and valid function calls.
  - Reject error envelopes, refusals, and invalid synchronous response states.
  - Keep provider reasoning and error text out of public responses.
  Validation:
  - Prove rejection through the public HTTP interface.
  - Run focused tests and final CI with I041.
  Resolution:
  The xAI codec now parses typed output and rejects error envelopes before returning success.
  Public rejection tests passed. Final CI passed all 12 gates with 100% Go statement coverage on 2026-09-05.

- [x] [B190] (P1) Correct historical request dispositions for unconfigured providers.
  Goal:
  Historical requests without a provider connection must appear only in the rejected-request report, as F037 requires.
  Evidence:
  - The live dashboard on 2026-09-05 shows three requests each for DeepSeek, DashScope, MiniMax, SiliconFlow, and Z.AI.
  - The failure report contains these 15 requests under the Default tenant on 2026-09-02.
  - Each record has HTTP `503`, outcome `service_unavailable`, a provider/model pair, and latency of `0 ms`.
  - The three request groups have displayed times `01:54:00`, `01:54:43`, and `01:54:56`.
  - The published website marker identifies `v1.5.0` and commit `2ea0bd2f45cf5a5bd791dce82f44c00f3bd3c3c2`.
  - B145 explicitly recorded catalog defaults for unconfigured-provider `503` responses.
  - B174 preserves historical provider/model pairs that match the catalog.
  - F037 and B179 classify historical `service_unavailable` records as rejections only when both route fields are empty.
  - Thus, a catalog-valid pair from an unconfigured request remains a failed execution after migration.
  Requirements:
  - Reproduce the B145 historical record through the real migration and management HTTP resources.
  - Establish exact evidence for each historical request selected for correction.
  - Use one bounded migration to assign `rejected` and `provider_not_configured` to proven unconfigured-provider requests.
  - Include databases that already completed schema version 13.
  - Preserve safe metadata and actual execution failures.
  - Treat status, zero tokens, zero latency, and current credential absence as insufficient proof when used alone.
  - Exclude corrected records from execution totals, failure reports, charts, and provider/model usage.
  - Keep corrected records in the separate rejection count and report.
  - Keep the provider catalog cards available for connection settings.
  Validation:
  - Confirm the expected failing integration test before the production code change.
  - Verify both account and tenant reports through HTTP.
  - Verify migration rollback and repeated startup with real SQLite storage.
  - Verify chart exclusion and rejection details through the browser.
  - Run `make ci` after the last application change.
  Resolution:
  - On 2026-09-05, the user authorized a one-off migration of the affected historical records.
  - The migration corrected 15 exact records in the production database at schema version 13.
  - Record IDs: `2579` through `2583`, and `2585` through `2594`.
  - The reviewed input includes each original timestamp, tenant, route, status, disposition, outcome, latency, and token count.
  - One GORM transaction changed only the disposition and outcome to `rejected` and `provider_not_configured`.
  - Each record had to match its complete reviewed input before the update.
  - The original HTTP status and all other safe metadata remain unchanged.
  - A native SQLite backup preceded the transaction and passed the same input verification.
  - Backup on `192.168.1.252`: `/volume1/homes/tyemirov/llm-proxy-b190-before-20260905.sqlite`.
  - Local evidence and recovery records: `.git/b190/receipt.json` and `.git/b190/rows-before.json`.
  - Verification found 15 corrected records and zero further updates.
  - Live account execution requests changed from 1,955 to 1,940, and failures changed from 584 to 569.
  - Provider usage changed from 11 providers to six. Token usage remained 64,860,374.
  - The Default tenant rejection report shows all 15 corrected records as `Provider not configured`.
  - The initial integration test failed with `B190 correction not implemented`.
  - The completed test passed through the temporary CLI, real SQLite storage, and account and tenant HTTP resources.
  - The test verified rollback after a changed second record, repeat execution, startup, and unchanged metadata.
  - `make ci` passed all 12 gates in 179 seconds with 100 percent Go statement coverage.
  - The browser suite passed 95 tests. Live browser checks verified both usage scopes and all corrected rejection rows.
  - The temporary executable, command, and test were removed after validation. Their source remains in the local evidence record.
  - The service schema and deployment remain unchanged.

- [x] [B189] (P2) Use the selected release version in Python package metadata.
  The validator accepts `v1.5.0`, but Python installations still report version `1.4.1`.
  Requirements:
  - Read the Gix-selected release tag during Python package builds.
  - Keep that version in wheel and source distribution metadata.
  - Remove the obsolete manual version contract.
  Validation:
  - Build and install tagged packages through public package commands.
  - Run `make test-release-policy`, `make python-package-install-test`, and `make ci`.
  Resolution:
  - Gix remains the authority for every `v1.X.X` release version.
  - Python builds read the release tag through `setuptools-scm` and keep its version in package metadata.
  - The manual version command and stored version declarations are removed.
  - CI fetches Git history and tags. Package cache keys include commits and tags.
  - The tagged installation regressions failed with version `1.4.1` before the correction.
  - Five package regressions passed, including source distribution builds and a cached installation after a new release tag.
  - All 12 CI gates passed in 172 seconds with 100 percent Go statement coverage.
  Changed: `python/pyproject.toml`, `python/uv.lock`, `Makefile`, `scripts/run_ci.sh`, and `.github/workflows/test.yml`.
  Changed: `tests/python_package_contract_test.py`, `tests/lifecycle_contract_test.go`, `tests/operational_contract_test.go`, and `tests/frontend_dependency_contract_test.go`.
  Changed: `README.md` and `docs/implementation/provider-routing-plan.md`.
  Removed: `VERSION` and `scripts/release_version.py`.
  Event contracts: No change.

- [x] [B188] (P1) Accept the Gix release decision.
  Gix selected `v1.5.0`, but the application validator required the stored value `v1.4.1`.
  The automatic release failed before CI.
  Requirements:
  - Accept the version from the Gix decision without comparison to stored package metadata.
  - Retain the application policy for major version `1`.
  Validation:
  - Run `make test-release-policy` and `make ci`.
  Resolution:
  - The validator accepts the Gix version without a comparison to stored package metadata.
  - This correction replaces the B175 requirement for equality with `VERSION`.
  - The exact saved `v1.5.0` decision passed the corrected validator.
  - The regression failed before the correction and passed after it.
  - All 12 CI gates passed in 174 seconds with 100 percent Go statement coverage.
  Changed: Release policy validator, integration tests, Makefile, release documentation, and terminology.
  Event contracts: No change.

- [x] [B187] (P1) Repair the OpenAPI merge result.
  After the B186 correction, CI reports `bad indentation of a mapping entry (83:50)`.
  The merge joined two YAML keys and inserted a duplicate `/` path.
  The generated API reference differs from the canonical source.
  Requirements:
  - Remove the duplicate path and separate the YAML keys.
  - Preserve the health resource and client protocol operations.
  - Generate the API reference from the corrected source.
  Validation:
  - Run `make test-client-contracts`, `make frontend-lint`, and `make ci`.
  Resolution:
  - The corrected YAML retains each current operation once.
  - The generated API reference matches the canonical source.
  - The existing public API tests failed before the correction and passed after it.
  - All 12 CI gates passed with 100 percent Go statement coverage.
  Changed: `docs/openapi.yaml` and `site/docs/index.html`.
  Event contracts: No change.

- [x] [B186] (P1) Correct Go formatting after the merge.
  GitHub run `33925059200` failed at the Go formatting gate on commit `86baabd`.
  `make check-format` reports `internal/proxy/openapi_contract_test.go` locally and on GitHub.
  Requirements:
  - Apply the repository formatter to the affected file.
  - Preserve the test assertions and public contracts.
  Validation:
  - Run `make check-format` and `make ci`.
  Resolution:
  - The repository formatter separated the switch case and its assignment.
  - `make check-format` passed after the correction.
  - After the B187 correction, all 12 CI gates passed with 100 percent Go statement coverage.
  Changed: `internal/proxy/openapi_contract_test.go`.
  Event contracts: No change.

- [x] [B184] (P1) Keep partial function calls pending during background polling.
  Goal:
  Background Responses snapshots can contain empty or partial function arguments.
  The parser rejects these snapshots before the provider completes the response.
  Requirements:
  - Continue polling while the response is queued or in progress.
  - Validate complete function calls before returning a terminal result.
  - Cover partial snapshots and malformed completed calls through the real HTTP lifecycle.
  Validation:
  - Confirm the HTTP regression fails before the production change.
  - Run `make test-client-contracts` and the final `make ci` gate.
  Evidence:
  - Before the fix, both public protocols returned `502` after one poll instead of four.
  - After the fix, both protocols return completed calls and reject malformed terminal arguments.
  - `make test-client-contracts` passed.
  Resolution:
  - Pending snapshots no longer require complete function arguments.
  - Terminal calls still require valid identifiers, names, and JSON objects.
  - All 12 CI gates passed with 100 percent Go statement coverage.
  Changed: `internal/proxy/openai.go` and `internal/proxy/client_protocol_lifecycle_test.go`.
  Event contracts: No change.

- [x] [B185] (P2) Normalize Python roles during tool history validation.
  Goal:
  Python accepts and serializes normalized roles but rejects the same roles in tool history.
  The empty tool-result exception also compares the original role.
  Requirements:
  - Use the same role normalization for message validation, tool history, and serialization.
  - Cover mixed case, surrounding whitespace, and empty tool results through the real proxy.
  - Keep invalid tool history rejection.
  Validation:
  - Confirm the native Python regression fails before the production change.
  - Run `make test-client-contracts`, `make python-test`, and the final `make ci` gate.
  Evidence:
  - Before the fix, the native Python test rejected mixed-case and whitespace-padded roles.
  - The errors were `function calls require assistant role`, `missing tool result`, and `empty message content`.
  - After the fix, normalized tool history reaches the real proxy, including empty tool results.
  - Invalid tool histories still fail before dispatch.
  - `make test-client-contracts` and all 45 Python tests passed.
  - The Python package installation check passed.
  Resolution:
  - Constructor and tool history checks now use normalized roles.
  - Native HTTP regressions cover normalized roles, empty results, and invalid history rejection.
  - All 12 CI gates passed with 100 percent Go statement coverage.
  Changed: `python/llm_proxy_client/client.py` and `tests/client-protocols/native-python.py`.
  Event contracts: No change.

- [x] [B183] (P1) Wait for usage persistence before DashScope test teardown.
  The workspace routing test removes its temporary database while the usage writer is active.
  Two `make go-test` runs failed with `TempDir RemoveAll cleanup: directory not empty`.
  Read the tenant usage resource until the routed request is present before the test exits.
  Validate the public usage assertion and the repository test gate.
  Resolution: The test confirms one persisted request through the tenant usage API before teardown.
  The focused test and all 12 CI gates passed.
  Changed: `internal/proxy/management_provider_key_verification_test.go`.
  No event contract changed.

- [x] [B182] (P1) Bound the request-disposition migration insert.
  Goal:
  The schema 12 upgrade must retain a large usage history. The current copy uses
  one insert and exceeds the SQLite variable limit for realistic histories.
  Requirements:
  - Copy migrated usage records in fixed-size batches.
  - Keep the schema replacement in one transaction.
  - Preserve each record, index, constraint, and migration version.
  Deliverables:
  - Add bounded inserts to the schema 13 migration.
  - Add a real-database regression with a history above the prior limit.
  Validation:
  - Run the focused Go integration test.
  - Run `make ci` after the last review fix.
  Resolution:
  - The schema 13 copy now inserts at most 256 usage records in each batch.
  - One SQLite startup test migrates 2,600 records and verifies the final record.
  - `make go-test` passed with 100 percent Go statement coverage.

- [x] [B181] (P2) Bound request-disposition startup validation.
  Goal:
  Schema 13 startup must validate disposition pairs without reading the full
  usage history. The current query loads all event columns and rows.
  Requirements:
  - Read each distinct disposition and outcome pair once.
  - Retain one record ID for each invalid-pair error.
  - Reject each invalid disposition, outcome, or pair.
  Deliverables:
  - Replace the full-history validation query with a bounded projection.
  - Extend real-database validation coverage.
  Validation:
  - Run the focused Go integration test.
  - Run `make ci` after the last review fix.
  Resolution:
  - Startup now reads each distinct disposition and outcome pair once.
  - Each projected pair retains the first record ID for exact errors.
  - A 2,600-record SQLite regression verifies repeated valid pairs.
  - `make go-test` passed with 100 percent Go statement coverage.

- [x] [B180] (P2) Count asset-store errors as proxy failures.
  Goal:
  A valid V2 request can fail because the proxy cannot read its asset store.
  The current recorder classifies this internal `500` response as rejected.
  Requirements:
  - Assign `proxy_error` to a V2 asset-store failure before persistence.
  - Keep input and missing-asset errors as rejected requests.
  - Keep the request out of provider dispatch.
  Deliverables:
  - Correct the V2 asset error outcome mapping.
  - Add a public HTTP and management-report regression.
  Validation:
  - Run the focused Go integration test.
  - Run `make ci` after the last review fix.
  Resolution:
  - V2 asset-store `500` responses now persist the `proxy_error` outcome.
  - Missing assets and invalid asset input remain rejected requests.
  - A public HTTP test verifies zero provider dispatch and failure-only reporting.
  - `make go-test` passed with 100 percent Go statement coverage.

- [x] [B179] (P2) Preserve legacy unconfigured-provider rejections.
  Goal:
  The oldest tenant migration must retain unconfigured-provider requests as
  rejections. It currently converts a blank-route `503` event to a failure.
  Requirements:
  - Map a legacy blank-route `503` event to `provider_not_configured`.
  - Assign the `rejected` disposition to the migrated event.
  - Preserve the current mapping for an execution `503` event.
  Deliverables:
  - Correct the legacy tenant migration mapping.
  - Add exact migration coverage for both `503` event types.
  Validation:
  - Run the focused Go integration test.
  - Run `make ci` after the last review fix.
  Resolution:
  - The oldest tenant migration maps blank-route `503` events to
    `provider_not_configured` and the `rejected` disposition.
  - Routed execution `503` events remain `service_unavailable` failures.
  - A real SQLite migration test verifies both event types.
  - `make go-test` passed with 100 percent Go statement coverage.

- [x] [B178] (P2) Record invalid timeout headers as rejections.
  Goal:
  Each authenticated proxy request must produce a usage record. The timeout
  middleware currently returns `400` before it records an invalid header.
  Requirements:
  - Record invalid timeout headers for text, V2, and dictation requests.
  - Assign `invalid_request` and the `rejected` disposition.
  - Keep invalid timeout headers out of provider dispatch.
  - Do not record unauthenticated requests or asset operations as proxy usage.
  Deliverables:
  - Add managed rejection recording to the proxy timeout boundary.
  - Add public HTTP and management-report regressions.
  Validation:
  - Run the focused Go integration test.
  - Run `make ci` after the last review fix.
  Resolution:
  - The timeout boundary now records authenticated invalid headers for text,
    V2, and dictation requests as `invalid_request` rejections.
  - Unauthenticated requests and asset operations remain outside proxy usage.
  - A public HTTP test verifies all route types, reports, and zero dispatch.
  - `make go-test` passed with 100 percent Go statement coverage.

- [x] [B177] (P2) Remove request-disposition panic paths.
  Goal:
  Usage domain errors must propagate through library boundaries. Two new code
  paths panic for an invalid outcome or execution disposition.
  Requirements:
  - Return a validated error for an invalid outcome mapping.
  - Propagate an invalid execution disposition through the usage report path.
  - Remove the direct panic test.
  - Cover the behavior through a database or public API boundary.
  Deliverables:
  - Replace both new panic paths with error propagation.
  - Add boundary regression coverage for corrupt usage data.
  Validation:
  - Run the focused Go integration test.
  - Run `make ci` after the last review fix.
  Resolution:
  - Invalid outcome and disposition data now returns a validated store error.
  - Usage aggregation accepts only a validated execution record and has no
    disposition fallback or panic.
  - A public SQLite and HTTP regression verifies tenant and admin report errors
    for an invalid outcome, disposition, or outcome pair.
  - `make ci` passed all 12 gates with 100 percent Go statement coverage.

- [x] [B176] (P1) Show the Usage chart X axis.
  Goal:
  Each Usage time-series chart defines a UTC X axis. The browser does not show
  this axis because the dynamic SVG view-box attribute has the wrong DOM case.
  Requirements:
  - Bind the SVG view box with its exact DOM attribute name.
  - Show the UTC axis title and each selected time label.
  - Preserve the current chart dimensions and metric-specific Y axis.
  - Cover the actual SVG attribute and visible X axis in the browser.
  Deliverables:
  - Correct both Usage time-series SVG elements.
  - Add browser regression coverage for the rendered view box and X axis.
  Validation:
  - Run the frontend browser tests.
  - Run `make ci` after the last application change.
  - Run the STE check on each changed technical document.
  - Run `git diff --check`.
  Resolution:
  - Both Usage SVG elements now bind the exact `viewBox` DOM attribute.
  - The UTC X axis and labels now remain inside each chart.
  - Browser coverage verifies the view box and visible X-axis bounds.
  - The frontend browser tests passed with 84 tests.
  - `make ci` passed all 12 reported gates with 100 percent Go statement
    coverage.
  - The focused STE review passed for changed prose.

- [x] [B175] (P0) Use one repository release version.
  Goal:
  One neutral repository value must identify the application and each bundled
  client release. The Python client currently owns the accepted application
  version by accident. A new release decision fails until its version is
  manually copied into the Python project and lock metadata.
  Evidence:
  - The last sealed release is `v1.4.0`.
  - Gix selected `v1.4.1` for the current application commit.
  - The release decision validator rejected `v1.4.1` because the Python client
    still declared `1.4.0`.
  Requirements:
  - Add one canonical repository release version.
  - Set the canonical version to `1.4.1`.
  - Keep the application and each bundled client on the exact canonical
    version.
  - Keep each release in major version `1`.
  - Make the Python project and lock metadata consumers of the canonical
    version.
  - Make the release decision validator use the canonical version.
  - Add one repository command that updates all explicit version values.
  - Reject a malformed version, a version above major `1`, and a version
    decrease before any file changes.
  - Make CI reject version drift before the test gates.
  - Do not change `mprlab-gateway` or the selected manifest schema.
  - Preserve all sealed and unsealed lifecycle records.
  Deliverables:
  - Add the canonical version file and version management program.
  - Update the release decision validator and repository automation.
  - Document the version authority and release preparation step.
  - Add public command and validator coverage.
  Validation:
  - Prove that the canonical version, Python project, and Python lock agree.
  - Prove that one command updates all explicit values together.
  - Prove that invalid or decreasing values do not change any file.
  - Prove that the validator accepts only `v1.4.1` with fixed major `1`.
  - Run `make python-package-install-test`.
  - Run `make ci` after the last application change.
  - Run the STE check on each changed technical document.
  - Run `git diff --check`.
  Review evidence:
  - The release tests embed version `1.4.1` instead of reading the root
    `VERSION` file.
  - The documented version command makes the next CI run fail after a version
    increase.
  - The hosted CI path filter does not include the root `VERSION` file.
  Review requirements:
  - Derive release-policy expectations and update candidates from the root
    `VERSION` file.
  - Start hosted CI when a pull request changes the root `VERSION` file.
  - Cover both corrections through repository public-contract tests.
  Resolution:
  - The root `VERSION` file now owns repository release version `1.4.1`.
  - The Python project and lock metadata now match that version.
  - The Go clients receive the same version from the repository tag.
  - One version command updates all explicit version values and rejects invalid
    or decreasing values before it changes a file.
  - CI checks version equality before all test gates.
  - The release decision validator now requires the exact repository version
    and fixed major `1`.
  - Public command and validator tests read the canonical version and calculate
    the next patch version.
  - The update test verifies that the next release decision succeeds.
  - The hosted CI workflow starts when the root `VERSION` file changes.
  - The workflow contract test requires that path filter.
  - Public command tests also cover drift and rejection without file changes.
  - `make python-package-install-test` passed for version `1.4.1`.
  - `make ci` passed all 12 gates with 100 percent Go statement coverage.

- [x] [B174] (P1) Exclude unresolved routes from usage dimensions.
  Goal:
  Usage reports must not identify a raw request value as a provider or model.
  A rejected request with provider `__credential_validation__` and no model
  currently creates a `__credential_validation__ /` model row.
  Route validation rejects the request before it creates a typed route. The
  failure recorder then persists the raw provider and blank model. Usage
  aggregation treats these values as canonical dimensions, and the browser
  displays them.
  Requirements:
  - Give the usage recorder a provider and model only from a resolved typed
    route.
  - Do not copy raw provider or model request values into a usage event when
    route validation fails.
  - Keep each invalid request in the total, status, outcome, latency, and
    failure-report data.
  - Exclude an event without a resolved route from provider and model
    breakdowns.
  - Keep resolved provider and model dimensions for a request that fails after
    route resolution.
  - Add one bounded migration for persisted events whose provider and model do
    not identify a canonical catalog route.
  - The migration must remove both invalid dimension values and preserve all
    other usage data.
  - Do not add a browser filter, compatibility path, or special rule for
    `__credential_validation__`.
  Deliverables:
  - Move usage dimension ownership from raw request fields to the resolved
    typed route.
  - Update usage aggregation to omit events without a resolved route from the
    provider and model buckets.
  - Add the bounded persisted-data migration and its startup verification.
  - Add public API and real-store regression coverage.
  Validation:
  - Submit a request with provider `__credential_validation__` and no model.
  - Prove that route validation rejects the request.
  - Prove that totals, status reporting, and the failure report include the
    rejected request.
  - Prove that no provider or model bucket contains the raw provider value or
    a blank model value.
  - Prove that an upstream failure after route resolution retains its canonical
    provider and model dimensions.
  - Upgrade a store that contains invalid persisted dimensions and prove that
    the migration removes only those dimensions.
  - Run `make ci` after the last application change.
  Resolution:
  - The route resolver now owns the typed provider and model usage dimensions.
  - Pre-route failures keep totals and failure data without provider or model
    dimensions.
  - Failures after route resolution keep their exact canonical dimensions.
  - Schema version 12 clears both dimensions from invalid persisted pairs and
    preserves all other usage data.
  - Public API and real SQLite tests cover recording, aggregation, migration,
    startup verification, and rollback.
  - Current startup validates distinct route combinations instead of all usage
    events.
  - `make ci` passed all 11 gates with 100 percent Go statement coverage.

- [x] [B173] (P0) Keep release versions in Go major version 1.
  Goal:
  The release policy must keep each release version in major version `1`.
  The current release transaction selects `v2.0.0`, and the release decision
  validator rejects that version.
  Evidence:
  - B164 requires the removal of each release identity above major version `1`.
  - The selected manifest does not declare the permanent fixed-major policy.
  - Gix selects `v2.0.0` for the current breaking change.
  - The official Python client version and release decision validator still use
    `v1.3.0`.
  Requirements:
  - Set the selected manifest fixed major to `1`.
  - Set the official Python client and lock metadata version to `1.4.0`.
  - Require the exact fixed-major release decision for `v1.4.0`.
  - Keep the Go module path unchanged.
  - Do not create a version above major version `1`.
  - Do not change `mprlab-gateway`.
  - Preserve each sealed historical lifecycle record.
  Deliverables:
  - Correct the application release policy and official client version.
  - Add public lifecycle coverage for the fixed-major release decision.
  Validation:
  - Prove that Gix selects `v1.4.0` with fixed major `1`.
  - Prove that the release decision validator accepts only the exact decision.
  - Run `make python-package-install-test`.
  - Run `make ci` after the last application change.
  - Run the STE check on each changed technical document.
  - Run `git diff --check`.
  Resolution:
  - The selected manifest declares SemVer with fixed major `1`.
  - The official Python client and lock metadata use version `1.4.0`.
  - The release decision validator requires fixed major `1` and exact version
    `v1.4.0`.
  - Public lifecycle tests reject a missing fixed major, a different fixed
    major, an incorrect `v1` version, and a version above major `1`.
  - Gix selected `v1.4.0` for the current source with fixed major `1`.
  - The failed unsealed `v2.0.0` staging record was removed. Each sealed
    historical lifecycle record remains unchanged.
  - `make python-package-install-test` passed for version `1.4.0`.
  - `make ci` passed all 11 gates with 100 percent Go statement coverage.

- [x] [B172] (P1) {I238,B171} Start each provider card with the Default tenant.
  Goal:
  A provider card must start with the first account tenant. It starts with the
  retained Settings modal tenant after that tenant changes.
  Requirements:
  - Initialize each newly opened provider card with the first account tenant.
  - Keep provider card initialization independent from the Settings modal tenant.
  - Preserve the isolated provider card tenant change behavior.
  Deliverables:
  - Correct the provider card tenant initialization boundary.
  - Add browser coverage for a retained non-default Settings modal tenant.
  Validation:
  - Select a non-default tenant in the Settings modal.
  - Close the Settings modal.
  - Open a provider card.
  - Prove that the provider card selects the first account tenant.
  - Prove that the Settings modal keeps its prior tenant.
  - Run `make ci` after the last application change.
  Resolution:
  New provider cards now select the first account tenant. This selection does
  not use the retained Settings modal tenant. Browser coverage verifies the
  Default provider profile and the retained Settings modal context.

- [x] [B171] (P1) {I238} Keep tenant changes inside the open provider card.
  Goal:
  The tenant selector must be the only tenant name in an open provider card.
  A tenant change reloads the management site instead of only the provider card.
  Requirements:
  - Remove the tenant heading below the provider API label.
  - Keep the tenant selector as the only tenant name.
  - Load the selected tenant profile without a change to the application context.
  - Preserve the dashboard view and the Usage tenant during a provider tenant change.
  - Save pending provider edits before a provider tenant change.
  - Keep the system prompt collapsed after a provider tenant change.
  Deliverables:
  - Isolate provider card tenant state from the management site tenant state.
  - Add browser coverage for the tenant name and tenant change behavior.
  Validation:
  - Prove that an open provider card shows one tenant name.
  - Prove that a provider tenant change does not reload the management site.
  - Prove that the Usage tenant and dashboard view do not change.
  - Prove that the selected provider settings use the new tenant profile.
  - Run `make ci` after the last application change.
  Resolution:
  Removed the repeated tenant heading. The open card now owns its tenant profile
  and load request. Tenant changes preserve the dashboard, provider grid, Usage
  tenant, and Settings modal tenant. Browser coverage verifies the isolated load
  and the selected tenant profile.

- [x] [B170] (P1) Apply each theme to the route explorer.
  Goal:
  The selected theme must change all route explorer colors. The route explorer
  keeps dark backgrounds after a user selects a light theme.
  Requirements:
  - Map each route explorer color to the selected palette.
  - Keep the current default dark appearance.
  - Keep the route explorer readable in each light and dark theme.
  Deliverables:
  - Add route explorer tokens to each canonical palette.
  - Replace fixed route explorer colors with the canonical tokens.
  - Add browser coverage for route explorer theme changes.
  Validation:
  - Select each theme with the footer control.
  - Verify the route explorer surface and control colors for each theme.
  Resolution:
  - Each palette now owns the route explorer surface, control, node, connector,
    focus, and shadow colors.
  - The route explorer redraws its canvas when MPR UI changes the theme.
  - Browser tests verified all four palettes. `make ci` passed all 11 gates with
    100.0% Go statement coverage.

- [x] [B169] (P1) Make all four theme positions selectable.
  Goal:
  The footer theme control shows four theme positions, but only two positions
  select a theme.
  Requirements:
  - Keep the MPR UI quadrant picker presentation.
  - Configure default light, sunrise light, default dark, and forest dark modes.
  - Keep default dark as the initial theme.
  - Apply each selected palette to the complete page.
  - Keep MPR UI ownership of the theme state.
  - Use the same footer theme control on every HTML route.
  Deliverables:
  - Add the four-mode configuration to the canonical footer generator.
  - Add shared theme tokens for all four modes.
  - Regenerate all footer markup.
  - Add browser coverage for all four theme positions.
  Validation:
  - Prove that each quadrant selects its configured mode and palette.
  - Prove that the four modes use four different page colors.
  - Run `make ci` after the last application change.
  Resolution:
  - The canonical footer defines four modes for the MPR UI quadrant picker.
  - Shared theme tokens apply each palette to all HTML route types.
  - The browser test selects each quadrant and proves four different page colors.
  - `make ci` passed all 11 gates with 100% Go statement coverage.

- [x] [B168] (P1) Separate the disposable live-test tenant key.
  Goal:
  The local live-provider harness uses an internal name for its disposable
  tenant key. The private Gemini input contains one canonical assignment.
  Evidence:
  - The harness stores a generated disposable tenant key in the public Default
    tenant key variable.
  - `configs/.env` contains two different `GEMINI_API_KEY` assignments.
  - One Gemini assignment matches the single private deployment assignment.
  Requirements:
  - Use a lowercase internal variable for the disposable tenant key.
  - Keep `LLM_PROXY_DEFAULT_TENANT_KEY` for the CLI and production live test.
  - Remove only the noncanonical duplicate Gemini assignment.
  - Keep the private files ignored, untracked, nonempty, and mode `0600`.
  - Do not print or record a credential value.
  - Do not change `mprlab-gateway`.
  Validation:
  - Prove that the local provider harness has no Default tenant key variable.
  - Run the focused operational contract test.
  - Run the Gemini provider harness with the canonical private input.
  - Run `make ci` after the last application change.
  - Run the STE check on the changed issue text.
  - Run `git diff --check`.
  Resolution:
  - The local harness stores its disposable tenant key in the internal
    `live_tenant_key` variable.
  - The CLI and production live test retain
    `LLM_PROXY_DEFAULT_TENANT_KEY` as their canonical input.
  - The private Gemini input retains one nonempty canonical assignment that
    matches the private deployment input. Both files remain ignored and mode
    `0600`.
  - The focused Go contract suite passed with 100 percent statement coverage.
  - The Gemini verification and smoke request passed with HTTP status 200 for
    both private inputs.
  - `make ci` passed all 11 gates with 100 percent Go statement coverage.

- [x] [B167] (P0) Use the canonical Default tenant key variable.
  Goal:
  Each client and live test uses `LLM_PROXY_DEFAULT_TENANT_KEY` for the Default tenant key.
  Evidence:
  - The private source defines `LLM_PROXY_DEFAULT_TENANT_KEY` with the current key.
  - The inherited generic secret value selects a different tenant.
  - Current source, tests, and documents still require the obsolete variable.
  Requirements:
  - Replace each obsolete reference with `LLM_PROXY_DEFAULT_TENANT_KEY`.
  - Keep one canonical variable name without an alias or fallback.
  - Preserve secret redaction and tenant identity verification.
  - Update the CLI, live tests, public resources, documents, and issue records.
  - Do not change `mprlab-gateway`.
  Validation:
  - Prove that the live test requires the canonical variable.
  - Prove that the CLI reads the canonical variable.
  - Confirm that no tracked obsolete reference remains.
  - Run `make ci` after the last application change.
  - Run the STE check on each changed technical document.
  - Run `git diff --check`.
  Resolution:
  - The CLI, live-test commands, tests, generated public resources, and
    documents use only `LLM_PROXY_DEFAULT_TENANT_KEY`.
  - The owned generator rebuilt all 46 public resource pages.
  - The focused Go contract suite passed with 100 percent statement coverage.
  - `make ci` passed all 11 gates with 100 percent Go statement coverage.
  - The production identity preflight selected the expected Default tenant.
    All nine production provider cases passed.

- [x] [B166] (P1) Identify the current release in the client documentation.
  Goal:
  The main Python install command and changelog identify the current release.
  Evidence:
  - GitHub, Go Proxy, GHCR, Pages, and production use `v1.3.0`.
  - The Python install command still selects `v1.2.2`.
  - The changelog still puts the `v1.3.0` changes under `Unreleased`.
  Requirements:
  - Select the exact `v1.3.0` tag in the main Python install command.
  - Put the released changes in a dated `v1.3.0` changelog section.
  - Keep a new `Unreleased` section for later changes.
  - Do not change an immutable release identity.
  Resolution:
  - The main Python install command selects the exact `v1.3.0` tag.
  - The changelog has a dated `v1.3.0` section and a new `Unreleased` section.
  - The installed package from the exact tag reports version `1.3.0`.
  - Go Proxy resolves the exact tag to the released application commit.
  Validation:
  - Install the Python package from the exact `v1.3.0` tag.
  - Confirm that the installed package reports version `1.3.0`.
  - Run the STE check on each changed technical document.
  - Run `git diff --check`.

- [x] [B165] (P0) Prepare the next official client version.
  Goal:
  Current source and each official client use the next valid version.
  Evidence:
  - Release `v1.2.2` identifies commit `aea550bb2a5f3728e7fbe058446f11ade11a53c3`.
  - The default branch contains seven later commits with new public behavior.
  - Expected: current source uses the next SemVer minor version, `1.3.0`.
  - Actual: the Python source and lock metadata still use `1.2.2`.
  - Actual: the Python installation example selects the default branch.
  - Actual: the changelog keeps released `v1.2.2` changes under `Unreleased`.
  Requirements:
  - Set the Python source and lock metadata to `1.3.0`.
  - Keep the Go module path unchanged.
  - Require the release decision to match `v1.3.0`.
  - Keep the completed `v1.2.2` changes in one dated changelog section.
  - Keep only later changes under `Unreleased`.
  - Install the Python client from an exact released tag in the main example.
  - Keep release, publication, and deployment outside this development issue.
  Resolution:
  - The Python source and lock metadata use `1.3.0`.
  - The release decision tests accept only the exact `v1.3.0` version.
  - The Go module path is unchanged.
  - The changelog has a dated `v1.2.2` section and a new `Unreleased` section.
  - The Python installation example uses the released `v1.2.2` tag.
  - `make python-package-install-test` passed for version `1.3.0`.
  - `make ci` passed all 11 gates with 100 percent Go coverage.
  - GitHub published `v1.3.0` for application commit `7a61fc55f36786086cb887a6f9b91f4450119e8d`.
  - Go Proxy resolves `v1.3.0` to the same application commit.
  - The installed Python package from the exact `v1.3.0` tag reports version `1.3.0`.
  - GHCR, Pages, and the production state use the exact `v1.3.0` publication.
  Validation:
  - Run `make python-package-install-test`.
  - Run `make ci` after the last application change.
  - Run the STE check on each changed technical document.
  - Run `git diff --check`.

- [x] [B164] (P0) Use one version for each current release surface.
  Goal:
  The application and each official client use one current `v1` release.
  Evidence:
  - Go Proxy selects `v1.2.1` for the official Go module.
  - The deployed application uses the valid `v1.2.1` release identity.
  - The Python client declares version `1.2.0`.
  - The next forward-only patch release is `v1.2.2`.
  - Expected result: each current release surface uses `v1.2.2`.
  - Actual result: current release surfaces use different versions.
  Requirements:
  - Set the selected manifest fixed major to `1`.
  - Validate the exact decision that the gateway transaction prepares or reuses.
  - Require the release decision to match the official client version.
  - Set the Python client version to `1.2.2`.
  - Keep the Go module path unchanged.
  - Publish the current source only as `v1.2.2`.
  - Remove each release identity above major version `1`.
  - Deploy the exact `v1.2.2` publication.
  Validation:
  - Verify that Go Proxy selects `v1.2.2`.
  - Verify that the Python package reports `1.2.2`.
  - Verify that GitHub, GHCR, Pages, and production report `v1.2.2`.
  - Verify that no release tag has a major version above `1`.
  - Run `make ci`.
  Progress:
  - Source and lock metadata now set the Python client version to `1.2.2`.
  - The application-owned validator requires the exact `v1.2.2` decision.
  - The gateway runs the validator for each prepared or reused decision.
  - The selected manifest declares SemVer with fixed major `1`.
  - The gateway contains no LLM Proxy version policy or cutover exception.
  - `make python-package-install-test` passed for version `1.2.2`.
  - `make ci` passed all 11 gates with 100 percent Go coverage.
  - The gateway formatting, lint, and test suites passed.
  - The execution chain committed and pushed the source changes.
  - GitHub, GHCR, Pages, and the deployment selection use `v1.2.2`.
  - The live Pages marker identifies `v1.2.2` and the exact release commit.
  - The API authentication and configuration checks return the expected status.
  - The obsolete v7 GHCR version and Pages tag were removed.
  - GitHub and GHCR contain no release identity above major version `1`.
  - `make ci` passed all 11 gates with 100 percent Go coverage.

- [x] [B163] (P0) {F022} Remove content hashes from the canonical media contract.
  Goal:
  Identify public media through semantic fields and opaque owner identifiers.
  Keep content hashes out of the public media contract. Keep private integrity
  checks for stored bytes and provider file uploads.
  Evidence:
  - Expected: a caller can send current image or audio bytes with a MIME type,
    or refer to a current tenant asset with its opaque `asset_id` and MIME type.
  - Actual: every inline attachment requires a `sha256` field even though its
    bytes are already present in the same request.
  - Actual: every asset reference repeats the asset content hash even though
    `asset_id` is the tenant-scoped identity and the server owns its metadata.
  - Actual: asset upload requires `X-LLM-Proxy-Asset-SHA256`, returns `sha256`,
    and persists that value beside the asset data.
  - Actual: the Go and Python clients calculate SHA-256 for inline attachments
    and uploads. Their asset-reference constructors require the returned hash.
  - Required integrity control: asset resolution must compare stored bytes to
    server-owned integrity metadata before each provider dispatch.
  - Required integrity control: Gemini file validation must compare the
    provider `sha256Hash` value to a server-owned checksum.
  - Creative Director owns a hashless current-state workflow. Its official
    LLM Proxy client dependency still calculates and serializes hashes when it
    sends generated media for semantic QA.
  - The generated-artifact hash is not a provider retrieval handle. It cannot
    identify a current tenant asset or retrieve provider history.
  - F033 made the hash a required identity field. This issue replaces that
    requirement. It does not preserve the obsolete wire or persisted shape.
  Requirements:
  - Define inline media with exactly `type`, `mime_type`, and `data`.
  - Define stored-asset media with exactly `type`, `mime_type`, and `asset_id`.
  - Keep `asset_id` as the opaque tenant-scoped asset identity.
  - Define asset upload with the authenticated request body and exact
    `Content-Type`. Do not accept an artifact-hash header.
  - Return only semantic asset metadata: asset id, MIME type, byte size,
    lifecycle state, creation time, expiry, and deletion time when applicable.
  - Persist the semantic metadata, tenant owner, and one private integrity
    checksum. The checksum is not an asset identity or public field.
  - Validate inline data as nonempty canonical base64. Calculate a private
    checksum from the decoded bytes when a provider integrity check needs it.
  - Resolve an asset by tenant, asset id, MIME type, lifecycle state, expiry,
    and stored byte size. Compare its bytes to the private integrity checksum.
  - Validate a provider file through its provider-owned name or URI, MIME type,
    byte size, lifecycle state, and provider checksum.
  - Remove media-hash fields, parameters, error codes, constructors, examples,
    and prose from the canonical OpenAPI document, generated API reference,
    official clients, root README, and provider routing guide.
  - Reject the obsolete `sha256` attachment field as an unknown field.
  - Reject the obsolete upload hash header as undeclared input.
  - Reject obsolete persisted tenant asset metadata that contains the old
    public `sha256` field. Do not add a dual read, migration bridge, alias,
    optional legacy field, or fallback.
  - Do not change credential-secret digests, structured-request intent
    digests, catalog revision digests, package checksums, or other
    non-artifact security and protocol mechanisms.
  - Make Creative Director use the supplied Go client for capability discovery,
    media construction, request construction, dispatch, and reconciliation.
    Creative Director must not reproduce the client protocol.
  Deliverables:
  - Replace the media and asset wire schemas and regenerate every derived API
    artifact.
  - Replace server parsing and public error mapping with the hashless public
    contract. Keep private asset and provider-file integrity validation.
  - Replace the Go and Python client attachment and asset APIs with semantic
    inputs and responses.
  - Update F022 and F033 text so their current requirements do not require
    artifact hashes.
  - Update the README, provider routing guide, examples, and release notes.
  - Add public capability discovery to the supplied Go client so downstream
    applications need no direct proxy HTTP operation.
  Implementation evidence on 2026-08-29:
  - The server accepts inline media by type, MIME type, and canonical base64
    data. It accepts stored media by type, MIME type, and tenant-scoped asset
    id. Neither public path accepts or returns a content hash.
  - Asset upload calculates and stores a private checksum with the semantic
    metadata. Asset resolution rejects same-length byte replacement before
    provider dispatch.
  - The obsolete upload hash header and old hash-bearing metadata shape are
    rejected.
  - Gemini file acceptance uses provider name and URI, MIME type, byte size,
    lifecycle state, and provider checksum. A checksum mismatch fails closed.
  - The Go and Python client APIs, OpenAPI document, generated API reference,
    README, routing guide, feature contracts, and release notes use the same
    hashless media contract.
  - Public-router regressions prove exact ordered bytes and MIME types reach
    the selected fake provider. They also prove asset and provider checksum
    failures stop before model dispatch.
  - The live media harness emits the exact hashless inline attachment shape.
  - The capability client validates each provider route and its required media
    limit relationships. It rejects incomplete or malformed successful catalogs.
  - The README defines the coordinated media client and server update. It
    includes official client edits, direct HTTP edits, and fake server fixtures.
  - The update procedure stops media traffic and preserves structured request
    journals. It moves version 1 tenant asset files before server start. It
    requires new uploads for retained source media.
  - The breaking changelog entry identifies the incompatible media versions and
    the README update procedure.
  - The new migration prose passed the scoped STE check and `git diff --check`.
    The Governor check reported only the existing M012 and M013 governance work.
  - The post-review `make go-test` run passes with 100.0% Go statement coverage.
  - The post-review final `make ci` run passes all 11 gates in 134 seconds. It
    includes 45 Python tests, 95 browser tests, and 100.0% Go statement coverage.
  Validation:
  - Send inline image and audio attachments through the real `/v2` router and
    prove the selected fake provider receives the exact ordered bytes and MIME
    types without a hash field.
  - Upload an image and audio asset through the real asset route without an
    artifact-hash header. Prove the response contains no artifact hash and the
    persisted metadata contains a private integrity checksum.
  - Send tenant asset references through the real `/v2` router and prove
    tenant ownership, asset id, MIME type, state, expiry, and byte-size checks
    occur before provider dispatch.
  - Prove foreign, missing, expired, deleted, wrong-MIME, malformed-size,
    same-length-corrupted, and obsolete-metadata assets fail safely without
    provider dispatch.
  - Prove `sha256` on an inline or asset attachment and the obsolete upload
    header are rejected at the public boundary.
  - Prove Gemini file acceptance rejects a wrong checksum, provider name or
    URI, MIME type, byte size, or state.
  - Exercise every corrected official Go and Python client path against a fake
    server. Prove no media request or client value contains an artifact hash.
  - Scan production media, asset, provider, client, schema, and documentation
    paths for artifact-hash fields and calculations. Keep documented security
    and protocol exclusions explicit.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.

- [x] [B160] (P1) Reject a production live test for an unexpected tenant.
  Goal:
  `make live-test` must prove the expected production tenant identity before it
  sends paid provider requests.
  Evidence:
  - The command states that `LLM_PROXY_DEFAULT_TENANT_KEY` is the Default tenant client
    secret.
  - `run_client_authentication_preflight` accepts HTTP `400` from an invalid
    request body as sufficient client authentication proof.
  - The preflight does not verify the tenant identity that the client secret
    selects.
  - The 2026-08-29 production run used tenant
    `managed-df47df8e2b434a62b0b1c5e1ca619f11`.
  - The production database identifies that tenant as `Social Threader`.
  - That tenant has OpenAI, Anthropic, and Meta provider connections. It does
    not have Gemini or Moonshot provider connections.
  - The Gemini and Moonshot requests returned HTTP `503` before provider
    execution. Their usage records contain `service_unavailable`.
  - The management interface loaded the Default tenant
    `managed-60634d2470f31557462f52e319f07d07`.
  - The Default tenant has saved Gemini and Moonshot provider connections.
    Their provider profiles select `gemini-3.5-flash` and `kimi-k3`.
  - The failed Moonshot request selected the catalog default `kimi-k2.6`. This
    selection confirms that the request did not load the Default profile.
  - `managedTenantStore.authenticate` finds one tenant by the presented secret
    digest. The database query loads provider connections and profiles for each
    request.
  - `saveProviderConnectionsHandler` verifies a changed connection before
    persistence. It saves the connection under the tenant identifier in the
    management URL.
  - The evidence disproves a missing-key, failed-persistence, decryption, or
    stale-cache diagnosis for the Default tenant.
  Requirements:
  - Require one canonical expected tenant identifier for each production
    live-test invocation.
  - Compare the server-resolved tenant identifier with the expected identifier
    before provider traffic.
  - Stop the command before paid provider traffic when the identifiers differ.
  - Do not use request validation as tenant identity proof.
  - Do not put client secrets or provider credentials in output or stored
    files.
  - Preserve the unchanged provider matrix and request bodies after identity
    verification.
  - Do not rotate or save provider credentials as part of this repair.
  - Keep B128 provider lifecycle work independent from this tenant identity
    defect.
  Deliverables:
  - Add a server-owned authenticated identity contract for a tenant client
    secret.
  - Bind `scripts/live_test.sh` to the expected tenant identifier through that
    contract.
  - Add public black-box coverage for correct and incorrect tenant secrets.
  - Update the live-test operator documentation for the new identity input.
  Validation:
  - Create two managed tenants with valid client secrets and different provider
    connections.
  - Prove that the expected tenant secret passes the identity preflight.
  - Prove that the other valid tenant secret fails before a provider request.
  - Prove that the failed preflight sends zero requests to each fake provider.
  - Prove that the accepted request loads only the expected tenant provider
    profile.
  - Run the production identity preflight with the Default tenant identifier.
  - Run the unchanged production matrix only after the identity preflight
    passes.
  - Correlate each production request with its tenant identifier and I045 phase
    summary.
  - Run `make ci` after the last repository change.
  Production evidence:
  - Release `v1.3.0` deployed application commit
    `7a61fc55f36786086cb887a6f9b91f4450119e8d`.
  - The canonical Default tenant key returned HTTP `200` for tenant
    `managed-60634d2470f31557462f52e319f07d07`. The validated request ID was
    `G44SXGGQTV36CVIAKMSGB24DGZ`.
  - The unchanged production matrix ran only after identity verification. All
    nine cases passed.
  Resolution:
  - The authenticated identity resource and live-test preflight require an
    exact expected tenant identifier before provider traffic.
  - Black-box contracts reject another valid tenant and send no provider
    request after that rejection.
  - Production accepted the canonical Default tenant key and the exact Default
    tenant identifier before all nine provider requests.

- [x] [B128] (P1) Restore Gemini long-completion production acceptance after first-read visibility failure.
  Goal:
  Make the Default tenant's Gemini 3.5 Flash background case complete the
  production live-test contract. Release the corrected pollable-resource
  lifecycle before another acceptance run.
  Completion boundary:
  - Repository changes and repository validation prove development completion.
  - Production acceptance is an explicit completion condition for this issue.
  Evidence:
  - On 2026-08-10, `gemini-echo` returned HTTP `200` with its exact marker, so
    the saved Default-tenant credential and ordinary Gemini route were active.
  - The later `gemini-background-polling` case returned the sanitized provider
    HTTP `429` boundary with 162 response bytes and request id
    `H3VZZRB52HTFOBITJH22NNZ3WR`.
  - On 2026-08-19, production `v3.1.1` accepted the current Default-tenant
    client key. Both exact Gemini cases reached `gemini-3.5-flash`, then
    returned sanitized HTTP `502` responses with 156 response bytes. The
    validated request ids were `NHZT3Z3NJD3MPHO7SKD5T74RCZ` for echo and
    `EM73QSJWEYKXDL4FTHXYGJ5TSF` for background polling.
  - I045 correlated each request with a non-retryable upstream HTTP `403` on
    the first resource poll. The background summary recorded 500 milliseconds
    of provider poll wait and zero proxy rate-limit wait. Resource cleanup also
    failed at the provider boundary.
  - A bounded operator-key control first reproduced the provider boundary.
    Create returned HTTP `200` with `in_progress`. The first read returned HTTP
    `403` with `permission_denied`, and deletion returned HTTP `200`.
  - On 2026-08-21, a second read of that same resource returned HTTP `200` with
    `completed` after 500 milliseconds. The credential and resource id did not
    change.
  - Two later Gemini 3.5 Flash controls waited two seconds after create. The
    first read returned HTTP `200` with `completed`, and deletion returned HTTP
    `200` in both controls.
  - These controls classify the failure as first-read resource visibility, not
    a durable credential permission failure. The checks retained no provider
    resource id or response body.
  - PR #288 deployed as `v4.0.0` from application commit `48adaed`. The runtime
    uses only tenant-managed provider credentials from the retained database.
  - The deployed verifier sends `background: false` and `store: false` for all
    Gemini models. It accepts a synchronous create response without retrieval.
  - The deployed verifier and request path treat the first retrieval HTTP `403`
    as a durable failure. They do not reconcile later visibility.
  Requirements:
  - For a pollable Gemini model, verify the stored background lifecycle.
  - Create and observe one interaction with the candidate key.
  - Cancel the interaction when the retrieved state is active.
  - Delete every stored verification interaction.
  - Use the shared `pollable_resource` visibility contract for the first read.
  - Do each create, transport attempt, later read, cancel, and delete one time.
  - Limit each successful provider response to 1 MiB.
  - Persist the candidate only after all required lifecycle operations succeed.
  - Reject a candidate when the reconciliation read returns HTTP `403`.
  - Preserve the prior key, settings, and defaults after a rejected replacement.
  - Preserve the 900-second production request budget and response redaction.
  Validation:
  - Prove the pollable verification request uses `background: true` and
    `store: true`.
  - Prove successful verification performs create, retrieve, cancel, and delete
    before persistence.
  - Prove a transient first-read HTTP `403` reaches the same resource and lets
    verification complete.
  - Prove a persistent HTTP `403` rejects the candidate and preserves prior
    state.
  - Prove a lost lifecycle response does not retry its provider operation.
  - Prove an oversized lifecycle response rejects the candidate.
  - After the identified boundary is resolved, run the exact Gemini echo and
    background cases with only `LLM_PROXY_DEFAULT_TENANT_KEY`. Prove HTTP `200`, the final
    markers, validated request ids, and no response-body disclosure.
  - For any source change, run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Development resolution:
  - On 2026-08-19, pollable Gemini verification now performs one stored
    background create, one retrieval, cancellation of active work, and deletion.
  - The management transaction starts only after the complete lifecycle succeeds.
  - Regression coverage proves that retrieval HTTP `403` rejects a candidate,
    cleans up the interaction, and preserves the prior credential and settings.
  - Review corrections make each lifecycle operation a single attempt.
  - Review corrections limit each successful lifecycle response to 1 MiB.
  - The review follow-up `make ci` passed all 11 gates with 100% Go statement
    coverage.
  - The required baseline and final `make ci` runs passed all 11 gates with
    100% Go statement coverage.
  - On 2026-08-21, release `v5.0.1` deployed application commit `9e6f585`,
    which contains PR #289.
  - The release receipt records container image
    `sha256:26e83fea6cfa8b207771f6b19bee6743fc1938b4a33d97c48229a94d69e9c72b`.
  - The public release marker identifies the same version and application
    commit.
  - The management configuration health check returned HTTP `200`.
  - The unauthenticated API root returned HTTP `403`.
  - On 2026-08-21, B149 added a provider-neutral first-read visibility
    lifecycle to the development checkout.
  - The existing Authorization key passed local `gemini-3.5-flash` provider
    verification and text smoke checks through the corrected lifecycle.
  - The local checks did not release or deploy the change.
  - On 2026-08-29, production release `v6.1.0` included B149 and accepted the
    expected Default tenant before provider traffic.
  - The current production matrix passed `gemini-echo` and seven other cases.
  - `gemini-background-polling` returned HTTP `502` twice with no upstream
    status or safe terminal code.
  - B162 development now preserves validated terminal codes before resource
    deletion and records each Gemini lifecycle operation in I045.
  - Release `v1.3.0` deployed the B162 correction from application commit
    `7a61fc55f36786086cb887a6f9b91f4450119e8d`.
  - The deployed image uses digest
    `sha256:291b5b3226ce442e4c0a46a9132602e4029e1ae1d849b5b5aad3edf611e58732`.
  - The current private canonical key passed the Default tenant identity
    preflight and the exact Gemini echo case.
  - The exact Gemini background case returned HTTP `200` with validated request
    ID `SPMKDEPJ7PVRNOVVMRZQYPJWQT` and 16,624 response bytes.
  - All nine cases in the production matrix passed without response-body or
    credential disclosure.
  Production resolution:
  - The deployed lifecycle correction completed the exact Gemini background
    case through the canonical Default tenant key.
  - The passing production matrix satisfies this issue's production acceptance
    condition.

- [x] [B141] (P1) Center the X icon inside the top-right square.
  Goal:
  Align the X icon so it is visually centered within the square control in the top-right corner, matching the intended UI layout shown in the attached screenshot.
  
  Requirements:
  Preserve the existing square size, placement, styling, and click/interaction behavior. Adjust only the icon positioning/alignment needed to center the X horizontally and vertically within its container. Ensure the fix remains responsive and does not introduce layout shifts in nearby content.
  
  Deliverables:
  Code changes that correct the X icon alignment in the top-right square control. Include any related style/layout updates needed for consistent centering across supported viewports.
  
  Validation:
  Open the affected screen and confirm the X appears centered within the top-right square. Verify the square remains in the same top-right position, the control still functions as before, and no surrounding UI elements are visually displaced.

  Resolution:
  - The shared `button.icon-only` rule controls icon alignment and padding.
  - All close controls use the same SVG X.
  - The square remains 30 pixels wide and 30 pixels high.
  - Before the correction, browser tests measured a 6.5-pixel horizontal offset in the request dialogs.
  - Eight browser cases passed at widths of 1280 and 390 pixels.
  - These cases include Settings, provider cards, failed requests, and rejected requests.
  - Keyboard focus and the close action passed in each case.
  - `make ci` passed all 12 gates with 100.0% Go statement coverage.
  - The change is local. Production publication remains a separate operation.

## Improvements

- [x] [I030] Document LLM Proxy client authentication and configuration boundaries.
- [x] [I028] Emit LLM Proxy page views to its dedicated GA4 property.
- [x] [I026] {B036} Add provider/model-capability-driven reasoning-effort to tenant routing defaults.
- [x] [I024] Add Qwen 3.8 Token Plan and MiniMax M2.7 providers.
- [x] [I023] Add GLM-5.2 to the existing BigModel/Zhipu catalog.
- [x] [I022] Correct the Moonshot catalog for the Kimi K3 launch.
- [x] [I021] Refresh documented model catalogs for existing providers.
- [x] [I020] Declare LLM Proxy's TAuth tenant requirements in the app-owned deployment manifest.
- [x] [I001] Make missing placeholder handling field-aware.
- [x] [I002] Require API keys only for tenant default providers.
- [x] [I003] Address provider config review followups.
- [x] [I004] Add dynamic live provider smoke tests.
- [x] [I005] Move provider model catalogs into config.yml.
- [x] [I006] Add Grok/xAI and Zhipu dictation support.
- [x] [I007] Make OpenAI dictation URL explicit in static provider config.
- [x] [I008] Add OpenAI base URL to explicit provider config.
- [x] [I009] Make static provider configuration explicit and key-complete.
- [x] [I010] Decouple OpenAI background polling from text worker occupancy.
- [x] [I011] Codify provider default model selection for omitted JSON model fields.
- [x] [I012] Make bundled clients canonical v2-only transports.
- [x] [I013] Limit upstream HTTP call rate in shared HTTP client for text and dictation, without provider‑specific logic.
- [x] [I014] Align the management header avatar with the right edge.
- [x] [I015] Add LLM Proxy icon and favicon assets.
- [x] [I016] Encrypt managed provider API keys at rest.
- [x] [I017] Let Settings request examples fold as one usage segment.
- [x] [I018] Add repo-grounded SEO resource pages.
- [x] [I019] Add LoopAware traffic pixel to all pages of LLM-proxy.
- [x] [I025] Let users reveal and edit their saved provider API keys.


### Complete entries archived 2026-08-10

- [x] [I216] (P0) Make one model-operation capability and pricing catalog authoritative.
  Goal:
  Publish one tenant-safe catalog for every provider-backed model operation.
  Use it for planning, routing, validation, pricing, public discovery, and
  official clients.
  Cross-repository source:
  - Completed MediaOps I068 supplies exact condition matching and reviewed
    pricing records for the bounded import into LLM Proxy.
  Requirements:
  - Extend the normalized I221 catalog with credential kinds, operation kinds,
    pricing, controls, enums, bounds, account-dependent limits, and artifact
    types.
  - Add typed prices with components, currency, units, exact conditions,
    minimum charges, official source, verification date, and an explicit
    unavailable reason.
  - Require exact price-condition matches. Missing, incomplete, or conflicting
    conditions must return a typed unavailable result.
  - Keep observed provider usage as execution evidence separate from published
    pricing and management usage telemetry.
  - Use organization-level canonical provider identifiers at shared credential
    boundaries. Keep `gemini` and `vertex` distinct because they use different
    APIs and credentials. Make `xai` canonical for xAI text and video, migrate
    persisted managed `grok` routes once, and remove the dual selector.
  - Expose one catalog service consumed by the later
    `GET /model/v1/capabilities` handler, planning validation, the public
    catalog, and provider-management choices.
  - Import the stabilized MediaOps pricing data in one bounded migration and
    remove each migrated MediaOps provider record during its family cutover.
  Deliverables:
  - Add the catalog types, strict loader, deterministic public projection,
    exact price selector, one-off xAI route migration, and generated docs.
  - Add catalog revision identifiers that bind plans to the exact capability
    and pricing snapshot used to create them.
  Validation:
  - Prove all accepted operation routes and prices are catalog-backed and that
    unknown fields, duplicate identifiers, unsupported credential/lifecycle
    pairs, and ambiguous prices fail startup.
  - Prove the public projection excludes credentials, private account state,
    provider handles, and tenant defaults.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolved 2026-08-10:
  - Catalog revision `2026-08-10.i216.1` now defines operation artifacts,
    provider credential kinds, exact offerings, controls, limits, and one typed
    price descriptor for every offering operation.
  - Exact condition selection imports the reviewed xAI video rates from
    MediaOps I068 and returns typed unavailable results for every missing or
    incomplete match. Historical managed usage remains separate telemetry.
  - `xai` is the sole current provider identifier. Schema version 6 re-encrypts
    stored `grok` credentials for `xai`, rewrites current text and dictation
    defaults once, and preserves historical `grok` usage unchanged.
  - Public REST data, OpenAPI, generated site content, runtime configuration,
    management choices, and the catalog service share the same validated
    snapshot without provider-native models, credentials, tenant defaults, or
    private account state.
  - The final `make ci` passed all 11 reported gates with 100.0% Go statement
    coverage and 90 browser scenarios.

- [x] [I221] (P0) Make model identity independent from provider offerings.
  Goal:
  Make models the primary public discovery items. Keep each selected model's
  publisher and provider offerings explicit.
  Requirements:
  - Replace `ProviderModelCatalogs` and public `providers[].models` with one
    normalized catalog for model publishers, model families, exact models,
    providers, and provider offerings.
  - Give each exact model one canonical identifier, model publisher, model
    family, version, operation set, and model media capabilities.
  - Give each provider offering one provider, exact model, provider-native
    model identifier, wire contract, lifecycle, limits, defaults, and
    route-specific capabilities.
  - Use a canonical provider and model pair as the external route. Resolve that
    pair to exactly one provider offering.
  - Map each provider-native model identifier only at the provider offering
    boundary.
  - Migrate configured catalogs, public REST data, OpenAPI, management data,
    and stored routing defaults in one forward-only change.
  - Preserve I039's historical `qwencloud` usage as observed execution
    evidence. Do not expose it as a current provider offering.
  - Do not relabel historical `qwencloud` usage as `dashscope` usage.
  - Add one bounded migration that maps each stored provider/model route to its
    canonical provider offering. Remove the obsolete shape after migration.
  - Reject duplicate identifiers, dangling references, route conflicts,
    missing relationships, and invalid route capabilities at startup.
  - Use model publishers as model groups and labels in the public route
    explorer.
  - Open a model picker when the user selects a model publisher. Provide
    exact model search and model family and operation filters.
  - Show a model count for each model publisher. Group exact models by model
    family in the model picker.
  - Show the selected model publisher and exact model in one compact model
    stage.
  - Keep only the selected exact model and its provider offerings in the
    expanded graph.
  - Keep the complete catalog available in semantic HTML and the searchable
    capability table.
  - Render one capability row for each exact model. Show its provider offerings
    within that model row.
  - Report separate model publisher, exact model, provider, and provider
    offering counts.
  - Make the graph compact for model publishers with many model families and
    exact models.
  - Keep provider credential settings provider-first. Derive each provider's
    exact models from its provider offerings.
  - Keep provider keys, base URLs, private handles, tenant defaults, and
    provider-native model identifiers outside public data.
  - Generate every publisher, model, family, and provider option from catalog
    REST data.
  Deliverables:
  - Add normalized catalog types, a strict loader, a deterministic public
    projection, and an exact provider-offering resolver.
  - Update runtime configuration, public REST and OpenAPI contracts,
    management profiles, stored routes, and configuration examples.
  - Add the model picker and provider fan for the selected model to the
    frontend-owned route explorer.
  - Add the bounded migration. Remove the provider-nested catalog structures
    and public fields.
  - Update current documentation and black-box integration tests.
  Validation:
  - Prove that one exact model can have multiple provider offerings without
    duplicate model records.
  - Prove that the capability table shows one row for each exact model and all
    provider offerings for that model.
  - Prove that one model publisher with many exact models remains compact while
    every exact model stays discoverable.
  - Prove that a proprietary model with one provider offering uses the same
    normalized structure.
  - Prove that model publisher, exact model, and provider offering selections
    update one explicit provider/model route.
  - Prove that each valid provider/model route resolves to one offering. Reject
    every unknown or ambiguous pair before execution.
  - Prove that the bounded migration maps every stored route. Require exact
    route context when an invalid route stops startup.
  - Prove semantic no-JavaScript access, keyboard operation, reduced-motion
    behavior, and responsive containment through the public browser entrypoint.
  - Prove that public data excludes credentials, private provider data, tenant
    defaults, and provider-native model identifiers.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolved 2026-08-10:
  - The normalized catalog separates publishers, families, exact models,
    providers, and provider offerings.
  - Schema version 5 maps stored provider-native routes to canonical exact
    model identifiers and preserves historical usage.
  - Public REST, OpenAPI, management profiles, the route explorer, and the
    capability table use the normalized catalog and exclude private provider
    data, native model identifiers, and tenant defaults.
  - The final `make ci` passed all 11 reported gates with 100.0% Go statement
    coverage and 90 browser scenarios.

- [x] [I039] (P0) Retire Qwen Cloud Token Plan and keep DashScope.
  Goal:
  Keep `dashscope` as the only Alibaba provider for application API traffic.
  Remove the interactive `qwencloud` plan from the runtime.
  Evidence:
  - The canonical `qwencloud` provider points at
    `https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1`.
  - Alibaba restricts Token Plan to interactive coding and agent tools. Alibaba
    prohibits its use for automated scripts and application backends:
    https://www.alibabacloud.com/help/en/model-studio/base-url
    https://www.alibabacloud.com/help/en/model-studio/token-plan-overview
    https://www.alibabacloud.com/help/en/model-studio/more-tools
  - Alibaba identifies Model Studio pay-as-you-go as the service for custom
    applications. Its production guidance recommends a workspace domain.
  Decision:
  - P007 selects Alibaba Cloud Model Studio pay-as-you-go under canonical
    provider `dashscope`.
  - Use a Model Studio API key from the same region as its base URL.
  - Use the Singapore workspace URL in production:
    `https://{workspace-id}.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1`.
  - Keep `qwen-plus` and synchronous Chat Completions until I038 or I221 changes
    that exact provider offering.
  Requirements:
  - Remove the `qwencloud` selector, provider registry entry, Token Plan domain,
    configuration fields, environment placeholders, UI option, docs, and tests.
  - Remove `qwencloud` from the live-provider harness and the public provider
    catalog.
  - Configure production `dashscope` with the selected workspace URL. Keep the
    workspace and credential in the same Alibaba region.
  - Replace the tracked DashScope base URL with a required
    `${DASHSCOPE_BASE_URL}` placeholder in production configuration.
  - Bind that value from the private deployment input. Do not store the
    workspace URL in the tracked manifest.
  - Add a bounded schema-version-4 migration for managed `qwencloud` records.
  - Delete each stored `qwencloud` key, selected model, and provider system
    prompt. Do not copy these values to `dashscope`.
  - If a tenant routes text through `qwencloud`, select the first remaining
    keyed provider by canonical provider identifier.
  - Use that provider's stored text model. Clear the route when no provider key
    remains.
  - If migration removes a tenant's only provider key, make Settings mandatory
    again.
  - To continue Alibaba traffic, require affected users to add a Model Studio
    pay-as-you-go key through `dashscope`.
  - Clear an incompatible reasoning effort during the same transaction.
  - Preserve tenant timestamps and historical usage records. Keep each usage
    record's observed provider and model identifiers unchanged.
  - Verify all changed rows before the migration records schema version 4.
  - After migration, reject `qwencloud` in static configuration, managed
    settings, routing defaults, and application model profiles.
  - Do not create a `qwencloud` alias, shared credential, dual route, or runtime
    fallback to `dashscope`.
  - Keep Alibaba console and interactive-tool integration out of scope.
  Validation:
  - Prove through public black-box tests that no Token Plan domain can receive
    an inference request.
  - Prove the schema-version-4 migration with disposable databases for
    `qwencloud`-only and mixed-provider tenants.
  - Prove current-schema startup rejects each obsolete `qwencloud` shape.
  - Prove historical usage keeps its original provider and model identifiers.
  - Prove a `qwencloud`-only tenant cannot route traffic until it saves a valid
    provider key.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolved 2026-08-10:
  - `dashscope` is now the only Alibaba runtime provider and its production
    workspace URL comes from the private deployment binding.
  - Schema version 4 removes current `qwencloud` records, reconciles routes,
    and verifies retained state and historical usage before committing.
  - Static configuration, managed settings, routing defaults, and official
    client profiles reject `qwencloud`.
  - Black-box and migration coverage enforce the forward-only boundary.

- [x] [I222] (P0) Adopt CalVer for product releases.
  Goal:
  Use the canonical UTC CalVer decision for every new llm-proxy release.
  Requirements:
  - Declare CalVer as the single current Gix release scheme.
  - Align current release documentation with the declaration.
  - Preserve the zero-argument release, publication, and deployment lifecycle.
  Validation:
  - Prove Gix selects CalVer for a fixed UTC release timestamp.
  - Prove current repository guidance contains no obsolete release claim.
  - Run the Governor check and `git diff --check` for the `.mprlab/` change.
  Resolved 2026-08-10:
  - `.mprlab/release.yml` now selects CalVer. Current release documentation
    describes the same lifecycle.
  - Gix selected `26.810.235959` for the fixed UTC validation timestamp.
  - The changed documentation passed the scoped STE check. The Governor check
    reported only the existing M012 and M013 governance drift.

- [x] [I220] (P1) {I215,I219} Distribute routing columns across the graph canvas.
  Goal:
  Use the routing tree's connector lanes to distribute its four node columns
  across the available canvas, from the product at the left content edge to
  the active model at the right content edge.
  Evidence:
  - The model group is left-aligned in a flexible final grid track, which
    collects unused space after the model cards at the graph's right edge.
  - Fixed narrow gaps pack the product, proxy, provider, and model columns near
    the graph center instead of letting the measured Bezier connectors absorb
    the available horizontal space.
  Requirements:
  - Align `Your product` to the map's left padded edge and the active model
    group to its right padded edge in the desktop graph.
  - Distribute the available horizontal space through the product-to-proxy,
    proxy-to-provider, and provider-to-model connector lanes.
  - Shorten desktop model cards to the compact provider-column width while
    preserving complete labels and selected-default metadata.
  - Preserve measured connector endpoints, provider/model interactions,
    semantic no-JavaScript output, and responsive containment.
  Validation:
  - Browser geometry proves desktop edge alignment, three positive distributed
    connector lanes, compact model-card width, and selected connector endpoints.
  - Visually inspect the graph at 1280-, 900-, and 390-pixel widths.
  - Run final `timeout -k 350s -s SIGKILL 350s make ci` after the last code
    edit; reuse the current exact-code passing result as the baseline.
  Resolution:
  - Bounded desktop tracks now place the product and active model group on the
    map's left and right padded edges. `space-between` assigns the remaining
    width to the three measured connector lanes, and desktop model cards are
    capped at the provider column's compact 220-pixel width.
  - The browser regression first failed against the previous 280-pixel model
    cards, then passed edge alignment within 1 pixel, connector lanes of at
    least 90 pixels with no more than 16 pixels of spread, and selected Bezier
    endpoint evidence after provider and model changes.
  - Desktop, compact, and mobile captures at 1280, 900, and 390 pixels confirm
    the edge-to-edge desktop distribution and responsive containment.
  - Final `make ci` passed all 11 gates in 117 seconds with 89 frontend browser
    scenarios and 100.0% Go statement coverage.

- [x] [I219] (P1) {F019,I215,F031} Move public-site rendering out of Go.
  Goal:
  Restore the repository's frontend/backend ownership boundary so Go owns
  validated capability data and REST delivery while frontend tooling owns all
  public-site markup, copy, rendering, and interaction structure.
  Evidence:
  - `cmd/cli/site_render.go` currently embeds the routing-tree and capability-
    catalog HTML templates, rewrites HTML attributes and source markers, and
    exposes Pages-rendering flags from the backend CLI.
  - `docker/pages/Dockerfile` builds and invokes that Go UI renderer to create
    the published static artifact.
  Requirements:
  - Expose one sanitized public capability resource through the canonical REST
    API, derived from the same validated provider registry used for request
    routing, with no credentials, tenant state, configured base URLs, or UI
    presentation logic in the backend.
  - Make frontend-owned tooling consume that REST representation and own static
    site copying, production config-URL injection, routing-tree markup,
    capability-catalog markup, user-facing copy, and deterministic Pages output.
  - Preserve the complete generated provider/model catalog, current defaults
    and limits, semantic no-JavaScript output, SEO metadata, local/production
    config-URL profiles, and browser interactions.
  - Remove the Go HTML templates, marker replacement, site-rendering flags, and
    renderer implementation after the frontend path is authoritative. Keep no
    alias, compatibility command, dual renderer, or fallback artifact path.
  - Keep Node and generation dependencies in build/test stages only; the
    published Pages artifact remains static.
  Deliverables:
  - Add the public REST schema and handler, frontend site generator, updated
    Pages Docker build, current documentation, and migrated black-box coverage.
  - Delete the obsolete Go rendering surface and its implementation-specific
    tests.
  Validation:
  - Prove the public REST response exactly reflects changed validated catalog
    data and excludes private configuration through the real HTTP router.
  - Prove frontend generation fails on missing or invalid REST data and emits
    the complete static routing tree, capability catalog, request limits, and
    production config URL without retaining source markers.
  - Prove the generated site remains complete without JavaScript and preserves
    current browser behavior and responsive containment.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolved 2026-08-09:
  - Go now exposes the unauthenticated, secret-free
    `GET /api/public/capabilities` resource from the validated routing registry;
    real-router coverage proves changed catalog data is returned without
    provider credentials, configured base URLs, or tenant secrets.
  - Frontend-owned `scripts/render_public_site.mjs` now fetches that REST
    resource and exclusively owns static copying, config-URL injection,
    routing-tree markup, capability-catalog markup, and public copy. The Go HTML
    templates, marker replacement, renderer flags, and implementation were
    deleted without aliases or a fallback path.
  - The Pages build and local orchestration use the backend REST surface plus a
    build-only Node renderer; both production container targets build, and the
    final Pages image remains static.
  - Frontend black-box coverage proves fail-fast missing/invalid REST behavior,
    complete no-JavaScript output, current interactions, and responsive
    containment. The rendered landing was visually inspected at desktop and
    mobile widths.
  - The 119-second baseline and 112-second post-correction final `make ci` runs
    pass all 11 gates with 100.0% Go statement coverage.

- [x] [I217] (P1) {B123} Split the management application by UI responsibility.
  Goal:
  Replace the misleading key-management controller name and isolate the
  authenticated application's lifecycle, tenant, usage, provider, routing,
  client-access, notification, and presentation responsibilities.
  Requirements:
  - Keep `app.js` as the browser composition root and register one accurately
    named Alpine management-application factory.
  - Move cohesive state and behavior into responsibility-named ES modules;
    keep authentication lifecycle checks out of provider-key modules.
  - Preserve the current tenant, usage, Settings, mutation serialization,
    secret handling, and MPR UI authentication contracts without aliases or
    compatibility exports for the obsolete names.
  - Give the management-application module group a responsibility document and
    update source-evidence references to the modules that own each example.
  Validation:
  - Browser coverage loads the complete renamed module graph through the real
    application entry point and preserves all authenticated UI scenarios.
  - Run the required final
    `timeout -k 350s -s SIGKILL 350s make ci`.
  Resolution:
  - `app.js` now composes `llmProxyManagementApplication` from documented
    lifecycle, tenant, usage, provider-editor, provider-credential,
    provider-settings, routing, client-access, notification, dialog, and
    presentation responsibilities; the obsolete key-management module,
    factory, element, and compatibility names are removed.
  - Generated resource evidence points to `authenticationLifecycle.js` and
    `requestExamples.js`, and the complete application module graph uses the
    bounded `20260809i217` revision.
  - The responsibility-graph browser contract failed first because the new
    modules did not exist, then all 87 frontend scenarios passed after the
    split. Final `make ci` passed all 11 gates in 112 seconds with 100.0% Go
    statement coverage and the TAuth browser black box passing.

- [x] [I215] (P1) {F019,I212,I213} Show the generated provider-to-model routing tree.
  Goal:
  Make the single LLM Proxy connection and its fan-out across current providers
  and exact model versions immediately understandable on the public landing
  page.
  Requirements:
  - Generate every provider leaf, text-model leaf, model count, and
    provider-catalog default from the same validated public capability catalog
    used by routing and the model matrix.
  - Keep providers and models as two distinct selectable levels. Selecting a
    provider must reveal only that provider's exact supported text models and
    select its catalog default; selecting a model must update the final route.
  - Preserve semantic no-JavaScript content, keyboard operation, compact MPR
    styling, responsive geometry, and the existing public catalog above the
    visualization.
  - Do not hardcode provider or model identifiers in landing-page JavaScript or
    expose credentials, base URLs, tenant defaults, or private configuration.
  Validation:
  - Prove renderer output changes with catalog data and contains every current
    provider and text model without retaining either source marker.
  - Prove provider and model selection, default selection, no-JavaScript
    content, and narrow-screen geometry through Playwright.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - Replaced the framed, scrollable selector with the approved open fork: one
    visible product-to-proxy connection, a curved fan to every provider, and a
    second curved fan from the selected provider to a two-column exact-model
    leaf grid.
  - The server renderer still emits all 12 current providers and all 53 current
    text models from the validated public capability catalog. Browser
    enhancement promotes the generated provider with the most model leaves,
    selects its catalog default, and draws every curve from measured generated
    DOM nodes without a provider or model inventory in JavaScript.
  - Provider and model controls remain semantic and keyboard-operable. The
    no-JavaScript artifact keeps the complete generated catalog, while the
    390-pixel layout stacks the same leaves without horizontal overflow.
  - Playwright visually verified the open desktop fork and compact mobile
    composition. Its black-box contract proves all provider leaves remain
    visible without internal scrolling, the selected model leaves use two
    desktop columns, path drawing completes, provider and model selection
    updates the route, and mobile containment holds.
  - The required baseline passed all 11 gates in 114 seconds. The final run
    followed the last code edit and passed all 11 gates in 122 seconds with 86
    browser tests, 36 Python tests, the real TAuth black-box scenario,
    live-provider preflight, and exact 100% Go statement coverage.
  Follow-up resolution:
  - Moved the routing tree directly below the Providers and model capabilities
    heading and before the summary, search, and generated matrix. The hero is
    focused on its primary integration message again.
  - The tree and table now inherit the same 1180-pixel catalog shell. Browser
    coverage proves their left edge, right edge, and width align within one CSS
    pixel at desktop width.
  - Kept the outer canvas full width while capping desktop provider leaves at
    300 pixels and exact-model leaves at 280 pixels. Mobile leaves continue to
    use the available responsive width.
  - Playwright visually verified the full-width fork with compact leaves. The
    correction baseline passed all 11 gates in 121 seconds; the final run after
    the last code edit passed all 11 gates in 125 seconds with 86 browser tests,
    36 Python tests, the real TAuth black-box scenario, live-provider preflight,
    and exact 100% Go statement coverage.
  Second follow-up resolution:
  - Moved the generated routing tree after the complete capability catalog,
    including its summary, search, matrix, and request limits.
  - The tree and table retain the same 1180-pixel catalog shell. Browser
    coverage proves the table precedes the tree and their left edge, right edge,
    and width align within one CSS pixel at desktop width.
  - Provider leaves remain capped at 300 pixels and exact-model leaves at 280
    pixels on desktop. Mobile leaves continue to use the available responsive
    width.
  - Playwright visually verified the catalog-first desktop composition. The
    correction baseline passed all 11 gates in 128 seconds; the final run after
    the last code edit passed all 11 gates in 119 seconds with 86 browser tests,
    36 Python tests, the real TAuth black-box scenario, live-provider preflight,
    and exact 100% Go statement coverage.

- [x] [I214] (P1) {B111,B114} Keep the shared footer sticky on every page.
  Goal:
  Pin the canonical compact MPR footer to the viewport bottom across the
  landing, app, documentation, legal, resource-hub, and resource-article
  routes.
  Requirements:
  - Set the canonical generated footer to the supported sticky state and
    regenerate every HTML route from its maintained source.
  - Retain the component's in-flow host footprint so the final main content
    remains reachable above its fixed hydrated surface.
  - Preserve the identical footer links, semantic no-JavaScript fallback,
    compact responsive geometry, and exact header -> main -> footer order.
  Validation:
  - Static browser coverage proves every public route and `/app/` carries the
    same sticky footer contract.
  - Hydrated browser coverage proves the footer remains fixed to every viewport
    edge, does not cause horizontal overflow, and does not cover the end of
    `main` at desktop and mobile widths.
  - Rebuild and inspect representative routes through
    `http://localhost:4179/`.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - The canonical shell now renders `sticky="true"`, and all 52 landing, app,
    documentation, legal, hub, and resource-article HTML pages were regenerated
    from their maintained sources.
  - Browser coverage now verifies the identical sticky contract across every
    public route and `/app/`, then checks the hydrated footer surface against
    all viewport edges, its in-flow main-content clearance, responsive width,
    and settings-overlay layer at desktop and mobile widths.
  - The rebuilt local stack at `http://localhost:4179/` kept the desktop landing
    footer fixed from 0 to 1280 pixels and the 390-pixel documentation footer
    fixed without internal overflow; both cleared the end of `main` within the
    browser's subpixel tolerance. Mobile visual inspection confirmed the
    wrapped controls remained readable. The local stack was then stopped.
  - The required baseline passed all 11 gates in 101 seconds. The final run
    passed all 11 gates in 109 seconds with 85 browser tests, 36 Python tests,
    the TAuth black-box scenario, live-provider preflight, and exact 100% Go
    statement coverage.

- [x] [I213] (P1) {F019} Clarify default-route badges in the public model catalog.
  Goal:
  Keep model identifiers and their default-route metadata visually distinct and
  make the default behavior understandable without repository knowledge.
  Requirements:
  - Give the model identifier and every default-route badge an explicit,
    responsive gap in generated catalog rows.
  - Replace "Default text" and "Default dictation" with plain labels and
    tooltips that distinguish provider-catalog defaults from account routing
    settings.
  Validation:
  - Site-render and browser scenarios prove the exact labels, explanations, and
    model-to-badge separation at desktop and mobile widths.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - Generated catalog rows now use responsive flex spacing between model ids and
    default-route badges. Desktop keeps an explicit horizontal gap, while narrow
    layouts wrap the badge onto a separately spaced line.
  - Badges now read "Default for text" or "Default for dictation" and explain
    that the marked model is the provider-catalog default while account routing
    settings can select another model. Site-render and Playwright assertions
    cover both labels, tooltips, and desktop/mobile spacing.
  - The required baseline passed all 11 gates in 92 seconds; the final post-edit
    run passed all 11 gates in 96 seconds with 82 browser tests, 36 Python tests,
    the TAuth black-box scenario, live-provider preflight, and exact 100% Go
    statement coverage.
- [x] [I212] (P1) {F019} Center the public site on one integration across supported models.
  Goal:
  Make "Integrate once. Use the model that fits." the primary public promise,
  with official clients and direct HTTP presented before the capability matrix.
  Requirements:
  - Reorder and rewrite the landing page so the stable integration contract is
    primary, the Go, Python, CLI, and direct HTTP surfaces are prominent, and
    the generated capability matrix remains the current proof of supported
    routes.
  - Give AI-assisted builders, startups and product teams, and institutional
    platform or engineering teams distinct crawlable paths into existing
    high-value resources without generating thin audience-swap pages.
  - Revamp the resource hub and the multi-provider, native-provider comparison,
    and internal-gateway cornerstone pages from current repository evidence.
    Keep generation, canonical metadata, dated significant updates, author
    attribution, sitemap integration, and source-backed examples deterministic.
  - Describe only the current validated support contract. Do not publish an
    uptime guarantee, provider-longevity term, or model-onboarding target before
    the separate SLA policy defines those commitments.
  Validation:
  - Browser scenarios prove the new message hierarchy, integration and audience
    paths, current model catalog, responsive layout, and crawlable resource
    navigation.
  - Generator and SEO evaluation prove factual integrity, differentiated page
    intent, repository evidence, author attribution, metadata, and indexing
    readiness.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - The landing page now leads with one stable integration across supported text
    routes, presents direct HTTP and the official Go, Python, and CLI clients
    before the generated capability matrix, and gives each target audience a
    distinct path into substantive resources.
  - The generated resource hub and three cornerstone guides now carry current,
    source-backed examples, visible author attribution, canonical metadata,
    significant-update dates, and sitemap discovery. The independent SEO
    evaluation passed every binding threshold with factual integrity at 5/5.
  - Future provider-lifecycle, model-onboarding, and hosted uptime commitments
    remain in P006 pending an approved measurable policy. The required baseline
    and final CI runs passed all 11 gates; the final run completed in 90 seconds
    with 82 browser tests, 36 Python tests, the TAuth black-box scenario,
    live-provider preflight, and exact 100% Go statement coverage.
- [x] [I211] (P1) {F019} Keep execution lifecycle internal to the public capability catalog.
  Goal:
  Present the model matrix as a catalog of abilities that callers can select or
  use, while llm-proxy owns how each provider request reaches its final response.
  Requirements:
  - Remove synchronous and background execution from public model capability
    badges, filter pills, search metadata, capability counts, and capability-sort
    counts.
  - Keep the internal `execution_lifecycle` model contract, validation, provider
    routing, polling coordinator, timeout behavior, and blocking client response
    contract unchanged.
  - Retain the complete generated provider/model matrix and its user-actionable
    text, dictation, image input, audio input, web search, and reasoning
    capabilities with no-JavaScript access.
  Validation:
  - Public catalog projection tests prove lifecycle identifiers are absent while
    internal routing tests retain their lifecycle assertions.
  - Site-render and Playwright scenarios prove six filterable capabilities,
    lifecycle-free rows and search metadata, and unchanged catalog interaction.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - The public catalog now projects only text generation, dictation, image
    input, audio input, web search, and reasoning. Synchronous and background
    execution no longer appear in badges, filters, row metadata, search, or
    capability-sort counts.
  - Internal execution-lifecycle configuration, validation, upstream polling,
    routing, timeouts, and the blocking client response contract remain intact.
  - Catalog projection, CLI rendering, and Playwright coverage reject public
    lifecycle capabilities and prove the complete six-filter production
    matrix. The required baseline passed all 11 gates in 90 seconds; the final
    post-edit run passed all 11 gates in 109 seconds with 82 browser tests, 36
    Python tests, the TAuth black-box scenario, live-provider preflight, and
    exact 100% Go statement coverage.
- [x] [I209] (P1) {F019} Streamline capability catalog search and table sorting.
  Goal:
  Replace the multi-control capability toolbar with the compact search-first
  interaction established by Kamu while keeping the generated model matrix
  accessible and complete.
  Requirements:
  - Keep one unified search field that matches every published model
    characteristic: provider, model, defaults, capability labels and
    identifiers, wire contract, reasoning efforts, lifecycle, and output limit.
  - Remove the provider and sort dropdowns. Expand the capability-filter pill
    row when search starts or the search icon is activated.
  - Move sorting into accessible Provider, Model, and Capabilities table-header
    controls with visible direction state and deterministic tie-breaking.
  - Preserve match-all capability filtering, live result count, reset,
    capability-badge activation, the complete no-JavaScript matrix, and compact
    responsive MPR styling.
  Validation:
  - Add site-render CLI and Playwright coverage for the search-first disclosure,
    all-characteristics matching, pill filters, sortable headers, keyboard
    behavior, reset, no-JavaScript rendering, and mobile containment.
  - Verify the rendered landing in Chromium at desktop and mobile widths.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - The matrix now has one all-characteristics search field. Starting a search
    or activating its magnifying-glass control discloses the compact match-all
    capability-pill row, live count, and reset action.
  - Provider, Model, and Capabilities headers own accessible ascending and
    descending sorting with visible direction state and deterministic ties;
    the standalone provider and sort dropdowns are removed.
  - CLI rendering and Playwright cover no-JavaScript completeness, provider,
    model, default, capability identifier and label, contract, reasoning,
    lifecycle, output-limit searches, pill activation, reset, keyboard use,
    sorting, and mobile containment. Chromium verification passed at 1280 by
    800 and 390 by 780 without document overflow.
  - The required baseline passed all 11 gates in 96 seconds. The final
    post-edit run passed all 11 gates in 92 seconds with 82 browser tests, 36
    Python tests, the TAuth black-box scenario, live-provider preflight, and
    exact 100% Go statement coverage.
- [x] [I208] (P1) {F019} Make the public capability catalog model-centric and filterable.
  Goal:
  Replace the split text/dictation presentation with one compact model matrix
  that lets visitors compare the exact capabilities of every supported route.
  Requirements:
  - Project text and dictation models into one deterministic, secret-free public
    model capability contract derived only from the validated provider registry.
  - Render exactly Provider, Model, and Capabilities columns. Represent text,
    dictation, media input, web search, reasoning, lifecycle, wire contract,
    defaults, and output limits as clear model metadata instead of publishing a
    dedicated dictation column.
  - Add accessible search, provider, capability, and sort controls. Capability
    filters use match-all semantics, update a live result count, expose a reset
    action, and retain the complete crawlable matrix without JavaScript.
  - Use the compact MPR public-site language: thin controls, dense bordered
    rows, restrained semantic badges, and responsive behavior.
  - Replace the obsolete split public projection and presentation rather than
    retaining aliases, dual shapes, or compatibility markup.
  Validation:
  - Add Go integration coverage through the site-render CLI for the unified
    catalog and browser coverage for sorting, filtering, reset, accessibility,
    responsive layout, and the no-JavaScript matrix.
  - Verify the rendered local landing in Chromium at desktop and mobile widths.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution 2026-08-06:
  - The secret-free public registry projection now unifies text and dictation
    routes into one deterministic provider/model contract with exact
    capabilities, defaults, wire contract, reasoning efforts, and output limit.
  - The landing renders all 12 providers, 58 models, and 8 filterable
    capabilities in exactly Provider, Model, and Capabilities columns. Search,
    provider selection, match-all capability filters, sorting, live count, and
    reset progressively enhance the complete no-JavaScript table.
  - Chromium verification passed at 1440 by 1000 and 390 by 844, including a
    live Gemini plus Image input filter. The baseline `make ci` passed in 90
    seconds; the final post-edit run passed all 11 gates in 95 seconds with 82
    browser tests and 100 percent Go statement coverage.
- [x] [I204] (P0) Adopt the app-owned resource and sibling-gateway lifecycle.
  Goal:
  Make llm-proxy independently releasable and deployable through the shared
  `mprlab-gateway` orchestrator without retaining a second production
  controller inside this repository.
  Requirements:
  - Keep exactly one tracked production declaration at
    `.mprlab/deploy/resources.yml`, using schema version 2 for the llm-proxy
    runtime, retained data, HTTP capability, public route and health checks,
    GitHub Pages site, and TAuth tenant.
  - Keep `make release`, `make publish`, and `make deploy` as thin entrypoints
    into the exact sibling `../mprlab-gateway`, with this checkout passed as the
    selected app. Do not discover an installed binary, controller bundle,
    alternate gateway path, or unrelated repository.
  - Remove application-owned production Ansible, Compose, Caddy, release,
    publish, and deploy implementation. Local and black-box development
    orchestration remains application-owned.
  - Render the Pages artifact from committed source with the Go CLI. Keep Node
    only for developer frontend validation; declare no Node or npm production
    resource.
  Validation:
  - Black-box Go coverage validates the exact resource declaration, sibling
    lifecycle entrypoint, forbidden production paths, and Go-only Pages build.
  - The final `make ci` passes on the complete tracked change.
  - The sibling gateway accepts a clean committed checkout with
    `make plan-app-release MPRLAB_APP_ROOT=<llm-proxy checkout>` without
    publishing or deploying.
  Resolved 2026-07-30:
  - Commit `0749142` replaces schema-v1 and app-owned production machinery with
    the schema-v2 declaration, exact sibling lifecycle wrapper, and Go-only
    Pages container. Node remains only in developer lint and browser tests.
  - The complete final `make ci` passes with 100% Go statement coverage, 33
    Python tests, 75 browser tests, the TAuth black-box test, and the live
    provider harness preflight.
  - Gateway commit `6e26e3e` accepts the clean app commit through
    `plan-app-release`, validates every committed source input and declared
    resource, and derives only the `tauth.tenants` runtime requirement. No
    release, publication, deployment, or unrelated-repository scan runs.
- [x] [I042] (P1) Remove managed-request serialization from SQLite authentication.
  Goal:
  Keep SQLite as the sole managed-tenant source of truth while allowing each
  proxy request to wait only for its own database read and selected provider,
  rather than another request's usage write or management mutation.
  Requirements:
  - Open the canonical runtime GORM SQLite database in WAL mode with a bounded
    busy timeout. Keep injected dialectors test-only and add no application
    cache, replica, dual read, or invalidation path.
  - Propagate the caller context into managed authentication and load each
    tenant plus its provider settings through one consistent GORM read
    transaction.
  - Remove authentication and single-event usage persistence from the
    process-wide management mutation lock. Keep multi-statement management
    changes atomic through their existing GORM transactions.
  - Preserve the blocking public response, secret-digest comparison,
    provider-key decryption, routing-default, usage, and migration contracts.
  - Document SQLite/GORM concurrency ownership in the canonical management
    persistence guidance.
  Validation:
  - Public HTTP coverage using a disposable runtime SQLite database proves WAL
    mode permits managed authentication and upstream routing while another
    connection holds an exclusive write transaction.
  - Concurrency coverage proves an in-flight managed usage write cannot block
    authentication for an independent request.
  - The required post-change `make ci` passes after the final code edit.
  Resolved 2026-07-27:
  - Runtime managed SQLite connections now use WAL journaling and a five-second
    busy timeout. Authentication uses the caller context and one read-only GORM
    transaction, while authentication and single usage inserts bypass the
    management mutation mutex without adding an application cache.
  - Public HTTP coverage reaches the selected provider while another connection
    holds an exclusive SQLite writer, and deterministic store coverage proves a
    blocked usage insert cannot delay independent authentication.
  - The final `make ci` passed with exact 100% Go statement coverage, all Python
    and frontend tests, the TAuth black-box test, release tests, and the live
    provider harness preflight.
- [x] [I037] (P1) Model provider wire contracts separately from execution lifecycles.
  Goal:
  Let each configured text model use its provider's exact current request shape
  and execution lifecycle without inferring OpenAI Responses semantics from an
  endpoint name, an SDK, or the presence of an upstream identifier.
  Evidence:
  - The registry currently collapses wire format and execution behavior into
    four transport constants: OpenAI Responses, OpenAI-compatible Chat
    Completions, Gemini `generateContent`, and Anthropic Messages.
  - OpenAI Responses is a pollable background resource; Gemini Interactions is
    independently pollable; xAI Responses is synchronous even though it exposes
    stored response IDs; and several SDKs call ordinary concurrent HTTP methods
    `async` without creating a server-side job.
  - The shared output-continuation coordinator starts a new inference request
    after output-limit exhaustion. That is distinct from observing one
    in-progress upstream request and must not be described or implemented as
    polling.
  - The provider-by-provider audit produced these no-migration conclusions:
    - OpenAI already uses its native pollable Responses background resource;
      move its remaining router special case behind this shared capability
      model rather than opening a separate provider refactor:
      https://developers.openai.com/api/docs/guides/background
    - DeepSeek, Moonshot, MiniMax, SiliconFlow, and Zhipu each document an
      interactive synchronous or streaming Chat Completions contract. Keep
      those canonical paths; do not turn response IDs, SDK concurrency, media
      task APIs, or offline Batch APIs into text polling:
      https://api-docs.deepseek.com/api/create-chat-completion
      https://platform.kimi.ai/docs/api/chat
      https://platform.minimax.io/docs/api-reference/text-chat-openai
      https://docs.siliconflow.com/en/userguide/capabilities/text-generation
      https://docs.bigmodel.cn/api-reference/%E6%A8%A1%E5%9E%8B-api/%E5%AF%B9%E8%AF%9D%E8%A1%A5%E5%85%A8
    - Anthropic's canonical interactive inference surface remains Messages.
      Message Batches and Managed Agents change workload and product semantics,
      so neither belongs in the blocking proxy completion path:
      https://platform.claude.com/docs/en/api/messages/create
      https://platform.claude.com/docs/en/managed-agents/overview
    - Meta remains on the currently evidenced synchronous Chat contract. Its
      authoritative model and Chat references are authentication-gated, so no
      newer lifecycle may be proposed until accessible first-party evidence
      exists:
      https://dev.meta.ai/docs/features/chat-completion
  - Only DashScope, Qwen Cloud, Gemini, and Grok produced independently
    actionable provider work; their issues follow this prerequisite.
  Requirements:
  - Replace the combined transport enum with closed, validated provider/model
    capabilities for wire contract and execution lifecycle. Active lifecycle
    variants must distinguish synchronous completion from reusable pollable
    resources; if a one-read deferred result is ever adopted, give it a
    separate exact variant rather than reusing the pollable-resource contract.
    Do not use `supports_responses`, `background`, or similarly ambiguous
    booleans.
  - Resolve capabilities at the model boundary when a provider supports a newer
    API for only part of its catalog. Each registered model has one canonical
    current path; do not add runtime fallback from a newer API to Chat
    Completions.
  - Keep the public `GET /`, `POST /`, and `POST /v2` contract blocking. An
    adapter may create and observe an upstream resource internally, but callers
    continue to receive one final proxy response or one terminal proxy error.
  - Keep output-limit continuation as a separate coordinator concern.
    Continuation may consume a provider-native continuation primitive only when
    its semantics are documented; it must never poll an arbitrary response ID.
  - Make each adapter own its exact request fields, terminal states, result
    retrieval rules, cancellation/deletion behavior, usage extraction, and
    safe error translation. Offline batch products and managed-agent sessions
    are not interactive lifecycle variants.
  - Move OpenAI's existing background-resource behavior behind the new
    capability model as part of this issue. Record the audited no-migration
    providers in the registry and provider-routing documentation without
    creating replacement wire adapters for them.
  - Document the capability matrix and data-retention consequences in the
    canonical provider-routing guide and generated API reference. Delete the
    obsolete combined transport contract in the same forward-only change.
  Validation:
  - Black-box routing coverage enumerates every registered provider/model and
    fails when its wire contract or lifecycle is absent, contradictory, or
    inferred from another field.
  - Public-boundary fixtures prove synchronous, pollable, continuation,
    cancellation, timeout, and provider-terminal-error behavior without
    exposing upstream IDs or provider bodies.
  Resolution:
  - Every configured text model now declares one closed `wire_contract` and one
    closed `execution_lifecycle`. Startup rejects absent, unknown,
    provider-incompatible, contradictory, or dictation-scoped declarations;
    no route capability is inferred from a provider, URL, request profile, or
    upstream identifier.
  - Model-owned route adapters now select OpenAI Responses polling,
    OpenAI-compatible Chat Completions, Gemini generateContent, or Anthropic
    Messages. Provider-key verification uses the selected model's wire
    contract, and the obsolete provider-level combined transport enum is gone.
  - The checked-in catalog records OpenAI as `pollable_resource` and every
    audited no-migration provider as `synchronous_completion`. Public routing
    coverage enumerates every configured provider/model, while existing public
    lifecycle fixtures continue to prove continuation, cancellation, timeout,
    terminal errors, safe responses, and blocking callers.
  - README, the canonical provider-routing guide, and OpenAPI now publish the
    capability matrix, continuation separation, and exact upstream storage,
    cancellation, deletion, and retention consequences.
- [x] [I040] (P1) {I037,B087} Migrate Gemini from generateContent to Interactions resources.
  Goal:
  Adopt Google's recommended current Gemini interface and use its real
  background interaction lifecycle for models that support it.
  Evidence:
  - Google states that the Interactions API is GA, recommended for new
    projects, and the home for future Gemini capabilities:
    https://ai.google.dev/gemini-api/docs/interactions-overview
  - `background: true` creates an interaction that can be retrieved, cancelled,
    or deleted; statuses include `in_progress`, `requires_action`, `completed`,
    `failed`, and `cancelled`:
    https://ai.google.dev/gemini-api/docs/background-execution
  Requirements:
  - Verify Interactions and background support for every configured Gemini
    model and register capability per model. Migrate eligible models from
    `generateContent` to one native Interactions adapter with the required API
    revision; do not try Interactions and fall back at runtime.
  - Create background interactions, poll only while the documented status is
    active, extract final text and complete usage, and translate every terminal
    state. Treat unexpected `requires_action` as a stable unsupported-action
    error until the public proxy contract deliberately supports tool handoff.
  - Define and document storage, deletion, and cancellation behavior. Delete or
    cancel provider resources when the request completes or the caller leaves,
    subject to Google's documented lifecycle.
  - Keep the public proxy request blocking and keep output-limit continuation
    separate from observing the original interaction.
  Validation:
  - Public fixtures cover immediate and delayed completion, terminal failure,
    cancellation, deletion, `requires_action`, usage, and safe errors.
  - The Default-tenant Gemini echo and complex live cases run through
    Interactions and prove actual upstream polling.
  Resolution:
  - Paid Google boundary checks recorded background `in_progress` support for
    `gemini-3.5-flash`, `gemini-3.1-pro-preview`, `gemini-3-flash-preview`, and
    `gemini-3.1-flash-lite`. Each configured 2.5 model returned HTTP 400
    `INVALID_ARGUMENT` with `Model '<model-id>' does not support background
    interactions.`; a synchronous non-stored 2.5 Interaction completed without
    an id.
  - One `gemini_interactions` adapter now uses the exact model-owned lifecycle:
    3.x creates stored background interactions, polls only `queued` and
    `in_progress`, cancels active resources, and deletes every resource through
    independent bounded cancel and delete contexts. Gemini 2.5 sends
    `background: false` and `store: false` and requires an immediate terminal
    result. Both use `Api-Revision: 2026-05-20`, normalized complete usage, safe
    terminal errors, and distinct output-limit continuation calls.
  - Public black-box fixtures cover synchronous id-less completion, delayed and
    immediate background completion, usage including thought tokens, every
    terminal status, cancellation, cancel/delete ordering, independent cleanup
    contexts, cleanup failures, media shape, continuation, and credential
    verification. The production live suite pins its complex Gemini case to
    `gemini-3.5-flash` while the echo retains the saved Default-tenant model.
  - Paid branch acceptance passed for both `gemini-2.5-flash` and
    `gemini-3.5-flash`. The final `make ci` passed all 11 gates with 100.0% Go
    statement coverage; deployment and the post-deploy production invocation
    remain operator-owned.
- [x] [I036] (P1) {F014,B081} Verify pasted provider API keys before persisting them.
  Goal:
  Make a provider connected and routing-eligible only after LLM Proxy
  automatically proves that a newly supplied credential is operational for
  the exact selected provider and text model.
  Evidence:
  - The Settings API-key input currently marks the provider draft dirty on
    input and submits it only on change, provider switch, or Settings close. A
    paste has no immediate verification state or provider request.
  - `PUT /api/management/tenants/:tenant_id/provider-keys/:provider` currently
    validates the body, provider, and model, then encrypts and persists any
    nonblank key without contacting the selected provider. The real-router
    suite proves that an arbitrary short value such as `skhort` returns `200`
    and masked saved-key state.
  - A successful save sets `providers[].has_key`, makes that provider eligible
    for routing defaults, and may establish the tenant's first default route.
    An unusable credential can therefore appear connected until the user's
    first real proxy request fails upstream.
  Requirements:
  - Treat every nonempty `api_key` submitted to the existing provider-settings
    operation as an unverified new or replacement credential. Verify it
    server-side before any provider-key, provider-settings, or routing-default
    database mutation. An empty `api_key` remains the exact retain-existing-key
    operation for model or system-prompt updates and does not reverify the
    stored credential.
  - Add one provider-neutral verification boundary covering every canonical
    provider. Each provider adapter must perform its exact documented,
    authenticated, non-user-content operation and report success only when the
    supplied credential is accepted and the selected text model is available
    to it. Key shape, encryption success, catalog membership, a global
    credential, or another provider must never count as verification.
  - A paste into the selected provider's API-key field must start verification
    automatically without waiting for blur, Settings close, provider switch,
    or a separate Verify/Save action. Any non-paste replacement submitted
    through the canonical operation receives the same server-side verification
    guarantee.
  - Show one explicit `Verifying key` pending state and lock conflicting
    provider, tenant, model, reveal, remove, and Settings-close actions until
    that attempt settles. A newer paste, authentication reset, tenant switch,
    provider switch, model change, or editor replacement must cancel or
    invalidate the prior attempt so a stale result cannot mutate or render in
    the new context.
  - On verification success, encrypt and persist the credential together with
    the submitted provider model and system prompt, reconcile routing defaults,
    and return the complete profile in one atomic mutation. Only that response
    may set `has_key`, unlock mandatory setup, or expose the provider in routing
    selectors. Clear the raw pasted value from browser state and return to the
    existing masked-key presentation.
  - On verification rejection, keep a new provider unkeyed. For a failed
    replacement, retain the prior verified encrypted credential, provider
    settings, and routing defaults unchanged. Keep the rejected draft available
    only in the current editor for correction or retry, and state visibly
    whether no key was saved or the previous key remains active.
  - Distinguish a provider credential/model rejection from an unconfirmed
    timeout, rate limit, or provider outage through stable, documented,
    provider-neutral management errors. None of those outcomes may save the
    candidate key. Never return, persist, log, or render the key, authenticated
    URL, raw provider body, probe response, prompt, or provider-specific
    free-form error.
  - Run verification under the request context and the existing shared
    upstream admission and origin-rate-limit boundaries. Make exactly one
    documented verification operation per submitted candidate; add no hidden
    retry, alternate endpoint, generation fallback, background continuation,
    or deferred save. Verification attempts do not create managed usage events.
  - Update the canonical OpenAPI source and generated reference, README,
    provider-routing documentation, frontend types and copy, and CHANGELOG in
    the same implementation.
  Deliverables:
  - One provider-neutral operational credential verifier with exact adapters
    for all canonical providers, wired into the existing authenticated
    provider-settings mutation before its database transaction.
  - Automatic paste-triggered verification with explicit pending, success,
    rejection, transient-failure, retry, and stale-response behavior in
    Settings.
  - Canonical and generated documentation for the verify-before-persist
    contract and its safe failure statuses.
  Validation:
  - Exercise every canonical provider through the real management router and
    its actual transport shape against controlled upstream servers. Prove an
    accepted credential/model pair performs exactly one verification operation
    and only then returns a keyed profile and eligible defaults.
  - For every transport family, prove authentication rejection, model-access
    rejection, rate limiting, upstream failure, timeout, malformed success, and
    cancellation leave the database and routing defaults unchanged and expose
    only the documented safe management error.
  - Prove a rejected first key leaves mandatory setup locked, while a rejected
    replacement leaves the prior key operational and selected defaults
    unchanged. Prove no verification attempt creates a managed usage event or
    leaks candidate material through responses, logs, profile data, or the DOM.
  - Add rendered Playwright coverage showing that paste starts verification
    before blur, displays and announces the pending state, locks conflicting
    actions, applies success once, retains a failed draft for retry, and rejects
    stale completions after every tenant/provider/model/auth context change.
  - Extend the opt-in live-provider harness to verify each available real key
    through the same operational verifier before its provider smoke request;
    keep paid live calls and secrets outside `make ci`.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair for the implementation, with
    the final run after the last code edit.
  Resolved 2026-07-27:
  - Added one provider-neutral, single-operation verifier for every canonical
    transport and made the authenticated provider-settings mutation verify
    before atomically persisting keys, settings, or routing defaults.
  - Added automatic paste verification with accessible pending and safe-failure
    states, locked conflicting actions, retry, raw-draft cleanup on success, and
    stale-attempt rejection across every editor context boundary.
  - Controlled real-router coverage proves all 12 providers, transport-family
    failures, exact one-operation admission, unchanged state on failure, safe
    responses and logs, and no managed usage. The rendered suite passes all 75
    scenarios; OpenAPI, generated docs, the real auth black-box, and the
    live-provider harness were updated.
  - The required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` runs passed with the Go coverage
    gate at 100%.
  - Disposable paid runs verified the supplied OpenAI, Gemini, Anthropic, and
    Muse11 credentials and their smoke requests (`200`/`200`). The supplied
    Kimi credential authenticated successfully, the former configured model
    was absent and rejected, and cataloged `kimi-k2.6` verified and completed
    (`200`/`200`); the production default-route repair was separately tracked
    by B087.
  - Follow-up 2026-07-28: removed the unavailable former Moonshot model from the
    current catalog and promoted verified `kimi-k2.6` to the canonical default
    without an alias or fallback. The disposable managed-key verification and
    default-model smoke request both returned `200`.
- [x] [I043] (P1) Persist managed usage through one bounded asynchronous writer.
  Goal:
  Keep selected proxy responses independent from managed usage database
  latency without creating one goroutine per response or an unbounded
  telemetry backlog.
  Evidence:
  - Every managed proxy request currently flushes its selected response and
    then performs `recordUsage` synchronously on the request goroutine under a
    detached five-second persistence budget.
  - `recordUsage` takes the process-wide managed-store write lock before its
    database insert, so persistence work retains the handler and can contend
    with unrelated managed authentication and mutation traffic.
  - Managed usage powers operational dashboards and failure inspection; it is
    not a billing, accounting, or provider-job ledger.
  Requirements:
  - Give each management runtime exactly one bounded FIFO usage channel and one
    writer goroutine. Add a positive `management.usage_queue_size` setting with
    a documented default; do not reuse the upstream provider queue.
  - After selecting and flushing a managed response, construct one immutable
    usage event and attempt a non-blocking enqueue. A successful enqueue
    returns immediately and never waits for the database operation.
  - Drain accepted events in FIFO order and attempt each database insert once
    under the existing detached five-second persistence budget. Log database
    failures with safe request metadata; do not retry or start per-event
    goroutines.
  - When the channel is full, drop the newest event, retain every previously
    accepted event, emit one stable `managed_usage_queue_full` warning, and
    leave the selected proxy response unchanged.
  - Document the exact durability contract: accepted events are process-local,
    at-most-once work until their database insert commits. Queue contents are
    not crash-durable, and database failures or process termination can lose
    uncommitted events.
  - Keep prompts, responses, audio, transcripts, secrets, raw provider bodies,
    and free-form upstream errors out of both queued events and logs.
  Validation:
  - Public HTTP coverage blocks the first usage insert, proves later managed
    responses still complete, fills the one-slot test queue, and proves the
    newest event is dropped without changing its response.
  - The same coverage releases persistence and proves the accepted events are
    stored once in FIFO order while the overflow emits exactly one safe stable
    warning.
  - Configuration coverage proves omitted queue size receives the default and
    non-positive explicit values are rejected at the configuration edge.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair for the implementation, with
    the final run after the last code edit.
  Resolution:
  - Managed responses now flush before a non-blocking send to one bounded,
    runtime-owned FIFO writer. Accepted records receive one detached insert
    attempt; saturation drops only the newest record with the stable safe
    `managed_usage_queue_full` warning.
  - A context-aware database-write gate sequences the writer with management
    mutations without taking the management mutation mutex; authentication
    bypasses both and retains the I042 caller-scoped read path.
  - Added the positive `management.usage_queue_size` contract, explicit
    process-local at-most-once durability documentation, and public HTTP
    coverage for response independence, FIFO persistence, overflow, and safe
    logging.
- [x] [I034] (P2) {B079} Minimize Settings system-prompt editors by default.
  Goal:
  Keep tenant-wide and provider-specific system prompts out of the dense
  Settings layout until a user explicitly asks to edit one.
  Requirements:
  - Render each system-prompt editor as a semantic disclosure that is collapsed
    whenever Settings opens and whenever its tenant or provider context changes.
  - Make the visible System prompt label activate the disclosure through pointer
    and keyboard input, and show an explicit visible indicator while the field
    is hidden.
  - Preserve the existing values, disabled states, serialized mutation
    behavior, and autosave-on-field-exit contract.
  Validation:
  - Exercise both disclosures through the rendered browser, including initial
    hidden state, pointer and keyboard expansion, context-reset behavior, and
    autosave after editing an expanded field.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair, with the final run after the
    last tracked edit.
  Resolved 2026-07-26:
  - Tenant-wide and provider-specific prompts now use collapsed semantic
    disclosures with visible `Hidden` and `Expanded` indicators.
  - Settings-open, tenant-switch, and provider-switch resets preserve values,
    disabled states, focus behavior, and autosave-on-field-exit semantics.
  - All 67 rendered-browser scenarios and the real TAuth management flow pass.
- [x] [I033] (P2) {B076,I029} Keep the visible Usage Overview automatically fresh.
  Decision 2026-07-30:
  Retired before implementation. F017 addresses the unattended stale-session
  symptom through shared MPR UI inactivity warning and logout. Keep the
  explicit Refresh action for active authenticated sessions; do not add the
  proposed polling scheduler, last-updated state, or visibility-triggered Usage
  requests. Record a new issue with fresh evidence if foreground Usage
  freshness remains necessary after F017 is deployed.
- [x] [I031] (P1) {I029,F014} Add tenant-scoped failure details to the usage dashboard.
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
  - Run the real SQLite migration path with historical events;
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
  Resolved 2026-07-25:
  - Added the owner-only tenant failure operation with strict interval, limit,
    and cursor inputs; stable snapshot pagination; exact safe row fields; and
    indistinguishable missing/foreign-tenant behavior.
  - Added canonical outcome recording at the managed request boundary and the
    transactional version-2 SQLite migration with exact historical
    normalization, unsupported-status rollback, and the current failure-query
    index.
  - Added the selected-period failed-request action and accessible details
    dialog with status context, safe labels, pagination, responsive behavior,
    retry and stale-response protection, while keeping administrators
    aggregate-only. Updated the canonical OpenAPI contract and generated,
    repository, routing, and public usage documentation.
  - The required pre-change and post-change `make ci` runs pass. The final run
    followed the last code edit and passed static analysis, exact 100% Go
    coverage, real SQLite migration checks, 33 Python tests, package
    installation, 63 browser scenarios, the TAuth black-box test, release
    checks, and the live-provider harness preflight.
- [x] [I029] (P1) {B069,F014} Publish one canonical OpenAPI contract and enforce server/client conformance.
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
  Dependencies:
  - B069 must settle the final public per-request timeout and attributable
    error boundary before this issue freezes its request/response
    documentation and client conformance expectations.
  - F014 replaces the singular management endpoints with tenant-scoped routes.
    This contract must describe only that final management surface, rather than
    documenting a shape that F014 immediately removes.
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
  Resolved 2026-07-25:
  - Added `docs/openapi.yaml` as the sole OpenAPI 3.1 source for all 18 owned
    operations, with exact production server, authentication, request,
    response, status, header, content-type, and `/v2` reasoning contracts.
  - Added bidirectional real-router inventory and representative HTTP exchange
    conformance, plus request/response coverage from the Go package, Python
    package, and Go CLI against the same artifact.
  - Derived the human API reference and resource-page wire inventory from the
    contract, and made the Pages build publish the committed schema
    byte-for-byte with provenance and tamper/drift rejection.
  - The required pre-change and post-change `make ci` runs pass. The final run
    followed the last code edit and passed static analysis, exact 100% Go
    coverage, SQLite checks, 33 Python tests, package installation,
    60 browser scenarios, the TAuth black-box test, release checks, and the
    live-provider harness preflight. Live publication verification remains
    user-owned after production deployment.
- [x] [I206] (P0) Carry provider-neutral image and audio attachments on the canonical messages API.
  Goal:
  Let application-owned clients send exact current media bytes through the
  standard `POST /v2` messages contract without adding product-specific
  endpoints, schemas, prompts, policies, or provider dependencies to the
  generic proxy.
  Requirements:
  - Extend only canonical `POST /v2` messages with optional ordered image and
    audio attachments on user messages. Keep compatibility `POST /`, `GET /`,
    and `/dictate` on their existing distinct contracts.
  - Define one provider-neutral attachment wire shape with an exact media type,
    canonical MIME type, canonical base64 bytes, and matching lowercase SHA-256
    digest. Preserve message order and attachment order exactly.
  - Extend the official Go client with constructor-only immutable image and
    audio attachment values. Constructors must copy and hash caller bytes;
    callers must not be able to construct a zero-but-invalid attachment.
  - Validate external attachment data exactly once at the HTTP edge. Reject
    malformed, empty, noncanonical, hash-mismatched, unsupported-role,
    unsupported-MIME, oversized, or unsupported model-route media before any
    upstream call.
  - Declare media-input capabilities on exact model catalog entries and map
    validated attachments only through provider adapters that implement those
    capabilities. Provider selection remains configuration; no public
    OpenAI-specific field may enter the canonical request.
  - Never echo media bytes in response metadata, persist them in managed usage,
    or expose them in provider errors.
  Deliverables:
  - Canonical `/v2`, model-catalog, official Go client, and provider-adapter
    implementation for image and audio attachments.
  - Updated OpenAPI, README, model-capability table, and provider-routing
    contract with the exact current behavior and limits.
  Validation:
  - Add black-box public HTTP scenarios proving exact ordered image and audio
    bytes reach a capable provider adapter and unsupported media makes zero
    upstream calls.
  - Add public Go-client scenarios proving constructor immutability, canonical
    serialization, exact ordering, and rejection of every invalid attachment
    state.
  - Cover startup rejection for invalid or adapter-incompatible model media
    declarations and response metadata that omits encoded media.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair, with the final run after
    the last code edit.
  Resolution:
  - Canonical `POST /v2` now accepts exact ordered image and audio attachments
    on user messages through one provider-neutral hash-bound wire contract;
    compatibility routes remain text-only.
  - The official Go client owns immutable constructor-only media values, and
    the proxy independently validates their canonical bytes, digest, role,
    MIME type, resolved-model capability, and bounded request size before
    upstream admission.
  - Exact configured Gemini models declare image/audio input support and map
    validated media to native ordered `inlineData` parts. Other adapters and
    undeclared exact models remain fail-closed and provider selection stays
    configuration-owned.
  - OpenAPI, generated API reference, README capability documentation, and the
    provider-routing contract describe the current standard API. Public-client
    and black-box router scenarios cover immutability, ordering, malformed and
    unsupported rejection, zero upstream work, and non-echoed media.
  - Review follow-up makes the canonical OpenAPI schema reject mismatched
    attachment type/MIME pairs and attachments on non-user messages. The
    optional query `web_search` parameter now accepts only exact `true` or
    `false`; aliases and malformed supplied values fail at the HTTP boundary.
  - The breaking query migration is published in the changelog, README,
    OpenAPI, provider-routing guide, and generated public resource. Go package,
    Go CLI, and Python contract tests prove native JSON booleans, while the
    Python constructor rejects non-boolean runtime values before HTTP.


- [x] [I045] (P1) Correlate proxy phase latency and provider progress.
  Goal:
  Make a slow or timed-out request diagnosable without exposing request or
  response content. Distinguish authentication, proxy admission, rate-limit
  waiting, provider work, polling, continuation, formatting, and post-response
  usage enqueue time under the same proxy-owned request id.
  Evidence:
  - The request logger records only total response latency. Managed usage stores
    only the same end-to-end latency, so neither surface identifies which
    proxy-owned phase consumed a request's budget.
  - The shared upstream limiter emits origin-level rate-limit delay logs, but
    those events are not a complete request timeline and do not expose ordinary
    admission wait or aggregate provider HTTP time.
  - OpenAI's background loop polls until a terminal state without emitting a
    content-free poll count, provider state, elapsed time, or output-size
    progress event. The provider-neutral continuation coordinator likewise
    accumulates output across attempts without attempt or accumulated-byte
    telemetry.
  - B088 has reproducible full-budget OpenAI and Meta failures. The production
    live harness reports only case, provider, HTTP status, and response size,
    even though B089 already returns a safe request id that could correlate the
    failed case with structured server evidence.
  Requirements:
  - Define one centralized structured telemetry contract keyed by the existing
    proxy request id. A terminal request summary must carry endpoint, canonical
    provider and model, effective request budget, total latency, and explicit
    millisecond totals for authentication, upstream admission, upstream
    rate-limit waiting, provider HTTP work, provider poll waiting,
    continuation waiting, response formatting, and managed-usage enqueue.
    Phases not entered use zero; omit no phase and do not infer one phase by
    subtracting unrelated totals.
  - Emit content-free provider progress for every OpenAI create/poll lifecycle
    and every provider-neutral continuation attempt. Include attempt or poll
    count, normalized provider state or completion signal, elapsed
    milliseconds, current output bytes, and accumulated output bytes. Do not
    log upstream response ids, prompts, messages, generated text, provider
    bodies, headers beyond already-sanitized metadata, credentials, or tenant
    secrets.
  - Use monotonic in-process timing and one request-scoped accumulator rather
    than reconstructing phases from independent log timestamps. Preserve the
    existing request budget and cancellation ownership; telemetry must not add
    retries, polling, goroutines, blocking persistence, or timeout inflation.
  - Keep structured logs as the observability boundary. Do not add phase fields
    to managed usage persistence, public response bodies, OpenAPI schemas, or
    bundled client models. The existing `X-LLM-Proxy-Request-ID` remains the
    sole public correlation value.
  - Make `make live-test` print the validated proxy request id from the response
    header on every passed or failed HTTP case while continuing to suppress the
    tenant secret and response body. A transport failure with no response
    reports no invented id.
  - Keep the README command summary aligned with the current production target:
    all five echo cases plus OpenAI, Anthropic, Meta, and Gemini long-completion
    cases.
    Document the phase and progress field meanings in the canonical provider
    routing guidance without claiming billing accuracy or provider-side
    execution time outside observed HTTP boundaries.
  Deliverables:
  - One request-scoped phase accumulator, centralized safe log event and field
    constants, OpenAI polling and shared-continuation progress events, and a
    terminal phase summary for every accepted proxy request.
  - Request-id correlation in the production live harness plus updated README
    and provider-routing documentation.
  - No persistent schema change, public payload expansion, upstream identifier
    disclosure, or content-bearing telemetry.
  Validation:
  - Drive real public proxy handlers against controlled upstream servers and
    assert exact phase summaries for success, queue wait, configured rate-limit
    delay, provider failure, caller cancellation, and proxy-budget expiry.
  - Cover OpenAI `queued` and `in_progress` polling through completion and
    provider-neutral output-limit continuation through multiple attempts.
    Prove counts, normalized states, elapsed values, current bytes, accumulated
    bytes, and terminal totals belong to the same request id.
  - Exercise the production live-test script through its fake-curl boundary and
    prove it reports validated response request ids without printing secrets or
    bodies, and reports no fabricated id for a transport failure.
  - Assert that prompts, messages, generated output, upstream response ids,
    provider bodies, credentials, cookies, and tenant secrets are absent from
    every new event and command output.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair for the implementation, with
    the final run after the last code edit.
  Resolution:
  Added request-scoped monotonic phase and progress telemetry, correlated
  validated request ids across the current nine-case live-test harness, and
  documented the safe structured-log contract. Public-handler and fake-curl
  coverage passed; the baseline and final 11-gate `make ci` runs passed with
  100.0% Go statement coverage.
- [x] [I205] (P2) Let's align the text inside the card to the left so it's on the same vertical line as the left of the title, let's align both the text of the card, such as goal etc to the left so it's visually aligned with the ttitle of the card.
  ![image](images/1785478561383_image.png)
  Resolved 2026-08-10:
  - The screenshot shows the ISSUES.md editor issue card. That interface is
    owned by the ISSUES.md repository and is outside the LLM Proxy product
    contract. The request is retired from this repository's active backlog.

### Complete entries archived 2026-08-30

- [x] [I236] (P1) Add live-provider acceptance to local Compose.
  Goal:
  Route paid provider acceptance through the current local Compose API before
  production deployment.
  Requirements:
  - Start the current local Compose services in one isolated test project.
  - Use temporary management and TAuth volumes for each test run.
  - Load provider credentials through the current live-test environment file.
  - Save each selected provider connection through the local management API.
  - Send each selected smoke request through the local `/v2` API.
  - Do not start a host proxy process for this acceptance path.
  - Remove all test containers, networks, volumes, and temporary files at exit.
  - Keep `make up`, `make down`, and production `make live-test` unchanged.
  - Keep paid local acceptance outside `make ci`.
  Validation:
  - Prove the command uses the current Docker image and local Compose contract.
  - Prove provider verification occurs before each smoke request.
  - Prove a failed run removes all test orchestration state.
  - Prove the command rejects a non-loopback API origin.
  - Run one registered Gemini matrix through the local Compose API.
  - Run `make ci` after the last application change.
  Resolution:
  - Added isolated projects with the `llm-proxy-live-test-` prefix and a unique
    random suffix. Each project uses allocated loopback ports, temporary
    volumes, current-checkout image builds, and complete cleanup.
  - Added a local-origin mode that saves and verifies provider connections
    through the Dockerized management API before canonical `POST /v2` smoke
    requests. This mode does not start a host proxy process.
  - Proved HTTP 200 Gemini requests for `gemini-3-flash-preview` and
    `gemini-3.5-flash`. The current catalog publishes no reasoning levels for
    these routes, so the matrix used the omitted-effort case for each model.
  - Proved that the test project retained no containers, networks, or volumes.
  - Passed `make ci` with all 11 gates and 100.0% Go statement coverage.
  Review resolution:
  - Cleared every catalog provider field before the filtered file reload.
  - Made `LIVE_ENV_FILE` the only provider-value source when it is set.
  - Preserved the scoped local management session during the filtered reload.
  - Added black-box coverage for inherited keys and old Compose project state.
  - A real Docker run ignored an invalid inherited Gemini key.
  - The run authenticated locally and passed the first Gemini smoke request.
  - It also passed `gemini-3.5-flash` verification before Google returned 429.
  - Cleanup left no test containers, networks, or volumes.
  - Passed the final `make ci` with all 11 gates and 100.0% coverage.

- [x] [I232] (P1) Remove two Gemini model routes.
  Goal:
  Keep only the selected Gemini exact models in the current provider catalog.
  Evidence:
  - `gemini-3.1-flash-lite` passed provider verification but failed its live
    smoke request with proxy HTTP `502` and upstream HTTP `400`.
  - `gemini-3.1-pro-preview` returned HTTP `429` during two provider
    verification attempts. Its live smoke request did not start.
  Requirements:
  - Remove both exact models and their provider offerings.
  - Migrate each stored selection to `gemini-3.5-flash` in one bounded
    operation.
  - Preserve provider connections, system prompts, timestamps, and historical
    usage records.
  - Reject both removed models after the migration.
  - Remove both models from public documentation and generated capability
    data.
  Validation:
  - Prove that public capabilities omit both removed models.
  - Prove the stored profile and same-provider default migration for both
    removed models.
  - Run `make ci` after the last repository change.
  Resolution:
  - The canonical catalog now contains Gemini 3.5 Flash and Gemini 3 Flash
    Preview. Public capabilities contain 62 models and 63 offerings.
  - Managed schema version 11 atomically replaces both removed selections
    with Gemini 3.5 Flash. It preserves provider connections, system prompts,
    timestamps, and historical usage records.
  - Public routing rejects both removed model IDs before provider dispatch.
    The management profile and generated public resources omit both models.
  - Changed files include the provider catalog, managed-store migration,
    public and migration regressions, current documentation, the resource
    generator, and its generated Gemini pages. No event contract changed.
  - `make go-test` passed with 100.0 percent Go statement coverage.
    `make frontend-test` passed all 95 browser scenarios. The final `make ci`
    passed all 11 gates in 134 seconds with 100.0 percent Go coverage.

- [x] [I231] (P1) Remove Gemini 2.5 model routes.
  Goal:
  Keep only Gemini 3.x exact models in the provider catalog.
  Requirements:
  - Remove all Gemini 2.5 exact models and provider offerings.
  - Remove the unused synchronous Gemini provider transport.
  - Make `gemini-3.5-flash` the Gemini default text model.
  - Migrate each stored Gemini 2.5 provider profile and routing default to
    `gemini-3.5-flash` once.
  - Reject each retired model after the migration. Do not add an alias or a
    fallback.
  - Keep historical usage model identifiers unchanged.
  - Update public capabilities, generated resources, examples, and tests.
  Validation:
  - Prove that startup and public capabilities contain only Gemini 3.x models.
  - Prove that the migration is atomic and maps each retired model.
  - Prove that each retired model fails before an upstream request.
  - Run `make ci` after the last repository change.
  Resolution:
  - The catalog contains four Gemini 3.x models and one pollable Gemini
    transport. Gemini 3.5 Flash is the provider default.
  - Managed schema version 10 atomically replaces each stored Gemini 2.5 text
    selection with Gemini 3.5 Flash.
  - Public routing rejects each retired model before an upstream request.
  - Public capabilities, generated resources, examples, and tests use the
    four-model Gemini 3.x catalog.
  - The focused `make go-test` passed with 100.0 percent Go statement and block
    coverage.

- [x] [I230] (P0) Use the permanent versionless selected application manifest.
  Goal:
  The selected application manifest uses one permanent contract without a
  schema number.
  Requirements:
  - Remove `schema_version` from `.mprlab/deploy/resources.yml`.
  - Require only `owner`, `release`, and `resources` at the manifest root.
  - Reject each numbered selected application manifest form.
  - Keep independent schema contracts unchanged.
  - Keep local orchestration separate from the production manifest.
  Validation:
  - Run `make ci` after the last repository change.
  - Run the gateway `plan-app-release` target against the committed branch.

  Resolution:
  - The selected application manifest now has no schema number.
  - The compiled lifecycle test rejects a `schema_version` field.
  - Current operator documents describe the permanent versionless contract.
  - The final repository validation passed.
  - Hosted GitHub `Test` attempt 1 ended with exit 137 when the workflow's
    outer 350-second timeout stopped gate 10 after gates 1 through 9 passed.
    Attempt 2 completed successfully at `2026-08-20T23:19:58Z`.
  - Gateway commit `753c727` planned release for application commit
    `490acd7` with 218 tasks passed, 4 changed, and 0 failed.

- [x] [I229] (P0) Add schema-constrained, resumable semantic-review requests.
  Goal:
  Let Creative Director submit one semantic-review inference with a declared
  JSON Schema. Reconcile the durable request after a caller transport failure
  without another paid provider submission.
  Requirements:
  - Extend canonical `POST /v2` with one optional `structured_output` object
    containing the exact caller-owned JSON Schema. Require one valid
    `Idempotency-Key` header when that object is present, and reject either
    input when supplied alone.
  - Validate the schema before provider dispatch. Route it only through exact
    provider adapters with a current structured-output wire mapping. Reject an
    unsupported route before upstream work. Do not use prompt-only JSON or a
    repair request.
  - Map the schema to OpenAI Responses `text.format`, Gemini Interactions
    `response_format`, and Anthropic Messages `output_config.format`. Validate
    the terminal provider text against the same schema before accepting it.
  - Persist the tenant-bound idempotency intent before dispatch. Record exact
    `not_dispatched`, `dispatched`, `succeeded`, `failed`, and `uncertain`
    states with atomic mode-0600 records under retained server storage.
  - Repeating one key and identical intent must replay success or report an
    active or uncertain state without a second provider call. A known failed
    state can start an explicit new attempt. A different intent must return a
    conflict.
  - Add an authenticated `GET /v2/requests` reconciliation operation keyed by
    the same header. Return the stored JSON result, visible in-flight timing,
    a terminal safe failure, or an explicit uncertain outcome. A restart must
    classify an interrupted dispatch as uncertain rather than resubmit it.
  - Keep request timeout and transport policy out of the idempotency intent.
    Keep tenant secrets, provider credentials, prompts, schemas, results, and
    raw provider bodies out of URLs, logs, usage records, and status metadata.
  - Extend the official Go client with validated structured-output and
    reconciliation types. Update Creative Director to use its deterministic
    semantic request id and decision schema and to recover a completed result
    from its existing durable review-progress record.
  Deliverables:
  - Strict OpenAPI, server state machine, provider adapter, Go client, and
    Creative Director integration changes.
  - Updated README, routing notes, release notes, and focused fake-provider and
    restart/reconciliation coverage.
  Validation:
  - Prove exact provider payloads and one-call success for OpenAI, Gemini, and
    Anthropic. Prove unsupported routes fail before dispatch.
  - Prove duplicate convergence, intent conflict, tenant isolation, restart
    uncertainty, successful result replay, invalid provider output rejection,
    and secret/content-safe status responses.
  - Prove Creative Director no longer makes a second inference for malformed
    structured output and can bind a reconciled success to the current report.
  - Run the required pre-change and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair, plus Smith repository CI.

  Resolution:
  - `POST /v2` now validates the caller JSON Schema before dispatch. The exact
    provider adapters map it to OpenAI Responses, Gemini Interactions, and
    Anthropic Messages structured-output fields.
  - Tenant-bound mode-0600 records persist request intent and the five durable
    states. Replays converge without a second provider call, conflicting
    intent fails, and interrupted dispatch becomes `uncertain` after restart.
  - Authenticated `GET /v2/requests` returns safe progress, result, failure, or
    uncertainty data. It does not expose prompts, schemas, credentials, or raw
    provider bodies.
  - Review hardening removes interrupted atomic-write temporary records and
    expires known terminal records during lookup or repeated submission.
  - Reconciliation responses now use `Cache-Control: no-store`. Caller
    cancellation cannot replace a saved known success with uncertainty.
  - The Go client returns a typed pending error for HTTP `202`. Provider-specific
    subset admission rejects unsupported schemas before provider dispatch.
  - The Go and Python clients expose validated structured requests. Creative
    Director persists visible review progress and reconciles the same request
    identity without a repair inference.
  - Fake-provider tests cover exact provider payloads, one-call success,
    unsupported routes, replay, conflicts, tenant isolation, restart
    uncertainty, invalid output, and safe status data.
  - The review-correction final bounded `make ci` passed all 11 gates, including
    94 browser scenarios, with 100.0% Go statement coverage. Smith Creative
    Director CI and the no-spend Kamu pipeline integration also passed.
  - Validation made no paid provider call and performed no release,
    publication, or deployment.

- [x] [I224] (P0) {F035} Add a paid live matrix for provider image routes.
  Goal:
  Add one repository command that proves each current provider image route
  accepts a canonical image request through LLM Proxy.
  Requirements:
  - Add an explicit image mode to the disposable live-provider harness.
  - Run the image matrix for OpenAI, Anthropic, Gemini, and xAI by default.
  - Verify each provider key before its image request.
  - Select each image model from the validated public provider catalog.
  - Use the configured provider default when that model supports image input.
  - Otherwise, require one exact image model for that provider.
  - Send one deterministic inline PNG through canonical `POST /v2`.
  - Require HTTP `200` and the exact expected response marker.
  - Keep provider keys, tenant secrets, image data, and response bodies private.
  - Keep paid provider requests outside `make ci`.
  Deliverables:
  - Add a Make target for the four-provider image matrix.
  - Add fake-boundary coverage for model selection, request order, and payload.
  - Document the paid command and its environment requirements.
  Validation:
  - Run the focused live-harness contract tests.
  - Run the non-paid live-provider harness preflight.
  - Run `make ci` after the last application change.
  Resolution:
  - Added one catalog-selected image mode and its Make target.
  - Added exact payload, request order, redaction, and rejection tests.
  - On 2026-08-11, the paid OpenAI, Anthropic, and Gemini image cases returned
    HTTP `200`.
  - The xAI credential was found in the private MediaOps environment, but xAI
    rejected `grok-4.5` verification with HTTP `422`. No image request ran.
  - `make ci` passed all 11 gates with 93 browser tests and 100.0% Go statement
    coverage.

- [x] [I223] (P0) {I216,I221} Load all supported providers and models from one catalog file.
  Goal:
  Make `configs/providers.yml` the only source that defines supported providers,
  exact models, provider offerings, operations, controls, limits, and prices.
  The application loads this file at startup and builds one validated provider
  catalog. Provider onboarding changes only this file when current protocol
  adapters can represent the complete provider contract.
  Evidence:
  - The current catalog has 11 providers, 66 exact models, 67 provider
    offerings, and 67 price records.
  - Provider data also exists in Go types, registry branches, persistence,
    management APIs, UI forms, environment variables, and live-test lists.
  - Current provider offerings use six request protocols and two execution
    lifecycles.
  Requirements:
  - Inventory every datum that defines a provider, exact model, or provider
    offering.
  - Map each datum to one schema field or one reusable protocol adapter.
  - Record the complete mapping in the provider catalog documentation.
  - Add one strict, versioned schema for `configs/providers.yml`.
  - Use root records for `schema_version`, `operations`, `publishers`,
    `families`, `models`, and `providers`.
  - Keep each exact model independent from its provider offerings.
  - Put each provider offering inside its provider definition.
  - Reference each exact model from a provider offering by its canonical
    identifier.
  - Define each provider identifier, display label, aliases, credential fields,
    setting fields, and transports.
  - Define each provider field's identifier, label, type, requirement, default,
    secrecy, validation rules, and environment binding.
  - Define each transport endpoint rule, authentication rule, request protocol,
    response protocol, usage mapping, and lifecycle.
  - Define protocol parameters for token fields, output fields, finish rules,
    continuation rules, error rules, and usage fields.
  - Define each provider offering's upstream model, operations, default state,
    capabilities, controls, limits, request profile, and price.
  - Put each price record inside its provider offering.
  - Define optional environment bindings for each credential field and setting
    field.
  - Keep credential values and tenant setting values outside the provider
    catalog.
  - Support these existing protocol adapter identifiers:
    - `openai_responses`
    - `openai_chat_completions`
    - `anthropic_messages`
    - `gemini_interactions`
    - `multipart_transcription`
    - `xai_videos_generations`
  - Support `synchronous_completion` and `pollable_resource` as catalog
    lifecycle values.
  - Reject unknown fields and unsupported schema versions during startup.
  - Reject duplicate identifiers, alias collisions, missing references,
    unsupported protocols, and invalid numeric bounds.
  - Require exactly one default provider offering for each supported provider
    operation.
  - Require one valid price for each declared provider offering operation.
  - Validate each control, limit, media declaration, and request profile against
    its selected protocol adapter.
  - Load the provider catalog once before runtime configuration validation.
  - Compile one immutable registry from the validated provider catalog.
  - Use this registry for routing, key verification, management APIs, public
    capabilities, UI generation, persistence validation, and live tests.
  - Select protocol adapters only by catalog identifiers.
  - Keep provider identifiers out of generic protocol dispatch.
  - When no current protocol adapter can implement a required protocol, add one
    new protocol adapter.
  - Resolve declared environment bindings through one generic configuration
    loader.
  - After the provider catalog replaces them, remove provider blocks from
    `configs/config.yml`.
  - Store provider connection values by provider identifier and catalog field
    identifier.
  - Encrypt each secret provider connection value with the current encryption
    boundary.
  - Add one bounded migration from provider-specific columns to provider
    connection records.
  - Read only provider connection records after the migration.
  - Return generic provider field definitions and connection state from the
    management API.
  - Render management provider forms from the returned field definitions.
  - Publish only safe exact model and provider offering data through public
    capability resources.
  - Keep credentials, private settings, authentication bindings, and upstream
    model identifiers out of public data.
  - Derive live-provider discovery and environment checks from the provider
    catalog.
  Deliverables:
  - Add `configs/providers.yml` with all 11 providers, 66 exact models, 67
    provider offerings, and 67 price records.
  - Add the schema types, parser, semantic validator, immutable registry, and
    safe data projections.
  - Replace provider-specific runtime, management, persistence, UI, routing,
    configuration, and test-discovery paths with generic consumers.
  - Add the bounded provider connection migration.
  - Document the schema and the one-definition provider onboarding procedure.
  - Remove obsolete provider-specific fields, registry maps, UI branches,
    database columns, and live-test lists.
  Validation:
  - When no satisfactory baseline result applies, run `make ci` before
    application changes.
  - Prove exact catalog counts for 11 providers, 66 exact models, 67 provider
    offerings, and 67 price records.
  - Prove startup rejection for each invalid schema and reference condition.
  - Add a test provider definition that uses an existing protocol adapter.
  - Prove the test provider appears in routing, management, UI schema,
    persistence, public capabilities, and live-test discovery.
  - Prove key verification and request routing against fake upstream servers.
  - Prove provider connection migration and encrypted round-trip behavior.
  - Prove public data contains no private provider catalog data.
  - Prove controls and limits at each exact boundary.
  - Prove the test provider requires no provider-specific production source
    change.
  - After the last application change, run `make ci`.
  Resolution:
  - Added strict schema version 1 in `configs/providers.yml`. It defines 11
    providers, 66 exact models, 67 offerings, and 67 prices.
  - Catalog validation now compiles one immutable registry. Routing, key
    verification, management, persistence, public capabilities, browser forms,
    environment loading, and live discovery use this registry.
  - Added generic encrypted provider connection records and the bounded schema
    version 9 migration. Current reads no longer use provider-specific columns.
  - A synthetic provider proves onboarding through an existing adapter without
    production provider code.
  - Updated documentation, OpenAPI, and generated public resources. The final
    `make ci` passed all 11 gates with 100.0% Go statement coverage.

### Complete entries archived 2026-09-08

- [x] [I253] (P2) Add provider and model logos to management and the public catalog.
  Goal: Implement the logo presentation from P013 on both surfaces.
  Requirements:
  - Use local SVG assets and one shared provider and family manifest.
  - Preserve API connection identities and model family identities.
  - Keep SenseVoice text-only and retain the current Moonshot identity.
  - Validate mappings, asset files, source digests, and disabled catalog identities during the build.
  - Preserve visible labels, accessible controls, and narrow layouts.
  Validation: Start with failing browser tests. Run focused browser and asset checks, then final `make ci`.
  Results:
  - Added 18 local SVG assets with a retained license and pinned source digests.
  - The shared manifest covers 13 providers and 29 families, including disabled candidates.
  - Both management card faces and family labels show the selected logos.
  - The public renderer adds icons to route labels and model matrix labels.
  - SenseVoice remains text-only. Existing service and model identities remain intact.
  - Initial browser tests failed on missing icons before the application changes.
  - The focused browser and renderer suite passed 27 tests.
  - The final icon suite passed 11 tests, including invalid build inputs and 390px layouts.
  - Frontend lint and the Governor check passed. Changed prose has no mechanical language findings.
  - Initial CI found a missing validator in the dependency test fixture: `MODULE_NOT_FOUND`.
  - Updated the fixture and added dependency coverage for `make check-brand-icons`. The dependency contract target passed.
  - Full browser validation found a 5.3px card header offset and two obsolete HTML assertions.
  - Centered the card header items and updated the assertions to require generated family icons.
  - All 13 focused correction tests passed. Final `make ci` passed all 12 gates with 100.0% Go statement coverage.
  - Full CI passed 110 browser tests, the Pages artifact check, and the management authentication test.
  - Updated files: brand modules, SVG assets, styles, both HTML entry points, renderer, validator, browser tests, Makefile, README, and icon documentation.
  - No API field or event contract changed.
  Resolution: Both surfaces show the selected local logos. Repository validation passed. The changes remain local and uncommitted.

- [x] [I252] (P1) Qualify the current Gemini candidates on Vertex AI.
  Goal:
  Establish whether Vertex resolves the current Google quota and request failures before the provider migration.
  Requirements:
  - Verify the existing LLM Proxy Cloud project, billing state, enabled APIs, and available credentials.
  - Use the global Vertex endpoint with exact model IDs and the standard Google Cloud credential flow.
  - Verify Gemini 3.1 Pro Preview, Gemini 3.8 Flash, and Gemini 3.5 Flash-Lite.
  - Run bounded reasoning, structured output, image, and audio requests when model access permits them.
  - Record exact provider errors and successful responses without credentials or private input.
  - Specify the transport and runtime credential changes that the evidence supports.
  Validation:
  - Separate direct Vertex qualification from proxy integration and production acceptance.
  - Run the applicable document and repository checks after the evidence update.
  Resolution:
  - Enabled the Vertex API on the existing billed `llm-proxy-499919` project.
  - Passed 22 direct requests with exact expected output and HTTP 200 on September 7, 2026.
  - Pro and Flash each passed seven cases. Flash-Lite passed eight cases.
  - The cases covered all declared reasoning levels, structured output, image input, and audio input.
  - The requests used local Google Cloud user credentials and the global `v1 generateContent` endpoint.
  - Recorded credential-free results in `docs/evidence/vertex-gemini-2026-09-07.json`.
  - Recorded the interpretation and F060 migration requirements in `docs/vertex-gemini-qualification.md`.
  - The Governor check and `git diff --check` passed. The language check found no errors in the changed prose.
  - Proxy routing, runtime identity, and production acceptance remain separate from this direct qualification.
  Changed:
  - `docs/vertex-gemini-qualification.md`, its JSON evidence, and `docs/gemini-current-models.md`.
  - I252 and F060. No public API or event contract changed.

- [x] [I251] (P1) Diagnose Gemini candidate acceptance failures with safe provider details.
  Goal:
  Identify the causes of I234 and F047 failures through the current acceptance command.
  Requirements:
  - Report provider error details without credentials, request content, or generated output.
  - Preserve failure exit status and required acceptance checks.
  - Reproduce the failures and record the provider evidence.
  - Correct confirmed repository defects through separate BugFix issues.
  Validation:
  - Start with a failing CLI test for diagnostics and private data exclusion.
  - Run focused checks, live diagnosis, and final CI.
  Implementation:
  - Failed candidate requests report bounded provider error fields, quota violations, and retry metadata.
  - Diagnostics exclude credentials, request input, generated output, quota dimensions, and unrelated headers.
  - The original failure exit status remains intact.
  - I234 has zero free-tier quota. F047 has a request-quota failure and the separate B197 transport defect.
  Validation:
  - The initial CLI test failed because `RESOURCE_EXHAUSTED` was absent.
  - Focused diagnostics, privacy, malformed-response, and shell-contract checks pass.
  - CI first rejected Go formatting, then rejected a heredoc in the diagnostics invocation.
  - Both corrections passed focused checks. Final CI passed all 12 gates with 100.0% Go coverage in 245 seconds.
  - Evidence: `/tmp/llm-proxy-i251-red.log` and `/tmp/llm-proxy-i251-contract-final.log`.
  Changed files:
  - `scripts/test_live_gemini_candidates.sh`, `tests/operational_contract_test.go`, and `Makefile`.
  - `docs/gemini-current-models.md` and the I234, F047, I251, and B197 records.
  Event contracts: None.
  Resolution (2026-09-07): Source changes and required validation passed.
  Evidence: `/tmp/llm-proxy-b197-ci-verified.log`.

- [x] [I250] (P1) Wait for committed catalog usage before database restart.
  Goal:
  Make the public catalog integration test wait for its asynchronous usage writes.
  Evidence:
  B194 CI failed in `TestCatalogDefinedProviderFlowsThroughEveryGenericConsumer` with `database is locked (5) (SQLITE_BUSY)`.
  The test reopened the database immediately after a generation response.
  Requirements:
  - Wait for the first committed usage event before the second router starts.
  - Wait for both committed usage events before temporary database cleanup.
  - Reuse the existing persistence helper.
  Validation:
  - Run the failed Go target and final CI.
  Resolution (2026-09-05):
  The test waits for one committed usage event before database restart and two events before cleanup.
  The fixture connection stays open until both checks finish.
  Focused catalog tests and the Go suite pass.
  Final CI passes all 12 gates with 100.0% Go statement coverage.

- [x] [I249] (P1) Refresh Gemini inline request evidence.
  Goal:
  Verify current image and audio request limits for the two enabled Gemini offerings.
  Evidence:
  The general file guide lists 100 MB, but both media-specific guides state a 20 MB total request limit.
  The existing 20,000,000-byte bounds are correct for the supported media inputs.
  B194 corrects the two candidate records that used the general guide alone.
  Requirements:
  - Retain the documented media-specific bound for `gemini-3.5-flash` and `gemini-3-flash-preview`.
  - Refresh source references and verification dates.
  - Explain the source distinction in the operator documentation.
  Validation:
  - Verify public metadata for all four Gemini offerings.
  - Verify exact Files API uploads above the common inline bound.
  Sources:
  - https://ai.google.dev/gemini-api/docs/file-input-methods
  - https://ai.google.dev/gemini-api/docs/image-understanding
  - https://ai.google.dev/gemini-api/docs/audio
  Resolution (2026-09-05):
  Both enabled offerings retain the correct 20,000,000-byte media request bound.
  Their source now names the image guide, with verification dated September 5, 2026.
  The operator documents explain why the media-specific bound applies.
  B194 verifies public metadata and exact Files API behavior for all four Gemini offerings.
  Final CI passes all 12 gates and 95 browser tests with 100.0% Go statement coverage.

- [x] [I248] (P1) Retire direct Anthropic Opus 4.1 routes.
  Goal:
  Replace retired Opus 4.1 selections with a qualified current Anthropic model.
  Evidence:
  The audit found `claude-opus-4-1` and `claude-opus-4-1-20250805` in the direct Claude API catalog.
  Anthropic marks Opus 4.1 as retired from that API. Both IDs are absent from the current authenticated Models API response.
  Source: https://platform.claude.com/docs/en/about-claude/pricing
  Requirements:
  - Qualify the replacement through F046 before the cutover.
  - Inventory affected model selections, defaults, clients, and examples.
  - Add a bounded migration into the current model contract.
  - Preserve credentials, unrelated settings, and historical usage identities.
  - Remove both retired routes and reject new selections before provider dispatch.
  - Document operator activation and verify migration rollback and repeated startup.
  Validation:
  - Start with failing public HTTP and SQLite migration tests.
  - Verify current discovery and final CI after the cutover.
  Resolution (2026-09-05):
  Schema version 15 replaces both source selections with `claude-opus-5` at effort `high`.
  The transaction preserves credentials, prompts, timestamps, unrelated settings, and historical usage identities.
  An inherited profile effort conflict prevents startup and rolls back the migration.
  Both retired offerings and their exported constants are removed. Public requests and management selections reject both source IDs before provider dispatch.
  SQLite and HTTP tests verify repeated startup, older database upgrades, current dispatch, and complete rollback after write failures.
  The first fixture failed with `reason=provider_key_ineligible`. The corrected fixture confirmed the expected retirement failures before production changes.
  Initial CI found an uncovered final schema-write failure. The added fault scenario passed with the Go coverage gate.
  Final `make ci` passed all 12 gates, with 100.0% Go statement coverage and 95 browser tests.
  Evidence: `/tmp/llm-proxy-i248-red-final.log` and `/tmp/llm-proxy-i248-ci-final.log`.
  The public catalog now contains 61 enabled models. Event schemas remain unchanged.
  The operator procedure is in `docs/claude-retirement.md`. Production migration and deployment remain operator-owned.

- [x] [I247] (P1) Complete usage persistence checks in the retirement test.
  Goal:
  Complete the retirement integration test only after its HTTP usage records reach SQLite.
  Evidence:
  Final CI reported `TempDir RemoveAll cleanup: directory not empty` in `TestDeepSeekRetirementStartup/success`.
  The test issues two HTTP requests but ends before their asynchronous usage writes finish.
  Requirements:
  - Wait for both committed usage records before temporary database cleanup.
  - Verify the current provider and model identities on these records.
  - Preserve the existing historical usage assertions.
  Validation:
  - Run the focused retirement target and final CI.
  Resolution (2026-09-05):
  The success scenario waits for both committed request records before database cleanup.
  It verifies current DeepSeek identities and preserves the historical record checks.
  `make test-deepseek-retirement` passed. Final CI passed all 12 gates in 181 seconds with 100% Go statement coverage.
  Evidence: `/tmp/llm-proxy-f045-b193-i247-ci.log`.

- [x] [I245] (P1) Decommission retired direct DeepSeek model names.
  Development evidence (2026-09-05):
  - Removed both retired direct offerings and the unused chat model and V3 family records.
  - Added exact `none`, `low`, `high`, and `max` controls to both current V4 routes.
  - Added schema version 14 for matching profiles and tenant defaults. Historical usage identities and SiliconFlow R1 remain unchanged.
  - Startup rejects conflicting inherited profile reasoning with `provider_reasoning_decision_required` and rolls back the transaction.
  - HTTP and SQLite tests cover explicit controls, default dispatch, retired-route rejection, repeated startup, and failures during each migration stage.
  - Browser tests cover model discovery and reasoning selection. `make ci` passed all 12 gates with 100.0% Go statement coverage.
  - Updated `configs/providers.yml`, the catalog and transport code, managed storage, client fixtures, browser fixtures, and public capability tests.
  - Added `docs/deepseek-retirement.md` and updated `docs/provider-catalog.md` and `README.md`.
  - Added private migration fields `target_reasoning_effort` and `preserve_source_usage`. Public event contracts did not change.
  Resolution (2026-09-07):
  - Used the authorized `DEEPSEEK_API_KEY` from `configs/.env` through the standard live-test loader.
  - Both V4 models passed key verification and text requests with omitted, `none`, `low`, `high`, and `max` reasoning.
  - All twelve live checks returned HTTP 200, and `make test-live-providers` exited successfully.
  - Evidence: `/tmp/llm-proxy-i245-live-20260907.log`. Source changes, production selection inventory, and live provider qualification are completed.
  - Backup verification and production activation remain operator steps. This result does not establish production migration acceptance.
  Production inventory (2026-09-06):
  - Inspected the live database on `tutosh` through its declared retained volume with read-only SQLite queries.
  - At `2026-09-07T06:30:22Z`, the database had schema version 13, two tenants, and nine provider profiles.
  - DeepSeek profiles, DeepSeek defaults, and stored DeepSeek keys each numbered zero.
  - No current production selection requires a reasoning decision before the schema-14 migration.
  - Repeat the inventory before activation if tenant settings change.
  - Historical usage contained three direct V4 Flash events and three SiliconFlow R1 events. SQLite `quick_check` returned `ok`.
  - Recorded source commits, runtime identity, counts, and remaining acceptance steps in `docs/deepseek-retirement.md`.
  - At inventory time, live qualification required a DeepSeek key. The 2026-09-07 resolution records its completed acceptance.
  Goal:
  Remove retired direct DeepSeek routes and migrate current tenant selections to qualified canonical models.
  Evidence:
  - The 2026-09-05 catalog registers direct `deepseek-chat` and `deepseek-reasoner` offerings.
  - DeepSeek announced their retirement for 2026-07-24:
    https://api-docs.deepseek.com/updates/
  - The notice maps these names to non-thinking and thinking modes of `deepseek-v4-flash`, respectively.
  - The catalog also uses `deepseek-reasoner` for SiliconFlow's separate `deepseek-ai/DeepSeek-R1` offering.
  - The provider notice establishes the retirement date. Current account behavior still requires verification.
  Requirements:
  - Verify the current direct API contract and record the exact replacement model and reasoning controls for each retired route.
  - Qualify the replacement behavior before the migration. Record any unsupported reasoning requirement as a blocker.
  - Inventory affected tenant profiles, defaults, static configuration, client examples, and public discovery resources.
  - Add one bounded database migration for the exact direct-provider selections and their associated controls.
  - Preserve tenant credentials, unrelated settings, historical request identities, and usage records.
  - Remove the two direct provider offerings from routing, key verification, defaults, and public and management discovery.
  - Reject new requests for the retired direct routes before provider dispatch. Remove aliases and compatibility paths.
  - Keep the SiliconFlow R1 offering, its model record, saved selections, and historical migration records unchanged.
  - Remove root model records only when no retained provider offering references them.
  - Update the catalog, constants, documentation, clients, examples, generated resources, and affected fixtures together.
  Deliverables:
  - Canonical direct routes, a bounded selection migration, updated discovery resources, and an operator migration procedure.
  Validation:
  - Start with a failing public integration test for retired-route rejection and replacement behavior.
  - Do a test of migration rollback and repeated startup with real SQLite storage.
  - Prove historical usage and SiliconFlow routing remain unchanged.
  - Prove explicit and default requests use the qualified replacement model and required reasoning controls.
  - Verify model selection through public HTTP resources and the management browser interface.
  - Run `make ci` after the last application change.
  - Record authorized live-provider acceptance separately from local validation. Keep production activation operator-owned.

- [x] [I246] (P2) Decommission deprecated OpenAI transcription models before provider shutdown.
  Goal:
  Replace `gpt-4o-mini-transcribe` and `gpt-4o-transcribe` in the current transcription contract before 2027-02-26.
  Authorization (2026-09-06):
  The user approved reuse of the existing OpenAI key for implementation and provider acceptance.
  The existing process key returned HTTP 200 for `GET /v1/models/gpt-transcribe`.
  The model record reports `created=1785168027`.
  Both public transcription interfaces passed live acceptance before and after catalog replacement on 2026-09-07.
  Evidence:
  - Both models remain in the 2026-09-05 provider catalog.
  - OpenAI announced their API removal for February 26, 2027, on August 26, 2026:
    https://developers.openai.com/api/docs/deprecations
  - OpenAI lists `gpt-transcribe` and `gpt-live-transcribe` as replacements.
  - OpenAI released both replacements on July 28, 2026:
    https://developers.openai.com/api/docs/changelog
  Requirements:
  - Qualify `gpt-transcribe` for the current complete-response transcription contract before removing the old offerings.
  - Verify exact request fields, accepted audio formats, limits, response formats, usage, prices, and error behavior against official documentation.
  - Record the source and verification date for each replacement capability, limit, and price.
  - Preserve the existing public transcription interfaces and supported outputs through the replacement provider transport.
  - Keep new streaming transcription capabilities in a separate Feature issue when required.
  - Inventory affected tenant dictation selections, defaults, static configuration, client examples, and discovery resources.
  - Add one bounded migration from both deprecated OpenAI selections to the qualified replacement.
  - Preserve credentials, unrelated tenant settings, historical model identities, and usage records.
  - Remove deprecated offerings from routing, key verification, defaults, public discovery, and management selection.
  - Reject new requests for deprecated models before dispatch after the cutover. Remove compatibility aliases and fallback paths.
  - Update the catalog, adapters, clients, documentation, generated resources, examples, and affected fixtures together.
  - Document an operator activation schedule that completes the production migration before 2027-02-26.
  Deliverables:
  - A qualified replacement route, bounded selection migration, updated discovery resources, and a dated operator migration procedure.
  Validation:
  - Start with failing public integration tests for replacement transcription and deprecated-model rejection.
  - Do a test of each supported public transcription interface and official client against a controlled provider boundary.
  - Prove explicit and default requests use the replacement model with the expected output and usage metadata.
  - Do a test of migration rollback, repeated startup, and historical usage preservation with real SQLite storage.
  - Verify model discovery and management selection through HTTP and browser tests.
  - Run `make ci` after the last application change.
  - Record authorized live-provider acceptance separately from local validation. Keep production activation operator-owned.

  Resolution (2026-09-07):
  - Replaced both retired OpenAI offerings and the dictation default with `gpt-transcribe`.
  - Added schema version 16 and the bounded replacement for earlier ownership schemas.
  - Preserved credentials, unrelated settings, timestamps, and historical usage. Repeated startup and rollback tests passed.
  - Updated client fixtures, browser selection, examples, discovery, and the documented duration price.
  - Both public endpoints passed paid provider acceptance with the existing OpenAI key before and after catalog replacement.
  - Final `make ci` passed all 12 gates in 244 seconds, with 100 percent Go statement coverage.
  - The local read-only inventory found one tenant with the old mini transcription selection in the earlier ownership schema.
  - `docs/openai-transcription-retirement.md` records the migration procedure and the 2027-02-26 deadline.
  - Production inventory, deployment, and production acceptance remain operator-owned.

- [x] [I243] (P2) Update Governor-managed documents to the current templates.
  Goal:
  Eight managed documents differ from the current Governor templates.
  Requirements:
  - Update the five stack guides, planning contract, policy, and root managed block.
  - Preserve repository-owned instructions outside the root managed block.
  - Preserve all pending application changes.
  Validation:
  - Run the Governor dry run and check.
  - Review the changed prose.
  - Run `git diff --check`.
  - Compare application file contents before and after normalization.
  Resolution:
  - Updated the eight documents to the current Governor templates.
  - Added the integration-test sequence, GitHub Pages rules, and file permission boundary.
  - The Governor check reported no required changes.
  - The changed prose check and `git diff --check` passed.
  - Existing prose errors remain outside the managed sections and changed issue text.
  - Application files and repository-owned root instructions remain unchanged.
  Changed: `AGENTS.md`, `.mprlab/POLICY.md`, and `.mprlab/PLANNING.md`.
  Changed: `.mprlab/AGENTS.API.md`, `.mprlab/AGENTS.DOCKER.md`, and `.mprlab/AGENTS.FRONTEND.md`.
  Changed: `.mprlab/AGENTS.GO.md`, `.mprlab/AGENTS.PY.md`, and the issue tracker.
  Event contracts: No change.

- [x] [I242] (P1) Keep the application title and usage controls concise.
  Goal:
  The management application shows unnecessary words in two user-visible
  places. Its document title includes `Manage`, and each provider card repeats
  a `used` label beside its request count. One shared text control also changes
  two usage breakdowns together.
  Requirements:
  - Set the management application document title to exact `LLM Proxy`.
  - Keep the document title unchanged across application states.
  - Remove the `used` label from every provider card.
  - Remove the derived `used` state and its copy after the label removal.
  - Keep the `active` label for the selected tenant default route.
  - Preserve provider request totals, token totals, and activity graphs.
  - Put one icon-only chart toggle in each usage breakdown card.
  - Make the provider and model chart selections independent.
  - Use one button in each card to switch between the two chart types.
  - Show `Provider usage` once in its card.
  - Show `Model usage` once in its card.
  - Update the current management UI documentation.
  Deliverables:
  - Update the management page, frontend state, and copy.
  - Add browser coverage for the title, status labels, and independent toggles.
  - Update the README, implementation document, and changelog.
  Validation:
  - Run the frontend browser tests.
  - Run `make ci` after the last application change.
  - Run the STE check on each changed technical document.
  - Run `git diff --check`.
  Resolution:
  - The management document title is now exact `LLM Proxy`.
  - Provider cards no longer show or calculate a `used` state.
  - Provider usage and Model usage each have one title and one icon-only chart
    toggle.
  - Each toggle changes only its card and makes no usage request.
  - Both choices persist through Usage changes and reset on reload or
    authentication reset.
  - The generated public Usage resource documents the independent controls and
    preserves its publication brief.
  - Browser coverage verifies titles, icons, independence, persistence, reset,
    and compact layout.
  - No API or event contract changed.
  - `make ci` passed all 12 reported gates with 100 percent Go statement
    coverage.
  - The focused STE review passed for changed prose.

- [x] [I239] (P1) Standardize HTTP health at `/healthz`.
  Goal:
  Make `/healthz` the canonical health endpoint for the LLM Proxy API and
  static web origins. Use the endpoint for readiness without business-resource requests.

  Requirements:
  - Add unauthenticated `GET /healthz` to the API origin.
  - Publish a static `/healthz` resource for the GitHub Pages origin.
  - Return `200` only when each origin can serve its current application contract.
  - Return a non-success status when a required runtime dependency prevents API service.
  - Send `Cache-Control: no-store` on API and local health responses.
  - Use the GitHub Pages cache policy for production static health responses.
  - Keep each response free from credentials, provider data, and internal state.
  - Do not verify provider credentials or dispatch a provider during a probe.
  - Do not mutate application state during a probe.
  - Do not record a probe as application usage or an audit event.
  - Do not emit routine information-level request events for successful probes.
  - Keep failed probe evidence in container and deployment diagnostics.
  - Replace capability, config, and business-root readiness probes with `/healthz`.
  - Keep capability and config endpoints for their application functions only.
  - Use `/healthz` for local Compose, runtime capability, and public health checks.
  - Set `start_interval: 1s` and `interval: 30s` for Docker probes.
  - Set a bounded `start_period` for the API startup contract.
  - Keep the selected manifest contract unchanged.

  Deliverables:
  - Update the API, static artifact, request logging, orchestration, manifest, documentation, and black-box tests.

  Validation:
  - Verify unauthenticated `GET /healthz` returns `200` on each origin.
  - Verify API and local health responses use `Cache-Control: no-store`.
  - Verify a required dependency failure returns a non-success API status without provider work.
  - Verify the static publication artifact contains `/healthz`.
  - Verify no readiness probe requests a capability, config, or business resource.
  - Verify Docker probes use the required startup and steady intervals.
  - Verify successful probes create no routine request events.
  - Verify failed probes retain diagnostic evidence.
  - Run `make ci`.

  Implementation:
  - Added public API health with a bounded, read-only database check.
  - Added the static health resource and its publication check.
  - Changed readiness probes to use `/healthz` and kept failed probe evidence.
  - Verified quiet success responses, dependency failures, and unchanged usage.
  - Passed all 12 `make ci` gates, 85 browser tests, and 100% Go coverage.

  Cache policy:
  The operator approved the GitHub Pages cache-policy exception on 2026-09-04.
  This exception applies only to production static health responses.
  API and local health responses still require `Cache-Control: no-store`.

  Resolution:
  Implemented and verified API health, the static artifact, and readiness probes.
  Full local `make ci` passed. The approved cache exception removes the remaining blocker.

- [x] [I238] (P1) {I027,I237} Make provider card settings compact and automatic.
  Goal:
  A user can manage one provider connection without a manual completion action
  or an initial tenant selection.
  Requirements:
  - Use the first account tenant as the initial provider Settings tenant.
  - Always show the tenant selector on an open provider card.
  - Keep the provider Settings tenant independent from the Usage tenant.
  - Save provider model and prompt changes without a `Done` action.
  - Wait for pending provider saves when the user closes the card or changes
    the tenant.
  - Collapse the provider system prompt when the card or tenant context changes.
  - Let pointer and keyboard actions expand the provider system prompt.
  - Put `Get API key` on the same row as the catalog API service label.
  - Remove the `Replace key` action.
  - Accept a new key directly in the masked key field.
  - Verify each pasted key before the application saves it.
  - Preserve the prior key and provider settings when verification fails.
  - Put an icon-only `Delete key` action on the key input row.
  - Keep the provider card compact, accessible, and responsive.
  Deliverables:
  - Update the provider card editor and its saved state behavior.
  - Update the durable provider Settings documentation.
  - Add browser coverage for the new controls and save behavior.
  Validation:
  - Prove that a card opens with the Default tenant selected.
  - Prove that the tenant selector can change the provider Settings tenant.
  - Prove that Usage tenant selection does not change this context.
  - Prove that the card has no `Done` or `Replace key` action.
  - Prove that the prompt starts collapsed and autosaves after an edit.
  - Prove that a pasted replacement key is accepted or rejected automatically.
  - Prove that the trash icon has the `Delete key` accessible name.
  - Prove that the API key link and API service label share one row.
  - Prove that the card stays within each supported viewport.
  - Run `make ci` after the last application change.
  Resolution:
  Made provider card settings compact and automatic. The Default Settings
  tenant is ready when a card opens, while a persistent selector changes the
  tenant context. Provider edits now autosave. The key link and delete icon are
  inline, replacement keys validate on entry, and the system prompt starts
  collapsed. Browser coverage verifies the save, accessibility, and responsive
  layout contracts.

- [x] [I237] (P1) {I027} Separate provider APIs from model families and capabilities on provider cards.
  Goal:
  Provider cards show provider APIs, model families, and capabilities as
  different catalog concepts. A user can identify where an API key applies and
  what each provider offering can accept or produce.
  Requirements:
  - Rename the provider card section to `API connections`.
  - Show the provider definition as the primary API connection identity.
  - Add one catalog-owned API service label to each provider definition.
  - Use explicit service labels, such as `Gemini API` and `Meta API`.
  - Do not show an `API connection` label above the provider title.
  - Do not show a model publisher group on provider cards.
  - Show model families and capabilities in separate labeled groups.
  - Show capabilities only under a visible `Capabilities` label.
  - Do not show capability chips next to the provider title without a group
    label.
  - Get family relationships from provider offerings and exact models in the
    provider catalog.
  - Add the necessary family presentation fields to the authenticated
    management provider response.
  - Include offering media inputs in the provider capability list.
  - Use `Image analysis` for an image input capability.
  - Do not use `Image generation` for an image input capability.
  - Keep `Video generation` for an actual video generation operation.
  - Declare image input for `qwen3.7-plus` and `qwen3.6-flash`.
  - Keep `qwen-plus` and `qwen3.7-max` as text-only models.
  - Do not infer labels or catalog relationships from identifiers in the
    browser.
  - Keep canonical provider identifiers and provider offering selectors
    unchanged.
  - Change `Set key` to `Set API key`.
  - Change `Key settings` to `API key settings`.
  - Put the provider settings control in the top-right corner of each card.
  - Show only a settings gear icon in the control.
  - Use `Set API key` or `API key settings` as the accessible control name.
  - Show one request volume bar on each provider card.
  - Scale each bar against the highest provider request count in the current
    Usage scope.
  - Show an empty graph track when the provider has zero requests.
  - Use the existing provider aggregates without an additional request.
  - Give each graph an accessible label with its exact count and scale.
  - Keep the front and back faces at the same width and height.
  - Preserve the activity, tenant scope, key safety, and card interaction
    contracts from I027.
  - Keep the cards compact, accessible, and responsive in the MPR visual
    language.
  Deliverables:
  - Add the catalog and management data for model families and capabilities.
  - Update the provider card front, card back, user-visible copy, and canonical
    API documentation.
  - Add browser coverage for provider identities, families, and capabilities.
  Validation:
  - Prove that `OpenAI API` shows GPT-4, GPT-5, and GPT Transcribe as model
    families.
  - Prove that `Gemini API` shows Gemini as its model family.
  - Prove that `Meta API` shows Muse Spark as its model family.
  - Prove that `SiliconFlow API` shows DeepSeek R1 and SenseVoice as model
    families.
  - Prove that `DashScope API` shows image analysis and not image generation.
  - Prove that Qwen image input uses the OpenAI-compatible message shape.
  - Prove that capability labels remain separate from all identity labels.
  - Prove that each catalog provider still produces exactly one card.
  - Prove that the provider settings control has no visible text.
  - Prove that the provider settings control keeps its state-specific accessible
    name.
  - Prove that request bars update from the current provider aggregates.
  - Prove that each graph exposes its exact count and relative scale.
  - Prove that the two card faces have the same dimensions after a flip.
  - Cover keyboard access, semantic labels, desktop layout, and narrow-screen
    layout through Playwright.
  - Run `make ci` after the last application change.
  Resolution: Separated API services, model families, and capabilities. Added
  verified Qwen image input, icon-only provider settings controls, and request
  volume bars that use existing provider aggregates. Browser coverage confirms
  accessible graphs and equal card-face dimensions.

- [x] [I233] (P1) Add Gemini candidate models to the live test.
  Goal:
  Make the Gemini live test validate two stable candidates before public route
  registration.
  Initial evidence:
  - I207 was blocked because active retrieval and cancellation did not
    pass for `gemini-3.6-flash` or `gemini-3.7-flash`.
  - The current harness discovers only registered provider offerings. It
    cannot validate an unregistered candidate route.
  Requirements:
  - Add both exact candidates to the default `make test-live-gemini` flow.
  - Send omitted and supported thinking levels to the official Interactions
    API.
  - Prove one completed background lifecycle for each candidate.
  - Prove active retrieval, cancellation, and deletion for each candidate.
  - Keep both candidates outside the public provider catalog until I207
    satisfies its registration gate.
  - Load `GEMINI_API_KEY` through the current safe environment boundary.
    Never print the key or private provider response bodies.
  - Add black-box CLI coverage with a local fake provider boundary.
  Validation:
  - Run the candidate mode with the repository private environment file.
  - Run `make ci` after the last implementation change.
  Progress:
  - Added the exact-model candidate mode to the default Gemini wrapper. The
    mode validates the thinking matrix, completion, active retrieval,
    cancellation, and deletion without registering either public route.
  - Added local fake-provider coverage for the direct request matrix and the
    wrapper order.
  - Paid reasoning passed for the omitted, `minimal`, `low`, `medium`, and
    `high` Gemini 3.6 Flash requests. Its stored completion reached
    `completed` and deletion succeeded.
  - On 2026-09-05, `make test-live-gemini LIVE_ENV_FILE=configs/.env`
    passed the Gemini 3.6 Flash omitted, `minimal`, and `low` requests.
    The `medium` request then failed with curl error 28 after 45,003 milliseconds.
    The provider returned no response bytes before the configured timeout.
    The command stopped before `high`, background checks, and Gemini 3.7 Flash.
  Resolution (2026-09-05):
  Independent exact-model runs passed for Gemini 3.6 Flash and Gemini 3.7 Flash with the existing repository key.
  Both runs passed every declared reasoning level and one omitted level.
  Both runs proved background completion, active retrieval, cancellation, and deletion.
  The earlier timeout and visibility failures did not recur.
  The current harness passed the full CI checkpoint for F032 with 100.0% Go statement coverage.
  No harness code changed during this acceptance run.
  Evidence: `/tmp/llm-proxy-i233-gemini36-independent.log` and `/tmp/llm-proxy-i233-gemini37-independent.log`.
  I207 now owns registration of the two qualified routes.

- [x] [I207] (P1) Add Gemini 3.6 and 3.7 Flash with route-bound Interactions thinking levels.
  Goal:
  Add Google's current stable Flash models to the Gemini Interactions catalog
  and carry the provider-neutral `reasoning_effort` contract onto their
  documented thinking controls.
  Evidence:
  - Google identifies `gemini-3.6-flash` and `gemini-3.7-flash` as stable model
    IDs. Both models have a 65,536-token output limit:
    https://ai.google.dev/gemini-api/docs/models/gemini-3.6-flash
    https://ai.google.dev/gemini-api/docs/models/gemini-3.7-flash
  - Both model pages document text, image, video, audio, and PDF input, text
    output, and thinking:
    https://ai.google.dev/gemini-api/docs/models/gemini-3.6-flash
    https://ai.google.dev/gemini-api/docs/models/gemini-3.7-flash
  - Google's Interactions thinking guide documents
    `generation_config.thinking_level`. Gemini 3.6 Flash supports `minimal`,
    `low`, `medium`, and `high`. Gemini 3.7 Flash supports `low`, `medium`, and
    `high`. Both models use `medium` when the field is absent:
    https://ai.google.dev/gemini-api/docs/thinking
  - The Interactions schema defines the `thinking_level` field:
    https://ai.google.dev/api/interactions-api
  - Google's current Gemini migration contract rejects a request whose final
    nonempty turn is a model turn with HTTP 400. It directs multi-turn
    Interactions callers away from manually prefilled model turns:
    https://ai.google.dev/gemini-api/docs/latest-model
  Requirements:
  - Add only the exact stable model IDs `gemini-3.6-flash` and
    `gemini-3.7-flash`. Do not add a moving `gemini-flash-latest` alias or
    change the Gemini provider default as part of this issue.
  - Before registering the route lifecycle, verify at the paid Google boundary
    that both exact models support stored background Interactions through
    create, active status, retrieval, cancellation, and deletion. Register
    each model as `gemini_interactions` plus `pollable_resource` only after that
    proof succeeds.
  - At each resolved Gemini Interactions route edge, reject with the canonical
    HTTP 400 invalid-messages response when the request contains an assistant
    turn. Do not send it as `model_output`, rewrite it into another role, or
    change assistant-prefill behavior for other model routes.
  - Add the exact `gemini_interactions` reasoning-effort adapter, valid only for
    the Gemini Interactions text route. Declare Gemini 3.6 Flash with the
    ordered values `minimal`, `low`, `medium`, and `high`. Declare Gemini 3.7
    Flash with `low`, `medium`, and `high`. Serialize an explicitly resolved
    public effort unchanged as `generation_config.thinking_level` on the
    request.
  - When neither the request nor tenant default selects an effort, omit
    `thinking_level` so Google's `medium` default remains authoritative. Reject
    blank, `none`, `xhigh`, `max`, and every other value not declared for the
    exact route before an upstream call. In particular, reject `minimal` for
    Gemini 3.7 Flash. Do not translate values, send a 2.5-era
    `thinking_budget`, or add a second Gemini request adapter.
  - Keep the current 65,536 proxy output limit. Declare only the image and audio
    inputs that the public messages contract implements. Do not imply support
    for video, PDF, tools, thought summaries, streaming, or computer use.
  - Expose the exact route capability through the management profile and
    Settings autosave contract, and update configuration, constants, README,
    OpenAPI, provider-routing documentation, generated resources, and the model
    capability table together. No persisted-routing migration or default-model
    change is part of this issue.
  Validation:
  - Startup and public-boundary fixtures prove the Gemini-only adapter mapping.
    They prove each model's exact effort vocabulary and explicit payloads. They
    prove omission, continuation, unsupported-effort rejection, and the token
    limit. They also prove media, management, saved defaults, and terminal-turn
    rejection for only the two new routes.
  - Authenticated branch acceptance runs one small request for each explicit
    thinking level and one omitted-level request for each model. It proves one
    background create/poll/delete flow and one cancellation flow for each exact
    model. The final `make ci` passes after the last implementation edit.
    Deployment and production acceptance remain operator-owned.
  Prior acceptance: The new credential passed each declared reasoning request for both
  exact models. Both paid lifecycle probes created an `in_progress` resource.
  Google returned `403` and then `400` for every active retrieval attempt.
  Gemini 3.6 became readable only after completion. Gemini 3.7 stayed
  unreadable through the bounded probe. Both cleanup delete requests returned
  HTTP 200. Neither model supplied the required active retrieve and cancel
  proof. The provider catalog did not register these two routes.
  Current acceptance (2026-09-05):
  I233 passed each reasoning matrix and both complete stored interaction lifecycles.
  Resolution (2026-09-05):
  Registered both exact models as enabled Gemini Interactions routes after I233 passed live acceptance.
  Gemini 3.6 accepts `minimal`, `low`, `medium`, and `high`.
  Gemini 3.7 accepts `low`, `medium`, and `high`.
  Both routes retain the 65,536 output limit and current image and audio inputs.
  The Gemini provider default and existing tenant selections remain unchanged.
  HTTP tests cover explicit and omitted effort, invalid values, media, structured output, assistant rejection, and incomplete-result cleanup.
  Management tests cover saved model selections and every declared effort.
  Browser tests cover exact effort options, Settings autosave, and public model discovery.
  Updated the catalog, model constants, README, OpenAPI reference, routing document, and model table.
  Added `docs/gemini-qualified-models.md` as the current route and acceptance reference.
  The initial public HTTP tests returned `400 unknown model` for both routes.
  Final `make ci` passed all 12 gates, 97 browser tests, and 100.0% Go statement coverage in 234 seconds.
  The first CI run found an obsolete two-model Gemini profile expectation. The corrected fixture passed before final CI.
  Evidence: `/tmp/llm-proxy-i207-ci-final.log` and the I233 live acceptance receipts.
  No event contract changed. Deployment and production acceptance remain operator-owned.

- [x] [I027] (P1) Put provider usage and key settings on provider cards.
  Goal:
  The authenticated dashboard has one catalog-owned card for each supported
  provider. Each card combines provider activity with tenant-owned key and
  provider profile settings. A provider always exists through the provider
  catalog. Key presence and historical activity never define provider
  membership.
  Requirements:
  - Render one card for every provider definition in deterministic catalog
    order. Do not hard-code a provider list in the browser.
  - Do not use `Connected`, `Configured`, or `Verified` as user-facing provider
    states. Do not add an `Add provider` or `Remove provider` action.
  - Use `active` only for the tenant's selected default route. Use `used` only
    for activity in the selected Usage interval.
  - Show the provider label, catalog capabilities, request total, and token
    total on the card front.
  - For one Usage tenant, show that tenant's selected provider text model.
  - For `All tenants`, do not synthesize one selected model from different
    tenant profiles.
  - Match activity by exact canonical provider ID. Keep historical activity
    visible after key deletion.
  - Show zero activity only after a successful usage load. Show unavailable
    activity after a usage-load failure.
  - Rename the current `Providers` metric to `Providers used`. Count exact
    provider IDs with activity in the selected interval.
  - For one tenant, label the card action `Set key` when no key is saved.
    Label the action `Key settings` when a key is saved.
  - For `All tenants`, label the action `Key settings`. Require an exact owned
    tenant selection before key controls become available.
  - Do not put a persistent key-status badge on the card front.
  - Flip the card only through the explicit `Set key`, `Key settings`, or
    `Done` control. Do not make the complete card a button.
  - Permit only one open card back at a time. Keep its provider and tenant
    identity fixed until the user closes or discards the editor.
  - Show the exact tenant, catalog-defined provider fields, selected text model,
    and provider system prompt on the card back.
  - When no key is saved, show the key field and the provider's official
    key-acquisition link.
  - Load each official key-acquisition URL from the validated provider catalog.
    Open it with `target="_blank"` and `rel="noopener noreferrer"`.
  - Do not send a tenant ID, authentication value, provider key, or tracking
    value to the key-acquisition URL.
  - When a key is saved, show a generic mask and explicit `Replace key` and
    `Delete key` actions. Do not reveal the key when the card flips.
  - Delete only the saved provider key. Keep the provider definition, provider
    profile, non-secret fields, and historical usage.
  - Preserve valid tenant routing defaults when key deletion makes a route
    unavailable.
  - Verify each new or replacement key through the exact selected provider
    route before persistence.
  - Show `Checking key...` only while the verification request is active. Lock
    conflicting card actions during that request.
  - After successful verification, save the accepted key and show its generic
    mask. Do not store or render a persistent verification state.
  - Keep the card back open after successful verification until the user selects
    `Done`.
  - After failed verification, keep the candidate unsaved and show the exact
    safe error. Preserve a previously saved key and settings.
  - Reject each stale verification, save, deletion, or usage response after a
    provider, tenant, interval, view, or authentication change.
  - Keep the independent Usage tenant filter unchanged during card key actions.
  - For `All tenants`, aggregate each card's activity across owned tenants.
    Bind back-face key actions only to the explicitly selected tenant.
  - Use the canonical tenant profile for key and provider profile state. Do not
    add an owner-wide key-state projection or browser request fan-out.
  - Remove the duplicate provider key, model, and provider prompt editor from
    Settings after the card back owns these controls.
  - Keep client access, routing defaults, the tenant prompt, and request examples
    in Settings because these values are not provider keys.
  - Supersede P001 only for provider-editor placement. Retain its catalog URL,
    tenant isolation, atomic save, and client-key separation requirements.
  - Keep raw key input transient on the active card back. Never put it in card
    fronts, attributes, accessible names, browser storage, logs, or usage data.
  - Keep a masked key value out of the card front and its accessible name.
  - Use semantic provider articles and explicit controls with `aria-expanded`
    and `aria-controls`.
  - Make the inactive face `inert` and hidden from accessibility APIs. Move
    focus between the action and the first applicable back-face control.
  - Use a restrained 180-to-220-millisecond flip. Replace rotation with an
    immediate face change when `prefers-reduced-motion` requests less motion.
  - Use solid MPR charcoal surfaces, thin borders, compact controls, and
    semantic status colors. Do not use a blurred-glass face.
  - Keep the card grid aligned without horizontal overflow on narrow screens.
  - Keep provider key controls unavailable on the administrator dashboard.
  Deliverables:
  - Replace the prior provider-membership proposal with the catalog-owned
    provider card grid and scope-correct activity presentation.
  - Add the accessible front/back card interaction and the tenant-bound key
    editor.
  - Move provider key, model, and provider prompt controls from Settings to the
    card back. Keep tenant-owned controls in Settings.
  - Replace provider removal with key-only deletion that preserves provider
    profile settings, non-secret fields, and historical usage.
  - Add the validated key-acquisition URL to the safe management catalog
    projection.
  - Update frontend types, canonical API documentation, generated references,
    self-service documentation, and user-facing copy.
  Validation:
  - Add Playwright scenarios for every catalog provider and deterministic card
    order.
  - Cover one explicit Usage tenant and `All tenants` with one provider card per
    provider definition.
  - Cover exact provider activity, zero activity, unavailable activity, and
    historical activity after key deletion.
  - Cover explicit-tenant model presentation. Prove `All tenants` does not show
    one synthetic selected model.
  - Cover no-key, saved-key, replacement, deletion, active verification,
    rejection, timeout, rate-limit, and unavailable verification states.
  - Prove successful verification creates no persistent `Verified` state.
  - Prove key deletion keeps the provider card, provider profile settings,
    non-secret fields, and historical usage.
  - Prove card opening does not reveal a key, mutate settings, or send a
    provider request.
  - Prove `All tenants` key actions require an exact tenant and do not change
    the Usage tenant filter.
  - Prove another tenant or user cannot receive key state, draft values,
    provider settings, or activity.
  - Prove only one card editor owns a raw draft. Reject stale responses after
    each provider, tenant, interval, view, or authentication change.
  - Prove raw and masked keys stay out of card fronts, attributes, accessible
    names, browser storage, logs, and usage data.
  - Cover keyboard operation, focus return, inactive-face isolation, reduced
    motion, desktop layout, and narrow-screen layout.
  - Prove the Settings modal has no duplicate provider editor after the card
    cutover.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair for implementation.
  Resolution: Added catalog-owned Usage Overview provider cards, tenant-bound
  key controls, safe catalog links, and credential-only deletion. Removed the
  duplicate Settings editor.

- [x] [I032] (P2) {I027} Add donut breakdowns and meaningful axes to Usage Overview charts.
  Goal:
  Let a signed-in user choose one clear presentation for both the selected
  Usage scope's Provider usage and Model usage activity breakdowns, and make
  the Requests and Tokens time-series charts explain their scales without
  guesswork. Preserve the Usage tenant, interval, exact request and token
  counts, and the separation between provider activity and catalog-owned
  provider cards.
  Evidence:
  - The current usage summary already returns deterministically ordered
    provider and model aggregates with request counts. The existing rows are
    ranked horizontal bars scaled to the largest category, not shares of the
    breakdown total.
  - The summary has time buckets only for total requests and tokens. It has no
    provider- or model-specific time series, so the breakdown's `Bar graph`
    choice must mean the existing ranked horizontal-bar display rather than a
    new trend chart.
  - The current Requests and Tokens panels render only a bordered SVG and an
    independently scaled polyline. They have no visible axes, ticks, time
    labels, quantity labels, or numeric scale, so the curve alone cannot tell a
    user when activity happened or whether a peak represents one request or
    thousands.
  - The canonical summary already supplies `interval`, `bucket_unit`, and each
    ordered bucket's RFC3339 `start`, `data.requests`, and
    `data.total_tokens`. Meaningful time and quantity axes require no new
    management payload.
  - The current Usage contract provides account-wide and explicitly
    tenant-filtered scopes. I027 establishes the final dashboard layout and
    reserves provider/model breakdowns for selected-period activity. Provider
    key presence does not define card membership.
  Requirements:
  - Implement after I027 against the canonical response for the selected Usage
    scope. Do not add a presentation-specific endpoint, response field, server
    persistence, URL parameter, tenant setting, browser storage, or
    client-library change.
    If final implementation exposes a genuinely missing data field, file and
    order a separate contract issue rather than broadening this UI issue.
  - Add one shared, visible, keyboard-operable `Breakdown view` control for
    both activity panels. It has exactly `Bar graph` and `Donut chart` choices;
    `Bar graph` is the default and is the existing ranked horizontal-bar
    presentation. Switching a mode changes both panels together so their
    distributions remain directly comparable.
  - Keep the choice local to the mounted authenticated dashboard. It survives
    interval selection, Refresh, and Usage tenant selection, but resets on
    authentication reset and a full page reload. A mode change is a
    pure presentation action. It must not fetch, mutate the selected interval
    or Usage tenant, or weaken the current request-identity and stale-response
    rules.
  - Build every donut from the same ordered `providers[].data.requests` or
    `models[].data.requests` data that Bar graph renders. The percentage
    denominator is the complete source breakdown total, never token counts or
    the largest row. Preserve every source category exactly once: Bar graph
    always lists each category; the donut may combine the ordered tail into a
    visibly labelled `Other` segment only when a named, documented,
    geometry-derived donut-capacity rule would otherwise make the compact panel
    unreadable. `Other` must expose its exact aggregate count and deterministic
    share; it cannot discard or relabel source data.
  - Render the alternative as a dependency-free SVG donut chart with an
    unmistakable center cutout in the existing compact dark dashboard style.
    Give each segment a deterministic palette assignment from the canonical
    summary order, but never use color or hover alone to communicate meaning.
    Show a visible semantic legend/list with category name, request count, and
    deterministic percentage; rounded legend shares must total 100 percent.
    Handle zero activity with the existing empty state and one-category
    activity as one 100-percent segment without invalid SVG geometry.
  - Treat the Requests and Tokens panels as time-series line charts distinct
    from the breakdown presentation mode. Give each chart visible X and Y axis
    lines, tick marks, tick values, and axis titles. The X-axis is `Time (UTC)`
    and comes directly from ordered `buckets[].start`: show UTC hour labels for
    the `1d` hourly buckets and UTC date labels for the `7d`, `30d`, and `all`
    daily buckets. The Y-axis begins at zero and uses deterministic integer
    ticks. Its title is `Requests per hour` or `Requests per day` for
    `data.requests`, and `Tokens per hour` or `Tokens per day` for
    `data.total_tokens`, according to `bucket_unit`. Never label the Tokens
    chart as requests or imply that the two metrics share a numeric scale.
  - Derive one typed, centralized chart-axis model from the accepted summary.
    Select a bounded, deterministic subset of X ticks that includes the first
    and last bucket when they are distinct, keeps labels legible at the current
    width, and never changes the plotted bucket order or values. Use readable
    locale-independent UTC labels and compact but unambiguous integer
    formatting; expose the exact value when compact visible notation is used.
    Preserve zero-valued buckets, do not smooth or interpolate the source
    series, and handle a flat or all-zero metric without division-by-zero or a
    misleading nonzero range.
  - Use centralized frontend copy and typed presentation data for the control,
    mode names, legend, `Other`, axis titles, tick labels, and accessible SVG
    text. Preserve visible focus and full keyboard operation (`aria-pressed` or
    an equivalent single-choice control), and keep breakdown
    labels/counts/shares plus every time bucket's exact UTC start and metric
    value available to assistive technology without hover or a tooltip.
    Validate desktop and narrow layouts without clipping, tick-label overlap,
    or horizontal overflow.
  - Keep the scope to the authenticated user's Usage Overview. I027's provider
    cards remain catalog-owned and show selected-scope activity.
  - Apply this control only to the dedicated Provider usage and Model usage
    panels. Do not change card faces or tenant key settings.
  - Do not add this control to the aggregate administrator dashboard. Do not
    expose credentials, keys, prompts, responses, or other sensitive usage data.
  - Document the resulting presentation contract in README, CHANGELOG.md, and
    `docs/implementation/provider-routing-plan.md`. Update the source in
    `scripts/generate_seo_resources.mjs` and regenerate the managed-tenant
    usage resource; do not hand-maintain a divergent generated page. State
    explicitly that this is a client-side view of existing aggregate request
    data, define both line charts' UTC time and per-bucket quantity axes, and
    state that the presentation is not a billing, provider-performance,
    provider-key, token-share, exact-event-time, or new management-API
    feature. This repository has no PRD.md or ARCHITECTURE.md; do not create
    partial placeholders for this UI change.
  Deliverables:
  - One typed local presentation-mode contract, pure provider/model distribution
    transform, shared selector, semantic bar/donut renderings, responsive
    styles, and centralized copy in Usage Overview.
  - A legible, deterministic SVG donut/legend treatment that preserves all
    request counts and makes any `Other` aggregation explicit.
  - One typed time-series axis/tick contract and two semantic SVG line charts
    whose visible and accessible labels identify UTC time and requests or total
    tokens per canonical hour/day bucket without changing the usage API.
  - Updated README, CHANGELOG.md, implementation documentation, generator-owned
    public usage resource, generated artifact, and browser coverage; no
    management API, Go client, Python client, or CLI wire-contract change.
  Validation:
  - Add Playwright coverage through the real management dashboard showing the
    default Bar graph mode, keyboard selection of Donut chart, simultaneous
    changes to provider and model panels, visible names/counts/shares, and no
    additional usage request when the presentation changes.
  - Exercise interval changes, Refresh, Usage tenant selection, loading/failure, and
    out-of-order response scenarios; prove the local mode remains selected only
    where specified and never presents a stale Usage scope or interval snapshot.
  - Cover zero, one, and many-category distributions, including deterministic
    `Other` aggregation, exact request-count conservation, share totals of 100
    percent, Bar graph access to every source category, non-color-only
    semantics, administrator isolation, and desktop/narrow viewport geometry.
  - Cover `1d`, `7d`, `30d`, and `all` summaries and prove visible X ticks map
    to the supplied UTC bucket starts, Requests Y ticks and points map only to
    `data.requests`, Tokens Y ticks and points map only to
    `data.total_tokens`, both Y scales start at zero, and exact bucket values
    remain programmatically available. Exercise empty, flat-zero, single-peak,
    large-value, desktop, and narrow-viewport cases without clipped or
    overlapping axes, invented data, tooltip-only meaning, or an additional
    management request.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair for the implementation, with
    the final run after the last code edit.
  Resolution: Added one local bar/donut control and semantic request-share
  legends. Added zero-based UTC quantity axes and exact accessible bucket data.

## Maintenance

- [x] [M009] Consolidate repository runbook documents under `.mprlab/`. (historical M009R reclassified as a completed one-off)
- [x] [M010] Document 60-day social media advertising campaign.
- [x] [M011] Require CI before and after every code-changing task.
- [x] [M014] Patch the canonical Go toolchain security release.
- [x] [M015] {M014} Remove the reachable HTTP/3 QPACK vulnerability from the Go graph.
- [x] [M016] {M015} Upgrade the reachable PostgreSQL driver dependency past SQL-injection fixes.
- [x] [M017] {M016} Upgrade mapstructure past sensitive-error leakage.
- [x] [M018] {M017} Remediate the reachable Go security graph and remaining reported advisories.
- [x] [M020] Adopt the activated canonical MPR UI integration contract.

### Complete entries archived 2026-08-30

- [x] [M022] (P1) Remove static tenant mode.
  Goal:
  The service has one tenant contract. Each client uses a managed tenant key
  that an authenticated user creates through the management API.
  Requirements:
  - Make management configuration mandatory. Remove `management.enabled` and
    its disabled state.
  - Remove the `tenants` configuration block and all static tenant types,
    validation, authentication, and routing paths.
  - Remove configuration-level provider API keys. Load provider credentials
    only from encrypted managed tenant records.
  - Authenticate public proxy requests only against managed tenant secret
    digests.
  - Reject obsolete `management.enabled`, `tenants`, and provider `api_key`
    fields as unknown configuration. Do not add a migration or compatibility
    path.
  - Remove `SERVICE_SECRET` from environment documentation, examples, test
    inputs, and generated public content. Use `LLM_PROXY_DEFAULT_TENANT_KEY` only for
    registered client keys.
  - Replace the static live-provider preflight with a non-paid managed
    preflight. Create a disposable user, tenant, client key, and provider
    connection through current public boundaries.
  - Remove static mode from application code, tests, fixtures, and current
    documentation. Keep issue archive records unchanged.
  - Preserve public `key` authentication with generated managed tenant keys.
  Deliverables:
  - Implement a managed-only configuration, startup, authentication, and
    routing contract.
  - Implement a managed live-provider preflight with no static tenant
    configuration.
  - Update configuration examples, environment documentation, README,
    routing documentation, generated public content, and contract tests.
  Validation:
  - Prove the service starts with the complete managed configuration.
  - Prove obsolete static fields fail strict configuration loading.
  - Create a managed client key through the management API. Use that key for
    successful public text and dictation requests.
  - Prove missing, unknown, and replaced managed client keys fail public
    authentication.
  - Prove current application code and user documentation contain no static
    tenant contract or `SERVICE_SECRET` input.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - Made management configuration mandatory and removed static tenant,
    configuration-level provider-key, authentication, and routing paths.
  - Public proxy calls now authenticate only with managed tenant client keys.
    Provider credentials and defaults come only from encrypted tenant records.
  - Managed authentication now sends request cancellation to the tenant
    database query.
  - The non-paid preflight now saves a generated OpenAI key through the
    management API. It routes one prompt through a loopback Responses server.
  - Updated current configuration, tests, documentation, and generated public
    content to use `LLM_PROXY_DEFAULT_TENANT_KEY` as the only client-key input.
  - The follow-up `make ci` passed all 11 gates with 100.0% Go statement
    coverage and the managed provider preflight.

### Complete entries archived 2026-09-08

- [x] [M013] (P2) Resolve missing product-context document references.
  Goal:
  Keep the root governance entrypoint limited to product-context documents that exist and represent the current contract.
  Requirements:
  - Decide whether current `PRD.md` and `ARCHITECTURE.md` documents are required or whether their references are stale.
  - Add current canonical documents or remove the obsolete references; do not add placeholders or compatibility documents.
  - Treat B069's final bounded client-selected request budget, ingress deadline
    ownership, caller-cancellation distinction, and gateway outer-guard
    invariant as required source material if canonical product or architecture
    documents are added.
  Deliverables:
  - Root governance references that resolve to current product-context files.
  Validation:
  - Verify every product-context path named by root `AGENTS.md` exists and contains current repository guidance.
  Resolution:
  - `README.md` remains the canonical product-context document.
  - The `docs/` directory contains current integration and API guidance.
  - The repository does not require separate `PRD.md` or `ARCHITECTURE.md`
    documents.
  - Root guidance no longer references the two absent documents.
  - The retained product-context paths exist.

- [x] [M012] (P2) {M013} Reconcile repository governance with the MPR Lab normalizer.
  Goal:
  Make the governance normalizer check pass without deleting repository-owned binding contracts.
  Requirements:
  - Resolve M013's product-context document decision first so the normalizer
    works from the final repository-owned root guidance.
  - Inspect the normalizer differences reported for root `AGENTS.md` and every managed `.mprlab/` guide.
  - Replace M011's per-task pre-change CI run with the current Governor
    completion checkpoint.
  - Retain final CI after the last stack change.
  - Prohibit unit tests in each stack.
  - Preserve all other current repository-owned rules.
  - Update the appropriate managed templates, boundaries, or repository
    documents as one canonical forward-only contract.
  - Do not apply a destructive bulk rewrite.
  Deliverables:
  - A reviewed governance normalization change with no unrelated product or runtime edits.
  Validation:
  - Run the MPR Lab governor in `--dry-run` and `--check` modes and require no pending managed-file changes.
  Progress: 2026-08-20. Applied the current managed updates to
  `.mprlab/POLICY.md` and `.mprlab/issues-md-format.md`. At that time, the
  Governor check was clean, but M013 blocked completion.
  Resolution:
  - M013 selected `README.md` and `docs/` as the current product-context
    sources.
  - The Governor detected the API, Go, Python, frontend, and Docker profiles.
  - The current validation policy supersedes M011's per-task pre-change CI run.
  - The policy retains final CI after the last stack change.
  - The current test policy prohibits unit tests in each stack.
  - Root guidance now owns issue classification and resolved-issue hygiene.
  - The issue format document now contains syntax and identifier rules only.
  - The policy and Docker guide now contain the versionless selected-manifest
    contract.
  - The Governor dry-run and check report no required changes.
  - The four fully managed documents pass the mechanical STE check.
  - The focused root guidance and issue text pass the mechanical STE check.

- [x] [M019] (P2) Refresh non-security direct dependency pins.
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
  Resolution:
  - Gin `v1.12.0`, JWT `v5.3.1`, Viper `v1.21.0`, Zap `v1.28.0`, and GORM `v1.31.2` remain unchanged because they are current.
  - TAuth now uses `v1.2.7`. Its imported session-validator contract is unchanged.
  - Alpine, js-yaml, Playwright, and the Node.js type definitions now use their current releases. The obsolete separate js-yaml type package was removed, and the public site uses the js-yaml v5 browser export.
  - The Python lock now selects mypy `2.3.1`, pytest `9.1.1`, and their current transitive development dependencies.
  - `go mod verify`, `npm audit --json`, and the exported locked Python audit passed with no known vulnerabilities.
  - The required baseline and final CI commands passed all 11 gates with 100 percent Go coverage.

## Features

- [x] [F062] (P1) Return completion measurements through the official Go client
  Goal: TellTale I022 can record the resolved model and available token counts for each provider call.
  Requirements:
  - Return response text, resolved model identity, and optional token usage through a typed client result.
  - Keep available metadata when a provider call fails.
  - Keep unavailable usage distinct from measured zero tokens.
  - Define resolved identity as the exact catalog model selected for dispatch.
  Deliverables:
  - The official Go client method and canonical HTTP metadata contract.
  - Real HTTP tests for tenant defaults, explicit models, usage, and failures.
  Validation:
  - Run `make test-client-contracts` and final `make ci`.
  Resolution: Added typed completion results, resolved catalog model headers, and optional usage on successful and failed calls.
  Validation result: Final CI passed all 12 gates with 100% Go statement coverage on 2026-09-08.
  Documentation: Updated the OpenAPI contract, generated API page, and Go client guide.
  Publication: A release with F062 remains required for TellTale I022.

- [x] [F001] Add authenticated self-service API key and tenant secret management UI.
- [x] [F002] Add one-time migration from legacy config tenants and provider API keys into the DB.
- [x] [F003] Support explicit GORM database dialects for management persistence.
- [x] [F004] Make packaged management DB dialect and DSN configurable through expandable config variables.
- [x] [F005] Remove placeholder default syntax and source the SQLite management DB path from `.env`.
- [x] [F006] Split the self-service management frontend onto GitHub Pages and keep llm-proxy as an API backend.
- [x] [F007] Move management settings into an avatar-menu modal and make the dashboard usage-focused.
- [x] [F008] Make the management dashboard and Settings modal more compact.
- [x] [F009] Add administrator visibility for all managed users.
- [x] [F012] Add GPT 5.6 to the list of supported OpenAI models including the level of efforts.
- [x] [F013] Add selectable usage-dashboard time intervals.
- [x] [F010] Add Meta Model API and Muse Spark 1.1 as a supported text provider.
- [x] [F011] Migrate the legacy global token to its authenticated user account.
- [x] [F015] Let application users change the model through reloadable client profiles.

### Complete entries archived 2026-08-10

- [x] [F031] (P1) {I215} Surface the routing graph directly below the landing hero.
  Goal:
  Make the one-contract product boundary and the interactive provider/model
  route understandable before visitors reach the longer integration and
  capability sections.
  Requirements:
  - Add one second landing-page section immediately after the hero containing
    the existing One endpoint, One credential, and One contract facts followed
    by the complete generated routing graph.
  - Keep the facts and graph aligned to one 1180-pixel responsive shell without
    duplicating the facts or retaining the graph below the capability matrix.
  - Give the routing overview its own full-width bordered surface and section
    spacing so it is visibly distinct from both the hero and the following
    integration section, not merely a new semantic wrapper on the page canvas.
  - Preserve generated provider/model completeness, keyboard selection,
    measured connector geometry, semantic no-JavaScript content, and narrow
    viewport containment.
  - Keep the later Providers and model capabilities section focused on the
    generated capability catalog and request-limit contract.
  Validation:
  - Browser coverage proves the new hero -> routing overview -> integration ->
    audience -> capabilities -> catalog order and exact facts/graph ownership.
  - Playwright visually verifies the combined section at desktop and mobile
    widths, including graph interaction and containment.
  - Run the required final
    `timeout -k 350s -s SIGKILL 350s make ci`.
  Resolved 2026-08-09:
  - The post-hero routing overview is now a visibly distinct full-width page
    section with its own bordered surface and responsive section padding. Its
    1180-pixel inner shell owns the three product facts and the single generated
    provider/model routing graph; the later Models section owns only the
    capability catalog and request-limit content.
  - Browser coverage proves the section class, boundary borders, background
    separation from the page canvas and following section, desktop/mobile
    spacing, unique graph ownership, interaction geometry, keyboard selection,
    and semantic no-JavaScript content. Fresh 1280-pixel and 390-pixel captures
    verified the corrected visual boundary and containment.
  - All 87 frontend browser tests and the required final
    `timeout -k 350s -s SIGKILL 350s make ci` passed; CI completed all 11 gates
    in 119 seconds with 100.0% Go statement coverage.

- [x] [F014] (P1) Support multiple isolated tenants per managed user.
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
  - Add one bounded, versioned, all-or-nothing GORM migration for the configured SQLite management database. Do not add raw-SQL persistence, dual reads/writes, a runtime fallback to the old schema, or a compatibility response shape.
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
  - Add disposable pre-migration SQLite database fixtures containing multiple current users, encrypted provider keys, generated-secret digests, defaults, and usage. Run the real migration entrypoint and prove exact preservation, ciphertext re-binding, old-schema removal, idempotent version rejection, rollback on corrupted/orphaned rows, and unchanged client-secret routing.
  - Add Playwright coverage for first-user bootstrap, create, switch, URL reload/history, independent tabs, rename, guarded final-tenant deletion, confirmed deletion, unsaved-edit handling, one-time secret/key cleanup, response-order races, explicit invalid-URL errors, admin tenant lists, keyboard use, and desktop/mobile geometry.
  - Extend the real local TAuth black-box path to create and use two tenants for one verified session and prove a second verified user cannot access either tenant.
  - Run the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci` pair for the implementation, with the final run occurring after the last code edit.
  Resolved 2026-07-25:
  - Replaced the one-user/one-tenant persistence and unscoped management
    surface with account-owned opaque tenants, tenant-bound credentials,
    defaults, secrets, usage, administrator projections, and canonical
    `/api/management/tenants/:tenant_id/...` operations.
  - Added the transactional version-1 SQLite ownership migration,
    provider-key ciphertext rebinding, strict preflight/verification/rollback,
    disposable database fixtures, and operator-owned migration runbook.
  - Added URL-owned workspace selection and full create, switch, rename, and
    delete interaction with isolation, confirmation, cancellation, stale
    response protection, credential cleanup, responsive behavior, and updated
    generated documentation.
  - The required pre-change and post-change `make ci` runs pass. The final run
    followed the last code edit and passed static analysis, exact 100% Go
    coverage, the real SQLite migrations, 33 Python tests,
    package installation, 59 browser scenarios, the real two-user TAuth
    black-box test, release checks, and live-provider harness preflight.
- [x] [F019] (P1) Create a canonical public landing page and generated capability catalog.
  Goal:
  Make the Pages root an indexable, useful LLM Proxy landing page that accurately
  explains what the service does, who it is for, how it is used, and every
  currently supported provider/model capability. Move the existing authenticated
  management workspace to the one canonical `/app/` route so public product
  discovery and key-management workspaces are not competing root pages.
  Requirements:
  - Serve a public, useful `https://llm-proxy.mprlab.com/` landing page without
    requiring a management session. Keep the separate API origin's `GET /`,
    `POST /`, `/v2`, and `/dictate` contracts unchanged; only the Pages
    information architecture changes.
  - Move the current MPR UI/TAuth management shell, its rendered
    `data-config-url`, header navigation, logout destination, browser tests,
    and release renderer to `/app/`. `/app/` is a private workspace
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
  - Provide clear crawlable calls to action for `/app/`, the resource hub,
    and current integration documentation. Use semantic HTML, visible focus,
    accessible tables/filters, concise unique metadata, canonical root URLs,
    and structured data that describes only visible landing-page content.
  Deliverables:
  - Add the canonical catalog projection/build contract and a static public
    landing page with capability sections, provider/model matrix, limitations,
    and conversion paths.
  - Relocate and render the management application at `/app/`, update all
    root/resource/header/footer links, and document the new public-vs-private
    Pages route contract in README and deployment/site-render guidance.
  - Update the resource hub and shared site shell so public navigation points to
    the landing page while management calls to action point only to `/app/`.
  - Do not duplicate catalogs in HTML/JavaScript/docs, make availability claims
    based on whether a particular user has a key, expose secrets, or preserve a
    second root management implementation.
  Validation:
  - Add black-box build/render coverage proving the public matrix exactly
    reflects the validated catalog, has no secret-bearing fields, and rejects
    catalog/render drift.
  - Add Playwright coverage for an anonymous public landing, its accessible
    provider/model matrix and CTAs, navigation to `/app/`, and the full
    existing authenticated management lifecycle at that new route.
  - Verify root canonical, Open Graph, JSON-LD, sitemap, and resource links use
    the final public URL form, while `/app/` is noindex and excluded from
    sitemap output.
  - Run the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci`
    pair for the implementation, with the final run after the last code edit.
  Resolved 2026-08-06:
  - Replaced the Pages root management shell with an accessible public product
    landing and moved the only authenticated workspace to noindex `/app/`.
  - Added a deterministic, secret-free provider/model capability projection
    from the validated runtime registry and made invalid config, catalog, or
    landing markers fail the Pages render.
  - Updated resource/API navigation, metadata, sitemap, deployment rendering,
    README guidance, and browser flows for the public/private route boundary.
  - The required pre-change and post-change `make ci` runs pass. The final run
    followed the last code edit and passed all 11 gates: exact 100% Go coverage,
    36 Python tests, 76 browser scenarios, the real TAuth black-box scenario,
    Pages artifact checks, and live-provider harness preflight.
  - Follow-up 2026-08-06: replaced the 130px hand-built landing footer with the
    shared in-flow `<mpr-footer size="small">`, retained all five public
    destinations with compact labels, and added desktop/mobile browser checks
    for a maximum 56px rendered height and no horizontal overflow. The required
    baseline and final `make ci` runs pass; the final run passed all 11 gates.
  - Follow-up 2026-08-06: moved the value strip's border and raised background
    from the full-width section to the centered three-item grid, removing the
    empty side rectangles without changing the One endpoint, One credential,
    or One contract panels. Desktop/mobile browser geometry and all 11 `make ci`
    gates pass.



- [x] [F018] (P1) Redesign dashboard around unified graphs and provider cards.
  Goal:
  Simplify the app UI by making the dashboard the primary place for unified graphs and provider configuration, reducing or eliminating the need for a separate settings area.

  Requirements:
  Preserve the existing dashboard concept for unified graphs. Add a clear card-based area for each provider beneath the graphs. Each provider card should let users enter that provider’s credentials and default options. The redesign should make provider setup easier to discover and should keep the app feeling simple rather than adding extra navigation or complexity.

  Deliverables:
  A proposed UI redesign for the dashboard showing unified graphs plus per-provider configuration cards. Updated implementation or design artifacts for the new provider-card flow. Any necessary cleanup of settings-related UI if the new dashboard flow replaces it.

  Validation:
  A user can open the dashboard, see the unified graphs, find a card for each provider, and enter credentials and defaults without needing a separate settings page. The resulting UI is easier to understand at a glance and does not remove required provider configuration capabilities.
  Resolved 2026-08-10:
  - I027 supersedes this proposal. The current product direction keeps usage
    and connected-provider metadata on the dashboard and keeps credential and
    routing edits in the Settings tenant context.

### Complete entries archived 2026-08-30

- [x] [F035] (P0) {F033} Add verified provider image routes.
  Goal:
  LLM Proxy routes canonical image and audio attachments only through Gemini.
  As a result, the Image input filter shows only the Gemini family. Provider
  documents describe image input for OpenAI, Anthropic, and xAI models in the
  current catalog. The Audio input filter represents conversational audio
  input. The Dictation filter represents speech transcription.
  Requirements:
  - Reverify image and audio support for each exact model from official
    provider documents.
  - Add OpenAI Responses image transport for each verified exact model:
    https://developers.openai.com/api/docs/guides/images-vision
  - Add Anthropic Messages image transport for each verified exact model:
    https://platform.claude.com/docs/en/build-with-claude/vision
  - Add xAI Responses image transport for each verified exact model:
    https://docs.x.ai/developers/model-capabilities/images/understanding
  - Audit every Gemini exact model with the official image and audio guides:
    https://ai.google.dev/gemini-api/docs/image-understanding
    https://ai.google.dev/gemini-api/docs/audio
  - Declare `media_inputs` only for a provider offering with a working
    code-owned transport.
  - Reject each media declaration that has no code-owned provider transport.
  - Preserve attachment order, media bytes, and MIME type through provider
    serialization. Validate provider-returned checksums for uploaded files.
  - Use a provider file API when inline transport cannot carry accepted media.
  - Apply each provider offering's declared media limits before dispatch.
  - Publish each media limit with its source and verification date.
  - Derive public capabilities and route filters from the validated catalog.
  - Declare audio input only for verified conversational routes.
  - Keep dictation models under the Dictation filter.
  Validation:
  - Prove each new provider transport through canonical `POST /v2` requests.
  - Prove the exact provider payload, attachment order, media bytes, and MIME
    type.
  - Prove each bounded media limit at its edge.
  - Prove that unsupported media routes fail before provider dispatch.
  - Prove the exact media routes in `/api/public/capabilities`.
  - Prove the Image input and Audio input route filters in a browser.
  - Prove complete semantic route content without JavaScript.
  - Run `make ci` after the last application change.
  Resolution:
  - Added image transport for all verified OpenAI and Anthropic text models.
  - Added synchronous xAI Responses image transport for `grok-4.5`.
  - Declared image and conversational audio input for all seven Gemini text models.
  - Added provider MIME checks and offering-owned request, count, and attachment limits.
  - The Image input filter now shows 8 proprietary families and 29 exact models.
  - The Audio message input filter now shows 1 family and 7 exact models.
  - Public `POST /v2`, capability, browser, and no-JavaScript tests passed.
  - `make ci` passed all 11 gates with 93 browser tests and 100.0% Go statement
    coverage.

- [x] [F034] (P1) Filter the route explorer by weight access and capability.
  Goal:
  Reduce the model family fan with compact route filters in the diagram title.
  Requirements:
  - Add one explicit `weight_access` value to each model family.
  - Accept only `proprietary` and `open_weights` values.
  - Publish weight access through the public capability resource and OpenAPI.
  - Reuse the canonical capability definitions and compact capability pills.
  - Keep the title and filters in one header row.
  - Keep filtered counts in that row when the available width permits it.
  - Select one or more weight access values. Select `Proprietary` by default.
  - Select exactly one capability. Select `Text generation` by default.
  - Show a family when its weight access and one provider offering match the
    active filters.
  - Show only exact models and provider offerings that match the active
    filters.
  - Keep the current route when it remains valid. Otherwise, select the first
    valid route in catalog order.
  - Update the visible family, exact model, and provider offering counts.
  - Show an explicit empty result when no route matches the active filters.
  - Preserve complete semantic HTML without JavaScript.
  - Preserve the five-stage route and responsive page containment.
  Validation:
  - Prove the default `Proprietary` and `Text generation` selections.
  - Prove each access selection and each canonical capability selection.
  - Prove that weight access keeps one or more selections.
  - Prove that capability keeps exactly one selection.
  - Prove the empty result for a valid filter combination without a route.
  - Prove filtered connector endpoints and deterministic route selection.
  - Inspect the single header row at 1280, 900, and 390 pixels.
  - Run `make ci` after the last application change.
  Resolution:
  - The model catalog classifies each family as proprietary or open weights.
  - The public capability resource and OpenAPI publish this classification.
  - The title row contains a multi-choice weight access group. It also contains
    a single-choice capability group.
  - Weight access keeps at least one selection. Capability keeps exactly one
    selection.
  - `Proprietary` and `Text generation` are the default selections.
  - The route fan, counts, empty result, selection, and connectors follow all
    selected values.
  - Visual checks passed at 1280, 900, and 390 pixels.
  - `make ci` passed all 11 gates with 93 browser tests and 100.0% Go statement
    coverage.

- [x] [F033] (P0) {F022} Pass canonical message media to selected providers.
  Goal:
  Let `/v2` accept provider-neutral media without a smaller LLM Proxy media
  limit. Translate each attachment into the selected provider's supported
  transport. Preserve exact media bytes, order, and MIME type.
  Observed failure:
  - Creative Director sends `master-character-sheet.png` through
    `NewImageAttachment` for Gemini semantic image QA.
  - The PNG is 3,326,724 bytes. Canonical base64 requires 4,435,632 bytes
    before JSON and prompt content.
  - The deployed capability resource reports `max_prompt_bytes: 4194304`.
  - LLM Proxy returns HTTP 413 `prompt payload too large` before Gemini
    receives the image.
  - Gemini permits this request under its documented inline request limit.
  - Creative Director receives no QA result. It cannot produce the next
    MediaOps operation.
  Provider limit evidence:
  - The following provider limits were verified on 2026-08-11.
  - Gemini Interactions permits 20 MB for a request with inline image data.
    This total includes text, system instructions, and inline bytes. Gemini
    permits 3,600 image files per request. Gemini directs larger requests to
    its Files API:
    https://ai.google.dev/gemini-api/docs/image-understanding
  - OpenAI vision permits a 512 MB total request payload. It permits 1,500
    image inputs per request:
    https://developers.openai.com/api/docs/guides/images-vision
  - Anthropic Messages permits a 32 MB request. The direct Claude API permits
    10 MB for each base64 image:
    https://platform.claude.com/docs/en/api/errors
    https://platform.claude.com/docs/en/build-with-claude/vision
  - Anthropic permits 100 images for models with a 200,000-token context.
    Anthropic permits 600 images for other models.
  - xAI permits 20 MiB for each image. xAI publishes no image-count limit:
    https://docs.x.ai/developers/model-capabilities/images/understanding
  - These values are provider facts. They do not define one proxy limit.
  - LLM Proxy currently routes message media only to Gemini. Other values
    define contracts for future provider adapters.
  Contract gap:
  - The current `/v2` contract applies one 4 MiB limit before provider
    dispatch.
  - The selected Gemini provider permits the rejected request.
  - One proxy limit cannot represent different provider limits or transports.
  - F022 defines asset upload. It does not connect assets to `/v2` attachments.
  Requirements:
  - Use the F022 tenant asset store for asset-backed attachments.
  - Add an asset-reference variant to the canonical user-message attachment
    union.
  - Require `type`, `asset_id`, and `mime_type` in each asset
    reference.
  - Reject an attachment that contains both `data` and `asset_id`.
  - Remove `server.max_prompt_bytes` as a provider-independent `/v2` media
    admission rule.
  - Do not define a smaller proxy-owned media limit.
  - Apply the selected provider offering limits before provider dispatch.
  - Count bytes with the selected provider's documented unit and scope.
  - Include base64 expansion only when the provider counts encoded request
    bytes.
  - Send inline media when the selected provider accepts that transport and
    size.
  - Use provider file upload when the selected provider requires that
    transport for the media size.
  - Return a stable provider-limit error when no provider transport accepts
    the media.
  - Send the request when the provider publishes no applicable limit.
  - Map a provider limit response into the stable LLM Proxy error contract.
  - Keep provider limits in provider offering data, not one server setting.
  - Add media limits only to provider offerings that declare the media input.
  - Publish each limit's value, unit, scope, source, and verification date.
  - Represent an explicit provider no-limit value as `unbounded`.
  - Represent an unpublished provider limit as `unknown`.
  - Reverify provider limits during implementation.
  - Validate tenant ownership, asset state, expiry, MIME type, size, and stored
    byte integrity before provider dispatch.
  - Preserve message order and attachment order after asset resolution.
  - Preserve caller bytes without resize, compression, or format conversion.
  - Keep caller filesystem paths outside the HTTP contract.
  - Exclude asset bytes and authenticated asset URLs from logs, responses, and
    usage records.
  - Add official-client constructors for asset-backed image and audio
    attachments.
  Deliverables:
  - Add the OpenAPI asset-reference schema and provider limit schema.
  - Add tenant asset resolution and provider transport selection.
  - Add provider offering limits to the public capability resource.
  - Add official-client support for asset-backed attachments and provider
    limits.
  - Update the root README, API reference, provider routing guide, and release
    notes.
  Validation:
  - Send the 3,326,724-byte image inline through `/v2` to fake Gemini.
  - Require provider dispatch without a proxy 413 response.
  - Upload the same fixture through F022. Send one `/v2` asset reference.
  - Require fake Gemini to receive the exact bytes, MIME type, and order.
  - Do a test of each provider boundary at the limit and one unit above it.
  - Do a test of Gemini inline and Files API transport selection.
  - Do a test of each documented provider limit record.
  - Prove no provider-valid request fails because of a smaller proxy limit.
  - Cover missing, foreign, expired, deleted, wrong-MIME, and
    same-length-corrupted assets.
  - Cover one image, ordered images, audio, and mixed media.
  - Prove asset cleanup cannot change an admitted request or expose another
    tenant's asset.
  - Prove logs, errors, responses, and usage records contain no asset bytes or
    authenticated asset URLs.
  - Exercise the public router and every released official client against fake
    providers.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Resolution:
  - Added tenant asset upload, deletion, strict resolution, and official client
    support.
  - Added offering-owned media limits and Gemini inline or Files API routing.
  - Added OpenAPI, public capability, Go client, Python client, and browser
    contracts.
  - Kept asset and management data on the hosted retained `/data` volume.
  - Bounded `/v2` request ingestion from the catalog contract and applied the
    authenticated request budget to asset uploads.
  - Removed upload-stream lock contention and added restart-safe scheduled
    expiry reclamation.
  - Made Gemini Files API deletion authoritative after every finalized upload,
    including polling and cancellation failures.
  - Passed the final 11-gate CI run with 100.0% Go statement coverage.

### Complete entries archived 2026-09-08

- [x] [F053] (P1) Add Gemini 3.5 file transcription.
  Goal:
  Add `gemini-3.5-transcribe` to the current tenant-owned file-dictation interface.
  Requirements:
  - Qualify exact audio bytes and a known transcript through synchronous Interactions before route activation.
  - Use the existing Gemini provider credential and preserve current tenant defaults.
  - Reuse the public file-dictation request and text response contracts.
  - Send audio input with `background: false` and `store: false`.
  - Preserve the provider's default verbatim transcription and automatic language detection.
  - Validate supported audio inputs at the route boundary and preserve existing upload limits.
  - Reuse provider file upload and cleanup when the selected transport requires it.
  - Keep native provider identifiers and uploaded media out of public responses and logs.
  - Update catalog metadata, management selection, public discovery, clients, and current documentation together.
  - Treat live transcription, diarization, timestamps, and smart formatting as separate interface requests.
  Validation:
  - Start with failing public-entrypoint tests for candidate acceptance and native dictation behavior.
  - Verify an exact known speech fixture through the paid provider boundary.
  - Reject incomplete, empty, or incorrect acceptance output without registering the route.
  - Prove local HTTP routing, malformed responses, usage, cancellation, and file cleanup.
  - Run final CI after the last application change.
  Work completed:
  - Paid acceptance returned the expected words from a generated English PCM WAV on September 5, 2026.
  - Both public transcription endpoints use the native synchronous route. Large uploads use Files API cleanup.
  - The catalog contains one enabled Gemini dictation model with current token prices.
  - Management selection, tenant usage, client discovery, and browser Settings checks pass.
  - Empty saved dictation selections remain valid after catalog capability changes. Explicit selections still require an eligible provider credential.
  - The initial native HTTP test returned 400 with unsupported provider endpoint.
  - The restart test reproduced provider_key_ineligible for a tenant with an empty dictation selection. The corrected test preserves that selection.
  - The accounting test initially recorded an unsupported format as accepted work. The corrected test records invalid_request.
  Resolution:
  - The native file-dictation route and current documents satisfy the acceptance checks.
  - Final CI passed all 12 gates in 241 seconds, with 100 percent Go coverage and 98 browser tests.
  - Paid acceptance passed the known speech fixture. Deployment and production acceptance remain operator-owned.
  Sources:
  - https://ai.google.dev/gemini-api/docs/transcribe
  - https://ai.google.dev/gemini-api/docs/models/gemini-3.5-transcribe
  - https://ai.google.dev/api/interactions-api

- [x] [F046] (P1) Add Claude Fable 5.1 and Opus 5.
  Goal:
  Expose the current Claude models through the existing Anthropic connection and public text contract.
  Requirements:
  - Add exact IDs `claude-fable-5-1` and `claude-opus-5` to their existing families.
  - Preserve the current Anthropic default and tenant selections.
  - Declare a 1,000,000-token context and 128,000-token synchronous output maximum.
  - Support the current JPEG, PNG, and WebP inputs and existing structured output contract.
  - Map public efforts `low`, `medium`, `high`, `xhigh`, and `max` to `output_config.effort`.
  - Preserve the provider's adaptive thinking default when effort is omitted.
  - Keep thinking blocks absent from public answers and logs.
  - Record current image limits and standard input, output, and cache prices.
  - Qualify disabled candidates through the disposable harness before activation.
  - Record Fable 5.1's provider retention requirement in the operator documentation.
  Sources:
  - https://platform.claude.com/docs/en/models/fable-5-1/overview
  - https://platform.claude.com/docs/en/models/fable-5-1/migration-guide
  - https://platform.claude.com/docs/en/models/opus-5/overview
  - https://platform.claude.com/docs/en/models/opus-5/whats-new-opus-5
  - https://platform.claude.com/docs/en/build-with-claude/vision
  - https://platform.claude.com/docs/en/about-claude/pricing
  - https://platform.claude.com/docs/en/api/models/list
  Validation:
  - Prove exact routing, effort, structured output, images, usage, and continuation through public HTTP.
  - Prove invalid efforts and limits fail before provider dispatch.
  - Verify key selection, public capabilities, and management controls.
  - Run live text, effort, and image qualification for each exact model.
  - Run focused targets, browser tests, and final CI.
  Progress (2026-09-05):
  The authenticated Models API returned HTTP 200 with both exact IDs and their documented capabilities.
  Evidence: `/tmp/llm-proxy-f046-anthropic-models.json`.
  Resolution (2026-09-05):
  Both models passed live key verification, default text, all five efforts, and image requests before catalog activation.
  Public HTTP tests cover routing, structured output, continuation, private thinking exclusion, usage, limits, and management verification.
  The Anthropic adapter maps effort to `output_config.effort` and retains the existing structured output format.
  The catalog now publishes 63 enabled models. Current limits, prices, and retention requirements are in `docs/claude-current-models.md`.
  Initial CI failed on an outdated Anthropic offering count and verification date assertion. The corrected catalog tests passed.
  Final `make ci` passed all 12 gates with 100.0% Go statement coverage and 95 browser tests.
  Evidence: `/tmp/llm-proxy-f046-ci-final.log`. Live qualification receipts are listed in the runbook.
  Changes are in the repository checkout. Production deployment remains separate.

- [x] [F044] (P1) Add explicit model activation to the provider catalog.
  Reclassified from I235 because this change adds an operator capability.
  Resolution (2026-09-05):
  - Added required model activation values to the private schema and all 62 model records.
  - Startup validates retained metadata and compiles only enabled models, offerings, and prices.
  - Disabled defaults and migration targets fail validation. Disabled routes are absent from public and management discovery.
  - HTTP tests prove rejection before dispatch. SQLite startup tests prove selection migration and rollback when the migration is missing.
  - `make test-provider-catalog` and `make ci` passed. All 12 CI gates passed with 100.0% Go statement coverage.
  - Updated the catalog documentation and terminology. Public event contracts did not change.
  Goal:
  Keep exact model data in the provider catalog without exposing an unaccepted
  model route.
  Requirements:
  - Add a required `enabled` Boolean field to each exact model in
    `configs/providers.yml`.
  - Do not use an implicit default for the field.
  - Include only enabled exact models and their provider offerings in the
    immutable runtime registry.
  - Exclude disabled exact models from route resolution, defaults, management
    projections, public capabilities, and standard live-test discovery.
  - Permit a disabled exact model to retain its provider offerings, limits,
    controls, and prices in the provider catalog.
  - Reject a provider default that references a disabled exact model.
  - Require an explicit model migration for each stored selection that
    references a disabled exact model.
  - Update the provider catalog documentation and all current catalog records.
  Validation:
  - Prove that startup rejects a missing or invalid `enabled` value.
  - Prove that startup rejects a disabled provider default.
  - Prove that public discovery omits a disabled exact model and its offerings.
  - Prove that route resolution rejects a disabled exact model before provider
    dispatch.
  - Prove that an explicit migration moves a disabled stored selection to an
    enabled exact model.
  - Run `make ci` after the last application change.
  Media qualification:
  - Qualify each provider offering and operation before advertising the migrated media capability.
  - A model activation flag alone does not prove every provider operation is available.

- [x] [F038] (P0) Add first-class client protocol adapters.
  Reclassified from I240 because this change adds public client interfaces.
  Resolution: Added all four `/v1` interfaces through the shared completion coordinator.
  Added native caller tools, tenant bearer authentication, exact model discovery, and buffered server-sent events.
  Updated the Go and Python clients, provider catalog, OpenAPI, public site, and OpenCode examples.
  Pinned OpenAI SDK and OpenCode tests passed through real HTTP with local provider fixtures.
  All 12 CI gates passed with 100% Go statement coverage and no uncovered blocks.
  The final API documentation and contract checks passed.
  B183 corrected the discovered test teardown race.
  Changed: `internal/proxy/client_*.go`, `caller_tools.go`, `completion_result.go`, and the shared router and provider adapters.
  Changed: `pkg/llmproxyclient/`, `python/llm_proxy_client/`, `configs/providers.yml`, `Makefile`, and pinned test dependencies.
  Changed: `docs/openapi.yaml`, `docs/client-protocols.md`, `README.md`, the provider routing document, and generated site files.
  Changed: `examples/opencode/`, public contract tests, SDK scripts, and browser tests.
  Event contracts: Chat Completions chunks and ordered Responses events describe a buffered canonical result.
  I243 resolved the Governor-managed document drift after this feature.


  Goal:
  Make llm-proxy directly usable by OpenCode and standard OpenAI clients.
  Keep `/v2` as the canonical llm-proxy contract for repository-owned clients.
  Support multiple public protocols without duplicate provider logic or
  inconsistent request accounting.

  Requirements:
  - Add a client protocol adapter boundary between public handlers and the
    canonical completion coordinator.
  - Give each adapter ownership of its paths, authentication input, request
    validation, canonical translation, response encoding, and error encoding.
  - Register all client protocol adapters through one typed route registry.
  - Fail startup when two adapters register the same HTTP method and path.
  - Do not let a client protocol adapter call a provider transport directly.
  - Do not let one client protocol adapter call another client protocol adapter.
  - Keep outbound provider protocol adapters separate from client protocol
    adapters.
  - Put the current text and dictation interfaces behind explicit client
    protocol adapters.
  - Keep capability, asset, management, and health resources outside this
    adapter registry.
  - Route each accepted request through the same tenant, catalog, timeout,
    admission, continuation, usage, and request identifier services.
  - Keep one canonical domain request and one closed canonical result type.
  - Let the canonical result contain final text, structured data, or client
    tool calls.
  - Do not expose an upstream resource identifier or native provider payload.
  - Treat each new endpoint as a current product interface, not a fallback or
    deprecated compatibility path.
  - Keep `docs/openapi.yaml` as the sole HTTP contract for every public endpoint.

  - Keep `POST /v2` as the native messages endpoint.
  - Keep the official Go package, Python package, and Go CLI on `/v2`.
  - Keep the current root, `/v2`, `/dictate`, asset, and management wire
    contracts unchanged.
  - Extend `/v2` with provider-neutral function declarations and tool results.
  - Represent a tool request as a typed result, not as failed provider text.
  - Accept assistant tool calls and `tool` role results in later `/v2` turns.
  - Require one exact tool call identifier for each tool result.
  - Keep `/v2` blocking and retain its current provider lifecycle ownership.

  - Add `POST /v1/chat/completions` for the supported OpenAI Chat Completions
    subset.
  - Add `POST /v1/responses` for the supported OpenAI Responses subset.
  - Add `GET /v1/models` for authenticated route discovery.
  - Add `POST /v1/audio/transcriptions` for supported dictation routes.
  - Publish an exact supported-field matrix for each OpenAI-style endpoint.
  - Reject each unsupported field with the documented OpenAI-style error shape.
  - Do not claim support for an OpenAI operation that the proxy does not expose.

  - Require `Authorization: Bearer <tenant-client-key>` on each `/v1` endpoint.
  - Do not accept a tenant key in a `/v1` URL or request body.
  - Use the bearer value only as an llm-proxy tenant client key.
  - Never forward the bearer value as an upstream provider credential.
  - Preserve the current server-side provider credential boundary.
  - Keep each `/v1` request stateless.
  - Accept `store: false` or omission and reject `store: true`.
  - Reject `previous_response_id` until a separate durable conversation
    contract exists.
  - Use proxy-owned identifiers in Chat Completions and Responses objects.

  - Use `provider/model` as the exact `/v1` model identifier.
  - Resolve that identifier to one enabled provider offering before dispatch.
  - Reject aliases, ambiguous model-only identifiers, and unknown offerings.
  - Return only routes that the authenticated tenant can use from `/v1/models`.
  - Derive model records from the immutable provider catalog and tenant key
    state.
  - Do not invent a model owner, creation date, capability, or limit.
  - Add required catalog metadata when an OpenAI model field needs that data.

  - Support text messages, system instructions, output limits, and supported
    reasoning controls on both text endpoints.
  - Support caller function declarations, tool selection, and parallel tool
    calls where the exact provider offering supports them.
  - Relay client tool calls to the caller without executing those tools.
  - Preserve exact tool names, call identifiers, and JSON argument text.
  - Validate each tool result against an earlier assistant tool call in the
    submitted messages.
  - Reject client tools before dispatch when the selected offering lacks the
    explicit capability.
  - Keep built-in provider web search separate from caller function tools.
  - Add an explicit caller-tool capability to each eligible provider offering.
  - Do not infer caller-tool support from a provider protocol or model name.
  - Map supported JSON Schema output controls to the canonical structured
    request contract.

  - Return protocol-correct non-streaming Chat Completions and Responses objects.
  - Support `stream: true` with the required server-sent event sequence.
  - Emit canonical result events without exposing native provider events.
  - For a blocking provider transport, emit the event sequence after the final
    canonical result exists.
  - Do not describe a buffered event sequence as provider token streaming.
  - Propagate caller cancellation through the coordinator to active provider
    work.
  - Keep each streaming response within the current request timeout policy.
  - Return normalized usage in each protocol's documented usage fields.
  - Record one managed usage event for one accepted client request.
  - Do not record one usage event for each streamed event or tool item.

  - Map validation, authentication, rate limit, capacity, timeout, and provider
    failures to one documented OpenAI-style error object.
  - Include the proxy request identifier in each error response.
  - Keep provider bodies, provider errors, credentials, prompts, tool arguments,
    and generated content out of logs.
  - Keep the current safe management failure records and usage summaries.
  - Use the same response headers and request timing evidence across adapters.

  - Add an OpenCode example that uses the OpenAI-compatible provider package.
  - Add an OpenCode example that uses the Responses provider package.
  - Require no custom OpenCode provider code for either example.
  - Require only a base URL and tenant client key in standard OpenAI SDKs.
  - Do not require an official llm-proxy client for a `/v1` request.
  - Load the tenant client key from an environment variable in each example.
  - Do not put a tenant client key in a tracked configuration file or URL.
  - Keep provider selection visible in the configured `provider/model` value.
  - Document the native `/v2` advantages and each compatibility boundary.
  - Update the public integration routes and API reference from verified
    contracts.

  Deliverables:
  - Add the typed client adapter registry and canonical tool-call domain types.
  - Add the Chat Completions, Responses, model discovery, and transcription
    endpoints.
  - Update `/v2`, the official clients, the provider catalog, and OpenAPI.
  - Add OpenCode configuration examples and an exact compatibility matrix.
  - Add black-box tests through the real HTTP server and fake provider servers.
  - Pin each tested OpenCode and OpenAI SDK version in test dependencies.

  Validation:
  - Prove each public adapter reaches the same route and completion coordinator.
  - Prove no adapter calls a provider transport or another adapter directly.
  - Prove bearer authentication selects one tenant and never reaches upstream.
  - Prove `/v1/models` returns only exact usable `provider/model` routes.
  - Prove Chat Completions text and tool-call rounds with the official SDK.
  - Prove Responses text and function-call rounds with the official SDK.
  - Prove one OpenCode task calls a safe local tool through Chat Completions.
  - Prove one OpenCode task calls a safe local tool through Responses.
  - Prove both OpenCode tasks use llm-proxy tenant authentication.
  - Prove non-streaming and event-stream results have valid protocol shapes.
  - Prove a blocking provider produces a valid buffered event sequence.
  - Prove caller cancellation stops active provider work.
  - Prove unsupported fields and capabilities fail before provider dispatch.
  - Prove public errors and logs do not expose protected request data.
  - Prove each accepted request records exactly one managed usage event.
  - Prove current `/v2` text, media, structured output, and lifecycle behavior.
  - Run `make ci` after the last application change.

- [x] [F037] (P1) Report rejected requests separately from proxy failures.
  Goal:
  The Usage view must keep rejected requests visible without treating them as
  proxy failures. A request for an unconfigured provider cannot execute, but
  the current report includes it in request totals and failed requests.
  Requirements:
  - Define `rejected`, `succeeded`, and `failed` as the request dispositions.
  - Reject an unconfigured provider before provider dispatch.
  - Return HTTP `409` with the stable `provider_not_configured` outcome.
  - Persist each rejected request without prompt, response, credential, or provider error data.
  - Keep only succeeded and failed requests in usage totals, charts, dimensions, status groups, and success rates.
  - Keep only failed requests in the failure report.
  - Add a separate rejected-request count and paginated report.
  - Include only a resolved typed route in rejected-request provider and model fields.
  - Add account and tenant rejected-request resources to the management API.
  - Use one bounded migration to replace the persisted success flag with the request disposition.
  - Map historical pre-route validation and unconfigured-provider events to rejected requests.
  - Remove the old success flag after the migration.
  - Update the management UI, API schema, product documentation, and public Usage resource.
  Deliverables:
  - Add the request disposition domain type and persisted schema.
  - Add rejected-request aggregates, resources, and management UI details.
  - Add public API, real-store, migration, and browser regression coverage.
  Validation:
  - Prove that an unconfigured provider receives no provider dispatch.
  - Prove that its request appears only in the rejected-request report.
  - Prove that a provider or proxy execution error remains a failed request.
  - Prove that rejected requests do not change usage totals or success rates.
  - Upgrade persisted usage data and prove the exact disposition mapping.
  - Run the frontend browser tests.
  - Run `make ci` after the last application change.
  - Run the STE check on each changed technical document.
  - Run `git diff --check`.
  Resolution:
  - Added the required rejected, succeeded, and failed request dispositions.
  - Unconfigured routes now return exact `409 provider_not_configured` before dispatch.
  - Rejected records contain safe route metadata and have a separate paginated report.
  - Usage totals and failure reports now include attempted executions only.
  - Schema version 13 replaces the success flag and maps historical records.
  - API, migration, real-store, browser, and TAuth tests verify the separation.
  - The public Usage resource passed its independent SEO evaluation with 55/55.
  - `make ci` passed all gates with 100 percent Go statement coverage.
  - The final run included 45 Python tests and 85 browser tests.
  - Focused changed-prose STE review and `git diff --check` passed.

- [x] [F032] (P1) Add Baidu Qianfan as a user-configurable text provider.
  Goal:
  Let a managed user paste, verify, and save a Baidu Qianfan API key through
  the existing tenant-scoped provider editor. Route blocking LLM Proxy text
  requests through current international Qianfan model offerings.
  Evidence:
  - Baidu's Qianfan Quick Start documents one API key used as
    `Authorization: Bearer <API Key>`, an OpenAI-compatible client at
    `https://api.baiduqianfan.ai/v1`, and an optional `appid` header:
    https://intl.cloud.baidu.com/en/doc/qianfan/s/qm8qxemze-intl-en
  - The text-generation API is synchronous when `stream` is false or omitted
    and uses `POST https://api.baiduqianfan.ai/v1/chat/completions` with
    `model`, ordered `messages`, optional `max_tokens`, OpenAI-compatible
    choices and usage, and `stop`, `length`, `content_filter`, or `tool_calls`
    finish reasons. It also publishes Qianfan's `flag` content-safety signal:
    https://intl.cloud.baidu.com/en/doc/qianfan/s/3m7of64lb-intl-en
  - The current international model list publishes text-generation ids
    `ernie-5.0`, `deepseek-v4-pro`, `deepseek-v4-flash`, and
    `deepseek-v3.2`, with documented maximum output limits of 65536, 131072,
    131072, and 32768 tokens respectively:
    https://intl.cloud.baidu.com/en/doc/qianfan/s/7m95lyy43-intl-en
  Requirements:
  - Add one `baidu` provider definition through the current provider catalog.
    Use `baidu` as its canonical provider id. Use `Baidu Qianfan` as its display
    label. Declare no aliases.
  - Register exact Qianfan text offerings `ernie-5.0`, `deepseek-v4-pro`,
    `deepseek-v4-flash`, and `deepseek-v3.2`. Use `ernie-5.0` as the Baidu
    provider default and record each documented output-token limit on its
    provider offering.
  - Use `https://api.baiduqianfan.ai/v1`, bearer authentication, the existing
    `openai_chat_completions` wire contract, `synchronous_completion`, and the
    upstream `max_tokens` field. Omit the optional `appid`. Make one pasted API
    key sufficient for the complete supported flow.
  - Declare one opaque nonblank `api_key` credential field and one `base_url`
    setting field in the provider definition. Bind `BAIDU_API_KEY` through
    catalog metadata for explicit static or paid-live inputs.
  - Use the shared Chat Completions protocol adapter. Declare the Qianfan
    response policy in catalog data. `finish_reason=stop` completes, `length`
    enters the common
    missing-suffix coordinator, and other reasons fail safely. When `flag` is
    present, accept only documented continue values `0` and `1`. Reject `2`,
    `3`, `4`, and unknown values without exposing partial text. Apply the same
    structural and safety checks to provider-key verification.
  - Expose Baidu through the generic authenticated provider-key operation.
    Render Baidu in the provider editor from its provider definition. A paste
    must automatically verify the exact selected Qianfan model before the
    encrypted key, provider settings, and eligible routing default are
    atomically saved.
    Keep the raw key, authorization header, and Qianfan body out of responses
    and logs.
  - Project the new provider offerings through the canonical management
    profile, routing selectors, public capability REST resource, frontend-owned
    routing graph and model catalog, usage dimensions, and examples. Make Baidu
    available to official clients through the existing provider selector.
    Generate each view from the canonical provider and model inventory.
  - Keep the initial integration text-only and non-streaming. Qianfan visual
    input, deep-thinking controls, configurable reasoning effort, tools,
    structured output, search results, and streaming require separate explicit
    capability issues. Apply the same rule to AppBuilder, ModelBuilder, custom
    deployments, and dictation.
  Deliverables:
  - Add one Baidu provider definition with its provider offerings, Qianfan
    response policy, credential fields, settings, and live-test metadata.
  - Regenerate the OpenAPI artifact, public artifacts, documentation,
    environment examples, and black-box fixtures from the provider catalog.
  Validation:
  - Fake-Qianfan public-boundary scenarios prove the exact URL and a redacted
    bearer header. Cover ordered messages, selected model, `max_tokens`, usage,
    finish reasons, safe `flag` handling, sanitized errors, and timeout. Cover
    capability rejection through `GET /`, compatibility `POST /`, and
    canonical `POST /v2`.
  - Prove exact catalog membership and limits through configuration tests.
  - Apply the current managed-only runtime contract.
  - Reject Baidu requests without a saved tenant key before dispatch.
  - Reject config-level provider-key blocks.
  - Management API and Playwright scenarios prove a user can select Baidu,
    paste a key, see verification state, and save after successful Qianfan
    verification. Prove the user can choose a Qianfan model and default route.
    Keep credentials absent from profiles, examples, browser storage, logs,
    and public data.
  - Public catalog coverage proves each DeepSeek model has distinct direct and
    Qianfan provider offerings without duplicate exact-model records and that
    `ernie-5.0` appears as Baidu's default text offering.
  - Extend the live-provider preflight without an upstream call. During
    implementation acceptance, run one explicitly paid Baidu key verification
    and one small canonical text request with
    `LLM_PROXY_LIVE_PROVIDERS=baidu`. Keep the key and response body out of test
    output. Run final `make ci` after the last stack change, as POLICY requires.
    Keep deployment and production acceptance operator-owned.
  Resolution (2026-09-05):
  Baidu Qianfan now exposes the four requested text offerings through one managed provider connection.
  `ernie-5.0` is the provider default.
  The catalog records output limits, international prices, credential metadata, and the default API URL.
  The DeepSeek V4 offerings reuse the direct provider's exact model records.
  The shared Chat Completions adapter applies the catalog `qianfan` response policy.
  It validates finish reasons and every choice flag before it exposes text.
  Key verification uses the same parser before the connection and eligible defaults are saved.
  The current managed-only contract supersedes the original static-key requirement.
  Public HTTP tests cover all four models, continuation, usage, unsupported capabilities, invalid responses, sanitized errors, and timeout.
  Managed API tests prove encrypted storage and unchanged state after rejected verification.
  Browser tests prove model selection, key-paste verification, URL defaults, and saved routing defaults.
  The initial HTTP test returned `400 unknown provider: baidu`.
  The first catalog load rejected a key-acquisition URL fragment.
  The provider now links to the official Qianfan Quick Start.
  The first CI run stopped on a stale browser expectation of `1 of 61 model`.
  The corrected expectation uses 63 exact models and passed its focused browser test.
  Final CI passed all 12 gates and 97 browser tests with 100.0% Go statement coverage in 236 seconds.
  B196 corrected the live harness for the catalog URL default.
  Paid default-model verification and canonical text generation both returned HTTP 200 with the existing repository key.
  The other three models have local protocol coverage. This run did not qualify them against live Qianfan.
  Production deployment and acceptance remain operator-owned.
  Evidence: `/tmp/llm-proxy-f032-red.log`, `/tmp/llm-proxy-f032-focused.log`, `/tmp/llm-proxy-f032-live.log`, and `/tmp/llm-proxy-f032-ci-final.log`.
  Changed files:
  `configs/providers.yml`, `configs/.env.sample`, `internal/proxy/provider_catalog_schema.go`,
  `internal/proxy/provider_types.go`, `internal/proxy/provider_registry.go`, `internal/proxy/openai_compatible_chat.go`,
  `internal/proxy/qianfan_response_policy.go`, `internal/proxy/provider_key_verifier.go`,
  `internal/proxy/managed_router_test.go`, `internal/proxy/baidu_test.go`, `internal/proxy/baidu_management_test.go`,
  `internal/proxy/openapi_contract_test.go`, `cmd/cli/root_test.go`, `tests/capabilities_test.go`,
  `tests/e2e/management-ui.spec.js`, `Makefile`, `README.md`, `docs/provider-catalog.md`, and `docs/baidu-qianfan.md`.
  Public discovery adds Baidu and four offerings. Public event schemas did not change.

## Planning

### Complete entries archived 2026-08-10

- [x] [P004] (P1) {F019,P003} Make Resources an always-available footer surface and enforce the resource-page shell.
  Goal:
  Make the public Resources entry point continuously discoverable from the
  shared footer, and make every public resource page use one unambiguous
  document order: header, resource content, then footer.
  Requirements:
  - Render a semantic `Resources` navigation section in the shared public
    footer on the landing page, the resource hub, and every generated public
    resource page. It must contain a descriptive, crawlable anchor to the
    canonical `/resources/` hub; it must not depend on JavaScript interaction,
    a sitemap, or an authenticated `/app/` page to discover the resources.
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
  - Preserve F019's public-root versus private-`/app/` separation and
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
    a normal crawlable anchor and keeps `/app/`, APIs, secrets, redirects,
    and noindex pages out of resource navigation.
  - Run the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci`
    pair for the implementation, with the final run after the last code edit.
  Resolution:
  - The deterministic shared shell now gives every public route a crawlable
    no-JavaScript `/resources/` footer link and canonical header-main-footer
    ordering.
  - Generator checks and browser coverage enforce the shared shell, canonical
    resource target, safe public navigation, and responsive desktop/mobile
    layout across the landing page, hub, and generated resource pages.
  - B111's required baseline and final CI runs provide the implementation
    validation; the final run passed all 11 gates in 100 seconds.
- [x] [P005] (P1) {F019,P004} Normalize public Privacy and Terms pages using PoodleScanner's legal-page contract as the structural reference.
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
    F019 and P004: one shared header, one `main` element containing the legal
    document, and the shared footer. The footer must expose descriptive,
    crawlable `Privacy` and `Terms` links on the landing page, `/app/`, the
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
  Resolution:
  - One repository-owned legal source now generates canonical `/privacy/` and
    `/terms/` pages with route-specific metadata, effective/updated dates,
    semantic static fallback content, and the current MPR legal component.
  - The policies use verified LLM Proxy storage, usage, authentication,
    provider, Google Analytics, and LoopAware boundaries plus the canonical MPR
    legal profile; shared footer links and sitemap discovery stay synchronized.
  - Static, hydrated, narrow-screen, link, generation, and TAuth browser
    coverage passed in B111's final 11-gate CI run in 100 seconds.

- [x] [P007] (P1) Select Alibaba Model Studio for backend API access.
  Goal:
  Select one Alibaba service that supports LLM Proxy application traffic.
  Decision:
  - Keep `dashscope` as the only Alibaba backend provider.
  - Use Alibaba Cloud Model Studio pay-as-you-go billing.
  - Use a Model Studio API key and base URL from the same region.
  - Use the Singapore workspace URL in production:
    `https://{workspace-id}.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1`.
  - Keep `qwen-plus` and synchronous Chat Completions until I038 or I221 changes
    that exact provider offering.
  - Use only the provider HTTP API. Alibaba console and interactive-tool
    integration are out of scope.
  - Retire `qwencloud` because Alibaba prohibits Token Plan use by application
    backends and automated scripts.
  Consequences:
  - I039 removes `qwencloud` from runtime, configuration, management, public
    discovery, tests, documentation, and live-provider tooling.
  - I039 binds the production workspace URL from the private deployment input.
  - I039 deletes stored `qwencloud` credentials and provider settings through a
    bounded schema-version-4 migration.
  - The migration reconciles affected text routes with remaining keyed
    providers. It clears the route when no provider key remains.
  - The migration preserves tenant timestamps and historical usage records.
  - No `qwencloud` value becomes a `dashscope` credential, alias, route, model,
    or provider setting.
  Validation:
  - The product owner approved the backend-only selection rule on 2026-08-10.
  - Alibaba identifies Model Studio pay-as-you-go as its custom-application
    service:
    https://www.alibabacloud.com/help/en/model-studio/more-tools
  - Alibaba prohibits Token Plan use for automated scripts and application
    backends:
    https://www.alibabacloud.com/help/en/model-studio/token-plan-overview
  - Alibaba recommends a workspace domain for production pay-as-you-go traffic:
    https://www.alibabacloud.com/help/en/model-studio/base-url
  Resolved 2026-08-10:
  - Selected `dashscope` with Model Studio pay-as-you-go billing.
  - Assigned provider removal and bounded data migration to I039.
