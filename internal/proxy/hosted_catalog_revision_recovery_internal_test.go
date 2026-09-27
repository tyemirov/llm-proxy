package proxy

import (
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestHostedAcceptedRecoveryRejectsChangedCatalogRevision(t *testing.T) {
	fixture := newJournalInterruptionFixture(t, "accepted")
	fixture.generation.Close()
	var price managedPriceSnapshotRecord
	if err := fixture.database.database.First(&price, "request_id = ?", fixture.request.ID).Error; err != nil {
		t.Fatal(err)
	}
	catalog, conditions := hostedRatingTextCatalog()
	catalog.Revision = "qualified-next-catalog"
	settings, err := newHostedRuntimeSettings(&HostedConfiguration{Offerings: []HostedOfferingConfiguration{{Provider: "openai", Model: "gpt-4.1", Operation: ModelOperationText, MaximumAttempts: 1, Conditions: categoricalPriceConditions(conditions)}}}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", hostedJournalFixtureGrantID).UpdateColumn("catalog_revision", catalog.Revision).Error; err != nil {
		t.Fatal(err)
	}
	configure := func(dependencies *hostedTextRequestDependencies) {
		dependencies.catalogRevision = catalog.Revision
		dependencies.authorize = settings.authorizeCompletion
		dependencies.now = func() time.Time { return time.Unix(0, fixture.now.Load()) }
	}
	// Start while the old claim is live, then expire it at the request boundary.
	// Startup otherwise closes an undispatched request before it can be resumed.
	expired := fixture.now.Load()
	fixture.now.Store(fixture.request.CreatedAt.UnixNano())
	server := newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, fixture.database), fixture.upstreamURL, fixture.responseRoot, configure)
	fixture.now.Store(expired)
	hostedIdentityHTTP(t, server, textExecutionRecoveryKey, "funded prompt", http.StatusConflict)
	hostedIdentityHTTP(t, server, textExecutionRecoveryKey, "funded prompt", http.StatusBadGateway)
	var retained managedPriceSnapshotRecord
	if err := fixture.database.database.First(&retained, "request_id = ?", fixture.request.ID).Error; err != nil {
		t.Fatal(err)
	}
	if fixture.calls.Load() != 0 || !reflect.DeepEqual(price, retained) {
		t.Fatal("catalog replacement repriced or dispatched accepted work")
	}
	server.Close()
	var settled map[string]any
	for iteration := range 2 {
		database := openJournalTransactionInstance(t, fixture.database)
		server = newHostedIdentityHTTPServer(t, database, fixture.upstreamURL, fixture.responseRoot, configure)
		hostedIdentityHTTP(t, server, "new-catalog-request", "funded prompt", http.StatusOK)
		if err := database.reconcileHostedFunds(t.Context(), time.Unix(0, fixture.now.Load())); err != nil {
			t.Fatal(err)
		}
		assertHostedFundsBalance(t, fixture.database, 5, 5)
		assertFundsCreditRemainder(t, fixture.database, "91", "25000")
		current := fixture.state(t)
		if fixture.calls.Load() != 1 || (iteration > 0 && !reflect.DeepEqual(settled, current)) {
			t.Fatal("new catalog replay repeated provider or financial effects")
		}
		settled = current
		server.Close()
	}
}
