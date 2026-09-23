package proxy

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestHostedProviderServiceUsesExistingExecution(t *testing.T) {
	for _, operation := range []string{ModelOperationPronunciationDictionaryCreation, ModelOperationAudioAlignment} {
		for _, requestID := range []string{"native-service-request", ""} {
			t.Run(operation+"/request_id="+requestID, func(t *testing.T) { testHostedProviderService(t, operation, requestID) })
		}
	}
}

func testHostedProviderService(t *testing.T, operation, requestID string) {
	t.Helper()
	database, _, read := newJournalTransactionFixture(t)
	const audio = "controlled audio input"
	var submissions, reservations atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		submissions.Add(1)
		if request.Method != http.MethodPost || request.Header.Get("xi-api-key") != "hosted-service-secret" {
			t.Errorf("service submission lost pinned authority: %s", request.Method)
		}
		writer.Header().Set("Content-Type", "application/json")
		if operation == ModelOperationAudioAlignment {
			if err := request.ParseMultipartForm(4096); err != nil {
				t.Error(err)
				return
			}
			defer request.MultipartForm.RemoveAll()
			file, _, err := request.FormFile("file")
			if err != nil {
				t.Error(err)
				return
			}
			defer file.Close()
			data, err := io.ReadAll(file)
			if err != nil || string(data) != audio || request.FormValue("text") != "Names" {
				t.Errorf("alignment input changed: data=%q text=%q error=%v", data, request.FormValue("text"), err)
			}
			fmt.Fprint(writer, `{"characters":[{"text":"N","start":0,"end":1}],"words":[{"text":"Names","start":0,"end":1}],"loss":0}`)
		} else {
			writer.Header().Set("request-id", requestID)
			fmt.Fprint(writer, `{"id":"native-dictionary","version_id":"native-version","name":"Names","description":"private dictionary","created_by":"provider-user","creation_time_unix":1,"version_rules_num":1}`)
		}
	}))
	t.Cleanup(upstream.Close)
	server, service := newHostedMediaAdmissionHTTPServer(t, database)
	service.hostedAdmission = func(_ *gorm.DB, request managedJournalRequestRecord) error {
		if request.Model != "" || request.Provider != "elevenlabs" || request.Operation != operation {
			t.Errorf("incorrect service attribution: %+v", request)
		}
		reservations.Add(1)
		return nil
	}
	imageAdapter := service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageGenerate, "openai", "gpt-image-2")].(*imageGenerationAdapter)
	provider := service.providers.definitions[providerID("elevenlabs")]
	for id, transport := range provider.transports {
		transport.endpointURLOverride = upstream.URL
		provider.transports[id] = transport
	}
	route, err := service.catalog.ResolveService("elevenlabs", ModelOperationPronunciationDictionaryCreation)
	if err != nil {
		t.Fatal(err)
	}
	service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityAudioDictionaryCreate, "elevenlabs", "")] = newProviderDictionaryAdapter(route, provider, imageAdapter.tenants, service.store)
	alignment, err := service.catalog.ResolveService("elevenlabs", ModelOperationAudioAlignment)
	if err != nil {
		t.Fatal(err)
	}
	service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityAudioAlign, "elevenlabs", "")] = newProviderAlignmentAdapter(alignment, provider, imageAdapter.tenants, service.store, service.assets)
	key, err := imageAdapter.tenants.providerKeyCipher.encryptConnection(rand.Reader, platformCredentialReference("platform-service", 1), "elevenlabs", CatalogCredentialAPIKey, "hosted-service-secret")
	if err != nil {
		t.Fatal(err)
	}
	fields, err := json.Marshal(map[string]string{CatalogCredentialAPIKey: key})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, record := range []any{
		&managedPlatformConnectionRecord{ID: "platform-service", Provider: "elevenlabs", Name: "Service", Version: 1, CreatedAt: now, UpdatedAt: now},
		&managedPlatformCredentialRecord{ConnectionID: "platform-service", Version: 1, Fields: fields, QualifiedAt: now, CreatedAt: now},
		&managedHostedGrantRecord{ID: "grant-service", BillingAccountID: "billing-journal", TenantID: "managed-first", PlatformConnectionID: "platform-service", Provider: "elevenlabs", CatalogRevision: service.catalog.Revision(), Offerings: []byte(fmt.Sprintf(`[{"operations":[%q]}]`, operation)), State: hostedGrantActive, Revision: 1, CreatedAt: now, UpdatedAt: now},
		&managedHostedGrantRevisionRecord{GrantID: "grant-service", Revision: 1, State: hostedGrantActive, ActorUserID: "operator", Reason: "Service acceptance", CreatedAt: now},
		&managedHostedTenantAssignmentRecord{TenantID: "managed-first", ProviderID: "elevenlabs", GrantID: "grant-service", CreatedAt: now},
		&managedProviderProfileRecord{TenantID: "managed-first", ProviderID: "elevenlabs", CreatedAt: now, UpdatedAt: now},
	} {
		if err := database.database.Omit(clause.Associations).Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	exchange := func(key, body string, want int) map[string]any {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, server.URL+llmproxycontract.MediaOperationsPath, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(llmproxycontract.HeaderIdempotencyKey, key)
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		encoded, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != want {
			t.Fatalf("service status=%d want=%d body=%s", response.StatusCode, want, encoded)
		}
		validateHostedIdentityResponse(t, request, response, encoded)
		var result map[string]any
		if err := json.Unmarshal(encoded, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	intent := `{"capability":"audio.dictionary.create","provider":"elevenlabs","input":{"name":"Names","description":"private dictionary","rules":[{"type":"alias","string_to_replace":"A","alias":"Alpha"}]},"controls":{}}`
	capability, outside := llmproxycontract.MediaCapabilityAudioDictionaryCreate, llmproxycontract.MediaCapabilityAudioAlign
	if operation == ModelOperationAudioAlignment {
		asset, err := service.assets.upload(tenant{identifier: tenantID("managed-first")}, "audio/wav", bytes.NewReader([]byte(audio)))
		if err != nil {
			t.Fatal(err)
		}
		intent = fmt.Sprintf(`{"capability":"audio.align","provider":"elevenlabs","input":{"audio_asset_id":%q,"transcript":"Names"},"controls":{}}`, asset.AssetID)
		capability, outside = outside, capability
	}
	accepted := exchange("hosted-service", intent, http.StatusAccepted)
	id := accepted["operation_id"].(string)
	service.runOperation("service-worker", id)
	result := hostedMediaWorkerStatus(t, server, id)
	if result["state"] != MediaOperationStateSucceeded || len(result["outputs"].([]any)) != 1 {
		t.Fatalf("service result=%v", result)
	}
	entry := read("")["requests"].([]any)[0].(map[string]any)
	if _, present := entry["model"]; present {
		t.Fatalf("service journal invented model: %v", entry)
	}
	if entry["state"] != string(journalRequestCompleted) || entry["operation"] != operation || entry["usage_state"] != string(journalUsageUnknown) {
		t.Fatalf("service journal=%v", entry)
	}
	var attempt managedJournalAttemptRecord
	if err := database.database.Where("request_id = ?", entry["id"]).First(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	wantRequestID := ""
	if operation == ModelOperationPronunciationDictionaryCreation {
		wantRequestID = requestID
		var retained mediaOperationRecord
		if err := database.database.Where("operation_id = ?", id).First(&retained).Error; err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(retained.ProviderHandle, "private dictionary") {
			t.Fatal("operation lost the recovery receipt")
		}
	}
	if attempt.ProviderRequestID != wantRequestID {
		t.Fatalf("journal provider identity=%q want=%q; recovery content must remain with the operation", attempt.ProviderRequestID, wantRequestID)
	}
	if replay := exchange("hosted-service", intent, http.StatusOK); replay["operation_id"] != id {
		t.Fatal("service replay changed identity")
	}
	exchange("hosted-service", strings.Replace(intent, "Names", "Other", 1), http.StatusConflict)
	exchange("outside-grant", strings.Replace(intent, capability, outside, 1), http.StatusUnprocessableEntity)
	if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-service").Update("state", hostedGrantRevoked).Error; err != nil {
		t.Fatal(err)
	}
	exchange("hosted-service", intent, http.StatusOK)
	exchange("revoked-service", intent, http.StatusUnprocessableEntity)
	service.runOperation("duplicate-service-worker", id)
	if submissions.Load() != 1 || reservations.Load() != 2 {
		t.Fatalf("service effects: submissions=%d reservations=%d", submissions.Load(), reservations.Load())
	}
}
