package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
)

const hostedRecoveryProcessInput = "LLM_PROXY_HOSTED_RECOVERY_PROCESS_INPUT"

type hostedRecoveryProcessSettings struct {
	Kind         journalExecutionKind
	DatabasePath string
	ResponseRoot string
	UpstreamURL  string
	Stage        string
	ReadyPath    string
	BoundaryPath string
}

func TestHostedTextRecoveryProcess(t *testing.T) {
	input := os.Getenv(hostedRecoveryProcessInput)
	if input == "" {
		t.Skip("subprocess fixture")
	}
	var settings hostedRecoveryProcessSettings
	if err := json.Unmarshal([]byte(input), &settings); err != nil {
		t.Fatal(err)
	}
	database, err := newGORMManagedTenantDatabase(ManagementConfiguration{DatabasePath: settings.DatabasePath}, internalManagedProviderKeyCipher(), internalManagementProviderRegistry())
	if err != nil {
		t.Fatal(err)
	}
	server := newHostedRecoveryHTTPServer(t, settings.Kind, database, settings.UpstreamURL, settings.ResponseRoot, func(service *hostedTextRequestDependencies) {
		if settings.Stage == "after_completion" || settings.Stage == "after_publication" {
			publish := service.responses.publish
			service.responses.publish = func(path string, record structuredRequestRecord) error {
				if record.State != structuredRequestStateSucceeded {
					return publish(path, record)
				}
				if settings.Stage == "after_publication" {
					if err := publish(path, record); err != nil {
						return err
					}
				}
				writeHostedRecoverySignal(t, settings.BoundaryPath, "stopped")
				<-t.Context().Done()
				return t.Context().Err()
			}
			return
		}
		if settings.Stage == "after_dispatch" {
			return
		}
		var stopped atomic.Bool
		err := service.database.database.Callback().Update().Before("gorm:update").Register("hosted_recovery_boundary", func(transaction *gorm.DB) {
			changes, ok := transaction.Statement.Dest.(map[string]any)
			if !ok {
				return
			}
			state := fmt.Sprint(changes["state"])
			boundary := settings.Stage == "before_dispatch" && transaction.Statement.Table == "managed_journal_attempt_records" && state == string(journalAttemptDispatched)
			boundary = boundary || settings.Stage == "after_observation" && transaction.Statement.Table == "managed_journal_request_records" && state == string(journalRequestCompleted)
			if boundary && stopped.CompareAndSwap(false, true) {
				writeHostedRecoverySignal(t, settings.BoundaryPath, "stopped")
				<-t.Context().Done()
			}
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	writeHostedRecoverySignal(t, settings.ReadyPath, server.URL)
	<-t.Context().Done()
}

func TestHostedTextRecoveryAfterProcessInterruption(t *testing.T) {
	testHostedCompletionRecovery(t, journalExecutionText)
}

func TestHostedDictationRecoveryAfterProcessInterruption(t *testing.T) {
	testHostedCompletionRecovery(t, journalExecutionDictation)
}

func newHostedRecoveryHTTPServer(t *testing.T, kind journalExecutionKind, database *gormManagedTenantDatabase, upstreamURL, root string, configure ...func(*hostedTextRequestDependencies)) *httptest.Server {
	t.Helper()
	switch kind {
	case journalExecutionText:
		return newHostedIdentityHTTPServer(t, database, upstreamURL, root, configure...)
	case journalExecutionDictation:
		return newHostedDictationServer(t, database, upstreamURL, root, configure...)
	default:
		t.Fatalf("unknown recovery kind %s", kind)
		return nil
	}
}

func testHostedCompletionRecovery(t *testing.T, kind journalExecutionKind) {
	t.Helper()
	for _, stage := range []string{"before_dispatch", "after_dispatch", "after_observation", "after_completion", "after_publication"} {
		t.Run(stage, func(t *testing.T) {
			database, _, read := newJournalTransactionFixture(t)
			if kind == journalExecutionDictation {
				grantHostedDictation(t, database)
			}
			var source struct{ File string }
			if err := database.database.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&source).Error; err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			var calls atomic.Int64
			reached := make(chan struct{}, 1)
			release := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if _, err := io.Copy(io.Discard, request.Body); err != nil {
					t.Error(err)
					return
				}
				ordinal := calls.Add(1)
				if stage == "after_dispatch" && ordinal == 1 {
					reached <- struct{}{}
					select {
					case <-request.Context().Done():
					case <-release:
					}
					return
				}
				writer.Header().Set("Content-Type", "application/json")
				if kind == journalExecutionDictation {
					fmt.Fprint(writer, `{"text":"recovered result","usage":{"type":"duration","seconds":1.25}}`)
					return
				}
				fmt.Fprint(writer, `{"id":"private-recovery","status":"completed","output_text":"recovered result","usage":{"input_tokens":10,"output_tokens":3,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
			}))
			t.Cleanup(upstream.Close)
			t.Cleanup(func() { close(release) })
			settings := hostedRecoveryProcessSettings{Kind: kind, DatabasePath: source.File, ResponseRoot: filepath.Join(root, "responses"), UpstreamURL: upstream.URL, Stage: stage, ReadyPath: filepath.Join(root, "ready"), BoundaryPath: filepath.Join(root, "boundary")}
			encoded, err := json.Marshal(settings)
			if err != nil {
				t.Fatal(err)
			}
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			command := exec.CommandContext(t.Context(), executable, "-test.run=^TestHostedTextRecoveryProcess$", "-test.v")
			command.Env = append(os.Environ(), hostedRecoveryProcessInput+"="+string(encoded))
			var output bytes.Buffer
			command.Stdout, command.Stderr = &output, &output
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			stopped := false
			stop := func() {
				if stopped {
					return
				}
				stopped = true
				if err := command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
					t.Errorf("stop service: %v", err)
				}
				var exit *exec.ExitError
				if err := command.Wait(); !errors.As(err, &exit) {
					t.Errorf("service did not stop at the boundary: %v output=%s", err, output.String())
				}
			}
			t.Cleanup(stop)
			serverURL := waitHostedRecoverySignal(t, settings.ReadyPath, stop, &output)
			path := "/?key=" + hostedIdentityFixtureKey + "&provider=openai&model=gpt-4.1"
			var body io.Reader = strings.NewReader(`{"prompt":"interrupted prompt"}`)
			contentType := "application/json"
			if kind == journalExecutionDictation {
				var audio bytes.Buffer
				form := multipart.NewWriter(&audio)
				file, err := form.CreateFormFile(formFieldAudio, "audio.wav")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := io.WriteString(file, "interrupted audio"); err != nil {
					t.Fatal(err)
				}
				if err := form.Close(); err != nil {
					t.Fatal(err)
				}
				path = dictatePath + "?key=" + hostedIdentityFixtureKey + "&provider=openai&model=gpt-transcribe"
				body, contentType = &audio, form.FormDataContentType()
			}
			request, err := http.NewRequest(http.MethodPost, serverURL+path, body)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", contentType)
			request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "interrupted")
			finished := make(chan error, 1)
			go func() {
				response, err := http.DefaultClient.Do(request)
				if response != nil {
					body, readErr := io.ReadAll(response.Body)
					response.Body.Close()
					err = fmt.Errorf("service returned before interruption: status=%d body=%s read=%v", response.StatusCode, body, readErr)
				}
				finished <- err
			}()
			if stage == "after_dispatch" {
				select {
				case <-reached:
				case err := <-finished:
					t.Fatalf("service did not dispatch: %v", err)
				case <-time.After(10 * time.Second):
					t.Fatal("provider dispatch did not start")
				}
			} else {
				waitHostedRecoverySignal(t, settings.BoundaryPath, stop, &output)
			}
			stop()
			if err := <-finished; err == nil {
				t.Fatal("killed service completed the HTTP request")
			}
			var original managedJournalRequestRecord
			if err := database.database.Where("key_digest = ?", sha256Hex("interrupted")).First(&original).Error; err != nil {
				t.Fatal(err)
			}
			var originalAttempt managedJournalAttemptRecord
			if err := database.database.Where("request_id = ?", original.ID).First(&originalAttempt).Error; err != nil {
				t.Fatal(err)
			}
			recoveryNow := original.ClaimExpiresAt.Add(time.Second)
			restarted := newHostedRecoveryHTTPServer(t, kind, openJournalTransactionInstance(t, database), upstream.URL, settings.ResponseRoot, func(service *hostedTextRequestDependencies) {
				service.now = func() time.Time { return recoveryNow }
			})
			replay := func(want int) {
				if kind == journalExecutionDictation {
					hostedDictationHTTP(t, restarted, transcriptionsPath, "interrupted", "interrupted audio", want)
				} else {
					hostedIdentityHTTP(t, restarted, "interrupted", "interrupted prompt", want)
				}
			}
			if stage == "before_dispatch" || stage == "after_publication" {
				replay(http.StatusOK)
				hostedIdentityStatusHTTP(t, restarted, "interrupted", http.StatusOK)
			} else {
				hostedIdentityStatusHTTP(t, restarted, "interrupted", http.StatusConflict)
				replay(http.StatusConflict)
			}
			if calls.Load() != 1 {
				t.Fatalf("recovery repeated paid work: calls=%d", calls.Load())
			}
			state := read("/" + original.ID)
			if state["execution_id"] != original.ExecutionID {
				t.Fatalf("recovery replaced execution: %v", state)
			}
			var attempts []managedJournalAttemptRecord
			if err := database.database.Where("request_id = ?", original.ID).Find(&attempts).Error; err != nil || len(attempts) != 1 || attempts[0].ID != originalAttempt.ID {
				t.Fatalf("recovery replaced attempt: %v error=%v", attempts, err)
			}
			if (stage == "after_observation" || stage == "after_completion") && (state["state"] != "uncertain" || state["usage_state"] != "complete") {
				t.Fatalf("recovery discarded known usage: %v", state)
			}
			if stage == "after_dispatch" && (state["state"] != "uncertain" || state["usage_state"] != "unknown") {
				t.Fatalf("recovery lost unknown usage: %v", state)
			}
			if stage == "after_completion" {
				cases := read("/" + original.ID + "/reconciliation-cases")["cases"].([]any)
				if len(cases) != 1 || cases[0].(map[string]any)["reason"] != journalCaseResultUnknown || cases[0].(map[string]any)["state"] != "open" {
					t.Fatalf("result recovery case unavailable: %v", cases)
				}
			}
		})
	}
}

func TestHostedTextRecoveryFencesReplacedWorker(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	var calls, reservations atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"id":"private-new-owner","status":"completed","output_text":"current result","usage":{"input_tokens":10,"output_tokens":3,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	t.Cleanup(upstream.Close)
	root := t.TempDir()
	entered, release := make(chan struct{}), make(chan struct{})
	var stopped atomic.Bool
	if err := database.database.Callback().Query().After("gorm:query").Register("pause_original_worker", func(transaction *gorm.DB) {
		if transaction.Statement.Table == "managed_platform_credential_records" && hostedTextExecutionFromContext(transaction.Statement.Context) != nil && stopped.CompareAndSwap(false, true) {
			close(entered)
			select {
			case <-release:
			case <-t.Context().Done():
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	reserve := func(*gorm.DB, managedJournalRequestRecord) error { reservations.Add(1); return nil }
	first := newHostedIdentityHTTPServer(t, database, upstream.URL, root, func(service *hostedTextRequestDependencies) {
		service.authorize = reserve
	})
	t.Cleanup(func() { close(release) })
	request, err := http.NewRequest(http.MethodPost, first.URL+"/?provider=openai&model=gpt-4.1", strings.NewReader(`{"prompt":"same intent"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "replaced")
	finished := make(chan error, 1)
	go func() {
		response, err := first.Client().Do(request)
		if err == nil {
			body, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if readErr != nil {
				err = readErr
			} else if response.StatusCode != http.StatusConflict || !bytes.Contains(body, []byte("journal_claim_lost")) {
				err = fmt.Errorf("replaced worker response: status=%d body=%s", response.StatusCode, body)
			}
		}
		finished <- err
	}()
	select {
	case <-entered:
	case err := <-finished:
		t.Fatalf("worker did not reach credential load: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not reach credential load")
	}
	var original managedJournalRequestRecord
	if err := database.database.Where("key_digest = ?", sha256Hex("replaced")).First(&original).Error; err != nil {
		t.Fatal(err)
	}
	second := newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, database), upstream.URL, root, func(service *hostedTextRequestDependencies) {
		service.authorize = reserve
		service.now = func() time.Time { return original.ClaimExpiresAt.Add(time.Second) }
	})
	hostedIdentityHTTP(t, second, "replaced", "same intent", http.StatusOK)
	// Permit the old worker to proceed only after its replacement has published.
	release <- struct{}{}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	hostedIdentityHTTP(t, second, "replaced", "same intent", http.StatusOK)
	hostedIdentityStatusHTTP(t, second, "replaced", http.StatusOK)
	if calls.Load() != 1 || reservations.Load() != 2 {
		t.Fatalf("claim replacement repeated effects: provider=%d reservations=%d", calls.Load(), reservations.Load())
	}
	var current managedJournalRequestRecord
	if err := database.database.Where("id = ?", original.ID).First(&current).Error; err != nil {
		t.Fatal(err)
	}
	if current.OwnerToken == original.OwnerToken || current.ExecutionID != original.ExecutionID || current.ResultPublishedAt == nil || current.State != journalRequestCompleted {
		t.Fatalf("replaced worker changed the accepted result: %+v", current)
	}
}

func TestHostedTextRecoveryAfterResultWriteFailure(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	var calls atomic.Int64
	var now atomic.Int64
	now.Store(time.Now().UnixNano())
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"id":"private-write-failure","status":"completed","output_text":"lost result","usage":{"input_tokens":10,"output_tokens":3,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	t.Cleanup(upstream.Close)
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), func(service *hostedTextRequestDependencies) {
		service.now = func() time.Time { return time.Unix(0, now.Load()) }
		publish := service.responses.publish
		service.responses.publish = func(path string, record structuredRequestRecord) error {
			if record.State == structuredRequestStateSucceeded {
				return errors.New("injected result write failure")
			}
			return publish(path, record)
		}
	})
	hostedIdentityHTTP(t, server, "write-failed", "same intent", http.StatusBadGateway)
	var accepted managedJournalRequestRecord
	if err := database.database.Where("key_digest = ?", sha256Hex("write-failed")).First(&accepted).Error; err != nil {
		t.Fatal(err)
	}
	now.Store(accepted.ClaimExpiresAt.Add(time.Second).UnixNano())
	hostedIdentityHTTP(t, server, "write-failed", "same intent", http.StatusConflict)
	hostedIdentityStatusHTTP(t, server, "write-failed", http.StatusConflict)
	if calls.Load() != 1 {
		t.Fatalf("failed result storage repeated provider work: %d", calls.Load())
	}
}

func writeHostedRecoverySignal(t *testing.T, path, value string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := io.WriteString(file, value)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		t.Fatal(err)
	}
}

func waitHostedRecoverySignal(t *testing.T, path string, stop func(), output *bytes.Buffer) string {
	t.Helper()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		value, err := os.ReadFile(path)
		if err == nil && len(value) > 0 {
			return string(value)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		select {
		case <-ticker.C:
		case <-timer.C:
			stop()
			t.Fatalf("service did not reach %s: %s", filepath.Base(path), output.String())
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		}
	}
}
