package proxy

import (
	"net/http"
	"reflect"
	"testing"

	"gorm.io/gorm/clause"
)

func TestHostedMediaMissingAcceptedOperationPreventsDuplicateAdmission(t *testing.T) {
	fixture := newFundedMediaRecoveryFixture(t)
	before := fixture.state(t)
	var operation mediaOperationRecord
	if err := fixture.database.database.First(&operation, "operation_id = ?", fixture.operationID).Error; err != nil {
		t.Fatal(err)
	}
	var request managedJournalRequestRecord
	if err := fixture.database.database.First(&request, "execution_id = ?", fixture.operationID).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.database.database.Delete(&operation).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		hostedMediaAdmissionHTTP(t, fixture.server, mediaRecoveryKey, mediaRecoveryPrompt, http.StatusInternalServerError)
		if fixture.calls.Load() != 0 || !reflect.DeepEqual(before, fixture.state(t)) || len(fixture.worker.queue) != 1 {
			t.Fatal("missing operation replay changed admission or provider work")
		}
		var current managedJournalRequestRecord
		if err := fixture.database.database.First(&current, "id = ?", request.ID).Error; err != nil || !reflect.DeepEqual(request, current) {
			t.Fatalf("missing operation changed journal identity: request=%+v error=%v", current, err)
		}
		var count int64
		if err := fixture.database.database.Model(&mediaOperationRecord{}).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("missing operation retained replacement records: count=%d error=%v", count, err)
		}
	}
	if err := fixture.database.database.Omit(clause.Associations).Create(&operation).Error; err != nil {
		t.Fatal(err)
	}
	replayed := hostedMediaAdmissionHTTP(t, fixture.server, mediaRecoveryKey, mediaRecoveryPrompt, http.StatusOK)
	if replayed["operation_id"] != fixture.operationID || fixture.calls.Load() != 0 || !reflect.DeepEqual(before, fixture.state(t)) {
		t.Fatal("restored operation changed reservation or request identity")
	}
	fixture.recover(t, MediaOperationStateQueued)
}
