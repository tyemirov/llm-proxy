package proxy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGeminiCurrentModelsVertexOfferingActivation(t *testing.T) {
	schema := testfixtures.ProviderCatalog(t).Schema()
	for i := range schema.Models {
		if schema.Models[i].ID == "gemini-3.8-flash" {
			schema.Models[i].Enabled = proxy.ModelEnabled
		}
	}
	for i := range schema.Providers {
		if schema.Providers[i].ID == "vertex" {
			schema.Providers[i].Enabled = proxy.ModelEnabled
		}
	}
	for p := range schema.Providers {
		for o := range schema.Providers[p].Offerings {
			schema.Providers[p].Offerings[o].Enabled = 0
		}
	}
	document, err := yaml.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(document, &raw); err != nil {
		t.Fatal(err)
	}
	for _, rawProvider := range raw["providers"].([]any) {
		provider := rawProvider.(map[string]any)
		if provider["id"] != "gemini" {
			continue
		}
		for _, rawOffering := range provider["offerings"].([]any) {
			offering := rawOffering.(map[string]any)
			if offering["model"] == "gemini-3.8-flash" {
				offering["enabled"] = false
			}
		}
	}
	document, err = yaml.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := proxy.ParseProviderCatalog(document)
	if err != nil {
		t.Fatal(err)
	}
	vertexFound := false
	for _, offering := range catalog.ModelCatalog().Offerings {
		if offering.Model == "gemini-3.8-flash" {
			if offering.Provider == "gemini" {
				t.Fatal("disabled Developer offering is active")
			}
			if offering.Provider == "vertex" {
				vertexFound = true
			}
		}
	}
	if !vertexFound {
		t.Fatal("enabled Vertex offering is absent")
	}
	invalid := catalog.Schema()
	invalid.Providers[0].Offerings[0].Enabled = proxy.ModelActivation(99)
	if _, err := proxy.NewProviderCatalog(invalid); err == nil {
		t.Fatal("invalid offering activation accepted")
	}
	wrongAdapter := catalog.ModelCatalog()
	for i := range wrongAdapter.Offerings {
		if wrongAdapter.Offerings[i].Provider == "vertex" && wrongAdapter.Offerings[i].ReasoningEffort != nil {
			wrongAdapter.Offerings[i].WireContract = "openai_chat_completions"
			break
		}
	}
	if _, err := proxy.NewCatalogService(wrongAdapter); err == nil || !strings.Contains(err.Error(), "Vertex reasoning adapter") {
		t.Fatalf("wrong Vertex protocol error=%v", err)
	}
}

func TestGeminiCurrentModelsVertexCancellation(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	server, _ := vertexHTTPFixture(t, func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-r.Context().Done():
			close(canceled)
		case <-time.After(5 * time.Second):
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/?key="+TestSecret+"&provider=vertex&prompt=test", nil)
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() {
		response, err := http.DefaultClient.Do(request)
		if response != nil {
			response.Body.Close()
		}
		finished <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("Vertex dispatch did not start")
	}
	cancel()
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("client cancellation did not reach Vertex")
	}
	if err := <-finished; err == nil {
		t.Fatal("canceled client request succeeded")
	}
}

func vertexCatalog(t *testing.T) *proxy.ProviderCatalog {
	t.Helper()
	schema := testfixtures.ProviderCatalog(t).Schema()
	for i := range schema.Providers {
		provider := &schema.Providers[i]
		if provider.ID == "vertex" {
			provider.Enabled = proxy.ModelEnabled
		}
		if provider.ID != "gemini" {
			continue
		}
		for j := range provider.Transports {
			transport := &provider.Transports[j]
			transport.Endpoint.DefaultBaseURL = "https://aiplatform.googleapis.com/v1"
			transport.Endpoint.Path = "/publishers/google/models"
			transport.Components.Authentication.Kind = "header"
			transport.Components.Authentication.Header = "x-goog-api-key"
			transport.Components.Authentication.Prefix = ""
			transport.Headers = nil
			transport.Components.RequestCodec = proxy.ProviderCatalogCodecReference{ID: proxy.CatalogProtocolVertexGenerateContent}
			transport.Components.ResponseCodec = proxy.ProviderCatalogCodecReference{ID: proxy.CatalogProtocolVertexGenerateContent}
			transport.Components.Execution = proxy.ProviderCatalogExecutionReference{ID: "synchronous_completion"}
		}
		for j := range provider.Offerings {
			for _, media := range provider.Offerings[j].MediaInputs {
				found := false
				for _, limit := range provider.Offerings[j].MediaLimits {
					if limit.ID == media+"_inline_bytes" {
						found = true
					}
				}
				if !found {
					value := int64(20000000)
					provider.Offerings[j].MediaLimits = append(provider.Offerings[j].MediaLimits, proxy.CatalogMediaLimit{ID: media + "_inline_bytes", MediaType: media, Transport: "inline", Status: "bounded", Value: &value, Unit: "bytes", Scope: "attachment", Source: "https://cloud.google.com/vertex-ai/generative-ai/docs/multimodal/overview", LastVerified: "2026-09-07"})
				}
			}
			if provider.Offerings[j].ReasoningEffort != nil {
				provider.Offerings[j].ReasoningEffort.Adapter = "vertex_generate_content"
			}
		}
	}
	catalog, err := proxy.NewProviderCatalog(schema)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func vertexHTTPFixture(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	calls := &atomic.Int32{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("x-goog-api-key") != "sk-gemini" || r.Header.Get("Authorization") != "" || r.Header.Get("x-goog-user-project") != "" {
			t.Error("incorrect Vertex API-key authorization")
		}
		handler(w, r)
	}))
	t.Cleanup(upstream.Close)
	config := proxy.Configuration{AssetStorePath: t.TempDir(), ProviderCatalog: vertexCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"gemini": upstream.URL, "vertex": upstream.URL}, map[string]map[string]string{"gemini": {"dictation": upstream.URL + "/publishers/google/models"}})}
	tenantConfig := proxy.StandardManagedTenantTestConfiguration(TestSecret)
	tenantConfig.ProviderKeys["vertex"] = "sk-gemini"
	router, err := proxy.BuildRouterWithManagedTenantForTest(t, config, zap.NewNop().Sugar(), tenantConfig)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server, calls
}

const vertexSuccess = `{"candidates":[{"content":{"parts":[{"text":"private thought","thought":true},{"text":"Vertex OK"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2,"thoughtsTokenCount":4,"totalTokenCount":16}}`

func TestGeminiCurrentModelsVertexContract(t *testing.T) {
	server, calls := vertexHTTPFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/publishers/google/models/gemini-3.5-flash:generateContent") {
			t.Errorf("path=%s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["contents"] == nil || body["input"] != nil || body["background"] != nil {
			t.Error("incorrect native Vertex request")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, vertexSuccess)
	})
	for range 2 {
		response, err := http.Get(server.URL + "/?key=" + TestSecret + "&provider=gemini&prompt=test")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 200 || !strings.Contains(string(body), "Vertex OK") || strings.Contains(string(body), "private thought") {
			t.Fatalf("status=%d body=%s", response.StatusCode, body)
		}
		if response.Header.Get("X-LLM-Proxy-Total-Tokens") != "16" {
			t.Errorf("usage headers=%v", response.Header)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("upstream calls=%d", calls.Load())
	}
}

func TestGeminiCurrentModelsVertexMediaAndSchema(t *testing.T) {
	server, _ := vertexHTTPFixture(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Contents []struct {
				Role  string `json:"role"`
				Parts []struct {
					Text       string `json:"text"`
					InlineData *struct {
						MIME string `json:"mimeType"`
						Data string `json:"data"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"contents"`
			System     any `json:"systemInstruction"`
			Generation struct {
				Thinking struct {
					Level string `json:"thinkingLevel"`
				} `json:"thinkingConfig"`
				Format []any `json:"responseFormat"`
				Max    int   `json:"maxOutputTokens"`
			} `json:"generationConfig"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if len(body.Contents) != 2 || body.Contents[0].Role != "model" || len(body.Contents[1].Parts) != 3 || body.System == nil || body.Generation.Thinking.Level != "HIGH" || body.Generation.Max != 1024 || len(body.Generation.Format) != 1 {
			t.Errorf("body=%+v", body)
		}
		if body.Contents[1].Parts[1].InlineData.MIME != "image/png" || body.Contents[1].Parts[2].InlineData.MIME != "audio/wav" {
			t.Error("media order or MIME mismatch")
		}
		io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"{\"answer\":\"OK\"}"}]},"finishReason":"STOP"}]}`)
	})
	payload := `{"model":"gemini-3.7-flash","reasoning_effort":"high","max_tokens":1024,"messages":[{"role":"system","content":"Be exact"},{"role":"assistant","content":"Earlier answer"},{"role":"user","content":"inspect","attachments":[{"type":"image","mime_type":"image/png","data":"aW1hZ2U="},{"type":"audio","mime_type":"audio/wav","data":"YXVkaW8="}]}],"structured_output":{"schema":{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}}}`
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v2?key="+TestSecret+"&provider=gemini", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "vertex-schema-test")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestGeminiCurrentModelsVertexFailures(t *testing.T) {
	for _, testCase := range []struct {
		name, body   string
		status, want int
	}{
		{"quota", `{"error":"private provider detail"}`, 429, 429},
		{"provider", `{"error":"private provider detail"}`, 403, 502},
		{"malformed", `{`, 200, 502},
		{"missing candidate", `{}`, 200, 502},
		{"blocked", `{"candidates":[{"finishReason":"SAFETY"}]}`, 200, 502},
		{"output limit", `{"candidates":[{"finishReason":"MAX_TOKENS"}]}`, 200, 502},
		{"empty", `{"candidates":[{"finishReason":"STOP"}]}`, 200, 502},
		{"negative thoughts", `{"usageMetadata":{"thoughtsTokenCount":-1}}`, 200, 502},
		{"negative input", `{"usageMetadata":{"promptTokenCount":-1}}`, 200, 502},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server, _ := vertexHTTPFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(testCase.status)
				io.WriteString(w, testCase.body)
			})
			response, err := http.Get(server.URL + "/?key=" + TestSecret + "&provider=gemini&prompt=test")
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, _ := io.ReadAll(response.Body)
			if response.StatusCode != testCase.want || strings.Contains(string(body), "private provider detail") {
				t.Fatalf("status=%d body=%s", response.StatusCode, body)
			}
		})
	}
}

func TestGeminiCurrentModelsVertexShippedRoute(t *testing.T) {
	server, _ := vertexHTTPFixture(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, vertexSuccess) })
	response, err := http.Get(server.URL + "/?key=" + TestSecret + "&provider=vertex&prompt=test")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 || !strings.Contains(string(body), "Vertex OK") {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestGeminiCurrentModelsVertexGemini4(t *testing.T) {
	catalog := testfixtures.ProviderCatalog(t)
	for _, model := range vertexGemini4Models {
		outputLimit := 65536
		highestEffort, rejectedEffort := "xhigh", "minimal"
		if model == vertexProjectCarbonModel {
			outputLimit = 32768
			highestEffort, rejectedEffort = "high", "xhigh"
		}
		for _, scenario := range []struct {
			name       string
			parameters string
			status     int
			effort     string
			maxOutput  int
		}{
			{"omitted", "", http.StatusOK, "", outputLimit},
			{"highest effort", "&reasoning_effort=" + highestEffort + "&max_tokens=128", http.StatusOK, strings.ToUpper(highestEffort), 128},
			{"unsupported effort", "&reasoning_effort=" + rejectedEffort, http.StatusBadRequest, "", 0},
			{"synchronous output limit", "&max_tokens=" + strconv.Itoa(outputLimit+1), http.StatusBadRequest, "", 0},
		} {
			t.Run(model+"/"+scenario.name, func(t *testing.T) {
				projectPath := "/v1/projects/test-project/locations/global"
				if model == vertexProjectDefaultModel {
					projectPath = "/v1/projects/123456789012/locations/global"
				}
				const apiKey = "fixture-vertex-project-key"
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("x-goog-api-key") != apiKey || r.Header.Get("Authorization") != "" {
						t.Error("incorrect project-scoped Vertex authentication")
					}
					if calls.Add(1) == 1 {
						if r.URL.Path != projectPath+"/publishers/google/models/"+vertexProjectDefaultModel+":generateContent" {
							t.Errorf("wrong project verification path: %s", r.URL.Path)
						}
						io.WriteString(w, vertexSuccess)
						return
					}
					if r.Method != http.MethodPost || r.URL.Path != projectPath+"/publishers/google/models/"+model+":generateContent" {
						t.Errorf("unexpected Vertex request: %s %s", r.Method, r.URL.Path)
					}
					var body struct {
						Generation struct {
							Thinking *struct {
								Level string `json:"thinkingLevel"`
							} `json:"thinkingConfig"`
							MaxOutput int `json:"maxOutputTokens"`
						} `json:"generationConfig"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						return
					}
					thinking := body.Generation.Thinking
					if scenario.effort == "" && thinking != nil || scenario.effort != "" && (thinking == nil || thinking.Level != scenario.effort) {
						t.Errorf("thinking config=%+v, want effort %q", thinking, scenario.effort)
					}
					if body.Generation.MaxOutput != scenario.maxOutput {
						t.Errorf("Vertex maxOutputTokens=%d, want %d", body.Generation.MaxOutput, scenario.maxOutput)
					}
					io.WriteString(w, vertexSuccess)
				}))
				t.Cleanup(upstream.Close)
				config := proxy.Configuration{ProviderCatalog: catalog, Endpoints: providerEndpoints(upstream.URL+projectPath, vertexProjectProvider)}
				router := newOperationalProviderKeyVerificationRouter(t, config, zap.NewNop().Sugar(), t.TempDir()+"/managed.db", TestTimeout)
				owner := managementSessionCookie(t, "vertex-project-fixture")
				if model == vertexProjectDefaultModel && scenario.name == "omitted" {
					for _, rejected := range []string{
						"",
						"https://elsewhere.example" + projectPath,
						"https://aiplatform.googleapis.com:443" + projectPath,
						"https://aiplatform.googleapis.com" + projectPath + "/publishers/google/models",
						"https://aiplatform.googleapis.com/v1/projects/test%2Fproject/locations/global",
					} {
						response := createVertexProjectConnection(t, router, owner, apiKey, rejected)
						if response.Code != http.StatusBadRequest || calls.Load() != 0 {
							t.Fatalf("invalid project connection URL %q: status=%d calls=%d", rejected, response.Code, calls.Load())
						}
					}
				}
				secret := assignVertexProjectConnection(t, router, owner, apiKey, "https://aiplatform.googleapis.com"+projectPath)
				if model == vertexProjectDefaultModel && scenario.name == "omitted" {
					other := managementSessionCookie(t, "vertex-project-unconnected")
					otherTenantID := managementDefaultTenantTestID(t, router, other)
					otherSecret := generateManagementTenantSecret(t, router, other, otherTenantID)
					response := httptest.NewRecorder()
					router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?key="+otherSecret+"&provider="+vertexProjectProvider+"&model="+model+"&prompt=test", nil))
					if response.Code != http.StatusConflict || calls.Load() != 1 {
						t.Fatalf("tenant without its own project connection: status=%d, calls=%d", response.Code, calls.Load())
					}
				}
				server := httptest.NewServer(router)
				t.Cleanup(server.Close)
				request, err := http.NewRequest(http.MethodGet, server.URL+"/?key="+secret+"&provider="+vertexProjectProvider+"&model="+model+"&prompt=test"+scenario.parameters, nil)
				if err != nil {
					t.Fatal(err)
				}
				response, err := server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				if response.StatusCode != scenario.status {
					body, _ := io.ReadAll(response.Body)
					t.Fatalf("status=%d, want %d: %s", response.StatusCode, scenario.status, body)
				}
				if scenario.status == http.StatusOK {
					if calls.Load() != 2 || response.Header.Get("X-LLM-Proxy-Total-Tokens") != "16" {
						t.Fatal("expected one Vertex dispatch and measured usage")
					}
				} else if calls.Load() != 1 {
					t.Fatal("rejected request reached Vertex")
				}
			})
		}
	}
}

func TestGeminiCurrentModelsVertexCarbonPriceSchedule(t *testing.T) {
	service, err := proxy.NewCatalogService(testfixtures.ProviderCatalog(t).ModelCatalog())
	if err != nil {
		t.Fatal(err)
	}
	listPriceStart := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
	for _, component := range []struct {
		id          string
		cacheClass  string
		promotional proxy.CatalogDecimal
		list        proxy.CatalogDecimal
	}{
		{"input_tokens", "", "2", "4"},
		{"output_tokens", "", "10", "20"},
		{"prompt_cache_read_tokens", "read", "0.1", "0.2"},
	} {
		t.Run(component.id, func(t *testing.T) {
			conditions := proxy.CatalogPriceConditions{BillingMode: "standard", Region: "global", ServiceTier: "standard", CacheClass: component.cacheClass}
			promotional := service.SelectApplicablePrice(vertexProjectProvider, vertexProjectCarbonModel, proxy.ModelOperationText, component.id, conditions, 100, listPriceStart.Add(-time.Second))
			list := service.SelectApplicablePrice(vertexProjectProvider, vertexProjectCarbonModel, proxy.ModelOperationText, component.id, conditions, 100, listPriceStart)
			if !promotional.Available || promotional.Rate.Rate != component.promotional || !list.Available || list.Rate.Rate != component.list {
				t.Fatalf("Carbon %s rate at the promotional boundary: promotional=%+v list=%+v", component.id, promotional, list)
			}
		})
	}
}

func TestGeminiCurrentModelsVertexDictationProtocol(t *testing.T) {
	for _, filename := range []string{"voice.wav", "voice.unknown"} {
		t.Run(filename, func(t *testing.T) {
			server, _ := vertexHTTPFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/gemini-3.5-transcribe:generateContent") {
					t.Errorf("path=%s", r.URL.Path)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["contents"] == nil {
					t.Error("missing native audio contents")
				}
				io.WriteString(w, vertexSuccess)
			})
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			part, err := writer.CreateFormFile("audio", filename)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = part.Write([]byte("audio")); err != nil {
				t.Fatal(err)
			}
			if err = writer.Close(); err != nil {
				t.Fatal(err)
			}
			response, err := http.Post(server.URL+"/dictate?key="+TestSecret+"&provider=gemini&model=gemini-3.5-transcribe", writer.FormDataContentType(), &body)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			result, _ := io.ReadAll(response.Body)
			want := 200
			if filename == "voice.unknown" {
				want = 400
			}
			if response.StatusCode != want {
				t.Fatalf("status=%d body=%s", response.StatusCode, result)
			}
		})
	}
}

func TestGeminiCurrentModelsVertexActivationGate(t *testing.T) {
	schemaInput := testfixtures.ProviderCatalog(t).Schema()
	for i := range schemaInput.Providers {
		if schemaInput.Providers[i].ID == "vertex" {
			schemaInput.Providers[i].Enabled = proxy.ModelDisabled
		}
	}
	catalog, err := proxy.NewProviderCatalog(schemaInput)
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range catalog.ModelCatalog().Providers {
		if provider.ID == "vertex" {
			t.Fatal("unqualified Vertex provider is active")
		}
	}
	for _, offering := range catalog.ModelCatalog().Offerings {
		if offering.Provider == "vertex" {
			t.Fatal("unqualified Vertex offering is active")
		}
	}
	for _, price := range catalog.ModelCatalog().Prices {
		if price.Provider == "vertex" {
			t.Fatal("unqualified Vertex price is active")
		}
	}
	schema := catalog.Schema()
	for index := range schema.Providers {
		if schema.Providers[index].ID == "vertex" {
			schema.Providers[index].Enabled = 99
		}
	}
	if _, err := proxy.NewProviderCatalog(schema); err == nil {
		t.Fatal("invalid provider activation accepted")
	}
}
