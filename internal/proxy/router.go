package proxy

import (
	"bytes"
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
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
)

// chatRequestPayload is the JSON contract for POST / LLM requests.
// Client authentication stays outside this body on the key query parameter; provider credentials are loaded from tenant-managed settings.
type chatRequestPayload struct {
	Prompt          string                      `json:"prompt"`
	Messages        *[]chatMessagePayload       `json:"messages"`
	Model           string                      `json:"model"`
	WebSearch       bool                        `json:"web_search"`
	SystemPrompt    string                      `json:"system_prompt"`
	MaxTokens       *int                        `json:"max_tokens"`
	ReasoningEffort requestReasoningEffortInput `json:"reasoning_effort"`
}

type chatV2RequestPayload struct {
	Prompt           json.RawMessage             `json:"prompt"`
	Messages         *[]chatV2MessagePayload     `json:"messages"`
	Model            string                      `json:"model"`
	WebSearch        bool                        `json:"web_search"`
	SystemPrompt     json.RawMessage             `json:"system_prompt"`
	MaxTokens        *int                        `json:"max_tokens"`
	ReasoningEffort  requestReasoningEffortInput `json:"reasoning_effort"`
	StructuredOutput json.RawMessage             `json:"structured_output"`
}

type requestReasoningEffortInput struct {
	value    *string
	supplied bool
}

func (input *requestReasoningEffortInput) UnmarshalJSON(rawInput []byte) error {
	var value *string
	if unmarshalError := json.Unmarshal(rawInput, &value); unmarshalError != nil {
		return unmarshalError
	}
	input.value = value
	input.supplied = true
	return nil
}

// chatRequestParameters is the normalized request shape shared by GET and POST handlers after edge validation.
type chatRequestParameters struct {
	messages                   chatMessages
	requestDisplay             string
	provider                   providerDefinition
	model                      textModelDefinition
	webSearchEnabled           bool
	maxTokens                  *int
	reasoningEffort            string
	chatCompletionContinuation *chatCompletionContinuation
	structuredOutput           *structuredOutputSchema
	idempotencyKey             string
}

type dictationRequestParameters struct {
	provider    providerDefinition
	model       modelID
	fileName    string
	audioReader io.Reader
}

// BuildRouter constructs the HTTP router used by the proxy. configuration supplies management, routing, queue, worker, and timeout settings. structuredLogger records structured log messages during routing.
func BuildRouter(configuration Configuration, structuredLogger *zap.SugaredLogger) (*gin.Engine, error) {
	return buildRouter(configuration, structuredLogger, newManagedTenantStore)
}

type managedTenantStoreOpener func(ManagementConfiguration, *providerRegistry) (*managedTenantStore, error)

func buildRouter(configuration Configuration, structuredLogger *zap.SugaredLogger, openManagedTenantStore managedTenantStoreOpener) (*gin.Engine, error) {
	configuration, validationError := ensureValidatedConfiguration(configuration)
	if validationError != nil {
		return nil, validationError
	}
	upstreamHTTPClient := newLimitedHTTPDoer(HTTPClient, configuration.WorkerCount, configuration.QueueSize, configuration.upstreamRateLimits, structuredLogger, systemUpstreamRateLimitClock{})
	if structuredLogger == nil {
		structuredLogger = zap.NewNop().Sugar()
	}

	providers := newProviderRegistry(configuration)
	capabilityCatalog := newPublicCapabilityCatalog(configuration)

	if strings.ToLower(configuration.LogLevel) == LogLevelDebug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(requestIdentifierHandler())
	if normalizedLogLevel := strings.ToLower(configuration.LogLevel); normalizedLogLevel == LogLevelInfo || normalizedLogLevel == LogLevelDebug {
		router.Use(requestResponseLogger(structuredLogger))
	}

	openAIClient := NewOpenAIClient(upstreamHTTPClient, configuration.Endpoints)
	chatClient := newOpenAICompatibleChatClient(upstreamHTTPClient)
	geminiClient := newGeminiInteractionsClient(upstreamHTTPClient)
	anthropicClient := newAnthropicMessagesClient(upstreamHTTPClient)
	upstreamProviders := newProviderRouter(openAIClient, chatClient, geminiClient, anthropicClient)
	keyVerifier := newOperationalProviderKeyVerifier(upstreamHTTPClient, configuration.Endpoints, time.Duration(configuration.RequestTimeoutSeconds)*time.Second, structuredLogger)
	managedTenants, storeError := openManagedTenantStore(configuration.Management, providers)
	if storeError != nil {
		return nil, storeError
	}
	tenantAuthenticator := newTenantAuthenticator(managedTenants)
	assetStore := newTenantAssetStore(configuration.AssetStorePath, configuration.MaxAssetBytes, configuration.AssetRetentionSeconds)
	structuredRequests, structuredStoreError := newStructuredRequestStore(configuration.AssetStorePath, configuration.AssetRetentionSeconds)
	if structuredStoreError != nil {
		return nil, structuredStoreError
	}

	router.Use(gin.Recovery())
	registerPublicCapabilityRoutes(router, capabilityCatalog)
	rootProxyHandler := tenantAuthenticatedHandler(
		tenantAuthenticator,
		structuredLogger,
		requestTimeoutHandler(configuration.requestTimeoutPolicy, structuredLogger, chatHandler(upstreamProviders, providers, managedTenants, structuredLogger)),
	)
	managementService := newManagementService(configuration.Management, configuration.managementSessionValidator, managedTenants, providers, keyVerifier, structuredLogger)
	managementService.registerRoutes(router)
	router.GET(rootPath, rootProxyHandler)
	router.POST(rootPath, tenantAuthenticatedHandler(tenantAuthenticator, structuredLogger, requestTimeoutHandler(configuration.requestTimeoutPolicy, structuredLogger, chatJSONHandler(upstreamProviders, providers, configuration.MaxPromptBytes, managedTenants, structuredLogger))))
	router.POST(v2Path, tenantAuthenticatedHandler(tenantAuthenticator, structuredLogger, requestTimeoutHandler(configuration.requestTimeoutPolicy, structuredLogger, chatV2JSONHandler(upstreamProviders, providers, maximumV2RequestBytes(configuration.MaxPromptBytes, configuration.ModelCatalog), assetStore, structuredRequests, managedTenants, structuredLogger))))
	router.GET(llmproxycontract.TenantIdentityPath, tenantAuthenticatedHandler(tenantAuthenticator, structuredLogger, tenantIdentityHandler()))
	router.GET(llmproxycontract.StructuredRequestPath, tenantAuthenticatedHandler(tenantAuthenticator, structuredLogger, structuredRequestStatusHandler(structuredRequests)))
	router.POST(llmproxycontract.AssetPath, tenantAuthenticatedHandler(tenantAuthenticator, structuredLogger, requestTimeoutHandler(configuration.requestTimeoutPolicy, structuredLogger, tenantAssetUploadHandler(assetStore))))
	router.DELETE(llmproxycontract.AssetPath+"/:asset_id", tenantAuthenticatedHandler(tenantAuthenticator, structuredLogger, tenantAssetDeleteHandler(assetStore)))
	router.POST(dictatePath, tenantAuthenticatedHandler(tenantAuthenticator, structuredLogger, requestTimeoutHandler(configuration.requestTimeoutPolicy, structuredLogger, dictateHandler(upstreamProviders, providers, configuration.MaxInputAudioBytes, managedTenants, structuredLogger))))
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
func chatHandler(upstreamProviders *providerRouter, providers *providerRegistry, managedTenants *managedTenantStore, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestStart := time.Now()
		requestTenant := authenticatedTenantFromContext(ginContext)
		textDefaults := textRequestDefaultsForProvider(ginContext.Query(queryParameterProvider), requestTenant, providers)
		if rejectClientProviderCredentialsFromQuery(ginContext) {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointText, usageTextProviderIdentifier(ginContext, textDefaults), usageTextModelIdentifier(ginContext, constants.EmptyString, textDefaults), requestStart)
			return
		}
		validator := newModelValidator(providers.forTenant(requestTenant))
		chatRequest, ok := chatRequestFromQuery(ginContext, textDefaults, validator, structuredLogger)
		if !ok {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointText, usageTextProviderIdentifier(ginContext, textDefaults), usageTextModelIdentifier(ginContext, constants.EmptyString, textDefaults), requestStart)
			return
		}
		submitChatRequest(ginContext, upstreamProviders, chatRequest, requestTenant, usageEndpointText, managedTenants, structuredLogger)
	}
}

// chatJSONHandler accepts large prompt bodies while preserving the same model validation and response formatting used by GET /.
func chatJSONHandler(upstreamProviders *providerRouter, providers *providerRegistry, maxPromptBytes int64, managedTenants *managedTenantStore, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestStart := time.Now()
		requestTenant := authenticatedTenantFromContext(ginContext)
		textDefaults := textRequestDefaultsForProvider(ginContext.Query(queryParameterProvider), requestTenant, providers)
		if rejectClientProviderCredentialsFromQuery(ginContext) {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointText, usageTextProviderIdentifier(ginContext, textDefaults), usageTextModelIdentifier(ginContext, constants.EmptyString, textDefaults), requestStart)
			return
		}
		ginContext.Request.Body = http.MaxBytesReader(ginContext.Writer, ginContext.Request.Body, maxPromptBytes)
		bodyBytes, readBodyOK := readJSONProxyBody(ginContext)
		if !readBodyOK {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointText, usageTextProviderIdentifier(ginContext, textDefaults), usageTextModelIdentifier(ginContext, constants.EmptyString, textDefaults), requestStart)
			return
		}
		if rejectClientProviderCredentialsFromJSONBody(ginContext, bodyBytes) {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointText, usageTextProviderIdentifier(ginContext, textDefaults), usageTextModelIdentifier(ginContext, constants.EmptyString, textDefaults), requestStart)
			return
		}
		var payload chatRequestPayload
		jsonDecoder := json.NewDecoder(bytes.NewReader(bodyBytes))
		jsonDecoder.DisallowUnknownFields()
		if decodeError := jsonDecoder.Decode(&payload); decodeError != nil {
			ginContext.String(http.StatusBadRequest, errorInvalidJSONRequest)
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointText, usageTextProviderIdentifier(ginContext, textDefaults), usageTextModelIdentifier(ginContext, constants.EmptyString, textDefaults), requestStart)
			return
		}

		validator := newModelValidator(providers.forTenant(requestTenant))
		chatRequest, ok := chatRequestFromPayload(ginContext, payload, textDefaults, validator)
		if !ok {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointText, usageTextProviderIdentifier(ginContext, textDefaults), usageTextModelIdentifier(ginContext, payload.Model, textDefaults), requestStart)
			return
		}
		submitChatRequest(ginContext, upstreamProviders, chatRequest, requestTenant, usageEndpointText, managedTenants, structuredLogger)
	}
}

func chatV2JSONHandler(upstreamProviders *providerRouter, providers *providerRegistry, maxRequestBytes int64, assetStore *tenantAssetStore, structuredRequests *structuredRequestStore, managedTenants *managedTenantStore, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestStart := time.Now()
		requestTenant := authenticatedTenantFromContext(ginContext)
		textDefaults := textRequestDefaultsForProvider(ginContext.Query(queryParameterProvider), requestTenant, providers)
		if rejectClientProviderCredentialsFromQuery(ginContext) {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointV2, usageTextProviderIdentifier(ginContext, textDefaults), usageTextModelIdentifier(ginContext, constants.EmptyString, textDefaults), requestStart)
			return
		}
		ginContext.Request.Body = http.MaxBytesReader(ginContext.Writer, ginContext.Request.Body, maxRequestBytes)
		bodyBytes, readBodyOK := readJSONProxyBody(ginContext)
		if !readBodyOK {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointV2, usageTextProviderIdentifier(ginContext, textDefaults), usageTextModelIdentifier(ginContext, constants.EmptyString, textDefaults), requestStart)
			return
		}
		if rejectClientProviderCredentialsFromJSONBody(ginContext, bodyBytes) {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointV2, usageTextProviderIdentifier(ginContext, textDefaults), usageTextModelIdentifier(ginContext, constants.EmptyString, textDefaults), requestStart)
			return
		}
		var payload chatV2RequestPayload
		jsonDecoder := json.NewDecoder(bytes.NewReader(bodyBytes))
		jsonDecoder.DisallowUnknownFields()
		if decodeError := jsonDecoder.Decode(&payload); decodeError != nil {
			ginContext.String(http.StatusBadRequest, errorInvalidJSONRequest)
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointV2, usageTextProviderIdentifier(ginContext, textDefaults), usageTextModelIdentifier(ginContext, constants.EmptyString, textDefaults), requestStart)
			return
		}

		validator := newModelValidator(providers.forTenant(requestTenant))
		chatRequest, ok := chatRequestFromV2Payload(ginContext, payload, textDefaults, validator, requestTenant, assetStore)
		if !ok {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointV2, usageTextProviderIdentifier(ginContext, textDefaults), usageTextModelIdentifier(ginContext, payload.Model, textDefaults), requestStart)
			return
		}
		defer chatRequest.messages.closeMedia()
		if chatRequest.structuredOutput == nil {
			submitChatRequest(ginContext, upstreamProviders, chatRequest, requestTenant, usageEndpointV2, managedTenants, structuredLogger)
			return
		}
		submitStructuredChatRequest(ginContext, upstreamProviders, chatRequest, requestTenant, bodyBytes, structuredRequests, managedTenants, structuredLogger, requestStart)
	}
}

type textRequestDefaults struct {
	provider        string
	model           string
	systemPrompt    string
	reasoningEffort string
}

func textRequestDefaultsForProvider(rawProvider string, requestTenant tenant, providers *providerRegistry) textRequestDefaults {
	providerExplicit := strings.TrimSpace(rawProvider) != constants.EmptyString
	defaults := textRequestDefaults{
		provider:        requestTenant.defaults.provider,
		model:           requestTenant.defaults.model,
		systemPrompt:    requestTenant.defaults.systemPrompt,
		reasoningEffort: requestTenant.defaults.reasoningEffort,
	}
	if providerExplicit {
		defaults.model = constants.EmptyString
	}
	if !providerExplicit {
		return defaults
	}
	providerCandidate := strings.TrimSpace(rawProvider)
	providerDefinition, catalogDefaultModel, resolutionError := providers.resolveTextModel(
		providerCandidate,
		constants.EmptyString,
		constants.EmptyString,
		constants.EmptyString,
		false,
	)
	if resolutionError != nil {
		return defaults
	}
	defaults.model = catalogDefaultModel.string()
	settings, hasSettings := requestTenant.providerSettings[providerDefinition.identifier]
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

	webSearchEnabled := false
	webSearchQuery, hasWebSearchQuery := ginContext.GetQuery(queryParameterWebSearch)
	if hasWebSearchQuery {
		var webSearchParseError error
		webSearchEnabled, webSearchParseError = parseWebSearchParameter(webSearchQuery)
		if webSearchParseError != nil {
			structuredLogger.Warnw(logEventParseWebSearchParameterFailed)
			ginContext.String(http.StatusBadRequest, errorInvalidWebSearch)
			return chatRequestParameters{}, false
		}
	}
	maxTokens, maxTokensError := parseMaxTokensParameter(ginContext.Query(queryParameterMaxTokens))
	if maxTokensError != nil {
		ginContext.String(http.StatusBadRequest, errorInvalidMaxTokens)
		return chatRequestParameters{}, false
	}
	requestReasoningEffort := requestReasoningEffortFromQuery(ginContext)

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
	reasoningEffort, reasoningEffortError := requestReasoningEffortForResolvedTextRoute(providerDefinition, modelIdentifier, defaults.reasoningEffort, requestReasoningEffort)
	if reasoningEffortError != nil {
		ginContext.String(http.StatusBadRequest, errorInvalidReasoningEffort)
		return chatRequestParameters{}, false
	}
	return chatRequestParameters{
		messages:         messages,
		requestDisplay:   userPrompt,
		provider:         providerDefinition,
		model:            modelIdentifier,
		webSearchEnabled: webSearchEnabled,
		maxTokens:        maxTokens,
		reasoningEffort:  reasoningEffort,
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
	reasoningEffort, reasoningEffortError := requestReasoningEffortForResolvedTextRoute(providerDefinition, resolvedModel, defaults.reasoningEffort, payload.ReasoningEffort)
	if reasoningEffortError != nil {
		ginContext.String(http.StatusBadRequest, errorInvalidReasoningEffort)
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
	if routeMessageError := validateMessagesForResolvedTextRoute(resolvedModel, messages); routeMessageError != nil {
		ginContext.String(statusCodeForError(routeMessageError), responseMessageForError(routeMessageError))
		return chatRequestParameters{}, false
	}

	return chatRequestParameters{
		messages:         messages,
		requestDisplay:   requestDisplay,
		provider:         providerDefinition,
		model:            resolvedModel,
		webSearchEnabled: payload.WebSearch,
		maxTokens:        payload.MaxTokens,
		reasoningEffort:  reasoningEffort,
	}, true
}

func chatRequestFromV2Payload(ginContext *gin.Context, payload chatV2RequestPayload, defaults textRequestDefaults, validator *modelValidator, requestTenant tenant, assetStore *tenantAssetStore) (chatRequestParameters, bool) {
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
	reasoningEffort, reasoningEffortError := requestReasoningEffortForResolvedTextRoute(providerDefinition, resolvedModel, defaults.reasoningEffort, payload.ReasoningEffort)
	if reasoningEffortError != nil {
		ginContext.String(http.StatusBadRequest, errorInvalidReasoningEffort)
		return chatRequestParameters{}, false
	}
	structuredOutput, structuredOutputError := newStructuredOutputSchema(payload.StructuredOutput)
	if structuredOutputError != nil {
		ginContext.String(http.StatusBadRequest, "invalid structured_output")
		return chatRequestParameters{}, false
	}
	idempotencyKey, idempotencyError := structuredRequestKey(ginContext, structuredOutput)
	if idempotencyError != nil {
		ginContext.String(http.StatusBadRequest, "invalid Idempotency-Key")
		return chatRequestParameters{}, false
	}
	if structuredOutput != nil && payload.WebSearch {
		ginContext.String(http.StatusBadRequest, "structured_output does not support web_search")
		return chatRequestParameters{}, false
	}
	if routeError := validateStructuredOutputRoute(resolvedModel, structuredOutput); routeError != nil {
		ginContext.String(http.StatusBadRequest, "structured_output is unsupported for the selected route")
		return chatRequestParameters{}, false
	}
	messages, messageError := newV2PayloadChatMessages(*payload.Messages, defaults.systemPrompt, requestTenant, assetStore)
	if messageError != nil {
		if isTenantAssetError(messageError) {
			writeTenantAssetError(ginContext, messageError)
		} else {
			ginContext.String(statusCodeForError(messageError), responseMessageForError(messageError))
		}
		return chatRequestParameters{}, false
	}
	if routeMessageError := validateMessagesForResolvedTextRoute(resolvedModel, messages); routeMessageError != nil {
		messages.closeMedia()
		ginContext.String(statusCodeForError(routeMessageError), responseMessageForError(routeMessageError))
		return chatRequestParameters{}, false
	}
	if mediaCapabilityError := validateMessageMediaForResolvedTextRoute(providerDefinition, resolvedModel, messages); mediaCapabilityError != nil {
		messages.closeMedia()
		ginContext.String(statusCodeForError(mediaCapabilityError), responseMessageForError(mediaCapabilityError))
		return chatRequestParameters{}, false
	}

	return chatRequestParameters{
		messages:         messages,
		requestDisplay:   messages.requestDisplayText(),
		provider:         providerDefinition,
		model:            resolvedModel,
		webSearchEnabled: payload.WebSearch,
		maxTokens:        payload.MaxTokens,
		reasoningEffort:  reasoningEffort,
		structuredOutput: structuredOutput,
		idempotencyKey:   idempotencyKey,
	}, true
}

func structuredRequestKey(ginContext *gin.Context, structuredOutput *structuredOutputSchema) (string, error) {
	values := ginContext.Request.Header.Values(llmproxycontract.HeaderIdempotencyKey)
	if structuredOutput == nil {
		if len(values) == 0 {
			return "", nil
		}
		return "", errStructuredOutputInvalid
	}
	if len(values) != 1 || !validIdempotencyKey(values[0]) {
		return "", errStructuredOutputInvalid
	}
	return values[0], nil
}

func reasoningEffortForResolvedTextRoute(model textModelDefinition, rawEffort string) string {
	if rawEffort == constants.EmptyString || !model.reasoningEffort.supports(rawEffort) {
		return constants.EmptyString
	}
	return rawEffort
}

func requestReasoningEffortFromQuery(ginContext *gin.Context) requestReasoningEffortInput {
	value, supplied := ginContext.GetQuery(queryParameterReasoningEffort)
	if !supplied {
		return requestReasoningEffortInput{}
	}
	return requestReasoningEffortInput{value: &value, supplied: true}
}

func requestReasoningEffortForResolvedTextRoute(provider providerDefinition, model textModelDefinition, defaultEffort string, requestedEffort requestReasoningEffortInput) (string, error) {
	if !requestedEffort.supplied {
		return reasoningEffortForResolvedTextRoute(model, defaultEffort), nil
	}
	if requestedEffort.value == nil || *requestedEffort.value == constants.EmptyString {
		return constants.EmptyString, unsupportedReasoningEffortError(provider, model, constants.EmptyString)
	}
	if validationError := validateReasoningEffortForResolvedTextRoute(provider, model, *requestedEffort.value); validationError != nil {
		return constants.EmptyString, validationError
	}
	return *requestedEffort.value, nil
}

func submitChatRequest(ginContext *gin.Context, upstreamProviders *providerRouter, chatRequest chatRequestParameters, requestTenant tenant, usageEndpoint string, managedTenants *managedTenantStore, structuredLogger *zap.SugaredLogger) {
	requestStart := time.Now()
	bindRequestTelemetryRoute(ginContext, chatRequest.provider.identifier.string(), chatRequest.model.string())
	generation, requestError := upstreamProviders.generateText(ginContext.Request.Context(), chatRequest, structuredLogger)
	if requestError != nil {
		if requestContextEnded(ginContext) {
			recordManagedUsage(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpoint, chatRequest.provider.identifier.string(), chatRequest.model.string(), ginContext.Writer.Status(), generation.usage, requestStart)
			return
		}
		markRequestOutcome(ginContext, requestFailureOutcome(requestError), managedRequestFailureOutcome(requestError))
		statusCode := statusCodeForError(requestError)
		formattingStartedAt := time.Now()
		writeProviderRequestErrorResponse(ginContext, chatRequest.provider.identifier.string(), requestError, structuredLogger)
		addRequestTelemetryPhase(ginContext.Request.Context(), requestTelemetryPhaseResponseFormatting, formattingStartedAt)
		recordManagedUsage(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpoint, chatRequest.provider.identifier.string(), chatRequest.model.string(), statusCode, generation.usage, requestStart)
		return
	}
	if requestContextEnded(ginContext) {
		recordManagedUsage(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpoint, chatRequest.provider.identifier.string(), chatRequest.model.string(), ginContext.Writer.Status(), generation.usage, requestStart)
		return
	}
	completeChatRequest(ginContext, chatRequest, generation, requestTenant, usageEndpoint, managedTenants, structuredLogger, requestStart)
}

func completeChatRequest(ginContext *gin.Context, chatRequest chatRequestParameters, generation textGenerationResult, requestTenant tenant, usageEndpoint string, managedTenants *managedTenantStore, structuredLogger *zap.SugaredLogger, requestStart time.Time) {
	formattingStartedAt := time.Now()
	mime := preferredMime(ginContext)
	formattedBody, contentType := formatResponse(generation.text, mime, chatRequest, generation.usage)
	addRequestTelemetryPhase(ginContext.Request.Context(), requestTelemetryPhaseResponseFormatting, formattingStartedAt)
	if requestContextEnded(ginContext) {
		recordManagedUsage(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpoint, chatRequest.provider.identifier.string(), chatRequest.model.string(), ginContext.Writer.Status(), generation.usage, requestStart)
		return
	}
	markRequestOutcome(ginContext, requestOutcomeSuccess, managedUsageOutcomeSuccess)
	formattingStartedAt = time.Now()
	writeTokenUsageHeaders(ginContext.Writer.Header(), generation.usage)
	ginContext.Data(http.StatusOK, contentType, []byte(formattedBody))
	addRequestTelemetryPhase(ginContext.Request.Context(), requestTelemetryPhaseResponseFormatting, formattingStartedAt)
	recordManagedUsage(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpoint, chatRequest.provider.identifier.string(), chatRequest.model.string(), http.StatusOK, generation.usage, requestStart)
}

func dictateHandler(upstreamProviders *providerRouter, providers *providerRegistry, maxInputAudioBytes int64, managedTenants *managedTenantStore, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestStart := time.Now()
		requestTenant := authenticatedTenantFromContext(ginContext)
		if rejectClientProviderCredentialsFromQuery(ginContext) {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointDictation, usageDictationProviderIdentifier(ginContext, requestTenant.defaults), usageDictationModelIdentifier(ginContext, requestTenant.defaults), requestStart)
			return
		}
		ginContext.Request.Body = http.MaxBytesReader(ginContext.Writer, ginContext.Request.Body, maxInputAudioBytes+dictationMultipartOverheadBytes)
		if parseError := ginContext.Request.ParseMultipartForm(maxInputAudioBytes); parseError != nil {
			if requestContextEnded(ginContext) {
				recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointDictation, usageDictationProviderIdentifier(ginContext, requestTenant.defaults), usageDictationModelIdentifier(ginContext, requestTenant.defaults), requestStart)
				return
			}
			statusCode := http.StatusBadRequest
			responseMessage := errorInvalidAudioForm
			var maxBytesError *http.MaxBytesError
			if errors.As(parseError, &maxBytesError) {
				statusCode = http.StatusRequestEntityTooLarge
				responseMessage = errorAudioPayloadTooLarge
			}
			ginContext.String(statusCode, responseMessage)
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointDictation, usageDictationProviderIdentifier(ginContext, requestTenant.defaults), usageDictationModelIdentifier(ginContext, requestTenant.defaults), requestStart)
			return
		}
		defer ginContext.Request.MultipartForm.RemoveAll()
		if rejectClientProviderCredentialsFromForm(ginContext) {
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointDictation, usageDictationProviderIdentifier(ginContext, requestTenant.defaults), usageDictationModelIdentifier(ginContext, requestTenant.defaults), requestStart)
			return
		}

		audioFile, header, fileError := ginContext.Request.FormFile(formFieldAudio)
		if fileError != nil {
			ginContext.String(http.StatusBadRequest, errorMissingAudioFile)
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointDictation, usageDictationProviderIdentifier(ginContext, requestTenant.defaults), usageDictationModelIdentifier(ginContext, requestTenant.defaults), requestStart)
			return
		}
		defer audioFile.Close()
		if header.Size > maxInputAudioBytes {
			ginContext.String(http.StatusRequestEntityTooLarge, errorAudioPayloadTooLarge)
			recordManagedUsageValidationFailure(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointDictation, usageDictationProviderIdentifier(ginContext, requestTenant.defaults), usageDictationModelIdentifier(ginContext, requestTenant.defaults), requestStart)
			return
		}

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
			audioReader: contextReader{contextValue: ginContext.Request.Context(), reader: audioFile},
		}
		bindRequestTelemetryRoute(ginContext, providerDefinition.identifier.string(), modelIdentifier.string())
		transcribedText, requestError := upstreamProviders.transcribeAudio(ginContext.Request.Context(), dictationRequest, structuredLogger)
		if requestError != nil {
			if requestContextEnded(ginContext) {
				recordManagedUsage(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointDictation, providerDefinition.identifier.string(), modelIdentifier.string(), ginContext.Writer.Status(), nil, requestStart)
				return
			}
			markRequestOutcome(ginContext, requestFailureOutcome(requestError), managedRequestFailureOutcome(requestError))
			statusCode := statusCodeForError(requestError)
			formattingStartedAt := time.Now()
			writeProviderRequestErrorResponse(ginContext, providerDefinition.identifier.string(), requestError, structuredLogger)
			addRequestTelemetryPhase(ginContext.Request.Context(), requestTelemetryPhaseResponseFormatting, formattingStartedAt)
			recordManagedUsage(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointDictation, providerDefinition.identifier.string(), modelIdentifier.string(), statusCode, nil, requestStart)
			return
		}
		if requestContextEnded(ginContext) {
			recordManagedUsage(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointDictation, providerDefinition.identifier.string(), modelIdentifier.string(), ginContext.Writer.Status(), nil, requestStart)
			return
		}

		completeDictationRequest(ginContext, transcribedText, requestTenant, providerDefinition, modelIdentifier, managedTenants, structuredLogger, requestStart)
	}
}

func completeDictationRequest(ginContext *gin.Context, transcribedText string, requestTenant tenant, providerDefinition providerDefinition, modelIdentifier modelID, managedTenants *managedTenantStore, structuredLogger *zap.SugaredLogger, requestStart time.Time) {
	formattingStartedAt := time.Now()
	responseBody, _ := json.Marshal(gin.H{keyText: transcribedText})
	addRequestTelemetryPhase(ginContext.Request.Context(), requestTelemetryPhaseResponseFormatting, formattingStartedAt)
	if requestContextEnded(ginContext) {
		recordManagedUsage(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointDictation, providerDefinition.identifier.string(), modelIdentifier.string(), ginContext.Writer.Status(), nil, requestStart)
		return
	}
	markRequestOutcome(ginContext, requestOutcomeSuccess, managedUsageOutcomeSuccess)
	formattingStartedAt = time.Now()
	ginContext.Data(http.StatusOK, mimeApplicationJSON, responseBody)
	addRequestTelemetryPhase(ginContext.Request.Context(), requestTelemetryPhaseResponseFormatting, formattingStartedAt)
	recordManagedUsage(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointDictation, providerDefinition.identifier.string(), modelIdentifier.string(), http.StatusOK, nil, requestStart)
}

func bindRequestTelemetryRoute(ginContext *gin.Context, providerIdentifier string, modelIdentifier string) {
	requestTelemetryFromContext(ginContext.Request.Context()).bindRoute(
		providerIdentifier,
		modelIdentifier,
		requestTimeoutStateFromContext(ginContext).budget.seconds,
	)
}

func recordManagedUsage(managedTenants *managedTenantStore, structuredLogger *zap.SugaredLogger, ginContext *gin.Context, requestTenant tenant, endpoint string, providerIdentifier string, modelIdentifier string, statusCode int, usage *tokenUsage, requestStart time.Time) {
	formattingStartedAt := time.Now()
	ginContext.Writer.Flush()
	addRequestTelemetryPhase(ginContext.Request.Context(), requestTelemetryPhaseResponseFormatting, formattingStartedAt)
	enqueueStartedAt := time.Now()
	managedTenants.usageWriter.submit(requestTenant, managedUsageEvent{
		endpoint:            endpoint,
		providerIdentifier:  providerIdentifier,
		modelIdentifier:     modelIdentifier,
		statusCode:          statusCode,
		outcomeCode:         requestTimeoutStateFromContext(ginContext).managedUsageOutcome,
		latencyMilliseconds: time.Since(requestStart).Milliseconds(),
		usage:               usage,
	}, structuredLogger)
	addRequestTelemetryPhase(ginContext.Request.Context(), requestTelemetryPhaseManagedUsageEnqueue, enqueueStartedAt)
}

func recordManagedUsageValidationFailure(managedTenants *managedTenantStore, structuredLogger *zap.SugaredLogger, ginContext *gin.Context, requestTenant tenant, endpoint string, providerIdentifier string, modelIdentifier string, requestStart time.Time) {
	statusCode := ginContext.Writer.Status()
	if outcomeCode, outcomeError := historicalManagedUsageOutcome(false, statusCode); outcomeError == nil {
		markManagedUsageOutcome(ginContext, outcomeCode)
	}
	recordManagedUsage(managedTenants, structuredLogger, ginContext, requestTenant, endpoint, providerIdentifier, modelIdentifier, statusCode, nil, requestStart)
}

func usageTextProviderIdentifier(ginContext *gin.Context, defaults textRequestDefaults) string {
	providerIdentifier := strings.TrimSpace(ginContext.Query(queryParameterProvider))
	if providerIdentifier != constants.EmptyString {
		return providerIdentifier
	}
	return defaults.provider
}

func usageTextModelIdentifier(ginContext *gin.Context, bodyModel string, defaults textRequestDefaults) string {
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
	switch rawValue {
	case "true":
		return true, nil
	case "false":
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
	case errors.Is(requestError, ErrUnknownProvider), errors.Is(requestError, ErrUnknownModel), errors.Is(requestError, ErrUnsupportedCapability), errors.Is(requestError, ErrUnsupportedEndpoint), errors.Is(requestError, ErrConflictingModelParameters), errors.Is(requestError, ErrInvalidChatMessages), errors.Is(requestError, errAssetInvalid), errors.Is(requestError, errAssetMIMEMismatch):
		return http.StatusBadRequest
	case errors.Is(requestError, errAssetNotFound):
		return http.StatusNotFound
	case errors.Is(requestError, errAssetExpired), errors.Is(requestError, errAssetDeleted):
		return http.StatusGone
	case errors.Is(requestError, errAssetTooLarge), errors.Is(requestError, ErrProviderMediaLimit):
		return http.StatusRequestEntityTooLarge
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
	return requestError.Error()
}
