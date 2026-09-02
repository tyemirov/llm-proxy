// @ts-check

import { COPY } from "../constants.js?v=20260902a237";
import { removeProviderConnection as requestRemoveProviderConnection } from "../core/backendClient.js?v=20260902a237";
import { profileProvider } from "../core/managementProfile.js?v=20260902a237";

const EMPTY_STRING = "";
const SAVED_PROVIDER_KEY_MASK = "••••••••";

/** @typedef {ReturnType<typeof import("./managementApplicationState.js").createManagementApplicationState>} ManagementApplicationState */
/** @typedef {ManagementApplicationState & import("../types.d.js").AlpineMagic & {
 *   selectedProvider: import("../types.d.js").ProviderProfile | null,
 *   selectedProviderID: string,
 *   autosaveSelectedProvider: () => Promise<boolean>,
 *   markSelectedProviderDirty: () => void,
 *   replaceProviderEditorSession: (providerID: string) => void,
 *   runProfileMutation: (mutation: () => Promise<import("../types.d.js").ManagementTenantProfile>, successMessage: string) => Promise<boolean>
 * }} ProviderCredentialsHost */

/** @template {object} Responsibility @param {Responsibility & ThisType<ProviderCredentialsHost & Responsibility>} responsibility */
function providerCredentialsResponsibility(responsibility) {
  return responsibility;
}

/** Create candidate-key verification and credential-only deletion behavior. */
export function createProviderCredentialsResponsibility() {
  return providerCredentialsResponsibility({
    get providerKeyRevealPending() { return false; },
    get providerKeyVerificationFailed() { return this.providerKeyVerificationFailure !== EMPTY_STRING; },
    get providerRemovalConfirmationOpen() { return this.providerRemovalConfirmationProviderID !== EMPTY_STRING; },
    get selectedProviderHasConnectionInput() {
      return Object.values(this.providerEditorSession.fieldInputs).some((value) => value.trim() !== EMPTY_STRING);
    },

    /** @param {import("../types.d.js").ProviderFieldProfile} field */
    providerFieldHasInput(field) {
      return String(this.providerEditorSession.fieldInputs[field.id] ?? EMPTY_STRING) !== EMPTY_STRING;
    },

    /** @param {import("../types.d.js").ProviderFieldProfile} field */
    providerFieldInputValue(field) {
      if (field.secret && field.configured && !this.providerEditorSession.fieldDirty[field.id]) return SAVED_PROVIDER_KEY_MASK;
      return String(this.providerEditorSession.fieldInputs[field.id] ?? EMPTY_STRING);
    },

    /** @param {import("../types.d.js").ProviderFieldProfile} field */
    providerFieldInputReadOnly(field) {
      return Boolean(field.secret && field.configured && !this.providerEditorSession.fieldDirty[field.id]);
    },

    /** @param {import("../types.d.js").ProviderFieldProfile} field */
    beginProviderKeyReplacement(field) {
      if (!field.secret) return;
      this.abortProviderKeyVerification();
      this.providerKeyVerificationFailure = EMPTY_STRING;
      this.providerEditorSession.fieldInputs[field.id] = EMPTY_STRING;
      this.providerEditorSession.fieldDirty[field.id] = true;
      this.$nextTick(() => {
        const input = document.getElementById(`provider-card-field-${this.selectedProviderID}-${field.id}`);
        if (input instanceof HTMLInputElement) input.focus();
      });
    },

    /** @param {import("../types.d.js").ProviderFieldProfile} field @param {Event} event */
    handleProviderFieldInput(field, event) {
      const provider = this.selectedProvider;
      if (!provider || !provider.fields.some((candidate) => candidate.id === field.id)) return;
      this.abortProviderKeyVerification();
      this.providerKeyVerificationFailure = EMPTY_STRING;
      this.providerEditorSession.fieldInputs[field.id] = /** @type {HTMLInputElement} */ (event.target).value;
      this.providerEditorSession.fieldDirty[field.id] = true;
      this.markSelectedProviderDirty();
    },

    /** @param {import("../types.d.js").ProviderFieldProfile} field */
    handleProviderFieldPaste(field) {
      if (!field.secret) return;
      this.$nextTick(() => { void this.verifyPastedProviderField(field.id); });
    },

    /** @param {string} fieldID */
    async verifyPastedProviderField(fieldID) {
      if (this.providerAutosavePromise) await this.providerAutosavePromise;
      if (this.providerEditorSession.fieldDirty[fieldID] && String(this.providerEditorSession.fieldInputs[fieldID] ?? EMPTY_STRING).trim()) {
        await this.autosaveSelectedProvider();
      }
    },

    async retrySelectedProviderKeyVerification() {
      this.providerKeyVerificationFailure = EMPTY_STRING;
      await this.autosaveSelectedProvider();
    },

    clearProviderKeyMaterial() { this.replaceProviderEditorSession(this.selectedProviderID); },

    abortProviderKeyVerification() {
      if (this.providerKeyVerificationController) {
        this.providerKeyVerificationController.abort();
        this.providerKeyVerificationController = null;
      }
      this.providerKeyVerificationPending = false;
    },

    requestSelectedProviderKeyRemoval() {
      const provider = this.selectedProvider;
      if (!provider || !provider.configured) return;
      this.providerRemovalConfirmationProviderID = provider.id;
      this.$nextTick(() => { this.$refs.providerRemovalCancel.focus(); });
    },

    dismissProviderKeyRemovalConfirmation() { this.providerRemovalConfirmationProviderID = EMPTY_STRING; },
    cancelProviderKeyRemoval() {
      const providerID = this.providerRemovalConfirmationProviderID;
      this.dismissProviderKeyRemovalConfirmation();
      this.$nextTick(() => {
        const button = document.querySelector(`[data-provider-card="${CSS.escape(providerID)}"] .provider-key-remove`);
        if (button instanceof HTMLElement) button.focus();
      });
    },

    async confirmProviderKeyRemoval() {
      const provider = profileProvider(this.providers, this.providerRemovalConfirmationProviderID);
      this.dismissProviderKeyRemovalConfirmation();
      await this.removeProviderKey(provider);
    },

    /** @param {KeyboardEvent} event */
    trapProviderKeyRemovalFocus(event) {
      const cancelButton = this.$refs.providerRemovalCancel;
      const confirmButton = this.$refs.providerRemovalConfirm;
      if (event.shiftKey && document.activeElement === cancelButton) {
        event.preventDefault(); confirmButton.focus(); return;
      }
      if (!event.shiftKey && document.activeElement === confirmButton) {
        event.preventDefault(); cancelButton.focus();
      }
    },

    /** @param {import("../types.d.js").ProviderProfile} provider */
    async removeProviderKey(provider) {
      const tenantID = this.providerCardTenantID;
      const lifetimeController = this.tenantLifetimeController;
      if (!tenantID || !lifetimeController) return;
      try {
        await this.runProfileMutation(
          async () => requestRemoveProviderConnection(tenantID, provider.id, lifetimeController.signal),
          COPY.providerKeyRemoved,
        );
      } finally {
        this.clearProviderKeyMaterial();
      }
    },
  });
}
