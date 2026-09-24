package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var errHostedResultExpired = errors.New(llmproxycontract.ErrorCodeHostedResultExpired)

func hostedRequestErrorCode(err error) (string, bool) {
	for _, failure := range []error{errUsageJournalConflict, errJournalClaimLost, errHostedAuthorityDenied, errHostedResultExpired, errInsufficientFunds, errFinancialAdmissionUnavailable} {
		if errors.Is(err, failure) {
			return failure.Error(), true
		}
	}
	return "", false
}

type hostedTextIdentityContextKey struct{}

type hostedTextIdentity struct {
	tenant      tenant
	key         string
	executionID string
}

func hostedTextKeyValues(values []string) (string, error) {
	if len(values) != 1 || !validIdempotencyKey(values[0]) {
		return "", errUsageJournalInvalid
	}
	return values[0], nil
}

type hostedTextRequestDependencies struct {
	database        *gormManagedTenantDatabase
	responses       *structuredRequestStore
	cipher          managedProviderKeyCipher
	catalogRevision string
	authorize       func(*gorm.DB, managedJournalRequestRecord, hostedCompletionIntent) error
	now             func() time.Time
	entropy         io.Reader
}

type hostedTextRequests struct{ hostedTextRequestDependencies }

// Financial admission is supplied by the funds owner. The same executor and
// response store serve hosted work; the journal owns cross-instance dispatch.
func newHostedTextRequests(ctx context.Context, dependencies hostedTextRequestDependencies) (*hostedTextRequests, error) {
	startupContext, cancel := context.WithTimeout(ctx, journalPersistenceTimeout)
	defer cancel()
	for _, kind := range []journalExecutionKind{journalExecutionText, journalExecutionDictation} {
		if err := dependencies.database.recoverJournalDispatches(startupContext, kind, dependencies.now().UTC()); err != nil {
			return nil, fmt.Errorf("recover hosted completion execution: %w", err)
		}
	}
	service := &hostedTextRequests{hostedTextRequestDependencies: dependencies}
	if err := service.recoverResults(startupContext); err != nil {
		return nil, fmt.Errorf("recover hosted text results: %w", err)
	}
	if err := dependencies.database.reconcileHostedFunds(startupContext, dependencies.now().UTC()); err != nil {
		return nil, fmt.Errorf("recover hosted funds: %w", err)
	}
	dependencies.responses.hosted = service
	return service, nil
}

type hostedRequestReplay struct{ record structuredRequestRecord }

func (replay *hostedRequestReplay) Error() string { return "hosted_request_" + replay.record.State }

type hostedStoredCompletion struct {
	Text      string         `json:"text"`
	ToolCalls []functionCall `json:"tool_calls,omitempty"`
	Usage     *tokenUsage    `json:"usage,omitempty"`
}

func decodeHostedStoredCompletion(encoded []byte) (hostedStoredCompletion, error) {
	var input struct {
		Text      *string        `json:"text"`
		ToolCalls []functionCall `json:"tool_calls,omitempty"`
		Usage     *tokenUsage    `json:"usage,omitempty"`
	}
	if err := decodeStrictJSON(encoded, &input); err != nil {
		return hostedStoredCompletion{}, err
	}
	if input.Text == nil {
		return hostedStoredCompletion{}, errors.New("stored completion requires a text string")
	}
	return hostedStoredCompletion{Text: *input.Text, ToolCalls: input.ToolCalls, Usage: input.Usage}, nil
}

func (service *hostedTextRequests) execute(ctx context.Context, router *providerRouter, request chatRequestParameters, identity hostedTextIdentity, logger *zap.SugaredLogger) (completionResult, error) {
	canonical, err := hostedTextIntent(request)
	if err != nil {
		return completionResult{}, err
	}
	intent := hostedCompletionIntent{provider: request.provider, model: request.model.identifier, operation: ModelOperationText,
		kind: journalExecutionText, canonical: canonical, endpoint: endpointKindText, webSearch: request.webSearchEnabled, maxTokens: request.maxTokens}
	return service.executeCompletion(ctx, intent, identity, func(ctx context.Context, provider providerDefinition) (completionResult, error) {
		request.provider = provider
		return router.generateText(ctx, request, logger)
	}, func(text string) completionContent {
		if request.structuredOutput != nil {
			return completedStructuredData(text)
		}
		return completedText(text)
	})
}

type hostedCompletionIntent struct {
	provider  providerDefinition
	model     modelID
	operation string
	kind      journalExecutionKind
	canonical []byte
	endpoint  endpointKind
	webSearch bool
	maxTokens *int
}

func (service *hostedTextRequests) executeCompletion(ctx context.Context, request hostedCompletionIntent, identity hostedTextIdentity, run func(context.Context, providerDefinition) (completionResult, error), restore func(string) completionContent) (completionResult, error) {
	deadline, bounded := ctx.Deadline()
	if !bounded {
		return completionResult{}, errUsageJournalInvalid
	}
	owner, err := newHostedResourceID("worker-", service.entropy)
	if err != nil {
		return completionResult{}, err
	}
	intent, err := newJournalAdmissionIntent(journalAdmissionInput{
		OwnerUserID: identity.tenant.userID, TenantID: identity.tenant.identifier.string(), Provider: request.provider.identifier.string(), Model: request.model.string(), Operation: request.operation,
		CatalogRevision: service.catalogRevision, IdempotencyKey: identity.key, CanonicalIntent: request.canonical, ExecutionKind: request.kind, ExecutionID: identity.executionID,
		OwnerToken: owner, Now: service.now(), ClaimExpiresAt: service.now().Add(time.Until(deadline) + journalPersistenceTimeout),
	}, service.entropy)
	if err != nil {
		return completionResult{}, err
	}
	authorize := func(transaction *gorm.DB, record managedJournalRequestRecord) error {
		return service.authorize(transaction, record, request)
	}
	accepted, err := service.database.admitJournalRequest(ctx, intent, authorize)
	if err != nil {
		return completionResult{}, err
	}
	if accepted.OwnerToken != intent.record.OwnerToken {
		if accepted.State == journalRequestCompleted && accepted.ResultPublishedAt == nil && !accepted.ClaimExpiresAt.After(service.now().UTC()) {
			if err := service.recoverResult(ctx, accepted.ID, service.now().UTC()); err != nil {
				return completionResult{}, err
			}
			accepted, err = service.database.journalRequest(ctx, accepted.BillingAccountID, accepted.ID)
			if err != nil {
				return completionResult{}, err
			}
		}
		return service.replay(accepted, restore)
	}
	execution := newHostedTextExecution(service.database, accepted, authorize, service.now, service.entropy)
	execution.webSearch = request.webSearch
	ctx = contextWithHostedTextExecution(ctx, execution)
	provider, err := service.pinnedProvider(ctx, accepted, request.provider, request.endpoint)
	if err != nil {
		return completionResult{}, errors.Join(err, execution.finish(ctx, err))
	}
	if err := service.publishResponse(ctx, accepted, func(current managedJournalRequestRecord) error {
		_, err := service.responses.beginAdmitted(identity.tenant, identity.key, current)
		return err
	}); err != nil {
		return completionResult{}, errors.Join(err, execution.finish(ctx, err))
	}
	completion, executionError := run(ctx, provider)
	if executionError != nil {
		// The journal is authoritative if the response store is unavailable.
		status := statusCodeForError(executionError)
		persistError := service.publishResponse(ctx, accepted, func(current managedJournalRequestRecord) error {
			// A failed terminal journal write leaves execution pending. Its
			// response cannot claim failure before journal recovery decides it.
			if current.State == journalRequestAccepted || current.State == journalRequestExecuting {
				return nil
			}
			if current.State == journalRequestUncertain {
				return service.responses.uncertain(identity.tenant, identity.key, accepted.IntentDigest)
			}
			return service.responses.fail(identity.tenant, identity.key, accepted.IntentDigest, status, structuredRequestFailureCause(status))
		})
		return completion, errors.Join(executionError, persistError)
	}
	completion.receipt = &completionReceipt{executionID: accepted.ExecutionID, createdAt: accepted.CreatedAt}
	result, err := json.Marshal(hostedStoredCompletion{Text: completion.content.text(), ToolCalls: completion.content.toolCalls(), Usage: completion.usage})
	if err != nil {
		return completion, fmt.Errorf("encode result for request %s: %w", accepted.ID, err)
	}
	if err := service.publishResponse(ctx, accepted, func(managedJournalRequestRecord) error {
		return service.responses.succeed(identity.tenant, identity.key, accepted.IntentDigest, string(result))
	}); err != nil {
		return completion, fmt.Errorf("save result for request %s: %w", accepted.ID, err)
	}
	return completion, nil
}

func (service *hostedTextRequests) replay(accepted managedJournalRequestRecord, restore func(string) completionContent) (completionResult, error) {
	// A failed/uncertain journal receipt must never be reset by response expiry or
	// the ordinary structured-request retry policy.
	if accepted.State == journalRequestUncertain {
		return completionResult{}, &hostedRequestReplay{record: hostedRequestStatusRecord(accepted, structuredRequestStateUncertain)}
	}
	if accepted.State == journalRequestFailed {
		return completionResult{}, &hostedRequestReplay{record: hostedRequestStatusRecord(accepted, structuredRequestStateFailed)}
	}
	if accepted.State != journalRequestCompleted {
		return completionResult{}, &hostedRequestReplay{record: hostedRequestStatusRecord(accepted, structuredRequestStateDispatched)}
	}
	record, err := service.responses.lookupHosted(accepted)
	if errors.Is(err, errStructuredRequestNotFound) {
		if accepted.ResultPublishedAt == nil {
			return completionResult{}, &hostedRequestReplay{record: hostedRequestStatusRecord(accepted, structuredRequestStateDispatched)}
		}
		return completionResult{}, errHostedResultExpired
	}
	if err != nil {
		return completionResult{}, err
	}
	if record.State != structuredRequestStateSucceeded {
		return completionResult{}, &hostedRequestReplay{record: record}
	}
	stored, err := decodeHostedStoredCompletion(record.Result)
	if err != nil {
		return completionResult{}, fmt.Errorf("decode result for request %s: %w", accepted.ID, err)
	}
	content := restore(stored.Text)
	if len(stored.ToolCalls) > 0 {
		content = completedFunctionCalls{visibleText: stored.Text, calls: stored.ToolCalls}
	}
	return completionResult{content: content, usage: stored.Usage, receipt: &completionReceipt{executionID: accepted.ExecutionID, createdAt: accepted.CreatedAt}}, nil
}

func hostedRequestStatusRecord(request managedJournalRequestRecord, state string) structuredRequestRecord {
	return structuredRequestRecord{ProxyRequestID: request.ExecutionID, Provider: request.Provider, Model: request.Model, State: state,
		StartedAt: request.CreatedAt.Format(time.RFC3339Nano), UpdatedAt: request.UpdatedAt.Format(time.RFC3339Nano), StatusCode: http.StatusBadGateway, FailureCode: request.FailureCode}
}

func (service *hostedTextRequests) pinnedProvider(ctx context.Context, accepted managedJournalRequestRecord, provider providerDefinition, endpoint endpointKind) (providerDefinition, error) {
	provider, err := loadJournalProvider(ctx, service.database.database, service.cipher, accepted, provider)
	if err != nil {
		return providerDefinition{}, err
	}
	resolved, ok := provider.resolvedTransport(provider.activeTransport.identifier)
	if !ok || resolved.credentialFor(endpoint) == "" || resolved.textEndpointURL == "" {
		return providerDefinition{}, errHostedAuthorityDenied
	}
	return resolved, nil
}

func hostedTextIntent(request chatRequestParameters) ([]byte, error) {
	type attachment struct {
		Type   messageMediaType `json:"type"`
		MIME   string           `json:"mime_type"`
		Digest string           `json:"sha256"`
		Size   int64            `json:"size"`
	}
	type message struct {
		Role        chatRole       `json:"role"`
		Content     string         `json:"content"`
		ToolCalls   []functionCall `json:"tool_calls,omitempty"`
		ToolCallID  string         `json:"tool_call_id,omitempty"`
		Attachments []attachment   `json:"attachments,omitempty"`
	}
	type toolContract struct {
		Declarations []functionDeclaration `json:"declarations"`
		Selection    toolSelection         `json:"selection"`
		Parallel     *bool                 `json:"parallel"`
	}
	intent := struct {
		Messages  []message       `json:"messages"`
		WebSearch bool            `json:"web_search"`
		MaxTokens *int            `json:"max_tokens"`
		Reasoning string          `json:"reasoning_effort"`
		Schema    json.RawMessage `json:"schema,omitempty"`
		Tools     *toolContract   `json:"tools,omitempty"`
	}{WebSearch: request.webSearchEnabled, MaxTokens: request.maxTokens, Reasoning: request.reasoningEffort}
	for _, value := range request.messages {
		item := message{Role: value.role, Content: value.content, ToolCalls: value.toolCalls, ToolCallID: value.toolCallID}
		for _, media := range value.attachments {
			item.Attachments = append(item.Attachments, attachment{media.mediaType, media.mimeType, media.contentSHA256, media.sizeBytes})
		}
		intent.Messages = append(intent.Messages, item)
	}
	if request.structuredOutput != nil {
		intent.Schema = request.structuredOutput.canonical
	}
	if request.tools != nil {
		intent.Tools = &toolContract{request.tools.declarations, request.tools.selection, request.tools.parallel}
	}
	encoded, err := json.Marshal(intent)
	if err != nil {
		return nil, fmt.Errorf("encode hosted text intent: %w", err)
	}
	return canonicalJSON(encoded)
}
