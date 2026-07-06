# LLM Proxy 60-Day Social Media Campaign

Audience: developers, technical founders, internal-tool builders, and AI platform operators who need a cleaner way to route LLM and dictation traffic.

Cadence: two posts per day from 2026-07-06 through 2026-09-03.

Constraint: every post in the `Post` column is under 300 characters.

| Day | Date | Slot | Post |
| --- | --- | --- | --- |
| 1 | 2026-07-06 | AM | Teams keep leaking provider keys into scripts, notebooks, and browser apps. LLM Proxy keeps upstream API keys server-side and gives clients a tenant secret instead. |
| 1 | 2026-07-06 | PM | Switching from OpenAI to Gemini or Claude should not mean rewriting every caller. LLM Proxy lets each request pick a provider while keeping one simple HTTP contract. |
| 2 | 2026-07-07 | AM | Your app should not know every provider's payload quirks. LLM Proxy normalizes OpenAI, Claude, Gemini, Grok, DeepSeek, Qwen, Kimi, GLM, and more behind one proxy. |
| 2 | 2026-07-07 | PM | Long prompts do not belong in brittle query strings. LLM Proxy supports JSON POST bodies for large prompts and canonical chat transcripts. |
| 3 | 2026-07-08 | AM | Chat histories arrive out of order when clients stitch messages from events. LLM Proxy's `/v2` contract supports ordered messages before routing upstream. |
| 3 | 2026-07-08 | PM | Vendor-specific defaults create hidden behavior. LLM Proxy makes provider and model defaults explicit per tenant, per provider, and per request. |
| 4 | 2026-07-09 | AM | Teams often need one API for text and voice. LLM Proxy routes text generation and `/dictate` audio transcription through the same authenticated service boundary. |
| 4 | 2026-07-09 | PM | Client apps should never ask users for raw OpenAI, Anthropic, Gemini, or xAI keys. LLM Proxy stores provider credentials server-side and rejects them on public proxy requests. |
| 5 | 2026-07-10 | AM | Testing a cheaper model should be a request change, not a code fork. LLM Proxy lets callers choose `provider` and `model` without changing the client integration. |
| 5 | 2026-07-10 | PM | Upstream APIs return different token metadata. LLM Proxy normalizes request, response, and total token usage into common headers and JSON fields. |
| 6 | 2026-07-11 | AM | Your team wants AI access, but not shared secrets in Slack. LLM Proxy's management UI lets signed-in users generate their own proxy secrets. |
| 6 | 2026-07-11 | PM | Rotating a user-facing LLM secret should not touch provider keys. LLM Proxy stores generated tenant secrets separately and lets revocation block future calls. |
| 7 | 2026-07-12 | AM | A database backup should not expose raw provider credentials. LLM Proxy encrypts managed provider API keys at rest before persistence. |
| 7 | 2026-07-12 | PM | Security claims need precision. LLM Proxy protects stored provider keys from direct DB exposure while still decrypting them only on the runtime path that calls providers. |
| 8 | 2026-07-13 | AM | Teams lose time copying half-right curl commands. LLM Proxy's management UI shows copyable default and selected-provider examples for real client access. |
| 8 | 2026-07-13 | PM | A user should see useful examples before their first secret exists. LLM Proxy shows complete request examples with a generated-secret placeholder. |
| 9 | 2026-07-14 | AM | Admins need usage visibility without seeing secrets or prompts. LLM Proxy admin views summarize users, tenants, and 30-day usage without exposing sensitive content. |
| 9 | 2026-07-14 | PM | Teams need to know whether AI calls are succeeding. LLM Proxy records status, latency, provider, model, and token counts for managed tenants. |
| 10 | 2026-07-15 | AM | Usage dashboards often become prompt databases by accident. LLM Proxy tracks usage metadata without storing prompts, transcripts, responses, tenant secrets, or provider keys. |
| 10 | 2026-07-15 | PM | Your AI gateway should show cost signals, not just request counts. LLM Proxy includes 30-day request and token graphs plus provider and model breakdowns. |
| 11 | 2026-07-16 | AM | Browser apps need current config, not stale build-time files. LLM Proxy serves management runtime config from the backend at `/config-ui.yaml`. |
| 11 | 2026-07-16 | PM | Static sites should not carry live environment secrets. LLM Proxy keeps the management frontend static while the backend owns runtime config and API calls. |
| 12 | 2026-07-17 | AM | Some providers support web search and others do not. LLM Proxy exposes web search per request only when the selected OpenAI model is configured for it. |
| 12 | 2026-07-17 | PM | A provider without a configured key should fail clearly. LLM Proxy returns a service-level error when a selected non-default provider is unavailable. |
| 13 | 2026-07-18 | AM | Teams waste time debugging unknown model names after upstream calls. LLM Proxy validates provider model catalogs before routing requests. |
| 13 | 2026-07-18 | PM | Max-token mistakes can become expensive upstream calls. LLM Proxy rejects known provider-specific output limits at the request edge. |
| 14 | 2026-07-19 | AM | Claude, Gemini, and OpenAI-compatible APIs do not speak the same dialect. LLM Proxy maps a shared request into each provider's native or compatible contract. |
| 14 | 2026-07-19 | PM | A simple REST client should not stream, poll, or resume provider jobs. LLM Proxy owns provider polling and returns the final answer in one blocking response. |
| 15 | 2026-07-20 | AM | OpenAI background responses can tie client logic in knots. LLM Proxy handles stored background requests and server-side polling for callers. |
| 15 | 2026-07-20 | PM | Worker pools should protect upstream calls, not punish long polling sleeps. LLM Proxy separates provider HTTP concurrency from OpenAI polling waits. |
| 16 | 2026-07-21 | AM | When LLM traffic spikes, unlimited upstream calls become a reliability problem. LLM Proxy uses shared worker and queue controls to bound provider HTTP operations. |
| 16 | 2026-07-21 | PM | A full queue should be visible to callers. LLM Proxy returns clear overload behavior instead of silently dropping or hiding queued upstream work. |
| 17 | 2026-07-22 | AM | Dictation should not require a separate security model. LLM Proxy puts audio transcription behind the same tenant-secret boundary as text requests. |
| 17 | 2026-07-22 | PM | Different speech providers expect different transcription URLs and fields. LLM Proxy keeps those transport details server-side. |
| 18 | 2026-07-23 | AM | Some apps need plain text. Others need JSON, XML, or CSV. LLM Proxy supports multiple response formats from the same endpoint contract. |
| 18 | 2026-07-23 | PM | Token accounting should survive format changes. LLM Proxy keeps normalized usage headers even when the body is plain text, CSV, or XML. |
| 19 | 2026-07-24 | AM | AI client libraries should not expose every legacy endpoint. LLM Proxy's Go and Python clients use the canonical `/v2` messages contract. |
| 19 | 2026-07-24 | PM | CLI workflows need the same clean transport as apps. LLM Proxy includes an installable prompt client that sends canonical JSON messages. |
| 20 | 2026-07-25 | AM | Provider experiments often create scattered config drift. LLM Proxy keeps model catalogs and defaults in runtime config while provider transports stay code-owned. |
| 20 | 2026-07-25 | PM | Ops teams need config failures before traffic starts. LLM Proxy fails startup on unknown YAML keys, invalid catalogs, and missing required placeholders. |
| 21 | 2026-07-26 | AM | A typo in a default provider can break production later. LLM Proxy validates tenant provider and model defaults at startup. |
| 21 | 2026-07-26 | PM | Non-default providers should stay disabled until configured, not block every deploy. LLM Proxy allows blank optional provider keys while protecting active defaults. |
| 22 | 2026-07-27 | AM | Moving to self-service AI access should not mutate config files. LLM Proxy stores user signups, enabled providers, defaults, and secret digests in the management DB. |
| 22 | 2026-07-27 | PM | Generated secrets should be shown once, not retrievable forever. LLM Proxy returns them once and stores only SHA-256 digests. |
| 23 | 2026-07-28 | AM | Provider-specific system prompts belong with that provider, not in a global note. LLM Proxy stores model and prompt settings per managed provider. |
| 23 | 2026-07-28 | PM | Selected-provider requests should use the user's saved routing context. LLM Proxy can apply saved provider model and system prompt settings when the request omits them. |
| 24 | 2026-07-29 | AM | Teams need one place to edit provider key, model, and prompt. LLM Proxy's Settings modal gives each user a selected-provider editor. |
| 24 | 2026-07-29 | PM | "Remove key" should not be ambiguous. LLM Proxy makes provider key removal a clear provider-settings operation. |
| 25 | 2026-07-30 | AM | Product teams need AI access without teaching every app OAuth. LLM Proxy puts login and provider-key management in one TAuth-protected management UI. |
| 25 | 2026-07-30 | PM | Your public proxy should not double as a management website. LLM Proxy keeps `GET /` as a proxy endpoint and serves the UI separately as static Pages. |
| 26 | 2026-07-31 | AM | Split-origin deployments get messy when config is duplicated. LLM Proxy uses the API hostname for runtime config and proxy calls while Pages hosts the static UI. |
| 26 | 2026-07-31 | PM | An app should not ship a stale API hostname inside HTML. LLM Proxy's browser config is served by the running backend, not rendered into the Pages artifact. |
| 27 | 2026-08-01 | AM | Engineers need to know which model a request actually used. LLM Proxy includes provider and model metadata in JSON responses and usage events. |
| 27 | 2026-08-01 | PM | Provider migrations should not break callers that omit `model`. LLM Proxy resolves omitted models through tenant defaults or selected-provider defaults. |
| 28 | 2026-08-02 | AM | Plain prompts and chat transcripts should not require two products. LLM Proxy supports simple prompts, compatibility message bodies, and canonical `/v2` messages. |
| 28 | 2026-08-02 | PM | System instructions can collide when clients send both body prompts and role messages. LLM Proxy rejects ambiguous input before any upstream call. |
| 29 | 2026-08-03 | AM | Bad request bodies should not become provider bills. LLM Proxy validates missing prompts, empty messages, unsupported roles, and duplicate order values at the edge. |
| 29 | 2026-08-03 | PM | Provider-key leaks can hide in JSON or multipart forms. LLM Proxy rejects client-supplied provider key fields on public proxy requests. |
| 30 | 2026-08-04 | AM | Teams need one service boundary for different LLM vendors. LLM Proxy routes native OpenAI, Anthropic, and Gemini calls alongside OpenAI-compatible providers. |
| 30 | 2026-08-04 | PM | Your app should not hardcode provider base URLs everywhere. LLM Proxy keeps base URLs and transcription URLs in central runtime config. |
| 31 | 2026-08-05 | AM | A new provider model should not require a client release. LLM Proxy treats model IDs and metadata as config data. |
| 31 | 2026-08-05 | PM | Upstream providers change model lists faster than app release cycles. LLM Proxy lets operators update catalogs in config and restart the service. |
| 32 | 2026-08-06 | AM | Multi-provider routing can turn into branch soup. LLM Proxy uses provider selectors and aliases so callers can choose a route explicitly. |
| 32 | 2026-08-06 | PM | Default routes should be boring. LLM Proxy lets omitted-provider requests use the authenticated tenant default instead of duplicating provider flags everywhere. |
| 33 | 2026-08-07 | AM | AI errors are hard enough without vague status codes. LLM Proxy maps missing keys, bad inputs, rate limits, disabled providers, timeouts, and upstream failures clearly. |
| 33 | 2026-08-07 | PM | A timeout should not tell clients to guess the provider state. LLM Proxy returns a normal gateway timeout when the overall proxy deadline expires. |
| 34 | 2026-08-08 | AM | Provider polling should not leak into product code. LLM Proxy hides synchronous, background, and provider-specific polling behind one REST request. |
| 34 | 2026-08-08 | PM | Teams need predictable request limits. LLM Proxy enforces prompt and audio payload caps before routing upstream. |
| 35 | 2026-08-09 | AM | Audio uploads can be too large for casual handlers. LLM Proxy gives `/dictate` its own max input audio byte limit. |
| 35 | 2026-08-09 | PM | Dictation integrations should not guess form fields. LLM Proxy defines a clear multipart contract with `audio` as the file field. |
| 36 | 2026-08-10 | AM | Ad hoc AI gateways become hard to audit. LLM Proxy centralizes authentication, provider choice, request limits, and usage metadata in one service. |
| 36 | 2026-08-10 | PM | Each app should not rebuild provider usage reporting. LLM Proxy gives managed users a usage dashboard out of the box. |
| 37 | 2026-08-11 | AM | You should be able to trial DeepSeek for one workflow and Claude for another. LLM Proxy supports per-request provider selection under the same tenant secret. |
| 37 | 2026-08-11 | PM | Model comparison should not require five SDKs. LLM Proxy keeps the client-side integration stable while the backend routes to each provider. |
| 38 | 2026-08-12 | AM | Web search should be a controlled capability, not a magic flag. LLM Proxy checks request intent and model catalog support before enabling it. |
| 38 | 2026-08-12 | PM | Unsupported provider capabilities should fail before money is spent. LLM Proxy rejects invalid provider, endpoint, model, and web-search combinations early. |
| 39 | 2026-08-13 | AM | Your internal tools need AI, but your security team needs a smaller attack surface. LLM Proxy keeps upstream credentials away from clients. |
| 39 | 2026-08-13 | PM | Key management should not require every engineer to edit YAML. LLM Proxy's authenticated UI lets users save provider keys and generate access secrets. |
| 40 | 2026-08-14 | AM | Teams need provider choices without teaching users provider APIs. LLM Proxy exposes simple selectors while preserving provider-specific routing behind the service. |
| 40 | 2026-08-14 | PM | OpenAI-compatible providers still differ in setup and availability. LLM Proxy centralizes credentials, base URLs, defaults, and disabled-provider behavior. |
| 41 | 2026-08-15 | AM | Native Gemini requests should not force a Gemini-specific client in every app. LLM Proxy maps shared messages into Gemini `generateContent`. |
| 41 | 2026-08-15 | PM | Native Anthropic Messages should not require separate client plumbing. LLM Proxy handles Claude routing and output-token requirements server-side. |
| 42 | 2026-08-16 | AM | Transcripts, prompts, and model outputs are sensitive. LLM Proxy's managed usage events omit those contents while still preserving operational metrics. |
| 42 | 2026-08-16 | PM | Admin views should help operators without becoming a data leak. LLM Proxy admin usage summaries exclude provider keys, prompts, responses, audio, and transcripts. |
| 43 | 2026-08-17 | AM | A self-service AI portal should be useful after login, not just a key form. LLM Proxy opens to a usage dashboard and keeps settings one click away. |
| 43 | 2026-08-17 | PM | Client access docs should reflect the current provider and generated secret. LLM Proxy renders examples from live profile data. |
| 44 | 2026-08-18 | AM | Teams need local and hosted profiles to behave the same way. LLM Proxy's management config uses explicit placeholders and fails startup when required values are missing. |
| 44 | 2026-08-18 | PM | Static tenants and managed tenants can get tangled. LLM Proxy migrates legacy config tenants once, then makes database state authoritative in management mode. |
| 45 | 2026-08-19 | AM | Legacy config should not keep influencing runtime after migration. LLM Proxy records a migration marker and ignores obsolete config-file tenant credentials afterward. |
| 45 | 2026-08-19 | PM | New user signups should survive restarts. LLM Proxy persists managed tenants, provider settings, secret digests, and usage through GORM. |
| 46 | 2026-08-20 | AM | You should not need raw SQL for normal tenant management. LLM Proxy keeps management persistence behind GORM model APIs. |
| 46 | 2026-08-20 | PM | SQLite is useful for local management mode, Postgres for hosted mode. LLM Proxy supports both configured GORM dialects. |
| 47 | 2026-08-21 | AM | AI platform work gets brittle when config comes from flags, env, and files at once. LLM Proxy uses YAML as the service config and env only for placeholder expansion. |
| 47 | 2026-08-21 | PM | Operators need strict config loading, not surprise defaults. LLM Proxy rejects unknown YAML keys and missing required placeholders before serving traffic. |
| 48 | 2026-08-22 | AM | A provider model list is business data, not client code. LLM Proxy keeps provider model catalogs in config so apps can stay unchanged. |
| 48 | 2026-08-22 | PM | OpenAI web search should be model-aware. LLM Proxy enables the tool only for catalog entries marked with web-search support. |
| 49 | 2026-08-23 | AM | Teams need to cap outputs per request. LLM Proxy maps `max_tokens` to each provider's expected field and validates known limits. |
| 49 | 2026-08-23 | PM | Provider output limits should not surprise clients late. LLM Proxy checks configured Gemini and Claude token ceilings before the upstream call. |
| 50 | 2026-08-24 | AM | Product code should not care whether text comes from OpenAI Responses or chat completions. LLM Proxy normalizes the route behind a shared HTTP API. |
| 50 | 2026-08-24 | PM | Multi-provider AI should not erase caller-visible request metadata. LLM Proxy JSON responses echo caller-visible messages and usage in a consistent shape. |
| 51 | 2026-08-25 | AM | An AI gateway should protect both secrets and budgets. LLM Proxy combines server-side credentials, request caps, queue limits, and usage reporting. |
| 51 | 2026-08-25 | PM | Teams need to route audio and text through the same governance layer. LLM Proxy covers text generation and dictation with one tenant-secret model. |
| 52 | 2026-08-26 | AM | Integrations should start with curl, then graduate to clients. LLM Proxy supports direct REST calls, an installable Go CLI, a Go package, and a Python package. |
| 52 | 2026-08-26 | PM | AI callers should not decide whether a provider is native or compatible. LLM Proxy owns the adapter choice for OpenAI, Claude, Gemini, and compatible chat APIs. |
| 53 | 2026-08-27 | AM | Long-running model work should not force app teams to build polling loops. LLM Proxy holds the provider lifecycle server-side and returns the final response. |
| 53 | 2026-08-27 | PM | When an upstream provider rate-limits you, clients need a clear signal. LLM Proxy maps provider rate limits to a consistent public response. |
| 54 | 2026-08-28 | AM | Your app should not need a redeploy to move default routing. LLM Proxy lets operators change tenant and provider defaults in management data or runtime config. |
| 54 | 2026-08-28 | PM | Managed users should not wait on ops for every provider change. LLM Proxy lets them save provider keys, choose models, and set provider prompts in Settings. |
| 55 | 2026-08-29 | AM | Teams need exact examples for the current proxy origin. LLM Proxy generates request examples from the runtime `proxyOrigin` instead of hardcoded docs. |
| 55 | 2026-08-29 | PM | Copy-paste mistakes slow adoption. LLM Proxy provides copyable text, `/v2`, and dictation examples for default and selected-provider routes. |
| 56 | 2026-08-30 | AM | AI usage should be visible by provider and model, not trapped in logs. LLM Proxy summarizes managed tenant usage across daily, provider, model, and status buckets. |
| 56 | 2026-08-30 | PM | Internal AI tools need admin insight without raw content access. LLM Proxy shows tenant and usage facts while excluding prompts, responses, and secrets. |
| 57 | 2026-08-31 | AM | Provider choice should not be a permanent architecture decision. LLM Proxy lets you add, test, disable, or switch providers at the service boundary. |
| 57 | 2026-08-31 | PM | Teams need one clean path from prototype to production. LLM Proxy starts with simple HTTP calls and grows into managed users, dashboards, and provider controls. |
| 58 | 2026-09-01 | AM | Apps should not carry different auth rules for every LLM provider. LLM Proxy gives clients one tenant-secret contract while storing upstream credentials server-side. |
| 58 | 2026-09-01 | PM | A broken model ID should not become an upstream mystery. LLM Proxy validates model selections against the configured provider catalog. |
| 59 | 2026-09-02 | AM | The fastest way to clean up LLM sprawl is to move the boundary. LLM Proxy centralizes provider routing, secrets, request validation, and usage signals. |
| 59 | 2026-09-02 | PM | Need Claude for reasoning, Gemini for long context, and OpenAI for web search? LLM Proxy lets each request choose the provider that fits. |
| 60 | 2026-09-03 | AM | Your AI stack should be easy for clients and strict for operators. LLM Proxy gives clients one HTTP surface and gives operators config, limits, and visibility. |
| 60 | 2026-09-03 | PM | Scattered LLM integrations create security, cost, and maintenance drag. LLM Proxy turns them into one governed, multi-provider service boundary. |
