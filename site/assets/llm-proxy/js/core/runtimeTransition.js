// @ts-check

import { EVENTS } from "../constants.js?v=20261121payment";
import { waitForMprUIAutoOrchestrationReady } from "./mprShell.js?v=20261121payment";

/**
 * @returns {Promise<void>}
 */
export async function dispatchManagementReady() {
  await waitForMprUIAutoOrchestrationReady();
  document.dispatchEvent(new CustomEvent(EVENTS.MANAGEMENT_READY));
}
