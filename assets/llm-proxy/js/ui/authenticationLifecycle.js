// @ts-check

import {
  AUTH_STATES,
  COPY,
  DASHBOARD_VIEWS,
  DEFAULT_USAGE_INTERVAL,
  EVENTS,
  NOTICE_KINDS,
  NOTICE_SURFACES,
  PUBLIC_SITE_PATH,
} from "../constants.js?v=20260903f037";
import {
  fetchAccount,
  loadFrontendRuntimeConfig,
} from "../core/backendClient.js?v=20260903f037";
import {
  emptyDefaults,
  isAbortError,
  profileFailureMessage,
} from "../core/managementProfile.js?v=20260903f037";
import {
  applyUserMenuItems,
  readMprUIAuthStatus,
  waitForMprUIAutoOrchestrationReady,
} from "../core/mprShell.js?v=20260903f037";
import { dispatchManagementReady } from "../core/runtimeTransition.js?v=20260903f037";

const EMPTY_STRING = "";

/** @typedef {ReturnType<typeof import("./managementApplicationState.js").createManagementApplicationState>} ManagementApplicationState */
/** @typedef {ManagementApplicationState & {
 *   applyProfile: (profile: import("../types.d.js").ManagementTenantProfile, preserveProviderEditor?: boolean, preserveRoutingDefaults?: boolean) => void,
 *   clearNotice: () => void,
 *   clearUsageState: () => void,
 *   resetUsageBreakdownViews: () => void,
 *   handleUserMenuItem: (event: Event) => void,
 *   loadUsageSummary: (showSuccessNotice: boolean) => Promise<void>,
 *   setNotice: (kind: string, message: string, surface: string) => void,
 *   setPageNotice: (kind: string, message: string) => void
 * }} AuthenticationLifecycleHost */

/**
 * @template {object} Responsibility
 * @param {Responsibility & ThisType<AuthenticationLifecycleHost & Responsibility>} responsibility
 * @returns {Responsibility}
 */
function authenticationLifecycleResponsibility(responsibility) {
  return responsibility;
}

/** Create MPR UI authentication lifecycle, application hydration, and boundary cleanup. */
export function createAuthenticationLifecycleResponsibility() {
  return authenticationLifecycleResponsibility({
    init() {
      document.addEventListener(EVENTS.AUTHENTICATED, () => {
        void this.loadAuthenticatedApp();
      });
      document.addEventListener(EVENTS.UNAUTHENTICATED, () => {
        this.setUnauthenticated();
      });
      document.addEventListener(EVENTS.AUTH_STATUS_CHANGE, (event) => {
        const customEvent = /** @type {CustomEvent<{ status?: string }>} */ (event);
        const status = customEvent.detail ? customEvent.detail.status : EMPTY_STRING;
        if (status === AUTH_STATES.UNAUTHENTICATED) {
          this.setUnauthenticated();
        }
      });
      document.addEventListener(EVENTS.USER_MENU_ITEM, (event) => {
        this.handleUserMenuItem(event);
      });
      void this.start();
    },

    async start() {
      try {
        this.runtimeConfig = await loadFrontendRuntimeConfig();
        await waitForMprUIAutoOrchestrationReady();
        const authStatus = readMprUIAuthStatus();
        if (authStatus === AUTH_STATES.AUTHENTICATED) {
          await this.loadAuthenticatedApp();
        } else if (authStatus === AUTH_STATES.UNAUTHENTICATED) {
          this.setUnauthenticated();
        }
      } catch (requestError) {
        this.clearAuthenticatedState();
        this.authState = AUTH_STATES.ERROR;
        this.setPageNotice(NOTICE_KINDS.ERROR, COPY.requestFailed);
        dispatchManagementReady();
      }
    },

    async loadApp() {
      if (this.appLoadPromise) {
        return this.appLoadPromise;
      }
      this.appLoadPromise = this.loadAppOnce();
      try {
        await this.appLoadPromise;
      } finally {
        this.appLoadPromise = null;
      }
    },

    async loadAuthenticatedApp() {
      if (this.authState === AUTH_STATES.AUTHENTICATED || this.authState === AUTH_STATES.ERROR) {
        await dispatchManagementReady();
        return;
      }
      this.authState = AUTH_STATES.LOADING;
      await this.loadApp();
      if (this.authState === AUTH_STATES.LOADING && readMprUIAuthStatus() === AUTH_STATES.AUTHENTICATED) {
        await this.loadApp();
      }
    },

    async loadAppOnce() {
      const appVersion = this.appVersion;
      if (this.accountRequestController) {
        this.accountRequestController.abort();
      }
      const accountRequestController = new AbortController();
      this.accountRequestController = accountRequestController;
      this.busy = true;
      try {
        const loadedAccount = await fetchAccount(accountRequestController.signal);
        if (!this.canApplyAuthenticatedApp(appVersion)) {
          return;
        }
        this.account = loadedAccount;
        this.tenants = loadedAccount.tenants;
        applyUserMenuItems(Boolean(loadedAccount.user.is_admin));
        this.authState = AUTH_STATES.AUTHENTICATED;
        if (this.authState === AUTH_STATES.AUTHENTICATED) {
          await this.loadUsageSummary(false);
        }
      } catch (requestError) {
        if (!isAbortError(requestError) && this.canApplyAuthenticatedApp(appVersion)) {
          this.clearAuthenticatedState();
          this.authState = AUTH_STATES.ERROR;
          this.setPageNotice(NOTICE_KINDS.ERROR, profileFailureMessage(requestError));
        }
      } finally {
        if (this.accountRequestController === accountRequestController) {
          this.accountRequestController = null;
        }
        this.busy = false;
        dispatchManagementReady();
      }
    },

    /**
     * @param {number} appVersion
     * @returns {boolean}
     */
    canApplyAuthenticatedApp(appVersion) {
      return (
        this.appVersion === appVersion &&
        readMprUIAuthStatus() === AUTH_STATES.AUTHENTICATED
      );
    },

    /**
     * @param {number} appVersion
     * @param {string} tenantID
     * @returns {boolean}
     */
    canApplySettingsTenant(appVersion, tenantID) {
      return this.canApplyAuthenticatedApp(appVersion) && this.settingsTenantID === tenantID;
    },

    setUnauthenticated() {
      if (this.authState === AUTH_STATES.UNAUTHENTICATED) {
        return;
      }
      this.clearAuthenticatedState();
      this.authState = AUTH_STATES.UNAUTHENTICATED;
      window.location.replace(PUBLIC_SITE_PATH);
    },

    clearAuthenticatedState() {
      this.appVersion += 1;
      if (this.accountRequestController) {
        this.accountRequestController.abort();
        this.accountRequestController = null;
      }
      if (this.tenantRequestController) {
        this.tenantRequestController.abort();
        this.tenantRequestController = null;
      }
      if (this.usageRequestController) {
        this.usageRequestController.abort();
        this.usageRequestController = null;
      }
      if (this.usageDetailsRequestController) {
        this.usageDetailsRequestController.abort();
        this.usageDetailsRequestController = null;
      }
      if (this.tenantLifetimeController) {
        this.tenantLifetimeController.abort();
        this.tenantLifetimeController = null;
      }
      this.selectedUsageInterval = DEFAULT_USAGE_INTERVAL;
      this.selectedUsageTenantID = EMPTY_STRING;
      this.resetUsageBreakdownViews();
      this.clearNotice();
      this.profile = null;
      this.providers = [];
      this.defaults = emptyDefaults();
      this.clearUsageState();
      this.account = null;
      this.tenants = [];
      this.settingsTenantID = EMPTY_STRING;
      this.adminUsers = [];
      this.dashboardView = DASHBOARD_VIEWS.USAGE;
      applyUserMenuItems(false);
    },
  });
}
