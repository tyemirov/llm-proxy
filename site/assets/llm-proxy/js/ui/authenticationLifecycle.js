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
} from "../constants.js?v=20261116b323";
import {
  fetchAccount,
  FrontendConfigRateLimitError,
  loadFrontendRuntimeConfig,
} from "../core/backendClient.js?v=20261116b323";
import {
  emptyDefaults,
  isAbortError,
  profileFailureMessage,
} from "../core/managementProfile.js?v=20261116b323";
import {
  applyUserMenuItems,
  readMprUIAuthStatus,
  waitForMprUIAutoOrchestrationReady,
} from "../core/mprShell.js?v=20261116b323";
import { dispatchManagementReady } from "../core/runtimeTransition.js?v=20261116b323";

const EMPTY_STRING = "";

/** @typedef {ReturnType<typeof import("./managementApplicationState.js").createManagementApplicationState>} ManagementApplicationState */
/** @typedef {ManagementApplicationState & {
 *   applyProfile: (profile: import("../types.d.js").ManagementTenantProfile, preserveProviderEditor?: boolean, preserveRoutingDefaults?: boolean) => void,
 *   clearNotice: () => void,
 *   clearUsageState: () => void,
 *   resetUsageBreakdownViews: () => void,
 *   startAutoRefresh: () => void,
 *   stopAutoRefresh: () => void,
 *   handleUserMenuItem: (event: Event) => void,
 *   loadUsageSummary: () => Promise<void>,
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
  /** @type {Promise<void> | null} */
  let startupPromise = null;
  /** @type {FrontendConfigRateLimitError | null} */
  let bootstrapRateLimit = null;
  /** @type {number | null} */
  let recoveryTimer = null;
  const lifetime = new AbortController();
  return authenticationLifecycleResponsibility({
    init() {
      document.addEventListener(EVENTS.AUTHENTICATED, () => {
        void this.resumeAuthenticatedApp();
      }, { signal: lifetime.signal });
      document.addEventListener(EVENTS.UNAUTHENTICATED, () => {
        this.setUnauthenticated();
      }, { signal: lifetime.signal });
      document.addEventListener(EVENTS.AUTH_STATUS_CHANGE, (event) => {
        const customEvent = /** @type {CustomEvent<{ status?: string }>} */ (event);
        const status = customEvent.detail ? customEvent.detail.status : EMPTY_STRING;
        if (status === AUTH_STATES.UNAUTHENTICATED) {
          this.setUnauthenticated();
        }
      }, { signal: lifetime.signal });
      document.addEventListener(EVENTS.USER_MENU_ITEM, (event) => {
        this.handleUserMenuItem(event);
      }, { signal: lifetime.signal });
      window.addEventListener("focus", () => void this.requestBootstrapRecovery(), { signal: lifetime.signal });
      window.addEventListener("online", () => void this.requestBootstrapRecovery(), { signal: lifetime.signal });
      void this.start();
    },

    destroy() {
      lifetime.abort();
      this.clearAuthenticatedState();
    },

    async resumeAuthenticatedApp() {
      if (startupPromise) await startupPromise;
      if (lifetime.signal.aborted) return;
      if (bootstrapRateLimit) {
        await this.requestBootstrapRecovery();
      } else if (this.authState !== AUTH_STATES.ERROR) {
        await this.loadAuthenticatedApp();
      }
    },

    async requestBootstrapRecovery() {
      if (!bootstrapRateLimit || recoveryTimer !== null || lifetime.signal.aborted) return;
      const appVersion = this.appVersion;
      try {
        await waitForMprUIAutoOrchestrationReady();
      } catch {
        this.setPageNotice(NOTICE_KINDS.ERROR, COPY.requestFailed);
        return;
      }
      if (appVersion !== this.appVersion || !bootstrapRateLimit || recoveryTimer !== null || lifetime.signal.aborted ||
          readMprUIAuthStatus() !== AUTH_STATES.AUTHENTICATED) return;
      const wait = Math.max(0, bootstrapRateLimit.retryAt - Date.now());
      recoveryTimer = window.setTimeout(() => {
        recoveryTimer = null;
        if (appVersion !== this.appVersion || lifetime.signal.aborted || readMprUIAuthStatus() !== AUTH_STATES.AUTHENTICATED) return;
        bootstrapRateLimit = null;
        void this.start("recovery");
      }, Math.min(wait, 2147483647));
    },

    /** @param {"initial" | "recovery"} [origin] */
    async start(origin = "initial") {
      if (startupPromise) return startupPromise;
      const attempt = this.startOnce(origin);
      startupPromise = attempt;
      try { await attempt; }
      finally { if (startupPromise === attempt) startupPromise = null; }
    },

    /** @param {"initial" | "recovery"} origin */
    async startOnce(origin) {
      const appVersion = this.appVersion;
      try {
        const runtimeConfig = await loadFrontendRuntimeConfig();
        await waitForMprUIAutoOrchestrationReady();
        if (appVersion !== this.appVersion || lifetime.signal.aborted) return;
        this.runtimeConfig = runtimeConfig;
        if (origin === "recovery") this.authState = AUTH_STATES.LOADING;
        this.clearNotice();
        const authStatus = readMprUIAuthStatus();
        if (authStatus === AUTH_STATES.AUTHENTICATED) {
          await this.loadAuthenticatedApp();
        } else if (authStatus === AUTH_STATES.UNAUTHENTICATED) {
          this.setUnauthenticated();
        }
      } catch (requestError) {
        if (appVersion !== this.appVersion || lifetime.signal.aborted) return;
        this.clearAuthenticatedState();
        this.authState = AUTH_STATES.ERROR;
        this.setPageNotice(NOTICE_KINDS.ERROR, COPY.requestFailed);
        dispatchManagementReady();
        if (requestError instanceof FrontendConfigRateLimitError) {
          bootstrapRateLimit = requestError;
          if (origin === "initial") void this.requestBootstrapRecovery();
        }
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
          await this.loadUsageSummary();
          this.startAutoRefresh();
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
      bootstrapRateLimit = null;
      if (recoveryTimer !== null) {
        window.clearTimeout(recoveryTimer);
        recoveryTimer = null;
      }
      this.appVersion += 1;
      this.stopAutoRefresh();
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
