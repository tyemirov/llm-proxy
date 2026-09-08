# Agent MCP protocol map

Checked on 2026-09-08. MCP revisions describe the client protocol, not the model.
The same agent can change protocols after an update or a runtime setting change.
An initial version offer proves that revision. It does not identify every revision the client can accept.

## Client evidence

| Agent or client | Build or scope checked | Protocol evidence | Streamable HTTP and OAuth | LLM Proxy acceptance |
| --- | --- | --- | --- | --- |
| Codex CLI | 0.153.4 | Measured `initialize` offer: `2025-06-18`. The initial request omits the version header. | Both supported. Live TAuth login passed. | Local tenant discovery, route reads, and generation passed with a TAuth-issued token. |
| Codex desktop runtime | Bundled CLI 0.153.4 | The bundled version matches the measured CLI. Desktop tool loading requires a separate check. | Configured HTTP server and browser OAuth flow. | Live OAuth passed. Live MCP acceptance awaits the proxy update. |
| Claude Code | Installed 2.1.206 | Measured `initialize` offer: `2025-11-25`. Later requests send that version header. | Both documented. | Loopback protocol and tool discovery passed. LLM Proxy OAuth and generation were not tested with this client. |
| Claude Code v2 runtime | Vendor documents 2.1.232 and later | Documents support for `2026-07-28`. Runtime and negotiation settings affect the selected revision. | HTTP can negotiate the modern revision. Some provider environments select v1. | Vendor evidence only. This runtime was not exercised locally. |
| Gemini CLI | Installed 0.45.3 | Installed SDK declares `2025-11-25` as its default. Its accepted list also includes `2025-06-18` and `2025-03-26`. | Both documented. | Source inspection only. The management list command did not establish a wire revision or generation result. |
| OpenCode | Installed 1.18.27, test dependency 1.18.28 | Measured 1.18.27 offer: `2025-11-25`. The pinned 1.18.28 client passes the endpoint check. | Remote HTTP and OAuth documented. | 1.18.28 connection and tool discovery passed with a local TAuth-issued bearer token. Native OAuth was not tested. |
| Cursor | Current vendor documentation | Exact MCP revision is not stated in the referenced page. | Streamable HTTP and OAuth documented. | Protocol and end-to-end acceptance are unverified. |
| GitHub Copilot in VS Code | Current vendor documentation | Exact MCP revision is not stated in the referenced page. | Remote MCP HTTP and OAuth documented. | Protocol and end-to-end acceptance are unverified. |

The proxy supports `2026-07-28`, `2025-11-25`, `2025-06-18`, and `2025-03-26` on `/mcp`.
The `2025` revisions use `initialize`. The `2026` revision uses `server/discover`.
The endpoint supports JSON responses and no standalone SSE stream.
A shared protocol revision is necessary, but OAuth client registration and tenant configuration also affect connection success.

## Published release check

The npm registry reported these latest releases on the check date. Installed builds above can differ.
An untested release does not inherit the acceptance result of a tested build.

| Package | Latest release | Evidence for that release |
| --- | --- | --- |
| `@openai/codex` | 0.153.4 | Matches the tested CLI. |
| `@anthropic-ai/claude-code` | 2.1.265 | Vendor runtime guidance applies to 2.1.232 and later. This release was not tested locally. |
| `@google/gemini-cli` | 0.59.0 | The published bundle contains SDK declarations for both `2025-06-18` and `2025-11-25`. Its core dependency declares SDK 1.23.0, whose default is `2025-06-18`. The active runtime path was not measured. |
| `opencode-ai` | 1.18.29 | This release was not tested locally. The tested dependency is 1.18.28. |

Read release metadata with `npm view <package> version`.
The Gemini release evidence comes from its published npm archive and
[`@modelcontextprotocol/sdk` 1.23.0 declarations](https://unpkg.com/@modelcontextprotocol/sdk@1.23.0/dist/esm/types.js).

## Sources and reproduction

- [Codex MCP configuration](https://developers.openai.com/codex/mcp) documents HTTP servers and `codex mcp login`.
- [Claude Code MCP runtimes](https://code.claude.com/docs/en/mcp#mcp-client-runtimes) documents the two SDK generations and their selection rules.
- [Gemini CLI MCP configuration](https://geminicli.com/docs/tools/mcp-server/) documents `httpUrl` and OAuth configuration.
- [Gemini CLI source at v0.45.3](https://github.com/google-gemini/gemini-cli/tree/v0.45.3) identifies the client release inspected.
- [OpenCode MCP configuration](https://opencode.ai/v2/docs/mcp-servers) documents remote Streamable HTTP and OAuth.
- [Cursor MCP support](https://cursor.com/docs/mcp) lists transports and authentication.
- [VS Code MCP support](https://code.visualstudio.com/docs/agent-customization/mcp-servers) documents HTTP setup and OAuth.
- [MCP lifecycle for 2025-06-18](https://modelcontextprotocol.io/specification/2025-06-18/basic/lifecycle) defines initialization and version agreement.
- [MCP HTTP transport for 2025-06-18](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports) defines notification responses and version headers.

For measured offers, a loopback HTTP fixture recorded only the method, offered revision, and protocol header.
The fixture returned an initialization result and an empty tool list. It used no credentials or model calls.
Claude Code used an isolated `CLAUDE_CONFIG_DIR` and `claude mcp list`.
OpenCode used an isolated configuration and `opencode mcp list`.
Codex used its app-server API with an ephemeral context and no model turn.
The Gemini observation came from the installed bundle's `LATEST_PROTOCOL_VERSION` and `SUPPORTED_PROTOCOL_VERSIONS` declarations.

Run `make test-mcp-versions` to qualify the supported handshake revisions through real authenticated HTTP requests.
Run `make test-mcp-oauth` for local TAuth, Go SDK, MCP Inspector, and OpenCode acceptance.
Run `make test-mcp-codex` with Codex installed for its actual tenant, route, and generation tool calls.
These tests use local TAuth and a provider fixture. They do not establish production acceptance.

When a client changes, record its exact build, observed offer, transport, OAuth result, and tool result in this map.
Keep undocumented revisions marked unverified until a protocol capture or vendor source establishes them.
