package proxy_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/temirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

const (
	testAudioPayload = "fake-audio-binary"
)

func newDictationRouter(t *testing.T, transcriptionsURL string, requestTimeoutSeconds int) *gin.Engine {
	t.Helper()
	endpoints := proxy.NewEndpoints()
	endpoints.SetTranscriptionsURL(transcriptionsURL)

	logger, _ := zap.NewDevelopment()
	t.Cleanup(func() { _ = logger.Sync() })

	router, buildError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:              TestSecret,
		OpenAIKey:                  TestAPIKey,
		LogLevel:                   proxy.LogLevelDebug,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      requestTimeoutSeconds,
		UpstreamPollTimeoutSeconds: requestTimeoutSeconds,
		DictationModel:             proxy.DefaultDictationModel,
		MaxInputAudioBytes:         1024 * 1024,
		Endpoints:                  endpoints,
	}, logger.Sugar())
	if buildError != nil {
		t.Fatalf("BuildRouter error: %v", buildError)
	}
	return router
}

func buildMultipartAudioRequest(t *testing.T, formFieldName string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	filePart, createError := writer.CreateFormFile(formFieldName, "recording.webm")
	if createError != nil {
		t.Fatalf("CreateFormFile error: %v", createError)
	}
	if _, copyError := io.Copy(filePart, strings.NewReader(testAudioPayload)); copyError != nil {
		t.Fatalf("Copy error: %v", copyError)
	}
	if closeError := writer.Close(); closeError != nil {
		t.Fatalf("Close writer error: %v", closeError)
	}
	return body, writer.FormDataContentType()
}

func decodeTextResponse(t *testing.T, responseBody []byte) string {
	t.Helper()
	var payload map[string]string
	if decodeError := json.Unmarshal(responseBody, &payload); decodeError != nil {
		t.Fatalf("decode response error: %v body=%s", decodeError, string(responseBody))
	}
	return payload["text"]
}

func TestDictateHandlerSuccessWithAudioField(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("method=%s want=%s", request.Method, http.MethodPost)
		}
		if authorizationHeader := request.Header.Get("Authorization"); authorizationHeader != "Bearer "+TestAPIKey {
			t.Fatalf("authorization=%q want=%q", authorizationHeader, "Bearer "+TestAPIKey)
		}
		if parseError := request.ParseMultipartForm(1024 * 1024); parseError != nil {
			t.Fatalf("ParseMultipartForm error: %v", parseError)
		}
		if model := request.FormValue("model"); model != proxy.DefaultDictationModel {
			t.Fatalf("model=%q want=%q", model, proxy.DefaultDictationModel)
		}
		file, _, fileError := request.FormFile("file")
		if fileError != nil {
			t.Fatalf("FormFile(file) error: %v", fileError)
		}
		defer file.Close()
		rawAudio, readError := io.ReadAll(file)
		if readError != nil {
			t.Fatalf("ReadAll(file) error: %v", readError)
		}
		if string(rawAudio) != testAudioPayload {
			t.Fatalf("audio payload mismatch got=%q want=%q", string(rawAudio), testAudioPayload)
		}

		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"text":"transcribed audio"}`))
	}))
	defer upstreamServer.Close()

	router := newDictationRouter(t, upstreamServer.URL, TestTimeout)
	body, contentType := buildMultipartAudioRequest(t, "audio")
	request := httptest.NewRequest(http.MethodPost, "/dictate?key="+TestSecret, body)
	request.Header.Set("Content-Type", contentType)
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusOK, responseRecorder.Body.String())
	}
	if responseText := decodeTextResponse(t, responseRecorder.Body.Bytes()); responseText != "transcribed audio" {
		t.Fatalf("text=%q want=%q", responseText, "transcribed audio")
	}
}

func TestDictateHandlerAcceptsFileAlias(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if parseError := request.ParseMultipartForm(1024 * 1024); parseError != nil {
			t.Fatalf("ParseMultipartForm error: %v", parseError)
		}
		if model := request.FormValue("model"); model != proxy.DefaultDictationModel {
			t.Fatalf("model=%q want=%q", model, proxy.DefaultDictationModel)
		}
		if _, _, fileError := request.FormFile("file"); fileError != nil {
			t.Fatalf("FormFile(file) error: %v", fileError)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"text":"ok from alias"}`))
	}))
	defer upstreamServer.Close()

	router := newDictationRouter(t, upstreamServer.URL, TestTimeout)
	body, contentType := buildMultipartAudioRequest(t, "file")
	request := httptest.NewRequest(http.MethodPost, "/dictate?key="+TestSecret, body)
	request.Header.Set("Content-Type", contentType)
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusOK, responseRecorder.Body.String())
	}
	if responseText := decodeTextResponse(t, responseRecorder.Body.Bytes()); responseText != "ok from alias" {
		t.Fatalf("text=%q want=%q", responseText, "ok from alias")
	}
}

func TestDictateHandlerSupportsModelOverride(t *testing.T) {
	const modelOverride = "gpt-4o-transcribe"
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if parseError := request.ParseMultipartForm(1024 * 1024); parseError != nil {
			t.Fatalf("ParseMultipartForm error: %v", parseError)
		}
		if model := request.FormValue("model"); model != modelOverride {
			t.Fatalf("model=%q want=%q", model, modelOverride)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"text":"model override ok"}`))
	}))
	defer upstreamServer.Close()

	router := newDictationRouter(t, upstreamServer.URL, TestTimeout)
	body, contentType := buildMultipartAudioRequest(t, "audio")

	query := url.Values{}
	query.Set("key", TestSecret)
	query.Set("model", modelOverride)
	request := httptest.NewRequest(http.MethodPost, "/dictate?"+query.Encode(), body)
	request.Header.Set("Content-Type", contentType)
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusOK, responseRecorder.Body.String())
	}
	if responseText := decodeTextResponse(t, responseRecorder.Body.Bytes()); responseText != "model override ok" {
		t.Fatalf("text=%q want=%q", responseText, "model override ok")
	}
}

func TestDictateHandlerValidationAndAuth(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"text":"should-not-be-used"}`))
	}))
	defer upstreamServer.Close()

	router := newDictationRouter(t, upstreamServer.URL, TestTimeout)

	t.Run("missing key is forbidden", func(subTest *testing.T) {
		body, contentType := buildMultipartAudioRequest(subTest, "audio")
		request := httptest.NewRequest(http.MethodPost, "/dictate", body)
		request.Header.Set("Content-Type", contentType)
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request)
		if responseRecorder.Code != http.StatusForbidden {
			subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusForbidden, responseRecorder.Body.String())
		}
	})

	t.Run("invalid multipart form returns bad request", func(subTest *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/dictate?key="+TestSecret, strings.NewReader("not-multipart"))
		request.Header.Set("Content-Type", "text/plain")
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request)
		if responseRecorder.Code != http.StatusBadRequest {
			subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusBadRequest, responseRecorder.Body.String())
		}
	})

	t.Run("multipart without audio returns bad request", func(subTest *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		if writeError := writer.WriteField("note", "missing audio"); writeError != nil {
			subTest.Fatalf("WriteField error: %v", writeError)
		}
		if closeError := writer.Close(); closeError != nil {
			subTest.Fatalf("Close writer error: %v", closeError)
		}

		request := httptest.NewRequest(http.MethodPost, "/dictate?key="+TestSecret, body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request)
		if responseRecorder.Code != http.StatusBadRequest {
			subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusBadRequest, responseRecorder.Body.String())
		}
	})
}

func TestDictateHandlerUpstreamFailures(t *testing.T) {
	t.Run("upstream non-2xx returns bad gateway", func(subTest *testing.T) {
		upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			responseWriter.WriteHeader(http.StatusInternalServerError)
			_, _ = responseWriter.Write([]byte(`{"error":"upstream failed"}`))
		}))
		defer upstreamServer.Close()

		router := newDictationRouter(subTest, upstreamServer.URL, TestTimeout)
		body, contentType := buildMultipartAudioRequest(subTest, "audio")
		request := httptest.NewRequest(http.MethodPost, "/dictate?key="+TestSecret, body)
		request.Header.Set("Content-Type", contentType)
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request)

		if responseRecorder.Code != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusBadGateway, responseRecorder.Body.String())
		}
	})

	t.Run("upstream timeout returns gateway timeout", func(subTest *testing.T) {
		upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			time.Sleep(1500 * time.Millisecond)
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{"text":"late"}`))
		}))
		defer upstreamServer.Close()

		router := newDictationRouter(subTest, upstreamServer.URL, 1)
		body, contentType := buildMultipartAudioRequest(subTest, "audio")
		request := httptest.NewRequest(http.MethodPost, "/dictate?key="+TestSecret, body)
		request.Header.Set("Content-Type", contentType)
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request)

		if responseRecorder.Code != http.StatusGatewayTimeout {
			subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusGatewayTimeout, responseRecorder.Body.String())
		}
	})
}
