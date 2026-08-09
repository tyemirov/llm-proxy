// @ts-check

import { expect, test } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const executeFile = promisify(execFile);
const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const publicCapabilitiesPath = "/api/public/capabilities";

test("public site rendering rejects a missing capability REST resource", async () => {
  await withCapabilityServer(404, { error: "missing" }, async (capabilitiesURL) => {
    const fixture = await siteFixture();
    try {
      await expect(renderFixture(fixture, capabilitiesURL)).rejects.toThrow(/public_capabilities_request_failed: status=404/u);
    } finally {
      await rm(fixture.root, { recursive: true, force: true });
    }
  });
});

test("public site rendering rejects an invalid capability REST representation", async () => {
  await withCapabilityServer(200, {
    providers: [],
    max_prompt_bytes: 4194304,
    max_input_audio_bytes: 26214400,
    max_request_timeout_seconds: 3600,
  }, async (capabilitiesURL) => {
    const fixture = await siteFixture();
    try {
      await expect(renderFixture(fixture, capabilitiesURL)).rejects.toThrow(/catalog\.providers must not be empty/u);
    } finally {
      await rm(fixture.root, { recursive: true, force: true });
    }
  });
});

/**
 * @param {{root: string, source: string, output: string}} fixture
 * @param {string} capabilitiesURL
 */
function renderFixture(fixture, capabilitiesURL) {
  return executeFile("node", [
    "scripts/render_public_site.mjs",
    "--source",
    fixture.source,
    "--output",
    fixture.output,
    "--config-url",
    "/config-ui.yaml",
    "--capabilities-url",
    capabilitiesURL,
  ], { cwd: repositoryRoot });
}

async function siteFixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), "llm-proxy-renderer-"));
  const source = path.join(root, "source");
  await mkdir(path.join(source, "app"), { recursive: true });
  await writeFile(path.join(source, "CNAME"), "llm-proxy.example\n", "utf8");
  await writeFile(
    path.join(source, "index.html"),
    '<mpr-header data-config-url="/config-ui.yaml"></mpr-header><!-- llm-proxy-routing-tree --><!-- llm-proxy-capability-catalog -->',
    "utf8",
  );
  await writeFile(
    path.join(source, "app/index.html"),
    '<mpr-header data-config-url="/config-ui.yaml"></mpr-header>',
    "utf8",
  );
  return { root, source, output: path.join(root, "output") };
}

/**
 * @param {number} statusCode
 * @param {unknown} responseBody
 * @param {(capabilitiesURL: string) => Promise<void>} assertion
 */
async function withCapabilityServer(statusCode, responseBody, assertion) {
  const server = http.createServer((request, response) => {
    if (request.url !== publicCapabilitiesPath) {
      response.writeHead(404).end();
      return;
    }
    response.writeHead(statusCode, { "Content-Type": "application/json" });
    response.end(JSON.stringify(responseBody));
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || typeof address === "string") {
    throw new Error("public_capability_fixture_address_missing");
  }
  try {
    await assertion(`http://127.0.0.1:${address.port}${publicCapabilitiesPath}`);
  } finally {
    await new Promise((resolve, reject) => {
      server.close((closeError) => closeError ? reject(closeError) : resolve(undefined));
    });
  }
}
