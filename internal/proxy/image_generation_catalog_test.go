package proxy_test

import (
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"gopkg.in/yaml.v3"
)

func TestImageGenerationCatalogRejectsInvalidProtocolAndControlContracts(t *testing.T) {
	for name, change := range map[string]func(*proxy.ProviderCatalogOffering, *proxy.ProviderCatalogTransport){
		"unknown codec": func(_ *proxy.ProviderCatalogOffering, transport *proxy.ProviderCatalogTransport) {
			transport.Components.RequestCodec.ID = "unknown_image_codec"
		},
		"codec pair": func(_ *proxy.ProviderCatalogOffering, transport *proxy.ProviderCatalogTransport) {
			transport.Components.ResponseCodec.ID = proxy.CatalogProtocolOpenAIResponses
		},
		"retrieval lifecycle": func(_ *proxy.ProviderCatalogOffering, transport *proxy.ProviderCatalogTransport) {
			transport.Components.Execution.ID = "pollable_resource"
		},
		"text transport": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Transport = "text"
		},
		"text request profile": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.RequestProfile = "openai_responses_reasoning_tools"
		},
		"missing controls": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Controls = nil
		},
		"unknown control": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Controls[0].ID = "unknown"
		},
		"unsupported quality": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Controls[0].Values = []string{"ultra"}
		},
		"quality kind": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Controls[0] = proxy.CatalogControl{ID: "quality", Kind: proxy.CatalogControlBoolean}
		},
		"missing dimensions": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Controls[1].ImageSize = nil
		},
		"dimensions with values": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Controls[1].Values = []string{"auto"}
		},
		"zero dimension multiple": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Controls[1].ImageSize.DimensionMultiple = 0
		},
		"too large dimension": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Controls[1].ImageSize.MaximumEdge = 65536
		},
		"unsupported dimension": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Controls[1].ImageSize.MaximumEdge = 4096
		},
		"unsupported pixels": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Controls[1].ImageSize.MaximumPixels = 10000000
		},
		"inverted pixels": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Controls[1].ImageSize.MinimumPixels = 10000000
		},
		"impossible pixels": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Controls[1].ImageSize.MaximumPixels = 20000000
		},
		"zero aspect ratio": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Controls[1].ImageSize.MaximumAspectRatio = 0
		},
		"enum dimensions": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Controls[0].ImageSize = offering.Controls[1].ImageSize
		},
		"integer dimensions": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Controls[4].ImageSize = offering.Controls[1].ImageSize
		},
		"boolean dimensions": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Controls[0] = proxy.CatalogControl{ID: "quality", Kind: proxy.CatalogControlBoolean, ImageSize: offering.Controls[1].ImageSize}
		},
		"unbounded output count": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			*offering.Controls[5].Maximum = 11
		},
		"zero output count": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			*offering.Controls[5].Minimum = 0
		},
		"compression maximum": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			*offering.Controls[4].Maximum = 101
		},
		"stream kind": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Controls[6] = proxy.CatalogControl{ID: "stream", Kind: proxy.CatalogControlEnum, Values: []string{"yes"}}
		},
		"partial count": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			*offering.Controls[7].Maximum = 4
		},
		"partial minimum": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			*offering.Controls[7].Minimum = 1
		},
		"missing Responses route": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.ImageRoutes.Responses = ""
		},
		"unknown Responses route": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.ImageRoutes.Responses = "missing"
		},
		"wrong Responses codec": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.ImageRoutes.Responses = "image"
		},
		"unknown Responses model": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Controls[9].Values = []string{"not-in-catalog"}
		},
		"image model as Responses model": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Controls[9].Values = []string{"gpt-image-2"}
		},
		"missing Responses surface": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Controls[8].Values = []string{"images"}
		},
		"Responses output count": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			*offering.Limits[6].Value = 2
		},
		"missing editing route": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.ImageRoutes.Editing = ""
		},
		"unknown editing route": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.ImageRoutes.Editing = "missing"
		},
		"wrong editing codec": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.ImageRoutes.Editing = "text"
		},
		"editing without generation": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Operations = []string{proxy.ModelOperationImageEditing}
			offering.DefaultOperations = []string{proxy.ModelOperationImageEditing}
		},
		"route without editing": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Operations = []string{proxy.ModelOperationImageGeneration}
			offering.DefaultOperations = []string{proxy.ModelOperationImageGeneration}
		},
		"too many inputs": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			*offering.Limits[2].Value = 17
		},
		"input byte maximum": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			*offering.Limits[3].Value = 50_000_000
		},
		"input pixel maximum": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			*offering.Limits[4].Value = 1 << 30
		},
		"stream output count": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			*offering.Limits[5].Value = 2
		},
		"account dependent quality": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Controls[0].AccountDependent = true
		},
		"unknown limit": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Limits[0].ID = "unknown"
		},
		"prompt limit": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			*offering.Limits[0].Value = 32001
		},
		"output bytes limit": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			*offering.Limits[1].Value = 1 << 30
		},
		"prompt unit": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Limits[0].Unit = "tokens"
		},
		"account dependent limit": func(offering *proxy.ProviderCatalogOffering, _ *proxy.ProviderCatalogTransport) {
			offering.Limits[0].Value = nil
			offering.Limits[0].AccountDependent = true
		},
	} {
		t.Run(name, func(t *testing.T) {
			schema := testfixtures.ProviderCatalog(t).Schema()
			for providerIndex := range schema.Providers {
				provider := &schema.Providers[providerIndex]
				if provider.ID != "openai" {
					continue
				}
				var imageTransport *proxy.ProviderCatalogTransport
				for transportIndex := range provider.Transports {
					if provider.Transports[transportIndex].ID == "image" {
						imageTransport = &provider.Transports[transportIndex]
					}
				}
				for offeringIndex := range provider.Offerings {
					if provider.Offerings[offeringIndex].Model == "gpt-image-2" {
						change(&provider.Offerings[offeringIndex], imageTransport)
					}
				}
			}
			document, err := yaml.Marshal(schema)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := proxy.ParseProviderCatalog(document); err == nil {
				t.Fatal("invalid image catalog accepted")
			}
		})
	}
}
