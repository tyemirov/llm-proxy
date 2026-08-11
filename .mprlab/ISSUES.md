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
- [!] [B126] (P1) Activate the current v0.4.0 release on every public surface.
  Goal:
  Make the production API, container, and Pages site serve the immutable
  `v0.4.0` source revision.
  Evidence:
  - GitHub release `v0.4.0` points to source commit
    `0cc9537bbdc838d62961d88ce32195f7b4578fab`.
  - On 2026-08-10, the public Pages marker reported `v0.3.0` and source commit
    `7917ce1ed824b9946d2a98a5a55b90c443db884a`.
  - On the same date, the public API configuration route returned HTTP `200`
    and the proxy root returned its expected HTTP `403` authentication
    boundary. These checks prove availability but do not identify the running
    container revision.
  Requirements:
  - Use the repository lifecycle to publish and activate the exact `v0.4.0`
    container and Pages artifacts.
  - Preserve the retained management data volume and apply only migrations
    owned by the released binary.
  - Record the release tag, source commit, container manifest digest, running
    image ID, repository digest, and Pages marker as one activation receipt.
  - Verify the API and Pages surfaces independently after activation.
  Validation:
  - Prove the versioned image and `latest` resolve to the recorded manifest.
  - Prove the running image matches that manifest and source revision.
  - Prove the public Pages marker reports `v0.4.0` and the same source commit.
  - Prove the configuration route returns HTTP `200` and the proxy root returns
    HTTP `403` after activation.
  Blocked: the production operator must run the repository-owned deployment
  lifecycle and record the image and Pages activation receipt.
- [!] [B127] (P1) Activate I045 telemetry for B088 production acceptance.
  Goal:
  This activation issue makes the production proxy emit I045's safe request
  phase and provider-progress events. A public request id can then identify the
  exact long-completion boundary.
  Evidence:
  - The I045 work is completed. Its baseline and final 11-gate `make ci` runs
    passed with 100.0% Go statement coverage.
  - The currently active production revision does not contain the I045 source.
  - The 2026-08-10 B088 paid run produced correlated OpenAI and Meta request
    ids, but the running service cannot emit I045's phase totals or progress
    events for them.
  Requirements:
  - Complete the repository execution chain for the I045 source, release the
    resulting immutable revision, and activate that exact revision through the
    repository-owned production lifecycle.
  - Preserve the current production configuration and managed data volume.
    Record the release tag, source commit, container manifest digest, running
    image id, and repository digest in the activation receipt.
  Validation:
  - Send one redacted accepted request and prove its validated
    `X-LLM-Proxy-Request-ID` correlates with one terminal phase summary and the
    applicable content-free provider-progress events in production logs.
  - Prove the running revision matches the recorded immutable source and image
    before B088 spends another paid long-completion or canary request.
  Blocked: the execution chain must merge and release the completed I045 source,
  then the production operator must activate that immutable revision and record
  the receipt.
- [ ] [B088] (P1) {B127} Restore Default-tenant long completion routing for OpenAI and Meta.
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
    credential and model through their echo cases. Exact phase classification
    now depends on B127 because the running production revision predates I045.
  Requirements:
  - Diagnose and restore the exact OpenAI and Meta production routes through
    the saved Default-tenant provider configuration. Retain OpenAI's
    server-owned Responses polling and Meta's canonical blocking request
    contract.
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
  - For any source change, run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
- [ ] [B128] (P1) Restore Gemini long-completion production acceptance after provider rate limiting.
  Goal:
  Make the Default tenant's Gemini 3.5 Flash background case complete the
  production live-test contract. First, classify and resolve the provider
  rate-limit failure.
  Completion boundary:
  - Repository changes and repository validation prove development completion.
  - Production acceptance is an explicit completion condition for this issue.
  Evidence:
  - On 2026-08-10, `gemini-echo` returned HTTP `200` with its exact marker, so
    the saved Default-tenant credential and ordinary Gemini route were active.
  - The later `gemini-background-polling` case returned the sanitized provider
    HTTP `429` boundary with 162 response bytes and request id
    `H3VZZRB52HTFOBITJH22NNZ3WR`.
  Requirements:
  - Correlate the failed request with safe provider rate-limit metadata and
    determine the exact quota, billing, or configured proxy-window boundary.
  - Preserve Gemini 3.5 Flash's stored background Interactions lifecycle, the
    900-second request budget, response redaction, and the current deterministic
    long-completion case.
  - Apply only the correction supported by that evidence. Keep provider quota
    or billing work operational and proxy rate-window work in source/config.
  Validation:
  - After the identified boundary is resolved, run the exact Gemini echo and
    background cases with only the Default-tenant secret and prove HTTP `200`,
    the final markers, validated request ids, and no response-body disclosure.
  - For any source change, run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.
## Improvements

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

- [ ] [I210] (P1) Add Meta Muse Spark 1.2 as a selectable Standard-tier model.
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
- [ ] [I207] (P1) Add Gemini 3.6 Flash with route-bound Interactions thinking levels.
  Goal:
  Add Google's current stable Flash model to the Gemini Interactions catalog
  and carry the existing provider-neutral `reasoning_effort` contract onto its
  documented thinking controls.
  Evidence:
  - Google identifies Gemini 3.6 Flash as GA and production-ready under the
    exact stable model id `gemini-3.6-flash`, with a 65,536-token output limit
    and `medium` as its default thinking level:
    https://ai.google.dev/gemini-api/docs/latest-model
  - The model page confirms that `gemini-3.6-flash` supports text, image, video,
    audio, and PDF input, text output, and thinking:
    https://ai.google.dev/gemini-api/docs/models/gemini-3.6-flash
  - Google's Interactions thinking guide documents
    `generation_config.thinking_level`; Gemini 3.6 Flash supports exactly
    `minimal`, `low`, `medium`, and `high`, with `medium` as the default:
    https://ai.google.dev/gemini-api/docs/thinking
  - The Interactions schema defines the same four-value `thinking_level` enum:
    https://ai.google.dev/api/interactions-api
  - Google's Gemini 3.6 migration contract rejects a request whose final
    nonempty turn is a model turn with HTTP 400 and directs multi-turn
    Interactions callers away from manually prefilled model turns:
    https://ai.google.dev/gemini-api/docs/latest-model
  Requirements:
  - Add only the exact stable model id `gemini-3.6-flash`. Do not add or alias
    the requested but unpublished `gemini-2.6-flash` name, use a moving
    `gemini-flash-latest` alias, or change the Gemini provider default as an
    incidental part of this issue.
  - Before registering the route lifecycle, verify at the paid Google boundary
    that this exact model supports stored background Interactions through
    create, active status, retrieval, cancellation, and deletion. Register the
    model as `gemini_interactions` plus `pollable_resource` only after that
    proof succeeds.
  - At the resolved Gemini 3.6 route edge, reject with the canonical HTTP 400
    invalid-messages response any request whose final non-system message is an
    assistant turn. Do not send that turn as a terminal `model_output`, rewrite
    it into another role, or change assistant-prefill behavior for other model
    routes as an incidental part of this issue.
  - Add the exact `gemini_interactions` reasoning-effort adapter, valid only for
    the Gemini Interactions text route. Declare the model's ordered capability
    as `minimal`, `low`, `medium`, and `high`; serialize an explicitly resolved
    public effort unchanged as `generation_config.thinking_level` on the
    initial request and every output-limit continuation.
  - When neither the request nor tenant default selects an effort, omit
    `thinking_level` so Google's documented `medium` default remains
    authoritative. Reject blank, `none`, `xhigh`, `max`, and every other
    unsupported value before an upstream call. Do not translate values, send a
    2.5-era `thinking_budget`, or add a second Gemini request adapter.
  - Keep the current 65,536 proxy output limit. Declare only the image and audio
    inputs already implemented by the public messages contract; do not imply
    public video, PDF, tools, thought-summary, streaming, or computer-use
    support from the broader upstream model capability list.
  - Expose the exact route capability through the management profile and
    Settings autosave contract, and update configuration, constants, README,
    OpenAPI, provider-routing documentation, generated resources, and the model
    capability table together. No persisted-routing migration or default-model
    change is part of this issue.
  Validation:
  - Startup and public-boundary fixtures prove the Gemini-only adapter mapping,
    all four explicit effort payloads, omitted-effort omission, continuation
    preservation, early rejection of unsupported efforts, 65,536-token cap,
    media declarations, management profile exposure, saved tenant-default
    routing, and pre-upstream rejection of a terminal assistant turn only on
    the Gemini 3.6 route.
  - Authenticated branch acceptance runs one small request for each explicit
    thinking level and one omitted-level request, then proves one background
    create/poll/delete flow for `gemini-3.6-flash`. The final `make ci` passes
    after the last implementation edit; deployment and production acceptance
    remain operator-owned.
- [ ] [I027] (P1) Redesign the user dashboard around connected-provider widgets.
  Goal:
  Make the authenticated dashboard answer, at a glance, which upstream
  providers the selected Usage scope has connected. Preserve usage reporting as
  a separate measure of activity so an unused connected provider remains
  visible and historical traffic never implies that a provider is still
  connected.
  Current scope:
  - Usage is account-wide by default and has an independent tenant filter. One
    selected Usage tenant shows that tenant's connections. `All tenants` shows
    tenant-labelled connections across all owned tenants. The Settings tenant
    does not control the dashboard projection.
  Requirements:
  - Define a connected provider solely from canonical authenticated profile
    data whose `has_key` value is `true`. Do not infer connection from catalog
    membership, aliases, routing defaults, local environment credentials, or a
    provider's presence in historical usage.
  - Add a prominent `Connected providers` section to the user usage dashboard
    and render exactly one widget for each tenant/provider connection in the
    selected Usage scope. An explicit tenant uses that profile's deterministic
    provider order. `All tenants` groups connections by account tenant order and
    then provider order, labels every group with tenant name and opaque ID, and
    does not merge the same provider across two tenants. Do not hard-code
    provider names or duplicate provider-registration state in the browser.
  - Give each widget a concise, consistent summary: the profile label,
    `Connected` status, saved text model, declared text/dictation capabilities,
    and current-period request and token totals matched by exact canonical
    provider ID. A connected provider with no usage in the period must still
    render with zero activity. A usage-load failure must render as unavailable,
    not as a false zero or a disconnected provider.
  - Add a provider-specific `Manage` action that opens Settings with that exact
    tenant and provider selected without changing the Usage tenant filter. It
    must not reveal a key, invoke the key-reveal endpoint, or alter
    provider/default settings merely by opening the editor.
  - Replace the ambiguous usage-derived `Providers` summary metric with a
    `Connected providers` count derived from the same scope-correct `has_key`
    projection. Under `All tenants`, count tenant/provider connections rather
    than deduplicated provider IDs.
    Keep provider/model usage breakdowns explicitly labeled as activity for the
    selected reporting period, including historical rows for providers that are
    no longer connected.
  - Render a purposeful empty state when no providers are connected, with one
    action that opens Settings. The state must coexist with mandatory onboarding
    and must not create a path around its persisted-key requirements.
  - Keep the widgets synchronized with canonical profile state: a successful
    provider-key autosave adds its widget, a successful removal removes it,
    failed mutations leave the current projection unchanged, and dashboard
    refresh reloads both connection state and usage for the selected Usage
    scope. Never let an out-of-order response restore stale connection state.
  - Treat widgets as non-secret metadata. Never render provider API keys,
    masked-key suffixes, client keys, system prompts, or credential-bearing
    values in widget text, attributes, accessible names, or browser storage.
  - Use semantic headings and per-provider articles, unique accessible action
    names such as `Manage OpenAI`, full keyboard operation, and a responsive
    grid that remains aligned without horizontal overflow on narrow screens.
    Keep the provider widgets confined to the current user's owned tenants. The
    admin dashboard must not project another tenant's provider credentials or
    connection state.
  - Add one canonical owner-wide safe connection projection because the account
    summary does not contain provider `has_key` facts and the browser must not
    fan out profile requests under `All tenants`. Preserve the existing tenant
    profile as the canonical explicitly selected-tenant projection. Do not add
    cached shadow state, compatibility aliases, fallback matching, or expose
    masked/raw key material in the owner-wide response.
  - Update dashboard and self-service documentation so `connected provider` and
    `active provider` have explicit, non-overlapping meanings.
  Deliverables:
  - Add the connected-provider widget grid, connected count, provider-specific
    Settings navigation, empty/error states, and responsive styling to the user
    dashboard.
  - Add the owner-wide safe connection projection plus one derived presentation
    model that joins tenant/provider connections to usage by exact canonical IDs
    while keeping registration authoritative to `has_key`.
  - Update first-party frontend types, copy, documentation, and rendered-browser
    coverage for the final dashboard contract.
  Validation:
  - Add Playwright scenarios for `All tenants` and one explicit Usage tenant.
  - Cover zero, one, and multiple connected providers.
  - Cover duplicate provider IDs in two tenants and deterministic group order.
  - Cover a connected provider with zero activity.
  - Cover an unconnected provider with historical activity.
  - Cover exact model, capability, usage, and connected-provider count values.
  - Prove successful key autosave/removal and dashboard refresh update the
    widgets, while rejected or out-of-order requests do not mutate the visible
    projection and usage failure leaves connection state intact with activity
    marked unavailable.
  - Prove each `Manage` action selects the intended Settings tenant and provider
    without changing the Usage tenant or making a reveal/mutation request, no
    secret-bearing value reaches the rendered dashboard or browser storage, and
    admin/user dashboard switching preserves isolation.
  - Cover keyboard navigation, accessible names, and desktop/narrow viewport
    layout without overlap or horizontal overflow.
  - Run the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci`
    pair for the implementation, with the final run after the last code edit.
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
- [ ] [I032] (P2) {I027} Add donut breakdowns and meaningful axes to Usage Overview charts.
  Goal:
  Let a signed-in user choose one clear presentation for both the selected
  Usage scope's Provider usage and Model usage activity breakdowns, and make
  the Requests and Tokens time-series charts explain their scales without
  guesswork. Preserve the Usage tenant, interval, exact request and token
  counts, and the distinction between historical activity and currently
  connected providers.
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
    reserves provider/model breakdowns for historical selected-period
    activity, rather than current `has_key` connection state.
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
  - Keep the scope to the authenticated user's Usage Overview. I027's connected
    provider widgets remain a separate `has_key` projection; an inactive
    connected provider and historical activity for a disconnected provider must
    retain their existing meanings. Do not add this control to the aggregate
    admin dashboard or expose credentials, keys, prompts, responses, or other
    sensitive usage data.
  - Document the resulting presentation contract in README, CHANGELOG.md, and
    `docs/implementation/provider-routing-plan.md`. Update the source in
    `scripts/generate_seo_resources.mjs` and regenerate the managed-tenant
    usage resource; do not hand-maintain a divergent generated page. State
    explicitly that this is a client-side view of existing aggregate request
    data, define both line charts' UTC time and per-bucket quantity axes, and
    state that the presentation is not a billing, provider-performance,
    connected-provider, token-share, exact-event-time, or new management-API
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
  Last run: 2026-08-10 open-issue audit. GitHub release `v0.4.0` points to
  source commit `0cc9537bbdc838d62961d88ce32195f7b4578fab`. The live API
  returned the expected proxy (`403`) and configuration (`200`) boundaries.
  The public Pages marker still reports `v0.3.0` and source commit
  `7917ce1ed824b9946d2a98a5a55b90c443db884a`. B126 owns that exact activation
  drift and the operator-held running-image receipt.
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
  Last run: 2026-08-10. Reviewed 84 resolved or retired non-recurring issues
  against current source and durable documentation. Archived 56 BugFixes, 22
  Improvements, four Features, and two Planning issues. Kept all 44 current
  open, blocked, Planning, and recurring entries active. Removed 28 satisfied
  dependency tokens from 17 active entries during the archive pass.
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
- [ ] [M013] (P2) Resolve missing product-context document references.
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
- [ ] [M012] (P2) {M013} Reconcile repository governance with the MPR Lab normalizer.
  Goal:
  Make the governance normalizer check pass without deleting repository-owned binding contracts.
  Requirements:
  - Resolve M013's product-context document decision first so the normalizer
    works from the final repository-owned root guidance.
  - Inspect the normalizer differences reported for root `AGENTS.md` and every managed `.mprlab/` guide.
  - Preserve the M011 pre-change and post-change CI requirement and all other current repository-owned rules.
  - Update the appropriate managed templates, boundaries, or repository
    documents as one canonical forward-only contract.
  - Do not apply a destructive bulk rewrite.
  Deliverables:
  - A reviewed governance normalization change with no unrelated product or runtime edits.
  Validation:
  - Run the MPR Lab governor in `--dry-run` and `--check` modes and require no pending managed-file changes.
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

- [x] [F033] (P0) {F022} Pass canonical message media to selected providers.
  Goal:
  Let `/v2` accept provider-neutral media without a smaller LLM Proxy media
  limit. Translate each attachment into the selected provider's supported
  transport. Preserve exact media bytes, order, MIME type, and SHA-256.

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
  - Require `type`, `asset_id`, `mime_type`, and `sha256` in each asset
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
  - Validate tenant ownership, asset state, expiry, MIME type, size, and
    SHA-256 before provider dispatch.
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
  - Require fake Gemini to receive the exact bytes, MIME type, SHA-256, and
    order.
  - Do a test of each provider boundary at the limit and one unit above it.
  - Do a test of Gemini inline and Files API transport selection.
  - Do a test of each documented provider limit record.
  - Prove no provider-valid request fails because of a smaller proxy limit.
  - Cover missing, foreign, expired, deleted, wrong-MIME, and wrong-digest
    assets.
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
  - Passed the final 11-gate CI run with 100.0% Go statement coverage.

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
  - Implement after I221 so `baidu` becomes a distinct provider offering for
    canonical ERNIE and DeepSeek model identities instead of adding duplicate
    provider-nested model records. Use canonical provider id `baidu`, display
    label `Baidu Qianfan`, and no aliases.
  - Register exact Qianfan text offerings `ernie-5.0`, `deepseek-v4-pro`,
    `deepseek-v4-flash`, and `deepseek-v3.2`. Use `ernie-5.0` as the Baidu
    provider default and record each documented output-token limit on its
    provider offering.
  - Use `https://api.baiduqianfan.ai/v1`, bearer authentication, the existing
    `openai_chat_completions` wire contract, `synchronous_completion`, and the
    upstream `max_tokens` field. Omit the optional `appid`. Make one pasted API
    key sufficient for the complete supported flow.
  - Treat the API key as an opaque nonblank credential. Add
    `providers.baidu.api_key` and `providers.baidu.base_url` to the service
    configuration contract. Keep provider keys absent from the checked-in
    managed configuration. Use `BAIDU_API_KEY` only for explicit static or
    paid-live inputs.
  - Reuse the shared Chat Completions request codec and add an explicit Qianfan
    response policy. `finish_reason=stop` completes, `length` enters the common
    missing-suffix coordinator, and other reasons fail safely. When `flag` is
    present, accept only documented continue values `0` and `1`. Reject `2`,
    `3`, `4`, and unknown values without exposing partial text. Apply the same
    structural and safety checks to provider-key verification.
  - Expose Baidu through the existing authenticated
    `provider-keys/baidu` operation and generated provider editor. A paste must
    automatically verify the exact selected Qianfan model before the encrypted
    key, provider settings, and eligible routing default are atomically saved.
    Keep the raw key, authorization header, and Qianfan body out of responses
    and logs.
  - Project the new provider offerings through the canonical management
    profile, routing selectors, public capability REST resource, frontend-owned
    routing graph and model catalog, usage dimensions, and examples. Add Baidu
    selection to the official Go, Python, and CLI clients. Generate each view
    from the canonical provider and model inventory.
  - Keep the initial integration text-only and non-streaming. Qianfan visual
    input, deep-thinking controls, configurable reasoning effort, tools,
    structured output, search results, and streaming require separate explicit
    capability issues. Apply the same rule to AppBuilder, ModelBuilder, custom
    deployments, and dictation.
  Deliverables:
  - Add the Baidu provider/configuration boundary, normalized provider
    offerings, Qianfan response policy, management-key verification path, and
    paid-live harness registration with `LLM_PROXY_LIVE_BAIDU_MODEL` for an
    optional exact model override.
  - Update the canonical OpenAPI artifact, checked-in configuration, README,
    provider-routing guide, generated public artifacts, environment examples,
    and affected black-box fixtures together.
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
  - Record MIME type, byte size, SHA-256, ownership, creation time, retention
    expiry, and provider-readable staging state for every asset.
  - Add a strict object-store configuration with a filesystem fixture backend
    and a GCS production backend for provider staging.
  - Stream large bytes through the asset store and use asset identifiers in
    operation payloads. Keep local filesystem paths at the caller boundary.
  - Materialize provider outputs into gateway-owned artifacts before reporting
    operation success unless the catalog declares a durable provider artifact
    that the gateway can retrieve on demand.
  - Enforce tenant isolation, bounded uploads/downloads, MIME and digest
    verification, expiry, and deterministic cleanup.
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
  - Prove truncated uploads, digest mismatch, oversized media, cross-tenant
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
- [!] [F021] (P1) Add OAuth-authenticated tenant-scoped MCP access.
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
  - The gateway cannot yet generate the root OAuth block or tenant OAuth policy.
  Requirements:
  - Serve MCP protocol version `2026-07-28` at the exact resource URL
    `https://llm-proxy-api.mprlab.com/mcp/{tenant_id}`.
  - Use the official Go MCP SDK in stateless Streamable HTTP mode. Return JSON
    responses and reject every unsupported protocol version.
  - Do not create an MCP session or implement an earlier MCP transport.
  - Treat the exact MCP URL as the OAuth protected-resource identifier and
    access-token audience.
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
  Blocked: mprlab-gateway F001 must complete the TAuth OAuth aggregate
  deployment contract before this feature can declare its protected resources.

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
