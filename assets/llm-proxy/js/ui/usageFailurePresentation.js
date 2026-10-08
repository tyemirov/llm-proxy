// @ts-check

import {
  APP_INTEGRITY_ERROR,
  COPY,
  USAGE_ENDPOINT_LABELS,
  USAGE_OUTCOME_LABELS,
  USAGE_STATUS_LABELS,
} from "../constants.js?v=20260903f037";

const EMPTY_STRING = "";
const FAILURE_OUTCOMES = new Set(["rate_limited", "service_unavailable", "request_timeout", "upstream_error", "proxy_error"]);
const REJECTION_OUTCOMES = new Set(["invalid_request", "payload_too_large", "provider_not_configured"]);

/**
 * @param {import("../types.d.js").ManagementUsageFailurePage | import("../types.d.js").ManagementAccountUsageFailurePage | import("../types.d.js").ManagementUsageRejectionPage | import("../types.d.js").ManagementAccountUsageRejectionPage} response
 * @param {import("../types.d.js").UsageInterval} interval
 * @param {boolean} accountScope
 * @param {import("../types.d.js").UsageDetailKind} kind
 * @returns {{ items: Array<import("../types.d.js").ManagementUsageFailure | import("../types.d.js").ManagementAccountUsageFailure | import("../types.d.js").ManagementUsageRejection | import("../types.d.js").ManagementAccountUsageRejection>, next_cursor?: string }}
 */
export function normalizedUsageDetailPage(response, interval, accountScope, kind) {
  /** @type {Array<import("../types.d.js").ManagementUsageFailure | import("../types.d.js").ManagementAccountUsageFailure | import("../types.d.js").ManagementUsageRejection | import("../types.d.js").ManagementAccountUsageRejection> | null} */
  let items = null;
  if (response && kind === "failures" && "failures" in response) items = response.failures;
  if (response && kind === "rejections" && "rejections" in response) items = response.rejections;
  if (!response || response.interval !== interval || !Array.isArray(items)) {
    throw new Error(APP_INTEGRITY_ERROR);
  }
  if (response.next_cursor !== undefined && (typeof response.next_cursor !== "string" || !response.next_cursor)) {
    throw new Error(APP_INTEGRITY_ERROR);
  }
  return {
    items: items.map((detail) => normalizedUsageDetail(detail, accountScope, kind)),
    ...(response.next_cursor ? { next_cursor: response.next_cursor } : {}),
  };
}

/**
 * @param {import("../types.d.js").ManagementUsageFailure | import("../types.d.js").ManagementAccountUsageFailure | import("../types.d.js").ManagementUsageRejection | import("../types.d.js").ManagementAccountUsageRejection} detail
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
export function usageDetailPresentation(detail) {
  return {
    tenant: "tenant_name" in detail ? `${detail.tenant_name} · ${detail.tenant_id}` : EMPTY_STRING,
    occurredAt: new Intl.DateTimeFormat("en-US", {
      dateStyle: "medium",
      timeStyle: "medium",
      timeZone: "UTC",
    }).format(new Date(detail.occurred_at)),
    endpoint: usageLabel(USAGE_ENDPOINT_LABELS, detail.endpoint),
    provider: detail.provider || COPY.usageDetailsNotResolved,
    model: detail.model || COPY.usageDetailsNotResolved,
    status: `${detail.status_code} ${usageStatusLabel(detail.status_code)}`,
    outcome: usageLabel(USAGE_OUTCOME_LABELS, detail.outcome_code),
    latency: `${formatNumber(detail.latency_ms)} ms`,
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
 * @param {import("../types.d.js").ManagementUsageFailure | import("../types.d.js").ManagementAccountUsageFailure | import("../types.d.js").ManagementUsageRejection | import("../types.d.js").ManagementAccountUsageRejection} detail
 * @param {boolean} accountScope
 * @param {import("../types.d.js").UsageDetailKind} kind
 * @returns {import("../types.d.js").ManagementUsageFailure | import("../types.d.js").ManagementAccountUsageFailure | import("../types.d.js").ManagementUsageRejection | import("../types.d.js").ManagementAccountUsageRejection}
 */
function normalizedUsageDetail(detail, accountScope, kind) {
  const occurredAt = new Date(detail ? detail.occurred_at : EMPTY_STRING);
  const allowedOutcomes = kind === "failures" ? FAILURE_OUTCOMES : kind === "rejections" ? REJECTION_OUTCOMES : null;
  if (
    !detail ||
    !allowedOutcomes ||
    typeof detail.occurred_at !== "string" ||
    Number.isNaN(occurredAt.valueOf()) ||
    typeof detail.endpoint !== "string" ||
    !hasLabel(USAGE_ENDPOINT_LABELS, detail.endpoint) ||
    typeof detail.provider !== "string" ||
    typeof detail.model !== "string" ||
    !Number.isInteger(detail.status_code) ||
    !hasLabel(USAGE_STATUS_LABELS, String(detail.status_code)) ||
    typeof detail.outcome_code !== "string" ||
    !allowedOutcomes.has(detail.outcome_code) ||
    !hasLabel(USAGE_OUTCOME_LABELS, detail.outcome_code) ||
    !Number.isInteger(detail.latency_ms) ||
    detail.latency_ms < 0
  ) {
    throw new Error(APP_INTEGRITY_ERROR);
  }
  const commonDetail = {
    occurred_at: detail.occurred_at,
    endpoint: detail.endpoint,
    provider: detail.provider,
    model: detail.model,
    status_code: detail.status_code,
    outcome_code: detail.outcome_code,
    latency_ms: detail.latency_ms,
  };
  if (accountScope) {
    if (!("tenant_id" in detail) || !("tenant_name" in detail)) {
      throw new Error(APP_INTEGRITY_ERROR);
    }
    if (
      typeof detail.tenant_id !== "string" ||
      !detail.tenant_id ||
      typeof detail.tenant_name !== "string" ||
      !detail.tenant_name
    ) {
      throw new Error(APP_INTEGRITY_ERROR);
    }
    return {
      tenant_id: detail.tenant_id,
      tenant_name: detail.tenant_name,
      ...commonDetail,
    };
  }
  if (Object.hasOwn(detail, "tenant_id") || Object.hasOwn(detail, "tenant_name")) {
    throw new Error(APP_INTEGRITY_ERROR);
  }
  return /** @type {import("../types.d.js").ManagementUsageFailure | import("../types.d.js").ManagementUsageRejection} */ (commonDetail);
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
