package proxy

import (
	"strings"
	"testing"
)

func TestCatalogServiceResolvesExactRoutesAndPrices(t *testing.T) {
	minimum := 1
	maximum := 15
	reasoningOffering := internalTestOffering(ProviderNameOpenAI, ModelNameGPT55, []string{ModelOperationText}, []string{ModelOperationText})
	reasoningOffering.RequestProfile = string(requestProfileOpenAIResponsesReasoningTools)
	reasoningOffering.ReasoningEffort = &ReasoningEffortCapability{
		Adapter: string(reasoningEffortAdapterOpenAIResponses),
		Efforts: []string{"minimal", "low", "medium", "high"},
	}
	catalog := internalTestModelCatalog(
		internalTestOffering(ProviderNameXAI, ModelNameGrok43, []string{ModelOperationText}, []string{ModelOperationText}),
		ProviderOffering{
			Provider: ProviderNameXAI, Model: "grok-imagine-video-1.5", ProviderModel: "grok-imagine-video-1.5",
			Operations: []string{ModelOperationVideoGeneration}, DefaultOperations: []string{ModelOperationVideoGeneration},
			WireContract: "xai_videos_generations", ExecutionLifecycle: string(textExecutionLifecyclePollableResource),
			Controls: []CatalogControl{{ID: "duration", Kind: CatalogControlInteger, Minimum: &minimum, Maximum: &maximum}},
			Limits:   []CatalogLimit{{ID: "concurrent_requests", Unit: "requests", AccountDependent: true}},
		},
		reasoningOffering,
	)
	conditions := CatalogPriceConditions{Resolution: "720p", Duration: "output"}
	videoPriceIndex := -1
	for priceIndex := range catalog.Prices {
		if catalog.Prices[priceIndex].Operation != ModelOperationVideoGeneration {
			continue
		}
		videoPriceIndex = priceIndex
		catalog.Prices[priceIndex] = CatalogPriceDescriptor{
			Provider: ProviderNameXAI, Model: "grok-imagine-video-1.5", Operation: ModelOperationVideoGeneration,
			Available:     true,
			Rates:         []CatalogPriceRate{{Component: "output_video", Currency: CatalogCurrencyUSD, Rate: 0.14, Unit: "USD/output_second", Conditions: conditions}},
			MinimumCharge: &CatalogMinimumCharge{Currency: CatalogCurrencyUSD, Amount: 0.14, Unit: "USD/request"},
			Source:        "https://docs.x.ai/developers/pricing", LastVerified: "2026-08-08",
		}
	}

	service, serviceError := NewCatalogService(catalog)
	if serviceError != nil {
		t.Fatalf("NewCatalogService error: %v", serviceError)
	}
	catalog.Offerings[1].Operations[0] = ModelOperationDictation
	*catalog.Offerings[1].Controls[0].Minimum = 9
	catalog.Offerings[1].Limits[0].Unit = "changed"
	catalog.Offerings[2].ReasoningEffort.Efforts[0] = "changed"
	catalog.Prices[videoPriceIndex].Rates[0].Rate = 9
	catalog.Prices[videoPriceIndex].MinimumCharge.Amount = 9
	if service.Revision() != catalog.Revision {
		t.Fatalf("revision=%q", service.Revision())
	}
	offering, offeringError := service.ResolveOffering(" xai ", " grok-imagine-video-1.5 ")
	if offeringError != nil || offering.Model != "grok-imagine-video-1.5" || offering.Operations[0] != ModelOperationVideoGeneration || *offering.Controls[0].Minimum != 1 || offering.Limits[0].Unit != "requests" {
		t.Fatalf("ResolveOffering offering=%+v error=%v", offering, offeringError)
	}
	offering.Operations[0] = ModelOperationDictation
	*offering.Controls[0].Minimum = 12
	offering.Limits[0].Unit = "changed again"
	resolvedAgain, resolvedAgainError := service.ResolveOffering(ProviderNameXAI, "grok-imagine-video-1.5")
	if resolvedAgainError != nil || resolvedAgain.Operations[0] != ModelOperationVideoGeneration || *resolvedAgain.Controls[0].Minimum != 1 || resolvedAgain.Limits[0].Unit != "requests" {
		t.Fatalf("ResolveOffering second result=%+v error=%v", resolvedAgain, resolvedAgainError)
	}
	reasoningResolved, reasoningResolvedError := service.ResolveOffering(ProviderNameOpenAI, ModelNameGPT55)
	if reasoningResolvedError != nil || reasoningResolved.ReasoningEffort == nil || reasoningResolved.ReasoningEffort.Efforts[0] != "minimal" {
		t.Fatalf("ResolveOffering reasoning result=%+v error=%v", reasoningResolved, reasoningResolvedError)
	}
	reasoningResolved.ReasoningEffort.Efforts[0] = "changed again"
	reasoningResolvedAgain, reasoningResolvedAgainError := service.ResolveOffering(ProviderNameOpenAI, ModelNameGPT55)
	if reasoningResolvedAgainError != nil || reasoningResolvedAgain.ReasoningEffort == nil || reasoningResolvedAgain.ReasoningEffort.Efforts[0] != "minimal" {
		t.Fatalf("ResolveOffering reasoning second result=%+v error=%v", reasoningResolvedAgain, reasoningResolvedAgainError)
	}
	if _, offeringError := service.ResolveOffering(ProviderNameXAI, "missing"); offeringError == nil {
		t.Fatal("ResolveOffering accepted an unknown route")
	}
	selection := service.SelectPrice(ProviderNameXAI, "grok-imagine-video-1.5", ModelOperationVideoGeneration, " output_video ", conditions)
	if !selection.Available || selection.Rate == nil || selection.Rate.Rate != 0.14 || selection.MinimumCharge == nil {
		t.Fatalf("exact selection=%+v", selection)
	}
	selection.Rate.Rate = 12
	selection.MinimumCharge.Amount = 12
	selectionAgain := service.SelectPrice(ProviderNameXAI, "grok-imagine-video-1.5", ModelOperationVideoGeneration, "output_video", conditions)
	if selectionAgain.Rate == nil || selectionAgain.Rate.Rate != 0.14 || selectionAgain.MinimumCharge == nil || selectionAgain.MinimumCharge.Amount != 0.14 {
		t.Fatalf("exact selection after result mutation=%+v", selectionAgain)
	}
	if unavailable := service.SelectPrice(ProviderNameXAI, ModelNameGrok43, ModelOperationText, "input", CatalogPriceConditions{}); unavailable.Available || unavailable.UnavailableReason != "Test price is unavailable." {
		t.Fatalf("unavailable selection=%+v", unavailable)
	}
	if mismatch := service.SelectPrice(ProviderNameXAI, "grok-imagine-video-1.5", ModelOperationVideoGeneration, "output_video", CatalogPriceConditions{Resolution: "480p"}); mismatch.UnavailableReason != "exact_price_unavailable" {
		t.Fatalf("mismatch=%+v", mismatch)
	}
	if missing := service.SelectPrice(ProviderNameXAI, "missing", ModelOperationText, "input", CatalogPriceConditions{}); missing.UnavailableReason != "price_not_cataloged" {
		t.Fatalf("missing=%+v", missing)
	}
}

func TestCatalogCapabilityValidationRejectsIncompleteContracts(t *testing.T) {
	integer := func(value int) *int { return &value }
	assertError := func(t *testing.T, validationError error, expected string) {
		t.Helper()
		if validationError == nil || !strings.Contains(validationError.Error(), expected) {
			t.Fatalf("error=%v want contains %q", validationError, expected)
		}
	}

	t.Run("catalog service", func(t *testing.T) {
		_, validationError := NewCatalogService(ModelCatalog{})
		assertError(t, validationError, "catalog.revision")
	})
	t.Run("catalog operations", func(t *testing.T) {
		catalog := internalTestModelCatalog(internalTestOffering(ProviderNameOpenAI, ModelNameGPT41, []string{ModelOperationText}, []string{ModelOperationText}))
		catalog.Operations = nil
		_, validationError := NewCatalogService(catalog)
		assertError(t, validationError, "catalog.operations")
	})
	t.Run("catalog credentials", func(t *testing.T) {
		catalog := internalTestModelCatalog(internalTestOffering(ProviderNameOpenAI, ModelNameGPT41, []string{ModelOperationText}, []string{ModelOperationText}))
		catalog.Providers[0].CredentialKinds = []string{"oauth"}
		_, validationError := NewCatalogService(catalog)
		assertError(t, validationError, "credential_kinds")
	})
	t.Run("catalog controls", func(t *testing.T) {
		catalog := internalTestModelCatalog(internalTestOffering(ProviderNameOpenAI, ModelNameGPT41, []string{ModelOperationText}, []string{ModelOperationText}))
		catalog.Offerings[0].Controls = []CatalogControl{{ID: "mode", Kind: "future"}}
		_, validationError := NewCatalogService(catalog)
		assertError(t, validationError, "controls")
	})
	t.Run("catalog limits", func(t *testing.T) {
		catalog := internalTestModelCatalog(internalTestOffering(ProviderNameOpenAI, ModelNameGPT41, []string{ModelOperationText}, []string{ModelOperationText}))
		catalog.Offerings[0].Limits = []CatalogLimit{{ID: "requests", Unit: "requests"}}
		_, validationError := NewCatalogService(catalog)
		assertError(t, validationError, "limits")
	})
	t.Run("catalog video route", func(t *testing.T) {
		catalog := internalTestModelCatalog(ProviderOffering{
			Provider: ProviderNameXAI, Model: "video", ProviderModel: "video",
			Operations: []string{ModelOperationVideoGeneration}, DefaultOperations: []string{ModelOperationVideoGeneration},
			WireContract: "xai_videos_generations", ExecutionLifecycle: string(textExecutionLifecyclePollableResource),
			Controls: []CatalogControl{{ID: "enabled", Kind: CatalogControlBoolean}},
			Limits:   []CatalogLimit{{ID: "requests", Unit: "requests", AccountDependent: true}},
		})
		catalog.Offerings[0].WireContract = "invalid"
		_, validationError := NewCatalogService(catalog)
		assertError(t, validationError, "unsupported_video_route")
	})
	for _, secondaryOperation := range []string{ModelOperationVideoGeneration, ModelOperationDictation} {
		t.Run("catalog validates secondary "+secondaryOperation, func(t *testing.T) {
			catalog := internalTestModelCatalog(internalTestOffering(
				ProviderNameXAI,
				ModelNameGrok43,
				[]string{ModelOperationText, secondaryOperation},
				[]string{ModelOperationText, secondaryOperation},
			))
			_, validationError := NewCatalogService(catalog)
			assertError(t, validationError, "unsupported_"+strings.TrimSuffix(secondaryOperation, "_generation")+"_route")
		})
	}
	t.Run("catalog publisher without exact model", func(t *testing.T) {
		catalog := internalTestModelCatalog(internalTestOffering(ProviderNameOpenAI, ModelNameGPT41, []string{ModelOperationText}, []string{ModelOperationText}))
		catalog.Publishers = append(catalog.Publishers, ModelPublisher{ID: "unused", Label: "Unused"})
		catalog.Families = append(catalog.Families, ModelFamily{ID: "unused", Publisher: "unused", Label: "Unused"})
		_, validationError := NewCatalogService(catalog)
		assertError(t, validationError, "publisher=unused reason=missing_exact_model")
	})
	t.Run("catalog prices", func(t *testing.T) {
		catalog := internalTestModelCatalog(internalTestOffering(ProviderNameOpenAI, ModelNameGPT41, []string{ModelOperationText}, []string{ModelOperationText}))
		catalog.Prices = nil
		_, validationError := NewCatalogService(catalog)
		assertError(t, validationError, "catalog.prices")
	})
	assertError(t, validateCatalogRevision(" Future "), "not_canonical")

	operationCases := []struct {
		name       string
		operations []ModelOperationKind
		expected   string
	}{
		{name: "empty", expected: "catalog.operations"},
		{name: "noncanonical", operations: []ModelOperationKind{{ID: "Text"}}, expected: "not_canonical"},
		{name: "unknown", operations: []ModelOperationKind{{ID: "future", InputArtifacts: []string{"text"}, OutputArtifacts: []string{"text"}}}, expected: "operation=future"},
		{name: "duplicate", operations: []ModelOperationKind{{ID: "text", InputArtifacts: []string{"text"}, OutputArtifacts: []string{"text"}}, {ID: "text", InputArtifacts: []string{"text"}, OutputArtifacts: []string{"text"}}}, expected: "duplicate_identifier=text"},
		{name: "input", operations: []ModelOperationKind{{ID: "text", OutputArtifacts: []string{"text"}}}, expected: "input_artifacts"},
		{name: "output", operations: []ModelOperationKind{{ID: "text", InputArtifacts: []string{"text"}}}, expected: "output_artifacts"},
		{name: "missing required", operations: []ModelOperationKind{{ID: "text", InputArtifacts: []string{"text"}, OutputArtifacts: []string{"text"}}}, expected: "reason=missing"},
	}
	for _, testCase := range operationCases {
		t.Run("operation "+testCase.name, func(t *testing.T) {
			assertError(t, validateModelOperationKinds(testCase.operations, map[string]ModelOperationKind{}), testCase.expected)
		})
	}
	assertError(t, validateArtifactKinds(nil, "artifacts"), "field=artifacts")
	assertError(t, validateArtifactKinds([]string{"future"}, "artifacts"), "artifact=future")
	assertError(t, validateArtifactKinds([]string{"text", "text"}, "artifacts"), "duplicate=text")
	assertError(t, validateCredentialKinds([]string{"oauth"}, "credentials"), "field=credentials")

	controlCases := []struct {
		name     string
		controls []CatalogControl
		expected string
	}{
		{name: "identifier", controls: []CatalogControl{{ID: "Future Control", Kind: CatalogControlBoolean}}, expected: "not_canonical"},
		{name: "duplicate", controls: []CatalogControl{{ID: "mode", Kind: CatalogControlBoolean}, {ID: "mode", Kind: CatalogControlBoolean}}, expected: "duplicate_identifier=mode"},
		{name: "enum shape", controls: []CatalogControl{{ID: "mode", Kind: CatalogControlEnum}}, expected: "kind=enum"},
		{name: "enum value", controls: []CatalogControl{{ID: "mode", Kind: CatalogControlEnum, Values: []string{" "}}}, expected: "values[0]"},
		{name: "enum duplicate", controls: []CatalogControl{{ID: "mode", Kind: CatalogControlEnum, Values: []string{"fast", "fast"}}}, expected: "duplicate=fast"},
		{name: "integer", controls: []CatalogControl{{ID: "duration", Kind: CatalogControlInteger, Minimum: integer(2), Maximum: integer(1)}}, expected: "kind=integer"},
		{name: "negative minimum", controls: []CatalogControl{{ID: "duration", Kind: CatalogControlInteger, Minimum: integer(-1), Maximum: integer(1)}}, expected: "kind=integer"},
		{name: "negative maximum", controls: []CatalogControl{{ID: "duration", Kind: CatalogControlInteger, Minimum: integer(0), Maximum: integer(-1)}}, expected: "kind=integer"},
		{name: "boolean", controls: []CatalogControl{{ID: "audio", Kind: CatalogControlBoolean, Values: []string{"yes"}}}, expected: "kind=boolean"},
		{name: "kind", controls: []CatalogControl{{ID: "mode", Kind: "future"}}, expected: "kind=future"},
	}
	for _, testCase := range controlCases {
		t.Run("control "+testCase.name, func(t *testing.T) {
			assertError(t, validateCatalogControls(testCase.controls, "controls"), testCase.expected)
		})
	}

	limitCases := []struct {
		name     string
		limits   []CatalogLimit
		expected string
	}{
		{name: "identifier", limits: []CatalogLimit{{ID: "Future Limit", Unit: "requests", AccountDependent: true}}, expected: "not_canonical"},
		{name: "duplicate", limits: []CatalogLimit{{ID: "jobs", Unit: "requests", AccountDependent: true}, {ID: "jobs", Unit: "requests", AccountDependent: true}}, expected: "duplicate_identifier=jobs"},
		{name: "unit", limits: []CatalogLimit{{ID: "jobs", Unit: " ", AccountDependent: true}}, expected: ".unit"},
		{name: "account dependent value", limits: []CatalogLimit{{ID: "jobs", Unit: "requests", AccountDependent: true, Value: integer(1)}}, expected: "reason=account_dependent"},
		{name: "fixed value", limits: []CatalogLimit{{ID: "jobs", Unit: "requests"}}, expected: ".value"},
	}
	for _, testCase := range limitCases {
		t.Run("limit "+testCase.name, func(t *testing.T) {
			assertError(t, validateCatalogLimits(testCase.limits, "limits"), testCase.expected)
		})
	}

	validVideo := ProviderOffering{
		Provider: ProviderNameXAI, WireContract: "xai_videos_generations", ExecutionLifecycle: string(textExecutionLifecyclePollableResource),
		Controls: []CatalogControl{{ID: "enabled", Kind: CatalogControlBoolean}},
		Limits:   []CatalogLimit{{ID: "jobs", Unit: "requests", AccountDependent: true}},
	}
	invalidVideo := validVideo
	invalidVideo.Provider = ProviderNameOpenAI
	assertError(t, validateVideoOffering(invalidVideo, "video"), "unsupported_video_route")
	invalidVideo = validVideo
	invalidVideo.WebSearch = true
	assertError(t, validateVideoOffering(invalidVideo, "video"), "text_capabilities_on_video_route")
	invalidVideo = validVideo
	invalidVideo.Controls = nil
	assertError(t, validateVideoOffering(invalidVideo, "video"), "incomplete_video_capabilities")
}

func TestCatalogPriceValidationRejectsAmbiguousRecords(t *testing.T) {
	validOffering := ProviderOffering{Provider: ProviderNameXAI, Model: "model", Operations: []string{ModelOperationVideoGeneration}}
	validRate := CatalogPriceRate{Component: "output", Currency: CatalogCurrencyUSD, Rate: 1, Unit: "USD/second", Conditions: CatalogPriceConditions{Resolution: "720p"}}
	validPrice := CatalogPriceDescriptor{
		Provider: ProviderNameXAI, Model: "model", Operation: ModelOperationVideoGeneration, Available: true,
		Rates: []CatalogPriceRate{validRate}, Source: "https://example.com/pricing", LastVerified: "2026-08-10",
	}
	freshPrice := func() CatalogPriceDescriptor {
		price := validPrice
		price.Rates = append([]CatalogPriceRate(nil), validPrice.Rates...)
		return price
	}
	newCatalog := func() validatedModelCatalog {
		return validatedModelCatalog{offerings: map[string]ProviderOffering{providerOfferingIdentifier(validOffering.Provider, validOffering.Model): validOffering}, prices: map[string]CatalogPriceDescriptor{}}
	}
	assertError := func(t *testing.T, prices []CatalogPriceDescriptor, catalog validatedModelCatalog, expected string) {
		t.Helper()
		validationError := validateCatalogPrices(prices, catalog)
		if validationError == nil || !strings.Contains(validationError.Error(), expected) {
			t.Fatalf("error=%v want contains %q", validationError, expected)
		}
	}
	assertError(t, nil, newCatalog(), "catalog.prices")
	dangling := freshPrice()
	dangling.Model = "missing"
	assertError(t, []CatalogPriceDescriptor{dangling}, newCatalog(), "dangling_reference")
	assertError(t, []CatalogPriceDescriptor{freshPrice(), freshPrice()}, newCatalog(), "price_conflict")
	invalid := freshPrice()
	invalid.Source = "http://example.com"
	assertError(t, []CatalogPriceDescriptor{invalid}, newCatalog(), ".source")
	invalid = freshPrice()
	invalid.LastVerified = "today"
	assertError(t, []CatalogPriceDescriptor{invalid}, newCatalog(), ".last_verified")
	invalid = freshPrice()
	invalid.Rates = nil
	assertError(t, []CatalogPriceDescriptor{invalid}, newCatalog(), "incomplete_available_price")
	invalid = freshPrice()
	invalid.Available = false
	assertError(t, []CatalogPriceDescriptor{invalid}, newCatalog(), "incomplete_unavailable_price")
	invalid = freshPrice()
	invalid.Rates[0].Currency = "EUR"
	assertError(t, []CatalogPriceDescriptor{invalid}, newCatalog(), ".rates[0]")
	invalid = freshPrice()
	invalid.Rates = append(invalid.Rates, validRate)
	assertError(t, []CatalogPriceDescriptor{invalid}, newCatalog(), "reason=ambiguous")
	invalid = freshPrice()
	invalid.MinimumCharge = &CatalogMinimumCharge{Currency: "EUR", Amount: 1, Unit: "EUR/request"}
	assertError(t, []CatalogPriceDescriptor{invalid}, newCatalog(), ".minimum_charge")
	missingOperationCatalog := newCatalog()
	multipleOperations := validOffering
	multipleOperations.Operations = []string{ModelOperationVideoGeneration, ModelOperationText}
	missingOperationCatalog.offerings[providerOfferingIdentifier(multipleOperations.Provider, multipleOperations.Model)] = multipleOperations
	assertError(t, []CatalogPriceDescriptor{freshPrice()}, missingOperationCatalog, "reason=missing")
}
