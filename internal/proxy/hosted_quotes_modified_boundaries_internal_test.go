package proxy

import (
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
)

func TestHostedQuotePublicMethodsDeclareResourcesWithoutEffects(t *testing.T) {
	fixture := newQuotedFixture(t)
	registerClientMethodErrors(fixture.server.Config.Handler.(*gin.Engine))
	for _, scenario := range []struct {
		path, allow string
		status      int
	}{
		{llmproxycontract.FinancialQuotesPath, "GET, POST", 405},
		{llmproxycontract.FinancialQuotesPath + "/quote-" + strings.Repeat("a", 32), "GET, PATCH", 405},
		{llmproxycontract.FinancialQuotesPath + "/quote-" + strings.Repeat("a", 32) + "/executions", "GET, POST", 405},
		{llmproxycontract.FinancialQuotesPath + "/quote-" + strings.Repeat("a", 32) + "/other", "", 404},
		{llmproxycontract.FinancialQuotesPath + "/quote-" + strings.Repeat("a", 32) + "/executions/other", "", 404},
	} {
		request, _ := http.NewRequest("DELETE", fixture.server.URL+scenario.path, nil)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != scenario.status || response.Header.Get("Allow") != scenario.allow {
			t.Fatalf("path=%s status=%d allow=%q", scenario.path, response.StatusCode, response.Header.Get("Allow"))
		}
		if scenario.status == 405 && (response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Vary") != "Authorization") {
			t.Fatal("financial method response cacheable")
		}
	}
	if fixture.calls.Load() != 0 {
		t.Fatal("method rejection dispatched")
	}
}

func TestHostedQuoteFinancialIdentityReadFailures(t *testing.T) {
	for _, scenario := range []string{"identity mismatch", "identity IO"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newQuotedFixture(t)
			q := fixture.exchange(t, "POST", "", "finance-identity", quoteNativeInput(), 201)
			db := fixture.database.database
			if scenario == "identity mismatch" {
				if err := db.Model(&managedFinancialQuoteRecord{}).Where("id = ?", q["id"]).Update("request_digest", strings.Repeat("f", 64)).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				db.Callback().Query().Before("gorm:query").Register("quote:financial-identity", func(tx *gorm.DB) {
					if tx.Statement.Table == "managed_financial_quote_records" {
						tx.AddError(errors.New("controlled financial identity read failure"))
					}
				})
				defer db.Callback().Query().Remove("quote:financial-identity")
			}
			request, _ := http.NewRequest("GET", fixture.server.URL+llmproxycontract.FinancialRequestsPath+"/"+q["request_id"].(string), nil)
			request.Header.Set("Authorization", "Bearer "+hostedIdentityFixtureKey)
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != 503 || strings.Contains(string(body), "controlled") {
				t.Fatalf("status=%d body=%s", response.StatusCode, body)
			}
			if fixture.calls.Load() != 0 {
				t.Fatal("financial read dispatched")
			}
		})
	}
}

func TestHostedQuoteStartupRejectsQuoteStorageFailureAtomically(t *testing.T) {
	fixture := newPaymentStartupFixture(t)
	configuration := fixture.configuration
	var failures atomic.Int64
	configuration.Management.DatabaseDialector = financialSchemaFailureDialector{Dialector: configuration.Management.DatabaseDialector, table: "managed_financial_quote_records", operation: "columns", failures: &failures}
	fixture.reject(t, configuration, "inspect table=managed_financial_quote_records")
	if failures.Load() != 1 {
		t.Fatal("quote storage failure not exercised")
	}
	fixture.recover(t)
}

func newFailedQuotedRecovery(t *testing.T) (quotedFixture, map[string]any) {
	t.Helper()
	fixture := newQuotedFixture(t)
	fixture.output.Store(`{"id":"paid-invalid-result","status":"completed","output_text":"invalid","usage":{"input_tokens":1000,"output_tokens":100,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	q := fixture.exchange(t, "POST", "", "recovery-failure", quoteNativeInput(), 201)
	fixture.exchange(t, "POST", "/"+q["id"].(string)+"/executions", "", quoteNativeInput(), 502)
	return fixture, q
}

func TestHostedQuoteFailedRecoveryProofAndIOBoundaries(t *testing.T) {
	for _, scenario := range []string{"quote read failure", "terminal proof read failure", "unfinished attempt", "incomplete usage", "post-settlement reservation read"} {
		t.Run(scenario, func(t *testing.T) {
			fixture, q := newFailedQuotedRecovery(t)
			db := fixture.database.database
			name := "quote:failed-recovery"
			hits := 0
			wantError := false
			switch scenario {
			case "incomplete usage":
				if err := db.Model(&managedJournalRequestRecord{}).Where("id = ?", q["request_id"]).Update("usage_state", journalUsageUnknown).Error; err != nil {
					t.Fatal(err)
				}
			case "unfinished attempt":
				if err := db.Model(&managedJournalAttemptRecord{}).Where("request_id = ?", q["request_id"]).Update("state", journalAttemptDispatched).Error; err != nil {
					t.Fatal(err)
				}
			default:
				wantError = true
				db.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
					table := "managed_financial_quote_records"
					nth := 1
					if scenario == "terminal proof read failure" {
						table = "managed_journal_attempt_records"
						nth = 2
					}
					if scenario == "post-settlement reservation read" {
						table = "managed_funds_reservation_records"
						nth = 3
					}
					if tx.Statement.Table == table {
						hits++
						if hits == nth {
							tx.AddError(errors.New("controlled failed recovery IO failure"))
						}
					}
				})
				defer db.Callback().Query().Remove(name)
			}
			err := fixture.database.reconcileHostedReservation(t.Context(), q["request_id"].(string), ratingTestAcceptanceTime())
			db.Callback().Query().Remove(name)
			if wantError && err == nil || !wantError && err != nil {
				t.Fatalf("hits=%d err=%v", hits, err)
			}
			var reservation managedFundsReservationRecord
			if err := db.First(&reservation, "request_id = ?", q["request_id"]).Error; err != nil {
				t.Fatal(err)
			}
			want := fundsReservationReconciliation
			if wantError {
				want = fundsReservationHeld
			}
			if reservation.State != want {
				t.Fatalf("reservation=%s want=%s", reservation.State, want)
			}
			if fixture.calls.Load() != 1 {
				t.Fatal("financial recovery redispatched")
			}
		})
	}
}

func TestHostedQuoteFailedRecoverySettlesPreviouslyDeliveredCharge(t *testing.T) {
	fixture, q := newFailedQuotedRecovery(t)
	db := fixture.database.database
	// Reproduce the durable ordering where usage delivery completes before the
	// execution owner publishes its terminal processing failure.
	if err := db.Model(&managedJournalRequestRecord{}).Where("id = ?", q["request_id"]).Update("state", journalRequestExecuting).Error; err != nil {
		t.Fatal(err)
	}
	if err := deliverFundsFixture(t, fixture.database); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&managedJournalRequestRecord{}).Where("id = ?", q["request_id"]).Update("state", journalRequestFailed).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.database.reconcileHostedReservation(t.Context(), q["request_id"].(string), ratingTestAcceptanceTime()); err != nil {
		t.Fatal(err)
	}
	var reservation managedFundsReservationRecord
	if err := db.First(&reservation, "request_id = ?", q["request_id"]).Error; err != nil || reservation.State != fundsReservationSettled {
		t.Fatalf("reservation=%s err=%v", reservation.State, err)
	}
	if fixture.calls.Load() != 1 {
		t.Fatal("recovery redispatched")
	}
}

func TestHostedQuoteUnconsumedReservationWaitsForQuoteRecovery(t *testing.T) {
	fixture := newQuotedFixture(t)
	q := fixture.exchange(t, "POST", "", "reserved-not-dispatched", quoteNativeInput(), 201)
	if err := fixture.database.reconcileHostedReservation(t.Context(), q["request_id"].(string), ratingTestAcceptanceTime()); err != nil {
		t.Fatal(err)
	}
	retained := fixture.exchange(t, "GET", "/"+q["id"].(string), "", nil, 200)
	if retained["state"] != "reserved" {
		t.Fatal(retained)
	}
	assertHostedFundsBalance(t, fixture.database, 5, 2)
}

func TestHostedQuoteMediaHoldSurvivesFundsRecoveryDuringProviderExecution(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	encoded := base64.StdEncoding.EncodeToString(imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 1024, 1024))))
	fixture := newFundedMediaRecoveryFixtureWithResponse(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(started)
		select {
		case <-release:
		case <-request.Context().Done():
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(writer, `{"data":[{"b64_json":%q}],"usage":{"total_tokens":22,"input_tokens":13,"output_tokens":9,"input_tokens_details":{"text_tokens":10,"image_tokens":3},"output_tokens_details":{"text_tokens":2,"image_tokens":7}}}`, encoded)
	}))
	done := make(chan struct{})
	go func() { defer close(done); fixture.worker.runOperation("live-media-hold", fixture.operationID) }()
	var once sync.Once
	defer func() { once.Do(func() { close(release) }); <-done }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("provider dispatch did not start")
	}
	var request managedJournalRequestRecord
	if err := fixture.database.database.First(&request, "execution_id = ?", fixture.operationID).Error; err != nil {
		t.Fatal(err)
	}
	if request.State != journalRequestExecuting {
		t.Fatalf("provider not running: %s", request.State)
	}
	if err := fixture.database.reconcileHostedReservation(t.Context(), request.ID, request.ClaimExpiresAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	assertHostedFundsBalance(t, fixture.database, 500, 461)
	if fixture.calls.Load() != 1 {
		t.Fatal("funds recovery repeated live dispatch")
	}
	once.Do(func() { close(release) })
	<-done
	if current := hostedMediaWorkerStatus(t, fixture.server, fixture.operationID); current["state"] != MediaOperationStateSucceeded {
		t.Fatal(current)
	}
}
