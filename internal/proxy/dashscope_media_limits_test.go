package proxy_test

import (
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

func TestDashScopeMediaImageCount(t *testing.T) {
	for _, model := range []string{"qwen3.7-plus", "qwen3.6-flash"} {
		t.Run(model, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				content := payload["input"].([]any)[0].(map[string]any)["content"].([]any)
				if len(content) != 251 {
					t.Errorf("provider content count=%d", len(content))
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"accepted"}]}]}`)
			}))
			defer upstream.Close()
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{"dashscope": upstream.URL}, nil)}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			attachments := make([]map[string]any, 251)
			for index := range attachments {
				attachments[index] = messageMediaPayload("image", "image/png", []byte("x"))
			}
			for _, count := range []int{250, 251} {
				response, err := http.Post(server.URL+"/v2?provider=dashscope&key="+TestSecret, "application/json", strings.NewReader(mediaV2RequestBody(t, model, "inspect", attachments[:count])))
				if err != nil {
					t.Fatal(err)
				}
				data, _ := io.ReadAll(response.Body)
				response.Body.Close()
				want := http.StatusOK
				if count == 251 {
					want = http.StatusRequestEntityTooLarge
				}
				if response.StatusCode != want || calls != 1 {
					t.Fatalf("count=%d status=%d calls=%d body=%s", count, response.StatusCode, calls, data)
				}
			}
		})
	}
}

func TestDashScopeMediaDataURIBoundary(t *testing.T) {
	for _, model := range []string{"qwen3.7-plus", "qwen3.6-flash"} {
		t.Run(model, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				content := payload["input"].([]any)[0].(map[string]any)["content"].([]any)
				uri := content[0].(map[string]any)["image_url"].(string)
				if len(uri) != 30 || uri != "data:image/png;base64,YWJjZGVm" {
					t.Errorf("URI=%s size=%d", uri, len(uri))
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"accepted"}]}]}`)
			}))
			defer upstream.Close()
			catalog := testfixtures.ModelCatalog(t)
			setProviderMediaLimit(t, &catalog, "dashscope", model, proxy.CatalogMediaLimitIDImageInlineBytes, 30)
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{ModelCatalog: catalog, Endpoints: providerEndpointOverrides(map[string]string{"dashscope": upstream.URL}, nil)}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			for _, scenario := range []struct {
				mime, data string
				status     int
			}{{"image/png", "abcdef", 200}, {"image/jpeg", "abcdef", 413}, {"image/png", "abcdefg", 413}} {
				response, err := http.Post(server.URL+"/v2?provider=dashscope&key="+TestSecret, "application/json", strings.NewReader(mediaV2RequestBody(t, model, "inspect", []map[string]any{messageMediaPayload("image", scenario.mime, []byte(scenario.data))})))
				if err != nil {
					t.Fatal(err)
				}
				data, _ := io.ReadAll(response.Body)
				response.Body.Close()
				if response.StatusCode != scenario.status || calls != 1 {
					t.Fatalf("mime=%s size=%d status=%d calls=%d body=%s", scenario.mime, len(scenario.data), response.StatusCode, calls, data)
				}
			}
		})
	}
}

func TestDashScopeMediaCatalog(t *testing.T) {
	public, err := proxy.NewPublicCapabilityCatalog(proxy.Configuration{ProviderCatalog: testfixtures.ProviderCatalog(t)})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, offering := range public.Offerings {
		if offering.Provider != "dashscope" || len(offering.MediaLimits) == 0 {
			continue
		}
		count++
		for _, limit := range offering.MediaLimits {
			if limit.ID == proxy.CatalogMediaLimitIDInlineRequestBytes {
				if limit.Status != "unknown" || limit.Value != nil {
					t.Errorf("request limit=%+v", limit)
				}
				continue
			}
			if limit.Source != "https://www.alibabacloud.com/help/en/model-studio/vision" || limit.LastVerified != "2026-09-05" || limit.Status != "bounded" || limit.Value == nil {
				t.Fatalf("source/status=%+v", limit)
			}
			switch limit.ID {
			case proxy.CatalogMediaLimitIDImageCount:
				if *limit.Value != 250 || limit.Scope != "request" || limit.Unit != "files" {
					t.Errorf("count=%+v", limit)
				}
			case proxy.CatalogMediaLimitIDImageInlineBytes:
				if *limit.Value != 20000000 || limit.Scope != "attachment_data_uri_bytes" || limit.Unit != "bytes" {
					t.Errorf("image=%+v", limit)
				}
			default:
				t.Errorf("unexpected limit=%+v", limit)
			}
		}
	}
	if count != 2 {
		t.Fatalf("image offerings=%d", count)
	}
}
