package proxy_test

import (
	"context"
	"encoding/json"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

func TestClientProtocolsCompletionMeasurements(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if strings.Contains(fmtPayload(payload), "fail request") {
			io.WriteString(w, `{"id":"private-id","status":"failed","usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}}`)
			return
		}
		io.WriteString(w, `{"id":"private-id","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"measured answer"}]}],"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameOpenAI: upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{Protocol: llmproxyclient.ProtocolOpenAIResponses, BaseURL: server.URL, Secret: TestSecret, Provider: "openai"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	for _, prompt := range []string{"success request", "fail request"} {
		request, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Model: "gpt-5.6", Messages: []llmproxyclient.MessageInput{{Role: "user", Content: prompt}}})
		if err != nil {
			t.Fatal(err)
		}
		result, err := client.PostMessagesCompletion(context.Background(), request)
		if (err != nil) != (prompt == "fail request") {
			t.Fatalf("prompt=%s error=%v", prompt, err)
		}
		if prompt == "success request" && (result.ResolvedModel() != "openai/gpt-5.6" || result.Usage() == nil || result.Usage().InputTokens() != 10 || result.Usage().OutputTokens() != 3) {
			t.Fatalf("completion=%+v usage=%+v", result, result.Usage())
		}
		if prompt == "fail request" && (result.Usage() != nil || result.ResolvedModel() != "") {
			t.Fatal("failed call invented measurements")
		}
	}

}

func fmtPayload(payload map[string]any) string { data, _ := json.Marshal(payload); return string(data) }
