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
	v2Path   = "/v2"
	// dictatePath defines the HTTP path for audio transcription requests.
	dictatePath = "/dictate"

	queryParameterPrompt          = "prompt"
	queryParameterKey             = "key"
	queryParameterProvider        = "provider"
	queryParameterModel           = "model"
	queryParameterWebSearch       = "web_search"
	queryParameterSystemPrompt    = "system_prompt"
	queryParameterMaxTokens       = "max_tokens"
	queryParameterReasoningEffort = "reasoning_effort"
	queryParameterFormat          = "format"

	formFieldAudio = "audio"
	formFieldFile  = "file"

	dictationMultipartOverheadBytes = int64(2 * 1024 * 1024)

	contextKeyRequestID = "request_id"
	contextKeyTenant    = "tenant"

	mimeApplicationJSON = "application/json"
	mimeApplicationXML  = "application/xml"
	mimeTextXML         = "text/xml"
	mimeTextCSV         = "text/csv"
	mimeTextPlain       = "text/plain; charset=utf-8"

	errorMissingPrompt          = "missing prompt parameter"
	errorInvalidJSONRequest     = "invalid JSON request"
	errorInvalidWebSearch       = "invalid web_search parameter"
	errorInvalidMaxTokens       = "invalid max_tokens parameter"
	errorInvalidReasoningEffort = "invalid reasoning_effort parameter"
	errorPromptPayloadTooLarge  = "prompt payload too large"
	// errorMissingClientKey indicates that the key query parameter is missing.
	errorMissingClientKey           = "unknown client key"
	errorOpenAIRequest              = "OpenAI request error"
	errorOpenAIAPI                  = "OpenAI API error"
	errorOpenAIAPINoText            = "OpenAI API error (no text)"
	errorOpenAIFailedStatus         = "OpenAI API error (failed status)"
	errorOpenAIContinue             = "OpenAI API continue error"
	errorUnknownProvider            = "unknown provider"
	errorProviderNotConfigured      = "provider not configured"
	errorClientProviderAPIKey       = "client provider API keys are not accepted"
	errorUnsupportedCapability      = "unsupported provider capability"
	errorUnsupportedEndpoint        = "unsupported provider endpoint"
	errorConflictingModelParameters = "conflicting model parameters"
	errorProviderRateLimited        = "provider rate limited"
	errorProviderAPI                = "provider API error"
	errorProviderNoText             = "provider API error (no text)"
	errorInvalidChatMessages        = "invalid messages parameter"
	errorConflictingPromptMessages  = "conflicting prompt and messages parameters"
	errorMissingMessages            = "missing messages parameter"
	errorUnsupportedPromptParameter = "unsupported prompt parameter"
	errorUnsupportedSystemPrompt    = "unsupported system_prompt parameter"
	errorInvalidAudioForm           = "invalid audio form"
	errorMissingAudioFile           = "missing audio file"
	errorAudioPayloadTooLarge       = "audio payload too large"
	// errorUnknownModel indicates that a model identifier is not recognized.
	errorUnknownModel = "unknown model"
	// errorQueueFull indicates that the internal request queue cannot accept additional tasks.
	errorQueueFull = "request queue full"

	toolTypeWebSearch = "web_search"

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
	keyBackground         = "background"
	keyStore              = "store"
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

	jsonFieldID           = "id"
	jsonFieldChoices      = "choices"
	jsonFieldContent      = "content"
	jsonFieldFinishReason = "finish_reason"
	jsonFieldIndex        = "index"
	jsonFieldMessage      = "message"
	jsonFieldMessages     = "messages"
	jsonFieldObject       = "object"
	jsonFieldOutputText   = "output_text"
	jsonFieldRole         = "role"
	jsonFieldStatus       = "status"
	jsonFieldResponse     = "response"
	jsonFieldUsage        = "usage"
	chatCompletionObject  = "chat.completion"
	finishReasonStop      = "stop"

	statusCompleted  = "completed"
	statusCancelled  = "cancelled"
	statusFailed     = "failed"
	statusIncomplete = "incomplete"
	statusInProgress = "in_progress"
	statusQueued     = "queued"

	logFieldHTTPStatus                      = "http_status"
	logFieldAPIStatus                       = "api_status"
	logFieldMethod                          = "method"
	logFieldClientIP                        = "client_ip"
	logFieldStatus                          = "status"
	logFieldTenantID                        = "tenant_id"
	logFieldEndpoint                        = "endpoint"
	logFieldProvider                        = "provider"
	logFieldModel                           = "model"
	logFieldRequestID                       = "request_id"
	logFieldRetryAfter                      = "retry_after"
	logFieldRetryable                       = "retryable"
	logFieldUpstreamCode                    = "upstream_status"
	logFieldRequestTimeoutSeconds           = "request_timeout_seconds"
	logFieldOutcome                         = "outcome"
	logFieldTotalLatencyMilliseconds        = "total_latency_ms"
	logFieldAuthenticationMilliseconds      = "authentication_ms"
	logFieldUpstreamAdmissionMilliseconds   = "upstream_admission_ms"
	logFieldUpstreamRateLimitMilliseconds   = "upstream_rate_limit_wait_ms"
	logFieldProviderHTTPMilliseconds        = "provider_http_ms"
	logFieldProviderPollWaitMilliseconds    = "provider_poll_wait_ms"
	logFieldContinuationWaitMilliseconds    = "continuation_wait_ms"
	logFieldResponseFormattingMilliseconds  = "response_formatting_ms"
	logFieldManagedUsageEnqueueMilliseconds = "managed_usage_enqueue_ms"
	logFieldProgressKind                    = "progress_kind"
	logFieldAttemptCount                    = "attempt_count"
	logFieldPollCount                       = "poll_count"
	logFieldProviderState                   = "provider_state"
	logFieldCompletionSignal                = "completion_signal"
	logFieldElapsedMilliseconds             = "elapsed_ms"
	logFieldCurrentOutputBytes              = "current_output_bytes"
	logFieldAccumulatedOutputBytes          = "accumulated_output_bytes"
	logEventOpenAIRequestError              = "OpenAI request error"
	logEventOpenAIResponse                  = "OpenAI API response"
	logEventOpenAIPollError                 = "OpenAI poll error"
	logEventOpenAIContinueError             = "OpenAI continue error"
	logEventProviderRequestError            = "provider request error"
	logEventProviderFailure                 = "provider failure"
	// logEventMissingFinalMessage indicates that the response completed without a final assistant message.
	logEventMissingFinalMessage           = "response is 'completed' but lacks final message; starting synthesis continuation"
	logEventForbiddenRequest              = "forbidden request"
	logEventRequestReceived               = "request received"
	logEventResponseSent                  = "response sent"
	logEventBuildHTTPRequest              = "build HTTP request failed"
	logEventParseWebSearchParameterFailed = "parse web_search parameter failed"
	logEventUsageRecordDropped            = "management usage record dropped"
	logEventUsageRecordFailed             = "management usage record failed"
	logEventRequestPhaseSummary           = "proxy request phase summary"
	logEventProviderProgress              = "proxy provider progress"

	telemetryProgressKindOpenAICreate        = "openai_create"
	telemetryProgressKindOpenAIPoll          = "openai_poll"
	telemetryProgressKindContinuationAttempt = "continuation_attempt"
	telemetryProviderStateUnknown            = "unknown"
	telemetryCompletionPending               = "pending"
	telemetryCompletionComplete              = "complete"
	telemetryCompletionOutputLimit           = "output_limit"
	telemetryCompletionCanceled              = "canceled"
	telemetryCompletionFailure               = "failure"

	responseRequestAttribute = "request"
)
