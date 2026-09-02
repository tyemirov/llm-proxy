// @ts-check

import {
  COPY,
  DASHBOARD_VIEWS,
  NOTICE_KINDS,
} from "../constants.js?v=20260902c239";
import { fetchAdminUsers } from "../core/backendClient.js?v=20260902c239";
import { formatNumber } from "./usageFailurePresentation.js?v=20260902c239";
import { successRateLabel } from "./usagePresentation.js?v=20260902c239";

/** @typedef {ReturnType<typeof import("./managementApplicationState.js").createManagementApplicationState>} ManagementApplicationState */
/** @typedef {ManagementApplicationState & {
 *   clearUsageFailures: (restoreFocus: boolean) => void,
 *   resetProviderCard: () => void,
 *   setPageNotice: (kind: string, message: string) => void
 * }} AdminDashboardHost */

/**
 * @template {object} Responsibility
 * @param {Responsibility & ThisType<AdminDashboardHost & Responsibility>} responsibility
 * @returns {Responsibility}
 */
function adminDashboardResponsibility(responsibility) {
  return responsibility;
}

/** Create administrator usage-dashboard behavior and presentation. */
export function createAdminDashboardResponsibility() {
  return adminDashboardResponsibility({
    get isAdmin() {
      return Boolean(this.account && this.account.user.is_admin);
    },

    get dashboardEyebrow() {
      return this.dashboardView === DASHBOARD_VIEWS.ADMIN ? COPY.adminDashboardEyebrow : COPY.dashboardEyebrow;
    },

    get dashboardTitle() {
      return this.dashboardView === DASHBOARD_VIEWS.ADMIN ? COPY.adminDashboardTitle : COPY.dashboardTitle;
    },

    get dashboardRefreshCopy() {
      return this.dashboardView === DASHBOARD_VIEWS.ADMIN ? COPY.refreshAdmin : COPY.refreshUsage;
    },

    get dashboardRefreshDisabled() {
      return this.busy || this.usageLoading;
    },

    get hasAdminUsers() {
      return this.adminUsers.length > 0;
    },

    async refreshAdminUsers() {
      if (!this.isAdmin) {
        return;
      }
      this.busy = true;
      try {
        const adminUsersResponse = await fetchAdminUsers();
        this.adminUsers = adminUsersResponse.users;
        this.setPageNotice(NOTICE_KINDS.SUCCESS, COPY.usageRefreshed);
      } catch (requestError) {
        this.adminUsers = [];
        this.setPageNotice(NOTICE_KINDS.ERROR, COPY.requestFailed);
      } finally {
        this.busy = false;
      }
    },

    async openAdminDashboard() {
      if (!this.isAdmin) {
        return;
      }
      this.resetProviderCard();
      this.clearUsageFailures(false);
      this.dashboardView = DASHBOARD_VIEWS.ADMIN;
      await this.refreshAdminUsers();
    },

    openUsageDashboard() {
      this.dashboardView = DASHBOARD_VIEWS.USAGE;
    },

    /**
     * @param {import("../types.d.js").ManagementAdminUser} adminUser
     * @returns {string}
     */
    adminUserLabel(adminUser) {
      return adminUser.user.email || adminUser.user.display_name || adminUser.user.id || COPY.adminUserFallback;
    },

    /**
     * @param {{ usage: import("../types.d.js").ManagementAdminUsageSummary }} adminTenant
     * @returns {string}
     */
    adminTenantRequests(adminTenant) {
      return formatNumber(adminTenant.usage.totals.requests);
    },

    /**
     * @param {{ usage: import("../types.d.js").ManagementAdminUsageSummary }} adminTenant
     * @returns {string}
     */
    adminTenantTokens(adminTenant) {
      return formatNumber(adminTenant.usage.totals.total_tokens);
    },

    /**
     * @param {{ usage: import("../types.d.js").ManagementAdminUsageSummary }} adminTenant
     * @returns {string}
     */
    adminTenantSuccessRate(adminTenant) {
      return successRateLabel(adminTenant.usage.totals);
    },
  });
}
