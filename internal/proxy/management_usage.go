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
	usageEndpointDictation = "dictation"
	usageDateFormat        = "2006-01-02"
	usageIntervalAll       = usageInterval("all")
	usageIntervalThirtyDay = usageInterval("30d")
	usageIntervalSevenDay  = usageInterval("7d")
	usageIntervalOneDay    = usageInterval("1d")
	usageBucketUnitDay     = usageBucketUnit("day")
	usageBucketUnitHour    = usageBucketUnit("hour")

	managedUsageDetailCursorVersion = 3
	managedUsageDetailDefaultLimit  = 25
	managedUsageDetailMaximumLimit  = 100
	managedUsagePersistenceTimeout  = 5 * time.Second
	usageDetailIntervalQuery        = "interval"
	usageDetailLimitQuery           = "limit"
	usageDetailCursorQuery          = "cursor"
	managedUsageAllTenantsScope     = "all-tenants"

	managedUsageOutcomeSuccess               = managedUsageOutcomeCode("success")
	managedUsageOutcomeInvalidRequest        = managedUsageOutcomeCode("invalid_request")
	managedUsageOutcomePayloadTooLarge       = managedUsageOutcomeCode("payload_too_large")
	managedUsageOutcomeProviderNotConfigured = managedUsageOutcomeCode("provider_not_configured")
	managedUsageOutcomeRateLimited           = managedUsageOutcomeCode("rate_limited")
	managedUsageOutcomeServiceUnavailable    = managedUsageOutcomeCode("service_unavailable")
	managedUsageOutcomeRequestTimeout        = managedUsageOutcomeCode("request_timeout")
	managedUsageOutcomeUpstreamError         = managedUsageOutcomeCode("upstream_error")
	managedUsageOutcomeProxyError            = managedUsageOutcomeCode("proxy_error")

	managedUsageDispositionRejected  = managedUsageDisposition("rejected")
	managedUsageDispositionSucceeded = managedUsageDisposition("succeeded")
	managedUsageDispositionFailed    = managedUsageDisposition("failed")
)

var (
	errManagedUsageIntervalInvalid    = errors.New("managed_usage_interval_invalid")
	errManagedUsageDetailQuery        = errors.New("managed_usage_detail_query_invalid")
	errManagedUsageOutcomeInvalid     = errors.New("managed_usage_outcome_invalid")
	errManagedUsageDispositionInvalid = errors.New("managed_usage_disposition_invalid")
)

type usageInterval string

type usageBucketUnit string

type managedUsageOutcomeCode string

type managedUsageDisposition string

type managedUsageRoute struct {
	providerIdentifier providerID
	modelIdentifier    modelID
}

type managedUsageEvent struct {
	endpoint            string
	route               *managedUsageRoute
	statusCode          int
	outcomeCode         managedUsageOutcomeCode
	latencyMilliseconds int64
	usage               *tokenUsage
}

type managedUsageDetailQuery struct {
	interval    usageInterval
	scope       string
	disposition managedUsageDisposition
	limit       int
	cursor      *managedUsageDetailCursor
}

type managedUsageDetailCursor struct {
	interval    usageInterval
	scope       string
	disposition managedUsageDisposition
	snapshotAt  time.Time
	snapshotID  uint
	positionAt  time.Time
	positionID  uint
}

type managedUsageDetailCursorPayload struct {
	Version     int    `json:"v"`
	Interval    string `json:"i"`
	Scope       string `json:"o"`
	Disposition string `json:"d"`
	SnapshotAt  string `json:"s"`
	SnapshotID  uint   `json:"x"`
	PositionAt  string `json:"p"`
	PositionID  uint   `json:"n"`
}

type managedUsageDetailRecordQuery struct {
	periodStart *time.Time
	disposition managedUsageDisposition
	snapshotAt  time.Time
	snapshotID  *uint
	position    *managedUsageDetailPosition
	limit       int
}

type managedUsageDetailPosition struct {
	occurredAt time.Time
	recordID   uint
}

type managedUsageDetailPage struct {
	interval   usageInterval
	details    []managedUsageDetail
	nextCursor string
}

type managedUsageDetail struct {
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

type managedUsageDetailRecord struct {
	recordID uint
	detail   managedUsageDetail
}

type managedUsageSummary struct {
	interval         usageInterval
	bucketUnit       usageBucketUnit
	rejectedRequests int
	totals           managedUsageAggregate
	buckets          []managedUsageBucket
	providers        []managedUsageProviderBucket
	models           []managedUsageModelBucket
	statusCodes      []managedUsageStatusBucket
}

type managedAdminUsageSummary struct {
	periodDays       int
	rejectedRequests int
	totals           managedUsageAggregate
	daily            []managedUsageDailyBucket
	providers        []managedUsageProviderBucket
	models           []managedUsageModelBucket
	statusCodes      []managedUsageStatusBucket
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
	interval         usageInterval
	bucketUnit       usageBucketUnit
	periodStart      time.Time
	periodEnd        time.Time
	bucketDuration   time.Duration
	buckets          []managedUsageBucket
	rejectedRequests int
	usage            managedUsageAccumulator
}

type managedUsageExecution struct {
	succeeded bool
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
		managedUsageOutcomeProviderNotConfigured,
		managedUsageOutcomeRateLimited,
		managedUsageOutcomeServiceUnavailable,
		managedUsageOutcomeRequestTimeout,
		managedUsageOutcomeUpstreamError,
		managedUsageOutcomeProxyError:
		return outcomeCode, nil
	default:
		return "", fmt.Errorf("%w: outcome_code=%q", errManagedUsageOutcomeInvalid, value)
	}
}

func newManagedUsageDisposition(value string) (managedUsageDisposition, error) {
	disposition := managedUsageDisposition(value)
	switch disposition {
	case managedUsageDispositionRejected, managedUsageDispositionSucceeded, managedUsageDispositionFailed:
		return disposition, nil
	default:
		return "", fmt.Errorf("%w: disposition=%q", errManagedUsageDispositionInvalid, value)
	}
}

func managedUsageDispositionForOutcome(outcomeCode managedUsageOutcomeCode) (managedUsageDisposition, error) {
	switch outcomeCode {
	case managedUsageOutcomeSuccess:
		return managedUsageDispositionSucceeded, nil
	case managedUsageOutcomeInvalidRequest, managedUsageOutcomePayloadTooLarge, managedUsageOutcomeProviderNotConfigured:
		return managedUsageDispositionRejected, nil
	case managedUsageOutcomeRateLimited, managedUsageOutcomeServiceUnavailable, managedUsageOutcomeRequestTimeout, managedUsageOutcomeUpstreamError, managedUsageOutcomeProxyError:
		return managedUsageDispositionFailed, nil
	default:
		return "", fmt.Errorf("%w: outcome_code=%q", errManagedUsageOutcomeInvalid, outcomeCode)
	}
}

func managedUsageRecordDisposition(record managedUsageEventRecord) (managedUsageDisposition, error) {
	disposition, dispositionError := newManagedUsageDisposition(string(record.Disposition))
	if dispositionError != nil {
		return "", dispositionError
	}
	expectedDisposition, mappingError := managedUsageDispositionForOutcome(record.OutcomeCode)
	if mappingError != nil {
		return "", mappingError
	}
	if disposition != expectedDisposition {
		return "", fmt.Errorf("%w: disposition=%q outcome_code=%q", errManagedUsageDispositionInvalid, disposition, record.OutcomeCode)
	}
	return disposition, nil
}

func newManagedUsageDetailQuery(queryValues url.Values, scope string, disposition managedUsageDisposition) (managedUsageDetailQuery, error) {
	if strings.TrimSpace(scope) == constants.EmptyString {
		return managedUsageDetailQuery{}, fmt.Errorf("%w: scope", errManagedUsageDetailQuery)
	}
	if _, dispositionError := newManagedUsageDisposition(string(disposition)); dispositionError != nil || disposition == managedUsageDispositionSucceeded {
		return managedUsageDetailQuery{}, fmt.Errorf("%w: disposition=%q", errManagedUsageDetailQuery, disposition)
	}
	for queryName := range queryValues {
		switch queryName {
		case usageDetailIntervalQuery, usageDetailLimitQuery, usageDetailCursorQuery:
		default:
			return managedUsageDetailQuery{}, fmt.Errorf("%w: unknown_query=%s", errManagedUsageDetailQuery, queryName)
		}
	}
	intervalValues := queryValues[usageDetailIntervalQuery]
	if len(intervalValues) != 1 {
		return managedUsageDetailQuery{}, fmt.Errorf("%w: interval_count=%d", errManagedUsageDetailQuery, len(intervalValues))
	}
	interval, intervalError := newUsageInterval(intervalValues[0])
	if intervalError != nil {
		return managedUsageDetailQuery{}, fmt.Errorf("%w: %v", errManagedUsageDetailQuery, intervalError)
	}
	limit := managedUsageDetailDefaultLimit
	if limitValues, supplied := queryValues[usageDetailLimitQuery]; supplied {
		if len(limitValues) != 1 || !decimalDigits(limitValues[0]) {
			return managedUsageDetailQuery{}, fmt.Errorf("%w: limit", errManagedUsageDetailQuery)
		}
		parsedLimit, parseError := strconv.Atoi(limitValues[0])
		if parseError != nil || parsedLimit < 1 || parsedLimit > managedUsageDetailMaximumLimit || strconv.Itoa(parsedLimit) != limitValues[0] {
			return managedUsageDetailQuery{}, fmt.Errorf("%w: limit=%q", errManagedUsageDetailQuery, limitValues[0])
		}
		limit = parsedLimit
	}
	var cursor *managedUsageDetailCursor
	if cursorValues, supplied := queryValues[usageDetailCursorQuery]; supplied {
		if len(cursorValues) != 1 || strings.TrimSpace(cursorValues[0]) == constants.EmptyString {
			return managedUsageDetailQuery{}, fmt.Errorf("%w: cursor", errManagedUsageDetailQuery)
		}
		parsedCursor, cursorError := newManagedUsageDetailCursor(cursorValues[0], interval, scope, disposition)
		if cursorError != nil {
			return managedUsageDetailQuery{}, cursorError
		}
		cursor = &parsedCursor
	}
	return managedUsageDetailQuery{interval: interval, scope: scope, disposition: disposition, limit: limit, cursor: cursor}, nil
}

func newManagedUsageDetailCursor(rawCursor string, expectedInterval usageInterval, expectedScope string, expectedDisposition managedUsageDisposition) (managedUsageDetailCursor, error) {
	cursorBytes, decodeError := base64.RawURLEncoding.DecodeString(rawCursor)
	if decodeError != nil {
		return managedUsageDetailCursor{}, fmt.Errorf("%w: cursor_encoding: %v", errManagedUsageDetailQuery, decodeError)
	}
	decoder := json.NewDecoder(bytes.NewReader(cursorBytes))
	decoder.DisallowUnknownFields()
	var payload managedUsageDetailCursorPayload
	if decodeError := decoder.Decode(&payload); decodeError != nil {
		return managedUsageDetailCursor{}, fmt.Errorf("%w: cursor_payload: %v", errManagedUsageDetailQuery, decodeError)
	}
	var trailingValue any
	if trailingError := decoder.Decode(&trailingValue); !errors.Is(trailingError, io.EOF) {
		return managedUsageDetailCursor{}, fmt.Errorf("%w: cursor_trailing", errManagedUsageDetailQuery)
	}
	if payload.Version != managedUsageDetailCursorVersion || payload.Interval != string(expectedInterval) || payload.Scope != expectedScope || payload.Disposition != string(expectedDisposition) ||
		payload.SnapshotID == 0 || payload.PositionID == 0 || payload.PositionID > payload.SnapshotID {
		return managedUsageDetailCursor{}, fmt.Errorf("%w: cursor_fields", errManagedUsageDetailQuery)
	}
	snapshotAt, snapshotError := time.Parse(time.RFC3339Nano, payload.SnapshotAt)
	if snapshotError != nil {
		return managedUsageDetailCursor{}, fmt.Errorf("%w: cursor_snapshot: %v", errManagedUsageDetailQuery, snapshotError)
	}
	positionAt, positionError := time.Parse(time.RFC3339Nano, payload.PositionAt)
	if positionError != nil || positionAt.After(snapshotAt) {
		return managedUsageDetailCursor{}, fmt.Errorf("%w: cursor_position", errManagedUsageDetailQuery)
	}
	cursor := managedUsageDetailCursor{
		interval:    expectedInterval,
		scope:       expectedScope,
		disposition: expectedDisposition,
		snapshotAt:  snapshotAt.UTC(),
		snapshotID:  payload.SnapshotID,
		positionAt:  positionAt.UTC(),
		positionID:  payload.PositionID,
	}
	if cursor.encoded() != rawCursor {
		return managedUsageDetailCursor{}, fmt.Errorf("%w: cursor_noncanonical", errManagedUsageDetailQuery)
	}
	return cursor, nil
}

func (cursor managedUsageDetailCursor) encoded() string {
	payload := `{"v":` + strconv.Itoa(managedUsageDetailCursorVersion) +
		`,"i":` + strconv.Quote(string(cursor.interval)) +
		`,"o":` + strconv.Quote(cursor.scope) +
		`,"d":` + strconv.Quote(string(cursor.disposition)) +
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
	providerIdentifier := constants.EmptyString
	modelIdentifier := constants.EmptyString
	if event.route != nil {
		providerIdentifier = event.route.providerIdentifier.string()
		modelIdentifier = event.route.modelIdentifier.string()
	}
	disposition, dispositionError := managedUsageDispositionForOutcome(event.outcomeCode)
	if dispositionError != nil {
		return managedUsageEventRecord{
			TenantID:   requestTenant.identifier.string(),
			Endpoint:   event.endpoint,
			ProviderID: providerIdentifier,
			ModelID:    modelIdentifier,
			StatusCode: event.statusCode,
		}, fmt.Errorf("%w: tenant_id=%s: %w", errManagedTenantStorePersist, requestTenant.identifier.string(), dispositionError)
	}
	usageRecord := managedUsageEventRecord{
		TenantID:            requestTenant.identifier.string(),
		Endpoint:            event.endpoint,
		ProviderID:          providerIdentifier,
		ModelID:             modelIdentifier,
		StatusCode:          event.statusCode,
		Disposition:         disposition,
		OutcomeCode:         event.outcomeCode,
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

func (store *managedTenantStore) usageDetails(principal managementPrincipal, tenantIdentifier managedTenantIdentifier, query managedUsageDetailQuery) (managedUsageDetailPage, error) {
	snapshotAt, recordQuery := managedUsageDetailRecordQueryFor(query, store.now().UTC())
	records, resolvedSnapshotID, recordsError := store.database.usageDetailsByOwnerAndTenant(
		principal.userID,
		tenantIdentifier.string(),
		recordQuery,
	)
	if recordsError != nil {
		return managedUsageDetailPage{}, managedTenantQueryError(principal.userID, tenantIdentifier.string(), recordsError)
	}
	return managedUsageDetailPageFor(records, resolvedSnapshotID, query, snapshotAt), nil
}

func (store *managedTenantStore) accountUsageDetails(principal managementPrincipal, query managedUsageDetailQuery) (managedUsageDetailPage, error) {
	store.mutex.Lock()
	accountRecord, accountError := store.ensureUserLocked(principal)
	snapshotAt := store.now().UTC()
	store.mutex.Unlock()
	if accountError != nil {
		return managedUsageDetailPage{}, accountError
	}
	tenantIDs := managedTenantRecordIDs(accountRecord.Tenants)
	tenantNames := make(map[string]string, len(accountRecord.Tenants))
	for _, tenantRecord := range accountRecord.Tenants {
		tenantNames[tenantRecord.TenantID] = tenantRecord.Name
	}
	snapshotAt, recordQuery := managedUsageDetailRecordQueryFor(query, snapshotAt)
	records, resolvedSnapshotID, recordsError := store.database.usageDetailsByTenantIDs(tenantIDs, recordQuery)
	if recordsError != nil {
		return managedUsageDetailPage{}, fmt.Errorf("%w: user_id=%s: %v", errManagedTenantStorePersist, principal.userID, recordsError)
	}
	for recordIndex := range records {
		records[recordIndex].detail.tenantName = tenantNames[records[recordIndex].detail.tenantIdentifier]
	}
	return managedUsageDetailPageFor(records, resolvedSnapshotID, query, snapshotAt), nil
}

func managedUsageDetailRecordQueryFor(query managedUsageDetailQuery, currentTime time.Time) (time.Time, managedUsageDetailRecordQuery) {
	snapshotAt := currentTime
	var snapshotID *uint
	var position *managedUsageDetailPosition
	if query.cursor != nil {
		snapshotAt = query.cursor.snapshotAt
		snapshotIDValue := query.cursor.snapshotID
		snapshotID = &snapshotIDValue
		position = &managedUsageDetailPosition{
			occurredAt: query.cursor.positionAt,
			recordID:   query.cursor.positionID,
		}
	}
	var periodStart *time.Time
	if windowDuration, _, _, finite := query.interval.finiteWindow(); finite {
		periodStartValue := snapshotAt.Add(-windowDuration)
		periodStart = &periodStartValue
	}
	return snapshotAt, managedUsageDetailRecordQuery{
		periodStart: periodStart,
		disposition: query.disposition,
		snapshotAt:  snapshotAt,
		snapshotID:  snapshotID,
		position:    position,
		limit:       query.limit + 1,
	}
}

func managedUsageDetailPageFor(records []managedUsageDetailRecord, resolvedSnapshotID uint, query managedUsageDetailQuery, snapshotAt time.Time) managedUsageDetailPage {
	hasNextPage := len(records) > query.limit
	if hasNextPage {
		records = records[:query.limit]
	}
	details := make([]managedUsageDetail, 0, len(records))
	for _, record := range records {
		details = append(details, record.detail)
	}
	page := managedUsageDetailPage{interval: query.interval, details: details}
	if hasNextPage {
		positionRecord := records[len(records)-1]
		cursor := managedUsageDetailCursor{
			interval:    query.interval,
			scope:       query.scope,
			disposition: query.disposition,
			snapshotAt:  snapshotAt,
			snapshotID:  resolvedSnapshotID,
			positionAt:  positionRecord.detail.occurredAt,
			positionID:  positionRecord.recordID,
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
			usageSummary, usageError := summarizeManagedAdminUsage(usageRecordsByTenantID[tenantRecord.TenantID], timestamp)
			if usageError != nil {
				return nil, fmt.Errorf("%w: admin_usage tenant_id=%s: %v", errManagedTenantStorePersist, tenantRecord.TenantID, usageError)
			}
			tenantSnapshots = append(tenantSnapshots, managedAdminTenantSnapshot{
				tenantID:  tenantRecord.TenantID,
				name:      tenantRecord.Name,
				hasSecret: tenantRecord.SecretDigest != nil,
				createdAt: tenantRecord.CreatedAt,
				updatedAt: tenantRecord.UpdatedAt,
				usage:     usageSummary,
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

func (accumulator *managedUsageSummaryAccumulator) apply(record managedUsageEventRecord) error {
	if record.CreatedAt.After(accumulator.periodEnd) {
		return nil
	}
	disposition, dispositionError := managedUsageRecordDisposition(record)
	if dispositionError != nil {
		return dispositionError
	}
	if disposition == managedUsageDispositionRejected {
		if accumulator.interval == usageIntervalAll || !record.CreatedAt.Before(accumulator.periodStart) {
			accumulator.rejectedRequests++
		}
		return nil
	}
	if record.CreatedAt.Before(accumulator.periodStart) {
		return nil
	}
	execution := managedUsageExecution{succeeded: disposition == managedUsageDispositionSucceeded}
	accumulator.usage.apply(record, execution)
	bucketPosition := int(record.CreatedAt.Sub(accumulator.periodStart) / accumulator.bucketDuration)
	if bucketPosition == len(accumulator.buckets) {
		bucketPosition--
	}
	applyUsageRecord(&accumulator.buckets[bucketPosition].aggregate, record, execution)
	return nil
}

func (accumulator managedUsageSummaryAccumulator) summary() managedUsageSummary {
	for bucketIndex := range accumulator.buckets {
		finalizeUsageAggregate(&accumulator.buckets[bucketIndex].aggregate)
	}
	totals, providers, models, statuses := accumulator.usage.summary()
	return managedUsageSummary{
		interval:         accumulator.interval,
		bucketUnit:       accumulator.bucketUnit,
		rejectedRequests: accumulator.rejectedRequests,
		totals:           totals,
		buckets:          accumulator.buckets,
		providers:        providers,
		models:           models,
		statusCodes:      statuses,
	}
}

func earliestManagedUsageEvent(records []managedUsageEventRecord, periodEnd time.Time) time.Time {
	var earliest time.Time
	for _, record := range records {
		recordTime := record.CreatedAt.UTC()
		if record.Disposition == managedUsageDispositionRejected || recordTime.After(periodEnd) {
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

func summarizeManagedAdminUsage(records []managedUsageEventRecord, now time.Time) (managedAdminUsageSummary, error) {
	periodStart := usagePeriodStart(now)
	dailyBuckets := make([]managedUsageDailyBucket, 0, managedUsageSummaryDays)
	dailyIndex := make(map[string]int, managedUsageSummaryDays)
	for dayOffset := 0; dayOffset < managedUsageSummaryDays; dayOffset++ {
		date := periodStart.AddDate(0, 0, dayOffset).Format(usageDateFormat)
		dailyIndex[date] = len(dailyBuckets)
		dailyBuckets = append(dailyBuckets, managedUsageDailyBucket{date: date})
	}
	accumulator := newManagedUsageAccumulator()
	rejectedRequests := 0
	for _, record := range records {
		if record.CreatedAt.Before(periodStart) || record.CreatedAt.After(now) {
			continue
		}
		disposition, dispositionError := managedUsageRecordDisposition(record)
		if dispositionError != nil {
			return managedAdminUsageSummary{}, dispositionError
		}
		if disposition == managedUsageDispositionRejected {
			rejectedRequests++
			continue
		}
		execution := managedUsageExecution{succeeded: disposition == managedUsageDispositionSucceeded}
		accumulator.apply(record, execution)
		recordDate := record.CreatedAt.UTC().Format(usageDateFormat)
		if dailyPosition, exists := dailyIndex[recordDate]; exists {
			applyUsageRecord(&dailyBuckets[dailyPosition].aggregate, record, execution)
		}
	}
	for dailyIndex := range dailyBuckets {
		finalizeUsageAggregate(&dailyBuckets[dailyIndex].aggregate)
	}
	totals, providers, models, statuses := accumulator.summary()
	return managedAdminUsageSummary{
		periodDays:       managedUsageSummaryDays,
		rejectedRequests: rejectedRequests,
		totals:           totals,
		daily:            dailyBuckets,
		providers:        providers,
		models:           models,
		statusCodes:      statuses,
	}, nil
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

func (accumulator *managedUsageAccumulator) apply(record managedUsageEventRecord, execution managedUsageExecution) {
	applyUsageRecord(&accumulator.totals, record, execution)
	if record.ProviderID != constants.EmptyString && record.ModelID != constants.EmptyString {
		providerAggregate := accumulator.providers[record.ProviderID]
		applyUsageRecord(&providerAggregate, record, execution)
		accumulator.providers[record.ProviderID] = providerAggregate

		modelKey := record.ProviderID + "\x00" + record.ModelID
		modelBucket := accumulator.models[modelKey]
		modelBucket.providerIdentifier = record.ProviderID
		modelBucket.modelIdentifier = record.ModelID
		applyUsageRecord(&modelBucket.aggregate, record, execution)
		accumulator.models[modelKey] = modelBucket
	}
	accumulator.statuses[record.StatusCode]++
}

func (accumulator managedUsageAccumulator) summary() (managedUsageAggregate, []managedUsageProviderBucket, []managedUsageModelBucket, []managedUsageStatusBucket) {
	finalizeUsageAggregate(&accumulator.totals)
	return accumulator.totals,
		usageProviderBucketList(accumulator.providers),
		usageModelBucketList(accumulator.models),
		usageStatusBucketList(accumulator.statuses)
}

func applyUsageRecord(aggregate *managedUsageAggregate, record managedUsageEventRecord, execution managedUsageExecution) {
	aggregate.requests++
	if execution.succeeded {
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
