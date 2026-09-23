package proxy

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestHostedTextIdentityReplaysOneExecution(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Header.Get("Authorization") != "Bearer sk-platform-pinned" {
			t.Error("dispatch did not use the accepted platform credential")
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"id":"private-replay","status":"completed","output_text":"retained answer","usage":{"input_tokens":10,"output_tokens":3,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	t.Cleanup(upstream.Close)
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir())
	first := hostedIdentityHTTP(t, server, "repeat", "private prompt", http.StatusOK)
	second := hostedIdentityHTTP(t, server, "repeat", "private prompt", http.StatusOK)
	if first != "retained answer" || second != first || calls.Load() != 1 {
		t.Fatalf("replay: first=%q second=%q calls=%d", first, second, calls.Load())
	}
	hostedIdentityHTTP(t, server, "repeat", "changed prompt", http.StatusConflict)
	if calls.Load() != 1 {
		t.Fatalf("changed intent dispatched: %d", calls.Load())
	}
	response := read("")
	items := response["requests"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["state"] != "completed" {
		t.Fatalf("journal identities=%v", response)
	}
	status := hostedIdentityStatusHTTP(t, server, "repeat", http.StatusOK)
	var stored map[string]any
	if err := json.Unmarshal([]byte(status), &stored); err != nil {
		t.Fatal(err)
	}
	if stored["text"] != "retained answer" {
		t.Fatalf("retained result resource=%s", status)
	}
}

func TestHostedTextIdentitySurvivesRevocationAndResponseExpiry(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"id":"private-expiry","status":"completed","output_text":"saved result","usage":{"input_tokens":10,"output_tokens":3,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	t.Cleanup(upstream.Close)
	var responses *structuredRequestStore
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), func(service *hostedTextRequestDependencies) { responses = service.responses })
	hostedIdentityHTTP(t, server, "expiry", "private prompt", http.StatusOK)
	if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-journal").Update("state", hostedGrantRevoked).Error; err != nil {
		t.Fatal(err)
	}
	hostedIdentityHTTP(t, server, "expiry", "private prompt", http.StatusOK)
	hostedIdentityHTTP(t, server, "fresh", "private prompt", http.StatusForbidden)
	expiredAt := time.Now().Add(2 * time.Minute)
	responses.now = func() time.Time { return expiredAt }
	hostedIdentityHTTP(t, server, "expiry", "private prompt", http.StatusGone)
	hostedIdentityStatusHTTP(t, server, "expiry", http.StatusGone)
	if calls.Load() != 1 {
		t.Fatalf("expiry or revocation dispatched=%d", calls.Load())
	}
	if entries := read("")["requests"].([]any); len(entries) != 1 || entries[0].(map[string]any)["state"] != "completed" {
		t.Fatalf("expiry deleted journal: %v", entries)
	}
}

func TestHostedTextIdentityLostResponseCannotRetryPaidWork(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		connection, _, err := writer.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		if err := connection.Close(); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(upstream.Close)
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir())
	hostedIdentityHTTP(t, server, "lost", "private prompt", http.StatusBadGateway)
	hostedIdentityHTTP(t, server, "lost", "private prompt", http.StatusConflict)
	hostedIdentityStatusHTTP(t, server, "lost", http.StatusConflict)
	if calls.Load() != 1 {
		t.Fatalf("uncertain request repeated paid work: %d", calls.Load())
	}
}

func TestHostedTextIdentityConcurrentInstancesAndRestart(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	secondDatabase := openJournalTransactionInstance(t, database)
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"id":"private-concurrent","status":"completed","output_text":"one result","usage":{"input_tokens":10,"output_tokens":3,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	t.Cleanup(upstream.Close)
	root := t.TempDir()
	first := newHostedIdentityHTTPServer(t, database, upstream.URL, root)
	second := newHostedIdentityHTTPServer(t, secondDatabase, upstream.URL, root)
	t.Cleanup(unblock)
	request, err := http.NewRequest(http.MethodPost, first.URL+"/?provider=openai&model=gpt-4.1", strings.NewReader(`{"prompt":"concurrent prompt"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "concurrent")
	result := make(chan *http.Response, 1)
	failures := make(chan error, 1)
	go func() {
		response, err := first.Client().Do(request)
		result <- response
		failures <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not start")
	}
	pending := hostedIdentityHTTP(t, second, "concurrent", "concurrent prompt", http.StatusAccepted)
	if !strings.Contains(pending, `"state":"dispatched"`) {
		t.Fatalf("concurrent state=%s", pending)
	}
	hostedIdentityStatusHTTP(t, second, "concurrent", http.StatusAccepted)
	if calls.Load() != 1 {
		t.Fatalf("concurrent dispatches=%d", calls.Load())
	}
	journal := read("")["requests"].([]any)[0].(map[string]any)
	if !strings.Contains(pending, fmt.Sprintf(`"proxy_request_id":%q`, journal["execution_id"])) {
		t.Fatalf("request identity changed: %s %v", pending, journal)
	}
	unblock()
	response := <-result
	if err := <-failures; err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("first status=%d", response.StatusCode)
	}
	first.Close()
	second.Close()
	restarted := newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, database), upstream.URL, root)
	if result := hostedIdentityHTTP(t, restarted, "concurrent", "concurrent prompt", http.StatusOK); result != "one result" {
		t.Fatalf("restart result=%q", result)
	}
	if calls.Load() != 1 {
		t.Fatalf("restart dispatched=%d", calls.Load())
	}
}

func TestHostedTextIdentityMissingKeyDoesNotAdmit(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	server := newHostedIdentityHTTPServer(t, database, "http://unused.invalid", t.TempDir())
	hostedIdentityHTTP(t, server, "", "private prompt", http.StatusBadRequest)
	if items := read("")["requests"].([]any); len(items) != 0 {
		t.Fatalf("missing key created requests=%v", items)
	}
}

func TestHostedTextIdentityPinsCredentialAcrossRotation(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Header.Get("Authorization") != "Bearer sk-platform-pinned" {
			t.Error("rotation changed the accepted credential")
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"id":"private-rotation","status":"completed","output_text":"pinned result","usage":{"input_tokens":10,"output_tokens":3,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	t.Cleanup(upstream.Close)
	var rotated atomic.Bool
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), func(service *hostedTextRequestDependencies) {
		service.authorize = func(transaction *gorm.DB, record managedJournalRequestRecord) error {
			if rotated.Load() {
				return nil
			}
			key, err := service.cipher.encryptConnection(rand.Reader, platformCredentialReference(record.PlatformConnectionID, 2), record.Provider, CatalogCredentialAPIKey, "sk-platform-next")
			if err != nil {
				return err
			}
			fields, err := json.Marshal(map[string]string{CatalogCredentialAPIKey: key})
			if err != nil {
				return err
			}
			credential := managedPlatformCredentialRecord{ConnectionID: record.PlatformConnectionID, Version: 2, Fields: fields, QualifiedAt: record.CreatedAt, CreatedAt: record.CreatedAt}
			if err := transaction.Omit(clause.Associations).Create(&credential).Error; err != nil {
				return err
			}
			if err := transaction.Model(&managedPlatformConnectionRecord{}).Where("id = ?", record.PlatformConnectionID).Update("version", 2).Error; err != nil {
				return err
			}
			rotated.Store(true)
			return nil
		}
	})
	if body := hostedIdentityHTTP(t, server, "rotation", "private prompt", http.StatusOK); body != "pinned result" {
		t.Fatalf("rotation response=%q", body)
	}
	if !rotated.Load() || calls.Load() != 1 {
		t.Fatalf("rotation=%t calls=%d", rotated.Load(), calls.Load())
	}
}

func TestHostedTextIdentityIsSharedAcrossHTTPProtocols(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"id":"private-protocols","status":"completed","output_text":"shared result","usage":{"input_tokens":10,"output_tokens":3,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	t.Cleanup(upstream.Close)
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir())
	hostedIdentityHTTP(t, server, "protocols", "private prompt", http.StatusOK)
	journal := read("")["requests"].([]any)[0].(map[string]any)
	for _, scenario := range []struct{ path, body string }{
		{v2Path + "?provider=openai&model=gpt-4.1", `{"messages":[{"role":"user","content":"private prompt"}]}`},
		{chatCompletionsPath, `{"model":"openai/gpt-4.1","messages":[{"role":"user","content":"private prompt"}]}`},
		{responsesPath, `{"model":"openai/gpt-4.1","input":"private prompt"}`},
	} {
		request, err := http.NewRequest(http.MethodPost, server.URL+scenario.path, strings.NewReader(scenario.body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "protocols")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "shared result") {
			t.Fatalf("protocol=%s status=%d body=%s", scenario.path, response.StatusCode, body)
		}
		validateHostedIdentityResponse(t, request, response, body)
		if scenario.path == chatCompletionsPath || scenario.path == responsesPath {
			var result map[string]any
			if err := json.Unmarshal(body, &result); err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(result["id"].(string), journal["execution_id"].(string)) {
				t.Fatalf("protocol replay changed execution identity: %v journal=%v", result, journal)
			}
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("protocol replay dispatched=%d", calls.Load())
	}
}

func TestHostedTextIdentityPreservesStructuredJSON(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"id":"private-structured","status":"completed","output_text":"{\"answer\":\"saved JSON\"}","usage":{"input_tokens":10,"output_tokens":3,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	t.Cleanup(upstream.Close)
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir())
	for range 2 {
		request, err := http.NewRequest(http.MethodPost, server.URL+v2Path+"?provider=openai&model=gpt-4.1", strings.NewReader(`{"messages":[{"role":"user","content":"private prompt"}],"structured_output":{"schema":{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}}}`))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json")
		request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "structured")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err := json.Unmarshal(body, &value); err != nil {
			t.Fatalf("status=%d body=%s error=%v", response.StatusCode, body, err)
		}
		if response.StatusCode != http.StatusOK || value["answer"] != "saved JSON" || !strings.Contains(response.Header.Get("Content-Type"), "application/json") {
			t.Fatalf("structured result changed: status=%d type=%s body=%s", response.StatusCode, response.Header.Get("Content-Type"), body)
		}
		validateHostedIdentityResponse(t, request, response, body)
	}
	if calls.Load() != 1 {
		t.Fatalf("structured replay dispatched=%d", calls.Load())
	}
}

func hostedIdentityStatusHTTP(t *testing.T, server *httptest.Server, key string, want int) string {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, server.URL+llmproxycontract.StructuredRequestPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(llmproxycontract.HeaderIdempotencyKey, key)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != want || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d want=%d cache=%q body=%s", response.StatusCode, want, response.Header.Get("Cache-Control"), body)
	}
	validateHostedIdentityResponse(t, request, response, body)
	return string(body)
}

func hostedIdentityHTTP(t *testing.T, server *httptest.Server, key, prompt string, want int) string {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, server.URL+"/?provider=openai&model=gpt-4.1", strings.NewReader(fmt.Sprintf(`{"prompt":%q}`, prompt)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if key != "" {
		request.Header.Set(llmproxycontract.HeaderIdempotencyKey, key)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != want {
		t.Fatalf("status=%d want=%d body=%s", response.StatusCode, want, body)
	}
	validateHostedIdentityResponse(t, request, response, body)
	return strings.TrimSpace(string(body))
}

func validateHostedIdentityResponse(t *testing.T, request *http.Request, response *http.Response, body []byte) {
	t.Helper()
	contract, err := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if err != nil {
		t.Fatal(err)
	}
	if err := contract.ValidateResponse(request.URL.Path, request.Method, response.StatusCode, response.Header, body); err != nil {
		t.Fatalf("hosted response contract: %s %s status=%d body=%s error=%v", request.Method, request.URL.Path, response.StatusCode, body, err)
	}
}

func newHostedIdentityHTTPServer(t *testing.T, database *gormManagedTenantDatabase, upstreamURL, responseRoot string, configure ...func(*hostedTextRequestDependencies)) *httptest.Server {
	t.Helper()
	router, _, _ := newHostedIdentityHTTPHandler(t, database, upstreamURL, responseRoot, configure...)
	server := httptest.NewServer(router)
	server.Client().Transport = hostedIdentityTransport{next: server.Client().Transport}
	t.Cleanup(server.Close)
	return server
}

func newHostedIdentityHTTPHandler(t *testing.T, database *gormManagedTenantDatabase, upstreamURL, responseRoot string, configure ...func(*hostedTextRequestDependencies)) (*gin.Engine, *managementService, *providerRouter) {
	t.Helper()
	providers := internalManagementProviderRegistry()
	definition := providers.definitions[providerID("openai")]
	for identifier, transport := range definition.transports {
		transport.endpointURLOverride = upstreamURL
		definition.transports[identifier] = transport
	}
	providers.definitions[providerID("openai")] = definition
	service := newInternalManagementService(t, newFakeManagedTenantDatabase(), providers)
	service.store.database = database
	if err := database.database.Model(&managedTenantRecord{}).Where("tenant_id = ?", "managed-first").Update("secret_digest", sha256Hex(hostedIdentityFixtureKey)).Error; err != nil {
		t.Fatal(err)
	}
	responses, err := newStructuredRequestStore(responseRoot, 60, database)
	if err != nil {
		t.Fatal(err)
	}
	upstream := newProviderRouter(NewOpenAIClient(http.DefaultClient, NewEndpoints()), newOpenAICompatibleChatClient(http.DefaultClient), newGeminiInteractionsClient(http.DefaultClient), newAnthropicMessagesClient(http.DefaultClient))
	key, err := service.store.providerKeyCipher.encryptConnection(rand.Reader, platformCredentialReference("platform-journal", 1), "openai", CatalogCredentialAPIKey, "sk-platform-pinned")
	if err != nil {
		t.Fatal(err)
	}
	fields, err := json.Marshal(map[string]string{CatalogCredentialAPIKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.database.Model(&managedPlatformCredentialRecord{}).Where("connection_id = ? AND version = ?", "platform-journal", 1).Update("fields", fields).Error; err != nil {
		t.Fatal(err)
	}
	dependencies := hostedTextRequestDependencies{database: database, responses: responses, cipher: service.store.providerKeyCipher, catalogRevision: "journal-catalog", authorize: func(*gorm.DB, managedJournalRequestRecord) error { return nil }, now: time.Now, entropy: rand.Reader}
	for _, apply := range configure {
		apply(&dependencies)
	}
	upstream.hostedText, err = newHostedTextRequests(t.Context(), dependencies)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(requestIdentifierHandler())
	policy, err := newRequestTimeoutPolicy(30, 30)
	if err != nil {
		t.Fatal(err)
	}
	auth := newTenantAuthenticator(service.store)
	router.POST(rootPath, tenantAuthenticatedHandler(auth, zap.NewNop().Sugar(), requestTimeoutHandler(policy, zap.NewNop().Sugar(), chatJSONHandler(upstream, providers, 1024, service.store, zap.NewNop().Sugar()))))
	router.POST(v2Path, tenantAuthenticatedHandler(auth, zap.NewNop().Sugar(), requestTimeoutHandler(policy, zap.NewNop().Sugar(), chatV2JSONHandler(upstream, providers, 4096, newTenantAssetStore(responseRoot, 4096, 60), responses, service.store, zap.NewNop().Sugar()))))
	router.POST(chatCompletionsPath, bearerTenantHandler(auth, requestTimeoutHandler(policy, zap.NewNop().Sugar(), openAITextHandler(clientChatProtocol, 4096, providers, upstream, service.store, zap.NewNop().Sugar()))))
	router.POST(responsesPath, bearerTenantHandler(auth, requestTimeoutHandler(policy, zap.NewNop().Sugar(), openAITextHandler(clientResponsesProtocol, 4096, providers, upstream, service.store, zap.NewNop().Sugar()))))
	router.GET(llmproxycontract.StructuredRequestPath, tenantAuthenticatedHandler(auth, zap.NewNop().Sugar(), structuredRequestStatusHandler(responses)))
	return router, service, upstream
}

const hostedIdentityFixtureKey = "hosted-identity-fixture-key"

type hostedIdentityTransport struct{ next http.RoundTripper }

func (transport hostedIdentityTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	authorized := request.Clone(request.Context())
	if strings.HasPrefix(authorized.URL.Path, "/v1/") {
		authorized.Header.Set("Authorization", "Bearer "+hostedIdentityFixtureKey)
	} else {
		query := authorized.URL.Query()
		query.Set("key", hostedIdentityFixtureKey)
		authorized.URL.RawQuery = query.Encode()
	}
	return transport.next.RoundTrip(authorized)
}
