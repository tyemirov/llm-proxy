// @ts-check

import {
  APP_INTEGRITY_ERROR,
  AUTH_STATES,
  COPY,
  NOTICE_KINDS,
} from "../constants.js?v=20260902a237";
import { updateDefaults as requestUpdateDefaults } from "../core/backendClient.js?v=20260902a237";
import {
  profileFailureMessage,
  profileProvider,
} from "../core/managementProfile.js?v=20260902a237";

const EMPTY_STRING = "";

/** @typedef {ReturnType<typeof import("./managementApplicationState.js").createManagementApplicationState>} ManagementApplicationState */
/** @typedef {ManagementApplicationState & {
 *   selectedProviderID: string,
 *   applyProfile: (profile: import("../types.d.js").ManagementTenantProfile, preserveProviderEditor?: boolean, preserveRoutingDefaults?: boolean) => void,
 *   enqueueProfileMutation: (appVersion: number, mutation: () => Promise<boolean>) => Promise<boolean | null>,
 *   setSettingsNotice: (kind: string, message: string) => void
 * }} RoutingDefaultsHost */

/**
 * @template {object} Responsibility
 * @param {Responsibility & ThisType<RoutingDefaultsHost & Responsibility>} responsibility
 * @returns {Responsibility}
 */
function routingDefaultsResponsibility(responsibility) {
  return responsibility;
}

/** Create tenant routing-default selection and autosave behavior. */
export function createRoutingDefaultsResponsibility() {
  return routingDefaultsResponsibility({
    get selectedTextModels() {
      const provider = this.providers.find((/** @type {import("../types.d.js").ProviderProfile} */ candidateProvider) => candidateProvider.id === this.defaults.provider);
      return provider ? provider.text_models.map((/** @type {import("../types.d.js").TextModelProfile} */ model) => model.id) : [];
    },

    get keyedTextProviders() {
      return this.providers.filter((/** @type {import("../types.d.js").ProviderProfile} */ provider) => provider.configured);
    },

    get hasKeyedTextProviders() {
      return this.keyedTextProviders.length > 0;
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
      return this.keyedTextProviders.filter((/** @type {import("../types.d.js").ProviderProfile} */ provider) => provider.supports_dictation);
    },

    get hasDictationProviders() {
      return this.dictationProviders.length > 0;
    },

    get selectedDictationModels() {
      const provider = this.providers.find((/** @type {import("../types.d.js").ProviderProfile} */ candidateProvider) => candidateProvider.id === this.defaults.dictation_provider);
      return provider ? provider.dictation_models : [];
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
        const appVersion = this.appVersion;
        const tenantID = this.settingsTenantID;
        const lifetimeController = this.tenantLifetimeController;
        if (!lifetimeController) {
          return false;
        }
        this.routingDefaultsDirty = false;
        try {
          const profileApplied = await this.enqueueProfileMutation(appVersion, async () => {
            const updatedProfile = await requestUpdateDefaults(tenantID, defaults, lifetimeController.signal);
            if (!this.canApplyRoutingDefaultsAutosave(appVersion)) {
              return false;
            }
            if (this.routingDefaultsEditVersion !== editVersion) {
              return true;
            }
            this.applyProfile(updatedProfile, true);
            this.setSettingsNotice(NOTICE_KINDS.SUCCESS, COPY.defaultsSaved);
            return true;
          });
          if (!profileApplied) {
            return false;
          }
        } catch (requestError) {
          if (this.canApplyRoutingDefaultsAutosave(appVersion)) {
            this.routingDefaultsDirty = true;
            this.setSettingsNotice(NOTICE_KINDS.ERROR, profileFailureMessage(requestError));
          }
          return false;
        }
      }
      return true;
    },

    /**
     * @param {number} appVersion
     * @returns {boolean}
     */
    canApplyRoutingDefaultsAutosave(appVersion) {
      return (
        this.settingsOpen &&
        this.authState === AUTH_STATES.AUTHENTICATED &&
        this.appVersion === appVersion
      );
    },

    /** @param {Event} event */
    async handleTextProviderDefaultChange(event) {
      const providerSelect = /** @type {HTMLSelectElement} */ (event.target);
      const providerIdentifier = providerSelect.value;
      const selectionVersion = this.routingProviderSelectionVersion + 1;
      const appVersion = this.appVersion;
      const tenantIdentifier = this.settingsTenantID;
      this.routingProviderSelectionVersion = selectionVersion;
      this.defaults.provider = providerIdentifier;
      const initialProvider = profileProvider(this.providers, providerIdentifier);
      this.defaults.model = initialProvider.text_model;
      this.normalizeReasoningEffortDefault();
      const matchingProviderAutosave = providerIdentifier === this.selectedProviderID
        ? this.providerAutosavePromise
        : null;
      if (matchingProviderAutosave) {
        this.routingProviderSelectionPending = true;
        try {
          await matchingProviderAutosave;
        } finally {
          if (this.routingProviderSelectionVersion === selectionVersion) {
            this.routingProviderSelectionPending = false;
          }
        }
      }
      if (
        !this.settingsOpen ||
        this.authState !== AUTH_STATES.AUTHENTICATED ||
        this.appVersion !== appVersion ||
        this.settingsTenantID !== tenantIdentifier ||
        this.routingProviderSelectionVersion !== selectionVersion
      ) {
        return;
      }
      this.defaults.provider = providerIdentifier;
      const provider = profileProvider(this.providers, providerIdentifier);
      this.defaults.model = provider.text_model;
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
        throw new Error(APP_INTEGRITY_ERROR);
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
  });
}
