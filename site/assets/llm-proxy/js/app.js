// @ts-check

import { RUNTIME_UI } from "./constants.js?v=20260811c131";
import { initializeMprShell } from "./core/mprShell.js?v=20260811c131";
import { failApplicationStartup } from "./ui/applicationStartup.js?v=20260811c131";
import { createManagementApplication } from "./ui/managementApplication.js?v=20260811c131";

initializeMprShell();

const alpineRuntimeScript = document.createElement("script");
alpineRuntimeScript.type = "module";
alpineRuntimeScript.src = RUNTIME_UI.ALPINE_RUNTIME_MODULE_URL;
alpineRuntimeScript.addEventListener("load", startApplication, { once: true });
alpineRuntimeScript.addEventListener("error", () => void failApplicationStartup(), { once: true });
document.head.appendChild(alpineRuntimeScript);

/**
 * @returns {void}
 */
function startApplication() {
  const alpineRuntime =
    /** @type {typeof globalThis & { Alpine?: { data: (name: string, factory: () => object) => void, start: () => void } }} */ (
      globalThis
    );
  if (!alpineRuntime.Alpine) {
    void failApplicationStartup();
    return;
  }

  alpineRuntime.Alpine.data("llmProxyManagementApplication", createManagementApplication);
  alpineRuntime.Alpine.start();
  document.documentElement.setAttribute(RUNTIME_UI.APPLICATION_READY_ATTRIBUTE, "ready");
}
