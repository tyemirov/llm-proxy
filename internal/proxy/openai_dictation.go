package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/constants"
	"github.com/tyemirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

type transcriptionResponse struct {
	Text string `json:"text"`
}

func (client *OpenAIClient) transcribeAudioWithURL(parentContext context.Context, openAIKey string, transcriptionsURL string, modelFieldName string, modelIdentifier string, fileName string, audioReader io.Reader, structuredLogger *zap.SugaredLogger) (string, error) {
	modelFieldName = strings.TrimSpace(modelFieldName)
	modelIdentifier = strings.TrimSpace(modelIdentifier)
	fileName = strings.TrimSpace(fileName)

	payloadBuffer := &bytes.Buffer{}
	multipartWriter := multipart.NewWriter(payloadBuffer)

	if modelFieldName != constants.EmptyString {
		_ = multipartWriter.WriteField(modelFieldName, modelIdentifier)
	}

	filePart, _ := multipartWriter.CreateFormFile(formFieldFile, fileName)
	if _, copyError := io.Copy(filePart, audioReader); copyError != nil {
		return constants.EmptyString, copyError
	}

	_ = multipartWriter.Close()

	requestBody := bytes.NewReader(payloadBuffer.Bytes())
	httpRequest, buildError := http.NewRequestWithContext(parentContext, http.MethodPost, transcriptionsURL, requestBody)
	if buildError != nil {
		return constants.EmptyString, buildError
	}
	httpRequest.Header.Set(headerAuthorization, headerAuthorizationPrefix+openAIKey)
	httpRequest.Header.Set(headerContentType, multipartWriter.FormDataContentType())
	httpRequest.Header.Set(headerAccept, mimeApplicationJSON)

	statusCode, responseBytes, responseHeader, _, requestError := client.performTranscriptionsRequest(httpRequest, structuredLogger)
	if responseError := providerResponseError(statusCode, responseHeader, requestError); responseError != nil {
		return constants.EmptyString, responseError
	}

	transcribedText, parseError := parseTranscriptionText(responseBytes)
	if parseError != nil {
		return constants.EmptyString, parseError
	}
	return transcribedText, nil
}

func parseTranscriptionText(rawPayload []byte) (string, error) {
	var response transcriptionResponse
	if unmarshalError := json.Unmarshal(rawPayload, &response); unmarshalError != nil {
		return constants.EmptyString, fmt.Errorf("decode transcription response: %w", unmarshalError)
	}
	text := strings.TrimSpace(response.Text)
	if text == constants.EmptyString {
		return constants.EmptyString, errors.New(errorOpenAIAPINoText)
	}
	return text, nil
}

func (client *OpenAIClient) performTranscriptionsRequest(httpRequest *http.Request, structuredLogger *zap.SugaredLogger) (int, []byte, http.Header, int64, error) {
	return utils.PerformHTTPRequest(client.httpClient.Do, httpRequest, structuredLogger, logEventOpenAIRequestError)
}
