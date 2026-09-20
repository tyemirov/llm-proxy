# FAL Image Operations

## Catalog And Connection

F041 adds the `reve-2.1` exact model through the existing media operation interface.
One `fal` provider definition owns both transports in `configs/providers.yml`.
The account transport verifies the shared API key through the authenticated pricing resource.
The image transport uses the same account connection and tenant assignment.
Connection verification does not submit image generation.

The public catalog, connection form, model matrix, and route explorer use the validated catalog.
FAL and Reve use explicit text labels because no logo asset is selected.
FAL video offerings belong to F025 and will extend this provider definition.

## Image Request

Create an operation at `POST /model/v1/operations` with an `Idempotency-Key` header.
Select `image.generate`, provider `fal`, and model `reve-2.1`.
Read the selected offering for accepted controls and limits.

```json
{
  "capability": "image.generate",
  "provider": "fal",
  "model": "reve-2.1",
  "input": {"prompt": "A lighthouse beside the sea"},
  "controls": {
    "aspect_ratio": "1:1",
    "output_format": "png",
    "output_count": 2
  }
}
```

This route accepts prompt-only generation. It requires no input staging.
The catalog lists aspect ratios, PNG, JPEG, WebP, and one through four outputs.
It limits prompts to 4,000 characters.
The gateway also limits each output to 64 MiB and 67,108,864 pixels.

The Go client supplies `AspectRatioImageGenerationInput` and `CreateAspectRatioImageGeneration`.
The Python client supplies `ClientAspectRatioImageInput.operation()` for `create_media_operation`.
Both use the shared operation, artifact, and asset representations.

## Queue Evidence And Artifacts

The `fal_queue_images` codec submits the native model path from the offering.
The adapter persists the returned request identifier and queue URLs before status polling.
Those values remain private. Public callers use the gateway operation identifier.
Restart recovery reads the original request. It never repeats the generation submission.
A changed connection, missing handle, or changed execution contract leaves the outcome uncertain.

Queue URLs must use the configured origin and the same request root.
Artifact URLs must use an exact origin declared in the image transport's `artifact_origins` list.
The declaration also supplies origins to upstream capacity validation.
Runtime capacity values remain in the deployment configuration.
The default HTTP client rejects redirects so each request uses its admitted origin.

Artifact downloads omit provider credentials.
The adapter checks response size, image format, decoded pixels, complete image data, and output count.
It publishes ordered tenant assets only after all outputs pass these checks.
A retrieval failure leaves the operation uncertain with its original private handle.

Cancellation uses the operation cancellation resource.
An upstream acknowledgement preserves `cancellation_state: requested` while the operation remains active.
The acknowledgement does not prove a terminal cancellation.
A provider result can still establish success or failure after that acknowledgement.

## Validation And Migration

`make test-fal-images` exercises real gateway HTTP endpoints with local provider servers.
Tests cover two provider identities, shared credentials, ordered assets, idempotency, restart recovery, and connection detachment.
They also cover malformed responses, output limits, redirects, foreign URLs, cancellation, and worker fencing.
A browser test saves and reloads one FAL connection and shows the Reve model.
Local protocol tests do not establish live provider acceptance.

Native MediaOps handles require exact source-account, destination-account, connection, and tenant ownership records before import.
The bounded source audit inspected 1,233 operation records and 667 sidecar JSON files across four workspaces.
It found one FAL catalog result and no FAL generation record or native recovery handle.
The private receipt is `.git/f041-fal-source-inventory.json`.
The operation digests match the prior MediaOps inventory.
This result does not cover external provider accounts or other source scopes.
No retained FAL record was imported, deleted, or submitted again by this change.
Client publication, deployment, live acceptance, and MediaOps source retirement remain separate steps.

## Upstream References

The source review date is September 20, 2026.

- [Reve 2.1 model API](https://fal.ai/models/reve/2.1/text-to-image/api)
- [FAL queue lifecycle](https://fal.ai/docs/documentation/model-apis/inference/queue)
- [Authenticated model pricing](https://fal.ai/docs/platform-apis/v1/models/pricing)
- [FAL CDN](https://fal.ai/docs/documentation/model-apis/fal-cdn)
