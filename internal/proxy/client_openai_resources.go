package proxy

import (
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type openAIModelRecord struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

func openAIModelsHandler(catalog *ProviderCatalog, providers *providerRegistry) gin.HandlerFunc {
	owners := map[string]string{}
	for _, model := range catalog.schema.Models {
		owners[model.ID] = model.Publisher
	}
	return func(c *gin.Context) {
		if c.Request.ContentLength != 0 || len(c.Request.TransferEncoding) > 0 {
			writeOpenAIError(c, 400, "unsupported_parameter", "A model discovery body is unsupported.")
			return
		}
		if c.Request.URL.RawQuery != "" {
			writeOpenAIError(c, 400, "unsupported_parameter", "Query parameters are unsupported.")
			return
		}
		registry := providers.forTenant(authenticatedTenantFromContext(c))
		records := []openAIModelRecord{}
		for _, provider := range catalog.schema.Providers {
			for _, offering := range provider.Offerings {
				supported := false
				for _, operation := range offering.Operations {
					switch operation {
					case ModelOperationText:
						_, _, err := registry.resolveTextRequest(provider.ID, offering.Model, "", "", false)
						supported = supported || err == nil
					case ModelOperationDictation:
						_, _, err := registry.resolveDictationRequest(provider.ID, offering.Model, "", "")
						supported = supported || err == nil
					}
				}
				if supported {
					records = append(records, openAIModelRecord{ID: provider.ID + "/" + offering.Model, Object: "model", Created: offering.Created, OwnedBy: owners[offering.Model]})
				}
			}
		}
		sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
		c.JSON(http.StatusOK, gin.H{"object": "list", "data": records})
	}
}
func openAITranscriptionHandler(maxBytes int64, providers *providerRegistry, coordinator *providerRouter, store *managedTenantStore, logger *zap.SugaredLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		tenant := authenticatedTenantFromContext(c)
		reject := func(status int, message string) {
			writeOpenAIError(c, status, "invalid_request", message)
			recordManagedUsageValidationFailure(store, logger, c, tenant, usageEndpointDictation, start)
		}
		if c.Request.URL.RawQuery != "" {
			reject(400, "Query parameters are unsupported.")
			return
		}
		if c.ContentType() != "multipart/form-data" {
			reject(415, "Use multipart/form-data.")
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes+dictationMultipartOverheadBytes)
		if err := c.Request.ParseMultipartForm(maxBytes); err != nil {
			var limit *http.MaxBytesError
			if errors.As(err, &limit) {
				reject(413, "Audio exceeds its limit.")
			} else {
				reject(400, "Invalid audio form.")
			}
			return
		}
		defer c.Request.MultipartForm.RemoveAll()
		form := c.Request.MultipartForm
		for key, values := range form.Value {
			if (key != "model" && key != "response_format") || len(values) != 1 {
				reject(400, "Unsupported form fields.")
				return
			}
		}
		if len(form.File) != 1 || len(form.File["file"]) != 1 || len(form.Value["model"]) != 1 {
			reject(400, "One file and model are required.")
			return
		}
		if format := c.Request.FormValue("response_format"); format != "" && format != "json" {
			reject(400, "Only JSON transcription is supported.")
			return
		}
		provider, model, err := exactClientModel(c.Request.FormValue("model"))
		if err != nil {
			reject(400, "Use provider/model.")
			return
		}
		resolved, resolvedModel, err := newModelValidator(providers.forTenant(tenant)).ResolveDictation(provider, model, "", "")
		if err != nil || resolved.identifier.string() != provider || resolvedModel.string() != model {
			reject(400, "Invalid or unavailable transcription route.")
			return
		}
		bindRequestTelemetryRoute(c, resolved, resolvedModel)
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			reject(400, "Invalid audio file.")
			return
		}
		defer file.Close()
		if header.Size == 0 || header.Size > maxBytes {
			reject(413, "Invalid audio size.")
			return
		}
		request := dictationRequestParameters{provider: resolved, model: resolvedModel, fileName: header.Filename, audioReader: contextReader{contextValue: c.Request.Context(), reader: file}}
		submitDictationRequest(c, coordinator, request, tenant, store, logger, start)
	}
}
