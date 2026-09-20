package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/tyemirov/llm-proxy/internal/constants"
	"go.uber.org/zap"
)

type upstreamScopeContextKey struct{}

type releasingReadCloser struct {
	body        io.ReadCloser
	releaseOnce sync.Once
	release     func()
}

func (body *releasingReadCloser) Read(buffer []byte) (int, error) {
	return body.body.Read(buffer)
}

func (body *releasingReadCloser) Close() error {
	err := body.body.Close()
	body.releaseOnce.Do(body.release)
	return err
}

var (
	errUpstreamRequestScope = errors.New("upstream_request_scope_required")
	// Only admission errors carry this marker; the provider transport was not called.
	errUpstreamNotDispatched = errors.New("upstream_request_not_dispatched")
)

type upstreamAdmissionClock interface {
	Now() time.Time
	Wait(context.Context, <-chan struct{}, time.Duration) error
}

type systemUpstreamAdmissionClock struct{}

func (systemUpstreamAdmissionClock) Now() time.Time { return time.Now() }

func (systemUpstreamAdmissionClock) Wait(ctx context.Context, changed <-chan struct{}, duration time.Duration) error {
	var elapsed <-chan time.Time
	if duration > 0 {
		timer := time.NewTimer(duration)
		defer timer.Stop()
		elapsed = timer.C
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-changed:
		return nil
	case <-elapsed:
		return nil
	}
}

type admissionHTTPDoer struct {
	next      HTTPDoer
	scheduler *upstreamAdmissionScheduler
	clock     upstreamAdmissionClock
	logger    *zap.SugaredLogger
}

type scopedUpstreamHTTPDoer struct {
	next  HTTPDoer
	scope upstreamRequestScope
}

func (doer scopedUpstreamHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	return doer.next.Do(request.Clone(requestContextWithUpstreamScope(request.Context(), doer.scope)))
}

func newAdmissionHTTPDoer(next HTTPDoer, capacity upstreamCapacity, rates upstreamRateLimits, logger *zap.SugaredLogger, clock upstreamAdmissionClock) HTTPDoer {
	return &admissionHTTPDoer{next: next, scheduler: newUpstreamAdmissionScheduler(capacity, rates), clock: clock, logger: logger}
}

func requestContextWithUpstreamScope(ctx context.Context, scope upstreamRequestScope) context.Context {
	return context.WithValue(ctx, upstreamScopeContextKey{}, scope)
}

func (doer *admissionHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	scope, present := request.Context().Value(upstreamScopeContextKey{}).(upstreamRequestScope)
	if !present || scope.tenant == "" || scope.account == "" || scope.class >= upstreamWorkClassCount {
		return nil, backoff.Permanent(errors.Join(errUpstreamNotDispatched, errUpstreamRequestScope))
	}
	started := time.Now()
	origin := upstreamRequestOrigin(request.URL)
	admission, err := doer.scheduler.admit(request.Context(), origin, scope, doer.clock.Now())
	addRequestTelemetryPhase(request.Context(), requestTelemetryPhaseUpstreamAdmission, started)
	if err != nil {
		doer.logAdmission(request.Context(), origin, scope, upstreamDecisionRejected)
		return nil, errors.Join(errUpstreamNotDispatched, err)
	}
	doer.logAdmission(request.Context(), origin, scope, upstreamDecisionAdmitted)
	if err := doer.acquire(admission, origin); err != nil {
		return nil, errors.Join(errUpstreamNotDispatched, err)
	}
	providerStarted := time.Now()
	response, err := doer.next.Do(request)
	if err != nil {
		addRequestTelemetryPhase(request.Context(), requestTelemetryPhaseProviderHTTP, providerStarted)
		doer.release(admission)
		return nil, err
	}
	response.Body = &releasingReadCloser{body: response.Body, release: func() {
		addRequestTelemetryPhase(request.Context(), requestTelemetryPhaseProviderHTTP, providerStarted)
		doer.release(admission)
	}}
	return response, nil
}

func (doer *admissionHTTPDoer) acquire(admission *upstreamAdmission, origin string) (resultError error) {
	var initialRateWait, totalRateWait time.Duration
	lastWait := upstreamAdmissionDecision("")
	defer func() {
		doer.logRateWait(admission.ctx, origin, admission.origin.rate, initialRateWait, totalRateWait, resultError)
	}()
	for {
		scheduler := doer.scheduler
		scheduler.mutex.Lock()
		if err := admission.ctx.Err(); err != nil {
			scheduler.release(admission)
			scheduler.dispatch(doer.clock.Now())
			scheduler.mutex.Unlock()
			doer.logAdmission(admission.ctx, origin, admission.scope, upstreamDecisionCanceled)
			return err
		}
		now := doer.clock.Now()
		scheduler.dispatch(now)
		if admission.state == upstreamAdmissionActive {
			scheduler.mutex.Unlock()
			doer.logAdmission(admission.ctx, origin, admission.scope, upstreamDecisionActive)
			return nil
		}
		nextWake := scheduler.nextRateWait(now)
		ownRateWait := time.Duration(0)
		if scheduler.canActivate(admission) {
			ownRateWait = admission.origin.rateWait(now)
		}
		changed := scheduler.changed
		scheduler.mutex.Unlock()

		decision := upstreamDecisionCapacityWait
		if ownRateWait > 0 {
			decision = upstreamDecisionRateWait
		}
		if decision != lastWait {
			doer.logAdmission(admission.ctx, origin, admission.scope, decision)
			lastWait = decision
		}
		if ownRateWait > 0 && initialRateWait == 0 {
			initialRateWait = ownRateWait
		}
		wallStarted := time.Now()
		waitStarted := doer.clock.Now()
		waitError := doer.clock.Wait(admission.ctx, changed, nextWake)
		phase := requestTelemetryPhaseUpstreamAdmission
		if ownRateWait > 0 {
			totalRateWait += doer.clock.Now().Sub(waitStarted)
			phase = requestTelemetryPhaseUpstreamRateLimit
		}
		addRequestTelemetryPhase(admission.ctx, phase, wallStarted)
		if waitError != nil {
			doer.release(admission)
			doer.logAdmission(admission.ctx, origin, admission.scope, upstreamDecisionCanceled)
			return waitError
		}
	}
}

func (doer *admissionHTTPDoer) release(admission *upstreamAdmission) {
	doer.scheduler.mutex.Lock()
	doer.scheduler.release(admission)
	doer.scheduler.dispatch(doer.clock.Now())
	doer.scheduler.mutex.Unlock()
	doer.logAdmission(admission.ctx, admission.origin.name, admission.scope, upstreamDecisionReleased)
}

func (doer *admissionHTTPDoer) logRateWait(ctx context.Context, origin string, rule upstreamRateLimitRule, initial, total time.Duration, err error) {
	if initial == 0 {
		return
	}
	fields := []any{
		constants.LogFieldUpstreamOrigin, origin,
		constants.LogFieldRateLimitMaxRequests, rule.maxRequests,
		constants.LogFieldRateLimitInterval, rule.interval.String(),
		constants.LogFieldRateLimitInitialWaitMilliseconds, initial.Milliseconds(),
		constants.LogFieldRateLimitTotalWaitMilliseconds, total.Milliseconds(),
	}
	if requestID, present := ctx.Value(requestIdentifierContextKey{}).(string); present {
		fields = append(fields, logFieldRequestID, requestID)
	}
	doer.logger.Infow(constants.LogEventUpstreamRateLimitDelayed, fields...)
	if err != nil {
		doer.logger.Warnw(constants.LogEventUpstreamRateLimitCanceled, append(fields, constants.LogFieldError, err)...)
	}
}
