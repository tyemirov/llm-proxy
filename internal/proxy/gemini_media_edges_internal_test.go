package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type geminiEdgeDoer func(*http.Request) (*http.Response, error)

func (doer geminiEdgeDoer) Do(request *http.Request) (*http.Response, error) {
	return doer(request)
}

type geminiErrorReader struct{}

func (geminiErrorReader) Read([]byte) (int, error) {
	return 0, errAssetEdge
}

func geminiEdgeResponse(status int, body io.Reader, headers http.Header) *http.Response {
	if headers == nil {
		headers = http.Header{}
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(body), Header: headers}
}

func TestGeminiFileRequestFailureContracts(t *testing.T) {
	newRequest := func(requestContext context.Context) *http.Request {
		request, requestError := http.NewRequestWithContext(requestContext, http.MethodGet, "https://provider.test/files/media", nil)
		if requestError != nil {
			t.Fatalf("request: %v", requestError)
		}
		return request
	}

	cancelledContext, cancel := context.WithCancel(context.Background())
	cancel()
	testCases := []struct {
		name      string
		context   context.Context
		do        geminiEdgeDoer
		wantError error
	}{
		{
			name:      "context error",
			context:   cancelledContext,
			do:        func(*http.Request) (*http.Response, error) { return nil, errAssetEdge },
			wantError: context.Canceled,
		},
		{
			name:      "queue full",
			context:   context.Background(),
			do:        func(*http.Request) (*http.Response, error) { return nil, errQueueFull },
			wantError: errQueueFull,
		},
		{
			name:      "provider transport",
			context:   context.Background(),
			do:        func(*http.Request) (*http.Response, error) { return nil, errAssetEdge },
			wantError: ErrProviderAPI,
		},
		{
			name:    "read response",
			context: context.Background(),
			do: func(*http.Request) (*http.Response, error) {
				return geminiEdgeResponse(http.StatusOK, geminiErrorReader{}, nil), nil
			},
			wantError: ErrProviderAPI,
		},
		{
			name:    "oversized response",
			context: context.Background(),
			do: func(*http.Request) (*http.Response, error) {
				return geminiEdgeResponse(http.StatusOK, strings.NewReader(strings.Repeat("x", 1024*1024+1)), nil), nil
			},
			wantError: ErrProviderAPI,
		},
		{
			name:    "media limit",
			context: context.Background(),
			do: func(*http.Request) (*http.Response, error) {
				return geminiEdgeResponse(http.StatusRequestEntityTooLarge, strings.NewReader("limit"), nil), nil
			},
			wantError: ErrProviderMediaLimit,
		},
		{
			name:    "provider status",
			context: context.Background(),
			do: func(*http.Request) (*http.Response, error) {
				return geminiEdgeResponse(http.StatusInternalServerError, strings.NewReader("failure"), nil), nil
			},
			wantError: ErrProviderAPI,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			client := newGeminiInteractionsClient(testCase.do)
			_, _, requestError := client.performGeminiFileRequest(newRequest(testCase.context))
			if !errors.Is(requestError, testCase.wantError) {
				subTest.Fatalf("error=%v want=%v", requestError, testCase.wantError)
			}
		})
	}
	client := newGeminiInteractionsClient(geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
		return geminiEdgeResponse(http.StatusOK, strings.NewReader("ok"), http.Header{"X-Test": []string{"yes"}}), nil
	}))
	body, headers, requestError := client.performGeminiFileRequest(newRequest(context.Background()))
	if requestError != nil || string(body) != "ok" || headers.Get("X-Test") != "yes" {
		t.Fatalf("body=%q headers=%v error=%v", body, headers, requestError)
	}
}

func TestGeminiFileUploadValidationAndPollingContracts(t *testing.T) {
	baseURL := "https://provider.test/v1beta"
	mediaBytes := []byte("gemini-file")
	digestBytes := sha256.Sum256(mediaBytes)
	digest := hex.EncodeToString(digestBytes[:])
	encodedDigest := base64.StdEncoding.EncodeToString(digestBytes[:])
	attachment := &messageMedia{mediaType: messageMediaTypeImage, mimeType: "image/png", sha256: digest, sizeBytes: int64(len(mediaBytes)), data: mediaBytes}
	fileJSON := func(state string) string {
		return `{"file":{"name":"files/media_1","mimeType":"image/png","sizeBytes":"11","sha256Hash":"` + encodedDigest + `","uri":"https://provider.test/v1beta/files/media_1","state":"` + state + `"}}`
	}
	directFileJSON := func(state string) string {
		return `{"name":"files/media_1","mimeType":"image/png","sizeBytes":"11","sha256Hash":"` + encodedDigest + `","uri":"https://provider.test/v1beta/files/media_1","state":"` + state + `"}`
	}
	startHeaders := http.Header{}
	startHeaders.Set(geminiUploadURLHeader, "https://provider.test/upload-session")

	t.Run("start request failure", func(subTest *testing.T) {
		client := newGeminiInteractionsClient(geminiEdgeDoer(func(*http.Request) (*http.Response, error) { return nil, errAssetEdge }))
		if _, uploadError := client.uploadGeminiFile(context.Background(), "key", baseURL, attachment); !errors.Is(uploadError, ErrProviderAPI) {
			subTest.Fatalf("error=%v", uploadError)
		}
	})

	t.Run("foreign upload URL", func(subTest *testing.T) {
		client := newGeminiInteractionsClient(geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
			return geminiEdgeResponse(http.StatusOK, strings.NewReader(""), http.Header{geminiUploadURLHeader: []string{"https://foreign.test/upload"}}), nil
		}))
		if _, uploadError := client.uploadGeminiFile(context.Background(), "key", baseURL, attachment); !errors.Is(uploadError, ErrProviderAPI) {
			subTest.Fatalf("error=%v", uploadError)
		}
	})

	t.Run("asset reader", func(subTest *testing.T) {
		closedFile, fileError := os.CreateTemp(subTest.TempDir(), "closed")
		if fileError != nil {
			subTest.Fatalf("file: %v", fileError)
		}
		_ = closedFile.Close()
		assetAttachment := *attachment
		assetAttachment.data = nil
		assetAttachment.asset = &tenantAssetReader{file: closedFile}
		client := newGeminiInteractionsClient(geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
			return geminiEdgeResponse(http.StatusOK, strings.NewReader(""), startHeaders), nil
		}))
		if _, uploadError := client.uploadGeminiFile(context.Background(), "key", baseURL, &assetAttachment); !errors.Is(uploadError, errAssetStore) {
			subTest.Fatalf("error=%v", uploadError)
		}
	})

	t.Run("upload request failure", func(subTest *testing.T) {
		calls := 0
		client := newGeminiInteractionsClient(geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return geminiEdgeResponse(http.StatusOK, strings.NewReader(""), startHeaders), nil
			}
			return nil, errAssetEdge
		}))
		if _, uploadError := client.uploadGeminiFile(context.Background(), "key", baseURL, attachment); !errors.Is(uploadError, ErrProviderAPI) {
			subTest.Fatalf("error=%v", uploadError)
		}
	})

	t.Run("invalid upload response", func(subTest *testing.T) {
		calls := 0
		client := newGeminiInteractionsClient(geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return geminiEdgeResponse(http.StatusOK, strings.NewReader(""), startHeaders), nil
			}
			return geminiEdgeResponse(http.StatusOK, strings.NewReader("{"), nil), nil
		}))
		if _, uploadError := client.uploadGeminiFile(context.Background(), "key", baseURL, attachment); !errors.Is(uploadError, ErrProviderAPI) {
			subTest.Fatalf("error=%v", uploadError)
		}
	})

	t.Run("processing cancellation", func(subTest *testing.T) {
		requestContext, cancelRequest := context.WithCancel(context.Background())
		calls := 0
		client := newGeminiInteractionsClient(geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return geminiEdgeResponse(http.StatusOK, strings.NewReader(""), startHeaders), nil
			}
			cancelRequest()
			return geminiEdgeResponse(http.StatusOK, strings.NewReader(fileJSON(geminiFileStateProcessing)), nil), nil
		}))
		uploadedFile, uploadError := client.uploadGeminiFile(requestContext, "key", baseURL, attachment)
		if !errors.Is(uploadError, context.Canceled) || uploadedFile.name != "files/media_1" {
			subTest.Fatalf("file=%+v error=%v", uploadedFile, uploadError)
		}
	})

	t.Run("processing poll failure", func(subTest *testing.T) {
		calls := 0
		client := newGeminiInteractionsClient(geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
			calls++
			switch calls {
			case 1:
				return geminiEdgeResponse(http.StatusOK, strings.NewReader(""), startHeaders), nil
			case 2:
				return geminiEdgeResponse(http.StatusOK, strings.NewReader(fileJSON(geminiFileStateProcessing)), nil), nil
			default:
				return nil, errAssetEdge
			}
		}))
		uploadedFile, uploadError := client.uploadGeminiFile(context.Background(), "key", baseURL, attachment)
		if !errors.Is(uploadError, ErrProviderAPI) || uploadedFile.name != "files/media_1" {
			subTest.Fatalf("file=%+v error=%v", uploadedFile, uploadError)
		}
	})

	t.Run("processing becomes active", func(subTest *testing.T) {
		calls := 0
		client := newGeminiInteractionsClient(geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
			calls++
			switch calls {
			case 1:
				return geminiEdgeResponse(http.StatusOK, strings.NewReader(""), startHeaders), nil
			case 2:
				return geminiEdgeResponse(http.StatusOK, strings.NewReader(fileJSON(geminiFileStateProcessing)), nil), nil
			default:
				return geminiEdgeResponse(http.StatusOK, strings.NewReader(directFileJSON(geminiFileStateActive)), nil), nil
			}
		}))
		uploadedFile, uploadError := client.uploadGeminiFile(context.Background(), "key", baseURL, attachment)
		if uploadError != nil || uploadedFile.name != "files/media_1" {
			subTest.Fatalf("file=%+v error=%v", uploadedFile, uploadError)
		}
	})

	t.Run("terminal non-active state", func(subTest *testing.T) {
		calls := 0
		client := newGeminiInteractionsClient(geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return geminiEdgeResponse(http.StatusOK, strings.NewReader(""), startHeaders), nil
			}
			return geminiEdgeResponse(http.StatusOK, strings.NewReader(fileJSON("FAILED")), nil), nil
		}))
		uploadedFile, uploadError := client.uploadGeminiFile(context.Background(), "key", baseURL, attachment)
		if !errors.Is(uploadError, ErrProviderAPI) || uploadedFile.name != "files/media_1" {
			subTest.Fatalf("file=%+v error=%v", uploadedFile, uploadError)
		}
	})

	if _, validationError := validatedGeminiFile([]byte("{"), attachment, baseURL); !errors.Is(validationError, ErrProviderAPI) {
		t.Fatalf("decode validation error=%v", validationError)
	}
	validFile, validationError := validatedGeminiFile([]byte(fileJSON(geminiFileStateActive)), attachment, baseURL)
	if validationError != nil || validFile.Name != "files/media_1" {
		t.Fatalf("file=%+v error=%v", validFile, validationError)
	}
	invalidAttachment := *attachment
	invalidAttachment.sha256 = "bad"
	if _, validationError := validatedGeminiFileObject(validFile, &invalidAttachment, baseURL); !errors.Is(validationError, ErrProviderAPI) {
		t.Fatalf("object validation error=%v", validationError)
	}
}

func TestGeminiFileResourceAndInteractionInputEdges(t *testing.T) {
	if geminiFilesUploadURL("%") != "" {
		t.Fatal("invalid upload base URL accepted")
	}
	for _, fileName := range []string{"media", "files/", "files/a/b"} {
		if _, resourceError := geminiFileResourceURL("https://provider.test", fileName); !errors.Is(resourceError, ErrProviderAPI) {
			t.Fatalf("file=%q error=%v", fileName, resourceError)
		}
	}
	resourceURL, resourceError := geminiFileResourceURL("https://provider.test", "files/media")
	if resourceError != nil || resourceURL != "https://provider.test/files/media" {
		t.Fatalf("url=%q error=%v", resourceURL, resourceError)
	}
	if providerOwnedFileURI("https://foreign.test/files/media", "https://provider.test", "files/media") {
		t.Fatal("foreign file URI accepted")
	}

	client := newGeminiInteractionsClient(geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
		return nil, errAssetEdge
	}))
	attachmentData := []byte("a")
	digest := sha256.Sum256(attachmentData)
	attachment := &messageMedia{mediaType: messageMediaTypeAudio, mimeType: "audio/wav", sha256: hex.EncodeToString(digest[:]), sizeBytes: 1, data: attachmentData}
	if _, getError := client.getGeminiFile(context.Background(), "key", "https://provider.test", "bad", attachment); !errors.Is(getError, ErrProviderAPI) {
		t.Fatalf("invalid name error=%v", getError)
	}
	if _, getError := client.getGeminiFile(context.Background(), "key", "%", "files/media", attachment); !errors.Is(getError, ErrProviderAPI) {
		t.Fatalf("invalid URL error=%v", getError)
	}
	if _, getError := client.getGeminiFile(context.Background(), "key", "https://provider.test", "files/media", attachment); !errors.Is(getError, ErrProviderAPI) {
		t.Fatalf("request error=%v", getError)
	}

	client.httpClient = geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
		return geminiEdgeResponse(http.StatusOK, strings.NewReader("{"), nil), nil
	})
	if _, getError := client.getGeminiFile(context.Background(), "key", "https://provider.test", "files/media", attachment); !errors.Is(getError, ErrProviderAPI) {
		t.Fatalf("decode error=%v", getError)
	}

	fileDigest := base64.StdEncoding.EncodeToString(digest[:])
	client.httpClient = geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
		body := `{"name":"files/media","mimeType":"audio/wav","sizeBytes":"1","sha256Hash":"` + fileDigest + `","uri":"https://provider.test/files/media","state":"ACTIVE"}`
		return geminiEdgeResponse(http.StatusOK, strings.NewReader(body), nil), nil
	})
	if file, getError := client.getGeminiFile(context.Background(), "key", "https://provider.test", "files/media", attachment); getError != nil || file.Name != "files/media" {
		t.Fatalf("file=%+v error=%v", file, getError)
	}

	model := textModelDefinition{providerIdentifier: newModelID("models/gemini"), mediaLimits: []CatalogMediaLimit{
		{ID: CatalogMediaLimitIDImageCount, MediaType: "image", Status: CatalogMediaLimitStatusUnbounded},
		{ID: CatalogMediaLimitIDAudioCount, MediaType: "audio", Status: CatalogMediaLimitStatusBounded, Value: int64Pointer(1)},
		{ID: CatalogMediaLimitIDAudioFileBytes, MediaType: "audio", Status: CatalogMediaLimitStatusBounded, Value: int64Pointer(1)},
	}}
	messages := chatMessages{{role: chatRoleSystem, content: "system"}, {role: chatRoleAssistant, content: "answer"}, {role: chatRoleUser, content: "listen", attachments: []messageMedia{*attachment, *attachment}}}
	if countError := validateGeminiMediaCounts(model, messages); !errors.Is(countError, ErrProviderMediaLimit) {
		t.Fatalf("count error=%v", countError)
	}
	largeAudio := *attachment
	largeAudio.sizeBytes = 2
	if limitError := validateGeminiFileLimit(model, &largeAudio); !errors.Is(limitError, ErrProviderMediaLimit) {
		t.Fatalf("file limit error=%v", limitError)
	}
	if limitError := validateGeminiFileLimit(model, attachment); limitError != nil {
		t.Fatalf("boundary file error=%v", limitError)
	}

	if _, _, inputError := messages.geminiInteractionInput([]string{"only-one"}); !errors.Is(inputError, ErrProviderAPI) {
		t.Fatalf("URI count error=%v", inputError)
	}
	if _, _, inputError := messages.geminiInteractionInput([]string{"one", "two", "three"}); !errors.Is(inputError, ErrProviderAPI) {
		t.Fatalf("extra URI error=%v", inputError)
	}
	if input, systemInstruction, inputError := messages.geminiInteractionInput([]string{"one", "two"}); inputError != nil || systemInstruction != "system" || len(input) != 2 || input[0].Type != geminiInteractionStepModelOutput {
		t.Fatalf("input=%+v system=%q error=%v", input, systemInstruction, inputError)
	}

	closedFile, fileError := os.CreateTemp(t.TempDir(), "closed-input")
	if fileError != nil {
		t.Fatalf("file: %v", fileError)
	}
	_ = closedFile.Close()
	closedMessages := chatMessages{{role: chatRoleUser, content: "listen", attachments: []messageMedia{{mediaType: messageMediaTypeAudio, mimeType: "audio/wav", sizeBytes: 1, asset: &tenantAssetReader{file: closedFile}}}}}
	if _, _, inputError := closedMessages.geminiInteractionInput(nil); !errors.Is(inputError, errAssetStore) {
		t.Fatalf("closed input error=%v", inputError)
	}

	if geminiInteractionPayloadHasMedia("not a payload") || geminiInteractionPayloadHasMedia(geminiInteractionRequest{}) {
		t.Fatal("non-media payload classified as media")
	}
	if !geminiInteractionPayloadHasMedia(geminiInteractionRequest{Input: []geminiInteractionStep{{Content: []geminiInteractionContent{{Type: "audio"}}}}}) {
		t.Fatal("audio payload not classified as media")
	}
	usage, usageError := parseGeminiInteractionTokenUsage(&geminiInteractionTokenUsage{TotalInputTokens: intPointer(1), TotalOutputTokens: intPointer(2), TotalTokens: intPointer(3)})
	if usageError != nil || usage == nil {
		t.Fatalf("usage=%+v error=%v", usage, usageError)
	}
	if nilUsage, usageError := parseGeminiInteractionTokenUsage(nil); usageError != nil || nilUsage != nil {
		t.Fatalf("nil usage=%+v error=%v", nilUsage, usageError)
	}

	cleanupClient := newGeminiInteractionsClient(geminiEdgeDoer(func(*http.Request) (*http.Response, error) { return nil, errAssetEdge }))
	if cleanupError := cleanupClient.releaseGeminiFiles(context.Background(), "key", "https://provider.test", []geminiUploadedFile{{name: "bad"}, {name: "files/media"}}); !errors.Is(cleanupError, ErrProviderAPI) {
		t.Fatalf("cleanup error=%v", cleanupError)
	}
}

func TestGeminiPrepareInteractionPayloadFailureContracts(t *testing.T) {
	data := []byte("media")
	digest := sha256.Sum256(data)
	inlineAttachment := messageMedia{mediaType: messageMediaTypeImage, mimeType: "image/png", sha256: hex.EncodeToString(digest[:]), sizeBytes: int64(len(data)), data: data}
	messages := chatMessages{{role: chatRoleUser, content: "inspect", attachments: []messageMedia{inlineAttachment}}}
	client := newGeminiInteractionsClient(geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
		return nil, errAssetEdge
	}))
	if _, _, prepareError := client.prepareInteractionPayload(context.Background(), "key", "https://provider.test", textModelDefinition{}, messages, nil, false, nil); !errors.Is(prepareError, ErrProviderMediaLimit) {
		t.Fatalf("missing inline limit error=%v", prepareError)
	}

	closedFile, fileError := os.CreateTemp(t.TempDir(), "closed-prepare")
	if fileError != nil {
		t.Fatalf("file: %v", fileError)
	}
	_ = closedFile.Close()
	closedAttachment := inlineAttachment
	closedAttachment.data = nil
	closedAttachment.asset = &tenantAssetReader{file: closedFile}
	unboundedModel := textModelDefinition{mediaLimits: []CatalogMediaLimit{
		{ID: CatalogMediaLimitIDInlineRequestBytes, MediaType: CatalogMediaLimitTypeAll, Status: CatalogMediaLimitStatusUnbounded},
		{ID: CatalogMediaLimitIDImageCount, MediaType: "image", Status: CatalogMediaLimitStatusUnbounded},
	}}
	closedMessages := chatMessages{{role: chatRoleUser, content: "inspect", attachments: []messageMedia{closedAttachment}}}
	if _, _, prepareError := client.prepareInteractionPayload(context.Background(), "key", "https://provider.test", unboundedModel, closedMessages, nil, false, nil); !errors.Is(prepareError, errAssetStore) {
		t.Fatalf("inline payload error=%v", prepareError)
	}

	boundedModel := textModelDefinition{mediaLimits: []CatalogMediaLimit{
		{ID: CatalogMediaLimitIDInlineRequestBytes, MediaType: CatalogMediaLimitTypeAll, Status: CatalogMediaLimitStatusBounded, Value: int64Pointer(1)},
		{ID: CatalogMediaLimitIDImageCount, MediaType: "image", Status: CatalogMediaLimitStatusUnbounded},
		{ID: CatalogMediaLimitIDImageFileBytes, MediaType: "image", Status: CatalogMediaLimitStatusUnbounded},
	}}
	if _, _, prepareError := client.prepareInteractionPayload(context.Background(), "key", "https://provider.test", boundedModel, messages, nil, false, nil); !errors.Is(prepareError, ErrProviderAPI) {
		t.Fatalf("upload error=%v", prepareError)
	}
	if _, requestError := newGeminiInteractionRequest(textModelDefinition{}, messages, []string{"one", "two"}, nil, false, nil); !errors.Is(requestError, ErrProviderAPI) {
		t.Fatalf("request input error=%v", requestError)
	}
}

func TestGeminiFinalizedFileIsCleanedAfterProcessingCancellation(t *testing.T) {
	data := []byte("processing-media")
	digest := sha256.Sum256(data)
	attachment := messageMedia{mediaType: messageMediaTypeImage, mimeType: "image/png", sha256: hex.EncodeToString(digest[:]), sizeBytes: int64(len(data)), data: data}
	messages := chatMessages{{role: chatRoleUser, content: "inspect", attachments: []messageMedia{attachment}}}
	model := textModelDefinition{mediaLimits: []CatalogMediaLimit{
		{ID: CatalogMediaLimitIDInlineRequestBytes, MediaType: CatalogMediaLimitTypeAll, Status: CatalogMediaLimitStatusBounded, Value: int64Pointer(1)},
		{ID: CatalogMediaLimitIDImageCount, MediaType: "image", Status: CatalogMediaLimitStatusUnbounded},
		{ID: CatalogMediaLimitIDImageFileBytes, MediaType: "image", Status: CatalogMediaLimitStatusUnbounded},
	}}
	requestContext, cancelRequest := context.WithCancel(context.Background())
	calls := 0
	cleanupCalls := 0
	encodedDigest := base64.StdEncoding.EncodeToString(digest[:])
	processingBody := `{"file":{"name":"files/processing","mimeType":"image/png","sizeBytes":"16","sha256Hash":"` + encodedDigest + `","uri":"https://provider.test/files/processing","state":"PROCESSING"}}`
	if _, validationError := validatedGeminiFile([]byte(processingBody), &attachment, "https://provider.test"); validationError != nil {
		t.Fatalf("processing file validation: %v", validationError)
	}
	startHeaders := http.Header{}
	startHeaders.Set(geminiUploadURLHeader, "https://provider.test/upload-session")
	client := newGeminiInteractionsClient(geminiEdgeDoer(func(request *http.Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			return geminiEdgeResponse(http.StatusOK, strings.NewReader(""), startHeaders), nil
		case 2:
			cancelRequest()
			return geminiEdgeResponse(http.StatusOK, strings.NewReader(processingBody), nil), nil
		default:
			if request.Method != http.MethodDelete || request.URL.Path != "/files/processing" || request.Context().Err() != nil {
				t.Errorf("cleanup request method=%s path=%s context_error=%v", request.Method, request.URL.Path, request.Context().Err())
			}
			cleanupCalls++
			return geminiEdgeResponse(http.StatusNoContent, strings.NewReader(""), nil), nil
		}
	}))
	_, uploadedFiles, prepareError := client.prepareInteractionPayload(requestContext, "key", "https://provider.test", model, messages, nil, false, nil)
	if !errors.Is(prepareError, context.Canceled) || len(uploadedFiles) != 1 || uploadedFiles[0].name != "files/processing" {
		t.Fatalf("calls=%d files=%+v error=%v", calls, uploadedFiles, prepareError)
	}
	if cleanupError := client.releaseGeminiFiles(requestContext, "key", "https://provider.test", uploadedFiles); cleanupError != nil || cleanupCalls != 1 {
		t.Fatalf("cleanup_error=%v cleanup_calls=%d", cleanupError, cleanupCalls)
	}
}

func int64Pointer(value int64) *int64 {
	return &value
}

func intPointer(value int) *int {
	return &value
}
