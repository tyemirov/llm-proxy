//go:build pilot_presubmission_recovery

package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const pilotRecoveryTenant = "managed-60634d2470f31557462f52e319f07d07"
const pilotRecoveryOperation = "mop_ed5b76bf5e081112de5d109d853c6c0b"
const pilotRecoveryUsageID = 11237
const pilotRecoveryDelivery = "2026-11-04T07:51:14.363413403Z"
const pilotRecoverySchema = "llm-proxy.media-presubmission-recovery"

type pilotRecoveryEvidence struct {
	RecoveryID              string    `json:"recovery_id"`
	TenantID                string    `json:"tenant_id"`
	OperationID             string    `json:"operation_id"`
	UsageEventID            uint      `json:"usage_event_id"`
	DeliveryAt              time.Time `json:"delivery_at"`
	Reason                  string    `json:"reason"`
	SourceReference         string    `json:"source_reference"`
	NativeSubmissionReached *bool     `json:"native_submission_reached"`
	ProviderHistoryComplete *bool     `json:"provider_history_complete"`
	MatchingNativeVideos    *int      `json:"matching_native_videos"`
}
type pilotRecoverySnapshot struct {
	Operation               mediaOperationRecord                 `json:"operation"`
	References              []mediaOperationAssetReferenceRecord `json:"references"`
	Avatar                  mediaAvatarRecord                    `json:"avatar"`
	Asset                   tenantAssetMetadata                  `json:"asset"`
	Delivery                mediaOperationUsageDeliveryRecord    `json:"delivery"`
	Usage                   managedUsageEventRecord              `json:"usage"`
	CurrentCatalogRevision  string                               `json:"current_catalog_revision"`
	CurrentExecutionBinding string                               `json:"current_execution_binding"`
	CredentialReference     string                               `json:"credential_reference"`
}
type pilotRecoveryPlan struct {
	Schema         string                `json:"schema"`
	Evidence       pilotRecoveryEvidence `json:"evidence"`
	Snapshot       pilotRecoverySnapshot `json:"snapshot"`
	RenewalSeconds int                   `json:"renewal_seconds"`
}
type pilotRecoveryReceipt struct {
	RecoveryID   string    `json:"recovery_id"`
	OperationID  string    `json:"operation_id"`
	UsageEventID uint      `json:"usage_event_id"`
	Phase        string    `json:"phase"`
	State        string    `json:"state"`
	AcceptedAt   time.Time `json:"accepted_at"`
	DeadlineAt   time.Time `json:"deadline_at"`
	RecordedAt   time.Time `json:"recorded_at"`
}
type mediaPresubmissionRecoveryRecord struct {
	OperationID      string    `gorm:"primaryKey"`
	Plan             string    `gorm:"not null"`
	AppliedOperation string    `gorm:"not null"`
	ApplyReceipt     string    `gorm:"not null"`
	AppliedAt        time.Time `gorm:"not null"`
	FinalOperation   string
	FinalUsage       string
	FinalReceipt     string
	FinalizedAt      *time.Time
}

// RunMediaPresubmissionRecovery operates the temporary, exact pilot recovery.
// Preparation and preview use a read-only SQLite connection without migrations.
func RunMediaPresubmissionRecovery(ctx context.Context, configuration Configuration, phase string, input io.Reader) (result json.RawMessage, resultError error) {
	if phase != "prepare" && phase != "preview" && phase != "apply" && phase != "finalize" {
		return nil, errors.New("unknown recovery phase")
	}
	var plan pilotRecoveryPlan
	if phase == "prepare" {
		if err := decodePilotRecovery(input, &plan.Evidence); err != nil {
			return nil, err
		}
	} else if err := decodePilotRecovery(input, &plan); err != nil {
		return nil, err
	}
	if err := validatePilotRecoveryEvidence(plan.Evidence); err != nil {
		return nil, err
	}
	if phase != "prepare" && (plan.Schema != pilotRecoverySchema || plan.RenewalSeconds <= 0 || plan.RenewalSeconds > DefaultMediaOperationLifetimeSeconds) {
		return nil, errors.New("invalid recovery plan contract")
	}
	if strings.TrimSpace(configuration.Management.DatabasePath) == "" {
		return nil, errors.New("recovery requires an existing management database")
	}
	location := url.URL{Scheme: "file", Path: configuration.Management.DatabasePath}
	query := url.Values{"mode": []string{"rw"}, "_pragma": []string{"busy_timeout(5000)"}}
	if phase == "prepare" || phase == "preview" {
		query.Set("mode", "ro")
		query.Add("_pragma", "query_only(1)")
	}
	location.RawQuery = query.Encode()
	database, err := gorm.Open(sqlite.Open(location.String()), &gorm.Config{TranslateError: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open recovery database: %w", err)
	}
	connection, err := database.DB()
	if err != nil {
		return nil, err
	}
	defer func() { resultError = errors.Join(resultError, connection.Close()) }()
	database = database.WithContext(ctx)
	if phase == "prepare" || phase == "preview" {
		if phase == "preview" {
			audit, found, err := findPilotRecoveryAudit(database, plan)
			if err != nil {
				return nil, err
			}
			if found {
				return json.Marshal(map[string]any{"phase": "preview", "already_applied": true, "apply_receipt": json.RawMessage(audit.ApplyReceipt)})
			}
		}
		snapshot, err := loadPilotRecoverySnapshot(ctx, database, configuration, plan.Evidence)
		if err != nil {
			return nil, err
		}
		if phase == "prepare" {
			plan.Schema = pilotRecoverySchema
			plan.Snapshot = snapshot
			plan.RenewalSeconds = DefaultMediaOperationLifetimeSeconds
			return json.Marshal(plan)
		}
		if !pilotRecoveryEqual(snapshot, plan.Snapshot) {
			return nil, errors.New("recovery snapshot changed after preparation")
		}
		return json.Marshal(map[string]any{"phase": "preview", "eligible": true, "operation_id": pilotRecoveryOperation, "usage_event_id": pilotRecoveryUsageID, "renewal_seconds": plan.RenewalSeconds, "dispatch_after_apply": true})
	}
	err = database.Transaction(func(tx *gorm.DB) error {
		if phase == "apply" {
			if err := tx.Exec(`CREATE TABLE IF NOT EXISTS media_presubmission_recovery_records (operation_id TEXT PRIMARY KEY, plan TEXT NOT NULL, applied_operation TEXT NOT NULL, apply_receipt TEXT NOT NULL, applied_at DATETIME NOT NULL, final_operation TEXT, final_usage TEXT, final_receipt TEXT, finalized_at DATETIME)`).Error; err != nil {
				return fmt.Errorf("create private recovery audit: %w", err)
			}
		}
		lock := tx.Model(&mediaOperationRecord{}).Where("operation_id = ? AND tenant_id = ?", pilotRecoveryOperation, pilotRecoveryTenant).UpdateColumn("operation_id", gorm.Expr("operation_id"))
		if lock.Error != nil {
			return lock.Error
		}
		if lock.RowsAffected != 1 {
			return errors.New("recovery target is absent")
		}
		audit, found, err := findPilotRecoveryAudit(tx, plan)
		if err != nil {
			return err
		}
		if phase == "apply" {
			if found {
				result = json.RawMessage(audit.ApplyReceipt)
				return nil
			}
			snapshot, err := loadPilotRecoverySnapshot(ctx, tx, configuration, plan.Evidence)
			if err != nil {
				return err
			}
			if !pilotRecoveryEqual(snapshot, plan.Snapshot) {
				return errors.New("recovery snapshot changed before apply")
			}
			now := time.Now().UTC()
			after := snapshot.Operation
			after.PublicState = MediaOperationStateQueued
			after.ProviderExecutionState = MediaProviderExecutionNotDispatched
			after.PublicErrorCode = ""
			after.TerminalAt = nil
			after.UpdatedAt = now
			after.DeadlineAt = now.Add(time.Duration(plan.RenewalSeconds) * time.Second)
			update := tx.Model(&mediaOperationRecord{}).Where("operation_id = ? AND tenant_id = ? AND public_state = ? AND provider_execution_state = ? AND updated_at = ?", pilotRecoveryOperation, pilotRecoveryTenant, snapshot.Operation.PublicState, snapshot.Operation.ProviderExecutionState, snapshot.Operation.UpdatedAt).Select("public_state", "provider_execution_state", "public_error_code", "terminal_at", "updated_at", "deadline_at").Updates(&after)
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected != 1 {
				return errors.New("recovery operation compare-and-swap failed")
			}
			receipt := pilotRecoveryReceipt{RecoveryID: plan.Evidence.RecoveryID, OperationID: pilotRecoveryOperation, UsageEventID: pilotRecoveryUsageID, Phase: "apply", State: after.PublicState, AcceptedAt: after.AcceptedAt, DeadlineAt: after.DeadlineAt, RecordedAt: now}
			result, _ = json.Marshal(receipt)
			audit = mediaPresubmissionRecoveryRecord{OperationID: pilotRecoveryOperation, Plan: pilotRecoveryJSON(plan), AppliedOperation: pilotRecoveryJSON(after), ApplyReceipt: string(result), AppliedAt: now}
			return tx.Create(&audit).Error
		}
		if !found {
			return errors.New("recovery has not been applied")
		}
		if audit.FinalizedAt != nil {
			result = json.RawMessage(audit.FinalReceipt)
			return nil
		}
		var current mediaOperationRecord
		if err := tx.First(&current, "operation_id = ?", pilotRecoveryOperation).Error; err != nil {
			return err
		}
		if current.PublicState != MediaOperationStateSucceeded && current.PublicState != MediaOperationStateFailed && current.PublicState != MediaOperationStateCancelled {
			return errors.New("recovery operation has no concrete terminal outcome")
		}
		var applied mediaOperationRecord
		if err := json.Unmarshal([]byte(audit.AppliedOperation), &applied); err != nil {
			return fmt.Errorf("decode applied recovery audit: %w", err)
		}
		if current.TerminalAt == nil || !pilotRecoveryImmutableOperation(applied, current) {
			return errors.New("recovery terminal operation identity changed")
		}
		if err := pilotRecoveryNoWork(tx); err != nil {
			return err
		}
		usageLock := tx.Model(&managedUsageEventRecord{}).Where("id = ? AND tenant_id = ?", pilotRecoveryUsageID, pilotRecoveryTenant).UpdateColumn("id", gorm.Expr("id"))
		if usageLock.Error != nil {
			return usageLock.Error
		}
		if usageLock.RowsAffected != 1 {
			return errors.New("recovery usage event is absent")
		}
		delivery, usage, err := loadPilotRecoveryUsage(tx, plan.Evidence)
		if err != nil {
			return err
		}
		if !pilotRecoveryEqual(delivery, plan.Snapshot.Delivery) || !pilotRecoveryEqual(usage, plan.Snapshot.Usage) {
			return errors.New("recovery usage preimage changed")
		}
		usage.StatusCode = http.StatusBadGateway
		usage.Disposition = managedUsageDispositionFailed
		usage.OutcomeCode = managedUsageOutcomeUpstreamError
		if current.PublicState == MediaOperationStateSucceeded {
			usage.StatusCode = http.StatusOK
			usage.Disposition = managedUsageDispositionSucceeded
			usage.OutcomeCode = managedUsageOutcomeSuccess
		}
		if current.PublicState == MediaOperationStateCancelled {
			usage.StatusCode = statusClientClosedRequest
			usage.OutcomeCode = managedUsageOutcomeRequestTimeout
		}
		usage.LatencyMilliseconds = current.TerminalAt.Sub(current.AcceptedAt).Milliseconds()
		update := tx.Model(&managedUsageEventRecord{}).Where("id = ? AND tenant_id = ? AND status_code = ? AND disposition = ? AND outcome_code = ?", pilotRecoveryUsageID, pilotRecoveryTenant, plan.Snapshot.Usage.StatusCode, plan.Snapshot.Usage.Disposition, plan.Snapshot.Usage.OutcomeCode).Select("status_code", "disposition", "outcome_code", "latency_milliseconds").Updates(&usage)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return errors.New("recovery usage compare-and-swap failed")
		}
		now := time.Now().UTC()
		result, _ = json.Marshal(pilotRecoveryReceipt{RecoveryID: plan.Evidence.RecoveryID, OperationID: pilotRecoveryOperation, UsageEventID: pilotRecoveryUsageID, Phase: "finalize", State: current.PublicState, AcceptedAt: current.AcceptedAt, DeadlineAt: current.DeadlineAt, RecordedAt: now})
		update = tx.Model(&mediaPresubmissionRecoveryRecord{}).Where("operation_id = ? AND finalized_at IS NULL", pilotRecoveryOperation).Updates(map[string]any{"final_operation": pilotRecoveryJSON(current), "final_usage": pilotRecoveryJSON(usage), "final_receipt": string(result), "finalized_at": now})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return errors.New("recovery finalization compare-and-swap failed")
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%s exact media recovery: %w", phase, err)
	}
	return result, nil
}

func decodePilotRecovery(input io.Reader, destination any) error {
	if input == nil {
		return errors.New("recovery input is required")
	}
	decoder := json.NewDecoder(io.LimitReader(input, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode recovery input: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("recovery requires one complete JSON object")
	}
	return nil
}
func validatePilotRecoveryEvidence(evidence pilotRecoveryEvidence) error {
	expected, _ := time.Parse(time.RFC3339Nano, pilotRecoveryDelivery)
	if !validIdempotencyKey(evidence.RecoveryID) || evidence.TenantID != pilotRecoveryTenant || evidence.OperationID != pilotRecoveryOperation || evidence.UsageEventID != pilotRecoveryUsageID || !evidence.DeliveryAt.Equal(expected) || evidence.Reason != "heygen_wav_receipt_mime_mismatch" || strings.TrimSpace(evidence.SourceReference) == "" || evidence.NativeSubmissionReached == nil || *evidence.NativeSubmissionReached || evidence.ProviderHistoryComplete == nil || !*evidence.ProviderHistoryComplete || evidence.MatchingNativeVideos == nil || *evidence.MatchingNativeVideos != 0 {
		return errors.New("recovery evidence does not authorize the exact verified pre-submission operation")
	}
	return nil
}
func pilotRecoveryJSON(value any) string { data, _ := json.Marshal(value); return string(data) }
func pilotRecoveryEqual(left, right any) bool {
	return pilotRecoveryJSON(left) == pilotRecoveryJSON(right)
}
func findPilotRecoveryAudit(database *gorm.DB, plan pilotRecoveryPlan) (mediaPresubmissionRecoveryRecord, bool, error) {
	var audit mediaPresubmissionRecoveryRecord
	if !database.Migrator().HasTable(&audit) {
		return audit, false, nil
	}
	err := database.First(&audit, "operation_id = ?", pilotRecoveryOperation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return audit, false, nil
	}
	if err != nil {
		return audit, false, err
	}
	if audit.Plan != pilotRecoveryJSON(plan) {
		return audit, false, errors.New("recovery audit is already bound to another plan")
	}
	return audit, true, nil
}
func pilotRecoveryNoWork(database *gorm.DB) error {
	for _, scope := range []struct {
		model any
		where string
	}{
		{&mediaOperationClaimRecord{}, "operation_id = ?"}, {&mediaOperationPartialReferenceRecord{}, "operation_id = ?"},
		{&managedJournalRequestRecord{}, "id = ? OR execution_id = ?"}, {&managedJournalAttemptRecord{}, "request_id = ?"}, {&managedFundsReservationRecord{}, "request_id = ?"},
	} {
		var count int64
		arguments := []any{pilotRecoveryOperation}
		if strings.Contains(scope.where, " OR ") {
			arguments = append(arguments, pilotRecoveryOperation)
		}
		if err := database.Model(scope.model).Where(scope.where, arguments...).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return errors.New("recovery target has claims, previews, or hosted financial work")
		}
	}
	return nil
}
func loadPilotRecoveryUsage(database *gorm.DB, evidence pilotRecoveryEvidence) (mediaOperationUsageDeliveryRecord, managedUsageEventRecord, error) {
	var delivery mediaOperationUsageDeliveryRecord
	var usage managedUsageEventRecord
	if err := database.First(&delivery, "operation_id = ? AND tenant_id = ?", pilotRecoveryOperation, pilotRecoveryTenant).Error; err != nil {
		return delivery, usage, err
	}
	if delivery.DeliveredAt == nil || !delivery.CreatedAt.Equal(evidence.DeliveryAt) || !delivery.DeliveredAt.Equal(evidence.DeliveryAt) {
		return delivery, usage, errors.New("recovery delivery timestamp differs")
	}
	var deliveries []mediaOperationUsageDeliveryRecord
	if err := database.Where("tenant_id = ?", pilotRecoveryTenant).Find(&deliveries).Error; err != nil {
		return delivery, usage, err
	}
	matches := 0
	for _, candidate := range deliveries {
		if candidate.CreatedAt.Equal(evidence.DeliveryAt) {
			matches++
		}
	}
	if matches != 1 {
		return delivery, usage, errors.New("recovery delivery timestamp is ambiguous")
	}
	var events []managedUsageEventRecord
	if err := database.Where("tenant_id = ? AND endpoint = ? AND provider_id = ? AND model_id = ?", pilotRecoveryTenant, usageEndpointMedia, "heygen", "").Find(&events).Error; err != nil {
		return delivery, usage, err
	}
	matches = 0
	for _, candidate := range events {
		if candidate.CreatedAt.Equal(evidence.DeliveryAt) {
			matches++
			usage = candidate
		}
	}
	if matches != 1 || usage.ID != pilotRecoveryUsageID {
		return delivery, usage, errors.New("recovery usage event binding is ambiguous or absent")
	}
	return delivery, usage, nil
}
func loadPilotRecoverySnapshot(ctx context.Context, database *gorm.DB, configuration Configuration, evidence pilotRecoveryEvidence) (pilotRecoverySnapshot, error) {
	var snapshot pilotRecoverySnapshot
	if err := database.First(&snapshot.Operation, "operation_id = ? AND tenant_id = ?", pilotRecoveryOperation, pilotRecoveryTenant).Error; err != nil {
		return snapshot, err
	}
	record := snapshot.Operation
	if record.Capability != llmproxycontract.MediaCapabilityAvatarVideoGenerate || record.Provider != "heygen" || record.Model != "" || record.PublicState != MediaOperationStateUncertain || record.ProviderExecutionState != MediaProviderExecutionUncertain || record.ProviderHandle != "" || record.CancellationState != MediaCancellationNotRequested {
		return snapshot, errors.New("recovery target is not the verified uncertain pre-submission operation")
	}
	if err := pilotRecoveryNoWork(database); err != nil {
		return snapshot, err
	}
	if err := database.Where("operation_id = ?", pilotRecoveryOperation).Order("role, ordinal").Find(&snapshot.References).Error; err != nil {
		return snapshot, err
	}
	if len(snapshot.References) != 1 || snapshot.References[0].Role != "input" || snapshot.References[0].Ordinal != 0 || !snapshot.References[0].Active {
		return snapshot, errors.New("recovery requires only its active original input")
	}
	var input llmproxycontract.AvatarVideoSource
	if err := json.Unmarshal(record.NormalizedInput, &input); err != nil {
		return snapshot, err
	}
	reference := snapshot.References[0]
	if !assetIdentifierPattern.MatchString(input.AudioAssetID) || reference.AssetID != input.AudioAssetID || reference.TenantID != pilotRecoveryTenant || reference.MIMEType != "audio/wav" || reference.SizeBytes <= 0 {
		return snapshot, errors.New("recovery original input identity differs")
	}
	var err error
	snapshot.CredentialReference, err = (&mediaOperationStore{database: database}).credentialReference(ctx, pilotRecoveryTenant, providerID("heygen"))
	if err != nil {
		return snapshot, err
	}
	if snapshot.CredentialReference != record.CredentialReference {
		return snapshot, errors.New("recovery credential authority changed")
	}
	if err := database.First(&snapshot.Avatar, "avatar_id = ? AND tenant_id = ? AND provider = ? AND credential_reference = ?", input.AvatarID, pilotRecoveryTenant, "heygen", record.CredentialReference).Error; err != nil {
		return snapshot, err
	}
	if snapshot.Avatar.NativeLookID == "" {
		return snapshot, errors.New("recovery retained avatar has no native identity")
	}
	catalog, err := NewCatalogService(configuration.ModelCatalog)
	if err != nil {
		return snapshot, err
	}
	route, err := catalog.ResolveService("heygen", ModelOperationAvatarVideoGeneration)
	if err != nil {
		return snapshot, err
	}
	registry := newProviderRegistry(configuration)
	snapshot.CurrentExecutionBinding = providerServiceBinding(route, registry.definitions[providerID("heygen")], CatalogProtocolHeyGenAvatarVideoV3)
	snapshot.CurrentCatalogRevision = catalog.Revision()
	if snapshot.CurrentExecutionBinding != record.ExecutionBinding {
		return snapshot, errors.New("recovery execution route changed")
	}
	store := newTenantAssetStore(configuration.AssetStorePath, configuration.MaxAssetBytes, configuration.AssetRetentionSeconds)
	snapshot.Asset, err = store.readMetadata(input.AudioAssetID)
	if err != nil {
		return snapshot, err
	}
	if snapshot.Asset.TenantID != pilotRecoveryTenant || snapshot.Asset.State != assetStateAvailable || snapshot.Asset.MIMEType != reference.MIMEType || snapshot.Asset.SizeBytes != reference.SizeBytes {
		return snapshot, errors.New("recovery input metadata changed")
	}
	data, err := os.ReadFile(store.dataPath(input.AudioAssetID))
	if err != nil {
		return snapshot, err
	}
	if int64(len(data)) != reference.SizeBytes || mediaSHA256Hex(data) != snapshot.Asset.ContentSHA256 {
		return snapshot, errors.New("recovery input bytes changed")
	}
	snapshot.Delivery, snapshot.Usage, err = loadPilotRecoveryUsage(database, evidence)
	if err != nil {
		return snapshot, err
	}
	if snapshot.Usage.StatusCode != http.StatusBadGateway || snapshot.Usage.Disposition != managedUsageDispositionFailed || snapshot.Usage.OutcomeCode != managedUsageOutcomeUpstreamError {
		return snapshot, errors.New("recovery original usage outcome changed")
	}
	return snapshot, nil
}
func pilotRecoveryImmutableOperation(before, after mediaOperationRecord) bool {
	before.PublicState = after.PublicState
	before.ProviderExecutionState = after.ProviderExecutionState
	before.ProviderHandle = after.ProviderHandle
	before.PublicErrorCode = after.PublicErrorCode
	before.CancellationState = after.CancellationState
	before.UpdatedAt = after.UpdatedAt
	before.TerminalAt = after.TerminalAt
	return pilotRecoveryEqual(before, after)
}
