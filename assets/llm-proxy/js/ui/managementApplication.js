// @ts-check

import { createConnectionContextResponsibility } from './connectionContext.js?v=20260903f037';

import { APP_INTEGRITY_ERROR } from "../constants.js?v=20260903f037";
import { createAdminDashboardResponsibility } from "./adminDashboard.js?v=20260903f037";
import { createAuthenticationLifecycleResponsibility } from "./authenticationLifecycle.js?v=20260903f037";
import { createManagementApplicationState } from "./managementApplicationState.js?v=20260903f037";
import { createNotificationsResponsibility } from "./notifications.js?v=20260903f037";
import { createUsageDashboardResponsibility } from "./usageDashboard.js?v=20260903f037";

/**
 * Compose the authenticated management application from non-overlapping UI responsibilities.
 *
 * @returns {object}
 */
export function createManagementApplication() {
  return composeManagementApplication(
    createManagementApplicationState(),
    createConnectionContextResponsibility(),
    createAuthenticationLifecycleResponsibility(),
    createNotificationsResponsibility(),
    createUsageDashboardResponsibility(),
    createAdminDashboardResponsibility(),
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
