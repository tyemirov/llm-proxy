# Vertex Gemini

## P012 Customer Acceptance Gate

P012 requires each customer to connect through an independently managed API-key flow.
The operator credential procedure below does not satisfy that requirement.
The [customer connection assessment](gemini-customer-connections.md) defines the next acceptance steps and proposed F060 revision.
P012 now records 22 successful direct Vertex API-key cases with the selected existing account.
Public proxy acceptance of that key remains pending.
Complete the P012 decision before further authentication changes or customer cutover.
The remaining sections describe the implemented operator transport and its retained evidence.

## Implemented Transport

F060 adds the `vertex` provider for Google Cloud Gemini requests.
The route uses the global `v1 generateContent` endpoint with OAuth credentials.
I252 records the earlier direct-provider qualification.
The [proxy evidence](evidence/vertex-proxy-2026-09-07.json) records 28 successful requests with the service account on September 7, 2026.
The enabled text routes are `gemini-3.5-flash`, `gemini-3.1-pro-preview`, `gemini-3.8-flash`, and `gemini-3.5-flash-lite`.
The enabled dictation route is `gemini-3.5-transcribe-preview`.
Exact Vertex prices remain unavailable in the catalog until their import.

## Runtime credentials

An operator declares each Google credential profile in the selected `config.yml`.
Each profile belongs to one tenant and one Google Cloud project.
The tenant stores the profile ID in its `credential_profile` connection field.
The service loads the credential file through the Google authentication library.
The library obtains and refreshes OAuth tokens with the `cloud-platform` scope.
Each request uses the profile project in its URL and `x-goog-user-project` header.

```yaml
google_credential_profiles:
  - id: vertex-primary
    tenant_id: REPLACE_WITH_MANAGED_TENANT_ID
    project: llm-proxy-499919
    location: global
    credentials_file: /data/credentials/llm-proxy-vertex.json
    credential_type: service_account
```

The current location contract accepts `global`.
The file path must be absolute.
The credential type must be `service_account`, `external_account`, or `impersonated_service_account`.
The operator must approve the credential file and its identity before service startup.
The profile ID must be unique.
Unknown profiles and profiles for a different tenant fail before provider dispatch.
Tenant APIs accept the profile ID only.

The acceptance identity is `llm-proxy-vertex@llm-proxy-499919.iam.gserviceaccount.com`.
It has `roles/aiplatform.user` in `llm-proxy-499919`.
The local credential file is `tmp/vertex/credentials.json` in the primary checkout.
Git excludes this file.
The operator installs the credential in the existing `/data` volume through the production secret procedure.

## Request contract

The transport maps messages to `contents` and ordered `parts`.
System messages become `systemInstruction`.
Assistant messages use the `model` role.
Media bytes use `inlineData` with their MIME type.
Reasoning uses `generationConfig.thinkingConfig.thinkingLevel`.
Structured output uses `generationConfig.responseFormat` with `text.mimeType: APPLICATION_JSON` and `text.schema`.

The service accepts one candidate with a `STOP` finish reason.
It excludes thought parts from the returned text.
Output usage includes candidate tokens and thought tokens.
`MAX_TOKENS` returns the public output-limit error.
Provider failures preserve the public error contract and keep provider response bodies private.
Client cancellation reaches the upstream request.

The local request limit is 20,000,000 encoded bytes.
The local response limit is 16 MiB.
The text offerings accept at most 3,000 images and one audio attachment per request.
Each inline image has a 7,000,000-byte limit.
F043 owns provider staging for larger assets.

Vertex dictation uses the exact model `gemini-3.5-transcribe-preview` and audio-only input parts.
Google documents this model as a preview.
The Developer API identifier `gemini-3.5-transcribe` returned HTTP 404 on Vertex.

## Tenant cutover

The production operator owns deployment and production acceptance.
Use the following explicit transition for each tenant.

1. Install the approved credential file in the service runtime.
2. Add the tenant-bound profile to the selected service configuration.
3. Run `make release`, `make publish`, and `make deploy` from the clean release checkout.
4. Open the tenant settings with its authorized management session.
5. Save the Vertex connection with the profile ID, exact text model, and required system prompt.
6. Set the tenant text default to `vertex` and its qualified model.
7. If dictation must move, select `vertex` and `gemini-3.5-transcribe-preview` for dictation.
8. Send text, structured output, image, audio, and dictation requests through the production endpoint.
9. Record the production results against F060.

The connection operation is `PUT /api/management/tenants/{tenant_id}/provider-connections/vertex`.
For example, its body can contain:

```json
{
  "fields": {"credential_profile": "vertex-primary"},
  "text_model": "gemini-3.8-flash",
  "system_prompt": ""
}
```

The service verifies the new profile with Vertex before it saves the connection.
The default operation is `PUT /api/management/tenants/{tenant_id}/defaults`.
Submit all required fields from the [OpenAPI contract](openapi.yaml), including the current system prompt and dictation selection.
For each explicit request, change the provider to `vertex` and select one of its qualified exact models.
The two provider IDs have separate credential and usage records.

## Repeat acceptance

The live test uses a temporary proxy and tenant store.
It uses the real OAuth library, service account, and Vertex endpoint.
Use a WAV file with the spoken phrase `The quick brown fox jumps over the lazy dog`.

```bash
LLM_PROXY_LIVE_VERTEX=true \
GOOGLE_APPLICATION_CREDENTIALS="$PWD/tmp/vertex/credentials.json" \
GOOGLE_CLOUD_PROJECT=llm-proxy-499919 \
LLM_PROXY_LIVE_VERTEX_AUDIO=/absolute/path/acceptance.wav \
GOFLAGS=-v make test-gemini-current
```

A failed required case stops qualification.
The ordinary CI run skips paid requests.
Local proxy acceptance and production acceptance are separate results.

## Sources

- [Google authentication library](https://pkg.go.dev/cloud.google.com/go/auth/credentials)
- [Gemini 3.8 Flash](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/3-8-flash)
- [Gemini 3.1 Pro](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/3-1-pro)
- [Gemini 3.5 Flash-Lite](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/3-5-flash-lite)
- [Gemini 3.5 Transcribe](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/3-5-transcribe)
