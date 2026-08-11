// @ts-check

import { RUNTIME_UI } from "./constants.js?v=20260811b130";
import { failApplicationStartup } from "./ui/applicationStartup.js?v=20260811b130";

const applicationModule = document.getElementById(RUNTIME_UI.APPLICATION_MODULE_ID);
if (!applicationModule) {
  throw new Error(RUNTIME_UI.APPLICATION_MODULE_MISSING);
}
applicationModule.addEventListener("error", () => {
  document.documentElement.setAttribute(RUNTIME_UI.STARTUP_ERROR_ATTRIBUTE, "application-module-load");
  void failApplicationStartup();
}, { once: true });
window.addEventListener("error", (event) => {
  if (
    document.documentElement.getAttribute(RUNTIME_UI.APPLICATION_READY_ATTRIBUTE) !== "ready" &&
    event instanceof ErrorEvent &&
    event.filename.startsWith(`${window.location.origin}/assets/llm-proxy/js/`)
  ) {
    document.documentElement.setAttribute(RUNTIME_UI.STARTUP_ERROR_ATTRIBUTE, "application-runtime");
    void failApplicationStartup();
  }
}, { capture: true });
document.documentElement.setAttribute(RUNTIME_UI.GUARD_READY_ATTRIBUTE, "ready");
