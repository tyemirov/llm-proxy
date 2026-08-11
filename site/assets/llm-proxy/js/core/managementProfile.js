// @ts-check

import {
  APP_INTEGRITY_ERROR,
  COPY,
  ROUTING_DEFAULTS_INVALID_ERROR,
} from "../constants.js?v=20260811b130";

const EMPTY_STRING = "";
const TENANT_NAME_MAXIMUM_CHARACTERS = 80;

/** @returns {import("../types.d.js").TenantDefaults} */
export function emptyDefaults() {
  return {
    provider: EMPTY_STRING,
    model: EMPTY_STRING,
    dictation_provider: EMPTY_STRING,
    dictation_model: EMPTY_STRING,
    system_prompt: EMPTY_STRING,
    reasoning_effort: EMPTY_STRING,
  };
}

/**
 * @param {import("../types.d.js").ManagementTenantProfile} profile
 * @returns {import("../types.d.js").TenantDefaults}
 */
export function createAppRoutingDefaults(profile) {
  const tenant = profile && typeof profile.tenant === "object" ? profile.tenant : null;
  const defaults = tenant && typeof tenant.defaults === "object" ? tenant.defaults : null;
  const providers = Array.isArray(profile && profile.providers) ? profile.providers : null;
  if (!defaults || !providers || !routingDefaultsAreStrings(defaults) || Object.hasOwn(profile, "reasoning_effort_options")) {
    throw new Error(APP_INTEGRITY_ERROR);
  }
  for (const provider of providers) {
    assertProviderCatalog(provider);
  }
  const keyedTextProviders = providers.filter((provider) => provider.has_key);
  let textModel = null;
  if (keyedTextProviders.length === 0) {
    if (defaults.provider !== EMPTY_STRING || defaults.model !== EMPTY_STRING || defaults.reasoning_effort !== EMPTY_STRING) {
      throw new Error(APP_INTEGRITY_ERROR);
    }
  } else {
    const textProvider = profileProvider(keyedTextProviders, defaults.provider);
    textModel = textProvider.text_models.find((model) => model.id === defaults.model) || null;
    if (!textModel) {
      throw new Error(APP_INTEGRITY_ERROR);
    }
  }
  const dictationProviders = keyedTextProviders.filter((provider) => provider.supports_dictation);
  if (dictationProviders.length === 0) {
    if (defaults.dictation_provider !== EMPTY_STRING || defaults.dictation_model !== EMPTY_STRING) {
      throw new Error(APP_INTEGRITY_ERROR);
    }
  } else {
    const dictationProvider = profileProvider(dictationProviders, defaults.dictation_provider);
    if (!dictationProvider.dictation_models.includes(defaults.dictation_model)) {
      throw new Error(APP_INTEGRITY_ERROR);
    }
  }
  if (
    defaults.reasoning_effort !== EMPTY_STRING &&
    (!textModel || !reasoningEffortOptionsForTextModel(textModel).includes(defaults.reasoning_effort))
  ) {
    throw new Error(APP_INTEGRITY_ERROR);
  }
  return {
    provider: defaults.provider,
    model: defaults.model,
    dictation_provider: defaults.dictation_provider,
    dictation_model: defaults.dictation_model,
    system_prompt: defaults.system_prompt,
    reasoning_effort: defaults.reasoning_effort,
  };
}

/**
 * @param {import("../types.d.js").ProviderProfile[]} providers
 * @param {string} providerID
 * @returns {import("../types.d.js").ProviderProfile}
 */
export function profileProvider(providers, providerID) {
  const provider = providers.find((candidateProvider) => candidateProvider.id === providerID);
  if (!provider) {
    throw new Error(APP_INTEGRITY_ERROR);
  }
  return provider;
}

/**
 * @param {import("../types.d.js").ManagementAccount | null} account
 * @param {import("../types.d.js").ManagementTenantSummary[]} tenants
 * @returns {import("../types.d.js").ManagementAccount}
 */
export function managementAccountWithTenants(account, tenants) {
  if (!account) {
    throw new Error(APP_INTEGRITY_ERROR);
  }
  return { ...account, tenants };
}

/** @param {import("../types.d.js").ManagementAccount} account */
export function assertManagementAccount(account) {
  if (
    !account ||
    !account.user ||
    typeof account.user.id !== "string" ||
    account.user.id === EMPTY_STRING ||
    typeof account.user.is_admin !== "boolean" ||
    !Array.isArray(account.tenants) ||
    account.tenants.length === 0
  ) {
    throw new Error(APP_INTEGRITY_ERROR);
  }
  const tenantIDs = new Set();
  const tenantNames = new Set();
  for (const tenant of account.tenants) {
    if (
      !tenant ||
      typeof tenant.id !== "string" ||
      tenant.id === EMPTY_STRING ||
      typeof tenant.name !== "string" ||
      tenant.name === EMPTY_STRING ||
      typeof tenant.has_secret !== "boolean" ||
      typeof tenant.created_at !== "string" ||
      typeof tenant.updated_at !== "string" ||
      tenantIDs.has(tenant.id) ||
      tenantNames.has(tenant.name.toLocaleLowerCase("en-US"))
    ) {
      throw new Error(APP_INTEGRITY_ERROR);
    }
    tenantIDs.add(tenant.id);
    tenantNames.add(tenant.name.toLocaleLowerCase("en-US"));
  }
}

/**
 * @param {import("../types.d.js").ManagementTenantProfile} profile
 * @param {string} tenantID
 */
export function assertManagementTenantProfile(profile, tenantID) {
  if (
    !profile ||
    !profile.tenant ||
    profile.tenant.id !== tenantID ||
    typeof profile.tenant.name !== "string" ||
    profile.tenant.name === EMPTY_STRING ||
    typeof profile.tenant.has_secret !== "boolean" ||
    typeof profile.tenant.created_at !== "string" ||
    typeof profile.tenant.updated_at !== "string" ||
    !Array.isArray(profile.providers) ||
    !profile.proxy ||
    typeof profile.proxy.text_path !== "string" ||
    typeof profile.proxy.v2_path !== "string" ||
    typeof profile.proxy.dictation_path !== "string"
  ) {
    throw new Error(APP_INTEGRITY_ERROR);
  }
}

/**
 * @param {import("../types.d.js").ManagementTenantProfile} profile
 * @returns {import("../types.d.js").ManagementTenantSummary}
 */
export function tenantSummaryFromProfile(profile) {
  return {
    id: profile.tenant.id,
    name: profile.tenant.name,
    has_secret: profile.tenant.has_secret,
    created_at: profile.tenant.created_at,
    updated_at: profile.tenant.updated_at,
  };
}

/**
 * @param {string} value
 * @returns {string}
 */
export function validatedTenantName(value) {
  const name = value.trim();
  if (
    name === EMPTY_STRING ||
    Array.from(name).length > TENANT_NAME_MAXIMUM_CHARACTERS ||
    /[\p{Cc}\p{Cf}\p{Zl}\p{Zp}]/u.test(name)
  ) {
    throw new Error("managed_tenant_name_invalid");
  }
  return name;
}

/**
 * @param {unknown} requestError
 * @returns {boolean}
 */
export function isAbortError(requestError) {
  return requestError instanceof DOMException && requestError.name === "AbortError";
}

/**
 * @param {unknown} requestError
 * @returns {string}
 */
export function profileFailureMessage(requestError) {
  if (
    requestError instanceof Error &&
    (requestError.message === APP_INTEGRITY_ERROR || requestError.message.includes(ROUTING_DEFAULTS_INVALID_ERROR))
  ) {
    return COPY.appIntegrityError;
  }
  return COPY.requestFailed;
}

/**
 * @param {import("../types.d.js").TenantDefaults} defaults
 * @returns {boolean}
 */
function routingDefaultsAreStrings(defaults) {
  return (
    typeof defaults.provider === "string" &&
    typeof defaults.model === "string" &&
    typeof defaults.dictation_provider === "string" &&
    typeof defaults.dictation_model === "string" &&
    typeof defaults.system_prompt === "string" &&
    typeof defaults.reasoning_effort === "string"
  );
}

/** @param {import("../types.d.js").ProviderProfile} provider */
function assertProviderCatalog(provider) {
  if (
    !provider ||
    typeof provider.id !== "string" ||
    !provider.id ||
    typeof provider.has_key !== "boolean" ||
    !Array.isArray(provider.text_models) ||
    !provider.text_models.some((model) => model && model.id === provider.text_default_model) ||
    !provider.text_models.some((model) => model && model.id === provider.text_model)
  ) {
    throw new Error(APP_INTEGRITY_ERROR);
  }
  if (Object.hasOwn(provider, "reasoning_effort")) {
    throw new Error(APP_INTEGRITY_ERROR);
  }
  for (const model of provider.text_models) {
    if (!model || typeof model.id !== "string" || !model.id) {
      throw new Error(APP_INTEGRITY_ERROR);
    }
    assertReasoningEffortCapability(model.reasoning_effort);
  }
  if (
    provider.supports_dictation &&
    (!Array.isArray(provider.dictation_models) ||
      typeof provider.dictation_default_model !== "string" ||
      !provider.dictation_models.includes(provider.dictation_default_model))
  ) {
    throw new Error(APP_INTEGRITY_ERROR);
  }
}

/**
 * @param {import("../types.d.js").TextModelProfile} model
 * @returns {string[]}
 */
function reasoningEffortOptionsForTextModel(model) {
  return model.reasoning_effort ? model.reasoning_effort.efforts : [];
}

/** @param {unknown} capability */
function assertReasoningEffortCapability(capability) {
  if (capability === undefined) {
    return;
  }
  if (!capability || typeof capability !== "object") {
    throw new Error(APP_INTEGRITY_ERROR);
  }
  const candidate = /** @type {Record<string, unknown>} */ (capability);
  const efforts = candidate.efforts;
  if (
    typeof candidate.adapter !== "string" ||
    candidate.adapter === EMPTY_STRING ||
    candidate.adapter !== candidate.adapter.trim() ||
    !Array.isArray(efforts) ||
    efforts.length === 0 ||
    new Set(efforts).size !== efforts.length ||
    !efforts.every((/** @type {unknown} */ effort) => typeof effort === "string" && effort !== EMPTY_STRING && effort === effort.trim())
  ) {
    throw new Error(APP_INTEGRITY_ERROR);
  }
}
