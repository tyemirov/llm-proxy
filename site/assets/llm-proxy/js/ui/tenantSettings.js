// @ts-check

import {
  AUTH_STATES,
  COPY,
  NOTICE_KINDS,
  NOTICE_SURFACES,
} from "../constants.js?v=20260902c237";
import {
  BackendClientError,
  createTenant as requestCreateTenant,
  deleteTenant as requestDeleteTenant,
  renameTenant as requestRenameTenant,
} from "../core/backendClient.js?v=20260902c237";
import {
  assertManagementTenantProfile,
  isAbortError,
  managementAccountWithTenants,
  profileFailureMessage,
  tenantSummaryFromProfile,
  validatedTenantName,
} from "../core/managementProfile.js?v=20260902c237";
import { dispatchManagementReady } from "../core/runtimeTransition.js?v=20260902c237";
import { trapDialogFocus } from "./dialogFocus.js?v=20260902c237";
import { emptyUsageSummary } from "./usagePresentation.js?v=20260902c237";

const EMPTY_STRING = "";

/** @typedef {ReturnType<typeof import("./managementApplicationState.js").createManagementApplicationState>} ManagementApplicationState */
/** @typedef {ManagementApplicationState & import("../types.d.js").AlpineMagic & {
 *   hasUnsavedSettingsChanges: boolean,
 *   usageScopeIsAllTenants: boolean,
 *   applyProfile: (profile: import("../types.d.js").ManagementTenantProfile, preserveProviderEditor?: boolean, preserveRoutingDefaults?: boolean) => void,
 *   canApplyAuthenticatedApp: (appVersion: number) => boolean,
 *   canApplySettingsTenant: (appVersion: number, tenantID: string) => boolean,
 *   clearGeneratedSecret: () => void,
 *   clearProviderKeyMaterial: () => void,
 *   clearUsageFailures: (restoreFocus: boolean) => void,
 *   dismissClientKeyReplacementConfirmation: () => void,
 *   dismissProviderKeyRemovalConfirmation: () => void,
 *   enqueueProfileMutation: (appVersion: number, mutation: () => Promise<boolean>) => Promise<boolean | null>,
 *   hydrateSettingsTenant: (profile: import("../types.d.js").ManagementTenantProfile | null, appVersion: number, noticeSurface: string) => Promise<void>,
 *   loadUsageSummary: (showSuccessNotice: boolean) => Promise<void>,
 *   setSettingsNotice: (kind: string, message: string) => void
 * }} TenantSettingsHost */

/**
 * @template {object} Responsibility
 * @param {Responsibility & ThisType<TenantSettingsHost & Responsibility>} responsibility
 * @returns {Responsibility}
 */
function tenantSettingsResponsibility(responsibility) {
  return responsibility;
}

/** Create Settings-tenant selection, creation, rename, and deletion behavior. */
export function createTenantSettingsResponsibility() {
  return tenantSettingsResponsibility({
    get settingsTenant() {
      return this.tenants.find((/** @type {import("../types.d.js").ManagementTenantSummary} */ tenant) => tenant.id === this.settingsTenantID) || null;
    },

    get settingsTenantName() {
      return this.settingsTenant ? this.settingsTenant.name : EMPTY_STRING;
    },

    get canDeleteSettingsTenant() {
      return this.tenants.length > 1;
    },

    get deleteTenantTitle() {
      return this.settingsTenant ? `Delete “${this.settingsTenant.name}”?` : COPY.deleteTenantTitle;
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

    /** @param {string} tenantID */
    async requestSettingsTenantSwitch(tenantID) {
      if (!this.tenants.some((tenant) => tenant.id === tenantID)) {
        this.restoreSettingsTenantSelector();
        this.setSettingsNotice(NOTICE_KINDS.ERROR, COPY.requestFailed);
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
      this.appVersion += 1;
      const appVersion = this.appVersion;
      if (this.tenantRequestController) {
        this.tenantRequestController.abort();
      }
      this.replaceTenantLifetimeController();
      this.clearGeneratedSecret();
      this.clearProviderKeyMaterial();
      this.dismissProviderKeyRemovalConfirmation();
      this.dismissClientKeyReplacementConfirmation();
      this.resetTenantNameEdit();
      this.deleteTenantConfirmationOpen = false;
      this.createTenantDialogOpen = false;
      this.createTenantName = EMPTY_STRING;
      this.createTenantError = EMPTY_STRING;
      this.discardTenantChangesOpen = false;
      this.pendingTenantID = EMPTY_STRING;
      this.settingsTenantID = tenantID;
      this.busy = true;
      try {
        await this.hydrateSettingsTenant(prefetchedProfile, appVersion, NOTICE_SURFACES.SETTINGS);
      } finally {
        if (this.appVersion === appVersion) {
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

    /** @param {KeyboardEvent} event */
    trapRenameTenantFocus(event) {
      trapDialogFocus(event, this.$refs.tenantRenameDialog);
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
      const appVersion = this.appVersion;
      const lifetimeController = this.tenantLifetimeController;
      if (!lifetimeController) {
        return;
      }
      this.createTenantPending = true;
      try {
        const createdProfile = await requestCreateTenant(name, lifetimeController.signal);
        if (
          !this.canApplyAuthenticatedApp(appVersion) ||
          this.tenantLifetimeController !== lifetimeController ||
          !this.createTenantDialogOpen
        ) {
          return;
        }
        assertManagementTenantProfile(createdProfile, createdProfile.tenant.id);
        const createdSummary = tenantSummaryFromProfile(createdProfile);
        this.tenants = [...this.tenants, createdSummary];
        this.account = managementAccountWithTenants(this.account, this.tenants);
        this.createTenantDialogOpen = false;
        this.createTenantName = EMPTY_STRING;
        await this.switchSettingsTenant(createdSummary.id, createdProfile);
        if (this.settingsTenantID === createdSummary.id && this.authState === AUTH_STATES.AUTHENTICATED) {
          this.setSettingsNotice(NOTICE_KINDS.SUCCESS, COPY.tenantCreated);
        }
      } catch (requestError) {
        if (!isAbortError(requestError) && this.tenantLifetimeController === lifetimeController) {
          this.createTenantError = requestError instanceof BackendClientError && requestError.status === 409
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
      this.tenantRenameDialogOpen = true;
      this.$nextTick(() => {
        this.$refs.tenantNameInput.focus();
      });
    },

    resetTenantNameEdit() {
      this.tenantNameDraft = this.settingsTenantName;
      this.tenantRenameDialogOpen = false;
      this.tenantNameDirty = false;
      this.tenantNameError = EMPTY_STRING;
    },

    cancelTenantNameEdit() {
      if (this.busy) {
        return;
      }
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
      const appVersion = this.appVersion;
      const lifetimeController = this.tenantLifetimeController;
      if (!lifetimeController || !this.tenantNameDirty) {
        return;
      }
      let tenantRenamed = false;
      this.busy = true;
      try {
        tenantRenamed = Boolean(await this.enqueueProfileMutation(appVersion, async () => {
          const updatedProfile = await requestRenameTenant(tenantID, name, lifetimeController.signal);
          if (!this.canApplySettingsTenant(appVersion, tenantID)) {
            return false;
          }
          assertManagementTenantProfile(updatedProfile, tenantID);
          this.tenants = this.tenants.map((tenant) => (
            tenant.id === tenantID ? tenantSummaryFromProfile(updatedProfile) : tenant
          ));
          this.account = managementAccountWithTenants(this.account, this.tenants);
          this.tenantNameDraft = updatedProfile.tenant.name;
          this.tenantRenameDialogOpen = false;
          this.tenantNameDirty = false;
          this.applyProfile(
            updatedProfile,
            this.providerEditorSession.dirty || this.providerAutosavePending,
            this.routingDefaultsDirty || this.routingDefaultsAutosavePending,
          );
          this.setSettingsNotice(NOTICE_KINDS.SUCCESS, COPY.tenantRenamed);
          return true;
        }));
      } catch (requestError) {
        if (!isAbortError(requestError) && this.canApplySettingsTenant(appVersion, tenantID)) {
          this.tenantNameError = requestError instanceof BackendClientError && requestError.status === 409
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
        this.setSettingsNotice(NOTICE_KINDS.ERROR, COPY.finalTenantDeletion);
        return;
      }
      this.deleteTenantConfirmationOpen = true;
      this.$nextTick(() => {
        this.$refs.deleteTenantCancel.focus();
      });
    },

    cancelTenantDeletion() {
      if (this.deleteTenantPending) {
        return;
      }
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
        this.account = managementAccountWithTenants(this.account, this.tenants);
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
          this.setSettingsNotice(NOTICE_KINDS.SUCCESS, COPY.tenantDeleted);
        }
      } catch (requestError) {
        if (!isAbortError(requestError) && this.settingsTenantID === deletedTenantID) {
          this.setSettingsNotice(
            NOTICE_KINDS.ERROR,
            requestError instanceof BackendClientError && requestError.status === 409 ? COPY.finalTenantDeletion : profileFailureMessage(requestError),
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
  });
}
