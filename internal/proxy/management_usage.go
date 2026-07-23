package proxy

import (
	"errors"
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
	usageIntervalAll       = usageInterval("all")
	usageIntervalThirtyDay = usageInterval("30d")
	usageIntervalSevenDay  = usageInterval("7d")
	usageIntervalOneDay    = usageInterval("1d")
	usageBucketUnitDay     = usageBucketUnit("day")
	usageBucketUnitHour    = usageBucketUnit("hour")
)

var errManagedUsageIntervalInvalid = errors.New("managed_usage_interval_invalid")

type usageInterval string

type usageBucketUnit string

type managedUsageEvent struct {
	endpoint            string
	providerIdentifier  string
	modelIdentifier     string
	statusCode          int
	latencyMilliseconds int64
	usage               *tokenUsage
}

type managedUsageSummary struct {
	interval    usageInterval
	bucketUnit  usageBucketUnit
	totals      managedUsageAggregate
	buckets     []managedUsageBucket
	providers   []managedUsageProviderBucket
	models      []managedUsageModelBucket
	statusCodes []managedUsageStatusBucket
}

type managedAdminUsageSummary struct {
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

type managedUsageBucket struct {
	start     time.Time
	aggregate managedUsageAggregate
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

type managedUsageAccumulator struct {
	totals    managedUsageAggregate
	providers map[string]managedUsageAggregate
	models    map[string]managedUsageModelBucket
	statuses  map[int]int
}

func newUsageInterval(value string) (usageInterval, error) {
	interval := usageInterval(value)
	switch interval {
	case usageIntervalAll, usageIntervalThirtyDay, usageIntervalSevenDay, usageIntervalOneDay:
		return interval, nil
	default:
		return "", fmt.Errorf("%w: interval=%q", errManagedUsageIntervalInvalid, value)
	}
}

func (interval usageInterval) finiteWindow() (time.Duration, int, usageBucketUnit, bool) {
	switch interval {
	case usageIntervalThirtyDay:
		return managedUsageSummaryDays * 24 * time.Hour, managedUsageSummaryDays, usageBucketUnitDay, true
	case usageIntervalSevenDay:
		return 7 * 24 * time.Hour, 7, usageBucketUnitDay, true
	case usageIntervalOneDay:
		return 24 * time.Hour, 24, usageBucketUnitHour, true
	case usageIntervalAll:
		return 0, 0, usageBucketUnitDay, false
	default:
		panic(errManagedUsageIntervalInvalid)
	}
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

func (store *managedTenantStore) usageSummary(principal managementPrincipal, interval usageInterval) (managedUsageSummary, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, recordError := store.ensureRecordLocked(principal)
	if recordError != nil {
		return managedUsageSummary{}, recordError
	}
	timestamp := store.now()
	windowDuration, _, _, finite := interval.finiteWindow()
	var records []managedUsageEventRecord
	var recordsError error
	if finite {
		records, recordsError = store.database.usageEventsByUserIDBetween(record.UserID, timestamp.Add(-windowDuration), timestamp)
	} else {
		records, recordsError = store.database.usageEventsByUserIDThrough(record.UserID, timestamp)
	}
	if recordsError != nil {
		return managedUsageSummary{}, fmt.Errorf("%w: user_id=%s: %v", errManagedTenantStorePersist, record.UserID, recordsError)
	}
	return summarizeManagedUsage(records, interval, timestamp), nil
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
		adminSnapshots = append(adminSnapshots, tenantRecord.adminSnapshot(summarizeManagedAdminUsage(usageRecordsByUserID[tenantRecord.UserID], timestamp)))
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

func summarizeManagedUsage(records []managedUsageEventRecord, interval usageInterval, now time.Time) managedUsageSummary {
	windowDuration, bucketCount, bucketUnit, finite := interval.finiteWindow()
	periodEnd := now.UTC()
	periodStart := periodEnd.Add(-windowDuration)
	if !finite {
		periodStart, bucketCount = allUsagePeriod(records, periodEnd)
	}
	buckets := make([]managedUsageBucket, 0, bucketCount)
	bucketDuration := 24 * time.Hour
	if bucketUnit == usageBucketUnitHour {
		bucketDuration = time.Hour
	}
	for bucketIndex := 0; bucketIndex < bucketCount; bucketIndex++ {
		buckets = append(buckets, managedUsageBucket{start: periodStart.Add(time.Duration(bucketIndex) * bucketDuration)})
	}
	accumulator := newManagedUsageAccumulator()
	for _, record := range records {
		if record.CreatedAt.Before(periodStart) || record.CreatedAt.After(periodEnd) {
			continue
		}
		accumulator.apply(record)
		bucketPosition := int(record.CreatedAt.Sub(periodStart) / bucketDuration)
		if bucketPosition == len(buckets) {
			bucketPosition--
		}
		applyUsageRecord(&buckets[bucketPosition].aggregate, record)
	}
	for bucketIndex := range buckets {
		finalizeUsageAggregate(&buckets[bucketIndex].aggregate)
	}
	totals, providers, models, statuses := accumulator.summary()
	return managedUsageSummary{
		interval:    interval,
		bucketUnit:  bucketUnit,
		totals:      totals,
		buckets:     buckets,
		providers:   providers,
		models:      models,
		statusCodes: statuses,
	}
}

func allUsagePeriod(records []managedUsageEventRecord, periodEnd time.Time) (time.Time, int) {
	var earliest time.Time
	for _, record := range records {
		recordTime := record.CreatedAt.UTC()
		if recordTime.After(periodEnd) {
			continue
		}
		if earliest.IsZero() || recordTime.Before(earliest) {
			earliest = recordTime
		}
	}
	if earliest.IsZero() {
		return periodEnd, 0
	}
	periodStart := time.Date(earliest.Year(), earliest.Month(), earliest.Day(), 0, 0, 0, 0, time.UTC)
	currentDay := time.Date(periodEnd.Year(), periodEnd.Month(), periodEnd.Day(), 0, 0, 0, 0, time.UTC)
	bucketCount := int(currentDay.Sub(periodStart)/(24*time.Hour)) + 1
	return periodStart, bucketCount
}

func summarizeManagedAdminUsage(records []managedUsageEventRecord, now time.Time) managedAdminUsageSummary {
	periodStart := usagePeriodStart(now)
	dailyBuckets := make([]managedUsageDailyBucket, 0, managedUsageSummaryDays)
	dailyIndex := make(map[string]int, managedUsageSummaryDays)
	for dayOffset := 0; dayOffset < managedUsageSummaryDays; dayOffset++ {
		date := periodStart.AddDate(0, 0, dayOffset).Format(usageDateFormat)
		dailyIndex[date] = len(dailyBuckets)
		dailyBuckets = append(dailyBuckets, managedUsageDailyBucket{date: date})
	}
	accumulator := newManagedUsageAccumulator()
	for _, record := range records {
		if record.CreatedAt.Before(periodStart) || record.CreatedAt.After(now) {
			continue
		}
		accumulator.apply(record)
		recordDate := record.CreatedAt.UTC().Format(usageDateFormat)
		if dailyPosition, exists := dailyIndex[recordDate]; exists {
			applyUsageRecord(&dailyBuckets[dailyPosition].aggregate, record)
		}
	}
	for dailyIndex := range dailyBuckets {
		finalizeUsageAggregate(&dailyBuckets[dailyIndex].aggregate)
	}
	totals, providers, models, statuses := accumulator.summary()
	return managedAdminUsageSummary{
		periodDays:  managedUsageSummaryDays,
		totals:      totals,
		daily:       dailyBuckets,
		providers:   providers,
		models:      models,
		statusCodes: statuses,
	}
}

func usagePeriodStart(now time.Time) time.Time {
	utcNow := now.UTC()
	today := time.Date(utcNow.Year(), utcNow.Month(), utcNow.Day(), 0, 0, 0, 0, time.UTC)
	return today.AddDate(0, 0, -managedUsageSummaryDays+1)
}

func newManagedUsageAccumulator() managedUsageAccumulator {
	return managedUsageAccumulator{
		providers: map[string]managedUsageAggregate{},
		models:    map[string]managedUsageModelBucket{},
		statuses:  map[int]int{},
	}
}

func (accumulator *managedUsageAccumulator) apply(record managedUsageEventRecord) {
	applyUsageRecord(&accumulator.totals, record)
	providerAggregate := accumulator.providers[record.ProviderID]
	applyUsageRecord(&providerAggregate, record)
	accumulator.providers[record.ProviderID] = providerAggregate

	modelKey := record.ProviderID + "\x00" + record.ModelID
	modelBucket := accumulator.models[modelKey]
	modelBucket.providerIdentifier = record.ProviderID
	modelBucket.modelIdentifier = record.ModelID
	applyUsageRecord(&modelBucket.aggregate, record)
	accumulator.models[modelKey] = modelBucket
	accumulator.statuses[record.StatusCode]++
}

func (accumulator managedUsageAccumulator) summary() (managedUsageAggregate, []managedUsageProviderBucket, []managedUsageModelBucket, []managedUsageStatusBucket) {
	finalizeUsageAggregate(&accumulator.totals)
	return accumulator.totals,
		usageProviderBucketList(accumulator.providers),
		usageModelBucketList(accumulator.models),
		usageStatusBucketList(accumulator.statuses)
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
