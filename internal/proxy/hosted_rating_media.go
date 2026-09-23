package proxy

import (
	"fmt"
	"strconv"
	"time"
)

type mediaPriceComponent struct{ component, unit, cacheClass string }
type mediaPriceRoute struct{ codec, operation string }

// Catalog limits name the same native quantities as the retained usage journal.
// Different units require an explicit price conversion, never a guessed bound.
var mediaCatalogLimitUnits = map[string]string{
	"token": "tokens", "second": "seconds", "provider_unit": "provider_units",
	"image": "images", "call": "calls",
}

// Billing limits describe native usage ceilings, not request controls.
// Capability validators still check every other limit against their route.
func mediaCapabilityLimits(offering ProviderOffering) []CatalogLimit {
	dimensions := map[string]bool{}
	for _, operation := range offering.Operations {
		for _, dimension := range mediaPriceProfiles[mediaPriceRoute{offering.WireContract, operation}] {
			dimensions[dimension] = true
		}
	}
	if offering.WireContract == CatalogProtocolGeminiInteractions || offering.WireContract == CatalogProtocolVertexGenerateContent {
		dimensions["reasoning_tokens"] = true
	}
	limits := make([]CatalogLimit, 0, len(offering.Limits))
	for _, limit := range offering.Limits {
		if !dimensions[limit.ID] {
			limits = append(limits, limit)
		}
	}
	return limits
}

func newHostedMediaPriceAdmission(service CatalogService, request managedJournalRequestRecord, codec string, conditions CatalogPriceConditions, attempts uint32) (journalReservation, error) {
	snapshot, err := newHostedMediaRatingSnapshot(service, request.Provider, request.Model, request.Operation, codec, request.CreatedAt, conditions)
	if err != nil {
		return nil, err
	}
	offering, err := service.ResolveOffering(request.Provider, request.Model)
	if err != nil {
		return nil, err
	}
	limits := make(map[string]CatalogLimit, len(offering.Limits))
	for _, limit := range offering.Limits {
		limits[limit.ID] = limit
	}
	bounds := []CatalogUsageBound{}
	for _, component := range snapshot.components {
		for _, dimension := range []string{component.binding.Dimension, component.binding.AdditionalDimension} {
			if dimension == "" {
				continue
			}
			limit, found := limits[dimension]
			unit := component.unit.quantity
			if !found || limit.AccountDependent || limit.Value == nil || limit.Unit != mediaCatalogLimitUnits[unit] {
				return nil, fmt.Errorf("%w: fixed media limit required for dimension=%s unit=%s", ErrCatalogRatingUnavailable, dimension, unit)
			}
			bounds = append(bounds, CatalogUsageBound{Dimension: dimension, Unit: unit, Maximum: strconv.Itoa(*limit.Value)})
		}
	}
	for _, dimension := range snapshot.zeroDimensions {
		bounds = append(bounds, CatalogUsageBound{Dimension: dimension, Unit: "token", Maximum: "0"})
	}
	return newHostedPriceAdmission(snapshot, bounds, attempts)
}

var imagePriceDimensions = map[mediaPriceComponent]string{
	{"input_text", "USD/1M_tokens", ""}:   "input_text_tokens",
	{"input_image", "USD/1M_tokens", ""}:  "input_image_tokens",
	{"output_text", "USD/1M_tokens", ""}:  "output_text_tokens",
	{"output_image", "USD/1M_tokens", ""}: "output_image_tokens",
}

var multipartPriceDimensions = map[mediaPriceComponent]string{
	{"input_audio", "USD/minute", ""}:     "audio_seconds",
	{"input_audio", "USD/hour", ""}:       "audio_seconds",
	{"input_audio", "USD/second", ""}:     "audio_seconds",
	{"input_audio", "USD/1M_tokens", ""}:  "input_audio_tokens",
	{"input_text", "USD/1M_tokens", ""}:   "input_text_tokens",
	{"input_tokens", "USD/1M_tokens", ""}: "input_tokens",
	{"output_text", "USD/1M_tokens", ""}:  "output_tokens",
}

var speechPriceDimensions = map[mediaPriceComponent]string{
	{"character_cost", "USD/provider_unit", ""}: "character_cost",
}

var googleAudioPriceDimensions = map[mediaPriceComponent]string{
	{"input_audio", "USD/1M_tokens", ""}: "input_audio_tokens",
	{"input_text", "USD/1M_tokens", ""}:  "input_text_tokens",
	{"output_text", "USD/1M_tokens", ""}: "output_tokens",
}

// Every conversion names the native meter quantity and its catalog unit.
// A provider-reported cost unit has no implicit USD, character, or time value.
var mediaPriceProfiles = map[mediaPriceRoute]map[mediaPriceComponent]string{
	{CatalogProtocolMultipartTranscription, ModelOperationDictation}:      multipartPriceDimensions,
	{CatalogProtocolGeminiInteractions, ModelOperationDictation}:          googleAudioPriceDimensions,
	{CatalogProtocolVertexGenerateContent, ModelOperationDictation}:       googleAudioPriceDimensions,
	{CatalogProtocolOpenAIImages, ModelOperationImageGeneration}:          imagePriceDimensions,
	{CatalogProtocolOpenAIImages, ModelOperationImageEditing}:             imagePriceDimensions,
	{CatalogProtocolElevenLabsSpeech, ModelOperationSpeechGeneration}:     speechPriceDimensions,
	{CatalogProtocolElevenLabsConversion, ModelOperationSpeechConversion}: speechPriceDimensions,
	{CatalogProtocolFALQueueImages, ModelOperationImageGeneration}: {
		{"billable_units", "USD/provider_unit", ""}: "billable_units",
	},
	{CatalogProtocolDictatorSpeechV1, ModelOperationSpeechGeneration}: {
		{"output_audio", "USD/second", ""}: "output_audio_seconds",
		{"output_audio", "USD/minute", ""}: "output_audio_seconds",
		{"output_audio", "USD/hour", ""}:   "output_audio_seconds",
	},
}

func newHostedMediaRatingSnapshot(service CatalogService, provider, model, operation, codec string, acceptedAt time.Time, conditions CatalogPriceConditions) (*CatalogRatingSnapshot, error) {
	profile, supported := mediaPriceProfiles[mediaPriceRoute{codec, operation}]
	if !supported {
		return nil, fmt.Errorf("%w: native media price mapping unavailable for codec=%s operation=%s", ErrCatalogRatingUnavailable, codec, operation)
	}
	if conditions != categoricalPriceConditions(conditions) || conditions.CacheClass != "" || conditions.Duration != "" {
		return nil, fmt.Errorf("%w: unsupported media price conditions", ErrCatalogRatingUnavailable)
	}
	descriptor, found := service.catalog.prices[catalogPriceIdentifier(provider, model, operation)]
	if !found || !descriptor.Available {
		return nil, fmt.Errorf("%w: media price unavailable", ErrCatalogRatingUnavailable)
	}
	bindings := []CatalogRateBinding{}
	selected := map[CatalogRateBinding]bool{}
	zeroDimensions := []string{}
	if codec == CatalogProtocolGeminiInteractions || codec == CatalogProtocolVertexGenerateContent {
		zeroDimensions = append(zeroDimensions, "tool_input_tokens")
	}
	for _, rate := range descriptor.Rates {
		categorical := categoricalPriceConditions(rate.Conditions)
		base := categorical
		base.CacheClass = ""
		if base != conditions || !catalogPriceActiveAt(rate.Conditions, acceptedAt) {
			continue
		}
		dimension, found := profile[mediaPriceComponent{rate.Component, rate.Unit, categorical.CacheClass}]
		if !found {
			return nil, fmt.Errorf("%w: media component=%s unit=%s has no native billing quantity", ErrCatalogRatingUnavailable, rate.Component, rate.Unit)
		}
		binding := CatalogRateBinding{Dimension: dimension, Component: rate.Component, Conditions: categorical}
		if dimension == "output_tokens" && (codec == CatalogProtocolGeminiInteractions || codec == CatalogProtocolVertexGenerateContent) {
			binding.AdditionalDimension = "reasoning_tokens"
		}
		if !selected[binding] {
			selected[binding] = true
			bindings = append(bindings, binding)
		}
	}
	return service.newRatingSnapshot(provider, model, operation, acceptedAt, bindings, nil, zeroDimensions)
}
