// @ts-check

import { DEFAULT_USAGE_INTERVAL, USAGE_INTERVALS } from "../constants.js";

const EMPTY_STRING = "";
const CHART_WIDTH = 640;
const CHART_HEIGHT = 168;
const CHART_PADDING = 12;
const PERCENT_SCALE = 100;

export const USAGE_CHART = Object.freeze({
  width: CHART_WIDTH,
  height: CHART_HEIGHT,
});

export const USAGE_METRICS = Object.freeze({
  REQUESTS: "requests",
  TOTAL_TOKENS: "total_tokens",
});

/**
 * @param {import("../types.d.js").UsageInterval} [interval]
 * @returns {import("../types.d.js").ManagementUsageSummary}
 */
export function emptyUsageSummary(interval = DEFAULT_USAGE_INTERVAL) {
  const intervalDefinition = USAGE_INTERVALS.find((candidate) => candidate.id === interval);
  if (!intervalDefinition) {
    throw new Error(`usage_interval_invalid:${interval}`);
  }
  return {
    interval,
    bucket_unit: intervalDefinition.bucketUnit,
    totals: emptyUsageAggregate(),
    buckets: [],
    providers: [],
    models: [],
    status_codes: [],
  };
}

/**
 * @returns {import("../types.d.js").UsageAggregate}
 */
function emptyUsageAggregate() {
  return {
    requests: 0,
    successful_requests: 0,
    failed_requests: 0,
    text_requests: 0,
    dictation_requests: 0,
    request_tokens: 0,
    response_tokens: 0,
    total_tokens: 0,
    average_latency_ms: 0,
  };
}

/**
 * @param {import("../types.d.js").ManagementUsageSummary | null} usage
 * @param {string} metric
 * @returns {string}
 */
export function usagePolyline(usage, metric) {
  const buckets = usage ? usage.buckets : [];
  if (buckets.length === 0) {
    return EMPTY_STRING;
  }
  const maxValue = Math.max(1, ...buckets.map((bucket) => usageMetric(bucket.data, metric)));
  const pointSpacing = buckets.length > 1 ? (CHART_WIDTH - CHART_PADDING * 2) / (buckets.length - 1) : 0;
  return buckets
    .map((bucket, bucketIndex) => {
      const x = CHART_PADDING + bucketIndex * pointSpacing;
      const valueRatio = usageMetric(bucket.data, metric) / maxValue;
      const y = CHART_HEIGHT - CHART_PADDING - valueRatio * (CHART_HEIGHT - CHART_PADDING * 2);
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(" ");
}

/**
 * @param {import("../types.d.js").ManagementUsageSummary | null} usage
 * @returns {Array<{ label: string, requests: number, width: string }>}
 */
export function providerRows(usage) {
  const providers = usage ? usage.providers : [];
  const maxRequests = Math.max(1, ...providers.map((provider) => provider.data.requests));
  return providers.map((provider) => ({
    label: provider.provider,
    requests: provider.data.requests,
    width: `${Math.max(1, Math.round((provider.data.requests / maxRequests) * PERCENT_SCALE))}%`,
  }));
}

/**
 * @param {import("../types.d.js").ManagementUsageSummary | null} usage
 * @returns {Array<{ label: string, requests: number, width: string }>}
 */
export function modelRows(usage) {
  const models = usage ? usage.models : [];
  const maxRequests = Math.max(1, ...models.map((model) => model.data.requests));
  return models.map((model) => ({
    label: `${model.provider} / ${model.model}`,
    requests: model.data.requests,
    width: `${Math.max(1, Math.round((model.data.requests / maxRequests) * PERCENT_SCALE))}%`,
  }));
}

/**
 * @param {import("../types.d.js").UsageAggregate} aggregate
 * @returns {string}
 */
export function successRateLabel(aggregate) {
  if (aggregate.requests === 0) {
    return "0%";
  }
  return `${Math.round((aggregate.successful_requests / aggregate.requests) * PERCENT_SCALE)}%`;
}

/**
 * @param {import("../types.d.js").UsageAggregate} aggregate
 * @param {string} metric
 * @returns {number}
 */
function usageMetric(aggregate, metric) {
  if (metric === USAGE_METRICS.TOTAL_TOKENS) {
    return aggregate.total_tokens;
  }
  return aggregate.requests;
}
