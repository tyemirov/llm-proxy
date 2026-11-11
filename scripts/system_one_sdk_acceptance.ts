import { APIError, APITimeoutError, APIUserAbortError, box, choice, image, noul, score, TypeSafeClient } from "@typesafe-ai/sdk";
import { Buffer } from "node:buffer";
import process from "node:process";

const [baseURL, apiKey, fixtureControlURL] = process.argv.slice(2);
if (!baseURL || !apiKey || !fixtureControlURL) {
  throw new Error("system_one_sdk_acceptance_requires_base_url_and_test_key");
}
const client = new TypeSafeClient({
  baseURL,
  apiKey,
  defaultModel: "jev-latest",
  retry: { maxRetries: 0 },
  logLevel: "off",
});
const { data: first, requestId } = await client.systemOne({
  state: { message: "sample ".repeat(400), context: null },
  questions: {
    routing: choice("Choose a route", { a: null, b: "Other" }),
    severity: score("Rate the severity", ["Low", "High"]),
    accepted: noul("The request is accepted"),
  },
}).withResponse();
if (requestId !== "typesafe-sdk-request") {
  throw new Error("system_one_sdk_request_id_changed");
}
if (first.answers.routing.choice !== "a" || first.answers.severity.score !== 0.25 || first.answers.accepted.noul !== 0.8) {
  throw new Error("system_one_sdk_typed_answers_changed");
}
const photo = image(Buffer.from("iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAIAAACQkWg2AAAAHElEQVR4nGP4TyJgoKcGBgbsaFQDfTUQB4aDBgDAen2fpitsVAAAAABJRU5ErkJggg==", "base64"));
const second = await client.systemOne({
  model: "jev-1.14.0",
  state: { photo },
  questions: { region: box("Locate the rectangle") },
}, { headers: { "TypeSafe-Danger-Zone-Images": "true" } });
if (second.answers.region.box.top !== 0 || second.answers.region.confidence !== null || second.usage.input_tokens !== null) {
  throw new Error("system_one_sdk_image_or_null_usage_changed");
}
try {
  await client.systemOne({ state: { scenario: "error" }, questions: { check: noul("Is this true?") } });
  throw new Error("system_one_sdk_provider_error_was_not_raised");
} catch (error) {
  if (!(error instanceof APIError) || error.status !== 429 || error.requestId !== "typesafe-sdk-request"
    || error.headers.get("retry-after") !== "0" || JSON.stringify(error.body).includes("sdk-provider-secret")) {
    throw error;
  }
}
const retry = await client.systemOne({ state: { scenario: "retry" }, questions: { check: noul("Is this true?") } }, {
  retry: { maxRetries: 1, backoffInitialMs: 1, backoffMaxMs: 1, backoffJitter: 0 },
});
if (retry.answers.check.noul !== 1) throw new Error("system_one_sdk_retry_failed");
const controller = new AbortController();
const cancelled = client.systemOne({ state: { scenario: "cancel" }, questions: { check: noul("Is this true?") } }, {
  signal: controller.signal,
}).catch(error => error);
const started = await fetch(`${fixtureControlURL}/cancel-started`);
if (started.status !== 204) throw new Error("system_one_sdk_cancellation_fixture_failed");
controller.abort();
if (!(await cancelled instanceof APIUserAbortError)) throw new Error("system_one_sdk_cancellation_failed");
try {
  await client.systemOne({ state: { scenario: "timeout" }, questions: { check: noul("Is this true?") } }, { timeout: 1000 });
  throw new Error("system_one_sdk_timeout_was_not_raised");
} catch (error) {
  if (!(error instanceof APITimeoutError)) throw error;
}
process.stdout.write(JSON.stringify({ sdk: "0.7.0", requests: 7 }));
