// @ts-check

import { expect, test } from "@playwright/test";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  localManagementProfile,
  startLocalManagementStack,
} from "./localManagementStack.mjs";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const httpOK = 200;
const httpNoContent = 204;
const httpUnauthorized = 401;

let stack;

test.beforeAll(async () => {
  stack = await startLocalManagementStack();
});

test.afterAll(async () => {
  if (stack) {
    await stack.stop();
  }
});

test("TAuth session survives reload and only explicit sign out clears it", async ({ context, page }) => {
  await installLocalAssetRoutes(page);
  await installAuthStateHistory(page);

  const browserConfigResponse = await context.request.get(`${stack.frontendOrigin}/config-ui.yaml`);
  expect(browserConfigResponse.status()).toBe(httpOK);
  const browserConfig = await browserConfigResponse.text();
  expect(browserConfig).toContain(`managementApiOrigin: "${stack.llmProxyOrigin}"`);
  expect(browserConfig).toContain(`tauthUrl: "${stack.tAuthOrigin}"`);
  expect(browserConfig).toContain(`tenantId: "${localManagementProfile.tenantID}"`);

  const anonymousProfileResponse = await context.request.get(`${stack.llmProxyOrigin}/api/management/profile`, {
    headers: { Origin: stack.frontendOrigin },
  });
  expect(anonymousProfileResponse.status()).toBe(httpUnauthorized);

  const anonymousBrowserProfileResponsePromise = waitForManagementProfile(page);
  await page.goto(stack.frontendOrigin);
  expect((await anonymousBrowserProfileResponsePromise).status()).toBe(httpUnauthorized);
  await expect(page.getByRole("heading", { name: "Sign in to manage LLM Proxy keys" })).toBeVisible();
  await expect(page.locator("llm-proxy-key-management")).toHaveAttribute("data-auth-state", "unauthenticated");
  await expect(page.locator("mpr-header")).toHaveAttribute("data-mpr-auth-status", "unauthenticated");

  const loginResponsePromise = page.waitForResponse(
    (response) =>
      response.url() === `${stack.tAuthOrigin}/auth/password/login` && response.request().method() === "POST",
  );
  const loginResult = await page.evaluate(async ({ tAuthOrigin, tenantID, email, password }) => {
    const response = await fetch(`${tAuthOrigin}/auth/password/login`, {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        "X-Requested-With": "XMLHttpRequest",
        "X-TAuth-Tenant": tenantID,
      },
      body: JSON.stringify({ email, password }),
    });
    return {
      status: response.status,
      profile: await response.json(),
    };
  }, {
    tAuthOrigin: stack.tAuthOrigin,
    tenantID: localManagementProfile.tenantID,
    email: localManagementProfile.operatorEmail,
    password: localManagementProfile.operatorPassword,
  });
  const loginResponse = await loginResponsePromise;
  expect(loginResponse.status()).toBe(httpOK);
  expect(loginResponse.headers()["access-control-allow-origin"]).toBe(stack.frontendOrigin);
  expect(loginResponse.headers()["access-control-allow-credentials"]).toBe("true");
  expect(loginResult).toMatchObject({
    status: httpOK,
    profile: {
      user_email: localManagementProfile.operatorEmail,
      display: "Local Operator",
    },
  });

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

  const authenticatedProfileResponsePromise = waitForManagementProfile(page);
  await page.evaluate((profile) => {
    window.MPRUI.testing.authenticate(document.querySelector("mpr-header"), profile);
  }, loginResult.profile);
  const authenticatedProfileResponse = await authenticatedProfileResponsePromise;
  expect(authenticatedProfileResponse.status()).toBe(httpOK);
  expect(await authenticatedProfileResponse.json()).toMatchObject({
    user: {
      email: localManagementProfile.operatorEmail,
      display_name: "Local Operator",
    },
  });

  await expectAuthenticatedDashboard(page);
  await expectNoSignedOutStateAfterAuthentication(page);

  const ordinaryReloadSessionResponsePromise = waitForSessionRestore(page);
  await page.reload();
  expect((await ordinaryReloadSessionResponsePromise).status()).toBe(httpOK);
  await expectAuthenticatedDashboard(page);
  await expectNoSignedOutState(page);

  await context.clearCookies({ name: localManagementProfile.sessionCookieName });
  await expectCookies(context, {
    session: false,
    refresh: true,
  });

  const recoveredSessionResponsePromise = waitForSessionRestore(page);
  await page.reload();
  expect((await recoveredSessionResponsePromise).status()).toBe(httpOK);
  await expectAuthenticatedDashboard(page);
  await expectNoSignedOutState(page);
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

  await expect(page.getByRole("heading", { name: "Sign in to manage LLM Proxy keys" })).toBeVisible();
  await expect(page.locator("llm-proxy-key-management")).toHaveAttribute("data-auth-state", "unauthenticated");
  await expectCookies(context, {
    session: false,
    refresh: false,
  });

  const signedOutTAuthResponse = await context.request.get(`${stack.tAuthOrigin}/auth/session`, {
    headers: {
      Origin: stack.frontendOrigin,
      "X-Requested-With": "XMLHttpRequest",
      "X-TAuth-Tenant": localManagementProfile.tenantID,
    },
  });
  expect(signedOutTAuthResponse.status()).toBe(httpNoContent);

  const signedOutProfileResponse = await context.request.get(`${stack.llmProxyOrigin}/api/management/profile`, {
    headers: { Origin: stack.frontendOrigin },
  });
  expect(signedOutProfileResponse.status()).toBe(httpUnauthorized);
});

async function expectAuthenticatedDashboard(page) {
  await expect(page.locator("llm-proxy-key-management")).toHaveAttribute("data-auth-state", "authenticated");
  await expect(page.getByRole("heading", { name: "Usage overview" })).toBeVisible();
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

function waitForSessionRestore(page) {
  return page.waitForResponse(
    (response) => response.url() === `${stack.tAuthOrigin}/auth/session` && response.request().method() === "GET",
  );
}

function waitForManagementProfile(page) {
  return page.waitForResponse(
    (response) =>
      response.url() === `${stack.llmProxyOrigin}/api/management/profile` && response.request().method() === "GET",
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
  await page.route("https://accounts.google.com/**", async (route) => route.abort());
  await page.route("**/alpinejs@3.13.5/dist/module.esm.js", async (route) =>
    fulfillLocalFile(route, "node_modules/alpinejs/dist/module.esm.js", "application/javascript"),
  );
  await page.route("**/js-yaml@4.3.0/dist/js-yaml.min.js", async (route) =>
    fulfillLocalFile(route, "node_modules/js-yaml/dist/js-yaml.min.js", "application/javascript"),
  );
  await page.route("**/mpr-ui@v3.11.1/mpr-ui.css", async (route) =>
    fulfillLocalFile(route, "node_modules/mpr-ui/mpr-ui.css", "text/css"),
  );
  await page.route("**/mpr-ui@v3.11.1/mpr-ui-config.js", async (route) =>
    fulfillLocalFile(route, "node_modules/mpr-ui/mpr-ui-config.js", "application/javascript"),
  );
  await page.route("**/mpr-ui@v3.11.1/mpr-ui.js", async (route) =>
    fulfillLocalFile(route, "node_modules/mpr-ui/mpr-ui.js", "application/javascript"),
  );
}

async function fulfillLocalFile(route, relativePath, contentType) {
  const body = await readFile(path.join(repoRoot, relativePath));
  await route.fulfill({ body, contentType });
}
