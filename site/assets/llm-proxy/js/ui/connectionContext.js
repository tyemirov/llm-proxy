// @ts-check
import { MENU_ACTIONS } from '../constants.js?v=20260903f037';

/** @typedef {ReturnType<typeof import('./managementApplicationState.js').createManagementApplicationState> & {
 * openAdminDashboard:()=>Promise<void>,
 * openUsageDashboard:()=>void,
 * loadUsageSummary:(showSuccessNotice:boolean)=>Promise<void>,
 * clearUsageDetails:(restoreFocus:boolean)=>void
 * }} ConnectionContextHost */
/** @template {object} T @param {T & ThisType<ConnectionContextHost & T>} value @returns {T} */
function responsibility(value) {return value;}

export function createConnectionContextResponsibility() {
 return responsibility({
  /** @param {import('../types.d.js').ManagementTenantProfile} profile */
  applyProfile(profile) {
   this.defaults={...profile.tenant.defaults};this.profile=profile;this.providers=profile.providers;
  },
  /** @param {Event} event */
  handleUserMenuItem(event) {
   const action=/** @type {CustomEvent<{action:string}>} */(event).detail?.action;
   if(action===MENU_ACTIONS.OPEN_ADMIN)void this.openAdminDashboard();
   if(action===MENU_ACTIONS.OPEN_SETTINGS) {
    this.openUsageDashboard();
    requestAnimationFrame(()=>{const dashboard=document.querySelector('connection-dashboard');dashboard?.scrollIntoView({block:'start'});dashboard?.querySelector('button')?.focus();});
   }
  },
  /** @param {CustomEvent<{tenantID:string,tenants:import('../types.d.js').ManagementTenantSummary[],profile:import('../types.d.js').ManagementTenantProfile|null}>} event */
  async applyConnectionContext(event) {
   const {tenantID,tenants,profile}=event.detail;
   this.clearUsageDetails(false);
   this.tenants=tenants;
   if(this.account)this.account={...this.account,tenants};
   this.settingsTenantID=tenantID;
   this.selectedUsageTenantID=tenantID;
   this.usageProfile=profile;
   if(profile)this.applyProfile(profile);
   await this.loadUsageSummary(false);
  }
 });
}
