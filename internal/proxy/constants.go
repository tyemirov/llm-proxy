package proxy

const (
	// LogLevelDebug indicates that the application should log debug information.
	LogLevelDebug = "debug"

	// LogLevelInfo indicates that the application should log informational messages.
	LogLevelInfo = "info"

	headerAuthorization          = "Authorization"
	headerContentType            = "Content-Type"
	headerAccept                 = "Accept"
	headerAuthorizationPrefix    = "Bearer "
	headerLLMProxyRequestTokens  = "X-LLM-Proxy-Request-Tokens"
	headerLLMProxyResponseTokens = "X-LLM-Proxy-Response-Tokens"
	headerLLMProxyTotalTokens    = "X-LLM-Proxy-Total-Tokens"

	// rootPath defines the HTTP path for the root endpoint.
	rootPath = "/"
	// dictatePath defines the HTTP path for audio transcription requests.
	dictatePath = "/dictate"

	queryParameterPrompt       = "prompt"
	queryParameterKey          = "key"
	queryParameterProvider     = "provider"
	queryParameterModel        = "model"
	queryParameterWebSearch    = "web_search"
	queryParameterSystemPrompt = "system_prompt"
	queryParameterMaxTokens    = "max_tokens"
	queryParameterFormat       = "format"

	formFieldAudio = "audio"
	formFieldFile  = "file"

	redactedPlaceholder = "***REDACTED***"

	mimeApplicationJSON = "application/json"
	mimeApplicationXML  = "application/xml"
	mimeTextXML         = "text/xml"
	mimeTextCSV         = "text/csv"
	mimeTextPlain       = "text/plain; charset=utf-8"

	errorMissingPrompt         = "missing prompt parameter"
	errorInvalidJSONRequest    = "invalid JSON request"
	errorInvalidMaxTokens      = "invalid max_tokens parameter"
	errorPromptPayloadTooLarge = "prompt payload too large"
	// errorMissingClientKey indicates that the key query parameter is missing.
	errorMissingClientKey           = "unknown client key"
	errorRequestTimedOut            = "request timed out"
	errorOpenAIRequest              = "OpenAI request error"
	errorOpenAIAPI                  = "OpenAI API error"
	errorOpenAIAPINoText            = "OpenAI API error (no text)"
	errorOpenAIFailedStatus         = "OpenAI API error (failed status)"
	errorOpenAIContinue             = "OpenAI API continue error"
	errorUnknownProvider            = "unknown provider"
	errorProviderNotConfigured      = "provider not configured"
	errorUnsupportedCapability      = "unsupported provider capability"
	errorUnsupportedEndpoint        = "unsupported provider endpoint"
	errorConflictingModelParameters = "conflicting model parameters"
	errorProviderRateLimited        = "provider rate limited"
	errorProviderAPI                = "provider API error"
	errorProviderNoText             = "provider API error (no text)"
	errorInvalidAudioForm           = "invalid audio form"
	errorMissingAudioFile           = "missing audio file"
	// errorUpstreamIncomplete indicates that the upstream provider returned an incomplete response.
	errorUpstreamIncomplete = "OpenAI API error (incomplete response)"
	// errorUnknownModel indicates that a model identifier is not recognized.
	errorUnknownModel = "unknown model"
	// errorQueueFull indicates that the internal request queue cannot accept additional tasks.
	errorQueueFull = "request queue full"

	toolTypeWebSearch = "web_search"
	// reasoningEffortMedium denotes a medium reasoning effort level.
	reasoningEffortMedium = "medium"
	// reasoningEffortMinimal denotes a minimal reasoning effort level.
	reasoningEffortMinimal = "minimal"

	// responseTypeMessage identifies a message output item in the upstream response.
	responseTypeMessage = "message"

	// responseRoleAssistant identifies the assistant role in output items.
	responseRoleAssistant = "assistant"

	// responseTypeWebSearchCall identifies a web search tool call in the output array.
	responseTypeWebSearchCall = "web_search_call"

	// outputPartType identifies an output_text part in a content array.
	outputPartType = "output_text"

	// textPartType identifies a plain text part in a content array.
	textPartType = "text"

	// fallbackFinalAnswerFormat formats a message when the model does not provide a final answer.
	fallbackFinalAnswerFormat = "Model did not provide a final answer. Last web search: \"%s\""

	keyModel              = "model"
	keyInput              = "input"
	keyTemperature        = "temperature"
	keyMaxOutputTokens    = "max_output_tokens"
	keyTools              = "tools"
	keyType               = "type"
	keyToolChoice         = "tool_choice"
	keyReasoning          = "reasoning"
	keyAuto               = "auto"
	keyPreviousResponseID = "previous_response_id"
	keyEffort             = "effort"
	keyText               = "text"
	keyFormat             = "format"
	keyVerbosity          = "verbosity"
	toolChoiceNone        = "none"
	textFormatType        = "text"
	verbosityLow          = "low"

	jsonFieldID       = "id"
	jsonFieldStatus   = "status"
	jsonFieldResponse = "response"
	jsonFieldUsage    = "usage"

	statusCompleted  = "completed"
	statusSucceeded  = "succeeded"
	statusDone       = "done"
	statusCancelled  = "cancelled"
	statusFailed     = "failed"
	statusErrored    = "errored"
	statusIncomplete = "incomplete"

	jsonFieldIncompleteDetails      = "incomplete_details"
	jsonFieldReason                 = "reason"
	incompleteReasonMaxTokens       = "max_tokens"
	incompleteReasonMaxOutputTokens = "max_output_tokens"

	logFieldHTTPStatus   = "http_status"
	logFieldAPIStatus    = "api_status"
	logFieldResponseText = "response_text"
	// logFieldResponseBody captures the raw body returned by the upstream API.
	logFieldResponseBody = "response_body"
	logFieldMethod       = "method"
	logFieldPath         = "path"
	logFieldClientIP     = "client_ip"
	logFieldStatus       = "status"
	logFieldValue        = "value"
	// logFieldID identifies the response identifier logged for traceability.
	logFieldID = "id"

	// logFieldExpectedFingerprint identifies the fingerprint of the expected client key.
	logFieldExpectedFingerprint = "expected_fingerprint"

	logEventOpenAIRequestError = "OpenAI request error"
	logEventOpenAIResponse     = "OpenAI API response"
	logEventOpenAIPollError    = "OpenAI poll error"
	// logEventOpenAIPollResponseBody records the body returned when polling OpenAI for a response.
	logEventOpenAIPollResponseBody = "OpenAI poll response body"
	logEventOpenAIContinueError    = "OpenAI continue error"
	// logEventOpenAIInitialResponseBody records the body of the initial response from OpenAI.
	logEventOpenAIInitialResponseBody = "OpenAI initial response body"
	logEventProviderRequestError      = "provider request error"
	// logEventMissingFinalMessage indicates that the response completed without a final assistant message.
	logEventMissingFinalMessage           = "response is 'completed' but lacks final message; starting synthesis continuation"
	logEventForbiddenRequest              = "forbidden request"
	logEventRequestReceived               = "request received"
	logEventResponseSent                  = "response sent"
	logEventBuildHTTPRequest              = "build HTTP request failed"
	logEventParseWebSearchParameterFailed = "parse web_search parameter failed"

	responseRequestAttribute = "request"
)
