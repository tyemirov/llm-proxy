# Provider Services

`configs/providers.yml` is the source for model offerings, account resources, and provider services.
A service is an operation that does not select a model.
Each `providers[].services[]` entry declares an operation, transport, controls, limits, and price observation.
The transport uses the same provider fields and account connection as its model offerings.
The parser rejects an unsupported service and transport combination.

Public and management provider objects include a `services` array.
These objects exclude private transport identifiers and authentication settings.
Tenant discovery at `GET /model/v1/capabilities` includes available services separately from model routes and account resources.
The website and connection details show the declared services without additional model families.

## Forced Alignment

The ElevenLabs provider declares `audio_alignment` with the `elevenlabs_alignment` protocol component.
The durable operation API uses this request:

```json
{
  "capability": "audio.align",
  "provider": "elevenlabs",
  "input": {
    "audio_asset_id": "ast_0123456789abcdef0123456789abcdef",
    "transcript": "The words in this recording."
  },
  "controls": {}
}
```

Send this body to `POST /model/v1/operations` with the tenant bearer key and an `Idempotency-Key` header.
Omit `model` to select a declared service. An explicit empty or null model is invalid.
Model offerings continue to require their exact model identifier.
A service response omits `model`.
The Go client uses an empty `MediaOperationInput.Model` to omit that field.
The Python client uses `ClientMediaOperationInput(model=None, ...)`.
MCP uses the same operation and catalog binding.
The CLI `media submit` command preserves model presence and rejects invalid values before HTTP submission.

The adapter streams the owned audio asset and transcript to the configured native endpoint.
The current native contract is `POST /v1/forced-alignment` with multipart fields `file` and `text`.
It does not send a native model identifier.
The effective input limit is the lower of the catalog service limit and the proxy asset limit.
The catalog byte limit is 1,000,000,000 bytes.
The catalog also declares the provider duration bound of 36000 seconds.
Financial admission reserves this duration because the proxy does not decode audio duration before submission.
The alignment catalog requires both limits. A lower duration bound cannot limit the actual provider work.

The output is one tenant-owned JSON asset with `characters`, `words`, and an optional `loss`.
Each segment has `text`, `start`, and `end`. Optional segment loss values remain in the output.
Word entries without a Unicode letter or number are removed.
Character entries remain unchanged.
Malformed timing, missing required fields, negative loss, and an output without spoken words produce an uncertain operation.
Unknown native fields are not copied into the public artifact.

The service uses shared account admission and credential version checks.
Provider usage includes service operations. Model usage does not contain a fabricated model bucket.
Accepted input assets remain referenced while work is active.
A repeated idempotency key with the same intent returns the same operation.
An uncertain submission is never sent again during recovery.
Native alignment has no recovery or cancellation endpoint in this contract.
The adapter saves the validated output, provider trace, and request interval before result publication.
The private receipt uses adapter revision `elevenlabs_alignment:2` in the execution binding.
After a failed publication, a replacement worker validates this receipt and restores the output without another provider request.
An absent or invalid receipt leaves the outcome uncertain. A failed receipt write does not publish the output.
The provider trace is a private journal identifier. The receipt and request interval remain outside public responses.
Queued work can be cancelled. Running work reports unsupported cancellation.

Hosted execution reads native usage with the saved trace and request interval before result publication.
Empty usage rows keep the operation pending. One measured request supplies the exact input duration.
Shutdown preserves a valid receipt and releases the worker claim for recovery.
The replacement worker reads usage without another paid submission. Ledger keeps the funds reservation until settlement.
See [Hosted provider services](hosted-billing.md#hosted-provider-services) for the native measurement and price contracts.

## Validation

`make test-provider-services` exercises the real HTTP API, clients, database, assets, account credentials, and workers with local native servers.
It checks a second provider identity, exact multipart submission, output timing, usage, invalid requests, cancellation, and recovery without resubmission.
Browser tests check public service discovery and one ElevenLabs connection.
These tests do not establish native provider connectivity, client publication, deployment, or MediaOps consumer acceptance.

Pronunciation dictionary creation uses the same service contract.
See [Provider pronunciation dictionaries](provider-dictionaries.md) for its request, retained references, and recovery rules.

The remaining ElevenLabs speech, conversion, voice-library, history, and music operations remain under F026.
Their model routes and services must extend the same provider definition.

Native reference: [Create forced alignment](https://elevenlabs.io/docs/api-reference/forced-alignment/create).
