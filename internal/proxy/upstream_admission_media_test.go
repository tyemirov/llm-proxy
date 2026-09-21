package proxy_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type admissionMediaAdapter struct {
	origin  string
	phase   string
	started chan error
}

func (*admissionMediaAdapter) Validate(_ context.Context, request proxy.MediaOperationAdapterRequest) (proxy.MediaOperationValidatedRequest, error) {
	return proxy.MediaOperationValidatedRequest{Input: request.Input, Controls: request.Controls}, nil
}

func (adapter *admissionMediaAdapter) Execute(ctx context.Context, execution proxy.MediaOperationExecutionRequest) proxy.MediaOperationExecutionResult {
	clients := map[string]proxy.HTTPDoer{"media": execution.HTTP.Submission, "status": execution.HTTP.Status, "transfer": execution.HTTP.Transfer}
	client := clients[adapter.phase]
	if client == nil {
		adapter.started <- fmt.Errorf("scoped %s HTTP client is missing", adapter.phase)
		return proxy.MediaOperationExecutionResult{State: proxy.MediaOperationStateFailed, ErrorCode: "provider_error"}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, adapter.origin+"/media", nil)
	if err != nil {
		adapter.started <- err
		return proxy.MediaOperationExecutionResult{State: proxy.MediaOperationStateFailed, ErrorCode: "provider_error"}
	}
	adapter.started <- nil
	response, err := client.Do(request)
	if err != nil {
		return proxy.MediaOperationExecutionResult{State: proxy.MediaOperationStateFailed, ErrorCode: "provider_error"}
	}
	_, err = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		return proxy.MediaOperationExecutionResult{State: proxy.MediaOperationStateFailed, ErrorCode: "provider_error"}
	}
	return proxy.MediaOperationExecutionResult{State: proxy.MediaOperationStateSucceeded, ProviderHandle: "capacity-fixture", Outputs: []proxy.MediaOperationOutput{{MIMEType: "video/mp4", Data: []byte("video")}}}
}

func (*admissionMediaAdapter) Recover(context.Context, proxy.MediaOperationExecutionRequest) proxy.MediaOperationExecutionResult {
	return proxy.MediaOperationExecutionResult{State: proxy.MediaOperationStateUncertain}
}

func (*admissionMediaAdapter) Cancel(context.Context, proxy.MediaOperationExecutionRequest) proxy.MediaOperationCancellationResult {
	return proxy.MediaOperationCancellationResult{State: proxy.MediaCancellationUnsupported}
}

func TestUpstreamAdmissionPreservesTextDuringSameAccountMediaSaturation(t *testing.T) {
	for _, phase := range []string{"media", "status", "transfer"} {
		t.Run(phase, func(t *testing.T) {
			var activeMedia, maximumMedia atomic.Int32
			held := make(chan struct{}, 3)
			release := make(chan struct{})
			var releaseOnce sync.Once
			releaseMedia := func() { releaseOnce.Do(func() { close(release) }) }
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				if request.URL.Path != "/media" {
					_, _ = io.WriteString(writer, `{"id":"text","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"interactive progress"}]}]}`)
					return
				}
				active := activeMedia.Add(1)
				for previous := maximumMedia.Load(); active > previous; previous = maximumMedia.Load() {
					if maximumMedia.CompareAndSwap(previous, active) {
						break
					}
				}
				defer activeMedia.Add(-1)
				writer.WriteHeader(http.StatusOK)
				writer.(http.Flusher).Flush()
				held <- struct{}{}
				select {
				case <-release:
				case <-request.Context().Done():
				}
				_, _ = io.WriteString(writer, `{}`)
			}))
			defer upstream.Close()
			defer releaseMedia()
			adapter := &admissionMediaAdapter{origin: upstream.URL, phase: phase, started: make(chan error, 3)}
			endpoints := proxy.NewEndpoints()
			endpoints.SetProviderBaseURL(proxy.ProviderNameXAI, upstream.URL)
			tenant := testfixtures.StandardManagedTenant("capacity-media-tenant")
			tenant.Defaults = proxy.TenantDefaults{Provider: proxy.ProviderNameXAI, Model: proxy.ModelNameGrok43}
			observedCore, observedLogs := observer.New(zap.InfoLevel)
			router, err := testfixtures.BuildManagedRouter(t, proxy.Configuration{
				ProviderCatalog: testfixtures.ProviderCatalog(t), AssetStorePath: t.TempDir(), Endpoints: endpoints,
				UpstreamCapacity: testfixtures.UpstreamCapacity(2, 4), MediaOperationWorkers: 3,
				MediaOperationAdapters: map[string]proxy.MediaOperationAdapter{controlledMediaAdapterKey: adapter},
			}, zap.New(observedCore).Sugar(), tenant)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			defer releaseMedia()
			clientConfiguration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: tenant.Secret})
			if err != nil {
				t.Fatal(err)
			}
			client, err := llmproxyclient.NewClient(clientConfiguration, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			operations := []llmproxyclient.MediaOperation{}
			defer func() {
				releaseMedia()
				for _, operation := range operations {
					cleanupContext, cancelCleanup := context.WithTimeout(context.Background(), 2*time.Second)
					_, cleanupError := client.WaitMediaOperation(cleanupContext, operation.OperationID, 5*time.Millisecond)
					cancelCleanup()
					if cleanupError != nil {
						t.Errorf("finish accepted media operation: %v", cleanupError)
					}
				}
			}()
			for index := 0; index < 3; index++ {
				operation, err := client.CreateMediaOperation(context.Background(), fmt.Sprintf("capacity-%s-%d", phase, index), llmproxyclient.MediaOperationInput{
					Capability: "video.generate", Provider: "xai", Model: "grok-imagine-video-1.5",
					Input: json.RawMessage(`{"prompt":"capacity fixture"}`), Controls: json.RawMessage(`{}`),
				})
				if err != nil {
					t.Fatal(err)
				}
				operations = append(operations, operation)
				select {
				case err := <-adapter.started:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(time.Second):
					t.Fatal("accepted media worker did not start")
				}
			}
			select {
			case <-held:
			case <-time.After(time.Second):
				t.Fatal("media HTTP request did not start")
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/?key="+url.QueryEscape(tenant.Secret)+"&prompt=text", nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatalf("text during %s saturation: %v", phase, err)
			}
			body, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err != nil || response.StatusCode != http.StatusOK {
				t.Fatalf("text during %s saturation: status=%d body=%s error=%v", phase, response.StatusCode, body, err)
			}
			releaseMedia()
			for _, operation := range operations {
				waitCtx, cancelWait := context.WithTimeout(context.Background(), 2*time.Second)
				result, err := client.WaitMediaOperation(waitCtx, operation.OperationID, 5*time.Millisecond)
				cancelWait()
				if err != nil || result.State != proxy.MediaOperationStateSucceeded {
					t.Fatalf("media result=%+v error=%v", result, err)
				}
			}
			operationEvents := map[string]bool{}
			for _, event := range observedLogs.FilterMessage("upstream HTTP admission").All() {
				fields := event.ContextMap()
				if fields["work_class"] == phase {
					id, ok := fields["operation_id"].(string)
					if !ok || id == "" {
						t.Fatalf("media admission has no operation correlation: %v", fields)
					}
					operationEvents[id] = true
				}
			}
			for _, operation := range operations {
				if !operationEvents[operation.OperationID] {
					t.Fatalf("missing media admission events for %s", operation.OperationID)
				}
			}
			if maximumMedia.Load() != 1 {
				t.Fatalf("maximum %s HTTP concurrency=%d want=1", phase, maximumMedia.Load())
			}
		})
	}
}
