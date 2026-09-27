package proxy

import (
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestHostedFundsRecoveryRejectsEmptyStoredRequestIdentity(t *testing.T) {
	fixture := newJournalInterruptionFixture(t, "accepted")
	before := fixture.resources(t)
	resultPath := filepath.Join(fixture.responseRoot, structuredRequestDirectoryName, sha256Hex(fixture.request.TenantID), fixture.request.KeyDigest+".json")
	originalResult, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var reservation managedFundsReservationRecord
	if err := fixture.database.database.First(&reservation, "request_id = ?", fixture.request.ID).Error; err != nil {
		t.Fatal(err)
	}
	replaceIdentity := func(previous, next string) {
		t.Helper()
		if err := fixture.database.database.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("PRAGMA defer_foreign_keys = ON").Error; err != nil {
				return err
			}
			tables, err := tx.Migrator().GetTables()
			if err != nil {
				return err
			}
			for _, table := range tables {
				if tx.Migrator().HasColumn(table, "request_id") {
					if err := tx.Table(table).Where("request_id = ?", previous).UpdateColumn("request_id", next).Error; err != nil {
						return err
					}
				}
			}
			return tx.Model(&managedJournalRequestRecord{}).Where("id = ?", previous).UpdateColumn("id", next).Error
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := fixture.database.database.Delete(&managedFundsReservationRecord{}, "request_id = ?", fixture.request.ID).Error; err != nil {
		t.Fatal(err)
	}
	replaceIdentity(fixture.request.ID, "")
	hostedIdentityHTTP(t, fixture.generation, textExecutionRecoveryKey, "funded prompt", http.StatusServiceUnavailable)
	if fixture.calls.Load() != 0 {
		t.Fatal("empty stored request identity dispatched provider work")
	}
	history := accountConnectionHTTPExchange(t, fixture.management, http.MethodGet, "/billing-accounts/billing-journal/ledger-entries?limit=100", "", http.StatusOK)
	if !reflect.DeepEqual(before["ledger-entries"], history) {
		t.Fatal("empty stored request identity changed the original Ledger hold")
	}
	replaceIdentity("", fixture.request.ID)
	if err := fixture.database.database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&managedJournalRequestRecord{}).Where("id = ?", fixture.request.ID).Select("*").Omit(clause.Associations).UpdateColumns(&fixture.request).Error; err != nil {
			return err
		}
		return tx.Omit(clause.Associations).Create(&reservation).Error
	}); err != nil {
		t.Fatal(err)
	}
	for resource, current := range fixture.resources(t) {
		if !reflect.DeepEqual(before[resource], current) {
			t.Fatalf("failed reservation changed restored %s: before=%v after=%v", resource, before[resource], current)
		}
	}
	writeHostedRecoverySignal(t, resultPath, string(originalResult))
	fixture.recover(t, "accepted")
}
