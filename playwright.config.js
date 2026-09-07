// @ts-check

import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests/e2e",
  globalSetup: "./tests/e2e/globalSetup.js",
  timeout: 30000,
  reporter: "list",
  use: {
    browserName: "chromium",
    trace: "retain-on-failure",
  },
});
