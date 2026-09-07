# Qualified Gemini Flash Models

I207 registers `gemini-3.6-flash` and `gemini-3.7-flash` as enabled text routes.
The Gemini provider default remains `gemini-3.5-flash`.
Existing tenant selections remain unchanged.

| Model | Explicit reasoning efforts | Output limit | Context window |
| --- | --- | --- | --- |
| `gemini-3.6-flash` | `minimal`, `low`, `medium`, `high` | 65,536 tokens | 1,048,576 tokens |
| `gemini-3.7-flash` | `low`, `medium`, `high` | 65,536 tokens | 1,048,576 tokens |

Both routes use the existing `gemini_interactions` adapter and `pollable_resource` lifecycle.
The adapter sends the selected effort as `generation_config.thinking_level`.
If the request and tenant default omit the effort, the field is absent.
Google then applies its documented `medium` default.
The proxy rejects blank or undeclared efforts before provider dispatch.

Both routes accept the current public text, image, and audio message shapes.
Media requests use synchronous Interactions.
The encoded inline request limit is 20,000,000 bytes.
The image count limit is 3,600, and the declared file limit is 2,000,000,000 bytes.
The catalog marks the audio count limit as unknown.
Assistant history is rejected before dispatch.
An incomplete result returns a provider error after resource cleanup.

## Prices

As verified on September 5, 2026, both models have these standard USD rates per million tokens:

| Input | Output | Cache read | Cache storage per hour |
| --- | --- | --- | --- |
| 0.75 | 3.75 | 0.075 | 0.50 |

These prices apply through December 31, 2026.
Google lists rates of 1.50, 7.50, 0.15, and 1.00 from January 1, 2027.
The catalog records the current rates and verification date.

## Acceptance

On September 5, 2026, I233 passed each exact model in an independent paid run.
Each run passed omitted effort and every declared explicit effort.
Each run also passed background creation, active retrieval, completion, cancellation, and deletion.
The commands used the existing repository credential through the standard environment loader:

```sh
LLM_PROXY_LIVE_GEMINI_MODEL=gemini-3.6-flash make test-live-gemini-candidate LIVE_ENV_FILE=configs/.env
LLM_PROXY_LIVE_GEMINI_MODEL=gemini-3.7-flash make test-live-gemini-candidate LIVE_ENV_FILE=configs/.env
```

Local HTTP tests cover reasoning, media, structured output, incomplete results, invalid requests, and saved model selections.
Browser tests cover each model's effort selector and Settings autosave.
Deployment and production acceptance remain operator-owned.

## Sources

- [Gemini 3.6 Flash](https://ai.google.dev/gemini-api/docs/models/gemini-3.6-flash)
- [Gemini 3.7 Flash](https://ai.google.dev/gemini-api/docs/models/gemini-3.7-flash)
- [Gemini prices](https://ai.google.dev/gemini-api/docs/pricing)
- [Interactions API](https://ai.google.dev/api/interactions-api)
- [Background execution](https://ai.google.dev/gemini-api/docs/background-execution)
