// @ts-check

import Alpine from "https://cdn.jsdelivr.net/npm/alpinejs@3.13.5/dist/module.esm.js";

/** @type {Window & typeof globalThis & { Alpine?: typeof Alpine }} */
const alpineWindow = window;
alpineWindow.Alpine = Alpine;
