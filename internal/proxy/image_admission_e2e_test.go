package proxy_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
)

func TestImageGenerationAdmissionRejectionIsDefinite(t *testing.T) {
	for _, surface := range []string{"images", "responses"} {
		for _, capability := range []string{llmproxycontract.MediaCapabilityImageGenerate, llmproxycontract.MediaCapabilityImageEdit} {
			t.Run(surface+"/"+capability, func(t *testing.T) {
				var calls atomic.Int32
				started := make(chan struct{}, 1)
				release := make(chan struct{})
				var releaseOnce sync.Once
				releaseProvider := func() { releaseOnce.Do(func() { close(release) }) }
				upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					calls.Add(1)
					started <- struct{}{}
					select {
					case <-release:
					case <-request.Context().Done():
						return
					}
					writer.WriteHeader(http.StatusServiceUnavailable)
				}))
				defer upstream.Close()
				defer releaseProvider()
				endpoints := proxy.NewEndpoints()
				endpoints.SetProviderBaseURL(proxy.ProviderNameOpenAI, upstream.URL)
				capacity := testfixtures.UpstreamCapacity(4, 4)
				capacity.Media = proxy.UpstreamCapacityLimit{Active: 1, Admitted: 1}
				router, err := testfixtures.BuildManagedRouter(t, proxy.Configuration{
					ProviderCatalog: testfixtures.ProviderCatalog(t), AssetStorePath: t.TempDir(), Endpoints: endpoints,
					UpstreamCapacity: capacity, MediaOperationWorkers: 2,
				}, zap.NewNop().Sugar(), testfixtures.ManagedTenant{
					Secret: "image-admission-owner", Defaults: proxy.TenantDefaults{Provider: proxy.ProviderNameOpenAI, Model: proxy.ModelNameGPT41},
					ProviderKeys: map[string]string{proxy.ProviderNameOpenAI: "image-admission-provider"},
				})
				if err != nil {
					t.Fatal(err)
				}
				server := httptest.NewServer(router)
				defer server.Close()
				defer releaseProvider()
				configuration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "image-admission-owner"})
				if err != nil {
					t.Fatal(err)
				}
				client, err := llmproxyclient.NewClient(configuration, server.Client())
				if err != nil {
					t.Fatal(err)
				}
				input := imageGenerationTestIntent()
				input.Surface = surface
				if surface == "responses" {
					input.ResponsesModel = "gpt-5"
				}
				first, err := client.CreateImageGeneration(t.Context(), "holds-media-capacity", input)
				if err != nil {
					t.Fatal(err)
				}
				select {
				case <-started:
				case <-time.After(3 * time.Second):
					t.Fatal("first image submission did not start")
				}
				assetID := ""
				if capability == llmproxycontract.MediaCapabilityImageEdit {
					asset, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "image/png", Data: streamingImageFixtures(t)[0]})
					if err != nil {
						t.Fatal(err)
					}
					assetID = asset.AssetID
				}
				create := func() (llmproxyclient.MediaOperation, error) {
					if capability == llmproxycontract.MediaCapabilityImageEdit {
						return client.CreateImageEditing(t.Context(), "rejected-media-capacity", llmproxyclient.ImageEditingInput{ImageGenerationInput: input, ImageAssetIDs: []string{assetID}})
					}
					return client.CreateImageGeneration(t.Context(), "rejected-media-capacity", input)
				}
				accepted, err := create()
				if err != nil {
					t.Fatal(err)
				}
				completed := waitForImageGeneration(t, client, accepted)
				if completed.State != proxy.MediaOperationStateFailed || completed.Error == nil || completed.Error.Code != llmproxycontract.ErrorCodeMediaOperationUnavailable || calls.Load() != 1 {
					t.Fatalf("local admission rejection: state=%s error=%v provider calls=%d", completed.State, completed.Error, calls.Load())
				}
				duplicate, err := create()
				if err != nil || duplicate.OperationID != accepted.OperationID || duplicate.State != proxy.MediaOperationStateFailed || calls.Load() != 1 {
					t.Fatalf("repeated rejected intent: result=%+v error=%v calls=%d", duplicate, err, calls.Load())
				}
				if assetID != "" {
					if err := client.DeleteAsset(t.Context(), assetID); err != nil {
						t.Fatalf("known rejection retained input asset: %v", err)
					}
				}
				releaseProvider()
				waitContext, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				unknown, err := client.WaitMediaOperation(waitContext, first.OperationID, time.Millisecond)
				if err != nil || unknown.State != proxy.MediaOperationStateUncertain || unknown.Error == nil || unknown.Error.Code != "provider_outcome_unknown" {
					t.Fatalf("dispatched server failure: result=%+v error=%v", unknown, err)
				}
			})
		}
	}
}
