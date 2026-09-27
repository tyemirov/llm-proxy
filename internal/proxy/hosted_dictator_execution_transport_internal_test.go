package proxy

import (
	"context"
	"errors"
	"net"
	"net/http"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dictator "github.com/tyemirov/dictator/sdk/go/dictatorspeechv1"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type hostedMediaRecoveryProbe struct {
	MediaOperationAdapter
	probe func(context.Context, MediaOperationExecutionRequest)
}

func (adapter hostedMediaRecoveryProbe) Recover(ctx context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	adapter.probe(ctx, request)
	return adapter.MediaOperationAdapter.Recover(ctx, request)
}

func TestHostedDictatorRecoveryRejectsUploadWithoutRepeatingPaidWork(t *testing.T) {
	database, _, management, _ := newHostedRatingFixture(t)
	upstream := &hostedDictatorUsageUpstream{duration: 1.125}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	native := grpc.NewServer()
	dictator.RegisterVoiceServiceServer(native, upstream)
	dictator.RegisterArtifactServiceServer(native, upstream)
	go func() {
		if err := native.Serve(listener); err != nil {
			t.Error(err)
		}
	}()
	t.Cleanup(native.Stop)
	server, worker, intent := newHostedDictatorUsageFixture(t, database, ModelNameDictatorQwen3TTS, listener.Addr().String())
	key := mediaOperationAdapterKey(llmproxycontract.MediaCapabilityAudioSpeechGenerate, ProviderNameDictator, ModelNameDictatorQwen3TTS)
	adapter := worker.adapters[key].(*accountDictatorAdapter)
	var probes atomic.Int64
	worker.adapters[key] = hostedMediaRecoveryProbe{MediaOperationAdapter: adapter, probe: func(ctx context.Context, request MediaOperationExecutionRequest) {
		protocol, closeConnection, err := adapter.bindProtocol(ctx, request.TenantID, request.CredentialReference)
		if err != nil {
			t.Error(err)
			return
		}
		defer closeConnection()
		if stream, err := dictator.NewArtifactServiceClient(protocol.connection).UploadArtifact(ctx); stream != nil || !errors.Is(err, errHostedAuthorityDenied) {
			t.Errorf("recovery permitted another artifact upload: %v", err)
			return
		}
		probes.Add(1)
	}}
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var firstPoll atomic.Bool
	var releaseOnce sync.Once
	beforePoll := func(ctx context.Context) {
		if firstPoll.CompareAndSwap(false, true) {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
	}
	upstream.beforePoll.Store(&beforePoll)
	seedHostedFunds(t, database, 500)
	id := hostedSpeechHTTP(t, server, "recovery-authority", intent, http.StatusAccepted)["operation_id"].(string)
	go func() { defer close(done); worker.runOperation("original-dictator-worker", id) }()
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }); waitHostedMediaWorker(t, done) })
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("Dictator did not reach the provider poll")
	}
	if err := database.database.Model(&mediaOperationClaimRecord{}).Where("operation_id = ?", id).Update("expires_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	worker.runOperation("replacement-dictator-worker", id)
	releaseOnce.Do(func() { close(release) })
	waitHostedMediaWorker(t, done)
	result := hostedMediaWorkerStatus(t, server, id)
	if result["state"] != MediaOperationStateSucceeded || probes.Load() != 1 || upstream.submissions.Load() != 1 {
		t.Fatalf("Dictator recovery result=%v probes=%d submissions=%d", result, probes.Load(), upstream.submissions.Load())
	}
	assertDictatorFinancialOutcome(t, database, management, "exact")
	financial := fundsStartupFixture{database: database, management: management}
	before := financial.state(t)
	for range 2 {
		restarted := openJournalTransactionInstance(t, database)
		if err := restarted.reconcileHostedFunds(t.Context(), time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		worker.runOperation("replayed-dictator-worker", id)
		replay := hostedSpeechHTTP(t, server, "recovery-authority", intent, http.StatusOK)
		if replay["operation_id"] != id || upstream.submissions.Load() != 1 || probes.Load() != 1 || !reflect.DeepEqual(before, financial.state(t)) {
			t.Fatal("Dictator recovery replay repeated provider or financial effects")
		}
	}
}

func TestHostedDictatorExecutionTransportPreservesPaidAuthority(t *testing.T) {
	database, _, management, _ := newHostedRatingFixture(t)
	upstream := &hostedDictatorOperationsUpstream{}
	scenario := hostedDictatorInputScenarios[0]
	server, worker, intent := newHostedDictatorInputFixture(t, database, ModelNameDictatorWhisperBase, upstream, scenario)
	settings := hostedDictatorInputFinancialSettings(t, scenario)
	worker.catalog = settings.catalog
	worker.hostedAdmission = settings.mediaAdmission(worker.providers)
	key := mediaOperationAdapterKey(scenario.capability, ProviderNameDictator, ModelNameDictatorWhisperBase)
	adapter := worker.adapters[key].(*accountDictatorAdapter)
	var probes atomic.Int64
	worker.adapters[key] = hostedMediaExecutionProbe{MediaOperationAdapter: adapter, probe: func(ctx context.Context, request MediaOperationExecutionRequest) {
		protocol, closeConnection, err := adapter.bindProtocol(ctx, request.TenantID, request.CredentialReference)
		if err != nil {
			t.Error(err)
			return
		}
		defer closeConnection()
		if _, err := dictator.NewVoiceServiceClient(protocol.connection).ListSynthesisVoices(ctx, &dictator.ListSynthesisVoicesRequest{}); !errors.Is(err, errHostedAuthorityDenied) {
			t.Errorf("execution permitted metadata without metadata authority: %v", err)
			return
		}
		closeConnection()
		stream, err := dictator.NewArtifactServiceClient(protocol.connection).UploadArtifact(ctx)
		if stream != nil || status.Code(err) != codes.Canceled {
			t.Errorf("closed native connection error=%v stream=%v", err, stream)
			return
		}
		if upstream.submissions.Load() != 0 {
			t.Error("failed transport boundary submitted provider work")
			return
		}
		probes.Add(1)
	}}
	seedHostedFunds(t, database, 500)
	id := hostedSpeechHTTP(t, server, "transport-authority", intent, http.StatusAccepted)["operation_id"].(string)
	assertHostedFundsBalance(t, database, 500, 448)
	worker.runOperation("transport-authority-worker", id)
	result := hostedMediaWorkerStatus(t, server, id)
	if result["state"] != MediaOperationStateSucceeded || probes.Load() != 1 || upstream.submissions.Load() != 1 {
		t.Fatalf("transport recovery result=%v probes=%d submissions=%d", result, probes.Load(), upstream.submissions.Load())
	}
	financial := fundsStartupFixture{database: database, management: management}
	var previous map[string]any
	for iteration := range 2 {
		restarted := openJournalTransactionInstance(t, database)
		if err := restarted.reconcileHostedFunds(t.Context(), time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		worker.runOperation("transport-authority-replay", id)
		replay := hostedSpeechHTTP(t, server, "transport-authority", intent, http.StatusOK)
		if replay["operation_id"] != id || !reflect.DeepEqual(result, hostedMediaWorkerStatus(t, server, id)) || upstream.submissions.Load() != 1 || probes.Load() != 1 {
			t.Fatal("transport recovery or replay repeated provider work")
		}
		assertHostedFundsBalance(t, database, 490, 490)
		assertFundsCreditRemainder(t, database, "16013", "4000000")
		current := financial.state(t)
		if iteration > 0 && !reflect.DeepEqual(previous, current) {
			t.Fatal("transport recovery repeated financial effects")
		}
		previous = current
	}
}
