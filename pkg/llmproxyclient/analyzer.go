package llmproxyclient

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

const (
	analyzerEndpointSuffix = "/analyze"
	contentTypeAudio       = "audio"
	contentTypeImage       = "image"
	contentTypeText        = "text"
)

const (
	imageMIMEJPEG = "image/jpeg"
	imageMIMEPNG  = "image/png"
	imageMIMEWebP = "image/webp"
	audioMIMEMP4  = "audio/mp4"
	audioMIMEMPEG = "audio/mpeg"
	audioMIMEWAV  = "audio/wav"
)

// ImageDetail is the provider-neutral image inspection fidelity.
type ImageDetail string

const (
	// ImageDetailAuto delegates image fidelity selection to the supported route.
	ImageDetailAuto ImageDetail = "auto"
	// ImageDetailLow requests low-fidelity image inspection.
	ImageDetailLow ImageDetail = "low"
	// ImageDetailHigh requests high-fidelity image inspection.
	ImageDetailHigh ImageDetail = "high"
)

// ContentPart is one validated analyzer message content part. Values are
// created only through NewTextContent, NewImageContent, or NewAudioContent.
type ContentPart interface {
	analyzerContentPart() analyzerContent
}

type analyzerContent struct {
	contentType string
	text        string
	mimeType    string
	data        []byte
	sha256      string
	detail      ImageDetail
}

func (content analyzerContent) analyzerContentPart() analyzerContent {
	return content
}

// ImageContentInput is unvalidated image content supplied to NewImageContent.
type ImageContentInput struct {
	MIMEType string
	Data     []byte
	Detail   ImageDetail
}

// AudioContentInput is unvalidated audio content supplied to NewAudioContent.
type AudioContentInput struct {
	MIMEType string
	Data     []byte
}

// NewTextContent validates one nonblank analyzer text part.
func NewTextContent(text string) (ContentPart, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("%w: analyzer text content is blank", ErrInvalidClientRequest)
	}
	return analyzerContent{contentType: contentTypeText, text: text}, nil
}

// NewImageContent validates and hash-binds exact image bytes.
func NewImageContent(input ImageContentInput) (ContentPart, error) {
	mimeType := strings.ToLower(strings.TrimSpace(input.MIMEType))
	switch mimeType {
	case imageMIMEJPEG, imageMIMEPNG, imageMIMEWebP:
	default:
		return nil, fmt.Errorf("%w: unsupported analyzer image MIME type=%q", ErrInvalidClientRequest, input.MIMEType)
	}
	if len(input.Data) == 0 {
		return nil, fmt.Errorf("%w: analyzer image content is empty", ErrInvalidClientRequest)
	}
	switch input.Detail {
	case ImageDetailAuto, ImageDetailLow, ImageDetailHigh:
	default:
		return nil, fmt.Errorf("%w: unsupported analyzer image detail=%q", ErrInvalidClientRequest, input.Detail)
	}
	return newBinaryAnalyzerContent(contentTypeImage, mimeType, input.Data, input.Detail), nil
}

// NewAudioContent validates and hash-binds exact audio bytes. The proxy rejects
// the part before upstream work unless the resolved analyzer route supports
// both audio input and strict structured output.
func NewAudioContent(input AudioContentInput) (ContentPart, error) {
	mimeType := strings.ToLower(strings.TrimSpace(input.MIMEType))
	switch mimeType {
	case audioMIMEMP4, audioMIMEMPEG, audioMIMEWAV:
	default:
		return nil, fmt.Errorf("%w: unsupported analyzer audio MIME type=%q", ErrInvalidClientRequest, input.MIMEType)
	}
	if len(input.Data) == 0 {
		return nil, fmt.Errorf("%w: analyzer audio content is empty", ErrInvalidClientRequest)
	}
	return newBinaryAnalyzerContent(contentTypeAudio, mimeType, input.Data, ""), nil
}

func newBinaryAnalyzerContent(contentType string, mimeType string, data []byte, detail ImageDetail) analyzerContent {
	copiedData := append([]byte(nil), data...)
	digest := sha256.Sum256(copiedData)
	return analyzerContent{
		contentType: contentType,
		mimeType:    mimeType,
		data:        copiedData,
		sha256:      hex.EncodeToString(digest[:]),
		detail:      detail,
	}
}

// StrictOutputSchemaInput is unvalidated JSON Schema selection.
type StrictOutputSchemaInput struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

// StrictOutputSchema is a validated strict JSON Schema selection.
type StrictOutputSchema struct {
	name        string
	description string
	schema      json.RawMessage
}

// NewStrictOutputSchema validates one named JSON Schema object. Strictness is
// mandatory and is not caller-configurable.
func NewStrictOutputSchema(input StrictOutputSchemaInput) (StrictOutputSchema, error) {
	name := strings.TrimSpace(input.Name)
	if !validOutputSchemaName(name) {
		return StrictOutputSchema{}, fmt.Errorf("%w: invalid analyzer output schema name", ErrInvalidClientRequest)
	}
	var decodedSchema map[string]json.RawMessage
	if len(input.Schema) == 0 || json.Unmarshal(input.Schema, &decodedSchema) != nil || decodedSchema == nil {
		return StrictOutputSchema{}, fmt.Errorf("%w: analyzer output schema must be one JSON object", ErrInvalidClientRequest)
	}
	return StrictOutputSchema{
		name:        name,
		description: strings.TrimSpace(input.Description),
		schema:      append(json.RawMessage(nil), input.Schema...),
	}, nil
}

func validOutputSchemaName(name string) bool {
	if len(name) == 0 || len(name) > 64 {
		return false
	}
	for _, character := range name {
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '_' ||
			character == '-' {
			continue
		}
		return false
	}
	return true
}

func (schema StrictOutputSchema) valid() bool {
	return validOutputSchemaName(schema.name) && len(schema.schema) > 0
}

// AnalyzerMessageInput is an unvalidated system or user analyzer message.
type AnalyzerMessageInput struct {
	Role    string
	Content []ContentPart
	// Order is optional; when any message sets it, every message must set a
	// unique non-negative value.
	Order *int
}

type analyzerMessage struct {
	role    string
	content []analyzerContent
	order   *int
}

// AnalyzerRequestInput is unvalidated input for POST /v2/analyze.
type AnalyzerRequestInput struct {
	Messages              []AnalyzerMessageInput
	OutputSchema          StrictOutputSchema
	Model                 string
	MaxTokens             *int
	ReasoningEffort       *string
	RequestTimeoutSeconds *int
}

// AnalyzerRequest is one validated analyzer request.
type AnalyzerRequest struct {
	messages              []analyzerMessage
	outputSchema          StrictOutputSchema
	model                 string
	maxTokens             *int
	reasoningEffort       *string
	requestTimeoutSeconds *int
}

// NewAnalyzerRequest validates a hash-bound, strict-output analyzer request.
func NewAnalyzerRequest(input AnalyzerRequestInput) (AnalyzerRequest, error) {
	if len(input.Messages) == 0 {
		return AnalyzerRequest{}, fmt.Errorf("%w: missing analyzer messages", ErrInvalidClientRequest)
	}
	if !input.OutputSchema.valid() {
		return AnalyzerRequest{}, fmt.Errorf("%w: missing analyzer output schema", ErrInvalidClientRequest)
	}
	model := strings.TrimSpace(input.Model)
	if model == "" {
		return AnalyzerRequest{}, fmt.Errorf("%w: missing analyzer model", ErrInvalidClientRequest)
	}
	if input.MaxTokens != nil && *input.MaxTokens <= 0 {
		return AnalyzerRequest{}, fmt.Errorf("%w: analyzer max_tokens must be positive", ErrInvalidClientRequest)
	}
	if input.ReasoningEffort != nil && strings.TrimSpace(*input.ReasoningEffort) == "" {
		return AnalyzerRequest{}, fmt.Errorf("%w: analyzer reasoning_effort must be nonblank", ErrInvalidClientRequest)
	}
	if input.RequestTimeoutSeconds != nil && *input.RequestTimeoutSeconds <= 0 {
		return AnalyzerRequest{}, fmt.Errorf("%w: analyzer request_timeout_seconds must be positive", ErrInvalidClientRequest)
	}
	messages, messageError := newAnalyzerMessages(input.Messages)
	if messageError != nil {
		return AnalyzerRequest{}, messageError
	}
	return AnalyzerRequest{
		messages:              messages,
		outputSchema:          input.OutputSchema,
		model:                 model,
		maxTokens:             copyOptionalInteger(input.MaxTokens),
		reasoningEffort:       copyOptionalString(input.ReasoningEffort),
		requestTimeoutSeconds: copyOptionalInteger(input.RequestTimeoutSeconds),
	}, nil
}

func newAnalyzerMessages(inputMessages []AnalyzerMessageInput) ([]analyzerMessage, error) {
	orderedMessages, orderError := sortAnalyzerMessages(inputMessages)
	if orderError != nil {
		return nil, orderError
	}
	messages := make([]analyzerMessage, 0, len(orderedMessages))
	hasUserMessage := false
	for messageIndex, inputMessage := range orderedMessages {
		role := strings.ToLower(strings.TrimSpace(inputMessage.Role))
		if role != messageRoleSystem && role != messageRoleUser {
			return nil, fmt.Errorf("%w: analyzer messages[%d].role unsupported", ErrInvalidClientRequest, messageIndex)
		}
		if len(inputMessage.Content) == 0 {
			return nil, fmt.Errorf("%w: analyzer messages[%d].content is empty", ErrInvalidClientRequest, messageIndex)
		}
		content := make([]analyzerContent, 0, len(inputMessage.Content))
		for contentIndex, inputContent := range inputMessage.Content {
			if inputContent == nil {
				return nil, fmt.Errorf("%w: analyzer messages[%d].content[%d] is invalid", ErrInvalidClientRequest, messageIndex, contentIndex)
			}
			validatedContent := inputContent.analyzerContentPart()
			if role == messageRoleSystem && validatedContent.contentType != contentTypeText {
				return nil, fmt.Errorf("%w: analyzer system messages accept text only", ErrInvalidClientRequest)
			}
			content = append(content, validatedContent)
		}
		if role == messageRoleUser {
			hasUserMessage = true
		}
		messages = append(messages, analyzerMessage{
			role:    role,
			content: content,
			order:   copyOptionalInteger(inputMessage.Order),
		})
	}
	if !hasUserMessage {
		return nil, fmt.Errorf("%w: analyzer messages must include a user message", ErrInvalidClientRequest)
	}
	return messages, nil
}

func sortAnalyzerMessages(inputMessages []AnalyzerMessageInput) ([]AnalyzerMessageInput, error) {
	orderedMessages := append([]AnalyzerMessageInput(nil), inputMessages...)
	hasExplicitOrder := false
	for _, inputMessage := range orderedMessages {
		if inputMessage.Order != nil {
			hasExplicitOrder = true
			break
		}
	}
	if !hasExplicitOrder {
		return orderedMessages, nil
	}
	seenOrders := map[int]struct{}{}
	for messageIndex, inputMessage := range orderedMessages {
		if inputMessage.Order == nil {
			return nil, fmt.Errorf("%w: analyzer messages[%d].order missing", ErrInvalidClientRequest, messageIndex)
		}
		if *inputMessage.Order < 0 {
			return nil, fmt.Errorf("%w: analyzer messages[%d].order is negative", ErrInvalidClientRequest, messageIndex)
		}
		if _, exists := seenOrders[*inputMessage.Order]; exists {
			return nil, fmt.Errorf("%w: duplicate analyzer messages order=%d", ErrInvalidClientRequest, *inputMessage.Order)
		}
		seenOrders[*inputMessage.Order] = struct{}{}
	}
	sort.SliceStable(orderedMessages, func(firstIndex int, secondIndex int) bool {
		return *orderedMessages[firstIndex].Order < *orderedMessages[secondIndex].Order
	})
	return orderedMessages, nil
}

func copyOptionalInteger(value *int) *int {
	if value == nil {
		return nil
	}
	copiedValue := *value
	return &copiedValue
}

func copyOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	copiedValue := strings.TrimSpace(*value)
	return &copiedValue
}

func (request AnalyzerRequest) payloadBody() []byte {
	payload := map[string]any{
		"messages":      analyzerMessagePayload(request.messages),
		"model":         request.model,
		"output_schema": strictOutputSchemaPayload(request.outputSchema),
	}
	if request.maxTokens != nil {
		payload["max_tokens"] = *request.maxTokens
	}
	if request.reasoningEffort != nil {
		payload["reasoning_effort"] = *request.reasoningEffort
	}
	payloadBytes, _ := json.Marshal(payload)
	return payloadBytes
}

func analyzerMessagePayload(messages []analyzerMessage) []map[string]any {
	payload := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		payloadMessage := map[string]any{
			"role":    message.role,
			"content": analyzerContentPayload(message.content),
		}
		if message.order != nil {
			payloadMessage["order"] = *message.order
		}
		payload = append(payload, payloadMessage)
	}
	return payload
}

func analyzerContentPayload(content []analyzerContent) []map[string]any {
	payload := make([]map[string]any, 0, len(content))
	for _, contentPart := range content {
		payloadPart := map[string]any{"type": contentPart.contentType}
		if contentPart.contentType == contentTypeText {
			payloadPart["text"] = contentPart.text
		} else {
			payloadPart["mime_type"] = contentPart.mimeType
			payloadPart["data"] = base64.StdEncoding.EncodeToString(contentPart.data)
			payloadPart["sha256"] = contentPart.sha256
			if contentPart.contentType == contentTypeImage {
				payloadPart["detail"] = string(contentPart.detail)
			}
		}
		payload = append(payload, payloadPart)
	}
	return payload
}

func strictOutputSchemaPayload(outputSchema StrictOutputSchema) map[string]any {
	payload := map[string]any{
		"name":   outputSchema.name,
		"schema": outputSchema.schema,
	}
	if outputSchema.description != "" {
		payload["description"] = outputSchema.description
	}
	return payload
}

// AnalyzerResponse is one completed analyzer response and its proxy identity.
type AnalyzerResponse struct {
	text      string
	requestID string
}

// Text returns the strict structured-output response text.
func (response AnalyzerResponse) Text() string {
	return response.text
}

// RequestID returns the proxy-owned request identifier.
func (response AnalyzerResponse) RequestID() string {
	return response.requestID
}

// PostAnalyzer sends a validated POST /v2/analyze request.
func (client Client) PostAnalyzer(contextValue context.Context, request AnalyzerRequest) (AnalyzerResponse, error) {
	if client.config.modelProfilePath != "" {
		return AnalyzerResponse{}, fmt.Errorf("%w: analyzer requests require explicit per-request model selection", ErrInvalidModelProfile)
	}
	requestURL := client.config.analyzerPostURLForProvider(client.config.provider)
	response, postError := client.postPayloadResponse(contextValue, requestURL, request.payloadBody(), request.requestTimeoutSeconds, acceptApplicationJSON)
	if postError != nil {
		return AnalyzerResponse{}, postError
	}
	requestID := strings.TrimSpace(response.header.Get(llmproxycontract.HeaderRequestID))
	if requestID == "" {
		return AnalyzerResponse{}, fmt.Errorf("%w: successful analyzer response missing request id", ErrClientHTTPFailure)
	}
	return AnalyzerResponse{text: string(response.body), requestID: requestID}, nil
}

func (config Config) analyzerPostURLForProvider(provider string) url.URL {
	requestURL := *config.baseURL
	requestURL.Path = analyzerEndpointPath(requestURL.Path)
	requestURL = config.authenticatedPostURL(requestURL, provider)
	queryValues := requestURL.Query()
	queryValues.Del(queryFormat)
	requestURL.RawQuery = queryValues.Encode()
	return requestURL
}

func analyzerEndpointPath(basePath string) string {
	trimmedPath := strings.TrimRight(strings.TrimSpace(basePath), "/")
	if strings.HasSuffix(trimmedPath, "/v2"+analyzerEndpointSuffix) {
		return trimmedPath
	}
	return v2EndpointPath(trimmedPath) + analyzerEndpointSuffix
}
