package proxy_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/glebarez/sqlite"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
)

func TestProviderServicesAlignWithoutInventedModel(t *testing.T) {
	for _, provider := range []string{"elevenlabs", "alignment-fixture"} {
		t.Run(provider, func(t *testing.T) {
			audio := metaTranscriptionWAV(16000, 1)
			var submissions atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("xi-api-key") != "alignment-secret" {
					t.Error("alignment lost shared credential")
					w.WriteHeader(401)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet && r.URL.Path == "/v1/user/subscription" {
					_, _ = io.WriteString(w, elevenQuotaFixture)
					return
				}
				if r.Method != http.MethodPost || r.URL.Path != "/v1/forced-alignment" {
					t.Errorf("unexpected service request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
					return
				}
				submissions.Add(1)
				if err := r.ParseMultipartForm(2 << 20); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				defer r.MultipartForm.RemoveAll()
				if len(r.MultipartForm.Value) != 1 || r.FormValue("text") != "hello 42" || len(r.MultipartForm.File) != 1 {
					t.Errorf("invalid alignment fields: %v", r.MultipartForm.Value)
				}
				file, _, err := r.FormFile("file")
				if err != nil {
					t.Error(err)
					return
				}
				defer file.Close()
				data, _ := io.ReadAll(file)
				if !bytes.Equal(data, audio) {
					t.Error("alignment changed the input asset")
				}
				_, _ = io.WriteString(w, `{"characters":[{"text":"h","start":0.1,"end":0.2}],"words":[{"text":"hello","start":0.1,"end":0.5,"loss":0.25},{"text":"!","start":0.5,"end":0.6,"loss":0.1},{"text":"42","start":0.6,"end":1.0,"loss":0.5}],"loss":0.2}`)
			}))
			defer upstream.Close()
			catalog := elevenResourceCatalog(t, provider, upstream.URL)
			router := newManagementRouter(t, proxy.Configuration{ProviderCatalog: catalog, AssetStorePath: t.TempDir(), UpstreamCapacity: testfixtures.UpstreamCapacity(4, 100)})
			server := httptest.NewServer(router)
			defer server.Close()
			owner := managementSessionCookie(t, "service-owner")
			tenantID := managementDefaultTenantTestID(t, router, owner)
			secret := generateManagementTenantSecret(t, router, owner, tenantID)
			connection := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{"name": "One media account", "provider": provider, "fields": map[string]string{"resource_token": "alignment-secret"}}, http.StatusCreated)
			accountConnectionExchange(t, router, owner, http.MethodPut, "/tenants/"+tenantID+"/connections/"+provider, map[string]string{"kind": "account_connection", "resource_id": connection["id"].(string)}, http.StatusOK)
			configuration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: secret})
			if err != nil {
				t.Fatal(err)
			}
			client, err := llmproxyclient.NewClient(configuration, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			response, err := server.Client().Get(server.URL + "/api/public/capabilities")
			if err != nil {
				t.Fatal(err)
			}
			var discovery struct {
				Providers []struct {
					Identifier string `json:"identifier"`
					Services   []struct {
						Operation string `json:"operation"`
					} `json:"services"`
				} `json:"providers"`
			}
			err = json.NewDecoder(response.Body).Decode(&discovery)
			response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, item := range discovery.Providers {
				if item.Identifier == provider {
					found = len(item.Services) == 2 && item.Services[0].Operation == "audio_alignment"
				}
			}
			if !found {
				t.Fatal("public discovery omitted provider service")
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			asset, err := client.UploadAsset(ctx, llmproxyclient.AssetUploadInput{MIMEType: "audio/wav", Data: audio})
			if err != nil {
				t.Fatal(err)
			}
			input := llmproxyclient.MediaOperationInput{Capability: "audio.align", Provider: provider, Input: json.RawMessage(`{"audio_asset_id":"` + asset.AssetID + `","transcript":"hello 42"}`), Controls: json.RawMessage(`{}`)}
			accepted, err := client.CreateMediaOperation(ctx, "model-free-alignment", input)
			if err != nil {
				t.Fatalf("declared service must accept an omitted model: %v", err)
			}
			completed, err := client.WaitMediaOperation(ctx, accepted.OperationID, 5*time.Millisecond)
			if err != nil || completed.State != proxy.MediaOperationStateSucceeded || len(completed.Outputs) != 1 {
				t.Fatalf("service result=%+v error=%v native=%d failure=%+v", completed, err, submissions.Load(), completed.Error)
			}
			output, err := client.GetAsset(ctx, completed.Outputs[0].AssetID)
			if err != nil {
				t.Fatal(err)
			}
			data, err := client.DownloadAsset(ctx, output)
			if err != nil {
				t.Fatal(err)
			}
			var aligned struct {
				Words []struct {
					Text string  `json:"text"`
					Loss float64 `json:"loss"`
				} `json:"words"`
				Characters []json.RawMessage `json:"characters"`
				Loss       float64           `json:"loss"`
			}
			if err := json.Unmarshal(data, &aligned); err != nil || len(aligned.Words) != 2 || aligned.Words[0].Text != "hello" || aligned.Words[0].Loss != 0.25 || aligned.Words[1].Text != "42" || len(aligned.Characters) != 1 || aligned.Loss != 0.2 {
				t.Fatalf("alignment artifact=%s error=%v", data, err)
			}
			repeated, err := client.CreateMediaOperation(ctx, "model-free-alignment", input)
			if err != nil || repeated.OperationID != accepted.OperationID || submissions.Load() != 1 {
				t.Fatalf("service replay=%+v count=%d error=%v", repeated, submissions.Load(), err)
			}
			capabilities, err := client.GetMediaCapabilities(ctx)
			if err != nil || len(capabilities.Services) != 2 || capabilities.Services[0].Provider != provider || capabilities.Services[0].Capability != "audio.align" {
				t.Fatalf("services=%+v error=%v", capabilities.Services, err)
			}
			usage := requestManagementUsage(t, router, owner, "30d")
			if len(usage.Providers) != 1 || usage.Providers[0].Provider != provider || usage.Providers[0].Data.Requests != 1 || len(usage.Models) != 0 {
				t.Fatalf("model-free usage=%+v", usage)
			}
			for index, model := range []string{`null`, `""`, `" "`, `3`} {
				body := fmt.Sprintf(`{"capability":"audio.align","provider":%q,"model":%s,"input":%s,"controls":{}}`, provider, model, input.Input)
				req, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/model/v1/operations", strings.NewReader(body))
				req.Header.Set("Authorization", "Bearer "+secret)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Idempotency-Key", fmt.Sprintf("bad-model-%d", index))
				response, err := server.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if response.StatusCode != 400 || submissions.Load() != 1 {
					t.Fatalf("invalid model status=%d", response.StatusCode)
				}
			}
			unavailable := input
			unavailable.Provider = "openai"
			if _, err := client.CreateMediaOperation(ctx, "undeclared-service", unavailable); err == nil {
				t.Fatal("undeclared service accepted")
			}
			input.Model = "eleven_v3"
			if _, err := client.CreateMediaOperation(ctx, "invalid-service-model", input); err == nil || submissions.Load() != 1 {
				t.Fatal("service accepted an invented model")
			}
		})
	}
}

// The fixture keeps real HTTP, storage, credentials, admission, and workers.
func providerServicesFixture(t *testing.T, native http.HandlerFunc, changes ...func(*proxy.Configuration)) (llmproxyclient.Client, *gorm.DB, func() llmproxyclient.Client) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/user/subscription" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, elevenQuotaFixture)
			return
		}
		native(w, r)
	}))
	t.Cleanup(upstream.Close)
	catalog := elevenResourceCatalog(t, "elevenlabs", upstream.URL)
	databasePath := filepath.Join(t.TempDir(), "alignment.sqlite")
	configuration := proxy.Configuration{ProviderCatalog: catalog, AssetStorePath: t.TempDir(), MaxAssetBytes: 128 << 10, UpstreamCapacity: testfixtures.UpstreamCapacity(4, 100), MediaOperationWorkers: 1, MediaOperationClaimSeconds: 7200, MediaOperationClaimRenewalSeconds: 3600}
	for _, change := range changes {
		change(&configuration)
	}
	router := newManagementRouterWithDatabasePath(t, configuration, databasePath)
	owner := managementSessionCookie(t, "service-fixture")
	tenantID := managementDefaultTenantTestID(t, router, owner)
	secret := generateManagementTenantSecret(t, router, owner, tenantID)
	connection := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{"name": "Alignment", "provider": "elevenlabs", "fields": map[string]string{"resource_token": "alignment-secret"}}, http.StatusCreated)
	accountConnectionExchange(t, router, owner, http.MethodPut, "/tenants/"+tenantID+"/connections/elevenlabs", map[string]string{"kind": "account_connection", "resource_id": connection["id"].(string)}, http.StatusOK)
	connect := func(handler http.Handler) llmproxyclient.Client {
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: secret})
		if err != nil {
			t.Fatal(err)
		}
		client, err := llmproxyclient.NewClient(config, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	database, err := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return connect(router), database, func() llmproxyclient.Client {
		return connect(newManagementRouterWithDatabasePath(t, configuration, databasePath))
	}
}

func providerServicesInput(t *testing.T, client llmproxyclient.Client) llmproxyclient.MediaOperationInput {
	t.Helper()
	asset, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "audio/wav", Data: metaTranscriptionWAV(16000, 1)})
	if err != nil {
		t.Fatal(err)
	}
	return llmproxyclient.MediaOperationInput{Capability: "audio.align", Provider: "elevenlabs", Input: json.RawMessage(`{"audio_asset_id":"` + asset.AssetID + `","transcript":"hello"}`), Controls: json.RawMessage(`{}`)}
}

func TestProviderServicesNativeFailuresAndRecoveryNeverResubmit(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		status int
		body   string
		state  string
	}{
		{"rejected", 401, `{"detail":"secret native text"}`, "failed"},
		{"timeout", 408, `{}`, "uncertain"},
		{"server", 503, `{}`, "uncertain"},
		{"malformed", 200, `{`, "uncertain"},
		{"oversize", 200, strings.Repeat("x", 129<<10), "uncertain"},
		{"truncated", -1, "", "uncertain"},
		{"missing arrays", 200, `{"words":[]}`, "uncertain"},
		{"missing timing", 200, `{"characters":[],"words":[{"text":"hello"}]}`, "uncertain"},
		{"reversed timing", 200, `{"characters":[],"words":[{"text":"hello","start":1,"end":0}]}`, "uncertain"},
		{"negative loss", 200, `{"characters":[],"words":[],"loss":-1}`, "uncertain"},
		{"punctuation only", 200, `{"characters":[],"words":[{"text":"!","start":0,"end":1}]}`, "uncertain"},
		{"disconnected", 0, "", "uncertain"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var submissions atomic.Int32
			client, database, restart := providerServicesFixture(t, func(w http.ResponseWriter, r *http.Request) {
				submissions.Add(1)
				if scenario.status == -1 {
					w.Header().Set("Content-Length", "100")
					w.WriteHeader(200)
					_, _ = io.WriteString(w, `{`)
					return
				}
				if scenario.status == 0 {
					connection, _, _ := w.(http.Hijacker).Hijack()
					_ = connection.Close()
					return
				}
				w.WriteHeader(scenario.status)
				_, _ = io.WriteString(w, scenario.body)
			})
			input := providerServicesInput(t, client)
			accepted, err := client.CreateMediaOperation(t.Context(), "native-failure", input)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			result, err := client.WaitMediaOperation(ctx, accepted.OperationID, time.Millisecond)
			if err != nil || result.State != scenario.state || len(result.Outputs) != 0 || result.Error == nil || (result.Error.Code != "provider_error" && result.Error.Code != "provider_outcome_unknown") {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			// Simulate a worker lost after dispatch, before its terminal result was stored.
			if err := database.Table("media_operation_records").Where("operation_id = ?", accepted.OperationID).Updates(map[string]any{"public_state": "running", "provider_execution_state": "dispatched", "terminal_at": nil}).Error; err != nil {
				t.Fatal(err)
			}
			recoveredClient := restart()
			recovered, err := recoveredClient.WaitMediaOperation(ctx, accepted.OperationID, time.Millisecond)
			if err != nil || recovered.State != "uncertain" || submissions.Load() != 1 {
				t.Fatalf("recovered=%+v err=%v posts=%d", recovered, err, submissions.Load())
			}
			repeated, err := recoveredClient.CreateMediaOperation(ctx, "native-failure", input)
			if err != nil || repeated.OperationID != accepted.OperationID || submissions.Load() != 1 {
				t.Fatalf("replay=%+v err=%v", repeated, err)
			}
		})
	}
}

func TestProviderServicesInvalidInputsAndQueuedCancellation(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var submissions atomic.Int32
	client, _, _ := providerServicesFixture(t, func(w http.ResponseWriter, r *http.Request) {
		submissions.Add(1)
		close(entered)
		<-release
		_, _ = io.WriteString(w, `{"characters":[],"words":[{"text":"hello","start":0,"end":1}]}`)
	})
	input := providerServicesInput(t, client)
	for index, body := range []string{`{}`, `{"audio_asset_id":"ast_0123456789abcdef0123456789abcdef","transcript":"hello"}`, `{"transcript":" "}`, `{"transcript":"hello","unknown":1}`} {
		invalid := input
		invalid.Input = json.RawMessage(body)
		if _, err := client.CreateMediaOperation(t.Context(), fmt.Sprintf("invalid-%d", index), invalid); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	invalid := input
	invalid.Controls = json.RawMessage(`{"model":"invented"}`)
	if _, err := client.CreateMediaOperation(t.Context(), "invalid-controls", invalid); err == nil {
		t.Fatal("unknown control accepted")
	}
	first, err := client.CreateMediaOperation(t.Context(), "first", input)
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	// Cleanup releases native work even if a later assertion fails.
	t.Cleanup(func() {
		close(release)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		result, err := client.WaitMediaOperation(ctx, first.OperationID, time.Millisecond)
		if err != nil || result.State != "succeeded" {
			t.Errorf("released operation=%+v err=%v", result, err)
		}
	})
	canceled, err := client.CancelMediaOperation(t.Context(), first.OperationID)
	if err != nil || canceled.CancellationState != "unsupported" {
		t.Fatalf("running cancel=%+v err=%v", canceled, err)
	}
	second, err := client.CreateMediaOperation(t.Context(), "second", input)
	if err != nil {
		t.Fatal(err)
	}
	canceled, err = client.CancelMediaOperation(t.Context(), second.OperationID)
	if err != nil || canceled.State != "cancelled" || canceled.CancellationState != "confirmed" || submissions.Load() != 1 {
		t.Fatalf("queued cancel=%+v err=%v", canceled, err)
	}
}

func TestProviderServicesCatalogRejectsInvalidBindings(t *testing.T) {
	for name, change := range map[string]func(*proxy.ProviderCatalogProvider){
		"missing transport":     func(p *proxy.ProviderCatalogProvider) { p.Services[0].Transport = "absent" },
		"wrong protocol":        func(p *proxy.ProviderCatalogProvider) { p.Services[0].Transport = "models" },
		"wrong operation":       func(p *proxy.ProviderCatalogProvider) { p.Services[0].Operation = "text" },
		"duplicate":             func(p *proxy.ProviderCatalogProvider) { p.Services = append(p.Services, p.Services[0]) },
		"wrong price operation": func(p *proxy.ProviderCatalogProvider) { p.Services[0].Price.Operation = "text" },
		"wrong price":           func(p *proxy.ProviderCatalogProvider) { p.Services[0].Price.Source = "http://example.com" },
		"controls": func(p *proxy.ProviderCatalogProvider) {
			p.Services[0].Controls = []proxy.CatalogControl{{ID: "mode", Kind: "enum", Values: []string{"fast"}}}
		},
		"dictionary controls": func(p *proxy.ProviderCatalogProvider) {
			p.Services[1].Controls = []proxy.CatalogControl{{ID: "mode", Kind: "enum", Values: []string{"fast"}}}
		},
		"dictionary limits": func(p *proxy.ProviderCatalogProvider) { p.Services[1].Limits = p.Services[0].Limits },
		"limit":             func(p *proxy.ProviderCatalogProvider) { p.Services[0].Limits[0].Value = new(1000000001) },
	} {
		t.Run(name, func(t *testing.T) {
			schema := testfixtures.ProviderCatalog(t).Schema()
			for i := range schema.Providers {
				if schema.Providers[i].ID == "elevenlabs" {
					change(&schema.Providers[i])
				}
			}
			data, err := yaml.Marshal(schema)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := proxy.ParseProviderCatalog(data); err == nil {
				t.Fatal("invalid service catalog accepted")
			}
		})
	}
}

func TestProviderServicesCatalogServiceSnapshot(t *testing.T) {
	catalog := testfixtures.ProviderCatalog(t).ModelCatalog()
	index := 0
	for i, p := range catalog.Providers {
		if p.ID == "elevenlabs" {
			index = i
		}
	}
	catalog.Providers[index].Services[0].Price.MinimumCharge = &proxy.CatalogMinimumCharge{Currency: "USD", Amount: "1", Unit: "request"}
	service, err := proxy.NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	route, err := service.ResolveService("elevenlabs", "audio_alignment")
	if err != nil {
		t.Fatal(err)
	}
	*route.Limits[0].Value = 2
	route.Price.MinimumCharge.Amount = "999"
	route, err = service.ResolveService("elevenlabs", "audio_alignment")
	if err != nil || *route.Limits[0].Value != 1000000000 || route.Price.MinimumCharge.Amount != "1" {
		t.Fatalf("mutated immutable route=%+v error=%v", route, err)
	}
	for name, change := range map[string]func(*proxy.ProviderCatalogService){
		"invalid transport": func(s *proxy.ProviderCatalogService) { s.Transport = " " },
		"invalid controls": func(s *proxy.ProviderCatalogService) {
			s.Controls = []proxy.CatalogControl{{ID: "mode", Kind: "unknown"}}
		},
		"invalid limits": func(s *proxy.ProviderCatalogService) { s.Limits[0].ID = "" },
		"invalid price":  func(s *proxy.ProviderCatalogService) { s.Price.Source = "invalid" },
	} {
		t.Run(name, func(t *testing.T) {
			modelCatalog := testfixtures.ProviderCatalog(t).ModelCatalog()
			change(&modelCatalog.Providers[index].Services[0])
			if _, err := proxy.NewCatalogService(modelCatalog); err == nil {
				t.Fatal("invalid service snapshot accepted")
			}
		})
	}
}

func TestProviderServicesAndModelOfferingsShareOneConnection(t *testing.T) {
	var output bytes.Buffer
	if err := png.Encode(&output, image.NewRGBA(image.Rect(0, 0, 1024, 1024))); err != nil {
		t.Fatal(err)
	}
	var images, alignments atomic.Int32
	client, _, _ := providerServicesFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/forced-alignment":
			if r.Header.Get("xi-api-key") != "alignment-secret" {
				t.Error("service lost shared credential")
			}
			alignments.Add(1)
			_, _ = io.WriteString(w, `{"characters":[],"words":[{"text":"hello","start":0,"end":1}]}`)
		case "/images/generations":
			if r.Header.Get("Authorization") != "Bearer alignment-secret" {
				t.Error("model route lost shared credential")
			}
			images.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(output.Bytes())}}})
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}, func(configuration *proxy.Configuration) {
		schema := configuration.ProviderCatalog.Schema()
		var models proxy.ProviderCatalogProvider
		for _, provider := range schema.Providers {
			if provider.ID == "openai" {
				models = provider
			}
		}
		for i := range schema.Providers {
			p := &schema.Providers[i]
			if p.ID != "elevenlabs" {
				continue
			}
			origin := p.Transports[0].Endpoint.DefaultBaseURL
			p.Offerings = append(p.Offerings, models.Offerings...)
			for _, transport := range models.Transports {
				transport.Endpoint.DefaultBaseURL = origin
				transport.Components.Authentication.Field = "resource_token"
				p.Transports = append(p.Transports, transport)
			}
		}
		data, err := yaml.Marshal(schema)
		if err != nil {
			t.Fatal(err)
		}
		configuration.ProviderCatalog, err = proxy.ParseProviderCatalog(data)
		if err != nil {
			t.Fatal(err)
		}
	})
	inputs := []llmproxyclient.MediaOperationInput{
		providerServicesInput(t, client),
		{Capability: "image.generate", Provider: "elevenlabs", Model: "gpt-image-2", Input: json.RawMessage(`{"prompt":"hello"}`), Controls: json.RawMessage(`{"surface":"images","quality":"low","size":"1024x1024","background":"opaque","output_format":"png","output_count":1}`)},
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for index, input := range inputs {
		accepted, err := client.CreateMediaOperation(ctx, fmt.Sprintf("shared-%d", index), input)
		if err != nil {
			t.Fatal(err)
		}
		result, err := client.WaitMediaOperation(ctx, accepted.OperationID, time.Millisecond)
		if err != nil || result.State != "succeeded" {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	}
	if images.Load() != 1 || alignments.Load() != 1 {
		t.Fatal("shared routes did not execute")
	}
}

func TestProviderServicesMCPModelPresence(t *testing.T) {
	var submissions atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, elevenQuotaFixture)
			return
		}
		submissions.Add(1)
		_, _ = io.WriteString(w, `{"characters":[],"words":[{"text":"hello","start":0,"end":1}]}`)
	}))
	defer upstream.Close()
	fixture := newMCPFixtureWithConfig(t, nil, proxy.Configuration{ProviderCatalog: elevenResourceCatalog(t, "elevenlabs", upstream.URL), AssetStorePath: t.TempDir(), UpstreamCapacity: testfixtures.UpstreamCapacity(4, 100)})
	owner := managementSessionCookie(t, "alignment-mcp")
	tenantID := managementDefaultTenantTestID(t, fixture.router, owner)
	connection := accountConnectionExchange(t, fixture.router, owner, http.MethodPost, "/connections", map[string]any{"name": "Alignment", "provider": "elevenlabs", "fields": map[string]string{"resource_token": "alignment-secret"}}, http.StatusCreated)
	accountConnectionExchange(t, fixture.router, owner, http.MethodPut, "/tenants/"+tenantID+"/connections/elevenlabs", map[string]string{"kind": "account_connection", "resource_id": connection["id"].(string)}, http.StatusOK)
	configuration, _ := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: fixture.server.URL, Secret: generateManagementTenantSecret(t, fixture.router, owner, tenantID)})
	client, err := llmproxyclient.NewClient(configuration, fixture.server.Client())
	if err != nil {
		t.Fatal(err)
	}
	input := providerServicesInput(t, client)
	var body map[string]any
	_ = json.Unmarshal(input.Input, &body)
	arguments := map[string]any{"tenant_id": tenantID, "idempotency_key": "mcp-align", "capability": "audio.align", "provider": "elevenlabs", "input": body, "controls": map[string]any{}}
	session := fixture.client(t, "alignment-mcp")
	result := mcpCall(t, session, "llm_proxy.create_media_operation", arguments)
	if result.IsError {
		t.Fatalf("model-free MCP request=%+v", result)
	}
	encoded, _ := json.Marshal(result.StructuredContent)
	var operation llmproxyclient.MediaOperation
	_ = json.Unmarshal(encoded, &operation)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	completed, err := client.WaitMediaOperation(ctx, operation.OperationID, time.Millisecond)
	if err != nil || completed.State != "succeeded" {
		t.Fatalf("MCP operation=%+v error=%v", completed, err)
	}
	for index, model := range []any{"", " ", nil, 3, "invented"} {
		arguments["model"] = model
		arguments["idempotency_key"] = fmt.Sprintf("mcp-invalid-%d", index)
		result = mcpCall(t, session, "llm_proxy.create_media_operation", arguments)
		if !result.IsError {
			t.Fatalf("MCP accepted model=%v", model)
		}
	}
	if submissions.Load() != 1 {
		t.Fatalf("MCP dispatches=%d", submissions.Load())
	}
}

func TestProviderServicesQueuedStateAndAssetFailuresDoNotDispatch(t *testing.T) {
	for _, failure := range []string{"binding", "metadata", "data"} {
		t.Run(failure, func(t *testing.T) {
			var posts atomic.Int32
			var root string
			client, database, restart := providerServicesFixture(t, func(w http.ResponseWriter, r *http.Request) {
				posts.Add(1)
				_, _ = io.WriteString(w, `{"characters":[],"words":[{"text":"hello","start":0,"end":1}]}`)
			}, func(configuration *proxy.Configuration) { root = configuration.AssetStorePath })
			input := providerServicesInput(t, client)
			accepted, err := client.CreateMediaOperation(t.Context(), "fault", input)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			complete, err := client.WaitMediaOperation(ctx, accepted.OperationID, time.Millisecond)
			if err != nil || complete.State != "succeeded" {
				t.Fatalf("initial=%+v err=%v", complete, err)
			}
			changes := map[string]any{"public_state": "queued", "provider_execution_state": "not_dispatched", "terminal_at": nil}
			if failure == "binding" {
				changes["execution_binding"] = "obsolete-transport"
			} else {
				var value struct {
					AssetID string `json:"audio_asset_id"`
				}
				_ = json.Unmarshal(input.Input, &value)
				suffix := ".data"
				if failure == "metadata" {
					suffix = ".json"
				}
				if err := os.Remove(filepath.Join(root, value.AssetID+suffix)); err != nil {
					t.Fatal(err)
				}
			}
			if err := database.Table("media_operation_records").Where("operation_id = ?", accepted.OperationID).Updates(changes).Error; err != nil {
				t.Fatal(err)
			}
			result, err := restart().WaitMediaOperation(ctx, accepted.OperationID, time.Millisecond)
			if err != nil || result.State != "failed" || posts.Load() != 1 {
				t.Fatalf("fault=%s result=%+v err=%v posts=%d", failure, result, err, posts.Load())
			}
		})
	}
}
