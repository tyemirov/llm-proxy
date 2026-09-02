# Provider Catalog

## Ownership

[`configs/providers.yml`](../configs/providers.yml) is the only provider catalog.
It defines all supported providers, exact models, provider offerings, managed
model migrations, controls, limits, and prices.

The loader reads `providers.yml` from the directory of the selected
`config.yml`. It parses the provider catalog before it validates service
configuration. The loader accepts only schema version 1.

The current provider catalog has these records:

- 11 provider definitions.
- 11 model publishers.
- 24 model families.
- 62 exact models.
- 63 provider offerings.
- 63 price records.
- Nine managed model migrations.
- Six protocol adapters.
- Two lifecycle values.

The application calculates a SHA-256 catalog revision from the exact file
bytes. It compiles one immutable registry from the validated snapshot.

The provider catalog contains definitions only. It never contains a credential
value, a tenant setting value, a system prompt, or a routing default.

## Root record mapping

| Provider datum | Schema location | Runtime use |
|---|---|---|
| Schema contract version | `schema_version` | Selects the only accepted parser contract. |
| Managed schema version | `model_migrations[].managed_schema_version` | Selects the database migration that consumes the record. |
| Migrated provider | `model_migrations[].provider` | Selects the persisted provider identity. |
| Migrated operation | `model_migrations[].operation` | Selects the persisted route operation. |
| Source model | `model_migrations[].source_model` | Identifies the exact persisted model value to replace or retire. |
| Target model | `model_migrations[].target_model` | References the current provider offering that replaces the source. |
| Operation identifier | `operations[].id` | Identifies `text`, `dictation`, or `video_generation`. |
| Operation inputs | `operations[].input_artifacts` | Declares accepted artifact kinds. |
| Operation outputs | `operations[].output_artifacts` | Declares result artifact kinds. |
| Publisher identity | `publishers[].id` | Owns one model publisher identity. |
| Publisher label | `publishers[].label` | Supplies the public publisher label. |
| Family identity | `families[].id` | Owns one model family identity. |
| Family publisher | `families[].publisher` | References one publisher. |
| Family label | `families[].label` | Supplies the public family label. |
| Family weight access | `families[].weight_access` | Selects `proprietary` or `open_weights`. |
| Exact model identity | `models[].id` | Owns the provider-independent model identifier. |
| Exact model publisher | `models[].publisher` | References one publisher. |
| Exact model family | `models[].family` | References one family from the same publisher. |
| Exact model version | `models[].version` | Records the exact model version. |
| Exact model operations | `models[].operations` | Declares the operations that the exact model supports. |
| Exact model media | `models[].media_inputs` | Declares the combined media set of all offerings. |

Each managed model migration belongs to one database schema version. A source
model can be a retired public identifier or an old upstream identifier. A
nonempty target must reference a current offering for the same provider and
operation. The target can be empty only when the provider is absent from the
current catalog. The migration changes selectable tenant state and preserves
historical usage records.

## Provider record mapping

Each item in `providers` is one provider definition.

| Provider datum | Schema location | Runtime use |
|---|---|---|
| Canonical provider identifier | `providers[].id` | Owns routing, persistence, and public identity. |
| Display label | `providers[].label` | Supplies the management and public label. |
| Key-acquisition URL | `providers[].key_acquisition_url` | Supplies the official HTTPS destination for the provider card. It cannot contain credentials, a query, or a fragment. |
| Request aliases | `providers[].aliases` | Resolve to the canonical provider identifier. |
| Provider fields | `providers[].fields` | Define tenant and environment connection inputs. |
| Provider transports | `providers[].transports` | Select endpoints and protocol adapters. |
| Provider offerings | `providers[].offerings` | Define the exact model routes for this provider. |

Each item in `providers[].fields` is one provider field.

| Field datum | Schema location | Runtime use |
|---|---|---|
| Field identifier | `fields[].id` | Owns the persistence and request-map key. |
| Field label | `fields[].label` | Supplies the management form label. |
| Field kind | `fields[].kind` | Selects `credential` or `setting`. |
| Value type | `fields[].type` | Selects `opaque` or `url`. |
| Requirement | `fields[].required` | Requires the value for a usable provider connection. |
| Static default | `fields[].default` | Supplies a non-tenant default value. |
| Secrecy | `fields[].secret` | Selects encrypted storage and masked output. |
| Minimum length | `fields[].validation.minimum_length` | Sets the lower text-length boundary. |
| Pattern | `fields[].validation.pattern` | Sets the exact regular-expression boundary. |
| URL schemes | `fields[].validation.allowed_schemes` | Sets the accepted URL schemes. |
| Environment name | `fields[].environment` | Maps one optional environment value to this field. |

All transport authentication fields must reference a required credential field.
The generic environment loader resolves each declared environment name. A
missing environment value does not enable that provider connection.

## Provider transport mapping

Each item in `providers[].transports` is one provider transport.

| Transport datum | Schema location | Runtime use |
|---|---|---|
| Transport identifier | `transports[].id` | Connects a provider offering to one transport. |
| HTTP method | `transports[].endpoint.method` | Selects the adapter request method. |
| Static base URL | `transports[].endpoint.default_base_url` | Supplies a catalog-owned endpoint base. |
| Tenant URL field | `transports[].endpoint.setting_field` | References a tenant-owned endpoint base. |
| Endpoint path | `transports[].endpoint.path` | Appends the adapter path to the selected base. |
| Authentication kind | `transports[].authentication.kind` | Selects bearer or direct-header authentication. |
| Authentication field | `transports[].authentication.field` | References the credential provider field. |
| Authentication header | `transports[].authentication.header` | Selects the exact HTTP header. |
| Authentication prefix | `transports[].authentication.prefix` | Supplies the value prefix for that header. |
| Static headers | `transports[].headers` | Supplies exact nonsecret headers. |
| Request adapter | `transports[].request_protocol` | Selects the request protocol adapter. |
| Response adapter | `transports[].response_protocol` | Selects the response protocol adapter. |
| Usage adapter | `transports[].usage_mapping` | Selects the usage protocol adapter. |
| Lifecycle | `transports[].lifecycle` | Selects synchronous completion or a pollable resource. |
| Visibility retry interval | `transports[].resource_visibility.retry_interval_milliseconds` | Declares the wait between created-resource visibility reads. |
| Visibility retry limit | `transports[].resource_visibility.retry_limit` | Bounds created-resource visibility retries. |
| Visibility retry statuses | `transports[].resource_visibility.retry_status_codes` | Declares the provider HTTP statuses that mean the created resource is not visible yet. |
| Upstream model field | `protocol_parameters.model_field` | Declares the upstream model field. |
| Upstream token field | `protocol_parameters.token_field` | Declares the upstream output-token field. |
| Output fields | `protocol_parameters.output_fields` | Declares the visible output locations. |
| Complete rules | `protocol_parameters.finish_rules.complete` | Declares successful terminal signals. |
| Continue rules | `protocol_parameters.finish_rules.continue` | Declares output-limit signals. |
| Continuation rules | `protocol_parameters.continuation_rules` | Declares the canonical continuation actions. |
| Error rules | `protocol_parameters.error_rules` | Declares provider failure signals. |
| Input usage field | `protocol_parameters.usage_fields.input` | Maps the provider input count. |
| Output usage field | `protocol_parameters.usage_fields.output` | Maps the provider output count. |
| Total usage field | `protocol_parameters.usage_fields.total` | Maps or derives the provider total count. |

An endpoint must use one base source. It must use either
`default_base_url` or `setting_field`.

## Protocol adapters

Protocol adapters own request serialization, response parsing, usage mapping,
and lifecycle behavior. Provider identifiers do not select protocol code.

| Adapter identifier | Accepted lifecycle |
|---|---|
| `openai_responses` | `synchronous_completion` or `pollable_resource` |
| `openai_chat_completions` | `synchronous_completion` |
| `anthropic_messages` | `synchronous_completion` |
| `gemini_interactions` | `pollable_resource` |
| `multipart_transcription` | `synchronous_completion` |
| `xai_videos_generations` | `pollable_resource` |

The shared `pollable_resource` lifecycle owns post-create observation for all
protocol adapters. Each shared text transport declares a bounded
`resource_visibility` policy. The policy lists the provider statuses that mean
a created resource is not visible yet, the retry interval, and the retry limit.
The lifecycle reads the resource immediately and applies that policy without
provider-specific control flow. The caller context bounds every wait. A status
outside the declared list or an exhausted retry limit stops the lifecycle.

The OpenAI transport allows one retry after two seconds for `403` or `404`.
The Gemini transport allows six retries at five-second intervals for `400`,
`403`, or `404`.

The schema records each adapter contract in `protocol_parameters`. Startup
compares those values with the selected protocol adapter. A mismatch stops
startup.

The completion coordinator starts a new request only when the transport
declares continuation actions. An empty `continuation_rules` list makes an
output-limit signal a provider error. The Gemini Interactions transport uses
this empty list because the public request cannot carry provider interaction
state or thought signatures.

## Provider offering mapping

Each item in `providers[].offerings` is one provider offering.

| Offering datum | Schema location | Runtime use |
|---|---|---|
| Exact model reference | `offerings[].model` | References one root exact model. |
| Provider model identifier | `offerings[].upstream_model` | Supplies the private upstream model value. |
| Transport reference | `offerings[].transport` | Selects one transport in the provider definition. |
| Supported operations | `offerings[].operations` | Declares the operations for this route. |
| Provider defaults | `offerings[].default_operations` | Selects the default offering for each provider operation. |
| Request profile | `offerings[].request_profile` | Selects one stable OpenAI Responses payload profile. |
| Web search | `offerings[].web_search` | Declares route-specific web search support. |
| Output boundary | `offerings[].output_token_limit` | Sets the public output-token boundary. |
| Reasoning adapter | `offerings[].reasoning_effort.adapter` | Selects the reusable reasoning map. |
| Reasoning values | `offerings[].reasoning_effort.efforts` | Declares the accepted public values. |
| Media input set | `offerings[].media_inputs` | Declares route-specific media inputs. |
| Media limits | `offerings[].media_limits` | Declares media admission rules and sources. |
| Request controls | `offerings[].controls` | Declares route-specific request controls. |
| Route limits | `offerings[].limits` | Declares fixed or account-dependent limits. |
| Operation prices | `offerings[].prices` | Owns one price record for each offering operation. |

The accepted request profiles are
`openai_responses_temperature`,
`openai_responses_temperature_tools`, and
`openai_responses_reasoning_tools`.

The accepted reasoning adapters are `openai_responses`,
`openai_chat_completions`, and `gemini_interactions`. Startup requires each
adapter to match its exact wire contract. Each offering declares only the
ordered effort values that its exact provider/model route accepts.

## Capability and price mapping

Each media limit uses these fields:

- `id`, `media_type`, `transport`, `status`, `value`, `unit`, and `scope`.
- `source` and `last_verified` for the official evidence.

Each control uses `id`, `kind`, `values`, `minimum`, `maximum`, and
`account_dependent`. Each limit uses `id`, `value`, `unit`, and
`account_dependent`.

Each price record uses these fields:

- `operation`, `available`, `source`, `last_verified`, and
  `unavailable_reason`.
- `rates[].component`, `currency`, `rate`, `unit`, and `conditions`.
- `minimum_charge.currency`, `amount`, and `unit`.

Price conditions use these optional fields:

- `resolution`, `generated_audio`, `input_media`, and `output_media`.
- `duration`, `quantity`, `quality`, and `mode`.
- `api_version`, `avatar_type`, `billing_mode`, and `billing_outcome`.

The runtime selects a price only for an exact component and condition match.
It never estimates a missing price.

## Data outside the provider catalog

| Data | Owner |
|---|---|
| Service limits, management settings, and database paths | `config.yml` |
| Static credential values and static setting values | Declared environment bindings |
| Tenant credential values and tenant setting values | Provider connection records |
| Selected provider text model and provider system prompt | Provider profile records |
| Tenant route defaults | Tenant records |
| HTTP request and response schemas | `docs/openapi.yaml` |
| Protocol implementation | Reusable protocol adapter code |
| Fake upstream endpoint changes | Explicit test-only endpoint controls |

The database uses `(tenant_id, provider_id, field_id)` as the provider
connection identity. It encrypts every secret value with the current
management encryption boundary. Current-schema reads use only provider
connection records and provider profile records.

The management API returns provider definitions in catalog order. Its safe
tenant profile projection includes the key-acquisition URL, offering-derived
capabilities, field definitions, selected model, provider prompt, and masked
connection state. The management app builds one provider card from each item.
It never uses key presence or usage history to define provider membership.

The card editor never requests a saved raw credential. Credential deletion
removes only encrypted credential fields. It preserves non-secret connection
fields and the provider profile.

The public capability resource omits provider fields, environment names,
authentication rules, private settings, and upstream model identifiers.

## Runtime consumers

| Consumer | Catalog use |
|---|---|
| Configuration loader | Reads the file and resolves declared environment bindings. |
| Provider registry | Compiles providers, aliases, fields, transports, offerings, and defaults. |
| Request router | Resolves a provider and exact model to one provider offering. |
| Credential verifier | Uses the selected transport and exact model. |
| Management API | Returns ordered provider-card definitions, fields, capabilities, and safe connection state. |
| Persistence layer | Validates provider and field identities before each read or write. |
| Public capability API | Publishes the safe exact model and offering projection. |
| Management UI | Builds ordered Usage Overview cards and tenant-bound editors from returned definitions. |
| Live test harness | Discovers provider environment bindings from the catalog-only CLI output. |

## Startup validation

Startup rejects these conditions:

- An unsupported schema version, an unknown field, or a second YAML document.
- A noncanonical identifier, duplicate identifier, alias collision, or duplicate environment binding.
- A missing provider field, transport, offering, operation, publisher, family, model, or price reference.
- An invalid field type, requirement, default, secrecy rule, validation rule, or environment name.
- An invalid endpoint source, URL, method, authentication rule, or static header.
- An unsupported protocol, lifecycle, request profile, or adapter contract.
- An invalid operation, default operation, capability, control, limit, or media declaration.
- A missing or duplicate provider-operation default.
- A missing, duplicate, invalid, or nonfinite price value.

## Add a provider

Use this procedure when an existing protocol adapter represents the complete
provider contract:

1. Add the publisher and model family records when they do not exist.
2. Add each new exact model to the root `models` list.
3. Add one provider definition to the root `providers` list.
4. Define every credential field and setting field in `fields`.
5. Add an environment name only when static or live-test input is necessary.
6. Define each provider transport with one supported protocol adapter.
7. Add each provider offering and reference one exact model and one transport.
8. Add one default offering for every supported provider operation.
9. Add one valid price for every offering operation.
10. Keep all credential values and tenant setting values outside the file.
11. Run `make ci` after the catalog change.
12. Run the authorized paid live gate separately when the issue requires it.

Do not change provider-specific production source for this case. The generic
consumers receive the new provider from the compiled registry.

If no adapter represents the complete contract, add one reusable protocol
adapter first. Do not approximate the provider through a different adapter.

Use this safe discovery command to inspect provider environment bindings:

```shell
go run ./cmd/cli --config configs/config.yml --provider-catalog-only
```
