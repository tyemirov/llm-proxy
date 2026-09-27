package proxy

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

const voiceCursorPurpose = "voice-page"
const voicePageLifetime = 15 * time.Minute

type mediaVoiceCursor struct {
	Query     llmproxycontract.MediaVoiceQuery `json:"query"`
	Token     string                           `json:"token"`
	Authority string                           `json:"authority"`
	Revision  string                           `json:"revision"`
	Expires   time.Time                        `json:"expires"`
}

func (service *mediaOperationService) mediaVoiceCollectionHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		query, err := llmproxycontract.ParseMediaVoiceQuery(c.Request.URL.Query())
		if err != nil || service.voiceProviders[query.Provider] == nil {
			writeMediaVoiceError(c, errors.New(llmproxycontract.ErrorCodeMediaVoiceInvalid))
			return
		}
		requestTenant := authenticatedTenantFromContext(c)
		tenant := requestTenant.identifier.string()
		readContext, err := service.voiceReadContext(c.Request.Context(), requestTenant, query.Provider)
		if err != nil {
			writeMediaVoiceError(c, err)
			return
		}
		nativeQuery := MediaVoiceQuery{MediaVoiceQuery: query}
		authority, authorityError := service.voiceProviders[query.Provider].MediaVoiceAuthority(readContext, tenant)
		if authorityError != nil {
			writeMediaVoiceError(c, errors.New(llmproxycontract.ErrorCodeMediaVoiceProvider))
			return
		}
		if query.Cursor != "" {
			raw, decodeErr := service.voiceCursorCipher.decryptConnectionValue(tenant, query.Provider, voiceCursorPurpose, query.Cursor)
			var cursor mediaVoiceCursor
			if decodeErr != nil || json.Unmarshal([]byte(raw), &cursor) != nil || cursor.Authority != voiceCursorAuthority(readContext, authority) || cursor.Revision != service.catalog.Revision() || !service.store.now().Before(cursor.Expires) {
				writeMediaVoiceError(c, errors.New(llmproxycontract.ErrorCodeMediaVoiceInvalid))
				return
			}
			nativeQuery = MediaVoiceQuery{MediaVoiceQuery: cursor.Query, PageToken: cursor.Token}
		}
		discovery, err := service.voiceProviders[query.Provider].DiscoverMediaVoices(readContext, tenant, nativeQuery)
		if err != nil {
			if !errors.Is(err, errHostedAuthorityDenied) && err.Error() != llmproxycontract.ErrorCodeMediaVoiceInvalid {
				err = errors.New(llmproxycontract.ErrorCodeMediaVoiceProvider)
			}
			writeMediaVoiceError(c, err)
			return
		}
		if err := authorizeVoiceRead(readContext); err != nil {
			writeMediaVoiceError(c, err)
			return
		}
		currentAuthority, currentError := service.voiceProviders[query.Provider].MediaVoiceAuthority(readContext, tenant)
		if currentError != nil || currentAuthority != authority || discovery.Authority != authority {
			writeMediaVoiceError(c, errors.New(llmproxycontract.ErrorCodeMediaVoiceProvider))
			return
		}
		if err := service.store.persistMediaVoices(c.Request.Context(), tenant, query.Provider, discovery.Voices); err != nil {
			writeMediaVoiceError(c, err)
			return
		}
		var voices []mediaVoiceResponse
		if discovery.NativePage {
			voices = make([]mediaVoiceResponse, 0, len(discovery.Voices))
			for _, observed := range discovery.Voices {
				var record mediaVoiceRecord
				if err := service.store.database.WithContext(c.Request.Context()).Where("tenant_id = ? AND provider = ? AND provider_voice_reference = ?", tenant, query.Provider, observed.ProviderVoiceReference).First(&record).Error; err != nil {
					writeMediaVoiceError(c, errors.New(llmproxycontract.ErrorCodeMediaVoiceStore))
					return
				}
				voice, err := publicMediaVoice(record)
				if err != nil {
					writeMediaVoiceError(c, err)
					return
				}
				voices = append(voices, voice)
			}
		} else {
			voices, err = service.store.listMediaVoices(c.Request.Context(), tenant, query.Provider, discovery.Authority)
			if err != nil {
				writeMediaVoiceError(c, err)
				return
			}
			voices, discovery.NextPageToken, discovery.TotalCount, err = pageMediaVoiceSnapshot(voices, nativeQuery)
			if err != nil {
				writeMediaVoiceError(c, err)
				return
			}
		}
		result := mediaVoiceCollectionResponse{Voices: voices, HasMore: discovery.NextPageToken != "", TotalCount: discovery.TotalCount}
		if result.HasMore {

			raw, _ := json.Marshal(mediaVoiceCursor{Query: nativeQuery.MediaVoiceQuery, Token: discovery.NextPageToken, Authority: voiceCursorAuthority(readContext, authority), Revision: service.catalog.Revision(), Expires: service.store.now().Add(voicePageLifetime)})
			cursor, err := service.voiceCursorCipher.encryptConnection(service.voiceCursorRandom, tenant, query.Provider, voiceCursorPurpose, string(raw))
			if err != nil {
				writeMediaVoiceError(c, errors.New(llmproxycontract.ErrorCodeMediaVoiceStore))
				return
			}
			result.NextCursor = &cursor
		}
		c.JSON(http.StatusOK, result)
	}
}

func pageMediaVoiceSnapshot(voices []mediaVoiceResponse, query MediaVoiceQuery) ([]mediaVoiceResponse, string, *int, error) {
	if query.VoiceType != "" || query.Category != "" || query.Sort == "created_at_unix" {
		return nil, "", nil, errors.New(llmproxycontract.ErrorCodeMediaVoiceInvalid)
	}
	filtered := make([]mediaVoiceResponse, 0, len(voices))
	for _, voice := range voices {
		if strings.Contains(strings.ToLower(voice.DisplayName), strings.ToLower(query.Search)) {
			filtered = append(filtered, voice)
		}
	}
	if query.Sort == "name" {
		sort.SliceStable(filtered, func(i, j int) bool {
			if filtered[i].DisplayName == filtered[j].DisplayName {
				return filtered[i].VoiceID < filtered[j].VoiceID
			}
			return filtered[i].DisplayName < filtered[j].DisplayName
		})
	}
	if query.SortDirection == "desc" {
		slices.Reverse(filtered)
	}
	var total *int
	if query.IncludeTotalCount != nil && *query.IncludeTotalCount {
		count := len(filtered)
		total = &count
	}
	offset := 0
	if query.PageToken != "" {
		value, err := strconv.Atoi(query.PageToken)
		if err != nil || value < 0 || value > len(filtered) {
			return nil, "", nil, errors.New(llmproxycontract.ErrorCodeMediaVoiceInvalid)
		}
		offset = value
	}
	end := min(offset+query.PageSize, len(filtered))
	next := ""
	if end < len(filtered) {
		next = strconv.Itoa(end)
	}
	return filtered[offset:end], next, total, nil
}
