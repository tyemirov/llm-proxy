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

func registerMCPGeneration(server *mcp.Server, configuration Configuration, service *managementService, upstream *providerRouter, assets *tenantAssetStore) {
	// The fixed output DTO contains only JSON primitives and token counts.
	schema, _ := jsonschema.For[mcpGenerateOutput](nil)
	inputSchema, _ := jsonschema.For[mcpGenerateInput](nil)
	// Optional controls accept their declared type when present.
	inputSchema.Properties["reasoning_effort"] = &jsonschema.Schema{Type: "string"}
	inputSchema.Properties["max_tokens"] = &jsonschema.Schema{Type: "integer"}
	inputSchema.Properties["request_timeout_seconds"] = &jsonschema.Schema{Type: "integer"}
	mcp.AddTool[mcpGenerateInput, any](server, &mcp.Tool{
		Name:         mcpGenerateTextTool,
		OutputSchema: schema,
		InputSchema:  inputSchema,
		Description:  "Generate text using one owned tenant. Each call can incur provider charges.",
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
		request, err := prepareV2TextRequest(ctx, payload, input.Provider, "", nil, budget, textRequestDefaultsForProvider(input.Provider, requestTenant, service.providers), newModelValidator(service.providers.forTenant(requestTenant)), requestTenant, assets)
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
		generation, err := executeText(ctx, upstream, request, service.structuredLogger)
		status, outcome := textExecutionOutcome(ctx, err)
		enqueueManagedUsage(service.store, service.structuredLogger, ctx, requestTenant, usageEndpointMCP, status, outcome, generation.usage, startedAt)
		if status != http.StatusOK {
			return mcpToolFailure(string(outcome)), nil, nil
		}
		output := mcpGenerateOutput{Text: generation.content.text(), RequestID: identity.requestID, Provider: request.provider.identifier.string(), Model: request.model.identifier.string(), Usage: generation.usage, RequestTimeoutSeconds: budget.seconds}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: output.Text}}}, output, nil
	})
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
