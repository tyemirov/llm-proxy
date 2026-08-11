package proxy

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	CatalogMediaLimitStatusBounded   = "bounded"
	CatalogMediaLimitStatusUnbounded = "unbounded"
	CatalogMediaLimitStatusUnknown   = "unknown"

	CatalogMediaTransportAny    = "any"
	CatalogMediaTransportFile   = "file"
	CatalogMediaTransportInline = "inline"

	CatalogMediaLimitUnitBytes = "bytes"
	CatalogMediaLimitUnitFiles = "files"

	CatalogMediaLimitScopeAttachment          = "attachment"
	CatalogMediaLimitScopeRequest             = "request"
	CatalogMediaLimitScopeRequestEncodedBytes = "request_encoded_bytes"

	CatalogMediaLimitTypeAll = "all"

	CatalogMediaLimitIDInlineRequestBytes = "inline_request_bytes"
	CatalogMediaLimitIDImageCount         = "image_count"
	CatalogMediaLimitIDAudioCount         = "audio_count"
	CatalogMediaLimitIDImageFileBytes     = "image_file_bytes"
	CatalogMediaLimitIDAudioFileBytes     = "audio_file_bytes"
)

// CatalogMediaLimit declares one provider-offering media admission rule.
type CatalogMediaLimit struct {
	ID           string `json:"id" mapstructure:"id"`
	MediaType    string `json:"media_type" mapstructure:"media_type"`
	Transport    string `json:"transport" mapstructure:"transport"`
	Status       string `json:"status" mapstructure:"status"`
	Value        *int64 `json:"value" mapstructure:"value"`
	Unit         string `json:"unit" mapstructure:"unit"`
	Scope        string `json:"scope" mapstructure:"scope"`
	Source       string `json:"source" mapstructure:"source"`
	LastVerified string `json:"last_verified" mapstructure:"last_verified"`
}

func cloneCatalogMediaLimits(limits []CatalogMediaLimit) []CatalogMediaLimit {
	cloned := make([]CatalogMediaLimit, len(limits))
	for limitIndex, limit := range limits {
		cloned[limitIndex] = limit
		if limit.Value != nil {
			value := *limit.Value
			cloned[limitIndex].Value = &value
		}
	}
	return cloned
}

func validateCatalogMediaLimits(limits []CatalogMediaLimit, mediaInputs []string, field string) error {
	if len(mediaInputs) == 0 {
		if len(limits) != 0 {
			return fmt.Errorf("%w: field=%s reason=media_limits_without_media_inputs", ErrInvalidModelCatalog, field)
		}
		return nil
	}
	if len(limits) == 0 {
		return fmt.Errorf("%w: field=%s reason=media_limits_missing", ErrInvalidModelCatalog, field)
	}
	mediaInputSet := configuredMediaInputSet(mediaInputs)
	seen := map[string]struct{}{}
	configured := map[string]CatalogMediaLimit{}
	for limitIndex, limit := range limits {
		limitField := fmt.Sprintf("%s[%d]", field, limitIndex)
		identifier, identifierError := canonicalCatalogIdentifier(limit.ID, limitField+".id")
		if identifierError != nil {
			return identifierError
		}
		if _, duplicate := seen[identifier]; duplicate {
			return fmt.Errorf("%w: field=%s.id duplicate_identifier=%s", ErrInvalidModelCatalog, limitField, identifier)
		}
		seen[identifier] = struct{}{}
		configured[identifier] = limit
		if limit.MediaType != CatalogMediaLimitTypeAll {
			if _, supported := mediaInputSet[messageMediaType(limit.MediaType)]; !supported {
				return fmt.Errorf("%w: field=%s.media_type media_type=%s", ErrInvalidModelCatalog, limitField, limit.MediaType)
			}
		}
		if limit.Transport != CatalogMediaTransportAny && limit.Transport != CatalogMediaTransportInline && limit.Transport != CatalogMediaTransportFile {
			return fmt.Errorf("%w: field=%s.transport transport=%s", ErrInvalidModelCatalog, limitField, limit.Transport)
		}
		if limit.Unit != CatalogMediaLimitUnitBytes && limit.Unit != CatalogMediaLimitUnitFiles {
			return fmt.Errorf("%w: field=%s.unit unit=%s", ErrInvalidModelCatalog, limitField, limit.Unit)
		}
		if limit.Scope != CatalogMediaLimitScopeAttachment && limit.Scope != CatalogMediaLimitScopeRequest && limit.Scope != CatalogMediaLimitScopeRequestEncodedBytes {
			return fmt.Errorf("%w: field=%s.scope scope=%s", ErrInvalidModelCatalog, limitField, limit.Scope)
		}
		switch limit.Status {
		case CatalogMediaLimitStatusBounded:
			if limit.Value == nil || *limit.Value <= 0 {
				return fmt.Errorf("%w: field=%s.value", ErrInvalidModelCatalog, limitField)
			}
		case CatalogMediaLimitStatusUnbounded, CatalogMediaLimitStatusUnknown:
			if limit.Value != nil {
				return fmt.Errorf("%w: field=%s.value status=%s", ErrInvalidModelCatalog, limitField, limit.Status)
			}
		default:
			return fmt.Errorf("%w: field=%s.status status=%s", ErrInvalidModelCatalog, limitField, limit.Status)
		}
		parsedSource, sourceError := url.ParseRequestURI(strings.TrimSpace(limit.Source))
		if sourceError != nil || parsedSource.Scheme != "https" || parsedSource.Host == "" || limit.Source != strings.TrimSpace(limit.Source) {
			return fmt.Errorf("%w: field=%s.source", ErrInvalidModelCatalog, limitField)
		}
		if _, dateError := time.Parse(time.DateOnly, limit.LastVerified); dateError != nil {
			return fmt.Errorf("%w: field=%s.last_verified", ErrInvalidModelCatalog, limitField)
		}
	}
	requiredLimits := []CatalogMediaLimit{
		{ID: CatalogMediaLimitIDInlineRequestBytes, MediaType: CatalogMediaLimitTypeAll, Transport: CatalogMediaTransportInline, Unit: CatalogMediaLimitUnitBytes, Scope: CatalogMediaLimitScopeRequestEncodedBytes},
	}
	for _, mediaInput := range mediaInputs {
		switch messageMediaType(mediaInput) {
		case messageMediaTypeImage:
			requiredLimits = append(requiredLimits,
				CatalogMediaLimit{ID: CatalogMediaLimitIDImageCount, MediaType: mediaInput, Transport: CatalogMediaTransportAny, Unit: CatalogMediaLimitUnitFiles, Scope: CatalogMediaLimitScopeRequest},
				CatalogMediaLimit{ID: CatalogMediaLimitIDImageFileBytes, MediaType: mediaInput, Transport: CatalogMediaTransportFile, Unit: CatalogMediaLimitUnitBytes, Scope: CatalogMediaLimitScopeAttachment},
			)
		case messageMediaTypeAudio:
			requiredLimits = append(requiredLimits,
				CatalogMediaLimit{ID: CatalogMediaLimitIDAudioCount, MediaType: mediaInput, Transport: CatalogMediaTransportAny, Unit: CatalogMediaLimitUnitFiles, Scope: CatalogMediaLimitScopeRequest},
				CatalogMediaLimit{ID: CatalogMediaLimitIDAudioFileBytes, MediaType: mediaInput, Transport: CatalogMediaTransportFile, Unit: CatalogMediaLimitUnitBytes, Scope: CatalogMediaLimitScopeAttachment},
			)
		}
	}
	for _, requiredLimit := range requiredLimits {
		limit, exists := configured[requiredLimit.ID]
		if !exists || limit.MediaType != requiredLimit.MediaType || limit.Transport != requiredLimit.Transport || limit.Unit != requiredLimit.Unit || limit.Scope != requiredLimit.Scope {
			return fmt.Errorf("%w: field=%s required_limit=%s", ErrInvalidModelCatalog, field, requiredLimit.ID)
		}
	}
	return nil
}

func boundedCatalogMediaLimit(limits []CatalogMediaLimit, identifier string, mediaType messageMediaType) (int64, bool) {
	limit, found := catalogMediaLimit(limits, identifier, mediaType)
	if !found || limit.Status != CatalogMediaLimitStatusBounded {
		return 0, false
	}
	return *limit.Value, true
}

func catalogMediaLimit(limits []CatalogMediaLimit, identifier string, mediaType messageMediaType) (CatalogMediaLimit, bool) {
	for _, limit := range limits {
		if limit.ID != identifier || (limit.MediaType != CatalogMediaLimitTypeAll && limit.MediaType != string(mediaType)) {
			continue
		}
		return limit, true
	}
	return CatalogMediaLimit{}, false
}
