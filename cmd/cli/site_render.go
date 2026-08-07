package main

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/viper"
	"github.com/tyemirov/llm-proxy/internal/constants"
	"github.com/tyemirov/llm-proxy/internal/proxy"
)

const (
	siteCNAMEFileName            = "CNAME"
	siteIndexFileName            = "index.html"
	siteApplicationDirectory     = "app"
	siteConfigURLAttribute       = "data-config-url"
	siteConfigURLSourceAttribute = siteConfigURLAttribute + `="` + proxy.ManagementConfigUIPath + `"`
	siteLegacyRuntimeConfig      = "llm-proxy-config.json"
	siteCapabilityCatalogMarker  = "<!-- llm-proxy-capability-catalog -->"
	renderedSiteFilePerm         = 0o644
	defaultSiteSourceDirectory   = "site"
	defaultSiteConfigURL         = proxy.ManagementConfigUIPath
	secureSiteConfigURLScheme    = "https"
	binaryBytesPerMiB            = 1024 * 1024
)

const siteCapabilityCatalogTemplate = `<capability-catalog data-enhanced="false">
  <div class="catalog-summary" aria-label="Catalog summary">
    <p><strong>{{.ProviderCount}}</strong><span>Providers</span></p>
    <p><strong>{{.ModelCount}}</strong><span>Models</span></p>
    <p><strong>{{.CapabilityCount}}</strong><span>Filterable capabilities</span></p>
  </div>
  <form class="catalog-toolbar" data-catalog-toolbar role="search" aria-label="Search and filter models">
    <div class="catalog-search-row">
      <input type="search" name="catalog-search" autocomplete="off" aria-label="Search all model characteristics" placeholder="Search provider, model, capability, contract, or limit" data-catalog-search>
      <button type="button" class="catalog-search-submit" aria-label="Toggle capability filters" aria-controls="catalog-capability-filters" aria-expanded="false" data-catalog-search-submit>
        <svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="10.5" cy="10.5" r="6.5"></circle><path d="m15.5 15.5 4.5 4.5"></path></svg>
      </button>
    </div>
    <div id="catalog-capability-filters" class="catalog-filter-panel" data-catalog-filter-panel hidden>
      <fieldset class="catalog-filter-group">
        <legend>Capabilities <span>Match all selected</span></legend>
        <div class="catalog-filters">
          {{range .CapabilityFilters}}<label class="catalog-filter"><input type="checkbox" name="catalog-capability" value="{{.Identifier}}" data-catalog-capability><span>{{.Label}}</span></label>{{end}}
        </div>
      </fieldset>
      <div class="catalog-toolbar__status">
        <output aria-live="polite" data-catalog-result-count>{{.ModelCount}} models</output>
        <button type="reset" data-catalog-reset>Reset</button>
      </div>
    </div>
  </form>
  <div class="catalog-table-wrap" tabindex="0" role="region" aria-label="Provider and model capability matrix">
    <table class="catalog-table">
      <caption>Current model capabilities generated from the validated LLM Proxy provider registry.</caption>
      <thead>
        <tr>
          <th scope="col" aria-sort="ascending" data-catalog-sort-header="provider"><button type="button" class="catalog-sort-button" data-catalog-sort="provider" data-sort-label="Provider" disabled>Provider<span class="catalog-sort-indicator" aria-hidden="true"></span></button></th>
          <th scope="col" data-catalog-sort-header="model"><button type="button" class="catalog-sort-button" data-catalog-sort="model" data-sort-label="Model" disabled>Model<span class="catalog-sort-indicator" aria-hidden="true"></span></button></th>
          <th scope="col" data-catalog-sort-header="capabilities"><button type="button" class="catalog-sort-button" data-catalog-sort="capabilities" data-sort-label="Capabilities" disabled>Capabilities<span class="catalog-sort-indicator" aria-hidden="true"></span></button></th>
        </tr>
      </thead>
      <tbody data-catalog-body>
        {{range .Models}}<tr data-catalog-row data-provider="{{.ProviderIdentifier}}" data-model="{{.Identifier}}" data-capabilities="{{.CapabilityIdentifiers}}" data-capability-count="{{len .Capabilities}}" data-catalog-search-text="{{.SearchText}}">
          <td class="catalog-provider"><strong>{{.ProviderLabel}}</strong><code>{{.ProviderIdentifier}}</code></td>
          <td class="catalog-model"><code data-catalog-model-id>{{.Identifier}}</code>{{range .Defaults}}<span class="catalog-model__default" title="{{.Description}}">{{.Label}}</span>{{end}}</td>
          <td><div class="catalog-capabilities">
            {{range .Capabilities}}<button type="button" class="capability-badge {{.ClassName}}" aria-label="Filter by {{.Label}}" data-catalog-capability-action="{{.Identifier}}" disabled>{{.Label}}</button>{{end}}
          </div>
          <div class="catalog-technical">
            {{if .WireContract}}<code>{{.WireContract}}</code>{{end}}
            {{if .ReasoningEfforts}}<span>Reasoning: {{.ReasoningEfforts}}</span>{{end}}
            {{if .OutputLimitLabel}}<span>{{.OutputLimitLabel}}</span>{{end}}
          </div></td>
        </tr>{{end}}
      </tbody>
    </table>
    <p class="catalog-empty" data-catalog-empty hidden>No models match the selected filters.</p>
  </div>
</capability-catalog>
<div class="catalog-limits" aria-label="Proxy request limits">
  <p><strong>{{.MaxPromptSize}}</strong>Maximum JSON request body</p>
  <p><strong>{{.MaxAudioSize}}</strong>Maximum input audio</p>
  <p><strong>{{.MaxRequestTimeout}}</strong>Maximum request work budget</p>
</div>`

var errSiteRenderFailed = errors.New("site_render_failed")

type siteConfigURL string

type siteCapabilityCatalogView struct {
	ProviderCount     int
	ModelCount        int
	CapabilityCount   int
	CapabilityFilters []siteCapabilityDefinition
	Models            []siteCapabilityModelView
	MaxPromptSize     string
	MaxAudioSize      string
	MaxRequestTimeout string
}

type siteCapabilityModelView struct {
	ProviderIdentifier    string
	ProviderLabel         string
	Identifier            string
	Defaults              []siteDefaultDefinition
	Capabilities          []siteCapabilityDefinition
	CapabilityIdentifiers string
	WireContract          string
	OutputTokenLimit      int
	OutputLimitLabel      string
	ReasoningEfforts      string
	SearchText            string
}

type siteCapabilityDefinition struct {
	Identifier string
	Label      string
	ClassName  string
}

type siteDefaultDefinition struct {
	Label       string
	Description string
}

var (
	siteCapabilityCatalogTemplateSource = siteCapabilityCatalogTemplate
	siteCopyFS                          = os.CopyFS
	siteExecuteCapabilityCatalog        = func(catalogTemplate *template.Template, renderedCatalog *strings.Builder, catalogView siteCapabilityCatalogView) error {
		return catalogTemplate.Execute(renderedCatalog, catalogView)
	}
	sitePathAbs   = filepath.Abs
	sitePathRel   = filepath.Rel
	siteReadFile  = os.ReadFile
	siteRemove    = os.Remove
	siteStat      = os.Stat
	siteURLParse  = url.Parse
	siteWriteFile = os.WriteFile
)

func newSiteConfigURL(rawValue string) (siteConfigURL, error) {
	normalizedValue := strings.TrimSpace(rawValue)
	parsedURL, parseError := siteURLParse(normalizedValue)
	if parseError != nil {
		return "", fmt.Errorf("%w: site config URL=%q: %v", errSiteRenderFailed, normalizedValue, parseError)
	}
	if parsedURL.RawQuery != constants.EmptyString || parsedURL.Fragment != constants.EmptyString || parsedURL.Path != proxy.ManagementConfigUIPath {
		return "", fmt.Errorf("%w: site config URL=%q must target %s without query or fragment", errSiteRenderFailed, normalizedValue, proxy.ManagementConfigUIPath)
	}
	if parsedURL.IsAbs() {
		if parsedURL.Scheme != secureSiteConfigURLScheme || parsedURL.Host == constants.EmptyString {
			return "", fmt.Errorf("%w: site config URL=%q must use https", errSiteRenderFailed, normalizedValue)
		}
		return siteConfigURL(normalizedValue), nil
	}
	if normalizedValue != proxy.ManagementConfigUIPath {
		return "", fmt.Errorf("%w: site config URL=%q must be %s or an absolute https URL", errSiteRenderFailed, normalizedValue, proxy.ManagementConfigUIPath)
	}
	return siteConfigURL(normalizedValue), nil
}

func loadSiteCapabilityCatalog(rawConfigPath string) (proxy.PublicCapabilityCatalog, error) {
	configPath := normalizedConfigPath(rawConfigPath)
	configBytes, readError := readConfigBytes(configPath)
	if readError != nil {
		return proxy.PublicCapabilityCatalog{}, fmt.Errorf("%w: path=%s: %v", errSiteRenderFailed, configPath, readError)
	}
	configReader := viper.New()
	configReader.SetConfigType(configFileType)
	if readConfigError := configReader.ReadConfig(bytes.NewReader(configBytes)); readConfigError != nil {
		return proxy.PublicCapabilityCatalog{}, fmt.Errorf("%w: path=%s: %v", errSiteRenderFailed, configPath, readConfigError)
	}
	serverReader := configReader.Sub("server")
	if serverReader == nil {
		return proxy.PublicCapabilityCatalog{}, fmt.Errorf("%w: path=%s field=server", errSiteRenderFailed, configPath)
	}
	providersReader := configReader.Sub("providers")
	if providersReader == nil {
		return proxy.PublicCapabilityCatalog{}, fmt.Errorf("%w: path=%s field=providers", errSiteRenderFailed, configPath)
	}
	var serverConfig serverConfiguration
	if unmarshalError := serverReader.UnmarshalExact(&serverConfig); unmarshalError != nil {
		return proxy.PublicCapabilityCatalog{}, fmt.Errorf("%w: path=%s field=server: %v", errSiteRenderFailed, configPath, unmarshalError)
	}
	var providersConfig providersConfiguration
	if unmarshalError := providersReader.UnmarshalExact(&providersConfig); unmarshalError != nil {
		return proxy.PublicCapabilityCatalog{}, fmt.Errorf("%w: path=%s field=providers: %v", errSiteRenderFailed, configPath, unmarshalError)
	}
	maxRequestTimeoutSeconds, timeoutError := configuredPositiveInteger(serverConfig.MaxRequestTimeoutSeconds, proxy.DefaultMaxRequestTimeoutSeconds, "server.max_request_timeout_seconds")
	if timeoutError != nil {
		return proxy.PublicCapabilityCatalog{}, fmt.Errorf("%w: path=%s: %v", errSiteRenderFailed, configPath, timeoutError)
	}
	capabilityCatalog, catalogError := proxy.NewPublicCapabilityCatalog(proxy.Configuration{
		ProviderModels:           providersConfig.providerModelCatalogs(),
		MaxPromptBytes:           serverConfig.MaxPromptBytes,
		MaxInputAudioBytes:       serverConfig.MaxInputAudioBytes,
		MaxRequestTimeoutSeconds: maxRequestTimeoutSeconds,
	})
	if catalogError != nil {
		return proxy.PublicCapabilityCatalog{}, fmt.Errorf("%w: path=%s: %v", errSiteRenderFailed, configPath, catalogError)
	}
	return capabilityCatalog, nil
}

func renderSiteArtifact(sourceDirectory string, outputDirectory string, configURL siteConfigURL, capabilityCatalog proxy.PublicCapabilityCatalog) error {
	siteSourceDirectory := strings.TrimSpace(sourceDirectory)
	if siteSourceDirectory == constants.EmptyString {
		siteSourceDirectory = defaultSiteSourceDirectory
	}
	siteOutputDirectory := strings.TrimSpace(outputDirectory)
	if siteOutputDirectory == constants.EmptyString {
		return fmt.Errorf("%w: output directory is required", errSiteRenderFailed)
	}
	sourceInfo, sourceError := siteStat(siteSourceDirectory)
	if sourceError != nil {
		return fmt.Errorf("%w: source=%s: %v", errSiteRenderFailed, siteSourceDirectory, sourceError)
	}
	if !sourceInfo.IsDir() {
		return fmt.Errorf("%w: source=%s is not a directory", errSiteRenderFailed, siteSourceDirectory)
	}
	if outputInsideSource, pathError := outputDirectoryInsideSource(siteSourceDirectory, siteOutputDirectory); pathError != nil {
		return pathError
	} else if outputInsideSource {
		return fmt.Errorf("%w: output=%s is inside source=%s", errSiteRenderFailed, siteOutputDirectory, siteSourceDirectory)
	}
	if _, statError := siteStat(siteOutputDirectory); statError == nil {
		return fmt.Errorf("%w: output=%s already exists", errSiteRenderFailed, siteOutputDirectory)
	} else if !os.IsNotExist(statError) {
		return fmt.Errorf("%w: output=%s: %v", errSiteRenderFailed, siteOutputDirectory, statError)
	}
	if copyError := copyStaticSiteSource(siteSourceDirectory, siteOutputDirectory); copyError != nil {
		return copyError
	}
	if writeError := writeRenderedSiteShell(siteOutputDirectory, configURL, capabilityCatalog); writeError != nil {
		return writeError
	}
	return nil
}

func outputDirectoryInsideSource(sourceDirectory string, outputDirectory string) (bool, error) {
	absoluteSourceDirectory, sourcePathError := sitePathAbs(sourceDirectory)
	if sourcePathError != nil {
		return false, fmt.Errorf("%w: source=%s: %v", errSiteRenderFailed, sourceDirectory, sourcePathError)
	}
	absoluteOutputDirectory, outputPathError := sitePathAbs(outputDirectory)
	if outputPathError != nil {
		return false, fmt.Errorf("%w: output=%s: %v", errSiteRenderFailed, outputDirectory, outputPathError)
	}
	relativeOutputDirectory, relativePathError := sitePathRel(absoluteSourceDirectory, absoluteOutputDirectory)
	if relativePathError != nil {
		return false, fmt.Errorf("%w: output=%s source=%s: %v", errSiteRenderFailed, outputDirectory, sourceDirectory, relativePathError)
	}
	return relativeOutputDirectory == "." ||
		(relativeOutputDirectory != ".." &&
			!strings.HasPrefix(relativeOutputDirectory, ".."+string(os.PathSeparator)) &&
			!filepath.IsAbs(relativeOutputDirectory)), nil
}

func copyStaticSiteSource(sourceDirectory string, outputDirectory string) error {
	if copyError := siteCopyFS(outputDirectory, os.DirFS(sourceDirectory)); copyError != nil {
		return fmt.Errorf("%w: output=%s source=%s: %v", errSiteRenderFailed, outputDirectory, sourceDirectory, copyError)
	}
	return nil
}

func writeRenderedSiteShell(outputDirectory string, configURL siteConfigURL, capabilityCatalog proxy.PublicCapabilityCatalog) error {
	for _, staticConfigFile := range []string{proxy.ManagementConfigUIFileName, siteLegacyRuntimeConfig} {
		if removeError := removeCopiedStaticConfig(outputDirectory, staticConfigFile); removeError != nil {
			return removeError
		}
	}
	if indexError := writeRenderedManagementIndex(outputDirectory, configURL); indexError != nil {
		return indexError
	}
	if catalogError := writeRenderedCapabilityCatalog(outputDirectory, capabilityCatalog); catalogError != nil {
		return catalogError
	}
	if _, statError := siteStat(filepath.Join(outputDirectory, siteCNAMEFileName)); statError != nil {
		return fmt.Errorf("%w: output=%s: %v", errSiteRenderFailed, filepath.Join(outputDirectory, siteCNAMEFileName), statError)
	}
	return nil
}

func removeCopiedStaticConfig(outputDirectory string, fileName string) error {
	outputPath := filepath.Join(outputDirectory, fileName)
	if removeError := siteRemove(outputPath); removeError != nil && !os.IsNotExist(removeError) {
		return fmt.Errorf("%w: output=%s: %v", errSiteRenderFailed, outputPath, removeError)
	}
	return nil
}

func writeRenderedManagementIndex(outputDirectory string, configURL siteConfigURL) error {
	outputPath := filepath.Join(outputDirectory, siteApplicationDirectory, siteIndexFileName)
	indexBytes, readError := siteReadFile(outputPath)
	if readError != nil {
		return fmt.Errorf("%w: output=%s: %v", errSiteRenderFailed, outputPath, readError)
	}
	indexHTML := string(indexBytes)
	if strings.Count(indexHTML, siteConfigURLSourceAttribute) != 1 {
		return fmt.Errorf("%w: output=%s must contain exactly one %s", errSiteRenderFailed, outputPath, siteConfigURLSourceAttribute)
	}
	renderedAttribute := siteConfigURLAttribute + `="` + string(configURL) + `"`
	renderedHTML := strings.Replace(indexHTML, siteConfigURLSourceAttribute, renderedAttribute, 1)
	if writeError := siteWriteFile(outputPath, []byte(renderedHTML), renderedSiteFilePerm); writeError != nil {
		return fmt.Errorf("%w: output=%s: %v", errSiteRenderFailed, outputPath, writeError)
	}
	return nil
}

func writeRenderedCapabilityCatalog(outputDirectory string, capabilityCatalog proxy.PublicCapabilityCatalog) error {
	outputPath := filepath.Join(outputDirectory, siteIndexFileName)
	indexBytes, readError := siteReadFile(outputPath)
	if readError != nil {
		return fmt.Errorf("%w: output=%s: %v", errSiteRenderFailed, outputPath, readError)
	}
	indexHTML := string(indexBytes)
	if strings.Count(indexHTML, siteCapabilityCatalogMarker) != 1 {
		return fmt.Errorf("%w: output=%s must contain exactly one %s", errSiteRenderFailed, outputPath, siteCapabilityCatalogMarker)
	}
	catalogHTML, renderError := renderSiteCapabilityCatalog(capabilityCatalog)
	if renderError != nil {
		return renderError
	}
	renderedHTML := strings.Replace(indexHTML, siteCapabilityCatalogMarker, catalogHTML, 1)
	if writeError := siteWriteFile(outputPath, []byte(renderedHTML), renderedSiteFilePerm); writeError != nil {
		return fmt.Errorf("%w: output=%s: %v", errSiteRenderFailed, outputPath, writeError)
	}
	return nil
}

func renderSiteCapabilityCatalog(capabilityCatalog proxy.PublicCapabilityCatalog) (string, error) {
	catalogView, viewError := newSiteCapabilityCatalogView(capabilityCatalog)
	if viewError != nil {
		return constants.EmptyString, fmt.Errorf("%w: capability catalog: %v", errSiteRenderFailed, viewError)
	}
	catalogTemplate, parseError := template.New("capability-catalog").Parse(siteCapabilityCatalogTemplateSource)
	if parseError != nil {
		return constants.EmptyString, fmt.Errorf("%w: capability catalog template: %v", errSiteRenderFailed, parseError)
	}
	var renderedCatalog strings.Builder
	if executeError := siteExecuteCapabilityCatalog(catalogTemplate, &renderedCatalog, catalogView); executeError != nil {
		return constants.EmptyString, fmt.Errorf("%w: capability catalog template: %v", errSiteRenderFailed, executeError)
	}
	return renderedCatalog.String(), nil
}

func newSiteCapabilityCatalogView(capabilityCatalog proxy.PublicCapabilityCatalog) (siteCapabilityCatalogView, error) {
	modelViews := make([]siteCapabilityModelView, 0)
	availableCapabilities := make(map[string]struct{})
	for _, provider := range capabilityCatalog.Providers {
		for _, model := range provider.Models {
			capabilities, capabilitiesError := siteCapabilities(model.Capabilities)
			if capabilitiesError != nil {
				return siteCapabilityCatalogView{}, fmt.Errorf("provider=%s model=%s: %w", provider.Identifier, model.Identifier, capabilitiesError)
			}
			for _, capability := range capabilities {
				availableCapabilities[capability.Identifier] = struct{}{}
			}
			modelViews = append(modelViews, siteCapabilityModelView{
				ProviderIdentifier:    provider.Identifier,
				ProviderLabel:         provider.Label,
				Identifier:            model.Identifier,
				Defaults:              siteDefaults(model.DefaultEndpoints),
				Capabilities:          capabilities,
				CapabilityIdentifiers: strings.Join(model.Capabilities, " "),
				WireContract:          model.WireContract,
				OutputTokenLimit:      model.OutputTokenLimit,
				OutputLimitLabel:      siteOutputLimitLabel(model),
				ReasoningEfforts:      strings.Join(model.ReasoningEfforts, ", "),
				SearchText:            siteModelSearchText(provider.Identifier, provider.Label, model, capabilities),
			})
		}
	}
	capabilityFilters := make([]siteCapabilityDefinition, 0, len(availableCapabilities))
	for _, definition := range siteCapabilityDefinitions {
		if _, available := availableCapabilities[definition.Identifier]; available {
			capabilityFilters = append(capabilityFilters, definition)
		}
	}
	return siteCapabilityCatalogView{
		ProviderCount:     len(capabilityCatalog.Providers),
		ModelCount:        len(modelViews),
		CapabilityCount:   len(capabilityFilters),
		CapabilityFilters: capabilityFilters,
		Models:            modelViews,
		MaxPromptSize:     publicBinarySize(capabilityCatalog.MaxPromptBytes),
		MaxAudioSize:      publicBinarySize(capabilityCatalog.MaxInputAudioBytes),
		MaxRequestTimeout: strconv.Itoa(capabilityCatalog.MaxRequestTimeoutSeconds) + " seconds",
	}, nil
}

func siteModelSearchText(providerIdentifier string, providerLabel string, model proxy.PublicModelCapability, capabilities []siteCapabilityDefinition) string {
	searchValues := []string{
		providerIdentifier,
		providerLabel,
		model.Identifier,
		model.WireContract,
		strconv.Itoa(model.OutputTokenLimit),
		siteOutputLimitLabel(model),
	}
	searchValues = append(searchValues, model.DefaultEndpoints...)
	for _, defaultDefinition := range siteDefaults(model.DefaultEndpoints) {
		searchValues = append(searchValues, defaultDefinition.Label, defaultDefinition.Description)
	}
	searchValues = append(searchValues, model.Capabilities...)
	searchValues = append(searchValues, model.ReasoningEfforts...)
	for _, capability := range capabilities {
		searchValues = append(searchValues, capability.Label)
	}
	return strings.Join(searchValues, " ")
}

var siteCapabilityDefinitions = []siteCapabilityDefinition{
	{Identifier: proxy.PublicModelCapabilityText, Label: "Text generation", ClassName: "capability-badge--primary"},
	{Identifier: proxy.PublicModelCapabilityDictation, Label: "Dictation", ClassName: "capability-badge--info"},
	{Identifier: proxy.PublicModelCapabilityImageInput, Label: "Image input", ClassName: "capability-badge--info"},
	{Identifier: proxy.PublicModelCapabilityAudioInput, Label: "Audio message input", ClassName: "capability-badge--info"},
	{Identifier: proxy.PublicModelCapabilityWebSearch, Label: "Web search", ClassName: "capability-badge--success"},
	{Identifier: proxy.PublicModelCapabilityReasoning, Label: "Reasoning", ClassName: "capability-badge--success"},
}

func siteCapabilities(capabilityIdentifiers []string) ([]siteCapabilityDefinition, error) {
	for _, capabilityIdentifier := range capabilityIdentifiers {
		presentationCount := 0
		for _, definition := range siteCapabilityDefinitions {
			if definition.Identifier == capabilityIdentifier {
				presentationCount++
			}
		}
		if presentationCount != 1 {
			return nil, fmt.Errorf("capability_presentation_invalid: capability=%s presentations=%d", capabilityIdentifier, presentationCount)
		}
	}
	capabilities := make([]siteCapabilityDefinition, 0, len(capabilityIdentifiers))
	for _, definition := range siteCapabilityDefinitions {
		if containsString(capabilityIdentifiers, definition.Identifier) {
			capabilities = append(capabilities, definition)
		}
	}
	return capabilities, nil
}

func siteDefaults(defaultEndpoints []string) []siteDefaultDefinition {
	defaults := make([]siteDefaultDefinition, 0, len(defaultEndpoints))
	if containsString(defaultEndpoints, proxy.PublicModelCapabilityText) {
		defaults = append(defaults, siteDefaultDefinition{
			Label:       "Default for text",
			Description: "This is the provider catalog default for text routing; account settings can select another model.",
		})
	}
	if containsString(defaultEndpoints, proxy.PublicModelCapabilityDictation) {
		defaults = append(defaults, siteDefaultDefinition{
			Label:       "Default for dictation",
			Description: "This is the provider catalog default for dictation routing; account settings can select another model.",
		})
	}
	return defaults
}

func siteOutputLimitLabel(model proxy.PublicModelCapability) string {
	if !containsString(model.Capabilities, proxy.PublicModelCapabilityText) {
		return constants.EmptyString
	}
	if model.OutputTokenLimit == 0 {
		return "Provider-enforced output"
	}
	return strconv.Itoa(model.OutputTokenLimit) + " token output"
}

func containsString(values []string, expectedValue string) bool {
	for _, value := range values {
		if value == expectedValue {
			return true
		}
	}
	return false
}

func publicBinarySize(byteCount int64) string {
	if byteCount%binaryBytesPerMiB == 0 {
		return strconv.FormatInt(byteCount/binaryBytesPerMiB, 10) + " MiB"
	}
	return strconv.FormatInt(byteCount, 10) + " bytes"
}
