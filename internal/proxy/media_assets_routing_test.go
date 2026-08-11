package proxy_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
)

type mediaAssetResponse struct {
	AssetID   string `json:"asset_id"`
	MIMEType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
	State     string `json:"state"`
}

func TestV2LargeInlineMediaBypassesCompatibilityPromptLimit(t *testing.T) {
	mediaBytes := bytes.Repeat([]byte{0x89, 0x50, 0x4e, 0x47}, 831681)
	digest := sha256.Sum256(mediaBytes)
	var receivedMedia []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/interactions" {
			http.NotFound(responseWriter, request)
			return
		}
		var payload struct {
			Input []struct {
				Content []struct {
					Type string `json:"type"`
					Data string `json:"data"`
				} `json:"content"`
			} `json:"input"`
		}
		if decodeError := json.NewDecoder(request.Body).Decode(&payload); decodeError != nil {
			t.Errorf("decode interaction: %v", decodeError)
			return
		}
		for _, step := range payload.Input {
			for _, content := range step.Content {
				if content.Type == "image" {
					receivedMedia, _ = base64.StdEncoding.DecodeString(content.Data)
				}
			}
		}
		writeGeminiCompletedResponse(responseWriter)
	}))
	defer upstream.Close()
	router := mediaAssetRouter(t, upstream.URL, t.TempDir(), testfixtures.ModelCatalog(t), 60)
	payload := map[string]any{
		"messages": []any{map[string]any{
			"role": "user", "content": "inspect exact image bytes",
			"attachments": []any{map[string]any{
				"type": "image", "mime_type": "image/png",
				"data": base64.StdEncoding.EncodeToString(mediaBytes), "sha256": hex.EncodeToString(digest[:]),
			}},
		}},
	}
	requestBody, _ := json.Marshal(payload)
	if len(requestBody) <= proxy.DefaultMaxPromptBytes {
		t.Fatalf("fixture body=%d want greater than %d", len(requestBody), proxy.DefaultMaxPromptBytes)
	}
	response := postV2MediaRequest(t, router, "secret-a", requestBody)
	if response.Code != http.StatusOK || !bytes.Equal(receivedMedia, mediaBytes) {
		t.Fatalf("status=%d received=%d want=%d", response.Code, len(receivedMedia), len(mediaBytes))
	}
}

func TestGeminiImageCountLimitAdmitsBoundaryAndRejectsOneAbove(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		upstreamCalls++
		_, _ = io.Copy(io.Discard, request.Body)
		writeGeminiCompletedResponse(responseWriter)
	}))
	defer upstream.Close()
	router := mediaAssetRouter(t, upstream.URL, t.TempDir(), testfixtures.ModelCatalog(t), 60)
	attachment := messageMediaPayload("image", "image/png", []byte("x"))
	attachments := make([]map[string]any, 3_600)
	for attachmentIndex := range attachments {
		attachments[attachmentIndex] = attachment
	}
	boundaryBody := mediaV2RequestBody(t, proxy.ModelNameGemini25Flash, "inspect", attachments)
	boundaryResponse := postV2MediaRequest(t, router, "secret-a", []byte(boundaryBody))
	if boundaryResponse.Code != http.StatusOK || upstreamCalls != 1 {
		t.Fatalf("boundary status=%d calls=%d body=%s", boundaryResponse.Code, upstreamCalls, boundaryResponse.Body.String())
	}
	aboveBody := mediaV2RequestBody(t, proxy.ModelNameGemini25Flash, "inspect", append(attachments, attachment))
	aboveResponse := postV2MediaRequest(t, router, "secret-a", []byte(aboveBody))
	if aboveResponse.Code != http.StatusRequestEntityTooLarge || upstreamCalls != 1 || !strings.Contains(aboveResponse.Body.String(), llmproxycontract.ErrorCodeProviderMediaLimitExceeded) {
		t.Fatalf("above status=%d calls=%d body=%s", aboveResponse.Code, upstreamCalls, aboveResponse.Body.String())
	}
}

func TestGeminiInlineRequestLimitSelectsInlineAtBoundaryAndFilesOneAbove(t *testing.T) {
	mediaBytes := []byte("inline-boundary-image")
	digest := sha256.Sum256(mediaBytes)
	var upstream *httptest.Server
	inlineRequestSizes := []int{}
	fileInteractions := 0
	uploadCount := 0
	upstream = httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/upload/v1beta/files":
			responseWriter.Header().Set("X-Goog-Upload-URL", upstream.URL+"/upload-session")
			responseWriter.WriteHeader(http.StatusOK)
		case request.Method == http.MethodPost && request.URL.Path == "/upload-session":
			uploadedBytes, _ := io.ReadAll(request.Body)
			if !bytes.Equal(uploadedBytes, mediaBytes) {
				t.Errorf("uploaded bytes=%q", uploadedBytes)
			}
			uploadCount++
			_, _ = fmt.Fprintf(responseWriter, `{"file":{"name":"files/inline-boundary","mimeType":"image/png","sizeBytes":"%d","sha256Hash":"%s","uri":"%s/files/inline-boundary","state":"ACTIVE"}}`, len(mediaBytes), base64.StdEncoding.EncodeToString(digest[:]), upstream.URL)
		case request.Method == http.MethodPost && request.URL.Path == "/interactions":
			body, _ := io.ReadAll(request.Body)
			if bytes.Contains(body, []byte(`"data"`)) {
				inlineRequestSizes = append(inlineRequestSizes, len(body))
			}
			if bytes.Contains(body, []byte(`"uri"`)) {
				fileInteractions++
			}
			writeGeminiCompletedResponse(responseWriter)
		case request.Method == http.MethodDelete && request.URL.Path == "/files/inline-boundary":
			responseWriter.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(responseWriter, request)
		}
	}))
	defer upstream.Close()
	requestBody := []byte(mediaV2RequestBody(t, proxy.ModelNameGemini25Flash, "inspect", []map[string]any{
		messageMediaPayload("image", "image/png", mediaBytes),
	}))
	executeCatalog := func(catalog proxy.ModelCatalog) *httptest.ResponseRecorder {
		return postV2MediaRequest(t, mediaAssetRouter(t, upstream.URL, t.TempDir(), catalog, 60), "secret-a", requestBody)
	}
	executeBounded := func(inlineLimit int64) *httptest.ResponseRecorder {
		catalog := testfixtures.ModelCatalog(t)
		setGeminiMediaLimit(t, &catalog, proxy.ModelNameGemini25Flash, proxy.CatalogMediaLimitIDInlineRequestBytes, inlineLimit)
		setGeminiMediaLimit(t, &catalog, proxy.ModelNameGemini25Flash, proxy.CatalogMediaLimitIDImageFileBytes, int64(len(mediaBytes)))
		return executeCatalog(catalog)
	}
	initialResponse := executeBounded(20_000_000)
	if initialResponse.Code != http.StatusOK || len(inlineRequestSizes) != 1 {
		t.Fatalf("initial status=%d inline sizes=%v body=%s", initialResponse.Code, inlineRequestSizes, initialResponse.Body.String())
	}
	inlineBoundary := int64(inlineRequestSizes[0])
	boundaryResponse := executeBounded(inlineBoundary)
	aboveResponse := executeBounded(inlineBoundary - 1)
	if boundaryResponse.Code != http.StatusOK || aboveResponse.Code != http.StatusOK || len(inlineRequestSizes) != 2 || int64(inlineRequestSizes[1]) != inlineBoundary || uploadCount != 1 || fileInteractions != 1 {
		t.Fatalf("boundary=%d statuses=%d,%d inline=%v uploads=%d file_interactions=%d", inlineBoundary, boundaryResponse.Code, aboveResponse.Code, inlineRequestSizes, uploadCount, fileInteractions)
	}
	for _, status := range []string{proxy.CatalogMediaLimitStatusUnbounded, proxy.CatalogMediaLimitStatusUnknown} {
		catalog := testfixtures.ModelCatalog(t)
		setGeminiMediaLimitStatus(t, &catalog, proxy.ModelNameGemini25Flash, proxy.CatalogMediaLimitIDInlineRequestBytes, status)
		response := executeCatalog(catalog)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%s response=%d body=%s", status, response.Code, response.Body.String())
		}
	}
	if len(inlineRequestSizes) != 4 || uploadCount != 1 || fileInteractions != 1 {
		t.Fatalf("non-bounded inline=%v uploads=%d file_interactions=%d", inlineRequestSizes, uploadCount, fileInteractions)
	}
}

func TestTenantAssetReferenceValidationAndExactRouting(t *testing.T) {
	mediaBytes := []byte("tenant-owned-image-bytes")
	var receivedMedia []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(request.Body).Decode(&payload)
		encoded := payload["input"].([]any)[0].(map[string]any)["content"].([]any)[1].(map[string]any)["data"].(string)
		receivedMedia, _ = base64.StdEncoding.DecodeString(encoded)
		writeGeminiCompletedResponse(responseWriter)
	}))
	defer upstream.Close()
	assetRoot := t.TempDir()
	router := mediaAssetRouter(t, upstream.URL, assetRoot, testfixtures.ModelCatalog(t), 60)
	asset := uploadTestAsset(t, router, "secret-a", "image/png", mediaBytes)

	assetPayload := v2AssetReferencePayload(asset.AssetID, asset.MIMEType, asset.SHA256)
	response := postV2MediaRequest(t, router, "secret-a", assetPayload)
	if response.Code != http.StatusOK || !bytes.Equal(receivedMedia, mediaBytes) {
		t.Fatalf("status=%d body=%s received=%q", response.Code, response.Body.String(), receivedMedia)
	}

	for _, testCase := range []struct {
		name       string
		secret     string
		payload    []byte
		wantStatus int
	}{
		{name: "foreign tenant", secret: "secret-b", payload: assetPayload, wantStatus: http.StatusNotFound},
		{name: "missing asset", secret: "secret-a", payload: v2AssetReferencePayload("ast_00000000000000000000000000000000", asset.MIMEType, asset.SHA256), wantStatus: http.StatusNotFound},
		{name: "wrong MIME", secret: "secret-a", payload: v2AssetReferencePayload(asset.AssetID, "image/jpeg", asset.SHA256), wantStatus: http.StatusBadRequest},
		{name: "wrong digest", secret: "secret-a", payload: v2AssetReferencePayload(asset.AssetID, asset.MIMEType, strings.Repeat("0", 64)), wantStatus: http.StatusBadRequest},
		{name: "both union variants", secret: "secret-a", payload: v2ConflictingAttachmentPayload(asset, mediaBytes), wantStatus: http.StatusBadRequest},
	} {
		t.Run(testCase.name, func(subTest *testing.T) {
			validationResponse := postV2MediaRequest(subTest, router, testCase.secret, testCase.payload)
			if validationResponse.Code != testCase.wantStatus {
				subTest.Fatalf("status=%d body=%s", validationResponse.Code, validationResponse.Body.String())
			}
		})
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, llmproxycontract.AssetPath+"/"+asset.AssetID+"?key=secret-a", nil)
	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}
	deletedResponse := postV2MediaRequest(t, router, "secret-a", assetPayload)
	if deletedResponse.Code != http.StatusGone || !strings.Contains(deletedResponse.Body.String(), "asset_deleted") {
		t.Fatalf("deleted status=%d body=%s", deletedResponse.Code, deletedResponse.Body.String())
	}
}

func TestGeminiFileTransportPreservesAssetBytesAndCleansUp(t *testing.T) {
	mediaBytes := bytes.Repeat([]byte("media"), 300)
	digest := sha256.Sum256(mediaBytes)
	var upstream *httptest.Server
	var mutex sync.Mutex
	uploadedBytes := []byte(nil)
	interactionURI := ""
	cleanupCount := 0
	upstream = httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/upload/v1beta/files":
			responseWriter.Header().Set("X-Goog-Upload-URL", upstream.URL+"/upload-session")
			responseWriter.WriteHeader(http.StatusOK)
		case request.Method == http.MethodPost && request.URL.Path == "/upload-session":
			body, _ := io.ReadAll(request.Body)
			mutex.Lock()
			uploadedBytes = body
			mutex.Unlock()
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(responseWriter, `{"file":{"name":"files/media-1","mimeType":"image/png","sizeBytes":"%d","sha256Hash":"%s","uri":"%s/files/media-1","state":"ACTIVE"}}`, len(mediaBytes), base64.StdEncoding.EncodeToString(digest[:]), upstream.URL)
		case request.Method == http.MethodPost && request.URL.Path == "/interactions":
			var payload map[string]any
			_ = json.NewDecoder(request.Body).Decode(&payload)
			interactionURI = payload["input"].([]any)[0].(map[string]any)["content"].([]any)[1].(map[string]any)["uri"].(string)
			writeGeminiCompletedResponse(responseWriter)
		case request.Method == http.MethodDelete && request.URL.Path == "/files/media-1":
			cleanupCount++
			responseWriter.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(responseWriter, request)
		}
	}))
	defer upstream.Close()
	catalog := testfixtures.ModelCatalog(t)
	setGeminiMediaLimit(t, &catalog, proxy.ModelNameGemini25Flash, proxy.CatalogMediaLimitIDInlineRequestBytes, 256)
	setGeminiMediaLimit(t, &catalog, proxy.ModelNameGemini25Flash, proxy.CatalogMediaLimitIDImageFileBytes, int64(len(mediaBytes)))
	router := mediaAssetRouter(t, upstream.URL, t.TempDir(), catalog, 60)
	asset := uploadTestAsset(t, router, "secret-a", "image/png", mediaBytes)
	response := postV2MediaRequest(t, router, "secret-a", v2AssetReferencePayload(asset.AssetID, asset.MIMEType, asset.SHA256))
	mutex.Lock()
	defer mutex.Unlock()
	if response.Code != http.StatusOK || !bytes.Equal(uploadedBytes, mediaBytes) || interactionURI != upstream.URL+"/files/media-1" || cleanupCount != 1 {
		t.Fatalf("status=%d upload=%d uri=%q cleanup=%d body=%s", response.Code, len(uploadedBytes), interactionURI, cleanupCount, response.Body.String())
	}
}

func TestDeletingAssetAfterAdmissionKeepsTheOpenRequestStable(t *testing.T) {
	mediaBytes := bytes.Repeat([]byte("admitted-media"), 100)
	digest := sha256.Sum256(mediaBytes)
	startReached := make(chan struct{})
	releaseStart := make(chan struct{})
	var upstream *httptest.Server
	var uploadedBytes []byte
	upstream = httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/upload/v1beta/files":
			close(startReached)
			<-releaseStart
			responseWriter.Header().Set("X-Goog-Upload-URL", upstream.URL+"/upload-session")
			responseWriter.WriteHeader(http.StatusOK)
		case request.Method == http.MethodPost && request.URL.Path == "/upload-session":
			uploadedBytes, _ = io.ReadAll(request.Body)
			_, _ = fmt.Fprintf(responseWriter, `{"file":{"name":"files/admitted","mimeType":"image/png","sizeBytes":"%d","sha256Hash":"%s","uri":"%s/files/admitted","state":"ACTIVE"}}`, len(mediaBytes), base64.StdEncoding.EncodeToString(digest[:]), upstream.URL)
		case request.Method == http.MethodPost && request.URL.Path == "/interactions":
			writeGeminiCompletedResponse(responseWriter)
		case request.Method == http.MethodDelete && request.URL.Path == "/files/admitted":
			responseWriter.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(responseWriter, request)
		}
	}))
	defer upstream.Close()
	catalog := testfixtures.ModelCatalog(t)
	setGeminiMediaLimit(t, &catalog, proxy.ModelNameGemini25Flash, proxy.CatalogMediaLimitIDInlineRequestBytes, 1)
	router := mediaAssetRouter(t, upstream.URL, t.TempDir(), catalog, 60)
	asset := uploadTestAsset(t, router, "secret-a", "image/png", mediaBytes)
	requestComplete := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		requestComplete <- postV2MediaRequest(t, router, "secret-a", v2AssetReferencePayload(asset.AssetID, asset.MIMEType, asset.SHA256))
	}()
	select {
	case <-startReached:
	case <-time.After(2 * time.Second):
		t.Fatal("provider file admission did not start")
	}
	deleteRequest := httptest.NewRequest(http.MethodDelete, llmproxycontract.AssetPath+"/"+asset.AssetID+"?key=secret-a", nil)
	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, deleteRequest)
	close(releaseStart)
	response := <-requestComplete
	if deleteResponse.Code != http.StatusNoContent || response.Code != http.StatusOK || !bytes.Equal(uploadedBytes, mediaBytes) {
		t.Fatalf("delete=%d request=%d uploaded=%d body=%s", deleteResponse.Code, response.Code, len(uploadedBytes), response.Body.String())
	}
}

func TestExpiredAssetAndProviderMediaLimitsFailWithStableCodes(t *testing.T) {
	t.Run("expired asset", func(subTest *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			subTest.Error("expired asset reached provider")
		}))
		defer upstream.Close()
		router := mediaAssetRouter(subTest, upstream.URL, subTest.TempDir(), testfixtures.ModelCatalog(subTest), 1)
		asset := uploadTestAsset(subTest, router, "secret-a", "image/png", []byte("expires"))
		time.Sleep(1100 * time.Millisecond)
		response := postV2MediaRequest(subTest, router, "secret-a", v2AssetReferencePayload(asset.AssetID, asset.MIMEType, asset.SHA256))
		if response.Code != http.StatusGone || !strings.Contains(response.Body.String(), "asset_expired") {
			subTest.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("configured file limit", func(subTest *testing.T) {
		upstreamCalls := 0
		upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			upstreamCalls++
		}))
		defer upstream.Close()
		catalog := testfixtures.ModelCatalog(subTest)
		setGeminiMediaLimit(subTest, &catalog, proxy.ModelNameGemini25Flash, proxy.CatalogMediaLimitIDInlineRequestBytes, 1)
		setGeminiMediaLimit(subTest, &catalog, proxy.ModelNameGemini25Flash, proxy.CatalogMediaLimitIDImageFileBytes, int64(len("above-limit")-1))
		router := mediaAssetRouter(subTest, upstream.URL, subTest.TempDir(), catalog, 60)
		asset := uploadTestAsset(subTest, router, "secret-a", "image/png", []byte("above-limit"))
		response := postV2MediaRequest(subTest, router, "secret-a", v2AssetReferencePayload(asset.AssetID, asset.MIMEType, asset.SHA256))
		if response.Code != http.StatusRequestEntityTooLarge || !strings.Contains(response.Body.String(), llmproxycontract.ErrorCodeProviderMediaLimitExceeded) || upstreamCalls != 0 {
			subTest.Fatalf("status=%d calls=%d body=%s", response.Code, upstreamCalls, response.Body.String())
		}
	})

	t.Run("upstream media limit", func(subTest *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
			responseWriter.WriteHeader(http.StatusRequestEntityTooLarge)
		}))
		defer upstream.Close()
		router := mediaAssetRouter(subTest, upstream.URL, subTest.TempDir(), testfixtures.ModelCatalog(subTest), 60)
		mediaBytes := []byte("inline-media")
		digest := sha256.Sum256(mediaBytes)
		requestBody, _ := json.Marshal(map[string]any{"messages": []any{map[string]any{
			"role": "user", "content": "inspect",
			"attachments": []any{map[string]any{"type": "image", "mime_type": "image/png", "data": base64.StdEncoding.EncodeToString(mediaBytes), "sha256": hex.EncodeToString(digest[:])}},
		}}})
		response := postV2MediaRequest(subTest, router, "secret-a", requestBody)
		if response.Code != http.StatusRequestEntityTooLarge || !strings.Contains(response.Body.String(), llmproxycontract.ErrorCodeProviderMediaLimitExceeded) {
			subTest.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})
}

func mediaAssetRouter(t *testing.T, geminiBaseURL string, assetRoot string, catalog proxy.ModelCatalog, retentionSeconds int) http.Handler {
	t.Helper()
	configuration := proxy.Configuration{
		Tenants: []proxy.TenantConfiguration{
			{ID: "tenant-a", Secret: "secret-a", Defaults: proxy.TenantDefaults{Provider: proxy.ProviderNameGemini, Model: proxy.ModelNameGemini25Flash, DictationProvider: proxy.ProviderNameOpenAI, DictationModel: proxy.DefaultDictationModel}},
			{ID: "tenant-b", Secret: "secret-b", Defaults: proxy.TenantDefaults{Provider: proxy.ProviderNameGemini, Model: proxy.ModelNameGemini25Flash, DictationProvider: proxy.ProviderNameOpenAI, DictationModel: proxy.DefaultDictationModel}},
		},
		OpenAIKey: "openai-key", GeminiKey: "gemini-key", GeminiBaseURL: geminiBaseURL,
		WorkerCount: 2, QueueSize: 4, RequestTimeoutSeconds: 10, MaxPromptBytes: proxy.DefaultMaxPromptBytes,
		MaxAssetBytes: 10 * 1024 * 1024, AssetRetentionSeconds: retentionSeconds, AssetStorePath: assetRoot,
		ModelCatalog: catalog,
	}
	router, buildError := proxy.BuildRouter(configuration, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf("BuildRouter error: %v", buildError)
	}
	return router
}

func uploadTestAsset(t *testing.T, router http.Handler, secret string, mimeType string, data []byte) mediaAssetResponse {
	t.Helper()
	digest := sha256.Sum256(data)
	request := httptest.NewRequest(http.MethodPost, llmproxycontract.AssetPath+"?key="+secret, bytes.NewReader(data))
	request.Header.Set("Content-Type", mimeType)
	request.Header.Set(llmproxycontract.HeaderAssetSHA256, hex.EncodeToString(digest[:]))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", response.Code, response.Body.String())
	}
	var asset mediaAssetResponse
	if decodeError := json.Unmarshal(response.Body.Bytes(), &asset); decodeError != nil {
		t.Fatalf("decode asset: %v", decodeError)
	}
	return asset
}

func postV2MediaRequest(t *testing.T, router http.Handler, secret string, payload []byte) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v2?key="+secret+"&format=text/plain", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func v2AssetReferencePayload(assetID string, mimeType string, digest string) []byte {
	payload, _ := json.Marshal(map[string]any{"messages": []any{map[string]any{
		"role": "user", "content": "inspect asset",
		"attachments": []any{map[string]any{"type": "image", "asset_id": assetID, "mime_type": mimeType, "sha256": digest}},
	}}})
	return payload
}

func v2ConflictingAttachmentPayload(asset mediaAssetResponse, data []byte) []byte {
	payload, _ := json.Marshal(map[string]any{"messages": []any{map[string]any{
		"role": "user", "content": "inspect asset",
		"attachments": []any{map[string]any{"type": "image", "asset_id": asset.AssetID, "data": base64.StdEncoding.EncodeToString(data), "mime_type": asset.MIMEType, "sha256": asset.SHA256}},
	}}})
	return payload
}

func setGeminiMediaLimit(t *testing.T, catalog *proxy.ModelCatalog, model string, limitID string, value int64) {
	t.Helper()
	for offeringIndex := range catalog.Offerings {
		offering := &catalog.Offerings[offeringIndex]
		if offering.Provider != proxy.ProviderNameGemini || offering.Model != model {
			continue
		}
		for limitIndex := range offering.MediaLimits {
			if offering.MediaLimits[limitIndex].ID == limitID {
				offering.MediaLimits[limitIndex].Value = &value
				return
			}
		}
	}
	t.Fatalf("missing Gemini media limit=%s", limitID)
}

func setGeminiMediaLimitStatus(t *testing.T, catalog *proxy.ModelCatalog, model string, limitID string, status string) {
	t.Helper()
	for offeringIndex := range catalog.Offerings {
		offering := &catalog.Offerings[offeringIndex]
		if offering.Provider != proxy.ProviderNameGemini || offering.Model != model {
			continue
		}
		for limitIndex := range offering.MediaLimits {
			if offering.MediaLimits[limitIndex].ID == limitID {
				offering.MediaLimits[limitIndex].Status = status
				offering.MediaLimits[limitIndex].Value = nil
				return
			}
		}
	}
	t.Fatalf("missing Gemini media limit=%s", limitID)
}

func writeGeminiCompletedResponse(responseWriter http.ResponseWriter) {
	responseWriter.Header().Set("Content-Type", "application/json")
	_, _ = responseWriter.Write([]byte(`{"status":"completed","steps":[{"type":"model_output","content":[{"type":"text","text":"accepted"}]}]}`))
}
