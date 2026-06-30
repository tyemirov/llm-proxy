import { expect, test } from "@playwright/test";
import { createReadStream } from "node:fs";
import { readFile } from "node:fs/promises";
import http from "node:http";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const siteRoot = path.join(repoRoot, "site");
const configPath = "/config-ui.yaml";
const faviconPath = "/assets/llm-proxy/img/favicon.svg";
const appIconPath = "/assets/llm-proxy/img/llm-proxy-icon.svg";
const httpOK = 200;
const httpInternalServerError = 500;
const mimeTypes = Object.freeze({
  ".css": "text/css",
  ".html": "text/html",
  ".js": "application/javascript",
  ".svg": "image/svg+xml",
  ".yaml": "application/yaml",
});

let server;
let baseURL = "";

test.beforeAll(async () => {
  server = http.createServer(staticSiteHandler);
  await new Promise((resolve) => {
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  if (!address || typeof address === "string") {
    throw new Error("static_server_address_missing");
  }
  baseURL = `http://127.0.0.1:${address.port}`;
});

test.afterAll(async () => {
  await new Promise((resolve, reject) => {
    server.close((closeError) => {
      if (closeError) {
        reject(closeError);
        return;
      }
      resolve();
    });
  });
});

test("site exposes product icon and favicon assets", async ({ request }) => {
  const htmlResponse = await request.get(baseURL);
  expect(htmlResponse.status()).toBe(httpOK);
  const html = await htmlResponse.text();
  expect(html).toContain('<meta name="theme-color" content="#0076c3">');
  expect(html).toContain(`<link rel="icon" type="image/svg+xml" href="${faviconPath}">`);
  expect(html).toContain(`<link rel="apple-touch-icon" href="${appIconPath}">`);
  expect(html).not.toContain("data-config-url");
  expect(html).not.toContain(configPath);

  const faviconResponse = await request.get(`${baseURL}${faviconPath}`);
  expect(faviconResponse.status()).toBe(httpOK);
  expect(faviconResponse.headers()["content-type"]).toContain(mimeTypes[".svg"]);
  const faviconSVG = await faviconResponse.text();
  expect(faviconSVG).toContain("LLM Proxy favicon");
  expect(faviconSVG).toContain("#ffd369");
  expect(faviconSVG).toContain("#4ad3d9");

  const appIconResponse = await request.get(`${baseURL}${appIconPath}`);
  expect(appIconResponse.status()).toBe(httpOK);
  expect(appIconResponse.headers()["content-type"]).toContain(mimeTypes[".svg"]);
  const appIconSVG = await appIconResponse.text();
  expect(appIconSVG).toContain("LLM Proxy icon");
  expect(appIconSVG).toContain("#ffd369");
  expect(appIconSVG).toContain("#4ad3d9");
});

test("dashboard shows usage and settings opens from avatar menu before sign out", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page);

  await page.goto(baseURL);

  await expect(page.getByRole("heading", { name: "Usage overview" })).toBeVisible();
  await expect(page.locator("usage-card").filter({ hasText: "Requests" }).locator("strong")).toHaveText("37");
  await expect(page.locator("usage-card").filter({ hasText: "Tokens" }).locator("strong")).toHaveText("12,345");
  await expect(page.locator("usage-card").filter({ hasText: "Success rate" }).locator("strong")).toHaveText("95%");
  await expect(page.locator("usage-chart-panel").first().locator("polyline")).toHaveAttribute("points", /,/);
  await expect(page.locator("usage-breakdown").first()).toContainText("openai");
  await expect(page.locator("usage-breakdown").first()).toContainText("24");

  await page.getByTestId("avatar-menu").click();
  await expect(page.getByTestId("avatar-menu-item").nth(0)).toHaveText("Settings");
  await expect(page.getByTestId("sign-out")).toHaveText("Sign out");

  await page.getByTestId("avatar-menu-item").nth(0).click();
  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  await expect(settingsDialog).toBeVisible();
  await expect(settingsDialog.getByRole("heading", { name: "Client access" })).toBeVisible();
  await expect(settingsDialog.getByRole("heading", { name: "Routing defaults" })).toBeVisible();
  await expect(settingsDialog.getByRole("heading", { name: "Request examples" })).toBeVisible();
  await expect(settingsDialog.getByRole("heading", { name: "Provider keys" })).toBeVisible();
});

test("settings modal remains usable on narrow screens", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 780 });
  await installAssetRoutes(page);
  await installManagementRoutes(page);

  await page.goto(baseURL);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  const modalBox = await page.locator("settings-modal").boundingBox();
  expect(modalBox).not.toBeNull();
  expect(modalBox.width).toBeLessThanOrEqual(390);
  await expect(page.getByRole("dialog", { name: "Settings" }).getByRole("button", { name: "Close" })).toBeVisible();
});

test("settings stays reachable when usage summary fails", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page, { usageStatus: httpInternalServerError });

  await page.goto(baseURL);

  await expect(page.getByRole("heading", { name: "Usage overview" })).toBeVisible();
  await expect(page.locator("usage-card").filter({ hasText: "Requests" }).locator("strong")).toHaveText("0");
  await expect(page.getByText("Request failed")).toBeVisible();

  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();
  await expect(page.getByRole("dialog", { name: "Settings" })).toBeVisible();
});

test("usage refresh clears stale metrics when summary reload fails", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page);

  await page.goto(baseURL);

  await expect(page.locator("usage-card").filter({ hasText: "Requests" }).locator("strong")).toHaveText("37");
  await page.unroute(`${baseURL}/api/management/usage`);
  await page.route(`${baseURL}/api/management/usage`, async (route) => {
    await route.fulfill({ status: httpInternalServerError, json: { error: "usage_failed" } });
  });
  await page.getByRole("button", { name: "Refresh" }).click();

  await expect(page.getByText("Request failed")).toBeVisible();
  await expect(page.locator("usage-card").filter({ hasText: "Requests" }).locator("strong")).toHaveText("0");
  await expect(page.locator("usage-chart-panel").first()).toContainText("No usage recorded");
});

test("admin menu opens all users dashboard", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page, { admin: true });

  await page.goto(baseURL);

  await page.getByTestId("avatar-menu").click();
  await expect(page.getByTestId("avatar-menu-item").nth(0)).toHaveText("Admin");
  await expect(page.getByTestId("avatar-menu-item").nth(1)).toHaveText("Settings");

  await page.getByTestId("avatar-menu-item").nth(0).click();

  await expect(page.getByRole("heading", { name: "All users" })).toBeVisible();
  await expect(page.locator("admin-user-card").filter({ hasText: "owner@example.com" })).toContainText("37");
  await expect(page.locator("admin-user-card").filter({ hasText: "teammate@example.com" })).toContainText("0");
  await expect(page.locator("admin-dashboard")).not.toContainText("sk-");
  await expect(page.locator("admin-dashboard")).not.toContainText("masked_key");
});

/**
 * @param {import("@playwright/test").Page} page
 * @returns {Promise<void>}
 */
async function installAssetRoutes(page) {
  await page.route("https://accounts.google.com/**", async (route) => route.abort());
  await page.route("**/alpinejs@3.13.5/dist/module.esm.js", async (route) =>
    fulfillFile(route, "node_modules/alpinejs/dist/module.esm.js", "application/javascript"),
  );
  await page.route("**/js-yaml@4.3.0/dist/js-yaml.min.js", async (route) =>
    fulfillFile(route, "node_modules/js-yaml/dist/js-yaml.min.js", "application/javascript"),
  );
  await page.route("**/mpr-ui.css", async (route) => route.fulfill({ body: "", contentType: "text/css" }));
  await page.route("**/mpr-ui-config.js", async (route) =>
    route.fulfill({
      body: "globalThis.MPRUI = { applyYamlConfig: async () => undefined };",
      contentType: "application/javascript",
    }),
  );
  await page.route("**/mpr-ui.js", async (route) =>
    route.fulfill({ body: mprUIBundleMock(), contentType: "application/javascript" }),
  );
}

/**
 * @param {import("@playwright/test").Page} page
 * @param {{ usageStatus?: number, admin?: boolean }} options
 * @returns {Promise<void>}
 */
async function installManagementRoutes(page, options = {}) {
  await page.route(`${baseURL}/api/management/profile`, async (route) => {
    await route.fulfill({ json: managementProfile(options.admin || false) });
  });
  await page.route(`${baseURL}/api/management/usage`, async (route) => {
    await route.fulfill({ status: options.usageStatus || httpOK, json: managementUsage() });
  });
  await page.route(`${baseURL}/api/management/admin/users`, async (route) => {
    await route.fulfill({ json: managementAdminUsers() });
  });
}

/**
 * @param {import("@playwright/test").Route} route
 * @param {string} relativePath
 * @param {string} contentType
 * @returns {Promise<void>}
 */
async function fulfillFile(route, relativePath, contentType) {
  await route.fulfill({
    body: await readFile(path.join(repoRoot, relativePath), "utf8"),
    contentType,
  });
}

/**
 * @param {http.IncomingMessage} request
 * @param {http.ServerResponse} response
 * @returns {Promise<void>}
 */
async function staticSiteHandler(request, response) {
  const requestURL = new URL(request.url || "/", baseURL);
  if (requestURL.pathname === configPath) {
    response.writeHead(200, { "Content-Type": mimeTypes[".yaml"] });
    response.end(`llmProxy:\n  managementApiOrigin: ${baseURL}\n  proxyOrigin: ${baseURL}\n`);
    return;
  }

  const routePath = requestURL.pathname === "/" ? "/index.html" : requestURL.pathname;
  const filePath = path.normalize(path.join(siteRoot, routePath));
  if (!filePath.startsWith(siteRoot)) {
    response.writeHead(404);
    response.end();
    return;
  }

  if (path.basename(filePath) === "index.html") {
    const html = await readFile(filePath, "utf8");
    response.writeHead(200, { "Content-Type": mimeTypes[".html"] });
    response.end(html);
    return;
  }

  response.writeHead(200, { "Content-Type": mimeTypes[path.extname(filePath)] || "application/octet-stream" });
  createReadStream(filePath).pipe(response);
}

/**
 * @param {boolean} isAdmin
 * @returns {object}
 */
function managementProfile(isAdmin = false) {
  return {
    user: {
      id: "user_1",
      email: "owner@example.com",
      display_name: "Owner",
      is_admin: isAdmin,
    },
    tenant: {
      id: "tenant_1",
      has_secret: true,
      defaults: {
        provider: "openai",
        model: "gpt-4.1",
        dictation_provider: "openai",
        dictation_model: "gpt-4o-mini-transcribe",
        system_prompt: "",
      },
    },
    providers: [
      {
        id: "openai",
        label: "OpenAI",
        aliases: [],
        has_key: true,
        masked_key: "sk-...1234",
        text_default_model: "gpt-4.1",
        text_models: ["gpt-4.1", "gpt-4o-mini"],
        supports_dictation: true,
        dictation_default_model: "gpt-4o-mini-transcribe",
        dictation_models: ["gpt-4o-mini-transcribe"],
      },
      {
        id: "deepseek",
        label: "DeepSeek",
        aliases: [],
        has_key: true,
        masked_key: "sk-...5678",
        text_default_model: "deepseek-chat",
        text_models: ["deepseek-chat"],
        supports_dictation: false,
        dictation_models: [],
      },
    ],
    proxy: {
      text_path: "/",
      v2_path: "/v2",
      dictation_path: "/dictate",
    },
  };
}

/**
 * @returns {object}
 */
function managementAdminUsers() {
  return {
    period_days: 30,
    users: [
      {
        user: {
          id: "user_1",
          email: "owner@example.com",
          display_name: "Owner",
          is_admin: true,
        },
        tenant: {
          id: "tenant_1",
          has_secret: true,
          created_at: "2026-06-01T00:00:00Z",
          updated_at: "2026-06-29T00:00:00Z",
        },
        usage: managementUsage(),
      },
      {
        user: {
          id: "user_2",
          email: "teammate@example.com",
          display_name: "Teammate",
          is_admin: false,
        },
        tenant: {
          id: "tenant_2",
          has_secret: false,
          created_at: "2026-06-10T00:00:00Z",
          updated_at: "2026-06-10T00:00:00Z",
        },
        usage: {
          ...managementUsage(),
          totals: usageAggregate(),
          providers: [],
          models: [],
          status_codes: [],
        },
      },
    ],
  };
}

/**
 * @returns {object}
 */
function managementUsage() {
  const daily = Array.from({ length: 30 }, (_, index) => ({
    date: `2026-06-${String(index + 1).padStart(2, "0")}`,
    data: usageAggregate(),
  }));
  daily[28].data = usageAggregate({ requests: 17, successful_requests: 17, text_requests: 17, total_tokens: 6000 });
  daily[29].data = usageAggregate({
    requests: 20,
    successful_requests: 18,
    failed_requests: 2,
    text_requests: 18,
    dictation_requests: 2,
    total_tokens: 6345,
  });
  return {
    period_days: 30,
    totals: usageAggregate({
      requests: 37,
      successful_requests: 35,
      failed_requests: 2,
      text_requests: 35,
      dictation_requests: 2,
      request_tokens: 4567,
      response_tokens: 7778,
      total_tokens: 12345,
      average_latency_ms: 312,
    }),
    daily,
    providers: [
      { provider: "openai", data: usageAggregate({ requests: 24 }) },
      { provider: "deepseek", data: usageAggregate({ requests: 13 }) },
    ],
    models: [
      { provider: "openai", model: "gpt-4.1", data: usageAggregate({ requests: 21 }) },
      { provider: "deepseek", model: "deepseek-chat", data: usageAggregate({ requests: 13 }) },
      { provider: "openai", model: "gpt-4o-mini-transcribe", data: usageAggregate({ requests: 3 }) },
    ],
    status_codes: [
      { status_code: 200, requests: 35 },
      { status_code: 502, requests: 2 },
    ],
  };
}

/**
 * @param {Partial<Record<string, number>>} overrides
 * @returns {object}
 */
function usageAggregate(overrides = {}) {
  return {
    requests: 0,
    successful_requests: 0,
    failed_requests: 0,
    text_requests: 0,
    dictation_requests: 0,
    request_tokens: 0,
    response_tokens: 0,
    total_tokens: 0,
    average_latency_ms: 0,
    ...overrides,
  };
}

/**
 * @returns {string}
 */
function mprUIBundleMock() {
  return `
class MprHeader extends HTMLElement {}
class MprFooter extends HTMLElement {}
class MprUser extends HTMLElement {
  static get observedAttributes() {
    return ["menu-items"];
  }

  connectedCallback() {
    this.render();
  }

  attributeChangedCallback() {
    if (this.isConnected) {
      this.render();
    }
  }

  render() {
    const menuItems = JSON.parse(this.getAttribute("menu-items") || "[]");
    const logoutLabel = this.getAttribute("logout-label") || "Sign out";
    this.innerHTML = [
      '<button type="button" data-testid="avatar-menu">User</button>',
      '<div data-testid="avatar-dropdown" hidden>',
      ...menuItems.map((item, index) => '<button type="button" data-testid="avatar-menu-item" data-index="' + index + '">' + item.label + '</button>'),
      '<button type="button" data-testid="sign-out">' + logoutLabel + '</button>',
      '</div>'
    ].join("");
    const dropdown = this.querySelector('[data-testid="avatar-dropdown"]');
    this.querySelector('[data-testid="avatar-menu"]').addEventListener("click", () => {
      dropdown.hidden = false;
    });
    this.querySelectorAll('[data-testid="avatar-menu-item"]').forEach((button) => {
      button.addEventListener("click", () => {
        const item = menuItems[Number(button.dataset.index)];
        this.dispatchEvent(new CustomEvent("mpr-user:menu-item", { bubbles: true, detail: item }));
      });
    });
  }
}
customElements.define("mpr-header", MprHeader);
customElements.define("mpr-footer", MprFooter);
customElements.define("mpr-user", MprUser);
`;
}
