package proxy

import (
	"errors"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestHostedDictatorStreamAuthorityFailuresRetainFundsWithoutResubmission(t *testing.T) {
	for _, scenario := range []struct {
		name, direction string
		read            int64
	}{
		{"upload-open", "upload", 1},
		{"upload-metadata", "upload", 2},
		{"upload-audio", "upload", 3},
		{"upload-receipt", "upload", 4},
		{"download-open", "download", 1},
		{"download-request", "download", 2},
		{"download-response", "download", 3},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, _, management, _ := newHostedRatingFixture(t)
			upstream := &hostedDictatorOperationsUpstream{}
			operation := hostedDictatorInputScenarios[0]
			if scenario.direction == "download" {
				operation = hostedDictatorInputScenarios[2]
			}
			server, worker, intent := newHostedDictatorInputFixture(t, database, ModelNameDictatorWhisperBase, upstream, operation)
			settings := hostedDictatorInputFinancialSettings(t, operation)
			worker.catalog = settings.catalog
			worker.hostedAdmission = settings.mediaAdmission(worker.providers)
			seedHostedFunds(t, database, 500)
			id := hostedSpeechHTTP(t, server, scenario.name, intent, http.StatusAccepted)["operation_id"].(string)
			assertHostedFundsBalance(t, database, 500, 448)
			var reads, failures atomic.Int64
			var observed atomic.Bool
			const callback = "test:dictator_stream_authority"
			queries := database.database.Callback().Query()
			creates := database.database.Callback().Create()
			updates := database.database.Callback().Update()
			if scenario.direction == "upload" {
				if err := queries.Before("gorm:query").Register(callback, func(tx *gorm.DB) {
					if tx.DryRun || tx.Statement.Table != "managed_hosted_grant_records" || tx.Statement.Context.Value(hostedMediaAuthorizationContextKey{}) == nil {
						return
					}
					if reads.Add(1) == scenario.read {
						failures.Add(1)
						tx.AddError(errors.New("controlled Dictator upload authority read failure"))
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = queries.Remove(callback) })
			} else {
				if err := creates.After("gorm:create").Register(callback, func(tx *gorm.DB) {
					if !tx.DryRun && tx.Error == nil && tx.Statement.Table == "managed_journal_observation_records" {
						observed.Store(true)
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = creates.Remove(callback) })
				if err := updates.Before("gorm:update").Register(callback, func(tx *gorm.DB) {
					if tx.DryRun || !observed.Load() || tx.Statement.Table != "media_operation_claim_records" || tx.Statement.Context.Value(hostedMediaAuthorizationContextKey{}) == nil {
						return
					}
					if reads.Add(1) == scenario.read {
						failures.Add(1)
						tx.AddError(errors.New("controlled Dictator download claim failure"))
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = updates.Remove(callback) })
			}
			worker.runOperation("stream-authority-worker", id)
			if failures.Load() != 1 || reads.Load() != scenario.read {
				t.Fatalf("stream authority failure did not stop transfer: reads=%d failures=%d", reads.Load(), failures.Load())
			}
			if scenario.direction == "upload" {
				if err := queries.Remove(callback); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := creates.Remove(callback); err != nil {
					t.Fatal(err)
				}
				if err := updates.Remove(callback); err != nil {
					t.Fatal(err)
				}
			}
			current := hostedMediaWorkerStatus(t, server, id)
			if current["state"] != MediaOperationStateUncertain || len(current["outputs"].([]any)) != 0 {
				t.Fatalf("stream authority failure published a result: %v", current)
			}
			wantSubmissions := int64(0)
			if scenario.direction == "download" {
				wantSubmissions = 1
			}
			if upstream.submissions.Load() != wantSubmissions {
				t.Fatalf("stream authority failure submitted unexpected provider work: %d", upstream.submissions.Load())
			}
			financial := fundsStartupFixture{database: database, management: management}
			var previous map[string]any
			for iteration := range 2 {
				restarted := openJournalTransactionInstance(t, database)
				if err := restarted.reconcileHostedFunds(t.Context(), time.Now().UTC()); err != nil {
					t.Fatal(err)
				}
				worker.runOperation("stream-authority-replay", id)
				replay := hostedSpeechHTTP(t, server, scenario.name, intent, http.StatusOK)
				if replay["operation_id"] != id || !reflect.DeepEqual(current, hostedMediaWorkerStatus(t, server, id)) || upstream.submissions.Load() != wantSubmissions {
					t.Fatal("restored authority changed uncertain operation or repeated provider work")
				}
				assertHostedFundsBalance(t, restarted, 500, 448)
				state := financial.state(t)
				charges := state["charges"].(map[string]any)["charges"].([]any)
				if int64(len(charges)) != wantSubmissions {
					t.Fatalf("stream failure lost usage or invented a charge: %v", charges)
				}
				if scenario.direction == "download" {
					charge := charges[0].(map[string]any)
					summary := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/requests/"+charge["request_id"].(string)+"/charge-summary", "", http.StatusOK)
					cost := map[string]any{"numerator": "32001", "denominator": "400000"}
					if summary["state"] != string(requestChargeUnresolved) || summary["customer_charge"] != nil || !reflect.DeepEqual(summary["provider_cost"], cost) {
						t.Fatalf("failed download changed native provider cost or settled the customer: %v", summary)
					}
				}
				if iteration > 0 && !reflect.DeepEqual(previous, state) {
					t.Fatal("repeated stream recovery changed financial resources")
				}
				previous = state
			}
		})
	}
}
