package tests_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

const systemOneLiveFixtureKey = "private-proxy-fixture-key"
const systemOneLiveFixtureRequestID = "AAAAAAAAAAAAAAAAAAAAAAAAAA"

func runSystemOneLiveAcceptance(testingInstance *testing.T, origin, timeout, caseName string) (string, error) {
	testingInstance.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "node", "--experimental-strip-types", "scripts/system_one_live_acceptance.ts", origin, timeout, caseName)
	command.Dir = operationalRepositoryRoot(testingInstance)
	command.Env = append(os.Environ(), "LLM_PROXY_DEFAULT_TENANT_KEY="+systemOneLiveFixtureKey)
	output, commandError := command.CombinedOutput()
	if strings.Contains(string(output), systemOneLiveFixtureKey) {
		testingInstance.Fatal("helper exposed proxy credential")
	}
	return string(output), commandError
}

func TestSystemOneLiveAcceptanceFixtureAndPreflight(testingInstance *testing.T) {
	fixture, readError := os.ReadFile(filepath.Join(operationalRepositoryRoot(testingInstance), "tests/testdata/system-one-black-rectangle.png"))
	if readError != nil {
		testingInstance.Fatal(readError)
	}
	config, decodeError := png.DecodeConfig(bytes.NewReader(fixture))
	if decodeError != nil || config.Width != 512 || config.Height != 512 {
		testingInstance.Fatalf("live image must be 512 by 512 (provider minimum 280): config=%+v error=%v", config, decodeError)
	}
	image, imageError := png.Decode(bytes.NewReader(fixture))
	if imageError != nil {
		testingInstance.Fatalf("decode synthetic live PNG: %v", imageError)
	}
	red, green, blue, _ := image.At(256, 256).RGBA()
	if red != 0 || green != 0 || blue != 0 {
		testingInstance.Fatal("synthetic live PNG center must be black")
	}
	red, green, blue, _ = image.At(0, 0).RGBA()
	if red != 65535 || green != 65535 || blue != 65535 {
		testingInstance.Fatal("synthetic live PNG background must be white")
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { calls.Add(1); writer.WriteHeader(500) }))
	defer server.Close()
	output, commandError := runSystemOneLiveAcceptance(testingInstance, server.URL, "1", "preflight")
	if commandError != nil || calls.Load() != 0 {
		testingInstance.Fatalf("preflight made HTTP calls or failed: calls=%d error=%v output=%s", calls.Load(), commandError, output)
	}
	for _, arguments := range [][2]string{{"0", "text"}, {"no-timeout", "image"}, {"1", "unknown"}} {
		output, commandError = runSystemOneLiveAcceptance(testingInstance, server.URL, arguments[0], arguments[1])
		if commandError == nil || calls.Load() != 0 {
			testingInstance.Fatalf("invalid CLI args reached provider: args=%v calls=%d error=%v output=%s", arguments, calls.Load(), commandError, output)
		}
	}
}

func TestSystemOneLiveAcceptanceHTTPContract(testingInstance *testing.T) {
	for _, caseName := range []string{"text", "image"} {
		for _, nullableUsage := range []bool{false, true} {
			testingInstance.Run(caseName+map[bool]string{true: "-null-usage", false: "-usage"}[nullableUsage], func(testingInstance *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					calls.Add(1)
					if request.Method != http.MethodPost || request.URL.Path != "/v1/systemone" {
						testingInstance.Errorf("unexpected endpoint: %s %s", request.Method, request.URL.Path)
					}
					if request.Header.Get("Authorization") != "Bearer "+systemOneLiveFixtureKey {
						testingInstance.Error("missing SDK tenant authentication")
					}
					if request.Header.Get("Content-Encoding") == "zstd" {
						decoder, decodeError := zstd.NewReader(request.Body)
						if decodeError != nil {
							testingInstance.Errorf("open compressed SDK request: %v", decodeError)
							writer.WriteHeader(400)
							return
						}
						body, readError := io.ReadAll(decoder)
						decoder.Close()
						if readError != nil {
							testingInstance.Errorf("decompress SDK request: %v", readError)
							writer.WriteHeader(400)
							return
						}
						request.Body = io.NopCloser(bytes.NewReader(body))
					}
					var payload map[string]any
					if caseName == "image" {
						if parseError := request.ParseMultipartForm(2 << 20); parseError != nil {
							testingInstance.Errorf("parse image multipart: %v", parseError)
							writer.WriteHeader(400)
							return
						}
						if request.Header.Get("TypeSafe-Danger-Zone-Images") != "true" {
							testingInstance.Error("image opt-in header missing")
						}
						for _, value := range request.MultipartForm.Value {
							for _, item := range value {
								if json.Unmarshal([]byte(item), &payload) == nil && payload["model"] != nil {
									break
								}
							}
						}
						files := request.MultipartForm.File
						if len(files) != 1 {
							testingInstance.Errorf("expected exactly one attached image: %d", len(files))
						}
						defer request.MultipartForm.RemoveAll()
						for _, attachments := range files {
							for _, attachment := range attachments {
								asset, openError := attachment.Open()
								if openError != nil {
									testingInstance.Errorf("open image attachment: %v", openError)
									continue
								}
								dimensions, imageError := png.DecodeConfig(asset)
								asset.Close()
								if imageError != nil || dimensions.Width != 512 || dimensions.Height != 512 {
									testingInstance.Errorf("invalid live attachment dimensions: %+v error=%v", dimensions, imageError)
								}
							}
						}
					} else {
						if decodeError := json.NewDecoder(request.Body).Decode(&payload); decodeError != nil {
							testingInstance.Errorf("decode text request: %v", decodeError)
						}
					}
					if payload["model"] != "jev-1.14.0" {
						testingInstance.Errorf("model not pinned: %v", payload["model"])
					}
					writer.Header().Set("Content-Type", "application/json")
					writer.Header().Set("X-LLM-Proxy-Request-ID", systemOneLiveFixtureRequestID)
					writer.Header().Set("X-TypeSafe-Request-ID", "typesafe-fixture-request")
					usage := `{"input_tokens":400,"output_tokens":20}`
					if nullableUsage {
						usage = `{"input_tokens":null,"output_tokens":null}`
					}
					answers := `{"color":{"type":"choice","choice":"blue","confidence":0.9,"probabilities":{"blue":0.9,"red":0.1}},"severity":{"type":"score","score":0.9,"confidence":0.9,"legend":{"0":"low","1":"high"},"probabilities":{"0":0.1,"1":0.9}},"success":{"type":"noul","noul":0.95}}`
					if caseName == "image" {
						answers = `{"rectangle":{"type":"box","box":{"top":0.25,"bottom":0.75,"left":0.25,"right":0.75},"confidence":null}}`
					}
					io.WriteString(writer, `{"model":"jev-1.14.0","answers":`+answers+`,"usage":`+usage+`}`)
				}))
				defer server.Close()
				output, commandError := runSystemOneLiveAcceptance(testingInstance, server.URL, "2", caseName)
				if commandError != nil || calls.Load() != 1 {
					testingInstance.Fatalf("helper failed valid result or retried: calls=%d error=%v output=%s", calls.Load(), commandError, output)
				}
				for _, expected := range []string{"case=typesafe-" + caseName, "status=200", "request_id=" + systemOneLiveFixtureRequestID, "typesafe_request_id=typesafe-fixture-request", "model=jev-1.14.0"} {
					if !strings.Contains(output, expected) {
						testingInstance.Errorf("missing safe result %q: %s", expected, output)
					}
				}
				expectedUsage := "input_tokens=400 output_tokens=20"
				if nullableUsage {
					expectedUsage = "input_tokens=null output_tokens=null"
				}
				if !strings.Contains(output, expectedUsage) {
					testingInstance.Errorf("usage changed: %s", output)
				}
			})
		}
	}
}

func TestSystemOneLiveAcceptanceRejectsInvalidAndFailedResults(testingInstance *testing.T) {
	scenarios := []struct {
		name      string
		status    int
		body      string
		timeout   bool
		invalidID bool
	}{
		{name: "invalid-answers", status: 200, body: `{"model":"jev-1.14.0","answers":{},"usage":{"input_tokens":1,"output_tokens":1}}`},
		{name: "secret-error", status: 429, body: `{"error":{"message":"private-proxy-fixture-key upstream-secret-marker","type":"rate_limit_error"}}`},
		{name: "timeout", timeout: true},
		{name: "invalid-request-id", status: 200, body: `{}`, invalidID: true},
	}
	for _, caseName := range []string{"text", "image"} {
		for _, scenario := range scenarios {
			testingInstance.Run(caseName+"/"+scenario.name, func(testingInstance *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					calls.Add(1)
					if scenario.timeout {
						select {
						case <-request.Context().Done():
						case <-time.After(1500 * time.Millisecond):
						}
						return
					}
					writer.Header().Set("Content-Type", "application/json")
					writer.Header().Set("X-LLM-Proxy-Request-ID", systemOneLiveFixtureRequestID)
					if scenario.invalidID {
						writer.Header().Set("X-LLM-Proxy-Request-ID", "unsafe secret-marker")
					}
					writer.Header().Set("X-TypeSafe-Request-ID", "typesafe-fixture-request")
					writer.WriteHeader(scenario.status)
					body := scenario.body
					if scenario.invalidID {
						answers := `{"color":{"type":"choice","choice":"blue","confidence":0.9,"probabilities":{"blue":0.9,"red":0.1}},"severity":{"type":"score","score":0.9,"confidence":0.9,"legend":{"0":"low","1":"high"},"probabilities":{"0":0.1,"1":0.9}},"success":{"type":"noul","noul":0.95}}`
						if caseName == "image" {
							answers = `{"rectangle":{"type":"box","box":{"top":0.25,"bottom":0.75,"left":0.25,"right":0.75},"confidence":null}}`
						}
						body = `{"model":"jev-1.14.0","answers":` + answers + `,"usage":{"input_tokens":1,"output_tokens":1}}`
					}
					io.WriteString(writer, body)
				}))
				defer server.Close()
				output, commandError := runSystemOneLiveAcceptance(testingInstance, server.URL, "1", caseName)
				if commandError == nil || calls.Load() != 1 {
					testingInstance.Fatalf("helper accepted invalid result or retried: calls=%d error=%v output=%s", calls.Load(), commandError, output)
				}
				for _, secret := range []string{"upstream-secret-marker", "unsafe secret-marker", "rate_limit_error"} {
					if strings.Contains(output, secret) {
						testingInstance.Fatalf("helper leaked error detail: %s", output)
					}
				}
			})
		}
	}
}
