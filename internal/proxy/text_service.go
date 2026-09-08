package proxy

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type textValidationError struct{ message string }

func (err *textValidationError) Error() string { return err.message }
func textValidationResponse(err error) (int, string) {
	var validation *textValidationError
	if errors.As(err, &validation) {
		return http.StatusBadRequest, validation.Error()
	}
	return statusCodeForError(err), responseMessageForError(err)
}
func bindTextTelemetryRoute(ctx context.Context, provider providerDefinition, model modelID, budget requestTimeoutBudget) {
	if telemetry := requestTelemetryFromContext(ctx); telemetry != nil {
		telemetry.bindRoute(managedUsageRoute{providerIdentifier: provider.identifier, modelIdentifier: model}, budget.seconds)
	}
}

// executeText is the shared execution boundary for native and MCP generation.
// The provider router owns admission, rate limits, queues, retries, and continuation.
func executeText(ctx context.Context, upstream *providerRouter, request chatRequestParameters, logger *zap.SugaredLogger) (completionResult, error) {
	return upstream.generateText(ctx, request, logger)
}

func prepareV2TextRequest(ctx context.Context, payload chatV2RequestPayload, queryProvider, queryModel string, idempotencyValues []string, budget requestTimeoutBudget, defaults textRequestDefaults, validator *modelValidator, requestTenant tenant, assetStore *tenantAssetStore) (chatRequestParameters, error) {
	if payload.Prompt != nil {
		return chatRequestParameters{}, &textValidationError{message: errorUnsupportedPromptParameter}
	}
	if payload.SystemPrompt != nil {
		return chatRequestParameters{}, &textValidationError{message: errorUnsupportedSystemPrompt}
	}
	if payload.Messages == nil {
		return chatRequestParameters{}, &textValidationError{message: errorMissingMessages}
	}

	modelIdentifier, modelParameterError := resolveJSONModelParameter(queryModel, payload.Model)
	if modelParameterError != nil {
		return chatRequestParameters{}, modelParameterError
	}
	if payload.MaxTokens != nil && *payload.MaxTokens <= 0 {
		return chatRequestParameters{}, &textValidationError{message: errorInvalidMaxTokens}
	}

	providerDefinition, resolvedModel, verificationError := validator.ResolveText(
		queryProvider,
		modelIdentifier,
		defaults.provider,
		defaults.model,
		payload.WebSearch,
	)
	if verificationError != nil {
		if errors.Is(verificationError, ErrProviderNotConfigured) {
			bindTextTelemetryRoute(ctx, providerDefinition, resolvedModel.identifier, budget)
		}
		return chatRequestParameters{}, verificationError
	}
	bindTextTelemetryRoute(ctx, providerDefinition, resolvedModel.identifier, budget)
	if maxTokensError := validateTextMaxTokens(providerDefinition, resolvedModel, payload.MaxTokens); maxTokensError != nil {
		return chatRequestParameters{}, &textValidationError{message: errorInvalidMaxTokens}
	}
	reasoningEffort, reasoningEffortError := requestReasoningEffortForResolvedTextRoute(providerDefinition, resolvedModel, defaults.reasoningEffort, payload.ReasoningEffort)
	if reasoningEffortError != nil {
		return chatRequestParameters{}, &textValidationError{message: errorInvalidReasoningEffort}
	}
	structuredOutput, structuredOutputError := newStructuredOutputSchema(payload.StructuredOutput)
	if structuredOutputError != nil {
		return chatRequestParameters{}, &textValidationError{message: "invalid structured_output"}
	}
	idempotencyKey, idempotencyError := structuredRequestKeyValues(idempotencyValues, structuredOutput)
	if idempotencyError != nil {
		return chatRequestParameters{}, &textValidationError{message: "invalid Idempotency-Key"}
	}
	if structuredOutput != nil && payload.WebSearch {
		return chatRequestParameters{}, &textValidationError{message: "structured_output does not support web_search"}
	}
	if routeError := validateStructuredOutputRoute(resolvedModel, structuredOutput); routeError != nil {
		return chatRequestParameters{}, &textValidationError{message: "structured_output is unsupported for the selected route"}
	}
	messages, messageError := newV2PayloadChatMessages(*payload.Messages, defaults.systemPrompt, requestTenant, assetStore)
	if messageError != nil {
		return chatRequestParameters{}, messageError
	}
	if routeMessageError := validateMessagesForResolvedTextRoute(resolvedModel, messages); routeMessageError != nil {
		messages.closeMedia()
		return chatRequestParameters{}, routeMessageError
	}
	if mediaCapabilityError := validateMessageMediaForResolvedTextRoute(providerDefinition, resolvedModel, messages); mediaCapabilityError != nil {
		messages.closeMedia()
		return chatRequestParameters{}, mediaCapabilityError
	}

	tools, toolsError := newCallerTools(payload.Tools, payload.ToolChoice, payload.ParallelToolCalls, resolvedModel, messages)
	if toolsError != nil || (tools != nil && structuredOutput != nil) {
		messages.closeMedia()
		return chatRequestParameters{}, &textValidationError{message: "invalid or unsupported caller tools"}
	}

	return chatRequestParameters{
		tools:            tools,
		messages:         messages,
		requestDisplay:   messages.requestDisplayText(),
		provider:         providerDefinition,
		model:            resolvedModel,
		webSearchEnabled: payload.WebSearch,
		maxTokens:        payload.MaxTokens,
		reasoningEffort:  reasoningEffort,
		structuredOutput: structuredOutput,
		idempotencyKey:   idempotencyKey,
	}, nil
}

func textExecutionOutcome(ctx context.Context, err error) (int, managedUsageOutcomeCode) {
	if cause := context.Cause(ctx); cause != nil {
		if errors.Is(cause, errRequestTimeoutBudgetExpired) {
			return http.StatusGatewayTimeout, managedUsageOutcomeRequestTimeout
		}
		return statusClientClosedRequest, managedUsageOutcomeRequestTimeout
	}
	if err != nil {
		return statusCodeForError(err), managedRequestFailureOutcome(err)
	}
	return http.StatusOK, managedUsageOutcomeSuccess
}

func enqueueManagedUsage(managedTenants *managedTenantStore, structuredLogger *zap.SugaredLogger, ctx context.Context, requestTenant tenant, endpoint string, statusCode int, outcome managedUsageOutcomeCode, usage *tokenUsage, requestStart time.Time) {
	enqueueStartedAt := time.Now()
	var route *managedUsageRoute
	if telemetry := requestTelemetryFromContext(ctx); telemetry != nil {
		route = telemetry.usageRoute()
	}
	managedTenants.usageWriter.submit(requestTenant, managedUsageEvent{
		endpoint:            endpoint,
		route:               route,
		statusCode:          statusCode,
		outcomeCode:         outcome,
		latencyMilliseconds: time.Since(requestStart).Milliseconds(),
		usage:               usage,
	}, structuredLogger)
	addRequestTelemetryPhase(ctx, requestTelemetryPhaseManagedUsageEnqueue, enqueueStartedAt)
}
