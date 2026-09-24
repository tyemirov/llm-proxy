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
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

type hostedCancellationAuthorityUpstream struct {
	hostedDictatorUsageUpstream
	cancellations atomic.Int64
	reject        atomic.Bool
}

func (upstream *hostedCancellationAuthorityUpstream) CancelSynthesizeSpeechJob(ctx context.Context, request *dictator.CancelSynthesizeSpeechJobRequest) (*dictator.CancelSynthesizeSpeechJobResponse, error) {
	upstream.cancellations.Add(1)
	if upstream.reject.Load() {
		return nil, status.Error(codes.Unavailable, "controlled cancellation outage")
	}
	return upstream.hostedSpeechUpstream.CancelSynthesizeSpeechJob(ctx, request)
}

func TestHostedMediaCancellationAuthorityFailuresPreserveFundsAndPermitRetry(t *testing.T) {
	for _, scenario := range []string{"authority-read", "authority-absent", "provider-error", "observation-write"} {
		t.Run(scenario, func(t *testing.T) {
			database, _, management, _ := newHostedRatingFixture(t)
			upstream := &hostedCancellationAuthorityUpstream{}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			grpcServer := grpc.NewServer()
			dictator.RegisterVoiceServiceServer(grpcServer, upstream)
			dictator.RegisterArtifactServiceServer(grpcServer, upstream)
			go func() {
				if err := grpcServer.Serve(listener); err != nil {
					t.Error(err)
				}
			}()
			t.Cleanup(grpcServer.Stop)
			server, worker, intent := newHostedDictatorUsageFixture(t, database, ModelNameDictatorQwen3TTS, listener.Addr().String())
			seedHostedFunds(t, database, 500)
			id := hostedSpeechHTTP(t, server, "cancel-authority", intent, http.StatusAccepted)["operation_id"].(string)
			started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var announce, releaseOnce sync.Once
			beforePoll := func(ctx context.Context) {
				announce.Do(func() { close(started) })
				select {
				case <-release:
				case <-ctx.Done():
				}
			}
			upstream.beforePoll.Store(&beforePoll)
			go func() { defer close(done); worker.runOperation("cancellation-authority-worker", id) }()
			t.Cleanup(func() {
				releaseOnce.Do(func() { close(release) })
				waitHostedMediaWorker(t, done)
				worker.workers.Wait()
			})
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("funded speech did not reach provider status")
			}
			fixture := fundsStartupFixture{database: database, management: management}
			before := fixture.state(t)
			var failures atomic.Int64
			queries := database.database.Callback().Query()
			const callback = "test:media_cancellation_authority"
			if scenario == "authority-read" || scenario == "authority-absent" {
				if err := queries.After("gorm:query").Register(callback, func(tx *gorm.DB) {
					count, ok := tx.Statement.Dest.(*int64)
					if !ok || tx.DryRun || tx.Error != nil || tx.Statement.Table != "media_operation_records" || tx.Statement.Context.Value(hostedMediaAuthorizationContextKey{}) == nil {
						return
					}
					failures.Add(1)
					if scenario == "authority-read" {
						tx.AddError(errors.New("controlled cancellation authority failure"))
					} else {
						*count = 0
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = queries.Remove(callback) })
			}
			wantStatus, wantCalls := http.StatusOK, int64(0)
			if scenario == "provider-error" {
				upstream.reject.Store(true)
				wantCalls = 1
			}
			if scenario == "observation-write" {
				if err := database.database.Exec("CREATE TRIGGER reject_cancel_observation BEFORE INSERT ON managed_journal_observation_records BEGIN SELECT RAISE(ABORT, 'controlled cancellation observation failure'); END").Error; err != nil {
					t.Fatal(err)
				}
				wantStatus, wantCalls = http.StatusInternalServerError, 1
			}
			cancelFundedMediaHTTP(t, server, id, wantStatus)
			if scenario == "authority-read" || scenario == "authority-absent" {
				if err := queries.Remove(callback); err != nil {
					t.Fatal(err)
				}
				if failures.Load() != 1 {
					t.Fatalf("cancellation authority failures=%d want=1", failures.Load())
				}
			}
			current := hostedMediaWorkerStatus(t, server, id)
			wantCancellation := MediaCancellationUnsupported
			if scenario == "observation-write" {
				wantCancellation = MediaCancellationRequested
			}
			if current["state"] != MediaOperationStateRunning || current["cancellation_state"] != wantCancellation || upstream.cancellations.Load() != wantCalls || upstream.submissions.Load() != 1 || !reflect.DeepEqual(before, fixture.state(t)) {
				t.Fatalf("failed cancellation lost retry or funds: operation=%v cancellations=%d submissions=%d", current, upstream.cancellations.Load(), upstream.submissions.Load())
			}
			upstream.reject.Store(false)
			if scenario == "observation-write" {
				if err := database.database.Exec("DROP TRIGGER reject_cancel_observation").Error; err != nil {
					t.Fatal(err)
				}
			}
			cancelled := cancelFundedMediaHTTP(t, server, id, http.StatusOK)
			if cancelled["state"] != MediaOperationStateCancelled || cancelled["cancellation_state"] != MediaCancellationConfirmed || upstream.cancellations.Load() != wantCalls+1 {
				t.Fatalf("restored cancellation did not finish once: %v calls=%d", cancelled, upstream.cancellations.Load())
			}
			releaseOnce.Do(func() { close(release) })
			waitHostedMediaWorker(t, done)
			var previous map[string]any
			for iteration := 0; iteration < 2; iteration++ {
				restarted := openJournalTransactionInstance(t, database)
				if err := restarted.reconcileHostedFunds(t.Context(), time.Now().UTC()); err != nil {
					t.Fatal(err)
				}
				worker.runOperation("cancelled-replay-worker", id)
				cancelFundedMediaHTTP(t, server, id, http.StatusOK)
				replay := hostedSpeechHTTP(t, server, "cancel-authority", intent, http.StatusOK)
				if replay["operation_id"] != id || replay["state"] != MediaOperationStateCancelled || upstream.cancellations.Load() != wantCalls+1 || upstream.submissions.Load() != 1 {
					t.Fatal("cancellation replay repeated provider work")
				}
				assertHostedFundsBalance(t, restarted, 500, 448)
				financial := fixture.state(t)
				if iteration > 0 && !reflect.DeepEqual(previous, financial) {
					t.Fatal("cancellation replay repeated financial effects")
				}
				previous = financial
			}
		})
	}
}
