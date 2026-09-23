package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type hostedTextExecutionContextKey struct{}
type hostedProviderRoleContextKey struct{}
type hostedProviderRole uint8

const (
	hostedProviderGeneration hostedProviderRole = iota + 1
	hostedProviderObservation
	hostedProviderAuxiliary
	hostedProviderStaging
	hostedProviderCancellation
	hostedProviderMetadata
)

const journalPersistenceTimeout = 10 * time.Second

// One admitted execution owns all of its provider attempts, including protocol
// synthesis calls and HTTP retries below the visible continuation loop.
type hostedTextExecution struct {
	mutex         sync.Mutex
	database      *gormManagedTenantDatabase
	request       managedJournalRequestRecord
	authorize     journalReservation
	now           func() time.Time
	entropy       io.Reader
	activeAttempt string
	webSearch     bool
}

func newHostedTextExecution(database *gormManagedTenantDatabase, request managedJournalRequestRecord, authorize journalReservation, now func() time.Time, entropy io.Reader) *hostedTextExecution {
	return &hostedTextExecution{database: database, request: request, authorize: authorize, now: now, entropy: entropy}
}

func contextWithHostedTextExecution(ctx context.Context, execution *hostedTextExecution) context.Context {
	return context.WithValue(ctx, hostedTextExecutionContextKey{}, execution)
}

func hostedTextExecutionFromContext(ctx context.Context) *hostedTextExecution {
	execution, _ := ctx.Value(hostedTextExecutionContextKey{}).(*hostedTextExecution)
	return execution
}

func contextWithHostedProviderRole(ctx context.Context, role hostedProviderRole) context.Context {
	if hostedTextExecutionFromContext(ctx) == nil {
		return ctx
	}
	return context.WithValue(ctx, hostedProviderRoleContextKey{}, role)
}

func (execution *hostedTextExecution) claim() journalWorkerClaim {
	return journalWorkerClaim{requestID: execution.request.ID, owner: execution.request.OwnerToken, now: execution.now().UTC()}
}

func (execution *hostedTextExecution) do(next HTTPDoer, request *http.Request, transport providerTransportDefinition) (*http.Response, error) {
	execution.mutex.Lock()
	defer execution.mutex.Unlock()
	role, _ := request.Context().Value(hostedProviderRoleContextKey{}).(hostedProviderRole)
	switch role {
	case hostedProviderAuxiliary, hostedProviderStaging:
		if (role == hostedProviderStaging && request.Method != http.MethodPost) ||
			(role == hostedProviderAuxiliary && request.Method != http.MethodGet && request.Method != http.MethodDelete) {
			return nil, errHostedAuthorityDenied
		}
		if err := execution.authorizeTransfer(request.Context(), role); err != nil {
			return nil, err
		}
		return next.Do(request)
	case hostedProviderGeneration:
		if request.Method != http.MethodPost {
			return nil, errHostedAuthorityDenied
		}
	case hostedProviderObservation:
		if request.Method != http.MethodGet || execution.activeAttempt == "" {
			return nil, errHostedAuthorityDenied
		}
		if err := execution.authorizeTransfer(request.Context(), role); err != nil {
			return nil, err
		}
	default:
		return nil, errHostedAuthorityDenied
	}
	profile, err := newCompletionJournalMeter(transport.responseCodec, execution.request.Operation)
	if err != nil {
		return nil, err
	}
	// Cleanup and read-only polling do not create another paid generation attempt.
	if role == hostedProviderGeneration {
		identifier, err := newHostedResourceID(journalAttemptIDPrefix, execution.entropy)
		if err != nil {
			return nil, err
		}
		attempt, err := execution.database.prepareJournalAttempt(request.Context(), execution.claim(), identifier, execution.authorize)
		if err != nil {
			return nil, err
		}
		if err := execution.database.dispatchJournalAttempt(request.Context(), execution.claim(), attempt.ID); err != nil {
			return nil, err
		}
		execution.activeAttempt = attempt.ID
	}
	response, err := next.Do(request)
	if err != nil {
		return response, err
	}
	body, readError := io.ReadAll(response.Body)
	closeError := response.Body.Close()
	if err := errors.Join(readError, closeError); err != nil {
		return nil, fmt.Errorf("read hosted provider evidence: %w", err)
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	observation, terminal := profile.observe(body, response.StatusCode, execution.activeAttempt, execution.now())
	persistContext, cancel := context.WithTimeout(context.WithoutCancel(request.Context()), journalPersistenceTimeout)
	defer cancel()
	if observation.ProviderRequestID != "" {
		if err := execution.database.bindJournalProviderRequest(persistContext, execution.claim(), execution.activeAttempt, observation.ProviderRequestID); err != nil {
			return nil, err
		}
	}
	if !terminal {
		return response, nil
	}
	if execution.webSearch {
		observation.recordWebSearchUsage(body, transport.responseCodec)
	}
	evidence, err := newJournalUsageEvidence(observation, execution.entropy)
	if err != nil {
		return nil, fmt.Errorf("normalize provider usage for attempt %s: %w", execution.activeAttempt, err)
	}
	if _, err := execution.database.observeJournalAttempt(persistContext, execution.claim(), evidence); err != nil {
		return nil, err
	}
	return response, nil
}

func (execution *hostedTextExecution) authorizeTransfer(ctx context.Context, role hostedProviderRole) error {
	return execution.database.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		request, err := lockJournalClaim(transaction, execution.claim())
		if err != nil {
			return err
		}
		if role == hostedProviderStaging {
			return requireJournalAuthority(transaction, request)
		}
		return nil
	})
}

func (execution *hostedTextExecution) finish(ctx context.Context, executionError error) error {
	execution.mutex.Lock()
	defer execution.mutex.Unlock()
	persistContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), journalPersistenceTimeout)
	defer cancel()
	state, failure := journalRequestCompleted, ""
	if executionError != nil {
		state, failure = journalRequestFailed, "provider_execution_failed"
	}
	return execution.database.finishJournalText(persistContext, execution.claim(), state, failure)
}

func (database *gormManagedTenantDatabase) bindJournalProviderRequest(ctx context.Context, claim journalWorkerClaim, attemptID, providerRequestID string) error {
	return database.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if _, err := lockJournalClaim(transaction, claim); err != nil {
			return err
		}
		result := transaction.Model(&managedJournalAttemptRecord{}).Where("id = ? AND request_id = ? AND state IN ? AND (provider_request_id = '' OR provider_request_id = ?)", attemptID, claim.requestID, []journalAttemptState{journalAttemptDispatched, journalAttemptObserved}, providerRequestID).
			Updates(map[string]any{"provider_request_id": providerRequestID, "updated_at": claim.now})
		if result.Error != nil {
			return fmt.Errorf("bind provider identity for attempt %s: %w", attemptID, result.Error)
		}
		if result.RowsAffected != 1 {
			return errUsageJournalConflict
		}
		return nil
	})
}

func (database *gormManagedTenantDatabase) finishJournalText(ctx context.Context, claim journalWorkerClaim, state journalRequestState, failure string) error {
	return database.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		request, err := lockJournalClaim(transaction, claim)
		if err != nil {
			return err
		}
		var outstanding int64
		if err := transaction.Model(&managedJournalAttemptRecord{}).Where("request_id = ? AND state = ?", request.ID, journalAttemptDispatched).Count(&outstanding).Error; err != nil {
			return fmt.Errorf("read unfinished attempts for request %s: %w", request.ID, err)
		}
		usage := request.UsageState
		if outstanding > 0 {
			state, usage = journalRequestUncertain, journalUsageUnknown
			if err := transaction.Model(&managedJournalAttemptRecord{}).Where("request_id = ? AND state = ?", request.ID, journalAttemptDispatched).
				Updates(map[string]any{"state": journalAttemptUncertain, "updated_at": claim.now}).Error; err != nil {
				return fmt.Errorf("retain unfinished attempts for request %s: %w", request.ID, err)
			}
			reconciliation := managedJournalCaseRecord{ID: journalCaseIDPrefix + sha256Hex(request.ID + ":" + journalCaseDispatchUnknown)[:32], RequestID: request.ID, Reason: journalCaseDispatchUnknown, CreatedAt: claim.now}
			if err := transaction.Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&reconciliation).Error; err != nil {
				return fmt.Errorf("retain unfinished request %s: %w", request.ID, err)
			}
		}
		if err := transaction.Model(&managedJournalRequestRecord{}).Where("id = ?", request.ID).Updates(map[string]any{"state": state, "usage_state": usage, "failure_code": failure, "updated_at": claim.now}).Error; err != nil {
			return fmt.Errorf("complete journal request %s: %w", request.ID, err)
		}
		return nil
	})
}
