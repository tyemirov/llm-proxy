// @ts-check

import {assertManagementAccount, assertManagementTenantProfile, assertProviderCatalog, assertProviderField} from "./managementProfile.js?v=20260903f037";

import { APP_INTEGRITY_ERROR, MPR_UI, CAPABILITY_DOMAINS } from "../constants.js?v=20260903f037";

const MANAGEMENT_BASE_PATH = "/api/management";
const HEADER_CONTENT_TYPE = "Content-Type";
const MIME_JSON = "application/json";
const EMPTY_STRING = "";
const BILLING_ACCOUNTS_PATH = `${MANAGEMENT_BASE_PATH}/billing-accounts`;
const HOSTED_GRANTS_PATH = `${MANAGEMENT_BASE_PATH}/hosted-access-grants`;

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
  return requestTenantProfile(`${managementTenantPath(tenantID)}/connections/${encodeURIComponent(provider)}`, {method:'PUT',body:{kind:'account_connection',resource_id:connectionID},signal}, tenantID);
}

/** @param {import('../types.d.js').BillingAccount} account */
function assertBillingAccount(account) {
  if (!account || typeof account.id !== 'string' || !/^billing-[a-f0-9]{32}$/.test(account.id) || account.currency !== 'USD' ||
      typeof account.created_at !== 'string' || !Number.isFinite(Date.parse(account.created_at))) throw new Error(APP_INTEGRITY_ERROR);
}

/** @param {AbortSignal} [signal] @returns {Promise<import('../types.d.js').BillingAccount|null>} */
export async function fetchBillingAccount(signal) {
  const result = await requestJSON(BILLING_ACCOUNTS_PATH, {method:'GET',signal});
  if (!result || !Array.isArray(result.billing_accounts) || result.billing_accounts.length > 1) throw new Error(APP_INTEGRITY_ERROR);
  result.billing_accounts.forEach(assertBillingAccount);
  return result.billing_accounts[0] || null;
}

/** @param {string} idempotencyKey @param {AbortSignal} [signal] @returns {Promise<import('../types.d.js').BillingAccount>} */
export async function createBillingAccount(idempotencyKey, signal) {
  const account = await requestJSON(BILLING_ACCOUNTS_PATH, {method:'POST',body:{currency:'USD'},idempotencyKey,signal});
  assertBillingAccount(account);
  return account;
}

/** @param {import('../types.d.js').JournalRequest} request */
function assertJournalRequest(request) {
  if (!request || typeof request.id !== 'string' || !/^request-[a-f0-9]{32}$/.test(request.id) ||
      typeof request.grant_id !== 'string' || !/^grant-[a-f0-9]{32}$/.test(request.grant_id) ||
      ![request.tenant_id,request.provider,request.operation,request.catalog_revision,request.execution_id].every(value=>typeof value==='string' && value.length>0) ||
      (request.model === undefined ? request.execution_kind !== 'media_operation' : typeof request.model !== 'string' || !request.model || request.model !== request.model.trim()) ||
      !Number.isSafeInteger(request.grant_revision) || request.grant_revision < 1 ||
      !['text_request','media_operation','dictation_request'].includes(request.execution_kind) ||
      !['accepted','executing','completed','failed','uncertain'].includes(request.state) ||
      !['pending','complete','unknown'].includes(request.usage_state) ||
      (request.failure_code !== undefined && typeof request.failure_code !== 'string') ||
      ![request.created_at,request.updated_at].every(value=>typeof value==='string' && Number.isFinite(Date.parse(value)))) throw new Error(APP_INTEGRITY_ERROR);
}

/** @param {string} accountID @returns {string} */
function journalRequestsPath(accountID) {
  if (!/^billing-[a-f0-9]{32}$/.test(accountID)) throw new Error(APP_INTEGRITY_ERROR);
  return `${BILLING_ACCOUNTS_PATH}/${encodeURIComponent(accountID)}/requests`;
}

/** @param {string} accountID @param {string} [cursor] @param {AbortSignal} [signal] @returns {Promise<import('../types.d.js').JournalRequestPage>} */
export async function fetchJournalRequests(accountID, cursor='', signal) {
  if (cursor && !/^request-[a-f0-9]{32}$/.test(cursor)) throw new Error(APP_INTEGRITY_ERROR);
  const page=await requestJSON(`${journalRequestsPath(accountID)}?limit=50${cursor?'&cursor='+encodeURIComponent(cursor):''}`,{method:'GET',signal});
  if (!page || !Array.isArray(page.requests) || page.requests.length>50 || typeof page.next_cursor!=='string') throw new Error(APP_INTEGRITY_ERROR);
  let previous=cursor;
  for (const request of page.requests) {
    assertJournalRequest(request);
    if (request.id<=previous) throw new Error(APP_INTEGRITY_ERROR);
    previous=request.id;
  }
  if (page.next_cursor && (!page.requests.length || page.next_cursor!==previous)) throw new Error(APP_INTEGRITY_ERROR);
  return page;
}

/** @param {string} accountID @param {string} requestID @param {AbortSignal} [signal] @returns {Promise<import('../types.d.js').JournalRequest>} */
export async function fetchJournalRequest(accountID, requestID, signal) {
  if (!/^request-[a-f0-9]{32}$/.test(requestID)) throw new Error(APP_INTEGRITY_ERROR);
  const request=await requestJSON(`${journalRequestsPath(accountID)}/${encodeURIComponent(requestID)}`,{method:'GET',signal});
  assertJournalRequest(request);
  if (request.id!==requestID) throw new Error(APP_INTEGRITY_ERROR);
  return request;
}

/** @param {import('../types.d.js').HostedAccessGrant} grant */
function assertHostedAccessGrant(grant) {
  if (!grant || typeof grant.id !== 'string' || !/^grant-[a-f0-9]{32}$/.test(grant.id) ||
      ![grant.billing_account_id,grant.tenant_id,grant.provider,grant.catalog_revision].every(value=>typeof value==='string' && value.length>0) ||
      !['active','suspended','revoked'].includes(grant.state) || !Number.isSafeInteger(grant.revision) || grant.revision < 1 ||
      ![grant.created_at,grant.updated_at].every(value=>typeof value==='string' && Number.isFinite(Date.parse(value))) ||
      !Array.isArray(grant.offerings) || !grant.offerings.length) throw new Error(APP_INTEGRITY_ERROR);
  for (const offering of grant.offerings) {
    if (!offering || (offering.model !== undefined && (typeof offering.model!=='string' || !offering.model || offering.model!==offering.model.trim())) || !Array.isArray(offering.operations) || !offering.operations.length ||
        !offering.operations.every(value=>typeof value==='string' && value.length>0)) throw new Error(APP_INTEGRITY_ERROR);
  }
}

/** @param {AbortSignal} [signal] @returns {Promise<import('../types.d.js').HostedAccessGrant[]>} */
export async function fetchHostedAccessGrants(signal) {
  /** @type {import('../types.d.js').HostedAccessGrant[]} */
  const grants=[];
  const identifiers=new Set();
  let cursor='';
  do {
    const page=await requestJSON(`${HOSTED_GRANTS_PATH}?limit=100${cursor?'&cursor='+encodeURIComponent(cursor):''}`,{method:'GET',signal});
    if (!page || !Array.isArray(page.hosted_access_grants) || typeof page.next_cursor!=='string') throw new Error(APP_INTEGRITY_ERROR);
    for (const grant of page.hosted_access_grants) {
      assertHostedAccessGrant(grant);
      if (identifiers.has(grant.id)) throw new Error(APP_INTEGRITY_ERROR);
      identifiers.add(grant.id);
      grants.push(grant);
    }
    if (page.next_cursor && (page.next_cursor===cursor || page.next_cursor!==grants.at(-1)?.id)) throw new Error(APP_INTEGRITY_ERROR);
    cursor=page.next_cursor;
  } while(cursor);
  return grants;
}

/** @param {string} tenantID @param {AbortSignal} [signal] @returns {Promise<import('../types.d.js').ProviderAssignment[]>} */
export async function fetchProviderAssignments(tenantID, signal) {
  const result=await requestJSON(`${managementTenantPath(tenantID)}/connections`,{method:'GET',signal});
  if (!result || !Array.isArray(result.assignments)) throw new Error(APP_INTEGRITY_ERROR);
  const providers=new Set();
  for(const assignment of result.assignments) {
    if (!assignment || typeof assignment.provider!=='string' || !assignment.provider || providers.has(assignment.provider) ||
        !['account_connection','hosted_access_grant'].includes(assignment.kind) || typeof assignment.resource_id!=='string' || !assignment.resource_id) throw new Error(APP_INTEGRITY_ERROR);
    providers.add(assignment.provider);
  }
  return result.assignments;
}

/** @param {string} tenantID @param {import('../types.d.js').HostedAccessGrant} grant @param {AbortSignal} [signal] */
export function assignHostedAccess(tenantID, grant, signal) {
  return requestTenantProfile(`${managementTenantPath(tenantID)}/connections/${encodeURIComponent(grant.provider)}`,{method:'PUT',body:{kind:'hosted_access_grant',resource_id:grant.id},signal},tenantID);
}

/** @param {string} tenantID @param {string} provider @param {boolean} clearDefaults @param {AbortSignal} [signal] @returns {Promise<void>} */
export function detachConnection(tenantID, provider, clearDefaults, signal) {
  return requestJSON(`${managementTenantPath(tenantID)}/connections/${encodeURIComponent(provider)}?clear_defaults=${clearDefaults}`, {method:'DELETE',signal});
}

/** @param {string} tenantID @param {string} provider @param {{text_model:string, system_prompt:string}} body @param {AbortSignal} [signal] @returns {Promise<import('../types.d.js').ManagementTenantProfile>} */
export function saveTenantProviderProfile(tenantID, provider, body, signal) {
  return requestTenantProfile(`${managementTenantPath(tenantID)}/provider-profiles/${encodeURIComponent(provider)}`, {method:'PUT',body,signal}, tenantID);
}

/** @param {AbortSignal} [signal] @returns {Promise<{families:Record<string, string>, offerings:import('../types.d.js').DashboardOffering[]}>} */
export async function fetchDashboardModels(signal) {
  const config = await loadFrontendRuntimeConfig();
  const response = await fetch(`${config.managementApiOrigin}/api/public/capabilities`, {signal, credentials:'omit'});
  if (!response.ok) throw new BackendClientError(await response.text(), response.status);
  const result = await response.json();
  if (!result || !Array.isArray(result.models) || !Array.isArray(result.offerings)) throw new Error('Invalid model catalog');
  /** @type {Record<string, string>} */
  const families = {};
  for (const model of result.models) {
    if (!model || typeof model.identifier !== 'string' || typeof model.family !== 'string' || !model.identifier || !model.family || Object.hasOwn(families, model.identifier)) throw new Error('Invalid model identity');
    families[model.identifier] = model.family;
  }
  for (const offering of result.offerings) {
    if (!offering || typeof offering.provider !== 'string' || !offering.provider || typeof offering.model !== 'string' || !Object.hasOwn(families, offering.model) || !Array.isArray(offering.capabilities) || !offering.capabilities.every((/** @type {unknown} */ value)=>typeof value==='string')) throw new Error('Invalid provider offering');
    if (!Array.isArray(offering.domains) || offering.domains.length === 0 || new Set(offering.domains).size !== offering.domains.length || !offering.domains.every((/** @type {unknown} */ value)=>Object.values(CAPABILITY_DOMAINS).some(domain=>domain===value))) throw new Error('Invalid provider offering domains');
  }
  return {families, offerings:result.offerings};
}

/** @param {string} accountID @param {string} requestID */
function journalEvidencePath(accountID, requestID) {
  if (!/^request-[a-f0-9]{32}$/.test(requestID)) throw new Error(APP_INTEGRITY_ERROR);
  return `${journalRequestsPath(accountID)}/${encodeURIComponent(requestID)}`;
}
/** @param {unknown} value */
function journalTimestamp(value) { return typeof value==='string' && Number.isFinite(Date.parse(value)); }
/**
 * @template {{id:string}} T
 * @param {T[]} records @param {string} next @param {string} cursor @param {RegExp} pattern @param {(record:T)=>void} validate
 */
function assertJournalPage(records, next, cursor, pattern, validate) {
  if (!Array.isArray(records) || records.length>50 || typeof next!=='string') throw new Error(APP_INTEGRITY_ERROR);
  let previous=cursor;
  for (const record of records) {
    if (!record || typeof record.id!=='string' || !pattern.test(record.id) || record.id<=previous) throw new Error(APP_INTEGRITY_ERROR);
    validate(record); previous=record.id;
  }
  if (next && (!records.length || next!==previous)) throw new Error(APP_INTEGRITY_ERROR);
}
/** @param {import('../types.d.js').JournalAttempt} record */
function assertJournalAttempt(record) {
  if (!Number.isSafeInteger(record.number) || record.number<1 || !['prepared','dispatched','observed','uncertain'].includes(record.state) ||
      !journalTimestamp(record.created_at) || !journalTimestamp(record.updated_at) ||
      (record.dispatched_at!==undefined && !journalTimestamp(record.dispatched_at)) || (record.observed_at!==undefined && !journalTimestamp(record.observed_at))) throw new Error(APP_INTEGRITY_ERROR);
}
/** @param {import('../types.d.js').JournalObservation} record */
function assertJournalObservation(record) {
  if (!/^attempt-[a-f0-9]{32}$/.test(record.attempt_id) || !Array.isArray(record.quantities) || !record.quantities.length ||
      !['complete','unknown'].includes(record.completeness) || !['continue','complete','fail'].includes(record.outcome) ||
      !journalTimestamp(record.created_at) || !journalTimestamp(record.observed_at) || (record.failure_code!==undefined && typeof record.failure_code!=='string')) throw new Error(APP_INTEGRITY_ERROR);
  const dimensions=new Set();
  for (const quantity of record.quantities) {
    if (!quantity || !/^[a-z][a-z0-9_]{0,63}$/.test(quantity.dimension) || !/^[a-z][a-z0-9_]{0,63}$/.test(quantity.unit) || dimensions.has(quantity.dimension) ||
        (quantity.included_in!==undefined && !/^[a-z][a-z0-9_]{0,63}$/.test(quantity.included_in)) ||
        (quantity.value===undefined ? !['not_reported','invalid_quantity','unsupported_meter'].includes(quantity.unknown_reason || '') : typeof quantity.value!=='string' || !/^(0|[1-9][0-9]*)(\.[0-9]+)?$/.test(quantity.value) || quantity.unknown_reason!==undefined)) throw new Error(APP_INTEGRITY_ERROR);
    dimensions.add(quantity.dimension);
  }
  for (const quantity of record.quantities) {
    const seen=new Set([quantity.dimension]);
    let parent=quantity.included_in;
    while (parent!==undefined) {
      const inclusive=record.quantities.find(value=>value.dimension===parent);
      if (!inclusive || inclusive.unit!==quantity.unit || seen.has(parent)) throw new Error(APP_INTEGRITY_ERROR);
      seen.add(parent); parent=inclusive.included_in;
    }
  }
  if ((record.completeness==='unknown')!==record.quantities.some(quantity=>quantity.unknown_reason!==undefined)) throw new Error(APP_INTEGRITY_ERROR);
}
/** @param {import('../types.d.js').JournalCase} record */
function assertJournalCase(record) {
  if (!['usage_unknown','dispatch_outcome_unknown','execution_outcome_unknown','execution_result_unknown'].includes(record.reason) || !['open','resolved'].includes(record.state) ||
      !journalTimestamp(record.created_at) || (record.state==='resolved' ? !journalTimestamp(record.resolved_at) : record.resolved_at!==undefined)) throw new Error(APP_INTEGRITY_ERROR);
}
/** @param {string} accountID @param {string} requestID @param {string} [cursor] @param {AbortSignal} [signal] @returns {Promise<import('../types.d.js').JournalAttemptPage>} */
export async function fetchJournalAttempts(accountID,requestID,cursor='',signal) {
  const pattern=/^attempt-[a-f0-9]{32}$/;
  if (cursor && !pattern.test(cursor)) throw new Error(APP_INTEGRITY_ERROR);
  const page=await requestJSON(`${journalEvidencePath(accountID,requestID)}/attempts?limit=50${cursor?'&cursor='+encodeURIComponent(cursor):''}`,{method:'GET',signal});
  if (!page) throw new Error(APP_INTEGRITY_ERROR);
  assertJournalPage(page.attempts,page.next_cursor,cursor,pattern,assertJournalAttempt);
  return page;
}
/** @param {string} accountID @param {string} requestID @param {string} [cursor] @param {AbortSignal} [signal] @returns {Promise<import('../types.d.js').JournalObservationPage>} */
export async function fetchJournalObservations(accountID,requestID,cursor='',signal) {
  const pattern=/^observation-[a-f0-9]{32}$/;
  if (cursor && !pattern.test(cursor)) throw new Error(APP_INTEGRITY_ERROR);
  const page=await requestJSON(`${journalEvidencePath(accountID,requestID)}/observations?limit=50${cursor?'&cursor='+encodeURIComponent(cursor):''}`,{method:'GET',signal});
  if (!page) throw new Error(APP_INTEGRITY_ERROR);
  assertJournalPage(page.observations,page.next_cursor,cursor,pattern,assertJournalObservation);
  return page;
}
/** @param {string} accountID @param {string} requestID @param {string} [cursor] @param {AbortSignal} [signal] @returns {Promise<import('../types.d.js').JournalCasePage>} */
export async function fetchJournalCases(accountID,requestID,cursor='',signal) {
  const pattern=/^case-[a-f0-9]{32}$/;
  if (cursor && !pattern.test(cursor)) throw new Error(APP_INTEGRITY_ERROR);
  const page=await requestJSON(`${journalEvidencePath(accountID,requestID)}/reconciliation-cases?limit=50${cursor?'&cursor='+encodeURIComponent(cursor):''}`,{method:'GET',signal});
  if (!page) throw new Error(APP_INTEGRITY_ERROR);
  assertJournalPage(page.cases,page.next_cursor,cursor,pattern,assertJournalCase);
  return page;
}
