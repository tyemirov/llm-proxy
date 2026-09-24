package proxy

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func corruptRetainedPaymentAdjustment(t *testing.T, database *gormManagedTenantDatabase, orderID, scenario string) func() {
	t.Helper()
	var original managedPaymentAdjustmentRecord
	if err := database.database.First(&original, "order_id = ?", orderID).Error; err != nil {
		t.Fatal(err)
	}
	evidence, digest := original.Evidence, original.EvidenceDigest
	switch scenario {
	case "malformed":
		evidence = "{"
	case "null":
		evidence = "null"
	case "changed-total":
		evidence = strings.Replace(evidence, `"total":"550"`, `"total":"549"`, 1)
		if evidence == original.Evidence {
			t.Fatal("retained total was not changed")
		}
	case "digest":
		digest = "corrupt-evidence-digest"
	default:
		t.Fatalf("unknown corruption scenario %q", scenario)
	}
	write := func(evidence, digest string) {
		t.Helper()
		if err := database.database.Model(&managedPaymentAdjustmentRecord{}).Where("order_id = ?", orderID).Updates(map[string]any{"evidence": evidence, "evidence_digest": digest}).Error; err != nil {
			t.Fatal(err)
		}
	}
	write(evidence, digest)
	return func() { write(original.Evidence, original.EvidenceDigest) }
}

func TestHostedPaymentsRetainedAdjustmentIntegrityPreservesRefundFunds(t *testing.T) {
	for _, scenario := range []string{"malformed", "null", "changed-total", "digest"} {
		for _, transition := range []string{"approval", "replay"} {
			t.Run(scenario+"/"+transition, func(t *testing.T) {
				fixture := newPaymentAuditFixture(t)
				fixture.applyAdjustment(t, "pending_approval", 200)
				before := paymentAdjustmentResources(t, fixture)
				var revisions []managedPaymentAdjustmentRevisionRecord
				if err := fixture.database.database.Order("revision").Find(&revisions).Error; err != nil {
					t.Fatal(err)
				}
				status, revision := "approved", 2
				if transition == "replay" {
					status, revision = "pending_approval", 1
				}
				next := paymentAdjustmentFixture(fixture.processor, status, 200, revision)
				sendPaymentEventFixture(t, fixture.database, next, "adjustment.updated", 3)
				restore := corruptRetainedPaymentAdjustment(t, fixture.database, fixture.orderID, scenario)
				err := paymentProcessorFixture(t, fixture.checkout, fixture.database).reconcile(t.Context())
				restore()
				if err == nil {
					t.Fatal("changed retained adjustment evidence was accepted")
				}
				recovery := adjustmentReadFixture{fixture, next}
				recovery.assertUnapplied(t, before, paymentInboxPending)
				var after []managedPaymentAdjustmentRevisionRecord
				if err := fixture.database.database.Order("revision").Find(&after).Error; err != nil || !reflect.DeepEqual(revisions, after) {
					t.Fatalf("failed adjustment changed immutable revisions: error=%v", err)
				}
				if transition == "replay" {
					worker := paymentProcessorFixture(t, fixture.checkout, openJournalTransactionInstance(t, fixture.database))
					if err := worker.reconcile(t.Context()); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(before, paymentAdjustmentResources(t, fixture)) {
						t.Fatal("restored replay changed financial resources")
					}
					recovery.next = paymentAdjustmentFixture(fixture.processor, "approved", 200, 2)
					sendPaymentEventFixture(t, fixture.database, recovery.next, "adjustment.updated", 4)
				}
				recovery.recover(t)
			})
		}
	}
}

func TestHostedPaymentsReceiptRejectsChangedRetainedAdjustment(t *testing.T) {
	for _, scenario := range []string{"malformed", "null", "changed-total", "digest"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newPaymentAuditFixture(t)
			fixture.applyAdjustment(t, "pending_approval", 200)
			before := paymentAdjustmentResources(t, fixture)
			restore := corruptRetainedPaymentAdjustment(t, fixture.database, fixture.orderID, scenario)
			response := paymentOrderHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+fixture.orderID+"/receipt", "", "", http.StatusServiceUnavailable)
			restore()
			if !reflect.DeepEqual(response, map[string]any{"error": map[string]any{"code": errFundingUnavailable.Error()}}) {
				t.Fatalf("corrupt receipt exposed partial or private data: %v", response)
			}
			if !reflect.DeepEqual(before, paymentAdjustmentResources(t, fixture)) {
				t.Fatal("receipt restoration changed financial resources")
			}
		})
	}
}
