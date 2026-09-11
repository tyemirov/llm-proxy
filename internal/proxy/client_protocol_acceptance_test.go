package proxy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"go.uber.org/zap"
)

const protocolFixtureProviderSecret = "provider-fixture-secret"

func TestClientProtocolsSharedAdapterCatalog(t *testing.T) {
	catalog, fixtures := testfixtures.ProviderCatalogWithProtocolFixtures(t)
	expected := map[string]bool{
		protocolFixtureKey(proxy.CatalogProtocolOpenAIResponses, "pollable_resource", proxy.ModelOperationText):                  true,
		protocolFixtureKey(proxy.CatalogProtocolDashScopeResponses, "synchronous_completion", proxy.ModelOperationText):          true,
		protocolFixtureKey(proxy.CatalogProtocolXAIResponses, "synchronous_completion", proxy.ModelOperationText):                true,
		protocolFixtureKey(proxy.CatalogProtocolOpenAIChatCompletions, "synchronous_completion", proxy.ModelOperationText):       true,
		protocolFixtureKey(proxy.CatalogProtocolGeminiInteractions, "pollable_resource", proxy.ModelOperationText):               true,
		protocolFixtureKey(proxy.CatalogProtocolGeminiInteractions, "synchronous_completion", proxy.ModelOperationText):          true,
		protocolFixtureKey(proxy.CatalogProtocolAnthropicMessages, "synchronous_completion", proxy.ModelOperationText):           true,
		protocolFixtureKey(proxy.CatalogProtocolVertexGenerateContent, "synchronous_completion", proxy.ModelOperationText):       true,
		protocolFixtureKey(proxy.CatalogProtocolMultipartTranscription, "synchronous_completion", proxy.ModelOperationDictation): true,
		protocolFixtureKey(proxy.CatalogProtocolMetaTranscription, "synchronous_completion", proxy.ModelOperationDictation):      true,
		protocolFixtureKey(proxy.CatalogProtocolGeminiInteractions, "synchronous_completion", proxy.ModelOperationDictation):     true,
		protocolFixtureKey(proxy.CatalogProtocolVertexGenerateContent, "synchronous_completion", proxy.ModelOperationDictation):  true,
	}
	providersByProtocol := map[string]map[string]bool{}
	for _, provider := range catalog.Schema().Providers {
		for _, transport := range provider.Transports {
			if providersByProtocol[transport.RequestProtocol] == nil {
				providersByProtocol[transport.RequestProtocol] = map[string]bool{}
			}
			providersByProtocol[transport.RequestProtocol][provider.ID] = true
		}
	}
	for _, fixture := range fixtures {
		if fixture.Protocol == "" || fixture.Provider == "" || fixture.Model == "" || fixture.Operation == "" ||
			fixture.Lifecycle == "" || fixture.EndpointPath == "" || fixture.AuthenticationHeader == "" {
			t.Fatalf("incomplete protocol fixture: %+v", fixture)
		}
		providers := providersByProtocol[fixture.Protocol]
		if len(providers) < 2 || !providers[fixture.Provider] {
			t.Fatalf("protocol=%s providers=%v fixture=%s", fixture.Protocol, providers, fixture.Provider)
		}
		key := protocolFixtureKey(fixture.Protocol, fixture.Lifecycle, fixture.Operation)
		if !expected[key] {
			t.Fatalf("unexpected executable adapter fixture: %+v", fixture)
		}
		delete(expected, key)
	}
	if len(expected) != 0 {
		t.Fatalf("missing executable adapter fixtures: %v", expected)
	}
}

func protocolFixtureKey(protocol string, lifecycle string, operation string) string {
	return strings.Join([]string{protocol, lifecycle, operation}, "/")
}

func TestClientProtocolsSharedAdapterExecution(t *testing.T) {
	catalog, fixtures := testfixtures.ProviderCatalogWithProtocolFixtures(t)
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(strings.Join([]string{fixture.Protocol, fixture.Lifecycle, fixture.Operation}, "/"), func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				body, readError := io.ReadAll(request.Body)
				if readError != nil {
					t.Fatal(readError)
				}
				if !strings.HasPrefix(request.URL.Path, fixture.EndpointPath) {
					t.Errorf("path=%s want_prefix=%s", request.URL.Path, fixture.EndpointPath)
				}
				if request.Header.Get(fixture.AuthenticationHeader) != fixture.AuthenticationPrefix+protocolFixtureProviderSecret {
					t.Errorf("provider authentication header=%s", fixture.AuthenticationHeader)
				}
				if bytes.Contains(body, []byte(protocolFixtureTenantSecret(fixture))) {
					t.Error("tenant credential reached provider")
				}
				writeProtocolFixtureResponse(t, responseWriter, fixture)
			}))
			defer upstream.Close()

			endpoints := proxy.NewEndpoints()
			endpoints.SetProviderBaseURL(fixture.Provider, upstream.URL)
			tenantSecret := protocolFixtureTenantSecret(fixture)
			defaults := proxy.TenantDefaults{}
			textModels := map[string]string{}
			if fixture.Operation == proxy.ModelOperationText {
				defaults.Provider = fixture.Provider
				defaults.Model = fixture.Model
				textModels[fixture.Provider] = fixture.Model
			} else {
				defaults.DictationProvider = fixture.Provider
				defaults.DictationModel = fixture.Model
			}
			tenant := proxy.ManagedTenantTestConfiguration{
				ID: fixture.Provider, Secret: tenantSecret, Defaults: defaults,
				ProviderKeys:       map[string]string{fixture.Provider: protocolFixtureProviderSecret},
				ProviderTextModels: textModels,
			}
			router, routerError := buildRouterWithManagedTenant(t, proxy.Configuration{
				ProviderCatalog: catalog, Endpoints: endpoints, AssetStorePath: t.TempDir(),
			}, zap.NewNop().Sugar(), tenant)
			if routerError != nil {
				t.Fatal(routerError)
			}
			server := httptest.NewServer(router)
			defer server.Close()

			clientConfiguration, configurationError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
				Protocol: llmproxyclient.ProtocolOpenAIResponses, BaseURL: server.URL,
				Secret: tenantSecret, Provider: fixture.Provider,
			})
			if configurationError != nil {
				t.Fatal(configurationError)
			}
			client, clientError := llmproxyclient.NewClient(clientConfiguration, server.Client())
			if clientError != nil {
				t.Fatal(clientError)
			}
			capabilities, capabilitiesError := client.GetPublicCapabilities(context.Background())
			if capabilitiesError != nil {
				t.Fatal(capabilitiesError)
			}
			assertProtocolFixtureDiscovered(t, capabilities, fixture)
			if fixture.Operation == proxy.ModelOperationText {
				exerciseProtocolFixtureText(t, client, fixture)
			} else {
				exerciseProtocolFixtureDictation(t, server, tenantSecret, fixture)
			}
			expectedCalls := int32(1)
			if fixture.Protocol == proxy.CatalogProtocolGeminiInteractions && fixture.Lifecycle == "pollable_resource" {
				expectedCalls = 2
			}
			if calls.Load() != expectedCalls {
				t.Fatalf("provider calls=%d want=%d", calls.Load(), expectedCalls)
			}
		})
	}
}

func TestClientProtocolsQualificationFaults(t *testing.T) {
	catalog, fixtures := testfixtures.ProviderCatalogWithProtocolFixtures(t)
	fixture := findProtocolFixture(t, fixtures, proxy.CatalogProtocolOpenAIChatCompletions, proxy.ModelOperationText)
	for _, scenario := range []struct {
		name       string
		baseSuffix string
		response   string
	}{
		{name: "wrong endpoint", baseSuffix: "/wrong", response: `{"choices":[{"message":{"content":"fixture result"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`},
		{name: "malformed result", response: `{}`},
		{name: "incorrect usage total", response: `{"choices":[{"message":{"content":"fixture result"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":6}}`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				responseWriter.Header().Set("Content-Type", "application/json")
				if request.URL.Path != fixture.EndpointPath {
					http.NotFound(responseWriter, request)
					return
				}
				_, _ = io.WriteString(responseWriter, scenario.response)
			}))
			defer upstream.Close()
			server, client := newProtocolFixtureTextClient(t, catalog, fixture, upstream.URL+scenario.baseSuffix)
			defer server.Close()
			request, requestError := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{
				Model: fixture.Model, Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "fixture request"}},
			})
			if requestError != nil {
				t.Fatal(requestError)
			}
			result, completionError := client.PostMessagesCompletion(context.Background(), request)
			if completionError == nil {
				completionError = validateProtocolFixtureCompletion(result, fixture)
			}
			if completionError == nil {
				t.Fatal("qualification fault was accepted")
			}
			if calls.Load() != 1 {
				t.Fatalf("provider calls=%d want=1", calls.Load())
			}
		})
	}

	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		responseWriter.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server, _ := newProtocolFixtureTextClient(t, catalog, fixture, upstream.URL)
	defer server.Close()
	for _, scenario := range []struct {
		name, credential, body string
		status                 int
	}{
		{name: "unsupported field", credential: protocolFixtureTenantSecret(fixture), body: `{"model":"` + fixture.Provider + `/` + fixture.Model + `","input":"fixture request","temperature":0.5}`, status: http.StatusBadRequest},
		{name: "invalid credential", credential: "invalid", body: `{"model":"` + fixture.Provider + `/` + fixture.Model + `","input":"fixture request"}`, status: http.StatusUnauthorized},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			request, requestError := http.NewRequest(http.MethodPost, server.URL+"/v1/responses", strings.NewReader(scenario.body))
			if requestError != nil {
				t.Fatal(requestError)
			}
			request.Header.Set("Authorization", "Bearer "+scenario.credential)
			request.Header.Set("Content-Type", "application/json")
			response, responseError := server.Client().Do(request)
			if responseError != nil {
				t.Fatal(responseError)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode != scenario.status {
				t.Fatalf("status=%d want=%d", response.StatusCode, scenario.status)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("rejected requests dispatched %d provider calls", calls.Load())
	}
}

func findProtocolFixture(t *testing.T, fixtures []testfixtures.ProtocolFixture, protocol string, operation string) testfixtures.ProtocolFixture {
	t.Helper()
	for _, fixture := range fixtures {
		if fixture.Protocol == protocol && fixture.Operation == operation {
			return fixture
		}
	}
	t.Fatalf("missing fixture protocol=%s operation=%s", protocol, operation)
	return testfixtures.ProtocolFixture{}
}

func newProtocolFixtureTextClient(t *testing.T, catalog *proxy.ProviderCatalog, fixture testfixtures.ProtocolFixture, upstreamBaseURL string) (*httptest.Server, llmproxyclient.Client) {
	t.Helper()
	endpoints := proxy.NewEndpoints()
	endpoints.SetProviderBaseURL(fixture.Provider, upstreamBaseURL)
	tenantSecret := protocolFixtureTenantSecret(fixture)
	tenant := proxy.ManagedTenantTestConfiguration{
		ID: fixture.Provider, Secret: tenantSecret,
		Defaults:           proxy.TenantDefaults{Provider: fixture.Provider, Model: fixture.Model},
		ProviderKeys:       map[string]string{fixture.Provider: protocolFixtureProviderSecret},
		ProviderTextModels: map[string]string{fixture.Provider: fixture.Model},
	}
	router, routerError := buildRouterWithManagedTenant(t, proxy.Configuration{ProviderCatalog: catalog, Endpoints: endpoints}, zap.NewNop().Sugar(), tenant)
	if routerError != nil {
		t.Fatal(routerError)
	}
	server := httptest.NewServer(router)
	configuration, configurationError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		Protocol: llmproxyclient.ProtocolOpenAIResponses, BaseURL: server.URL,
		Secret: tenantSecret, Provider: fixture.Provider,
	})
	if configurationError != nil {
		server.Close()
		t.Fatal(configurationError)
	}
	client, clientError := llmproxyclient.NewClient(configuration, server.Client())
	if clientError != nil {
		server.Close()
		t.Fatal(clientError)
	}
	return server, client
}

func protocolFixtureTenantSecret(fixture testfixtures.ProtocolFixture) string {
	return "tenant-" + fixture.Provider
}

func writeProtocolFixtureResponse(t *testing.T, responseWriter http.ResponseWriter, fixture testfixtures.ProtocolFixture) {
	t.Helper()
	responseWriter.Header().Set("Content-Type", "application/json")
	switch fixture.Protocol {
	case proxy.CatalogProtocolOpenAIResponses, proxy.CatalogProtocolDashScopeResponses, proxy.CatalogProtocolXAIResponses:
		_, _ = io.WriteString(responseWriter, `{"id":"private-response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"fixture result"}]}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`)
	case proxy.CatalogProtocolOpenAIChatCompletions:
		_, _ = io.WriteString(responseWriter, `{"id":"private-chat","choices":[{"message":{"content":"fixture result"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`)
	case proxy.CatalogProtocolAnthropicMessages:
		_, _ = io.WriteString(responseWriter, `{"id":"private-message","content":[{"type":"text","text":"fixture result"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3}}`)
	case proxy.CatalogProtocolVertexGenerateContent:
		_, _ = io.WriteString(responseWriter, `{"candidates":[{"content":{"parts":[{"text":"fixture result"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3,"thoughtsTokenCount":0,"totalTokenCount":5}}`)
	case proxy.CatalogProtocolGeminiInteractions:
		writeGeminiInteractionSnapshot(t, responseWriter, "private-interaction", "completed", "fixture result", &testGeminiInteractionUsage{Input: 2, Output: 3, Total: 5})
	case proxy.CatalogProtocolMultipartTranscription:
		_, _ = io.WriteString(responseWriter, `{"text":"fixture result","id":"private-transcription"}`)
	case proxy.CatalogProtocolMetaTranscription:
		_, _ = io.WriteString(responseWriter, `{"transcript":"fixture result","sessionId":"private-transcription"}`)
	default:
		t.Fatalf("missing protocol response fixture: %s", fixture.Protocol)
	}
}

func assertProtocolFixtureDiscovered(t *testing.T, catalog llmproxyclient.PublicCapabilityCatalog, fixture testfixtures.ProtocolFixture) {
	t.Helper()
	for _, offering := range catalog.Offerings {
		if offering.Provider == fixture.Provider && offering.Model == fixture.Model &&
			offering.WireContract == fixture.Protocol && offering.ExecutionLifecycle == fixture.Lifecycle {
			return
		}
	}
	t.Fatalf("fixture route is absent from public capabilities: %+v", fixture)
}

func exerciseProtocolFixtureText(t *testing.T, client llmproxyclient.Client, fixture testfixtures.ProtocolFixture) {
	t.Helper()
	request, requestError := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{
		Model: fixture.Model, Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "fixture request"}},
	})
	if requestError != nil {
		t.Fatal(requestError)
	}
	result, completionError := client.PostMessagesCompletion(context.Background(), request)
	if completionError != nil {
		t.Fatal(completionError)
	}
	if validationError := validateProtocolFixtureCompletion(result, fixture); validationError != nil {
		t.Fatal(validationError)
	}
}

func validateProtocolFixtureCompletion(result llmproxyclient.CompletionResult, fixture testfixtures.ProtocolFixture) error {
	usage := result.Usage()
	if result.Text() != "fixture result" || result.ResolvedModel() != fixture.Provider+"/"+fixture.Model ||
		usage == nil || usage.InputTokens() != 2 || usage.OutputTokens() != 3 || usage.TotalTokens() != 5 {
		return fmt.Errorf("completion text=%q model=%q usage=%+v", result.Text(), result.ResolvedModel(), usage)
	}
	return nil
}

func exerciseProtocolFixtureDictation(t *testing.T, server *httptest.Server, tenantSecret string, fixture testfixtures.ProtocolFixture) {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if fieldError := form.WriteField("model", fixture.Provider+"/"+fixture.Model); fieldError != nil {
		t.Fatal(fieldError)
	}
	file, fileError := form.CreateFormFile("file", "fixture.wav")
	if fileError != nil {
		t.Fatal(fileError)
	}
	if _, writeError := file.Write(metaTranscriptionWAV(16000, 1)); writeError != nil {
		t.Fatal(writeError)
	}
	if closeError := form.Close(); closeError != nil {
		t.Fatal(closeError)
	}
	request, requestError := http.NewRequest(http.MethodPost, server.URL+"/v1/audio/transcriptions", &body)
	if requestError != nil {
		t.Fatal(requestError)
	}
	request.Header.Set("Authorization", "Bearer "+tenantSecret)
	request.Header.Set("Content-Type", form.FormDataContentType())
	response, responseError := server.Client().Do(request)
	if responseError != nil {
		t.Fatal(responseError)
	}
	defer response.Body.Close()
	responseBody, readError := io.ReadAll(response.Body)
	if readError != nil {
		t.Fatal(readError)
	}
	var result struct {
		Text string `json:"text"`
	}
	if decodeError := json.Unmarshal(responseBody, &result); decodeError != nil || response.StatusCode != http.StatusOK || result.Text != "fixture result" || bytes.Contains(responseBody, []byte("private")) {
		t.Fatalf("dictation status=%d body=%s error=%v", response.StatusCode, responseBody, decodeError)
	}
}
