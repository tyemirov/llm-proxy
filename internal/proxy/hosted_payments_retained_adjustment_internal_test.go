package proxy

import (
	"encoding/json"
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
	reversed, pending, held, holdID := original.ReversedCents, original.PendingCents, original.HeldCents, original.HoldID
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
	case "reversed-cents":
		reversed++
	case "pending-cents":
		pending++
	case "excess-held-cents":
		held = pending + 1
	case "unheld-id":
		held = 0
	case "hold-id":
		holdID += "-foreign"
	case "invalid-reversed-exact", "negative-reversed-exact", "invalid-pending-exact", "negative-pending-exact", "excess-reversed-exact":
		var values map[string]any
		if err := json.Unmarshal([]byte(evidence), &values); err != nil {
			t.Fatal(err)
		}
		field, replacement := "reversed_exact_cents", "invalid"
		if strings.Contains(scenario, "pending") {
			field = "pending_exact_cents"
		}
		if strings.HasPrefix(scenario, "negative") {
			replacement = "-1"
		}
		if scenario == "excess-reversed-exact" {
			replacement = "9223372036854775808"
		}
		values[field] = replacement
		encoded, err := json.Marshal(values)
		if err != nil {
			t.Fatal(err)
		}
		evidence = string(encoded)
		digest = sha256Hex(evidence)
	default:
		t.Fatalf("unknown corruption scenario %q", scenario)
	}
	write := func(evidence, digest string, reversed, pending, held int64, holdID string) {
		t.Helper()
		if err := database.database.Model(&managedPaymentAdjustmentRecord{}).Where("order_id = ?", orderID).Updates(map[string]any{"evidence": evidence, "evidence_digest": digest, "reversed_cents": reversed, "pending_cents": pending, "held_cents": held, "hold_id": holdID}).Error; err != nil {
			t.Fatal(err)
		}
	}
	write(evidence, digest, reversed, pending, held, holdID)
	return func() {
		write(original.Evidence, original.EvidenceDigest, original.ReversedCents, original.PendingCents, original.HeldCents, original.HoldID)
	}
}

func TestHostedPaymentsRetainedAdjustmentIntegrityPreservesRefundFunds(t *testing.T) {
	for _, scenario := range retainedPaymentAdjustmentCorruptionScenarios {
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
					t.Fatalf("changed retained adjustment evidence was accepted: balance=%v", fixture.balance(t))
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
	for _, scenario := range retainedPaymentAdjustmentCorruptionScenarios {
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

var retainedPaymentAdjustmentCorruptionScenarios = []string{
	"malformed", "null", "changed-total", "digest",
	"reversed-cents", "pending-cents", "excess-held-cents", "unheld-id", "hold-id",
	"invalid-reversed-exact", "negative-reversed-exact", "invalid-pending-exact", "negative-pending-exact", "excess-reversed-exact",
}
