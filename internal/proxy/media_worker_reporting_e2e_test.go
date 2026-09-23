package proxy_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
)

func TestMediaOperationWorkerReportsPersistenceFailures(t *testing.T) {
	for _, scenario := range []struct {
		name, trigger, state string
		calls                int32
	}{
		{"claim", "BEFORE INSERT ON media_operation_claim_records", proxy.MediaOperationStateQueued, 0},
		{"dispatch", "BEFORE UPDATE OF provider_execution_state ON media_operation_records WHEN NEW.provider_execution_state = 'dispatched'", proxy.MediaOperationStateFailed, 0},
		{"finish", "BEFORE UPDATE OF public_state ON media_operation_records WHEN NEW.public_state = 'failed'", proxy.MediaOperationStateRunning, 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				writer.WriteHeader(http.StatusBadRequest)
			}))
			t.Cleanup(upstream.Close)
			endpoints := proxy.NewEndpoints()
			endpoints.SetProviderBaseURL(proxy.ProviderNameOpenAI, upstream.URL)
			configuration, err := testfixtures.ProvisionManagedRouter(t, proxy.Configuration{
				ProviderCatalog: testfixtures.ProviderCatalog(t), AssetStorePath: t.TempDir(), Endpoints: endpoints,
			}, zap.NewNop().Sugar(), testfixtures.ManagedTenant{
				Secret: "worker-report-tenant", Defaults: proxy.TenantDefaults{Provider: proxy.ProviderNameOpenAI, Model: proxy.ModelNameGPT41},
				ProviderKeys: map[string]string{proxy.ProviderNameOpenAI: "private-report-provider-key"},
			})
			if err != nil {
				t.Fatal(err)
			}
			core, logs := observer.New(zap.InfoLevel)
			router, err := proxy.BuildRouter(configuration, zap.New(core).Sugar())
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			t.Cleanup(server.Close)
			database, err := gorm.Open(configuration.Management.DatabaseDialector, &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := database.Exec("CREATE TRIGGER reject_worker_write " + scenario.trigger + " BEGIN SELECT RAISE(ABORT, 'controlled worker persistence failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			clientConfiguration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "worker-report-tenant"})
			if err != nil {
				t.Fatal(err)
			}
			client, err := llmproxyclient.NewClient(clientConfiguration, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			input := llmproxyclient.MediaOperationInput{Capability: "image.generate", Provider: "openai", Model: "gpt-image-2",
				Input:    json.RawMessage(`{"prompt":"private-report-prompt"}`),
				Controls: json.RawMessage(`{"surface":"images","quality":"low","size":"1024x1024","background":"opaque","output_format":"png","output_count":1}`),
			}
			accepted, err := client.CreateMediaOperation(t.Context(), "reported-worker-failure", input)
			if err != nil {
				t.Fatal(err)
			}
			deadline := time.NewTimer(2 * time.Second)
			defer deadline.Stop()
			ticker := time.NewTicker(5 * time.Millisecond)
			defer ticker.Stop()
			for logs.FilterMessage("media operation worker failed").Len() == 0 {
				select {
				case <-ticker.C:
				case <-deadline.C:
					t.Fatal("worker persistence failure had no operator report")
				}
			}
			reports := logs.FilterMessage("media operation worker failed").All()
			fields := reports[0].ContextMap()
			errorText, _ := fields["error"].(string)
			if fields["operation_id"] != accepted.OperationID || !strings.Contains(errorText, scenario.name) || !strings.Contains(errorText, "controlled worker persistence failure") {
				t.Fatalf("worker report lacks failure context: %v", fields)
			}
			if strings.Contains(errorText, "private-report") {
				t.Fatalf("worker report exposed private input: %v", fields)
			}
			current, err := client.GetMediaOperation(context.Background(), accepted.OperationID)
			if err != nil || current.State != scenario.state || calls.Load() != scenario.calls {
				t.Fatalf("failed worker state=%+v calls=%d error=%v", current, calls.Load(), err)
			}
			replay, err := client.CreateMediaOperation(t.Context(), "reported-worker-failure", input)
			if err != nil || replay.OperationID != accepted.OperationID || calls.Load() != scenario.calls {
				t.Fatalf("failed worker replay=%+v calls=%d error=%v", replay, calls.Load(), err)
			}
		})
	}
}
