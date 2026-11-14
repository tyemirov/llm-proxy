# TypeSafe Operational Acceptance

## Record: 2026-11-12

Status: Production proxy acceptance passed.
Owner: LLM Proxy maintainer.
Related development: F074 and I321.

The maintainer authorized the saved connection and production Jev requests.
The management UI confirmed `TypeSafe Jev` as connected to `Default`.
The authenticated tenant identity matched the selected Default tenant.

The official TypeScript SDK used the production proxy URL and the existing tenant key issued by LLM Proxy.
The SDK process received no provider credential.
The proxy used the saved TypeSafe connection.
Both requests used `jev-1.14.0`, with automatic retries disabled.

| Capability | Result | Proxy request ID | TypeSafe request ID | Input / output tokens |
| --- | --- | --- | --- | --- |
| Choice, Score, and Noul | HTTP 200. All assertions passed. | `3D32PX3UFAYCK5HHWVIMBO4CWH` | `req_01a1d39896a870fcab814bbe9eb0023a` | 375 / 61 |
| Image and Box | HTTP 200. All assertions passed. | `GL3OVB3S7ZLJHMDB742UZFCBYW` | `req_01a1d398975d7340981d879cd700a209` | 1036 / 20 |

The request endpoint was `https://llm-proxy-api.mprlab.com/v1/systemone`.
The SDK version was `0.7.0`.
The published website release marker reported `v1.12.12`.
The [sanitized production evidence](../.mprlab/evidence/typesafe-live-20261112.json) contains timestamps, source hashes, assertions, and results.

The run used the same text and image consumer that `make live-test` invokes.
It selected only the two Jev cases.
It did not repeat the nine other paid provider cases.
The November 11 CI run already passed all 15 gates with 100.0 percent Go statement coverage.
This operational run changed no application code.

## Prior record: 2026-11-11

Historical status: Direct provider checks passed. Proxy execution awaited connection approval.
Owner: LLM Proxy maintainer.
Related development: F074 is closed on completed implementation and deterministic CI evidence.

The final development run passed all 15 CI gates with 100.0 percent Go statement coverage and zero uncovered blocks.
Operational acceptance requires live TypeSafe execution through the implemented proxy endpoint.
This record tracks operational acceptance separately from development completion.

### Historical required inputs

- Confirm the TypeSafe account and organization for the live run.
- Supply the provider credential through its approved private input.
- Select the proxy runtime and tenant connection for the check.
- Obtain authorization for the live provider requests.

At development completion, the documented `TYPESAFE_API_KEY` environment input was absent.
This historical input check does not establish the current credential state.

The November 11 check found the existing provider key and default tenant key in the documented `configs/.env` input.
The selected key matches the prefix and suffix displayed for the active organization key.
The proxy account had no saved TypeSafe connection.
The prepared connection was named `TypeSafe Jev` and selected the `Default` tenant.
Its creation awaited approval because it granted that tenant provider access.

### Historical pending acceptance

1. Completed: Verify the selected provider credential with authenticated `GET https://api.typesafe.ai/v1/models`.
2. Completed: Select an available exact model and record its identifier.
3. Run authorized live requests through `POST /v1/systemone` with the selected tenant connection.
4. Examine typed Choice, Score, Noul, and Box results against the TypeSafe contract.
5. Examine model identity, answer identifiers, usage, request identifiers, status codes, and provider errors.
6. Record the exact model, endpoint, usage, and sanitized request and response evidence.
7. Keep credentials and request content outside public evidence.
8. Record the result and execution date in this operational record.

Use the [integration guide](typesafe-system-one.md) for setup and wire behavior.
Qualify each future upstream provider separately before operational activation.

The repeatable production command is `make live-test`.
It includes Jev Choice, Score, Noul, image, and Box checks through the official TypeScript SDK.
It verifies the Default tenant identity and uses its saved TypeSafe connection.
See [production live test guidance](development.md#production-default-tenant-live-test) for the complete eleven-case suite.
`make test-production-live-contract` checks the command against local protocol fixtures.
These controlled checks do not establish production connectivity.

### Historical live evidence

| Field | Current evidence |
| --- | --- |
| Execution date | November 11, 2026. partial verification |
| Account and organization | Marco Polo Research Lab. confirmed through the existing console session |
| Proxy runtime and source revision | Production release `v1.12.12`. deployed proxy execution pending |
| Provider endpoint and exact model | Direct `https://api.typesafe.ai/v1/systemone`. `jev-1.14.0` |
| Credential verification | `GET /v1/models`: HTTP 200. request `req_01a1ccecb2cf7729a0d042128a6c020d` |
| Choice, Score, and Noul | Direct HTTP 200. request `req_01a1cceee5b274029ade5fe030cc5b97` |
| Image and Box | Direct HTTP 200. request `req_01a1ccf2f4ab7493ab0be9cec4ad4442` |
| Usage | Text: 375 input and 61 output tokens. Image: 1040 input and 20 output tokens. |
| Proxy authentication | Invalid bearer: HTTP 401 through the SDK. Existing default tenant key: HTTP 200 on model discovery. |
| Tenant identity | `/v2/identity` returned HTTP 200. Its tenant identifier matches the selected `Default` management tenant. |
| Sanitized request and response evidence | [Verification evidence](../.mprlab/evidence/typesafe-live-20261111.json). credentials and request bodies excluded |
| Acceptance result | Direct provider checks passed. Full proxy acceptance remains pending. |

The TypeScript SDK version is `0.7.0`. Automatic retries were disabled.
The provider advertised `jev-latest` and `jev-preview` through model discovery.
The direct calls also confirmed that `jev-1.14.0` accepts text and image requests.

The provider rejected the initial 256 × 256 image with HTTP 400 and `Invalid image dimensions.`
The corrected 512 × 512 image passed.
The current [image limits](https://docs.typesafe.ai/images#limits) require at least 280 pixels on each side.
