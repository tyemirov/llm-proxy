package proxy

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

type fakeManagedTenantDatabase struct {
	usersByID                   map[string]managedUserRecord
	tenantsByID                 map[string]managedTenantRecord
	usageEvents                 []managedUsageEventRecord
	userByIDErrors              []error
	usersError                  error
	saveUserError               error
	createUserAndTenantErrors   []error
	tenantByOwnerAndIDErrors    []error
	tenantByTenantIDError       error
	tenantBySecretDigestErrors  []error
	tenantBySecretDigestRecord  *managedTenantRecord
	tenantIDExistsErrors        []error
	tenantIDExistsResults       []bool
	tenantNameExistsErrors      []error
	tenantNameExistsResults     []bool
	createTenantErrors          []error
	saveTenantErrors            []error
	deleteTenantErrors          []error
	providerKeysError           error
	saveProviderKeyErrors       []error
	deleteProviderKeyErrors     []error
	createUsageEventError       error
	earliestUsageEventError     error
	streamUsageEventsError      error
	usageEventsSinceError       error
	usageFailuresError          error
	usageEventsQueryPeriodStart time.Time
	usageEventsQueryPeriodEnd   time.Time
	usageEventsQueryMode        string
}

func newFakeManagedTenantDatabase() *fakeManagedTenantDatabase {
	return &fakeManagedTenantDatabase{
		usersByID:   map[string]managedUserRecord{},
		tenantsByID: map[string]managedTenantRecord{},
	}
}

func (database *fakeManagedTenantDatabase) userByID(userID string) (managedUserRecord, error) {
	if queryError, configured := popFakeError(&database.userByIDErrors); configured && queryError != nil {
		return managedUserRecord{}, queryError
	}
	record, found := database.usersByID[userID]
	if !found {
		return managedUserRecord{}, gorm.ErrRecordNotFound
	}
	record.Tenants = database.tenantsForOwner(userID)
	return cloneManagedUserRecord(record), nil
}

func (database *fakeManagedTenantDatabase) users() ([]managedUserRecord, error) {
	if database.usersError != nil {
		return nil, database.usersError
	}
	records := make([]managedUserRecord, 0, len(database.usersByID))
	for userID, record := range database.usersByID {
		record.Tenants = database.tenantsForOwner(userID)
		records = append(records, cloneManagedUserRecord(record))
	}
	sort.Slice(records, func(first int, second int) bool {
		firstEmail := strings.ToLower(records[first].UserEmail)
		secondEmail := strings.ToLower(records[second].UserEmail)
		if firstEmail == secondEmail {
			return records[first].UserID < records[second].UserID
		}
		return firstEmail < secondEmail
	})
	return records, nil
}

func (database *fakeManagedTenantDatabase) saveUser(record managedUserRecord) error {
	if database.saveUserError != nil {
		return database.saveUserError
	}
	if _, found := database.usersByID[record.UserID]; !found {
		return gorm.ErrRecordNotFound
	}
	record.Tenants = nil
	database.usersByID[record.UserID] = cloneManagedUserRecord(record)
	return nil
}

func (database *fakeManagedTenantDatabase) createUserAndTenant(user managedUserRecord, tenant managedTenantRecord) error {
	if createError, configured := popFakeError(&database.createUserAndTenantErrors); configured && createError != nil {
		return createError
	}
	if _, found := database.usersByID[user.UserID]; found {
		return gorm.ErrDuplicatedKey
	}
	if _, found := database.tenantsByID[tenant.TenantID]; found {
		return gorm.ErrDuplicatedKey
	}
	user.Tenants = nil
	database.usersByID[user.UserID] = cloneManagedUserRecord(user)
	database.tenantsByID[tenant.TenantID] = cloneManagedTenantRecord(tenant)
	return nil
}

func (database *fakeManagedTenantDatabase) tenantByOwnerAndID(ownerUserID string, tenantID string) (managedTenantRecord, error) {
	if queryError, configured := popFakeError(&database.tenantByOwnerAndIDErrors); configured && queryError != nil {
		return managedTenantRecord{}, queryError
	}
	record, found := database.tenantsByID[tenantID]
	if !found || record.OwnerUserID != ownerUserID {
		return managedTenantRecord{}, gorm.ErrRecordNotFound
	}
	return cloneManagedTenantRecord(record), nil
}

func (database *fakeManagedTenantDatabase) tenantByTenantID(_ context.Context, tenantID string) (managedTenantRecord, error) {
	if database.tenantByTenantIDError != nil {
		return managedTenantRecord{}, database.tenantByTenantIDError
	}
	record, found := database.tenantsByID[tenantID]
	if !found {
		return managedTenantRecord{}, gorm.ErrRecordNotFound
	}
	return cloneManagedTenantRecord(record), nil
}

func (database *fakeManagedTenantDatabase) tenantBySecretDigest(secretDigest string) (managedTenantRecord, error) {
	if queryError, configured := popFakeError(&database.tenantBySecretDigestErrors); configured && queryError != nil {
		return managedTenantRecord{}, queryError
	}
	if database.tenantBySecretDigestRecord != nil {
		return cloneManagedTenantRecord(*database.tenantBySecretDigestRecord), nil
	}
	for _, record := range database.tenantsByID {
		if record.SecretDigest != nil && *record.SecretDigest == secretDigest {
			return cloneManagedTenantRecord(record), nil
		}
	}
	return managedTenantRecord{}, gorm.ErrRecordNotFound
}

func (database *fakeManagedTenantDatabase) tenantIDExists(tenantID string) (bool, error) {
	if queryError, configured := popFakeError(&database.tenantIDExistsErrors); configured && queryError != nil {
		return false, queryError
	}
	if result, configured := popFakeBool(&database.tenantIDExistsResults); configured {
		return result, nil
	}
	_, found := database.tenantsByID[tenantID]
	return found, nil
}

func (database *fakeManagedTenantDatabase) tenantNameExists(ownerUserID string, nameKey string, excludedTenantID string) (bool, error) {
	if queryError, configured := popFakeError(&database.tenantNameExistsErrors); configured && queryError != nil {
		return false, queryError
	}
	if result, configured := popFakeBool(&database.tenantNameExistsResults); configured {
		return result, nil
	}
	for _, record := range database.tenantsByID {
		if record.OwnerUserID == ownerUserID && record.TenantID != excludedTenantID && record.NameKey == nameKey {
			return true, nil
		}
	}
	return false, nil
}

func (database *fakeManagedTenantDatabase) createTenant(record managedTenantRecord) error {
	if createError, configured := popFakeError(&database.createTenantErrors); configured && createError != nil {
		return createError
	}
	if _, found := database.tenantsByID[record.TenantID]; found {
		return gorm.ErrDuplicatedKey
	}
	database.tenantsByID[record.TenantID] = cloneManagedTenantRecord(record)
	return nil
}

func (database *fakeManagedTenantDatabase) saveTenant(record managedTenantRecord) error {
	if saveError, configured := popFakeError(&database.saveTenantErrors); configured && saveError != nil {
		return saveError
	}
	existing, found := database.tenantsByID[record.TenantID]
	if !found || existing.OwnerUserID != record.OwnerUserID {
		return gorm.ErrRecordNotFound
	}
	record.ProviderAPIKeys = append([]managedProviderAPIKeyRecord(nil), existing.ProviderAPIKeys...)
	database.tenantsByID[record.TenantID] = cloneManagedTenantRecord(record)
	return nil
}

func (database *fakeManagedTenantDatabase) deleteTenant(ownerUserID string, tenantID string) error {
	if deleteError, configured := popFakeError(&database.deleteTenantErrors); configured && deleteError != nil {
		return deleteError
	}
	record, found := database.tenantsByID[tenantID]
	if !found || record.OwnerUserID != ownerUserID {
		return gorm.ErrRecordNotFound
	}
	if len(database.tenantsForOwner(ownerUserID)) <= 1 {
		return errManagedFinalTenantDeletion
	}
	delete(database.tenantsByID, tenantID)
	filtered := database.usageEvents[:0]
	for _, usageRecord := range database.usageEvents {
		if usageRecord.TenantID != tenantID {
			filtered = append(filtered, usageRecord)
		}
	}
	database.usageEvents = filtered
	return nil
}

func (database *fakeManagedTenantDatabase) providerKeys() ([]managedProviderAPIKeyRecord, error) {
	if database.providerKeysError != nil {
		return nil, database.providerKeysError
	}
	records := []managedProviderAPIKeyRecord{}
	for _, tenantRecord := range database.tenantsByID {
		records = append(records, tenantRecord.ProviderAPIKeys...)
	}
	return records, nil
}

func (database *fakeManagedTenantDatabase) saveProviderKey(ownerUserID string, record managedProviderAPIKeyRecord, updatedAt time.Time) error {
	if saveError, configured := popFakeError(&database.saveProviderKeyErrors); configured && saveError != nil {
		return saveError
	}
	tenantRecord, found := database.tenantsByID[record.TenantID]
	if !found || tenantRecord.OwnerUserID != ownerUserID {
		return gorm.ErrRecordNotFound
	}
	providerRecords := make([]managedProviderAPIKeyRecord, 0, len(tenantRecord.ProviderAPIKeys)+1)
	replaced := false
	for _, existingRecord := range tenantRecord.ProviderAPIKeys {
		if existingRecord.ProviderID == record.ProviderID {
			providerRecords = append(providerRecords, record)
			replaced = true
		} else {
			providerRecords = append(providerRecords, existingRecord)
		}
	}
	if !replaced {
		providerRecords = append(providerRecords, record)
	}
	tenantRecord.ProviderAPIKeys = providerRecords
	tenantRecord.UpdatedAt = updatedAt
	database.tenantsByID[record.TenantID] = cloneManagedTenantRecord(tenantRecord)
	return nil
}

func (database *fakeManagedTenantDatabase) deleteProviderKey(ownerUserID string, tenantID string, providerID string, updatedAt time.Time) error {
	if deleteError, configured := popFakeError(&database.deleteProviderKeyErrors); configured && deleteError != nil {
		return deleteError
	}
	tenantRecord, found := database.tenantsByID[tenantID]
	if !found || tenantRecord.OwnerUserID != ownerUserID {
		return gorm.ErrRecordNotFound
	}
	providerRecords := make([]managedProviderAPIKeyRecord, 0, len(tenantRecord.ProviderAPIKeys))
	for _, existingRecord := range tenantRecord.ProviderAPIKeys {
		if existingRecord.ProviderID != providerID {
			providerRecords = append(providerRecords, existingRecord)
		}
	}
	tenantRecord.ProviderAPIKeys = providerRecords
	tenantRecord.UpdatedAt = updatedAt
	database.tenantsByID[tenantID] = cloneManagedTenantRecord(tenantRecord)
	return nil
}

func (database *fakeManagedTenantDatabase) createUsageEvent(_ context.Context, record managedUsageEventRecord) error {
	if database.createUsageEventError != nil {
		return database.createUsageEventError
	}
	record.ID = uint(len(database.usageEvents) + 1)
	database.usageEvents = append(database.usageEvents, record)
	return nil
}

func (database *fakeManagedTenantDatabase) earliestUsageEventByTenantIDThrough(tenantID string, periodEnd time.Time) (time.Time, error) {
	if database.earliestUsageEventError != nil {
		return time.Time{}, database.earliestUsageEventError
	}
	database.usageEventsQueryPeriodEnd = periodEnd
	database.usageEventsQueryMode = "all"
	var earliest time.Time
	for _, record := range database.usageEvents {
		if record.TenantID == tenantID && !record.CreatedAt.After(periodEnd) && (earliest.IsZero() || record.CreatedAt.Before(earliest)) {
			earliest = record.CreatedAt
		}
	}
	return earliest, nil
}

func (database *fakeManagedTenantDatabase) streamUsageEventsByTenantIDBetween(tenantID string, periodStart time.Time, periodEnd time.Time, visit managedUsageEventVisitor) error {
	if database.streamUsageEventsError != nil {
		return database.streamUsageEventsError
	}
	database.usageEventsQueryPeriodStart = periodStart
	database.usageEventsQueryPeriodEnd = periodEnd
	database.usageEventsQueryMode = "finite"
	for _, record := range database.usageEvents {
		if record.TenantID == tenantID && !record.CreatedAt.Before(periodStart) && !record.CreatedAt.After(periodEnd) {
			visit(record)
		}
	}
	return nil
}

func (database *fakeManagedTenantDatabase) streamUsageEventsByTenantIDThrough(tenantID string, periodEnd time.Time, visit managedUsageEventVisitor) error {
	if database.streamUsageEventsError != nil {
		return database.streamUsageEventsError
	}
	database.usageEventsQueryPeriodEnd = periodEnd
	database.usageEventsQueryMode = "all"
	for _, record := range database.usageEvents {
		if record.TenantID == tenantID && !record.CreatedAt.After(periodEnd) {
			visit(record)
		}
	}
	return nil
}

func (database *fakeManagedTenantDatabase) usageEventsSince(periodStart time.Time) ([]managedUsageEventRecord, error) {
	if database.usageEventsSinceError != nil {
		return nil, database.usageEventsSinceError
	}
	database.usageEventsQueryPeriodStart = periodStart
	records := []managedUsageEventRecord{}
	for _, record := range database.usageEvents {
		if !record.CreatedAt.Before(periodStart) {
			records = append(records, record)
		}
	}
	return records, nil
}

func (database *fakeManagedTenantDatabase) usageFailuresByOwnerAndTenant(ownerUserID string, tenantID string, query managedUsageFailureRecordQuery) ([]managedUsageFailureRecord, uint, error) {
	if database.usageFailuresError != nil {
		return nil, 0, database.usageFailuresError
	}
	tenantRecord, found := database.tenantsByID[tenantID]
	if !found || tenantRecord.OwnerUserID != ownerUserID {
		return nil, 0, gorm.ErrRecordNotFound
	}
	var resolvedSnapshotID uint
	if query.snapshotID != nil {
		resolvedSnapshotID = *query.snapshotID
	} else {
		for _, record := range database.usageEvents {
			if record.TenantID == tenantID && !record.CreatedAt.After(query.snapshotAt) && record.ID > resolvedSnapshotID {
				resolvedSnapshotID = record.ID
			}
		}
	}
	usageRecords := make([]managedUsageEventRecord, 0, query.limit)
	for _, record := range database.usageEvents {
		if record.TenantID != tenantID || record.Success || record.ID > resolvedSnapshotID || record.CreatedAt.After(query.snapshotAt) {
			continue
		}
		if query.periodStart != nil && record.CreatedAt.Before(*query.periodStart) {
			continue
		}
		if query.position != nil && (record.CreatedAt.After(query.position.occurredAt) ||
			(record.CreatedAt.Equal(query.position.occurredAt) && record.ID >= query.position.recordID)) {
			continue
		}
		usageRecords = append(usageRecords, record)
	}
	sort.Slice(usageRecords, func(first int, second int) bool {
		if usageRecords[first].CreatedAt.Equal(usageRecords[second].CreatedAt) {
			return usageRecords[first].ID > usageRecords[second].ID
		}
		return usageRecords[first].CreatedAt.After(usageRecords[second].CreatedAt)
	})
	if len(usageRecords) > query.limit {
		usageRecords = usageRecords[:query.limit]
	}
	records := make([]managedUsageFailureRecord, 0, len(usageRecords))
	for _, record := range usageRecords {
		outcomeCode, outcomeError := newManagedUsageOutcomeCode(string(record.OutcomeCode))
		if outcomeError != nil {
			return nil, 0, outcomeError
		}
		records = append(records, managedUsageFailureRecord{
			recordID: record.ID,
			failure: managedUsageFailure{
				occurredAt:          record.CreatedAt.UTC(),
				endpoint:            record.Endpoint,
				providerIdentifier:  record.ProviderID,
				modelIdentifier:     record.ModelID,
				statusCode:          record.StatusCode,
				outcomeCode:         outcomeCode,
				latencyMilliseconds: record.LatencyMilliseconds,
			},
		})
	}
	return records, resolvedSnapshotID, nil
}

func (database *fakeManagedTenantDatabase) tenantsForOwner(ownerUserID string) []managedTenantRecord {
	records := []managedTenantRecord{}
	for _, record := range database.tenantsByID {
		if record.OwnerUserID == ownerUserID {
			records = append(records, cloneManagedTenantRecord(record))
		}
	}
	sort.Slice(records, func(first int, second int) bool {
		if records[first].CreatedAt.Equal(records[second].CreatedAt) {
			return records[first].TenantID < records[second].TenantID
		}
		return records[first].CreatedAt.Before(records[second].CreatedAt)
	})
	return records
}

func cloneManagedUserRecord(record managedUserRecord) managedUserRecord {
	record.Tenants = append([]managedTenantRecord(nil), record.Tenants...)
	for index := range record.Tenants {
		record.Tenants[index] = cloneManagedTenantRecord(record.Tenants[index])
	}
	return record
}

func cloneManagedTenantRecord(record managedTenantRecord) managedTenantRecord {
	record.ProviderAPIKeys = append([]managedProviderAPIKeyRecord(nil), record.ProviderAPIKeys...)
	record.UsageEvents = append([]managedUsageEventRecord(nil), record.UsageEvents...)
	if record.SecretDigest != nil {
		digest := *record.SecretDigest
		record.SecretDigest = &digest
	}
	return record
}

func popFakeError(errorsQueue *[]error) (error, bool) {
	if len(*errorsQueue) == 0 {
		return nil, false
	}
	configuredError := (*errorsQueue)[0]
	*errorsQueue = (*errorsQueue)[1:]
	return configuredError, true
}

func popFakeBool(values *[]bool) (bool, bool) {
	if len(*values) == 0 {
		return false, false
	}
	value := (*values)[0]
	*values = (*values)[1:]
	return value, true
}

func fakeTenantRecord(ownerUserID string, tenantIDValue string, tenantName string, timestamp time.Time) managedTenantRecord {
	name, nameError := newManagedTenantName(tenantName)
	if nameError != nil {
		panic(nameError)
	}
	identifier, identifierError := newManagedTenantIdentifier(tenantIDValue)
	if identifierError != nil {
		panic(identifierError)
	}
	return newManagedTenantRecord(ownerUserID, identifier, name, timestamp)
}

func fakeUserWithTenant(database *fakeManagedTenantDatabase, principal managementPrincipal, tenantIDValue string, tenantName string, timestamp time.Time) managedTenantIdentifier {
	identifier, identifierError := newManagedTenantIdentifier(tenantIDValue)
	if identifierError != nil {
		panic(identifierError)
	}
	database.usersByID[principal.userID] = managedUserRecord{
		UserID:          principal.userID,
		UserEmail:       principal.userEmail,
		UserDisplayName: principal.userDisplayName,
		UserAvatarURL:   principal.userAvatarURL,
		CreatedAt:       timestamp,
		UpdatedAt:       timestamp,
	}
	database.tenantsByID[tenantIDValue] = fakeTenantRecord(principal.userID, tenantIDValue, tenantName, timestamp)
	return identifier
}

func assertManagedError(t testingT, actual error, expected error) {
	t.Helper()
	if !errors.Is(actual, expected) {
		t.Fatalf("error=%v want=%v", actual, expected)
	}
}

type testingT interface {
	Helper()
	Fatalf(string, ...interface{})
}
