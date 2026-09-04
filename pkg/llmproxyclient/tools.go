package llmproxyclient

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// FunctionDeclaration is input for a caller-owned function. The caller executes it.
type FunctionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
	Strict      *bool           `json:"strict,omitempty"`
}

// FunctionCall contains the exact call identifier, name, and JSON argument text.
type FunctionCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolChoice selects auto, none, required, or one named function.
type ToolChoice struct {
	Mode string `json:"mode"`
	Name string `json:"name,omitempty"`
}

var clientFunctionNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func validateClientCalls(calls []FunctionCall) error {
	seen := map[string]bool{}
	for _, call := range calls {
		var args map[string]json.RawMessage
		if call.ID == "" || seen[call.ID] || !clientFunctionNamePattern.MatchString(call.Name) || json.Unmarshal([]byte(call.Arguments), &args) != nil || args == nil {
			return fmt.Errorf("%w: invalid function call", ErrInvalidClientRequest)
		}
		seen[call.ID] = true
	}
	return nil
}
func validateClientToolHistory(messages []message) error {
	pending := map[string]bool{}
	seen := map[string]bool{}
	for _, m := range messages {
		if m.role == "tool" {
			if !pending[m.toolCallID] || len(m.toolCalls) > 0 {
				return fmt.Errorf("%w: unmatched tool result", ErrInvalidClientRequest)
			}
			delete(pending, m.toolCallID)
			continue
		}
		if len(pending) > 0 || m.toolCallID != "" {
			return fmt.Errorf("%w: missing tool result", ErrInvalidClientRequest)
		}
		if len(m.toolCalls) > 0 {
			if m.role != messageRoleAssistant {
				return fmt.Errorf("%w: calls require assistant", ErrInvalidClientRequest)
			}
			if err := validateClientCalls(m.toolCalls); err != nil {
				return err
			}
			for _, call := range m.toolCalls {
				if seen[call.ID] {
					return fmt.Errorf("%w: duplicate tool identifier", ErrInvalidClientRequest)
				}
				pending[call.ID] = true
				seen[call.ID] = true
			}
		}
	}
	if len(pending) > 0 {
		return fmt.Errorf("%w: missing tool result", ErrInvalidClientRequest)
	}
	return nil
}
func newClientTools(input []FunctionDeclaration, selection *ToolChoice, parallel *bool) ([]FunctionDeclaration, *ToolChoice, error) {
	result := make([]FunctionDeclaration, 0, len(input))
	names := map[string]bool{}
	for _, declaration := range input {
		var parameters map[string]json.RawMessage
		if !clientFunctionNamePattern.MatchString(declaration.Name) || names[declaration.Name] || json.Unmarshal(declaration.Parameters, &parameters) != nil || string(parameters["type"]) != `"object"` {
			return nil, nil, fmt.Errorf("%w: invalid function declaration", ErrInvalidClientRequest)
		}
		names[declaration.Name] = true
		declaration.Parameters = append(json.RawMessage(nil), declaration.Parameters...)
		if declaration.Strict != nil {
			strict := *declaration.Strict
			declaration.Strict = &strict
		}
		result = append(result, declaration)
	}
	if selection == nil {
		if parallel != nil && len(input) == 0 {
			return nil, nil, fmt.Errorf("%w: tools required", ErrInvalidClientRequest)
		}
		return result, nil, nil
	}
	chosen := *selection
	if len(input) == 0 {
		return nil, nil, fmt.Errorf("%w: tools required", ErrInvalidClientRequest)
	}
	switch chosen.Mode {
	case "auto", "none", "required":
		if chosen.Name != "" {
			return nil, nil, fmt.Errorf("%w: unexpected tool name", ErrInvalidClientRequest)
		}
	case "function":
		if !names[chosen.Name] {
			return nil, nil, fmt.Errorf("%w: unknown selected function", ErrInvalidClientRequest)
		}
	default:
		return nil, nil, fmt.Errorf("%w: invalid tool choice", ErrInvalidClientRequest)
	}
	return result, &chosen, nil
}
