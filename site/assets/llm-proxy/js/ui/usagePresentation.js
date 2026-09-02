// @ts-check

import { COPY, DEFAULT_USAGE_INTERVAL, USAGE_INTERVALS } from "../constants.js?v=20260811c131";

const CHART_WIDTH = 640;
const CHART_HEIGHT = 240;
const CHART_LEFT = 68;
const CHART_RIGHT = 18;
const CHART_TOP = 14;
const CHART_BOTTOM = 52;
const CHART_MAXIMUM_X_TICKS = 5;
const CHART_TARGET_Y_INTERVALS = 4;
const PERCENT_SCALE = 100;
const SVG_NAMESPACE = "http://www.w3.org/2000/svg";
const DONUT_PALETTE = Object.freeze([
  "#5d93ff",
  "#66d19e",
  "#f5bd4f",
  "#cb8cff",
  "#ff8177",
  "#58c7d9",
  "#d7dd68",
  "#f28fc5",
]);

export const USAGE_CHART = Object.freeze({
  width: CHART_WIDTH,
  height: CHART_HEIGHT,
});

export const USAGE_BREAKDOWN_VIEWS = Object.freeze({
  BAR: "bar",
  DONUT: "donut",
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

/** @returns {import("../types.d.js").UsageAggregate} */
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
 * @returns {import("../types.d.js").UsageDistribution}
 */
export function providerDistribution(usage) {
  return usageDistribution((usage ? usage.providers : []).map((provider) => ({
    label: provider.provider,
    requests: provider.data.requests,
  })));
}

/**
 * @param {import("../types.d.js").ManagementUsageSummary | null} usage
 * @returns {import("../types.d.js").UsageDistribution}
 */
export function modelDistribution(usage) {
  return usageDistribution((usage ? usage.models : []).map((model) => ({
    label: `${model.provider} / ${model.model}`,
    requests: model.data.requests,
  })));
}

/**
 * Convert one canonical, ordered request breakdown into bar and donut presentation data.
 * @param {Array<{ label: string, requests: number }>} sourceRows
 * @returns {import("../types.d.js").UsageDistribution}
 */
export function usageDistribution(sourceRows) {
  const totalRequests = sourceRows.reduce((total, row) => total + row.requests, 0);
  const maximumRequests = Math.max(1, ...sourceRows.map((row) => row.requests));
  const roundedPercentages = roundedShares(sourceRows.map((row) => row.requests), totalRequests);
  let consumedShare = 0;
  const rows = sourceRows.map((row, rowIndex) => {
    const exactShare = totalRequests === 0 ? 0 : (row.requests / totalRequests) * PERCENT_SCALE;
    const presentationRow = {
      label: row.label,
      requests: row.requests,
      width: `${Math.max(1, Math.round((row.requests / maximumRequests) * PERCENT_SCALE))}%`,
      percentage: roundedPercentages[rowIndex],
      percentageLabel: `${roundedPercentages[rowIndex]}%`,
      color: DONUT_PALETTE[rowIndex % DONUT_PALETTE.length],
      dashArray: `${exactShare} ${PERCENT_SCALE - exactShare}`,
      dashOffset: -consumedShare,
    };
    consumedShare += exactShare;
    return presentationRow;
  });
  return { totalRequests, rows };
}

/** @param {number[]} values @param {number} total @returns {number[]} */
function roundedShares(values, total) {
  if (total === 0) {
    return values.map(() => 0);
  }
  const exactShares = values.map((value) => (value / total) * PERCENT_SCALE);
  const shares = exactShares.map(Math.floor);
  const remaining = PERCENT_SCALE - shares.reduce((sum, share) => sum + share, 0);
  const remainderOrder = exactShares
    .map((share, index) => ({ index, remainder: share - Math.floor(share) }))
    .sort((left, right) => right.remainder - left.remainder || left.index - right.index);
  for (let index = 0; index < remaining; index += 1) {
    shares[remainderOrder[index].index] += 1;
  }
  return shares;
}

/**
 * @param {import("../types.d.js").ManagementUsageSummary} usage
 * @param {string} metric
 * @returns {import("../types.d.js").UsageTimeSeriesChart}
 */
export function usageTimeSeriesChart(usage, metric) {
  const values = usage.buckets.map((bucket) => usageMetric(bucket.data, metric));
  const maximumValue = Math.max(0, ...values);
  const yTickValues = chartYTickValues(maximumValue);
  const scaleMaximum = yTickValues.at(-1) || 1;
  const plotWidth = CHART_WIDTH - CHART_LEFT - CHART_RIGHT;
  const plotHeight = CHART_HEIGHT - CHART_TOP - CHART_BOTTOM;
  const xAxisY = CHART_TOP + plotHeight;
  const points = usage.buckets.map((bucket, bucketIndex) => {
    const x = usage.buckets.length > 1
      ? CHART_LEFT + (bucketIndex / (usage.buckets.length - 1)) * plotWidth
      : CHART_LEFT + plotWidth / 2;
    const value = values[bucketIndex];
    const y = xAxisY - (value / scaleMaximum) * plotHeight;
    return {
      x,
      y,
      start: bucket.start,
      value,
      accessibleLabel: `${bucket.start}: ${formatExactInteger(value)} ${metric === USAGE_METRICS.TOTAL_TOKENS ? COPY.usageChartTokensValue : COPY.usageBreakdownRequests}`,
    };
  });
  const xTicks = selectedXTickIndexes(usage.buckets.length).map((bucketIndex) => ({
    x: points[bucketIndex].x,
    label: utcTickLabel(usage.buckets[bucketIndex].start, usage.bucket_unit),
    exactLabel: usage.buckets[bucketIndex].start,
  }));
  const yTicks = yTickValues.map((value) => ({
    y: xAxisY - (value / scaleMaximum) * plotHeight,
    label: compactInteger(value),
    exactValue: formatExactInteger(value),
  }));
  const metricName = metric === USAGE_METRICS.TOTAL_TOKENS ? COPY.usageTokens : COPY.usageRequests;
  const yAxisTitle = usageYAxisTitle(metric, usage.bucket_unit);
  return {
    viewBox: `0 0 ${CHART_WIDTH} ${CHART_HEIGHT}`,
    polyline: points.map((point) => `${point.x.toFixed(1)},${point.y.toFixed(1)}`).join(" "),
    xAxisY,
    yAxisX: CHART_LEFT,
    xTicks,
    yTicks,
    points,
    xAxisTitle: COPY.usageTimeAxis,
    yAxisTitle,
    accessibleLabel: `${metricName} by ${usage.bucket_unit}. ${COPY.usageTimeAxis}. ${yAxisTitle}.`,
  };
}

/**
 * Render deterministic chart ticks and source points without parsing markup.
 * @param {SVGElement} target
 * @param {import("../types.d.js").UsageTimeSeriesChart} chart
 */
export function renderUsageChartPlot(target, chart) {
  const fragment = document.createDocumentFragment();
  for (const tick of chart.yTicks) {
    const group = svgElement("g", "usage-y-tick");
    group.append(
      svgLine({ x1: 63, y1: tick.y, x2: 68, y2: tick.y }),
      svgText({ x: 59, y: tick.y + 4 }, tick.label, tick.exactValue, "end"),
    );
    fragment.append(group);
  }
  for (const tick of chart.xTicks) {
    const group = svgElement("g", "usage-x-tick");
    group.append(
      svgLine({ x1: tick.x, y1: chart.xAxisY, x2: tick.x, y2: chart.xAxisY + 5 }),
      svgText({ x: tick.x, y: chart.xAxisY + 17 }, tick.label, tick.exactLabel, "middle"),
    );
    fragment.append(group);
  }
  for (const point of chart.points) {
    const circle = svgElement("circle", "usage-chart-point");
    setSVGAttributes(circle, { cx: point.x, cy: point.y, r: 2.5 });
    fragment.append(circle);
  }
  target.replaceChildren(fragment);
}

/**
 * Render exact donut geometry from the ordered presentation rows.
 * @param {SVGElement} target
 * @param {import("../types.d.js").UsageDistributionRow[]} rows
 */
export function renderUsageDonutSegments(target, rows) {
  const fragment = document.createDocumentFragment();
  for (const row of rows) {
    const circle = svgElement("circle", "usage-donut-segment");
    setSVGAttributes(circle, {
      cx: 60,
      cy: 60,
      r: 42,
      pathLength: 100,
      stroke: row.color,
      "stroke-dasharray": row.dashArray,
      "stroke-dashoffset": row.dashOffset,
    });
    fragment.append(circle);
  }
  target.replaceChildren(fragment);
}

/** @param {string} name @param {string} className @returns {SVGElement} */
function svgElement(name, className) {
  const element = document.createElementNS(SVG_NAMESPACE, name);
  element.setAttribute("class", className);
  return element;
}

/** @param {Record<string, number>} attributes @returns {SVGElement} */
function svgLine(attributes) {
  const line = svgElement("line", "");
  setSVGAttributes(line, attributes);
  return line;
}

/**
 * @param {Record<string, number>} attributes
 * @param {string} label
 * @param {string} exactLabel
 * @param {string} anchor
 * @returns {SVGElement}
 */
function svgText(attributes, label, exactLabel, anchor) {
  const text = svgElement("text", "");
  setSVGAttributes(text, attributes);
  text.setAttribute("text-anchor", anchor);
  text.setAttribute("aria-label", exactLabel);
  text.textContent = label;
  return text;
}

/** @param {SVGElement} element @param {Record<string, string | number>} attributes */
function setSVGAttributes(element, attributes) {
  for (const [name, value] of Object.entries(attributes)) {
    element.setAttribute(name, String(value));
  }
}

/** @param {number} bucketCount @returns {number[]} */
function selectedXTickIndexes(bucketCount) {
  if (bucketCount === 0) {
    return [];
  }
  if (bucketCount <= CHART_MAXIMUM_X_TICKS) {
    return Array.from({ length: bucketCount }, (_, index) => index);
  }
  const indexes = new Set();
  for (let tickIndex = 0; tickIndex < CHART_MAXIMUM_X_TICKS; tickIndex += 1) {
    indexes.add(Math.round((tickIndex / (CHART_MAXIMUM_X_TICKS - 1)) * (bucketCount - 1)));
  }
  return [...indexes].sort((left, right) => left - right);
}

/** @param {number} maximumValue @returns {number[]} */
function chartYTickValues(maximumValue) {
  if (maximumValue === 0) {
    return [0];
  }
  const roughStep = maximumValue / CHART_TARGET_Y_INTERVALS;
  const magnitude = 10 ** Math.floor(Math.log10(roughStep));
  const normalizedStep = roughStep / magnitude;
  const niceMultiplier = normalizedStep <= 1 ? 1 : normalizedStep <= 2 ? 2 : normalizedStep <= 5 ? 5 : 10;
  const step = Math.max(1, niceMultiplier * magnitude);
  const scaleMaximum = Math.ceil(maximumValue / step) * step;
  const ticks = [];
  for (let value = 0; value <= scaleMaximum; value += step) {
    ticks.push(value);
  }
  return ticks;
}

/** @param {string} start @param {"day" | "hour"} bucketUnit @returns {string} */
function utcTickLabel(start, bucketUnit) {
  const instant = new Date(start);
  if (bucketUnit === "hour") {
    return `${String(instant.getUTCHours()).padStart(2, "0")}:00`;
  }
  return `${instant.getUTCFullYear()}-${String(instant.getUTCMonth() + 1).padStart(2, "0")}-${String(instant.getUTCDate()).padStart(2, "0")}`;
}

/** @param {number} value @returns {string} */
function compactInteger(value) {
  if (value < 1_000) {
    return String(value);
  }
  if (value < 1_000_000) {
    return `${trimDecimal(value / 1_000)}K`;
  }
  return `${trimDecimal(value / 1_000_000)}M`;
}

/** @param {number} value @returns {string} */
function trimDecimal(value) {
  return value.toFixed(value >= 10 || Number.isInteger(value) ? 0 : 1).replace(/\.0$/u, "");
}

/** @param {number} value @returns {string} */
function formatExactInteger(value) {
  return new Intl.NumberFormat("en-US", { maximumFractionDigits: 0, useGrouping: true }).format(value);
}

/** @param {string} metric @param {"day" | "hour"} bucketUnit @returns {string} */
function usageYAxisTitle(metric, bucketUnit) {
  if (metric === USAGE_METRICS.TOTAL_TOKENS) {
    return bucketUnit === "hour" ? COPY.usageTokensPerHour : COPY.usageTokensPerDay;
  }
  return bucketUnit === "hour" ? COPY.usageRequestsPerHour : COPY.usageRequestsPerDay;
}

/** @param {import("../types.d.js").UsageAggregate} aggregate @returns {string} */
export function successRateLabel(aggregate) {
  if (aggregate.requests === 0) {
    return "0%";
  }
  return `${Math.round((aggregate.successful_requests / aggregate.requests) * PERCENT_SCALE)}%`;
}

/** @param {import("../types.d.js").UsageAggregate} aggregate @param {string} metric @returns {number} */
function usageMetric(aggregate, metric) {
  return metric === USAGE_METRICS.TOTAL_TOKENS ? aggregate.total_tokens : aggregate.requests;
}
