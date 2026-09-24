package proxy

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

func rejectFundedJournalAdmission(t *testing.T, fixture fundsAdmissionFixture, status int) {
	t.Helper()
	for range 2 {
		body := hostedIdentityHTTP(t, fixture.generation, "recover-admission", "funded prompt", status)
		if strings.Contains(body, "controlled_") || strings.Contains(body, "platform-journal") || strings.Contains(body, "grant-journal") {
			t.Fatalf("admission exposed storage or authority details: %s", body)
		}
	}
	if fixture.calls.Load() != 0 {
		t.Fatalf("rejected journal admission dispatched provider work: calls=%d", fixture.calls.Load())
	}
}

func TestHostedJournalAdmissionWriteFailuresPreserveUnreservedFunds(t *testing.T) {
	for _, scenario := range []struct{ name, statement string }{
		{"tenant-lock", "BEFORE UPDATE OF tenant_id ON managed_tenant_records"},
		{"request-create", "BEFORE INSERT ON managed_journal_request_records"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			before := fixture.state(t)
			if err := fixture.database.database.Exec("CREATE TRIGGER reject_journal_admission " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_journal_admission_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			rejectFundedJournalAdmission(t, fixture, http.StatusBadGateway)
			fixture.assertRolledBack(t, before)
			if err := fixture.database.database.Exec("DROP TRIGGER reject_journal_admission").Error; err != nil {
				t.Fatal(err)
			}
			fixture.recoverAdmission(t)
		})
	}
}

func TestHostedJournalAdmissionReadFailuresPreserveUnreservedFunds(t *testing.T) {
	for _, table := range []string{"managed_tenant_records", "managed_journal_request_records", "managed_hosted_grant_records", "managed_platform_connection_records", "managed_platform_credential_records"} {
		t.Run(table, func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			before := fixture.state(t)
			var failures atomic.Int64
			callback := fixture.database.database.Callback().Query()
			const name = "test:journal_admission_read"
			if err := callback.Before("gorm:query").Register(name, func(tx *gorm.DB) {
				_, inTransaction := tx.Statement.ConnPool.(gorm.TxCommitter)
				_, hosted := tx.Statement.Context.Value(hostedTextIdentityContextKey{}).(hostedTextIdentity)
				if hosted && inTransaction && !tx.DryRun && tx.Statement.Table == table {
					failures.Add(1)
					tx.AddError(errors.New("controlled_journal_admission_read_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = callback.Remove(name) })
			rejectFundedJournalAdmission(t, fixture, http.StatusBadGateway)
			if err := callback.Remove(name); err != nil {
				t.Fatal(err)
			}
			if failures.Load() != 2 {
				t.Fatalf("admission read failures=%d want=2", failures.Load())
			}
			fixture.assertRolledBack(t, before)
			fixture.recoverAdmission(t)
		})
	}
}

func TestHostedJournalAdmissionRejectsInconsistentAuthorityReads(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		status int
	}{
		{"malformed-offerings", http.StatusBadGateway},
		{"unqualified-credential", http.StatusForbidden},
		{"future-qualification", http.StatusForbidden},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			before := fixture.state(t)
			callback := fixture.database.database.Callback().Query()
			var changed atomic.Int64
			const name = "test:journal_authority_result"
			if err := callback.After("gorm:query").Register(name, func(tx *gorm.DB) {
				_, inTransaction := tx.Statement.ConnPool.(gorm.TxCommitter)
				_, hosted := tx.Statement.Context.Value(hostedTextIdentityContextKey{}).(hostedTextIdentity)
				if !hosted || !inTransaction || tx.DryRun || tx.Error != nil {
					return
				}
				if scenario.name == "malformed-offerings" {
					if grant, ok := tx.Statement.Dest.(*managedHostedGrantRecord); ok {
						grant.Offerings = []byte("controlled_invalid_json")
						changed.Add(1)
					}
				} else if credential, ok := tx.Statement.Dest.(*managedPlatformCredentialRecord); ok {
					credential.QualifiedAt = time.Time{}
					if scenario.name == "future-qualification" {
						credential.QualifiedAt = time.Now().Add(time.Hour)
					}
					changed.Add(1)
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = callback.Remove(name) })
			rejectFundedJournalAdmission(t, fixture, scenario.status)
			if err := callback.Remove(name); err != nil {
				t.Fatal(err)
			}
			if changed.Load() != 2 {
				t.Fatalf("inconsistent admission reads=%d want=2", changed.Load())
			}
			fixture.assertRolledBack(t, before)
			fixture.recoverAdmission(t)
		})
	}
}

func TestHostedJournalAdmissionEntropyFailuresPreserveUnreservedFunds(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		available int
	}{{"worker-identity", 0}, {"request-identity", hostedResourceIDBytes}} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			handler, _, router := newHostedIdentityHTTPHandler(t, fixture.database, fixture.upstreamURL, fixture.responseRoot, fundsDependencies(fixture.prices))
			server := httptest.NewServer(handler)
			t.Cleanup(server.Close)
			server.Client().Transport = hostedIdentityTransport{next: server.Client().Transport}
			fixture.generation = server
			before := fixture.state(t)
			original := router.hostedText.entropy
			for range 2 {
				router.hostedText.entropy = bytes.NewReader(bytes.Repeat([]byte{7}, scenario.available))
				body := hostedIdentityHTTP(t, server, "recover-admission", "funded prompt", http.StatusBadGateway)
				if strings.Contains(body, "EOF") || fixture.calls.Load() != 0 {
					t.Fatalf("entropy failure exposed details or dispatched work: body=%s calls=%d", body, fixture.calls.Load())
				}
			}
			router.hostedText.entropy = original
			fixture.assertRolledBack(t, before)
			fixture.recoverAdmission(t)
		})
	}
}
