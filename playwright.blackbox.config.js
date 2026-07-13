import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests/blackbox",
  timeout: 180000,
  workers: 1,
  fullyParallel: false,
  reporter: "list",
  use: {
    browserName: "chromium",
    trace: "retain-on-failure",
  },
});
