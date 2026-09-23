package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const hostedFundsProcessInput = "LLM_PROXY_HOSTED_FUNDS_PROCESS_INPUT"

type hostedFundsProcessSettings struct {
	DatabasePath string
	ResponseRoot string
	UpstreamURL  string
	ReadyPath    string
	BoundaryPath string
	Stage        string
	Now          time.Time
	Port         int
}

func TestHostedFundsServiceProcess(t *testing.T) {
	input := os.Getenv(hostedFundsProcessInput)
	if input == "" {
		t.Skip("subprocess fixture")
	}
	var settings hostedFundsProcessSettings
	if err := json.Unmarshal([]byte(input), &settings); err != nil {
		t.Fatal(err)
	}
	if settings.Stage == "financial_runtime" {
		management := newInternalManagementService(t, newFakeManagedTenantDatabase(), internalManagementProviderRegistry()).configuration
		management.DatabasePath = settings.DatabasePath
		configuration := withInternalUpstreamCapacity(t, Configuration{Port: settings.Port, Management: management, ProviderCatalog: internalCanonicalProviderCatalog(), AssetStorePath: settings.ResponseRoot})
		writeHostedRecoverySignal(t, settings.ReadyPath, fmt.Sprintf("http://127.0.0.1:%d", settings.Port))
		if err := Serve(configuration, zap.NewNop().Sugar()); err != nil {
			t.Fatal(err)
		}
		return
	}
	database, err := newGORMManagedTenantDatabase(ManagementConfiguration{DatabasePath: settings.DatabasePath}, internalManagedProviderKeyCipher(), internalManagementProviderRegistry())
	if err != nil {
		t.Fatal(err)
	}
	server := newHostedIdentityHTTPServer(t, database, settings.UpstreamURL, settings.ResponseRoot, fundsDependencies(hostedRatingFixtureAdmission(t, 2)), func(dependencies *hostedTextRequestDependencies) {
		dependencies.now = func() time.Time { return settings.Now }
		if settings.Stage == "before_settlement_receipt" {
			if err := database.database.Callback().Create().Before("gorm:create").Register("funds_process_settlement_boundary", func(transaction *gorm.DB) {
				if transaction.Statement.Table == "managed_funds_settlement_records" {
					writeHostedRecoverySignal(t, settings.BoundaryPath, "stopped")
					<-t.Context().Done()
				}
			}); err != nil {
				t.Fatal(err)
			}
		}
		if settings.Stage == "before_dispatch" {
			var stopped atomic.Bool
			if err := database.database.Callback().Update().Before("gorm:update").Register("funds_process_dispatch_boundary", func(transaction *gorm.DB) {
				changes, ok := transaction.Statement.Dest.(map[string]any)
				if ok && transaction.Statement.Table == "managed_journal_attempt_records" && fmt.Sprint(changes["state"]) == string(journalAttemptDispatched) && stopped.CompareAndSwap(false, true) {
					writeHostedRecoverySignal(t, settings.BoundaryPath, "stopped")
					<-t.Context().Done()
				}
			}); err != nil {
				t.Fatal(err)
			}
		}
	})
	writeHostedRecoverySignal(t, settings.ReadyPath, server.URL)
	<-t.Context().Done()
}

func startHostedFundsProcess(t *testing.T, settings hostedFundsProcessSettings) (string, func(), func()) {
	t.Helper()
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), executable, "-test.run=^TestHostedFundsServiceProcess$", "-test.v")
	command.Env = append(os.Environ(), hostedFundsProcessInput+"="+string(encoded))
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			if err := command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				t.Errorf("stop funds process: %v", err)
			}
			var exit *exec.ExitError
			if err := command.Wait(); !errors.As(err, &exit) {
				t.Errorf("funds process did not stop: %v output=%s", err, output.String())
			}
		})
	}
	t.Cleanup(stop)
	return waitHostedRecoverySignal(t, settings.ReadyPath, stop, &output), stop, func() { waitHostedRecoverySignal(t, settings.BoundaryPath, stop, &output) }
}

func fundsProcessHTTP(address, key string) (int, string, error) {
	request, err := http.NewRequest(http.MethodPost, address+"/?key="+hostedIdentityFixtureKey+"&provider=openai&model=gpt-4.1", strings.NewReader(`{"prompt":"funded prompt"}`))
	if err != nil {
		return 0, "", err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(llmproxycontract.HeaderIdempotencyKey, key)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	return response.StatusCode, string(body), err
}

func TestHostedFundsAdmissionAcrossProcessesAndRestart(t *testing.T) {
	testHostedFundsAdmissionAcrossProcessesAndRestart(t, 5, "")
}

func TestHostedFundsTenantLimitAcrossProcessesAndRestart(t *testing.T) {
	testHostedFundsAdmissionAcrossProcessesAndRestart(t, 10, `{"limit_cents":"3","revision":0}`)
}

func testHostedFundsAdmissionAcrossProcessesAndRestart(t *testing.T, funds int64, limit string) {
	t.Helper()
	database, _, management, _ := newHostedRatingFixture(t)
	seedHostedFunds(t, database, funds)
	if limit != "" {
		ratingHTTPExchange(t, management, http.MethodPut, tenantFundsLimitTestPath, limit, http.StatusOK)
	}
	var source struct{ File string }
	if err := database.database.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&source).Error; err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	root := t.TempDir()
	settings := hostedFundsProcessSettings{DatabasePath: source.File, ResponseRoot: filepath.Join(root, "responses"), UpstreamURL: upstream.URL, Now: ratingTestAcceptanceTime()}
	settings.ReadyPath = filepath.Join(root, "ready-first")
	first, stopFirst, _ := startHostedFundsProcess(t, settings)
	settings.ReadyPath = filepath.Join(root, "ready-second")
	second, stopSecond, _ := startHostedFundsProcess(t, settings)
	var group sync.WaitGroup
	start := make(chan struct{})
	statuses := make([]int, 2)
	for index, address := range []string{first, second} {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			status, body, err := fundsProcessHTTP(address, fmt.Sprintf("process-%d", index))
			if err != nil || (status != http.StatusOK && status != http.StatusPaymentRequired) {
				t.Errorf("process admission: status=%d body=%s error=%v", status, body, err)
			}
			statuses[index] = status
		}()
	}
	close(start)
	group.Wait()
	if !((statuses[0] == http.StatusOK && statuses[1] == http.StatusPaymentRequired) || (statuses[1] == http.StatusOK && statuses[0] == http.StatusPaymentRequired)) {
		t.Fatalf("processes exceeded balance: statuses=%v", statuses)
	}
	assertHostedFundsBalance(t, database, funds, funds-3)
	stopFirst()
	stopSecond()
	settings.ReadyPath = filepath.Join(root, "ready-restarted")
	restarted, _, _ := startHostedFundsProcess(t, settings)
	assertHostedFundsBalance(t, database, funds, funds)
	winner := 0
	if statuses[1] == http.StatusOK {
		winner = 1
	}
	status, body, err := fundsProcessHTTP(restarted, fmt.Sprintf("process-%d", winner))
	if err != nil || status != http.StatusOK || body != "funded result" || calls.Load() != 1 {
		t.Fatalf("restart replay: status=%d body=%s error=%v provider=%d", status, body, err, calls.Load())
	}
	assertHostedFundsBalance(t, database, funds, funds)
	if limit != "" {
		status, body, err := fundsProcessHTTP(restarted, "new-after-tenant-limit-recovery")
		if err != nil || status != http.StatusPaymentRequired || calls.Load() != 1 {
			t.Fatalf("recovered tenant allowance: status=%d body=%s error=%v calls=%d", status, body, err, calls.Load())
		}
	}
}

func TestHostedFundsRecoveryAfterProcessInterruption(t *testing.T) {
	for _, stage := range []string{"before_dispatch", "after_dispatch"} {
		t.Run(stage, func(t *testing.T) {
			database, _, _, _ := newHostedRatingFixture(t)
			seedHostedFunds(t, database, 5)
			var source struct{ File string }
			if err := database.database.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&source).Error; err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int64
			reached := make(chan struct{}, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				if _, err := io.Copy(io.Discard, request.Body); err != nil {
					t.Error(err)
					return
				}
				reached <- struct{}{}
				<-request.Context().Done()
			}))
			t.Cleanup(upstream.Close)
			root := t.TempDir()
			settings := hostedFundsProcessSettings{DatabasePath: source.File, ResponseRoot: filepath.Join(root, "responses"), UpstreamURL: upstream.URL, ReadyPath: filepath.Join(root, "ready"), BoundaryPath: filepath.Join(root, "boundary"), Stage: stage, Now: ratingTestAcceptanceTime()}
			address, stop, boundary := startHostedFundsProcess(t, settings)
			finished := make(chan error, 1)
			go func() {
				status, body, err := fundsProcessHTTP(address, "process-interruption")
				if err == nil {
					err = fmt.Errorf("request returned before interruption: status=%d body=%s", status, body)
				}
				finished <- err
			}()
			if stage == "before_dispatch" {
				boundary()
			} else {
				select {
				case <-reached:
				case err := <-finished:
					t.Fatalf("request did not dispatch: %v", err)
				case <-time.After(10 * time.Second):
					t.Fatal("request did not reach provider")
				}
			}
			stop()
			if err := <-finished; err == nil {
				t.Fatal("interrupted request succeeded")
			}
			assertHostedFundsBalance(t, database, 5, 2)
			var request managedJournalRequestRecord
			if err := database.database.First(&request).Error; err != nil {
				t.Fatal(err)
			}
			settings.Stage, settings.ReadyPath, settings.Now = "", filepath.Join(root, "restarted"), request.ClaimExpiresAt.Add(time.Second)
			restarted, _, _ := startHostedFundsProcess(t, settings)
			wantStatus, wantAvailable, wantCalls, wantState := http.StatusBadGateway, int64(5), int64(0), fundsReservationReleased
			if stage == "after_dispatch" {
				wantStatus, wantAvailable, wantCalls, wantState = http.StatusConflict, 2, 1, fundsReservationReconciliation
			}
			status, body, err := fundsProcessHTTP(restarted, "process-interruption")
			if err != nil || status != wantStatus || calls.Load() != wantCalls {
				t.Fatalf("interrupted recovery: status=%d want=%d body=%s error=%v calls=%d", status, wantStatus, body, err, calls.Load())
			}
			assertHostedFundsBalance(t, database, 5, wantAvailable)
			var reservation managedFundsReservationRecord
			if err := database.database.First(&reservation).Error; err != nil {
				t.Fatal(err)
			}
			if reservation.State != wantState || reservation.Revision != 2 {
				t.Fatalf("recovered funds: state=%s revision=%d", reservation.State, reservation.Revision)
			}
		})
	}
}

func TestHostedFundsSettlementProcessInterruptionRollsBackDebit(t *testing.T) {
	database, _, _, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	var source struct{ File string }
	if err := database.database.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&source).Error; err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodDelete {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"id":"settlement-crash","status":"completed","output_text":"funded result","usage":{"input_tokens":1000,"output_tokens":1000,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	t.Cleanup(upstream.Close)
	root := t.TempDir()
	responseRoot := filepath.Join(root, "responses")
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, responseRoot, fundsDependencies(prices))
	hostedIdentityHTTP(t, server, "settlement-crash", "funded prompt", http.StatusOK)
	server.Close()
	boundaryPath := filepath.Join(root, "settlement-boundary")
	settings := hostedFundsProcessSettings{DatabasePath: source.File, ResponseRoot: responseRoot, UpstreamURL: upstream.URL, ReadyPath: boundaryPath, BoundaryPath: boundaryPath, Stage: "before_settlement_receipt", Now: ratingTestAcceptanceTime()}
	_, stop, _ := startHostedFundsProcess(t, settings)
	// Release and Spend have executed in the caller's uncommitted transaction.
	stop()
	assertHostedFundsBalance(t, database, 5, 2)
	for _, model := range []any{&managedFundsSettlementRecord{}, &managedChargeRecord{}} {
		var count int64
		if err := database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("interrupted %T retained %d records: %v", model, count, err)
		}
	}
	pending, err := database.pendingJournalDeliveries(t.Context(), 100)
	if err != nil || len(pending) != 1 {
		t.Fatalf("interrupted delivery=%v error=%v", pending, err)
	}
	settings.Stage, settings.ReadyPath = "", filepath.Join(root, "settlement-restarted")
	restarted, _, _ := startHostedFundsProcess(t, settings)
	status, body, err := fundsProcessHTTP(restarted, "settlement-crash")
	if err != nil || status != http.StatusOK || body != "funded result" || calls.Load() != 1 {
		t.Fatalf("settlement recovery: status=%d body=%s error=%v calls=%d", status, body, err, calls.Load())
	}
	assertHostedFundsBalance(t, database, 4, 4)
	var financial managedFundsAccountRecord
	if err := database.database.First(&financial).Error; err != nil {
		t.Fatal(err)
	}
	if financial.RemainderNumerator != "3" || financial.RemainderDenominator != "1000" {
		t.Fatalf("settlement remainder=%s/%s", financial.RemainderNumerator, financial.RemainderDenominator)
	}
}
