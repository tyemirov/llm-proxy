package proxy

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const (
	imageControlQuality        = "quality"
	imageControlSize           = "size"
	imageControlBackground     = "background"
	imageControlFormat         = "output_format"
	imageControlCompression    = "output_compression"
	imageControlCount          = "output_count"
	imageControlStream         = "stream"
	imageControlPartials       = "partial_images"
	imageControlSurface        = "surface"
	imageControlResponsesModel = "responses_model"
	imageLimitResponsesOutputs = "responses_output_images"
	imageLimitStreamOutputs    = "stream_output_images"
	imageLimitPrompt           = "prompt_characters"
	imageLimitOutput           = "output_bytes"
	imageMaximumOutputBytes    = 64 << 20
	imageLimitInputs           = "input_images"
	imageLimitInputBytes       = "input_image_bytes"
	imageLimitInputPixels      = "input_image_pixels"
)

// CatalogImageRoutes selects additional transports for an image offering.
type CatalogImageRoutes struct {
	Editing   string `yaml:"editing,omitempty" mapstructure:"editing"`
	Responses string `yaml:"responses,omitempty" mapstructure:"responses"`
}

func validateCatalogImageRoutes(offering ProviderCatalogOffering, offerings []ProviderCatalogOffering, models map[string]ModelActivation, transports map[string]ProviderCatalogTransport, field string) error {
	edits := slices.Contains(offering.Operations, ModelOperationImageEditing)
	if edits || offering.ImageRoutes.Editing != "" {
		transport, exists := transports[offering.ImageRoutes.Editing]
		if !edits || !slices.Contains(offering.Operations, ModelOperationImageGeneration) || !exists || transport.Components.RequestCodec.ID != CatalogProtocolOpenAIImages || transport.Components.Execution.ID != string(textExecutionLifecycleSynchronousCompletion) {
			return fmt.Errorf("%w: field=%s.image_routes reason=invalid_editing_transport", ErrInvalidModelCatalog, field)
		}
	}
	if offering.ImageRoutes.Responses != "" {
		transport, exists := transports[offering.ImageRoutes.Responses]
		if !slices.Contains(offering.Operations, ModelOperationImageGeneration) || !exists || transport.Components.RequestCodec.ID != CatalogProtocolOpenAIResponses || transport.Components.Execution.ID != string(textExecutionLifecyclePollableResource) {
			return fmt.Errorf("%w: field=%s.image_routes reason=invalid_responses_transport", ErrInvalidModelCatalog, field)
		}
		for _, control := range offering.Controls {
			if control.ID != imageControlResponsesModel {
				continue
			}
			for _, model := range control.Values {
				found := false
				for _, candidate := range offerings {
					if candidate.Model == model && candidate.Enabled != ModelDisabled && models[model] == ModelEnabled && candidate.Transport == offering.ImageRoutes.Responses && slices.Contains(candidate.Operations, ModelOperationText) && slices.Contains(candidate.MediaInputs, CatalogArtifactImage) {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("%w: field=%s.controls model=%s reason=invalid_responses_model", ErrInvalidModelCatalog, field, model)
				}
			}
		}
	}
	return nil
}

func validateImageGenerationOffering(offering ProviderOffering, field string) error {
	if offering.WireContract != CatalogProtocolOpenAIImages || offering.ExecutionLifecycle != string(textExecutionLifecycleSynchronousCompletion) {
		return fmt.Errorf("%w: field=%s reason=unsupported_image_route", ErrInvalidModelCatalog, field)
	}
	if offering.RequestProfile != "" || offering.WebSearch || offering.CallerTools || offering.OutputTokenLimit != 0 || offering.ReasoningEffort != nil || len(offering.MediaInputs) != 0 || len(offering.ImageMIMETypes) != 0 || len(offering.MediaLimits) != 0 {
		return fmt.Errorf("%w: field=%s reason=text_capabilities_on_image_route", ErrInvalidModelCatalog, field)
	}
	limitCount := 3
	controlCount := 9
	edits := slices.Contains(offering.Operations, ModelOperationImageEditing)
	if edits {
		limitCount = 6
		if offering.ImageRoutes.Editing == "" {
			return fmt.Errorf("%w: field=%s.image_routes reason=missing_editing_transport", ErrInvalidModelCatalog, field)
		}
	}
	responses := offering.ImageRoutes.Responses != ""
	if responses {
		controlCount++
		limitCount++
	}
	if len(offering.Controls) != controlCount || len(offering.Limits) != limitCount {
		return fmt.Errorf("%w: field=%s reason=incomplete_image_capabilities", ErrInvalidModelCatalog, field)
	}
	for _, control := range offering.Controls {
		valid := !control.AccountDependent
		switch control.ID {
		case imageControlQuality:
			valid = valid && imageEnumSubset(control, []string{"auto", "low", "medium", "high"})
		case imageControlBackground:
			valid = valid && imageEnumSubset(control, []string{"auto", "opaque", "transparent"})
		case imageControlFormat:
			valid = valid && imageEnumSubset(control, []string{"png", "jpeg", "webp"})
		case imageControlSize:
			valid = valid && control.Kind == CatalogControlImageSize && control.ImageSize.MaximumEdge <= 3840 && control.ImageSize.MaximumPixels <= 8294400
		case imageControlCompression:
			valid = valid && control.Kind == CatalogControlInteger && *control.Maximum <= 100
		case imageControlCount:
			valid = valid && control.Kind == CatalogControlInteger && *control.Minimum >= 1 && *control.Maximum <= 10
		case imageControlStream:
			valid = valid && control.Kind == CatalogControlBoolean
		case imageControlPartials:
			valid = valid && control.Kind == CatalogControlInteger && *control.Minimum == 0 && *control.Maximum <= 3
		case imageControlSurface:
			values := []string{"images"}
			if responses {
				values = append(values, "responses")
			}
			valid = valid && control.Kind == CatalogControlEnum && slices.Equal(control.Values, values)
		case imageControlResponsesModel:
			valid = valid && responses && control.Kind == CatalogControlEnum
		default:
			valid = false
		}
		if !valid {
			return fmt.Errorf("%w: field=%s.controls control=%s reason=unsupported_image_control", ErrInvalidModelCatalog, field, control.ID)
		}
	}
	for _, limit := range offering.Limits {
		valid := !limit.AccountDependent
		switch limit.ID {
		case imageLimitPrompt:
			valid = valid && limit.Unit == "characters" && *limit.Value <= 32000
		case imageLimitOutput:
			valid = valid && limit.Unit == "bytes" && *limit.Value <= imageMaximumOutputBytes
		case imageLimitInputs:
			valid = valid && edits && limit.Unit == "images" && *limit.Value <= 16
		case imageLimitInputBytes:
			valid = valid && edits && limit.Unit == "bytes" && *limit.Value < 50_000_000
		case imageLimitInputPixels:
			valid = valid && edits && limit.Unit == "pixels" && *limit.Value <= 64<<20
		case imageLimitStreamOutputs:
			valid = valid && limit.Unit == "images" && *limit.Value == 1
		case imageLimitResponsesOutputs:
			valid = valid && responses && limit.Unit == "images" && *limit.Value == 1
		default:
			valid = false
		}
		if !valid {
			return fmt.Errorf("%w: field=%s.limits limit=%s reason=unsupported_image_limit", ErrInvalidModelCatalog, field, limit.ID)
		}
	}
	return nil
}

func imageEnumSubset(control CatalogControl, supported []string) bool {
	if control.Kind != CatalogControlEnum {
		return false
	}
	for _, value := range control.Values {
		if !slices.Contains(supported, value) {
			return false
		}
	}
	return true
}

func (constraints CatalogImageSizeConstraints) acceptsDimensions(width, height int) bool {
	if width <= 0 || height <= 0 || width > constraints.MaximumEdge || height > constraints.MaximumEdge {
		return false
	}
	pixels := width * height
	return width%constraints.DimensionMultiple == 0 && height%constraints.DimensionMultiple == 0 && pixels >= constraints.MinimumPixels && pixels <= constraints.MaximumPixels && width <= height*constraints.MaximumAspectRatio && height <= width*constraints.MaximumAspectRatio
}

func (constraints CatalogImageSizeConstraints) acceptsSize(size string) bool {
	if size == "auto" {
		return constraints.Automatic
	}
	widthString, heightString, found := strings.Cut(size, "x")
	width, widthError := strconv.Atoi(widthString)
	height, heightError := strconv.Atoi(heightString)
	return found && widthError == nil && heightError == nil && strconv.Itoa(width) == widthString && strconv.Itoa(height) == heightString && constraints.acceptsDimensions(width, height)
}
