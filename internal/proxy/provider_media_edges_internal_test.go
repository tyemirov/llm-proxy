package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"
)

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
	model := textModelDefinition{
		providerIdentifier: newModelID("provider-model"),
		requestProfile:     requestProfileOpenAIResponsesTemperature,
		outputTokenLimit:   32,
	}
	if _, requestError := openAIClient.openAIRequest(context.Background(), "key", model, closedMessages, false, nil, "", logger); !errors.Is(requestError, errAssetStore) {
		t.Fatalf("OpenAI serialization error=%v", requestError)
	}
	if _, requestError := openAIClient.xAIResponsesRequest(context.Background(), "key", "https://provider.test", model, closedMessages, nil, logger); !errors.Is(requestError, errAssetStore) {
		t.Fatalf("xAI serialization error=%v", requestError)
	}
	if _, requestError := anthropicClient.generateText(context.Background(), "key", "https://provider.test", model, closedMessages, nil, logger); !errors.Is(requestError, errAssetStore) {
		t.Fatalf("Anthropic serialization error=%v", requestError)
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
		Value:     int64Pointer(1),
	}}
	if _, requestError := openAIClient.openAIRequest(context.Background(), "key", model, inlineMessages, false, nil, "", logger); !errors.Is(requestError, ErrProviderMediaLimit) {
		t.Fatalf("OpenAI media limit error=%v", requestError)
	}
	if _, requestError := openAIClient.xAIResponsesRequest(context.Background(), "key", "https://provider.test", model, inlineMessages, nil, logger); !errors.Is(requestError, ErrProviderMediaLimit) {
		t.Fatalf("xAI media limit error=%v", requestError)
	}
	if _, requestError := anthropicClient.generateText(context.Background(), "key", "https://provider.test", model, inlineMessages, nil, logger); !errors.Is(requestError, ErrProviderMediaLimit) {
		t.Fatalf("Anthropic media limit error=%v", requestError)
	}
}

func TestSynchronousResponsesFailureContracts(t *testing.T) {
	logger := zap.NewNop().Sugar()
	messages := chatMessages{{role: chatRoleUser, content: "inspect"}}
	model := textModelDefinition{providerIdentifier: newModelID("grok-test")}

	client := NewOpenAIClient(geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
		return nil, context.Canceled
	}), NewEndpoints())
	if _, requestError := client.xAIResponsesRequest(context.Background(), "key", "http://[::1", model, messages, nil, logger); requestError == nil {
		t.Fatal("xAI accepted invalid Responses URL")
	}
	if _, requestError := client.xAIResponsesRequest(context.Background(), "key", "https://provider.test", model, messages, nil, logger); requestError == nil {
		t.Fatal("xAI transport error was accepted")
	}

	client = NewOpenAIClient(geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("{")),
		}, nil
	}), NewEndpoints())
	if _, requestError := client.xAIResponsesRequest(context.Background(), "key", "https://provider.test", model, messages, nil, logger); requestError == nil {
		t.Fatal("xAI malformed response was accepted")
	}

	for _, testCase := range []struct {
		name     string
		snapshot openAIResponseSnapshot
		want     error
	}{
		{name: "blank completed", snapshot: openAIResponseSnapshot{status: statusCompleted}, want: errors.New(errorOpenAIAPI)},
		{name: "output limit", snapshot: openAIResponseSnapshot{status: statusIncomplete, incompleteReason: "max_output_tokens", text: "partial"}, want: errProviderOutputLimitReached},
		{name: "other incomplete", snapshot: openAIResponseSnapshot{status: statusIncomplete, incompleteReason: "content_filter"}, want: ErrProviderAPI},
		{name: "failed", snapshot: openAIResponseSnapshot{status: statusFailed}, want: errors.New(errorOpenAIFailedStatus)},
		{name: "unknown", snapshot: openAIResponseSnapshot{status: statusInProgress}, want: errors.New(errorOpenAIAPI)},
	} {
		t.Run(testCase.name, func(subTest *testing.T) {
			_, resolveError := resolveSynchronousResponsesSnapshot(testCase.snapshot)
			if resolveError == nil {
				subTest.Fatal("snapshot resolved without error")
			}
			if errors.Is(testCase.want, ErrProviderAPI) && !errors.Is(resolveError, ErrProviderAPI) {
				subTest.Fatalf("resolve error=%v want=%v", resolveError, testCase.want)
			}
			if testCase.want == errProviderOutputLimitReached && !errors.Is(resolveError, errProviderOutputLimitReached) {
				subTest.Fatalf("resolve error=%v want=%v", resolveError, testCase.want)
			}
		})
	}
}

func TestInlineProviderMediaLimitEdges(t *testing.T) {
	image := messageMedia{mediaType: messageMediaTypeImage, mimeType: messageImageMIMEPNG, sizeBytes: 1, data: []byte("x")}
	oneImage := chatMessages{{role: chatRoleUser, content: "inspect", attachments: []messageMedia{image}}}
	twoImages := chatMessages{{role: chatRoleUser, content: "inspect", attachments: []messageMedia{image, image}}}

	requestLimit := CatalogMediaLimit{ID: CatalogMediaLimitIDInlineRequestBytes, MediaType: CatalogMediaLimitTypeAll, Status: CatalogMediaLimitStatusBounded, Value: int64Pointer(4)}
	if limitError := validateInlineMessageMediaLimits(textModelDefinition{mediaLimits: []CatalogMediaLimit{requestLimit}}, oneImage, []byte("1234")); limitError != nil {
		t.Fatalf("request boundary error=%v", limitError)
	}
	if limitError := validateInlineMessageMediaLimits(textModelDefinition{mediaLimits: []CatalogMediaLimit{requestLimit}}, oneImage, []byte("12345")); !errors.Is(limitError, ErrProviderMediaLimit) {
		t.Fatalf("request above error=%v", limitError)
	}

	countLimit := CatalogMediaLimit{ID: CatalogMediaLimitIDImageCount, MediaType: string(messageMediaTypeImage), Status: CatalogMediaLimitStatusBounded, Value: int64Pointer(1)}
	if limitError := validateInlineMessageMediaLimits(textModelDefinition{mediaLimits: []CatalogMediaLimit{countLimit}}, oneImage, nil); limitError != nil {
		t.Fatalf("count boundary error=%v", limitError)
	}
	if limitError := validateInlineMessageMediaLimits(textModelDefinition{mediaLimits: []CatalogMediaLimit{countLimit}}, twoImages, nil); !errors.Is(limitError, ErrProviderMediaLimit) {
		t.Fatalf("count above error=%v", limitError)
	}

	encodedLimit := CatalogMediaLimit{ID: CatalogMediaLimitIDImageInlineBytes, MediaType: string(messageMediaTypeImage), Status: CatalogMediaLimitStatusBounded, Value: int64Pointer(4), Scope: CatalogMediaLimitScopeAttachmentEncodedBytes}
	if limitError := validateInlineMessageMediaLimits(textModelDefinition{mediaLimits: []CatalogMediaLimit{encodedLimit}}, oneImage, nil); limitError != nil {
		t.Fatalf("encoded boundary error=%v", limitError)
	}
	encodedLimit.Value = int64Pointer(3)
	if limitError := validateInlineMessageMediaLimits(textModelDefinition{mediaLimits: []CatalogMediaLimit{encodedLimit}}, oneImage, nil); !errors.Is(limitError, ErrProviderMediaLimit) {
		t.Fatalf("encoded above error=%v", limitError)
	}

	rawImage := image
	rawImage.sizeBytes = 2
	rawImage.data = []byte("xx")
	rawLimit := CatalogMediaLimit{ID: CatalogMediaLimitIDImageInlineBytes, MediaType: string(messageMediaTypeImage), Status: CatalogMediaLimitStatusBounded, Value: int64Pointer(1), Scope: CatalogMediaLimitScopeAttachment}
	if limitError := validateInlineMessageMediaLimits(textModelDefinition{mediaLimits: []CatalogMediaLimit{rawLimit}}, chatMessages{{role: chatRoleUser, attachments: []messageMedia{rawImage}}}, nil); !errors.Is(limitError, ErrProviderMediaLimit) {
		t.Fatalf("raw above error=%v", limitError)
	}
}
