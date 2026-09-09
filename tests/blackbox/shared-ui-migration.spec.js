// @ts-check
import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { assets, directory } from "./sharedUIAssets.mjs";
import { localManagementProfile, startLocalManagementStack } from "./localManagementStack.mjs";

test.use({ actionTimeout: 5000, navigationTimeout: 10000 });

for (const authRouting of ["frontend", "direct"]) {
  test.describe(`shared UI ${authRouting} auth origin`, () => {
    let stack;
    test.beforeAll(async () => { stack = await startLocalManagementStack(authRouting); });
    test.afterAll(async () => { await stack.stop(); });
    for (const width of [390, 1280]) {
      test(`login, restoration, recovery, logout, and footer at ${width}px`, async ({ browser }) => {
        const context = await browser.newContext({ viewport: { width, height: 900 } });
        try {
          for (const name of Object.keys(assets)) {
            const body = await readFile(path.join(directory, name));
            await context.route(`https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/${name}*`, route =>
              route.fulfill({ body, contentType: name.endsWith(".css") ? "text/css" : "application/javascript" }));
          }
          for (const [pattern, relative] of [
            ["**/alpinejs@3.17.1/dist/module.esm.js", "node_modules/alpinejs/dist/module.esm.js"],
            ["**/js-yaml@5.4.1/dist/browser/js-yaml.umd.min.js", "node_modules/js-yaml/dist/browser/js-yaml.umd.min.js"],
          ]) {
            const body = await readFile(relative);
            await context.route(pattern, route => route.fulfill({ body, contentType: "application/javascript" }));
          }
          const googleScript = await readFile(new URL("./googleIdentityFixture.js", import.meta.url));
          await context.route("https://accounts.google.com/gsi/client", route => route.fulfill({ body: googleScript, contentType: "application/javascript" }));
          let exchanges = 0;
          await context.route(`${stack.tAuthOrigin}/auth/google`, async route => {
            if (route.request().method() === "OPTIONS") return route.continue();
            expect(route.request().postDataJSON()).toMatchObject({ google_id_token: "local-blackbox-google-credential", nonce_token: expect.any(String) });
            const response = await context.request.post(`${stack.tAuthOrigin}/auth/password/login`, {
              headers: { Origin: stack.frontendOrigin, "X-TAuth-Tenant": localManagementProfile.tenantID },
              data: { email: localManagementProfile.operatorEmail, password: localManagementProfile.operatorPassword },
            });
            expect(response.status()).toBe(200);
            exchanges += 1;
            await route.fulfill({ response });
          });
          const page = await context.newPage();
          let sessions = 0;
          page.on("request", request => {
            if (request.url() === `${stack.tAuthOrigin}/auth/session` && request.method() === "GET") sessions += 1;
          });
          await page.goto(stack.frontendOrigin);
          const footer = page.locator("mpr-footer");
          await footer.getByRole("button", { name: "Built by Marco Polo Research Lab", exact: true }).click();
          await expect(footer.getByRole("link", { name: "Gravity Notes", exact: true })).toHaveAttribute("href", "https://gravity.mprlab.com");
          await page.keyboard.press("Escape");
          await page.getByRole("button", { name: "Sign in with Google", exact: true }).click();
          await page.waitForURL(`${stack.frontendOrigin}/app/`);
          await expect(page.locator("mpr-header")).toHaveAttribute("data-mpr-auth-status", "authenticated");
          expect(exchanges).toBe(1);
          const beforeReload = sessions;
          await page.reload();
          await expect(page.locator("mpr-header")).toHaveAttribute("data-mpr-auth-status", "authenticated");
          expect(sessions).toBeGreaterThan(beforeReload);
          const attempts = [];
          const rejected = new Set();
          for (const endpoint of ["account", "tenants"]) {
            await context.route(`${stack.llmProxyOrigin}/api/management/${endpoint}`, async route => {
              const method = route.request().method();
              if (method === "OPTIONS") return route.continue();
              attempts.push(method);
              if (!rejected.has(method)) {
                rejected.add(method);
                return route.fulfill({ status: 401, headers: { "Access-Control-Allow-Origin": stack.frontendOrigin, "Access-Control-Allow-Credentials": "true" }, body: "Unauthorized" });
              }
              return route.continue();
            });
          }
          const result = await page.evaluate(async tenantName => {
            const client = await import("/assets/llm-proxy/js/core/backendClient.js?v=20260903f037");
            const before = await client.fetchAccount();
            const created = await client.createTenant(tenantName);
            const after = await client.fetchAccount();
            return { before: before.tenants.length, after: after.tenants.length, created: created.tenant.id, ids: after.tenants.map(tenant => tenant.id) };
          }, `Migration ${authRouting} ${width}`);
          expect(result.after).toBe(result.before + 1);
          expect(result.ids.filter(id => id === result.created)).toHaveLength(1);
          expect(attempts).toEqual(["GET", "GET", "POST", "POST", "GET"]);
          const dialog = page.getByRole("dialog", { name: "Settings" });
          if (await dialog.isVisible()) await dialog.getByRole("button", { name: "Close" }).click();
          await page.locator('mpr-user [data-mpr-user="trigger"]').click();
          await page.locator('mpr-user [data-mpr-user="logout"]').click();
          await expect(page).toHaveURL(`${stack.frontendOrigin}/`);
          await expect(page.getByRole("heading", { name: "Integrate once. Use the model that fits." })).toBeVisible();
          expect((await context.request.get(`${stack.llmProxyOrigin}/api/management/account`)).status()).toBe(401);
        } finally {
          await context.close();
        }
      });
    }
  });
}
