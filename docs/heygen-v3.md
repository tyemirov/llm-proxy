# HeyGen v3

The gateway owns the HeyGen account connection and native API requests.
MediaOps keeps product jobs, user approvals, local files, and media processing.
All HeyGen transports in this implementation use API v3.
HeyGen retires API v1 and v2 after October 31, 2026.
See the [native migration guide](https://developers.heygen.com/endpoint-version-comparison).

## Services

The provider catalog declares four services without model identifiers.
The services use the common durable operation API and one tenant account connection.

| Capability | Input references | Native route |
| --- | --- | --- |
| `video.lipsync` | Video and audio assets | `/v3/lipsyncs` |
| `video.translate` | Video asset and optional audio asset | `/v3/video-translations` |
| `avatar.create` | Image asset | `/v3/avatars` |
| `avatar.video.generate` | Gateway avatar and audio asset | `/v3/videos` |

Upload input files through the gateway asset API.
Supply only tenant-owned gateway asset identifiers in operation requests.
The gateway uploads each selected file through `/v3/assets`.
The maximum native upload size is 32 MiB.
The catalog and gateway asset limits also apply.

Select `speed` or `precision` for lip sync and translation.
Supply translation languages as an ordered `output_languages` array.
The gateway retains each returned native job identifier before it reads job status.
Output order follows the requested language order.
Native job identifiers remain private.

The Go client provides `CreateVideoLipSync`, `CreateVideoTranslation`, `CreateAvatar`, and `CreateAvatarVideo`.
Use `GetMediaCapabilities` to read the available services and controls.
The Python client supports these capabilities through `create_media_operation`.
The OpenAPI document specifies their input and control fields.

## Avatar Resources

Avatar creation returns a JSON asset with `avatar_id`, `provider`, and `name`.
The gateway also saves a retained avatar record.
This record keeps the native look identifier private.
It remains available after the creation operation expires.
Rendering requires the same tenant, provider, and account authority.

The current rendering engine is `avatar_iv`.
Avatar V remains separate work under F028.
Motion uses `motion_prompt` and `expressiveness` during video generation.
This implementation has no separate add-motion operation.
It does not expose native folder, glossary, or callback identifiers.
Deprecated caption controls are absent.

## Account Observations

`GetProviderAccount` reads `/model/v1/provider-resources/heygen/account`.
The gateway uses `/v3/users/me` for this read-only request.
The result preserves the native billing type and its selected balance contract.
Wallet observations retain their USD or credit currency.
Subscription observations retain separate credit pools.
Metered observations retain the credit balance and USD spending fields.
An absent balance remains null.
Account observations do not become operation prices or usage charges.

## Recovery And Acceptance

The gateway saves accepted provider receipts before status polling.
Recovery reads existing jobs and does not repeat a paid submission.
Saved outputs and failed translation-child states remain available during recovery.
An ambiguous submission records an uncertain outcome.
Queued work supports cancellation.
Native work reports unsupported cancellation after dispatch.
Completed output bytes become tenant-owned gateway assets.
Provider artifact downloads exclude account credentials.

`make test-heygen` uses local native protocol servers and the real gateway HTTP API.
The tests use the catalog loader, SQLite, filesystem assets, and official Go client.
A second provider identity uses the same adapters with different authentication.
Source tests do not prove native provider connectivity or service activation.

F027 remains open for its Kling work and other account resources.
The MediaOps I012 cutover requires the released client and active gateway.
Existing MediaOps provider records need an inventory before their direct path can be removed.
Live paid acceptance, publication, and deployment remain separate operator steps.

The native schemas come from the official HeyGen CLI at commit
[`aeaeac04e726c9a246f3957f1fa74461b9e05c6c`](https://github.com/heygen-com/heygen-cli/tree/aeaeac04e726c9a246f3957f1fa74461b9e05c6c/gen).
The schema source date is September 30, 2026.
