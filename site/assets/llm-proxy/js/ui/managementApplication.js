// @ts-check

import { APP_INTEGRITY_ERROR } from "../constants.js?v=20260902c240";
import { createAdminDashboardResponsibility } from "./adminDashboard.js?v=20260902c240";
import { createAuthenticationLifecycleResponsibility } from "./authenticationLifecycle.js?v=20260902c240";
import { createClientAccessResponsibility } from "./clientAccess.js?v=20260902c240";
import { createManagementApplicationPresentationResponsibility } from "./managementApplicationPresentation.js?v=20260902c240";
import { createManagementApplicationState } from "./managementApplicationState.js?v=20260902c240";
import { createNotificationsResponsibility } from "./notifications.js?v=20260902c240";
import { createProfileMutationsResponsibility } from "./profileMutations.js?v=20260902c240";
import { createProviderCredentialsResponsibility } from "./providerCredentials.js?v=20260902c240";
import { createProviderCardsResponsibility } from "./providerCards.js?v=20260902c240";
import { createProviderEditorResponsibility } from "./providerEditor.js?v=20260902c240";
import { createProviderSettingsResponsibility } from "./providerSettings.js?v=20260902c240";
import { createRequestExamplesResponsibility } from "./requestExamples.js?v=20260902c240";
import { createRoutingDefaultsResponsibility } from "./routingDefaults.js?v=20260902c240";
import { createSettingsDialogResponsibility } from "./settingsDialog.js?v=20260902c240";
import { createTenantSettingsResponsibility } from "./tenantSettings.js?v=20260902c240";
import { createUsageDashboardResponsibility } from "./usageDashboard.js?v=20260902c240";

/**
 * Compose the authenticated management application from non-overlapping UI responsibilities.
 *
 * @returns {object}
 */
export function createManagementApplication() {
  return composeManagementApplication(
    createManagementApplicationState(),
    createManagementApplicationPresentationResponsibility(),
    createAuthenticationLifecycleResponsibility(),
    createNotificationsResponsibility(),
    createProfileMutationsResponsibility(),
    createTenantSettingsResponsibility(),
    createUsageDashboardResponsibility(),
    createAdminDashboardResponsibility(),
    createSettingsDialogResponsibility(),
    createProviderCardsResponsibility(),
    createProviderEditorResponsibility(),
    createProviderCredentialsResponsibility(),
    createProviderSettingsResponsibility(),
    createRoutingDefaultsResponsibility(),
    createClientAccessResponsibility(),
    createRequestExamplesResponsibility(),
  );
}

/**
 * @param {...object} responsibilities
 * @returns {object}
 */
function composeManagementApplication(...responsibilities) {
  const application = {};
  for (const responsibility of responsibilities) {
    const descriptors = Object.getOwnPropertyDescriptors(responsibility);
    for (const propertyName of Object.keys(descriptors)) {
      if (Object.hasOwn(application, propertyName)) {
        throw new Error(`${APP_INTEGRITY_ERROR}:duplicate_management_application_property:${propertyName}`);
      }
    }
    Object.defineProperties(application, descriptors);
  }
  return application;
}
