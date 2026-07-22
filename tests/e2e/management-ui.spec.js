import { expect, test } from "@playwright/test";
import { createReadStream } from "node:fs";
import { mkdir, readFile, stat } from "node:fs/promises";
import http from "node:http";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const siteRoot = path.join(repoRoot, "site");
const configPath = "/config-ui.yaml";
const managementProviderKeysPath = "/api/management/provider-keys";
const faviconPath = "/assets/llm-proxy/img/favicon.svg";
const appIconPath = "/assets/llm-proxy/img/llm-proxy-icon.svg";
const resourcesPath = "/resources/";
const representativeResourcePath = "/resources/multi-provider-llm-proxy/";
const sitemapPath = "/sitemap.xml";
const robotsPath = "/robots.txt";
const b020ScreenshotDirectory = path.join(repoRoot, "output/playwright");
const httpOK = 200;
const httpInternalServerError = 500;
const mimeTypes = Object.freeze({
  ".css": "text/css",
  ".html": "text/html",
  ".js": "application/javascript",
  ".svg": "image/svg+xml",
  ".txt": "text/plain",
  ".xml": "application/xml",
  ".yaml": "application/yaml",
});
const generatedResourcePageCount = 45;
const seoContentModifiedDate = "2026-07-11";
const settingsLayerViewports = Object.freeze([
  { name: "desktop", width: 1280, height: 720 },
  { name: "compact", width: 480, height: 780 },
  { name: "mobile", width: 390, height: 780 },
]);

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
  expect(html).toContain('<link rel="canonical" href="https://llm-proxy.mprlab.com/">');
  expect(html).toContain(`<a href="${resourcesPath}">Browse resources</a>`);
  expect(html).toContain('<meta name="theme-color" content="#0076c3">');
  expect(html).toContain(`<link rel="icon" type="image/svg+xml" href="${faviconPath}">`);
  expect(html).toContain(`<link rel="apple-touch-icon" href="${appIconPath}">`);
  expect(html).toContain(`data-config-url="${configPath}"`);
  expect(html).toContain("data-mpr-ui-bundle-src=");
  expect(html).not.toContain("tauth.js");
  expect(html).toMatch(/<notification-region\s+slot="aux"[\s\S]*?<mpr-user\s+slot="aux"/);
  expect(html).toContain('<body x-data="llmProxyKeyManagement" x-init="init()">');
  expect(html).not.toContain('x-init="bindNotificationRegion($el)"');
  expect(html).toContain('<a slot="brand" class="llm-proxy-header-brand" href="/" aria-label="LLM Proxy home">');
  expect(html).toContain(`<img class="llm-proxy-header-brand__logo" src="${appIconPath}" alt="" aria-hidden="true">`);
  expect(html).toContain('<span class="llm-proxy-header-brand__title">LLM Proxy</span>');
  expect(html).not.toContain("brand-label=");
  expect(html).not.toContain("data:image");
  expect(html).toContain(
    '<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Material+Symbols+Outlined&amp;icon_names=delete,visibility,visibility_off&amp;display=block">',
  );
  expect(html).toContain(
    '<span class="material-symbols-outlined" x-show="!providerKeyVisible" aria-hidden="true">visibility</span>',
  );
  expect(html).toContain(
    '<span class="material-symbols-outlined" x-show="providerKeyVisible" aria-hidden="true">visibility_off</span>',
  );
  expect(html).toContain('<span class="material-symbols-outlined" aria-hidden="true">delete</span>');
  expect(html).toContain('class="icon-only danger provider-key-remove"');
  expect(html).not.toContain("provider-editor-actions");
  expect(html).not.toContain('<svg x-show="!providerKeyVisible"');
  expect(html).not.toContain('<svg x-show="providerKeyVisible"');
  const providerSelectorOffset = html.indexOf('<label class="provider-selector">');
  const providerKeyFieldOffset = html.indexOf("<provider-key-field>");
  const textModelOffset = html.indexOf('x-model="selectedProvider.text_model"');
  const providerRemovalOffset = html.indexOf('class="icon-only danger provider-key-remove"');
  expect(providerSelectorOffset).toBeGreaterThan(-1);
  expect(providerSelectorOffset).toBeLessThan(providerKeyFieldOffset);
  expect(providerKeyFieldOffset).toBeLessThan(textModelOffset);
  expect(textModelOffset).toBeLessThan(providerRemovalOffset);
  expect(html).toContain(
    '<client-access-row>',
  );
  expect(html).toContain('<strong x-text="tenantName"></strong>');
  expect(html).toContain(
    '<button type="button" class="icon-button client-key-create" x-on:click="generateSecret()" x-bind:disabled="busy" x-bind:title="clientKeyCreateCopy">',
  );
  expect(html).toContain(
    '<button type="button" class="icon-only client-key-copy" x-cloak x-show="hasGeneratedSecret" x-on:click="copyGeneratedSecret()" x-bind:disabled="busy" x-bind:title="copy.copyClientKey" x-bind:aria-label="copy.copyClientKey">',
  );
  expect(html).toContain(
    '<button type="button" class="icon-only danger client-key-revoke" x-cloak x-show="hasSecret" x-on:click="revokeSecret()" x-bind:disabled="busy" x-bind:title="copy.revokeKey" x-bind:aria-label="copy.revokeKey">',
  );
  expect(html).toContain('<span class="material-symbols-outlined" x-show="!generatedSecretVisible" aria-hidden="true">visibility</span>');
  expect(html).toContain('<span class="material-symbols-outlined" x-show="generatedSecretVisible" aria-hidden="true">visibility_off</span>');
  expect(html).toContain(
    '<svg class="copy-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true" focusable="false">',
  );
  expect(html).toContain('<rect x="6" y="5" width="10" height="12" rx="1.5"></rect>');
  expect(html).toContain('<rect x="8" y="7" width="10" height="12" rx="1.5"></rect>');
  expect(html).not.toContain("tenant-facts");
  expect(html).not.toContain("secret-output");
  expect(html).not.toContain("copy.tenantId");
  expect(html).not.toContain("copy.copySecret");
  expect(html).not.toContain("Generated secret");
  expect(html).toContain('x-model="defaults.reasoning_effort"');
  expect(html).toContain('class="text-routing-controls"');
  expect(html).toContain('x-on:change="normalizeReasoningEffortDefault()"');
  expect(html).toContain('copy.reasoningEffortUnsupported');
  expect(html).not.toContain("reasoning_effort_options");

  const mprShellResponse = await request.get(`${baseURL}/assets/llm-proxy/js/core/mprShell.js`);
  expect(mprShellResponse.status()).toBe(httpOK);
  const mprShellJavaScript = await mprShellResponse.text();
  expect(mprShellJavaScript).toContain("whenAutoOrchestrationReady");
  expect(mprShellJavaScript).not.toContain("data-mpr-user-status");
  expect(mprShellJavaScript).not.toContain("MutationObserver");
  expect(mprShellJavaScript).not.toContain("applyYamlConfig");

  const keyManagementResponse = await request.get(`${baseURL}/assets/llm-proxy/js/ui/keyManagement.js`);
  expect(keyManagementResponse.status()).toBe(httpOK);
  const keyManagementJavaScript = await keyManagementResponse.text();
  expect(keyManagementJavaScript).toContain("readMprUIAuthStatus");
  expect(keyManagementJavaScript).not.toContain("authenticatedShellProfileRequested");
  expect(keyManagementJavaScript).not.toContain("shellAuthenticationSettled");
  expect(keyManagementJavaScript).not.toContain("document.cookie");
  expect(keyManagementJavaScript).not.toContain("localStorage");
  expect(keyManagementJavaScript).not.toContain("/auth/session");
  expect(keyManagementJavaScript).not.toContain("ResizeObserver");
  expect(keyManagementJavaScript).not.toContain("NOTIFICATION_HEADER_BOTTOM_PROPERTY");
  expect(keyManagementJavaScript).not.toContain("bindNotificationRegion");
  expect(keyManagementJavaScript.match(/window\.setTimeout/g)).toHaveLength(1);
  expect(keyManagementJavaScript).toContain("NOTICE_AUTO_DISMISS_MILLISECONDS");

  const constantsResponse = await request.get(`${baseURL}/assets/llm-proxy/js/constants.js`);
  expect(constantsResponse.status()).toBe(httpOK);
  expect(await constantsResponse.text()).toContain("export const NOTICE_AUTO_DISMISS_MILLISECONDS = 10_000;");

  const stylesheetResponse = await request.get(`${baseURL}/assets/llm-proxy/styles.css`);
  expect(stylesheetResponse.status()).toBe(httpOK);
  const stylesheet = await stylesheetResponse.text();
  expect(stylesheet).toContain("#llm-proxy-header notification-region[slot=\"aux\"]");
  expect(stylesheet).toContain("order: -1;");
  expect(stylesheet).not.toContain("shadowRoot");

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

test("SEO resource pages are crawlable from the public site", async ({ request }) => {
  const hubResponse = await request.get(`${baseURL}${resourcesPath}`);
  expect(hubResponse.status()).toBe(httpOK);
  expect(hubResponse.headers()["content-type"]).toContain(mimeTypes[".html"]);
  const hubHTML = await hubResponse.text();
  expect(hubHTML).toContain("LLM Proxy resource hub");
  expect(hubHTML).toContain('<link rel="canonical" href="https://llm-proxy.mprlab.com/resources/">');
  expect(hubHTML).toContain('"@type":"CollectionPage"');
  expect(hubHTML).toContain(`href="${representativeResourcePath}"`);
  const resourceLinks = hubHTML.match(/href="\/resources\/[^"]+\/"/g) || [];
  expect(new Set(resourceLinks).size).toBe(generatedResourcePageCount);

  const pageResponse = await request.get(`${baseURL}${representativeResourcePath}`);
  expect(pageResponse.status()).toBe(httpOK);
  expect(pageResponse.headers()["content-type"]).toContain(mimeTypes[".html"]);
  const pageHTML = await pageResponse.text();
  expect(pageHTML).toContain("<h1>Multi-provider LLM proxy for internal tools</h1>");
  expect(pageHTML).toContain('<link rel="canonical" href="https://llm-proxy.mprlab.com/resources/multi-provider-llm-proxy/">');
  expect(pageHTML).toContain('"@type":"FAQPage"');
  expect(pageHTML).toContain('<a class="resource-button" href="/">Open LLM Proxy</a>');
  expect(pageHTML).toContain('href="/resources/openai-claude-gemini-one-endpoint/"');
  expect(pageHTML).toContain(`"dateModified":"${seoContentModifiedDate}"`);
});

test("SEO reliability pages describe configured upstream rate limits", async ({ request }) => {
  for (const slug of ["upstream-worker-queue-limits", "provider-overload-timeout-handling"]) {
    const response = await request.get(`${baseURL}/resources/${slug}/`);
    expect(response.status()).toBe(httpOK);
    const pageHTML = await response.text();
    expect(pageHTML).toContain("server.upstream_rate_limits");
    expect(pageHTML).not.toContain("I013 tracks future");
  }
});

test("SEO sitemap and robots expose canonical resource URLs", async ({ request }) => {
  const sitemapResponse = await request.get(`${baseURL}${sitemapPath}`);
  expect(sitemapResponse.status()).toBe(httpOK);
  expect(sitemapResponse.headers()["content-type"]).toContain(mimeTypes[".xml"]);
  const sitemapXML = await sitemapResponse.text();
  const sitemapLocations = sitemapXML.match(/<loc>/g) || [];
  expect(sitemapLocations).toHaveLength(generatedResourcePageCount + 2);
  expect(sitemapXML).toContain("<loc>https://llm-proxy.mprlab.com/</loc>");
  expect(sitemapXML).toContain("<loc>https://llm-proxy.mprlab.com/resources/</loc>");
  expect(sitemapXML).toContain(
    "<loc>https://llm-proxy.mprlab.com/resources/multi-provider-llm-proxy/</loc>",
  );
  const sitemapModificationDates = sitemapXML.match(/<lastmod>[^<]+<\/lastmod>/g) || [];
  expect(sitemapModificationDates).toHaveLength(generatedResourcePageCount + 2);
  expect(new Set(sitemapModificationDates)).toEqual(new Set([`<lastmod>${seoContentModifiedDate}</lastmod>`]));
  expect(sitemapXML).not.toContain("config-ui.yaml");
  expect(sitemapXML).not.toContain("llm-proxy-config.json");

  const robotsResponse = await request.get(`${baseURL}${robotsPath}`);
  expect(robotsResponse.status()).toBe(httpOK);
  expect(robotsResponse.headers()["content-type"]).toContain(mimeTypes[".txt"]);
  const robotsText = await robotsResponse.text();
  expect(robotsText).toContain("User-agent: *");
  expect(robotsText).toContain("Sitemap: https://llm-proxy.mprlab.com/sitemap.xml");
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
  const clientAccessRow = settingsDialog.locator("client-access-row");
  await expect(clientAccessRow).toContainText("Tenant");
  await expect(clientAccessRow).toContainText("Default");
  await expect(clientAccessRow).toContainText("Key retained");
  await expect(clientAccessRow).not.toContainText("Tenant ID");
  await expect(clientAccessRow.getByRole("button", { name: "Revoke key" })).toBeVisible();
  await expect(settingsDialog.getByRole("heading", { name: "Routing defaults" })).toBeVisible();
  await expect(settingsDialog.getByRole("heading", { name: "Request examples" })).toBeVisible();
  const requestExamplesSection = settingsDialog.locator(".usage-examples-section");
  await expect(requestExamplesSection).not.toHaveAttribute("open");
  await expect(settingsDialog.locator('request-example[data-example-id="default-text"]')).toBeHidden();
  await requestExamplesSection.locator("summary").click();
  await expect(requestExamplesSection).toHaveAttribute("open");
  await expect(settingsDialog.locator("request-example")).toHaveCount(6);
  await expect(settingsDialog.locator('request-example[data-example-id="default-text"]')).toBeVisible();
  await expect(settingsDialog.locator('request-example[data-example-id="default-text"]')).toContainText("Default text");
  await expect(settingsDialog.locator('request-example[data-example-id="default-v2"] .usage-snippet')).toContainText(
    "/v2?key=<generated-secret>",
  );
  await expect(settingsDialog.locator('request-example[data-example-id="default-dictation"] .usage-snippet')).toContainText(
    "/dictate?key=<generated-secret>",
  );
  await expect(settingsDialog.locator('request-example[data-example-id="provider-text"] .usage-snippet')).toContainText(
    "provider=openai",
  );
  await expect(settingsDialog.locator('request-example[data-example-id="provider-v2"] .usage-snippet')).toContainText(
    "provider=openai",
  );
  await expect(settingsDialog.locator('request-example[data-example-id="provider-dictation"] .usage-snippet')).toContainText(
    "provider=openai",
  );
  const providerEditor = settingsDialog.locator("provider-editor");
  await providerEditor.scrollIntoViewIfNeeded();
  await expect(settingsDialog.getByRole("heading", { name: "Provider settings" })).toBeVisible();
  await expect(providerEditor).toBeInViewport();
  const providerSelector = providerEditor.getByRole("combobox", { name: "Provider", exact: true });
  await expect(providerEditor.locator("provider-settings-fields")).toHaveCount(1);
  await expect(settingsDialog.locator("provider-key-card")).toHaveCount(0);
  await expect(providerEditor.locator("provider-status")).toHaveCount(0);
  await expect(providerEditor.locator(".provider-selector > .visually-hidden")).toHaveText("Provider");
  await expect(providerSelector).toHaveValue("openai");
  const providerKeyInput = providerEditor.getByRole("textbox", { name: "OpenAI API key" });
  const providerModelSelector = providerEditor.getByRole("combobox", { name: "Provider default model" });
  await expect(providerKeyInput).toHaveValue("****1234");
  await expect(providerKeyInput).toHaveAttribute("readonly", "readonly");
  await expect(providerEditor.getByRole("button", { name: "Show key" })).toHaveAttribute("aria-pressed", "false");
  const providerRemovalButton = providerEditor.getByRole("button", { name: "Remove provider key and settings" });
  await expect(providerRemovalButton).toBeVisible();
  await expect(providerRemovalButton.locator(".material-symbols-outlined")).toHaveText("delete");
  await expect(providerModelSelector).toHaveValue("gpt-4.1");
  await expect(providerEditor.getByRole("textbox", { name: "System prompt" })).toHaveValue("Use concise answers.");

  const providerControlBoxes = await Promise.all(
    [providerSelector, providerKeyInput, providerModelSelector].map((control) => control.boundingBox()),
  );
  const [providerSelectorBox, providerKeyBox, providerModelBox] = providerControlBoxes;
  if (!providerSelectorBox || !providerKeyBox || !providerModelBox) {
    throw new Error("desktop_provider_controls_missing");
  }
  expect(Math.abs(providerSelectorBox.y + providerSelectorBox.height - (providerKeyBox.y + providerKeyBox.height))).toBeLessThanOrEqual(1);
  expect(Math.abs(providerKeyBox.y + providerKeyBox.height - (providerModelBox.y + providerModelBox.height))).toBeLessThanOrEqual(1);
  expect(providerSelectorBox.x + providerSelectorBox.width).toBeLessThanOrEqual(providerKeyBox.x);
  expect(providerKeyBox.x + providerKeyBox.width).toBeLessThanOrEqual(providerModelBox.x);

  await providerSelector.selectOption("deepseek");
  await expect(providerEditor.getByRole("textbox", { name: "DeepSeek API key" })).toHaveValue("****5678");
  await expect(providerEditor.getByRole("combobox", { name: "Provider default model" })).toHaveValue("deepseek-chat");
  await expect(providerEditor.getByRole("textbox", { name: "System prompt" })).toHaveValue("");
  await expect(settingsDialog.locator("request-example")).toHaveCount(5);
  await expect(settingsDialog.locator('request-example[data-example-id="provider-text"] .usage-snippet')).toContainText(
    "provider=deepseek",
  );
  await expect(settingsDialog.locator('request-example[data-example-id="provider-v2"] .usage-snippet')).toContainText(
    "provider=deepseek",
  );
  await expect(settingsDialog.locator('request-example[data-example-id="provider-dictation"]')).toHaveCount(0);

  await providerSelector.selectOption("meta");
  await expect(providerEditor.getByRole("textbox", { name: "Meta API key" })).toHaveValue("****meta");
  await expect(providerEditor.getByRole("combobox", { name: "Provider default model" })).toHaveValue("muse-spark-1.1");
  await expect(settingsDialog.locator("request-example")).toHaveCount(5);
  await expect(settingsDialog.locator('request-example[data-example-id="provider-text"] .usage-snippet')).toContainText(
    "provider=meta",
  );
  await expect(settingsDialog.locator('request-example[data-example-id="provider-v2"] .usage-snippet')).toContainText(
    "provider=meta",
  );
  await expect(settingsDialog.locator('request-example[data-example-id="provider-dictation"]')).toHaveCount(0);
});

test("saved provider keys reveal, edit, and clear without browser persistence", async ({ page }) => {
  const revealedProviderKey = "sk-owner-openai-revealed";
  const editedProviderKey = "sk-owner-openai-edited";
  let revealRequestCount = 0;
  let savedProviderKeyPayload = null;
  page.on("request", (request) => {
    if (request.url() === providerKeyEndpointURL("openai", "reveal")) {
      revealRequestCount += 1;
    }
    if (request.url() === providerKeyEndpointURL("openai") && request.method() === "PUT") {
      savedProviderKeyPayload = JSON.parse(request.postData() || "{}");
    }
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page, { providerKeys: { openai: revealedProviderKey } });

  await page.goto(baseURL);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const providerEditor = settingsDialog.locator("provider-editor");
  const providerSelector = providerEditor.getByRole("combobox", { name: "Provider", exact: true });
  const providerKeyInput = providerEditor.getByRole("textbox", { name: "OpenAI API key" });
  await expect(providerKeyInput).toHaveValue("****1234");
  await expect(providerKeyInput).toHaveAttribute("readonly", "readonly");
  await expect(providerEditor.getByRole("button", { name: "Show key" })).toBeVisible();
  const visibilitySymbols = providerEditor
    .locator(".provider-key-visibility-toggle")
    .locator(".material-symbols-outlined");
  await expect(visibilitySymbols).toHaveCount(2);
  await expect(visibilitySymbols.nth(0)).toHaveText("visibility");
  await expect(visibilitySymbols.nth(0)).toBeVisible();
  await expect(visibilitySymbols.nth(1)).toHaveText("visibility_off");
  await expect(visibilitySymbols.nth(1)).toBeHidden();
  await expect(providerEditor.getByRole("button", { name: "Remove provider key and settings" }).locator(".material-symbols-outlined")).toHaveText("delete");
  await expect(settingsDialog.locator("example-list")).not.toContainText(revealedProviderKey);

  await providerEditor.getByRole("button", { name: "Show key" }).click();
  await expect(providerKeyInput).toHaveValue(revealedProviderKey);
  await expect(providerKeyInput).not.toHaveAttribute("readonly", "readonly");
  await expect(providerEditor.getByRole("button", { name: "Hide key" })).toBeVisible();
  await expect(visibilitySymbols.nth(0)).toBeHidden();
  await expect(visibilitySymbols.nth(1)).toBeVisible();
  await expect(providerEditor.getByText("Hide key", { exact: true })).toHaveCount(0);
  expect(revealRequestCount).toBe(1);
  expect(await providerKeyInput.evaluate((inputElement) => inputElement.outerHTML)).not.toContain(revealedProviderKey);
  await expect(settingsDialog.locator("example-list")).not.toContainText(revealedProviderKey);

  await providerEditor.getByRole("button", { name: "Hide key" }).click();
  await expect(providerKeyInput).toHaveValue("****aled");
  await expect(providerKeyInput).toHaveAttribute("readonly", "readonly");
  await providerEditor.getByRole("button", { name: "Show key" }).click();
  await expect(providerKeyInput).toHaveValue(revealedProviderKey);
  expect(revealRequestCount).toBe(1);

  await providerKeyInput.fill(editedProviderKey);
  await providerEditor.getByRole("button", { name: "Update key" }).click();
  await expect(providerKeyInput).not.toHaveValue(editedProviderKey);
  await expect(providerKeyInput).toHaveAttribute("readonly", "readonly");
  await expect(providerEditor.getByRole("button", { name: "Show key" })).toBeVisible();
  expect(savedProviderKeyPayload).toMatchObject({ api_key: editedProviderKey });
  await expect(settingsDialog.locator("example-list")).not.toContainText(editedProviderKey);
  expect(await browserStorageContains(page, revealedProviderKey)).toBe(false);
  expect(await browserStorageContains(page, editedProviderKey)).toBe(false);

  await providerEditor.getByRole("button", { name: "Show key" }).click();
  await expect(providerKeyInput).toHaveValue(editedProviderKey);
  await providerSelector.selectOption("deepseek");
  const deepSeekKeyInput = providerEditor.getByRole("textbox", { name: "DeepSeek API key" });
  await expect(deepSeekKeyInput).toHaveValue("****5678");
  await expect(deepSeekKeyInput).toHaveAttribute("readonly", "readonly");
  await providerSelector.selectOption("openai");
  await expect(providerKeyInput).not.toHaveValue(editedProviderKey);
  await expect(providerKeyInput).toHaveAttribute("readonly", "readonly");

  await providerEditor.getByRole("button", { name: "Show key" }).click();
  await expect(providerKeyInput).toHaveValue(editedProviderKey);
  await settingsDialog.getByRole("button", { name: "Close" }).click();
  await expect(settingsDialog).toBeHidden();
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();
  await expect(providerKeyInput).not.toHaveValue(editedProviderKey);
  await expect(providerKeyInput).toHaveAttribute("readonly", "readonly");

  await providerEditor.getByRole("button", { name: "Show key" }).click();
  await expect(providerKeyInput).toHaveValue(editedProviderKey);
  await page.reload();
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();
  await expect(providerKeyInput).not.toHaveValue(editedProviderKey);
  await expect(providerKeyInput).toHaveAttribute("readonly", "readonly");

  await providerEditor.getByRole("button", { name: "Show key" }).click();
  await expect(providerKeyInput).toHaveValue(editedProviderKey);
  await page.evaluate(() => {
    document.dispatchEvent(new CustomEvent("mpr-ui:auth:unauthenticated"));
  });
  await expect(page.getByRole("heading", { name: "Sign in to manage LLM Proxy keys" })).toBeVisible();
  expect(await browserStorageContains(page, editedProviderKey)).toBe(false);
});

test("removing a revealed provider key clears the selected editor", async ({ page }) => {
  const revealedProviderKey = "sk-owner-openai-remove";
  await installAssetRoutes(page);
  await installManagementRoutes(page, { providerKeys: { openai: revealedProviderKey } });

  await page.goto(baseURL);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const providerEditor = settingsDialog.locator("provider-editor");
  const providerKeyInput = providerEditor.getByRole("textbox", { name: "OpenAI API key" });
  await providerEditor.getByRole("button", { name: "Show key" }).click();
  await expect(providerKeyInput).toHaveValue(revealedProviderKey);
  await providerEditor.getByRole("button", { name: "Remove provider key and settings" }).click();

  await expect(providerKeyInput).toHaveValue("");
  await expect(providerKeyInput).not.toHaveAttribute("readonly", "readonly");
  await expect(providerEditor.getByRole("button", { name: "Show key" })).toBeHidden();
  expect(await browserStorageContains(page, revealedProviderKey)).toBe(false);
});

test("late provider-key reveals cannot populate a reopened editor", async ({ page }) => {
  const delayedProviderKey = "sk-owner-openai-delayed";
  let fulfillReveal;
  const revealFulfilled = new Promise((resolve) => {
    fulfillReveal = resolve;
  });
  let revealStarted;
  const revealRequested = new Promise((resolve) => {
    revealStarted = resolve;
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page, { providerKeys: { openai: delayedProviderKey } });
  await page.route(providerKeyEndpointURL("openai", "reveal"), async (route) => {
    revealStarted();
    await revealFulfilled;
    await route.fulfill({ headers: { "Cache-Control": "no-store" }, json: { api_key: delayedProviderKey } });
  });

  await page.goto(baseURL);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const providerEditor = settingsDialog.locator("provider-editor");
  const providerSelector = providerEditor.getByRole("combobox", { name: "Provider", exact: true });
  const providerKeyInput = providerEditor.getByRole("textbox", { name: "OpenAI API key" });
  await providerEditor.getByRole("button", { name: "Show key" }).click();
  await revealRequested;
  await expect(providerSelector).toBeDisabled();
  await expect(providerEditor.getByRole("button", { name: "Show key" })).toBeDisabled();
  await expect(providerEditor.getByRole("button", { name: "Update key" })).toBeDisabled();
  await expect(providerEditor.getByRole("button", { name: "Remove provider key and settings" })).toBeDisabled();

  await settingsDialog.getByRole("button", { name: "Close" }).click();
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();
  await providerSelector.selectOption("deepseek");
  fulfillReveal();
  const deepSeekKeyInput = providerEditor.getByRole("textbox", { name: "DeepSeek API key" });
  await expect(deepSeekKeyInput).toHaveValue("****5678");
  await expect(deepSeekKeyInput).toHaveAttribute("readonly", "readonly");
  expect(await browserStorageContains(page, delayedProviderKey)).toBe(false);

  await providerSelector.selectOption("openai");
  await expect(providerKeyInput).not.toHaveValue(delayedProviderKey);
  await expect(providerKeyInput).toHaveAttribute("readonly", "readonly");
});

test("short saved provider keys use a generic mask", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page, { maskedKeys: { meta: "saved" } });

  await page.goto(baseURL);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const providerEditor = settingsDialog.locator("provider-editor");
  await providerEditor.getByRole("combobox", { name: "Provider", exact: true }).selectOption("meta");
  const providerKeyInput = providerEditor.getByRole("textbox", { name: "Meta API key" });
  await expect(providerKeyInput).toHaveValue("****");
  await expect(providerKeyInput).not.toHaveValue("****saved");
});

test("routing defaults save only complete provider and model pairs", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page);

  await page.goto(baseURL);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const textProvider = settingsDialog.getByRole("combobox", { name: "Text provider" });
  const textModel = settingsDialog.getByRole("combobox", { name: "Text model" }).first();
  const dictationProvider = settingsDialog.getByRole("combobox", { name: "Dictation provider" });
  const dictationModel = settingsDialog.getByRole("combobox", { name: "Dictation model" });

  await expect(textProvider).toHaveValue("openai");
  await expect(textModel).toHaveValue("gpt-4.1");
  await expect(dictationProvider).toHaveValue("openai");
  await expect(dictationModel).toHaveValue("gpt-4o-mini-transcribe");
  await expect(dictationModel.locator('option[value=""]')).toHaveCount(0);

  await textProvider.selectOption("deepseek");
  await expect(textModel).toHaveValue("deepseek-chat");
  await dictationProvider.selectOption("grok");
  await expect(dictationModel).toHaveValue("xai-stt");

  const defaultsRequest = page.waitForRequest(`${baseURL}/api/management/defaults`);
  const defaultsResponse = page.waitForResponse(`${baseURL}/api/management/defaults`);
  await settingsDialog.getByRole("button", { name: "Save defaults" }).click();
  const submittedRequest = await defaultsRequest;
  const savedDefaultsResponse = await defaultsResponse;
  expect(submittedRequest.postDataJSON()).toEqual({
    provider: "deepseek",
    model: "deepseek-chat",
    dictation_provider: "grok",
    dictation_model: "xai-stt",
    system_prompt: "",
    reasoning_effort: "",
  });
  expect((await savedDefaultsResponse.json()).tenant.defaults).toEqual({
    provider: "deepseek",
    model: "deepseek-chat",
    dictation_provider: "grok",
    dictation_model: "xai-stt",
    system_prompt: "",
    reasoning_effort: "",
  });
  await expect(page.locator("notification-region")).toHaveText("Defaults saved");

  const reloadedProfileResponse = page.waitForResponse(`${baseURL}/api/management/profile`);
  await page.reload();
  expect((await reloadedProfileResponse).status()).toBe(httpOK);
  expect(await (await reloadedProfileResponse).json()).toMatchObject({
    tenant: { defaults: { provider: "deepseek", model: "deepseek-chat" } },
  });
  await expect(page.getByRole("heading", { name: "Usage overview" })).toBeVisible();
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();
  await expect(settingsDialog.getByRole("combobox", { name: "Text provider" })).toHaveValue("deepseek");
  await expect(settingsDialog.getByRole("combobox", { name: "Text model" }).first()).toHaveValue("deepseek-chat");
  await expect(settingsDialog.getByRole("combobox", { name: "Dictation provider" })).toHaveValue("grok");
  await expect(settingsDialog.getByRole("combobox", { name: "Dictation model" })).toHaveValue("xai-stt");
});

test("reasoning effort is exact to the selected text route and the controls remain responsive", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page);

  await page.goto(baseURL);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const textRoutingControls = settingsDialog.locator(".text-routing-controls");
  const textProvider = textRoutingControls.getByRole("combobox", { name: "Text provider" });
  const textModel = textRoutingControls.getByRole("combobox", { name: "Text model" });
  const reasoningEffort = textRoutingControls.getByRole("combobox", { name: "Reasoning effort" });
  const unsupportedEffort = textRoutingControls.locator(".reasoning-effort-unsupported");

  await expect(unsupportedEffort).toBeVisible();
  await expect(unsupportedEffort).toContainText("Not supported");
  await textModel.selectOption("gpt-5");
  await expect(reasoningEffort).toHaveValue("");
  await expect(reasoningEffort.locator("option")).toHaveText(["Not set", "minimal", "low", "medium", "high"]);
  await reasoningEffort.selectOption("high");

  const desktopBoxes = await Promise.all([textProvider, textModel, reasoningEffort].map((control) => control.boundingBox()));
  const [desktopProviderBox, desktopModelBox, desktopEffortBox] = desktopBoxes;
  if (!desktopProviderBox || !desktopModelBox || !desktopEffortBox) {
    throw new Error("desktop_text_routing_controls_missing");
  }
  expect(Math.abs(desktopProviderBox.y - desktopModelBox.y)).toBeLessThanOrEqual(1);
  expect(Math.abs(desktopModelBox.y - desktopEffortBox.y)).toBeLessThanOrEqual(1);
  expect(desktopProviderBox.x + desktopProviderBox.width).toBeLessThanOrEqual(desktopModelBox.x);
  expect(desktopModelBox.x + desktopModelBox.width).toBeLessThanOrEqual(desktopEffortBox.x);

  await textModel.selectOption("gpt-5.6");
  await expect(reasoningEffort).toHaveValue("high");
  await expect(reasoningEffort.locator("option")).toHaveText(["Not set", "none", "low", "medium", "high", "xhigh", "max"]);
  await reasoningEffort.selectOption("max");
  await textModel.selectOption("gpt-5");
  await expect(reasoningEffort).toHaveValue("");
  await expect(reasoningEffort.locator('option[value="max"]')).toHaveCount(0);

  await textProvider.selectOption("deepseek");
  await expect(textModel).toHaveValue("deepseek-chat");
  await expect(reasoningEffort).toBeHidden();
  await expect(unsupportedEffort).toBeVisible();

  await textProvider.selectOption("openai");
  await textModel.selectOption("gpt-5.6");
  await reasoningEffort.selectOption("max");

  const defaultsRequest = page.waitForRequest(`${baseURL}/api/management/defaults`);
  await settingsDialog.getByRole("button", { name: "Save defaults" }).click();
  expect((await defaultsRequest).postDataJSON()).toMatchObject({
    provider: "openai",
    model: "gpt-5.6",
    reasoning_effort: "max",
  });

  await page.setViewportSize({ width: 390, height: 780 });
  await expect(reasoningEffort).toBeVisible();
  const narrowBoxes = await Promise.all([textProvider, textModel, reasoningEffort].map((control) => control.boundingBox()));
  const [narrowProviderBox, narrowModelBox, narrowEffortBox] = narrowBoxes;
  if (!narrowProviderBox || !narrowModelBox || !narrowEffortBox) {
    throw new Error("narrow_text_routing_controls_missing");
  }
  expect(narrowModelBox.y).toBeGreaterThan(narrowProviderBox.y);
  expect(narrowEffortBox.y).toBeGreaterThan(narrowModelBox.y);
  for (const box of [narrowProviderBox, narrowModelBox, narrowEffortBox]) {
    expect(box.width).toBeGreaterThan(0);
    expect(box.x).toBeGreaterThanOrEqual(0);
    expect(box.x + box.width).toBeLessThanOrEqual(390);
  }

  const reloadedProfileResponse = page.waitForResponse(`${baseURL}/api/management/profile`);
  await page.reload();
  expect((await reloadedProfileResponse).status()).toBe(httpOK);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();
  await expect(settingsDialog.locator(".text-routing-controls").getByRole("combobox", { name: "Reasoning effort" })).toHaveValue("max");
});

test("malformed routing-default profiles become workspace integrity errors", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page, { malformedRoutingDefaults: true });

  await page.goto(baseURL);

  await expect(page.getByRole("heading", { name: "Unable to load key workspace" })).toBeVisible();
  await expect(page.getByText("Workspace integrity error")).toBeVisible();
  await expect(page.getByRole("dialog", { name: "Settings" })).toBeHidden();
});

test("invalid persisted routing-default profiles become workspace integrity errors", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page, { profileStatus: 500, profileError: "managed_routing_defaults_invalid" });

  await page.goto(baseURL);

  await expect(page.getByRole("heading", { name: "Unable to load key workspace" })).toBeVisible();
  await expect(page.getByText("Workspace integrity error")).toBeVisible();
});

test("dashboard loads only after MPR UI authenticates the user", async ({ page }) => {
  const profileRequests = [];
  page.on("request", (request) => {
    if (request.url() === `${baseURL}/api/management/profile`) {
      profileRequests.push(request);
    }
  });
  await installAssetRoutes(page, { initialAuthStatus: "unauthenticated" });
  await installManagementRoutes(page);

  await page.goto(baseURL);

  await expect(page.getByRole("heading", { name: "Sign in to manage LLM Proxy keys" })).toBeVisible();
  expect(profileRequests).toHaveLength(0);
  await page.evaluate(() => window.__llmProxyMprAuthenticate());

  await expect(page.getByRole("heading", { name: "Usage overview" })).toBeVisible();
  await expect(page.locator("usage-card").filter({ hasText: "Requests" }).locator("strong")).toHaveText("37");
  expect(profileRequests).toHaveLength(1);
});

test("startup reconciles MPR UI authentication after the lifecycle event has passed", async ({ page }) => {
  const profileRequests = [];
  page.on("request", (request) => {
    if (request.url() === `${baseURL}/api/management/profile`) {
      profileRequests.push(request);
    }
  });
  await installAssetRoutes(page, { emitInitialAuthEvent: false });
  await installManagementRoutes(page);

  await page.goto(baseURL);

  await expect(page.locator("mpr-header")).toHaveAttribute("data-mpr-auth-status", "authenticated");
  await expect(page.getByRole("heading", { name: "Usage overview" })).toBeVisible();
  expect(profileRequests).toHaveLength(1);
});

test("authenticated profile failures replace loading and signed-out states", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page, { profileStatus: 409 });

  await page.goto(baseURL);

  await expect(page.getByRole("heading", { name: "Unable to load key workspace" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Loading key workspace" })).toBeHidden();
  await expect(page.getByRole("heading", { name: "Sign in to manage LLM Proxy keys" })).toBeHidden();

  await page.reload();
  await expect(page.getByRole("heading", { name: "Unable to load key workspace" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Loading key workspace" })).toBeHidden();
});

test("signed-out panel presents a direct sign-in prompt without auth instructions", async ({ page }) => {
  await page.setViewportSize({ width: 1121, height: 253 });
  await installAssetRoutes(page, { initialAuthStatus: "unauthenticated" });
  await installManagementRoutes(page);

  await page.goto(baseURL);

  const signedOutPanel = page.locator("section.llm-panel").filter({
    has: page.getByRole("heading", { name: "Sign in to manage LLM Proxy keys" }),
  });
  await expect(signedOutPanel).toBeVisible();
  await expect(signedOutPanel.locator("p:not(.eyebrow)")).toHaveCount(0);
});

test("settings shows placeholder request examples before generated secret exists", async ({ page }) => {
  await installClipboardMock(page);
  await installAssetRoutes(page);
  await installManagementRoutes(page, { hasSecret: false });

  await page.goto(baseURL);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  await expect(settingsDialog).toBeVisible();
  await expect(settingsDialog.getByRole("heading", { name: "Request examples" })).toBeVisible();
  await settingsDialog.locator(".usage-examples-section summary").click();
  await expect(settingsDialog.locator('request-example[data-example-id="default-text"] .usage-snippet')).toContainText(
    "key=<generated-secret>",
  );
  await expect(settingsDialog.locator('request-example[data-example-id="default-v2"] .usage-snippet')).toContainText(
    "/v2?key=<generated-secret>",
  );
  await expect(settingsDialog.locator('request-example[data-example-id="default-dictation"] .usage-snippet')).toContainText(
    "/dictate?key=<generated-secret>",
  );
  await settingsDialog.locator('request-example[data-example-id="default-v2"]').getByRole("button", { name: "Copy" }).click();
  expect(await copiedText(page)).toContain("/v2?key=<generated-secret>");
  await settingsDialog.locator('request-example[data-example-id="provider-v2"]').getByRole("button", { name: "Copy" }).click();
  expect(await copiedText(page)).toContain("provider=openai");
  expect(await copiedText(page)).toContain("key=<generated-secret>");
  await expect(page.getByText("Example copied")).toBeVisible();
  await expect(settingsDialog.getByRole("heading", { name: "Provider settings" })).toBeVisible();
});

test("settings request examples retain the generated-key placeholder", async ({ page }) => {
  const generatedSecret = "llmp_test_generated_secret";
  await installAssetRoutes(page);
  await installManagementRoutes(page, { hasSecret: false, generatedSecret });

  await page.goto(baseURL);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  await settingsDialog.locator(".usage-examples-section summary").click();
  const defaultTextExample = settingsDialog.locator('request-example[data-example-id="default-text"] .usage-snippet');
  const providerV2Example = settingsDialog.locator('request-example[data-example-id="provider-v2"] .usage-snippet');
  await expect(defaultTextExample).toContainText("key=<generated-secret>");
  await expect(providerV2Example).toContainText("key=<generated-secret>");

  await settingsDialog.getByRole("button", { name: "Create key" }).click();

  await expect(settingsDialog.locator("client-access-row").getByRole("textbox", { name: "Key", exact: true })).not.toHaveValue(
    generatedSecret,
  );
  await expect(defaultTextExample).toContainText("key=<generated-secret>");
  await expect(settingsDialog.locator('request-example[data-example-id="default-v2"] .usage-snippet')).toContainText(
    "/v2?key=<generated-secret>",
  );
  await expect(settingsDialog.locator('request-example[data-example-id="default-dictation"] .usage-snippet')).toContainText(
    "/dictate?key=<generated-secret>",
  );
  await expect(providerV2Example).toContainText("key=<generated-secret>");
  await expect(settingsDialog.locator("example-list")).not.toContainText(generatedSecret);
});

test("late generated client keys cannot populate a reopened Settings session", async ({ page }) => {
  const lateGeneratedSecret = "llmp_test_late_settings_secret";
  let fulfillSecretResponse;
  const secretResponseReady = new Promise((resolve) => {
    fulfillSecretResponse = resolve;
  });
  let generatedSecretRequested;
  const secretRequestStarted = new Promise((resolve) => {
    generatedSecretRequested = resolve;
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page, { hasSecret: false });
  await page.route(`${baseURL}/api/management/secrets`, async (route) => {
    generatedSecretRequested();
    await secretResponseReady;
    await route.fulfill({
      headers: { "Cache-Control": "no-store" },
      json: {
        secret: lateGeneratedSecret,
        profile: managementProfile(false, true),
      },
    });
  });

  await page.goto(baseURL);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  await settingsDialog.getByRole("button", { name: "Create key" }).click();
  await secretRequestStarted;
  await settingsDialog.getByRole("button", { name: "Close" }).click();
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();
  fulfillSecretResponse();

  const clientAccessRow = settingsDialog.locator("client-access-row");
  await expect(clientAccessRow.getByText("No key created")).toBeVisible();
  await expect(clientAccessRow.getByRole("textbox", { name: "Key", exact: true })).toBeHidden();
  await expect(settingsDialog.getByRole("button", { name: "Create key" })).toBeEnabled();
  await expect(settingsDialog).not.toContainText(lateGeneratedSecret);
  expect(await browserStorageContains(page, lateGeneratedSecret)).toBe(false);
});

test("late generated client keys cannot restore after session cleanup", async ({ page }) => {
  const lateGeneratedSecret = "llmp_test_late_session_secret";
  let fulfillSecretResponse;
  const secretResponseReady = new Promise((resolve) => {
    fulfillSecretResponse = resolve;
  });
  let generatedSecretRequested;
  const secretRequestStarted = new Promise((resolve) => {
    generatedSecretRequested = resolve;
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page, { hasSecret: false });
  await page.route(`${baseURL}/api/management/secrets`, async (route) => {
    generatedSecretRequested();
    await secretResponseReady;
    await route.fulfill({
      headers: { "Cache-Control": "no-store" },
      json: {
        secret: lateGeneratedSecret,
        profile: managementProfile(false, true),
      },
    });
  });

  await page.goto(baseURL);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  await settingsDialog.getByRole("button", { name: "Create key" }).click();
  await secretRequestStarted;
  await page.evaluate(() => {
    document.dispatchEvent(new CustomEvent("mpr-ui:auth:unauthenticated"));
  });
  await expect(page.getByRole("heading", { name: "Sign in to manage LLM Proxy keys" })).toBeVisible();
  const generatedSecretResponse = page.waitForResponse(
    (response) => response.url() === `${baseURL}/api/management/secrets` && response.request().method() === "POST",
  );
  fulfillSecretResponse();
  await generatedSecretResponse;
  await expect(page.locator("body")).not.toContainText(lateGeneratedSecret);
  expect(await browserStorageContains(page, lateGeneratedSecret)).toBe(false);

  await page.evaluate(() => window.__llmProxyMprAuthenticate());
  await expect(page.getByRole("heading", { name: "Usage overview" })).toBeVisible();
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();
  const clientAccessRow = settingsDialog.locator("client-access-row");
  await expect(settingsDialog.getByRole("button", { name: "Create key" })).toBeEnabled();
  await expect(clientAccessRow.getByText("No key created")).toBeVisible();
  await expect(clientAccessRow.getByRole("textbox", { name: "Key", exact: true })).toBeHidden();
  await expect(settingsDialog).not.toContainText(lateGeneratedSecret);
});

test("new client keys are masked, revealable, copyable, and revocable per row", async ({ page }) => {
  const generatedSecret = "llmp_test_generated_secret";
  await installClipboardMock(page);
  await installAssetRoutes(page);
  await installManagementRoutes(page, { hasSecret: false, generatedSecret });

  for (const viewport of settingsLayerViewports) {
    await page.setViewportSize({ width: viewport.width, height: viewport.height });
    await page.goto(baseURL);
    await page.getByTestId("avatar-menu").click();
    await page.getByTestId("avatar-menu-item").nth(0).click();

    const settingsDialog = page.getByRole("dialog", { name: "Settings" });
    await settingsDialog.getByRole("button", { name: "Create key" }).click();

    const clientAccessRow = settingsDialog.locator("client-access-row");
    const clientTenant = clientAccessRow.locator("client-access-tenant");
    const clientKeyInput = clientAccessRow.getByRole("textbox", { name: "Key", exact: true });
    const visibilityButton = clientAccessRow.getByRole("button", { name: "Show key", exact: true });
    const copyButton = clientAccessRow.getByRole("button", { name: "Copy key", exact: true });
    const revokeButton = clientAccessRow.getByRole("button", { name: "Revoke key", exact: true });
    const copyIcon = copyButton.locator("svg.copy-icon");
    const visibilitySymbols = visibilityButton.locator(".material-symbols-outlined");
    await expect(clientKeyInput).toHaveValue("••••••••••••");
    await expect(clientKeyInput).toHaveAttribute("readonly", "");
    const replaceKeyButton = clientAccessRow.getByRole("button", { name: "Replace key", exact: true });
    await expect(replaceKeyButton).toBeVisible();
    await expect(visibilityButton).toHaveAttribute("aria-pressed", "false");
    await expect(visibilitySymbols).toHaveCount(2);
    await expect(visibilitySymbols.nth(0)).toHaveText("visibility");
    await expect(visibilitySymbols.nth(0)).toBeVisible();
    await expect(visibilitySymbols.nth(1)).toHaveText("visibility_off");
    await expect(visibilitySymbols.nth(1)).toBeHidden();
    await expect(revokeButton.locator(".material-symbols-outlined")).toHaveText("delete");
    await expect(copyButton).toHaveAttribute("title", "Copy key");
    await expect(copyIcon).toHaveCount(1);
    await expect(copyIcon).toHaveAttribute("aria-hidden", "true");
    await expect(copyIcon).toHaveAttribute("focusable", "false");
    await expect(copyIcon).toHaveAttribute("viewBox", "0 0 24 24");
    await expect(copyIcon.locator("rect")).toHaveCount(2);
    await expect(copyButton).not.toContainText("[]");

    await copyButton.focus();
    await expect(copyButton).toBeFocused();
    const copyButtonBox = await copyButton.boundingBox();
    const copyIconBox = await copyIcon.boundingBox();
    const settingsDialogBox = await settingsDialog.boundingBox();
    expect(copyButtonBox).not.toBeNull();
    expect(copyIconBox).not.toBeNull();
    expect(settingsDialogBox).not.toBeNull();
    if (!copyButtonBox || !copyIconBox || !settingsDialogBox) {
      throw new Error(`generated_secret_copy_geometry_missing:${viewport.name}`);
    }
    expect(copyButtonBox.width).toBeGreaterThanOrEqual(30);
    expect(copyIconBox.width).toBeGreaterThan(0);
    expect(copyIconBox.x).toBeGreaterThanOrEqual(copyButtonBox.x);
    expect(copyIconBox.x + copyIconBox.width).toBeLessThanOrEqual(copyButtonBox.x + copyButtonBox.width);
    expect(copyButtonBox.x).toBeGreaterThanOrEqual(settingsDialogBox.x);
    expect(copyButtonBox.x + copyButtonBox.width).toBeLessThanOrEqual(settingsDialogBox.x + settingsDialogBox.width);

    const clientTenantBox = await clientTenant.boundingBox();
    const clientKeyInputBox = await clientKeyInput.boundingBox();
    const replaceKeyButtonBox = await replaceKeyButton.boundingBox();
    if (!clientTenantBox || !clientKeyInputBox || !replaceKeyButtonBox) {
      throw new Error(`client_access_geometry_missing:${viewport.name}`);
    }
    if (viewport.name === "mobile") {
      expect(clientKeyInputBox.y).toBeGreaterThan(clientTenantBox.y);
      expect(replaceKeyButtonBox.y).toBeGreaterThan(clientKeyInputBox.y);
    } else {
      expect(clientTenantBox.x + clientTenantBox.width).toBeLessThanOrEqual(clientKeyInputBox.x);
      expect(clientKeyInputBox.x + clientKeyInputBox.width).toBeLessThanOrEqual(replaceKeyButtonBox.x);
      expect(Math.abs(clientTenantBox.y + clientTenantBox.height - (clientKeyInputBox.y + clientKeyInputBox.height))).toBeLessThanOrEqual(1);
      expect(Math.abs(clientKeyInputBox.y + clientKeyInputBox.height - (replaceKeyButtonBox.y + replaceKeyButtonBox.height))).toBeLessThanOrEqual(1);
    }

    await visibilityButton.click();
    await expect(clientKeyInput).toHaveValue(generatedSecret);
    const hideKeyButton = clientAccessRow.getByRole("button", { name: "Hide key", exact: true });
    await expect(hideKeyButton).toHaveAttribute("aria-pressed", "true");
    expect(await clientKeyInput.evaluate((inputElement) => inputElement.outerHTML)).not.toContain(generatedSecret);
    await expect(settingsDialog.locator("example-list")).not.toContainText(generatedSecret);
    expect(await browserStorageContains(page, generatedSecret)).toBe(false);

    await copyButton.click();
    expect(await copiedText(page)).toBe(generatedSecret);
    await expect(page.locator("#llm-proxy-header .notice")).toHaveText("Key copied");

    await hideKeyButton.click();
    await expect(clientKeyInput).toHaveValue("••••••••••••");
    await settingsDialog.getByRole("button", { name: "Close" }).click();
    await expect(settingsDialog).toBeHidden();

    await page.getByTestId("avatar-menu").click();
    await page.getByTestId("avatar-menu-item").nth(0).click();
    await expect(clientAccessRow.getByText("Key retained — create a replacement to copy a new value")).toBeVisible();
    await expect(clientKeyInput).toBeHidden();
    await expect(clientAccessRow.getByRole("button", { name: "Show key", exact: true })).toBeHidden();
    await expect(clientAccessRow.getByRole("button", { name: "Copy key", exact: true })).toBeHidden();
    await expect(revokeButton).toBeVisible();

    await revokeButton.click();
    await expect(clientAccessRow.getByText("No key created")).toBeVisible();
    await expect(revokeButton).toBeHidden();
    await expect(settingsDialog.getByRole("button", { name: "Create key" })).toBeVisible();
  }
});

test("settings modal remains usable on narrow screens", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 780 });
  await installAssetRoutes(page);
  await installManagementRoutes(page);

  await page.goto(baseURL);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  await expect(page.getByRole("dialog", { name: "Settings" })).toBeVisible();
  const modalBox = await page.locator("settings-modal").boundingBox();
  expect(modalBox).not.toBeNull();
  expect(modalBox.width).toBeLessThanOrEqual(390);
  await expect(page.getByRole("dialog", { name: "Settings" }).getByRole("button", { name: "Close" })).toBeVisible();
});

test("settings modal overlays MPR header and footer layers", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page);

  for (const viewport of settingsLayerViewports) {
    await page.setViewportSize({ width: viewport.width, height: viewport.height });
    await page.goto(baseURL);
    await page.getByTestId("avatar-menu").click();
    await page.getByTestId("avatar-menu-item").nth(0).click();

    const settingsDialog = page.getByRole("dialog", { name: "Settings" });
    await expect(settingsDialog).toBeVisible();
    await expect(settingsDialog.getByRole("button", { name: "Close" })).toBeVisible();

    const layerFacts = await settingsLayerFacts(page);
    expect(layerFacts.overlayZIndex).toBeGreaterThan(layerFacts.headerZIndex);
    expect(layerFacts.overlayZIndex).toBeGreaterThan(layerFacts.footerZIndex);
    expect(layerFacts.closeButtonHit.inSettingsModal).toBe(true);
    expect(layerFacts.closeButtonHit.inMprHeader).toBe(false);
    expect(layerFacts.modalBottomHit.inSettingsModal || layerFacts.modalBottomHit.inSettingsOverlay).toBe(true);
    expect(layerFacts.modalBottomHit.inMprFooter).toBe(false);
    expect(layerFacts.noticeHit.inSettingsModal || layerFacts.noticeHit.inSettingsOverlay).toBe(true);
    expect(layerFacts.noticeHit.inNotice).toBe(false);
    expect(layerFacts.headerHit.inSettingsModal || layerFacts.headerHit.inSettingsOverlay).toBe(true);
    expect(layerFacts.headerHit.inMprHeader).toBe(false);
    expect(layerFacts.footerHit.inSettingsModal || layerFacts.footerHit.inSettingsOverlay).toBe(true);
    expect(layerFacts.footerHit.inMprFooter).toBe(false);

    if (process.env.B020_SCREENSHOTS === "1") {
      await mkdir(b020ScreenshotDirectory, { recursive: true });
      await page.screenshot({ path: path.join(b020ScreenshotDirectory, `B020-settings-${viewport.name}.png`) });
    }

    await settingsDialog.getByRole("button", { name: "Close" }).click();
    await expect(settingsDialog).toBeHidden();
    await page.getByTestId("avatar-menu-item").nth(0).click();
    await expect(settingsDialog).toBeVisible();
  }
});

test("management notices occupy the header aux slot immediately before the avatar", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page);

  for (const viewport of settingsLayerViewports) {
    await page.setViewportSize({ width: viewport.width, height: viewport.height });
    await page.goto(baseURL);

    const notificationRegion = page.locator("#llm-proxy-header notification-region");
    const notice = notificationRegion.locator(".notice");
    await expect(notificationRegion).toHaveAttribute("role", "status");
    await expect(notificationRegion).toHaveAttribute("aria-live", "polite");
    await expect(notificationRegion).toHaveAttribute("aria-atomic", "true");
    await expect(notice).toHaveText("Workspace loaded");
    await expect(notice).toHaveAttribute("data-kind", "success");
    await expectHeaderNoticeGeometry(page);

    await page.getByRole("button", { name: "Refresh" }).click();
    await expect(notice).toHaveText("Usage refreshed");
    await expect(notice).toHaveAttribute("data-kind", "success");
    await expectHeaderNoticeGeometry(page);

    await installUsageResponse(page, httpInternalServerError);
    await page.getByRole("button", { name: "Refresh" }).click();
    await expect(notice).toHaveText("Request failed");
    await expect(notice).toHaveAttribute("data-kind", "error");
    await expectHeaderNoticeGeometry(page);

    await page.getByTestId("avatar-menu").click();
    await expect(page.getByTestId("avatar-dropdown")).toBeVisible();
    await page.getByTestId("avatar-menu-item").nth(0).click();

    const settingsDialog = page.getByRole("dialog", { name: "Settings" });
    await expect(settingsDialog).toBeVisible();
    const layerFacts = await settingsLayerFacts(page);
    expect(layerFacts.noticeHit.inSettingsModal || layerFacts.noticeHit.inSettingsOverlay).toBe(true);
    expect(layerFacts.noticeHit.inNotice).toBe(false);

    await settingsDialog.getByRole("button", { name: "Close" }).click();
    await expect(settingsDialog).toBeHidden();
    await installUsageResponse(page, httpOK);
    await page.getByRole("button", { name: "Refresh" }).click();
    await expect(notice).toHaveText("Usage refreshed");
    await expectHeaderNoticeGeometry(page);
  }
});

test("signed-out management notices occupy the header immediately before Sign in", async ({ page }) => {
  await installAssetRoutes(page, { initialAuthStatus: "unauthenticated" });
  await installManagementRoutes(page);

  for (const viewport of settingsLayerViewports) {
    await page.setViewportSize({ width: viewport.width, height: viewport.height });
    await page.goto(baseURL);

    const notificationRegion = page.locator("#llm-proxy-header notification-region");
    const notice = notificationRegion.locator(".notice");
    const signIn = page.getByRole("button", { name: "Sign in" });
    await expect(notice).toHaveText("Authentication required");
    await expect(signIn).toBeVisible();
    await expectHeaderNoticeSignInGeometry(page);

    await signIn.focus();
    await expect(signIn).toBeFocused();
  }
});

test("management notices auto-dismiss after ten seconds and replacement notices own a new deadline", async ({ page }) => {
  await page.clock.install({ time: new Date("2026-07-21T12:00:00Z") });
  await installAssetRoutes(page);
  await installManagementRoutes(page);
  await page.goto(baseURL);

  const notificationRegion = page.locator("#llm-proxy-header notification-region");
  const notice = notificationRegion.locator(".notice");
  const refresh = page.getByRole("button", { name: "Refresh" });
  const requests = page.locator("usage-card").filter({ hasText: "Requests" }).locator("strong");
  await expect(notice).toHaveText("Workspace loaded");
  await page.clock.fastForward(9_999);
  await expect(notificationRegion).toBeVisible();
  await page.clock.fastForward(1);
  await expect(notificationRegion).toBeHidden();

  await refresh.click();
  await expect(notice).toHaveText("Usage refreshed");
  await page.clock.fastForward(5_000);
  await installUsageResponse(page, httpOK, managementUsage({
    requests: 38,
    successful_requests: 36,
    text_requests: 36,
  }));
  await refresh.click();
  await expect(requests).toHaveText("38");
  await expect(notice).toHaveText("Usage refreshed");
  await page.clock.fastForward(5_000);
  await expect(notificationRegion).toBeVisible();
  await page.clock.fastForward(5_000);
  await expect(notificationRegion).toBeHidden();

  await installUsageResponse(page, httpInternalServerError);
  await refresh.click();
  await expect(notice).toHaveText("Request failed");
  await page.clock.fastForward(5_000);
  await installUsageResponse(page, httpOK);
  await refresh.click();
  await expect(notice).toHaveText("Usage refreshed");
  await page.clock.fastForward(5_000);
  await expect(notificationRegion).toBeVisible();
  await page.clock.fastForward(5_000);
  await expect(notificationRegion).toBeHidden();
});

test("informational notices auto-dismiss without impairing the signed-out Sign in control", async ({ page }) => {
  await page.clock.install({ time: new Date("2026-07-21T12:00:00Z") });
  await installAssetRoutes(page, { initialAuthStatus: "unauthenticated" });
  await installManagementRoutes(page);
  await page.goto(baseURL);

  const notificationRegion = page.locator("#llm-proxy-header notification-region");
  await expect(notificationRegion.locator(".notice")).toHaveText("Authentication required");
  await expectHeaderNoticeSignInGeometry(page);
  await page.clock.fastForward(9_999);
  await expect(notificationRegion).toBeVisible();
  await page.clock.fastForward(1);
  await expect(notificationRegion).toBeHidden();
  await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
});

test("header brand uses the local logo before its title without crowding the notice or avatar", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page);

  for (const viewport of settingsLayerViewports) {
    await page.setViewportSize({ width: viewport.width, height: viewport.height });
    await page.goto(baseURL);

    const brand = page.locator("#llm-proxy-header .llm-proxy-header-brand");
    const logo = brand.locator(".llm-proxy-header-brand__logo");
    const title = brand.locator(".llm-proxy-header-brand__title");
    await expect(brand).toHaveCount(1);
    await expect(brand).toHaveAttribute("slot", "brand");
    await expect(brand).toHaveAttribute("href", "/");
    await expect(brand).toHaveAttribute("aria-label", "LLM Proxy home");
    await expect(page.getByRole("link", { name: "LLM Proxy home" })).toHaveCount(1);
    await expect(logo).toHaveAttribute("src", appIconPath);
    await expect(logo).toHaveAttribute("alt", "");
    await expect(logo).toHaveAttribute("aria-hidden", "true");
    await expect(title).toHaveText("LLM Proxy");
    await expect(page.getByText("LLM Proxy", { exact: true })).toHaveCount(1);
    await brand.focus();
    await expect(brand).toBeFocused();
    await expectHeaderBrandGeometry(page);

    await page.getByRole("button", { name: "Refresh" }).click();
    await expect(page.locator("#llm-proxy-header .notice")).toHaveText("Usage refreshed");
    await expectHeaderBrandGeometry(page);

    await page.getByTestId("avatar-menu").click();
    await page.getByTestId("avatar-menu-item").nth(0).click();
    const settingsDialog = page.getByRole("dialog", { name: "Settings" });
    await expect(settingsDialog).toBeVisible();
    const brandHit = await page.locator("#llm-proxy-header .llm-proxy-header-brand").evaluate((brandElement) => {
      const rect = brandElement.getBoundingClientRect();
      const hitElement = document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2);
      return Boolean(hitElement?.closest("settings-overlay") || hitElement?.closest("settings-modal"));
    });
    expect(brandHit).toBe(true);
  }
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
 * @param {{ initialAuthStatus?: "authenticated" | "unauthenticated", emitInitialAuthEvent?: boolean }} options
 * @returns {Promise<void>}
 */
async function installAssetRoutes(page, options = {}) {
  await page.route("https://loopaware.mprlab.com/**", async (route) =>
    route.fulfill({ body: "", contentType: "application/javascript" }),
  );
  await page.route("https://accounts.google.com/**", async (route) => route.abort());
  await page.route("**/alpinejs@3.13.5/dist/module.esm.js", async (route) =>
    fulfillFile(route, "node_modules/alpinejs/dist/module.esm.js", "application/javascript"),
  );
  await page.route("**/js-yaml@4.3.0/dist/js-yaml.min.js", async (route) =>
    fulfillFile(route, "node_modules/js-yaml/dist/js-yaml.min.js", "application/javascript"),
  );
  await page.route("**/mpr-ui.css", async (route) =>
    route.fulfill({ body: mprShellLayerCSS(), contentType: "text/css" }),
  );
  await page.route("**/mpr-ui-config.js", async (route) =>
    route.fulfill({
      body: mprUIConfigMock(),
      contentType: "application/javascript",
    }),
  );
  await page.route("**/mpr-ui.js", async (route) =>
    route.fulfill({
      body: mprUIBundleMock(options.initialAuthStatus || "authenticated", options.emitInitialAuthEvent !== false),
      contentType: "application/javascript",
    }),
  );
}

/**
 * @param {import("@playwright/test").Page} page
 * @returns {Promise<void>}
 */
async function installClipboardMock(page) {
  await page.addInitScript(() => {
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: async (text) => {
          window.__llmProxyCopiedText = String(text);
        },
      },
    });
  });
}

/**
 * @param {import("@playwright/test").Page} page
 * @returns {Promise<string>}
 */
async function copiedText(page) {
  return page.evaluate(() => window.__llmProxyCopiedText || "");
}

/**
 * @param {import("@playwright/test").Page} page
 * @returns {Promise<{
 *   overlayZIndex: number,
 *   headerZIndex: number,
 *   footerZIndex: number,
 *   closeButtonHit: { inSettingsModal: boolean, inSettingsOverlay: boolean, inMprHeader: boolean, inMprFooter: boolean },
 *   modalBottomHit: { inSettingsModal: boolean, inSettingsOverlay: boolean, inMprHeader: boolean, inMprFooter: boolean },
 *   noticeHit: { inSettingsModal: boolean, inSettingsOverlay: boolean, inMprHeader: boolean, inMprFooter: boolean, inNotice: boolean },
 *   headerHit: { inSettingsModal: boolean, inSettingsOverlay: boolean, inMprHeader: boolean, inMprFooter: boolean },
 *   footerHit: { inSettingsModal: boolean, inSettingsOverlay: boolean, inMprHeader: boolean, inMprFooter: boolean },
 * }>}
 */
async function settingsLayerFacts(page) {
  return page.evaluate(() => {
    const overlayElement = document.querySelector("settings-overlay");
    const modalElement = document.querySelector("settings-modal");
    const closeButton = modalElement?.querySelector(".settings-header button");
    const headerElement = document.querySelector("mpr-header");
    const footerElement = document.querySelector("mpr-footer");
    const notificationRegion = document.querySelector("notification-region");
    const noticeElement = document.querySelector(".notice");
    if (!overlayElement || !modalElement || !closeButton || !headerElement || !footerElement || !notificationRegion || !noticeElement) {
      throw new Error("settings_layer_elements_missing");
    }

    const modalRect = modalElement.getBoundingClientRect();
    const closeButtonRect = closeButton.getBoundingClientRect();
    const headerRect = headerElement.getBoundingClientRect();
    const footerRect = footerElement.getBoundingClientRect();
    const noticeRect = noticeElement.getBoundingClientRect();
    const viewportWidth = document.documentElement.clientWidth;
    const hitAt = (xCoordinate, yCoordinate) => {
      const element = document.elementFromPoint(xCoordinate, yCoordinate);
      return {
        inSettingsModal: Boolean(element?.closest("settings-modal")),
        inSettingsOverlay: Boolean(element?.closest("settings-overlay")),
        inMprHeader: Boolean(element?.closest("mpr-header")),
        inMprFooter: Boolean(element?.closest("mpr-footer")),
        inNotice: Boolean(element?.closest(".notice")),
      };
    };
    const safeBandCenter = (rect) => rect.top + Math.min(Math.max(rect.height / 2, 2), Math.max(rect.height - 2, 2));

    return {
      overlayZIndex: Number.parseInt(getComputedStyle(overlayElement).zIndex, 10),
      headerZIndex: Number.parseInt(getComputedStyle(headerElement).zIndex, 10),
      footerZIndex: Number.parseInt(getComputedStyle(footerElement).zIndex, 10),
      closeButtonHit: hitAt(
        closeButtonRect.left + closeButtonRect.width / 2,
        closeButtonRect.top + closeButtonRect.height / 2,
      ),
      modalBottomHit: hitAt(modalRect.left + modalRect.width / 2, modalRect.bottom - 4),
      noticeHit: hitAt(noticeRect.left + noticeRect.width / 2, noticeRect.top + noticeRect.height / 2),
      headerHit: hitAt(viewportWidth / 2, safeBandCenter(headerRect)),
      footerHit: hitAt(viewportWidth / 2, safeBandCenter(footerRect)),
    };
  });
}

/**
 * @param {import("@playwright/test").Page} page
 * @returns {Promise<void>}
 */
async function expectHeaderNoticeGeometry(page) {
  const noticeFacts = await headerNoticeFacts(page);
  expect(noticeFacts.regionSlot).toBe("aux");
  expect(noticeFacts.regionBeforeAvatar).toBe(true);
  expect(noticeFacts.regionPointerEvents).toBe("none");
  expect(noticeFacts.noticePointerEvents).toBe("none");
  expect(noticeFacts.notice.top).toBeGreaterThanOrEqual(noticeFacts.header.top);
  expect(noticeFacts.notice.bottom).toBeLessThanOrEqual(noticeFacts.header.bottom);
  expect(noticeFacts.notice.left).toBeGreaterThanOrEqual(noticeFacts.header.left);
  expect(noticeFacts.notice.right).toBeLessThanOrEqual(noticeFacts.header.right);
  expect(noticeFacts.notice.right).toBeLessThanOrEqual(noticeFacts.avatar.left);
  expect(noticeFacts.avatar.right).toBeLessThanOrEqual(noticeFacts.header.right);
  expect(noticeFacts.avatar.top).toBeGreaterThanOrEqual(noticeFacts.header.top);
  expect(noticeFacts.avatar.bottom).toBeLessThanOrEqual(noticeFacts.header.bottom);
  expect(noticeFacts.avatarHit.inUser).toBe(true);
  expect(noticeFacts.avatarHit.inNotice).toBe(false);
}

/**
 * @param {import("@playwright/test").Page} page
 * @returns {Promise<void>}
 */
async function expectHeaderNoticeSignInGeometry(page) {
  const noticeFacts = await page.evaluate(() => {
    const headerElement = document.querySelector("#llm-proxy-header");
    const notificationRegion = headerElement?.querySelector("notification-region");
    const noticeElement = notificationRegion?.querySelector(".notice");
    const signInButton = headerElement?.querySelector('[data-testid="sign-in"]');
    if (!headerElement || !notificationRegion || !noticeElement || !signInButton) {
      throw new Error("header_sign_in_notification_elements_missing");
    }
    const noticeRect = noticeElement.getBoundingClientRect();
    const signInRect = signInButton.getBoundingClientRect();
    const headerRect = headerElement.getBoundingClientRect();
    const hit = document.elementFromPoint(signInRect.left + signInRect.width / 2, signInRect.top + signInRect.height / 2);
    return {
      header: { top: headerRect.top, right: headerRect.right, bottom: headerRect.bottom, left: headerRect.left },
      notice: { top: noticeRect.top, right: noticeRect.right, bottom: noticeRect.bottom, left: noticeRect.left },
      signIn: { top: signInRect.top, right: signInRect.right, bottom: signInRect.bottom, left: signInRect.left },
      signInHit: Boolean(hit?.closest('[data-testid="sign-in"]')),
    };
  });
  expect(noticeFacts.notice.top).toBeGreaterThanOrEqual(noticeFacts.header.top);
  expect(noticeFacts.notice.bottom).toBeLessThanOrEqual(noticeFacts.header.bottom);
  expect(noticeFacts.notice.right).toBeLessThanOrEqual(noticeFacts.signIn.left);
  expect(noticeFacts.signIn.right).toBeLessThanOrEqual(noticeFacts.header.right);
  expect(noticeFacts.signInHit).toBe(true);
}

/**
 * @param {import("@playwright/test").Page} page
 * @returns {Promise<{
 *   regionSlot: string | null,
 *   regionBeforeAvatar: boolean,
 *   regionPointerEvents: string,
 *   noticePointerEvents: string,
 *   header: { top: number, right: number, bottom: number, left: number },
 *   notice: { top: number, right: number, bottom: number, left: number },
 *   avatar: { top: number, right: number, bottom: number, left: number },
 *   avatarHit: { inUser: boolean, inNotice: boolean },
 * }>}
 */
async function headerNoticeFacts(page) {
  return page.evaluate(() => {
    const headerElement = document.querySelector("#llm-proxy-header");
    const notificationRegion = headerElement?.querySelector("notification-region");
    const noticeElement = notificationRegion?.querySelector(".notice");
    const userMenu = headerElement?.querySelector("mpr-user");
    const avatarButton = userMenu?.querySelector('[data-testid="avatar-menu"]');
    if (!headerElement || !notificationRegion || !noticeElement || !userMenu || !avatarButton) {
      throw new Error("header_notification_elements_missing");
    }

    const noticeRect = noticeElement.getBoundingClientRect();
    const headerRect = headerElement.getBoundingClientRect();
    const avatarRect = avatarButton.getBoundingClientRect();
    const hitAtElementCenter = (element) => {
      const rect = element.getBoundingClientRect();
      return document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2);
    };
    const avatarHit = hitAtElementCenter(avatarButton);

    return {
      regionSlot: notificationRegion.getAttribute("slot"),
      regionBeforeAvatar: Boolean(notificationRegion.compareDocumentPosition(userMenu) & Node.DOCUMENT_POSITION_FOLLOWING),
      regionPointerEvents: getComputedStyle(notificationRegion).pointerEvents,
      noticePointerEvents: getComputedStyle(noticeElement).pointerEvents,
      header: {
        top: headerRect.top,
        right: headerRect.right,
        bottom: headerRect.bottom,
        left: headerRect.left,
      },
      notice: {
        top: noticeRect.top,
        right: noticeRect.right,
        bottom: noticeRect.bottom,
        left: noticeRect.left,
      },
      avatar: {
        top: avatarRect.top,
        right: avatarRect.right,
        bottom: avatarRect.bottom,
        left: avatarRect.left,
      },
      avatarHit: {
        inUser: Boolean(avatarHit?.closest("mpr-user")),
        inNotice: Boolean(avatarHit?.closest(".notice")),
      },
    };
  });
}

/**
 * @param {import("@playwright/test").Page} page
 * @returns {Promise<void>}
 */
async function expectHeaderBrandGeometry(page) {
  const brandFacts = await headerBrandFacts(page);
  expect(brandFacts.logoBeforeTitle).toBe(true);
  expect(brandFacts.brand.top).toBeGreaterThanOrEqual(brandFacts.header.top);
  expect(brandFacts.brand.bottom).toBeLessThanOrEqual(brandFacts.header.bottom);
  expect(brandFacts.brand.left).toBeGreaterThanOrEqual(brandFacts.header.left);
  expect(brandFacts.logo.left).toBeGreaterThanOrEqual(brandFacts.brand.left);
  expect(brandFacts.logo.right).toBeLessThanOrEqual(brandFacts.title.left);
  expect(brandFacts.title.right).toBeLessThanOrEqual(brandFacts.brand.right);
  expect(brandFacts.brand.right).toBeLessThanOrEqual(brandFacts.notice.left);
  expect(brandFacts.notice.right).toBeLessThanOrEqual(brandFacts.avatar.left);
  expect(brandFacts.avatar.right).toBeLessThanOrEqual(brandFacts.header.right);
}

/**
 * @param {import("@playwright/test").Page} page
 * @returns {Promise<{
 *   logoBeforeTitle: boolean,
 *   header: { top: number, right: number, bottom: number, left: number },
 *   brand: { top: number, right: number, bottom: number, left: number },
 *   logo: { top: number, right: number, bottom: number, left: number },
 *   title: { top: number, right: number, bottom: number, left: number },
 *   notice: { top: number, right: number, bottom: number, left: number },
 *   avatar: { top: number, right: number, bottom: number, left: number },
 * }>}
 */
async function headerBrandFacts(page) {
  return page.evaluate(() => {
    const headerElement = document.querySelector("#llm-proxy-header");
    const brandElement = headerElement?.querySelector(".llm-proxy-header-brand");
    const logoElement = brandElement?.querySelector(".llm-proxy-header-brand__logo");
    const titleElement = brandElement?.querySelector(".llm-proxy-header-brand__title");
    const noticeElement = headerElement?.querySelector(".notice");
    const avatarButton = headerElement?.querySelector('[data-testid="avatar-menu"]');
    if (!headerElement || !brandElement || !logoElement || !titleElement || !noticeElement || !avatarButton) {
      throw new Error("header_brand_elements_missing");
    }

    const headerRect = headerElement.getBoundingClientRect();
    const brandRect = brandElement.getBoundingClientRect();
    const logoRect = logoElement.getBoundingClientRect();
    const titleRect = titleElement.getBoundingClientRect();
    const noticeRect = noticeElement.getBoundingClientRect();
    const avatarRect = avatarButton.getBoundingClientRect();
    const rectFacts = (rect) => ({
      top: rect.top,
      right: rect.right,
      bottom: rect.bottom,
      left: rect.left,
    });

    return {
      logoBeforeTitle: Boolean(logoElement.compareDocumentPosition(titleElement) & Node.DOCUMENT_POSITION_FOLLOWING),
      header: rectFacts(headerRect),
      brand: rectFacts(brandRect),
      logo: rectFacts(logoRect),
      title: rectFacts(titleRect),
      notice: rectFacts(noticeRect),
      avatar: rectFacts(avatarRect),
    };
  });
}

/**
 * @param {import("@playwright/test").Page} page
 * @param {number} status
 * @param {object} [usage]
 * @returns {Promise<void>}
 */
async function installUsageResponse(page, status, usage = managementUsage()) {
  await page.unroute(`${baseURL}/api/management/usage`);
  await page.route(`${baseURL}/api/management/usage`, async (route) => {
    await route.fulfill({ status, json: usage });
  });
}

/**
 * @param {import("@playwright/test").Page} page
 * @param {{ usageStatus?: number, admin?: boolean, hasSecret?: boolean, generatedSecret?: string, profileStatus?: number, profileStatuses?: number[], profileError?: string, malformedRoutingDefaults?: boolean, maskedKeys?: Record<string, string>, providerKeys?: Record<string, string> }} options
 * @returns {Promise<void>}
 */
async function installManagementRoutes(page, options = {}) {
  const profileStatuses = [...(options.profileStatuses || [])];
  const profile = managementProfile(options.admin || false, options.hasSecret !== false);
  const providerKeys = {
    openai: "sk-owner-openai",
    deepseek: "sk-owner-deepseek",
    meta: "sk-owner-meta",
    ...options.providerKeys,
  };
  for (const [providerID, maskedKey] of Object.entries(options.maskedKeys || {})) {
    const provider = profile.providers.find((candidateProvider) => candidateProvider.id === providerID);
    if (!provider) {
      throw new Error(`management_fixture_provider_missing:${providerID}`);
    }
    provider.masked_key = maskedKey;
  }
  if (options.malformedRoutingDefaults) {
    profile.providers.push({
      id: "anthropic",
      label: "Anthropic",
      aliases: [],
      has_key: false,
      text_model: "claude-sonnet-5",
      system_prompt: "",
      text_default_model: "claude-sonnet-5",
      text_models: [{ id: "claude-sonnet-5" }],
      supports_dictation: false,
      dictation_models: [],
    });
    profile.tenant.defaults.provider = "anthropic";
  }
  await page.route(`${baseURL}/api/management/profile`, async (route) => {
    const profileStatus = profileStatuses.length > 0 ? profileStatuses.shift() : options.profileStatus;
    if (profileStatus && profileStatus !== httpOK) {
      await route.fulfill({ status: profileStatus, body: options.profileError || "authentication_required" });
      return;
    }
    await route.fulfill({ headers: { "Cache-Control": "no-store" }, json: profile });
  });
  await page.route(`${baseURL}/api/management/usage`, async (route) => {
    await route.fulfill({ status: options.usageStatus || httpOK, json: managementUsage() });
  });
  await page.route(`${baseURL}/api/management/admin/users`, async (route) => {
    await route.fulfill({ json: managementAdminUsers() });
  });
  await page.route(`${baseURL}/api/management/secrets`, async (route) => {
    if (route.request().method() === "POST") {
      profile.tenant.has_secret = true;
      await route.fulfill({
        headers: { "Cache-Control": "no-store" },
        json: {
          secret: options.generatedSecret || "llmp_test_generated_secret",
          profile,
        },
      });
      return;
    }
    profile.tenant.has_secret = false;
    await route.fulfill({ headers: { "Cache-Control": "no-store" }, json: profile });
  });
	await page.route(`${baseURL}/api/management/defaults`, async (route) => {
    const defaults = /** @type {typeof profile.tenant.defaults} */ (route.request().postDataJSON());
    profile.tenant.defaults = defaults;
		await route.fulfill({ headers: { "Cache-Control": "no-store" }, json: profile });
	});
  await page.route(`${baseURL}${managementProviderKeysPath}/**`, async (route) => {
    const request = route.request();
    const providerPath = new URL(request.url()).pathname.slice(`${managementProviderKeysPath}/`.length);
    const [providerID, action] = providerPath.split("/");
    const provider = profile.providers.find((candidateProvider) => candidateProvider.id === providerID);
    if (!provider) {
      await route.fulfill({ status: 404 });
      return;
    }
    if (action === "reveal") {
      if (!provider.has_key) {
        await route.fulfill({ status: 404 });
        return;
      }
      await route.fulfill({ headers: { "Cache-Control": "no-store" }, json: { api_key: providerKeys[providerID] } });
      return;
    }
    if (request.method() === "PUT") {
      const providerSettings = request.postDataJSON();
      if (providerSettings.api_key) {
        providerKeys[providerID] = providerSettings.api_key;
      }
      provider.has_key = true;
      provider.masked_key = "sk-...saved";
      provider.text_model = providerSettings.text_model;
      provider.system_prompt = providerSettings.system_prompt;
      await route.fulfill({ headers: { "Cache-Control": "no-store" }, json: profile });
      return;
    }
    if (request.method() === "DELETE") {
      delete providerKeys[providerID];
      provider.has_key = false;
      delete provider.masked_key;
      await route.fulfill({ headers: { "Cache-Control": "no-store" }, json: profile });
      return;
    }
    await route.fulfill({ status: httpInternalServerError });
  });
}

/**
 * @param {string} providerID
 * @param {string} [action]
 * @returns {string}
 */
function providerKeyEndpointURL(providerID, action = "") {
	return `${baseURL}${managementProviderKeysPath}/${providerID}${action ? `/${action}` : ""}`;
}

/**
 * @param {import("@playwright/test").Page} page
 * @param {string} value
 * @returns {Promise<boolean>}
 */
async function browserStorageContains(page, value) {
	return page.evaluate((candidateValue) => {
		const browserStorageValues = [
			...Object.values(localStorage),
			...Object.values(sessionStorage),
		];
		return browserStorageValues.some((storedValue) => storedValue.includes(candidateValue));
	}, value);
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

  const routePath =
    requestURL.pathname === "/" || requestURL.pathname.endsWith("/")
      ? path.join(requestURL.pathname, "index.html")
      : requestURL.pathname;
  const filePath = path.normalize(path.join(siteRoot, routePath));
  if (!filePath.startsWith(siteRoot)) {
    response.writeHead(404);
    response.end();
    return;
  }
  const fileStats = await stat(filePath).catch(() => null);
  if (!fileStats || fileStats.isDirectory()) {
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
 * @param {boolean} hasSecret
 * @returns {object}
 */
function managementProfile(isAdmin = false, hasSecret = true) {
  return {
    user: {
      id: "user_1",
      email: "owner@example.com",
      display_name: "Owner",
      is_admin: isAdmin,
    },
    tenant: {
      id: "tenant_1",
      has_secret: hasSecret,
      defaults: {
        provider: "openai",
        model: "gpt-4.1",
        dictation_provider: "openai",
        dictation_model: "gpt-4o-mini-transcribe",
        system_prompt: "",
        reasoning_effort: "",
      },
    },
    providers: [
      {
        id: "openai",
        label: "OpenAI",
        aliases: [],
        has_key: true,
        masked_key: "sk-...1234",
        text_model: "gpt-4.1",
        system_prompt: "Use concise answers.",
        text_default_model: "gpt-4.1",
        text_models: [
          { id: "gpt-4.1" },
          { id: "gpt-4o-mini" },
          {
            id: "gpt-5",
            reasoning_effort: {
              adapter: "openai_responses",
              efforts: ["minimal", "low", "medium", "high"],
            },
          },
          {
            id: "gpt-5.5",
            reasoning_effort: {
              adapter: "openai_responses",
              efforts: ["none", "low", "medium", "high", "xhigh"],
            },
          },
          {
            id: "gpt-5.5-pro",
            reasoning_effort: {
              adapter: "openai_responses",
              efforts: ["medium", "high", "xhigh"],
            },
          },
          {
            id: "gpt-5.6",
            reasoning_effort: {
              adapter: "openai_responses",
              efforts: ["none", "low", "medium", "high", "xhigh", "max"],
            },
          },
        ],
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
        text_model: "deepseek-chat",
        system_prompt: "",
        text_default_model: "deepseek-chat",
        text_models: [{ id: "deepseek-chat" }],
        supports_dictation: false,
        dictation_models: [],
      },
      {
        id: "meta",
        label: "Meta",
        aliases: [],
        has_key: true,
        masked_key: "sk-...meta",
        text_model: "muse-spark-1.1",
        system_prompt: "",
        text_default_model: "muse-spark-1.1",
        text_models: [{ id: "muse-spark-1.1" }],
        supports_dictation: false,
        dictation_models: [],
      },
      {
        id: "grok",
        label: "Grok",
        aliases: ["xai"],
        has_key: false,
        text_model: "grok-4.3",
        system_prompt: "",
        text_default_model: "grok-4.3",
        text_models: [{ id: "grok-4.3" }],
        supports_dictation: true,
        dictation_default_model: "xai-stt",
        dictation_models: ["xai-stt"],
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
 * @param {Partial<Record<string, number>>} [totalOverrides]
 * @returns {object}
 */
function managementUsage(totalOverrides = {}) {
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
      ...totalOverrides,
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
 * @param {"authenticated" | "unauthenticated"} initialAuthStatus
 * @param {boolean} emitInitialAuthEvent
 * @returns {string}
 */
function mprUIBundleMock(initialAuthStatus, emitInitialAuthEvent) {
  return `
class MprHeader extends HTMLElement {
  connectedCallback() {
    this.mountActions();
    this.setAuthStatus(${JSON.stringify(initialAuthStatus)});
    queueMicrotask(() => {
      this.dispatchEvent(new CustomEvent("mpr-ui:auth:status-change", {
        bubbles: true,
        detail: { status: ${JSON.stringify(initialAuthStatus)} }
      }));
      if (${JSON.stringify(initialAuthStatus)} === "authenticated" && ${JSON.stringify(emitInitialAuthEvent)}) {
        this.dispatchEvent(new CustomEvent("mpr-ui:auth:authenticated", {
          bubbles: true,
          detail: { profile: { user_id: "user-1", user_email: "user@example.com" } }
        }));
      }
    });
  }

  mountActions() {
    const actions = document.createElement("div");
    actions.className = "mpr-header__actions";
    const signIn = document.createElement("button");
    signIn.type = "button";
    signIn.dataset.testid = "sign-in";
    signIn.textContent = "Sign in";
    actions.append(signIn, ...this.querySelectorAll('[slot="aux"]'));
    this.append(actions);
  }

  setAuthStatus(status) {
    this.setAttribute("data-mpr-auth-status", status);
    const signIn = this.querySelector('[data-testid="sign-in"]');
    const userMenu = this.querySelector("mpr-user");
    if (signIn) {
      signIn.hidden = status === "authenticated";
    }
    if (userMenu) {
      userMenu.hidden = status !== "authenticated";
    }
  }
}
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
window.__llmProxyMprAuthenticate = () => {
  const header = document.querySelector("mpr-header");
  if (!header) {
    throw new Error("mpr_header_missing");
  }
  header.setAuthStatus("authenticated");
  header.dispatchEvent(new CustomEvent("mpr-ui:auth:status-change", {
    bubbles: true,
    detail: { status: "authenticated" }
  }));
  header.dispatchEvent(new CustomEvent("mpr-ui:auth:authenticated", {
    bubbles: true,
    detail: { profile: { user_id: "user-1", user_email: "user@example.com" } }
  }));
};
`;
}

/**
 * @returns {string}
 */
function mprUIConfigMock() {
  return `
(() => {
  let orchestrationPromise = null;

  function autoOrchestrate() {
    const header = document.querySelector("mpr-header[data-config-url]");
    const bundleMarker = document.querySelector("script[data-mpr-ui-bundle-src]");
    if (!header || !bundleMarker) {
      throw new Error("mpr_ui_declarative_contract_missing");
    }
    const configUrl = header.getAttribute("data-config-url");
    const bundleUrl = bundleMarker.getAttribute("data-mpr-ui-bundle-src");
    orchestrationPromise = fetch(configUrl, { cache: "no-store" })
      .then((response) => {
        if (!response.ok) {
          throw new Error("mpr_ui_config_request_failed");
        }
        return response.text();
      })
      .then(() => new Promise((resolve, reject) => {
        const bundleScript = document.createElement("script");
        bundleScript.src = bundleUrl;
        bundleScript.onload = resolve;
        bundleScript.onerror = () => reject(new Error("mpr_ui_bundle_request_failed"));
        document.head.appendChild(bundleScript);
      }));
    return orchestrationPromise;
  }

  window.MPRUI = {
    whenAutoOrchestrationReady: () => orchestrationPromise || Promise.resolve()
  };
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", autoOrchestrate, { once: true });
  } else {
    autoOrchestrate();
  }
})();
`;
}

/**
 * @returns {string}
 */
function mprShellLayerCSS() {
  return `
mpr-header {
  position: sticky;
  top: 0;
  z-index: 1200;
  display: flex;
  min-height: 56px;
  align-items: center;
  justify-content: flex-end;
  box-sizing: border-box;
  padding: 0 16px;
  background: rgba(3, 23, 32, 0.95);
}

mpr-header .mpr-header__actions {
  display: flex;
  min-inline-size: 0;
  align-items: center;
}

mpr-footer {
  position: fixed;
  right: 0;
  bottom: 0;
  left: 0;
  z-index: 1200;
  display: block;
  min-height: 64px;
  background: rgba(3, 23, 32, 0.95);
}
`;
}
