package proxy

import (
	"crypto/rand"
	"errors"
	"io"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
)

type mediaAdmissionEntropyFailure struct {
	next     io.Reader
	armed    atomic.Bool
	failures atomic.Int64
}

func (source *mediaAdmissionEntropyFailure) Read(buffer []byte) (int, error) {
	if source.armed.CompareAndSwap(true, false) {
		source.failures.Add(1)
		return 0, errors.New("controlled_media_journal_entropy_failure")
	}
	return source.next.Read(buffer)
}

func TestHostedMediaAdmissionJournalEntropyFailurePreservesFunds(t *testing.T) {
	database, _, management, _ := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 500)
	server, worker := newHostedMediaAdmissionHTTPServer(t, database)
	worker.hostedAdmission = hostedImageFinancialSettings(t, ModelOperationImageGeneration).mediaAdmission(worker.providers)
	financial := fundsStartupFixture{database: database, management: management}
	before := financial.state(t)
	source := &mediaAdmissionEntropyFailure{next: rand.Reader}
	callback := database.database.Callback().Query()
	const callbackName = "test:media_admission_entropy"
	if err := callback.After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if _, count := tx.Statement.Dest.(*int64); count && tx.Statement.Table == "media_operation_records" && tx.Error == nil {
			source.armed.Store(true)
		}
	}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		func() {
			rand.Reader = source
			defer func() { rand.Reader = source.next }()
			hostedMediaAdmissionHTTP(t, server, "media-journal-identity", "funded image", http.StatusInternalServerError)
		}()
		if !reflect.DeepEqual(before, financial.state(t)) || len(worker.queue) != 0 {
			t.Fatal("failed journal identifier changed funds or queued work")
		}
	}
	if err := callback.Remove(callbackName); err != nil {
		t.Fatal(err)
	}
	if source.failures.Load() != 2 {
		t.Fatalf("journal identifier failures=%d want=2", source.failures.Load())
	}
	for _, model := range []any{&managedJournalRequestRecord{}, &mediaOperationRecord{}, &managedFundsReservationRecord{}} {
		var count int64
		if err := database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("failed journal identifier retained %T: count=%d error=%v", model, count, err)
		}
	}
	accepted := hostedMediaAdmissionHTTP(t, server, "media-journal-identity", "funded image", http.StatusAccepted)
	replayed := hostedMediaAdmissionHTTP(t, server, "media-journal-identity", "funded image", http.StatusOK)
	if accepted["operation_id"] != replayed["operation_id"] || len(worker.queue) != 1 {
		t.Fatal("journal identifier recovery duplicated accepted work")
	}
	assertHostedFundsBalance(t, database, 500, 461)
	assertFundsCreditRemainder(t, database, "0", "1")
}
