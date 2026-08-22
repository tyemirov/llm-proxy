package proxy

import (
	"context"
	"time"
)

const pollableResourcePollInterval = 500 * time.Millisecond

type pollableResourceRetryDecision uint8

const (
	pollableResourceDoNotRetry pollableResourceRetryDecision = iota
	pollableResourceRetryVisibility
)

type pollableResourceLifecycle[snapshot any] struct {
	observe           func(context.Context) (snapshot, error)
	isPending         func(snapshot) bool
	recordObservation func(snapshot, error, pollableResourceRetryDecision)
	visibilityPolicy  pollableResourceVisibilityPolicy
}

type pollableResourceVisibilityPolicy struct {
	retryInterval time.Duration
	retryLimit    int
	statusCodes   []int
}

func pollableResourceVisibilityPolicyFromCatalog(configuration ProviderCatalogResourceVisibility) pollableResourceVisibilityPolicy {
	return pollableResourceVisibilityPolicy{
		retryInterval: time.Duration(configuration.RetryIntervalMilliseconds) * time.Millisecond,
		retryLimit:    configuration.RetryLimit,
		statusCodes:   append([]int(nil), configuration.RetryStatusCodes...),
	}
}

func (lifecycle pollableResourceLifecycle[snapshot]) observeCreated(parentContext context.Context) (snapshot, error) {
	if contextError := parentContext.Err(); contextError != nil {
		var emptySnapshot snapshot
		return emptySnapshot, contextError
	}
	visibilityRetryCount := 0
	for {
		observedSnapshot, observationError := lifecycle.observe(parentContext)
		retryDecision := lifecycle.visibilityPolicy.retryDecision(observationError, visibilityRetryCount)
		lifecycle.recordObservation(observedSnapshot, observationError, retryDecision)
		if retryDecision == pollableResourceDoNotRetry {
			return observedSnapshot, observationError
		}
		if waitError := waitForRequestTelemetryPhase(parentContext, lifecycle.visibilityPolicy.retryInterval, requestTelemetryPhaseProviderPollWait); waitError != nil {
			return observedSnapshot, waitError
		}
		visibilityRetryCount++
	}
}

func (lifecycle pollableResourceLifecycle[snapshot]) observeUntilTerminal(parentContext context.Context) (snapshot, error) {
	observedSnapshot, observationError := lifecycle.observeCreated(parentContext)
	for observationError == nil && lifecycle.isPending(observedSnapshot) {
		if waitError := waitForRequestTelemetryPhase(parentContext, pollableResourcePollInterval, requestTelemetryPhaseProviderPollWait); waitError != nil {
			return observedSnapshot, waitError
		}
		observedSnapshot, observationError = lifecycle.observe(parentContext)
		lifecycle.recordObservation(observedSnapshot, observationError, pollableResourceDoNotRetry)
	}
	return observedSnapshot, observationError
}

func (policy pollableResourceVisibilityPolicy) retryDecision(observationError error, retryCount int) pollableResourceRetryDecision {
	if retryCount >= policy.retryLimit {
		return pollableResourceDoNotRetry
	}
	statusCode, _, _, hasProviderStatus := providerHTTPMetadata(observationError)
	if !hasProviderStatus {
		return pollableResourceDoNotRetry
	}
	for _, retryStatusCode := range policy.statusCodes {
		if statusCode == retryStatusCode {
			return pollableResourceRetryVisibility
		}
	}
	return pollableResourceDoNotRetry
}
