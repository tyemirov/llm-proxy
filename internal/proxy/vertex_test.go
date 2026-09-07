package proxy_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
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
	}, "test", vertexTokenReply)
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
			transport.Endpoint.Path = "/projects"
			transport.Authentication.Kind = "google_credentials"
			transport.Authentication.Header = "Authorization"
			transport.Authentication.Prefix = "Bearer "
			transport.Headers = nil
			transport.RequestProtocol = "vertex_generate_content"
			transport.ResponseProtocol = "vertex_generate_content"
			transport.UsageMapping = "vertex_generate_content"
			transport.Lifecycle = "synchronous_completion"
			transport.ResourceVisibility = proxy.ProviderCatalogResourceVisibility{}
			transport.ProtocolParameters = proxy.ProviderCatalogProtocolParameters{
				ModelField: "path.model", TokenField: "generationConfig.maxOutputTokens", MediaExecutionLifecycle: "synchronous_completion",
				OutputFields: []string{"candidates[].content.parts[].text"},
				FinishRules:  proxy.ProviderCatalogFinishRules{Complete: []string{"STOP"}},
				ErrorRules:   []string{"MAX_TOKENS", "blocked", "unknown_finish_reason"},
				UsageFields:  proxy.ProviderCatalogUsageFields{Input: "usageMetadata.promptTokenCount", Output: "usageMetadata.candidatesTokenCount+thoughtsTokenCount", Total: "usageMetadata.totalTokenCount"},
			}
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

func vertexCredentialFile(t *testing.T, tokenURL string) string {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	credentialBytes, _ := json.Marshal(map[string]string{"type": "service_account", "project_id": "vertex-test-project", "client_email": "vertex@test.iam.gserviceaccount.com", "private_key": string(privatePEM), "token_uri": tokenURL})
	file, err := os.CreateTemp(t.TempDir(), "credential-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.Write(credentialBytes); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	return file.Name()
}

func vertexHTTPFixture(t *testing.T, handler http.HandlerFunc, profileTenant string, tokenReply string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	calls := &atomic.Int32{}
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || r.Form.Get("assertion") == "" {
			t.Error("missing signed credential assertion")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, tokenReply)
	}))
	t.Cleanup(tokenServer.Close)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer vertex-test-token" || r.Header.Get("x-goog-user-project") != "vertex-test-project" {
			t.Error("incorrect OAuth authorization or quota project")
		}
		handler(w, r)
	}))
	t.Cleanup(upstream.Close)
	config := proxy.Configuration{AssetStorePath: t.TempDir(), ProviderCatalog: vertexCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"gemini": upstream.URL, "vertex": upstream.URL}, map[string]map[string]string{"gemini": {"dictation": upstream.URL + "/projects"}}), GoogleCredentialProfiles: []proxy.GoogleCredentialProfile{{ID: "sk-gemini", TenantID: profileTenant, Project: "vertex-test-project", Location: "global", CredentialsFile: vertexCredentialFile(t, tokenServer.URL), CredentialType: "service_account"}}}
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

const vertexTokenReply = `{"access_token":"vertex-test-token","token_type":"Bearer","expires_in":3600}`

const vertexSuccess = `{"candidates":[{"content":{"parts":[{"text":"private thought","thought":true},{"text":"Vertex OK"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2,"thoughtsTokenCount":4,"totalTokenCount":16}}`

func TestGeminiCurrentModelsVertexManagementConnection(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, vertexTokenReply)
	}))
	defer tokenServer.Close()
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer vertex-test-token" || !strings.Contains(r.URL.Path, "/projects/vertex-test-project/") {
			t.Error("invalid managed Vertex authorization")
		}
		io.WriteString(w, vertexSuccess)
	}))
	defer upstream.Close()
	config := proxy.Configuration{ProviderCatalog: testfixtures.ProviderCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"vertex": upstream.URL}, nil)}
	database := t.TempDir() + "/managed.db"
	router := newOperationalProviderKeyVerificationRouter(t, config, zap.NewNop().Sugar(), database, TestTimeout)
	cookie := managementSessionCookie(t, "vertex-owner")
	tenantID := managementDefaultTenantTestID(t, router, cookie)
	config.GoogleCredentialProfiles = []proxy.GoogleCredentialProfile{{ID: "vertex-owned-profile", TenantID: tenantID, Project: "vertex-test-project", Location: "global", CredentialType: "service_account", CredentialsFile: vertexCredentialFile(t, tokenServer.URL)}}
	router = newOperationalProviderKeyVerificationRouter(t, config, zap.NewNop().Sugar(), database, TestTimeout)
	putConnection := func(tenantID string, cookie *http.Cookie) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPut, "/api/management/tenants/"+tenantID+"/provider-connections/vertex", strings.NewReader(`{"fields":{"credential_profile":"vertex-owned-profile"},"text_model":"gemini-3.8-flash","system_prompt":"Vertex system"}`))
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	response := putConnection(tenantID, cookie)
	if response.Code != 200 || calls != 1 {
		t.Fatalf("save Vertex status=%d calls=%d body=%s", response.Code, calls, response.Body)
	}
	profile := requestProviderKeyVerificationProfile(t, router, cookie, tenantID)
	provider := verificationProfileProvider(t, profile, "vertex")
	if !provider.Configured || provider.TextModel != "gemini-3.8-flash" || profile.Tenant.Defaults.Provider != "vertex" {
		t.Fatal("verified Vertex connection was not saved")
	}
	otherCookie := managementSessionCookie(t, "vertex-other-owner")
	otherTenant := managementDefaultTenantTestID(t, router, otherCookie)
	response = putConnection(otherTenant, otherCookie)
	if response.Code != 503 || calls != 1 {
		t.Fatalf("cross-tenant profile status=%d calls=%d", response.Code, calls)
	}
	if strings.Contains(response.Body.String(), "vertex-owned-profile") {
		t.Fatal("credential profile leaked in rejection")
	}
}

func TestGeminiCurrentModelsVertexContract(t *testing.T) {
	server, calls := vertexHTTPFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/projects/vertex-test-project/locations/global/publishers/google/models/gemini-3.5-flash:generateContent") {
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
	}, "test", vertexTokenReply)
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
	if calls.Load() != 1 {
		t.Fatalf("token cache calls=%d", calls.Load())
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
	}, "test", vertexTokenReply)
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
			}, "test", vertexTokenReply)
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

func TestGeminiCurrentModelsVertexTenantIsolation(t *testing.T) {
	server, calls := vertexHTTPFixture(t, func(w http.ResponseWriter, r *http.Request) { t.Error("cross-tenant dispatch") }, "another-tenant", vertexTokenReply)
	response, err := http.Get(server.URL + "/?key=" + TestSecret + "&provider=gemini&prompt=test")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 502 || calls.Load() != 0 {
		t.Fatalf("status=%d token calls=%d", response.StatusCode, calls.Load())
	}
}

func TestGeminiCurrentModelsVertexCredentialProfiles(t *testing.T) {
	valid := proxy.GoogleCredentialProfile{ID: "profile", TenantID: "test", Project: "vertex-test-project", Location: "global", CredentialType: "service_account", CredentialsFile: vertexCredentialFile(t, "https://oauth2.googleapis.com/token")}
	for _, testCase := range []struct {
		name string
		edit func(*proxy.GoogleCredentialProfile)
	}{
		{"missing id", func(p *proxy.GoogleCredentialProfile) { p.ID = "" }},
		{"missing tenant", func(p *proxy.GoogleCredentialProfile) { p.TenantID = "" }},
		{"invalid project", func(p *proxy.GoogleCredentialProfile) { p.Project = "../another-project" }},
		{"invalid location", func(p *proxy.GoogleCredentialProfile) { p.Location = "../../global" }},
		{"caller path", func(p *proxy.GoogleCredentialProfile) { p.CredentialsFile = "caller.json" }},
		{"unknown credential type", func(p *proxy.GoogleCredentialProfile) { p.CredentialType = "authorized_user" }},
		{"unavailable file", func(p *proxy.GoogleCredentialProfile) { p.CredentialsFile = t.TempDir() + "/missing.json" }},
		{"wrong file type", func(p *proxy.GoogleCredentialProfile) { p.CredentialType = "external_account" }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			profile := valid
			testCase.edit(&profile)
			_, err := proxy.NewConfiguration(proxy.Configuration{ProviderCatalog: vertexCatalog(t), GoogleCredentialProfiles: []proxy.GoogleCredentialProfile{profile}})
			if err == nil {
				t.Fatal("invalid profile accepted")
			}
		})
	}
	if _, err := proxy.NewConfiguration(proxy.Configuration{ProviderCatalog: vertexCatalog(t), GoogleCredentialProfiles: []proxy.GoogleCredentialProfile{valid, valid}}); err == nil {
		t.Fatal("duplicate profile accepted")
	}
}

func TestGeminiCurrentModelsVertexOAuthRefreshAndFailure(t *testing.T) {
	for _, testCase := range []struct {
		name, reply string
		want, calls int
	}{
		{"refresh window", `{"access_token":"vertex-test-token","token_type":"Bearer","expires_in":1}`, 200, 2},
		{"rejected", `{"error":"invalid_grant","error_description":"private credential detail"}`, 502, 2},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server, calls := vertexHTTPFixture(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, vertexSuccess) }, "test", testCase.reply)
			for index := range 2 {
				if index == 1 && testCase.name == "refresh window" {
					time.Sleep(1100 * time.Millisecond)
				}
				response, err := http.Get(server.URL + "/?key=" + TestSecret + "&provider=gemini&prompt=test")
				if err != nil {
					t.Fatal(err)
				}
				body, _ := io.ReadAll(response.Body)
				response.Body.Close()
				if response.StatusCode != testCase.want || strings.Contains(string(body), "private credential detail") {
					t.Fatalf("status=%d body=%s", response.StatusCode, body)
				}
			}
			if int(calls.Load()) != testCase.calls {
				t.Fatalf("token requests=%d", calls.Load())
			}
		})
	}
}

func TestGeminiCurrentModelsVertexShippedRoute(t *testing.T) {
	server, _ := vertexHTTPFixture(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, vertexSuccess) }, "test", vertexTokenReply)
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
			}, "test", vertexTokenReply)
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
