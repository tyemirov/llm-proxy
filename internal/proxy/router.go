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

// result holds the outcome returned by a provider goroutine, including the text response
// and any error encountered during the upstream provider request.
type result struct {
	generation   textGenerationResult
	requestError error
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
	return buildRouter(configuration, structuredLogger, newManagedTenantStore)
}

type managedTenantStoreOpener func(ManagementConfiguration) (*managedTenantStore, error)

func buildRouter(configuration Configuration, structuredLogger *zap.SugaredLogger, openManagedTenantStore managedTenantStoreOpener) (*gin.Engine, error) {
	configuration, validationError := ensureValidatedConfiguration(configuration)
	if validationError != nil {
		return nil, validationError
	}

	if configuration.Endpoints == nil {
		configuration.Endpoints = NewEndpointsForURLs(configuration.OpenAIBaseURL, configuration.OpenAITranscriptionsURL)
	}

	providers := newProviderRegistry(configuration)

	if strings.ToLower(configuration.LogLevel) == LogLevelDebug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	if normalizedLogLevel := strings.ToLower(configuration.LogLevel); normalizedLogLevel == LogLevelInfo || normalizedLogLevel == LogLevelDebug {
		router.Use(requestResponseLogger(structuredLogger))
	}

	requestTimeout := time.Duration(configuration.RequestTimeoutSeconds) * time.Second
	upstreamHTTPClient := newLimitedHTTPDoer(HTTPClient, configuration.WorkerCount, configuration.QueueSize, configuration.upstreamRateLimits, structuredLogger, systemUpstreamRateLimitClock{})
	openAIClient := NewOpenAIClient(upstreamHTTPClient, configuration.Endpoints, requestTimeout)
	chatClient := newOpenAICompatibleChatClient(upstreamHTTPClient, requestTimeout)
	geminiClient := newGeminiGenerateContentClient(upstreamHTTPClient, requestTimeout)
	anthropicClient := newAnthropicMessagesClient(upstreamHTTPClient, requestTimeout)
	upstreamProviders := newProviderRouter(openAIClient, chatClient, geminiClient, anthropicClient)
	var managedTenants *managedTenantStore
	runtimeStaticTenants := configuration.tenants
	if configuration.Management.Enabled {
		var storeError error
		managedTenants, storeError = openManagedTenantStore(configuration.Management)
		if storeError != nil {
			return nil, storeError
		}
		if migrationError := managedTenants.migrateProviderTextSettings(providers); migrationError != nil {
			return nil, migrationError
		}
		runtimeStaticTenants = tenantRegistry{}
	}
	tenantAuthenticator := newTenantAuthenticator(runtimeStaticTenants, managedTenants)

	router.Use(gin.Recovery())
	rootProxyHandler := tenantAuthenticatedHandler(tenantAuthenticator, structuredLogger, chatHandler(upstreamProviders, providers, requestTimeout, managedTenants, structuredLogger))
	if configuration.Management.Enabled {
		managementService := newManagementService(configuration.Management, managedTenants, providers, tenantAuthenticator, structuredLogger)
		managementService.registerRoutes(router)
	}
	router.GET(rootPath, rootProxyHandler)
	router.POST(rootPath, tenantAuthenticatedHandler(tenantAuthenticator, structuredLogger, chatJSONHandler(upstreamProviders, providers, requestTimeout, configuration.MaxPromptBytes, managedTenants, structuredLogger)))
	router.POST(v2Path, tenantAuthenticatedHandler(tenantAuthenticator, structuredLogger, chatV2JSONHandler(upstreamProviders, providers, requestTimeout, configuration.MaxPromptBytes, managedTenants, structuredLogger)))
	router.POST(dictatePath, tenantAuthenticatedHandler(tenantAuthenticator, structuredLogger, dictateHandler(upstreamProviders, providers, configuration.MaxInputAudioBytes, managedTenants, structuredLogger)))
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

// chatHandler returns a handler that forwards query-string requests to upstream providers.
func chatHandler(upstreamProviders *providerRouter, providers *providerRegistry, requestTimeout time.Duration, managedTenants *managedTenantStore, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestStart := time.Now()
		requestTenant := authenticatedTenantFromContext(ginContext)
		if rejectClientProviderCredentialsFromQuery(ginContext) {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointText, usageTextProviderIdentifier(ginContext, requestTenant.defaults), usageTextModelIdentifier(ginContext, constants.EmptyString, requestTenant.defaults), requestStart)
			return
		}
		validator := newModelValidator(providers.forTenant(requestTenant))
		textDefaults := textRequestDefaultsForProvider(ginContext.Query(queryParameterProvider), requestTenant, providers)
		chatRequest, ok := chatRequestFromQuery(ginContext, textDefaults, validator, structuredLogger)
		if !ok {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointText, usageTextProviderIdentifier(ginContext, requestTenant.defaults), usageTextModelIdentifier(ginContext, constants.EmptyString, requestTenant.defaults), requestStart)
			return
		}
		submitChatRequest(ginContext, upstreamProviders, chatRequest, requestTenant, usageEndpointText, requestTimeout, managedTenants, structuredLogger)
	}
}

// chatJSONHandler accepts large prompt bodies while preserving the same model validation and response formatting used by GET /.
func chatJSONHandler(upstreamProviders *providerRouter, providers *providerRegistry, requestTimeout time.Duration, maxPromptBytes int64, managedTenants *managedTenantStore, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestStart := time.Now()
		requestTenant := authenticatedTenantFromContext(ginContext)
		if rejectClientProviderCredentialsFromQuery(ginContext) {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointText, usageTextProviderIdentifier(ginContext, requestTenant.defaults), usageTextModelIdentifier(ginContext, constants.EmptyString, requestTenant.defaults), requestStart)
			return
		}
		ginContext.Request.Body = http.MaxBytesReader(ginContext.Writer, ginContext.Request.Body, maxPromptBytes)
		bodyBytes, readBodyOK := readJSONProxyBody(ginContext)
		if !readBodyOK {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointText, usageTextProviderIdentifier(ginContext, requestTenant.defaults), usageTextModelIdentifier(ginContext, constants.EmptyString, requestTenant.defaults), requestStart)
			return
		}
		if rejectClientProviderCredentialsFromJSONBody(ginContext, bodyBytes) {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointText, usageTextProviderIdentifier(ginContext, requestTenant.defaults), usageTextModelIdentifier(ginContext, constants.EmptyString, requestTenant.defaults), requestStart)
			return
		}
		var payload chatRequestPayload
		if decodeError := json.Unmarshal(bodyBytes, &payload); decodeError != nil {
			ginContext.String(http.StatusBadRequest, errorInvalidJSONRequest)
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointText, usageTextProviderIdentifier(ginContext, requestTenant.defaults), usageTextModelIdentifier(ginContext, constants.EmptyString, requestTenant.defaults), requestStart)
			return
		}

		validator := newModelValidator(providers.forTenant(requestTenant))
		textDefaults := textRequestDefaultsForProvider(ginContext.Query(queryParameterProvider), requestTenant, providers)
		chatRequest, ok := chatRequestFromPayload(ginContext, payload, textDefaults, validator)
		if !ok {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointText, usageTextProviderIdentifier(ginContext, requestTenant.defaults), usageTextModelIdentifier(ginContext, payload.Model, requestTenant.defaults), requestStart)
			return
		}
		submitChatRequest(ginContext, upstreamProviders, chatRequest, requestTenant, usageEndpointText, requestTimeout, managedTenants, structuredLogger)
	}
}

func chatV2JSONHandler(upstreamProviders *providerRouter, providers *providerRegistry, requestTimeout time.Duration, maxPromptBytes int64, managedTenants *managedTenantStore, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestStart := time.Now()
		requestTenant := authenticatedTenantFromContext(ginContext)
		if rejectClientProviderCredentialsFromQuery(ginContext) {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointV2, usageTextProviderIdentifier(ginContext, requestTenant.defaults), usageTextModelIdentifier(ginContext, constants.EmptyString, requestTenant.defaults), requestStart)
			return
		}
		ginContext.Request.Body = http.MaxBytesReader(ginContext.Writer, ginContext.Request.Body, maxPromptBytes)
		bodyBytes, readBodyOK := readJSONProxyBody(ginContext)
		if !readBodyOK {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointV2, usageTextProviderIdentifier(ginContext, requestTenant.defaults), usageTextModelIdentifier(ginContext, constants.EmptyString, requestTenant.defaults), requestStart)
			return
		}
		if rejectClientProviderCredentialsFromJSONBody(ginContext, bodyBytes) {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointV2, usageTextProviderIdentifier(ginContext, requestTenant.defaults), usageTextModelIdentifier(ginContext, constants.EmptyString, requestTenant.defaults), requestStart)
			return
		}
		var payload chatV2RequestPayload
		jsonDecoder := json.NewDecoder(strings.NewReader(string(bodyBytes)))
		jsonDecoder.DisallowUnknownFields()
		if decodeError := jsonDecoder.Decode(&payload); decodeError != nil {
			ginContext.String(http.StatusBadRequest, errorInvalidJSONRequest)
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointV2, usageTextProviderIdentifier(ginContext, requestTenant.defaults), usageTextModelIdentifier(ginContext, constants.EmptyString, requestTenant.defaults), requestStart)
			return
		}

		validator := newModelValidator(providers.forTenant(requestTenant))
		textDefaults := textRequestDefaultsForProvider(ginContext.Query(queryParameterProvider), requestTenant, providers)
		chatRequest, ok := chatRequestFromV2Payload(ginContext, payload, textDefaults, validator)
		if !ok {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointV2, usageTextProviderIdentifier(ginContext, requestTenant.defaults), usageTextModelIdentifier(ginContext, payload.Model, requestTenant.defaults), requestStart)
			return
		}
		submitChatRequest(ginContext, upstreamProviders, chatRequest, requestTenant, usageEndpointV2, requestTimeout, managedTenants, structuredLogger)
	}
}

type textRequestDefaults struct {
	provider     string
	model        string
	systemPrompt string
}

func textRequestDefaultsForProvider(rawProvider string, requestTenant tenant, providers *providerRegistry) textRequestDefaults {
	providerExplicit := strings.TrimSpace(rawProvider) != constants.EmptyString
	defaults := textRequestDefaults{
		provider:     requestTenant.defaults.provider,
		model:        requestTenant.defaults.model,
		systemPrompt: requestTenant.defaults.systemPrompt,
	}
	if providerExplicit {
		defaults.model = constants.EmptyString
	}
	if !requestTenant.managed || !providerExplicit {
		return defaults
	}
	providerCandidate := strings.TrimSpace(rawProvider)
	providerIdentifier, providerError := providers.canonicalProviderID(providerCandidate)
	if providerError != nil {
		return defaults
	}
	settings, hasSettings := requestTenant.providerSettings[providerIdentifier]
	if !hasSettings {
		return defaults
	}
	defaults.model = settings.textModel
	defaults.systemPrompt = settings.systemPrompt
	return defaults
}

func chatRequestFromQuery(ginContext *gin.Context, defaults textRequestDefaults, validator *modelValidator, structuredLogger *zap.SugaredLogger) (chatRequestParameters, bool) {
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

func chatRequestFromPayload(ginContext *gin.Context, payload chatRequestPayload, defaults textRequestDefaults, validator *modelValidator) (chatRequestParameters, bool) {
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

func chatRequestFromV2Payload(ginContext *gin.Context, payload chatV2RequestPayload, defaults textRequestDefaults, validator *modelValidator) (chatRequestParameters, bool) {
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

func submitChatRequest(ginContext *gin.Context, upstreamProviders *providerRouter, chatRequest chatRequestParameters, requestTenant tenant, usageEndpoint string, requestTimeout time.Duration, managedTenants *managedTenantStore, structuredLogger *zap.SugaredLogger) {
	requestStart := time.Now()
	replyChannel := make(chan result, 1)
	requestContext, requestCancel := context.WithTimeout(ginContext.Request.Context(), requestTimeout)
	defer requestCancel()
	go func() {
		generation, requestError := upstreamProviders.generateText(requestContext, chatRequest, structuredLogger)
		replyChannel <- result{generation: generation, requestError: requestError}
	}()

	select {
	case outcome := <-replyChannel:
		if outcome.requestError != nil {
			statusCode := statusCodeForError(outcome.requestError)
			recordManagedUsage(managedTenants, structuredLogger, requestTenant, usageEndpoint, chatRequest.provider.identifier.string(), chatRequest.model.string(), statusCode, nil, requestStart)
			ginContext.String(statusCode, responseMessageForError(outcome.requestError))
			return
		}
		mime := preferredMime(ginContext)
		writeTokenUsageHeaders(ginContext.Writer.Header(), outcome.generation.usage)
		formattedBody, contentType := formatResponse(outcome.generation.text, mime, chatRequest, outcome.generation.usage)
		recordManagedUsage(managedTenants, structuredLogger, requestTenant, usageEndpoint, chatRequest.provider.identifier.string(), chatRequest.model.string(), http.StatusOK, outcome.generation.usage, requestStart)
		ginContext.Data(http.StatusOK, contentType, []byte(formattedBody))
	case <-requestContext.Done():
		recordManagedUsage(managedTenants, structuredLogger, requestTenant, usageEndpoint, chatRequest.provider.identifier.string(), chatRequest.model.string(), http.StatusGatewayTimeout, nil, requestStart)
		ginContext.String(http.StatusGatewayTimeout, errorRequestTimedOut)
	}
}

func dictateHandler(upstreamProviders *providerRouter, providers *providerRegistry, maxInputAudioBytes int64, managedTenants *managedTenantStore, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestStart := time.Now()
		requestTenant := authenticatedTenantFromContext(ginContext)
		if rejectClientProviderCredentialsFromQuery(ginContext) {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointDictation, usageDictationProviderIdentifier(ginContext, requestTenant.defaults), usageDictationModelIdentifier(ginContext, requestTenant.defaults), requestStart)
			return
		}
		ginContext.Request.Body = http.MaxBytesReader(ginContext.Writer, ginContext.Request.Body, maxInputAudioBytes+2*1024*1024)
		if parseError := ginContext.Request.ParseMultipartForm(maxInputAudioBytes); parseError != nil {
			ginContext.String(http.StatusBadRequest, errorInvalidAudioForm)
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointDictation, usageDictationProviderIdentifier(ginContext, requestTenant.defaults), usageDictationModelIdentifier(ginContext, requestTenant.defaults), requestStart)
			return
		}
		if rejectClientProviderCredentialsFromForm(ginContext) {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointDictation, usageDictationProviderIdentifier(ginContext, requestTenant.defaults), usageDictationModelIdentifier(ginContext, requestTenant.defaults), requestStart)
			return
		}

		audioFile, header, fileError := ginContext.Request.FormFile(formFieldAudio)
		if fileError != nil {
			audioFile, header, fileError = ginContext.Request.FormFile(formFieldFile)
			if fileError != nil {
				ginContext.String(http.StatusBadRequest, errorMissingAudioFile)
				recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointDictation, usageDictationProviderIdentifier(ginContext, requestTenant.defaults), usageDictationModelIdentifier(ginContext, requestTenant.defaults), requestStart)
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

		validator := newModelValidator(providers.forTenant(requestTenant))
		providerDefinition, modelIdentifier, verificationError := validator.ResolveDictation(
			ginContext.Query(queryParameterProvider),
			ginContext.Query(queryParameterModel),
			requestTenant.defaults.dictationProvider,
			requestTenant.defaults.dictationModel,
		)
		if verificationError != nil {
			ginContext.String(statusCodeForError(verificationError), responseMessageForError(verificationError))
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointDictation, usageDictationProviderIdentifier(ginContext, requestTenant.defaults), usageDictationModelIdentifier(ginContext, requestTenant.defaults), requestStart)
			return
		}

		dictationRequest := dictationRequestParameters{
			provider:    providerDefinition,
			model:       modelIdentifier,
			fileName:    fileName,
			audioReader: audioFile,
		}
		transcribedText, requestError := upstreamProviders.transcribeAudio(ginContext.Request.Context(), dictationRequest, structuredLogger)
		if requestError != nil {
			statusCode := statusCodeForError(requestError)
			recordManagedUsage(managedTenants, structuredLogger, requestTenant, usageEndpointDictation, providerDefinition.identifier.string(), modelIdentifier.string(), statusCode, nil, requestStart)
			ginContext.String(statusCode, responseMessageForError(requestError))
			return
		}

		recordManagedUsage(managedTenants, structuredLogger, requestTenant, usageEndpointDictation, providerDefinition.identifier.string(), modelIdentifier.string(), http.StatusOK, nil, requestStart)
		ginContext.JSON(http.StatusOK, gin.H{keyText: transcribedText})
	}
}

func recordManagedUsage(managedTenants *managedTenantStore, structuredLogger *zap.SugaredLogger, requestTenant tenant, endpoint string, providerIdentifier string, modelIdentifier string, statusCode int, usage *tokenUsage, requestStart time.Time) {
	if managedTenants == nil || !requestTenant.managed {
		return
	}
	recordError := managedTenants.recordUsage(requestTenant, managedUsageEvent{
		endpoint:            endpoint,
		providerIdentifier:  providerIdentifier,
		modelIdentifier:     modelIdentifier,
		statusCode:          statusCode,
		latencyMilliseconds: time.Since(requestStart).Milliseconds(),
		usage:               usage,
	})
	if recordError != nil {
		structuredLogger.Warnw(
			logEventUsageRecordFailed,
			logFieldTenantID, requestTenant.identifier.string(),
			logFieldEndpoint, endpoint,
			logFieldProvider, providerIdentifier,
			logFieldModel, modelIdentifier,
			logFieldStatus, statusCode,
			constants.LogFieldError, recordError,
		)
	}
}

func recordManagedUsageValidationFailure(managedTenants *managedTenantStore, structuredLogger *zap.SugaredLogger, ginContext *gin.Context, requestTenant tenant, endpoint string, providerIdentifier string, modelIdentifier string, requestStart time.Time) {
	statusCode := ginContext.Writer.Status()
	recordManagedUsage(managedTenants, structuredLogger, requestTenant, endpoint, providerIdentifier, modelIdentifier, statusCode, nil, requestStart)
}

func usageTextProviderIdentifier(ginContext *gin.Context, defaults tenantDefaults) string {
	providerIdentifier := strings.TrimSpace(ginContext.Query(queryParameterProvider))
	if providerIdentifier != constants.EmptyString {
		return providerIdentifier
	}
	return defaults.provider
}

func usageTextModelIdentifier(ginContext *gin.Context, bodyModel string, defaults tenantDefaults) string {
	modelIdentifier := strings.TrimSpace(ginContext.Query(queryParameterModel))
	if modelIdentifier != constants.EmptyString {
		return modelIdentifier
	}
	modelIdentifier = strings.TrimSpace(bodyModel)
	if modelIdentifier != constants.EmptyString {
		return modelIdentifier
	}
	return defaults.model
}

func usageDictationProviderIdentifier(ginContext *gin.Context, defaults tenantDefaults) string {
	providerIdentifier := strings.TrimSpace(ginContext.Query(queryParameterProvider))
	if providerIdentifier != constants.EmptyString {
		return providerIdentifier
	}
	return defaults.dictationProvider
}

func usageDictationModelIdentifier(ginContext *gin.Context, defaults tenantDefaults) string {
	modelIdentifier := strings.TrimSpace(ginContext.Query(queryParameterModel))
	if modelIdentifier != constants.EmptyString {
		return modelIdentifier
	}
	return defaults.dictationModel
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
	case errors.Is(requestError, ErrProviderNotConfigured), errors.Is(requestError, errQueueFull):
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
