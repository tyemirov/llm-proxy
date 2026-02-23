package proxy_test

import (
	"testing"

	"github.com/temirov/llm-proxy/internal/proxy"
)

const (
	firstResponsesURL  = "https://one.local/v1/responses"
	secondResponsesURL = "https://two.local/v1/responses"
	firstDictateURL    = "https://one.local/v1/audio/transcriptions"
	secondDictateURL   = "https://two.local/v1/audio/transcriptions"
)

// TestEndpointsIsolation verifies that endpoint instances remain independent when used in parallel tests.
func TestEndpointsIsolation(testingInstance *testing.T) {
	testingInstance.Run("first", func(subTest *testing.T) {
		subTest.Parallel()
		endpoints := proxy.NewEndpoints()
		endpoints.SetResponsesURL(firstResponsesURL)
		endpoints.SetTranscriptionsURL(firstDictateURL)
		if endpoints.GetResponsesURL() != firstResponsesURL {
			subTest.Fatalf("responsesURL=%s want=%s", endpoints.GetResponsesURL(), firstResponsesURL)
		}
		if endpoints.GetTranscriptionsURL() != firstDictateURL {
			subTest.Fatalf("transcriptionsURL=%s want=%s", endpoints.GetTranscriptionsURL(), firstDictateURL)
		}
	})
	testingInstance.Run("second", func(subTest *testing.T) {
		subTest.Parallel()
		endpoints := proxy.NewEndpoints()
		endpoints.SetResponsesURL(secondResponsesURL)
		endpoints.SetTranscriptionsURL(secondDictateURL)
		if endpoints.GetResponsesURL() != secondResponsesURL {
			subTest.Fatalf("responsesURL=%s want=%s", endpoints.GetResponsesURL(), secondResponsesURL)
		}
		if endpoints.GetTranscriptionsURL() != secondDictateURL {
			subTest.Fatalf("transcriptionsURL=%s want=%s", endpoints.GetTranscriptionsURL(), secondDictateURL)
		}
	})
}
