// @ts-check

import { AUTH_STATES, COPY } from "../constants.js?v=20260811c131";

/** @typedef {ReturnType<typeof import("./managementApplicationState.js").createManagementApplicationState>} ManagementApplicationState */
/** @typedef {ManagementApplicationState & { hasSecret: boolean }} ManagementApplicationPresentationHost */

/**
 * @template {object} Responsibility
 * @param {Responsibility & ThisType<ManagementApplicationPresentationHost & Responsibility>} responsibility
 * @returns {Responsibility}
 */
function managementApplicationPresentationResponsibility(responsibility) {
  return responsibility;
}

/** Create cross-responsibility Settings readiness and control presentation. */
export function createManagementApplicationPresentationResponsibility() {
  return managementApplicationPresentationResponsibility({
    get hasUnsavedSettingsChanges() {
      return Boolean(
        this.settingsOpen &&
        (
          this.tenantNameDirty ||
          this.providerEditorSession.dirty ||
          this.providerAutosavePending ||
          this.providerKeyVerificationPending ||
          this.routingDefaultsDirty ||
          this.routingDefaultsAutosavePending
        )
      );
    },

    get hasSavedProviderKey() {
      return this.providers.some((provider) => provider.configured);
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
      return (
        this.busy ||
        this.settingsClosePending ||
        this.providerKeyVerificationPending ||
        this.routingProviderSelectionPending
      );
    },
  });
}
