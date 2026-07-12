// @ts-check

import { AUTH_STATES, COPY, DASHBOARD_VIEWS, EVENTS, MENU_ACTIONS, NOTICE_KINDS } from "../constants.js";
import {
  BackendClientError,
  fetchAdminUsers,
  fetchProfile,
  fetchUsageSummary,
  generateSecret as requestGeneratedSecret,
  loadFrontendRuntimeConfig,
  removeProviderKey as requestRemoveProviderKey,
  revokeSecret as requestRevokeSecret,
  saveProviderKey as requestSaveProviderKey,
  updateDefaults as requestUpdateDefaults,
} from "../core/backendClient.js";
import {
  emptyUsageSummary,
  modelRows,
  providerRows,
  successRateLabel,
  usagePolyline,
  USAGE_CHART,
  USAGE_METRICS,
} from "./usagePresentation.js";
import { applyUserMenuItems, waitForMprUIAutoOrchestrationReady } from "../core/mprShell.js";

const EMPTY_SECRET_PLACEHOLDER = "<generated-secret>";
const EMPTY_STRING = "";
const DEFAULT_TEXT_EXAMPLE_ID = "default-text";
const DEFAULT_V2_EXAMPLE_ID = "default-v2";
const DEFAULT_DICTATION_EXAMPLE_ID = "default-dictation";
const PROVIDER_TEXT_EXAMPLE_ID = "provider-text";
const PROVIDER_V2_EXAMPLE_ID = "provider-v2";
const PROVIDER_DICTATION_EXAMPLE_ID = "provider-dictation";
const JSON_CONTENT_TYPE_HEADER = "Content-Type: application/json";
const SAMPLE_TEXT_PROMPT = "Hello";
const SAMPLE_AUDIO_FILE = "recording.webm";

export function createKeyManagement() {
  return {
    states: {
      loading: AUTH_STATES.LOADING,
      authenticated: AUTH_STATES.AUTHENTICATED,
      unauthenticated: AUTH_STATES.UNAUTHENTICATED,
    },
    dashboardViews: DASHBOARD_VIEWS,
    copy: COPY,
    authState: AUTH_STATES.LOADING,
    busy: false,
    dashboardView: DASHBOARD_VIEWS.USAGE,
    /** @type {import("../types.d.js").ManagementProfile | null} */
    profile: null,
    /** @type {import("../types.d.js").FrontendRuntimeConfig | null} */
    runtimeConfig: null,
    /** @type {import("../types.d.js").ProviderProfile[]} */
    providers: [],
    selectedProviderID: EMPTY_STRING,
    /** @type {Record<string, string>} */
    providerInputs: {},
    /** @type {import("../types.d.js").TenantDefaults} */
    defaults: emptyDefaults(),
    /** @type {import("../types.d.js").ManagementUsageSummary} */
    usage: emptyUsageSummary(),
    /** @type {import("../types.d.js").ManagementAdminUser[]} */
    adminUsers: [],
    /** @type {Promise<void> | null} */
    profileLoadPromise: null,
    authenticatedShellProfileRequested: false,
    generatedSecret: EMPTY_STRING,
    settingsOpen: false,
    usageExamplesOpen: false,
    notice: {
      kind: NOTICE_KINDS.INFO,
      message: EMPTY_STRING,
    },

    init() {
      document.addEventListener(EVENTS.AUTHENTICATED, () => {
        this.loadProfileForAuthenticatedShell();
      });
      document.addEventListener(EVENTS.UNAUTHENTICATED, () => {
        this.clearAuthenticatedState();
        this.authState = AUTH_STATES.UNAUTHENTICATED;
        dispatchManagementReady();
      });
      document.addEventListener(EVENTS.USER_MENU_ITEM, (event) => {
        this.handleUserMenuItem(event);
      });
      void this.start();
    },

    get hasSecret() {
      return Boolean(this.profile && this.profile.tenant.has_secret);
    },

    get isAdmin() {
      return Boolean(this.profile && this.profile.user.is_admin);
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

    get hasAdminUsers() {
      return this.adminUsers.length > 0;
    },

    get tenantId() {
      if (!this.profile) {
        return EMPTY_STRING;
      }
      return this.profile.tenant.id;
    },

    get selectedTextModels() {
      const provider = this.providers.find((candidateProvider) => candidateProvider.id === this.defaults.provider);
      return provider ? provider.text_models : [];
    },

    get dictationProviders() {
      return this.providers.filter((provider) => provider.supports_dictation);
    },

    get selectedDictationModels() {
      const provider = this.providers.find((candidateProvider) => candidateProvider.id === this.defaults.dictation_provider);
      return provider ? provider.dictation_models : [];
    },

    /** @returns {import("../types.d.js").ProviderProfile | null} */
    get selectedProvider() {
      return this.providers.find((candidateProvider) => candidateProvider.id === this.selectedProviderID) || null;
    },

    get chartViewBox() {
      return `0 0 ${USAGE_CHART.width} ${USAGE_CHART.height}`;
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
      return formatNumber(this.usage.providers.length);
    },

    get usageRequestPolyline() {
      return usagePolyline(this.usage, USAGE_METRICS.REQUESTS);
    },

    get usageTokenPolyline() {
      return usagePolyline(this.usage, USAGE_METRICS.TOTAL_TOKENS);
    },

    get providerUsageRows() {
      return providerRows(this.usage);
    },

    get modelUsageRows() {
      return modelRows(this.usage);
    },

    get requestExamples() {
      const defaultExamples = [
        createRequestExample(DEFAULT_TEXT_EXAMPLE_ID, COPY.defaultTextExample, this.defaultTextCurl()),
        createRequestExample(DEFAULT_V2_EXAMPLE_ID, COPY.defaultV2Example, this.defaultV2Curl()),
        createRequestExample(DEFAULT_DICTATION_EXAMPLE_ID, COPY.defaultDictationExample, this.defaultDictationCurl()),
      ];
      if (!this.selectedProvider) {
        return defaultExamples;
      }
      const providerExamples = [
        createRequestExample(
          PROVIDER_TEXT_EXAMPLE_ID,
          `${this.selectedProvider.label}${COPY.providerTextExampleSuffix}`,
          this.providerTextCurl(this.selectedProvider),
        ),
        createRequestExample(
          PROVIDER_V2_EXAMPLE_ID,
          `${this.selectedProvider.label}${COPY.providerV2ExampleSuffix}`,
          this.providerV2Curl(this.selectedProvider),
        ),
      ];
      if (this.selectedProvider.supports_dictation) {
        providerExamples.push(
          createRequestExample(
            PROVIDER_DICTATION_EXAMPLE_ID,
            `${this.selectedProvider.label}${COPY.providerDictationExampleSuffix}`,
            this.providerDictationCurl(this.selectedProvider),
          ),
        );
      }
      return [...defaultExamples, ...providerExamples];
    },

    get exampleSecret() {
      return this.generatedSecret || EMPTY_SECRET_PLACEHOLDER;
    },

    get proxyOrigin() {
      return this.runtimeConfig ? this.runtimeConfig.proxyOrigin : window.location.origin;
    },

    defaultTextCurl() {
      return [
        `curl --get ${JSON.stringify(`${this.proxyOrigin}/`)} \\`,
        `  --data-urlencode 'key=${this.exampleSecret}' \\`,
        `  --data-urlencode 'prompt=${SAMPLE_TEXT_PROMPT}'`,
      ].join("\n");
    },

    defaultV2Curl() {
      const secret = this.generatedSecret || EMPTY_SECRET_PLACEHOLDER;
      return [
        `curl -sS ${JSON.stringify(`${this.proxyOrigin}/v2?key=${secret}`)} \\`,
        `  -H '${JSON_CONTENT_TYPE_HEADER}' \\`,
        `  --data '${JSON.stringify({ messages: [{ role: "user", content: SAMPLE_TEXT_PROMPT }] })}'`,
      ].join("\n");
    },

    defaultDictationCurl() {
      return [
        `curl -sS -X POST ${JSON.stringify(`${this.proxyOrigin}/dictate?key=${this.exampleSecret}`)} \\`,
        `  -F 'audio=@${SAMPLE_AUDIO_FILE}'`,
      ].join("\n");
    },

    /**
     * @param {import("../types.d.js").ProviderProfile} provider
     * @returns {string}
     */
    providerTextCurl(provider) {
      return [
        `curl --get ${JSON.stringify(`${this.proxyOrigin}/`)} \\`,
        `  --data-urlencode 'key=${this.exampleSecret}' \\`,
        `  --data-urlencode 'provider=${provider.id}' \\`,
        `  --data-urlencode 'model=${provider.text_model}' \\`,
        `  --data-urlencode 'prompt=${SAMPLE_TEXT_PROMPT}'`,
      ].join("\n");
    },

    /**
     * @param {import("../types.d.js").ProviderProfile} provider
     * @returns {string}
     */
    providerV2Curl(provider) {
      const requestBody = {
        messages: [{ role: "user", content: SAMPLE_TEXT_PROMPT }],
        model: provider.text_model,
      };
      return [
        `curl -sS ${JSON.stringify(`${this.proxyOrigin}/v2?key=${this.exampleSecret}&provider=${provider.id}`)} \\`,
        `  -H '${JSON_CONTENT_TYPE_HEADER}' \\`,
        `  --data '${JSON.stringify(requestBody)}'`,
      ].join("\n");
    },

    /**
     * @param {import("../types.d.js").ProviderProfile} provider
     * @returns {string}
     */
    providerDictationCurl(provider) {
      return [
        `curl -sS -X POST ${JSON.stringify(`${this.proxyOrigin}/dictate?key=${this.exampleSecret}&provider=${provider.id}`)} \\`,
        `  -F 'audio=@${SAMPLE_AUDIO_FILE}'`,
      ].join("\n");
    },

    async start() {
      try {
        this.runtimeConfig = await loadFrontendRuntimeConfig();
        await this.loadProfile();
      } catch (requestError) {
        this.clearAuthenticatedState();
        this.authState = AUTH_STATES.UNAUTHENTICATED;
        this.setNotice(NOTICE_KINDS.ERROR, COPY.requestFailed);
        dispatchManagementReady();
      }
    },

    async loadProfile() {
      if (this.profileLoadPromise) {
        return this.profileLoadPromise;
      }
      this.profileLoadPromise = this.loadProfileOnce();
      try {
        await this.profileLoadPromise;
      } finally {
        const retryRequested = this.authState === AUTH_STATES.UNAUTHENTICATED && this.authenticatedShellProfileRequested;
        this.profileLoadPromise = null;
        this.authenticatedShellProfileRequested = false;
        if (retryRequested) {
          await this.loadProfile();
        }
      }
    },

    async loadProfileOnce() {
      this.busy = true;
      try {
        const loadedProfile = await fetchProfile();
        this.applyProfile(loadedProfile);
        this.authState = AUTH_STATES.AUTHENTICATED;
        this.setNotice(NOTICE_KINDS.SUCCESS, COPY.profileLoaded);
        await this.loadUsageForAuthenticatedProfile();
      } catch (requestError) {
        if (requestError instanceof BackendClientError && requestError.status === 401) {
          this.clearAuthenticatedState();
          this.authState = AUTH_STATES.UNAUTHENTICATED;
          this.setNotice(NOTICE_KINDS.INFO, COPY.authenticationRequired);
        } else {
          this.setNotice(NOTICE_KINDS.ERROR, COPY.requestFailed);
        }
      } finally {
        this.busy = false;
        dispatchManagementReady();
      }
    },

    loadProfileForAuthenticatedShell() {
      if (this.profileLoadPromise) {
        this.authenticatedShellProfileRequested = true;
        return;
      }
      void this.loadProfile();
    },

    async loadUsageForAuthenticatedProfile() {
      try {
        this.usage = await fetchUsageSummary();
      } catch {
        this.usage = emptyUsageSummary();
        this.setNotice(NOTICE_KINDS.ERROR, COPY.requestFailed);
      }
    },

    async refreshDashboard() {
      if (this.dashboardView === DASHBOARD_VIEWS.ADMIN) {
        await this.refreshAdminUsers();
        return;
      }
      await this.refreshUsage();
    },

    async refreshUsage() {
      this.busy = true;
      try {
        this.usage = await fetchUsageSummary();
        this.setNotice(NOTICE_KINDS.SUCCESS, COPY.usageRefreshed);
      } catch (requestError) {
        this.usage = emptyUsageSummary();
        this.setNotice(NOTICE_KINDS.ERROR, COPY.requestFailed);
      } finally {
        this.busy = false;
      }
    },

    async refreshAdminUsers() {
      if (!this.isAdmin) {
        return;
      }
      this.busy = true;
      try {
        const adminUsersResponse = await fetchAdminUsers();
        this.adminUsers = adminUsersResponse.users;
        this.setNotice(NOTICE_KINDS.SUCCESS, COPY.usageRefreshed);
      } catch (requestError) {
        this.adminUsers = [];
        this.setNotice(NOTICE_KINDS.ERROR, COPY.requestFailed);
      } finally {
        this.busy = false;
      }
    },

    /**
     * @param {Event} event
     */
    handleUserMenuItem(event) {
      const customEvent = /** @type {CustomEvent<{ action?: string }>} */ (event);
      if (!customEvent.detail) {
        return;
      }
      if (customEvent.detail.action === MENU_ACTIONS.OPEN_ADMIN) {
        void this.openAdminDashboard();
      }
      if (customEvent.detail.action === MENU_ACTIONS.OPEN_SETTINGS) {
        this.openSettings();
      }
    },

    async openAdminDashboard() {
      if (!this.isAdmin) {
        return;
      }
      this.dashboardView = DASHBOARD_VIEWS.ADMIN;
      await this.refreshAdminUsers();
    },

    openUsageDashboard() {
      this.dashboardView = DASHBOARD_VIEWS.USAGE;
    },

    openSettings() {
      this.usageExamplesOpen = false;
      this.settingsOpen = true;
      requestAnimationFrame(() => {
        if (this.$refs.settingsClose) {
          this.$refs.settingsClose.focus();
        }
      });
    },

    closeSettings() {
      this.settingsOpen = false;
    },

    async saveSelectedProviderKey() {
      if (!this.selectedProvider) {
        return;
      }
      await this.saveProviderKey(this.selectedProvider);
    },

    async removeSelectedProviderKey() {
      if (!this.selectedProvider) {
        return;
      }
      await this.removeProviderKey(this.selectedProvider);
    },

    /**
     * @param {import("../types.d.js").ProviderProfile} provider
     */
    async saveProviderKey(provider) {
      const apiKey = String(this.providerInputs[provider.id] || EMPTY_STRING).trim();
      if (!apiKey && !provider.has_key) {
        this.setNotice(NOTICE_KINDS.ERROR, COPY.providerMissing);
        return;
      }
      await this.runProfileMutation(
        async () => requestSaveProviderKey(provider.id, apiKey, provider.text_model, provider.system_prompt),
        COPY.providerKeySaved,
      );
      this.providerInputs[provider.id] = EMPTY_STRING;
    },

    /**
     * @param {import("../types.d.js").ProviderProfile} provider
     */
    async removeProviderKey(provider) {
      await this.runProfileMutation(async () => requestRemoveProviderKey(provider.id), COPY.providerKeyRemoved);
    },

    async saveDefaults() {
      await this.runProfileMutation(async () => requestUpdateDefaults(this.defaults), COPY.defaultsSaved);
    },

    async generateSecret() {
      this.busy = true;
      try {
        const secretResponse = await requestGeneratedSecret();
        this.generatedSecret = secretResponse.secret;
        this.applyProfile(secretResponse.profile);
        this.setNotice(NOTICE_KINDS.SUCCESS, COPY.secretGenerated);
      } catch (requestError) {
        this.setNotice(NOTICE_KINDS.ERROR, COPY.requestFailed);
      } finally {
        this.busy = false;
      }
    },

    async revokeSecret() {
      await this.runProfileMutation(async () => requestRevokeSecret(), COPY.secretRevoked);
      this.generatedSecret = EMPTY_STRING;
    },

    async copyGeneratedSecret() {
      if (!this.generatedSecret || !navigator.clipboard) {
        this.setNotice(NOTICE_KINDS.ERROR, COPY.copyUnavailable);
        return;
      }
      await navigator.clipboard.writeText(this.generatedSecret);
      this.setNotice(NOTICE_KINDS.SUCCESS, COPY.secretCopied);
    },

    /**
     * @param {string} command
     */
    async copyRequestExample(command) {
      if (!navigator.clipboard) {
        this.setNotice(NOTICE_KINDS.ERROR, COPY.copyUnavailable);
        return;
      }
      await navigator.clipboard.writeText(command);
      this.setNotice(NOTICE_KINDS.SUCCESS, COPY.exampleCopied);
    },

    selectTextProviderDefaultModel() {
      const provider = this.providers.find((candidateProvider) => candidateProvider.id === this.defaults.provider);
      this.defaults.model = provider ? provider.text_default_model : EMPTY_STRING;
    },

    selectDictationProviderDefaultModel() {
      const provider = this.providers.find((candidateProvider) => candidateProvider.id === this.defaults.dictation_provider);
      this.defaults.dictation_model = provider ? provider.dictation_default_model || EMPTY_STRING : EMPTY_STRING;
    },

    /**
     * @param {() => Promise<import("../types.d.js").ManagementProfile>} mutation
     * @param {string} successMessage
     */
    async runProfileMutation(mutation, successMessage) {
      this.busy = true;
      try {
        const updatedProfile = await mutation();
        this.applyProfile(updatedProfile);
        this.setNotice(NOTICE_KINDS.SUCCESS, successMessage);
      } catch (requestError) {
        this.setNotice(NOTICE_KINDS.ERROR, COPY.requestFailed);
      } finally {
        this.busy = false;
      }
    },

    /**
     * @param {import("../types.d.js").ManagementProfile} nextProfile
     */
    applyProfile(nextProfile) {
      this.profile = nextProfile;
      applyUserMenuItems(Boolean(nextProfile.user.is_admin));
      this.providers = nextProfile.providers;
      this.defaults = {
        provider: nextProfile.tenant.defaults.provider,
        model: nextProfile.tenant.defaults.model,
        dictation_provider: nextProfile.tenant.defaults.dictation_provider,
        dictation_model: nextProfile.tenant.defaults.dictation_model,
        system_prompt: nextProfile.tenant.defaults.system_prompt,
      };
      for (const provider of this.providers) {
        if (this.providerInputs[provider.id] === undefined) {
          this.providerInputs[provider.id] = EMPTY_STRING;
        }
      }
      if (!this.providers.find((provider) => provider.id === this.selectedProviderID)) {
        const defaultProvider = this.providers.find((provider) => provider.id === nextProfile.tenant.defaults.provider);
        this.selectedProviderID = defaultProvider ? defaultProvider.id : (this.providers[0] ? this.providers[0].id : EMPTY_STRING);
      }
    },

    clearAuthenticatedState() {
      this.profile = null;
      this.providers = [];
      this.selectedProviderID = EMPTY_STRING;
      this.providerInputs = {};
      this.defaults = emptyDefaults();
      this.usage = emptyUsageSummary();
      this.adminUsers = [];
      this.generatedSecret = EMPTY_STRING;
      this.settingsOpen = false;
      this.dashboardView = DASHBOARD_VIEWS.USAGE;
      applyUserMenuItems(false);
    },

    /**
     * @param {import("../types.d.js").ManagementAdminUser} adminUser
     * @returns {string}
     */
    adminUserLabel(adminUser) {
      return adminUser.user.email || adminUser.user.display_name || adminUser.user.id || COPY.adminUserFallback;
    },

    /**
     * @param {import("../types.d.js").ManagementAdminUser} adminUser
     * @returns {string}
     */
    adminUserRequests(adminUser) {
      return formatNumber(adminUser.usage.totals.requests);
    },

    /**
     * @param {import("../types.d.js").ManagementAdminUser} adminUser
     * @returns {string}
     */
    adminUserTokens(adminUser) {
      return formatNumber(adminUser.usage.totals.total_tokens);
    },

    /**
     * @param {import("../types.d.js").ManagementAdminUser} adminUser
     * @returns {string}
     */
    adminUserSuccessRate(adminUser) {
      return successRateLabel(adminUser.usage.totals);
    },

    /**
     * @param {string} kind
     * @param {string} message
     */
    setNotice(kind, message) {
      this.notice = { kind, message };
    },
  };
}

/**
 * @returns {import("../types.d.js").TenantDefaults}
 */
function emptyDefaults() {
  return {
    provider: EMPTY_STRING,
    model: EMPTY_STRING,
    dictation_provider: EMPTY_STRING,
    dictation_model: EMPTY_STRING,
    system_prompt: EMPTY_STRING,
  };
}

async function dispatchManagementReady() {
  await waitForMprUIAutoOrchestrationReady();
  document.dispatchEvent(new CustomEvent(EVENTS.MANAGEMENT_READY));
}

/**
 * @param {string} id
 * @param {string} title
 * @param {string} command
 * @returns {import("../types.d.js").RequestExample}
 */
function createRequestExample(id, title, command) {
  return { id, title, command };
}

/**
 * @param {number} value
 * @returns {string}
 */
function formatNumber(value) {
  return Number(value || 0).toLocaleString("en-US");
}
