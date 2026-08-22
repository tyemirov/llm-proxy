package llmproxyclient_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

func TestClientPostMessagesSerializesImmutableOrderedMediaAttachments(testingInstance *testing.T) {
	contract := loadCanonicalOpenAPIContract(testingInstance)
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		bodyBytes, readError := io.ReadAll(httpRequest.Body)
		if readError != nil {
			testingInstance.Fatalf("read request body: %v", readError)
		}
		if validationError := contract.ValidateRequest("/v2", httpRequest.Method, httpRequest, bodyBytes); validationError != nil {
			testingInstance.Fatalf("media request violates OpenAPI: %v", validationError)
		}
		if decodeError := json.Unmarshal(bodyBytes, &capturedBody); decodeError != nil {
			testingInstance.Fatalf("decode request body: %v", decodeError)
		}
		responseWriter.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = responseWriter.Write([]byte("media ok"))
	}))
	defer server.Close()

	imageBytes := []byte("exact-image")
	audioBytes := []byte("exact-audio")
	expectedImageBytes := append([]byte(nil), imageBytes...)
	expectedAudioBytes := append([]byte(nil), audioBytes...)
	imageAttachment, imageError := llmproxyclient.NewImageAttachment(llmproxyclient.ImageAttachmentInput{
		MIMEType: " IMAGE/PNG ",
		Data:     imageBytes,
	})
	if imageError != nil {
		testingInstance.Fatalf("image attachment: %v", imageError)
	}
	audioAttachment, audioError := llmproxyclient.NewAudioAttachment(llmproxyclient.AudioAttachmentInput{
		MIMEType: " AUDIO/M4A ",
		Data:     audioBytes,
	})
	if audioError != nil {
		testingInstance.Fatalf("audio attachment: %v", audioError)
	}
	attachments := []llmproxyclient.MessageAttachment{imageAttachment, audioAttachment}
	order := 4
	maxTokens := 512
	request, requestError := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{
		Messages: []llmproxyclient.MessageInput{{
			Role:        "user",
			Content:     "Inspect these exact inputs in order.",
			Attachments: attachments,
			Order:       &order,
		}},
		Model:     "gemini-3.5-flash",
		MaxTokens: &maxTokens,
	})
	if requestError != nil {
		testingInstance.Fatalf("messages request: %v", requestError)
	}

	imageBytes[0] = 'X'
	audioBytes[0] = 'X'
	attachments[0] = nil
	order = 99
	maxTokens = 1

	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL:  server.URL,
		Secret:   "sekret",
		Provider: "gemini",
	})
	if configError != nil {
		testingInstance.Fatalf("client config: %v", configError)
	}
	client, clientError := llmproxyclient.NewClient(config, server.Client())
	if clientError != nil {
		testingInstance.Fatalf("client: %v", clientError)
	}
	responseText, postError := client.PostMessages(context.Background(), request)
	if postError != nil {
		testingInstance.Fatalf("post media messages: %v", postError)
	}
	if responseText != "media ok" {
		testingInstance.Fatalf("response=%q", responseText)
	}

	rawMessages, messagesOK := capturedBody["messages"].([]any)
	if !messagesOK || len(rawMessages) != 1 {
		testingInstance.Fatalf("messages=%v", capturedBody["messages"])
	}
	messagePayload, messageOK := rawMessages[0].(map[string]any)
	if !messageOK || messagePayload["order"] != float64(4) {
		testingInstance.Fatalf("message=%v", rawMessages[0])
	}
	rawAttachments, attachmentsOK := messagePayload["attachments"].([]any)
	if !attachmentsOK || len(rawAttachments) != 2 {
		testingInstance.Fatalf("attachments=%v", messagePayload["attachments"])
	}
	assertClientAttachmentPayload(testingInstance, rawAttachments[0], "image", "image/png", expectedImageBytes)
	assertClientAttachmentPayload(testingInstance, rawAttachments[1], "audio", "audio/m4a", expectedAudioBytes)
	if capturedBody["max_tokens"] != float64(512) || capturedBody["reasoning_effort"] != nil {
		testingInstance.Fatalf("mutable request fields leaked into payload=%v", capturedBody)
	}
}

func TestClientSerializesKimiK3ImageAndReasoningSelection(testingInstance *testing.T) {
	var capturedBody map[string]any
	var capturedProvider string
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		capturedProvider = httpRequest.URL.Query().Get("provider")
		if decodeError := json.NewDecoder(httpRequest.Body).Decode(&capturedBody); decodeError != nil {
			testingInstance.Fatalf("decode request body: %v", decodeError)
		}
		_, _ = responseWriter.Write([]byte("kimi ok"))
	}))
	defer server.Close()

	imageAttachment, imageError := llmproxyclient.NewImageAttachment(llmproxyclient.ImageAttachmentInput{MIMEType: "image/webp", Data: []byte("kimi-image")})
	if imageError != nil {
		testingInstance.Fatalf("image attachment: %v", imageError)
	}
	reasoningEffort := "max"
	request, requestError := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{
		Messages:        []llmproxyclient.MessageInput{{Role: "user", Content: "Inspect.", Attachments: []llmproxyclient.MessageAttachment{imageAttachment}}},
		Model:           "kimi-k3",
		ReasoningEffort: &reasoningEffort,
	})
	if requestError != nil {
		testingInstance.Fatalf("messages request: %v", requestError)
	}
	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "sekret", Provider: "moonshot"})
	if configError != nil {
		testingInstance.Fatalf("client config: %v", configError)
	}
	client, clientError := llmproxyclient.NewClient(config, server.Client())
	if clientError != nil {
		testingInstance.Fatalf("client: %v", clientError)
	}
	if response, postError := client.PostMessages(context.Background(), request); postError != nil || response != "kimi ok" {
		testingInstance.Fatalf("response=%q error=%v", response, postError)
	}
	if capturedProvider != "moonshot" || capturedBody["model"] != "kimi-k3" || capturedBody["reasoning_effort"] != "max" {
		testingInstance.Fatalf("provider=%s payload=%v", capturedProvider, capturedBody)
	}
}

func TestClientUploadsAssetAndSerializesImageAndAudioAssetReferences(testingInstance *testing.T) {
	imageBytes := []byte("uploaded-image")
	imageDigest := sha256.Sum256(imageBytes)
	digestText := hex.EncodeToString(imageDigest[:])
	assetID := "ast_0123456789abcdef0123456789abcdef"
	var capturedAttachment map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case llmproxycontract.AssetPath:
			body, _ := io.ReadAll(request.Body)
			if !bytes.Equal(body, imageBytes) || request.Header.Get("Content-Type") != "image/png" || request.Header.Get(llmproxycontract.HeaderAssetSHA256) != digestText {
				testingInstance.Errorf("asset upload body=%q content-type=%q digest=%q", body, request.Header.Get("Content-Type"), request.Header.Get(llmproxycontract.HeaderAssetSHA256))
			}
			responseWriter.Header().Set("Content-Type", "application/json")
			responseWriter.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprintf(responseWriter, `{"asset_id":"%s","mime_type":"image/png","size_bytes":%d,"sha256":"%s","state":"available","created_at":"2026-08-11T10:00:00Z","expires_at":"2026-08-13T10:00:00Z"}`, assetID, len(imageBytes), digestText)
		case "/v2":
			var payload map[string]any
			_ = json.NewDecoder(request.Body).Decode(&payload)
			capturedAttachment = payload["messages"].([]any)[0].(map[string]any)["attachments"].([]any)[0].(map[string]any)
			_, _ = responseWriter.Write([]byte("asset ok"))
		default:
			http.NotFound(responseWriter, request)
		}
	}))
	defer server.Close()
	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "sekret", Provider: "gemini"})
	if configError != nil {
		testingInstance.Fatalf("config: %v", configError)
	}
	if assetURL := config.AssetUploadURL(); !strings.Contains(assetURL, llmproxycontract.AssetPath+"?key=sekret") {
		testingInstance.Fatalf("asset URL=%q", assetURL)
	}
	client, clientError := llmproxyclient.NewClient(config, server.Client())
	if clientError != nil {
		testingInstance.Fatalf("client: %v", clientError)
	}
	asset, uploadError := client.UploadAsset(context.Background(), llmproxyclient.AssetUploadInput{MIMEType: " IMAGE/PNG ", Data: imageBytes})
	if uploadError != nil || asset.AssetID != assetID || asset.CreatedAt != time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC) {
		testingInstance.Fatalf("asset=%+v error=%v", asset, uploadError)
	}
	imageAttachment, imageError := llmproxyclient.NewImageAssetAttachment(llmproxyclient.ImageAssetAttachmentInput{AssetID: asset.AssetID, MIMEType: asset.MIMEType, SHA256: asset.SHA256})
	if imageError != nil {
		testingInstance.Fatalf("image asset attachment: %v", imageError)
	}
	if _, audioError := llmproxyclient.NewAudioAssetAttachment(llmproxyclient.AudioAssetAttachmentInput{AssetID: asset.AssetID, MIMEType: "audio/wav", SHA256: asset.SHA256}); audioError != nil {
		testingInstance.Fatalf("audio asset attachment: %v", audioError)
	}
	request, requestError := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "inspect", Attachments: []llmproxyclient.MessageAttachment{imageAttachment}}}})
	if requestError != nil {
		testingInstance.Fatalf("request: %v", requestError)
	}
	response, postError := client.PostMessages(context.Background(), request)
	if postError != nil || response != "asset ok" || capturedAttachment["asset_id"] != assetID || capturedAttachment["data"] != nil || capturedAttachment["sha256"] != digestText {
		testingInstance.Fatalf("response=%q error=%v attachment=%v", response, postError, capturedAttachment)
	}
}

type assetRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper assetRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTripper(request)
}

type assetErrorReader struct{}

func (assetErrorReader) Read([]byte) (int, error) {
	return 0, fmt.Errorf("asset response read failure")
}

func TestClientAssetUploadRejectsInvalidInputsAndResponses(testingInstance *testing.T) {
	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: "https://proxy.example/review/v2", Secret: "sekret"})
	if configError != nil {
		testingInstance.Fatalf("config: %v", configError)
	}
	if assetURL := config.AssetUploadURL(); !strings.Contains(assetURL, "/review"+llmproxycontract.AssetPath+"?key=sekret") {
		testingInstance.Fatalf("asset URL=%q", assetURL)
	}
	clientFor := func(roundTripper assetRoundTripper) llmproxyclient.Client {
		testingInstance.Helper()
		client, clientError := llmproxyclient.NewClient(config, &http.Client{Transport: roundTripper})
		if clientError != nil {
			testingInstance.Fatalf("client: %v", clientError)
		}
		return client
	}
	if _, uploadError := clientFor(func(*http.Request) (*http.Response, error) {
		testingInstance.Fatal("invalid input reached transport")
		return nil, nil
	}).UploadAsset(context.Background(), llmproxyclient.AssetUploadInput{MIMEType: "text/plain", Data: []byte("x")}); uploadError == nil {
		testingInstance.Fatal("unsupported MIME accepted")
	}
	if _, uploadError := clientFor(func(*http.Request) (*http.Response, error) {
		testingInstance.Fatal("empty input reached transport")
		return nil, nil
	}).UploadAsset(context.Background(), llmproxyclient.AssetUploadInput{MIMEType: "image/png"}); uploadError == nil {
		testingInstance.Fatal("empty asset accepted")
	}

	validInput := llmproxyclient.AssetUploadInput{MIMEType: "image/png", Data: []byte("x")}
	testCases := []struct {
		name string
		do   assetRoundTripper
	}{
		{name: "transport failure", do: func(*http.Request) (*http.Response, error) { return nil, fmt.Errorf("transport failure") }},
		{name: "read failure", do: func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(assetErrorReader{}), Header: http.Header{}}, nil
		}},
		{name: "oversized response", do: func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", 64*1024+1))), Header: http.Header{}}, nil
		}},
		{name: "HTTP failure", do: func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"asset_invalid"}}`)), Header: http.Header{}}, nil
		}},
		{name: "malformed response", do: func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader("{")), Header: http.Header{}}, nil
		}},
		{name: "trailing response", do: func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`{} {}`)), Header: http.Header{}}, nil
		}},
		{name: "invalid response", do: func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`{"asset_id":"bad"}`)), Header: http.Header{}}, nil
		}},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			if _, uploadError := clientFor(testCase.do).UploadAsset(context.Background(), validInput); uploadError == nil {
				subTest.Fatal("invalid asset exchange accepted")
			}
		})
	}
}

func TestMessageAttachmentConstructorsEnforceCanonicalMediaContract(testingInstance *testing.T) {
	for _, testCase := range []struct {
		name      string
		construct func() (llmproxyclient.MessageAttachment, error)
		wantError bool
	}{
		{
			name: "JPEG image",
			construct: func() (llmproxyclient.MessageAttachment, error) {
				return llmproxyclient.NewImageAttachment(llmproxyclient.ImageAttachmentInput{MIMEType: "image/jpeg", Data: []byte("jpeg")})
			},
		},
		{
			name: "WebP image",
			construct: func() (llmproxyclient.MessageAttachment, error) {
				return llmproxyclient.NewImageAttachment(llmproxyclient.ImageAttachmentInput{MIMEType: "image/webp", Data: []byte("webp")})
			},
		},
		{
			name: "MPEG audio",
			construct: func() (llmproxyclient.MessageAttachment, error) {
				return llmproxyclient.NewAudioAttachment(llmproxyclient.AudioAttachmentInput{MIMEType: "audio/mpeg", Data: []byte("mpeg")})
			},
		},
		{
			name: "WAV audio",
			construct: func() (llmproxyclient.MessageAttachment, error) {
				return llmproxyclient.NewAudioAttachment(llmproxyclient.AudioAttachmentInput{MIMEType: "audio/wav", Data: []byte("wav")})
			},
		},
		{
			name: "unsupported image MIME",
			construct: func() (llmproxyclient.MessageAttachment, error) {
				return llmproxyclient.NewImageAttachment(llmproxyclient.ImageAttachmentInput{MIMEType: "image/gif", Data: []byte("gif")})
			},
			wantError: true,
		},
		{
			name: "empty image",
			construct: func() (llmproxyclient.MessageAttachment, error) {
				return llmproxyclient.NewImageAttachment(llmproxyclient.ImageAttachmentInput{MIMEType: "image/png"})
			},
			wantError: true,
		},
		{
			name: "unsupported audio MIME",
			construct: func() (llmproxyclient.MessageAttachment, error) {
				return llmproxyclient.NewAudioAttachment(llmproxyclient.AudioAttachmentInput{MIMEType: "audio/ogg", Data: []byte("ogg")})
			},
			wantError: true,
		},
		{
			name: "empty audio",
			construct: func() (llmproxyclient.MessageAttachment, error) {
				return llmproxyclient.NewAudioAttachment(llmproxyclient.AudioAttachmentInput{MIMEType: "audio/wav"})
			},
			wantError: true,
		},
		{
			name: "unsupported image asset MIME",
			construct: func() (llmproxyclient.MessageAttachment, error) {
				return llmproxyclient.NewImageAssetAttachment(llmproxyclient.ImageAssetAttachmentInput{MIMEType: "image/gif"})
			},
			wantError: true,
		},
		{
			name: "unsupported audio asset MIME",
			construct: func() (llmproxyclient.MessageAttachment, error) {
				return llmproxyclient.NewAudioAssetAttachment(llmproxyclient.AudioAssetAttachmentInput{MIMEType: "audio/ogg"})
			},
			wantError: true,
		},
		{
			name: "invalid asset identifier",
			construct: func() (llmproxyclient.MessageAttachment, error) {
				return llmproxyclient.NewImageAssetAttachment(llmproxyclient.ImageAssetAttachmentInput{AssetID: "bad", MIMEType: "image/png", SHA256: strings.Repeat("0", 64)})
			},
			wantError: true,
		},
		{
			name: "invalid asset digest",
			construct: func() (llmproxyclient.MessageAttachment, error) {
				return llmproxyclient.NewAudioAssetAttachment(llmproxyclient.AudioAssetAttachmentInput{AssetID: "ast_0123456789abcdef0123456789abcdef", MIMEType: "audio/wav", SHA256: "bad"})
			},
			wantError: true,
		},
	} {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			attachment, attachmentError := testCase.construct()
			if testCase.wantError {
				if attachmentError == nil || attachment != nil {
					subTest.Fatalf("attachment=%v error=%v", attachment, attachmentError)
				}
				return
			}
			if attachmentError != nil || attachment == nil {
				subTest.Fatalf("attachment=%v error=%v", attachment, attachmentError)
			}
		})
	}
}

func TestMessagesRequestRejectsInvalidAttachmentPlacement(testingInstance *testing.T) {
	imageAttachment, imageError := llmproxyclient.NewImageAttachment(llmproxyclient.ImageAttachmentInput{
		MIMEType: "image/png",
		Data:     []byte("image"),
	})
	if imageError != nil {
		testingInstance.Fatalf("image attachment: %v", imageError)
	}
	for _, testCase := range []struct {
		name    string
		message llmproxyclient.MessageInput
	}{
		{
			name:    "nil attachment",
			message: llmproxyclient.MessageInput{Role: "user", Content: "inspect", Attachments: []llmproxyclient.MessageAttachment{nil}},
		},
		{
			name:    "system attachment",
			message: llmproxyclient.MessageInput{Role: "system", Content: "inspect", Attachments: []llmproxyclient.MessageAttachment{imageAttachment}},
		},
		{
			name:    "blank content",
			message: llmproxyclient.MessageInput{Role: "user", Content: " \t"},
		},
	} {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			_, requestError := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{
				Messages: []llmproxyclient.MessageInput{testCase.message},
			})
			if requestError == nil {
				subTest.Fatal("expected request rejection")
			}
		})
	}
}

func assertClientAttachmentPayload(testingInstance *testing.T, rawAttachment any, attachmentType string, mimeType string, expectedData []byte) {
	testingInstance.Helper()
	attachmentPayload, attachmentOK := rawAttachment.(map[string]any)
	if !attachmentOK {
		testingInstance.Fatalf("attachment=%v", rawAttachment)
	}
	digest := sha256.Sum256(expectedData)
	expectedDigest := hex.EncodeToString(digest[:])
	if attachmentPayload["type"] != attachmentType ||
		attachmentPayload["mime_type"] != mimeType ||
		attachmentPayload["data"] != base64.StdEncoding.EncodeToString(expectedData) ||
		attachmentPayload["sha256"] != expectedDigest {
		testingInstance.Fatalf("attachment payload=%v", attachmentPayload)
	}
}
