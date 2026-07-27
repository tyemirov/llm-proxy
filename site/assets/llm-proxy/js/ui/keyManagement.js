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
  USAGE_ENDPOINT_LABELS,
  USAGE_FAILURE_PAGE_LIMIT,
  USAGE_INTERVALS,
  USAGE_OUTCOME_LABELS,
  USAGE_STATUS_LABELS,
  WORKSPACE_INTEGRITY_ERROR,
} from "../constants.js?v=20260727";
import {
  createTenant as requestCreateTenant,
  deleteTenant as requestDeleteTenant,
  fetchAccountUsageFailures,
  fetchAccountUsageSummary,
  fetchAdminUsers,
  fetchAccount,
  fetchTenant,
  fetchUsageFailures,
  fetchUsageSummary,
  generateSecret as requestGeneratedSecret,
  loadFrontendRuntimeConfig,
  removeProviderKey as requestRemoveProviderKey,
  renameTenant as requestRenameTenant,
  revealProviderKey as requestRevealProviderKey,
  revokeSecret as requestRevokeSecret,
  saveProviderKey as requestSaveProviderKey,
  updateDefaults as requestUpdateDefaults,
} from "../core/backendClient.js?v=20260727";
import {
  emptyUsageSummary,
  modelRows,
  providerRows,
  successRateLabel,
  usagePolyline,
  USAGE_CHART,
  USAGE_METRICS,
} from "./usagePresentation.js?v=20260727";
import {
  applyUserMenuItems,
  readMprUIAuthStatus,
  waitForMprUIAutoOrchestrationReady,
} from "../core/mprShell.js?v=20260727";
import { dispatchManagementReady } from "../core/runtimeTransition.js?v=20260727";

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
const TENANT_NAME_MAXIMUM_CHARACTERS = 80;

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
    selectedUsageTenantID: EMPTY_STRING,
    usageLoading: false,
    usageLoadVersion: 0,
    /** @type {import("../types.d.js").ManagementAccount | null} */
    account: null,
    /** @type {import("../types.d.js").ManagementTenantSummary[]} */
    tenants: [],
    settingsTenantID: EMPTY_STRING,
    /** @type {import("../types.d.js").ManagementTenantProfile | null} */
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
    usageFailuresOpen: false,
    usageFailuresLoading: false,
    usageFailuresError: EMPTY_STRING,
    usageFailuresLoadVersion: 0,
    /** @type {Array<import("../types.d.js").ManagementUsageFailure | import("../types.d.js").ManagementAccountUsageFailure>} */
    usageFailures: [],
    usageFailuresNextCursor: EMPTY_STRING,
    /** @type {import("../types.d.js").ManagementAdminUser[]} */
    adminUsers: [],
    /** @type {Promise<void> | null} */
    workspaceLoadPromise: null,
    workspaceVersion: 0,
    /** @type {AbortController | null} */
    accountRequestController: null,
    /** @type {AbortController | null} */
    tenantLifetimeController: null,
    /** @type {AbortController | null} */
    tenantRequestController: null,
    /** @type {AbortController | null} */
    usageRequestController: null,
    /** @type {AbortController | null} */
    usageFailuresRequestController: null,
    generatedSecret: EMPTY_STRING,
    generatedSecretVisible: false,
    generatedSecretVersion: 0,
    settingsOpen: false,
    settingsClosePending: false,
    usageExamplesOpen: false,
    tenantNameDraft: EMPTY_STRING,
    tenantNameEditing: false,
    tenantNameDirty: false,
    tenantNameError: EMPTY_STRING,
    createTenantDialogOpen: false,
    createTenantName: EMPTY_STRING,
    createTenantError: EMPTY_STRING,
    createTenantPending: false,
    deleteTenantConfirmationOpen: false,
    deleteTenantPending: false,
    discardTenantChangesOpen: false,
    pendingTenantID: EMPTY_STRING,
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

    get settingsTenant() {
      return this.tenants.find((tenant) => tenant.id === this.settingsTenantID) || null;
    },

    get settingsTenantName() {
      return this.settingsTenant ? this.settingsTenant.name : EMPTY_STRING;
    },

    get canDeleteSettingsTenant() {
      return this.tenants.length > 1;
    },

    get usageScopeIsAllTenants() {
      return this.selectedUsageTenantID === EMPTY_STRING;
    },

    get hasUnsavedSettingsChanges() {
      return Boolean(
        this.settingsOpen &&
        (
          this.tenantNameDirty ||
          this.providerEditorSession.dirty ||
          this.providerAutosavePending ||
          this.routingDefaultsDirty ||
          this.routingDefaultsAutosavePending
        )
      );
    },

    get deleteTenantTitle() {
      return this.settingsTenant ? `Delete “${this.settingsTenant.name}”?` : COPY.deleteTenantTitle;
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

    get hasUsageFailures() {
      return this.usage.totals.failed_requests > 0;
    },

    get usageFailuresActionCopy() {
      const failureCount = this.usage.totals.failed_requests;
      const noun = failureCount === 1 ? "failed request" : "failed requests";
      return `${formatNumber(failureCount)} ${noun}`;
    },

    get usageFailuresIntervalLabel() {
      const interval = this.usageIntervals.find((candidate) => candidate.id === this.selectedUsageInterval);
      if (!interval) {
        throw new Error(`usage_interval_invalid:${this.selectedUsageInterval}`);
      }
      return interval.label;
    },

    get usageFailureStatusRows() {
      return this.usage.status_codes
        .filter((status) => status.status_code >= 400)
        .map((status) => ({
          statusCode: status.status_code,
          label: usageStatusLabel(status.status_code),
          requests: formatNumber(status.requests),
        }));
    },

    get usageFailureRows() {
      return this.usageFailures.map((failure) => usageFailurePresentation(failure));
    },

    get hasLoadedUsageFailures() {
      return this.usageFailures.length > 0;
    },

    get canLoadMoreUsageFailures() {
      return Boolean(this.usageFailuresNextCursor);
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

    async loadWorkspace() {
      if (this.workspaceLoadPromise) {
        return this.workspaceLoadPromise;
      }
      this.workspaceLoadPromise = this.loadWorkspaceOnce();
      try {
        await this.workspaceLoadPromise;
      } finally {
        this.workspaceLoadPromise = null;
      }
    },

    async loadAuthenticatedWorkspace() {
      if (this.authState === AUTH_STATES.AUTHENTICATED || this.authState === AUTH_STATES.ERROR) {
        return;
      }
      this.authState = AUTH_STATES.LOADING;
      await this.loadWorkspace();
      if (this.authState === AUTH_STATES.LOADING && readMprUIAuthStatus() === AUTH_STATES.AUTHENTICATED) {
        await this.loadWorkspace();
      }
    },

    async loadWorkspaceOnce() {
      const workspaceVersion = this.workspaceVersion;
      if (this.accountRequestController) {
        this.accountRequestController.abort();
      }
      const accountRequestController = new AbortController();
      this.accountRequestController = accountRequestController;
      this.busy = true;
      try {
        const loadedAccount = await fetchAccount(accountRequestController.signal);
        if (!this.canApplyAuthenticatedWorkspace(workspaceVersion)) {
          return;
        }
        assertManagementAccount(loadedAccount);
        this.account = loadedAccount;
        this.tenants = loadedAccount.tenants;
        applyUserMenuItems(Boolean(loadedAccount.user.is_admin));
        this.settingsTenantID = this.tenants[0].id;
        this.replaceTenantLifetimeController();
        await this.hydrateSettingsTenant(null, workspaceVersion);
        if (this.authState === AUTH_STATES.AUTHENTICATED) {
          await this.loadUsageSummary(false);
        }
      } catch (requestError) {
        if (!isAbortError(requestError) && this.canApplyAuthenticatedWorkspace(workspaceVersion)) {
          this.clearAuthenticatedState();
          this.authState = AUTH_STATES.ERROR;
          this.setNotice(NOTICE_KINDS.ERROR, profileFailureMessage(requestError));
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
     * @param {number} workspaceVersion
     */
    async hydrateSettingsTenant(prefetchedProfile, workspaceVersion) {
      const tenantID = this.settingsTenantID;
      if (this.tenantRequestController) {
        this.tenantRequestController.abort();
      }
      const tenantRequestController = new AbortController();
      this.tenantRequestController = tenantRequestController;
      this.clearSettingsTenantState();
      try {
        const loadedProfile = prefetchedProfile || await fetchTenant(tenantID, tenantRequestController.signal);
        if (!this.canApplySettingsTenant(workspaceVersion, tenantID)) {
          return;
        }
        assertManagementTenantProfile(loadedProfile, tenantID);
        this.applyProfile(loadedProfile);
        this.authState = AUTH_STATES.AUTHENTICATED;
        this.setNotice(NOTICE_KINDS.SUCCESS, COPY.profileLoaded);
        if (this.settingsRequired) {
          this.openSettings();
        }
        if (!this.hasSecret) {
          await this.requestAndApplyGeneratedSecret();
        }
      } catch (requestError) {
        if (!isAbortError(requestError) && this.canApplySettingsTenant(workspaceVersion, tenantID)) {
          this.clearSettingsTenantState();
          if (this.authState !== AUTH_STATES.AUTHENTICATED) {
            this.authState = AUTH_STATES.ERROR;
          }
          this.setNotice(NOTICE_KINDS.ERROR, profileFailureMessage(requestError));
        }
      } finally {
        if (this.tenantRequestController === tenantRequestController) {
          this.tenantRequestController = null;
        }
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

    /**
     * @param {number} workspaceVersion
     * @param {string} tenantID
     * @returns {boolean}
     */
    canApplySettingsTenant(workspaceVersion, tenantID) {
      return this.canApplyAuthenticatedWorkspace(workspaceVersion) && this.settingsTenantID === tenantID;
    },

    replaceTenantLifetimeController() {
      if (this.tenantLifetimeController) {
        this.tenantLifetimeController.abort();
      }
      this.tenantLifetimeController = new AbortController();
    },

    /** @param {Event} event */
    handleSettingsTenantSelection(event) {
      const tenantSelect = /** @type {HTMLSelectElement} */ (event.target);
      void this.requestSettingsTenantSwitch(tenantSelect.value);
    },

    /**
     * @param {string} tenantID
     */
    async requestSettingsTenantSwitch(tenantID) {
      if (!this.tenants.some((tenant) => tenant.id === tenantID)) {
        this.restoreSettingsTenantSelector();
        this.setNotice(NOTICE_KINDS.ERROR, COPY.requestFailed);
        return;
      }
      if (tenantID === this.settingsTenantID) {
        this.restoreSettingsTenantSelector();
        return;
      }
      if (this.hasUnsavedSettingsChanges) {
        this.pendingTenantID = tenantID;
        this.discardTenantChangesOpen = true;
        this.$nextTick(() => {
          this.$refs.discardTenantStay.focus();
        });
        return;
      }
      await this.switchSettingsTenant(tenantID);
    },

    restoreSettingsTenantSelector() {
      this.$nextTick(() => {
        if (this.$refs.settingsTenantSelector) {
          this.$refs.settingsTenantSelector.value = this.settingsTenantID;
        }
      });
    },

    cancelTenantSwitch() {
      this.discardTenantChangesOpen = false;
      this.pendingTenantID = EMPTY_STRING;
      this.restoreSettingsTenantSelector();
    },

    async confirmTenantSwitch() {
      const tenantID = this.pendingTenantID;
      this.discardTenantChangesOpen = false;
      this.pendingTenantID = EMPTY_STRING;
      this.discardLocalTenantEdits();
      await this.switchSettingsTenant(tenantID);
    },

    /**
     * @param {string} tenantID
     * @param {import("../types.d.js").ManagementTenantProfile | null} [prefetchedProfile]
     */
    async switchSettingsTenant(tenantID, prefetchedProfile = null) {
      this.workspaceVersion += 1;
      const workspaceVersion = this.workspaceVersion;
      if (this.tenantRequestController) {
        this.tenantRequestController.abort();
      }
      this.replaceTenantLifetimeController();
      this.clearGeneratedSecret();
      this.clearProviderKeyMaterial();
      this.dismissProviderKeyRemovalConfirmation();
      this.deleteTenantConfirmationOpen = false;
      this.createTenantDialogOpen = false;
      this.createTenantName = EMPTY_STRING;
      this.createTenantError = EMPTY_STRING;
      this.discardTenantChangesOpen = false;
      this.pendingTenantID = EMPTY_STRING;
      this.settingsTenantID = tenantID;
      this.busy = true;
      try {
        await this.hydrateSettingsTenant(prefetchedProfile, workspaceVersion);
      } finally {
        if (this.workspaceVersion === workspaceVersion) {
          this.busy = false;
        }
        dispatchManagementReady();
      }
    },

    openCreateTenantDialog() {
      this.createTenantName = EMPTY_STRING;
      this.createTenantError = EMPTY_STRING;
      this.createTenantDialogOpen = true;
      this.$nextTick(() => {
        this.$refs.createTenantName.focus();
      });
    },

    closeCreateTenantDialog() {
      if (this.createTenantPending) {
        return;
      }
      this.createTenantDialogOpen = false;
      this.createTenantName = EMPTY_STRING;
      this.createTenantError = EMPTY_STRING;
      this.$nextTick(() => {
        if (this.$refs.createTenantButton) {
          this.$refs.createTenantButton.focus();
        }
      });
    },

    /** @param {KeyboardEvent} event */
    trapCreateTenantFocus(event) {
      trapDialogFocus(event, this.$refs.createTenantDialog);
    },

    /** @param {KeyboardEvent} event */
    trapDiscardTenantFocus(event) {
      trapDialogFocus(event, this.$refs.discardTenantDialog);
    },

    /** @param {KeyboardEvent} event */
    trapDeleteTenantFocus(event) {
      trapDialogFocus(event, this.$refs.deleteTenantDialog);
    },

    /** @param {Event} event */
    handleCreateTenantNameInput(event) {
      this.createTenantName = /** @type {HTMLInputElement} */ (event.target).value;
      this.createTenantError = EMPTY_STRING;
    },

    async submitCreateTenant() {
      let name;
      try {
        name = validatedTenantName(this.createTenantName);
      } catch {
        this.createTenantError = COPY.tenantNameInvalid;
        return;
      }
      const workspaceVersion = this.workspaceVersion;
      const lifetimeController = this.tenantLifetimeController;
      if (!lifetimeController) {
        return;
      }
      this.createTenantPending = true;
      try {
        const createdProfile = await requestCreateTenant(name, lifetimeController.signal);
        if (
          !this.canApplyAuthenticatedWorkspace(workspaceVersion) ||
          this.tenantLifetimeController !== lifetimeController ||
          !this.createTenantDialogOpen
        ) {
          return;
        }
        assertManagementTenantProfile(createdProfile, createdProfile.tenant.id);
        const createdSummary = tenantSummaryFromProfile(createdProfile);
        this.tenants = [...this.tenants, createdSummary];
        this.account = { ...this.account, tenants: this.tenants };
        this.createTenantDialogOpen = false;
        this.createTenantName = EMPTY_STRING;
        await this.switchSettingsTenant(createdSummary.id, createdProfile);
        if (this.settingsTenantID === createdSummary.id && this.authState === AUTH_STATES.AUTHENTICATED) {
          this.setNotice(NOTICE_KINDS.SUCCESS, COPY.tenantCreated);
        }
      } catch (requestError) {
        if (!isAbortError(requestError) && this.tenantLifetimeController === lifetimeController) {
          this.createTenantError = requestError && requestError.status === 409
            ? COPY.tenantNameConflict
            : profileFailureMessage(requestError);
        }
      } finally {
        this.createTenantPending = false;
      }
    },

    /** @param {Event} event */
    handleTenantNameInput(event) {
      this.tenantNameDraft = /** @type {HTMLInputElement} */ (event.target).value;
      this.tenantNameDirty = this.tenantNameDraft !== this.settingsTenantName;
      this.tenantNameError = EMPTY_STRING;
    },

    beginTenantNameEdit() {
      this.tenantNameDraft = this.settingsTenantName;
      this.tenantNameDirty = false;
      this.tenantNameError = EMPTY_STRING;
      this.tenantNameEditing = true;
      this.$nextTick(() => {
        this.$refs.tenantNameInput.focus();
      });
    },

    resetTenantNameEdit() {
      this.tenantNameDraft = this.settingsTenantName;
      this.tenantNameEditing = false;
      this.tenantNameDirty = false;
      this.tenantNameError = EMPTY_STRING;
    },

    cancelTenantNameEdit() {
      this.resetTenantNameEdit();
      this.$nextTick(() => {
        this.$refs.tenantRenameButton.focus();
      });
    },

    async saveTenantName() {
      let name;
      try {
        name = validatedTenantName(this.tenantNameDraft);
      } catch {
        this.tenantNameError = COPY.tenantNameInvalid;
        return;
      }
      const tenantID = this.settingsTenantID;
      const workspaceVersion = this.workspaceVersion;
      const lifetimeController = this.tenantLifetimeController;
      if (!lifetimeController || !this.tenantNameDirty) {
        return;
      }
      let tenantRenamed = false;
      this.busy = true;
      try {
        tenantRenamed = Boolean(await this.enqueueProfileMutation(workspaceVersion, async () => {
          const updatedProfile = await requestRenameTenant(tenantID, name, lifetimeController.signal);
          if (!this.canApplySettingsTenant(workspaceVersion, tenantID)) {
            return false;
          }
          assertManagementTenantProfile(updatedProfile, tenantID);
          this.tenants = this.tenants.map((tenant) => (
            tenant.id === tenantID ? tenantSummaryFromProfile(updatedProfile) : tenant
          ));
          this.account = { ...this.account, tenants: this.tenants };
          this.tenantNameDraft = updatedProfile.tenant.name;
          this.tenantNameEditing = false;
          this.tenantNameDirty = false;
          this.applyProfile(
            updatedProfile,
            this.providerEditorSession.dirty || this.providerAutosavePending,
            this.routingDefaultsDirty || this.routingDefaultsAutosavePending,
          );
          this.setNotice(NOTICE_KINDS.SUCCESS, COPY.tenantRenamed);
          return true;
        }));
      } catch (requestError) {
        if (!isAbortError(requestError) && this.canApplySettingsTenant(workspaceVersion, tenantID)) {
          this.tenantNameError = requestError && requestError.status === 409
            ? COPY.tenantNameConflict
            : profileFailureMessage(requestError);
        }
      } finally {
        this.busy = false;
      }
      if (tenantRenamed) {
        this.$nextTick(() => {
          requestAnimationFrame(() => {
            this.$refs.tenantRenameButton.focus();
          });
        });
      }
    },

    requestTenantDeletion() {
      if (!this.canDeleteSettingsTenant) {
        this.setNotice(NOTICE_KINDS.ERROR, COPY.finalTenantDeletion);
        return;
      }
      this.deleteTenantConfirmationOpen = true;
      this.$nextTick(() => {
        this.$refs.deleteTenantCancel.focus();
      });
    },

    cancelTenantDeletion() {
      this.deleteTenantConfirmationOpen = false;
      this.$nextTick(() => {
        if (this.$refs.deleteTenantButton) {
          this.$refs.deleteTenantButton.focus();
        }
      });
    },

    async confirmTenantDeletion() {
      const deletedTenantID = this.settingsTenantID;
      const lifetimeController = this.tenantLifetimeController;
      if (!lifetimeController || !this.canDeleteSettingsTenant) {
        return;
      }
      this.deleteTenantPending = true;
      try {
        await requestDeleteTenant(deletedTenantID, lifetimeController.signal);
        if (this.settingsTenantID !== deletedTenantID || this.tenantLifetimeController !== lifetimeController) {
          return;
        }
        this.tenants = this.tenants.filter((tenant) => tenant.id !== deletedTenantID);
        this.account = { ...this.account, tenants: this.tenants };
        this.deleteTenantConfirmationOpen = false;
        const usageNeedsRefresh = this.usageScopeIsAllTenants || this.selectedUsageTenantID === deletedTenantID;
        if (this.selectedUsageTenantID === deletedTenantID) {
          this.selectedUsageTenantID = EMPTY_STRING;
        }
        await this.switchSettingsTenant(this.tenants[0].id);
        if (usageNeedsRefresh) {
          this.clearUsageFailures(false);
          this.usage = emptyUsageSummary(this.selectedUsageInterval);
          await this.loadUsageSummary(false);
        }
        if (this.authState === AUTH_STATES.AUTHENTICATED) {
          this.setNotice(NOTICE_KINDS.SUCCESS, COPY.tenantDeleted);
        }
      } catch (requestError) {
        if (!isAbortError(requestError) && this.settingsTenantID === deletedTenantID) {
          this.setNotice(
            NOTICE_KINDS.ERROR,
            requestError && requestError.status === 409 ? COPY.finalTenantDeletion : profileFailureMessage(requestError),
          );
        }
      } finally {
        this.deleteTenantPending = false;
      }
    },

    discardLocalTenantEdits() {
      this.providerEditorSession.dirty = false;
      this.routingDefaultsDirty = false;
      this.resetTenantNameEdit();
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
      this.clearUsageFailures(false);
      this.selectedUsageInterval = interval;
      this.usage = emptyUsageSummary(interval);
      await this.loadUsageSummary(false);
    },

    /** @param {Event} event */
    async handleUsageTenantSelection(event) {
      const tenantSelect = /** @type {HTMLSelectElement} */ (event.target);
      const tenantID = tenantSelect.value;
      if (tenantID && !this.tenants.some((tenant) => tenant.id === tenantID)) {
        tenantSelect.value = this.selectedUsageTenantID;
        this.setNotice(NOTICE_KINDS.ERROR, COPY.requestFailed);
        return;
      }
      if (tenantID === this.selectedUsageTenantID) {
        return;
      }
      this.clearUsageFailures(false);
      this.selectedUsageTenantID = tenantID;
      this.usage = emptyUsageSummary(this.selectedUsageInterval);
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

    /**
     * @param {boolean} append
     */
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

    /**
     * @param {boolean} restoreFocus
     */
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

    /**
     * @param {boolean} showSuccessNotice
     */
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
      try {
        const usage = tenantID
          ? await fetchUsageSummary(tenantID, interval, usageRequestController.signal)
          : await fetchAccountUsageSummary(interval, usageRequestController.signal);
        if (!this.canApplyUsageSummary(tenantID, loadVersion, interval)) {
          return;
        }
        if (usage.interval !== interval) {
          throw new Error(WORKSPACE_INTEGRITY_ERROR);
        }
        this.usage = usage;
        if (!this.hasUsageFailures) {
          this.clearUsageFailures(false);
        }
        if (showSuccessNotice) {
          this.setNotice(NOTICE_KINDS.SUCCESS, COPY.usageRefreshed);
        }
      } catch (requestError) {
        if (!isAbortError(requestError) && this.canApplyUsageSummary(tenantID, loadVersion, interval)) {
          this.clearUsageFailures(false);
          this.usage = emptyUsageSummary(interval);
          this.setNotice(NOTICE_KINDS.ERROR, COPY.requestFailed);
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
      this.clearUsageFailures(false);
      this.dashboardView = DASHBOARD_VIEWS.ADMIN;
      await this.refreshAdminUsers();
    },

    openUsageDashboard() {
      this.dashboardView = DASHBOARD_VIEWS.USAGE;
    },

    openSettings() {
      this.clearUsageFailures(false);
      this.usageExamplesOpen = false;
      this.dismissProviderKeyRemovalConfirmation();
      this.resetTenantNameEdit();
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
        this.resetTenantNameEdit();
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
      const tenantID = this.settingsTenantID;
      const workspaceVersion = this.workspaceVersion;
      const lifetimeController = this.tenantLifetimeController;
      if (!lifetimeController) {
        return;
      }
      this.providerEditorSession.revealVersion = revealVersion;
      this.providerEditorSession.revealPending = true;
      try {
        const revealResponse = await requestRevealProviderKey(tenantID, revealProviderID, lifetimeController.signal);
        if (!this.canApplyProviderKeyReveal(tenantID, workspaceVersion, revealProviderID, revealVersion)) {
          return;
        }
        this.providerEditorSession.keyInput = revealResponse.api_key;
        this.providerEditorSession.keyVisible = true;
      } catch (requestError) {
        if (
          !isAbortError(requestError) &&
          this.canApplyProviderKeyReveal(tenantID, workspaceVersion, revealProviderID, revealVersion)
        ) {
          this.setNotice(NOTICE_KINDS.ERROR, profileFailureMessage(requestError));
        }
      } finally {
        if (revealVersion === this.providerEditorSession.revealVersion) {
          this.providerEditorSession.revealPending = false;
        }
      }
    },

    /**
     * @param {string} tenantID
     * @param {number} workspaceVersion
     * @param {string} providerID
     * @param {number} revealVersion
     */
    canApplyProviderKeyReveal(tenantID, workspaceVersion, providerID, revealVersion) {
      return (
        this.settingsOpen &&
        this.settingsTenantID === tenantID &&
        this.workspaceVersion === workspaceVersion &&
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
        const tenantID = this.settingsTenantID;
        const lifetimeController = this.tenantLifetimeController;
        if (!lifetimeController) {
          return false;
        }
        editorSession.dirty = false;
        try {
          const profileApplied = await this.enqueueProfileMutation(workspaceVersion, async () => {
            const updatedProfile = await requestSaveProviderKey(
              tenantID,
              providerID,
              apiKey,
              editorSession.textModel,
              editorSession.systemPrompt,
              lifetimeController.signal,
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
      const tenantID = this.settingsTenantID;
      const lifetimeController = this.tenantLifetimeController;
      if (!lifetimeController) {
        return;
      }
      try {
        await this.runProfileMutation(
          async () => requestRemoveProviderKey(tenantID, provider.id, lifetimeController.signal),
          COPY.providerKeyRemoved,
        );
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
        const tenantID = this.settingsTenantID;
        const lifetimeController = this.tenantLifetimeController;
        if (!lifetimeController) {
          return false;
        }
        this.routingDefaultsDirty = false;
        try {
          const profileApplied = await this.enqueueProfileMutation(workspaceVersion, async () => {
            const updatedProfile = await requestUpdateDefaults(tenantID, defaults, lifetimeController.signal);
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
      const tenantID = this.settingsTenantID;
      const lifetimeController = this.tenantLifetimeController;
      if (!lifetimeController) {
        return false;
      }
      try {
        const profileApplied = await this.enqueueProfileMutation(workspaceVersion, async () => {
          const secretResponse = await requestGeneratedSecret(tenantID, lifetimeController.signal);
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
      const tenantID = this.settingsTenantID;
      const lifetimeController = this.tenantLifetimeController;
      if (!lifetimeController) {
        return;
      }
      const keyRevoked = await this.runClientKeyMutation(
        async () => this.runProfileMutation(
          async () => requestRevokeSecret(tenantID, lifetimeController.signal),
          COPY.keyRevoked,
        ),
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
     * @param {() => Promise<import("../types.d.js").ManagementTenantProfile>} mutation
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
     * @param {import("../types.d.js").ManagementTenantProfile} nextProfile
     * @param {boolean} [preserveProviderEditor]
     * @param {boolean} [preserveRoutingDefaults]
     */
    applyProfile(nextProfile, preserveProviderEditor = false, preserveRoutingDefaults = false) {
      assertManagementTenantProfile(nextProfile, this.settingsTenantID);
      const defaults = createWorkspaceRoutingDefaults(nextProfile);
      const profileApplicationVersion = this.profileApplicationVersion + 1;
      this.profileApplicationVersion = profileApplicationVersion;
      const selectedProviderID = this.selectedProviderID;
      if (preserveProviderEditor) {
        profileProvider(nextProfile.providers, selectedProviderID);
      }
      this.dismissProviderKeyRemovalConfirmation();
      this.profile = nextProfile;
      this.providers = nextProfile.providers;
      if (!this.tenantNameDirty) {
        this.tenantNameDraft = nextProfile.tenant.name;
      }
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

    clearSettingsTenantState() {
      this.profileApplicationVersion += 1;
      this.providerAutosavePromise = null;
      this.providerAutosavePending = false;
      this.routingDefaultsAutosavePromise = null;
      this.routingDefaultsAutosavePending = false;
      this.routingDefaultsDirty = false;
      this.routingDefaultsEditVersion += 1;
      this.clientKeyMutationPromise = null;
      this.profileMutationTail = Promise.resolve();
      this.profileMutationFailureVersion = 0;
      this.settingsClosePending = false;
      this.dismissProviderKeyRemovalConfirmation();
      this.profile = null;
      this.providers = [];
      this.replaceProviderEditorSession(EMPTY_STRING);
      this.defaults = emptyDefaults();
      this.clearGeneratedSecret();
      this.tenantNameDraft = EMPTY_STRING;
      this.tenantNameEditing = false;
      this.tenantNameDirty = false;
      this.tenantNameError = EMPTY_STRING;
      this.usageExamplesOpen = false;
    },

    clearUsageState() {
      this.usageLoadVersion += 1;
      this.usageLoading = false;
      this.clearUsageFailures(false);
      this.usage = emptyUsageSummary(this.selectedUsageInterval);
    },

    clearAuthenticatedState() {
      this.workspaceVersion += 1;
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
 * @param {KeyboardEvent} event
 * @param {HTMLElement} dialog
 */
function trapDialogFocus(event, dialog) {
  const focusableControls = [...dialog.querySelectorAll(
    'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
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
}

/**
 * @param {import("../types.d.js").ManagementUsageFailurePage | import("../types.d.js").ManagementAccountUsageFailurePage} response
 * @param {import("../types.d.js").UsageInterval} interval
 * @param {boolean} accountScope
 * @returns {import("../types.d.js").ManagementUsageFailurePage | import("../types.d.js").ManagementAccountUsageFailurePage}
 */
function normalizedUsageFailurePage(response, interval, accountScope) {
  if (!response || response.interval !== interval || !Array.isArray(response.failures)) {
    throw new Error(WORKSPACE_INTEGRITY_ERROR);
  }
  if (response.next_cursor !== undefined && (typeof response.next_cursor !== "string" || !response.next_cursor)) {
    throw new Error(WORKSPACE_INTEGRITY_ERROR);
  }
  return {
    interval,
    failures: response.failures.map((failure) => normalizedUsageFailure(failure, accountScope)),
    ...(response.next_cursor ? { next_cursor: response.next_cursor } : {}),
  };
}

/**
 * @param {import("../types.d.js").ManagementUsageFailure | import("../types.d.js").ManagementAccountUsageFailure} failure
 * @param {boolean} accountScope
 * @returns {import("../types.d.js").ManagementUsageFailure | import("../types.d.js").ManagementAccountUsageFailure}
 */
function normalizedUsageFailure(failure, accountScope) {
  const occurredAt = new Date(failure ? failure.occurred_at : EMPTY_STRING);
  if (
    !failure ||
    typeof failure.occurred_at !== "string" ||
    Number.isNaN(occurredAt.valueOf()) ||
    typeof failure.endpoint !== "string" ||
    !hasLabel(USAGE_ENDPOINT_LABELS, failure.endpoint) ||
    typeof failure.provider !== "string" ||
    typeof failure.model !== "string" ||
    !Number.isInteger(failure.status_code) ||
    !hasLabel(USAGE_STATUS_LABELS, String(failure.status_code)) ||
    typeof failure.outcome_code !== "string" ||
    failure.outcome_code === "success" ||
    !hasLabel(USAGE_OUTCOME_LABELS, failure.outcome_code) ||
    !Number.isInteger(failure.latency_ms) ||
    failure.latency_ms < 0
  ) {
    throw new Error(WORKSPACE_INTEGRITY_ERROR);
  }
  const commonFailure = {
    occurred_at: failure.occurred_at,
    endpoint: failure.endpoint,
    provider: failure.provider,
    model: failure.model,
    status_code: failure.status_code,
    outcome_code: failure.outcome_code,
    latency_ms: failure.latency_ms,
  };
  if (accountScope) {
    if (
      typeof failure.tenant_id !== "string" ||
      !failure.tenant_id ||
      typeof failure.tenant_name !== "string" ||
      !failure.tenant_name
    ) {
      throw new Error(WORKSPACE_INTEGRITY_ERROR);
    }
    return {
      tenant_id: failure.tenant_id,
      tenant_name: failure.tenant_name,
      ...commonFailure,
    };
  }
  if (Object.hasOwn(failure, "tenant_id") || Object.hasOwn(failure, "tenant_name")) {
    throw new Error(WORKSPACE_INTEGRITY_ERROR);
  }
  return commonFailure;
}

/**
 * @param {import("../types.d.js").ManagementUsageFailure | import("../types.d.js").ManagementAccountUsageFailure} failure
 * @returns {{
 *   tenant: string,
 *   occurredAt: string,
 *   endpoint: string,
 *   provider: string,
 *   model: string,
 *   status: string,
 *   outcome: string,
 *   latency: string
 * }}
 */
function usageFailurePresentation(failure) {
  return {
    tenant: "tenant_name" in failure ? `${failure.tenant_name} · ${failure.tenant_id}` : EMPTY_STRING,
    occurredAt: new Intl.DateTimeFormat("en-US", {
      dateStyle: "medium",
      timeStyle: "medium",
      timeZone: "UTC",
    }).format(new Date(failure.occurred_at)),
    endpoint: usageLabel(USAGE_ENDPOINT_LABELS, failure.endpoint),
    provider: failure.provider || COPY.usageFailuresNotResolved,
    model: failure.model || COPY.usageFailuresNotResolved,
    status: `${failure.status_code} ${usageStatusLabel(failure.status_code)}`,
    outcome: usageLabel(USAGE_OUTCOME_LABELS, failure.outcome_code),
    latency: `${formatNumber(failure.latency_ms)} ms`,
  };
}

/**
 * @param {number} statusCode
 * @returns {string}
 */
function usageStatusLabel(statusCode) {
  return usageLabel(USAGE_STATUS_LABELS, String(statusCode));
}

/**
 * @param {Readonly<Record<string, string>>} labels
 * @param {string} value
 * @returns {boolean}
 */
function hasLabel(labels, value) {
  return Object.prototype.hasOwnProperty.call(labels, value);
}

/**
 * @param {Readonly<Record<string, string>>} labels
 * @param {string} value
 * @returns {string}
 */
function usageLabel(labels, value) {
  if (!hasLabel(labels, value)) {
    throw new Error(WORKSPACE_INTEGRITY_ERROR);
  }
  return labels[value];
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
 * @param {import("../types.d.js").ManagementTenantProfile} profile
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
 * @param {import("../types.d.js").ManagementAccount} account
 */
function assertManagementAccount(account) {
  if (
    !account ||
    !account.user ||
    typeof account.user.id !== "string" ||
    account.user.id === EMPTY_STRING ||
    typeof account.user.is_admin !== "boolean" ||
    !Array.isArray(account.tenants) ||
    account.tenants.length === 0
  ) {
    throw new Error(WORKSPACE_INTEGRITY_ERROR);
  }
  const tenantIDs = new Set();
  const tenantNames = new Set();
  for (const tenant of account.tenants) {
    if (
      !tenant ||
      typeof tenant.id !== "string" ||
      tenant.id === EMPTY_STRING ||
      typeof tenant.name !== "string" ||
      tenant.name === EMPTY_STRING ||
      typeof tenant.has_secret !== "boolean" ||
      typeof tenant.created_at !== "string" ||
      typeof tenant.updated_at !== "string" ||
      tenantIDs.has(tenant.id) ||
      tenantNames.has(tenant.name.toLocaleLowerCase("en-US"))
    ) {
      throw new Error(WORKSPACE_INTEGRITY_ERROR);
    }
    tenantIDs.add(tenant.id);
    tenantNames.add(tenant.name.toLocaleLowerCase("en-US"));
  }
}

/**
 * @param {import("../types.d.js").ManagementTenantProfile} profile
 * @param {string} tenantID
 */
function assertManagementTenantProfile(profile, tenantID) {
  if (
    !profile ||
    !profile.tenant ||
    profile.tenant.id !== tenantID ||
    typeof profile.tenant.name !== "string" ||
    profile.tenant.name === EMPTY_STRING ||
    typeof profile.tenant.has_secret !== "boolean" ||
    typeof profile.tenant.created_at !== "string" ||
    typeof profile.tenant.updated_at !== "string" ||
    !Array.isArray(profile.providers) ||
    !profile.proxy ||
    typeof profile.proxy.text_path !== "string" ||
    typeof profile.proxy.v2_path !== "string" ||
    typeof profile.proxy.dictation_path !== "string"
  ) {
    throw new Error(WORKSPACE_INTEGRITY_ERROR);
  }
}

/**
 * @param {import("../types.d.js").ManagementTenantProfile} profile
 * @returns {import("../types.d.js").ManagementTenantSummary}
 */
function tenantSummaryFromProfile(profile) {
  return {
    id: profile.tenant.id,
    name: profile.tenant.name,
    has_secret: profile.tenant.has_secret,
    created_at: profile.tenant.created_at,
    updated_at: profile.tenant.updated_at,
  };
}

/**
 * @param {string} value
 * @returns {string}
 */
function validatedTenantName(value) {
  const name = value.trim();
  if (
    name === EMPTY_STRING ||
    Array.from(name).length > TENANT_NAME_MAXIMUM_CHARACTERS ||
    /[\p{Cc}\p{Cf}\p{Zl}\p{Zp}]/u.test(name)
  ) {
    throw new Error("managed_tenant_name_invalid");
  }
  return name;
}

/**
 * @param {unknown} requestError
 * @returns {boolean}
 */
function isAbortError(requestError) {
  return requestError instanceof DOMException && requestError.name === "AbortError";
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
