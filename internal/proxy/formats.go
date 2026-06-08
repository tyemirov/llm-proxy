package proxy

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/constants"
)

// preferredMime determines the response MIME type using the format query parameter or the Accept header.
func preferredMime(ginContext *gin.Context) string {
	if explicitFormat := ginContext.Query(queryParameterFormat); explicitFormat != constants.EmptyString {
		return strings.ToLower(strings.TrimSpace(explicitFormat))
	}
	return strings.ToLower(strings.TrimSpace(ginContext.GetHeader(headerAccept)))
}

// formatResponse renders a textual model output into the requested MIME type and returns the body and content type.
func formatResponse(modelText string, preferred string, request chatRequestParameters, usage *tokenUsage) (string, string) {
	switch {
	case strings.Contains(preferred, mimeApplicationJSON):
		envelope := map[string]any{
			responseRequestAttribute: request.requestDisplay,
			jsonFieldResponse:        modelText,
			jsonFieldObject:          chatCompletionObject,
			keyModel:                 request.model.string(),
			jsonFieldChoices: []map[string]any{{
				jsonFieldIndex:        0,
				jsonFieldFinishReason: finishReasonStop,
				jsonFieldMessage: map[string]string{
					jsonFieldRole:    string(chatRoleAssistant),
					jsonFieldContent: modelText,
				},
			}},
			jsonFieldMessages: request.messages.responseRequestMessages(),
		}
		if usage != nil {
			envelope[jsonFieldUsage] = usage
		}
		encodedJSON, _ := json.Marshal(envelope)
		return string(encodedJSON), mimeApplicationJSON
	case strings.Contains(preferred, mimeApplicationXML) || strings.Contains(preferred, mimeTextXML):
		type xmlEnvelope struct {
			XMLName xml.Name `xml:"response"`
			Request string   `xml:"request,attr"`
			Text    string   `xml:",chardata"`
		}
		encodedXML, _ := xml.Marshal(xmlEnvelope{Request: request.requestDisplay, Text: modelText})
		return string(encodedXML), mimeApplicationXML
	case strings.Contains(preferred, mimeTextCSV):
		escaped := strings.ReplaceAll(modelText, `"`, `""`)
		return fmt.Sprintf(`"%s"`+"\n", escaped), mimeTextCSV
	default:
		return modelText, mimeTextPlain
	}
}
