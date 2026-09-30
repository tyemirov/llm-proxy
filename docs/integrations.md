# Integration register

This register records LLM Proxy integrations and their evidence.
The LLM Proxy maintainer owns this document, consumer update coordination, and the [promotional claims](marketing/integrations.md).
The [OpenAPI contract](openapi.yaml) owns HTTP behavior.
The [client protocols](client-protocols.md) and [MCP client map](mcp-clients.md) contain protocol details.

## Current records

Records reviewed on September 29, 2026, in America/Los_Angeles.
Source inspection, successful tests, upstream acceptance, and publication are separate results.

| Consumer | Connection | Evidence and current state | Required work |
| --- | --- | --- | --- |
| [Goose](https://github.com/aaif-goose/goose) | Existing OpenAI custom provider. `/v1/chat/completions` with a tenant bearer key. No LLM Proxy SDK dependency. | [Maintainer confirmation](https://github.com/aaif-goose/goose/issues/12482#issuecomment-5902687237). Issue 12482 is closed. Board state is Done. No integration PR. Configuration source inspected at `ac15f938151bb8c0efd93ed9c2c1cf61298934bf`. | Use the [Goose guide](integrations/goose.md). Complete I296 before claiming tested Goose execution. |
| [PAL MCP Server](https://github.com/BeehiveInnovations/pal-mcp-server) | Proposed optional Python SDK provider. Native `/v2` text requests. | [PR 483](https://github.com/BeehiveInnovations/pal-mcp-server/pull/483) is open at `4c08ea8722719a4dcec0c4141bbd15fb57e2369a`. Python 3.10–3.12, lint, and Docker checks passed. Upstream merge awaits review. | Coordinate SDK changes with the PR adapter, installation instructions, and model configuration. Track I288–I294. |
| [OpenCode](https://github.com/anomalyco/opencode) | Standard OpenAI provider packages. `/v1` Chat Completions and Responses. | Repository examples and `TestClientProtocolsOpenCode` cover text and a local file read. `package.json` pins `opencode-ai` to `1.18.28`. This document change did not rerun those tests. | Use the [client examples](client-protocols.md#opencode-examples). Run `make test-client-protocols` after relevant changes. |
| Official Go SDK, Python SDK, and CLI | Native `/v2`, selected client protocols, and media resources. | Repository-owned clients. [Client protocols](client-protocols.md) describe their current interfaces. Release publication requires its own evidence. | Run the applicable client checks. Record the published version and each affected consumer update. |
| MCP consumers | `/mcp` with tenant authorization through TAuth. | The [MCP client map](mcp-clients.md) records individual builds, negotiation, authorization, and tool results. | Update that map after relevant protocol or client changes. Record successful tool execution separately from login. |

Goose has confirmed configuration support. Its LLM Proxy execution still requires integration qualification.
PAL has a proposed adapter. Its open PR does not establish an upstream release.

## Record requirements

For each consumer, record:

- The upstream repository and integration owner.
- The connection protocol and authentication method.
- The exact consumer version or commit, proxy revision, and SDK version when applicable.
- The configuration guide and upstream issue or PR.
- The test date, environment, command, result, and retained evidence link.
- The upstream merge and publication states when applicable.
- The next action and its issue ID.

Keep secrets, prompts, and generated private content outside these records.
Use exact revisions for tests. A check of `main` alone does not qualify a released consumer.

## Update procedure

M023R owns this recurring procedure.
Run it for each LLM Proxy release, SDK release, relevant protocol change, and consumer version update.
Review upstream releases monthly when no release triggers a check.

1. Compare changed interfaces with each integration record.
2. Record affected consumers and the reason for each decision.
3. Update affected adapters, package versions, configuration examples, and documentation to the current contract.
4. For Goose, review custom-provider fields, authentication, request fields, model identifiers, and tool history.
5. Run the applicable repository tests with controlled providers.
6. Run each affected real consumer against the selected proxy revision.
7. Record text, event-stream, tool-call, authorization, timeout, and error results where applicable.
8. File each reproducible defect as a separate BugFix issue.
9. Record required upstream changes with their issue or PR links.
10. Update the record and promotional claims from the observed results.

For an SDK-only change, Goose can require no source update because it uses HTTP directly.
Record that decision with the reviewed revisions and reason.
For an affected Goose contract, update its configuration guide and qualify the selected Goose build.
Obey upstream contribution requirements before submitting a Goose code or documentation change.

Mark an affected integration as pending or failed until its required checks pass.
Keep the corresponding promotional claim within the established evidence.
If a release includes an affected integration, require its successful qualification before claiming current integration support.

## Review history

| Date | Trigger | Result | Follow-up |
| --- | --- | --- | --- |
| September 29, 2026 | Goose maintainer response and initial register. | Goose configuration confirmed. PAL PR remains open with successful test, lint, and Docker checks. Consumer execution was not rerun. | I296 for Goose qualification. M023R for subsequent release checks. |
