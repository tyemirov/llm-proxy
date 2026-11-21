package proxy_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

func TestOpenAIGPT56RetirementPublic(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"model":"gpt-6.2-sol"`) {
			t.Errorf("wrong current route: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"completed","output_text":"current route"}`)
	}))
	t.Cleanup(upstream.Close)
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: testfixtures.ProviderCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"openai": upstream.URL}, nil), AssetStorePath: t.TempDir()}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	retired := []string{"gpt-4o-mini", "gpt-4o", "gpt-4.1", "gpt-5-mini", "gpt-5", "gpt-5.5", "gpt-5.5-pro"}
	for _, model := range retired {
		for _, endpoint := range []struct{ path, payload string }{
			{"/v2?key=" + TestSecret + "&provider=openai", `{"model":"` + model + `","messages":[{"role":"user","content":"test"}]}`},
			{"/v1/chat/completions", `{"model":"openai/` + model + `","messages":[{"role":"user","content":"test"}]}`},
			{"/v1/responses", `{"model":"openai/` + model + `","input":"test"}`},
		} {
			request, err := http.NewRequest(http.MethodPost, server.URL+endpoint.path, strings.NewReader(endpoint.payload))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+TestSecret)
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != http.StatusBadRequest || calls.Load() != 0 {
				t.Fatalf("retired=%s path=%s status=%d calls=%d body=%s", model, endpoint.path, response.StatusCode, calls.Load(), body)
			}
		}
	}

	for _, path := range []string{proxy.PublicCapabilitiesPath, "/v1/models"} {
		request, _ := http.NewRequest(http.MethodGet, server.URL+path, nil)
		request.Header.Set("Authorization", "Bearer "+TestSecret)
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("discovery status=%d body=%s", response.StatusCode, body)
		}
		activeModels := map[string]bool{}
		if path == proxy.PublicCapabilitiesPath {
			var catalog proxy.PublicCapabilityCatalog
			if err := json.Unmarshal(body, &catalog); err != nil {
				t.Fatal(err)
			}
			for _, offering := range catalog.Offerings {
				if offering.Provider == "openai" {
					activeModels[offering.Model] = true
				}
			}
		} else {
			var catalog struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			if err := json.Unmarshal(body, &catalog); err != nil {
				t.Fatal(err)
			}
			for _, model := range catalog.Data {
				activeModels[strings.TrimPrefix(model.ID, "openai/")] = true
			}
		}
		for _, model := range retired {
			if activeModels[model] {
				t.Fatalf("discovery retained exact model %s", model)
			}
		}
		for _, model := range []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-6-luna", "gpt-6.2-sol", "gpt-6.2-astra"} {
			if !strings.Contains(string(body), model) {
				t.Fatalf("discovery omitted current %s", model)
			}
		}
	}
	response, err := server.Client().Get(server.URL + "/?key=" + TestSecret + "&prompt=default")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || string(body) != "current route" || calls.Load() != 1 {
		t.Fatalf("new default status=%d calls=%d body=%s", response.StatusCode, calls.Load(), body)
	}
	if proxy.DefaultTenantDefaults().Model != "gpt-6.2-sol" {
		t.Fatal("new tenant uses retired default")
	}
}
