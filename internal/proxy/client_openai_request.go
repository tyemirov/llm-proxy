package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type clientTextProtocol uint8

const (
	clientChatProtocol clientTextProtocol = iota
	clientResponsesProtocol
)

type openAIChatInput struct {
	Model               string                      `json:"model"`
	Messages            []openAIChatMessage         `json:"messages"`
	MaxTokens           *int                        `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int                        `json:"max_completion_tokens,omitempty"`
	ReasoningEffort     requestReasoningEffortInput `json:"reasoning_effort,omitempty"`
	Tools               []struct {
		Type     string              `json:"type"`
		Function functionDeclaration `json:"function"`
	} `json:"tools,omitempty"`
	ToolChoice        json.RawMessage `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool           `json:"parallel_tool_calls,omitempty"`
	Stream            bool            `json:"stream,omitempty"`
	StreamOptions     *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
	ResponseFormat *struct {
		Type       string            `json:"type"`
		JSONSchema *openAIJSONSchema `json:"json_schema,omitempty"`
	} `json:"response_format,omitempty"`
	Store *bool `json:"store,omitempty"`
}
type openAIJSONSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema"`
	Strict      *bool           `json:"strict,omitempty"`
}
type openAIChatMessage struct {
	Refusal    json.RawMessage    `json:"refusal,omitempty"`
	Role       string             `json:"role"`
	Content    json.RawMessage    `json:"content"`
	ToolCalls  []chatFunctionCall `json:"tool_calls,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
}
type openAIResponsesInput struct {
	Include         []string        `json:"include,omitempty"`
	PromptCacheKey  json.RawMessage `json:"prompt_cache_key,omitempty"`
	Model           string          `json:"model"`
	Input           json.RawMessage `json:"input"`
	Instructions    string          `json:"instructions,omitempty"`
	MaxOutputTokens *int            `json:"max_output_tokens,omitempty"`
	Reasoning       *struct {
		Effort  requestReasoningEffortInput `json:"effort"`
		Summary json.RawMessage             `json:"summary,omitempty"`
	} `json:"reasoning,omitempty"`
	Tools []struct {
		Type        string          `json:"type"`
		Name        string          `json:"name"`
		Description string          `json:"description,omitempty"`
		Parameters  json.RawMessage `json:"parameters"`
		Strict      *bool           `json:"strict,omitempty"`
	} `json:"tools,omitempty"`
	ToolChoice        json.RawMessage `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool           `json:"parallel_tool_calls,omitempty"`
	Stream            bool            `json:"stream,omitempty"`
	Store             *bool           `json:"store,omitempty"`
	Text              *struct {
		Verbosity json.RawMessage `json:"verbosity,omitempty"`
		Format    *struct {
			Type        string          `json:"type"`
			Name        string          `json:"name,omitempty"`
			Description string          `json:"description,omitempty"`
			Schema      json.RawMessage `json:"schema,omitempty"`
			Strict      *bool           `json:"strict,omitempty"`
		} `json:"format"`
	} `json:"text,omitempty"`
}
type responseInputItem struct {
	Type      string          `json:"type,omitempty"`
	Role      string          `json:"role,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	ID        string          `json:"id,omitempty"`
	Status    string          `json:"status,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments string          `json:"arguments,omitempty"`
	Output    *string         `json:"output,omitempty"`
}
type decodedClientCompletion struct {
	instructions string
	model        string
	candidates   []chatMessageCandidate
	tools        []functionDeclaration
	selection    *toolSelection
	parallel     *bool
	maxTokens    *int
	reasoning    requestReasoningEffortInput
	schema       json.RawMessage
	stream       bool
	includeUsage bool
}

func textContent(raw json.RawMessage, role string, protocol clientTextProtocol) (string, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	var parts []struct {
		Type        string            `json:"type"`
		Text        string            `json:"text"`
		Annotations []json.RawMessage `json:"annotations,omitempty"`
		Logprobs    []json.RawMessage `json:"logprobs,omitempty"`
	}
	if err := decodeStrictJSON(raw, &parts); err != nil {
		return "", err
	}
	var content strings.Builder
	for _, part := range parts {
		expected := "text"
		if protocol == clientResponsesProtocol {
			expected = "input_text"
			if role == "assistant" {
				expected = "output_text"
			}
		}
		if part.Type != expected || len(part.Annotations) > 0 || len(part.Logprobs) > 0 {
			return "", fmt.Errorf("unsupported content part")
		}
		content.WriteString(part.Text)
	}
	return content.String(), nil
}
func decodeToolChoice(raw json.RawMessage, protocol clientTextProtocol) (*toolSelection, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var mode string
	if json.Unmarshal(raw, &mode) == nil {
		return &toolSelection{Mode: mode}, nil
	}
	if protocol == clientChatProtocol {
		var choice struct {
			Type     string `json:"type"`
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		}
		if err := decodeStrictJSON(raw, &choice); err != nil {
			return nil, err
		}
		if choice.Type != "function" {
			return nil, fmt.Errorf("unsupported tool selection")
		}
		return &toolSelection{Mode: "function", Name: choice.Function.Name}, nil
	}
	var choice struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := decodeStrictJSON(raw, &choice); err != nil {
		return nil, err
	}
	if choice.Type != "function" {
		return nil, fmt.Errorf("unsupported tool selection")
	}
	return &toolSelection{Mode: "function", Name: choice.Name}, nil
}
func decodeClientCompletion(protocol clientTextProtocol, body []byte) (decodedClientCompletion, error) {
	result := decodedClientCompletion{}
	if protocol == clientChatProtocol {
		var input openAIChatInput
		if err := decodeStrictJSON(body, &input); err != nil {
			return result, err
		}
		if input.Store != nil && *input.Store {
			return result, fmt.Errorf("store=true unsupported")
		}
		if input.MaxTokens != nil && input.MaxCompletionTokens != nil {
			return result, fmt.Errorf("conflicting token limits")
		}
		if input.StreamOptions != nil && !input.Stream {
			return result, fmt.Errorf("stream_options require stream")
		}
		result.model = input.Model
		result.maxTokens = input.MaxCompletionTokens
		if result.maxTokens == nil {
			result.maxTokens = input.MaxTokens
		}
		result.reasoning = input.ReasoningEffort
		result.parallel = input.ParallelToolCalls
		result.stream = input.Stream
		if input.StreamOptions != nil {
			result.includeUsage = input.StreamOptions.IncludeUsage
		}
		var err error
		result.selection, err = decodeToolChoice(input.ToolChoice, protocol)
		if err != nil {
			return result, err
		}
		for _, message := range input.Messages {
			switch message.Role {
			case "system", "developer", "user", "assistant", "tool":
			default:
				return result, fmt.Errorf("unsupported role")
			}
			if len(message.Refusal) > 0 && !bytes.Equal(message.Refusal, []byte("null")) {
				return result, fmt.Errorf("refusal history unsupported")
			}
			role := message.Role
			if role == "developer" {
				role = "system"
			}
			content, err := textContent(message.Content, role, protocol)
			if err != nil {
				return result, err
			}
			calls, err := canonicalChatFunctionCalls(message.ToolCalls)
			if err != nil {
				return result, err
			}
			result.candidates = append(result.candidates, chatMessageCandidate{role: role, content: content, toolCalls: calls, toolCallID: message.ToolCallID})
		}
		for _, tool := range input.Tools {
			if tool.Type != "function" {
				return result, fmt.Errorf("unsupported tool type")
			}
			result.tools = append(result.tools, tool.Function)
		}
		if input.ResponseFormat != nil {
			switch input.ResponseFormat.Type {
			case "text":
				if input.ResponseFormat.JSONSchema != nil {
					return result, fmt.Errorf("unexpected json_schema")
				}
			case "json_schema":
				if input.ResponseFormat.JSONSchema == nil || !functionNamePattern.MatchString(input.ResponseFormat.JSONSchema.Name) || len(input.ResponseFormat.JSONSchema.Schema) == 0 {
					return result, fmt.Errorf("json_schema requires name and schema")
				}
				result.schema = input.ResponseFormat.JSONSchema.Schema
			default:
				return result, fmt.Errorf("unsupported response format")
			}
		}
		return result, nil
	}
	var input openAIResponsesInput
	if err := decodeStrictJSON(body, &input); err != nil {
		return result, err
	}
	if input.Store != nil && *input.Store {
		return result, fmt.Errorf("store=true unsupported")
	}
	if len(input.Include) > 0 || !omittedOrNull(input.PromptCacheKey) || (input.Reasoning != nil && !omittedOrNull(input.Reasoning.Summary)) || (input.Text != nil && !omittedOrNull(input.Text.Verbosity)) {
		return result, fmt.Errorf("unsupported provider-specific controls")
	}
	result.model = input.Model
	result.maxTokens = input.MaxOutputTokens
	result.parallel = input.ParallelToolCalls
	result.stream = input.Stream
	if input.Reasoning != nil {
		result.reasoning = input.Reasoning.Effort
	}
	var err error
	result.selection, err = decodeToolChoice(input.ToolChoice, protocol)
	if err != nil {
		return result, err
	}
	result.instructions = input.Instructions
	if input.Instructions != "" {
		result.candidates = append(result.candidates, chatMessageCandidate{role: "system", content: input.Instructions})
	}
	var inputText string
	if json.Unmarshal(input.Input, &inputText) == nil && !bytes.Equal(input.Input, []byte("null")) {
		result.candidates = append(result.candidates, chatMessageCandidate{role: "user", content: inputText})
	} else {
		var items []responseInputItem
		if err := decodeStrictJSON(input.Input, &items); err != nil {
			return result, err
		}
		for _, item := range items {
			switch item.Type {
			case "", "message":
				if item.CallID != "" || item.Name != "" || item.Arguments != "" || item.Output != nil {
					return result, fmt.Errorf("invalid message item")
				}
				role := item.Role
				if role == "developer" {
					role = "system"
				}
				if role != "system" && role != "user" && role != "assistant" {
					return result, fmt.Errorf("invalid message role")
				}
				content, err := textContent(item.Content, role, protocol)
				if err != nil {
					return result, err
				}
				result.candidates = append(result.candidates, chatMessageCandidate{role: role, content: content})
			case "function_call":
				if item.Role != "" || len(item.Content) > 0 || item.Output != nil {
					return result, fmt.Errorf("invalid function call")
				}
				call := functionCall{ID: item.CallID, Name: item.Name, Arguments: item.Arguments}
				// Responses represents parallel calls as adjacent items in one assistant turn.
				last := len(result.candidates) - 1
				if last >= 0 && len(result.candidates[last].toolCalls) > 0 {
					result.candidates[last].toolCalls = append(result.candidates[last].toolCalls, call)
				} else {
					result.candidates = append(result.candidates, chatMessageCandidate{role: "assistant", toolCalls: []functionCall{call}})
				}
			case "function_call_output":
				if item.Output == nil || item.Name != "" || item.Arguments != "" || item.Role != "" || len(item.Content) > 0 {
					return result, fmt.Errorf("invalid function result")
				}
				result.candidates = append(result.candidates, chatMessageCandidate{role: "tool", content: *item.Output, toolCallID: item.CallID})
			default:
				return result, fmt.Errorf("unsupported input item")
			}
		}
	}
	for _, tool := range input.Tools {
		if tool.Type != "function" {
			return result, fmt.Errorf("unsupported tool type")
		}
		result.tools = append(result.tools, functionDeclaration{Name: tool.Name, Description: tool.Description, Parameters: tool.Parameters, Strict: tool.Strict})
	}
	if input.Text != nil && input.Text.Format != nil {
		switch input.Text.Format.Type {
		case "text":
			if len(input.Text.Format.Schema) > 0 || input.Text.Format.Name != "" || input.Text.Format.Strict != nil {
				return result, fmt.Errorf("unexpected text format controls")
			}
		case "json_schema":
			if !functionNamePattern.MatchString(input.Text.Format.Name) || len(input.Text.Format.Schema) == 0 {
				return result, fmt.Errorf("json_schema requires name and schema")
			}
			result.schema = input.Text.Format.Schema
		default:
			return result, fmt.Errorf("unsupported text format")
		}
	}
	return result, nil
}

func exactClientModel(raw string) (string, string, error) {
	provider, model, ok := strings.Cut(raw, "/")
	if !ok || provider == "" || model == "" || strings.TrimSpace(raw) != raw {
		return "", "", fmt.Errorf("model must be provider/model")
	}
	return provider, model, nil
}
func resolveClientCompletion(input decodedClientCompletion, tenant tenant, providers *providerRegistry) (chatRequestParameters, error) {
	request := chatRequestParameters{}
	provider, model, err := exactClientModel(input.model)
	if err != nil {
		return request, err
	}
	request.provider, request.model, err = newModelValidator(providers.forTenant(tenant)).ResolveText(provider, model, "", "", false)
	if err != nil {
		return request, err
	}
	if request.provider.identifier.string() != provider || request.model.string() != model {
		return request, fmt.Errorf("model aliases are unsupported")
	}
	request.messages, err = newCandidateChatMessages(input.candidates, textRequestDefaultsForProvider(provider, tenant, providers).systemPrompt, "")
	if err != nil {
		return request, err
	}
	if err := validateMessagesForResolvedTextRoute(request.model, request.messages); err != nil {
		return request, err
	}
	if input.maxTokens != nil && *input.maxTokens <= 0 {
		return request, fmt.Errorf("invalid output limit")
	}
	if err := validateTextMaxTokens(request.provider, request.model, input.maxTokens); err != nil {
		return request, err
	}
	request.maxTokens = input.maxTokens
	request.reasoningEffort, err = requestReasoningEffortForResolvedTextRoute(request.provider, request.model, tenant.defaults.reasoningEffort, input.reasoning)
	if err != nil {
		return request, err
	}
	request.tools, err = newCallerTools(input.tools, input.selection, input.parallel, request.model, request.messages)
	if err != nil {
		return request, err
	}
	if len(input.schema) > 0 {
		if request.tools != nil {
			return request, fmt.Errorf("caller tools and structured output cannot be combined")
		}
		encoded, _ := json.Marshal(struct {
			Schema json.RawMessage `json:"schema"`
		}{input.schema})
		request.structuredOutput, err = newStructuredOutputSchema(encoded)
		if err != nil {
			return request, err
		}
		if err := validateStructuredOutputRoute(request.model, request.structuredOutput); err != nil {
			return request, err
		}
	}
	request.requestDisplay = request.messages.requestDisplayText()
	return request, nil
}
func openAITextHandler(protocol clientTextProtocol, maxBytes int64, providers *providerRegistry, coordinator *providerRouter, store *managedTenantStore, logger *zap.SugaredLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		tenant := authenticatedTenantFromContext(c)
		reject := func(status int, code, message string) {
			writeOpenAIError(c, status, code, message)
			recordManagedUsageValidationFailure(store, logger, c, tenant, usageEndpointV2, start)
		}
		if c.Request.URL.RawQuery != "" {
			reject(400, "unsupported_parameter", "Query parameters are unsupported.")
			return
		}
		if c.ContentType() != "application/json" {
			reject(415, "unsupported_media_type", "Use application/json.")
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			if requestContextEnded(c) {
				recordManagedUsageValidationFailure(store, logger, c, tenant, usageEndpointV2, start)
				return
			}
			reject(413, "request_too_large", "The request body exceeds its limit.")
			return
		}
		decoded, err := decodeClientCompletion(protocol, body)
		if err != nil {
			reject(400, "invalid_request", "Invalid or unsupported request fields.")
			return
		}
		request, err := resolveClientCompletion(decoded, tenant, providers)
		if err != nil {
			bindRejectedTextRequestRoute(c, request.provider, request.model, err)
			reject(400, "invalid_request", "Invalid or unsupported model, messages, or controls.")
			return
		}
		bindRequestTelemetryRoute(c, request.provider, request.model.identifier)
		encoder := func(c *gin.Context, request chatRequestParameters, result completionResult) {
			encodeOpenAICompletion(c, protocol, decoded, request, result)
		}
		submitChatRequest(c, coordinator, request, tenant, usageEndpointV2, store, logger, encoder)
	}
}

func omittedOrNull(raw json.RawMessage) bool {
	return len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}
