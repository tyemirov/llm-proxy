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
	response, err := server.Client().Post(server.URL+"/v2?key="+TestSecret+"&provider=openai", "application/json", strings.NewReader(`{"model":"gpt-5.6","messages":[{"role":"user","content":"private prompt"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
	if got := response.Header.Get("X-LLM-Proxy-Resolved-Model"); got != "gpt-5.6" {
		t.Fatalf("resolved model=%q", got)
	}
	if got := response.Header.Get("X-LLM-Proxy-Request-Tokens"); got != "10" {
		t.Fatalf("input tokens=%q", got)
	}
	config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: TestSecret, Provider: "openai"})
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
		if result.ResolvedModel() != "gpt-5.6" || result.Usage() == nil || result.Usage().InputTokens() != 10 || result.Usage().OutputTokens() != 3 {
			t.Fatalf("completion=%+v usage=%+v", result, result.Usage())
		}
	}

}

func fmtPayload(payload map[string]any) string { data, _ := json.Marshal(payload); return string(data) }
