package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tyemirov/llm-proxy/internal/constants"
)

const (
	usageEndpointText      = "text"
	usageEndpointV2        = "v2"
	usageEndpointAnalyzer  = "analyzer"
	usageEndpointDictation = "dictation"
	usageDateFormat        = "2006-01-02"
	usageIntervalAll       = usageInterval("all")
	usageIntervalThirtyDay = usageInterval("30d")
	usageIntervalSevenDay  = usageInterval("7d")
	usageIntervalOneDay    = usageInterval("1d")
	usageBucketUnitDay     = usageBucketUnit("day")
	usageBucketUnitHour    = usageBucketUnit("hour")

	managedUsageFailureCursorVersion = 2
	managedUsageFailureDefaultLimit  = 25
	managedUsageFailureMaximumLimit  = 100
	managedUsagePersistenceTimeout   = 5 * time.Second
	usageFailureIntervalQuery        = "interval"
	usageFailureLimitQuery           = "limit"
	usageFailureCursorQuery          = "cursor"
	managedUsageAllTenantsScope      = "all-tenants"

	managedUsageOutcomeSuccess            = managedUsageOutcomeCode("success")
	managedUsageOutcomeInvalidRequest     = managedUsageOutcomeCode("invalid_request")
	managedUsageOutcomePayloadTooLarge    = managedUsageOutcomeCode("payload_too_large")
	managedUsageOutcomeRateLimited        = managedUsageOutcomeCode("rate_limited")
	managedUsageOutcomeServiceUnavailable = managedUsageOutcomeCode("service_unavailable")
	managedUsageOutcomeRequestTimeout     = managedUsageOutcomeCode("request_timeout")
	managedUsageOutcomeUpstreamError      = managedUsageOutcomeCode("upstream_error")
)

var (
	errManagedUsageIntervalInvalid = errors.New("managed_usage_interval_invalid")
	errManagedUsageFailureQuery    = errors.New("managed_usage_failure_query_invalid")
	errManagedUsageOutcomeInvalid  = errors.New("managed_usage_outcome_invalid")
)

type usageInterval string

type usageBucketUnit string

type managedUsageOutcomeCode string

type managedUsageEvent struct {
	endpoint            string
	providerIdentifier  string
	modelIdentifier     string
	statusCode          int
	outcomeCode         managedUsageOutcomeCode
	latencyMilliseconds int64
	usage               *tokenUsage
}

type managedUsageFailureQuery struct {
	interval usageInterval
	scope    string
	limit    int
	cursor   *managedUsageFailureCursor
}

type managedUsageFailureCursor struct {
	interval   usageInterval
	scope      string
	snapshotAt time.Time
	snapshotID uint
	positionAt time.Time
	positionID uint
}

type managedUsageFailureCursorPayload struct {
	Version    int    `json:"v"`
	Interval   string `json:"i"`
	Scope      string `json:"o"`
	SnapshotAt string `json:"s"`
	SnapshotID uint   `json:"x"`
	PositionAt string `json:"p"`
	PositionID uint   `json:"n"`
}

type managedUsageFailureRecordQuery struct {
	periodStart *time.Time
	snapshotAt  time.Time
	snapshotID  *uint
	position    *managedUsageFailurePosition
	limit       int
}

type managedUsageFailurePosition struct {
	occurredAt time.Time
	recordID   uint
}

type managedUsageFailurePage struct {
	interval   usageInterval
	failures   []managedUsageFailure
	nextCursor string
}

type managedUsageFailure struct {
	tenantIdentifier    string
	tenantName          string
	occurredAt          time.Time
	endpoint            string
	providerIdentifier  string
	modelIdentifier     string
	statusCode          int
	outcomeCode         managedUsageOutcomeCode
	latencyMilliseconds int64
}

type managedUsageFailureRecord struct {
	recordID uint
	failure  managedUsageFailure
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

type managedUsageSummaryAccumulator struct {
	interval       usageInterval
	bucketUnit     usageBucketUnit
	periodStart    time.Time
	periodEnd      time.Time
	bucketDuration time.Duration
	buckets        []managedUsageBucket
	usage          managedUsageAccumulator
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

func newManagedUsageOutcomeCode(value string) (managedUsageOutcomeCode, error) {
	outcomeCode := managedUsageOutcomeCode(value)
	switch outcomeCode {
	case managedUsageOutcomeSuccess,
		managedUsageOutcomeInvalidRequest,
		managedUsageOutcomePayloadTooLarge,
		managedUsageOutcomeRateLimited,
		managedUsageOutcomeServiceUnavailable,
		managedUsageOutcomeRequestTimeout,
		managedUsageOutcomeUpstreamError:
		return outcomeCode, nil
	default:
		return "", fmt.Errorf("%w: outcome_code=%q", errManagedUsageOutcomeInvalid, value)
	}
}

func newManagedUsageFailureQuery(queryValues url.Values, scope string) (managedUsageFailureQuery, error) {
	if strings.TrimSpace(scope) == constants.EmptyString {
		return managedUsageFailureQuery{}, fmt.Errorf("%w: scope", errManagedUsageFailureQuery)
	}
	for queryName := range queryValues {
		switch queryName {
		case usageFailureIntervalQuery, usageFailureLimitQuery, usageFailureCursorQuery:
		default:
			return managedUsageFailureQuery{}, fmt.Errorf("%w: unknown_query=%s", errManagedUsageFailureQuery, queryName)
		}
	}
	intervalValues := queryValues[usageFailureIntervalQuery]
	if len(intervalValues) != 1 {
		return managedUsageFailureQuery{}, fmt.Errorf("%w: interval_count=%d", errManagedUsageFailureQuery, len(intervalValues))
	}
	interval, intervalError := newUsageInterval(intervalValues[0])
	if intervalError != nil {
		return managedUsageFailureQuery{}, fmt.Errorf("%w: %v", errManagedUsageFailureQuery, intervalError)
	}
	limit := managedUsageFailureDefaultLimit
	if limitValues, supplied := queryValues[usageFailureLimitQuery]; supplied {
		if len(limitValues) != 1 || !decimalDigits(limitValues[0]) {
			return managedUsageFailureQuery{}, fmt.Errorf("%w: limit", errManagedUsageFailureQuery)
		}
		parsedLimit, parseError := strconv.Atoi(limitValues[0])
		if parseError != nil || parsedLimit < 1 || parsedLimit > managedUsageFailureMaximumLimit || strconv.Itoa(parsedLimit) != limitValues[0] {
			return managedUsageFailureQuery{}, fmt.Errorf("%w: limit=%q", errManagedUsageFailureQuery, limitValues[0])
		}
		limit = parsedLimit
	}
	var cursor *managedUsageFailureCursor
	if cursorValues, supplied := queryValues[usageFailureCursorQuery]; supplied {
		if len(cursorValues) != 1 || strings.TrimSpace(cursorValues[0]) == constants.EmptyString {
			return managedUsageFailureQuery{}, fmt.Errorf("%w: cursor", errManagedUsageFailureQuery)
		}
		parsedCursor, cursorError := newManagedUsageFailureCursor(cursorValues[0], interval, scope)
		if cursorError != nil {
			return managedUsageFailureQuery{}, cursorError
		}
		cursor = &parsedCursor
	}
	return managedUsageFailureQuery{interval: interval, scope: scope, limit: limit, cursor: cursor}, nil
}

func newManagedUsageFailureCursor(rawCursor string, expectedInterval usageInterval, expectedScope string) (managedUsageFailureCursor, error) {
	cursorBytes, decodeError := base64.RawURLEncoding.DecodeString(rawCursor)
	if decodeError != nil {
		return managedUsageFailureCursor{}, fmt.Errorf("%w: cursor_encoding: %v", errManagedUsageFailureQuery, decodeError)
	}
	decoder := json.NewDecoder(bytes.NewReader(cursorBytes))
	decoder.DisallowUnknownFields()
	var payload managedUsageFailureCursorPayload
	if decodeError := decoder.Decode(&payload); decodeError != nil {
		return managedUsageFailureCursor{}, fmt.Errorf("%w: cursor_payload: %v", errManagedUsageFailureQuery, decodeError)
	}
	var trailingValue any
	if trailingError := decoder.Decode(&trailingValue); !errors.Is(trailingError, io.EOF) {
		return managedUsageFailureCursor{}, fmt.Errorf("%w: cursor_trailing", errManagedUsageFailureQuery)
	}
	if payload.Version != managedUsageFailureCursorVersion || payload.Interval != string(expectedInterval) || payload.Scope != expectedScope ||
		payload.SnapshotID == 0 || payload.PositionID == 0 || payload.PositionID > payload.SnapshotID {
		return managedUsageFailureCursor{}, fmt.Errorf("%w: cursor_fields", errManagedUsageFailureQuery)
	}
	snapshotAt, snapshotError := time.Parse(time.RFC3339Nano, payload.SnapshotAt)
	if snapshotError != nil {
		return managedUsageFailureCursor{}, fmt.Errorf("%w: cursor_snapshot: %v", errManagedUsageFailureQuery, snapshotError)
	}
	positionAt, positionError := time.Parse(time.RFC3339Nano, payload.PositionAt)
	if positionError != nil || positionAt.After(snapshotAt) {
		return managedUsageFailureCursor{}, fmt.Errorf("%w: cursor_position", errManagedUsageFailureQuery)
	}
	cursor := managedUsageFailureCursor{
		interval:   expectedInterval,
		scope:      expectedScope,
		snapshotAt: snapshotAt.UTC(),
		snapshotID: payload.SnapshotID,
		positionAt: positionAt.UTC(),
		positionID: payload.PositionID,
	}
	if cursor.encoded() != rawCursor {
		return managedUsageFailureCursor{}, fmt.Errorf("%w: cursor_noncanonical", errManagedUsageFailureQuery)
	}
	return cursor, nil
}

func (cursor managedUsageFailureCursor) encoded() string {
	payload := `{"v":` + strconv.Itoa(managedUsageFailureCursorVersion) +
		`,"i":` + strconv.Quote(string(cursor.interval)) +
		`,"o":` + strconv.Quote(cursor.scope) +
		`,"s":` + strconv.Quote(cursor.snapshotAt.UTC().Format(time.RFC3339Nano)) +
		`,"x":` + strconv.FormatUint(uint64(cursor.snapshotID), 10) +
		`,"p":` + strconv.Quote(cursor.positionAt.UTC().Format(time.RFC3339Nano)) +
		`,"n":` + strconv.FormatUint(uint64(cursor.positionID), 10) + `}`
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

func decimalDigits(value string) bool {
	if value == constants.EmptyString {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
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

func (store *managedTenantStore) newManagedUsageRecord(requestTenant tenant, event managedUsageEvent) (managedUsageEventRecord, error) {
	timestamp := store.now()
	outcomeCode, outcomeError := newManagedUsageOutcomeCode(string(event.outcomeCode))
	if outcomeError != nil {
		return managedUsageEventRecord{
			TenantID:   requestTenant.identifier.string(),
			Endpoint:   event.endpoint,
			ProviderID: event.providerIdentifier,
			ModelID:    event.modelIdentifier,
			StatusCode: event.statusCode,
		}, fmt.Errorf("%w: tenant_id=%s: %w", errManagedTenantStorePersist, requestTenant.identifier.string(), outcomeError)
	}
	usageRecord := managedUsageEventRecord{
		TenantID:            requestTenant.identifier.string(),
		Endpoint:            event.endpoint,
		ProviderID:          event.providerIdentifier,
		ModelID:             event.modelIdentifier,
		StatusCode:          event.statusCode,
		Success:             outcomeCode == managedUsageOutcomeSuccess,
		OutcomeCode:         outcomeCode,
		LatencyMilliseconds: event.latencyMilliseconds,
		CreatedAt:           timestamp,
	}
	if event.usage != nil {
		usageRecord.RequestTokens = event.usage.RequestTokens
		usageRecord.ResponseTokens = event.usage.ResponseTokens
		usageRecord.TotalTokens = event.usage.TotalTokens
	}
	return usageRecord, nil
}

func (store *managedTenantStore) persistManagedUsageRecord(requestContext context.Context, usageRecord managedUsageEventRecord) error {
	if lockError := store.mutex.DatabaseWriteLockContext(requestContext); lockError != nil {
		return fmt.Errorf("%w: tenant_id=%s: %w", errManagedTenantStorePersist, usageRecord.TenantID, lockError)
	}
	defer store.mutex.DatabaseWriteUnlock()
	if persistError := store.database.createUsageEvent(requestContext, usageRecord); persistError != nil {
		return fmt.Errorf("%w: tenant_id=%s: %w", errManagedTenantStorePersist, usageRecord.TenantID, persistError)
	}
	return nil
}

func (store *managedTenantStore) usageSummary(principal managementPrincipal, tenantIdentifier managedTenantIdentifier, interval usageInterval) (managedUsageSummary, error) {
	store.mutex.Lock()
	_, accountError := store.ensureUserLocked(principal)
	record, recordError := store.database.tenantByOwnerAndID(principal.userID, tenantIdentifier.string())
	timestamp := store.now()
	store.mutex.Unlock()
	if accountError != nil {
		return managedUsageSummary{}, accountError
	}
	if recordError != nil {
		return managedUsageSummary{}, managedTenantQueryError(principal.userID, tenantIdentifier.string(), recordError)
	}
	return store.usageSummaryByTenantIDs(
		[]string{record.TenantID},
		interval,
		timestamp,
		"tenant_id="+record.TenantID,
	)
}

func (store *managedTenantStore) accountUsageSummary(principal managementPrincipal, interval usageInterval) (managedUsageSummary, error) {
	store.mutex.Lock()
	accountRecord, accountError := store.ensureUserLocked(principal)
	timestamp := store.now()
	store.mutex.Unlock()
	if accountError != nil {
		return managedUsageSummary{}, accountError
	}
	tenantIDs := managedTenantRecordIDs(accountRecord.Tenants)
	return store.usageSummaryByTenantIDs(
		tenantIDs,
		interval,
		timestamp,
		"user_id="+principal.userID,
	)
}

func (store *managedTenantStore) usageSummaryByTenantIDs(tenantIDs []string, interval usageInterval, timestamp time.Time, subject string) (managedUsageSummary, error) {
	windowDuration, _, _, finite := interval.finiteWindow()
	var earliestUsageEvent time.Time
	var recordsError error
	if !finite {
		earliestUsageEvent, recordsError = store.database.earliestUsageEventByTenantIDsThrough(tenantIDs, timestamp)
	}
	if recordsError != nil {
		return managedUsageSummary{}, fmt.Errorf("%w: %s: %v", errManagedTenantStorePersist, subject, recordsError)
	}
	accumulator := newManagedUsageSummaryAccumulator(interval, timestamp, earliestUsageEvent)
	if finite {
		recordsError = store.database.streamUsageEventsByTenantIDsBetween(tenantIDs, timestamp.Add(-windowDuration), timestamp, accumulator.apply)
	} else {
		recordsError = store.database.streamUsageEventsByTenantIDsThrough(tenantIDs, timestamp, accumulator.apply)
	}
	if recordsError != nil {
		return managedUsageSummary{}, fmt.Errorf("%w: %s: %v", errManagedTenantStorePersist, subject, recordsError)
	}
	return accumulator.summary(), nil
}

func managedTenantRecordIDs(records []managedTenantRecord) []string {
	tenantIDs := make([]string, 0, len(records))
	for _, record := range records {
		tenantIDs = append(tenantIDs, record.TenantID)
	}
	return tenantIDs
}

func (store *managedTenantStore) usageFailures(principal managementPrincipal, tenantIdentifier managedTenantIdentifier, query managedUsageFailureQuery) (managedUsageFailurePage, error) {
	snapshotAt, recordQuery := managedUsageFailureRecordQueryFor(query, store.now().UTC())
	records, resolvedSnapshotID, recordsError := store.database.usageFailuresByOwnerAndTenant(
		principal.userID,
		tenantIdentifier.string(),
		recordQuery,
	)
	if recordsError != nil {
		return managedUsageFailurePage{}, managedTenantQueryError(principal.userID, tenantIdentifier.string(), recordsError)
	}
	return managedUsageFailurePageFor(records, resolvedSnapshotID, query, snapshotAt), nil
}

func (store *managedTenantStore) accountUsageFailures(principal managementPrincipal, query managedUsageFailureQuery) (managedUsageFailurePage, error) {
	store.mutex.Lock()
	accountRecord, accountError := store.ensureUserLocked(principal)
	snapshotAt := store.now().UTC()
	store.mutex.Unlock()
	if accountError != nil {
		return managedUsageFailurePage{}, accountError
	}
	tenantIDs := managedTenantRecordIDs(accountRecord.Tenants)
	tenantNames := make(map[string]string, len(accountRecord.Tenants))
	for _, tenantRecord := range accountRecord.Tenants {
		tenantNames[tenantRecord.TenantID] = tenantRecord.Name
	}
	snapshotAt, recordQuery := managedUsageFailureRecordQueryFor(query, snapshotAt)
	records, resolvedSnapshotID, recordsError := store.database.usageFailuresByTenantIDs(tenantIDs, recordQuery)
	if recordsError != nil {
		return managedUsageFailurePage{}, fmt.Errorf("%w: user_id=%s: %v", errManagedTenantStorePersist, principal.userID, recordsError)
	}
	for recordIndex := range records {
		records[recordIndex].failure.tenantName = tenantNames[records[recordIndex].failure.tenantIdentifier]
	}
	return managedUsageFailurePageFor(records, resolvedSnapshotID, query, snapshotAt), nil
}

func managedUsageFailureRecordQueryFor(query managedUsageFailureQuery, currentTime time.Time) (time.Time, managedUsageFailureRecordQuery) {
	snapshotAt := currentTime
	var snapshotID *uint
	var position *managedUsageFailurePosition
	if query.cursor != nil {
		snapshotAt = query.cursor.snapshotAt
		snapshotIDValue := query.cursor.snapshotID
		snapshotID = &snapshotIDValue
		position = &managedUsageFailurePosition{
			occurredAt: query.cursor.positionAt,
			recordID:   query.cursor.positionID,
		}
	}
	var periodStart *time.Time
	if windowDuration, _, _, finite := query.interval.finiteWindow(); finite {
		periodStartValue := snapshotAt.Add(-windowDuration)
		periodStart = &periodStartValue
	}
	return snapshotAt, managedUsageFailureRecordQuery{
		periodStart: periodStart,
		snapshotAt:  snapshotAt,
		snapshotID:  snapshotID,
		position:    position,
		limit:       query.limit + 1,
	}
}

func managedUsageFailurePageFor(records []managedUsageFailureRecord, resolvedSnapshotID uint, query managedUsageFailureQuery, snapshotAt time.Time) managedUsageFailurePage {
	hasNextPage := len(records) > query.limit
	if hasNextPage {
		records = records[:query.limit]
	}
	failures := make([]managedUsageFailure, 0, len(records))
	for _, record := range records {
		failures = append(failures, record.failure)
	}
	page := managedUsageFailurePage{interval: query.interval, failures: failures}
	if hasNextPage {
		positionRecord := records[len(records)-1]
		cursor := managedUsageFailureCursor{
			interval:   query.interval,
			scope:      query.scope,
			snapshotAt: snapshotAt,
			snapshotID: resolvedSnapshotID,
			positionAt: positionRecord.failure.occurredAt,
			positionID: positionRecord.recordID,
		}
		page.nextCursor = cursor.encoded()
	}
	return page
}

func (store *managedTenantStore) adminUsersSummary() ([]managedAdminUserSnapshot, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	timestamp := store.now()
	periodStart := usagePeriodStart(timestamp)
	userRecords, userRecordsError := store.database.users()
	if userRecordsError != nil {
		return nil, fmt.Errorf("%w: admin_users: %v", errManagedTenantStorePersist, userRecordsError)
	}
	usageRecords, usageRecordsError := store.database.usageEventsSince(periodStart)
	if usageRecordsError != nil {
		return nil, fmt.Errorf("%w: admin_usage: %v", errManagedTenantStorePersist, usageRecordsError)
	}
	usageRecordsByTenantID := make(map[string][]managedUsageEventRecord)
	for _, usageRecord := range usageRecords {
		usageRecordsByTenantID[usageRecord.TenantID] = append(usageRecordsByTenantID[usageRecord.TenantID], usageRecord)
	}
	adminSnapshots := make([]managedAdminUserSnapshot, 0, len(userRecords))
	for _, userRecord := range userRecords {
		tenantSnapshots := make([]managedAdminTenantSnapshot, 0, len(userRecord.Tenants))
		for _, tenantRecord := range userRecord.Tenants {
			tenantSnapshots = append(tenantSnapshots, managedAdminTenantSnapshot{
				tenantID:  tenantRecord.TenantID,
				name:      tenantRecord.Name,
				hasSecret: tenantRecord.SecretDigest != nil,
				createdAt: tenantRecord.CreatedAt,
				updatedAt: tenantRecord.UpdatedAt,
				usage:     summarizeManagedAdminUsage(usageRecordsByTenantID[tenantRecord.TenantID], timestamp),
			})
		}
		adminSnapshots = append(adminSnapshots, managedAdminUserSnapshot{
			userID:          userRecord.UserID,
			userEmail:       userRecord.UserEmail,
			userDisplayName: userRecord.UserDisplayName,
			userAvatarURL:   userRecord.UserAvatarURL,
			tenants:         tenantSnapshots,
		})
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
	periodEnd := now.UTC()
	var earliestUsageEvent time.Time
	_, _, _, finite := interval.finiteWindow()
	if !finite {
		earliestUsageEvent = earliestManagedUsageEvent(records, periodEnd)
	}
	accumulator := newManagedUsageSummaryAccumulator(interval, now, earliestUsageEvent)
	for _, record := range records {
		accumulator.apply(record)
	}
	return accumulator.summary()
}

func newManagedUsageSummaryAccumulator(interval usageInterval, now time.Time, earliestUsageEvent time.Time) managedUsageSummaryAccumulator {
	windowDuration, bucketCount, bucketUnit, finite := interval.finiteWindow()
	periodEnd := now.UTC()
	periodStart := periodEnd.Add(-windowDuration)
	if !finite {
		periodStart, bucketCount = allUsagePeriod(earliestUsageEvent, periodEnd)
	}
	buckets := make([]managedUsageBucket, 0, bucketCount)
	bucketDuration := 24 * time.Hour
	if bucketUnit == usageBucketUnitHour {
		bucketDuration = time.Hour
	}
	for bucketIndex := 0; bucketIndex < bucketCount; bucketIndex++ {
		buckets = append(buckets, managedUsageBucket{start: periodStart.Add(time.Duration(bucketIndex) * bucketDuration)})
	}
	return managedUsageSummaryAccumulator{
		interval:       interval,
		bucketUnit:     bucketUnit,
		periodStart:    periodStart,
		periodEnd:      periodEnd,
		bucketDuration: bucketDuration,
		buckets:        buckets,
		usage:          newManagedUsageAccumulator(),
	}
}

func (accumulator *managedUsageSummaryAccumulator) apply(record managedUsageEventRecord) {
	if record.CreatedAt.Before(accumulator.periodStart) || record.CreatedAt.After(accumulator.periodEnd) {
		return
	}
	accumulator.usage.apply(record)
	bucketPosition := int(record.CreatedAt.Sub(accumulator.periodStart) / accumulator.bucketDuration)
	if bucketPosition == len(accumulator.buckets) {
		bucketPosition--
	}
	applyUsageRecord(&accumulator.buckets[bucketPosition].aggregate, record)
}

func (accumulator managedUsageSummaryAccumulator) summary() managedUsageSummary {
	for bucketIndex := range accumulator.buckets {
		finalizeUsageAggregate(&accumulator.buckets[bucketIndex].aggregate)
	}
	totals, providers, models, statuses := accumulator.usage.summary()
	return managedUsageSummary{
		interval:    accumulator.interval,
		bucketUnit:  accumulator.bucketUnit,
		totals:      totals,
		buckets:     accumulator.buckets,
		providers:   providers,
		models:      models,
		statusCodes: statuses,
	}
}

func earliestManagedUsageEvent(records []managedUsageEventRecord, periodEnd time.Time) time.Time {
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
	return earliest
}

func allUsagePeriod(earliestUsageEvent time.Time, periodEnd time.Time) (time.Time, int) {
	if earliestUsageEvent.IsZero() {
		return periodEnd, 0
	}
	earliestUTC := earliestUsageEvent.UTC()
	periodStart := time.Date(earliestUTC.Year(), earliestUTC.Month(), earliestUTC.Day(), 0, 0, 0, 0, time.UTC)
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
