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
} from "../constants.js?v=20260902c239";
import {
  fetchAccount,
  fetchTenant,
  loadFrontendRuntimeConfig,
} from "../core/backendClient.js?v=20260902c239";
import {
  assertManagementAccount,
  assertManagementTenantProfile,
  emptyDefaults,
  isAbortError,
  profileFailureMessage,
} from "../core/managementProfile.js?v=20260902c239";
import {
  applyUserMenuItems,
  readMprUIAuthStatus,
  waitForMprUIAutoOrchestrationReady,
} from "../core/mprShell.js?v=20260902c239";
import { dispatchManagementReady } from "../core/runtimeTransition.js?v=20260902c239";

const EMPTY_STRING = "";

/** @typedef {ReturnType<typeof import("./managementApplicationState.js").createManagementApplicationState>} ManagementApplicationState */
/** @typedef {ManagementApplicationState & {
 *   settingsRequired: boolean,
 *   hasSecret: boolean,
 *   applyProfile: (profile: import("../types.d.js").ManagementTenantProfile, preserveProviderEditor?: boolean, preserveRoutingDefaults?: boolean) => void,
 *   clearGeneratedSecret: () => void,
 *   clearNotice: () => void,
 *   clearUsageState: () => void,
 *   collapseSystemPromptEditors: () => void,
 *   dismissClientKeyReplacementConfirmation: () => void,
 *   dismissProviderKeyRemovalConfirmation: () => void,
 *   handleUserMenuItem: (event: Event) => void,
 *   loadUsageSummary: (showSuccessNotice: boolean) => Promise<void>,
 *   openSettings: () => void,
 *   replaceProviderEditorSession: (providerID: string) => void,
 *   replaceTenantLifetimeController: () => void,
 *   resetProviderCard: () => void,
 *   requestAndApplyGeneratedSecret: (successMessage?: string) => Promise<boolean>,
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
        assertManagementAccount(loadedAccount);
        this.account = loadedAccount;
        this.tenants = loadedAccount.tenants;
        applyUserMenuItems(Boolean(loadedAccount.user.is_admin));
        this.settingsTenantID = this.tenants[0].id;
        this.replaceTenantLifetimeController();
        await this.hydrateSettingsTenant(null, appVersion, NOTICE_SURFACES.HEADER);
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
     * @param {import("../types.d.js").ManagementTenantProfile | null} prefetchedProfile
     * @param {number} appVersion
     * @param {string} noticeSurface
     */
    async hydrateSettingsTenant(prefetchedProfile, appVersion, noticeSurface) {
      const tenantID = this.settingsTenantID;
      if (this.tenantRequestController) {
        this.tenantRequestController.abort();
      }
      const tenantRequestController = new AbortController();
      this.tenantRequestController = tenantRequestController;
      this.clearSettingsTenantState();
      try {
        const loadedProfile = prefetchedProfile || await fetchTenant(tenantID, tenantRequestController.signal);
        if (!this.canApplySettingsTenant(appVersion, tenantID)) {
          return;
        }
        assertManagementTenantProfile(loadedProfile, tenantID);
        this.applyProfile(loadedProfile);
        this.authState = AUTH_STATES.AUTHENTICATED;
        this.setNotice(NOTICE_KINDS.SUCCESS, COPY.profileLoaded, noticeSurface);
        if (this.settingsRequired) {
          this.openSettings();
        }
        if (!this.hasSecret) {
          await this.requestAndApplyGeneratedSecret();
        }
      } catch (requestError) {
        if (!isAbortError(requestError) && this.canApplySettingsTenant(appVersion, tenantID)) {
          this.clearSettingsTenantState();
          if (this.authState !== AUTH_STATES.AUTHENTICATED) {
            this.authState = AUTH_STATES.ERROR;
          }
          this.setNotice(NOTICE_KINDS.ERROR, profileFailureMessage(requestError), noticeSurface);
        }
      } finally {
        if (this.tenantRequestController === tenantRequestController) {
          this.tenantRequestController = null;
        }
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

    clearSettingsTenantState() {
      this.profileApplicationVersion += 1;
      this.providerAutosavePromise = null;
      this.providerAutosavePending = false;
      this.routingDefaultsAutosavePromise = null;
      this.routingDefaultsAutosavePending = false;
      this.routingDefaultsDirty = false;
      this.routingDefaultsEditVersion += 1;
      this.routingProviderSelectionPending = false;
      this.routingProviderSelectionVersion += 1;
      this.clientKeyMutationPromise = null;
      this.profileMutationTail = Promise.resolve();
      this.profileMutationFailureVersion = 0;
      this.settingsClosePending = false;
      this.dismissProviderKeyRemovalConfirmation();
      this.dismissClientKeyReplacementConfirmation();
      this.clientKeyReplacementPending = false;
      this.profile = null;
      this.providers = [];
      this.replaceProviderEditorSession(EMPTY_STRING);
      this.defaults = emptyDefaults();
      this.clearGeneratedSecret();
      this.tenantNameDraft = EMPTY_STRING;
      this.tenantRenameDialogOpen = false;
      this.tenantNameDirty = false;
      this.tenantNameError = EMPTY_STRING;
      this.usageExamplesOpen = false;
      this.collapseSystemPromptEditors();
    },

    clearAuthenticatedState() {
      this.appVersion += 1;
      this.resetProviderCard();
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
      if (this.usageFailuresRequestController) {
        this.usageFailuresRequestController.abort();
        this.usageFailuresRequestController = null;
      }
      if (this.tenantLifetimeController) {
        this.tenantLifetimeController.abort();
        this.tenantLifetimeController = null;
      }
      this.selectedUsageInterval = DEFAULT_USAGE_INTERVAL;
      this.selectedUsageTenantID = EMPTY_STRING;
      this.usageBreakdownView = "bar";
      this.clearNotice();
      this.clearSettingsTenantState();
      this.clearUsageState();
      this.account = null;
      this.tenants = [];
      this.settingsTenantID = EMPTY_STRING;
      this.adminUsers = [];
      this.settingsOpen = false;
      this.createTenantDialogOpen = false;
      this.createTenantName = EMPTY_STRING;
      this.createTenantError = EMPTY_STRING;
      this.createTenantPending = false;
      this.deleteTenantConfirmationOpen = false;
      this.deleteTenantPending = false;
      this.discardTenantChangesOpen = false;
      this.pendingTenantID = EMPTY_STRING;
      this.dashboardView = DASHBOARD_VIEWS.USAGE;
      applyUserMenuItems(false);
    },
  });
}
