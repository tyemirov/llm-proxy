package proxy

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type lifecycleUsageDialector struct {
	gorm.Dialector
	connection *sql.DB
	started    chan struct{}
	release    chan struct{}
	once       sync.Once
	configure  func(*gorm.DB) error
}

func TestHostedRuntimeFinancialSchemaCreationFailureIsAtomic(t *testing.T) {
	for _, scenario := range []struct{ name, statement, reason string }{
		{"hosted-account", "CREATE TABLE `managed_billing_account_records`", "operation=create_hosted_schema"},
		{"assignment-constraint", "CREATE TRIGGER guard_hosted_access_grant_insert", "create assignment constraint guard_hosted_access_grant_insert"},
		{"journal", "CREATE TABLE `managed_journal_request_records`", "operation=create_usage_journal"},
		{"rating", "CREATE TABLE `managed_price_snapshot_records`", "create hosted rating schema"},
		{"funds", "CREATE TABLE `managed_funds_account_records`", "create hosted funds schema"},
		{"funding-orders", "CREATE TABLE `managed_funding_order_records`", "create funding orders"},
		{"payments", "CREATE TABLE `managed_payment_inbox_records`", "create payment records"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			configuration := withInternalUpstreamCapacity(t, Configuration{Management: managedRouterTestManagementConfiguration(), ProviderCatalog: internalCanonicalProviderCatalog(), AssetStorePath: filepath.Join(root, "assets")})
			configuration.Management.DatabasePath = filepath.Join(root, "managed.sqlite")
			inspection, err := gorm.Open(sqlite.Open(configuration.Management.DatabasePath+managedSQLiteRuntimeQuery), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			connection, err := inspection.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := connection.Close(); err != nil {
					t.Error(err)
				}
			})
			before := financialSchemaSnapshot(t, inspection)
			failure := errors.New("controlled_financial_schema_creation_failure")
			var failures atomic.Int64
			dialector := &lifecycleUsageDialector{
				Dialector: sqlite.Open(configuration.Management.DatabasePath + managedSQLiteRuntimeQuery),
				configure: func(database *gorm.DB) error {
					return database.Callback().Raw().Before("gorm:raw").Register("test:financial_schema_creation", func(tx *gorm.DB) {
						if strings.HasPrefix(tx.Statement.SQL.String(), scenario.statement) {
							failures.Add(1)
							tx.AddError(failure)
						}
					})
				},
			}
			configuration.Management.DatabaseDialector = dialector
			for attempt := int64(1); attempt <= 2; attempt++ {
				router, err := BuildRouter(configuration, zap.NewNop().Sugar())
				if err == nil {
					router.Close()
					t.Fatal("startup accepted a failed financial schema write")
				}
				if !errors.Is(err, failure) || !strings.Contains(err.Error(), scenario.reason) {
					t.Fatalf("startup lost schema failure context: %v", err)
				}
				if failures.Load() != attempt {
					t.Fatalf("schema write failures=%d want=%d", failures.Load(), attempt)
				}
				if err := dialector.connection.PingContext(t.Context()); err == nil {
					t.Fatal("failed financial startup retained its database")
				}
				if !reflect.DeepEqual(before, financialSchemaSnapshot(t, inspection)) {
					t.Fatal("failed financial startup retained a partial schema")
				}
			}
			configuration.Management.DatabaseDialector = sqlite.Open(configuration.Management.DatabasePath + managedSQLiteRuntimeQuery)
			for range 2 {
				router, err := BuildRouter(configuration, zap.NewNop().Sugar())
				if err != nil {
					t.Fatal(err)
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
				if err := router.Close(); err != nil {
					t.Fatal(err)
				}
				if response.Code != http.StatusOK || response.Body.String() != `{"status":"ok"}` {
					t.Fatalf("recovered health status=%d body=%s", response.Code, response.Body.String())
				}
			}
		})
	}
}

func (dialector *lifecycleUsageDialector) Initialize(database *gorm.DB) error {
	if err := dialector.Dialector.Initialize(database); err != nil {
		return err
	}
	connection, err := database.DB()
	if err != nil {
		return err
	}
	dialector.connection = connection
	if dialector.configure != nil {
		if err := dialector.configure(database); err != nil {
			return err
		}
	}
	return database.Callback().Create().Before("gorm:create").Register("test:accepted_usage_shutdown", func(tx *gorm.DB) {
		if tx.Statement.Table == managedUsageEventTable {
			dialector.once.Do(func() {
				close(dialector.started)
				<-dialector.release
			})
		}
	})
}

func TestHostedRuntimeShutdownDrainsUsageAndClosesDatabase(t *testing.T) {
	database := newCanonicalGORMFixture(t, time.Now())
	const secret = "shutdown-usage-key"
	if err := database.database.Model(&managedTenantRecord{}).Where("tenant_id = ?", "managed-first").Update("secret_digest", sha256Hex(secret)).Error; err != nil {
		t.Fatal(err)
	}
	var source struct{ File string }
	if err := database.database.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&source).Error; err != nil {
		t.Fatal(err)
	}
	configuration := withInternalUpstreamCapacity(t, Configuration{Management: managedRouterTestManagementConfiguration(), ProviderCatalog: internalCanonicalProviderCatalog(), AssetStorePath: t.TempDir()})
	configuration.Management.DatabasePath = source.File
	dialector := &lifecycleUsageDialector{Dialector: sqlite.Open(source.File + managedSQLiteRuntimeQuery), started: make(chan struct{}), release: make(chan struct{})}
	configuration.Management.DatabaseDialector = dialector
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(dialector.release) }) }
	t.Cleanup(release)
	router, err := BuildRouter(configuration, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		release()
		if err := router.Close(); err != nil {
			t.Error(err)
		}
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	response, err := server.Client().Post(server.URL+"/?key="+secret, "application/json", strings.NewReader("{"))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("request status=%d", response.StatusCode)
	}
	select {
	case <-dialector.started:
	case <-time.After(5 * time.Second):
		t.Fatal("accepted usage did not reach storage")
	}
	server.Close()
	closed := make(chan error, 3)
	for range 3 {
		go func() { closed <- router.Close() }()
	}
	select {
	case <-closed:
		t.Fatal("router shutdown returned before accepted usage completed")
	case <-time.After(50 * time.Millisecond):
	}
	release()
	for range 3 {
		select {
		case err := <-closed:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("router shutdown did not complete")
		}
	}
	if err := dialector.connection.PingContext(context.Background()); err == nil {
		t.Fatal("router retained its owned SQL connection after shutdown")
	}
	if err := router.Close(); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := database.database.Model(&managedUsageEventRecord{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("accepted usage after shutdown: count=%d error=%v", count, err)
	}
}

func TestHostedRuntimeStartupFailureClosesOwnedDatabase(t *testing.T) {
	for _, scenario := range []string{"schema", "media-store"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			configuration := withInternalUpstreamCapacity(t, Configuration{Management: managedRouterTestManagementConfiguration(), ProviderCatalog: internalCanonicalProviderCatalog(), AssetStorePath: filepath.Join(root, "assets")})
			configuration.Management.DatabasePath = filepath.Join(root, "managed.sqlite")
			dialector := &lifecycleUsageDialector{Dialector: sqlite.Open(configuration.Management.DatabasePath + managedSQLiteRuntimeQuery)}
			configuration.Management.DatabaseDialector = dialector
			if scenario == "schema" {
				dialector.configure = func(database *gorm.DB) error {
					return database.Callback().Row().Before("gorm:row").Register("test:reject_schema", func(tx *gorm.DB) { tx.AddError(errors.New("controlled_schema_read_failure")) })
				}
			} else {
				file, err := os.Create(configuration.AssetStorePath)
				if err != nil {
					t.Fatal(err)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
			}
			router, err := BuildRouter(configuration, zap.NewNop().Sugar())
			if err == nil {
				router.Close()
				t.Fatal("invalid startup succeeded")
			}
			if dialector.connection == nil {
				t.Fatalf("startup did not reach database: %v", err)
			}
			if err := dialector.connection.PingContext(t.Context()); err == nil {
				t.Fatal("failed startup retained its database")
			}
		})
	}
}

func TestHostedRuntimeIdleRestartClosesEachOwnedDatabase(t *testing.T) {
	root := t.TempDir()
	configuration := withInternalUpstreamCapacity(t, Configuration{Management: managedRouterTestManagementConfiguration(), ProviderCatalog: internalCanonicalProviderCatalog(), AssetStorePath: filepath.Join(root, "assets")})
	configuration.Management.DatabasePath = filepath.Join(root, "managed.sqlite")
	for range 3 {
		dialector := &lifecycleUsageDialector{Dialector: sqlite.Open(configuration.Management.DatabasePath + managedSQLiteRuntimeQuery)}
		configuration.Management.DatabaseDialector = dialector
		router, err := BuildRouter(configuration, zap.NewNop().Sugar())
		if err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := router.Close(); err != nil {
				t.Fatal(err)
			}
		}
		if err := dialector.connection.PingContext(t.Context()); err == nil {
			t.Fatal("idle router retained its database")
		}
	}
}

func TestHostedRuntimeStorageCloseFailureReachesCaller(t *testing.T) {
	for _, scenario := range []string{"shutdown", "failed-construction"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			configuration := withInternalUpstreamCapacity(t, Configuration{Management: managedRouterTestManagementConfiguration(), ProviderCatalog: internalCanonicalProviderCatalog(), AssetStorePath: filepath.Join(root, "assets")})
			configuration.Management.DatabasePath = filepath.Join(root, "managed.sqlite")
			if scenario == "failed-construction" {
				file, err := os.Create(configuration.AssetStorePath)
				if err != nil {
					t.Fatal(err)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
			}
			closeFailure := errors.New("controlled_storage_close_failure")
			closeCalls := 0
			var connection *sql.DB
			router, buildError := buildRouter(configuration, zap.NewNop().Sugar(), func(settings ManagementConfiguration, providers *providerRegistry) (*managedTenantStore, error) {
				store, err := newManagedTenantStore(settings, providers)
				if err != nil {
					return nil, err
				}
				connection, err = store.database.(*gormManagedTenantDatabase).database.DB()
				if err != nil {
					return nil, errors.Join(err, store.close())
				}
				release := store.close
				store.close = func() error { closeCalls++; return errors.Join(release(), closeFailure) }
				return store, nil
			})
			if scenario == "shutdown" {
				if buildError != nil {
					t.Fatal(buildError)
				}
				for range 2 {
					if err := router.Close(); !errors.Is(err, closeFailure) {
						t.Fatalf("close error=%v", err)
					}
				}
			} else if !errors.Is(buildError, closeFailure) || !strings.Contains(buildError.Error(), "not a directory") {
				t.Fatalf("construction lost its original or cleanup error: %v", buildError)
			}
			if closeCalls != 1 {
				t.Fatalf("storage close calls=%d", closeCalls)
			}
			if err := connection.PingContext(t.Context()); err == nil {
				t.Fatal("close failure prevented database release")
			}
		})
	}
}
