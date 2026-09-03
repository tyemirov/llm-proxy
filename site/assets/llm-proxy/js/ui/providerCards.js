// @ts-check

import { AUTH_STATES, COPY, DASHBOARD_VIEWS, NOTICE_KINDS, PROVIDER_CAPABILITY_LABELS } from "../constants.js?v=20260902c240";
import { fetchTenant } from "../core/backendClient.js?v=20260902c240";
import {
  assertManagementTenantProfile,
  isAbortError,
  profileFailureMessage,
  profileProvider,
} from "../core/managementProfile.js?v=20260902c240";
import { formatNumber } from "./usageFailurePresentation.js?v=20260902c240";

const EMPTY_STRING = "";

/** @typedef {ReturnType<typeof import("./managementApplicationState.js").createManagementApplicationState>} ManagementApplicationState */
/** @typedef {ManagementApplicationState & import("../types.d.js").AlpineMagic & {
 *   autosaveSelectedProvider: () => Promise<boolean>,
 *   abortProviderKeyVerification: () => void,
 *   applyProfile: (profile: import("../types.d.js").ManagementTenantProfile, preserveProviderEditor?: boolean, preserveRoutingDefaults?: boolean) => void,
 *   dismissProviderKeyRemovalConfirmation: () => void,
 *   replaceProviderEditorSession: (providerID: string) => void,
 *   setPageNotice: (kind: string, message: string) => void
 * }} ProviderCardsHost */

/** @template {object} Responsibility @param {Responsibility & ThisType<ProviderCardsHost & Responsibility>} responsibility */
function providerCardsResponsibility(responsibility) {
  return responsibility;
}

/** Create the tenant-scoped provider card presentation and editor lifecycle. */
export function createProviderCardsResponsibility() {
  return providerCardsResponsibility({
    get providerCards() {
      return this.providers;
    },

    get providerCardTenantSelectionID() {
      return this.providerCardPendingTenantID || this.providerCardTenantID;
    },

    get providerCardTenantLoadPending() {
      return this.providerCardPendingTenantID !== EMPTY_STRING;
    },

    /** @param {import("../types.d.js").ProviderProfile} provider */
    providerCardIsOpen(provider) {
      return this.providerCardProviderID === provider.id;
    },

    /** @param {import("../types.d.js").ProviderProfile} provider */
    providerCardActivity(provider) {
      if (this.usageLoadState === "loading") {
        return { available: false, requests: "Loading", tokens: "Loading", used: false };
      }
      if (this.usageLoadState !== "available") {
        return { available: false, requests: "Unavailable", tokens: "Unavailable", used: false };
      }
      const activity = providerUsageAggregate(this.usage, provider.id);
      const requests = activity ? activity.data.requests : 0;
      const tokens = activity ? activity.data.total_tokens : 0;
      return { available: true, requests: formatNumber(requests), tokens: formatNumber(tokens), used: requests > 0 };
    },

    /** @param {import("../types.d.js").ProviderProfile} provider */
    providerCardRequestGraph(provider) {
      const activity = providerUsageAggregate(this.usage, provider.id);
      const requests = activity ? activity.data.requests : 0;
      const maximumRequests = Math.max(0, ...this.usage.providers.map((candidate) => candidate.data.requests));
      const scalePercentage = maximumRequests === 0 || requests === 0
        ? 0
        : Math.max(1, Math.round((requests / maximumRequests) * 100));
      return {
        width: `${scalePercentage}%`,
        accessibleLabel: `${formatNumber(requests)} ${COPY.usageBreakdownRequests}. ${scalePercentage}% ${COPY.providerRequestGraphScale}.`,
      };
    },

    /** @param {import("../types.d.js").ProviderProfile} provider */
    providerCardIsActive(provider) {
      return Boolean(this.usageProfile && this.usageProfile.tenant.defaults.provider === provider.id);
    },

    /** @param {import("../types.d.js").ProviderProfile} provider */
    providerCardModel(provider) {
      if (!this.usageProfile) return EMPTY_STRING;
      return this.usageProfile.providers.find((candidate) => candidate.id === provider.id)?.text_model || EMPTY_STRING;
    },

    /** @param {import("../types.d.js").ProviderProfile} provider */
    providerCardConfigured(provider) {
      return Boolean(this.usageProfile?.providers.find((candidate) => candidate.id === provider.id)?.configured);
    },

    /** @param {string} capability */
    providerCapabilityLabel(capability) {
      return /** @type {Record<string, string>} */ (PROVIDER_CAPABILITY_LABELS)[capability];
    },

    /** @param {import("../types.d.js").ProviderProfile} provider */
    async openProviderCard(provider) {
      if (this.dashboardView !== DASHBOARD_VIEWS.USAGE || this.authState !== AUTH_STATES.AUTHENTICATED) return;
      this.providerCardProviderID = provider.id;
      this.providerCardTenantID = EMPTY_STRING;
      this.providerCardPendingTenantID = EMPTY_STRING;
      this.providerCardProfile = null;
      this.providerSystemPromptOpen = false;
      this.providerCardVersion += 1;
      await this.selectProviderCardTenant(this.tenants[0].id);
    },

    /** @param {Event} event */
    async handleProviderCardTenantSelection(event) {
      const tenantSelect = /** @type {HTMLSelectElement} */ (event.target);
      const tenantID = tenantSelect.value;
      if (!tenantID || !(await this.selectProviderCardTenant(tenantID))) {
        tenantSelect.value = this.providerCardTenantID;
      }
    },

    /** @param {string} tenantID @returns {Promise<boolean>} */
    async selectProviderCardTenant(tenantID) {
      const providerID = this.providerCardProviderID;
      const version = this.providerCardVersion;
      if (!this.tenants.some((tenant) => tenant.id === tenantID)) return false;
      if (tenantID === this.providerCardTenantID && this.providerCardProfile) return true;
      if (this.providerCardTenantID && this.providerCardTenantID !== tenantID && !(await this.autosaveSelectedProvider())) {
        return false;
      }
      this.providerSystemPromptOpen = false;
      if (this.providerCardRequestController) this.providerCardRequestController.abort();
      const requestController = new AbortController();
      this.providerCardRequestController = requestController;
      this.providerCardPendingTenantID = tenantID;
      try {
        const profile = tenantID === this.settingsTenantID && this.profile
          ? this.profile
          : await fetchTenant(tenantID, requestController.signal);
        if (!this.canApplyProviderCardTenantLoad(providerID, tenantID, version, requestController)) return false;
        assertManagementTenantProfile(profile, tenantID);
        profileProvider(profile.providers, providerID);
        this.providerCardProfile = profile;
        this.providerCardTenantID = tenantID;
        this.providerCardPendingTenantID = EMPTY_STRING;
        this.replaceProviderEditorSession(providerID);
        this.focusProviderCardControl(providerID, ".provider-card-tenant select");
        return true;
      } catch (requestError) {
        if (!isAbortError(requestError) && this.canApplyProviderCardTenantLoad(providerID, tenantID, version, requestController)) {
          this.setPageNotice(NOTICE_KINDS.ERROR, profileFailureMessage(requestError));
        }
        return false;
      } finally {
        if (this.providerCardRequestController === requestController) {
          this.providerCardRequestController = null;
          this.providerCardPendingTenantID = EMPTY_STRING;
        }
      }
    },

    /**
     * @param {string} providerID
     * @param {string} tenantID
     * @param {number} version
     * @param {AbortController} requestController
     */
    canApplyProviderCardTenantLoad(providerID, tenantID, version, requestController) {
      return (
        this.authState === AUTH_STATES.AUTHENTICATED &&
        this.providerCardProviderID === providerID &&
        this.providerCardPendingTenantID === tenantID &&
        this.providerCardVersion === version &&
        this.providerCardRequestController === requestController
      );
    },

    /**
     * @param {import("../types.d.js").ManagementTenantProfile} nextProfile
     * @param {boolean} [preserveProviderEditor]
     */
    applyProviderCardProfile(nextProfile, preserveProviderEditor = false) {
      const tenantID = this.providerCardTenantID;
      const providerID = this.providerCardProviderID;
      assertManagementTenantProfile(nextProfile, tenantID);
      profileProvider(nextProfile.providers, providerID);
      this.providerCardProfile = nextProfile;
      if (this.settingsTenantID === tenantID) {
        this.applyProfile(
          nextProfile,
          true,
          this.routingDefaultsDirty || this.routingDefaultsAutosavePending,
        );
      } else if (this.selectedUsageTenantID === tenantID) {
        this.usageProfile = nextProfile;
      }
      if (!preserveProviderEditor) this.replaceProviderEditorSession(providerID);
    },

    /** @param {string} providerID @param {string} selector */
    focusProviderCardControl(providerID, selector) {
      this.$nextTick(() => {
        requestAnimationFrame(() => {
          const cardBack = document.querySelector(`[data-provider-card="${CSS.escape(providerID)}"] .provider-card-back`);
          const control = cardBack?.querySelector(selector);
          if (control instanceof HTMLElement) control.focus();
        });
      });
    },

    async closeProviderCard() {
      if (!(await this.autosaveSelectedProvider())) return;
      const providerID = this.providerCardProviderID;
      this.resetProviderCard();
      this.$nextTick(() => {
        const action = document.querySelector(`[data-provider-card-action="${CSS.escape(providerID)}"]`);
        if (action instanceof HTMLElement) action.focus();
      });
    },

    resetProviderCard() {
      this.abortProviderKeyVerification();
      if (this.providerCardRequestController) {
        this.providerCardRequestController.abort();
        this.providerCardRequestController = null;
      }
      this.dismissProviderKeyRemovalConfirmation();
      this.providerCardProviderID = EMPTY_STRING;
      this.providerCardTenantID = EMPTY_STRING;
      this.providerCardPendingTenantID = EMPTY_STRING;
      this.providerCardProfile = null;
      this.providerCardVersion += 1;
      this.replaceProviderEditorSession(EMPTY_STRING);
    },
  });
}

/**
 * @param {import("../types.d.js").ManagementUsageSummary} usage
 * @param {string} providerID
 */
function providerUsageAggregate(usage, providerID) {
  return usage.providers.find((candidate) => candidate.provider === providerID);
}
