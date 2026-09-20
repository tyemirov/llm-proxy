package proxy

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const (
	imageControlAspectRatio = "aspect_ratio"
	imageLimitOutputPixels  = "output_pixels"
)

var queueImageModelPath = regexp.MustCompile(`^[A-Za-z0-9_-]+(/[A-Za-z0-9_.-]+)+$`)

var queueImageAspectRatios = []string{"auto", "4:1", "3:1", "21:9", "2:1", "17:9", "16:9", "3:2", "4:3", "5:4", "1:1", "4:5", "3:4", "2:3", "9:16", "1:2", "1:3", "1:4"}

func validateArtifactOrigins(transport ProviderCatalogTransport, field string) error {
	queue := transport.Components.RequestCodec.ID == CatalogProtocolFALQueueImages
	artifacts := queue || transport.Components.RequestCodec.ID == CatalogProtocolElevenLabsVoices
	if artifacts != (len(transport.ArtifactOrigins) > 0) || (queue && transport.Endpoint.Path != "/{model}") {
		return fmt.Errorf("%w: field=%s.artifact_origins reason=codec_composition", ErrInvalidModelCatalog, field)
	}
	seen := map[string]bool{}
	for _, origin := range transport.ArtifactOrigins {
		normalized, err := normalizedUpstreamOrigin(origin)
		endpoint := ProviderCatalogEndpoint{Protocol: CatalogEndpointProtocolHTTP, DefaultBaseURL: origin, Path: "/", Method: CatalogEndpointMethodGet}
		if err != nil || normalized != origin || validateProviderCatalogEndpoint(endpoint, nil, field) != nil || seen[origin] {
			return fmt.Errorf("%w: field=%s.artifact_origins reason=invalid_origin", ErrInvalidModelCatalog, field)
		}
		seen[origin] = true
	}
	return nil
}

func validateQueueImageOffering(offering ProviderOffering, field string) error {
	if !queueImageModelPath.MatchString(offering.ProviderModel) || strings.Contains(offering.ProviderModel, "/../") || strings.HasSuffix(offering.ProviderModel, "/..") || strings.Contains(offering.ProviderModel, "/./") || strings.HasSuffix(offering.ProviderModel, "/.") || offering.ExecutionLifecycle != CatalogExecutionAsynchronousJob || !slices.Equal(offering.Operations, []string{ModelOperationImageGeneration}) || offering.ImageRoutes != (CatalogImageRoutes{}) || offering.RequestProfile != "" || offering.WebSearch || offering.CallerTools || offering.OutputTokenLimit != 0 || offering.ReasoningEffort != nil || len(offering.MediaInputs) != 0 || len(offering.ImageMIMETypes) != 0 || len(offering.MediaLimits) != 0 || len(offering.Controls) != 3 || len(offering.Limits) != 3 {
		return fmt.Errorf("%w: field=%s reason=invalid_queue_image_capabilities", ErrInvalidModelCatalog, field)
	}
	for _, control := range offering.Controls {
		valid := !control.AccountDependent
		switch control.ID {
		case imageControlAspectRatio:
			valid = valid && imageEnumSubset(control, queueImageAspectRatios)
		case imageControlFormat:
			valid = valid && imageEnumSubset(control, []string{"png", "jpeg", "webp"})
		case imageControlCount:
			valid = valid && control.Kind == CatalogControlInteger && *control.Minimum >= 1 && *control.Maximum <= 4
		default:
			valid = false
		}
		if !valid {
			return fmt.Errorf("%w: field=%s.controls control=%s reason=unsupported_queue_image_control", ErrInvalidModelCatalog, field, control.ID)
		}
	}
	for _, limit := range offering.Limits {
		valid := !limit.AccountDependent
		switch limit.ID {
		case imageLimitPrompt:
			valid = valid && limit.Unit == "characters" && *limit.Value <= 4000
		case imageLimitOutput:
			valid = valid && limit.Unit == "bytes" && *limit.Value <= imageMaximumOutputBytes
		case imageLimitOutputPixels:
			valid = valid && limit.Unit == "pixels" && *limit.Value <= 64<<20
		default:
			valid = false
		}
		if !valid {
			return fmt.Errorf("%w: field=%s.limits limit=%s reason=unsupported_queue_image_limit", ErrInvalidModelCatalog, field, limit.ID)
		}
	}
	return nil
}
