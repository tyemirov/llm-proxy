package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

const mediaVoicePreviewMaximumBytes = 8 << 20

func (service *mediaOperationService) currentMediaVoice(ctx context.Context, tenant, id string) (mediaVoiceRecord, error) {
	record, err := service.store.providerMediaVoice(ctx, tenant, id)
	if err != nil {
		return mediaVoiceRecord{}, err
	}
	provider := service.voiceProviders[record.Provider]
	if provider == nil {
		return mediaVoiceRecord{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceNotFound)
	}
	authority, err := provider.MediaVoiceAuthority(ctx, tenant)
	if err != nil || authority != record.Authority {
		return mediaVoiceRecord{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceNotFound)
	}
	return record, nil
}

func (service *mediaOperationService) mediaVoicePreviewHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		tenant := authenticatedTenantFromContext(c).identifier.string()
		record, err := service.currentMediaVoice(c.Request.Context(), tenant, c.Param("voice_id"))
		if err != nil {
			writeMediaVoiceError(c, err)
			return
		}
		index, err := strconv.Atoi(c.Param("preview"))
		var previews []string
		if json.Unmarshal(record.PreviewURLs, &previews) != nil {
			writeMediaVoiceError(c, errors.New(llmproxycontract.ErrorCodeMediaVoiceStore))
			return
		}
		if err != nil || index < 0 || index >= len(previews) || previews[index] == "" || c.Request.URL.RawQuery != "" {
			writeMediaVoiceError(c, errors.New(llmproxycontract.ErrorCodeMediaVoiceNotFound))
			return
		}
		definition := service.providers.definitions[providerID(record.Provider)]
		var origins []string
		for _, resource := range definition.resources {
			if resource.Kind == llmproxycontract.ProviderResourceVoices {
				origins = definition.transports[resource.Transport].artifactOrigins
			}
		}
		if !validMediaVoicePreviewURL(previews[index], origins) {
			writeMediaVoiceError(c, errors.New(llmproxycontract.ErrorCodeMediaVoiceProvider))
			return
		}
		reference, err := service.store.credentialReference(c.Request.Context(), tenant, providerID(record.Provider))
		if err != nil {
			writeMediaVoiceError(c, errors.New(llmproxycontract.ErrorCodeMediaVoiceNotFound))
			return
		}
		account, _, _ := strings.Cut(reference, ":v")
		client := scopedUpstreamHTTPDoer{next: service.httpClient, scope: upstreamRequestScope{tenant: tenant, account: account, class: upstreamTransfer}}
		ctx, cancel := context.WithTimeout(c.Request.Context(), providerMetadataTimeout)
		defer cancel()
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, previews[index], nil)
		response, err := client.Do(request)
		if err != nil {
			writeMediaVoiceError(c, errors.New(llmproxycontract.ErrorCodeMediaVoiceProvider))
			return
		}
		defer response.Body.Close()
		mediaType, _, mimeErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
		body, err := io.ReadAll(io.LimitReader(response.Body, mediaVoicePreviewMaximumBytes+1))
		if err != nil || response.StatusCode != http.StatusOK || mimeErr != nil || !strings.HasPrefix(mediaType, "audio/") || len(body) == 0 || len(body) > mediaVoicePreviewMaximumBytes {
			writeMediaVoiceError(c, errors.New(llmproxycontract.ErrorCodeMediaVoiceProvider))
			return
		}
		c.Header("X-Content-Type-Options", "nosniff")
		c.Data(http.StatusOK, mediaType, body)
	}
}
