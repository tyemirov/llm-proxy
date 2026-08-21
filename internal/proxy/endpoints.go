package proxy

import (
	"strings"
	"sync"
)

const (
	defaultOpenAIBaseURL     = "https://api.openai.com/v1"
	defaultResponsesURL      = defaultOpenAIBaseURL + "/responses"
	defaultModelsURL         = defaultOpenAIBaseURL + "/models"
	defaultTranscriptionsURL = defaultOpenAIBaseURL + "/audio/transcriptions"
)

// Endpoints provides concurrency-safe access to OpenAI endpoint URLs.
type Endpoints struct {
	accessMutex              sync.RWMutex
	responsesURL             string
	modelsURL                string
	transcriptionsURL        string
	providerBaseURL          map[string]string
	providerTransportBaseURL map[string]string
	providerTransportURL     map[string]string
}

// NewEndpoints creates an Endpoints instance initialized with default URLs.
func NewEndpoints() *Endpoints {
	return NewEndpointsForURLs(defaultOpenAIBaseURL, defaultTranscriptionsURL)
}

// NewEndpointsForURLs creates an Endpoints instance from configured OpenAI URLs.
func NewEndpointsForURLs(rawBaseURL string, rawTranscriptionsURL string) *Endpoints {
	baseURL := strings.TrimRight(strings.TrimSpace(rawBaseURL), "/")
	transcriptionsURL := strings.TrimSpace(rawTranscriptionsURL)
	return &Endpoints{
		responsesURL:             baseURL + "/responses",
		modelsURL:                baseURL + "/models",
		transcriptionsURL:        transcriptionsURL,
		providerBaseURL:          map[string]string{},
		providerTransportBaseURL: map[string]string{},
		providerTransportURL:     map[string]string{},
	}
}

func newEndpointsForResponsesURL(rawResponsesURL string) *Endpoints {
	return &Endpoints{
		responsesURL:             strings.TrimSpace(rawResponsesURL),
		providerBaseURL:          map[string]string{},
		providerTransportBaseURL: map[string]string{},
		providerTransportURL:     map[string]string{},
	}
}

func providerTransportEndpointKey(provider string, transport string) string {
	return strings.TrimSpace(provider) + "\x00" + strings.TrimSpace(transport)
}

// SetProviderBaseURL replaces the catalog base URL for every transport of one provider.
// It is intended for black-box tests that route catalog adapters to a fake upstream.
func (endpointConfiguration *Endpoints) SetProviderBaseURL(provider string, rawBaseURL string) {
	endpointConfiguration.accessMutex.Lock()
	defer endpointConfiguration.accessMutex.Unlock()
	if endpointConfiguration.providerBaseURL == nil {
		endpointConfiguration.providerBaseURL = map[string]string{}
	}
	endpointConfiguration.providerBaseURL[strings.TrimSpace(provider)] = strings.TrimRight(strings.TrimSpace(rawBaseURL), "/")
}

// SetProviderTransportBaseURL replaces the catalog base URL for one provider transport.
// It is intended for black-box tests that route a catalog adapter to a fake upstream.
func (endpointConfiguration *Endpoints) SetProviderTransportBaseURL(provider string, transport string, rawBaseURL string) {
	endpointConfiguration.accessMutex.Lock()
	defer endpointConfiguration.accessMutex.Unlock()
	if endpointConfiguration.providerTransportBaseURL == nil {
		endpointConfiguration.providerTransportBaseURL = map[string]string{}
	}
	endpointConfiguration.providerTransportBaseURL[providerTransportEndpointKey(provider, transport)] = strings.TrimRight(strings.TrimSpace(rawBaseURL), "/")
}

// SetProviderTransportURL replaces the complete catalog URL for one provider transport.
// It is intended for black-box tests that route a catalog adapter to a fake upstream.
func (endpointConfiguration *Endpoints) SetProviderTransportURL(provider string, transport string, rawURL string) {
	endpointConfiguration.accessMutex.Lock()
	defer endpointConfiguration.accessMutex.Unlock()
	if endpointConfiguration.providerTransportURL == nil {
		endpointConfiguration.providerTransportURL = map[string]string{}
	}
	endpointConfiguration.providerTransportURL[providerTransportEndpointKey(provider, transport)] = strings.TrimSpace(rawURL)
}

func (endpointConfiguration *Endpoints) providerTransportEndpoint(provider string, transport string, catalogEndpoint ProviderCatalogEndpoint) string {
	endpointConfiguration.accessMutex.RLock()
	defer endpointConfiguration.accessMutex.RUnlock()
	key := providerTransportEndpointKey(provider, transport)
	if endpointURL := endpointConfiguration.providerTransportURL[key]; endpointURL != "" {
		return endpointURL
	}
	if baseURL := endpointConfiguration.providerTransportBaseURL[key]; baseURL != "" {
		return baseURL + catalogEndpoint.Path
	}
	if baseURL := endpointConfiguration.providerBaseURL[strings.TrimSpace(provider)]; baseURL != "" {
		return baseURL + catalogEndpoint.Path
	}
	return ""
}

// GetResponsesURL returns the URL used for the OpenAI responses endpoint.
func (endpointConfiguration *Endpoints) GetResponsesURL() string {
	endpointConfiguration.accessMutex.RLock()
	defer endpointConfiguration.accessMutex.RUnlock()
	return endpointConfiguration.responsesURL
}

// SetResponsesURL sets the URL for the OpenAI responses endpoint.
func (endpointConfiguration *Endpoints) SetResponsesURL(newURL string) {
	endpointConfiguration.accessMutex.Lock()
	defer endpointConfiguration.accessMutex.Unlock()
	endpointConfiguration.responsesURL = newURL
}

// ResetResponsesURL resets the responses endpoint to the default.
func (endpointConfiguration *Endpoints) ResetResponsesURL() {
	endpointConfiguration.accessMutex.Lock()
	defer endpointConfiguration.accessMutex.Unlock()
	endpointConfiguration.responsesURL = defaultResponsesURL
}

// GetModelsURL returns the URL used for the OpenAI models endpoint.
func (endpointConfiguration *Endpoints) GetModelsURL() string {
	endpointConfiguration.accessMutex.RLock()
	defer endpointConfiguration.accessMutex.RUnlock()
	return endpointConfiguration.modelsURL
}

// SetModelsURL sets the URL for the OpenAI models endpoint.
func (endpointConfiguration *Endpoints) SetModelsURL(newURL string) {
	endpointConfiguration.accessMutex.Lock()
	defer endpointConfiguration.accessMutex.Unlock()
	endpointConfiguration.modelsURL = newURL
}

// ResetModelsURL resets the models endpoint to the default.
func (endpointConfiguration *Endpoints) ResetModelsURL() {
	endpointConfiguration.accessMutex.Lock()
	defer endpointConfiguration.accessMutex.Unlock()
	endpointConfiguration.modelsURL = defaultModelsURL
}

// GetTranscriptionsURL returns the URL used for the OpenAI audio transcriptions endpoint.
func (endpointConfiguration *Endpoints) GetTranscriptionsURL() string {
	endpointConfiguration.accessMutex.RLock()
	defer endpointConfiguration.accessMutex.RUnlock()
	return endpointConfiguration.transcriptionsURL
}

// SetTranscriptionsURL sets the URL for the OpenAI audio transcriptions endpoint.
func (endpointConfiguration *Endpoints) SetTranscriptionsURL(newURL string) {
	endpointConfiguration.accessMutex.Lock()
	defer endpointConfiguration.accessMutex.Unlock()
	endpointConfiguration.transcriptionsURL = newURL
}

// ResetTranscriptionsURL resets the transcriptions endpoint to the default.
func (endpointConfiguration *Endpoints) ResetTranscriptionsURL() {
	endpointConfiguration.accessMutex.Lock()
	defer endpointConfiguration.accessMutex.Unlock()
	endpointConfiguration.transcriptionsURL = defaultTranscriptionsURL
}
