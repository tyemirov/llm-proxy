package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func admissionTestHTTPDoer(t *testing.T, firstOrigin, secondOrigin string, rates upstreamRateLimits) *admissionHTTPDoer {
	t.Helper()
	raw := admissionTestConfiguration()
	raw.Origins = []UpstreamOriginCapacity{
		{Origin: firstOrigin, Active: 1, Queued: 2},
		{Origin: secondOrigin, Active: 1, Queued: 2},
	}
	capacity, err := newUpstreamCapacity(raw, rates)
	if err != nil {
		t.Fatal(err)
	}
	return newAdmissionHTTPDoer(http.DefaultClient, capacity, rates, zap.NewNop().Sugar(), systemUpstreamAdmissionClock{}).(*admissionHTTPDoer)
}

func admissionTestHTTPRequest(t *testing.T, ctx context.Context, target string) *http.Request {
	t.Helper()
	ctx = requestContextWithUpstreamScope(ctx, upstreamRequestScope{tenant: "tenant", account: "account", class: upstreamInteractive})
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func assertAdmissionHTTPIdle(t *testing.T, doer *admissionHTTPDoer) {
	t.Helper()
	doer.scheduler.mutex.Lock()
	defer doer.scheduler.mutex.Unlock()
	if doer.scheduler.usage != (upstreamCapacityUsage{}) {
		t.Fatalf("capacity retained: %+v", doer.scheduler.usage)
	}
	for _, origin := range doer.scheduler.origins {
		if len(origin.accounts) != 0 || len(origin.tenants) != 0 || len(origin.tenantOrder) != 0 {
			t.Fatal("finished HTTP requests retained principal state")
		}
	}
}

func TestUpstreamAdmissionHTTPBodyOwnsPermitUntilClose(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, "body")
	}))
	defer upstream.Close()
	other := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, "independent")
	}))
	defer other.Close()
	doer := admissionTestHTTPDoer(t, upstream.URL, other.URL, upstreamRateLimits{})
	response, err := doer.Do(admissionTestHTTPRequest(t, context.Background(), upstream.URL))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	// Reading to EOF does not surrender ownership of the response body.
	if _, err := io.ReadAll(response.Body); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if _, err := doer.Do(admissionTestHTTPRequest(t, ctx, upstream.URL)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("same-origin request while body remains open: %v", err)
	}
	independent, err := doer.Do(admissionTestHTTPRequest(t, context.Background(), other.URL))
	if err != nil {
		t.Fatalf("independent origin: %v", err)
	}
	_ = independent.Body.Close()
	_ = response.Body.Close()
	_ = response.Body.Close()
	replacement, err := doer.Do(admissionTestHTTPRequest(t, context.Background(), upstream.URL))
	if err != nil {
		t.Fatalf("capacity after body close: %v", err)
	}
	_ = replacement.Body.Close()
	assertAdmissionHTTPIdle(t, doer)
}

func TestUpstreamAdmissionHTTPCancellationAndRateWaitReleaseCapacity(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, "body")
	}))
	defer upstream.Close()
	rates := upstreamRateLimits{rules: map[string]upstreamRateLimitRule{upstream.URL: {maxRequests: 1, interval: 80 * time.Millisecond}}}
	doer := admissionTestHTTPDoer(t, upstream.URL, admissionOriginB, rates)
	response, err := doer.Do(admissionTestHTTPRequest(t, context.Background(), upstream.URL))
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := doer.Do(admissionTestHTTPRequest(t, ctx, upstream.URL)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("rate wait cancellation: %v", err)
	}
	assertAdmissionHTTPIdle(t, doer)
	replacement, err := doer.Do(admissionTestHTTPRequest(t, context.Background(), upstream.URL))
	if err != nil {
		t.Fatalf("rate window recovery: %v", err)
	}
	_ = replacement.Body.Close()
	assertAdmissionHTTPIdle(t, doer)
}

func TestUpstreamAdmissionHTTPConcurrentCancellationDoesNotLeak(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		writer.(http.Flusher).Flush()
		<-request.Context().Done()
	}))
	defer upstream.Close()
	doer := admissionTestHTTPDoer(t, upstream.URL, admissionOriginB, upstreamRateLimits{})
	const count = 40
	var callers sync.WaitGroup
	callers.Add(count)
	start := make(chan struct{})
	results := make(chan error, count)
	for index := 0; index < count; index++ {
		request := admissionTestHTTPRequest(t, context.Background(), upstream.URL)
		go func() {
			defer callers.Done()
			<-start
			ctx, cancel := context.WithTimeout(request.Context(), 30*time.Millisecond)
			defer cancel()
			response, err := doer.Do(request.WithContext(ctx))
			if err == nil {
				_, _ = io.ReadAll(response.Body)
				_ = response.Body.Close()
			} else if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, errQueueFull) {
				results <- err
			}
		}()
	}
	close(start)
	callers.Wait()
	close(results)
	for err := range results {
		t.Errorf("concurrent request: %v", err)
	}
	assertAdmissionHTTPIdle(t, doer)
}

func TestUpstreamAdmissionHTTPRejectsMissingScope(t *testing.T) {
	doer := admissionTestHTTPDoer(t, admissionOriginA, admissionOriginB, upstreamRateLimits{})
	request, err := http.NewRequest(http.MethodGet, admissionOriginA, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doer.Do(request); !errors.Is(err, errUpstreamRequestScope) {
		t.Fatalf("missing scope: %v", err)
	}
	assertAdmissionHTTPIdle(t, doer)
}

func TestUpstreamAdmissionHTTPCancellationAfterAdmissionPreventsDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	doer := admissionTestHTTPDoer(t, admissionOriginA, admissionOriginB, upstreamRateLimits{})
	doer.next = geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
		t.Fatal("canceled request reached upstream")
		return nil, nil
	})
	// Cancel when admission is reported, before the transport can acquire its permit.
	doer.logger = zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(io.Discard), zapcore.InfoLevel), zap.Hooks(func(entry zapcore.Entry) error {
		cancel()
		return nil
	})).Sugar()
	if _, err := doer.Do(admissionTestHTTPRequest(t, ctx, admissionOriginA)); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled admission: %v", err)
	}
	assertAdmissionHTTPIdle(t, doer)
}

func TestUpstreamAdmissionHTTPInteractiveOnlyOriginRejectsBulk(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { _, _ = io.WriteString(writer, "interactive") }))
	defer upstream.Close()
	doer := admissionTestHTTPDoer(t, upstream.URL, admissionOriginB, upstreamRateLimits{})
	for _, class := range []upstreamWorkClass{upstreamMedia, upstreamStatus, upstreamTransfer} {
		ctx := requestContextWithUpstreamScope(context.Background(), upstreamRequestScope{tenant: "tenant", account: "account", class: class})
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := doer.Do(request); !errors.Is(err, errQueueFull) {
			t.Fatalf("interactive-only bulk class=%v: %v", class, err)
		}
	}
	response, err := doer.Do(admissionTestHTTPRequest(t, context.Background(), upstream.URL))
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	assertAdmissionHTTPIdle(t, doer)
}
