package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	MediaVoiceModePreset    = "preset"
	MediaVoiceModeExtracted = "extracted"
)

var mediaVoiceIdentifierPattern = regexp.MustCompile(`^voi_[0-9a-f]{32}$`)

// MediaVoiceProvider discovers native voices through one private provider
// connection. The shared service assigns tenant-owned gateway identifiers.
type MediaVoiceProvider interface {
	MediaVoiceAuthority(context.Context, string) (string, error)
	DiscoverMediaVoices(context.Context, string, MediaVoiceQuery) (MediaVoiceDiscovery, error)
}

// MediaVoiceDiscovery contains voices and the current private connection authority.
// MediaVoiceQuery is the validated source query. PageToken is private.
type MediaVoiceQuery struct {
	llmproxycontract.MediaVoiceQuery
	PageToken string
}

type MediaVoiceDiscovery struct {
	NativePage    bool
	NextPageToken string
	TotalCount    *int
	Authority     string
	Voices        []MediaVoiceProviderRecord
}

// MediaVoiceProviderRecord is a private voice observation. Model records
// extraction provenance. Account-discovered presets do not require a model.
// Model and provider reference values are never returned publicly.
type MediaVoiceProviderRecord struct {
	Metadata               llmproxycontract.MediaVoiceMetadata
	PreviewURLs            []string
	Authority              string
	Provider               string
	Model                  string
	Mode                   string
	Language               string
	DisplayName            string
	Default                bool
	SampleRates            []int
	DefaultSampleRate      int
	ProviderVoiceReference string
}

type mediaVoiceRecord struct {
	Metadata               []byte `gorm:"not null;default:'{\"description\":null,\"category\":null,\"labels\":{},\"high_quality_base_model_ids\":[],\"verified_languages\":[],\"preview\":null}'"`
	PreviewURLs            []byte `gorm:"not null;default:'[]'"`
	Authority              string
	VoiceID                string `gorm:"primaryKey"`
	TenantID               string `gorm:"not null;uniqueIndex:idx_media_voice_native,priority:1;index:idx_media_voice_tenant,priority:1"`
	Provider               string `gorm:"not null;uniqueIndex:idx_media_voice_native,priority:2;index:idx_media_voice_tenant,priority:2"`
	ProviderVoiceReference string `gorm:"not null;uniqueIndex:idx_media_voice_native,priority:3"`
	Model                  string `gorm:"not null"`
	Mode                   string `gorm:"not null"`
	Language               string `gorm:"not null"`
	DisplayName            string `gorm:"not null"`
	Default                bool   `gorm:"not null"`
	SampleRates            []byte `gorm:"not null"`
	DefaultSampleRate      int    `gorm:"not null"`
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type mediaVoiceResponse struct {
	llmproxycontract.MediaVoiceMetadata
	VoiceID           string  `json:"voice_id"`
	Provider          string  `json:"provider"`
	Mode              string  `json:"mode"`
	Language          *string `json:"language"`
	DisplayName       string  `json:"display_name"`
	Default           bool    `json:"default"`
	SampleRates       []int   `json:"sample_rates"`
	DefaultSampleRate *int    `json:"default_sample_rate"`
}

type mediaVoiceCollectionResponse struct {
	Voices     []mediaVoiceResponse `json:"voices"`
	HasMore    bool                 `json:"has_more"`
	TotalCount *int                 `json:"total_count"`
	NextCursor *string              `json:"next_cursor"`
}

func (service *mediaOperationService) mediaVoiceHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		ginContext.Header("Cache-Control", "no-store")
		requestTenant := authenticatedTenantFromContext(ginContext)
		record, voiceError := service.currentMediaVoice(ginContext.Request.Context(), requestTenant.identifier.string(), ginContext.Param("voice_id"))
		var voice mediaVoiceResponse
		if voiceError == nil {
			voice, voiceError = publicMediaVoice(record)
		}
		if voiceError != nil {
			writeMediaVoiceError(ginContext, voiceError)
			return
		}
		ginContext.JSON(http.StatusOK, voice)
	}
}

func (store *mediaOperationStore) persistMediaVoices(requestContext context.Context, tenantID string, provider string, discovered []MediaVoiceProviderRecord) error {
	now := store.now()
	return store.database.WithContext(requestContext).Transaction(func(transaction *gorm.DB) error {
		for _, voice := range discovered {
			if _, createError := persistMediaVoice(transaction, tenantID, provider, voice, now); createError != nil {
				if createError.Error() == llmproxycontract.ErrorCodeMediaVoiceProvider {
					return createError
				}
				return errors.New(llmproxycontract.ErrorCodeMediaVoiceStore)
			}
		}
		return nil
	})
}

func persistMediaVoice(transaction *gorm.DB, tenantID string, provider string, voice MediaVoiceProviderRecord, now time.Time) (mediaVoiceResponse, error) {
	record, recordError := newMediaVoiceRecord(tenantID, provider, voice, now)
	if recordError != nil {
		return mediaVoiceResponse{}, recordError
	}
	conflict := clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "provider"}, {Name: "provider_voice_reference"}},
		DoUpdates: clause.AssignmentColumns([]string{"authority", "model", "mode", "language", "display_name", "default", "sample_rates", "default_sample_rate", "metadata", "preview_urls", "updated_at"}),
	}
	if createError := transaction.Clauses(conflict).Create(&record).Error; createError != nil {
		return mediaVoiceResponse{}, createError
	}
	var persisted mediaVoiceRecord
	if queryError := transaction.Where("tenant_id = ? AND provider = ? AND provider_voice_reference = ?", tenantID, provider, record.ProviderVoiceReference).First(&persisted).Error; queryError != nil {
		return mediaVoiceResponse{}, queryError
	}
	return publicMediaVoice(persisted)
}

func (store *mediaOperationStore) providerMediaVoice(requestContext context.Context, tenantID string, voiceID string) (mediaVoiceRecord, error) {
	if !mediaVoiceIdentifierPattern.MatchString(voiceID) {
		return mediaVoiceRecord{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceNotFound)
	}
	var record mediaVoiceRecord
	queryError := store.database.WithContext(requestContext).Where("tenant_id = ? AND voice_id = ?", tenantID, voiceID).First(&record).Error
	if errors.Is(queryError, gorm.ErrRecordNotFound) {
		return mediaVoiceRecord{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceNotFound)
	}
	if queryError != nil {
		return mediaVoiceRecord{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceStore)
	}
	return record, nil
}

func newMediaVoiceRecord(tenantID string, provider string, voice MediaVoiceProviderRecord, now time.Time) (mediaVoiceRecord, error) {
	voice.Provider = strings.TrimSpace(voice.Provider)
	voice.Model = strings.TrimSpace(voice.Model)
	voice.Mode = strings.TrimSpace(voice.Mode)
	voice.Language = strings.TrimSpace(voice.Language)
	voice.DisplayName = strings.TrimSpace(voice.DisplayName)
	voice.ProviderVoiceReference = strings.TrimSpace(voice.ProviderVoiceReference)
	sort.Ints(voice.SampleRates)
	if tenantID == "" || provider == "" || voice.Provider != provider || (voice.Mode == MediaVoiceModeExtracted && voice.Model == "") || (voice.Mode != MediaVoiceModePreset && voice.Mode != MediaVoiceModeExtracted) || (voice.Mode == MediaVoiceModeExtracted && voice.Language == "") || voice.DisplayName == "" || voice.ProviderVoiceReference == "" || (len(voice.SampleRates) == 0 && voice.DefaultSampleRate != 0) {
		return mediaVoiceRecord{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceProvider)
	}
	defaultFound := len(voice.SampleRates) == 0
	for index, sampleRate := range voice.SampleRates {
		if sampleRate <= 0 || (index > 0 && sampleRate == voice.SampleRates[index-1]) {
			return mediaVoiceRecord{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceProvider)
		}
		defaultFound = defaultFound || sampleRate == voice.DefaultSampleRate
	}
	if !defaultFound {
		return mediaVoiceRecord{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceProvider)
	}
	if voice.SampleRates == nil {
		voice.SampleRates = []int{}
	}
	if voice.Metadata.Labels == nil {
		voice.Metadata.Labels = map[string]string{}
	}
	if voice.Metadata.HighQualityBaseModelIDs == nil {
		voice.Metadata.HighQualityBaseModelIDs = []string{}
	}
	if voice.Metadata.VerifiedLanguages == nil {
		voice.Metadata.VerifiedLanguages = []llmproxycontract.MediaVoiceLanguage{}
	}
	if voice.PreviewURLs == nil {
		voice.PreviewURLs = []string{}
	}
	metadata, _ := json.Marshal(voice.Metadata)
	previews, _ := json.Marshal(voice.PreviewURLs)
	sampleRates, _ := json.Marshal(voice.SampleRates)
	return mediaVoiceRecord{
		Metadata: metadata, PreviewURLs: previews,
		VoiceID: newMediaVoiceIdentifier(), TenantID: tenantID, Provider: provider,
		Authority: voice.Authority, ProviderVoiceReference: voice.ProviderVoiceReference, Model: voice.Model, Mode: voice.Mode,
		Language: voice.Language, DisplayName: voice.DisplayName, Default: voice.Default,
		SampleRates: sampleRates, DefaultSampleRate: voice.DefaultSampleRate, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (store *mediaOperationStore) listMediaVoices(requestContext context.Context, tenantID string, provider string, authority string) ([]mediaVoiceResponse, error) {
	var records []mediaVoiceRecord
	if queryError := store.database.WithContext(requestContext).Where("tenant_id = ? AND provider = ? AND authority = ?", tenantID, provider, authority).Order("mode, display_name, voice_id").Find(&records).Error; queryError != nil {
		return nil, errors.New(llmproxycontract.ErrorCodeMediaVoiceStore)
	}
	voices := make([]mediaVoiceResponse, 0, len(records))
	for _, record := range records {
		voice, responseError := publicMediaVoice(record)
		if responseError != nil {
			return nil, responseError
		}
		voices = append(voices, voice)
	}
	return voices, nil
}

func (store *mediaOperationStore) mediaVoice(requestContext context.Context, tenantID string, voiceID string) (mediaVoiceResponse, error) {
	if !mediaVoiceIdentifierPattern.MatchString(voiceID) {
		return mediaVoiceResponse{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceNotFound)
	}
	var record mediaVoiceRecord
	queryError := store.database.WithContext(requestContext).Where("tenant_id = ? AND voice_id = ?", tenantID, voiceID).First(&record).Error
	if errors.Is(queryError, gorm.ErrRecordNotFound) {
		return mediaVoiceResponse{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceNotFound)
	}
	if queryError != nil {
		return mediaVoiceResponse{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceStore)
	}
	return publicMediaVoice(record)
}

func publicMediaVoice(record mediaVoiceRecord) (mediaVoiceResponse, error) {
	var sampleRates []int
	if decodeError := json.Unmarshal(record.SampleRates, &sampleRates); decodeError != nil || sampleRates == nil {
		return mediaVoiceResponse{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceStore)
	}
	var metadata llmproxycontract.MediaVoiceMetadata
	if err := json.Unmarshal(record.Metadata, &metadata); err != nil || metadata.Labels == nil || metadata.HighQualityBaseModelIDs == nil || metadata.VerifiedLanguages == nil || metadata.Preview != nil {
		return mediaVoiceResponse{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceStore)
	}
	for _, language := range metadata.VerifiedLanguages {
		if language.Preview != nil {
			return mediaVoiceResponse{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceStore)
		}
	}
	var previews []string
	if json.Unmarshal(record.PreviewURLs, &previews) != nil || previews == nil || (len(previews) > 0 && len(previews) != len(metadata.VerifiedLanguages)+1) {
		return mediaVoiceResponse{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceStore)
	}
	for index, preview := range previews {
		if preview == "" {
			continue
		}
		link := llmproxycontract.MediaVoicesPath + "/" + record.VoiceID + "/previews/" + strconv.Itoa(index)
		if index == 0 {
			metadata.Preview = &link
		} else {
			metadata.VerifiedLanguages[index-1].Preview = &link
		}
	}
	var language *string
	if record.Language != "" {
		language = &record.Language
	}
	var rate *int
	if record.DefaultSampleRate != 0 {
		rate = &record.DefaultSampleRate
	}
	return mediaVoiceResponse{
		MediaVoiceMetadata: metadata,
		VoiceID:            record.VoiceID, Provider: record.Provider, Mode: record.Mode, Language: language,
		DisplayName: record.DisplayName, Default: record.Default, SampleRates: sampleRates, DefaultSampleRate: rate,
	}, nil
}

func writeMediaVoiceError(ginContext *gin.Context, voiceError error) {
	code := voiceError.Error()
	status := http.StatusInternalServerError
	switch code {
	case llmproxycontract.ErrorCodeMediaVoiceInvalid:
		status = http.StatusBadRequest
	case llmproxycontract.ErrorCodeMediaVoiceNotFound:
		status = http.StatusNotFound
	case llmproxycontract.ErrorCodeMediaVoiceProvider:
		status = http.StatusBadGateway
	case llmproxycontract.ErrorCodeMediaVoiceStore:
		status = http.StatusInternalServerError
	default:
		code = llmproxycontract.ErrorCodeMediaVoiceStore
	}
	ginContext.JSON(status, mediaOperationErrorEnvelope{Error: mediaOperationErrorResponse{Code: code}})
}

func newMediaVoiceIdentifier() string {
	return "voi_" + strings.TrimPrefix(newMediaOperationIdentifier(), "mop_")
}
