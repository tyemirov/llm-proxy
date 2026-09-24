package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestHostedToolLimitReadFailuresPreventUnboundedDispatch(t *testing.T) {
	for _, phase := range []struct {
		name  string
		calls int64
	}{{"initial", 0}, {"continuation", 1}} {
		for _, failure := range []string{"read", "digest"} {
			t.Run(phase.name+"/"+failure, func(t *testing.T) {
				database, _, management, _ := newHostedRatingFixture(t)
				seedHostedFunds(t, database, 100)
				var calls, failures atomic.Int64
				upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					if request.Method == http.MethodDelete {
						writer.WriteHeader(http.StatusNoContent)
						return
					}
					calls.Add(1)
					var payload map[string]json.RawMessage
					if err := json.NewDecoder(request.Body).Decode(&payload); err != nil || string(payload["max_tool_calls"]) != "2" {
						t.Errorf("accepted search limit missing: payload=%v error=%v", payload, err)
					}
					writer.Header().Set("Content-Type", "application/json")
					fmt.Fprint(writer, `{"id":"bounded-search-result","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output_text":"partial answer","output":[{"type":"web_search_call","id":"search-1","status":"completed","action":{"type":"search"}}],"usage":{"input_tokens":10,"output_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
				}))
				t.Cleanup(upstream.Close)
				searchPrices := hostedSearchRatingAuthorization(t)
				configure := func(dependencies *hostedTextRequestDependencies) {
					searchPrices(dependencies)
					prices := dependencies.authorize
					dependencies.now = ratingTestAcceptanceTime
					dependencies.authorize = func(tx *gorm.DB, request managedJournalRequestRecord, intent hostedCompletionIntent) error {
						return newHostedFundsAdmission(func(tx *gorm.DB, request managedJournalRequestRecord) error {
							return prices(tx, request, intent)
						})(tx, request)
					}
				}
				root := t.TempDir()
				server := newHostedIdentityHTTPServer(t, database, upstream.URL, root, configure)
				callback := database.database.Callback().Query()
				const callbackName = "test:accepted_search_limit"
				if err := callback.After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
					if tx.DryRun || tx.Error != nil || tx.Statement.Table != "managed_price_snapshot_records" || hostedTextExecutionFromContext(tx.Statement.Context) == nil || calls.Load() != phase.calls || !failures.CompareAndSwap(0, 1) {
						return
					}
					if failure == "read" {
						tx.AddError(errors.New("controlled_search_limit_read_failure"))
						return
					}
					record := tx.Statement.Dest.(*managedPriceSnapshotRecord)
					record.Document = []byte("corrupt retained price")
				}); err != nil {
					t.Fatal(err)
				}
				hostedSearchHTTP(t, server, "search-limit-failure", http.StatusForbidden)
				if err := callback.Remove(callbackName); err != nil {
					t.Fatal(err)
				}
				if failures.Load() != 1 || calls.Load() != phase.calls {
					t.Fatalf("limit failures=%d provider calls=%d want=%d", failures.Load(), calls.Load(), phase.calls)
				}
				fixture := fundsStartupFixture{database, management, &calls}
				before := fixture.state(t)
				hostedSearchHTTP(t, server, "search-limit-failure", http.StatusBadGateway)
				if !reflect.DeepEqual(before, fixture.state(t)) {
					t.Fatal("failed search replay changed financial resources")
				}
				var accepted managedJournalRequestRecord
				if err := database.database.Where("key_digest = ?", sha256Hex("search-limit-failure")).First(&accepted).Error; err != nil {
					t.Fatal(err)
				}
				var reservation managedFundsReservationRecord
				if err := database.database.Where("request_id = ?", accepted.ID).First(&reservation).Error; err != nil {
					t.Fatal(err)
				}
				assertHostedFundsBalance(t, database, 100, 100-reservation.MaximumCents)
				for _, model := range []any{&managedJournalAttemptRecord{}, &managedJournalObservationRecord{}} {
					var count int64
					if err := database.database.Model(model).Count(&count).Error; err != nil || count != phase.calls {
						t.Fatalf("dispatch evidence %T count=%d want=%d error=%v", model, count, phase.calls, err)
					}
				}
				var recovered map[string]any
				for iteration := range 2 {
					restarted := newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, database), upstream.URL, root, configure, func(dependencies *hostedTextRequestDependencies) {
						dependencies.now = func() time.Time { return accepted.ClaimExpiresAt.Add(time.Second) }
					})
					hostedSearchHTTP(t, restarted, "search-limit-failure", http.StatusBadGateway)
					current := fixture.state(t)
					if iteration == 0 {
						recovered = current
					} else if !reflect.DeepEqual(recovered, current) {
						t.Fatal("repeated restart changed financial resources")
					}
					restarted.Close()
				}
				available, state := int64(100), fundsReservationReleased
				if phase.calls != 0 {
					available, state = 100-reservation.MaximumCents, fundsReservationReconciliation
				}
				assertHostedFundsBalance(t, database, 100, available)
				assertFundsCreditRemainder(t, database, "0", "1")
				retained := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/reservations/"+accepted.ID, "", http.StatusOK)
				if retained["state"] != string(state) || calls.Load() != phase.calls {
					t.Fatalf("recovered search reservation=%v provider calls=%d", retained, calls.Load())
				}
				charges := recovered["charges"].(map[string]any)["charges"].([]any)
				if int64(len(charges)) != phase.calls {
					t.Fatalf("retained search charges=%v want=%d", charges, phase.calls)
				}
				if phase.calls != 0 {
					charge := charges[0].(map[string]any)
					rating := charge["rating"].(map[string]any)
					if !reflect.DeepEqual(rating["provider_cost"], map[string]any{"numerator": "2509", "denominator": "250000"}) || !reflect.DeepEqual(charge["customer_charge"], map[string]any{"numerator": "32617", "denominator": "2500000"}) {
						t.Fatalf("prior search usage lost its exact costs: %v", charge)
					}
				}
			})
		}
	}
}
