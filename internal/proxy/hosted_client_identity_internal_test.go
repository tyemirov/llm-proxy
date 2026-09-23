package proxy

import (
	"errors"
	"fmt"
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

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

func TestHostedClientIdentityAcrossLibraries(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"id":"private-library-result","status":"completed","output_text":"client result","usage":{"input_tokens":10,"output_tokens":3,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	t.Cleanup(upstream.Close)
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir())
	for _, protocol := range []llmproxyclient.Protocol{llmproxyclient.ProtocolNative, llmproxyclient.ProtocolOpenAIResponses} {
		t.Run(string(protocol), func(t *testing.T) {
			config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: hostedIdentityFixtureKey, Provider: "openai", Protocol: protocol})
			if err != nil {
				t.Fatal(err)
			}
			client, err := llmproxyclient.NewClient(config, http.DefaultClient)
			if err != nil {
				t.Fatal(err)
			}
			request, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "client prompt"}}, Model: "gpt-4.1", IdempotencyKey: "shared-client-key"})
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				result, err := client.PostMessages(t.Context(), request)
				if err != nil || result != "client result" {
					t.Fatalf("hosted Go client: result=%q error=%v", result, err)
				}
			}
		})
	}
	t.Run("cli", func(t *testing.T) {
		for range 2 {
			command := exec.CommandContext(t.Context(), "go", "run", "../../llm-proxy-client", "--base-url", server.URL, "--secret", hostedIdentityFixtureKey, "--provider", "openai", "--model", "gpt-4.1", "--prompt", "client prompt", "--idempotency-key", "shared-client-key")
			output, err := command.CombinedOutput()
			if err != nil || string(output) != "client result" {
				t.Fatalf("hosted CLI: output=%s error=%v", output, err)
			}
		}
	})
	t.Run("python", func(t *testing.T) {
		script := `import sys
from llm_proxy_client import Client, ClientConfig, ClientMessage, ClientMessagesRequest
client = Client(ClientConfig(base_url=sys.argv[1], secret=sys.argv[2], provider="openai"))
request = ClientMessagesRequest(messages=(ClientMessage(role="user", content="client prompt"),), model="gpt-4.1", idempotency_key="shared-client-key")
assert client.post_messages(request) == "client result"
assert client.post_messages(request) == "client result"
assert client.get_text_request("shared-client-key").state == "succeeded"
print("client result")
`
		command := hostedClientPythonCommand(t, script, server.URL, hostedIdentityFixtureKey)
		output, err := command.CombinedOutput()
		if err != nil || strings.TrimSpace(string(output)) != "client result" {
			t.Fatalf("hosted Python client: output=%s error=%v", output, err)
		}
	})
	if calls.Load() != 1 {
		t.Fatalf("client protocols repeated hosted work: %d", calls.Load())
	}
}

func TestHostedClientIdentityPendingAndExpired(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"id":"private-pending-client","status":"completed","output_text":"client result","usage":{"input_tokens":10,"output_tokens":3,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	t.Cleanup(upstream.Close)
	var responses *structuredRequestStore
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), func(dependencies *hostedTextRequestDependencies) { responses = dependencies.responses })
	t.Cleanup(unblock)
	request, err := http.NewRequest(http.MethodPost, server.URL+"/?provider=openai&model=gpt-4.1", strings.NewReader(`{"prompt":"client prompt"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "client-pending")
	finished := make(chan error, 1)
	go func() {
		response, err := server.Client().Do(request)
		if response != nil {
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				err = fmt.Errorf("initial status %d", response.StatusCode)
			}
		}
		finished <- err
	}()
	select {
	case <-started:
	case err := <-finished:
		t.Fatalf("provider did not start: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not start")
	}
	var accepted managedJournalRequestRecord
	if err := database.database.Where("key_digest = ?", sha256Hex("client-pending")).First(&accepted).Error; err != nil {
		t.Fatal(err)
	}
	for _, protocol := range []llmproxyclient.Protocol{llmproxyclient.ProtocolNative, llmproxyclient.ProtocolOpenAIResponses} {
		t.Run("pending_"+string(protocol), func(t *testing.T) {
			client, input := hostedIdentityGoClient(t, server.URL, protocol)
			text, err := client.PostMessages(t.Context(), input)
			var pending *llmproxyclient.StructuredRequestPendingError
			if text != "" || !errors.As(err, &pending) || pending.Snapshot().ProxyRequestID != accepted.ExecutionID {
				t.Fatalf("pending Go result: text=%q error=%v", text, err)
			}
			status, err := client.GetStructuredRequest(t.Context(), "client-pending")
			if err != nil || status.State != "dispatched" || status.ProxyRequestID != accepted.ExecutionID {
				t.Fatalf("pending Go status: %+v error=%v", status, err)
			}
		})
	}
	t.Run("pending_python", func(t *testing.T) {
		script := `import sys
from llm_proxy_client import Client, ClientConfig, ClientMessage, ClientMessagesRequest
client = Client(ClientConfig(base_url=sys.argv[1], secret=sys.argv[2], provider="openai"))
request = ClientMessagesRequest(messages=(ClientMessage(role="user", content="client prompt"),), model="gpt-4.1", idempotency_key="client-pending")
try:
    client.post_messages(request)
except RuntimeError as error:
    assert type(error).__name__ == "LLMProxyRequestPendingError", repr(error)
    assert error.snapshot.proxy_request_id == sys.argv[3]
else:
    raise AssertionError("pending response was returned as generated text")
status = client.get_text_request("client-pending")
assert status.state == "dispatched" and status.proxy_request_id == sys.argv[3]
`
		command := hostedClientPythonCommand(t, script, server.URL, hostedIdentityFixtureKey, accepted.ExecutionID)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("pending Python result: %s error=%v", output, err)
		}
	})
	t.Run("pending_cli", func(t *testing.T) {
		command := exec.CommandContext(t.Context(), "go", "run", "../../llm-proxy-client", "--base-url", server.URL, "--secret", hostedIdentityFixtureKey, "--provider", "openai", "--model", "gpt-4.1", "--prompt", "client prompt", "--idempotency-key", "client-pending")
		output, err := command.CombinedOutput()
		if err == nil || !strings.Contains(string(output), "structured_request_pending: state=dispatched") {
			t.Fatalf("pending CLI result: %s error=%v", output, err)
		}
	})
	unblock()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	responses.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	t.Run("expired_python", func(t *testing.T) {
		script := `import sys
from llm_proxy_client import Client, ClientConfig, ClientMessage, ClientMessagesRequest, LLMProxyHTTPError
client = Client(ClientConfig(base_url=sys.argv[1], secret=sys.argv[2], provider="openai"))
request = ClientMessagesRequest(messages=(ClientMessage(role="user", content="client prompt"),), model="gpt-4.1", idempotency_key="client-pending")
for operation in (lambda: client.post_messages(request), lambda: client.get_text_request("client-pending")):
    try:
        operation()
    except LLMProxyHTTPError as error:
        assert error.status_code == 410 and error.proxy_error_code == "hosted_result_expired", repr(error)
    else:
        raise AssertionError("expired request did not retain its failure")
`
		if output, err := hostedClientPythonCommand(t, script, server.URL, hostedIdentityFixtureKey).CombinedOutput(); err != nil {
			t.Fatalf("expired Python result: %s error=%v", output, err)
		}
	})
	for _, protocol := range []llmproxyclient.Protocol{llmproxyclient.ProtocolNative, llmproxyclient.ProtocolOpenAIResponses} {
		t.Run("expired_"+string(protocol), func(t *testing.T) {
			client, input := hostedIdentityGoClient(t, server.URL, protocol)
			_, err := client.PostMessages(t.Context(), input)
			var failure *llmproxyclient.HTTPFailure
			if !errors.As(err, &failure) || failure.StatusCode() != http.StatusGone || failure.ProxyErrorCode() != "hosted_result_expired" {
				t.Fatalf("expired Go result: %v", err)
			}
		})
	}
	if calls.Load() != 1 {
		t.Fatalf("client reconciliation dispatched %d times", calls.Load())
	}
}

func hostedClientPythonCommand(t *testing.T, script string, arguments ...string) *exec.Cmd {
	t.Helper()
	root, err := filepath.Abs("../../python")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "python3", append([]string{"-c", script}, arguments...)...)
	command.Env = append(os.Environ(), "PYTHONPATH="+root)
	return command
}

func hostedIdentityGoClient(t *testing.T, address string, protocol llmproxyclient.Protocol) (llmproxyclient.Client, llmproxyclient.MessagesRequest) {
	t.Helper()
	config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: address, Secret: hostedIdentityFixtureKey, Provider: "openai", Protocol: protocol})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(config, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	input, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "client prompt"}}, Model: "gpt-4.1", IdempotencyKey: "client-pending"})
	if err != nil {
		t.Fatal(err)
	}
	return client, input
}
