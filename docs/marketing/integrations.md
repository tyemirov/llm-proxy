# Integration promotional materials

The [integration register](../integrations.md) owns integration status and evidence.
Update this copy after each affected integration review.

## Goose copy

Status: configuration path confirmed by a Goose maintainer. Execution qualification remains under I296.

### Product description

Connect Goose to LLM Proxy through Goose's OpenAI custom provider.
Use one tenant client key and an exact model identifier.
LLM Proxy keeps provider credentials on the server and gives Goose a shared model access point.
See the [configuration guide](../integrations/goose.md) and its current qualification status.

### Social post

Goose can connect to LLM Proxy through its OpenAI custom provider. Use a tenant key and an exact model identifier. Provider credentials stay on the proxy. A Goose maintainer confirmed the setup path. Execution qualification remains open.

### Claim updates

After I296 passes, add the tested Goose version and a link to its execution evidence.
Describe successful text, event-stream, and tool behavior only when those checks pass.
Keep PAL promotion at the proposed-integration stage until its upstream PR merges.
