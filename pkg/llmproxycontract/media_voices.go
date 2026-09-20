package llmproxycontract

import (
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// MediaVoiceQuery selects one page from a provider voice collection.
// Cursor continuation accepts only Provider and Cursor.
type MediaVoiceQuery struct {
	Provider          string `json:"provider"`
	Cursor            string `json:"cursor,omitempty"`
	Search            string `json:"search,omitempty"`
	PageSize          int    `json:"page_size,omitempty"`
	Sort              string `json:"sort,omitempty"`
	SortDirection     string `json:"sort_direction,omitempty"`
	VoiceType         string `json:"voice_type,omitempty"`
	Category          string `json:"category,omitempty"`
	IncludeTotalCount *bool  `json:"include_total_count,omitempty"`
}

// MediaVoiceMetadata preserves account voice observations without native references.
type MediaVoiceMetadata struct {
	Description             *string              `json:"description"`
	Category                *string              `json:"category"`
	Labels                  map[string]string    `json:"labels"`
	HighQualityBaseModelIDs []string             `json:"high_quality_base_model_ids"`
	VerifiedLanguages       []MediaVoiceLanguage `json:"verified_languages"`
	Preview                 *string              `json:"preview"`
}

// MediaVoiceLanguage is one provider-verified language and its local preview link.
type MediaVoiceLanguage struct {
	Language string  `json:"language"`
	ModelID  string  `json:"model_id"`
	Accent   *string `json:"accent"`
	Locale   *string `json:"locale"`
	Preview  *string `json:"preview"`
}

// ParseMediaVoiceQuery validates one public voice collection query.
func ParseMediaVoiceQuery(values url.Values) (MediaVoiceQuery, error) {
	invalid := errors.New(ErrorCodeMediaVoiceInvalid)
	allowed := []string{"provider", "cursor", "search", "page_size", "sort", "sort_direction", "voice_type", "category", "include_total_count"}
	for key, items := range values {
		if !slices.Contains(allowed, key) || len(items) != 1 || strings.TrimSpace(items[0]) == "" {
			return MediaVoiceQuery{}, invalid
		}
	}
	query := MediaVoiceQuery{Provider: values.Get("provider"), Cursor: values.Get("cursor"), Search: values.Get("search"), Sort: values.Get("sort"), SortDirection: values.Get("sort_direction"), VoiceType: values.Get("voice_type"), Category: values.Get("category")}
	if query.Provider == "" || query.Provider != strings.ToLower(strings.TrimSpace(query.Provider)) || !utf8.ValidString(query.Search) || utf8.RuneCountInString(query.Search) > 1000 || len(query.Cursor) > 16384 {
		return query, invalid
	}
	if query.Cursor != "" {
		if len(values) != 2 {
			return query, invalid
		}
		return query, nil
	}
	query.PageSize = 10
	if raw := values.Get("page_size"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			return query, invalid
		}
		query.PageSize = value
	}
	if raw := values.Get("include_total_count"); raw != "" {
		if raw != "true" && raw != "false" {
			return query, invalid
		}
		value := raw == "true"
		query.IncludeTotalCount = &value
	}
	if !slices.Contains([]string{"", "name", "created_at_unix"}, query.Sort) || !slices.Contains([]string{"", "asc", "desc"}, query.SortDirection) || !slices.Contains([]string{"", "personal", "community", "default", "account", "non-default", "non-community", "saved"}, query.VoiceType) || !slices.Contains([]string{"", "premade", "cloned", "generated", "professional"}, query.Category) {
		return query, invalid
	}
	return query, nil
}

// Values validates and encodes the query for the public voice collection.
func (query MediaVoiceQuery) Values() (url.Values, error) {
	values := url.Values{"provider": {query.Provider}}
	for key, value := range map[string]string{"cursor": query.Cursor, "search": query.Search, "sort": query.Sort, "sort_direction": query.SortDirection, "voice_type": query.VoiceType, "category": query.Category} {
		if value != "" {
			values.Set(key, value)
		}
	}
	if query.PageSize != 0 {
		values.Set("page_size", strconv.Itoa(query.PageSize))
	}
	if query.IncludeTotalCount != nil {
		values.Set("include_total_count", strconv.FormatBool(*query.IncludeTotalCount))
	}
	_, err := ParseMediaVoiceQuery(values)
	return values, err
}
