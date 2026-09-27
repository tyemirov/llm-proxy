package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHostedDictationRejectsUnsupportedPriceConditionsAndRecovers(t *testing.T) {
	database, _, management, _ := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 500)
	grantHostedDictation(t, database)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"text":"funded transcript","usage":{"type":"duration","seconds":1.25}}`)
	}))
	t.Cleanup(upstream.Close)
	catalog := internalCanonicalProviderCatalog().ModelCatalog()
	settings, err := newHostedRuntimeSettings(&HostedConfiguration{Offerings: []HostedOfferingConfiguration{{Provider: "openai", Model: "gpt-transcribe", Operation: ModelOperationDictation, MaximumAttempts: 1, Conditions: CatalogPriceConditions{Duration: "output"}}}}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	start := func(database *gormManagedTenantDatabase) *httptest.Server {
		return newHostedDictationServer(t, database, upstream.URL, root, func(dependencies *hostedTextRequestDependencies) {
			dependencies.authorize = settings.authorizeCompletion
		})
	}
	server := start(database)
	fixture := fundsAdmissionFixture{fundsStartupFixture: fundsStartupFixture{database: database, management: management, calls: &calls}}
	before := fixture.state(t)
	const audio = "RIFF\x26\x00\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00\x80\x3e\x00\x00\x00\x7d\x00\x00\x02\x00\x10\x00data\x02\x00\x00\x00\x00\x00"
	for _, path := range []string{dictatePath, transcriptionsPath} {
		hostedDictationHTTP(t, server, path, "dictation-price-conditions", audio, http.StatusServiceUnavailable)
		fixture.assertRolledBack(t, before)
	}
	if calls.Load() != 0 {
		t.Fatal("unsupported dictation price permitted provider work")
	}
	server.Close()
	prices, err := NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	offering, err := prices.ResolveOffering("openai", "gpt-transcribe")
	if err != nil {
		t.Fatal(err)
	}
	settings = hostedDictationFinancialSettings(t, offering, "duration")
	var settled map[string]any
	for iteration := range 2 {
		reopened := openJournalTransactionInstance(t, database)
		server = start(reopened)
		for _, path := range []string{dictatePath, transcriptionsPath} {
			body := hostedDictationHTTP(t, server, path, "dictation-price-conditions", audio, http.StatusOK)
			if !strings.Contains(body, "funded transcript") {
				t.Fatalf("corrected dictation lost transcript: %s", body)
			}
		}
		if err := reopened.reconcileHostedFunds(t.Context(), ratingTestAcceptanceTime()); err != nil {
			t.Fatal(err)
		}
		assertHostedFundsBalance(t, database, 499, 499)
		assertFundsCreditRemainder(t, database, "1", "160")
		current := fixture.state(t)
		if calls.Load() != 1 || (iteration > 0 && !reflect.DeepEqual(settled, current)) {
			t.Fatal("corrected dictation replay repeated provider or financial effects")
		}
		settled = current
		server.Close()
	}
}
