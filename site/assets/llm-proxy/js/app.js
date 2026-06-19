// @ts-check

import Alpine from "https://cdn.jsdelivr.net/npm/alpinejs@3.13.5/dist/module.esm.js";
import { createKeyManagement } from "./ui/keyManagement.js";

window.Alpine = Alpine;
Alpine.data("llmProxyKeyManagement", createKeyManagement);
Alpine.start();
