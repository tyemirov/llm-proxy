# ISSUES

Working backlog for this repository. Keep it current and small. Use @issues-md-format.md for the canonical format.

- Status markers: `[ ]` open, `[!]` blocked (must include a `Blocked:` line), `[x]` closed.
- Hygiene: once a closed issue's consequences are reflected in code/tests and in user-facing docs, remove the entry from this file. Git history remains the record. (Recurring runbooks below are the exception: keep them open.)

## BugFixes

- [ ] [B001] (P0) Prepare a provider-routing plan using query-string parameters (Claude, Gemini, and others).
  ### Summary
  Create an implementation plan to extend `llm-proxy` so requests can select an LLM provider via query-string parameters (for example, OpenAI, Anthropic Claude, Google Gemini), rather than assuming OpenAI-only routing.
  
  ### Analysis
  The current service is documented as OpenAI-specific: `GET /` forwards to the OpenAI Responses API and `POST /dictate` uses OpenAI transcription models. A `model` query parameter already exists, but provider selection does not.
  
  A robust plan should define how provider choice interacts with existing behavior:
  - Backward compatibility for current OpenAI clients.
  - Parameter contract for provider selection (`provider`, `model`, and precedence rules).
  - Capability differences (for example, web search/tooling support varies by provider/model).
  - Configuration model for multiple API keys/secrets.
  - Error mapping and response format consistency across providers.
  - Impact on `/` and `/dictate` endpoints, including unsupported provider/endpoint combinations.
  - Documentation updates (README endpoint docs, model/provider capability table, and examples).
  
  The plan should be implementation-ready, with clear phases and test strategy aligned to this repository’s black-box/integration-first testing philosophy.
  
  ### Deliverables
  - A written implementation plan that includes:
  1. Proposed request contract for provider selection, including exact query parameters and defaults.
  2. Routing design that maps `(provider, model, endpoint)` to the correct upstream client.
  3. Configuration changes for provider credentials and runtime validation rules.
  4. Provider capability matrix (text generation, dictation/transcription, web search/tool use) and fallback behavior.
  5. Error-handling contract with HTTP status mapping for invalid provider/model/capability combinations.
  6. Test plan covering integration scenarios for OpenAI (regression) and at least one non-OpenAI provider path.
  7. Documentation plan listing required updates to README and any related docs.
  
  Acceptance criteria:
  1. The plan explicitly preserves current OpenAI behavior when `provider` is omitted.
  2. The plan defines unambiguous precedence/validation rules for `provider` and `model`.
  3. The plan identifies required code touchpoints (request parsing, routing layer, provider clients, docs, tests).
  4. The plan includes concrete test cases for success and failure paths (including unsupported combinations).
  5. The plan is detailed enough that implementation can proceed without additional requirement clarification.


## Improvements

## Maintenance

## Features

## Planning
*do not implement yet*

