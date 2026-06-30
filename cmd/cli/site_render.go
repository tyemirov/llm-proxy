package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/constants"
)

const (
	siteCNAMEFileName          = "CNAME"
	siteIndexFileName          = "index.html"
	siteConfigURLPlaceholder   = "__LLM_PROXY_CONFIG_URL__"
	siteConfigURLAttribute     = "data-config-url"
	siteLegacyRuntimeConfig    = "llm-proxy-config.json"
	defaultSiteSourceDirectory = "site"
)

var errSiteRenderFailed = errors.New("site_render_failed")

var (
	siteCopyFS   = os.CopyFS
	sitePathAbs  = filepath.Abs
	sitePathRel  = filepath.Rel
	siteReadFile = os.ReadFile
	siteRemove   = os.Remove
	siteStat     = os.Stat
)

func renderSiteArtifact(sourceDirectory string, outputDirectory string) error {
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
	if writeError := writeRenderedSiteShell(siteOutputDirectory); writeError != nil {
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

func writeRenderedSiteShell(outputDirectory string) error {
	for _, staticConfigFile := range []string{"config-ui.yaml", siteLegacyRuntimeConfig} {
		if removeError := removeCopiedStaticConfig(outputDirectory, staticConfigFile); removeError != nil {
			return removeError
		}
	}
	if indexError := validateRenderedSiteIndex(outputDirectory); indexError != nil {
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

func validateRenderedSiteIndex(outputDirectory string) error {
	outputPath := filepath.Join(outputDirectory, siteIndexFileName)
	indexBytes, readError := siteReadFile(outputPath)
	if readError != nil {
		return fmt.Errorf("%w: output=%s: %v", errSiteRenderFailed, outputPath, readError)
	}
	indexHTML := string(indexBytes)
	if strings.Contains(indexHTML, siteConfigURLPlaceholder) {
		return fmt.Errorf("%w: output=%s contains retired %s", errSiteRenderFailed, outputPath, siteConfigURLPlaceholder)
	}
	if strings.Contains(indexHTML, siteConfigURLAttribute) {
		return fmt.Errorf("%w: output=%s contains static %s", errSiteRenderFailed, outputPath, siteConfigURLAttribute)
	}
	return nil
}
