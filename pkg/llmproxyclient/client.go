// Package llmproxyclient provides an HTTP client for llm-proxy v2 JSON POST requests.
package llmproxyclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
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
	queryModel                = "model"
	queryProvider             = "provider"
)

const (
	messageRoleSystem    = "system"
	messageRoleUser      = "user"
	messageRoleAssistant = "assistant"
)

var (
	// ErrInvalidClientConfig reports invalid llm-proxy client configuration.
	ErrInvalidClientConfig = errors.New("llm_proxy_client_invalid_config")
	// ErrInvalidClientRequest reports invalid llm-proxy request input.
	ErrInvalidClientRequest = errors.New("llm_proxy_client_invalid_request")
	// ErrInvalidModelProfile reports an unreadable or invalid model-profile document.
	ErrInvalidModelProfile = errors.New("llm_proxy_client_invalid_model_profile")
	// ErrClientHTTPFailure reports an unsuccessful llm-proxy HTTP response.
	ErrClientHTTPFailure = errors.New("llm_proxy_client_http_failure")
)

var postBodyQueryKeys = map[string]struct{}{
	"messages":          {},
	queryModel:          {},
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
	BaseURL            string
	Secret             string
	Provider           string
	ModelProfilePath   string
	ModelProfileReader ModelProfileReader
	Timeout            time.Duration
}

// Config is validated llm-proxy client configuration.
type Config struct {
	baseURL            *url.URL
	secret             string
	provider           string
	modelProfilePath   string
	modelProfileReader ModelProfileReader
	timeout            time.Duration
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
	trimmedProvider := strings.TrimSpace(input.Provider)
	trimmedModelProfilePath := strings.TrimSpace(input.ModelProfilePath)
	if trimmedModelProfilePath == "" && input.ModelProfileReader != nil {
		return Config{}, fmt.Errorf("%w: model_profile_reader requires model_profile_path", ErrInvalidClientConfig)
	}
	if trimmedModelProfilePath != "" {
		if input.ModelProfileReader == nil {
			return Config{}, fmt.Errorf("%w: model_profile_path requires model_profile_reader", ErrInvalidClientConfig)
		}
		if trimmedProvider != "" {
			return Config{}, fmt.Errorf("%w: model_profile_path conflicts with provider", ErrInvalidClientConfig)
		}
		queryValues := parsedBaseURL.Query()
		if queryValues.Has(queryProvider) {
			return Config{}, fmt.Errorf("%w: model_profile_path conflicts with base_url provider query", ErrInvalidClientConfig)
		}
		if queryValues.Has(queryModel) {
			return Config{}, fmt.Errorf("%w: model_profile_path conflicts with base_url model query", ErrInvalidClientConfig)
		}
	}
	trimmedSecret := strings.TrimSpace(input.Secret)
	if trimmedSecret == "" {
		return Config{}, fmt.Errorf("%w: missing secret", ErrInvalidClientConfig)
	}
	if input.Timeout <= 0 {
		return Config{}, fmt.Errorf("%w: timeout must be positive", ErrInvalidClientConfig)
	}
	return Config{
		baseURL:            parsedBaseURL,
		secret:             trimmedSecret,
		provider:           trimmedProvider,
		modelProfilePath:   trimmedModelProfilePath,
		modelProfileReader: input.ModelProfileReader,
		timeout:            input.Timeout,
	}, nil
}

// Timeout returns the validated client timeout.
func (config Config) Timeout() time.Duration {
	return config.timeout
}

// MessagesPostURL builds the authenticated v2 JSON POST URL for this config.
func (config Config) MessagesPostURL() (string, error) {
	requestURL, requestError := config.messagesPostURL()
	if requestError != nil {
		return "", requestError
	}
	return requestURL.String(), nil
}

func (config Config) messagesPostURL() (url.URL, error) {
	provider := config.provider
	if config.modelProfilePath != "" {
		modelProfile, profileError := config.currentModelProfile()
		if profileError != nil {
			return url.URL{}, profileError
		}
		provider = modelProfile.provider
	}
	return config.messagesPostURLForProvider(provider), nil
}

func (config Config) messagesPostURLForProvider(provider string) url.URL {
	requestURL := *config.baseURL
	requestURL.Path = v2EndpointPath(requestURL.Path)
	return config.authenticatedPostURL(requestURL, provider)
}

func (config Config) authenticatedPostURL(requestURL url.URL, provider string) url.URL {
	queryValues := requestURL.Query()
	for queryKeyName := range postBodyQueryKeys {
		queryValues.Del(queryKeyName)
	}
	queryValues.Set(queryKey, config.secret)
	queryValues.Set(queryFormat, formatQueryValueTextPlain)
	if provider != "" {
		queryValues.Set(queryProvider, provider)
	}
	requestURL.RawQuery = queryValues.Encode()
	return requestURL
}

func v2EndpointPath(basePath string) string {
	trimmedPath := strings.TrimRight(strings.TrimSpace(basePath), "/")
	if trimmedPath == "" {
		return "/v2"
	}
	if trimmedPath == "/v2" || strings.HasSuffix(trimmedPath, "/v2") {
		return trimmedPath
	}
	return trimmedPath + "/v2"
}

// MessageInput is an unvalidated chat message for a v2 JSON POST request.
type MessageInput struct {
	Role    string
	Content string
	// Order is optional; when any message sets it, every message in the request must set a unique non-negative value.
	Order *int
}

type message struct {
	role    string
	content string
	order   *int
}

// MessagesRequestInput is the unvalidated external input for a v2 messages-only JSON POST request.
type MessagesRequestInput struct {
	Messages  []MessageInput
	Model     string
	WebSearch bool
	MaxTokens *int
}

// MessagesRequest is a validated v2 messages-only JSON POST request.
type MessagesRequest struct {
	messages  []message
	model     string
	webSearch bool
	maxTokens *int
}

// NewMessagesRequest validates v2 messages-only request input.
func NewMessagesRequest(input MessagesRequestInput) (MessagesRequest, error) {
	if len(input.Messages) == 0 {
		return MessagesRequest{}, fmt.Errorf("%w: missing messages", ErrInvalidClientRequest)
	}
	if input.MaxTokens != nil && *input.MaxTokens <= 0 {
		return MessagesRequest{}, fmt.Errorf("%w: max_tokens must be positive", ErrInvalidClientRequest)
	}
	messages, messageError := newMessages(input.Messages)
	if messageError != nil {
		return MessagesRequest{}, messageError
	}
	return MessagesRequest{
		messages:  messages,
		model:     strings.TrimSpace(input.Model),
		webSearch: input.WebSearch,
		maxTokens: input.MaxTokens,
	}, nil
}

func (request MessagesRequest) payloadBody(model string) []byte {
	payload := map[string]any{
		"messages":   messagePayload(request.messages),
		"web_search": request.webSearch,
	}
	if model != "" {
		payload[queryModel] = model
	}
	if request.maxTokens != nil {
		payload["max_tokens"] = *request.maxTokens
	}
	payloadBytes, _ := json.Marshal(payload)
	return payloadBytes
}

func newMessages(inputMessages []MessageInput) ([]message, error) {
	orderedInputMessages, orderError := sortInputMessagesByOrder(inputMessages)
	if orderError != nil {
		return nil, orderError
	}
	messages := make([]message, 0, len(inputMessages))
	hasUserMessage := false
	for messageIndex, inputMessage := range orderedInputMessages {
		role := strings.ToLower(strings.TrimSpace(inputMessage.Role))
		switch role {
		case messageRoleSystem, messageRoleUser, messageRoleAssistant:
		default:
			return nil, fmt.Errorf("%w: messages[%d].role unsupported", ErrInvalidClientRequest, messageIndex)
		}
		if inputMessage.Content == "" {
			return nil, fmt.Errorf("%w: messages[%d].content is empty", ErrInvalidClientRequest, messageIndex)
		}
		if role == messageRoleUser {
			hasUserMessage = true
		}
		messages = append(messages, message{role: role, content: inputMessage.Content, order: inputMessage.Order})
	}
	if len(inputMessages) > 0 && !hasUserMessage {
		return nil, fmt.Errorf("%w: messages must include a user message", ErrInvalidClientRequest)
	}
	return messages, nil
}

func sortInputMessagesByOrder(inputMessages []MessageInput) ([]MessageInput, error) {
	orderedInputMessages := append([]MessageInput(nil), inputMessages...)
	hasExplicitOrder := false
	for _, inputMessage := range orderedInputMessages {
		if inputMessage.Order != nil {
			hasExplicitOrder = true
			break
		}
	}
	if !hasExplicitOrder {
		return orderedInputMessages, nil
	}
	seenOrders := map[int]struct{}{}
	for messageIndex, inputMessage := range orderedInputMessages {
		if inputMessage.Order == nil {
			return nil, fmt.Errorf("%w: messages[%d].order missing", ErrInvalidClientRequest, messageIndex)
		}
		if *inputMessage.Order < 0 {
			return nil, fmt.Errorf("%w: messages[%d].order is negative", ErrInvalidClientRequest, messageIndex)
		}
		if _, exists := seenOrders[*inputMessage.Order]; exists {
			return nil, fmt.Errorf("%w: duplicate messages order=%d", ErrInvalidClientRequest, *inputMessage.Order)
		}
		seenOrders[*inputMessage.Order] = struct{}{}
	}
	sort.SliceStable(orderedInputMessages, func(firstIndex int, secondIndex int) bool {
		return *orderedInputMessages[firstIndex].Order < *orderedInputMessages[secondIndex].Order
	})
	return orderedInputMessages, nil
}

func messagePayload(messages []message) []map[string]any {
	payload := make([]map[string]any, 0, len(messages))
	for _, requestMessage := range messages {
		payloadMessage := map[string]any{
			"role":    requestMessage.role,
			"content": requestMessage.content,
		}
		if requestMessage.order != nil {
			payloadMessage["order"] = *requestMessage.order
		}
		payload = append(payload, payloadMessage)
	}
	return payload
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

// PostMessages sends a v2 JSON POST messages request and returns the response text.
func (client Client) PostMessages(contextValue context.Context, request MessagesRequest) (string, error) {
	requestURL, requestBody, requestError := client.messagesPostRequest(request)
	if requestError != nil {
		return "", requestError
	}
	return client.postPayload(contextValue, requestURL, requestBody)
}

func (client Client) messagesPostRequest(request MessagesRequest) (url.URL, []byte, error) {
	if client.config.modelProfilePath == "" {
		return client.config.messagesPostURLForProvider(client.config.provider), request.payloadBody(request.model), nil
	}
	if request.model != "" {
		return url.URL{}, nil, fmt.Errorf(
			"%w: request model conflicts with model_profile path=%q",
			ErrInvalidModelProfile,
			client.config.modelProfilePath,
		)
	}
	modelProfile, profileError := client.config.currentModelProfile()
	if profileError != nil {
		return url.URL{}, nil, profileError
	}
	return client.config.messagesPostURLForProvider(modelProfile.provider), request.payloadBody(modelProfile.model), nil
}

func (client Client) postPayload(contextValue context.Context, requestURL url.URL, requestBody []byte) (string, error) {
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
	responseBody, readError := io.ReadAll(httpResponse.Body)
	_ = httpResponse.Body.Close()
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
