// @ts-check

import {
  AUTH_STATES,
  MENU_ACTIONS,
  NOTICE_KINDS,
} from "../constants.js?v=20260902a237";

/** @typedef {ReturnType<typeof import("./managementApplicationState.js").createManagementApplicationState>} ManagementApplicationState */
/** @typedef {ManagementApplicationState & import("../types.d.js").AlpineMagic & {
 *   hasGeneratedSecret: boolean,
 *   hasSecret: boolean,
 *   providerRemovalConfirmationOpen: boolean,
 *   settingsRequired: boolean,
 *   settingsRequirementCopy: string,
 *   autosaveRoutingDefaults: () => Promise<boolean>,
 *   autosaveSelectedProvider: () => Promise<boolean>,
 *   cancelClientKeyReplacement: () => void,
 *   cancelProviderKeyRemoval: () => void,
 *   cancelTenantDeletion: () => void,
 *   cancelTenantNameEdit: () => void,
 *   cancelTenantSwitch: () => void,
 *   clearGeneratedSecret: () => void,
 *   clearProviderKeyMaterial: () => void,
 *   clearUsageFailures: (restoreFocus: boolean) => void,
 *   closeCreateTenantDialog: () => void,
 *   dismissClientKeyReplacementConfirmation: () => void,
 *   dismissProviderKeyRemovalConfirmation: () => void,
 *   openAdminDashboard: () => Promise<void>,
 *   resetTenantNameEdit: () => void,
 *   setSettingsNotice: (kind: string, message: string) => void,
 *   waitForProfileMutations: () => Promise<void>
 * }} SettingsDialogHost */

/**
 * @template {object} Responsibility
 * @param {Responsibility & ThisType<SettingsDialogHost & Responsibility>} responsibility
 * @returns {Responsibility}
 */
function settingsDialogResponsibility(responsibility) {
  return responsibility;
}

/** Create Settings modal orchestration and mandatory-setup behavior. */
export function createSettingsDialogResponsibility() {
  return settingsDialogResponsibility({
    /** @param {Event} event */
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

    collapseSystemPromptEditors() {
      this.routingSystemPromptOpen = false;
      this.providerSystemPromptOpen = false;
    },

    openSettings() {
      this.clearUsageFailures(false);
      this.usageExamplesOpen = false;
      this.collapseSystemPromptEditors();
      this.dismissProviderKeyRemovalConfirmation();
      this.dismissClientKeyReplacementConfirmation();
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
          this.setSettingsNotice(NOTICE_KINDS.ERROR, this.settingsRequirementCopy);
          this.focusSettingsRequirement();
          return;
        }
        this.dismissProviderKeyRemovalConfirmation();
        this.dismissClientKeyReplacementConfirmation();
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

    /** @param {KeyboardEvent} event */
    trapSettingsFocus(event) {
      if (!this.settingsRequired) {
        return;
      }
      const focusableControls = [.../** @type {NodeListOf<HTMLElement>} */ (this.$refs.settingsModal.querySelectorAll(
        'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), summary, [tabindex]:not([tabindex="-1"])',
      ))].filter((control) => control.getClientRects().length > 0);
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

    handleSettingsEscape() {
      if (this.createTenantDialogOpen) {
        this.closeCreateTenantDialog();
        return;
      }
      if (this.discardTenantChangesOpen) {
        this.cancelTenantSwitch();
        return;
      }
      if (this.tenantRenameDialogOpen) {
        this.cancelTenantNameEdit();
        return;
      }
      if (this.clientKeyReplacementConfirmationOpen) {
        this.cancelClientKeyReplacement();
        return;
      }
      if (this.deleteTenantConfirmationOpen) {
        this.cancelTenantDeletion();
        return;
      }
      if (this.providerRemovalConfirmationOpen) {
        this.cancelProviderKeyRemoval();
        return;
      }
      void this.closeSettings();
    },
  });
}
