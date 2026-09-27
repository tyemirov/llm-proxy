package proxy

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// These helpers build claims and inspect pending evidence for test fixtures.
// The runtime builds execution claims and selects funded observations in recovery.
func newJournalWorkerClaim(requestID, owner string, now time.Time) (journalWorkerClaim, error) {
	if !strings.HasPrefix(requestID, journalRequestIDPrefix) || owner == "" || now.IsZero() {
		return journalWorkerClaim{}, errUsageJournalInvalid
	}
	return journalWorkerClaim{requestID: requestID, owner: owner, now: now.UTC()}, nil
}

func (database *gormManagedTenantDatabase) pendingJournalDeliveries(ctx context.Context, limit int) ([]managedJournalObservationRecord, error) {
	if limit < 1 || limit > 100 {
		return nil, errUsageJournalInvalid
	}
	observations := []managedJournalObservationRecord{}
	err := database.database.WithContext(ctx).Where("id IN (?)", database.database.Model(&managedJournalDeliveryRecord{}).Select("observation_id").Where("delivered_at IS NULL")).
		Order("created_at, id").Limit(limit).Find(&observations).Error
	if err != nil {
		return nil, fmt.Errorf("read pending usage delivery: %w", err)
	}
	return observations, nil
}
