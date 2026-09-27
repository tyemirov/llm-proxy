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
			t.Run(operation+"/request_id="+requestID, func(t *testing.T) { testHostedProviderService(t, operation, requestID, "") })
		}
	}
}

func TestHostedProviderServiceDictionaryReceiptEvidence(t *testing.T) {
	for _, fault := range []string{"publication", "usage", "invalid_receipt"} {
		t.Run(fault, func(t *testing.T) {
			testHostedProviderService(t, ModelOperationPronunciationDictionaryCreation, "native-service-request", fault)
		})
	}
}

func TestHostedProviderServiceAlignmentReceiptRecovery(t *testing.T) {
	for _, requestID := range []string{"private-alignment-trace", ""} {
		t.Run("trace="+requestID, func(t *testing.T) {
			testHostedProviderService(t, ModelOperationAudioAlignment, requestID, "publication")
		})
	}
	for _, fault := range []string{"receipt_write", "invalid_receipt", "stored_json", "stored_output", "stored_start", "stored_end", "stored_trace", "stored_binding"} {
		t.Run(fault, func(t *testing.T) {
			testHostedProviderService(t, ModelOperationAudioAlignment, "private-alignment-trace", fault)
		})
	}
}

func testHostedProviderService(t *testing.T, operation, requestID, fault string) {
	t.Helper()
	database, _, read := newJournalTransactionFixture(t)
	const audio = "controlled audio input"
	var submissions, reservations atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("xi-api-key") != "hosted-service-secret" {
			t.Errorf("service submission lost pinned authority: %s", request.Method)
		}
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == alignmentUsagePath {
			fmt.Fprint(writer, `{"columns":["total_minutes","total_cost","usage_count"],"column_types":["Float","Float","Int"],"column_units":["min","usd",null],"rows":[[0.0299375,0.00010977083333333333,1]]}`)
			return
		}
		submissions.Add(1)
		if fault == "invalid_receipt" {
			fmt.Fprint(writer, `{}`)
			return
		}
		if operation == ModelOperationAudioAlignment {
			writer.Header().Set("x-trace-id", requestID)
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
	service.hostedAdmission = func(_ *gorm.DB, request managedJournalRequestRecord, _ mediaOperationRecord) error {
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
	storedFault := strings.HasPrefix(fault, "stored_")
	recoveryFault := fault == "publication" || fault == "usage" || storedFault
	if recoveryFault {
		if err := database.database.Exec("CREATE TRIGGER reject_dictionary_publication BEFORE UPDATE OF public_state ON media_operation_records WHEN OLD.public_state = 'running' AND NEW.public_state != 'running' BEGIN SELECT RAISE(ABORT, 'controlled dictionary publication failure'); END").Error; err != nil {
			t.Fatal(err)
		}
	}
	if fault == "usage" {
		if err := database.database.Exec("CREATE TRIGGER reject_dictionary_usage BEFORE INSERT ON managed_journal_observation_records BEGIN SELECT RAISE(ABORT, 'controlled dictionary usage failure'); END").Error; err != nil {
			t.Fatal(err)
		}
	}
	if fault == "receipt_write" {
		if err := database.database.Exec("CREATE TRIGGER reject_service_receipt BEFORE UPDATE OF provider_handle ON media_operation_records WHEN NEW.provider_handle != '' BEGIN SELECT RAISE(ABORT, 'controlled service receipt failure'); END").Error; err != nil {
			t.Fatal(err)
		}
	}
	service.runOperation("service-worker", id)
	if fault == "invalid_receipt" || fault == "receipt_write" {
		if result := hostedMediaWorkerStatus(t, server, id); result["state"] != MediaOperationStateUncertain {
			t.Fatalf("invalid receipt result=%v", result)
		}
		entry := read("")["requests"].([]any)[0].(map[string]any)
		if entry["state"] != string(journalRequestUncertain) || entry["usage_state"] == string(journalUsageComplete) {
			t.Fatalf("invalid receipt journal=%v", entry)
		}
		var observations int64
		if err := database.database.Model(&managedJournalObservationRecord{}).Count(&observations).Error; err != nil {
			t.Fatal(err)
		}
		if observations != 0 {
			t.Fatal("invalid receipt acquired measured usage")
		}
		var retained mediaOperationRecord
		if err := database.database.Where("operation_id = ?", id).First(&retained).Error; err != nil {
			t.Fatal(err)
		}
		var attempt managedJournalAttemptRecord
		if err := database.database.Where("request_id = ?", entry["id"]).First(&attempt).Error; err != nil {
			t.Fatal(err)
		}
		if retained.ProviderHandle != "" || attempt.ProviderRequestID != "" {
			t.Fatal("failed receipt retained partial provider evidence")
		}
		if fault == "receipt_write" {
			if err := database.database.Exec("DROP TRIGGER reject_service_receipt").Error; err != nil {
				t.Fatal(err)
			}
		}
		exchange("hosted-service", intent, http.StatusOK)
		service.runOperation("duplicate-uncertain", id)
		if submissions.Load() != 1 {
			t.Fatal("uncertain request repeated provider work")
		}
		return
	}
	if recoveryFault {
		if result := hostedMediaWorkerStatus(t, server, id); result["state"] != MediaOperationStateRunning {
			t.Fatalf("failed publication result=%v", result)
		}
		if storedFault {
			testHostedAlignmentStoredReceiptFailure(t, database, service, server, id, fault, &submissions)
		}
		if err := database.database.Exec("DROP TRIGGER reject_dictionary_publication").Error; err != nil {
			t.Fatal(err)
		}
		if err := database.database.Model(&mediaOperationClaimRecord{}).Where("operation_id = ?", id).Update("expires_at", time.Now().Add(-time.Hour)).Error; err != nil {
			t.Fatal(err)
		}
		if fault == "usage" {
			var before int64
			if err := database.database.Model(&managedJournalObservationRecord{}).Count(&before).Error; err != nil {
				t.Fatal(err)
			}
			if before != 0 {
				t.Fatal("failed usage write retained an observation")
			}
			if err := database.database.Exec("DROP TRIGGER reject_dictionary_usage").Error; err != nil {
				t.Fatal(err)
			}
		}
		service.runOperation("recovery-worker", id)
		var observations int64
		if err := database.database.Model(&managedJournalObservationRecord{}).Count(&observations).Error; err != nil {
			t.Fatal(err)
		}
		if observations != 1 {
			t.Fatalf("recovery retained %d observations, want one", observations)
		}
	}
	result := hostedMediaWorkerStatus(t, server, id)
	if result["state"] != MediaOperationStateSucceeded || len(result["outputs"].([]any)) != 1 {
		t.Fatalf("service result=%v", result)
	}
	entry := read("")["requests"].([]any)[0].(map[string]any)
	if _, present := entry["model"]; present {
		t.Fatalf("service journal invented model: %v", entry)
	}
	wantUsage := journalUsageUnknown
	if operation == ModelOperationPronunciationDictionaryCreation || requestID != "" {
		wantUsage = journalUsageComplete
	}
	if entry["state"] != string(journalRequestCompleted) || entry["operation"] != operation || entry["usage_state"] != string(wantUsage) {
		t.Fatalf("service journal=%v", entry)
	}
	var attempt managedJournalAttemptRecord
	if err := database.database.Where("request_id = ?", entry["id"]).First(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	wantRequestID := requestID
	if operation == ModelOperationPronunciationDictionaryCreation {
		var retained mediaOperationRecord
		if err := database.database.Where("operation_id = ?", id).First(&retained).Error; err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(retained.ProviderHandle, "private dictionary") {
			t.Fatal("operation lost the recovery receipt")
		}
	} else {
		var retained mediaOperationRecord
		if err := database.database.Where("operation_id = ?", id).First(&retained).Error; err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(retained.ProviderHandle, "Names") || requestID != "" && !strings.Contains(retained.ProviderHandle, requestID) {
			t.Fatal("alignment lost its output or provider trace receipt")
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

func testHostedAlignmentStoredReceiptFailure(t *testing.T, database *gormManagedTenantDatabase, service *mediaOperationService, server *httptest.Server, id, fault string, submissions *atomic.Int64) {
	t.Helper()
	var original mediaOperationRecord
	if err := database.database.Where("operation_id = ?", id).First(&original).Error; err != nil {
		t.Fatal(err)
	}
	var initialObservations int64
	if err := database.database.Model(&managedJournalObservationRecord{}).Count(&initialObservations).Error; err != nil {
		t.Fatal(err)
	}
	var receipt map[string]any
	if err := json.Unmarshal([]byte(original.ProviderHandle), &receipt); err != nil {
		t.Fatal(err)
	}
	changes := map[string]any{}
	switch fault {
	case "stored_json":
		changes["provider_handle"] = "{"
	case "stored_output":
		receipt["output"] = map[string]any{"characters": []any{}, "words": []any{}}
	case "stored_start":
		delete(receipt, "started_at")
	case "stored_end":
		receipt["completed_at"] = "2020-01-01T00:00:00Z"
	case "stored_trace":
		receipt["trace_id"] = " private-alignment-trace "
	case "stored_binding":
		changes["execution_binding"] = "changed-alignment-binding"
	}
	if len(changes) == 0 {
		encoded, err := json.Marshal(receipt)
		if err != nil {
			t.Fatal(err)
		}
		changes["provider_handle"] = string(encoded)
	}
	if err := database.database.Model(&mediaOperationRecord{}).Where("operation_id = ?", id).Updates(changes).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := database.database.Model(&mediaOperationClaimRecord{}).Where("operation_id = ?", id).Update("expires_at", time.Now().Add(-time.Hour)).Error; err != nil {
			t.Fatal(err)
		}
		service.runOperation("invalid-receipt-worker", id)
		result := hostedMediaWorkerStatus(t, server, id)
		var outputs, observations int64
		if err := database.database.Model(&mediaOperationAssetReferenceRecord{}).Where("operation_id = ? AND role = ?", id, "output").Count(&outputs).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.database.Model(&managedJournalObservationRecord{}).Count(&observations).Error; err != nil {
			t.Fatal(err)
		}
		if result["state"] != MediaOperationStateRunning || outputs != 0 || observations != initialObservations || submissions.Load() != 1 {
			t.Fatalf("invalid receipt changed accepted work: result=%v outputs=%d observations=%d calls=%d", result, outputs, observations, submissions.Load())
		}
	}
	if err := database.database.Model(&mediaOperationRecord{}).Where("operation_id = ?", id).Updates(map[string]any{"provider_handle": original.ProviderHandle, "execution_binding": original.ExecutionBinding}).Error; err != nil {
		t.Fatal(err)
	}
}
