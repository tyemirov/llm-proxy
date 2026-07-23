import { spawn } from "node:child_process";
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
  operatorPassword,
});

export async function startLocalManagementStack() {
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

    const tAuthConfigPath = path.join(temporaryDirectory, "tauth-config.yaml");
    await writeFile(tAuthConfigPath, tAuthConfig(tAuthPort, frontendOrigin), { mode: 0o600 });

    const packagedLLMProxyConfig = await readFile(path.join(repoRoot, "configs/config.yml"), "utf8");
    let llmProxyConfig = packagedLLMProxyConfig.replace("  port: 8080\n", `  port: ${llmProxyPort}\n`);
    if (llmProxyConfig === packagedLLMProxyConfig) {
      throw new Error("llm_proxy_blackbox_port_contract_missing");
    }
    const legacyMigrationBlock = [
      "  legacy_token_migration:",
      "    tenant_id: default",
      '    owner_email: "${LLM_PROXY_MANAGEMENT_LEGACY_TOKEN_OWNER_EMAIL}"',
      "",
    ].join("\n");
    const configWithoutLegacyMigration = llmProxyConfig.replace(legacyMigrationBlock, "");
    if (configWithoutLegacyMigration === llmProxyConfig) {
      throw new Error("llm_proxy_blackbox_legacy_migration_contract_missing");
    }
    llmProxyConfig = configWithoutLegacyMigration;
    const llmProxyConfigPath = path.join(temporaryDirectory, "llm-proxy-config.yml");
    await writeFile(llmProxyConfigPath, llmProxyConfig, { mode: 0o600 });

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
      llmProxyEnvironment(frontendOrigin, tAuthOrigin, llmProxyOrigin, temporaryDirectory),
    );
    serviceProcesses.push(llmProxyProcess);
    await waitForHTTP(`${llmProxyOrigin}/config-ui.yaml`, {}, llmProxyProcess, 200);

    return {
      frontendOrigin,
      tAuthOrigin,
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
  const server = http.createServer((request, response) => {
    void handleFrontendRequest(request, response, managementAPIOrigin);
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
  };
}

async function handleFrontendRequest(request, response, managementAPIOrigin) {
  try {
    const requestURL = new URL(request.url || "/", "http://localhost");
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

function tAuthConfig(port, frontendOrigin) {
  return `server:
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
    jwt_signing_key: "${jwtSigningKey}"
    cookie_domain: ""
    session_cookie_name: "${sessionCookieName}"
    refresh_cookie_name: "${refreshCookieName}"
    session_ttl: "30m"
    refresh_ttl: "720h"
    nonce_ttl: "5m"
    allow_insecure_http: true
`;
}

function llmProxyEnvironment(frontendOrigin, tAuthOrigin, llmProxyOrigin, temporaryDirectory) {
  return {
    ...process.env,
    LLM_PROXY_MANAGEMENT_ENABLED: "true",
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
    LLM_PROXY_MANAGEMENT_DATABASE_DIALECT: "sqlite",
    LLM_PROXY_MANAGEMENT_DATABASE_DSN: path.join(temporaryDirectory, "management.sqlite"),
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
