# ISSUES

Entries record newly discovered requests or changes.

Read @AGENTS.md (Workflow section), @POLICY.md, and relevant stack guides before implementing changes.

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

## BugFixes

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
- [ ] [B141] (P1) Center the X icon inside the top-right square.
  Goal:
  Align the X icon so it is visually centered within the square control in the top-right corner, matching the intended UI layout shown in the attached screenshot.
  
  Requirements:
  Preserve the existing square size, placement, styling, and click/interaction behavior. Adjust only the icon positioning/alignment needed to center the X horizontally and vertically within its container. Ensure the fix remains responsive and does not introduce layout shifts in nearby content.
  
  Deliverables:
  Code changes that correct the X icon alignment in the top-right square control. Include any related style/layout updates needed for consistent centering across supported viewports.
  
  Validation:
  Open the affected screen and confirm the X appears centered within the top-right square. Verify the square remains in the same top-right position, the control still functions as before, and no surrounding UI elements are visually displaced.


## Improvements

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

- [ ] [I240] (P0) Add first-class client protocol adapters.
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

- [!] [I239] (P1) Standardize HTTP health at `/healthz`.
  Goal:
  Make `/healthz` the canonical health endpoint for the LLM Proxy API and
  static web origins. Use the endpoint for readiness without business-resource requests.

  Requirements:
  - Add unauthenticated `GET /healthz` to the API origin.
  - Publish a static `/healthz` resource for the GitHub Pages origin.
  - Return `200` only when each origin can serve its current application contract.
  - Return a non-success status when a required runtime dependency prevents API service.
  - Send `Cache-Control: no-store` on every health response.
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
  - Verify unauthenticated `GET /healthz` returns `200` and `Cache-Control: no-store` on each origin.
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
  Blocked:
  GitHub Pages controls production response headers. The static origin cannot
  meet the no-store requirement without a hosting or cache-policy decision.

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
- [ ] [I235] (P1) Add explicit model activation to the provider catalog.
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
- [!] [I233] (P1) Add Gemini candidate models to the live test.
  Goal:
  Make the Gemini live test validate two stable candidates before public route
  registration.
  Evidence:
  - I207 remains blocked because active retrieval and cancellation did not
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
  Blocked: The long Gemini 3.6 Flash cancellation resource was not readable
  during any of seven bounded active retrieval attempts. Google returned HTTP
  400 on the final attempt. The harness stopped before cancellation and before
  the Gemini 3.7 Flash matrix. I207 remains blocked, and neither candidate is
  registered in the public provider catalog.
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
- [ ] [I041] (P1) Migrate xAI text routes to Responses without OpenAI background assumptions.
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
  Live acceptance cannot start because `MODEL_API_KEY` is absent from the
  process and each authorized repository environment file. The selected
  harness stops before provider dispatch. The operator must supply
  `MODEL_API_KEY` and authorize authenticated `GET /v1/models`, one key
  verification, and one text request.
- [ ] [I046] (P1) Make upstream admission fair across provider origins.
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
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair for the implementation, with
    the final run after the last code edit.
- [!] [I207] (P1) Add Gemini 3.6 and 3.7 Flash with route-bound Interactions thinking levels.
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
  Blocked: The new credential passed each declared reasoning request for both
  exact models. Both paid lifecycle probes created an `in_progress` resource.
  Google returned `403` and then `400` for every active retrieval attempt.
  Gemini 3.6 became readable only after completion. Gemini 3.7 stayed
  unreadable through the bounded probe. Both cleanup delete requests returned
  HTTP 200. Neither model supplied the required active retrieve and cancel
  proof. The provider catalog does not register these two routes.
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
- [ ] [I038] (P2) Adopt DashScope's synchronous Responses API without background mode.
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

- [ ] [M021] (P1) {F024,F025,F026,F027} Remove the completed MediaOps operation-import bridge.
  Goal:
  Leave only the canonical model-operation contract after migration of every
  selected MediaOps provider record.
  Cross-repository prerequisite:
  - MediaOps M227 must produce the operator-held final per-tenant migration
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
    enters through plan plus idempotent create.
  Validation:
  - Run static contract checks and public black-box tests proving no migration
    entrypoint or legacy record shape remains and migrated operations retain
    status, tenant isolation, recovery, and artifact behavior.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
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
  Last run: 2026-08-30.
  active tracker and archived 34 BugFixes, seven Improvements, one
  Maintenance, and three Features. Kept all 49 open, blocked, Planning,
  and recurring entries active. Confirmed eight recurring Maintenance
  entries remain open with the `R` suffix. Removed six satisfied I223
  dependency markers from active entries. Filed no follow-up issues.
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
  - Check links, command names, paths, and public contract descriptions touched by the pass.
  - Confirm docs describe the current canonical path only.
  - Confirm issue archive and active tracker references remain consistent.
  Last run: 2026-08-10. Reviewed README, OpenAPI, provider routing, dictation,
  CHANGELOG, and generated resources against 80 resolved issues. Added missing
  Unreleased summaries for current public, provider, media, and client
  contracts. Replaced the obsolete dictation implementation plan with its
  current endpoint contract. Regenerated 46 SEO resources. Their content
  already matched the current source. `PRD.md` and `ARCHITECTURE.md` remain
  absent, and M013 tracks that decision.
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
- [ ] [F032] (P1) Add Baidu Qianfan as a user-configurable text provider.
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
  - Configuration tests prove exact catalog membership and limits, a missing
    non-default static key leaves Baidu disabled, a static Baidu default
    requires its key, and managed mode rejects config-level provider keys.
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
    output. Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair. Keep deployment and
    production acceptance operator-owned.
- [ ] [F022] (P1) Add the durable model-operation, asset, and official-client foundation.
  Goal:
  Extend LLM Proxy from blocking text and dictation into a shared model-provider
  data plane while keeping MediaOps and other callers responsible for their
  product operations.
  Progress:
  - F033 added the tenant asset upload, store, reference, and official-client
    foundation.
  - The durable model-operation and worker foundation remains open.
  Requirements:
  - Keep `/v2` as the canonical blocking messages contract and `/v3` as the
    planned sampling contract. Add a distinct `/model/v1` namespace to the
    canonical OpenAPI document.
  - Define `GET /model/v1/capabilities`, `POST /model/v1/plans`,
    `POST /model/v1/operations`, and
    `GET /model/v1/operations/{operation_id}` with strict request and response
    schemas and the existing tenant-client-key security boundary.
  - Make plans provider-call-free and immutable. A plan must contain its exact
    normalized request, resolved provider/model, catalog revision, estimated
    price result, expiry, and `plan_id`.
  - Accept one required `Idempotency-Key` when creating an operation. Repeating
    the same key and intent must return the existing operation; reusing the key
    with a different intent digest must return a canonical conflict.
  - Persist the operation and normalized intent before provider dispatch. Use
    exactly `not_dispatched`, `dispatched`, `succeeded`, `failed`, and
    `uncertain` as provider-execution states.
  - Advance to `dispatched` immediately before the provider boundary. Resume a
    `dispatched` or `uncertain` operation only through its stored provider
    handle; automatic provider resubmission is outside the contract.
  - Add a durable worker lease/recovery model that safely resumes
    `not_dispatched` work after restart and preserves terminal records.
  - Add a temporary operator-only, non-HTTP import command for strict
    tenant-scoped migration manifests. It must create canonical operation rows
    without provider dispatch, require a family-specific validator, converge by
    source-record digest, and emit a deterministic source-to-operation receipt
    with the manifest digest. M021 removes this bridge after all MediaOps family
    cutovers complete.
  - Store provider-native task, request, response, interaction, history, and
    file handles only in the encrypted server-side operation record. Return one
    proxy operation identifier to callers.
  - Support typed tenant provider credential profiles, including API keys and
    Google Cloud workload-identity/service-account references with exact
    project and location metadata. Publish availability without secret values.
  - Keep the durable operation ledger separate from the current at-most-once
    managed usage telemetry.
  - Return sanitized, correlated errors with provider, model, proxy request id,
    retryability, and exact pre-dispatch or post-dispatch classification.
  - Add `POST /model/v1/assets` for bounded streaming upload and
    `GET /model/v1/artifacts/{artifact_id}` for authenticated download through
    opaque tenant-scoped identifiers.
  - Record MIME type, byte size, ownership, creation time, retention
    expiry, and provider-readable staging state for every asset.
  - Add a strict object-store configuration with a filesystem fixture backend
    and a GCS production backend for provider staging.
  - Stream large bytes through the asset store and use asset identifiers in
    operation payloads. Keep local filesystem paths at the caller boundary.
  - Materialize provider outputs into gateway-owned artifacts before reporting
    operation success unless the catalog declares a durable provider artifact
    that the gateway can retrieve on demand.
  - Enforce tenant isolation, bounded uploads/downloads, MIME validation,
    expiry, and deterministic cleanup.
  - Extend `pkg/llmproxyclient` with validated constructors and typed
    `PlanOperation`, `CreateOperation`, `GetOperation`, `UploadAsset`, and
    `DownloadArtifact` APIs.
  - Release the official Go client before a downstream application begins its
    integration foundation.
  Deliverables:
  - Add the OpenAPI schemas, transport-neutral operation service, durable store
    migrations, worker, idempotency index, typed credential profiles, and
    operation-status handlers plus the temporary import framework.
  - Add the asset handlers, object-store abstraction, cleanup worker, official
    Go client surface, examples, and release notes.
  - Document the ownership boundary, state machine, plan/execute flow,
    retention policy, and restart behavior.
  Validation:
  - Prove persist-before-dispatch, duplicate-key convergence, intent conflict,
    restart recovery, worker lease expiry, tenant isolation, and each terminal
    state through public handlers and fake providers.
  - Prove cancellation and transport loss after dispatch become `uncertain`
    with reusable provider evidence.
  - Prove logs, responses, and usage records exclude credentials, raw provider
    bodies, prompts, generated media, and provider-native handles.
  - Use a local fake server to prove every official-client path, authentication
    shape, idempotency header, typed error, and streaming cancellation path.
  - Prove truncated uploads, oversized media, cross-tenant
    reads, expired assets, interrupted downloads, and cleanup races fail with
    durable and sanitized evidence.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
- [ ] [F024] (P1) {F022} Add image generation and editing to model operations.
  Goal:
  Make LLM Proxy the sole provider boundary for the current OpenAI, Vertex, and
  FAL image-generation and image-editing routes.
  Cross-repository sequence:
  - MediaOps I069 consumes this released image slice as its first cutover.
  Requirements:
  - Add typed `image.generate` and `image.edit` schemas covering the current
    prompts, image and mask roles, output counts, sizes, aspects, quality,
    background, formats, compression, OpenAI Images/Responses controls, and
    supported multi-image inputs.
  - Add `openai`, `vertex`, and `fal` provider adapters backed by tenant-owned
    credential profiles and the authoritative model-operation catalog.
  - Convert uploaded image asset identifiers into each provider's exact input
    shape and materialize all terminal output bytes as gateway artifacts.
  - Preserve FAL queue request/response handles in the durable operation and
    resolve recovery through the proxy operation id.
  - Register the exact recoverable FAL image record shapes with the temporary
    import command, classify their current state without dispatch, and return a
    canonical gateway operation mapping to the MediaOps cutover.
  - Record normalized usage and exact price evidence without treating either
    as billing settlement.
  Deliverables:
  - Add provider adapters, catalog entries, plan validation, recovery handlers,
    fake upstream fixtures, official-client request types, and public docs.
  Validation:
  - Prove generate/edit parity for every current route, multi-output ordering,
    asset integrity, provider error sanitization, restart recovery, and
    idempotent duplicate submission through public black-box tests.
  - Run one explicitly enabled minimal live request per provider after the fake
    provider suite and repository CI pass.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
- [ ] [F025] (P1) {F022} Add durable video generation to model operations.
  Goal:
  Make LLM Proxy the sole provider boundary for Vertex Veo, Vertex Gemini Omni,
  Runway, FAL, Kling, and xAI video generation.
  Cross-repository sequence:
  - MediaOps I071 consumes this released video slice independently of the
    image cutover.
  Requirements:
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
  - Prove every text, keyframe, reference, extension, and source-video mode,
    long polling across restart, uncertain transport recovery, output hash and
    MIME validation, and provider resource cleanup.
  - Run the existing minimal opt-in live canaries through LLM Proxy after the
    fake provider suite and repository CI pass.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
- [ ] [F026] (P1) {F022} Add ElevenLabs speech, music, and alignment operations.
  Goal:
  Make LLM Proxy the sole external-provider boundary for the current
  ElevenLabs account while MediaOps retains narration planning and local audio
  assembly.
  Cross-repository sequence:
  - MediaOps I072 consumes this released audio slice independently of the
    image and video cutovers.
  Requirements:
  - Add typed operations for speech generation, speech conversion, voice
    discovery, history listing/download, prompt music, composition plans,
    detailed and streamed composition, composition upload, video-to-music,
    stem separation, and forced alignment.
  - Preserve exact voice/model settings, pronunciation dictionaries,
    continuity context, timestamps, seed, normalization, pacing/speed
    translation, formats, provider concurrency, and history identifiers.
  - Keep MediaOps-owned render-plan chunking, narrative cadence, deterministic
    chunk reuse, stitching, and final composite validation outside the gateway;
    each provider request becomes one durable gateway operation.
  - Materialize provider audio and JSON outputs as typed artifacts and retain
    history or song identifiers as internal recovery evidence.
  - Keep Dictator as a separately consumed MPR gRPC service.
  - Register exact import validators for selected ElevenLabs history and
    provider-operation records and return canonical operation mappings without
    replaying generation or mutation.
  Deliverables:
  - Add the ElevenLabs credential profile, adapters, catalogs, prices,
    official-client types, recovery handlers, fake upstream fixtures, and docs.
  Validation:
  - Prove all read-only, paid, upload, streaming, and recovery paths; bounded
    concurrency; cancellation; exact output order; and artifact integrity.
  - Run explicitly enabled minimal ElevenLabs live acceptance only after the
    fake provider suite and repository CI pass.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
- [ ] [F027] (P1) {F022} Add provider account mutations, avatars, translation, and lip-sync.
  Goal:
  Complete gateway ownership of external media-provider credentials and
  provider-native task recovery for HeyGen and Kling account operations.
  Cross-repository sequence:
  - MediaOps I073 consumes this released account-operation slice independently
    of the other current-provider cutovers.
  Requirements:
  - Add typed operations for HeyGen translation, existing-video lip-sync,
    photo-avatar creation, motion enhancement, avatar-video generation, quota
    reads, and Kling/HeyGen lip-sync where currently supported.
  - Add typed Kling reusable-asset create/list/get/delete operations with exact
    mutation and destructive classifications.
  - Require immutable plans and idempotency for paid and mutating work. Preserve
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
  - Prove read-only versus paid versus mutating versus destructive behavior,
    duplicate idempotency, restart recovery, asset ownership, quota reporting,
    and sanitized errors through public black-box tests.
  - Run explicitly enabled minimal live acceptance only after the fake provider
    suite and repository CI pass.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
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
- [ ] [F021] (P1) Add OAuth-authenticated tenant-scoped MCP access.
  Goal:
  An authenticated user can connect a remote MCP client to one owned tenant.
  The server uses that tenant's saved provider credentials, routing defaults,
  and standard text-generation lifecycle for each MCP request.
  Current contract:
  - Public proxy requests use a generated tenant secret in `key=...`.
  - TAuth browser sessions authorize management operations.
  - Provider credentials remain on the server and belong to one tenant.
  - TAuth provides the OAuth authorization-server contract that a remote MCP
    client requires.
  - The gateway generates the root OAuth block and the tenant OAuth policy.
  - The deployment manifest declares the protected API origin and the
    `llm-proxy:use` scope.
  Requirements:
  - Serve MCP protocol version `2026-07-28` at the exact resource URL
    `https://llm-proxy-api.mprlab.com/mcp/{tenant_id}`.
  - Use the official Go MCP SDK in stateless Streamable HTTP mode. Return JSON
    responses and reject every unsupported protocol version.
  - Do not create an MCP session or implement an earlier MCP transport.
  - Use `https://llm-proxy-api.mprlab.com` as the OAuth protected-resource
    identifier and access-token audience.
  - Confirm the token subject owns the exact `{tenant_id}` path resource.
  - Publish path-specific OAuth Protected Resource Metadata at
    `/.well-known/oauth-protected-resource/mcp/{tenant_id}`.
  - Return an RFC-compliant Bearer challenge for an unauthenticated MCP
    request. Include the protected-resource metadata URL in the challenge.
  - Validate each JWT access token signature, issuer, subject, exact audience,
    expiry, and required `llm-proxy:use` scope at the HTTP edge.
  - Confirm that the token subject owns `{tenant_id}` before an MCP operation.
    Return the same `404 Not Found` result for a missing tenant and a foreign
    tenant after successful token validation.
  - Keep provider credentials on the server. Never return provider credentials,
    tenant secrets, access tokens, refresh tokens, or session data through MCP.
  - Expose one tool named `llm_proxy.generate_text`. Require `messages` and
    accept optional `provider`, `model`, `web_search`, `max_tokens`,
    `reasoning_effort`, and `request_timeout_seconds` inputs.
  - Use the canonical `/v2` message and attachment contract for the tool.
    Preserve ordered image and audio attachments on user messages.
  - Return generated text and structured `request_id`, `provider`, `model`,
    `usage`, and `request_timeout_seconds` fields from a successful tool call.
  - Mark the tool as not read-only, not destructive, not idempotent, and
    open-world because one call can create provider charges.
  - Return a sanitized MCP tool error with `isError: true` for an accepted tool
    call that fails. Preserve the canonical proxy error classification without
    exposing an upstream body, provider message, prompt, response, or secret.
  - Expose `llm-proxy://routes` as a tenant-scoped resource. Return only the
    tenant's configured text routes, defaults, and declared capabilities.
  - Keep management and dictation outside this first MCP contract.
  - Extract one transport-neutral text-generation service from the current
    HTTP handlers. Make `/v2` and MCP use the same routing, admission, queue,
    rate-limit, timeout, cancellation, continuation, error, and usage logic.
  - Record MCP usage with endpoint value `mcp`. Record the logical proxy result
    status when an MCP tool error uses a successful HTTP transport response.
  - Add a `Copy MCP URL` action for each owned tenant. Show the action only
    after the tenant has at least one configured text route.
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
  - Add the shared text-generation service, `llm_proxy.generate_text` tool,
    and `llm-proxy://routes` resource.
  - Add tenant UI copy, configuration, OpenAPI, deployment, and user-guide
    updates for the MCP connection contract.
  - Add black-box integration and browser coverage through public entry points.
  Validation:
  - Run an official MCP client against the real local HTTP route, real local
    TAuth OAuth flow, and a fake upstream provider.
  - Verify successful authorization-code and PKCE login, token refresh, token
    revocation, and consent behavior.
  - Verify missing, malformed, expired, wrong-issuer, wrong-audience, and
    wrong-scope tokens. Verify missing and foreign tenant identifiers.
  - Verify exact tool discovery, input-schema validation, route selection,
    defaults, media inputs, structured success output, and route-resource data.
  - Verify queue rejection, provider rate limits, request timeouts, caller
    cancellation, sanitized tool errors, and managed usage records.
  - Verify that logs, MCP results, OAuth responses, and browser content contain
    no provider credential, tenant secret, token, prompt, or generated response.
  - Run `/v2` regression scenarios to prove one shared execution lifecycle and
    unchanged tenant-secret authentication for the REST contract.
  - Use MCP Inspector and one supported remote MCP client for manual local
    acceptance. Record live-host acceptance as a separate deployment result.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
  Dependency handoff: 2026-08-15 — gateway F001 and both application manifests
  passed local contract validation. Production activation remains separate.
- [ ] [F028] (P2) {F027} Add HeyGen Avatar V as a gateway-owned avatar engine.
  Goal:
  Add the current Avatar V engine to the gateway HeyGen avatar contract before
  MediaOps exposes it through its product surfaces.
  Cross-repository sequence:
  - MediaOps I066 consumes this after its I073 base HeyGen/Kling cutover.
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
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
- [ ] [F029] (P2) {F025} Add MiniMax H3 V2 video generation to model operations.
  Goal:
  Add the provider-qualified MiniMax H3 V2 route to the gateway before MediaOps
  exposes the model through its video product contracts.
  Cross-repository sequence:
  - MediaOps I060 consumes this after its I071 base video cutover.
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
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
- [ ] [F030] (P2) {F026} Add Speechify text-to-speech and voice discovery to model operations.
  Goal:
  Add the current Speechify complete-response speech and voice-discovery
  contracts to the gateway before MediaOps exposes them in narration flows.
  Cross-repository sequence:
  - MediaOps I058 consumes this after its I072 base audio cutover.
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
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
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
*do not implement yet*

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
  - Move Dictator capabilities through the LLM Proxy boundary:
    - Inventory transcription, diarization, subtitle generation, alignment,
      speech synthesis, reference-sample extraction, artifacts, and job APIs
      against the current Dictator gRPC contract.
    - Define resource-oriented LLM Proxy operations for the retained
      capabilities. Do not expose Dictator gRPC names, internal artifact IDs,
      storage paths, or job IDs in the public contract.
    - Make LLM Proxy own tenant assets and public operation IDs. Define the
      private mapping and transfer contract for Dictator inputs, outputs,
      progress, cancellation, failures, and restart reconciliation.
    - Keep a Dictator runtime resident while one of its accepted background
      jobs is active. Add an exact drain and activity signal before the node
      controller can stop it.
    - Decide whether one Dictator container profile can satisfy measured GPU
      limits. Split transcription, analysis, and synthesis into separate
      profiles when one process keeps incompatible models resident.
    - Decide whether the current `/dictate` route becomes the canonical local
      transcription resource or is removed in the same forward migration.
      Do not create a second permanent transcription contract.
  - Define the public LLM Proxy contract as one atomic release boundary:
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
    - Update F026 and its dependent speech issues before implementation. F026
      currently requires clients to consume Dictator separately, which
      conflicts with this target boundary.
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
    - Select public asynchronous operation storage and restart behavior. State
      which component is authoritative for each state transition.
    - Select the exact SAM prompt set, coordinate system, mask format, maximum
      image size, maximum instance count, and retention limits.
    - Select the Dictator profile split, artifact transfer method, public
      operation shapes, `/dictate` disposition, and bounded caller cutover.
    - Decide whether local usage has a billable price, an internal cost only,
      or no price. Keep the decision explicit in catalog and usage contracts.
  - Use this implementation sequence after the architecture is approved:
    1. Qualify the hardware and each pinned runtime on `computercat`. Measure
       Qwen at 16K and 32K, SAM prompt and output cases, each Dictator profile,
       repeated runtime switches, and complete GPU-memory release.
    2. Approve public API ownership, catalog identities, asset ownership,
       operation state, private control, deployment, and security contracts.
       Update the planning records for F020, F026, and F030 where required.
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
    7. Add the retained Dictator speech resources and private adapter. Migrate
       public operation and asset ownership to LLM Proxy.
    8. Deploy the private controller and runtime profiles on `computercat`.
       Remove public runtime ports and routes. Install only the credentials and
       model artifacts that each component requires.
    9. Migrate every first-party caller to the released official LLM Proxy
       client. Remove direct Dictator clients, configuration, public DNS and
       route ownership, and obsolete integration contract text in the same
       bounded cutover.
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
  - Approve one deletion receipt for the direct Dictator public route, direct
    first-party clients, old configuration, and obsolete contract text. Do not
    mark this planning issue complete from a partial or dual-path migration.
- [ ] [P001] (P1) Design a tenant-scoped provider, model, and key-acquisition onboarding flow.
  Goal:
  Let a signed-in managed user complete one clear text-routing setup: select a
  supported provider, select one of that provider's supported text models, and
  either paste an existing provider API key or open that provider's official
  key-acquisition page in a new window before returning to paste it. A completed
  setup must make the chosen provider/model the Settings tenant's usable text
  route without asking the user to reconcile separate provider, default, and
  client-secret forms.
  Requirements:
  - Build the flow inside the current editor-only `Settings tenant` context.
    It must read and write only that selected tenant and must not change the
    independent `Usage tenant` filter. Another tenant or user must never
    inherit a provider key, model choice, in-progress form value, or completion
    state.
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
