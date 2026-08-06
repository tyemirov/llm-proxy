// @ts-check

import { MPR_UI } from "../constants.js?v=20260806b109";

const MANAGEMENT_BASE_PATH = "/api/management";
const HEADER_CONTENT_TYPE = "Content-Type";
const MIME_JSON = "application/json";
const EMPTY_STRING = "";

/** @type {Promise<import("../types.d.js").FrontendRuntimeConfig> | null} */
let frontendRuntimeConfigPromise = null;

export class BackendClientError extends Error {
  /**
   * @param {string} message
   * @param {number} status
   */
  constructor(message, status) {
    super(message);
    this.name = "BackendClientError";
    this.status = status;
  }
}

/**
 * @param {AbortSignal} [signal]
 * @returns {Promise<import("../types.d.js").ManagementAccount>}
 */
export function fetchAccount(signal) {
  return requestJSON(`${MANAGEMENT_BASE_PATH}/account`, { method: "GET", signal });
}

/**
 * @param {string} tenantID
 * @param {AbortSignal} [signal]
 * @returns {Promise<import("../types.d.js").ManagementTenantProfile>}
 */
export function fetchTenant(tenantID, signal) {
  return requestJSON(managementTenantPath(tenantID), { method: "GET", signal });
}

/**
 * @param {string} name
 * @param {AbortSignal} [signal]
 * @returns {Promise<import("../types.d.js").ManagementTenantProfile>}
 */
export function createTenant(name, signal) {
  return requestJSON(`${MANAGEMENT_BASE_PATH}/tenants`, {
    method: "POST",
    body: { name },
    signal,
  });
}

/**
 * @param {string} tenantID
 * @param {string} name
 * @param {AbortSignal} [signal]
 * @returns {Promise<import("../types.d.js").ManagementTenantProfile>}
 */
export function renameTenant(tenantID, name, signal) {
  return requestJSON(managementTenantPath(tenantID), {
    method: "PUT",
    body: { name },
    signal,
  });
}

/**
 * @param {string} tenantID
 * @param {AbortSignal} [signal]
 * @returns {Promise<void>}
 */
export function deleteTenant(tenantID, signal) {
  return requestJSON(managementTenantPath(tenantID), {
    method: "DELETE",
    signal,
  });
}

/**
 * @param {string} tenantID
 * @param {import("../types.d.js").UsageInterval} interval
 * @param {AbortSignal} [signal]
 * @returns {Promise<import("../types.d.js").ManagementUsageSummary>}
 */
export function fetchUsageSummary(tenantID, interval, signal) {
  return requestJSON(`${managementTenantPath(tenantID)}/usage?interval=${encodeURIComponent(interval)}`, {
    method: "GET",
    signal,
  });
}

/**
 * @param {import("../types.d.js").UsageInterval} interval
 * @param {AbortSignal} [signal]
 * @returns {Promise<import("../types.d.js").ManagementUsageSummary>}
 */
export function fetchAccountUsageSummary(interval, signal) {
  return requestJSON(`${MANAGEMENT_BASE_PATH}/usage?interval=${encodeURIComponent(interval)}`, {
    method: "GET",
    signal,
  });
}

/**
 * @param {string} tenantID
 * @param {import("../types.d.js").UsageInterval} interval
 * @param {number} limit
 * @param {string} cursor
 * @param {AbortSignal} [signal]
 * @returns {Promise<import("../types.d.js").ManagementUsageFailurePage>}
 */
export function fetchUsageFailures(tenantID, interval, limit, cursor, signal) {
  const query = new URLSearchParams({
    interval,
    limit: String(limit),
  });
  if (cursor) {
    query.set("cursor", cursor);
  }
  return requestJSON(`${managementTenantPath(tenantID)}/usage/failures?${query}`, {
    method: "GET",
    signal,
  });
}

/**
 * @param {import("../types.d.js").UsageInterval} interval
 * @param {number} limit
 * @param {string} cursor
 * @param {AbortSignal} [signal]
 * @returns {Promise<import("../types.d.js").ManagementAccountUsageFailurePage>}
 */
export function fetchAccountUsageFailures(interval, limit, cursor, signal) {
  const query = new URLSearchParams({
    interval,
    limit: String(limit),
  });
  if (cursor) {
    query.set("cursor", cursor);
  }
  return requestJSON(`${MANAGEMENT_BASE_PATH}/usage/failures?${query}`, {
    method: "GET",
    signal,
  });
}

/**
 * @returns {Promise<import("../types.d.js").ManagementAdminUsersResponse>}
 */
export function fetchAdminUsers() {
  return requestJSON(`${MANAGEMENT_BASE_PATH}/admin/users`, { method: "GET" });
}

/**
 * @param {string} tenantID
 * @param {string} provider
 * @param {string} apiKey
 * @param {string} textModel
 * @param {string} systemPrompt
 * @param {AbortSignal} [signal]
 * @returns {Promise<import("../types.d.js").ManagementTenantProfile>}
 */
export function saveProviderKey(tenantID, provider, apiKey, textModel, systemPrompt, signal) {
  return requestJSON(`${managementTenantPath(tenantID)}/provider-keys/${encodeURIComponent(provider)}`, {
    method: "PUT",
    body: { api_key: apiKey, text_model: textModel, system_prompt: systemPrompt },
    signal,
  });
}

/**
 * @param {string} tenantID
 * @param {string} provider
 * @param {AbortSignal} [signal]
 * @returns {Promise<import("../types.d.js").ManagementTenantProfile>}
 */
export function removeProviderKey(tenantID, provider, signal) {
  return requestJSON(`${managementTenantPath(tenantID)}/provider-keys/${encodeURIComponent(provider)}`, {
    method: "DELETE",
    signal,
  });
}

/**
 * @param {string} tenantID
 * @param {string} provider
 * @param {AbortSignal} [signal]
 * @returns {Promise<import("../types.d.js").ProviderKeyReveal>}
 */
export function revealProviderKey(tenantID, provider, signal) {
  return requestJSON(`${managementTenantPath(tenantID)}/provider-keys/${encodeURIComponent(provider)}/reveal`, {
    method: "POST",
    signal,
  });
}

/**
 * @param {string} tenantID
 * @param {import("../types.d.js").TenantDefaults} defaults
 * @param {AbortSignal} [signal]
 * @returns {Promise<import("../types.d.js").ManagementTenantProfile>}
 */
export function updateDefaults(tenantID, defaults, signal) {
  return requestJSON(`${managementTenantPath(tenantID)}/defaults`, {
    method: "PUT",
    body: defaults,
    signal,
  });
}

/**
 * @param {string} tenantID
 * @param {AbortSignal} [signal]
 * @returns {Promise<import("../types.d.js").SecretResponse>}
 */
export function generateSecret(tenantID, signal) {
  return requestJSON(`${managementTenantPath(tenantID)}/secrets`, { method: "POST", signal });
}

/**
 * @returns {Promise<import("../types.d.js").FrontendRuntimeConfig>}
 */
export function loadFrontendRuntimeConfig() {
  if (!frontendRuntimeConfigPromise) {
    const configUrl = frontendConfigURL();
    frontendRuntimeConfigPromise = fetch(configUrl, { credentials: "include" })
      .then(async (response) => {
        if (!response.ok) {
          throw new BackendClientError(await response.text(), response.status);
        }
        return response.text();
      })
      .then((configText) => createFrontendRuntimeConfig(parseFrontendConfig(configText), configUrl));
  }
  return frontendRuntimeConfigPromise;
}

/**
 * @param {string} path
 * @param {{ method: string, body?: unknown, signal?: AbortSignal }} options
 * @returns {Promise<any>}
 */
async function requestJSON(path, options) {
  const runtimeConfig = await loadFrontendRuntimeConfig();
  const requestInit = {
    method: options.method,
    credentials: "include",
    headers: {},
    signal: options.signal,
  };
  if (options.method !== "GET") {
    requestInit.headers = { [HEADER_CONTENT_TYPE]: MIME_JSON };
  }
  if (options.body !== undefined) {
    requestInit.body = JSON.stringify(options.body);
  }
  const response = await fetch(`${runtimeConfig.managementApiOrigin}${path}`, requestInit);
  if (!response.ok) {
    throw new BackendClientError(await response.text(), response.status);
  }
  if (response.status === 204) {
    return undefined;
  }
  return response.json();
}

/**
 * @param {string} tenantID
 * @returns {string}
 */
function managementTenantPath(tenantID) {
  return `${MANAGEMENT_BASE_PATH}/tenants/${encodeURIComponent(tenantID)}`;
}

/**
 * @param {unknown} rawConfig
 * @param {string} configUrl
 * @returns {import("../types.d.js").FrontendRuntimeConfig}
 */
function createFrontendRuntimeConfig(rawConfig, configUrl) {
  if (!rawConfig || typeof rawConfig !== "object") {
    throw new Error("frontend_config_invalid");
  }
  const configRecord = /** @type {{ llmProxy?: { managementApiOrigin?: unknown, proxyOrigin?: unknown } }} */ (rawConfig);
  if (!configRecord.llmProxy || typeof configRecord.llmProxy !== "object") {
    throw new Error("frontend_config_invalid: llmProxy");
  }
  return {
    configUrl,
    managementApiOrigin: normalizedOrigin(configRecord.llmProxy.managementApiOrigin, "llmProxy.managementApiOrigin"),
    proxyOrigin: normalizedOrigin(configRecord.llmProxy.proxyOrigin, "llmProxy.proxyOrigin"),
  };
}

/**
 * @returns {string}
 */
function frontendConfigURL() {
  const header = document.getElementById(MPR_UI.HEADER_ID);
  const configUrl = String(header ? header.getAttribute(MPR_UI.CONFIG_URL_ATTRIBUTE) : EMPTY_STRING).trim();
  if (!configUrl) {
    throw new Error("frontend_config_url_missing");
  }
  return new URL(configUrl, window.location.href).toString();
}

/**
 * @param {string} configText
 * @returns {unknown}
 */
function parseFrontendConfig(configText) {
  const runtimeGlobal = /** @type {typeof globalThis & { jsyaml?: { load?: (source: string) => unknown } }} */ (globalThis);
  if (!runtimeGlobal.jsyaml || typeof runtimeGlobal.jsyaml.load !== "function") {
    throw new Error(MPR_UI.YAML_LOADER_MISSING);
  }
  return runtimeGlobal.jsyaml.load(configText);
}

/**
 * @param {unknown} rawOrigin
 * @param {string} fieldName
 * @returns {string}
 */
function normalizedOrigin(rawOrigin, fieldName) {
  const origin = String(rawOrigin || EMPTY_STRING).trim();
  if (!origin) {
    throw new Error(`frontend_config_invalid: ${fieldName}`);
  }
  return new URL(origin).origin;
}
