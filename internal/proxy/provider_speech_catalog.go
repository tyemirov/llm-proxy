package proxy

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

var providerSpeechFormats = speechFormatDefinitions()

func speechFormatDefinitions() map[string]llmproxycontract.MediaAudioDescription {
	formats := map[string]llmproxycontract.MediaAudioDescription{}
	for _, format := range []string{"mp3_22050_32", "mp3_24000_48", "mp3_44100_32", "mp3_44100_64", "mp3_44100_96", "mp3_44100_128", "mp3_44100_192"} {
		formats[format] = llmproxycontract.MediaAudioDescription{MIMEType: "audio/mpeg"}
	}
	for _, bitRate := range []int{32, 64, 96, 128, 192} {
		formats["opus_48000_"+strconv.Itoa(bitRate)] = llmproxycontract.MediaAudioDescription{MIMEType: "audio/ogg"}
	}
	for _, sampleRate := range []int{8000, 16000, 22050, 24000, 32000, 44100, 48000} {
		formats["wav_"+strconv.Itoa(sampleRate)] = llmproxycontract.MediaAudioDescription{MIMEType: "audio/wav"}
		formats["pcm_"+strconv.Itoa(sampleRate)] = llmproxycontract.MediaAudioDescription{MIMEType: "application/octet-stream", RawAudio: &llmproxycontract.RawAudioEncoding{Encoding: "s16le", SampleRateHz: sampleRate, Channels: 1}}
	}
	for format, encoding := range map[string]string{"alaw_8000": "alaw", "ulaw_8000": "mulaw"} {
		formats[format] = llmproxycontract.MediaAudioDescription{MIMEType: "application/octet-stream", RawAudio: &llmproxycontract.RawAudioEncoding{Encoding: encoding, SampleRateHz: 8000, Channels: 1}}
	}
	return formats
}

func validateSpeechConversionOffering(offering ProviderOffering, field string) error {
	if offering.WireContract != CatalogProtocolElevenLabsConversion || offering.ExecutionLifecycle != string(textExecutionLifecycleSynchronousCompletion) || !slices.Equal(offering.Operations, []string{ModelOperationSpeechConversion}) || len(offering.Controls) != 8 || len(offering.Limits) != 0 {
		return fmt.Errorf("%w: field=%s reason=invalid_conversion_route", ErrInvalidModelCatalog, field)
	}
	if offering.RequestProfile != "" || offering.WebSearch || offering.CallerTools || offering.OutputTokenLimit != 0 || offering.ReasoningEffort != nil || len(offering.MediaInputs) != 0 || len(offering.ImageMIMETypes) != 0 || len(offering.MediaLimits) != 0 {
		return fmt.Errorf("%w: field=%s reason=text_capabilities_on_conversion_route", ErrInvalidModelCatalog, field)
	}
	for _, control := range offering.Controls {
		valid := !control.AccountDependent
		switch control.ID {
		case speechControlOutputFormat:
			valid = control.Kind == CatalogControlEnum
			for _, value := range control.Values {
				_, known := providerSpeechFormats[value]
				valid = valid && known
			}
		case speechControlInputFormat:
			valid = valid && imageEnumSubset(control, []string{"other", speechRawInputFormat})
		case speechControlStability, speechControlSimilarity, speechControlStyle:
			valid = valid && control.Kind == CatalogControlNumber && *control.Minimum >= 0 && *control.Maximum <= 1
		case speechControlSeed:
			valid = valid && control.Kind == CatalogControlInteger && *control.Minimum == 0 && *control.Maximum <= 4294967295
		case speechControlSpeakerBoost, speechControlNoise:
			valid = valid && control.Kind == CatalogControlBoolean
		default:
			valid = false
		}
		if !valid {
			return fmt.Errorf("%w: field=%s.controls control=%s reason=invalid_conversion_control", ErrInvalidModelCatalog, field, control.ID)
		}
	}
	return nil
}

func validateSpeechGenerationOffering(offering ProviderOffering, field string) error {
	if offering.ExecutionLifecycle != string(textExecutionLifecycleSynchronousCompletion) || !slices.Equal(offering.Operations, []string{ModelOperationSpeechGeneration}) || (len(offering.Controls) < 8 || len(offering.Controls) > 11) || len(offering.Limits) != 3 {
		return fmt.Errorf("%w: field=%s reason=invalid_speech_route", ErrInvalidModelCatalog, field)
	}
	if offering.RequestProfile != "" || offering.WebSearch || offering.CallerTools || offering.OutputTokenLimit != 0 || offering.ReasoningEffort != nil || len(offering.MediaInputs) != 0 || len(offering.ImageMIMETypes) != 0 || len(offering.MediaLimits) != 0 {
		return fmt.Errorf("%w: field=%s reason=text_capabilities_on_speech_route", ErrInvalidModelCatalog, field)
	}
	for _, control := range offering.Controls {
		valid := !control.AccountDependent
		switch control.ID {
		case speechControlOutputFormat:
			valid = control.Kind == CatalogControlEnum
			for _, value := range control.Values {
				_, known := providerSpeechFormats[value]
				valid = valid && known
			}
		case speechControlStability, speechControlSimilarity, speechControlStyle:
			valid = valid && control.Kind == CatalogControlNumber && *control.Minimum >= 0 && *control.Maximum <= 1
		case speechControlSpeed:
			valid = valid && control.Kind == CatalogControlNumber && *control.Minimum >= 0.7 && *control.Maximum <= 1.2
		case speechControlSeed:
			valid = valid && control.Kind == CatalogControlInteger && *control.Minimum == 0 && *control.Maximum <= 4294967295
		case speechControlSpeakerBoost, speechControlTimestamps, speechControlLanguageNormalization:
			valid = valid && control.Kind == CatalogControlBoolean
		case speechControlNormalization:
			valid = imageEnumSubset(control, []string{"auto", "on", "off"})
		case speechControlLanguage:
			valid = valid && control.Kind == CatalogControlEnum
			for _, value := range control.Values {
				valid = valid && (len(value) == 2 || len(value) == 3)
				for _, character := range value {
					valid = valid && character >= 'a' && character <= 'z'
				}
			}
		default:
			valid = false
		}
		if !valid {
			return fmt.Errorf("%w: field=%s.controls control=%s reason=invalid_speech_control", ErrInvalidModelCatalog, field, control.ID)
		}
	}
	required := map[string]bool{speechControlOutputFormat: false, speechControlStability: false, speechControlStyle: false, speechControlSpeed: false, speechControlSeed: false, speechControlTimestamps: false, speechControlLanguageNormalization: false, speechControlNormalization: false}
	for _, control := range offering.Controls {
		delete(required, control.ID)
	}
	if len(required) != 0 {
		return fmt.Errorf("%w: field=%s.controls reason=missing_speech_control", ErrInvalidModelCatalog, field)
	}
	for _, limit := range offering.Limits {
		valid := !limit.AccountDependent
		switch limit.ID {
		case speechTextLimit:
			valid = valid && limit.Unit == "characters" && *limit.Value <= 40000
		case speechDictionaryLimit:
			valid = valid && limit.Unit == "dictionaries" && *limit.Value <= 3
		case speechContextLimit:
			valid = valid && limit.Unit == "operations_per_direction" && *limit.Value <= 3
		default:
			valid = false
		}
		if !valid {
			return fmt.Errorf("%w: field=%s.limits limit=%s reason=invalid_speech_limit", ErrInvalidModelCatalog, field, limit.ID)
		}
	}
	return nil
}
