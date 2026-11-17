import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import process from "node:process";
import type { Usage } from "@typesafe-ai/sdk";

const MODEL = "jev-1.14.0";
const PROVIDER = "typesafe";
const CASE_NAMES = ["preflight", "text", "image"] as const;
const BOX_TOLERANCE = 0.1;
const FIXTURE = new URL("../tests/testdata/system-one-black-rectangle.png", import.meta.url);
const PROXY_REQUEST_ID_HEADER = "x-llm-proxy-request-id";

type CaseName = typeof CASE_NAMES[number];
type Metadata = { status?: number; requestID?: string; typesafeRequestID?: string };
let selectedCase: CaseName | undefined;
let tenantKey = "";
let phase: "configuration" | "request" = "configuration";
let sdk: typeof import("@typesafe-ai/sdk") | undefined;
let metadata: Metadata = {};

function safeProxyRequestID(value: string | null): string | undefined {
  return value && /^[A-Z2-7]{26,}$/.test(value) && !value.includes(tenantKey) ? value : undefined;
}

function safeTypeSafeRequestID(value: string | undefined): string | undefined {
  return value && /^[A-Za-z0-9._:-]{1,128}$/.test(value) && !value.includes(tenantKey)
    && !/^(?:apikey_|sk[-_])/i.test(value) ? value : undefined;
}

function captureMetadata(result: { response: Response; requestId: string | undefined }): void {
  metadata = {
    status: result.response.status,
    requestID: safeProxyRequestID(result.response.headers.get(PROXY_REQUEST_ID_HEADER)),
    typesafeRequestID: safeTypeSafeRequestID(result.requestId),
  };
  assert.equal(metadata.status, 200);
  assert(metadata.requestID);
  assert(metadata.typesafeRequestID);
}

function assertProbability(value: number): void {
  assert(Number.isFinite(value) && value >= 0 && value <= 1);
}

function assertUsage(usage: Usage): void {
  for (const value of [usage.input_tokens, usage.output_tokens]) {
    assert(value === null || (Number.isSafeInteger(value) && value >= 0));
  }
}

function reportSuccess(usage: Usage): void {
  assertUsage(usage);
  process.stdout.write(`live test passed: case=typesafe-${selectedCase} provider=${PROVIDER} status=200 request_id=${metadata.requestID} typesafe_request_id=${metadata.typesafeRequestID} model=${MODEL} input_tokens=${usage.input_tokens} output_tokens=${usage.output_tokens}\n`);
}

async function run(): Promise<void> {
  const argumentsList = process.argv.slice(2);
  assert.equal(argumentsList.length, 3);
  const [origin, rawTimeout, caseName] = argumentsList;
  assert(CASE_NAMES.includes(caseName as CaseName));
  selectedCase = caseName as CaseName;
  const endpoint = new URL(origin);
  assert(["https:", "http:"].includes(endpoint.protocol));
  assert.equal(endpoint.username + endpoint.password + endpoint.search + endpoint.hash, "");
  assert.equal(endpoint.pathname, "/");
  const timeoutSeconds = Number(rawTimeout);
  assert(Number.isSafeInteger(timeoutSeconds) && timeoutSeconds >= 1 && timeoutSeconds <= 3600);
  tenantKey = process.env.LLM_PROXY_DEFAULT_TENANT_KEY ?? "";
  assert(tenantKey && tenantKey.trim() === tenantKey);
  sdk = await import("@typesafe-ai/sdk");
  const png = await readFile(FIXTURE);
  assert(png.length > 24 && png.subarray(0, 8).equals(Buffer.from([137, 80, 78, 71, 13, 10, 26, 10])));
  assert.equal(png.toString("ascii", 12, 16), "IHDR");
  assert.equal(png.readUInt32BE(16), 512);
  assert.equal(png.readUInt32BE(20), 512);
  const picture = sdk.image(png);
  const client = new sdk.TypeSafeClient({
    baseURL: endpoint.origin,
    apiKey: tenantKey,
    defaultModel: MODEL,
    retry: { maxRetries: 0 },
    timeout: timeoutSeconds * 1000,
    logLevel: "off",
  });
  if (selectedCase === "preflight") {
    process.stdout.write("live test System One prerequisites passed\n");
    return;
  }
  phase = "request";
  if (selectedCase === "text") {
    const result = await client.systemOne({
      state: { color: "blue", severity: "high", message: "The request completed successfully." },
      questions: {
        color: sdk.choice("What is the color?", { blue: "Blue", red: "Red" }),
        severity: sdk.score("Rate the stated severity.", ["low", "high"]),
        success: sdk.noul("Did the request complete successfully?"),
      },
    }).withResponse();
    captureMetadata(result);
    assert.equal(result.data.model, MODEL);
    const answers = result.data.answers;
    assert.deepEqual(Object.keys(answers).sort(), ["color", "severity", "success"]);
    assert.equal(answers.color.type, "choice");
    assert.equal(answers.color.choice, "blue");
    assertProbability(answers.color.confidence);
    assertProbability(answers.color.probabilities.blue);
    assertProbability(answers.color.probabilities.red);
    assert.equal(answers.severity.type, "score");
    assertProbability(answers.severity.score);
    assert(answers.severity.score >= 0.5);
    assertProbability(answers.severity.confidence);
    assert.equal(answers.severity.legend["0"], "low");
    assert.equal(answers.severity.legend["1"], "high");
    assertProbability(answers.severity.probabilities["0"]);
    assertProbability(answers.severity.probabilities["1"]);
    assert.equal(answers.success.type, "noul");
    assertProbability(answers.success.noul);
    assert(answers.success.noul > 0.5);
    reportSuccess(result.data.usage);
  } else {
    const result = await client.systemOne({
      state: { picture },
      questions: { rectangle: sdk.box("Locate the single black rectangle on the white background.") },
    }, { headers: { "TypeSafe-Danger-Zone-Images": "true" } }).withResponse();
    captureMetadata(result);
    assert.equal(result.data.model, MODEL);
    assert.deepEqual(Object.keys(result.data.answers), ["rectangle"]);
    const answer = result.data.answers.rectangle;
    assert.equal(answer.type, "box");
    for (const [edge, expected] of Object.entries({ top: 0.25, left: 0.25, bottom: 0.75, right: 0.75 })) {
      const actual = answer.box[edge as keyof typeof answer.box];
      assertProbability(actual);
      assert(Math.abs(actual - expected) <= BOX_TOLERANCE);
    }
    assert(answer.box.top < answer.box.bottom && answer.box.left < answer.box.right);
    if (answer.confidence !== null) assertProbability(answer.confidence);
    reportSuccess(result.data.usage);
  }
}

run().catch((error: unknown) => {
  let failure = phase === "configuration" ? "invalid_configuration" : "invalid_response";
  if (sdk && error instanceof sdk.APIError) {
    failure = "http_error";
    metadata = {
      status: error.status,
      requestID: safeProxyRequestID(error.headers.get(PROXY_REQUEST_ID_HEADER)),
      typesafeRequestID: safeTypeSafeRequestID(error.requestId),
    };
  } else if (sdk && error instanceof sdk.APITimeoutError) {
    failure = "timeout";
  } else if (sdk && error instanceof sdk.APIConnectionError) {
    failure = "transport_error";
  }
  const fields = [`case=typesafe-${selectedCase ?? "configuration"}`, `provider=${PROVIDER}`, `failure=${failure}`];
  if (metadata.status !== undefined) fields.push(`status=${metadata.status}`);
  if (metadata.requestID) fields.push(`request_id=${metadata.requestID}`);
  if (metadata.typesafeRequestID) fields.push(`typesafe_request_id=${metadata.typesafeRequestID}`);
  process.stderr.write(`live test failed: ${fields.join(" ")}\n`);
  process.exitCode = 1;
});
