package llmproxyclient_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
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
		MIMEType: " AUDIO/MP4 ",
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
		Model:     "gemini-2.5-flash",
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
	assertClientAttachmentPayload(testingInstance, rawAttachments[1], "audio", "audio/mp4", expectedAudioBytes)
	if capturedBody["max_tokens"] != float64(512) || capturedBody["reasoning_effort"] != nil {
		testingInstance.Fatalf("mutable request fields leaked into payload=%v", capturedBody)
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
