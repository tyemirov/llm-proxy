# Speech Workflows

F042 moves shared speech execution from MediaOps into LLM Proxy.
LLM Proxy owns tenant assets, voices, operation state, provider calls, cancellation, and recovery.
Dictator owns speech engines and native workers.
WriterBlock is paused and is outside this migration.

## CLI

Use the `llm-proxy-client media` commands with an assigned tenant connection.
Set `LLM_PROXY_BASE_URL` to the API origin.
Set `LLM_PROXY_DEFAULT_TENANT_KEY` to the selected tenant's bearer key.
Keep provider credentials in the account connection.

| Command | Input | Result |
| --- | --- | --- |
| `capabilities` | Tenant configuration | Available media routes |
| `voices --provider dictator` | Provider ID | Tenant voice collection |
| `upload --file sample.wav --mime-type audio/wav` | Local audio file | Tenant asset metadata |
| `submit --idempotency-key extraction-001` | One operation JSON object on stdin | Accepted operation |
| `status --operation-id mop_...` | Saved operation ID | Current operation |
| `wait --operation-id mop_...` | Saved operation ID | Terminal operation |
| `cancel --operation-id mop_...` | Saved operation ID | Observed cancellation result |
| `download --asset-id ast_...` | Output asset ID | Exact bytes on stdout |

Save upload metadata and the accepted operation response.
Use the saved asset ID in the operation input.
Reuse the same idempotency key only with the same complete request.
After interruption, use `status` or `wait` with the saved operation ID.
The `--timeout` option limits the client wait. It does not cancel accepted work.
An operation response can report `failed`, `cancelled`, or `uncertain`.
Check `state` before you use its outputs.
Use `cancel` for an explicit cancellation request. Check `cancellation_state` in its response.

This extraction request preserves the MediaOps duration control:

```json
{
  "capability": "audio.voice.extract",
  "provider": "dictator",
  "model": "dictator-speech-v1",
  "input": {
    "audio_asset_id": "ast_0123456789abcdef0123456789abcdef",
    "transcript": "hello world",
    "display_name": "Narrator",
    "language": "en"
  },
  "controls": {"model_size": "base", "duration_seconds": 0.5}
}
```

`duration_seconds` must be positive. Omission selects 20 seconds.
Other speech capabilities reject this control.
The [OpenAPI contract](openapi.yaml) defines all six operation requests.
Extraction returns a tenant voice resource through an output asset.
Use its `voice_id` for subsequent synthesis.
Download each output separately. Its MIME type and ordinal identify its result role.

## MCP

The [account MCP interface](mcp.md) exposes media submission, status, and cancellation.
Each tool checks the OAuth account's ownership of `tenant_id`.
Upload source files and discover voices through the CLI or official client.
Use the resulting tenant asset and voice IDs in MCP operation inputs.
The MCP server does not read caller filesystem paths.
HTTP and MCP share the same operation store, queue, provider adapter, and idempotency contract.

## Acceptance And Remaining Migration

`make test-media-cli` checks command inputs, outputs, and failures.
`make test-mcp` runs all six speech capabilities through authenticated MCP and a local Dictator protocol server.
That test also compiles the CLI and verifies extraction, wait, and download against the same real gateway.
`make test-dictator-live` qualifies all six HTTP capabilities against the selected live provider.

These checks establish destination execution. They do not establish a production consumer switch.
MediaOps I087 still owns source caller retirement after the bounded switch and consumer acceptance.
F071 owns the remaining application migration, including browser interfaces and general composition.
The ElevenLabs render-plan workflow belongs to that separate provider and application scope.
The [migration audit](dictator-migration-audit.md) records production ownership and store evidence.
