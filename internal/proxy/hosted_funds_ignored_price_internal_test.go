package proxy

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestHostedFundsIgnoredPriceInsertPreventsAdmission(t *testing.T) {
	fixture := newFundsAdmissionFixture(t)
	before := fixture.state(t)
	if err := fixture.database.database.Exec(`CREATE TRIGGER ignore_price_snapshot
		BEFORE INSERT ON managed_price_snapshot_records BEGIN SELECT RAISE(IGNORE); END`).Error; err != nil {
		t.Fatal(err)
	}
	const key = "ignored-price-snapshot"
	for range 2 {
		body := hostedIdentityHTTP(t, fixture.generation, key, "funded prompt", http.StatusServiceUnavailable)
		if !strings.Contains(body, `"code":"financial_admission_unavailable"`) || fixture.calls.Load() != 0 || !reflect.DeepEqual(before, fixture.state(t)) {
			t.Fatalf("ignored price insert changed admission: calls=%d body=%s", fixture.calls.Load(), body)
		}
	}
	for _, model := range []any{&managedJournalRequestRecord{}, &managedPriceSnapshotRecord{}, &managedFundsReservationRecord{}} {
		var count int64
		if err := fixture.database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("ignored price retained partial %T records=%d error=%v", model, count, err)
		}
	}
	if err := fixture.database.database.Exec("DROP TRIGGER ignore_price_snapshot").Error; err != nil {
		t.Fatal(err)
	}
	if body := hostedIdentityHTTP(t, fixture.generation, key, "funded prompt", http.StatusOK); body != "funded result" || fixture.calls.Load() != 1 {
		t.Fatalf("restored price admission: calls=%d body=%s", fixture.calls.Load(), body)
	}
	var settled map[string]any
	for iteration := range 3 {
		fixture.generation.Close()
		fixture.generation = newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, fixture.database), fixture.upstreamURL, fixture.responseRoot, fundsDependencies(fixture.prices))
		if body := hostedIdentityHTTP(t, fixture.generation, key, "funded prompt", http.StatusOK); body != "funded result" || fixture.calls.Load() != 1 {
			t.Fatalf("restored price admission: calls=%d body=%s", fixture.calls.Load(), body)
		}
		assertHostedFundsBalance(t, fixture.database, 5, 5)
		assertFundsCreditRemainder(t, fixture.database, "91", "25000")
		current := fixture.state(t)
		if iteration == 0 {
			settled = current
		} else if !reflect.DeepEqual(settled, current) {
			t.Fatal("restored price admission repeated financial effects")
		}
	}
}
