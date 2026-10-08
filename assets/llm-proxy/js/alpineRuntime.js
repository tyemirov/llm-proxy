// @ts-check

import Alpine from "https://cdn.jsdelivr.net/npm/alpinejs@3.17.1/dist/module.esm.js";

/** @type {Window & typeof globalThis & { Alpine?: typeof Alpine }} */
const alpineWindow = window;
alpineWindow.Alpine = Alpine;
