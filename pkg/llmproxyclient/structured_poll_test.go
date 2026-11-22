package llmproxyclient_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

const pollPendingBody = `{"state":"dispatched","proxy_request_id":"proxy-pending","started_at":"2026-11-22T00:00:00Z","updated_at":"2026-11-22T00:00:01Z","elapsed_seconds":1}`

func TestStructuredRequestPollAtSchedule(testingInstance *testing.T) {
	now := time.Date(2026, time.November, 22, 0, 0, 0, 0, time.UTC)
	for _, example := range []struct {
		attempt uint64
		delay   time.Duration
	}{{0, 10 * time.Second}, {1, 10 * time.Second}, {2, 20 * time.Second}, {3, 40 * time.Second}, {4, time.Minute}, {5, time.Minute}, {math.MaxUint64, time.Minute}} {
		if got := llmproxyclient.StructuredRequestPollAt(now, example.attempt, llmproxyclient.StructuredRequestResult{}, nil); !got.Equal(now.Add(example.delay)) {
			testingInstance.Fatalf("attempt=%d deadline=%v want=%v", example.attempt, got, now.Add(example.delay))
		}
	}
}

func TestStructuredRequestRetryAfterHTTPBoundaries(testingInstance *testing.T) {
	now := time.Date(2026, time.November, 22, 0, 0, 0, 0, time.UTC)
	for _, example := range []struct {
		name   string
		header string
		delay  time.Duration
	}{
		{"missing", "", 0},
		{"short", "2", 2 * time.Second},
		{"above cap", "120", 120 * time.Second},
		{"HTTP date", now.Add(3 * time.Minute).Format(http.TimeFormat), 3 * time.Minute},
		{"past date", now.Add(-time.Minute).Format(http.TimeFormat), 0},
		{"current date", now.Format(http.TimeFormat), 0},
		{"zero seconds", "0", 0},
		{"malformed", "private-invalid-header", 0},
		{"negative", "-1", 0},
		{"signed", "+10", 0},
		{"fractional", "1.5", 0},
		{"duration overflow", "9223372037", time.Duration(math.MaxInt64)},
		{"integer overflow", "18446744073709551616", time.Duration(math.MaxInt64)},
		{"huge invalid", strings.Repeat("9", 100) + "x", 0},
	} {
		testingInstance.Run(example.name, func(subTest *testing.T) {
			for _, status := range []int{http.StatusAccepted, http.StatusTooManyRequests} {
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					calls++
					if request.Method != http.MethodGet || request.URL.Path != "/v2/requests" || request.Header.Get(llmproxycontract.HeaderIdempotencyKey) != "original-key" {
						subTest.Error("the reader changed the original GET identity")
					}
					writer.Header().Set("Retry-After", example.header)
					writer.WriteHeader(status)
					if status == http.StatusAccepted {
						_, _ = writer.Write([]byte(pollPendingBody))
					}
				}))
				client := pollTestClient(subTest, server)
				result, err := client.GetStructuredRequest(context.Background(), "original-key")
				want := time.Time{}
				if example.delay != 0 {
					want = now.Add(example.delay)
				}
				if status == http.StatusAccepted {
					if err != nil || result.State != "dispatched" || !result.RetryAfter(now).Equal(want) {
						subTest.Fatalf("pending retry=%v want=%v err=%v", result.RetryAfter(now), want, err)
					}
				} else {
					var failure *llmproxyclient.HTTPFailure
					if !errors.As(err, &failure) || !errors.Is(err, llmproxyclient.ErrClientHTTPFailure) || failure.StatusCode() != status || failure.ProxyErrorCode() != "" || !failure.RetryAfter(now).Equal(want) {
						subTest.Fatalf("empty edge failure retry=%v want=%v error=%v", failure, want, err)
					}
					if strings.Contains(err.Error(), example.header) && example.header != "" && example.header != "2" && example.header != "0" {
						subTest.Fatal("HTTP error exposed untrusted response metadata")
					}
					err = fmt.Errorf("read original request: %w", err)
				}
				deadline := llmproxyclient.StructuredRequestPollAt(now, 4, result, err)
				minimum := now.Add(time.Minute)
				if want.After(minimum) {
					minimum = want
				}
				if !deadline.Equal(minimum) || calls != 1 {
					subTest.Fatalf("poll=%v want=%v HTTP calls=%d", deadline, minimum, calls)
				}
				server.Close()
			}
		})
	}
}

func TestStructuredRequestPostRetryAfterAndPendingError(testingInstance *testing.T) {
	now := time.Date(2026, time.November, 22, 0, 0, 0, 0, time.UTC)
	for _, status := range []int{http.StatusAccepted, http.StatusTooManyRequests} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			calls++
			if request.Method != http.MethodPost || request.Header.Get(llmproxycontract.HeaderIdempotencyKey) != "original-key" {
				testingInstance.Error("the caller's POST identity changed")
			}
			writer.Header().Set("Retry-After", "120")
			writer.WriteHeader(status)
			if status == http.StatusAccepted {
				_, _ = writer.Write([]byte(pollPendingBody))
			}
		}))
		client := pollTestClient(testingInstance, server)
		request, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "controlled prompt"}}, IdempotencyKey: "original-key"})
		if err != nil {
			testingInstance.Fatal(err)
		}
		_, err = client.PostMessages(context.Background(), request)
		if status == http.StatusAccepted {
			var pending *llmproxyclient.StructuredRequestPendingError
			if !errors.As(err, &pending) || !errors.Is(err, llmproxyclient.ErrStructuredRequestPending) || !pending.Snapshot().RetryAfter(now).Equal(now.Add(2*time.Minute)) {
				testingInstance.Fatalf("POST pending metadata unavailable: %v", err)
			}
		} else {
			var failure *llmproxyclient.HTTPFailure
			if !errors.As(err, &failure) || !failure.RetryAfter(now).Equal(now.Add(2*time.Minute)) {
				testingInstance.Fatalf("POST completed failure metadata unavailable: %v", err)
			}
		}
		if deadline := llmproxyclient.StructuredRequestPollAt(now, 1, llmproxyclient.StructuredRequestResult{}, fmt.Errorf("wrapped: %w", err)); !deadline.Equal(now.Add(2*time.Minute)) || calls != 1 {
			testingInstance.Fatalf("wrapped POST deadline=%v calls=%d", deadline, calls)
		}
		server.Close()
	}
}

func TestStructuredRequestPostRetryAfterOnInvalidResponse(testingInstance *testing.T) {
	now := time.Date(2026, time.November, 22, 0, 0, 0, 0, time.UTC)
	for _, example := range []struct {
		name      string
		status    int
		truncated bool
	}{
		{"truncated body", http.StatusTooManyRequests, true},
		{"malformed pending status", http.StatusAccepted, false},
	} {
		testingInstance.Run(example.name, func(subTest *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodPost || request.Header.Get(llmproxycontract.HeaderIdempotencyKey) != "original-key" {
					subTest.Error("the caller's POST identity changed")
				}
				writer.Header().Set("Retry-After", "120")
				if example.truncated {
					writer.Header().Set("Content-Length", "1000")
				}
				writer.WriteHeader(example.status)
				_, _ = writer.Write([]byte(`{"private_detail":"private-output"}`))
			}))
			client := pollTestClient(subTest, server)
			request, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{
				Messages:       []llmproxyclient.MessageInput{{Role: "user", Content: "controlled prompt"}},
				IdempotencyKey: "original-key",
			})
			if err != nil {
				subTest.Fatal(err)
			}
			output, err := client.PostMessages(context.Background(), request)
			var failure *llmproxyclient.HTTPFailure
			if !errors.Is(err, llmproxyclient.ErrClientHTTPFailure) || errors.As(err, &failure) ||
				errors.Is(err, llmproxyclient.ErrStructuredRequestPending) || output != "" || strings.Contains(err.Error(), "private-output") {
				subTest.Fatalf("invalid POST response was misclassified: output=%q err=%v", output, err)
			}
			wrapped := fmt.Errorf("submit original request: %w", err)
			if deadline := llmproxyclient.StructuredRequestPollAt(now, 1, llmproxyclient.StructuredRequestResult{}, wrapped); !deadline.Equal(now.Add(2 * time.Minute)) {
				subTest.Fatalf("invalid POST response lost its retry deadline: %v", deadline)
			}
		})
	}
}

func pollTestClient(testingInstance *testing.T, server *httptest.Server) llmproxyclient.Client {
	testingInstance.Helper()
	testingInstance.Cleanup(server.Close)
	configuration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "controlled-tenant-key"})
	if err != nil {
		testingInstance.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(configuration, server.Client())
	if err != nil {
		testingInstance.Fatal(err)
	}
	return client
}

func TestClientRetryAfterCompletedFailureRoutes(testingInstance *testing.T) {
	now := time.Date(2026, time.November, 22, 0, 0, 0, 0, time.UTC)
	for _, example := range []struct {
		name string
		call func(llmproxyclient.Client) error
	}{
		{"capabilities", func(client llmproxyclient.Client) error {
			_, err := client.GetPublicCapabilities(context.Background())
			return err
		}},
		{"asset upload", func(client llmproxyclient.Client) error {
			_, err := client.UploadAsset(context.Background(), llmproxyclient.AssetUploadInput{MIMEType: "image/png", Data: []byte("controlled image bytes")})
			return err
		}},
		{"media resource", func(client llmproxyclient.Client) error {
			_, err := client.GetAsset(context.Background(), "ast_0123456789abcdef0123456789abcdef")
			return err
		}},
		{"asset download", func(client llmproxyclient.Client) error {
			_, err := client.DownloadAsset(context.Background(), llmproxyclient.Asset{AssetID: "ast_0123456789abcdef0123456789abcdef", MIMEType: "image/png", SizeBytes: 4, State: "available", CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
			return err
		}},
		{"System One", func(client llmproxyclient.Client) error {
			_, err := client.PostSystemOne(context.Background(), []byte(`{}`), llmproxyclient.SystemOneRequestOptions{})
			return err
		}},
	} {
		testingInstance.Run(example.name, func(subTest *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				calls++
				writer.Header().Set("Retry-After", "120")
				writer.WriteHeader(http.StatusTooManyRequests)
			}))
			err := example.call(pollTestClient(subTest, server))
			var failure *llmproxyclient.HTTPFailure
			if !errors.As(err, &failure) || !failure.RetryAfter(now).Equal(now.Add(2*time.Minute)) || calls != 1 {
				subTest.Fatalf("completed HTTP failure retry metadata lost: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestStructuredRequestRetryAfterErrorSnapshotAndReadFault(testingInstance *testing.T) {
	now := time.Date(2026, time.November, 22, 0, 0, 0, 0, time.UTC)
	for _, readFault := range []bool{false, true} {
		testingInstance.Run(fmt.Sprintf("read fault=%t", readFault), func(subTest *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				calls++
				writer.Header().Set("Retry-After", "120")
				if readFault {
					writer.Header().Set("Content-Length", "1000")
				}
				writer.WriteHeader(http.StatusConflict)
				_, _ = writer.Write([]byte(`{"error":{"code":"structured_request_outcome_unknown","state":"outcome_unknown","proxy_request_id":"proxy-existing","cause":"request_timeout","private_detail":"private-output"}}`))
			}))
			result, err := pollTestClient(subTest, server).GetStructuredRequest(context.Background(), "original-key")
			if !errors.Is(err, llmproxyclient.ErrClientHTTPFailure) || !result.RetryAfter(now).Equal(now.Add(2*time.Minute)) || calls != 1 {
				subTest.Fatalf("error snapshot lost safe timing: retry=%v calls=%d err=%v", result.RetryAfter(now), calls, err)
			}
			var failure *llmproxyclient.HTTPFailure
			if readFault {
				if errors.As(err, &failure) || result.State != "" || result.Output != "" {
					subTest.Fatal("a failed body read fabricated completed state or exposed output")
				}
			} else if !errors.As(err, &failure) || result.State != "outcome_unknown" || result.ProxyRequestID != "proxy-existing" || failure.ProxyErrorCode() != "structured_request_outcome_unknown" {
				subTest.Fatal("completed uncertain response lost its typed classification")
			}
			if strings.Contains(err.Error(), "private-output") || !llmproxyclient.StructuredRequestPollAt(now, 1, result, nil).Equal(now.Add(2*time.Minute)) {
				subTest.Fatal("consuming the classified error lost timing or exposed the response")
			}
		})
	}
}
