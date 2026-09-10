// @ts-check

import {assertManagementAccount, assertManagementTenantProfile, assertProviderCatalog, assertProviderField} from "./managementProfile.js?v=20260903f037";

import { APP_INTEGRITY_ERROR, MPR_UI } from "../constants.js?v=20260903f037";

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

const MANAGEMENT_FAILURE_MESSAGES = new Map([
  ['managed_tenant_name_conflict', 'A tenant already uses this name. Choose another name.'],
  ['managed_connection_conflict', 'The connection changed. Reload its details before saving.'],
  ['managed_connection_assigned', 'Detach this connection from its tenants before deleting it.'],
  ['managed_connection_invalid', 'Check the connection name, credentials, and required settings.'],
  ['provider_key_rejected', 'The provider rejected these credentials. Check the key and required settings.'],
  ['provider_key_verification_rate_limited', 'The provider rate limit prevented verification. Try again later.'],
  ['provider_key_verification_timed_out', 'Provider verification timed out. Try again.'],
  ['provider_key_verification_unavailable', 'Provider verification is unavailable. Try again later.'],
]);

/** @param {BackendClientError} error @returns {string} */
export function managementFailureMessage(error) {
  let code = error.message;
  if (code.startsWith('{')) {
    try { code = JSON.parse(code).error?.code; }
    catch { code = EMPTY_STRING; }
  }
  return MANAGEMENT_FAILURE_MESSAGES.get(code) || 'Unable to complete this change. Try again.';
}

/**
 * @param {AbortSignal} [signal]
 * @returns {Promise<import("../types.d.js").ManagementAccount>}
 */
export async function fetchAccount(signal) {
  const account = await requestJSON(`${MANAGEMENT_BASE_PATH}/account`, { method: "GET", signal });
  assertManagementAccount(account);
  return account;
}

/**
 * @param {string} tenantID
 * @param {AbortSignal} [signal]
 * @returns {Promise<import("../types.d.js").ManagementTenantProfile>}
 */
export function fetchTenant(tenantID, signal) {
  return requestTenantProfile(managementTenantPath(tenantID), { method: "GET", signal }, tenantID);
}

/**
 * @param {string} name
 * @param {AbortSignal} [signal]
 * @returns {Promise<import("../types.d.js").ManagementTenantProfile>}
 */
export function createTenant(name, signal) {
  return requestTenantProfile(`${MANAGEMENT_BASE_PATH}/tenants`, {
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
  return requestTenantProfile(managementTenantPath(tenantID), {
    method: "PUT",
    body: { name },
    signal,
  }, tenantID);
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
 * @param {string} tenantID
 * @param {import("../types.d.js").UsageInterval} interval
 * @param {number} limit
 * @param {string} cursor
 * @param {AbortSignal} [signal]
 * @returns {Promise<import("../types.d.js").ManagementUsageRejectionPage>}
 */
export function fetchUsageRejections(tenantID, interval, limit, cursor, signal) {
  const query = new URLSearchParams({ interval, limit: String(limit) });
  if (cursor) query.set("cursor", cursor);
  return requestJSON(`${managementTenantPath(tenantID)}/usage/rejections?${query}`, {
    method: "GET",
    signal,
  });
}

/**
 * @param {import("../types.d.js").UsageInterval} interval
 * @param {number} limit
 * @param {string} cursor
 * @param {AbortSignal} [signal]
 * @returns {Promise<import("../types.d.js").ManagementAccountUsageRejectionPage>}
 */
export function fetchAccountUsageRejections(interval, limit, cursor, signal) {
  const query = new URLSearchParams({ interval, limit: String(limit) });
  if (cursor) query.set("cursor", cursor);
  return requestJSON(`${MANAGEMENT_BASE_PATH}/usage/rejections?${query}`, {
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
 * @param {import("../types.d.js").TenantDefaults} defaults
 * @param {AbortSignal} [signal]
 * @returns {Promise<import("../types.d.js").ManagementTenantProfile>}
 */
export function updateDefaults(tenantID, defaults, signal) {
  return requestTenantProfile(`${managementTenantPath(tenantID)}/defaults`, {
    method: "PUT",
    body: defaults,
    signal,
  }, tenantID);
}

/**
 * @param {string} tenantID
 * @param {AbortSignal} [signal]
 * @returns {Promise<import("../types.d.js").SecretResponse>}
 */
export async function generateSecret(tenantID, signal) {
  const response = await requestJSON(`${managementTenantPath(tenantID)}/secrets`, { method: "POST", signal });
  if (!response || typeof response.secret !== 'string' || !response.secret.trim()) throw new Error(APP_INTEGRITY_ERROR);
  assertManagementTenantProfile(response.profile, tenantID);
  if (!response.profile.tenant.has_secret) throw new Error(APP_INTEGRITY_ERROR);
  return response;
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
 * @param {{ method: string, body?: unknown, signal?: AbortSignal, idempotencyKey?: string }} options
 * @returns {Promise<any>}
 */
async function requestJSON(path, options) {
  const runtimeConfig = await loadFrontendRuntimeConfig();
  /** @type {RequestInit} */
  const requestInit = {
    method: options.method,
    credentials: "include",
    headers: {},
    signal: options.signal,
  };
  if (options.method !== "GET") {
    requestInit.headers = { [HEADER_CONTENT_TYPE]: MIME_JSON };
  }
  if (options.idempotencyKey) {
    requestInit.headers = {...requestInit.headers, "Idempotency-Key": options.idempotencyKey};
  }
  if (options.body !== undefined) {
    requestInit.body = JSON.stringify(options.body);
  }
  const authHost = document.getElementById(MPR_UI.HEADER_ID);
  if (!authHost) throw new Error(MPR_UI.HEADER_MISSING);
  const runtimeGlobal = /** @type {typeof globalThis & { MPRUI: {
   * authenticatedFetch: (host: HTMLElement, url: string, init: RequestInit, options: {
   *   mutationReplay: "authorization-before-domain-work"
   * }) => Promise<Response>
   * } }} */ (globalThis);
  const response = await runtimeGlobal.MPRUI.authenticatedFetch(
    authHost,
    `${runtimeConfig.managementApiOrigin}${path}`,
    requestInit,
    { mutationReplay: "authorization-before-domain-work" },
  );
  if (!response.ok) {
    throw new BackendClientError(await response.text(), response.status);
  }
  if (response.status === 204) {
    return undefined;
  }
  return response.json();
}

/**
 * @param {string} path
 * @param {{method:string, body?:unknown, signal?:AbortSignal}} options
 * @param {string} [tenantID]
 * @returns {Promise<import('../types.d.js').ManagementTenantProfile>}
 */
async function requestTenantProfile(path, options, tenantID) {
  const profile = await requestJSON(path, options);
  assertManagementTenantProfile(profile, tenantID);
  return profile;
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

/** @param {AbortSignal} [signal] @returns {Promise<import('../types.d.js').AccountConnections>} */
export async function fetchConnections(signal) {
  /** @type {import('../types.d.js').AccountConnection[]} */
  const connections = [];
  const identifiers = new Set();
  const assignments = new Set();
  let cursor = EMPTY_STRING;
  let result;
  do {
    const query = cursor ? `?cursor=${encodeURIComponent(cursor)}` : EMPTY_STRING;
    result = await requestJSON(`${MANAGEMENT_BASE_PATH}/connections${query}`, {method: 'GET', signal});
    if (!result || !Array.isArray(result.connections) || !Array.isArray(result.providers) ||
        typeof result.next_cursor !== 'string' || (result.next_cursor !== EMPTY_STRING &&
        (!CONNECTION_ID_PATTERN.test(result.next_cursor) || result.next_cursor <= cursor || !result.connections.length))) {
      throw new Error(APP_INTEGRITY_ERROR);
    }
    const providerIDs = new Set();
    for (const provider of result.providers) {
      assertProviderCatalog(provider);
      if (providerIDs.has(provider.id)) throw new Error(APP_INTEGRITY_ERROR);
      providerIDs.add(provider.id);
    }
    for (const connection of result.connections) {
      assertAccountConnection(connection);
      if (identifiers.has(connection.id) || !providerIDs.has(connection.provider)) throw new Error(APP_INTEGRITY_ERROR);
      identifiers.add(connection.id);
      for (const tenantID of connection.tenant_ids) {
        const assignment = JSON.stringify([tenantID, connection.provider]);
        if (assignments.has(assignment)) throw new Error(APP_INTEGRITY_ERROR);
        assignments.add(assignment);
      }
    }
    if (result.next_cursor && result.next_cursor !== result.connections.at(-1).id) throw new Error(APP_INTEGRITY_ERROR);
    connections.push(...result.connections);
    cursor = result.next_cursor;
  } while (cursor);
  return {connections, providers: result.providers};
}

const CONNECTION_ID_PATTERN = /^connection-[a-f0-9]{32}$/;

/** @param {import('../types.d.js').AccountConnection} connection */
function assertAccountConnection(connection) {
  if (!connection || typeof connection.id !== 'string' || !CONNECTION_ID_PATTERN.test(connection.id) ||
      typeof connection.name !== 'string' || !connection.name.trim() || typeof connection.provider !== 'string' || !connection.provider ||
      !Number.isSafeInteger(connection.version) || connection.version < 1 ||
      typeof connection.created_at !== 'string' || !Number.isFinite(Date.parse(connection.created_at)) ||
      typeof connection.updated_at !== 'string' || !Number.isFinite(Date.parse(connection.updated_at)) ||
      !Array.isArray(connection.tenant_ids) || !connection.tenant_ids.every(id => typeof id === 'string' && id.length > 0) ||
      new Set(connection.tenant_ids).size !== connection.tenant_ids.length ||
      !Array.isArray(connection.fields) || !connection.fields.length) throw new Error(APP_INTEGRITY_ERROR);
  const fieldIDs = new Set();
  for (const field of connection.fields) {
    assertProviderField(field);
    if (fieldIDs.has(field.id)) throw new Error(APP_INTEGRITY_ERROR);
    fieldIDs.add(field.id);
  }
}

/** @param {string} id @param {{name:string, provider:string, fields:Record<string,string>, version:number}} body @param {AbortSignal} [signal] @param {string} [idempotencyKey] @returns {Promise<import('../types.d.js').AccountConnection>} */
export async function saveConnection(id, body, signal, idempotencyKey) {
  const connection = await requestJSON(`${MANAGEMENT_BASE_PATH}/connections${id ? '/' + encodeURIComponent(id) : ''}`, {method:id ? 'PUT':'POST', body, signal, idempotencyKey});
  assertAccountConnection(connection);
  if ((id && connection.id !== id) || connection.provider !== body.provider || connection.version !== body.version + 1) throw new Error(APP_INTEGRITY_ERROR);
  return connection;
}

/** @param {string} id @param {AbortSignal} [signal] @returns {Promise<void>} */
export function deleteConnection(id, signal) {
  return requestJSON(`${MANAGEMENT_BASE_PATH}/connections/${encodeURIComponent(id)}`, {method:'DELETE', signal});
}

/** @param {string} tenantID @param {string} provider @param {string} connectionID @param {AbortSignal} [signal] @returns {Promise<import('../types.d.js').ManagementTenantProfile>} */
export function assignConnection(tenantID, provider, connectionID, signal) {
  return requestTenantProfile(`${managementTenantPath(tenantID)}/connections/${encodeURIComponent(provider)}`, {method:'PUT',body:{connection_id:connectionID},signal}, tenantID);
}

/** @param {string} tenantID @param {string} provider @param {boolean} clearDefaults @param {AbortSignal} [signal] @returns {Promise<void>} */
export function detachConnection(tenantID, provider, clearDefaults, signal) {
  return requestJSON(`${managementTenantPath(tenantID)}/connections/${encodeURIComponent(provider)}?clear_defaults=${clearDefaults}`, {method:'DELETE',signal});
}

/** @param {string} tenantID @param {string} provider @param {{text_model:string, system_prompt:string}} body @param {AbortSignal} [signal] @returns {Promise<import('../types.d.js').ManagementTenantProfile>} */
export function saveTenantProviderProfile(tenantID, provider, body, signal) {
  return requestTenantProfile(`${managementTenantPath(tenantID)}/provider-profiles/${encodeURIComponent(provider)}`, {method:'PUT',body,signal}, tenantID);
}

/** @param {AbortSignal} [signal] @returns {Promise<Record<string, string>>} */
export async function fetchModelFamilies(signal) {
  const config = await loadFrontendRuntimeConfig();
  const response = await fetch(`${config.managementApiOrigin}/api/public/capabilities`, {signal, credentials:'omit'});
  if (!response.ok) throw new BackendClientError(await response.text(), response.status);
  const result = await response.json();
  if (!result || !Array.isArray(result.models)) throw new Error('Invalid model catalog');
  /** @type {Record<string, string>} */
  const families = {};
  for (const model of result.models) {
    if (!model || typeof model.identifier !== 'string' || typeof model.family !== 'string' || !model.identifier || !model.family || Object.hasOwn(families, model.identifier)) throw new Error('Invalid model identity');
    families[model.identifier] = model.family;
  }
  return families;
}
