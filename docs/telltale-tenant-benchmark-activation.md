# TellTale tenant benchmark activation

The authorized operation repairs the live dashboard, configures the existing
TellTale tenant, and measures Story quality, generation time, and recurring cost.
The selected weights are 50%, 15%, and 35%, respectively.

## Identity and boundary

- Proxy origin: `https://llm-proxy-api.mprlab.com`.
- Management page: `https://llm-proxy.mprlab.com/app/`.
- Existing tenant: `managed-13f5fc612c3fac74f219d0bd483286be`, named `TellTale`.
- Browser profile: `Vadym`; the existing shared session identified Vadym Tyemirov.
- Client inputs: the proxy origin and tenant key from TellTale's declared private
  environment. Provider credentials remain in proxy-owned account connections.

Provider discovery, verification, and generation must pass through LLM Proxy.
Reuse existing account connections and inspect their assignments before changes.
Configure any missing provider connection only in LLM Proxy. Retain the tenant
key, unrelated assignments, and current default route.

The reused authenticated account currently has 14 connections, including a
Gemini connection named `Default`. TellTale has three assignments: OpenAI,
Anthropic, and `Alibaba Singapore`. Attach the existing Gemini connection after
deployment; create only the missing Cerebras connection, named `TellTale
benchmark`. These observations do not verify the candidate models' paid access.
The Alibaba connection's visible API URL uses `ap-southeast-1`; its qualification
policy explicitly selects the catalog price region `singapore`.

## Prepared implementation

The dashboard's first configuration read received HTTP 429 with `Retry-After: 6`.
Caddy supplied credentialed CORS for the frontend and exposed `Retry-After`.
The application cached the rejected promise and never requested its account,
even after shared authentication and another configuration read succeeded.

The source correction evicts rejected configuration promises, honors the server
deadline, and coalesces bounded recovery after shared authentication, focus, or
reconnection. It cancels obsolete work on sign-out or disposal. Invalid
configuration remains terminal. The route rate limit remains unchanged.
Focused real-page recovery and existing authentication tests passed.

The [model qualification record](model-qualification-2026-11-16.md) identifies
source activation, controls, pricing tiers, and local protocol evidence.
TellTale's local `docs/story-provider-benchmark.md` defines the frozen Plot
workflow and quality-qualified score. Local preparation
passed for both Qwen configurations and the three budget candidates, with no
credentials, network calls, or datastores.

## Remaining execution gates

1. Use the explicit November 16 ordering decision: finish Proxy prerequisites,
   run Proxy full CI and deploy, then run the benchmark and final TellTale CI.
   The selected manifest's enabled CI remains intact. This is the scoped
   exception to the earlier whole-task CI timing instruction.
2. Prepare normal source delivery from the primary checkout. The current checkout
   has uncommitted changes on a working branch; the installed lifecycle requires
   the checked-out remote default-branch source. Preserve unrelated changes.
3. Apply the completed opt-in operator qualification guard for the existing
   tenant's account-owned connections before paid work. Its focused tests, race
   checks, static checks, and independent reviews pass. Every new guard function
   has 100% statement coverage in the focused profile. Ordinary text
   continuation and transport retries have no configured total attempt cap.
   Successful public usage alone does not enumerate earlier uncertain transport
   attempts. Hosted quote controls require a hosted access grant and do not
   apply to these account connections. The qualification policy uses separate
   durable receipts and leaves hosted billing records unchanged. Its cap is an
   uncached catalog list estimate, consistent with the historical experiment;
   provider invoices and credits remain separate observations.
4. Apply the approved release through `make deploy`. Verify the live dashboard,
   source/runtime catalog revision, existing tenant identity, connection ownership,
   and exact candidate routes. Qualify each operation before economic scoring.
5. Execute the predeclared screen and any qualified finalist confirmation through
   the existing tenant. Reconcile every attempt and preserve unknown charges as
   reservations. Review every completed output under blinded labels, then record
   raw quality, recurring cost, and qualified latency beside the score.
6. Finish analysis, documentation, independent review, cleanup, and the explicitly
   agreed final validation gates.

The cumulative provider-spend ledger is $0.644939 against the existing $5 cap.
The user clarified that this cap covers the Story experiment; the auxiliary
release-version decision is separate. Gix uses the same TellTale tenant through
LLM Proxy, with a task-scoped configuration that excludes direct-provider
credentials and preserves global user settings.
This preparation phase dispatched no paid generation. The prior local diagnostic
budget wrapper does not protect the deployed tenant; it must not be presented as
the guard for this operation. Source preparation and focused tests do not establish
production recovery, candidate availability, or a completed benchmark.
