# MCP access

The account MCP URL is `https://llm-proxy-api.mprlab.com/mcp`.
The server supports `2026-07-28`, `2025-11-25`, `2025-06-18`, and `2025-03-26`.
All four revisions use the same stateless Streamable HTTP endpoint with JSON responses.
See the [agent protocol map](mcp-clients.md) for measured client versions and vendor documentation.
Each request selects its tenant. The connection has no active tenant or MCP session.

## Connect a client

1. Sign in to the application.
2. Open **Settings** and select **Copy MCP URL**.
3. Add that URL as a Streamable HTTP server in a client that supports one listed revision and OAuth.
4. Complete the TAuth login and consent steps in the browser.
5. Call `llm_proxy.list_tenants` with `{}`.
6. Select a returned tenant ID for generation or route discovery.

The copy action and tenant discovery work before provider setup.
Generation requires a configured text route in the selected tenant.
Set the provider credentials and defaults through the application.

For MCP Inspector 2.5.0, use this server configuration:

```json
{
  "mcpServers": {
    "llm-proxy": {
      "type": "http",
      "url": "https://llm-proxy-api.mprlab.com/mcp",
      "protocolEra": "modern"
    }
  }
}
```

Use a TAuth-registered OAuth client or an HTTPS Client ID Metadata Document that TAuth accepts.
The document must declare the client's redirect URI and permitted grant.
The client stores and refreshes its OAuth tokens.
Do not put a tenant secret or provider credential in the MCP configuration.

Codex 0.153.4 passed local tenant discovery, route reads, and generation with a TAuth-issued token.
OpenCode 1.18.28 passed local connection and tool discovery checks.
MCP Inspector 2.5.0 and the official Go SDK v1.7.0 passed local connection checks.

## Protocol negotiation

The official Go SDK handles both protocol families.
The `2026-07-28` revision uses `server/discover` and protocol metadata on each request.
The three `2025` revisions use `initialize`, followed by `notifications/initialized`.
The server returns the requested supported handshake revision.
For another offered handshake revision, it returns `2025-11-25` for client agreement.

The initial handshake can omit `MCP-Protocol-Version`.
Subsequent clients send the negotiated version in that header.
When the header is absent, the SDK applies the protocol-defined `2025-03-26` default.
An explicit unsupported header receives HTTP `400`.
The endpoint rejects repeated version headers, query parameters, and session IDs.
Authentication and tenant ownership checks apply to all revisions.
Every response retains `Cache-Control: no-store`.

The endpoint provides no standalone SSE stream. A GET receives HTTP `405`.
Earlier protocol dates in this support list use Streamable HTTP, without a separate SSE endpoint.

## Authorization

An unauthenticated `POST /mcp` returns `401` with a Bearer challenge.
Its `resource_metadata` field identifies `/.well-known/oauth-protected-resource/mcp` on the API origin.
That public metadata supplies the TAuth issuer and the `llm-proxy:use` scope.
The OAuth resource identifier and token audience are the API origin, without `/mcp`.

TAuth owns authorization-code issuance, PKCE, login, consent, refresh, and revocation.
The proxy uses TAuth's `pkg/oauthvalidator` to validate the access token on each request.
Validation checks the signature, issuer, subject, exact audience, expiry, and scope.
The proxy also checks the configured TAuth tenant ID.
A token with an incorrect scope receives `403`.

The grant permits access to every tenant that the account currently owns.
The proxy checks ownership for each generation and resource read.
Tenant creation, renaming, and deletion change subsequent discovery results.
A foreign tenant and a missing tenant produce the same error.
TAuth refresh-token revocation prevents another refresh.
An issued access token remains valid until expiry, which is five minutes under the declared TAuth policy.

## Tools and resources

`llm_proxy.list_tenants` accepts an empty object and returns this structured shape:

```json
{
  "tenants": [
    {
      "id": "managed-example",
      "name": "Project",
      "has_secret": false,
      "created_at": "2026-09-08T00:00:00Z",
      "updated_at": "2026-09-08T00:00:00Z"
    }
  ]
}
```

Discovery uses the same summary query and fields as `GET /api/management/account`.
It creates no account, tenant, secret, provider request, or usage event.
An account without tenants receives `{"tenants":[]}`.
The REST account endpoint retains TAuth session authentication and account setup.

`llm_proxy.generate_text` requires `tenant_id` and canonical `/v2` messages:

```json
{
  "tenant_id": "managed-example",
  "messages": [{"role": "user", "content": "Explain the result."}],
  "request_timeout_seconds": 60
}
```

Optional controls are `provider`, `model`, `web_search`, `max_tokens`, `reasoning_effort`, and `request_timeout_seconds`.
Use the canonical ordered image and audio attachment fields on user messages.
The selected route must support the requested media and controls.
Omitted route controls use the selected tenant's defaults.
Unknown fields and null optional controls are rejected.

A successful tool result contains a text block and structured fields:
`text`, `request_id`, `provider`, `model`, `usage`, and `request_timeout_seconds`.
The `usage` value is null when the provider supplies no token counts.
An accepted call that fails returns `isError: true` and a structured `code`.
Codes include `not_found`, `invalid_request`, `invalid_request_timeout`, `provider_not_configured`,
`rate_limited`, `service_unavailable`, `request_timeout`, `upstream_error`, and `proxy_error`.
The error contains no upstream response body or prompt.

Read `llm-proxy://tenants/{tenant_id}/routes` to obtain `tenant_id`, `defaults`, and `routes`.
Defaults contain `provider`, `model`, and `reasoning_effort`.
Each route contains `provider`, `model`, `web_search`, `media_inputs`, and `reasoning_efforts`.
The resource includes only configured text routes.
It contains no system prompt, provider credential, or tenant secret.

Generation can create provider charges.
REST and MCP share route validation, media handling, admission, queues, rate limits, continuation, cancellation, and usage submission.
Usage records use endpoint `mcp` and the logical proxy status.
For example, queue rejection records `503` even when the MCP transport returns `200`.
Schema rejection and discovery create no generation usage event.
Management mutations and dictation are outside this MCP interface.

The server applies the configured message limit to each authenticated request body, with a maximum of 8 MiB.
The upload must finish within `server.request_timeout_seconds`.
An upload timeout returns HTTP `408` before SDK dispatch and creates no generation usage event.
Request cancellation stops the upload read.
The server clears the upload deadline before tool execution.
Generation then uses its separate `request_timeout_seconds` budget, or the server default if omitted.

## Runtime and local acceptance

The server derives its issuer from `management.tauth_url` and its audience from `management.proxy_origin`.
The deployment manifest already declares the API route, TAuth resource, and required scope.
The gateway owns the deployed TAuth OAuth configuration.
The API's existing public route serves `/mcp` and its metadata endpoint.

For local Compose, set `TAUTH_OAUTH_ES256_PRIVATE_KEY_BASE64` in the private `configs/.env.local` input.
The value is base64-encoded PKCS8 PEM for an ES256 private key.
The local TAuth template reads this key and declares the same resource and scope.
The frontend proxy serves `/oauth/*` and `/.well-known/oauth-authorization-server` from TAuth.
The local MCP URL uses the API origin and `/mcp`.

Run `make test-mcp` for public HTTP and SDK integration checks.
Run `make test-mcp-oauth` for the real local TAuth browser flow.
That command uses ephemeral keys, a local provider fixture, the official Go client, and MCP Inspector.
It checks PKCE, consent, generation, refresh rotation, and revocation.
It reads the OAuth blocks from `configs/tauth.local.yml`.
It also verifies the pinned OpenCode client connection.
Run `make test-mcp-versions` for the three handshake revisions, authorization, isolation, generation, and version negotiation.
Run `make test-mcp-codex` with Codex installed for actual Codex MCP calls after the local TAuth flow.
This target creates an ephemeral client context and runs no model turn.
Run `make ci` for repository validation.
These commands do not provide production acceptance evidence.

For `2026-07-28`, low-level callers must send the method headers and per-request `_meta` fields.
The handshake revisions use their negotiated version header without those modern fields.
Use an SDK to construct the fields for the selected revision.
The generated OpenAPI reference describes the HTTP envelope and generation schema.
