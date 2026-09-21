// @ts-check
import * as yaml from "js-yaml";
import { spawn } from "node:child_process";
import { generateKeyPairSync } from "node:crypto";
import { createReadStream } from "node:fs";
import { mkdtemp, readFile, rename, rm, stat, writeFile } from "node:fs/promises";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const siteRoot = path.join(repoRoot, "site");
const tAuthModulePath = "github.com/tyemirov/tauth";
const tAuthModulePackage = `${tAuthModulePath}/cmd/server`;
const localHost = "localhost";
const sessionCookieName = "app_session_llm_proxy";
const refreshCookieName = "app_refresh_llm_proxy";
const tenantID = "llm-proxy";
const operatorEmail = "operator@example.com";
const secondOperatorEmail = "second-operator@example.com";
const operatorPassword = "llm-proxy-local-password";
const operatorPasswordHash = "$2y$10$1C96ZZ4ykZDQ6QXBoeDi8ONWnf2U7kf5eyY4P2Dm8ntJzBWIsdRS.";
const jwtSigningKey = "llm-proxy-local-blackbox-signing-key-2026-07-13";
const providerKeyEncryptionKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=";
const startupTimeoutMilliseconds = 30000;
const processExitTimeoutMilliseconds = 5000;

const mimeTypes = Object.freeze({
  ".css": "text/css",
  ".html": "text/html",
  ".js": "application/javascript",
  ".json": "application/json",
  ".svg": "image/svg+xml",
  ".txt": "text/plain",
  ".xml": "application/xml",
  ".yaml": "application/yaml",
});

export const localManagementProfile = Object.freeze({
  tenantID,
  sessionCookieName,
  refreshCookieName,
  operatorEmail,
  secondOperatorEmail,
  operatorPassword,
});

export async function startLocalManagementStack(authRouting = "frontend") {
  if (!["frontend", "direct"].includes(authRouting)) throw new Error(`auth_routing_invalid:${authRouting}`);
  const temporaryDirectory = await mkdtemp(path.join(os.tmpdir(), "llm-proxy-management-blackbox-"));
  const tAuthBinaryPath = path.join(temporaryDirectory, "tauth");
  const llmProxyBinaryPath = path.join(temporaryDirectory, "llm-proxy");
  const serviceProcesses = [];
  let frontendServer;

  try {
    await buildTAuthBinary(tAuthBinaryPath, temporaryDirectory);
    await buildBinary(llmProxyBinaryPath, "./cmd/cli");

    const frontend = await startFrontendServer();
    frontendServer = frontend.server;
    const tAuthPort = await reserveLocalPort();
    const llmProxyPort = await reserveLocalPort();
    const frontendOrigin = `http://${localHost}:${frontend.port}`;
    const tAuthOrigin = `http://${localHost}:${tAuthPort}`;
    const llmProxyOrigin = `http://${localHost}:${llmProxyPort}`;
    frontend.setManagementAPIOrigin(llmProxyOrigin);
    frontend.setTAuthOrigin(tAuthOrigin);

    const tAuthConfigPath = path.join(temporaryDirectory, "tauth-config.yaml");
    const oauthKey = Buffer.from(generateKeyPairSync("ec", { namedCurve: "prime256v1" }).privateKey.export({ type: "pkcs8", format: "pem" })).toString("base64");
    await writeFile(tAuthConfigPath, await tAuthConfig(tAuthPort, frontendOrigin, llmProxyOrigin, oauthKey), { mode: 0o600 });

    const packagedLLMProxyConfig = await readFile(path.join(repoRoot, "configs/config.yml"), "utf8");
    let llmProxyConfig = packagedLLMProxyConfig.replace("  port: 8080\n", `  port: ${llmProxyPort}\n`);
    if (llmProxyConfig === packagedLLMProxyConfig) {
      throw new Error("llm_proxy_blackbox_port_contract_missing");
    }
    const capacityConfig = yaml.load(llmProxyConfig);
    capacityConfig.server.upstream_capacity.origins = capacityConfig.server.upstream_capacity.origins.filter(rule => rule.origin !== "https://api.fal.ai");
    capacityConfig.server.upstream_capacity.origins.push({origin: frontendOrigin, active: 4, queued: 24});
    llmProxyConfig = yaml.dump(capacityConfig);
    const packagedProviderCatalog = await readFile(path.join(repoRoot, "configs/providers.yml"), "utf8");
    const providerCatalog = packagedProviderCatalog.replace(
      "            default_base_url: https://api.openai.com/v1\n            path: /responses\n",
      `            default_base_url: ${frontendOrigin}/v1\n            path: /responses\n`,
    );
    if (providerCatalog === packagedProviderCatalog) {
      throw new Error("llm_proxy_blackbox_openai_transport_contract_missing");
    }
    const speechCatalog = yaml.load(providerCatalog);
    speechCatalog.providers.find(provider => provider.id === "fal").transports.find(transport => transport.id === "account").endpoint.default_base_url = frontendOrigin;
    speechCatalog.providers.find(provider => provider.id === "elevenlabs").transports.find(transport => transport.id === "subscription").endpoint.default_base_url = frontendOrigin;
    const speechProvider = structuredClone(speechCatalog.providers.find(provider => provider.id === "dictator"));
    const speechModel = structuredClone(speechCatalog.models.find(model => model.id === "whisper-base"));
    speechProvider.id = "speech-fixture";
    speechProvider.label = "Second speech provider";
    speechProvider.api_service_label = "Second speech service";
    speechModel.id = "speech-fixture-v1";
    for (const field of speechProvider.fields) {
      if (field.id === "grpc_address") { field.id = "speech_endpoint"; field.label = "Speech endpoint"; }
      if (field.id === "grpc_auth_token") { field.id = "speech_bearer"; field.label = "Speech bearer credential"; }
    }
    speechProvider.transports[0].endpoint.setting_field = "speech_endpoint";
    speechProvider.transports[0].components.authentication.field = "speech_bearer";
    speechProvider.offerings[0].model = speechModel.id;
    speechProvider.offerings[0].upstream_model = "private-speech-fixture";
    speechCatalog.models.push(speechModel);
    speechCatalog.providers.push(speechProvider);
    const resourceProvider = structuredClone(speechProvider);
    resourceProvider.id = "resource-fixture";
    resourceProvider.label = "Resource fixture";
    resourceProvider.offerings = [];
    speechCatalog.providers.push(resourceProvider);
    const mediaProvider = structuredClone(speechCatalog.providers.find(provider => provider.id === "xai"));
    mediaProvider.id = "media-fixture";
    mediaProvider.label = "Media fixture";
    mediaProvider.api_service_label = "Media fixture API";
    mediaProvider.aliases = [];
    mediaProvider.fields = [mediaProvider.fields[0]];
    mediaProvider.fields[0].id = "media_token";
    mediaProvider.fields[0].label = "Media account token";
    delete mediaProvider.fields[0].environment;
    mediaProvider.offerings = mediaProvider.offerings.filter(offering => offering.operations.includes("video_generation"));
    mediaProvider.transports = mediaProvider.transports.filter(transport => transport.id === mediaProvider.offerings[0].transport);
    mediaProvider.transports[0].components.authentication.field = "media_token";
    mediaProvider.transports.push({
      id: "account",
      endpoint: {protocol: "http", method: "GET", default_base_url: frontendOrigin, path: "/provider-account"},
      components: {
        request_codec: {id: "json_resource"}, response_codec: {id: "json_resource"},
        authentication: {kind: "header", field: "media_token", header: "X-Media-Token", prefix: ""},
        execution: {id: "read_only"},
      },
    });
    mediaProvider.verification = {transport: "account"};
    speechCatalog.providers.push(mediaProvider);
    const llmProxyConfigPath = path.join(temporaryDirectory, "llm-proxy-config.yml");
    await writeFile(llmProxyConfigPath, llmProxyConfig, { mode: 0o600 });
    await writeFile(path.join(temporaryDirectory, "providers.yml"), yaml.dump(speechCatalog), { mode: 0o600 });

    const tAuthProcess = startService("tauth", tAuthBinaryPath, ["--config", tAuthConfigPath]);
    serviceProcesses.push(tAuthProcess);
    await waitForHTTP(`${tAuthOrigin}/auth/session`, {
      Origin: frontendOrigin,
      "X-Requested-With": "XMLHttpRequest",
      "X-TAuth-Tenant": tenantID,
    }, tAuthProcess);

    const llmProxyProcess = startService(
      "llm-proxy",
      llmProxyBinaryPath,
      ["--config", llmProxyConfigPath],
      llmProxyEnvironment(frontendOrigin, authRouting === "frontend" ? frontendOrigin : tAuthOrigin, llmProxyOrigin, temporaryDirectory),
    );
    serviceProcesses.push(llmProxyProcess);
    await waitForHTTP(`${llmProxyOrigin}/config-ui.yaml`, {}, llmProxyProcess, 200);

    return {
      frontendOrigin,
      tAuthOrigin: authRouting === "frontend" ? frontendOrigin : tAuthOrigin,
      llmProxyOrigin,
      async stop() {
        await stopStack(frontendServer, serviceProcesses, temporaryDirectory);
      },
    };
  } catch (error) {
    await stopStack(frontendServer, serviceProcesses, temporaryDirectory);
    throw error;
  }
}

async function buildBinary(outputPath, packagePath) {
  await runCommand("go", ["build", "-o", outputPath, packagePath], repoRoot);
}

async function buildTAuthBinary(outputPath, temporaryDirectory) {
  const tAuthVersion = (
    await runCommand("go", ["list", "-m", "-f", "{{.Version}}", tAuthModulePath], repoRoot)
  ).trim();
  if (!/^v\d+\.\d+\.\d+$/.test(tAuthVersion)) {
    throw new Error(`tauth_module_version_invalid: ${tAuthVersion}`);
  }
  const installDirectory = path.join(temporaryDirectory, "tauth-install");
  await runCommand(
    "go",
    ["install", `${tAuthModulePackage}@${tAuthVersion}`],
    repoRoot,
    { ...process.env, GOBIN: installDirectory },
  );
  await rename(path.join(installDirectory, "server"), outputPath);
}

async function runCommand(command, argumentsList, workingDirectory, environment = process.env) {
  const childProcess = spawn(command, argumentsList, {
    cwd: workingDirectory,
    env: environment,
    stdio: ["ignore", "pipe", "pipe"],
  });
  const output = collectOutput(childProcess);
  const exitCode = await new Promise((resolve, reject) => {
    childProcess.once("error", reject);
    childProcess.once("exit", resolve);
  });
  if (exitCode !== 0) {
    throw new Error(`${command}_failed: exit=${exitCode}\n${output.value()}`);
  }
  return output.value();
}

function startService(name, binaryPath, argumentsList, environment = process.env) {
  const childProcess = spawn(binaryPath, argumentsList, {
    cwd: repoRoot,
    env: environment,
    stdio: ["ignore", "pipe", "pipe"],
  });
  const output = collectOutput(childProcess);
  childProcess.once("error", (error) => {
    output.append(`${name}_spawn_error: ${error.message}`);
  });
  return { name, childProcess, output };
}

function collectOutput(childProcess) {
  let bufferedOutput = "";
  const append = (chunk) => {
    bufferedOutput = `${bufferedOutput}${String(chunk)}`.slice(-20000);
  };
  childProcess.stdout.on("data", append);
  childProcess.stderr.on("data", append);
  return {
    append,
    value: () => bufferedOutput,
  };
}

async function waitForHTTP(url, headers, serviceProcess, expectedStatus) {
  const deadline = Date.now() + startupTimeoutMilliseconds;
  let lastError = "service_not_ready";
  while (Date.now() < deadline) {
    if (serviceProcess.childProcess.exitCode !== null) {
      throw new Error(
        `${serviceProcess.name}_exited_before_ready: exit=${serviceProcess.childProcess.exitCode}\n${serviceProcess.output.value()}`,
      );
    }
    try {
      const response = await fetch(url, { headers });
      if (expectedStatus === undefined || response.status === expectedStatus) {
        return;
      }
      lastError = `status=${response.status}`;
    } catch (error) {
      lastError = error instanceof Error ? error.message : String(error);
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error(`${serviceProcess.name}_startup_timeout: ${lastError}\n${serviceProcess.output.value()}`);
}

async function reserveLocalPort() {
  const server = http.createServer();
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, resolve);
  });
  const address = server.address();
  if (!address || typeof address === "string") {
    throw new Error("local_port_reservation_failed");
  }
  const { port } = address;
  await closeHTTPServer(server);
  return port;
}

async function startFrontendServer() {
  let managementAPIOrigin = "";
  let tAuthOrigin = "";
  const server = http.createServer((request, response) => {
    void handleFrontendRequest(request, response, managementAPIOrigin, tAuthOrigin);
  });
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, resolve);
  });
  const address = server.address();
  if (!address || typeof address === "string") {
    throw new Error("frontend_server_address_missing");
  }
  return {
    server,
    port: address.port,
    setManagementAPIOrigin(origin) {
      managementAPIOrigin = origin;
    },
    setTAuthOrigin(origin) {
      tAuthOrigin = origin;
    },
  };
}

async function handleFrontendRequest(request, response, managementAPIOrigin, tAuthOrigin) {
  try {
    const requestURL = new URL(request.url || "/", "http://localhost");
    if (requestURL.pathname === "/assets/llm-proxy/js/brandIconManifest.js") {
      const manifest = await readFile(path.join(siteRoot, "assets/llm-proxy/js/brandIconManifest.js"), "utf8");
      response.writeHead(200, {"content-type": "application/javascript"});
      response.end(`${manifest}\nbrandIconManifest.providers["speech-fixture"] = null;\nbrandIconManifest.providers["media-fixture"] = null;\nbrandIconManifest.providers["resource-fixture"] = null;\n`);
      return;
    }
    if (requestURL.pathname === "/config-ui.yaml") {
      if (!managementAPIOrigin) {
        throw new Error("management_api_origin_not_ready");
      }
      const upstreamResponse = await fetch(`${managementAPIOrigin}/config-ui.yaml`);
      response.writeHead(upstreamResponse.status, {
        "content-type": upstreamResponse.headers.get("content-type") || mimeTypes[".yaml"],
      });
      response.end(Buffer.from(await upstreamResponse.arrayBuffer()));
      return;
    }
    if (requestURL.pathname === "/auth" || requestURL.pathname.startsWith("/auth/") || requestURL.pathname === "/me" || requestURL.pathname.startsWith("/oauth/") || requestURL.pathname.startsWith("/.well-known/")) {
      if (!tAuthOrigin) {
        throw new Error("tauth_origin_not_ready");
      }
      await proxyFrontendRequest(request, response, tAuthOrigin);
      return;
    }
    if (requestURL.pathname === "/v1/models/pricing") {
      response.writeHead(request.headers.authorization === "Key local-fal-key" ? 200 : 401, {"Content-Type": "application/json"});
      response.end(JSON.stringify({prices: []}));
      return;
    }
    if (requestURL.pathname === "/v1/user/subscription") {
      const accepted = request.method === "GET" && request.headers["xi-api-key"] === "local-eleven-key";
      response.writeHead(accepted ? 200 : 401, {"Content-Type": "application/json"});
      response.end(JSON.stringify({tier: "creator", status: "active"}));
      return;
    }
    if (requestURL.pathname === "/provider-account") {
      const accepted = request.method === "GET" && request.headers["x-media-token"] === "browser-media-token";
      response.writeHead(accepted ? 200 : 401, { "content-type": mimeTypes[".json"] });
      response.end(accepted ? '{"subscription":"active"}' : '{}');
      return;
    }
    if (request.method === "POST" && requestURL.pathname === "/v1/responses") {
      response.writeHead(200, { "content-type": mimeTypes[".json"] });
      response.end(JSON.stringify({ id: "resp_local_provider_key_verification", status: "completed", output_text: "Local MCP answer", usage: { input_tokens: 2, output_tokens: 3, total_tokens: 5 } }));
      return;
    }

    const requestedPath = decodeURIComponent(requestURL.pathname);
    const relativePath = requestedPath === "/" ? "index.html" : requestedPath.slice(1);
    let filePath = path.resolve(siteRoot, relativePath);
    if (filePath !== siteRoot && !filePath.startsWith(`${siteRoot}${path.sep}`)) {
      response.writeHead(403);
      response.end("Forbidden");
      return;
    }
    const fileStats = await stat(filePath);
    if (fileStats.isDirectory()) {
      filePath = path.join(filePath, "index.html");
    }
    const extension = path.extname(filePath);
    response.writeHead(200, { "content-type": mimeTypes[extension] || "application/octet-stream" });
    createReadStream(filePath).pipe(response);
  } catch (error) {
    const statusCode = error && typeof error === "object" && "code" in error && error.code === "ENOENT" ? 404 : 502;
    response.writeHead(statusCode);
    response.end(statusCode === 404 ? "Not Found" : "Bad Gateway");
  }
}

async function proxyFrontendRequest(request, response, upstreamOrigin) {
  const upstreamURL = new URL(request.url || "/", upstreamOrigin);
  await new Promise((resolve, reject) => {
    const upstreamRequest = http.request(upstreamURL, {
      method: request.method,
      headers: {
        ...request.headers,
        host: upstreamURL.host,
      },
    }, (upstreamResponse) => {
      response.writeHead(upstreamResponse.statusCode || 502, upstreamResponse.headers);
      upstreamResponse.pipe(response);
      upstreamResponse.once("end", resolve);
    });
    upstreamRequest.once("error", reject);
    request.pipe(upstreamRequest);
  });
}

async function tAuthConfig(port, frontendOrigin, llmProxyOrigin, oauthKey) {
  const config = yaml.load(`server:
  listen_addr: ":${port}"
  database_url: ""
  enable_cors: true
  cors_allowed_origins:
    - "${frontendOrigin}"
  cors_allowed_origin_exceptions: []
  enable_tenant_header_override: true

tenants:
  - id: "${tenantID}"
    display_name: "LLM Proxy"
    tenant_origins:
      - "${frontendOrigin}"
    password_auth:
      enabled: true
      users:
        - email: "${operatorEmail}"
          display_name: "Local Operator"
          avatar_url: "${frontendOrigin}/assets/llm-proxy/img/llm-proxy-icon.svg"
          password_hash: "${operatorPasswordHash}"
        - email: "${secondOperatorEmail}"
          display_name: "Second Local Operator"
          avatar_url: "${frontendOrigin}/assets/llm-proxy/img/llm-proxy-icon.svg"
          password_hash: "${operatorPasswordHash}"
    jwt_signing_key: "${jwtSigningKey}"
    cookie_domain: ""
    session_cookie_name: "${sessionCookieName}"
    refresh_cookie_name: "${refreshCookieName}"
    session_ttl: "30m"
    refresh_ttl: "720h"
    nonce_ttl: "5m"
    allow_insecure_http: true
`);
  const template = await readFile(path.join(repoRoot, "configs/tauth.local.yml"), "utf8");
  const replacements = { LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN: frontendOrigin, LLM_PROXY_MANAGEMENT_PROXY_ORIGIN: llmProxyOrigin, TAUTH_OAUTH_ES256_PRIVATE_KEY_BASE64: oauthKey };
  const local = yaml.load(template.replace(/\$\{([^}]+)\}/g, (match, key) => replacements[key] ?? match));
  config.oauth = local.oauth;
  config.tenants[0].oauth = local.tenants[0].oauth;
  config.tenants[0].oauth.clients = [{ id: "local-mcp-client", display_name: "Local MCP Client", application_type: "native", redirect_uris: [`${frontendOrigin}/oauth/callback`], grants: [{resource: llmProxyOrigin, scopes: ["llm-proxy:use"]}] }];
  return yaml.dump(config);
}

function llmProxyEnvironment(frontendOrigin, tAuthOrigin, llmProxyOrigin, temporaryDirectory) {
  return {
    ...process.env,
    LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN: frontendOrigin,
    LLM_PROXY_MANAGEMENT_LOOPBACK_ORIGIN: frontendOrigin,
    LLM_PROXY_MANAGEMENT_LOCALHOST_ORIGIN: frontendOrigin,
    LLM_PROXY_MANAGEMENT_UI_DESCRIPTION: "Local black-box",
    LLM_PROXY_MANAGEMENT_ADMIN_EMAILS: "[]",
    LLM_PROXY_MANAGEMENT_TAUTH_URL: tAuthOrigin,
    LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID: tenantID,
    LLM_PROXY_MANAGEMENT_GOOGLE_CLIENT_ID: "local-blackbox.apps.googleusercontent.com",
    LLM_PROXY_MANAGEMENT_TAUTH_LOGIN_PATH: "/auth/google",
    LLM_PROXY_MANAGEMENT_TAUTH_LOGOUT_PATH: "/auth/logout",
    LLM_PROXY_MANAGEMENT_TAUTH_NONCE_PATH: "/auth/nonce",
    LLM_PROXY_MANAGEMENT_TAUTH_SESSION_PATH: "/auth/session",
    LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY: jwtSigningKey,
    LLM_PROXY_MANAGEMENT_JWT_ISSUER: "tauth",
    LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME: sessionCookieName,
    LLM_PROXY_MANAGEMENT_DATABASE_PATH: path.join(temporaryDirectory, "management.sqlite"),
    LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY: providerKeyEncryptionKey,
    LLM_PROXY_MANAGEMENT_API_ORIGIN: llmProxyOrigin,
    LLM_PROXY_MANAGEMENT_PROXY_ORIGIN: llmProxyOrigin,
  };
}

async function stopStack(frontendServer, serviceProcesses, temporaryDirectory) {
  if (frontendServer) {
    await closeHTTPServer(frontendServer);
  }
  for (const serviceProcess of [...serviceProcesses].reverse()) {
    await stopService(serviceProcess);
  }
  await rm(temporaryDirectory, { force: true, recursive: true });
}

async function stopService(serviceProcess) {
  const { childProcess } = serviceProcess;
  if (childProcess.exitCode !== null) {
    return;
  }
  childProcess.kill("SIGTERM");
  const exited = await Promise.race([
    new Promise((resolve) => childProcess.once("exit", () => resolve(true))),
    new Promise((resolve) => setTimeout(() => resolve(false), processExitTimeoutMilliseconds)),
  ]);
  if (!exited && childProcess.exitCode === null) {
    childProcess.kill("SIGKILL");
    await new Promise((resolve) => childProcess.once("exit", resolve));
  }
}

async function closeHTTPServer(server) {
  if (!server.listening) {
    return;
  }
  await new Promise((resolve, reject) => {
    server.close((error) => {
      if (error) {
        reject(error);
        return;
      }
      resolve();
    });
  });
}
