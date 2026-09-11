package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sort"
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
	DiscoverMediaVoices(context.Context) ([]MediaVoiceProviderRecord, error)
}

// MediaVoiceProviderRecord is a validated private voice observation. Model and
// provider reference values are retained privately and never returned publicly.
type MediaVoiceProviderRecord struct {
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
	VoiceID           string `json:"voice_id"`
	Provider          string `json:"provider"`
	Mode              string `json:"mode"`
	Language          string `json:"language"`
	DisplayName       string `json:"display_name"`
	Default           bool   `json:"default"`
	SampleRates       []int  `json:"sample_rates"`
	DefaultSampleRate int    `json:"default_sample_rate"`
}

type mediaVoiceCollectionResponse struct {
	Voices []mediaVoiceResponse `json:"voices"`
}

func (service *mediaOperationService) mediaVoiceCollectionHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		provider := strings.TrimSpace(ginContext.Query("provider"))
		voiceProvider := service.voiceProviders[provider]
		if provider == "" || provider != strings.ToLower(provider) || voiceProvider == nil {
			writeMediaVoiceError(ginContext, errors.New(llmproxycontract.ErrorCodeMediaVoiceInvalid))
			return
		}
		discovered, discoveryError := voiceProvider.DiscoverMediaVoices(ginContext.Request.Context())
		if discoveryError != nil {
			writeMediaVoiceError(ginContext, errors.New(llmproxycontract.ErrorCodeMediaVoiceProvider))
			return
		}
		requestTenant := authenticatedTenantFromContext(ginContext)
		if persistError := service.store.persistMediaVoices(ginContext.Request.Context(), requestTenant.identifier.string(), provider, discovered); persistError != nil {
			writeMediaVoiceError(ginContext, persistError)
			return
		}
		voices, listError := service.store.listMediaVoices(ginContext.Request.Context(), requestTenant.identifier.string(), provider)
		if listError != nil {
			writeMediaVoiceError(ginContext, listError)
			return
		}
		ginContext.JSON(http.StatusOK, mediaVoiceCollectionResponse{Voices: voices})
	}
}

func (service *mediaOperationService) mediaVoiceHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestTenant := authenticatedTenantFromContext(ginContext)
		voice, voiceError := service.store.mediaVoice(ginContext.Request.Context(), requestTenant.identifier.string(), ginContext.Param("voice_id"))
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
		DoUpdates: clause.AssignmentColumns([]string{"model", "mode", "language", "display_name", "default", "sample_rates", "default_sample_rate", "updated_at"}),
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
	if tenantID == "" || provider == "" || voice.Provider != provider || voice.Model == "" || (voice.Mode != MediaVoiceModePreset && voice.Mode != MediaVoiceModeExtracted) || voice.Language == "" || voice.DisplayName == "" || voice.ProviderVoiceReference == "" || len(voice.SampleRates) == 0 || voice.DefaultSampleRate <= 0 {
		return mediaVoiceRecord{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceProvider)
	}
	defaultFound := false
	for index, sampleRate := range voice.SampleRates {
		if sampleRate <= 0 || (index > 0 && sampleRate == voice.SampleRates[index-1]) {
			return mediaVoiceRecord{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceProvider)
		}
		defaultFound = defaultFound || sampleRate == voice.DefaultSampleRate
	}
	if !defaultFound {
		return mediaVoiceRecord{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceProvider)
	}
	sampleRates, _ := json.Marshal(voice.SampleRates)
	return mediaVoiceRecord{
		VoiceID: newMediaVoiceIdentifier(), TenantID: tenantID, Provider: provider,
		ProviderVoiceReference: voice.ProviderVoiceReference, Model: voice.Model, Mode: voice.Mode,
		Language: voice.Language, DisplayName: voice.DisplayName, Default: voice.Default,
		SampleRates: sampleRates, DefaultSampleRate: voice.DefaultSampleRate, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (store *mediaOperationStore) listMediaVoices(requestContext context.Context, tenantID string, provider string) ([]mediaVoiceResponse, error) {
	var records []mediaVoiceRecord
	if queryError := store.database.WithContext(requestContext).Where("tenant_id = ? AND provider = ?", tenantID, provider).Order("mode, display_name, voice_id").Find(&records).Error; queryError != nil {
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
	if decodeError := json.Unmarshal(record.SampleRates, &sampleRates); decodeError != nil || len(sampleRates) == 0 {
		return mediaVoiceResponse{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceStore)
	}
	return mediaVoiceResponse{
		VoiceID: record.VoiceID, Provider: record.Provider, Mode: record.Mode, Language: record.Language,
		DisplayName: record.DisplayName, Default: record.Default, SampleRates: sampleRates, DefaultSampleRate: record.DefaultSampleRate,
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
