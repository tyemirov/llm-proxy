# ISSUES

Working backlog for this repository. Keep entries in the canonical ISSUES.md format described in `.mprlab/issues-md-format.md`.

## BugFixes

- [x] [B001] (P1) Gemini POST responses can return thought or partial text as successful output
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

## Improvements

## Maintenance

- [x] [M001] (P1) Consolidate repository runbook documents under `.mprlab/`
  ### Summary
  The repository had duplicate runbook and issue-tracker documents under `issues.md/`, `.mprl/`, and `.mprlab/`. Keep the active tracker and relevant recurring procedures under `.mprlab/`, then remove the old duplicate locations.

  ### Resolution
  Consolidated the current policy, planning, issue-format, and stack-guide documents under `.mprlab/`; kept `.mprlab/ISSUES.md` as the active tracker; carried forward recurring housekeeping runbooks from `issues.md/ISSUES.md`; updated stale runbook path references; and removed the duplicate `issues.md/` and `.mprl/` directories.

### Recurring (runbooks; keep open)

These entries are always-available procedures. Keep them `[ ]` so they remain runnable; when you run one, update a short `Last run:` line in the body and optionally link the PR or commit.

- [ ] [M400] (P2) Backlog housekeeping
  1. Validate `.mprlab/ISSUES.md` matches `.mprlab/issues-md-format.md`.
  2. Confirm user-facing consequences of recently closed work are documented in README, ARCHITECTURE, or PRD.
  3. Prune closed entries once documented.
  4. Merge duplicates and delete irrelevant issues.

- [ ] [M401] (P2) Polish open issues
  1. For each open issue, add missing context, dependencies, repro steps, acceptance criteria, and test expectations.
  2. Ensure each issue has a clear priority and concrete deliverables.

- [ ] [M402] (P2) Architecture and policy review
  1. Review the codebase against `.mprlab/POLICY.md` and stack guides.
  2. Record findings as new Maintenance issues, or close as "no action" if already covered.

## Features

- [ ] [F001] (P1) Add authenticated self-service API key and tenant secret management UI
  ### Summary
  Users need an authenticated browser UI where they sign in through the MPR/TAuth login surface, ask llm-proxy to create a new client key for them, bring their own upstream provider API keys for any supported provider, choose tenant defaults, and then use the service with the generated llm-proxy tenant secret. This should turn llm-proxy from an operator-provisioned static-tenant service into a self-service tenant onboarding surface without changing the public proxy request contract: clients still call `GET /`, JSON `POST /`, `POST /v2`, and `POST /dictate` with `key=<tenant secret>`, while upstream provider API keys stay server-side.

  ### MPR UI and authentication contract
  The UI must follow the `mpr-integration` `mpr-ui` and TAuth contracts:

  1. Serve `/config-ui.yaml` as the only browser-facing MPR UI auth config surface.
  2. Load a pinned `mpr-ui.css`, Google Identity Services, `js-yaml`, pinned `mpr-ui-config.js`, and a bundle marker with `data-mpr-ui-bundle-src`.
  3. Render the shared shell through `<mpr-header data-config-url="/config-ui.yaml">`, `<mpr-user>`, and `<mpr-footer>` rather than direct `tauth.js` loading or manual `tauth-*` attributes.
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

## Planning
*do not implement yet*
