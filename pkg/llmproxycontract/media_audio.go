package llmproxycontract

// MediaAudioDescription describes the exact audio bytes in output ordinal zero.
type MediaAudioDescription struct {
	OutputFormat string            `json:"output_format"`
	MIMEType     string            `json:"mime_type"`
	RawAudio     *RawAudioEncoding `json:"raw_audio,omitempty"`
}

// RawAudioEncoding supplies the interpretation required by headerless audio.
type RawAudioEncoding struct {
	Encoding     string `json:"encoding"`
	SampleRateHz int    `json:"sample_rate_hz"`
	Channels     int    `json:"channels"`
}

// MediaSpeechTiming preserves provider character timestamps for caller text.
type MediaSpeechTiming struct {
	Alignment           *SpeechCharacterAlignment `json:"alignment"`
	NormalizedAlignment *SpeechCharacterAlignment `json:"normalized_alignment"`
}

// SpeechCharacterAlignment records character boundaries in seconds.
type SpeechCharacterAlignment struct {
	Characters []string  `json:"characters"`
	Starts     []float64 `json:"character_start_times_seconds"`
	Ends       []float64 `json:"character_end_times_seconds"`
}
