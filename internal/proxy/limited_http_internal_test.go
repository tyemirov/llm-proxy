package proxy

import (
	"context"
	"errors"
	"testing"
)

func TestLimitedHTTPDoerReleasesWorkerWhenAcquiredContextIsCanceled(testingInstance *testing.T) {
	doer := &limitedHTTPDoer{active: make(chan struct{}, 1)}
	doer.active <- struct{}{}
	requestContext, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()

	contextError := doer.releaseWorkerForCanceledContext(requestContext)
	if !errors.Is(contextError, context.Canceled) {
		testingInstance.Fatalf("context error=%v want=%v", contextError, context.Canceled)
	}
	if activeWorkerCount := len(doer.active); activeWorkerCount != 0 {
		testingInstance.Fatalf("active worker count=%d want=0", activeWorkerCount)
	}
}
