// @ts-check

import { AUTH_STATES, COPY, NOTICE_KINDS } from "../constants.js?v=20260811c131";
import { generateSecret as requestGeneratedSecret } from "../core/backendClient.js?v=20260811c131";
import { profileFailureMessage } from "../core/managementProfile.js?v=20260811c131";
import { trapDialogFocus } from "./dialogFocus.js?v=20260811c131";

const EMPTY_STRING = "";
const MASKED_CLIENT_KEY = "••••••••••••";

/** @typedef {ReturnType<typeof import("./managementApplicationState.js").createManagementApplicationState>} ManagementApplicationState */
/** @typedef {ManagementApplicationState & import("../types.d.js").AlpineMagic & {
 *   applyProfile: (profile: import("../types.d.js").ManagementTenantProfile, preserveProviderEditor?: boolean, preserveRoutingDefaults?: boolean) => void,
 *   enqueueProfileMutation: (appVersion: number, mutation: () => Promise<boolean>) => Promise<boolean | null>,
 *   setSettingsNotice: (kind: string, message: string) => void
 * }} ClientAccessHost */

/**
 * @template {object} Responsibility
 * @param {Responsibility & ThisType<ClientAccessHost & Responsibility>} responsibility
 * @returns {Responsibility}
 */
function clientAccessResponsibility(responsibility) {
  return responsibility;
}

/** Create one-time client-key generation, replacement, reveal, and copy behavior. */
export function createClientAccessResponsibility() {
  return clientAccessResponsibility({
    get hasSecret() {
      return Boolean(this.profile && this.profile.tenant.has_secret);
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

    /**
     * @param {string} [successMessage]
     * @returns {Promise<boolean>}
     */
    async requestAndApplyGeneratedSecret(successMessage = COPY.keyGenerated) {
      return this.runClientKeyMutation(async () => this.generateAndApplySecret(successMessage));
    },

    /**
     * @param {string} successMessage
     * @returns {Promise<boolean>}
     */
    async generateAndApplySecret(successMessage) {
      const generatedSecretVersion = this.generatedSecretVersion;
      const appVersion = this.appVersion;
      const tenantID = this.settingsTenantID;
      const lifetimeController = this.tenantLifetimeController;
      if (!lifetimeController) {
        return false;
      }
      try {
        const profileApplied = await this.enqueueProfileMutation(appVersion, async () => {
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
          this.setSettingsNotice(NOTICE_KINDS.SUCCESS, successMessage);
          return true;
        });
        return Boolean(profileApplied);
      } catch (requestError) {
        if (this.canApplyGeneratedSecret(generatedSecretVersion)) {
          this.setSettingsNotice(NOTICE_KINDS.ERROR, profileFailureMessage(requestError));
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

    /**
     * @param {string} [successMessage]
     * @returns {Promise<boolean>}
     */
    async generateSecret(successMessage = COPY.keyGenerated) {
      this.busy = true;
      try {
        return await this.requestAndApplyGeneratedSecret(successMessage);
      } finally {
        this.busy = false;
      }
    },

    requestClientKeyReplacement() {
      if (!this.hasSecret || this.clientKeyReplacementPending) {
        return;
      }
      this.clientKeyReplacementConfirmationOpen = true;
      this.$nextTick(() => {
        this.$refs.clientKeyReplacementCancel.focus();
      });
    },

    dismissClientKeyReplacementConfirmation() {
      this.clientKeyReplacementConfirmationOpen = false;
    },

    cancelClientKeyReplacement() {
      if (this.clientKeyReplacementPending) {
        return;
      }
      this.dismissClientKeyReplacementConfirmation();
      this.$nextTick(() => {
        if (this.$refs.clientKeyReplaceButton) {
          this.$refs.clientKeyReplaceButton.focus();
        }
      });
    },

    async confirmClientKeyReplacement() {
      if (!this.hasSecret || this.clientKeyReplacementPending) {
        return;
      }
      this.clientKeyReplacementPending = true;
      try {
        const keyReplaced = await this.generateSecret(COPY.keyReplaced);
        if (!keyReplaced) {
          return;
        }
        this.dismissClientKeyReplacementConfirmation();
        this.$nextTick(() => {
          requestAnimationFrame(() => {
            if (this.$refs.clientKeyCopyButton) {
              this.$refs.clientKeyCopyButton.focus();
            }
          });
        });
      } finally {
        this.clientKeyReplacementPending = false;
      }
    },

    /** @param {KeyboardEvent} event */
    trapClientKeyReplacementFocus(event) {
      trapDialogFocus(event, this.$refs.clientKeyReplacementDialog);
    },

    toggleGeneratedSecretVisibility() {
      if (!this.generatedSecret) {
        return;
      }
      this.generatedSecretVisible = !this.generatedSecretVisible;
    },

    async copyGeneratedSecret() {
      if (!this.generatedSecret || !navigator.clipboard) {
        this.setSettingsNotice(NOTICE_KINDS.ERROR, COPY.copyUnavailable);
        return;
      }
      await navigator.clipboard.writeText(this.generatedSecret);
      this.setSettingsNotice(NOTICE_KINDS.SUCCESS, COPY.keyCopied);
    },
  });
}
