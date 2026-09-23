package proxy

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
)

func TestHostedMediaWorkerPublishesOneResultAndJournalOutcome(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	var calls atomic.Int64
	encoded := base64.StdEncoding.EncodeToString(imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 1024, 1024))))
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Header.Get("Authorization") != "Bearer sk-platform-pinned" {
			t.Error("media worker did not use the accepted platform credential")
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(writer, `{"data":[{"b64_json":%q}]}`, encoded)
	}))
	t.Cleanup(upstream.Close)
	server, service := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(upstream.URL))
	var reservations atomic.Int64
	service.hostedAdmission = func(*gorm.DB, managedJournalRequestRecord, mediaOperationRecord) error {
		reservations.Add(1)
		return nil
	}
	accepted := hostedMediaAdmissionHTTP(t, server, "worker-result", "private image prompt", http.StatusAccepted)
	id := accepted["operation_id"].(string)
	service.runOperation("worker-first", id)
	result := hostedMediaWorkerStatus(t, server, id)
	if result["state"] != MediaOperationStateSucceeded || len(result["outputs"].([]any)) != 1 || calls.Load() != 1 {
		t.Fatalf("media result=%v calls=%d", result, calls.Load())
	}
	replay := hostedMediaAdmissionHTTP(t, server, "worker-result", "private image prompt", http.StatusOK)
	service.runOperation("worker-duplicate", id)
	if replay["operation_id"] != id || reservations.Load() != 2 || calls.Load() != 1 {
		t.Fatalf("duplicate dispatch or funds effect: reservations=%d calls=%d", reservations.Load(), calls.Load())
	}
	entries := read("")["requests"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["state"] != string(journalRequestCompleted) || entries[0].(map[string]any)["usage_state"] != string(journalUsageUnknown) {
		t.Fatalf("media journal outcome=%v", entries)
	}
}

func TestHostedMediaWorkerRevocationPreventsSubmission(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)
	server, service := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(upstream.URL))
	accepted := hostedMediaAdmissionHTTP(t, server, "revoked-worker", "private image prompt", http.StatusAccepted)
	if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-journal").Update("state", hostedGrantRevoked).Error; err != nil {
		t.Fatal(err)
	}
	service.runOperation("revoked-worker", accepted["operation_id"].(string))
	result := hostedMediaWorkerStatus(t, server, accepted["operation_id"].(string))
	if result["state"] != MediaOperationStateFailed || calls.Load() != 0 {
		t.Fatalf("revoked media result=%v calls=%d", result, calls.Load())
	}
	entries := read("")["requests"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["state"] != string(journalRequestFailed) {
		t.Fatalf("revoked journal request=%v", entries)
	}
}

func TestHostedMediaWorkerRetainsCredentialVersion(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Header.Get("Authorization") != "Bearer sk-platform-pinned" {
			t.Error("rotation replaced accepted media credentials")
		}
		writer.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(upstream.Close)
	server, service := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(upstream.URL))
	accepted := hostedMediaAdmissionHTTP(t, server, "rotated-worker", "private image prompt", http.StatusAccepted)
	cipher := internalManagedProviderKeyCipher()
	key, err := cipher.encryptConnection(rand.Reader, platformCredentialReference("platform-journal", 2), "openai", CatalogCredentialAPIKey, "sk-platform-next")
	if err != nil {
		t.Fatal(err)
	}
	fields, err := json.Marshal(map[string]string{CatalogCredentialAPIKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.database.Create(&managedPlatformCredentialRecord{ConnectionID: "platform-journal", Version: 2, Fields: fields, QualifiedAt: time.Now(), CreatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.database.Model(&managedPlatformConnectionRecord{}).Where("id = ?", "platform-journal").Update("version", 2).Error; err != nil {
		t.Fatal(err)
	}
	service.runOperation("rotated-worker", accepted["operation_id"].(string))
	result := hostedMediaWorkerStatus(t, server, accepted["operation_id"].(string))
	if result["state"] != MediaOperationStateFailed || calls.Load() != 1 || read("")["requests"].([]any)[0].(map[string]any)["state"] != string(journalRequestFailed) {
		t.Fatalf("rotated media result=%v calls=%d", result, calls.Load())
	}
}

func TestHostedMediaWorkerLostResponseRemainsUncertain(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
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
	server, service := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(upstream.URL))
	accepted := hostedMediaAdmissionHTTP(t, server, "lost-media", "private image prompt", http.StatusAccepted)
	id := accepted["operation_id"].(string)
	service.runOperation("lost-worker", id)
	service.runOperation("duplicate-worker", id)
	result := hostedMediaWorkerStatus(t, server, id)
	replay := hostedMediaAdmissionHTTP(t, server, "lost-media", "private image prompt", http.StatusOK)
	if result["state"] != MediaOperationStateUncertain || replay["operation_id"] != id || calls.Load() != 1 {
		t.Fatalf("lost response result=%v calls=%d", result, calls.Load())
	}
	entry := read("")["requests"].([]any)[0].(map[string]any)
	if entry["state"] != string(journalRequestUncertain) || entry["usage_state"] != string(journalUsageUnknown) {
		t.Fatalf("lost response journal=%v", entry)
	}
}

func TestHostedMediaWorkerProviderCancellationFinishesJournal(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	var submissions, cancellations atomic.Int64
	firstPoll, releasePoll, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var release sync.Once
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/resp_media_cancel/cancel":
			cancellations.Add(1)
			fmt.Fprint(writer, `{"id":"resp_media_cancel","status":"cancelled"}`)
		case request.Method == http.MethodPost:
			submissions.Add(1)
			fmt.Fprint(writer, `{"id":"resp_media_cancel","status":"queued"}`)
		default:
			close(firstPoll)
			select {
			case <-releasePoll:
			case <-request.Context().Done():
				return
			}
			fmt.Fprint(writer, `{"id":"resp_media_cancel","status":"cancelled"}`)
		}
	}))
	t.Cleanup(upstream.Close)
	server, service := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(upstream.URL))
	controls := json.RawMessage(`{"surface":"responses","responses_model":"gpt-5","quality":"low","size":"1024x1024","background":"opaque","output_format":"png","output_count":1}`)
	accepted := hostedMediaAdmissionHTTP(t, server, "provider-cancel", "private image prompt", http.StatusAccepted, controls)
	id := accepted["operation_id"].(string)
	go func() { defer close(done); service.runOperation("cancelled-provider-worker", id) }()
	t.Cleanup(func() { release.Do(func() { close(releasePoll) }); waitHostedMediaWorker(t, done) })
	select {
	case <-firstPoll:
	case <-time.After(5 * time.Second):
		t.Fatal("provider job did not reach its first poll")
	}
	if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-journal").Update("state", hostedGrantRevoked).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		request, err := http.NewRequest(http.MethodPut, server.URL+llmproxycontract.MediaOperationsPath+"/"+id+"/cancellation", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("provider cancellation status=%d", response.StatusCode)
		}
	}
	release.Do(func() { close(releasePoll) })
	waitHostedMediaWorker(t, done)
	result := hostedMediaWorkerStatus(t, server, id)
	entry := read("")["requests"].([]any)[0].(map[string]any)
	if result["state"] != MediaOperationStateCancelled || entry["state"] != string(journalRequestFailed) || entry["usage_state"] != string(journalUsageUnknown) || submissions.Load() != 1 || cancellations.Load() != 1 {
		t.Fatalf("cancelled provider result=%v journal=%v submissions=%d cancellations=%d", result, entry, submissions.Load(), cancellations.Load())
	}
}

func TestHostedMediaWorkerReplacementPreventsObsoleteSubmission(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(upstream.Close)
	server, service := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(upstream.URL))
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	adapterKey := mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageGenerate, "openai", "gpt-image-2")
	service.adapters[adapterKey] = hostedMediaExecuteHook{MediaOperationAdapter: service.adapters[adapterKey], before: func() { close(entered); <-release }}
	accepted := hostedMediaAdmissionHTTP(t, server, "obsolete-media", "private image prompt", http.StatusAccepted)
	id := accepted["operation_id"].(string)
	go func() { defer close(done); service.runOperation("obsolete-worker", id) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("media worker did not reach the adapter")
	}
	if err := database.database.Model(&mediaOperationClaimRecord{}).Where("operation_id = ?", id).Update("expires_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	service.runOperation("replacement-worker", id)
	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("obsolete worker did not finish")
	}
	result := hostedMediaWorkerStatus(t, server, id)
	if result["state"] != MediaOperationStateUncertain || calls.Load() != 0 || read("")["requests"].([]any)[0].(map[string]any)["state"] != string(journalRequestUncertain) {
		t.Fatalf("obsolete worker result=%v calls=%d", result, calls.Load())
	}
}

func TestHostedMediaWorkerQueuedCancellationFinishesJournal(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	server, service := newHostedMediaAdmissionHTTPServer(t, database)
	accepted := hostedMediaAdmissionHTTP(t, server, "cancelled-media", "private image prompt", http.StatusAccepted)
	id := accepted["operation_id"].(string)
	request, err := http.NewRequest(http.MethodPut, server.URL+llmproxycontract.MediaOperationsPath+"/"+id+"/cancellation", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("cancellation status=%d", response.StatusCode)
	}
	service.runOperation("cancelled-worker", id)
	result := hostedMediaWorkerStatus(t, server, id)
	entry := read("")["requests"].([]any)[0].(map[string]any)
	if result["state"] != MediaOperationStateCancelled || entry["state"] != string(journalRequestFailed) || entry["failure_code"] != "operation_cancelled" {
		t.Fatalf("cancelled media=%v journal=%v", result, entry)
	}
}

func TestHostedMediaWorkerRecoversProviderHandleAfterRevocation(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	var posts, polls, reservations atomic.Int64
	firstPoll, releasePoll, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var release sync.Once
	encoded := base64.StdEncoding.EncodeToString(imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 1024, 1024))))
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer sk-platform-pinned" {
			t.Error("provider recovery changed its accepted credential")
		}
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPost {
			posts.Add(1)
			fmt.Fprint(writer, `{"id":"resp_media_recovery","status":"queued"}`)
			return
		}
		if polls.Add(1) == 1 {
			close(firstPoll)
			select {
			case <-releasePoll:
			case <-request.Context().Done():
				return
			}
		}
		fmt.Fprintf(writer, `{"id":"resp_media_recovery","status":"completed","usage":{"input_tokens":42,"output_tokens":0,"total_tokens":42},"output":[{"id":"ig_media_recovery","type":"image_generation_call","status":"completed","result":%q}]}`, encoded)
	}))
	t.Cleanup(upstream.Close)
	first, firstService := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(upstream.URL))
	second, secondService := newHostedMediaAdmissionHTTPServer(t, openJournalTransactionInstance(t, database), hostedMediaWorkerProvider(upstream.URL), func(service *mediaOperationService) { service.assets = firstService.assets })
	reserve := func(*gorm.DB, managedJournalRequestRecord, mediaOperationRecord) error {
		reservations.Add(1)
		return nil
	}
	firstService.hostedAdmission, secondService.hostedAdmission = reserve, reserve
	controls := json.RawMessage(`{"surface":"responses","responses_model":"gpt-5","quality":"low","size":"1024x1024","background":"opaque","output_format":"png","output_count":1}`)
	accepted := hostedMediaAdmissionHTTP(t, first, "provider-recovery", "private image prompt", http.StatusAccepted, controls)
	id := accepted["operation_id"].(string)
	go func() { defer close(done); firstService.runOperation("first-provider-worker", id) }()
	t.Cleanup(func() {
		release.Do(func() { close(releasePoll) })
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("first provider worker did not finish")
		}
	})
	select {
	case <-firstPoll:
	case <-time.After(5 * time.Second):
		t.Fatal("provider job did not reach its first poll")
	}
	var attempt managedJournalAttemptRecord
	if err := database.database.First(&attempt).Error; err != nil || attempt.ProviderRequestID != "resp_media_recovery" {
		t.Fatalf("provider handle was not journaled before polling: id=%q error=%v", attempt.ProviderRequestID, err)
	}
	if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-journal").Update("state", hostedGrantRevoked).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.database.Model(&mediaOperationClaimRecord{}).Where("operation_id = ?", id).Update("expires_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	secondService.runOperation("recovered-provider-worker", id)
	release.Do(func() { close(releasePoll) })
	waitHostedMediaWorker(t, done)
	result := hostedMediaWorkerStatus(t, second, id)
	entry := read("")["requests"].([]any)[0].(map[string]any)
	if result["state"] != MediaOperationStateSucceeded || entry["state"] != string(journalRequestCompleted) || posts.Load() != 1 || polls.Load() != 2 || reservations.Load() != 2 {
		t.Fatalf("recovery result=%v journal=%v posts=%d polls=%d reservations=%d", result, entry, posts.Load(), polls.Load(), reservations.Load())
	}
	observations, err := database.pendingJournalDeliveries(context.Background(), 100)
	if err != nil || len(observations) != 1 || observations[0].AdapterRevision != CatalogProtocolOpenAIResponses+":image:1" {
		t.Fatalf("recovered usage=%v error=%v", observations, err)
	}
	var quantities []journalQuantity
	if err := json.Unmarshal(observations[0].Quantities, &quantities); err != nil {
		t.Fatal(err)
	}
	for _, quantity := range quantities {
		if quantity.Dimension == "responses_input_tokens" && quantity.Value == "42" {
			return
		}
	}
	t.Fatalf("recovered input usage was lost: %s", observations[0].Quantities)
}

func waitHostedMediaWorker(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("media worker did not finish")
	}
}

type hostedMediaExecuteHook struct {
	MediaOperationAdapter
	before func()
}

func (adapter hostedMediaExecuteHook) Execute(ctx context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	adapter.before()
	return adapter.MediaOperationAdapter.Execute(ctx, request)
}

func hostedMediaWorkerProvider(endpoint string) func(*mediaOperationService) {
	return func(service *mediaOperationService) {
		service.assets.maxAssetBytes = 1 << 20
		definition := service.providers.definitions[providerID("openai")]
		for identifier, transport := range definition.transports {
			transport.endpointURLOverride = endpoint
			definition.transports[identifier] = transport
		}
		service.providers.definitions[providerID("openai")] = definition
	}
}

func hostedMediaWorkerStatus(t *testing.T, server *httptest.Server, id string) map[string]any {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, server.URL+llmproxycontract.MediaOperationsPath+"/"+id, nil)
	if err != nil {
		t.Fatal(err)
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
	if response.StatusCode != http.StatusOK {
		t.Fatalf("media status=%d body=%s", response.StatusCode, body)
	}
	request.URL.Path = llmproxycontract.MediaOperationsPath + "/{operation_id}"
	validateHostedIdentityResponse(t, request, response, body)
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
