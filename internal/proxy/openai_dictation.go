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

	"github.com/temirov/llm-proxy/internal/constants"
	"github.com/temirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

type transcriptionResponse struct {
	Text       string `json:"text"`
	Transcript string `json:"transcript"`
	OutputText string `json:"output_text"`
}

func (client *OpenAIClient) transcribeAudio(openAIKey string, modelIdentifier string, fileName string, audioReader io.Reader, structuredLogger *zap.SugaredLogger) (string, error) {
	modelIdentifier = strings.TrimSpace(modelIdentifier)
	if modelIdentifier == constants.EmptyString {
		modelIdentifier = DefaultDictationModel
	}
	fileName = strings.TrimSpace(fileName)
	if fileName == constants.EmptyString {
		fileName = "audio.webm"
	}

	payloadBuffer := &bytes.Buffer{}
	multipartWriter := multipart.NewWriter(payloadBuffer)

	if writeError := multipartWriter.WriteField(keyModel, modelIdentifier); writeError != nil {
		return constants.EmptyString, writeError
	}

	filePart, createFileError := multipartWriter.CreateFormFile(formFieldFile, fileName)
	if createFileError != nil {
		return constants.EmptyString, createFileError
	}
	if _, copyError := io.Copy(filePart, audioReader); copyError != nil {
		return constants.EmptyString, copyError
	}

	if closeWriterError := multipartWriter.Close(); closeWriterError != nil {
		return constants.EmptyString, closeWriterError
	}

	requestContext, cancelRequest := context.WithTimeout(context.Background(), client.requestTimeout)
	defer cancelRequest()

	requestBody := bytes.NewReader(payloadBuffer.Bytes())
	httpRequest, buildError := http.NewRequestWithContext(requestContext, http.MethodPost, client.endpoints.GetTranscriptionsURL(), requestBody)
	if buildError != nil {
		return constants.EmptyString, buildError
	}
	httpRequest.Header.Set(headerAuthorization, headerAuthorizationPrefix+openAIKey)
	httpRequest.Header.Set(headerContentType, multipartWriter.FormDataContentType())
	httpRequest.Header.Set(headerAccept, mimeApplicationJSON)

	statusCode, responseBytes, _, requestError := client.performTranscriptionsRequest(httpRequest, structuredLogger)
	if requestError != nil {
		if errors.Is(requestError, context.DeadlineExceeded) {
			return constants.EmptyString, requestError
		}
		return constants.EmptyString, errors.New(errorOpenAIRequest)
	}

	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return constants.EmptyString, fmt.Errorf("%s: status=%d", errorOpenAIAPI, statusCode)
	}

	transcribedText, parseError := parseTranscriptionText(responseBytes)
	if parseError != nil {
		return constants.EmptyString, parseError
	}
	return transcribedText, nil
}

func parseTranscriptionText(rawPayload []byte) (string, error) {
	trimmedPayload := strings.TrimSpace(string(rawPayload))
	if trimmedPayload == constants.EmptyString {
		return constants.EmptyString, errors.New(errorOpenAIAPINoText)
	}

	if strings.HasPrefix(trimmedPayload, "{") {
		var response transcriptionResponse
		if unmarshalError := json.Unmarshal(rawPayload, &response); unmarshalError != nil {
			return constants.EmptyString, unmarshalError
		}

		for _, candidate := range []string{response.Text, response.Transcript, response.OutputText} {
			trimmedCandidate := strings.TrimSpace(candidate)
			if trimmedCandidate != constants.EmptyString {
				return trimmedCandidate, nil
			}
		}
		return constants.EmptyString, errors.New(errorOpenAIAPINoText)
	}

	return trimmedPayload, nil
}

func (client *OpenAIClient) performTranscriptionsRequest(httpRequest *http.Request, structuredLogger *zap.SugaredLogger) (int, []byte, int64, error) {
	return utils.PerformHTTPRequest(client.httpClient.Do, httpRequest, structuredLogger, logEventOpenAIRequestError)
}
