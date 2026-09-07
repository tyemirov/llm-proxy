# Provider Catalog

## Ownership

[`configs/providers.yml`](../configs/providers.yml) is the only provider catalog.
It defines all supported providers, exact models, provider offerings, managed
model migrations, controls, limits, and prices.

The loader reads `providers.yml` from the directory of the selected
`config.yml`. It parses the provider catalog before it validates service
configuration. The loader accepts only schema version 1.

The current provider catalog has these records:

- 13 provider definitions.
- 12 model publishers.
- 29 model families, with 26 runtime families.
- 81 exact models, with 71 enabled models.
- 86 provider offerings, with 74 runtime offerings.
- 86 price records, with 74 runtime records.
- 13 managed model migrations.
- Ten configured request protocols.
- Two lifecycle values.

The two [GLM 5.3 candidates](zai-current-models.md) remain disabled during provider qualification.
The five [Qwen 3.8 candidates](qwen-current-models.md) remain disabled during provider qualification.
The two open models use the `qwen3-8` family with `open_weights` metadata.
Gemini 3.5 Transcribe is enabled for file dictation. See the [transcription contract](gemini-transcription.md).
Gemini 3.6 and 3.7 Flash are enabled. See [qualified Gemini models](gemini-qualified-models.md).
Gemini 3.8 Flash and 3.5 Flash-Lite are enabled on Vertex after service-account qualification.
Their Developer API offerings remain disabled.
See the [Vertex contract and cutover procedure](vertex-gemini.md).
Their [candidate contract](gemini-current-models.md) records current limits, prices, effort levels, and acceptance requirements.
The disabled [Muse Spark 1.3 candidate](meta-current-model.md) adds Standard-tier text and six reasoning effort levels.
The disabled [Grok 4.6 candidate](grok-current-model.md) adds xAI reasoning controls and current price tiers.

The application calculates a SHA-256 catalog revision from the exact file
bytes. It compiles one immutable registry from the validated snapshot.

The provider catalog contains definitions only. It never contains a credential
value, a tenant setting value, a system prompt, or a routing default.

The [Baidu Qianfan integration](baidu-qianfan.md) adds four text offerings and a catalog response policy.
The two DeepSeek V4 offerings share exact model records with the direct provider.

## Image formats and dimensions

An offering can declare `image_mime_types` to restrict the protocol adapter's image formats.
Omission uses the adapter's current format set.
The public offering includes the field when the catalog declares it.
The loader rejects duplicates, unsupported formats, and format lists without image input.

Image dimensions use `image_width_pixels` and `image_height_pixels` in `media_limits`.
Each descriptor uses media type `image`, transport `any`, unit `pixels`, and scope `attachment`.
Dimension checks currently require an explicit JPEG/PNG format list because these header decoders are available.
The loader rejects other dimension formats and invalid dimension descriptors.
The server checks inline images and tenant assets before dispatch.
It preserves the asset reader contents for the provider request.

## Provider and offering activation

A provider or offering can declare `enabled: false` to stop its runtime routes.
Omission means enabled for these two record types.
The loader validates disabled records before it removes them from the runtime catalog.
A disabled offering cannot be a provider default.
An offering requires an enabled provider, model, and offering record.
The model can remain available through another qualified provider.
This rule keeps Vertex qualification separate from Developer API qualification.

## Model activation

Each exact model must declare `enabled: true` or `enabled: false`.
The loader rejects a missing value, null, and each non-Boolean value.
The Go schema uses `ModelEnabled` and `ModelDisabled` for these explicit states.

The private catalog retains disabled models and their provider offerings, controls, limits, and prices.
Startup validates this retained metadata.
The runtime excludes disabled models, offerings, and prices from routing and discovery.
Runtime families contain only families referenced by enabled models.
The private schema retains all family metadata.
Management profiles, public capabilities, client model discovery, and standard live tests use this runtime catalog.

A provider default must reference an enabled model.
Before disabling a model with stored selections, add an explicit managed model migration to an enabled offering.
Use a new migration version for databases that completed the previous migration.
Startup rejects a stored disabled selection without its required migration.
A migration target must reference an enabled offering.

Model activation does not replace live acceptance for each provider offering and operation.
MiniMax M3 remains disabled until live text and image qualification passes.

The disposable live harness accepts `--candidate-model <provider/model>` for one disabled model.
It enables that model only in its copied catalog and selects the exact provider offering.
It rejects absent offerings, enabled models, and invalid selectors.
Candidate mode requires a disposable proxy and excludes the all-model matrix.
Use `--write-config <path>` to inspect the copy without provider calls.
Use `--media` to qualify image input through the copied catalog.
See [MiniMax M3](minimax-m3.md) for the current candidate contract and qualification command.

## Root record mapping

| Provider datum | Schema location | Runtime use |
|---|---|---|
| Schema contract version | `schema_version` | Selects the only accepted parser contract. |
| Managed schema version | `model_migrations[].managed_schema_version` | Selects the database migration that consumes the record. |
| Migrated provider | `model_migrations[].provider` | Selects the persisted provider identity. |
| Migrated operation | `model_migrations[].operation` | Selects the persisted route operation. |
| Source model | `model_migrations[].source_model` | Identifies the exact persisted model value to replace or retire. |
| Target model | `model_migrations[].target_model` | References the current provider offering that replaces the source. |
| Migration reasoning | `model_migrations[].target_reasoning_effort` | Sets the replacement text route effort when declared. |
| Historical source | `model_migrations[].preserve_source_usage` | Permits the exact source identity in historical usage records only. |
| Operation identifier | `operations[].id` | Identifies `text`, `dictation`, or `video_generation`. |
| Operation inputs | `operations[].input_artifacts` | Declares accepted artifact kinds. |
| Operation outputs | `operations[].output_artifacts` | Declares result artifact kinds. |
| Publisher identity | `publishers[].id` | Owns one model publisher identity. |
| Publisher label | `publishers[].label` | Supplies the public publisher label. |
| Family identity | `families[].id` | Owns one model family identity. |
| Family publisher | `families[].publisher` | References one publisher. |
| Family label | `families[].label` | Supplies the public family label. |
| Family weight access | `families[].weight_access` | Selects `proprietary` or `open_weights`. |
| Model activation | `models[].enabled` | Includes enabled models in runtime routing and discovery. |
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
| API service label | `providers[].api_service_label` | Supplies the authenticated provider card title. |
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
The generic environment loader resolves each declared environment name.
The live CLI discovery also publishes each field default.
The live harness uses that default when the environment supplies no value.
Required credentials have empty defaults and remain mandatory.
A setting with a default can omit its environment binding.

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
| Response policy | `protocol_parameters.response_policy` | Selects the `qianfan` response checks on the shared Chat Completions adapter. Omission uses the standard protocol policy. |
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
| `openai_responses` | `pollable_resource` |
| `xai_responses` | `synchronous_completion` |
| `dashscope_responses` | `synchronous_completion` |
| `openai_chat_completions` | `synchronous_completion` |
| `anthropic_messages` | `synchronous_completion` |
| `vertex_generate_content` | `synchronous_completion` for text and dictation. |
| `gemini_interactions` | `pollable_resource` or `synchronous_completion` for text. `synchronous_completion` for dictation. |
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
| Request profile | `offerings[].request_profile` | Selects one stable protocol-specific payload profile. |
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
`openai_responses_temperature_tools`,
`openai_responses_reasoning_tools`, and `minimax_chat_completions`.
The three OpenAI profiles require `openai_responses`. The MiniMax profile
requires `openai_chat_completions` and sends `reasoning_split: true`.
Generation and credential verification use the same profile.

The accepted reasoning adapters are `xai_responses`, `openai_responses`,
`openai_chat_completions`, `chat_completions_thinking`, `gemini_interactions`, `vertex_generate_content`, and `anthropic_messages`. Startup requires each
adapter to match its exact wire contract. Each offering declares only the
ordered effort values that its exact provider/model route accepts.

The `xai_responses` reasoning adapter sends `reasoning.effort` on the xAI Responses route.
It supports `low`, `medium`, `high`, and `xhigh` when the offering declares those values.
It omits the field when the caller does not supply an effort.

The `chat_completions_thinking` adapter maps `none` to `thinking.type: disabled`.
It maps `low`, `high`, and `max` to enabled thinking and the matching `reasoning_effort`.
An omitted effort enables thinking and uses the provider default.
DeepSeek V4 Flash and V4 Pro use this adapter.

The `anthropic_messages` reasoning adapter maps effort to `output_config.effort`.
It preserves `output_config.format` when the request also requires structured output.
An omitted effort preserves the provider default.
Claude Fable 5.1 and Opus 5 declare `low`, `medium`, `high`, `xhigh`, and `max`.
See [current Claude models](claude-current-models.md) for limits, prices, retention requirements, and qualification.

See [DeepSeek retirement](deepseek-retirement.md) for the schema-version-14 migration and its operator checks.

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
tenant profile projection includes the API service label and the key-acquisition
URL. It also includes offering-derived model families and capabilities.
Capabilities include operations and accepted media inputs. The projection
includes field definitions, the selected model,
the provider prompt, and masked connection state. The management app builds one
provider card from each item. It never uses key presence or usage history to
define provider membership.

The DashScope catalog declares image input for `qwen3.7-plus` and
`qwen3.6-flash`. These models accept image content and return text.
Each route accepts at most 250 images and 20,000,000 bytes per complete image Data URI.
The `attachment_data_uri_bytes` scope includes the URI prefix, MIME type, and Base64 content.
The
`qwen-plus` and `qwen3.7-max` aliases accept text only. The management UI labels
image input as `Image analysis`. It does not present image input as an image
generation capability.

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
2. Add each new exact model to the root `models` list with an explicit `enabled` value.
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

## Gemini media request limits

All six Gemini text offerings declare `inline_request_bytes` at 20,000,000 encoded bytes for image and audio requests.
This includes the enabled `gemini-3.5-flash` and `gemini-3-flash-preview` offerings and the two disabled F047 candidates.
Both the [image guide](https://ai.google.dev/gemini-api/docs/image-understanding) and [audio guide](https://ai.google.dev/gemini-api/docs/audio) state this total request bound.
The [general file guide](https://ai.google.dev/gemini-api/docs/file-input-methods) lists 100 MB but states that limits can vary by file type and model.
The catalog uses the media-specific bound for the input types that these routes support.
The Files API handles attachments when the encoded request exceeds that bound.
The catalog records the image guide as the common bound's source, verified on September 5, 2026.
See [current Gemini candidates](gemini-current-models.md) for qualification state.

## SiliconFlow expansion assessment

P010 records six proposed SiliconFlow offerings, exact dated DeepSeek selectors, and conflicting provider limits.
See the [SiliconFlow expansion assessment](siliconflow-expansion.md) for the acceptance sequence and current credential blocker.

## Meta media assessment

See [Muse Spark 1.3](meta-current-model.md) for the enabled text offering and live acceptance.

P009 separates file dictation, image operations, and realtime transcription.
See the [Meta media assessment](meta-media-assessment.md) for exact endpoints, limits, retention gaps, and proposed acceptance requirements.

See [OpenAI transcription retirement](openai-transcription-retirement.md) for the schema-version-16 migration and operator acceptance.

See [GPT-6 Astra](astra.md) for the enabled Responses offering, limits, price tiers, and live acceptance.

## Google credential profiles

The `google_credentials` authentication kind resolves a tenant-bound operator profile.
The public credential kind is `google_credential_profile`.
The `vertex_generate_content` protocol uses synchronous completion and the Google OAuth library.
The [Vertex contract](vertex-gemini.md) defines the configuration, request mapping, limits, and tenant cutover.
