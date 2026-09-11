package proxy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const controlledMediaAdapterKey = "video.generate|xai|grok-imagine-video-1.5"

type controlledMediaOperationAdapter struct {
	executeStarted  chan struct{}
	releaseExecute  chan struct{}
	executeCalls    atomic.Int32
	validationCalls atomic.Int32
}

type restartMediaOperationAdapter struct {
	database     *gorm.DB
	executeCalls atomic.Int32
	recoverCalls atomic.Int32
}

func (*restartMediaOperationAdapter) Validate(request proxy.MediaOperationAdapterRequest) (proxy.MediaOperationValidatedRequest, error) {
	return proxy.MediaOperationValidatedRequest{Input: request.Input, Controls: request.Controls}, nil
}

func (adapter *restartMediaOperationAdapter) Execute(_ context.Context, request proxy.MediaOperationExecutionRequest) proxy.MediaOperationExecutionResult {
	adapter.executeCalls.Add(1)
	_ = adapter.database.Exec("UPDATE media_operation_claim_records SET worker_id = ?, generation = generation + 1, expires_at = ? WHERE operation_id = ?", "crashed-worker", time.Now().Add(100*time.Millisecond), request.OperationID).Error
	return proxy.MediaOperationExecutionResult{State: proxy.MediaOperationStateSucceeded, ProviderHandle: "lost-response-handle"}
}

func (adapter *restartMediaOperationAdapter) Recover(_ context.Context, request proxy.MediaOperationExecutionRequest) proxy.MediaOperationExecutionResult {
	adapter.recoverCalls.Add(1)
	if request.DispatchToken == "" {
		return proxy.MediaOperationExecutionResult{State: proxy.MediaOperationStateUncertain}
	}
	return proxy.MediaOperationExecutionResult{State: proxy.MediaOperationStateSucceeded, ProviderHandle: "recovered-handle", Outputs: []proxy.MediaOperationOutput{{MIMEType: "video/mp4", Data: []byte("recovered-video")}}}
}

func (*restartMediaOperationAdapter) Cancel(context.Context, proxy.MediaOperationExecutionRequest) proxy.MediaOperationCancellationResult {
	return proxy.MediaOperationCancellationResult{State: proxy.MediaCancellationUnsupported}
}

func (adapter *controlledMediaOperationAdapter) Validate(request proxy.MediaOperationAdapterRequest) (proxy.MediaOperationValidatedRequest, error) {
	adapter.validationCalls.Add(1)
	var input struct {
		Prompt  string `json:"prompt"`
		AssetID string `json:"asset_id"`
	}
	if json.Unmarshal(request.Input, &input) != nil || strings.TrimSpace(input.Prompt) == "" {
		return proxy.MediaOperationValidatedRequest{}, io.ErrUnexpectedEOF
	}
	validated := proxy.MediaOperationValidatedRequest{Input: request.Input, Controls: request.Controls}
	if input.AssetID != "" {
		validated.InputAssetIDs = []string{input.AssetID}
	}
	return validated, nil
}

func (adapter *controlledMediaOperationAdapter) Execute(requestContext context.Context, _ proxy.MediaOperationExecutionRequest) proxy.MediaOperationExecutionResult {
	adapter.executeCalls.Add(1)
	select {
	case adapter.executeStarted <- struct{}{}:
	default:
	}
	select {
	case <-requestContext.Done():
		return proxy.MediaOperationExecutionResult{State: proxy.MediaOperationStateUncertain}
	case <-adapter.releaseExecute:
		return proxy.MediaOperationExecutionResult{
			State: proxy.MediaOperationStateSucceeded, ProviderHandle: "controlled-operation-1",
			Outputs: []proxy.MediaOperationOutput{{MIMEType: "video/mp4", Data: []byte("controlled-video")}},
		}
	}
}

func (*controlledMediaOperationAdapter) Recover(context.Context, proxy.MediaOperationExecutionRequest) proxy.MediaOperationExecutionResult {
	return proxy.MediaOperationExecutionResult{State: proxy.MediaOperationStateUncertain}
}

func (*controlledMediaOperationAdapter) Cancel(context.Context, proxy.MediaOperationExecutionRequest) proxy.MediaOperationCancellationResult {
	return proxy.MediaOperationCancellationResult{State: proxy.MediaCancellationUnsupported}
}

type invalidSuccessMediaOperationAdapter struct{}

func (*invalidSuccessMediaOperationAdapter) Validate(request proxy.MediaOperationAdapterRequest) (proxy.MediaOperationValidatedRequest, error) {
	return proxy.MediaOperationValidatedRequest{Input: request.Input, Controls: request.Controls}, nil
}

func (*invalidSuccessMediaOperationAdapter) Execute(context.Context, proxy.MediaOperationExecutionRequest) proxy.MediaOperationExecutionResult {
	return proxy.MediaOperationExecutionResult{State: proxy.MediaOperationStateSucceeded, ProviderHandle: "invalid-success"}
}

func (*invalidSuccessMediaOperationAdapter) Recover(context.Context, proxy.MediaOperationExecutionRequest) proxy.MediaOperationExecutionResult {
	return proxy.MediaOperationExecutionResult{State: proxy.MediaOperationStateUncertain}
}

func (*invalidSuccessMediaOperationAdapter) Cancel(context.Context, proxy.MediaOperationExecutionRequest) proxy.MediaOperationCancellationResult {
	return proxy.MediaOperationCancellationResult{State: proxy.MediaCancellationUnsupported}
}

func TestMediaOperationLifecycleUsesDurableTenantResources(testingInstance *testing.T) {
	adapter := &controlledMediaOperationAdapter{executeStarted: make(chan struct{}, 1), releaseExecute: make(chan struct{})}
	tenantSecret := "media-operation-tenant-secret"
	router := buildMediaOperationRouter(testingInstance, tenantSecret, adapter)
	server := httptest.NewServer(router)
	defer server.Close()
	clientConfiguration, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: tenantSecret})
	if configError != nil {
		testingInstance.Fatal(configError)
	}
	client, clientError := llmproxyclient.NewClient(clientConfiguration, server.Client())
	if clientError != nil {
		testingInstance.Fatal(clientError)
	}
	capabilities, capabilitiesError := client.GetMediaCapabilities(context.Background())
	if capabilitiesError != nil || capabilities.CatalogRevision == "" || len(capabilities.Routes) != 1 || capabilities.Routes[0].Capability != llmproxycontract.MediaCapabilityVideoGenerate {
		testingInstance.Fatalf("capabilities=%+v error=%v", capabilities, capabilitiesError)
	}

	input := llmproxyclient.MediaOperationInput{
		Capability: "video.generate", Provider: "xai", Model: "grok-imagine-video-1.5",
		Input:    json.RawMessage(`{"prompt":"fixture motion"}`),
		Controls: json.RawMessage(`{"aspect_ratio":"16:9","duration_seconds":1,"resolution":"480p"}`),
	}
	operation, createError := client.CreateMediaOperation(context.Background(), "media-operation-create-1", input)
	if createError != nil || operation.OperationID == "" || operation.State != proxy.MediaOperationStateQueued || operation.CatalogRevision == "" {
		testingInstance.Fatalf("accepted operation=%+v error=%v", operation, createError)
	}
	select {
	case <-adapter.executeStarted:
	case <-time.After(time.Second):
		testingInstance.Fatal("accepted operation was not dispatched")
	}

	duplicate, duplicateError := client.CreateMediaOperation(context.Background(), "media-operation-create-1", input)
	if duplicateError != nil || duplicate.OperationID != operation.OperationID || adapter.executeCalls.Load() != 1 || adapter.validationCalls.Load() != 1 {
		testingInstance.Fatalf("duplicate=%+v error=%v executes=%d validations=%d", duplicate, duplicateError, adapter.executeCalls.Load(), adapter.validationCalls.Load())
	}
	changed := input
	changed.Input = json.RawMessage(`{"prompt":"changed intent"}`)
	if _, conflictError := client.CreateMediaOperation(context.Background(), "media-operation-create-1", changed); httpFailureStatus(conflictError) != http.StatusConflict || adapter.validationCalls.Load() != 1 {
		testingInstance.Fatalf("conflict error=%v validations=%d", conflictError, adapter.validationCalls.Load())
	}

	close(adapter.releaseExecute)
	waitContext, cancelWait := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelWait()
	completed, waitError := client.WaitMediaOperation(waitContext, operation.OperationID, 10*time.Millisecond)
	if waitError != nil || completed.State != proxy.MediaOperationStateSucceeded || len(completed.Outputs) != 1 {
		testingInstance.Fatalf("completed=%+v error=%v", completed, waitError)
	}
	asset, assetError := client.GetAsset(context.Background(), completed.Outputs[0].AssetID)
	if assetError != nil || asset.MIMEType != "video/mp4" || asset.SizeBytes != int64(len("controlled-video")) {
		testingInstance.Fatalf("asset=%+v error=%v", asset, assetError)
	}
	content, downloadError := client.DownloadAsset(context.Background(), asset)
	if downloadError != nil || string(content) != "controlled-video" {
		testingInstance.Fatalf("content=%q error=%v", content, downloadError)
	}
	if deleteError := client.DeleteAsset(context.Background(), asset.AssetID); httpFailureStatus(deleteError) != http.StatusConflict {
		testingInstance.Fatalf("active output deletion error=%v", deleteError)
	}

	foreignRequest := httptest.NewRequest(http.MethodGet, llmproxycontract.MediaOperationsPath+"/"+operation.OperationID, nil)
	foreignRequest.Header.Set("Authorization", "Bearer wrong-tenant-secret")
	foreignResponse := httptest.NewRecorder()
	router.ServeHTTP(foreignResponse, foreignRequest)
	if foreignResponse.Code != http.StatusForbidden {
		testingInstance.Fatalf("foreign status=%d", foreignResponse.Code)
	}
}

func TestMediaOperationRejectsInvalidIntentWithoutDispatchAndCancelsQueuedWork(testingInstance *testing.T) {
	adapter := &controlledMediaOperationAdapter{executeStarted: make(chan struct{}, 2), releaseExecute: make(chan struct{})}
	tenantSecret := "media-operation-cancel-secret"
	router := buildMediaOperationRouter(testingInstance, tenantSecret, adapter)
	server := httptest.NewServer(router)
	defer server.Close()
	clientConfiguration, _ := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: tenantSecret})
	client, _ := llmproxyclient.NewClient(clientConfiguration, server.Client())

	inputAsset, uploadError := client.UploadAsset(context.Background(), llmproxyclient.AssetUploadInput{MIMEType: "image/png", Data: []byte("input-image")})
	if uploadError != nil {
		testingInstance.Fatal(uploadError)
	}
	validInput := llmproxyclient.MediaOperationInput{Capability: "video.generate", Provider: "xai", Model: "grok-imagine-video-1.5", Input: json.RawMessage(`{"prompt":"one","asset_id":"` + inputAsset.AssetID + `"}`), Controls: json.RawMessage(`{}`)}
	first, firstError := client.CreateMediaOperation(context.Background(), "blocking-operation", validInput)
	if firstError != nil {
		testingInstance.Fatal(firstError)
	}
	select {
	case <-adapter.executeStarted:
	case <-time.After(time.Second):
		testingInstance.Fatal("first operation did not start")
	}
	if deleteError := client.DeleteAsset(context.Background(), inputAsset.AssetID); httpFailureStatus(deleteError) != http.StatusConflict {
		testingInstance.Fatalf("active input deletion error=%v", deleteError)
	}
	unsupported, unsupportedError := client.CancelMediaOperation(context.Background(), first.OperationID)
	if unsupportedError != nil || unsupported.State != proxy.MediaOperationStateRunning || unsupported.CancellationState != proxy.MediaCancellationUnsupported {
		testingInstance.Fatalf("unsupported cancellation=%+v error=%v", unsupported, unsupportedError)
	}
	secondInput := validInput
	secondInput.Input = json.RawMessage(`{"prompt":"two"}`)
	second, secondError := client.CreateMediaOperation(context.Background(), "queued-operation", secondInput)
	if secondError != nil || second.State != proxy.MediaOperationStateQueued {
		testingInstance.Fatalf("second=%+v error=%v", second, secondError)
	}
	cancelled, cancellationError := client.CancelMediaOperation(context.Background(), second.OperationID)
	if cancellationError != nil || cancelled.State != proxy.MediaOperationStateCancelled || cancelled.CancellationState != proxy.MediaCancellationConfirmed {
		testingInstance.Fatalf("cancelled=%+v error=%v", cancelled, cancellationError)
	}
	invalid := validInput
	invalid.Input = json.RawMessage(`{"prompt":""}`)
	if _, invalidError := client.CreateMediaOperation(context.Background(), "invalid-operation", invalid); httpFailureStatus(invalidError) != http.StatusBadRequest || adapter.executeCalls.Load() != 1 {
		testingInstance.Fatalf("invalid error=%v executes=%d", invalidError, adapter.executeCalls.Load())
	}
	close(adapter.releaseExecute)
	waitContext, cancelWait := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelWait()
	if _, waitError := client.WaitMediaOperation(waitContext, first.OperationID, 10*time.Millisecond); waitError != nil {
		testingInstance.Fatal(waitError)
	}
	if deleteError := client.DeleteAsset(context.Background(), inputAsset.AssetID); deleteError != nil {
		testingInstance.Fatalf("terminal input deletion error=%v", deleteError)
	}
	if adapter.executeCalls.Load() != 1 {
		testingInstance.Fatalf("cancelled queued operation dispatched: %d", adapter.executeCalls.Load())
	}
}

func TestMediaOperationAssetsRequireCanonicalBearerAuthentication(testingInstance *testing.T) {
	adapter := &controlledMediaOperationAdapter{executeStarted: make(chan struct{}, 1), releaseExecute: make(chan struct{})}
	router := buildMediaOperationRouter(testingInstance, "asset-bearer-secret", adapter)
	queryRequest := httptest.NewRequest(http.MethodPost, llmproxycontract.AssetPath+"?key=asset-bearer-secret", bytes.NewReader([]byte("asset")))
	queryRequest.Header.Set("Content-Type", "image/png")
	queryResponse := httptest.NewRecorder()
	router.ServeHTTP(queryResponse, queryRequest)
	if queryResponse.Code != http.StatusForbidden {
		testingInstance.Fatalf("query authentication status=%d", queryResponse.Code)
	}
}

func TestMediaOperationRejectsProviderSuccessWithoutOutputArtifacts(testingInstance *testing.T) {
	router := buildMediaOperationRouter(testingInstance, "invalid-success-secret", &invalidSuccessMediaOperationAdapter{})
	server := httptest.NewServer(router)
	defer server.Close()
	clientConfiguration, _ := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "invalid-success-secret"})
	client, _ := llmproxyclient.NewClient(clientConfiguration, server.Client())
	accepted, createError := client.CreateMediaOperation(context.Background(), "invalid-success", llmproxyclient.MediaOperationInput{
		Capability: "video.generate", Provider: "xai", Model: "grok-imagine-video-1.5", Input: json.RawMessage(`{"prompt":"missing bytes"}`), Controls: json.RawMessage(`{}`),
	})
	if createError != nil {
		testingInstance.Fatal(createError)
	}
	waitContext, cancelWait := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelWait()
	completed, waitError := client.WaitMediaOperation(waitContext, accepted.OperationID, 10*time.Millisecond)
	if waitError != nil || completed.State != proxy.MediaOperationStateFailed || completed.Error == nil || completed.Error.Code != "provider_result_invalid" || len(completed.Outputs) != 0 {
		testingInstance.Fatalf("completed=%+v error=%v", completed, waitError)
	}
}

func TestMediaOperationRestartRecoversDispatchedWorkAndDeduplicatesUsage(testingInstance *testing.T) {
	adapter := &restartMediaOperationAdapter{}
	tenantSecret := "media-operation-restart-secret"
	tenant := testfixtures.StandardManagedTenant(tenantSecret)
	tenant.Defaults = proxy.TenantDefaults{Provider: proxy.ProviderNameXAI, Model: proxy.ModelNameGrok43}
	configuration, provisionError := testfixtures.ProvisionManagedRouter(testingInstance, proxy.Configuration{
		ProviderCatalog: testfixtures.ProviderCatalog(testingInstance), AssetStorePath: testingInstance.TempDir(), MediaOperationWorkers: 1,
		MediaOperationClaimSeconds: 2, MediaOperationClaimRenewalSeconds: 1,
		MediaOperationAdapters: map[string]proxy.MediaOperationAdapter{controlledMediaAdapterKey: adapter},
	}, zap.NewNop().Sugar(), tenant)
	if provisionError != nil {
		testingInstance.Fatal(provisionError)
	}
	database, databaseError := gorm.Open(configuration.Management.DatabaseDialector, &gorm.Config{})
	if databaseError != nil {
		testingInstance.Fatal(databaseError)
	}
	adapter.database = database
	firstRouter, firstBuildError := proxy.BuildRouter(configuration, zap.NewNop().Sugar())
	if firstBuildError != nil {
		testingInstance.Fatal(firstBuildError)
	}
	firstServer := httptest.NewServer(firstRouter)
	clientConfiguration, _ := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: firstServer.URL, Secret: tenantSecret})
	firstClient, _ := llmproxyclient.NewClient(clientConfiguration, firstServer.Client())
	operationInput := llmproxyclient.MediaOperationInput{Capability: "video.generate", Provider: "xai", Model: "grok-imagine-video-1.5", Input: json.RawMessage(`{"prompt":"recover me"}`), Controls: json.RawMessage(`{}`)}
	accepted, createError := firstClient.CreateMediaOperation(context.Background(), "restart-operation", operationInput)
	if createError != nil {
		testingInstance.Fatal(createError)
	}
	var credentialVersion struct {
		CredentialReference string
		ConnectionID        string
		Version             uint64
	}
	credentialQuery := `SELECT operations.credential_reference, assignments.connection_id, connections.version
		FROM media_operation_records AS operations
		JOIN managed_tenant_connection_records AS assignments ON assignments.tenant_id = operations.tenant_id AND assignments.provider_id = operations.provider
		JOIN managed_account_connection_records AS connections ON connections.id = assignments.connection_id
		WHERE operations.operation_id = ?`
	if credentialError := database.Raw(credentialQuery, accepted.OperationID).Scan(&credentialVersion).Error; credentialError != nil || credentialVersion.CredentialReference != fmt.Sprintf("%s:v%d", credentialVersion.ConnectionID, credentialVersion.Version) {
		testingInstance.Fatalf("credential version=%+v error=%v", credentialVersion, credentialError)
	}
	deadline := time.Now().Add(2 * time.Second)
	for adapter.executeCalls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	firstServer.Close()
	if adapter.executeCalls.Load() != 1 {
		testingInstance.Fatalf("execute calls=%d", adapter.executeCalls.Load())
	}

	secondRouter, secondBuildError := proxy.BuildRouter(configuration, zap.NewNop().Sugar())
	if secondBuildError != nil {
		testingInstance.Fatal(secondBuildError)
	}
	secondServer := httptest.NewServer(secondRouter)
	defer secondServer.Close()
	secondConfiguration, _ := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: secondServer.URL, Secret: tenantSecret})
	secondClient, _ := llmproxyclient.NewClient(secondConfiguration, secondServer.Client())
	waitContext, cancelWait := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelWait()
	recovered, waitError := secondClient.WaitMediaOperation(waitContext, accepted.OperationID, 10*time.Millisecond)
	if waitError != nil || recovered.State != proxy.MediaOperationStateSucceeded || adapter.recoverCalls.Load() != 1 || adapter.executeCalls.Load() != 1 {
		testingInstance.Fatalf("recovered=%+v error=%v executes=%d recovers=%d", recovered, waitError, adapter.executeCalls.Load(), adapter.recoverCalls.Load())
	}

	if updateError := database.Table("media_operation_records").Where("operation_id = ?", accepted.OperationID).Update("terminal_at", time.Now().Add(-49*time.Hour)).Error; updateError != nil {
		testingInstance.Fatal(updateError)
	}
	thirdRouter, thirdBuildError := proxy.BuildRouter(configuration, zap.NewNop().Sugar())
	if thirdBuildError != nil || thirdRouter == nil {
		testingInstance.Fatalf("third router=%v error=%v", thirdRouter, thirdBuildError)
	}
	thirdServer := httptest.NewServer(thirdRouter)
	defer thirdServer.Close()
	thirdConfiguration, _ := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: thirdServer.URL, Secret: tenantSecret})
	thirdClient, _ := llmproxyclient.NewClient(thirdConfiguration, thirdServer.Client())
	if _, expiredError := thirdClient.CreateMediaOperation(context.Background(), "restart-operation", operationInput); httpFailureStatus(expiredError) != http.StatusGone {
		testingInstance.Fatalf("expired duplicate error=%v", expiredError)
	}
	changedInput := operationInput
	changedInput.Input = json.RawMessage(`{"prompt":"new paid intent"}`)
	if _, conflictError := thirdClient.CreateMediaOperation(context.Background(), "restart-operation", changedInput); httpFailureStatus(conflictError) != http.StatusConflict {
		testingInstance.Fatalf("tombstone conflict error=%v", conflictError)
	}
	var deliveryCount int64
	if countError := database.Table("media_operation_usage_delivery_records").Where("operation_id = ?", accepted.OperationID).Count(&deliveryCount).Error; countError != nil || deliveryCount != 1 {
		testingInstance.Fatalf("delivery count=%d error=%v", deliveryCount, countError)
	}
	var usageCount int64
	if countError := database.Table("managed_usage_event_records").Where("endpoint = ? AND provider_id = ? AND model_id = ?", "media", "xai", "grok-imagine-video-1.5").Count(&usageCount).Error; countError != nil || usageCount != 1 {
		testingInstance.Fatalf("usage count=%d error=%v", usageCount, countError)
	}
	var tombstoneCount int64
	if countError := database.Table("media_operation_tombstone_records").Where("operation_id = ?", accepted.OperationID).Count(&tombstoneCount).Error; countError != nil || tombstoneCount != 1 {
		testingInstance.Fatalf("tombstone count=%d error=%v", tombstoneCount, countError)
	}
}

func buildMediaOperationRouter(testingInstance *testing.T, tenantSecret string, adapter proxy.MediaOperationAdapter) http.Handler {
	testingInstance.Helper()
	tenant := testfixtures.StandardManagedTenant(tenantSecret)
	tenant.Defaults = proxy.TenantDefaults{Provider: proxy.ProviderNameXAI, Model: proxy.ModelNameGrok43}
	router, routerError := testfixtures.BuildManagedRouter(testingInstance, proxy.Configuration{
		ProviderCatalog: testfixtures.ProviderCatalog(testingInstance), AssetStorePath: testingInstance.TempDir(),
		MediaOperationWorkers:  1,
		MediaOperationAdapters: map[string]proxy.MediaOperationAdapter{controlledMediaAdapterKey: adapter},
	}, zap.NewNop().Sugar(), tenant)
	if routerError != nil {
		testingInstance.Fatal(routerError)
	}
	return router
}

func httpFailureStatus(errorValue error) int {
	var failure *llmproxyclient.HTTPFailure
	if !errors.As(errorValue, &failure) {
		return 0
	}
	return failure.StatusCode()
}
