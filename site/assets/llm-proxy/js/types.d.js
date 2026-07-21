// @ts-check

/**
 * @typedef {{
 *   provider: string,
 *   model: string,
 *   dictation_provider: string,
 *   dictation_model: string,
 *   system_prompt: string,
 *   reasoning_effort: string
 * }} TenantDefaults
 */

/**
 * @typedef {{
 *   adapter: string,
 *   efforts: string[]
 * }} ReasoningEffortCapability
 */

/**
 * @typedef {{
 *   id: string,
 *   reasoning_effort?: ReasoningEffortCapability
 * }} TextModelProfile
 */

/**
 * @typedef {{
 *   id: string,
 *   label: string,
 *   aliases: string[],
 *   has_key: boolean,
 *   masked_key?: string,
 *   text_model: string,
 *   system_prompt: string,
 *   text_default_model: string,
 *   text_models: TextModelProfile[],
 *   reasoning_effort?: ReasoningEffortCapability,
 *   supports_dictation: boolean,
 *   dictation_default_model?: string,
 *   dictation_models: string[]
 * }} ProviderProfile
 */

/**
 * @typedef {{
 *   api_key: string
 * }} ProviderKeyReveal
 */

/**
 * @typedef {{
 *   id: string,
 *   has_secret: boolean,
 *   defaults: TenantDefaults
 * }} TenantProfile
 */

/**
 * @typedef {{
 *   user: { id: string, email?: string, display_name?: string, avatar_url?: string, is_admin: boolean },
 *   tenant: TenantProfile,
 *   providers: ProviderProfile[],
 *   reasoning_effort_options: string[],
 *   proxy: { text_path: string, v2_path: string, dictation_path: string }
 * }} ManagementProfile
 */

/**
 * @typedef {{
 *   secret: string,
 *   profile: ManagementProfile
 * }} SecretResponse
 */

/**
 * @typedef {{
 *   configUrl: string,
 *   managementApiOrigin: string,
 *   proxyOrigin: string
 * }} FrontendRuntimeConfig
 */

/**
 * @typedef {{
 *   id: string,
 *   title: string,
 *   command: string
 * }} RequestExample
 */

/**
 * @typedef {{
 *   requests: number,
 *   successful_requests: number,
 *   failed_requests: number,
 *   text_requests: number,
 *   dictation_requests: number,
 *   request_tokens: number,
 *   response_tokens: number,
 *   total_tokens: number,
 *   average_latency_ms: number
 * }} UsageAggregate
 */

/**
 * @typedef {{
 *   period_days: number,
 *   totals: UsageAggregate,
 *   daily: Array<{ date: string, data: UsageAggregate }>,
 *   providers: Array<{ provider: string, data: UsageAggregate }>,
 *   models: Array<{ provider: string, model: string, data: UsageAggregate }>,
 *   status_codes: Array<{ status_code: number, requests: number }>
 * }} ManagementUsageSummary
 */

/**
 * @typedef {{
 *   user: { id: string, email?: string, display_name?: string, avatar_url?: string, is_admin: boolean },
 *   tenant: { id: string, has_secret: boolean, created_at?: string, updated_at?: string },
 *   usage: ManagementUsageSummary
 * }} ManagementAdminUser
 */

/**
 * @typedef {{
 *   period_days: number,
 *   users: ManagementAdminUser[]
 * }} ManagementAdminUsersResponse
 */

export {};
