// @ts-check

/**
 * @typedef {{
 *   provider: string,
 *   model: string,
 *   dictation_provider: string,
 *   dictation_model: string,
 *   system_prompt: string
 * }} TenantDefaults
 */

/**
 * @typedef {{
 *   id: string,
 *   label: string,
 *   aliases: string[],
 *   has_key: boolean,
 *   masked_key?: string,
 *   text_default_model: string,
 *   text_models: string[],
 *   supports_dictation: boolean,
 *   dictation_default_model?: string,
 *   dictation_models: string[]
 * }} ProviderProfile
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
 *   user: { id: string, email?: string, display_name?: string, avatar_url?: string },
 *   tenant: TenantProfile,
 *   providers: ProviderProfile[],
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

export {};
