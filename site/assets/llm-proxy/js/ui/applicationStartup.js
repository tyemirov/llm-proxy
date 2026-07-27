// @ts-check

import { RUNTIME_UI } from "../constants.js?v=20260727";
import { dispatchManagementReady } from "../core/runtimeTransition.js?v=20260727";
import { renderRuntimeFailure } from "./runtimeFailure.js?v=20260727";

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
