// @ts-check

import {
  APP_INTEGRITY_ERROR,
  COPY,
  USAGE_ENDPOINT_LABELS,
  USAGE_OUTCOME_LABELS,
  USAGE_STATUS_LABELS,
} from "../constants.js?v=20260902c237";

const EMPTY_STRING = "";

/**
 * @param {import("../types.d.js").ManagementUsageFailurePage | import("../types.d.js").ManagementAccountUsageFailurePage} response
 * @param {import("../types.d.js").UsageInterval} interval
 * @param {boolean} accountScope
 * @returns {import("../types.d.js").ManagementUsageFailurePage | import("../types.d.js").ManagementAccountUsageFailurePage}
 */
export function normalizedUsageFailurePage(response, interval, accountScope) {
  if (!response || response.interval !== interval || !Array.isArray(response.failures)) {
    throw new Error(APP_INTEGRITY_ERROR);
  }
  if (response.next_cursor !== undefined && (typeof response.next_cursor !== "string" || !response.next_cursor)) {
    throw new Error(APP_INTEGRITY_ERROR);
  }
  return {
    interval,
    failures: response.failures.map((failure) => normalizedUsageFailure(failure, accountScope)),
    ...(response.next_cursor ? { next_cursor: response.next_cursor } : {}),
  };
}

/**
 * @param {import("../types.d.js").ManagementUsageFailure | import("../types.d.js").ManagementAccountUsageFailure} failure
 * @returns {{
 *   tenant: string,
 *   occurredAt: string,
 *   endpoint: string,
 *   provider: string,
 *   model: string,
 *   status: string,
 *   outcome: string,
 *   latency: string
 * }}
 */
export function usageFailurePresentation(failure) {
  return {
    tenant: "tenant_name" in failure ? `${failure.tenant_name} · ${failure.tenant_id}` : EMPTY_STRING,
    occurredAt: new Intl.DateTimeFormat("en-US", {
      dateStyle: "medium",
      timeStyle: "medium",
      timeZone: "UTC",
    }).format(new Date(failure.occurred_at)),
    endpoint: usageLabel(USAGE_ENDPOINT_LABELS, failure.endpoint),
    provider: failure.provider || COPY.usageFailuresNotResolved,
    model: failure.model || COPY.usageFailuresNotResolved,
    status: `${failure.status_code} ${usageStatusLabel(failure.status_code)}`,
    outcome: usageLabel(USAGE_OUTCOME_LABELS, failure.outcome_code),
    latency: `${formatNumber(failure.latency_ms)} ms`,
  };
}

/**
 * @param {number} statusCode
 * @returns {string}
 */
export function usageStatusLabel(statusCode) {
  return usageLabel(USAGE_STATUS_LABELS, String(statusCode));
}

/**
 * @param {number} value
 * @returns {string}
 */
export function formatNumber(value) {
  return Number(value || 0).toLocaleString("en-US");
}

/**
 * @param {import("../types.d.js").ManagementUsageFailure | import("../types.d.js").ManagementAccountUsageFailure} failure
 * @param {boolean} accountScope
 * @returns {import("../types.d.js").ManagementUsageFailure | import("../types.d.js").ManagementAccountUsageFailure}
 */
function normalizedUsageFailure(failure, accountScope) {
  const occurredAt = new Date(failure ? failure.occurred_at : EMPTY_STRING);
  if (
    !failure ||
    typeof failure.occurred_at !== "string" ||
    Number.isNaN(occurredAt.valueOf()) ||
    typeof failure.endpoint !== "string" ||
    !hasLabel(USAGE_ENDPOINT_LABELS, failure.endpoint) ||
    typeof failure.provider !== "string" ||
    typeof failure.model !== "string" ||
    !Number.isInteger(failure.status_code) ||
    !hasLabel(USAGE_STATUS_LABELS, String(failure.status_code)) ||
    typeof failure.outcome_code !== "string" ||
    failure.outcome_code === "success" ||
    !hasLabel(USAGE_OUTCOME_LABELS, failure.outcome_code) ||
    !Number.isInteger(failure.latency_ms) ||
    failure.latency_ms < 0
  ) {
    throw new Error(APP_INTEGRITY_ERROR);
  }
  const commonFailure = {
    occurred_at: failure.occurred_at,
    endpoint: failure.endpoint,
    provider: failure.provider,
    model: failure.model,
    status_code: failure.status_code,
    outcome_code: failure.outcome_code,
    latency_ms: failure.latency_ms,
  };
  if (accountScope) {
    if (!("tenant_id" in failure) || !("tenant_name" in failure)) {
      throw new Error(APP_INTEGRITY_ERROR);
    }
    if (
      typeof failure.tenant_id !== "string" ||
      !failure.tenant_id ||
      typeof failure.tenant_name !== "string" ||
      !failure.tenant_name
    ) {
      throw new Error(APP_INTEGRITY_ERROR);
    }
    return {
      tenant_id: failure.tenant_id,
      tenant_name: failure.tenant_name,
      ...commonFailure,
    };
  }
  if (Object.hasOwn(failure, "tenant_id") || Object.hasOwn(failure, "tenant_name")) {
    throw new Error(APP_INTEGRITY_ERROR);
  }
  return commonFailure;
}

/**
 * @param {Readonly<Record<string, string>>} labels
 * @param {string} value
 * @returns {boolean}
 */
function hasLabel(labels, value) {
  return Object.prototype.hasOwnProperty.call(labels, value);
}

/**
 * @param {Readonly<Record<string, string>>} labels
 * @param {string} value
 * @returns {string}
 */
function usageLabel(labels, value) {
  if (!hasLabel(labels, value)) {
    throw new Error(APP_INTEGRITY_ERROR);
  }
  return labels[value];
}
