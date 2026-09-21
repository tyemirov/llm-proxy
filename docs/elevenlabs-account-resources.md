# ElevenLabs Account Resources

The provider catalog contains one `elevenlabs` provider definition.
Its resources share one account-owned connection and one `api_key` field.
The same YAML defines the credential form, verification route, resource bindings, and native transports.
The resource adapter selects these bindings without a provider-name branch.

## Public Resources

| Method and path | Response |
| --- | --- |
| `GET /model/v1/provider-resources/{provider}/metadata` | Provider identity and account-visible model observations |
| `GET /model/v1/provider-resources/{provider}/quotas` | Provider identity and the current subscription observation |

Both requests require the tenant bearer key and an assigned provider connection.
Both responses use `Cache-Control: no-store`.
An absent binding or connection returns `404` with `provider_resource_not_found`.
An invalid native response or failed native request returns `502` with `provider_resource_unavailable`.
The gateway does not return native error bodies or credentials.

Model observations contain the native model ID, name, speech capabilities, and text limits.
These observations do not create executable routes or change the public model catalog.
Only accepted offerings in `configs/providers.yml` define executable model routes.
Unknown optional model limits use `null`.

The subscription contains the tier, status, character usage, character limit, credit extension, overage, invoice state, currency, and next reset.
The `credit_extension` object contains `unlimited` and `value`.
An unlimited extension has `unlimited: true` and `value: null`.
A finite extension has `unlimited: false` and a nonnegative integer `value`.
The overage amount remains a decimal string.
Quota observations do not replace the prices in the provider catalog.

## Native Requests

The metadata transport uses `GET /v1/models`.
The quota and credential verification transport uses `GET /v1/user/subscription`.
Both transports use the `xi-api-key` header.
The resource requests use shared upstream admission with the tenant and account connection identity.
Each resource request has a 30-second timeout and a 1 MiB response limit.
The gateway does not retry these resource requests.

The current subscription adapter reads `max_credit_limit_extension` as an integer or `"unlimited"`.
It does not read the deprecated `max_character_limit_extension` or `allowed_to_extend_character_limit` fields.
The public `can_extend_credit_limit` field uses the native `can_extend_character_limit` observation.

## Clients And Validation

The Go client supplies `GetProviderMetadata` and `GetProviderQuotas`.
The Python client supplies `get_provider_metadata` and `get_provider_quotas`.
Both clients return typed resource objects.
[OpenAPI](openapi.yaml) defines the complete response schemas.

Local HTTP tests use the real gateway and a controlled native server.
The tests cover connection replacement, tenant separation, connection removal, response limits, and sanitized failures.
A second provider identity uses the same adapters through changed YAML bindings.
The browser test creates and reloads one ElevenLabs connection and shows its declared resources.
These tests do not establish live provider connectivity or MediaOps caller acceptance.

## Remaining Migration

F026 remains open for the rest of the [source inventory](media-provider-completeness.md#elevenlabs-coverage).
This account resource slice does not implement speech, conversion, history, dictionaries, or music.
F026 adds [voice discovery and previews](provider-voices.md) through the same provider resource branch.
F079 adds [forced alignment](provider-services.md) under the same provider.
The remaining capabilities must use the same provider definition and account connection.

## Native References

- [Authentication](https://elevenlabs.io/docs/api-reference/authentication)
- [Models](https://elevenlabs.io/docs/api-reference/models/list)
- [Subscription](https://elevenlabs.io/docs/api-reference/user/subscription/get)
