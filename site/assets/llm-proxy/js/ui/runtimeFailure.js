// @ts-check

import { COPY, RUNTIME_UI } from "../constants.js?v=20260902c237";

/**
 * @returns {HTMLElement}
 */
export function renderRuntimeFailure() {
  const failureSurface = requiredElement(RUNTIME_UI.FAILURE_SURFACE_ID);
  requiredElement(RUNTIME_UI.FAILURE_EYEBROW_ID).textContent = COPY.runtimeFailureEyebrow;
  requiredElement(RUNTIME_UI.FAILURE_TITLE_ID).textContent = COPY.runtimeFailureTitle;
  requiredElement(RUNTIME_UI.FAILURE_DESCRIPTION_ID).textContent = COPY.runtimeFailureDescription;
  const reloadButton = requiredElement(RUNTIME_UI.FAILURE_RELOAD_ID);
  reloadButton.textContent = COPY.runtimeFailureReload;
  reloadButton.addEventListener("click", () => window.location.reload(), { once: true });
  failureSurface.hidden = false;
  return failureSurface;
}

/**
 * @param {string} elementID
 * @returns {HTMLElement}
 */
function requiredElement(elementID) {
  const element = document.getElementById(elementID);
  if (!element) {
    throw new Error(`${RUNTIME_UI.FAILURE_SURFACE_MISSING}:${elementID}`);
  }
  return element;
}
