package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/constants"
	"go.uber.org/zap"
)

// result holds the outcome returned by a worker, including the text response
// and any error encountered during the upstream provider request.
type result struct {
	generation   textGenerationResult
	requestError error
}

// requestTask carries all details needed to process a user request in the
// worker queue.
type requestTask struct {
	context    context.Context
	parameters chatRequestParameters
	reply      chan result
}

// chatRequestPayload is the JSON contract for POST / LLM requests.
// Client authentication stays outside this body on the key query parameter; provider credentials are loaded from server configuration.
type chatRequestPayload struct {
	Prompt       string                `json:"prompt"`
	Messages     *[]chatMessagePayload `json:"messages"`
	Model        string                `json:"model"`
	WebSearch    bool                  `json:"web_search"`
	SystemPrompt string                `json:"system_prompt"`
	MaxTokens    *int                  `json:"max_tokens"`
}

type chatV2RequestPayload struct {
	Prompt       json.RawMessage       `json:"prompt"`
	Messages     *[]chatMessagePayload `json:"messages"`
	Model        string                `json:"model"`
	WebSearch    bool                  `json:"web_search"`
	SystemPrompt json.RawMessage       `json:"system_prompt"`
	MaxTokens    *int                  `json:"max_tokens"`
}

// chatRequestParameters is the normalized request shape shared by GET and POST handlers after edge validation.
type chatRequestParameters struct {
	messages         chatMessages
	requestDisplay   string
	provider         providerDefinition
	model            textModelDefinition
	webSearchEnabled bool
	maxTokens        *int
}

type dictationRequestParameters struct {
	provider    providerDefinition
	model       modelID
	fileName    string
	audioReader io.Reader
}

// BuildRouter constructs the HTTP router used by the proxy. configuration supplies queue sizes, worker counts, timeout values, API credentials and other settings. structuredLogger records structured log messages during routing.
func BuildRouter(configuration Configuration, structuredLogger *zap.SugaredLogger) (*gin.Engine, error) {
	configuration, validationError := ensureValidatedConfiguration(configuration)
	if validationError != nil {
		return nil, validationError
	}

	if configuration.Endpoints == nil {
		configuration.Endpoints = NewEndpointsForURLs(configuration.OpenAIBaseURL, configuration.OpenAITranscriptionsURL)
	}

	providers := newProviderRegistry(configuration)
	validator := newModelValidator(providers)

	if strings.ToLower(configuration.LogLevel) == LogLevelDebug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	if normalizedLogLevel := strings.ToLower(configuration.LogLevel); normalizedLogLevel == LogLevelInfo || normalizedLogLevel == LogLevelDebug {
		router.Use(requestResponseLogger(structuredLogger))
	}

	taskQueue := make(chan requestTask, configuration.QueueSize)
	requestTimeout := time.Duration(configuration.RequestTimeoutSeconds) * time.Second
	openAIClient := NewOpenAIClient(HTTPClient, configuration.Endpoints, requestTimeout)
	chatClient := newOpenAICompatibleChatClient(HTTPClient, requestTimeout)
	geminiClient := newGeminiGenerateContentClient(HTTPClient, requestTimeout)
	anthropicClient := newAnthropicMessagesClient(HTTPClient, requestTimeout)
	upstreamProviders := newProviderRouter(openAIClient, chatClient, geminiClient, anthropicClient)
	for workerIndex := 0; workerIndex < configuration.WorkerCount; workerIndex++ {
		go func() {
			for pending := range taskQueue {
				generation, requestError := upstreamProviders.generateText(pending.context, pending.parameters, structuredLogger)
				pending.reply <- result{generation: generation, requestError: requestError}
			}
		}()
	}

	router.Use(gin.Recovery(), tenantMiddleware(configuration.tenants, structuredLogger))
	router.GET(rootPath, chatHandler(taskQueue, validator, requestTimeout, structuredLogger))
	router.POST(rootPath, chatJSONHandler(taskQueue, validator, requestTimeout, configuration.MaxPromptBytes, structuredLogger))
	router.POST(v2Path, chatV2JSONHandler(taskQueue, validator, requestTimeout, configuration.MaxPromptBytes, structuredLogger))
	router.POST(dictatePath, dictateHandler(upstreamProviders, validator, configuration.MaxInputAudioBytes, structuredLogger))
	return router, nil
}

// Serve builds the router from the supplied configuration and structuredLogger and starts the HTTP server on the configured port.
func Serve(configuration Configuration, structuredLogger *zap.SugaredLogger) error {
	router, buildError := BuildRouter(configuration, structuredLogger)
	if buildError != nil {
		return buildError
	}
	return router.Run(fmt.Sprintf(":%d", configuration.Port))
}

// chatHandler returns a handler that forwards query-string requests to the task queue.
func chatHandler(taskQueue chan requestTask, validator *modelValidator, requestTimeout time.Duration, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestTenant := authenticatedTenantFromContext(ginContext)
		chatRequest, ok := chatRequestFromQuery(ginContext, requestTenant.defaults, validator, structuredLogger)
		if !ok {
			return
		}
		submitChatRequest(ginContext, taskQueue, chatRequest, requestTimeout, structuredLogger)
	}
}

// chatJSONHandler accepts large prompt bodies while preserving the same model validation, queueing, and response formatting used by GET /.
func chatJSONHandler(taskQueue chan requestTask, validator *modelValidator, requestTimeout time.Duration, maxPromptBytes int64, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestTenant := authenticatedTenantFromContext(ginContext)
		ginContext.Request.Body = http.MaxBytesReader(ginContext.Writer, ginContext.Request.Body, maxPromptBytes)
		var payload chatRequestPayload
		if decodeError := json.NewDecoder(ginContext.Request.Body).Decode(&payload); decodeError != nil {
			var maxBytesError *http.MaxBytesError
			if errors.As(decodeError, &maxBytesError) {
				ginContext.String(http.StatusRequestEntityTooLarge, errorPromptPayloadTooLarge)
				return
			}
			ginContext.String(http.StatusBadRequest, errorInvalidJSONRequest)
			return
		}

		chatRequest, ok := chatRequestFromPayload(ginContext, payload, requestTenant.defaults, validator)
		if !ok {
			return
		}
		submitChatRequest(ginContext, taskQueue, chatRequest, requestTimeout, structuredLogger)
	}
}

func chatV2JSONHandler(taskQueue chan requestTask, validator *modelValidator, requestTimeout time.Duration, maxPromptBytes int64, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestTenant := authenticatedTenantFromContext(ginContext)
		ginContext.Request.Body = http.MaxBytesReader(ginContext.Writer, ginContext.Request.Body, maxPromptBytes)
		var payload chatV2RequestPayload
		jsonDecoder := json.NewDecoder(ginContext.Request.Body)
		jsonDecoder.DisallowUnknownFields()
		if decodeError := jsonDecoder.Decode(&payload); decodeError != nil {
			var maxBytesError *http.MaxBytesError
			if errors.As(decodeError, &maxBytesError) {
				ginContext.String(http.StatusRequestEntityTooLarge, errorPromptPayloadTooLarge)
				return
			}
			ginContext.String(http.StatusBadRequest, errorInvalidJSONRequest)
			return
		}

		chatRequest, ok := chatRequestFromV2Payload(ginContext, payload, requestTenant.defaults, validator)
		if !ok {
			return
		}
		submitChatRequest(ginContext, taskQueue, chatRequest, requestTimeout, structuredLogger)
	}
}

func chatRequestFromQuery(ginContext *gin.Context, defaults tenantDefaults, validator *modelValidator, structuredLogger *zap.SugaredLogger) (chatRequestParameters, bool) {
	userPrompt := ginContext.Query(queryParameterPrompt)
	systemPrompt := ginContext.Query(queryParameterSystemPrompt)
	systemPromptVisibleInResponse := systemPrompt != constants.EmptyString
	if systemPrompt == constants.EmptyString {
		systemPrompt = defaults.systemPrompt
	}
	messages, messageError := newPromptChatMessages(userPrompt, systemPrompt, systemPromptVisibleInResponse)
	if messageError != nil {
		ginContext.String(http.StatusBadRequest, errorMissingPrompt)
		return chatRequestParameters{}, false
	}

	webSearchQuery := strings.TrimSpace(ginContext.Query(queryParameterWebSearch))
	webSearchEnabled, webSearchParseError := parseWebSearchParameter(webSearchQuery)
	if webSearchParseError != nil {
		structuredLogger.Warnw(
			logEventParseWebSearchParameterFailed,
			logFieldValue, webSearchQuery,
			constants.LogFieldError, webSearchParseError,
		)
	}
	maxTokens, maxTokensError := parseMaxTokensParameter(ginContext.Query(queryParameterMaxTokens))
	if maxTokensError != nil {
		ginContext.String(http.StatusBadRequest, errorInvalidMaxTokens)
		return chatRequestParameters{}, false
	}

	providerDefinition, modelIdentifier, verificationError := validator.ResolveText(
		ginContext.Query(queryParameterProvider),
		ginContext.Query(queryParameterModel),
		defaults.provider,
		defaults.model,
		webSearchEnabled,
	)
	if verificationError != nil {
		ginContext.String(statusCodeForError(verificationError), responseMessageForError(verificationError))
		return chatRequestParameters{}, false
	}
	if maxTokensError := validateTextMaxTokens(providerDefinition, modelIdentifier, maxTokens); maxTokensError != nil {
		ginContext.String(http.StatusBadRequest, errorInvalidMaxTokens)
		return chatRequestParameters{}, false
	}
	return chatRequestParameters{
		messages:         messages,
		requestDisplay:   userPrompt,
		provider:         providerDefinition,
		model:            modelIdentifier,
		webSearchEnabled: webSearchEnabled,
		maxTokens:        maxTokens,
	}, true
}

func chatRequestFromPayload(ginContext *gin.Context, payload chatRequestPayload, defaults tenantDefaults, validator *modelValidator) (chatRequestParameters, bool) {
	hasPrompt := payload.Prompt != constants.EmptyString
	hasMessages := payload.Messages != nil
	if hasPrompt && hasMessages {
		ginContext.String(http.StatusBadRequest, errorConflictingPromptMessages)
		return chatRequestParameters{}, false
	}
	if !hasPrompt && !hasMessages {
		ginContext.String(http.StatusBadRequest, errorMissingPrompt)
		return chatRequestParameters{}, false
	}

	modelIdentifier, modelParameterError := resolveJSONModelParameter(ginContext.Query(queryParameterModel), payload.Model)
	if modelParameterError != nil {
		ginContext.String(statusCodeForError(modelParameterError), responseMessageForError(modelParameterError))
		return chatRequestParameters{}, false
	}
	if payload.MaxTokens != nil && *payload.MaxTokens <= 0 {
		ginContext.String(http.StatusBadRequest, errorInvalidMaxTokens)
		return chatRequestParameters{}, false
	}

	providerDefinition, resolvedModel, verificationError := validator.ResolveText(
		ginContext.Query(queryParameterProvider),
		modelIdentifier,
		defaults.provider,
		defaults.model,
		payload.WebSearch,
	)
	if verificationError != nil {
		ginContext.String(statusCodeForError(verificationError), responseMessageForError(verificationError))
		return chatRequestParameters{}, false
	}
	if maxTokensError := validateTextMaxTokens(providerDefinition, resolvedModel, payload.MaxTokens); maxTokensError != nil {
		ginContext.String(http.StatusBadRequest, errorInvalidMaxTokens)
		return chatRequestParameters{}, false
	}
	var messages chatMessages
	var messageError error
	var requestDisplay string
	if hasPrompt {
		systemPrompt := payload.SystemPrompt
		systemPromptVisibleInResponse := systemPrompt != constants.EmptyString
		if systemPrompt == constants.EmptyString {
			systemPrompt = defaults.systemPrompt
		}
		messages, messageError = newPromptChatMessages(payload.Prompt, systemPrompt, systemPromptVisibleInResponse)
		requestDisplay = payload.Prompt
	} else {
		messages, messageError = newPayloadChatMessages(*payload.Messages, defaults.systemPrompt, payload.SystemPrompt)
		requestDisplay = messages.requestDisplayText()
	}
	if messageError != nil {
		ginContext.String(statusCodeForError(messageError), responseMessageForError(messageError))
		return chatRequestParameters{}, false
	}

	return chatRequestParameters{
		messages:         messages,
		requestDisplay:   requestDisplay,
		provider:         providerDefinition,
		model:            resolvedModel,
		webSearchEnabled: payload.WebSearch,
		maxTokens:        payload.MaxTokens,
	}, true
}

func chatRequestFromV2Payload(ginContext *gin.Context, payload chatV2RequestPayload, defaults tenantDefaults, validator *modelValidator) (chatRequestParameters, bool) {
	if payload.Prompt != nil {
		ginContext.String(http.StatusBadRequest, errorUnsupportedPromptParameter)
		return chatRequestParameters{}, false
	}
	if payload.SystemPrompt != nil {
		ginContext.String(http.StatusBadRequest, errorUnsupportedSystemPrompt)
		return chatRequestParameters{}, false
	}
	if payload.Messages == nil {
		ginContext.String(http.StatusBadRequest, errorMissingMessages)
		return chatRequestParameters{}, false
	}

	modelIdentifier, modelParameterError := resolveJSONModelParameter(ginContext.Query(queryParameterModel), payload.Model)
	if modelParameterError != nil {
		ginContext.String(statusCodeForError(modelParameterError), responseMessageForError(modelParameterError))
		return chatRequestParameters{}, false
	}
	if payload.MaxTokens != nil && *payload.MaxTokens <= 0 {
		ginContext.String(http.StatusBadRequest, errorInvalidMaxTokens)
		return chatRequestParameters{}, false
	}

	providerDefinition, resolvedModel, verificationError := validator.ResolveText(
		ginContext.Query(queryParameterProvider),
		modelIdentifier,
		defaults.provider,
		defaults.model,
		payload.WebSearch,
	)
	if verificationError != nil {
		ginContext.String(statusCodeForError(verificationError), responseMessageForError(verificationError))
		return chatRequestParameters{}, false
	}
	if maxTokensError := validateTextMaxTokens(providerDefinition, resolvedModel, payload.MaxTokens); maxTokensError != nil {
		ginContext.String(http.StatusBadRequest, errorInvalidMaxTokens)
		return chatRequestParameters{}, false
	}
	messages, messageError := newPayloadChatMessages(*payload.Messages, defaults.systemPrompt, constants.EmptyString)
	if messageError != nil {
		ginContext.String(statusCodeForError(messageError), responseMessageForError(messageError))
		return chatRequestParameters{}, false
	}

	return chatRequestParameters{
		messages:         messages,
		requestDisplay:   messages.requestDisplayText(),
		provider:         providerDefinition,
		model:            resolvedModel,
		webSearchEnabled: payload.WebSearch,
		maxTokens:        payload.MaxTokens,
	}, true
}

func submitChatRequest(ginContext *gin.Context, taskQueue chan requestTask, chatRequest chatRequestParameters, requestTimeout time.Duration, structuredLogger *zap.SugaredLogger) {
	replyChannel := make(chan result, 1)
	requestContext, requestCancel := context.WithTimeout(ginContext.Request.Context(), requestTimeout)
	defer requestCancel()
	select {
	case taskQueue <- requestTask{
		context:    requestContext,
		parameters: chatRequest,
		reply:      replyChannel,
	}:
	case <-requestContext.Done():
		ginContext.String(http.StatusServiceUnavailable, errorQueueFull)
		return
	}

	select {
	case outcome := <-replyChannel:
		if outcome.requestError != nil {
			ginContext.String(statusCodeForError(outcome.requestError), responseMessageForError(outcome.requestError))
			return
		}
		mime := preferredMime(ginContext)
		writeTokenUsageHeaders(ginContext.Writer.Header(), outcome.generation.usage)
		formattedBody, contentType := formatResponse(outcome.generation.text, mime, chatRequest, outcome.generation.usage)
		ginContext.Data(http.StatusOK, contentType, []byte(formattedBody))
	case <-requestContext.Done():
		ginContext.String(http.StatusGatewayTimeout, errorRequestTimedOut)
	}
}

func dictateHandler(upstreamProviders *providerRouter, validator *modelValidator, maxInputAudioBytes int64, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestTenant := authenticatedTenantFromContext(ginContext)
		ginContext.Request.Body = http.MaxBytesReader(ginContext.Writer, ginContext.Request.Body, maxInputAudioBytes+2*1024*1024)
		if parseError := ginContext.Request.ParseMultipartForm(maxInputAudioBytes); parseError != nil {
			ginContext.String(http.StatusBadRequest, errorInvalidAudioForm)
			return
		}

		audioFile, header, fileError := ginContext.Request.FormFile(formFieldAudio)
		if fileError != nil {
			audioFile, header, fileError = ginContext.Request.FormFile(formFieldFile)
			if fileError != nil {
				ginContext.String(http.StatusBadRequest, errorMissingAudioFile)
				return
			}
		}
		defer audioFile.Close()

		fileName := "audio.webm"
		if header != nil {
			trimmedFileName := strings.TrimSpace(header.Filename)
			if trimmedFileName != constants.EmptyString {
				fileName = trimmedFileName
			}
		}

		providerDefinition, modelIdentifier, verificationError := validator.ResolveDictation(
			ginContext.Query(queryParameterProvider),
			ginContext.Query(queryParameterModel),
			requestTenant.defaults.dictationProvider,
			requestTenant.defaults.dictationModel,
		)
		if verificationError != nil {
			ginContext.String(statusCodeForError(verificationError), responseMessageForError(verificationError))
			return
		}

		dictationRequest := dictationRequestParameters{
			provider:    providerDefinition,
			model:       modelIdentifier,
			fileName:    fileName,
			audioReader: audioFile,
		}
		transcribedText, requestError := upstreamProviders.transcribeAudio(dictationRequest, structuredLogger)
		if requestError != nil {
			ginContext.String(statusCodeForError(requestError), responseMessageForError(requestError))
			return
		}

		ginContext.JSON(http.StatusOK, gin.H{keyText: transcribedText})
	}
}

func parseWebSearchParameter(rawValue string) (bool, error) {
	if rawValue == constants.EmptyString {
		return false, nil
	}
	normalizedValue := strings.ToLower(strings.TrimSpace(rawValue))
	switch normalizedValue {
	case "1", "t", "true", "y", "yes":
		return true, nil
	case "0", "f", "false", "n", "no":
		return false, nil
	default:
		return false, fmt.Errorf("invalid web_search value: %s", rawValue)
	}
}

func parseMaxTokensParameter(rawValue string) (*int, error) {
	trimmedValue := strings.TrimSpace(rawValue)
	if trimmedValue == constants.EmptyString {
		return nil, nil
	}
	maxTokens, parseError := strconv.Atoi(trimmedValue)
	if parseError != nil || maxTokens <= 0 {
		return nil, fmt.Errorf("invalid max_tokens value: %s", rawValue)
	}
	return &maxTokens, nil
}

func resolveJSONModelParameter(queryModel string, bodyModel string) (string, error) {
	trimmedQueryModel := strings.TrimSpace(queryModel)
	trimmedBodyModel := strings.TrimSpace(bodyModel)
	if trimmedQueryModel != constants.EmptyString && trimmedBodyModel != constants.EmptyString && trimmedQueryModel != trimmedBodyModel {
		return constants.EmptyString, fmt.Errorf("%w: query=%s body=%s", ErrConflictingModelParameters, trimmedQueryModel, trimmedBodyModel)
	}
	if trimmedQueryModel != constants.EmptyString {
		return trimmedQueryModel, nil
	}
	return trimmedBodyModel, nil
}

func validateTextMaxTokens(providerDefinition providerDefinition, modelIdentifier textModelDefinition, maxTokens *int) error {
	if maxTokens == nil {
		return nil
	}
	if !modelIdentifier.hasOutputTokenLimit || *maxTokens <= modelIdentifier.outputTokenLimit {
		return nil
	}
	return fmt.Errorf(
		"invalid max_tokens value: provider=%s model=%s max_tokens=%d output_token_limit=%d",
		providerDefinition.identifier.string(),
		modelIdentifier.string(),
		*maxTokens,
		modelIdentifier.outputTokenLimit,
	)
}

func statusCodeForError(requestError error) int {
	switch {
	case errors.Is(requestError, ErrUnknownProvider), errors.Is(requestError, ErrUnknownModel), errors.Is(requestError, ErrUnsupportedCapability), errors.Is(requestError, ErrUnsupportedEndpoint), errors.Is(requestError, ErrConflictingModelParameters), errors.Is(requestError, ErrInvalidChatMessages):
		return http.StatusBadRequest
	case errors.Is(requestError, ErrProviderNotConfigured):
		return http.StatusServiceUnavailable
	case errors.Is(requestError, ErrProviderRateLimited):
		return http.StatusTooManyRequests
	case errors.Is(requestError, context.DeadlineExceeded), errors.Is(requestError, context.Canceled):
		return http.StatusGatewayTimeout
	default:
		return http.StatusBadGateway
	}
}

func responseMessageForError(requestError error) string {
	if errors.Is(requestError, context.DeadlineExceeded) || errors.Is(requestError, context.Canceled) {
		return errorRequestTimedOut
	}
	return requestError.Error()
}
