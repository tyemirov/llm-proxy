package proxy

import (
	"fmt"
	"math/big"
)

const responsesPricePrefix = "responses_"

// One Responses request buys work from the response model and its image tool.
// The accepted image controls bind the selected model and one tool invocation.
func newHostedResponsesImagePriceAdmission(service CatalogService, request managedJournalRequestRecord, conditions CatalogPriceConditions, attempts uint32, controls imageGenerationControls) (journalReservation, error) {
	image, bounds, err := newHostedMediaPricing(service, request, CatalogProtocolOpenAIImages, conditions)
	if err != nil {
		return nil, err
	}
	// Catalog validation resolves every advertised response model. The image
	// adapter accepts only those models before financial admission.
	responseOffering, _ := service.ResolveOffering(request.Provider, controls.ResponsesModel)
	conditions.Quality, conditions.Resolution = "", ""
	scope := hostedTextPriceScope{catalog: service, offering: responseOffering, conditions: conditions, maximumAttempts: attempts}
	response, responseBounds, err := scope.pricing(CatalogProtocolOpenAIResponses, request.CreatedAt, false)
	if err != nil {
		return nil, err
	}
	// The current advertised OpenAI image and response schedules have no
	// per-request minimum or input tiers. Such schedules need separate groups.
	if image.descriptor.MinimumCharge != nil || response.descriptor.MinimumCharge != nil {
		return nil, fmt.Errorf("%w: combined image prices cannot share a request minimum", ErrCatalogRatingUnavailable)
	}
	for _, snapshot := range []*CatalogRatingSnapshot{image, response} {
		origin := &catalogPriceOrigin{Provider: snapshot.descriptor.Provider, Model: snapshot.descriptor.Model, Operation: snapshot.descriptor.Operation, Source: snapshot.descriptor.Source, LastVerified: snapshot.descriptor.LastVerified}
		for index := range snapshot.components {
			component := &snapshot.components[index]
			component.origin = origin
			for _, rate := range component.rates {
				interval := rate.Conditions.InputTokens
				if interval.Minimum != 0 || interval.MaximumExclusive != 0 {
					return nil, fmt.Errorf("%w: combined image prices require an unconditional token schedule", ErrCatalogRatingUnavailable)
				}
			}
		}
	}
	for _, component := range response.components {
		component.binding.Dimension = responsesPricePrefix + component.binding.Dimension
		component.binding.Component = responsesPricePrefix + component.binding.Component
		for index := range component.rates {
			component.rates[index].Component = responsesPricePrefix + component.rates[index].Component
		}
		image.components = append(image.components, component)
	}
	// Include the pass before the tool and the pass after it. Catalog ceilings
	// bound each pass. Cache ceilings never establish a discount for the hold.
	passes := big.NewRat(int64(controls.OutputCount)+1, 1)
	for _, bound := range responseBounds {
		bound.Dimension = responsesPricePrefix + bound.Dimension
		bound.Maximum = ratingDecimal(new(big.Rat).Mul(ratingRational(bound.Maximum), passes))
		bounds = append(bounds, bound)
	}
	return newHostedPriceAdmission(image, bounds, attempts)
}
