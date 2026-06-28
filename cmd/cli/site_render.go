package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/constants"
	"github.com/tyemirov/llm-proxy/internal/proxy"
)

const (
	siteCNAMEFileName          = "CNAME"
	siteIndexFileName          = "index.html"
	siteConfigURLPlaceholder   = "__LLM_PROXY_CONFIG_URL__"
	renderedSiteFilePerm       = 0o644
	defaultSiteSourceDirectory = "site"
)

var errSiteRenderFailed = errors.New("site_render_failed")

var (
	siteCopyFS    = os.CopyFS
	sitePathAbs   = filepath.Abs
	sitePathRel   = filepath.Rel
	siteReadFile  = os.ReadFile
	siteRemove    = os.Remove
	siteStat      = os.Stat
	siteURLParse  = url.Parse
	siteWriteFile = os.WriteFile
)

func renderSiteArtifact(sourceDirectory string, outputDirectory string, configuration proxy.Configuration) error {
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
	if writeError := writeRenderedSiteConfig(siteOutputDirectory, configuration.Management); writeError != nil {
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

func writeRenderedSiteConfig(outputDirectory string, configuration proxy.ManagementConfiguration) error {
	siteDomain, domainError := managementSiteDomain(configuration.PublicOrigin)
	if domainError != nil {
		return domainError
	}
	if removeError := removeCopiedStaticConfig(outputDirectory, proxy.ManagementConfigUIFileName); removeError != nil {
		return removeError
	}
	if removeError := removeCopiedStaticConfig(outputDirectory, "llm-proxy-config.json"); removeError != nil {
		return removeError
	}
	if indexError := writeRenderedSiteIndex(outputDirectory, proxy.ManagementConfigUIURL(configuration.ManagementAPIOrigin)); indexError != nil {
		return indexError
	}
	files := map[string]string{
		siteCNAMEFileName: siteDomain + "\n",
	}
	for fileName, fileContent := range files {
		outputPath := filepath.Join(outputDirectory, fileName)
		if writeError := siteWriteFile(outputPath, []byte(fileContent), renderedSiteFilePerm); writeError != nil {
			return fmt.Errorf("%w: output=%s: %v", errSiteRenderFailed, outputPath, writeError)
		}
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

func writeRenderedSiteIndex(outputDirectory string, configURL string) error {
	outputPath := filepath.Join(outputDirectory, siteIndexFileName)
	indexBytes, readError := siteReadFile(outputPath)
	if readError != nil {
		return fmt.Errorf("%w: output=%s: %v", errSiteRenderFailed, outputPath, readError)
	}
	indexHTML := string(indexBytes)
	if !strings.Contains(indexHTML, siteConfigURLPlaceholder) {
		return fmt.Errorf("%w: output=%s missing %s", errSiteRenderFailed, outputPath, siteConfigURLPlaceholder)
	}
	renderedHTML := strings.ReplaceAll(indexHTML, siteConfigURLPlaceholder, configURL)
	if writeError := siteWriteFile(outputPath, []byte(renderedHTML), renderedSiteFilePerm); writeError != nil {
		return fmt.Errorf("%w: output=%s: %v", errSiteRenderFailed, outputPath, writeError)
	}
	return nil
}

func managementSiteDomain(publicOrigin string) (string, error) {
	parsedOrigin, parseError := siteURLParse(publicOrigin)
	if parseError != nil {
		return constants.EmptyString, fmt.Errorf("%w: management.public_origin=%s: %v", errSiteRenderFailed, publicOrigin, parseError)
	}
	if parsedOrigin.Host == constants.EmptyString {
		return constants.EmptyString, fmt.Errorf("%w: management.public_origin=%s has no host", errSiteRenderFailed, publicOrigin)
	}
	if parsedOrigin.Port() != constants.EmptyString {
		return constants.EmptyString, fmt.Errorf("%w: management.public_origin=%s must not include a port for CNAME", errSiteRenderFailed, publicOrigin)
	}
	return parsedOrigin.Hostname(), nil
}
