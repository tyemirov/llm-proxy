package proxy

import (
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestHostedResultReplayRejectsSavedIdentityConflicts(t *testing.T) {
	for _, checkpoint := range []string{"unpublished", "published"} {
		t.Run(checkpoint, func(t *testing.T) {
			for _, scenario := range []struct {
				name   string
				change func(*structuredRequestRecord)
			}{
				{"request", func(record *structuredRequestRecord) { record.ProxyRequestID += "-other" }},
				{"intent", func(record *structuredRequestRecord) { record.IntentSHA256 = sha256Hex("other intent") }},
				{"tenant", func(record *structuredRequestRecord) { record.TenantSHA256 = sha256Hex("other tenant") }},
				{"key", func(record *structuredRequestRecord) { record.IdempotencySHA256 = sha256Hex("other key") }},
				{"provider", func(record *structuredRequestRecord) { record.Provider = "other-provider" }},
				{"model", func(record *structuredRequestRecord) { record.Model = "other-model" }},
			} {
				t.Run(scenario.name, func(t *testing.T) {
					fixture := newPublicationRecoveryFixture(t, "receipt")
					if checkpoint == "published" {
						fixture.recover(t, "receipt")
					} else {
						fixture.now.Store(fixture.request.CreatedAt.UnixNano())
					}
					before := fixture.state(t)
					encoded, err := os.ReadFile(fixture.resultPath)
					if err != nil {
						t.Fatal(err)
					}
					var record structuredRequestRecord
					if err := json.Unmarshal(encoded, &record); err != nil {
						t.Fatal(err)
					}
					scenario.change(&record)
					data, err := json.Marshal(record)
					if err != nil {
						t.Fatal(err)
					}
					writeHostedRecoverySignal(t, fixture.resultPath, string(data))
					hostedIdentityStatusHTTP(t, fixture.generation, publicationRecoveryKey, http.StatusInternalServerError)
					hostedIdentityHTTP(t, fixture.generation, publicationRecoveryKey, "funded prompt", http.StatusConflict)
					if checkpoint == "unpublished" {
						fixture.assertRetained(t, before)
						fixture.now.Store(fixture.request.ClaimExpiresAt.Add(time.Second).UnixNano())
						hostedIdentityHTTP(t, fixture.generation, publicationRecoveryKey, "funded prompt", http.StatusConflict)
						fixture.assertRetained(t, before)
					} else if !reflect.DeepEqual(before, fixture.state(t)) || fixture.calls.Load() != 1 {
						t.Fatal("saved identity conflict changed funds or dispatched work")
					}
					writeHostedRecoverySignal(t, fixture.resultPath, string(encoded))
					fixture.recover(t, "receipt")
				})
			}
		})
	}
}

func TestHostedResultReplayWaitsForPublicationClaim(t *testing.T) {
	for _, result := range []string{"dispatched", "missing"} {
		t.Run(result, func(t *testing.T) {
			fixture := newPublicationRecoveryFixture(t, "file")
			fixture.now.Store(fixture.request.CreatedAt.UnixNano())
			if result == "missing" {
				if err := os.Remove(fixture.resultPath); err != nil {
					t.Fatal(err)
				}
			}
			before := fixture.state(t)
			for range 2 {
				hostedIdentityStatusHTTP(t, fixture.generation, publicationRecoveryKey, http.StatusAccepted)
				hostedIdentityHTTP(t, fixture.generation, publicationRecoveryKey, "funded prompt", http.StatusAccepted)
				fixture.assertRetained(t, before)
			}
			fixture.now.Store(fixture.request.ClaimExpiresAt.Add(time.Second).UnixNano())
			fixture.recover(t, "file")
		})
	}
}
