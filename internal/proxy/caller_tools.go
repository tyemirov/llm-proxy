package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
)

const chatRoleTool chatRole = "tool"

// These wire values are shared by the native protocol and its domain constructor.
type functionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
	Strict      *bool           `json:"strict,omitempty"`
}
type functionCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
type toolSelection struct {
	Mode string `json:"mode"`
	Name string `json:"name,omitempty"`
}
type callerTools struct {
	declarations []functionDeclaration
	selection    toolSelection
	parallel     *bool
}

var functionNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON object: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("expected one JSON value")
	}
	return nil
}

func newCallerTools(declarations []functionDeclaration, selection *toolSelection, parallel *bool, model textModelDefinition, messages chatMessages) (*callerTools, error) {
	hasHistory := messages.hasToolHistory()
	if len(declarations) == 0 && selection == nil && parallel == nil && !hasHistory {
		return nil, nil
	}
	if !model.callerTools {
		return nil, fmt.Errorf("%w: caller tools unsupported", ErrInvalidChatMessages)
	}
	names := map[string]bool{}
	for _, declaration := range declarations {
		if !functionNamePattern.MatchString(declaration.Name) || names[declaration.Name] {
			return nil, fmt.Errorf("%w: invalid or duplicate tool name", ErrInvalidChatMessages)
		}
		names[declaration.Name] = true
		raw := []byte(`{"schema":` + string(declaration.Parameters) + `}`)
		schema, err := newStructuredOutputSchema(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid tool parameters", ErrInvalidChatMessages)
		}
		document := schema.document.(map[string]any)
		if declaration.Strict != nil && *declaration.Strict {
			if err := validateStructuredOutputSchemaSubset(document, openAIStructuredOutputRules, true); err != nil {
				return nil, fmt.Errorf("%w: strict tool parameters: %v", ErrInvalidChatMessages, err)
			}
		}
		if document["type"] != "object" {
			return nil, fmt.Errorf("%w: tool parameters require object", ErrInvalidChatMessages)
		}
	}
	chosen := toolSelection{Mode: "auto"}
	if selection != nil {
		chosen = *selection
	}
	switch chosen.Mode {
	case "auto", "none", "required":
		if chosen.Name != "" {
			return nil, fmt.Errorf("%w: unexpected tool name", ErrInvalidChatMessages)
		}
	case "function":
		if !names[chosen.Name] {
			return nil, fmt.Errorf("%w: unknown selected tool", ErrInvalidChatMessages)
		}
	default:
		return nil, fmt.Errorf("%w: invalid tool selection", ErrInvalidChatMessages)
	}
	if len(declarations) == 0 && (selection != nil || parallel != nil) {
		return nil, fmt.Errorf("%w: tools required", ErrInvalidChatMessages)
	}
	return &callerTools{declarations: declarations, selection: chosen, parallel: parallel}, nil
}

func validateFunctionCalls(calls []functionCall) error {
	seen := map[string]bool{}
	for _, call := range calls {
		var arguments map[string]json.RawMessage
		if call.ID == "" || seen[call.ID] || !functionNamePattern.MatchString(call.Name) || json.Unmarshal([]byte(call.Arguments), &arguments) != nil || arguments == nil {
			return fmt.Errorf("invalid function call")
		}
		seen[call.ID] = true
	}
	return nil
}

func validateToolHistory(messages chatMessages) error {
	pending := map[string]bool{}
	seen := map[string]bool{}
	for _, message := range messages {
		if message.role == chatRoleTool {
			if message.toolCallID == "" || !pending[message.toolCallID] || len(message.toolCalls) > 0 {
				return fmt.Errorf("%w: unmatched tool result", ErrInvalidChatMessages)
			}
			delete(pending, message.toolCallID)
			continue
		}
		if len(pending) > 0 || message.toolCallID != "" {
			return fmt.Errorf("%w: missing tool result", ErrInvalidChatMessages)
		}
		if len(message.toolCalls) > 0 {
			if message.role != chatRoleAssistant || validateFunctionCalls(message.toolCalls) != nil {
				return fmt.Errorf("%w: invalid assistant tool calls", ErrInvalidChatMessages)
			}
			for _, call := range message.toolCalls {
				if seen[call.ID] {
					return fmt.Errorf("%w: duplicate call identifier", ErrInvalidChatMessages)
				}
				seen[call.ID] = true
				pending[call.ID] = true
			}
		}
	}
	if len(pending) > 0 {
		return fmt.Errorf("%w: missing tool results", ErrInvalidChatMessages)
	}
	return nil
}
func (messages chatMessages) hasToolHistory() bool {
	for _, m := range messages {
		if len(m.toolCalls) > 0 || m.role == chatRoleTool {
			return true
		}
	}
	return false
}

func (tools *callerTools) validateResult(calls []functionCall) error {
	if len(calls) == 0 {
		if tools != nil && (tools.selection.Mode == "required" || tools.selection.Mode == "function") {
			return fmt.Errorf("%w: required function call missing", ErrProviderAPI)
		}
		return nil
	}
	if tools == nil || tools.selection.Mode == "none" || (tools.parallel != nil && !*tools.parallel && len(calls) > 1) {
		return fmt.Errorf("%w: unexpected function calls", ErrProviderAPI)
	}
	for _, call := range calls {
		found := false
		for _, decl := range tools.declarations {
			if decl.Name == call.Name {
				found = true
				break
			}
		}
		if !found || (tools.selection.Mode == "function" && tools.selection.Name != call.Name) {
			return fmt.Errorf("%w: unexpected function name", ErrProviderAPI)
		}
	}
	return nil
}

type chatFunctionCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func chatFunctionCalls(calls []functionCall) []chatFunctionCall {
	result := make([]chatFunctionCall, 0, len(calls))
	for _, call := range calls {
		item := chatFunctionCall{ID: call.ID, Type: "function"}
		item.Function.Name = call.Name
		item.Function.Arguments = call.Arguments
		result = append(result, item)
	}
	return result
}
func canonicalChatFunctionCalls(calls []chatFunctionCall) ([]functionCall, error) {
	result := make([]functionCall, 0, len(calls))
	for _, call := range calls {
		if call.Type != "function" {
			return nil, fmt.Errorf("unsupported tool type")
		}
		result = append(result, functionCall{ID: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments})
	}
	return result, validateFunctionCalls(result)
}
func (tools *callerTools) responsesTools() []map[string]any {
	result := make([]map[string]any, 0, len(tools.declarations))
	for _, decl := range tools.declarations {
		item := map[string]any{"type": "function", "name": decl.Name, "parameters": decl.Parameters, "description": decl.Description}
		if decl.Strict != nil {
			item["strict"] = *decl.Strict
		}
		result = append(result, item)
	}
	return result
}
func (tools *callerTools) responsesChoice() any {
	if tools.selection.Mode == "function" {
		return map[string]string{"type": "function", "name": tools.selection.Name}
	}
	return tools.selection.Mode
}
func (tools *callerTools) applyResponses(payload map[string]any) {
	if tools == nil || len(tools.declarations) == 0 {
		return
	}
	existing, _ := payload["tools"].([]map[string]any)
	payload["tools"] = append(existing, tools.responsesTools()...)
	payload["tool_choice"] = tools.responsesChoice()
	if tools.parallel != nil {
		payload["parallel_tool_calls"] = *tools.parallel
	}
}
