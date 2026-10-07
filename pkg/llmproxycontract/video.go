package llmproxycontract

const (
	// MediaCapabilityVideoLipSync identifies replacement audio on an existing video.
	MediaCapabilityVideoLipSync = "video.lipsync"
	// MediaCapabilityVideoTranslate identifies translation into ordered target languages.
	MediaCapabilityVideoTranslate = "video.translate"
	// MediaCapabilityAvatarCreate identifies creation of a reusable avatar resource.
	MediaCapabilityAvatarCreate = "avatar.create"
	// MediaCapabilityAvatarVideoGenerate identifies an audio-driven avatar video.
	MediaCapabilityAvatarVideoGenerate = "avatar.video.generate"
)

// VideoLipSyncSource identifies tenant-owned source video and replacement audio.
type VideoLipSyncSource struct {
	VideoAssetID string `json:"video_asset_id"`
	AudioAssetID string `json:"audio_asset_id"`
}

// VideoTranslationSource identifies tenant-owned media for translation.
type VideoTranslationSource struct {
	VideoAssetID string `json:"video_asset_id"`
	AudioAssetID string `json:"audio_asset_id,omitempty"`
}

// VideoProcessingControls records explicit processing settings for a video operation.
type VideoProcessingControls struct {
	Mode                    string   `json:"mode"`
	Title                   string   `json:"title,omitempty"`
	DisableMusicTrack       *bool    `json:"disable_music_track,omitempty"`
	EnableDynamicDuration   *bool    `json:"enable_dynamic_duration,omitempty"`
	EnableSpeechEnhancement *bool    `json:"enable_speech_enhancement,omitempty"`
	EnableWatermark         *bool    `json:"enable_watermark,omitempty"`
	KeepTheSameFormat       *bool    `json:"keep_the_same_format,omitempty"`
	FPSMode                 string   `json:"fps_mode,omitempty"`
	StartTime               *float64 `json:"start_time,omitempty"`
	EndTime                 *float64 `json:"end_time,omitempty"`
}

// VideoLipSyncControls selects speed or precision processing.
type VideoLipSyncControls struct {
	VideoProcessingControls
}

// VideoTranslationControls selects ordered languages and translation settings.
type VideoTranslationControls struct {
	VideoProcessingControls
	OutputLanguages    []string `json:"output_languages"`
	InputLanguage      string   `json:"input_language,omitempty"`
	SpeakerNum         *int     `json:"speaker_num,omitempty"`
	TranslateAudioOnly *bool    `json:"translate_audio_only,omitempty"`
}

// AvatarCreationSource identifies the image and name for a reusable photo avatar.
type AvatarCreationSource struct {
	ImageAssetID string `json:"image_asset_id"`
	Name         string `json:"name"`
}

// AvatarVideoSource identifies a gateway avatar and its tenant-owned audio.
type AvatarVideoSource struct {
	AvatarID     string `json:"avatar_id"`
	AudioAssetID string `json:"audio_asset_id"`
}

// AvatarVideoControls selects the rendering engine and output settings.
type AvatarVideoControls struct {
	Engine         string `json:"engine"`
	AspectRatio    string `json:"aspect_ratio"`
	Resolution     string `json:"resolution"`
	Title          string `json:"title,omitempty"`
	MotionPrompt   string `json:"motion_prompt,omitempty"`
	Expressiveness string `json:"expressiveness,omitempty"`
}

// MediaAvatar describes a retained gateway avatar without native provider identifiers.
type MediaAvatar struct {
	AvatarID string `json:"avatar_id"`
	Provider string `json:"provider"`
	Name     string `json:"name"`
}
