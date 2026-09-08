package llmproxyclient_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
)

func TestMessagesRequestCompletion(t *testing.T) {
	for _, test := range []struct {
		name                 string
		status               int
		tokens               []string
		wantUsage, wantError bool
	}{
		{"measured", 200, []string{"10", "3", "13"}, true, false},
		{"truncated response", 200, []string{"10", "3", "13"}, true, true},
		{"measured zero", 200, []string{"0", "0", "0"}, true, false},
		{"unavailable", 200, nil, false, false},
		{"failed with usage", 502, []string{"10", "3", "13"}, true, true},
		{"failed without usage", 502, nil, false, true},
		{"missing model", 200, nil, false, true},
		{"duplicate", 200, []string{"10", "3", "13"}, false, true},
		{"partial", 200, []string{"10"}, false, true},
		{"negative", 200, []string{"-1", "3", "2"}, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("format") != "text/plain" || r.Header.Get("Accept") != "text/plain" {
					t.Error("wrong representation")
				}
				if test.name != "missing model" {
					w.Header().Set("X-LLM-Proxy-Resolved-Model", "exact-model")
				}
				names := []string{"X-LLM-Proxy-Request-Tokens", "X-LLM-Proxy-Response-Tokens", "X-LLM-Proxy-Total-Tokens"}
				for index, value := range test.tokens {
					w.Header().Set(names[index], value)
				}
				if test.name == "duplicate" {
					w.Header().Add("X-LLM-Proxy-Request-Tokens", "10")
				}
				if test.name == "truncated response" {
					w.Header().Set("Content-Length", "100")
				}
				w.WriteHeader(test.status)
				io.WriteString(w, "private response")
			}))
			defer server.Close()
			config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "secret", Provider: "openai"})
			if err != nil {
				t.Fatal(err)
			}
			client, err := llmproxyclient.NewClient(config, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			request, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Model: "requested-alias", Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "private prompt"}}})
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.PostMessagesCompletion(context.Background(), request)
			if (err != nil) != test.wantError {
				t.Fatalf("error=%v", err)
			}
			if err != nil && !errors.Is(err, llmproxyclient.ErrClientHTTPFailure) {
				t.Fatalf("untyped error=%v", err)
			}
			if (result.Usage() != nil) != test.wantUsage {
				t.Fatalf("usage=%v", result.Usage())
			}
			if test.wantUsage && result.Usage().InputTokens()+result.Usage().OutputTokens() != result.Usage().TotalTokens() {
				t.Fatal("token counts changed")
			}
			if test.status != 200 && result.Text() != "" {
				t.Fatal("error body exposed as completion")
			}
			if !test.wantError && (result.Text() != "private response" || result.ResolvedModel() != "exact-model") {
				t.Fatalf("completion=%+v", result)
			}
		})
	}
}

func TestMessagesRequestCompletionRejectsUnsupportedInputs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid request reached server") }))
	defer server.Close()
	config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "secret", ModelProfilePath: filepath.Join(t.TempDir(), "missing.json"), ModelProfileReader: os.ReadFile})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	request, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "hello"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.PostMessagesCompletion(context.Background(), request); !errors.Is(err, llmproxyclient.ErrInvalidModelProfile) {
		t.Fatalf("profile error=%v", err)
	}
	request, err = llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "hello"}}, StructuredOutput: &llmproxyclient.StructuredOutputInput{JSONSchema: []byte(`{"type":"object","properties":{"decision":{"type":"string"}},"required":["decision"],"additionalProperties":false}`)}, IdempotencyKey: "measurement-test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.PostMessagesCompletion(context.Background(), request); !errors.Is(err, llmproxyclient.ErrInvalidClientRequest) {
		t.Fatalf("structured error=%v", err)
	}
}
