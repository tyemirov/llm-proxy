package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHostedDictationExplicitMetaCatalogPreservesUnknownUsage(t *testing.T) {
	const model = "muse-voice-transcribe-1.0"
	schema := internalCanonicalProviderCatalog().schema
	for index := range schema.Models {
		if schema.Models[index].ID == model {
			schema.Models[index].Enabled = ModelEnabled
		}
	}
	for provider := range schema.Providers {
		if schema.Providers[provider].ID != "meta" {
			continue
		}
		for offering := range schema.Providers[provider].Offerings {
			if schema.Providers[provider].Offerings[offering].Model == model {
				schema.Providers[provider].Offerings[offering].DefaultOperations = []string{ModelOperationDictation}
			}
		}
	}
	catalog, err := NewProviderCatalog(schema)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := newHostedRuntimeSettings(&HostedConfiguration{Offerings: []HostedOfferingConfiguration{{Provider: "meta", Model: model, Operation: ModelOperationDictation, MaximumAttempts: 1}}}, catalog.ModelCatalog())
	if err != nil {
		t.Fatal(err)
	}
	for _, funded := range []bool{false, true} {
		t.Run(fmt.Sprintf("funded=%t", funded), func(t *testing.T) {
			database, _, management, _ := newHostedRatingFixture(t)
			seedHostedFunds(t, database, 500)
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer sk-platform-dictation" {
					t.Error("Meta dictation lost its accepted credential")
				}
				writer.Header().Set("Content-Type", "application/json")
				fmt.Fprint(writer, `{"transcript":"qualified transcript"}`)
			}))
			defer upstream.Close()
			configure := func(dependencies *hostedTextRequestDependencies) {
				if funded {
					dependencies.authorize = settings.authorizeCompletion
				}
			}
			server := newHostedDictationProviderServerWithRegistry(t, database, upstream.URL, t.TempDir(), "meta", model, newProviderRegistry(Configuration{ProviderCatalog: catalog}), configure)
			fixture := fundsAdmissionFixture{fundsStartupFixture: fundsStartupFixture{database: database, management: management, calls: &calls}}
			before := fixture.state(t)
			want := http.StatusOK
			if funded {
				want = http.StatusServiceUnavailable
			}
			const audio = "RIFF\x26\x00\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00\x80\x3e\x00\x00\x00\x7d\x00\x00\x02\x00\x10\x00data\x02\x00\x00\x00\x00\x00"
			for _, path := range []string{dictatePath, transcriptionsPath} {
				body := hostedDictationProviderHTTP(t, server, path, "meta-qualification", audio, "meta", model, want)
				if !funded && !strings.Contains(body, "qualified transcript") {
					t.Fatalf("Meta dictation lost its transcript: %s", body)
				}
			}
			if funded {
				fixture.assertRolledBack(t, before)
				if calls.Load() != 0 {
					t.Fatal("unmetered Meta dictation spent customer funds")
				}
				return
			}
			var observations []managedJournalObservationRecord
			if err := database.database.Find(&observations).Error; err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 1 || len(observations) != 1 || observations[0].AdapterRevision != CatalogProtocolMetaTranscription+":1" || !strings.Contains(string(observations[0].Quantities), string(journalQuantityUnsupported)) {
				t.Fatalf("Meta invented native usage or repeated dispatch: calls=%d observations=%+v", calls.Load(), observations)
			}
			assertHostedFundsBalance(t, database, 500, 500)
		})
	}
}
