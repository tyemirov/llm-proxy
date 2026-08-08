# LLM Proxy SEO Resource Cluster Report

Generated: 2026-08-08

## Repo Analysis Report

### Source Files Reviewed

| File | Type | Why reviewed | Key findings | Confidence |
|---|---|---|---|---|
| README.md | Required product docs | Primary product description, REST contract, config, public landing, management UI, clients, deployment, and security wording | LLM Proxy is a lightweight HTTP proxy for text and dictation providers with tenant-secret auth, server-side credentials, a public capability landing page, a TAuth management app at /app/, usage dashboards, provider routing, and strict config loading. | High |
| docs/implementation/provider-routing-plan.md | Implementation notes | Provider routing, config ownership, management mode, error contract, and adapter notes | Multi-provider routing, model catalogs, omitted-model behavior, split-origin management, usage storage, and provider-key security are implemented contracts. | High |
| docs/implementation/dictation-endpoint-plan.md | Implementation notes | Dictation endpoint contract | /dictate accepts multipart audio with key auth and returns JSON text; dictation routing is implemented for supported providers in the current README. | High |
| docs/marketing/social-media-60-day-campaign.md | Marketing copy | Existing audience and claim framing | Existing public claims emphasize server-side provider keys, multi-provider routing, dictation, usage dashboards, API-served config, and careful encrypted-at-rest wording. | Medium |

### Product Summary

- Product name: LLM Proxy
- Product category: Multi-provider LLM integration layer with direct HTTP, official clients, a public capability catalog, and a self-service management app.
- One-sentence description: LLM Proxy lets applications integrate once through one authenticated messages contract, then select supported providers and models while the proxy owns upstream credentials and provider-specific APIs.
- Primary users: AI-assisted builders, startups and product teams, institutional platform and engineering teams, and internal-tool developers.
- Secondary users: Managed end users who sign in, receive a client key, save provider keys, copy request examples, and inspect usage.
- Primary job-to-be-done: Keep one durable application integration while provider and model choice changes behind a validated routing boundary.
- Installation or usage model: Run the Go backend from config.yml; publish the static landing and authenticated app at /app/ from site/ to GitHub Pages; call GET /, POST /, POST /v2, or POST /dictate with key=<tenant secret>.
- Current maturity: Implemented repo contract with Go/Python/frontend validation and documented release/deploy workflows.

### Product Capabilities

| Capability | Description | Evidence source | Confidence | Current / roadmap / unclear | Safe for page copy? |
|---|---|---|---|---|---|
| Multi-provider text routing | Routes OpenAI Responses, Meta Muse Spark and other OpenAI-compatible providers, Anthropic Messages, Gemini Interactions, and Grok/xAI text. | README provider matrix, provider routing notes | High | Current | Yes |
| Dictation endpoint | Routes multipart audio through /dictate for supported dictation providers. | README dictation section, dictation plan | High | Current | Yes |
| Tenant-secret auth | Public proxy endpoints require key=<tenant secret>. | README REST and security sections | High | Current | Yes |
| Server-side provider credentials | Public requests must not send upstream provider keys; credentials stay server-side. | README security, provider routing notes | High | Current | Yes |
| TAuth management UI | Static noindex Pages app at /app/ with authenticated profile, provider key, generated secret, settings, usage, and admin views. | README management UI section | High | Current | Yes |
| Encrypted-at-rest managed provider keys | AES-GCM storage with base64 32-byte key and honest non-zero-knowledge wording. | README management UI section | High | Current | Yes, with caution wording |
| Usage dashboard | Selectable all-time, 30-day, 7-day, and 1-day usage summaries by request, token, provider, model, status, and time bucket. | README management UI section | High | Current | Yes |
| API-served runtime config | Browser config comes from backend /config-ui.yaml, not a static Pages config artifact. | README hosted split-origin section | High | Current | Yes |
| Bundled v2-only clients | Go package, Go CLI, and Python package send canonical /v2 messages for text. | README clients section | High | Current | Yes |
| Public-page telemetry | Public static pages load Google Analytics and LoopAware page-view scripts. | site/index.html, resource generator | High | Current | Yes, with privacy caveat |
| Worker/queue controls | server.workers limits upstream HTTP operations and queue_size limits pending operations. | README REST contract and config section | High | Current | Yes |

### Non-Capabilities, Limits, and Cautions

| Item | Why it matters | Evidence | Copywriting rule |
|---|---|---|---|
| No zero-knowledge guarantee | Backend decrypts provider keys to call upstream providers. | README management UI | Say encrypted at rest for storage/backups/dumps, not user-only decryption. |
| Not every upstream feature is exposed | Provider adapters define current capabilities. | README provider and dictation matrices | Do not claim universal provider feature parity. |
| Meta support is text-only | Muse Spark 1.1 uses the shared Chat Completions adapter. | README provider-specific details | Do not imply Meta dictation, web search, tools, multimodal inputs, or Responses fallback. |
| Web search limited to configured OpenAI models | Other providers are marked unsupported. | README provider-specific details | Do not imply search across all providers. |
| Third-party static-page telemetry | Public static pages load Google Analytics and LoopAware scripts. | site/index.html, resource generator | Do not claim collection, retention, consent, or opt-out behavior without approved legal or provider documentation. |
| Live provider tests can spend money | Live smoke tests are not part of CI. | README local automation | Do not present live tests as routine CI. |
| Management requires TAuth/database config | Self-service UI needs several hosted values. | README management UI and split-origin setup | Do not imply zero-config hosted management. |

### Safe Claims

- LLM Proxy exposes GET /, POST /, POST /v2, and POST /dictate behind tenant-secret authentication.
- Direct HTTP, the official Go package, the official Python package, and the installable CLI converge on canonical POST /v2 messages for text.
- Provider and model selection can change the supported route without replacing the application's canonical messages integration.
- It routes text to OpenAI, Meta Muse Spark 1.1 and other OpenAI-compatible providers, Anthropic, Gemini, and Grok/xAI as documented in the provider matrix.
- It routes dictation through /dictate for OpenAI, SiliconFlow, Zhipu, and Grok/xAI as documented.
- It keeps upstream provider API keys server-side and rejects provider-key-like fields on public proxy requests.
- It can run a TAuth-protected self-service management UI that automatically creates a missing client key, autosaves selected-provider settings, and requires one persisted provider key before Settings can close.
- Managed provider keys are encrypted at rest with AES-GCM; this protects storage/backups/dumps and is not a zero-knowledge guarantee.
- Browser runtime config is served by the backend /config-ui.yaml endpoint.

### Unsupported Claims Excluded

- Customer logos, testimonials, case studies, revenue impact, benchmark results, search volume, pricing savings, provider-longevity promises, model-onboarding targets, uptime guarantees, compliance certifications, or named competitor comparisons.

## Positioning Opportunity Decisions

| Opportunity | Decision | Reason | Public path |
|---|---|---|---|
| Integrate once across supported models | Refresh one evidence-backed cornerstone | It is the primary product job and can own the shared contract, clients, route choice, limits, and proof without an audience swap. | /resources/multi-provider-llm-proxy/ |
| AI-assisted builder page | Serve through the hub and copyable-request guide | A standalone audience page would substantially repeat the HTTP/client quick start. The existing guide provides the concrete request this audience needs. | /resources/copyable-llm-curl-examples/ |
| Startup-specific LLM proxy page | Serve through the hub and native-provider comparison | A standalone startup page would repeat provider comparison and model-selection content. The existing comparison is a distinct product-team job. | /resources/openai-claude-gemini-one-endpoint/ |
| Institutional engineering page | Refresh the existing internal-gateway resource | This audience has a distinct ownership, credential, routing-default, usage, and operating-boundary job. | /resources/internal-ai-gateway-for-product-tools/ |

## Use-Case Opportunity List

| Priority | Page idea | Audience | Problem | Primary keyword candidate | Category | Public URL | Doorway risk | Recommendation |
|---:|---|---|---|---|---|---|---|---|
| 1 | Integrate once through one multi-provider LLM proxy | AI-assisted builders, startups, product teams, and platform groups that need model choice without repeated provider integrations. | Adding a provider SDK, credential flow, payload mapper, and error parser for every model family turns model choice into application integration work. | multi-provider LLM proxy | Provider routing | /resources/multi-provider-llm-proxy/ | Low | Refresh as cornerstone |
| 2 | Keep provider API keys server-side for LLM apps | Internal-tool builders who need AI calls without distributing raw upstream keys. | Client apps, notebooks, browser utilities, and scripts can drift into storing raw OpenAI, Anthropic, Gemini, or xAI keys. Once those keys leave the backend, rotation and audit work become harder. | server-side provider API keys | Security | /resources/server-side-provider-api-keys/ | Low | Maintain existing resource |
| 3 | Tenant-secret AI gateway for internal applications | Teams building internal apps that need a single guarded AI service boundary. | When every app gets its own provider credential and routing rules, access control becomes difficult to reason about and providers become embedded in product code. | tenant secret AI gateway | Security | /resources/tenant-secret-ai-gateway/ | Low | Maintain existing resource |
| 4 | Self-service LLM key management for internal teams | Teams that want user-owned AI access without asking operators to edit YAML for every change. | Operator-provisioned AI access does not scale when each user or team needs provider keys, defaults, generated secrets, and examples updated separately. | self-service LLM key management | Management UI | /resources/self-service-llm-key-management/ | Low | Maintain existing resource |
| 5 | Bring-your-own provider key portal for AI access | Organizations where users or teams own their upstream provider accounts. | BYO provider keys can become risky when users paste keys into product apps, scripts, or support messages instead of a controlled backend surface. | bring your own provider key portal | Management UI | /resources/bring-your-own-provider-key-portal/ | Low | Maintain existing resource |
| 6 | Canonical /v2 chat messages API for LLM calls | Developers who want a stable chat transcript contract instead of provider-specific payloads. | Chat transcript callers can end up building OpenAI, Anthropic, Gemini, and compatible-provider request bodies separately, with different message rules in each client. | v2 chat messages API | API contract | /resources/canonical-v2-chat-messages-api/ | Low | Maintain existing resource |
| 7 | Large prompt JSON POST for LLM requests | Developers moving from quick prompt calls to large documents or generated prompt bodies. | GET query strings are convenient for small prompts, but large or non-ASCII prompts need a request body and clear size validation. | large prompt JSON POST | API contract | /resources/large-prompt-json-post/ | Low | Maintain existing resource |
| 8 | Audio transcription proxy API behind tenant secrets | Teams adding voice input or dictation to internal tools without a separate provider credential path. | Dictation integrations often grow a separate security and provider configuration path from text generation, even when the same apps need both. | audio transcription proxy API | Dictation | /resources/audio-transcription-proxy-api/ | Low | Maintain existing resource |
| 9 | Switch OpenAI, Claude, and Gemini behind one endpoint | Startups and product teams comparing native model families without maintaining three product integrations. | OpenAI Responses, Anthropic Messages, and Gemini Interactions use different authentication, payload, model-limit, lifecycle, and response contracts. | OpenAI Claude Gemini one endpoint | Provider routing | /resources/openai-claude-gemini-one-endpoint/ | Low | Refresh as cornerstone |
| 10 | OpenAI background response polling without client loops | Backend teams with long OpenAI prompts that should not require client polling logic. | Long OpenAI Responses work can push polling, resume tokens, or streaming complexity into every caller if the gateway does not own the lifecycle. | OpenAI background response polling | Reliability | /resources/openai-background-response-polling/ | Low | Maintain existing resource |
| 11 | Upstream worker and queue limits for LLM traffic | Operators who need predictable capacity limits for provider HTTP calls. | Unlimited upstream calls can exhaust provider quotas or local resources, while long OpenAI polling sleeps should not occupy scarce worker capacity. | upstream worker queue limits | Reliability | /resources/upstream-worker-queue-limits/ | Low | Maintain existing resource |
| 12 | LLM model catalog configuration in config.yml | Operators maintaining provider model availability without changing application code. | Model lists change faster than client release cycles. Hardcoded model IDs in callers make provider updates brittle. | LLM model catalog configuration | Configuration | /resources/model-catalog-configuration/ | Low | Maintain existing resource |
| 13 | Provider default model selection for omitted models | Client developers and operators who want explicit defaults without hardcoding a model in every request. | If clients omit model, each provider route needs a clear rule. Otherwise requests can accidentally inherit a stale model from the wrong provider. | provider default model selection | Configuration | /resources/provider-default-model-selection/ | Low | Maintain existing resource |
| 14 | OpenAI web search guardrails in an LLM proxy | Teams that need controlled search-enabled model calls without making web search a universal flag. | A generic web_search flag can be misleading when only some providers and models support a search tool. | OpenAI web search guardrails | API contract | /resources/openai-web-search-guardrails/ | Low | Maintain existing resource |
| 15 | Normalized token usage metadata across providers | Teams that need operational usage signals without parsing every provider's response shape. | Providers report usage differently, and response format choices can make token accounting disappear from caller code. | normalized token usage metadata | Usage | /resources/normalized-token-usage-metadata/ | Low | Maintain existing resource |
| 16 | Account-wide managed tenant usage dashboard for LLMs | Teams giving users self-service AI access while keeping usage visible. | A multi-tenant key-management portal is incomplete when its default dashboard hides every tenant except one or implies that selecting a tenant activates it. | managed tenant usage dashboard | Usage | /resources/managed-tenant-usage-dashboard/ | Low | Maintain existing resource |
| 17 | Admin usage visibility without exposing secrets | Operators who need oversight of managed AI access without turning dashboards into sensitive data exports. | Admin views can become dangerous if they show generated secrets, provider keys, prompts, transcripts, or model responses. | admin usage visibility without secrets | Usage | /resources/admin-usage-visibility-without-secrets/ | Low | Maintain existing resource |
| 18 | API-served runtime config for a static LLM UI | Teams deploying a split-origin static management UI and backend API. | Static frontends can accidentally ship stale API origins, OAuth values, or runtime config if those values are rendered into the artifact. | API-served runtime config static UI | Deployment | /resources/api-served-runtime-config-for-static-ui/ | Low | Maintain existing resource |
| 19 | TAuth-protected management API for LLM Proxy | Teams adopting the MPR/TAuth shell for authenticated AI self-service. | Key management APIs need a stronger boundary than a public static page. They must know who is signed in and which tenant that user owns. | TAuth protected management API | Management UI | /resources/tauth-protected-management-api/ | Low | Maintain existing resource |
| 20 | Rotate generated LLM Proxy client keys with confidence | Teams that want self-service client access without permanent retrievable secrets. | Long-lived client secrets become harder to control when users can retrieve old raw values or when rotation requires operator edits. | generated LLM proxy secret rotation | Security | /resources/generated-secret-rotation/ | Low | Maintain existing resource |
| 21 | Encrypted provider key storage for managed tenants | Teams evaluating how LLM Proxy stores BYO provider credentials in management mode. | Provider API keys are high-value secrets. A management database should not store raw upstream credentials as plaintext rows. | encrypted provider key storage | Security | /resources/encrypted-provider-key-storage/ | Low | Maintain existing resource |
| 22 | Reject client-supplied provider key leaks | Security-conscious teams that want mistakes to fail before provider credentials spread. | A caller may accidentally include an OpenAI or provider api_key field in a proxy request body, query string, or multipart form. | reject client provider key leaks | Security | /resources/reject-client-provider-key-leaks/ | Low | Maintain existing resource |
| 23 | Strict YAML config placeholders for LLM Proxy | Operators who want predictable startup behavior and no hidden runtime defaults. | Services that merge flags, env, defaults, and files can start with surprising configuration. Missing secrets may appear only when traffic arrives. | strict YAML config placeholders | Configuration | /resources/strict-yaml-config-placeholders/ | Low | Maintain existing resource |
| 24 | Transactional multi-tenant account ownership migration | Operators upgrading an existing management database to the multi-tenant ownership schema. | The earlier persistence shape coupled one tenant row to one user, so one TAuth subject could not own multiple isolated tenants. | multi-tenant ownership migration | Configuration | /resources/multi-tenant-ownership-migration/ | Low | Maintain existing resource |
| 25 | GORM-managed tenant persistence for LLM Proxy | Backend operators deciding how management-mode state is stored. | Self-service management needs persistent tenant state without mutating runtime config files or adding raw SQL paths. | GORM managed tenant persistence | Configuration | /resources/gorm-managed-tenant-persistence/ | Low | Maintain existing resource |
| 26 | Authenticate an LLM Proxy client with a tenant secret | Developers connecting an application, script, or command-line workflow to LLM Proxy. | A client integration can confuse three different credentials: the tenant secret that authorizes proxy requests, the TAuth session that authorizes management actions, and upstream provider API keys that must stay server-side. | LLM Proxy client authentication | Clients | /resources/llm-proxy-client-authentication/ | Low | Maintain existing resource |
| 27 | Go LLM Proxy client with a v2-only transport | Go developers integrating application backends with LLM Proxy. | Reusable clients can expose too many legacy request shapes and force callers to choose between prompt JSON and chat messages. | Go LLM proxy client v2 | Clients | /resources/go-client-v2-only-llm-proxy/ | Low | Maintain existing resource |
| 28 | Python LLM Proxy client with v2 messages | Python workflow authors and service developers standardizing on the /v2 messages contract. | Python callers often start with raw requests and then duplicate provider-specific payload details in scripts. | Python LLM proxy client v2 | Clients | /resources/python-client-v2-only-llm-proxy/ | Low | Maintain existing resource |
| 29 | Installable LLM Proxy CLI for prompt workflows | Developers who want a simple shell client for tenant-secret authenticated LLM calls. | Curl is useful, but repeated prompt workflows need a small client that understands the proxy's canonical text contract. | installable LLM proxy CLI | Clients | /resources/installable-llm-proxy-cli/ | Low | Maintain existing resource |
| 30 | LLM response formats: JSON, XML, CSV, and text | Developers integrating proxy responses into scripts, services, and data pipelines. | Different callers need different output shapes. A shell script may want text while an application wants JSON with request and usage metadata. | LLM response formats JSON XML CSV text | API contract | /resources/llm-response-formats-json-xml-csv-text/ | Low | Maintain existing resource |
| 31 | LLM Proxy status code map for callers | Developers who need predictable error handling around LLM and dictation calls. | Provider errors can be inconsistent. Callers need to know whether a request failed because of authentication, validation, capacity, provider rate limits, or upstream failure. | LLM proxy status code map | Reliability | /resources/llm-proxy-status-code-map/ | Low | Maintain existing resource |
| 32 | Dictation provider routing for OpenAI, Zhipu, Grok, and SiliconFlow | Teams testing transcription providers behind one proxy endpoint. | Speech providers use different URLs, models, and multipart details. Client apps should not carry those differences. | dictation provider routing | Dictation | /resources/dictation-provider-routing/ | Low | Maintain existing resource |
| 33 | OpenAI-compatible provider gateway | Teams adopting OpenAI-compatible chat providers without rewriting every caller. | OpenAI-compatible providers share a broad shape but still need different base URLs, keys, defaults, and availability rules. | OpenAI-compatible provider gateway | Provider routing | /resources/openai-compatible-provider-gateway/ | Low | Maintain existing resource |
| 34 | Gemini Interactions proxy for shared LLM calls | Developers adding Gemini as a provider without bringing Gemini-specific payloads into every app. | Gemini models differ between stored background Interactions and non-stored synchronous Interactions, which shared proxy callers should not have to coordinate. | Gemini Interactions proxy | Provider routing | /resources/gemini-interactions-proxy/ | Low | Maintain existing resource |
| 35 | Anthropic Claude Messages proxy for /v2 callers | Teams adding Claude behind the same tenant-secret proxy used for other providers. | Anthropic Messages has native system and max_tokens requirements that differ from OpenAI-compatible chat routes. | Anthropic Claude Messages proxy | Provider routing | /resources/anthropic-claude-messages-proxy/ | Low | Maintain existing resource |
| 36 | Per-request provider and model selection | Application developers who want simple defaults plus controlled overrides for special tasks. | A single application may need mostly default routing but occasional provider or model changes for a specific workflow. | per-request provider model selection | API contract | /resources/per-request-provider-model-selection/ | Low | Maintain existing resource |
| 37 | System prompt handling without ambiguous inputs | Developers who need predictable system-instruction behavior across providers. | System instructions can collide when a request sends both a body system_prompt and a system role message, or when tenant defaults also exist. | LLM system prompt handling | API contract | /resources/system-prompt-handling/ | Low | Maintain existing resource |
| 38 | max_tokens validation across LLM providers | Developers and operators who need predictable output caps without provider-specific client code. | Output-token fields differ across OpenAI, compatible chat providers, Anthropic, and Gemini, and some providers have known ceilings. | max tokens provider limit validation | API contract | /resources/max-tokens-provider-limit-validation/ | Low | Maintain existing resource |
| 39 | Usage metadata without storing prompts or responses | Teams that need AI usage visibility without making the usage store a sensitive content log. | Usage dashboards can accidentally become stores of prompts, transcripts, uploaded audio names, model responses, provider keys, or generated secrets. | usage metadata without prompts | Usage | /resources/usage-metadata-without-prompts/ | Low | Maintain existing resource |
| 40 | Copyable LLM curl examples from current profile data | Users onboarding themselves to LLM Proxy through the management UI. | Docs and examples drift when they hardcode hosts, provider choices, or secret placeholders that do not match the signed-in user's state. | copyable LLM curl examples | Management UI | /resources/copyable-llm-curl-examples/ | Low | Maintain existing resource |
| 41 | Provider-specific system prompts in LLM Proxy Settings | Users who need different instructions for different upstream providers. | A single global system prompt can be too blunt when different providers are used for different jobs or when provider-selected requests need their own context. | provider-specific system prompts | Management UI | /resources/provider-specific-system-prompts/ | Low | Maintain existing resource |
| 42 | Local and hosted LLM Proxy config profiles | Operators who need local development and production profiles to stay aligned. | Local profiles often accumulate defaults, fallback values, or alternate config paths that do not match hosted runtime behavior. | local and hosted LLM proxy config | Deployment | /resources/local-and-hosted-llm-proxy-config-profiles/ | Low | Maintain existing resource |
| 43 | GitHub Pages management UI for LLM Proxy | Teams deploying the self-service management UI as a static site. | Serving the management frontend from the API backend couples static hosting, runtime config, and proxy endpoints in one deployment surface. | GitHub Pages LLM management UI | Deployment | /resources/github-pages-llm-management-ui/ | Low | Maintain existing resource |
| 44 | Live provider smoke tests for LLM Proxy | Operators validating provider credentials and hosted provider routes after config or deployment changes. | CI should be deterministic and avoid paid provider calls, but some changes still need live confirmation against real upstream providers. | live provider smoke tests | Validation | /resources/live-provider-smoke-tests/ | Low | Maintain existing resource |
| 45 | Internal AI gateway for durable product integrations | Institutional engineering and platform teams standardizing AI access across multiple applications and product groups. | Applications that adopt provider SDKs independently create duplicated credential handling, inconsistent route validation, provider-specific failures, and fragmented usage visibility. | internal AI gateway | Use cases | /resources/internal-ai-gateway-for-product-tools/ | Low | Refresh as cornerstone |
| 46 | Provider overload and timeout handling for LLM calls | Developers building retry and alerting behavior around LLM Proxy. | Failures are harder to handle when overload, provider timeout, missing credentials, and upstream errors collapse into one generic exception. | LLM provider overload timeout handling | Reliability | /resources/provider-overload-timeout-handling/ | Low | Maintain existing resource |

## Category Mix

| Category | Page count | Doorway safety note |
|---|---:|---|
| Provider routing | 5 | Distinct workflow and examples within this category. |
| Security | 5 | Distinct workflow and examples within this category. |
| Management UI | 5 | Distinct workflow and examples within this category. |
| API contract | 7 | Distinct workflow and examples within this category. |
| Dictation | 2 | Distinct workflow and examples within this category. |
| Reliability | 4 | Distinct workflow and examples within this category. |
| Configuration | 5 | Distinct workflow and examples within this category. |
| Usage | 4 | Distinct workflow and examples within this category. |
| Deployment | 3 | Distinct workflow and examples within this category. |
| Clients | 4 | Distinct workflow and examples within this category. |
| Validation | 1 | Distinct workflow and examples within this category. |
| Use cases | 1 | Distinct workflow and examples within this category. |

## Page-Specific Publication Briefs

| Page | Allowed claims | Forbidden claims | Differentiation | Repository evidence | CTA | Canonical path | Significant update |
|---|---|---|---|---|---|---|---|
| Integrate once through one multi-provider LLM proxy | One canonical POST /v2 contract, direct HTTP, official Go, Python, and CLI clients, explicit supported-route selection, server-side provider credentials, and a generated current capability matrix. | Universal upstream feature parity, automatic fallback, benchmark leadership, savings, provider longevity, model-onboarding time, or hosted uptime guarantees. | This is the integrate-once cornerstone: it explains how the client contract stays stable while provider and model routing change. | README.md, verified 2026-08-08 | Compare integration options | /resources/multi-provider-llm-proxy/ | 2026-08-08 |
| Switch OpenAI, Claude, and Gemini behind one endpoint | One canonical messages body, native OpenAI Responses, Anthropic Messages, and Gemini Interactions adapters, explicit route selection, blocking caller behavior, and current catalog validation. | Identical upstream behavior, identical capabilities, provider performance rankings, automatic fallback, availability guarantees, or universal model access. | This page is a concrete three-provider comparison for product teams; it explains which contract stays shared and which behavior remains route-specific. | README.md, verified 2026-08-08 | Explore supported models | /resources/openai-claude-gemini-one-endpoint/ | 2026-08-08 |
| Transactional multi-tenant account ownership migration | Bounded preflight, atomic migration, preserved tenant ids and usage, and tenant-bound provider-key re-encryption. | Performance, pricing, compliance, benchmark, and zero-downtime claims. | Operator runbook for the one-tenant-per-user ownership upgrade, distinct from general GORM persistence guidance. | internal/proxy/management_store.go, verified 2026-08-08 | Read the migration runbook | /resources/multi-tenant-ownership-migration/ | 2026-08-08 |
| Internal AI gateway for durable product integrations | One tenant-client-key boundary, canonical messages integration, server-side provider keys, managed routing defaults, route validation, official clients, and content-free usage summaries. | Complete AI governance, compliance certification, data residency, procurement guarantees, hosted availability, or replacement of organizational security controls. | This page addresses institutional ownership and operating boundaries across many applications rather than provider comparison or individual developer setup. | README.md, verified 2026-08-08 | Read the authentication guide | /resources/internal-ai-gateway-for-product-tools/ | 2026-08-08 |

## Site Integration And Discoverability

- The main page links to integration guides, audience resources, /docs/, the generated model matrix, and /resources/ through crawlable anchors; the shared MPR header owns authenticated entry to /app/.
- The /resources/ hub leads with HTTP, Go, Python, and CLI integration paths, routes three audience needs into differentiated existing guides, and links every generated page by category.
- Every resource page carries author attribution and links to the derived API reference, /resources/, the shared public shell, and related resources; /docs/ exposes view and download actions for the exact OpenAPI schema.
- sitemap.xml lists /, /docs/, /resources/, /privacy/, /terms/, and all 46 resource URLs with the same trailing-slash canonical form used in internal links.
- robots.txt allows crawling and references the sitemap.

## Evaluation Report

Independent quality and risk evaluation: 2026-08-08

| Category | Score | Notes |
|---|---:|---|
| Repo grounding | 5 | API, client, routing, credential, capability, usage, and lifecycle claims trace to README and implementation contracts. |
| Use-case specificity | 4 | Builder, product-team, and institutional paths have distinct problems, workflows, examples, and CTAs. |
| Doorway-page safety | 5 | Audience-swapped pages were rejected; existing concrete guides serve each path. |
| SEO metadata quality | 4 | Unique titles, descriptions, canonicals, Open Graph, Twitter metadata, and appropriate schema are present. |
| Keyword naturalness | 5 | Integrate-once and provider terms read as product language rather than keyword insertion. |
| Factual integrity | 5 | Current capabilities are repository-supported, route-specific limits are disclosed, and no unapproved SLA or proof claim is current copy. |
| Conversion clarity | 5 | Landing, hub, and cornerstone pages lead to integration selection, model comparison, or authentication guidance. |
| Duplicate-content risk | 4 | Shared structure remains, but cornerstone workflows, evidence, examples, limitations, FAQ, and CTAs differ materially. |
| Site integration and discoverability | 5 | Landing, hub, headers, related links, breadcrumbs, categories, sitemap, and robots form complete crawlable paths. |
| Google indexing readiness | 4 | Canonicals, sitemap URLs, lastmod, schema dates, verification dates, and robots align; post-deploy Search Console validation remains. |
| Subagent handoff quality | 5 | The report preserves source analysis, excluded claims, opportunity decisions, publication briefs, differentiation, dates, and canonical paths. |

Highest-risk copy was qualified to supported text routes so the promoted POST /v2 clients cannot be read as covering dictation-only models. Future SLA publication language was also reduced to the exact current-contract boundary.

Final decision: Pass. All binding thresholds are met, including factual integrity at 5.
