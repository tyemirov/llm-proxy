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

const siteCapabilityCatalogTemplate = `<div class="catalog-summary" aria-label="Catalog summary">
  <p><strong>{{.ProviderCount}}</strong><span>Text providers</span></p>
  <p><strong>{{.TextModelCount}}</strong><span>Text routes</span></p>
  <p><strong>{{.DictationProviderCount}}</strong><span>Dictation providers</span></p>
  <p><strong>{{.DictationModelCount}}</strong><span>Dictation routes</span></p>
</div>
<div class="catalog-table-wrap" tabindex="0" role="region" aria-label="Provider and model capability matrix">
  <table class="catalog-table">
    <caption>Current text and dictation routes generated from the validated LLM Proxy provider registry.</caption>
    <thead>
      <tr>
        <th scope="col">Provider</th>
        <th scope="col">Text model</th>
        <th scope="col">Route capabilities</th>
        <th scope="col">Output limit</th>
        <th scope="col">Dictation models</th>
      </tr>
    </thead>
    {{range .Providers}}<tbody>
      {{range .TextModels}}<tr>
        <td class="catalog-provider"><strong>{{ProviderLabel .ProviderIdentifier}}</strong><code>{{.ProviderIdentifier}}</code></td>
        <td class="catalog-model"><code>{{.Identifier}}</code>{{if .Default}}<span class="catalog-model__default">Default</span>{{end}}</td>
        <td><div class="catalog-capabilities">
          <span class="capability-badge">{{.WireContract}}</span>
          <span class="capability-badge">{{.ExecutionLifecycle}}</span>
          {{if .WebSearch}}<span class="capability-badge capability-badge--success">Web search</span>{{end}}
          {{range .MediaInputs}}<span class="capability-badge capability-badge--info">{{.}} input</span>{{end}}
          {{if .ReasoningEfforts}}<span class="capability-badge capability-badge--info">Reasoning: {{.ReasoningEfforts}}</span>{{end}}
        </div></td>
        <td>{{if .OutputTokenLimit}}<code>{{.OutputTokenLimit}}</code> tokens{{else}}<span class="catalog-muted">Provider enforced</span>{{end}}</td>
        <td>{{if .DictationModels}}<code>{{.DictationModels}}</code>{{if .DictationDefaultModel}}<span class="catalog-model__default">Default: {{.DictationDefaultModel}}</span>{{end}}{{else}}<span class="catalog-muted">—</span>{{end}}</td>
      </tr>{{end}}
    </tbody>{{end}}
  </table>
</div>
<div class="catalog-limits" aria-label="Proxy request limits">
  <p><strong>{{.MaxPromptSize}}</strong>Maximum JSON request body</p>
  <p><strong>{{.MaxAudioSize}}</strong>Maximum input audio</p>
  <p><strong>{{.MaxRequestTimeout}}</strong>Maximum request work budget</p>
</div>`

var errSiteRenderFailed = errors.New("site_render_failed")

type siteConfigURL string

type siteCapabilityCatalogView struct {
	ProviderCount          int
	TextModelCount         int
	DictationProviderCount int
	DictationModelCount    int
	Providers              []siteCapabilityProviderView
	MaxPromptSize          string
	MaxAudioSize           string
	MaxRequestTimeout      string
	providerLabels         map[string]string
}

type siteCapabilityProviderView struct {
	TextModels []siteCapabilityModelView
}

type siteCapabilityModelView struct {
	ProviderIdentifier    string
	Identifier            string
	Default               bool
	WireContract          string
	ExecutionLifecycle    string
	WebSearch             bool
	OutputTokenLimit      int
	ReasoningEfforts      string
	MediaInputs           []string
	DictationModels       string
	DictationDefaultModel string
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
	catalogView := newSiteCapabilityCatalogView(capabilityCatalog)
	catalogTemplate, parseError := template.New("capability-catalog").Funcs(template.FuncMap{
		"ProviderLabel": catalogView.providerLabel,
	}).Parse(siteCapabilityCatalogTemplateSource)
	if parseError != nil {
		return constants.EmptyString, fmt.Errorf("%w: capability catalog template: %v", errSiteRenderFailed, parseError)
	}
	var renderedCatalog strings.Builder
	if executeError := siteExecuteCapabilityCatalog(catalogTemplate, &renderedCatalog, catalogView); executeError != nil {
		return constants.EmptyString, fmt.Errorf("%w: capability catalog template: %v", errSiteRenderFailed, executeError)
	}
	return renderedCatalog.String(), nil
}

func newSiteCapabilityCatalogView(capabilityCatalog proxy.PublicCapabilityCatalog) siteCapabilityCatalogView {
	providerViews := make([]siteCapabilityProviderView, 0, len(capabilityCatalog.Providers))
	providerLabels := make(map[string]string, len(capabilityCatalog.Providers))
	textModelCount := 0
	dictationProviderCount := 0
	dictationModelCount := 0
	for _, provider := range capabilityCatalog.Providers {
		providerLabels[provider.Identifier] = provider.Label
		textModelCount += len(provider.TextModels)
		if len(provider.DictationModels) != 0 {
			dictationProviderCount++
			dictationModelCount += len(provider.DictationModels)
		}
		textModelViews := make([]siteCapabilityModelView, 0, len(provider.TextModels))
		for _, model := range provider.TextModels {
			textModelViews = append(textModelViews, siteCapabilityModelView{
				ProviderIdentifier:    provider.Identifier,
				Identifier:            model.Identifier,
				Default:               model.Default,
				WireContract:          publicCapabilityLabel(model.WireContract),
				ExecutionLifecycle:    publicCapabilityLabel(model.ExecutionLifecycle),
				WebSearch:             model.WebSearch,
				OutputTokenLimit:      model.OutputTokenLimit,
				ReasoningEfforts:      strings.Join(model.ReasoningEfforts, ", "),
				MediaInputs:           append([]string(nil), model.MediaInputs...),
				DictationModels:       strings.Join(provider.DictationModels, ", "),
				DictationDefaultModel: provider.DictationDefaultModel,
			})
		}
		providerViews = append(providerViews, siteCapabilityProviderView{TextModels: textModelViews})
	}
	return siteCapabilityCatalogView{
		ProviderCount:          len(capabilityCatalog.Providers),
		TextModelCount:         textModelCount,
		DictationProviderCount: dictationProviderCount,
		DictationModelCount:    dictationModelCount,
		Providers:              providerViews,
		MaxPromptSize:          publicBinarySize(capabilityCatalog.MaxPromptBytes),
		MaxAudioSize:           publicBinarySize(capabilityCatalog.MaxInputAudioBytes),
		MaxRequestTimeout:      strconv.Itoa(capabilityCatalog.MaxRequestTimeoutSeconds) + " seconds",
		providerLabels:         providerLabels,
	}
}

func (view siteCapabilityCatalogView) providerLabel(providerIdentifier string) string {
	return view.providerLabels[providerIdentifier]
}

func publicCapabilityLabel(rawValue string) string {
	return strings.ReplaceAll(rawValue, "_", " ")
}

func publicBinarySize(byteCount int64) string {
	if byteCount%binaryBytesPerMiB == 0 {
		return strconv.FormatInt(byteCount/binaryBytesPerMiB, 10) + " MiB"
	}
	return strconv.FormatInt(byteCount, 10) + " bytes"
}
