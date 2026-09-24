package proxy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"github.com/tyemirov/utils/billing"
	"gorm.io/gorm"
)

func paymentAdjustmentResources(t *testing.T, fixture paymentAuditFixture) map[string]any {
	t.Helper()
	order := paymentOrdersTestPath + "/" + fixture.orderID
	resources := map[string]any{}
	for _, path := range []string{order, order + "/receipt", fundsBalanceTestPath, "/billing-accounts/billing-journal/ledger-entries?limit=100"} {
		resources[path] = paymentOrderHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, path, "", "", http.StatusOK)
	}
	return resources
}

type adjustmentReadFixture struct {
	paymentAuditFixture
	next map[string]any
}

func newAdjustmentReadFixture(t *testing.T) adjustmentReadFixture {
	t.Helper()
	fixture := newPaymentAuditFixture(t)
	fixture.applyAdjustment(t, "pending_approval", 200)
	next := paymentAdjustmentFixture(fixture.processor, "approved", 200, 2)
	sendPaymentEventFixture(t, fixture.database, next, "adjustment.updated", 3)
	return adjustmentReadFixture{fixture, next}
}

func (fixture adjustmentReadFixture) assertUnapplied(t *testing.T, before map[string]any, state string) managedPaymentInboxRecord {
	t.Helper()
	if !reflect.DeepEqual(before, paymentAdjustmentResources(t, fixture.paymentAuditFixture)) {
		t.Fatal("unapplied refund changed financial resources")
	}
	var event managedPaymentInboxRecord
	if err := fixture.database.database.Where("event_id = ?", "evt_00000000000000000000000003").First(&event).Error; err != nil || event.State != state {
		t.Fatalf("refund event not retained: event=%+v error=%v", event, err)
	}
	return event
}

func (fixture adjustmentReadFixture) recover(t *testing.T) {
	t.Helper()
	worker := paymentProcessorFixture(t, fixture.checkout, openJournalTransactionInstance(t, fixture.database))
	resumedAt := time.Now().Add(2 * paymentCheckoutRetry)
	worker.now = func() time.Time { return resumedAt }
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	balance := fixture.balance(t)
	if balance["posted_cents"] != "300" || balance["available_cents"] != "300" {
		t.Fatalf("refund recovery balance=%v", balance)
	}
	after := paymentAdjustmentResources(t, fixture.paymentAuditFixture)
	sendPaymentEventFixture(t, fixture.database, fixture.next, "adjustment.updated", 4)
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, paymentAdjustmentResources(t, fixture.paymentAuditFixture)) {
		t.Fatal("replayed refund repeated financial effects")
	}
	var event managedPaymentInboxRecord
	if err := fixture.database.database.Where("event_id = ?", "evt_00000000000000000000000003").First(&event).Error; err != nil || event.State != paymentInboxApplied {
		t.Fatalf("recovered refund not acknowledged: event=%+v error=%v", event, err)
	}
}

func TestHostedPaymentsAdjustmentReadFailuresPreserveRefundHold(t *testing.T) {
	for _, table := range []string{"managed_payment_inbox_records", "managed_payment_checkout_records", "managed_funding_order_records", "managed_payment_receipt_records", "managed_payment_adjustment_records", "managed_payment_state_observation_records", "ledger_accounts", "reservations"} {
		t.Run(table, func(t *testing.T) {
			fixture := newAdjustmentReadFixture(t)
			before := paymentAdjustmentResources(t, fixture.paymentAuditFixture)
			var failures atomic.Int64
			callback := fixture.database.database.Callback().Query()
			if err := callback.Before("gorm:query").Register("test:refund_read", func(tx *gorm.DB) {
				if !tx.DryRun && tx.Statement.Table == table {
					failures.Add(1)
					tx.AddError(errors.New("controlled_refund_read_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			err := paymentProcessorFixture(t, fixture.checkout, fixture.database).reconcile(t.Context())
			if removeErr := callback.Remove("test:refund_read"); removeErr != nil {
				t.Fatal(removeErr)
			}
			if err == nil || failures.Load() == 0 {
				t.Fatalf("refund read failure ignored: error=%v failures=%d", err, failures.Load())
			}
			fixture.assertUnapplied(t, before, paymentInboxPending)
			fixture.recover(t)
		})
	}
}

func TestHostedPaymentsAdjustmentBalanceFailuresRollBackRefund(t *testing.T) {
	for _, table := range []string{"ledger_entries", "reservations"} {
		t.Run(table, func(t *testing.T) {
			fixture := newAdjustmentReadFixture(t)
			before := paymentAdjustmentResources(t, fixture.paymentAuditFixture)
			var failures atomic.Int64
			callback := fixture.database.database.Callback().Row()
			if err := callback.Before("gorm:row").Register("test:refund_balance_read", func(tx *gorm.DB) {
				if !tx.DryRun && tx.Statement.Table == table {
					failures.Add(1)
					tx.AddError(errors.New("controlled_refund_balance_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			err := paymentProcessorFixture(t, fixture.checkout, fixture.database).reconcile(t.Context())
			if removeErr := callback.Remove("test:refund_balance_read"); removeErr != nil {
				t.Fatal(removeErr)
			}
			if err == nil || failures.Load() == 0 {
				t.Fatalf("refund balance failure ignored: error=%v failures=%d", err, failures.Load())
			}
			fixture.assertUnapplied(t, before, paymentInboxPending)
			fixture.recover(t)
		})
	}
}

func TestHostedPaymentsAdjustmentCorruptProjectionPreservesRefundHold(t *testing.T) {
	for _, column := range []string{"evidence", "hold_id"} {
		t.Run(column, func(t *testing.T) {
			fixture := newAdjustmentReadFixture(t)
			before := paymentAdjustmentResources(t, fixture.paymentAuditFixture)
			var projection managedPaymentAdjustmentRecord
			if err := fixture.database.database.First(&projection, "order_id = ?", fixture.orderID).Error; err != nil {
				t.Fatal(err)
			}
			original, replacement := projection.Evidence, "{"
			if column == "hold_id" {
				original, replacement = projection.HoldID, ""
			}
			if err := fixture.database.database.Model(&projection).UpdateColumn(column, replacement).Error; err != nil {
				t.Fatal(err)
			}
			if err := paymentProcessorFixture(t, fixture.checkout, fixture.database).reconcile(t.Context()); err == nil {
				t.Fatal("corrupt refund projection accepted")
			}
			if err := fixture.database.database.Model(&projection).UpdateColumn(column, original).Error; err != nil {
				t.Fatal(err)
			}
			fixture.assertUnapplied(t, before, paymentInboxPending)
			fixture.recover(t)
		})
	}
}

func TestHostedPaymentsAdjustmentProcessorOutagesRetainRetryableEvidence(t *testing.T) {
	for _, scenario := range []struct {
		name, path, reason string
		rejectCheckpoint   bool
	}{
		{"transaction", "/transactions/", "transaction_unavailable", false},
		{"adjustments", "/adjustments", "adjustment_evidence_unavailable", false},
		{"transaction-checkpoint", "/transactions/", "transaction_unavailable", true},
		{"adjustments-checkpoint", "/adjustments", "adjustment_evidence_unavailable", true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newAdjustmentReadFixture(t)
			before := paymentAdjustmentResources(t, fixture.paymentAuditFixture)
			var failures atomic.Int64
			processor := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if strings.HasPrefix(request.URL.Path, scenario.path) {
					failures.Add(1)
					writer.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				fixture.processor.server.Config.Handler.ServeHTTP(writer, request)
			}))
			t.Cleanup(processor.Close)
			client, err := billing.NewPaddleCommerceClient("sandbox", "checkout-fixture-key", processor.URL, processor.Client())
			if err != nil {
				t.Fatal(err)
			}
			worker, err := newPaddlePaymentProcessor(fixture.database, fixture.checkout.catalog, client)
			if err != nil {
				t.Fatal(err)
			}
			if scenario.rejectCheckpoint {
				if err := fixture.database.database.Exec("CREATE TRIGGER reject_refund_retry BEFORE UPDATE ON managed_payment_inbox_records WHEN NEW.state = 'reconciliation_required' BEGIN SELECT RAISE(ABORT, 'controlled_refund_retry_failure'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			err = worker.reconcile(t.Context())
			if failures.Load() == 0 || (err != nil) != scenario.rejectCheckpoint {
				t.Fatalf("outage result mismatch: failures=%d error=%v", failures.Load(), err)
			}
			expectedState := paymentInboxReconciliation
			if scenario.rejectCheckpoint {
				expectedState = paymentInboxPending
			}
			event := fixture.assertUnapplied(t, before, expectedState)
			if !scenario.rejectCheckpoint && event.Reason != scenario.reason {
				t.Fatalf("retry reason=%s want=%s", event.Reason, scenario.reason)
			}
			if scenario.rejectCheckpoint {
				if err := fixture.database.database.Exec("DROP TRIGGER reject_refund_retry").Error; err != nil {
					t.Fatal(err)
				}
			}
			fixture.recover(t)
		})
	}
}

func TestHostedPaymentsAdjustmentHoldDeficitRefreshesBeforeUsage(t *testing.T) {
	for _, scenario := range []string{"success", "order-read", "retained-evidence", "revision-write"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newPaymentAuditFixture(t)
			account, err := newHostedLedgerAccount(fixture.database.database, "billing-journal", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			amount, err := ledger.NewPositiveAmountCents(400)
			if err != nil {
				t.Fatal(err)
			}
			reservation, err := ledger.NewReservationID("controlled-prior-usage")
			if err != nil {
				t.Fatal(err)
			}
			holdKey, err := ledger.NewIdempotencyKey("controlled-prior-hold")
			if err != nil {
				t.Fatal(err)
			}
			metadata, err := ledger.NewMetadataJSON(`{"source":"controlled-prior-usage"}`)
			if err != nil {
				t.Fatal(err)
			}
			if err := account.service.Reserve(t.Context(), account.tenant, account.user, account.namespace, amount, reservation, holdKey, 0, metadata); err != nil {
				t.Fatal(err)
			}
			fixture.applyAdjustment(t, "pending_approval", 200)
			balance := fixture.balance(t)
			if balance["posted_cents"] != "500" || balance["available_cents"] != "0" || balance["state"] != "suspended" {
				t.Fatalf("refund shortfall not restricted: %v", balance)
			}
			releaseKey, err := ledger.NewIdempotencyKey("controlled-prior-release")
			if err != nil {
				t.Fatal(err)
			}
			if err := account.service.Release(t.Context(), account.tenant, account.user, account.namespace, reservation, releaseKey, metadata); err != nil {
				t.Fatal(err)
			}
			var projection managedPaymentAdjustmentRecord
			if err := fixture.database.database.First(&projection, "order_id = ?", fixture.orderID).Error; err != nil {
				t.Fatal(err)
			}
			if projection.PendingCents != 200 || projection.HeldCents != 100 {
				t.Fatalf("expected partial refund hold: %+v", projection)
			}
			var calls atomic.Int64
			upstream := fundsUpstream(t, &calls)
			generation := newHostedIdentityHTTPServer(t, fixture.database, upstream.URL, t.TempDir(), fundsDependencies(hostedRatingFixtureAdmission(t, 2)))
			before := paymentAdjustmentResources(t, fixture)
			var failures atomic.Int64
			retainedEvidence := projection.Evidence
			switch scenario {
			case "order-read":
				if err := fixture.database.database.Callback().Query().Before("gorm:query").Register("test:refund_deficit_order", func(tx *gorm.DB) {
					if !tx.DryRun && tx.Statement.Table == "managed_funding_order_records" {
						failures.Add(1)
						tx.AddError(errors.New("controlled_refund_deficit_order_failure"))
					}
				}); err != nil {
					t.Fatal(err)
				}
			case "retained-evidence":
				if err := fixture.database.database.Model(&projection).UpdateColumn("evidence", "{").Error; err != nil {
					t.Fatal(err)
				}
			case "revision-write":
				if err := fixture.database.database.Exec("CREATE TRIGGER reject_refund_refresh BEFORE INSERT ON managed_payment_adjustment_revision_records BEGIN SELECT RAISE(ABORT, 'controlled_refund_refresh_failure'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			if scenario != "success" {
				body := hostedIdentityHTTP(t, generation, "refresh-refund-hold", "funded prompt", http.StatusServiceUnavailable)
				if !strings.Contains(body, `"code":"financial_admission_unavailable"`) || strings.Contains(body, "controlled_") || calls.Load() != 0 {
					t.Fatalf("unsafe refund refresh: body=%s calls=%d", body, calls.Load())
				}
				switch scenario {
				case "order-read":
					if err := fixture.database.database.Callback().Query().Remove("test:refund_deficit_order"); err != nil {
						t.Fatal(err)
					}
					if failures.Load() == 0 {
						t.Fatal("order read failure not exercised")
					}
				case "retained-evidence":
					if err := fixture.database.database.Model(&projection).UpdateColumn("evidence", retainedEvidence).Error; err != nil {
						t.Fatal(err)
					}
				case "revision-write":
					if err := fixture.database.database.Exec("DROP TRIGGER reject_refund_refresh").Error; err != nil {
						t.Fatal(err)
					}
				}
				if !reflect.DeepEqual(before, paymentAdjustmentResources(t, fixture)) {
					t.Fatal("failed refund refresh changed financial resources")
				}
				var count int64
				if err := fixture.database.database.Model(&managedJournalRequestRecord{}).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("failed refresh retained request: count=%d error=%v", count, err)
				}
			}
			for range 2 {
				hostedIdentityHTTP(t, generation, "refresh-refund-hold", "funded prompt", http.StatusOK)
			}
			balance = fixture.balance(t)
			if balance["posted_cents"] != "500" || balance["available_cents"] != "297" || balance["state"] != "active" || calls.Load() != 1 {
				t.Fatalf("refund refresh did not reserve before usage: balance=%v calls=%d", balance, calls.Load())
			}
			var refreshed managedPaymentAdjustmentRecord
			if err := fixture.database.database.First(&refreshed, "order_id = ?", fixture.orderID).Error; err != nil {
				t.Fatal(err)
			}
			if refreshed.HeldCents != 200 || refreshed.Revision != projection.Revision+1 || refreshed.EvidenceDigest != projection.EvidenceDigest || refreshed.ReversedCents != 0 {
				t.Fatalf("refund refresh changed evidence or repeated: %+v", refreshed)
			}
		})
	}
}
