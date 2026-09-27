package proxy

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type hostedResultOverlapDialector struct {
	gorm.Dialector
	match   func(*gorm.DB) bool
	recover func() error
}

func (dialector hostedResultOverlapDialector) Initialize(database *gorm.DB) error {
	if err := dialector.Dialector.Initialize(database); err != nil {
		return err
	}
	var recovered atomic.Bool
	return database.Callback().Query().After("gorm:query").Register("test:overlapping_result_recovery", func(tx *gorm.DB) {
		if tx.DryRun || tx.Error != nil || tx.RowsAffected != 1 || !dialector.match(tx) {
			return
		}
		if recovered.CompareAndSwap(false, true) {
			if err := dialector.recover(); err != nil {
				tx.AddError(err)
			}
		}
	})
}

func TestHostedResultOverlappingStartupPreservesOneSettlement(t *testing.T) {
	fixture := newPublicationRecoveryFixture(t, "receipt")
	configuration, _ := hostedRecoveryApplicationConfiguration(t, fixture.fundsAdmissionFixture)
	secondConfiguration := configuration
	var second *Router
	var settled map[string]any
	configuration.Management.DatabaseDialector = hostedResultOverlapDialector{
		Dialector: fixture.database.database.Dialector,
		match: func(tx *gorm.DB) bool {
			return tx.Statement.Table == "managed_journal_request_records" && strings.Contains(tx.Statement.SQL.String(), "result_published_at IS NULL")
		},
		recover: func() error {
			var err error
			second, err = BuildRouter(secondConfiguration, zap.NewNop().Sugar())
			if err != nil {
				return err
			}
			settled = fixture.resultState(t)
			return nil
		},
	}
	t.Cleanup(func() {
		if second != nil {
			if err := second.Close(); err != nil {
				t.Error(err)
			}
		}
	})
	first, err := BuildRouter(configuration, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := first.Close(); err != nil {
			t.Error(err)
		}
	})
	if second == nil || settled == nil {
		t.Fatal("startup did not reach the overlapping result recovery")
	}
	for _, router := range []*Router{first, second} {
		server := httptest.NewServer(router)
		server.Client().Transport = hostedIdentityTransport{next: server.Client().Transport}
		for range 2 {
			if result := hostedIdentityHTTP(t, server, publicationRecoveryKey, "funded prompt", http.StatusOK); result != "funded result" {
				t.Fatalf("overlapping startup changed result: %q", result)
			}
			hostedIdentityStatusHTTP(t, server, publicationRecoveryKey, http.StatusOK)
			if fixture.calls.Load() != 1 || !reflect.DeepEqual(settled, fixture.resultState(t)) {
				t.Fatal("overlapping startup or replay repeated financial effects")
			}
		}
		server.Close()
	}
	assertHostedFundsBalance(t, fixture.database, 5, 5)
	assertFundsCreditRemainder(t, fixture.database, "91", "25000")
	fixture.recover(t, "receipt")
}

func TestHostedFundsOverlappingStartupReleasesOneReservation(t *testing.T) {
	fixture := newJournalInterruptionFixture(t, "accepted")
	configuration, _ := hostedRecoveryApplicationConfiguration(t, fixture.fundsAdmissionFixture)
	secondConfiguration := configuration
	var second *Router
	var released map[string]any
	configuration.Management.DatabaseDialector = hostedResultOverlapDialector{
		Dialector: fixture.database.database.Dialector,
		match: func(tx *gorm.DB) bool {
			return tx.Statement.Table == "managed_funds_reservation_records" && strings.Contains(tx.Statement.SQL.String(), "request_id >")
		},
		recover: func() error {
			var err error
			second, err = BuildRouter(secondConfiguration, zap.NewNop().Sugar())
			if err != nil {
				return err
			}
			released = fixture.resources(t)
			return nil
		},
	}
	t.Cleanup(func() {
		if second != nil {
			if err := second.Close(); err != nil {
				t.Error(err)
			}
		}
	})
	first, err := BuildRouter(configuration, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := first.Close(); err != nil {
			t.Error(err)
		}
	})
	if second == nil || released == nil {
		t.Fatal("startup did not reach overlapping reservation recovery")
	}
	for _, router := range []*Router{first, second} {
		server := httptest.NewServer(router)
		server.Client().Transport = hostedIdentityTransport{next: server.Client().Transport}
		for range 2 {
			hostedIdentityHTTP(t, server, textExecutionRecoveryKey, "funded prompt", http.StatusBadGateway)
			if fixture.calls.Load() != 0 || !reflect.DeepEqual(released, fixture.resources(t)) {
				t.Fatal("overlapping reservation recovery repeated financial effects or provider work")
			}
		}
		server.Close()
	}
	assertHostedFundsBalance(t, fixture.database, 5, 5)
	assertFundsCreditRemainder(t, fixture.database, "0", "1")
	var reservation managedFundsReservationRecord
	if err := fixture.database.database.Where("request_id = ?", fixture.request.ID).First(&reservation).Error; err != nil || reservation.State != fundsReservationReleased || reservation.Revision != 2 {
		t.Fatalf("overlapping startup did not release one reservation: reservation=%+v error=%v", reservation, err)
	}
	recoverHostedTextExecution(t, fixture.fundsAdmissionFixture, fixture.request, 0)
}
