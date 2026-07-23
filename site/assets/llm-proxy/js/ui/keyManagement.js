// @ts-check

import {
  AUTH_STATES,
  COPY,
  DASHBOARD_VIEWS,
  EVENTS,
  MENU_ACTIONS,
  NOTICE_AUTO_DISMISS_MILLISECONDS,
  NOTICE_KINDS,
  ROUTING_DEFAULTS_INVALID_ERROR,
  WORKSPACE_INTEGRITY_ERROR,
} from "../constants.js";
import {
  fetchAdminUsers,
  fetchProfile,
  fetchUsageSummary,
  generateSecret as requestGeneratedSecret,
  loadFrontendRuntimeConfig,
  removeProviderKey as requestRemoveProviderKey,
  revealProviderKey as requestRevealProviderKey,
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
import { applyUserMenuItems, readMprUIAuthStatus, waitForMprUIAutoOrchestrationReady } from "../core/mprShell.js";

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
const MASKED_PROVIDER_KEY_PREFIX = "****";
const MASKED_PROVIDER_KEY_FINAL_CHARACTER_COUNT = 4;
const MASKED_CLIENT_KEY = "••••••••••••";
const SAVED_PROVIDER_KEY_MASK = "saved";

/**
 * @param {string} keyValue
 * @returns {string}
 */
function maskedProviderKey(keyValue) {
  return `${MASKED_PROVIDER_KEY_PREFIX}${keyValue.slice(-MASKED_PROVIDER_KEY_FINAL_CHARACTER_COUNT)}`;
}

export function createKeyManagement() {
  return {
    states: {
      loading: AUTH_STATES.LOADING,
      authenticated: AUTH_STATES.AUTHENTICATED,
      unauthenticated: AUTH_STATES.UNAUTHENTICATED,
      error: AUTH_STATES.ERROR,
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
    providerEditorSession: createProviderEditorSession(EMPTY_STRING, 0),
    providerRemovalConfirmationProviderID: EMPTY_STRING,
    /** @type {import("../types.d.js").TenantDefaults} */
    defaults: emptyDefaults(),
    /** @type {import("../types.d.js").ManagementUsageSummary} */
    usage: emptyUsageSummary(),
    /** @type {import("../types.d.js").ManagementAdminUser[]} */
    adminUsers: [],
    /** @type {Promise<void> | null} */
    profileLoadPromise: null,
    workspaceVersion: 0,
    generatedSecret: EMPTY_STRING,
    generatedSecretVisible: false,
    generatedSecretVersion: 0,
    settingsOpen: false,
    usageExamplesOpen: false,
    notice: {
      kind: NOTICE_KINDS.INFO,
      message: EMPTY_STRING,
    },
    /** @type {number | null} */
    noticeDismissTimerID: null,
    noticeVersion: 0,

    init() {
      document.addEventListener(EVENTS.AUTHENTICATED, () => {
        void this.loadAuthenticatedWorkspace();
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

    get hasSecret() {
      return Boolean(this.profile && this.profile.tenant.has_secret);
    },

    get hasSavedProviderKey() {
      return this.providers.some((provider) => provider.has_key);
    },

    get settingsRequired() {
      return (
        this.authState === AUTH_STATES.AUTHENTICATED &&
        Boolean(this.profile) &&
        (!this.hasSecret || !this.hasSavedProviderKey)
      );
    },

    get settingsRequirementCopy() {
      if (!this.hasSecret && !this.hasSavedProviderKey) {
        return COPY.settingsRequiresClientAndProviderKey;
      }
      if (!this.hasSecret) {
        return COPY.settingsRequiresClientKey;
      }
      return COPY.settingsRequiresProviderKey;
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

    get hasGeneratedSecret() {
      return Boolean(this.generatedSecret);
    },

    get generatedSecretValue() {
      return this.generatedSecretVisible ? this.generatedSecret : MASKED_CLIENT_KEY;
    },

    get generatedSecretVisibilityActionCopy() {
      return this.generatedSecretVisible ? COPY.hideClientKey : COPY.showClientKey;
    },

    get selectedTextModels() {
      const provider = this.providers.find((candidateProvider) => candidateProvider.id === this.defaults.provider);
      return provider ? provider.text_models.map((model) => model.id) : [];
    },

    /** @returns {import("../types.d.js").ProviderProfile | null} */
    get selectedTextProvider() {
      return this.providers.find((candidateProvider) => candidateProvider.id === this.defaults.provider) || null;
    },

    /** @returns {import("../types.d.js").TextModelProfile | null} */
    get selectedTextModel() {
      if (!this.selectedTextProvider) {
        return null;
      }
      return this.selectedTextProvider.text_models.find((model) => model.id === this.defaults.model) || null;
    },

    get reasoningEffortOptions() {
      return this.selectedTextModel && this.selectedTextModel.reasoning_effort ? this.selectedTextModel.reasoning_effort.efforts : [];
    },

    get hasReasoningEffortOptions() {
      return this.reasoningEffortOptions.length > 0;
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

    get selectedProviderID() {
      return this.providerEditorSession.providerID;
    },

    get providerKeyVisible() {
      return this.providerEditorSession.keyVisible;
    },

    get providerKeyRevealPending() {
      return this.providerEditorSession.revealPending;
    },

    get providerRemovalConfirmationOpen() {
      return this.providerRemovalConfirmationProviderID !== EMPTY_STRING;
    },

    get selectedProviderKeyHasInput() {
      return this.providerEditorSession.keyInput !== EMPTY_STRING;
    },

    get selectedProviderKeyInputValue() {
      const provider = this.selectedProvider;
      if (!provider) {
        return EMPTY_STRING;
      }
      const providerKeyInput = this.providerEditorSession.keyInput;
      if (this.providerKeyVisible || (!provider.has_key && !providerKeyInput)) {
        return providerKeyInput;
      }
      const providerMaskedKey = String(provider.masked_key || EMPTY_STRING);
      if (!providerKeyInput && providerMaskedKey === SAVED_PROVIDER_KEY_MASK) {
        return MASKED_PROVIDER_KEY_PREFIX;
      }
      return maskedProviderKey(providerKeyInput || providerMaskedKey);
    },

    get selectedProviderKeyInputReadOnly() {
      const provider = this.selectedProvider;
      if (!provider) {
        return false;
      }
      return Boolean(!this.providerKeyVisible && (provider.has_key || this.selectedProviderKeyHasInput));
    },

    get selectedProviderKeyActionCopy() {
      return this.providerKeyVisible ? COPY.hideProviderKey : COPY.showProviderKey;
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
      return EMPTY_SECRET_PLACEHOLDER;
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
      const secret = this.exampleSecret;
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
        await waitForMprUIAutoOrchestrationReady();
        const authStatus = readMprUIAuthStatus();
        if (authStatus === AUTH_STATES.AUTHENTICATED) {
          await this.loadAuthenticatedWorkspace();
        } else if (authStatus === AUTH_STATES.UNAUTHENTICATED) {
          this.setUnauthenticated();
        }
      } catch (requestError) {
        this.clearAuthenticatedState();
        this.authState = AUTH_STATES.ERROR;
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
        this.profileLoadPromise = null;
      }
    },

    async loadAuthenticatedWorkspace() {
      if (this.authState === AUTH_STATES.AUTHENTICATED || this.authState === AUTH_STATES.ERROR) {
        return;
      }
      this.authState = AUTH_STATES.LOADING;
      await this.loadProfile();
      if (this.authState === AUTH_STATES.LOADING && readMprUIAuthStatus() === AUTH_STATES.AUTHENTICATED) {
        await this.loadProfile();
      }
    },

    async loadProfileOnce() {
      const workspaceVersion = this.workspaceVersion;
      this.busy = true;
      try {
        const loadedProfile = await fetchProfile();
        if (!this.canApplyAuthenticatedWorkspace(workspaceVersion)) {
          return;
        }
        this.applyProfile(loadedProfile);
        this.authState = AUTH_STATES.AUTHENTICATED;
        this.setNotice(NOTICE_KINDS.SUCCESS, COPY.profileLoaded);
        if (this.settingsRequired) {
          this.openSettings();
        }
        if (!this.hasSecret) {
          await this.requestAndApplyGeneratedSecret();
        }
        if (!this.canApplyAuthenticatedWorkspace(workspaceVersion) || this.authState !== AUTH_STATES.AUTHENTICATED) {
          return;
        }
        await this.loadUsageForAuthenticatedProfile();
      } catch (requestError) {
        if (this.canApplyAuthenticatedWorkspace(workspaceVersion)) {
          this.clearAuthenticatedState();
          this.authState = AUTH_STATES.ERROR;
          this.setNotice(NOTICE_KINDS.ERROR, profileFailureMessage(requestError));
        }
      } finally {
        this.busy = false;
        dispatchManagementReady();
      }
    },

    /**
     * @param {number} workspaceVersion
     * @returns {boolean}
     */
    canApplyAuthenticatedWorkspace(workspaceVersion) {
      return (
        this.workspaceVersion === workspaceVersion &&
        readMprUIAuthStatus() === AUTH_STATES.AUTHENTICATED
      );
    },

    setUnauthenticated() {
      if (this.authState === AUTH_STATES.UNAUTHENTICATED) {
        return;
      }
      this.clearAuthenticatedState();
      this.authState = AUTH_STATES.UNAUTHENTICATED;
      this.setNotice(NOTICE_KINDS.INFO, COPY.authenticationRequired);
      dispatchManagementReady();
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
      this.dismissProviderKeyRemovalConfirmation();
      this.settingsOpen = true;
      requestAnimationFrame(() => {
        const entryControl = this.settingsRequired ? this.$refs.settingsRequirement : this.$refs.settingsClose;
        entryControl.focus();
      });
    },

    closeSettings() {
      if (this.settingsRequired) {
        this.setNotice(NOTICE_KINDS.ERROR, this.settingsRequirementCopy);
        this.focusSettingsRequirement();
        return;
      }
      this.dismissProviderKeyRemovalConfirmation();
      this.clearProviderKeyMaterial();
      this.clearGeneratedSecret();
      this.settingsOpen = false;
    },

    focusSettingsRequirement() {
      this.$nextTick(() => {
        requestAnimationFrame(() => {
          this.$refs.settingsRequirement.focus();
        });
      });
    },

    /**
     * @param {KeyboardEvent} event
     */
    trapSettingsFocus(event) {
      if (!this.settingsRequired) {
        return;
      }
      const focusableControls = [...this.$refs.settingsModal.querySelectorAll(
        'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), summary, [tabindex]:not([tabindex="-1"])',
      )].filter((control) => control.getClientRects().length > 0);
      const firstControl = focusableControls[0];
      const lastControl = focusableControls[focusableControls.length - 1];
      if (event.shiftKey && document.activeElement === firstControl) {
        event.preventDefault();
        lastControl.focus();
        return;
      }
      if (!event.shiftKey && document.activeElement === lastControl) {
        event.preventDefault();
        firstControl.focus();
      }
    },

    /**
     * @param {string} providerID
     */
    selectProvider(providerID) {
      profileProvider(this.providers, providerID);
      this.dismissProviderKeyRemovalConfirmation();
      this.replaceProviderEditorSession(providerID);
    },

    async handleSelectedProviderKeyAction() {
      const provider = this.selectedProvider;
      if (!provider) {
        return;
      }
      if (this.selectedProviderKeyHasInput) {
        this.providerEditorSession.keyVisible = !this.providerKeyVisible;
        return;
      }
      if (provider.has_key) {
        await this.revealSelectedProviderKey();
      }
    },

    /**
     * @param {Event} event
     */
    handleSelectedProviderKeyInput(event) {
      const provider = this.selectedProvider;
      if (!provider) {
        return;
      }
      const keyInput = /** @type {HTMLInputElement} */ (event.target);
      this.providerEditorSession.keyInput = keyInput.value;
      this.providerEditorSession.keyVisible = true;
    },

    async revealSelectedProviderKey() {
      const provider = this.selectedProvider;
      if (!provider || !provider.has_key || this.providerKeyRevealPending) {
        return;
      }
      const revealProviderID = provider.id;
      const revealVersion = this.providerEditorSession.revealVersion + 1;
      this.providerEditorSession.revealVersion = revealVersion;
      this.providerEditorSession.revealPending = true;
      try {
        const revealResponse = await requestRevealProviderKey(revealProviderID);
        if (!this.canApplyProviderKeyReveal(revealProviderID, revealVersion)) {
          return;
        }
        this.providerEditorSession.keyInput = revealResponse.api_key;
        this.providerEditorSession.keyVisible = true;
      } catch (requestError) {
        if (this.canApplyProviderKeyReveal(revealProviderID, revealVersion)) {
          this.setNotice(NOTICE_KINDS.ERROR, profileFailureMessage(requestError));
        }
      } finally {
        if (revealVersion === this.providerEditorSession.revealVersion) {
          this.providerEditorSession.revealPending = false;
        }
      }
    },

    /**
     * @param {string} providerID
     * @param {number} revealVersion
     */
    canApplyProviderKeyReveal(providerID, revealVersion) {
      return (
        this.settingsOpen &&
        this.selectedProviderID === providerID &&
        this.providerEditorSession.revealVersion === revealVersion
      );
    },

    clearProviderKeyMaterial() {
      this.replaceProviderEditorSession(this.selectedProviderID);
    },

    /**
     * @param {string} providerID
     */
    replaceProviderEditorSession(providerID) {
      this.providerEditorSession = createProviderEditorSession(
        providerID,
        this.providerEditorSession.revealVersion + 1,
      );
    },

    clearGeneratedSecret() {
      this.generatedSecretVersion += 1;
      this.generatedSecret = EMPTY_STRING;
      this.generatedSecretVisible = false;
    },

    /**
     * @param {number} generatedSecretVersion
     * @returns {boolean}
     */
    canApplyGeneratedSecret(generatedSecretVersion) {
      return (
        this.settingsOpen &&
        this.authState === AUTH_STATES.AUTHENTICATED &&
        this.generatedSecretVersion === generatedSecretVersion
      );
    },

    async saveSelectedProviderKey() {
      if (!this.selectedProvider) {
        return;
      }
      await this.saveProviderKey(this.selectedProvider);
    },

    requestSelectedProviderKeyRemoval() {
      const provider = this.selectedProvider;
      if (!provider) {
        return;
      }
      this.providerRemovalConfirmationProviderID = provider.id;
      this.$nextTick(() => {
        this.$refs.providerRemovalCancel.focus();
      });
    },

    dismissProviderKeyRemovalConfirmation() {
      this.providerRemovalConfirmationProviderID = EMPTY_STRING;
    },

    cancelProviderKeyRemoval() {
      this.dismissProviderKeyRemovalConfirmation();
      this.$nextTick(() => {
        this.$refs.providerKeyRemove.focus();
      });
    },

    async confirmProviderKeyRemoval() {
      const provider = profileProvider(this.providers, this.providerRemovalConfirmationProviderID);
      this.dismissProviderKeyRemovalConfirmation();
      await this.removeProviderKey(provider);
      if (this.settingsRequired) {
        this.focusSettingsRequirement();
        return;
      }
      this.$nextTick(() => {
        this.$refs.providerSelector.focus();
      });
    },

    /**
     * @param {KeyboardEvent} event
     */
    trapProviderKeyRemovalFocus(event) {
      const cancelButton = this.$refs.providerRemovalCancel;
      const confirmButton = this.$refs.providerRemovalConfirm;
      if (event.shiftKey && document.activeElement === cancelButton) {
        event.preventDefault();
        confirmButton.focus();
        return;
      }
      if (!event.shiftKey && document.activeElement === confirmButton) {
        event.preventDefault();
        cancelButton.focus();
      }
    },

    /**
     * @param {import("../types.d.js").ProviderProfile} provider
     */
    async saveProviderKey(provider) {
      const apiKey = this.providerEditorSession.keyInput.trim();
      if (!apiKey && !provider.has_key) {
        this.setNotice(NOTICE_KINDS.ERROR, COPY.providerMissing);
        return;
      }
      try {
        await this.runProfileMutation(
          async () => requestSaveProviderKey(provider.id, apiKey, provider.text_model, provider.system_prompt),
          COPY.providerKeySaved,
        );
      } finally {
        this.clearProviderKeyMaterial();
      }
    },

    /**
     * @param {import("../types.d.js").ProviderProfile} provider
     */
    async removeProviderKey(provider) {
      try {
        await this.runProfileMutation(async () => requestRemoveProviderKey(provider.id), COPY.providerKeyRemoved);
      } finally {
        this.clearProviderKeyMaterial();
      }
    },

    async saveDefaults() {
      await this.runProfileMutation(async () => requestUpdateDefaults(this.defaults), COPY.defaultsSaved);
    },

    async requestAndApplyGeneratedSecret() {
      const generatedSecretVersion = this.generatedSecretVersion;
      try {
        const secretResponse = await requestGeneratedSecret();
        if (!this.canApplyGeneratedSecret(generatedSecretVersion)) {
          return;
        }
        this.generatedSecret = secretResponse.secret;
        this.generatedSecretVisible = false;
        this.applyProfile(secretResponse.profile);
        this.setNotice(NOTICE_KINDS.SUCCESS, COPY.keyGenerated);
      } catch (requestError) {
        if (this.canApplyGeneratedSecret(generatedSecretVersion)) {
          this.setNotice(NOTICE_KINDS.ERROR, profileFailureMessage(requestError));
        }
      }
    },

    async generateSecret() {
      this.busy = true;
      try {
        await this.requestAndApplyGeneratedSecret();
      } finally {
        this.busy = false;
      }
    },

    async revokeSecret() {
      const keyRevoked = await this.runProfileMutation(async () => requestRevokeSecret(), COPY.keyRevoked);
      if (keyRevoked) {
        this.clearGeneratedSecret();
        this.focusSettingsRequirement();
      }
    },

    toggleGeneratedSecretVisibility() {
      if (!this.generatedSecret) {
        return;
      }
      this.generatedSecretVisible = !this.generatedSecretVisible;
    },

    async copyGeneratedSecret() {
      if (!this.generatedSecret || !navigator.clipboard) {
        this.setNotice(NOTICE_KINDS.ERROR, COPY.copyUnavailable);
        return;
      }
      await navigator.clipboard.writeText(this.generatedSecret);
      this.setNotice(NOTICE_KINDS.SUCCESS, COPY.keyCopied);
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
      const provider = profileProvider(this.providers, this.defaults.provider);
      this.defaults.model = provider.text_default_model;
      this.normalizeReasoningEffortDefault();
    },

    normalizeReasoningEffortDefault() {
      if (!this.reasoningEffortOptions.includes(this.defaults.reasoning_effort)) {
        this.defaults.reasoning_effort = EMPTY_STRING;
      }
    },

    selectDictationProviderDefaultModel() {
      const provider = profileProvider(this.providers, this.defaults.dictation_provider);
      if (!provider.supports_dictation || !provider.dictation_default_model) {
        throw new Error(WORKSPACE_INTEGRITY_ERROR);
      }
      this.defaults.dictation_model = provider.dictation_default_model;
    },

    /**
     * @param {() => Promise<import("../types.d.js").ManagementProfile>} mutation
     * @param {string} successMessage
     * @returns {Promise<boolean>}
     */
    async runProfileMutation(mutation, successMessage) {
      this.busy = true;
      try {
        const updatedProfile = await mutation();
        this.applyProfile(updatedProfile);
        this.setNotice(NOTICE_KINDS.SUCCESS, successMessage);
        return true;
      } catch (requestError) {
        this.setNotice(NOTICE_KINDS.ERROR, profileFailureMessage(requestError));
        return false;
      } finally {
        this.busy = false;
      }
    },

    /**
     * @param {import("../types.d.js").ManagementProfile} nextProfile
     */
    applyProfile(nextProfile) {
      const defaults = createWorkspaceRoutingDefaults(nextProfile);
      const selectedProviderID = this.selectedProviderID;
      this.dismissProviderKeyRemovalConfirmation();
      this.profile = nextProfile;
      applyUserMenuItems(Boolean(nextProfile.user.is_admin));
      this.providers = nextProfile.providers;
      this.defaults.provider = defaults.provider;
      this.defaults.model = defaults.model;
      this.defaults.dictation_provider = defaults.dictation_provider;
      this.defaults.dictation_model = defaults.dictation_model;
      this.defaults.system_prompt = defaults.system_prompt;
      this.defaults.reasoning_effort = defaults.reasoning_effort;
      const nextProviderID = this.providers.some((provider) => provider.id === selectedProviderID)
        ? selectedProviderID
        : defaults.provider;
      this.replaceProviderEditorSession(nextProviderID);
      this.$nextTick(() => {
        this.defaults = { ...defaults };
      });
    },

    clearAuthenticatedState() {
      this.workspaceVersion += 1;
      this.clearNotice();
      this.dismissProviderKeyRemovalConfirmation();
      this.profile = null;
      this.providers = [];
      this.replaceProviderEditorSession(EMPTY_STRING);
      this.defaults = emptyDefaults();
      this.usage = emptyUsageSummary();
      this.adminUsers = [];
      this.clearGeneratedSecret();
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
      this.clearNotice();
      this.notice = { kind, message };
      if (message === EMPTY_STRING) {
        return;
      }
      const noticeVersion = this.noticeVersion + 1;
      this.noticeVersion = noticeVersion;
      this.noticeDismissTimerID = window.setTimeout(() => {
        if (this.noticeVersion !== noticeVersion) {
          return;
        }
        this.clearNotice();
      }, NOTICE_AUTO_DISMISS_MILLISECONDS);
    },

    clearNotice() {
      this.noticeVersion += 1;
      if (this.noticeDismissTimerID !== null) {
        window.clearTimeout(this.noticeDismissTimerID);
        this.noticeDismissTimerID = null;
      }
      this.notice = { kind: NOTICE_KINDS.INFO, message: EMPTY_STRING };
    },
  };
}

/**
 * @param {string} providerID
 * @param {number} revealVersion
 * @returns {{ providerID: string, keyInput: string, keyVisible: boolean, revealPending: boolean, revealVersion: number }}
 */
function createProviderEditorSession(providerID, revealVersion) {
  return {
    providerID,
    keyInput: EMPTY_STRING,
    keyVisible: false,
    revealPending: false,
    revealVersion,
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
    reasoning_effort: EMPTY_STRING,
  };
}

/**
 * @param {import("../types.d.js").ManagementProfile} profile
 * @returns {import("../types.d.js").TenantDefaults}
 */
function createWorkspaceRoutingDefaults(profile) {
  const tenant = profile && typeof profile.tenant === "object" ? profile.tenant : null;
  const defaults = tenant && typeof tenant.defaults === "object" ? tenant.defaults : null;
  const providers = Array.isArray(profile && profile.providers) ? profile.providers : null;
  if (!defaults || !providers || !routingDefaultsAreStrings(defaults) || Object.hasOwn(profile, "reasoning_effort_options")) {
    throw new Error(WORKSPACE_INTEGRITY_ERROR);
  }
  for (const provider of providers) {
    assertProviderCatalog(provider);
  }
  const textProvider = profileProvider(providers, defaults.provider);
  const textModel = textProvider.text_models.find((model) => model.id === defaults.model);
  if (!textModel) {
    throw new Error(WORKSPACE_INTEGRITY_ERROR);
  }
  const dictationProvider = profileProvider(providers, defaults.dictation_provider);
  if (!dictationProvider.supports_dictation || !dictationProvider.dictation_models.includes(defaults.dictation_model)) {
    throw new Error(WORKSPACE_INTEGRITY_ERROR);
  }
  if (defaults.reasoning_effort !== EMPTY_STRING && !reasoningEffortOptionsForTextModel(textModel).includes(defaults.reasoning_effort)) {
    throw new Error(WORKSPACE_INTEGRITY_ERROR);
  }
  return {
    provider: defaults.provider,
    model: defaults.model,
    dictation_provider: defaults.dictation_provider,
    dictation_model: defaults.dictation_model,
    system_prompt: defaults.system_prompt,
    reasoning_effort: defaults.reasoning_effort,
  };
}

/**
 * @param {import("../types.d.js").TenantDefaults} defaults
 * @returns {boolean}
 */
function routingDefaultsAreStrings(defaults) {
  return (
    typeof defaults.provider === "string" &&
    typeof defaults.model === "string" &&
    typeof defaults.dictation_provider === "string" &&
    typeof defaults.dictation_model === "string" &&
    typeof defaults.system_prompt === "string" &&
    typeof defaults.reasoning_effort === "string"
  );
}

/**
 * @param {import("../types.d.js").ProviderProfile} provider
 * @returns {void}
 */
function assertProviderCatalog(provider) {
  if (
    !provider ||
    typeof provider.id !== "string" ||
    !provider.id ||
    !Array.isArray(provider.text_models) ||
    !provider.text_models.some((model) => model && model.id === provider.text_default_model)
  ) {
    throw new Error(WORKSPACE_INTEGRITY_ERROR);
  }
  if (Object.hasOwn(provider, "reasoning_effort")) {
    throw new Error(WORKSPACE_INTEGRITY_ERROR);
  }
  for (const model of provider.text_models) {
    if (!model || typeof model.id !== "string" || !model.id) {
      throw new Error(WORKSPACE_INTEGRITY_ERROR);
    }
    assertReasoningEffortCapability(model.reasoning_effort);
  }
  if (
    provider.supports_dictation &&
    (!Array.isArray(provider.dictation_models) ||
      typeof provider.dictation_default_model !== "string" ||
      !provider.dictation_models.includes(provider.dictation_default_model))
  ) {
    throw new Error(WORKSPACE_INTEGRITY_ERROR);
  }
}

/**
 * @param {import("../types.d.js").TextModelProfile} model
 * @returns {string[]}
 */
function reasoningEffortOptionsForTextModel(model) {
  return model.reasoning_effort ? model.reasoning_effort.efforts : [];
}

/**
 * @param {unknown} capability
 * @returns {void}
 */
function assertReasoningEffortCapability(capability) {
  if (capability === undefined) {
    return;
  }
  if (
    !capability ||
    typeof capability.adapter !== "string" ||
    capability.adapter === EMPTY_STRING ||
    capability.adapter !== capability.adapter.trim() ||
    !Array.isArray(capability.efforts) ||
    capability.efforts.length === 0 ||
    new Set(capability.efforts).size !== capability.efforts.length ||
    !capability.efforts.every((effort) => typeof effort === "string" && effort !== EMPTY_STRING && effort === effort.trim())
  ) {
    throw new Error(WORKSPACE_INTEGRITY_ERROR);
  }
}

/**
 * @param {import("../types.d.js").ProviderProfile[]} providers
 * @param {string} providerID
 * @returns {import("../types.d.js").ProviderProfile}
 */
function profileProvider(providers, providerID) {
  const provider = providers.find((candidateProvider) => candidateProvider.id === providerID);
  if (!provider) {
    throw new Error(WORKSPACE_INTEGRITY_ERROR);
  }
  return provider;
}

/**
 * @param {unknown} requestError
 * @returns {string}
 */
function profileFailureMessage(requestError) {
  if (
    requestError instanceof Error &&
    (requestError.message === WORKSPACE_INTEGRITY_ERROR || requestError.message.includes(ROUTING_DEFAULTS_INVALID_ERROR))
  ) {
    return COPY.workspaceIntegrityError;
  }
  return COPY.requestFailed;
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
