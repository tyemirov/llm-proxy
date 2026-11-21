// @ts-check

import { RUNTIME_UI } from "../constants.js?v=20261121payment";
import { dispatchManagementReady } from "../core/runtimeTransition.js?v=20261121payment";
import { renderRuntimeFailure } from "./runtimeFailure.js?v=20261121payment";

/** @type {Promise<void> | null} */
let startupFailurePromise = null;

/**
 * @returns {Promise<void>}
 */
export function failApplicationStartup() {
  if (!startupFailurePromise) {
    startupFailurePromise = renderApplicationStartupFailure();
  }
  return startupFailurePromise;
}

/**
 * @returns {Promise<void>}
 */
async function renderApplicationStartupFailure() {
  const failureSurface = renderRuntimeFailure();
  failureSurface.focus();
  try {
    await dispatchManagementReady();
  } catch {
    failureSurface.setAttribute(RUNTIME_UI.TRANSITION_COMPLETION_FAILED_ATTRIBUTE, "");
  }
}
