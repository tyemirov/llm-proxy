package proxy

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"slices"
	"strings"
)

// validateCatalogImageInputs validates format restrictions and dimension descriptors at catalog import.
func validateCatalogImageInputs(offering ProviderOffering, route textRouteCapabilities, field string) error {
	seen := map[string]struct{}{}
	for _, mimeType := range offering.ImageMIMETypes {
		_, duplicate := seen[mimeType]
		if duplicate || !slices.Contains(offering.MediaInputs, string(messageMediaTypeImage)) || !textRouteSupportsMessageMediaMIME(route, messageMediaTypeImage, mimeType) {
			return fmt.Errorf("%w: field=%s.image_mime_types mime_type=%s", ErrInvalidModelCatalog, field, mimeType)
		}
		seen[mimeType] = struct{}{}
	}
	for _, limit := range offering.MediaLimits {
		dimension := limit.ID == CatalogMediaLimitIDImageWidthPixels || limit.ID == CatalogMediaLimitIDImageHeightPixels
		if !dimension && limit.Unit != CatalogMediaLimitUnitPixels {
			continue
		}
		if !dimension || limit.MediaType != string(messageMediaTypeImage) || limit.Unit != CatalogMediaLimitUnitPixels || limit.Scope != CatalogMediaLimitScopeAttachment || limit.Transport != CatalogMediaTransportAny {
			return fmt.Errorf("%w: field=%s.media_limits dimension=%s", ErrInvalidModelCatalog, field, limit.ID)
		}
		if len(offering.ImageMIMETypes) == 0 {
			return fmt.Errorf("%w: field=%s.image_mime_types reason=dimension_formats_missing", ErrInvalidModelCatalog, field)
		}
		for _, mimeType := range offering.ImageMIMETypes {
			if mimeType != messageImageMIMEPNG && mimeType != messageImageMIMEJPEG {
				return fmt.Errorf("%w: field=%s.image_mime_types reason=dimension_decoder_missing mime_type=%s", ErrInvalidModelCatalog, field, mimeType)
			}
		}
	}
	return nil
}

func validateImageDimensions(model textModelDefinition, attachment messageMedia) error {
	width, widthBounded := boundedCatalogMediaLimit(model.mediaLimits, CatalogMediaLimitIDImageWidthPixels, messageMediaTypeImage)
	height, heightBounded := boundedCatalogMediaLimit(model.mediaLimits, CatalogMediaLimitIDImageHeightPixels, messageMediaTypeImage)
	if attachment.mediaType != messageMediaTypeImage || (!widthBounded && !heightBounded) {
		return nil
	}
	reader, err := attachment.reader()
	if err != nil {
		return fmt.Errorf("inspect image dimensions: %w", err)
	}
	dimensions, format, err := image.DecodeConfig(reader)
	if err != nil || format != strings.TrimPrefix(attachment.mimeType, "image/") {
		return fmt.Errorf("%w: image header does not match mime_type=%s", ErrInvalidChatMessages, attachment.mimeType)
	}
	if (widthBounded && int64(dimensions.Width) > width) || (heightBounded && int64(dimensions.Height) > height) {
		return fmt.Errorf("%w: image dimensions exceed model=%s limit", ErrProviderMediaLimit, model.string())
	}
	return nil
}
