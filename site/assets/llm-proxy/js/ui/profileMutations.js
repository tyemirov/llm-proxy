// @ts-check

import { AUTH_STATES, NOTICE_KINDS } from "../constants.js?v=20260902c240";
import {
  assertManagementTenantProfile,
  createAppRoutingDefaults,
  profileFailureMessage,
  profileProvider,
} from "../core/managementProfile.js?v=20260902c240";

const EMPTY_STRING = "";

/** @typedef {ReturnType<typeof import("./managementApplicationState.js").createManagementApplicationState>} ManagementApplicationState */
/** @typedef {ManagementApplicationState & import("../types.d.js").AlpineMagic & {
 *   selectedProviderID: string,
 *   applyProviderCardProfile: (profile: import("../types.d.js").ManagementTenantProfile, preserveProviderEditor?: boolean) => void,
 *   canApplyProviderAutosave: (providerID: string, revealVersion: number, appVersion: number) => boolean,
 *   dismissProviderKeyRemovalConfirmation: () => void,
 *   replaceProviderEditorSession: (providerID: string) => void,
 *   setPageNotice: (kind: string, message: string) => void
 * }} ProfileMutationHost */

/**
 * @template {object} Responsibility
 * @param {Responsibility & ThisType<ProfileMutationHost & Responsibility>} responsibility
 * @returns {Responsibility}
 */
function profileMutationResponsibility(responsibility) {
  return responsibility;
}

/** Create the single serialized whole-profile mutation boundary. */
export function createProfileMutationsResponsibility() {
  return profileMutationResponsibility({
    /**
     * @param {() => Promise<import("../types.d.js").ManagementTenantProfile>} mutation
     * @param {string} successMessage
     * @returns {Promise<boolean>}
     */
    async runProviderCardMutation(mutation, successMessage) {
      const appVersion = this.appVersion;
      const providerID = this.selectedProviderID;
      const revealVersion = this.providerEditorSession.revealVersion;
      this.providerAutosavePending = true;
      try {
        const profileApplied = await this.enqueueProfileMutation(appVersion, async () => {
          const updatedProfile = await mutation();
          if (!this.canApplyProviderAutosave(providerID, revealVersion, appVersion)) {
            return false;
          }
          this.applyProviderCardProfile(updatedProfile);
          this.setPageNotice(NOTICE_KINDS.SUCCESS, successMessage);
          return true;
        });
        return Boolean(profileApplied);
      } catch (requestError) {
        if (this.canApplyProviderAutosave(providerID, revealVersion, appVersion)) {
          this.setPageNotice(NOTICE_KINDS.ERROR, profileFailureMessage(requestError));
        }
        return false;
      } finally {
        this.providerAutosavePending = false;
      }
    },

    /**
     * @template MutationResult
     * @param {number} appVersion
     * @param {() => Promise<MutationResult>} mutation
     * @returns {Promise<MutationResult | null>}
     */
    async enqueueProfileMutation(appVersion, mutation) {
      const previousMutation = this.profileMutationTail;
      /** @type {() => void} */
      let releaseMutation = () => {};
      /** @type {Promise<void>} */
      const mutationCompleted = new Promise((resolve) => {
        releaseMutation = resolve;
      });
      this.profileMutationTail = previousMutation.then(() => mutationCompleted);
      await previousMutation;
      try {
        if (!this.canApplyProfileMutation(appVersion)) {
          return null;
        }
        return await mutation();
      } catch (requestError) {
        if (this.canApplyProfileMutation(appVersion)) {
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
     * @param {number} appVersion
     * @returns {boolean}
     */
    canApplyProfileMutation(appVersion) {
      return (
        (this.settingsOpen || this.providerCardTenantID !== EMPTY_STRING) &&
        this.authState === AUTH_STATES.AUTHENTICATED &&
        this.appVersion === appVersion
      );
    },

    /**
     * @param {import("../types.d.js").ManagementTenantProfile} nextProfile
     * @param {boolean} [preserveProviderEditor]
     * @param {boolean} [preserveRoutingDefaults]
     */
    applyProfile(nextProfile, preserveProviderEditor = false, preserveRoutingDefaults = false) {
      assertManagementTenantProfile(nextProfile, this.settingsTenantID);
      const defaults = createAppRoutingDefaults(nextProfile);
      const profileApplicationVersion = this.profileApplicationVersion + 1;
      this.profileApplicationVersion = profileApplicationVersion;
      const selectedProviderID = this.selectedProviderID;
      if (preserveProviderEditor) {
        profileProvider(nextProfile.providers, selectedProviderID);
      }
      this.dismissProviderKeyRemovalConfirmation();
      this.profile = nextProfile;
      this.providers = nextProfile.providers;
      if (this.selectedUsageTenantID === this.settingsTenantID) {
        this.usageProfile = nextProfile;
      }
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
        : defaults.provider || (this.providers[0] ? this.providers[0].id : EMPTY_STRING);
      if (!preserveProviderEditor) {
        this.replaceProviderEditorSession(nextProviderID);
      }
      if (!preserveRoutingDefaults) {
        const appVersion = this.appVersion;
        const routingDefaultsEditVersion = this.routingDefaultsEditVersion;
        this.$nextTick(() => {
          if (
            this.appVersion === appVersion &&
            this.profileApplicationVersion === profileApplicationVersion &&
            this.routingDefaultsEditVersion === routingDefaultsEditVersion
          ) {
            this.defaults = { ...defaults };
          }
        });
      }
    },
  });
}
