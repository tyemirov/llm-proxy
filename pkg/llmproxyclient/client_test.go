package llmproxyclient_test

import (
	"net/url"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
)

func TestConfigPostURLShapesAuthenticatedJSONPostURL(testingInstance *testing.T) {
	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL:  "https://proxy.example/review?prompt=old&model=old&max_tokens=9&web_search=true&provider=gemini&keep=1",
		Secret:   "sekret",
		Provider: "deepseek",
		Timeout:  time.Second,
	})
	if configError != nil {
		testingInstance.Fatalf("config error: %v", configError)
	}

	parsedURL, parseError := url.Parse(config.PostURL())
	if parseError != nil {
		testingInstance.Fatalf("parse post url: %v", parseError)
	}
	if parsedURL.Path != "/review" {
		testingInstance.Fatalf("path=%q", parsedURL.Path)
	}
	queryValues := parsedURL.Query()
	if queryValues.Get("key") != "sekret" {
		testingInstance.Fatalf("key=%q", queryValues.Get("key"))
	}
	if queryValues.Get("format") != "text/plain" {
		testingInstance.Fatalf("format=%q", queryValues.Get("format"))
	}
	if queryValues.Get("provider") != "deepseek" {
		testingInstance.Fatalf("provider=%q", queryValues.Get("provider"))
	}
	if queryValues.Get("keep") != "1" {
		testingInstance.Fatalf("keep=%q", queryValues.Get("keep"))
	}
	for _, removedQueryKey := range []string{"prompt", "model", "max_tokens", "web_search"} {
		if queryValues.Has(removedQueryKey) {
			testingInstance.Fatalf("query key %s should have been removed", removedQueryKey)
		}
	}
}
