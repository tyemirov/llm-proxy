package proxy_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestMiniMaxReasoningSeparation(t *testing.T) {
	for _, model := range []string{"minimax-m2.7", "minimax-m2.7-highspeed", "minimax-m2.5", "minimax-m2.5-highspeed", "minimax-m2.1", "minimax-m2.1-highspeed", "minimax-m2"} {
		t.Run(model, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if r.URL.Path != "/chat/completions" || r.Method != "POST" {
					t.Errorf("upstream=%s %s", r.Method, r.URL.Path)
				}
				if _, found := body["thinking"]; found {
					t.Errorf("unexpected thinking control")
				}
				content := "visible "
				finish := "length"
				if calls == 2 {
					content = "suffix"
					finish = "stop"
				}
				if body["reasoning_split"] != true {
					content = "<think>private reasoning</think>" + content
				}
				if calls == 2 {
					messages := body["messages"].([]any)
					for _, raw := range messages {
						message := raw.(map[string]any)
						if strings.Contains(fmt.Sprint(message["content"]), "private reasoning") {
							t.Error("reasoning entered visible continuation history")
						}
					}
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content, "reasoning_content": "private reasoning", "reasoning_details": []any{map[string]any{"type": "text", "text": "private reasoning"}}}, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": 2, "completion_tokens": 3, "total_tokens": 5}})
			}))
			defer upstream.Close()
			core, observed := observer.New(zap.DebugLevel)
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{"minimax": upstream.URL}, nil)}, zap.New(core).Sugar())
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			response, err := http.Get(server.URL + "/?key=" + TestSecret + "&provider=minimax&model=" + model + "&prompt=test")
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, _ := io.ReadAll(response.Body)
			if response.StatusCode != 200 || string(body) != "visible suffix" || calls != 2 || response.Header.Get("X-LLM-Proxy-Total-Tokens") != "10" {
				t.Fatalf("status=%d calls=%d body=%s usage=%s", response.StatusCode, calls, body, response.Header.Get("X-LLM-Proxy-Total-Tokens"))
			}
			for _, entry := range observed.All() {
				if strings.Contains(entry.Message+fmt.Sprint(entry.ContextMap()), "private reasoning") {
					t.Fatal("reasoning entered logs")
				}
			}
		})
	}
}

func TestMiniMaxReasoningProfileRejectsWrongProtocol(t *testing.T) {
	schema := testfixtures.ProviderCatalog(t).Schema()
	for p := range schema.Providers {
		if schema.Providers[p].ID == "openai" {
			schema.Providers[p].Offerings[0].RequestProfile = "minimax_chat_completions"
		}
	}
	_, err := proxy.NewProviderCatalog(schema)
	if err == nil || !strings.Contains(err.Error(), "profile=minimax_chat_completions") {
		t.Fatalf("invalid profile error=%v", err)
	}
}
