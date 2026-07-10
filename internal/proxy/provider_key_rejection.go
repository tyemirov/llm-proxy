package proxy

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/constants"
)

var forbiddenClientProviderCredentialParameters = map[string]struct{}{
	"api_key":             {},
	"provider_api_key":    {},
	"upstream_api_key":    {},
	"openai_api_key":      {},
	"deepseek_api_key":    {},
	"dashscope_api_key":   {},
	"qwen_api_key":        {},
	"moonshot_api_key":    {},
	"kimi_api_key":        {},
	"siliconflow_api_key": {},
	"zhipu_api_key":       {},
	"glm_api_key":         {},
	"gemini_api_key":      {},
	"anthropic_api_key":   {},
	"claude_api_key":      {},
	"model_api_key":       {},
	"meta_api_key":        {},
	"grok_api_key":        {},
	"xai_api_key":         {},
}

func rejectClientProviderCredentialsFromQuery(ginContext *gin.Context) bool {
	for parameterName := range ginContext.Request.URL.Query() {
		if forbiddenClientProviderCredentialParameter(parameterName) {
			ginContext.String(http.StatusBadRequest, errorClientProviderAPIKey)
			return true
		}
	}
	return false
}

func readJSONProxyBody(ginContext *gin.Context) ([]byte, bool) {
	bodyBytes, readError := io.ReadAll(ginContext.Request.Body)
	if readError != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(readError, &maxBytesError) {
			ginContext.String(http.StatusRequestEntityTooLarge, errorPromptPayloadTooLarge)
			return nil, false
		}
		ginContext.String(http.StatusBadRequest, errorInvalidJSONRequest)
		return nil, false
	}
	return bodyBytes, true
}

func rejectClientProviderCredentialsFromJSONBody(ginContext *gin.Context, bodyBytes []byte) bool {
	var bodyFields map[string]json.RawMessage
	if unmarshalError := json.Unmarshal(bodyBytes, &bodyFields); unmarshalError != nil {
		return false
	}
	for fieldName := range bodyFields {
		if forbiddenClientProviderCredentialParameter(fieldName) {
			ginContext.String(http.StatusBadRequest, errorClientProviderAPIKey)
			return true
		}
	}
	return false
}

func rejectClientProviderCredentialsFromForm(ginContext *gin.Context) bool {
	if ginContext.Request.MultipartForm == nil {
		return false
	}
	for fieldName := range ginContext.Request.MultipartForm.Value {
		if forbiddenClientProviderCredentialParameter(fieldName) {
			ginContext.String(http.StatusBadRequest, errorClientProviderAPIKey)
			return true
		}
	}
	return false
}

func forbiddenClientProviderCredentialParameter(rawName string) bool {
	normalizedName := strings.ToLower(strings.TrimSpace(rawName))
	if normalizedName == constants.EmptyString {
		return false
	}
	_, forbidden := forbiddenClientProviderCredentialParameters[normalizedName]
	return forbidden
}
