package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestProviderRequestCodecsRejectMalformedEndpointsBeforeDispatch(t *testing.T) {
	const malformed = "http://[::1"
	logger := zap.NewNop().Sugar()
	doer := geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
		t.Fatal("malformed endpoint reached provider")
		return nil, nil
	})
	endpoints := NewEndpoints()
	endpoints.SetResponsesURL(malformed)
	client := NewOpenAIClient(doer, endpoints)
	ctx := requestContextWithTelemetry(context.Background(), newRequestTelemetry("malformed-provider-url", "/v2"))
	model := textModelDefinition{providerIdentifier: newModelID("provider-model"), requestProfile: requestProfileOpenAIResponsesTemperature, outputTokenLimit: 32}
	messages := chatMessages{{role: chatRoleUser, content: "inspect"}}
	checks := []struct {
		name    string
		request func() error
	}{
		{"Responses", func() error {
			_, err := client.openAIRequest(ctx, "key", model, messages, false, nil, "", nil, nil, logger)
			return err
		}},
		{"transcription", func() error {
			_, err := client.transcribeAudioWithURL(context.Background(), "key", malformed, "model", "whisper-1", "audio.wav", strings.NewReader("audio"), logger)
			return err
		}},
		{"Anthropic", func() error {
			_, err := newAnthropicMessagesClient(doer).generateText(context.Background(), "key", malformed, model, messages, nil, "", nil, logger)
			return err
		}},
		{"Chat Completions", func() error {
			_, err := newOpenAICompatibleChatClient(doer).generateText(context.Background(), "key", malformed, model, messages, nil, chatCompletionTokenLimitMaxTokens, "", nil, nil, logger)
			return err
		}},
		{"Gemini", func() error {
			_, err := newGeminiInteractionsClient(doer).performInteractionRequest(ctx, http.MethodPost, malformed, "key", nil, logger)
			return err
		}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.request(); err == nil {
				t.Fatal("malformed endpoint accepted")
			}
		})
	}
	provider := providerDefinition{textEndpointURL: malformed}
	model.wireContract = textWireContractAnthropicMessages
	model.executionLifecycle = textExecutionLifecycleSynchronousCompletion
	verifier := newOperationalProviderKeyVerifier(doer, endpoints, time.Second, logger)
	if err := verifier.verify(context.Background(), provider, model, "key"); !errors.Is(err, errProviderKeyVerificationUnavailable) {
		t.Fatalf("verification error=%v", err)
	}
}

func TestProviderImageSerializationAndLimitFailureContracts(t *testing.T) {
	logger := zap.NewNop().Sugar()
	closedFile, fileError := os.CreateTemp(t.TempDir(), "closed-provider-media")
	if fileError != nil {
		t.Fatalf("create media: %v", fileError)
	}
	if closeError := closedFile.Close(); closeError != nil {
		t.Fatalf("close media: %v", closeError)
	}
	closedMessages := chatMessages{{
		role:    chatRoleUser,
		content: "inspect",
		attachments: []messageMedia{{
			mediaType: messageMediaTypeImage,
			mimeType:  messageImageMIMEPNG,
			sizeBytes: 1,
			asset:     &tenantAssetReader{file: closedFile},
		}},
	}}
	openAIClient := NewOpenAIClient(geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
		t.Fatal("serialization failure reached provider")
		return nil, nil
	}), NewEndpoints())
	anthropicClient := newAnthropicMessagesClient(geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
		t.Fatal("serialization failure reached provider")
		return nil, nil
	}))
	chatClient := newOpenAICompatibleChatClient(geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
		t.Fatal("serialization failure reached provider")
		return nil, nil
	}))
	model := textModelDefinition{
		providerIdentifier: newModelID("provider-model"),
		requestProfile:     requestProfileOpenAIResponsesTemperature,
		outputTokenLimit:   32,
	}
	if _, requestError := openAIClient.openAIRequest(context.Background(), "key", model, closedMessages, false, nil, "", nil, nil, logger); !errors.Is(requestError, errAssetStore) {
		t.Fatalf("OpenAI serialization error=%v", requestError)
	}
	if _, requestError := (xaiResponsesClient{httpClient: openAIClient.httpClient}).generateText(context.Background(), "key", "https://provider.test", model, closedMessages, nil, "", nil, nil, logger); !errors.Is(requestError, errAssetStore) {
		t.Fatalf("xAI serialization error=%v", requestError)
	}
	if _, requestError := (dashScopeResponsesClient{httpClient: openAIClient.httpClient}).generateText(context.Background(), "key", "https://provider.test", model, closedMessages, nil, "", logger); !errors.Is(requestError, errAssetStore) {
		t.Fatalf("DashScope serialization error=%v", requestError)
	}
	if _, requestError := anthropicClient.generateText(context.Background(), "key", "https://provider.test", model, closedMessages, nil, "", nil, logger); !errors.Is(requestError, errAssetStore) {
		t.Fatalf("Anthropic serialization error=%v", requestError)
	}
	if _, requestError := chatClient.generateText(context.Background(), "key", "https://provider.test", model, closedMessages, nil, chatCompletionTokenLimitMaxTokens, "", nil, nil, logger); !errors.Is(requestError, errAssetStore) {
		t.Fatalf("Chat Completions serialization error=%v", requestError)
	}

	closedMessages[0].attachments[0].sizeBytes = 2
	model.mediaLimits = []CatalogMediaLimit{{
		ID:        CatalogMediaLimitIDImageInlineBytes,
		MediaType: string(messageMediaTypeImage),
		Status:    CatalogMediaLimitStatusBounded,
		Value:     int64Pointer(1),
		Scope:     CatalogMediaLimitScopeAttachment,
	}}
	if _, requestError := openAIClient.openAIRequest(context.Background(), "key", model, closedMessages, false, nil, "", nil, nil, logger); !errors.Is(requestError, ErrProviderMediaLimit) {
		t.Fatalf("OpenAI pre-serialization media limit error=%v", requestError)
	}
	if _, requestError := (xaiResponsesClient{httpClient: openAIClient.httpClient}).generateText(context.Background(), "key", "https://provider.test", model, closedMessages, nil, "", nil, nil, logger); !errors.Is(requestError, ErrProviderMediaLimit) {
		t.Fatalf("xAI pre-serialization media limit error=%v", requestError)
	}
	if _, requestError := (dashScopeResponsesClient{httpClient: openAIClient.httpClient}).generateText(context.Background(), "key", "https://provider.test", model, closedMessages, nil, "", logger); !errors.Is(requestError, ErrProviderMediaLimit) {
		t.Fatalf("DashScope pre-serialization media limit error=%v", requestError)
	}
	if _, requestError := anthropicClient.generateText(context.Background(), "key", "https://provider.test", model, closedMessages, nil, "", nil, logger); !errors.Is(requestError, ErrProviderMediaLimit) {
		t.Fatalf("Anthropic pre-serialization media limit error=%v", requestError)
	}

	inlineMessages := chatMessages{{
		role:    chatRoleUser,
		content: "inspect",
		attachments: []messageMedia{{
			mediaType: messageMediaTypeImage,
			mimeType:  messageImageMIMEPNG,
			sizeBytes: 1,
			data:      []byte("x"),
		}},
	}}
	model.mediaLimits = []CatalogMediaLimit{{
		ID:        CatalogMediaLimitIDInlineRequestBytes,
		MediaType: CatalogMediaLimitTypeAll,
		Status:    CatalogMediaLimitStatusBounded,
		Value:     int64Pointer(4),
	}}
	if _, requestError := openAIClient.openAIRequest(context.Background(), "key", model, inlineMessages, false, nil, "", nil, nil, logger); !errors.Is(requestError, ErrProviderMediaLimit) {
		t.Fatalf("OpenAI media limit error=%v", requestError)
	}
	if _, requestError := (xaiResponsesClient{httpClient: openAIClient.httpClient}).generateText(context.Background(), "key", "https://provider.test", model, inlineMessages, nil, "", nil, nil, logger); !errors.Is(requestError, ErrProviderMediaLimit) {
		t.Fatalf("xAI media limit error=%v", requestError)
	}
	if _, requestError := (dashScopeResponsesClient{httpClient: openAIClient.httpClient}).generateText(context.Background(), "key", "https://provider.test", model, inlineMessages, nil, "", logger); !errors.Is(requestError, ErrProviderMediaLimit) {
		t.Fatalf("DashScope media limit error=%v", requestError)
	}
	if _, requestError := anthropicClient.generateText(context.Background(), "key", "https://provider.test", model, inlineMessages, nil, "", nil, logger); !errors.Is(requestError, ErrProviderMediaLimit) {
		t.Fatalf("Anthropic media limit error=%v", requestError)
	}
	if _, requestError := chatClient.generateText(context.Background(), "key", "https://provider.test", model, inlineMessages, nil, chatCompletionTokenLimitMaxTokens, "", nil, nil, logger); !errors.Is(requestError, ErrProviderMediaLimit) {
		t.Fatalf("Chat Completions media limit error=%v", requestError)
	}
}

func TestSynchronousResponsesFailureContracts(t *testing.T) {
	logger := zap.NewNop().Sugar()
	messages := chatMessages{{role: chatRoleUser, content: "inspect"}}
	model := textModelDefinition{providerIdentifier: newModelID("grok-test")}

	client := NewOpenAIClient(geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
		return nil, context.Canceled
	}), NewEndpoints())
	if _, requestError := (xaiResponsesClient{httpClient: client.httpClient}).generateText(context.Background(), "key", "http://[::1", model, messages, nil, "", nil, nil, logger); requestError == nil {
		t.Fatal("xAI accepted invalid Responses URL")
	}
	if _, requestError := (dashScopeResponsesClient{httpClient: client.httpClient}).generateText(context.Background(), "key", "http://[::1", model, messages, nil, "", logger); requestError == nil {
		t.Fatal("DashScope accepted invalid Responses URL")
	}
	if _, requestError := (xaiResponsesClient{httpClient: client.httpClient}).generateText(context.Background(), "key", "https://provider.test", model, messages, nil, "", nil, nil, logger); requestError == nil {
		t.Fatal("xAI transport error was accepted")
	}
	if _, requestError := (dashScopeResponsesClient{httpClient: client.httpClient}).generateText(context.Background(), "key", "https://provider.test", model, messages, nil, "", logger); requestError == nil {
		t.Fatal("DashScope transport error was accepted")
	}

	client = NewOpenAIClient(geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("{")),
		}, nil
	}), NewEndpoints())
	if _, requestError := (xaiResponsesClient{httpClient: client.httpClient}).generateText(context.Background(), "key", "https://provider.test", model, messages, nil, "", nil, nil, logger); requestError == nil {
		t.Fatal("xAI malformed response was accepted")
	}
	if _, requestError := (dashScopeResponsesClient{httpClient: client.httpClient}).generateText(context.Background(), "key", "https://provider.test", model, messages, nil, "", logger); requestError == nil {
		t.Fatal("DashScope malformed response was accepted")
	}

}

func TestInlineProviderMediaLimitEdges(t *testing.T) {
	image := messageMedia{mediaType: messageMediaTypeImage, mimeType: messageImageMIMEPNG, sizeBytes: 1, data: []byte("x")}
	oneImage := chatMessages{{role: chatRoleUser, content: "inspect", attachments: []messageMedia{image}}}
	twoImages := chatMessages{{role: chatRoleUser, content: "inspect", attachments: []messageMedia{image, image}}}

	requestLimit := CatalogMediaLimit{ID: CatalogMediaLimitIDInlineRequestBytes, MediaType: CatalogMediaLimitTypeAll, Status: CatalogMediaLimitStatusBounded, Value: int64Pointer(4)}
	if limitError := validateInlineMessageMediaBeforeSerialization(textModelDefinition{mediaLimits: []CatalogMediaLimit{requestLimit}}, oneImage); limitError != nil {
		t.Fatalf("request pre-serialization boundary error=%v", limitError)
	}
	if limitError := validateInlineMessageMediaRequestLimit(textModelDefinition{mediaLimits: []CatalogMediaLimit{requestLimit}}, oneImage, []byte("1234")); limitError != nil {
		t.Fatalf("request boundary error=%v", limitError)
	}
	if limitError := validateInlineMessageMediaRequestLimit(textModelDefinition{mediaLimits: []CatalogMediaLimit{requestLimit}}, oneImage, []byte("12345")); !errors.Is(limitError, ErrProviderMediaLimit) {
		t.Fatalf("request above error=%v", limitError)
	}
	requestLimit.Value = int64Pointer(3)
	if limitError := validateInlineMessageMediaBeforeSerialization(textModelDefinition{mediaLimits: []CatalogMediaLimit{requestLimit}}, oneImage); !errors.Is(limitError, ErrProviderMediaLimit) {
		t.Fatalf("encoded request minimum above error=%v", limitError)
	}

	countLimit := CatalogMediaLimit{ID: CatalogMediaLimitIDImageCount, MediaType: string(messageMediaTypeImage), Status: CatalogMediaLimitStatusBounded, Value: int64Pointer(1)}
	if limitError := validateInlineMessageMediaBeforeSerialization(textModelDefinition{mediaLimits: []CatalogMediaLimit{countLimit}}, oneImage); limitError != nil {
		t.Fatalf("count boundary error=%v", limitError)
	}
	if limitError := validateInlineMessageMediaBeforeSerialization(textModelDefinition{mediaLimits: []CatalogMediaLimit{countLimit}}, twoImages); !errors.Is(limitError, ErrProviderMediaLimit) {
		t.Fatalf("count above error=%v", limitError)
	}

	encodedLimit := CatalogMediaLimit{ID: CatalogMediaLimitIDImageInlineBytes, MediaType: string(messageMediaTypeImage), Status: CatalogMediaLimitStatusBounded, Value: int64Pointer(4), Scope: CatalogMediaLimitScopeAttachmentEncodedBytes}
	if limitError := validateInlineMessageMediaBeforeSerialization(textModelDefinition{mediaLimits: []CatalogMediaLimit{encodedLimit}}, oneImage); limitError != nil {
		t.Fatalf("encoded boundary error=%v", limitError)
	}
	encodedLimit.Value = int64Pointer(3)
	if limitError := validateInlineMessageMediaBeforeSerialization(textModelDefinition{mediaLimits: []CatalogMediaLimit{encodedLimit}}, oneImage); !errors.Is(limitError, ErrProviderMediaLimit) {
		t.Fatalf("encoded above error=%v", limitError)
	}

	rawImage := image
	rawImage.sizeBytes = 2
	rawImage.data = []byte("xx")
	rawLimit := CatalogMediaLimit{ID: CatalogMediaLimitIDImageInlineBytes, MediaType: string(messageMediaTypeImage), Status: CatalogMediaLimitStatusBounded, Value: int64Pointer(1), Scope: CatalogMediaLimitScopeAttachment}
	if limitError := validateInlineMessageMediaBeforeSerialization(textModelDefinition{mediaLimits: []CatalogMediaLimit{rawLimit}}, chatMessages{{role: chatRoleUser, attachments: []messageMedia{rawImage}}}); !errors.Is(limitError, ErrProviderMediaLimit) {
		t.Fatalf("raw above error=%v", limitError)
	}
}
