package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tyemirov/llm-proxy/internal/constants"
	"github.com/tyemirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

const (
	geminiAPIKeyHeader                    = "x-goog-api-key"
	geminiAPIRevisionHeader               = "Api-Revision"
	geminiAPIRevisionValue                = "2026-05-20"
	geminiInteractionStatusRequiresAction = "requires_action"
	geminiInteractionStatusBudgetExceeded = "budget_exceeded"
	geminiInteractionStepUserInput        = "user_input"
	geminiInteractionStepModelOutput      = "model_output"
	geminiInteractionContentText          = "text"
	geminiInteractionCleanupTimeout       = 5 * time.Second
	geminiFileStateActive                 = "ACTIVE"
	geminiFileStateProcessing             = "PROCESSING"
	geminiFilePollInterval                = 500 * time.Millisecond
)

var geminiFileNamePattern = regexp.MustCompile(`^files/[A-Za-z0-9_-]+$`)

const (
	geminiUploadProtocolHeader     = "X-Goog-Upload-Protocol"
	geminiUploadCommandHeader      = "X-Goog-Upload-Command"
	geminiUploadHeaderLengthHeader = "X-Goog-Upload-Header-Content-Length"
	geminiUploadHeaderTypeHeader   = "X-Goog-Upload-Header-Content-Type"
	geminiUploadOffsetHeader       = "X-Goog-Upload-Offset"
	geminiUploadURLHeader          = "X-Goog-Upload-URL"
	geminiUploadProtocolResumable  = "resumable"
	geminiUploadCommandStart       = "start"
	geminiUploadCommandFinalize    = "upload, finalize"
)

type geminiInteractionsClient struct {
	httpClient             HTTPDoer
	performInteractionHTTP geminiInteractionHTTPPerformer
}

type geminiInteractionHTTPPerformer func(HTTPDoer, *http.Request, *zap.SugaredLogger) (int, []byte, http.Header, error)

type geminiInteractionCleanupMode uint8

const (
	geminiInteractionDeleteOnly geminiInteractionCleanupMode = iota
	geminiInteractionCancelAndDelete
)

type geminiInteractionRequest struct {
	Model             string                           `json:"model"`
	Input             []geminiInteractionStep          `json:"input"`
	SystemInstruction string                           `json:"system_instruction,omitempty"`
	GenerationConfig  *geminiInteractionGeneration     `json:"generation_config,omitempty"`
	Background        bool                             `json:"background"`
	Store             bool                             `json:"store"`
	ResponseFormat    []geminiStructuredResponseFormat `json:"response_format,omitempty"`
}

type geminiInteractionGeneration struct {
	MaxOutputTokens int `json:"max_output_tokens,omitempty"`
}

type geminiInteractionStep struct {
	Type    string                     `json:"type"`
	Content []geminiInteractionContent `json:"content,omitempty"`
}

type geminiInteractionContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	URI      string `json:"uri,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
}

type geminiFileResponse struct {
	File geminiFile `json:"file"`
}

type geminiFile struct {
	Name       string `json:"name"`
	MIMEType   string `json:"mimeType"`
	SizeBytes  string `json:"sizeBytes"`
	SHA256Hash string `json:"sha256Hash"`
	URI        string `json:"uri"`
	State      string `json:"state"`
}

type geminiUploadedFile struct {
	name string
	uri  string
}

type geminiInteractionResponse struct {
	ID     string                       `json:"id"`
	Status string                       `json:"status"`
	Steps  []geminiInteractionStep      `json:"steps"`
	Usage  *geminiInteractionTokenUsage `json:"usage"`
}

type geminiInteractionTokenUsage struct {
	TotalInputTokens  *int `json:"total_input_tokens"`
	TotalOutputTokens *int `json:"total_output_tokens"`
	TotalTokens       *int `json:"total_tokens"`
}

type geminiInteractionSnapshot struct {
	identifier string
	status     string
	text       string
	usage      *tokenUsage
}

func newGeminiInteractionsClient(httpClient HTTPDoer) *geminiInteractionsClient {
	return newGeminiInteractionsClientWithHTTPPerformer(httpClient, performRetryingGeminiInteractionHTTP)
}

func newGeminiInteractionsClientWithHTTPPerformer(httpClient HTTPDoer, performer geminiInteractionHTTPPerformer) *geminiInteractionsClient {
	return &geminiInteractionsClient{
		httpClient:             httpClient,
		performInteractionHTTP: performer,
	}
}

func performRetryingGeminiInteractionHTTP(httpClient HTTPDoer, httpRequest *http.Request, structuredLogger *zap.SugaredLogger) (int, []byte, http.Header, error) {
	statusCode, responseBytes, responseHeader, _, requestError := utils.PerformHTTPRequest(httpClient.Do, httpRequest, structuredLogger, logEventProviderRequestError)
	return statusCode, responseBytes, responseHeader, requestError
}

func (client *geminiInteractionsClient) generateText(parentContext context.Context, apiKey string, baseURL string, modelIdentifier textModelDefinition, messages chatMessages, maxTokens *int, executionLifecycle textExecutionLifecycle, structuredOutput *structuredOutputSchema, structuredLogger *zap.SugaredLogger) (generation textGenerationResult, generationError error) {
	background := executionLifecycle == textExecutionLifecyclePollableResource
	payload, uploadedFiles, payloadError := client.prepareInteractionPayload(parentContext, apiKey, baseURL, modelIdentifier, messages, maxTokens, background, structuredOutput)
	if len(uploadedFiles) > 0 {
		defer func() {
			cleanupError := client.releaseGeminiFiles(parentContext, apiKey, baseURL, uploadedFiles)
			if cleanupError == nil {
				return
			}
			client.logFileCleanupError(cleanupError, structuredLogger)
			generation = textGenerationResult{usage: generation.usage}
			if errors.Is(generationError, errProviderOutputLimitReached) {
				generationError = cleanupError
				return
			}
			generationError = errors.Join(generationError, cleanupError)
		}()
	}
	if payloadError != nil {
		return textGenerationResult{}, payloadError
	}

	snapshot, createError := client.createInteraction(parentContext, apiKey, baseURL, payload, structuredLogger)
	if createError != nil {
		if background && !utils.IsBlank(snapshot.identifier) {
			cleanupError := client.releaseInteraction(parentContext, apiKey, baseURL, snapshot.identifier, snapshot.cleanupMode(), structuredLogger)
			if cleanupError != nil {
				client.logInteractionCleanupError(cleanupError, structuredLogger)
			}
		}
		return textGenerationResult{usage: snapshot.usage}, createError
	}
	if !background {
		if snapshot.isPending() {
			return textGenerationResult{usage: snapshot.usage}, fmt.Errorf("%w: synchronous Gemini Interaction returned active status=%s", ErrProviderAPI, snapshot.status)
		}
		return snapshot.resolve()
	}
	if utils.IsBlank(snapshot.identifier) {
		return textGenerationResult{usage: snapshot.usage}, fmt.Errorf("%w: Gemini Interactions create response missing id", ErrProviderAPI)
	}

	interactionIdentifier := snapshot.identifier
	latestUsage := snapshot.usage
	cleanupMode := snapshot.cleanupMode()
	defer func() {
		cleanupError := client.releaseInteraction(parentContext, apiKey, baseURL, interactionIdentifier, cleanupMode, structuredLogger)
		if cleanupError == nil {
			return
		}
		client.logInteractionCleanupError(cleanupError, structuredLogger)
		if generationError == nil || errors.Is(generationError, errProviderOutputLimitReached) {
			generation = textGenerationResult{usage: generation.usage}
			generationError = cleanupError
		}
	}()

	for snapshot.isPending() {
		if waitError := waitForRequestTelemetryPhase(parentContext, responsePollInterval, requestTelemetryPhaseProviderPollWait); waitError != nil {
			return textGenerationResult{usage: latestUsage}, parentContext.Err()
		}

		polledSnapshot, pollError := client.getInteraction(parentContext, apiKey, baseURL, interactionIdentifier, structuredLogger)
		if polledSnapshot.usage != nil {
			latestUsage = polledSnapshot.usage
		}
		if pollError != nil {
			if parentContext.Err() != nil {
				return textGenerationResult{usage: latestUsage}, parentContext.Err()
			}
			return textGenerationResult{usage: latestUsage}, pollError
		}
		snapshot = polledSnapshot
		cleanupMode = snapshot.cleanupMode()
	}

	snapshot.usage = latestUsage
	return snapshot.resolve()
}

func (client *geminiInteractionsClient) createInteraction(parentContext context.Context, apiKey string, baseURL string, payload geminiInteractionRequest, structuredLogger *zap.SugaredLogger) (geminiInteractionSnapshot, error) {
	responseBytes, requestError := client.performInteractionRequest(
		parentContext,
		http.MethodPost,
		geminiInteractionsURL(baseURL),
		apiKey,
		payload,
		structuredLogger,
	)
	if requestError != nil {
		return geminiInteractionSnapshot{}, requestError
	}
	return newGeminiInteractionSnapshot(responseBytes)
}

func (client *geminiInteractionsClient) getInteraction(parentContext context.Context, apiKey string, baseURL string, interactionIdentifier string, structuredLogger *zap.SugaredLogger) (geminiInteractionSnapshot, error) {
	responseBytes, requestError := client.performInteractionRequest(
		parentContext,
		http.MethodGet,
		geminiInteractionResourceURL(baseURL, interactionIdentifier),
		apiKey,
		nil,
		structuredLogger,
	)
	if requestError != nil {
		return geminiInteractionSnapshot{}, requestError
	}
	return newGeminiInteractionSnapshot(responseBytes)
}

func (client *geminiInteractionsClient) releaseInteraction(parentContext context.Context, apiKey string, baseURL string, interactionIdentifier string, cleanupMode geminiInteractionCleanupMode, structuredLogger *zap.SugaredLogger) error {
	detachedContext := context.WithoutCancel(parentContext)
	var cancelError error
	if cleanupMode == geminiInteractionCancelAndDelete {
		cancelError = client.performInteractionCleanupRequest(
			detachedContext,
			http.MethodPost,
			geminiInteractionResourceURL(baseURL, interactionIdentifier)+"/cancel",
			apiKey,
			structuredLogger,
		)
	}
	deleteError := client.performInteractionCleanupRequest(
		detachedContext,
		http.MethodDelete,
		geminiInteractionResourceURL(baseURL, interactionIdentifier),
		apiKey,
		structuredLogger,
	)
	if cancelError != nil {
		return cancelError
	}
	return deleteError
}

func (client *geminiInteractionsClient) performInteractionCleanupRequest(parentContext context.Context, method string, requestURL string, apiKey string, structuredLogger *zap.SugaredLogger) error {
	cleanupContext, cancelCleanup := context.WithTimeout(parentContext, geminiInteractionCleanupTimeout)
	defer cancelCleanup()
	_, requestError := client.performInteractionRequest(cleanupContext, method, requestURL, apiKey, nil, structuredLogger)
	return requestError
}

func (client *geminiInteractionsClient) performInteractionRequest(parentContext context.Context, method string, requestURL string, apiKey string, payload any, structuredLogger *zap.SugaredLogger) ([]byte, error) {
	var requestBody io.Reader
	if payload != nil {
		payloadBytes, _ := json.Marshal(payload)
		requestBody = bytes.NewReader(payloadBytes)
	}
	httpRequest, buildError := http.NewRequestWithContext(parentContext, method, requestURL, requestBody)
	if buildError != nil {
		structuredLogger.Errorw(logEventBuildHTTPRequest, constants.LogFieldError, buildError)
		return nil, buildError
	}
	httpRequest.Header.Set(geminiAPIKeyHeader, strings.TrimSpace(apiKey))
	httpRequest.Header.Set(geminiAPIRevisionHeader, geminiAPIRevisionValue)
	if payload != nil {
		httpRequest.Header.Set(headerContentType, mimeApplicationJSON)
	}

	statusCode, responseBytes, responseHeader, requestError := client.performInteractionHTTP(client.httpClient, httpRequest, structuredLogger)
	if statusCode == http.StatusRequestEntityTooLarge && geminiInteractionPayloadHasMedia(payload) {
		return nil, newProviderMediaLimitHTTPError(statusCode, responseHeader)
	}
	if responseError := providerResponseError(statusCode, responseHeader, requestError); responseError != nil {
		return nil, responseError
	}
	return responseBytes, nil
}

func geminiInteractionPayloadHasMedia(payload any) bool {
	interaction, isInteraction := payload.(geminiInteractionRequest)
	if !isInteraction {
		return false
	}
	for _, step := range interaction.Input {
		for _, content := range step.Content {
			if content.Type == string(messageMediaTypeImage) || content.Type == string(messageMediaTypeAudio) {
				return true
			}
		}
	}
	return false
}

func (client *geminiInteractionsClient) logInteractionCleanupError(cleanupError error, structuredLogger *zap.SugaredLogger) {
	structuredLogger.Errorw(
		"Gemini interaction cleanup error",
		constants.LogFieldError, cleanupError,
	)
}

func (client *geminiInteractionsClient) logFileCleanupError(cleanupError error, structuredLogger *zap.SugaredLogger) {
	structuredLogger.Errorw(
		"Gemini file cleanup error",
		constants.LogFieldError, cleanupError,
	)
}

func (client *geminiInteractionsClient) prepareInteractionPayload(parentContext context.Context, apiKey string, baseURL string, model textModelDefinition, messages chatMessages, maxTokens *int, background bool, structuredOutput *structuredOutputSchema) (geminiInteractionRequest, []geminiUploadedFile, error) {
	inlineLimit, hasInlineLimit := catalogMediaLimit(model.mediaLimits, CatalogMediaLimitIDInlineRequestBytes, messageMediaTypeImage)
	mediaCount := messages.mediaCount()
	if mediaCount == 0 {
		payload, payloadError := newGeminiInteractionRequest(model, messages, nil, maxTokens, background, structuredOutput)
		return payload, nil, payloadError
	}
	if !hasInlineLimit {
		return geminiInteractionRequest{}, nil, ErrProviderMediaLimit
	}
	if countError := validateGeminiMediaCounts(model, messages); countError != nil {
		return geminiInteractionRequest{}, nil, countError
	}
	inlinePossible := inlineLimit.Status != CatalogMediaLimitStatusBounded || messages.base64MediaBytes() < *inlineLimit.Value
	if inlinePossible {
		inlinePayload, payloadError := newGeminiInteractionRequest(model, messages, nil, maxTokens, background, structuredOutput)
		if payloadError != nil {
			return geminiInteractionRequest{}, nil, payloadError
		}
		payloadBytes, _ := json.Marshal(inlinePayload)
		if inlineLimit.Status != CatalogMediaLimitStatusBounded || int64(len(payloadBytes)) <= *inlineLimit.Value {
			return inlinePayload, nil, nil
		}
	}

	uploadedFiles := make([]geminiUploadedFile, 0, mediaCount)
	mediaURIs := make([]string, 0, mediaCount)
	for messageIndex := range messages {
		for attachmentIndex := range messages[messageIndex].attachments {
			attachment := &messages[messageIndex].attachments[attachmentIndex]
			if limitError := validateGeminiFileLimit(model, attachment); limitError != nil {
				return geminiInteractionRequest{}, uploadedFiles, limitError
			}
			uploadedFile, uploadError := client.uploadGeminiFile(parentContext, apiKey, baseURL, attachment)
			if uploadedFile.name != constants.EmptyString {
				uploadedFiles = append(uploadedFiles, uploadedFile)
			}
			if uploadError != nil {
				return geminiInteractionRequest{}, uploadedFiles, uploadError
			}
			mediaURIs = append(mediaURIs, uploadedFile.uri)
		}
	}
	payload, payloadError := newGeminiInteractionRequest(model, messages, mediaURIs, maxTokens, background, structuredOutput)
	return payload, uploadedFiles, payloadError
}

func newGeminiInteractionRequest(model textModelDefinition, messages chatMessages, mediaURIs []string, maxTokens *int, background bool, structuredOutput *structuredOutputSchema) (geminiInteractionRequest, error) {
	input, systemInstruction, inputError := messages.geminiInteractionInput(mediaURIs)
	if inputError != nil {
		return geminiInteractionRequest{}, inputError
	}
	payload := geminiInteractionRequest{
		Model:             model.providerString(),
		Input:             input,
		SystemInstruction: systemInstruction,
		Background:        background,
		Store:             background,
		ResponseFormat:    geminiStructuredResponseFormats(structuredOutput),
	}
	if maxTokens != nil {
		payload.GenerationConfig = &geminiInteractionGeneration{MaxOutputTokens: *maxTokens}
	}
	return payload, nil
}

func (messages chatMessages) mediaCount() int {
	count := 0
	for _, message := range messages {
		count += len(message.attachments)
	}
	return count
}

func (messages chatMessages) mediaTypeCount(mediaType messageMediaType) int64 {
	var count int64
	for _, message := range messages {
		for _, attachment := range message.attachments {
			if attachment.mediaType == mediaType {
				count++
			}
		}
	}
	return count
}

func (messages chatMessages) base64MediaBytes() int64 {
	var encodedBytes int64
	for _, message := range messages {
		for _, attachment := range message.attachments {
			encodedBytes += ((attachment.sizeBytes + 2) / 3) * 4
		}
	}
	return encodedBytes
}

func validateGeminiMediaCounts(model textModelDefinition, messages chatMessages) error {
	for _, mediaType := range []messageMediaType{messageMediaTypeImage, messageMediaTypeAudio} {
		limitID := CatalogMediaLimitIDImageCount
		if mediaType == messageMediaTypeAudio {
			limitID = CatalogMediaLimitIDAudioCount
		}
		limit, bounded := boundedCatalogMediaLimit(model.mediaLimits, limitID, mediaType)
		if bounded && messages.mediaTypeCount(mediaType) > limit {
			return ErrProviderMediaLimit
		}
	}
	return nil
}

func validateGeminiFileLimit(model textModelDefinition, attachment *messageMedia) error {
	limitID := CatalogMediaLimitIDImageFileBytes
	if attachment.mediaType == messageMediaTypeAudio {
		limitID = CatalogMediaLimitIDAudioFileBytes
	}
	limit, bounded := boundedCatalogMediaLimit(model.mediaLimits, limitID, attachment.mediaType)
	if bounded && attachment.sizeBytes > limit {
		return ErrProviderMediaLimit
	}
	return nil
}

func (client *geminiInteractionsClient) uploadGeminiFile(parentContext context.Context, apiKey string, baseURL string, attachment *messageMedia) (geminiUploadedFile, error) {
	startPayload := []byte(`{"file":{"display_name":"llm-proxy-media"}}`)
	startRequest, _ := http.NewRequestWithContext(parentContext, http.MethodPost, geminiFilesUploadURL(baseURL), bytes.NewReader(startPayload))
	startRequest.Header.Set(geminiAPIKeyHeader, strings.TrimSpace(apiKey))
	startRequest.Header.Set(headerContentType, mimeApplicationJSON)
	startRequest.Header.Set(geminiUploadProtocolHeader, geminiUploadProtocolResumable)
	startRequest.Header.Set(geminiUploadCommandHeader, geminiUploadCommandStart)
	startRequest.Header.Set(geminiUploadHeaderLengthHeader, strconv.FormatInt(attachment.sizeBytes, 10))
	startRequest.Header.Set(geminiUploadHeaderTypeHeader, attachment.mimeType)
	_, startHeader, startError := client.performGeminiFileRequest(startRequest)
	if startError != nil {
		return geminiUploadedFile{}, startError
	}
	uploadURL := strings.TrimSpace(startHeader.Get(geminiUploadURLHeader))
	if !providerOwnedURL(uploadURL, baseURL) {
		return geminiUploadedFile{}, ErrProviderAPI
	}
	mediaReader, readerError := attachment.reader()
	if readerError != nil {
		return geminiUploadedFile{}, readerError
	}
	uploadRequest, _ := http.NewRequestWithContext(parentContext, http.MethodPost, uploadURL, mediaReader)
	uploadRequest.ContentLength = attachment.sizeBytes
	uploadRequest.Header.Set(headerContentType, attachment.mimeType)
	uploadRequest.Header.Set(geminiUploadCommandHeader, geminiUploadCommandFinalize)
	uploadRequest.Header.Set(geminiUploadOffsetHeader, "0")
	responseBytes, _, uploadError := client.performGeminiFileRequest(uploadRequest)
	if uploadError != nil {
		return geminiUploadedFile{}, uploadError
	}
	file, parseError := validatedGeminiFile(responseBytes, attachment, baseURL)
	if parseError != nil {
		return geminiUploadedFile{}, parseError
	}
	uploadedFile := geminiUploadedFile{name: file.Name, uri: file.URI}
	for file.State == geminiFileStateProcessing {
		waitTimer := time.NewTimer(geminiFilePollInterval)
		select {
		case <-parentContext.Done():
			waitTimer.Stop()
			return uploadedFile, parentContext.Err()
		case <-waitTimer.C:
		}
		file, parseError = client.getGeminiFile(parentContext, apiKey, baseURL, file.Name, attachment)
		if parseError != nil {
			return uploadedFile, parseError
		}
		uploadedFile = geminiUploadedFile{name: file.Name, uri: file.URI}
	}
	if file.State != geminiFileStateActive {
		return uploadedFile, ErrProviderAPI
	}
	return uploadedFile, nil
}

func (client *geminiInteractionsClient) getGeminiFile(parentContext context.Context, apiKey string, baseURL string, fileName string, attachment *messageMedia) (geminiFile, error) {
	requestURL, urlError := geminiFileResourceURL(baseURL, fileName)
	if urlError != nil {
		return geminiFile{}, urlError
	}
	request, buildError := http.NewRequestWithContext(parentContext, http.MethodGet, requestURL, nil)
	if buildError != nil {
		return geminiFile{}, ErrProviderAPI
	}
	request.Header.Set(geminiAPIKeyHeader, strings.TrimSpace(apiKey))
	responseBytes, _, requestError := client.performGeminiFileRequest(request)
	if requestError != nil {
		return geminiFile{}, requestError
	}
	var file geminiFile
	if decodeError := json.Unmarshal(responseBytes, &file); decodeError != nil {
		return geminiFile{}, ErrProviderAPI
	}
	return validatedGeminiFileObject(file, attachment, baseURL)
}

func validatedGeminiFile(responseBytes []byte, attachment *messageMedia, baseURL string) (geminiFile, error) {
	var response geminiFileResponse
	if decodeError := json.Unmarshal(responseBytes, &response); decodeError != nil {
		return geminiFile{}, ErrProviderAPI
	}
	return validatedGeminiFileObject(response.File, attachment, baseURL)
}

func validatedGeminiFileObject(file geminiFile, attachment *messageMedia, baseURL string) (geminiFile, error) {
	sizeBytes, sizeError := strconv.ParseInt(file.SizeBytes, 10, 64)
	digestBytes, digestError := base64.StdEncoding.DecodeString(file.SHA256Hash)
	expectedDigest, expectedDigestError := hex.DecodeString(attachment.sha256)
	if sizeError != nil || sizeBytes != attachment.sizeBytes || digestError != nil || expectedDigestError != nil || !bytes.Equal(digestBytes, expectedDigest) || file.MIMEType != attachment.mimeType || !geminiFileNamePattern.MatchString(file.Name) || !providerOwnedFileURI(file.URI, baseURL, file.Name) {
		return geminiFile{}, ErrProviderAPI
	}
	return file, nil
}

func (client *geminiInteractionsClient) performGeminiFileRequest(request *http.Request) ([]byte, http.Header, error) {
	response, requestError := client.httpClient.Do(request)
	if requestError != nil {
		if contextError := request.Context().Err(); contextError != nil {
			return nil, nil, contextError
		}
		if errors.Is(requestError, errQueueFull) {
			return nil, nil, requestError
		}
		return nil, nil, ErrProviderAPI
	}
	defer response.Body.Close()
	responseBytes, readError := io.ReadAll(io.LimitReader(response.Body, 1024*1024+1))
	if readError != nil || len(responseBytes) > 1024*1024 {
		return nil, nil, ErrProviderAPI
	}
	if response.StatusCode == http.StatusRequestEntityTooLarge {
		return nil, response.Header, newProviderMediaLimitHTTPError(response.StatusCode, response.Header)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, response.Header, newProviderHTTPError(response.StatusCode, response.Header)
	}
	return responseBytes, response.Header, nil
}

func (client *geminiInteractionsClient) releaseGeminiFiles(parentContext context.Context, apiKey string, baseURL string, files []geminiUploadedFile) error {
	cleanupErrors := make([]error, 0)
	for _, file := range files {
		requestURL, urlError := geminiFileResourceURL(baseURL, file.name)
		if urlError != nil {
			cleanupErrors = append(cleanupErrors, urlError)
			continue
		}
		cleanupContext, cancelCleanup := context.WithTimeout(context.WithoutCancel(parentContext), geminiInteractionCleanupTimeout)
		request, buildError := http.NewRequestWithContext(cleanupContext, http.MethodDelete, requestURL, nil)
		if buildError == nil {
			request.Header.Set(geminiAPIKeyHeader, strings.TrimSpace(apiKey))
			_, _, buildError = client.performGeminiFileRequest(request)
		}
		cancelCleanup()
		if buildError != nil {
			cleanupErrors = append(cleanupErrors, buildError)
		}
	}
	return errors.Join(cleanupErrors...)
}

func geminiFilesUploadURL(baseURL string) string {
	parsedURL, parseError := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if parseError != nil {
		return constants.EmptyString
	}
	parsedURL.Path = strings.TrimSuffix(parsedURL.Path, "/v1beta") + "/upload/v1beta/files"
	parsedURL.RawQuery = constants.EmptyString
	parsedURL.Fragment = constants.EmptyString
	return parsedURL.String()
}

func geminiFileResourceURL(baseURL string, fileName string) (string, error) {
	identifier := strings.TrimPrefix(fileName, "files/")
	if identifier == fileName || identifier == constants.EmptyString || strings.Contains(identifier, "/") {
		return constants.EmptyString, ErrProviderAPI
	}
	return strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/files/" + url.PathEscape(identifier), nil
}

func providerOwnedURL(candidate string, baseURL string) bool {
	candidateURL, candidateError := url.Parse(candidate)
	providerURL, providerError := url.Parse(baseURL)
	return candidateError == nil && providerError == nil && candidateURL.IsAbs() && candidateURL.Scheme == providerURL.Scheme && candidateURL.Host == providerURL.Host && candidateURL.User == nil && candidateURL.Fragment == constants.EmptyString
}

func providerOwnedFileURI(candidate string, baseURL string, fileName string) bool {
	if !providerOwnedURL(candidate, baseURL) {
		return false
	}
	candidateURL, parseError := url.Parse(candidate)
	return parseError == nil && strings.HasSuffix(strings.TrimRight(candidateURL.Path, "/"), "/"+fileName)
}

func (messages chatMessages) geminiInteractionInput(mediaURIs []string) ([]geminiInteractionStep, string, error) {
	input := make([]geminiInteractionStep, 0, len(messages))
	systemInstructions := []string{}
	mediaIndex := 0
	for _, message := range messages {
		if message.role == chatRoleSystem {
			systemInstructions = append(systemInstructions, message.content)
			continue
		}

		stepType := geminiInteractionStepUserInput
		if message.role == chatRoleAssistant {
			stepType = geminiInteractionStepModelOutput
		}
		content := []geminiInteractionContent{{
			Type: geminiInteractionContentText,
			Text: message.content,
		}}
		for attachmentIndex := range message.attachments {
			attachment := &message.attachments[attachmentIndex]
			mediaContent := geminiInteractionContent{Type: string(attachment.mediaType), MIMEType: attachment.mimeType}
			if len(mediaURIs) > 0 {
				if mediaIndex >= len(mediaURIs) {
					return nil, constants.EmptyString, ErrProviderAPI
				}
				mediaContent.URI = mediaURIs[mediaIndex]
			} else {
				attachmentBytes, attachmentError := attachment.bytes()
				if attachmentError != nil {
					return nil, constants.EmptyString, attachmentError
				}
				mediaContent.Data = base64.StdEncoding.EncodeToString(attachmentBytes)
			}
			mediaIndex++
			content = append(content, mediaContent)
		}
		input = append(input, geminiInteractionStep{Type: stepType, Content: content})
	}
	if len(mediaURIs) > 0 && mediaIndex != len(mediaURIs) {
		return nil, constants.EmptyString, ErrProviderAPI
	}
	return input, strings.Join(systemInstructions, "\n\n"), nil
}

func geminiInteractionsURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/interactions"
}

func geminiInteractionResourceURL(baseURL string, interactionIdentifier string) string {
	return geminiInteractionsURL(baseURL) + "/" + url.PathEscape(interactionIdentifier)
}

func newGeminiInteractionSnapshot(responseBytes []byte) (geminiInteractionSnapshot, error) {
	var response geminiInteractionResponse
	if decodeError := json.Unmarshal(responseBytes, &response); decodeError != nil {
		return geminiInteractionSnapshot{}, decodeError
	}
	usage, usageError := parseGeminiInteractionTokenUsage(response.Usage)
	snapshot := geminiInteractionSnapshot{
		identifier: strings.TrimSpace(response.ID),
		status:     strings.TrimSpace(response.Status),
		text:       visibleGeminiInteractionText(response.Steps),
		usage:      usage,
	}
	if usageError != nil {
		return snapshot, usageError
	}
	return snapshot, nil
}

func (snapshot geminiInteractionSnapshot) isPending() bool {
	return snapshot.status == statusQueued || snapshot.status == statusInProgress
}

func (snapshot geminiInteractionSnapshot) cleanupMode() geminiInteractionCleanupMode {
	if snapshot.isPending() {
		return geminiInteractionCancelAndDelete
	}
	return geminiInteractionDeleteOnly
}

func (snapshot geminiInteractionSnapshot) resolve() (textGenerationResult, error) {
	generation := textGenerationResult{text: snapshot.text, usage: snapshot.usage}
	switch snapshot.status {
	case statusCompleted:
		if utils.IsBlank(snapshot.text) {
			return textGenerationResult{usage: snapshot.usage}, fmt.Errorf("%w: Gemini Interactions completed without model text", ErrProviderAPI)
		}
		return generation, nil
	case statusIncomplete:
		return generation, errProviderOutputLimitReached
	case geminiInteractionStatusRequiresAction:
		return textGenerationResult{usage: snapshot.usage}, fmt.Errorf("%w: Gemini Interactions requires_action is unsupported", ErrProviderAPI)
	case statusFailed, statusCancelled, geminiInteractionStatusBudgetExceeded:
		return textGenerationResult{usage: snapshot.usage}, fmt.Errorf("%w: Gemini Interactions terminal status=%s", ErrProviderAPI, snapshot.status)
	default:
		return textGenerationResult{usage: snapshot.usage}, fmt.Errorf("%w: Gemini Interactions unknown status=%s", ErrProviderAPI, snapshot.status)
	}
}

func visibleGeminiInteractionText(steps []geminiInteractionStep) string {
	visibleText := constants.EmptyString
	for _, step := range steps {
		if step.Type != geminiInteractionStepModelOutput {
			continue
		}
		var textBuilder strings.Builder
		for _, content := range step.Content {
			if content.Type == geminiInteractionContentText && !utils.IsBlank(content.Text) {
				textBuilder.WriteString(content.Text)
			}
		}
		if textBuilder.Len() > 0 {
			visibleText = textBuilder.String()
		}
	}
	return visibleText
}

func parseGeminiInteractionTokenUsage(usage *geminiInteractionTokenUsage) (*tokenUsage, error) {
	if usage == nil {
		return nil, nil
	}
	return normalizeTokenUsage(usage.TotalInputTokens, usage.TotalOutputTokens, usage.TotalTokens)
}
