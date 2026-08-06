// @ts-check

import { expect, test } from "@playwright/test";
import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import { createReadStream } from "node:fs";
import { mkdir, mkdtemp, readFile, rm, stat } from "node:fs/promises";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const siteSourceRoot = path.join(repoRoot, "site");
const executeFile = promisify(execFile);
const canonicalOpenAPIFile = path.join(repoRoot, "docs/openapi.yaml");
const configPath = "/config-ui.yaml";
const applicationPath = "/app/";
const removedApplicationPath = "/manage/";
const defaultTenantID = "tenant_1";
const managementDefaultTenantPath = `/api/management/tenants/${defaultTenantID}`;
const managementProviderKeysPath = `${managementDefaultTenantPath}/provider-keys`;
const faviconPath = "/assets/llm-proxy/img/favicon.svg";
const appIconPath = "/assets/llm-proxy/img/llm-proxy-icon.svg";
const resourcesPath = "/resources/";
const representativeResourcePath = "/resources/multi-provider-llm-proxy/";
const clientAuthenticationResourcePath = "/resources/llm-proxy-client-authentication/";
const sitemapPath = "/sitemap.xml";
const robotsPath = "/robots.txt";
const apiDocumentationPath = "/docs/";
const openAPIPath = "/openapi.yaml";
const repositoryUsageURL = "https://github.com/tyemirov/llm-proxy#usage";
const mprUICSSURL = "https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui.css";
const mprUIConfigURL = "https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui-config.js";
const mprUIBundleURL = "https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui.js";
const compactLandingFooterMaxHeight = 56;
const b020ScreenshotDirectory = path.join(repoRoot, "output/playwright");
const httpOK = 200;
const httpNotFound = 404;
const httpInternalServerError = 500;
const noticeClockPauseLeadMilliseconds = 5_000;
const noticeClockPreDeadlineAdvanceMilliseconds = 4_000;
const noticeClockPostDeadlineAdvanceMilliseconds = 2_000;
const tenantAccessDesktopMaxHeight = 64;
const usageIntervals = Object.freeze([
  { id: "all", label: "ALL", requests: 91, totalTokens: 91_000, providerCount: 1 },
  { id: "30d", label: "30 days", requests: 37, totalTokens: 12_345, providerCount: 2 },
  { id: "7d", label: "7 days", requests: 7, totalTokens: 7_000, providerCount: 1 },
  { id: "1d", label: "1 day", requests: 1, totalTokens: 1_000, providerCount: 1 },
]);
const mimeTypes = Object.freeze({
  ".css": "text/css",
  ".html": "text/html",
  ".js": "application/javascript",
  ".svg": "image/svg+xml",
  ".txt": "text/plain",
  ".xml": "application/xml",
  ".yaml": "application/yaml",
});
const generatedResourcePageCount = 46;
const landingModifiedDate = "2026-08-06";
const seoContentModifiedDate = "2026-07-11";
const seoCurrentContentModifiedDate = "2026-07-22";
const seoMigrationContentModifiedDate = "2026-07-25";
const seoUsageContentModifiedDate = "2026-07-26";
const seoClientDocumentationModifiedDate = "2026-07-26";
const settingsLayerViewports = Object.freeze([
  { name: "desktop", width: 1280, height: 720 },
  { name: "compact", width: 480, height: 780 },
  { name: "mobile", width: 390, height: 780 },
]);

let server;
let baseURL = "";
let siteRoot = siteSourceRoot;
let renderedSiteTempRoot = "";
let renderedSiteRoot = "";

test.beforeAll(async () => {
  renderedSiteTempRoot = await mkdtemp(path.join(os.tmpdir(), "llm-proxy-site-"));
  renderedSiteRoot = path.join(renderedSiteTempRoot, "rendered");
  await executeFile(
    "go",
    [
      "run",
      "./cmd/cli",
      "--config",
      "configs/config.yml",
      "--site-source",
      "site",
      "--render-site-output",
      renderedSiteRoot,
    ],
    { cwd: repoRoot },
  );
  await executeFile(
    "./scripts/stage-openapi-publication.sh",
    ["docs/openapi.yaml", renderedSiteRoot],
    { cwd: repoRoot },
  );
  siteRoot = renderedSiteRoot;
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
  await rm(renderedSiteTempRoot, { recursive: true, force: true });
});

test("public landing explains the product and exposes the generated capability catalog", async ({ request }) => {
  const htmlResponse = await request.get(baseURL);
  expect(htmlResponse.status()).toBe(httpOK);
  let html = await htmlResponse.text();
  expect(html).toContain('<link rel="canonical" href="https://llm-proxy.mprlab.com/">');
  expect(html).toContain("One stable interface for the models your products depend on.");
  expect(html).toContain("Client access can be generated and rotated independently");
  expect(html).toContain("plain text, JSON, XML, or CSV responses");
  expect(html).toContain(`href="${applicationPath}"`);
  expect(html).toContain(`href="${resourcesPath}"`);
  expect(html).toContain(`href="${apiDocumentationPath}"`);
  expect(html).toContain(`"href":"${openAPIPath}"`);
  expect(html).toContain('<table class="catalog-table">');
  expect(html).toContain('<strong>12</strong><span>Text providers</span>');
  expect(html).toContain('<code>gpt-4.1</code><span class="catalog-model__default">Default</span>');
  expect(html).toContain("gpt-4o-mini-transcribe");
  expect(html).toContain("gemini-2.5-flash");
  expect(html).toContain("claude-sonnet-4-6");
  expect(html).toContain("grok-4.3");
  expect(html).not.toContain("api_key");
  expect(html).not.toContain("base_url");
  expect(html).not.toContain("data-config-url");
  expect(html).toContain(`<link rel="stylesheet" href="${mprUICSSURL}">`);
  expect(html).toContain(`<script defer src="${mprUIBundleURL}"></script>`);
  expect(html).not.toContain(mprUIConfigURL);
  expect(html).toContain('<mpr-footer\n      size="small"\n      sticky="false"');
  expect(html).toContain('prefix-text="LLM Proxy"');
  expect(html).toContain('privacy-link-hidden="true"');
  expect(html).toContain('<meta name="theme-color" content="#0f1114">');

  const page = await request.get(`${baseURL}${applicationPath}`);
  expect(page.status()).toBe(httpOK);
  const managementHTML = await page.text();
  expect(managementHTML).toContain('<meta name="robots" content="noindex, nofollow">');
  expect(managementHTML).toContain('<link rel="canonical" href="https://llm-proxy.mprlab.com/app/">');
  expect(managementHTML).toContain(`data-config-url="${configPath}"`);
  expect(managementHTML).toContain(`<link rel="stylesheet" href="${mprUICSSURL}">`);
  expect(managementHTML).toContain(`<script src="${mprUIConfigURL}"></script>`);
  expect(managementHTML).toContain(`data-mpr-ui-bundle-src="${mprUIBundleURL}"`);
  expect(managementHTML).toContain('<script type="module" src="/assets/llm-proxy/js/startupGuard.js?v=20260727i036"></script>');
  expect(managementHTML).toContain(
    '<script id="llm-proxy-application-module" type="module" src="/assets/llm-proxy/js/app.js?v=20260727i036"></script>',
  );
  expect(managementHTML).not.toContain("MarcoPoloResearchLab/mpr-ui@v");
  expect(managementHTML).not.toContain("tauth.js");
  expect(managementHTML).toMatch(/<notification-region\s+slot="aux"[\s\S]*?<mpr-user\s+slot="aux"/);
  expect(managementHTML).toContain('<body x-data="llmProxyKeyManagement" x-init="init()">');
  expect(managementHTML).not.toContain('x-init="bindNotificationRegion($el)"');
  expect(managementHTML).toContain('<a slot="brand" class="llm-proxy-header-brand" href="/" aria-label="LLM Proxy home">');
  expect(managementHTML).toContain(`<img class="llm-proxy-header-brand__logo" src="${appIconPath}" alt="" aria-hidden="true">`);
  expect(managementHTML).toContain('<span class="llm-proxy-header-brand__title">LLM Proxy</span>');
  expect(managementHTML).not.toContain("brand-label=");
  expect(managementHTML).not.toContain("data:image");
  expect(managementHTML).toContain(
    '<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Material+Symbols+Outlined&amp;icon_names=content_copy,delete,key,visibility,visibility_off&amp;display=block">',
  );
  expect(managementHTML).toContain(
    '<span class="material-symbols-outlined" x-show="!providerKeyVisible" aria-hidden="true">visibility</span>',
  );
  expect(managementHTML).toContain(
    '<span class="material-symbols-outlined" x-show="providerKeyVisible" aria-hidden="true">visibility_off</span>',
  );

  const removedApplicationResponse = await request.get(`${baseURL}${removedApplicationPath}`);
  expect(removedApplicationResponse.status()).toBe(httpNotFound);

  html = managementHTML;
  expect(html).toContain('<meta name="theme-color" content="#0076c3">');
  expect(html).toContain(`<link rel="icon" type="image/svg+xml" href="${faviconPath}">`);
  expect(html).toContain(`<link rel="apple-touch-icon" href="${appIconPath}">`);
  expect(html).toContain(`data-config-url="${configPath}"`);
  expect(html).toContain(`<link rel="stylesheet" href="${mprUICSSURL}">`);
  expect(html).toContain(`<script src="${mprUIConfigURL}"></script>`);
  expect(html).toContain(`data-mpr-ui-bundle-src="${mprUIBundleURL}"`);
  expect(html).toContain('<script type="module" src="/assets/llm-proxy/js/startupGuard.js?v=20260727i036"></script>');
  expect(html).toContain(
    '<script id="llm-proxy-application-module" type="module" src="/assets/llm-proxy/js/app.js?v=20260727i036"></script>',
  );
  expect(html).not.toContain("MarcoPoloResearchLab/mpr-ui@v");
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
    '<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Material+Symbols+Outlined&amp;icon_names=content_copy,delete,key,visibility,visibility_off&amp;display=block">',
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
  const providerVisibilityOffset = html.indexOf('class="icon-only provider-key-visibility-toggle"');
  const textModelOffset = html.indexOf('x-on:change="handleSelectedProviderTextModelChange($event)"');
  const providerRemovalOffset = html.indexOf('class="icon-only danger provider-key-remove"');
  expect(providerSelectorOffset).toBeGreaterThan(-1);
  expect(providerSelectorOffset).toBeLessThan(providerKeyFieldOffset);
  expect(providerKeyFieldOffset).toBeLessThan(providerVisibilityOffset);
  expect(providerVisibilityOffset).toBeLessThan(providerRemovalOffset);
  expect(providerRemovalOffset).toBeLessThan(textModelOffset);
  expect(html).toContain('<h2 id="provider-settings-title" class="eyebrow" x-text="copy.providersEyebrow"></h2>');
  expect(html).not.toContain('x-text="copy.providersTitle"');
  expect(html).toContain('role="alertdialog"');
  expect(html).toContain('x-on:click="requestSelectedProviderKeyRemoval()"');
  expect(html).toContain('x-show="selectedProvider.has_key || selectedProviderKeyHasInput"');
  expect(html).toContain('x-on:paste="handleSelectedProviderKeyPaste()"');
  expect(html).toContain('x-on:change="autosaveSelectedProvider()"');
  expect(html).toContain('role="status" aria-live="polite"');
  expect(html).toContain('x-show="providerKeyVerificationPending"');
  expect(html).toContain('x-show="providerKeyVerificationFailed"');
  expect(html).not.toContain("provider-settings-form");
  expect(html).not.toContain("saveSelectedProviderKey");
  expect(html).not.toContain('x-on:click="removeSelectedProviderKey()"');
  expect(html).toContain('<p id="settings-title" class="eyebrow" x-text="copy.settingsEyebrow"></p>');
  expect(html).toContain('class="icon-only settings-close"');
  expect(html).toContain('x-ref="settingsModal"');
  expect(html).toContain("x-bind:aria-describedby=\"settingsRequired ? 'settings-requirement' : null\"");
  expect(html).toContain('x-on:keydown.tab="trapSettingsFocus($event)"');
  expect(html).toContain('id="settings-requirement"');
  expect(html).toContain('x-show="settingsRequired"');
  expect(html).toContain(
    '<svg class="utility-icon close-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true" focusable="false">',
  );
  expect(html).not.toContain('x-text="copy.settingsTitle"');
  expect(html).not.toContain("<tenant-context-bar");
  expect(html).not.toContain("Active tenant");
  expect(html).toContain('x-bind:aria-label="copy.usageTenant"');
  expect(html).toContain('x-on:change="handleUsageTenantSelection($event)"');
  expect(html).toContain('x-text="copy.allTenants"');
  expect(html).toContain('<tenant-access-row role="group" x-bind:aria-label="copy.tenantAccess">');
  expect(html).toContain('x-on:change="handleSettingsTenantSelection($event)"');
  expect(html).toContain('x-bind:aria-label="copy.tenantContext"');
  expect(html).toContain("<client-access-key>");
  expect(html).not.toContain("tenant-management");
  expect(html).not.toContain("client-access-tenant");
  expect(html).not.toContain('x-text="settingsTenantID"');
  expect(html).toContain('x-on:click="beginTenantNameEdit()"');
  expect(html).toContain('x-on:click="cancelTenantNameEdit()"');
  expect(html).toContain('x-on:input="handleTenantNameInput($event)"');
  expect(html).toContain('x-on:click="requestTenantDeletion()"');
  expect(html).toContain('class="tenant-rename-dialog"');
  expect(html).toContain('class="client-key-replace-dialog"');
  expect(html).not.toContain("copy.tenantTitle");
  expect(html).toContain('class="icon-button client-key-create"');
  expect(html).toContain('x-show="!hasSecret"');
  expect(html).toContain('class="icon-button client-key-replace"');
  expect(html).toContain('x-on:click="requestClientKeyReplacement()"');
  expect(html).toContain('<span class="material-symbols-outlined" aria-hidden="true">key</span>');
  expect(html).toContain('<span class="tenant-access-action-label" x-text="copy.replaceKey"></span>');
  expect(html).not.toContain("recycle-icon");
  expect(html).toContain('class="icon-only client-key-copy"');
  expect(html).toContain('x-on:click="copyGeneratedSecret()"');
  expect(html).not.toContain("client-key-revoke");
  expect(html).not.toContain("revokeSecret()");
  expect(html).toContain('<span class="material-symbols-outlined" x-show="!generatedSecretVisible" aria-hidden="true">visibility</span>');
  expect(html).toContain('<span class="material-symbols-outlined" x-show="generatedSecretVisible" aria-hidden="true">visibility_off</span>');
  expect(html).toContain('<span class="material-symbols-outlined" aria-hidden="true">content_copy</span>');
  expect(html).not.toContain('class="copy-icon"');
  expect(html).not.toContain("tenant-facts");
  expect(html).not.toContain("secret-output");
  expect(html).not.toContain("copy.copySecret");
  expect(html).not.toContain("Generated secret");
  expect(html).toContain('x-model="defaults.reasoning_effort"');
  expect(html).toContain('class="text-routing-controls"');
  expect(html).toContain('x-on:change="handleTextProviderDefaultChange($event)"');
  expect(html).toContain('x-on:change="handleTextModelDefaultChange($event)"');
  expect(html).toContain('x-on:change="handleReasoningEffortDefaultChange($event)"');
  expect(html).toContain('x-on:change="handleDictationProviderDefaultChange($event)"');
  expect(html).toContain('x-on:change="handleDictationModelDefaultChange($event)"');
  expect(html).toContain('x-on:change="autosaveRoutingDefaults()"');
  expect(html).toContain('class="settings-form-wide system-prompt-disclosure"');
  expect(html).toContain('x-bind:open="routingSystemPromptOpen"');
  expect(html).toContain('x-on:toggle="routingSystemPromptOpen = $event.currentTarget.open"');
  expect(html).toContain('class="provider-system-prompt system-prompt-disclosure"');
  expect(html).toContain('x-bind:open="providerSystemPromptOpen"');
  expect(html).toContain('x-on:toggle="providerSystemPromptOpen = $event.currentTarget.open"');
  expect(html).toContain('class="system-prompt-disclosure-state"');
  expect(html).toContain('aria-labelledby="routing-system-prompt-label"');
  expect(html).toContain('aria-labelledby="provider-system-prompt-label"');
  expect(html).not.toContain("saveDefaults()");
  expect(html).not.toContain('copy.saveDefaults');
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
  expect(keyManagementJavaScript).toContain("providerEditorSession");
  expect(keyManagementJavaScript).toContain("autosaveSelectedProvider");
  expect(keyManagementJavaScript).toContain("autosaveRoutingDefaults");
  expect(keyManagementJavaScript).toContain("enqueueProfileMutation");
  expect(keyManagementJavaScript).toContain("waitForProfileMutations");
  expect(keyManagementJavaScript).not.toContain("saveSelectedProviderKey");
  expect(keyManagementJavaScript).not.toContain("saveDefaults");
  expect(keyManagementJavaScript).toContain("requestAndApplyGeneratedSecret");
  expect(keyManagementJavaScript).toContain("settingsRequired");
  expect(keyManagementJavaScript).toContain("hasSavedProviderKey");
  expect(keyManagementJavaScript).not.toContain("providerInputs");
  expect(keyManagementJavaScript).not.toContain("revealedProviderID");
  expect(keyManagementJavaScript.match(/window\.setTimeout/g)).toHaveLength(1);
  expect(keyManagementJavaScript).toContain("NOTICE_AUTO_DISMISS_MILLISECONDS");

  const constantsResponse = await request.get(`${baseURL}/assets/llm-proxy/js/constants.js`);
  expect(constantsResponse.status()).toBe(httpOK);
  const constantsJavaScript = await constantsResponse.text();
  expect(constantsJavaScript).toContain("export const NOTICE_AUTO_DISMISS_MILLISECONDS = 10_000;");
  expect(constantsJavaScript).toContain("Provider settings saved");
  expect(constantsJavaScript).toContain('systemPromptHidden: "Hidden"');
  expect(constantsJavaScript).toContain('systemPromptExpanded: "Expanded"');
  expect(constantsJavaScript).not.toContain('saveProviderKey: "Save key"');
  expect(constantsJavaScript).not.toContain('updateProviderKey: "Update key"');
  expect(constantsJavaScript).not.toContain('saveDefaults: "Save defaults"');

  const stylesheetResponse = await request.get(`${baseURL}/assets/llm-proxy/styles.css`);
  expect(stylesheetResponse.status()).toBe(httpOK);
  const stylesheet = await stylesheetResponse.text();
  expect(stylesheet).toContain("#llm-proxy-header notification-region[slot=\"aux\"]");
  expect(stylesheet).toContain("order: -1;");
  expect(stylesheet).not.toContain("shadowRoot");
  expect(stylesheet).not.toContain('.settings-grid-form button[type="submit"]');
  expect(stylesheet).toContain(".system-prompt-disclosure[open] .system-prompt-summary::after");

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

test("public landing is keyboard navigable and responsive in Chromium", async ({ page }) => {
  await installAssetRoutes(page);
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto(baseURL);

  await expect(page.getByRole("heading", { level: 1 })).toHaveText(
    "One stable interface for the models your products depend on.",
  );
  await expect(page.getByRole("link", { name: "Log In" }).first()).toHaveAttribute("href", applicationPath);
  await expect(page.getByRole("region", { name: "Provider and model capability matrix" })).toBeVisible();
  await expect(page.getByRole("table")).toBeVisible();
  await expectCenteredValueStrip(page);
  const footer = page.locator("mpr-footer");
  await expect(footer).toHaveAttribute("size", "small");
  await expect(footer).toHaveAttribute("sticky", "false");
  await expect(footer.getByRole("contentinfo")).toBeVisible();
  await expect(footer.getByRole("link", { name: "Log In" })).toHaveAttribute("href", applicationPath);
  await expect(footer.getByRole("link", { name: "API", exact: true })).toHaveAttribute("href", apiDocumentationPath);
  await expect(footer.getByRole("link", { name: "OpenAPI", exact: true })).toHaveAttribute("href", openAPIPath);
  await expect(footer.getByRole("link", { name: "Resources" })).toHaveAttribute("href", resourcesPath);
  await expect(footer.getByRole("link", { name: "GitHub" })).toHaveAttribute(
    "href",
    "https://github.com/tyemirov/llm-proxy",
  );
  await expectCompactFooterGeometry(footer);

  await page.keyboard.press("Tab");
  await expect(page.getByRole("link", { name: "Skip to content" })).toBeFocused();

  await page.setViewportSize({ width: 390, height: 780 });
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  await expect(page.getByRole("link", { name: "Log In" }).first()).toBeVisible();
  await expect(page.getByRole("region", { name: "Provider and model capability matrix" })).toBeVisible();
  await expectCenteredValueStrip(page);
  await expectCompactFooterGeometry(footer);
});

test("site publishes the exact canonical OpenAPI artifact and its derived reference", async ({ request }) => {
  const canonicalSource = await readFile(canonicalOpenAPIFile, "utf8");
  const schemaResponse = await request.get(`${baseURL}${openAPIPath}`);
  expect(schemaResponse.status()).toBe(httpOK);
  expect(schemaResponse.headers()["content-type"]).toContain(mimeTypes[".yaml"]);
  expect(await schemaResponse.text()).toBe(canonicalSource);

  const documentationResponse = await request.get(`${baseURL}${apiDocumentationPath}`);
  expect(documentationResponse.status()).toBe(httpOK);
  expect(documentationResponse.headers()["content-type"]).toContain(mimeTypes[".html"]);
  const documentationHTML = await documentationResponse.text();
  const sourceDigest = createHash("sha256").update(canonicalSource).digest("hex");
  expect(canonicalSource).not.toContain("deleteManagementTenantSecret");
  expect(documentationHTML).toContain(`<link rel="canonical" href="https://llm-proxy.mprlab.com${apiDocumentationPath}">`);
  expect(documentationHTML).toContain(`data-openapi-source-sha256="${sourceDigest}"`);
  expect(documentationHTML).toContain("https://llm-proxy-api.mprlab.com");
  expect(documentationHTML).toContain('id="operation-postV2Messages"');
  expect(documentationHTML).not.toContain('id="operation-deleteManagementTenantSecret"');
  expect(documentationHTML).toContain("<code>reasoning_effort</code>");
  expect(documentationHTML).toContain(`href="${openAPIPath}"`);
  expect(documentationHTML.match(/<section class="api-operation"/g) || []).toHaveLength(20);
});

test("SEO resource pages are crawlable from the public site", async ({ request }) => {
  const hubResponse = await request.get(`${baseURL}${resourcesPath}`);
  expect(hubResponse.status()).toBe(httpOK);
  expect(hubResponse.headers()["content-type"]).toContain(mimeTypes[".html"]);
  const hubHTML = await hubResponse.text();
  expect(hubHTML).toContain("LLM Proxy resource hub");
  expect(hubHTML).toContain('<script defer src="/assets/llm-proxy/js/googleAnalytics.js"></script>');
  expect(hubHTML).toContain('<link rel="canonical" href="https://llm-proxy.mprlab.com/resources/">');
  expect(hubHTML).toContain('"@type":"CollectionPage"');
  expect(hubHTML).toContain(`href="${apiDocumentationPath}"`);
  expect(hubHTML).toContain(`href="${openAPIPath}"`);
  expect(hubHTML).toContain(`href="${representativeResourcePath}"`);
  expect(hubHTML).toContain(`href="${clientAuthenticationResourcePath}"`);
  const resourceLinks = hubHTML.match(/href="\/resources\/[^"]+\/"/g) || [];
  expect(new Set(resourceLinks).size).toBe(generatedResourcePageCount);

  const pageResponse = await request.get(`${baseURL}${representativeResourcePath}`);
  expect(pageResponse.status()).toBe(httpOK);
  expect(pageResponse.headers()["content-type"]).toContain(mimeTypes[".html"]);
  const pageHTML = await pageResponse.text();
  expect(pageHTML).toContain("<h1>Multi-provider LLM proxy for internal tools</h1>");
  expect(pageHTML).toContain('<script defer src="/assets/llm-proxy/js/googleAnalytics.js"></script>');
  expect(pageHTML).toContain('<link rel="canonical" href="https://llm-proxy.mprlab.com/resources/multi-provider-llm-proxy/">');
  expect(pageHTML).toContain('"@type":"FAQPage"');
  expect(pageHTML).toContain(`<a class="resource-button" href="${apiDocumentationPath}">Open API reference</a>`);
  expect(pageHTML).toContain('href="/resources/openai-claude-gemini-one-endpoint/"');
  expect(pageHTML).toContain(`"dateModified":"${seoContentModifiedDate}"`);
});

test("SEO client-authentication guide documents the credential and configuration boundaries", async ({ request }) => {
  const response = await request.get(`${baseURL}${clientAuthenticationResourcePath}`);
  expect(response.status()).toBe(httpOK);
  const pageHTML = await response.text();
  expect(pageHTML).toContain("<h1>Authenticate an LLM Proxy client with a tenant secret</h1>");
  expect(pageHTML).toContain(
    '<link rel="canonical" href="https://llm-proxy.mprlab.com/resources/llm-proxy-client-authentication/">',
  );
  expect(pageHTML).toContain(`"dateModified":"${seoClientDocumentationModifiedDate}"`);
  expect(pageHTML).toContain(`"datePublished":"${seoClientDocumentationModifiedDate}"`);
  expect(pageHTML).toContain("curl -X POST");
  expect(pageHTML).toContain("/v2?key=mysecret&amp;provider=deepseek");
  expect(pageHTML).toContain("no user-level or system-level YAML lookup");
  expect(pageHTML).toContain("LLM_PROXY_BASE_URL and LLM_PROXY_SECRET");
  expect(pageHTML).toContain("configured MPR UI and TAuth session");
  expect(pageHTML).toContain(`<a href="${repositoryUsageURL}">README</a>`);
  expect(pageHTML).toContain('href="/resources/tenant-secret-ai-gateway/"');
  expect(pageHTML).toContain('href="/resources/server-side-provider-api-keys/"');
  expect(pageHTML).toContain('href="/resources/canonical-v2-chat-messages-api/"');
  expect(pageHTML).toContain("<h2>Repository evidence</h2>");
  expect(pageHTML).toContain(`Verified ${seoClientDocumentationModifiedDate}`);
});

test("SEO client and security resources link to the canonical authentication guide", async ({ request }) => {
  const linkedResourcePaths = [
    "/resources/go-client-v2-only-llm-proxy/",
    "/resources/python-client-v2-only-llm-proxy/",
    "/resources/installable-llm-proxy-cli/",
    "/resources/server-side-provider-api-keys/",
    "/resources/tenant-secret-ai-gateway/",
    "/resources/copyable-llm-curl-examples/",
  ];
  for (const resourcePath of linkedResourcePaths) {
    const response = await request.get(`${baseURL}${resourcePath}`);
    expect(response.status()).toBe(httpOK);
    const pageHTML = await response.text();
    expect(pageHTML).toContain(`href="${clientAuthenticationResourcePath}"`);
    expect(pageHTML).toContain(`<a href="${repositoryUsageURL}">README</a>`);
  }
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

test("SEO usage resource documents account-wide and tenant-filtered intervals", async ({ request }) => {
  const response = await request.get(`${baseURL}/resources/managed-tenant-usage-dashboard/`);
  expect(response.status()).toBe(httpOK);
  const pageHTML = await response.text();
  expect(pageHTML).toContain(`"dateModified":"${seoUsageContentModifiedDate}"`);
  expect(pageHTML).toContain("Usage opens on All tenants and 30 days");
  expect(pageHTML).toContain("Usage tenant selector immediately before ALL");
  expect(pageHTML).toContain("GET /api/management/usage?interval=30d");
  expect(pageHTML).toContain(
    "GET /api/management/tenants/:tenant_id/usage?interval=30d",
  );
});

test("SEO management resources document required onboarding and secret-safe examples", async ({ request }) => {
  const resourceExpectations = [
    {
      slug: "self-service-llm-key-management",
      title: "Self-service LLM key management for internal teams",
      copy: "creates a missing client key after authentication, autosaves provider settings, and keeps Settings open",
      faqQuestion: "What lets a user leave Settings?",
      modifiedDate: seoCurrentContentModifiedDate,
    },
    {
      slug: "generated-secret-rotation",
      title: "Rotate generated LLM Proxy client keys with confidence",
      copy: "Request examples retain the &lt;generated-secret&gt; placeholder after creation.",
      faqQuestion: "Can the raw generated client key be retrieved later?",
      modifiedDate: seoClientDocumentationModifiedDate,
    },
    {
      slug: "copyable-llm-curl-examples",
      title: "Copyable LLM curl examples from current profile data",
      copy: "Examples always use &lt;generated-secret&gt;, including after automatic client-key creation.",
      faqQuestion: "Can copying an example expose the raw generated key?",
      modifiedDate: seoCurrentContentModifiedDate,
    },
  ];
  for (const resourceExpectation of resourceExpectations) {
    const response = await request.get(`${baseURL}/resources/${resourceExpectation.slug}/`);
    expect(response.status()).toBe(httpOK);
    const pageHTML = await response.text();
    expect(pageHTML).toContain(
      `<link rel="canonical" href="https://llm-proxy.mprlab.com/resources/${resourceExpectation.slug}/">`,
    );
    expect(pageHTML).toContain(`"dateModified":"${resourceExpectation.modifiedDate}"`);
    expect(pageHTML).toContain(`<title>${resourceExpectation.title}</title>`);
    expect(resourceExpectation.title.length).toBeGreaterThanOrEqual(50);
    expect(resourceExpectation.title.length).toBeLessThanOrEqual(60);
    expect(pageHTML).toContain(resourceExpectation.copy);
    expect(pageHTML).toContain("<strong>Quick verdict</strong>");
    expect(pageHTML).toContain("<h2>Repository evidence</h2>");
    expect(pageHTML).toContain(`Verified ${resourceExpectation.modifiedDate}`);
    expect(pageHTML).toContain('href="https://github.com/tyemirov" rel="author"');
    expect(pageHTML).toContain(`<summary>${resourceExpectation.faqQuestion}</summary>`);
    expect(pageHTML).not.toContain("Does this page claim provider performance or pricing advantages?");
    expect(pageHTML).not.toContain("Where should setup details come from?");
    const jsonLDBlocks = [...pageHTML.matchAll(/<script type="application\/ld\+json">([^<]+)<\/script>/g)];
    expect(jsonLDBlocks.length).toBeGreaterThan(0);
    for (const jsonLDBlock of jsonLDBlocks) {
      expect(() => JSON.parse(jsonLDBlock[1])).not.toThrow();
    }
  }
});

test("SEO sitemap and robots expose canonical resource URLs", async ({ request }) => {
  const sitemapResponse = await request.get(`${baseURL}${sitemapPath}`);
  expect(sitemapResponse.status()).toBe(httpOK);
  expect(sitemapResponse.headers()["content-type"]).toContain(mimeTypes[".xml"]);
  const sitemapXML = await sitemapResponse.text();
  const sitemapLocations = sitemapXML.match(/<loc>/g) || [];
  expect(sitemapLocations).toHaveLength(generatedResourcePageCount + 3);
  expect(sitemapXML).toContain("<loc>https://llm-proxy.mprlab.com/</loc>");
  expect(sitemapXML).toContain("<loc>https://llm-proxy.mprlab.com/resources/</loc>");
  expect(sitemapXML).toContain(`<loc>https://llm-proxy.mprlab.com${apiDocumentationPath}</loc>`);
  expect(sitemapXML).toContain(
    "<loc>https://llm-proxy.mprlab.com/resources/multi-provider-llm-proxy/</loc>",
  );
  const sitemapModificationDates = sitemapXML.match(/<lastmod>[^<]+<\/lastmod>/g) || [];
  expect(sitemapModificationDates).toHaveLength(generatedResourcePageCount + 3);
  expect(new Set(sitemapModificationDates)).toEqual(
    new Set([
      `<lastmod>${seoContentModifiedDate}</lastmod>`,
      `<lastmod>${seoCurrentContentModifiedDate}</lastmod>`,
      `<lastmod>${seoMigrationContentModifiedDate}</lastmod>`,
      `<lastmod>${seoUsageContentModifiedDate}</lastmod>`,
      `<lastmod>${seoClientDocumentationModifiedDate}</lastmod>`,
      `<lastmod>${landingModifiedDate}</lastmod>`,
    ]),
  );
  expect(sitemapXML).toContain(
    `<loc>https://llm-proxy.mprlab.com/resources/self-service-llm-key-management/</loc>\n    <lastmod>${seoCurrentContentModifiedDate}</lastmod>`,
  );
  expect(sitemapXML).toContain(
    `<loc>https://llm-proxy.mprlab.com/resources/managed-tenant-usage-dashboard/</loc>\n    <lastmod>${seoUsageContentModifiedDate}</lastmod>`,
  );
  expect(sitemapXML).toContain(
    `<loc>https://llm-proxy.mprlab.com/resources/llm-proxy-client-authentication/</loc>\n    <lastmod>${seoClientDocumentationModifiedDate}</lastmod>`,
  );
  expect(sitemapXML).toContain(
    `<loc>https://llm-proxy.mprlab.com/resources/generated-secret-rotation/</loc>\n    <lastmod>${seoClientDocumentationModifiedDate}</lastmod>`,
  );
  expect(sitemapXML).not.toContain("generated-secret-rotation-and-revocation");
  expect(sitemapXML).not.toContain("config-ui.yaml");
  expect(sitemapXML).not.toContain("llm-proxy-config.json");
  expect(sitemapXML).not.toContain(applicationPath);
  expect(sitemapXML).not.toContain(removedApplicationPath);

  const robotsResponse = await request.get(`${baseURL}${robotsPath}`);
  expect(robotsResponse.status()).toBe(httpOK);
  expect(robotsResponse.headers()["content-type"]).toContain(mimeTypes[".txt"]);
  const robotsText = await robotsResponse.text();
  expect(robotsText).toContain("User-agent: *");
  expect(robotsText).toContain("Sitemap: https://llm-proxy.mprlab.com/sitemap.xml");
});

test("usage defaults to all tenants while tenant management lives in Settings", async ({ page }) => {
  await installAssetRoutes(page);
  await installMultiTenantRoutes(page);

  await page.goto(`${baseURL}${applicationPath}`);

  const usageTenantSelector = page.getByRole("combobox", { name: "Usage tenant" });
  await expect(usageTenantSelector).toHaveValue("");
  await expect(usageTenantSelector.locator("option:checked")).toHaveText("All tenants");
  await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("44");
  await expect(page.locator("tenant-context-bar")).toHaveCount(0);
  await expect(page.getByText("Active tenant", { exact: true })).toHaveCount(0);

  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").getByText("Settings").click();
  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const settingsTenantSelector = settingsDialog.getByRole("combobox", { name: "Tenant" });
  await expect(settingsTenantSelector).toHaveValue("tenant_1");
  await expect(settingsDialog.getByRole("button", { name: "Create tenant" })).toBeVisible();
  await expect(settingsTenantSelector.locator("option:checked")).toHaveText("Default");
  await expect(settingsDialog.getByRole("group", { name: "Tenant access" })).not.toContainText("tenant_1");
});

test("the Tenant control in Settings and Usage tenant selection remain independent", async ({ page }) => {
  await installAssetRoutes(page);
  await installMultiTenantRoutes(page);

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").getByText("Settings").click();
  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const settingsTenantSelector = settingsDialog.getByRole("combobox", { name: "Tenant" });
  await settingsTenantSelector.selectOption("tenant_2");
  await expect(settingsTenantSelector).toHaveValue("tenant_2");
  await expect(settingsTenantSelector.locator("option:checked")).toHaveText("Research");
  await settingsDialog.getByRole("button", { name: "Close" }).click();

  const usageTenantSelector = page.getByRole("combobox", { name: "Usage tenant" });
  await expect(usageTenantSelector).toHaveValue("");
  await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("44");
  await usageTenantSelector.selectOption("tenant_1");
  await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("37");

  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").getByText("Settings").click();
  await expect(settingsDialog.getByRole("combobox", { name: "Tenant" })).toHaveValue("tenant_2");
  await expect(page).toHaveURL(`${baseURL}${applicationPath}`);
});

test("obsolete tenant query parameters do not choose Settings or Usage state", async ({ page }) => {
  await installAssetRoutes(page);
  await installMultiTenantRoutes(page);

  await page.goto(`${baseURL}${applicationPath}?tenant=tenant_2`);

  await expect(page.getByRole("combobox", { name: "Usage tenant" })).toHaveValue("");
  await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("44");
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").getByText("Settings").click();
  await expect(page.getByRole("dialog", { name: "Settings" }).getByRole("combobox", { name: "Tenant" })).toHaveValue("tenant_1");
  await expect(page.getByRole("heading", { name: "Unable to load key workspace" })).toHaveCount(0);
});

test("tenant lifecycle is keyboard accessible, responsive, and guards the final tenant", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 780 });
  await installAssetRoutes(page);
  const managementState = await installMultiTenantRoutes(page, {
    profiles: [managementTenantProfile("tenant_1", "Default")],
  });

  await page.goto(`${baseURL}${applicationPath}`);

  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").getByText("Settings").click();
  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const tenantAccess = settingsDialog.getByRole("group", { name: "Tenant access" });
  const settingsTenantSelector = tenantAccess.getByRole("combobox", { name: "Tenant" });
  const renameTenantButton = tenantAccess.getByRole("button", { name: "Rename" });
  const deleteTenantButton = tenantAccess.getByRole("button", { name: "Delete tenant" });
  await expect(settingsTenantSelector.locator("option:checked")).toHaveText("Default");
  await expect(tenantAccess).not.toContainText("tenant_1");
  await expect(deleteTenantButton).toBeDisabled();
  await expect(deleteTenantButton).toHaveAttribute("aria-describedby", "final-tenant-deletion");
  await expect(deleteTenantButton).toHaveAttribute("title", "Your final tenant cannot be deleted.");
  await renameTenantButton.click();
  const initialRenameDialog = page.getByRole("dialog", { name: "Rename tenant" });
  const initialTenantName = initialRenameDialog.getByRole("textbox", { name: "Tenant name" });
  await expect(initialTenantName).toBeFocused();
  await initialRenameDialog.getByRole("button", { name: "Cancel" }).click();
  await expect(initialRenameDialog).toBeHidden();
  await expect(renameTenantButton).toBeFocused();

  const createTenantButton = settingsDialog.getByRole("button", { name: "Create tenant" });
  await createTenantButton.focus();
  await page.keyboard.press("Enter");
  const createDialog = page.getByRole("dialog", { name: "Create tenant" });
  const createName = createDialog.getByRole("textbox", { name: "Tenant name" });
  await expect(createName).toBeFocused();
  await createDialog.getByRole("button", { name: "Create", exact: true }).click();
  await expect(createDialog.getByRole("alert")).toHaveText("Enter a tenant name with 1–80 visible characters.");
  await createName.fill("default");
  await createDialog.getByRole("button", { name: "Create", exact: true }).click();
  await expect(createDialog.getByRole("alert")).toHaveText("A tenant with that name already exists.");
  await createName.fill("Research");
  await createDialog.getByRole("button", { name: "Create", exact: true }).click();

  await expect(page).toHaveURL(`${baseURL}${applicationPath}`);
  await expect(settingsDialog.getByRole("combobox", { name: "Tenant" })).toHaveValue("tenant_2");
  await expect(settingsDialog).toBeVisible();
  await renameTenantButton.click();
  const renameDialog = page.getByRole("dialog", { name: "Rename tenant" });
  const tenantName = renameDialog.getByRole("textbox", { name: "Tenant name" });
  await tenantName.fill("Default");
  await renameDialog.getByRole("button", { name: "Save name" }).click();
  await expect(renameDialog.getByRole("alert")).toHaveText("A tenant with that name already exists.");
  await expect(renameDialog).toBeVisible();
  await tenantName.fill("Research Lab");
  await renameDialog.getByRole("button", { name: "Save name" }).click();
  await expect(renameTenantButton).toBeFocused();
  await expect(renameDialog).toBeHidden();
  await expect(settingsDialog.getByRole("combobox", { name: "Tenant" }).locator("option:checked")).toHaveText("Research Lab");

  await deleteTenantButton.click();
  const deleteDialog = page.getByRole("alertdialog", { name: "Delete “Research Lab”?" });
  await expect(deleteDialog.getByText("Research Lab", { exact: true })).toBeVisible();
  await expect(deleteDialog).toContainText("This permanently deletes the tenant");
  await deleteDialog.getByRole("button", { name: "Cancel" }).click();
  await expect(deleteDialog).toBeHidden();
  await deleteTenantButton.click();
  await deleteDialog.getByRole("button", { name: "Delete", exact: true }).click();

  await expect(page).toHaveURL(`${baseURL}${applicationPath}`);
  await expect(settingsDialog.getByRole("combobox", { name: "Tenant" })).toHaveValue("tenant_1");
  await expect(page.getByRole("combobox", { name: "Usage tenant" })).toHaveValue("");
  expect(managementState.order).toEqual(["tenant_1"]);
  const tenantAccessBox = await tenantAccess.boundingBox();
  if (!tenantAccessBox) {
    throw new Error("tenant_access_missing");
  }
  expect(tenantAccessBox.x).toBeGreaterThanOrEqual(0);
  expect(tenantAccessBox.x + tenantAccessBox.width).toBeLessThanOrEqual(390);
});

test("tenant switching requires discard and clears one-time and revealed credentials", async ({ page }) => {
  await installAssetRoutes(page);
  await installMultiTenantRoutes(page);
  let releaseProviderSave;
  const providerSaveReleased = new Promise((resolve) => {
    releaseProviderSave = resolve;
  });
  let providerSaveStarted;
  const providerSaveRequested = new Promise((resolve) => {
    providerSaveStarted = resolve;
  });
  await page.route(`${baseURL}/api/management/tenants/tenant_1/provider-keys/openai`, async (route) => {
    if (route.request().method() !== "PUT") {
      await route.fallback();
      return;
    }
    providerSaveStarted();
    await providerSaveReleased;
    await route.fallback();
  });

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").getByText("Settings").click();
  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const clientAccess = settingsDialog.getByRole("group", { name: "Tenant access" });
  const providerEditor = settingsDialog.locator("provider-editor");

  await clientAccess.getByRole("button", { name: "Replace key" }).click();
  await page.getByRole("alertdialog", { name: "Replace client key?" }).getByRole("button", { name: "Replace key" }).click();
  await clientAccess.getByRole("button", { name: "Show key", exact: true }).click();
  await expect(clientAccess.getByRole("textbox", { name: "Key", exact: true })).toHaveValue("llmp_tenant_1_generated");
  await providerEditor.getByRole("button", { name: "Show key", exact: true }).click();
  await expect(providerEditor.getByRole("textbox", { name: "OpenAI API key" })).toHaveValue("sk-tenant_1-openai");
  await providerEditor.locator("summary.system-prompt-summary").click();
  await providerEditor.getByRole("textbox", { name: "System prompt" }).fill("Unsaved tenant one prompt");
  await page.keyboard.press("Tab");
  await providerSaveRequested;

  const settingsTenantSelector = settingsDialog.getByRole("combobox", { name: "Tenant" });
  await settingsTenantSelector.selectOption("tenant_2");
  const discardDialog = page.getByRole("alertdialog", { name: "Discard unsaved changes?" });
  await expect(discardDialog).toBeVisible();
  await discardDialog.getByRole("button", { name: "Stay" }).click();
  await expect(settingsTenantSelector).toHaveValue("tenant_1");
  await expect(providerEditor.getByRole("textbox", { name: "System prompt" })).toHaveValue("Unsaved tenant one prompt");

  await settingsTenantSelector.selectOption("tenant_2");
  await discardDialog.getByRole("button", { name: "Discard and switch" }).click();
  releaseProviderSave();
  await expect(settingsTenantSelector).toHaveValue("tenant_2");
  await expect(page.getByRole("combobox", { name: "Usage tenant" })).toHaveValue("");
  await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("44");
  await expect(page.locator("body")).not.toContainText("llmp_tenant_1_generated");
  await expect(page.locator("body")).not.toContainText("sk-tenant_1-openai");
  expect(await browserStorageContains(page, "llmp_tenant_1_generated")).toBe(false);
  expect(await browserStorageContains(page, "sk-tenant_1-openai")).toBe(false);
});

test("concurrent tabs keep independent Settings and Usage tenant state", async ({ context, page }) => {
  const secondPage = await context.newPage();
  await installAssetRoutes(page);
  await installAssetRoutes(secondPage);
  await installMultiTenantRoutes(page);
  await installMultiTenantRoutes(secondPage);

  await Promise.all([
    page.goto(`${baseURL}${applicationPath}`),
    secondPage.goto(`${baseURL}${applicationPath}`),
  ]);

  await page.getByRole("combobox", { name: "Usage tenant" }).selectOption("tenant_1");
  await secondPage.getByRole("combobox", { name: "Usage tenant" }).selectOption("tenant_2");
  await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("37");
  await expect(secondPage.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("7");

  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").getByText("Settings").click();
  await page.getByRole("dialog", { name: "Settings" }).getByRole("combobox", { name: "Tenant" }).selectOption("tenant_2");
  await secondPage.getByTestId("avatar-menu").click();
  await secondPage.getByTestId("avatar-menu-item").getByText("Settings").click();
  await expect(secondPage.getByRole("dialog", { name: "Settings" }).getByRole("combobox", { name: "Tenant" })).toHaveValue("tenant_1");
  await expect(page.getByRole("combobox", { name: "Usage tenant" })).toHaveValue("tenant_1");
  await expect(secondPage.getByRole("combobox", { name: "Usage tenant" })).toHaveValue("tenant_2");
});

test("late tenant usage cannot overwrite a newer Usage tenant selection", async ({ page }) => {
  await installAssetRoutes(page);
  await installMultiTenantRoutes(page);
  let releaseFirstUsage;
  const firstUsageReleased = new Promise((resolve) => {
    releaseFirstUsage = resolve;
  });
  let firstUsageStarted;
  const firstUsageRequested = new Promise((resolve) => {
    firstUsageStarted = resolve;
  });
  await page.route(`${baseURL}/api/management/tenants/tenant_1/usage?interval=*`, async (route) => {
    firstUsageStarted();
    await firstUsageReleased;
    await route.fallback();
  });

  await page.goto(`${baseURL}${applicationPath}`);
  await page.locator("llm-proxy-key-management").evaluate((applicationElement) => {
    const alpineRuntime = /** @type {typeof globalThis & { Alpine?: { $data: (element: Element) => any } }} */ (globalThis);
    const applicationState = alpineRuntime.Alpine?.$data(applicationElement);
    if (!applicationState) {
      throw new Error("usage_tenant_state_missing");
    }
    void applicationState.handleUsageTenantSelection({ target: { value: "tenant_1" } });
  });
  await firstUsageRequested;
  await page.locator("llm-proxy-key-management").evaluate((applicationElement) => {
    const alpineRuntime = /** @type {typeof globalThis & { Alpine?: { $data: (element: Element) => any } }} */ (globalThis);
    const applicationState = alpineRuntime.Alpine?.$data(applicationElement);
    if (!applicationState) {
      throw new Error("usage_tenant_state_missing");
    }
    void applicationState.handleUsageTenantSelection({ target: { value: "tenant_2" } });
  });
  releaseFirstUsage();

  await expect(page).toHaveURL(`${baseURL}${applicationPath}`);
  await expect(page.getByRole("combobox", { name: "Usage tenant" })).toHaveValue("tenant_2");
  await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("7");
  await page.waitForTimeout(50);
  await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("7");
});

test("late tenant lifecycle responses cannot select or overwrite another tenant", async ({ page }) => {
  await installAssetRoutes(page);
  await installMultiTenantRoutes(page);
  const delayedCreateProfile = managementTenantProfile("tenant_3", "Late Create");
  let releaseCreate;
  const createReleased = new Promise((resolve) => {
    releaseCreate = resolve;
  });
  let createStarted;
  const createRequested = new Promise((resolve) => {
    createStarted = resolve;
  });
  await page.route(`${baseURL}/api/management/tenants`, async (route) => {
    createStarted();
    await createReleased;
    await route.fulfill({ status: 201, json: delayedCreateProfile }).catch(() => {});
  });

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").getByText("Settings").click();
  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const settingsTenantSelector = settingsDialog.getByRole("combobox", { name: "Tenant" });
  await settingsDialog.getByRole("button", { name: "Create tenant" }).click();
  await page.getByRole("dialog", { name: "Create tenant" }).getByRole("textbox", { name: "Tenant name" }).fill("Late Create");
  await page.getByRole("dialog", { name: "Create tenant" }).getByRole("button", { name: "Create", exact: true }).click();
  await createRequested;
  await page.locator("llm-proxy-key-management").evaluate((applicationElement) => {
    const alpineRuntime = /** @type {typeof globalThis & { Alpine?: { $data: (element: Element) => any } }} */ (globalThis);
    const applicationState = alpineRuntime.Alpine?.$data(applicationElement);
    if (!applicationState) {
      throw new Error("settings_tenant_state_missing");
    }
    void applicationState.requestSettingsTenantSwitch("tenant_2");
  });
  releaseCreate();
  await expect(settingsTenantSelector).toHaveValue("tenant_2");
  await expect(settingsTenantSelector.locator("option")).toHaveCount(2);
  await expect(settingsDialog).not.toContainText("Late Create");

  let releaseRename;
  const renameReleased = new Promise((resolve) => {
    releaseRename = resolve;
  });
  let renameStarted;
  const renameRequested = new Promise((resolve) => {
    renameStarted = resolve;
  });
  await page.route(`${baseURL}/api/management/tenants/tenant_2`, async (route) => {
    if (route.request().method() !== "PUT") {
      await route.fallback();
      return;
    }
    renameStarted();
    await renameReleased;
    const renamedProfile = managementTenantProfile("tenant_2", "Late Rename");
    await route.fulfill({ json: renamedProfile }).catch(() => {});
  });
  const tenantAccess = settingsDialog.getByRole("group", { name: "Tenant access" });
  await tenantAccess.getByRole("button", { name: "Rename" }).click();
  const tenantNameEditor = page.getByRole("dialog", { name: "Rename tenant" });
  await tenantNameEditor.getByRole("textbox", { name: "Tenant name" }).fill("Late Rename");
  await tenantNameEditor.getByRole("button", { name: "Save name" }).click();
  await renameRequested;
  await page.keyboard.press("Escape");
  await expect(tenantNameEditor).toBeVisible();
  await page.locator("llm-proxy-key-management").evaluate((applicationElement) => {
    const alpineRuntime = /** @type {typeof globalThis & { Alpine?: { $data: (element: Element) => any } }} */ (globalThis);
    const applicationState = alpineRuntime.Alpine?.$data(applicationElement);
    if (!applicationState) {
      throw new Error("settings_tenant_state_missing");
    }
    void applicationState.requestSettingsTenantSwitch("tenant_1");
  });
  await page.getByRole("alertdialog", { name: "Discard unsaved changes?" }).getByRole("button", { name: "Discard and switch" }).click();
  releaseRename();
  await expect(settingsTenantSelector).toHaveValue("tenant_1");
  await expect(settingsTenantSelector.locator('option[value="tenant_2"]')).toHaveText("Research");
  await expect(page.getByRole("combobox", { name: "Usage tenant" })).toHaveValue("");
  await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("44");
});

test("dashboard shows usage and settings opens from avatar menu before sign out", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page);

  await page.goto(`${baseURL}${applicationPath}`);

  await expect(page.getByRole("heading", { name: "Usage overview" })).toBeVisible();
  await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("37");
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
  await expect(settingsDialog.locator(".settings-header .eyebrow")).toHaveText("Settings");
  await expect(settingsDialog.locator(".settings-header h2")).toHaveCount(0);
  const closeSettingsButton = settingsDialog.getByRole("button", { name: "Close" });
  await expect(closeSettingsButton).toHaveText("");
  await expect(closeSettingsButton.locator("svg.close-icon path")).toHaveCount(2);
  await expect(settingsDialog.getByRole("heading", { name: "Client access" })).toHaveCount(0);
  const tenantAccessRow = settingsDialog.getByRole("group", { name: "Tenant access" });
  await expect(settingsDialog.locator("settings-body > tenant-access-row")).toHaveCount(1);
  await expect(settingsDialog.locator("settings-section tenant-access-row")).toHaveCount(0);
  await expect(tenantAccessRow.locator("client-access-tenant")).toHaveCount(0);
  await expect(tenantAccessRow.getByRole("combobox", { name: "Tenant" }).locator("option:checked")).toHaveText("Default");
  await expect(tenantAccessRow).toContainText("Saved; replace to reveal a new key.");
  const replaceKeyButton = tenantAccessRow.getByRole("button", { name: "Replace key" });
  await expect(replaceKeyButton.locator(".material-symbols-outlined")).toHaveText("key");
  await expect(replaceKeyButton.locator(".tenant-access-action-label")).toHaveText("Replace key");
  await expect(replaceKeyButton.locator("svg")).toHaveCount(0);
  await expect(tenantAccessRow.getByRole("button", { name: "Revoke key" })).toHaveCount(0);
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
  await expect(settingsDialog.getByRole("heading", { name: "Providers", exact: true })).toBeVisible();
  await expect(settingsDialog.getByRole("heading", { name: "Provider settings" })).toHaveCount(0);
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
  const providerVisibilityButton = providerEditor.getByRole("button", { name: "Show key" });
  await expect(providerVisibilityButton).toHaveAttribute("aria-pressed", "false");
  const providerRemovalButton = providerEditor.getByRole("button", { name: "Remove provider key and settings" });
  await expect(providerRemovalButton).toBeVisible();
  await expect(providerRemovalButton.locator(".material-symbols-outlined")).toHaveText("delete");
  await expect(providerModelSelector).toHaveValue("gpt-4.1");
  await expect(providerEditor.locator("#provider-system-prompt-input")).toHaveValue("Use concise answers.");

  const providerControlBoxes = await Promise.all(
    [providerSelector, providerKeyInput, providerVisibilityButton, providerRemovalButton, providerModelSelector].map((control) =>
      control.boundingBox(),
    ),
  );
  const [providerSelectorBox, providerKeyBox, providerVisibilityBox, providerRemovalBox, providerModelBox] = providerControlBoxes;
  if (!providerSelectorBox || !providerKeyBox || !providerVisibilityBox || !providerRemovalBox || !providerModelBox) {
    throw new Error("desktop_provider_controls_missing");
  }
  for (const controlBox of providerControlBoxes) {
    expect(controlBox.height).toBe(30);
    expect(controlBox.y).toBe(providerSelectorBox.y);
  }
  expect(providerSelectorBox.x + providerSelectorBox.width).toBeLessThanOrEqual(providerKeyBox.x);
  expect(providerKeyBox.x + providerKeyBox.width).toBeLessThanOrEqual(providerVisibilityBox.x);
  expect(providerVisibilityBox.x + providerVisibilityBox.width).toBeLessThanOrEqual(providerRemovalBox.x);
  expect(providerRemovalBox.x + providerRemovalBox.width).toBeLessThanOrEqual(providerModelBox.x);

  await providerSelector.selectOption("deepseek");
  await expect(providerEditor.getByRole("textbox", { name: "DeepSeek API key" })).toHaveValue("****5678");
  await expect(providerEditor.getByRole("combobox", { name: "Provider default model" })).toHaveValue("deepseek-chat");
  await expect(providerEditor.locator("#provider-system-prompt-input")).toHaveValue("");
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

test("system prompt editors stay hidden until their labels expand them and reset with context", async ({ page }) => {
  await installAssetRoutes(page);
  await installMultiTenantRoutes(page);

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").getByText("Settings").click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const defaultsForm = settingsDialog.locator(".settings-grid-form");
  const providerEditor = settingsDialog.locator("provider-editor");
  const routingDisclosure = defaultsForm.locator("details.system-prompt-disclosure");
  const providerDisclosure = providerEditor.locator("details.system-prompt-disclosure");
  const routingSummary = routingDisclosure.locator("summary.system-prompt-summary");
  const providerSummary = providerDisclosure.locator("summary.system-prompt-summary");
  const routingPrompt = defaultsForm.locator("#routing-system-prompt-input");
  const providerPrompt = providerEditor.locator("#provider-system-prompt-input");

  await expect(routingPrompt).toBeHidden();
  await expect(providerPrompt).toBeHidden();
  await expect(routingDisclosure.locator(".system-prompt-disclosure-state")).toHaveText("Hidden");
  await expect(providerDisclosure.locator(".system-prompt-disclosure-state")).toHaveText("Hidden");

  await routingSummary.click();
  await expect(routingPrompt).toBeVisible();
  await expect(routingDisclosure.locator(".system-prompt-disclosure-state")).toHaveText("Expanded");

  await providerSummary.focus();
  await page.keyboard.press("Enter");
  await expect(providerPrompt).toBeVisible();
  await expect(providerDisclosure.locator(".system-prompt-disclosure-state")).toHaveText("Expanded");

  await providerEditor.getByRole("combobox", { name: "Provider", exact: true }).selectOption("deepseek");
  await expect(providerPrompt).toBeHidden();
  await expect(providerDisclosure.locator(".system-prompt-disclosure-state")).toHaveText("Hidden");

  await providerSummary.click();
  await expect(providerPrompt).toBeVisible();
  const settingsTenantSelector = settingsDialog.getByRole("combobox", { name: "Tenant" });
  await settingsTenantSelector.selectOption("tenant_2");
  await expect(settingsTenantSelector).toHaveValue("tenant_2");
  await expect(routingPrompt).toBeHidden();
  await expect(providerPrompt).toBeHidden();
  await expect(routingDisclosure.locator(".system-prompt-disclosure-state")).toHaveText("Hidden");
  await expect(providerDisclosure.locator(".system-prompt-disclosure-state")).toHaveText("Hidden");

  await routingSummary.click();
  await providerSummary.click();
  await settingsDialog.getByRole("button", { name: "Close" }).click();
  await expect(settingsDialog).toBeHidden();
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").getByText("Settings").click();
  await expect(routingPrompt).toBeHidden();
  await expect(providerPrompt).toBeHidden();
});

test("usage intervals load every dashboard surface, remain active on refresh, and fit mobile", async ({ page }) => {
  const requestedIntervals = [];
  page.on("request", (request) => {
    const requestURL = new URL(request.url());
    if (requestURL.pathname === "/api/management/usage") {
      requestedIntervals.push(requestURL.searchParams.get("interval"));
    }
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page);

  await page.goto(`${baseURL}${applicationPath}`);

  const intervalGroup = page.getByRole("group", { name: "Usage interval" });
  const intervalButtons = intervalGroup.getByRole("button");
  await expect(intervalButtons).toHaveCount(usageIntervals.length);
  await expect(intervalButtons).toHaveText(usageIntervals.map((interval) => interval.label));
  const activeIntervalButton = intervalGroup.getByRole("button", { name: "30 days" });
  await expect(activeIntervalButton).toHaveAttribute("aria-pressed", "true");
  const activeIntervalStyle = async () =>
    activeIntervalButton.evaluate((button) => {
      const style = getComputedStyle(button);
      return {
        backgroundColor: style.backgroundColor,
        borderColor: style.borderColor,
        color: style.color,
      };
    });
  const expectedActiveIntervalStyle = {
    backgroundColor: "rgba(93, 147, 255, 0.14)",
    borderColor: "rgb(93, 147, 255)",
    color: "rgb(93, 147, 255)",
  };
  expect(await activeIntervalStyle()).toEqual(expectedActiveIntervalStyle);
  await activeIntervalButton.hover();
  expect(await activeIntervalStyle()).toEqual(expectedActiveIntervalStyle);
  expect(requestedIntervals).toEqual(["30d"]);

  for (const interval of usageIntervals) {
    await intervalGroup.getByRole("button", { name: interval.label, exact: true }).click();
    await expect(intervalGroup.getByRole("button", { name: interval.label, exact: true })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText(
      interval.requests.toLocaleString("en-US"),
    );
    await expect(page.locator("usage-card").filter({ hasText: "Tokens" }).locator("strong")).toHaveText(
      interval.totalTokens.toLocaleString("en-US"),
    );
    await expect(page.locator("usage-card").filter({ hasText: "Providers" }).locator("strong")).toHaveText(
      String(interval.providerCount),
    );
    await expect(page.locator("usage-chart-panel").first().locator("polyline")).toHaveAttribute("points", /,/);
    await expect(page.locator("usage-breakdown").first()).toContainText(
      interval.id === "30d" ? "openai" : `provider-${interval.id}`,
    );
    await expect(page.locator("usage-breakdown").nth(1)).toContainText(
      interval.id === "30d" ? "gpt-4.1" : `model-${interval.id}`,
    );
    await expect(page.getByRole("button", { name: /failed request/ })).toHaveCount(interval.id === "30d" ? 1 : 0);
  }

  const selectedInterval = usageIntervals.at(-1);
  if (!selectedInterval) {
    throw new Error("usage_interval_fixture_missing");
  }
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  expect(requestedIntervals.at(-1)).toBe(selectedInterval.id);
  await expect(intervalGroup.getByRole("button", { name: selectedInterval.label, exact: true })).toHaveAttribute(
    "aria-pressed",
    "true",
  );

  await page.setViewportSize({ width: 390, height: 780 });
  await expect(intervalGroup).toBeVisible();
  const intervalGroupBox = await intervalGroup.boundingBox();
  if (!intervalGroupBox) {
    throw new Error("usage_interval_group_missing");
  }
  expect(intervalGroupBox.x).toBeGreaterThanOrEqual(0);
  expect(intervalGroupBox.x + intervalGroupBox.width).toBeLessThanOrEqual(390);
  for (const intervalButton of await intervalButtons.all()) {
    await intervalButton.focus();
    await expect(intervalButton).toBeFocused();
  }
});

test("usage interval loading blocks controls, ignores stale responses, and clears failed selections", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page);
  await page.goto(`${baseURL}${applicationPath}`);
  await page.unroute(usageRequestPattern());
  /** @type {() => void} */
  let releaseSevenDayResponse = () => {};
  const sevenDayResponseGate = new Promise((resolve) => {
    releaseSevenDayResponse = () => resolve(undefined);
  });
  await page.route(usageRequestPattern(), async (route) => {
    const interval = new URL(route.request().url()).searchParams.get("interval") || "";
    if (interval === "7d") {
      await sevenDayResponseGate;
    }
    await route.fulfill({ status: httpOK, json: managementUsage(interval) });
  });

  const intervalGroup = page.getByRole("group", { name: "Usage interval" });
  const sevenDayButton = intervalGroup.getByRole("button", { name: "7 days" });
  try {
    await sevenDayButton.click();
    for (const intervalButton of await intervalGroup.getByRole("button").all()) {
      await expect(intervalButton).toBeDisabled();
    }
    await expect(page.getByRole("button", { name: "Refresh", exact: true })).toBeDisabled();
    await expect(sevenDayButton).toHaveAttribute("aria-pressed", "true");
    await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("0");
    await expect(page.locator("usage-chart-panel").first()).toContainText("No usage recorded");
    await page.locator("llm-proxy-key-management").evaluate((applicationElement) => {
      const alpineRuntime = /** @type {typeof globalThis & { Alpine?: { $data: (element: Element) => any } }} */ (globalThis);
      const applicationState = alpineRuntime.Alpine?.$data(applicationElement);
      if (!applicationState) {
        throw new Error("usage_interval_state_missing");
      }
      void applicationState.selectUsageInterval("1d");
    });
    await expect(intervalGroup.getByRole("button", { name: "1 day" })).toHaveAttribute("aria-pressed", "true");
    await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("1");
  } finally {
    releaseSevenDayResponse();
  }
  await page.waitForLoadState("networkidle");
  await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("1");

  await page.unroute(usageRequestPattern());
  await page.route(usageRequestPattern(), async (route) => {
    await route.fulfill({ status: httpInternalServerError, json: { error: "usage_failed" } });
  });
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(page.getByText("Request failed")).toBeVisible();
  await expect(intervalGroup.getByRole("button", { name: "1 day" })).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("0");
  await expect(page.locator("usage-chart-panel").first()).toContainText("No usage recorded");
});

test("failed-request details expose 10 of 22 requests as safe, focus-managed metadata on desktop and mobile", async ({ page }) => {
  const usage = managementUsage("30d", {
    requests: 22,
    successful_requests: 12,
    failed_requests: 10,
    text_requests: 20,
    dictation_requests: 2,
  });
  usage.status_codes = [
    { status_code: 200, requests: 12 },
    { status_code: 400, requests: 3 },
    { status_code: 429, requests: 2 },
    { status_code: 499, requests: 1 },
    { status_code: 502, requests: 4 },
  ];
  await installAssetRoutes(page);
  await installManagementRoutes(page);
  await installUsageResponse(page, httpOK, usage);
  await installUsageFailuresResponse(page, managementUsageFailures("30d", 10));

  await page.goto(`${baseURL}${applicationPath}`);

  const successRateCard = page.locator("usage-card").filter({ hasText: "Success rate" });
  await expect(successRateCard.locator("strong")).toHaveText("55%");
  const failureAction = successRateCard.getByRole("button", { name: "10 failed requests" });
  await expect(failureAction).toBeVisible();
  await failureAction.click();

  const dialog = page.getByRole("dialog", { name: "Failed request details" });
  const closeButton = dialog.getByRole("button", { name: "Close failed request details" });
  await expect(dialog).toBeVisible();
  await expect(dialog).toHaveAttribute("aria-modal", "true");
  await expect(dialog).toHaveAttribute("aria-busy", "false");
  await expect(closeButton).toBeFocused();
  await expect(dialog.getByRole("heading", { name: "Status breakdown" })).toBeVisible();
  await expect(dialog.locator("usage-failure-status")).toHaveCount(4);
  await expect(dialog.locator("usage-failure-status").nth(0)).toContainText("400");
  await expect(dialog.locator("usage-failure-status").nth(0)).toContainText("Bad request");
  await expect(dialog.locator("usage-failure-status").nth(0)).toContainText("3");
  await expect(dialog.locator("usage-failure-row")).toHaveCount(10);
  await expect(dialog.locator("usage-failure-row").first()).toContainText("Default · tenant_1");
  await expect(dialog.locator("usage-failure-row").first()).toContainText("V2");
  await expect(dialog.locator("usage-failure-row").first()).toContainText("502 Upstream error");
  await expect(dialog.locator("usage-failure-row").first()).toContainText("Upstream error");
  await expect(dialog.locator("usage-failure-row").first()).toContainText("245 ms");
  await expect(dialog.locator("usage-failure-status").filter({ hasText: "499" })).toContainText("Client closed request");
  await expect(dialog.locator("usage-failure-row").filter({ hasText: "499 Client closed request" })).toContainText("Request timeout");
  await expect(dialog).not.toContainText("raw-provider-body");
  await expect(dialog).not.toContainText("sk-never-render");
  await expect(dialog).not.toContainText("private prompt");
  const browserStorageText = await page.evaluate(() => {
    const storageValues = [];
    for (const storage of [localStorage, sessionStorage]) {
      for (let index = 0; index < storage.length; index += 1) {
        const key = storage.key(index);
        storageValues.push(key || "", key ? storage.getItem(key) || "" : "");
      }
    }
    return storageValues.join("\n");
  });
  expect(browserStorageText).not.toContain("raw-provider-body");
  expect(browserStorageText).not.toContain("sk-never-render");
  expect(browserStorageText).not.toContain("private prompt");

  await closeButton.press("Tab");
  await expect(closeButton).toBeFocused();

  await page.setViewportSize({ width: 390, height: 780 });
  const dialogBox = await dialog.boundingBox();
  if (!dialogBox) {
    throw new Error("usage_failures_dialog_missing");
  }
  expect(dialogBox.x).toBeGreaterThanOrEqual(0);
  expect(dialogBox.y).toBeGreaterThanOrEqual(0);
  expect(dialogBox.x + dialogBox.width).toBeLessThanOrEqual(390);
  expect(dialogBox.y + dialogBox.height).toBeLessThanOrEqual(780);

  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  await expect(failureAction).toBeFocused();
});

test("failed-request pagination preserves metrics across loading and retryable errors", async ({ page }) => {
  const usage = managementUsage("30d", {
    requests: 40,
    successful_requests: 14,
    failed_requests: 26,
    text_requests: 38,
    dictation_requests: 2,
  });
  usage.status_codes = [
    { status_code: 200, requests: 14 },
    { status_code: 502, requests: 26 },
  ];
  await installAssetRoutes(page);
  await installManagementRoutes(page);
  await installUsageResponse(page, httpOK, usage);

  /** @type {() => void} */
  let releaseFirstPage = () => {};
  const firstPageGate = new Promise((resolve) => {
    releaseFirstPage = () => resolve(undefined);
  });
  let failFirstPage = false;
  await page.route(usageFailuresRequestPattern(), async (route) => {
    const requestURL = new URL(route.request().url());
    if (failFirstPage) {
      await route.fulfill({ status: httpInternalServerError, body: "raw-provider-body sk-never-render" });
      return;
    }
    if (!requestURL.searchParams.has("cursor")) {
      await firstPageGate;
      await route.fulfill({ json: managementUsageFailures("30d", 25, "page-2") });
      return;
    }
    expect(requestURL.searchParams.get("cursor")).toBe("page-2");
    await route.fulfill({ json: managementUsageFailures("30d", 1, "", 25) });
  });

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByRole("button", { name: "26 failed requests" }).click();
  const dialog = page.getByRole("dialog", { name: "Failed request details" });
  await expect(dialog).toHaveAttribute("aria-busy", "true");
  await expect(dialog.getByText("Loading failed requests")).toBeVisible();
  await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("40");

  releaseFirstPage();
  await expect(dialog.locator("usage-failure-row")).toHaveCount(25);
  const loadMore = dialog.getByRole("button", { name: "Load more" });
  await expect(loadMore).toBeVisible();
  await loadMore.click();
  await expect(dialog.locator("usage-failure-row")).toHaveCount(26);
  await expect(loadMore).toBeHidden();

  await dialog.getByRole("button", { name: "Close failed request details" }).click();
  failFirstPage = true;
  await page.getByRole("button", { name: "26 failed requests" }).click();
  await expect(dialog.getByRole("alert")).toContainText("Unable to load failed requests");
  await expect(dialog).not.toContainText("raw-provider-body");
  await expect(dialog).not.toContainText("sk-never-render");
  await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("40");

  failFirstPage = false;
  await dialog.getByRole("button", { name: "Retry" }).click();
  await expect(dialog.locator("usage-failure-row")).toHaveCount(25);
});

test("failed-request responses cannot cross interval or Usage tenant boundaries", async ({ page }) => {
  await installAssetRoutes(page);
  const routeState = await installMultiTenantRoutes(page, {
    usageRequests: { tenant_1: 22, tenant_2: 7 },
  });
  /** @type {() => void} */
  let releaseFailureResponse = () => {};
  const failureResponseGate = new Promise((resolve) => {
    releaseFailureResponse = () => resolve(undefined);
  });
  await page.route(usageFailuresRequestPattern(), async (route) => {
    await failureResponseGate;
    await route.fulfill({ json: managementUsageFailures("30d", 10) });
  });

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByRole("button", { name: "2 failed requests" }).click();
  await expect(page.getByRole("dialog", { name: "Failed request details" })).toHaveAttribute("aria-busy", "true");

  await page.locator("llm-proxy-key-management").evaluate((applicationElement) => {
    const alpineRuntime = /** @type {typeof globalThis & { Alpine?: { $data: (element: Element) => any } }} */ (globalThis);
    const applicationState = alpineRuntime.Alpine?.$data(applicationElement);
    if (!applicationState) {
      throw new Error("usage_failures_state_missing");
    }
    void applicationState.selectUsageInterval("7d");
  });
  await expect(page.getByRole("dialog", { name: "Failed request details" })).toBeHidden();
  await expect(page.getByRole("button", { name: /failed request/ })).toHaveCount(0);

  await page.locator("llm-proxy-key-management").evaluate((applicationElement) => {
    const alpineRuntime = /** @type {typeof globalThis & { Alpine?: { $data: (element: Element) => any } }} */ (globalThis);
    const applicationState = alpineRuntime.Alpine?.$data(applicationElement);
    if (!applicationState) {
      throw new Error("usage_failures_state_missing");
    }
    void applicationState.handleUsageTenantSelection({ target: { value: "tenant_2" } });
  });
  releaseFailureResponse();
  await expect(page.getByRole("combobox", { name: "Usage tenant" })).toHaveValue("tenant_2");
  await expect(page.getByRole("dialog", { name: "Failed request details" })).toBeHidden();
  expect(routeState.requests.some((request) => request.path === "/api/management/tenants/tenant_2/usage")).toBe(true);
});

test("pasting a provider key verifies before blur, locks conflicts, and masks the accepted key", async ({ page }) => {
  const pastedProviderKey = "sk-pasted-operational-key";
  let providerSaveRequestCount = 0;
  /** @type {() => void} */
  let releaseProviderSave = () => {};
  const providerSaveReleased = new Promise((resolve) => {
    releaseProviderSave = () => resolve(undefined);
  });
  /** @type {() => void} */
  let providerSaveStarted = () => {};
  const providerSaveRequested = new Promise((resolve) => {
    providerSaveStarted = () => resolve(undefined);
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page, { savedProviderIDs: [] });
  await page.route(providerKeyEndpointURL("openai"), async (route) => {
    providerSaveRequestCount += 1;
    providerSaveStarted();
    await providerSaveReleased;
    await route.fallback();
  });

  await page.goto(`${baseURL}${applicationPath}`);
  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const providerEditor = settingsDialog.locator("provider-editor");
  await providerEditor.getByRole("combobox", { name: "Provider", exact: true }).selectOption("openai");
  const providerKeyInput = providerEditor.getByRole("textbox", { name: "OpenAI API key" });
  await pasteProviderKey(providerKeyInput, pastedProviderKey);
  await providerSaveRequested;

  await expect(providerKeyInput).toBeFocused();
  await expect(providerKeyInput).toBeEnabled();
  await expect(providerEditor.getByRole("status")).toHaveText("Verifying key");
  await expect(settingsDialog.getByRole("button", { name: "Close" })).toBeDisabled();
  await expect(settingsDialog.getByRole("combobox", { name: "Tenant" })).toBeDisabled();
  await expect(providerEditor.getByRole("combobox", { name: "Provider", exact: true })).toBeDisabled();
  await expect(providerEditor.getByRole("combobox", { name: "Provider default model" })).toBeDisabled();
  await expect(providerEditor.getByRole("button", { name: "Hide key" })).toBeDisabled();
  await expect(providerEditor.getByRole("button", { name: "Remove provider key and settings" })).toBeDisabled();
  expect(providerSaveRequestCount).toBe(1);

  releaseProviderSave();
  await expect(providerEditor.getByRole("status")).toBeHidden();
  await expect(page.locator("#llm-proxy-header .notice")).toHaveText("Provider key verified and settings saved");
  await expect(providerKeyInput).toHaveValue("****aved");
  await expect(providerKeyInput).toHaveAttribute("readonly", "readonly");
  await expect(providerKeyInput).not.toHaveValue(pastedProviderKey);
  expect(await browserStorageContains(page, pastedProviderKey)).toBe(false);
  expect(providerSaveRequestCount).toBe(1);
});

test("rejected pasted keys remain editable and retry through the same operation", async ({ page }) => {
  const rejectedProviderKey = "sk-rejected-provider-key";
  let providerSaveRequestCount = 0;
  await installAssetRoutes(page);
  await installManagementRoutes(page, { savedProviderIDs: [] });
  await page.route(providerKeyEndpointURL("openai"), async (route) => {
    providerSaveRequestCount += 1;
    if (providerSaveRequestCount === 1) {
      await route.fulfill({ status: 422, body: "provider_key_rejected" });
      return;
    }
    await route.fallback();
  });

  await page.goto(`${baseURL}${applicationPath}`);
  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const providerEditor = settingsDialog.locator("provider-editor");
  await providerEditor.getByRole("combobox", { name: "Provider", exact: true }).selectOption("openai");
  const providerKeyInput = providerEditor.getByRole("textbox", { name: "OpenAI API key" });
  await pasteProviderKey(providerKeyInput, rejectedProviderKey);

  const verificationFailure = providerEditor.getByRole("alert");
  await expect(verificationFailure).toContainText("Key was rejected. No provider key was saved.");
  await expect(providerKeyInput).toHaveValue(rejectedProviderKey);
  await expect(settingsDialog).toBeVisible();
  expect(providerSaveRequestCount).toBe(1);

  await verificationFailure.getByRole("button", { name: "Retry verification" }).click();
  await expect(page.locator("#llm-proxy-header .notice")).toHaveText("Provider key verified and settings saved");
  await expect(verificationFailure).toBeHidden();
  await expect(providerKeyInput).toHaveValue("****aved");
  expect(providerSaveRequestCount).toBe(2);
});

test("a rejected pasted replacement keeps the previous verified key active", async ({ page }) => {
  const rejectedReplacement = "sk-rejected-replacement";
  await installAssetRoutes(page);
  await installManagementRoutes(page);
  await page.route(providerKeyEndpointURL("openai"), async (route) => {
    await route.fulfill({ status: 422, body: "provider_key_rejected" });
  });

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();
  const providerEditor = page.getByRole("dialog", { name: "Settings" }).locator("provider-editor");
  const providerKeyInput = providerEditor.getByRole("textbox", { name: "OpenAI API key" });
  await providerEditor.getByRole("button", { name: "Show key" }).click();
  await pasteProviderKey(providerKeyInput, rejectedReplacement);

  await expect(providerEditor.getByRole("alert")).toContainText(
    "Key was rejected. The previous key remains active.",
  );
  await expect(providerKeyInput).toHaveValue(rejectedReplacement);
});

test("a newer pasted key cancels the stale verification and applies only the newest result", async ({ page }) => {
  const staleProviderKey = "sk-stale-pasted-key";
  const currentProviderKey = "sk-current-pasted-key";
  const submittedKeys = [];
  /** @type {() => void} */
  let releaseStaleVerification = () => {};
  const staleVerificationReleased = new Promise((resolve) => {
    releaseStaleVerification = () => resolve(undefined);
  });
  /** @type {() => void} */
  let staleVerificationStarted = () => {};
  const staleVerificationRequested = new Promise((resolve) => {
    staleVerificationStarted = () => resolve(undefined);
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page, { savedProviderIDs: [] });
  await page.route(providerKeyEndpointURL("openai"), async (route) => {
    const submittedKey = route.request().postDataJSON().api_key;
    submittedKeys.push(submittedKey);
    if (submittedKey === staleProviderKey) {
      staleVerificationStarted();
      await staleVerificationReleased;
      await route.fulfill({ status: 422, body: "provider_key_rejected" }).catch(() => {});
      return;
    }
    await route.fallback();
  });

  await page.goto(`${baseURL}${applicationPath}`);
  const providerEditor = page.getByRole("dialog", { name: "Settings" }).locator("provider-editor");
  await providerEditor.getByRole("combobox", { name: "Provider", exact: true }).selectOption("openai");
  const providerKeyInput = providerEditor.getByRole("textbox", { name: "OpenAI API key" });
  await pasteProviderKey(providerKeyInput, staleProviderKey);
  await staleVerificationRequested;
  await pasteProviderKey(providerKeyInput, currentProviderKey);

  await expect.poll(() => submittedKeys).toEqual([staleProviderKey, currentProviderKey]);
  await expect(page.locator("#llm-proxy-header .notice")).toHaveText("Provider key verified and settings saved");
  await expect(providerKeyInput).toHaveValue("****aved");
  releaseStaleVerification();
  await expect(providerEditor.getByRole("alert")).toBeHidden();
  expect(submittedKeys).toEqual([staleProviderKey, currentProviderKey]);
});

for (const verificationContextChange of [
  { id: "tenant", label: "tenant switch", article: "a", multiTenant: true },
  { id: "provider", label: "provider switch", article: "a", multiTenant: false },
  { id: "model", label: "model change", article: "a", multiTenant: false },
  { id: "editor", label: "editor replacement", article: "an", multiTenant: false },
]) {
  test(`${verificationContextChange.article} ${verificationContextChange.label} rejects a stale provider-key verification completion`, async ({ page }) => {
    const staleProviderKey = `sk-stale-${verificationContextChange.id}-context`;
    /** @type {() => void} */
    let releaseVerification = () => {};
    const verificationReleased = new Promise((resolve) => {
      releaseVerification = () => resolve(undefined);
    });
    /** @type {() => void} */
    let verificationStarted = () => {};
    const verificationRequested = new Promise((resolve) => {
      verificationStarted = () => resolve(undefined);
    });
    await installAssetRoutes(page);
    if (verificationContextChange.multiTenant) {
      await installMultiTenantRoutes(page);
    } else {
      await installManagementRoutes(page, { savedProviderIDs: [] });
    }
    await page.route(providerKeyEndpointURL("openai"), async (route) => {
      verificationStarted();
      await verificationReleased;
      await route.fallback().catch(() => {});
    });

    await page.goto(`${baseURL}${applicationPath}`);
    if (verificationContextChange.multiTenant) {
      await page.getByTestId("avatar-menu").click();
      await page.getByTestId("avatar-menu-item").nth(0).click();
    }
    const settingsDialog = page.getByRole("dialog", { name: "Settings" });
    const providerEditor = settingsDialog.locator("provider-editor");
    const providerSelector = providerEditor.getByRole("combobox", { name: "Provider", exact: true });
    if (!verificationContextChange.multiTenant) {
      await providerSelector.selectOption("openai");
    }
    const providerKeyInput = providerEditor.getByRole("textbox", { name: "OpenAI API key" });
    if (verificationContextChange.multiTenant) {
      await providerEditor.getByRole("button", { name: "Show key" }).click();
    }
    await pasteProviderKey(providerKeyInput, staleProviderKey);
    await verificationRequested;
    await expect(providerEditor.getByRole("status")).toHaveText("Verifying key");

    await page.locator("llm-proxy-key-management").evaluate(
      async (applicationElement, contextChange) => {
        const alpineRuntime = /** @type {typeof globalThis & { Alpine?: { $data: (element: Element) => any } }} */ (globalThis);
        const applicationState = alpineRuntime.Alpine?.$data(applicationElement);
        if (!applicationState) {
          throw new Error("provider_verification_state_missing");
        }
        switch (contextChange) {
          case "tenant":
            await applicationState.switchSettingsTenant("tenant_2");
            break;
          case "provider":
            applicationState.replaceProviderEditorSession("anthropic");
            break;
          case "model":
            applicationState.handleSelectedProviderTextModelChange({ target: { value: "gpt-4o-mini" } });
            break;
          case "editor":
            applicationState.replaceProviderEditorSession(applicationState.selectedProviderID);
            break;
          default:
            throw new Error("provider_verification_context_change_invalid");
        }
      },
      verificationContextChange.id,
    );
    releaseVerification();

    await expect(providerEditor.getByRole("status")).toBeHidden();
    await expect(providerEditor.getByRole("alert")).toBeHidden();
    await expect(page.locator("#llm-proxy-header .notice")).not.toHaveText(
      "Provider key verified and settings saved",
    );
    if (verificationContextChange.id === "tenant") {
      await expect(settingsDialog.getByRole("combobox", { name: "Tenant" })).toHaveValue("tenant_2");
    } else if (verificationContextChange.id === "provider") {
      await expect(providerSelector).toHaveValue("anthropic");
    } else if (verificationContextChange.id === "model") {
      await expect(providerEditor.getByRole("combobox", { name: "Provider default model" })).toHaveValue(
        "gpt-4o-mini",
      );
    } else {
      await expect(providerKeyInput).toHaveValue("");
    }
  });
}

test("provider selection autosaves its exact editor while transient removal stays local", async ({ page }) => {
  const firstGrokKey = "xai-provider-first-1111";
  const secondGrokKey = "xai-provider-second-2222";
  const deepSeekProviderKey = "sk-owner-deepseek-revealed";
  const providerMutations = [];
  page.on("request", (request) => {
    if (!request.url().includes(managementProviderKeysPath) || !["PUT", "DELETE"].includes(request.method())) {
      return;
    }
    providerMutations.push({
      method: request.method(),
      url: request.url(),
      payload: request.method() === "PUT" ? request.postDataJSON() : null,
    });
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page, { providerKeys: { deepseek: deepSeekProviderKey } });

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const providerEditor = settingsDialog.locator("provider-editor");
  const providerSelector = providerEditor.getByRole("combobox", { name: "Provider", exact: true });

  await providerSelector.selectOption("grok");
  const grokKeyInput = providerEditor.getByRole("textbox", { name: "Grok API key" });
  await grokKeyInput.fill(firstGrokKey);
  const providerRemovalButton = providerEditor.getByRole("button", { name: "Remove provider key and settings" });
  await expect(providerRemovalButton).toBeVisible();
  await providerRemovalButton.click();
  await expect(grokKeyInput).toHaveValue("");
  await expect(grokKeyInput).toBeFocused();
  await expect(page.getByRole("alertdialog", { name: "Remove provider key?" })).toBeHidden();
  expect(providerMutations).toHaveLength(0);

  await grokKeyInput.fill(firstGrokKey);
  await expect(providerEditor.getByRole("button", { name: /^(Save|Update) key$/ })).toHaveCount(0);
  await providerSelector.selectOption("meta");
  await expect.poll(() => providerMutations.length).toBe(1);
  expect(providerMutations.at(-1)).toMatchObject({
    method: "PUT",
    url: providerKeyEndpointURL("grok"),
    payload: { api_key: firstGrokKey, text_model: "grok-4.3", system_prompt: "" },
  });
  const metaKeyInput = providerEditor.getByRole("textbox", { name: "Meta API key" });
  await expect(metaKeyInput).toHaveValue("****meta");
  await expect(metaKeyInput).toHaveAttribute("readonly", "readonly");
  await expect(providerEditor.getByRole("combobox", { name: "Provider default model" })).toHaveValue("muse-spark-1.1");

  await providerSelector.selectOption("grok");
  await expect(grokKeyInput).toHaveValue("****aved");
  await expect(grokKeyInput).toHaveAttribute("readonly", "readonly");
  await providerEditor.getByRole("button", { name: "Show key" }).click();
  await expect(grokKeyInput).toHaveValue(firstGrokKey);
  await grokKeyInput.fill(secondGrokKey);
  await providerEditor.getByRole("button", { name: "Hide key" }).click();
  await expect(grokKeyInput).toHaveValue("****2222");
  await providerSelector.selectOption("deepseek");
  await expect.poll(() => providerMutations.length).toBe(2);
  expect(providerMutations.at(-1)).toMatchObject({
    method: "PUT",
    url: providerKeyEndpointURL("grok"),
    payload: { api_key: secondGrokKey, text_model: "grok-4.3", system_prompt: "" },
  });
  const deepSeekKeyInput = providerEditor.getByRole("textbox", { name: "DeepSeek API key" });
  await expect(deepSeekKeyInput).toHaveValue("****5678");
  await providerEditor.getByRole("button", { name: "Show key" }).click();
  await expect(deepSeekKeyInput).toHaveValue(deepSeekProviderKey);

  await providerSelector.selectOption("grok");
  await expect(grokKeyInput).toHaveValue("****aved");
  await providerEditor.getByRole("button", { name: "Show key" }).click();
  await expect(grokKeyInput).toHaveValue(secondGrokKey);
  await providerSelector.selectOption("deepseek");
  await expect(deepSeekKeyInput).toHaveValue("****5678");
  await expect(deepSeekKeyInput).not.toHaveValue(deepSeekProviderKey);
  await expect(deepSeekKeyInput).toHaveAttribute("readonly", "readonly");

  await providerSelector.selectOption("grok");
  await expect(grokKeyInput).not.toHaveValue(secondGrokKey);
  expect(await browserStorageContains(page, firstGrokKey)).toBe(false);
  expect(await browserStorageContains(page, secondGrokKey)).toBe(false);

  await providerRemovalButton.click();
  const removalConfirmation = page.getByRole("alertdialog", { name: "Remove provider key?" });
  await expect(removalConfirmation).toBeVisible();
  await expect(settingsDialog).toHaveAttribute("inert", /^(|inert|true)$/);
  expect(providerMutations.filter((mutation) => mutation.method === "DELETE")).toHaveLength(0);
  const cancelRemovalButton = removalConfirmation.getByRole("button", { name: "Cancel" });
  const confirmRemovalButton = removalConfirmation.getByRole("button", { name: "Remove key" });
  await expect(cancelRemovalButton).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(confirmRemovalButton).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(cancelRemovalButton).toBeFocused();
  await cancelRemovalButton.click();
  await expect(removalConfirmation).toBeHidden();
  await expect(settingsDialog).not.toHaveAttribute("inert", "inert");
  await expect(providerRemovalButton).toBeFocused();
  expect(providerMutations.filter((mutation) => mutation.method === "DELETE")).toHaveLength(0);

  await providerRemovalButton.click();
  await removalConfirmation.getByRole("button", { name: "Remove key" }).click();
  await expect.poll(() => providerMutations.filter((mutation) => mutation.method === "DELETE").length).toBe(1);
  expect(providerMutations.at(-1)).toMatchObject({
    method: "DELETE",
    url: providerKeyEndpointURL("grok"),
  });
  await expect(grokKeyInput).toHaveValue("");
});

test("Settings close waits for the current provider autosave", async ({ page }) => {
  let releaseProviderSave;
  const providerSaveReleased = new Promise((resolve) => {
    releaseProviderSave = resolve;
  });
  let providerSaveStarted;
  const providerSaveRequested = new Promise((resolve) => {
    providerSaveStarted = resolve;
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page, { savedProviderIDs: [] });
  await page.route(providerKeyEndpointURL("openai"), async (route) => {
    providerSaveStarted();
    await providerSaveReleased;
    await route.fallback();
  });

  await page.goto(`${baseURL}${applicationPath}`);
  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const providerEditor = settingsDialog.locator("provider-editor");
  await providerEditor.getByRole("combobox", { name: "Provider", exact: true }).selectOption("openai");
  await providerEditor.getByRole("textbox", { name: "OpenAI API key" }).fill("sk-close-autosave");
  await settingsDialog.getByRole("button", { name: "Close" }).click();
  await providerSaveRequested;
  await expect(settingsDialog).toBeVisible();
  await expect(page.locator("#llm-proxy-header .notice")).not.toHaveText(
    "Add at least one provider API key before leaving Settings.",
  );

  releaseProviderSave();
  await expect(settingsDialog).toBeHidden();
});

test("failed provider autosave preserves its editor and blocks provider switching", async ({ page }) => {
  const editedProviderKey = "sk-openai-autosave-failure";
  let providerSaveRequestCount = 0;
  await installAssetRoutes(page);
  await installManagementRoutes(page);
  await page.route(providerKeyEndpointURL("openai"), async (route) => {
    providerSaveRequestCount += 1;
    await route.fulfill({ status: httpInternalServerError, body: "request_failed" });
  });

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();
  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const providerEditor = settingsDialog.locator("provider-editor");
  const providerSelector = providerEditor.getByRole("combobox", { name: "Provider", exact: true });
  const providerKeyInput = providerEditor.getByRole("textbox", { name: "OpenAI API key" });
  await providerEditor.getByRole("button", { name: "Show key" }).click();
  await providerKeyInput.fill(editedProviderKey);
  await page.keyboard.press("Tab");

  await expect.poll(() => providerSaveRequestCount).toBe(1);
  await expect(page.locator("#llm-proxy-header .notice")).toHaveText("Request failed");
  await expect(providerKeyInput).toHaveValue(editedProviderKey);
  await expect(settingsDialog).toBeVisible();

  await providerSelector.selectOption("deepseek");
  await expect.poll(() => providerSaveRequestCount).toBe(2);
  await expect(providerSelector).toHaveValue("openai");
  await expect(providerKeyInput).toHaveValue(editedProviderKey);
});

test("session cleanup cancels provider autosaves before they can repopulate state", async ({ page }) => {
  const lateProviderKey = "sk-anthropic-late-autosave";
  /** @type {() => void} */
  let releaseProviderSave = () => {};
  const providerSaveReleased = new Promise((resolve) => {
    releaseProviderSave = () => resolve(undefined);
  });
  /** @type {() => void} */
  let providerSaveStarted = () => {};
  const providerSaveRequested = new Promise((resolve) => {
    providerSaveStarted = () => resolve(undefined);
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page);
  await page.route(providerKeyEndpointURL("anthropic"), async (route) => {
    providerSaveStarted();
    await providerSaveReleased;
    await route.fulfill({ status: httpOK, json: managementProfile() }).catch(() => {});
  });

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();
  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const providerEditor = settingsDialog.locator("provider-editor");
  const providerSelector = providerEditor.getByRole("combobox", { name: "Provider", exact: true });
  await providerSelector.selectOption("anthropic");
  await pasteProviderKey(providerEditor.getByRole("textbox", { name: "Anthropic API key" }), lateProviderKey);
  await providerSaveRequested;
  await expect(providerEditor.getByRole("status")).toHaveText("Verifying key");

  await page.evaluate(() => {
    document.dispatchEvent(new CustomEvent("mpr-ui:auth:unauthenticated"));
  });
  await expect(page.getByRole("heading", { name: "Sign in to manage LLM Proxy keys" })).toBeVisible();
  releaseProviderSave();
  await page.waitForTimeout(50);
  await expect(page.locator("#llm-proxy-header .notice")).toHaveText("Authentication required");
  await expect(page.locator("body")).not.toContainText(lateProviderKey);
  expect(await browserStorageContains(page, lateProviderKey)).toBe(false);

  await page.evaluate(() => window.__llmProxyMprAuthenticate());
  await expect(page.getByRole("heading", { name: "Usage overview" })).toBeVisible();
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();
  await providerSelector.selectOption("anthropic");
  await expect(providerEditor.getByRole("textbox", { name: "Anthropic API key" })).toHaveValue("");
  await expect(providerEditor).not.toContainText(lateProviderKey);
});

test("saved provider keys reveal, edit, and clear without browser persistence", async ({ page }) => {
  const revealedProviderKey = "sk-owner-openai-revealed";
  const editedProviderKey = "sk-owner-openai-edited";
  let revealRequestCount = 0;
  const savedProviderSettingsPayloads = [];
  page.on("request", (request) => {
    if (request.url() === providerKeyEndpointURL("openai", "reveal")) {
      revealRequestCount += 1;
    }
    if (request.url() === providerKeyEndpointURL("openai") && request.method() === "PUT") {
      savedProviderSettingsPayloads.push(JSON.parse(request.postData() || "{}"));
    }
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page, { providerKeys: { openai: revealedProviderKey } });

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const providerEditor = settingsDialog.locator("provider-editor");
  const providerSelector = providerEditor.getByRole("combobox", { name: "Provider", exact: true });
  const providerKeyInput = providerEditor.getByRole("textbox", { name: "OpenAI API key" });
  const providerVisibilityButton = providerEditor.locator(".provider-key-visibility-toggle");
  await expect(providerKeyInput).toHaveValue("****1234");
  await expect(providerKeyInput).toHaveAttribute("readonly", "readonly");
  await expect(providerVisibilityButton).toHaveAccessibleName("Show key");
  await expect(providerVisibilityButton).toBeVisible();
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

  const visibilityBoxBeforePress = await providerVisibilityButton.boundingBox();
  if (!visibilityBoxBeforePress) {
    throw new Error("provider_visibility_control_missing");
  }
  await page.mouse.move(
    visibilityBoxBeforePress.x + visibilityBoxBeforePress.width / 2,
    visibilityBoxBeforePress.y + visibilityBoxBeforePress.height / 2,
  );
  await page.mouse.down();
  await page.waitForTimeout(160);
  expect(await providerVisibilityButton.boundingBox()).toEqual(visibilityBoxBeforePress);
  await page.mouse.up();
  await expect(providerKeyInput).toHaveValue(revealedProviderKey);
  await expect(providerKeyInput).not.toHaveAttribute("readonly", "readonly");
  await expect(providerEditor.getByRole("button", { name: "Hide key" })).toBeVisible();
  await expect(visibilitySymbols.nth(0)).toBeHidden();
  await expect(visibilitySymbols.nth(1)).toBeVisible();
  await expect(providerEditor.getByText("Hide key", { exact: true })).toHaveCount(0);
  expect(await providerVisibilityButton.boundingBox()).toEqual(visibilityBoxBeforePress);
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
  await page.keyboard.press("Tab");
  await expect.poll(() => savedProviderSettingsPayloads.length).toBe(1);
  await expect(providerKeyInput).not.toHaveValue(editedProviderKey);
  await expect(providerKeyInput).toHaveAttribute("readonly", "readonly");
  await expect(providerEditor.getByRole("button", { name: "Show key" })).toBeVisible();
  expect(savedProviderSettingsPayloads.at(-1)).toEqual({
    api_key: editedProviderKey,
    text_model: "gpt-4.1",
    system_prompt: "Use concise answers.",
  });

  const providerModelSelector = providerEditor.getByRole("combobox", { name: "Provider default model" });
  await providerModelSelector.selectOption("gpt-4o-mini");
  await expect.poll(() => savedProviderSettingsPayloads.length).toBe(2);
  expect(savedProviderSettingsPayloads.at(-1)).toEqual({
    api_key: "",
    text_model: "gpt-4o-mini",
    system_prompt: "Use concise answers.",
  });
  await providerEditor.locator("summary.system-prompt-summary").click();
  const providerSystemPrompt = providerEditor.getByRole("textbox", { name: "System prompt" });
  await providerSystemPrompt.fill("Use autosaved provider guidance.");
  await page.keyboard.press("Tab");
  await expect.poll(() => savedProviderSettingsPayloads.length).toBe(3);
  expect(savedProviderSettingsPayloads.at(-1)).toEqual({
    api_key: "",
    text_model: "gpt-4o-mini",
    system_prompt: "Use autosaved provider guidance.",
  });
  await expect(providerEditor.getByRole("button", { name: /^(Save|Update) key$/ })).toHaveCount(0);
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
  let removalRequestCount = 0;
  page.on("request", (request) => {
    if (request.url() === providerKeyEndpointURL("openai") && request.method() === "DELETE") {
      removalRequestCount += 1;
    }
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page, {
    providerKeys: { openai: revealedProviderKey },
    savedProviderIDs: ["openai"],
  });

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const providerEditor = settingsDialog.locator("provider-editor");
  const providerKeyInput = providerEditor.getByRole("textbox", { name: "OpenAI API key" });
  await providerEditor.getByRole("button", { name: "Show key" }).click();
  await expect(providerKeyInput).toHaveValue(revealedProviderKey);
  const removalButton = providerEditor.getByRole("button", { name: "Remove provider key and settings" });
  await removalButton.click();
  const removalConfirmation = page.getByRole("alertdialog", { name: "Remove provider key?" });
  await expect(removalConfirmation).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(removalConfirmation).toBeHidden();
  expect(removalRequestCount).toBe(0);
  await expect(providerKeyInput).toHaveValue(revealedProviderKey);

  await removalButton.click();
  await removalConfirmation.getByRole("button", { name: "Remove key" }).click();
  await expect.poll(() => removalRequestCount).toBe(1);

  await expect(providerKeyInput).toHaveValue("");
  await expect(providerKeyInput).not.toHaveAttribute("readonly", "readonly");
  await expect(providerEditor.getByRole("button", { name: "Show key" })).toBeHidden();
  await expect(settingsDialog.getByRole("alert")).toHaveText(
    "Add at least one provider API key before leaving Settings.",
  );
  await expect(settingsDialog.getByRole("alert")).toBeFocused();
  await settingsDialog.getByRole("button", { name: "Close" }).click();
  await expect(settingsDialog).toBeVisible();
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

  await page.goto(`${baseURL}${applicationPath}`);
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
  await expect(providerEditor.getByRole("button", { name: /^(Save|Update) key$/ })).toHaveCount(0);
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

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const providerEditor = settingsDialog.locator("provider-editor");
  await providerEditor.getByRole("combobox", { name: "Provider", exact: true }).selectOption("meta");
  const providerKeyInput = providerEditor.getByRole("textbox", { name: "Meta API key" });
  await expect(providerKeyInput).toHaveValue("****");
  await expect(providerKeyInput).not.toHaveValue("****saved");
});

test("routing defaults autosave complete provider and model pairs without a manual action", async ({ page }) => {
  const defaultsMutations = [];
  page.on("request", (request) => {
    if (request.url() === `${baseURL}${managementDefaultTenantPath}/defaults`) {
      defaultsMutations.push(request.postDataJSON());
    }
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page, { savedProviderIDs: ["openai", "deepseek", "meta", "grok"] });

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const defaultsForm = settingsDialog.locator(".settings-grid-form");
  const textProvider = defaultsForm.getByRole("combobox", { name: "Text provider" });
  const textModel = defaultsForm.getByRole("combobox", { name: "Text model" });
  const dictationProvider = defaultsForm.getByRole("combobox", { name: "Dictation provider" });
  const dictationModel = defaultsForm.getByRole("combobox", { name: "Dictation model" });
  const systemPromptDisclosure = defaultsForm.locator("details.system-prompt-disclosure");
  const systemPrompt = defaultsForm.locator("#routing-system-prompt-input");

  await expect(textProvider).toHaveValue("openai");
  await expect(textModel).toHaveValue("gpt-4.1");
  await expect(dictationProvider).toHaveValue("openai");
  await expect(dictationModel).toHaveValue("gpt-4o-mini-transcribe");
  await expect(dictationModel.locator('option[value=""]')).toHaveCount(0);
  await expect(defaultsForm.getByRole("button", { name: "Save defaults" })).toHaveCount(0);

  await textProvider.selectOption("deepseek");
  await expect(textModel).toHaveValue("deepseek-chat");
  await expect.poll(() => defaultsMutations.length).toBe(1);
  expect(defaultsMutations.at(-1)).toEqual({
    provider: "deepseek",
    model: "deepseek-chat",
    dictation_provider: "openai",
    dictation_model: "gpt-4o-mini-transcribe",
    system_prompt: "",
    reasoning_effort: "",
  });

  await dictationProvider.selectOption("grok");
  await expect(dictationModel).toHaveValue("xai-stt");
  await expect.poll(() => defaultsMutations.length).toBe(2);
  expect(defaultsMutations.at(-1)).toEqual({
    provider: "deepseek",
    model: "deepseek-chat",
    dictation_provider: "grok",
    dictation_model: "xai-stt",
    system_prompt: "",
    reasoning_effort: "",
  });

  await systemPromptDisclosure.locator("summary.system-prompt-summary").click();
  await systemPrompt.fill("Use tenant-wide autosaved guidance.");
  expect(defaultsMutations).toHaveLength(2);
  await page.keyboard.press("Tab");
  await expect.poll(() => defaultsMutations.length).toBe(3);
  expect(defaultsMutations.at(-1)).toEqual({
    provider: "deepseek",
    model: "deepseek-chat",
    dictation_provider: "grok",
    dictation_model: "xai-stt",
    system_prompt: "Use tenant-wide autosaved guidance.",
    reasoning_effort: "",
  });
  await expect(page.locator("notification-region")).toHaveText("Defaults saved");
  await expect(defaultsForm).not.toHaveAttribute("aria-busy", "true");

  const reloadedProfileResponse = page.waitForResponse(`${baseURL}${managementDefaultTenantPath}`);
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
  await expect(settingsDialog.locator("#routing-system-prompt-input")).toHaveValue(
    "Use tenant-wide autosaved guidance.",
  );
});

test("routing defaults expose only keyed providers and disable unavailable dictation", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page, { savedProviderIDs: ["deepseek"] });

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const defaultsForm = settingsDialog.locator(".settings-grid-form");
  const textProvider = defaultsForm.getByRole("combobox", { name: "Text provider" });
  const textModel = defaultsForm.getByRole("combobox", { name: "Text model" });
  const dictationProvider = defaultsForm.getByRole("combobox", { name: "Dictation provider" });
  const dictationModel = defaultsForm.getByRole("combobox", { name: "Dictation model" });

  await expect(textProvider).toHaveValue("deepseek");
  await expect(textProvider.locator("option")).toHaveText(["DeepSeek"]);
  await expect(textModel).toHaveValue("deepseek-chat");
  await expect(dictationProvider).toBeDisabled();
  await expect(dictationModel).toBeDisabled();
  await expect(dictationProvider).toHaveValue("");
  await expect(dictationModel).toHaveValue("");
  await expect(dictationProvider.locator("option")).toHaveText(["Not configured"]);
  await expect(settingsDialog.getByText("Default dictation", { exact: true })).toHaveCount(0);

  const providerEditor = settingsDialog.locator("provider-editor");
  await providerEditor.getByRole("combobox", { name: "Provider", exact: true }).selectOption("openai");
  await providerEditor.getByRole("textbox", { name: "OpenAI API key" }).fill("sk-openai-new");
  await page.keyboard.press("Tab");

  await expect(page.locator("notification-region")).toHaveText("Provider key verified and settings saved");
  await expect(textProvider.locator("option")).toHaveText(["OpenAI", "DeepSeek"]);
  await expect(textProvider).toHaveValue("deepseek");
  await expect(dictationProvider).toBeEnabled();
  await expect(dictationModel).toBeEnabled();
  await expect(dictationProvider).toHaveValue("openai");
  await expect(dictationModel).toHaveValue("gpt-4o-mini-transcribe");
  await expect(settingsDialog.getByText("Default dictation", { exact: true })).toHaveCount(1);
});

test("routing-default autosave queues newer edits without resetting the provider editor", async ({ page }) => {
  const revealedProviderKey = "sk-routing-autosave-owner";
  const defaultsMutations = [];
  let releaseFirstDefaultsSave;
  const firstDefaultsSaveReleased = new Promise((resolve) => {
    releaseFirstDefaultsSave = resolve;
  });
  let firstDefaultsSaveStarted;
  const firstDefaultsSaveRequested = new Promise((resolve) => {
    firstDefaultsSaveStarted = resolve;
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page, {
    providerKeys: { openai: revealedProviderKey },
    savedProviderIDs: ["openai", "deepseek", "meta", "grok"],
  });
  await page.route(`${baseURL}${managementDefaultTenantPath}/defaults`, async (route) => {
    defaultsMutations.push(route.request().postDataJSON());
    if (defaultsMutations.length === 1) {
      firstDefaultsSaveStarted();
      await firstDefaultsSaveReleased;
    }
    await route.fallback();
  });

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const providerEditor = settingsDialog.locator("provider-editor");
  const providerKeyInput = providerEditor.getByRole("textbox", { name: "OpenAI API key" });
  await providerEditor.getByRole("button", { name: "Show key" }).click();
  await expect(providerKeyInput).toHaveValue(revealedProviderKey);

  const defaultsForm = settingsDialog.locator(".settings-grid-form");
  await defaultsForm.getByRole("combobox", { name: "Text provider" }).selectOption("deepseek");
  await firstDefaultsSaveRequested;
  await defaultsForm.getByRole("combobox", { name: "Dictation provider" }).selectOption("grok");
  await defaultsForm.locator("summary.system-prompt-summary").click();
  await defaultsForm.getByRole("textbox", { name: "System prompt" }).fill("Keep the latest defaults only.");
  await page.keyboard.press("Tab");

  releaseFirstDefaultsSave();
  await expect.poll(() => defaultsMutations.length).toBe(2);
  expect(defaultsMutations.at(-1)).toEqual({
    provider: "deepseek",
    model: "deepseek-chat",
    dictation_provider: "grok",
    dictation_model: "xai-stt",
    system_prompt: "Keep the latest defaults only.",
    reasoning_effort: "",
  });
  await expect(providerKeyInput).toHaveValue(revealedProviderKey);
  await expect(page.locator("notification-region")).toHaveText("Defaults saved");
  await expect(defaultsForm).not.toHaveAttribute("aria-busy", "true");
});

test("provider and routing autosaves serialize whole-profile mutations in both directions", async ({ page }) => {
  const providerMutations = [];
  const defaultsMutations = [];
  let releaseFirstProviderSave;
  const firstProviderSaveReleased = new Promise((resolve) => {
    releaseFirstProviderSave = resolve;
  });
  let firstProviderSaveStarted;
  const firstProviderSaveRequested = new Promise((resolve) => {
    firstProviderSaveStarted = resolve;
  });
  let releaseSecondDefaultsSave;
  const secondDefaultsSaveReleased = new Promise((resolve) => {
    releaseSecondDefaultsSave = resolve;
  });
  let secondDefaultsSaveStarted;
  const secondDefaultsSaveRequested = new Promise((resolve) => {
    secondDefaultsSaveStarted = resolve;
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page);
  await page.route(providerKeyEndpointURL("openai"), async (route) => {
    if (route.request().method() !== "PUT") {
      await route.fallback();
      return;
    }
    providerMutations.push(route.request().postDataJSON());
    if (providerMutations.length === 1) {
      firstProviderSaveStarted();
      await firstProviderSaveReleased;
    }
    await route.fallback();
  });
  await page.route(`${baseURL}${managementDefaultTenantPath}/defaults`, async (route) => {
    defaultsMutations.push(route.request().postDataJSON());
    if (defaultsMutations.length === 2) {
      secondDefaultsSaveStarted();
      await secondDefaultsSaveReleased;
    }
    await route.fallback();
  });

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const providerEditor = settingsDialog.locator("provider-editor");
  const providerModel = providerEditor.getByRole("combobox", { name: "Provider default model" });
  await providerEditor.locator("summary.system-prompt-summary").click();
  const providerPrompt = providerEditor.getByRole("textbox", { name: "System prompt" });
  const defaultsForm = settingsDialog.locator(".settings-grid-form");
  const defaultTextProvider = defaultsForm.getByRole("combobox", { name: "Text provider" });

  await providerModel.selectOption("gpt-5-mini");
  await firstProviderSaveRequested;
  await defaultTextProvider.selectOption("deepseek");
  await page.waitForTimeout(50);
  expect(defaultsMutations).toHaveLength(0);

  releaseFirstProviderSave();
  await expect.poll(() => defaultsMutations.length).toBe(1);
  await expect(providerModel).toHaveValue("gpt-5-mini");
  await expect(defaultTextProvider).toHaveValue("deepseek");

  await defaultTextProvider.selectOption("openai");
  await secondDefaultsSaveRequested;
  await providerPrompt.fill("Keep both serialized changes.");
  await page.keyboard.press("Tab");
  await page.waitForTimeout(50);
  expect(providerMutations).toHaveLength(1);

  releaseSecondDefaultsSave();
  await expect.poll(() => providerMutations.length).toBe(2);
  await expect(providerModel).toHaveValue("gpt-5-mini");
  await expect(providerPrompt).toHaveValue("Keep both serialized changes.");
  await expect(defaultTextProvider).toHaveValue("openai");
  await expect(page.locator("notification-region")).toHaveText("Provider settings saved");

  const reloadedProfileResponse = page.waitForResponse(`${baseURL}${managementDefaultTenantPath}`);
  await page.reload();
  const reloadedProfile = await (await reloadedProfileResponse).json();
  expect(reloadedProfile.tenant.defaults).toMatchObject({ provider: "openai", model: "gpt-4.1" });
  expect(reloadedProfile.providers.find((provider) => provider.id === "openai")).toMatchObject({
    text_model: "gpt-5-mini",
    system_prompt: "Keep both serialized changes.",
  });
});

test("Settings close waits for the current routing-default autosave", async ({ page }) => {
  let releaseDefaultsSave;
  const defaultsSaveReleased = new Promise((resolve) => {
    releaseDefaultsSave = resolve;
  });
  let defaultsSaveStarted;
  const defaultsSaveRequested = new Promise((resolve) => {
    defaultsSaveStarted = resolve;
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page);
  await page.route(`${baseURL}${managementDefaultTenantPath}/defaults`, async (route) => {
    defaultsSaveStarted();
    await defaultsSaveReleased;
    await route.fallback();
  });

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  await settingsDialog.getByRole("combobox", { name: "Text provider" }).selectOption("deepseek");
  await defaultsSaveRequested;
  await settingsDialog.getByRole("button", { name: "Close" }).click();
  await expect(settingsDialog).toBeVisible();

  releaseDefaultsSave();
  await expect(settingsDialog).toBeHidden();
});

test("failed routing-default autosave retains edits and blocks Settings close", async ({ page }) => {
  let defaultsSaveRequestCount = 0;
  await installAssetRoutes(page);
  await installManagementRoutes(page);
  await page.route(`${baseURL}${managementDefaultTenantPath}/defaults`, async (route) => {
    defaultsSaveRequestCount += 1;
    await route.fulfill({ status: httpInternalServerError, body: "request_failed" });
  });

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const textProvider = settingsDialog.getByRole("combobox", { name: "Text provider" });
  const textModel = settingsDialog.getByRole("combobox", { name: "Text model" }).first();
  await textProvider.selectOption("deepseek");
  await expect.poll(() => defaultsSaveRequestCount).toBe(1);
  await expect(page.locator("#llm-proxy-header .notice")).toHaveText("Request failed");
  await expect(textProvider).toHaveValue("deepseek");
  await expect(textModel).toHaveValue("deepseek-chat");

  await settingsDialog.getByRole("button", { name: "Close" }).click();
  await expect.poll(() => defaultsSaveRequestCount).toBe(2);
  await expect(settingsDialog).toBeVisible();
  await expect(textProvider).toHaveValue("deepseek");
  await expect(textModel).toHaveValue("deepseek-chat");
});

test("session cleanup cancels routing-default autosaves before they can repopulate state", async ({ page }) => {
  let releaseDefaultsSave;
  const defaultsSaveReleased = new Promise((resolve) => {
    releaseDefaultsSave = resolve;
  });
  let defaultsSaveStarted;
  const defaultsSaveRequested = new Promise((resolve) => {
    defaultsSaveStarted = resolve;
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page);
  await page.route(`${baseURL}${managementDefaultTenantPath}/defaults`, async (route) => {
    defaultsSaveStarted();
    await defaultsSaveReleased;
    await route.fallback();
  });

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();
  await page.getByRole("dialog", { name: "Settings" }).getByRole("combobox", { name: "Text provider" }).selectOption("deepseek");
  await defaultsSaveRequested;

  await page.evaluate(() => {
    document.dispatchEvent(new CustomEvent("mpr-ui:auth:unauthenticated"));
  });
  await expect(page.getByRole("heading", { name: "Sign in to manage LLM Proxy keys" })).toBeVisible();
  releaseDefaultsSave();
  await page.waitForTimeout(50);
  await expect(page.locator("#llm-proxy-header .notice")).toHaveText("Authentication required");
  await expect(page.locator('option[value="deepseek"]')).toHaveCount(0);
});

test("reasoning effort is exact to the selected text route and the controls remain responsive", async ({ page }) => {
  const defaultsMutations = [];
  page.on("request", (request) => {
    if (request.url() === `${baseURL}${managementDefaultTenantPath}/defaults`) {
      defaultsMutations.push(request.postDataJSON());
    }
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page);

  await page.goto(`${baseURL}${applicationPath}`);
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
  await expect(reasoningEffort).toBeHidden();
  await textModel.selectOption("gpt-5-mini");
  await expect(unsupportedEffort).toBeHidden();
  await expect(reasoningEffort).toBeVisible();
  await expect(reasoningEffort).toHaveValue("");
  await expect(reasoningEffort.locator("option")).toHaveText(["Not set", "minimal", "low", "medium", "high"]);
  await reasoningEffort.selectOption("high");
  await textModel.selectOption("gpt-4.1");
  await expect(reasoningEffort).toBeHidden();
  await expect(unsupportedEffort).toBeVisible();
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

  await expect.poll(() => defaultsMutations.some((defaults) => (
    defaults.provider === "openai" && defaults.model === "gpt-5.6" && defaults.reasoning_effort === "max"
  ))).toBe(true);
  expect(defaultsMutations.at(-1)).toMatchObject({ provider: "openai", model: "gpt-5.6", reasoning_effort: "max" });
  await expect(settingsDialog.locator(".settings-grid-form")).not.toHaveAttribute("aria-busy", "true");

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

  const reloadedProfileResponse = page.waitForResponse(`${baseURL}${managementDefaultTenantPath}`);
  await page.reload();
  expect((await reloadedProfileResponse).status()).toBe(httpOK);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();
  await expect(settingsDialog.locator(".text-routing-controls").getByRole("combobox", { name: "Reasoning effort" })).toHaveValue("max");
});

test("malformed routing-default profiles become workspace integrity errors", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page, { malformedRoutingDefaults: true });

  await page.goto(`${baseURL}${applicationPath}`);

  await expect(page.getByRole("heading", { name: "Unable to load key workspace" })).toBeVisible();
  await expect(page.getByText("Workspace integrity error")).toBeVisible();
  await expect(page.getByRole("dialog", { name: "Settings" })).toBeHidden();
});

test("invalid persisted routing-default profiles become workspace integrity errors", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page, { profileStatus: 500, profileError: "managed_routing_defaults_invalid" });

  await page.goto(`${baseURL}${applicationPath}`);

  await expect(page.getByRole("heading", { name: "Unable to load key workspace" })).toBeVisible();
  await expect(page.getByText("Workspace integrity error")).toBeVisible();
});

test("dashboard loads only after MPR UI authenticates the user", async ({ page }) => {
  const profileRequests = [];
  page.on("request", (request) => {
    if (request.url() === `${baseURL}${managementDefaultTenantPath}`) {
      profileRequests.push(request);
    }
  });
  await installAssetRoutes(page, { initialAuthStatus: "unauthenticated" });
  await installManagementRoutes(page);

  await page.goto(`${baseURL}${applicationPath}`);

  await expect(page.getByRole("heading", { name: "Sign in to manage LLM Proxy keys" })).toBeVisible();
  expect(profileRequests).toHaveLength(0);
  await page.evaluate(() => window.__llmProxyMprAuthenticate());

  await expect(page.getByRole("heading", { name: "Usage overview" })).toBeVisible();
  await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("37");
  expect(profileRequests).toHaveLength(1);
});

test("startup reconciles MPR UI authentication after the lifecycle event has passed", async ({ page }) => {
  const profileRequests = [];
  page.on("request", (request) => {
    if (request.url() === `${baseURL}${managementDefaultTenantPath}`) {
      profileRequests.push(request);
    }
  });
  await installAssetRoutes(page, { emitInitialAuthEvent: false });
  await installManagementRoutes(page);

  await page.goto(`${baseURL}${applicationPath}`);

  await expect(page.locator("mpr-header")).toHaveAttribute("data-mpr-auth-status", "authenticated");
  await expect(page.getByRole("heading", { name: "Usage overview" })).toBeVisible();
  expect(profileRequests).toHaveLength(1);
});

test("blocked Alpine startup becomes an actionable application error", async ({ page }) => {
  const accountRequests = [];
  const blockedAlpineRequests = [];
  page.on("request", (request) => {
    if (request.url() === `${baseURL}/api/management/account`) {
      accountRequests.push(request);
    }
  });
  page.on("requestfailed", (request) => {
    if (request.url().includes("/alpinejs@3.13.5/dist/module.esm.js")) {
      blockedAlpineRequests.push(request);
    }
  });
  await page.addInitScript(() => {
    window.__llmProxyManagementReadyCount = 0;
    document.addEventListener("llm-proxy:management-ready", () => {
      window.__llmProxyManagementReadyCount += 1;
    });
  });
  await installAssetRoutes(page, { alpineModuleFailure: true });
  await installManagementRoutes(page);

  await page.goto(`${baseURL}${applicationPath}`);

  const failureSurface = page.getByRole("alert");
  await expect(failureSurface).toBeVisible();
  await expect(failureSurface).toBeFocused();
  await expect(failureSurface.getByText("Application startup")).toBeVisible();
  await expect(failureSurface.getByRole("heading", { name: "Unable to open LLM Proxy" })).toBeVisible();
  await expect(failureSurface).toContainText(
    "Your browser could not load the current application files. Allow this site and cdn.jsdelivr.net in browser controls, then reload.",
  );
  await expect(failureSurface.getByRole("button", { name: "Reload LLM Proxy" })).toBeVisible();
  await expect(page.locator("mpr-header")).toHaveAttribute("data-mpr-auth-status", "authenticated");
  await expect.poll(() => page.evaluate(() => window.__llmProxyManagementReadyCount)).toBe(1);
  expect(blockedAlpineRequests).toHaveLength(1);
  expect(accountRequests).toHaveLength(0);
});

test("incompatible cached application module becomes an actionable application error", async ({ page }) => {
  const accountRequests = [];
  page.on("request", (request) => {
    if (request.url() === `${baseURL}/api/management/account`) {
      accountRequests.push(request);
    }
  });
  await page.addInitScript(() => {
    window.__llmProxyManagementReadyCount = 0;
    document.addEventListener("llm-proxy:management-ready", () => {
      window.__llmProxyManagementReadyCount += 1;
    });
  });
  await installAssetRoutes(page, { backendModuleMismatch: true });
  await installManagementRoutes(page);

  await page.goto(`${baseURL}${applicationPath}`);

  const failureSurface = page.getByRole("alert");
  await expect(failureSurface).toBeVisible();
  await expect(failureSurface).toBeFocused();
  await expect(failureSurface.getByText("Application startup")).toBeVisible();
  await expect(failureSurface.getByRole("heading", { name: "Unable to open LLM Proxy" })).toBeVisible();
  await expect(failureSurface).toContainText(
    "Your browser could not load the current application files. Allow this site and cdn.jsdelivr.net in browser controls, then reload.",
  );
  await expect(failureSurface.getByRole("button", { name: "Reload LLM Proxy" })).toBeVisible();
  await expect(page.locator("mpr-header")).toHaveAttribute("data-mpr-auth-status", "authenticated");
  await expect.poll(() => page.evaluate(() => window.__llmProxyManagementReadyCount)).toBe(1);
  expect(accountRequests).toHaveLength(0);
});

test("authenticated profile failures replace loading and signed-out states", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page, { profileStatus: 409 });

  await page.goto(`${baseURL}${applicationPath}`);

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

  await page.goto(`${baseURL}${applicationPath}`);

  const signedOutPanel = page.locator("section.llm-panel").filter({
    has: page.getByRole("heading", { name: "Sign in to manage LLM Proxy keys" }),
  });
  await expect(signedOutPanel).toBeVisible();
  await expect(signedOutPanel.locator("p:not(.eyebrow)")).toHaveCount(0);
});

test("fresh authenticated users receive one client key and must add a provider key before closing Settings", async ({ page }) => {
  const generatedSecret = "llmp_test_fresh_user_secret";
  let secretRequestCount = 0;
  let defaultsRequestCount = 0;
  const providerMutations = [];
  page.on("request", (request) => {
    if (request.url() === `${baseURL}${managementDefaultTenantPath}/secrets` && request.method() === "POST") {
      secretRequestCount += 1;
    }
    if (request.url() === `${baseURL}${managementDefaultTenantPath}/defaults`) {
      defaultsRequestCount += 1;
    }
    if (request.url() === providerKeyEndpointURL("openai") && request.method() === "PUT") {
      providerMutations.push(request.postDataJSON());
    }
  });
  await installClipboardMock(page);
  await installAssetRoutes(page);
  await installManagementRoutes(page, { hasSecret: false, generatedSecret, savedProviderIDs: [] });

  await page.goto(`${baseURL}${applicationPath}`);

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  await expect(settingsDialog).toBeVisible();
  await expect.poll(() => secretRequestCount).toBe(1);
  await page.evaluate(() => {
    document.dispatchEvent(new CustomEvent("mpr-ui:auth:authenticated"));
    document.dispatchEvent(new CustomEvent("mpr-ui:auth:authenticated"));
  });
  await page.waitForTimeout(50);
  expect(secretRequestCount).toBe(1);

  const setupRequirement = settingsDialog.getByRole("alert");
  await expect(setupRequirement).toHaveText("Add at least one provider API key before leaving Settings.");
  const clientKeyInput = settingsDialog.getByRole("textbox", { name: "Key", exact: true });
  await expect(clientKeyInput).toHaveValue("••••••••••••");
  await expect(clientKeyInput).toHaveAttribute("readonly", "");
  await settingsDialog.locator("tenant-access-row").getByRole("button", { name: "Show key", exact: true }).click();
  await expect(clientKeyInput).toHaveValue(generatedSecret);
  expect(await clientKeyInput.evaluate((inputElement) => inputElement.outerHTML)).not.toContain(generatedSecret);
  await settingsDialog.getByRole("button", { name: "Copy key", exact: true }).click();
  expect(await copiedText(page)).toBe(generatedSecret);
  expect(await browserStorageContains(page, generatedSecret)).toBe(false);
  await settingsDialog.getByRole("button", { name: "Hide key", exact: true }).click();

  const closeButton = settingsDialog.getByRole("button", { name: "Close" });
  await closeButton.click();
  await expect(settingsDialog).toBeVisible();
  await expect(setupRequirement).toBeFocused();
  await expect(page.locator("#llm-proxy-header .notice")).toHaveText(
    "Add at least one provider API key before leaving Settings.",
  );
  await page.keyboard.press("Escape");
  await expect(settingsDialog).toBeVisible();
  await expect(setupRequirement).toBeFocused();
  await page.locator("settings-overlay").click({ position: { x: 2, y: 2 } });
  await expect(settingsDialog).toBeVisible();
  await expect(setupRequirement).toBeFocused();

  const providerEditor = settingsDialog.locator("provider-editor");
  await providerEditor.getByRole("combobox", { name: "Provider", exact: true }).selectOption("openai");
  const providerSystemPromptSummary = providerEditor.locator("summary.system-prompt-summary");
  await providerSystemPromptSummary.click();
  const providerSystemPrompt = providerEditor.getByRole("textbox", { name: "System prompt" });
  await providerSystemPrompt.focus();
  await page.keyboard.press("Tab");
  await expect(closeButton).toBeFocused();
  await page.keyboard.press("Shift+Tab");
  await expect(providerSystemPrompt).toBeFocused();

  const providerKeyInput = providerEditor.getByRole("textbox", { name: "OpenAI API key" });
  await providerKeyInput.fill("sk-fresh-openai");
  await page.keyboard.press("Tab");
  await expect.poll(() => providerMutations.length).toBe(1);
  await expect(setupRequirement).toBeHidden();
  await expect(settingsDialog).toBeVisible();
  await expect(providerEditor.getByRole("button", { name: /^(Save|Update) key$/ })).toHaveCount(0);
  expect(providerMutations).toEqual([
    {
      api_key: "sk-fresh-openai",
      text_model: "gpt-4.1",
      system_prompt: "Use concise answers.",
    },
  ]);
  expect(defaultsRequestCount).toBe(0);
  await expect(settingsDialog.getByRole("combobox", { name: "Text provider" })).toHaveValue("openai");

  await closeButton.click();
  await expect(settingsDialog).toBeHidden();
});

test("configured users reach the dashboard without reopening Settings or generating a key", async ({ page }) => {
  let secretRequestCount = 0;
  page.on("request", (request) => {
    if (request.url() === `${baseURL}${managementDefaultTenantPath}/secrets` && request.method() === "POST") {
      secretRequestCount += 1;
    }
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page);

  await page.goto(`${baseURL}${applicationPath}`);

  await expect(page.getByRole("heading", { name: "Usage overview" })).toBeVisible();
  await expect(page.getByRole("dialog", { name: "Settings" })).toBeHidden();
  await page.evaluate(() => {
    document.dispatchEvent(new CustomEvent("mpr-ui:auth:authenticated"));
  });
  await page.waitForTimeout(50);
  expect(secretRequestCount).toBe(0);
});

test("automatically generated client keys never enter request examples", async ({ page }) => {
  const generatedSecret = "llmp_test_generated_secret";
  await installAssetRoutes(page);
  await installManagementRoutes(page, { hasSecret: false, generatedSecret });

  await page.goto(`${baseURL}${applicationPath}`);

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  await settingsDialog.locator(".usage-examples-section summary").click();
  const defaultTextExample = settingsDialog.locator('request-example[data-example-id="default-text"] .usage-snippet');
  const providerV2Example = settingsDialog.locator('request-example[data-example-id="provider-v2"] .usage-snippet');
  await expect(defaultTextExample).toContainText("key=<generated-secret>");
  await expect(providerV2Example).toContainText("key=<generated-secret>");
  await expect(settingsDialog.locator("tenant-access-row").getByRole("textbox", { name: "Key", exact: true })).not.toHaveValue(
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

test("failed automatic client-key creation stays locked and retries through Create key", async ({ page }) => {
  const generatedSecret = "llmp_test_retry_secret";
  let secretRequestCount = 0;
  await installAssetRoutes(page);
  await installManagementRoutes(page, { hasSecret: false });
  await page.route(`${baseURL}${managementDefaultTenantPath}/secrets`, async (route) => {
    secretRequestCount += 1;
    if (secretRequestCount === 1) {
      await route.fulfill({ status: httpInternalServerError, body: "request_failed" });
      return;
    }
    await route.fulfill({
      headers: { "Cache-Control": "no-store" },
      json: {
        secret: generatedSecret,
        profile: managementProfile(false, true),
      },
    });
  });

  await page.goto(`${baseURL}${applicationPath}`);

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  await expect(settingsDialog).toBeVisible();
  await expect(settingsDialog.getByRole("alert")).toHaveText("Create a client key before leaving Settings.");
  await expect(page.locator("#llm-proxy-header .notice")).toHaveText("Request failed");
  const createKeyButton = settingsDialog.getByRole("button", { name: "Create key" });
  await expect(createKeyButton).toBeEnabled();
  await settingsDialog.getByRole("button", { name: "Close" }).click();
  await expect(settingsDialog).toBeVisible();

  await createKeyButton.click();

  expect(secretRequestCount).toBe(2);
  await expect(settingsDialog.getByRole("alert")).toBeHidden();
  await expect(settingsDialog.getByRole("textbox", { name: "Key", exact: true })).toHaveValue("••••••••••••");
  await settingsDialog.getByRole("button", { name: "Close" }).click();
  await expect(settingsDialog).toBeHidden();
});

test("Settings remains locked while automatic client-key creation is pending", async ({ page }) => {
  const generatedSecret = "llmp_test_pending_secret";
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
  await page.route(`${baseURL}${managementDefaultTenantPath}/secrets`, async (route) => {
    generatedSecretRequested();
    await secretResponseReady;
    await route.fulfill({
      headers: { "Cache-Control": "no-store" },
      json: {
        secret: generatedSecret,
        profile: managementProfile(false, true),
      },
    });
  });

  await page.goto(`${baseURL}${applicationPath}`);
  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  await secretRequestStarted;
  await settingsDialog.getByRole("button", { name: "Close" }).click();
  await expect(settingsDialog).toBeVisible();
  await expect(settingsDialog.getByRole("alert")).toHaveText("Create a client key before leaving Settings.");
  fulfillSecretResponse();

  await expect(settingsDialog.getByRole("alert")).toBeHidden();
  await expect(settingsDialog.getByRole("textbox", { name: "Key", exact: true })).toHaveValue("••••••••••••");
  await settingsDialog.getByRole("button", { name: "Close" }).click();
  await expect(settingsDialog).toBeHidden();
});

test("Settings stays inert and keeps a replacement client key available during rotation", async ({ page }) => {
  const replacementSecret = "llmp_test_pending_replacement";
  let releaseSecretReplacement;
  const secretReplacementReleased = new Promise((resolve) => {
    releaseSecretReplacement = resolve;
  });
  let secretReplacementStarted;
  const secretReplacementRequested = new Promise((resolve) => {
    secretReplacementStarted = resolve;
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page, { generatedSecret: replacementSecret });
  await page.route(`${baseURL}${managementDefaultTenantPath}/secrets`, async (route) => {
    if (route.request().method() !== "POST") {
      await route.fallback();
      return;
    }
    secretReplacementStarted();
    await secretReplacementReleased;
    await route.fallback();
  });

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();
  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const tenantAccessRow = settingsDialog.locator("tenant-access-row");
  const replaceKeyButton = tenantAccessRow.getByRole("button", { name: "Replace key" });

  await replaceKeyButton.click();
  const replacementDialog = page.getByRole("alertdialog", { name: "Replace client key?" });
  await expect(replacementDialog).toContainText(
    "The current key will stop working immediately. Copy the replacement now; it cannot be shown again.",
  );
  await replacementDialog.getByRole("button", { name: "Cancel" }).click();
  await expect(replacementDialog).toBeHidden();
  await expect(replaceKeyButton).toBeFocused();
  await replaceKeyButton.click();
  await replacementDialog.getByRole("button", { name: "Replace key" }).click();
  await secretReplacementRequested;
  await page.keyboard.press("Escape");
  await expect(settingsDialog).toBeVisible();
  await expect(replacementDialog).toBeVisible();
  expect(await settingsDialog.evaluate((dialogElement) => dialogElement.inert)).toBe(true);

  const replacementResponse = page.waitForResponse(
    (response) => response.url() === `${baseURL}${managementDefaultTenantPath}/secrets` && response.request().method() === "POST",
  );
  releaseSecretReplacement();
  await replacementResponse;
  await expect(settingsDialog).toBeVisible();
  await expect(replacementDialog).toBeHidden();
  await expect(settingsDialog.getByRole("button", { name: "Close" })).toBeEnabled();
  const clientKeyInput = tenantAccessRow.getByRole("textbox", { name: "Key", exact: true });
  await expect(clientKeyInput).toHaveValue("••••••••••••");
  await tenantAccessRow.getByRole("button", { name: "Show key", exact: true }).click();
  await expect(clientKeyInput).toHaveValue(replacementSecret);
  expect(await browserStorageContains(page, replacementSecret)).toBe(false);

  await settingsDialog.getByRole("button", { name: "Close" }).click();
  await expect(settingsDialog).toBeHidden();
});

test("pending last-provider removal enforces mandatory Settings before close", async ({ page }) => {
  let releaseProviderRemoval;
  const providerRemovalReleased = new Promise((resolve) => {
    releaseProviderRemoval = resolve;
  });
  let providerRemovalStarted;
  const providerRemovalRequested = new Promise((resolve) => {
    providerRemovalStarted = resolve;
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page, { savedProviderIDs: ["openai"] });
  await page.route(providerKeyEndpointURL("openai"), async (route) => {
    if (route.request().method() !== "DELETE") {
      await route.fallback();
      return;
    }
    providerRemovalStarted();
    await providerRemovalReleased;
    await route.fallback();
  });

  await page.goto(`${baseURL}${applicationPath}`);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();
  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const closeSettings = settingsDialog.getByRole("button", { name: "Close" });
  const providerEditor = settingsDialog.locator("provider-editor");
  await providerEditor.getByRole("button", { name: "Remove provider key and settings" }).click();
  await page.getByRole("alertdialog", { name: "Remove provider key?" }).getByRole("button", { name: "Remove key" }).click();
  await providerRemovalRequested;
  await closeSettings.click();
  await expect(settingsDialog).toBeVisible();
  releaseProviderRemoval();
  await expect(settingsDialog.getByRole("alert")).toHaveText("Add at least one provider API key before leaving Settings.");
  await expect(settingsDialog).toBeVisible();
});

test("session cleanup cancels generated client keys before they can restore state", async ({ page }) => {
  const lateGeneratedSecret = "llmp_test_late_session_secret";
  const currentGeneratedSecret = "llmp_test_current_session_secret";
  let secretRequestCount = 0;
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
  await page.route(`${baseURL}${managementDefaultTenantPath}/secrets`, async (route) => {
    secretRequestCount += 1;
    if (secretRequestCount > 1) {
      await route.fulfill({
        headers: { "Cache-Control": "no-store" },
        json: {
          secret: currentGeneratedSecret,
          profile: managementProfile(false, true),
        },
      });
      return;
    }
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

  await page.goto(`${baseURL}${applicationPath}`);
  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  await secretRequestStarted;
  await page.evaluate(() => {
    document.dispatchEvent(new CustomEvent("mpr-ui:auth:unauthenticated"));
  });
  await expect(page.getByRole("heading", { name: "Sign in to manage LLM Proxy keys" })).toBeVisible();
  fulfillSecretResponse();
  await page.waitForTimeout(50);
  await expect(page.locator("body")).not.toContainText(lateGeneratedSecret);
  expect(await browserStorageContains(page, lateGeneratedSecret)).toBe(false);

  await page.evaluate(() => window.__llmProxyMprAuthenticate());
  await expect(page.getByRole("heading", { name: "Usage overview" })).toBeVisible();
  await expect(settingsDialog).toBeVisible();
  expect(secretRequestCount).toBe(2);
  await settingsDialog.locator("tenant-access-row").getByRole("button", { name: "Show key", exact: true }).click();
  await expect(settingsDialog.getByRole("textbox", { name: "Key", exact: true })).toHaveValue(currentGeneratedSecret);
  await expect(settingsDialog).not.toContainText(lateGeneratedSecret);
});

test("tenant access stays compact while one-time client keys support confirmed replacement", async ({ page }) => {
  const generatedSecret = "llmp_test_generated_secret";
  await installClipboardMock(page);
  await installAssetRoutes(page);

  for (const viewport of settingsLayerViewports) {
    await installManagementRoutes(page, { hasSecret: false, generatedSecret });
    await page.setViewportSize({ width: viewport.width, height: viewport.height });
    await page.goto(`${baseURL}${applicationPath}`);

    const settingsDialog = page.getByRole("dialog", { name: "Settings" });
    await expect(settingsDialog).toBeVisible();

    const tenantAccessRow = settingsDialog.getByRole("group", { name: "Tenant access" });
    const tenantSelector = tenantAccessRow.getByRole("combobox", { name: "Tenant" });
    const renameTenantButton = tenantAccessRow.getByRole("button", { name: "Rename" });
    const deleteTenantButton = tenantAccessRow.getByRole("button", { name: "Delete tenant" });
    const createTenantButton = tenantAccessRow.getByRole("button", { name: "Create tenant" });
    const deleteTenantIcon = deleteTenantButton.locator(".material-symbols-outlined");
    const createTenantSymbol = createTenantButton.locator(".tenant-create-symbol");
    const clientKey = tenantAccessRow.locator("client-access-key");
    const keyLabel = clientKey.locator(".eyebrow");
    const clientKeyRow = clientKey.locator("client-key-row");
    const clientKeyInput = tenantAccessRow.getByRole("textbox", { name: "Key", exact: true });
    const visibilityButton = tenantAccessRow.getByRole("button", { name: "Show key", exact: true });
    const copyButton = tenantAccessRow.getByRole("button", { name: "Copy key", exact: true });
    const copyIcon = copyButton.locator(".material-symbols-outlined");
    const visibilitySymbols = visibilityButton.locator(".material-symbols-outlined");
    const replaceKeyButton = tenantAccessRow.getByRole("button", { name: "Replace key", exact: true });
    const replaceKeyIcon = replaceKeyButton.locator(".material-symbols-outlined");
    const replaceKeyLabel = replaceKeyButton.locator(".tenant-access-action-label");
    await expect(tenantSelector.locator("option:checked")).toHaveText("Default");
    await expect(renameTenantButton).toBeVisible();
    await expect(deleteTenantButton).toBeDisabled();
    await expect(createTenantButton).toBeVisible();
    await expect(tenantAccessRow.locator(".tenant-delete + .tenant-create")).toHaveCount(1);
    await expect(tenantAccessRow.getByRole("button", { name: "Revoke key" })).toHaveCount(0);
    await expect(clientKeyInput).toHaveValue("••••••••••••");
    await expect(clientKeyInput).toHaveAttribute("readonly", "");
    await expect(replaceKeyButton).toBeVisible();
    await expect(replaceKeyButton).toHaveClass(/icon-button/);
    await expect(replaceKeyButton).toHaveAttribute("title", "Replace key");
    await expect(replaceKeyButton.locator("svg")).toHaveCount(0);
    await expect(replaceKeyIcon).toHaveAttribute("aria-hidden", "true");
    await expect(replaceKeyIcon).toHaveText("key");
    await expect(replaceKeyIcon).toBeVisible();
    await expect(replaceKeyLabel).toHaveText("Replace key");
    await expect(deleteTenantIcon).toHaveText("delete");
    await expect(deleteTenantIcon).toHaveAttribute("aria-hidden", "true");
    await expect(createTenantButton).toHaveClass(/icon-only/);
    await expect(createTenantButton).not.toHaveClass(/icon-button/);
    await expect(createTenantButton).toHaveAttribute("title", "Create tenant");
    await expect(createTenantButton).toHaveAttribute("aria-label", "Create tenant");
    await expect(createTenantButton.locator(".tenant-access-action-label")).toHaveCount(0);
    await expect(createTenantSymbol).toHaveText("+");
    await expect(createTenantSymbol).toHaveAttribute("aria-hidden", "true");
    await expect(createTenantSymbol).toBeVisible();
    const deleteTenantIconFontSize = await deleteTenantIcon.evaluate(
      (iconElement) => Number.parseFloat(window.getComputedStyle(iconElement).fontSize),
    );
    const createTenantSymbolFontSize = await createTenantSymbol.evaluate(
      (symbolElement) => Number.parseFloat(window.getComputedStyle(symbolElement).fontSize),
    );
    expect(createTenantSymbolFontSize).toBeGreaterThan(deleteTenantIconFontSize);
    if (viewport.name === "desktop") {
      await expect(replaceKeyLabel).toBeVisible();
    } else {
      await expect(replaceKeyLabel).toBeHidden();
    }
    await expect(visibilityButton).toHaveAttribute("aria-pressed", "false");
    await expect(visibilitySymbols).toHaveCount(2);
    await expect(visibilitySymbols.nth(0)).toHaveText("visibility");
    await expect(visibilitySymbols.nth(0)).toBeVisible();
    await expect(visibilitySymbols.nth(1)).toHaveText("visibility_off");
    await expect(visibilitySymbols.nth(1)).toBeHidden();
    await expect(copyButton).toHaveAttribute("title", "Copy key");
    await expect(copyIcon).toHaveCount(1);
    await expect(copyIcon).toHaveAttribute("aria-hidden", "true");
    await expect(copyIcon).toHaveText("content_copy");
    await expect(copyIcon).toBeVisible();
    await expect(copyButton.locator("svg")).toHaveCount(0);
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

    const tenantAccessRowBox = await tenantAccessRow.boundingBox();
    const tenantSelectorBox = await tenantSelector.boundingBox();
    const renameTenantButtonBox = await renameTenantButton.boundingBox();
    const deleteTenantButtonBox = await deleteTenantButton.boundingBox();
    const createTenantButtonBox = await createTenantButton.boundingBox();
    const clientKeyBox = await clientKey.boundingBox();
    const clientKeyInputBox = await clientKeyInput.boundingBox();
    const replaceKeyButtonBox = await replaceKeyButton.boundingBox();
    const replaceKeyIconBox = await replaceKeyIcon.boundingBox();
    const keyLabelBox = await keyLabel.boundingBox();
    const clientKeyRowBox = await clientKeyRow.boundingBox();
    if (
      !tenantAccessRowBox ||
      !tenantSelectorBox ||
      !renameTenantButtonBox ||
      !deleteTenantButtonBox ||
      !createTenantButtonBox ||
      !clientKeyBox ||
      !clientKeyInputBox ||
      !replaceKeyButtonBox ||
      !replaceKeyIconBox ||
      !keyLabelBox ||
      !clientKeyRowBox
    ) {
      throw new Error(`client_access_geometry_missing:${viewport.name}`);
    }
    expect(tenantAccessRowBox.x).toBeGreaterThanOrEqual(settingsDialogBox.x);
    expect(tenantAccessRowBox.x + tenantAccessRowBox.width).toBeLessThanOrEqual(
      settingsDialogBox.x + settingsDialogBox.width,
    );
    for (const controlBox of [
      tenantSelectorBox,
      renameTenantButtonBox,
      clientKeyBox,
      replaceKeyButtonBox,
      deleteTenantButtonBox,
      createTenantButtonBox,
    ]) {
      expect(controlBox.x).toBeGreaterThanOrEqual(tenantAccessRowBox.x);
      expect(controlBox.x + controlBox.width).toBeLessThanOrEqual(
        tenantAccessRowBox.x + tenantAccessRowBox.width,
      );
    }
    expect(clientKeyBox.x).toBeGreaterThan(tenantAccessRowBox.x);
    expect(keyLabelBox.x + keyLabelBox.width).toBeLessThanOrEqual(clientKeyRowBox.x);
    expect(
      Math.abs(keyLabelBox.y + keyLabelBox.height / 2 - (clientKeyRowBox.y + clientKeyRowBox.height / 2)),
    ).toBeLessThanOrEqual(1);
    if (viewport.name === "desktop") {
      expect(tenantAccessRowBox.height).toBeLessThanOrEqual(tenantAccessDesktopMaxHeight);
      expect(clientKeyInputBox.x + clientKeyInputBox.width).toBeLessThanOrEqual(replaceKeyButtonBox.x);
      const desktopControlCenters = [
        tenantSelectorBox,
        renameTenantButtonBox,
        clientKeyInputBox,
        replaceKeyButtonBox,
        deleteTenantButtonBox,
        createTenantButtonBox,
      ].map((box) => box.y + box.height / 2);
      expect(Math.max(...desktopControlCenters) - Math.min(...desktopControlCenters)).toBeLessThanOrEqual(2);
      expect(replaceKeyButtonBox.width).toBeGreaterThan(30);
    } else {
      expect(tenantAccessRowBox.height).toBeLessThanOrEqual(92);
      expect(
        Math.abs(tenantSelectorBox.y + tenantSelectorBox.height / 2 - (renameTenantButtonBox.y + renameTenantButtonBox.height / 2)),
      ).toBeLessThanOrEqual(1);
      expect(
        Math.abs(clientKeyRowBox.y + clientKeyRowBox.height / 2 - (replaceKeyButtonBox.y + replaceKeyButtonBox.height / 2)),
      ).toBeLessThanOrEqual(1);
      expect(
        Math.abs(clientKeyRowBox.y + clientKeyRowBox.height / 2 - (deleteTenantButtonBox.y + deleteTenantButtonBox.height / 2)),
      ).toBeLessThanOrEqual(1);
      expect(
        Math.abs(clientKeyRowBox.y + clientKeyRowBox.height / 2 - (createTenantButtonBox.y + createTenantButtonBox.height / 2)),
      ).toBeLessThanOrEqual(1);
      expect(tenantSelectorBox.y + tenantSelectorBox.height).toBeLessThanOrEqual(clientKeyRowBox.y);
      expect(replaceKeyButtonBox.width).toBe(30);
    }
    expect(replaceKeyButtonBox.width).toBeLessThanOrEqual(120);
    expect(createTenantButtonBox.width).toBe(30);
    expect(createTenantButtonBox.x).toBeGreaterThanOrEqual(deleteTenantButtonBox.x + deleteTenantButtonBox.width);
    expect(replaceKeyIconBox.x).toBeGreaterThanOrEqual(replaceKeyButtonBox.x);
    expect(replaceKeyButtonBox.x + replaceKeyButtonBox.width).toBeLessThanOrEqual(
      tenantAccessRowBox.x + tenantAccessRowBox.width,
    );

    await visibilityButton.click();
    await expect(clientKeyInput).toHaveValue(generatedSecret);
    const hideKeyButton = tenantAccessRow.getByRole("button", { name: "Hide key", exact: true });
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
    await expect(tenantAccessRow.getByText("Saved; replace to reveal a new key.")).toBeVisible();
    await expect(clientKeyInput).toBeHidden();
    await expect(tenantAccessRow.getByRole("button", { name: "Show key", exact: true })).toBeHidden();
    await expect(tenantAccessRow.getByRole("button", { name: "Copy key", exact: true })).toBeHidden();

    await replaceKeyButton.click();
    const replacementDialog = page.getByRole("alertdialog", { name: "Replace client key?" });
    await expect(replacementDialog).toBeVisible();
    await expect(replacementDialog.getByRole("button", { name: "Cancel" })).toBeFocused();
    await replacementDialog.getByRole("button", { name: "Replace key" }).click();
    await expect(replacementDialog).toBeHidden();
    await expect(clientKeyInput).toBeVisible();
    await expect(clientKeyInput).toHaveValue("••••••••••••");
    await expect(page.locator("#llm-proxy-header .notice")).toHaveText("Key replaced");
    await expect(copyButton).toBeFocused();
  }
});

test("settings modal remains usable on narrow screens", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 780 });
  await installAssetRoutes(page);
  await installManagementRoutes(page);

  await page.goto(`${baseURL}${applicationPath}`);
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
    await page.goto(`${baseURL}${applicationPath}`);
    const usageTenantSelector = page.getByRole("combobox", { name: "Usage tenant" });
    const usageTenantSelectorBox = await usageTenantSelector.boundingBox();
    const allIntervalButtonBox = await page.getByRole("button", { name: "ALL", exact: true }).boundingBox();
    if (!usageTenantSelectorBox || !allIntervalButtonBox) {
      throw new Error(`usage_tenant_geometry_missing:${viewport.name}`);
    }
    expect(usageTenantSelectorBox.x).toBeGreaterThanOrEqual(0);
    expect(usageTenantSelectorBox.x + usageTenantSelectorBox.width).toBeLessThanOrEqual(viewport.width);
    await expect(page.locator(".usage-tenant-control + .usage-interval-control")).toHaveCount(1);
    if (viewport.name === "desktop") {
      expect(usageTenantSelectorBox.x + usageTenantSelectorBox.width).toBeLessThanOrEqual(allIntervalButtonBox.x);
    } else {
      expect(usageTenantSelectorBox.y).toBeLessThanOrEqual(allIntervalButtonBox.y);
    }
    await page.getByTestId("avatar-menu").click();
    await page.getByTestId("avatar-menu-item").nth(0).click();

    const settingsDialog = page.getByRole("dialog", { name: "Settings" });
    await expect(settingsDialog).toBeVisible();
    await expect(settingsDialog.getByRole("button", { name: "Close" })).toBeVisible();
    const tenantAccess = settingsDialog.getByRole("group", { name: "Tenant access" });
    const createTenantButton = tenantAccess.getByRole("button", { name: "Create tenant" });
    const tenantAccessBox = await tenantAccess.boundingBox();
    const createTenantButtonBox = await createTenantButton.boundingBox();
    const settingsDialogBox = await settingsDialog.boundingBox();
    if (!tenantAccessBox || !createTenantButtonBox || !settingsDialogBox) {
      throw new Error(`settings_tenant_geometry_missing:${viewport.name}`);
    }
    expect(tenantAccessBox.x).toBeGreaterThanOrEqual(settingsDialogBox.x);
    expect(tenantAccessBox.x + tenantAccessBox.width).toBeLessThanOrEqual(
      settingsDialogBox.x + settingsDialogBox.width,
    );
    expect(createTenantButtonBox.x).toBeGreaterThanOrEqual(tenantAccessBox.x);
    expect(createTenantButtonBox.x + createTenantButtonBox.width).toBeLessThanOrEqual(
      tenantAccessBox.x + tenantAccessBox.width,
    );

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
    await page.goto(`${baseURL}${applicationPath}`);

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
    await page.goto(`${baseURL}${applicationPath}`);

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
  await page.goto(`${baseURL}${applicationPath}`);

  const notificationRegion = page.locator("#llm-proxy-header notification-region");
  const notice = notificationRegion.locator(".notice");
  const refresh = page.getByRole("button", { name: "Refresh" });
  const requests = page.locator("usage-metrics usage-card").first().locator("strong");
  await expect(notice).toHaveText("Workspace loaded");
  await page.clock.fastForward(9_000);
  await expect(notificationRegion).toBeVisible();
  await page.clock.fastForward(1_000);
  await expect(notificationRegion).toBeHidden();

  await refresh.click();
  await expect(notice).toHaveText("Usage refreshed");
  await page.clock.fastForward(5_000);
  await installUsageResponse(page, httpOK, managementUsage("30d", {
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
  await page.goto(`${baseURL}${applicationPath}`);

  const notificationRegion = page.locator("#llm-proxy-header notification-region");
  await expect(notificationRegion.locator(".notice")).toHaveText("Authentication required");
  await expectHeaderNoticeSignInGeometry(page);
  const noticeClockPauseTime = await page.evaluate(
    (pauseLeadMilliseconds) => Date.now() + pauseLeadMilliseconds,
    noticeClockPauseLeadMilliseconds,
  );
  await page.clock.pauseAt(noticeClockPauseTime);
  await page.clock.runFor(noticeClockPreDeadlineAdvanceMilliseconds);
  await expect(notificationRegion).toBeVisible();
  await page.clock.runFor(noticeClockPostDeadlineAdvanceMilliseconds);
  await expect(notificationRegion).toBeHidden();
  await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
});

test("header brand uses the local logo before its title without crowding the notice or avatar", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page);

  for (const viewport of settingsLayerViewports) {
    await page.setViewportSize({ width: viewport.width, height: viewport.height });
    await page.goto(`${baseURL}${applicationPath}`);

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

  await page.goto(`${baseURL}${applicationPath}`);

  await expect(page.getByRole("heading", { name: "Usage overview" })).toBeVisible();
  await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("0");
  await expect(page.getByText("Request failed")).toBeVisible();

  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").nth(0).click();
  await expect(page.getByRole("dialog", { name: "Settings" })).toBeVisible();
});

test("usage refresh clears stale metrics when summary reload fails", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page);

  await page.goto(`${baseURL}${applicationPath}`);

  await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("37");
  await page.unroute(usageRequestPattern());
  await page.route(usageRequestPattern(), async (route) => {
    await route.fulfill({ status: httpInternalServerError, json: { error: "usage_failed" } });
  });
  await page.getByRole("button", { name: "Refresh" }).click();

  await expect(page.getByText("Request failed")).toBeVisible();
  await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("0");
  await expect(page.locator("usage-chart-panel").first()).toContainText("No usage recorded");
});

test("admin menu opens all users dashboard", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page, { admin: true });

  await page.goto(`${baseURL}${applicationPath}`);

  await page.getByTestId("avatar-menu").click();
  await expect(page.getByTestId("avatar-menu-item").nth(0)).toHaveText("Admin");
  await expect(page.getByTestId("avatar-menu-item").nth(1)).toHaveText("Settings");

  await page.getByTestId("avatar-menu-item").nth(0).click();

  await expect(page.getByRole("heading", { name: "All users" })).toBeVisible();
  const ownerCard = page.locator("admin-user-card").filter({ hasText: "owner@example.com" });
  await expect(ownerCard).toContainText("2 tenants");
  await expect(ownerCard.locator("admin-tenant-card")).toHaveCount(2);
  await expect(ownerCard).toContainText("Default");
  await expect(ownerCard).toContainText("Research");
  await expect(ownerCard).toContainText("37");
  await expect(ownerCard).toContainText("5");
  await expect(page.locator("admin-user-card").filter({ hasText: "teammate@example.com" })).toContainText("0");
  await expect(page.locator("admin-dashboard")).not.toContainText("sk-");
  await expect(page.locator("admin-dashboard")).not.toContainText("masked_key");
  await expect(page.getByRole("button", { name: /failed request/ })).toHaveCount(0);
  await expect(page.getByRole("dialog", { name: "Failed request details" })).toHaveCount(0);
});

/**
 * @param {import("@playwright/test").Page} page
 * @param {{ initialAuthStatus?: "authenticated" | "unauthenticated", emitInitialAuthEvent?: boolean, alpineModuleFailure?: boolean, backendModuleMismatch?: boolean }} options
 * @returns {Promise<void>}
 */
async function installAssetRoutes(page, options = {}) {
  await page.route("https://loopaware.mprlab.com/**", async (route) =>
    route.fulfill({ body: "", contentType: "application/javascript" }),
  );
  await page.route("https://accounts.google.com/**", async (route) => route.abort());
  await page.route("**/alpinejs@3.13.5/dist/module.esm.js", async (route) => {
    if (options.alpineModuleFailure) {
      await route.abort("blockedbyclient");
      return;
    }
    await fulfillFile(route, "node_modules/alpinejs/dist/module.esm.js", "application/javascript");
  });
  if (options.backendModuleMismatch) {
    await page.route("**/assets/llm-proxy/js/core/backendClient.js*", async (route) =>
      route.fulfill({ body: "export {};", contentType: "application/javascript" }),
    );
  }
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
 * @param {import("@playwright/test").Locator} footer
 * @returns {Promise<void>}
 */
async function expectCompactFooterGeometry(footer) {
  const geometry = await footer.evaluate((footerElement) => ({
    clientWidth: footerElement.clientWidth,
    height: footerElement.getBoundingClientRect().height,
    scrollWidth: footerElement.scrollWidth,
  }));
  expect(geometry.height).toBeLessThanOrEqual(compactLandingFooterMaxHeight);
  expect(geometry.scrollWidth).toBeLessThanOrEqual(geometry.clientWidth);
}

/**
 * @param {import("@playwright/test").Page} page
 * @returns {Promise<void>}
 */
async function expectCenteredValueStrip(page) {
  const facts = await page.locator(".value-strip").evaluate((stripElement) => {
    const gridElement = stripElement.querySelector(".value-strip__grid");
    if (!gridElement) {
      throw new Error("landing_value_strip_grid_missing");
    }
    const stripStyle = getComputedStyle(stripElement);
    const gridStyle = getComputedStyle(gridElement);
    const gridRect = gridElement.getBoundingClientRect();
    return {
      gridBackground: gridStyle.backgroundColor,
      gridBorderTopWidth: gridStyle.borderTopWidth,
      itemCount: gridElement.querySelectorAll(":scope > p").length,
      leftGap: gridRect.left,
      rightGap: document.documentElement.clientWidth - gridRect.right,
      stripBackground: stripStyle.backgroundColor,
      stripBorderTopWidth: stripStyle.borderTopWidth,
    };
  });
  expect(facts.itemCount).toBe(3);
  expect(facts.stripBorderTopWidth).toBe("0px");
  expect(facts.gridBorderTopWidth).toBe("1px");
  expect(facts.stripBackground).not.toBe(facts.gridBackground);
  expect(facts.leftGap).toBeGreaterThan(0);
  expect(facts.rightGap).toBeGreaterThan(0);
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
async function installUsageResponse(page, status, usage = managementUsage("30d")) {
  await page.unroute(usageRequestPattern());
  await page.route(usageRequestPattern(), async (route) => {
    await route.fulfill({ status, json: usage });
  });
}

/**
 * @returns {string}
 */
function usageRequestPattern() {
  return `${baseURL}/api/management/usage?interval=*`;
}

/**
 * @returns {string}
 */
function usageFailuresRequestPattern() {
  return `${baseURL}/api/management/usage/failures?*`;
}

/**
 * @param {import("@playwright/test").Page} page
 * @param {object} response
 * @returns {Promise<void>}
 */
async function installUsageFailuresResponse(page, response) {
  await page.route(usageFailuresRequestPattern(), async (route) => {
    const requestURL = new URL(route.request().url());
    expect(requestURL.searchParams.get("interval")).toBe(response.interval);
    expect(requestURL.searchParams.get("limit")).toBe("25");
    expect(requestURL.searchParams.getAll("interval")).toHaveLength(1);
    expect(requestURL.searchParams.getAll("limit")).toHaveLength(1);
    await route.fulfill({ headers: { "Cache-Control": "no-store" }, json: response });
  });
}

/**
 * @param {import("@playwright/test").Page} page
 * @param {{ profiles?: object[], usageRequests?: Record<string, number>, accountUsageRequests?: number, admin?: boolean }} [options]
 * @returns {Promise<{
 *   order: string[],
 *   profiles: Map<string, any>,
 *   providerKeys: Map<string, Record<string, string>>,
 *   requests: Array<{ method: string, path: string }>
 * }>}
 */
async function installMultiTenantRoutes(page, options = {}) {
  const initialProfiles = options.profiles || [
    managementTenantProfile("tenant_1", "Default"),
    managementTenantProfile("tenant_2", "Research"),
  ];
  const state = {
    order: initialProfiles.map((profile) => profile.tenant.id),
    profiles: new Map(initialProfiles.map((profile) => [profile.tenant.id, profile])),
    providerKeys: new Map(initialProfiles.map((profile) => [
      profile.tenant.id,
      {
        openai: `sk-${profile.tenant.id}-openai`,
        deepseek: `sk-${profile.tenant.id}-deepseek`,
        meta: `sk-${profile.tenant.id}-meta`,
      },
    ])),
    requests: [],
  };
  let createdTenantSequence = initialProfiles.length + 1;
  const tenantRoutePattern = new RegExp(`${baseURL.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}/api/management/tenants/[^/]+(?:/.*)?$`);

  await page.route(tenantRoutePattern, async (route) => {
    const request = route.request();
    const requestURL = new URL(request.url());
    const path = requestURL.pathname;
    state.requests.push({ method: request.method(), path });
    const relativePath = path.slice("/api/management/tenants/".length);
    const [tenantID, resource, providerID, action] = relativePath.split("/");
    const profile = state.profiles.get(tenantID);
    if (!profile) {
      await route.fulfill({ status: 404 });
      return;
    }
    if (!resource) {
      if (request.method() === "GET") {
        await route.fulfill({ headers: { "Cache-Control": "no-store" }, json: profile });
        return;
      }
      if (request.method() === "PUT") {
        const { name } = request.postDataJSON();
        if (state.order.some((candidateID) => (
          candidateID !== tenantID &&
          state.profiles.get(candidateID).tenant.name.toLocaleLowerCase("en-US") === String(name).trim().toLocaleLowerCase("en-US")
        ))) {
          await route.fulfill({ status: 409, body: "managed_tenant_name_conflict" });
          return;
        }
        profile.tenant.name = String(name).trim();
        profile.tenant.updated_at = "2026-07-25T12:00:00Z";
        await route.fulfill({ headers: { "Cache-Control": "no-store" }, json: profile });
        return;
      }
      if (request.method() === "DELETE") {
        if (state.order.length === 1) {
          await route.fulfill({ status: 409, body: "managed_final_tenant_deletion" });
          return;
        }
        state.order = state.order.filter((candidateID) => candidateID !== tenantID);
        state.profiles.delete(tenantID);
        state.providerKeys.delete(tenantID);
        await route.fulfill({ status: 204, body: "" });
        return;
      }
    }
    if (resource === "usage" && !providerID && request.method() === "GET") {
      const interval = requestURL.searchParams.get("interval") || "";
      await route.fulfill({
        json: managementUsage(interval, {
          requests: options.usageRequests?.[tenantID] ?? (tenantID === "tenant_1" ? 37 : 7),
        }),
      });
      return;
    }
    if (resource === "secrets") {
      if (request.method() === "POST") {
        profile.tenant.has_secret = true;
        await route.fulfill({
          headers: { "Cache-Control": "no-store" },
          json: { secret: `llmp_${tenantID}_generated`, profile },
        });
        return;
      }
    }
    if (resource === "defaults" && request.method() === "PUT") {
      profile.tenant.defaults = request.postDataJSON();
      await route.fulfill({ headers: { "Cache-Control": "no-store" }, json: profile });
      return;
    }
    if (resource === "provider-keys") {
      const provider = profile.providers.find((candidateProvider) => candidateProvider.id === providerID);
      if (!provider) {
        await route.fulfill({ status: 404 });
        return;
      }
      const providerKeys = state.providerKeys.get(tenantID);
      if (action === "reveal" && request.method() === "POST") {
        if (!provider.has_key || !providerKeys[providerID]) {
          await route.fulfill({ status: 404 });
          return;
        }
        await route.fulfill({
          headers: { "Cache-Control": "no-store" },
          json: { api_key: providerKeys[providerID] },
        });
        return;
      }
      if (!action && request.method() === "PUT") {
        const settings = request.postDataJSON();
        if (settings.api_key) {
          providerKeys[providerID] = settings.api_key;
        }
        provider.has_key = true;
        provider.masked_key = "saved";
        provider.text_model = settings.text_model;
        provider.system_prompt = settings.system_prompt;
        reconcileManagementProfileRoutingDefaults(profile);
        await route.fulfill({ headers: { "Cache-Control": "no-store" }, json: profile });
        return;
      }
      if (!action && request.method() === "DELETE") {
        delete providerKeys[providerID];
        provider.has_key = false;
        delete provider.masked_key;
        reconcileManagementProfileRoutingDefaults(profile);
        await route.fulfill({ headers: { "Cache-Control": "no-store" }, json: profile });
        return;
      }
    }
    await route.fulfill({ status: httpInternalServerError });
  });

  await page.route(`${baseURL}/api/management/tenants`, async (route) => {
    const request = route.request();
    state.requests.push({ method: request.method(), path: new URL(request.url()).pathname });
    if (request.method() !== "POST") {
      await route.fulfill({ status: 405 });
      return;
    }
    const name = String(request.postDataJSON().name || "").trim();
    if (state.order.some((tenantID) => (
      state.profiles.get(tenantID).tenant.name.toLocaleLowerCase("en-US") === name.toLocaleLowerCase("en-US")
    ))) {
      await route.fulfill({ status: 409, body: "managed_tenant_name_conflict" });
      return;
    }
    const tenantID = `tenant_${createdTenantSequence}`;
    createdTenantSequence += 1;
    const profile = managementTenantProfile(tenantID, name, false);
    for (const provider of profile.providers) {
      provider.has_key = false;
      delete provider.masked_key;
    }
    reconcileManagementProfileRoutingDefaults(profile);
    state.order.push(tenantID);
    state.profiles.set(tenantID, profile);
    state.providerKeys.set(tenantID, {});
    await route.fulfill({ status: 201, headers: { "Cache-Control": "no-store" }, json: profile });
  });

  await page.route(`${baseURL}/api/management/account`, async (route) => {
    state.requests.push({ method: route.request().method(), path: new URL(route.request().url()).pathname });
    await route.fulfill({
      headers: { "Cache-Control": "no-store" },
      json: {
        user: {
          id: "user_1",
          email: "owner@example.com",
          display_name: "Owner",
          is_admin: options.admin || false,
        },
        tenants: state.order.map((tenantID) => {
          const tenant = state.profiles.get(tenantID).tenant;
          return {
            id: tenant.id,
            name: tenant.name,
            has_secret: tenant.has_secret,
            created_at: tenant.created_at,
            updated_at: tenant.updated_at,
          };
        }),
      },
    });
  });

  await page.route(usageRequestPattern(), async (route) => {
    const requestURL = new URL(route.request().url());
    const interval = requestURL.searchParams.get("interval") || "";
    const accountRequests = options.accountUsageRequests ?? state.order.reduce(
      (total, tenantID) => total + (options.usageRequests?.[tenantID] ?? (tenantID === "tenant_1" ? 37 : 7)),
      0,
    );
    state.requests.push({ method: route.request().method(), path: requestURL.pathname });
    await route.fulfill({
      headers: { "Cache-Control": "no-store" },
      json: managementUsage(interval, { requests: accountRequests }),
    });
  });

  await page.route(`${baseURL}/api/management/admin/users`, async (route) => {
    await route.fulfill({ json: managementAdminUsers() });
  });
  return state;
}

/**
 * @param {import("@playwright/test").Page} page
 * @param {{ usageStatus?: number, admin?: boolean, hasSecret?: boolean, generatedSecret?: string, profileStatus?: number, profileStatuses?: number[], profileError?: string, malformedRoutingDefaults?: boolean, maskedKeys?: Record<string, string>, providerKeys?: Record<string, string>, savedProviderIDs?: string[] }} options
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
  if (options.savedProviderIDs) {
    for (const provider of profile.providers) {
      provider.has_key = options.savedProviderIDs.includes(provider.id);
      if (provider.has_key) {
        provider.masked_key = provider.masked_key || "sk-...saved";
      } else {
        delete provider.masked_key;
      }
    }
    reconcileManagementProfileRoutingDefaults(profile);
  }
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
  await page.route(`${baseURL}/api/management/account`, async (route) => {
    await route.fulfill({
      headers: { "Cache-Control": "no-store" },
      json: {
        user: {
          id: "user_1",
          email: "owner@example.com",
          display_name: "Owner",
          is_admin: options.admin || false,
        },
        tenants: [{
          id: profile.tenant.id,
          name: profile.tenant.name,
          has_secret: profile.tenant.has_secret,
          created_at: profile.tenant.created_at,
          updated_at: profile.tenant.updated_at,
        }],
      },
    });
  });
  await page.route(`${baseURL}${managementDefaultTenantPath}`, async (route) => {
    const profileStatus = profileStatuses.length > 0 ? profileStatuses.shift() : options.profileStatus;
    if (profileStatus && profileStatus !== httpOK) {
      await route.fulfill({ status: profileStatus, body: options.profileError || "authentication_required" });
      return;
    }
    await route.fulfill({ headers: { "Cache-Control": "no-store" }, json: profile });
  });
  await page.route(usageRequestPattern(), async (route) => {
    const interval = new URL(route.request().url()).searchParams.get("interval") || "";
    const supportedInterval = usageIntervals.some((candidate) => candidate.id === interval);
    if (!supportedInterval) {
      await route.fulfill({ status: 400, json: { error: "managed_usage_interval_invalid" } });
      return;
    }
    await route.fulfill({ status: options.usageStatus || httpOK, json: managementUsage(interval) });
  });
  await page.route(`${baseURL}/api/management/admin/users`, async (route) => {
    await route.fulfill({ json: managementAdminUsers() });
  });
  await page.route(`${baseURL}${managementDefaultTenantPath}/secrets`, async (route) => {
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
	await page.route(`${baseURL}${managementDefaultTenantPath}/defaults`, async (route) => {
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
      reconcileManagementProfileRoutingDefaults(profile);
      await route.fulfill({ headers: { "Cache-Control": "no-store" }, json: profile });
      return;
    }
    if (request.method() === "DELETE") {
      delete providerKeys[providerID];
      provider.has_key = false;
      delete provider.masked_key;
      reconcileManagementProfileRoutingDefaults(profile);
      await route.fulfill({ headers: { "Cache-Control": "no-store" }, json: profile });
      return;
    }
    await route.fulfill({ status: httpInternalServerError });
  });
}

/**
 * @param {ReturnType<typeof managementProfile>} profile
 * @returns {void}
 */
function reconcileManagementProfileRoutingDefaults(profile) {
  const keyedProviders = profile.providers
    .filter((provider) => provider.has_key)
    .toSorted((first, second) => first.id.localeCompare(second.id));
  const currentTextProvider = keyedProviders.find((provider) => provider.id === profile.tenant.defaults.provider);
  if (!currentTextProvider) {
    const nextTextProvider = keyedProviders[0];
    profile.tenant.defaults.provider = nextTextProvider ? nextTextProvider.id : "";
    profile.tenant.defaults.model = nextTextProvider ? nextTextProvider.text_model : "";
    profile.tenant.defaults.reasoning_effort = "";
  }
  const keyedDictationProviders = keyedProviders.filter((provider) => provider.supports_dictation);
  const currentDictationProvider = keyedDictationProviders.find(
    (provider) => provider.id === profile.tenant.defaults.dictation_provider,
  );
  if (!currentDictationProvider) {
    const nextDictationProvider = keyedDictationProviders[0];
    profile.tenant.defaults.dictation_provider = nextDictationProvider ? nextDictationProvider.id : "";
    profile.tenant.defaults.dictation_model = nextDictationProvider ? nextDictationProvider.dictation_default_model : "";
  }
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
 * @param {import("@playwright/test").Locator} providerKeyInput
 * @param {string} value
 * @returns {Promise<void>}
 */
async function pasteProviderKey(providerKeyInput, value) {
  await providerKeyInput.focus();
  await providerKeyInput.evaluate((inputElement, pastedValue) => {
    const input = /** @type {HTMLInputElement} */ (inputElement);
    const clipboardData = new DataTransfer();
    clipboardData.setData("text/plain", pastedValue);
    input.dispatchEvent(new ClipboardEvent("paste", {
      bubbles: true,
      cancelable: true,
      clipboardData,
    }));
    input.value = pastedValue;
    input.dispatchEvent(new InputEvent("input", {
      bubbles: true,
      data: pastedValue,
      inputType: "insertFromPaste",
    }));
  }, value);
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
    tenant: {
      id: "tenant_1",
      name: "Default",
      has_secret: hasSecret,
      created_at: "2026-07-01T00:00:00Z",
      updated_at: "2026-07-01T00:00:00Z",
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
        id: "anthropic",
        label: "Anthropic",
        aliases: ["claude"],
        has_key: false,
        text_model: "claude-sonnet-4-6",
        system_prompt: "",
        text_default_model: "claude-sonnet-4-6",
        text_models: [{ id: "claude-sonnet-4-6" }],
        supports_dictation: false,
        dictation_models: [],
      },
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
            id: "gpt-5-mini",
            reasoning_effort: {
              adapter: "openai_responses",
              efforts: ["minimal", "low", "medium", "high"],
            },
          },
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
 * @param {string} tenantID
 * @param {string} name
 * @param {boolean} [hasSecret]
 * @returns {object}
 */
function managementTenantProfile(tenantID, name, hasSecret = true) {
  const profile = managementProfile(false, hasSecret);
  profile.tenant.id = tenantID;
  profile.tenant.name = name;
  return profile;
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
        tenant_count: 2,
        tenants: [
          {
            id: "tenant_1",
            name: "Default",
            has_secret: true,
            created_at: "2026-06-01T00:00:00Z",
            updated_at: "2026-06-29T00:00:00Z",
            usage: managementAdminUsage(),
          },
          {
            id: "tenant_3",
            name: "Research",
            has_secret: false,
            created_at: "2026-06-15T00:00:00Z",
            updated_at: "2026-06-20T00:00:00Z",
            usage: {
              ...managementAdminUsage(),
              totals: usageAggregate({
                requests: 5,
                successful_requests: 4,
                failed_requests: 1,
                total_tokens: 500,
              }),
            },
          },
        ],
      },
      {
        user: {
          id: "user_2",
          email: "teammate@example.com",
          display_name: "Teammate",
          is_admin: false,
        },
        tenant_count: 1,
        tenants: [{
          id: "tenant_2",
          name: "Default",
          has_secret: false,
          created_at: "2026-06-10T00:00:00Z",
          updated_at: "2026-06-10T00:00:00Z",
          usage: {
            ...managementAdminUsage(),
            totals: usageAggregate(),
            providers: [],
            models: [],
            status_codes: [],
          },
        }],
      },
    ],
  };
}

/**
 * @param {string} [interval]
 * @param {Partial<Record<string, number>>} [totalOverrides]
 * @returns {object}
 */
function managementUsage(interval = "30d", totalOverrides = {}) {
  const intervalFixture = usageIntervals.find((candidate) => candidate.id === interval);
  if (!intervalFixture) {
    throw new Error(`management_usage_interval_invalid:${interval}`);
  }
  const bucketCount = interval === "all" ? 1 : interval === "1d" ? 24 : Number.parseInt(interval, 10);
  const bucketUnit = interval === "1d" ? "hour" : "day";
  const buckets = Array.from({ length: bucketCount }, (_, index) => ({
    start: new Date(Date.UTC(2026, 5, index + 1)).toISOString(),
    data: usageAggregate(),
  }));
  buckets[buckets.length - 1].data = usageAggregate({
    requests: intervalFixture.requests,
    successful_requests: intervalFixture.requests,
    text_requests: intervalFixture.requests,
    total_tokens: intervalFixture.totalTokens,
  });
  const isDefaultInterval = interval === "30d";
  if (isDefaultInterval) {
    buckets[28].data = usageAggregate({
      requests: 17,
      successful_requests: 17,
      text_requests: 17,
      total_tokens: 6000,
    });
    buckets[29].data = usageAggregate({
      requests: 20,
      successful_requests: 18,
      failed_requests: 2,
      text_requests: 18,
      dictation_requests: 2,
      total_tokens: 6345,
    });
  }
  const providerBreakdown = isDefaultInterval
    ? [
        { provider: "openai", data: usageAggregate({ requests: 24 }) },
        { provider: "deepseek", data: usageAggregate({ requests: 13 }) },
      ]
    : [
        {
          provider: `provider-${interval}`,
          data: usageAggregate({ requests: intervalFixture.requests }),
        },
      ];
  const modelBreakdown = isDefaultInterval
    ? [
        { provider: "openai", model: "gpt-4.1", data: usageAggregate({ requests: 21 }) },
        { provider: "deepseek", model: "deepseek-chat", data: usageAggregate({ requests: 13 }) },
        { provider: "openai", model: "gpt-4o-mini-transcribe", data: usageAggregate({ requests: 3 }) },
      ]
    : [
        {
          provider: `provider-${interval}`,
          model: `model-${interval}`,
          data: usageAggregate({ requests: intervalFixture.requests }),
        },
      ];
  return {
    interval,
    bucket_unit: bucketUnit,
    totals: usageAggregate({
      requests: intervalFixture.requests,
      successful_requests: isDefaultInterval ? 35 : intervalFixture.requests,
      failed_requests: isDefaultInterval ? 2 : 0,
      text_requests: isDefaultInterval ? 35 : intervalFixture.requests,
      dictation_requests: isDefaultInterval ? 2 : 0,
      request_tokens: 4567,
      response_tokens: 7778,
      total_tokens: intervalFixture.totalTokens,
      average_latency_ms: 312,
      ...totalOverrides,
    }),
    buckets,
    providers: providerBreakdown,
    models: modelBreakdown,
    status_codes: [
      { status_code: 200, requests: isDefaultInterval ? 35 : intervalFixture.requests },
      ...(isDefaultInterval ? [{ status_code: 502, requests: 2 }] : []),
    ],
  };
}

/**
 * @param {string} interval
 * @param {number} count
 * @param {string} [nextCursor]
 * @param {number} [offset]
 * @returns {object}
 */
function managementUsageFailures(interval, count, nextCursor = "", offset = 0) {
  const failureTemplates = [
    {
      endpoint: "v2",
      provider: "openai",
      model: "gpt-4.1",
      status_code: 502,
      outcome_code: "upstream_error",
      latency_ms: 245,
    },
    {
      endpoint: "text",
      provider: "",
      model: "",
      status_code: 400,
      outcome_code: "invalid_request",
      latency_ms: 3,
    },
    {
      endpoint: "dictation",
      provider: "openai",
      model: "gpt-4o-mini-transcribe",
      status_code: 429,
      outcome_code: "rate_limited",
      latency_ms: 81,
    },
    {
      endpoint: "v2",
      provider: "meta",
      model: "muse-spark-1.1",
      status_code: 503,
      outcome_code: "service_unavailable",
      latency_ms: 9,
    },
    {
      endpoint: "text",
      provider: "deepseek",
      model: "deepseek-chat",
      status_code: 504,
      outcome_code: "request_timeout",
      latency_ms: 1_000,
    },
    {
      endpoint: "v2",
      provider: "openai",
      model: "gpt-4.1",
      status_code: 413,
      outcome_code: "payload_too_large",
      latency_ms: 2,
    },
    {
      endpoint: "text",
      provider: "deepseek",
      model: "deepseek-chat",
      status_code: 499,
      outcome_code: "request_timeout",
      latency_ms: 17,
    },
  ];
  const failures = Array.from({ length: count }, (_, index) => {
    const template = failureTemplates[(offset + index) % failureTemplates.length];
    return {
      tenant_id: (offset + index) % 2 === 0 ? "tenant_1" : "tenant_2",
      tenant_name: (offset + index) % 2 === 0 ? "Default" : "Research",
      occurred_at: new Date(Date.UTC(2026, 6, 25, 12, 0, 0) - ((offset + index) * 1_000)).toISOString(),
      ...template,
      ...(index === 0 && offset === 0
        ? {
            provider_error: "raw-provider-body",
            credential: "sk-never-render",
            prompt: "private prompt",
          }
        : {}),
    };
  });
  return {
    interval,
    failures,
    ...(nextCursor ? { next_cursor: nextCursor } : {}),
  };
}

/**
 * @returns {object}
 */
function managementAdminUsage() {
  const summary = managementUsage("30d");
  return {
    period_days: 30,
    totals: summary.totals,
    daily: summary.buckets.map((bucket) => ({
      date: bucket.start.slice(0, 10),
      data: bucket.data,
    })),
    providers: summary.providers,
    models: summary.models,
    status_codes: summary.status_codes,
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
class MprFooter extends HTMLElement {
  connectedCallback() {
    const horizontalLinks = JSON.parse(this.getAttribute("horizontal-links") || '{"links":[]}');
    const footer = document.createElement("footer");
    footer.setAttribute("role", "contentinfo");
    const navigation = document.createElement("nav");
    navigation.setAttribute("aria-label", "Utility links");
    horizontalLinks.links.forEach((item) => {
      const anchor = document.createElement("a");
      anchor.textContent = item.label;
      anchor.setAttribute("href", item.href || item.url);
      navigation.append(anchor);
    });
    const prefix = document.createElement("span");
    prefix.textContent = this.getAttribute("prefix-text") || "";
    footer.append(navigation, prefix);
    this.replaceChildren(footer);
  }
}
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

mpr-footer[sticky="false"] {
  position: relative;
}

mpr-footer[size="small"] {
  min-height: 48px;
}

mpr-footer footer {
  display: flex;
  min-height: inherit;
  box-sizing: border-box;
  padding: 8px 12px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  border-top: 1px solid rgba(148, 163, 184, 0.25);
}

mpr-footer nav {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
  font-size: 10px;
  white-space: nowrap;
}

mpr-footer footer > span {
  flex: 0 0 auto;
  font-size: 10px;
  font-weight: 600;
  white-space: nowrap;
}
`;
}
