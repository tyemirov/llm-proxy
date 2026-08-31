// @ts-check

import { COPY, NOTICE_KINDS } from "../constants.js?v=20260811c131";
import {
  removeProviderConnection as requestRemoveProviderConnection,
  revealProviderConnectionField as requestRevealProviderConnectionField,
} from "../core/backendClient.js?v=20260811c131";
import {
  isAbortError,
  profileFailureMessage,
  profileProvider,
} from "../core/managementProfile.js?v=20260811c131";

const EMPTY_STRING = "";
const MASKED_PROVIDER_FIELD_PREFIX = "****";
const MASKED_PROVIDER_FIELD_FINAL_CHARACTER_COUNT = 4;
const SAVED_PROVIDER_FIELD_MASK = "saved";

/** @typedef {ReturnType<typeof import("./managementApplicationState.js").createManagementApplicationState>} ManagementApplicationState */
/** @typedef {ManagementApplicationState & import("../types.d.js").AlpineMagic & {
 *   selectedProvider: import("../types.d.js").ProviderProfile | null,
 *   selectedProviderID: string,
 *   settingsRequired: boolean,
 *   abortProviderKeyVerification: () => void,
 *   autosaveSelectedProvider: () => Promise<boolean>,
 *   focusSettingsRequirement: () => void,
 *   markSelectedProviderDirty: () => void,
 *   replaceProviderEditorSession: (providerID: string) => void,
 *   runProfileMutation: (mutation: () => Promise<import("../types.d.js").ManagementTenantProfile>, successMessage: string) => Promise<boolean>,
 *   setSettingsNotice: (kind: string, message: string) => void
 * }} ProviderCredentialsHost */

/**
 * @template {object} Responsibility
 * @param {Responsibility & ThisType<ProviderCredentialsHost & Responsibility>} responsibility
 * @returns {Responsibility}
 */
function providerCredentialsResponsibility(responsibility) {
  return responsibility;
}

/** Create catalog-field entry, reveal, verification, and provider removal behavior. */
export function createProviderCredentialsResponsibility() {
  return providerCredentialsResponsibility({
    get providerKeyRevealPending() {
      return this.providerEditorSession.revealPendingField !== EMPTY_STRING;
    },

    get providerKeyVerificationFailed() {
      return this.providerKeyVerificationFailure !== EMPTY_STRING;
    },

    get providerRemovalConfirmationOpen() {
      return this.providerRemovalConfirmationProviderID !== EMPTY_STRING;
    },

    get selectedProviderHasConnectionInput() {
      return Object.values(this.providerEditorSession.fieldInputs).some((value) => value.trim() !== EMPTY_STRING);
    },

    /** @param {import("../types.d.js").ProviderFieldProfile} field */
    providerFieldVisible(field) {
      return Boolean(this.providerEditorSession.fieldVisible[field.id]);
    },

    /** @param {import("../types.d.js").ProviderFieldProfile} field */
    providerFieldHasInput(field) {
      return String(this.providerEditorSession.fieldInputs[field.id] ?? EMPTY_STRING) !== EMPTY_STRING;
    },

    /** @param {import("../types.d.js").ProviderFieldProfile} field */
    providerFieldInputValue(field) {
      const input = String(this.providerEditorSession.fieldInputs[field.id] ?? EMPTY_STRING);
      if (!field.secret || this.providerFieldVisible(field) || (!field.configured && input === EMPTY_STRING)) {
        return input;
      }
      const maskedValue = String(field.masked_value || EMPTY_STRING);
      if (input === EMPTY_STRING && maskedValue === SAVED_PROVIDER_FIELD_MASK) {
        return MASKED_PROVIDER_FIELD_PREFIX;
      }
      return maskedProviderField(input || maskedValue);
    },

    /** @param {import("../types.d.js").ProviderFieldProfile} field */
    providerFieldInputReadOnly(field) {
      return Boolean(field.secret && !this.providerFieldVisible(field) && (field.configured || this.providerFieldHasInput(field)));
    },

    /** @param {import("../types.d.js").ProviderFieldProfile} field */
    providerFieldActionCopy(field) {
      return this.providerFieldVisible(field) ? COPY.hideProviderKey : COPY.showProviderKey;
    },

    /** @param {import("../types.d.js").ProviderFieldProfile} field */
    async handleProviderFieldAction(field) {
      if (!field.secret) {
        return;
      }
      if (this.providerFieldHasInput(field)) {
        this.providerEditorSession.fieldVisible[field.id] = !this.providerFieldVisible(field);
        return;
      }
      if (field.configured) {
        await this.revealProviderField(field);
      }
    },

    /**
     * @param {import("../types.d.js").ProviderFieldProfile} field
     * @param {Event} event
     */
    handleProviderFieldInput(field, event) {
      const provider = this.selectedProvider;
      if (!provider || !provider.fields.some((candidate) => candidate.id === field.id)) {
        return;
      }
      this.abortProviderKeyVerification();
      this.providerKeyVerificationFailure = EMPTY_STRING;
      const input = /** @type {HTMLInputElement} */ (event.target);
      this.providerEditorSession.fieldInputs[field.id] = input.value;
      this.providerEditorSession.fieldVisible[field.id] = field.secret;
      this.providerEditorSession.fieldDirty[field.id] = true;
      this.markSelectedProviderDirty();
    },

    /** @param {import("../types.d.js").ProviderFieldProfile} field */
    handleProviderFieldPaste(field) {
      if (!field.secret) {
        return;
      }
      this.$nextTick(() => {
        void this.verifyPastedProviderField(field.id);
      });
    },

    /** @param {string} fieldID */
    async verifyPastedProviderField(fieldID) {
      if (this.providerAutosavePromise) {
        await this.providerAutosavePromise;
      }
      if (
        this.providerEditorSession.fieldDirty[fieldID] &&
        String(this.providerEditorSession.fieldInputs[fieldID] ?? EMPTY_STRING).trim() !== EMPTY_STRING
      ) {
        await this.autosaveSelectedProvider();
      }
    },

    async retrySelectedProviderKeyVerification() {
      this.providerKeyVerificationFailure = EMPTY_STRING;
      await this.autosaveSelectedProvider();
    },

    /** @param {import("../types.d.js").ProviderFieldProfile} field */
    async revealProviderField(field) {
      const provider = this.selectedProvider;
      if (!provider || !field.secret || !field.configured || this.providerKeyRevealPending) {
        return;
      }
      const revealProviderID = provider.id;
      const revealFieldID = field.id;
      const revealVersion = this.providerEditorSession.revealVersion + 1;
      const tenantID = this.settingsTenantID;
      const appVersion = this.appVersion;
      const lifetimeController = this.tenantLifetimeController;
      if (!lifetimeController) {
        return;
      }
      this.providerEditorSession.revealVersion = revealVersion;
      this.providerEditorSession.revealPendingField = revealFieldID;
      try {
        const revealResponse = await requestRevealProviderConnectionField(
          tenantID,
          revealProviderID,
          revealFieldID,
          lifetimeController.signal,
        );
        if (
          revealResponse.field_id !== revealFieldID ||
          !this.canApplyProviderFieldReveal(tenantID, appVersion, revealProviderID, revealFieldID, revealVersion)
        ) {
          return;
        }
        this.providerEditorSession.fieldInputs[revealFieldID] = revealResponse.value;
        this.providerEditorSession.fieldVisible[revealFieldID] = true;
      } catch (requestError) {
        if (
          !isAbortError(requestError) &&
          this.canApplyProviderFieldReveal(tenantID, appVersion, revealProviderID, revealFieldID, revealVersion)
        ) {
          this.setSettingsNotice(NOTICE_KINDS.ERROR, profileFailureMessage(requestError));
        }
      } finally {
        if (revealVersion === this.providerEditorSession.revealVersion) {
          this.providerEditorSession.revealPendingField = EMPTY_STRING;
        }
      }
    },

    /**
     * @param {string} tenantID
     * @param {number} appVersion
     * @param {string} providerID
     * @param {string} fieldID
     * @param {number} revealVersion
     */
    canApplyProviderFieldReveal(tenantID, appVersion, providerID, fieldID, revealVersion) {
      return (
        this.settingsOpen &&
        this.settingsTenantID === tenantID &&
        this.appVersion === appVersion &&
        this.selectedProviderID === providerID &&
        this.providerEditorSession.revealPendingField === fieldID &&
        this.providerEditorSession.revealVersion === revealVersion
      );
    },

    clearProviderKeyMaterial() {
      this.replaceProviderEditorSession(this.selectedProviderID);
    },

    abortProviderKeyVerification() {
      if (this.providerKeyVerificationController) {
        this.providerKeyVerificationController.abort();
        this.providerKeyVerificationController = null;
      }
      this.providerKeyVerificationPending = false;
    },

    requestSelectedProviderKeyRemoval() {
      const provider = this.selectedProvider;
      if (!provider) {
        return;
      }
      if (!provider.configured) {
        this.clearProviderKeyMaterial();
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
        this.$refs.providerSelector.focus();
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

    /** @param {KeyboardEvent} event */
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

    /** @param {import("../types.d.js").ProviderProfile} provider */
    async removeProviderKey(provider) {
      const tenantID = this.settingsTenantID;
      const lifetimeController = this.tenantLifetimeController;
      if (!lifetimeController) {
        return;
      }
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

/** @param {string} fieldValue */
function maskedProviderField(fieldValue) {
  return `${MASKED_PROVIDER_FIELD_PREFIX}${fieldValue.slice(-MASKED_PROVIDER_FIELD_FINAL_CHARACTER_COUNT)}`;
}
