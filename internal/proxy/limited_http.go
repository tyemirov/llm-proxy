package proxy

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/tyemirov/llm-proxy/internal/constants"
	"go.uber.org/zap"
)

type limitedHTTPDoer struct {
	next         HTTPDoer
	active       chan struct{}
	admitted     chan struct{}
	rateLimiters map[string]*upstreamRateLimiter
	clock        upstreamRateLimitClock
	logger       *zap.SugaredLogger
}

type upstreamRateLimiter struct {
	rule       upstreamRateLimitRule
	timestamps []time.Time
	mutex      sync.Mutex
}

type upstreamRateLimitWait struct {
	initial time.Duration
	total   time.Duration
}

type upstreamRateLimitClock interface {
	Now() time.Time
	Wait(context.Context, time.Duration) error
}

type systemUpstreamRateLimitClock struct{}

type releasingReadCloser struct {
	body        io.ReadCloser
	releaseOnce sync.Once
	release     func()
}

func newLimitedHTTPDoer(next HTTPDoer, workerCount int, queueSize int, rateLimits upstreamRateLimits, structuredLogger *zap.SugaredLogger, clock upstreamRateLimitClock) HTTPDoer {
	rateLimiters := make(map[string]*upstreamRateLimiter, len(rateLimits.rules))
	for origin, rule := range rateLimits.rules {
		rateLimiters[origin] = &upstreamRateLimiter{rule: rule}
	}
	if structuredLogger == nil {
		structuredLogger = zap.NewNop().Sugar()
	}
	return &limitedHTTPDoer{
		next:         next,
		active:       make(chan struct{}, workerCount),
		admitted:     make(chan struct{}, workerCount+queueSize),
		rateLimiters: rateLimiters,
		clock:        clock,
		logger:       structuredLogger,
	}
}

func (doer *limitedHTTPDoer) Do(httpRequest *http.Request) (*http.Response, error) {
	if admissionError := doer.admit(); admissionError != nil {
		return nil, admissionError
	}
	if acquireError := doer.acquireUpstreamWorker(httpRequest); acquireError != nil {
		doer.releaseAdmission()
		return nil, acquireError
	}

	httpResponse, requestError := doer.next.Do(httpRequest)
	if requestError != nil {
		doer.releaseActive()
		doer.releaseAdmission()
		return nil, requestError
	}
	httpResponse.Body = &releasingReadCloser{
		body: httpResponse.Body,
		release: func() {
			doer.releaseActive()
			doer.releaseAdmission()
		},
	}
	return httpResponse, nil
}

func (doer *limitedHTTPDoer) acquireUpstreamWorker(httpRequest *http.Request) error {
	origin := upstreamRequestOrigin(httpRequest.URL)
	rateLimiter, rateLimited := doer.rateLimiters[origin]
	if !rateLimited {
		return doer.acquire(httpRequest)
	}
	wait, waitError := doer.acquireRateLimitedWorker(httpRequest, rateLimiter)
	if wait.initial > 0 {
		doer.logger.Infow(
			constants.LogEventUpstreamRateLimitDelayed,
			constants.LogFieldUpstreamOrigin, origin,
			constants.LogFieldRateLimitMaxRequests, rateLimiter.rule.maxRequests,
			constants.LogFieldRateLimitInterval, rateLimiter.rule.interval.String(),
			constants.LogFieldRateLimitInitialWaitMilliseconds, wait.initial.Milliseconds(),
			constants.LogFieldRateLimitTotalWaitMilliseconds, wait.total.Milliseconds(),
		)
	}
	if waitError != nil {
		if wait.initial > 0 {
			doer.logger.Warnw(
				constants.LogEventUpstreamRateLimitCanceled,
				constants.LogFieldUpstreamOrigin, origin,
				constants.LogFieldRateLimitMaxRequests, rateLimiter.rule.maxRequests,
				constants.LogFieldRateLimitInterval, rateLimiter.rule.interval.String(),
				constants.LogFieldRateLimitTotalWaitMilliseconds, wait.total.Milliseconds(),
				constants.LogFieldError, waitError,
			)
		}
		return waitError
	}
	return nil
}

func (doer *limitedHTTPDoer) acquireRateLimitedWorker(httpRequest *http.Request, rateLimiter *upstreamRateLimiter) (upstreamRateLimitWait, error) {
	wait := upstreamRateLimitWait{}
	waitStartedAt := doer.clock.Now()
	for {
		if acquireError := doer.acquire(httpRequest); acquireError != nil {
			wait.total = doer.clock.Now().Sub(waitStartedAt)
			return wait, acquireError
		}
		if contextError := httpRequest.Context().Err(); contextError != nil {
			doer.releaseActive()
			wait.total = doer.clock.Now().Sub(waitStartedAt)
			return wait, contextError
		}
		waitDuration := rateLimiter.nextWaitDuration(doer.clock.Now())
		if waitDuration <= 0 {
			if wait.initial > 0 {
				wait.total = doer.clock.Now().Sub(waitStartedAt)
			}
			return wait, nil
		}
		doer.releaseActive()
		if wait.initial == 0 {
			wait.initial = waitDuration
		}
		if waitError := doer.clock.Wait(httpRequest.Context(), waitDuration); waitError != nil {
			wait.total = doer.clock.Now().Sub(waitStartedAt)
			return wait, waitError
		}
	}
}

func (rateLimiter *upstreamRateLimiter) nextWaitDuration(now time.Time) time.Duration {
	rateLimiter.mutex.Lock()
	defer rateLimiter.mutex.Unlock()

	for len(rateLimiter.timestamps) > 0 && !rateLimiter.timestamps[0].Add(rateLimiter.rule.interval).After(now) {
		rateLimiter.timestamps = rateLimiter.timestamps[1:]
	}
	if len(rateLimiter.timestamps) < rateLimiter.rule.maxRequests {
		rateLimiter.timestamps = append(rateLimiter.timestamps, now)
		return 0
	}
	return rateLimiter.timestamps[0].Add(rateLimiter.rule.interval).Sub(now)
}

func (systemUpstreamRateLimitClock) Now() time.Time {
	return time.Now()
}

func (systemUpstreamRateLimitClock) Wait(requestContext context.Context, waitDuration time.Duration) error {
	waitTimer := time.NewTimer(waitDuration)
	defer waitTimer.Stop()
	select {
	case <-requestContext.Done():
		return requestContext.Err()
	case <-waitTimer.C:
		return nil
	}
}

func (doer *limitedHTTPDoer) admit() error {
	select {
	case doer.admitted <- struct{}{}:
		return nil
	default:
		return backoff.Permanent(errQueueFull)
	}
}

func (doer *limitedHTTPDoer) acquire(httpRequest *http.Request) error {
	select {
	case doer.active <- struct{}{}:
		return nil
	case <-httpRequest.Context().Done():
		return httpRequest.Context().Err()
	}
}

func (doer *limitedHTTPDoer) releaseActive() {
	<-doer.active
}

func (doer *limitedHTTPDoer) releaseAdmission() {
	<-doer.admitted
}

func (body *releasingReadCloser) Read(buffer []byte) (int, error) {
	return body.body.Read(buffer)
}

func (body *releasingReadCloser) Close() error {
	closeError := body.body.Close()
	body.releaseOnce.Do(body.release)
	return closeError
}
