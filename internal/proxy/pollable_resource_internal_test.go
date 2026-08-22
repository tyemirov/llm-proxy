package proxy

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestPollableResourceVisibilityExhaustionReturnsLastProviderError(testingInstance *testing.T) {
	providerErrors := []error{
		newProviderHTTPError(http.StatusForbidden, nil),
		newProviderHTTPError(http.StatusBadRequest, nil),
		newProviderHTTPError(http.StatusNotFound, nil),
	}
	observationCount := 0
	retryDecisions := []pollableResourceRetryDecision{}
	lifecycle := pollableResourceLifecycle[int]{
		observe: func(context.Context) (int, error) {
			observationError := providerErrors[observationCount]
			observationCount++
			return observationCount, observationError
		},
		isPending: func(int) bool { return false },
		recordObservation: func(_ int, _ error, retryDecision pollableResourceRetryDecision) {
			retryDecisions = append(retryDecisions, retryDecision)
		},
		visibilityPolicy: pollableResourceVisibilityPolicy{
			retryInterval: time.Nanosecond,
			retryLimit:    2,
			statusCodes:   []int{http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound},
		},
	}

	_, observationError := lifecycle.observeCreated(context.Background())
	statusCode, _, _, hasProviderStatus := providerHTTPMetadata(observationError)
	if !hasProviderStatus || statusCode != http.StatusNotFound || observationCount != 3 {
		testingInstance.Fatalf("status=%d has_status=%t observations=%d", statusCode, hasProviderStatus, observationCount)
	}
	expectedDecisions := []pollableResourceRetryDecision{
		pollableResourceRetryVisibility,
		pollableResourceRetryVisibility,
		pollableResourceDoNotRetry,
	}
	for decisionIndex, expectedDecision := range expectedDecisions {
		if retryDecisions[decisionIndex] != expectedDecision {
			testingInstance.Fatalf("decision[%d]=%d want=%d", decisionIndex, retryDecisions[decisionIndex], expectedDecision)
		}
	}
}
