# Dictator Live Acceptance

## Scope

`make test-dictator-live` runs the current gateway against the selected live Dictator service.
The test starts a local gateway with temporary SQLite and asset stores.
It creates an account connection through the management API and assigns the connection to a temporary tenant.
The official Go client then calls the gateway's public API.

The test covers these results:

- Rejection of an invalid upstream credential.
- Provider capabilities and voice discovery.
- Transcription, diarization, alignment, and SRT subtitles.
- Voice extraction and synthesis with the extracted voice.
- Audio, timeline, and result downloads through tenant assets.
- The same operation identity after a duplicate request.
- An explicit unavailable-cost reason.

The test submits real upstream jobs.
It does not transfer MediaOps records or configure a deployed gateway.
Production ownership and consumer acceptance remain separate F042 requirements.
WriterBlock is excluded.

## Inputs

Set these environment variables before execution.

| Variable | Required value |
| --- | --- |
| `DICTATOR_GRPC_HOST` | The selected service hostname. |
| `DICTATOR_GRPC_PORT` | The service port. |
| `DICTATOR_GRPC_AUTH_TOKEN` | The service bearer credential. |
| `DICTATOR_GRPC_USE_TLS` | A boolean accepted by Go `strconv.ParseBool`. The harness sends the canonical boolean string to the gateway. |
| `LLM_PROXY_DICTATOR_LIVE_WAV_PATH` | A WAV file with the spoken English words `hello world`. |

Use an audio file with a duration of at least 0.5 seconds.
The harness requests an extraction duration of 0.5 seconds.
The API selects 20 seconds only when `duration_seconds` is absent.
The reference transcript is `hello world`.
Keep the spoken content consistent with that transcript.

Supply credentials through the process environment.
If a private environment file supplies them, clear the listed variables before you source that file.
Do not print the credential.
The target does not load an environment file automatically.

```sh
make test-dictator-live
```

The target enables the live test explicitly and uses a 20-minute process limit.
Missing inputs and failed operations fail the target.
Routine CI skips the live test.
`make test-dictator` exercises the same acceptance assertions with a local protocol fixture.

## Acceptance Record: 2026-09-15

The selected MediaOps environment supplied the existing service connection.
The fixture contained generated English speech and silence, with a total duration of 24 seconds.
The six live capabilities passed in 22.94 seconds.
The official client downloaded eight result assets.
The service rejected the invalid credential.
Duplicate requests retained their operation identities.

The first fixture was 0.74 seconds long.
Transcription, diarization, alignment, and subtitles passed with that fixture.
Extraction returned a terminal provider error.
The longer fixture passed extraction and synthesis without a production code change.

The local gateway used the current source and released Dictator SDK `v1.11.0`.
This result proves live provider execution through the gateway API.
It does not prove gateway deployment, retained-resource transfer, or MediaOps consumer acceptance.

## Duration Acceptance: 2026-09-15

The original 0.74-second fixture passed all six capabilities after the duration control change.
The harness requested `duration_seconds: 0.5` through the public operation API.
`make test-dictator-live` passed in 5.18 seconds.
The final run passed in 5.21 seconds after explicit-null rejection was added.
The official client downloaded all eight result assets.
This result replaces the longer-fixture workaround for the current harness.
