package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/temirov/llm-proxy/internal/constants"
	"go.uber.org/zap"
)

// result holds the outcome returned by a worker, including the text response
// and any error encountered during the OpenAI request.
type result struct {
	text         string
	requestError error
}

// requestTask carries all details needed to process a user request in the
// worker queue.
type requestTask struct {
	prompt           string
	systemPrompt     string
	model            string
	webSearchEnabled bool
	reply            chan result
}

// chatRequestPayload is the JSON contract for POST / LLM requests.
// Client authentication stays outside this body on the key query parameter; OpenAI credentials are loaded from server configuration.
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
	modelIdentifier  string
	webSearchEnabled bool
}

// BuildRouter constructs the HTTP router used by the proxy. configuration supplies queue sizes, worker counts, timeout values, API credentials and other settings. structuredLogger records structured log messages during routing.
func BuildRouter(configuration Configuration, structuredLogger *zap.SugaredLogger) (*gin.Engine, error) {
	if validationError := validateConfig(configuration); validationError != nil {
		return nil, validationError
	}

	configuration.ApplyTunables()
	if configuration.Endpoints == nil {
		configuration.Endpoints = NewEndpoints()
	}

	validator, validatorError := newModelValidator()
	if validatorError != nil {
		return nil, validatorError
	}

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
	for workerIndex := 0; workerIndex < configuration.WorkerCount; workerIndex++ {
		go func() {
			for pending := range taskQueue {
				text, requestError := openAIClient.openAIRequest(
					configuration.OpenAIKey,
					pending.model,
					pending.prompt,
					pending.systemPrompt,
					pending.webSearchEnabled,
					structuredLogger,
				)
				pending.reply <- result{text: text, requestError: requestError}
			}
		}()
	}

	router.Use(gin.Recovery(), secretMiddleware(configuration.ServiceSecret, structuredLogger))
	router.GET(rootPath, chatHandler(taskQueue, configuration.SystemPrompt, validator, requestTimeout, structuredLogger))
	router.POST(rootPath, chatJSONHandler(taskQueue, configuration.SystemPrompt, validator, requestTimeout, configuration.MaxPromptBytes, structuredLogger))
	router.POST(dictatePath, dictateHandler(openAIClient, configuration.OpenAIKey, configuration.DictationModel, configuration.MaxInputAudioBytes, structuredLogger))
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
func chatHandler(taskQueue chan requestTask, defaultSystemPrompt string, validator *modelValidator, requestTimeout time.Duration, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		chatRequest, ok := chatRequestFromQuery(ginContext, defaultSystemPrompt, validator, structuredLogger)
		if !ok {
			return
		}
		submitChatRequest(ginContext, taskQueue, chatRequest, requestTimeout, structuredLogger)
	}
}

// chatJSONHandler accepts large prompt bodies while preserving the same model validation, queueing, and response formatting used by GET /.
func chatJSONHandler(taskQueue chan requestTask, defaultSystemPrompt string, validator *modelValidator, requestTimeout time.Duration, maxPromptBytes int64, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
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

		chatRequest, validationMessage, ok := chatRequestFromPayload(payload, defaultSystemPrompt, validator)
		if !ok {
			ginContext.String(http.StatusBadRequest, validationMessage)
			return
		}
		submitChatRequest(ginContext, taskQueue, chatRequest, requestTimeout, structuredLogger)
	}
}

func chatRequestFromQuery(ginContext *gin.Context, defaultSystemPrompt string, validator *modelValidator, structuredLogger *zap.SugaredLogger) (chatRequestParameters, bool) {
	userPrompt := ginContext.Query(queryParameterPrompt)
	if userPrompt == constants.EmptyString {
		ginContext.String(http.StatusBadRequest, errorMissingPrompt)
		return chatRequestParameters{}, false
	}

	systemPrompt := ginContext.Query(queryParameterSystemPrompt)
	if systemPrompt == constants.EmptyString {
		systemPrompt = defaultSystemPrompt
	}

	modelIdentifier := ginContext.Query(queryParameterModel)
	if modelIdentifier == constants.EmptyString {
		modelIdentifier = DefaultModel
	}
	if verificationError := validator.Verify(modelIdentifier); verificationError != nil {
		ginContext.String(http.StatusBadRequest, verificationError.Error())
		return chatRequestParameters{}, false
	}

	webSearchQuery := strings.TrimSpace(ginContext.Query(queryParameterWebSearch))
	webSearchEnabled := false
	if webSearchQuery != constants.EmptyString {
		parsedWebSearch, parseError := strconv.ParseBool(webSearchQuery)
		if parseError != nil {
			structuredLogger.Warnw(
				logEventParseWebSearchParameterFailed,
				logFieldValue, webSearchQuery,
				constants.LogFieldError, parseError,
			)
		} else {
			webSearchEnabled = parsedWebSearch
		}
	}

	return chatRequestParameters{
		prompt:           userPrompt,
		systemPrompt:     systemPrompt,
		modelIdentifier:  modelIdentifier,
		webSearchEnabled: webSearchEnabled,
	}, true
}

func chatRequestFromPayload(payload chatRequestPayload, defaultSystemPrompt string, validator *modelValidator) (chatRequestParameters, string, bool) {
	if payload.Prompt == constants.EmptyString {
		return chatRequestParameters{}, errorMissingPrompt, false
	}

	systemPrompt := payload.SystemPrompt
	if systemPrompt == constants.EmptyString {
		systemPrompt = defaultSystemPrompt
	}

	modelIdentifier := payload.Model
	if modelIdentifier == constants.EmptyString {
		modelIdentifier = DefaultModel
	}
	if verificationError := validator.Verify(modelIdentifier); verificationError != nil {
		return chatRequestParameters{}, verificationError.Error(), false
	}

	return chatRequestParameters{
		prompt:           payload.Prompt,
		systemPrompt:     systemPrompt,
		modelIdentifier:  modelIdentifier,
		webSearchEnabled: payload.WebSearch,
	}, constants.EmptyString, true
}

func submitChatRequest(ginContext *gin.Context, taskQueue chan requestTask, chatRequest chatRequestParameters, requestTimeout time.Duration, structuredLogger *zap.SugaredLogger) {
	replyChannel := make(chan result, 1)
	requestDeadline, deadlineFound := ginContext.Request.Context().Deadline()
	enqueueDuration := requestTimeout
	if deadlineFound {
		enqueueDuration = time.Until(requestDeadline)
	}
	enqueueContext, enqueueCancel := context.WithTimeout(ginContext.Request.Context(), enqueueDuration)
	select {
	case taskQueue <- requestTask{
		prompt:           chatRequest.prompt,
		systemPrompt:     chatRequest.systemPrompt,
		model:            chatRequest.modelIdentifier,
		webSearchEnabled: chatRequest.webSearchEnabled,
		reply:            replyChannel,
	}:
		enqueueCancel()
	case <-enqueueContext.Done():
		enqueueCancel()
		ginContext.String(http.StatusServiceUnavailable, errorQueueFull)
		return
	}

	requestContext, requestCancel := context.WithTimeout(ginContext.Request.Context(), requestTimeout)
	select {
	case outcome := <-replyChannel:
		requestCancel()
		if outcome.requestError != nil {
			if errors.Is(outcome.requestError, ErrUnknownModel) {
				ginContext.String(http.StatusBadRequest, outcome.requestError.Error())
			} else if errors.Is(outcome.requestError, context.DeadlineExceeded) {
				ginContext.String(http.StatusGatewayTimeout, errorRequestTimedOut)
			} else {
				ginContext.String(http.StatusBadGateway, outcome.requestError.Error())
			}
			return
		}
		mime := preferredMime(ginContext)
		formattedBody, contentType := formatResponse(outcome.text, mime, chatRequest.prompt, structuredLogger)
		ginContext.Data(http.StatusOK, contentType, []byte(formattedBody))
	case <-requestContext.Done():
		requestCancel()
		ginContext.String(http.StatusGatewayTimeout, errorRequestTimedOut)
	}
}

func dictateHandler(openAIClient *OpenAIClient, openAIKey string, defaultModel string, maxInputAudioBytes int64, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
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

		modelIdentifier := strings.TrimSpace(ginContext.Query(queryParameterModel))
		if modelIdentifier == constants.EmptyString {
			modelIdentifier = defaultModel
		}

		transcribedText, requestError := openAIClient.transcribeAudio(openAIKey, modelIdentifier, fileName, audioFile, structuredLogger)
		if requestError != nil {
			if errors.Is(requestError, context.DeadlineExceeded) {
				ginContext.String(http.StatusGatewayTimeout, errorRequestTimedOut)
				return
			}
			ginContext.String(http.StatusBadGateway, requestError.Error())
			return
		}

		ginContext.JSON(http.StatusOK, gin.H{keyText: transcribedText})
	}
}
