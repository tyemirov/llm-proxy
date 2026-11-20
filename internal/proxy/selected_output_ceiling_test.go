package proxy_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

func TestSelectedOutputCeilingsFollowCatalog(t *testing.T) {
	for _, route := range []struct {
		provider, model string
		maximum         int
	}{{"cerebras", "qwen3.8-27b", 40960}, {"dashscope", "qwen3.8-27b", 131072}, {"dashscope", "qwen3.8-flash", 65536}, {"openai", "gpt-6-luna", 128000}, {"anthropic", "claude-haiku-5-5", 128000}, {"gemini", "gemini-3.6-flash", 65536}} {
		t.Run(route.provider+"/"+route.model, func(t *testing.T) {
			var maxima []int
			incomplete := false
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodDelete {
					io.WriteString(w, `{}`)
					return
				}
				if r.Method == http.MethodPost {
					var payload map[string]any
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
						return
					}
					field := "max_output_tokens"
					values := payload
					switch route.provider {
					case "cerebras":
						field = "max_completion_tokens"
					case "anthropic":
						field = "max_tokens"
					case "gemini":
						values = payload["generation_config"].(map[string]any)
					}
					maximum, _ := values[field].(float64)
					maxima = append(maxima, int(maximum))
				}
				firstIncomplete := incomplete && len(maxima) == 1
				switch route.provider {
				case "cerebras":
					text, finish := "OK", "stop"
					if firstIncomplete {
						text = ""
						finish = "length"
					}
					fmt.Fprintf(w, `{"choices":[{"message":{"content":%q},"finish_reason":%q}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, text, finish)
				case "anthropic":
					text, finish := "OK", "end_turn"
					if firstIncomplete {
						text = ""
						finish = "max_tokens"
					}
					fmt.Fprintf(w, `{"content":[{"type":"text","text":%q}],"stop_reason":%q,"usage":{"input_tokens":1,"output_tokens":1}}`, text, finish)
				case "gemini":
					if firstIncomplete {
						writeGeminiInteractionSnapshot(t, w, "cap-probe", "incomplete", "", nil)
						return
					}
					writeGeminiInteractionSnapshot(t, w, "cap-probe", "completed", "OK", &testGeminiInteractionUsage{Input: 1, Output: 1, Total: 2})
				default:
					if firstIncomplete {
						io.WriteString(w, `{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
					} else {
						io.WriteString(w, `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
					}
				}
			}))
			defer upstream.Close()
			catalog := testfixtures.ProviderCatalog(t)
			tenant := proxy.StandardManagedTenantTestConfiguration(TestSecret)
			tenant.Defaults = proxy.TenantDefaults{Provider: route.provider, Model: route.model}
			tenant.ProviderKeys = map[string]string{route.provider: "provider-fixture"}
			tenant.ProviderTextModels = map[string]string{route.provider: route.model}
			router, err := buildRouterWithManagedTenant(t, proxy.Configuration{ProviderCatalog: catalog, Endpoints: providerEndpointOverrides(map[string]string{route.provider: upstream.URL}, nil)}, zap.NewNop().Sugar(), tenant)
			if err != nil {
				t.Fatal(err)
			}
			discovery := httptest.NewRecorder()
			router.ServeHTTP(discovery, httptest.NewRequest(http.MethodGet, proxy.PublicCapabilitiesPath, nil))
			var public proxy.PublicCapabilityCatalog
			if err := json.Unmarshal(discovery.Body.Bytes(), &public); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, offering := range public.Offerings {
				if offering.Provider == route.provider && offering.Model == route.model {
					found = true
					controlOK := false
					for _, control := range offering.Controls {
						if control.ID == "max_tokens" && control.Maximum != nil && *control.Maximum == float64(route.maximum) {
							controlOK = true
						}
					}
					if !controlOK {
						t.Error("discovery max_tokens maximum must equal the offering ceiling")
					}
				}
			}
			if !found {
				t.Fatal("offering missing from discovery")
			}
			for _, provider := range catalog.Schema().Providers {
				if provider.ID == route.provider {
					for _, offering := range provider.Offerings {
						if offering.Model == route.model && offering.OutputTokenLimit != route.maximum {
							t.Errorf("output_token_limit=%d want offering ceiling", offering.OutputTokenLimit)
						}
					}
				}
			}
			request := func(maximum int) int {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?key=%s&provider=%s&model=%s&prompt=test&reasoning_effort=medium&max_tokens=%d", TestSecret, route.provider, route.model, maximum), nil))
				return response.Code
			}
			if status := request(route.maximum); status != 200 || !slices.Equal(maxima, []int{route.maximum}) {
				t.Fatalf("catalog ceiling status=%d native_maxima=%v", status, maxima)
			}
			if status := request(route.maximum + 1); status != 400 || len(maxima) != 1 {
				t.Fatalf("above catalog ceiling status=%d native_maxima=%v", status, maxima)
			}
			if route.provider == "gemini" {
				maxima = nil
				incomplete = true
				if status := request(route.maximum); status != 502 || !slices.Equal(maxima, []int{route.maximum}) {
					t.Fatalf("terminal incomplete status=%d native_maxima=%v", status, maxima)
				}
			}
			if route.provider != "gemini" {
				maxima = nil
				incomplete = true
				if status := request(40000); status != 200 || !slices.Equal(maxima, []int{40000, min(80000, route.maximum)}) {
					t.Fatalf("empty continuation status=%d native_maxima=%v", status, maxima)
				}
			}
		})
	}
}
