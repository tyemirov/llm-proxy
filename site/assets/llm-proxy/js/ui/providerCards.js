// @ts-check

import { AUTH_STATES, DASHBOARD_VIEWS } from "../constants.js?v=20260902a237";
import { formatNumber } from "./usageFailurePresentation.js?v=20260902a237";

const EMPTY_STRING = "";

/** @typedef {ReturnType<typeof import("./managementApplicationState.js").createManagementApplicationState>} ManagementApplicationState */
/** @typedef {ManagementApplicationState & import("../types.d.js").AlpineMagic & {
 *   autosaveSelectedProvider: () => Promise<boolean>,
 *   abortProviderKeyVerification: () => void,
 *   dismissProviderKeyRemovalConfirmation: () => void,
 *   replaceProviderEditorSession: (providerID: string) => void,
 *   switchSettingsTenant: (tenantID: string) => Promise<void>
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
      const activity = this.usage.providers.find((candidate) => candidate.provider === provider.id);
      const requests = activity ? activity.data.requests : 0;
      const tokens = activity ? activity.data.total_tokens : 0;
      return { available: true, requests: formatNumber(requests), tokens: formatNumber(tokens), used: requests > 0 };
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
      if (capability === "video_generation") return "Video generation";
      if (capability === "dictation") return "Dictation";
      return "Text";
    },

    /** @param {import("../types.d.js").ProviderProfile} provider */
    async openProviderCard(provider) {
      if (this.dashboardView !== DASHBOARD_VIEWS.USAGE || this.authState !== AUTH_STATES.AUTHENTICATED) return;
      this.providerCardProviderID = provider.id;
      this.providerCardTenantID = EMPTY_STRING;
      this.providerCardVersion += 1;
      if (this.selectedUsageTenantID) {
        await this.selectProviderCardTenant(this.selectedUsageTenantID);
        return;
      }
      this.focusProviderCardControl(provider.id, "select");
    },

    /** @param {Event} event */
    async handleProviderCardTenantSelection(event) {
      const tenantID = /** @type {HTMLSelectElement} */ (event.target).value;
      if (tenantID) await this.selectProviderCardTenant(tenantID);
    },

    /** @param {string} tenantID */
    async selectProviderCardTenant(tenantID) {
      const providerID = this.providerCardProviderID;
      const version = this.providerCardVersion;
      if (!this.tenants.some((tenant) => tenant.id === tenantID)) return;
      if (this.settingsTenantID !== tenantID) await this.switchSettingsTenant(tenantID);
      if (this.providerCardProviderID !== providerID || this.providerCardVersion !== version) return;
      this.providerCardTenantID = tenantID;
      this.replaceProviderEditorSession(providerID);
      this.focusProviderCardControl(providerID, "a, input, select, textarea, button");
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
      this.dismissProviderKeyRemovalConfirmation();
      this.providerCardProviderID = EMPTY_STRING;
      this.providerCardTenantID = EMPTY_STRING;
      this.providerCardVersion += 1;
      this.replaceProviderEditorSession(EMPTY_STRING);
    },
  });
}
