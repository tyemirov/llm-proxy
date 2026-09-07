# Gemini Qualification On Vertex AI

P012 supersedes the customer migration recommendation in this earlier transport assessment.
The [customer connection assessment](gemini-customer-connections.md) requires independent API-key setup before the support decision.
The results below remain evidence for the tested OAuth transport only.

I252 completed direct Vertex qualification on September 7, 2026.
All 22 requests returned HTTP 200 with the expected result and `STOP` finish reason.
The requests used the existing `llm-proxy-499919` Google Cloud project and the global endpoint.

## Project And Authentication

The project was active and already had billing enabled.
The Vertex API, `aiplatform.googleapis.com`, was disabled.
I252 enabled that API and verified its enabled state.

The local Google Cloud account and Application Default Credentials could both obtain access tokens.
The installed CLI has no `gcloud ai model-garden models describe` command.
The Model Garden list command first failed because its quota project was absent.
The explicit `--billing-project=llm-proxy-499919` option corrected that query.
Model generation supplied the same project through `x-goog-user-project`.

The generation probes used the existing local Google Cloud user identity.
They obtained short-lived OAuth tokens through `gcloud auth print-access-token`.
The probes kept tokens in memory and excluded them from the evidence.
These checks prove development access, not a production runtime identity.

## Results

Each text case required the exact answer `OK`.
The JSON case required an object with `answer` equal to `OK`.
The image case required correct recognition of a generated red PNG fixture.
The audio case required the exact words from a generated speech fixture, with case and punctuation normalization.
Each request had a 45-second deadline and a 4,096-token output limit.

| Exact model | Text reasoning cases | JSON | Image | Audio | Passed | Request latency |
|---|---|---|---|---|---:|---|
| `gemini-3.1-pro-preview` | Omitted, low, medium, high | Pass | Pass | Pass | 7/7 | 2.277–4.063 seconds |
| `gemini-3.8-flash` | Omitted, low, medium, high | Pass | Pass | Pass | 7/7 | 1.571–2.871 seconds |
| `gemini-3.5-flash-lite` | Omitted, minimal, low, medium, high | Pass | Pass | Pass | 8/8 | 0.486–1.178 seconds |

No request returned a quota error or exceeded its deadline.
The latency values describe these small sequential fixtures, not production load or a service guarantee.
Provider usage and per-request results are in the [qualification evidence](evidence/vertex-gemini-2026-09-07.json).

## Interpretation

Vertex accepted the same Pro model that had zero free-tier quota through the Developer API credential.
It also accepted the Flash reasoning cases and Flash-Lite text and media cases that blocked the earlier qualification.
This evidence supports the Vertex migration for these exact models and tested capabilities.
The previous Developer API results remain valid evidence for their original route and credential.

The tested endpoint is:

```text
POST https://aiplatform.googleapis.com/v1/projects/llm-proxy-499919/locations/global/publishers/google/models/{model}:generateContent
```

Google's Vertex Interactions reference still labels that API experimental.
F060 must use the tested `v1 generateContent` contract for the Gemini text migration.
The Developer API Interactions status does not establish the Vertex Interactions status.

## Required Proxy Changes

F060 owns the Gemini text transport migration.
F043 owns the typed Google credential profiles needed by that transport.

1. Add the Google runtime credential profile with explicit project, location, and service-account or workload-identity references.
2. Obtain and refresh scoped OAuth tokens through the Google authentication library.
3. Implement the native `contents`, `parts`, `generationConfig`, `candidates`, and `usageMetadata` mappings.
4. Map reasoning levels to `generationConfig.thinkingConfig.thinkingLevel`.
5. Preserve structured output, media order, deadlines, output-limit errors, and usage accounting through the public proxy APIs.
6. Qualify each existing Gemini offering that the migration affects, including separate dictation work where applicable.
7. Migrate tenant credentials and routes through one explicit transition into the Vertex contract.
8. Run proxy acceptance and final CI before the operator production cutover.

F060 adds the separate Vertex provider and its Google credential profiles.
See the [Vertex contract and cutover procedure](vertex-gemini.md).
I252 changed the Cloud API state and qualification documents, not proxy routing or tenant credentials.
Production cutover and acceptance remain operator work.

## Sources

- [Google Cloud migration guide](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/migrate/migrate-google-ai)
- [Gemini 3.8 Flash guide and model comparison](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/guides/gemini-3-8-flash)
- [Vertex generation request and response contract](https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/models/inference)
- [Vertex Interactions API status](https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/models/interactions-api)
