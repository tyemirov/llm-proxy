// @ts-check

import { AUTH_STATES, COPY, EVENTS, MENU_ACTIONS, NOTICE_KINDS } from "../constants.js";
import {
  BackendClientError,
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

const EMPTY_SECRET_PLACEHOLDER = "<generated-secret>";
const EMPTY_STRING = "";

export function createKeyManagement() {
  return {
    states: {
      loading: AUTH_STATES.LOADING,
      authenticated: AUTH_STATES.AUTHENTICATED,
      unauthenticated: AUTH_STATES.UNAUTHENTICATED,
    },
    copy: COPY,
    authState: AUTH_STATES.LOADING,
    busy: false,
    /** @type {import("../types.d.js").ManagementProfile | null} */
    profile: null,
    /** @type {import("../types.d.js").FrontendRuntimeConfig | null} */
    runtimeConfig: null,
    /** @type {import("../types.d.js").ProviderProfile[]} */
    providers: [],
    /** @type {Record<string, string>} */
    providerInputs: {},
    /** @type {import("../types.d.js").TenantDefaults} */
    defaults: emptyDefaults(),
    /** @type {import("../types.d.js").ManagementUsageSummary} */
    usage: emptyUsageSummary(),
    generatedSecret: EMPTY_STRING,
    settingsOpen: false,
    notice: {
      kind: NOTICE_KINDS.INFO,
      message: EMPTY_STRING,
    },

    init() {
      document.addEventListener(EVENTS.AUTHENTICATED, () => {
        void this.loadProfile();
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

    get usageCurl() {
      const secret = this.generatedSecret || (this.hasSecret ? EMPTY_SECRET_PLACEHOLDER : EMPTY_STRING);
      if (!secret) {
        return EMPTY_STRING;
      }
      const proxyOrigin = this.runtimeConfig ? this.runtimeConfig.proxyOrigin : window.location.origin;
      return [
        `curl --get ${JSON.stringify(`${proxyOrigin}/`)} \\`,
        `  --data-urlencode 'key=${secret}' \\`,
        "  --data-urlencode 'prompt=Hello'",
        "",
        `curl -sS ${JSON.stringify(`${proxyOrigin}/v2?key=${secret}`)} \\`,
        "  -H 'Content-Type: application/json' \\",
        "  --data '{\"messages\":[{\"role\":\"user\",\"content\":\"Hello\"}]}'",
        "",
        `curl -sS -X POST ${JSON.stringify(`${proxyOrigin}/dictate?key=${secret}`)} \\`,
        "  -F 'audio=@recording.webm'",
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
      this.busy = true;
      try {
        const [loadedProfile, loadedUsage] = await Promise.all([fetchProfile(), fetchUsageSummary()]);
        this.applyProfile(loadedProfile);
        this.usage = loadedUsage;
        this.authState = AUTH_STATES.AUTHENTICATED;
        this.setNotice(NOTICE_KINDS.SUCCESS, COPY.profileLoaded);
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

    async refreshUsage() {
      this.busy = true;
      try {
        this.usage = await fetchUsageSummary();
        this.setNotice(NOTICE_KINDS.SUCCESS, COPY.usageRefreshed);
      } catch (requestError) {
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
      if (!customEvent.detail || customEvent.detail.action !== MENU_ACTIONS.OPEN_SETTINGS) {
        return;
      }
      this.openSettings();
    },

    openSettings() {
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

    /**
     * @param {import("../types.d.js").ProviderProfile} provider
     */
    async saveProviderKey(provider) {
      const apiKey = String(this.providerInputs[provider.id] || EMPTY_STRING).trim();
      if (!apiKey) {
        this.setNotice(NOTICE_KINDS.ERROR, COPY.providerMissing);
        return;
      }
      await this.runProfileMutation(async () => requestSaveProviderKey(provider.id, apiKey), COPY.providerKeySaved);
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
    },

    clearAuthenticatedState() {
      this.profile = null;
      this.providers = [];
      this.providerInputs = {};
      this.defaults = emptyDefaults();
      this.usage = emptyUsageSummary();
      this.generatedSecret = EMPTY_STRING;
      this.settingsOpen = false;
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

function dispatchManagementReady() {
  document.dispatchEvent(new CustomEvent(EVENTS.MANAGEMENT_READY));
}

/**
 * @param {number} value
 * @returns {string}
 */
function formatNumber(value) {
  return Number(value || 0).toLocaleString("en-US");
}
