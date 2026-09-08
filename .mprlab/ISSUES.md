# ISSUES

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

## BugFixes

- [x] [B205] (P2) Sequence the key persistence test across router reloads.
  Observed: GitHub run `34264915879` failed at commit `b2731cb`.
  `TestManagementProviderKeyRevealPersistsUpdatedKey` expected two usage records but observed one before its one-second deadline.
  The test sent requests through two database runtimes before either usage write completed.
  The second write continued after test cleanup started.
  Requirements: Verify each usage record before the next test phase.
  Construct the second router after the first request and its usage write complete.
  Retain the key, response, usage, and corrupt-record assertions. Keep the current timeout.
  Validation: Run focused HTTP tests with coverage and the race detector. Run `make ci`.
  Resolution: The test now verifies the first usage record before it constructs the second router.
  It verifies both records after the second request. All previous assertions remain in place.
  Added `make test-management-persistence` for the related HTTP tests.
  Focused tests passed with coverage and the race detector.
  `make ci` passed all 12 gates with 100% Go statement coverage.
  The Governor check and changed-prose review passed. Application and event contracts did not change.
  The changes remain uncommitted. GitHub did not run the fix.

- [x] [B204] (P2) Bound MCP request uploads in time.
  Observed: An authenticated client can leave an incomplete `/mcp` upload open indefinitely.
  The SDK waits for the body before the F021 generation timeout starts.
  The initial HTTP regression exceeded the one-second server budget and failed
  with `stalled MCP upload did not terminate within the server budget`.
  Requirements: Apply the server default timeout before SDK body processing.
  Stop reads on cancellation. Clear the upload deadline before generation.
  Return HTTP `408` for an upload timeout and retain the byte limit.
  Validation: Use real HTTP uploads and SDK calls. Run `make test-mcp` and `make ci`.
  Resolution: Added a cancellable socket read deadline before SDK dispatch.
  Cleared that deadline before generation. Retained the request byte limit.
  Verified stalled uploads, cancellation, read errors, deadline errors, and longer generation budgets.
  `make test-mcp` and `GOFLAGS=-race make test-mcp` passed.
  `make ci` passed all 12 gates with 100% Go coverage.
  Updated the MCP guide and OpenAPI reference. Usage event contracts did not change.
  The initial Governor check reported format template drift. I254 records its correction.

## Improvements

- [ ] [I256] (P1) Add shared public scenarios for protocol acceptance.
  Goal:
  Add reusable acceptance evidence before changes to the provider catalog and protocol adapters.
  Requirements:
  - Extend the current catalog tests and provider HTTP fixtures through the real service and official Go client.
  - Cover Chat Completions, Responses, Anthropic Messages, Google native protocols, and current dictation adapters.
  - Record each supported operation, control, protocol variation, and lifecycle in the test cases.
  - Exercise text, media input, caller tools, structured output, errors, usage, and continuation where the current route supports them.
  - Prove rejected requests cause zero dispatch for unsupported controls and invalid credentials.
  - Use controlled external protocols for timeout, cancellation, visibility retry, and terminal-state tests.
  - Add a second provider definition for each executable protocol adapter through disposable catalog data and connection values.
  - Use the same executable and clients for both provider definitions.
  - Use public tests for discovery, connection forms, secret protection, routing, restart persistence, tenant isolation, and usage.
  - Keep provider constants and production registrations outside the second-provider fixture change.
  - Keep live-provider qualification separate from local protocol acceptance.
  Deliverables:
  - Shared protocol tests, a repository Make target, and inclusion in the applicable CI gate.
  - A documented procedure to qualify another provider through catalog data.
  Validation:
  - Run existing public tests before extraction. Add characterization tests where public coverage is absent.
  - Prove that the suite detects a wrong endpoint, unsupported field, malformed result, and incorrect usage total.
  - Run the focused target and applicable repository checks.
  - Keep all current runtime and public API behavior unchanged.

- [ ] [I257] (P1) {I256} Remove duplicated protocol facts from provider catalog records.
  Goal:
  Give each protocol invariant one authoritative definition and reduce provider additions to their required data.
  The catalog currently repeats adapter constants that `validateProviderCatalogAdapterContract` compares with Go definitions.
  Requirements:
  - Keep `configs/providers.yml` as the sole provider catalog.
  - Select one protocol definition per transport instead of three identifiers that must be equal.
  - Give each codec one definition for its fixed response fields, finish rules, error rules, and usage mapping.
  - Derive validation and runtime metadata from that definition.
  - Keep endpoints, credential fields, settings, offerings, controls, limits, prices, and supported protocol variations in provider data.
  - Keep every retained variation typed and validate it at the catalog boundary.
  - Remove repeated invariant values from all provider records and test catalogs in one coordinated schema change.
  - Reject obsolete fields and shapes. Keep one accepted schema without aliases, dual reads, or compatibility parsers.
  - Keep provider identities, tenant connections, route defaults, and historical usage.
  - Rebuild all discovery, management, and live-harness projections from the normalized registry.
  - Update the catalog reference, onboarding procedure, and affected configuration examples.
  Deliverables:
  - A reduced catalog schema, authoritative codec definitions, and converted catalog consumers.
  Validation:
  - Run I256 characterization tests before production changes.
  - First prove rejection of the new catalog shape, then implement its loader and conversion.
  - Prove rejection of obsolete shapes, unknown variations, missing references, and invalid combinations.
  - Prove equivalent public discovery, credentials, requests, responses, and usage for every active route.
  - Run the focused catalog target and applicable repository checks.

- [ ] [I258] (P1) {I257} Separate protocol codecs, authentication, and execution lifecycles.
  Goal:
  Let provider transports combine reusable components through validated catalog data.
  The current adapter validator couples protocol selection to exact authentication headers and permitted lifecycle values.
  Requirements:
  - Use separate typed components for request and response codecs, authentication, and execution lifecycles.
  - Select those components through each catalog transport and construct one validated route at startup.
  - Reject unsupported combinations through explicit component contracts.
  - Keep provider identity as route data instead of a selector for shared execution code.
  - Keep tenant secrets and settings in their existing connection stores.
  - Keep bearer authentication, direct-header authentication, required static headers, and credential verification behavior.
  - Keep synchronous completion, resource polling, visibility rules, deadlines, cancellation, and usage accounting.
  - Share lifecycle code where the execution contract is equal. Retain explicit protocol requirements where behavior differs.
  - Keep Google credential expansion and media staging under F043.
  - Keep durable media workers and recovery under F022, and network fairness under I046.
  - Update the provider catalog reference and media architecture with the final component ownership.
  Deliverables:
  - Catalog-selected transport components and one startup composition path for current routes.
  Validation:
  - Run I256 tests before extraction and after each component change.
  - Prove that two catalog-defined providers share a codec with different supported authentication configurations.
  - Prove that supported synchronous and pollable routes share codec logic without changing their execution behavior.
  - Prove invalid combinations stop startup and rejected requests cause zero upstream dispatch.
  - Keep public errors, credential isolation, continuation, timeout budgets, and usage totals.
  - Run focused transport tests and applicable repository checks.

- [ ] [I259] (P2) {I258} Use one Responses codec with explicit protocol variations.
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


- [x] [I254] (P2) Use the Governor template for the managed issue format.
  Goal: Remove the format template drift reported during B204 validation.
  Requirements: Keep the cross-repository dependency specification in a separate document.
  Link that document from the tracker. Use the canonical template for the managed format guide.
  Validation: Run the Governor check, changed-prose review, identifier checks, and `git diff --check`.
  Resolution: Restored the managed format guide from the current Governor template.
  Moved the complete dependency specification to `DEPENDENCY-REFERENCES.md` and linked it from this tracker.
  The Governor check passed with no drift or warnings. Changed-prose checks and identifier checks passed.
  No application or event contract changed.

- [ ] [I244] (P1) {F024,F025,F026,F027,F039,F040,F041,F042} Remove the completed MediaOps operation-import bridge.
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
  - Run the final `make ci` target under the current repository validation policy.
  Media expansion:
  - Prove bounded interactive text progress while image or video traffic saturates the same upstream origin.
  - Define separate capacity ownership for accepted jobs, active HTTP requests, status reads, and artifact transfers.
  - Release active HTTP capacity between remote-job polls.
  - Include tenant fairness and shared provider-account limits in the chosen capacity contract.
  - Make this acceptance part of F024 before general media availability.
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
  Last run: 2026-09-08.
  Maintenance, and six Features. Kept all 59 unresolved entries visible.
  Kept all nine Planning entries visible. Confirmed eight recurring
  Maintenance entries remain open with the `R` suffix. Removed one satisfied
  I252 dependency marker. Found no duplicate active IDs, broken local
  dependency markers, invalid priorities, or blocked entries without a
  `Blocked:` line. Filed no follow-up issues.
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


## Features

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
- [ ] [F055] (P1) {F022,F054} Add Meta file transcription controls and speaker results.
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
- [ ] [F057] (P1) {F022,I046} Add Meta image generation and edit operations.
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
- [ ] [F039] (P1) {F024} Complete OpenAI image editing and progressive output.
  Goal:
  Preserve the current MediaOps OpenAI image contract before its complete provider cutover.
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
- [ ] [F040] (P1) {F022,F043} Add Vertex image operations through tenant provider connections.
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
- [ ] [F041] (P1) {F022} Add FAL image operations and queue recovery.
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
- [ ] [F042] (P1) {F022} Expose Dictator media capabilities through the tenant gateway.
  Goal:
  Make Dictator a private media provider behind LLM Proxy's public API.
  Current boundary:
  - MediaOps currently calls Dictator through one internal adapter.
  - After consolidation, LLM Proxy owns that adapter and the public tenant authorization boundary.
  Requirements:
  - Apply the provider catalog and protocol adapter contract in P011 before each capability release.
  - Inventory retained capabilities against the current Dictator gRPC contract and MediaOps capability matrix.
  - Cover transcription, diarization, subtitles, alignment, synthesis, voice extraction, and discovery where currently supported.
  - Define tenant-owned voices, history, assets, artifacts, and operation resources for retained capabilities.
  - Keep native voice, job, history, and artifact identifiers inside private provider records.
  - Map progress, cancellation, failures, and restart recovery through the common operation contract.
  - Authenticate the internal LLM Proxy-to-Dictator connection through deployment-owned configuration.
  - Keep speech engines and native workers in Dictator.
  - Use the existing qualified runtime when it satisfies the retained capability.
  - Treat P008 hardware or controller changes as dependencies only when measured requirements make them necessary.
  - Extend official Go and Python clients and current public caller interfaces in the selected capability release.
  - Inventory retained voices and jobs before migration. Require explicit tenant and provider-account ownership mapping.
  - Coordinate MediaOps I087 and every discovered direct caller in one bounded cutover.
  - Remove obsolete public runtime routing and direct caller credentials after the cutover acceptance.
  Deliverables:
  - Private Dictator adapter, tenant media resources, official clients, caller migration inventory, and activation plan.
  Open decisions:
  - Use `dictator` as the canonical provider identity. Specify the public capability schemas before implementation.
  - Select the current transcription route disposition without creating competing permanent contracts.
  - Inventory runtime access and retained data before deciding exact migration and deployment changes.
  Validation:
  - Complete the P011 second-provider acceptance procedure for each protocol adapter added or changed in this slice.
  - Start with real public API and persistence tests against a controlled Dictator protocol boundary.
  - Prove tenant isolation for voices, histories, operations, and artifacts.
  - Prove exact speech behavior, native identifier privacy, restart recovery, and truthful cancellation.
  - Prove Dictator unavailability does not prevent admitted cloud operations.
  - Run repository validation and record real-runtime and consumer acceptance separately.
- [ ] [F043] (P1) {F022} Add provider-readable media staging and Google credential profiles.
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
  Resolve P012 and approve a separate storage ownership contract before further Google credential implementation under F043.
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
- [ ] [F022] (P1) {I258} Add durable tenant-owned media operations to the existing gateway.
  Goal:
  Extend the existing tenant API with durable media execution and result artifacts.
  Use P011 and `docs/media-gateway-consolidation.md` as the current implementation contract.
  Requirements:
  - Extend the catalog and transport components completed by I257 and I258.
  - Extend I256 public protocol tests with durable operations, artifacts, cancellation, and restart recovery.
  - Apply the provider catalog and protocol adapter contract in P011 before each capability release.
  - Reuse managed tenant keys and bearer authentication for media resources.
  - Extend the existing asset contract and official Go client in the same API change.
  - Add capabilities, operations, cancellation resources, asset metadata reads, and bounded byte downloads under `/model/v1`.
  - Validate and accept each request in one operation creation call.
  - Keep the accepted intent, route, and catalog revision in the operation record.
  - Keep a separate public plan resource outside this release.
  - Return explicit unavailable cost evidence when exact catalog pricing is absent.
  - Require a tenant-bound idempotency key and enforce a unique database constraint.
  - Return the same operation for the same key and intent, including terminal and uncertain states.
  - Reject changed intent with `409` before dispatch.
  - Resolve an accepted request before current catalog validation.
  - Add operation, claim, asset-reference, and usage-delivery tables in the existing managed SQLite database.
  - Use bounded in-process workers for the first deployment.
  - Persist acceptance, input references, and dispatch intent before provider execution.
  - Use claim generations and transactional updates to reject stale workers.
  - Resume undispatched work after restart. Recover dispatched work through recorded provider evidence.
  - Keep accepted work independent from the initiating HTTP connection.
  - Keep provider execution evidence separate from public operation state and cancellation observations.
  - Use the state schemas, deadlines, retention, and initial capacity values specified by P011.
  - Keep native handles and byte digests private.
  - Coordinate asset publication and deletion with durable active references.
  - Return output assets through authenticated metadata and content reads.
  - Keep minimal idempotency tombstones for the tenant lifetime after terminal data expiry.
  - Keep unresolved operation evidence until reconciliation or explicit disposition.
  - Deduplicate execution usage by operation identifier through a durable delivery record.
  - Keep status reads and downloads separate from generation charges.
  - Coordinate network capacity with I046. Keep provider-specific staging in F043.
  - Add migration tools only when an actual retained-record inventory requires them.
  Deliverables:
  - A reusable second-provider acceptance harness with catalog-selected adapters and controlled external protocols.
  - Operation service, SQLite tables, worker, asset lifecycle, OpenAPI contract, and official Go client methods.
  - A real-service client example with controlled provider protocols.
  Validation:
  - Exercise the P011 second-provider procedure through the shared service harness.
  - Start with a failing integration test through the real HTTP listener, SQLite database, and filesystem.
  - Prove duplicate convergence, intent conflicts, tenant isolation, and zero dispatch for rejected requests.
  - Prove restart recovery, stale-worker rejection, explicit uncertainty, and truthful cancellation.
  - Prove active references, interrupted downloads, output expiry, tombstones, and deduplicated usage after restart.
  - Preserve current text behavior through integration tests before shared-code extraction.
  - Run the current repository checks. Record provider and hosted acceptance separately.
- [ ] [F024] (P1) {F022,I046} Deliver the first OpenAI image-generation capability.
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
  - MediaOps I084 consumes the complete OpenAI slice after F039 preserves its current image capabilities.
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
- [ ] [F025] (P1) {F022,F043} Add durable video generation to model operations.
  Goal:
  Make LLM Proxy the sole provider boundary for Vertex Veo, Vertex Gemini Omni,
  Runway, FAL, Kling, and xAI video generation.
  Cross-repository sequence:
  - MediaOps I010 consumes this released video slice independently of the
    image cutover.
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
  - Implement and release provider slices with explicit acceptance and removal receipts.
  - Keep each current capability available under its single owner until its verified cutover.
  - Add import validators only for records identified by the migration inventory.
  - Use the current repository validation policy instead of older baseline CI instructions.
- [ ] [F026] (P1) {F022} Add ElevenLabs speech, music, and alignment operations.
  Goal:
  Make LLM Proxy the sole external-provider boundary for the current
  ElevenLabs account while MediaOps retains narration planning and local audio
  assembly.
  Cross-repository sequence:
  - MediaOps I011 consumes this released audio slice independently of the
    image and video cutovers.
  Requirements:
  - Apply the provider catalog and protocol adapter contract in P011 before each capability release.
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
- [ ] [F027] (P1) {F022} Add provider account mutations, avatars, translation, and lip-sync.
  Goal:
  Complete gateway ownership of external media-provider credentials and
  provider-native task recovery for HeyGen and Kling account operations.
  Cross-repository sequence:
  - MediaOps I012 consumes this released account-operation slice independently
    of the other current-provider cutovers.
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
  Goal:
  Add the current Avatar V engine to the gateway HeyGen avatar contract before
  MediaOps exposes it through its product surfaces.
  Cross-repository sequence:
  - MediaOps F022 consumes this after its I012 base HeyGen/Kling cutover.
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
  Goal:
  Add the provider-qualified MiniMax H3 V2 route to the gateway before MediaOps
  exposes the model through its video product contracts.
  Cross-repository sequence:
  - MediaOps F023 consumes this after its I010 base video cutover.
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
  Goal:
  Add the current Speechify complete-response speech and voice-discovery
  contracts to the gateway before MediaOps exposes them in narration flows.
  Cross-repository sequence:
  - MediaOps F024 consumes this after its I011 base audio cutover.
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
*do not implement yet*

- [x] [P013] (P2) Plan provider and model logos for management and the public catalog.
  Goal: Define small logos that distinguish API connections from model families on both surfaces.
  Requirements:
  - Record exact provider and family mappings with asset sources.
  - Preserve the API connection and model family separation from I237.
  - Specify icon placement, dimensions, accessibility, and shared asset ownership.
  - Record uncertain brand identities and the implementation acceptance criteria.
  Deliverable: `docs/provider-model-icons.md`.
  Scope: This issue defines the plan. It does not authorize application implementation.
  Resolution (2026-09-07): Recorded 13 provider mappings and 29 family mappings with 18 candidate SVG assets.
  The proposal gives SenseVoice an explicit text-only presentation and records the current Moonshot identity decision.
  The visual comparison passed a theme-switch check and a 390px layout check.
  The plan passed the prose checker and Governor check. No application or event contract changed.
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
- [x] [P010] (P2) Plan current SiliconFlow model expansion.
  Goal:
  Define the route mappings and acceptance sequence for the six SiliconFlow gaps in the provider audit.
  Requirements:
  - Verify exact upstream IDs for GLM-5.3, DeepSeek V4 Pro and Flash, Kimi K3, Hy3, and LongCat 2.0.
  - Inspect provider-specific controls, conflicting limits, and existing model identities.
  - Define independent qualification without exposing an unaccepted offering through an enabled shared model.
  - Examine the existing repository credential inputs before declaring a paid acceptance blocker.
  Deliverable:
  `docs/siliconflow-expansion.md` records the proposed identities, provider evidence, source conflicts, and ordered acceptance sequence.
  Scope:
  This assessment records the expansion plan. It does not register or activate provider offerings.
  Resolution (2026-09-05):
  Verified all six upstream model strings and recorded the proposed public identities in the assessment.
  The DeepSeek releases use separate dated selectors. The earlier audit's undated-update interpretation was incorrect.
  Recorded conflicting Kimi K3 and LongCat output limits as unresolved provider evidence.
  Defined disposable-catalog acceptance before canonical registration to preserve shared model activation behavior.
  The SiliconFlow key is absent from the process and all six repository private environment files.
  Paid discovery and acceptance remain pending. No provider offering or event contract changed.
  Documentation checks and the Governor check passed.
  Approval (2026-09-06):
  The user approved the full P010 implementation scope.
  F059 owns all six offerings, provider-specific controls, Kimi image assessment, and qualification.
- [x] [P009] (P2) Assess new Meta image and transcription models.
  Goal:
  Define the integration scope for current Meta media models before implementation.
  Evidence (2026-09-05):
  - Meta lists hosted `muse-image-1.0` for image generation and image edits.
  - Its documented endpoints include `/v1/images/generations`, `/v1/images/edits`, and Responses.
  - Meta lists hosted `muse-voice-transcribe-1.0` for file and realtime transcription.
  - Its documented endpoints are `POST /v1/asr/transcribe` and `wss://api.meta.ai/v1/asr/realtime`.
  - Published prices are USD 0.01 per generated image and USD 0.18 per audio hour.
  - Muse Glimmer requires self-hosting and belongs to the local-inference scope in P008.
  Requirements:
  - Read the exact request, response, limit, retention, and error contracts for each hosted model.
  - Assess image generation and edits against the media operation design in F022.
  - Assess file transcription against the current dictation interface.
  - Treat realtime transcription as a separate public interface decision.
  - Split approved implementation outcomes into Features after this assessment.
  Sources:
  - https://dev.meta.ai/docs/models
  - https://dev.meta.ai/docs/pricing-rate-limits
  - https://research.meta.ai/blog/introducing-muse-voice-transcribe
  Scope:
  This issue records newly discovered provider capabilities.
  It does not authorize media adapter implementation.
  Resolution (2026-09-05):
  Recorded the assessment in `docs/meta-media-assessment.md`.
  Defined native file dictation requirements and image operation dependencies on F022.
  Recorded the separate realtime interface decision, request limits, retention gaps, and acceptance requirements.
  Implementation approval remains pending, so no Feature issues were created.
  The Meta key is absent from the process and all six repository private environment files.
  Paid provider acceptance remains pending. No runtime offering or event contract changed.
  Changed documentation passed the language review and repository checks.
  Approval (2026-09-06):
  The user approved the full P009 implementation scope.
  F054, F055, F056, F057, and F058 own file dictation, structured file results, realtime sessions, images, and image conversations.
  This approval supersedes the earlier implementation hold.
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
- [x] [P011] (P1) Specify the MediaOps gateway migration.
  Goal:
  Define one shared gateway product and a complete first consumer delivery.
  Deliverables:
  - Specify catalog ownership, protocol adapter selection, and acceptance through a second provider definition.
  - Record API, storage, worker, authentication, capacity, and resource ownership decisions.
  - Specify the provider sequence, paired consumer changes, data receipts, and release evidence.
  - Align F022 and related migration issues with one idempotent operation creation contract.
  - Preserve unrelated provider-audit work.
  Validation:
  - Verify source paths, issue references, API consistency, and changed documentation language.
  - Verify that staged changes exclude unrelated provider work.
  Resolution:
  - Added the provider catalog and protocol adapter contract on 2026-09-08.
  - Required a second provider to use the same executable and clients through catalog data and connection values.
  - Assigned the harness to F022 and the first image proof to F024.
  - Applied the acceptance requirement to each affected media slice.
  - Recorded the concrete contract in `docs/media-gateway-consolidation.md`.
  - Paired the consumer delivery with MediaOps P006.
  - Kept implementation issues open and identified the required FamilyHome P003 revision.
