// @ts-check

const MANAGEMENT_BASE_PATH = "/api/management";
const FRONTEND_CONFIG_PATH = "/llm-proxy-config.json";
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
 * @returns {Promise<import("../types.d.js").ManagementProfile>}
 */
export function fetchProfile() {
  return requestJSON(`${MANAGEMENT_BASE_PATH}/profile`, { method: "GET" });
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
    frontendRuntimeConfigPromise = fetch(FRONTEND_CONFIG_PATH, { credentials: "same-origin" })
      .then(async (response) => {
        if (!response.ok) {
          throw new BackendClientError(await response.text(), response.status);
        }
        return response.json();
      })
      .then((rawConfig) => createFrontendRuntimeConfig(rawConfig));
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
  if (options.body !== undefined) {
    requestInit.headers = { [HEADER_CONTENT_TYPE]: MIME_JSON };
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
 * @returns {import("../types.d.js").FrontendRuntimeConfig}
 */
function createFrontendRuntimeConfig(rawConfig) {
  if (!rawConfig || typeof rawConfig !== "object") {
    throw new Error("frontend_config_invalid");
  }
  const configRecord = /** @type {{ managementApiOrigin?: unknown, proxyOrigin?: unknown }} */ (rawConfig);
  return {
    managementApiOrigin: normalizedOrigin(configRecord.managementApiOrigin, "managementApiOrigin"),
    proxyOrigin: normalizedOrigin(configRecord.proxyOrigin, "proxyOrigin"),
  };
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
