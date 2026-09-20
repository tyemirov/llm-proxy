package integration_test

import (
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestIntegrationUpstreamAdmissionIsolatesSaturatedOrigin(testingInstance *testing.T) {
	releaseSlow := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseSlow) }) }
	defer release()
	slowUpstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		select {
		case <-releaseSlow:
		case <-request.Context().Done():
			return
		}
		writer.Header().Set("Content-Type", contentTypeJSON)
		_, _ = io.WriteString(writer, rateLimitTextResponse)
	}))
	defer slowUpstream.Close()
	fastUpstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", contentTypeJSON)
		_, _ = io.WriteString(writer, rateLimitTextResponse)
	}))
	defer fastUpstream.Close()

	configuration := rateLimitIntegrationConfiguration(slowUpstream.URL)
	configuration.UpstreamCapacity = testfixtures.UpstreamCapacity(1, 2)
	configuration.Endpoints.SetProviderBaseURL(proxy.ProviderNameMoonshot, fastUpstream.URL)
	observedCore, observedLogs := observer.New(zap.InfoLevel)
	application := httptest.NewServer(buildRateLimitIntegrationRouter(testingInstance, configuration, zap.New(observedCore).Sugar()))
	observedLogs.TakeAll()
	defer application.Close()
	defer release()

	const slowRequestCount = 8
	results := make(chan rateLimitRequestResult, slowRequestCount)
	for index := 0; index < slowRequestCount; index++ {
		go func() {
			status, err := performRateLimitTextRequest(application.Client(), application.URL, proxy.ProviderNameDeepSeek)
			results <- rateLimitRequestResult{statusCode: status, requestError: err}
		}()
	}
	select {
	case result := <-results:
		if result.requestError != nil || result.statusCode != http.StatusServiceUnavailable {
			testingInstance.Fatalf("saturated origin result: status=%d error=%v", result.statusCode, result.requestError)
		}
	case <-time.After(queueAssertionTimeout):
		testingInstance.Fatal("saturated origin did not reject excess work")
	}

	status, err := performRateLimitTextRequest(application.Client(), application.URL, proxy.ProviderNameMoonshot)
	if err != nil || status != http.StatusOK {
		testingInstance.Fatalf("independent origin must retain capacity: status=%d error=%v", status, err)
	}
	release()
	for index := 1; index < slowRequestCount; index++ {
		select {
		case result := <-results:
			if result.requestError != nil || (result.statusCode != http.StatusOK && result.statusCode != http.StatusServiceUnavailable) {
				testingInstance.Fatalf("slow origin result: status=%d error=%v", result.statusCode, result.requestError)
			}
		case <-time.After(queueAssertionTimeout):
			testingInstance.Fatal("slow origin request did not finish")
		}
	}

	decisions := map[string]bool{}
	for _, event := range observedLogs.FilterMessage("upstream HTTP admission").All() {
		fields := event.ContextMap()
		if fields["request_id"] == nil || fields["request_id"] == "" {
			testingInstance.Fatalf("admission event has no request correlation: %v", fields)
		}
		if fields["upstream_origin"] != slowUpstream.URL && fields["upstream_origin"] != fastUpstream.URL {
			testingInstance.Fatalf("admission event has unexpected origin: %v", fields)
		}
		decisions[fields["decision"].(string)] = true
		for _, secretField := range []string{"prompt", "api_key", "authorization", "connection_values"} {
			if _, present := fields[secretField]; present {
				testingInstance.Fatalf("admission event exposed %s", secretField)
			}
		}
	}
	for _, decision := range []string{"admitted", "active", "capacity_wait", "released", "rejected"} {
		if !decisions[decision] {
			testingInstance.Errorf("missing admission decision %q in %v", decision, decisions)
		}
	}
}
