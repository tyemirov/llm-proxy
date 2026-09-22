// @ts-check

import { defineConfig } from "@playwright/test";

export default defineConfig({
  outputDir: "./test-results/frontend",
  testDir: "./tests/e2e",
  globalSetup: "./tests/e2e/globalSetup.js",
  timeout: 30000,
  reporter: "list",
  use: {
    browserName: "chromium",
    trace: "retain-on-failure",
  },
});
