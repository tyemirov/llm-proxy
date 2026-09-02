// @ts-check

import { EVENTS } from "../constants.js?v=20260902c237";
import { waitForMprUIAutoOrchestrationReady } from "./mprShell.js?v=20260902c237";

/**
 * @returns {Promise<void>}
 */
export async function dispatchManagementReady() {
  await waitForMprUIAutoOrchestrationReady();
  document.dispatchEvent(new CustomEvent(EVENTS.MANAGEMENT_READY));
}
