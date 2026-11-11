# TypeSafe System One

LLM Proxy exposes the TypeSafe API at `POST /v1/systemone`.
The request and response use the TypeSafe wire contract.
The proxy selects the upstream connection from the tenant configuration.
The client supplies a tenant bearer key and the original TypeSafe model name.

The [OpenAPI contract](openapi.yaml) owns the public schemas.
The runtime schema file is generated from that contract with `make generate-api-docs`.
The [TypeSafe API reference](https://docs.typesafe.ai/api/system-one) defines the upstream interface.

## Provider setup

1. Open the [TypeSafe console](https://console.typesafe.ai/keys) with the account that owns the provider access.
2. Create an API key through the console.
3. Create a TypeSafe provider connection in LLM Proxy.
4. Supply the provider key.
5. Assign the connection to the tenant.
6. Create a tenant client key for the application.

Connection verification uses authenticated `GET /v1/models` through the `system_one_models` protocol.
The verifier checks the model list and its required metadata.
Provider credentials remain in the encrypted connection store.
The provider catalog owns the upstream API origin, normally `https://api.typesafe.ai`.

The catalog records the exact model `jev-1.14.0`.
The public request keeps TypeSafe names such as `jev-latest`, `jev-preview`, and `jev-1.14.0` unchanged.
The upstream API determines whether a model name is available.
The proxy rejects an ambiguous provider selection before an upstream request.

## TypeScript SDK

Use the official `@typesafe-ai/sdk` package with the proxy API origin and tenant key.
The SDK adds `/v1/systemone` to its base URL.

```typescript
import { noul, TypeSafeClient } from "@typesafe-ai/sdk";

const client = new TypeSafeClient({
  baseURL: process.env.LLM_PROXY_BASE_URL,
  apiKey: process.env.LLM_PROXY_TENANT_KEY,
  defaultModel: "jev-latest",
});
const result = await client.systemOne({
  state: { message: "The export failed." },
  questions: { failed: noul("Did the export fail?") },
});
console.log(result.answers.failed.noul);
```

The SDK and API have different validation rules.
The published API schema accepts a Score rubric with one level.
SDK `0.7.0` requires at least two levels before dispatch.
The proxy follows the API contract and uses the existing Go HTTP transport.
The TypeScript SDK serves as a real consumer in acceptance tests.

The SDK retries some failed requests by default.
Set `retry: { maxRetries: 0 }` when the application requires one attempt.
The proxy makes one upstream attempt for each accepted HTTP request.

## Wire behavior

The endpoint accepts JSON and multipart requests with the TypeSafe fields and question types.
Choice, Score, Noul, Box, structured instructions, and image assets keep their upstream representations.
Images and Box questions require `TypeSafe-Danger-Zone-Images: true`.
Long context uses `TypeSafe-Danger-Zone-Long-Context: true`.

The endpoint accepts one gzip or zstd compression layer.
The `x-gzip` name is also accepted.
The compressed and decoded body limits are 32 MiB.
The zstd window limit is 8 MiB.
Each structured state, instruction, and criterion has a 64-level nesting limit.

Accepted request bytes reach the selected provider without question reconstruction.
Successful response bytes, model names, answer identifiers, confidence, probabilities, and usage remain unchanged.
Null token counts remain null in the public response.
Internal usage records keep the distinction between measured and absent quantities.

The official SDK [usage type](https://docs.typesafe.ai/sdk/javascript/api/interfaces/Usage) permits null counts.
The published upstream OpenAPI schema still lists integer counts.
The proxy schema accepts both forms to retain the documented provider result.

Upstream errors keep their status, documented body fields, and supported request and retry headers.
The proxy removes provider credential values from error content.
Local failures use TypeSafe `detail` error bodies.
Malformed input returns `400`.
Schema violations return `422`.

Body limits return `413`.
Unsupported content or compression returns `415`.
The existing request budget and upstream admission controls apply.

The endpoint has no result resource, retention requirement, or mandatory idempotency key.
Provider errors do not select another provider.
The System One Adapter remains a separate model-backed evaluator outside this Jev route.

## Native clients

The Go client provides `PostSystemOne` with wire bytes and `SystemOneRequestOptions`.
Its result keeps the status, body, and response headers.
The Python client provides `post_system_one` with a JSON mapping or wire bytes.
Both methods accept content metadata and experimental headers without their own question rules.

## Development acceptance

Run `make test-system-one` for real HTTP and official SDK acceptance with local provider fixtures.
Run `make test-system-one-clients` for the Go and Python clients.
Run `make test-provider-catalog` and `make test-protocol-acceptance` for connection and route contracts.
Run `make ci` after the final implementation change.

The local tests cover compressed JSON, multipart images, typed answers, errors, tenant isolation, and cancellation.
The SDK consumer version is `0.7.0`.

## Operational acceptance

The [operational acceptance record](typesafe-live-acceptance.md) tracks pending live TypeSafe qualification.
It contains the required inputs, live checks, and evidence fields.
F074 is closed on completed implementation and deterministic CI evidence.

Future provider adapters must satisfy the same public TypeSafe contract.
Qualify each provider's authentication, models, schemas, usage, errors, terms, and prices before activation.
