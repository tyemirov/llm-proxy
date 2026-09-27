package proxy

import (
	"encoding/base64"
	"fmt"
	"image"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
)

func TestHostedMediaUsageAuthorityFailurePreservesFunds(t *testing.T) {
	for _, scenario := range []string{"missing-request", "observed-attempt"} {
		t.Run(scenario, func(t *testing.T) {
			encoded := base64.StdEncoding.EncodeToString(imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 1024, 1024))))
			var fixture fundedMediaRecoveryFixture
			var faults atomic.Int64
			fixture = newFundedMediaRecoveryFixtureWithResponse(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				if scenario == "observed-attempt" {
					result := fixture.database.database.Model(&managedJournalAttemptRecord{}).Where("state = ?", journalAttemptDispatched).Update("state", journalAttemptObserved)
					if result.Error != nil || result.RowsAffected != 1 {
						t.Errorf("stored attempt update: rows=%d error=%v", result.RowsAffected, result.Error)
						writer.WriteHeader(http.StatusInternalServerError)
						return
					}
					faults.Add(1)
				}
				writer.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(writer, `{"data":[{"b64_json":%q}],"usage":{"total_tokens":22,"input_tokens":13,"output_tokens":9,"input_tokens_details":{"text_tokens":10,"image_tokens":3},"output_tokens_details":{"text_tokens":2,"image_tokens":7}}}`, encoded)
			}))
			before := fixture.state(t)
			callback := fixture.database.database.Callback().Query()
			const callbackName = "test:missing_media_usage_request"
			if scenario == "missing-request" {
				if err := callback.After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
					if fixture.calls.Load() == 1 && tx.Statement.Table == "managed_journal_request_records" && strings.Contains(tx.Statement.SQL.String(), "execution_kind = ? AND execution_id = ?") && faults.CompareAndSwap(0, 1) {
						tx.AddError(gorm.ErrRecordNotFound)
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = callback.Remove(callbackName) })
			}
			fixture.worker.runOperation("usage-authority-worker", fixture.operationID)
			if scenario == "missing-request" {
				if err := callback.Remove(callbackName); err != nil {
					t.Fatal(err)
				}
			}
			fixture.assertFailure(t, MediaOperationStateUncertain, 1)
			current := hostedMediaWorkerStatus(t, fixture.server, fixture.operationID)
			if faults.Load() != 1 || current["error"].(map[string]any)["code"] != "usage_journal_unavailable" || !reflect.DeepEqual(before, fixture.state(t)) {
				t.Fatalf("usage authority failure changed funds: faults=%d state=%v", faults.Load(), current)
			}
			for _, model := range []any{&managedJournalObservationRecord{}, &managedJournalDeliveryRecord{}, &managedChargeRecord{}, &managedFundsSettlementRecord{}} {
				var count int64
				if err := fixture.database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("usage authority failure retained partial %T records=%d error=%v", model, count, err)
				}
			}
			fixture.recover(t, MediaOperationStateUncertain)
			assertFundsCreditRemainder(t, fixture.database, "0", "1")
		})
	}
}
