// @ts-check

import {
  APP_INTEGRITY_ERROR,
  AUTH_STATES,
  COPY,
  DASHBOARD_VIEWS,
  NOTICE_KINDS,
  USAGE_DETAIL_KINDS,
  USAGE_DETAIL_PAGE_LIMIT,
} from "../constants.js?v=20260903f037";
import {
  fetchAccountUsageFailures,
  fetchAccountUsageRejections,
  fetchAccountUsageSummary,
  fetchTenant,
  fetchUsageFailures,
  fetchUsageRejections,
  fetchUsageSummary,
} from "../core/backendClient.js?v=20260903f037";
import { assertManagementTenantProfile, isAbortError } from "../core/managementProfile.js?v=20260903f037";
import { trapDialogFocus } from "./dialogFocus.js?v=20260903f037";
import {
  formatNumber,
  normalizedUsageDetailPage,
  usageDetailPresentation,
  usageStatusLabel,
} from "./usageFailurePresentation.js?v=20260903f037";
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
} from "./usagePresentation.js?v=20260903f037";

const EMPTY_STRING = "";
const HTTP_ERROR_STATUS_MINIMUM = 400;

/** @typedef {ReturnType<typeof import("./managementApplicationState.js").createManagementApplicationState>} ManagementApplicationState */
/** @typedef {ManagementApplicationState & import("../types.d.js").AlpineMagic & {
 *   hasUsageFailures: boolean,
 *   hasLoadedUsageDetails: boolean,
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

    get hasUsageRejections() {
      return this.usage.rejected_requests > 0;
    },

    get usageRejectionsActionCopy() {
      const rejectionCount = this.usage.rejected_requests;
      const noun = rejectionCount === 1 ? "rejected request" : "rejected requests";
      return `${formatNumber(rejectionCount)} ${noun}`;
    },

    get usageDetailsIntervalLabel() {
      const interval = this.usageIntervals.find((/** @type {{ id: import("../types.d.js").UsageInterval }} */ candidate) => candidate.id === this.selectedUsageInterval);
      if (!interval) {
        throw new Error(`usage_interval_invalid:${this.selectedUsageInterval}`);
      }
      return interval.label;
    },

    get usageDetailsOpen() {
      return Boolean(this.usageDetailsKind);
    },

    get usageDetailsAreFailures() {
      return this.usageDetailsKind === USAGE_DETAIL_KINDS.FAILURES;
    },

    get usageDetailsTitle() {
      return this.usageDetailsAreFailures ? COPY.usageFailuresTitle : COPY.usageRejectionsTitle;
    },

    get usageDetailsDescription() {
      return this.usageDetailsAreFailures ? COPY.usageFailuresDescription : COPY.usageRejectionsDescription;
    },

    get usageDetailsLoadingCopy() {
      return this.usageDetailsAreFailures ? COPY.usageFailuresLoading : COPY.usageRejectionsLoading;
    },

    get usageDetailsEmptyCopy() {
      return this.usageDetailsAreFailures ? COPY.usageFailuresEmpty : COPY.usageRejectionsEmpty;
    },

    get usageDetailsCloseCopy() {
      return this.usageDetailsAreFailures ? COPY.closeUsageFailures : COPY.closeUsageRejections;
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

    get usageDetailRows() {
      return this.usageDetails.map((/** @type {import("../types.d.js").ManagementUsageFailure | import("../types.d.js").ManagementAccountUsageFailure | import("../types.d.js").ManagementUsageRejection | import("../types.d.js").ManagementAccountUsageRejection} */ detail) => usageDetailPresentation(detail));
    },

    get hasLoadedUsageDetails() {
      return this.usageDetails.length > 0;
    },

    get canLoadMoreUsageDetails() {
      return Boolean(this.usageDetailsNextCursor);
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

    get providerUsageBreakdownIsBar() {
      return this.providerUsageBreakdownView === USAGE_BREAKDOWN_VIEWS.BAR;
    },

    get providerUsageBreakdownIsDonut() {
      return this.providerUsageBreakdownView === USAGE_BREAKDOWN_VIEWS.DONUT;
    },

    get modelUsageBreakdownIsBar() {
      return this.modelUsageBreakdownView === USAGE_BREAKDOWN_VIEWS.BAR;
    },

    get modelUsageBreakdownIsDonut() {
      return this.modelUsageBreakdownView === USAGE_BREAKDOWN_VIEWS.DONUT;
    },

    get providerUsageBreakdownToggleLabel() {
      return breakdownToggleLabel(this.providerUsageBreakdownView);
    },

    get modelUsageBreakdownToggleLabel() {
      return breakdownToggleLabel(this.modelUsageBreakdownView);
    },

    toggleProviderUsageBreakdownView() {
      this.providerUsageBreakdownView = toggledBreakdownView(this.providerUsageBreakdownView);
    },

    toggleModelUsageBreakdownView() {
      this.modelUsageBreakdownView = toggledBreakdownView(this.modelUsageBreakdownView);
    },

    resetUsageBreakdownViews() {
      this.providerUsageBreakdownView = USAGE_BREAKDOWN_VIEWS.BAR;
      this.modelUsageBreakdownView = USAGE_BREAKDOWN_VIEWS.BAR;
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
      this.clearUsageDetails(false);
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
      this.clearUsageDetails(false);
      this.selectedUsageTenantID = tenantID;
      this.usage = emptyUsageSummary(this.selectedUsageInterval);
      this.usageProfile = null;
      await this.loadUsageSummary(false);
    },

    openUsageFailures() {
      this.openUsageDetails(USAGE_DETAIL_KINDS.FAILURES);
    },

    openUsageRejections() {
      this.openUsageDetails(USAGE_DETAIL_KINDS.REJECTIONS);
    },

    /** @param {import("../types.d.js").UsageDetailKind} kind */
    openUsageDetails(kind) {
      const hasDetails = kind === USAGE_DETAIL_KINDS.FAILURES ? this.hasUsageFailures : this.hasUsageRejections;
      if (!hasDetails || this.dashboardView !== DASHBOARD_VIEWS.USAGE) return;
      this.clearUsageDetails(false);
      this.usageDetailsKind = kind;
      this.$nextTick(() => this.$refs.usageDetailsClose.focus());
      void this.loadUsageDetailsPage(false);
    },

    closeUsageDetails() {
      this.clearUsageDetails(true);
    },

    /** @param {KeyboardEvent} event */
    trapUsageDetailsFocus(event) {
      trapDialogFocus(event, this.$refs.usageDetailsDialog);
    },

    async retryUsageDetails() {
      await this.loadUsageDetailsPage(this.hasLoadedUsageDetails);
    },

    async loadMoreUsageDetails() {
      await this.loadUsageDetailsPage(true);
    },

    /** @param {boolean} append */
    async loadUsageDetailsPage(append) {
      if (!this.usageDetailsOpen) return;
      const kind = /** @type {import("../types.d.js").UsageDetailKind} */ (this.usageDetailsKind);
      const cursor = append ? this.usageDetailsNextCursor : EMPTY_STRING;
      if (append && !cursor) return;
      const tenantID = this.selectedUsageTenantID;
      const interval = this.selectedUsageInterval;
      const loadVersion = this.usageDetailsLoadVersion + 1;
      this.usageDetailsLoadVersion = loadVersion;
      if (this.usageDetailsRequestController) this.usageDetailsRequestController.abort();
      const requestController = new AbortController();
      this.usageDetailsRequestController = requestController;
      this.usageDetailsLoading = true;
      this.usageDetailsError = EMPTY_STRING;
      try {
        const response = await fetchUsageDetails(kind, tenantID, interval, cursor, requestController.signal);
        if (!this.canApplyUsageDetails(kind, tenantID, loadVersion, interval)) return;
        const page = normalizedUsageDetailPage(response, interval, !tenantID, kind);
        this.usageDetails = append ? [...this.usageDetails, ...page.items] : page.items;
        this.usageDetailsNextCursor = page.next_cursor || EMPTY_STRING;
      } catch (requestError) {
        if (!isAbortError(requestError) && this.canApplyUsageDetails(kind, tenantID, loadVersion, interval)) {
          this.usageDetailsError = kind === USAGE_DETAIL_KINDS.FAILURES ? COPY.usageFailuresError : COPY.usageRejectionsError;
        }
      } finally {
        if (this.usageDetailsRequestController === requestController) this.usageDetailsRequestController = null;
        if (this.canApplyUsageDetails(kind, tenantID, loadVersion, interval)) this.usageDetailsLoading = false;
      }
    },

    /**
     * @param {import("../types.d.js").UsageDetailKind} kind
     * @param {string} tenantID
     * @param {number} loadVersion
     * @param {import("../types.d.js").UsageInterval} interval
     * @returns {boolean}
     */
    canApplyUsageDetails(kind, tenantID, loadVersion, interval) {
      return this.usageDetailsKind === kind &&
        this.selectedUsageTenantID === tenantID &&
        this.usageDetailsLoadVersion === loadVersion &&
        this.selectedUsageInterval === interval &&
        this.authState === AUTH_STATES.AUTHENTICATED;
    },

    /** @param {boolean} restoreFocus */
    clearUsageDetails(restoreFocus) {
      const previousKind = this.usageDetailsKind;
      const restoreActionFocus = restoreFocus && this.usageDetailsOpen;
      if (this.usageDetailsRequestController) {
        this.usageDetailsRequestController.abort();
        this.usageDetailsRequestController = null;
      }
      this.usageDetailsLoadVersion += 1;
      this.usageDetailsKind = EMPTY_STRING;
      this.usageDetailsLoading = false;
      this.usageDetailsError = EMPTY_STRING;
      this.usageDetails = [];
      this.usageDetailsNextCursor = EMPTY_STRING;
      if (restoreActionFocus) {
        this.$nextTick(() => {
          const action = previousKind === USAGE_DETAIL_KINDS.FAILURES ? this.$refs.usageFailuresAction : this.$refs.usageRejectionsAction;
          if (action) action.focus();
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
        if (usage.interval !== interval || !Number.isInteger(usage.rejected_requests) || usage.rejected_requests < 0) {
          throw new Error(APP_INTEGRITY_ERROR);
        }
        if (usageProfile) assertManagementTenantProfile(usageProfile, tenantID);
        this.usage = usage;
        this.usageProfile = usageProfile;
        this.usageLoadState = "available";
        if (
          (this.usageDetailsKind === USAGE_DETAIL_KINDS.FAILURES && !this.hasUsageFailures) ||
          (this.usageDetailsKind === USAGE_DETAIL_KINDS.REJECTIONS && !this.hasUsageRejections)
        ) {
          this.clearUsageDetails(false);
        }
        if (showSuccessNotice) {
          this.setPageNotice(NOTICE_KINDS.SUCCESS, COPY.usageRefreshed);
        }
      } catch (requestError) {
        if (!isAbortError(requestError) && this.canApplyUsageSummary(tenantID, loadVersion, interval)) {
          this.clearUsageDetails(false);
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
      this.clearUsageDetails(false);
      this.usage = emptyUsageSummary(this.selectedUsageInterval);
      this.usageProfile = null;
    },
  });
}

/** @param {import("../types.d.js").UsageBreakdownView} view */
function breakdownToggleLabel(view) {
  const nextView = toggledBreakdownView(view);
  return nextView === USAGE_BREAKDOWN_VIEWS.DONUT ? COPY.usageShowBreakdownDonut : COPY.usageShowBreakdownBar;
}

/** @param {import("../types.d.js").UsageBreakdownView} view @returns {import("../types.d.js").UsageBreakdownView} */
function toggledBreakdownView(view) {
  if (view === USAGE_BREAKDOWN_VIEWS.BAR) return USAGE_BREAKDOWN_VIEWS.DONUT;
  if (view === USAGE_BREAKDOWN_VIEWS.DONUT) return USAGE_BREAKDOWN_VIEWS.BAR;
  throw new Error(`usage_breakdown_view_invalid:${view}`);
}

/**
 * @param {import("../types.d.js").UsageDetailKind} kind
 * @param {string} tenantID
 * @param {import("../types.d.js").UsageInterval} interval
 * @param {string} cursor
 * @param {AbortSignal} signal
 */
function fetchUsageDetails(kind, tenantID, interval, cursor, signal) {
  if (kind === USAGE_DETAIL_KINDS.FAILURES) {
    return tenantID
      ? fetchUsageFailures(tenantID, interval, USAGE_DETAIL_PAGE_LIMIT, cursor, signal)
      : fetchAccountUsageFailures(interval, USAGE_DETAIL_PAGE_LIMIT, cursor, signal);
  }
  if (kind === USAGE_DETAIL_KINDS.REJECTIONS) {
    return tenantID
      ? fetchUsageRejections(tenantID, interval, USAGE_DETAIL_PAGE_LIMIT, cursor, signal)
      : fetchAccountUsageRejections(interval, USAGE_DETAIL_PAGE_LIMIT, cursor, signal);
  }
  throw new Error(`usage_detail_kind_invalid:${kind}`);
}
