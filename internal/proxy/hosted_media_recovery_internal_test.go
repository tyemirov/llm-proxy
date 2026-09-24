package proxy

import (
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
)

const mediaRecoveryKey = "funded-media-recovery"
const mediaRecoveryPrompt = "funded image"

type fundedMediaRecoveryFixture struct {
	fundsStartupFixture
	server      *httptest.Server
	worker      *mediaOperationService
	upstreamURL string
	operationID string
}

func newFundedMediaRecoveryWorker(t *testing.T, database *gormManagedTenantDatabase, endpoint string, assets *tenantAssetStore) (*httptest.Server, *mediaOperationService) {
	t.Helper()
	server, worker := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(endpoint), func(service *mediaOperationService) {
		if assets != nil {
			service.assets = assets
		}
	})
	settings := hostedImageFinancialSettings(t, ModelOperationImageGeneration)
	worker.catalog = settings.catalog
	worker.hostedAdmission = settings.mediaAdmission(worker.providers)
	key := mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageGenerate, "openai", "gpt-image-2")
	adapter := worker.adapters[key].(*imageGenerationAdapter)
	offering, err := settings.catalog.ResolveOffering("openai", "gpt-image-2")
	if err != nil {
		t.Fatal(err)
	}
	worker.adapters[key] = newImageGenerationAdapter(offering, worker.providers.definitions[providerID("openai")], adapter.tenants, worker.store, worker.assets, settings.catalog)
	return server, worker
}

func newFundedMediaRecoveryFixture(t *testing.T) fundedMediaRecoveryFixture {
	t.Helper()
	encoded := base64.StdEncoding.EncodeToString(imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 1024, 1024))))
	return newFundedMediaRecoveryFixtureWithResponse(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(writer, `{"data":[{"b64_json":%q}],"usage":{"total_tokens":22,"input_tokens":13,"output_tokens":9,"input_tokens_details":{"text_tokens":10,"image_tokens":3},"output_tokens_details":{"text_tokens":2,"image_tokens":7}}}`, encoded)
	}))
}

func newFundedMediaRecoveryFixtureWithResponse(t *testing.T, response http.Handler) fundedMediaRecoveryFixture {
	t.Helper()
	database, _, management, _ := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 500)
	calls := &atomic.Int64{}
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer sk-platform-pinned" {
			t.Error("media recovery changed dispatch authority")
		}
		response.ServeHTTP(writer, request)
	}))
	t.Cleanup(upstream.Close)
	server, worker := newFundedMediaRecoveryWorker(t, database, upstream.URL, nil)
	accepted := hostedMediaAdmissionHTTP(t, server, mediaRecoveryKey, mediaRecoveryPrompt, http.StatusAccepted)
	assertHostedFundsBalance(t, database, 500, 461)
	return fundedMediaRecoveryFixture{fundsStartupFixture{database, management, calls}, server, worker, upstream.URL, accepted["operation_id"].(string)}
}

func (fixture fundedMediaRecoveryFixture) assertFailure(t *testing.T, state string, calls int64) {
	t.Helper()
	current := hostedMediaWorkerStatus(t, fixture.server, fixture.operationID)
	if current["state"] != state || fixture.calls.Load() != calls || len(current["outputs"].([]any)) != 0 {
		t.Fatalf("failed media state=%v calls=%d want=%s/%d", current, fixture.calls.Load(), state, calls)
	}
	assertHostedFundsBalance(t, fixture.database, 500, 461)
	var outputs int64
	if err := fixture.database.database.Model(&mediaOperationAssetReferenceRecord{}).Where("operation_id = ? AND role = ?", fixture.operationID, "output").Count(&outputs).Error; err != nil || outputs != 0 {
		t.Fatalf("failed media published partial outputs=%d error=%v", outputs, err)
	}
}

func (fixture fundedMediaRecoveryFixture) recover(t *testing.T, originalState string) {
	t.Helper()
	wantState, wantCalls := MediaOperationStateUncertain, fixture.calls.Load()
	posted, available := int64(500), int64(461)
	switch originalState {
	case MediaOperationStateQueued:
		wantState, posted, available = MediaOperationStateSucceeded, 498, 498
		wantCalls++
	case MediaOperationStateFailed:
		wantState, wantCalls, available = MediaOperationStateFailed, 0, 500
	}
	now := fixture.worker.store.now().Add(2 * time.Minute)
	var previous map[string]any
	for iteration := 0; iteration < 2; iteration++ {
		database := openJournalTransactionInstance(t, fixture.database)
		server, worker := newFundedMediaRecoveryWorker(t, database, fixture.upstreamURL, fixture.worker.assets)
		worker.store.now = func() time.Time { return now }
		worker.runOperation("recovered-media-worker", fixture.operationID)
		if err := database.reconcileHostedFunds(t.Context(), now); err != nil {
			t.Fatal(err)
		}
		current := hostedMediaWorkerStatus(t, server, fixture.operationID)
		replay := hostedMediaAdmissionHTTP(t, server, mediaRecoveryKey, mediaRecoveryPrompt, http.StatusOK)
		if current["state"] != wantState || replay["operation_id"] != fixture.operationID || fixture.calls.Load() != wantCalls {
			t.Fatalf("media recovery state=%v calls=%d want=%s/%d", current, fixture.calls.Load(), wantState, wantCalls)
		}
		assertHostedFundsBalance(t, database, posted, available)
		financial := fixture.state(t)
		if iteration == 0 {
			previous = financial
		} else if !reflect.DeepEqual(previous, financial) {
			t.Fatal("media restart repeated financial effects")
		}
		server.Close()
	}
}

func TestHostedMediaRecoveryWriteFailuresPreserveFundedOperation(t *testing.T) {
	for _, scenario := range []struct {
		name, statement, state string
		calls                  int64
	}{
		{"claim", "BEFORE INSERT ON media_operation_claim_records", MediaOperationStateQueued, 0},
		{"journal-claim", "BEFORE UPDATE ON managed_journal_request_records WHEN NEW.owner_token != OLD.owner_token", MediaOperationStateQueued, 0},
		{"running", "BEFORE UPDATE ON media_operation_records WHEN OLD.public_state = 'queued' AND NEW.public_state = 'running'", MediaOperationStateQueued, 0},
		{"attempt", "BEFORE INSERT ON managed_journal_attempt_records", MediaOperationStateFailed, 0},
		{"journal-dispatch", "BEFORE UPDATE ON managed_journal_attempt_records WHEN NEW.state = 'dispatched'", MediaOperationStateFailed, 0},
		{"media-dispatch", "BEFORE UPDATE ON media_operation_records WHEN NEW.provider_execution_state = 'dispatched'", MediaOperationStateFailed, 0},
		{"output-reference", "BEFORE INSERT ON media_operation_asset_reference_records WHEN NEW.role = 'output'", MediaOperationStateRunning, 1},
		{"media-completion", "BEFORE UPDATE ON media_operation_records WHEN NEW.public_state = 'succeeded'", MediaOperationStateRunning, 1},
		{"usage-delivery", "BEFORE INSERT ON media_operation_usage_delivery_records", MediaOperationStateRunning, 1},
		{"journal-completion", "BEFORE UPDATE ON managed_journal_request_records WHEN NEW.state = 'completed'", MediaOperationStateRunning, 1},
		{"claim-release", "BEFORE DELETE ON media_operation_claim_records", MediaOperationStateRunning, 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundedMediaRecoveryFixture(t)
			if err := fixture.database.database.Exec("CREATE TRIGGER reject_media_execution " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_media_execution_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			fixture.worker.runOperation("failed-media-worker", fixture.operationID)
			fixture.assertFailure(t, scenario.state, scenario.calls)
			if err := fixture.database.database.Exec("DROP TRIGGER reject_media_execution").Error; err != nil {
				t.Fatal(err)
			}
			fixture.recover(t, scenario.state)
		})
	}
}

func TestHostedMediaRecoveryReadFailuresPreserveFundedOperation(t *testing.T) {
	for _, scenario := range []struct {
		table, state string
		read, calls  int64
	}{
		{"media_operation_records", MediaOperationStateQueued, 1, 0},
		{"media_operation_claim_records", MediaOperationStateQueued, 1, 0},
		{"managed_journal_request_records", MediaOperationStateQueued, 1, 0},
		{"managed_journal_request_records", MediaOperationStateFailed, 2, 0},
		{"media_operation_records", MediaOperationStateFailed, 2, 0},
		{"managed_hosted_grant_records", MediaOperationStateFailed, 1, 0},
		{"managed_journal_attempt_records", MediaOperationStateFailed, 1, 0},
		{"managed_journal_attempt_records", MediaOperationStateFailed, 2, 0},
		{"managed_journal_request_records", MediaOperationStateUncertain, 1, 1},
		{"managed_journal_attempt_records", MediaOperationStateUncertain, 1, 1},
		{"managed_journal_observation_records", MediaOperationStateUncertain, 1, 1},
		{"media_operation_claim_records", MediaOperationStateRunning, 1, 1},
		{"media_operation_records", MediaOperationStateRunning, 1, 1},
		{"managed_journal_attempt_records", MediaOperationStateRunning, 3, 1},
	} {
		t.Run(scenario.table+"/"+strconv.FormatInt(scenario.read, 10)+"/calls-"+strconv.FormatInt(scenario.calls, 10), func(t *testing.T) {
			fixture := newFundedMediaRecoveryFixture(t)
			var reads, failures atomic.Int64
			callback := fixture.database.database.Callback().Query()
			if err := callback.Before("gorm:query").Register("test:media_execution_read", func(tx *gorm.DB) {
				if !tx.DryRun && fixture.calls.Load() == scenario.calls && tx.Statement.Table == scenario.table && reads.Add(1) == scenario.read {
					failures.Add(1)
					tx.AddError(errors.New("controlled_media_execution_read_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			fixture.worker.runOperation("failed-media-worker", fixture.operationID)
			if err := callback.Remove("test:media_execution_read"); err != nil {
				t.Fatal(err)
			}
			if failures.Load() != 1 {
				t.Fatalf("media read failures=%d", failures.Load())
			}
			fixture.assertFailure(t, scenario.state, scenario.calls)
			fixture.recover(t, scenario.state)
		})
	}
}
