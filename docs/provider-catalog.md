# Provider Catalog

## Ownership

[`configs/providers.yml`](../configs/providers.yml) is the only provider catalog.
It defines all supported providers, exact models, provider offerings, managed
model migrations, controls, limits, and prices.

The loader reads `providers.yml` from the directory of the selected
`config.yml`. It parses the provider catalog before it validates service
configuration. The loader accepts only schema version 6.

The catalog defines provider declarations, exact models, offerings, prices, and managed model migrations.
Read the YAML for the complete inventory. Use public capability discovery for enabled routes.
Disabled entries remain validated source declarations. They do not establish runtime availability.

The two [GLM 5.3 candidates](zai-current-models.md) remain disabled during provider qualification.
[Qwen 3.8 Flash](qwen-current-models.md) is enabled after live text, reasoning, image, and usage qualification.
The other four Qwen 3.8 offerings remain disabled.
The two open models use the `qwen3-8` family with `open_weights` metadata.
Gemini 3.5 Transcribe is enabled for file dictation. See the [transcription contract](gemini-transcription.md).
Gemini 3.6 and 3.7 Flash are enabled. See [qualified Gemini models](gemini-qualified-models.md).
Gemini 3.8 Flash and 3.5 Flash-Lite are enabled on Vertex after service-account qualification.
Their Developer API offerings remain disabled.
See the [Vertex contract and cutover procedure](vertex-gemini.md).
Their [candidate contract](gemini-current-models.md) records current limits, prices, effort levels, and acceptance requirements.
The enabled [Muse Spark 1.3 offering](meta-current-model.md) adds Standard-tier text and six reasoning effort levels.
The disabled [Grok 4.6 candidate](grok-current-model.md) adds xAI reasoning controls and current price tiers.

The application calculates a SHA-256 catalog revision from the exact file
bytes. It compiles one immutable registry from the validated snapshot.

The provider catalog contains definitions only. It never contains a credential
value, a tenant setting value, a system prompt, or a routing default.

The [Baidu Qianfan integration](baidu-qianfan.md) adds four text offerings and a typed protocol variation.
The two DeepSeek V4 offerings share exact model records with the direct provider.

## Alibaba Cloud connection labels

The `dashscope` provider appears as **Alibaba Cloud** in connection setup and public model discovery.
The provider catalog supplies the labels and the English [API-key setup link](https://www.alibabacloud.com/help/en/model-studio/get-api-key).
The connection fields are **Alibaba Cloud API key** and **Alibaba Cloud API URL**.
Qwen remains the model family name.
The provider identifier, `dashscope_responses` codec, and `DASHSCOPE_*` environment variables retain their technical names.

The setup guide includes a separate procedure for US (Virginia).
F064 tracks US East endpoint support and the regional model inventory.

## DashScope structured output

An offering can declare `structured_output: true` for a qualified synchronous
DashScope Responses text route. The committed catalog declares it only for
`qwen3.8-flash`; sharing that wire contract does not enable other offerings.
Public discovery adds `structured_output` to that offering's capability list.
The request boundary validates the provider schema subset and requires text-only
messages. Image input and structured output are separate capabilities that cannot
be combined on this route.
See the [strict schema contract](dashscope-responses.md#structured-generation).

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
The runtime excludes a model when its last active provider offering is disabled.
This rule keeps Vertex qualification separate from Developer API qualification.

## Model activation

Each exact model must declare `enabled: true` or `enabled: false`.
The loader rejects a missing value, null, and each non-Boolean value.
The Go schema uses `ModelEnabled` and `ModelDisabled` for these explicit states.

The private catalog retains disabled models and their provider offerings, controls, limits, and prices.
Startup validates this retained metadata.
The runtime excludes disabled models, offerings, and prices from routing and discovery.
Runtime families contain only families referenced by runtime models with active provider offerings.
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
| Operation identifier | `operations[].id` | Identifies one text, dictation, video, or durable speech operation. |
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
| Connection ownership | `providers[].connection_ownership` | Selects a tenant-managed connection or deployment-owned runtime configuration. |
| Key-acquisition URL | `providers[].key_acquisition_url` | Supplies the official HTTPS destination for a tenant-owned provider card. Deployment-owned providers omit it. |
| Request aliases | `providers[].aliases` | Resolve to the canonical provider identifier. |
| Provider fields | `providers[].fields` | Define tenant and environment connection inputs. |
| Provider transports | `providers[].transports` | Select endpoints and reusable transport components. |
| Provider offerings | `providers[].offerings` | Define the exact model routes for this provider. |

Each item in `providers[].fields` is one provider field.

| Field datum | Schema location | Runtime use |
|---|---|---|
| Field identifier | `fields[].id` | Owns the persistence and request-map key. |
| Field label | `fields[].label` | Supplies the management form label. |
| Field kind | `fields[].kind` | Selects `credential` or `setting`. |
| Value type | `fields[].type` | Selects `opaque`, `url`, `grpc_target`, or `boolean`. |
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
Every deployment-owned field is required and has an environment binding.
Deployment-owned providers do not appear in tenant connection management.

## Connection verification

Each provider declares one `verification` binding in the provider catalog.
The binding references a transport in that provider definition.
Text verification also references a declared exact model through `verification.model`.
If that model or offering is disabled, connection verification returns an unavailable error without an upstream request.
Resource verification omits the model field.
The loader rejects absent bindings, dangling references, and unsupported protocol combinations.

Connection creation and credential replacement use this binding.
All capability branches retain the same provider fields and tenant assignments.
A text default change does not change the declared verification model.

The `json_resource` codec uses HTTP GET and the `read_only` lifecycle.
Its response must have a successful HTTP status and contain one JSON object.
The request uses the transport's declared authentication and the shared account credential.
The verifier bounds response size and request duration.
Provider rejection, rate limits, and transport errors use the existing connection error contract.
The Dictator binding uses its existing authenticated gRPC discovery check.
New media-only providers do not require text offerings or paid generation for connection verification.

Schema version 5 replaces version 4. The loader rejects the previous schema.
Provider resource APIs remain separate from model offerings. F077 owns their typed catalog bindings.

## Provider transport mapping

Each item in `providers[].transports` is one provider transport.

| Transport datum | Schema location | Runtime use |
|---|---|---|
| Transport identifier | `transports[].id` | Connects a provider offering to one transport. |
| Transport protocol | `transports[].endpoint.protocol` | Selects `http` or `grpc`. |
| HTTP method | `transports[].endpoint.method` | Selects the adapter request method. |
| Static base URL | `transports[].endpoint.default_base_url` | Supplies a catalog-owned endpoint base. |
| Endpoint setting field | `transports[].endpoint.setting_field` | References a URL or gRPC target from the provider connection owner. |
| Endpoint path | `transports[].endpoint.path` | Appends the adapter path to the selected base. |
| Static headers | `transports[].headers` | Supplies exact nonsecret headers. |
| Request codec | `transports[].components.request_codec.id` | Selects request serialization and request-field rules. |
| Request variation | `transports[].components.request_codec.variation` | Selects one typed request-codec difference. |
| Response codec | `transports[].components.response_codec.id` | Selects response, finish, continuation, error, and usage rules. |
| Response variation | `transports[].components.response_codec.variation` | Selects one typed response-codec difference. |
| Authentication kind | `transports[].components.authentication.kind` | Selects HTTP bearer, direct-header, or gRPC bearer credential injection. |
| Authentication field | `transports[].components.authentication.field` | References the credential provider field. |
| Authentication header | `transports[].components.authentication.header` | Selects the exact HTTP header. |
| Authentication prefix | `transports[].components.authentication.prefix` | Supplies the value prefix for that header. |
| Execution lifecycle | `transports[].components.execution.id` | Selects synchronous completion, a pollable resource, or an asynchronous job. |
| Visibility retry interval | `transports[].components.execution.resource_visibility.retry_interval_milliseconds` | Declares the wait between created-resource visibility reads. |
| Visibility retry limit | `transports[].components.execution.resource_visibility.retry_limit` | Bounds created-resource visibility retries. |
| Visibility retry statuses | `transports[].components.execution.resource_visibility.retry_status_codes` | Declares the provider HTTP statuses that mean the created resource is not visible yet. |

An HTTP endpoint must use one base source: `default_base_url` or
`setting_field`. A gRPC endpoint uses one `grpc_target` setting field and has no
HTTP method, path, or base URL.

## Transport components

Request codecs own serialization and request-field rules. Response codecs own
response parsing, finish and continuation decisions, public error mapping, and
usage mapping. Authentication components inject one stored credential through
bearer or direct-header authentication. Execution components own synchronous
completion or resource polling. Provider identifiers do not select component
code.

The startup composer accepts these codec and lifecycle combinations:

| Codec identifier | Accepted lifecycle |
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
| `dictator_speech_v1` | `asynchronous_job` |

The three Responses identifiers select closed protocol variations in one shared implementation.
They identify wire behavior, independent of provider identity.
OpenAI keeps background execution, polling, response synthesis, and its current text extraction rules.
xAI keeps synchronous completion, image detail, caller tools, and reason-specific output-limit continuation.
DashScope keeps synchronous completion, images without detail, and continuation after any error-free terminal `incomplete` result.
Both synchronous variations send `store:false` and omit `background`.
Another provider with an existing variation uses that catalog identifier and its own connection values.
The shared protocol tests exercise each variation through a second provider definition.

The shared `pollable_resource` execution component owns post-create
observation for every compatible codec. Each shared text transport declares a
bounded `components.execution.resource_visibility` policy. The policy lists the provider statuses that mean
a created resource is not visible yet, the retry interval, and the retry limit.
The lifecycle reads the resource immediately and applies that policy without
provider-specific control flow. The caller context bounds every wait. A status
outside the declared list or an exhausted retry limit stops the lifecycle.

The OpenAI transport allows one retry after two seconds for `403` or `404`.
The Gemini transport allows six retries at five-second intervals for `400`,
`403`, or `404`.

The catalog does not repeat fixed codec behavior. Startup composes the selected
request codec, response codec, authentication component, and execution
component once. It rejects unknown components, unsupported variations,
incompatible codec pairs, missing required static headers, and unsupported
codec-lifecycle combinations.

The Chat Completions request codec requires one of these variations:

- `max_tokens`
- `max_completion_tokens`

Its response codec optionally selects the `qianfan` variation. The multipart
transcription request codec requires `model` or `model_omitted`. Other current
codecs do not accept a variation.

The completion coordinator starts a new request only when the codec definition
includes continuation actions. A codec without these actions returns an
output-limit signal as a provider error. Gemini Interactions has no continuation
actions because the public request cannot carry provider interaction state or
thought signatures.

## Provider offering mapping

Each item in `providers[].offerings` is one provider offering.

| Offering datum | Schema location | Runtime use |
|---|---|---|
| Exact model reference | `offerings[].model` | References one root exact model. |
| Provider model identifier | `offerings[].upstream_model` | Supplies the private upstream model value. |
| Transport reference | `offerings[].transport` | Selects one transport in the provider definition. |
| Image editing transport | `offerings[].image_routes.editing` | Selects the Images editing transport for the same image offering. |
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

An image offering with `image_editing` must declare `image_routes.editing`.
The reference must select a synchronous `openai_images` transport in the same provider.
The offering also declares `input_images`, `input_image_bytes`, and `input_image_pixels` limits.
The last limit bounds decoded input memory in the gateway.
The loader rejects absent references, incompatible components, and image routes without the corresponding operation.
Image offerings declare the Boolean `stream` control and the integer `partial_images` control.
The current codec accepts zero through three previews and one streamed output.
The `stream_output_images` limit declares that output bound.
Requests with previews must enable streaming. Terminal requests retain the separate `output_count` range.
The required `surface` enum declares `images` and any configured `responses` route.
`image_routes.responses` selects an `openai_responses` transport with the `pollable_resource` lifecycle.
Its `responses_model` enum references enabled text offerings with image input support on that transport.
The codec resolves each selected text model to its catalog upstream model.
The `responses_output_images` limit is one. The request forces one image tool call.
Responses masks are not part of the current image contract.
The loader rejects unknown models, disabled models, wrong transports, and inconsistent surface declarations.


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
The `number` control kind permits decimal bounds, such as a speed range from `0.7` to `1.2`.
Both numeric bounds must be finite, and the minimum must not exceed the maximum.
The `integer` kind requires nonnegative whole-number bounds.
Fractional integer bounds are invalid and must not be rounded or truncated.
Number and integer controls use empty `values` arrays in public discovery.
Provider adapters must use the declared bounds when they validate operation controls.


Each price record uses these fields:

- `operation`, `available`, `source`, `last_verified`, and
  `unavailable_reason`.
- `rates[].component`, `currency`, `rate`, `unit`, and `conditions`.
- `minimum_charge.currency`, `amount`, and `unit`.

Price conditions use these optional fields:

- `resolution`, `generated_audio`, `input_media`, and `output_media`.
- `duration`, `quantity`, `quality`, and `mode`.
- `api_version`, `avatar_type`, `billing_mode`, and `billing_outcome`.
- `input_tokens`, `cache_class`, `service_tier`, and `region`.
- `effective_from` and `effective_until`.

The runtime selects a price only for an exact component and condition match.
It never estimates a missing price.

The typed selector also matches the input token count and acceptance time.
The `input_tokens.minimum` value is inclusive. A nonzero `maximum_exclusive` value is exclusive.
A zero maximum means that the range has no upper bound.
Public JSON represents token boundaries as integer strings.
An `unresolved_reason` preserves a published boundary that cannot be converted to an exact token count.
Such a boundary cannot authorize paid work.

Cache classes distinguish reads, writes, five-minute writes, one-hour writes, and storage.
Service tiers and regions must match the selected conditions exactly.
The loader rejects overlapping token and time intervals for the same component and categorical conditions.
Effective times use UTC. The start is inclusive and the end is exclusive.
New hosted admission requires an explicit start and rejects an inactive interval.
Retained snapshots use their acceptance time when they validate historical prices.

The initial local effective start is midnight UTC on the recorded verification date.
This start defines local catalog eligibility. It does not claim the provider first published a rate at that time.
An explicit promotional end separates the promotional interval from the following list-price interval.

Gemini 3.6, 3.7, and 3.8 Flash rates have a local eligibility end of `2027-01-01T00:00:00Z`.
The [official Google prices](https://ai.google.dev/gemini-api/docs/pricing) publish higher rates from January 1, 2027.
The source gives a date without a time zone. The catalog end follows the local UTC eligibility convention.
The catalog does not yet authorize the later rates. After this boundary, new paid admission needs a verified replacement schedule.
Accepted snapshots retain their original rates.

Media offerings can declare fixed billing limits in `limits` with identifiers from their native usage quantities.
For example, `audio_seconds` uses `seconds`, and `character_cost` uses `provider_units`.
Token quantities use `tokens`. Each bound applies to one provider attempt.
These limits describe verified usage ceilings. They do not define new request controls.
Capability validation continues to require the route's control limits.
Hosted admission requires a fixed limit with the matching unit for each priced native dimension.
An unknown or account-dependent limit cannot authorize paid work.

Rates and minimum charges use exact decimal strings in YAML and public JSON.
Each amount has at most 128 characters and no exponent, leading zeros, or trailing fractional zeros.
Zero is `"0"`. A rate of one quarter is `"0.25"`.
The loader rejects numeric amount fields and invalid decimal strings.
The catalog and hosted rating share this amount type. Neither requires a floating-point conversion.

## Data outside the provider catalog

| Data | Owner |
|---|---|
| Service limits, management settings, and database paths | `config.yml` |
| Static credential values and static setting values | Declared environment bindings |
| Tenant credential values and tenant setting values | Provider connection records |
| Selected provider text model and provider system prompt | Provider profile records |
| Tenant route defaults | Tenant records |
| HTTP request and response schemas | `docs/openapi.yaml` |
| Request and response implementation | Reusable codec code |
| Credential injection | Reusable authentication component code |
| Synchronous and pollable execution | Reusable lifecycle component code |
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

The Alibaba Cloud catalog declares image input for `qwen3.7-plus` and
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
- An unsupported codec, codec variation, authentication kind, lifecycle, request profile, or component combination.
- An invalid operation, default operation, capability, control, limit, or media declaration.
- A missing or duplicate provider-operation default.
- A missing, duplicate, invalid, or nonfinite price value.

## Add a provider

Use this procedure when existing transport components represent the complete
provider contract:

1. Add the publisher and model family records when they do not exist.
2. Add each new exact model to the root `models` list with an explicit `enabled` value.
3. Add one provider definition to the root `providers` list.
4. Define every credential field and setting field in `fields`.
5. Add an environment name only when static or live-test input is necessary.
6. Define each provider transport with request codec, response codec, authentication, and execution component references.
7. Select a codec variation only when that codec requires it.
8. Add each provider offering and reference one exact model and one transport.
9. Add one default offering for every supported provider operation.
10. Add one valid price for every offering operation.
11. Keep all credential values and tenant setting values outside the file.
12. Run `make test-protocol-acceptance` after the catalog change.
13. Run `make ci` after the local qualification passes.
14. Run the authorized paid live gate separately when the issue requires it.

Do not change provider-specific production source for this case. The generic
consumers receive the new provider from the compiled registry.

If no supported component composition represents the complete contract, add
the missing reusable component and its explicit composition contract first. Do
not approximate the provider through a different composition.

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

## Vertex API Keys

The `vertex_generate_content` protocol uses synchronous completion with a customer API key.
Its authentication kind is `header`, and its public credential kind is `api_key`.
The catalog selects `x-goog-api-key` and the key-only model endpoint.
The [Vertex contract](vertex-gemini.md) defines tenant connections, request mapping, limits, and the operator-profile transition.

## Provider Resources

`providers[].resources[]` binds account resources to declared transports.
Each binding contains `kind` and `transport`. It has no model field.
A provider can declare resources without model offerings.
The provider fields and account connection remain shared across all bindings.

The resource vocabulary contains `voices`, `voice_library`, `history`, `pronunciation_dictionaries`, `metadata`, `quotas`, and `elements`.
The catalog rejects unknown kinds, duplicate kinds, and dangling transport references.
It also rejects a kind and codec pair without an implemented adapter.
The current `voices` binding uses `dictator_speech_v1` and the existing tenant voice API.
The `metadata` binding uses `elevenlabs_models`.
The `quotas` binding uses `elevenlabs_subscription`.
Both resources use authenticated HTTP GET requests through the same provider connection.
See [ElevenLabs account resources](elevenlabs-account-resources.md) for the public representations and limits.
F026 and F027 retain the remaining native resource adapters.
A vocabulary entry alone does not advertise an implemented resource.

Public and management provider discovery include a `resources` array of declared resource kinds.
Connection details show these resources even when the provider has no model offerings.
Tenant media discovery includes `resources` entries with `provider` and `kind`.
Tenant discovery includes only resources with the required connection fields.
These representations exclude private transport names, credentials, and upstream identifiers.
The Go and Python clients require the current resource discovery shape.

Voice discovery uses the declared transport without a default speech model.
Preset voices do not require model provenance. Extracted voices retain their source model privately.
Native synthesis-engine checks remain inside the speech adapter.

## Queue Image Transport

The `fal_queue_images` codec uses the `asynchronous_job` lifecycle.
Its endpoint path is `/{model}`. The offering supplies the native model path.
Its `artifact_origins` list declares exact HTTP origins for result downloads.
Use HTTPS origins, except for loopback origins in local tests.
Other codecs reject this field until they implement its transfer contract.
The shared admission configuration must assign capacity to every declared origin.

The `key` authentication component requires the `Authorization` header and the `Key ` prefix.
It references a shared credential field inside the provider definition.
The account verification transport and image transport use this same field.
See [FAL image operations](fal-image-operations.md) for request, recovery, and artifact contracts.

## Model-Free Services

`providers[].services[]` declares operations that do not select a model.
Each entry references a transport in the same provider and declares controls, limits, and a price observation.
Public, tenant, and management discovery expose these services without additional model identities.
See [Provider services](provider-services.md) for forced alignment, client requests, and recovery.

## Text provider transports

Provider selectors and aliases are accepted anywhere the public API accepts
`provider`. Omitted text models use the authenticated tenant default when
`provider` is omitted. Otherwise, they use the selected provider offering that
declares the text default.

This table describes the original text-provider transports.
Vertex, Dictator, FAL, ElevenLabs, and HeyGen have additional provider and media contracts below.
The complete inventory remains in [configs/providers.yml](../configs/providers.yml).
Upstream providers may expose additional speech APIs that need separate proxy
adapters before they are available through `/dictate`.

| Provider selector | Aliases | Wire contract | Execution lifecycle | Configured default text model | Credential field | Default base URL | Dictation | Web search |
|-------------------|---------|---------------|---------------------|-------------------------------|------------------|------------------|-----------|------------|
| `openai` | none | `openai_responses` | `pollable_resource` | `gpt-4.1` | Account connection API key | `https://api.openai.com/v1` | Yes: `gpt-transcribe` | Yes, on marked OpenAI models |
| `baidu` | none | `openai_chat_completions` | `synchronous_completion` | `ernie-5.0` | Account connection API key | `https://api.baiduqianfan.ai/v1` | No | No |
| `meta` | none | `openai_chat_completions` | `synchronous_completion` | `muse-spark-1.1` | Account connection API key | `https://api.meta.ai/v1` | No | No |
| `deepseek` | none | `openai_chat_completions` | `synchronous_completion` | `deepseek-v4-flash` | Account connection API key | `https://api.deepseek.com` | No | No |
| `dashscope` | `qwen` | `dashscope_responses` | `synchronous_completion` | `qwen-plus` | Account connection API key | Account connection Singapore workspace URL | No | No |
| `moonshot` | `kimi` | `openai_chat_completions` | `synchronous_completion` | `kimi-k2.6` | Account connection API key | `https://api.moonshot.ai/v1` | No | No |
| `minimax` | none | `openai_chat_completions` | `synchronous_completion` | `minimax-m2.7` | Account connection API key | `https://api.minimax.io/v1` | No | No |
| `siliconflow` | none | `openai_chat_completions` | `synchronous_completion` | `deepseek-reasoner` | Account connection API key | `https://api.siliconflow.com/v1` | Yes: `sensevoice-small` | No |
| `zai` | none | `openai_chat_completions` | `synchronous_completion` | `glm-5.1` | Account connection API key | `https://api.z.ai/api/paas/v4` | Yes: `glm-asr-2512` | No |
| `gemini` | none | `gemini_interactions` | `pollable_resource` | `gemini-3.5-flash` | Account connection API key | `https://generativelanguage.googleapis.com/v1beta` | Yes: `gemini-3.5-transcribe` | No |
| `anthropic` | `claude` | `anthropic_messages` | `synchronous_completion` | `claude-sonnet-4-6` | Account connection API key | `https://api.anthropic.com` | No | No |
| `xai` | none | `xai_responses` | `synchronous_completion` | `grok-4.3` | Account connection API key | `https://api.x.ai/v1` | Yes: `xai-stt` | No |

All upstream provider credentials are server-side only. Client requests must
never send OpenAI, Meta, Anthropic, xAI, Gemini, or other upstream API keys.
Media input is model-scoped. The checked-in catalog declares media input only
when the provider offering has a code-owned transport.

Standard-message media input depends on the exact offering's `media_inputs` declaration.
Read the catalog for the permitted media types and transport limits.
A capability on another model does not authorize media for the selected route.
Use native `/v2` for these attachments.

## Route execution

Anthropic includes `claude-fable-5-1` and `claude-opus-5` with text, image input, structured output, and five reasoning effort levels.
Both models have a 1,000,000-token context and a 128,000-token output maximum.
`claude-sonnet-4-6` remains the Anthropic default.
See [current Claude models](claude-current-models.md) for prices, provider retention requirements, and live acceptance evidence.
See [Opus 4.1 retirement](claude-retirement.md) for the bounded Opus 5 selection migration and operator procedure.
Gemini 3.6 and 3.7 Flash are enabled after their live reasoning and stored interaction checks passed.
See [qualified Gemini models](gemini-qualified-models.md) for the exact route controls.
Gemini 3.8 Flash and 3.5 Flash-Lite are enabled through Vertex after service-account qualification.
Their Developer API offerings remain disabled.
See the [Vertex configuration and tenant cutover](vertex-gemini.md).
See [current Gemini candidates](gemini-current-models.md) for their limits, prices, and acceptance checks.
Grok 4.6 remains disabled until live qualification passes.
See [the GLM 5.3 candidates](zai-current-models.md) for text and Flash image contracts, current prices, and qualification requirements.
See [the Grok 4.6 candidate](grok-current-model.md) for its reasoning controls, image limits, and two price tiers.

The versioned [provider catalog](provider-catalog.md) has root records for
operations, publishers, families, exact models, and provider definitions. Each
provider definition owns its fields, transports, provider offerings, and
prices.

The loader rejects unknown fields and unsupported schema versions. It also
rejects invalid identities, references, defaults, protocols, capabilities,
limits, and prices.

The catalog defines the current inventory.
Use public capability discovery for enabled routes.
A declared or enabled route does not establish provider connectivity or production acceptance.
The application compiles one immutable registry from this snapshot.

[Baidu Qianfan](baidu-qianfan.md) provides four text offerings through one API key.
Its response policy checks finish reasons and Qianfan flags before text or key verification succeeds.

Each provider offering references one exact model and one provider transport.
The transport selects reusable request and response codecs, authentication,
and an execution lifecycle. Provider identifiers remain route data and do not
select shared execution code.

Public resources omit credential fields, setting fields, authentication data,
environment names, and upstream model identifiers. Management resources return
safe field definitions and masked connection state.

Offerings use `request_profile` to select a stable payload shape:

| Request profile | Payload behavior |
|-----------------|------------------|
| `openai_responses_temperature` | Adds `temperature`. |
| `openai_responses_temperature_tools` | Adds `temperature` and enabled web-search tools. |
| `openai_responses_reasoning_tools` | Adds reasoning controls and enabled web-search tools. |
| `minimax_chat_completions` | Sends `reasoning_split: true` for separate reasoning output. |

All OpenAI Responses text requests also send `background: true` and
`store: true`. llm-proxy polls the stored OpenAI response server-side until it
reaches a documented terminal state or the request's effective work budget
expires. Only `queued` and `in_progress` are pending states; unknown states do
not trigger polling. Only `completed` can produce a successful response.
Plain REST callers use one `GET /`, `POST /`, or `POST /v2` request and receive
the final formatted answer; they do not stream, poll, or follow a separate
resume endpoint. Separately published provider-specific deferred, batch, or
asynchronous APIs are not implicit variants of these routes and are not
activated by an arbitrary response `id`.

The OpenAI adapter requests provider storage because background Responses need `store: true`.
For ordinary customer-credential text, the response identifier remains in the active request lifecycle.
Hosted recovery retains private provider identifiers in its journal.
The proxy does not expose these provider identifiers.

For ordinary customer-credential text, the adapter stops observation when the caller cancels or the proxy budget expires.
It does not issue an upstream cancel or delete after success, failure, timeout, or cancellation.
The OpenAI account retention policy governs the stored resource.

Gemini 3.x Interactions requires `store: true` for background execution.
The proxy sends `Api-Revision: 2026-05-20` and polls only `queued` and `in_progress`.
It keeps interaction identifiers private.
Ordinary customer-credential calls retain them in the active lifecycle. Hosted recovery can persist them as private journal evidence.

On every exit, it cancels an active interaction and then deletes the resource.
It deletes a terminal interaction directly.
Cancel and delete each receive an independent bounded cleanup context.
A stalled or failed cancel cannot consume the delete attempt.
A failed deletion prevents a successful or output-limit result from escaping as success.

When a transport declares continuation actions, output-limit continuation
starts a distinct inference request. It never treats an arbitrary upstream
identifier as pollable state. Each Gemini Interactions route rejects a request
that contains an `assistant` message.

Provider-specific details:

* OpenAI is the only provider currently exposed with `web_search` support.
  This support applies only to catalog entries with `web_search: true`.
  OpenAI derives each endpoint from its provider transports in `providers.yml`.
  Text and dictation use the same catalog-defined credential field.
  Each catalog model with image input sends ordered Data URLs through OpenAI Responses.
  The adapter uses `detail: auto` for each image.
* OpenAI-compatible text providers send chat completion requests with
  `Authorization: Bearer <api_key>` and the selected provider base URL. The
  shared adapter normalizes `finish_reason=length` into the common
  missing-suffix loop and accepts the assembled text only after
  `finish_reason=stop`; `content_filter`, `tool_calls`, missing, and
  provider-specific non-stop reasons are upstream failures.
* Moonshot Kimi K3 accepts exact public `reasoning_effort` values `low`,
  `high`, and `max`. The adapter sends the selected value as the top-level
  provider field and omits it when no value is selected. Kimi K2.6 and both
  Kimi K2.7 Code routes keep Moonshot's thinking default because the proxy does
  not send a `thinking` field. Kimi K3, K2.6, K2.7 Code, and K2.7 Code
  Highspeed accept ordered JPEG, PNG, or WebP inputs through canonical
  `POST /v2` messages. The adapter sends exact bytes as ordered Data URL blocks
  before message text. Only visible answer `content` enters the proxy response
  and usage path. For output continuation, the adapter stores the complete
  truncated assistant message as private state. It sends that message,
  including `reasoning_content`, only to the next upstream Kimi request.
  Provider `reasoning_content` stays absent from public responses and logs. See
  Moonshot's
  [Kimi K3 guide](https://platform.kimi.ai/docs/guide/kimi-k3-quickstart),
  [reasoning-effort guide](https://platform.kimi.ai/docs/guide/use-reasoning-effort),
  [vision guide](https://platform.kimi.ai/docs/guide/use-kimi-vision-model),
  and [Chat Completions reference](https://platform.kimi.ai/docs/api/chat).
* MiniMax uses selector `minimax`, a tenant-managed API key, and
  `https://api.minimax.io/v1`. `minimax-m2.7` remains the default. The catalog
  also includes M2.7 Highspeed, M2.5, M2.5 Highspeed, M2.1, M2.1 Highspeed,
  and M2. The shared Chat Completions adapter maps public `max_tokens` to
  upstream `max_completion_tokens`. All seven routes enforce the documented
  204,800-token completion maximum. Generation and key verification send
  `reasoning_split: true`. Public answers contain only visible content.
  Reasoning remains private during output continuation.
  The private catalog also contains the disabled [M3 candidate](minimax-m3.md).
  M3 adds JPEG, PNG, and WebP input with a 524,288-token output maximum.
  Live text and image acceptance must pass before M3 activation. See the official
  [OpenAI SDK guide](https://platform.minimax.io/docs/api-reference/text-openai-api),
  [Chat Completions reference](https://platform.minimax.io/docs/api-reference/text-chat-openai),
  and [PAYG prices](https://platform.minimax.io/docs/guides/pricing-paygo).
* Meta Model API requests use that shared Chat Completions adapter with the
  exact `meta` selector and `https://api.meta.ai/v1` base URL. The provider
  offerings are `muse-spark-1.1`, `muse-spark-1.2`, and `muse-spark-1.3`. Muse Spark 1.1 remains
  the Meta default. llm-proxy exposes the public `max_tokens` input upstream as Meta's current
  `max_completion_tokens` field rather than Meta's deprecated `max_tokens` field.
  The enabled [Muse Spark 1.3 offering](meta-current-model.md) passed live qualification with the existing Meta key.
  That Standard-tier model adds six reasoning effort levels and current prices.
  The proxy exposes these models as text generation through `GET /`,
  `POST /`, and `POST /v2`. Meta describes Muse Spark 1.2 as coding-focused.
  This focus does not change the provider transport or add agent orchestration.
  The proxy does not expose Meta dictation, `web_search`, tools, multimodal
  input, or a Responses API fallback. See Meta's
  [Muse Spark 1.2 announcement](https://research.meta.ai/blog/introducing-muse-code-and-muse-spark-1-2),
  [model reference](https://dev.meta.ai/docs/models),
  [Chat Completions reference](https://dev.meta.ai/docs/protocols/chat-completions),
  and [pricing and rate-limit documentation](https://dev.meta.ai/docs/pricing-rate-limits).
* Gemini file dictation uses synchronous `gemini_interactions`. See the
  [transcription contract](gemini-transcription.md).
* Other dictation providers use a `multipart_transcription` transport in
  `providers.yml`. That transport owns the exact endpoint and model-field rule.
* Gemini text requests use native `POST /interactions` against the configured
  `v1beta` base URL with `x-goog-api-key` and
  `Api-Revision: 2026-05-20`. Every configured Gemini 3.x route sends
  `background: true` and `store: true`. The proxy polls its `queued` and
  `in_progress` states. User messages become `user_input` steps. System
  messages become the top-level `system_instruction`. Assistant history is
  invalid because the public request cannot carry provider interaction state or
  thought signatures. Only `completed` with visible model text succeeds.
  `incomplete` returns a provider error after interaction deletion. A `failed`
  response can include diagnostic errors. The adapter accepts each code only
  when it is a canonical HTTPS URI of 2,048 characters or fewer. The URI must
  contain a host and a clean path. It must not contain user information, a
  query, a fragment, or an escaped path. The final path segment must be a
  1-to-64-character `snake_case` value. The adapter discards each provider
  message. A malformed URI is a response protocol failure. A failed
  Interaction is retryable only when every URI has a retryable final path
  segment. The retryable segments are `aborted`, `api_error`,
  `deadline_exceeded`, `rate_limit_exceeded`, `service_unavailable`, and
  `too_many_requests`. An empty list or an unknown segment fails closed.
  `cancelled`, `requires_action`, malformed, missing, and unknown states are
  safe upstream failures. Usage
  totals map from `total_input_tokens`, `total_output_tokens`, and
  `total_tokens`, preserving provider-counted thought tokens. For exact models
  whose catalog declares `media_inputs`, ordered image and audio attachments
  become native typed interaction content after the message text. The adapter
  sends an inline request when the complete encoded request is at most the
  offering's inline limit of 20,000,000 bytes for image and audio requests.
  It streams exact media bytes through the Gemini
  Files API when the encoded request is larger, then deletes each provider
  file after the interaction ends. See Google's
  [Interactions overview](https://ai.google.dev/gemini-api/docs/interactions-overview),
  [file input methods](https://ai.google.dev/gemini-api/docs/file-input-methods),
  [Files API guide](https://ai.google.dev/gemini-api/docs/files),
  [background execution guide](https://ai.google.dev/gemini-api/docs/background-execution),
  and [Interactions API reference](https://ai.google.dev/api/interactions-api).
* Anthropic text requests use `POST /v1/messages` with `x-api-key` and
  `anthropic-version: 2023-06-01`. The proxy maps system messages to Anthropic's
  top-level `system` field. Anthropic requires `max_tokens`.
  When the client omits it, the proxy sends the selected model output limit.
  `stop_reason=max_tokens` starts the common missing-suffix loop.
  `end_turn` or `stop_sequence` completes the assembled answer.
  Tool use, paused turns, refusals, and unknown reasons remain upstream failures.
  Each catalog model with image input sends ordered base64 image blocks before the message text.
* Z.AI dictation uses the catalog-defined GLM-ASR transport with the selected
  exact dictation model.
* All xAI text models use the xAI Responses codec at
  `https://api.x.ai/v1/responses`. Requests use `store: false` and omit `background`.
  The `grok-4.5` image blocks use `detail: high`. Grok/xAI dictation
  uses the catalog-defined xAI STT transport. The upstream STT endpoint does
  not receive a `model` multipart field.

See the [xAI Responses contract and live acceptance procedure](xai-responses.md).

Tenant access keys, defaults, prompts, and usage belong to the tenant.
Customer provider credentials and fields belong to named account connections.
A tenant selects one account connection or one hosted access grant per provider.
Multiple tenants in the same account can use the same connection.
A customer-credential request requires an assigned connection with its required fields.
Otherwise, the native proxy returns `409 provider_not_configured` before dispatch.
Hosted assignments require financial admission. The tracked configuration omits `hosted`, so it does not activate hosted execution.

Static provider URLs and paths belong to `providers.yml`.
Alibaba Cloud and Baidu also use a `base_url` connection field.
Baidu supplies the international Qianfan URL as its catalog default.
The `management` configuration is mandatory.
Provider blocks in `config.yml` are unknown YAML keys and fail startup.

## Additional provider contracts

| Provider | Current reference |
| --- | --- |
| Vertex AI | [Google credentials, Gemini, and image routes](vertex-gemini.md) |
| Dictator | [Speech workflows](speech-workflows.md) and [acceptance scope](dictator-live-acceptance.md) |
| FAL | [Image operations](fal-image-operations.md) |
| ElevenLabs | [Speech](provider-speech.md), [voices](provider-voices.md), [dictionaries](provider-dictionaries.md), and [account resources](elevenlabs-account-resources.md) |
| HeyGen | [Video and resource contract](heygen-v3.md) |

The [provider inventory](media-provider-completeness.md) separates retained capability work from completed adapters.

## Model catalog

The [catalog](../configs/providers.yml) defines exact models and their controls.
The `/dictate` endpoint uses the saved tenant transcription route when provider and model are omitted.
Transcription providers include OpenAI, SiliconFlow, Z.AI, Gemini, and xAI.
Web search requires an offering with `web_search: true`.

### Reasoning effort

Read the exact model's `reasoning_effort` declaration in the catalog.
Omission retains the saved tenant effort. An explicit value must belong to that route's permitted list.
See [Astra](astra.md), [Claude](claude-current-models.md), [Gemini](gemini-qualified-models.md), and [Qwen](qwen-current-models.md) for provider evidence.

See OpenAI's [GPT-4.1 model reference](https://developers.openai.com/api/docs/models/gpt-4.1),
[GPT-5 API launch contract](https://openai.com/index/introducing-gpt-5-for-developers/),
[GPT-5 model reference](https://developers.openai.com/api/docs/models/gpt-5),
[GPT-5.5 model reference](https://developers.openai.com/api/docs/models/gpt-5.5),
[GPT-5.5 Pro model reference](https://developers.openai.com/api/docs/models/gpt-5.5-pro),
and [latest-model guide](https://developers.openai.com/api/docs/guides/latest-model).

### DeepSeek model retirement

Direct DeepSeek routes use `deepseek-v4-flash` and `deepseek-v4-pro`.
Both routes accept `reasoning_effort` values `none`, `low`, `high`, and `max`.
The schema-version-14 migration replaces saved direct selections and retains historical usage identities.
See the [operator procedure](deepseek-retirement.md) for acceptance, migration checks, and conflicting profile settings.

### Exact models and controls

Use [the catalog](../configs/providers.yml) for exact models, provider defaults, prices, and limits.
Use `/api/public/capabilities` for enabled catalog routes and `/v1/models` for usable routes under a tenant bearer key.
The [route explorer](https://llm-proxy.mprlab.com/) presents capability tasks for each offering.
The catalog validates disabled candidates but excludes them from runtime discovery.

Qwen 3.7 Plus and Qwen 3.6 Flash accept image input and return text. Qwen Plus
and Qwen 3.7 Max remain text-only offerings. Each route uses the tenant's exact
Singapore workspace URL and the DashScope Responses endpoint.
`max_tokens` maps to `max_output_tokens` and has an inclusive `16..65536` proxy boundary. Each versioned model has a 1,000,000-token context window.
The catalog records Singapore pay-as-you-go list rates for the lowest published
input tier. Qwen 3.7 Max costs $2.50 for input and $7.50 for output per million
tokens. Qwen 3.7 Plus costs $0.40 for input and $1.60 for output. Qwen 3.6 Flash
costs $0.25 for input and $1.50 for output. The limits, prices, Singapore
availability, and workspace URL were verified on 2026-08-13 against Alibaba's
[text model guide](https://www.alibabacloud.com/help/en/model-studio/text-generation-model),
[model catalog](https://www.alibabacloud.com/help/en/model-studio/models),
[OpenAI compatibility guide](https://www.alibabacloud.com/help/en/model-studio/compatibility-with-openai-responses-api),
and [pricing page](https://www.alibabacloud.com/help/en/model-studio/model-pricing).
The image input declarations were verified on 2026-09-02 against Alibaba's
[visual understanding guide](https://www.alibabacloud.com/help/en/model-studio/vision-model)
and [Chat Completions guide](https://www.alibabacloud.com/help/en/model-studio/qwen-api-via-openai-chat-completions).

All seven MiniMax M2 routes are text-only Chat Completions offerings with a
204,800-token context record and output boundary. M2.7 input and output cost
$0.30 and $1.20 per million tokens. M2.7 Highspeed costs $0.60 and $2.40.
The M2.5, M2.1, and M2 routes use the same standard input and output rates as
M2.7. Their Highspeed variants use the M2.7 Highspeed rates. The catalog also
records the published prompt-cache read and write rates. MiniMax now lists
M2.5, M2.1, and M2 under legacy prices, but still includes them in the supported
OpenAI-compatible model list. Limits, availability, and standard PAYG prices
were verified on 2026-08-13 against the MiniMax references in [Route execution](#route-execution).

### Dictation capabilities

| Provider selector | Models | Credential field | Provider transport | Notes |
|-------------------|--------|------------------|--------------------|-------|
| `openai` | `gpt-transcribe` | Account connection API key | Catalog `dictation` transport | OpenAI catalog default `gpt-transcribe`. The saved tenant transcription route controls omitted provider selection. |
| `siliconflow` | `sensevoice-small` | Account connection API key | Catalog `dictation` transport | OpenAI-compatible audio transcription. |
| `zai` | `glm-asr-2512` | Account connection API key | Catalog `dictation` transport | Z.AI GLM-ASR sends `model=glm-asr-2512`. |
| `xai` | `xai-stt` | Account connection API key | Catalog `dictation` transport | The upstream request omits the multipart `model` field. |
| `gemini` | `gemini-3.5-transcribe` | Account connection API key | Synchronous Interactions | See [Gemini transcription](gemini-transcription.md). |

See [OpenAI transcription retirement](openai-transcription-retirement.md) for the current model, saved-selection migration, and operator activation checks.

## Public catalog presentation

The landing page and route explorer use the validated runtime catalog.
The explorer starts with All capabilities and both weight access groups selected.
Visitors can select Tools, Web search, Reasoning, Images, Audio, or Video.
Input and output filters must match one task on the same offering.
For a media group, the task must use a capability in that group.

Model details identify each supported task and its input and output direction.
The explorer selects a family, exact model, and provider offering in that order.
The model matrix has one row for each exact model and shows its current provider offerings.
A catalog change updates both interfaces without a separate inventory.

LLM Proxy owns the durable media gateway, tenant assets, and official gateway client.
MediaOps retains product workflows, application data, and local processing.
I286 defines the current [provider boundary](mediaops-model-access-boundary.md) and replaces F071.
The [consolidation contract](media-gateway-consolidation.md) records shared lifecycle requirements.
