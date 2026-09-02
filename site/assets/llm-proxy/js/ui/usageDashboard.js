// @ts-check

import {
  APP_INTEGRITY_ERROR,
  AUTH_STATES,
  COPY,
  DASHBOARD_VIEWS,
  NOTICE_KINDS,
  USAGE_FAILURE_PAGE_LIMIT,
} from "../constants.js?v=20260902c239";
import {
  fetchAccountUsageFailures,
  fetchAccountUsageSummary,
  fetchTenant,
  fetchUsageFailures,
  fetchUsageSummary,
} from "../core/backendClient.js?v=20260902c239";
import { assertManagementTenantProfile, isAbortError } from "../core/managementProfile.js?v=20260902c239";
import { trapDialogFocus } from "./dialogFocus.js?v=20260902c239";
import {
  formatNumber,
  normalizedUsageFailurePage,
  usageFailurePresentation,
  usageStatusLabel,
} from "./usageFailurePresentation.js?v=20260902c239";
import {
  emptyUsageSummary,
  modelDistribution,
  providerDistribution,
  renderUsageChartPlot,
  renderUsageDonutSegments,
  successRateLabel,
  usageTimeSeriesChart,
  USAGE_BREAKDOWN_VIEWS,
  USAGE_METRICS,
} from "./usagePresentation.js?v=20260902c239";

const EMPTY_STRING = "";
const HTTP_ERROR_STATUS_MINIMUM = 400;

/** @typedef {ReturnType<typeof import("./managementApplicationState.js").createManagementApplicationState>} ManagementApplicationState */
/** @typedef {ManagementApplicationState & import("../types.d.js").AlpineMagic & {
 *   hasUsageFailures: boolean,
 *   hasLoadedUsageFailures: boolean,
 *   refreshAdminUsers: () => Promise<void>,
 *   resetProviderCard: () => void,
 *   setPageNotice: (kind: string, message: string) => void
 * }} UsageDashboardHost */

/**
 * @template {object} Responsibility
 * @param {Responsibility & ThisType<UsageDashboardHost & Responsibility>} responsibility
 * @returns {Responsibility}
 */
function usageDashboardResponsibility(responsibility) {
  return responsibility;
}

/** Create account/tenant usage summary and failure-detail behavior. */
export function createUsageDashboardResponsibility() {
  return usageDashboardResponsibility({
    get usageScopeIsAllTenants() {
      return this.selectedUsageTenantID === EMPTY_STRING;
    },

    get usageControlsDisabled() {
      return this.busy || this.usageLoading;
    },

    get hasUsage() {
      return this.usage.totals.requests > 0;
    },

    get usageTotals() {
      return this.usage.totals;
    },

    get usageTotalRequests() {
      return formatNumber(this.usage.totals.requests);
    },

    get usageTotalTokens() {
      return formatNumber(this.usage.totals.total_tokens);
    },

    get usageSuccessRate() {
      return successRateLabel(this.usage.totals);
    },

    get usageProviderCount() {
      return formatNumber(this.usage.providers.filter((/** @type {{ provider: string, data: import("../types.d.js").UsageAggregate }} */ provider) => provider.data.requests > 0).length);
    },

    get hasUsageFailures() {
      return this.usage.totals.failed_requests > 0;
    },

    get usageFailuresActionCopy() {
      const failureCount = this.usage.totals.failed_requests;
      const noun = failureCount === 1 ? "failed request" : "failed requests";
      return `${formatNumber(failureCount)} ${noun}`;
    },

    get usageFailuresIntervalLabel() {
      const interval = this.usageIntervals.find((/** @type {{ id: import("../types.d.js").UsageInterval }} */ candidate) => candidate.id === this.selectedUsageInterval);
      if (!interval) {
        throw new Error(`usage_interval_invalid:${this.selectedUsageInterval}`);
      }
      return interval.label;
    },

    get usageFailureStatusRows() {
      return this.usage.status_codes
        .filter((/** @type {{ status_code: number, requests: number }} */ status) => status.status_code >= HTTP_ERROR_STATUS_MINIMUM)
        .map((/** @type {{ status_code: number, requests: number }} */ status) => ({
          statusCode: status.status_code,
          label: usageStatusLabel(status.status_code),
          requests: formatNumber(status.requests),
        }));
    },

    get usageFailureRows() {
      return this.usageFailures.map((/** @type {import("../types.d.js").ManagementUsageFailure | import("../types.d.js").ManagementAccountUsageFailure} */ failure) => usageFailurePresentation(failure));
    },

    get hasLoadedUsageFailures() {
      return this.usageFailures.length > 0;
    },

    get canLoadMoreUsageFailures() {
      return Boolean(this.usageFailuresNextCursor);
    },

    get usageRequestChart() {
      return usageTimeSeriesChart(this.usage, USAGE_METRICS.REQUESTS);
    },

    get usageTokenChart() {
      return usageTimeSeriesChart(this.usage, USAGE_METRICS.TOTAL_TOKENS);
    },

    get providerUsageDistribution() {
      return providerDistribution(this.usage);
    },

    get modelUsageDistribution() {
      return modelDistribution(this.usage);
    },

    get hasProviderUsageBreakdown() {
      return this.providerUsageDistribution.totalRequests > 0;
    },

    get hasModelUsageBreakdown() {
      return this.modelUsageDistribution.totalRequests > 0;
    },

    get usageBreakdownIsBar() {
      return this.usageBreakdownView === USAGE_BREAKDOWN_VIEWS.BAR;
    },

    get usageBreakdownIsDonut() {
      return this.usageBreakdownView === USAGE_BREAKDOWN_VIEWS.DONUT;
    },

    /** @param {import("../types.d.js").UsageBreakdownView} view */
    selectUsageBreakdownView(view) {
      if (!Object.values(USAGE_BREAKDOWN_VIEWS).includes(view)) {
        throw new Error(`usage_breakdown_view_invalid:${view}`);
      }
      this.usageBreakdownView = view;
    },

    /** @param {SVGElement} target @param {import("../types.d.js").UsageTimeSeriesChart} chart */
    renderUsageChartPlot(target, chart) {
      renderUsageChartPlot(target, chart);
    },

    /** @param {SVGElement} target @param {import("../types.d.js").UsageDistributionRow[]} rows */
    renderUsageDonutSegments(target, rows) {
      renderUsageDonutSegments(target, rows);
    },

    async refreshDashboard() {
      if (this.dashboardView === DASHBOARD_VIEWS.ADMIN) {
        await this.refreshAdminUsers();
        return;
      }
      await this.refreshUsage();
    },

    async refreshUsage() {
      await this.loadUsageSummary(true);
    },

    /** @param {import("../types.d.js").UsageInterval} interval */
    async selectUsageInterval(interval) {
      if (!this.usageIntervals.some((candidate) => candidate.id === interval)) {
        throw new Error(`usage_interval_invalid:${interval}`);
      }
      this.resetProviderCard();
      this.clearUsageFailures(false);
      this.selectedUsageInterval = interval;
      this.usage = emptyUsageSummary(interval);
      this.usageProfile = null;
      await this.loadUsageSummary(false);
    },

    /** @param {Event} event */
    async handleUsageTenantSelection(event) {
      const tenantSelect = /** @type {HTMLSelectElement} */ (event.target);
      const tenantID = tenantSelect.value;
      if (tenantID && !this.tenants.some((tenant) => tenant.id === tenantID)) {
        tenantSelect.value = this.selectedUsageTenantID;
        this.setPageNotice(NOTICE_KINDS.ERROR, COPY.requestFailed);
        return;
      }
      if (tenantID === this.selectedUsageTenantID) {
        return;
      }
      this.resetProviderCard();
      this.clearUsageFailures(false);
      this.selectedUsageTenantID = tenantID;
      this.usage = emptyUsageSummary(this.selectedUsageInterval);
      this.usageProfile = null;
      await this.loadUsageSummary(false);
    },

    openUsageFailures() {
      if (!this.hasUsageFailures || this.dashboardView !== DASHBOARD_VIEWS.USAGE) {
        return;
      }
      this.clearUsageFailures(false);
      this.usageFailuresOpen = true;
      this.$nextTick(() => {
        this.$refs.usageFailuresClose.focus();
      });
      void this.loadUsageFailuresPage(false);
    },

    closeUsageFailures() {
      this.clearUsageFailures(true);
    },

    /** @param {KeyboardEvent} event */
    trapUsageFailuresFocus(event) {
      trapDialogFocus(event, this.$refs.usageFailuresDialog);
    },

    async retryUsageFailures() {
      await this.loadUsageFailuresPage(this.hasLoadedUsageFailures);
    },

    async loadMoreUsageFailures() {
      await this.loadUsageFailuresPage(true);
    },

    /** @param {boolean} append */
    async loadUsageFailuresPage(append) {
      if (!this.usageFailuresOpen) {
        return;
      }
      const cursor = append ? this.usageFailuresNextCursor : EMPTY_STRING;
      if (append && !cursor) {
        return;
      }
      const tenantID = this.selectedUsageTenantID;
      const interval = this.selectedUsageInterval;
      const loadVersion = this.usageFailuresLoadVersion + 1;
      this.usageFailuresLoadVersion = loadVersion;
      if (this.usageFailuresRequestController) {
        this.usageFailuresRequestController.abort();
      }
      const requestController = new AbortController();
      this.usageFailuresRequestController = requestController;
      this.usageFailuresLoading = true;
      this.usageFailuresError = EMPTY_STRING;
      try {
        const response = tenantID
          ? await fetchUsageFailures(
            tenantID,
            interval,
            USAGE_FAILURE_PAGE_LIMIT,
            cursor,
            requestController.signal,
          )
          : await fetchAccountUsageFailures(
            interval,
            USAGE_FAILURE_PAGE_LIMIT,
            cursor,
            requestController.signal,
          );
        if (!this.canApplyUsageFailures(tenantID, loadVersion, interval)) {
          return;
        }
        const page = normalizedUsageFailurePage(response, interval, !tenantID);
        this.usageFailures = append ? [...this.usageFailures, ...page.failures] : page.failures;
        this.usageFailuresNextCursor = page.next_cursor || EMPTY_STRING;
      } catch (requestError) {
        if (
          !isAbortError(requestError) &&
          this.canApplyUsageFailures(tenantID, loadVersion, interval)
        ) {
          this.usageFailuresError = COPY.usageFailuresError;
        }
      } finally {
        if (this.usageFailuresRequestController === requestController) {
          this.usageFailuresRequestController = null;
        }
        if (this.canApplyUsageFailures(tenantID, loadVersion, interval)) {
          this.usageFailuresLoading = false;
        }
      }
    },

    /**
     * @param {string} tenantID
     * @param {number} loadVersion
     * @param {import("../types.d.js").UsageInterval} interval
     * @returns {boolean}
     */
    canApplyUsageFailures(tenantID, loadVersion, interval) {
      return (
        this.usageFailuresOpen &&
        this.selectedUsageTenantID === tenantID &&
        this.usageFailuresLoadVersion === loadVersion &&
        this.selectedUsageInterval === interval &&
        this.authState === AUTH_STATES.AUTHENTICATED
      );
    },

    /** @param {boolean} restoreFocus */
    clearUsageFailures(restoreFocus) {
      const restoreActionFocus = restoreFocus && this.usageFailuresOpen && this.hasUsageFailures;
      if (this.usageFailuresRequestController) {
        this.usageFailuresRequestController.abort();
        this.usageFailuresRequestController = null;
      }
      this.usageFailuresLoadVersion += 1;
      this.usageFailuresOpen = false;
      this.usageFailuresLoading = false;
      this.usageFailuresError = EMPTY_STRING;
      this.usageFailures = [];
      this.usageFailuresNextCursor = EMPTY_STRING;
      if (restoreActionFocus) {
        this.$nextTick(() => {
          if (this.$refs.usageFailuresAction) {
            this.$refs.usageFailuresAction.focus();
          }
        });
      }
    },

    /** @param {boolean} showSuccessNotice */
    async loadUsageSummary(showSuccessNotice) {
      const tenantID = this.selectedUsageTenantID;
      const interval = this.selectedUsageInterval;
      const loadVersion = this.usageLoadVersion + 1;
      this.usageLoadVersion = loadVersion;
      if (this.usageRequestController) {
        this.usageRequestController.abort();
      }
      const usageRequestController = new AbortController();
      this.usageRequestController = usageRequestController;
      this.usageLoading = true;
      this.usageLoadState = "loading";
      try {
        const [usage, usageProfile] = tenantID
          ? await Promise.all([
            fetchUsageSummary(tenantID, interval, usageRequestController.signal),
            fetchTenant(tenantID, usageRequestController.signal),
          ])
          : [await fetchAccountUsageSummary(interval, usageRequestController.signal), null];
        if (!this.canApplyUsageSummary(tenantID, loadVersion, interval)) {
          return;
        }
        if (usage.interval !== interval) {
          throw new Error(APP_INTEGRITY_ERROR);
        }
        if (usageProfile) assertManagementTenantProfile(usageProfile, tenantID);
        this.usage = usage;
        this.usageProfile = usageProfile;
        this.usageLoadState = "available";
        if (!this.hasUsageFailures) {
          this.clearUsageFailures(false);
        }
        if (showSuccessNotice) {
          this.setPageNotice(NOTICE_KINDS.SUCCESS, COPY.usageRefreshed);
        }
      } catch (requestError) {
        if (!isAbortError(requestError) && this.canApplyUsageSummary(tenantID, loadVersion, interval)) {
          this.clearUsageFailures(false);
          this.usage = emptyUsageSummary(interval);
          this.usageProfile = null;
          this.usageLoadState = "unavailable";
          this.setPageNotice(NOTICE_KINDS.ERROR, COPY.requestFailed);
        }
      } finally {
        if (this.usageRequestController === usageRequestController) {
          this.usageRequestController = null;
        }
        if (
          this.selectedUsageTenantID === tenantID &&
          this.usageLoadVersion === loadVersion
        ) {
          this.usageLoading = false;
        }
      }
    },

    /**
     * @param {string} tenantID
     * @param {number} loadVersion
     * @param {import("../types.d.js").UsageInterval} interval
     * @returns {boolean}
     */
    canApplyUsageSummary(tenantID, loadVersion, interval) {
      return (
        this.selectedUsageTenantID === tenantID &&
        this.usageLoadVersion === loadVersion &&
        this.selectedUsageInterval === interval &&
        this.authState === AUTH_STATES.AUTHENTICATED
      );
    },

    clearUsageState() {
      this.usageLoadVersion += 1;
      this.usageLoading = false;
      this.usageLoadState = "loading";
      this.clearUsageFailures(false);
      this.usage = emptyUsageSummary(this.selectedUsageInterval);
      this.usageProfile = null;
    },
  });
}
