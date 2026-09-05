package llmproxyclient_test

import (
	"encoding/json"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
)

func TestMessagesRequestToolContract(t *testing.T) {
	user := llmproxyclient.MessageInput{Role: "user", Content: "read"}
	call := llmproxyclient.FunctionCall{ID: "a", Name: "read", Arguments: "{}"}
	assistant := llmproxyclient.MessageInput{Role: "assistant", ToolCalls: []llmproxyclient.FunctionCall{call}}
	output := llmproxyclient.MessageInput{Role: "tool", ToolCallID: "a", Content: ""}
	strict := true
	parallel := false
	tool := llmproxyclient.FunctionDeclaration{Name: "read", Parameters: json.RawMessage(`{"type":"object"}`), Strict: &strict}
	for _, test := range []struct {
		name  string
		input llmproxyclient.MessagesRequestInput
	}{
		{"bad function", llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{user}, Tools: []llmproxyclient.FunctionDeclaration{{Name: "bad name"}}}},
		{"parallel without tools", llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{user}, ParallelToolCalls: &parallel}},
		{"choice without tools", llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{user}, ToolChoice: &llmproxyclient.ToolChoice{Mode: "required"}}},
		{"name on auto", llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{user}, Tools: []llmproxyclient.FunctionDeclaration{tool}, ToolChoice: &llmproxyclient.ToolChoice{Mode: "auto", Name: "read"}}},
		{"unknown choice", llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{user}, Tools: []llmproxyclient.FunctionDeclaration{tool}, ToolChoice: &llmproxyclient.ToolChoice{Mode: "function", Name: "other"}}},
		{"bad choice", llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{user}, Tools: []llmproxyclient.FunctionDeclaration{tool}, ToolChoice: &llmproxyclient.ToolChoice{Mode: "invalid"}}},
		{"unmatched result", llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{user, output}}},
		{"missing result", llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{user, assistant}}},
		{"missing intermediate result", llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{user, assistant, user}}},
		{"wrong role", llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "read", ToolCalls: []llmproxyclient.FunctionCall{call}}}}},
		{"malformed arguments", llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{user, {Role: "assistant", ToolCalls: []llmproxyclient.FunctionCall{{ID: "a", Name: "read", Arguments: "[]"}}}, output}}},
		{"duplicate identifier", llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{user, assistant, output, assistant, output}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := llmproxyclient.NewMessagesRequest(test.input); err == nil {
				t.Fatal("invalid tool contract accepted")
			}
		})
	}
	for _, mode := range []string{"auto", "none", "required", "function"} {
		choice := &llmproxyclient.ToolChoice{Mode: mode}
		if mode == "function" {
			choice.Name = "read"
		}
		request, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{user, assistant, output}, Tools: []llmproxyclient.FunctionDeclaration{tool}, ToolChoice: choice, ParallelToolCalls: &parallel})
		if err != nil {
			t.Fatal(err)
		}
		_ = request
	}
}
