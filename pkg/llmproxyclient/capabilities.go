package llmproxyclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// PublicCapabilitiesPath is the current public capability resource path.
	PublicCapabilitiesPath         = "/api/public/capabilities"
	publicCapabilitiesMaximumBytes = 8 * 1024 * 1024
)

// PublicCapabilityCatalog contains the provider offerings needed by model clients.
type PublicCapabilityCatalog struct {
	Offerings []PublicProviderOffering `json:"offerings"`
}

// PublicProviderOffering describes one selectable provider and model route.
type PublicProviderOffering struct {
	Identifier         string             `json:"identifier"`
	Provider           string             `json:"provider"`
	Model              string             `json:"model"`
	Capabilities       []string           `json:"capabilities"`
	WireContract       string             `json:"wire_contract"`
	ExecutionLifecycle string             `json:"execution_lifecycle"`
	MediaLimits        []PublicMediaLimit `json:"media_limits"`
}

// PublicMediaLimit describes one current media boundary for an offering.
type PublicMediaLimit struct {
	ID           string `json:"id"`
	MediaType    string `json:"media_type"`
	Transport    string `json:"transport"`
	Status       string `json:"status"`
	Value        *int64 `json:"value"`
	Unit         string `json:"unit"`
	Scope        string `json:"scope"`
	Source       string `json:"source"`
	LastVerified string `json:"last_verified"`
}

const (
	publicCapabilityAudioInput = "audio_input"
	publicCapabilityImageInput = "image_input"

	publicWireContractAnthropicMessages      = "anthropic_messages"
	publicWireContractGeminiInteractions     = "gemini_interactions"
	publicWireContractMultipartTranscription = "multipart_transcription"
	publicWireContractOpenAIChatCompletions  = "openai_chat_completions"
	publicWireContractOpenAIResponses        = "openai_responses"
	publicWireContractXAIVideosGenerations   = "xai_videos_generations"

	publicExecutionLifecyclePollable    = "pollable_resource"
	publicExecutionLifecycleSynchronous = "synchronous_completion"

	publicMediaTypeAll   = "all"
	publicMediaTypeAudio = "audio"
	publicMediaTypeImage = "image"

	publicMediaTransportAny    = "any"
	publicMediaTransportFile   = "file"
	publicMediaTransportInline = "inline"

	publicMediaLimitInlineRequestBytes = "inline_request_bytes"
	publicMediaLimitImageCount         = "image_count"
	publicMediaLimitAudioCount         = "audio_count"
	publicMediaLimitImageInlineBytes   = "image_inline_bytes"
	publicMediaLimitAudioInlineBytes   = "audio_inline_bytes"
	publicMediaLimitImageFileBytes     = "image_file_bytes"
	publicMediaLimitAudioFileBytes     = "audio_file_bytes"

	publicMediaLimitUnitBytes = "bytes"
	publicMediaLimitUnitFiles = "files"

	publicMediaLimitScopeAttachment             = "attachment"
	publicMediaLimitScopeAttachmentEncodedBytes = "attachment_encoded_bytes"
	publicMediaLimitScopeRequest                = "request"
	publicMediaLimitScopeRequestEncodedBytes    = "request_encoded_bytes"
)

var publicCapabilityValues = map[string]struct{}{
	publicCapabilityAudioInput: {}, "dictation": {}, publicCapabilityImageInput: {}, "reasoning": {},
	"text": {}, "video_generation": {}, "web_search": {},
}

type publicOfferingRoute struct {
	wireContract       string
	executionLifecycle string
}

var publicOfferingMediaTransports = map[publicOfferingRoute]string{
	{wireContract: publicWireContractAnthropicMessages, executionLifecycle: publicExecutionLifecycleSynchronous}:      publicMediaTransportInline,
	{wireContract: publicWireContractGeminiInteractions, executionLifecycle: publicExecutionLifecyclePollable}:        publicMediaTransportFile,
	{wireContract: publicWireContractMultipartTranscription, executionLifecycle: publicExecutionLifecycleSynchronous}: "",
	{wireContract: publicWireContractOpenAIChatCompletions, executionLifecycle: publicExecutionLifecycleSynchronous}:  publicMediaTransportInline,
	{wireContract: publicWireContractOpenAIResponses, executionLifecycle: publicExecutionLifecyclePollable}:           publicMediaTransportInline,
	{wireContract: publicWireContractOpenAIResponses, executionLifecycle: publicExecutionLifecycleSynchronous}:        publicMediaTransportInline,
	{wireContract: publicWireContractXAIVideosGenerations, executionLifecycle: publicExecutionLifecyclePollable}:      "",
}

var publicMediaLimitValues = struct {
	mediaTypes map[string]struct{}
	transports map[string]struct{}
	statuses   map[string]struct{}
	units      map[string]struct{}
	scopes     map[string]struct{}
}{
	mediaTypes: map[string]struct{}{publicMediaTypeAll: {}, publicMediaTypeAudio: {}, publicMediaTypeImage: {}},
	transports: map[string]struct{}{publicMediaTransportAny: {}, publicMediaTransportFile: {}, publicMediaTransportInline: {}},
	statuses:   map[string]struct{}{"bounded": {}, "unbounded": {}, "unknown": {}},
	units:      map[string]struct{}{publicMediaLimitUnitBytes: {}, publicMediaLimitUnitFiles: {}},
	scopes: map[string]struct{}{
		publicMediaLimitScopeAttachment: {}, publicMediaLimitScopeAttachmentEncodedBytes: {}, publicMediaLimitScopeRequest: {}, publicMediaLimitScopeRequestEncodedBytes: {},
	},
}

// GetPublicCapabilities reads the current public capability catalog.
func (client Client) GetPublicCapabilities(contextValue context.Context) (PublicCapabilityCatalog, error) {
	requestURL := client.config.publicCapabilitiesURL()
	request := (&http.Request{Method: http.MethodGet, URL: &requestURL, Header: http.Header{}}).WithContext(contextValue)
	request.Header.Set(headerAccept, "application/json")
	response, requestError := client.httpClient.Do(request)
	if requestError != nil {
		return PublicCapabilityCatalog{}, fmt.Errorf("%w: read public capabilities", ErrClientHTTPFailure)
	}
	if response == nil || response.Body == nil {
		return PublicCapabilityCatalog{}, fmt.Errorf("%w: invalid public capabilities response", ErrClientHTTPFailure)
	}
	defer response.Body.Close()
	responseBody, readError := io.ReadAll(io.LimitReader(response.Body, publicCapabilitiesMaximumBytes+1))
	if readError != nil || len(responseBody) > publicCapabilitiesMaximumBytes {
		return PublicCapabilityCatalog{}, fmt.Errorf("%w: read public capabilities response", ErrClientHTTPFailure)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return PublicCapabilityCatalog{}, newHTTPFailure(response.StatusCode, responseBody)
	}
	var catalog PublicCapabilityCatalog
	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	if decodeError := decoder.Decode(&catalog); decodeError != nil {
		return PublicCapabilityCatalog{}, fmt.Errorf("%w: decode public capabilities response", ErrClientHTTPFailure)
	}
	if trailingError := decoder.Decode(&struct{}{}); trailingError != io.EOF {
		return PublicCapabilityCatalog{}, fmt.Errorf("%w: decode public capabilities response", ErrClientHTTPFailure)
	}
	if !validPublicCapabilityCatalog(catalog) {
		return PublicCapabilityCatalog{}, fmt.Errorf("%w: invalid public capabilities response", ErrClientHTTPFailure)
	}
	return catalog, nil
}

func validPublicCapabilityCatalog(catalog PublicCapabilityCatalog) bool {
	if len(catalog.Offerings) == 0 {
		return false
	}
	seenOfferings := make(map[string]struct{}, len(catalog.Offerings))
	for _, offering := range catalog.Offerings {
		if !validPublicProviderOffering(offering) {
			return false
		}
		if _, duplicate := seenOfferings[offering.Identifier]; duplicate {
			return false
		}
		seenOfferings[offering.Identifier] = struct{}{}
	}
	return true
}

func validPublicProviderOffering(offering PublicProviderOffering) bool {
	if offering.Identifier != offering.Provider+":"+offering.Model || !canonicalPublicValue(offering.Provider) || !canonicalPublicValue(offering.Model) || len(offering.Capabilities) == 0 || offering.MediaLimits == nil {
		return false
	}
	mediaTransport, routeValid := publicOfferingMediaTransports[publicOfferingRoute{wireContract: offering.WireContract, executionLifecycle: offering.ExecutionLifecycle}]
	if !routeValid {
		return false
	}
	seenCapabilities := make(map[string]struct{}, len(offering.Capabilities))
	for _, capability := range offering.Capabilities {
		if _, supported := publicCapabilityValues[capability]; !supported {
			return false
		}
		if _, duplicate := seenCapabilities[capability]; duplicate {
			return false
		}
		seenCapabilities[capability] = struct{}{}
	}
	seenLimits := make(map[string]PublicMediaLimit, len(offering.MediaLimits))
	for _, limit := range offering.MediaLimits {
		if !validPublicMediaLimit(limit) {
			return false
		}
		if limit.MediaType == publicMediaTypeImage {
			if _, supported := seenCapabilities[publicCapabilityImageInput]; !supported {
				return false
			}
		}
		if limit.MediaType == publicMediaTypeAudio {
			if _, supported := seenCapabilities[publicCapabilityAudioInput]; !supported {
				return false
			}
		}
		if _, duplicate := seenLimits[limit.ID]; duplicate {
			return false
		}
		seenLimits[limit.ID] = limit
	}
	return validPublicOfferingMediaLimits(seenCapabilities, seenLimits, mediaTransport)
}

func validPublicOfferingMediaLimits(capabilities map[string]struct{}, limits map[string]PublicMediaLimit, mediaTransport string) bool {
	_, hasImage := capabilities[publicCapabilityImageInput]
	_, hasAudio := capabilities[publicCapabilityAudioInput]
	if !hasImage && !hasAudio {
		return len(limits) == 0
	}
	if mediaTransport == "" || !matchesPublicMediaLimit(limits[publicMediaLimitInlineRequestBytes], PublicMediaLimit{
		ID: publicMediaLimitInlineRequestBytes, MediaType: publicMediaTypeAll, Transport: publicMediaTransportInline,
		Unit: publicMediaLimitUnitBytes, Scope: publicMediaLimitScopeRequestEncodedBytes,
	}) {
		return false
	}
	if hasImage && !validPublicMediaTypeLimits(limits, publicMediaTypeImage, mediaTransport) {
		return false
	}
	return !hasAudio || validPublicMediaTypeLimits(limits, publicMediaTypeAudio, mediaTransport)
}

func validPublicMediaTypeLimits(limits map[string]PublicMediaLimit, mediaType string, mediaTransport string) bool {
	countIdentifier := publicMediaLimitImageCount
	inlineIdentifier := publicMediaLimitImageInlineBytes
	fileIdentifier := publicMediaLimitImageFileBytes
	if mediaType == publicMediaTypeAudio {
		countIdentifier = publicMediaLimitAudioCount
		inlineIdentifier = publicMediaLimitAudioInlineBytes
		fileIdentifier = publicMediaLimitAudioFileBytes
	}
	if !matchesPublicMediaLimit(limits[countIdentifier], PublicMediaLimit{
		ID: countIdentifier, MediaType: mediaType, Transport: publicMediaTransportAny,
		Unit: publicMediaLimitUnitFiles, Scope: publicMediaLimitScopeRequest,
	}) {
		return false
	}
	if mediaTransport == publicMediaTransportFile {
		return matchesPublicMediaLimit(limits[fileIdentifier], PublicMediaLimit{
			ID: fileIdentifier, MediaType: mediaType, Transport: publicMediaTransportFile,
			Unit: publicMediaLimitUnitBytes, Scope: publicMediaLimitScopeAttachment,
		})
	}
	limit := limits[inlineIdentifier]
	return matchesPublicMediaLimit(limit, PublicMediaLimit{
		ID: inlineIdentifier, MediaType: mediaType, Transport: publicMediaTransportInline,
		Unit: publicMediaLimitUnitBytes,
	}) && (limit.Scope == publicMediaLimitScopeAttachment || limit.Scope == publicMediaLimitScopeAttachmentEncodedBytes)
}

func matchesPublicMediaLimit(limit PublicMediaLimit, required PublicMediaLimit) bool {
	return limit.ID == required.ID && limit.MediaType == required.MediaType && limit.Transport == required.Transport && limit.Unit == required.Unit && (required.Scope == "" || limit.Scope == required.Scope)
}

func validPublicMediaLimit(limit PublicMediaLimit) bool {
	_, mediaTypeValid := publicMediaLimitValues.mediaTypes[limit.MediaType]
	_, transportValid := publicMediaLimitValues.transports[limit.Transport]
	_, statusValid := publicMediaLimitValues.statuses[limit.Status]
	_, unitValid := publicMediaLimitValues.units[limit.Unit]
	_, scopeValid := publicMediaLimitValues.scopes[limit.Scope]
	boundedValueValid := limit.Status == "bounded" && limit.Value != nil && *limit.Value > 0
	unboundedValueValid := (limit.Status == "unbounded" || limit.Status == "unknown") && limit.Value == nil
	sourceURL, sourceError := url.ParseRequestURI(limit.Source)
	sourceValid := sourceError == nil && sourceURL != nil && sourceURL.Scheme == "https" && sourceURL.Host != "" && limit.Source == strings.TrimSpace(limit.Source)
	_, dateError := time.Parse(time.DateOnly, limit.LastVerified)
	return canonicalPublicValue(limit.ID) && mediaTypeValid && transportValid && statusValid && unitValid && scopeValid && (boundedValueValid || unboundedValueValid) && sourceValid && dateError == nil
}

func canonicalPublicValue(value string) bool {
	return value != "" && value == strings.TrimSpace(value)
}

func (config Config) publicCapabilitiesURL() url.URL {
	requestURL := *config.baseURL
	requestURL.Path = publicCapabilitiesEndpointPath(requestURL.Path)
	requestURL.RawQuery = ""
	requestURL.Fragment = ""
	return requestURL
}

func publicCapabilitiesEndpointPath(basePath string) string {
	trimmedPath := strings.TrimRight(strings.TrimSpace(basePath), "/")
	trimmedPath = strings.TrimSuffix(trimmedPath, "/v2")
	if trimmedPath == "" {
		return PublicCapabilitiesPath
	}
	return trimmedPath + PublicCapabilitiesPath
}
