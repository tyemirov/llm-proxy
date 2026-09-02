// @ts-check

import { AUTH_STATES } from "../constants.js?v=20260902c239";
import { profileProvider } from "../core/managementProfile.js?v=20260902c239";

const EMPTY_STRING = "";

/** @typedef {ReturnType<typeof import("./managementApplicationState.js").createManagementApplicationState>} ManagementApplicationState */
/** @typedef {ManagementApplicationState & import("../types.d.js").AlpineMagic & {
 *   abortProviderKeyVerification: () => void,
 *   autosaveSelectedProvider: () => Promise<boolean>,
 *   dismissProviderKeyRemovalConfirmation: () => void
 * }} ProviderEditorHost */

/**
 * @template {object} Responsibility
 * @param {Responsibility & ThisType<ProviderEditorHost & Responsibility>} responsibility
 * @returns {Responsibility}
 */
function providerEditorResponsibility(responsibility) {
  return responsibility;
}

/** Create provider selection and editable provider-default state behavior. */
export function createProviderEditorResponsibility() {
  return providerEditorResponsibility({
    /** @returns {import("../types.d.js").ProviderProfile | null} */
    get selectedProvider() {
      const providers = this.providerCardProfile ? this.providerCardProfile.providers : this.providers;
      return providers.find((candidateProvider) => candidateProvider.id === this.selectedProviderID) || null;
    },

    get selectedProviderID() {
      return this.providerEditorSession.providerID;
    },

    /** @param {string} providerID */
    async selectProvider(providerID) {
      profileProvider(this.providers, providerID);
      if (providerID === this.selectedProviderID) {
        return;
      }
      this.abortProviderKeyVerification();
      this.restoreSelectedProviderControl();
      if (!(await this.autosaveSelectedProvider())) {
        this.restoreSelectedProviderControl();
        return;
      }
      if (!this.providerCardTenantID || this.authState !== AUTH_STATES.AUTHENTICATED) {
        return;
      }
      this.dismissProviderKeyRemovalConfirmation();
      this.replaceProviderEditorSession(providerID);
    },

    restoreSelectedProviderControl() {
      this.$nextTick(() => {
        if (this.providerCardTenantID && this.$refs.providerSelector) {
          this.$refs.providerSelector.value = this.selectedProviderID;
        }
      });
    },

    /** @param {Event} event */
    handleSelectedProviderTextModelChange(event) {
      this.abortProviderKeyVerification();
      this.providerKeyVerificationFailure = EMPTY_STRING;
      const modelSelect = /** @type {HTMLSelectElement} */ (event.target);
      this.providerEditorSession.textModel = modelSelect.value;
      this.markSelectedProviderDirty();
      void this.autosaveSelectedProvider();
    },

    /** @param {Event} event */
    handleSelectedProviderSystemPromptInput(event) {
      const systemPromptInput = /** @type {HTMLTextAreaElement} */ (event.target);
      this.providerEditorSession.systemPrompt = systemPromptInput.value;
      this.markSelectedProviderDirty();
    },

    markSelectedProviderDirty() {
      this.providerEditorSession.dirty = true;
      this.providerEditorSession.editVersion += 1;
    },

    /** @param {string} providerID */
    replaceProviderEditorSession(providerID) {
      this.abortProviderKeyVerification();
      this.providerKeyVerificationFailure = EMPTY_STRING;
      const providerChanged = providerID !== this.selectedProviderID;
      const providers = this.providerCardProfile ? this.providerCardProfile.providers : this.providers;
      const provider = providerID === EMPTY_STRING ? null : profileProvider(providers, providerID);
      this.providerEditorSession = createProviderEditorSession(
        providerID,
        this.providerEditorSession.revealVersion + 1,
        provider ? provider.fields : [],
        provider ? provider.text_model : EMPTY_STRING,
        provider ? provider.system_prompt : EMPTY_STRING,
      );
      if (providerChanged) {
        this.providerSystemPromptOpen = false;
      }
    },
  });
}

/**
 * Create a provider editor session whose raw key material is browser-memory only.
 *
 * @param {string} providerID
 * @param {number} revealVersion
 * @param {import("../types.d.js").ProviderFieldProfile[]} [fields]
 * @param {string} [textModel]
 * @param {string} [systemPrompt]
 * @returns {import("../types.d.js").ProviderEditorSession}
 */
export function createProviderEditorSession(providerID, revealVersion, fields = [], textModel = EMPTY_STRING, systemPrompt = EMPTY_STRING) {
  /** @type {Record<string, string>} */
  const fieldInputs = {};
  for (const field of fields) {
    fieldInputs[field.id] = field.secret ? EMPTY_STRING : String(field.value);
  }
  return {
    providerID,
    fieldInputs,
    fieldDirty: {},
    fieldVisible: {},
    textModel,
    systemPrompt,
    dirty: false,
    editVersion: 0,
    revealPendingField: EMPTY_STRING,
    revealVersion,
  };
}
