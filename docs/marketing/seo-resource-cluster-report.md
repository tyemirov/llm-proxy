# LLM Proxy SEO Resource Cluster Report

Generated: 2026-07-22

## Repo Analysis Report

### Source Files Reviewed

| File | Type | Why reviewed | Key findings | Confidence |
|---|---|---|---|---|
| README.md | Required product docs | Primary product description, REST contract, config, management UI, clients, deployment, and security wording | LLM Proxy is a lightweight HTTP proxy for text and dictation providers with tenant-secret auth, server-side credentials, optional TAuth management UI, usage dashboards, provider routing, and strict config loading. | High |
| docs/implementation/provider-routing-plan.md | Implementation notes | Provider routing, config ownership, management mode, error contract, and adapter notes | Multi-provider routing, model catalogs, omitted-model behavior, split-origin management, usage storage, and provider-key security are implemented contracts. | High |
| docs/implementation/dictation-endpoint-plan.md | Implementation notes | Dictation endpoint contract | /dictate accepts multipart audio with key auth and returns JSON text; dictation routing is implemented for supported providers in the current README. | High |
| docs/marketing/social-media-60-day-campaign.md | Marketing copy | Existing audience and claim framing | Existing public claims emphasize server-side provider keys, multi-provider routing, dictation, usage dashboards, API-served config, and careful encrypted-at-rest wording. | Medium |

### Product Summary

- Product name: LLM Proxy
- Product category: Multi-provider LLM and dictation HTTP proxy with optional self-service management UI.
- One-sentence description: LLM Proxy forwards authenticated text and audio requests to configured upstream providers while keeping provider credentials server-side and giving callers one tenant-secret HTTP contract.
- Primary users: Developers, platform engineers, technical founders, AI platform operators, and internal-tool teams.
- Secondary users: Managed end users who sign in, receive a client key, save provider keys, copy request examples, and inspect usage.
- Primary job-to-be-done: Centralize provider routing, credentials, request validation, response formatting, dictation, usage metadata, and management workflows behind one service boundary.
- Installation or usage model: Run the Go backend from config.yml; publish the static management UI from site/ to GitHub Pages; call GET /, POST /, POST /v2, or POST /dictate with key=<tenant secret>.
- Current maturity: Implemented repo contract with Go/Python/frontend validation and documented release/deploy workflows.

### Product Capabilities

| Capability | Description | Evidence source | Confidence | Current / roadmap / unclear | Safe for page copy? |
|---|---|---|---|---|---|
| Multi-provider text routing | Routes OpenAI Responses, Meta Muse Spark and other OpenAI-compatible providers, Anthropic Messages, Gemini generateContent, and Grok/xAI text. | README provider matrix, provider routing notes | High | Current | Yes |
| Dictation endpoint | Routes multipart audio through /dictate for supported dictation providers. | README dictation section, dictation plan | High | Current | Yes |
| Tenant-secret auth | Public proxy endpoints require key=<tenant secret>. | README REST and security sections | High | Current | Yes |
| Server-side provider credentials | Public requests must not send upstream provider keys; credentials stay server-side. | README security, provider routing notes | High | Current | Yes |
| TAuth management UI | Optional static Pages UI with authenticated profile, provider key, generated secret, settings, usage, and admin views. | README management UI section | High | Current | Yes |
| Encrypted-at-rest managed provider keys | AES-GCM storage with base64 32-byte key and honest non-zero-knowledge wording. | README management UI section | High | Current | Yes, with caution wording |
| Usage dashboard | 30-day usage summaries by request, token, provider, model, status, and daily bucket. | README management UI section | High | Current | Yes |
| API-served runtime config | Browser config comes from backend /config-ui.yaml, not a static Pages config artifact. | README hosted split-origin section | High | Current | Yes |
| Bundled v2-only clients | Go package, Go CLI, and Python package send canonical /v2 messages for text. | README clients section | High | Current | Yes |
| Worker/queue controls | server.workers limits upstream HTTP operations and queue_size limits pending operations. | README REST contract and config section | High | Current | Yes |

### Non-Capabilities, Limits, and Cautions

| Item | Why it matters | Evidence | Copywriting rule |
|---|---|---|---|
| No zero-knowledge guarantee | Backend decrypts provider keys to call upstream providers. | README management UI | Say encrypted at rest for storage/backups/dumps, not user-only decryption. |
| Not every upstream feature is exposed | Provider adapters define current capabilities. | README provider and dictation matrices | Do not claim universal provider feature parity. |
| Meta support is text-only | Muse Spark 1.1 uses the shared Chat Completions adapter. | README provider-specific details | Do not imply Meta dictation, web search, tools, multimodal inputs, or Responses fallback. |
| Web search limited to configured OpenAI models | Other providers are marked unsupported. | README provider-specific details | Do not imply search across all providers. |
| Live provider tests can spend money | Live smoke tests are not part of CI. | README local automation | Do not present live tests as routine CI. |
| Management requires TAuth/database config | Self-service UI needs several hosted values. | README management UI and split-origin setup | Do not imply zero-config hosted management. |

### Safe Claims

- LLM Proxy exposes GET /, POST /, POST /v2, and POST /dictate behind tenant-secret authentication.
- It routes text to OpenAI, Meta Muse Spark 1.1 and other OpenAI-compatible providers, Anthropic, Gemini, and Grok/xAI as documented in the provider matrix.
- It routes dictation through /dictate for OpenAI, SiliconFlow, Zhipu, and Grok/xAI as documented.
- It keeps upstream provider API keys server-side and rejects provider-key-like fields on public proxy requests.
- It can run a TAuth-protected self-service management UI that automatically creates a missing client key and requires one saved provider key before Settings can close.
- Managed provider keys are encrypted at rest with AES-GCM; this protects storage/backups/dumps and is not a zero-knowledge guarantee.
- Browser runtime config is served by the backend /config-ui.yaml endpoint.

### Unsupported Claims Excluded

- Customer logos, testimonials, case studies, revenue impact, benchmark results, search volume, pricing savings, uptime guarantees, compliance certifications, or named competitor comparisons.

## Use-Case Opportunity List

| Priority | Page idea | Audience | Problem | Primary keyword candidate | Category | Public URL | Doorway risk | Recommendation |
|---:|---|---|---|---|---|---|---|---|
| 1 | Multi-provider LLM proxy for internal tools | Platform engineers consolidating several model providers behind one internal service. | Teams often add one SDK, key path, and retry surface per provider. That creates scattered credentials, inconsistent defaults, and provider behavior that leaks into every caller. | multi-provider LLM proxy | Provider routing | /resources/multi-provider-llm-proxy/ | Low | Generate |
| 2 | Keep provider API keys server-side for LLM apps | Internal-tool builders who need AI calls without distributing raw upstream keys. | Client apps, notebooks, browser utilities, and scripts can drift into storing raw OpenAI, Anthropic, Gemini, or xAI keys. Once those keys leave the backend, rotation and audit work become harder. | server-side provider API keys | Security | /resources/server-side-provider-api-keys/ | Low | Generate |
| 3 | Tenant-secret AI gateway for internal applications | Teams building internal apps that need a single guarded AI service boundary. | When every app gets its own provider credential and routing rules, access control becomes difficult to reason about and providers become embedded in product code. | tenant secret AI gateway | Security | /resources/tenant-secret-ai-gateway/ | Low | Generate |
| 4 | Self-service LLM key management for internal teams | Teams that want user-owned AI access without asking operators to edit YAML for every change. | Operator-provisioned AI access does not scale when each user or team needs provider keys, defaults, generated secrets, and examples updated separately. | self-service LLM key management | Management UI | /resources/self-service-llm-key-management/ | Low | Generate |
| 5 | Bring-your-own provider key portal for AI access | Organizations where users or teams own their upstream provider accounts. | BYO provider keys can become risky when users paste keys into product apps, scripts, or support messages instead of a controlled backend surface. | bring your own provider key portal | Management UI | /resources/bring-your-own-provider-key-portal/ | Low | Generate |
| 6 | Canonical /v2 chat messages API for LLM calls | Developers who want a stable chat transcript contract instead of provider-specific payloads. | Chat transcript callers can end up building OpenAI, Anthropic, Gemini, and compatible-provider request bodies separately, with different message rules in each client. | v2 chat messages API | API contract | /resources/canonical-v2-chat-messages-api/ | Low | Generate |
| 7 | Large prompt JSON POST for LLM requests | Developers moving from quick prompt calls to large documents or generated prompt bodies. | GET query strings are convenient for small prompts, but large or non-ASCII prompts need a request body and clear size validation. | large prompt JSON POST | API contract | /resources/large-prompt-json-post/ | Low | Generate |
| 8 | Audio transcription proxy API behind tenant secrets | Teams adding voice input or dictation to internal tools without a separate provider credential path. | Dictation integrations often grow a separate security and provider configuration path from text generation, even when the same apps need both. | audio transcription proxy API | Dictation | /resources/audio-transcription-proxy-api/ | Low | Generate |
| 9 | OpenAI, Claude, and Gemini through one endpoint | Product teams comparing native model providers without adding three client integrations. | OpenAI Responses, Anthropic Messages, and Gemini generateContent have different payloads, model limits, auth headers, and response shapes. | OpenAI Claude Gemini one endpoint | Provider routing | /resources/openai-claude-gemini-one-endpoint/ | Low | Generate |
| 10 | OpenAI background response polling without client loops | Backend teams with long OpenAI prompts that should not require client polling logic. | Long OpenAI Responses work can push polling, resume tokens, or streaming complexity into every caller if the gateway does not own the lifecycle. | OpenAI background response polling | Reliability | /resources/openai-background-response-polling/ | Low | Generate |
| 11 | Upstream worker and queue limits for LLM traffic | Operators who need predictable capacity limits for provider HTTP calls. | Unlimited upstream calls can exhaust provider quotas or local resources, while long OpenAI polling sleeps should not occupy scarce worker capacity. | upstream worker queue limits | Reliability | /resources/upstream-worker-queue-limits/ | Low | Generate |
| 12 | LLM model catalog configuration in config.yml | Operators maintaining provider model availability without changing application code. | Model lists change faster than client release cycles. Hardcoded model IDs in callers make provider updates brittle. | LLM model catalog configuration | Configuration | /resources/model-catalog-configuration/ | Low | Generate |
| 13 | Provider default model selection for omitted models | Client developers and operators who want explicit defaults without hardcoding a model in every request. | If clients omit model, each provider route needs a clear rule. Otherwise requests can accidentally inherit a stale model from the wrong provider. | provider default model selection | Configuration | /resources/provider-default-model-selection/ | Low | Generate |
| 14 | OpenAI web search guardrails in an LLM proxy | Teams that need controlled search-enabled model calls without making web search a universal flag. | A generic web_search flag can be misleading when only some providers and models support a search tool. | OpenAI web search guardrails | API contract | /resources/openai-web-search-guardrails/ | Low | Generate |
| 15 | Normalized token usage metadata across providers | Teams that need operational usage signals without parsing every provider's response shape. | Providers report usage differently, and response format choices can make token accounting disappear from caller code. | normalized token usage metadata | Usage | /resources/normalized-token-usage-metadata/ | Low | Generate |
| 16 | Managed tenant usage dashboard for LLM requests | Teams giving users self-service AI access while keeping usage visible. | A key-management portal is incomplete if users cannot see whether their managed proxy traffic is succeeding or which providers and models they use. | managed tenant usage dashboard | Usage | /resources/managed-tenant-usage-dashboard/ | Low | Generate |
| 17 | Admin usage visibility without exposing secrets | Operators who need oversight of managed AI access without turning dashboards into sensitive data exports. | Admin views can become dangerous if they show generated secrets, provider keys, prompts, transcripts, or model responses. | admin usage visibility without secrets | Usage | /resources/admin-usage-visibility-without-secrets/ | Low | Generate |
| 18 | API-served runtime config for a static LLM UI | Teams deploying a split-origin static management UI and backend API. | Static frontends can accidentally ship stale API origins, OAuth values, or runtime config if those values are rendered into the artifact. | API-served runtime config static UI | Deployment | /resources/api-served-runtime-config-for-static-ui/ | Low | Generate |
| 19 | TAuth-protected management API for LLM Proxy | Teams adopting the MPR/TAuth shell for authenticated AI self-service. | Key management APIs need a stronger boundary than a public static page. They must know who is signed in and which tenant that user owns. | TAuth protected management API | Management UI | /resources/tauth-protected-management-api/ | Low | Generate |
| 20 | Generated LLM Proxy secret rotation and revocation | Teams that want self-service client access without permanent retrievable secrets. | Long-lived client secrets become harder to control when users can retrieve old raw values or when revocation requires operator edits. | generated LLM proxy secret rotation | Security | /resources/generated-secret-rotation-and-revocation/ | Low | Generate |
| 21 | Encrypted provider key storage for managed tenants | Teams evaluating how LLM Proxy stores BYO provider credentials in management mode. | Provider API keys are high-value secrets. A management database should not store raw upstream credentials as plaintext rows. | encrypted provider key storage | Security | /resources/encrypted-provider-key-storage/ | Low | Generate |
| 22 | Reject client-supplied provider key leaks | Security-conscious teams that want mistakes to fail before provider credentials spread. | A caller may accidentally include an OpenAI or provider api_key field in a proxy request body, query string, or multipart form. | reject client provider key leaks | Security | /resources/reject-client-provider-key-leaks/ | Low | Generate |
| 23 | Strict YAML config placeholders for LLM Proxy | Operators who want predictable startup behavior and no hidden runtime defaults. | Services that merge flags, env, defaults, and files can start with surprising configuration. Missing secrets may appear only when traffic arrives. | strict YAML config placeholders | Configuration | /resources/strict-yaml-config-placeholders/ | Low | Generate |
| 24 | Legacy token ownership migration in management mode | Operators retiring the final unowned management-mode token after moving to self-service accounts. | A token imported by an older release can still belong to a synthetic static-config user, so its real owner cannot see that token's usage after signing in. | static to managed tenant migration | Configuration | /resources/static-to-managed-tenant-migration/ | Low | Generate |
| 25 | GORM-managed tenant persistence for LLM Proxy | Backend operators deciding how management-mode state is stored. | Self-service management needs persistent tenant state without mutating runtime config files or adding raw SQL paths. | GORM managed tenant persistence | Configuration | /resources/gorm-managed-tenant-persistence/ | Low | Generate |
| 26 | Go LLM Proxy client with a v2-only transport | Go developers integrating application backends with LLM Proxy. | Reusable clients can expose too many legacy request shapes and force callers to choose between prompt JSON and chat messages. | Go LLM proxy client v2 | Clients | /resources/go-client-v2-only-llm-proxy/ | Low | Generate |
| 27 | Python LLM Proxy client with v2 messages | Python workflow authors and service developers standardizing on the /v2 messages contract. | Python callers often start with raw requests and then duplicate provider-specific payload details in scripts. | Python LLM proxy client v2 | Clients | /resources/python-client-v2-only-llm-proxy/ | Low | Generate |
| 28 | Installable LLM Proxy CLI for prompt workflows | Developers who want a simple shell client for tenant-secret authenticated LLM calls. | Curl is useful, but repeated prompt workflows need a small client that understands the proxy's canonical text contract. | installable LLM proxy CLI | Clients | /resources/installable-llm-proxy-cli/ | Low | Generate |
| 29 | LLM response formats: JSON, XML, CSV, and text | Developers integrating proxy responses into scripts, services, and data pipelines. | Different callers need different output shapes. A shell script may want text while an application wants JSON with request and usage metadata. | LLM response formats JSON XML CSV text | API contract | /resources/llm-response-formats-json-xml-csv-text/ | Low | Generate |
| 30 | LLM Proxy status code map for callers | Developers who need predictable error handling around LLM and dictation calls. | Provider errors can be inconsistent. Callers need to know whether a request failed because of authentication, validation, capacity, provider rate limits, or upstream failure. | LLM proxy status code map | Reliability | /resources/llm-proxy-status-code-map/ | Low | Generate |
| 31 | Dictation provider routing for OpenAI, Zhipu, Grok, and SiliconFlow | Teams testing transcription providers behind one proxy endpoint. | Speech providers use different URLs, models, and multipart details. Client apps should not carry those differences. | dictation provider routing | Dictation | /resources/dictation-provider-routing/ | Low | Generate |
| 32 | OpenAI-compatible provider gateway | Teams adopting OpenAI-compatible chat providers without rewriting every caller. | OpenAI-compatible providers share a broad shape but still need different base URLs, keys, defaults, and availability rules. | OpenAI-compatible provider gateway | Provider routing | /resources/openai-compatible-provider-gateway/ | Low | Generate |
| 33 | Gemini generateContent proxy for shared LLM calls | Developers adding Gemini as a provider without bringing Gemini-specific payloads into every app. | Gemini native generateContent uses a different route and content structure from OpenAI-compatible chat providers. | Gemini generateContent proxy | Provider routing | /resources/gemini-generatecontent-proxy/ | Low | Generate |
| 34 | Anthropic Claude Messages proxy for /v2 callers | Teams adding Claude behind the same tenant-secret proxy used for other providers. | Anthropic Messages has native system and max_tokens requirements that differ from OpenAI-compatible chat routes. | Anthropic Claude Messages proxy | Provider routing | /resources/anthropic-claude-messages-proxy/ | Low | Generate |
| 35 | Per-request provider and model selection | Application developers who want simple defaults plus controlled overrides for special tasks. | A single application may need mostly default routing but occasional provider or model changes for a specific workflow. | per-request provider model selection | API contract | /resources/per-request-provider-model-selection/ | Low | Generate |
| 36 | System prompt handling without ambiguous inputs | Developers who need predictable system-instruction behavior across providers. | System instructions can collide when a request sends both a body system_prompt and a system role message, or when tenant defaults also exist. | LLM system prompt handling | API contract | /resources/system-prompt-handling/ | Low | Generate |
| 37 | max_tokens validation across LLM providers | Developers and operators who need predictable output caps without provider-specific client code. | Output-token fields differ across OpenAI, compatible chat providers, Anthropic, and Gemini, and some providers have known ceilings. | max tokens provider limit validation | API contract | /resources/max-tokens-provider-limit-validation/ | Low | Generate |
| 38 | Usage metadata without storing prompts or responses | Teams that need AI usage visibility without making the usage store a sensitive content log. | Usage dashboards can accidentally become stores of prompts, transcripts, uploaded audio names, model responses, provider keys, or generated secrets. | usage metadata without prompts | Usage | /resources/usage-metadata-without-prompts/ | Low | Generate |
| 39 | Copyable LLM curl examples from current profile data | Users onboarding themselves to LLM Proxy through the management UI. | Docs and examples drift when they hardcode hosts, provider choices, or secret placeholders that do not match the signed-in user's state. | copyable LLM curl examples | Management UI | /resources/copyable-llm-curl-examples/ | Low | Generate |
| 40 | Provider-specific system prompts in LLM Proxy Settings | Users who need different instructions for different upstream providers. | A single global system prompt can be too blunt when different providers are used for different jobs or when provider-selected requests need their own context. | provider-specific system prompts | Management UI | /resources/provider-specific-system-prompts/ | Low | Generate |
| 41 | Local and hosted LLM Proxy config profiles | Operators who need local development and production profiles to stay aligned. | Local profiles often accumulate defaults, fallback values, or alternate config paths that do not match hosted runtime behavior. | local and hosted LLM proxy config | Deployment | /resources/local-and-hosted-llm-proxy-config-profiles/ | Low | Generate |
| 42 | GitHub Pages management UI for LLM Proxy | Teams deploying the self-service management UI as a static site. | Serving the management frontend from the API backend couples static hosting, runtime config, and proxy endpoints in one deployment surface. | GitHub Pages LLM management UI | Deployment | /resources/github-pages-llm-management-ui/ | Low | Generate |
| 43 | Live provider smoke tests for LLM Proxy | Operators validating provider credentials and hosted provider routes after config or deployment changes. | CI should be deterministic and avoid paid provider calls, but some changes still need live confirmation against real upstream providers. | live provider smoke tests | Validation | /resources/live-provider-smoke-tests/ | Low | Generate |
| 44 | Internal AI gateway for product tools | Product and platform teams with several internal tools calling LLMs or dictation providers. | Internal tools can quietly accumulate provider SDKs, raw keys, inconsistent defaults, and untracked usage as teams add AI features one by one. | internal AI gateway | Use cases | /resources/internal-ai-gateway-for-product-tools/ | Low | Generate |
| 45 | Provider overload and timeout handling for LLM calls | Developers building retry and alerting behavior around LLM Proxy. | Failures are harder to handle when overload, provider timeout, missing credentials, and upstream errors collapse into one generic exception. | LLM provider overload timeout handling | Reliability | /resources/provider-overload-timeout-handling/ | Low | Generate |

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
| Clients | 3 | Distinct workflow and examples within this category. |
| Validation | 1 | Distinct workflow and examples within this category. |
| Use cases | 1 | Distinct workflow and examples within this category. |

## Site Integration And Discoverability

- The main page links to /resources/ through a crawlable anchor in the public HTML.
- The /resources/ hub links every generated resource page grouped by category.
- Every resource page links back to /, /resources/, sitemap.xml, and related resources.
- sitemap.xml lists /, /resources/, and all 45 page URLs with the same trailing-slash canonical form used in internal links.
- robots.txt allows crawling and references the sitemap.

## Evaluation Report

| Category | Score | Notes |
|---|---:|---|
| Repo grounding | 5 | Claims are limited to README/docs evidence and the existing marketing document. |
| Use-case specificity | 4 | Pages are split by workflow, audience need, examples, and limitations. |
| Doorway-page safety | 4 | Pages do not vary only by keyword or industry; each uses a distinct product contract. |
| SEO metadata quality | 4 | Each page has unique title, description, canonical, Open Graph, Twitter card, and schema. |
| Keyword naturalness | 4 | Keywords are used in titles/H1/FAQ without repeated stuffing. |
| Factual integrity | 5 | Unsupported proof, customer, compliance, benchmark, pricing, and competitor claims are excluded. |
| Conversion clarity | 4 | CTAs route to the main management surface and resource hub. |
| Duplicate-content risk | 4 | Repeated template structure is balanced by distinct problem, workflow, feature, examples, and limitations. |
| Site integration and discoverability | 5 | Main page, hub, related links, breadcrumbs, sitemap, and robots all align. |
| Google indexing readiness | 4 | Static pages use canonical trailing-slash URLs, visible FAQ, JSON-LD, and crawlable links. |
| Subagent handoff quality | 4 | This report preserves evidence, opportunity list, integration plan, and evaluation notes. |

Final decision: Pass.
