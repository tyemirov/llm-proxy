package proxy

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/tyemirov/llm-proxy/internal/constants"
)

const (
	usageEndpointText      = "text"
	usageEndpointV2        = "v2"
	usageEndpointDictation = "dictation"
	usageDateFormat        = "2006-01-02"
)

type managedUsageEvent struct {
	endpoint            string
	providerIdentifier  string
	modelIdentifier     string
	statusCode          int
	latencyMilliseconds int64
	usage               *tokenUsage
}

type managedUsageSummary struct {
	periodDays  int
	totals      managedUsageAggregate
	daily       []managedUsageDailyBucket
	providers   []managedUsageProviderBucket
	models      []managedUsageModelBucket
	statusCodes []managedUsageStatusBucket
}

type managedUsageAggregate struct {
	requests             int
	successfulRequests   int
	failedRequests       int
	textRequests         int
	dictationRequests    int
	requestTokens        int
	responseTokens       int
	totalTokens          int
	latencyMilliseconds  int64
	averageLatencyMillis int64
}

type managedUsageDailyBucket struct {
	date      string
	aggregate managedUsageAggregate
}

type managedUsageProviderBucket struct {
	providerIdentifier string
	aggregate          managedUsageAggregate
}

type managedUsageModelBucket struct {
	providerIdentifier string
	modelIdentifier    string
	aggregate          managedUsageAggregate
}

type managedUsageStatusBucket struct {
	statusCode int
	requests   int
}

func (store *managedTenantStore) recordUsage(requestTenant tenant, event managedUsageEvent) error {
	if !requestTenant.managed || requestTenant.userID == constants.EmptyString {
		return nil
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	ownerRecord, ownerError := store.database.tenantByTenantID(requestTenant.identifier.string())
	if ownerError != nil {
		return fmt.Errorf("%w: tenant_id=%s: %v", errManagedTenantStorePersist, requestTenant.identifier.string(), ownerError)
	}
	if strings.HasPrefix(ownerRecord.UserID, legacyStaticTenantUserIDPrefix) {
		return fmt.Errorf("%w: tenant_id=%s", errManagedLegacyTokenUnowned, requestTenant.identifier.string())
	}
	timestamp := store.now()
	usageRecord := managedUsageEventRecord{
		UserID:              ownerRecord.UserID,
		TenantID:            requestTenant.identifier.string(),
		Endpoint:            event.endpoint,
		ProviderID:          event.providerIdentifier,
		ModelID:             event.modelIdentifier,
		StatusCode:          event.statusCode,
		Success:             event.statusCode >= http.StatusOK && event.statusCode < http.StatusBadRequest,
		LatencyMilliseconds: event.latencyMilliseconds,
		CreatedAt:           timestamp,
	}
	if event.usage != nil {
		usageRecord.RequestTokens = event.usage.RequestTokens
		usageRecord.ResponseTokens = event.usage.ResponseTokens
		usageRecord.TotalTokens = event.usage.TotalTokens
	}
	if persistError := store.database.createUsageEvent(usageRecord); persistError != nil {
		return fmt.Errorf("%w: user_id=%s: %v", errManagedTenantStorePersist, ownerRecord.UserID, persistError)
	}
	return nil
}

func (store *managedTenantStore) usageSummary(principal managementPrincipal) (managedUsageSummary, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, recordError := store.ensureRecordLocked(principal)
	if recordError != nil {
		return managedUsageSummary{}, recordError
	}
	timestamp := store.now()
	records, recordsError := store.database.usageEventsByUserIDSince(record.UserID, usagePeriodStart(timestamp))
	if recordsError != nil {
		return managedUsageSummary{}, fmt.Errorf("%w: user_id=%s: %v", errManagedTenantStorePersist, record.UserID, recordsError)
	}
	return summarizeManagedUsage(records, timestamp), nil
}

func (store *managedTenantStore) adminUsersSummary() ([]managedAdminUserSnapshot, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	timestamp := store.now()
	periodStart := usagePeriodStart(timestamp)
	tenantRecords, tenantRecordsError := store.database.tenants()
	if tenantRecordsError != nil {
		return nil, fmt.Errorf("%w: admin_users: %v", errManagedTenantStorePersist, tenantRecordsError)
	}
	usageRecords, usageRecordsError := store.database.usageEventsSince(periodStart)
	if usageRecordsError != nil {
		return nil, fmt.Errorf("%w: admin_usage: %v", errManagedTenantStorePersist, usageRecordsError)
	}
	usageRecordsByUserID := make(map[string][]managedUsageEventRecord, len(tenantRecords))
	for _, usageRecord := range usageRecords {
		usageRecordsByUserID[usageRecord.UserID] = append(usageRecordsByUserID[usageRecord.UserID], usageRecord)
	}
	adminSnapshots := make([]managedAdminUserSnapshot, 0, len(tenantRecords))
	for _, tenantRecord := range tenantRecords {
		adminSnapshots = append(adminSnapshots, tenantRecord.adminSnapshot(summarizeManagedUsage(usageRecordsByUserID[tenantRecord.UserID], timestamp)))
	}
	sort.Slice(adminSnapshots, func(firstIndex int, secondIndex int) bool {
		firstEmail := strings.ToLower(adminSnapshots[firstIndex].userEmail)
		secondEmail := strings.ToLower(adminSnapshots[secondIndex].userEmail)
		if firstEmail == secondEmail {
			return adminSnapshots[firstIndex].userID < adminSnapshots[secondIndex].userID
		}
		return firstEmail < secondEmail
	})
	return adminSnapshots, nil
}

func summarizeManagedUsage(records []managedUsageEventRecord, now time.Time) managedUsageSummary {
	periodStart := usagePeriodStart(now)
	dailyBuckets := make([]managedUsageDailyBucket, 0, managedUsageSummaryDays)
	dailyIndex := make(map[string]int, managedUsageSummaryDays)
	for dayOffset := 0; dayOffset < managedUsageSummaryDays; dayOffset++ {
		date := periodStart.AddDate(0, 0, dayOffset).Format(usageDateFormat)
		dailyIndex[date] = len(dailyBuckets)
		dailyBuckets = append(dailyBuckets, managedUsageDailyBucket{date: date})
	}
	providerBuckets := map[string]managedUsageAggregate{}
	modelBuckets := map[string]managedUsageModelBucket{}
	statusBuckets := map[int]int{}
	var totals managedUsageAggregate
	for _, record := range records {
		if record.CreatedAt.Before(periodStart) {
			continue
		}
		applyUsageRecord(&totals, record)
		recordDate := record.CreatedAt.UTC().Format(usageDateFormat)
		if dailyPosition, exists := dailyIndex[recordDate]; exists {
			applyUsageRecord(&dailyBuckets[dailyPosition].aggregate, record)
		}
		providerKey := record.ProviderID
		providerAggregate := providerBuckets[providerKey]
		applyUsageRecord(&providerAggregate, record)
		providerBuckets[providerKey] = providerAggregate

		modelKey := record.ProviderID + "\x00" + record.ModelID
		modelBucket := modelBuckets[modelKey]
		modelBucket.providerIdentifier = record.ProviderID
		modelBucket.modelIdentifier = record.ModelID
		applyUsageRecord(&modelBucket.aggregate, record)
		modelBuckets[modelKey] = modelBucket
		statusBuckets[record.StatusCode]++
	}
	finalizeUsageAggregate(&totals)
	for dailyIndex := range dailyBuckets {
		finalizeUsageAggregate(&dailyBuckets[dailyIndex].aggregate)
	}
	providerList := usageProviderBucketList(providerBuckets)
	modelList := usageModelBucketList(modelBuckets)
	statusList := usageStatusBucketList(statusBuckets)
	return managedUsageSummary{
		periodDays:  managedUsageSummaryDays,
		totals:      totals,
		daily:       dailyBuckets,
		providers:   providerList,
		models:      modelList,
		statusCodes: statusList,
	}
}

func usagePeriodStart(now time.Time) time.Time {
	utcNow := now.UTC()
	today := time.Date(utcNow.Year(), utcNow.Month(), utcNow.Day(), 0, 0, 0, 0, time.UTC)
	return today.AddDate(0, 0, -managedUsageSummaryDays+1)
}

func applyUsageRecord(aggregate *managedUsageAggregate, record managedUsageEventRecord) {
	aggregate.requests++
	if record.Success {
		aggregate.successfulRequests++
	} else {
		aggregate.failedRequests++
	}
	if record.Endpoint == usageEndpointDictation {
		aggregate.dictationRequests++
	} else {
		aggregate.textRequests++
	}
	aggregate.requestTokens += record.RequestTokens
	aggregate.responseTokens += record.ResponseTokens
	aggregate.totalTokens += record.TotalTokens
	aggregate.latencyMilliseconds += record.LatencyMilliseconds
}

func finalizeUsageAggregate(aggregate *managedUsageAggregate) {
	if aggregate.requests == 0 {
		return
	}
	aggregate.averageLatencyMillis = aggregate.latencyMilliseconds / int64(aggregate.requests)
}

func usageProviderBucketList(providerBuckets map[string]managedUsageAggregate) []managedUsageProviderBucket {
	providers := make([]managedUsageProviderBucket, 0, len(providerBuckets))
	for providerIdentifier, aggregate := range providerBuckets {
		finalizeUsageAggregate(&aggregate)
		providers = append(providers, managedUsageProviderBucket{providerIdentifier: providerIdentifier, aggregate: aggregate})
	}
	sort.Slice(providers, func(firstIndex int, secondIndex int) bool {
		if providers[firstIndex].aggregate.requests == providers[secondIndex].aggregate.requests {
			return providers[firstIndex].providerIdentifier < providers[secondIndex].providerIdentifier
		}
		return providers[firstIndex].aggregate.requests > providers[secondIndex].aggregate.requests
	})
	return providers
}

func usageModelBucketList(modelBuckets map[string]managedUsageModelBucket) []managedUsageModelBucket {
	models := make([]managedUsageModelBucket, 0, len(modelBuckets))
	for _, modelBucket := range modelBuckets {
		finalizeUsageAggregate(&modelBucket.aggregate)
		models = append(models, modelBucket)
	}
	sort.Slice(models, func(firstIndex int, secondIndex int) bool {
		if models[firstIndex].aggregate.requests == models[secondIndex].aggregate.requests {
			if models[firstIndex].providerIdentifier == models[secondIndex].providerIdentifier {
				return models[firstIndex].modelIdentifier < models[secondIndex].modelIdentifier
			}
			return models[firstIndex].providerIdentifier < models[secondIndex].providerIdentifier
		}
		return models[firstIndex].aggregate.requests > models[secondIndex].aggregate.requests
	})
	return models
}

func usageStatusBucketList(statusBuckets map[int]int) []managedUsageStatusBucket {
	statusCodes := make([]managedUsageStatusBucket, 0, len(statusBuckets))
	for statusCode, requests := range statusBuckets {
		statusCodes = append(statusCodes, managedUsageStatusBucket{statusCode: statusCode, requests: requests})
	}
	sort.Slice(statusCodes, func(firstIndex int, secondIndex int) bool {
		return statusCodes[firstIndex].statusCode < statusCodes[secondIndex].statusCode
	})
	return statusCodes
}
