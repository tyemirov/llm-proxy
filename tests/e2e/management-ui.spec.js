// @ts-check

import { expect, test } from "@playwright/test";
import { execFile, spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { createReadStream } from "node:fs";
import { mkdir, mkdtemp, readFile, rm, stat } from "node:fs/promises";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import {
  assertPublicDocumentShell,
  LOOPAWARE_PIXEL_URL,
  PUBLIC_FOOTER_COMPACT_MAX_HEIGHT,
  renderLandingHeader,
  renderPublicHeader,
} from "../../scripts/public_site_shell.mjs";
import {
  APPLICATION_PATH,
  LANDING_AUTHENTICATED_REDIRECT_ATTRIBUTE,
} from "../../site/assets/llm-proxy/js/constants.js";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const siteSourceRoot = path.join(repoRoot, "site");
const executeFile = promisify(execFile);
const canonicalOpenAPIFile = path.join(repoRoot, "docs/openapi.yaml");
const configPath = "/config-ui.yaml";
const applicationPath = APPLICATION_PATH;
const landingAuthRouteModuleRevision = "20260808b113";
const removedApplicationPath = "/manage/";
const defaultTenantID = "tenant_1";
const managementDefaultTenantPath = `/api/management/tenants/${defaultTenantID}`;
const managementProviderKeysPath = `${managementDefaultTenantPath}/provider-connections`;
const faviconPath = "/assets/llm-proxy/img/favicon.svg";
const appIconPath = "/assets/llm-proxy/img/llm-proxy-icon.svg";
const resourcesPath = "/resources/";
const privacyPath = "/privacy/";
const termsPath = "/terms/";
const representativeResourcePath = "/resources/multi-provider-llm-proxy/";
const clientAuthenticationResourcePath = "/resources/llm-proxy-client-authentication/";
const sitemapPath = "/sitemap.xml";
const robotsPath = "/robots.txt";
const apiDocumentationPath = "/docs/";
const openAPIPath = "/openapi.yaml";
const openAPISchemaViewerPath = `${apiDocumentationPath}#openapi-schema`;
const openAPIDownloadFilename = "llm-proxy-openapi.yaml";
const applicationModuleRevision = "20260903f037";
const applicationModuleFiles = Object.freeze([
  "alpineRuntime.js",
  "app.js",
  "constants.js",
  "startupGuard.js",
  "core/backendClient.js",
  "core/managementProfile.js",
  "core/mprShell.js",
  "core/runtimeTransition.js",
  "ui/adminDashboard.js",
  "ui/applicationStartup.js",
  "ui/authenticationLifecycle.js",
  "ui/clientAccess.js",
  "ui/dialogFocus.js",
  "ui/managementApplication.js",
  "ui/managementApplicationPresentation.js",
  "ui/managementApplicationState.js",
  "ui/notifications.js",
  "ui/profileMutations.js",
  "ui/providerCards.js",
  "ui/providerCredentials.js",
  "ui/providerEditor.js",
  "ui/providerSettings.js",
  "ui/requestExamples.js",
  "ui/routingDefaults.js",
  "ui/runtimeFailure.js",
  "ui/settingsDialog.js",
  "ui/tenantSettings.js",
  "ui/usageDashboard.js",
  "ui/usageFailurePresentation.js",
  "ui/usagePresentation.js",
]);
const applicationForbiddenTAuthFragments = Object.freeze([
  "/auth/google",
  "/auth/logout",
  "/auth/nonce",
  "/auth/session",
  "document.cookie",
  "localStorage",
]);
const repositoryURL = "https://github.com/tyemirov/llm-proxy";
const mprUICSSURL = "https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui.css";
const mprUIConfigURL = "https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui-config.js";
const mprUIBundleURL = "https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui.js";
const forbiddenTAuthBrowserClientURL = "https://tauth.mprlab.com/tauth.js";
const catalogColumnCount = 3;
const b020ScreenshotDirectory = path.join(repoRoot, "output/playwright");
const f034ScreenshotDirectory = path.join(repoRoot, "output/playwright");
const httpOK = 200;
const httpNotFound = 404;
const httpInternalServerError = 500;
const routingConnectorEndpointRadius = 6;
const publicCapabilitiesPath = "/api/public/capabilities";
const capabilityShutdownProbeTimeoutMilliseconds = 1_000;

/**
 * @param {import("@playwright/test").Locator} routingTree
 */
async function expectSelectedRoutingFanEndpoints(routingTree) {
  const endpointEvidence = await routingTree.evaluate((tree, endpointRadius) => {
    const canvas = tree.querySelector("[data-route-canvas]");
    const product = tree.querySelector("[data-route-product]");
    const proxy = tree.querySelector("[data-route-proxy]");
    const selectedFamily = tree.querySelector('[data-route-family][aria-pressed="true"]');
    const selectedModel = tree.querySelector('[data-route-model-group]:not([hidden]) [data-route-model][aria-pressed="true"]');
    const selectedProvider = tree.querySelector('[data-route-provider-group]:not([hidden]) [data-route-provider][aria-pressed="true"]');
    if (
      !(canvas instanceof HTMLCanvasElement)
      || !(product instanceof HTMLElement)
      || !(proxy instanceof HTMLElement)
      || !(selectedFamily instanceof HTMLElement)
      || !(selectedModel instanceof HTMLElement)
      || !(selectedProvider instanceof HTMLElement)
    ) {
      throw new Error("routing_tree_selected_connector_endpoint_missing");
    }
    const drawingContext = canvas.getContext("2d", { willReadFrequently: true });
    if (!drawingContext) {
      throw new Error("routing_tree_canvas_context_missing");
    }
    const canvasBounds = canvas.getBoundingClientRect();
    const productBounds = product.getBoundingClientRect();
    const proxyBounds = proxy.getBoundingClientRect();
    const familyBounds = selectedFamily.getBoundingClientRect();
    const providerBounds = selectedProvider.getBoundingClientRect();
    const modelBounds = selectedModel.getBoundingClientRect();
    const canvasScaleHorizontal = canvas.width / canvasBounds.width;
    const canvasScaleVertical = canvas.height / canvasBounds.height;
    const hasPaintedPixel = (pageHorizontal, pageVertical) => {
      const centerHorizontal = Math.round((pageHorizontal - canvasBounds.left) * canvasScaleHorizontal);
      const centerVertical = Math.round((pageVertical - canvasBounds.top) * canvasScaleVertical);
      const scaledRadius = Math.ceil(endpointRadius * Math.max(canvasScaleHorizontal, canvasScaleVertical));
      const sampleLeft = Math.max(0, centerHorizontal - scaledRadius);
      const sampleTop = Math.max(0, centerVertical - scaledRadius);
      const sampleRight = Math.min(canvas.width, centerHorizontal + scaledRadius + 1);
      const sampleBottom = Math.min(canvas.height, centerVertical + scaledRadius + 1);
      const pixelChannels = drawingContext.getImageData(
        sampleLeft,
        sampleTop,
        sampleRight - sampleLeft,
        sampleBottom - sampleTop,
      ).data;
      for (let channelOffset = 0; channelOffset < pixelChannels.length; channelOffset += 4) {
        if (pixelChannels[channelOffset + 3] > 0) {
          return true;
        }
      }
      return false;
    };
    return {
      productOutgoingEndpoint: hasPaintedPixel(productBounds.right, productBounds.top + productBounds.height / 2),
      proxyIncomingEndpoint: hasPaintedPixel(proxyBounds.left, proxyBounds.top + proxyBounds.height / 2),
      proxyOutgoingEndpoint: hasPaintedPixel(proxyBounds.right, proxyBounds.top + proxyBounds.height / 2),
      familyIncomingEndpoint: hasPaintedPixel(familyBounds.left, familyBounds.top + familyBounds.height / 2),
      familyOutgoingEndpoint: hasPaintedPixel(familyBounds.right, familyBounds.top + familyBounds.height / 2),
      modelIncomingEndpoint: hasPaintedPixel(modelBounds.left, modelBounds.top + modelBounds.height / 2),
      modelOutgoingEndpoint: hasPaintedPixel(modelBounds.right, modelBounds.top + modelBounds.height / 2),
      providerIncomingEndpoint: hasPaintedPixel(providerBounds.left, providerBounds.top + providerBounds.height / 2),
    };
  }, routingConnectorEndpointRadius);
  expect(Object.values(endpointEvidence).every(Boolean)).toBe(true);
}

/**
 * @param {import("@playwright/test").Locator} routingTree
 */
async function expectFiveStageRouteOrder(routingTree) {
  const stageOrder = await routingTree.evaluate((tree) => {
    const stages = [
      tree.querySelector("[data-route-product]"),
      tree.querySelector("[data-route-proxy]"),
      tree.querySelector('[data-route-family][aria-pressed="true"]'),
      tree.querySelector('[data-route-model-group]:not([hidden]) [data-route-model][aria-pressed="true"]'),
      tree.querySelector('[data-route-provider-group]:not([hidden]) [data-route-provider][aria-pressed="true"]'),
    ];
    if (!stages.every((stage) => stage instanceof HTMLElement)) {
      throw new Error("routing_tree_five_stage_route_missing");
    }
    return stages.map((stage) => stage.getBoundingClientRect());
  });
  for (let stageIndex = 1; stageIndex < stageOrder.length; stageIndex += 1) {
    expect(stageOrder[stageIndex - 1].right).toBeLessThan(stageOrder[stageIndex].left);
  }
}

/**
 * @param {import("@playwright/test").Locator} routingTree
 */
async function expectRoutingHeaderSingleRow(routingTree) {
  const geometry = await routingTree.evaluate((tree) => {
    const header = tree.querySelector(".routing-tree__header");
    const title = header?.querySelector("h2");
    const filters = header?.querySelector(".routing-tree__filters");
    const counts = header?.querySelector("[data-route-counts]");
    if (!(header instanceof HTMLElement) || !(title instanceof HTMLElement) || !(filters instanceof HTMLElement) || !(counts instanceof HTMLElement)) {
      throw new Error("routing_tree_header_surface_missing");
    }
    const visibleItems = [title, filters, counts].filter((item) => getComputedStyle(item).display !== "none");
    const itemCenters = visibleItems.map((item) => {
      const bounds = item.getBoundingClientRect();
      return bounds.top + bounds.height / 2;
    });
    const filterButtons = [...filters.querySelectorAll("button")];
    return {
      filterButtonTopRange: Math.max(...filterButtons.map((button) => button.getBoundingClientRect().top))
        - Math.min(...filterButtons.map((button) => button.getBoundingClientRect().top)),
      filterClientHeight: filters.clientHeight,
      filterScrollHeight: filters.scrollHeight,
      headerHeight: header.getBoundingClientRect().height,
      itemCenterRange: Math.max(...itemCenters) - Math.min(...itemCenters),
    };
  });
  expect(geometry.headerHeight).toBeLessThanOrEqual(60);
  expect(geometry.itemCenterRange).toBeLessThanOrEqual(1);
  expect(geometry.filterButtonTopRange).toBeLessThanOrEqual(1);
  expect(geometry.filterScrollHeight).toBeLessThanOrEqual(geometry.filterClientHeight + 1);
}

/**
 * @param {string} moduleSource
 * @returns {string[]}
 */
function runtimeJavaScriptSpecifiers(moduleSource) {
  return [...moduleSource.matchAll(/^\s*import(?:[\s\S]*?\sfrom\s+)?["']([^"']+\.js(?:\?v=[^"']+)*)["'];/gmu)]
    .map((match) => match[1])
    .filter((specifier) => specifier.startsWith("."));
}
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
const landingModifiedDate = "2026-08-08";
const seoContentModifiedDate = "2026-07-11";
const seoCurrentContentModifiedDate = "2026-07-22";
const seoResourceIndexModifiedDate = "2026-09-01";
const seoUsageContentModifiedDate = "2026-09-03";
const seoProviderCatalogModifiedDate = "2026-08-22";
const seoClientDocumentationPublishedDate = landingModifiedDate;
const seoClientDocumentationModifiedDate = seoProviderCatalogModifiedDate;
const seoSecretRotationModifiedDate = "2026-07-26";
const obsoletePublicTermPattern = new RegExp(["work", "space"].join(""), "i");
const settingsLayerViewports = Object.freeze([
  { name: "desktop", width: 1280, height: 720 },
  { name: "compact", width: 480, height: 780 },
  { name: "mobile", width: 390, height: 780 },
]);
const routingTreeViewports = Object.freeze([
  { name: "desktop", width: 1280, height: 800 },
  { name: "compact", width: 900, height: 900 },
  { name: "mobile", width: 390, height: 900 },
]);
const settingsHeaderAlignmentTolerance = 1;
const geometryCenterDivisor = 2;

let server;
let baseURL = "";
let siteRoot = siteSourceRoot;
let renderedSiteTempRoot = "";
let renderedSiteRoot = "";

test.beforeAll(async () => {
  renderedSiteTempRoot = await mkdtemp(path.join(os.tmpdir(), "llm-proxy-site-"));
  renderedSiteRoot = path.join(renderedSiteTempRoot, "rendered");
  const capabilityConfigPath = path.join(renderedSiteTempRoot, "capabilities.yml");
  const capabilityConfigResult = await executeFile(
    "node",
    ["scripts/create_public_capability_test_config.mjs", "configs/config.yml", capabilityConfigPath],
    { cwd: repoRoot },
  );
  const capabilityPort = Number(capabilityConfigResult.stdout.trim());
  if (!Number.isInteger(capabilityPort) || capabilityPort <= 0) {
    throw new Error(`public_capability_test_port_invalid: ${capabilityConfigResult.stdout.trim()}`);
  }
  const capabilityBinaryPath = path.join(renderedSiteTempRoot, "llm-proxy");
  await executeFile("go", ["build", "-o", capabilityBinaryPath, "./cmd/cli"], { cwd: repoRoot });
  const capabilityServer = spawn(
    capabilityBinaryPath,
    ["--config", capabilityConfigPath, "--public-capabilities-only"],
    { cwd: repoRoot },
  );
  let capabilityServerOutput = "";
  capabilityServer.stdout.on("data", (output) => { capabilityServerOutput += output.toString(); });
  capabilityServer.stderr.on("data", (output) => { capabilityServerOutput += output.toString(); });
  const capabilitiesURL = `http://127.0.0.1:${capabilityPort}${publicCapabilitiesPath}`;
  try {
    await waitForPublicCapabilities(capabilitiesURL, capabilityServer, () => capabilityServerOutput);
    await executeFile(
      "node",
      [
        "scripts/render_public_site.mjs",
        "--source",
        "site",
        "--output",
        renderedSiteRoot,
        "--config-url",
        configPath,
        "--capabilities-url",
        capabilitiesURL,
      ],
      { cwd: repoRoot },
    );
  } finally {
    await stopChildProcess(capabilityServer);
    await assertPublicCapabilitiesStopped(capabilitiesURL);
  }
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

/**
 * @param {string} capabilitiesURL
 * @param {import("node:child_process").ChildProcessWithoutNullStreams} capabilityServer
 * @param {() => string} serverOutput
 */
async function waitForPublicCapabilities(capabilitiesURL, capabilityServer, serverOutput) {
  let lastRequestError = "public_capability_server_not_ready";
  for (let attempt = 0; attempt < 300; attempt += 1) {
    if (capabilityServer.exitCode !== null) {
      throw new Error(`public_capability_server_stopped: ${serverOutput()}`);
    }
    try {
      const response = await fetch(capabilitiesURL);
      if (response.ok) {
        return;
      }
      lastRequestError = `status=${response.status}`;
    } catch (requestError) {
      lastRequestError = requestError instanceof Error ? requestError.message : String(requestError);
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error(`public_capability_server_timeout: ${lastRequestError}\n${serverOutput()}`);
}

/**
 * @param {import("node:child_process").ChildProcessWithoutNullStreams} childProcess
 */
async function stopChildProcess(childProcess) {
  if (childProcess.exitCode !== null) {
    return;
  }
  childProcess.kill("SIGTERM");
  await new Promise((resolve) => childProcess.once("exit", resolve));
}

/**
 * @param {string} capabilitiesURL
 */
async function assertPublicCapabilitiesStopped(capabilitiesURL) {
  try {
    await fetch(capabilitiesURL, {
      signal: AbortSignal.timeout(capabilityShutdownProbeTimeoutMilliseconds),
    });
  } catch (requestError) {
    if (requestError instanceof TypeError) {
      return;
    }
    throw requestError;
  }
  throw new Error(`public_capability_server_listener_open: ${capabilitiesURL}`);
}

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
  expect(html).toContain("Integrate once. Use the model that fits.");
  expect(html).toContain("official Go, Python, and command-line clients");
  expect(html).toContain("AI-assisted builders");
  expect(html).toContain("Startups and product teams");
  expect(html).toContain("Platform and engineering teams");
  expect(html).toContain("The catalog is the source of truth.");
  expect(html).toContain("Provider lifecycle, model-onboarding, and hosted uptime commitments are outside the current catalog contract");
  const heroOffset = html.indexOf('id="hero-title"');
  const routingOverviewOffset = html.indexOf('id="routing-overview"');
  const valueStripOffset = html.indexOf('class="value-strip__grid"');
  const integrationOffset = html.indexOf('id="integrate"');
  const audienceOffset = html.indexOf('id="audience-title"');
  const capabilitiesOffset = html.indexOf('id="capabilities"');
  const modelsOffset = html.indexOf('id="models"');
  const routingTreeOffset = html.indexOf('<routing-tree class="routing-tree"');
  const capabilityCatalogOffset = html.indexOf("<capability-catalog");
  expect(heroOffset).toBeGreaterThan(-1);
  expect(heroOffset).toBeLessThan(routingOverviewOffset);
  expect(routingOverviewOffset).toBeLessThan(valueStripOffset);
  expect(valueStripOffset).toBeLessThan(routingTreeOffset);
  expect(routingTreeOffset).toBeLessThan(integrationOffset);
  expect(integrationOffset).toBeLessThan(audienceOffset);
  expect(audienceOffset).toBeLessThan(capabilitiesOffset);
  expect(capabilitiesOffset).toBeLessThan(modelsOffset);
  expect(modelsOffset).toBeLessThan(capabilityCatalogOffset);
  expect(html).toContain(`${LANDING_AUTHENTICATED_REDIRECT_ATTRIBUTE}="${applicationPath}"`);
  expect(html).not.toContain("sign-in-redirect-url=");
  expect(html).toContain(`"href":"${resourcesPath}"`);
  expect(html).toContain(`href="${apiDocumentationPath}"`);
  expect(html).toContain('<routing-tree class="routing-tree" data-enhanced="false" aria-label="Interactive LLM routing map">');
  expect(html).toContain("One integration. Choose the exact route.");
  expect(html).toContain('<canvas class="routing-tree__connectors" data-route-canvas aria-hidden="true"></canvas>');
  expect(html).toContain('<output class="routing-tree__counts" aria-live="polite" data-route-counts>12 families · 39 exact models · 39 offerings</output>');
  expect(html).toContain('data-route-family="deepseek-r1"');
  expect(html).toContain('data-route-family="muse-spark"');
  expect(html).toContain('data-route-model="muse-spark-1.2" data-route-model-family="muse-spark"');
  expect(html).toContain('data-route-family="qwen"');
  expect(html).toContain('data-route-model="kimi-k2.6" data-route-model-family="kimi-k2"');
  expect(html).toContain('data-route-model="kimi-k3" data-route-model-family="kimi-k3"');
  expect(html).toContain('data-route-model="qwen-plus" data-route-model-family="qwen"');
  expect(html).toContain('data-route-model="qwen3.7-max" data-route-model-family="qwen"');
  expect(html).toContain('data-route-model="qwen3.7-plus" data-route-model-family="qwen"');
  expect(html).toContain('data-route-model="qwen3.6-flash" data-route-model-family="qwen"');
  expect(html).toContain('data-route-model="minimax-m2.7-highspeed" data-route-model-family="minimax-m2"');
  expect(html).toContain('data-route-offering="siliconflow:deepseek-reasoner"');
  expect(html).not.toContain("data-route-publisher");
  expect(html).not.toContain("llm-proxy-routing-tree");
  expect(html).toContain('<table class="catalog-table">');
  expect(html).toContain('<strong>11</strong><span>Providers</span>');
  expect(html).toContain('<strong>11</strong><span>Publishers</span>');
  expect(html).toContain('<strong>24</strong><span>Families</span>');
  expect(html).toContain('<strong>62</strong><span>Exact models</span>');
  expect(html).toContain('<strong>63</strong><span>Offerings</span>');
  expect(html).toContain('data-catalog-sort-header="publisher"');
  expect(html).toContain('data-catalog-sort-header="model"');
  expect(html).toContain('data-catalog-sort-header="capabilities"');
  expect(html).not.toContain('<th scope="col">Dictation models</th>');
  expect(html).toContain('<code data-catalog-model-id>gpt-4.1</code>');
  expect(html).toContain('<code data-catalog-model-id>gpt-4o-mini-transcribe</code>');
  expect(html).toContain('data-publisher="openai" data-provider="openai" data-model="gpt-4o-mini-transcribe"');
  expect(html).not.toContain("catalog-model__default");
  expect(html).toContain('aria-label="Search all model characteristics"');
  expect(html).toContain("data-catalog-search-submit");
  expect(html).toContain("data-catalog-filter-panel");
  expect(html).toContain("data-catalog-search-text");
  expect(html).toContain("data-capability-count");
  expect(html).toContain("data-catalog-capability");
  expect(html).toContain("data-catalog-sort");
  expect(html).not.toContain('data-catalog-capability-action="background"');
  expect(html).not.toContain('data-catalog-capability-action="synchronous"');
  expect(html).not.toContain("data-catalog-provider");
  expect(html).not.toContain("data-route-picker");
  expect(html).toContain("gpt-4o-mini-transcribe");
  expect(html).toContain("gemini-3.5-flash");
  expect(html).toContain("claude-sonnet-4-6");
  expect(html).toContain("grok-4.3");
  expect(html).toContain("grok-imagine-video-1.5");
  expect(html).not.toContain("api_key");
  expect(html).not.toContain("base_url");
  expect(html).not.toContain("provider_model");
  expect(html).not.toContain("default_operations");
  expect(html).toContain(`data-config-url="${configPath}"`);
  expect(html).toContain(`<link rel="stylesheet" href="${mprUICSSURL}">`);
  expect(html).not.toContain(`<script src="${forbiddenTAuthBrowserClientURL}"></script>`);
  expect(html).toContain(`<script src="${mprUIConfigURL}"></script>`);
  expect(html).toContain(`data-mpr-ui-bundle-src="${mprUIBundleURL}"`);
  expect(html).toContain(
    `<script type="module" src="/assets/llm-proxy/js/ui/landingAuthRoute.js?v=${landingAuthRouteModuleRevision}"></script>`,
  );
  expect(
    html.indexOf(
      `<script type="module" src="/assets/llm-proxy/js/ui/landingAuthRoute.js?v=${landingAuthRouteModuleRevision}"></script>`,
    ),
  ).toBeLessThan(html.indexOf(`<script src="${mprUIConfigURL}"></script>`));
  expect(html).toContain('<link rel="stylesheet" href="/assets/llm-proxy/public-shell.css">');
  expect(html).toContain('<mpr-header\n      class="public-site-header"');
  expect(html).toContain('<mpr-footer\n      class="public-site-footer"\n      size="small"\n      sticky="true"');
  expect(html).toContain(`privacy-link-href="${privacyPath}"`);
  expect(html).toContain('links-collection=');
  expect(html).toContain("Built by Marco Polo Research Lab");
  expect(html).toContain('<meta name="theme-color" content="#0f1114">');
  const publicShellCSS = await readFile(
    path.join(siteSourceRoot, "assets/llm-proxy/public-shell.css"),
    "utf8",
  );
  expect(publicShellCSS).toContain(".public-site-footer-layout");
  expect(publicShellCSS).not.toContain(".mpr-footer__");
  expect(publicShellCSS).not.toContain("[data-mpr-footer");

  const page = await request.get(`${baseURL}${applicationPath}`);
  expect(page.status()).toBe(httpOK);
  const managementHTML = await page.text();
  expect(managementHTML).toContain("<title>LLM Proxy</title>");
  expect(managementHTML).not.toContain("<title>Manage LLM Proxy</title>");
  expect(managementHTML).toContain('<meta name="robots" content="noindex, nofollow">');
  expect(managementHTML).toContain('<link rel="canonical" href="https://llm-proxy.mprlab.com/app/">');
  expect(managementHTML).toContain(`data-config-url="${configPath}"`);
  expect(managementHTML).toContain(`<link rel="stylesheet" href="${mprUICSSURL}">`);
  expect(managementHTML).not.toContain(`<script src="${forbiddenTAuthBrowserClientURL}"></script>`);
  expect(managementHTML).toContain(`<script src="${mprUIConfigURL}"></script>`);
  expect(managementHTML).toContain(`data-mpr-ui-bundle-src="${mprUIBundleURL}"`);
  expect(managementHTML).toContain(`<script type="module" src="/assets/llm-proxy/js/startupGuard.js?v=${applicationModuleRevision}"></script>`);
  expect(managementHTML).toContain(
    `<script id="llm-proxy-application-module" type="module" src="/assets/llm-proxy/js/app.js?v=${applicationModuleRevision}"></script>`,
  );
  expect(managementHTML).toContain('<mpr-footer\n      class="public-site-footer"\n      size="small"\n      sticky="true"');
  for (const moduleFile of applicationModuleFiles) {
    const moduleSource = await readFile(path.join(siteSourceRoot, "assets/llm-proxy/js", moduleFile), "utf8");
    for (const specifier of runtimeJavaScriptSpecifiers(moduleSource)) {
      expect(specifier, `${moduleFile} must use the current application module revision`).toContain(`?v=${applicationModuleRevision}`);
    }
    for (const revision of moduleSource.matchAll(/\?v=([a-z0-9]+)/gu)) {
      expect(revision[1], `${moduleFile} must not reference a stale application module revision`).toBe(applicationModuleRevision);
    }
    for (const forbiddenFragment of applicationForbiddenTAuthFragments) {
      expect(moduleSource, `${moduleFile} must delegate browser authentication to MPR UI`).not.toContain(
        forbiddenFragment,
      );
    }
  }
  const landingAuthRouteSource = await readFile(
    path.join(siteSourceRoot, "assets/llm-proxy/js/ui/landingAuthRoute.js"),
    "utf8",
  );
  for (const forbiddenFragment of applicationForbiddenTAuthFragments) {
    expect(
      landingAuthRouteSource,
      "the landing route policy must not implement or inspect TAuth state",
    ).not.toContain(forbiddenFragment);
  }
  expect(managementHTML).not.toContain("MarcoPoloResearchLab/mpr-ui@v");
  expect(managementHTML).not.toContain("Sign in to manage LLM Proxy keys");
  expect(managementHTML).toContain(`privacy-link-href="${privacyPath}"`);
  expect(managementHTML).toMatch(/<notification-region\s+slot="aux"[\s\S]*?<mpr-user\s+slot="aux"/);
  expect(managementHTML).toContain('<body x-data="llmProxyManagementApplication" x-init="init()">');
  expect(managementHTML).toContain("<llm-proxy-management-application");
  expect(managementHTML).not.toContain("llm-proxy-key-management");
  expect(managementHTML).not.toContain("providerUsed");
  expect(managementHTML).not.toContain('x-init="bindNotificationRegion($el)"');
  expect(managementHTML).toContain('<a slot="brand" class="llm-proxy-header-brand" href="/" aria-label="LLM Proxy home">');
  expect(managementHTML).toContain(`<img class="llm-proxy-header-brand__logo" src="${appIconPath}" alt="" aria-hidden="true">`);
  expect(managementHTML).toContain('<span class="llm-proxy-header-brand__title">LLM Proxy</span>');
  expect(managementHTML).not.toContain("brand-label=");
  expect(managementHTML).not.toContain("data:image");
  expect(managementHTML).toContain(
    '<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Material+Symbols+Outlined&amp;icon_names=content_copy,delete,help,key,visibility,visibility_off&amp;display=block">',
  );
  expect(managementHTML).toContain('<provider-card-grid>');
  expect(managementHTML).toContain('target="_blank" rel="noopener noreferrer"');

  const removedApplicationResponse = await request.get(`${baseURL}${removedApplicationPath}`);
  expect(removedApplicationResponse.status()).toBe(httpNotFound);

  html = managementHTML;
  expect(html).toContain('<meta name="theme-color" content="#0076c3">');
  expect(html).toContain(`<link rel="icon" type="image/svg+xml" href="${faviconPath}">`);
  expect(html).toContain(`<link rel="apple-touch-icon" href="${appIconPath}">`);
  expect(html).toContain(`data-config-url="${configPath}"`);
  expect(html).toContain(`<link rel="stylesheet" href="${mprUICSSURL}">`);
  expect(html).not.toContain(`<script src="${forbiddenTAuthBrowserClientURL}"></script>`);
  expect(html).toContain(`<script src="${mprUIConfigURL}"></script>`);
  expect(html).toContain(`data-mpr-ui-bundle-src="${mprUIBundleURL}"`);
  expect(html).toContain(`<script type="module" src="/assets/llm-proxy/js/startupGuard.js?v=${applicationModuleRevision}"></script>`);
  expect(html).toContain(
    `<script id="llm-proxy-application-module" type="module" src="/assets/llm-proxy/js/app.js?v=${applicationModuleRevision}"></script>`,
  );
  expect(html).not.toContain("MarcoPoloResearchLab/mpr-ui@v");
  expect(html).toMatch(/<notification-region\s+slot="aux"[\s\S]*?<mpr-user\s+slot="aux"/);
  expect(html).toContain('<body x-data="llmProxyManagementApplication" x-init="init()">');
  expect(html).not.toContain('x-init="bindNotificationRegion($el)"');
  expect(html).toContain('<a slot="brand" class="llm-proxy-header-brand" href="/" aria-label="LLM Proxy home">');
  expect(html).toContain(`<img class="llm-proxy-header-brand__logo" src="${appIconPath}" alt="" aria-hidden="true">`);
  expect(html).toContain('<span class="llm-proxy-header-brand__title">LLM Proxy</span>');
  expect(html).not.toContain("brand-label=");
  expect(html).not.toContain("data:image");
  expect(html).toContain(
    '<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Material+Symbols+Outlined&amp;icon_names=content_copy,delete,help,key,visibility,visibility_off&amp;display=block">',
  );
  expect(html).toContain('class="icon-only danger provider-key-remove"');
  expect(html).not.toContain("provider-editor-actions");
  const providerConnectionFieldsOffset = html.indexOf("<provider-connection-fields>");
  const textModelOffset = html.indexOf('x-on:change="handleSelectedProviderTextModelChange($event)"');
  const providerRemovalOffset = html.indexOf('class="icon-only danger provider-key-remove"');
  expect(providerConnectionFieldsOffset).toBeGreaterThan(-1);
  expect(providerConnectionFieldsOffset).toBeLessThan(providerRemovalOffset);
  expect(providerRemovalOffset).toBeLessThan(textModelOffset);
  expect(html).not.toContain('id="provider-settings-title"');
  expect(html).toContain('id="provider-cards-title"');
  expect(html).toContain('role="alertdialog"');
  expect(html).toContain('x-on:click="requestSelectedProviderKeyRemoval()"');
  expect(html).toContain('x-show="field.secret && field.configured"');
  expect(html).toContain('x-on:paste="handleProviderFieldPaste(field, $event)"');
  expect(html).toContain('x-on:change="autosaveSelectedProvider()"');
  expect(html).toContain('role="status" aria-live="polite"');
  expect(html).toContain('x-show="providerKeyVerificationPending"');
  expect(html).toContain('x-show="providerKeyVerificationFailed"');
  expect(html).not.toContain("provider-settings-form");
  expect(html).not.toContain("saveSelectedProviderKey");
  expect(html).not.toContain('x-on:click="removeSelectedProviderKey()"');
  expect(html).toContain('<p id="settings-title" class="eyebrow" x-text="copy.settingsEyebrow"></p>');
  expect(html).toContain('class="settings-notification"');
  expect(html).toContain('x-show="notice.message && notice.surface === noticeSurfaces.SETTINGS"');
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
  expect(html).not.toContain('<h3 x-text="settingsTenantName"></h3>');
  expect(html).toContain('x-bind:value="providerCardTenantSelectionID"');
  expect(html).toContain('x-show="providerCardTenantLoadPending"');
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
  expect(html).toContain('x-on:change="autosaveSelectedProvider()"');
  expect(html).toContain('x-on:beforeinput="prepareProviderFieldEntry(field, $event)"');
  expect(html).toContain('class="system-prompt-disclosure-state"');
  expect(html).toContain('aria-labelledby="routing-system-prompt-label"');
  expect(html).not.toContain('x-text="copy.providerDone"');
  expect(html).not.toContain('x-text="copy.providerReplaceKey"');
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

  const managementApplicationResponse = await request.get(`${baseURL}/assets/llm-proxy/js/ui/managementApplication.js`);
  expect(managementApplicationResponse.status()).toBe(httpOK);
  const managementApplicationJavaScript = await managementApplicationResponse.text();
  expect(managementApplicationJavaScript).toContain("createManagementApplication");
  expect(managementApplicationJavaScript).not.toContain("readMprUIAuthStatus");
  expect(managementApplicationJavaScript).not.toContain("autosaveSelectedProvider");

  const authenticationLifecycleResponse = await request.get(`${baseURL}/assets/llm-proxy/js/ui/authenticationLifecycle.js`);
  expect(authenticationLifecycleResponse.status()).toBe(httpOK);
  const authenticationLifecycleJavaScript = await authenticationLifecycleResponse.text();
  expect(authenticationLifecycleJavaScript).toContain("readMprUIAuthStatus");
  expect(authenticationLifecycleJavaScript).not.toContain("document.cookie");
  expect(authenticationLifecycleJavaScript).not.toContain("localStorage");
  expect(authenticationLifecycleJavaScript).not.toContain("/auth/session");

  const providerSettingsResponse = await request.get(`${baseURL}/assets/llm-proxy/js/ui/providerSettings.js`);
  expect(providerSettingsResponse.status()).toBe(httpOK);
  const providerSettingsJavaScript = await providerSettingsResponse.text();
  expect(providerSettingsJavaScript).toContain("providerEditorSession");
  expect(providerSettingsJavaScript).toContain("autosaveSelectedProvider");
  expect(providerSettingsJavaScript).not.toContain("saveSelectedProviderKey");
  expect(providerSettingsJavaScript).not.toContain("providerInputs");
  expect(providerSettingsJavaScript).not.toContain("revealedProviderID");

  const providerEditorResponse = await request.get(`${baseURL}/assets/llm-proxy/js/ui/providerEditor.js`);
  expect(providerEditorResponse.status()).toBe(httpOK);
  const providerEditorJavaScript = await providerEditorResponse.text();
  expect(providerEditorJavaScript).toContain("createProviderEditorSession");
  expect(providerEditorJavaScript).toContain("selectProvider");
  expect(providerEditorJavaScript).not.toContain("requestSaveProviderKey");

  const providerCredentialsResponse = await request.get(`${baseURL}/assets/llm-proxy/js/ui/providerCredentials.js`);
  expect(providerCredentialsResponse.status()).toBe(httpOK);
  const providerCredentialsJavaScript = await providerCredentialsResponse.text();
  expect(providerCredentialsJavaScript).not.toContain("revealProviderField");
  expect(providerCredentialsJavaScript).not.toContain("requestRevealProviderConnectionField");
  expect(providerCredentialsJavaScript).toContain("SAVED_PROVIDER_KEY_MASK");
  expect(providerCredentialsJavaScript).toContain("removeProviderKey");
  expect(providerCredentialsJavaScript).not.toContain("requestSaveProviderKey");

  const routingDefaultsResponse = await request.get(`${baseURL}/assets/llm-proxy/js/ui/routingDefaults.js`);
  expect(routingDefaultsResponse.status()).toBe(httpOK);
  const routingDefaultsJavaScript = await routingDefaultsResponse.text();
  expect(routingDefaultsJavaScript).toContain("autosaveRoutingDefaults");
  expect(routingDefaultsJavaScript).not.toContain("saveDefaults");

  const profileMutationsResponse = await request.get(`${baseURL}/assets/llm-proxy/js/ui/profileMutations.js`);
  expect(profileMutationsResponse.status()).toBe(httpOK);
  const profileMutationsJavaScript = await profileMutationsResponse.text();
  expect(profileMutationsJavaScript).toContain("enqueueProfileMutation");
  expect(profileMutationsJavaScript).toContain("waitForProfileMutations");

  const clientAccessResponse = await request.get(`${baseURL}/assets/llm-proxy/js/ui/clientAccess.js`);
  expect(clientAccessResponse.status()).toBe(httpOK);
  const clientAccessJavaScript = await clientAccessResponse.text();
  expect(clientAccessJavaScript).toContain("requestAndApplyGeneratedSecret");

  const applicationStateResponse = await request.get(`${baseURL}/assets/llm-proxy/js/ui/managementApplicationState.js`);
  expect(applicationStateResponse.status()).toBe(httpOK);
  const applicationStateJavaScript = await applicationStateResponse.text();
  expect(applicationStateJavaScript).toContain("providerEditorSession");
  expect(applicationStateJavaScript).toContain("providerCardProfile");
  expect(applicationStateJavaScript).toContain("routingDefaultsAutosavePromise");

  const providerCardsResponse = await request.get(`${baseURL}/assets/llm-proxy/js/ui/providerCards.js`);
  expect(providerCardsResponse.status()).toBe(httpOK);
  const providerCardsJavaScript = await providerCardsResponse.text();
  expect(providerCardsJavaScript).toContain("fetchTenant");
  expect(providerCardsJavaScript).toContain("providerCardProfile");
  expect(providerCardsJavaScript).not.toContain("switchSettingsTenant");

  const notificationsResponse = await request.get(`${baseURL}/assets/llm-proxy/js/ui/notifications.js`);
  expect(notificationsResponse.status()).toBe(httpOK);
  const notificationsJavaScript = await notificationsResponse.text();
  expect(notificationsJavaScript.match(/window\.setTimeout/g)).toHaveLength(1);
  expect(notificationsJavaScript).toContain("NOTICE_AUTO_DISMISS_MILLISECONDS");

  const constantsResponse = await request.get(`${baseURL}/assets/llm-proxy/js/constants.js`);
  expect(constantsResponse.status()).toBe(httpOK);
  const constantsJavaScript = await constantsResponse.text();
  expect(constantsJavaScript).toContain("export const NOTICE_AUTO_DISMISS_MILLISECONDS = 10_000;");
  expect(constantsJavaScript).toContain("Provider settings saved");
  expect(constantsJavaScript).toContain('systemPromptHidden: "Hidden"');
  expect(constantsJavaScript).toContain('systemPromptExpanded: "Expanded"');
  expect(constantsJavaScript).toContain('providerClose: "Close provider settings"');
  expect(constantsJavaScript).not.toContain("providerReplaceKey");
  expect(constantsJavaScript).not.toContain("providerDone");
  expect(constantsJavaScript).not.toContain('saveProviderKey: "Save key"');
  expect(constantsJavaScript).not.toContain('updateProviderKey: "Update key"');
  expect(constantsJavaScript).not.toContain('saveDefaults: "Save defaults"');
  expect(constantsJavaScript).not.toContain("providerBaseURL");
  expect(constantsJavaScript).not.toContain("providerKeySuffix");

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

test("the routing tree and capability catalog remain complete without JavaScript", async ({ browser }) => {
  const browserContext = await browser.newContext({ javaScriptEnabled: false });
  const page = await browserContext.newPage();
  await page.goto(baseURL);

  const routingTree = page.locator("routing-tree");
  await expect(page.locator("#routing-overview routing-tree")).toHaveCount(1);
  await expect(page.locator("#models > routing-tree")).toHaveCount(0);
  await expect(routingTree).toHaveAttribute("data-enhanced", "false");
  await expect(routingTree.locator("[data-route-family]")).toHaveCount(24);
  await expect(routingTree.locator("[data-route-provider]")).toHaveCount(63);
  await expect(routingTree.locator("[data-route-model]")).toHaveCount(62);
  await expect(routingTree.locator("[data-route-weight-access]")).toHaveCount(2);
  await expect(routingTree.locator("[data-route-capability]")).toHaveCount(8);
  await expect(routingTree.locator('[data-route-weight-access="proprietary"]')).toHaveAttribute("aria-pressed", "true");
  await expect(routingTree.locator('[data-route-weight-access="open_weights"]')).toHaveAttribute("aria-pressed", "false");
  await expect(routingTree.locator('[data-route-capability="text"]')).toHaveAttribute("aria-pressed", "true");
  await expect(routingTree.locator('[data-route-weight-access][aria-pressed="true"]')).toHaveCount(1);
  await expect(routingTree.locator('[data-route-capability][aria-pressed="true"]')).toHaveCount(1);
  await expect(routingTree.locator('[data-route-family]:visible')).toHaveCount(12);
  await expect(routingTree.locator("[data-route-counts]")).toHaveText("12 families · 39 exact models · 39 offerings");
  await expect(routingTree.locator('[data-route-family="claude-fable"]')).toHaveAttribute("aria-pressed", "true");
  await expect(routingTree.locator('[data-route-model-group="claude-fable"]')).toBeVisible();
  await expect(routingTree.locator('[data-route-model-group="grok"]')).toBeHidden();
  await expect(routingTree.locator('[data-route-provider-group="claude-fable-5"] [data-route-provider="anthropic"]')).toHaveAttribute("aria-pressed", "true");
  await expect(routingTree.locator('[data-route-selected-provider]')).toHaveText("anthropic");
  await expect(routingTree.locator('[data-route-selected-model]')).toHaveText("claude-fable-5");
  await expect(routingTree.locator("button:enabled")).toHaveCount(0);

  const catalog = page.locator("capability-catalog");
  await expect(catalog).toHaveAttribute("data-enhanced", "false");
  await expect(catalog.locator("[data-catalog-toolbar]")).toBeHidden();
  await expect(catalog.getByRole("columnheader")).toHaveText(["Publisher", "Model", "Provider offerings and capabilities"]);
  await expect(catalog.locator("[data-catalog-row]")).toHaveCount(62);
  await expect(catalog.locator('[data-model="gpt-4o-mini-transcribe"]')).toContainText("Dictation");
  await expect(catalog.locator('[data-model="gpt-4o-mini"]')).toContainText("Image input");
  await expect(catalog.locator('[data-model="claude-fable-5"]')).toContainText("Image input");
  await expect(catalog.locator('[data-model="grok-4.5"]')).toContainText("Image input");
  await expect(catalog.locator('[data-model="gemini-3.5-flash"]')).toContainText("Image input");
  await expect(catalog.locator('[data-model="gemini-3.5-flash"]')).toContainText("Audio message input");
  await expect(catalog.locator('[data-model="kimi-k2.6"]')).toContainText("Image input");
  await expect(catalog.locator('[data-model="kimi-k3"]')).toContainText("Reasoning");
  await expect(catalog.locator('[data-model="minimax-m2.7-highspeed"]')).toContainText("204800 token output");
  const museSpark12Row = catalog.locator('[data-model="muse-spark-1.2"]');
  await expect(museSpark12Row).toHaveCount(1);
  await expect(museSpark12Row).toContainText("Meta");
  await expect(museSpark12Row).toContainText("Text generation");
  await expect(museSpark12Row).not.toContainText("Image input");
  await expect(museSpark12Row).not.toContainText("Reasoning");
  await expect(catalog.locator('[data-model="muse-spark-1.2-contributor"]')).toHaveCount(0);

  const footer = page.locator(".public-site-footer-fallback");
  await expect(footer).toBeVisible();
  await expect(footer.getByRole("link", { name: "Resources" })).toHaveAttribute("href", resourcesPath);
  await expect(footer.getByRole("link", { name: "Privacy" })).toHaveAttribute("href", privacyPath);
  await expect(footer.getByRole("link", { name: "Terms" })).toHaveAttribute("href", termsPath);
  const projects = footer.locator("details");
  await projects.locator("summary").click();
  await expect(projects).toHaveAttribute("open", "");
  await expect(projects.getByRole("link")).toHaveCount(10);

  await browserContext.close();
});

test("the route explorer applies each canonical theme palette", async ({ page }) => {
  await installAssetRoutes(page, { initialAuthStatus: "unauthenticated" });
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto(baseURL);

  const routingTree = page.locator("routing-tree");
  const routeOverview = page.locator(".route-overview");
  const routeFilter = routingTree.locator('[data-route-weight-access="open_weights"]');
  const routeBranch = routingTree.locator('.routing-tree__family[aria-pressed="false"]:visible').first();
  const routeProduct = routingTree.locator("[data-route-product]");
  const routeProxy = routingTree.locator("[data-route-proxy]");
  const routeCanvas = routingTree.locator("[data-route-canvas]");
  await expect(routingTree).toHaveAttribute("data-enhanced", "true");

  const themeModes = [
    {
      theme: "light",
      palette: "default",
      routeColors: {
        section: "rgb(238, 242, 247)",
        surface: "rgb(255, 255, 255)",
        filter: "rgb(248, 250, 252)",
        branch: "rgb(248, 250, 252)",
        product: "rgb(248, 250, 252)",
        proxy: "rgb(236, 253, 245)",
      },
    },
    {
      theme: "light",
      palette: "sunrise",
      routeColors: {
        section: "rgb(255, 237, 213)",
        surface: "rgb(255, 251, 235)",
        filter: "rgb(255, 247, 237)",
        branch: "rgb(255, 247, 237)",
        product: "rgb(255, 247, 237)",
        proxy: "rgb(254, 243, 199)",
      },
    },
    {
      theme: "dark",
      palette: "default",
      routeColors: {
        section: "rgb(18, 21, 26)",
        surface: "rgb(17, 20, 25)",
        filter: "rgb(23, 26, 31)",
        branch: "rgb(24, 27, 32)",
        product: "rgb(24, 27, 32)",
        proxy: "rgb(20, 38, 34)",
      },
    },
    {
      theme: "dark",
      palette: "forest",
      routeColors: {
        section: "rgb(5, 46, 43)",
        surface: "rgb(8, 40, 32)",
        filter: "rgb(11, 48, 39)",
        branch: "rgb(13, 53, 43)",
        product: "rgb(13, 53, 43)",
        proxy: "rgb(15, 61, 46)",
      },
    },
  ];
  const routeCanvasImages = [];
  for (const themeMode of themeModes) {
    await page.evaluate(({ theme, palette }) => {
      document.documentElement.setAttribute("data-mpr-theme", theme);
      document.documentElement.setAttribute("data-llm-proxy-palette", palette);
      document.body.setAttribute("data-mpr-theme", theme);
      document.body.setAttribute("data-llm-proxy-palette", palette);
    }, themeMode);
    await expect.poll(async () => ({
      section: await routeOverview.evaluate((sectionElement) => getComputedStyle(sectionElement).backgroundColor),
      surface: await routingTree.evaluate((treeElement) => getComputedStyle(treeElement).backgroundColor),
      filter: await routeFilter.evaluate((filterElement) => getComputedStyle(filterElement).backgroundColor),
      branch: await routeBranch.evaluate((branchElement) => getComputedStyle(branchElement).backgroundColor),
      product: await routeProduct.evaluate((productElement) => getComputedStyle(productElement).backgroundColor),
      proxy: await routeProxy.evaluate((proxyElement) => getComputedStyle(proxyElement).backgroundColor),
    })).toEqual(themeMode.routeColors);
    await page.evaluate(() => new Promise((resolve) => {
      requestAnimationFrame(() => requestAnimationFrame(() => resolve(undefined)));
    }));
    routeCanvasImages.push(await routeCanvas.evaluate((canvasElement) => {
      if (!(canvasElement instanceof HTMLCanvasElement)) {
        throw new Error("routing_tree_canvas_missing");
      }
      return canvasElement.toDataURL();
    }));
  }
  expect(new Set(routeCanvasImages).size).toBe(4);
});

test("visitors can filter and choose a model family, exact model, and provider offering", async ({ page }) => {
  await installAssetRoutes(page, { initialAuthStatus: "unauthenticated" });
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto(baseURL);

  const routingTree = page.locator("routing-tree");
  const selectedProvider = routingTree.locator("[data-route-selected-provider]");
  const selectedModel = routingTree.locator("[data-route-selected-model]");
  const counts = routingTree.locator("[data-route-counts]");
  const proprietaryFilter = routingTree.locator('[data-route-weight-access="proprietary"]');
  const openWeightsFilter = routingTree.locator('[data-route-weight-access="open_weights"]');
  const textFilter = routingTree.locator('[data-route-capability="text"]');
  const imageInputFilter = routingTree.locator('[data-route-capability="image_input"]');
  const audioInputFilter = routingTree.locator('[data-route-capability="audio_input"]');
  const webSearchFilter = routingTree.locator('[data-route-capability="web_search"]');
  await expect(routingTree).toHaveAttribute("data-enhanced", "true");
  await expect(routingTree).toHaveAttribute("data-route-lines-rendered", "true");
  await expect(routingTree.locator("[data-route-family]")).toHaveCount(24);
  await expect(routingTree.locator("[data-route-model]")).toHaveCount(62);
  await expect(routingTree.locator("[data-route-provider]")).toHaveCount(63);
  await expect(routingTree.locator("[data-route-weight-access]")).toHaveCount(2);
  await expect(routingTree.locator("[data-route-capability]")).toHaveCount(8);
  await expect(proprietaryFilter).toHaveAttribute("aria-pressed", "true");
  await expect(openWeightsFilter).toHaveAttribute("aria-pressed", "false");
  await expect(textFilter).toHaveAttribute("aria-pressed", "true");
  await expect(routingTree.locator('[data-route-weight-access][aria-pressed="true"]')).toHaveCount(1);
  await expect(routingTree.locator('[data-route-capability][aria-pressed="true"]')).toHaveCount(1);
  await expect(counts).toHaveText("12 families · 39 exact models · 39 offerings");
  await expect(routingTree.locator("[data-route-family]:visible")).toHaveCount(12);
  await expect(routingTree.locator("[data-route-model]:visible")).toHaveCount(1);
  await expect(routingTree.locator("[data-route-provider]:visible")).toHaveCount(1);
  await expect(selectedModel).toHaveText("claude-fable-5");
  await expect(selectedProvider).toHaveText("anthropic");
  await expectRoutingHeaderSingleRow(routingTree);

  const routeCanvasDimensions = await routingTree.locator("[data-route-canvas]").evaluate((canvas) => ({
    height: canvas.height,
    width: canvas.width,
  }));
  expect(routeCanvasDimensions.height).toBeGreaterThan(0);
  expect(routeCanvasDimensions.width).toBeGreaterThan(0);
  await expectFiveStageRouteOrder(routingTree);
  await expectSelectedRoutingFanEndpoints(routingTree);

  await imageInputFilter.click();
  await expect(imageInputFilter).toHaveAttribute("aria-pressed", "true");
  await expect(textFilter).toHaveAttribute("aria-pressed", "false");
  await expect(counts).toHaveText("9 families · 26 exact models · 26 offerings");
  await expect(routingTree.locator("[data-route-family]:visible")).toHaveCount(9);
  await openWeightsFilter.click();
  await expect(counts).toHaveText("11 families · 30 exact models · 30 offerings");
  await expect(routingTree.locator('[data-route-family="kimi-k2"]')).toBeVisible();
  await expect(routingTree.locator('[data-route-family="kimi-k3"]')).toBeVisible();
  await openWeightsFilter.click();
  await routingTree.locator('[data-route-family="grok"]').click();
  await expect(routingTree.locator('[data-route-model-group="grok"] [data-route-model]:visible')).toHaveCount(1);
  await expect(routingTree.locator('[data-route-model="grok-4.5"]')).toHaveAttribute("aria-pressed", "true");
  await expect(routingTree.locator('[data-route-provider-group="grok-4.5"] [data-route-provider="xai"]')).toHaveAttribute("aria-pressed", "true");
  await expect(selectedModel).toHaveText("grok-4.5");
  await expect(selectedProvider).toHaveText("xai");

  await audioInputFilter.click();
  await expect(audioInputFilter).toHaveAttribute("aria-pressed", "true");
  await expect(imageInputFilter).toHaveAttribute("aria-pressed", "false");
  await expect(counts).toHaveText("1 family · 2 exact models · 2 offerings");
  await expect(routingTree.locator("[data-route-family]:visible")).toHaveCount(1);
  await expect(routingTree.locator('[data-route-family="gemini"]')).toHaveAttribute("aria-pressed", "true");
  await expect(routingTree.locator('[data-route-model-group="gemini"] [data-route-model]')).toHaveCount(2);
  await expect(selectedProvider).toHaveText("gemini");

  await textFilter.click();
  await routingTree.locator('[data-route-family="claude-fable"]').click();
  await expect(counts).toHaveText("12 families · 39 exact models · 39 offerings");
  await expect(selectedModel).toHaveText("claude-fable-5");
  await expect(selectedProvider).toHaveText("anthropic");

  await openWeightsFilter.click();
  await expect(openWeightsFilter).toHaveAttribute("aria-pressed", "true");
  await expect(proprietaryFilter).toHaveAttribute("aria-pressed", "true");
  await expect(routingTree.locator('[data-route-weight-access][aria-pressed="true"]')).toHaveCount(2);
  await expect(counts).toHaveText("19 families · 56 exact models · 57 offerings");
  await expect(routingTree.locator("[data-route-family]:visible")).toHaveCount(19);
  await expect(selectedModel).toHaveText("claude-fable-5");
  await expect(selectedProvider).toHaveText("anthropic");

  await proprietaryFilter.click();
  await expect(openWeightsFilter).toHaveAttribute("aria-pressed", "true");
  await expect(proprietaryFilter).toHaveAttribute("aria-pressed", "false");
  await expect(routingTree.locator('[data-route-weight-access][aria-pressed="true"]')).toHaveCount(1);
  await expect(counts).toHaveText("7 families · 17 exact models · 18 offerings");
  await expect(routingTree.locator("[data-route-family]:visible")).toHaveCount(7);
  await expect(routingTree.locator('[data-route-family="deepseek-r1"]')).toHaveAttribute("aria-pressed", "true");
  const deepSeekModels = routingTree.locator('[data-route-model-group="deepseek-r1"]');
  await expect(deepSeekModels).toBeVisible();
  await expect(deepSeekModels.locator("[data-route-model]")).toHaveCount(1);
  await expect(deepSeekModels.locator('[data-route-model="deepseek-reasoner"]')).toHaveAttribute("aria-pressed", "true");
  const reasonerProviders = routingTree.locator('[data-route-provider-group="deepseek-reasoner"]');
  await expect(reasonerProviders).toBeVisible();
  await expect(reasonerProviders.locator("[data-route-provider]:visible")).toHaveCount(2);
  await expect(selectedModel).toHaveText("deepseek-reasoner");
  await expect(selectedProvider).toHaveText("deepseek");
  await reasonerProviders.locator('[data-route-provider="siliconflow"]').click();
  await expect(selectedProvider).toHaveText("siliconflow");
  await openWeightsFilter.click();
  await expect(openWeightsFilter).toHaveAttribute("aria-pressed", "true");
  await expect(routingTree.locator('[data-route-weight-access][aria-pressed="true"]')).toHaveCount(1);
  await expect(counts).toHaveText("7 families · 17 exact models · 18 offerings");
  await expectFiveStageRouteOrder(routingTree);
  await expectSelectedRoutingFanEndpoints(routingTree);

  await webSearchFilter.click();
  await expect(webSearchFilter).toHaveAttribute("aria-pressed", "true");
  await expect(textFilter).toHaveAttribute("aria-pressed", "false");
  await expect(routingTree.locator('[data-route-capability][aria-pressed="true"]')).toHaveCount(1);
  await expect(counts).toHaveText("0 families · 0 exact models · 0 offerings");
  await expect(routingTree.locator("[data-route-empty]")).toBeVisible();
  await expect(routingTree.locator("[data-route-stage]:visible")).toHaveCount(0);
  await expect(routingTree.locator("[data-route-selection]")).toBeHidden();
  await expect(routingTree).toHaveAttribute("data-route-lines-rendered", "true");
  await webSearchFilter.click();
  await expect(webSearchFilter).toHaveAttribute("aria-pressed", "true");
  await expect(counts).toHaveText("0 families · 0 exact models · 0 offerings");
  await textFilter.click();
  await expect(textFilter).toHaveAttribute("aria-pressed", "true");
  await expect(webSearchFilter).toHaveAttribute("aria-pressed", "false");
  await expect(routingTree.locator('[data-route-capability][aria-pressed="true"]')).toHaveCount(1);
  await expect(counts).toHaveText("7 families · 17 exact models · 18 offerings");
  await expect(routingTree.locator("[data-route-empty]")).toBeHidden();
  await expect(selectedModel).toHaveText("deepseek-reasoner");
  await expect(selectedProvider).toHaveText("deepseek");

  await proprietaryFilter.click();
  await expect(openWeightsFilter).toHaveAttribute("aria-pressed", "true");
  await expect(proprietaryFilter).toHaveAttribute("aria-pressed", "true");
  await expect(routingTree.locator('[data-route-weight-access][aria-pressed="true"]')).toHaveCount(2);
  await expect(counts).toHaveText("19 families · 56 exact models · 57 offerings");
  await openWeightsFilter.click();
  await expect(openWeightsFilter).toHaveAttribute("aria-pressed", "false");
  await expect(proprietaryFilter).toHaveAttribute("aria-pressed", "true");
  await expect(routingTree.locator('[data-route-weight-access][aria-pressed="true"]')).toHaveCount(1);
  await expect(counts).toHaveText("12 families · 39 exact models · 39 offerings");
  await webSearchFilter.click();
  await expect(webSearchFilter).toHaveAttribute("aria-pressed", "true");
  await expect(textFilter).toHaveAttribute("aria-pressed", "false");
  await expect(counts).toHaveText("2 families · 9 exact models · 9 offerings");
  await expect(selectedModel).toHaveText("gpt-4.1");
  await webSearchFilter.click();
  await expect(webSearchFilter).toHaveAttribute("aria-pressed", "true");
  await expect(routingTree.locator('[data-route-capability][aria-pressed="true"]')).toHaveCount(1);
  await expect(selectedModel).toHaveText("gpt-4.1");

  const canonicalCapabilities = [
    "text",
    "dictation",
    "video_generation",
    "image_input",
    "audio_input",
    "web_search",
    "reasoning",
  ];
  await textFilter.evaluate((filter) => {
    filter.scrollIntoView({ block: "start", inline: "nearest", behavior: "instant" });
  });
  const routeFilterScrollGeometry = await textFilter.evaluate((filter) => {
    const stickyHeader = document.querySelector("body > mpr-header");
    if (!stickyHeader) {
      throw new Error("public_sticky_header_missing");
    }
    const filterBounds = filter.getBoundingClientRect();
    const headerBounds = stickyHeader.getBoundingClientRect();
    const hitTarget = document.elementFromPoint(
      filterBounds.left + (filterBounds.width / 2),
      filterBounds.top + (filterBounds.height / 2),
    );
    return {
      filterOwnsCenter: hitTarget === filter || filter.contains(hitTarget),
      filterTop: filterBounds.top,
      headerBottom: headerBounds.bottom,
    };
  });
  expect(routeFilterScrollGeometry.filterTop).toBeGreaterThanOrEqual(routeFilterScrollGeometry.headerBottom);
  expect(routeFilterScrollGeometry.filterOwnsCenter).toBe(true);
  for (const capability of canonicalCapabilities) {
    const capabilityFilter = routingTree.locator(`[data-route-capability="${capability}"]`);
    await capabilityFilter.click();
    await expect(capabilityFilter).toHaveAttribute("aria-pressed", "true");
    await expect(routingTree.locator('[data-route-capability][aria-pressed="true"]')).toHaveCount(1);
    await expect(counts).not.toHaveText(/^0 families/u);
    expect(await routingTree.locator("[data-route-family]:visible").count()).toBeGreaterThan(0);
  }

  await textFilter.click();
  await routingTree.locator('[data-route-family="gpt-5"]').click();
  const gptModels = routingTree.locator('[data-route-model-group="gpt-5"]');
  await expect(gptModels).toBeVisible();
  await expect(gptModels.locator("[data-route-model]")).toHaveCount(8);
  await gptModels.locator('[data-route-model="gpt-5.6-terra"]').click();
  await expect(selectedModel).toHaveText("gpt-5.6-terra");
  await expect(selectedProvider).toHaveText("openai");
  await expectFiveStageRouteOrder(routingTree);
  await expectSelectedRoutingFanEndpoints(routingTree);
  await routingTree.locator('[data-route-capability="reasoning"]').click();
  await expect(routingTree.locator('[data-route-capability][aria-pressed="true"]')).toHaveCount(1);
  await expect(selectedModel).toHaveText("gpt-5.6-terra");
  await expect(selectedProvider).toHaveText("openai");
  await textFilter.click();
  await expect(routingTree.locator('[data-route-capability][aria-pressed="true"]')).toHaveCount(1);
  await expect(selectedModel).toHaveText("gpt-5.6-terra");
  await expect(selectedProvider).toHaveText("openai");
  await textFilter.click();
  await expect(textFilter).toHaveAttribute("aria-pressed", "true");
  await expect(routingTree.locator('[data-route-capability][aria-pressed="true"]')).toHaveCount(1);
  await expect(selectedModel).toHaveText("gpt-5.6-terra");
  await expect(selectedProvider).toHaveText("openai");

  await page.setViewportSize({ width: 900, height: 900 });
  await expect(routingTree).toHaveAttribute("data-route-lines-rendered", "true");
  await expectRoutingHeaderSingleRow(routingTree);
  await expectFiveStageRouteOrder(routingTree);
  await expectSelectedRoutingFanEndpoints(routingTree);

  if (process.env.F034_SCREENSHOTS === "1") {
    await mkdir(f034ScreenshotDirectory, { recursive: true });
    await openWeightsFilter.click();
    await expect(routingTree.locator('[data-route-weight-access][aria-pressed="true"]')).toHaveCount(2);
    for (const viewport of routingTreeViewports) {
      await page.setViewportSize({ width: viewport.width, height: viewport.height });
      await expect(routingTree).toHaveAttribute(
        "data-route-lines-rendered",
        viewport.width > 680 ? "true" : "false",
      );
      await expectRoutingHeaderSingleRow(routingTree);
      await routingTree.screenshot({ path: path.join(f034ScreenshotDirectory, `F034-routing-${viewport.name}.png`) });
      const routingHeader = routingTree.locator(".routing-tree__header");
      await routingHeader.scrollIntoViewIfNeeded();
      await page.evaluate(() => window.scrollBy(0, -80));
      await routingHeader.screenshot({ path: path.join(f034ScreenshotDirectory, `F034-header-${viewport.name}.png`) });
    }
    await openWeightsFilter.click();
    await expect(routingTree.locator('[data-route-weight-access][aria-pressed="true"]')).toHaveCount(1);
  }

  await page.setViewportSize({ width: 390, height: 780 });
  await expect(routingTree).toHaveAttribute("data-route-lines-rendered", "false");
  await expectRoutingHeaderSingleRow(routingTree);
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  const routingTreeGeometry = await routingTree.evaluate((element) => ({
    left: element.getBoundingClientRect().left,
    right: element.getBoundingClientRect().right,
    viewportWidth: document.documentElement.clientWidth,
  }));
  expect(routingTreeGeometry.left).toBeGreaterThanOrEqual(0);
  expect(routingTreeGeometry.right).toBeLessThanOrEqual(routingTreeGeometry.viewportWidth);
});
test("visitors can disclose filters, search every characteristic, and sort through table headers", async ({ page }) => {
  await installAssetRoutes(page, { initialAuthStatus: "unauthenticated" });
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto(`${baseURL}/#models`);

  const catalog = page.locator("capability-catalog");
  const rows = catalog.locator("[data-catalog-row]");
  const visibleRows = catalog.locator("[data-catalog-row]:visible");
  const resultCount = catalog.locator("[data-catalog-result-count]");
  const searchInput = catalog.getByLabel("Search all model characteristics");
  const searchSubmit = catalog.getByRole("button", { name: "Toggle capability filters" });
  const filterPanel = catalog.locator("[data-catalog-filter-panel]");
  const publisherHeader = catalog.locator('[data-catalog-sort-header="publisher"]');
  const modelHeader = catalog.locator('[data-catalog-sort-header="model"]');
  const capabilitiesHeader = catalog.locator('[data-catalog-sort-header="capabilities"]');
  await expect(catalog).toHaveAttribute("data-enhanced", "true");
  await expect(rows).toHaveCount(62);
  await expect(resultCount).toHaveText("62 of 62 models");
  await expect(filterPanel).toBeHidden();
  await expect(searchSubmit).toHaveAttribute("aria-expanded", "false");

  await searchSubmit.click();
  await expect(filterPanel).toBeVisible();
  await expect(searchSubmit).toHaveAttribute("aria-expanded", "true");

  await searchSubmit.click();
  await expect(filterPanel).toBeHidden();
  await expect(searchSubmit).toHaveAttribute("aria-expanded", "false");

  await searchInput.focus();
  await expect(filterPanel).toBeVisible();
  await expect(searchSubmit).toHaveAttribute("aria-expanded", "true");
  await searchInput.press("Escape");
  await expect(filterPanel).toBeHidden();
  await expect(searchSubmit).toHaveAttribute("aria-expanded", "false");
  await expect(catalog.locator('[data-catalog-search-text*="background"]')).toHaveCount(0);
  await expect(catalog.locator('[data-catalog-search-text*="synchronous"]')).toHaveCount(0);
  await searchInput.fill("gemini-3.5-flash image_input audio_input");
  await expect(filterPanel).toBeVisible();
  await expect(resultCount).toHaveText("1 of 62 model");
  await expect(visibleRows).toHaveAttribute("data-model", "gemini-3.5-flash");

  await searchInput.fill("gpt-5.5-pro openai_responses xhigh");
  await expect(resultCount).toHaveText("1 of 62 model");
  await expect(visibleRows).toHaveAttribute("data-model", "gpt-5.5-pro");

  await searchInput.fill("glm-5.2 131072 token");
  await expect(resultCount).toHaveText("1 of 62 model");
  await expect(visibleRows).toHaveAttribute("data-model", "glm-5.2");

  await catalog.getByRole("button", { name: "Reset" }).click();
  await searchInput.fill("dictation");
  await expect(resultCount).toHaveText("5 of 62 models");
  for (const visibleRow of await visibleRows.all()) {
    await expect(visibleRow).toHaveAttribute("data-capabilities", "dictation");
  }

  await catalog.getByRole("button", { name: "Reset" }).click();
  await catalog.getByRole("checkbox", { name: "Image input" }).check();
  await catalog.getByRole("checkbox", { name: "Audio message input" }).check();
  await expect(resultCount).toHaveText("2 of 62 models");
  await expect(visibleRows).toHaveCount(2);

  await searchSubmit.click();
  await expect(filterPanel).toBeHidden();
  await expect(resultCount).toHaveText("2 of 62 models");
  await searchSubmit.click();
  await expect(filterPanel).toBeVisible();
  await expect(catalog.getByRole("checkbox", { name: "Image input" })).toBeChecked();
  await expect(catalog.getByRole("checkbox", { name: "Audio message input" })).toBeChecked();

  await catalog.getByRole("checkbox", { name: "Dictation" }).check();
  await expect(resultCount).toHaveText("0 of 62 models");
  await expect(catalog.locator("[data-catalog-empty]")).toBeVisible();

  await catalog.getByRole("button", { name: "Reset" }).click();
  await expect(resultCount).toHaveText("62 of 62 models");
  await searchInput.press("Escape");
  await catalog.getByRole("button", { name: "Filter by Dictation" }).first().click();
  await expect(filterPanel).toBeVisible();
  await expect(catalog.getByRole("checkbox", { name: "Dictation" })).toBeChecked();
  await expect(resultCount).toHaveText("5 of 62 models");

  await catalog.getByRole("button", { name: "Reset" }).click();
  await expect(publisherHeader).toHaveAttribute("aria-sort", "ascending");
  await catalog.getByRole("button", { name: "Sort by Publisher descending" }).click();
  await expect(publisherHeader).toHaveAttribute("aria-sort", "descending");
  await expect
    .poll(async () => rows.evaluateAll((elements) => {
        const publishersAndModels = elements.map((element) => [
          element.getAttribute("data-publisher") || "",
          element.getAttribute("data-model") || "",
        ]);
        return publishersAndModels.every((entry, index) => {
          if (index === 0) {
            return true;
          }
          const previous = publishersAndModels[index - 1];
          return previous[0].localeCompare(entry[0], undefined, { sensitivity: "base" }) > 0 ||
            (previous[0].localeCompare(entry[0], undefined, { sensitivity: "base" }) === 0 &&
              previous[1].localeCompare(entry[1], undefined, { sensitivity: "base" }) >= 0);
        });
      }))
    .toBe(true);

  await catalog.getByRole("button", { name: "Sort by Model ascending" }).click();
  await expect(modelHeader).toHaveAttribute("aria-sort", "ascending");
  await expect(publisherHeader).not.toHaveAttribute("aria-sort");
  await expect
    .poll(async () => rows.evaluateAll((elements) => {
        const models = elements.map((element) => element.getAttribute("data-model") || "");
        return models.every(
          (model, index) => index === 0 || models[index - 1].localeCompare(model, undefined, { sensitivity: "base" }) <= 0,
        );
      }))
    .toBe(true);

  await catalog.getByRole("button", { name: "Sort by Model descending" }).click();
  await expect(modelHeader).toHaveAttribute("aria-sort", "descending");
  await catalog.getByRole("button", { name: "Sort by Capabilities descending" }).click();
  await expect(capabilitiesHeader).toHaveAttribute("aria-sort", "descending");
  await expect
    .poll(async () => rows.evaluateAll((elements) => {
        const capabilityCounts = elements.map((element) => Number(element.getAttribute("data-capability-count")));
        return capabilityCounts.every(
          (capabilityCount, index) => index === 0 || capabilityCounts[index - 1] >= capabilityCount,
        );
      }))
    .toBe(true);

  await catalog.getByRole("button", { name: "Reset" }).click();
  await expect(publisherHeader).toHaveAttribute("aria-sort", "ascending");
  await expect(modelHeader).not.toHaveAttribute("aria-sort");
  await expect(capabilitiesHeader).not.toHaveAttribute("aria-sort");

  await searchInput.focus();
  await searchInput.press("Escape");
  await expect(filterPanel).toBeHidden();
  await searchInput.press("Enter");
  await expect(filterPanel).toBeVisible();

  await page.setViewportSize({ width: 390, height: 780 });
  await expect(searchInput).toBeVisible();
  await expect(filterPanel).toBeVisible();
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth === document.documentElement.clientWidth),
  ).toBe(true);
});

test("every public HTML page publishes its canonical MPR header and the identical footer", async ({ request }) => {
  const sitemapResponse = await request.get(`${baseURL}${sitemapPath}`);
  expect(sitemapResponse.status()).toBe(httpOK);
  const sitemapXML = await sitemapResponse.text();
  const publicPaths = [...sitemapXML.matchAll(/<loc>([^<]+)<\/loc>/g)].map((match) => new URL(match[1]).pathname);
  expect(publicPaths).toHaveLength(generatedResourcePageCount + 5);

  let canonicalFooter = "";
  for (const publicPath of publicPaths) {
    const response = await request.get(`${baseURL}${publicPath}`);
    expect(response.status(), publicPath).toBe(httpOK);
    const html = await response.text();
    const shell = publicShellMarkup(html);
    canonicalFooter ||= shell.footer;
    expect(shell.header, publicPath).toBe(publicPath === "/" ? renderLandingHeader() : renderPublicHeader());
    expect(shell.footer, publicPath).toBe(canonicalFooter);
    expect(html, publicPath).toContain(`<link rel="stylesheet" href="${mprUICSSURL}">`);
    expect(html, publicPath).toContain(`<script defer src="${LOOPAWARE_PIXEL_URL}"></script>`);
    expect(html, publicPath).toContain('<link rel="stylesheet" href="/assets/llm-proxy/public-shell.css">');
    expect(html, publicPath).toContain(`<script src="${mprUIConfigURL}"></script>`);
    expect(html, publicPath).toContain(`data-mpr-ui-bundle-src="${mprUIBundleURL}"`);
    expect(html, publicPath).toContain(`data-config-url="${configPath}"`);
    if (publicPath === "/") {
      expect(shell.header, publicPath).toContain(
        `${LANDING_AUTHENTICATED_REDIRECT_ATTRIBUTE}="${applicationPath}"`,
      );
      expect(shell.header, publicPath).not.toContain("sign-in-redirect-url=");
      expect(html, publicPath).toContain(
        `<script type="module" src="/assets/llm-proxy/js/ui/landingAuthRoute.js?v=${landingAuthRouteModuleRevision}"></script>`,
      );
    } else {
      expect(shell.header, publicPath).toContain(`sign-in-redirect-url="${applicationPath}"`);
      expect(shell.header, publicPath).not.toContain(LANDING_AUTHENTICATED_REDIRECT_ATTRIBUTE);
      expect(html, publicPath).not.toContain("/assets/llm-proxy/js/ui/landingAuthRoute.js");
    }
    expect(html, publicPath).not.toContain('class="resource-shell resource-topbar"');
    expect(html, publicPath).not.toContain('class="resource-shell resource-footer"');
    expect(html, publicPath).not.toContain('class="landing-header"');
    expect(html, publicPath).not.toMatch(/<a\b[^>]*\bhref=["']\/app\/["'][^>]*>/i);
    expect(shell.footer, publicPath).toContain('<a href="/resources/">Resources</a>');
    expect(shell.footer, publicPath).toContain('<a href="/privacy/">Privacy</a>');
    expect(shell.footer, publicPath).toContain('<a href="/terms/">Terms</a>');
    expect(shell.footer, publicPath).toContain('sticky="true"');
    expect(shell.footer, publicPath).toContain('theme-switcher="square"');
    expect(shell.footer, publicPath).toContain('"initialMode":"default-dark"');
    expect(shell.footer, publicPath).toContain('"value":"default-light"');
    expect(shell.footer, publicPath).toContain('"value":"sunrise-light"');
    expect(shell.footer, publicPath).toContain('"value":"default-dark"');
    expect(shell.footer, publicPath).toContain('"value":"forest-dark"');
    expect(shell.headerOffset, publicPath).toBeLessThan(shell.mainOffset);
    expect(shell.mainOffset, publicPath).toBeLessThan(shell.footerOffset);
  }
});

test("the public resource generator rejects incomplete or unsafe shell documents", async () => {
  const sourcePath = path.join(siteSourceRoot, "resources", "index.html");
  const source = await readFile(sourcePath, "utf8");
  expect(() => assertPublicDocumentShell(source, sourcePath)).not.toThrow();

  const invalidDocuments = [
    source.replace("<mpr-header", "<invalid-header"),
    source.replace("<main", "<invalid-main"),
    source.replace("<mpr-footer", "<invalid-footer"),
    source.replace("</mpr-header>", "</mpr-header><p>Escaped resource content</p>"),
    source.replace("</main>", '<a href="/app/">Bypass authentication</a></main>'),
    source.replace('<a href="/resources/">Resources</a>', '<a href="/resource/">Resources</a>'),
    source.replace(
      `<script src="${mprUIConfigURL}"></script>`,
      `<script src="${forbiddenTAuthBrowserClientURL}"></script><script src="${mprUIConfigURL}"></script>`,
    ),
  ];
  for (const invalidDocument of invalidDocuments) {
    expect(() => assertPublicDocumentShell(invalidDocument, sourcePath)).toThrow();
  }
});

test("legal pages publish one repo-grounded source with static and MPR-rendered semantics", async ({ request }) => {
  const legalPages = [
    {
      path: privacyPath,
      type: "privacy",
      title: "Privacy Policy - LLM Proxy",
      requiredCopy: "usage records do not store prompts",
    },
    {
      path: termsPath,
      type: "terms",
      title: "Terms of Service - LLM Proxy",
      requiredCopy: "Model outputs may be incomplete",
    },
  ];

  for (const legalPage of legalPages) {
    const response = await request.get(`${baseURL}${legalPage.path}`);
    expect(response.status(), legalPage.path).toBe(httpOK);
    const html = await response.text();
    expect(html, legalPage.path).toContain(`<link rel="canonical" href="https://llm-proxy.mprlab.com${legalPage.path}">`);
    expect(html, legalPage.path).toContain(`<mpr-legal-document\n        type="${legalPage.type}"`);
    expect(html, legalPage.path).toContain(`<h1 class="mpr-legal-document__title">${legalPage.title}</h1>`);
    expect(html, legalPage.path).toContain(legalPage.requiredCopy);
    expect(html, legalPage.path).toContain('effective-date="2026-08-08"');
    expect(html, legalPage.path).toContain('last-updated-date="2026-08-08"');
  }
});

test("the published site uses only app, login, account, and tenant terminology", async ({ request }) => {
  const sitemapResponse = await request.get(`${baseURL}${sitemapPath}`);
  expect(sitemapResponse.status()).toBe(httpOK);
  const sitemapXML = await sitemapResponse.text();
  expect(sitemapXML).not.toMatch(obsoletePublicTermPattern);
  expect(sitemapXML).toContain(
    "<loc>https://llm-proxy.mprlab.com/resources/multi-tenant-ownership-migration/</loc>",
  );

  const publicPaths = [...sitemapXML.matchAll(/<loc>([^<]+)<\/loc>/g)].map((match) => new URL(match[1]).pathname);
  for (const publicPath of [...publicPaths, applicationPath]) {
    const response = await request.get(`${baseURL}${publicPath}`);
    expect(response.status(), publicPath).toBe(httpOK);
    expect(await response.text(), publicPath).not.toMatch(obsoletePublicTermPattern);
  }
});

test("public route families hydrate the shared MPR shell at desktop and mobile widths", async ({ page }) => {
  await installAssetRoutes(page, { initialAuthStatus: "unauthenticated" });
  const representativePublicPaths = ["/", apiDocumentationPath, resourcesPath, representativeResourcePath, privacyPath, termsPath];

  for (const viewport of [
    { width: 1280, height: 800 },
    { width: 390, height: 780 },
  ]) {
    await page.setViewportSize(viewport);
    for (const publicPath of representativePublicPaths) {
      await page.goto(`${baseURL}${publicPath}`);
      const header = page.locator("mpr-header");
      const main = page.locator("body > main");
      const footer = page.locator("mpr-footer");
      await expect(header, publicPath).toHaveCount(1);
      await expect(main, publicPath).toHaveCount(1);
      await expect(footer, publicPath).toHaveCount(1);
      await expect(footer, publicPath).toHaveAttribute("sticky", "true");
      await expect(header.getByRole("link", { name: "LLM Proxy home" }), publicPath).toBeVisible();
      await expect(header.getByRole("link", { name: "API", exact: true }), publicPath).toHaveAttribute(
        "href",
        apiDocumentationPath,
      );
      await expect(header.getByRole("button", { name: "Log In" }), publicPath).toBeVisible();
      await expect(footer.getByRole("contentinfo"), publicPath).toBeVisible();
      await expect(footer.getByRole("link", { name: "Resources" }), publicPath).toHaveAttribute("href", resourcesPath);
      await expect(footer.getByRole("link", { name: "Privacy" }), publicPath).toHaveAttribute("href", privacyPath);
      await expect(footer.getByRole("link", { name: "Terms" }), publicPath).toHaveAttribute("href", termsPath);
      const documentOrderIsCanonical = await page.evaluate(() => {
        const headerElement = document.querySelector("body > mpr-header");
        const mainElement = document.querySelector("body > main");
        const footerElement = document.querySelector("body > mpr-footer");
        if (!headerElement || !mainElement || !footerElement) {
          return false;
        }
        return Boolean(
          headerElement.compareDocumentPosition(mainElement) & Node.DOCUMENT_POSITION_FOLLOWING &&
            mainElement.compareDocumentPosition(footerElement) & Node.DOCUMENT_POSITION_FOLLOWING,
        );
      });
      expect(documentOrderIsCanonical, publicPath).toBe(true);
      await expectStickyFooterGeometry(page, footer);
    }
  }
});

test("public landing is keyboard navigable and responsive in Chromium", async ({ page }) => {
  await installAssetRoutes(page, { initialAuthStatus: "unauthenticated" });
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto(baseURL);

  await expect(page.getByRole("heading", { level: 1 })).toHaveText(
    "Integrate once. Use the model that fits.",
  );
  const routingOverview = page.locator("#routing-overview");
  await expect(routingOverview).toBeVisible();
  await expect(routingOverview.getByText("One endpoint", { exact: true })).toBeVisible();
  await expect(routingOverview.getByText("One credential", { exact: true })).toBeVisible();
  await expect(routingOverview.getByText("One contract", { exact: true })).toBeVisible();
  await expect(routingOverview.locator("routing-tree")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Use the API directly or start with a client." })).toBeVisible();
  await expect(page.getByRole("link", { name: "Use the Go client" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Use the Python client" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Install the CLI" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "One boundary. Three ways to benefit." })).toBeVisible();
  await expect(page.getByRole("button", { name: "Log In" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Exact model provider offering matrix" })).toBeVisible();
  await expect(page.getByRole("table")).toBeVisible();
  const reasonerRow = page.locator('[data-publisher="deepseek"][data-model="deepseek-reasoner"]');
  await expect(reasonerRow.locator(".catalog-offerings > li")).toHaveCount(2);
  await expect(reasonerRow).toContainText("DeepSeek");
  await expect(reasonerRow).toContainText("SiliconFlow");
  await expect(reasonerRow).not.toContainText("deepseek-ai/DeepSeek-R1");
  await expectAlignedCatalogRows(page);
  await expectCenteredValueStrip(page);
  const footer = page.locator("mpr-footer");
  await expect(footer).toHaveAttribute("size", "small");
  await expect(footer).toHaveAttribute("sticky", "true");
  await expect(footer.getByRole("contentinfo")).toBeVisible();
  await expect(footer.getByRole("link", { name: "Privacy" })).toHaveAttribute("href", privacyPath);
  await expect(footer.getByRole("link", { name: "Terms" })).toHaveAttribute("href", termsPath);
  await expect(footer.getByRole("link", { name: "Resources" })).toHaveAttribute("href", resourcesPath);
  await expect(footer.getByRole("link", { name: "GitHub" })).toHaveAttribute(
    "href",
    "https://github.com/tyemirov/llm-proxy",
  );
  await expectStickyFooterGeometry(page, footer);

  await page.keyboard.press("Tab");
  await expect(page.getByRole("link", { name: "Skip to content" })).toBeFocused();

  await page.setViewportSize({ width: 390, height: 780 });
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  await expect(page.getByRole("button", { name: "Log In" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Exact model provider offering matrix" })).toBeVisible();
  await expect(reasonerRow.locator(".catalog-offerings > li")).toHaveCount(2);
  await expectAlignedCatalogRows(page);
  await expectCenteredValueStrip(page);
  await expectStickyFooterGeometry(page, footer);
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
  expect(documentationHTML).toContain('<section id="openapi-schema" class="resource-hero">');
  expect(documentationHTML).toContain('<a class="resource-button" href="#openapi-yaml">View YAML</a>');
  expect(documentationHTML).toContain(
    `<a class="resource-button" href="${openAPIPath}" download="${openAPIDownloadFilename}">Download YAML</a>`,
  );
  expect(documentationHTML).toContain('id="operation-postV2Messages"');
  expect(documentationHTML).toContain('id="operation-getV2StructuredRequest"');
  expect(documentationHTML).toContain('id="operation-uploadTenantAsset"');
  expect(documentationHTML).toContain('id="operation-deleteTenantAsset"');
  expect(documentationHTML).not.toContain('id="operation-deleteManagementTenantSecret"');
  expect(documentationHTML).toContain("<code>reasoning_effort</code>");
  expect(documentationHTML).toContain(`href="${openAPIPath}"`);
  expect(documentationHTML.match(/<section class="api-operation"/g) || []).toHaveLength(31);
});

test("OpenCode integration opens the bearer-authenticated client API reference", async ({ page }) => {
  await installAssetRoutes(page, { initialAuthStatus: "unauthenticated" });
  await page.goto(baseURL);
  await page.getByRole("link", { name: /Read the client API reference/u }).click();
  await expect(page).toHaveURL(`${baseURL}${apiDocumentationPath}#operation-createChatCompletion`);
  const operation = page.locator("#operation-createChatCompletion");
  await expect(operation).toContainText("Authorization: Bearer <tenant-client-key>");
  await expect(operation).toContainText("/v1/chat/completions");
});

test("OpenAPI reference views and downloads the exact canonical YAML", async ({ page }) => {
  const canonicalSource = await readFile(canonicalOpenAPIFile, "utf8");
  await installAssetRoutes(page);
  await page.goto(`${baseURL}${openAPISchemaViewerPath}`);

  const rawViewLink = page.getByRole("link", { name: "View YAML", exact: true });
  const downloadLink = page.getByRole("link", { name: "Download YAML" });
  await expect(rawViewLink).toHaveAttribute("href", "#openapi-yaml");
  await expect(downloadLink).toHaveAttribute("href", openAPIPath);
  await expect(downloadLink).toHaveAttribute("download", openAPIDownloadFilename);

  await rawViewLink.click();
  await expect(page).toHaveURL(`${baseURL}${apiDocumentationPath}#openapi-yaml`);
  const sourceViewer = page.locator(".api-schema-source");
  await expect(sourceViewer).toBeVisible();
  expect(await sourceViewer.textContent()).toBe(canonicalSource);

  const downloadStarted = page.waitForEvent("download");
  await page.getByRole("link", { name: "Download YAML" }).click();
  const download = await downloadStarted;
  expect(download.suggestedFilename()).toBe(openAPIDownloadFilename);
  const downloadedPath = await download.path();
  expect(downloadedPath).not.toBeNull();
  expect(await readFile(downloadedPath, "utf8")).toBe(canonicalSource);
});

test("SEO resource pages are crawlable from the public site", async ({ request }) => {
  const hubResponse = await request.get(`${baseURL}${resourcesPath}`);
  expect(hubResponse.status()).toBe(httpOK);
  expect(hubResponse.headers()["content-type"]).toContain(mimeTypes[".html"]);
  const hubHTML = await hubResponse.text();
  expect(hubHTML).toContain("Keep your model options open.");
  expect(hubHTML).toContain("Official Go client");
  expect(hubHTML).toContain("Official Python client");
  expect(hubHTML).toContain("Official CLI");
  expect(hubHTML).toContain("AI-assisted builders");
  expect(hubHTML).toContain("Startups and product teams");
  expect(hubHTML).toContain("Platform and engineering teams");
  expect(hubHTML).toContain('<script defer src="/assets/llm-proxy/js/googleAnalytics.js"></script>');
  expect(hubHTML).toContain('<link rel="canonical" href="https://llm-proxy.mprlab.com/resources/">');
  expect(hubHTML).toContain('"@type":"CollectionPage"');
  expect(hubHTML).toContain(`href="${apiDocumentationPath}"`);
  expect(hubHTML).toContain(`href="${representativeResourcePath}"`);
  expect(hubHTML).toContain(`href="${clientAuthenticationResourcePath}"`);
  const resourceLinks = hubHTML.match(/href="\/resources\/[^"]+\/"/g) || [];
  expect(new Set(resourceLinks).size).toBe(generatedResourcePageCount);

  const pageResponse = await request.get(`${baseURL}${representativeResourcePath}`);
  expect(pageResponse.status()).toBe(httpOK);
  expect(pageResponse.headers()["content-type"]).toContain(mimeTypes[".html"]);
  const pageHTML = await pageResponse.text();
  expect(pageHTML).toContain("<h1>Integrate once through one multi-provider LLM proxy</h1>");
  expect(pageHTML).toContain('<script defer src="/assets/llm-proxy/js/googleAnalytics.js"></script>');
  expect(pageHTML).toContain('<link rel="canonical" href="https://llm-proxy.mprlab.com/resources/multi-provider-llm-proxy/">');
  expect(pageHTML).toContain('"@type":"FAQPage"');
  expect(pageHTML).toContain('"author":{"@type":"Person","name":"Tyemirov","url":"https://github.com/tyemirov"}');
  expect(pageHTML).toContain(`<a class="resource-button" href="${apiDocumentationPath}">Open API reference</a>`);
  expect(pageHTML).toContain('href="/resources/openai-claude-gemini-one-endpoint/"');
  expect(pageHTML).toContain(`"dateModified":"${landingModifiedDate}"`);
  expect(pageHTML).toContain("https://llm-proxy-api.mprlab.com/v2?key=$LLM_PROXY_DEFAULT_TENANT_KEY&amp;provider=gemini");
  expect(pageHTML).toContain(`Reviewed ${landingModifiedDate}`);
  expect(pageHTML).toContain('href="https://github.com/tyemirov" rel="author"');

  const audienceResourceExpectations = [
    {
      path: "/resources/openai-claude-gemini-one-endpoint/",
      title: "Switch OpenAI, Claude, and Gemini behind one endpoint",
      audience: "Startups and product teams",
      evidence: "for provider in openai anthropic gemini; do",
      modifiedDate: seoProviderCatalogModifiedDate,
    },
    {
      path: "/resources/internal-ai-gateway-for-product-tools/",
      title: "Internal AI gateway for durable product integrations",
      audience: "Institutional engineering and platform teams",
      evidence: "$PRODUCT_TOOL_CLIENT_KEY",
      modifiedDate: landingModifiedDate,
    },
  ];
  for (const resourceExpectation of audienceResourceExpectations) {
    const audienceResourceResponse = await request.get(`${baseURL}${resourceExpectation.path}`);
    expect(audienceResourceResponse.status()).toBe(httpOK);
    const audienceResourceHTML = await audienceResourceResponse.text();
    expect(audienceResourceHTML).toContain(`<h1>${resourceExpectation.title}</h1>`);
    expect(audienceResourceHTML).toContain(resourceExpectation.audience);
    expect(audienceResourceHTML).toContain(resourceExpectation.evidence);
    expect(audienceResourceHTML).toContain("<strong>Quick verdict</strong>");
    expect(audienceResourceHTML).toContain("<h2>Repository evidence</h2>");
    expect(audienceResourceHTML).toContain(
      `"dateModified":"${resourceExpectation.modifiedDate}"`,
    );
    expect(audienceResourceHTML).toContain('href="https://github.com/tyemirov" rel="author"');
  }
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
  expect(pageHTML).toContain(`"datePublished":"${seoClientDocumentationPublishedDate}"`);
  expect(pageHTML).toContain("curl -X POST");
  expect(pageHTML).toContain("/v2?key=mysecret&amp;provider=deepseek");
  expect(pageHTML).toContain("no user-level or system-level YAML lookup");
  expect(pageHTML).toContain("LLM_PROXY_BASE_URL and LLM_PROXY_DEFAULT_TENANT_KEY");
  expect(pageHTML).toContain("config.yml and providers.yml belong to the service runtime");
  expect(pageHTML).toContain("configured MPR UI and TAuth session");
  expect(pageHTML).toContain(`"label":"GitHub","href":"${repositoryURL}"`);
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
    expect(pageHTML).toContain(`"label":"GitHub","href":"${repositoryURL}"`);
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
  expect(pageHTML).toContain("Usage keeps impossible requests visible as rejections");
  expect(pageHTML).toContain("rejected_requests");
  expect(pageHTML).toContain("provider_not_configured");
  expect(pageHTML).toContain("Independent chart toggles");
  expect(pageHTML).toContain("icon button to switch that card");
  expect(pageHTML).toContain("UTC and zero-based per-hour or per-day axes");
  expect(pageHTML).toContain("without counting them as executions or proxy failures");
  expect(pageHTML).toContain("GET /api/management/usage?interval=30d");
  expect(pageHTML).toContain("GET /api/management/usage/failures?interval=30d");
  expect(pageHTML).toContain("GET /api/management/usage/rejections?interval=30d");
  expect(pageHTML).toContain(
    "GET /api/management/tenants/:tenant_id/usage?interval=30d",
  );
  const seoReport = await readFile(
    path.join(repoRoot, "docs/marketing/seo-resource-cluster-report.md"),
    "utf8",
  );
  expect(seoReport).toContain(
    "| Account-wide managed tenant usage dashboard for LLMs | Account-wide and tenant-scoped execution aggregates, separate safe failure and rejection reports, independent local provider and model chart toggles",
  );
});

test("SEO management resources document required onboarding and secret-safe examples", async ({ request }) => {
  const resourceExpectations = [
    {
      slug: "self-service-llm-key-management",
      title: "Self-service LLM key management for internal teams",
      copy: "gives every catalog provider one tenant-bound Usage Overview card for key and profile settings",
      faqQuestion: "What lets a user leave Settings?",
      modifiedDate: landingModifiedDate,
    },
    {
      slug: "generated-secret-rotation",
      title: "Rotate generated LLM Proxy client keys with confidence",
      copy: "Request examples retain the &lt;generated-secret&gt; placeholder after creation.",
      faqQuestion: "Can the raw generated client key be retrieved later?",
      modifiedDate: seoSecretRotationModifiedDate,
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
  expect(sitemapLocations).toHaveLength(generatedResourcePageCount + 5);
  expect(sitemapXML).toContain("<loc>https://llm-proxy.mprlab.com/</loc>");
  expect(sitemapXML).toContain("<loc>https://llm-proxy.mprlab.com/resources/</loc>");
  expect(sitemapXML).toContain(
    `<loc>https://llm-proxy.mprlab.com/resources/</loc>\n    <lastmod>${seoResourceIndexModifiedDate}</lastmod>`,
  );
  expect(sitemapXML).toContain(`<loc>https://llm-proxy.mprlab.com${apiDocumentationPath}</loc>`);
  expect(sitemapXML).toContain(`<loc>https://llm-proxy.mprlab.com${privacyPath}</loc>`);
  expect(sitemapXML).toContain(`<loc>https://llm-proxy.mprlab.com${termsPath}</loc>`);
  expect(sitemapXML).toContain(
    `<loc>https://llm-proxy.mprlab.com${apiDocumentationPath}</loc>\n    <lastmod>${landingModifiedDate}</lastmod>`,
  );
  expect(sitemapXML).toContain(
    "<loc>https://llm-proxy.mprlab.com/resources/multi-provider-llm-proxy/</loc>",
  );
  const sitemapModificationDates = sitemapXML.match(/<lastmod>[^<]+<\/lastmod>/g) || [];
  expect(sitemapModificationDates).toHaveLength(generatedResourcePageCount + 5);
  expect(new Set(sitemapModificationDates)).toEqual(
    new Set([
      `<lastmod>${seoContentModifiedDate}</lastmod>`,
      `<lastmod>${seoCurrentContentModifiedDate}</lastmod>`,
      `<lastmod>${seoResourceIndexModifiedDate}</lastmod>`,
      `<lastmod>${seoUsageContentModifiedDate}</lastmod>`,
      `<lastmod>${seoSecretRotationModifiedDate}</lastmod>`,
      `<lastmod>${seoProviderCatalogModifiedDate}</lastmod>`,
      `<lastmod>${seoClientDocumentationModifiedDate}</lastmod>`,
      `<lastmod>${landingModifiedDate}</lastmod>`,
    ]),
  );
  expect(sitemapXML).toContain(
    `<loc>https://llm-proxy.mprlab.com/resources/self-service-llm-key-management/</loc>\n    <lastmod>${landingModifiedDate}</lastmod>`,
  );
  expect(sitemapXML).toContain(
    `<loc>https://llm-proxy.mprlab.com/resources/multi-tenant-ownership-migration/</loc>\n    <lastmod>${seoProviderCatalogModifiedDate}</lastmod>`,
  );
  expect(sitemapXML).toContain(
    `<loc>https://llm-proxy.mprlab.com/resources/managed-tenant-usage-dashboard/</loc>\n    <lastmod>${seoUsageContentModifiedDate}</lastmod>`,
  );
  expect(sitemapXML).toContain(
    `<loc>https://llm-proxy.mprlab.com/resources/llm-proxy-client-authentication/</loc>\n    <lastmod>${seoClientDocumentationModifiedDate}</lastmod>`,
  );
  expect(sitemapXML).toContain(
    `<loc>https://llm-proxy.mprlab.com/resources/generated-secret-rotation/</loc>\n    <lastmod>${seoSecretRotationModifiedDate}</lastmod>`,
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
  await expect(page.getByRole("heading", { name: "Unable to load LLM Proxy" })).toHaveCount(0);
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

test("tenant switching discards Settings edits without changing Usage or retaining one-time keys", async ({ page }) => {
  await installAssetRoutes(page);
  await installMultiTenantRoutes(page);
  await page.goto(baseURL + applicationPath);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").getByText("Settings").click();
  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const clientAccess = settingsDialog.getByRole("group", { name: "Tenant access" });

  await clientAccess.getByRole("button", { name: "Replace key" }).click();
  await page.getByRole("alertdialog", { name: "Replace client key?" }).getByRole("button", { name: "Replace key" }).click();
  await clientAccess.getByRole("button", { name: "Show key", exact: true }).click();
  await settingsDialog.locator("details.system-prompt-disclosure summary").click();
  await settingsDialog.locator("#routing-system-prompt-input").fill("Unsaved prompt");

  const settingsTenantSelector = settingsDialog.getByRole("combobox", { name: "Tenant" });
  await settingsTenantSelector.selectOption("tenant_2");
  const discardDialog = page.getByRole("alertdialog", { name: "Discard unsaved changes?" });
  await expect(discardDialog).toBeVisible();
  await discardDialog.getByRole("button", { name: "Discard and switch" }).click();
  await expect(settingsTenantSelector).toHaveValue("tenant_2");
  await expect(page.getByRole("combobox", { name: "Usage tenant" })).toHaveValue("");
  await expect(page.locator("body")).not.toContainText("llmp_tenant_1_generated");
  expect(await browserStorageContains(page, "llmp_tenant_1_generated")).toBe(false);
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
  await page.locator("llm-proxy-management-application").evaluate((applicationElement) => {
    const alpineRuntime = /** @type {typeof globalThis & { Alpine?: { $data: (element: Element) => any } }} */ (globalThis);
    const applicationState = alpineRuntime.Alpine?.$data(applicationElement);
    if (!applicationState) {
      throw new Error("usage_tenant_state_missing");
    }
    void applicationState.handleUsageTenantSelection({ target: { value: "tenant_1" } });
  });
  await firstUsageRequested;
  await page.locator("llm-proxy-management-application").evaluate((applicationElement) => {
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
  await page.locator("llm-proxy-management-application").evaluate((applicationElement) => {
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
  await page.locator("llm-proxy-management-application").evaluate((applicationElement) => {
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

test("dashboard shows usage and Settings excludes provider controls", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page);
  await page.goto(baseURL + applicationPath);

  await expect(page).toHaveTitle("LLM Proxy");
  await expect(page.getByRole("heading", { name: "Usage overview" })).toBeVisible();
  await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("37");
  await expect(page.locator("usage-card").filter({ hasText: "Tokens" }).locator("strong")).toHaveText("12,345");
  await expect(page.locator("usage-card").filter({ hasText: "Providers used" }).locator("strong")).toHaveText("2");
  await expect(page.locator("provider-card-grid")).toBeVisible();

  await page.getByTestId("avatar-menu").click();
  await expect(page.getByTestId("avatar-menu-item").nth(0)).toHaveText("Settings");
  await page.getByTestId("avatar-menu-item").nth(0).click();
  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  await expect(settingsDialog).toBeVisible();
  await expect(settingsDialog.getByRole("heading", { name: "Routing defaults" })).toBeVisible();
  await expect(settingsDialog.getByRole("heading", { name: "Request examples" })).toBeVisible();
  await expect(settingsDialog.locator("provider-editor")).toHaveCount(0);
  await expect(settingsDialog.getByRole("textbox", { name: /API key/ })).toHaveCount(0);
});


test("the tenant routing prompt disclosure resets with Settings context", async ({ page }) => {
  await installAssetRoutes(page);
  await installMultiTenantRoutes(page);
  await page.goto(baseURL + applicationPath);
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").getByText("Settings").click();

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const disclosure = settingsDialog.locator("details.system-prompt-disclosure");
  const prompt = settingsDialog.locator("#routing-system-prompt-input");
  await expect(prompt).toBeHidden();
  await disclosure.locator("summary").click();
  await expect(prompt).toBeVisible();
  await settingsDialog.getByRole("combobox", { name: "Tenant" }).selectOption("tenant_2");
  await expect(prompt).toBeHidden();
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

test("provider and model cards switch presentation independently without fetching", async ({ page }) => {
  let usageRequestCount = 0;
  page.on("request", (request) => {
    if (new URL(request.url()).pathname.match(/^\/api\/management\/(?:usage|tenants\/[^/]+\/usage)$/u)) {
      usageRequestCount += 1;
    }
  });
  await installAssetRoutes(page);
  await installMultiTenantRoutes(page);
  await page.goto(`${baseURL}${applicationPath}`);

  const providerBreakdown = page.locator("usage-breakdown").first();
  const modelBreakdown = page.locator("usage-breakdown").nth(1);
  await expect(providerBreakdown.getByRole("heading", { name: "Provider usage", exact: true })).toHaveCount(1);
  await expect(modelBreakdown.getByRole("heading", { name: "Model usage", exact: true })).toHaveCount(1);
  await expect(providerBreakdown.locator("header .eyebrow")).toHaveCount(0);
  await expect(modelBreakdown.locator("header .eyebrow")).toHaveCount(0);
  const providerToggle = providerBreakdown.getByRole("button");
  const modelToggle = modelBreakdown.getByRole("button");
  await expect(providerToggle).toHaveAccessibleName("Show donut chart");
  await expect(modelToggle).toHaveAccessibleName("Show donut chart");
  await expect(providerToggle).toHaveAttribute("title", "Show donut chart");
  await expect(modelToggle).toHaveAttribute("title", "Show donut chart");
  await expect(providerToggle).toHaveText("");
  await expect(modelToggle).toHaveText("");
  await expect(providerToggle.locator('[data-chart-icon="donut"]')).toBeVisible();
  await expect(modelToggle.locator('[data-chart-icon="donut"]')).toBeVisible();
  await expect(page.locator("usage-breakdown usage-row-list:visible")).toHaveCount(2);
  await expect(page.locator("usage-breakdown usage-donut:visible")).toHaveCount(0);
  const requestsBeforeModeChange = usageRequestCount;

  await providerToggle.focus();
  await providerToggle.press("Enter");
  await expect(providerToggle).toBeFocused();
  await expect(providerToggle).toHaveAccessibleName("Show bar graph");
  await expect(providerToggle.locator('[data-chart-icon="bar"]')).toBeVisible();
  await expect(modelToggle).toHaveAccessibleName("Show donut chart");
  await expect(providerBreakdown.locator("usage-row-list")).toBeHidden();
  await expect(providerBreakdown.locator("usage-donut")).toBeVisible();
  await expect(modelBreakdown.locator("usage-row-list")).toBeVisible();
  await expect(modelBreakdown.locator("usage-donut")).toBeHidden();
  expect(usageRequestCount).toBe(requestsBeforeModeChange);

  await modelToggle.click();
  await expect(modelToggle).toHaveAccessibleName("Show bar graph");
  await expect(page.locator("usage-breakdown usage-row-list:visible")).toHaveCount(0);
  await expect(page.locator("usage-breakdown usage-donut:visible")).toHaveCount(2);
  expect(usageRequestCount).toBe(requestsBeforeModeChange);

  const providerLegend = page.getByRole("list", { name: "Provider request shares" });
  expect((await providerLegend.getByRole("listitem").allTextContents()).map((value) => value.replace(/\s/gu, ""))).toEqual([
    "openai24requests·65%",
    "deepseek13requests·35%",
  ]);
  const modelLegend = page.getByRole("list", { name: "Model request shares" });
  expect((await modelLegend.getByRole("listitem").allTextContents()).map((value) => value.replace(/\s/gu, ""))).toEqual([
    "openai / gpt-4.121requests·57%",
    "deepseek / deepseek-chat13requests·35%",
    "openai / gpt-4o-mini-transcribe3requests·8%",
  ].map((value) => value.replace(/\s/gu, "")));
  const modelShares = await modelLegend.getByRole("listitem").locator("span:last-child").allTextContents();
  expect(modelShares.reduce((sum, share) => sum + Number.parseInt(share, 10), 0)).toBe(100);
  const modelRequests = await modelLegend.getByRole("listitem").locator("strong").allTextContents();
  expect(modelRequests.reduce((sum, requests) => sum + Number.parseInt(requests, 10), 0)).toBe(37);
  await expect(page.getByText("Other", { exact: true })).toHaveCount(0);

  await page.getByRole("group", { name: "Usage interval" }).getByRole("button", { name: "7 days" }).click();
  await expect(providerToggle).toHaveAccessibleName("Show bar graph");
  await expect(modelToggle).toHaveAccessibleName("Show bar graph");
  expect((await providerLegend.getByRole("listitem").innerText()).replace(/\s/gu, "")).toBe("provider-7d7requests·100%");
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(providerToggle).toHaveAccessibleName("Show bar graph");
  await expect(modelToggle).toHaveAccessibleName("Show bar graph");
  await page.getByRole("combobox", { name: "Usage tenant" }).selectOption("tenant_2");
  await expect(providerToggle).toHaveAccessibleName("Show bar graph");
  await expect(modelToggle).toHaveAccessibleName("Show bar graph");

  await page.setViewportSize({ width: 390, height: 800 });
  const application = page.locator("llm-proxy-management-application");
  const viewportGeometry = await application.evaluate((element) => ({
    scrollWidth: element.scrollWidth,
    clientWidth: element.clientWidth,
  }));
  expect(viewportGeometry.scrollWidth).toBeLessThanOrEqual(viewportGeometry.clientWidth);
  await expect(providerToggle).toBeVisible();
  await expect(modelToggle).toBeVisible();

  await page.reload();
  await expect(providerToggle).toHaveAccessibleName("Show donut chart");
  await expect(modelToggle).toHaveAccessibleName("Show donut chart");
  await modelToggle.click();
  await expect(providerToggle).toHaveAccessibleName("Show donut chart");
  await expect(modelToggle).toHaveAccessibleName("Show bar graph");

  const resetViews = await application.evaluate((applicationElement) => {
    const alpineRuntime = /** @type {typeof globalThis & { Alpine?: { $data: (element: Element) => any } }} */ (globalThis);
    const applicationState = alpineRuntime.Alpine?.$data(applicationElement);
    if (!applicationState) {
      throw new Error("usage_breakdown_state_missing");
    }
    applicationState.clearAuthenticatedState();
    return [applicationState.providerUsageBreakdownView, applicationState.modelUsageBreakdownView];
  });
  expect(resetViews).toEqual(["bar", "bar"]);
});

test("Usage time series expose UTC and metric-specific integer axes", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page);
  await page.goto(`${baseURL}${applicationPath}`);

  const requestChart = page.locator("usage-chart-panel").first();
  const tokenChart = page.locator("usage-chart-panel").nth(1);
  await expect(requestChart.locator("svg")).toHaveAttribute("viewBox", "0 0 640 240");
  await expect(tokenChart.locator("svg")).toHaveAttribute("viewBox", "0 0 640 240");
  await expect(requestChart.locator(".usage-axis-title-y")).toHaveText("Requests per day");
  await expect(tokenChart.locator(".usage-axis-title-y")).toHaveText("Tokens per day");
  await expect(requestChart.locator(".usage-axis-title-x")).toHaveText("Time (UTC)");
  await expect(tokenChart.locator(".usage-axis-title-x")).toHaveText("Time (UTC)");
  const tokenChartBounds = await tokenChart.locator("svg").boundingBox();
  const tokenXAxisBounds = await tokenChart.locator(".usage-axis-title-x").boundingBox();
  if (!tokenChartBounds || !tokenXAxisBounds) throw new Error("usage_token_x_axis_bounds_missing");
  expect(tokenXAxisBounds.y + tokenXAxisBounds.height).toBeLessThanOrEqual(tokenChartBounds.y + tokenChartBounds.height);
  await expect(requestChart.locator(".usage-x-tick text")).toHaveText([
    "2026-06-01", "2026-06-08", "2026-06-16", "2026-06-23", "2026-06-30",
  ]);
  await expect(requestChart.locator(".usage-y-tick text").first()).toHaveText("0");
  await expect(tokenChart.locator(".usage-y-tick text").first()).toHaveText("0");
  await expect(requestChart.locator(".usage-chart-point")).toHaveCount(30);
  await expect(tokenChart.locator(".usage-chart-point")).toHaveCount(30);
  const requestPoints = await requestChart.locator("polyline").getAttribute("points");
  const tokenPoints = await tokenChart.locator("polyline").getAttribute("points");
  expect(requestPoints).not.toBe(tokenPoints);
  await expect(requestChart.getByRole("list", { name: /Requests by day/u }).getByRole("listitem")).toHaveCount(30);
  await expect(tokenChart.getByRole("list", { name: /Tokens by day/u }).getByRole("listitem").last()).toContainText(
    "2026-06-30T00:00:00.000Z: 6,345 tokens",
  );

  await page.getByRole("group", { name: "Usage interval" }).getByRole("button", { name: "ALL" }).click();
  await expect(requestChart.locator(".usage-axis-title-y")).toHaveText("Requests per day");
  await expect(requestChart.locator(".usage-x-tick text")).toHaveText("2026-06-01");

  await page.getByRole("group", { name: "Usage interval" }).getByRole("button", { name: "1 day" }).click();
  await expect(requestChart.locator(".usage-axis-title-y")).toHaveText("Requests per hour");
  await expect(tokenChart.locator(".usage-axis-title-y")).toHaveText("Tokens per hour");
  await expect(requestChart.locator(".usage-x-tick text")).toHaveText(["00:00", "06:00", "12:00", "17:00", "23:00"]);
  await expect(requestChart.getByRole("list", { name: /Requests by hour/u }).getByRole("listitem")).toHaveCount(24);

  await page.unroute(usageRequestPattern());
  await page.route(usageRequestPattern(), async (route) => {
    const largeUsage = managementUsage("7d", { total_tokens: 1_250_000 });
    for (const bucket of largeUsage.buckets) {
      bucket.data.total_tokens = 0;
    }
    largeUsage.buckets.at(-1).data.total_tokens = 1_250_000;
    await route.fulfill({ status: httpOK, json: largeUsage });
  });
  await page.getByRole("group", { name: "Usage interval" }).getByRole("button", { name: "7 days" }).click();
  const compactTick = tokenChart.locator(".usage-y-tick text").filter({ hasText: "1.5M" });
  await expect(compactTick).toHaveAttribute("aria-label", "1,500,000");
  await expect(tokenChart.getByRole("list", { name: /Tokens by day/u }).getByRole("listitem").last()).toContainText(
    "1,250,000 tokens",
  );
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
  const modelBreakdownToggle = page.locator("usage-breakdown").nth(1).getByRole("button");
  await modelBreakdownToggle.click();
  try {
    await sevenDayButton.click();
    for (const intervalButton of await intervalGroup.getByRole("button").all()) {
      await expect(intervalButton).toBeDisabled();
    }
    await expect(page.getByRole("button", { name: "Refresh", exact: true })).toBeDisabled();
    await expect(sevenDayButton).toHaveAttribute("aria-pressed", "true");
    await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("0");
    await expect(page.locator("usage-chart-panel").first()).toContainText("No usage recorded");
    await expect(modelBreakdownToggle).toHaveAccessibleName("Show bar graph");
    await page.locator("llm-proxy-management-application").evaluate((applicationElement) => {
      const alpineRuntime = /** @type {typeof globalThis & { Alpine?: { $data: (element: Element) => any } }} */ (globalThis);
      const applicationState = alpineRuntime.Alpine?.$data(applicationElement);
      if (!applicationState) {
        throw new Error("usage_interval_state_missing");
      }
      void applicationState.selectUsageInterval("1d");
    });
    await expect(intervalGroup.getByRole("button", { name: "1 day" })).toHaveAttribute("aria-pressed", "true");
    await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("1");
    await expect(modelBreakdownToggle).toHaveAccessibleName("Show bar graph");
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
  await expect(page.locator("#llm-proxy-header .notice")).toHaveText("Request failed");
  await expect(intervalGroup.getByRole("button", { name: "1 day" })).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("0");
  await expect(page.locator("usage-chart-panel").first()).toContainText("No usage recorded");
  await expect(page.locator("usage-breakdown").first()).toContainText("No usage recorded");
  await expect(page.locator("usage-breakdown").nth(1)).toContainText("No usage recorded");
  await expect(modelBreakdownToggle).toHaveAccessibleName("Show bar graph");
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
    { status_code: 429, requests: 2 },
    { status_code: 499, requests: 1 },
    { status_code: 502, requests: 7 },
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
  await expect(dialog.locator("usage-failure-status")).toHaveCount(3);
  await expect(dialog.locator("usage-failure-status").nth(0)).toContainText("429");
  await expect(dialog.locator("usage-failure-status").nth(0)).toContainText("Rate limited");
  await expect(dialog.locator("usage-failure-status").nth(0)).toContainText("2");
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

test("rejected requests stay visible without entering execution or failure metrics", async ({ page }) => {
  const usage = managementUsage("30d", {
    requests: 2,
    successful_requests: 1,
    failed_requests: 1,
    text_requests: 2,
  });
  usage.rejected_requests = 3;
  usage.status_codes = [
    { status_code: 200, requests: 1 },
    { status_code: 502, requests: 1 },
  ];
  await installAssetRoutes(page);
  await installManagementRoutes(page);
  await installUsageResponse(page, httpOK, usage);
  await installUsageRejectionsResponse(page, managementUsageRejections("30d"));

  await page.goto(`${baseURL}${applicationPath}`);

  const requestsCard = page.locator("usage-card").filter({ hasText: "Requests" });
  await expect(requestsCard.locator("strong")).toHaveText("2");
  const rejectionAction = requestsCard.getByRole("button", { name: "3 rejected requests" });
  await rejectionAction.click();

  const dialog = page.getByRole("dialog", { name: "Rejected request details" });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole("heading", { name: "Status breakdown" })).toBeHidden();
  await expect(dialog.locator("usage-failure-row")).toHaveCount(3);
  await expect(dialog.locator("usage-failure-row").first()).toContainText("deepseek");
  await expect(dialog.locator("usage-failure-row").first()).toContainText("deepseek-v4-flash");
  await expect(dialog.locator("usage-failure-row").first()).toContainText("409 Conflict");
  await expect(dialog.locator("usage-failure-row").first()).toContainText("Provider not configured");
  await expect(dialog.locator("usage-failure-row").nth(1)).toContainText("Not resolved");

  await dialog.getByRole("button", { name: "Close rejected request details" }).click();
  await expect(dialog).toBeHidden();
  await expect(rejectionAction).toBeFocused();
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

  await page.locator("llm-proxy-management-application").evaluate((applicationElement) => {
    const alpineRuntime = /** @type {typeof globalThis & { Alpine?: { $data: (element: Element) => any } }} */ (globalThis);
    const applicationState = alpineRuntime.Alpine?.$data(applicationElement);
    if (!applicationState) {
      throw new Error("usage_failures_state_missing");
    }
    void applicationState.selectUsageInterval("7d");
  });
  await expect(page.getByRole("dialog", { name: "Failed request details" })).toBeHidden();
  await expect(page.getByRole("button", { name: /failed request/ })).toHaveCount(0);

  await page.locator("llm-proxy-management-application").evaluate((applicationElement) => {
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

test("provider cards follow catalog order and keep all-tenant activity provider-only", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await installAssetRoutes(page);
  await installManagementRoutes(page);
  await page.goto(baseURL + applicationPath);

  const cards = page.locator("[data-provider-card]");
  await expect(cards).toHaveCount(11);
  await expect(cards.locator(".provider-card-front h3")).toHaveText([
    "OpenAI API", "DeepSeek API", "DashScope API", "Moonshot API", "MiniMax API", "SiliconFlow API", "Z.AI API", "Gemini API", "Anthropic API", "Meta API", "xAI API",
  ]);
  const openAICard = page.locator('[data-provider-card="openai"]');
  await expect(openAICard.locator(".provider-card-taxonomy dt")).toHaveText(["Model families", "Capabilities"]);
  await expect(openAICard.locator(".provider-card-taxonomy dd").nth(0).locator("span")).toHaveText(["GPT-4", "GPT-5", "GPT Transcribe"]);
  await expect(openAICard.locator(".provider-card-taxonomy dd").nth(1).locator("span")).toHaveText(["Text", "Image analysis", "Dictation"]);
  await expect(openAICard.locator(".provider-card-front")).not.toContainText("API connection");
  await expect(openAICard.locator(".provider-card-front")).not.toContainText("Model publishers");
  await expect(openAICard.locator(".provider-card-front > header")).not.toContainText("Text");
  const geminiCard = page.locator('[data-provider-card="gemini"]');
  await expect(geminiCard.locator(".provider-card-taxonomy dd").nth(0).locator("span")).toHaveText(["Gemini"]);
  const metaCard = page.locator('[data-provider-card="meta"]');
  await expect(metaCard.locator(".provider-card-taxonomy dd").nth(0).locator("span")).toHaveText(["Muse Spark"]);
  const siliconFlowCard = page.locator('[data-provider-card="siliconflow"]');
  await expect(siliconFlowCard.locator(".provider-card-taxonomy dd").nth(0).locator("span")).toHaveText(["DeepSeek R1", "SenseVoice"]);
  const dashScopeCard = page.locator('[data-provider-card="dashscope"]');
  await expect(dashScopeCard.locator(".provider-card-taxonomy dd").nth(1).locator("span")).toHaveText(["Text", "Image analysis"]);
  await expect(dashScopeCard.locator(".provider-card-front")).not.toContainText("Image generation");
  expect(await cards.locator("provider-card-statuses span:visible").allTextContents()).not.toContain("active");
  await expect(cards.getByText("used", { exact: true })).toHaveCount(0);
  expect(await cards.locator(".provider-card-front dt:visible").allTextContents()).not.toContain("Provider default model");
  const providerSettingsAction = cards.first().getByRole("button", { name: "Set API key" });
  await expect(providerSettingsAction).toBeVisible();
  await expect(providerSettingsAction).toHaveAttribute("title", "Set API key");
  await expect(providerSettingsAction.locator("svg.provider-settings-icon")).toBeVisible();
  await expect(providerSettingsAction.locator("svg.provider-settings-icon")).toHaveAttribute("aria-hidden", "true");
  await expect(providerSettingsAction).toHaveText("");
  const cardFrontBounds = await openAICard.locator(".provider-card-front").boundingBox();
  const providerSettingsActionBounds = await providerSettingsAction.boundingBox();
  if (!cardFrontBounds || !providerSettingsActionBounds) throw new Error("provider_card_settings_bounds_missing");
  const actionRightInset = cardFrontBounds.x + cardFrontBounds.width - providerSettingsActionBounds.x - providerSettingsActionBounds.width;
  const actionTopInset = providerSettingsActionBounds.y - cardFrontBounds.y;
  expect(actionRightInset).toBeGreaterThanOrEqual(0);
  expect(actionRightInset).toBeLessThanOrEqual(14);
  expect(actionTopInset).toBeGreaterThanOrEqual(0);
  expect(actionTopInset).toBeLessThanOrEqual(14);
  const openAIRequestGraph = openAICard.getByRole("img", { name: "24 requests. 100% of the highest provider request count." });
  const deepSeekRequestGraph = page.locator('[data-provider-card="deepseek"]').getByRole("img", { name: "13 requests. 54% of the highest provider request count." });
  const metaRequestGraph = metaCard.getByRole("img", { name: "0 requests. 0% of the highest provider request count." });
  const openAIRequestTrackBounds = await openAIRequestGraph.locator("provider-request-track").boundingBox();
  const openAIRequestFillBounds = await openAIRequestGraph.locator("provider-request-track i").boundingBox();
  const deepSeekRequestTrackBounds = await deepSeekRequestGraph.locator("provider-request-track").boundingBox();
  const deepSeekRequestFillBounds = await deepSeekRequestGraph.locator("provider-request-track i").boundingBox();
  if (!openAIRequestTrackBounds || !openAIRequestFillBounds || !deepSeekRequestTrackBounds || !deepSeekRequestFillBounds) {
    throw new Error("provider_card_request_graph_bounds_missing");
  }
  expect(Math.abs(openAIRequestFillBounds.width - openAIRequestTrackBounds.width)).toBeLessThanOrEqual(2);
  expect(deepSeekRequestFillBounds.width / deepSeekRequestTrackBounds.width).toBeCloseTo(0.54, 2);
  expect(await deepSeekRequestGraph.locator("provider-request-track i").evaluate((element) => /** @type {HTMLElement} */ (element).style.width)).toBe("54%");
  expect(await metaRequestGraph.locator("provider-request-track i").evaluate((element) => /** @type {HTMLElement} */ (element).style.width)).toBe("0%");
  await expect(page.locator('[data-provider-card="dashscope"] .provider-card-usage dd:visible')).toHaveText(["0", "0"]);
  expect(await cards.first().locator(".provider-card-inner").evaluate((element) => getComputedStyle(element).transitionDuration)).toBe("0s");
  await page.setViewportSize({ width: 390, height: 844 });
  await expect.poll(async () => openAICard.locator(".provider-card-front").evaluate((element) => element.scrollHeight <= element.clientHeight)).toBe(true);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});

test("an explicit Usage tenant opens one safe compact provider card editor", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await installAssetRoutes(page);
  await installManagementRoutes(page);
  await page.goto(baseURL + applicationPath);
  await page.getByRole("combobox", { name: "Usage tenant" }).selectOption("tenant_1");

  const card = page.locator('[data-provider-card="openai"]');
  await expect(card.getByText("active", { exact: true })).toBeVisible();
  await expect(card.locator("provider-card-statuses span:visible")).toHaveText(["active"]);
  await expect(card.getByText("gpt-4.1", { exact: true })).toBeVisible();
  const providerSettingsAction = card.getByRole("button", { name: "API key settings" });
  await expect(providerSettingsAction).toHaveAttribute("title", "API key settings");
  const cardFrontBounds = await card.locator(".provider-card-front").boundingBox();
  if (!cardFrontBounds) throw new Error("provider_card_front_bounds_missing");
  await providerSettingsAction.click();

  const back = card.locator(".provider-card-back");
  await expect(back).toHaveAttribute("aria-hidden", "false");
  const cardBackBounds = await back.boundingBox();
  if (!cardBackBounds) throw new Error("provider_card_back_bounds_missing");
  expect(cardBackBounds.width).toBe(cardFrontBounds.width);
  expect(cardBackBounds.height).toBe(cardFrontBounds.height);
  const keyInput = back.getByRole("textbox", { name: "OpenAI API key" });
  await expect(keyInput).toHaveValue("••••••••");
  await expect(back.getByRole("button", { name: "Show key" })).toHaveCount(0);
  const acquisition = back.getByRole("link", { name: "Get API key" });
  await expect(acquisition).toHaveAttribute("href", "https://platform.openai.com/api-keys");
  await expect(acquisition).toHaveAttribute("target", "_blank");
  await expect(acquisition).toHaveAttribute("rel", "noopener noreferrer");
  const apiHeading = back.locator("provider-api-heading");
  await expect(apiHeading.getByText("OpenAI API", { exact: true })).toBeVisible();
  await expect(apiHeading.getByRole("link", { name: "Get API key" })).toBeVisible();
  const apiLabelBounds = await apiHeading.getByText("OpenAI API", { exact: true }).boundingBox();
  const acquisitionBounds = await acquisition.boundingBox();
  if (!apiLabelBounds || !acquisitionBounds) throw new Error("provider_api_heading_bounds_missing");
  expect(Math.abs(apiLabelBounds.y - acquisitionBounds.y)).toBeLessThanOrEqual(4);
  await expect(back.getByRole("combobox", { name: "Tenant" })).toBeFocused();
  await expect(back.getByRole("button", { name: "Done" })).toHaveCount(0);
  await expect(back.getByRole("button", { name: "Replace key" })).toHaveCount(0);
  const promptDisclosure = back.locator("details.provider-system-prompt");
  await expect(promptDisclosure).toHaveJSProperty("open", false);
  await expect(promptDisclosure.locator(".system-prompt-disclosure-state")).toHaveText("Hidden");
  const promptSummary = promptDisclosure.locator("summary");
  await promptSummary.focus();
  await promptSummary.press("Enter");
  await expect(promptDisclosure).toHaveJSProperty("open", true);
  await promptSummary.press("Enter");
  await expect(promptDisclosure).toHaveJSProperty("open", false);
  expect(await card.locator(".provider-card-front").evaluate((element) => element.inert)).toBe(true);
  await expect(back).toHaveAttribute("aria-hidden", "false");
  await expect(keyInput).toBeEditable();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect.poll(async () => back.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});

test("provider cards distinguish unavailable activity from a successful zero", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page, { usageStatus: httpInternalServerError });
  await page.goto(baseURL + applicationPath);

  const cards = page.locator("[data-provider-card]");
  await expect(cards).toHaveCount(11);
  await expect(cards.locator(".provider-card-usage dd:visible")).toHaveText(Array(22).fill("Unavailable"));
  await expect(cards.locator("provider-request-graph:visible")).toHaveCount(0);
  await expect(cards.locator(".provider-card-usage dd:visible").getByText("0", { exact: true })).toHaveCount(0);
});

test("provider card key verification is transient and preserves the prior key on safe failures", async ({ page }) => {
  const profile = managementProfile();
  await installAssetRoutes(page);
  await installManagementRoutes(page, { profile, savedProviderIDs: [] });
  const provider = profile.providers.find((candidate) => candidate.id === "openai");
  if (!provider) throw new Error("management_fixture_provider_missing:openai");
  /** @type {() => void} */
  let releaseVerification = () => {};
  const verificationGate = new Promise((resolve) => { releaseVerification = () => resolve(undefined); });
  const responses = [
    { status: 422, body: "provider_key_rejected", message: "Key was rejected. The previous key remains active." },
    { status: 429, body: "provider_key_verification_rate_limited", message: "Key could not be verified because the provider rate limit was reached. The previous key remains active." },
    { status: 504, body: "provider_key_verification_timed_out", message: "Key verification timed out. The previous key remains active." },
    { status: 503, body: "provider_key_verification_unavailable", message: "The provider could not verify this key. The previous key remains active." },
  ];
  const submittedKeys = [];
  let requestIndex = 0;
  await page.route(providerKeyEndpointURL("openai"), async (route) => {
    const payload = route.request().postDataJSON();
    submittedKeys.push(payload.fields.api_key);
    if (requestIndex === 0) {
      requestIndex += 1;
      await verificationGate;
      applyFixtureProviderConnection(provider, payload.fields, "saved");
      provider.text_model = payload.text_model;
      provider.system_prompt = payload.system_prompt;
      reconcileManagementProfileRoutingDefaults(profile);
      await route.fulfill({ headers: { "Cache-Control": "no-store" }, json: profile });
      return;
    }
    const response = responses[requestIndex - 1];
    requestIndex += 1;
    await route.fulfill({ status: response.status, body: response.body });
  });

  await page.goto(baseURL + applicationPath);
  await page.getByRole("combobox", { name: "Usage tenant" }).selectOption("tenant_1");
  const card = page.locator('[data-provider-card="openai"]');
  const action = card.getByRole("button", { name: "Set API key" });
  await action.click();
  const back = card.locator(".provider-card-back");
  const input = back.getByRole("textbox", { name: "OpenAI API key" });
  await pasteProviderKey(input, "first-candidate-key");
  await expect(back.getByText("Checking key...", { exact: true })).toBeVisible();
  await expect(back.getByRole("button", { name: "Replace key" })).toHaveCount(0);
  await expect(page.locator('[data-provider-card="deepseek"] button[data-provider-card-action]')).toBeDisabled();
  expect(await browserStorageContains(page, "first-candidate-key")).toBe(false);
  releaseVerification();
  await expect(input).toHaveValue("••••••••");
  await expect(back.getByText("Checking key...", { exact: true })).toBeHidden();
  await expect(card.locator("provider-card-statuses").getByText("Verified", { exact: true })).toHaveCount(0);

  for (const [index, response] of responses.entries()) {
    const candidate = `failed-candidate-${index}`;
    await pasteProviderKey(input, candidate);
    await expect(back.getByText(response.message, { exact: true })).toBeVisible();
    await expect(input).toHaveValue(candidate);
    expect(await card.locator(".provider-card-front").evaluate((element, raw) => element.outerHTML.includes(raw), candidate)).toBe(false);
    expect(await browserStorageContains(page, candidate)).toBe(false);
  }
  expect(submittedKeys).toEqual(["first-candidate-key", ...responses.map((_, index) => `failed-candidate-${index}`)]);
});

test("provider cards select the Default tenant and keep tenant settings independent from Usage", async ({ page }) => {
  await installAssetRoutes(page);
  const routeState = await installMultiTenantRoutes(page);
  /** @type {() => void} */
  let releaseTenantProfileLoad = () => {};
  const tenantProfileLoadGate = new Promise((resolve) => {
    releaseTenantProfileLoad = () => resolve(undefined);
  });
  await page.route(`${baseURL}/api/management/tenants/tenant_2`, async (route) => {
    if (route.request().method() === "GET") await tenantProfileLoadGate;
    await route.fallback();
  });
  let documentRequestCount = 0;
  page.on("request", (request) => {
    if (request.resourceType() === "document") documentRequestCount += 1;
  });
  await page.goto(baseURL + applicationPath);

  const card = page.locator('[data-provider-card="openai"]');
  await card.getByRole("button", { name: "Set API key" }).click();
  const cardBack = card.locator(".provider-card-back");
  const tenantSelector = card.getByRole("combobox", { name: "Tenant" });
  await expect(tenantSelector).toHaveValue("tenant_1");
  await expect(tenantSelector.locator("option:checked")).toHaveText("Default");
  await expect(cardBack.locator("header h3")).toHaveCount(0);
  await expect(cardBack.getByText("Default", { exact: true })).toHaveCount(1);
  await expect(card.getByRole("textbox", { name: "OpenAI API key" })).toHaveValue("••••••••");
  await expect(page.getByRole("combobox", { name: "Usage tenant" })).toHaveValue("");

  const promptDisclosure = card.locator("details.provider-system-prompt");
  await promptDisclosure.locator("summary").click();
  await expect(promptDisclosure).toHaveJSProperty("open", true);
  await card.getByRole("textbox", { name: "System prompt" }).fill("Default provider prompt.");
  await tenantSelector.selectOption("tenant_2");
  await expect.poll(() => routeState.profiles.get("tenant_1").providers.find((provider) => provider.id === "openai").system_prompt).toBe("Default provider prompt.");
  await expect(card.getByText("Loading provider settings...", { exact: true })).toBeVisible();
  await expect(page.locator("[data-provider-card]")).toHaveCount(11);
  await expect(page.locator('[data-provider-card="deepseek"]')).toBeVisible();
  await expect(page.getByRole("heading", { name: "Usage overview" })).toBeVisible();
  await expect(page.getByRole("combobox", { name: "Usage tenant" })).toHaveValue("");
  expect(documentRequestCount).toBe(1);
  releaseTenantProfileLoad();
  await expect(card.getByText("Loading provider settings...", { exact: true })).toBeHidden();
  await expect(tenantSelector).toHaveValue("tenant_2");
  await expect(cardBack.getByText("Research", { exact: true })).toHaveCount(1);
  await expect(card.getByRole("textbox", { name: "OpenAI API key" })).toHaveValue("••••••••");
  await expect(promptDisclosure).toHaveJSProperty("open", false);
  await expect(page.getByRole("combobox", { name: "Usage tenant" })).toHaveValue("");

  await promptDisclosure.locator("summary").click();
  await card.getByRole("textbox", { name: "System prompt" }).fill("Research provider prompt.");
  await card.getByRole("button", { name: "Close provider settings" }).click();
  await expect.poll(() => routeState.profiles.get("tenant_2").providers.find((provider) => provider.id === "openai").system_prompt).toBe("Research provider prompt.");
  await expect(card.locator(".provider-card-back")).toHaveAttribute("aria-hidden", "true");
  await expect(card.getByRole("button", { name: "Set API key" })).toBeFocused();
  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").getByText("Settings").click();
  await expect(page.getByRole("dialog", { name: "Settings" }).getByRole("combobox", { name: "Tenant" })).toHaveValue("tenant_1");
});

test("provider cards start with Default after Settings retains another tenant", async ({ page }) => {
  const defaultProfile = managementTenantProfile("tenant_1", "Default");
  const researchProfile = managementTenantProfile("tenant_2", "Research");
  defaultProfile.providers.find((provider) => provider.id === "openai").system_prompt = "Default card prompt.";
  researchProfile.providers.find((provider) => provider.id === "openai").system_prompt = "Research settings prompt.";
  await installAssetRoutes(page);
  await installMultiTenantRoutes(page, { profiles: [defaultProfile, researchProfile] });
  await page.goto(baseURL + applicationPath);

  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").getByText("Settings").click();
  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  const settingsTenantSelector = settingsDialog.getByRole("combobox", { name: "Tenant" });
  await settingsTenantSelector.selectOption("tenant_2");
  await expect(settingsTenantSelector).toHaveValue("tenant_2");
  await settingsDialog.getByRole("button", { name: "Close" }).click();

  const card = page.locator('[data-provider-card="openai"]');
  await card.getByRole("button", { name: "Set API key" }).click();
  const cardTenantSelector = card.getByRole("combobox", { name: "Tenant" });
  await expect(cardTenantSelector).toHaveValue("tenant_1");
  await expect(cardTenantSelector.locator("option:checked")).toHaveText("Default");
  await card.locator("details.provider-system-prompt summary").click();
  await expect(card.getByRole("textbox", { name: "System prompt" })).toHaveValue("Default card prompt.");
  await card.getByRole("button", { name: "Close provider settings" }).click();

  await page.getByTestId("avatar-menu").click();
  await page.getByTestId("avatar-menu-item").getByText("Settings").click();
  await expect(settingsDialog.getByRole("combobox", { name: "Tenant" })).toHaveValue("tenant_2");
});

test("deleting a provider key preserves its model, prompt, setting, and usage card", async ({ page }) => {
  const profile = managementProfile();
  const dashscope = profile.providers.find((provider) => provider.id === "dashscope");
  applyFixtureProviderConnection(dashscope, { api_key: "sk-dashscope", base_url: "https://dashscope.example/v1" }, "saved");
  dashscope.text_model = "qwen3.7-max";
  dashscope.system_prompt = "Preserve this prompt.";
  await installAssetRoutes(page);
  await installManagementRoutes(page, { profile, providerKeys: { dashscope: "sk-dashscope" } });
  await page.route(`${baseURL}${managementDefaultTenantPath}/usage?interval=*`, async (route) => {
    const interval = new URL(route.request().url()).searchParams.get("interval") || "";
    const usage = managementUsage(interval);
    usage.providers.push({ provider: "dashscope", data: usageAggregate({ requests: 5, total_tokens: 321 }) });
    await route.fulfill({ headers: { "Cache-Control": "no-store" }, json: usage });
  });
  await page.goto(baseURL + applicationPath);
  await page.getByRole("combobox", { name: "Usage tenant" }).selectOption("tenant_1");

  const card = page.locator('[data-provider-card="dashscope"]');
  await expect(card.locator(".provider-card-usage dd:visible")).toHaveText(["5", "321", "qwen3.7-max"]);
  await card.getByRole("button", { name: "API key settings" }).click();
  const keyInputRow = card.getByRole("textbox", { name: "DashScope API key" }).locator("..");
  const deleteKey = keyInputRow.getByRole("button", { name: "Delete key" });
  await expect(deleteKey).toHaveAttribute("title", "Delete key");
  await expect(deleteKey.locator(".material-symbols-outlined")).toHaveText("delete");
  await deleteKey.click();
  await page.getByRole("alertdialog", { name: "Remove provider key?" }).getByRole("button", { name: "Remove key" }).click();
  await expect(card.getByRole("textbox", { name: "DashScope API key" })).toHaveValue("");
  await expect(card.getByRole("textbox", { name: "DashScope API URL" })).toHaveValue("https://dashscope.example/v1");
  await expect(card.getByRole("combobox", { name: "Provider default model" })).toHaveValue("qwen3.7-max");
  await card.locator("details.provider-system-prompt summary").click();
  await expect(card.getByRole("textbox", { name: "System prompt" })).toHaveValue("Preserve this prompt.");
  await expect(card).toBeVisible();
  await expect(card.locator(".provider-card-usage dd:visible")).toHaveText(["5", "321", "qwen3.7-max"]);
});

test("routing defaults autosave complete provider and model pairs without a manual action", async ({ page }) => {
  const defaultsMutations = [];
  page.on("request", (request) => {
    if (request.url() === `${baseURL}${managementDefaultTenantPath}/defaults`) {
      defaultsMutations.push(request.postDataJSON());
    }
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page, { savedProviderIDs: ["openai", "deepseek", "meta", "xai"] });

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

  await dictationProvider.selectOption("xai");
  await expect(dictationModel).toHaveValue("xai-stt");
  await expect.poll(() => defaultsMutations.length).toBe(2);
  expect(defaultsMutations.at(-1)).toEqual({
    provider: "deepseek",
    model: "deepseek-chat",
    dictation_provider: "xai",
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
    dictation_provider: "xai",
    dictation_model: "xai-stt",
    system_prompt: "Use tenant-wide autosaved guidance.",
    reasoning_effort: "",
  });
  await expect(settingsDialog.locator(".settings-notification")).toHaveText("Defaults saved");
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
  await expect(settingsDialog.getByRole("combobox", { name: "Dictation provider" })).toHaveValue("xai");
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

  await expect(settingsDialog.locator("provider-editor")).toHaveCount(0);
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
  const settingsNotification = settingsDialog.locator(".settings-notification");
  await expect(settingsNotification).toHaveText("Request failed");
  await expect(settingsNotification.locator(".notice")).toHaveAttribute("data-kind", "error");
  await expect(page.locator("#llm-proxy-header notification-region")).toBeHidden();
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
    sessionStorage.setItem("llm-proxy-test-auth-status", "unauthenticated");
    document.dispatchEvent(new CustomEvent("mpr-ui:auth:unauthenticated"));
  });
  await expect(page).toHaveURL(`${baseURL}/`);
  releaseDefaultsSave();
  await page.waitForTimeout(50);
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
  await installManagementRoutes(page, { savedProviderIDs: ["openai", "deepseek", "meta", "moonshot"] });

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

  await textProvider.selectOption("moonshot");
  await expect(textModel).toHaveValue("kimi-k2.6");
  await expect(reasoningEffort).toBeHidden();
  await textModel.selectOption("kimi-k3");
  await expect(reasoningEffort).toBeVisible();
  await expect(reasoningEffort.locator("option")).toHaveText(["Not set", "low", "high", "max"]);
  await reasoningEffort.selectOption("max");
  await expect.poll(() => defaultsMutations.some((defaults) => (
    defaults.provider === "moonshot" && defaults.model === "kimi-k3" && defaults.reasoning_effort === "max"
  ))).toBe(true);
  await textProvider.selectOption("openai");
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

test("malformed routing-default profiles become app data integrity errors", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page, { malformedRoutingDefaults: true });

  await page.goto(`${baseURL}${applicationPath}`);

  await expect(page.getByRole("heading", { name: "Unable to load LLM Proxy" })).toBeVisible();
  await expect(page.getByText("App data integrity error")).toBeVisible();
  await expect(page.getByRole("dialog", { name: "Settings" })).toBeHidden();
});

test("malformed provider fields become app data integrity errors", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page, { malformedProviderField: true });

  await page.goto(`${baseURL}${applicationPath}`);

  await expect(page.getByRole("heading", { name: "Unable to load LLM Proxy" })).toBeVisible();
  await expect(page.getByText("App data integrity error")).toBeVisible();
  await expect(page.getByRole("dialog", { name: "Settings" })).toBeHidden();
});

test("invalid persisted routing-default profiles become app data integrity errors", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page, { profileStatus: 500, profileError: "managed_routing_defaults_invalid" });

  await page.goto(`${baseURL}${applicationPath}`);

  await expect(page.getByRole("heading", { name: "Unable to load LLM Proxy" })).toBeVisible();
  await expect(page.getByText("App data integrity error")).toBeVisible();
});

test("public Log In authenticates through MPR UI before opening the app", async ({ page }) => {
  const profileRequests = [];
  page.on("request", (request) => {
    if (request.url() === `${baseURL}${managementDefaultTenantPath}`) {
      profileRequests.push(request);
    }
  });
  await installAssetRoutes(page, { initialAuthStatus: "unauthenticated" });
  await installManagementRoutes(page);

  await page.goto(baseURL);

  const logIn = page.getByRole("button", { name: "Log In" });
  await expect(logIn).toBeVisible();
  expect(profileRequests).toHaveLength(0);
  const landingHistoryLength = await page.evaluate(() => history.length);
  await logIn.click();
  await page.evaluate(() => window.__llmProxyMprAuthenticate());

  await expect(page).toHaveURL(`${baseURL}${applicationPath}`);
  expect(await page.evaluate(() => history.length)).toBe(landingHistoryLength);
  await expect(page.getByRole("heading", { name: "Usage overview" })).toBeVisible();
  await expect(page.locator("usage-metrics usage-card").first().locator("strong")).toHaveText("37");
  expect(profileRequests).toHaveLength(1);
});

test("a restored authenticated session replaces the anonymous landing with the app", async ({ page }) => {
  await installAssetRoutes(page);
  await installManagementRoutes(page);

  await page.goto(baseURL);

  await expect(page).toHaveURL(`${baseURL}${applicationPath}`);
  await expect(page.getByRole("heading", { name: "Usage overview" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Integrate once. Use the model that fits." })).toHaveCount(0);
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
    if (request.url().includes("/alpinejs@3.17.1/dist/module.esm.js")) {
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

  await expect(page.getByRole("heading", { name: "Unable to load LLM Proxy" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Loading LLM Proxy" })).toBeHidden();
  await expect(page.getByRole("heading", { name: "Sign in to manage LLM Proxy keys" })).toBeHidden();

  await page.reload();
  await expect(page.getByRole("heading", { name: "Unable to load LLM Proxy" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Loading LLM Proxy" })).toBeHidden();
});

test("a direct anonymous app visit returns to the public page", async ({ page }) => {
  const accountRequests = [];
  page.on("request", (request) => {
    if (request.url() === `${baseURL}/api/management/account`) {
      accountRequests.push(request);
    }
  });
  await installAssetRoutes(page, { initialAuthStatus: "unauthenticated" });
  await installManagementRoutes(page);

  await page.goto(`${baseURL}${applicationPath}`);

  await expect(page).toHaveURL(`${baseURL}/`);
  await expect(page.getByRole("heading", { name: "Integrate once. Use the model that fits." })).toBeVisible();
  await expect(page.locator("llm-proxy-management-application")).toHaveCount(0);
  expect(accountRequests).toHaveLength(0);
});

test("fresh authenticated users can leave Settings after receiving one client key", async ({ page }) => {
  const generatedSecret = "llmp_test_fresh_user_secret";
  let secretRequestCount = 0;
  page.on("request", (request) => {
    if (request.url() === baseURL + managementDefaultTenantPath + "/secrets" && request.method() === "POST") secretRequestCount += 1;
  });
  await installAssetRoutes(page);
  await installManagementRoutes(page, { hasSecret: false, generatedSecret, savedProviderIDs: [] });
  await page.goto(baseURL + applicationPath);

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  await expect(settingsDialog).toBeVisible();
  await expect.poll(() => secretRequestCount).toBe(1);
  await expect(settingsDialog.getByRole("alert")).toBeHidden();
  await expect(settingsDialog.locator("provider-editor")).toHaveCount(0);
  await settingsDialog.getByRole("button", { name: "Close" }).click();
  await expect(settingsDialog).toBeHidden();
  await expect(page.locator('[data-provider-card="openai"]')).toBeVisible();
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
  await expect(settingsDialog.locator(".settings-notification .notice")).toHaveText("Request failed");
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
    sessionStorage.setItem("llm-proxy-test-auth-status", "unauthenticated");
    document.dispatchEvent(new CustomEvent("mpr-ui:auth:unauthenticated"));
  });
  await expect(page).toHaveURL(`${baseURL}/`);
  fulfillSecretResponse();
  await page.waitForTimeout(50);
  await expect(page.locator("body")).not.toContainText(lateGeneratedSecret);
  expect(await browserStorageContains(page, lateGeneratedSecret)).toBe(false);

  await page.getByRole("button", { name: "Log In" }).click();
  await page.evaluate(() => window.__llmProxyMprAuthenticate());
  await expect(page).toHaveURL(`${baseURL}${applicationPath}`);
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
    await expect(settingsDialog.locator(".settings-notification .notice")).toHaveText("Key copied");

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
    await expect(settingsDialog.locator(".settings-notification .notice")).toHaveText("Key replaced");
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

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  await expect(settingsDialog).toBeVisible();
  const modalBox = await page.locator("settings-modal").boundingBox();
  expect(modalBox).not.toBeNull();
  expect(modalBox.width).toBeLessThanOrEqual(390);
  await expect(settingsDialog.getByRole("button", { name: "Close" })).toBeVisible();

  for (const helpTooltip of [{
    button: settingsDialog.getByRole("button", { name: "About routing defaults" }),
    content: settingsDialog.locator("#routing-defaults-help"),
  }]) {
    await helpTooltip.button.scrollIntoViewIfNeeded();
    await helpTooltip.button.focus();
    await expect(helpTooltip.content).toBeVisible();
    const tooltipBox = await helpTooltip.content.boundingBox();
    if (!tooltipBox) {
      throw new Error("settings_help_tooltip_missing");
    }
    expect(tooltipBox.x).toBeGreaterThanOrEqual(0);
    expect(tooltipBox.x + tooltipBox.width).toBeLessThanOrEqual(390);
  }
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
    const sharedFooter = page.locator("mpr-footer");
    await sharedFooter.scrollIntoViewIfNeeded();
    await expect(sharedFooter).toBeInViewport();
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
    await expect(notice).toHaveText("App ready");
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
    const settingsNotification = settingsDialog.locator(".settings-notification");
    await expect(settingsNotification).toHaveAttribute("role", "status");
    await expect(settingsNotification).toHaveAttribute("aria-live", "polite");
    await expect(settingsNotification).toHaveAttribute("aria-atomic", "true");
    await expect(settingsNotification).toBeHidden();
    await expectSettingsHeaderCloseGeometry(settingsDialog);
    const layerFacts = await settingsLayerFacts(page);
    expect(layerFacts.noticeHit.inSettingsModal || layerFacts.noticeHit.inSettingsOverlay).toBe(true);
    expect(layerFacts.noticeHit.inNotice).toBe(false);

    await settingsDialog.getByRole("combobox", { name: "Text provider" }).selectOption("deepseek");
    await expect(settingsNotification.locator(".notice")).toHaveText("Defaults saved");
    await expect(settingsNotification.locator(".notice")).toHaveAttribute("data-kind", "success");
    await expect(notificationRegion).toBeHidden();
    await expectSettingsHeaderCloseGeometry(settingsDialog);
    const settingsHeaderBox = await settingsDialog.locator(".settings-header").boundingBox();
    const settingsNotificationBox = await settingsNotification.boundingBox();
    if (!settingsHeaderBox || !settingsNotificationBox) {
      throw new Error(`settings_notification_geometry_missing:${viewport.name}`);
    }
    expect(settingsNotificationBox.x).toBeGreaterThanOrEqual(settingsHeaderBox.x);
    expect(settingsNotificationBox.x + settingsNotificationBox.width).toBeLessThanOrEqual(
      settingsHeaderBox.x + settingsHeaderBox.width,
    );

    await settingsDialog.getByRole("button", { name: "Close" }).click();
    await expect(settingsDialog).toBeHidden();
    await installUsageResponse(page, httpOK);
    await page.getByRole("button", { name: "Refresh" }).click();
    await expect(notice).toHaveText("Usage refreshed");
    await expectHeaderNoticeGeometry(page);
  }
});

test("public Log In stays keyboard accessible at every supported width", async ({ page }) => {
  await installAssetRoutes(page, { initialAuthStatus: "unauthenticated" });

  for (const viewport of settingsLayerViewports) {
    await page.setViewportSize({ width: viewport.width, height: viewport.height });
    await page.goto(baseURL);

    const logIn = page.getByRole("button", { name: "Log In" });
    await expect(logIn).toBeVisible();
    await logIn.focus();
    await expect(logIn).toBeFocused();
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
  await expect(notice).toHaveText("App ready");
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

test("the shared footer project catalog opens as a keyboard-dismissible drop-up", async ({ page }) => {
  await installAssetRoutes(page, { initialAuthStatus: "unauthenticated" });
  await page.setViewportSize({ width: 390, height: 780 });
  await page.goto(baseURL);

  const footer = page.locator("mpr-footer");
  const projectCatalog = footer.getByRole("button", { name: "Built by Marco Polo Research Lab" });
  await footer.scrollIntoViewIfNeeded();
  await projectCatalog.focus();
  await expect(projectCatalog).toBeFocused();
  await projectCatalog.click();
  await expect(projectCatalog).toHaveAttribute("aria-expanded", "true");
  await expect(footer.locator("ul a")).toHaveCount(10);
  await expect(footer.getByRole("link", { name: "Marco Polo Research Lab" })).toHaveAttribute(
    "href",
    "https://mprlab.com",
  );
  await page.keyboard.press("Escape");
  await expect(projectCatalog).toHaveAttribute("aria-expanded", "false");
  await expectCompactFooterGeometry(footer);
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
  await expect(page.locator("#llm-proxy-header .notice")).toHaveText("Request failed");

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

  await expect(page.locator("#llm-proxy-header .notice")).toHaveText("Request failed");
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
  await expect(page.locator(".usage-breakdown-toggle")).toHaveCount(0);
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
  await page.route("**/alpinejs@3.17.1/dist/module.esm.js", async (route) => {
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
  await page.route("**/js-yaml@5.4.1/dist/browser/js-yaml.umd.min.js", async (route) =>
    fulfillFile(route, "node_modules/js-yaml/dist/browser/js-yaml.umd.min.js", "application/javascript"),
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
 * @param {import("@playwright/test").Locator} footer
 * @returns {Promise<void>}
 */
async function expectStickyFooterGeometry(page, footer) {
  await page.evaluate(async () => {
    const rootElement = document.documentElement;
    const previousScrollBehavior = rootElement.style.scrollBehavior;
    rootElement.style.scrollBehavior = "auto";
    window.scrollTo(0, rootElement.scrollHeight);
    await new Promise((resolve) => requestAnimationFrame(resolve));
    rootElement.style.scrollBehavior = previousScrollBehavior;
  });
  const geometry = await footer.getByRole("contentinfo").evaluate((footerSurfaceElement) => {
    const footerBounds = footerSurfaceElement.getBoundingClientRect();
    const mainElement = document.querySelector("body > main");
    if (!mainElement) {
      throw new Error("public_site_main_missing");
    }
    const mainBounds = mainElement.getBoundingClientRect();
    return {
      anchoredBottom: Math.abs(footerBounds.bottom - window.innerHeight) <= 0.5,
      anchoredLeft: Math.abs(footerBounds.left) <= 0.5,
      anchoredRight: Math.abs(footerBounds.right - document.documentElement.clientWidth) <= 0.5,
      mainFooterOverlap: mainBounds.bottom - footerBounds.top,
      position: getComputedStyle(footerSurfaceElement).position,
    };
  });
  expect(geometry.position).toBe("fixed");
  expect(geometry.anchoredBottom).toBe(true);
  expect(geometry.anchoredLeft).toBe(true);
  expect(geometry.anchoredRight).toBe(true);
  expect(geometry.mainFooterOverlap).toBeLessThanOrEqual(0.5);
  await expectCompactFooterGeometry(footer);
}

/**
 * @param {import("@playwright/test").Locator} footer
 * @returns {Promise<void>}
 */
async function expectCompactFooterGeometry(footer) {
  const geometry = await footer.getByRole("contentinfo").evaluate((footerSurfaceElement) => ({
    clientWidth: footerSurfaceElement.clientWidth,
    height: footerSurfaceElement.getBoundingClientRect().height,
    scrollWidth: footerSurfaceElement.scrollWidth,
  }));
  expect(geometry.height).toBeLessThanOrEqual(PUBLIC_FOOTER_COMPACT_MAX_HEIGHT);
  expect(geometry.scrollWidth).toBeLessThanOrEqual(geometry.clientWidth);
}

/**
 * @param {string} html
 * @returns {{ header: string, footer: string, headerOffset: number, mainOffset: number, footerOffset: number }}
 */
function publicShellMarkup(html) {
  const header = html.match(/    <mpr-header[\s\S]*?    <\/mpr-header>/)?.[0] || "";
  const footer = html.match(/    <mpr-footer[\s\S]*?    <\/mpr-footer>/)?.[0] || "";
  const headerOffset = html.indexOf(header);
  const mainOffset = html.indexOf("<main");
  const footerOffset = html.indexOf(footer);
  if (!header || !footer || headerOffset === -1 || mainOffset === -1 || footerOffset === -1) {
    throw new Error("public_site_shell_missing");
  }
  return { header, footer, headerOffset, mainOffset, footerOffset };
}

/**
 * @param {import("@playwright/test").Page} page
 * @returns {Promise<void>}
 */
async function expectCenteredValueStrip(page) {
  const facts = await page.locator("#routing-overview").evaluate((overviewElement) => {
    const gridElement = overviewElement.querySelector(".value-strip__grid");
    if (!gridElement) {
      throw new Error("landing_value_strip_grid_missing");
    }
    const stripStyle = getComputedStyle(overviewElement);
    const gridStyle = getComputedStyle(gridElement);
    const gridRect = gridElement.getBoundingClientRect();
    const followingSection = overviewElement.nextElementSibling;
    if (!followingSection) {
      throw new Error("landing_routing_overview_following_section_missing");
    }
    const followingSectionStyle = getComputedStyle(followingSection);
    const pageStyle = getComputedStyle(document.body);
    return {
      followsSectionContract: overviewElement.classList.contains("section"),
      gridBackground: gridStyle.backgroundColor,
      gridBorderTopWidth: gridStyle.borderTopWidth,
      itemCount: gridElement.querySelectorAll(":scope > p").length,
      leftGap: gridRect.left,
      pageBackground: pageStyle.backgroundColor,
      paddingBottom: parseFloat(stripStyle.paddingBottom),
      paddingTop: parseFloat(stripStyle.paddingTop),
      rightGap: document.documentElement.clientWidth - gridRect.right,
      stripBackground: stripStyle.backgroundColor,
      stripBorderBottomWidth: stripStyle.borderBottomWidth,
      stripBorderTopWidth: stripStyle.borderTopWidth,
      followingSectionBackground: followingSectionStyle.backgroundColor,
    };
  });
  expect(facts.itemCount).toBe(3);
  expect(facts.followsSectionContract).toBe(true);
  expect(facts.stripBorderTopWidth).toBe("1px");
  expect(facts.stripBorderBottomWidth).toBe("1px");
  expect(facts.gridBorderTopWidth).toBe("1px");
  expect(facts.stripBackground).not.toBe(facts.gridBackground);
  expect(facts.stripBackground).not.toBe(facts.pageBackground);
  expect(facts.stripBackground).not.toBe(facts.followingSectionBackground);
  expect(facts.paddingTop).toBeGreaterThanOrEqual(48);
  expect(facts.paddingBottom).toBeGreaterThanOrEqual(48);
  expect(facts.leftGap).toBeGreaterThan(0);
  expect(facts.rightGap).toBeGreaterThan(0);
}

/**
 * @param {import("@playwright/test").Page} page
 * @returns {Promise<void>}
 */
async function expectAlignedCatalogRows(page) {
  const rowsAreAligned = await page.locator("[data-catalog-row]").evaluateAll((rowElements, expectedColumnCount) =>
    rowElements.every((rowElement) => {
      const rowBounds = rowElement.getBoundingClientRect();
      const cellElements = Array.from(rowElement.querySelectorAll(":scope > td"));
      return cellElements.length === expectedColumnCount && cellElements.every((cellElement) => {
        const cellBounds = cellElement.getBoundingClientRect();
        return getComputedStyle(cellElement).display === "table-cell" &&
          cellBounds.top === rowBounds.top &&
          cellBounds.bottom === rowBounds.bottom;
      });
    }), catalogColumnCount);
  expect(rowsAreAligned).toBe(true);
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
    const footerSurfaceElement = footerElement?.querySelector("footer");
    const notificationRegion = document.querySelector("notification-region");
    const noticeElement = document.querySelector(".notice");
    if (!overlayElement || !modalElement || !closeButton || !headerElement || !footerElement || !footerSurfaceElement || !notificationRegion || !noticeElement) {
      throw new Error("settings_layer_elements_missing");
    }

    const modalRect = modalElement.getBoundingClientRect();
    const closeButtonRect = closeButton.getBoundingClientRect();
    const headerRect = headerElement.getBoundingClientRect();
    const footerRect = footerSurfaceElement.getBoundingClientRect();
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
      footerZIndex: Number.parseInt(getComputedStyle(footerSurfaceElement).zIndex, 10),
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
 * @param {import("@playwright/test").Locator} settingsDialog
 */
async function expectSettingsHeaderCloseGeometry(settingsDialog) {
  const geometry = await settingsDialog.locator(".settings-header").evaluate((headerElement, centerDivisor) => {
    const titleElement = headerElement.querySelector("#settings-title");
    const closeElement = headerElement.querySelector(".settings-close");
    if (!(titleElement instanceof HTMLElement) || !(closeElement instanceof HTMLElement)) {
      throw new Error("settings_header_geometry_elements_missing");
    }
    const headerBounds = headerElement.getBoundingClientRect();
    const titleBounds = titleElement.getBoundingClientRect();
    const closeBounds = closeElement.getBoundingClientRect();
    const headerStyle = getComputedStyle(headerElement);
    return {
      rightEdgeDifference: Math.abs(
        headerBounds.right - Number.parseFloat(headerStyle.paddingRight) - closeBounds.right,
      ),
      verticalCenterDifference: Math.abs(
        (titleBounds.top + titleBounds.bottom) / centerDivisor
          - (closeBounds.top + closeBounds.bottom) / centerDivisor,
      ),
    };
  }, geometryCenterDivisor);
  expect(geometry.rightEdgeDifference).toBeLessThanOrEqual(settingsHeaderAlignmentTolerance);
  expect(geometry.verticalCenterDifference).toBeLessThanOrEqual(settingsHeaderAlignmentTolerance);
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
 * @returns {string}
 */
function usageRejectionsRequestPattern() {
  return `${baseURL}/api/management/usage/rejections?*`;
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
 * @param {object} response
 * @returns {Promise<void>}
 */
async function installUsageRejectionsResponse(page, response) {
  await page.route(usageRejectionsRequestPattern(), async (route) => {
    const requestURL = new URL(route.request().url());
    expect(requestURL.searchParams.get("interval")).toBe(response.interval);
    expect(requestURL.searchParams.get("limit")).toBe("25");
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
    const [tenantID, resource, providerID, fieldCollection, fieldID, action] = relativePath.split("/");
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
    if (resource === "provider-connections") {
      const provider = profile.providers.find((candidateProvider) => candidateProvider.id === providerID);
      if (!provider) {
        await route.fulfill({ status: 404 });
        return;
      }
      const providerKeys = state.providerKeys.get(tenantID);
      if (fieldCollection === "fields" && action === "reveal" && request.method() === "POST") {
        const field = provider.fields.find((candidateField) => candidateField.id === fieldID);
        if (!field?.secret || !field.configured || !providerKeys[providerID]) {
          await route.fulfill({ status: 404 });
          return;
        }
        await route.fulfill({
          headers: { "Cache-Control": "no-store" },
          json: { field_id: fieldID, value: providerKeys[providerID] },
        });
        return;
      }
      if (!fieldCollection && request.method() === "PUT") {
        const settings = request.postDataJSON();
        if (settings.fields.api_key) {
          providerKeys[providerID] = settings.fields.api_key;
        }
        applyFixtureProviderConnection(provider, settings.fields, "saved");
        provider.text_model = settings.text_model;
        provider.system_prompt = settings.system_prompt;
        reconcileManagementProfileRoutingDefaults(profile);
        await route.fulfill({ headers: { "Cache-Control": "no-store" }, json: profile });
        return;
      }
      if (!fieldCollection && request.method() === "DELETE") {
        delete providerKeys[providerID];
        clearFixtureProviderConnection(provider);
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
      clearFixtureProviderConnection(provider);
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
 * @param {{ usageStatus?: number, admin?: boolean, hasSecret?: boolean, generatedSecret?: string, profile?: object, profileStatus?: number, profileStatuses?: number[], profileError?: string, malformedProviderField?: boolean, malformedRoutingDefaults?: boolean, maskedKeys?: Record<string, string>, providerKeys?: Record<string, string>, savedProviderIDs?: string[] }} options
 * @returns {Promise<void>}
 */
async function installManagementRoutes(page, options = {}) {
  const profileStatuses = [...(options.profileStatuses || [])];
  const profile = options.profile || managementProfile(options.admin || false, options.hasSecret !== false);
  const providerKeys = {
    openai: "sk-owner-openai",
    deepseek: "sk-owner-deepseek",
    meta: "sk-owner-meta",
    ...options.providerKeys,
  };
  if (options.savedProviderIDs) {
    for (const provider of profile.providers) {
      if (options.savedProviderIDs.includes(provider.id)) {
        const fieldValues = Object.fromEntries(provider.fields.map((field) => [
          field.id,
          field.secret ? `sk-owner-${provider.id}` : fixtureProviderSettingValue(provider.id, field),
        ]));
        applyFixtureProviderConnection(provider, fieldValues, "sk-...saved");
      } else {
        clearFixtureProviderConnection(provider);
      }
    }
    reconcileManagementProfileRoutingDefaults(profile);
  }
  for (const [providerID, maskedKey] of Object.entries(options.maskedKeys || {})) {
    const provider = profile.providers.find((candidateProvider) => candidateProvider.id === providerID);
    if (!provider) {
      throw new Error(`management_fixture_provider_missing:${providerID}`);
    }
    const secretField = provider.fields.find((field) => field.secret);
    if (!secretField) {
      throw new Error(`management_fixture_provider_secret_field_missing:${providerID}`);
    }
    secretField.masked_value = maskedKey;
  }
  if (options.malformedRoutingDefaults) {
    profile.providers.push({
      id: "anthropic",
      label: "Anthropic",
      aliases: [],
      configured: false,
      fields: [providerAPIKeyField("Anthropic API key")],
      text_model: "claude-sonnet-5",
      system_prompt: "",
      text_default_model: "claude-sonnet-5",
      text_models: [{ id: "claude-sonnet-5" }],
      supports_dictation: false,
      dictation_models: [],
    });
    profile.tenant.defaults.provider = "anthropic";
  }
  if (options.malformedProviderField) {
    const dashScopeProvider = profile.providers.find((provider) => provider.id === "dashscope");
    if (!dashScopeProvider) {
      throw new Error("management_fixture_provider_missing:dashscope");
    }
    const baseURLField = dashScopeProvider.fields.find((field) => field.id === "base_url");
    if (!baseURLField) {
      throw new Error("management_fixture_provider_field_missing:dashscope:base_url");
    }
    Reflect.deleteProperty(baseURLField, "value");
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
  await page.route(`${baseURL}${managementDefaultTenantPath}/usage?interval=*`, async (route) => {
    const interval = new URL(route.request().url()).searchParams.get("interval") || "";
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
    const [providerID, fieldCollection, fieldID, action] = providerPath.split("/");
    const provider = profile.providers.find((candidateProvider) => candidateProvider.id === providerID);
    if (!provider) {
      await route.fulfill({ status: 404 });
      return;
    }
    if (fieldCollection === "fields" && action === "reveal") {
      const field = provider.fields.find((candidateField) => candidateField.id === fieldID);
      if (!field?.secret || !field.configured) {
        await route.fulfill({ status: 404 });
        return;
      }
      await route.fulfill({
        headers: { "Cache-Control": "no-store" },
        json: { field_id: fieldID, value: providerKeys[providerID] },
      });
      return;
    }
    if (!fieldCollection && request.method() === "PUT") {
      const providerSettings = request.postDataJSON();
      const previousTextModel = provider.text_model;
      if (providerSettings.fields.api_key) {
        providerKeys[providerID] = providerSettings.fields.api_key;
      }
      applyFixtureProviderConnection(provider, providerSettings.fields, "sk-...saved");
      provider.text_model = providerSettings.text_model;
      provider.system_prompt = providerSettings.system_prompt;
      reconcileManagementProfileRoutingDefaults(profile);
      if (previousTextModel !== provider.text_model) {
        reconcileManagementProfileAfterProviderTextModelChange(profile, provider);
      }
      await route.fulfill({ headers: { "Cache-Control": "no-store" }, json: profile });
      return;
    }
    if (!fieldCollection && request.method() === "DELETE") {
      delete providerKeys[providerID];
      clearFixtureProviderConnection(provider);
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
    .filter((provider) => provider.configured)
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
 * @param {ReturnType<typeof managementProfile>} profile
 * @param {ReturnType<typeof managementProfile>["providers"][number]} provider
 * @returns {void}
 */
function reconcileManagementProfileAfterProviderTextModelChange(profile, provider) {
  if (profile.tenant.defaults.provider !== provider.id) {
    return;
  }
  profile.tenant.defaults.model = provider.text_model;
  const selectedModel = provider.text_models.find((model) => model.id === provider.text_model);
  const supportedEfforts = selectedModel && selectedModel.reasoning_effort
    ? selectedModel.reasoning_effort.efforts
    : [];
  if (profile.tenant.defaults.reasoning_effort && !supportedEfforts.includes(profile.tenant.defaults.reasoning_effort)) {
    profile.tenant.defaults.reasoning_effort = "";
  }
}

/**
 * @param {string} providerID
 * @param {string} [action]
 * @returns {string}
 */
function providerKeyEndpointURL(providerID, action = "") {
	return `${baseURL}${managementProviderKeysPath}/${providerID}${action ? `/fields/api_key/${action}` : ""}`;
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
 * @param {string} label
 * @param {boolean} [configured]
 * @param {string} [maskedValue]
 * @returns {object}
 */
function providerAPIKeyField(label, configured = false, maskedValue = "") {
  return {
    id: "api_key",
    label,
    kind: "credential",
    type: "opaque",
    required: true,
    default: "",
    secret: true,
    validation: { minimum_length: 1 },
    configured,
    ...(configured ? { masked_value: maskedValue } : {}),
  };
}

/** @returns {object} */
function dashScopeBaseURLField() {
  return {
    id: "base_url",
    label: "DashScope API URL",
    kind: "setting",
    type: "url",
    required: true,
    default: "",
    secret: false,
    validation: {
      pattern: "^https://[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\\.ap-southeast-1\\.maas\\.aliyuncs\\.com/compatible-mode/v1$",
      allowed_schemes: ["https"],
    },
    configured: false,
    value: "",
  };
}

/**
 * @param {string} providerID
 * @param {object} field
 * @returns {string}
 */
function fixtureProviderSettingValue(providerID, field) {
  if (providerID === "dashscope" && field.id === "base_url") {
    return "https://fixture.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1";
  }
  if (field.default) {
    return field.default;
  }
  throw new Error(`management_fixture_provider_setting_missing:${providerID}:${field.id}`);
}

/**
 * @param {object} provider
 * @param {Record<string, string>} fieldValues
 * @param {string} maskedValue
 */
function applyFixtureProviderConnection(provider, fieldValues, maskedValue) {
  for (const field of provider.fields) {
    const value = String(fieldValues[field.id] ?? "");
    if (field.secret) {
      if (value) {
        field.configured = true;
        field.masked_value = maskedValue;
      }
      continue;
    }
    field.value = value;
    field.configured = value !== "";
  }
  provider.configured = provider.fields.every((field) => !field.required || field.configured);
}

/** @param {object} provider */
function clearFixtureProviderConnection(provider) {
  provider.configured = false;
  for (const field of provider.fields) {
    if (field.secret) {
      field.configured = false;
      Reflect.deleteProperty(field, "masked_value");
    }
  }
}

/**
 * @param {boolean} isAdmin
 * @param {boolean} hasSecret
 * @returns {object}
 */
function managementProfile(isAdmin = false, hasSecret = true) {
  const profile = {
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
        configured: false,
        fields: [providerAPIKeyField("Anthropic API key")],
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
        configured: true,
        fields: [providerAPIKeyField("OpenAI API key", true, "sk-...1234")],
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
        configured: true,
        fields: [providerAPIKeyField("DeepSeek API key", true, "sk-...5678")],
        text_model: "deepseek-chat",
        system_prompt: "",
        text_default_model: "deepseek-chat",
        text_models: [{ id: "deepseek-chat" }],
        supports_dictation: false,
        dictation_models: [],
      },
      {
        id: "gemini",
        label: "Gemini",
        aliases: [],
        configured: false,
        fields: [providerAPIKeyField("Gemini API key")],
        text_model: "gemini-3.5-flash",
        system_prompt: "",
        text_default_model: "gemini-3.5-flash",
        text_models: [{ id: "gemini-3.5-flash" }],
        supports_dictation: false,
        dictation_models: [],
      },
      {
        id: "dashscope",
        label: "DashScope",
        aliases: ["qwen"],
        configured: false,
        fields: [providerAPIKeyField("DashScope API key"), dashScopeBaseURLField()],
        text_model: "qwen-plus",
        system_prompt: "",
        text_default_model: "qwen-plus",
        text_models: [
          { id: "qwen-plus" },
          { id: "qwen3.6-flash" },
          { id: "qwen3.7-max" },
          { id: "qwen3.7-plus" },
        ],
        supports_dictation: false,
        dictation_models: [],
      },
      {
        id: "meta",
        label: "Meta",
        aliases: [],
        configured: true,
        fields: [providerAPIKeyField("Meta API key", true, "sk-...meta")],
        text_model: "muse-spark-1.1",
        system_prompt: "",
        text_default_model: "muse-spark-1.1",
        text_models: [{ id: "muse-spark-1.1" }, { id: "muse-spark-1.2" }],
        supports_dictation: false,
        dictation_models: [],
      },
      {
        id: "minimax",
        label: "MiniMax",
        aliases: [],
        configured: false,
        fields: [providerAPIKeyField("MiniMax API key")],
        text_model: "minimax-m2.7",
        system_prompt: "",
        text_default_model: "minimax-m2.7",
        text_models: [
          { id: "minimax-m2" },
          { id: "minimax-m2.1" },
          { id: "minimax-m2.1-highspeed" },
          { id: "minimax-m2.5" },
          { id: "minimax-m2.5-highspeed" },
          { id: "minimax-m2.7" },
          { id: "minimax-m2.7-highspeed" },
        ],
        supports_dictation: false,
        dictation_models: [],
      },
      {
        id: "moonshot",
        label: "Moonshot",
        aliases: ["kimi"],
        configured: false,
        fields: [providerAPIKeyField("Moonshot API key")],
        text_model: "kimi-k2.6",
        system_prompt: "",
        text_default_model: "kimi-k2.6",
        text_models: [
          { id: "kimi-k2.6" },
          { id: "kimi-k2.7-code" },
          { id: "kimi-k2.7-code-highspeed" },
          {
            id: "kimi-k3",
            reasoning_effort: {
              adapter: "moonshot_chat_completions",
              efforts: ["low", "high", "max"],
            },
          },
        ],
        supports_dictation: false,
        dictation_models: [],
      },
      {
        id: "xai",
        label: "xAI",
        aliases: ["xai"],
        configured: false,
        fields: [providerAPIKeyField("xAI API key")],
        text_model: "grok-4.3",
        system_prompt: "",
        text_default_model: "grok-4.3",
        text_models: [{ id: "grok-4.3" }],
        supports_dictation: true,
        dictation_default_model: "xai-stt",
        dictation_models: ["xai-stt"],
      },
      {
        id: "zai",
        label: "Z.AI",
        aliases: [],
        configured: false,
        fields: [providerAPIKeyField("Z.AI API key")],
        text_model: "glm-5.1",
        system_prompt: "",
        text_default_model: "glm-5.1",
        text_models: [{ id: "glm-5.1" }, { id: "glm-5.2" }],
        supports_dictation: true,
        dictation_default_model: "glm-asr-2512",
        dictation_models: ["glm-asr-2512"],
      },
    ],
    proxy: {
      text_path: "/",
      v2_path: "/v2",
      dictation_path: "/dictate",
    },
  };
  profile.providers.push({
    id: "siliconflow",
    label: "SiliconFlow",
    aliases: [],
    configured: false,
    fields: [providerAPIKeyField("SiliconFlow API key")],
    text_model: "deepseek-reasoner",
    system_prompt: "",
    text_default_model: "deepseek-reasoner",
    text_models: [{ id: "deepseek-reasoner" }],
    supports_dictation: true,
    dictation_default_model: "sensevoice-small",
    dictation_models: ["sensevoice-small"],
  });
  /** @type {Record<string, {api_service_label: string, model_families: Array<{id: string, label: string}>}>} */
  const providerIdentities = {
    openai: {
      api_service_label: "OpenAI API",
      model_families: [
        { id: "gpt-4", label: "GPT-4" },
        { id: "gpt-5", label: "GPT-5" },
        { id: "gpt-transcribe", label: "GPT Transcribe" },
      ],
    },
    deepseek: {
      api_service_label: "DeepSeek API",
      model_families: [
        { id: "deepseek-v4", label: "DeepSeek V4" },
        { id: "deepseek-v3", label: "DeepSeek V3" },
        { id: "deepseek-r1", label: "DeepSeek R1" },
      ],
    },
    dashscope: {
      api_service_label: "DashScope API",
      model_families: [{ id: "qwen", label: "Qwen" }],
    },
    moonshot: {
      api_service_label: "Moonshot API",
      model_families: [
        { id: "kimi-k2", label: "Kimi K2" },
        { id: "kimi-k3", label: "Kimi K3" },
      ],
    },
    minimax: {
      api_service_label: "MiniMax API",
      model_families: [{ id: "minimax-m2", label: "MiniMax M2" }],
    },
    siliconflow: {
      api_service_label: "SiliconFlow API",
      model_families: [
        { id: "deepseek-r1", label: "DeepSeek R1" },
        { id: "sensevoice", label: "SenseVoice" },
      ],
    },
    zai: {
      api_service_label: "Z.AI API",
      model_families: [
        { id: "glm-5", label: "GLM-5" },
        { id: "glm-asr", label: "GLM ASR" },
      ],
    },
    gemini: {
      api_service_label: "Gemini API",
      model_families: [{ id: "gemini", label: "Gemini" }],
    },
    anthropic: {
      api_service_label: "Anthropic API",
      model_families: [
        { id: "claude-fable", label: "Claude Fable" },
        { id: "claude-sonnet", label: "Claude Sonnet" },
        { id: "claude-opus", label: "Claude Opus" },
        { id: "claude-haiku", label: "Claude Haiku" },
      ],
    },
    meta: {
      api_service_label: "Meta API",
      model_families: [{ id: "muse-spark", label: "Muse Spark" }],
    },
    xai: {
      api_service_label: "xAI API",
      model_families: [
        { id: "grok", label: "Grok" },
        { id: "grok-build", label: "Grok Build" },
        { id: "grok-code", label: "Grok Code" },
        { id: "grok-imagine", label: "Grok Imagine" },
        { id: "xai-stt", label: "xAI STT" },
      ],
    },
  };
  const catalogOrder = ["openai", "deepseek", "dashscope", "moonshot", "minimax", "siliconflow", "zai", "gemini", "anthropic", "meta", "xai"];
  const acquisitionURLs = {
    openai: "https://platform.openai.com/api-keys",
    deepseek: "https://platform.deepseek.com/api_keys",
    dashscope: "https://help.aliyun.com/en/model-studio/get-api-key",
    moonshot: "https://platform.kimi.ai/console/api-keys",
    minimax: "https://platform.minimax.io/docs/guides/quickstart-preparation",
    siliconflow: "https://cloud.siliconflow.com/account/ak",
    zai: "https://z.ai/manage-apikey/apikey-list",
    gemini: "https://aistudio.google.com/app/apikey",
    anthropic: "https://platform.claude.com/settings/keys",
    meta: "https://dev.meta.ai/",
    xai: "https://console.x.ai/team/default/api-keys",
  };
  /** @type {Record<string, string[]>} */
  const mediaCapabilities = {
    openai: ["image_input"],
    dashscope: ["image_input"],
    moonshot: ["image_input"],
    gemini: ["image_input", "audio_input"],
    anthropic: ["image_input"],
    xai: ["image_input"],
  };
  for (const provider of profile.providers) {
    const identity = providerIdentities[provider.id];
    if (!identity) throw new Error(`management_fixture_provider_identity_missing:${provider.id}`);
    Object.assign(provider, identity);
    provider.key_acquisition_url = acquisitionURLs[provider.id];
    provider.capabilities = ["text", ...(mediaCapabilities[provider.id] || []), ...(provider.supports_dictation ? ["dictation"] : []), ...(provider.id === "xai" ? ["video_generation"] : [])];
  }
  profile.providers.sort((first, second) => catalogOrder.indexOf(first.id) - catalogOrder.indexOf(second.id));
  return profile;
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
    start: new Date(Date.UTC(2026, 5, interval === "1d" ? 1 : index + 1, interval === "1d" ? index : 0)).toISOString(),
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
    rejected_requests: 0,
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
      endpoint: "text",
      provider: "deepseek",
      model: "deepseek-chat",
      status_code: 499,
      outcome_code: "request_timeout",
      latency_ms: 17,
    },
    {
      endpoint: "v2",
      provider: "openai",
      model: "gpt-4.1",
      status_code: 500,
      outcome_code: "proxy_error",
      latency_ms: 2,
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
 * @param {string} interval
 * @returns {object}
 */
function managementUsageRejections(interval) {
  return {
    interval,
    rejections: [
      {
        tenant_id: "tenant_1",
        tenant_name: "Default",
        occurred_at: "2026-07-25T12:00:00Z",
        endpoint: "v2",
        provider: "deepseek",
        model: "deepseek-v4-flash",
        status_code: 409,
        outcome_code: "provider_not_configured",
        latency_ms: 2,
      },
      {
        tenant_id: "tenant_1",
        tenant_name: "Default",
        occurred_at: "2026-07-25T11:59:59Z",
        endpoint: "text",
        provider: "",
        model: "",
        status_code: 400,
        outcome_code: "invalid_request",
        latency_ms: 1,
      },
      {
        tenant_id: "tenant_2",
        tenant_name: "Research",
        occurred_at: "2026-07-25T11:59:58Z",
        endpoint: "v2",
        provider: "",
        model: "",
        status_code: 413,
        outcome_code: "payload_too_large",
        latency_ms: 3,
      },
    ],
  };
}

/**
 * @returns {object}
 */
function managementAdminUsage() {
  const summary = managementUsage("30d");
  return {
    period_days: 30,
    rejected_requests: summary.rejected_requests,
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
    if (!this.hasAttribute("data-config-url")) {
      return;
    }
    const restoredStatus = sessionStorage.getItem("llm-proxy-test-auth-status") || ${JSON.stringify(initialAuthStatus)};
    this.setAuthStatus(restoredStatus);
    queueMicrotask(() => {
      this.dispatchEvent(new CustomEvent("mpr-ui:auth:status-change", {
        bubbles: true,
        detail: { status: restoredStatus }
      }));
      if (restoredStatus === "authenticated" && ${JSON.stringify(emitInitialAuthEvent)}) {
        this.dispatchEvent(new CustomEvent("mpr-ui:auth:authenticated", {
          bubbles: true,
          detail: { profile: { user_id: "user-1", user_email: "user@example.com" } }
        }));
      }
    });
  }

  mountActions() {
    const horizontalLinks = JSON.parse(this.getAttribute("horizontal-links") || '{"links":[]}');
    if (horizontalLinks.links.length > 0) {
      const navigation = document.createElement("nav");
      navigation.setAttribute("aria-label", "Utility links");
      horizontalLinks.links.forEach((item) => {
        const anchor = document.createElement("a");
        anchor.textContent = item.label;
        anchor.setAttribute("href", item.href || item.url);
        navigation.append(anchor);
      });
      this.append(navigation);
    }
    const actions = document.createElement("div");
    actions.className = "mpr-header__actions";
    if (this.hasAttribute("data-config-url")) {
      const signIn = document.createElement("button");
      signIn.type = "button";
      signIn.dataset.testid = "sign-in";
      signIn.textContent = this.getAttribute("sign-in-label") || "Sign in";
      signIn.addEventListener("click", () => {
        this.__signInIntent = true;
      });
      actions.append(signIn);
    }
    actions.append(...this.querySelectorAll('[slot="aux"]'));
    if (actions.childElementCount > 0) {
      this.append(actions);
    }
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
    const linksCollection = JSON.parse(this.getAttribute("links-collection") || '{"links":[]}');
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
    const privacy = document.createElement("a");
    privacy.textContent = this.getAttribute("privacy-link-label") || "Privacy";
    privacy.setAttribute("href", this.getAttribute("privacy-link-href") || "/privacy/");
    const theme = document.createElement("button");
    theme.type = "button";
    theme.setAttribute("aria-label", "Toggle theme");
    theme.textContent = "◩";
    const menuWrapper = document.createElement("div");
    const menuToggle = document.createElement("button");
    menuToggle.type = "button";
    menuToggle.textContent = linksCollection.text || "Built by Marco Polo Research Lab";
    menuToggle.setAttribute("aria-haspopup", "true");
    menuToggle.setAttribute("aria-expanded", "false");
    const menu = document.createElement("ul");
    menu.hidden = true;
    linksCollection.links.forEach((item) => {
      const menuItem = document.createElement("li");
      const anchor = document.createElement("a");
      anchor.textContent = item.label;
      anchor.setAttribute("href", item.url || item.href);
      menuItem.append(anchor);
      menu.append(menuItem);
    });
    const closeMenu = () => {
      menu.hidden = true;
      menuToggle.setAttribute("aria-expanded", "false");
    };
    menuToggle.addEventListener("click", () => {
      const opening = menu.hidden;
      menu.hidden = !opening;
      menuToggle.setAttribute("aria-expanded", opening ? "true" : "false");
    });
    document.addEventListener("keydown", (event) => {
      if (event.key === "Escape") {
        closeMenu();
      }
    });
    menuWrapper.append(theme, menuToggle, menu);
    footer.append(privacy, navigation, menuWrapper);
    this.replaceChildren(footer);
  }
}
class MprLegalDocument extends HTMLElement {
  connectedCallback() {
    this.setAttribute("data-mpr-legal-document-type", this.getAttribute("type") || "terms");
    const sections = JSON.parse(this.getAttribute("sections") || "[]");
    this.setAttribute("data-mpr-legal-document-section-count", String(sections.length));
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
customElements.define("mpr-legal-document", MprLegalDocument);
window.__llmProxyMprAuthenticate = () => {
  const header = document.querySelector("mpr-header");
  if (!header) {
    throw new Error("mpr_header_missing");
  }
  sessionStorage.setItem("llm-proxy-test-auth-status", "authenticated");
  header.setAuthStatus("authenticated");
  header.dispatchEvent(new CustomEvent("mpr-ui:auth:status-change", {
    bubbles: true,
    detail: { status: "authenticated" }
  }));
  header.dispatchEvent(new CustomEvent("mpr-ui:auth:authenticated", {
    bubbles: true,
    detail: { profile: { user_id: "user-1", user_email: "user@example.com" } }
  }));
  const redirectURL = header.getAttribute("sign-in-redirect-url");
  if (header.__signInIntent && redirectURL) {
    location.assign(redirectURL);
  }
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

mpr-header > nav {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
  font-size: 10px;
  white-space: nowrap;
}

mpr-footer {
  position: relative;
  display: block;
  min-height: 64px;
  background: rgba(3, 23, 32, 0.95);
}

mpr-footer[size="small"] {
  min-height: 48px;
}

mpr-footer footer {
  position: fixed;
  right: 0;
  bottom: 0;
  left: 0;
  z-index: 1200;
  display: flex;
  min-height: inherit;
  box-sizing: border-box;
  padding: 8px 12px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  border-top: 1px solid rgba(148, 163, 184, 0.25);
}

mpr-footer[sticky="false"] footer {
  position: relative;
}

mpr-footer footer > div {
  position: relative;
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 6px;
  font-size: 10px;
  white-space: nowrap;
}

mpr-footer footer > div ul {
  position: absolute;
  right: 0;
  bottom: 100%;
  width: 180px;
  margin: 0;
  padding: 8px;
  background: rgba(3, 23, 32, 0.98);
}

mpr-footer footer > div ul[hidden] {
  display: none;
}

mpr-footer nav {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
  font-size: 10px;
  white-space: nowrap;
}

mpr-footer footer > a {
  flex: 0 0 auto;
  font-size: 10px;
  font-weight: 600;
  white-space: nowrap;
}
`;
}
