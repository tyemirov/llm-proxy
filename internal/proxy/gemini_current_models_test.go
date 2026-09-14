package proxy_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
)

var currentGeminiModels = []string{"gemini-3.8-flash", "gemini-3.5-flash-lite"}

func currentGeminiEfforts(model string) []string {
	if model == "gemini-3.5-flash-lite" || model == "gemini-3.6-flash" {
		return []string{"minimal", "low", "medium", "high"}
	}
	return []string{"low", "medium", "high"}
}

func currentGeminiCandidateCatalog(t *testing.T) *proxy.ProviderCatalog {
	t.Helper()
	schema := testfixtures.ProviderCatalog(t).Schema()
	for index := range schema.Models {
		if slices.Contains(currentGeminiModels, schema.Models[index].ID) {
			schema.Models[index].Enabled = proxy.ModelEnabled
		}
	}
	for p := range schema.Providers {
		for o := range schema.Providers[p].Offerings {
			if slices.Contains(currentGeminiModels, schema.Providers[p].Offerings[o].Model) {
				schema.Providers[p].Offerings[o].Enabled = proxy.ModelEnabled
			}
		}
	}
	catalog, err := proxy.NewProviderCatalog(schema)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestGeminiCurrentModelsHTTP(t *testing.T) {
	testGeminiModelsHTTP(t, currentGeminiCandidateCatalog(t), currentGeminiModels)
}

func TestGeminiCurrentModelsSynchronousFailure(t *testing.T) {
	for _, status := range []string{"incomplete", "in_progress", "failed"} {
		t.Run(status, func(t *testing.T) {
			var methods []string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				methods = append(methods, r.Method)
				writeGeminiInteractionSnapshot(t, w, "", status, "private partial answer", nil)
			}))
			defer upstream.Close()
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: currentGeminiCandidateCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"gemini": upstream.URL}, nil)}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&provider=gemini&model=gemini-3.5-flash-lite&prompt=test", nil))
			if response.Code != http.StatusBadGateway || strings.Contains(response.Body.String(), "private partial answer") || !reflect.DeepEqual(methods, []string{http.MethodPost}) {
				t.Fatalf("status=%d methods=%v body=%s", response.Code, methods, response.Body)
			}
		})
	}
}

func TestGeminiCurrentModelsSynchronousKeyVerification(t *testing.T) {
	for _, candidate := range []struct {
		status         string
		upstreamStatus int
		want           int
	}{
		{"completed", http.StatusOK, http.StatusOK},
		{"incomplete", http.StatusOK, http.StatusOK},
		{"in_progress", http.StatusOK, http.StatusServiceUnavailable},
		{"failed", http.StatusOK, http.StatusServiceUnavailable},
		{"rejected", http.StatusBadRequest, http.StatusUnprocessableEntity},
		{"quota", http.StatusTooManyRequests, http.StatusTooManyRequests},
	} {
		t.Run(candidate.status, func(t *testing.T) {
			var methods []string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				methods = append(methods, r.Method)
				body := decodeGeminiInteractionRequest(t, r)
				if body["model"] != "gemini-3.5-flash-lite" || body["background"] != false || body["store"] != false {
					t.Errorf("synchronous verification request=%v", body)
				}
				if candidate.upstreamStatus != http.StatusOK {
					w.WriteHeader(candidate.upstreamStatus)
					_, _ = io.WriteString(w, `{"error":{"message":"private provider rejection"}}`)
					return
				}
				writeGeminiInteractionSnapshot(t, w, "", candidate.status, "verified", nil)
			}))
			defer upstream.Close()
			config := providerKeyVerificationConfiguration(upstream.URL)
			schema := currentGeminiCandidateCatalog(t).Schema()
			for index := range schema.Providers {
				if schema.Providers[index].ID != "gemini" {
					continue
				}
				for offeringIndex := range schema.Providers[index].Offerings {
					offering := &schema.Providers[index].Offerings[offeringIndex]
					offering.DefaultOperations = slices.DeleteFunc(offering.DefaultOperations, func(operation string) bool { return operation == proxy.ModelOperationText })
					if offering.Model == "gemini-3.5-flash-lite" {
						offering.DefaultOperations = append(offering.DefaultOperations, proxy.ModelOperationText)
					}
				}
			}
			var catalogError error
			config.ProviderCatalog, catalogError = proxy.NewProviderCatalog(schema)
			if catalogError != nil {
				t.Fatal(catalogError)
			}
			router := newOperationalProviderKeyVerificationRouter(t, config, zap.NewNop().Sugar(), t.TempDir()+"/managed.db", TestTimeout)
			cookie := managementSessionCookie(t, "gemini-lite-verification")
			tenantID := managementDefaultTenantTestID(t, router, cookie)
			response := putManagementProviderKey(t, router, cookie, tenantID, "gemini", "candidate-test-key", "gemini-3.5-flash-lite", "", context.Background())
			if response.Code != candidate.want || strings.Contains(response.Body.String(), "private provider rejection") || !reflect.DeepEqual(methods, []string{http.MethodPost}) {
				t.Fatalf("status=%d methods=%v body=%s", response.Code, methods, response.Body)
			}
		})
	}
}

func testGeminiModelsHTTP(t *testing.T, catalog *proxy.ProviderCatalog, models []string) {
	t.Helper()
	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			var expectedEffort string
			var observed []string
			pollable := model != "gemini-3.5-flash-lite"
			expectedMethods := []string{"POST"}
			if pollable {
				expectedMethods = []string{"POST", "GET", "DELETE"}
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				observed = append(observed, r.Method)
				switch r.Method {
				case http.MethodPost:
					body := decodeGeminiInteractionRequest(t, r)
					config, _ := body["generation_config"].(map[string]any)
					effort, _ := config["thinking_level"].(string)
					if r.URL.Path != testGeminiInteractionsPath || body["model"] != model || body["background"] != pollable || body["store"] != pollable || effort != expectedEffort || config["max_output_tokens"] != float64(65536) {
						t.Errorf("Gemini request=%v", body)
					}
					if config["thinking_budget"] != nil {
						t.Error("Gemini request contains an obsolete thinking budget")
					}
					if pollable {
						writeGeminiInteractionSnapshot(t, w, "gemini-current", "in_progress", "", nil)
					} else {
						writeGeminiInteractionSnapshot(t, w, "", "completed", "qualified result", &testGeminiInteractionUsage{Input: 2, Output: 3, Total: 5})
					}
				case http.MethodGet:
					writeGeminiInteractionSnapshot(t, w, "gemini-current", "completed", "qualified result", &testGeminiInteractionUsage{Input: 2, Output: 3, Total: 5})
				case http.MethodDelete:
					writeGeminiInteractionDeleted(t, w)
				default:
					t.Errorf("unexpected method=%s", r.Method)
				}
			}))
			defer upstream.Close()
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: catalog, Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameGemini: upstream.URL}, nil)}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			for _, effort := range append([]string{""}, currentGeminiEfforts(model)...) {
				expectedEffort = effort
				observed = nil
				path := "/?key=" + TestSecret + "&provider=gemini&model=" + model + "&prompt=test&max_tokens=65536"
				if effort != "" {
					path += "&reasoning_effort=" + effort
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
				if response.Code != http.StatusOK || response.Body.String() != "qualified result" || !reflect.DeepEqual(observed, expectedMethods) {
					t.Fatalf("effort=%s status=%d body=%s lifecycle=%v", effort, response.Code, response.Body.String(), observed)
				}
			}
			invalidValues := []string{"reasoning_effort=", "reasoning_effort=other", "reasoning_effort=none", "reasoning_effort=xhigh", "reasoning_effort=max", "max_tokens=65537"}
			if model == "gemini-3.8-flash" || model == "gemini-3.7-flash" {
				invalidValues = append(invalidValues, "reasoning_effort=minimal")
			}
			for _, invalid := range invalidValues {
				observed = nil
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&provider=gemini&model="+model+"&prompt=test&"+invalid, nil))
				if response.Code != http.StatusBadRequest || len(observed) != 0 {
					t.Fatalf("invalid=%s status=%d lifecycle=%v", invalid, response.Code, observed)
				}
			}
		})
	}
}

func TestGeminiCurrentModelsMediaAndStructuredOutput(t *testing.T) {
	testGeminiModelsMediaAndStructuredOutput(t, currentGeminiCandidateCatalog(t), currentGeminiModels)
}

func testGeminiModelsMediaAndStructuredOutput(t *testing.T, catalog *proxy.ProviderCatalog, models []string) {
	t.Helper()
	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			calls := 0
			mimes := []string{"image/jpeg", "image/png", "image/webp", "audio/wav"}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				body := decodeGeminiInteractionRequest(t, r)
				config := body["generation_config"].(map[string]any)
				format := body["response_format"].([]any)[0].(map[string]any)
				if body["model"] != model || body["background"] != false || body["store"] != false || config["thinking_level"] != "high" || format["mime_type"] != "application/json" || format["schema"] == nil {
					t.Errorf("Gemini media/structured request=%v", body)
				}
				content := body["input"].([]any)[0].(map[string]any)["content"].([]any)
				var observedMimes []string
				for _, raw := range content {
					item := raw.(map[string]any)
					if mime, ok := item["mime_type"].(string); ok {
						observedMimes = append(observedMimes, mime)
						if item["data"] != base64.StdEncoding.EncodeToString([]byte(mime)) {
							t.Errorf("Gemini media bytes=%v", item)
						}
					}
				}
				if !reflect.DeepEqual(observedMimes, mimes) {
					t.Errorf("Gemini media order=%v", observedMimes)
				}
				writeGeminiInteractionSnapshot(t, w, "gemini-media", "completed", `{"answer":"yes"}`, &testGeminiInteractionUsage{Input: 2, Output: 3, Total: 5})
			}))
			defer upstream.Close()
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: catalog, Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameGemini: upstream.URL}, nil), AssetStorePath: t.TempDir()}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			var attachments []any
			for _, mime := range mimes {
				kind := strings.Split(mime, "/")[0]
				attachments = append(attachments, map[string]string{"type": kind, "mime_type": mime, "data": base64.StdEncoding.EncodeToString([]byte(mime))})
			}
			body, _ := json.Marshal(map[string]any{
				"model": model, "reasoning_effort": "high", "messages": []any{map[string]any{"role": "user", "content": "inspect", "attachments": attachments}},
				"structured_output": map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{"answer": map[string]string{"type": "string"}}, "required": []string{"answer"}, "additionalProperties": false}},
			})
			response := performStructuredRequestForProvider(t, router, string(body), "gemini-current:"+model, "gemini")
			if response.Code != http.StatusOK || response.Body.String() != `{"answer":"yes"}` || calls != 1 {
				t.Fatalf("Gemini media status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
			}
			body, _ = json.Marshal(map[string]any{"model": model, "messages": []any{map[string]string{"role": "assistant", "content": "prefill"}}})
			request := httptest.NewRequest(http.MethodPost, "/v2?provider=gemini&key="+TestSecret, bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response = httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || calls != 1 {
				t.Fatalf("Gemini assistant history status=%d calls=%d", response.Code, calls)
			}
		})
	}
}

func TestGeminiCurrentModelsCatalog(t *testing.T) {
	catalog := currentGeminiCandidateCatalog(t)
	public, err := proxy.NewPublicCapabilityCatalog(proxy.Configuration{ProviderCatalog: catalog})
	if err != nil {
		t.Fatal(err)
	}
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: catalog}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: TestSecret})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := client.GetPublicCapabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range currentGeminiModels {
		index := slices.IndexFunc(decoded.Offerings, func(offering llmproxyclient.PublicProviderOffering) bool {
			return offering.Provider == "gemini" && offering.Model == model
		})
		lifecycle := "pollable_resource"
		if model == "gemini-3.5-flash-lite" {
			lifecycle = "synchronous_completion"
		}
		if index < 0 || decoded.Offerings[index].ExecutionLifecycle != lifecycle || decoded.Offerings[index].MediaExecutionLifecycle != "synchronous_completion" {
			t.Fatalf("candidate discovery lifecycle: model=%s offerings=%+v", model, decoded.Offerings)
		}
	}
	for index, model := range currentGeminiModels {
		found := false
		for _, offering := range public.Offerings {
			if offering.Provider != proxy.ProviderNameGemini || offering.Model != model {
				continue
			}
			found = true
			if offering.OutputTokenLimit != 65536 || !reflect.DeepEqual(offering.ReasoningEfforts, currentGeminiEfforts(model)) || !slices.Contains(offering.Capabilities, "image_input") || !slices.Contains(offering.Capabilities, "audio_input") {
				t.Fatalf("Gemini offering=%+v", offering)
			}
			for id, expected := range map[string]int64{proxy.CatalogMediaLimitIDInlineRequestBytes: 20_000_000} {
				limitIndex := slices.IndexFunc(offering.MediaLimits, func(limit proxy.CatalogMediaLimit) bool { return limit.ID == id })
				if limitIndex < 0 || offering.MediaLimits[limitIndex].Value == nil || *offering.MediaLimits[limitIndex].Value != expected {
					t.Fatalf("missing or incorrect media limit %s", id)
				}
			}
			if len(offering.Limits) != 1 || offering.Limits[0].Value == nil || *offering.Limits[0].Value != 1048576 {
				t.Fatalf("Gemini context=%v", offering.Limits)
			}
		}
		if !found {
			t.Fatalf("missing Gemini offering=%s", model)
		}
		expected := [][]float64{{0.75, 3.75, 0.075, 0.5}, {0.30, 2.50, 0.03, 1}}[index]
		foundPrice := false
		for _, price := range public.Prices {
			if price.Provider != proxy.ProviderNameGemini || price.Model != model {
				continue
			}
			foundPrice = true
			if !price.Available || len(price.Rates) != len(expected) {
				t.Fatalf("Gemini prices=%v", price)
			}
			for rateIndex, rate := range price.Rates {
				if rate.Rate != expected[rateIndex] || rate.Currency != "USD" || rate.Conditions.BillingMode != "standard" {
					t.Fatalf("Gemini rate=%v", rate)
				}
			}
		}
		if !foundPrice {
			t.Fatalf("missing Gemini price=%s", model)
		}
	}
}

func TestGeminiCurrentModelsDisabledDiscovery(t *testing.T) {
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, proxy.PublicCapabilitiesPath, nil))
	var public proxy.PublicCapabilityCatalog
	if err := json.Unmarshal(response.Body.Bytes(), &public); err != nil {
		t.Fatal(err)
	}
	for _, model := range currentGeminiModels {
		vertexFound := false
		for _, offering := range public.Offerings {
			if offering.Model != model {
				continue
			}
			if offering.Provider == "gemini" {
				t.Fatalf("unqualified Developer offering is public: %s", model)
			}
			if offering.Provider == "vertex" {
				vertexFound = true
			}
		}
		if !vertexFound {
			t.Fatalf("qualified Vertex offering is absent: %s", model)
		}
	}
}

func TestGeminiCurrentModelsRequireProviderDefault(t *testing.T) {
	catalog := currentGeminiCandidateCatalog(t).ModelCatalog()
	for index := range catalog.Offerings {
		if catalog.Offerings[index].Provider == proxy.ProviderNameGemini {
			catalog.Offerings[index].DefaultOperations = slices.DeleteFunc(catalog.Offerings[index].DefaultOperations, func(operation string) bool { return operation == proxy.ModelOperationText })
		}
	}
	_, err := proxy.NewCatalogService(catalog)
	if !errors.Is(err, proxy.ErrInvalidModelCatalog) || !strings.Contains(err.Error(), "provider=gemini operation=text default_count=0") {
		t.Fatalf("Gemini candidates must retain a text default: %v", err)
	}
}

func TestGeminiCurrentModelsMediaRequestBound(t *testing.T) {
	models := append([]string{"gemini-3.5-flash", "gemini-3-flash-preview", "gemini-3.6-flash", "gemini-3.7-flash"}, currentGeminiModels...)
	mediaBytes := bytes.Repeat([]byte("m"), 16_000_000)
	for _, model := range models {
		for _, mediaType := range []string{"image", "audio"} {
			t.Run(model+"/"+mediaType, func(t *testing.T) {
				mime := "audio/wav"
				if mediaType == "image" {
					mime = "image/png"
				}
				uploads, deletions, interactions := 0, 0, 0
				var upstream *httptest.Server
				upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodPost && r.URL.Path == "/upload/v1beta/files":
						w.Header().Set("X-Goog-Upload-URL", upstream.URL+"/upload-session")
					case r.Method == http.MethodPost && r.URL.Path == "/upload-session":
						body, _ := io.ReadAll(r.Body)
						if !bytes.Equal(body, mediaBytes) {
							t.Error("provider upload bytes differ")
						}
						uploads++
						_, _ = fmt.Fprintf(w, `{"file":{"name":"files/media-bound","mimeType":"%s","sizeBytes":"%d","sha256Hash":"%s","uri":"%s/files/media-bound","state":"ACTIVE"}}`, mime, len(mediaBytes), mediaSHA256Base64(mediaBytes), upstream.URL)
					case r.Method == http.MethodPost && r.URL.Path == "/interactions":
						interactions++
						payload := decodeGeminiInteractionRequest(t, r)
						content := payload["input"].([]any)[0].(map[string]any)["content"].([]any)[1].(map[string]any)
						if payload["model"] != model || content["uri"] != upstream.URL+"/files/media-bound" || content["data"] != nil || content["mime_type"] != mime {
							t.Error("request above 20 MB encoded size must use the exact Files API URI")
						}
						writeGeminiCompletedResponse(w)
					case r.Method == http.MethodDelete && r.URL.Path == "/files/media-bound":
						deletions++
						w.WriteHeader(http.StatusNoContent)
					default:
						t.Errorf("unexpected provider request %s %s", r.Method, r.URL.Path)
						http.NotFound(w, r)
					}
				}))
				defer upstream.Close()
				router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: currentGeminiCandidateCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameGemini: upstream.URL}, nil), AssetStorePath: t.TempDir(), MaxAssetBytes: 20_000_000}, zap.NewNop().Sugar())
				if err != nil {
					t.Fatal(err)
				}
				server := httptest.NewServer(router)
				defer server.Close()
				assetRequest, err := http.NewRequest(http.MethodPost, server.URL+llmproxycontract.AssetPath, bytes.NewReader(mediaBytes))
				if err != nil {
					t.Fatal(err)
				}
				assetRequest.Header.Set("Authorization", "Bearer "+TestSecret)
				assetRequest.Header.Set("Content-Type", mime)
				assetResponse, err := server.Client().Do(assetRequest)
				if err != nil {
					t.Fatal(err)
				}
				var asset mediaAssetResponse
				err = json.NewDecoder(assetResponse.Body).Decode(&asset)
				assetResponse.Body.Close()
				if assetResponse.StatusCode != http.StatusCreated || err != nil || asset.SizeBytes != int64(len(mediaBytes)) {
					t.Fatalf("asset status=%d error=%v size=%d", assetResponse.StatusCode, err, asset.SizeBytes)
				}
				body, _ := json.Marshal(map[string]any{"model": model, "messages": []any{map[string]any{"role": "user", "content": "inspect", "attachments": []any{map[string]string{"type": mediaType, "mime_type": mime, "asset_id": asset.AssetID}}}}})
				response, err := http.Post(server.URL+"/v2?provider=gemini&key="+TestSecret+"&format=text/plain", "application/json", bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				responseBody, _ := io.ReadAll(response.Body)
				response.Body.Close()
				if response.StatusCode != http.StatusOK || uploads != 1 || deletions != 1 || interactions != 1 {
					t.Fatalf("status=%d uploads=%d deletes=%d interactions=%d body=%s", response.StatusCode, uploads, deletions, interactions, responseBody)
				}
			})
		}
	}
}

func TestGeminiCurrentModelsMediaLimitDiscovery(t *testing.T) {
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: currentGeminiCandidateCatalog(t)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	response, err := http.Get(server.URL + proxy.PublicCapabilitiesPath)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var public proxy.PublicCapabilityCatalog
	if err := json.NewDecoder(response.Body).Decode(&public); err != nil {
		t.Fatal(err)
	}
	models := append([]string{"gemini-3.5-flash", "gemini-3-flash-preview", "gemini-3.6-flash", "gemini-3.7-flash"}, currentGeminiModels...)
	for _, model := range models {
		index := slices.IndexFunc(public.Offerings, func(offering proxy.PublicProviderOffering) bool {
			return offering.Provider == proxy.ProviderNameGemini && offering.Model == model
		})
		if index < 0 {
			t.Fatalf("missing Gemini model %s", model)
		}
		limits := public.Offerings[index].MediaLimits
		index = slices.IndexFunc(limits, func(limit proxy.CatalogMediaLimit) bool {
			return limit.ID == proxy.CatalogMediaLimitIDInlineRequestBytes
		})
		if len(limits) != 5 || index < 0 {
			t.Fatalf("incorrect Gemini media descriptors for %s", model)
		}
		limit := limits[index]
		if limit.MediaType != "all" || limit.Status != "bounded" || limit.Value == nil || *limit.Value != 20_000_000 || limit.Scope != "request_encoded_bytes" || limit.Source != "https://ai.google.dev/gemini-api/docs/image-understanding" || limit.LastVerified != "2026-09-05" {
			t.Fatalf("incorrect media limit for %s: %+v", model, limit)
		}
	}
}
