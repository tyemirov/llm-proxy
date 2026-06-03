package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/temirov/llm-proxy/internal/constants"
	"go.uber.org/zap"
)

// result holds the outcome returned by a worker, including the text response
// and any error encountered during the upstream provider request.
type result struct {
	text         string
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
	Prompt       string `json:"prompt"`
	Model        string `json:"model"`
	WebSearch    bool   `json:"web_search"`
	SystemPrompt string `json:"system_prompt"`
}

// chatRequestParameters is the normalized request shape shared by GET and POST handlers after edge validation.
type chatRequestParameters struct {
	prompt           string
	systemPrompt     string
	provider         providerDefinition
	model            modelID
	webSearchEnabled bool
}

type dictationRequestParameters struct {
	provider    providerDefinition
	model       modelID
	fileName    string
	audioReader io.Reader
}

// BuildRouter constructs the HTTP router used by the proxy. configuration supplies queue sizes, worker counts, timeout values, API credentials and other settings. structuredLogger records structured log messages during routing.
func BuildRouter(configuration Configuration, structuredLogger *zap.SugaredLogger) (*gin.Engine, error) {
	configuration.ApplyTunables()
	if validationError := validateConfig(configuration); validationError != nil {
		return nil, validationError
	}

	if configuration.Endpoints == nil {
		configuration.Endpoints = NewEndpoints()
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
	pollTimeout := time.Duration(configuration.UpstreamPollTimeoutSeconds) * time.Second
	openAIClient := NewOpenAIClient(HTTPClient, configuration.Endpoints, requestTimeout, configuration.MaxOutputTokens, pollTimeout)
	chatClient := newOpenAICompatibleChatClient(HTTPClient, requestTimeout, configuration.MaxOutputTokens)
	upstreamProviders := newProviderRouter(openAIClient, chatClient)
	for workerIndex := 0; workerIndex < configuration.WorkerCount; workerIndex++ {
		go func() {
			for pending := range taskQueue {
				text, requestError := upstreamProviders.generateText(pending.context, pending.parameters, structuredLogger)
				pending.reply <- result{text: text, requestError: requestError}
			}
		}()
	}

	router.Use(gin.Recovery(), secretMiddleware(configuration.ServiceSecret, structuredLogger))
	router.GET(rootPath, chatHandler(taskQueue, configuration.SystemPrompt, configuration.DefaultProvider, configuration.DefaultModel, validator, requestTimeout, structuredLogger))
	router.POST(rootPath, chatJSONHandler(taskQueue, configuration.SystemPrompt, configuration.DefaultProvider, configuration.DefaultModel, validator, requestTimeout, configuration.MaxPromptBytes, structuredLogger))
	router.POST(dictatePath, dictateHandler(upstreamProviders, configuration.DefaultDictationProvider, configuration.DictationModel, validator, configuration.MaxInputAudioBytes, structuredLogger))
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
func chatHandler(taskQueue chan requestTask, defaultSystemPrompt string, defaultProvider string, defaultModel string, validator *modelValidator, requestTimeout time.Duration, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		chatRequest, ok := chatRequestFromQuery(ginContext, defaultSystemPrompt, defaultProvider, defaultModel, validator, structuredLogger)
		if !ok {
			return
		}
		submitChatRequest(ginContext, taskQueue, chatRequest, requestTimeout, structuredLogger)
	}
}

// chatJSONHandler accepts large prompt bodies while preserving the same model validation, queueing, and response formatting used by GET /.
func chatJSONHandler(taskQueue chan requestTask, defaultSystemPrompt string, defaultProvider string, defaultModel string, validator *modelValidator, requestTimeout time.Duration, maxPromptBytes int64, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
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

		chatRequest, ok := chatRequestFromPayload(ginContext, payload, defaultSystemPrompt, defaultProvider, defaultModel, validator)
		if !ok {
			return
		}
		submitChatRequest(ginContext, taskQueue, chatRequest, requestTimeout, structuredLogger)
	}
}

func chatRequestFromQuery(ginContext *gin.Context, defaultSystemPrompt string, defaultProvider string, defaultModel string, validator *modelValidator, structuredLogger *zap.SugaredLogger) (chatRequestParameters, bool) {
	userPrompt := ginContext.Query(queryParameterPrompt)
	if userPrompt == constants.EmptyString {
		ginContext.String(http.StatusBadRequest, errorMissingPrompt)
		return chatRequestParameters{}, false
	}

	systemPrompt := ginContext.Query(queryParameterSystemPrompt)
	if systemPrompt == constants.EmptyString {
		systemPrompt = defaultSystemPrompt
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

	providerDefinition, modelIdentifier, verificationError := validator.ResolveText(
		ginContext.Query(queryParameterProvider),
		ginContext.Query(queryParameterModel),
		defaultProvider,
		defaultModel,
		webSearchEnabled,
	)
	if verificationError != nil {
		ginContext.String(statusCodeForError(verificationError), responseMessageForError(verificationError))
		return chatRequestParameters{}, false
	}

	return chatRequestParameters{
		prompt:           userPrompt,
		systemPrompt:     systemPrompt,
		provider:         providerDefinition,
		model:            modelIdentifier,
		webSearchEnabled: webSearchEnabled,
	}, true
}

func chatRequestFromPayload(ginContext *gin.Context, payload chatRequestPayload, defaultSystemPrompt string, defaultProvider string, defaultModel string, validator *modelValidator) (chatRequestParameters, bool) {
	if payload.Prompt == constants.EmptyString {
		ginContext.String(http.StatusBadRequest, errorMissingPrompt)
		return chatRequestParameters{}, false
	}

	systemPrompt := payload.SystemPrompt
	if systemPrompt == constants.EmptyString {
		systemPrompt = defaultSystemPrompt
	}

	modelIdentifier, modelParameterError := resolveJSONModelParameter(ginContext.Query(queryParameterModel), payload.Model)
	if modelParameterError != nil {
		ginContext.String(statusCodeForError(modelParameterError), responseMessageForError(modelParameterError))
		return chatRequestParameters{}, false
	}

	providerDefinition, resolvedModel, verificationError := validator.ResolveText(
		ginContext.Query(queryParameterProvider),
		modelIdentifier,
		defaultProvider,
		defaultModel,
		payload.WebSearch,
	)
	if verificationError != nil {
		ginContext.String(statusCodeForError(verificationError), responseMessageForError(verificationError))
		return chatRequestParameters{}, false
	}

	return chatRequestParameters{
		prompt:           payload.Prompt,
		systemPrompt:     systemPrompt,
		provider:         providerDefinition,
		model:            resolvedModel,
		webSearchEnabled: payload.WebSearch,
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
		formattedBody, contentType := formatResponse(outcome.text, mime, chatRequest.prompt)
		ginContext.Data(http.StatusOK, contentType, []byte(formattedBody))
	case <-requestContext.Done():
		ginContext.String(http.StatusGatewayTimeout, errorRequestTimedOut)
	}
}

func dictateHandler(upstreamProviders *providerRouter, defaultProvider string, defaultModel string, validator *modelValidator, maxInputAudioBytes int64, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
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
			defaultProvider,
			defaultModel,
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

func statusCodeForError(requestError error) int {
	switch {
	case errors.Is(requestError, ErrUnknownProvider), errors.Is(requestError, ErrUnknownModel), errors.Is(requestError, ErrUnsupportedCapability), errors.Is(requestError, ErrUnsupportedEndpoint), errors.Is(requestError, ErrConflictingModelParameters):
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
