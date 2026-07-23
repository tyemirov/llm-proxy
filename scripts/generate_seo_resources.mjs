// @ts-check

import { mkdir, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";

const PUBLIC_ORIGIN = "https://llm-proxy.mprlab.com";
const RESOURCE_ROOT = "site/resources";
const REPORT_PATH = "docs/marketing/seo-resource-cluster-report.md";
const RESOURCE_PUBLISHED_DATE = "2026-07-06";
const RESOURCE_DEFAULT_MODIFIED_DATE = "2026-07-11";
const PRODUCT_NAME = "LLM Proxy";
const MIN_PAGE_COUNT = 40;
const MAX_PAGE_COUNT = 50;

const evidence = Object.freeze({
  readme: "README.md",
  providerRouting: "docs/implementation/provider-routing-plan.md",
  dictation: "docs/implementation/dictation-endpoint-plan.md",
  campaign: "docs/marketing/social-media-60-day-campaign.md",
});

const pages = Object.freeze([
  page({
    slug: "multi-provider-llm-proxy",
    category: "Provider routing",
    primaryKeyword: "multi-provider LLM proxy",
    title: "Multi-provider LLM proxy for internal tools",
    description: "Route OpenAI, Meta Muse, Claude, Gemini, Grok, and compatible providers through one tenant-secret HTTP boundary.",
    audience: "Platform engineers consolidating several model providers behind one internal service.",
    problem: "Teams often add one SDK, key path, and retry surface per provider. That creates scattered credentials, inconsistent defaults, and provider behavior that leaks into every caller.",
    solution: "LLM Proxy keeps the caller contract centered on GET, JSON POST, canonical POST /v2, and /dictate while routing to configured providers server-side.",
    steps: [
      "Configure supported providers and model catalogs in config.yml.",
      "Give each tenant a default provider and model, or let managed users choose defaults in Settings.",
      "Send provider and model only when a request needs to override the default route.",
      "Keep upstream API keys server-side and authenticate clients with the tenant secret.",
    ],
    features: [
      ["Provider selector", "Callers can choose providers per request without switching client libraries.", "Use provider=meta for Muse Spark 1.1 and provider=anthropic for Claude."],
      ["Configured model catalogs", "Unknown or unsupported model choices fail before upstream calls.", "Gemini and Claude max-token ceilings are validated at the edge."],
      ["One response contract", "Plain text, JSON, XML, and CSV formatting stay behind the proxy.", "Apps keep one integration while the backend owns provider differences."],
    ],
    examples: [
      ["Model trial", "A team tests Meta Muse Spark 1.1, Claude, and Gemini by changing provider selectors, not app code."],
      ["Internal gateway", "A platform team gives product apps a tenant secret while keeping OpenAI, Meta, Anthropic, Gemini, and xAI credentials in backend config or management storage."],
      ["Provider fallback planning", "Operators can keep non-default providers disabled until a key exists, then enable them without changing callers."],
    ],
    limitations: [
      "A provider must be wired through the proxy before callers can use it.",
      "Provider API keys are still required server-side before a provider can serve traffic.",
      "The proxy does not make unsupported upstream capabilities available by naming a provider.",
    ],
  }),
  page({
    slug: "server-side-provider-api-keys",
    category: "Security",
    primaryKeyword: "server-side provider API keys",
    title: "Keep provider API keys server-side for LLM apps",
    description: "Use tenant secrets for clients while upstream provider credentials stay on the LLM Proxy server.",
    audience: "Internal-tool builders who need AI calls without distributing raw upstream keys.",
    problem: "Client apps, notebooks, browser utilities, and scripts can drift into storing raw OpenAI, Anthropic, Gemini, or xAI keys. Once those keys leave the backend, rotation and audit work become harder.",
    solution: "LLM Proxy rejects upstream-provider-key-like fields on public proxy requests and reads provider credentials only from server-side config or authenticated management storage.",
    steps: [
      "Load provider credentials through config.yml or save them in the authenticated management UI.",
      "Give clients only an llm-proxy tenant secret.",
      "Route text and dictation requests with key=<tenant secret>.",
      "Rotate provider keys in the backend without changing every client integration.",
    ],
    features: [
      ["Tenant secret authentication", "Clients authenticate to the proxy, not directly to provider APIs.", "A browser app can call the proxy without handling an OpenAI key."],
      ["Provider-key rejection", "Public endpoints reject provider key fields in query, JSON, and multipart input.", "A mistaken api_key field fails before the upstream call."],
      ["Managed provider storage", "Signed-in users can persist provider keys through authenticated management endpoints.", "Mutation responses return masked status; raw retrieval is a separate owner-authenticated reveal action."],
    ],
    examples: [
      ["Browser dashboard", "A static Pages app can show copyable proxy examples without embedding upstream keys."],
      ["Notebook access", "A data notebook uses a tenant secret while provider credentials remain in the service boundary."],
      ["Provider rotation", "Ops updates the provider key once in backend state instead of touching each app."],
    ],
    limitations: [
      "A tenant secret is still sensitive and should be handled as an application credential.",
      "Encrypted-at-rest managed provider keys are not a zero-knowledge guarantee.",
      "Network controls are still recommended before exposing the service broadly.",
    ],
  }),
  page({
    slug: "tenant-secret-ai-gateway",
    category: "Security",
    primaryKeyword: "tenant secret AI gateway",
    title: "Tenant-secret AI gateway for internal applications",
    description: "Give client apps one llm-proxy tenant secret while the service owns provider credentials and routing.",
    audience: "Teams building internal apps that need a single guarded AI service boundary.",
    problem: "When every app gets its own provider credential and routing rules, access control becomes difficult to reason about and providers become embedded in product code.",
    solution: "LLM Proxy keeps public proxy requests authenticated by key=<tenant secret>, then applies the tenant's configured defaults and server-side provider credentials.",
    steps: [
      "Create a tenant in static config or through the management UI.",
      "Generate or configure a tenant secret for the client.",
      "Use the same key parameter across GET, POST, /v2, and /dictate.",
      "Revoke or regenerate the secret when access should change.",
    ],
    features: [
      ["Single credential surface", "Apps call one proxy secret instead of many provider keys.", "The same secret can protect text and dictation traffic."],
      ["Tenant defaults", "Omitted provider and model values resolve through tenant defaults.", "A caller can stay simple until it needs a per-request override."],
      ["403 boundary", "Missing, invalid, or revoked tenant secrets return forbidden responses.", "Callers do not receive tenant discovery details."],
    ],
    examples: [
      ["Shared app platform", "A platform team provisions tenant secrets for several internal apps while keeping provider credentials central."],
      ["Access revocation", "A generated secret can be revoked so future proxy calls with that secret stop working."],
      ["Dictation plus text", "The same tenant-secret model can protect voice transcription and LLM response generation."],
    ],
    limitations: [
      "Tenant secrets should not be placed in public client code without appropriate controls.",
      "Management mode uses database state for generated secrets; persistence must be configured.",
      "This is not a full identity provider; it is the proxy authentication boundary.",
    ],
  }),
  evidencedPage({
    slug: "self-service-llm-key-management",
    category: "Management UI",
    primaryKeyword: "self-service LLM key management",
    title: "Self-service LLM key management for internal teams",
    description: "Automatically create a signed-in user's LLM Proxy client key, then autosave one provider API key before they leave Settings.",
    audience: "Teams that want user-owned AI access without asking operators to edit YAML for every change.",
    problem: "Operator-provisioned AI access does not scale when each user or team needs provider keys, defaults, generated secrets, and examples updated separately.",
    solution: "LLM Proxy includes an optional TAuth-protected management UI that creates a missing client key after authentication, autosaves provider settings, and keeps Settings open until at least one managed provider key persists.",
    steps: [
      "Enable management mode with TAuth and database configuration.",
      "Publish the static Pages UI and serve runtime config from the API backend.",
      "Users sign in; the UI creates and presents a missing client key once.",
      "Users enter at least one provider key; the selected provider settings autosave before they leave Settings.",
    ],
    features: [
      ["TAuth-gated UI", "Management controls appear only after login.", "Unauthenticated users see the sign-in state, not tenant controls."],
      ["Required first-run setup", "Settings stays open until a client key and one persisted provider key exist.", "Typed drafts and local dotenv credentials do not bypass onboarding."],
      ["Selected-provider editor", "Provider key, text model, and system prompt live together and autosave.", "A user can update one provider without scanning every provider card or pressing a save button."],
      ["Copyable examples", "Default and provider-specific curl examples use the current proxy origin and generated-secret placeholder.", "Users can start with the exact request shape shown in Settings."],
    ],
    examples: [
      ["New team onboarding", "A user signs in, copies the automatically created client key, enters an OpenAI key, and closes Settings after autosave completes."],
      ["Provider update", "A user switches the selected provider editor to DeepSeek and its changed model autosaves."],
      ["Usage review", "The user returns to the dashboard to see 30-day request and token summaries."],
    ],
    limitations: [
      "Management mode requires configured TAuth, CORS origins, database settings, and provider-key encryption key.",
      "The backend serves management APIs and runtime config; the static Pages app is only the shell.",
      "Autosave responses return masked key status; raw retrieval requires the separate owner-authenticated reveal action.",
    ],
    repoExample: {
      source: "site/assets/llm-proxy/js/ui/keyManagement.js",
      verifiedOn: "2026-07-22",
      code: `if (this.settingsRequired) {
  this.openSettings();
}
if (!this.hasSecret) {
  await this.requestAndApplyGeneratedSecret();
}`,
    },
    quickVerdict: "Use this flow when every authenticated user must leave first-run Settings with a client key and at least one server-stored provider credential.",
    faq: [
      {
        question: "How does first-run LLM Proxy setup begin?",
        answer: "After MPR UI reports an authenticated session, the management profile loads and the UI automatically creates a client key when the profile has none.",
      },
      {
        question: "What lets a user leave Settings?",
        answer: "The loaded profile must report a client key and at least one persisted managed provider key. Until both conditions are true, close actions explain what is missing and keep Settings open.",
      },
      {
        question: "Does typing a provider key unlock Settings?",
        answer: "No. Only a provider record returned with has_key after a successful autosave counts; drafts and local dotenv credentials do not satisfy managed onboarding.",
      },
      {
        question: "Does onboarding change provider or model defaults?",
        answer: "No. Automatic client-key creation and provider-key validation leave the user's existing provider and model defaults unchanged.",
      },
    ],
  }),
  page({
    slug: "bring-your-own-provider-key-portal",
    category: "Management UI",
    primaryKeyword: "bring your own provider key portal",
    title: "Bring-your-own provider key portal for AI access",
    description: "Use LLM Proxy Settings to let users save provider keys and keep public proxy calls key-free.",
    audience: "Organizations where users or teams own their upstream provider accounts.",
    problem: "BYO provider keys can become risky when users paste keys into product apps, scripts, or support messages instead of a controlled backend surface.",
    solution: "The management UI accepts provider API keys only through authenticated management endpoints and keeps public proxy requests on the tenant-secret contract.",
    steps: [
      "Sign in through the configured MPR/TAuth shell.",
      "Open Settings and select the provider to configure.",
      "Save the provider API key with the selected text model and provider system prompt.",
      "Use generated proxy examples that never include upstream provider credentials.",
    ],
    features: [
      ["Provider selector in Settings", "Users edit one provider at a time with explicit fields.", "The UI avoids a wall of provider cards."],
      ["Masked key status", "Saved keys are represented by status, not raw values.", "Users can confirm a key exists without retrieving it."],
      ["Provider-specific examples", "Examples include provider selection when users need a provider route.", "The copy action stays separate for default and selected-provider examples."],
    ],
    examples: [
      ["Team-owned OpenAI account", "A team saves its own OpenAI key and uses the generated tenant secret in internal tools."],
      ["Specialized provider trial", "A user adds a Gemini key for one workflow while leaving default examples available."],
      ["Key removal", "The user removes a provider key and related settings from the selected-provider editor."],
    ],
    limitations: [
      "Users need an authenticated management session before saving provider keys.",
      "A provider without a key cannot serve requests for that tenant.",
      "Provider-key encryption protects storage exposure, not runtime operators from all access.",
    ],
  }),
  page({
    slug: "canonical-v2-chat-messages-api",
    category: "API contract",
    primaryKeyword: "v2 chat messages API",
    title: "Canonical /v2 chat messages API for LLM calls",
    description: "Send ordered system, user, and assistant messages through one v2 endpoint before provider routing.",
    audience: "Developers who want a stable chat transcript contract instead of provider-specific payloads.",
    problem: "Chat transcript callers can end up building OpenAI, Anthropic, Gemini, and compatible-provider request bodies separately, with different message rules in each client.",
    solution: "LLM Proxy exposes POST /v2 as the canonical chat endpoint. It accepts messages, optional model, optional web_search, and optional max_tokens, then maps the request to the selected provider.",
    steps: [
      "Send POST /v2 with messages[] and the tenant key query parameter.",
      "Use system role messages for instructions rather than a body system_prompt.",
      "Provide order values only when every submitted message has a unique non-negative order.",
      "Let omitted model resolve to the tenant or selected-provider default.",
    ],
    features: [
      ["Messages-only input", "The canonical endpoint rejects prompt and body system_prompt fields.", "Ambiguous input fails before an upstream call."],
      ["Order support", "Messages can be sorted by explicit order when callers cannot rely on array position.", "Event-sourced transcripts can still route deterministically."],
      ["Provider mapping", "The proxy maps shared messages into OpenAI, Claude, Gemini, or compatible provider shapes.", "Client code does not need a provider SDK."],
    ],
    examples: [
      ["Chat transcript", "A support tool sends system, user, and assistant messages to /v2 with provider omitted."],
      ["Provider experiment", "The same /v2 body is sent with provider=anthropic for a Claude trial."],
      ["Ordered events", "A caller sends order values from persisted event IDs so the proxy sorts the transcript."],
    ],
    limitations: [
      "POST /v2 requires at least one user message.",
      "Unsupported roles, duplicate order values, and mixed prompt fields return 400.",
      "Server-injected tenant default system prompts are sent upstream but not echoed in response metadata.",
    ],
  }),
  page({
    slug: "large-prompt-json-post",
    category: "API contract",
    primaryKeyword: "large prompt JSON POST",
    title: "Large prompt JSON POST for LLM requests",
    description: "Use JSON POST bodies when prompts are too large or structured for query-string requests.",
    audience: "Developers moving from quick prompt calls to large documents or generated prompt bodies.",
    problem: "GET query strings are convenient for small prompts, but large or non-ASCII prompts need a request body and clear size validation.",
    solution: "LLM Proxy supports compatibility POST / with JSON prompt or messages input and canonical POST /v2 for messages-only chat transcripts.",
    steps: [
      "Choose POST / for a prompt field or compatibility messages[] body.",
      "Choose POST /v2 for the canonical messages-only contract.",
      "Keep key and provider in query parameters.",
      "Let max_prompt_bytes enforce request size before provider routing.",
    ],
    features: [
      ["JSON body support", "Large prompts can move out of the URL without changing authentication.", "The tenant secret remains in the key query parameter."],
      ["Prompt size cap", "Oversized bodies fail with 413 before upstream provider calls.", "Operators can set max_prompt_bytes in config."],
      ["Conflict checks", "Conflicting query/body model values return 400.", "The proxy rejects ambiguous request shapes early."],
    ],
    examples: [
      ["Document summarization", "A backend sends a long document as JSON prompt text instead of URL encoding it."],
      ["Generated transcript", "A workflow posts messages[] with explicit order values through /v2."],
      ["Controlled token cap", "A caller includes max_tokens to limit one generation."],
    ],
    limitations: [
      "POST /v2 does not accept prompt or body system_prompt.",
      "JSON bodies still need the tenant secret in the query string.",
      "The proxy validates request size, but callers must still handle provider errors and timeouts.",
    ],
  }),
  page({
    slug: "audio-transcription-proxy-api",
    category: "Dictation",
    primaryKeyword: "audio transcription proxy API",
    title: "Audio transcription proxy API behind tenant secrets",
    description: "Route multipart audio transcription through /dictate using the same tenant-secret boundary as text.",
    audience: "Teams adding voice input or dictation to internal tools without a separate provider credential path.",
    problem: "Dictation integrations often grow a separate security and provider configuration path from text generation, even when the same apps need both.",
    solution: "LLM Proxy exposes POST /dictate for multipart audio and routes dictation-capable providers behind the same tenant-secret authentication model.",
    steps: [
      "Send multipart/form-data to /dictate with audio or file as the audio part.",
      "Authenticate with key=<tenant secret> in the query string.",
      "Omit provider and model for tenant defaults or select a dictation-capable provider.",
      "Receive JSON with the transcribed text.",
    ],
    features: [
      ["/dictate endpoint", "Audio transcription has a public contract separate from text but shares authentication.", "The success response is JSON with a text field."],
      ["Dictation model catalogs", "OpenAI, SiliconFlow, Zhipu, and Grok/xAI dictation models are configured explicitly.", "Unknown dictation models fail at the edge."],
      ["Audio size limit", "max_input_audio_bytes protects the upstream call boundary.", "Oversized audio returns a client error before routing."],
    ],
    examples: [
      ["Voice note ingestion", "An internal app uploads recording.webm to /dictate and receives a transcript."],
      ["Provider-specific transcription", "A caller selects provider=siliconflow when that tenant has a configured key."],
      ["Shared governance", "Text and voice traffic use one tenant-secret model and shared usage metadata rules."],
    ],
    limitations: [
      "Only providers with implemented dictation adapters are available through /dictate.",
      "The audio part is required; missing or invalid multipart forms return 400.",
      "Not all provider text models are dictation models.",
    ],
  }),
  page({
    slug: "openai-claude-gemini-one-endpoint",
    category: "Provider routing",
    primaryKeyword: "OpenAI Claude Gemini one endpoint",
    title: "OpenAI, Claude, and Gemini through one endpoint",
    description: "Use one proxy contract while LLM Proxy maps requests into OpenAI, Anthropic, and Gemini APIs.",
    audience: "Product teams comparing native model providers without adding three client integrations.",
    problem: "OpenAI Responses, Anthropic Messages, and Gemini generateContent have different payloads, model limits, auth headers, and response shapes.",
    solution: "LLM Proxy keeps a shared caller contract and routes native providers server-side with provider-specific adapters.",
    steps: [
      "Configure native provider base URLs, keys, and model catalogs.",
      "Send the same GET, POST, or /v2 request shape to llm-proxy.",
      "Select provider=openai, provider=anthropic, or provider=gemini when needed.",
      "Read the formatted proxy response and normalized usage metadata when available.",
    ],
    features: [
      ["Native provider adapters", "The backend handles OpenAI Responses, Anthropic Messages, and Gemini generateContent differences.", "Gemini messages become native contents; Claude system messages use Anthropic's system field."],
      ["Common status mapping", "Provider errors, timeouts, rate limits, and invalid inputs map to documented proxy statuses.", "Callers do not parse three provider error formats."],
      ["Model validation", "Model IDs are checked against configured provider catalogs.", "A Gemini model is not accepted under Anthropic routing."],
    ],
    examples: [
      ["Claude writing pass", "A team sends an existing /v2 transcript with provider=anthropic for editorial output."],
      ["Gemini summarization pass", "The same transcript shape routes to provider=gemini for a long summary workflow."],
      ["OpenAI web search task", "The caller selects an OpenAI model marked for web search and includes web_search=true."],
    ],
    limitations: [
      "Provider-specific capabilities remain provider-specific; web search is currently exposed only for configured OpenAI models.",
      "Native provider model catalogs must be kept current in config.",
      "The endpoint is shared, but the selected provider still controls upstream behavior and availability.",
    ],
  }),
  page({
    slug: "openai-background-response-polling",
    category: "Reliability",
    primaryKeyword: "OpenAI background response polling",
    title: "OpenAI background response polling without client loops",
    description: "Let LLM Proxy own OpenAI Responses background mode and return the final answer in one REST call.",
    audience: "Backend teams with long OpenAI prompts that should not require client polling logic.",
    problem: "Long OpenAI Responses work can push polling, resume tokens, or streaming complexity into every caller if the gateway does not own the lifecycle.",
    solution: "LLM Proxy sends stored OpenAI background requests upstream and polls server-side until the answer is terminal or the configured request deadline expires.",
    steps: [
      "Call GET, POST, or /v2 as a normal blocking proxy request.",
      "Let the OpenAI adapter use background: true and store: true internally.",
      "Wait for the same HTTP response to return the final formatted answer.",
      "Treat 504 as the proxy deadline expiring, not as a prompt to poll llm-proxy.",
    ],
    features: [
      ["One-shot REST contract", "Clients do not stream, poll, or follow resume endpoints.", "The final answer arrives in the original response."],
      ["Server-side polling", "The backend polls stored OpenAI response IDs internally.", "Provider lifecycle details stay out of product code."],
      ["Timeout ownership", "server.request_timeout_seconds bounds the overall proxy request.", "A timeout becomes a documented 504."],
    ],
    examples: [
      ["Semantic review", "A workflow sends a long JSON-only review prompt and waits on one proxy request."],
      ["CLI caller", "The bundled CLI can stay a simple v2 transport without polling support."],
      ["Backend integration", "A service keeps its existing synchronous request path while the proxy handles OpenAI background work."],
    ],
    limitations: [
      "The client still blocks until the proxy returns or times out.",
      "OpenAI-specific background behavior does not imply other providers support the same upstream mode.",
      "Very long provider work must fit within the configured request timeout.",
    ],
  }),
  page({
    slug: "upstream-worker-queue-limits",
    category: "Reliability",
    primaryKeyword: "upstream worker queue limits",
    title: "Upstream worker and queue limits for LLM traffic",
    description: "Use shared worker and queue controls to bound upstream HTTP operations for text and dictation.",
    audience: "Operators who need predictable capacity limits for provider HTTP calls.",
    problem: "Unlimited upstream calls can exhaust provider quotas or local resources, while long OpenAI polling sleeps should not occupy scarce worker capacity.",
    solution: "LLM Proxy combines server.workers and server.queue_size concurrency bounds with server.upstream_rate_limits rolling-window rules applied at actual upstream admission.",
    steps: [
      "Set server.workers for active upstream HTTP concurrency.",
      "Set server.queue_size for pending upstream operations.",
      "Set server.upstream_rate_limits rules for strict call budgets keyed by normalized upstream origin.",
      "Let OpenAI background poll sleeps release worker capacity between polls.",
      "Handle 503 request queue full and 504 timeout responses at the caller boundary.",
    ],
    features: [
      ["Shared limiter", "Text providers and dictation use the same upstream HTTP operation limit.", "Capacity policy is centralized."],
      ["Queue pressure signal", "A full queue returns service-unavailable behavior.", "Callers can distinguish overload from provider failure."],
      ["Polling separation", "OpenAI poll sleeps do not occupy worker slots.", "Other requests can proceed while a background response waits."],
    ],
    examples: [
      ["Low-capacity environment", "A local deployment sets one worker and a small queue to keep provider traffic controlled."],
      ["Mixed text and audio", "Dictation and text calls share the same upstream HTTP admission boundary."],
      ["Operational debugging", "A queue-full status points to proxy-side capacity, not a malformed request."],
    ],
    limitations: [
      "These controls limit upstream HTTP operations, not the number of connected client requests.",
      "They do not replace provider-side rate limits.",
      "Rate rules are keyed by exact normalized HTTP(S) origin, so providers sharing an origin share the configured budget.",
    ],
  }),
  page({
    slug: "model-catalog-configuration",
    category: "Configuration",
    primaryKeyword: "LLM model catalog configuration",
    title: "LLM model catalog configuration in config.yml",
    description: "Keep provider model IDs, defaults, web-search flags, and token limits in runtime config.",
    audience: "Operators maintaining provider model availability without changing application code.",
    problem: "Model lists change faster than client release cycles. Hardcoded model IDs in callers make provider updates brittle.",
    solution: "LLM Proxy treats provider model catalogs as runtime configuration while provider transports stay code-owned.",
    steps: [
      "Declare each provider text catalog and default model in config.yml.",
      "Declare dictation catalogs for dictation-capable providers.",
      "Add OpenAI request profiles and web_search flags only where supported.",
      "Restart the service after changing runtime config.",
    ],
    features: [
      ["Config-owned models", "Model IDs and defaults live in config instead of client code.", "A new provider model can be added without releasing every app."],
      ["Startup validation", "Blank, duplicate, and missing default model entries fail startup.", "Bad catalogs are caught before serving traffic."],
      ["Provider metadata", "Output token limits and OpenAI request profiles are explicit.", "Known ceilings can be rejected before upstream calls."],
    ],
    examples: [
      ["Gemini catalog update", "An operator adds a Gemini model ID and its output token limit to config.yml."],
      ["Claude default change", "A tenant selects a configured Claude default without changing client calls."],
      ["OpenAI web-search gate", "Only OpenAI model entries marked with web_search expose that request option."],
    ],
    limitations: [
      "Adding a provider that has no adapter still requires code work.",
      "Config changes require restart to take effect.",
      "Unknown YAML keys fail startup under the current strict config contract.",
    ],
  }),
  page({
    slug: "provider-default-model-selection",
    category: "Configuration",
    primaryKeyword: "provider default model selection",
    title: "Provider default model selection for omitted models",
    description: "Let omitted model fields resolve through tenant defaults or selected-provider configured defaults.",
    audience: "Client developers and operators who want explicit defaults without hardcoding a model in every request.",
    problem: "If clients omit model, each provider route needs a clear rule. Otherwise requests can accidentally inherit a stale model from the wrong provider.",
    solution: "LLM Proxy resolves omitted model through the tenant default when provider is omitted, otherwise through the selected provider's configured default.",
    steps: [
      "Set each provider text default_model in config.yml.",
      "Set tenant defaults for omitted-provider requests.",
      "Send provider only when a request needs a provider override.",
      "Omit model in bundled clients when the caller wants provider defaults.",
    ],
    features: [
      ["Provider-scoped defaults", "A Gemini request without model uses Gemini's configured default.", "It does not inherit an OpenAI model name."],
      ["Client omission support", "Go, CLI, and Python v2 clients omit model when no model is specified.", "Provider default behavior stays server-owned."],
      ["Startup guardrails", "A keyed provider must have a valid configured text default model.", "Invalid defaults fail before traffic starts."],
    ],
    examples: [
      ["Provider trial", "A caller sets provider=gemini and omits model to use gemini-2.5-flash from config."],
      ["Default route", "A client omits provider and model to use the tenant default OpenAI route."],
      ["Managed provider settings", "A user saves a provider-specific text model in Settings for generated-secret traffic."],
    ],
    limitations: [
      "Omitted model behavior depends on current config and management state.",
      "A provider selected with an invalid model still returns 400.",
      "Dictation has separate provider and model defaults.",
    ],
  }),
  page({
    slug: "openai-web-search-guardrails",
    category: "API contract",
    primaryKeyword: "OpenAI web search guardrails",
    title: "OpenAI web search guardrails in an LLM proxy",
    description: "Expose web_search only when the selected OpenAI model is configured to support it.",
    audience: "Teams that need controlled search-enabled model calls without making web search a universal flag.",
    problem: "A generic web_search flag can be misleading when only some providers and models support a search tool.",
    solution: "LLM Proxy accepts web_search per request but enables it only for OpenAI model catalog entries marked with web_search support.",
    steps: [
      "Mark supported OpenAI model entries with web_search: true in config.yml.",
      "Send web_search=1 or web_search=true only when the selected route supports it.",
      "Expect unsupported provider/model combinations to fail before an upstream call.",
      "Keep non-OpenAI providers on normal text routing.",
    ],
    features: [
      ["Model-aware flag", "The request option is checked against provider and model metadata.", "DeepSeek with web_search returns a client error instead of a hidden fallback."],
      ["Config-owned support", "OpenAI web-search support is explicit in the model catalog.", "Operators can see which model entries expose the capability."],
      ["Early rejection", "Unsupported combinations fail before provider spend.", "Callers get a clear request error."],
    ],
    examples: [
      ["Research prompt", "A caller selects gpt-5 with web_search=1 for a search-enabled answer."],
      ["Non-search provider", "A Gemini request with web_search does not silently pretend to search."],
      ["Default model check", "If the tenant default model lacks web search, the request must choose a supporting OpenAI model."],
    ],
    limitations: [
      "Web search is currently exposed only for configured OpenAI model entries.",
      "The proxy does not claim provider-side search for providers without an implemented search adapter.",
      "Search-enabled answers are still provider outputs and should be evaluated by the caller.",
    ],
  }),
  page({
    slug: "normalized-token-usage-metadata",
    category: "Usage",
    primaryKeyword: "normalized token usage metadata",
    title: "Normalized token usage metadata across providers",
    description: "Expose request, response, and total token counts through common headers and JSON fields.",
    audience: "Teams that need operational usage signals without parsing every provider's response shape.",
    problem: "Providers report usage differently, and response format choices can make token accounting disappear from caller code.",
    solution: "LLM Proxy normalizes upstream token usage into common response headers and JSON usage fields when providers return token metadata.",
    steps: [
      "Call GET, POST, or /v2 with the desired response format.",
      "Read X-LLM-Proxy-Request-Tokens, X-LLM-Proxy-Response-Tokens, and X-LLM-Proxy-Total-Tokens when present.",
      "Use JSON format when the caller wants usage inside the response body.",
      "Review management usage dashboards for managed tenant aggregates.",
    ],
    features: [
      ["Common headers", "Token counts stay available for plain text, CSV, and XML bodies.", "The body format does not erase usage metadata."],
      ["JSON usage object", "JSON responses include normalized request, response, and total counts.", "Callers can store one shape."],
      ["Managed usage aggregation", "Usage events record normalized token counts without storing prompts or responses.", "Dashboards can show 30-day token totals."],
    ],
    examples: [
      ["Plain text caller", "A CLI reads the text body and token headers separately."],
      ["Dashboard metric", "The management UI shows request and token graphs for the signed-in tenant."],
      ["Provider comparison", "A team compares usage by provider and model using the normalized metadata available in management mode."],
    ],
    limitations: [
      "Usage metadata appears only when the upstream provider returns token information.",
      "Normalized counts are operational signals, not pricing calculations.",
      "Management usage stores metadata and excludes prompts, transcripts, responses, provider keys, and tenant secrets.",
    ],
  }),
  page({
    slug: "managed-tenant-usage-dashboard",
    category: "Usage",
    primaryKeyword: "managed tenant usage dashboard",
    title: "Managed tenant usage dashboard for LLM requests",
    description: "Show signed-in users 30-day request, token, provider, model, and status summaries.",
    audience: "Teams giving users self-service AI access while keeping usage visible.",
    problem: "A key-management portal is incomplete if users cannot see whether their managed proxy traffic is succeeding or which providers and models they use.",
    solution: "LLM Proxy's authenticated landing screen is a usage dashboard for the signed-in user's managed tenant.",
    steps: [
      "Enable management mode and generated-secret routing.",
      "Send proxy requests with the generated tenant secret.",
      "Record usage metadata for managed-tenant requests.",
      "View 30-day summaries through GET /api/management/usage and the dashboard UI.",
    ],
    features: [
      ["30-day totals", "Requests, tokens, success rate, providers, and models are visible after sign-in.", "Users can spot usage and error trends."],
      ["Daily buckets", "The dashboard renders request and token trend charts.", "Traffic changes are easier to inspect."],
      ["Sensitive-data exclusion", "Stored usage excludes prompts, audio, transcripts, responses, tenant secrets, and provider keys.", "Operational metrics do not become a content database."],
    ],
    examples: [
      ["User self-check", "A user sees that recent requests are mostly successful before debugging application code."],
      ["Provider breakdown", "A team sees whether traffic is going to OpenAI or DeepSeek."],
      ["Token trend", "The dashboard shows token totals rising after a new workflow launches."],
    ],
    limitations: [
      "Usage is recorded for managed tenants using generated secrets.",
      "The dashboard is not a billing system.",
      "Persistence requires a configured management database.",
    ],
  }),
  page({
    slug: "admin-usage-visibility-without-secrets",
    category: "Usage",
    primaryKeyword: "admin usage visibility without secrets",
    title: "Admin usage visibility without exposing secrets",
    description: "Let configured administrators inspect managed users and 30-day usage without raw keys or prompts.",
    audience: "Operators who need oversight of managed AI access without turning dashboards into sensitive data exports.",
    problem: "Admin views can become dangerous if they show generated secrets, provider keys, prompts, transcripts, or model responses.",
    solution: "LLM Proxy exposes admin-only user and usage summaries while explicitly excluding provider API keys, masked key strings, generated secrets, secret digests, prompts, responses, audio names, and transcripts.",
    steps: [
      "Configure administrator emails in management.admin_emails.",
      "Authenticate through TAuth with a matching email.",
      "Open the Admin item from the avatar menu.",
      "Review tenant facts and 30-day usage summaries for managed users.",
    ],
    features: [
      ["Config-owned admins", "Admin status comes from validated TAuth email and config.", "There is no hidden browser-only admin toggle."],
      ["Admin API", "GET /api/management/admin/users returns managed user and usage facts.", "Non-admin sessions receive 403."],
      ["No secret leakage", "Admin responses omit raw and masked provider keys plus generated secrets.", "Oversight stays operational."],
    ],
    examples: [
      ["Support triage", "An admin checks whether a user has a tenant secret and recent usage before asking for logs."],
      ["Traffic review", "An operator reviews user-level request counts and token totals."],
      ["Access check", "A non-admin user cannot call the admin endpoint successfully."],
    ],
    limitations: [
      "Admin emails must be configured exactly in runtime config.",
      "The admin dashboard summarizes usage; it does not expose prompt content.",
      "The API depends on valid TAuth session cookies.",
    ],
  }),
  page({
    slug: "api-served-runtime-config-for-static-ui",
    category: "Deployment",
    primaryKeyword: "API-served runtime config static UI",
    title: "API-served runtime config for a static LLM UI",
    description: "Keep the Pages frontend static while the backend serves current runtime config at /config-ui.yaml.",
    audience: "Teams deploying a split-origin static management UI and backend API.",
    problem: "Static frontends can accidentally ship stale API origins, OAuth values, or runtime config if those values are rendered into the artifact.",
    solution: "LLM Proxy's Pages app fetches backend-owned /config-ui.yaml at runtime, and the Pages artifact contains no static config-ui.yaml or llm-proxy-config.json.",
    steps: [
      "Publish the static UI from site/ to GitHub Pages.",
      "Run the backend with management config loaded from config.yml.",
      "Serve /config-ui.yaml from the API origin with allowed frontend CORS.",
      "Let the browser use that YAML for management API origin, proxy origin, MPR UI, and TAuth bootstrap.",
    ],
    features: [
      ["Single browser config surface", "The API backend projects runtime config into /config-ui.yaml.", "Pages does not own a duplicate JSON config."],
      ["Split-origin support", "managementApiOrigin and proxyOrigin are served to the browser.", "The UI can call llm-proxy-api while hosted on llm-proxy."],
      ["Publish validation", "The Pages publisher rejects forbidden static config artifacts.", "Stale config files are removed from the rendered artifact."],
    ],
    examples: [
      ["Production Pages host", "llm-proxy.mprlab.com serves the static UI while llm-proxy-api.mprlab.com serves config and APIs."],
      ["Local preview", "A test server can serve /config-ui.yaml for browser checks without editing the static HTML."],
      ["Config rotation", "Updating backend runtime config changes browser config without rebuilding the Pages artifact."],
    ],
    limitations: [
      "The static UI needs the backend config endpoint to load fully.",
      "CORS and TAuth origins must match the hosted profile.",
      "Do not publish static config files as a fallback.",
    ],
  }),
  page({
    slug: "tauth-protected-management-api",
    category: "Management UI",
    primaryKeyword: "TAuth protected management API",
    title: "TAuth-protected management API for LLM Proxy",
    description: "Gate tenant, provider key, defaults, usage, and admin APIs behind validated TAuth sessions.",
    audience: "Teams adopting the MPR/TAuth shell for authenticated AI self-service.",
    problem: "Key management APIs need a stronger boundary than a public static page. They must know who is signed in and which tenant that user owns.",
    solution: "LLM Proxy validates configured TAuth session cookies on /api/management/* and returns unauthenticated or forbidden responses for invalid sessions.",
    steps: [
      "Configure TAuth URL, tenant ID, session cookie name, issuer, and signing key.",
      "Serve browser-facing MPR UI/TAuth config through /config-ui.yaml.",
      "Require authenticated sessions before returning profile, provider, default, secret, usage, or admin data.",
      "Use the shared MPR header, user menu, and footer in the static UI.",
    ],
    features: [
      ["Session validation", "Management APIs validate TAuth session cookies server-side.", "Unauthenticated requests return 401."],
      ["Tenant ownership", "Signed-in users manage only their own tenant secrets, provider keys, defaults, and examples.", "Tenant data is user-scoped."],
      ["Admin derivation", "Admin status is derived from configured emails and authenticated session data.", "Admin APIs return 403 for non-admin users."],
    ],
    examples: [
      ["Profile load", "The static UI calls /api/management/profile after TAuth reports authentication."],
      ["Settings mutation", "Provider key saves and secret generation require JSON content and the public origin."],
      ["Admin dashboard", "A configured admin receives an Admin menu item after profile load."],
    ],
    limitations: [
      "TAuth tenant and cookie settings must match the deployment profile.",
      "The public proxy endpoints still use tenant-secret authentication, not TAuth sessions.",
      "The static UI should not load tauth.js directly; it uses the MPR shell contract.",
    ],
  }),
  evidencedPage({
    slug: "generated-secret-rotation-and-revocation",
    category: "Security",
    primaryKeyword: "generated LLM proxy secret rotation",
    title: "Generated LLM Proxy secret rotation and revocation",
    description: "Automatically create a missing LLM Proxy client key, show it once, store only its digest, and support explicit rotation and revocation.",
    audience: "Teams that want self-service client access without permanent retrievable secrets.",
    problem: "Long-lived client secrets become harder to control when users can retrieve old raw values or when revocation requires operator edits.",
    solution: "LLM Proxy generates tenant secrets, returns them once, stores only SHA-256 digests, and supports revocation through authenticated management APIs.",
    steps: [
      "Open Settings after signing in.",
      "Copy the one-time client key created automatically for a profile that does not have one.",
      "Use the generated secret in public proxy request examples.",
      "Revoke or regenerate the secret when access should change.",
    ],
    features: [
      ["One-time display", "Generated secrets are shown once after creation.", "The database stores only their digest."],
      ["Immediate revocation", "Revoked secrets stop authenticating future public proxy requests.", "Access can be cut without provider-key rotation."],
      ["Secret-safe examples", "Request examples retain the <generated-secret> placeholder after creation.", "Raw client keys remain confined to the one-time Key field."],
    ],
    examples: [
      ["New app secret", "A developer copies the automatically created client key and substitutes it for the /v2 example placeholder."],
      ["Compromised client", "A user revokes the generated secret and creates a replacement."],
      ["Provider key unchanged", "Rotating the tenant secret does not require changing the saved provider API key."],
    ],
    limitations: [
      "If the one-time value is lost, the user must generate a new secret.",
      "Tenant secrets remain application credentials and need normal secret handling.",
      "Revocation affects proxy authentication; it does not revoke upstream provider keys.",
    ],
    repoExample: {
      source: "site/assets/llm-proxy/js/core/backendClient.js",
      verifiedOn: "2026-07-22",
      code: `export function generateSecret() {
  return requestJSON(\`\${MANAGEMENT_BASE_PATH}/secrets\`, { method: "POST" });
}

export function revokeSecret() {
  return requestJSON(\`\${MANAGEMENT_BASE_PATH}/secrets\`, { method: "DELETE" });
}`,
    },
    quickVerdict: "Use the automatically created client key once, then replace or revoke it without rotating the separate upstream provider credentials.",
    faq: [
      {
        question: "When does LLM Proxy create a client key?",
        answer: "The management UI creates one after authentication when the current profile reports that no client key exists. A configured profile does not trigger another creation request.",
      },
      {
        question: "Can the raw generated client key be retrieved later?",
        answer: "No. The raw value is presented once in a masked, read-only field, while the backend stores only the digest used to authenticate proxy requests.",
      },
      {
        question: "What does revoking the client key affect?",
        answer: "Revocation stops that client key from authenticating public proxy requests. It does not remove or rotate separately stored upstream provider credentials.",
      },
      {
        question: "Do request examples embed the generated key?",
        answer: "No. Examples retain the <generated-secret> placeholder so the one-time raw value does not enter page markup or copied example commands.",
      },
    ],
  }),
  page({
    slug: "encrypted-provider-key-storage",
    category: "Security",
    primaryKeyword: "encrypted provider key storage",
    title: "Encrypted provider key storage for managed tenants",
    description: "Store tenant-owned provider API keys with AES-GCM encryption at rest and honest security wording.",
    audience: "Teams evaluating how LLM Proxy stores BYO provider credentials in management mode.",
    problem: "Provider API keys are high-value secrets. A management database should not store raw upstream credentials as plaintext rows.",
    solution: "LLM Proxy requires a base64 32-byte provider-key encryption key in management mode and encrypts managed provider API keys at rest with AES-GCM and row-bound associated data.",
    steps: [
      "Generate a base64 32-byte management.provider_key_encryption_key.",
      "Configure it as a backend deployment secret.",
      "Save provider API keys through authenticated management endpoints.",
      "Let startup migrate existing plaintext rows into encrypted records.",
    ],
    features: [
      ["AES-GCM storage", "Provider keys are encrypted before database persistence.", "Database dumps and direct storage access do not expose raw values."],
      ["Startup migration", "Existing plaintext rows are encrypted and cleared on management startup.", "The bridge is bounded to current schema migration."],
      ["Honest guarantee", "Docs describe encrypted-at-rest storage, not zero-knowledge storage.", "The runtime decrypts keys when routing upstream."],
    ],
    examples: [
      ["SQLite local profile", "A local management database stores encrypted provider key rows."],
      ["Postgres hosted profile", "Hosted management state uses the same encrypted key contract."],
      ["Storage exposure reduction", "A database backup does not contain raw provider API keys."],
    ],
    limitations: [
      "The backend must decrypt provider keys to call upstream APIs.",
      "Losing the encryption key can break access to stored provider keys.",
      "This does not remove the need for secure deployment secret handling.",
    ],
  }),
  page({
    slug: "reject-client-provider-key-leaks",
    category: "Security",
    primaryKeyword: "reject client provider key leaks",
    title: "Reject client-supplied provider key leaks",
    description: "Fail public proxy requests that try to send upstream provider API keys in query, JSON, or multipart input.",
    audience: "Security-conscious teams that want mistakes to fail before provider credentials spread.",
    problem: "A caller may accidentally include an OpenAI or provider api_key field in a proxy request body, query string, or multipart form.",
    solution: "LLM Proxy public endpoints reject provider-key-like fields so upstream credentials stay in server-side configuration or authenticated management storage.",
    steps: [
      "Keep upstream provider credentials out of public proxy requests.",
      "Authenticate clients only with key=<tenant secret>.",
      "Use management APIs for provider key save/update/removal.",
      "Treat rejected provider-key fields as integration bugs to remove.",
    ],
    features: [
      ["Query rejection", "Provider-key-like query fields fail at the public endpoint.", "A leaked api_key parameter is not forwarded."],
      ["JSON rejection", "Text request bodies cannot carry upstream provider keys.", "Prompt payloads stay separate from provider credentials."],
      ["Multipart rejection", "Dictation forms reject provider-key-like fields too.", "Audio upload paths follow the same boundary."],
    ],
    examples: [
      ["Frontend mistake", "A form accidentally posts provider_api_key and receives a client error."],
      ["Notebook cleanup", "A notebook migration removes OPENAI_API_KEY from request bodies and uses the tenant secret instead."],
      ["Dictation upload", "A multipart /dictate request includes only audio and proxy authentication."],
    ],
    limitations: [
      "This catches provider-key-like request fields; it does not inspect arbitrary prompt text for secrets.",
      "Tenant secrets must still be protected by client teams.",
      "Provider credentials belong in backend config or authenticated management storage.",
    ],
  }),
  page({
    slug: "strict-yaml-config-placeholders",
    category: "Configuration",
    primaryKeyword: "strict YAML config placeholders",
    title: "Strict YAML config placeholders for LLM Proxy",
    description: "Use config.yml as the only service config source and env only for strict placeholder expansion.",
    audience: "Operators who want predictable startup behavior and no hidden runtime defaults.",
    problem: "Services that merge flags, env, defaults, and files can start with surprising configuration. Missing secrets may appear only when traffic arrives.",
    solution: "LLM Proxy reads service configuration from config.yml, expands ${NAME} placeholders from env or a sibling .env file, rejects unknown YAML keys, and fails startup for missing required placeholders.",
    steps: [
      "Put service configuration in config.yml.",
      "Use process environment or a sibling .env only to satisfy placeholders.",
      "Avoid ${NAME:-default}; the loader supports plain ${NAME}.",
      "Let startup fail when required placeholders or unknown keys are present.",
    ],
    features: [
      ["Single config file", "Runtime code receives only validated config values.", "Flags and env are not alternate service config sources."],
      ["Strict placeholders", "Missing placeholders fail startup outside the exact optional provider-key case.", "Configuration errors are visible before traffic."],
      ["Unknown-key rejection", "Stale or misspelled YAML keys fail instead of being ignored.", "Forward-only config stays clean."],
    ],
    examples: [
      ["Hosted profile", "Deployment secrets provide LLM_PROXY_MANAGEMENT_* placeholder values."],
      ["Local profile", "configs/.env supplies SQLite management database values for local runs."],
      ["Provider disabled", "A non-default provider api_key that is exactly a missing placeholder can expand to blank and remain disabled."],
    ],
    limitations: [
      "Changing config requires restart.",
      "The optional-provider-key exception applies only to the whole api_key value.",
      "This is a config discipline feature, not a secrets manager.",
    ],
  }),
  page({
    slug: "static-to-managed-tenant-migration",
    category: "Configuration",
    primaryKeyword: "static to managed tenant migration",
    title: "Legacy token ownership migration in management mode",
    description: "Claim one prior static-config token for a verified user account without changing the token or losing usage history.",
    audience: "Operators retiring the final unowned management-mode token after moving to self-service accounts.",
    problem: "A token imported by an older release can still belong to a synthetic static-config user, so its real owner cannot see that token's usage after signing in.",
    solution: "LLM Proxy rejects the unowned token, then atomically replaces any empty account created by an earlier sign-in and rekeys the legacy tenant, encrypted provider settings, and usage events when the configured owner email signs in through TAuth.",
    steps: [
      "Configure the exact legacy tenant id and deployment-owned target email.",
      "Deploy the current binary and drain older service instances before the owner claim.",
      "Sign in with the configured email and verify the same token plus historical usage.",
      "Remove the temporary migration config after production verification.",
    ],
    features: [
      ["Verified owner claim", "Only a live TAuth session whose normalized email matches the configured owner can claim the tenant.", "The migration does not trust stored email alone."],
      ["Token and usage continuity", "The token digest, tenant defaults, creation time, and every usage event are preserved.", "Existing clients keep their token while the dashboard gains its history."],
      ["Provider key re-encryption", "Provider ciphertext is decrypted under the synthetic user id and re-encrypted for the TAuth subject inside one GORM transaction.", "Ownership changes without copying invalid ciphertext."],
    ],
    examples: [
      ["Unowned token before claim", "The legacy token returns 403 instead of continuing as a global system credential."],
      ["First matching sign-in", "The configured owner opens the management site and receives the existing tenant and usage history."],
      ["Post-migration cleanup", "Operators remove the temporary owner mapping after verifying the unchanged token in production."],
    ],
    limitations: [
      "The source must be the one configured static-config tenant; an existing destination is replaceable only when it has no generated secret, provider settings, or usage.",
      "A populated destination conflict returns 409 without merging or overwriting account state.",
      "Old service instances must be drained before the owner signs in.",
    ],
  }),
  page({
    slug: "gorm-managed-tenant-persistence",
    category: "Configuration",
    primaryKeyword: "GORM managed tenant persistence",
    title: "GORM-managed tenant persistence for LLM Proxy",
    description: "Persist signups, provider settings, generated secret digests, defaults, and usage through GORM.",
    audience: "Backend operators deciding how management-mode state is stored.",
    problem: "Self-service management needs persistent tenant state without mutating runtime config files or adding raw SQL paths.",
    solution: "LLM Proxy stores managed tenants, provider keys, defaults, generated secret digests, and usage events in a GORM-managed database.",
    steps: [
      "Configure management.database_dialect as postgres or sqlite.",
      "Configure management.database_dsn for the selected dialect.",
      "Run management mode with the required provider-key encryption key.",
      "Let GORM model APIs own signup, provider, defaults, secret, and usage state.",
    ],
    features: [
      ["Postgres or SQLite", "Hosted and local management profiles can use supported GORM dialects.", "SQLite uses a pure-Go driver for CGO_DISABLED builds."],
      ["No runtime config mutation", "User signup and provider enablement update database state, not config.yml.", "Configuration stays operator-owned."],
      ["Usage event persistence", "Managed request metadata is stored with tenant isolation.", "Dashboards survive restarts."],
    ],
    examples: [
      ["Local development", "A developer uses sqlite with a local ignored database file."],
      ["Hosted deployment", "Production uses a Postgres DSN supplied through deployment secrets."],
      ["Usage dashboard", "The dashboard reads aggregate usage from persisted events."],
    ],
    limitations: [
      "Only postgres and sqlite are documented supported dialects.",
      "Database outages can affect management state and dashboard access.",
      "Provider transport config and model catalogs still live in config.yml.",
    ],
  }),
  page({
    slug: "go-client-v2-only-llm-proxy",
    category: "Clients",
    primaryKeyword: "Go LLM proxy client v2",
    title: "Go LLM Proxy client with a v2-only transport",
    description: "Use the Go package to send canonical messages requests through POST /v2.",
    audience: "Go developers integrating application backends with LLM Proxy.",
    problem: "Reusable clients can expose too many legacy request shapes and force callers to choose between prompt JSON and chat messages.",
    solution: "The Go package under pkg/llmproxyclient exposes the canonical MessagesRequest and Client.PostMessages path for text requests.",
    steps: [
      "Import github.com/tyemirov/llm-proxy/pkg/llmproxyclient.",
      "Build a MessagesRequest with NewMessagesRequest.",
      "Configure base URL and tenant secret.",
      "Let omitted model stay omitted when provider defaults should apply.",
    ],
    features: [
      ["v2-only API", "The reusable package exposes messages requests rather than multiple text shapes.", "Callers standardize on POST /v2."],
      ["Provider query preservation", "Base URLs can include non-payload query parameters such as provider.", "Provider-selected requests still use the canonical body."],
      ["Omitted-model behavior", "The client omits model unless the caller specifies it.", "Server-side defaults remain authoritative."],
    ],
    examples: [
      ["Backend service", "A Go service sends system and user messages through Client.PostMessages."],
      ["Provider-specific base URL", "The base URL includes ?provider=gemini while the request body omits model."],
      ["Max-token override", "A caller adds max_tokens for one request without changing provider defaults."],
    ],
    limitations: [
      "The Go client is for text messages, not /dictate multipart uploads.",
      "It does not implement OpenAI background polling because the server owns that lifecycle.",
      "Direct REST callers can still use server GET and compatibility POST endpoints outside the client package.",
    ],
  }),
  page({
    slug: "python-client-v2-only-llm-proxy",
    category: "Clients",
    primaryKeyword: "Python LLM proxy client v2",
    title: "Python LLM Proxy client with v2 messages",
    description: "Use ClientMessagesRequest and post_messages for canonical text requests from Python.",
    audience: "Python workflow authors and service developers standardizing on the /v2 messages contract.",
    problem: "Python callers often start with raw requests and then duplicate provider-specific payload details in scripts.",
    solution: "The Python package exposes Client, ClientConfig, ClientMessage, ClientMessagesRequest, and Client.post_messages for v2-only text transport.",
    steps: [
      "Install the package from the repository.",
      "Create ClientConfig with base_url and secret.",
      "Build ClientMessagesRequest with one or more ClientMessage values.",
      "Call post_messages and let the proxy route provider details.",
    ],
    features: [
      ["Messages request object", "Callers send system, user, and assistant messages in one typed request shape.", "Chat workflows avoid ad hoc JSON."],
      ["Optional order", "Messages can include order values when array order is not enough.", "The proxy sorts before routing."],
      ["Transport context", "Python client errors include non-secret provider, model, and timeout context.", "Debugging avoids leaking credentials."],
    ],
    examples: [
      ["Scripted summary", "A Python script sends one user message to a configured provider default."],
      ["Transcript replay", "A workflow sends ordered chat messages for review."],
      ["Provider test", "The client base URL includes provider=deepseek and the body omits model."],
    ],
    limitations: [
      "The package is v2-only for text requests.",
      "It does not send raw provider API keys.",
      "Dictation requires direct multipart HTTP usage today.",
    ],
  }),
  page({
    slug: "installable-llm-proxy-cli",
    category: "Clients",
    primaryKeyword: "installable LLM proxy CLI",
    title: "Installable LLM Proxy CLI for prompt workflows",
    description: "Use llm-proxy-client to send prompt text as canonical /v2 messages from the command line.",
    audience: "Developers who want a simple shell client for tenant-secret authenticated LLM calls.",
    problem: "Curl is useful, but repeated prompt workflows need a small client that understands the proxy's canonical text contract.",
    solution: "The installable Go CLI reads prompt text from flags, files, or stdin, maps it into a v2 user message, and sends POST /v2 through the reusable Go client.",
    steps: [
      "Install with go install github.com/tyemirov/llm-proxy/llm-proxy-client@latest.",
      "Set --base-url and --secret, or use LLM_PROXY_BASE_URL and LLM_PROXY_SECRET.",
      "Send --prompt, --prompt-file, or stdin content.",
      "Optionally include --system-prompt, --model, max tokens, or provider in the base URL.",
    ],
    features: [
      ["Canonical POST /v2", "The CLI sends prompt input as v2 messages.", "CLI behavior matches reusable client behavior."],
      ["Environment support", "Base URL and secret can come from env for shell workflows.", "Prompt content can flow from stdin."],
      ["Payload/query cleanup", "The client strips body-owned query fields and preserves non-payload query parameters.", "Provider selection can stay in the base URL."],
    ],
    examples: [
      ["Quick summary", "Pipe text into llm-proxy-client with a configured base URL and secret."],
      ["Provider route", "Use a base URL with ?provider=gemini to test Gemini without changing the body."],
      ["System instruction", "Pass --system-prompt so the CLI sends a v2 system message."],
    ],
    limitations: [
      "The CLI is a text client; it does not upload audio to /dictate.",
      "It still requires a valid tenant secret.",
      "It is not a provider SDK and does not bypass proxy validation.",
    ],
  }),
  page({
    slug: "llm-response-formats-json-xml-csv-text",
    category: "API contract",
    primaryKeyword: "LLM response formats JSON XML CSV text",
    title: "LLM response formats: JSON, XML, CSV, and text",
    description: "Choose response formatting through format or Accept without changing the provider route.",
    audience: "Developers integrating proxy responses into scripts, services, and data pipelines.",
    problem: "Different callers need different output shapes. A shell script may want text while an application wants JSON with request and usage metadata.",
    solution: "LLM Proxy supports plain text by default and can return JSON, XML, or CSV through the format query parameter or Accept header.",
    steps: [
      "Send a normal authenticated text request.",
      "Omit format for text/plain or request application/json, application/xml, or text/csv.",
      "Read token usage headers when upstream usage is available.",
      "Use JSON when the caller needs request, response, choices, messages, and usage fields together.",
    ],
    features: [
      ["Format negotiation", "Callers choose output shape without changing provider selection.", "The same route can serve scripts and apps."],
      ["JSON metadata", "JSON responses include request, response, object, model, choices, messages, and usage when available.", "Apps get structured context."],
      ["Usage headers", "Token headers remain separate from body format.", "Plain text callers can still read usage counts."],
    ],
    examples: [
      ["Shell output", "A CLI caller accepts text/plain for direct terminal display."],
      ["Structured app", "A backend requests application/json to store response and token metadata."],
      ["CSV export", "A workflow asks for text/csv when it wants one escaped cell."],
    ],
    limitations: [
      "Response formatting does not change provider behavior.",
      "Usage appears only when the upstream provider reports usage.",
      "XML and CSV are formatting conveniences, not provider-native response shapes.",
    ],
  }),
  page({
    slug: "llm-proxy-status-code-map",
    category: "Reliability",
    primaryKeyword: "LLM proxy status code map",
    title: "LLM Proxy status code map for callers",
    description: "Handle missing keys, bad inputs, rate limits, disabled providers, timeouts, and upstream failures consistently.",
    audience: "Developers who need predictable error handling around LLM and dictation calls.",
    problem: "Provider errors can be inconsistent. Callers need to know whether a request failed because of authentication, validation, capacity, provider rate limits, or upstream failure.",
    solution: "LLM Proxy maps public errors to documented HTTP status codes across text and dictation routes.",
    steps: [
      "Treat 400 as invalid request parameters or unsupported provider/model/capability.",
      "Treat 403 as missing or invalid tenant secret.",
      "Treat 413 as prompt or audio payload too large.",
      "Handle 429, 503, 504, and 502 according to rate-limit, disabled-provider, timeout, and upstream-failure meanings.",
    ],
    features: [
      ["Client validation errors", "Bad provider/model choices and ambiguous bodies return 400.", "Callers can fix request shape."],
      ["Authentication boundary", "Missing or invalid key returns 403.", "The proxy does not reveal tenant internals."],
      ["Provider/capacity signals", "429, 503, 504, and 502 communicate distinct runtime conditions.", "Retry behavior can be more precise."],
    ],
    examples: [
      ["Disabled provider", "A selected non-default provider without an API key returns 503 provider not configured."],
      ["Long provider work", "A request exceeding server.request_timeout_seconds returns 504."],
      ["Provider outage", "A non-rate-limit upstream provider failure maps to bad gateway behavior."],
    ],
    limitations: [
      "The status map does not guarantee provider recovery.",
      "Callers should still log request context without secrets.",
      "Provider-specific error details may be normalized or wrapped by the proxy.",
    ],
  }),
  page({
    slug: "dictation-provider-routing",
    category: "Dictation",
    primaryKeyword: "dictation provider routing",
    title: "Dictation provider routing for OpenAI, Zhipu, Grok, and SiliconFlow",
    description: "Select dictation-capable providers through /dictate while keeping transcription URLs server-side.",
    audience: "Teams testing transcription providers behind one proxy endpoint.",
    problem: "Speech providers use different URLs, models, and multipart details. Client apps should not carry those differences.",
    solution: "LLM Proxy exposes dictation-capable providers through configured catalogs and provider-specific transcription URLs while preserving one /dictate endpoint.",
    steps: [
      "Configure dictation catalogs for OpenAI, SiliconFlow, Zhipu, or Grok/xAI as needed.",
      "Send multipart audio to /dictate with key=<tenant secret>.",
      "Omit provider/model for tenant defaults or select a dictation-capable provider and model.",
      "Receive JSON text output from the proxy.",
    ],
    features: [
      ["Dictation catalogs", "Supported providers declare default dictation models and model lists.", "Unknown models fail at the request edge."],
      ["Transcription URL config", "Each dictation-capable provider owns an explicit transcriptions_url.", "Client apps do not hardcode provider STT URLs."],
      ["Provider-specific multipart handling", "The backend handles details such as whether a model field is sent.", "Grok/xAI STT omits the multipart model field."],
    ],
    examples: [
      ["OpenAI default dictation", "A caller omits provider and model to use tenant dictation defaults."],
      ["Zhipu transcription", "A caller chooses provider=zhipu when that tenant has GLM-ASR configured."],
      ["Grok/xAI STT", "The proxy routes provider=grok to the configured xAI STT endpoint."],
    ],
    limitations: [
      "Text-only providers do not support /dictate through the proxy.",
      "Upstream products may expose other speech APIs that are not wired here.",
      "Audio payload size is still capped by max_input_audio_bytes.",
    ],
  }),
  page({
    slug: "openai-compatible-provider-gateway",
    category: "Provider routing",
    primaryKeyword: "OpenAI-compatible provider gateway",
    title: "OpenAI-compatible provider gateway",
    description: "Route Meta Muse Spark, DeepSeek, DashScope Qwen, Qwen Cloud, Kimi, MiniMax, SiliconFlow, Zhipu, and Grok text calls through one compatible adapter.",
    audience: "Teams adopting OpenAI-compatible chat providers without rewriting every caller.",
    problem: "OpenAI-compatible providers share a broad shape but still need different base URLs, keys, defaults, and availability rules.",
    solution: "LLM Proxy uses a shared compatible chat adapter for configured providers while keeping provider URLs, keys, and model catalogs in config.",
    steps: [
      "Configure provider base_url, api_key, and text catalog.",
      "Use provider selectors such as meta, deepseek, dashscope, qwencloud, moonshot, minimax, siliconflow, zhipu, or grok.",
      "Send GET, compatibility POST, or canonical /v2 requests.",
      "Let omitted model use the selected provider's configured default.",
    ],
    features: [
      ["Shared adapter", "Compatible chat providers use one proxy integration pattern.", "Provider-specific logic stays centralized."],
      ["Provider aliases", "Some providers expose aliases such as qwen, kimi, glm, or xai.", "Callers can use documented selectors."],
      ["Disabled-provider behavior", "Blank non-default provider keys keep startup working and return 503 when selected.", "Operators can stage provider support before credentials exist."],
    ],
    examples: [
      ["Meta Muse route", "A caller sends provider=meta and model=muse-spark-1.1 through Chat Completions."],
      ["Qwen alias", "A caller uses provider=qwen for DashScope routing."],
      ["Qwen Cloud route", "A caller uses provider=qwencloud with a Qwen Cloud Token Plan key; it is not the DashScope qwen alias."],
      ["MiniMax route", "A caller uses provider=minimax and model=MiniMax-M2.7 through the shared Chat Completions adapter."],
      ["xAI route", "A Grok text request uses the OpenAI-compatible chat adapter behind provider=grok."],
    ],
    limitations: [
      "Meta support is text-only; the proxy does not expose Meta dictation, web search, tools, multimodal inputs, or a Responses fallback.",
      "Each provider still needs a configured key before serving tenant traffic.",
      "Provider base URLs should stay explicit in config.",
    ],
  }),
  page({
    slug: "gemini-generatecontent-proxy",
    category: "Provider routing",
    primaryKeyword: "Gemini generateContent proxy",
    title: "Gemini generateContent proxy for shared LLM calls",
    description: "Map shared proxy messages into Gemini native generateContent requests with configured model limits.",
    audience: "Developers adding Gemini as a provider without bringing Gemini-specific payloads into every app.",
    problem: "Gemini native generateContent uses a different route and content structure from OpenAI-compatible chat providers.",
    solution: "LLM Proxy implements a Gemini adapter that receives the shared proxy request, routes to the configured Gemini base URL, and normalizes usage metadata where available.",
    steps: [
      "Configure providers.gemini.api_key, base_url, and text model catalog.",
      "Select provider=gemini or set Gemini as a tenant default.",
      "Send canonical messages through /v2 or compatibility text requests.",
      "Keep max_tokens within configured Gemini output limits.",
    ],
    features: [
      ["Native adapter", "The backend maps user and assistant messages into Gemini contents.", "System instructions use Gemini's systemInstruction shape."],
      ["Configured defaults", "Omitted model uses the Gemini text default_model.", "The default currently comes from config, not client code."],
      ["Output limit validation", "Gemini max_tokens values above configured limits return 400 before upstream calls.", "Known constraints are enforced at the edge."],
    ],
    examples: [
      ["Gemini summary", "A service sends provider=gemini and omits model to use the configured default."],
      ["Long transcript", "A /v2 messages request routes through Gemini without a Gemini SDK."],
      ["Limit check", "A caller sets max_tokens inside the configured Gemini limit."],
    ],
    limitations: [
      "Gemini dictation is not exposed through /dictate in this repo.",
      "Web search is not marked supported for Gemini in the current proxy catalog.",
      "The Gemini model list must be maintained in config.yml.",
    ],
  }),
  page({
    slug: "anthropic-claude-messages-proxy",
    category: "Provider routing",
    primaryKeyword: "Anthropic Claude Messages proxy",
    title: "Anthropic Claude Messages proxy for /v2 callers",
    description: "Route shared messages to Anthropic's native Messages API without changing client contracts.",
    audience: "Teams adding Claude behind the same tenant-secret proxy used for other providers.",
    problem: "Anthropic Messages has native system and max_tokens requirements that differ from OpenAI-compatible chat routes.",
    solution: "LLM Proxy maps shared messages into Anthropic's native Messages API, translates system messages to the top-level system field, and sends configured output limits when needed.",
    steps: [
      "Configure providers.anthropic.api_key, base_url, and Claude model catalog.",
      "Select provider=anthropic or use the claude alias.",
      "Send messages through /v2 with user and optional system messages.",
      "Let omitted max_tokens use the selected Claude model's configured output limit.",
    ],
    features: [
      ["Native Claude routing", "The proxy uses Anthropic Messages rather than treating Claude as a generic compatible chat endpoint.", "Provider semantics stay server-side."],
      ["System translation", "Shared system messages become Anthropic's top-level system field.", "Clients keep the v2 messages contract."],
      ["Output limits", "Claude model output_token_limit values are required and validated.", "Invalid max_tokens values fail before upstream calls."],
    ],
    examples: [
      ["Writing assistant", "A tool routes a /v2 writing request to provider=anthropic."],
      ["Default Claude route", "A tenant sets Anthropic as the default provider so callers can omit provider."],
      ["Max-token guard", "A caller's requested max_tokens above the configured Claude limit returns 400."],
    ],
    limitations: [
      "Anthropic dictation is not supported through /dictate.",
      "Web search is not marked supported for Anthropic in the current proxy catalog.",
      "The model catalog must include output token limits for Claude models.",
    ],
  }),
  page({
    slug: "per-request-provider-model-selection",
    category: "API contract",
    primaryKeyword: "per-request provider model selection",
    title: "Per-request provider and model selection",
    description: "Use provider and model parameters only when a request should override tenant or provider defaults.",
    audience: "Application developers who want simple defaults plus controlled overrides for special tasks.",
    problem: "A single application may need mostly default routing but occasional provider or model changes for a specific workflow.",
    solution: "LLM Proxy accepts provider and model on public endpoints while preserving omitted-provider and omitted-model default behavior.",
    steps: [
      "Omit provider and model for the authenticated tenant default route.",
      "Set provider when a request needs a different provider.",
      "Set model only when a request needs a specific configured model.",
      "Let query/body model conflict checks protect ambiguous JSON POST requests.",
    ],
    features: [
      ["Provider query parameter", "GET, POST, /v2, and /dictate accept provider where supported.", "A caller can override route per request."],
      ["Model request field", "Model can be set in query or JSON body according to endpoint rules.", "Unknown models return 400."],
      ["Default resolution", "Omitted values use tenant defaults or selected-provider defaults.", "Simple clients stay simple."],
    ],
    examples: [
      ["Default call", "An app sends only prompt and key to use tenant defaults."],
      ["Gemini experiment", "One endpoint call includes provider=gemini and omits model."],
      ["Specific OpenAI model", "A request includes model=gpt-5 when the task needs that configured model."],
    ],
    limitations: [
      "Model identifiers are provider-scoped.",
      "POST /v2 rejects prompt and body system_prompt even when provider/model are valid.",
      "Unsupported provider capabilities fail before provider calls.",
    ],
  }),
  page({
    slug: "system-prompt-handling",
    category: "API contract",
    primaryKeyword: "LLM system prompt handling",
    title: "System prompt handling without ambiguous inputs",
    description: "Keep system instructions explicit across prompt bodies, v2 messages, tenant defaults, and provider settings.",
    audience: "Developers who need predictable system-instruction behavior across providers.",
    problem: "System instructions can collide when a request sends both a body system_prompt and a system role message, or when tenant defaults also exist.",
    solution: "LLM Proxy validates ambiguous system prompt inputs and applies tenant or provider system prompts only when request-level instructions are absent.",
    steps: [
      "Use body system_prompt only on compatibility POST / prompt-style requests.",
      "Use system role messages on POST /v2.",
      "Avoid sending both body system_prompt and a system message.",
      "Configure tenant defaults or provider-specific system prompts for omitted request instructions.",
    ],
    features: [
      ["Ambiguity rejection", "Bodies that combine system_prompt with system messages return 400.", "The proxy avoids guessing instruction precedence."],
      ["Tenant default prompt", "A tenant default system prompt can be prepended when submitted messages lack a system message.", "Default behavior is server-owned."],
      ["Provider-specific prompt", "Managed provider settings can store a system prompt for provider-selected requests.", "Provider context travels with the saved provider."],
    ],
    examples: [
      ["Canonical /v2", "A chat app sends a system role message and no body system_prompt."],
      ["Managed provider route", "A request selects provider=deepseek and omits system instructions, so the saved provider prompt can apply."],
      ["Conflict cleanup", "A migration removes duplicate system_prompt from bodies that already contain system messages."],
    ],
    limitations: [
      "Server-injected system prompts are not echoed in response metadata.",
      "POST /v2 rejects body system_prompt by design.",
      "Provider-specific prompt behavior applies to managed provider-selected requests when request-level instructions are omitted.",
    ],
  }),
  page({
    slug: "max-tokens-provider-limit-validation",
    category: "API contract",
    primaryKeyword: "max tokens provider limit validation",
    title: "max_tokens validation across LLM providers",
    description: "Map max_tokens to provider-specific fields and reject known invalid token caps before upstream calls.",
    audience: "Developers and operators who need predictable output caps without provider-specific client code.",
    problem: "Output-token fields differ across OpenAI, compatible chat providers, Anthropic, and Gemini, and some providers have known ceilings.",
    solution: "LLM Proxy accepts max_tokens and maps it to the selected provider's expected field while validating known Gemini and Claude limits at the request edge.",
    steps: [
      "Send max_tokens as a positive integer on supported text endpoints.",
      "Let the proxy map it to max_output_tokens, max_tokens, or generationConfig.maxOutputTokens.",
      "Keep Gemini and Claude requests within configured output limits.",
      "Omit max_tokens to use provider defaults, except Anthropic where the proxy supplies configured limits.",
    ],
    features: [
      ["Shared request field", "Callers use one max_tokens input across providers.", "Provider payload mapping stays backend-owned."],
      ["Known ceiling checks", "Gemini and Claude limits are validated before upstream calls.", "Invalid caps return 400."],
      ["Anthropic requirement handling", "The proxy sends required Claude max_tokens from config when omitted.", "Clients do not need to know that Anthropic requires the field."],
    ],
    examples: [
      ["Short answer", "A caller sets max_tokens=512 for a concise response."],
      ["Gemini guard", "A Gemini request above 65536 is rejected before provider traffic."],
      ["Claude default", "A Claude request omits max_tokens and the proxy uses the configured model output limit."],
    ],
    limitations: [
      "A dash in documentation means the proxy only validates positive values and lets upstream enforce provider-side limits.",
      "Provider output behavior still depends on the selected upstream model.",
      "max_tokens affects one request, not tenant-level quota enforcement.",
    ],
  }),
  page({
    slug: "usage-metadata-without-prompts",
    category: "Usage",
    primaryKeyword: "usage metadata without prompts",
    title: "Usage metadata without storing prompts or responses",
    description: "Track managed-tenant usage signals while excluding prompt, transcript, audio, response, and secret content.",
    audience: "Teams that need AI usage visibility without making the usage store a sensitive content log.",
    problem: "Usage dashboards can accidentally become stores of prompts, transcripts, uploaded audio names, model responses, provider keys, or generated secrets.",
    solution: "LLM Proxy records operational metadata for managed-tenant usage while excluding raw content and secret material.",
    steps: [
      "Authenticate managed proxy requests with generated tenant secrets.",
      "Record endpoint, provider, model, status code, success flag, latency, and token counts.",
      "Exclude prompts, audio, transcripts, responses, tenant secrets, and provider keys from usage events.",
      "Expose aggregated summaries through user and admin dashboards.",
    ],
    features: [
      ["Metadata-only events", "Usage records describe routing and status, not request content.", "Dashboards can work without prompt storage."],
      ["Tenant isolation", "Signed-in users see their own usage summary.", "Admins get summaries without sensitive content."],
      ["Provider/model breakdown", "Aggregates show where traffic went.", "Operators can inspect patterns without reading prompts."],
    ],
    examples: [
      ["Privacy-conscious dashboard", "A user sees request counts and token totals but no prompt body."],
      ["Admin overview", "An admin sees tenant facts and usage summaries without generated secrets."],
      ["Error trend", "Status-code buckets reveal failures without storing responses."],
    ],
    limitations: [
      "Metadata can still be operationally sensitive and should be protected.",
      "The dashboard is not a complete audit log.",
      "Usage recording applies to managed tenants using generated-secret routing.",
    ],
  }),
  evidencedPage({
    slug: "copyable-llm-curl-examples",
    category: "Management UI",
    primaryKeyword: "copyable LLM curl examples",
    title: "Copyable LLM curl examples from current profile data",
    description: "Render copyable LLM curl examples from live profile data while keeping the one-time raw client key out of page markup and copied commands.",
    audience: "Users onboarding themselves to LLM Proxy through the management UI.",
    problem: "Docs and examples drift when they hardcode hosts, provider choices, or secret placeholders that do not match the signed-in user's state.",
    solution: "LLM Proxy Settings renders copyable examples from profile data while keeping raw client keys out of example markup and clipboard actions.",
    steps: [
      "Open Settings after signing in.",
      "Expand Usage / Request examples.",
      "Copy default text, default /v2, or default dictation examples.",
      "Select a provider to copy provider-specific text, /v2, and dictation examples when supported.",
    ],
    features: [
      ["Generated-secret placeholder", "Examples always use <generated-secret>, including after automatic client-key creation.", "Raw client keys never enter example markup."],
      ["Separate one-time key", "The newly created key appears masked in a read-only field with Show and Copy actions.", "Users substitute it for the placeholder outside the page."],
      ["Provider-specific variants", "Selected-provider examples include provider and model context.", "Dictation examples appear only for dictation-capable providers."],
    ],
    examples: [
      ["First run", "A new user copies the automatically created client key, saves a provider key, and then copies the default /v2 example."],
      ["After creation", "The example still contains <generated-secret> so the raw client key is not copied implicitly."],
      ["Provider switch", "Selecting DeepSeek updates provider-specific examples and hides dictation when unsupported."],
    ],
    limitations: [
      "Examples are for proxy calls, not direct provider API calls.",
      "The generated secret is shown once; copy it when created.",
      "Examples depend on the API-served profile and config payloads.",
    ],
    repoExample: {
      source: "site/assets/llm-proxy/js/ui/keyManagement.js",
      verifiedOn: "2026-07-22",
      code: `get exampleSecret() {
  return EMPTY_SECRET_PLACEHOLDER;
}`,
    },
    quickVerdict: "Copy the request shape with its placeholder, then supply the one-time client key separately so raw access material never enters example markup.",
    faq: [
      {
        question: "What do copied curl examples use for the client key?",
        answer: "Every example uses the <generated-secret> placeholder. The user supplies the separately copied one-time client key when preparing the real request.",
      },
      {
        question: "Where do example hosts and provider choices come from?",
        answer: "The Settings UI builds examples from the API-served runtime configuration and the current authenticated management profile instead of hardcoding deployment-specific values.",
      },
      {
        question: "Can copying an example expose the raw generated key?",
        answer: "No. The generated value stays in its dedicated read-only field, while example markup and clipboard actions use only the placeholder.",
      },
      {
        question: "When does a provider-specific dictation example appear?",
        answer: "It appears only when the selected provider supports dictation; text and /v2 examples remain available for configured text providers.",
      },
    ],
  }),
  page({
    slug: "provider-specific-system-prompts",
    category: "Management UI",
    primaryKeyword: "provider-specific system prompts",
    title: "Provider-specific system prompts in LLM Proxy Settings",
    description: "Store each provider's text model and system prompt with its managed provider configuration.",
    audience: "Users who need different instructions for different upstream providers.",
    problem: "A single global system prompt can be too blunt when different providers are used for different jobs or when provider-selected requests need their own context.",
    solution: "LLM Proxy stores text model and provider-specific system prompt settings with each managed provider profile.",
    steps: [
      "Open the selected-provider editor in Settings.",
      "Choose the provider to configure.",
      "Set the provider text model and provider system prompt.",
      "Leave the changed field or switch providers; the selected provider settings autosave without a separate save action.",
    ],
    features: [
      ["Provider-owned settings", "Model and prompt live with the selected provider record.", "Provider-specific routing context is explicit."],
      ["Runtime injection", "Managed provider-selected requests can use the saved model and prompt when request fields are omitted.", "Callers can stay concise."],
      ["Clear removal", "Removing a provider key also removes provider settings.", "The destructive boundary is explicit."],
    ],
    examples: [
      ["Concise OpenAI answers", "A user stores an OpenAI system prompt that asks for concise responses."],
      ["DeepSeek default model", "A user saves a DeepSeek model for provider-selected requests."],
      ["Provider cleanup", "A user removes a provider key and settings after a trial ends."],
    ],
    limitations: [
      "Request-level system instructions take precedence over server-injected defaults.",
      "Provider settings are scoped to the authenticated managed tenant.",
      "Autosave responses return masked key status; raw retrieval requires the separate owner-authenticated reveal action.",
    ],
  }),
  page({
    slug: "local-and-hosted-llm-proxy-config-profiles",
    category: "Deployment",
    primaryKeyword: "local and hosted LLM proxy config",
    title: "Local and hosted LLM Proxy config profiles",
    description: "Use the same strict config contract locally and in hosted split-origin deployments.",
    audience: "Operators who need local development and production profiles to stay aligned.",
    problem: "Local profiles often accumulate defaults, fallback values, or alternate config paths that do not match hosted runtime behavior.",
    solution: "LLM Proxy uses config.yml plus strict placeholder expansion for both local and hosted profiles, with ignored .env values supplying environment-specific secrets and database paths.",
    steps: [
      "Keep config.yml as the canonical service configuration.",
      "Use configs/.env locally for ignored placeholder values.",
      "Use deployment secrets for hosted LLM_PROXY_MANAGEMENT_* values.",
      "Run repo Makefile targets to validate config and tests.",
    ],
    features: [
      ["Same loader", "Local and hosted profiles pass through the same config loader.", "There is no separate Pages config expansion path."],
      ["Ignored local DB", "SQLite management files are ignored for local runs.", "State does not enter git."],
      ["Hosted split origins", "Production separates Pages frontend, API backend, and TAuth backend origins.", "CORS and runtime config stay explicit."],
    ],
    examples: [
      ["Local UI preview", "A developer serves site/ and mocks /config-ui.yaml for browser tests."],
      ["Hosted backend", "Production provides real management database and encryption key through deployment secrets."],
      ["Provider smoke", "Live provider smoke tests load available API keys from LIVE_ENV_FILE."],
    ],
    limitations: [
      "Do not commit configs/.env, SQLite databases, or OAuth client secrets.",
      "Hosted deployment still requires gateway and DNS setup.",
      "Local validation does not replace live provider smoke tests when real provider behavior matters.",
    ],
  }),
  page({
    slug: "github-pages-llm-management-ui",
    category: "Deployment",
    primaryKeyword: "GitHub Pages LLM management UI",
    title: "GitHub Pages management UI for LLM Proxy",
    description: "Publish the static management frontend from site/ while the backend owns API and runtime config.",
    audience: "Teams deploying the self-service management UI as a static site.",
    problem: "Serving the management frontend from the API backend couples static hosting, runtime config, and proxy endpoints in one deployment surface.",
    solution: "LLM Proxy keeps the frontend in site/ for GitHub Pages and keeps the Go backend responsible for /config-ui.yaml, /api/management/*, /, /v2, and /dictate.",
    steps: [
      "Run make release to render and package site/ as an immutable Pages release asset.",
      "Run make publish to upload that prepared asset without changing the live site.",
      "Run make deploy to activate the published asset on gh-pages.",
      "Configure GitHub Pages branch publishing and the custom domain.",
      "Point llm-proxy-api at the backend gateway route.",
      "Keep browser runtime config served only by the backend API origin.",
    ],
    features: [
      ["Pages-owned static UI", "llm-proxy.mprlab.com belongs to GitHub Pages.", "The backend does not serve management HTML."],
      ["Makefile lifecycle", "Release prepares, publish uploads, and deploy activates Pages.", "GitHub Actions does not own production deployment."],
      ["Artifact checks", "The rendered Pages artifact is checked for forbidden static config files.", "The static shell stays config-free."],
    ],
    examples: [
      ["Pages preparation", "make release renders the validated Pages archive locally."],
      ["Artifact publication", "make publish uploads the exact prepared image and Pages artifacts."],
      ["Backend deploy", "make deploy rolls out the backend and then activates the published Pages archive."],
    ],
    limitations: [
      "Production deploy still requires operator-controlled gateway work.",
      "GitHub Pages custom domain and HTTPS settings must be configured.",
      "The static UI needs the backend API host available at runtime.",
    ],
  }),
  page({
    slug: "live-provider-smoke-tests",
    category: "Validation",
    primaryKeyword: "live provider smoke tests",
    title: "Live provider smoke tests for LLM Proxy",
    description: "Run paid-provider smoke checks separately from CI when real upstream behavior needs verification.",
    audience: "Operators validating provider credentials and hosted provider routes after config or deployment changes.",
    problem: "CI should be deterministic and avoid paid provider calls, but some changes still need live confirmation against real upstream providers.",
    solution: "LLM Proxy keeps live provider smoke tests outside make ci, parses provider keys from a dotenv file without executing shell code, and forces the temporary proxy into management-disabled mode.",
    steps: [
      "Put provider API keys in an ignored env file.",
      "Run make test-live-providers LIVE_ENV_FILE=configs/.env.",
      "Limit the provider set with LLM_PROXY_LIVE_PROVIDERS when needed.",
      "Use --write-config to inspect the isolated temporary contract without making a provider call.",
      "Use model overrides only when debugging a specific provider/model pair.",
    ],
    features: [
      ["Separate from CI", "Live provider smoke tests are not part of make ci.", "Routine validation stays deterministic."],
      ["Provider discovery and isolation", "The dynamic target runs providers whose keys are available through a management-disabled temporary config.", "Missing optional keys and hosted management state do not affect unrelated smoke checks."],
      ["Default model path", "By default, smoke requests omit model so provider configured defaults are tested.", "The test exercises the default-selection contract."],
    ],
    examples: [
      ["OpenAI smoke", "Run LLM_PROXY_LIVE_PROVIDERS=openai with a live env file."],
      ["Gemini wrapper", "Use make test-live-gemini as the Gemini-specific compatibility target."],
      ["Post-deploy check", "After publishing, run selected live smokes for providers touched by config changes."],
    ],
    limitations: [
      "Live smoke tests can call paid APIs.",
      "They depend on secret availability and provider uptime.",
      "They are complementary to, not replacements for, black-box CI tests.",
    ],
  }),
  page({
    slug: "internal-ai-gateway-for-product-tools",
    category: "Use cases",
    primaryKeyword: "internal AI gateway",
    title: "Internal AI gateway for product tools",
    description: "Centralize provider routing, tenant secrets, request validation, and usage signals for internal AI tools.",
    audience: "Product and platform teams with several internal tools calling LLMs or dictation providers.",
    problem: "Internal tools can quietly accumulate provider SDKs, raw keys, inconsistent defaults, and untracked usage as teams add AI features one by one.",
    solution: "LLM Proxy gives internal tools one tenant-secret HTTP boundary while centralizing provider credentials, routing, request validation, and managed usage dashboards.",
    steps: [
      "Run llm-proxy as the shared service boundary.",
      "Configure tenant defaults and provider catalogs centrally.",
      "Give each internal app a tenant secret or generated managed secret.",
      "Review usage metadata in the dashboard without storing prompts or responses.",
    ],
    features: [
      ["Central routing", "Provider and model selection happen at the proxy boundary.", "Apps do not need provider-specific SDKs."],
      ["Credential separation", "Client apps receive tenant secrets, not upstream provider keys.", "Provider keys stay server-side."],
      ["Usage visibility", "Managed tenant dashboards summarize requests, tokens, provider, model, and status.", "Teams can see operational patterns."],
    ],
    examples: [
      ["Support console", "A support tool routes summaries through the shared proxy with a tenant secret."],
      ["Admin automation", "An internal script uses the CLI client for repeated analysis tasks."],
      ["Voice input", "A tool adds /dictate without creating a second credential path."],
    ],
    limitations: [
      "The proxy is not a full policy engine for every AI governance need.",
      "Provider spend and availability still depend on upstream accounts.",
      "Network controls are recommended before public exposure.",
    ],
  }),
  page({
    slug: "provider-overload-timeout-handling",
    category: "Reliability",
    primaryKeyword: "LLM provider overload timeout handling",
    title: "Provider overload and timeout handling for LLM calls",
    description: "Use clear 503 and 504 behavior for queue pressure and provider work that exceeds the proxy deadline.",
    audience: "Developers building retry and alerting behavior around LLM Proxy.",
    problem: "Failures are harder to handle when overload, provider timeout, missing credentials, and upstream errors collapse into one generic exception.",
    solution: "LLM Proxy separates request queue pressure, disabled provider credentials, upstream provider rate limits, gateway timeout, and provider failures through documented status codes.",
    steps: [
      "Use 503 request queue full or provider not configured as service-availability signals.",
      "Use 504 as the overall proxy request deadline expiring.",
      "Use server.upstream_rate_limits to pace calls at actual upstream admission for each configured origin.",
      "Use 429 for upstream provider rate limits.",
      "Use 502 for other upstream provider API failures.",
    ],
    features: [
      ["Queue status", "The shared upstream operation queue returns service-unavailable behavior when full.", "Callers can back off before adding more pressure."],
      ["Deadline status", "Long requests that exceed server.request_timeout_seconds return 504.", "Clients should not poll llm-proxy after a timeout."],
      ["Provider error mapping", "Rate limits and provider failures map to distinct statuses.", "Retry logic can be status-aware."],
    ],
    examples: [
      ["Capacity alert", "A spike in queue-full responses points to proxy capacity tuning."],
      ["Long prompt timeout", "A semantic-review request returns 504 when provider work exceeds the configured deadline."],
      ["Disabled provider", "A selected provider without an API key returns 503 provider not configured."],
    ],
    limitations: [
      "Status codes do not guarantee safe automatic retries for every prompt.",
      "Provider-side behavior may still vary by upstream service.",
      "Waiting for a server.upstream_rate_limits slot remains bounded by the request deadline and can therefore end as 504.",
    ],
  }),
]);

if (pages.length < MIN_PAGE_COUNT || pages.length > MAX_PAGE_COUNT) {
  throw new Error(`seo_resource_page_count_out_of_range: count=${pages.length}`);
}

const uniqueSlugs = new Set(pages.map((resourcePage) => resourcePage.slug));
if (uniqueSlugs.size !== pages.length) {
  throw new Error("seo_resource_duplicate_slug");
}

await rm(RESOURCE_ROOT, { force: true, recursive: true });
await mkdir(RESOURCE_ROOT, { recursive: true });

for (const resourcePage of pages) {
  const pageDirectory = join(RESOURCE_ROOT, resourcePage.slug);
  await mkdir(pageDirectory, { recursive: true });
  await writeFile(join(pageDirectory, "index.html"), renderResourcePage(resourcePage), "utf8");
}

await writeFile(join(RESOURCE_ROOT, "index.html"), renderHub(), "utf8");
await writeFile("site/sitemap.xml", renderSitemap(), "utf8");
await writeFile("site/robots.txt", renderRobots(), "utf8");
await writeFile(REPORT_PATH, renderReport(), "utf8");

console.log(`generated ${pages.length} SEO resource pages`);

/**
 * @param {{
 *   slug: string,
 *   modifiedDate?: string,
 *   category: string,
 *   primaryKeyword: string,
 *   title: string,
 *   description: string,
 *   audience: string,
 *   problem: string,
 *   solution: string,
 *   steps: string[],
 *   features: [string, string, string][],
 *   examples: [string, string][],
 *   limitations: string[],
 *   repoExample?: { source: string, code: string, verifiedOn: string },
 *   quickVerdict?: string,
 *   faq?: { question: string, answer: string }[],
 * }} input
 */
function page(input) {
  return Object.freeze({
    ...input,
    modifiedDate: input.modifiedDate || RESOURCE_DEFAULT_MODIFIED_DATE,
    path: `/resources/${input.slug}/`,
    canonical: `${PUBLIC_ORIGIN}/resources/${input.slug}/`,
  });
}

/**
 * Generates a resource whose current claims have direct, dated repository evidence.
 *
 * @param {Parameters<typeof page>[0] & {
 *   repoExample: { source: string, code: string, verifiedOn: string },
 *   quickVerdict: string,
 *   faq: { question: string, answer: string }[],
 * }} input
 */
function evidencedPage(input) {
  if (input.title.length < 50 || input.title.length > 60) {
    throw new Error(`seo_resource_title_length_out_of_range: slug=${input.slug} length=${input.title.length}`);
  }
  if (!input.quickVerdict.trim()) {
    throw new Error(`seo_resource_quick_verdict_missing: slug=${input.slug}`);
  }
  if (!input.repoExample.source.trim() || !input.repoExample.code.trim()) {
    throw new Error(`seo_resource_repository_evidence_missing: slug=${input.slug}`);
  }
  if (!/^\d{4}-\d{2}-\d{2}$/.test(input.repoExample.verifiedOn)) {
    throw new Error(`seo_resource_evidence_date_invalid: slug=${input.slug}`);
  }
  if (input.faq.length < 4 || input.faq.some((item) => !item.question.trim() || !item.answer.trim())) {
    throw new Error(`seo_resource_faq_count_too_small: slug=${input.slug}`);
  }
  return page({
    ...input,
    modifiedDate: input.repoExample.verifiedOn,
  });
}

/**
 * @param {ReturnType<typeof page>} resourcePage
 * @returns {string}
 */
function renderResourcePage(resourcePage) {
  const relatedPages = relatedFor(resourcePage);
  const faq = faqFor(resourcePage, relatedPages[0]);
  const articleJsonLd = {
    "@context": "https://schema.org",
    "@type": "Article",
    headline: resourcePage.title,
    description: resourcePage.description,
    url: resourcePage.canonical,
    datePublished: RESOURCE_PUBLISHED_DATE,
    dateModified: resourcePage.modifiedDate,
    mainEntityOfPage: resourcePage.canonical,
    about: [PRODUCT_NAME, resourcePage.category, resourcePage.primaryKeyword],
  };
  const breadcrumbJsonLd = {
    "@context": "https://schema.org",
    "@type": "BreadcrumbList",
    itemListElement: [
      { "@type": "ListItem", position: 1, name: "Home", item: `${PUBLIC_ORIGIN}/` },
      { "@type": "ListItem", position: 2, name: "Resources", item: `${PUBLIC_ORIGIN}/resources/` },
      { "@type": "ListItem", position: 3, name: resourcePage.title, item: resourcePage.canonical },
    ],
  };
  const faqJsonLd = {
    "@context": "https://schema.org",
    "@type": "FAQPage",
    mainEntity: faq.map((item) => ({
      "@type": "Question",
      name: item.question,
      acceptedAnswer: {
        "@type": "Answer",
        text: item.answer,
      },
    })),
  };

  return htmlDocument({
    title: resourcePage.title,
    description: resourcePage.description,
    canonical: resourcePage.canonical,
    bodyClass: "resource-page",
    jsonLd: [articleJsonLd, breadcrumbJsonLd, faqJsonLd],
    body: `
      <header class="resource-shell resource-topbar">
        <a class="resource-brand" href="/">LLM Proxy</a>
        <nav aria-label="Resource navigation">
          <a href="/resources/">Resources</a>
          <a href="/">Main page</a>
        </nav>
      </header>
      <main class="resource-shell resource-article">
        <nav class="breadcrumbs" aria-label="Breadcrumb">
          <a href="/">Home</a>
          <span>/</span>
          <a href="/resources/">Resources</a>
          <span>/</span>
          <span>${escapeHTML(resourcePage.title)}</span>
        </nav>
        <article>
          <header class="resource-hero">
            <p class="eyebrow">${escapeHTML(resourcePage.category)}</p>
            <h1>${escapeHTML(resourcePage.title)}</h1>
            <p class="resource-deck">${escapeHTML(resourcePage.description)}</p>
            <p class="resource-audience">${escapeHTML(resourcePage.audience)}</p>${resourcePage.quickVerdict ? `
            <aside class="resource-verdict" aria-label="Quick verdict">
              <strong>Quick verdict</strong>
              <p>${escapeHTML(resourcePage.quickVerdict)}</p>
            </aside>` : ""}
            <div class="resource-actions">
              <a class="resource-button" href="/">Open LLM Proxy</a>
              <a class="resource-link" href="/resources/">Browse all resources</a>
            </div>
          </header>

          <section>
            <h2>The problem</h2>
            <p>${escapeHTML(resourcePage.problem)}</p>
          </section>

          <section>
            <h2>How LLM Proxy helps</h2>
            <p>${escapeHTML(resourcePage.solution)}</p>
          </section>

          <section>
            <h2>How it works</h2>
            <ol class="resource-steps">
              ${resourcePage.steps.map((step) => `<li>${escapeHTML(step)}</li>`).join("\n")}
            </ol>
          </section>

          <section>
            <h2>Feature-to-benefit table</h2>
            <div class="resource-table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Feature</th>
                    <th>Why it matters here</th>
                    <th>Example</th>
                  </tr>
                </thead>
                <tbody>
                  ${resourcePage.features
                    .map(
                      ([feature, benefit, example]) => `
                        <tr>
                          <td>${escapeHTML(feature)}</td>
                          <td>${escapeHTML(benefit)}</td>
                          <td>${escapeHTML(example)}</td>
                        </tr>
                      `,
                    )
                    .join("\n")}
                </tbody>
              </table>
            </div>
          </section>

          <section>
            <h2>Use-case examples</h2>
            <div class="resource-card-grid">
              ${resourcePage.examples
                .map(
                  ([title, body]) => `
                    <section class="resource-card">
                      <h3>${escapeHTML(title)}</h3>
                      <p>${escapeHTML(body)}</p>
                    </section>
                  `,
                )
                .join("\n")}
            </div>
          </section>

          <section>
            <h2>Objections and limitations</h2>
            <ul class="resource-list">
              ${resourcePage.limitations.map((limitation) => `<li>${escapeHTML(limitation)}</li>`).join("\n")}
            </ul>
          </section>${resourcePage.repoExample ? `

          <section>
            <h2>Repository evidence</h2>
            <pre class="usage-snippet"><code>${escapeHTML(resourcePage.repoExample.code)}</code></pre>
            <p>Verified ${resourcePage.repoExample.verifiedOn} by <a href="https://github.com/tyemirov" rel="author">Tyemirov on GitHub</a> against <code>${escapeHTML(resourcePage.repoExample.source)}</code>.</p>
          </section>` : ""}

          <section>
            <h2>FAQ</h2>
            <div class="resource-faq">
              ${faq
                .map(
                  (item) => `
                    <details>
                      <summary>${escapeHTML(item.question)}</summary>
                      <p>${escapeHTML(item.answer)}</p>
                    </details>
                  `,
                )
                .join("\n")}
            </div>
          </section>

          <section>
            <h2>Related resources</h2>
            <div class="resource-related-grid">
              ${relatedPages
                .map(
                  (relatedPage) => `
                    <a class="resource-related" href="${relatedPage.path}">
                      <span>${escapeHTML(relatedPage.category)}</span>
                      <strong>${escapeHTML(relatedPage.title)}</strong>
                    </a>
                  `,
                )
                .join("\n")}
            </div>
          </section>

          <section class="resource-final-cta">
            <h2>Use this pattern in LLM Proxy</h2>
            <p>Start from the main management surface, then copy a current request example or use the documented REST contract for this workflow.</p>
            <a class="resource-button" href="/">Open the main page</a>
          </section>
        </article>
      </main>
      <footer class="resource-shell resource-footer">
        <a href="/">LLM Proxy main page</a>
        <a href="/resources/">Resource hub</a>
        <a href="/sitemap.xml">Sitemap</a>
      </footer>
    `,
  });
}

/**
 * @returns {string}
 */
function renderHub() {
  const categories = [...new Set(pages.map((resourcePage) => resourcePage.category))];
  const collectionJsonLd = {
    "@context": "https://schema.org",
    "@type": "CollectionPage",
    name: "LLM Proxy Resources",
    description: "Repo-grounded resources for using LLM Proxy as a multi-provider AI gateway.",
    url: `${PUBLIC_ORIGIN}/resources/`,
    mainEntity: pages.map((resourcePage) => ({
      "@type": "Article",
      headline: resourcePage.title,
      url: resourcePage.canonical,
    })),
  };
  const breadcrumbJsonLd = {
    "@context": "https://schema.org",
    "@type": "BreadcrumbList",
    itemListElement: [
      { "@type": "ListItem", position: 1, name: "Home", item: `${PUBLIC_ORIGIN}/` },
      { "@type": "ListItem", position: 2, name: "Resources", item: `${PUBLIC_ORIGIN}/resources/` },
    ],
  };

  return htmlDocument({
    title: "LLM Proxy Resources",
    description: "Use-case resources for routing LLM, dictation, provider keys, usage, and management workflows through LLM Proxy.",
    canonical: `${PUBLIC_ORIGIN}/resources/`,
    ogType: "website",
    bodyClass: "resource-page resource-hub-page",
    jsonLd: [collectionJsonLd, breadcrumbJsonLd],
    body: `
      <header class="resource-shell resource-topbar">
        <a class="resource-brand" href="/">LLM Proxy</a>
        <nav aria-label="Resource navigation">
          <a href="/resources/">Resources</a>
          <a href="/">Main page</a>
        </nav>
      </header>
      <main class="resource-shell resource-hub">
        <section class="resource-hero">
          <p class="eyebrow">Resources</p>
          <h1>LLM Proxy resource hub</h1>
          <p class="resource-deck">Use-case pages for teams routing text generation, dictation, provider credentials, runtime config, and usage visibility through LLM Proxy.</p>
          <div class="resource-actions">
            <a class="resource-button" href="/">Open LLM Proxy</a>
            <a class="resource-link" href="/sitemap.xml">View sitemap</a>
          </div>
        </section>
        ${categories
          .map((category) => {
            const categoryPages = pages.filter((resourcePage) => resourcePage.category === category);
            return `
              <section class="resource-category">
                <header class="section-header compact">
                  <div>
                    <p class="eyebrow">${escapeHTML(category)}</p>
                    <h2>${escapeHTML(category)} resources</h2>
                  </div>
                </header>
                <div class="resource-card-grid">
                  ${categoryPages
                    .map(
                      (resourcePage) => `
                        <a class="resource-index-card" href="${resourcePage.path}">
                          <span>${escapeHTML(resourcePage.primaryKeyword)}</span>
                          <strong>${escapeHTML(resourcePage.title)}</strong>
                          <p>${escapeHTML(resourcePage.description)}</p>
                        </a>
                      `,
                    )
                    .join("\n")}
                </div>
              </section>
            `;
          })
          .join("\n")}
      </main>
      <footer class="resource-shell resource-footer">
        <a href="/">LLM Proxy main page</a>
        <a href="/sitemap.xml">Sitemap</a>
      </footer>
    `,
  });
}

/**
 * @param {{
 *   title: string,
 *   description: string,
 *   canonical: string,
 *   ogType?: string,
 *   bodyClass: string,
 *   body: string,
 *   jsonLd: object[],
 * }} input
 * @returns {string}
 */
function htmlDocument(input) {
  return `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>${escapeHTML(input.title)}</title>
    <meta name="description" content="${escapeHTML(input.description)}">
    <link rel="canonical" href="${input.canonical}">
    <meta property="og:title" content="${escapeHTML(input.title)}">
    <meta property="og:description" content="${escapeHTML(input.description)}">
    <meta property="og:type" content="${escapeHTML(input.ogType || "article")}">
    <meta property="og:url" content="${input.canonical}">
    <meta name="twitter:card" content="summary">
    <meta name="theme-color" content="#0076c3">
    <link rel="icon" type="image/svg+xml" href="/assets/llm-proxy/img/favicon.svg">
    <link rel="apple-touch-icon" href="/assets/llm-proxy/img/llm-proxy-icon.svg">
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@v3.11.1/mpr-ui.css">
    <link rel="stylesheet" href="/assets/llm-proxy/styles.css">
    <link rel="stylesheet" href="/assets/llm-proxy/resources.css">
    ${input.jsonLd.map((schema) => `<script type="application/ld+json">${JSON.stringify(schema)}</script>`).join("\n    ")}
    <script defer src="https://loopaware.mprlab.com/pixel.js?site_id=839f018b-97a9-4955-a489-4ad5cb626f4f"></script>
  </head>
  <body class="${input.bodyClass}">
${input.body}
  </body>
</html>
`;
}

/**
 * @param {ReturnType<typeof page>} resourcePage
 * @returns {ReturnType<typeof page>[]}
 */
function relatedFor(resourcePage) {
  const sameCategory = pages.filter((candidate) => candidate.category === resourcePage.category && candidate.slug !== resourcePage.slug);
  const currentIndex = pages.findIndex((candidate) => candidate.slug === resourcePage.slug);
  const adjacent = [
    pages[(currentIndex + 1) % pages.length],
    pages[(currentIndex + pages.length - 1) % pages.length],
  ].filter((candidate) => candidate.slug !== resourcePage.slug);
  return uniquePages([...sameCategory.slice(0, 2), ...adjacent]).slice(0, 4);
}

/**
 * @param {ReturnType<typeof page>[]} resourcePages
 * @returns {ReturnType<typeof page>[]}
 */
function uniquePages(resourcePages) {
  const seen = new Set();
  return resourcePages.filter((resourcePage) => {
    if (seen.has(resourcePage.slug)) {
      return false;
    }
    seen.add(resourcePage.slug);
    return true;
  });
}

/**
 * @param {ReturnType<typeof page>} resourcePage
 * @param {ReturnType<typeof page>} relatedPage
 * @returns {{ question: string, answer: string }[]}
 */
function faqFor(resourcePage, relatedPage) {
  const pageSpecificFAQ = resourcePage.faq || [
    {
      question: `What is the main job of ${resourcePage.primaryKeyword}?`,
      answer: resourcePage.solution,
    },
    {
      question: `Who should read this ${resourcePage.category.toLowerCase()} resource?`,
      answer: resourcePage.audience,
    },
    {
      question: "Does this page claim provider performance or pricing advantages?",
      answer: "No. The supported claim is about LLM Proxy's documented routing, configuration, management, security, usage, and deployment contracts. Provider cost, speed, rankings, and benchmark claims are not made here.",
    },
    {
      question: "Where should setup details come from?",
      answer: `Use the main README and implementation notes for current command, config, and endpoint details. This page summarizes the workflow without replacing ${PRODUCT_NAME} documentation.`,
    },
  ];
  return [
    ...pageSpecificFAQ,
    {
      question: "What should I read next?",
      answer: `A closely related resource is ${relatedPage.title}, which covers ${relatedPage.primaryKeyword}.`,
    },
  ];
}

/**
 * @returns {string}
 */
function latestResourceModifiedDate() {
  return pages.reduce(
    (latestDate, resourcePage) => resourcePage.modifiedDate > latestDate ? resourcePage.modifiedDate : latestDate,
    RESOURCE_DEFAULT_MODIFIED_DATE,
  );
}

/**
 * @returns {string}
 */
function renderSitemap() {
  const currentResourceModifiedDate = latestResourceModifiedDate();
  const urls = [
    { loc: `${PUBLIC_ORIGIN}/`, lastmod: RESOURCE_DEFAULT_MODIFIED_DATE },
    { loc: `${PUBLIC_ORIGIN}/resources/`, lastmod: currentResourceModifiedDate },
    ...pages.map((resourcePage) => ({ loc: resourcePage.canonical, lastmod: resourcePage.modifiedDate })),
  ];
  return `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
${urls
  .map(
    (url) => `  <url>
    <loc>${url.loc}</loc>
    <lastmod>${url.lastmod}</lastmod>
  </url>`,
  )
  .join("\n")}
</urlset>
`;
}

/**
 * @returns {string}
 */
function renderRobots() {
  return `User-agent: *
Allow: /

Sitemap: ${PUBLIC_ORIGIN}/sitemap.xml
`;
}

/**
 * @returns {string}
 */
function renderReport() {
  const currentResourceModifiedDate = latestResourceModifiedDate();
  const pageRows = pages
    .map(
      (resourcePage, index) =>
        `| ${index + 1} | ${resourcePage.title} | ${resourcePage.audience} | ${resourcePage.problem} | ${resourcePage.primaryKeyword} | ${resourcePage.category} | /resources/${resourcePage.slug}/ | Low | Generate |`,
    )
    .join("\n");
  const categoryRows = [...new Set(pages.map((resourcePage) => resourcePage.category))]
    .map((category) => `| ${category} | ${pages.filter((resourcePage) => resourcePage.category === category).length} | Distinct workflow and examples within this category. |`)
    .join("\n");

  return `# LLM Proxy SEO Resource Cluster Report

Generated: ${currentResourceModifiedDate}

## Repo Analysis Report

### Source Files Reviewed

| File | Type | Why reviewed | Key findings | Confidence |
|---|---|---|---|---|
| ${evidence.readme} | Required product docs | Primary product description, REST contract, config, management UI, clients, deployment, and security wording | LLM Proxy is a lightweight HTTP proxy for text and dictation providers with tenant-secret auth, server-side credentials, optional TAuth management UI, usage dashboards, provider routing, and strict config loading. | High |
| ${evidence.providerRouting} | Implementation notes | Provider routing, config ownership, management mode, error contract, and adapter notes | Multi-provider routing, model catalogs, omitted-model behavior, split-origin management, usage storage, and provider-key security are implemented contracts. | High |
| ${evidence.dictation} | Implementation notes | Dictation endpoint contract | /dictate accepts multipart audio with key auth and returns JSON text; dictation routing is implemented for supported providers in the current README. | High |
| ${evidence.campaign} | Marketing copy | Existing audience and claim framing | Existing public claims emphasize server-side provider keys, multi-provider routing, dictation, usage dashboards, API-served config, and careful encrypted-at-rest wording. | Medium |

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
- It can run a TAuth-protected self-service management UI that automatically creates a missing client key, autosaves selected-provider settings, and requires one persisted provider key before Settings can close.
- Managed provider keys are encrypted at rest with AES-GCM; this protects storage/backups/dumps and is not a zero-knowledge guarantee.
- Browser runtime config is served by the backend /config-ui.yaml endpoint.

### Unsupported Claims Excluded

- Customer logos, testimonials, case studies, revenue impact, benchmark results, search volume, pricing savings, uptime guarantees, compliance certifications, or named competitor comparisons.

## Use-Case Opportunity List

| Priority | Page idea | Audience | Problem | Primary keyword candidate | Category | Public URL | Doorway risk | Recommendation |
|---:|---|---|---|---|---|---|---|---|
${pageRows}

## Category Mix

| Category | Page count | Doorway safety note |
|---|---:|---|
${categoryRows}

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
`;
}

/**
 * @param {string} value
 * @returns {string}
 */
function escapeHTML(value) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}
