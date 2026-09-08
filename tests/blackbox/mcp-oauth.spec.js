// @ts-check
import { expect, test } from "@playwright/test";
import { createHash, randomBytes } from "node:crypto";
import { spawn } from "node:child_process";
import { mkdtemp, writeFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { stripVTControlCharacters } from "node:util";
import { localManagementProfile, startLocalManagementStack } from "./localManagementStack.mjs";

test("MCP OAuth login, consent, generation, refresh, and revocation", async ({ page, context }) => {
  const stack = await startLocalManagementStack();
  try {
    const callback = `${stack.frontendOrigin}/oauth/callback`;
    await page.route(`${callback}**`, (route) => route.fulfill({ status: 200, contentType: "text/html", body: "Authorization complete" }));
    const verifier = randomBytes(32).toString("base64url");
    const challenge = createHash("sha256").update(verifier).digest("base64url");
    const authorization = new URL(`${stack.tAuthOrigin}/oauth/authorize`);
    authorization.search = new URLSearchParams({ response_type: "code", client_id: "local-mcp-client", redirect_uri: callback, resource: stack.llmProxyOrigin, scope: "llm-proxy:use", code_challenge: challenge, code_challenge_method: "S256", state: "local-acceptance" }).toString();
    await page.goto(authorization.href);
    await page.getByLabel("Email", { exact: true }).fill(localManagementProfile.operatorEmail);
    await page.getByLabel("Password", { exact: true }).fill(localManagementProfile.operatorPassword);
    await page.getByRole("button", { name: "Continue", exact: true }).click();
    await expect(page.getByText("Local MCP Client", { exact: true })).toBeVisible();
    await page.locator('button[value="deny"]').click();
    await page.waitForURL(`${callback}**`);
    expect(new URL(page.url()).searchParams.get("error")).toBe("access_denied");
    await page.goto(authorization.href);
    await expect(page.getByText("Local MCP Client", { exact: true })).toBeVisible();
    await page.locator('button[value="approve"]').click();
    await page.waitForURL(`${callback}**`);
    const redirect = new URL(page.url());
    expect(redirect.searchParams.get("state")).toBe("local-acceptance");
    expect(redirect.searchParams.get("iss")).toBe(stack.tAuthOrigin);
    const tokenResponse = await context.request.post(`${stack.tAuthOrigin}/oauth/token`, { form: { grant_type: "authorization_code", client_id: "local-mcp-client", code: redirect.searchParams.get("code") || "", redirect_uri: callback, resource: stack.llmProxyOrigin, code_verifier: verifier } });
    expect(tokenResponse.status()).toBe(200);
    const tokens = await tokenResponse.json();
    const accountResponse = await context.request.get(`${stack.llmProxyOrigin}/api/management/account`, { headers: { Origin: stack.frontendOrigin } });
    expect(accountResponse.status()).toBe(200);
    const account = await accountResponse.json();
    const tenantID = account.tenants[0].id;
    const provider = await context.request.put(`${stack.llmProxyOrigin}/api/management/tenants/${tenantID}/provider-connections/openai`, { headers: { Origin: stack.frontendOrigin }, data: { fields: { api_key: "local-mcp-provider-fixture" }, text_model: "gpt-4.1", system_prompt: "" } });
    expect(provider.status()).toBe(200);
    await officialClient({ endpoint: `${stack.llmProxyOrigin}/mcp`, token: tokens.access_token, tenant_id: tenantID });
    await inspectorClient(`${stack.llmProxyOrigin}/mcp`, tokens.access_token, tenantID);
    const refreshResponse = await context.request.post(`${stack.tAuthOrigin}/oauth/token`, { form: { grant_type: "refresh_token", refresh_token: tokens.refresh_token, client_id: "local-mcp-client", resource: stack.llmProxyOrigin } });
    expect(refreshResponse.status()).toBe(200);
    const refreshed = await refreshResponse.json();
    expect(refreshed.refresh_token !== tokens.refresh_token).toBe(true);
    await officialClient({ endpoint: `${stack.llmProxyOrigin}/mcp`, token: refreshed.access_token, tenant_id: tenantID });
    const revoke = await context.request.post(`${stack.tAuthOrigin}/oauth/revoke`, { form: { token: refreshed.refresh_token, token_type_hint: "refresh_token", client_id: "local-mcp-client" } });
    expect(revoke.status()).toBe(200);
    const rejectedRefresh = await context.request.post(`${stack.tAuthOrigin}/oauth/token`, { form: { grant_type: "refresh_token", refresh_token: refreshed.refresh_token, client_id: "local-mcp-client", resource: stack.llmProxyOrigin } });
    expect(rejectedRefresh.status()).toBe(400);
    // TAuth access tokens remain valid until expiry after grant revocation.
    await officialClient({ endpoint: `${stack.llmProxyOrigin}/mcp`, token: refreshed.access_token, tenant_id: tenantID });
  } finally {
    await stack.stop();
  }
});

/** @param {string} endpoint @param {string} token @param {string} tenantID */
async function inspectorClient(endpoint, token, tenantID) {
  const directory = await mkdtemp(path.join(os.tmpdir(), "llm-proxy-inspector-"));
  try {
    const config = path.join(directory, "servers.json");
    await writeFile(config, JSON.stringify({ mcpServers: { local: { type: "http", url: endpoint, protocolEra: "modern", headers: { Authorization: `Bearer ${token}` } } } }));
    for (const method of ["tools/list", "tools/call", "resources/read"]) {
      const args = ["--cli", "--config", config, "--server", "local", "--method", method, "--format", "json", "--stored-auth-only"];
      if (method === "tools/call") args.push("--tool-name", "llm_proxy.generate_text", "--tool-args-json", JSON.stringify({tenant_id: tenantID, messages: [{role: "user", content: "Inspector acceptance"}]}));
      if (method === "resources/read") args.push("--uri", `llm-proxy://tenants/${tenantID}/routes`);
      const child = spawn("node_modules/.bin/mcp-inspector", args, { env: { ...process.env, MCP_CLIENT_CONFIG_PATH: path.join(directory, "client.json"), MCP_OAUTH_STATE_FILE: path.join(directory, "oauth.json") }, stdio: ["ignore", "pipe", "pipe"] });
      let output = "";
      child.stdout.on("data", chunk => { output += chunk.toString(); });
      child.stderr.on("data", chunk => { output += chunk.toString(); });
      const status = await new Promise((resolve, reject) => { child.once("error", reject); child.once("exit", resolve); });
      expect(status, output.replaceAll(token, "[redacted]")).toBe(0);
      expect(output).toContain(method === "tools/call" ? "Local MCP answer" : method === "tools/list" ? "llm_proxy.list_tenants" : "routes");
    }
    const openCodeConfig = path.join(directory, "opencode.json");
    await writeFile(openCodeConfig, JSON.stringify({ mcp: { local: { type: "remote", url: endpoint, headers: { Authorization: `Bearer ${token}` }, oauth: false } } }));
    const architecture = process.arch === "x64" ? "x64" : process.arch;
    const binary = path.resolve(`node_modules/opencode-${process.platform}-${architecture}/bin/opencode`);
    const child = spawn(binary, ["mcp", "list"], { cwd: directory, env: { ...process.env, OPENCODE_CONFIG: openCodeConfig, XDG_CONFIG_HOME: path.join(directory, "config"), XDG_DATA_HOME: path.join(directory, "data"), XDG_CACHE_HOME: path.join(directory, "cache"), OPENCODE_DISABLE_AUTOUPDATE: "true", OPENCODE_DISABLE_MODELS_FETCH: "true", OPENCODE_DISABLE_DEFAULT_PLUGINS: "true" }, stdio: ["ignore", "pipe", "pipe"] });
    let output = "";
    child.stdout.on("data", chunk => { output += chunk.toString(); });
    child.stderr.on("data", chunk => { output += chunk.toString(); });
    const status = await new Promise((resolve, reject) => { child.once("error", reject); child.once("exit", resolve); });
    expect(status, output.replaceAll(token, "[redacted]")).toBe(0);
    // OpenCode 1.18.28 uses earlier MCP transports. Keep this qualification explicit.
    expect(stripVTControlCharacters(output).replaceAll(token, "[redacted]")).toContain("local failed");
    expect(stripVTControlCharacters(output).replaceAll(token, "[redacted]")).toContain("SSE error: Non-200 status code (405)");
  } finally { await rm(directory, {recursive: true, force: true}); }
}

/** @param {{endpoint: string, token: string, tenant_id: string}} input */
async function officialClient(input) {
  const child = spawn("go", ["run", "./tests/mcp-client"], { stdio: ["pipe", "pipe", "pipe"] });
  let output = "";
  child.stdout.on("data", (chunk) => { output += chunk.toString(); });
  child.stderr.on("data", (chunk) => { output += chunk.toString(); });
  child.stdin.end(JSON.stringify(input));
  const status = await new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("exit", resolve);
  });
  expect(status, output).toBe(0);
  expect(output).toContain("MCP OAuth discovery and generation passed");
}
