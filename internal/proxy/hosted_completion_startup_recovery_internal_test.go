package proxy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

func hostedRecoveryApplicationConfiguration(t *testing.T, fixture fundsAdmissionFixture) (Configuration, managedTenantStoreOpener) {
	t.Helper()
	management := newInternalManagementService(t, newFakeManagedTenantDatabase(), internalManagementProviderRegistry())
	management.store.database = fixture.database
	var source struct{ File string }
	if err := fixture.database.database.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&source).Error; err != nil {
		t.Fatal(err)
	}
	management.configuration.DatabasePath = source.File
	catalog := internalCanonicalProviderCatalog()
	catalog.modelCatalog.Revision = "journal-catalog"
	for _, schema := range []*ProviderCatalogSchema{&catalog.schema, &catalog.runtimeSchema} {
		for index := range schema.Providers {
			if schema.Providers[index].ID != "openai" {
				continue
			}
			for transport := range schema.Providers[index].Transports {
				if schema.Providers[index].Transports[transport].Endpoint.Protocol == CatalogEndpointProtocolHTTP {
					schema.Providers[index].Transports[transport].Endpoint.DefaultBaseURL = fixture.upstreamURL
				}
			}
		}
	}
	configuration := withInternalUpstreamCapacity(t, Configuration{
		Management: management.configuration, ProviderCatalog: catalog, AssetStorePath: fixture.responseRoot,
		Hosted: &HostedConfiguration{Offerings: []HostedOfferingConfiguration{{Provider: "openai", Model: "gpt-4.1", Operation: ModelOperationText, MaximumAttempts: 1}}},
	})
	return configuration, func(ManagementConfiguration, *providerRegistry) (*managedTenantStore, error) {
		return management.store, nil
	}
}

func rejectHostedRecoveryApplication(t *testing.T, configuration Configuration, openStore managedTenantStoreOpener, operation string) {
	t.Helper()
	application, err := buildProxyApplicationForTest(t, configuration, zap.NewNop().Sugar(), openStore)
	if err == nil || application != nil || !strings.Contains(err.Error(), operation) {
		t.Fatalf("startup recovery application=%v error=%v want=%s", application, err, operation)
	}
}

func replayHostedRecoveryApplication(t *testing.T, configuration Configuration, openStore managedTenantStoreOpener, key string, status int) {
	t.Helper()
	application, err := buildProxyApplicationForTest(t, configuration, zap.NewNop().Sugar(), openStore)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(application.router)
	defer server.Close()
	server.Client().Transport = hostedIdentityTransport{next: server.Client().Transport}
	hostedIdentityHTTP(t, server, key, "funded prompt", status)
	hostedIdentityStatusHTTP(t, server, key, status)
}

func TestHostedCompletionStartupDispatchWriteFailuresPreserveEvidence(t *testing.T) {
	for _, checkpoint := range []string{"dispatched", "observed"} {
		for _, scenario := range []struct{ name, statement, operation string }{
			{"response-lock", "BEFORE UPDATE OF state ON managed_journal_request_records WHEN OLD.state = 'executing' AND NEW.state = OLD.state", "lock hosted response recovery"},
			{"attempt", "BEFORE UPDATE OF state ON managed_journal_attempt_records WHEN NEW.state = 'uncertain'", "retain interrupted attempts"},
			{"request", "BEFORE UPDATE OF state ON managed_journal_request_records WHEN NEW.state = 'uncertain'", "retain interrupted request"},
			{"case", "BEFORE INSERT ON managed_journal_case_records", "create execution reconciliation"},
		} {
			if checkpoint == "observed" && scenario.name == "attempt" {
				continue
			}
			t.Run(checkpoint+"/"+scenario.name, func(t *testing.T) {
				fixture := newJournalInterruptionFixture(t, checkpoint)
				configuration, openStore := hostedRecoveryApplicationConfiguration(t, fixture.fundsAdmissionFixture)
				before := fixture.resources(t)
				if err := fixture.database.database.Exec("CREATE TRIGGER reject_startup_dispatch " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_startup_dispatch_failure'); END").Error; err != nil {
					t.Fatal(err)
				}
				rejectHostedRecoveryApplication(t, configuration, openStore, scenario.operation)
				if err := fixture.database.database.Exec("DROP TRIGGER reject_startup_dispatch").Error; err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(before, fixture.resources(t)) || fixture.calls.Load() != 1 {
					t.Fatal("failed startup recovery changed funds, journal evidence, or provider calls")
				}
				var recovered map[string]any
				for iteration := range 2 {
					replayHostedRecoveryApplication(t, configuration, openStore, textExecutionRecoveryKey, http.StatusConflict)
					assertHostedFundsBalance(t, fixture.database, 5, 2)
					current := fixture.resources(t)
					if iteration == 0 {
						recovered = current
					} else if !reflect.DeepEqual(recovered, current) {
						t.Fatal("repeated startup changed recovered financial or journal evidence")
					}
					if fixture.calls.Load() != 1 {
						t.Fatal("startup repeated provider work")
					}
				}
			})
		}
	}
}

func TestHostedCompletionStartupDispatchStorageFailuresPreserveEvidence(t *testing.T) {
	for _, operation := range []string{"lock expired journal dispatches", "read expired journal dispatches"} {
		t.Run(operation, func(t *testing.T) {
			fixture := newJournalInterruptionFixture(t, "dispatched")
			configuration, openStore := hostedRecoveryApplicationConfiguration(t, fixture.fundsAdmissionFixture)
			before := fixture.resources(t)
			var failures atomic.Int64
			inject := func(tx *gorm.DB) {
				if !tx.DryRun && tx.Statement.Table == "managed_journal_request_records" && strings.Contains(tx.Statement.SQL.String(), "execution_kind =") && strings.Contains(tx.Statement.SQL.String(), "claim_expires_at <=") {
					for _, value := range tx.Statement.Vars {
						if kind, ok := value.(journalExecutionKind); ok && kind == journalExecutionText {
							failures.Add(1)
							tx.AddError(errors.New("controlled_startup_dispatch_storage_failure"))
						}
					}
				}
			}
			var remove func() error
			if operation == "lock expired journal dispatches" {
				callback := fixture.database.database.Callback().Update()
				if err := callback.After("gorm:update").Register("test:startup_dispatch_storage", inject); err != nil {
					t.Fatal(err)
				}
				remove = func() error { return callback.Remove("test:startup_dispatch_storage") }
			} else {
				callback := fixture.database.database.Callback().Query()
				if err := callback.After("gorm:query").Register("test:startup_dispatch_storage", inject); err != nil {
					t.Fatal(err)
				}
				remove = func() error { return callback.Remove("test:startup_dispatch_storage") }
			}
			rejectHostedRecoveryApplication(t, configuration, openStore, operation)
			if err := remove(); err != nil {
				t.Fatal(err)
			}
			if failures.Load() != 1 || !reflect.DeepEqual(before, fixture.resources(t)) || fixture.calls.Load() != 1 {
				t.Fatalf("failed startup storage changed evidence: failures=%d calls=%d", failures.Load(), fixture.calls.Load())
			}
			replayHostedRecoveryApplication(t, configuration, openStore, textExecutionRecoveryKey, http.StatusConflict)
			fixture.recover(t, "dispatched")
		})
	}
}

func TestHostedCompletionStartupResultFailuresPreserveFunds(t *testing.T) {
	for _, scenario := range []string{"candidate-read", "receipt-write"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newPublicationRecoveryFixture(t, "receipt")
			configuration, openStore := hostedRecoveryApplicationConfiguration(t, fixture.fundsAdmissionFixture)
			before := fixture.state(t)
			var failures atomic.Int64
			callback := fixture.database.database.Callback().Query()
			operation := "recover result receipt"
			if scenario == "candidate-read" {
				operation = "read unpublished text results"
				if err := callback.After("gorm:query").Register("test:startup_result_read", func(tx *gorm.DB) {
					if !tx.DryRun && tx.Statement.Table == "managed_journal_request_records" && strings.Contains(tx.Statement.SQL.String(), "result_published_at IS NULL") {
						failures.Add(1)
						tx.AddError(errors.New("controlled_startup_result_read_failure"))
					}
				}); err != nil {
					t.Fatal(err)
				}
			} else if err := fixture.database.database.Exec("CREATE TRIGGER reject_startup_result BEFORE UPDATE OF result_published_at ON managed_journal_request_records BEGIN SELECT RAISE(ABORT, 'controlled_startup_receipt_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			rejectHostedRecoveryApplication(t, configuration, openStore, operation)
			if scenario == "candidate-read" {
				if err := callback.Remove("test:startup_result_read"); err != nil {
					t.Fatal(err)
				}
				if failures.Load() != 1 {
					t.Fatalf("startup result read failures=%d", failures.Load())
				}
			} else if err := fixture.database.database.Exec("DROP TRIGGER reject_startup_result").Error; err != nil {
				t.Fatal(err)
			}
			fixture.assertRetained(t, before)
			replayHostedRecoveryApplication(t, configuration, openStore, publicationRecoveryKey, http.StatusOK)
			fixture.recover(t, "receipt")
		})
	}
}

func TestHostedCompletionStartupFundsFailureRetainsRecoveredReceipt(t *testing.T) {
	fixture := newPublicationRecoveryFixture(t, "receipt")
	configuration, openStore := hostedRecoveryApplicationConfiguration(t, fixture.fundsAdmissionFixture)
	before := fixture.state(t)
	var failures atomic.Int64
	callback := fixture.database.database.Callback().Query()
	if err := callback.Before("gorm:query").Register("test:startup_funds_read", func(tx *gorm.DB) {
		if !tx.DryRun && tx.Statement.Table == "managed_journal_observation_records" {
			failures.Add(1)
			tx.AddError(errors.New("controlled_startup_funds_read_failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	rejectHostedRecoveryApplication(t, configuration, openStore, "recover hosted funds: read funded usage deliveries")
	if err := callback.Remove("test:startup_funds_read"); err != nil {
		t.Fatal(err)
	}
	if failures.Load() != 1 {
		t.Fatalf("startup funds read failures=%d", failures.Load())
	}
	fixture.assertPending(t, before)
	var request managedJournalRequestRecord
	if err := fixture.database.database.Where("id = ?", fixture.request.ID).First(&request).Error; err != nil {
		t.Fatal(err)
	}
	if request.State != journalRequestCompleted || request.ResultPublishedAt == nil || fixture.calls.Load() != 1 {
		t.Fatalf("failed funds startup lost recovered receipt: state=%s published=%v calls=%d", request.State, request.ResultPublishedAt, fixture.calls.Load())
	}
	replayHostedRecoveryApplication(t, configuration, openStore, publicationRecoveryKey, http.StatusOK)
	fixture.recover(t, "receipt")
}
