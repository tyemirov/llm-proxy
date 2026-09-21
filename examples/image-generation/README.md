# Image Client Example

The program reads `IMAGE_GENERATION_INPUT_JSON` and calls the official Go client.
Set the connection, tenant key, idempotency key, and output directory as described in the repository README.
Use one explicit surface in the input.

```json
{
  "provider": "openai",
  "model": "gpt-image-2",
  "surface": "images",
  "prompt": "A small lighthouse beside the sea",
  "quality": "low",
  "size": "1024x1024",
  "background": "opaque",
  "output_format": "png",
  "output_count": 1
}
```

For Responses generation, set `surface` to `responses` and add `responses_model` with a value from capability discovery.
The current OpenAI catalog selects `gpt-5` for that control.
Responses accepts one output image.
Set `stream` to `true` and `partial_images` to a value from zero through three to request progressive output.
Read `MediaOperation.PartialOutputs` through `GetMediaOperation` and download each new asset through the authenticated asset methods.
The example program waits for completion and saves final outputs.

Use the same client for a follow-up edit:

```go
followUp := llmproxyclient.ImageEditingInput{
    ImageGenerationInput: llmproxyclient.ImageGenerationInput{
        Provider: "openai", Model: "gpt-image-2",
        Surface: "responses", ResponsesModel: "gpt-5",
        PreviousOperationID: completed.OperationID,
        Prompt: "Change the scene to sunset",
        Quality: "low", Size: "1024x1024", Background: "opaque",
        OutputFormat: "png", OutputCount: 1,
        Stream: true, PartialImages: 2,
    },
}
operation, err := client.CreateImageEditing(ctx, editIdempotencyKey, followUp)
```

The parent must be a successful Responses operation from the same tenant and accepted route.
Add ordered `ImageAssetIDs` when the edit needs additional uploaded images.
Images editing requires those asset identifiers and accepts an optional PNG `MaskAssetID`.
Responses editing rejects masks.
The gateway stores native provider identifiers privately.
Reuse the original idempotency key and intent after an uncertain client response.
Do not submit a second operation to determine the first outcome.
