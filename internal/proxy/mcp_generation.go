package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
)

type mcpMessage struct {
	ToolCalls   []functionCall                 `json:"tool_calls,omitempty"`
	ToolCallID  string                         `json:"tool_call_id,omitempty"`
	Role        string                         `json:"role"`
	Content     string                         `json:"content"`
	Order       *int                           `json:"order,omitempty"`
	Attachments []chatMessageAttachmentPayload `json:"attachments,omitempty"`
}

type mcpGenerateInput struct {
	TenantID              string       `json:"tenant_id"`
	IdempotencyKey        *string      `json:"idempotency_key,omitempty"`
	Messages              []mcpMessage `json:"messages"`
	Provider              string       `json:"provider,omitempty"`
	Model                 string       `json:"model,omitempty"`
	WebSearch             bool         `json:"web_search,omitempty"`
	MaxTokens             *int         `json:"max_tokens,omitempty"`
	ReasoningEffort       *string      `json:"reasoning_effort,omitempty"`
	RequestTimeoutSeconds *int         `json:"request_timeout_seconds,omitempty"`
}

type mcpGenerateOutput struct {
	Text                  string      `json:"text"`
	RequestID             string      `json:"request_id"`
	Provider              string      `json:"provider"`
	Model                 string      `json:"model"`
	Usage                 *tokenUsage `json:"usage"`
	RequestTimeoutSeconds int         `json:"request_timeout_seconds"`
}

type mcpGeneratePendingOutput struct {
	State                 string `json:"state"`
	RequestID             string `json:"request_id"`
	Provider              string `json:"provider"`
	Model                 string `json:"model"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
}

func registerMCPGeneration(server *mcp.Server, configuration Configuration, service *managementService, upstream *providerRouter, assets *tenantAssetStore) {
	// The fixed output DTO contains only JSON primitives and token counts.
	schema, _ := jsonschema.For[mcpGenerateOutput](nil)
	pendingSchema, _ := jsonschema.For[mcpGeneratePendingOutput](nil)
	pendingSchema.Properties["state"].Enum = []any{structuredRequestStateDispatched}
	inputSchema, _ := jsonschema.For[mcpGenerateInput](nil)
	// Optional controls accept their declared type when present.
	inputSchema.Properties["reasoning_effort"] = &jsonschema.Schema{Type: "string"}
	inputSchema.Properties["max_tokens"] = &jsonschema.Schema{Type: "integer"}
	inputSchema.Properties["request_timeout_seconds"] = &jsonschema.Schema{Type: "integer"}
	inputSchema.Properties["idempotency_key"] = &jsonschema.Schema{Type: "string", Pattern: idempotencyKeyPattern.String()}
	mcp.AddTool[mcpGenerateInput, any](server, &mcp.Tool{
		Name:         mcpGenerateTextTool,
		OutputSchema: &jsonschema.Schema{Type: "object", OneOf: []*jsonschema.Schema{schema, pendingSchema}},
		InputSchema:  inputSchema,
		Description:  "Generate text using one owned tenant. Hosted access requires idempotency_key. Identical hosted requests return the accepted execution. Provider charges can apply.",
		Annotations:  &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(false), IdempotentHint: false, OpenWorldHint: new(true)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input mcpGenerateInput) (*mcp.CallToolResult, any, error) {
		identity := ctx.Value(mcpIdentityKey{}).(mcpIdentity)
		requestTenant, err := service.store.mcpTenant(identity.subject, input.TenantID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return mcpToolFailure(mcpNotFound), nil, nil
		}
		if err != nil {
			return mcpToolFailure(string(managedUsageOutcomeProxyError)), nil, nil
		}
		startedAt := time.Now()
		telemetry := newRequestTelemetry(identity.requestID, mcpPath)
		ctx = requestContextWithTelemetry(ctx, telemetry)
		var timeoutValues []string
		if input.RequestTimeoutSeconds != nil {
			timeoutValues = []string{strconv.Itoa(*input.RequestTimeoutSeconds)}
		}
		budget, valid := configuration.requestTimeoutPolicy.resolve(timeoutValues)
		if !valid {
			enqueueManagedUsage(service.store, service.structuredLogger, ctx, requestTenant, usageEndpointMCP, http.StatusBadRequest, managedUsageOutcomeInvalidRequest, nil, startedAt)
			return mcpToolFailure("invalid_request_timeout"), nil, nil
		}
		ctx, cancel := context.WithTimeoutCause(ctx, budget.duration, errRequestTimeoutBudgetExpired)
		defer cancel()
		messages := make([]chatV2MessagePayload, 0, len(input.Messages))
		for _, message := range input.Messages {
			var attachments json.RawMessage
			if message.Attachments != nil {
				encoded, _ := json.Marshal(message.Attachments)
				attachments = encoded
			}
			messages = append(messages, chatV2MessagePayload{ToolCalls: message.ToolCalls, ToolCallID: message.ToolCallID, Role: message.Role, Content: message.Content, Order: message.Order, Attachments: attachments})
		}
		payload := chatV2RequestPayload{Messages: &messages, Model: input.Model, WebSearch: input.WebSearch, MaxTokens: input.MaxTokens}
		if input.ReasoningEffort != nil {
			payload.ReasoningEffort = requestReasoningEffortInput{value: input.ReasoningEffort, supplied: true}
		}
		var idempotencyValues []string
		if input.IdempotencyKey != nil {
			idempotencyValues = []string{*input.IdempotencyKey}
		}
		request, err := prepareV2TextRequest(ctx, payload, input.Provider, "", idempotencyValues, budget, textRequestDefaultsForProvider(input.Provider, requestTenant, service.providers), newModelValidator(service.providers.forTenant(requestTenant)), requestTenant, assets)
		if err != nil {
			status, _ := textValidationResponse(err)
			outcome := managedUsageOutcomeInvalidRequest
			if errors.Is(err, ErrProviderNotConfigured) {
				outcome = managedUsageOutcomeProviderNotConfigured
			}
			if errors.Is(err, errAssetStore) {
				status, outcome = http.StatusInternalServerError, managedUsageOutcomeProxyError
			}
			enqueueManagedUsage(service.store, service.structuredLogger, ctx, requestTenant, usageEndpointMCP, status, outcome, nil, startedAt)
			return mcpToolFailure(string(outcome)), nil, nil
		}
		defer request.messages.closeMedia()
		if request.provider.hostedGrantID != "" {
			ctx = context.WithValue(ctx, hostedTextIdentityContextKey{}, hostedTextIdentity{tenant: requestTenant, key: request.idempotencyKey, executionID: identity.requestID})
		}
		generation, err := executeText(ctx, upstream, request, service.structuredLogger)
		var replay *hostedRequestReplay
		if errors.As(err, &replay) {
			return mcpHostedTextReplay(replay.record, budget)
		}
		status, outcome := textExecutionOutcome(ctx, err)
		enqueueManagedUsage(service.store, service.structuredLogger, ctx, requestTenant, usageEndpointMCP, status, outcome, generation.usage, startedAt)
		if status != http.StatusOK {
			if code, hosted := hostedRequestErrorCode(err); hosted {
				return mcpToolFailure(code), nil, nil
			}
			return mcpToolFailure(string(outcome)), nil, nil
		}
		output := mcpGenerateOutput{Text: generation.content.text(), RequestID: identity.requestID, Provider: request.provider.identifier.string(), Model: request.model.identifier.string(), Usage: generation.usage, RequestTimeoutSeconds: budget.seconds}
		if generation.receipt != nil {
			output.RequestID = generation.receipt.executionID
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: output.Text}}}, output, nil
	})
}

func mcpHostedTextReplay(record structuredRequestRecord, budget requestTimeoutBudget) (*mcp.CallToolResult, any, error) {
	if record.State == structuredRequestStateDispatched {
		output := mcpGeneratePendingOutput{State: record.State, RequestID: record.ProxyRequestID, Provider: record.Provider, Model: record.Model, RequestTimeoutSeconds: budget.seconds}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "The accepted request is still in progress. Repeat the same request and idempotency key to read its result."}}}, output, nil
	}
	code := llmproxycontract.ErrorCodeStructuredRequestFailed
	if record.State == structuredRequestStateUncertain {
		code = llmproxycontract.ErrorCodeStructuredRequestOutcomeUnknown
	}
	result := mcpToolFailure(code)
	result.StructuredContent = struct {
		Code      string `json:"code"`
		State     string `json:"state"`
		RequestID string `json:"request_id"`
	}{Code: code, State: record.State, RequestID: record.ProxyRequestID}
	return result, nil, nil
}

type mcpTextDefaults struct {
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
}

type mcpTextRoute struct {
	Provider         string   `json:"provider"`
	Model            string   `json:"model"`
	WebSearch        bool     `json:"web_search"`
	MediaInputs      []string `json:"media_inputs"`
	ReasoningEfforts []string `json:"reasoning_efforts"`
}

type mcpTenantRoutes struct {
	TenantID string          `json:"tenant_id"`
	Defaults mcpTextDefaults `json:"defaults"`
	Routes   []mcpTextRoute  `json:"routes"`
}

func mcpRoutesFor(requestTenant tenant, providers *providerRegistry) mcpTenantRoutes {
	result := mcpTenantRoutes{TenantID: requestTenant.identifier.string(), Defaults: mcpTextDefaults{Provider: requestTenant.defaults.provider, Model: requestTenant.defaults.model, ReasoningEffort: requestTenant.defaults.reasoningEffort}, Routes: []mcpTextRoute{}}
	registry := providers.forTenant(requestTenant)
	for _, provider := range registry.definitions {
		for _, model := range provider.textModels {
			if _, _, err := registry.resolveTextRequest(provider.identifier.string(), model.identifier.string(), "", "", false); err != nil {
				continue
			}
			route := mcpTextRoute{Provider: provider.identifier.string(), Model: model.identifier.string(), WebSearch: model.supportsWebSearch, MediaInputs: []string{}, ReasoningEfforts: []string{}}
			for input := range model.mediaInputs {
				route.MediaInputs = append(route.MediaInputs, string(input))
			}
			sort.Strings(route.MediaInputs)
			if model.reasoningEffort != nil {
				route.ReasoningEfforts = append(route.ReasoningEfforts, model.reasoningEffort.efforts...)
			}
			result.Routes = append(result.Routes, route)
		}
	}
	sort.Slice(result.Routes, func(a, b int) bool {
		if result.Routes[a].Provider == result.Routes[b].Provider {
			return result.Routes[a].Model < result.Routes[b].Model
		}
		return result.Routes[a].Provider < result.Routes[b].Provider
	})
	return result
}
