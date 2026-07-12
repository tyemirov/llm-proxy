// @ts-check

import { ADMIN_USER_MENU_ITEMS, MPR_UI, USER_MENU_ITEMS } from "../constants.js";

/**
 * @returns {void}
 */
export function initializeMprShell() {
  applyUserMenuItems(false);
}

/**
 * @param {boolean} isAdmin
 */
export function applyUserMenuItems(isAdmin) {
  const userMenu = document.querySelector(MPR_UI.USER_SELECTOR);
  if (userMenu) {
    userMenu.setAttribute(MPR_UI.USER_MENU_ITEMS_ATTRIBUTE, JSON.stringify(isAdmin ? ADMIN_USER_MENU_ITEMS : USER_MENU_ITEMS));
  }
}

/**
 * @returns {Promise<void>}
 */
export async function waitForMprUIAutoOrchestrationReady() {
  await waitForDocumentReady();
  const runtimeGlobal =
    /** @type {typeof globalThis & { MPRUI?: { whenAutoOrchestrationReady?: () => Promise<unknown> } }} */ (globalThis);
  if (!runtimeGlobal.MPRUI || typeof runtimeGlobal.MPRUI.whenAutoOrchestrationReady !== "function") {
    throw new Error(MPR_UI.ORCHESTRATION_LOADER_MISSING);
  }
  await runtimeGlobal.MPRUI.whenAutoOrchestrationReady();
}

/**
 * @returns {Promise<void>}
 */
function waitForDocumentReady() {
  if (document.readyState !== "loading") {
    return Promise.resolve();
  }
  return new Promise((resolve) => {
    document.addEventListener("DOMContentLoaded", () => resolve(), { once: true });
  });
}
