# Resolved Issue Archive

This is historical context, not an active backlog. `.mprlab/ISSUES.md`
contains only current active, blocked, planning, and recurring work.

Archive date: 2026-07-24
Source: the `v0.2.43` release snapshot. It retains every original issue body,
resolution note, and validation record:
`git show v0.2.43:.mprlab/ISSUES.md`

`CHANGELOG.md` remains the release-level history. This index keeps completed
issue titles discoverable without making the active tracker noisy.

## Archive conventions

- Resolved non-recurring issues are indexed below by their original tracker section.
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

## Features

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
