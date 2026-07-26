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
 *   name: string,
 *   has_secret: boolean,
 *   defaults: TenantDefaults,
 *   created_at: string,
 *   updated_at: string
 * }} TenantProfile
 */

/**
 * @typedef {{
 *   tenant: TenantProfile,
 *   providers: ProviderProfile[],
 *   proxy: { text_path: string, v2_path: string, dictation_path: string }
 * }} ManagementTenantProfile
 */

/**
 * @typedef {{
 *   id: string,
 *   email?: string,
 *   display_name?: string,
 *   avatar_url?: string,
 *   is_admin: boolean
 * }} ManagementUser
 */

/**
 * @typedef {{
 *   id: string,
 *   name: string,
 *   has_secret: boolean,
 *   created_at: string,
 *   updated_at: string
 * }} ManagementTenantSummary
 */

/**
 * @typedef {{
 *   user: ManagementUser,
 *   tenants: ManagementTenantSummary[]
 * }} ManagementAccount
 */

/**
 * @typedef {{
 *   secret: string,
 *   profile: ManagementTenantProfile
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
 * @typedef {"all" | "30d" | "7d" | "1d"} UsageInterval
 */

/**
 * @typedef {
 *   "success" |
 *   "invalid_request" |
 *   "payload_too_large" |
 *   "rate_limited" |
 *   "service_unavailable" |
 *   "request_timeout" |
 *   "upstream_error"
 * } UsageOutcomeCode
 */

/**
 * @typedef {{
 *   occurred_at: string,
 *   endpoint: string,
 *   provider: string,
 *   model: string,
 *   status_code: number,
 *   outcome_code: UsageOutcomeCode,
 *   latency_ms: number
 * }} ManagementUsageFailure
 */

/**
 * @typedef {ManagementUsageFailure & {
 *   tenant_id: string,
 *   tenant_name: string
 * }} ManagementAccountUsageFailure
 */

/**
 * @typedef {{
 *   interval: UsageInterval,
 *   failures: ManagementUsageFailure[],
 *   next_cursor?: string
 * }} ManagementUsageFailurePage
 */

/**
 * @typedef {{
 *   interval: UsageInterval,
 *   failures: ManagementAccountUsageFailure[],
 *   next_cursor?: string
 * }} ManagementAccountUsageFailurePage
 */

/**
 * @typedef {{
 *   interval: UsageInterval,
 *   bucket_unit: "day" | "hour",
 *   totals: UsageAggregate,
 *   buckets: Array<{ start: string, data: UsageAggregate }>,
 *   providers: Array<{ provider: string, data: UsageAggregate }>,
 *   models: Array<{ provider: string, model: string, data: UsageAggregate }>,
 *   status_codes: Array<{ status_code: number, requests: number }>
 * }} ManagementUsageSummary
 */

/**
 * @typedef {{
 *   period_days: number,
 *   totals: UsageAggregate,
 *   daily: Array<{ date: string, data: UsageAggregate }>,
 *   providers: Array<{ provider: string, data: UsageAggregate }>,
 *   models: Array<{ provider: string, model: string, data: UsageAggregate }>,
 *   status_codes: Array<{ status_code: number, requests: number }>
 * }} ManagementAdminUsageSummary
 */

/**
 * @typedef {{
 *   user: { id: string, email?: string, display_name?: string, avatar_url?: string, is_admin: boolean },
 *   tenant_count: number,
 *   tenants: Array<{
 *     id: string,
 *     name: string,
 *     has_secret: boolean,
 *     created_at: string,
 *     updated_at: string,
 *     usage: ManagementAdminUsageSummary
 *   }>
 * }} ManagementAdminUser
 */

/**
 * @typedef {{
 *   period_days: number,
 *   users: ManagementAdminUser[]
 * }} ManagementAdminUsersResponse
 */

export {};
