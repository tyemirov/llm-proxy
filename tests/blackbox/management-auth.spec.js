// @ts-check

import { expect, test } from "@playwright/test";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  localManagementProfile,
  startLocalManagementStack,
} from "./localManagementStack.mjs";
import {
  APPLICATION_PATH,
  LANDING_AUTHENTICATED_REDIRECT_ATTRIBUTE,
} from "../../site/assets/llm-proxy/js/constants.js";
import { PUBLIC_FOOTER_COMPACT_MAX_HEIGHT } from "../../scripts/public_site_shell.mjs";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const httpOK = 200;
const httpCreated = 201;
const httpNoContent = 204;
const httpBadRequest = 400;
const httpUnauthorized = 401;
const httpNotFound = 404;
const minimumReadableTextContrastRatio = 4.5;
const cssRGBChannelCount = 3;
const cssRGBChannelMaximum = 255;
const cssRGBLinearThreshold = 0.04045;
const cssRGBLinearDivisor = 12.92;
const cssRGBOffset = 0.055;
const cssRGBScale = 1.055;
const cssRGBExponent = 2.4;
const contrastLuminanceOffset = 0.05;
const redLuminanceWeight = 0.2126;
const greenLuminanceWeight = 0.7152;
const blueLuminanceWeight = 0.0722;
const mprUIBundleURL = "https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui.js";
const applicationPath = APPLICATION_PATH;

let stack;

test.beforeAll(async () => {
  stack = await startLocalManagementStack();
});

test.afterAll(async () => {
  if (stack) {
    await stack.stop();
  }
});

test("public Log In opens the authenticated app and the TAuth session survives until explicit sign out", async ({ browser, context, page }) => {
  let browserAccountRequestCount = 0;
  let browserSecretRequestCount = 0;
  const browserSessionRequestHeaders = [];
  page.on("request", (request) => {
    if (request.url() === `${stack.llmProxyOrigin}/api/management/account`) {
      browserAccountRequestCount += 1;
    }
    if (
      new URL(request.url()).pathname.match(/^\/api\/management\/tenants\/[^/]+\/secrets$/) &&
      request.method() === "POST"
    ) {
      browserSecretRequestCount += 1;
    }
    if (request.url() === `${stack.tAuthOrigin}/auth/session` && request.method() === "GET") {
      browserSessionRequestHeaders.push(request.headers());
    }
  });
  await installLocalAssetRoutes(page);
  const googleCredentialExchange = await installGoogleCredentialExchangeFixture(context, page);
  await installAuthStateHistory(page);

  const browserConfigResponse = await context.request.get(`${stack.frontendOrigin}/config-ui.yaml`);
  expect(browserConfigResponse.status()).toBe(httpOK);
  const browserConfig = await browserConfigResponse.text();
  expect(browserConfig).toContain(`managementApiOrigin: "${stack.llmProxyOrigin}"`);
  expect(browserConfig).toContain(`tauthUrl: "${stack.tAuthOrigin}"`);
  expect(browserConfig).toContain(`tenantId: "${localManagementProfile.tenantID}"`);
  expect(browserConfig).toContain('sessionPath: "/auth/session"');
  expect(browserConfig).not.toContain("authButton");

  const anonymousAccountResponse = await context.request.get(`${stack.llmProxyOrigin}/api/management/account`, {
    headers: { Origin: stack.frontendOrigin },
  });
  expect(anonymousAccountResponse.status()).toBe(httpUnauthorized);

  await page.goto(`${stack.frontendOrigin}/`);
  await expect(page.locator("#mpr-ui-bundle")).toHaveAttribute("data-mpr-ui-bundle-src", mprUIBundleURL);
  await expect.poll(() => page.evaluate(() => Boolean(customElements.get("mpr-legal-document")))).toBe(true);
  await expect(page.getByRole("heading", { name: "Integrate once. Use the model that fits." })).toBeVisible();
  await expect(page.locator("llm-proxy-key-management")).toHaveCount(0);
  await expect(page.locator("mpr-header")).toHaveAttribute("data-mpr-auth-status", "unauthenticated");
  await expect(page.locator("mpr-header")).toHaveAttribute("sign-in-label", "Log In");
  await expect(page.locator("mpr-header")).toHaveAttribute(
    LANDING_AUTHENTICATED_REDIRECT_ATTRIBUTE,
    applicationPath,
  );
  await expect(page.locator("mpr-header")).not.toHaveAttribute("sign-in-redirect-url", applicationPath);
  expect(browserAccountRequestCount).toBe(0);
  const signInButton = page.locator('[data-mpr-header="google-signin-button"]');
  await expect(signInButton).toBeVisible();
  await signInButton.hover();
  const signInHoverColors = await signInButton.evaluate((buttonElement) => {
    const buttonStyle = window.getComputedStyle(buttonElement);
    return {
      backgroundColor: buttonStyle.backgroundColor,
      color: buttonStyle.color,
    };
  });
  expect(cssColorContrastRatio(signInHoverColors.color, signInHoverColors.backgroundColor)).toBeGreaterThanOrEqual(
    minimumReadableTextContrastRatio,
  );

  await page.goto(`${stack.frontendOrigin}${applicationPath}`);
  await expect(page).toHaveURL(`${stack.frontendOrigin}/`);
  await expect(page.getByRole("heading", { name: "Integrate once. Use the model that fits." })).toBeVisible();
  await expect(page.locator("llm-proxy-key-management")).toHaveCount(0);
  expect(browserAccountRequestCount).toBe(0);

  const loginResponsePromise = page.waitForResponse(
    (response) =>
      response.url() === `${stack.tAuthOrigin}/auth/google` && response.request().method() === "POST",
  );
  const restoredSessionResponsePromise = waitForSessionRestore(page);
  const authenticatedAccountResponsePromise = waitForManagementAccount(page);
  const generatedSecretResponsePromise = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname.match(/^\/api\/management\/tenants\/[^/]+\/secrets$/) &&
      response.request().method() === "POST",
  );
  await signInButton.click();
  const loginResponse = await loginResponsePromise;
  expect(loginResponse.status()).toBe(httpOK);
  const credentialExchangeResult = await googleCredentialExchange.result;
  expect(credentialExchangeResult.googlePayload).toEqual({
    google_id_token: "local-blackbox-google-credential",
    nonce_token: expect.any(String),
  });
  expect(credentialExchangeResult.tAuthStatus).toBe(httpOK);
  expect(credentialExchangeResult.tAuthHeaders["access-control-allow-origin"]).toBe(stack.frontendOrigin);
  expect(credentialExchangeResult.tAuthHeaders["access-control-allow-credentials"]).toBe("true");

  await expect(page).toHaveURL(`${stack.frontendOrigin}${applicationPath}`);
  const restoredSessionResponse = await restoredSessionResponsePromise;
  expect(restoredSessionResponse.status()).toBe(httpOK);
  expect(browserSessionRequestHeaders).toEqual(expect.arrayContaining([
    expect.objectContaining({
      "x-tauth-tenant": localManagementProfile.tenantID,
    }),
  ]));
  expect(restoredSessionResponse.request().headers()).not.toHaveProperty("origin");
  expect(restoredSessionResponse.request().headers()["x-requested-with"]).toBe("XMLHttpRequest");

  const sessionCookies = await context.cookies();
  expect(sessionCookies).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        name: localManagementProfile.sessionCookieName,
        domain: "localhost",
        httpOnly: true,
        path: "/",
        secure: false,
      }),
      expect.objectContaining({
        name: localManagementProfile.refreshCookieName,
        domain: "localhost",
        httpOnly: true,
        path: "/auth",
        secure: false,
      }),
    ]),
  );

  await expect(page.locator("mpr-header")).toHaveAttribute("data-mpr-auth-status", "authenticated");
  const authenticatedAccountResponse = await authenticatedAccountResponsePromise;
  expect(authenticatedAccountResponse.status()).toBe(httpOK);
  const authenticatedAccount = await authenticatedAccountResponse.json();
  expect(authenticatedAccount).toMatchObject({
    user: {
      email: localManagementProfile.operatorEmail,
      display_name: "Local Operator",
    },
    tenants: [{ name: "Default" }],
  });
  const firstTenantID = authenticatedAccount.tenants[0].id;
  expect(firstTenantID).toMatch(/^managed-/);
  expect(browserAccountRequestCount).toBe(1);

  const generatedSecretResponse = await generatedSecretResponsePromise;
  expect(generatedSecretResponse.status()).toBe(httpOK);
  expect(generatedSecretResponse.headers()["cache-control"]).toBe("no-store");
  const firstGeneratedSecret = (await generatedSecretResponse.json()).secret;
  expect(firstGeneratedSecret).toMatch(/^llmp_/);
  expect(browserSecretRequestCount).toBe(1);

  const settingsDialog = page.getByRole("dialog", { name: "Settings" });
  await expect(settingsDialog).toBeVisible();
  await expect(settingsDialog.getByRole("combobox", { name: "Tenant" })).toHaveValue(firstTenantID);
  await expect(settingsDialog.getByRole("button", { name: "Create tenant" })).toBeVisible();
  await expect(settingsDialog.getByRole("alert")).toHaveText(
    "Add at least one provider API key before leaving Settings.",
  );
  const clientKeyInput = settingsDialog.getByRole("textbox", { name: "Key", exact: true });
  await expect(clientKeyInput).toHaveValue("••••••••••••");
  await expect(clientKeyInput).toHaveAttribute("readonly", "");
  await settingsDialog.locator("tenant-access-row").getByRole("button", { name: "Show key", exact: true }).click();
  await expect(clientKeyInput).toHaveValue(/^llmp_/);

  const providerEditor = settingsDialog.locator("provider-editor");
  await providerEditor.getByRole("combobox", { name: "Provider", exact: true }).selectOption("openai");
  await providerEditor.getByRole("textbox", { name: "OpenAI API key" }).fill("sk-local-blackbox-provider-key");
  const providerSaveResponsePromise = page.waitForResponse(
    (response) =>
      response.url() === `${stack.llmProxyOrigin}/api/management/tenants/${firstTenantID}/provider-keys/openai` &&
      response.request().method() === "PUT",
  );
  await page.keyboard.press("Tab");
  expect((await providerSaveResponsePromise).status()).toBe(httpOK);
  await expect(providerEditor.getByRole("button", { name: /^(Save|Update) key$/ })).toHaveCount(0);
  await expect(settingsDialog.getByRole("alert")).toBeHidden();
  await expect(settingsDialog).toBeVisible();
  await settingsDialog.getByRole("button", { name: "Close" }).click();
  await expect(settingsDialog).toBeHidden();

  await expectAuthenticatedDashboard(page);
  await expectNoSignedOutStateAfterAuthentication(page);

  const restoredLandingSessionResponsePromise = waitForSessionRestore(page);
  await page.goto(`${stack.frontendOrigin}/`);
  expect((await restoredLandingSessionResponsePromise).status()).toBe(httpOK);
  await expect(page).toHaveURL(`${stack.frontendOrigin}${applicationPath}`);
  await expectAuthenticatedDashboard(page);
  await expectNoSignedOutState(page);

  const createSecondTenantResponse = await context.request.post(
    `${stack.llmProxyOrigin}/api/management/tenants`,
    {
      headers: { Origin: stack.frontendOrigin },
      data: { name: "Research" },
    },
  );
  expect(createSecondTenantResponse.status()).toBe(httpCreated);
  const secondTenantProfile = await createSecondTenantResponse.json();
  const secondTenantID = secondTenantProfile.tenant.id;
  expect(secondTenantID).toMatch(/^managed-/);
  expect(secondTenantID).not.toBe(firstTenantID);

  const secondTenantSecretResponse = await context.request.post(
    `${stack.llmProxyOrigin}/api/management/tenants/${secondTenantID}/secrets`,
    {
      headers: { Origin: stack.frontendOrigin },
      data: {},
    },
  );
  expect(secondTenantSecretResponse.status()).toBe(httpOK);
  const secondGeneratedSecret = (await secondTenantSecretResponse.json()).secret;
  expect(secondGeneratedSecret).toMatch(/^llmp_/);
  expect(secondGeneratedSecret).not.toBe(firstGeneratedSecret);

  for (const generatedSecret of [firstGeneratedSecret, secondGeneratedSecret]) {
    const authenticatedPublicResponse = await context.request.get(
      `${stack.llmProxyOrigin}/?key=${encodeURIComponent(generatedSecret)}`,
    );
    expect(authenticatedPublicResponse.status()).toBe(httpBadRequest);
  }
  for (const tenantID of [firstTenantID, secondTenantID]) {
    const tenantProfileResponse = await context.request.get(
      `${stack.llmProxyOrigin}/api/management/tenants/${tenantID}`,
      { headers: { Origin: stack.frontendOrigin } },
    );
    expect(tenantProfileResponse.status()).toBe(httpOK);
    const tenantUsageResponse = await context.request.get(
      `${stack.llmProxyOrigin}/api/management/tenants/${tenantID}/usage?interval=30d`,
      { headers: { Origin: stack.frontendOrigin } },
    );
    expect(tenantUsageResponse.status()).toBe(httpOK);
  }
  const twoTenantAccountResponse = await context.request.get(
    `${stack.llmProxyOrigin}/api/management/account`,
    { headers: { Origin: stack.frontendOrigin } },
  );
  expect(twoTenantAccountResponse.status()).toBe(httpOK);
  expect((await twoTenantAccountResponse.json()).tenants.map((tenant) => tenant.id)).toEqual([
    firstTenantID,
    secondTenantID,
  ]);
  const accountUsageResponse = await context.request.get(
    `${stack.llmProxyOrigin}/api/management/usage?interval=30d`,
    { headers: { Origin: stack.frontendOrigin } },
  );
  expect(accountUsageResponse.status()).toBe(httpOK);
  expect(accountUsageResponse.headers()["cache-control"]).toBe("no-store");
  expect(await accountUsageResponse.json()).toMatchObject({
    interval: "30d",
    totals: {
      requests: 2,
      failed_requests: 2,
    },
  });
  const accountUsageFailuresResponse = await context.request.get(
    `${stack.llmProxyOrigin}/api/management/usage/failures?interval=30d&limit=10`,
    { headers: { Origin: stack.frontendOrigin } },
  );
  expect(accountUsageFailuresResponse.status()).toBe(httpOK);
  expect(accountUsageFailuresResponse.headers()["cache-control"]).toBe("no-store");
  const accountUsageFailures = await accountUsageFailuresResponse.json();
  expect(accountUsageFailures.interval).toBe("30d");
  expect(accountUsageFailures.failures).toHaveLength(2);
  expect(
    accountUsageFailures.failures
      .map((failure) => [failure.tenant_id, failure.tenant_name])
      .sort(([leftTenantID], [rightTenantID]) => leftTenantID.localeCompare(rightTenantID)),
  ).toEqual([
    [firstTenantID, "Default"],
    [secondTenantID, "Research"],
  ].sort(([leftTenantID], [rightTenantID]) => leftTenantID.localeCompare(rightTenantID)));
  expect(accountUsageFailures.failures).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        outcome_code: "invalid_request",
        status_code: httpBadRequest,
      }),
    ]),
  );

  const secondUserContext = await browser.newContext();
  try {
    const secondUserLoginResponse = await secondUserContext.request.post(
      `${stack.tAuthOrigin}/auth/password/login`,
      {
        headers: {
          Origin: stack.frontendOrigin,
          "X-Requested-With": "XMLHttpRequest",
          "X-TAuth-Tenant": localManagementProfile.tenantID,
        },
        data: {
          email: localManagementProfile.secondOperatorEmail,
          password: localManagementProfile.operatorPassword,
        },
      },
    );
    expect(secondUserLoginResponse.status()).toBe(httpOK);
    const secondUserAccountResponse = await secondUserContext.request.get(
      `${stack.llmProxyOrigin}/api/management/account`,
      { headers: { Origin: stack.frontendOrigin } },
    );
    expect(secondUserAccountResponse.status()).toBe(httpOK);
    const secondUserAccount = await secondUserAccountResponse.json();
    expect(secondUserAccount.user.email).toBe(localManagementProfile.secondOperatorEmail);
    expect(secondUserAccount.tenants).toHaveLength(1);
    expect(secondUserAccount.tenants.map((tenant) => tenant.id)).not.toContain(firstTenantID);
    expect(secondUserAccount.tenants.map((tenant) => tenant.id)).not.toContain(secondTenantID);

    for (const tenantID of [firstTenantID, secondTenantID]) {
      const foreignReadResponse = await secondUserContext.request.get(
        `${stack.llmProxyOrigin}/api/management/tenants/${tenantID}`,
        { headers: { Origin: stack.frontendOrigin } },
      );
      expect(foreignReadResponse.status()).toBe(httpNotFound);
      const foreignRenameResponse = await secondUserContext.request.put(
        `${stack.llmProxyOrigin}/api/management/tenants/${tenantID}`,
        {
          headers: { Origin: stack.frontendOrigin },
          data: { name: "Forbidden rename" },
        },
      );
      expect(foreignRenameResponse.status()).toBe(httpNotFound);
    }
  } finally {
    await secondUserContext.close();
  }

  const ordinaryReloadSessionResponsePromise = waitForSessionRestore(page);
  await page.reload();
  expect((await ordinaryReloadSessionResponsePromise).status()).toBe(httpOK);
  await expectAuthenticatedDashboard(page);
  await expect(settingsDialog).toBeHidden();
  await expectNoSignedOutState(page);
  expect(browserSecretRequestCount).toBe(1);

  await context.clearCookies({ name: localManagementProfile.sessionCookieName });
  await expectCookies(context, {
    session: false,
    refresh: true,
  });

  const recoveredSessionResponsePromise = waitForSessionRestore(page);
  await page.goto(`${stack.frontendOrigin}/`);
  expect((await recoveredSessionResponsePromise).status()).toBe(httpOK);
  await expectAuthenticatedDashboard(page);
  await expect(settingsDialog).toBeHidden();
  await expectNoSignedOutState(page);
  expect(browserSecretRequestCount).toBe(1);
  await expectCookies(context, {
    session: true,
    refresh: true,
  });

  const browserCookies = await page.evaluate(() => document.cookie);
  expect(browserCookies).not.toContain(localManagementProfile.sessionCookieName);
  expect(browserCookies).not.toContain(localManagementProfile.refreshCookieName);

  await page.locator('[data-mpr-user="trigger"]').click();
  const logoutResponsePromise = page.waitForResponse(
    (response) => response.url() === `${stack.tAuthOrigin}/auth/logout` && response.request().method() === "POST",
  );
  await page.getByRole("menuitem", { name: "Sign out" }).click();
  expect((await logoutResponsePromise).status()).toBe(httpNoContent);

  await expect(page).toHaveURL(`${stack.frontendOrigin}/`);
  await expect(page.getByRole("heading", { name: "Integrate once. Use the model that fits." })).toBeVisible();
  await expect(page.locator("llm-proxy-key-management")).toHaveCount(0);
  await expectCookies(context, {
    session: false,
    refresh: false,
  });

  const signedOutTAuthResponse = await context.request.get(`${stack.tAuthOrigin}/auth/session`, {
    headers: {
      "X-TAuth-Tenant": localManagementProfile.tenantID,
    },
  });
  expect(signedOutTAuthResponse.status()).toBe(httpNoContent);

  const signedOutAccountResponse = await context.request.get(`${stack.llmProxyOrigin}/api/management/account`, {
    headers: { Origin: stack.frontendOrigin },
  });
  expect(signedOutAccountResponse.status()).toBe(httpUnauthorized);

  await page.setViewportSize({ width: 390, height: 780 });
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  const portfolioToggle = page.getByRole("button", { name: "Built by Marco Polo Research Lab" });
  await expect(portfolioToggle).toBeVisible();
  const footerGeometry = await page.getByRole("contentinfo").evaluate((footerElement) => {
    const footerBounds = footerElement.getBoundingClientRect();
    return {
      anchoredBottom: Math.abs(footerBounds.bottom - window.innerHeight) <= 0.5,
      anchoredLeft: Math.abs(footerBounds.left) <= 0.5,
      anchoredRight: Math.abs(footerBounds.right - document.documentElement.clientWidth) <= 0.5,
      height: footerBounds.height,
      position: getComputedStyle(footerElement).position,
      widthFits: footerElement.scrollWidth <= footerElement.clientWidth,
    };
  });
  expect(footerGeometry.position).toBe("fixed");
  expect(footerGeometry.anchoredBottom).toBe(true);
  expect(footerGeometry.anchoredLeft).toBe(true);
  expect(footerGeometry.anchoredRight).toBe(true);
  expect(footerGeometry.height).toBeLessThanOrEqual(PUBLIC_FOOTER_COMPACT_MAX_HEIGHT);
  expect(footerGeometry.widthFits).toBe(true);
  await portfolioToggle.click();
  await expect(portfolioToggle).toHaveAttribute("aria-expanded", "true");
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  for (const projectLinkName of ["Marco Polo Research Lab", "Wallpapers"]) {
    const projectLinkBox = await page.getByRole("link", { name: projectLinkName, exact: true }).boundingBox();
    if (!projectLinkBox) {
      throw new Error(`portfolio_link_geometry_missing: ${projectLinkName}`);
    }
    expect(projectLinkBox.x).toBeGreaterThanOrEqual(0);
    expect(projectLinkBox.x + projectLinkBox.width).toBeLessThanOrEqual(390);
  }
  await page.keyboard.press("Escape");
  await expect(portfolioToggle).toHaveAttribute("aria-expanded", "false");
});

async function expectAuthenticatedDashboard(page) {
  await expect(page.locator("llm-proxy-key-management")).toHaveAttribute("data-auth-state", "authenticated");
  await expect(page.getByRole("heading", { name: "Usage overview" })).toBeVisible();
  const usageTenantSelector = page.getByRole("combobox", { name: "Usage tenant" });
  await expect(usageTenantSelector).toHaveValue("");
  await expect(usageTenantSelector.locator("option:checked")).toHaveText("All tenants");
  await expect(page.locator("tenant-context-bar")).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "Sign in to manage LLM Proxy keys" })).toBeHidden();
  await expect(page.locator("mpr-user")).toHaveAttribute("data-mpr-user-status", "authenticated");
  await expect(page.locator("mpr-user")).toHaveAttribute("data-user-email", localManagementProfile.operatorEmail);
  await expect(page.locator('[data-mpr-user="trigger"]')).toHaveAttribute("aria-label", "Local Operator");
}

async function expectNoSignedOutState(page) {
  const authStateHistory = await page.evaluate(() => Reflect.get(window, "__llmProxyAuthStateHistory"));
  expect(authStateHistory).toContain("authenticated");
  expect(authStateHistory).not.toContain("unauthenticated");
}

async function expectNoSignedOutStateAfterAuthentication(page) {
  const authStateHistory = await page.evaluate(() => Reflect.get(window, "__llmProxyAuthStateHistory"));
  const authenticatedStateIndex = authStateHistory.indexOf("authenticated");
  expect(authenticatedStateIndex).toBeGreaterThanOrEqual(0);
  expect(authStateHistory.slice(authenticatedStateIndex)).not.toContain("unauthenticated");
}

async function expectCookies(context, expected) {
  const cookies = await context.cookies();
  expect(cookies.some((cookie) => cookie.name === localManagementProfile.sessionCookieName)).toBe(expected.session);
  expect(cookies.some((cookie) => cookie.name === localManagementProfile.refreshCookieName)).toBe(expected.refresh);
}

/**
 * @param {string} foregroundColor
 * @param {string} backgroundColor
 * @returns {number}
 */
function cssColorContrastRatio(foregroundColor, backgroundColor) {
  const foregroundLuminance = cssColorRelativeLuminance(foregroundColor);
  const backgroundLuminance = cssColorRelativeLuminance(backgroundColor);
  const lighterLuminance = Math.max(foregroundLuminance, backgroundLuminance);
  const darkerLuminance = Math.min(foregroundLuminance, backgroundLuminance);
  return (lighterLuminance + contrastLuminanceOffset) / (darkerLuminance + contrastLuminanceOffset);
}

/**
 * @param {string} cssColor
 * @returns {number}
 */
function cssColorRelativeLuminance(cssColor) {
  const colorChannels = cssColor.match(/[\d.]+/g)?.slice(0, cssRGBChannelCount).map(Number);
  if (!colorChannels || colorChannels.length !== cssRGBChannelCount) {
    throw new Error(`css_color_invalid: ${cssColor}`);
  }
  const linearChannels = colorChannels.map((colorChannel) => {
    const normalizedChannel = colorChannel / cssRGBChannelMaximum;
    if (normalizedChannel <= cssRGBLinearThreshold) {
      return normalizedChannel / cssRGBLinearDivisor;
    }
    return ((normalizedChannel + cssRGBOffset) / cssRGBScale) ** cssRGBExponent;
  });
  const [redLuminance, greenLuminance, blueLuminance] = linearChannels;
  return (
    redLuminance * redLuminanceWeight +
    greenLuminance * greenLuminanceWeight +
    blueLuminance * blueLuminanceWeight
  );
}

function waitForSessionRestore(page) {
  return page.waitForResponse(
    (response) => response.url() === `${stack.tAuthOrigin}/auth/session` && response.request().method() === "GET",
  );
}

function waitForManagementAccount(page) {
  return page.waitForResponse(
    (response) =>
      response.url() === `${stack.llmProxyOrigin}/api/management/account` && response.request().method() === "GET",
  );
}

async function installAuthStateHistory(page) {
  await page.addInitScript(() => {
    const authStates = [];
    Reflect.set(window, "__llmProxyAuthStateHistory", authStates);
    const recordAuthState = () => {
      const authState = document.querySelector("llm-proxy-key-management")?.getAttribute("data-auth-state");
      if (authState && authStates.at(-1) !== authState) {
        authStates.push(authState);
      }
    };
    new MutationObserver(recordAuthState).observe(document, {
      attributes: true,
      attributeFilter: ["data-auth-state"],
      childList: true,
      subtree: true,
    });
  });
}

async function installLocalAssetRoutes(page) {
  await page.route("https://loopaware.mprlab.com/**", async (route) =>
    route.fulfill({ body: "", contentType: "application/javascript" }),
  );
  await page.route("https://accounts.google.com/gsi/client", async (route) =>
    route.fulfill({
      body: `(() => {
        if (window.google?.accounts?.id?.__llmProxyFixture) {
          return;
        }
        let initializeConfig = null;
        window.google = {
          accounts: {
            id: {
              __llmProxyFixture: true,
              initialize(config) {
                initializeConfig = config;
              },
              renderButton() {},
              prompt() {
                if (!initializeConfig || typeof initializeConfig.callback !== "function") {
                  throw new Error("google_identity_fixture_not_initialized");
                }
                queueMicrotask(() => initializeConfig.callback({
                  credential: "local-blackbox-google-credential",
                }));
              },
            },
          },
        };
      })();`,
      contentType: "application/javascript",
    }),
  );
  await page.route("**/alpinejs@3.13.5/dist/module.esm.js", async (route) =>
    fulfillLocalFile(route, "node_modules/alpinejs/dist/module.esm.js", "application/javascript"),
  );
  await page.route("**/js-yaml@4.3.0/dist/js-yaml.min.js", async (route) =>
    fulfillLocalFile(route, "node_modules/js-yaml/dist/js-yaml.min.js", "application/javascript"),
  );
}

async function installGoogleCredentialExchangeFixture(context, page) {
  /** @type {(value: { googlePayload: Record<string, unknown>, tAuthStatus: number, tAuthHeaders: Record<string, string> }) => void} */
  let resolveResult = () => {};
  /** @type {(reason?: unknown) => void} */
  let rejectResult = () => {};
  /** @type {Promise<{ googlePayload: Record<string, unknown>, tAuthStatus: number, tAuthHeaders: Record<string, string> }>} */
  const result = new Promise((resolve, reject) => {
    resolveResult = resolve;
    rejectResult = reject;
  });
  await page.route(`${stack.tAuthOrigin}/auth/google`, async (route) => {
    try {
      const googlePayload = route.request().postDataJSON();
      const tAuthResponse = await context.request.post(`${stack.tAuthOrigin}/auth/password/login`, {
        headers: {
          Origin: stack.frontendOrigin,
          "X-TAuth-Tenant": localManagementProfile.tenantID,
        },
        data: {
          email: localManagementProfile.operatorEmail,
          password: localManagementProfile.operatorPassword,
        },
      });
      const tAuthHeaders = tAuthResponse.headers();
      await route.fulfill({
        status: tAuthResponse.status(),
        contentType: tAuthHeaders["content-type"] || "application/json",
        body: await tAuthResponse.body(),
      });
      resolveResult({
        googlePayload,
        tAuthStatus: tAuthResponse.status(),
        tAuthHeaders,
      });
    } catch (error) {
      rejectResult(error);
      await route.abort("failed");
    }
  });
  return { result };
}

async function fulfillLocalFile(route, relativePath, contentType) {
  const body = await readFile(path.join(repoRoot, relativePath));
  await route.fulfill({ body, contentType });
}
