package proxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"

	"github.com/cenkalti/backoff/v4"
)

type qualificationExecutionKey struct{}
type qualificationVerificationKey struct{}
type qualificationTransportKey struct{}
type qualificationVerificationIdentity struct{ owner, provider, name string }
type qualificationTransportIdentity struct {
	provider  string
	scope     upstreamRequestScope
	transport providerTransportDefinition
}
type qualificationExecution struct {
	mutex     sync.Mutex
	id        string
	requestID string
	active    *qualificationBudgetAttempt
	offering  qualificationFrozenOffering
	budget    *qualificationBudget
}
type qualificationBudgetHTTPDoer struct {
	next      HTTPDoer
	qualified HTTPDoer
	budget    *qualificationBudget
}

func qualificationLogicalContext(ctx context.Context) context.Context {
	if _, exists := ctx.Value(qualificationExecutionKey{}).(*qualificationExecution); exists {
		return ctx
	}
	requestID, _ := ctx.Value(requestIdentifierContextKey{}).(string)
	return context.WithValue(ctx, qualificationExecutionKey{}, &qualificationExecution{id: rand.Text(), requestID: requestID})
}
func qualificationVerificationContext(ctx context.Context, owner, provider string, name managedTenantName) context.Context {
	return context.WithValue(qualificationLogicalContext(ctx), qualificationVerificationKey{}, qualificationVerificationIdentity{owner, provider, name.key})
}

func newQualificationSafeHTTPDoer(next HTTPDoer) (HTTPDoer, error) {
	admission, ok := next.(*admissionHTTPDoer)
	if !ok {
		return nil, fmt.Errorf("%w: admission transport required", errQualificationBudget)
	}
	client, ok := admission.next.(*http.Client)
	if !ok {
		return nil, fmt.Errorf("%w: standard HTTP client required", errQualificationBudget)
	}
	safeClient := *client
	var transport *http.Transport
	if client.Transport == nil {
		transport = http.DefaultTransport.(*http.Transport).Clone()
	} else if declared, ok := client.Transport.(*http.Transport); ok {
		transport = declared.Clone()
	} else {
		return nil, fmt.Errorf("%w: standard HTTP transport required", errQualificationBudget)
	}
	transport.DisableKeepAlives = true
	// HTTP/2 can replay refused streams internally. Qualification uses one fresh
	// HTTP/1 connection per request so every paid retry re-enters the journal.
	transport.ForceAttemptHTTP2 = false
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	transport.Protocols = protocols
	safeClient.Transport = transport
	safeClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	guarded := *admission
	guarded.next = &safeClient
	return &guarded, nil
}

func (doer *qualificationBudgetHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	budget := doer.budget
	if budget == nil {
		return doer.next.Do(request)
	}
	identity, present := request.Context().Value(qualificationTransportKey{}).(qualificationTransportIdentity)
	if !present {
		scope, _ := request.Context().Value(upstreamScopeContextKey{}).(upstreamRequestScope)
		if scope.tenant == budget.policy.TenantID && request.Method == http.MethodPost {
			return nil, backoff.Permanent(fmt.Errorf("%w: selected tenant generation lacks qualification attribution", errQualificationBudget))
		}
		return doer.next.Do(request)
	}
	selected := identity.scope.tenant == budget.policy.TenantID
	if verification, ok := request.Context().Value(qualificationVerificationKey{}).(qualificationVerificationIdentity); ok && verification.owner == budget.policy.OwnerUserID {
		selected = selected || budget.verification[verification.provider+"/"+verification.name]
	}
	if !selected {
		return doer.next.Do(request)
	}
	if identity.scope.account == "" {
		return nil, backoff.Permanent(fmt.Errorf("%w: missing connection attribution", errQualificationBudget))
	}
	logical, present := request.Context().Value(qualificationExecutionKey{}).(*qualificationExecution)
	if !present {
		if request.Method == http.MethodGet && string(identity.transport.lifecycle) == CatalogExecutionReadOnly {
			return doer.qualified.Do(request)
		}
		return nil, backoff.Permanent(fmt.Errorf("%w: logical execution required", errQualificationBudget))
	}
	if logical.requestID == "" {
		return nil, backoff.Permanent(fmt.Errorf("%w: originating HTTP request identity required", errQualificationBudget))
	}
	logical.mutex.Lock()
	defer logical.mutex.Unlock()
	logical.budget = budget
	if request.Method == http.MethodDelete {
		return doer.qualified.Do(request)
	}
	if request.Method == http.MethodGet {
		if logical.active == nil {
			return doer.qualified.Do(request)
		}
		if logical.active.ResourceID == "" || !strings.HasSuffix(request.URL.Path, "/"+logical.active.ResourceID) {
			return nil, backoff.Permanent(fmt.Errorf("%w: unmatched observation", errQualificationBudget))
		}
		return doer.observe(request, logical, *logical.active, logical.offering)
	}
	if request.Method != http.MethodPost || identity.transport.lifecycle != textExecutionLifecycleSynchronousCompletion && identity.transport.lifecycle != textExecutionLifecyclePollableResource {
		return nil, backoff.Permanent(fmt.Errorf("%w: unsupported operation", errQualificationBudget))
	}
	if logical.active != nil {
		return nil, backoff.Permanent(fmt.Errorf("%w: unresolved prior generation", errQualificationBudget))
	}
	prepared, offering, err := budget.prepare(request, identity)
	if err != nil {
		return nil, backoff.Permanent(errors.Join(errQualificationBudget, err))
	}
	attempt, err := budget.reserve(logical, offering, identity.scope.account)
	if err != nil {
		return nil, backoff.Permanent(errors.Join(errQualificationBudget, err))
	}
	logical.active = &attempt
	logical.offering = offering
	return doer.observe(prepared, logical, attempt, offering)
}

func (budget *qualificationBudget) prepare(request *http.Request, identity qualificationTransportIdentity) (*http.Request, qualificationFrozenOffering, error) {
	var offering qualificationFrozenOffering
	if request.Body == nil {
		return nil, offering, errQualificationBudget
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, MaxV2RequestBytes+1))
	closeErr := request.Body.Close()
	if err = errors.Join(err, closeErr); err != nil || len(body) > MaxV2RequestBytes {
		return nil, offering, fmt.Errorf("%w: generation body unavailable", errQualificationBudget)
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, offering, errQualificationBudget
	}
	model, _ := payload["model"].(string)
	found := false
	for _, candidate := range budget.frozen {
		if candidate.Provider == identity.provider && candidate.ProviderModel == model && candidate.Codec == identity.transport.responseCodec {
			offering = candidate
			found = true
			break
		}
	}
	if !found {
		return nil, offering, fmt.Errorf("%w: exact priced text offering required", errQualificationBudget)
	}
	if containsQualificationBillableFeature(payload) {
		return nil, offering, fmt.Errorf("%w: tools, caching controls, media, and service overrides are outside qualification", errQualificationBudget)
	}
	outputField := ""
	target := payload
	switch offering.Codec {
	case CatalogProtocolOpenAIResponses, CatalogProtocolDashScopeResponses, CatalogProtocolXAIResponses:
		outputField = "max_output_tokens"
	case CatalogProtocolOpenAIChatCompletions:
		outputField = "max_completion_tokens"
		if identity.transport.protocolParameters.TokenField != CatalogProtocolVariationMaxCompletionTokens {
			outputField = "max_tokens"
		}
	case CatalogProtocolAnthropicMessages:
		outputField = "max_tokens"
	case CatalogProtocolGeminiInteractions:
		outputField = "max_output_tokens"
		configuration, ok := payload["generation_config"].(map[string]any)
		if !ok {
			return nil, offering, fmt.Errorf("%w: serialized generation configuration required", errQualificationBudget)
		}
		target = configuration
	default:
		return nil, offering, fmt.Errorf("%w: unsupported qualification codec", errQualificationBudget)
	}
	number, ok := target[outputField].(json.Number)
	value, err := number.Int64()
	if !ok || err != nil || value < 1 || value > int64(offering.MaxOutput) {
		return nil, offering, fmt.Errorf("%w: explicit wire output allowance required within private maximum", errQualificationBudget)
	}
	encoded, _ := json.Marshal(payload)
	prepared := request.Clone(request.Context())
	prepared.Body = io.NopCloser(bytes.NewReader(encoded))
	prepared.ContentLength = int64(len(encoded))
	prepared.GetBody = nil
	prepared.Close = true
	return prepared, offering, nil
}

func containsQualificationBillableFeature(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "schema" {
				continue
			}
			switch key {
			case "tools", "tool_choice", "service_tier", "cache_control", "cached_content", "response_modalities", "modalities":
				if child != nil {
					return true
				}
			}
			if key == "type" {
				if kind, ok := child.(string); ok && (strings.Contains(kind, "image") || strings.Contains(kind, "audio") || strings.Contains(kind, "video") || kind == "file") {
					return true
				}
			}
			if containsQualificationBillableFeature(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsQualificationBillableFeature(child) {
				return true
			}
		}
	}
	return false
}

func (doer *qualificationBudgetHTTPDoer) observe(request *http.Request, logical *qualificationExecution, attempt qualificationBudgetAttempt, offering qualificationFrozenOffering) (*http.Response, error) {
	response, err := doer.qualified.Do(request)
	if err != nil {
		if request.Method == http.MethodPost && errors.Is(err, errUpstreamNotDispatched) {
			if releaseErr := doer.budget.releaseNotDispatched(attempt); releaseErr != nil {
				return response, backoff.Permanent(errors.Join(errQualificationBudget, err, releaseErr, doer.budget.suspend()))
			}
			logical.active = nil
			return response, backoff.Permanent(err)
		}
		return response, backoff.Permanent(errors.Join(errQualificationBudget, doer.budget.suspend()))
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, MaxV2RequestBytes+1))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil || len(body) > MaxV2RequestBytes {
		return nil, backoff.Permanent(errors.Join(errQualificationBudget, doer.budget.suspend()))
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	evidence, terminal := newTextJournalMeter(offering.Codec).observe(body, response.StatusCode, fmt.Sprint(attempt.ID), doer.budget.now())
	if !terminal {
		if evidence.ProviderRequestID == "" {
			return nil, backoff.Permanent(errors.Join(errQualificationBudget, doer.budget.suspend()))
		}
		attempt.ResourceID = evidence.ProviderRequestID
		if err := doer.budget.database.Model(&qualificationBudgetAttempt{}).Where("id = ?", attempt.ID).Updates(map[string]any{"resource_id": attempt.ResourceID, "state": "pending"}).Error; err != nil {
			return nil, backoff.Permanent(errors.Join(errQualificationBudget, doer.budget.suspend()))
		}
		logical.active = &attempt
		return response, nil
	}
	if err := validateQualificationSuppliedUsageTotal(offering.Codec, body); err != nil {
		return nil, backoff.Permanent(errors.Join(errQualificationBudget, doer.budget.suspend()))
	}
	if err := doer.budget.settle(attempt, offering, evidence); err != nil {
		return nil, backoff.Permanent(errors.Join(errQualificationBudget, doer.budget.suspend()))
	}
	logical.active = nil
	return response, nil
}

func finishQualificationExecution(ctx context.Context) error {
	// Both callers establish this context before installing their finalizer.
	logical := ctx.Value(qualificationExecutionKey{}).(*qualificationExecution)
	logical.mutex.Lock()
	defer logical.mutex.Unlock()
	if logical.active == nil || logical.budget == nil {
		return nil
	}
	markError := logical.budget.database.Model(&qualificationBudgetAttempt{}).Where("id = ?", logical.active.ID).Update("active", false).Error
	return errors.Join(errQualificationBudget, markError, logical.budget.suspend())
}

func validateQualificationSuppliedUsageTotal(codec string, body []byte) error {
	if codec != CatalogProtocolOpenAIResponses && codec != CatalogProtocolOpenAIChatCompletions && codec != CatalogProtocolDashScopeResponses && codec != CatalogProtocolXAIResponses && codec != CatalogProtocolGeminiInteractions {
		return nil
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return errQualificationBudget
	}
	usage, _ := payload["usage"].(map[string]any)
	parse := func(name string) (*big.Int, bool, error) {
		raw, present := usage[name]
		if !present {
			return nil, false, nil
		}
		number, ok := raw.(json.Number)
		if !ok || !journalDecimalPattern.MatchString(number.String()) || strings.Contains(number.String(), ".") {
			return nil, true, errQualificationBudget
		}
		value, _ := new(big.Int).SetString(number.String(), 10)
		return value, true, nil
	}
	inputName, outputName := "input_tokens", "output_tokens"
	if codec == CatalogProtocolOpenAIChatCompletions {
		inputName, outputName = "prompt_tokens", "completion_tokens"
	}
	if codec == CatalogProtocolGeminiInteractions {
		inputName, outputName = "total_input_tokens", "total_output_tokens"
	}
	input, hasInput, inputError := parse(inputName)
	output, hasOutput, outputError := parse(outputName)
	if inputError != nil || outputError != nil || !hasInput || !hasOutput {
		return errQualificationBudget
	}
	expectedTotal := new(big.Int).Add(input, output)
	if codec == CatalogProtocolGeminiInteractions {
		thoughts, present, err := parse("total_thought_tokens")
		if err != nil || !present {
			return errQualificationBudget
		}
		expectedTotal.Add(expectedTotal, thoughts)
		tools, present, err := parse("total_tool_use_tokens")
		if err != nil || present && tools.Sign() != 0 {
			return errQualificationBudget
		}
	} else {
		for _, alias := range []struct {
			name     string
			expected *big.Int
		}{{"input_tokens", input}, {"prompt_tokens", input}, {"output_tokens", output}, {"completion_tokens", output}} {
			value, present, err := parse(alias.name)
			if err != nil || present && value.Cmp(alias.expected) != 0 {
				return errQualificationBudget
			}
		}
	}
	total, present, err := parse("total_tokens")
	if err != nil || present && total.Cmp(expectedTotal) != 0 {
		return errQualificationBudget
	}
	return nil
}
