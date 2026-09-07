package proxy_test

import (
	"encoding/json"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

func TestDeepSeekRetirementHTTP(t *testing.T) {
	requests := make(chan map[string]any, 16)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		requests <- payload
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"qualified result","reasoning_content":"private reasoning"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameDeepSeek: upstream.URL, proxy.ProviderNameSiliconFlow: upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"deepseek-v4-flash", "deepseek-v4-pro"} {
		for _, effort := range []string{"", "none", "low", "high", "max", "medium", "xhigh"} {
			t.Run(model+"/"+effort, func(t *testing.T) {
				path := "/?key=" + TestSecret + "&provider=deepseek&model=" + model + "&prompt=test"
				if effort != "" {
					path += "&reasoning_effort=" + url.QueryEscape(effort)
				}
				request := httptest.NewRequest(http.MethodGet, path, nil)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if effort == "medium" || effort == "xhigh" {
					if response.Code != 400 || len(requests) != 0 {
						t.Fatalf("unsupported effort status=%d calls=%d", response.Code, len(requests))
					}
					return
				}
				if response.Code != 200 || response.Body.String() != "qualified result" {
					t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
				}
				payload := <-requests
				expectedType := "enabled"
				expectedEffort := effort
				if effort == "none" {
					expectedType = "disabled"
					expectedEffort = ""
				}
				thinking, ok := payload["thinking"].(map[string]any)
				if !ok || thinking["type"] != expectedType {
					t.Fatalf("thinking=%v expected=%s", payload["thinking"], expectedType)
				}
				actualEffort, _ := payload["reasoning_effort"].(string)
				if actualEffort != expectedEffort || payload["model"] != model {
					t.Fatalf("payload=%v", payload)
				}
			})
		}
	}
	for _, model := range []string{"deepseek-chat", "deepseek-reasoner"} {
		t.Run("retired/"+model, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&provider=deepseek&model="+model+"&prompt=test", nil))
			if response.Code != 400 || len(requests) != 0 {
				count := len(requests)
				for len(requests) > 0 {
					<-requests
				}
				t.Fatalf("retired route status=%d upstream calls=%d", response.Code, count)
			}
		})
	}
	t.Run("SiliconFlow R1 remains available", func(t *testing.T) {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&provider=siliconflow&model=deepseek-reasoner&prompt=test", nil))
		if response.Code != 200 {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		payload := <-requests
		if payload["model"] != "deepseek-ai/DeepSeek-R1" || payload["thinking"] != nil {
			t.Fatalf("SiliconFlow payload=%v", payload)
		}
	})
}

func TestDeepSeekRetirementMigrationControlsRequireSupportedTextTarget(t *testing.T) {
	for _, scenario := range []string{"unsupported effort", "dictation target", "absent target"} {
		t.Run(scenario, func(t *testing.T) {
			schema := testfixtures.ProviderCatalog(t).Schema()
			migrationIndex := slices.IndexFunc(schema.ModelMigrations, func(migration proxy.ProviderCatalogModelMigration) bool {
				return migration.Provider == proxy.ProviderNameDeepSeek && migration.SourceModel == "deepseek-reasoner"
			})
			migration := &schema.ModelMigrations[migrationIndex]
			switch scenario {
			case "unsupported effort":
				migration.TargetReasoningEffort = "medium"
			case "dictation target":
				migration.Operation = proxy.ModelOperationDictation
			case "absent target":
				migration.TargetModel = ""
			}
			if _, err := proxy.NewProviderCatalog(schema); err == nil || !strings.Contains(err.Error(), "target_reasoning_effort") {
				t.Fatalf("catalog accepted invalid control: %v", err)
			}
		})
	}
}
