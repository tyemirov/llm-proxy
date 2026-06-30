// @ts-check

import { MPR_UI } from "../constants.js";

const MANAGEMENT_BASE_PATH = "/api/management";
const HEADER_CONTENT_TYPE = "Content-Type";
const MIME_JSON = "application/json";
const EMPTY_STRING = "";
const CONFIG_UI_PATH = "/config-ui.yaml";
const PRODUCTION_FRONTEND_HOST = "llm-proxy.mprlab.com";
const PRODUCTION_BACKEND_ORIGIN = "https://llm-proxy-api.mprlab.com";

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
 * @returns {Promise<import("../types.d.js").ManagementProfile>}
 */
export function fetchProfile() {
  return requestJSON(`${MANAGEMENT_BASE_PATH}/profile`, { method: "GET" });
}

/**
 * @returns {Promise<import("../types.d.js").ManagementUsageSummary>}
 */
export function fetchUsageSummary() {
  return requestJSON(`${MANAGEMENT_BASE_PATH}/usage`, { method: "GET" });
}

/**
 * @returns {Promise<import("../types.d.js").ManagementAdminUsersResponse>}
 */
export function fetchAdminUsers() {
  return requestJSON(`${MANAGEMENT_BASE_PATH}/admin/users`, { method: "GET" });
}

/**
 * @param {string} provider
 * @param {string} apiKey
 * @returns {Promise<import("../types.d.js").ManagementProfile>}
 */
export function saveProviderKey(provider, apiKey) {
  return requestJSON(`${MANAGEMENT_BASE_PATH}/provider-keys/${encodeURIComponent(provider)}`, {
    method: "PUT",
    body: { api_key: apiKey },
  });
}

/**
 * @param {string} provider
 * @returns {Promise<import("../types.d.js").ManagementProfile>}
 */
export function removeProviderKey(provider) {
  return requestJSON(`${MANAGEMENT_BASE_PATH}/provider-keys/${encodeURIComponent(provider)}`, { method: "DELETE" });
}

/**
 * @param {import("../types.d.js").TenantDefaults} defaults
 * @returns {Promise<import("../types.d.js").ManagementProfile>}
 */
export function updateDefaults(defaults) {
  return requestJSON(`${MANAGEMENT_BASE_PATH}/defaults`, {
    method: "PUT",
    body: defaults,
  });
}

/**
 * @returns {Promise<import("../types.d.js").SecretResponse>}
 */
export function generateSecret() {
  return requestJSON(`${MANAGEMENT_BASE_PATH}/secrets`, { method: "POST" });
}

/**
 * @returns {Promise<import("../types.d.js").ManagementProfile>}
 */
export function revokeSecret() {
  return requestJSON(`${MANAGEMENT_BASE_PATH}/secrets`, { method: "DELETE" });
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
 * @param {{ method: string, body?: unknown }} options
 * @returns {Promise<any>}
 */
async function requestJSON(path, options) {
  const runtimeConfig = await loadFrontendRuntimeConfig();
  const requestInit = {
    method: options.method,
    credentials: "include",
    headers: {},
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
  return response.json();
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
  if (window.location.hostname === PRODUCTION_FRONTEND_HOST) {
    return new URL(CONFIG_UI_PATH, PRODUCTION_BACKEND_ORIGIN).toString();
  }
  return new URL(CONFIG_UI_PATH, window.location.href).toString();
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
