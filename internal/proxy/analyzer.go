package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/constants"
	"github.com/tyemirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

const (
	analyzerContentTypeAudio = "audio"
	analyzerContentTypeImage = "image"
	analyzerContentTypeText  = "text"
	analyzerImageMIMEJPEG    = "image/jpeg"
	analyzerImageMIMEPNG     = "image/png"
	analyzerImageMIMEWebP    = "image/webp"
	openAIInputImageType     = "input_image"
	openAIInputTextType      = "input_text"
	openAIJSONSchemaType     = "json_schema"
)

type analyzerRequestPayload struct {
	Messages        []analyzerMessagePayload    `json:"messages"`
	OutputSchema    analyzerOutputSchemaPayload `json:"output_schema"`
	Model           string                      `json:"model"`
	MaxTokens       *int                        `json:"max_tokens"`
	ReasoningEffort requestReasoningEffortInput `json:"reasoning_effort"`
}

type analyzerMessagePayload struct {
	Role    string                   `json:"role"`
	Content []analyzerContentPayload `json:"content"`
	Order   *int                     `json:"order,omitempty"`
}

type analyzerContentPayload struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
	Data     string `json:"data,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

type analyzerOutputSchemaPayload struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema"`
}

type analyzerContent struct {
	contentType string
	text        string
	mimeType    string
	data        []byte
	detail      string
}

type analyzerMessage struct {
	role    chatRole
	content []analyzerContent
	order   *int
}

type analyzerMessages []analyzerMessage

type analyzerOutputSchema struct {
	name        string
	description string
	schema      json.RawMessage
}

type analyzerRequestParameters struct {
	messages        analyzerMessages
	outputSchema    analyzerOutputSchema
	provider        providerDefinition
	model           textModelDefinition
	maxTokens       *int
	reasoningEffort string
}

func analyzerJSONHandler(upstreamProviders *providerRouter, providers *providerRegistry, maxPromptBytes int64, managedTenants *managedTenantStore, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestStart := time.Now()
		requestTenant := authenticatedTenantFromContext(ginContext)
		recordValidationFailure := func(model string) {
			recordManagedUsageValidationFailure(
				managedTenants,
				structuredLogger,
				ginContext,
				requestTenant,
				usageEndpointAnalyzer,
				usageTextProviderIdentifier(ginContext, requestTenant.defaults),
				usageTextModelIdentifier(ginContext, model, requestTenant.defaults),
				requestStart,
			)
		}
		if rejectClientProviderCredentialsFromQuery(ginContext) {
			recordValidationFailure(constants.EmptyString)
			return
		}
		if rejectAnalyzerQueryParameters(ginContext) {
			recordValidationFailure(constants.EmptyString)
			return
		}
		if _, suppliedModelQuery := ginContext.GetQuery(queryParameterModel); suppliedModelQuery {
			ginContext.String(http.StatusBadRequest, errorConflictingModelParameters)
			recordValidationFailure(constants.EmptyString)
			return
		}
		ginContext.Request.Body = http.MaxBytesReader(ginContext.Writer, ginContext.Request.Body, maxPromptBytes)
		bodyBytes, readBodyOK := readJSONProxyBody(ginContext)
		if !readBodyOK {
			recordValidationFailure(constants.EmptyString)
			return
		}
		if rejectClientProviderCredentialsFromJSONBody(ginContext, bodyBytes) {
			recordValidationFailure(constants.EmptyString)
			return
		}
		var payload analyzerRequestPayload
		if decodeError := decodeAnalyzerRequestPayload(bodyBytes, &payload); decodeError != nil {
			ginContext.String(http.StatusBadRequest, errorInvalidAnalyzerRequest)
			recordValidationFailure(constants.EmptyString)
			return
		}
		request, requestError := newAnalyzerRequestParameters(
			ginContext,
			payload,
			textRequestDefaultsForProvider(ginContext.Query(queryParameterProvider), requestTenant, providers),
			newModelValidator(providers.forTenant(requestTenant)),
		)
		if requestError != nil {
			ginContext.String(statusCodeForError(requestError), responseMessageForError(requestError))
			recordValidationFailure(payload.Model)
			return
		}
		submitAnalyzerRequest(ginContext, upstreamProviders, request, requestTenant, managedTenants, structuredLogger, requestStart)
	}
}

func rejectAnalyzerQueryParameters(ginContext *gin.Context) bool {
	for parameterName, parameterValues := range ginContext.Request.URL.Query() {
		switch parameterName {
		case queryParameterKey, queryParameterProvider, queryParameterModel:
			if len(parameterValues) == 1 {
				continue
			}
		}
		ginContext.String(http.StatusBadRequest, errorInvalidAnalyzerRequest)
		return true
	}
	return false
}

func decodeAnalyzerRequestPayload(bodyBytes []byte, payload *analyzerRequestPayload) error {
	decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
	decoder.DisallowUnknownFields()
	if decodeError := decoder.Decode(payload); decodeError != nil {
		return decodeError
	}
	var trailing any
	if trailingError := decoder.Decode(&trailing); !errors.Is(trailingError, io.EOF) {
		return fmt.Errorf("analyzer request must contain one JSON object")
	}
	return nil
}

func newAnalyzerRequestParameters(ginContext *gin.Context, payload analyzerRequestPayload, defaults textRequestDefaults, validator *modelValidator) (analyzerRequestParameters, error) {
	if payload.Model == constants.EmptyString || payload.Model != strings.TrimSpace(payload.Model) {
		return analyzerRequestParameters{}, fmt.Errorf("%w: missing model", ErrInvalidAnalyzerRequest)
	}
	if payload.MaxTokens != nil && *payload.MaxTokens <= 0 {
		return analyzerRequestParameters{}, fmt.Errorf("%w: max_tokens must be positive", ErrInvalidAnalyzerRequest)
	}
	outputSchema, schemaError := newAnalyzerOutputSchema(payload.OutputSchema)
	if schemaError != nil {
		return analyzerRequestParameters{}, schemaError
	}
	messages, messageError := newAnalyzerMessages(payload.Messages)
	if messageError != nil {
		return analyzerRequestParameters{}, messageError
	}
	providerDefinition, resolvedModel, resolutionError := validator.ResolveText(
		ginContext.Query(queryParameterProvider),
		payload.Model,
		defaults.provider,
		constants.EmptyString,
		false,
	)
	if resolutionError != nil {
		return analyzerRequestParameters{}, resolutionError
	}
	if providerDefinition.textTransport != textTransportOpenAIResponses {
		return analyzerRequestParameters{}, fmt.Errorf(
			"%w: provider=%s model=%s capability=multimodal_strict_output",
			ErrUnsupportedCapability,
			providerDefinition.identifier.string(),
			resolvedModel.string(),
		)
	}
	if analyzerMessagesContainAudio(messages) {
		return analyzerRequestParameters{}, fmt.Errorf(
			"%w: provider=%s model=%s capability=audio_strict_output",
			ErrUnsupportedCapability,
			providerDefinition.identifier.string(),
			resolvedModel.string(),
		)
	}
	if maxTokensError := validateTextMaxTokens(providerDefinition, resolvedModel, payload.MaxTokens); maxTokensError != nil {
		return analyzerRequestParameters{}, fmt.Errorf("%w: max_tokens exceeds model limit", ErrInvalidAnalyzerRequest)
	}
	reasoningEffort, reasoningError := requestReasoningEffortForResolvedTextRoute(
		providerDefinition,
		resolvedModel,
		defaults.reasoningEffort,
		payload.ReasoningEffort,
	)
	if reasoningError != nil {
		return analyzerRequestParameters{}, reasoningError
	}
	return analyzerRequestParameters{
		messages:        messages,
		outputSchema:    outputSchema,
		provider:        providerDefinition,
		model:           resolvedModel,
		maxTokens:       payload.MaxTokens,
		reasoningEffort: reasoningEffort,
	}, nil
}

func newAnalyzerOutputSchema(payload analyzerOutputSchemaPayload) (analyzerOutputSchema, error) {
	name := payload.Name
	if !validAnalyzerSchemaName(name) {
		return analyzerOutputSchema{}, fmt.Errorf("%w: invalid output_schema.name", ErrInvalidAnalyzerRequest)
	}
	var schema map[string]json.RawMessage
	if len(payload.Schema) == 0 || json.Unmarshal(payload.Schema, &schema) != nil || schema == nil {
		return analyzerOutputSchema{}, fmt.Errorf("%w: output_schema.schema must be one JSON object", ErrInvalidAnalyzerRequest)
	}
	return analyzerOutputSchema{
		name:        name,
		description: strings.TrimSpace(payload.Description),
		schema:      append(json.RawMessage(nil), payload.Schema...),
	}, nil
}

func validAnalyzerSchemaName(name string) bool {
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

func newAnalyzerMessages(payloadMessages []analyzerMessagePayload) (analyzerMessages, error) {
	if len(payloadMessages) == 0 {
		return nil, fmt.Errorf("%w: missing messages", ErrInvalidAnalyzerRequest)
	}
	orderedMessages, orderError := sortAnalyzerMessagePayloads(payloadMessages)
	if orderError != nil {
		return nil, orderError
	}
	messages := make(analyzerMessages, 0, len(orderedMessages))
	hasUserMessage := false
	for messageIndex, payloadMessage := range orderedMessages {
		role, roleError := newChatRole(payloadMessage.Role)
		if roleError != nil || role == chatRoleAssistant || string(role) != payloadMessage.Role {
			return nil, fmt.Errorf("%w: messages[%d].role unsupported", ErrInvalidAnalyzerRequest, messageIndex)
		}
		if len(payloadMessage.Content) == 0 {
			return nil, fmt.Errorf("%w: messages[%d].content is empty", ErrInvalidAnalyzerRequest, messageIndex)
		}
		content := make([]analyzerContent, 0, len(payloadMessage.Content))
		for contentIndex, payloadContent := range payloadMessage.Content {
			validatedContent, contentError := newAnalyzerContent(payloadContent)
			if contentError != nil {
				return nil, fmt.Errorf("%w: messages[%d].content[%d]: %v", ErrInvalidAnalyzerRequest, messageIndex, contentIndex, contentError)
			}
			if role == chatRoleSystem && validatedContent.contentType != analyzerContentTypeText {
				return nil, fmt.Errorf("%w: system messages accept text only", ErrInvalidAnalyzerRequest)
			}
			content = append(content, validatedContent)
		}
		if role == chatRoleUser {
			hasUserMessage = true
		}
		messages = append(messages, analyzerMessage{
			role:    role,
			content: content,
			order:   payloadMessage.Order,
		})
	}
	if !hasUserMessage {
		return nil, fmt.Errorf("%w: messages must include a user message", ErrInvalidAnalyzerRequest)
	}
	return messages, nil
}

func newAnalyzerContent(payload analyzerContentPayload) (analyzerContent, error) {
	contentType := payload.Type
	switch contentType {
	case analyzerContentTypeText:
		if strings.TrimSpace(payload.Text) == constants.EmptyString {
			return analyzerContent{}, errors.New("text is blank")
		}
		if payload.MIMEType != constants.EmptyString ||
			payload.Data != constants.EmptyString ||
			payload.SHA256 != constants.EmptyString ||
			payload.Detail != constants.EmptyString {
			return analyzerContent{}, errors.New("text has binary fields")
		}
		return analyzerContent{contentType: contentType, text: payload.Text}, nil
	case analyzerContentTypeImage:
		if payload.Text != constants.EmptyString {
			return analyzerContent{}, errors.New("image has text field")
		}
		mimeType := payload.MIMEType
		switch mimeType {
		case analyzerImageMIMEJPEG, analyzerImageMIMEPNG, analyzerImageMIMEWebP:
		default:
			return analyzerContent{}, fmt.Errorf("unsupported image MIME type=%q", payload.MIMEType)
		}
		detail := payload.Detail
		if detail != "auto" && detail != "low" && detail != "high" {
			return analyzerContent{}, fmt.Errorf("unsupported image detail=%q", payload.Detail)
		}
		data, digestError := decodeHashBoundAnalyzerData(payload.Data, payload.SHA256)
		if digestError != nil {
			return analyzerContent{}, digestError
		}
		return analyzerContent{contentType: contentType, mimeType: mimeType, data: data, detail: detail}, nil
	case analyzerContentTypeAudio:
		if payload.Text != constants.EmptyString || payload.Detail != constants.EmptyString {
			return analyzerContent{}, errors.New("audio has unsupported fields")
		}
		mimeType := payload.MIMEType
		if mimeType != "audio/mpeg" && mimeType != "audio/wav" {
			return analyzerContent{}, fmt.Errorf("unsupported audio MIME type=%q", payload.MIMEType)
		}
		data, digestError := decodeHashBoundAnalyzerData(payload.Data, payload.SHA256)
		if digestError != nil {
			return analyzerContent{}, digestError
		}
		return analyzerContent{contentType: contentType, mimeType: mimeType, data: data}, nil
	default:
		return analyzerContent{}, fmt.Errorf("unsupported content type=%q", payload.Type)
	}
}

func decodeHashBoundAnalyzerData(rawData string, rawDigest string) ([]byte, error) {
	if rawData == constants.EmptyString {
		return nil, errors.New("binary data is empty")
	}
	decodedData, decodeError := base64.StdEncoding.DecodeString(rawData)
	if decodeError != nil || len(decodedData) == 0 || base64.StdEncoding.EncodeToString(decodedData) != rawData {
		return nil, errors.New("binary data is not canonical base64")
	}
	digest := rawDigest
	digestBytes, digestError := hex.DecodeString(digest)
	if digestError != nil || len(digestBytes) != sha256.Size || strings.ToLower(digest) != digest {
		return nil, errors.New("sha256 is not canonical lowercase hex")
	}
	actualDigest := sha256.Sum256(decodedData)
	if !bytes.Equal(actualDigest[:], digestBytes) {
		return nil, errors.New("sha256 does not match binary data")
	}
	return decodedData, nil
}

func sortAnalyzerMessagePayloads(payloadMessages []analyzerMessagePayload) ([]analyzerMessagePayload, error) {
	orderedMessages := append([]analyzerMessagePayload(nil), payloadMessages...)
	hasExplicitOrder := false
	for _, message := range orderedMessages {
		if message.Order != nil {
			hasExplicitOrder = true
			break
		}
	}
	if !hasExplicitOrder {
		return orderedMessages, nil
	}
	seenOrders := map[int]struct{}{}
	for messageIndex, message := range orderedMessages {
		if message.Order == nil {
			return nil, fmt.Errorf("%w: messages[%d].order missing", ErrInvalidAnalyzerRequest, messageIndex)
		}
		if *message.Order < 0 {
			return nil, fmt.Errorf("%w: messages[%d].order is negative", ErrInvalidAnalyzerRequest, messageIndex)
		}
		if _, duplicate := seenOrders[*message.Order]; duplicate {
			return nil, fmt.Errorf("%w: duplicate messages order=%d", ErrInvalidAnalyzerRequest, *message.Order)
		}
		seenOrders[*message.Order] = struct{}{}
	}
	sort.SliceStable(orderedMessages, func(firstIndex int, secondIndex int) bool {
		return *orderedMessages[firstIndex].Order < *orderedMessages[secondIndex].Order
	})
	return orderedMessages, nil
}

func analyzerMessagesContainAudio(messages analyzerMessages) bool {
	for _, message := range messages {
		for _, content := range message.content {
			if content.contentType == analyzerContentTypeAudio {
				return true
			}
		}
	}
	return false
}

func (messages analyzerMessages) openAIInput() []map[string]any {
	input := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		content := make([]map[string]any, 0, len(message.content))
		for _, contentPart := range message.content {
			if contentPart.contentType == analyzerContentTypeText {
				content = append(content, map[string]any{
					keyType: openAIInputTextType,
					keyText: contentPart.text,
				})
				continue
			}
			content = append(content, map[string]any{
				keyType:     openAIInputImageType,
				"image_url": "data:" + contentPart.mimeType + ";base64," + base64.StdEncoding.EncodeToString(contentPart.data),
				"detail":    contentPart.detail,
			})
		}
		input = append(input, map[string]any{
			keyType:          responseTypeMessage,
			jsonFieldRole:    string(message.role),
			jsonFieldContent: content,
		})
	}
	return input
}

func (schema analyzerOutputSchema) openAIFormat() map[string]any {
	format := map[string]any{
		keyType:  openAIJSONSchemaType,
		"name":   schema.name,
		"schema": schema.schema,
		"strict": true,
	}
	if schema.description != constants.EmptyString {
		format["description"] = schema.description
	}
	return format
}

func (router *providerRouter) analyze(requestContext context.Context, request analyzerRequestParameters, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	return router.openAIClient.openAIAnalyzerRequest(
		requestContext,
		request.provider.credentialFor(endpointKindText),
		request.model,
		request.messages,
		request.outputSchema,
		request.maxTokens,
		request.reasoningEffort,
		structuredLogger,
	)
}

func (client *OpenAIClient) openAIAnalyzerRequest(parentContext context.Context, openAIKey string, model textModelDefinition, messages analyzerMessages, outputSchema analyzerOutputSchema, maxTokens *int, reasoningEffort string, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	payload := map[string]any{
		keyModel:      model.string(),
		keyInput:      messages.openAIInput(),
		keyBackground: true,
		keyStore:      true,
		keyText: map[string]any{
			keyFormat: outputSchema.openAIFormat(),
		},
	}
	if maxTokens != nil {
		payload[keyMaxOutputTokens] = *maxTokens
	}
	if reasoningEffort != constants.EmptyString {
		payload[keyReasoning] = map[string]any{keyEffort: reasoningEffort}
	}
	payloadBytes, _ := json.Marshal(payload)
	httpRequest, buildError := buildAuthorizedJSONRequest(parentContext, http.MethodPost, client.endpoints.GetResponsesURL(), openAIKey, bytes.NewReader(payloadBytes))
	if buildError != nil {
		structuredLogger.Errorw(logEventBuildHTTPRequest, constants.LogFieldError, buildError)
		return textGenerationResult{}, buildError
	}
	statusCode, responseBytes, _, latencyMillis, requestError := client.performResponsesRequest(httpRequest, structuredLogger, logEventOpenAIRequestError)
	if requestError != nil {
		return textGenerationResult{}, requestError
	}
	responseSnapshot, snapshotError := newOpenAIResponseSnapshot(responseBytes)
	structuredLogger.Infow(
		logEventOpenAIResponse,
		logFieldHTTPStatus, statusCode,
		logFieldAPIStatus, responseSnapshot.status,
		constants.LogFieldLatencyMilliseconds, latencyMillis,
	)
	if snapshotError != nil {
		return textGenerationResult{}, errors.New(errorOpenAIAPI)
	}
	return client.resolveOpenAIAnalyzerResponse(parentContext, openAIKey, responseSnapshot, structuredLogger)
}

func (client *OpenAIClient) resolveOpenAIAnalyzerResponse(parentContext context.Context, openAIKey string, responseSnapshot openAIResponseSnapshot, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	if responseSnapshot.status == statusCompleted {
		return completedAnalyzerGeneration(responseSnapshot)
	}
	if responseSnapshot.status == statusCancelled || responseSnapshot.status == statusFailed || responseSnapshot.status == statusIncomplete {
		return textGenerationResult{usage: responseSnapshot.usage}, fmt.Errorf("%w: analyzer response status=%s", ErrProviderAPI, responseSnapshot.status)
	}
	if !responseSnapshot.isPending() || utils.IsBlank(responseSnapshot.identifier) {
		return textGenerationResult{usage: responseSnapshot.usage}, errors.New(errorOpenAIAPI)
	}
	return client.pollOpenAIAnalyzerResponse(parentContext, openAIKey, responseSnapshot.identifier, responseSnapshot.usage, structuredLogger)
}

func completedAnalyzerGeneration(responseSnapshot openAIResponseSnapshot) (textGenerationResult, error) {
	if utils.IsBlank(responseSnapshot.text) || !json.Valid([]byte(responseSnapshot.text)) {
		return textGenerationResult{usage: responseSnapshot.usage}, fmt.Errorf("%w: analyzer response is not valid JSON", ErrProviderAPI)
	}
	return responseSnapshot.generation(), nil
}

func (client *OpenAIClient) pollOpenAIAnalyzerResponse(parentContext context.Context, openAIKey string, responseIdentifier string, latestUsage *tokenUsage, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	for {
		responseSnapshot, responseComplete, fetchError := client.fetchResponseByID(parentContext, openAIKey, responseIdentifier, structuredLogger)
		if responseSnapshot.usage != nil {
			latestUsage = responseSnapshot.usage
		}
		if fetchError != nil {
			if parentContext.Err() != nil {
				return textGenerationResult{usage: latestUsage}, parentContext.Err()
			}
			return textGenerationResult{usage: latestUsage}, fetchError
		}
		if responseComplete {
			responseSnapshot.usage = latestUsage
			if responseSnapshot.status == statusCompleted {
				return completedAnalyzerGeneration(responseSnapshot)
			}
			return textGenerationResult{usage: latestUsage}, fmt.Errorf("%w: analyzer response status=%s", ErrProviderAPI, responseSnapshot.status)
		}
		select {
		case <-time.After(responsePollInterval):
		case <-parentContext.Done():
			return textGenerationResult{usage: latestUsage}, parentContext.Err()
		}
	}
}

func submitAnalyzerRequest(ginContext *gin.Context, upstreamProviders *providerRouter, request analyzerRequestParameters, requestTenant tenant, managedTenants *managedTenantStore, structuredLogger *zap.SugaredLogger, requestStart time.Time) {
	generation, requestError := upstreamProviders.analyze(ginContext.Request.Context(), request, structuredLogger)
	if requestError != nil {
		if requestContextEnded(ginContext) {
			recordManagedUsage(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointAnalyzer, request.provider.identifier.string(), request.model.string(), ginContext.Writer.Status(), generation.usage, requestStart)
			return
		}
		markRequestOutcome(ginContext, requestFailureOutcome(requestError), managedRequestFailureOutcome(requestError))
		statusCode := statusCodeForError(requestError)
		writeProviderRequestErrorResponse(ginContext, request.provider.identifier.string(), requestError, structuredLogger)
		recordManagedUsage(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointAnalyzer, request.provider.identifier.string(), request.model.string(), statusCode, generation.usage, requestStart)
		return
	}
	if requestContextEnded(ginContext) {
		recordManagedUsage(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointAnalyzer, request.provider.identifier.string(), request.model.string(), ginContext.Writer.Status(), generation.usage, requestStart)
		return
	}
	markRequestOutcome(ginContext, requestOutcomeSuccess, managedUsageOutcomeSuccess)
	writeTokenUsageHeaders(ginContext.Writer.Header(), generation.usage)
	ginContext.Data(http.StatusOK, mimeApplicationJSON, []byte(generation.text))
	recordManagedUsage(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointAnalyzer, request.provider.identifier.string(), request.model.string(), http.StatusOK, generation.usage, requestStart)
}
