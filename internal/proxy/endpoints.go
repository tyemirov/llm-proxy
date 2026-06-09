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
	accessMutex       sync.RWMutex
	responsesURL      string
	modelsURL         string
	transcriptionsURL string
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
		responsesURL:      baseURL + "/responses",
		modelsURL:         baseURL + "/models",
		transcriptionsURL: transcriptionsURL,
	}
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
