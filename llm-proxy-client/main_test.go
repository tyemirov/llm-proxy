package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
)

type capturedProxyRequest struct {
	method      string
	path        string
	contentType string
	accept      string
	body        string
}

type failingHTTPDoer struct {
	err error
}

func (httpDoer failingHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	return nil, httpDoer.err
}

type readFailHTTPDoer struct{}

func (httpDoer readFailHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: failingReadCloser{}}, nil
}

type failingReadCloser struct{}

func (failingReadCloser) Read(buffer []byte) (int, error) {
	return 0, errors.New("read failed")
}

func (failingReadCloser) Close() error {
	return nil
}

type failingReader struct{}

func (failingReader) Read(buffer []byte) (int, error) {
	return 0, errors.New("stdin failed")
}

type failingWriter struct{}

func (failingWriter) Write(buffer []byte) (int, error) {
	return 0, errors.New("stdout failed")
}

func TestCommandPostsPromptAsJSONBody(t *testing.T) {
	capturedRequest := capturedProxyRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		bodyBytes, readError := io.ReadAll(httpRequest.Body)
		if readError != nil {
			t.Fatalf("read body: %v", readError)
		}
		capturedRequest = capturedProxyRequest{
			method:      httpRequest.Method,
			path:        httpRequest.URL.RequestURI(),
			contentType: httpRequest.Header.Get("Content-Type"),
			accept:      httpRequest.Header.Get("Accept"),
			body:        string(bodyBytes),
		}
		responseWriter.WriteHeader(http.StatusOK)
		_, _ = responseWriter.Write([]byte("reviewed"))
	}))
	t.Cleanup(server.Close)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run(
		[]string{
			"--base-url", server.URL + "/review?prompt=old&model=old&max_tokens=9&web_search=true&provider=gemini&keep=1",
			"--secret", "test-secret",
			"--model", " 5.5 ",
			"--prompt", "Проверить текст",
		},
		strings.NewReader(""),
		&stdout,
		&stderr,
		defaultHTTPClientFactory,
	)

	if exitCode != 0 {
		t.Fatalf("exit=%d stderr=%s", exitCode, stderr.String())
	}
	if stdout.String() != "reviewed" {
		t.Fatalf("stdout=%q", stdout.String())
	}
	if capturedRequest.method != http.MethodPost {
		t.Fatalf("method=%q", capturedRequest.method)
	}
	if capturedRequest.contentType != "application/json; charset=utf-8" {
		t.Fatalf("contentType=%q", capturedRequest.contentType)
	}
	if capturedRequest.accept != "text/plain" {
		t.Fatalf("accept=%q", capturedRequest.accept)
	}
	parsedRequestURL, parseError := url.Parse(capturedRequest.path)
	if parseError != nil {
		t.Fatalf("parse request path: %v", parseError)
	}
	queryValues := parsedRequestURL.Query()
	if queryValues.Get("key") != "test-secret" {
		t.Fatalf("key=%q", queryValues.Get("key"))
	}
	if queryValues.Get("format") != "text/plain" {
		t.Fatalf("format=%q", queryValues.Get("format"))
	}
	if queryValues.Get("provider") != "gemini" {
		t.Fatalf("provider=%q", queryValues.Get("provider"))
	}
	if queryValues.Get("keep") != "1" {
		t.Fatalf("keep=%q", queryValues.Get("keep"))
	}
	for _, removedQueryKey := range []string{"prompt", "model", "max_tokens", "web_search"} {
		if queryValues.Has(removedQueryKey) {
			t.Fatalf("query key %s should have been removed", removedQueryKey)
		}
	}
	for _, expectedBodyFragment := range []string{
		`"prompt":"Проверить текст"`,
		`"model":"5.5"`,
		`"web_search":false`,
	} {
		if !strings.Contains(capturedRequest.body, expectedBodyFragment) {
			t.Fatalf("body=%s missing %s", capturedRequest.body, expectedBodyFragment)
		}
	}
}

func TestCommandReadsEnvironmentAndStdin(t *testing.T) {
	capturedRequest := capturedProxyRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		bodyBytes, readError := io.ReadAll(httpRequest.Body)
		if readError != nil {
			t.Fatalf("read body: %v", readError)
		}
		capturedRequest = capturedProxyRequest{path: httpRequest.URL.RequestURI(), body: string(bodyBytes)}
		responseWriter.WriteHeader(http.StatusOK)
		_, _ = responseWriter.Write([]byte("stdin-ok"))
	}))
	t.Cleanup(server.Close)
	t.Setenv("LLM_PROXY_BASE_URL", server.URL)
	t.Setenv("LLM_PROXY_SECRET", "env-secret")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{}, strings.NewReader("stdin prompt"), &stdout, &stderr, defaultHTTPClientFactory)

	if exitCode != 0 {
		t.Fatalf("exit=%d stderr=%s", exitCode, stderr.String())
	}
	if stdout.String() != "stdin-ok" {
		t.Fatalf("stdout=%q", stdout.String())
	}
	if !strings.Contains(capturedRequest.path, "key=env-secret") {
		t.Fatalf("path=%q", capturedRequest.path)
	}
	if !strings.Contains(capturedRequest.body, `"prompt":"stdin prompt"`) {
		t.Fatalf("body=%s", capturedRequest.body)
	}
}

func TestCommandReadsPromptFileAndOptionalBodyFields(t *testing.T) {
	tempDir := t.TempDir()
	promptPath := filepath.Join(tempDir, "prompt.txt")
	if writeError := os.WriteFile(promptPath, []byte("file prompt"), 0600); writeError != nil {
		t.Fatalf("write prompt: %v", writeError)
	}
	capturedRequest := capturedProxyRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		bodyBytes, readError := io.ReadAll(httpRequest.Body)
		if readError != nil {
			t.Fatalf("read body: %v", readError)
		}
		capturedRequest = capturedProxyRequest{path: httpRequest.URL.RequestURI(), body: string(bodyBytes)}
		responseWriter.WriteHeader(http.StatusOK)
		_, _ = responseWriter.Write([]byte("file-ok"))
	}))
	t.Cleanup(server.Close)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run(
		[]string{
			"--base-url", server.URL + "?provider=openai",
			"--secret", "test-secret",
			"--provider", "deepseek",
			"--prompt-file", promptPath,
			"--web-search",
			"--system-prompt", "Be terse.",
			"--max-tokens", "42",
			"--timeout", "2s",
		},
		strings.NewReader(""),
		&stdout,
		&stderr,
		defaultHTTPClientFactory,
	)

	if exitCode != 0 {
		t.Fatalf("exit=%d stderr=%s", exitCode, stderr.String())
	}
	if stdout.String() != "file-ok" {
		t.Fatalf("stdout=%q", stdout.String())
	}
	if !strings.Contains(capturedRequest.path, "provider=deepseek") {
		t.Fatalf("path=%q", capturedRequest.path)
	}
	for _, expectedBodyFragment := range []string{
		`"prompt":"file prompt"`,
		`"web_search":true`,
		`"system_prompt":"Be terse."`,
		`"max_tokens":42`,
	} {
		if !strings.Contains(capturedRequest.body, expectedBodyFragment) {
			t.Fatalf("body=%s missing %s", capturedRequest.body, expectedBodyFragment)
		}
	}
}

func TestCommandRejectsInvalidInputs(t *testing.T) {
	testCases := []struct {
		name        string
		arguments   []string
		stdin       io.Reader
		errorString string
	}{
		{
			name:        "conflicting prompt sources",
			arguments:   []string{"--base-url", "http://example.test", "--secret", "sekret", "--prompt", "a", "--prompt-file", "b"},
			stdin:       strings.NewReader(""),
			errorString: "llm_proxy_client_prompt_source_conflict",
		},
		{
			name:        "missing base url",
			arguments:   []string{"--secret", "sekret", "--prompt", "prompt"},
			stdin:       strings.NewReader(""),
			errorString: "missing base_url",
		},
		{
			name:        "invalid base scheme",
			arguments:   []string{"--base-url", "ftp://example.test", "--secret", "sekret", "--prompt", "prompt"},
			stdin:       strings.NewReader(""),
			errorString: "base_url must use http or https",
		},
		{
			name:        "malformed base url",
			arguments:   []string{"--base-url", "http://[::1", "--secret", "sekret", "--prompt", "prompt"},
			stdin:       strings.NewReader(""),
			errorString: "parse base_url",
		},
		{
			name:        "missing base host",
			arguments:   []string{"--base-url", "http:///proxy", "--secret", "sekret", "--prompt", "prompt"},
			stdin:       strings.NewReader(""),
			errorString: "base_url must include host",
		},
		{
			name:        "missing secret",
			arguments:   []string{"--base-url", "http://example.test", "--prompt", "prompt"},
			stdin:       strings.NewReader(""),
			errorString: "missing secret",
		},
		{
			name:        "invalid timeout",
			arguments:   []string{"--base-url", "http://example.test", "--secret", "sekret", "--prompt", "prompt", "--timeout", "0s"},
			stdin:       strings.NewReader(""),
			errorString: "timeout must be positive",
		},
		{
			name:        "missing prompt",
			arguments:   []string{"--base-url", "http://example.test", "--secret", "sekret"},
			stdin:       strings.NewReader(""),
			errorString: "missing prompt",
		},
		{
			name:        "negative max tokens",
			arguments:   []string{"--base-url", "http://example.test", "--secret", "sekret", "--prompt", "prompt", "--max-tokens", "-1"},
			stdin:       strings.NewReader(""),
			errorString: "max_tokens must be positive",
		},
		{
			name:        "missing prompt file",
			arguments:   []string{"--base-url", "http://example.test", "--secret", "sekret", "--prompt-file", "/missing/prompt.txt"},
			stdin:       strings.NewReader(""),
			errorString: "llm_proxy_client_prompt_file_read_failed",
		},
		{
			name:        "stdin read failure",
			arguments:   []string{"--base-url", "http://example.test", "--secret", "sekret"},
			stdin:       failingReader{},
			errorString: "llm_proxy_client_stdin_read_failed",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exitCode := run(testCase.arguments, testCase.stdin, &stdout, &stderr, defaultHTTPClientFactory)
			if exitCode != 1 {
				t.Fatalf("exit=%d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), testCase.errorString) {
				t.Fatalf("stderr=%q missing %q", stderr.String(), testCase.errorString)
			}
		})
	}
}

func TestCommandReportsProxyAndIOErrors(t *testing.T) {
	t.Run("proxy status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.WriteHeader(http.StatusBadGateway)
			_, _ = responseWriter.Write([]byte("upstream failed"))
		}))
		t.Cleanup(server.Close)

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := run(
			[]string{"--base-url", server.URL, "--secret", "sekret", "--prompt", "prompt"},
			strings.NewReader(""),
			&stdout,
			&stderr,
			defaultHTTPClientFactory,
		)
		if exitCode != 1 {
			t.Fatalf("exit=%d", exitCode)
		}
		if !strings.Contains(stderr.String(), "status=502") {
			t.Fatalf("stderr=%q", stderr.String())
		}
	})
	t.Run("transport", func(t *testing.T) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := run(
			[]string{"--base-url", "http://example.test", "--secret", "sekret", "--prompt", "prompt"},
			strings.NewReader(""),
			&stdout,
			&stderr,
			func(timeout time.Duration) llmproxyclient.HTTPDoer {
				return failingHTTPDoer{err: errors.New("transport failed")}
			},
		)
		if exitCode != 1 {
			t.Fatalf("exit=%d", exitCode)
		}
		if !strings.Contains(stderr.String(), "transport failed") {
			t.Fatalf("stderr=%q", stderr.String())
		}
	})
	t.Run("read response", func(t *testing.T) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := run(
			[]string{"--base-url", "http://example.test", "--secret", "sekret", "--prompt", "prompt"},
			strings.NewReader(""),
			&stdout,
			&stderr,
			func(timeout time.Duration) llmproxyclient.HTTPDoer {
				return readFailHTTPDoer{}
			},
		)
		if exitCode != 1 {
			t.Fatalf("exit=%d", exitCode)
		}
		if !strings.Contains(stderr.String(), "read response body") {
			t.Fatalf("stderr=%q", stderr.String())
		}
	})
	t.Run("missing http client", func(t *testing.T) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := run(
			[]string{"--base-url", "http://example.test", "--secret", "sekret", "--prompt", "prompt"},
			strings.NewReader(""),
			&stdout,
			&stderr,
			func(timeout time.Duration) llmproxyclient.HTTPDoer {
				return nil
			},
		)
		if exitCode != 1 {
			t.Fatalf("exit=%d", exitCode)
		}
		if !strings.Contains(stderr.String(), "missing http client") {
			t.Fatalf("stderr=%q", stderr.String())
		}
	})
	t.Run("stdout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.WriteHeader(http.StatusOK)
			_, _ = responseWriter.Write([]byte("ok"))
		}))
		t.Cleanup(server.Close)

		var stderr bytes.Buffer
		exitCode := run(
			[]string{"--base-url", server.URL, "--secret", "sekret", "--prompt", "prompt"},
			strings.NewReader(""),
			failingWriter{},
			&stderr,
			defaultHTTPClientFactory,
		)
		if exitCode != 1 {
			t.Fatalf("exit=%d", exitCode)
		}
		if !strings.Contains(stderr.String(), "llm_proxy_client_stdout_write_failed") {
			t.Fatalf("stderr=%q", stderr.String())
		}
	})
}
