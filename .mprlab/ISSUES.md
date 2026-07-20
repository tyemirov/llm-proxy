# ISSUES

Entries record newly discovered requests or changes.

Read @AGENTS.md (Workflow section), @POLICY.md, and relevant stack guides before implementing changes.

Format: `- [ ] [B042] (P1) {I007} Title`

- `[ ]` open, `[-]` taken, `[!]` blocked, `[x]` closed.
- Blocked issues (`[!]`) must include a `Blocked:` line in the body.

## BugFixes

- [x] [B001] (P2) Make management request examples copyable and provider-specific.
  ### Summary
  The Settings modal renders request examples as one inert snippet. Users need default examples and selected-provider examples as separate copyable commands so they can copy the exact public proxy request shape they intend to use.
  ### Acceptance Criteria
  1. Default text, v2, and dictation examples remain visible before a generated secret exists and use the documented `<generated-secret>` placeholder.
  2. Each default example has its own copy action.
  3. The Settings modal shows provider-specific text and v2 examples for the currently selected provider.
  4. Provider-specific examples update when the selected provider changes.
  5. Freshly generated secrets replace the placeholder across default and provider-specific examples.
  6. Browser coverage proves the examples render and can be copied.
  ### Resolution
  The Settings modal now renders separate default text, default v2, default dictation, selected-provider text, and selected-provider v2 examples, plus selected-provider dictation when the provider supports `/dictate`. Each example has its own copy action, uses the configured proxy origin, uses `<generated-secret>` before secret creation, and updates to the freshly generated secret immediately after key generation. Provider-specific examples track the selected provider and selected provider text model. README and provider-routing docs describe the copyable default/provider examples contract. Validation passed with `timeout -k 180s -s SIGKILL 180s make frontend-lint`, `timeout -k 180s -s SIGKILL 180s npm run frontend:test -- management-ui.spec.js`, `timeout -k 180s -s SIGKILL 180s make frontend-test`, and `timeout -k 30s -s SIGKILL 30s git diff --check`.
- [x] [B002] (P2) Present provider settings through a selected-provider editor.
  ### Summary
  The Settings modal should not show one full provider-settings card for every supported provider. Routing defaults and request examples should stay as their own sections, and provider key/model/system prompt settings should be edited through one selected-provider form.
  ### Acceptance Criteria
  1. Routing defaults and request examples remain separate sections in Settings.
  2. The provider settings section exposes a provider selector.
  3. The selected provider editor lets users update API key, provider text model, and provider system prompt together.
  4. The selected provider editor shows masked key status and supports removing the selected provider key.
  5. Browser coverage proves provider selection updates the visible editor fields.
  ### Resolution
  The Settings modal now keeps routing defaults and request examples as separate sections, then shows one Provider settings editor with a provider selector. Selecting a provider updates the visible masked-key status, API-key input label, provider text model choices, and provider system prompt field. The save/remove actions operate on the selected provider while keeping the existing provider-settings API contract. README and provider-routing docs now describe the selected-provider editor. Validation passed with `timeout -k 180s -s SIGKILL 180s make frontend-lint`, `timeout -k 180s -s SIGKILL 180s make frontend-test`, and `git diff --check`.
- [x] [B003] (P2) Store text model and system prompt with each managed provider.
  ### Summary
  Provider key editing only collected API keys, while text model and system prompt lived in one global routing-default form. Managed tenants should own text model and system prompt settings per saved provider so provider-selected requests have complete provider-specific routing context.
  ### Acceptance Criteria
  1. Each managed provider profile includes a selected text model and provider-specific system prompt.
  2. Saving a provider key requires and validates the selected text model for that provider.
  3. Managed text requests that select a provider and omit `model` use that provider's saved text model.
  4. Managed text requests that select a provider and omit request-level system instructions use that provider's saved system prompt.
  5. The Settings provider editor lets users edit API key, text model, and provider system prompt together.
  6. Tests cover the API/store routing contract and the browser-visible provider controls.
  ### Resolution
  Managed provider-key records now include the selected text model and provider-specific system prompt. The management API validates provider text models on save, preserves an existing encrypted provider key when only model/prompt settings are changed, and returns provider settings in each profile provider. Managed text requests that select a provider and omit request-level model or system instructions now use that provider's saved text model and system prompt. Existing provider-key rows without text settings are normalized at startup to the current provider catalog default model. The Settings provider editor now exposes API key, text model, and system prompt controls together, and README/provider-routing docs describe the current contract. Validation passed with `timeout -k 180s -s SIGKILL 180s go test -count=1 ./internal/proxy -run 'TestManagement|TestManagedTenant|TestBuildRouterReturns|TestTextRequestDefaults'`, `timeout -k 300s -s SIGKILL 300s make go-test`, `timeout -k 180s -s SIGKILL 180s make frontend-lint`, `timeout -k 180s -s SIGKILL 180s make frontend-test`, and `git diff --check`.
- [x] [B004] (P2) Populate management request examples before secret generation.
  ### Summary
  The Settings modal should render useful request examples even before the signed-in user generates a client secret. The examples should use the generated secret when one was just created, and otherwise use the documented `<generated-secret>` placeholder.
  ### Acceptance Criteria
  1. The Settings modal always renders populated request examples for authenticated users.
  2. Users with a freshly generated secret see examples using the real generated secret.
  3. Users without a freshly generated secret see populated text, `/v2`, and dictation examples using `<generated-secret>`.
  4. Browser coverage proves the generated-secret and no-secret placeholder states through the real Settings modal.
  ### Resolution
  The Settings modal always renders the request examples section for authenticated users. The curl examples use the real generated secret immediately after secret generation, and otherwise use the `<generated-secret>` placeholder so users without a generated secret still see complete text, `/v2`, and dictation examples. Playwright coverage now asserts the populated placeholder state before secret generation and the populated generated-secret state. Validation passed with `timeout -k 180s -s SIGKILL 180s make frontend-lint`, `timeout -k 180s -s SIGKILL 180s make frontend-test`, and `git diff --check`.
- [x] [B005] (P1) Move Pages deployment out of GitHub Actions and keep browser config backend-owned.
  ### Summary
  GitHub Pages deployment must not spend GitHub Actions minutes. The management frontend also must not publish browser runtime config as a static Pages file or rendered HTML value; config belongs to the backend `/config-ui.yaml` projection.
  ### Acceptance Criteria
  1. Remove the GitHub Actions Pages deployment workflow.
  2. `make publish-pages`, `make publish`, and `make deploy` render and publish the Pages branch without Actions.
  3. The rendered Pages artifact contains no static `config-ui.yaml`, no `llm-proxy-config.json`, and no rendered `data-config-url`.
  4. The browser fetches runtime management config from the backend `/config-ui.yaml` endpoint.
  5. README and implementation docs describe the branch-published Pages contract and backend-owned browser config.
  ### Resolution
  Removed `.github/workflows/pages.yml` and the PR trigger entry for it. Added `scripts/publish_pages.sh` plus `make publish-pages`, wired `make publish` and `make deploy` to render and publish the `gh-pages` branch without GitHub Actions, and kept `--skip-pages`/`--skip-configure` operator controls. The Pages renderer no longer loads backend config, no longer writes static browser config, rejects retired static config URL markers, and removes stale `config-ui.yaml`/`llm-proxy-config.json` files from the artifact. The static frontend now resolves the backend config endpoint at runtime and fetches `/config-ui.yaml`; production uses `https://llm-proxy-api.mprlab.com/config-ui.yaml`, while local previews use same-origin `/config-ui.yaml`. README and provider-routing notes now document branch-published Pages and backend-owned browser config. Validation passed with `timeout -k 30s -s SIGKILL 30s bash -n scripts/publish_pages.sh scripts/publish.sh scripts/deploy.sh`, `timeout -k 120s -s SIGKILL 120s go test -count=1 ./cmd/cli -run 'TestRootCommand(Render|RejectsSite)'`, static JS `node --check`, `timeout -k 120s -s SIGKILL 120s ./scripts/publish_pages.sh --dry-run`, `timeout -k 350s -s SIGKILL 350s make go-test`, `timeout -k 180s -s SIGKILL 180s make frontend-lint`, `timeout -k 240s -s SIGKILL 240s make frontend-test`, `timeout -k 30s -s SIGKILL 30s git diff --check`, and `env -u LLM_PROXY_SECRET -u LLM_PROXY_BASE_URL timeout -k 500s -s SIGKILL 500s make ci`.
- [x] [B006] (P1) Make management admin configuration plural and deployable.
  ### Summary
  Gateway deployment failed when the hosted management config expected missing admin and provider-key encryption placeholders. The admin config was also represented as one env-backed list entry even though management supports multiple administrator emails.
  ### Acceptance Criteria
  1. Packaged `configs/config.yml` uses one plural admin-email placeholder that expands to the complete `management.admin_emails` list.
  2. The app repo owns a tracked hosted env sample for every packaged management placeholder, including admin emails and provider-key encryption.
  3. CLI config coverage proves the packaged config loads multiple admin emails through the real config entrypoint.
  4. Local ignored management env values include a generated provider-key encryption key without exposing the key in output.
  ### Resolution
  `configs/config.yml` now uses `LLM_PROXY_MANAGEMENT_ADMIN_EMAILS` as a YAML flow sequence for the full admin list. Added `configs/.env.sample` as the app-owned hosted env contract, updated README and implementation docs, and extended `TestRootCommandLoadsPackagedConfigWithManagementEnvironment` to load two admin emails through the packaged config path. The ignored local `configs/.env` was updated with the plural admin list and a generated stable provider-key encryption key without printing secret values. Validation passed with `timeout -k 240s -s SIGKILL 240s go test -count=1 ./cmd/cli -run TestRootCommandLoadsPackagedConfigWithManagementEnvironment` and `timeout -k 350s -s SIGKILL 350s make go-test`.
- [x] [B007] (P1) Make llm-proxy-client invalid-input tests immune to ambient client env.
  ### Summary
  `make release` failed in `TestCommandRejectsInvalidInputs/missing_secret` when the shell already had `LLM_PROXY_SECRET` set. The CLI correctly supports `LLM_PROXY_BASE_URL` and `LLM_PROXY_SECRET` as edge configuration, but the invalid-input table did not clear those inputs, so the missing-secret case proceeded to a real `example.test` POST instead of failing at config validation.
  ### Acceptance Criteria
  1. Invalid-input CLI subtests explicitly isolate `LLM_PROXY_BASE_URL` and `LLM_PROXY_SECRET`.
  2. The positive environment/stdin CLI test still proves env-based configuration works.
  3. Focused `llm-proxy-client` tests pass when the process has ambient `LLM_PROXY_SECRET` and `LLM_PROXY_BASE_URL`.
  ### Resolution
  `TestCommandRejectsInvalidInputs` now clears `LLM_PROXY_BASE_URL` and `LLM_PROXY_SECRET` at the test boundary, so every invalid-input scenario owns the external CLI configuration it is asserting. `TestCommandReadsEnvironmentAndStdin` still sets the same environment names explicitly and continues to prove env-backed client configuration. Validation passed with `env LLM_PROXY_SECRET=ambient-secret LLM_PROXY_BASE_URL=http://ambient.example timeout -k 120s -s SIGKILL 120s go test -count=1 ./llm-proxy-client -run TestCommandRejectsInvalidInputs`, `env LLM_PROXY_SECRET=ambient-secret LLM_PROXY_BASE_URL=http://ambient.example timeout -k 350s -s SIGKILL 350s make go-test`, and `env LLM_PROXY_SECRET=ambient-secret LLM_PROXY_BASE_URL=http://ambient.example timeout -k 350s -s SIGKILL 350s make ci`.
- [!] [B008] (P1) Published production image rejects current management config.
  ### Summary
  Production startup fails while parsing the mounted current `config.yml`:
  ```text
  config_file_parse_failed: path=config.yml: decoding failed due to the following error(s):
  '' has invalid keys: management
  ```
  The current repository source and CLI config tests accept the top-level `management` block, but the published `ghcr.io/tyemirov/llm-proxy:latest` image still rejects it. This indicates the deployed image is stale relative to the current config contract.
  ### Evidence
  1. Current source passes the packaged management config loader test:
  ```bash
  timeout -k 120s -s SIGKILL 120s go test -count=1 ./cmd/cli -run TestRootCommandLoadsPackagedConfigWithManagementEnvironment
  ```
  2. The published production image reproduces the production error with the current config mounted:
  ```bash
  docker run --rm -v "$(pwd)/configs/config.yml:/app/config.yml:ro" ghcr.io/tyemirov/llm-proxy:latest /usr/local/bin/llm-proxy --config /app/config.yml
  ```
  ### Expected
  The production image and mounted config must be advanced together. Do not remove `management` from config and do not add a compatibility parser path; `management` is part of the current canonical config contract.
  ### Blocked
  Blocked: Requires publishing an image built from the current management-aware source and redeploying the backend through the gateway-owned `deploy-llm-proxy-backend` path. Agent runs must stop before production deployment unless the execution chain or operator performs the publish/deploy step.
- [x] [B009] (P2) Validate management migration seed tenant defaults at startup.
  ### Summary
  Management mode allows an empty static `tenants` list because runtime authentication is DB-authoritative, but configured legacy tenants are still first-run migration seed data. Startup skipped tenant default provider/model validation whenever management mode was enabled, so a typoed seed default could be persisted and fail later at request time.
  ### Acceptance Criteria
  1. Management mode still accepts an empty static `tenants` list.
  2. Management mode validates any configured legacy tenant default text provider/model against the current provider catalog before migration.
  3. Management mode validates any configured legacy tenant default dictation provider/model against endpoint support and the current provider catalog before migration.
  4. Static provider credentials remain optional for management-mode seed validation because provider keys are DB-owned after migration.
  ### Resolution
  Split provider catalog resolution from request credential resolution. Static non-management startup still validates tenant defaults through the credential-aware request path, while management-mode seed tenants use catalog-only default validation before the migration runs. Added management startup coverage proving valid seed defaults do not require static provider credentials and invalid text/dictation default providers or models fail startup. Validation passed with `timeout -k 180s -s SIGKILL 180s go test -count=1 ./internal/proxy -run 'TestManagement(AcceptsLegacyTenantDefaultsWithoutStaticCredentials|RejectsInvalidLegacyTenantDefaults|MigratesLegacyConfigOnceThenUsesDatabase)'`, `timeout -k 350s -s SIGKILL 350s make go-test` (total coverage 100.0%), and `timeout -k 350s -s SIGKILL 350s make ci`.
- [x] [B010] (P2) Require expiration on management session JWTs.
  ### Summary
  Management session validation accepted a signed TAuth session JWT with the correct tenant and user claims when the token omitted `exp`. Because the JWT library validates expiration only when the claim exists, this could leave `/api/management/*` usable with a non-expiring signed cookie.
  ### Acceptance Criteria
  1. Management session validation rejects signed JWTs that omit `exp`.
  2. Existing valid TAuth session cookies with `exp` still authenticate management API requests.
  3. The regression is covered through the public management API route.
  ### Resolution
  `managementSessionValidator.validateToken` now rejects tokens whose registered claims do not include `ExpiresAt` before constructing a management principal. Added management API coverage for a signed session cookie with valid issuer, tenant, and user claims but no `exp`; `/api/management/profile` returns `401`. Validation passed with `timeout -k 180s -s SIGKILL 180s go test -count=1 ./internal/proxy -run TestManagementRejectsInvalidSessionsAndRequests`, `timeout -k 350s -s SIGKILL 350s make go-test` (total coverage 100.0%), and `timeout -k 350s -s SIGKILL 350s make ci`.
- [x] [B011] (P2) Remove unsupported no-dictation default option from management UI.
  ### Summary
  The management UI exposed a blank "No dictation default" option, but the backend has no persisted no-dictation state and normalizes empty dictation defaults back to the canonical OpenAI default. Selecting the blank option could make a text-only defaults save appear to work before the profile reloaded with OpenAI selected again.
  ### Acceptance Criteria
  1. The dictation-provider dropdown offers only concrete dictation-capable providers.
  2. The UI no longer sends empty dictation-provider defaults through a "none" selection.
  3. Removed UI copy is not left as a stale constant.
  ### Resolution
  Removed the blank dictation-provider `<option>` from `site/index.html` and deleted the unused `noDictationDefault` copy key. The dictation-provider select now lists only real dictation-capable providers from the backend profile. Validation passed with `timeout -k 30s -s SIGKILL 30s node --check` for the static JS modules and `timeout -k 350s -s SIGKILL 350s make ci`.
- [x] [B012] (P1) GitHub Pages frontend remains unavailable until the workflow fix reaches master.
  ### Summary
  `https://llm-proxy.mprlab.com/` presented GitHub's default `*.github.io` certificate and a Pages 404 because repository Pages was disabled and the Pages workflow render job depended on unset repository variables. The current split-origin contract requires `llm-proxy.mprlab.com` to be owned by GitHub Pages, not by the gateway backend.
  ### Evidence
  1. `dig +short llm-proxy.mprlab.com` points at `tyemirov.github.io` and GitHub Pages IPs.
  2. `curl -I https://llm-proxy.mprlab.com/` failed with `SSL: no alternative certificate subject name matches target host name 'llm-proxy.mprlab.com'`.
  3. `gh api repos/tyemirov/llm-proxy/pages` returned 404 before Pages was enabled.
  4. The latest Pages workflow failed while rendering `configs/config.yml` with empty `LLM_PROXY_MANAGEMENT_*` values and no render-only OpenAI key.
  ### Local Resolution
  Superseded by B015. Pages deployment is now Makefile-owned through `make publish-pages`, `make publish`, and `make deploy`; `.github/workflows/pages.yml` is removed. Local static rendering succeeds with `CNAME: llm-proxy.mprlab.com`, no static browser config files, and no rendered `data-config-url`.
  ### Remote State
  GitHub Pages should be configured for branch publishing from `gh-pages` at `/`, with `cname=llm-proxy.mprlab.com` and HTTPS enforcement enabled. `scripts/publish_pages.sh` configures this source through the GitHub API unless `--skip-configure` is passed. Public availability still requires an operator to run the publish/deploy command after this branch reaches the release path.
- [x] [B013] (P2) Fix F007 review issues in usage loading and usage queries.
  ### Summary
  F007 review found two dashboard/settings defects: authenticated profile loading could remain stuck on the loading panel when `/api/management/usage` failed, and the 30-day usage summary store query loaded every historical user usage event before filtering in memory.
  ### Acceptance Criteria
  1. A successful profile load transitions the UI into the authenticated workspace even when usage summary loading fails.
  2. The usage dashboard shows an error notice and empty usage state when usage loading fails, while Settings remains reachable from the avatar menu.
  3. The usage store query accepts the dashboard period start and only reads rows in the returned window.
  4. The usage event table has an index suitable for user/time usage summary reads.
  ### Resolution
  Decoupled frontend profile loading from usage loading, so profile success unlocks the authenticated dashboard and usage failures reset usage to an empty summary with a visible error notice. Added Playwright coverage for profile success plus usage failure still opening Settings. Changed the managed usage database interface to `usageEventsByUserIDSince`, passed the computed 30-day period start from `usageSummary`, filtered fake and GORM store reads at the database boundary, and added a composite GORM index on usage `user_id` plus `created_at`. Validation passed for this review-fix scope with `timeout -k 120s -s SIGKILL 120s go test -count=1 ./internal/proxy -run 'TestManagedTenantStoreUsageEdges|TestManagedUsageSummaryBucketsAndOrdering|TestManagementUsageSummaryRecordsManagedProxyRequests'`, `timeout -k 120s -s SIGKILL 120s make frontend-lint`, and `timeout -k 180s -s SIGKILL 180s make frontend-test` (3 Playwright tests passed). An initial full `timeout -k 350s -s SIGKILL 350s make ci` passed before unrelated admin/session edits appeared; a later current-worktree `make ci` stopped at check-format because unrelated `internal/proxy/management_session.go` needs gofmt.
- [x] [B014] (P2) Fix F007 usage dashboard follow-up review findings.
  ### Summary
  F007 follow-up review found three defects: validation failures after managed tenant authentication were not counted in usage summaries, admin usage range reads lacked a created-at-only index, and manual usage refresh failures left stale dashboard metrics visible.
  ### Acceptance Criteria
  1. Managed text, v2, and dictation validation failures record usage status metadata without storing prompts, request bodies, provider keys, tenant secrets, audio, transcripts, or responses.
  2. Usage events have an index suitable for admin time-window scans as well as signed-in user/time summary reads.
  3. Manual usage refresh failure resets the dashboard to the empty usage state and keeps Settings reachable.
  4. Focused Go and Playwright coverage proves the fixes.
  ### Resolution
  Authenticated managed proxy handlers now record validation failures after rejection responses using only endpoint, provider/model identifiers, status code, and latency metadata. Added the `created_at` usage index alongside the user/time composite index and covered both indexes in the GORM migration test. Manual usage refresh failures now reset the usage summary to the empty state, with Playwright coverage proving stale counts disappear while the dashboard remains usable. Validation passed with `timeout -k 120s -s SIGKILL 120s go test -count=1 ./internal/proxy -run 'TestManagementUsageSummaryRecordsManagedProxyRequests|TestManagedTenantGORMDatabaseMigratesUsageIndexes|TestManagedTenantStoreUsageEdges'`, `timeout -k 120s -s SIGKILL 120s make frontend-lint`, `timeout -k 180s -s SIGKILL 180s make frontend-test`, and `env -u LLM_PROXY_SECRET timeout -k 350s -s SIGKILL 350s make ci`. A plain `make ci` attempt in this shell first failed because ambient `LLM_PROXY_SECRET` bypassed the missing-secret CLI test; the clean-env CI run passed.
- [x] [B015] (P1) Gemini POST responses can return thought or partial text as successful output.
  ### Summary
  A production-comparable Russian semantic-stress QA run sent the full prompt through `POST /?provider=gemini` with `model=gemini-3.5-flash`, but the client received a non-JSON response and failed before materialization. The same prompt contract succeeds only when the proxy returns the model's answer text as parseable JSON or returns a structured proxy/provider error.
  ### Impact
  This blocks using Gemini as an alternate semantic reviewer for long JSON-only prompts. Downstream clients cannot distinguish model formatting drift, Gemini thought-part leakage, truncation, and proxy/provider errors; they only see invalid text even though the HTTP request path appears successful.
  ### Reproduction
  From the sibling Russian-language/Camu workflow, run the full pipeline rather than calling the proxy directly:
  ```bash
  /Users/tyemirov/Development/Smith/russian-language/russian_language/pipeline_runner.py \
    --output-dir /Users/tyemirov/Documents/Projects/Camu/fairy-tales/runs/russian-language-puzyr-vanilla-gemini35flash-20260604T052031Z \
    --llm-proxy-base-url "https://llm-proxy.mprlab.com/?provider=gemini" \
    --llm-proxy-model gemini-3.5-flash \
    --llm-proxy-timeout-seconds 300 \
    --llm-proxy-single-request \
    /Users/tyemirov/Documents/Projects/Camu/fairy-tales/puzyr-solominka-i-lapot/source/narration.txt
  ```
  The semantic QA stage sends a POST body equivalent to:
  ```json
  {
    "prompt": "<large semantic stress QA prompt>",
    "model": "gemini-3.5-flash",
    "web_search": false
  }
  ```
  ### Observed
  The full pipeline completed deterministic format, yofication, yofication QA, RuAccent stressor, and stressor QA, then failed in `llm-proxy` with:
  ```text
  semantic review response is not valid JSON and --llm-proxy-single-request forbids retry
  ```
  Earlier Gemini evidence for the same story captured a 1278-byte response that started with `thought`, then a fenced JSON block, and cut off mid-string:
  ```text
  /Users/tyemirov/Documents/Projects/Camu/fairy-tales/runs/russian-language-puzyr-vanilla-gemini35flash-20260604T052031Z/direct-semantic-evidence/puzyr-gemini35flash.llm_response.txt
  ```
  ### Suspected proxy gaps
  `internal/proxy/gemini.go` currently parses only `candidates[].content.parts[].text`, concatenates every non-empty text part, and does not model Gemini response fields such as part-level `thought` or candidate-level `finishReason`. That can make the proxy return internal/thought text or a non-final answer as HTTP 200 plain text instead of returning only the final answer text or a provider error.
  ### Expected
  For Gemini text generation, `llm-proxy` should:
  1. Return only final, user-visible answer text from Gemini response parts; thought/internal parts must not be concatenated into the client-visible response.
  2. Treat non-terminal or truncated Gemini candidates as a proxy/provider error rather than returning partial text as success.
  3. Preserve the existing plain-text response contract when the provider returns a complete final answer.
  ### Acceptance Criteria
  1. Add black-box/integration-style tests for `POST /?provider=gemini` with a fake Gemini upstream response containing a thought part plus an answer part; the proxy returns only the answer text.
  2. Add a fake Gemini upstream response with a truncation/non-final `finishReason`; the proxy maps it to a failure status instead of returning partial text.
  3. Rerun the Russian semantic-stress QA prompt through the full pipeline and verify the client receives either parseable JSON or a structured proxy error, not a thought-prefixed or truncated successful text body.
  ### Resolution
  Added black-box Gemini POST coverage for thought-marked parts returning only final answer text, non-final `finishReason` values mapping to `502`, and missing `finishReason` mapping to `502`. `internal/proxy/gemini.go` now models `parts[].thought` and candidate `finishReason`, returns only non-thought text for completed `STOP` candidates, and treats missing or non-`STOP` finish reasons as provider API errors instead of successful partial output. Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 350s -s SIGKILL 350s make test` (total coverage 100.0%), `timeout -k 350s -s SIGKILL 350s make lint`, and `timeout -k 350s -s SIGKILL 350s make ci` (total coverage 100.0%).
- [x] [B016] (P1) Long semantic-review POSTs fail transport while small requests pass.
  ### Summary
  Camu Russian semantic-stress QA can still fail before materialization on long full-story prompts even though a small `llm-proxy` smoke request succeeds. On 2026-06-09, `po-schuchemu-velenyu` text prep failed three times at the `llm-proxy` stage: one read timeout, then two SSL record-layer failures. A tiny Russian pipeline smoke test using the same Russian-language `pipeline_runner.py` reached `semantic-stress-qa` and `materialize` successfully, so the service is not fully down; the failure appears tied to long-running or larger semantic-review POSTs and/or chunk retry transport handling.
  ### Impact
  Downstream production workflows cannot safely materialize corrected TTS for long stories. In the observed Camu run, this blocked applying an occurrence-scoped pronunciation correction for `Слез` versus `Слёз`; regenerating audio from the existing stale `tts.txt` would reproduce the old pronunciation.
  ### Reproduction
  From the Camu checkout, run the long story materialization:
  ```bash
  tools/camu_audio.py prepare-text \
    fairy-tales/po-schuchemu-velenyu/source/narration.txt \
    --output fairy-tales/po-schuchemu-velenyu/pronunciation \
    --skip-speech-performance
  ```
  Retried variants also failed:
  ```bash
  tools/camu_audio.py prepare-text \
    fairy-tales/po-schuchemu-velenyu/source/narration.txt \
    --output fairy-tales/po-schuchemu-velenyu/pronunciation \
    --skip-speech-performance \
    --llm-proxy-timeout-seconds 240 \
    --llm-proxy-chunk-chars 3000
  LLM_PROXY_FORCE_CHUNKED=1 tools/camu_audio.py prepare-text \
    fairy-tales/po-schuchemu-velenyu/source/narration.txt \
    --output fairy-tales/po-schuchemu-velenyu/pronunciation \
    --skip-speech-performance \
    --llm-proxy-model gpt-5-mini \
    --llm-proxy-timeout-seconds 240 \
    --llm-proxy-chunk-chars 1800
  ```
  As a control, this tiny pipeline passed through `semantic-stress-qa` and `materialize`:
  ```bash
  mkdir -p /tmp/camu-llm-proxy-smoke
  printf 'Жил-был кот. Он любил тёплое молоко.\n' | \
    /Users/tyemirov/Development/Smith/russian-language/russian_language/pipeline_runner.py \
      --output-dir /tmp/camu-llm-proxy-smoke \
      --basename smoke \
      --to materialize \
      --quiet
  ```
  ### Observed
  The long story runs completed deterministic Russian-language stages and then failed at `llm-proxy` with:
  ```text
  llm_proxy_client_transport_failure: The read operation timed out
  [SSL] record layer failure (_ssl.c:2658)
  ```
  The forced-chunked `gpt-5-mini` retry also failed with the same SSL record-layer error before materialization. The small control request reported `semantic-stress-qa` passed with `transport: post`, `invocationMode: single`, and `materialize` passed.
  2026-06-10 live retry after updating the Russian-language caller to the current v2 Python client contract (`ClientMessagesRequest` + `POST /v2`) still failed against `https://llm-proxy.mprlab.com` for the 8,528-character Camu story. Focused Russian-language transport tests passed, and a tiny live v2 pipeline smoke reached `semantic-stress-qa` and `materialize`. The long story failed before materialization with:
  ```text
  llm_proxy_client_transport_failure: provider=omitted model=gpt-5.5 timeout_seconds=240 reason=[SSL] record layer failure (_ssl.c:2658)
  llm_proxy_client_transport_failure: provider=omitted model=gpt-5-mini timeout_seconds=600 reason=[SSL] record layer failure (_ssl.c:2658)
  ```
  Failed variants included normal v2 POST with `--llm-proxy-timeout-seconds 240 --llm-proxy-chunk-chars 1800`, forced chunking with `--llm-proxy-chunk-chars 900`, and forced chunking with `--llm-proxy-model gpt-5-mini --llm-proxy-timeout-seconds 600 --llm-proxy-chunk-chars 450`. This suggests the current branch/local replay fix has not reached, or is not sufficient for, the deployed default live endpoint used by Camu.
  2026-06-10 deployed gateway inspection found the public `llm-proxy` Caddy transport using `response_header_timeout 240s` and `read_timeout 240s`, exactly equal to the canonical `server.request_timeout_seconds: 240` staged from `configs/config.yml`. That can race the proxy-owned final response or structured `504` and surface as an SSL record-layer failure from the public TLS endpoint. A first gateway-side patch raised the dedicated Caddy transport to 300 seconds, but a later Camu production retry with `--llm-proxy-timeout-seconds 300 --llm-proxy-chunk-chars 1800` still failed with the same SSL record-layer transport error. The follow-up raised the proxy runtime deadline to 360 seconds, packaged client defaults and the Russian-language semantic gate to 390 seconds, and the gateway `llm_proxy_transport` to 420 seconds so the service owns completion or structured timeout before Caddy or clients close the socket. After the operator redeployed those changes, hosted long semantic-review replays through `https://llm-proxy.mprlab.com` returned HTTP 200 instead of SSL transport failures.
  ### Expected
  Long semantic-review POSTs should either complete successfully or return a structured proxy/provider error that the client can classify and retry. The proxy should not leave clients with opaque transport failures after the upstream request may still be in progress or recoverable.
  ### Acceptance Criteria
  1. Add production-comparable black-box coverage for a long POST body where the upstream completes after the normal client wait; the proxy either streams/polls/waits correctly or returns a structured timeout error with retry guidance.
  2. Add coverage for chunked semantic-review retry behavior so chunk transport failures are reported with chunk index, provider, model, timeout, and upstream status when available.
  3. Verify the Russian-language `po-schuchemu-velenyu` materialization path reaches `materialize` or returns a structured proxy error instead of `read operation timed out` or SSL record-layer failure.
  4. Preserve the existing successful behavior for small POST requests.
  ### Resolution
  OpenAI Responses text requests now run in background mode by sending `background: true` and `store: true` on initial, continuation, and synthesis payloads, then polling the returned response id instead of holding long semantic-review generations on one synchronous upstream HTTP read. Added black-box integration coverage for a long semantic-review JSON POST that returns a queued background response, polls by response id, and returns the completed body. Python client HTTP and transport failures now include non-secret provider, model, and timeout context, and raw OS/SSL-style failures are typed as `LLMProxyTransportError`. B003 supersedes the remaining manual tuning gap with the final one-shot REST contract.
  Validation for the initial background-mode fix passed with `timeout -k 350s -s SIGKILL 350s make go-test` (Go total coverage 100.0%), `timeout -k 350s -s SIGKILL 350s make python-test` (27 passed), `timeout -k 350s -s SIGKILL 350s make python-lint`, `timeout -k 350s -s SIGKILL 350s make go-lint`, and `timeout -k 350s -s SIGKILL 350s make ci` (Go total coverage 100.0%, Python 27 passed). The exact reported forced-chunked semantic-review payload was reconstructed from the failed Camu pipeline state (`chunkIndex=1`, `chunkChars=1800`, request SHA-256 `ba8737a7dd4b81dc0d52e68d9260756fdb0f32e6e12f3be2527d8be55df1a0cd`) and proved that OpenAI could complete the response in about 171 seconds, motivating B003.
  Post-redeploy live validation passed with a hosted `gpt-5.5` v2 smoke returning `OK` in 3.289s, hosted replay of `/tmp/llm-proxy-b002-direct/po-schuchemu-velenyu.chunk-0001.llm-proxy-request.json` returning HTTP 200 in 152.981s with valid JSON, 23 `targetReviews`, and `needsHumanReview=false`, hosted Python v2 `gpt-5.5` replay of the same long semantic-review prompt returning in 151.524s with valid JSON, 23 `targetReviews`, and `needsHumanReview=false`, and `timeout -k 240s -s SIGKILL 240s make test-live-providers LIVE_ENV_FILE=configs/.env LLM_PROXY_LIVE_PROVIDERS=openai` reporting `live provider smoke passed: provider=openai model=omitted status=200`.
- [x] [B017] (P1) OpenAI background semantic-review calls require manual timeout tuning.
  ### Summary
  B002 moved long OpenAI Responses calls into background mode, but the successful manual replay still required hand-editing timeout knobs. A normal llm-proxy REST call should not require operators, downstream workflows, or client libraries to guess provider polling behavior for a semantic-review payload.
  ### Impact
  Downstream callers can still fail a viable OpenAI background response, then retry from scratch or require caller-specific timeout overrides. This keeps long semantic review fragile even though the provider can complete the response.
  ### Reproduction
  Reuse the reported `po-schuchemu-velenyu` forced-chunked semantic-review request reconstructed from the failed Camu pipeline state:
  ```text
  /tmp/llm-proxy-b002-direct/po-schuchemu-velenyu.chunk-0001.llm-proxy-request.json
  ```
  Before this fix, the default proxy returned a classified 504 after about 60 seconds. With a temporary larger timeout, the same request returned HTTP 200 after about 171 seconds and validated as a full semantic-review response.
  ### Expected
  The proxy should own long OpenAI background response completion. A simple REST caller should issue one `GET /`, `POST /`, or `POST /v2` request through llm-proxy and receive the final answer without streaming, client-side polling, or a resume-token protocol.
  ### Acceptance Criteria
  1. OpenAI background response polling is server-side and bounded only by `server.request_timeout_seconds`.
  2. The public `GET /`, `POST /`, and `POST /v2` contract does not require streaming, client-side polling, or `/responses` resume calls.
  3. The obsolete poll-timeout configuration is removed from the public static config surface.
  4. Bundled Go and Python clients remain simple one-request REST transports; they do not implement OpenAI background polling.
  5. Black-box integration tests cover a long background semantic-review POST returning HTTP 200 in the original request after multiple server-side polls.
  ### Resolution
  OpenAI background response polling is now owned by llm-proxy for the normal REST request lifecycle. Initial, continuation, and synthesis OpenAI Responses payloads use `background: true` and `store: true`; when OpenAI returns a non-terminal response id, llm-proxy polls that stored response server-side until completion or the normal `server.request_timeout_seconds` deadline. The public `/responses` resume endpoint and the Go/Python client resume loops were removed, and `upstream_poll_timeout_seconds` was removed from the static config surface. Default `request_timeout_seconds` is now 360 seconds, while the packaged Go CLI and Python client default to a 390-second transport timeout so one normal REST request has room to complete.
  Validation passed with `timeout -k 350s -s SIGKILL 350s make go-test` (Go total coverage 100.0%), `timeout -k 350s -s SIGKILL 350s make python-test` (Python 27 passed), `timeout -k 350s -s SIGKILL 350s make ci` (Go/Python lint clean, Go total coverage 100.0%, Python 27 passed), and `timeout -k 30s -s SIGKILL 30s git diff --check`. Live OpenAI smoke passed with `model` omitted, using the provider default, and HTTP 200. Live exact-payload replay of `/tmp/llm-proxy-b002-direct/po-schuchemu-velenyu.chunk-0001.llm-proxy-request.json` through the local current-branch proxy returned HTTP 200 from one `POST /?provider=openai&format=text/plain` in 150.394s with no `X-LLM-Proxy-Resume-*` or raw upstream response-id headers. The final body was valid JSON with 23 `targetReviews`, `needsHumanReview=true`, and 2 human-review items for `проруби`/`прорубь` stress confirmation.
- [x] [B018] (P1) Polled OpenAI terminal responses skip continuation and synthesis handling.
  ### Summary
  B003 made OpenAI Responses run in background mode, so terminal OpenAI response payloads commonly arrive from the server-side GET poll path. The initial POST path still handled `status: "incomplete"` max-token continuations and completed responses without a final assistant message, but the poll path collapsed terminal responses to text too early and could return `502` or fallback text instead of continuing or synthesizing the final answer.
  ### Impact
  A simple REST caller could still fail a viable long OpenAI response when the stored response reached `incomplete_details.reason=max_output_tokens` during polling, or receive an unhelpful tool-call fallback when a polled completed response lacked a final assistant message.
  ### Acceptance Criteria
  1. Initial POST and GET poll OpenAI Responses terminal payloads share the same continuation and synthesis rules.
  2. A polled `status: "incomplete"` response with `max_tokens`/`max_output_tokens` incomplete reason starts a stored continuation and returns the final answer in the original REST response.
  3. A polled completed response without a final assistant message starts synthesis before returning to the caller.
  4. Existing background polling, timeout, and malformed-upstream error behavior remains covered by black-box HTTP tests.
  ### Resolution
  OpenAI response handling now parses initial and polled Responses payloads into a shared response snapshot and resolves terminal states through one lifecycle handler. The shared handler preserves max-token continuation, completed-without-final-message synthesis, usage merging, malformed JSON classification, and the blocking one-shot REST contract for both initial POST responses and polled GET responses. Added black-box router coverage for polled incomplete continuation and polled completed tool-only synthesis.
  Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 180s -s SIGKILL 180s go test -count=1 ./internal/proxy -run TestCoverageOpenAILifecycleBranches`, `timeout -k 180s -s SIGKILL 180s go test -count=1 ./tests/integration -run 'TestIntegration(LargeSemanticReviewPost|BackgroundPollSleepDoesNotOccupyUpstreamWorker)'`, `timeout -k 180s -s SIGKILL 180s go test -count=1 ./internal/proxy`, `timeout -k 350s -s SIGKILL 350s make go-test` (Go total coverage 100.0%), and `timeout -k 350s -s SIGKILL 350s make ci` (Go lint/staticcheck/ineffassign clean, Python mypy clean, Go total coverage 100.0%, Python 27 passed, root import smoke passed).
- [x] [B019] (P1) PR merge CI drops limiter coverage below 100%.
  ### Summary
  GitHub Actions ran PR #175 on the generated pull-request merge ref with Go 1.24.13 and failed `make go-test` at total coverage 99.8%. The function table showed partial coverage for `internal/proxy/limited_http.go` `Do` and `acquire`, even though branch-head local runs reached 100.0%.
  ### Impact
  The PR cannot merge while the coverage gate is red, and the gap is specifically in the concurrency limiter path that should stay covered because it controls upstream worker admission and queue release.
  ### Acceptance Criteria
  1. The limiter path where an admitted request times out before acquiring an upstream worker is covered deterministically.
  2. The test goes through the router/provider path rather than weakening the coverage gate.
  3. The Go 1.24 merged coverage gate reports total 100.0%.
  ### Resolution
  Added router-level coverage for a blocked DeepSeek-compatible upstream request holding the only active worker while a second admitted request times out before acquiring that worker. The scenario covers the limiter `acquire` context-cancel path and `Do` admission-release path deterministically.
  Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 180s -s SIGKILL 180s env GOTOOLCHAIN=go1.24.13 go test -count=1 ./internal/proxy -run 'TestCoverageProviderRoutingEdges/admitted_provider_request_timeout_releases_queue_slot_before_acquiring_worker'`, `timeout -k 350s -s SIGKILL 350s env GOTOOLCHAIN=go1.24.13 make go-test` (Go total coverage 100.0%), and `timeout -k 350s -s SIGKILL 350s make ci` (Go lint/staticcheck/ineffassign clean, Python mypy clean, Go total coverage 100.0%, Python 20 passed, root import smoke passed).
- [x] [B020] (P0) Adjust settings modal layout relative to header and footer.
  Goal:
  Ensure the settings modal is visually positioned between the header and footer (or layered in front of them) rather than appearing underneath them, so users can always see and interact with the modal contents.
  
  Requirements:
  - The settings modal must not render visually below or obscured by the header or footer.
  - The modal should either:
    - appear between the header and footer in the visual stacking order, or
    - appear in front of both header and footer as a proper overlay.
  - Preserve existing modal functionality and behavior (open/close, focus handling, scrolling, etc.).
  - Do not introduce layout regressions for other modals or pages that use the same layout system.
  - Respect existing design system tokens (spacing, colors, z-index scale) and responsive breakpoints.
  
  Deliverables:
  - Updated layout or styling for the settings modal to correct its stacking/placement relative to the header and footer.
  - Any necessary adjustments to shared layout/container components or z-index utilities to support the intended layering.
  - Inline code comments or brief documentation describing how the modal should be layered relative to header/footer to avoid future regressions.
  - Screenshot(s) or short clip demonstrating the corrected modal position on common viewports (e.g., desktop and one mobile size).
  
  Validation:
  - Open the settings modal and confirm it is fully visible and interactive, not hidden behind or rendered underneath the header or footer.
  - Resize the viewport across supported breakpoints and verify the modal remains correctly layered between or in front of header and footer.
  - Confirm that closing/opening the modal multiple times does not cause layout shifts or stacking anomalies.
  - Verify that other modals or overlays still appear correctly and are not negatively impacted by the changes.
  ### Resolution
  The Settings overlay now uses an explicit llm-proxy modal layer above the sticky MPR header/footer and the built-in MPR modal layer, with a CSS comment documenting that stacking contract. The notice layer stays below the Settings overlay so persistent notices remain visible outside modal flows without covering or intercepting Settings controls. Browser coverage now models the real MPR shell header/footer z-indexes, verifies the Settings modal and overlay win pointer hit-testing over header/footer and notices on desktop and mobile viewports, and proves repeated close/open remains stable. Screenshot evidence was captured at `output/playwright/B020-settings-desktop.png` and `output/playwright/B020-settings-mobile.png`. Validation passed with `timeout -k 180s -s SIGKILL 180s make frontend-lint`, `timeout -k 180s -s SIGKILL 180s npm run frontend:test -- --grep "settings modal overlays MPR header and footer layers"`, `timeout -k 240s -s SIGKILL 240s make frontend-test`, and `timeout -k 30s -s SIGKILL 30s git diff --check`.

- [x] [B021] (P1) Resolve Meta, upstream-rate, and release review regressions.
  ### Summary
  Review of the Meta Muse Spark 1.1 and shared upstream-rate branch found seven concrete regressions: rate reservations can expire while requests wait for workers; release commands depend on unavailable sibling scripts; canonical container preparation exposes ignored local files to BuildKit; Pages verifies the wrong commit after activation; the gateway no-op verifier no longer matches deploy CLI behavior; Meta live smoke reuses management state; and generated Meta pages carry a pre-launch modification date.
  ### Acceptance Criteria
  1. Strict rolling-window limits are enforced at actual upstream call admission even when a worker is saturated, and canceled worker waits do not consume rate slots.
  2. `make release`, `make publish`, and Pages deployment use a repository-owned, reproducible toolchain with black-box local release coverage.
  3. Release container builds use a tracked source snapshot that cannot contain `.git`, ignored dotenv credentials, or local artifact state.
  4. Pages activation verifies `.mprlab-release.json.source_commit` against the release manifest's `source_commit`, without mutating the branch and then failing against `release_commit`.
  5. The canonical gateway workflow no-op succeeds without CI, image, gateway, or Pages effects, while real deploys retain an explicit CI gate.
  6. The Meta live harness safely loads dotenv data, forces management off for its disposable static tenant, and never reuses the configured management database.
  7. Generated JSON-LD `dateModified`, sitemap `lastmod`, and report generation dates reflect the Meta content update.
  8. Release inputs and extracted Pages markers are validated before remote mutation, prerelease tags remain prereleases rather than `latest`, and release-tool-only changes trigger CI.
  9. Focused black-box tests, authenticated Meta live smoke, deterministic generation, and the full `make ci` gate pass.
  ### Resolution
  Moved strict rolling-window reservation to actual upstream admission after worker acquisition, releases workers while waiting for a rate window, and rejects post-acquire cancellation before consuming a slot. Vendored the release implementation under `tools/gitrelease`, made container preparation use `git archive HEAD`, constrained versions to the single canonical SemVer contract, honored selected remotes, kept prereleases out of `latest`, and made release-tool-only changes trigger CI. Pages deployment now validates retry inputs, archive entry types, case-insensitive `.git` components, payload hashes, remote tags, and the extracted source/version marker before branch mutation, then verifies the public marker against the manifest source commit. Restored the deploy CI/no-op contract so `--skip-gateway` requires no gateway checkout and cannot activate Pages. Combined the forward-only management changes with a disposable static live harness that parses dotenv without shell execution, supports non-paid preflight and config inspection, and never opens hosted management state. Regenerated all 45 resource pages with `dateModified`, sitemap `lastmod`, and report generation date `2026-07-09` while preserving the July 6 publication date.

  Validation passed with focused red-to-green operational and release tests, `go test -race -count=10 ./tests/integration -run 'TestIntegrationCanceledWorkerAcquisitionsDoNotReserveRateSlots|TestIntegrationUpstreamRateLimitReservesAtCallAdmissionAfterWorkerWait'`, `make release-test` (15 tests), `make test-live-provider-harness`, deterministic `node scripts/generate_seo_resources.mjs`, and final merged `make ci` (Go aggregate coverage 100.0%, Python 20 passed, Playwright 11 passed, release integrations 15 passed, non-paid live preflight passed). The authenticated Meta-only smoke mapped the ignored `MUSE11_API_KEY` into the canonical `MODEL_API_KEY` process variable and returned `200 OK` with the expected `OK` response without printing or persisting the credential. Final independent audits and `git diff --check` reported no remaining findings.

- [x] [B022] (P2) Validate the effective Pages push repository before deployment.
  ### Summary
  PR review found that Pages repository identity validation reads only `remote.<name>.pushurl`. When no explicit push URL exists, a configured `url.*.pushInsteadOf` rewrite is ignored by that check even though `git remote get-url --push` applies it to the later `gh-pages` push, allowing release and API operations to target the fetch repository while the branch is written elsewhere.
  ### Acceptance Criteria
  1. Pages deployment resolves the selected remote's effective push URL before release download or Pages mutation.
  2. The effective push destination must identify the same GitHub repository as the configured fetch URL or deployment fails before remote mutation.
  3. Black-box release coverage proves a mismatched `url.*.pushInsteadOf` destination is rejected when `remote.<name>.pushurl` is absent.
  4. The final branch push cannot apply another `pushInsteadOf` rewrite after the destination has passed validation.
  ### Resolution
  Pages deployment now resolves `git remote get-url --push` before release download, compares that effective destination with the configured fetch repository identity, and restricts `GH_REPO` fallback to fetch scoping so a different unparseable push target cannot inherit the expected identity. The deployment clone receives the validated URL as an explicit `pushurl`, its checkout-effective destination is validated again, and the branch is pushed by remote name so Git cannot apply `pushInsteadOf` a second time. Black-box coverage proves parseable and unparseable mismatched rewrites fail before GitHub access or either branch mutation, and a chained `A -> B -> C` rewrite deploys only to the validated `B` repository. Validation passed with `timeout -k 350s -s SIGKILL 350s make release-test` (30 tests), `git diff --check`, and an independent review with no remaining findings.

- [x] [B023] (P1) Preserve Pages release markers under branch publishing.
  ### Summary
  Repository-owned Pages artifacts can omit `.nojekyll`, allowing legacy branch publishing to filter the hidden `.mprlab-release.json` marker even when the deployed branch contains it. The artifact and public-marker contract also needs explicit black-box proof for schema, release version, source provenance, and the distinct release-tag commit.
  ### Acceptance Criteria
  1. Every prepared Pages archive contains an empty `.nojekyll` file alongside `.mprlab-release.json`.
  2. Deployment rejects a published Pages archive without `.nojekyll` before branch mutation.
  3. Extracted and public release markers validate schema version, release version, and source commit.
  4. Black-box coverage proves the remote tag matches `release_commit` while artifact and public marker provenance match the distinct `source_commit`.
  ### Implementation
  The Pages builder now creates an empty `.nojekyll` after copying the source tree, archive validation rejects payloads without it, and public verification checks the complete marker identity rather than source commit alone. Release-pipeline coverage now inspects the builder's tarball and marker, exercises missing `.nojekyll` and invalid marker fields, and proves a release succeeds with intentionally different release and source commits.
  ### Resolution
  The Pages builder now creates an empty `.nojekyll` after copying the source tree, archive validation rejects payloads without it before branch mutation, and public verification checks the complete marker identity rather than source commit alone. Release-pipeline coverage inspects the builder's tarball and marker, exercises missing `.nojekyll` and invalid marker fields, and proves a release succeeds with intentionally different release and source commits. Validation passed with `make release-test` (35 tests) and `make ci` (Go aggregate coverage 100.0%, Python 20 passed, Playwright 12 passed, release integrations 35 passed, and the live-provider preflight passed).

- [x] [B024] (P1) Prevent shell help deadlocks under constrained pipe limits.
  ### Summary
  The first release after the repository-owned toolchain changes stalls in `TestOperationalReleaseWrapperUsesRepositoryOwnedTools` until the outer 350-second CI guard sends `SIGKILL`. On shells with `ulimit -p 1`, Bash can block while staging a heredoc larger than the 512-byte pipe capacity before the external reader starts; the same pattern exists across release, deployment, artifact, coverage, publication, and live-provider scripts, including later Python transforms on the default release path.
  ### Acceptance Criteria
  1. CI, release, publish, deployment, artifact, Pages, and live-provider shell scripts do not feed external commands through heredocs.
  2. Release, deployment, artifact, Pages, container publication, and live-provider help commands terminate under the constrained-pipe shell contract.
  3. Dotenv parsing, coverage fixture generation, Pages marker generation, and container metadata transforms retain their exact current contracts without pipe-capacity dependence.
  4. Tracked container build context extraction preserves the complete `git archive HEAD` snapshot without a producer/consumer pipe that can surface a false `SIGPIPE` failure.
  5. Black-box operational coverage bounds help execution and rejects reintroduction of an external usage writer.
  6. The focused operational test, Go coverage suite, release integration suite, and full `make ci` gate pass without increasing the CI timeout.
  ### Resolution
  Replaced external-command heredocs across CI, release, publish, deployment, artifact, Pages, and live-provider scripts with Bash builtin output or direct Python command bodies, removing the pre-reader pipe staging that deadlocked with `ulimit -p 1`. Container build context extraction now writes `git archive HEAD` to a temporary tar before extraction so a completed reader cannot turn padding writes into a false `SIGPIPE` failure. Black-box operational coverage bounds every help command under the constrained-pipe contract, rejects shell heredocs in the governed script trees, validates the canonical container descriptor, and retains the existing dotenv, Pages marker, and publication coverage. Validation passed with focused operational tests, `bash -n`, `make go-test` (aggregate coverage 100.0%), `make release-test` (35 tests), `git diff --check`, and the unchanged 350-second `make ci` gate in 77.66 seconds.
- [x] [B025] (P1) Restore release pipeline tests after prepare release exits 2.
  ### Summary
  `python3 -m unittest discover -s tools/gitrelease/tests -p 'test_*.py'` fails three release-pipeline tests because `tools/gitrelease/scripts/prepare_release.sh --version v1.0.0` exits with status 2 in prepared-release paths.
  ### Acceptance Criteria
  1. `prepare_release.sh --version v1.0.0` succeeds in the black-box prepared-release fixture without weakening the canonical SemVer, artifact hashing, Pages marker, or selected-remote tag contracts.
  2. The prepared publish tests continue to validate the selected remote tag before and after publication.
  3. The full release integration suite and required final `make ci` pass after the fix.
  ### Resolution
  `prepare_release.sh` now runs its nested `make ci` with release-only artifact environment variables removed, while preserving those variables for the real artifact-preparation phase after CI. The release-pipeline test harness now isolates ambient release variables from parent shells, and new black-box coverage proves nested CI does not see release artifact variables while the artifact target still receives the current release version, timestamp, and artifact directory.

  Validation passed with baseline `timeout -k 350s -s SIGKILL 350s make ci`, reproduced failure via `timeout -k 120s -s SIGKILL 120s env RELEASE_ARTIFACT_TARGETS="container-artifacts pages-artifact" python3 -m unittest ...` before the patch, focused post-fix `timeout -k 180s -s SIGKILL 180s env RELEASE_ARTIFACT_TARGETS="container-artifacts pages-artifact" python3 -m unittest discover -s tools/gitrelease/tests -p 'test_*.py'` (36 tests), `timeout -k 120s -s SIGKILL 120s python3 -m unittest tools.gitrelease.tests.test_release_pipeline.ReleasePipelineTest.test_prepare_runs_ci_without_release_artifact_environment`, `timeout -k 30s -s SIGKILL 30s bash -n tools/gitrelease/scripts/prepare_release.sh`, and final `timeout -k 350s -s SIGKILL 350s make ci` (Go aggregate coverage 100.0%, Python 20 passed, Playwright 12 passed, release integrations 36 passed, live-provider preflight passed).

- [x] [B026] (P1) Retry management profile after MPR UI refreshes authentication.
  ### Summary
  The live Pages UI can show the shared MPR user avatar for `temirov@gmail.com` while the llm-proxy body remains on "Sign in to manage llm-proxy keys". The MPR UI bundle can refresh TAuth through `/auth/session` after llm-proxy's initial `/api/management/profile` call has already received `401`, leaving the app signed out even though the shared `<mpr-user>` component is authenticated.
  ### Acceptance Criteria
  1. The management app retries profile loading when the shared MPR shell emits `mpr-ui:auth:authenticated` after an initial profile `401`.
  2. The retry uses the current TAuth-authenticated shell state and does not read browser cookies, local storage, or session values directly.
  3. Truly unauthenticated users still see the sign-in panel.
  4. Browser coverage proves a delayed MPR authentication event transitions the app from the sign-in panel to the usage dashboard.
  ### Resolution
  Restored the canonical declarative MPR UI integration: source HTML owns `mpr-header[data-config-url="/config-ui.yaml"]` and the pinned bundle marker, while Pages rendering replaces that URL with the profile-owned production `PAGES_CONFIG_URL`. Removed application-owned MPR config application, bundle injection, production-host inference, and `<mpr-user>` internal status observation. The management app now queues one profile reload only from the documented `mpr-ui:auth:authenticated` event and waits for `MPRUI.whenAutoOrchestrationReady()` before releasing the shared auth transition. Static, renderer, and Playwright coverage reject direct `tauth.js`, missing declarative markers, internal MPR UI probing, invalid Pages config URLs, and the original profile-`401` followed by authenticated-event race. Validation passed with the required baseline and final `make ci` runs, including 100% Go block coverage, 20 Python tests, 13 Playwright tests, 36 release-contract tests, and the live-provider preflight; `git diff --check` also passed.

- [x] [B027] (P1) Make TAuth session validation and deployment one canonical contract.
  ### Summary
  Production can show an authenticated MPR user while `/api/management/profile` returns `401`. The prior race fix left a hand-written llm-proxy JWT parser in place and the gateway `llm-proxy` deployment target restarted llm-proxy without restarting TAuth or staging TAuth's runtime inputs, allowing validator and issuer runtime state to diverge.
  ### Acceptance Criteria
  1. The backend consumes TAuth's published `pkg/sessionvalidator`; no llm-proxy-owned JWT parser or duplicate claims schema remains.
  2. llm-proxy retains only product-owned tenant, required-expiry, and principal invariants after TAuth validation.
  3. Management-session rejection logs expose only stable categories and never cookies, tokens, or identity claims.
  4. Valid management API coverage signs the session with TAuth's published claims type, while invalid-cookie coverage preserves expiry, issuer, tenant, issued-at, principal, and missing-cookie rejection behavior.
  5. The gateway `llm-proxy` target stages TAuth env/config, restarts `tauth-api` and llm-proxy together, and verifies both public health checks before Pages activation.
  6. Repository documentation names the shared validator and coupled deployment contract.
  7. The app deployment accepts only the canonical gateway target and fails before release or remote operations unless a clean synchronized gateway `origin/master` passes a gateway-owned coupling check.
  ### Resolution
  Replaced llm-proxy's duplicate JWT parser and claims schema with TAuth `v1.1.8` `pkg/sessionvalidator`, retaining only the product-owned tenant, required-expiry, and principal checks. The validated runtime configuration now owns the constructed validator and returns constructor failures through startup instead of panicking; public startup coverage replaces the former direct panic test. Management middleware logs stable rejection categories without cookies, tokens, or identity claims, and black-box management API fixtures use TAuth's published claims type while preserving invalid-session coverage. Aligned the module, CI, and container builder with TAuth's Go 1.25.4 requirement. The companion gateway change makes the `llm-proxy` target stage TAuth env/config and Caddy inputs, restart `tauth-api` with llm-proxy, require both health checks, and expose `verify-llm-proxy-deployment-contract`. The app deploy path has one fixed gateway target and rejects non-git, dirty, non-`master`, unsynchronized, or contract-incomplete gateway checkouts before release or remote operations, so the companion gateway commit must land before deployment can proceed. Documentation records the shared validator and coupled rollout invariant. Validation passed with 100% aggregate Go coverage, 38 release-contract tests, the focused gateway-owned contract target, and the repository's remaining CI gates; no production deployment was executed.

- [x] [B028] (P1) Present a direct LLM Proxy sign-in experience.
  Goal:
  Make signing in an action that completes authentication and opens the management dashboard without explaining authentication mechanics to the user.
  Requirements:
  - Keep TAuth's session cookie and the documented `mpr-ui:auth:*` lifecycle as the only authentication source of truth.
  - Present one direct signed-out prompt with no special explanation of Google, avatars, cookies, or session state.
  - Use the existing canonical header sign-in action; successful authentication must transition directly to the dashboard without another auth controller or compatibility path.
  Deliverables:
  - Update the signed-out management panel copy.
  - Add Playwright coverage for the direct signed-out state at the compact viewport shown in the report and the authenticated dashboard transition.
  Validation:
  - Run focused Playwright browser coverage.
  - Run the required post-change `timeout -k 350s -s SIGKILL 350s make ci`.
  Resolution:
  The signed-out panel now presents only "Sign in to manage LLM Proxy keys" with no explanation of Google, avatars, cookies, or session mechanics. The existing mpr-ui header action remains the sole sign-in controller, and its authenticated lifecycle event replaces the signed-out state directly with the usage dashboard. Playwright coverage asserts the compact panel has no explanatory paragraph and that authentication opens the dashboard; the real local TAuth and LLM Proxy black-box scenario proves a TAuth-issued HttpOnly session unlocks the same dashboard. Focused `timeout -k 120s -s SIGKILL 120s make frontend-test` passed all 14 browser scenarios, focused `timeout -k 120s -s SIGKILL 120s make test-management-auth-blackbox` passed, and the required baseline/final `timeout -k 350s -s SIGKILL 350s make ci` runs passed. Final CI included 100% aggregate Go coverage, 20 Python tests, 14 fast Playwright tests, the real-stack Playwright test, 38 release-contract tests, and the live-provider preflight.

- [x] [B029] (P1) Exercise the real local TAuth and LLM Proxy session boundary in browser tests.
  Goal:
  Replace the missing cross-service authentication proof with a local black-box test that starts both services and drives the real public contracts.
  Requirements:
  - Start the TAuth version pinned by `go.mod` and the LLM Proxy binary built from the current checkout.
  - Use one explicit local profile for frontend origin, TAuth origin, management API origin, tenant, issuer, signing key, and session cookie name.
  - Obtain the session through TAuth's real password-login endpoint; do not mint a test-only JWT or mock TAuth and `/api/management/*`.
  - Prove the protected management API rejects the anonymous browser, accepts the TAuth-issued cookie, and the real static app plus pinned mpr-ui release renders the authenticated dashboard.
  - Keep third-party static assets local and deterministic during the browser run.
  Deliverables:
  - Add a repository-owned local-stack harness and Playwright black-box spec.
  - Add a dedicated Makefile target and include it in the canonical test/CI path.
  - Document the local black-box command and covered service boundary.
  Validation:
  - Run the focused local black-box target.
  - Run the required post-change `timeout -k 350s -s SIGKILL 350s make ci` after the final edit.
  Resolution:
  Added a disposable local-stack harness that derives TAuth's exact pinned version from `go.mod`, builds that TAuth server plus the current llm-proxy CLI, writes aligned localhost tenant/session configs, starts both services on disposable ports, and serves the real static management app. The Playwright spec obtains HttpOnly access and refresh cookies through TAuth's seeded `/auth/password/login`, proves anonymous `/api/management/profile` returns `401`, proves both TAuth `/auth/session` and the real LLM Proxy profile accept the issued session, then verifies the pinned `mpr-ui` user control and LLM Proxy dashboard render authenticated. Alpine, js-yaml, and the exact mpr-ui v3.11.1 release commit are pinned local test assets; TAuth and management API requests are not mocked. Added `make test-management-auth-blackbox`, included it in `make test` / `make ci`, expanded frontend syntax checks and CI path filters, and documented the command. Focused `make test-management-auth-blackbox` passed, and the required baseline/final `make ci` runs passed; final CI included 100% aggregate Go coverage, 20 Python tests, 14 fast Playwright tests, the new real-stack Playwright test, 38 release-contract tests, and the live-provider preflight.

- [x] [B030] (P1) Keep the authenticated session until explicit sign-out.
  Goal:
  Make authentication seamless: a successful sign-in opens the dashboard, page refresh restores the user without showing a signed-out detour, and only the explicit Sign out action clears the TAuth session.
  Requirements:
  - Keep TAuth's profile-specific HttpOnly access and refresh cookies as the only persisted session contract.
  - Restore a valid access session on ordinary reload and recover an expired access cookie from the still-valid refresh cookie through TAuth's browser session endpoint.
  - Do not clear cookies or present the signed-out state during successful startup recovery.
  - Explicit Sign out must call TAuth logout, clear both cookies, and return the management API and UI to the anonymous state.
  - Keep one forward-only `/config-ui.yaml` plus mpr-ui auth path without an application-owned session store or compatibility controller.
  Deliverables:
  - Pin a validated mpr-ui release with the current TAuth session-restore contract.
  - Add real local-stack Playwright coverage for sign-in, reload, refresh-cookie recovery, and explicit sign-out.
  - Document the session persistence behavior.
  Validation:
  - Run focused frontend and real-stack browser tests.
  - Run the required post-change `timeout -k 350s -s SIGKILL 350s make ci`.
  Resolution:
  Pinned MPR UI v3.11.1 and its exact release commit so the shared shell restores sessions through TAuth `/auth/session`. LLM Proxy now treats the documented final `mpr-ui:auth:status-change` as the anonymous boundary: an early management-profile `401` stays in the loading state while TAuth rotates a valid refresh cookie, and the authenticated event retries the profile without rendering the signed-out panel. The real local-stack Playwright scenario obtains TAuth access and refresh cookies, proves an ordinary reload remains authenticated, removes only the access cookie and proves silent refresh recovery, then clicks the visible **Sign out** action and proves `/auth/logout` clears both cookies, `/auth/session` returns `204`, and `/api/management/profile` returns `401`. Updated the local refresh profile to 720 hours, regenerated all 45 stage-owned resource pages with the current MPR UI CSS pin, and documented that normal reload and access-cookie expiration never invoke logout. Focused `timeout -k 120s -s SIGKILL 120s make frontend-test` passed all 14 browser scenarios, focused `timeout -k 120s -s SIGKILL 120s make test-management-auth-blackbox` passed the real service/browser scenario, and the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci` runs passed. Final CI included 100% aggregate Go coverage, 20 Python tests, 14 fast Playwright tests, the real-stack session-persistence test, 38 release-contract tests, and the live-provider preflight.

- [x] [B031] (P2) Drive real-stack sign-in through the browser lifecycle.
  Goal:
  Make the real-stack sign-in-to-reload scenario fail when browser login CORS, the mounted header's authenticated event, or MPR UI restore-hint persistence regresses.
  Requirements:
  - Submit TAuth password login from the loaded management page as a credentialed cross-origin browser request.
  - Drive the mounted header through the documented `MPRUI.testing.authenticate` adapter so the normal `mpr-ui:auth:*` lifecycle owns the dashboard transition and restore hint.
  - Do not seed MPR UI private local-storage keys or use `APIRequestContext` as the sign-in path.
  - Keep ordinary reload, refresh-cookie recovery, and explicit sign-out assertions on the real local stack.
  Deliverables:
  - Correct the real-stack Playwright scenario and its boundary documentation.
  Validation:
  - Run focused `timeout -k 120s -s SIGKILL 120s make test-management-auth-blackbox`.
  - Run the required final `timeout -k 350s -s SIGKILL 350s make ci` after the final code edit.
  Resolution:
  The real-stack Playwright scenario now loads the anonymous management page, submits TAuth password login with a credentialed cross-origin browser `fetch`, and asserts the credentialed CORS response before accepting the issued HttpOnly cookies. It passes the returned profile through the documented `MPRUI.testing.authenticate` adapter, observes the resulting browser management-profile request and authenticated dashboard transition, and relies on the adapter-owned restore hint for ordinary reload and refresh-cookie recovery; the test no longer writes `tauth.restore.v1` or posts login through `APIRequestContext`. Focused `timeout -k 120s -s SIGKILL 120s make test-management-auth-blackbox` passed, and the required baseline/final `timeout -k 350s -s SIGKILL 350s make ci` runs passed. Final CI included 100% aggregate Go coverage, 20 Python tests, 14 frontend Playwright scenarios, the corrected real-stack auth scenario, 38 release-contract tests, and the live-provider preflight.

- [x] [B032] (P1) Preserve sign-in button contrast on hover.
  Goal:
  Keep the shared MPR header sign-in action clearly legible while hovered.
  Requirements:
  - Keep the pinned MPR UI component responsible for its own sign-in control appearance.
  - Scope LLM Proxy form-control styles to application-owned controls so they cannot override shared shell hover states.
  - Preserve the existing LLM Proxy management button appearance and behavior.
  Deliverables:
  - Correct the static-site CSS ownership boundary for application buttons.
  - Add real-stack Playwright coverage for accessible sign-in text contrast in the hovered state.
  Validation:
  - Run the focused real-stack browser target.
  - Run the required final `timeout -k 350s -s SIGKILL 350s make ci` after the final code edit.
  Resolution:
  Scoped LLM Proxy font, field, label, and button rules to the application-owned `<llm-proxy-key-management>` surface, preventing the local button hover selector from overriding the pinned MPR header's primary sign-in hover palette while preserving management controls. The real local TAuth and LLM Proxy Playwright scenario now hovers the rendered shared sign-in action and requires at least 4.5:1 foreground/background contrast before completing the existing sign-in, reload, refresh-cookie recovery, and explicit sign-out flow. Focused `timeout -k 120s -s SIGKILL 120s make test-management-auth-blackbox` passed, and the required baseline/final `timeout -k 350s -s SIGKILL 350s make ci` runs passed. Final CI included 100% aggregate Go coverage, 20 Python tests, 14 frontend Playwright scenarios, the real-stack browser scenario, 38 release-contract tests, and the live-provider preflight.

- [x] [B033] (P0) Let the verified legacy-token owner reach the dashboard after earlier sign-in created an empty account.
  Goal:
  Make a successful TAuth login open the LLM Proxy dashboard instead of leaving the body signed out or permanently loading when the bounded legacy-token migration finds an account created by an earlier sign-in.
  Requirements:
  - Keep the configured owner email and verified TAuth subject as the sole authority for the one-off claim.
  - Atomically remove only an empty destination account with no generated secret, provider settings, or usage before rekeying the legacy tenant to that same verified subject.
  - Preserve the legacy tenant id, token digest, defaults, provider settings, creation timestamp, and usage history.
  - Continue returning a hard conflict for any destination account that owns a generated secret, provider settings, or usage; do not merge competing account state.
  - Present authenticated profile failures explicitly instead of disguising them as an unauthenticated prompt or an endless loading state.
  Deliverables:
  - Add black-box management HTTP coverage for the empty-destination recovery and non-empty-destination conflict.
  - Add browser coverage for authenticated profile failure state and successful dashboard transition.
  - Update the current migration documentation and stage-owned generated resources.
  Validation:
  - Run focused management and frontend browser targets.
  - Run the required final `timeout -k 350s -s SIGKILL 350s make ci` after the final code edit.
  Resolution:
  The one-off legacy claim now removes only a verified owner's empty destination row inside the same GORM transaction before rekeying the legacy tenant; generated secrets, provider records, and usage remain hard conflicts with no merge. Management profile failures now enter an explicit workspace-error state on both authentication retry and full-page reload instead of preserving the signed-out or loading panel. Added black-box management HTTP recovery/conflict scenarios, transaction rollback/count coverage, and browser assertions for dashboard success plus persistent profile failure. Regenerated the stage-owned migration resource and updated the current README and implementation contract. Focused `make go-test`, `make frontend-test`, and `make test-management-auth-blackbox` passed. The required baseline and final `timeout -k 350s -s SIGKILL 350s make ci` runs passed; final CI included 100.0% aggregate Go coverage, 20 Python tests, 15 frontend Playwright scenarios, the real local TAuth/LLM Proxy browser scenario, 38 release-contract tests, and the live-provider preflight.

- [x] [B034] (P0) Hydrate the dashboard only from the canonical MPR UI authentication lifecycle.
  Goal:
  Make every successful MPR UI/TAuth login open the LLM Proxy dashboard without an application-owned authentication race, retry protocol, or auth-state inference from the management API.
  Evidence:
  - Production release `v0.2.31` mounts the authenticated MPR user avatar while LLM Proxy independently probes `/api/management/profile` during startup and can leave the body signed out or loading.
  - The application currently starts its protected profile request before the pinned MPR UI v3.11.1 shell has declared `authenticated`, then uses bespoke flags to retry after a potentially missed event.
  - MPR UI v3.11.1 already owns TAuth session restoration and exposes its current lifecycle through the documented `data-mpr-auth-status` attribute plus `mpr-ui:auth:*` events.
  Requirements:
  - Treat MPR UI as the sole browser authentication authority; do not read TAuth cookies, storage, tokens, claims, or authentication endpoints from application code.
  - Do not request protected LLM Proxy workspace data until MPR UI reports `authenticated` through its documented lifecycle.
  - Register lifecycle listeners before application startup and reconcile an authenticated event that occurred before Alpine initialized by reading the documented `data-mpr-auth-status` value from the mounted header.
  - Treat `mpr-ui:auth:unauthenticated` and the initial final unauthenticated status as the only signed-out boundaries.
  - After MPR UI reports authenticated, load the workspace exactly once; any management API failure is an explicit workspace error and must not downgrade MPR UI authentication.
  - Delete application-owned authentication settlement and retry flags; introduce no alternate authentication path, fallback, compatibility shim, or private MPR UI inspection.
  Deliverables:
  - Replace startup profile probing and authenticated retry logic with canonical lifecycle-driven workspace hydration.
  - Add browser coverage proving zero protected management requests before MPR UI authentication, immediate hydration after same-page login, and recovery when authentication completes before Alpine initializes.
  - Keep real local TAuth cookies, documented MPR UI lifecycle adapters, reload restoration, access-cookie refresh, and explicit sign-out in the black-box scenario.
  Validation:
  - Run focused frontend and real-stack browser targets.
  - Run the required final `timeout -k 350s -s SIGKILL 350s make ci` after the final code edit.
  Resolution:
  MPR UI v3.11.1 is now the sole browser authentication authority. LLM Proxy registers the documented authenticated and unauthenticated lifecycle events before startup, waits for MPR UI orchestration, reconciles an already-settled lifecycle through the documented header `data-mpr-auth-status`, and makes no protected management request until that state is `authenticated`. Removed the application-owned authentication-settlement and queued-retry flags. Once MPR UI authenticates the user, the workspace loads exactly once; every management profile failure, including `401` and the legacy-claim `409`, renders an explicit workspace error without changing the MPR UI session to signed out. The fast browser suite proves zero pre-auth profile calls, one post-auth hydration call, already-settled authenticated startup, dashboard rendering, and explicit failure rendering. The real local-stack scenario proves the same request boundary with actual TAuth access/refresh cookies and the documented `MPRUI.testing.authenticate` lifecycle, then proves reload restoration, access-cookie refresh, and explicit sign-out. Updated README and implementation documentation to state the sole-authority boundary. Focused `make frontend-test` passed 16 scenarios and `make test-management-auth-blackbox` passed the real service/browser scenario. The required baseline and final `timeout -k 350s -s SIGKILL 350s make ci` runs passed; final CI included 100.0% aggregate Go coverage, 20 Python tests, 16 frontend Playwright scenarios, the real local TAuth/LLM Proxy browser scenario, 38 release-contract tests, and the live-provider preflight.

- [ ] [B035] (P2) Move workspace notifications above the footer.
  Goal:
  Render `Workspace loaded` and every other management notice in a dedicated top-right notification region directly below the shared MPR header. The current fixed bottom-right notice appears inside or over the footer and makes application feedback look like footer content.
  Requirements:
  - Remove the bottom-anchored notice placement; notifications must never render inside, overlap, or visually attach to `<mpr-footer>`.
  - Use one application-owned notification region aligned to the top-right below `<mpr-header>`, with spacing derived from the shared shell geometry or current design-system tokens rather than a viewport-specific magic offset.
  - Keep success, error, and informational notices in the same canonical region and preserve their existing visual kind distinctions and message behavior.
  - Keep notifications fully visible within the viewport and clear of the header at desktop and mobile widths. On narrow viewports, constrain or expand the notice within the available content width without horizontal clipping.
  - Preserve the B020 stacking contract: the Settings overlay remains above notifications, while notifications remain visible above the normal dashboard surface when Settings is closed.
  - The notification region must not intercept pointer input outside the visible notice or block the header's avatar/sign-in controls and primary dashboard actions.
  - Expose notice changes through an appropriate live-region/status semantic so placement changes do not reduce screen-reader feedback.
  Deliverables:
  - Update the management notice markup and static-site styles to own one top-right region below the shared header.
  - Update the existing Settings/header/footer layer coverage for the new notice geometry and stacking boundary.
  Validation:
  - Add Playwright coverage that triggers the `Workspace loaded` notice and representative success and error notices, then proves their bounding boxes remain below the header, above and separate from the footer, right-aligned, unclipped, and non-obstructive at desktop and mobile viewports.
  - Keep browser coverage proving the Settings modal overlays the notice and that repeated dashboard/settings actions do not move the notice into the footer region.
  - Run the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci` pair for the implementation, with the final run occurring after the last code edit.

- [ ] [B036] (P1) Keep routing default provider/model pairs valid.
  Goal:
  Ensure Settings never displays, saves, or executes a routing default whose model does not belong to its selected provider. The current workspace can render impossible combinations such as text provider `Anthropic` with model `gpt-4.1`, and dictation provider `Grok` with `No dictation model`.
  Requirements:
  - Treat each text provider/model pair and dictation provider/model pair as one invariant at the management persistence, profile API, update API, and runtime-routing boundaries.
  - Validate persisted defaults against the canonical configured provider catalogs before serving a workspace. A future invalid persisted pair must fail startup with tenant, endpoint, provider, and model context; do not silently select another provider or model.
  - Add a bounded, versioned one-off data migration for rows created under the broken contract: when a persisted provider remains valid but its model is blank or belongs to another provider, replace the model with that provider's configured default model. Reject an unknown or unsupported persisted provider instead of guessing. Remove the migration bridge after the stored data is upgraded; introduce no permanent fallback, dual-read, or compatibility path.
  - Keep management default updates atomic and reject a provider/model mismatch with a specific client error before writing either field.
  - On provider selection, immediately select that provider's configured default model and submit the resulting pair together so a stale model cannot survive the change.
  - Remove the `No dictation model` choice while a concrete dictation provider is required, preserving the resolved B011 contract that blank dictation defaults are not a supported persisted state.
  - Treat an invalid management-profile response as an explicit workspace integrity error at the browser boundary; do not render an impossible selection or repair server data in the UI.
  Deliverables:
  - Enforce the pair invariant in management storage/profile hydration, default updates, and runtime configuration.
  - Update the Settings controls so their selected model always comes from the selected provider's catalog.
  - Add the one-off persisted-data migration and document the canonical routing-default invariant in the management API and configuration documentation.
  Validation:
  - Add black-box management API coverage proving valid pairs round-trip, mismatched or blank models are rejected without partial writes, and profile responses contain only catalog-valid pairs.
  - Add migration/startup coverage proving repair of the known valid-provider mismatch shape and contextual rejection of unknown or unsupported providers after the migration boundary.
  - Add runtime coverage proving an omitted request model routes through the exact persisted provider/model pair.
  - Add Playwright coverage for initial Settings hydration, changing text and dictation providers, saving and reloading the resulting pairs, absence of the blank dictation-model option, and explicit failure for a malformed profile response.
  - Run the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci` pair for the implementation, with the final run occurring after the last code edit.

- [x] [B037] (P1) Declare the app-owned orchestration manifest completely.
  Goal:
  Make llm-proxy's repository the canonical owner of the declarative inputs that the gateway aggregates, while keeping concrete TAuth and service runtime values in the ignored local deployment environment.
  Requirements:
  - Declare the `llm-proxy` TAuth tenant in `.mprlab/deploy/resources.yml` with environment placeholders only; do not add tenant configuration to the independent vanilla TAuth repository or commit concrete credentials.
  - Describe the container through the current resource schema: explicit port records, compose file and project identity, and the exact private environment and application-config assets staged by the gateway.
  - Keep `.mprlab/deploy/.env` ignored and pair it with the tracked `configs/.env.sample`; keep `configs/config.yml` as the tracked application-owned runtime template.
  - Pass the repository CI gate and the gateway aggregate verifier from a clean committed owner state.
  Validation:
  - Run `timeout -k 350s -s SIGKILL 350s make ci` after the final manifest edit.
  - Run the gateway `make verify-app-workflows` aggregate after committing the owner contract.
  Resolution:
  Declared the `llm-proxy` TAuth tenant in the application-owned manifest using deployment-environment placeholders only, and described the container with the canonical port list, compose file/project identity, ignored private environment asset, tracked environment example, and tracked runtime configuration. The independent TAuth repository remains vanilla and no concrete credential values were added. The post-change `make ci` gate passed with 100.0% aggregate Go coverage, 20 Python tests, 16 Playwright management scenarios, the real local TAuth browser scenario, 38 release-contract tests, and the live-provider harness preflight. The gateway aggregate verifier is run from the gateway after this owner contract is committed.

- [x] [B038] (P1) Keep the DashScope catalog valid for the default endpoint.
  ### Summary
  The checked-in DashScope base URL is `https://dashscope-intl.aliyuncs.com/compatible-mode/v1`, but the refreshed catalog advertises `qwen3.7-max` and `qwen3.7-plus`. Those selections require a regional or workspace-specific DashScope route and credentials, so default deployments list models that reject requests.
  ### Acceptance Criteria
  1. The default DashScope catalog exposes only models supported by its checked-in International endpoint.
  2. The authenticated management profile does not expose either Qwen 3.7 model.
  3. The README catalog mirrors the packaged configuration.
  4. No regional endpoint, credential fallback, alias, or transport change is introduced.
  5. The required final `timeout -k 350s -s SIGKILL 350s make ci` passes after the last code edit.
  ### Resolution
  Removed `qwen3.7-max` and `qwen3.7-plus` from the default DashScope catalog while retaining `qwen-plus` at the checked-in International endpoint. The authenticated management-profile contract now asserts both unsupported Qwen 3.7 IDs remain absent, and the existing public routing scenario continues to exercise Qwen Plus. README configuration and model-capability tables mirror the packaged catalog. Validation passed with `timeout -k 350s -s SIGKILL 350s make ci`.

## Improvements

- [x] [I024] (P1) Add Qwen 3.8 Token Plan and MiniMax M2.7 providers.
  Goal:
  Expose `qwen3.8-max-preview` through Qwen Cloud Token Plan and `MiniMax-M2.7` through MiniMax as distinct, selectable text providers.

  Requirements:
  - Add canonical `qwencloud` and `minimax` provider selectors. Do not alias Qwen Cloud to `qwen`, which remains the DashScope selector alias and uses a different credential and endpoint contract.
  - Configure Qwen Cloud with its Token Plan OpenAI-compatible endpoint and dedicated `${QWEN_CLOUD_TOKEN_PLAN_API_KEY}`; configure MiniMax with `https://api.minimax.io/v1` and `${MINIMAX_API_KEY}`.
  - Route Qwen Cloud through the existing compatible Chat Completions payload with `max_tokens`. Route MiniMax M2.7 through that same adapter with `max_completion_tokens` and enforce its documented 2048-token completion maximum at the request edge.
  - Expose text generation only. Do not add provider-specific thinking, tool, streaming, multimodal, or dictation controls outside the existing public proxy contract.
  - Keep `configs/config.yml`, README, provider-routing documentation, live-provider harness, and management/profile contracts synchronized.
  - Preserve existing tenant defaults and leave default/persisted-selection migration behavior in separately tracked B036.

  Deliverables:
  - Both models appear in the authenticated management provider catalog under their canonical selectors.
  - Public HTTP routing coverage proves the endpoint, credential boundary, model ID, and token-field mapping; MiniMax requests above 2048 tokens fail before an upstream call.
  - The live-provider harness recognizes the two explicit credential variables and provider selectors without making an upstream call during repository CI.

  Validation:
  - Run the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci` pair, with the final run after the last code edit.

  Resolution:
  - Added canonical `qwencloud` and `minimax` text providers, synchronized their catalogs and harness contracts, and verified their routing, credential isolation, and MiniMax output limit through the required passing CI pair.

- [x] [I023] (P1) Add GLM-5.2 to the existing BigModel/Zhipu catalog.
  Goal:
  Make the documented GLM-5.2 text model selectable through the existing Zhipu OpenAI-compatible Chat Completions transport.

  Requirements:
  - Add the exact `glm-5.2` model identifier and its documented 128K output limit to the Zhipu text catalog.
  - Retain the existing `https://open.bigmodel.cn/api/paas/v4` base URL because the current BigModel documentation uses that endpoint for GLM-5.2.
  - Do not add optional GLM-specific `thinking` or `reasoning_effort` request controls that the existing public proxy contract does not own.
  - Keep `configs/config.yml`, the README catalog, and provider-routing documentation synchronized.
  - Leave default/persisted-selection migration behavior in the separately tracked B036 scope.

  Deliverables:
  - `glm-5.2` appears in the authenticated management profile.
  - Public HTTP routing coverage proves the selected model reaches the Zhipu-compatible Chat Completions payload with `max_tokens` and rejects values above its configured output limit.

  Validation:
  - Run the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci` pair, with the final run after the last code edit.

  ### Resolution
  Added `glm-5.2` with its documented 131072-token output limit to the existing BigModel/Zhipu catalog without changing its `open.bigmodel.cn` Chat Completions endpoint. The proxy keeps optional `thinking` and `reasoning_effort` controls outside its public request contract. Authenticated management-profile coverage exposes the model, and public HTTP routing coverage verifies the payload and local rejection above the configured output limit. Validation passed with `timeout -k 350s -s SIGKILL 350s make ci`.

- [x] [I022] (P1) Correct the Moonshot catalog for the Kimi K3 launch.
  Goal:
  Expose Kimi's currently documented K3 and K2.7 Code model IDs through the existing Moonshot Chat Completions transport.

  Requirements:
  - Add `kimi-k3`, `kimi-k2.7-code`, and `kimi-k2.7-code-highspeed` exactly as documented by Kimi.
  - Keep the existing Moonshot OpenAI-compatible Chat Completions transport and its current `max_completion_tokens` mapping; do not invent support for K3-only request controls such as `reasoning_effort`.
  - Keep `configs/config.yml`, the README catalog, and provider-routing documentation synchronized.
  - Leave default/persisted-selection migration behavior in the separately tracked B036 scope.

  Deliverables:
  - Kimi K3 and both current K2.7 Code selections appear in the authenticated management profile.
  - Public HTTP routing coverage demonstrates the K3 request reaches the Moonshot-compatible Chat Completions payload with the current token-limit field.

  Validation:
  - Run the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci` pair, with the final run after the last code edit.

  ### Resolution
  Added `kimi-k3`, `kimi-k2.7-code`, and `kimi-k2.7-code-highspeed` to the Moonshot catalog and its README/documented routing contract. The existing Chat Completions transport sends Kimi's current `max_completion_tokens` field and omits K3's fixed sampling controls. Authenticated management-profile coverage exposes all three selections, and public HTTP routing coverage verifies the K3 payload. Validation passed with `timeout -k 350s -s SIGKILL 350s make ci`.

- [x] [I021] (P1) Refresh documented model catalogs for existing providers.
  Goal:
  Make recently released, text-generation models selectable through the existing provider adapters without introducing a new provider or transport contract.

  Requirements:
  - Add only model identifiers verified in the current first-party provider documentation and compatible with the provider's existing request profile.
  - Preserve provider defaults for this additive catalog refresh; provider/model persistence migration remains the separately tracked B036 work.
  - Keep `configs/config.yml`, the README catalog, and provider-routing documentation synchronized.
  - Do not add request controls or capability fallbacks that are not already owned by the existing provider transport.

  Deliverables:
  - Current model options for the supported OpenAI, Anthropic, Gemini, xAI, DashScope, Moonshot, SiliconFlow, and Zhipu provider paths where current upstream releases exist.
  - Public HTTP integration coverage for representative new catalog selections across the affected provider protocols.

  Validation:
  - Run the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci` pair, with the final run after the last code edit.

  ### Resolution
  Added current OpenAI GPT-5.6 aliases and variants; Anthropic Claude Fable and Sonnet 5; Gemini 3.1 Pro and 3 Flash previews; DashScope Qwen 3.7 Max and Plus; Moonshot Kimi K2.6; and xAI Grok 4.5 and Grok 4.20 reasoning/non-reasoning selections. The Moonshot chat transport now maps the proxy `max_tokens` setting to Kimi's current `max_completion_tokens` request field. The README and provider-routing plan mirror the catalog contract. Authenticated management-profile and public HTTP routing coverage verify the new entries are visible and route through their intended provider protocols. Validation passed with `timeout -k 350s -s SIGKILL 350s make ci`.

- [x] [I020] (P1) Declare LLM Proxy's TAuth tenant requirements in the app-owned deployment manifest.
  ### Summary
  Move stable tenant identity, production origin, cookie names, and provider references into .mprlab/deploy/resources.yml while keeping credential values, shared TTLs, and server policy gateway-owned.
  ### Acceptance Criteria
  1. The gateway discovers the declaration without reading a TAuth-owned deployment registry.
  2. The assembled fleet configuration passes generic TAuth doctor validation.
  ### Resolution
  Added the canonical app-owned `tauth_tenant` declaration. The gateway assembled the 13-tenant fleet, the generic TAuth doctor accepted it with zero errors, and `make ci` passed.
- [x] [I001] (P1) Make missing placeholder handling field-aware.
  ### Summary
  Review found that allowing missing `${...}` placeholders globally can silently mutate non-key configuration values. Keep the new disabled-provider behavior for missing provider API-key placeholders, but fail startup for missing placeholders everywhere else and for partial API-key placeholders.
  ### Acceptance Criteria
  1. Missing placeholders outside provider `api_key` fields fail startup with `config_placeholder_missing`.
  2. A provider `api_key` value that is exactly a missing `${...}` placeholder expands to an empty key so a non-default provider can be disabled.
  3. Missing placeholders embedded inside longer provider `api_key` values fail startup rather than creating malformed credentials.
  4. README and provider-routing docs describe the field-aware placeholder behavior.
  ### Resolution
  Config placeholder expansion now allows a missing placeholder only when it is the whole provider `api_key` value; missing placeholders in tenant secrets, URLs, and partial API-key strings fail with `config_placeholder_missing`. Added black-box CLI coverage for missing non-key placeholders, exact optional provider API-key placeholders, partial API-key placeholders, and missing default-provider credentials. README and provider-routing docs now describe the field-aware placeholder rule. Validation passed with `timeout -k 180s -s SIGKILL 180s go test -count=1 ./cmd/cli -run 'TestRootCommand(RejectsMissingTenantSecretPlaceholder|AllowsMissingNonDefaultProviderKey|RejectsPartialMissingProviderKeyPlaceholder|RejectsMissingDefaultDictationProviderKey|RejectsMissingDefaultTextProviderKeys)'` and `timeout -k 350s -s SIGKILL 350s make ci`.
- [x] [I002] (P1) Require API keys only for tenant default providers.
  ### Summary
  Provider configuration should treat supported providers symmetrically: every provider keeps explicit URLs and model catalogs, but API credentials are only startup-required when a tenant uses that provider as a default text or dictation provider. Non-default providers with missing credentials should remain selectable by name but return a clear `provider not configured` response to that tenant instead of preventing service startup.
  ### Acceptance Criteria
  1. Static config loading no longer fails solely because a non-default provider API-key placeholder is missing or expands to blank.
  2. Static config loading fails startup when any tenant default text provider lacks its provider API key.
  3. Static config loading fails startup when any tenant default dictation provider lacks its provider API key.
  4. Requests that explicitly select a configured non-default provider without an API key return `503 provider not configured`.
  5. README and provider-routing docs describe provider keys as optional for non-default providers and required for tenant defaults.
  ### Resolution
  Static config placeholder expansion now turns missing `${...}` values into empty strings, allowing non-default provider API keys to be omitted without blocking startup. Config validation now requires API keys only for tenant default text providers and tenant default dictation providers that support dictation; unknown or unsupported defaults continue through the runtime provider/model validation path. Explicit requests for configured non-default providers without API keys return `503 provider not configured` with provider and endpoint detail. README and provider-routing docs now document optional non-default provider keys, disabled-provider request behavior, and startup-fatal default provider credentials. Validation passed with `timeout -k 180s -s SIGKILL 180s go test -count=1 ./cmd/cli`, `timeout -k 240s -s SIGKILL 240s go test -count=1 ./internal/proxy`, `timeout -k 240s -s SIGKILL 240s go test -count=1 ./tests/...`, `timeout -k 30s -s SIGKILL 30s git diff --check`, and `timeout -k 350s -s SIGKILL 350s make ci` (Go total coverage 100.0%, Python 26 passed).
- [x] [I003] (P1) Address provider config review followups.
  ### Summary
  Review found three gaps in the explicit provider config branch: non-OpenAI model catalogs can advertise `web_search` even though only OpenAI consumes it, programmatic runtime configuration still silently falls back to a hardcoded model catalog, and live provider smoke tests duplicate default model ids instead of exercising the configured provider defaults.
  ### Acceptance Criteria
  1. Static config validation rejects `web_search: true` for any non-OpenAI text model catalog entry and for dictation catalogs.
  2. Runtime `proxy.Configuration` no longer injects a hardcoded provider model catalog when `ProviderModels` is omitted.
  3. Tests that build routers/configuration programmatically pass explicit provider model catalogs through test fixtures rather than relying on runtime fallback.
  4. `make test-live-providers` omits the `model` field by default and sends a model only when a per-provider override is set.
  5. README documents that live smoke defaults come from `configs/config.yml` and override variables are optional.
  ### Resolution
  Static provider model catalog validation now rejects `web_search: true` outside OpenAI text model entries, and the CLI config test matrix covers non-OpenAI text and dictation catalog failures. Runtime configuration no longer injects a hardcoded model catalog fallback; programmatic tests load explicit provider model catalogs from `configs/config.yml` through test fixtures, preserving custom catalog tests. The live provider smoke runner now omits `model` by default so configured provider defaults are exercised, and only sends `model` when a per-provider override variable is set. Because the live run that omitted `model` exposed Gemini `gemini-3.5-flash` returning provider 503 while `gemini-2.5-flash` passed, the Gemini default in `configs/config.yml`, README, and representative CLI test fixtures was changed to `gemini-2.5-flash`. Validation passed with `timeout -k 120s -s SIGKILL 120s go test -count=1 ./cmd/cli -run TestRootCommandRejectsIncompleteStaticProviderConfig`, `timeout -k 180s -s SIGKILL 180s go test -count=1 ./internal/proxy`, `timeout -k 240s -s SIGKILL 240s go test -count=1 ./tests/...`, `bash -n scripts/test_live_providers.sh scripts/test_live_gemini.sh`, `scripts/test_live_providers.sh --help`, no-key skip via `env -i PATH="$PATH" HOME="$HOME" TMPDIR="${TMPDIR:-/tmp}" GOENV=off scripts/test_live_providers.sh`, explicit missing-key failure via `env -i PATH="$PATH" HOME="$HOME" TMPDIR="${TMPDIR:-/tmp}" LLM_PROXY_LIVE_PROVIDERS=openai GOENV=off scripts/test_live_providers.sh`, targeted Gemini override proof via `timeout -k 180s -s SIGKILL 180s env LLM_PROXY_LIVE_PROVIDERS=gemini LLM_PROXY_LIVE_GEMINI_MODEL=gemini-2.5-flash make test-live-providers LIVE_ENV_FILE=configs/.env`, dynamic live smoke via `timeout -k 180s -s SIGKILL 180s make test-live-providers LIVE_ENV_FILE=configs/.env` (OpenAI and Gemini passed with provider defaults), `timeout -k 350s -s SIGKILL 350s make ci`, and `timeout -k 30s -s SIGKILL 30s git diff --check`.
- [x] [I004] (P1) Add dynamic live provider smoke tests.
  ### Summary
  The repository has a stale Gemini-only live smoke target that now fails against the complete static config contract, and it does not exercise OpenAI even when `OPENAI_API_KEY` is present. Add a provider-aware live smoke target that builds a complete temporary config, runs text smoke tests for providers with available API keys, and keeps targeted runs explicit for debugging.
  ### Acceptance Criteria
  1. A canonical Makefile target runs live text smoke tests for every supported text provider whose API key is present in the environment or loaded `LIVE_ENV_FILE`.
  2. Missing provider keys do not fail the dynamic target unless the provider is explicitly requested.
  3. The temporary live config satisfies the complete static config contract without requiring real keys for providers that are not being tested.
  4. OpenAI is covered when `OPENAI_API_KEY` is present.
  5. The old Gemini live target still works as a Gemini-focused wrapper.
  6. README documents dynamic provider discovery, targeted provider selection, per-provider model overrides, and that live tests are not part of normal CI.
  ### Resolution
  Added `scripts/test_live_providers.sh` and `make test-live-providers` as the canonical live text smoke path. The runner sources optional `LIVE_ENV_FILE`, discovers provider keys, fills unused provider placeholders with dummy values so the complete static config contract is satisfied, builds a temporary binary/config, and calls only providers with real keys unless `LLM_PROXY_LIVE_PROVIDERS` explicitly selects a provider. OpenAI now runs when `OPENAI_API_KEY` is present. `scripts/test_live_gemini.sh` remains as a Gemini-focused compatibility wrapper and maps the old `LLM_PROXY_LIVE_MODEL` override to `LLM_PROXY_LIVE_GEMINI_MODEL`. README now documents dynamic discovery, targeted provider selection, model overrides, and that live tests are not part of normal CI. Validation passed with `scripts/test_live_providers.sh --help`, no-key skip via `env -i PATH="$PATH" HOME="$HOME" TMPDIR="${TMPDIR:-/tmp}" GOENV=off scripts/test_live_providers.sh`, explicit missing-key failure via `env -i PATH="$PATH" HOME="$HOME" TMPDIR="${TMPDIR:-/tmp}" LLM_PROXY_LIVE_PROVIDERS=openai GOENV=off scripts/test_live_providers.sh`, `timeout -k 180s -s SIGKILL 180s make test-live-providers LIVE_ENV_FILE=configs/.env` (live OpenAI and Gemini passed), `timeout -k 180s -s SIGKILL 180s make test-live-gemini LIVE_ENV_FILE=configs/.env` (live Gemini passed), `bash -n scripts/test_live_providers.sh scripts/test_live_gemini.sh`, `timeout -k 30s -s SIGKILL 30s git diff --check`, and `timeout -k 350s -s SIGKILL 350s make ci`.
- [x] [I005] (P1) Move provider model catalogs into config.yml.
  ### Summary
  Provider model ids, provider default models, dictation model ids, web-search support, request payload profiles, and provider-side output-token limits currently live in Go constants and provider registry tables. Move those changing model catalogs into `config.yml` so runtime model support changes are made through the authoritative YAML config, while code retains only stable provider transports and known request-profile implementations.
  ### Acceptance Criteria
  1. `configs/config.yml` declares each provider's text default model and allowed text models.
  2. Dictation-capable providers declare their dictation default model and allowed dictation models in `configs/config.yml`.
  3. Runtime static config loading rejects missing provider model catalogs, blank model ids, duplicate model ids, and defaults not present in the configured model list.
  4. Provider routing validates and routes text/dictation requests using configured model catalogs rather than hardcoded per-provider model sets.
  5. OpenAI model request-shape behavior is selected from configured stable request profiles, and model-specific web-search support/output-token limits come from model metadata.
  6. README and provider-routing docs document the model catalog schema and distinguish configurable model data from code-owned provider transports.
  7. Black-box tests prove adding a provider model in config routes upstream without code changes and invalid configured model catalogs fail startup.
  ### Resolution
  `configs/config.yml` now declares text catalogs for every supported provider and dictation catalogs for OpenAI, SiliconFlow, Zhipu, and Grok/xAI. Runtime configuration carries those catalogs through `ProviderModels`, validates missing/blank/duplicate/default-missing catalog data at startup, and builds provider routing from configured text and dictation models. OpenAI request payload shape now comes from configured stable `request_profile` values, while model-specific `web_search` support and output-token limits come from model metadata. README and `docs/implementation/provider-routing-plan.md` document the model catalog schema, default support matrix, dictation catalogs, and the split between config-owned model data and code-owned provider transports. Black-box CLI/config tests cover invalid catalog startup failures, and router/integration tests prove a configured model routes upstream without code changes and unsupported model-specific web search fails at the request edge. Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 60s -s SIGKILL 60s go test -count=1 ./cmd/cli -run TestRootCommandRejectsIncompleteStaticProviderConfig`, `timeout -k 60s -s SIGKILL 60s go test -count=1 ./internal/proxy -run 'TestProviderRouting(UsesConfiguredTextModelCatalog|RejectsMissingConfiguredProviderCatalog)'`, `timeout -k 180s -s SIGKILL 180s go test -count=1 ./tests/integration`, `timeout -k 350s -s SIGKILL 350s make test` (Go total coverage 100.0%, Python 26 passed), `timeout -k 350s -s SIGKILL 350s make lint`, `timeout -k 350s -s SIGKILL 350s make ci`, and `timeout -k 30s -s SIGKILL 30s git diff --check`.
- [x] [I006] (P1) Add Grok/xAI and Zhipu dictation support.
  ### Summary
  Upstream xAI and Z.AI currently expose speech-to-text APIs, but the proxy only supports `/dictate` for OpenAI and SiliconFlow. Add explicit Grok/xAI and Zhipu transcription URL configuration and wire `/dictate` to xAI STT and Zhipu GLM-ASR, while leaving text-only providers without dictation config fields.
  ### Acceptance Criteria
  1. `configs/config.yml` includes `providers.zhipu.transcriptions_url` and `providers.grok.transcriptions_url`.
  2. Runtime config loading accepts and requires Grok and Zhipu transcription URLs.
  3. `/dictate?provider=zhipu` sends multipart audio to the configured Zhipu transcription URL with model `glm-asr-2512`.
  4. `/dictate?provider=grok` sends multipart audio to the configured xAI STT URL without a model form field.
  5. README and provider-routing docs distinguish proxy-supported dictation from upstream-only capabilities and list OpenAI, SiliconFlow, Zhipu, and Grok/xAI as proxy-supported dictation providers.
  6. Black-box tests cover Grok and Zhipu dictation routing plus required transcription URL config validation.
  ### Resolution
  `configs/config.yml` now includes explicit `providers.zhipu.transcriptions_url` and `providers.grok.transcriptions_url` entries, and static config validation requires those fields alongside the provider API keys and base URLs. Runtime provider routing now supports `/dictate` for Zhipu through Z.AI GLM-ASR with `model=glm-asr-2512`, and Grok/xAI through xAI STT without sending a multipart `model` field. README and `docs/implementation/provider-routing-plan.md` now distinguish proxy-supported dictation from upstream-only speech APIs and list OpenAI, SiliconFlow, Zhipu, and Grok/xAI as dictation-capable providers. Black-box CLI coverage validates required Zhipu and Grok transcription URLs, and black-box router coverage verifies the multipart request shape and credential routing for both providers. Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 120s -s SIGKILL 120s go test -count=1 ./cmd/cli -run 'TestRootCommand(RunsConfiguredProxyFromConfigFile|RunsProductionLoggerFromConfigFile|RejectsIncompleteStaticProviderConfig)'`, `timeout -k 120s -s SIGKILL 120s go test -count=1 ./internal/proxy -run 'TestProviderRouting(SupportsZhipuAndGrokDictation|RejectsInvalidDefaultDictationProvider|RejectsAnthropicAndGrokUnsupportedCapabilities)'`, `timeout -k 350s -s SIGKILL 350s make test` (Go total coverage 100.0%, Python 26 passed), `timeout -k 350s -s SIGKILL 350s make lint`, `timeout -k 350s -s SIGKILL 350s make ci`, and `timeout -k 30s -s SIGKILL 30s git diff --check`.
- [x] [I007] (P1) Make OpenAI dictation URL explicit in static provider config.
  ### Summary
  Only OpenAI and SiliconFlow currently support `/dictate`. SiliconFlow already exposes `providers.siliconflow.transcriptions_url`, while OpenAI dictation still derives `/audio/transcriptions` from `providers.openai.base_url`. Add an explicit `providers.openai.transcriptions_url` so every dictation-capable provider declares the actual dictation endpoint URL in `config.yml`, without adding unsupported dictation fields to text-only providers.
  ### Acceptance Criteria
  1. `configs/config.yml` includes `providers.openai.transcriptions_url: "https://api.openai.com/v1/audio/transcriptions"`.
  2. Runtime config loading accepts and requires `providers.openai.transcriptions_url`.
  3. OpenAI dictation uses the configured transcription URL when no test endpoint override is supplied.
  4. README and provider-routing docs state that only dictation-capable providers have transcription URL fields.
  5. Black-box tests cover OpenAI transcription URL loading, required config validation, and dictation routing.
  ### Resolution
  `providers.openai.transcriptions_url` is now part of the static OpenAI provider config and is required alongside `providers.openai.api_key` and `providers.openai.base_url`. Runtime configuration carries it as `OpenAITranscriptionsURL`, and the OpenAI endpoint bundle now derives Responses and Models from `providers.openai.base_url` while routing `/dictate` through the configured transcription URL. `configs/config.yml`, README, and `docs/implementation/provider-routing-plan.md` now document that only dictation-capable providers expose `transcriptions_url` fields: OpenAI and SiliconFlow. Black-box config tests cover OpenAI transcription URL loading and startup rejection when it is blank, and router coverage proves OpenAI text and dictation can route through distinct configured URLs. Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 120s -s SIGKILL 120s go test -count=1 ./cmd/cli -run 'TestRootCommand(RunsConfiguredProxyFromConfigFile|RunsProductionLoggerFromConfigFile|RejectsIncompleteStaticProviderConfig)'`, `timeout -k 120s -s SIGKILL 120s go test -count=1 ./internal/proxy -run TestProviderRoutingUsesConfiguredOpenAIURLsForTextAndDictation`, `timeout -k 350s -s SIGKILL 350s make test` (Go total coverage 100.0%, Python 26 passed), `timeout -k 350s -s SIGKILL 350s make lint`, `timeout -k 350s -s SIGKILL 350s make ci`, and `timeout -k 30s -s SIGKILL 30s git diff --check`.
- [x] [I008] (P1) Add OpenAI base URL to explicit provider config.
  ### Summary
  OpenAI still uses internal fixed endpoint URLs while other providers expose `base_url` in static `config.yml`. Add `providers.openai.base_url` and derive OpenAI Responses and Models endpoints from it so every provider has an explicit configured text URL surface. OpenAI dictation URL configuration is handled separately by I003.
  ### Acceptance Criteria
  1. `configs/config.yml` includes `providers.openai.base_url: "https://api.openai.com/v1"`.
  2. Runtime config loading accepts and requires `providers.openai.base_url`.
  3. OpenAI Responses and Models endpoints are derived from the configured base URL when no test endpoint override is supplied.
  4. README and provider-routing docs document OpenAI `base_url` consistently with other providers.
  5. Black-box tests cover OpenAI base URL routing.
  ### Resolution
  `configs/config.yml` now includes `providers.openai.base_url: "https://api.openai.com/v1"` alongside the required `${OPENAI_API_KEY}` placeholder. Static config loading now requires `providers.openai.base_url`, runtime configuration carries it through `OpenAIBaseURL`, and OpenAI Responses and Models endpoints are derived from that base URL when tests do not inject endpoint overrides. README and `docs/implementation/provider-routing-plan.md` now document OpenAI with the same explicit provider shape as the other upstreams. Black-box CLI config coverage asserts OpenAI base URL loading and required-provider validation, and black-box router coverage verifies OpenAI text requests use the configured base URL. Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 120s -s SIGKILL 120s go test -count=1 ./cmd/cli -run 'TestRootCommand(RunsConfiguredProxyFromConfigFile|RunsProductionLoggerFromConfigFile|RejectsIncompleteStaticProviderConfig)'`, `timeout -k 120s -s SIGKILL 120s go test -count=1 ./internal/proxy -run TestProviderRoutingUsesConfiguredOpenAIURLsForTextAndDictation`, `timeout -k 350s -s SIGKILL 350s make test` (Go total coverage 100.0%, Python 26 passed), `timeout -k 350s -s SIGKILL 350s make lint`, `timeout -k 350s -s SIGKILL 350s make ci`, and `timeout -k 30s -s SIGKILL 30s git diff --check`.
- [x] [I009] (P1) Make static provider configuration explicit and key-complete.
  ### Summary
  The static `config.yml` surface should list concrete provider base URLs instead of relying on blank values plus runtime defaults, and every supported provider key should be required through config placeholders so missing provider credentials fail at startup instead of later per request.
  ### Acceptance Criteria
  1. `configs/config.yml` contains concrete base URLs for every provider with a `base_url` field and concrete SiliconFlow transcription URL.
  2. `configs/config.yml` uses `${...}` placeholders for every supported upstream provider API key.
  3. Runtime config validation rejects missing provider API keys for all supported providers at startup.
  4. README and provider-routing docs describe provider API keys as required and base URLs as explicit config values.
  5. Black-box CLI/config tests cover missing required provider credentials.
  ### Resolution
  `configs/config.yml` now uses `${...}` placeholders for every supported provider API key and explicit URLs for every provider `base_url` plus SiliconFlow `transcriptions_url`. The config-file loader now rejects blank provider API keys, blank provider base URLs, and blank SiliconFlow transcription URLs before building runtime configuration. README and provider-routing docs now describe provider keys as required and provider URLs as explicit config values. Black-box CLI config tests cover complete startup config, missing provider key, missing provider base URL, missing SiliconFlow transcription URL, and omitted tenant defaults. Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 120s -s SIGKILL 120s go test -count=1 ./cmd/cli -run 'TestRootCommand(RunsConfiguredProxyFromConfigFile|RunsProductionLoggerFromConfigFile|RejectsIncompleteStaticProviderConfig)'`, `timeout -k 350s -s SIGKILL 350s make test` (Go total coverage 100.0%, Python 26 passed), `timeout -k 350s -s SIGKILL 350s make lint`, and `timeout -k 350s -s SIGKILL 350s make ci`.
- [x] [I010] (P1) Decouple OpenAI background polling from text worker occupancy.
  ### Summary
  B003 made long OpenAI background Responses calls succeed through one blocking REST request, but the internal text worker queue still treats the whole request lifecycle as occupying a scarce worker. A long semantic-review call can spend most of its time sleeping between server-side polls while preventing other text requests from starting.
  ### Impact
  The public REST contract is correct, but internal capacity is still coupled to long provider lifecycles. With low `server.workers` values, a viable long OpenAI request can reduce unrelated request throughput even when no upstream HTTP operation is active.
  ### Expected
  Keep the external one-shot REST contract unchanged. Internally, `server.workers` should limit concurrent upstream HTTP operations, `server.queue_size` should limit pending upstream HTTP operations, and OpenAI poll sleeps should not occupy an upstream worker.
  ### Acceptance Criteria
  1. `GET /`, `POST /`, and `POST /v2` still return the final answer in the original HTTP response with no streaming, client polling, or resume endpoint.
  2. OpenAI background polling remains server-side and bounded by `server.request_timeout_seconds`.
  3. Long OpenAI poll sleeps do not occupy a `server.workers` slot.
  4. `server.workers` and `server.queue_size` apply consistently across text providers and dictation upstream HTTP calls.
  5. Black-box integration coverage proves a simple REST request can complete while another OpenAI background request is between server-side polls, even with one worker.
  ### Resolution
  Text requests now run provider work in per-request goroutines while the shared `limitedHTTPDoer` enforces `server.workers` as active upstream HTTP operation concurrency and `server.queue_size` as the pending upstream HTTP operation queue. OpenAI background-response sleeps release worker capacity between polls, and the same limiter is shared by OpenAI Responses, OpenAI-compatible chat providers, Gemini, Anthropic, and dictation. `/dictate` now propagates the inbound request context to upstream transcription calls. README and provider-routing docs describe the internal concurrency contract without changing the blocking REST caller contract.
  Validation passed with `timeout -k 120s -s SIGKILL 120s go test -count=1 ./tests/integration -run 'TestIntegration(BackgroundPollSleepDoesNotOccupyUpstreamWorker|HighLoadQueue|GatewayContextTimeoutCancelsUpstreamRequest|UpstreamRequestTimeoutTriggersGatewayTimeout)'`, `timeout -k 180s -s SIGKILL 180s go test -count=1 ./tests/llm-proxy -run 'TestEndpoint_Returns(ServiceUnavailableWhenQueueFull|GatewayTimeoutWhenWaitingForUpstreamWorker)'`, `timeout -k 350s -s SIGKILL 350s make go-test` (total coverage 100.0%), `timeout -k 350s -s SIGKILL 350s make ci` (Go lint/staticcheck/ineffassign, Python mypy, Go total coverage 100.0%, Python 27 passed, root import smoke), `timeout -k 30s -s SIGKILL 30s git diff --check`, and `timeout -k 180s -s SIGKILL 180s make test-live-providers LIVE_ENV_FILE=configs/.env LLM_PROXY_LIVE_PROVIDERS=openai` (`model` omitted for OpenAI, HTTP 200). Live exact-payload replay of `/tmp/llm-proxy-b002-direct/po-schuchemu-velenyu.chunk-0001.llm-proxy-request.json` through a temporary current-branch local proxy returned HTTP 200 in 159.191s from one `POST /?provider=openai&format=text/plain` and parsed as valid JSON with 23 `targetReviews`, `needsHumanReview=false`, and 0 `reviewItems`.
- [x] [I011] (P1) Codify provider default model selection for omitted JSON model fields.
  ### Summary
  Request model identifiers are provider-scoped, so `provider=gemini` with `model=gpt-5-mini` correctly returns `400`. When the client omits `model`, the selected provider's configured default model must be used universally for `GET /`, `POST /`, and `POST /v2`, and a provider with an API key must have a configured default model.
  ### Acceptance Criteria
  1. `POST /?provider=gemini` without a JSON body `model` routes to Gemini's configured text `default_model`.
  2. `POST /v2?provider=gemini` without a JSON body `model` routes to Gemini's configured text `default_model`.
  3. Static config validation rejects a keyed non-OpenAI provider whose text catalog has a blank `default_model`.
  4. README and provider-routing docs state that omitted `model` uses the selected provider default and that keyed providers require valid defaults.
  5. Live Gemini provider smoke proves the configured default works without a model override.
  6. Bundled Go, CLI, and Python clients omit `model` when callers do not specify a model while still preserving the selected provider.
  ### Resolution
  The existing provider registry already used the selected provider's configured default model when `model` was omitted. Added black-box JSON POST coverage for `POST /?provider=gemini` and `POST /v2?provider=gemini` without `model`, both asserting the upstream Gemini default path `/models/gemini-2.5-flash:generateContent`. Added CLI config validation coverage for a keyed Gemini provider with a blank text `default_model`. README and provider-routing docs now explicitly connect configured provider keys, required default models, and omitted-model request behavior. Added bundled Go, CLI, and Python client coverage proving provider-selected requests omit stale query/body `model` values when the caller does not specify a model. Python client failure context now reports omitted provider/model values as `provider=omitted model=omitted` instead of inventing a default model label.
  Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 120s -s SIGKILL 120s go test -count=1 ./internal/proxy -run TestProviderRoutingUsesGeminiDefaultModelForJSONPosts`, `timeout -k 120s -s SIGKILL 120s go test -count=1 ./cmd/cli -run TestRootCommandRejectsIncompleteStaticProviderConfig/blank_keyed_gemini_text_default_model`, `timeout -k 120s -s SIGKILL 120s go test -count=1 ./pkg/llmproxyclient -run TestClientOmitsModelWhenRequestUsesProviderDefault`, `timeout -k 120s -s SIGKILL 120s go test -count=1 ./llm-proxy-client -run TestCommandReadsPromptFileAndOptionalBodyFields`, `timeout -k 120s -s SIGKILL 120s bash -lc 'cd python && uv run --group dev pytest tests/test_client.py -k "omits_model or read_timeout"'`, `timeout -k 180s -s SIGKILL 180s go test -count=1 ./internal/proxy`, `timeout -k 180s -s SIGKILL 180s go test -count=1 ./cmd/cli`, `bash -n scripts/test_live_providers.sh scripts/test_live_gemini.sh`, `timeout -k 180s -s SIGKILL 180s make test-live-providers LIVE_ENV_FILE=configs/.env LLM_PROXY_LIVE_PROVIDERS=gemini` (`model=omitted`, HTTP 200), `timeout -k 30s -s SIGKILL 30s git diff --check`, and `timeout -k 350s -s SIGKILL 350s make ci` (Go/Python lint clean, Go total coverage 100.0%, Python 28 passed).
- [x] [I012] (P1) Make bundled clients canonical v2-only transports.
  ### Summary
  The public server remains a blocking REST proxy with `GET /`, compatibility JSON `POST /`, and canonical `POST /v2`, but bundled client libraries should not keep two text request shapes. They should expose only the canonical v2 messages contract so downstream callers do not have to choose between prompt JSON and messages JSON.
  ### Acceptance Criteria
  1. The reusable Go client exposes only `NewMessagesRequest` and `Client.PostMessages` for text requests.
  2. The installable Go CLI sends `--prompt`, stdin, or `--prompt-file` content as a v2 `user` message through `POST /v2`.
  3. The Python package exports only `ClientMessagesRequest` and `Client.post_messages` for text requests.
  4. README and provider-routing docs state that bundled clients are v2-only while the server still supports the direct REST endpoints.
  5. Black-box Go CLI, Go client, and Python client tests cover the v2-only path and omitted-model provider-default behavior.
  ### Resolution
  Removed the legacy Go `Request`/`NewRequest`/`Client.Post` API and the Python `ClientRequest`/`Client.post` API. The Go CLI now maps prompt inputs into v2 `messages[]`, sends through `Client.PostMessages`, preserves the clearer `missing prompt` boundary error, and still omits `model` when `--model` is not provided. README and provider-routing docs now state that bundled clients are v2-only while direct server callers may still use `GET /`, compatibility `POST /`, or canonical `POST /v2`.
  Validation passed with `timeout -k 30s -s SIGKILL 30s make fmt`, `timeout -k 120s -s SIGKILL 120s go test -count=1 ./pkg/llmproxyclient`, `timeout -k 120s -s SIGKILL 120s go test -count=1 ./llm-proxy-client`, `timeout -k 120s -s SIGKILL 120s bash -lc 'cd python && uv run --group dev pytest tests/test_client.py'` (Python 20 passed), `timeout -k 350s -s SIGKILL 350s make ci` (Go/Python lint clean, Go total coverage 100.0%, Python 20 passed, root import smoke passed), and `timeout -k 30s -s SIGKILL 30s git diff --check`.
- [x] [I013] (P0) Limit upstream HTTP call rate in shared HTTP client for text and dictation, without provider‑specific logic.
  Goal:
  Ensure the shared HTTP client layer enforces a consistent, configurable limit on upstream HTTP calls across both text and dictation flows, so that provider rate limits and system resource usage are controlled without duplicating provider-specific throttling logic in multiple places.
  
  Requirements:
  - Apply limiting at the shared HTTP client abstraction used by text and dictation, not in provider-specific integrations.
  - Support configuration of call limits (e.g., max requests per unit time) via existing config mechanisms or a new, clearly documented one.
  - Avoid changing existing provider-specific semantics beyond the addition of call limiting; existing error handling, retries, and timeouts should remain compatible.
  - Ensure limits can be tuned independently per upstream host or service if needed, without hard-coding provider identities.
  - Preserve observability: logging and/or metrics should make it possible to understand when and how often calls are limited.
  - Do not introduce breaking API changes for current callers of the shared HTTP client without a migration path.
  - Handle concurrent requests safely to avoid race conditions or inconsistent enforcement of limits.
  
  Deliverables:
  - Updated shared HTTP client implementation that enforces upstream HTTP call limits for all consumers, including text and dictation paths.
  - Configuration options (and defaults) for call limiting behavior, with inline code docs describing their usage.
  - Logging/metrics hooks that surface when requests are delayed, denied, or otherwise affected by the limiting behavior.
  - Brief developer documentation or comments explaining where the limiting occurs, how to configure it, and how it interacts with existing retry/timeout behavior.
  - Tests covering at least: respecting configured limits, behavior under concurrent load, interaction with retries, and no regression for existing non-limited behavior when limits are effectively disabled.
  
  Validation:
  - Run automated tests demonstrating that the shared HTTP client enforces configured call limits under concurrent traffic for both text and dictation code paths.
  - Confirm that no provider-specific modules contain duplicated or custom call-limiting logic for this purpose after the change, or that any remaining logic is clearly outside this shared concern.
  - Verify through logs/metrics that limiting events are emitted as expected when thresholds are reached.
  - Validate that typical workloads for text and dictation still succeed without unexpected errors when limits are set to realistic production values.
  - Perform a targeted regression check to ensure existing integrations using the shared HTTP client behave as before when limits are configured to be non-restrictive (e.g., effectively off).
  Resolution:
  Added validated `server.upstream_rate_limits` rules keyed by exact normalized HTTP(S) origin, with strict rolling-window enforcement in the shared `limitedHTTPDoer` after worker acquisition at actual upstream admission. Text, dictation, transport retries, and OpenAI response retries now consume the same origin budget without provider-specific throttling; distinct origins remain independent, and an empty rule list keeps limiting disabled. Delayed and context-canceled waits emit structured origin/limit/interval/wait logs, while cancellation preserves the existing gateway-timeout mapping. README and provider-routing documentation describe the canonical config and retry/queue interaction. Black-box CLI and router coverage proves config validation, concurrent enforcement, shared text/dictation limits, origin independence, retry accounting, cancellation logging, and unrestricted behavior when disabled. Validation passed with `timeout -k 180s -s SIGKILL 180s go test -race -count=1 ./tests/integration -run 'TestIntegration.*UpstreamRateLimit|TestIntegrationSharedUpstreamRateLimit'`, `timeout -k 350s -s SIGKILL 350s make go-test` (total coverage 100.0%), `timeout -k 350s -s SIGKILL 350s make lint`, `timeout -k 350s -s SIGKILL 350s make ci` (Go/Python/frontend gates; Go total coverage 100.0%, Python 20 passed, Playwright 11 passed), and a scoped `git diff --check` over I013-owned files.
- [x] [I014] (P2) Align the management header avatar with the right edge.
  Goal:
  Move the MPR header avatar/login control to the far right of the header content measure so it balances the left-side LLM Proxy brand title instead of sitting immediately beside it.
  Requirements:
  - Keep using the shared `<mpr-header>` and `<mpr-user>` components.
  - Scope the layout change to the llm-proxy static site rather than changing the shared MPR UI package.
  - Preserve authenticated avatar and unauthenticated login placement through the same header actions container.
  Deliverables:
  - Static-site CSS that pushes the MPR header actions region to the right edge.
  Validation:
  - Verify the rendered header in a browser preview at desktop and mobile widths.
  - Run focused frontend syntax checks for edited static files.
  Resolution:
  Added a scoped `#llm-proxy-header .mpr-header__actions` CSS override so the shared MPR header actions region uses `margin-left: auto`. Browser preview validation rendered the real static Pages app and CDN MPR UI bundle with local mock config/profile responses: desktop geometry placed the brand at x=100 and avatar at x=1146..1180, while mobile 390px geometry placed the brand at x=24..111 and avatar at x=332..366 with no overlap. Validation passed with `timeout -k 30s -s SIGKILL 30s node --check site/assets/llm-proxy/js/app.js`, `timeout -k 30s -s SIGKILL 30s node --check site/assets/llm-proxy/js/core/mprShell.js`, and `timeout -k 30s -s SIGKILL 30s git diff --check`.
- [x] [I015] (P2) Add LLM Proxy icon and favicon assets.
  Goal:
  Give the static management site a recognizable LLM Proxy product icon and browser favicon instead of the current empty favicon placeholder.
  Requirements:
  - Keep the assets local to the static Pages site.
  - Use the existing dark MPR UI palette and a proxy-routing mark that remains legible at favicon size.
  - Avoid introducing generated binary assets or external image dependencies.
  Deliverables:
  - Add SVG app-icon and favicon assets under `site/assets/llm-proxy/img/`.
  - Point `site/index.html` at the favicon and app icon.
  - Add focused static-site coverage for the icon links and SVG assets.
  Validation:
  - Run focused frontend syntax/tests for the touched static-site path.
  Resolution:
  Added local SVG app-icon and favicon assets under `site/assets/llm-proxy/img/` using the existing MPR Lab LLM Proxy project-card mark from `../marcopolo.github.io/assets/projects/llm-proxy/icon.svg`: dark teal/green gradient, gold routing paths, cyan endpoints, and a central proxy node. Replaced the empty favicon placeholder in `site/index.html` with real SVG favicon/app-icon links and the MPR favicon theme color. Added focused Playwright/static-site coverage that verifies the links, SVG content type, and MPR gold/cyan palette. Validation passed with `timeout -k 120s -s SIGKILL 120s make frontend-lint`, `timeout -k 180s -s SIGKILL 180s npx playwright test tests/e2e/management-ui.spec.js -g "site exposes product icon"`, and `timeout -k 30s -s SIGKILL 30s git diff --check`.
- [x] [I016] (P2) Encrypt managed provider API keys at rest.
  Goal:
  Store tenant-owned upstream provider API keys encrypted at rest while preserving the existing management UI and generated-secret proxy routing behavior.
  Requirements:
  - Add one canonical management config field for the provider-key encryption key.
  - Require a valid base64-encoded 32-byte encryption key whenever management mode is enabled.
  - Encrypt every newly saved managed provider API key before database persistence.
  - Migrate existing plaintext provider-key rows into encrypted records and clear plaintext values.
  - Keep generated tenant secrets digest-only and provider keys masked in management/admin API responses.
  - Document this as an encrypted-at-rest guarantee, not as user-only decryption or zero-knowledge.
  Deliverables:
  - Extend config parsing, packaged config, Pages render environment, and docs.
  - Add AES-GCM provider-key storage with row-bound associated data.
  - Add focused Go coverage for config validation, encrypted persistence, plaintext migration, and runtime decrypt routing.
  Validation:
  - Run focused management store/API and CLI config tests.
  - Run broader Go validation before marking resolved.
  Resolution:
  Added required `management.provider_key_encryption_key` configuration with base64 32-byte validation, AES-GCM encrypted provider-key persistence, row-bound associated data, and startup migration that encrypts and clears legacy plaintext `api_key` rows. Updated config templates, Pages render env, README, and provider-routing docs to state the exact encrypted-at-rest guarantee and explicitly distinguish it from zero-knowledge/user-only decryption. Added focused Go coverage for config parsing, validation, encrypted storage, plaintext migration, decrypt failures, and generated-secret proxy routing. Validation passed with `timeout -k 180s -s SIGKILL 180s go test -count=1 ./internal/proxy -run 'TestManagedTenant(StoreInternalEdges|StoreStaticConfigMigrationEdges|GORMDatabaseEncryptsProviderKeysAtRest)|TestManagementConfigurationValidationRequiresBackendAuthFields|TestManagementGeneratedSecretRoutesWithTenantProviderKey|TestManagementGeneratedSecretSupportsDictationAndRejectsMultipartProviderKeys'`, `timeout -k 180s -s SIGKILL 180s go test -count=1 ./cmd/cli -run 'TestRootCommand(RunsConfiguredProxyFromConfigFile|LoadsPackagedConfigWithManagementEnvironment|RendersSiteFromConfigFile|RejectsUnsupportedManagementDatabaseDialect)'`, `timeout -k 350s -s SIGKILL 350s make go-test`, `timeout -k 30s -s SIGKILL 30s make check-format`, and `timeout -k 30s -s SIGKILL 30s git diff --check`.
- [x] [I017] (P2) Let Settings request examples fold as one usage segment.
  Goal:
  Reduce Settings modal sprawl by letting users collapse the full Usage / Request examples segment while preserving copyable default and selected-provider commands.
  Requirements:
  - The Request examples segment starts folded when Settings opens.
  - Expanding the segment reveals the existing default and selected-provider request examples without changing their commands.
  - Copy actions and selected-provider updates keep working from the expanded segment.
  - Browser coverage proves the folded and expanded states through the real Settings modal.
  Deliverables:
  - Static management UI markup and state for the foldable request examples segment.
  - Focused Playwright coverage for the disclosure behavior.
  Validation:
  - Run focused frontend validation for the management Settings modal.
  Resolution:
  The Settings modal now renders the Usage / Request examples segment as a native disclosure that starts folded each time Settings opens. Expanding the segment reveals the existing default and selected-provider request examples without changing generated curl commands, copy actions, selected-provider updates, or generated-secret replacement. Playwright coverage now asserts the folded initial state and expands the segment before exercising command visibility, provider updates, clipboard copy, and generated-secret replacement. Validation passed with `timeout -k 180s -s SIGKILL 180s make frontend-lint`, `timeout -k 180s -s SIGKILL 180s npm run frontend:test -- tests/e2e/management-ui.spec.js -g "dashboard shows usage|settings shows placeholder request examples|settings request examples use"`, `timeout -k 180s -s SIGKILL 180s make frontend-test`, and `timeout -k 30s -s SIGKILL 30s git diff --check`.
- [x] [I018] (P1) Add repo-grounded SEO resource pages.
  Goal:
  Publish 40-50 public resource pages for LLM Proxy and expose them from the main static site through crawlable links.
  Requirements:
  - Use repo evidence for every product claim and avoid unsupported customer, benchmark, compliance, pricing, ranking, and competitor claims.
  - Generate a hub under `/resources/` and 40-50 distinct pages with page-specific metadata, canonical URLs, structured data, visible content, FAQ, and related links.
  - Link the resource hub from the main page with a normal HTML anchor.
  - Add sitemap and robots surfaces aligned to the canonical hosted URL form.
  - Keep the Pages artifact free of static browser runtime config.
  Deliverables:
  - Added `scripts/generate_seo_resources.mjs` as the structured source for the 45-page resource cluster.
  - Added `site/resources/`, `site/sitemap.xml`, `site/robots.txt`, and resource-page CSS.
  - Added `docs/marketing/seo-resource-cluster-report.md` with repo analysis, opportunities, integration notes, and evaluation.
  - Added frontend coverage for root discoverability, the resource hub, a representative resource page, sitemap, and robots.
  Validation:
  - Run focused and full frontend validation plus the site renderer check.
  Resolution:
  Generated 45 repo-grounded resource pages plus a `/resources/` hub, linked the hub from `site/index.html`, added canonical metadata/JSON-LD/FAQ/related links across resource pages, and added `sitemap.xml` plus `robots.txt` for the public URL set. The generator is covered by frontend syntax checks and writes the SEO cluster plus evaluation report deterministically. Validation passed with `timeout -k 120s -s SIGKILL 120s make frontend-lint`, `timeout -k 180s -s SIGKILL 180s npm run frontend:test -- tests/e2e/management-ui.spec.js -g "SEO"`, `timeout -k 240s -s SIGKILL 240s make frontend-test`, `timeout -k 180s -s SIGKILL 180s go test -count=1 ./cmd/cli -run 'TestRootCommand(Render|Renders)'`, `timeout -k 30s -s SIGKILL 30s git diff --check`, and `timeout -k 500s -s SIGKILL 500s make ci`.
- [x] [I019] (P1) Add LoopAware traffic pixel to all pages of LLM-proxy.
  ### Summary
  The user wants to add a traffic pixel from LoopAware to all of the pages of LLM-proxy.
  ### Acceptance Criteria
  1. Add the LoopAware script tag `https://loopaware.mprlab.com/pixel.js?site_id=839f018b-97a9-4955-a489-4ad5cb626f4f` to the head of `site/index.html`.
  2. Add the same LoopAware script tag to all generated resource pages in `site/resources/` via the template helper in `scripts/generate_seo_resources.mjs`.
  3. Update `RESOURCE_MODIFIED_DATE` in `scripts/generate_seo_resources.mjs` and `seoContentModifiedDate` in `tests/e2e/management-ui.spec.js`.
  4. Intercept/mock loopaware requests in e2e tests to prevent actual network calls.
  ### Resolution
  Added LoopAware traffic pixel script to site/index.html and generate_seo_resources.mjs htmlDocument, updated the modified date to 2026-07-11 in generator and tests, and mocked loopaware network requests in Playwright. All tests passed.

- [ ] [I025] (P1) Let users reveal and edit their saved provider API keys.
  Goal:
  Let an authenticated user select any provider whose API key they previously saved, explicitly reveal the complete decrypted key, edit it in the existing provider-key field, and save the updated value. The current profile exposes only masked status and leaves the edit field blank, so users cannot inspect or correct a saved key without replacing it from another source.
  Requirements:
  - Add one explicit owner-only reveal contract for the selected provider, `POST /api/management/provider-keys/:provider/reveal`, returning only `{ "api_key": "..." }` for the authenticated user's saved provider record.
  - Validate the canonical provider identifier at the HTTP edge, return `404 Not Found` when that user has no saved key for the provider, and never read or reveal another user's provider record.
  - Treat reveal as a sensitive credentialed management action: require the configured management origin and JSON content type, use the existing TAuth session-cookie boundary, and return `Cache-Control: no-store` so browsers and intermediaries do not retain the response.
  - Keep profile and administrator responses masked and free of raw provider keys. Do not include revealed values in notices, error bodies, logs, analytics, URLs, DOM attributes, local storage, session storage, or generated request examples.
  - Preserve AES-GCM encryption at rest and the current runtime-only decryption path for proxy routing; the new reveal endpoint is the sole additional path that may return decrypted provider-key material to its owning user.
  - In the selected-provider editor, show a `Show key` action only when the selected provider has a saved key. The action must fetch the key on demand and populate the existing API-key input as an editable value; it must not preload every provider key when Settings or the profile loads.
  - Let the user switch the populated input between visible and password-masked presentation without changing the value being edited. Saving must update the selected provider with the edited key through the current provider-key mutation contract.
  - Clear decrypted key material from application state and the input when the selected provider changes, Settings closes, the key is saved or removed, the user signs out, or the page reloads. A successful save must return the editor to masked-key status and require another explicit reveal to show the stored value.
  - Disable reveal/save/remove/provider-selection controls while the relevant request is in flight, and ensure a late reveal response cannot populate the field after the user changes provider or closes Settings.
  - Update the security documentation from the obsolete absolute claim that provider keys are never returned raw after save to the exact current contract: raw keys are returned only through an explicit authenticated owner reveal, while normal profile/admin responses remain masked and storage remains encrypted at rest.
  Deliverables:
  - Add the authenticated provider-key reveal handler, owner-scoped store operation, response type, no-store headers, and stable error behavior.
  - Add selected-provider reveal/hide/edit state and controls to the static Settings UI without adding a second provider editor or credential persistence path.
  - Update frontend types, user-facing copy, README, and provider-routing documentation for the reveal and edit security boundary.
  Validation:
  - Add black-box management HTTP coverage proving an authenticated owner can reveal the exact decrypted key, an anonymous or wrong-origin request is rejected, a different user cannot reveal the owner's key, missing/unknown providers fail explicitly, responses are non-cacheable, and profile/admin payloads still contain no raw keys.
  - Add persistence coverage proving reveal decrypts the encrypted record without writing plaintext to the database and an edited key is re-encrypted before save while generated-secret proxy routing uses the new value.
  - Add Playwright coverage for explicit reveal, visible/hidden presentation, editing and saving, masked post-save state, provider switching, Settings close/reopen, sign-out cleanup, request-order races, and absence of raw keys from browser storage and unrelated UI surfaces.
  - Run the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci` pair for the implementation, with the final run occurring after the last code edit.


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
  Last run: 2026-07-20. Removed the stale duplicate open F010 and F011 entries because the current provider catalog already includes Grok and GLM 5.2, reassigned the provider-key reveal work to I025, and updated F014 to reference I025. No active, blocked, recurring, or planning issue was archived.
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
  Last run: 2026-06-29.
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
- [x] [M009R] (P1) Consolidate repository runbook documents under `.mprlab/`.
  ### Summary
  The repository had duplicate runbook and issue-tracker documents under `issues.md/`, `.mprl/`, and `.mprlab/`. Keep the active tracker and relevant recurring procedures under `.mprlab/`, then remove the old duplicate locations.
  ### Resolution
  Consolidated the current policy, planning, issue-format, and stack-guide documents under `.mprlab/`; kept `.mprlab/ISSUES.md` as the active tracker; carried forward recurring housekeeping runbooks from `issues.md/ISSUES.md`; updated stale runbook path references; and removed the duplicate `issues.md/` and `.mprl/` directories.
- [x] [M010] (P2) Document 60-day social media advertising campaign.
  Goal:
  Prepare a twice-daily social media post schedule that advertises LLM Proxy through concrete problems it solves.
  Requirements:
  Include 120 posts for the 60-day period beginning 2026-07-06, keep each post under 300 characters, and ground claims in the documented product contract.
  Deliverables:
  Added `docs/marketing/social-media-60-day-campaign.md`.
  Validation:
  Verified the document contains 120 scheduled posts and every post is below the 300-character limit.
- [x] [M011] (P1) Require CI before and after every code-changing task.
  Goal:
  Make repository CI failures visible before code is edited and require the final implementation state to pass the canonical CI gate.
  Requirements:
  - Require agents to run `make ci` before the first code edit and after the final code edit for every code-changing task.
  - Keep both CI runs agent-owned even when the execution chain performs later completion checks.
  - Treat any later code edit as invalidating the final CI run.
  - Preserve concrete baseline failure evidence without blocking work whose purpose is to repair that failure.
  Deliverables:
  - Update the root `AGENTS.md` workflow so the requirement is binding and does not conflict with execution-chain ownership or routine-validation limits.
  Validation:
  - Inspect the final governance diff and run documentation whitespace checks.
  Resolution:
  Root `AGENTS.md` now requires an agent-owned `make ci` baseline immediately before the first code edit and a passing `make ci` run immediately after the final code edit for every code-changing task. Later code edits invalidate the final run, partial targets cannot replace either run, baseline failures must be reported with concrete output, and execution-chain completion checks do not replace the pair. The workflow, completion gate, testing guidance, and pre-finish checklist now carry the same canonical requirement without contradictory command restrictions. No code changed, so the new CI pair did not apply to this governance-only edit. `git diff --check` passed. The MPR Lab normalizer check still reports broad governance drift that was already present in the pre-edit dry run; follow-up is tracked in M012.
- [ ] [M012] (P2) Reconcile repository governance with the MPR Lab normalizer.
  Goal:
  Make the governance normalizer check pass without deleting repository-owned binding contracts.
  Requirements:
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


## Features

- [x] [F001] (P1) Add authenticated self-service API key and tenant secret management UI.
  ### Summary
  Users need an authenticated browser UI where they sign in through the MPR/TAuth login surface, ask llm-proxy to create a new client key for them, bring their own upstream provider API keys for any supported provider, choose tenant defaults, and then use the service with the generated llm-proxy tenant secret. This should turn llm-proxy from an operator-provisioned static-tenant service into a self-service tenant onboarding surface without changing the public proxy request contract: clients still call `GET /`, JSON `POST /`, `POST /v2`, and `POST /dictate` with `key=<tenant secret>`, while upstream provider API keys stay server-side.
  ### MPR UI and authentication contract
  The UI must follow the `mpr-integration` `mpr-ui` and TAuth contracts:
  1. Serve `/config-ui.yaml` as the only browser-facing MPR UI auth config surface; in split-origin deployments it is served by the API origin and consumed by the static Pages app.
  2. Load pinned `mpr-ui.css`, Google Identity Services, `js-yaml`, pinned `mpr-ui-config.js`, and the pinned `mpr-ui` bundle after applying the configured YAML.
  3. Render the shared shell through `<mpr-header>`, `<mpr-user>`, and `<mpr-footer>` rather than direct `tauth.js` loading or manual `tauth-*` attributes.
  4. Treat successful TAuth login as the gate before showing tenant, provider-key, or generated-secret controls.
  5. React to documented `mpr-ui:auth:authenticated`, `mpr-ui:auth:unauthenticated`, `mpr-ui:auth:status-change`, and `mpr-ui:auth:error` events as needed; use `auth-transition` only when the authenticated settings surface has a visible loading gap.
  6. Protect all key-management APIs with TAuth session-cookie validation. Unauthenticated requests return `401`, not a rendered management page or partial tenant data.
  7. Use exact profile values for frontend origin, backend/API origin, cookie names, issuer, tenant id, OAuth callback, TLS/cookie policy, CORS credentials, DNS, reverse proxy, and service port. Do not infer hosted values from localhost.
  ### Product contract
  1. An authenticated user sees a primary "Create a new key" flow that creates or selects their llm-proxy tenant and generates a client tenant secret.
  2. Tenant ownership is bound to the authenticated TAuth subject; users can manage only their own tenant secrets, provider keys, defaults, and usage examples.
  3. The user can add, update, or remove their own API keys for every provider supported by llm-proxy, then configure text/dictation defaults from those available providers.
  4. Provider API keys are accepted only through the authenticated management UI/API, stored server-side, masked after save, never returned raw, and never logged.
  5. Public proxy requests never accept upstream provider API keys in query parameters or JSON/multipart bodies.
  6. The tenant secret is generated by the server from a cryptographically strong source, is unique, is shown only once after generation, and can be revoked or regenerated from the authenticated UI.
  7. Generated secrets authenticate existing proxy endpoints through the current `key` query parameter and select that tenant's provider credentials and defaults.
  8. Missing, invalid, or revoked tenant secrets continue to return `403` on proxy endpoints without exposing whether a tenant exists.
  9. Tenant/provider configuration has one authoritative runtime store and loader. Do not preserve legacy static-only fallback reads or old single-secret schema interpretation for this feature unless a separate compatibility issue explicitly asks for it.
  ### Acceptance Criteria
  1. Add the MPR-authenticated browser shell and management pages using the declarative `/config-ui.yaml` + `mpr-ui-config.js` path; key-management controls are visible only after TAuth login.
  2. Add backend management endpoints for authenticated tenant profile, provider-key save/update/removal, default provider/model settings, tenant secret generation, revoke, and regenerate.
  3. Add a "create new key" flow that creates the tenant-owned client secret and presents copyable usage examples for text, `/v2`, and dictation calls.
  4. Add a validated domain model for tenant-owned provider credentials and generated client secrets with edge-only validation and no zero-but-invalid exported state.
  5. Store provider API keys and generated tenant secrets securely; raw provider keys and raw historical secrets are not readable through the UI/API after save.
  6. Route proxy text and dictation requests using the generated tenant secret and tenant-owned provider credentials/defaults.
  7. Document the self-service setup flow, generated-secret usage examples, provider-key security model, and hosted profile values required for production deployment.
  8. Add black-box HTTP tests for unauthenticated management `401`, authenticated key save/masking, cross-user tenant isolation, secret generation, generated-secret proxy success, revoked-secret `403`, and rejection of client-supplied provider keys on public proxy endpoints.
  9. Add Playwright browser coverage for the MPR shell rendering, login/session recovery, "create new key" behavior, provider-key form behavior, one-time secret display, revoke/regenerate flow, and copied usage example using the generated secret.
  ### Resolution
  Implemented management mode for the self-service browser UI using the MPR UI `/config-ui.yaml` contract, pinned MPR UI assets, `<mpr-header>`, `<mpr-user>`, and `<mpr-footer>`. Added TAuth session-cookie validation for `/api/management/*`, tenant-owned provider-key storage with masked responses, generated llm-proxy client secrets returned once and stored only as SHA-256 digests, default provider/model management, revoke/regenerate behavior, and managed-tenant routing through the existing `key=<tenant secret>` proxy contract. Management mode requires a configured persistent `management.database_dsn`; signup state, enabled providers, defaults, and generated-secret digests are persisted through GORM and are not stored by mutating the runtime config file. Public proxy endpoints now reject client-supplied provider API keys in query, JSON, and multipart inputs. Documented management configuration, production profile values required for deployment, security behavior, usage examples, and the no-raw-SQL GORM-only persistence rule in README/provider-routing docs and binding repo policy. Added black-box HTTP tests plus focused internal edge tests for unauthenticated access, key masking, cross-user isolation, generated-secret proxying, revoked-secret `403`, persistence failures, TAuth validation, and provider-key rejection; performed a local Playwright smoke of the rendered MPR shell because this repository has no committed browser-test harness. Validation passed with `timeout -k 350s -s SIGKILL 350s make test`, `timeout -k 350s -s SIGKILL 350s make lint`, and `timeout -k 350s -s SIGKILL 350s make ci`.
- [x] [F002] (P1) Add one-time migration from legacy config tenants and provider API keys into the DB.
  ### Summary
  Management mode needs a one-off migration from legacy YAML tenant/provider-key configuration into the GORM database. After that migration, tenant secrets and provider API keys in the config file are obsolete seed material and runtime proxy authentication should use the database. Server/runtime configuration stays in the config file, including management auth settings, provider base URLs, and provider model catalogs.
  ### Product contract
  1. When `management.enabled` is true, startup opens the management DB and records a durable migration marker.
  2. On the first management-mode startup before that marker exists, configured legacy tenants are imported into DB tenant rows with their configured tenant id, secret digest, defaults, and all nonblank configured provider API keys.
  3. After the marker exists, startup ignores legacy config tenants and provider API key fields even if they remain in the YAML file.
  4. Public proxy authentication in management mode is DB-authoritative after startup; static config tenants are migration seed input only.
  5. Server/runtime settings, TAuth/MPR UI settings, provider base URLs, transcription URLs, and model catalogs remain config-file-owned.
  ### Acceptance Criteria
  1. Add black-box HTTP coverage showing a legacy config tenant/key works after first DB migration.
  2. Add coverage showing the same tenant still works after config tenants/provider API keys are removed.
  3. Add coverage showing reintroduced stale config credentials do not overwrite the migrated DB data after the marker exists.
  4. Preserve the GORM-only/no-raw-SQL rule.
  ### Resolution
  Added a GORM-tracked static-config migration marker and a management-mode startup migration that imports legacy configured tenants into DB tenant rows with their original tenant id, secret digest, defaults, and every nonblank configured provider API key. After the marker exists, management-mode startup ignores config-file tenants and provider `api_key` values; public proxy authentication and provider credentials are DB-authoritative. Server/runtime settings, TAuth/MPR UI settings, provider base URLs, transcription URLs, and model catalogs remain config-file-owned. Static tenant/default credential validation is skipped in management mode because those fields are seed material only. Added black-box HTTP coverage proving the migrated legacy secret works after first startup, still works after tenant/key YAML removal, and keeps using the migrated DB provider key when stale config credentials are reintroduced. Validation passed with `timeout -k 240s -s SIGKILL 240s go test -count=1 ./cmd/cli`, `timeout -k 350s -s SIGKILL 350s make test` (Go total coverage 100.0%, Python 20 passed), `timeout -k 350s -s SIGKILL 350s make lint`, `timeout -k 350s -s SIGKILL 350s make ci` (Go total coverage 100.0%, Python 20 passed), `timeout -k 30s -s SIGKILL 30s git diff --check`, and GORM/no-stale-store scans for raw SQL/direct SQL and JSON-store wording.
- [x] [F003] (P1) Support explicit GORM database dialects for management persistence.
  ### Summary
  Management persistence should support multiple GORM-backed SQL dialects rather than assuming Postgres whenever `management.database_dsn` is present. The initial supported dialects are Postgres and SQLite.
  ### Product contract
  1. `management.database_dialect` selects the GORM dialector used for tenant-owned provider keys, defaults, generated-secret digests, and static-config migration markers.
  2. Supported values are `postgres` and `sqlite`; unknown values fail startup before serving.
  3. The default remains `postgres` so existing management configs keep the current behavior unless they opt into SQLite.
  4. `management.database_dsn` remains required in management mode and is passed unchanged to the selected GORM dialector.
  5. Database access continues to use GORM model APIs only; raw SQL and direct SQL clients remain prohibited.
  ### Acceptance Criteria
  1. Add CLI/config loading coverage for `management.database_dialect`.
  2. Add startup coverage proving SQLite opens through configured management settings without the test-only dialector injection.
  3. Add startup failure coverage for unsupported dialect values.
  4. Preserve existing Postgres behavior.
  ### Resolution
  Added `management.database_dialect` to the management YAML and runtime configuration, with `postgres` as the default/current behavior and `sqlite` as the second supported GORM dialect. The GORM opener now selects the dialector from the explicit dialect field and passes `management.database_dsn` unchanged to the selected driver. Unsupported dialects fail startup during management config validation. Added CLI coverage for dialect parsing and unsupported dialect rejection, real router/startup coverage that opens SQLite through configured management settings without test-only dialector injection, and direct GORM opener coverage for explicit/default Postgres and invalid dialect behavior. README and provider-routing docs now document supported dialects and DSN semantics, and `.mprlab/AGENTS.GO.md` lists both GORM dialect drivers as approved data dependencies. Validation passed with `timeout -k 240s -s SIGKILL 240s go test -count=1 ./cmd/cli ./internal/proxy`, `timeout -k 350s -s SIGKILL 350s make test` (Go total coverage 100.0%, Python 20 passed), `timeout -k 180s -s SIGKILL 180s make build`, `timeout -k 350s -s SIGKILL 350s make lint`, `timeout -k 350s -s SIGKILL 350s make ci` (Go total coverage 100.0%, Python 20 passed), `timeout -k 30s -s SIGKILL 30s git diff --check`, and a raw-SQL/direct-SQL scan.
- [x] [F004] (P1) Make packaged management DB dialect and DSN configurable through expandable config variables.
  ### Summary
  The packaged `configs/config.yml` should expose management DB dialect and DB location keys as expandable environment-backed values, while disabled management mode should not require operators to define management DB variables.
  ### Product contract
  1. `configs/config.yml` contains `management.database_dialect` and `management.database_dsn`.
  2. Those two values are supplied through config placeholders so deployment profiles can set them from environment.
  3. Placeholder defaults support disabled management mode without requiring DB environment variables.
  4. Existing missing-placeholder failures remain strict for required fields without defaults.
  ### Acceptance Criteria
  1. Add config placeholder support for `${NAME:-default}` values.
  2. Add packaged `configs/config.yml` management DB keys using expandable variables.
  3. Add CLI coverage proving placeholder defaults expand and missing non-default placeholders still fail.
  4. Preserve GORM/no-raw-SQL constraints.
  ### Resolution
  Added `${NAME:-default}` placeholder expansion in the config loader while preserving strict `config_placeholder_missing` failures for placeholders without defaults. Updated `configs/config.yml` to include a disabled management block with `management.database_dialect: "${LLM_PROXY_MANAGEMENT_DATABASE_DIALECT:-postgres}"` and `management.database_dsn: "${LLM_PROXY_MANAGEMENT_DATABASE_DSN:-}"`, so deployment profiles can supply DB dialect/path through environment variables and disabled management mode does not require management DB variables. README and provider-routing docs now document the env-backed DB placeholders. Added CLI coverage for non-empty placeholder defaults, empty placeholder defaults, packaged config loading without management DB env vars, and existing strict missing-placeholder behavior. Validation passed with `timeout -k 240s -s SIGKILL 240s go test -count=1 ./cmd/cli`, `timeout -k 350s -s SIGKILL 350s make test` (Go total coverage 100.0%, Python 20 passed), `timeout -k 180s -s SIGKILL 180s make build`, `timeout -k 350s -s SIGKILL 350s make lint`, `timeout -k 350s -s SIGKILL 350s make ci` (Go total coverage 100.0%, Python 20 passed), `timeout -k 30s -s SIGKILL 30s git diff --check`, and a raw-SQL/direct-SQL scan.
- [x] [F005] (P1) Remove placeholder default syntax and source the SQLite management DB path from `.env`.
  ### Summary
  Config placeholders should stay strict and simple. The packaged config should refer to environment-backed management DB values, and the local `.env` profile should provide the SQLite dialect and file path.
  ### Product contract
  1. `${NAME:-default}` syntax is not supported by the config loader.
  2. Missing placeholders without a matching `.env` or process environment value fail with `config_placeholder_missing`.
  3. `configs/config.yml` uses plain `${LLM_PROXY_MANAGEMENT_DATABASE_DIALECT}` and `${LLM_PROXY_MANAGEMENT_DATABASE_DSN}` placeholders.
  4. The local `configs/.env` profile provides SQLite management DB values.
  5. Management DB dialect has no implicit fallback; management mode requires an explicit supported dialect.
  ### Acceptance Criteria
  1. Remove default placeholder expansion from the loader.
  2. Add CLI coverage proving default placeholder syntax is rejected.
  3. Add CLI coverage proving the packaged config loads when `.env` provides SQLite DB variables.
  4. Remove the implicit management DB dialect default.
  5. Preserve GORM/no-raw-SQL constraints.
  ### Resolution
  Removed `${NAME:-default}` expansion from the config loader and kept placeholder handling strict to plain `${NAME}` values. `configs/config.yml` now uses `${LLM_PROXY_MANAGEMENT_DATABASE_DIALECT}` and `${LLM_PROXY_MANAGEMENT_DATABASE_DSN}` directly, the ignored local `configs/.env` profile provides `sqlite` plus the local management database file path, and `.gitignore` excludes generated `configs/*.sqlite` files. Removed the implicit Postgres fallback for `management.database_dialect`; management mode now requires an explicit `postgres` or `sqlite` value. Added CLI coverage proving default placeholder syntax fails with `config_placeholder_missing` and proving the packaged config loads when `.env` supplies SQLite management DB values. README and provider-routing docs now describe strict `.env`-backed management DB placeholders without default syntax or dialect fallback. Validation passed with `timeout -k 240s -s SIGKILL 240s go test -count=1 ./cmd/cli ./internal/proxy`, `timeout -k 350s -s SIGKILL 350s make test` (Go total coverage 100.0%, Python 20 passed), `timeout -k 180s -s SIGKILL 180s make build`, `timeout -k 350s -s SIGKILL 350s make lint`, `timeout -k 350s -s SIGKILL 350s make ci` (Go total coverage 100.0%, Python 20 passed), `timeout -k 30s -s SIGKILL 30s git diff --check`, `git check-ignore -v configs/llm-proxy-management.sqlite`, and a raw-SQL/direct-SQL scan.
- [x] [F006] (P1) Split the self-service management frontend onto GitHub Pages and keep llm-proxy as an API backend.
  ### Summary
  The management browser app should be served from GitHub Pages/CDN instead of being served by the Go backend. llm-proxy remains the API/proxy backend and exposes only API/config endpoints needed by the static frontend. Production uses split origins: frontend on `https://llm-proxy.mprlab.com`, backend on `https://llm-proxy-api.mprlab.com`, and TAuth on `https://tauth-api.mprlab.com`.
  ### Product contract
  1. The static frontend lives in a GitHub Pages-ready directory with no server-side template injection or Go embed dependency.
  2. `https://llm-proxy-api.mprlab.com/config-ui.yaml` is the API-served browser-facing MPR UI config that points at browser-reachable TAuth endpoints.
  3. Browser management API calls and MPR UI config fetches go to the configured backend origin, not same-origin `/api/management`.
  4. The backend allows credentialed browser requests only from the configured management frontend origin and returns `401` for unauthenticated management API calls.
  5. Public proxy examples generated by the UI point at the backend origin.
  6. The Go backend does not serve the management frontend HTML/assets in management mode.
  7. Required production setup is documented: DNS for GitHub Pages frontend, DNS/reverse proxy for the API backend, TAuth tenant origin/cookie settings, and GitHub Pages custom domain settings.
  ### Acceptance Criteria
  1. Move the management frontend to a static Pages directory, including `index.html` and assets, with no static `config-ui.yaml` in the Pages artifact.
  2. Render the API `config-ui.yaml` URL into `index.html`; the API-served YAML carries `llmProxy.managementApiOrigin`, `llmProxy.proxyOrigin`, and the MPR UI/TAuth environment config.
  3. Update frontend API client and usage snippets to use split-origin config with `credentials: "include"`.
  4. Add backend CORS support for `/config-ui.yaml` and `/api/management/*` using the configured frontend origin.
  5. Remove Go serving of management HTML/assets while keeping backend `/api/management/*` and proxy endpoints.
  6. Add black-box HTTP coverage for API-only management mode, CORS preflight/credentials headers, and absence of backend-served management frontend.
  7. Document the exact DNS/configuration/GitHub Pages steps.
  ### Resolution
  Moved the self-service management frontend into the GitHub Pages-ready `site/` directory with static `index.html`, pinned `mpr-ui` assets, generated `CNAME`, and `.nojekyll`. Added `.github/workflows/pages.yml` to publish a rendered static site through GitHub Pages and updated the PR workflow trigger for site/workflow changes. The browser management client consumes API-served `https://llm-proxy-api.mprlab.com/config-ui.yaml`, calls `https://llm-proxy-api.mprlab.com/api/management/*` with credentials, and generates proxy examples against the configured backend origin from that same YAML. The Go backend no longer serves management HTML/assets; it keeps `/config-ui.yaml`, `/api/management/*`, and proxy endpoints, and applies credentialed CORS only for `management.public_origin`. Added black-box management tests for API-only root behavior, API-served browser config, authenticated/unauthenticated management API behavior, and allowed/blocked CORS preflight handling. README and provider-routing docs now describe the required production setup: `llm-proxy.mprlab.com` on GitHub Pages, `llm-proxy-api.mprlab.com` on the MPR gateway/backend, the TAuth `llm-proxy` tenant/cookie settings, and the GitHub Pages custom domain/source settings. Validation passed with `timeout -k 240s -s SIGKILL 240s go test -count=1 ./cmd/cli ./internal/proxy`, static JS `node --check`, `timeout -k 350s -s SIGKILL 350s make test` (Go total coverage 100.0%, Python 20 passed), `timeout -k 180s -s SIGKILL 180s make build`, `timeout -k 350s -s SIGKILL 350s make lint`, `timeout -k 350s -s SIGKILL 350s make ci` (Go total coverage 100.0%, Python 20 passed), `timeout -k 30s -s SIGKILL 30s git diff --check`, raw-SQL/direct-SQL scan, stale backend-hosted UI symbol scan, and a local Playwright CLI preview of the static Pages app. The preview rendered the MPR UI/TAuth login shell; live sign-in/API calls remain pending production DNS plus a real `llm-proxy` TAuth tenant and Google OAuth client id.
  Review follow-up fixed production blockers from the split-origin branch: SQLite management persistence now uses a pure-Go GORM driver compatible with `CGO_ENABLED=0` release builds, the packaged disabled-management config no longer requires unused management DB placeholders, generated deployment metadata routes the backend through `llm-proxy-api.mprlab.com`, `make deploy` defaults to the backend-only gateway target, and the Pages config renders the production `llm-proxy` Google OAuth web client id from the authoritative `config.yml`.
  Review follow-up replaced tracked literal Pages config outputs with a Go CLI projection from the already-loaded `config.yml`. Browser-facing management fields now live under the `management` config block, the existing config loader remains the only environment-expansion gate, and the Pages workflow runs `llm-proxy --config configs/config.yml --render-site-output ...` to emit `CNAME` and a rendered `index.html` that points at API-served `/config-ui.yaml`. Google OAuth client exports are ignored so client secrets are not staged. Validation passed with `timeout -k 180s -s SIGKILL 180s go test -count=1 ./cmd/cli ./internal/proxy`, `timeout -k 350s -s SIGKILL 350s make go-test` (Go total coverage 100.0%), and `timeout -k 500s -s SIGKILL 500s make ci` (Go/Python lint clean, Go total coverage 100.0%, Python 20 passed).
  Review follow-up made browser/TAuth config consumable from `llm-proxy-api`: the backend now serves `/config-ui.yaml` from the validated management config with frontend-origin CORS, the Pages artifact keeps only `CNAME` plus the rendered `index.html` config URL, and the static frontend parses llm-proxy runtime origins from the same API-served YAML before bootstrapping `mpr-ui` and Alpine. The Pages workflow no longer receives production backend secrets; it uses non-sensitive placeholders for backend-only config fields during render. The renderer rejects public origins with ports before writing `CNAME` and removes any stale copied browser config files from the artifact. Validation passed with `timeout -k 30s -s SIGKILL 30s node --check site/assets/llm-proxy/js/app.js`, `timeout -k 30s -s SIGKILL 30s node --check site/assets/llm-proxy/js/core/backendClient.js`, `timeout -k 30s -s SIGKILL 30s node --check site/assets/llm-proxy/js/core/mprShell.js`, `timeout -k 240s -s SIGKILL 240s go test -count=1 ./cmd/cli ./internal/proxy`, `timeout -k 350s -s SIGKILL 350s make go-test` (Go total coverage 100.0%), `timeout -k 30s -s SIGKILL 30s git diff --check`, raw-SQL/direct-SQL scan, stale static-config scan, and `timeout -k 500s -s SIGKILL 500s make ci` (Go/Python lint clean, Go total coverage 100.0%, Python 20 passed).
  Final single-file follow-up collapsed the remaining two-config-file design: `/llm-proxy-config.json` is no longer routed or generated, `/config-ui.yaml` now includes `llmProxy.managementApiOrigin` and `llmProxy.proxyOrigin`, the rendered Pages `index.html` only carries the API YAML URL, and frontend code parses the same YAML before loading MPR UI. Validation passed with `timeout -k 30s -s SIGKILL 30s node --check site/assets/llm-proxy/js/app.js`, `timeout -k 30s -s SIGKILL 30s node --check site/assets/llm-proxy/js/core/backendClient.js`, `timeout -k 30s -s SIGKILL 30s node --check site/assets/llm-proxy/js/core/mprShell.js`, `timeout -k 240s -s SIGKILL 240s go test -count=1 ./cmd/cli ./internal/proxy`, `timeout -k 350s -s SIGKILL 350s make go-test` (Go total coverage 100.0%), `timeout -k 30s -s SIGKILL 30s git diff --check`, raw-SQL/direct-SQL scan, stale-contract scan with no active docs or route symbols for the removed JSON contract, and `timeout -k 500s -s SIGKILL 500s make ci` (Go/Python lint clean, Go total coverage 100.0%, Python 20 passed).
  Review follow-up hardened management mutations and defaults validation: authenticated unsafe `/api/management/*` requests now reject non-public `Origin` values and non-JSON content types, the static client sends JSON content type for secret generate/revoke mutations, and defaults updates validate normalized dictation defaults instead of accepting blank API fields that silently normalize to OpenAI. Added black-box HTTP coverage for blocked wrong-origin/simple mutation requests and DeepSeek-only blank dictation defaults. Validation passed with `go test -count=1 ./internal/proxy -run 'TestManagement(StaticPagesAndUnauthenticatedAPI|RejectsInvalidSessionsAndRequests|DatabasePersistenceAndOpenFailures|GeneratedSecretRoutesWithTenantProviderKey)'`, `go test -count=1 ./cmd/cli ./internal/proxy`, static JS `node --check`, `make go-test`, `make ci`, and `git diff --check`.
- [x] [F007] (P1) Move management settings into an avatar-menu modal and make the dashboard usage-focused.
  Goal:
  Keep the authenticated management experience self-service, but make the first screen a usage dashboard with graphs while moving the current key, provider, default-routing, and request-example controls into a large Settings modal opened from the authenticated avatar dropdown before Sign Out.
  Requirements:
  - Keep the split-origin Pages frontend plus backend-only `/api/management/*` contract.
  - Keep using the shared `<mpr-header>` and `<mpr-user>` components; add Settings through the `mpr-user` menu item contract rather than forking `mpr-ui`.
  - The Settings modal contains the current dashboard controls: generated tenant secret actions, tenant facts, one-time secret display/copy, routing defaults, usage examples, and provider API key management.
  - The main dashboard shows tenant usage summaries and graphs for managed-tenant requests, including text/dictation request counts, status counts, token totals when available, provider/model breakdown, and recent daily buckets.
  - Usage persistence must be tenant-isolated and must not store prompts, model responses, uploaded audio names, provider API keys, generated secrets, or raw TAuth identity payloads.
  - Persist usage through the existing GORM management store only; do not add raw SQL or a second persistence path.
  - Preserve existing proxy response behavior, token usage headers, management auth, CORS, and generated-secret routing.
  Deliverables:
  - Add a managed usage event store, recorder, authenticated usage-summary API, and black-box HTTP coverage.
  - Refactor the static management UI into dashboard, modal settings, and usage chart pieces while keeping user-facing strings in `site/assets/llm-proxy/js/constants.js`.
  - Add Settings as an avatar dropdown item before Sign Out and wire it to open the modal.
  - Add browser-visible coverage for dashboard charts and the avatar-menu Settings modal.
  - Update README/docs for the new dashboard/settings/usage contract.
  Validation:
  - Run focused Go management tests for usage recording, aggregation, and cross-user isolation.
  - Run static JS syntax checks for edited browser modules.
  - Run the browser test path covering Settings menu/modal behavior and dashboard graph rendering.
  - Run `timeout -k 350s -s SIGKILL 350s make ci` before marking the issue resolved.
  ### Resolution
  Added managed usage-event persistence through the existing GORM management store and exposed authenticated `GET /api/management/usage` summaries with 30-day totals, daily buckets, provider/model/status breakdowns, latency, and normalized token counts. Managed proxy text and dictation requests now record usage metadata for generated-secret tenants without storing prompts, responses, audio names, provider keys, generated secrets, or raw TAuth payloads; persistence failures are logged without changing proxy responses. Refactored the static management UI so the authenticated first screen is a usage dashboard with metric cards, SVG request/token graphs, and provider/model breakdowns. Moved tenant access, generated-secret controls, routing defaults, request examples, and provider key management into a large Settings modal opened by a `Settings` item inserted into the shared `<mpr-user>` avatar dropdown before `Sign out`. Added frontend usage presentation helpers, backend/management usage tests, internal usage edge coverage, a pinned Playwright browser harness, frontend syntax checking, CI Node/Playwright setup, and docs for the new usage/settings contract. Validation passed with `go test -count=1 ./internal/proxy -run 'TestManagement(UsageSummaryRecordsManagedProxyRequests|GeneratedSecretRoutesWithTenantProviderKey|GeneratedSecretSupportsDictationAndRejectsMultipartProviderKeys)'`, `make go-test` (Go total coverage 100.0%), `make frontend-lint`, `make frontend-test` (2 Playwright tests passed), `npm install` with 0 audit vulnerabilities after dependency pin updates, and `make ci` (Go/Python/frontend lint clean, Go total coverage 100.0%, Python 20 passed, Playwright 2 passed).
- [x] [F008] (P2) Make the management dashboard and Settings modal more compact.
  Goal:
  Tighten the F007 dashboard/modal visual treatment to match the compact operator-facing style in sibling `../ISSUES.md`.
  Requirements:
  - Preserve the dashboard/settings information architecture and behavior.
  - Keep the MPR dark, compact, workbench-like style: narrower shell, smaller type, tighter spacing, thin borders, restrained shadows, and compact controls.
  - Use the sibling ISSUES.md app as inspiration without copying its issue-tracker layout or semantics.
  - Keep desktop and mobile layouts readable without overlapping text or controls.
  Validation:
  - Run frontend syntax checks.
  - Run the browser test path covering dashboard rendering and Settings modal behavior.
  - Run whitespace checks.
  ### Resolution
  Retuned the management UI stylesheet toward the compact `../ISSUES.md` operator style: 960px centered shell, 15px base type, charcoal MPR token palette, flatter panels, 6px borders, tighter grid gaps, shorter usage metric cards, lower chart height, smaller headings, denser forms/buttons, compact provider cards, and a tighter Settings modal. Preserved the F007 dashboard/modal behavior and information architecture. Validation passed with `make frontend-lint`, `make frontend-test` (2 Playwright tests passed), and `git diff --check`.
- [x] [F009] (P1) Add administrator visibility for all managed users.
  Goal:
  Give configured administrators a management dashboard view across all managed users without exposing raw provider API keys or generated tenant secrets.
  Requirements:
  - Add administrators as normalized email addresses in `management.admin_emails`, populated from environment placeholders in public config.
  - Treat admin status as a management-session property derived from the authenticated TAuth email and validated config.
  - Show an Admin menu option only to administrators.
  - Admins can list managed users and see tenant facts plus 30-day usage summaries for each user.
  - Admin responses must not include provider API keys, masked provider key values, generated tenant secrets, secret digests, prompts, responses, audio names, or transcripts.
  - Non-admin authenticated users must receive `403` from admin-only APIs.
  Deliverables:
  - Extend management config parsing, validation, and docs for `management.admin_emails`.
  - Add an admin-only management API under `/api/management/admin/users`.
  - Add backend black-box coverage for admin access, non-admin rejection, and no key leakage.
  - Add a compact admin browser view reachable from the avatar menu only for admins.
  - Add browser-visible coverage for the Admin menu item and all-users dashboard.
  Validation:
  - Run focused Go management tests.
  - Run frontend syntax checks for edited browser modules.
  - Run the browser test path covering the admin menu/dashboard.
  ### Resolution
  Added `management.admin_emails` as the config-owned administrator list, validates configured email values at startup, derives `user.is_admin` from the validated TAuth session email, and keeps public config on environment placeholders for admin addresses. Added admin-only `GET /api/management/admin/users`, returning all managed users with tenant facts and 30-day usage summaries while excluding provider API keys, masked key strings, generated secrets, secret digests, prompts, responses, audio names, and transcripts. Non-admin authenticated sessions receive `403`. The static Pages UI now inserts an `Admin` avatar-menu item only for admin profiles and renders a compact all-users dashboard. README and provider-routing docs describe the admin contract.
  Validation passed with `timeout -k 180s -s SIGKILL 180s go test -count=1 ./internal/proxy -run 'TestManagement(AdminUsersDashboard|UsageSummaryRecordsManagedProxyRequests|ConfigurationValidationRequiresBackendAuthFields)'`, `timeout -k 180s -s SIGKILL 180s go test -count=1 ./cmd/cli -run 'TestRootCommand|TestRender'`, `timeout -k 240s -s SIGKILL 240s go test -count=1 ./internal/proxy`, `timeout -k 30s -s SIGKILL 30s npm run frontend:lint`, `timeout -k 180s -s SIGKILL 180s npm run frontend:test -- management-ui.spec.js`, and `git diff --check`.
- [ ] [F012] (P2) Add GPT 5.6 to the list of supported OpenAI models including the level of efforts.
- [ ] [F013] (P2) Add selectable usage-dashboard time intervals.
  Goal:
  Let signed-in users filter the complete usage dashboard through one compact interval control offering `ALL`, `30 days`, `7 days`, and `1 day`.
  Requirements:
  - Present the options in the exact order `ALL`, `30 days`, `7 days`, and `1 day`, with `30 days` selected when the authenticated dashboard first loads.
  - Add one canonical management API query contract: `GET /api/management/usage?interval=all|30d|7d|1d`. The browser must always send the selected interval, and the HTTP edge must reject a missing or unknown interval with `400 Bad Request`; do not preserve a second fixed-30-day request path.
  - Define `all` as every retained usage event owned by the authenticated managed tenant. Define `1d`, `7d`, and `30d` as trailing 24-hour, 7-day, and 30-day windows ending at one server-captured request timestamp.
  - Replace the fixed `period_days` and `daily` response assumptions with one interval-aware summary contract that identifies the selected interval, declares its bucket unit, and returns ordered chart buckets. Use hourly buckets for `1d`, daily buckets for `7d` and `30d`, and daily buckets from the earliest retained event through the current day for `all`; return an empty bucket series when no usage exists. Do not encode `all` as zero days or retain parallel legacy response fields.
  - Apply one selected-interval snapshot consistently to request and token totals, success rate, provider count, request and token charts, provider usage, and model usage.
  - Refresh must preserve and reload the currently selected interval. Interval controls and Refresh must prevent concurrent selections from applying responses out of order or leaving the active control inconsistent with the displayed data.
  - A failed interval load must clear the selected interval's dashboard data and show the existing explicit usage error state rather than leaving stale metrics from another interval visible.
  - Keep finite-range filtering at the database boundary and keep all queries tenant-isolated. Preserve the current usage-event privacy contract: prompts, request bodies, responses, audio names, transcripts, tenant secrets, provider API keys, and raw TAuth identity payloads are never returned or added to usage records.
  - Render the interval chooser as an accessible, keyboard-operable control with an explicit active state, disabled loading state, and compact desktop/mobile layout consistent with the existing MPR dashboard.
  - Keep the administrator all-users dashboard on its existing 30-day contract; adding administrator interval selection is outside this feature.
  Deliverables:
  - Add a validated usage-interval domain type and interval-aware management API, database query, aggregation, and generic chart-bucket response contract.
  - Add the dashboard interval control, selected-interval state, interval-aware backend client request, responsive styling, and generic chart presentation.
  - Update frontend types, user-facing copy, README, and provider-routing documentation to describe the selectable usage ranges and their exact semantics.
  Validation:
  - Add black-box management HTTP coverage for `all`, `30d`, `7d`, and `1d`, exact boundary inclusion/exclusion, tenant isolation, the default UI request, and missing or invalid interval rejection.
  - Add database-boundary coverage proving finite intervals do not load older rows and `all` reads the authenticated tenant's complete retained history.
  - Add Playwright coverage proving each option requests the canonical interval, becomes visibly active, updates every dashboard surface from one response, keeps its selection on Refresh, clears stale data on failure, prevents response-order races, and remains usable at desktop and mobile widths.
  - Run the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci` pair for the implementation, with the final run occurring after the last code edit.

- [ ] [F014] (P1) {B036,I025,F013} Support multiple isolated tenants per managed user.
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

- [x] [F010] (P1) Add Meta Model API and Muse Spark 1.1 as a supported text provider.
  ### Summary
  Meta launched Muse Spark 1.1 in public preview through Meta Model API on July 9, 2026. The official developer contract exposes the exact `muse-spark-1.1` model through an OpenAI-compatible Chat Completions API at `https://api.meta.ai/v1` with bearer-token authentication. llm-proxy should expose that verified text contract as one canonical provider without claiming unsupported proxy capabilities.
  ### Acceptance Criteria
  1. Add canonical provider selector `meta` with no aliases, base URL `https://api.meta.ai/v1`, credential placeholder `${MODEL_API_KEY}`, and configured default model `muse-spark-1.1`.
  2. Route `GET /`, compatibility `POST /`, and canonical `POST /v2` text requests through the shared OpenAI-compatible Chat Completions adapter with bearer authentication and normalized response/usage handling.
  3. Keep Meta dictation and llm-proxy web search unsupported; reject those capabilities at the existing HTTP edge without fallback transports or alternate model IDs.
  4. Surface Meta in registry-driven management profiles, provider key/model/system-prompt settings, generated request examples, and encrypted-at-rest managed routing.
  5. Add Meta to the dynamic live-provider smoke harness using `MODEL_API_KEY`, while keeping paid live calls outside `make ci` and conditional on credential availability.
  6. Update packaged config, env sample, README/provider-routing documentation, and generator-owned public provider resources from their canonical sources.
  7. Add black-box config, routing, management, and browser coverage for the Meta selector, default model, upstream path/auth/payload, normalized response usage, missing credential, and unsupported capability behavior.
  8. Pass repository CI with the existing 100% aggregate Go coverage gate; run a live Meta smoke only when a local credential is available.
  ### Resolution
  Added the canonical `meta` provider with `${MODEL_API_KEY}`, `https://api.meta.ai/v1`, and the sole configured `muse-spark-1.1` model. Meta text requests use the shared OpenAI-compatible Chat Completions adapter across `GET /`, compatibility `POST /`, and canonical `POST /v2`, with bearer authentication, normalized usage, and the current `max_completion_tokens` upstream field; there are no provider aliases, alternate transports, or fallbacks. The registry rejects Meta dictation and proxy `web_search`, management profiles expose the exact text-only capability and support encrypted tenant keys/defaults/system prompts/generated examples, and public requests reject Meta credential fields. Packaged config, env sample, the conditional live-provider harness, README/provider-routing docs, generator sources, and generated public resources now carry the same contract. Black-box config, routing, managed-encryption, and browser tests cover the integration. Validation passed with `make go-test` (aggregate Go coverage 100.0%), `make ci` (Go/Python/frontend lint and tests; Python 20 passed; Playwright 11 passed), deterministic `node scripts/generate_seo_resources.mjs` (45 pages), `bash -n scripts/test_live_providers.sh`, no-credential live-harness skip and explicit-Meta rejection checks, an authenticated Meta-only live smoke against `muse-spark-1.1` (`200 OK` with the expected `OK` response), and a scoped `git diff --check`. The locally supplied `MUSE11_API_KEY` was mapped to the canonical `MODEL_API_KEY` only in the smoke process without printing or persisting the secret.

- [x] [F011] (P1) Migrate the legacy global token to its authenticated user account.
  Goal:
  Preserve the existing llm-proxy client token while transferring its tenant, provider settings, and usage history from the unowned legacy static-config identity to the authenticated account configured by email.
  Requirements:
  - Configure exactly one bounded migration with a legacy tenant id and normalized owner email; keep the real personal email in deployment configuration rather than tracked source.
  - On the matching account's first authenticated management request, atomically replace the `static-config:<tenant-id>` owner with the verified TAuth subject and current profile identity.
  - Preserve the existing tenant id, secret digest, defaults, creation timestamp, provider settings, and usage events so the same raw token continues to work and historical usage appears on that account's dashboard.
  - Re-encrypt every migrated provider API key because provider-key ciphertext is bound to the owning user id.
  - Reject migration for non-matching emails and fail explicitly when a conflicting destination account already exists.
  - Stop creating static-config tenant rows in management mode and reject any remaining unowned static-config tenant during proxy authentication.
  - Keep static non-management deployments separate; this migration changes only the management-mode database ownership contract.
  Deliverables:
  - Add validated migration config and packaged environment-contract coverage.
  - Add one transactional GORM ownership operation without raw SQL or a second persistence path.
  - Add black-box HTTP coverage for first-login claim, unchanged-token routing, historical/new usage visibility, non-owner isolation, one-time behavior, and pre-claim legacy-token rejection.
  - Update README, provider-routing documentation, and generator-owned resource content to state that management-mode tokens are user-owned and the old static-config import path is retired.
  Validation:
  - Run focused management and config-loader tests.
  - Run `timeout -k 350s -s SIGKILL 350s make ci`.
  - Run `timeout -k 30s -s SIGKILL 30s git diff --check`.
  ### Resolution
  Replaced the management-mode static-config importer with one explicit, bounded ownership claim configured by legacy tenant id and normalized deployment-owned email. Before claim, every remaining `static-config:*` token is rejected; the matching account's first verified TAuth management session performs one GORM transaction that rekeys the tenant to the TAuth subject, preserves the tenant id, secret digest, defaults, creation time, provider settings, and all usage events, re-encrypts provider keys for the new owner id, and fails conflicts without partial writes. Management mode now rejects config tenants and global provider API keys, resolves usage ownership by preserved tenant id to prevent stale in-process writes, and removes the obsolete importer marker table through GORM. Packaged config, local environment guidance, provider-routing docs, SEO generator sources, generated resource pages, and the live-provider smoke harness now reflect the forward-only user-owned model; the harness builds an explicit static-mode fixture and has a non-paid authenticated-routing preflight in CI. Black-box coverage proves pre-claim rejection, non-owner isolation, first-login claim, unchanged-token routing, historical/new usage visibility, provider-key continuity, idempotent reload, and destination conflict rollback. Validation passed with `make go-test` (aggregate Go coverage 100.0%), `make ci` (Go/Python/frontend lint and tests; Python 20 passed; Playwright 11 passed; live-harness preflight passed), deterministic `node scripts/generate_seo_resources.mjs` (45 pages), `bash -n scripts/test_live_providers.sh`, the no-credential live-smoke skip, and `git diff --check`. Production activation remains an ordered rollout: deploy the current image, drain every old instance before owner sign-in, verify the unchanged token and dashboard history, then remove the temporary migration mapping.

- [x] [F015] (P1) Let application users change the model through reloadable client profiles.
  Goal:
  Let an application integrate an LLM Proxy client once, then let each of its
  users change the provider/model pair from the application without a source
  change, rebuild, restart, or deployment for each selection. This is distinct
  from a managed LLM Proxy tenant owner changing that tenant's routing default.

  Requirements:
  - Define one canonical, application-owned JSON model-profile document shared
    by the Go library, the installable Go CLI, and the Python package. The
    document contains exactly a `provider` and `model` pair; it never contains a
    tenant secret, upstream provider credential, or TAuth material.
  - Add an explicit profile path to the Go `ConfigInput`/`Config`, Go CLI, and
    Python `ClientConfig`. Each outbound v2 request reads the current profile so
    a completed application setting update affects the next request without
    recreating the client process.
  - Treat the profile as the sole provider/model source when configured. Reject
    a request-level model, a configured provider override, or a base-URL
    provider/model query that competes with it; do not silently prefer one
    source or retain a stale parsed profile.
  - Validate profile JSON and require nonblank provider and model identifiers at
    the client edge. The proxy remains the authority that validates the exact
    configured provider/model pair and returns its existing stable public error.
  - Require applications to publish profile updates atomically. If the client
    cannot read or validate the current profile, fail that request with explicit
    context; do not fall back to the previous profile, a request literal, a
    tenant default, or a provider default.
  - Preserve the current model-omitting path when no profile path is configured:
    it continues to delegate to the authenticated tenant default or selected
    provider default. Document this as a separate, mutually exclusive contract.
  - Support per-application-user selection by allowing each client instance to
    use that user's profile path. Application identity, authorization, storage,
    UI, and atomic profile publication remain application-owned.

  Deliverables:
  - Add the shared profile schema, Go/Python profile loaders, Go CLI
    `--model-profile` surface, edge validation, error types, and public API
    documentation.
  - Update Go and Python examples to show model omission for tenant-owned
    defaults and a separate app-user profile integration.
  - Do not add a second config format, an implicit environment alias, profile
    caching, a compatibility parser, or a best-effort resolution path.

  Validation:
  - Add black-box Go library, Go CLI, and Python client scenarios that send with
    profile A, atomically replace it with profile B, and prove the next request
    carries exactly B without client recreation.
  - Prove malformed, incomplete, unreadable, and competing profile/request
    inputs fail before an HTTP call and never reuse an earlier model.
  - Prove an unknown or unsupported profile pair reaches the proxy's existing
    public validation boundary, while a valid pair routes to the configured
    provider/model.
  - Run the required baseline and final `timeout -k 350s -s SIGKILL 350s make ci`
    pair for the implementation, with the final run after the last code edit.
  Resolution:
  Added the single strict JSON `provider`/`model` profile contract to the Go
  library, installable Go CLI (`--model-profile`), and Python package. Each
  profile-backed v2 request rereads its application-owned path; valid atomic A
  to B replacements change the next request without retaining a stale profile.
  Competing request/config/base-URL sources and unreadable, malformed,
  incomplete, or unsupported profile documents fail explicitly without an HTTP
  fallback, while unknown pairs reach the proxy validation boundary. README and
  implementation documentation now distinguish tenant-owned model omission
  from application-user profile selection. The required baseline and final
  `timeout -k 350s -s SIGKILL 350s make ci` runs passed; the final gate included
  100.0% Go coverage, 29 Python tests, 16 Playwright UI tests, the management
  auth black-box test, 38 release tests, and the live-provider preflight.
  Follow-up review fixes reject invalid UTF-8 Go profile bytes before JSON
  decoding or HTTP and convert every ordinary Python profile-reader failure to
  `LLMProxyModelProfileError` with profile-path context; the final same gate
  passed with 100.0% Go coverage, 30 Python tests, 16 Playwright UI tests, the
  management auth black-box test, 38 release tests, and the live-provider
  preflight.

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
