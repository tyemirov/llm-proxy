package proxy

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
)

type hostedShutdownConnector struct {
	driver driver.Driver
	dsn    string
	armed  *atomic.Bool
	closes *atomic.Int64
	err    error
}

func (connector hostedShutdownConnector) Driver() driver.Driver { return connector.driver }

func (connector hostedShutdownConnector) Connect(context.Context) (driver.Conn, error) {
	connection, err := connector.driver.Open(connector.dsn)
	if err != nil {
		return nil, err
	}
	return &hostedShutdownConnection{Conn: connection, connector: connector}, nil
}

type hostedShutdownConnection struct {
	driver.Conn
	connector hostedShutdownConnector
}

func (connection *hostedShutdownConnection) Close() error {
	err := connection.Conn.Close()
	if connection.connector.armed.Load() {
		connection.connector.closes.Add(1)
		return errors.Join(err, connection.connector.err)
	}
	return err
}

type hostedShutdownPool struct {
	*sql.DB
	armed   *atomic.Bool
	lookups *atomic.Int64
	err     error
}

func (pool hostedShutdownPool) GetDBConn() (*sql.DB, error) {
	if pool.armed.Load() {
		pool.lookups.Add(1)
		return nil, pool.err
	}
	return pool.DB, nil
}

func TestHostedRuntimeDatabaseShutdownFailurePreservesRecords(t *testing.T) {
	for _, scenario := range []string{"connection-lookup", "driver-close"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newPaymentAuditFixture(t)
			before := paymentAdjustmentResources(t, fixture)
			admissions := retainedFundingAdmissions(t, fixture.database)
			var source struct{ File string }
			if err := fixture.database.database.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&source).Error; err != nil {
				t.Fatal(err)
			}
			configuration := withInternalUpstreamCapacity(t, Configuration{
				Management: managedRouterTestManagementConfiguration(), ProviderCatalog: internalCanonicalProviderCatalog(), AssetStorePath: filepath.Join(t.TempDir(), "assets"),
			})
			configuration.Management.DatabasePath = source.File
			dsn := source.File + managedSQLiteRuntimeQuery
			base, err := sql.Open(sqlite.DriverName, dsn)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = base.Close() })
			var armed atomic.Bool
			var failures atomic.Int64
			failure := errors.New("controlled_database_shutdown_failure")
			owned := base
			expected := "resolve managed database for shutdown"
			if scenario == "driver-close" {
				owned = sql.OpenDB(hostedShutdownConnector{base.Driver(), dsn, &armed, &failures, failure})
				t.Cleanup(func() { _ = owned.Close() })
				configuration.Management.DatabaseDialector = &sqlite.Dialector{Conn: owned}
				expected = "close managed database"
			} else {
				configuration.Management.DatabaseDialector = &sqlite.Dialector{Conn: hostedShutdownPool{owned, &armed, &failures, failure}}
			}
			owned.SetMaxOpenConns(1)
			router, err := BuildRouter(configuration, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = router.Close() })
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("startup health=%d body=%s", response.Code, response.Body.String())
			}
			armed.Store(true)
			for range 2 {
				if err := router.Close(); !errors.Is(err, failure) || !strings.Contains(err.Error(), expected) {
					t.Fatalf("shutdown lost database error: %v", err)
				}
			}
			if failures.Load() != 1 || !reflect.DeepEqual(before, paymentAdjustmentResources(t, fixture)) || !reflect.DeepEqual(admissions, retainedFundingAdmissions(t, fixture.database)) || fixture.processor.creates.Load() != 1 {
				t.Fatalf("shutdown changed financial effects or repeated closure: failures=%d", failures.Load())
			}
			armed.Store(false)
			if err := owned.Close(); err != nil && !errors.Is(err, failure) {
				t.Fatal(err)
			}
			configuration.Management.DatabaseDialector = sqlite.Open(dsn)
			restored, err := BuildRouter(configuration, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			if err := restored.Close(); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, paymentAdjustmentResources(t, fixture)) || !reflect.DeepEqual(admissions, retainedFundingAdmissions(t, fixture.database)) {
				t.Fatal("restored database lifecycle changed financial records")
			}
		})
	}
}
