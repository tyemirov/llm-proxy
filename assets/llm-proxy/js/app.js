// @ts-check

import Alpine from "https://cdn.jsdelivr.net/npm/alpinejs@3.13.5/dist/module.esm.js";
import { initializeMprShell } from "./core/mprShell.js";
import { createKeyManagement } from "./ui/keyManagement.js";

initializeMprShell();

window.Alpine = Alpine;
Alpine.data("llmProxyKeyManagement", createKeyManagement);
Alpine.start();
