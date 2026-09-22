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
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
  },
});
