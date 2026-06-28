// @ts-check

import { MPR_UI } from "../constants.js";
import { loadFrontendRuntimeConfig } from "./backendClient.js";

/**
 * @returns {Promise<void>}
 */
export async function initializeMprShell() {
  const runtimeConfig = await loadFrontendRuntimeConfig();
  applyHeaderConfigURL(runtimeConfig.configUrl);
  await applyMprUIConfig(runtimeConfig.configUrl);
  await loadMprUIBundle();
}

/**
 * @param {string} configUrl
 */
function applyHeaderConfigURL(configUrl) {
  const header = document.getElementById(MPR_UI.HEADER_ID);
  if (header) {
    header.setAttribute("data-config-url", configUrl);
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
