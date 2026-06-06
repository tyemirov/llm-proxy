// Package llmproxyclient provides an HTTP client for llm-proxy JSON POST requests.
package llmproxyclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	formatQueryValueTextPlain = "text/plain"
	headerAccept              = "Accept"
	headerContentType         = "Content-Type"
	jsonContentType           = "application/json; charset=utf-8"
	queryFormat               = "format"
	queryKey                  = "key"
	queryProvider             = "provider"
)

var (
	// ErrInvalidClientConfig reports invalid llm-proxy client configuration.
	ErrInvalidClientConfig = errors.New("llm_proxy_client_invalid_config")
	// ErrInvalidClientRequest reports invalid llm-proxy request input.
	ErrInvalidClientRequest = errors.New("llm_proxy_client_invalid_request")
	// ErrClientHTTPFailure reports an unsuccessful llm-proxy HTTP response.
	ErrClientHTTPFailure = errors.New("llm_proxy_client_http_failure")
)

var postBodyQueryKeys = map[string]struct{}{
	"messages":          {},
	"model":             {},
	"max_output_tokens": {},
	"max_tokens":        {},
	"prompt":            {},
	"system_prompt":     {},
	"web_search":        {},
}

// HTTPDoer performs one HTTP request.
type HTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

// ConfigInput is the unvalidated external configuration for an llm-proxy client.
type ConfigInput struct {
	BaseURL  string
	Secret   string
	Provider string
	Timeout  time.Duration
}

// Config is validated llm-proxy client configuration.
type Config struct {
	baseURL  *url.URL
	secret   string
	provider string
	timeout  time.Duration
}

// NewConfig validates external client configuration.
func NewConfig(input ConfigInput) (Config, error) {
	trimmedBaseURL := strings.TrimSpace(input.BaseURL)
	if trimmedBaseURL == "" {
		return Config{}, fmt.Errorf("%w: missing base_url", ErrInvalidClientConfig)
	}
	parsedBaseURL, parseError := url.Parse(trimmedBaseURL)
	if parseError != nil {
		return Config{}, fmt.Errorf("%w: parse base_url: %v", ErrInvalidClientConfig, parseError)
	}
	if parsedBaseURL.Scheme != "http" && parsedBaseURL.Scheme != "https" {
		return Config{}, fmt.Errorf("%w: base_url must use http or https", ErrInvalidClientConfig)
	}
	if parsedBaseURL.Host == "" {
		return Config{}, fmt.Errorf("%w: base_url must include host", ErrInvalidClientConfig)
	}
	trimmedSecret := strings.TrimSpace(input.Secret)
	if trimmedSecret == "" {
		return Config{}, fmt.Errorf("%w: missing secret", ErrInvalidClientConfig)
	}
	if input.Timeout <= 0 {
		return Config{}, fmt.Errorf("%w: timeout must be positive", ErrInvalidClientConfig)
	}
	return Config{
		baseURL:  parsedBaseURL,
		secret:   trimmedSecret,
		provider: strings.TrimSpace(input.Provider),
		timeout:  input.Timeout,
	}, nil
}

// Timeout returns the validated client timeout.
func (config Config) Timeout() time.Duration {
	return config.timeout
}

// PostURL builds the authenticated JSON POST URL for this config.
func (config Config) PostURL() string {
	requestURL := config.postURL()
	return requestURL.String()
}

func (config Config) postURL() url.URL {
	requestURL := *config.baseURL
	if requestURL.Path == "" {
		requestURL.Path = "/"
	}
	queryValues := requestURL.Query()
	for queryKeyName := range postBodyQueryKeys {
		queryValues.Del(queryKeyName)
	}
	queryValues.Set(queryKey, config.secret)
	queryValues.Set(queryFormat, formatQueryValueTextPlain)
	if config.provider != "" {
		queryValues.Set(queryProvider, config.provider)
	}
	requestURL.RawQuery = queryValues.Encode()
	return requestURL
}

// RequestInput is the unvalidated external input for a JSON POST request.
type RequestInput struct {
	Prompt       string
	Model        string
	WebSearch    bool
	SystemPrompt string
	MaxTokens    *int
}

// Request is a validated llm-proxy JSON POST request.
type Request struct {
	prompt       string
	model        string
	webSearch    bool
	systemPrompt string
	maxTokens    *int
}

// NewRequest validates external request input.
func NewRequest(input RequestInput) (Request, error) {
	if input.Prompt == "" {
		return Request{}, fmt.Errorf("%w: missing prompt", ErrInvalidClientRequest)
	}
	if input.MaxTokens != nil && *input.MaxTokens <= 0 {
		return Request{}, fmt.Errorf("%w: max_tokens must be positive", ErrInvalidClientRequest)
	}
	return Request{
		prompt:       input.Prompt,
		model:        strings.TrimSpace(input.Model),
		webSearch:    input.WebSearch,
		systemPrompt: strings.TrimSpace(input.SystemPrompt),
		maxTokens:    input.MaxTokens,
	}, nil
}

func (request Request) payloadBody() []byte {
	var body strings.Builder
	body.WriteString(`{"prompt":`)
	body.WriteString(strconv.Quote(request.prompt))
	if request.model != "" {
		body.WriteString(`,"model":`)
		body.WriteString(strconv.Quote(request.model))
	}
	body.WriteString(`,"web_search":`)
	body.WriteString(strconv.FormatBool(request.webSearch))
	if request.systemPrompt != "" {
		body.WriteString(`,"system_prompt":`)
		body.WriteString(strconv.Quote(request.systemPrompt))
	}
	if request.maxTokens != nil {
		body.WriteString(`,"max_tokens":`)
		body.WriteString(strconv.Itoa(*request.maxTokens))
	}
	body.WriteByte('}')
	return []byte(body.String())
}

// Client calls llm-proxy using a configured HTTP transport.
type Client struct {
	config     Config
	httpClient HTTPDoer
}

// NewClient creates a client from validated config and an injected HTTP transport.
func NewClient(config Config, httpClient HTTPDoer) (Client, error) {
	if httpClient == nil {
		return Client{}, fmt.Errorf("%w: missing http client", ErrInvalidClientConfig)
	}
	return Client{config: config, httpClient: httpClient}, nil
}

// Post sends a JSON POST prompt request and returns the response text.
func (client Client) Post(contextValue context.Context, request Request) (string, error) {
	requestBody := request.payloadBody()
	requestURL := client.config.postURL()
	httpRequest := (&http.Request{
		Method:        http.MethodPost,
		URL:           &requestURL,
		Header:        http.Header{},
		Body:          io.NopCloser(bytes.NewReader(requestBody)),
		ContentLength: int64(len(requestBody)),
	}).WithContext(contextValue)
	httpRequest.Header.Set(headerAccept, formatQueryValueTextPlain)
	httpRequest.Header.Set(headerContentType, jsonContentType)

	httpResponse, httpError := client.httpClient.Do(httpRequest)
	if httpError != nil {
		return "", fmt.Errorf("%w: post request: %v", ErrClientHTTPFailure, httpError)
	}
	defer httpResponse.Body.Close()
	responseBody, readError := io.ReadAll(httpResponse.Body)
	if readError != nil {
		return "", fmt.Errorf("%w: read response body: %v", ErrClientHTTPFailure, readError)
	}
	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf(
			"%w: status=%d body=%q",
			ErrClientHTTPFailure,
			httpResponse.StatusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}
	return string(responseBody), nil
}
