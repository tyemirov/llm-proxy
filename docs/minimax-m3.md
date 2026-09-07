# MiniMax M3

F045 adds exact model `minimax-m3` with upstream selector `MiniMax-M3`.
The model uses the existing MiniMax provider connection and Chat Completions transport.
Its separate `minimax-m3` family has open weights.
The provider default remains `minimax-m2.7`.

## Activation

The canonical catalog keeps M3 disabled until live text and image acceptance passes.
Normal routing, management selection, public model discovery, and standard live tests exclude M3.
The runtime also excludes its family until M3 is enabled.
Startup still validates its private metadata.

The candidate harness enables M3 only in a copied catalog for a disposable proxy.
It verifies the provider key through the authenticated management operation before each generation test.
Successful local protocol tests prove repository behavior. Live qualification proves provider connectivity.

## Request contract

The candidate uses public text generation and ordered JPEG, PNG, and WebP attachments.
The `minimax_chat_completions` profile sends `reasoning_split: true` for generation and key verification.
Public answers contain visible content. Output continuation keeps provider reasoning private.
The adapter maps public `max_tokens` to upstream `max_completion_tokens`.

| Limit | Catalog value |
|---|---:|
| Context | 1,000,000 tokens |
| Output | 524,288 tokens |
| Minimum output request | 1 token |
| Inline image | 10,000,000 bytes |
| Encoded inline request | 64,000,000 bytes |
| Image count | Unknown |

The byte limits use decimal megabytes for the provider's published 10 MB and 64 MB limits.
The current public attachment contract excludes GIF and video input.
The provider documents these formats, but they require separate public capability work.

## Standard prices

Prices are USD per one million tokens, verified on 2026-09-05.
The catalog records the provider's two input context tiers under `pay_as_you_go_standard`.

| Input context tier | Input | Output | Cache read |
|---|---:|---:|---:|
| Up to 512k | 0.30 | 1.20 | 0.06 |
| Above 512k | 0.60 | 2.40 | 0.12 |

Each rate retains its tier in `conditions.mode`.
The tier identifiers are `input_tokens_up_to_512k` and `input_tokens_above_512k`.
The provider's standard service tier applies to these requests.

## Qualification

1. Set `MINIMAX_API_KEY` in the selected private environment file.
2. Run the paid text and image qualification command:

   ```sh
   make test-live-minimax-m3 LIVE_ENV_FILE=configs/.env
   ```

3. If qualification fails, correct the reported provider or request error before activation.
4. After both tests pass, record the evidence in F045 and enable M3 in the canonical catalog.
5. Run `make ci` after the activation change.

The command stops at the first failure.
A complete run submits two key verifications and two generation requests.
Output continuation can add provider calls.
It preserves the primary catalog and normal model availability.
The 2026-09-05 attempt stopped before a provider call because `MINIMAX_API_KEY` was absent from all authorized inputs.

## Sources

- [MiniMax OpenAI SDK guide](https://platform.minimax.io/docs/api-reference/text-openai-api)
- [MiniMax Chat Completions reference](https://platform.minimax.io/docs/api-reference/text-chat-openai)
- [MiniMax standard prices](https://platform.minimax.io/docs/guides/pricing-paygo)
- [Official MiniMax M3 weights](https://huggingface.co/MiniMaxAI/MiniMax-M3)
