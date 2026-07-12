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
	siteCNAMEFileName            = "CNAME"
	siteIndexFileName            = "index.html"
	siteConfigURLAttribute       = "data-config-url"
	siteConfigURLSourceAttribute = siteConfigURLAttribute + `="` + proxy.ManagementConfigUIPath + `"`
	siteLegacyRuntimeConfig      = "llm-proxy-config.json"
	renderedSiteFilePerm         = 0o644
	defaultSiteSourceDirectory   = "site"
	defaultSiteConfigURL         = proxy.ManagementConfigUIPath
	secureSiteConfigURLScheme    = "https"
)

var errSiteRenderFailed = errors.New("site_render_failed")

type siteConfigURL string

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

func renderSiteArtifact(sourceDirectory string, outputDirectory string, configURL siteConfigURL) error {
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
	if writeError := writeRenderedSiteShell(siteOutputDirectory, configURL); writeError != nil {
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

func writeRenderedSiteShell(outputDirectory string, configURL siteConfigURL) error {
	for _, staticConfigFile := range []string{proxy.ManagementConfigUIFileName, siteLegacyRuntimeConfig} {
		if removeError := removeCopiedStaticConfig(outputDirectory, staticConfigFile); removeError != nil {
			return removeError
		}
	}
	if indexError := writeRenderedSiteIndex(outputDirectory, configURL); indexError != nil {
		return indexError
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

func writeRenderedSiteIndex(outputDirectory string, configURL siteConfigURL) error {
	outputPath := filepath.Join(outputDirectory, siteIndexFileName)
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
