package proxy

import (
	"context"
	"net/http"
	"time"
)

const (
	pollableResourcePollInterval       = 500 * time.Millisecond
	pollableResourceVisibilityInterval = 2 * time.Second
)

type pollableResourceRetryDecision uint8

const (
	pollableResourceDoNotRetry pollableResourceRetryDecision = iota
	pollableResourceRetryVisibility
)

type pollableResourceLifecycle[snapshot any] struct {
	observe           func(context.Context) (snapshot, error)
	isPending         func(snapshot) bool
	recordObservation func(snapshot, error, pollableResourceRetryDecision)
}

func (lifecycle pollableResourceLifecycle[snapshot]) observeCreated(parentContext context.Context) (snapshot, error) {
	if contextError := parentContext.Err(); contextError != nil {
		var emptySnapshot snapshot
		return emptySnapshot, contextError
	}
	observedSnapshot, observationError := lifecycle.observe(parentContext)
	retryDecision := pollableResourceDoNotRetry
	if pollableResourceVisibilityError(observationError) {
		retryDecision = pollableResourceRetryVisibility
	}
	lifecycle.recordObservation(observedSnapshot, observationError, retryDecision)
	if retryDecision == pollableResourceDoNotRetry {
		return observedSnapshot, observationError
	}
	if waitError := waitForRequestTelemetryPhase(parentContext, pollableResourceVisibilityInterval, requestTelemetryPhaseProviderPollWait); waitError != nil {
		return observedSnapshot, waitError
	}
	observedSnapshot, observationError = lifecycle.observe(parentContext)
	lifecycle.recordObservation(observedSnapshot, observationError, pollableResourceDoNotRetry)
	return observedSnapshot, observationError
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

func pollableResourceVisibilityError(observationError error) bool {
	statusCode, _, _, hasProviderStatus := providerHTTPMetadata(observationError)
	return hasProviderStatus && (statusCode == http.StatusForbidden || statusCode == http.StatusNotFound)
}
