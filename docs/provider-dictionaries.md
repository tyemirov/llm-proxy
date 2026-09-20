# Provider Pronunciation Dictionaries

The existing ElevenLabs definition in `configs/providers.yml` declares `pronunciation_dictionary_creation` as a provider service.
The service uses the same API key, account connection, and admission allocation as the other ElevenLabs capabilities.
It does not select a model.

## Creation

Send the following body to `POST /model/v1/operations` with a tenant bearer key and an `Idempotency-Key` header:

```json
{
  "capability": "audio.dictionary.create",
  "provider": "elevenlabs",
  "input": {
    "name": "Names",
    "description": "Narration names",
    "rules": [
      {"type": "alias", "string_to_replace": "MPR", "alias": "Marco Polo Research"},
      {"type": "phoneme", "string_to_replace": "tomato", "phoneme": "təˈmeɪtoʊ", "alphabet": "ipa"}
    ]
  },
  "controls": {}
}
```

Omit `model` for this service.
The operation API, CLI, MCP interface, and both official clients use the same request contract.
Each rule has one type. Alias rules require an alias. Phoneme rules require a phoneme and an alphabet.
The gateway rejects empty required values, unknown fields, and fields from a different rule type before native submission.

The native request uses `POST /v1/pronunciation-dictionaries/add-from-rules`.
The gateway preserves the name, description, rule order, replacement text, and pronunciation values.
It trims the name and description as the MediaOps source does.

## Result And Recovery

A successful operation publishes one JSON asset with the `MediaDictionary` schema in OpenAPI.
The artifact contains `dictionary_id`, `version_id`, `provider`, `name`, `created_by`, `creation_time_unix`, `version_rules_num`, `permission_on_resource`, and `description`.
The dictionary and version references are opaque gateway identifiers.
Native dictionary and version identifiers remain in private storage.
The record binds these references to the tenant, provider, accepted account authority, and service transport.
Dictionary records remain after terminal operation data expires.

The worker saves the native creation observation before artifact publication.
It persists the dictionary record and terminal operation result under the current worker claim.
Recovery uses a saved native observation without another creation request.
A missing or malformed observation produces an uncertain operation.
A network failure, timeout response, or server error also produces an uncertain operation without an automatic retry.
Other native client errors produce a failed operation with a sanitized error.

A repeated idempotency key returns the same accepted operation.
After operation expiry, its tombstone prevents another submission with the same intent.
Queued work can be cancelled. Running native creation does not support cancellation.

## Scope And Validation

This service implements the MediaOps `CreatePronunciationDictionaryFromRules` method.
It does not add other native dictionary methods that MediaOps does not implement.
Speech routes that consume these references remain under F026.

`make test-provider-services` uses the real HTTP API, database, workers, and clients with local native protocol servers.
The tests cover a second provider identity, exact rule payloads, private identifiers, invalid input, recovery, and retained dictionary identity.
Local tests do not establish native provider connectivity, publication, deployment, or MediaOps consumer acceptance.

Native reference: [Create a pronunciation dictionary from rules](https://elevenlabs.io/docs/api-reference/pronunciation-dictionaries/create-from-rules).
