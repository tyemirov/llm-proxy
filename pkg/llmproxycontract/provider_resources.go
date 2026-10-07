package llmproxycontract

// ProviderResourceKind identifies an account resource independently of models.
type ProviderResourceKind string

const (
	ProviderResourceVoices       ProviderResourceKind = "voices"
	ProviderResourceVoiceLibrary ProviderResourceKind = "voice_library"
	ProviderResourceHistory      ProviderResourceKind = "history"
	ProviderResourceDictionaries ProviderResourceKind = "pronunciation_dictionaries"
	ProviderResourceMetadata     ProviderResourceKind = "metadata"
	ProviderResourceQuotas       ProviderResourceKind = "quotas"
	// ProviderResourceAccount identifies current billing and balance observations.
	ProviderResourceAccount  ProviderResourceKind = "account"
	ProviderResourceElements ProviderResourceKind = "elements"
)

// ValidProviderResourceKind validates the closed discovery vocabulary.
func ValidProviderResourceKind(kind ProviderResourceKind) bool {
	switch kind {
	case ProviderResourceVoices, ProviderResourceVoiceLibrary, ProviderResourceHistory,
		ProviderResourceDictionaries, ProviderResourceMetadata, ProviderResourceQuotas, ProviderResourceElements, ProviderResourceAccount:
		return true
	default:
		return false
	}
}

// ProviderResource identifies a resource available through one provider connection.
type ProviderResource struct {
	Provider string               `json:"provider"`
	Kind     ProviderResourceKind `json:"kind"`
}
