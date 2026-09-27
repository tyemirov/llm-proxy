package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestHostedRuntimeStartupFailureDoesNotStartMedia(t *testing.T) {
	fixture := newFundedMediaRecoveryFixture(t)
	before := fixture.state(t)
	configuration, management := fixture.restartConfiguration(t, &HostedConfiguration{Offerings: []HostedOfferingConfiguration{{
		Provider: "openai", Model: "gpt-image-2", Operation: ModelOperationImageGeneration, MaximumAttempts: 1,
		Conditions: CatalogPriceConditions{Quality: "low", Resolution: "1024x1024"},
	}}})
	configuration.MediaOperationClaimRenewalSeconds = 1
	application, err := buildProxyApplicationForTest(t, configuration, zap.NewNop().Sugar(), func(ManagementConfiguration, *providerRegistry) (*managedTenantStore, error) {
		return management, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	callback := management.database.(*gormManagedTenantDatabase).database.Callback().Query()
	if err := callback.Before("gorm:query").Register("test:startup_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "managed_journal_observation_records" {
			tx.AddError(errors.New("controlled_startup_reconciliation_failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = callback.Remove("test:startup_failure") })
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := application.serve(t.Context(), listener); err == nil || !strings.Contains(err.Error(), "controlled_startup_reconciliation_failure") {
		t.Fatalf("startup error=%v", err)
	}
	current := hostedMediaWorkerStatus(t, fixture.server, fixture.operationID)
	if current["state"] != MediaOperationStateQueued || fixture.calls.Load() != 0 || !reflect.DeepEqual(before, fixture.state(t)) {
		t.Fatalf("failed startup changed accepted media: state=%v calls=%d", current, fixture.calls.Load())
	}
	if err := callback.Remove("test:startup_failure"); err != nil {
		t.Fatal(err)
	}
	fixture.recover(t, MediaOperationStateQueued)
}

type mediaShutdownHTTPDoer struct {
	next    HTTPDoer
	entered chan struct{}
	release chan struct{}
}

func (client mediaShutdownHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	response, err := client.next.Do(request)
	if request.Context().Err() != nil {
		close(client.entered)
		<-client.release
	}
	return response, err
}

func TestHostedRuntimeShutdownStopsMediaWorkers(t *testing.T) {
	for _, scenario := range []struct {
		name            string
		active, cleanup bool
	}{{name: "idle"}, {name: "provider-request", active: true}, {name: "provider-cleanup", active: true, cleanup: true}} {
		t.Run(scenario.name, func(t *testing.T) {
			started, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			releaseProvider := func() { releaseOnce.Do(func() { close(release) }) }
			fixture := newFundedMediaRecoveryFixtureWithResponse(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if _, err := io.Copy(io.Discard, request.Body); err != nil {
					t.Error(err)
					return
				}
				close(started)
				select {
				case <-request.Context().Done():
					close(cancelled)
				case <-release:
				}
			}))
			t.Cleanup(releaseProvider)
			cleanupEntered, cleanupRelease := make(chan struct{}), make(chan struct{})
			var cleanupOnce sync.Once
			releaseCleanup := func() { cleanupOnce.Do(func() { close(cleanupRelease) }) }
			defer releaseCleanup()
			if scenario.cleanup {
				originalClient := HTTPClient
				HTTPClient = mediaShutdownHTTPDoer{next: originalClient, entered: cleanupEntered, release: cleanupRelease}
				defer func() { HTTPClient = originalClient }()
			}
			var hosted *HostedConfiguration
			if scenario.active {
				hosted = &HostedConfiguration{Offerings: []HostedOfferingConfiguration{{
					Provider: "openai", Model: "gpt-image-2", Operation: ModelOperationImageGeneration, MaximumAttempts: 1,
					Conditions: CatalogPriceConditions{Quality: "low", Resolution: "1024x1024"},
				}}}
			}
			configuration, management := fixture.restartConfiguration(t, hosted)
			configuration.MediaOperationClaimRenewalSeconds = 1
			application, err := buildProxyApplicationForTest(t, configuration, zap.NewNop().Sugar(), func(ManagementConfiguration, *providerRegistry) (*managedTenantStore, error) {
				return management, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			stopped := make(chan error, 1)
			go func() { stopped <- application.serve(ctx, listener) }()
			response, err := http.Get("http://" + listener.Addr().String() + healthPath)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("runtime health status=%d", response.StatusCode)
			}
			if scenario.active {
				select {
				case <-started:
				case <-time.After(5 * time.Second):
					t.Fatal("media provider request did not start")
				}
			}
			cancel()
			if scenario.cleanup {
				select {
				case <-cleanupEntered:
				case <-time.After(5 * time.Second):
					t.Fatal("cancelled transport did not enter cleanup")
				}
				select {
				case err := <-stopped:
					t.Fatalf("service returned before adapter cleanup: %v", err)
				case <-time.After(100 * time.Millisecond):
				}
				releaseCleanup()
			}
			select {
			case err := <-stopped:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("normal service did not stop")
			}
			if scenario.active {
				select {
				case <-cancelled:
				case <-time.After(time.Second):
					t.Fatal("service shutdown left the media provider request active")
				}
				fixture.assertFailure(t, MediaOperationStateUncertain, 1)
				current := hostedMediaWorkerStatus(t, fixture.server, fixture.operationID)
				if current["error"].(map[string]any)["code"] != "worker_shutdown" {
					t.Fatalf("shutdown reported an incorrect interruption: %v", current)
				}
				fixture.recover(t, MediaOperationStateUncertain)
			}
			// Observe two maintenance intervals after the public service has stopped.
			var reads atomic.Int64
			database := management.database.(*gormManagedTenantDatabase).database
			callback := database.Callback().Query()
			if err := callback.Before("gorm:query").Register("test:stopped_media", func(tx *gorm.DB) {
				if tx.Statement.Table == "media_operation_records" || tx.Statement.Table == "media_operation_usage_delivery_records" {
					reads.Add(1)
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = callback.Remove("test:stopped_media") })
			timer := time.NewTimer(2100 * time.Millisecond)
			defer timer.Stop()
			<-timer.C
			if count := reads.Load(); count != 0 {
				t.Fatalf("stopped media workers still read their database: reads=%d", count)
			}
		})
	}
}
