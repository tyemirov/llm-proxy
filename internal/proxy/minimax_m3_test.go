package proxy_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

func miniMaxM3CandidateCatalog(t *testing.T) *proxy.ProviderCatalog {
	t.Helper()
	schema := testfixtures.ProviderCatalog(t).Schema()
	for index := range schema.Models {
		if schema.Models[index].ID == "minimax-m3" {
			if schema.Models[index].Enabled != proxy.ModelDisabled {
				t.Fatal("unqualified M3 must be disabled")
			}
			schema.Models[index].Enabled = proxy.ModelEnabled
		}
	}
	catalog, err := proxy.NewProviderCatalog(schema)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestMiniMaxM3CandidateHTTP(t *testing.T) {
	catalog := miniMaxM3CandidateCatalog(t)
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if r.Method != "POST" || r.URL.Path != "/chat/completions" || body["model"] != "MiniMax-M3" || body["max_completion_tokens"] != float64(524288) || body["reasoning_split"] != true {
			t.Errorf("upstream=%s %s payload=%v", r.Method, r.URL.Path, body)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"visible M3 answer","reasoning_content":"private reasoning"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: catalog, Endpoints: providerEndpointOverrides(map[string]string{"minimax": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	for _, limit := range []string{"524288", "524289"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", "/?key="+TestSecret+"&provider=minimax&model=minimax-m3&prompt=test&max_tokens="+limit, nil))
		if limit == "524288" {
			if response.Code != 200 || response.Body.String() != "visible M3 answer" || response.Header().Get("X-LLM-Proxy-Total-Tokens") != "5" {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		} else if response.Code != 400 {
			t.Fatalf("limit status=%d", response.Code)
		}
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/api/public/capabilities", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"minimax-m3"`) || !strings.Contains(response.Body.String(), `524288`) {
		t.Fatalf("candidate catalog status=%d", response.Code)
	}
}

func TestMiniMaxM3DisabledHTTP(t *testing.T) {
	_ = miniMaxM3CandidateCatalog(t)
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/?key="+TestSecret+"&provider=minimax&model=minimax-m3&prompt=test", nil))
	if response.Code != 400 {
		t.Fatalf("disabled route status=%d", response.Code)
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/api/public/capabilities", nil))
	if strings.Contains(response.Body.String(), `"model":"minimax-m3"`) {
		t.Fatal("disabled candidate is publicly selectable")
	}
}

func TestMiniMaxM3ImagesAndPrices(t *testing.T) {
	catalog := miniMaxM3CandidateCatalog(t)
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["model"] != "MiniMax-M3" || payload["reasoning_split"] != true {
			t.Errorf("image model payload=%v", payload)
		}
		messages := payload["messages"].([]any)
		content := messages[0].(map[string]any)["content"].([]any)
		for index, mimeType := range []string{"image/jpeg", "image/png", "image/webp"} {
			image := content[index].(map[string]any)["image_url"].(map[string]any)
			expected := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString([]byte(mimeType))
			if image["url"] != expected {
				t.Errorf("image order/data=%v", image)
			}
		}
		if content[3].(map[string]any)["text"] != "inspect" {
			t.Errorf("image text=%v", content)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"accepted"},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: catalog, Endpoints: providerEndpointOverrides(map[string]string{"minimax": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"ordered images", "oversized image", "unsupported MIME"} {
		var attachments []map[string]any
		switch scenario {
		case "ordered images":
			for _, mimeType := range []string{"image/jpeg", "image/png", "image/webp"} {
				attachments = append(attachments, map[string]any{"type": "image", "mime_type": mimeType, "data": base64.StdEncoding.EncodeToString([]byte(mimeType))})
			}
		case "oversized image":
			attachments = []map[string]any{{"type": "image", "mime_type": "image/png", "data": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("a"), 10000001))}}
		case "unsupported MIME":
			attachments = []map[string]any{{"type": "image", "mime_type": "image/gif", "data": "YQ=="}}
		}
		body, _ := json.Marshal(map[string]any{"model": "minimax-m3", "messages": []any{map[string]any{"role": "user", "content": "inspect", "attachments": attachments}}})
		request := httptest.NewRequest("POST", "/v2?provider=minimax&key="+TestSecret, bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		expected := 200
		if scenario == "oversized image" {
			expected = 413
		}
		if scenario == "unsupported MIME" {
			expected = 400
		}
		if response.Code != expected {
			t.Fatalf("%s status=%d body=%s", scenario, response.Code, response.Body.String())
		}
	}
	if calls != 1 {
		t.Fatalf("image calls=%d", calls)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/api/public/capabilities", nil))
	var public proxy.PublicCapabilityCatalog
	if err := json.Unmarshal(response.Body.Bytes(), &public); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, price := range public.Prices {
		if price.Model != "minimax-m3" {
			continue
		}
		found = true
		expected := []float64{0.3, 1.2, 0.06, 0.6, 2.4, 0.12}
		if len(price.Rates) != len(expected) {
			t.Fatalf("price rates=%v", price.Rates)
		}
		for index, rate := range price.Rates {
			tier := "input_tokens_up_to_512k"
			if index >= 3 {
				tier = "input_tokens_above_512k"
			}
			if rate.Rate != expected[index] || rate.Conditions.Mode != tier || rate.Conditions.BillingMode != "pay_as_you_go_standard" {
				t.Fatalf("price rate=%v", rate)
			}
		}
	}
	if !found {
		t.Fatal("M3 candidate price absent")
	}
}

func TestMiniMaxM3ManagedVerification(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if r.URL.Path != "/chat/completions" || payload["model"] != "MiniMax-M3" || payload["reasoning_split"] != true || payload["max_completion_tokens"] != float64(16) {
			t.Errorf("verification payload=%v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"verified"},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	config := providerKeyVerificationConfiguration(upstream.URL)
	config.ProviderCatalog = miniMaxM3CandidateCatalog(t)
	router := newOperationalProviderKeyVerificationRouter(t, config, zap.NewNop().Sugar(), t.TempDir()+"/managed.db", TestTimeout)
	cookie := managementSessionCookie(t, "minimax-m3-verification")
	tenant := managementDefaultTenantTestID(t, router, cookie)
	response := putManagementProviderKey(t, router, cookie, tenant, "minimax", "candidate-minimax-m3-key", "minimax-m3", "", context.Background())
	if response.Code != 200 || calls != 1 {
		t.Fatalf("verification status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
	}
	profile := decodeProviderKeyVerificationProfile(t, response.Body.Bytes())
	if verificationProfileProvider(t, profile, "minimax").TextModel != "minimax-m3" {
		t.Fatal("verified M3 selection not saved")
	}
}
