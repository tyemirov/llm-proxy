package proxy

import (
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestHostedJournalTextTransportRecordsEachContinuation(t *testing.T) {
	database, intent, read := newJournalTransactionFixture(t)
	reserve := func(*gorm.DB, managedJournalRequestRecord) error { return nil }
	accepted, err := database.admitJournalRequest(t.Context(), intent("text-continuation"), reserve)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer sk-platform-journal" {
			t.Error("provider received another credential")
		}
		if request.Method == http.MethodDelete {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		ordinal := calls.Add(1)
		var attempts int64
		if err := database.database.Model(&managedJournalAttemptRecord{}).Where("request_id = ? AND state = ?", accepted.ID, journalAttemptDispatched).Count(&attempts).Error; err != nil || attempts != 1 {
			t.Errorf("provider dispatch preceded its journal: attempts=%d error=%v", attempts, err)
		}
		writer.Header().Set("Content-Type", "application/json")
		status, text, details := "completed", "second", ""
		if ordinal == 1 {
			status, text, details = "incomplete", "first ", `,"incomplete_details":{"reason":"max_output_tokens"}`
		}
		fmt.Fprintf(writer, `{"id":"private-response-%d","status":%q,"output_text":%q,"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}%s}`, ordinal, status, text, details)
	}))
	t.Cleanup(upstream.Close)
	execution := newHostedTextExecution(database, accepted, reserve, func() time.Time { return accepted.CreatedAt.Add(time.Second) }, rand.Reader)
	server := newJournalTextHTTPServer(t, database, upstream.URL, execution)
	response, err := server.Client().Post(server.URL+"/?provider=openai&model=gpt-4.1", "application/json", strings.NewReader(`{"prompt":"private prompt"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != "first second" || calls.Load() != 2 {
		t.Fatalf("status=%d body=%s calls=%d", response.StatusCode, body, calls.Load())
	}
	if state := read("/" + accepted.ID); state["state"] != "completed" || state["usage_state"] != "complete" {
		t.Fatalf("text journal=%v", state)
	}
	var observations []managedJournalObservationRecord
	if err := database.database.Order("created_at, id").Find(&observations).Error; err != nil {
		t.Fatal(err)
	}
	if len(observations) != 2 {
		t.Fatalf("continuation observations=%d", len(observations))
	}
	for _, observation := range observations {
		if strings.Contains(string(observation.SourceFields), "private prompt") || observation.Completeness != journalUsageComplete {
			t.Fatalf("unsafe or incomplete observation: %+v", observation)
		}
	}
}

func TestHostedJournalTextUnknownUsageStopsPaidContinuation(t *testing.T) {
	database, intent, read := newJournalTransactionFixture(t)
	reserve := func(*gorm.DB, managedJournalRequestRecord) error { return nil }
	accepted, err := database.admitJournalRequest(t.Context(), intent("text-unknown"), reserve)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodDelete {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"id":"private-unknown","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output_text":"partial"}`)
	}))
	t.Cleanup(upstream.Close)
	execution := newHostedTextExecution(database, accepted, reserve, func() time.Time { return accepted.CreatedAt.Add(time.Second) }, rand.Reader)
	server := newJournalTextHTTPServer(t, database, upstream.URL, execution)
	response, err := server.Client().Post(server.URL+"/?provider=openai&model=gpt-4.1", "application/json", strings.NewReader(`{"prompt":"private prompt"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusConflict || calls.Load() != 1 {
		t.Fatalf("unknown usage dispatched another paid call: status=%d calls=%d", response.StatusCode, calls.Load())
	}
	if state := read("/" + accepted.ID); state["usage_state"] != "unknown" {
		t.Fatalf("unknown usage changed to zero: %v", state)
	}
}

func TestHostedJournalTextPollingRetainsProviderIdentityBeforeObservation(t *testing.T) {
	database, intent, read := newJournalTransactionFixture(t)
	reserve := func(*gorm.DB, managedJournalRequestRecord) error { return nil }
	accepted, err := database.admitJournalRequest(t.Context(), intent("text-poll"), reserve)
	if err != nil {
		t.Fatal(err)
	}
	var creates, polls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPost {
			creates.Add(1)
			fmt.Fprint(writer, `{"id":"private-poll","status":"in_progress"}`)
			return
		}
		polls.Add(1)
		var attempt managedJournalAttemptRecord
		if err := database.database.Where("request_id = ?", accepted.ID).First(&attempt).Error; err != nil || attempt.ProviderRequestID != "private-poll" || attempt.State != journalAttemptDispatched {
			t.Errorf("provider polling has no durable recovery identity: %+v %v", attempt, err)
		}
		fmt.Fprint(writer, `{"id":"private-poll","status":"completed","output_text":"polled result","usage":{"input_tokens":12,"output_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	t.Cleanup(upstream.Close)
	execution := newHostedTextExecution(database, accepted, reserve, func() time.Time { return accepted.CreatedAt.Add(time.Second) }, rand.Reader)
	server := newJournalTextHTTPServer(t, database, upstream.URL, execution)
	response, err := server.Client().Post(server.URL+"/?provider=openai&model=gpt-4.1", "application/json", strings.NewReader(`{"prompt":"private prompt"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || creates.Load() != 1 || polls.Load() != 1 {
		t.Fatalf("status=%d creates=%d polls=%d", response.StatusCode, creates.Load(), polls.Load())
	}
	if state := read("/" + accepted.ID); state["state"] != "completed" {
		t.Fatalf("polled journal=%v", state)
	}
	var attempts int64
	if err := database.database.Model(&managedJournalAttemptRecord{}).Where("request_id = ?", accepted.ID).Count(&attempts).Error; err != nil || attempts != 1 {
		t.Fatalf("polling created paid attempts: %d %v", attempts, err)
	}
}

func TestHostedJournalTextStagingDoesNotBecomeAGenerationAttempt(t *testing.T) {
	database, intent, read := newJournalTransactionFixture(t)
	reserve := func(*gorm.DB, managedJournalRequestRecord) error { return nil }
	accepted, err := database.admitJournalRequest(t.Context(), intent("staging"), reserve)
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"file":{"name":"files/fixture"}}`)
	}))
	t.Cleanup(upstream.Close)
	provider := internalManagementProviderRegistry().definitions[providerID("openai")]
	model := provider.textModels["gpt-4.1"]
	provider, _ = provider.resolvedTransport(model.transportIdentifier)
	client := newGeminiInteractionsClient(newProviderTransportHTTPDoer(http.DefaultClient, provider, "sk-platform-journal"))
	execution := newHostedTextExecution(database, accepted, reserve, func() time.Time { return accepted.CreatedAt.Add(time.Second) }, rand.Reader)
	request, err := http.NewRequestWithContext(contextWithHostedTextExecution(t.Context(), execution), http.MethodPost, upstream.URL, strings.NewReader(`{"file":{"display_name":"fixture"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.performGeminiFileRequest(request); err != nil {
		t.Fatal(err)
	}
	if state := read("/" + accepted.ID); state["state"] != "accepted" {
		t.Fatalf("file staging became generation: %v", state)
	}
	var attempts int64
	if err := database.database.Model(&managedJournalAttemptRecord{}).Count(&attempts).Error; err != nil || attempts != 0 {
		t.Fatalf("file staging attempts=%d error=%v", attempts, err)
	}
}

func TestHostedJournalTextLostResponseRetainsUncertainDispatch(t *testing.T) {
	database, intent, read := newJournalTransactionFixture(t)
	reserve := func(*gorm.DB, managedJournalRequestRecord) error { return nil }
	accepted, err := database.admitJournalRequest(t.Context(), intent("lost-response"), reserve)
	if err != nil {
		t.Fatal(err)
	}
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
	execution := newHostedTextExecution(database, accepted, reserve, func() time.Time { return accepted.CreatedAt.Add(time.Second) }, rand.Reader)
	server := newJournalTextHTTPServer(t, database, upstream.URL, execution)
	response, err := server.Client().Post(server.URL+"/?provider=openai&model=gpt-4.1", "application/json", strings.NewReader(`{"prompt":"private prompt"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusBadRequest || response.StatusCode == http.StatusGatewayTimeout || calls.Load() != 1 {
		t.Fatalf("lost response retried: status=%d calls=%d", response.StatusCode, calls.Load())
	}
	if state := read("/" + accepted.ID); state["state"] != "uncertain" || state["usage_state"] != "unknown" {
		t.Fatalf("lost response journal=%v", state)
	}
	var cases []managedJournalCaseRecord
	if err := database.database.Where("request_id = ?", accepted.ID).Find(&cases).Error; err != nil || len(cases) != 1 || cases[0].Reason != journalCaseDispatchUnknown {
		t.Fatalf("lost response reconciliation=%v error=%v", cases, err)
	}
}

func TestHostedJournalTextRevokedGrantStopsSynthesis(t *testing.T) {
	database, intent, read := newJournalTransactionFixture(t)
	reserve := func(*gorm.DB, managedJournalRequestRecord) error { return nil }
	accepted, err := database.admitJournalRequest(t.Context(), intent("revoked-synthesis"), reserve)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", accepted.GrantID).Update("state", hostedGrantRevoked).Error; err != nil {
			t.Error(err)
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"id":"private-synthesis","status":"completed","output":[],"usage":{"input_tokens":10,"output_tokens":3,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	t.Cleanup(upstream.Close)
	execution := newHostedTextExecution(database, accepted, reserve, func() time.Time { return accepted.CreatedAt.Add(time.Second) }, rand.Reader)
	server := newJournalTextHTTPServer(t, database, upstream.URL, execution)
	response, err := server.Client().Post(server.URL+"/?provider=openai&model=gpt-4.1", "application/json", strings.NewReader(`{"prompt":"private prompt"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusForbidden || !strings.Contains(string(body), errHostedAuthorityDenied.Error()) || calls.Load() != 1 {
		t.Fatalf("revoked synthesis: status=%d body=%s calls=%d", response.StatusCode, body, calls.Load())
	}
	if state := read("/" + accepted.ID); state["state"] != "failed" || state["usage_state"] != "complete" {
		t.Fatalf("revoked synthesis lost observed usage: %v", state)
	}
}

func TestHostedJournalTextSynthesisCreatesAnotherAttempt(t *testing.T) {
	database, intent, read := newJournalTransactionFixture(t)
	reserve := func(*gorm.DB, managedJournalRequestRecord) error { return nil }
	accepted, err := database.admitJournalRequest(t.Context(), intent("synthesis"), reserve)
	if err != nil {
		t.Fatal(err)
	}
	var creates, polls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet {
			polls.Add(1)
			fmt.Fprint(writer, `{"id":"private-synthesis-final","status":"completed","output_text":"synthesized result","usage":{"input_tokens":12,"output_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
			return
		}
		if creates.Add(1) == 1 {
			fmt.Fprint(writer, `{"id":"private-synthesis-start","status":"completed","output":[],"usage":{"input_tokens":10,"output_tokens":3,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
			return
		}
		body, err := io.ReadAll(request.Body)
		if err != nil || !strings.Contains(string(body), `"previous_response_id":"private-synthesis-start"`) {
			t.Errorf("synthesis lost its provider identity: body=%s error=%v", body, err)
		}
		fmt.Fprint(writer, `{"id":"private-synthesis-final","status":"in_progress"}`)
	}))
	t.Cleanup(upstream.Close)
	execution := newHostedTextExecution(database, accepted, reserve, func() time.Time { return accepted.CreatedAt.Add(time.Second) }, rand.Reader)
	server := newJournalTextHTTPServer(t, database, upstream.URL, execution)
	response, err := server.Client().Post(server.URL+"/?provider=openai&model=gpt-4.1", "application/json", strings.NewReader(`{"prompt":"private prompt"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != "synthesized result" || creates.Load() != 2 || polls.Load() != 1 {
		t.Fatalf("synthesis: status=%d body=%s creates=%d polls=%d", response.StatusCode, body, creates.Load(), polls.Load())
	}
	if state := read("/" + accepted.ID); state["state"] != "completed" || state["usage_state"] != "complete" {
		t.Fatalf("synthesis journal=%v", state)
	}
	var attempts []managedJournalAttemptRecord
	if err := database.database.Where("request_id = ?", accepted.ID).Order("number").Find(&attempts).Error; err != nil || len(attempts) != 2 {
		t.Fatalf("synthesis attempts=%v error=%v", attempts, err)
	}
	if attempts[0].ProviderRequestID != "private-synthesis-start" || attempts[1].ProviderRequestID != "private-synthesis-final" || attempts[0].State != journalAttemptObserved || attempts[1].State != journalAttemptObserved {
		t.Fatalf("synthesis attribution=%v", attempts)
	}
}

func TestHostedJournalTextMalformedResponseCannotSupplyCompleteUsage(t *testing.T) {
	database, intent, read := newJournalTransactionFixture(t)
	reserve := func(*gorm.DB, managedJournalRequestRecord) error { return nil }
	accepted, err := database.admitJournalRequest(t.Context(), intent("malformed-response"), reserve)
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"id":"private-malformed","status":"completed","output_text":"result","usage":{"input_tokens":10,"output_tokens":3,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}} {"another":"value"}`)
	}))
	t.Cleanup(upstream.Close)
	execution := newHostedTextExecution(database, accepted, reserve, func() time.Time { return accepted.CreatedAt.Add(time.Second) }, rand.Reader)
	server := newJournalTextHTTPServer(t, database, upstream.URL, execution)
	response, err := server.Client().Post(server.URL+"/?provider=openai&model=gpt-4.1", "application/json", strings.NewReader(`{"prompt":"private prompt"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusBadRequest {
		t.Fatalf("malformed provider response succeeded: %d", response.StatusCode)
	}
	if state := read("/" + accepted.ID); state["usage_state"] != "unknown" {
		t.Fatalf("malformed response supplied complete usage: %v", state)
	}
}

// The fixture supplies the admitted request and qualified connection at the
// financial authority boundary. The HTTP handler, provider clients, and journal run unchanged.
func newJournalTextHTTPServer(t *testing.T, database *gormManagedTenantDatabase, upstreamURL string, execution *hostedTextExecution) *httptest.Server {
	t.Helper()
	providers := internalManagementProviderRegistry()
	definition := providers.definitions[providerID("openai")]
	definition.connectionValues = map[string]string{"api_key": "sk-platform-journal"}
	for identifier, transport := range definition.transports {
		transport.endpointURLOverride = upstreamURL
		definition.transports[identifier] = transport
	}
	providers.definitions[providerID("openai")] = definition
	service := newInternalManagementService(t, newFakeManagedTenantDatabase(), providers)
	service.store.database = database
	client := http.DefaultClient
	upstream := newProviderRouter(NewOpenAIClient(client, NewEndpoints()), newOpenAICompatibleChatClient(client), newGeminiInteractionsClient(client), newAnthropicMessagesClient(client))
	router := gin.New()
	router.Use(requestIdentifierHandler(), func(ctx *gin.Context) {
		ctx.Set(contextKeyTenant, tenant{identifier: tenantID("managed-first"), userID: "owner"})
		ctx.Request = ctx.Request.WithContext(requestContextWithTelemetry(ctx.Request.Context(), newRequestTelemetry(requestIDFromContext(ctx), usageEndpointText)))
		ctx.Request = ctx.Request.WithContext(contextWithHostedTextExecution(ctx.Request.Context(), execution))
	})
	policy, err := newRequestTimeoutPolicy(30, 30)
	if err != nil {
		t.Fatal(err)
	}
	router.POST(rootPath, requestTimeoutHandler(policy, zap.NewNop().Sugar(), chatJSONHandler(upstream, providers, 1024, service.store, zap.NewNop().Sugar())))
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server
}
