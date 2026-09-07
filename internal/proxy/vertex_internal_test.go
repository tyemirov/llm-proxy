package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/auth"
	"go.uber.org/zap"
)

type vertexTestTokenProvider struct{ err error }

func (provider vertexTestTokenProvider) Token(context.Context) (*auth.Token, error) {
	if provider.err != nil {
		return nil, provider.err
	}
	return &auth.Token{Value: "test-token", Expiry: time.Now().Add(time.Hour)}, nil
}
func vertexBoundaryProvider() providerDefinition {
	return providerDefinition{tenantIdentifier: "tenant", textAPIKey: "profile", textBaseURL: "https://vertex.example/v1", googleCredentials: map[string]googleCredentialProfile{"profile": {tenantID: "tenant", project: "test-project", location: "global", credentials: auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: vertexTestTokenProvider{}})}}}
}

func TestGeminiCurrentModelsVertexIOFailures(t *testing.T) {
	provider := vertexBoundaryProvider()
	for _, testCase := range []struct {
		name string
		doer geminiEdgeDoer
		want error
	}{
		{"transport", func(*http.Request) (*http.Response, error) { return nil, io.ErrUnexpectedEOF }, io.ErrUnexpectedEOF},
		{"response read", func(*http.Request) (*http.Response, error) {
			return geminiEdgeResponse(200, geminiErrorReader{}, nil), nil
		}, errAssetEdge},
		{"response limit", func(*http.Request) (*http.Response, error) {
			return geminiEdgeResponse(200, strings.NewReader(strings.Repeat("x", vertexResponseLimit+1)), nil), nil
		}, ErrProviderAPI},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := generateVertexContent(context.Background(), testCase.doer, provider, "gemini-3.8-flash", vertexRequest{})
			if !errors.Is(err, testCase.want) {
				t.Fatalf("error=%v", err)
			}
		})
	}
	provider.textBaseURL = ":invalid"
	if _, err := generateVertexContent(context.Background(), http.DefaultClient, provider, "gemini-3.8-flash", vertexRequest{}); err == nil {
		t.Fatal("invalid endpoint accepted")
	}
	provider = vertexBoundaryProvider()
	payload := vertexRequest{Contents: []vertexContent{{Parts: []vertexPart{{Text: strings.Repeat("x", vertexInlineRequestLimit)}}}}}
	if _, err := generateVertexContent(context.Background(), http.DefaultClient, provider, "gemini-3.8-flash", payload); !errors.Is(err, ErrProviderMediaLimit) {
		t.Fatalf("oversize request error=%v", err)
	}
	profile := provider.googleCredentials["profile"]
	profile.credentials = auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: vertexTestTokenProvider{err: errors.New("private credential failure")}})
	provider.googleCredentials["profile"] = profile
	if _, err := generateVertexContent(context.Background(), http.DefaultClient, provider, "gemini-3.8-flash", vertexRequest{}); !errors.Is(err, ErrProviderAPI) || strings.Contains(err.Error(), "private credential failure") {
		t.Fatalf("token failure=%v", err)
	}
}

func TestGeminiCurrentModelsVertexMediaReadAndLimits(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "media")
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	messages := chatMessages{{role: chatRoleUser, content: "inspect", attachments: []messageMedia{{mediaType: messageMediaTypeImage, mimeType: "image/png", sizeBytes: 1, asset: &tenantAssetReader{file: file}}}}}
	route := vertexTextRouteAdapter{}
	router := &providerRouter{geminiClient: newGeminiInteractionsClient(http.DefaultClient)}
	if _, err := route.generateText(context.Background(), router, chatRequestParameters{messages: messages}, zap.NewNop().Sugar()); !errors.Is(err, errAssetStore) {
		t.Fatalf("asset error=%v", err)
	}
	limit := int64(1)
	model := textModelDefinition{mediaLimits: []CatalogMediaLimit{{ID: CatalogMediaLimitIDInlineRequestBytes, MediaType: CatalogMediaLimitTypeAll, Status: CatalogMediaLimitStatusBounded, Value: &limit}}}
	messages[0].attachments[0] = messageMedia{mediaType: messageMediaTypeImage, mimeType: "image/png", sizeBytes: 1, data: []byte("x")}
	if _, err := route.generateText(context.Background(), router, chatRequestParameters{model: model, messages: messages}, zap.NewNop().Sugar()); !errors.Is(err, ErrProviderMediaLimit) {
		t.Fatalf("pre-serialization limit=%v", err)
	}
	limit = 10
	if _, err := route.generateText(context.Background(), router, chatRequestParameters{model: model, messages: messages}, zap.NewNop().Sugar()); !errors.Is(err, ErrProviderMediaLimit) {
		t.Fatalf("serialized envelope limit=%v", err)
	}
	for _, testCase := range []struct {
		name   string
		reader io.Reader
		want   error
	}{
		{"read failure", failedGeminiAudioReader{cause: io.ErrUnexpectedEOF}, io.ErrUnexpectedEOF},
		{"empty", strings.NewReader(""), ErrInvalidAudioInput},
		{"oversize", strings.NewReader(strings.Repeat("x", vertexInlineRequestLimit+1)), ErrProviderMediaLimit},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := transcribeVertexAudio(context.Background(), http.DefaultClient, dictationRequestParameters{fileName: "voice.wav", audioReader: testCase.reader})
			if !errors.Is(err, testCase.want) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestGeminiCurrentModelsVertexCredentialVerification(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"accepted", 200, `{"candidates":[{"content":{"parts":[{"text":"OK"}]},"finishReason":"STOP"}]}`, nil},
		{"quota", 429, "", errProviderKeyVerificationRateLimited},
		{"denied", 403, "", errProviderKeyRejected},
		{"malformed", 200, "{", errProviderKeyVerificationUnavailable},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			doer := geminiEdgeDoer(func(request *http.Request) (*http.Response, error) {
				if request.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("missing OAuth header")
				}
				return geminiEdgeResponse(testCase.status, strings.NewReader(testCase.body), nil), nil
			})
			verifier := newOperationalProviderKeyVerifier(doer, NewEndpoints(), time.Second, zap.NewNop().Sugar())
			err := verifier.verify(context.Background(), vertexBoundaryProvider(), textModelDefinition{providerIdentifier: modelID("gemini-3.8-flash"), wireContract: textWireContractVertexGenerateContent, executionLifecycle: textExecutionLifecycleSynchronousCompletion}, "profile")
			if !errors.Is(err, testCase.want) {
				t.Fatalf("verification error=%v want=%v", err, testCase.want)
			}
		})
	}
}
