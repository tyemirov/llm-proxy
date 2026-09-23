package proxy

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestHostedFundsRuntimeFailurePreservesSettlementAndStopsHTTP(t *testing.T) {
	database, _, management, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	application := &proxyApplication{router: management.Config.Handler.(*gin.Engine), database: database, now: ratingTestAcceptanceTime}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- application.serve(ctx, listener) }()
	response, err := http.Get("http://" + listener.Addr().String() + managementAPIPath + fundsBalanceTestPath)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("runtime balance status=%d", response.StatusCode)
	}
	if err := database.database.Exec("CREATE TRIGGER reject_runtime_settlement BEFORE INSERT ON managed_funds_settlement_records BEGIN SELECT RAISE(ABORT, 'controlled_runtime_settlement_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(prices))
	hostedIdentityHTTP(t, server, "runtime-write-failure", "funded prompt", http.StatusOK)
	select {
	case err := <-stopped:
		if err == nil || !strings.Contains(err.Error(), "controlled_runtime_settlement_failure") {
			t.Fatalf("worker failure=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("failed worker left HTTP admission running")
	}
	connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err == nil {
		connection.Close()
		t.Fatal("failed worker retained HTTP listener")
	}
	assertHostedFundsBalance(t, database, 5, 2)
	var charges int64
	if err := database.database.Model(&managedChargeRecord{}).Count(&charges).Error; err != nil || charges != 0 {
		t.Fatalf("partial settlement charges=%d error=%v", charges, err)
	}
	if err := database.database.Exec("DROP TRIGGER reject_runtime_settlement").Error; err != nil {
		t.Fatal(err)
	}
	restarted, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { stopped <- application.serve(ctx, restarted) }()
	response, err = http.Get("http://" + restarted.Addr().String() + managementAPIPath + fundsBalanceTestPath)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	assertHostedFundsBalance(t, database, 5, 5)
	cancel()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("graceful stop=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runtime did not stop")
	}
	if calls.Load() != 1 {
		t.Fatalf("recovery repeated provider work: %d", calls.Load())
	}
}

func TestHostedFundsRuntimeStartupFailureClosesListener(t *testing.T) {
	database, _, management, _ := newHostedRatingFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	application := &proxyApplication{router: management.Config.Handler.(*gin.Engine), database: database, now: ratingTestAcceptanceTime}
	if err := application.serve(ctx, listener); !errors.Is(err, context.Canceled) {
		t.Fatalf("startup cancellation=%v", err)
	}
	connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err == nil {
		connection.Close()
		t.Fatal("failed startup retained HTTP listener")
	}
}

func TestHostedFundsRuntimeShutdownCancelsRequests(t *testing.T) {
	database, _, _, _ := newHostedRatingFixture(t)
	router := gin.New()
	entered := make(chan struct{})
	router.GET("/active", func(request *gin.Context) {
		close(entered)
		<-request.Request.Context().Done()
		request.Status(http.StatusServiceUnavailable)
	})
	application := &proxyApplication{router: router, database: database, now: ratingTestAcceptanceTime}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- application.serve(ctx, listener) }()
	finished := make(chan error, 1)
	go func() {
		response, err := http.Get("http://" + listener.Addr().String() + "/active")
		if err == nil {
			response.Body.Close()
		}
		finished <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP request did not start")
	}
	cancel()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("shutdown=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown left a live worker or request")
	}
	if err := <-finished; err != nil {
		t.Fatalf("HTTP shutdown=%v", err)
	}
}

func TestHostedFundsRunningServiceSettlesNewRequests(t *testing.T) {
	database, _, management, _ := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	var source struct{ File string }
	if err := database.database.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&source).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		if err := listener.Close(); err != nil {
			t.Fatal(err)
		}
		root := t.TempDir()
		address, _, _ := startHostedFundsProcess(t, hostedFundsProcessSettings{DatabasePath: source.File, ResponseRoot: filepath.Join(root, "responses"), ReadyPath: filepath.Join(root, "ready"), Stage: "financial_runtime", Port: port})
		deadline := time.NewTimer(5 * time.Second)
		defer deadline.Stop()
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for {
			response, err := http.Get(address + healthPath)
			if err == nil {
				response.Body.Close()
				if response.StatusCode == http.StatusOK {
					break
				}
			}
			select {
			case <-tick.C:
			case <-deadline.C:
				t.Fatal("financial runtime did not start")
			}
		}
	}
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	acceptedAt := time.Now().UTC()
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(hostedRatingFixtureAdmissionAt(t, 2, acceptedAt)), func(dependencies *hostedTextRequestDependencies) {
		dependencies.now = func() time.Time { return acceptedAt }
	})
	hostedIdentityHTTP(t, server, "runtime-settlement", "funded prompt", http.StatusOK)
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		balance := ratingHTTPExchange(t, management, http.MethodGet, fundsBalanceTestPath, "", http.StatusOK)
		if balance["reserved_cents"] == "0" {
			break
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatalf("running service left settlement pending: %v", balance)
		}
	}
	assertFundsCreditRemainder(t, database, "91", "25000")
	if calls.Load() != 1 {
		t.Fatalf("financial worker repeated provider work: %d", calls.Load())
	}
}

func TestHostedFundsSettlementWaitsForPublication(t *testing.T) {
	database, intent, management, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	request, _ := observeRatedFixture(t, database, intent("unpublished-financial-result"), newHostedFundsAdmission(prices), journalOutcomeComplete, []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "1000"}, {Dimension: "output_tokens", Unit: "token", Value: "1000"}})
	if err := deliverFundsFixture(t, database); err != nil {
		t.Fatal(err)
	}
	balance := ratingHTTPExchange(t, management, http.MethodGet, fundsBalanceTestPath, "", http.StatusOK)
	if balance["reserved_cents"] != "3" || balance["posted_cents"] != "5" {
		t.Fatalf("unpublished result charged: %v", balance)
	}
	publishRatingSummaryResult(t, database, request)
	if err := database.reconcileHostedFunds(t.Context(), request.CreatedAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	balance = ratingHTTPExchange(t, management, http.MethodGet, fundsBalanceTestPath, "", http.StatusOK)
	if balance["reserved_cents"] != "0" || balance["posted_cents"] != "4" {
		t.Fatalf("published result not settled: %v", balance)
	}
}
