# ISSUES

Entries record newly discovered requests or changes.

Read @AGENTS.md (Workflow section), @POLICY.md, and relevant stack guides before implementing changes.

Format: `- [ ] [B042] (P1) {I007} Title`

- `[ ]` open, `[-]` taken, `[!]` blocked, `[x]` closed.
- Blocked issues (`[!]`) must include a `Blocked:` line in the body.

Resolved history: `.mprlab/ISSUES-ARCHIVE.md`; the complete original issue
bodies, resolution notes, and validation records remain in `v0.2.43`.

Triage, 2026-07-25: B069, F014, I029, and I031 are resolved. The selected
one-issue-at-a-time P1 execution tranche is complete. I027 and P001 are
independent B076 successors. I032 follows B076
and I027 so its activity-breakdown presentation is added to the final
Usage-scope dashboard rather than an obsolete global active-tenant layout. I033
follows B076 and I029 so its bounded dashboard freshness contract uses the
canonical account-wide and tenant-filtered operations and response headers. M019 is independently
ready because M018 is complete. M013 then M012 resolve the product-context
governance path. Planning proceeds P002 -> P003 -> P004 -> P005, with M020
already satisfied; recurring maintenance remains scheduled work.
I036 is an independently ready P1 improvement that verifies every newly
supplied provider credential before it can be persisted or become routing
eligible.
I035 is an independent B076 successor that persists only the authenticated
user's selected Usage interval across sessions.
F016 is independently ready and adds the canonical v2 client contract for
server-side Node.js applications.
B077 publication and both public serving boundaries are now verified at
v0.2.47. It remains operator-blocked only on the direct production-container
image receipt and the post-release Terra/max canary required by its completion
contract. The dominant active Meta 502 path is separately reproduced as
explicit caller completion-budget exhaustion, not missing B077 activation.

## BugFixes

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
  - Require only the canonical `LLM_PROXY_SECRET` tenant client secret. The
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
    returned nonzero. The echo failures remain B087; the long-completion
    failures are tracked in B088.

- [ ] [B087] (P1) Restore Default-tenant Gemini and Moonshot production routing.
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

- [ ] [B088] (P1) Restore Default-tenant long completion routing for OpenAI, Anthropic, and Meta.
  Goal:
  Make the Default tenant complete the production live test's deterministic
  large request through OpenAI, Anthropic, and Meta without a local provider
  credential, fallback provider, or client-side polling path.

  Evidence:
  - The expanded `make live-test` run returned HTTP `200` for the three
    providers' short echo requests using their saved Default-tenant models.
  - The same run gave OpenAI's named background-polling case its full
    900-second budget before a safe HTTP `504`; Anthropic and Meta long
    completion cases each returned safe HTTP `502`.
  - The harness sent the same request larger than 16 KiB to all three cases,
    required normalized output for all 120 fictional portfolio records before
    the final marker, printed no response body or credential, and continued
    through the complete eight-case matrix.

  Requirements:
  - Diagnose and restore the exact production route for each failed long
    completion through the saved Default-tenant provider configuration. Retain
    OpenAI's server-owned Responses polling and the canonical blocking request
    contract for Anthropic and Meta.
  - Do not weaken, skip, shorten, special-case, retry, or replace the
    large-completion live-test cases to conceal a provider, continuation, or
    request-deadline failure.
  - Do not add local provider keys, a client polling endpoint, a fallback
    provider, or an unbounded timeout. Keep request and response data redacted
    from user-facing failures and issue evidence.

  Validation:
  - Run `make live-test` with only the Default-tenant client secret and prove
    the named OpenAI background-polling, Anthropic long-completion, and Meta
    long-completion cases return HTTP `200` with their final marker.
  - For any source change, run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair.

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

- [x] [B080] (P1) Reject incomplete OpenAI responses that contain partial text
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

- [!] [B077] (P1) {B069,F014,I029,I031} Publish and activate the merged LLM Proxy contract.
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

## Improvements

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

- [ ] [I037] (P1) Model provider wire contracts separately from execution lifecycles.
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

- [ ] [I038] (P2) {I037} Adopt DashScope's synchronous Responses API without background mode.
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
    Migrate supported models to a dedicated DashScope Responses wire adapter;
    leave any unsupported model on one explicitly registered current contract
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

- [!] [I039] (P1) {I037} Replace or retire the backend-ineligible Qwen Cloud Token Plan provider.
  Goal:
  Stop treating an interactive-tool subscription as an application-backend
  provider before considering any Responses API refactor.

  Evidence:
  - The canonical `qwencloud` provider points at
    `https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1`.
  - Alibaba's current base-URL and integration documentation says Token Plan is
    for interactive AI coding tools and is not for custom applications,
    automated scripts, or application backends:
    https://www.alibabacloud.com/help/en/model-studio/base-url
    https://www.alibabacloud.com/help/en/model-studio/more-tools
  - Token Plan documentation mentions Responses-powered harness tools, but that
    does not authorize LLM Proxy production traffic or establish an interactive
    background-resource contract.

  Blocked:
  An operator/product decision and eligible Alibaba service credential are
  required: either replace `qwencloud` with a distinct backend-authorized
  service contract, or remove the redundant provider and use the canonical
  DashScope provider.

  Requirements:
  - Send no LLM Proxy backend traffic to the Token Plan endpoint after this
    issue is resolved. Do not probe newer API shapes with production tenant
    requests while eligibility is unresolved.
  - If a distinct backend-authorized service is selected, document its exact
    credential, base URL, models, wire API, lifecycle, and billing identity,
    then migrate persisted provider settings once into that canonical contract.
  - If no distinct service exists, reject new `qwencloud` settings, migrate or
    explicitly invalidate existing persisted selections, and delete the
    provider, environment placeholders, docs, and tests. Do not alias it to
    DashScope or silently fall back to DashScope credentials.

  Validation:
  - Runtime configuration and public routing tests prove that no Token Plan
    domain can receive backend inference requests and that obsolete persisted
    provider selections are handled by the chosen bounded migration.

- [ ] [I040] (P1) {I037,B087} Migrate Gemini from generateContent to Interactions resources.
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

- [ ] [I041] (P2) {I037} Migrate Grok to xAI Responses without OpenAI background assumptions.
  Goal:
  Move Grok off xAI's deprecated Chat Completions surface while preserving
  xAI's actual synchronous Responses behavior.

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
    `background` polling; existing xAI speech routing remains independent.

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
    (`200`/`200`); the production default-route repair remains tracked by B087.
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

- [ ] [I035] (P2) {B076} Persist each user's selected Usage interval across sessions.
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

- [ ] [I033] (P2) {B076,I029} Keep the visible Usage Overview automatically fresh.
  Goal:
  Let a user returning to an unattended account-wide or tenant-filtered Usage
  Overview see current activity without having to discover and press Refresh.
  Provide a bounded, observable near-real-time freshness contract rather than
  claiming a push-based real-time feed.

  Evidence:
  - The current dashboard loads usage at authenticated-workspace startup and on
    interval selection or explicit Refresh only. It has no usage timer or page
    visibility lifecycle, so an open page can display yesterday's snapshot
    indefinitely.
  - The current refresh path clears the rendered summary after a request error.
    That is safe for a newly selected interval, but an automatic refresh would
    turn a true prior snapshot into misleading zeroes after a transient failure.
  - The usage client still uses the browser's default cache behavior. B076
    established `Cache-Control: no-store` on both the account-wide and
    tenant-filtered summary operations, but foreground revalidation also needs
    an explicit browser request-cache contract.
  - B076 separates the `Usage tenant` scope from the `Settings tenant`; I029
    makes both usage scopes' headers and response behavior one canonical HTTP
    contract.

  Requirements:
  - Implement only after B076 and I029, against the canonical account-wide and
    tenant-filtered usage operations. Do not add a second polling endpoint,
    introduce WebSocket/SSE/service-worker push infrastructure, or add a
    browser-stored freshness preference. This issue is foreground revalidation
    of the existing aggregate snapshot, not a streaming product.
  - Define one centralized `USAGE_FRESHNESS_MILLISECONDS` budget of 60 seconds.
    It is a user-facing maximum ordinary age while the usage view is visible,
    not an arbitrary retry or transport timeout. The authenticated selected
    Usage scope revalidates no more often than that budget, and a return from a
    hidden page revalidates immediately when the accepted snapshot is older
    than the same budget or absent. Hidden tabs, the admin dashboard, and
    signed-out/error workspaces perform no periodic usage request.
  - Maintain exactly one scheduled usage revalidation and at most one in-flight
    usage request for the selected Usage scope and interval. Schedule the next foreground
    revalidation only after the current request settles; do not use overlapping
    interval callbacks or a hot retry loop. Cancel/invalidate scheduled work on
    logout, authentication reset, Usage tenant or interval change,
    dashboard-view change, and page teardown. Resume only after the final Usage context is
    established.
  - Reuse B076's Usage scope, interval, and request-identity guards. An
    automatic or visibility-triggered response can update only the still-selected
    Usage tenant scope and interval; it must not overwrite a newer manual
    refresh, Usage tenant selection, interval selection, authentication reset,
    Settings tenant change, or local I032 breakdown presentation choice. A
    manual Refresh may request immediate revalidation but must join the same
    single-request lifecycle and reschedule freshness.
  - Track and visibly expose the receipt time of the last accepted usage
    snapshot using centralized copy and semantic time markup. Do not announce a
    success toast every minute. Distinguish a current snapshot, an in-progress
    refresh, and a stale snapshot accessibly, without presenting browser-clock
    metadata as server event time.
  - Preserve a successfully rendered snapshot when a same-scope/same-interval
    manual, automatic, or return-to-visible refresh fails. Mark it stale and
    provide a clear retry path; do not replace its counts, charts, breakdowns,
    or I032 view with empty/zero data. Keep the current clear-before-load rule
    for a changed Usage tenant or interval so one scope's data can never appear
    as another scope's. An initial load with no prior accepted snapshot retains
    the explicit empty/error state rather than fabricating a last-updated time.
  - Preserve `Cache-Control: no-store` on every canonical account-wide and
    tenant-filtered usage response and make the browser usage fetch explicitly
    uncacheable with `cache: "no-store"` so revalidation cannot be satisfied by
    a stale private cache. Do not change the JSON payload merely to transport
    client receipt time.
  - Keep the refresh scope to aggregate usage metadata already authorized for
    the selected Usage scope. The `All tenants` scope may include only the
    authenticated owner's tenants; an explicit tenant may include only that
    tenant. Do not poll or reveal generated secrets, provider keys, prompts,
    responses, transcripts, audio names, another owner's tenants, or aggregate
    administrator facts. Continue to make `connected provider` state I027-owned
    rather than inferring it from refreshed historical activity.
  - Update README, CHANGELOG.md, `docs/implementation/provider-routing-plan.md`,
    and the source in `scripts/generate_seo_resources.mjs`, then regenerate the
    managed-tenant usage resource. Document the exact foreground/hidden behavior,
    60-second freshness meaning, manual Refresh role, last-updated/stale signal,
    and the fact that this is not a push, billing, provider-performance, or
    exact-event-time guarantee. This repository has no PRD.md or
    ARCHITECTURE.md; do not create partial placeholders for this behavior.

  Deliverables:
  - One typed usage-refresh reason/lifecycle contract, central freshness budget,
    visibility-aware single scheduler, cache-safe usage client request, and
    race-safe Usage-scope state integration.
  - A compact accessible last-updated/loading/stale status and retry behavior
    that preserves a valid current-context snapshot across refresh failures.
  - Canonical no-store response-header documentation/conformance plus updated
    repository and generated public documentation; no new streaming endpoint,
    persistence schema, or client-library API.

  Validation:
  - Preserve real management-router and OpenAPI coverage proving both canonical
    Usage scopes carry `Cache-Control: no-store` without changing their
    aggregate JSON shape.
  - Add Playwright scenarios with controlled time and page visibility for the
    initial load, one-minute foreground revalidation, no hidden/admin polling,
    stale-on-return immediate revalidation, one in-flight request, manual
    Refresh coordination, timer cleanup on logout/Usage-tenant/interval/view
    changes, and stale-response rejection across scope and interval races.
  - Prove a failed refresh after a successful snapshot preserves its exact data
    and marks it stale, while a successful later refresh updates counts and the
    receipt timestamp; prove a new Usage scope/interval never retains prior data.
    Cover keyboard/screen-reader status, narrow layouts, no browser storage,
    no success-notice spam, and absence of sensitive values from DOM/network
    payloads beyond the existing usage contract.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair for the implementation, with
    the final run after the last code edit.

- [ ] [I032] (P2) {B076,I027} Switch provider/model activity breakdowns between bar graphs and segmented disks.
  Goal:
  Let a signed-in user choose one clear presentation for both the selected
  Usage scope's Provider usage and Model usage activity breakdowns, while
  preserving the Usage tenant, interval, exact request counts, and the
  distinction between historical activity and currently connected providers.

  Evidence:
  - The current usage summary already returns deterministically ordered
    provider and model aggregates with request counts. The existing rows are
    ranked horizontal bars scaled to the largest category, not shares of the
    breakdown total.
  - The summary has time buckets only for total requests and tokens. It has no
    provider- or model-specific time series, so `Graph` must mean the ranked
    horizontal-bar display rather than a new trend chart.
  - B076 establishes canonical account-wide and explicitly tenant-filtered
    Usage scopes. I027 then establishes the final dashboard layout and explicitly
    reserves provider/model breakdowns for historical selected-period activity,
    rather than current `has_key` connection state.

  Requirements:
  - Implement only after B076 and I027, against the canonical response for the
    selected Usage scope. Do not add a presentation-specific endpoint, response
    field, server persistence, URL parameter, tenant setting, browser storage,
    or client-library change.
    If final implementation exposes a genuinely missing data field, file and
    order a separate contract issue rather than broadening this UI issue.
  - Add one shared, visible, keyboard-operable `Breakdown view` control for
    both activity panels. It has exactly `Graph` and `Segmented disk` choices;
    `Graph` is the default and is the existing ranked horizontal-bar graph.
    Switching a mode changes both panels together so their distributions remain
    directly comparable.
  - Keep the choice local to the mounted authenticated dashboard. It survives
    interval selection, Refresh, and Usage tenant selection, but resets on
    authentication reset and a full page reload. A mode change is a
    pure presentation action: it must not fetch, mutate the selected interval
    or Usage tenant, or weaken B076's request-identity/stale-response rules.
  - Build every disk from the same ordered `providers[].data.requests` or
    `models[].data.requests` data that Graph renders. The percentage denominator
    is the complete source breakdown total, never token counts or the largest
    row. Preserve every source category exactly once: Graph always lists each
    category; the disk may combine the ordered tail into a visibly labelled
    `Other` segment only when a named, documented, geometry-derived disk-capacity
    rule would otherwise make the compact panel unreadable. `Other` must expose
    its exact aggregate count and deterministic share; it cannot discard or
    relabel source data.
  - Render the alternative as a dependency-free SVG segmented disk in the
    existing compact dark dashboard style. Give each segment a deterministic
    palette assignment from the canonical summary order, but never use color or
    hover alone to communicate meaning. Show a visible semantic legend/list
    with category name, request count, and deterministic percentage; rounded
    legend shares must total 100 percent. Handle zero activity with the existing
    empty state and one-category activity as one 100-percent segment without
    invalid SVG geometry.
  - Use centralized frontend copy and typed presentation data for the control,
    mode names, legend, `Other`, and accessible SVG label. Preserve visible
    focus and full keyboard operation (`aria-pressed` or an equivalent
    single-choice control), and keep labels/counts/shares available to assistive
    technology without a tooltip. Validate desktop and narrow layouts without
    clipping, overlap, or horizontal overflow.
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
    data, not a billing, provider-performance, connected-provider, token-share,
    or new management-API feature. This repository has no PRD.md or
    ARCHITECTURE.md; do not create partial placeholders for this UI change.

  Deliverables:
  - One typed local presentation-mode contract, pure provider/model distribution
    transform, shared selector, semantic bar/disk renderings, responsive styles,
    and centralized copy in Usage Overview.
  - A legible, deterministic SVG disk/legend treatment that preserves all
    request counts and makes any `Other` aggregation explicit.
  - Updated README, CHANGELOG.md, implementation documentation, generator-owned
    public usage resource, generated artifact, and browser coverage; no
    management API, Go client, Python client, or CLI wire-contract change.

  Validation:
  - Add Playwright coverage through the real management dashboard showing the
    default Graph mode, keyboard selection of Segmented disk, simultaneous
    changes to provider and model panels, visible names/counts/shares, and no
    additional usage request when the presentation changes.
  - Exercise interval changes, Refresh, Usage tenant selection, loading/failure, and
    out-of-order response scenarios; prove the local mode remains selected only
    where specified and never presents a stale Usage scope or interval snapshot.
  - Cover zero, one, and many-category distributions, including deterministic
    `Other` aggregation, exact request-count conservation, share totals of 100
    percent, Graph access to every source category, non-color-only semantics,
    administrator isolation, and desktop/narrow viewport geometry.
  - Run the required baseline and final
    `timeout -k 350s -s SIGKILL 350s make ci` pair for the implementation, with
    the final run after the last code edit.

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

- [ ] [I027] (P1) {B076} Redesign the user dashboard around connected-provider widgets.
  Goal:
  Make the authenticated dashboard answer, at a glance, which upstream
  providers the selected Usage scope has connected. Preserve usage reporting as
  a separate measure of activity so an unused connected provider remains
  visible and historical traffic never implies that a provider is still
  connected.

  Dependencies:
  - B076 makes Usage account-wide by default and gives it an independent tenant
    filter. Build the widgets against that final scope contract: one explicitly
    selected Usage tenant shows that tenant's connections, while `All tenants`
    shows tenant-labelled connections across all owned tenants. The Settings
    tenant must not silently control the dashboard projection.

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
    render with zero activity; a usage-load failure must render as unavailable,
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
    Keep the provider widgets confined to the current user's owned tenants; the
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
  - Add Playwright scenarios for `All tenants` and one explicit Usage tenant;
    zero, one, and multiple connected providers; duplicate provider IDs in two
    tenants; deterministic grouping/order; a connected provider with zero
    activity; an unconnected provider with historical activity; exact
    model/capability and usage rendering; and the connected-provider count.
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
  Last run: 2026-07-24. Polished 13 non-recurring unresolved entries after the
  resolved-history archive. Added I029 dependencies on B069 and F014: the
  former settles the public per-request timeout/error contract and the latter
  replaces singular management routes. B069 now records its external gateway
  deployment dependency, and M013 waits for B069 so future product-context
  documents cannot omit the resulting timeout contract. Selected the
  sequential P1 tranche B069 -> F014 -> I029; I031 is the resulting convergence
  item, while I027 and P001 now follow B076's independent Settings and Usage
  selectors. Planning
  entries remain open but deferred under the repository workflow.
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
- [ ] [M013] (P2) {B069} Resolve missing product-context document references.
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
  - Match F015's application-user model-profile contract. A configured
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

## Planning
*do not implement yet*

- [ ] [P001] (P1) {B076} Design a tenant-scoped provider, model, and key-acquisition onboarding flow.
  Goal:
  Let a signed-in managed user complete one clear text-routing setup: select a
  supported provider, select one of that provider's supported text models, and
  either paste an existing provider API key or open that provider's official
  key-acquisition page in a new window before returning to paste it. A completed
  setup must make the chosen provider/model the Settings tenant's usable text
  route without asking the user to reconcile separate provider, default, and
  client-secret forms.

  Requirements:
  - Build the flow inside Settings on B076's editor-only `Settings tenant`
    context. It must read and write only that selected tenant and must not change
    the independent `Usage tenant` filter; another tenant or user must never
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
