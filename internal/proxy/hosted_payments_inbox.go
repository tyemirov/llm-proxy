package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/utils/billing"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const (
	paddlePaymentEventsPath      = "/api/payments/paddle/events"
	paymentEnvironmentSandbox    = "sandbox"
	paymentEnvironmentProduction = "production"
	paymentInboxPending          = "pending"
	paymentWebhookMaximumBytes   = 1 << 20
)

var (
	errPaymentEventInvalid  = errors.New("payment_event_invalid")
	errPaymentEventConflict = errors.New("payment_event_conflict")
	paddleEventIDPattern    = regexp.MustCompile(`^evt_[a-z0-9]{26}$`)
	paddleEntityIDPattern   = regexp.MustCompile(`^[a-z]{3}_[a-z0-9]{26}$`)
	paddleEventTypePattern  = regexp.MustCompile(`^[a-z_]+\.[a-z_]+$`)
)

type managedPaymentInboxRecord struct {
	ID                 string    `gorm:"primaryKey"`
	Environment        string    `gorm:"not null;uniqueIndex:payment_inbox_identity,priority:1;check:environment IN ('sandbox','production')"`
	ProcessorAccountID string    `gorm:"not null;uniqueIndex:payment_inbox_identity,priority:2"`
	EventID            string    `gorm:"not null;uniqueIndex:payment_inbox_identity,priority:3"`
	EventType          string    `gorm:"not null"`
	EntityID           string    `gorm:"not null;index"`
	OccurredAt         time.Time `gorm:"not null"`
	Payload            string    `gorm:"not null"`
	ContentDigest      string    `gorm:"not null"`
	State              string    `gorm:"not null;index"`
	Reason             string    `gorm:"not null"`
	RetryAt            time.Time `gorm:"not null;index"`
	ReceivedAt         time.Time `gorm:"not null"`
}

func initializeHostedPaymentsSchema(database *gorm.DB) error {
	if err := initializeFundingOrdersSchema(database); err != nil {
		return err
	}
	models := []any{&managedPaymentInboxRecord{}, &managedPaymentReceiptRecord{}, &managedPaymentAdjustmentRecord{}, &managedPaymentAdjustmentRevisionRecord{}, &managedPaymentEnvironmentRecord{}, &managedPaymentStateObservationRecord{}, &managedPaymentReconciliationRunRecord{}, &managedPaymentReconciliationItemRecord{}, &managedProviderCostEvidenceRecord{}, &managedProviderReconciliationRunRecord{}}
	present := 0
	for _, model := range models {
		if database.Migrator().HasTable(model) {
			present++
		}
	}
	if present == 0 {
		if err := database.AutoMigrate(models...); err != nil {
			return fmt.Errorf("%w: create payment records: %w", errManagedTenantSchemaMigration, err)
		}
		return nil
	}
	if present != len(models) {
		return fmt.Errorf("%w: partial payment records", errManagedTenantSchemaMigration)
	}
	for _, model := range models {
		if err := validateHostedTable(database, model); err != nil {
			return fmt.Errorf("%w: validate payment records: %w", errManagedTenantSchemaMigration, err)
		}
	}
	return nil
}

type paddlePaymentInbox struct {
	database    *gormManagedTenantDatabase
	environment string
	accountID   string
	verifier    *billing.PaddleWebhookVerifier
	now         func() time.Time
}

func newPaddlePaymentInbox(database *gormManagedTenantDatabase, environment, accountID, secret string, tolerance time.Duration) (*paddlePaymentInbox, error) {
	if database == nil || (environment != paymentEnvironmentSandbox && environment != paymentEnvironmentProduction) || !validIdempotencyKey(accountID) {
		return nil, fmt.Errorf("invalid Paddle payment inbox configuration")
	}
	verifier, err := billing.NewPaddleWebhookVerifier(secret, tolerance)
	if err != nil {
		return nil, fmt.Errorf("configure Paddle event verification: %w", err)
	}
	return &paddlePaymentInbox{database: database, environment: environment, accountID: accountID, verifier: verifier, now: time.Now}, nil
}

func (inbox *paddlePaymentInbox) decodeEvent(payload []byte) (managedPaymentInboxRecord, error) {
	var envelope struct {
		billing.PaddleWebhookEnvelope
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return managedPaymentInboxRecord{}, errPaymentEventInvalid
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, envelope.OccurredAt)
	if err != nil || occurredAt.IsZero() || !paddleEventIDPattern.MatchString(envelope.EventID) || !paddleEventTypePattern.MatchString(envelope.EventType) || len(envelope.EventType) > 128 {
		return managedPaymentInboxRecord{}, errPaymentEventInvalid
	}
	var data map[string]any
	decoder := json.NewDecoder(bytes.NewReader(envelope.Data))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil || data == nil {
		return managedPaymentInboxRecord{}, errPaymentEventInvalid
	}
	entityID, ok := data["id"].(string)
	if !ok || !paddleEntityIDPattern.MatchString(entityID) {
		return managedPaymentInboxRecord{}, errPaymentEventInvalid
	}
	// Notifications can differ while their event is identical. Canonical JSON
	// preserves exact numbers and makes property order irrelevant to replay.
	canonical, err := json.Marshal(struct {
		ID         string         `json:"event_id"`
		Type       string         `json:"event_type"`
		OccurredAt time.Time      `json:"occurred_at"`
		Data       map[string]any `json:"data"`
	}{envelope.EventID, envelope.EventType, occurredAt.UTC(), data})
	if err != nil {
		return managedPaymentInboxRecord{}, fmt.Errorf("canonicalize Paddle event: %w", err)
	}
	return managedPaymentInboxRecord{
		ID:          "payment-event-" + sha256Hex(inbox.environment + "\x00" + inbox.accountID + "\x00" + envelope.EventID)[:32],
		Environment: inbox.environment, ProcessorAccountID: inbox.accountID, EventID: envelope.EventID, EventType: envelope.EventType,
		EntityID: entityID, OccurredAt: occurredAt.UTC(), Payload: string(payload), ContentDigest: sha256Hex(string(canonical)), State: paymentInboxPending, ReceivedAt: inbox.now().UTC(), RetryAt: inbox.now().UTC(),
	}, nil
}

func (database *gormManagedTenantDatabase) retainPaymentEvent(ctx context.Context, record managedPaymentInboxRecord) error {
	privateQuery := database.database.Session(&gorm.Session{Logger: paymentInboxLogger{database.database.Logger}})
	return privateQuery.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
		if result.Error != nil {
			return fmt.Errorf("retain payment event %s: %w", record.EventID, result.Error)
		}
		if result.RowsAffected == 1 {
			return nil
		}
		var retained managedPaymentInboxRecord
		if err := tx.Where("id = ?", record.ID).First(&retained).Error; err != nil {
			return fmt.Errorf("read retained payment event %s: %w", record.EventID, err)
		}
		if retained.ContentDigest != record.ContentDigest {
			return errPaymentEventConflict
		}
		return nil
	})
}

// Keep database error diagnostics without interpolating private payment values.
type paymentInboxLogger struct{ logger.Interface }

func (paymentInboxLogger) ParamsFilter(_ context.Context, query string, _ ...any) (string, []any) {
	return query, nil
}

func registerPaddlePaymentRoutes(router *gin.Engine, inbox *paddlePaymentInbox) {
	router.POST(paddlePaymentEventsPath, func(ctx *gin.Context) {
		ctx.Header("Cache-Control", "no-store")
		mediaType, _, err := mime.ParseMediaType(ctx.GetHeader("Content-Type"))
		if err != nil || mediaType != "application/json" {
			writePaymentInboxError(ctx, http.StatusUnsupportedMediaType, "payment_content_type_invalid")
			return
		}
		if ctx.Request.URL.RawQuery != "" || (ctx.GetHeader("Content-Encoding") != "" && ctx.GetHeader("Content-Encoding") != "identity") {
			writePaymentInboxError(ctx, http.StatusBadRequest, errPaymentEventInvalid.Error())
			return
		}
		payload, err := io.ReadAll(http.MaxBytesReader(ctx.Writer, ctx.Request.Body, paymentWebhookMaximumBytes))
		if err != nil {
			var limit *http.MaxBytesError
			if errors.As(err, &limit) {
				writePaymentInboxError(ctx, http.StatusRequestEntityTooLarge, "payment_event_too_large")
			} else {
				writePaymentInboxError(ctx, http.StatusBadRequest, errPaymentEventInvalid.Error())
			}
			return
		}
		headers := ctx.Request.Header.Values(billing.PaddleWebhookSignatureHeaderName)
		if len(headers) != 1 || inbox.verifier.Verify(headers[0], payload) != nil {
			writePaymentInboxError(ctx, http.StatusUnauthorized, "payment_signature_invalid")
			return
		}
		record, err := inbox.decodeEvent(payload)
		if err != nil {
			writePaymentInboxError(ctx, http.StatusBadRequest, errPaymentEventInvalid.Error())
			return
		}
		if err := inbox.database.retainPaymentEvent(ctx.Request.Context(), record); err != nil {
			if errors.Is(err, errPaymentEventConflict) {
				writePaymentInboxError(ctx, http.StatusConflict, errPaymentEventConflict.Error())
			} else {
				writePaymentInboxError(ctx, http.StatusServiceUnavailable, "payment_inbox_unavailable")
			}
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"event_id": record.EventID, "status": "accepted"})
	})
}

func writePaymentInboxError(ctx *gin.Context, status int, code string) {
	ctx.JSON(status, gin.H{"error": gin.H{"code": code}})
}
