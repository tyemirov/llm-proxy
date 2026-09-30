# Goose configuration

Goose connects to LLM Proxy through its existing OpenAI custom provider.
The [Goose maintainer confirmed this configuration path](https://github.com/aaif-goose/goose/issues/12482#issuecomment-5902687237).
The [integration register](../integrations.md) records the source revision and qualification state.

## Prerequisites

Use a tenant client key and a configured provider connection.
Use customer-owned provider credentials for this recipe.
The current hosted contract requires request identity that this recipe does not establish.

Select an exact `provider/model` identifier from authenticated `GET /v1/models`.
For an agent session, select an offering that declares `caller_tools`.
Read its context limit from the current provider catalog.
The [client protocol contract](../client-protocols.md) defines the accepted request fields and capability limits.

## Configure Goose

1. Run `goose configure`.
2. Select `Custom Providers`, then `Add A Custom Provider`.
3. Select `OpenAI Compatible` and name the provider `LLM Proxy`.
4. Set the API URL to `https://llm-proxy-api.mprlab.com/v1/chat/completions`.
5. Enable authentication and supply the tenant client key as the API key.
6. Add the exact model identifier selected from `/v1/models`.
7. Enable event-stream support when testing the buffered event sequence.
8. Start a Goose session with the configured provider.

Goose's [configuration documentation](https://github.com/aaif-goose/goose/blob/ac15f938151bb8c0efd93ed9c2c1cf61298934bf/documentation/docs/getting-started/providers.md#configure-custom-provider) also describes JSON configuration.
On macOS and Linux, the directory is `~/.config/goose/custom_providers/`.
For a JSON configuration, use these fields:

| Field | Value |
| --- | --- |
| `name` | `llm_proxy` |
| `engine` | `openai` |
| `display_name` | `LLM Proxy` |
| `api_key_env` | `LLM_PROXY_CLIENT_KEY` |
| `base_url` | `https://llm-proxy-api.mprlab.com/v1/chat/completions` |
| `models` | Selected model records with exact `name` and catalog `context_limit`. |
| `requires_auth` | `true` |
| `supports_streaming` | `true` for buffered event tests. |

Supply `LLM_PROXY_CLIENT_KEY` through the process environment.
Start that configuration with `goose session start --provider llm_proxy`.

## Qualification

I296 owns the first executable Goose qualification.
Record the Goose build, proxy revision, selected route, configuration, commands, and safe test evidence.

1. Generate a text answer through a real Goose session.
2. Verify the buffered event sequence through Goose.
3. Execute a local file-read tool and complete the following model turn.
4. Verify missing and invalid tenant keys.
5. Verify unsupported request fields, timeouts, and provider errors through Goose.
6. Make sure diagnostic output excludes keys and private content.
7. Update the integration record with each observed result.

The proxy buffers event streams until model completion. Event-stream support does not establish incremental token delivery.
The proxy rejects sampling controls and unsupported fields before provider dispatch.
If Goose sends an unsupported field, record the failing request shape and file a focused issue.
Maintainer confirmation establishes the configuration path. It does not establish successful execution of this recipe.
