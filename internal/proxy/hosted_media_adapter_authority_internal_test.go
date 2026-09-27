package proxy

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

func TestHostedMediaAdapterCannotBorrowPaidAuthority(t *testing.T) {
	fixture := newFundedMediaRecoveryFixture(t)
	worker := fixture.worker
	key := mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageGenerate, "openai", "gpt-image-2")
	adapter := worker.adapters[key].(*imageGenerationAdapter)
	var probes atomic.Int64
	worker.adapters[key] = hostedMediaExecutionProbe{MediaOperationAdapter: adapter, probe: func(ctx context.Context, request MediaOperationExecutionRequest) {
		requestTenant, authenticated := adapter.tenants.authenticate(ctx, hostedIdentityFixtureKey)
		if !authenticated {
			t.Error("funded tenant authentication failed")
			return
		}
		provider, err := worker.providers.forTenant(requestTenant).resolveProvider("openai", "")
		if err != nil {
			t.Error(err)
			return
		}
		if reference, err := worker.mediaOperationCredentialReference(ctx, requestTenant, provider, adapter); reference != "" || !errors.Is(err, errMediaOperationUnavailable) {
			t.Errorf("unresolved grant exposed a media credential: reference=%q error=%v", reference, err)
			return
		}
		probes.Add(1)
		otherProvider := worker.providers.definitions[providerID("anthropic")]
		if reference, err := worker.store.credentialReference(ctx, request.TenantID, otherProvider.identifier); reference != "" || !errors.Is(err, errHostedAuthorityDenied) {
			t.Errorf("another provider borrowed media authority: reference=%q error=%v", reference, err)
			return
		}
		probes.Add(1)
		other := request
		other.CredentialReference = platformCredentialReference("platform-journal", 2)
		result := adapter.Execute(ctx, other)
		if result.State != MediaOperationStateFailed || result.ErrorCode != errMediaOperationUnavailable.Error() || fixture.calls.Load() != 0 {
			t.Errorf("another credential borrowed media authority: result=%v calls=%d", result, fixture.calls.Load())
			return
		}
		probes.Add(1)
	}}
	worker.runOperation("media-authority-worker", fixture.operationID)
	result := hostedMediaWorkerStatus(t, fixture.server, fixture.operationID)
	if result["state"] != MediaOperationStateSucceeded || len(result["outputs"].([]any)) != 1 || probes.Load() != 3 || fixture.calls.Load() != 1 {
		t.Fatalf("media authority prevented normal execution: result=%v probes=%d calls=%d", result, probes.Load(), fixture.calls.Load())
	}
	if err := fixture.database.reconcileHostedFunds(t.Context(), worker.store.now()); err != nil {
		t.Fatal(err)
	}
	assertHostedFundsBalance(t, fixture.database, 498, 498)
	before := fixture.state(t)
	fixture.server.Close()
	for range 2 {
		database := openJournalTransactionInstance(t, fixture.database)
		server, recovered := newFundedMediaRecoveryWorker(t, database, fixture.upstreamURL, worker.assets)
		replay := hostedMediaAdmissionHTTP(t, server, mediaRecoveryKey, mediaRecoveryPrompt, http.StatusOK)
		recovered.runOperation("replayed-media-authority", fixture.operationID)
		if err := database.reconcileHostedFunds(t.Context(), recovered.store.now()); err != nil {
			t.Fatal(err)
		}
		if replay["operation_id"] != fixture.operationID || fixture.calls.Load() != 1 || !reflect.DeepEqual(before, fixture.state(t)) {
			t.Fatal("media authority replay repeated provider or financial effects")
		}
		server.Close()
	}
}
