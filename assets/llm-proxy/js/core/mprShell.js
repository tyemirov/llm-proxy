// @ts-check

import { ADMIN_USER_MENU_ITEMS, MPR_UI, USER_MENU_ITEMS } from "../constants.js";
import { loadFrontendRuntimeConfig } from "./backendClient.js";

/**
 * @returns {Promise<void>}
 */
export async function initializeMprShell() {
  const runtimeConfig = await loadFrontendRuntimeConfig();
  applyHeaderConfigURL(runtimeConfig.configUrl);
  applyUserMenuItems(false);
  await applyMprUIConfig(runtimeConfig.configUrl);
  await loadMprUIBundle();
}

/**
 * @param {string} configUrl
 */
function applyHeaderConfigURL(configUrl) {
  const header = document.getElementById(MPR_UI.HEADER_ID);
  if (header) {
    header.setAttribute(MPR_UI.CONFIG_URL_ATTRIBUTE, configUrl);
  }
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
 * @param {string} configUrl
 * @returns {Promise<unknown>}
 */
function applyMprUIConfig(configUrl) {
  const runtimeGlobal =
    /** @type {typeof globalThis & { MPRUI?: { applyYamlConfig?: (options: { configUrl: string }) => Promise<unknown> } }} */ (globalThis);
  if (!runtimeGlobal.MPRUI || typeof runtimeGlobal.MPRUI.applyYamlConfig !== "function") {
    throw new Error(MPR_UI.CONFIG_LOADER_MISSING);
  }
  return runtimeGlobal.MPRUI.applyYamlConfig({ configUrl });
}

/**
 * @returns {Promise<void>}
 */
function loadMprUIBundle() {
  const existingScript = document.getElementById(MPR_UI.SCRIPT_ID);
  if (existingScript) {
    return Promise.resolve();
  }
  return new Promise((resolve, reject) => {
    const script = document.createElement("script");
    script.id = MPR_UI.SCRIPT_ID;
    script.src = MPR_UI.BUNDLE_URL;
    script.onload = () => resolve();
    script.onerror = () => reject(new Error(`script_load_failed: ${MPR_UI.BUNDLE_URL}`));
    document.head.appendChild(script);
  });
}
