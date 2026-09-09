# Agent MCP protocol map

Checked on 2026-09-08. MCP revisions describe the client protocol, not the model.
The same agent can change protocols after an update or a runtime setting change.
An initial version offer identifies the client's requested revision. It does not identify every revision the client can accept.
This map separates client requests, source inspection, vendor documentation, and reported tests.
An accepted revision does not prove successful authorization or tool execution.

## Server support and client demand

The [server support table](mcp.md#supported-mcp-revisions) defines the current repository implementation.
The server implements four revisions: `2026-07-28`, `2025-11-25`, `2025-06-18`, and `2025-03-26`.
The client results below establish active use of the first three revisions.
The current client requirement for `2025-03-26` remains an open decision.
A published Windsurf record identifies that revision but supplies no build or observation date.

Client offers alone do not establish the smallest sufficient server support list.
A client can accept a different revision in the server response.
Each proposed support change requires a connection test and a tool test with the affected clients.

## Client evidence

| Agent or client | Build or scope checked | Protocol evidence | Streamable HTTP and OAuth | LLM Proxy acceptance |
| --- | --- | --- | --- | --- |
| Codex CLI | 0.153.4 | Measured `initialize` offer: `2025-06-18`. The initial request omits the version header. | Both supported. Live TAuth login passed. | Local tenant discovery, route reads, and generation passed with a TAuth-issued token. |
| Codex desktop runtime | Bundled CLI 0.153.4 | The bundled version matches the measured CLI. Desktop tool loading requires a separate check. | Configured HTTP server and browser OAuth flow. | Live OAuth passed. Live MCP acceptance awaits the proxy update. |
| Claude Code | Installed 2.1.206 | Measured `initialize` offer: `2025-11-25`. Later requests send that version header. | Both documented. | Loopback protocol and tool discovery passed. LLM Proxy OAuth and generation were not tested with this client. |
| Claude Code v2 runtime | Vendor documents 2.1.232 and later | Documents support for `2026-07-28`. Runtime and negotiation settings affect the selected revision. | HTTP can negotiate the modern revision. Some provider environments select v1. | Vendor evidence only. This runtime was not exercised locally. |
| Gemini CLI | Installed 0.45.3 | Installed SDK declares `2025-11-25` as its default. Its accepted list also includes `2025-06-18` and `2025-03-26`. | Both documented. | Source inspection only. The management list command did not establish a wire revision or generation result. |
| OpenCode | Installed 1.18.27, test dependency 1.18.28 | Measured 1.18.27 offer: `2025-11-25`. The pinned 1.18.28 client passes the endpoint check. | Remote HTTP and OAuth documented. | 1.18.28 connection and tool discovery passed with a local TAuth-issued bearer token. Native OAuth was not tested. |
| Cursor | Firsthand July 2026 report | The reported stdio request offers `2025-11-25`. See [Cursor trace][cursor-trace]. | Streamable HTTP and OAuth documented. This trace covers stdio. | Current HTTP negotiation and LLM Proxy acceptance require a capture. |
| GitHub Copilot in VS Code | Installed 1.136.1 | The installed initializer selects `2025-11-25`. The [tagged source][vscode-source] agrees. | Remote MCP HTTP and OAuth documented. | Source inspection. LLM Proxy acceptance was not tested. |
| Claude Desktop | Installed 1.20186.1 | The installed local MCP initializer selects `2025-11-25`. | This result covers the local desktop runtime. Hosted connectors require separate results. | Source inspection. LLM Proxy acceptance was not tested. |

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
| `@google/gemini-cli` | 0.59.0 | The published connection code selects `2025-06-18`. The active initializer uses SDK 1.23.0. This is source inspection, without a wire capture. |
| `opencode-ai` | 1.18.29 | This release was not tested locally. The tested dependency is 1.18.28. |

Read release metadata with `npm view <package> version`.
The Gemini release evidence comes from its published npm archive and
[`@modelcontextprotocol/sdk` 1.23.0 declarations](https://unpkg.com/@modelcontextprotocol/sdk@1.23.0/dist/esm/types.js).
The published Gemini chunks create `gemini-cli-mcp-client` through that initializer.
The inspected chunks are `chunk-S4PJ76PA.js`, `chunk-SM627E5R.js`, and `chunk-YSBB75DZ.js` under `package/bundle/`.
The separate `2025-11-25` declaration belongs to another bundled component.
See the [tagged Gemini connection source][gemini-source].

## Additional client results

Reported tests are firsthand public observations. They are not local LLM Proxy acceptance results.
Source on a development branch can differ from a published release.

| Client | Build or scope | Revision and connection behavior | Result type and limit |
| --- | --- | --- | --- |
| Google Antigravity CLI | 1.1.5, reported July 21, 2026 | `server/discover` requests `2026-07-28`. Client name: `antigravity-client`. | [Firsthand stdio trace][antigravity-cli]. HTTP requires a separate capture. |
| Google Antigravity IDE | 2.5.5 | A report shows the `server/discover` connection method. | [Firsthand IDE report][antigravity-ide]. The exact HTTP revision remains unmeasured. |
| Claude.ai | Hosted connector, reported March 2026 | `claude-ai/0.1.0` offers `2025-11-25`. Anthropic also announced a `2026-07-28` rollout. | [Request trace][claude-trace] and [vendor announcement][claude-rollout]. Account rollout state can differ. |
| ChatGPT | Hosted connector, reported August 2026 | `openai-mcp/1.0.0` offers `2025-11-25`. | [Firsthand trace][chatgpt-trace]. A current account capture remains necessary for local acceptance. |
| Grok Build | Source commit `75810042ca2762aa0b0fa17864f3f68823ccbea5`, September 8, 2026 | HTTP probes `2026-07-28` when its time budget permits. The `initialize` path explicitly selects `2025-11-25`. Stdio uses initialization. | [Pinned connection source][grok-source]. Published binary behavior was not tested. This is separate from Grok Bot. |
| Z.AI ZCode | CLI 0.16.1, reported August 8, 2026 | The integration author reports `2026-07-28` discovery and an older initialization path. | [Firsthand integration result][zcode-bridge]. A [Z.AI report][zcode-report] also shows modern negotiation in agent 0.16.3. Hosted Z.AI requires separate results. |
| Zed | Tagged 1.17.2 | The initializer selects `2025-11-25`. | [Tagged client source][zed-source] and [version declaration][zed-types]. No local connection test. |
| Goose | 1.48.0 source, commit `25021517f12cab87c94bed0874fe7d28168dc264` | Default negotiation prefers `2026-07-28`. Its initialization path selects `2025-11-25` through rmcp 3.1.4. | [Pinned client source][goose-source] and [SDK declaration][rmcp-source]. No local connection test. |
| Continue | Development source checked September 8, 2026 | The client uses the SDK initializer without a version override. The declared SDK range starts at 1.25.2, whose default is `2025-11-25`. | [Client source][continue-source], [dependency][continue-package], and [SDK declaration][sdk-1252]. A published build requires a capture. |
| Cline SDK | Development source checked September 8, 2026 | The HTTP client uses the SDK path for `2025-11-25`. The separate stdio initializer explicitly selects `2024-11-05`. | [Client source][cline-source] and [dependency][cline-package]. The stdio result does not establish HTTP demand for November 2024. |

## Open decisions and required captures

| Client | Established information | Required observation |
| --- | --- | --- |
| Grok Bot | Grok Bot and Grok Build have separate connection implementations. The Bot's exact offered revision remains unknown. | Connect Grok Bot to an instrumented HTTPS server. Record its first requests and successful tool calls. |
| Z.AI hosted MCP calling | The [vendor guide][zai-hosted] describes remote MCP calls from the hosted API. ZCode is a separate client. | Trigger one hosted MCP call. Record discovery, initialization, selected revision, and tool results at the server. |
| Windsurf | The [client registry][apify-registry] records `2025-03-26`. Its records omit build numbers and observation dates. | Capture the latest installed Windsurf build over HTTP. Test server counteroffers before deciding whether March 2025 is required. |
| Antigravity and Cursor over HTTP | The reports above establish stdio or discovery behavior. They do not establish the complete current HTTP flow. | Record each installed build's HTTP requests, selected revision, authorization result, and tool results. |

The model provider does not determine the MCP client protocol.
GLM through OpenCode uses OpenCode's client. Z.AI hosted MCP calling uses Z.AI's service.
An ACP bridge can also use a different MCP client from the native application.

## Capture procedure

1. Record the application build, date, transport, and runtime settings.
2. Record `clientInfo`, the first method, offered revision, and protocol headers.
3. For `server/discover`, record protocol metadata and the selected revision.
4. Do a test of each proposed server revision with a separate connection.
5. Record whether the client accepts the server's selected revision.
6. Run `tools/list` and one harmless `tools/call` through each accepted connection.
7. Record OAuth, local acceptance, and production acceptance as separate results.
8. Exclude tokens, cookies, credentials, and prompt content from saved captures.

An SDK constant alone does not prove the active connection path.
Source inspection must trace the application client to its initializer or discovery call.
Binary strings alone do not establish the default revision.

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

[cursor-trace]: https://forum.cursor.com/t/stdio-mcp-oracle-sqlcl-dies-after-parallel-reconnect-and-resource-subscribe-with-failed-to-enqueue-message/166799

[vscode-source]: https://github.com/microsoft/vscode/blob/1.136.1/src/vs/platform/mcp/common/modelContextProtocol.ts

[gemini-source]: https://github.com/google-gemini/gemini-cli/blob/v0.59.0/packages/core/src/tools/mcp-client.ts

[antigravity-cli]: https://github.com/UsefulSoftwareCo/executor/issues/1449

[antigravity-ide]: https://github.com/yvgude/lean-ctx/issues/1454

[claude-trace]: https://github.com/anthropics/claude-ai-mcp/issues/77

[claude-rollout]: https://claude.com/blog/bringing-mcp-2026-07-28-to-claude

[chatgpt-trace]: https://community.openai.com/t/custom-mcp-connector-web-client-drops-the-stream-mid-turn-connection-interrupted-works-fine-on-mobile-same-server-same-connector/1388400

[grok-source]: https://github.com/xai-org/grok-build/blob/75810042ca2762aa0b0fa17864f3f68823ccbea5/crates/codegen/xai-grok-mcp/src/servers.rs

[zcode-bridge]: https://github.com/tizerluo/zcode-open-bridge

[zcode-report]: https://github.com/zai-org/feedback/issues/302

[zed-source]: https://github.com/zed-industries/zed/blob/v1.17.2/crates/context_server/src/protocol.rs

[zed-types]: https://github.com/zed-industries/zed/blob/v1.17.2/crates/context_server/src/types.rs

[goose-source]: https://github.com/aaif-goose/goose/blob/25021517f12cab87c94bed0874fe7d28168dc264/crates/goose/src/agents/mcp_client.rs

[rmcp-source]: https://docs.rs/rmcp/3.1.4/src/rmcp/model.rs.html

[continue-source]: https://github.com/continuedev/continue/blob/main/core/context/mcp/MCPConnection.ts

[continue-package]: https://github.com/continuedev/continue/blob/main/core/package.json

[sdk-1252]: https://unpkg.com/@modelcontextprotocol/sdk@1.25.2/dist/esm/types.js

[cline-source]: https://github.com/cline/cline/blob/main/sdk/packages/core/src/extensions/mcp/client.ts

[cline-package]: https://github.com/cline/cline/blob/main/sdk/packages/core/package.json

[zai-hosted]: https://docs.z.ai/guides/capabilities/mcp-call

[apify-registry]: https://github.com/apify/mcp-client-capabilities
