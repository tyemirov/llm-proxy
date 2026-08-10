// @ts-check

import { COPY, NOTICE_KINDS } from "../constants.js?v=20260809i217";
import {
  removeProviderKey as requestRemoveProviderKey,
  revealProviderKey as requestRevealProviderKey,
} from "../core/backendClient.js?v=20260809i217";
import {
  isAbortError,
  profileFailureMessage,
  profileProvider,
} from "../core/managementProfile.js?v=20260809i217";

const EMPTY_STRING = "";
const MASKED_PROVIDER_KEY_PREFIX = "****";
const MASKED_PROVIDER_KEY_FINAL_CHARACTER_COUNT = 4;
const SAVED_PROVIDER_KEY_MASK = "saved";

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

/** Create provider credential entry, reveal, verification, and removal behavior. */
export function createProviderCredentialsResponsibility() {
  return providerCredentialsResponsibility({
    get providerKeyVisible() {
      return this.providerEditorSession.keyVisible;
    },

    get providerKeyRevealPending() {
      return this.providerEditorSession.revealPending;
    },

    get providerKeyVerificationFailed() {
      return this.providerKeyVerificationFailure !== EMPTY_STRING;
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

    /** @param {Event} event */
    handleSelectedProviderKeyInput(event) {
      const provider = this.selectedProvider;
      if (!provider) {
        return;
      }
      this.abortProviderKeyVerification();
      this.providerKeyVerificationFailure = EMPTY_STRING;
      const keyInput = /** @type {HTMLInputElement} */ (event.target);
      this.providerEditorSession.keyInput = keyInput.value;
      this.providerEditorSession.keyVisible = true;
      this.providerEditorSession.keyDirty = true;
      this.markSelectedProviderDirty();
    },

    handleSelectedProviderKeyPaste() {
      this.$nextTick(() => {
        void this.verifyPastedProviderKey();
      });
    },

    async verifyPastedProviderKey() {
      if (this.providerAutosavePromise) {
        await this.providerAutosavePromise;
      }
      if (
        this.providerEditorSession.keyDirty &&
        this.providerEditorSession.keyInput.trim() !== EMPTY_STRING
      ) {
        await this.autosaveSelectedProvider();
      }
    },

    async retrySelectedProviderKeyVerification() {
      this.providerKeyVerificationFailure = EMPTY_STRING;
      await this.autosaveSelectedProvider();
    },

    async revealSelectedProviderKey() {
      const provider = this.selectedProvider;
      if (!provider || !provider.has_key || this.providerKeyRevealPending) {
        return;
      }
      const revealProviderID = provider.id;
      const revealVersion = this.providerEditorSession.revealVersion + 1;
      const tenantID = this.settingsTenantID;
      const appVersion = this.appVersion;
      const lifetimeController = this.tenantLifetimeController;
      if (!lifetimeController) {
        return;
      }
      this.providerEditorSession.revealVersion = revealVersion;
      this.providerEditorSession.revealPending = true;
      try {
        const revealResponse = await requestRevealProviderKey(tenantID, revealProviderID, lifetimeController.signal);
        if (!this.canApplyProviderKeyReveal(tenantID, appVersion, revealProviderID, revealVersion)) {
          return;
        }
        this.providerEditorSession.keyInput = revealResponse.api_key;
        this.providerEditorSession.keyVisible = true;
      } catch (requestError) {
        if (
          !isAbortError(requestError) &&
          this.canApplyProviderKeyReveal(tenantID, appVersion, revealProviderID, revealVersion)
        ) {
          this.setSettingsNotice(NOTICE_KINDS.ERROR, profileFailureMessage(requestError));
        }
      } finally {
        if (revealVersion === this.providerEditorSession.revealVersion) {
          this.providerEditorSession.revealPending = false;
        }
      }
    },

    /**
     * @param {string} tenantID
     * @param {number} appVersion
     * @param {string} providerID
     * @param {number} revealVersion
     */
    canApplyProviderKeyReveal(tenantID, appVersion, providerID, revealVersion) {
      return (
        this.settingsOpen &&
        this.settingsTenantID === tenantID &&
        this.appVersion === appVersion &&
        this.selectedProviderID === providerID &&
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
          async () => requestRemoveProviderKey(tenantID, provider.id, lifetimeController.signal),
          COPY.providerKeyRemoved,
        );
      } finally {
        this.clearProviderKeyMaterial();
      }
    },
  });
}

/** @param {string} keyValue */
function maskedProviderKey(keyValue) {
  return `${MASKED_PROVIDER_KEY_PREFIX}${keyValue.slice(-MASKED_PROVIDER_KEY_FINAL_CHARACTER_COUNT)}`;
}
