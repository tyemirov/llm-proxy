// @ts-check

import {
  AUTH_STATES,
  COPY,
  DASHBOARD_VIEWS,
  DEFAULT_USAGE_INTERVAL,
  EVENTS,
  MENU_ACTIONS,
  NOTICE_AUTO_DISMISS_MILLISECONDS,
  NOTICE_KINDS,
  ROUTING_DEFAULTS_INVALID_ERROR,
  USAGE_INTERVALS,
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
    usageIntervals: USAGE_INTERVALS,
    /** @type {import("../types.d.js").UsageInterval} */
    selectedUsageInterval: DEFAULT_USAGE_INTERVAL,
    usageLoading: false,
    usageLoadVersion: 0,
    /** @type {import("../types.d.js").ManagementProfile | null} */
    profile: null,
    /** @type {import("../types.d.js").FrontendRuntimeConfig | null} */
    runtimeConfig: null,
    /** @type {import("../types.d.js").ProviderProfile[]} */
    providers: [],
    providerEditorSession: createProviderEditorSession(EMPTY_STRING, 0),
    providerAutosavePending: false,
    /** @type {Promise<boolean> | null} */
    providerAutosavePromise: null,
    routingDefaultsDirty: false,
    routingDefaultsEditVersion: 0,
    routingDefaultsAutosavePending: false,
    /** @type {Promise<boolean> | null} */
    routingDefaultsAutosavePromise: null,
    /** @type {Promise<void>} */
    profileMutationTail: Promise.resolve(),
    profileMutationFailureVersion: 0,
    /** @type {Promise<boolean> | null} */
    clientKeyMutationPromise: null,
    profileApplicationVersion: 0,
    providerRemovalConfirmationProviderID: EMPTY_STRING,
    /** @type {import("../types.d.js").TenantDefaults} */
    defaults: emptyDefaults(),
    /** @type {import("../types.d.js").ManagementUsageSummary} */
    usage: emptyUsageSummary(DEFAULT_USAGE_INTERVAL),
    /** @type {import("../types.d.js").ManagementAdminUser[]} */
    adminUsers: [],
    /** @type {Promise<void> | null} */
    profileLoadPromise: null,
    workspaceVersion: 0,
    generatedSecret: EMPTY_STRING,
    generatedSecretVisible: false,
    generatedSecretVersion: 0,
    settingsOpen: false,
    settingsClosePending: false,
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

    get settingsControlsDisabled() {
      return this.busy || this.settingsClosePending;
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

    get dashboardRefreshDisabled() {
      return this.busy || this.usageLoading;
    },

    get usageControlsDisabled() {
      return this.busy || this.usageLoading;
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
      await this.loadUsageSummary(false);
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

    /**
     * @param {import("../types.d.js").UsageInterval} interval
     */
    async selectUsageInterval(interval) {
      if (!this.usageIntervals.some((candidate) => candidate.id === interval)) {
        throw new Error(`usage_interval_invalid:${interval}`);
      }
      this.selectedUsageInterval = interval;
      await this.loadUsageSummary(false);
    },

    /**
     * @param {boolean} showSuccessNotice
     */
    async loadUsageSummary(showSuccessNotice) {
      const workspaceVersion = this.workspaceVersion;
      const interval = this.selectedUsageInterval;
      const loadVersion = this.usageLoadVersion + 1;
      this.usageLoadVersion = loadVersion;
      this.usageLoading = true;
      try {
        const usage = await fetchUsageSummary(interval);
        if (!this.canApplyUsageSummary(workspaceVersion, loadVersion, interval)) {
          return;
        }
        if (usage.interval !== interval) {
          throw new Error(WORKSPACE_INTEGRITY_ERROR);
        }
        this.usage = usage;
        if (showSuccessNotice) {
          this.setNotice(NOTICE_KINDS.SUCCESS, COPY.usageRefreshed);
        }
      } catch {
        if (this.canApplyUsageSummary(workspaceVersion, loadVersion, interval)) {
          this.usage = emptyUsageSummary(interval);
          this.setNotice(NOTICE_KINDS.ERROR, COPY.requestFailed);
        }
      } finally {
        if (this.workspaceVersion === workspaceVersion && this.usageLoadVersion === loadVersion) {
          this.usageLoading = false;
        }
      }
    },

    /**
     * @param {number} workspaceVersion
     * @param {number} loadVersion
     * @param {import("../types.d.js").UsageInterval} interval
     * @returns {boolean}
     */
    canApplyUsageSummary(workspaceVersion, loadVersion, interval) {
      return (
        this.workspaceVersion === workspaceVersion &&
        this.usageLoadVersion === loadVersion &&
        this.selectedUsageInterval === interval &&
        this.authState === AUTH_STATES.AUTHENTICATED
      );
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

    async closeSettings() {
      if (this.settingsClosePending) {
        return;
      }
      const clientKeyMutationAtClose = this.clientKeyMutationPromise;
      const profileMutationFailureVersion = this.profileMutationFailureVersion;
      this.settingsClosePending = true;
      try {
        if (!(await this.autosaveSelectedProvider())) {
          return;
        }
        if (!(await this.autosaveRoutingDefaults())) {
          return;
        }
        await this.waitForProfileMutations();
        if (!this.settingsOpen || this.authState !== AUTH_STATES.AUTHENTICATED) {
          return;
        }
        if (this.profileMutationFailureVersion !== profileMutationFailureVersion) {
          return;
        }
        if (clientKeyMutationAtClose) {
          const clientKeyMutationSucceeded = await clientKeyMutationAtClose;
          if (!clientKeyMutationSucceeded || this.hasGeneratedSecret) {
            return;
          }
        }
        if (this.settingsRequired) {
          this.setNotice(NOTICE_KINDS.ERROR, this.settingsRequirementCopy);
          this.focusSettingsRequirement();
          return;
        }
        this.dismissProviderKeyRemovalConfirmation();
        this.clearProviderKeyMaterial();
        this.clearGeneratedSecret();
        this.settingsOpen = false;
      } finally {
        this.settingsClosePending = false;
      }
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
    async selectProvider(providerID) {
      profileProvider(this.providers, providerID);
      if (providerID === this.selectedProviderID) {
        return;
      }
      this.restoreSelectedProviderControl();
      if (!(await this.autosaveSelectedProvider())) {
        this.restoreSelectedProviderControl();
        return;
      }
      if (!this.settingsOpen || this.authState !== AUTH_STATES.AUTHENTICATED) {
        return;
      }
      this.dismissProviderKeyRemovalConfirmation();
      this.replaceProviderEditorSession(providerID);
    },

    restoreSelectedProviderControl() {
      this.$nextTick(() => {
        if (this.settingsOpen && this.$refs.providerSelector) {
          this.$refs.providerSelector.value = this.selectedProviderID;
        }
      });
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
      this.providerEditorSession.keyDirty = true;
      this.markSelectedProviderDirty();
    },

    /**
     * @param {Event} event
     */
    handleSelectedProviderTextModelChange(event) {
      const modelSelect = /** @type {HTMLSelectElement} */ (event.target);
      this.providerEditorSession.textModel = modelSelect.value;
      this.markSelectedProviderDirty();
      void this.autosaveSelectedProvider();
    },

    /**
     * @param {Event} event
     */
    handleSelectedProviderSystemPromptInput(event) {
      const systemPromptInput = /** @type {HTMLTextAreaElement} */ (event.target);
      this.providerEditorSession.systemPrompt = systemPromptInput.value;
      this.markSelectedProviderDirty();
    },

    markSelectedProviderDirty() {
      this.providerEditorSession.dirty = true;
      this.providerEditorSession.editVersion += 1;
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
      const provider = providerID === EMPTY_STRING ? null : profileProvider(this.providers, providerID);
      this.providerEditorSession = createProviderEditorSession(
        providerID,
        this.providerEditorSession.revealVersion + 1,
        provider ? provider.text_model : EMPTY_STRING,
        provider ? provider.system_prompt : EMPTY_STRING,
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

    requestSelectedProviderKeyRemoval() {
      const provider = this.selectedProvider;
      if (!provider) {
        return;
      }
      if (!provider.has_key) {
        this.clearProviderKeyMaterial();
        this.$nextTick(() => {
          this.$refs.providerKeyInput.focus();
        });
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

    async autosaveSelectedProvider() {
      if (this.providerAutosavePromise) {
        return this.providerAutosavePromise;
      }
      if (!this.providerEditorSession.dirty) {
        return true;
      }
      const autosavePromise = this.persistSelectedProviderChanges();
      this.providerAutosavePromise = autosavePromise;
      this.providerAutosavePending = true;
      try {
        return await autosavePromise;
      } finally {
        if (this.providerAutosavePromise === autosavePromise) {
          this.providerAutosavePromise = null;
          this.providerAutosavePending = false;
        }
      }
    },

    async persistSelectedProviderChanges() {
      while (this.providerEditorSession.dirty) {
        const provider = this.selectedProvider;
        if (!provider) {
          return false;
        }
        const editorSession = this.providerEditorSession;
        const apiKey = editorSession.keyDirty ? editorSession.keyInput.trim() : EMPTY_STRING;
        if (!provider.has_key && !apiKey) {
          editorSession.dirty = false;
          return true;
        }
        const providerID = provider.id;
        const revealVersion = editorSession.revealVersion;
        const editVersion = editorSession.editVersion;
        const workspaceVersion = this.workspaceVersion;
        editorSession.dirty = false;
        try {
          const profileApplied = await this.enqueueProfileMutation(workspaceVersion, async () => {
            const updatedProfile = await requestSaveProviderKey(
              providerID,
              apiKey,
              editorSession.textModel,
              editorSession.systemPrompt,
            );
            if (!this.canApplyProviderAutosave(providerID, revealVersion, workspaceVersion)) {
              return false;
            }
            const preserveProviderEditor = this.providerEditorSession.editVersion !== editVersion;
            this.applyProfile(
              updatedProfile,
              preserveProviderEditor,
              this.routingDefaultsDirty || this.routingDefaultsAutosavePending,
            );
            if (!preserveProviderEditor) {
              this.setNotice(NOTICE_KINDS.SUCCESS, COPY.providerSettingsSaved);
            }
            return true;
          });
          if (!profileApplied) {
            return false;
          }
        } catch (requestError) {
          if (this.canApplyProviderAutosave(providerID, revealVersion, workspaceVersion)) {
            this.providerEditorSession.dirty = true;
            this.setNotice(NOTICE_KINDS.ERROR, profileFailureMessage(requestError));
          }
          return false;
        }
      }
      return true;
    },

    /**
     * @param {string} providerID
     * @param {number} revealVersion
     * @param {number} workspaceVersion
     * @returns {boolean}
     */
    canApplyProviderAutosave(providerID, revealVersion, workspaceVersion) {
      return (
        this.settingsOpen &&
        this.authState === AUTH_STATES.AUTHENTICATED &&
        this.workspaceVersion === workspaceVersion &&
        this.selectedProviderID === providerID &&
        this.providerEditorSession.revealVersion === revealVersion
      );
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

    async autosaveRoutingDefaults() {
      if (this.routingDefaultsAutosavePromise) {
        return this.routingDefaultsAutosavePromise;
      }
      if (!this.routingDefaultsDirty) {
        return true;
      }
      const autosavePromise = this.persistRoutingDefaultsChanges();
      this.routingDefaultsAutosavePromise = autosavePromise;
      this.routingDefaultsAutosavePending = true;
      try {
        return await autosavePromise;
      } finally {
        if (this.routingDefaultsAutosavePromise === autosavePromise) {
          this.routingDefaultsAutosavePromise = null;
          this.routingDefaultsAutosavePending = false;
        }
      }
    },

    async persistRoutingDefaultsChanges() {
      while (this.routingDefaultsDirty) {
        const defaults = { ...this.defaults };
        const editVersion = this.routingDefaultsEditVersion;
        const workspaceVersion = this.workspaceVersion;
        this.routingDefaultsDirty = false;
        try {
          const profileApplied = await this.enqueueProfileMutation(workspaceVersion, async () => {
            const updatedProfile = await requestUpdateDefaults(defaults);
            if (!this.canApplyRoutingDefaultsAutosave(workspaceVersion)) {
              return false;
            }
            if (this.routingDefaultsEditVersion !== editVersion) {
              return true;
            }
            this.applyProfile(updatedProfile, true);
            this.setNotice(NOTICE_KINDS.SUCCESS, COPY.defaultsSaved);
            return true;
          });
          if (!profileApplied) {
            return false;
          }
        } catch (requestError) {
          if (this.canApplyRoutingDefaultsAutosave(workspaceVersion)) {
            this.routingDefaultsDirty = true;
            this.setNotice(NOTICE_KINDS.ERROR, profileFailureMessage(requestError));
          }
          return false;
        }
      }
      return true;
    },

    /**
     * @param {number} workspaceVersion
     * @returns {boolean}
     */
    canApplyRoutingDefaultsAutosave(workspaceVersion) {
      return (
        this.settingsOpen &&
        this.authState === AUTH_STATES.AUTHENTICATED &&
        this.workspaceVersion === workspaceVersion
      );
    },

    async requestAndApplyGeneratedSecret() {
      return this.runClientKeyMutation(async () => this.generateAndApplySecret());
    },

    async generateAndApplySecret() {
      const generatedSecretVersion = this.generatedSecretVersion;
      const workspaceVersion = this.workspaceVersion;
      try {
        const profileApplied = await this.enqueueProfileMutation(workspaceVersion, async () => {
          const secretResponse = await requestGeneratedSecret();
          if (!this.canApplyGeneratedSecret(generatedSecretVersion)) {
            return false;
          }
          this.generatedSecret = secretResponse.secret;
          this.generatedSecretVisible = false;
          this.applyProfile(
            secretResponse.profile,
            this.providerEditorSession.dirty || this.providerAutosavePending,
            this.routingDefaultsDirty || this.routingDefaultsAutosavePending,
          );
          this.setNotice(NOTICE_KINDS.SUCCESS, COPY.keyGenerated);
          return true;
        });
        return Boolean(profileApplied);
      } catch (requestError) {
        if (this.canApplyGeneratedSecret(generatedSecretVersion)) {
          this.setNotice(NOTICE_KINDS.ERROR, profileFailureMessage(requestError));
        }
        return false;
      }
    },

    /**
     * @param {() => Promise<boolean>} mutation
     * @returns {Promise<boolean>}
     */
    async runClientKeyMutation(mutation) {
      if (this.clientKeyMutationPromise) {
        return this.clientKeyMutationPromise;
      }
      const clientKeyMutationPromise = mutation();
      this.clientKeyMutationPromise = clientKeyMutationPromise;
      try {
        return await clientKeyMutationPromise;
      } finally {
        if (this.clientKeyMutationPromise === clientKeyMutationPromise) {
          this.clientKeyMutationPromise = null;
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
      const keyRevoked = await this.runClientKeyMutation(
        async () => this.runProfileMutation(async () => requestRevokeSecret(), COPY.keyRevoked),
      );
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

    /** @param {Event} event */
    handleTextProviderDefaultChange(event) {
      const providerSelect = /** @type {HTMLSelectElement} */ (event.target);
      this.defaults.provider = providerSelect.value;
      const provider = profileProvider(this.providers, providerSelect.value);
      this.defaults.model = provider.text_default_model;
      this.normalizeReasoningEffortDefault();
      this.markRoutingDefaultsDirty();
      void this.autosaveRoutingDefaults();
    },

    /** @param {Event} event */
    handleTextModelDefaultChange(event) {
      const modelSelect = /** @type {HTMLSelectElement} */ (event.target);
      this.defaults.model = modelSelect.value;
      this.normalizeReasoningEffortDefault();
      this.markRoutingDefaultsDirty();
      void this.autosaveRoutingDefaults();
    },

    normalizeReasoningEffortDefault() {
      if (!this.reasoningEffortOptions.includes(this.defaults.reasoning_effort)) {
        this.defaults.reasoning_effort = EMPTY_STRING;
      }
    },

    /** @param {Event} event */
    handleReasoningEffortDefaultChange(event) {
      const effortSelect = /** @type {HTMLSelectElement} */ (event.target);
      this.defaults.reasoning_effort = effortSelect.value;
      this.markRoutingDefaultsDirty();
      void this.autosaveRoutingDefaults();
    },

    /** @param {Event} event */
    handleDictationProviderDefaultChange(event) {
      const providerSelect = /** @type {HTMLSelectElement} */ (event.target);
      this.defaults.dictation_provider = providerSelect.value;
      const provider = profileProvider(this.providers, providerSelect.value);
      if (!provider.supports_dictation || !provider.dictation_default_model) {
        throw new Error(WORKSPACE_INTEGRITY_ERROR);
      }
      this.defaults.dictation_model = provider.dictation_default_model;
      this.markRoutingDefaultsDirty();
      void this.autosaveRoutingDefaults();
    },

    /** @param {Event} event */
    handleDictationModelDefaultChange(event) {
      const modelSelect = /** @type {HTMLSelectElement} */ (event.target);
      this.defaults.dictation_model = modelSelect.value;
      this.markRoutingDefaultsDirty();
      void this.autosaveRoutingDefaults();
    },

    /** @param {Event} event */
    handleRoutingSystemPromptInput(event) {
      const systemPromptInput = /** @type {HTMLTextAreaElement} */ (event.target);
      this.defaults.system_prompt = systemPromptInput.value;
      this.markRoutingDefaultsDirty();
    },

    markRoutingDefaultsDirty() {
      this.routingDefaultsDirty = true;
      this.routingDefaultsEditVersion += 1;
    },

    /**
     * @param {() => Promise<import("../types.d.js").ManagementProfile>} mutation
     * @param {string} successMessage
     * @returns {Promise<boolean>}
     */
    async runProfileMutation(mutation, successMessage) {
      const workspaceVersion = this.workspaceVersion;
      this.busy = true;
      try {
        const profileApplied = await this.enqueueProfileMutation(workspaceVersion, async () => {
          const updatedProfile = await mutation();
          if (!this.canApplyProfileMutation(workspaceVersion)) {
            return false;
          }
          this.applyProfile(
            updatedProfile,
            this.providerEditorSession.dirty || this.providerAutosavePending,
            this.routingDefaultsDirty || this.routingDefaultsAutosavePending,
          );
          this.setNotice(NOTICE_KINDS.SUCCESS, successMessage);
          return true;
        });
        return Boolean(profileApplied);
      } catch (requestError) {
        if (this.canApplyProfileMutation(workspaceVersion)) {
          this.setNotice(NOTICE_KINDS.ERROR, profileFailureMessage(requestError));
        }
        return false;
      } finally {
        this.busy = false;
      }
    },

    /**
     * @template MutationResult
     * @param {number} workspaceVersion
     * @param {() => Promise<MutationResult>} mutation
     * @returns {Promise<MutationResult | null>}
     */
    async enqueueProfileMutation(workspaceVersion, mutation) {
      const previousMutation = this.profileMutationTail;
      /** @type {() => void} */
      let releaseMutation = () => {};
      const mutationCompleted = new Promise((resolve) => {
        releaseMutation = resolve;
      });
      this.profileMutationTail = previousMutation.then(() => mutationCompleted);
      await previousMutation;
      try {
        if (!this.canApplyProfileMutation(workspaceVersion)) {
          return null;
        }
        return await mutation();
      } catch (requestError) {
        if (this.canApplyProfileMutation(workspaceVersion)) {
          this.profileMutationFailureVersion += 1;
        }
        throw requestError;
      } finally {
        releaseMutation();
      }
    },

    async waitForProfileMutations() {
      while (true) {
        const profileMutationTail = this.profileMutationTail;
        await profileMutationTail;
        if (profileMutationTail === this.profileMutationTail) {
          return;
        }
      }
    },

    /**
     * @param {number} workspaceVersion
     * @returns {boolean}
     */
    canApplyProfileMutation(workspaceVersion) {
      return (
        this.settingsOpen &&
        this.authState === AUTH_STATES.AUTHENTICATED &&
        this.workspaceVersion === workspaceVersion
      );
    },

    /**
     * @param {import("../types.d.js").ManagementProfile} nextProfile
     * @param {boolean} [preserveProviderEditor]
     * @param {boolean} [preserveRoutingDefaults]
     */
    applyProfile(nextProfile, preserveProviderEditor = false, preserveRoutingDefaults = false) {
      const defaults = createWorkspaceRoutingDefaults(nextProfile);
      const profileApplicationVersion = this.profileApplicationVersion + 1;
      this.profileApplicationVersion = profileApplicationVersion;
      const selectedProviderID = this.selectedProviderID;
      if (preserveProviderEditor) {
        profileProvider(nextProfile.providers, selectedProviderID);
      }
      this.dismissProviderKeyRemovalConfirmation();
      this.profile = nextProfile;
      applyUserMenuItems(Boolean(nextProfile.user.is_admin));
      this.providers = nextProfile.providers;
      if (!preserveRoutingDefaults) {
        this.defaults.provider = defaults.provider;
        this.defaults.model = defaults.model;
        this.defaults.dictation_provider = defaults.dictation_provider;
        this.defaults.dictation_model = defaults.dictation_model;
        this.defaults.system_prompt = defaults.system_prompt;
        this.defaults.reasoning_effort = defaults.reasoning_effort;
      }
      const nextProviderID = this.providers.some((provider) => provider.id === selectedProviderID)
        ? selectedProviderID
        : defaults.provider;
      if (!preserveProviderEditor) {
        this.replaceProviderEditorSession(nextProviderID);
      }
      if (!preserveRoutingDefaults) {
        const workspaceVersion = this.workspaceVersion;
        const routingDefaultsEditVersion = this.routingDefaultsEditVersion;
        this.$nextTick(() => {
          if (
            this.workspaceVersion === workspaceVersion &&
            this.profileApplicationVersion === profileApplicationVersion &&
            this.routingDefaultsEditVersion === routingDefaultsEditVersion
          ) {
            this.defaults = { ...defaults };
          }
        });
      }
    },

    clearAuthenticatedState() {
      this.workspaceVersion += 1;
      this.profileApplicationVersion += 1;
      this.usageLoadVersion += 1;
      this.usageLoading = false;
      this.selectedUsageInterval = DEFAULT_USAGE_INTERVAL;
      this.providerAutosavePromise = null;
      this.providerAutosavePending = false;
      this.routingDefaultsAutosavePromise = null;
      this.routingDefaultsAutosavePending = false;
      this.routingDefaultsDirty = false;
      this.routingDefaultsEditVersion += 1;
      this.clientKeyMutationPromise = null;
      this.profileMutationFailureVersion = 0;
      this.settingsClosePending = false;
      this.clearNotice();
      this.dismissProviderKeyRemovalConfirmation();
      this.profile = null;
      this.providers = [];
      this.replaceProviderEditorSession(EMPTY_STRING);
      this.defaults = emptyDefaults();
      this.usage = emptyUsageSummary(DEFAULT_USAGE_INTERVAL);
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
 * @param {string} [textModel]
 * @param {string} [systemPrompt]
 * @returns {{ providerID: string, keyInput: string, keyVisible: boolean, keyDirty: boolean, textModel: string, systemPrompt: string, dirty: boolean, editVersion: number, revealPending: boolean, revealVersion: number }}
 */
function createProviderEditorSession(providerID, revealVersion, textModel = EMPTY_STRING, systemPrompt = EMPTY_STRING) {
  return {
    providerID,
    keyInput: EMPTY_STRING,
    keyVisible: false,
    keyDirty: false,
    textModel,
    systemPrompt,
    dirty: false,
    editVersion: 0,
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
