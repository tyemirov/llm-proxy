// @ts-check

import { expect, test } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
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
  const capabilities = normalizedCapabilityFixture();
  capabilities.providers = [];
  await withCapabilityServer(200, capabilities, async (capabilitiesURL) => {
    const fixture = await siteFixture();
    try {
      await expect(renderFixture(fixture, capabilitiesURL)).rejects.toThrow(/catalog\.providers must not be empty/u);
    } finally {
      await rm(fixture.root, { recursive: true, force: true });
    }
  });
});

test("public site rendering writes the normalized exact model catalog", async () => {
  await withCapabilityServer(200, normalizedCapabilityFixture(), async (capabilitiesURL) => {
    const fixture = await siteFixture();
    try {
      await renderFixture(fixture, capabilitiesURL);
      const renderedLanding = await readFile(path.join(fixture.output, "index.html"), "utf8");
      expect(renderedLanding).toContain("1 publisher · 1 exact model · 1 offering");
      expect(renderedLanding).toContain('data-route-publisher="example-publisher"');
      expect(renderedLanding).toContain('data-route-model="example-model"');
      expect(renderedLanding).toContain('data-route-provider="example-provider"');
      expect(renderedLanding).toContain('<strong>1</strong><span>Exact models</span>');
      expect(renderedLanding).not.toContain("provider_model");
      expect(renderedLanding).not.toContain("default_operations");
    } finally {
      await rm(fixture.root, { recursive: true, force: true });
    }
  });
});

function normalizedCapabilityFixture() {
  return {
    revision: "2026-08-10.test.1",
    operations: [
      { id: "text", input_artifacts: ["text"], output_artifacts: ["text"] },
      { id: "dictation", input_artifacts: ["audio"], output_artifacts: ["text"] },
      { id: "video_generation", input_artifacts: ["text", "image"], output_artifacts: ["video"] },
    ],
    providers: [{ identifier: "example-provider", label: "Example Provider", credential_kinds: ["api_key"] }],
    publishers: [{ identifier: "example-publisher", label: "Example Publisher", model_count: 1 }],
    families: [{ identifier: "example-family", publisher: "example-publisher", label: "Example Family" }],
    models: [{
      identifier: "example-model",
      publisher: "example-publisher",
      family: "example-family",
      version: "1",
      operations: ["text"],
      media_inputs: [],
      capabilities: ["text"],
      provider_offerings: ["example-provider:example-model"],
    }],
    offerings: [{
      identifier: "example-provider:example-model",
      provider: "example-provider",
      model: "example-model",
      capabilities: ["text"],
      wire_contract: "openai_chat_completions",
      execution_lifecycle: "synchronous_completion",
      output_token_limit: 0,
      reasoning_efforts: [],
      controls: [],
      limits: [],
      media_limits: [],
    }],
    prices: [{
      provider: "example-provider",
      model: "example-model",
      operation: "text",
      available: false,
      rates: [],
      minimum_charge: null,
      source: "https://example.com/pricing",
      last_verified: "2026-08-10",
      unavailable_reason: "Test price is unavailable.",
    }],
    counts: {
      providers: 1,
      model_publishers: 1,
      model_families: 1,
      exact_models: 1,
      provider_offerings: 1,
    },
    max_prompt_bytes: 4194304,
    max_input_audio_bytes: 26214400,
    max_request_timeout_seconds: 3600,
  };
}

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
