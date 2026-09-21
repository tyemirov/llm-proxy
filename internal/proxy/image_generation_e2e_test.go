package proxy_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

func TestImageGenerationReturnsOrderedTenantArtifacts(t *testing.T) {
	const secret = "image-generation-tenant"
	const providerKey = "image-generation-provider-key"
	const prompt = "A small lighthouse beside the sea"
	outputs := make([][]byte, 2)
	for index := range outputs {
		canvas := image.NewRGBA(image.Rect(0, 0, 1024, 1024))
		canvas.SetRGBA(index, index, color.RGBA{R: uint8(80 + index), A: 255})
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, canvas); err != nil {
			t.Fatal(err)
		}
		outputs[index] = encoded.Bytes()
	}
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodPost || request.URL.Path != "/images/generations" || request.Header.Get("Authorization") != "Bearer "+providerKey {
			t.Errorf("unexpected image provider request: method=%s path=%s", request.Method, request.URL.Path)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		expected := map[string]any{"model": "gpt-image-2", "prompt": prompt, "quality": "low", "size": "1024x1024", "background": "opaque", "output_format": "png", "n": float64(2)}
		if !reflect.DeepEqual(payload, expected) {
			t.Errorf("image request=%v want=%v", payload, expected)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"created": time.Now().Unix(), "data": []map[string]string{
			{"b64_json": base64.StdEncoding.EncodeToString(outputs[0])},
			{"b64_json": base64.StdEncoding.EncodeToString(outputs[1])},
		}})
	}))
	defer upstream.Close()
	endpoints := proxy.NewEndpoints()
	endpoints.SetProviderBaseURL(proxy.ProviderNameOpenAI, upstream.URL)
	assetRoot := t.TempDir()
	routerConfiguration, err := testfixtures.ProvisionManagedRouter(t, proxy.Configuration{
		ProviderCatalog: testfixtures.ProviderCatalog(t), AssetStorePath: assetRoot, Endpoints: endpoints,
	}, zap.NewNop().Sugar(), testfixtures.ManagedTenant{
		Secret: secret, Defaults: proxy.TenantDefaults{Provider: proxy.ProviderNameOpenAI, Model: proxy.ModelNameGPT41},
		ProviderKeys: map[string]string{proxy.ProviderNameOpenAI: providerKey},
	})
	if err != nil {
		t.Fatal(err)
	}
	router, err := proxy.BuildRouter(routerConfiguration, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	configuration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: secret})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(configuration, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	input := llmproxyclient.MediaOperationInput{
		Capability: "image.generate", Provider: "openai", Model: "gpt-image-2",
		Input:    json.RawMessage(`{"prompt":"` + prompt + `"}`),
		Controls: json.RawMessage(`{"surface":"images","quality":"low","size":"1024x1024","background":"opaque","output_format":"png","output_count":2}`),
	}
	accepted, err := client.CreateMediaOperation(t.Context(), "first-openai-image", input)
	if err != nil {
		t.Fatalf("accept image generation through the official client: %v", err)
	}
	waitContext, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	completed, err := client.WaitMediaOperation(waitContext, accepted.OperationID, 5*time.Millisecond)
	if err != nil || completed.State != proxy.MediaOperationStateSucceeded || len(completed.Outputs) != len(outputs) {
		t.Fatalf("image result=%+v error=%v", completed, err)
	}
	for index, output := range completed.Outputs {
		if output.Ordinal != index || output.MIMEType != "image/png" || output.SizeBytes != int64(len(outputs[index])) {
			t.Fatalf("output %d metadata=%+v", index, output)
		}
		asset, err := client.GetAsset(t.Context(), output.AssetID)
		if err != nil {
			t.Fatal(err)
		}
		data, err := client.DownloadAsset(t.Context(), asset)
		if err != nil || !bytes.Equal(data, outputs[index]) {
			t.Fatalf("output %d bytes differ: error=%v", index, err)
		}
	}
	duplicate, err := client.CreateMediaOperation(t.Context(), "first-openai-image", input)
	if err != nil || duplicate.OperationID != accepted.OperationID || calls.Load() != 1 {
		t.Fatalf("duplicate=%+v error=%v provider_calls=%d", duplicate, err, calls.Load())
	}
	ownerCookie := imageGenerationOwnerCookie(t, routerConfiguration.Management, "foreign-image-owner")
	foreignAccount := requestManagementAccount(t, router, ownerCookie)
	foreignSecret := generateManagementTenantSecret(t, router, ownerCookie, foreignAccount.Tenants[0].ID)
	foreignConfiguration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: foreignSecret})
	if err != nil {
		t.Fatal(err)
	}
	foreignClient, err := llmproxyclient.NewClient(foreignConfiguration, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := foreignClient.GetMediaOperation(t.Context(), accepted.OperationID); httpFailureStatus(err) != http.StatusNotFound {
		t.Fatalf("foreign operation: %v", err)
	}
	if _, err := foreignClient.CancelMediaOperation(t.Context(), accepted.OperationID); httpFailureStatus(err) != http.StatusNotFound {
		t.Fatalf("foreign cancellation: %v", err)
	}
	if _, err := foreignClient.GetAsset(t.Context(), completed.Outputs[0].AssetID); httpFailureStatus(err) != http.StatusNotFound {
		t.Fatalf("foreign asset: %v", err)
	}
	asset, err := client.GetAsset(t.Context(), completed.Outputs[0].AssetID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := foreignClient.DownloadAsset(t.Context(), asset); httpFailureStatus(err) != http.StatusNotFound {
		t.Fatalf("foreign download: %v", err)
	}
	if _, err := foreignClient.CreateImageGeneration(t.Context(), "no-provider-connection", imageGenerationTestIntent()); err == nil {
		t.Fatal("unconfigured provider accepted")
	}
	invalidConfiguration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "invalid-image-tenant"})
	if err != nil {
		t.Fatal(err)
	}
	invalidClient, err := llmproxyclient.NewClient(invalidConfiguration, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := invalidClient.CreateImageGeneration(t.Context(), "invalid-credential", imageGenerationTestIntent()); httpFailureStatus(err) != http.StatusForbidden {
		t.Fatalf("invalid tenant credential: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("unauthorized provider calls=%d", calls.Load())
	}
	privateMetadata, err := os.ReadFile(filepath.Join(assetRoot, asset.AssetID+".json"))
	if err != nil || !bytes.Contains(privateMetadata, []byte("sha256")) {
		t.Fatalf("private integrity metadata missing: %v", err)
	}
	publicMetadata, _ := json.Marshal(asset)
	publicOperation, _ := json.Marshal(completed)
	for _, public := range [][]byte{publicMetadata, publicOperation} {
		if bytes.Contains(public, []byte(`"content_sha256":`)) || bytes.Contains(public, []byte(`"sha256":`)) || bytes.Contains(public, []byte(`"checksum":`)) || bytes.Contains(public, []byte(providerKey)) {
			t.Fatalf("private evidence in public result: %s", public)
		}
	}
	corrupted := append([]byte(nil), outputs[0]...)
	corrupted[len(corrupted)-1] ^= 1
	file, err := os.Create(filepath.Join(assetRoot, asset.AssetID+".data"))
	if err != nil {
		t.Fatal(err)
	}
	_, writeError := file.Write(corrupted)
	closeError := file.Close()
	if writeError != nil || closeError != nil {
		t.Fatalf("corrupt test artifact: write=%v close=%v", writeError, closeError)
	}
	if _, err := client.DownloadAsset(t.Context(), asset); err == nil {
		t.Fatal("corrupted stored image downloaded")
	}
}

func imageGenerationOwnerCookie(t *testing.T, configuration proxy.ManagementConfiguration, owner string) *http.Cookie {
	t.Helper()
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"iss": proxy.DefaultManagementJWTIssuer, "tenant_id": configuration.TAuthTenantID, "user_id": owner, "user_email": owner + "@example.com", "iat": now.Add(-time.Minute).Unix(), "exp": now.Add(time.Hour).Unix()})
	signed, err := token.SignedString([]byte(configuration.JWTSigningKey))
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: configuration.SessionCookieName, Value: signed}
}

func imageGenerationTestClient(t *testing.T, upstream *httptest.Server, catalog *proxy.ProviderCatalog, provider string) llmproxyclient.Client {
	t.Helper()
	endpoints := proxy.NewEndpoints()
	endpoints.SetProviderBaseURL(proxy.ProviderNameOpenAI, upstream.URL)
	endpoints.SetProviderBaseURL(provider, upstream.URL)
	router, err := testfixtures.BuildManagedRouter(t, proxy.Configuration{ProviderCatalog: catalog, AssetStorePath: t.TempDir(), Endpoints: endpoints}, zap.NewNop().Sugar(), testfixtures.ManagedTenant{
		Secret: "image-acceptance-tenant", Defaults: proxy.TenantDefaults{Provider: proxy.ProviderNameOpenAI, Model: proxy.ModelNameGPT41},
		ProviderKeys: map[string]string{proxy.ProviderNameOpenAI: "image-provider-secret", provider: "image-provider-secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	configuration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "image-acceptance-tenant"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(configuration, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func imageGenerationTestIntent() llmproxyclient.ImageGenerationInput {
	return llmproxyclient.ImageGenerationInput{
		Surface: "images", Provider: "openai", Model: "gpt-image-2", Prompt: "A small lighthouse beside the sea", Quality: "low", Size: "1024x1024", Background: "opaque", OutputFormat: "png", OutputCount: 1}
}

func waitForImageGeneration(t *testing.T, client llmproxyclient.Client, operation llmproxyclient.MediaOperation) llmproxyclient.MediaOperation {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	result, err := client.WaitMediaOperation(ctx, operation.OperationID, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestImageGenerationRejectsUnsupportedControlsBeforeDispatch(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { calls.Add(1); writer.WriteHeader(500) }))
	defer upstream.Close()
	client := imageGenerationTestClient(t, upstream, testfixtures.ProviderCatalog(t), "openai")
	for name, change := range map[string]func(*llmproxyclient.ImageGenerationInput){
		"missing surface":          func(input *llmproxyclient.ImageGenerationInput) { input.Surface = "" },
		"unknown surface":          func(input *llmproxyclient.ImageGenerationInput) { input.Surface = "other" },
		"Images text model":        func(input *llmproxyclient.ImageGenerationInput) { input.ResponsesModel = "gpt-5" },
		"empty prompt":             func(input *llmproxyclient.ImageGenerationInput) { input.Prompt = "  " },
		"long prompt":              func(input *llmproxyclient.ImageGenerationInput) { input.Prompt = strings.Repeat("界", 32001) },
		"quality":                  func(input *llmproxyclient.ImageGenerationInput) { input.Quality = "ultra" },
		"missing quality":          func(input *llmproxyclient.ImageGenerationInput) { input.Quality = "" },
		"background":               func(input *llmproxyclient.ImageGenerationInput) { input.Background = "white" },
		"format":                   func(input *llmproxyclient.ImageGenerationInput) { input.OutputFormat = "gif" },
		"zero count":               func(input *llmproxyclient.ImageGenerationInput) { input.OutputCount = 0 },
		"high count":               func(input *llmproxyclient.ImageGenerationInput) { input.OutputCount = 11 },
		"dimension multiple":       func(input *llmproxyclient.ImageGenerationInput) { input.Size = "1025x1024" },
		"dimension limit":          func(input *llmproxyclient.ImageGenerationInput) { input.Size = "4096x1024" },
		"too few pixels":           func(input *llmproxyclient.ImageGenerationInput) { input.Size = "256x256" },
		"too many pixels":          func(input *llmproxyclient.ImageGenerationInput) { input.Size = "3840x3840" },
		"aspect ratio":             func(input *llmproxyclient.ImageGenerationInput) { input.Size = "3840x256" },
		"noncanonical size":        func(input *llmproxyclient.ImageGenerationInput) { input.Size = "01024x1024" },
		"invalid size":             func(input *llmproxyclient.ImageGenerationInput) { input.Size = "large" },
		"negative size":            func(input *llmproxyclient.ImageGenerationInput) { input.Size = "-1024x1024" },
		"png compression":          func(input *llmproxyclient.ImageGenerationInput) { input.OutputCompression = new(int) },
		"jpeg compression omitted": func(input *llmproxyclient.ImageGenerationInput) { input.OutputFormat = "jpeg" },
		"jpeg transparency": func(input *llmproxyclient.ImageGenerationInput) {
			input.OutputFormat = "jpeg"
			input.OutputCompression = new(int)
			input.Background = "transparent"
		},
		"compression negative": func(input *llmproxyclient.ImageGenerationInput) {
			input.OutputFormat = "webp"
			value := -1
			input.OutputCompression = &value
		},
		"compression high": func(input *llmproxyclient.ImageGenerationInput) {
			input.OutputFormat = "webp"
			value := 101
			input.OutputCompression = &value
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := imageGenerationTestIntent()
			change(&input)
			if _, err := client.CreateImageGeneration(t.Context(), "invalid-image", input); err == nil || !strings.Contains(err.Error(), "media_operation_invalid") {
				t.Fatalf("invalid intent accepted: %v", err)
			}
		})
	}
	for _, input := range []llmproxyclient.MediaOperationInput{
		{Capability: "image.generate", Provider: "openai", Model: "gpt-image-2", Input: json.RawMessage(`{"prompt":"test","unknown":true}`), Controls: json.RawMessage(`{}`)},
		{Capability: "image.generate", Provider: "openai", Model: "gpt-image-2", Input: json.RawMessage(`{"prompt":"test"}`), Controls: json.RawMessage(`{"unknown":true}`)},
		{Capability: "image.generate", Provider: "openai", Model: "gpt-image-2", Input: json.RawMessage(`{"prompt":"test","image_asset_ids":["ast_0123456789abcdef0123456789abcdef"]}`), Controls: json.RawMessage(`{"surface":"images","quality":"low","size":"auto","background":"opaque","output_format":"png","output_count":1}`)},
		{Capability: "image.generate", Provider: "openai", Model: "gpt-image-2", Input: json.RawMessage(`{"prompt":"test","mask_asset_id":"ast_0123456789abcdef0123456789abcdef"}`), Controls: json.RawMessage(`{"surface":"images","quality":"low","size":"auto","background":"opaque","output_format":"png","output_count":1}`)},
	} {
		if _, err := client.CreateMediaOperation(t.Context(), "invalid-raw-image", input); err == nil {
			t.Fatal("unknown request field accepted")
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid requests dispatched %d provider calls", calls.Load())
	}
}

func TestImageGenerationReportsProviderOutcomesWithoutResubmission(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		status int
		body   string
		state  string
		code   string
	}{
		{"rejected", 400, `{"error":{"message":"private upstream detail"}}`, proxy.MediaOperationStateFailed, "provider_error"},
		{"rate limited", 429, `{"error":{"message":"private rate limit detail"}}`, proxy.MediaOperationStateFailed, "provider_rate_limited"},
		{"server failure", 503, "", proxy.MediaOperationStateUncertain, "provider_outcome_unknown"},
		{"timeout", 408, "", proxy.MediaOperationStateUncertain, "provider_outcome_unknown"},
		{"lost response", 0, "", proxy.MediaOperationStateUncertain, "provider_outcome_unknown"},
		{"truncated body", -1, "", proxy.MediaOperationStateUncertain, "provider_outcome_unknown"},
		{"invalid json", 200, "{", proxy.MediaOperationStateFailed, "provider_result_invalid"},
		{"wrong count", 200, `{"data":[]}`, proxy.MediaOperationStateFailed, "provider_result_invalid"},
		{"invalid base64", 200, `{"data":[{"b64_json":"?"}]}`, proxy.MediaOperationStateFailed, "provider_result_invalid"},
		{"invalid image", 200, `{"data":[{"b64_json":"aW52YWxpZA=="}]}`, proxy.MediaOperationStateFailed, "provider_result_invalid"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				if scenario.status <= 0 {
					connection, buffer, err := writer.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					if scenario.status == -1 {
						_, _ = buffer.WriteString("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 200\r\n\r\n{\"data\":")
						_ = buffer.Flush()
					}
					_ = connection.Close()
					return
				}
				writer.WriteHeader(scenario.status)
				_, _ = writer.Write([]byte(scenario.body))
			}))
			defer upstream.Close()
			client := imageGenerationTestClient(t, upstream, testfixtures.ProviderCatalog(t), "openai")
			input := imageGenerationTestIntent()
			accepted, err := client.CreateImageGeneration(t.Context(), "provider-outcome", input)
			if err != nil {
				t.Fatal(err)
			}
			completed := waitForImageGeneration(t, client, accepted)
			if completed.State != scenario.state || completed.Error == nil || completed.Error.Code != scenario.code || len(completed.Outputs) != 0 {
				t.Fatalf("outcome=%+v", completed)
			}
			duplicate, err := client.CreateImageGeneration(t.Context(), "provider-outcome", input)
			if err != nil || duplicate.OperationID != accepted.OperationID || calls.Load() != 1 {
				t.Fatalf("duplicate=%+v error=%v calls=%d", duplicate, err, calls.Load())
			}
		})
	}
}

func TestImageGenerationSecondProviderUsesCatalogDataAndExplicitZeroCompression(t *testing.T) {
	const providerID = "image-fixture"
	schema := testfixtures.ProviderCatalog(t).Schema()
	for _, provider := range schema.Providers {
		if provider.ID != "openai" {
			continue
		}
		provider.ID = providerID
		provider.Label = "Image fixture"
		provider.APIServiceLabel = "Image fixture API"
		provider.Fields = append([]proxy.ProviderCatalogField(nil), provider.Fields...)
		provider.Fields[0].ID = "image_token"
		provider.Fields[0].Environment = ""
		provider.Transports = append([]proxy.ProviderCatalogTransport(nil), provider.Transports...)
		for index := range provider.Transports {
			provider.Transports[index].Components.Authentication = proxy.ProviderCatalogAuthentication{Kind: proxy.CatalogAuthenticationHeader, Field: "image_token", Header: "X-Image-Token"}
			if provider.Transports[index].ID == "image" {
				provider.Transports[index].Endpoint.Path = "/render"
			}
		}
		provider.Offerings = append([]proxy.ProviderCatalogOffering(nil), provider.Offerings...)
		for index := range provider.Offerings {
			if provider.Offerings[index].Model == "gpt-image-2" {
				provider.Offerings[index].UpstreamModel = "fixture-image-v1"
			}
		}
		schema.Providers = append(schema.Providers, provider)
		break
	}
	document, err := yaml.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := proxy.ParseProviderCatalog(document)
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 1024, 1024)), nil); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if request.URL.Path != "/render" || request.Header.Get("X-Image-Token") != "image-provider-secret" || request.Header.Get("Authorization") != "" || payload["model"] != "fixture-image-v1" || payload["output_compression"] != float64(0) || payload["output_format"] != "jpeg" || payload["size"] != "auto" {
			t.Errorf("catalog route lost data: path=%s payload=%v", request.URL.Path, payload)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(encoded.Bytes())}}})
	}))
	defer upstream.Close()
	client := imageGenerationTestClient(t, upstream, catalog, providerID)
	capabilities, err := client.GetMediaCapabilities(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, route := range capabilities.Routes {
		if route.Provider == providerID && route.Capability == "image.generate" && len(route.Controls) == 10 {
			found = true
		}
	}
	if !found {
		t.Fatal("second image provider absent from tenant discovery")
	}
	if _, err := client.GetPublicCapabilities(t.Context()); err != nil {
		t.Fatal(err)
	}
	input := imageGenerationTestIntent()
	input.Provider = providerID
	input.Size = "auto"
	input.OutputFormat = "jpeg"
	input.OutputCompression = new(int)
	accepted, err := client.CreateImageGeneration(t.Context(), "second-provider-image", input)
	if err != nil {
		t.Fatal(err)
	}
	completed := waitForImageGeneration(t, client, accepted)
	if completed.State != proxy.MediaOperationStateSucceeded || len(completed.Outputs) != 1 || completed.Outputs[0].MIMEType != "image/jpeg" || completed.Outputs[0].SizeBytes != int64(encoded.Len()) || calls.Load() != 1 {
		t.Fatalf("second provider result=%+v calls=%s", completed, strconv.Itoa(int(calls.Load())))
	}
}

func TestImageGenerationRestartReportsUncertainWithoutRepeatingPaidSubmission(t *testing.T) {
	var database *gorm.DB
	var calls atomic.Int32
	var output bytes.Buffer
	if err := png.Encode(&output, image.NewRGBA(image.Rect(0, 0, 1024, 1024))); err != nil {
		t.Fatal(err)
	}
	providerReturned := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		// Simulate a worker crash after provider dispatch but before committing its response.
		if err := database.Exec("UPDATE media_operation_claim_records SET worker_id = ?, generation = generation + 1, expires_at = ?", "crashed-image-worker", time.Now().Add(-time.Second)).Error; err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(output.Bytes())}}})
		close(providerReturned)
	}))
	defer upstream.Close()
	endpoints := proxy.NewEndpoints()
	endpoints.SetProviderBaseURL(proxy.ProviderNameOpenAI, upstream.URL)
	tenant := testfixtures.ManagedTenant{Secret: "image-restart-tenant", Defaults: proxy.TenantDefaults{Provider: "openai", Model: proxy.ModelNameGPT41}, ProviderKeys: map[string]string{"openai": "image-restart-provider"}}
	configuration, err := testfixtures.ProvisionManagedRouter(t, proxy.Configuration{
		ProviderCatalog: testfixtures.ProviderCatalog(t), AssetStorePath: t.TempDir(), Endpoints: endpoints,
		Management:                 proxy.ManagementConfiguration{DatabasePath: filepath.Join(t.TempDir(), "image-restart.sqlite")},
		MediaOperationClaimSeconds: 7200, MediaOperationClaimRenewalSeconds: 3600,
	}, zap.NewNop().Sugar(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	database, err = gorm.Open(configuration.Management.DatabaseDialector, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	newServer := func() (*httptest.Server, llmproxyclient.Client) {
		router, err := proxy.BuildRouter(configuration, zap.NewNop().Sugar())
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(router)
		clientConfiguration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: tenant.Secret})
		if err != nil {
			t.Fatal(err)
		}
		client, err := llmproxyclient.NewClient(clientConfiguration, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		return server, client
	}
	firstServer, firstClient := newServer()
	defer firstServer.Close()
	input := imageGenerationTestIntent()
	accepted, err := firstClient.CreateImageGeneration(t.Context(), "restart-image", input)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-providerReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("image provider was not called")
	}
	firstServer.Close()
	secondServer, secondClient := newServer()
	defer secondServer.Close()
	completed := waitForImageGeneration(t, secondClient, accepted)
	if completed.State != proxy.MediaOperationStateUncertain || completed.Error == nil || completed.Error.Code != "provider_outcome_unknown" || len(completed.Outputs) != 0 {
		t.Fatalf("recovered image=%+v", completed)
	}
	duplicate, err := secondClient.CreateImageGeneration(t.Context(), "restart-image", input)
	if err != nil || duplicate.OperationID != accepted.OperationID || calls.Load() != 1 {
		t.Fatalf("recovered duplicate=%+v error=%v calls=%d", duplicate, err, calls.Load())
	}
	for _, table := range []string{"media_operation_usage_delivery_records", "managed_usage_event_records"} {
		query := database.Table(table)
		if table == "media_operation_usage_delivery_records" {
			query = query.Where("operation_id = ?", accepted.OperationID)
		} else {
			query = query.Where("endpoint = ? AND model_id = ?", "media", "gpt-image-2")
		}
		var count int64
		if err := query.Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("%s count=%d error=%v", table, count, err)
		}
	}
}

func TestImageGenerationKeepsTextCapacityAndProvesCancellationAuthority(t *testing.T) {
	var output bytes.Buffer
	if err := png.Encode(&output, image.NewRGBA(image.Rect(0, 0, 1024, 1024))); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseImages := func() { releaseOnce.Do(func() { close(release) }) }
	var calls, active, maximum atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/responses" {
			_, _ = io.WriteString(writer, `{"id":"text","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"interactive image progress"}]}]}`)
			return
		}
		calls.Add(1)
		current := active.Add(1)
		for previous := maximum.Load(); current > previous; previous = maximum.Load() {
			if maximum.CompareAndSwap(previous, current) {
				break
			}
		}
		defer active.Add(-1)
		writer.WriteHeader(http.StatusOK)
		writer.(http.Flusher).Flush()
		started <- struct{}{}
		select {
		case <-release:
		case <-request.Context().Done():
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(output.Bytes())}}})
	}))
	defer upstream.Close()
	defer releaseImages()
	endpoints := proxy.NewEndpoints()
	endpoints.SetProviderBaseURL("openai", upstream.URL)
	router, err := testfixtures.BuildManagedRouter(t, proxy.Configuration{ProviderCatalog: testfixtures.ProviderCatalog(t), AssetStorePath: t.TempDir(), Endpoints: endpoints, UpstreamCapacity: testfixtures.UpstreamCapacity(2, 4), MediaOperationWorkers: 3}, zap.NewNop().Sugar(), testfixtures.ManagedTenant{
		Secret: "image-capacity-tenant", Defaults: proxy.TenantDefaults{Provider: "openai", Model: proxy.ModelNameGPT41}, ProviderKeys: map[string]string{"openai": "image-capacity-provider"},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	defer releaseImages()
	configuration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "image-capacity-tenant"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(configuration, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	operations := make([]llmproxyclient.MediaOperation, 0, 3)
	for index := 0; index < 3; index++ {
		accepted, err := client.CreateImageGeneration(t.Context(), "capacity-image-"+strconv.Itoa(index), imageGenerationTestIntent())
		if err != nil {
			t.Fatal(err)
		}
		operations = append(operations, accepted)
		deadline := time.Now().Add(time.Second)
		for {
			current, err := client.GetMediaOperation(t.Context(), accepted.OperationID)
			if err != nil {
				t.Fatal(err)
			}
			if current.State == proxy.MediaOperationStateRunning {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("image worker did not start")
			}
			time.Sleep(time.Millisecond)
		}
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("image provider did not start")
	}
	queued, err := client.CreateImageGeneration(t.Context(), "cancel-queued-image", imageGenerationTestIntent())
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := client.CancelMediaOperation(t.Context(), queued.OperationID)
	if err != nil || cancelled.State != proxy.MediaOperationStateCancelled || cancelled.CancellationState != proxy.MediaCancellationConfirmed {
		t.Fatalf("queued cancellation=%+v error=%v", cancelled, err)
	}
	running, err := client.CancelMediaOperation(t.Context(), operations[0].OperationID)
	if err != nil || running.State != proxy.MediaOperationStateRunning || running.CancellationState != proxy.MediaCancellationUnsupported {
		t.Fatalf("running cancellation=%+v error=%v", running, err)
	}
	textContext, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(textContext, http.MethodGet, server.URL+"/?key=image-capacity-tenant&prompt=text", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("text starved by image traffic: %v", err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || !bytes.Contains(body, []byte("interactive image progress")) {
		t.Fatalf("text response status=%d body=%s error=%v", response.StatusCode, body, err)
	}
	if calls.Load() != 1 || maximum.Load() != 1 {
		t.Fatalf("image capacity calls=%d maximum=%d", calls.Load(), maximum.Load())
	}
	releaseImages()
	for _, accepted := range operations {
		completed := waitForImageGeneration(t, client, accepted)
		if completed.State != proxy.MediaOperationStateSucceeded {
			t.Fatalf("image result=%+v", completed)
		}
	}
	if calls.Load() != 3 || maximum.Load() != 1 {
		t.Fatalf("image dispatches=%d maximum=%d", calls.Load(), maximum.Load())
	}
}

func TestImageGenerationVerifiesWebPAndRejectsCorruptedPNG(t *testing.T) {
	webpBytes, err := os.ReadFile(filepath.Join("..", "..", "tests", "testdata", "image-generation.webp"))
	if err != nil {
		t.Fatal(err)
	}
	var encodedPNG bytes.Buffer
	if err := png.Encode(&encodedPNG, image.NewRGBA(image.Rect(0, 0, 1024, 1024))); err != nil {
		t.Fatal(err)
	}
	corrupted := append([]byte(nil), encodedPNG.Bytes()...)
	corrupted[len(corrupted)-1] ^= 1
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload struct {
			OutputFormat string `json:"output_format"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		data := webpBytes
		if payload.OutputFormat == "png" {
			data = corrupted
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(data)}}})
	}))
	defer upstream.Close()
	client := imageGenerationTestClient(t, upstream, testfixtures.ProviderCatalog(t), "openai")
	input := imageGenerationTestIntent()
	input.OutputFormat = "webp"
	input.OutputCompression = new(int)
	accepted, err := client.CreateImageGeneration(t.Context(), "webp-image", input)
	if err != nil {
		t.Fatal(err)
	}
	completed := waitForImageGeneration(t, client, accepted)
	if completed.State != proxy.MediaOperationStateSucceeded || len(completed.Outputs) != 1 || completed.Outputs[0].MIMEType != "image/webp" {
		t.Fatalf("WebP result=%+v", completed)
	}
	asset, err := client.GetAsset(t.Context(), completed.Outputs[0].AssetID)
	if err != nil {
		t.Fatal(err)
	}
	downloaded, err := client.DownloadAsset(t.Context(), asset)
	if err != nil || !bytes.Equal(downloaded, webpBytes) {
		t.Fatalf("WebP bytes differ: %v", err)
	}
	accepted, err = client.CreateImageGeneration(t.Context(), "corrupted-png", imageGenerationTestIntent())
	if err != nil {
		t.Fatal(err)
	}
	completed = waitForImageGeneration(t, client, accepted)
	if completed.State != proxy.MediaOperationStateFailed || completed.Error == nil || completed.Error.Code != "provider_result_invalid" || len(completed.Outputs) != 0 {
		t.Fatalf("corrupted PNG result=%+v", completed)
	}
}

func TestImageGenerationRejectsChangedOrCorruptedAcceptedCredentials(t *testing.T) {
	for name, mutation := range map[string]string{
		"changed connection":   "UPDATE managed_account_connection_records SET version = version + 1 WHERE provider_id = 'openai'",
		"corrupted credential": "UPDATE managed_connection_field_records SET value = 'corrupted' WHERE field_id = 'api_key'",
		"changed at dispatch":  "CREATE TRIGGER image_credential_change AFTER UPDATE OF provider_execution_state ON media_operation_records WHEN NEW.operation_id = '{operation}' AND NEW.provider_execution_state = 'dispatched' BEGIN UPDATE managed_account_connection_records SET version = version + 1 WHERE provider_id = 'openai'; END",
	} {
		t.Run(name, func(t *testing.T) {
			started, release := make(chan struct{}, 1), make(chan struct{})
			var releaseOnce sync.Once
			releaseProvider := func() { releaseOnce.Do(func() { close(release) }) }
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				started <- struct{}{}
				select {
				case <-release:
				case <-request.Context().Done():
					return
				}
				writer.WriteHeader(http.StatusBadRequest)
			}))
			defer upstream.Close()
			defer releaseProvider()
			endpoints := proxy.NewEndpoints()
			endpoints.SetProviderBaseURL("openai", upstream.URL)
			configuration, err := testfixtures.ProvisionManagedRouter(t, proxy.Configuration{ProviderCatalog: testfixtures.ProviderCatalog(t), AssetStorePath: t.TempDir(), Endpoints: endpoints, MediaOperationWorkers: 1}, zap.NewNop().Sugar(), testfixtures.ManagedTenant{
				Secret: "changed-image-credentials", Defaults: proxy.TenantDefaults{Provider: "openai", Model: proxy.ModelNameGPT41}, ProviderKeys: map[string]string{"openai": "original-image-key"},
			})
			if err != nil {
				t.Fatal(err)
			}
			router, err := proxy.BuildRouter(configuration, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			defer releaseProvider()
			clientConfiguration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "changed-image-credentials"})
			if err != nil {
				t.Fatal(err)
			}
			client, err := llmproxyclient.NewClient(clientConfiguration, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			first, err := client.CreateImageGeneration(t.Context(), "hold-image-worker", imageGenerationTestIntent())
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("image worker did not start")
			}
			queued, err := client.CreateImageGeneration(t.Context(), "changed-credential-image", imageGenerationTestIntent())
			if err != nil {
				t.Fatal(err)
			}
			database, err := gorm.Open(configuration.Management.DatabaseDialector, &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			var savedFields []struct {
				ConnectionID string
				FieldID      string
				Value        string
			}
			if err := database.Table("managed_connection_field_records").Find(&savedFields).Error; err != nil {
				t.Fatal(err)
			}
			if err := database.Exec(strings.ReplaceAll(mutation, "{operation}", queued.OperationID)).Error; err != nil {
				t.Fatal(err)
			}
			releaseProvider()
			// Credential corruption prevents fresh tenant resolution. Restore it after
			// the real worker has recorded the rejection, before reading via HTTP.
			deadline := time.Now().Add(2 * time.Second)
			for {
				var record struct{ PublicState string }
				if err := database.Table("media_operation_records").Select("public_state").Where("operation_id = ?", queued.OperationID).Scan(&record).Error; err != nil {
					t.Fatal(err)
				}
				if record.PublicState == proxy.MediaOperationStateFailed {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("credential rejection state=%s", record.PublicState)
				}
				time.Sleep(time.Millisecond)
			}
			for _, field := range savedFields {
				if err := database.Table("managed_connection_field_records").Where("connection_id = ? AND field_id = ?", field.ConnectionID, field.FieldID).Update("value", field.Value).Error; err != nil {
					t.Fatal(err)
				}
			}
			completed := waitForImageGeneration(t, client, queued)
			if completed.State != proxy.MediaOperationStateFailed || completed.Error == nil || completed.Error.Code != "media_operation_unavailable" || calls.Load() != 1 {
				t.Fatalf("credential result=%+v calls=%d", completed, calls.Load())
			}
			waitForImageGeneration(t, client, first)
		})
	}
}
