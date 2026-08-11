package proxy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

const (
	verificationTransportOpenAI    = "openai"
	verificationTransportChat      = "chat"
	verificationTransportGemini    = "gemini"
	verificationTransportAnthropic = "anthropic"
	testDashScopeWorkspaceURL      = "https://test-workspace.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1"
)

type providerKeyVerificationTransportCase struct {
	provider        string
	model           string
	providerModel   string
	transport       string
	tokenLimitField string
}

type providerKeyVerificationProfile struct {
	Tenant struct {
		Defaults managementTenantDefaultsTestResponse `json:"defaults"`
	} `json:"tenant"`
	Providers []struct {
		ID           string `json:"id"`
		HasKey       bool   `json:"has_key"`
		MaskedKey    string `json:"masked_key"`
		BaseURL      string `json:"base_url"`
		TextModel    string `json:"text_model"`
		SystemPrompt string `json:"system_prompt"`
	} `json:"providers"`
}

type failingVerificationReadCloser struct{}

func (failingVerificationReadCloser) Read([]byte) (int, error) {
	return 0, errors.New("verification response read failed")
}

func (failingVerificationReadCloser) Close() error {
	return nil
}

func TestManagementProviderKeyVerificationUsesEveryCanonicalTransportBeforePersistence(t *testing.T) {
	transportCases := []providerKeyVerificationTransportCase{
		{provider: proxy.ProviderNameOpenAI, model: proxy.ModelNameGPT41, transport: verificationTransportOpenAI},
		{provider: proxy.ProviderNameDeepSeek, model: proxy.ModelNameDeepSeekV4Flash, transport: verificationTransportChat, tokenLimitField: "max_tokens"},
		{provider: proxy.ProviderNameDashScope, model: proxy.ModelNameDashScopeQwenPlus, transport: verificationTransportChat, tokenLimitField: "max_tokens"},
		{provider: proxy.ProviderNameMoonshot, model: proxy.ModelNameMoonshotKimiK26, transport: verificationTransportChat, tokenLimitField: "max_completion_tokens"},
		{provider: proxy.ProviderNameMiniMax, model: proxy.ModelNameMiniMaxM27, providerModel: "MiniMax-M2.7", transport: verificationTransportChat, tokenLimitField: "max_completion_tokens"},
		{provider: proxy.ProviderNameSiliconFlow, model: proxy.ModelNameSiliconFlowDeepSeek, providerModel: "deepseek-ai/DeepSeek-R1", transport: verificationTransportChat, tokenLimitField: "max_tokens"},
		{provider: proxy.ProviderNameZhipu, model: proxy.ModelNameZhipuGLM, transport: verificationTransportChat, tokenLimitField: "max_tokens"},
		{provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini25Flash, transport: verificationTransportGemini},
		{provider: proxy.ProviderNameAnthropic, model: proxy.ModelNameClaudeSonnet46, transport: verificationTransportAnthropic},
		{provider: proxy.ProviderNameMeta, model: proxy.ModelNameMuseSpark11, transport: verificationTransportChat, tokenLimitField: "max_completion_tokens"},
		{provider: proxy.ProviderNameXAI, model: proxy.ModelNameGrok43, transport: verificationTransportChat, tokenLimitField: "max_tokens"},
	}

	for _, transportCase := range transportCases {
		t.Run(transportCase.provider, func(subTest *testing.T) {
			candidateKey := "candidate-" + transportCase.provider
			var upstreamRequests atomic.Int32
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				upstreamRequests.Add(1)
				assertProviderKeyVerificationRequest(subTest, request, transportCase, candidateKey)
				writeProviderKeyVerificationSuccess(responseWriter, transportCase.transport)
			}))
			subTest.Cleanup(upstreamServer.Close)
			observedWorkspaceRequestURL := ""
			previousHTTPClient := proxy.HTTPClient
			if transportCase.provider == proxy.ProviderNameDashScope {
				targetURL, _ := url.Parse(upstreamServer.URL)
				proxy.HTTPClient = coverageHTTPDoer(func(request *http.Request) (*http.Response, error) {
					observedWorkspaceRequestURL = request.URL.String()
					rewrittenRequest := request.Clone(request.Context())
					rewrittenRequest.URL.Scheme = targetURL.Scheme
					rewrittenRequest.URL.Host = targetURL.Host
					rewrittenRequest.Host = ""
					return previousHTTPClient.Do(rewrittenRequest)
				})
				defer func() { proxy.HTTPClient = previousHTTPClient }()
			}

			router := newOperationalProviderKeyVerificationRouter(
				subTest,
				providerKeyVerificationConfiguration(upstreamServer.URL),
				zap.NewNop().Sugar(),
				subTest.TempDir()+"/managed-tenants.db",
				TestTimeout,
			)
			sessionCookie := managementSessionCookie(subTest, "verification-success-"+transportCase.provider)
			tenantID := managementDefaultTenantTestID(subTest, router, sessionCookie)
			response := putManagementProviderKey(
				subTest,
				router,
				sessionCookie,
				tenantID,
				transportCase.provider,
				candidateKey,
				transportCase.model,
				"must-not-enter-verification",
				context.Background(),
			)
			if response.Code != http.StatusOK {
				subTest.Fatalf("status=%d body=%q", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), candidateKey) {
				subTest.Fatalf("candidate key leaked in response body=%q", response.Body.String())
			}
			profile := decodeProviderKeyVerificationProfile(subTest, response.Body.Bytes())
			savedProvider := verificationProfileProvider(subTest, profile, transportCase.provider)
			if !savedProvider.HasKey || savedProvider.MaskedKey == "" || savedProvider.TextModel != transportCase.model || savedProvider.SystemPrompt != "must-not-enter-verification" {
				subTest.Fatalf("saved provider=%+v", savedProvider)
			}
			expectedBaseURL := ""
			if transportCase.provider == proxy.ProviderNameDashScope {
				expectedBaseURL = testDashScopeWorkspaceURL
			}
			if savedProvider.BaseURL != expectedBaseURL {
				subTest.Fatalf("saved provider base URL=%q want=%q", savedProvider.BaseURL, expectedBaseURL)
			}
			if profile.Tenant.Defaults.Provider != transportCase.provider || profile.Tenant.Defaults.Model != transportCase.model {
				subTest.Fatalf("defaults=%+v", profile.Tenant.Defaults)
			}
			if upstreamRequests.Load() != 1 {
				subTest.Fatalf("verification requests=%d want=1", upstreamRequests.Load())
			}
			if transportCase.provider == proxy.ProviderNameDashScope && observedWorkspaceRequestURL != testDashScopeWorkspaceURL+"/chat/completions" {
				subTest.Fatalf("DashScope verification URL=%q", observedWorkspaceRequestURL)
			}
			if usage := requestManagementUsage(subTest, router, sessionCookie, "all"); usage.Totals.Requests != 0 {
				subTest.Fatalf("verification usage requests=%d want=0", usage.Totals.Requests)
			}
		})
	}
}

func TestManagementDashScopeWorkspaceChangeVerifiesRetainedKeyAndRoutesWithStoredURL(t *testing.T) {
	const (
		candidateKey = "candidate-dashscope"
		updatedURL   = "https://updated-workspace.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1"
	)
	requestURLs := make([]string, 0, 3)
	authorizations := make([]string, 0, 3)
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		authorizations = append(authorizations, request.Header.Get("Authorization"))
		responseWriter.Header().Set("Content-Type", "application/json")
		if len(authorizations) < 3 {
			_, _ = responseWriter.Write([]byte(`{"choices":[{}]}`))
			return
		}
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"tenant workspace ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`))
	}))
	defer upstreamServer.Close()

	upstreamTarget, _ := url.Parse(upstreamServer.URL)
	previousHTTPClient := proxy.HTTPClient
	proxy.HTTPClient = coverageHTTPDoer(func(request *http.Request) (*http.Response, error) {
		requestURLs = append(requestURLs, request.URL.String())
		rewrittenRequest := request.Clone(request.Context())
		rewrittenRequest.URL.Scheme = upstreamTarget.Scheme
		rewrittenRequest.URL.Host = upstreamTarget.Host
		rewrittenRequest.Host = ""
		return previousHTTPClient.Do(rewrittenRequest)
	})
	defer func() { proxy.HTTPClient = previousHTTPClient }()

	router := newOperationalProviderKeyVerificationRouter(
		t,
		providerKeyVerificationConfiguration(upstreamServer.URL),
		zap.NewNop().Sugar(),
		t.TempDir()+"/managed-tenants.db",
		TestTimeout,
	)
	sessionCookie := managementSessionCookie(t, "dashscope-workspace-change")
	tenantID := managementDefaultTenantTestID(t, router, sessionCookie)
	firstResponse := putManagementProviderKeyWithBaseURL(t, router, sessionCookie, tenantID, proxy.ProviderNameDashScope, candidateKey, testDashScopeWorkspaceURL, proxy.ModelNameDashScopeQwenPlus, "", context.Background())
	if firstResponse.Code != http.StatusOK {
		t.Fatalf("first save status=%d body=%q", firstResponse.Code, firstResponse.Body.String())
	}
	updatedResponse := putManagementProviderKeyWithBaseURL(t, router, sessionCookie, tenantID, proxy.ProviderNameDashScope, "", updatedURL, proxy.ModelNameDashScopeQwenPlus, "", context.Background())
	if updatedResponse.Code != http.StatusOK {
		t.Fatalf("updated save status=%d body=%q", updatedResponse.Code, updatedResponse.Body.String())
	}
	updatedProfile := decodeProviderKeyVerificationProfile(t, updatedResponse.Body.Bytes())
	if savedProvider := verificationProfileProvider(t, updatedProfile, proxy.ProviderNameDashScope); savedProvider.BaseURL != updatedURL || !savedProvider.HasKey {
		t.Fatalf("updated provider=%+v", savedProvider)
	}

	secret := generateManagementTenantSecret(t, router, sessionCookie, tenantID)
	proxyRequest := httptest.NewRequest(http.MethodGet, "/?key="+url.QueryEscape(secret)+"&provider=dashscope&model="+proxy.ModelNameDashScopeQwenPlus+"&prompt=hello", nil)
	proxyResponse := httptest.NewRecorder()
	router.ServeHTTP(proxyResponse, proxyRequest)
	if proxyResponse.Code != http.StatusOK || strings.TrimSpace(proxyResponse.Body.String()) != "tenant workspace ok" {
		t.Fatalf("proxy status=%d body=%q", proxyResponse.Code, proxyResponse.Body.String())
	}
	expectedURLs := []string{
		testDashScopeWorkspaceURL + "/chat/completions",
		updatedURL + "/chat/completions",
		updatedURL + "/chat/completions",
	}
	if len(requestURLs) != len(expectedURLs) {
		t.Fatalf("workspace requests=%v", requestURLs)
	}
	for index, expectedURL := range expectedURLs {
		if requestURLs[index] != expectedURL || authorizations[index] != "Bearer "+candidateKey {
			t.Fatalf("request[%d] URL=%q authorization=%q", index, requestURLs[index], authorizations[index])
		}
	}
}

func TestManagementProviderKeyVerificationRejectsSafeFailuresWithoutPersistence(t *testing.T) {
	transportCases := []providerKeyVerificationTransportCase{
		{provider: proxy.ProviderNameOpenAI, model: proxy.ModelNameGPT41, transport: verificationTransportOpenAI},
		{provider: proxy.ProviderNameDeepSeek, model: proxy.ModelNameDeepSeekV4Flash, transport: verificationTransportChat, tokenLimitField: "max_tokens"},
		{provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini25Flash, transport: verificationTransportGemini},
		{provider: proxy.ProviderNameAnthropic, model: proxy.ModelNameClaudeSonnet46, transport: verificationTransportAnthropic},
	}
	failureCases := []struct {
		name                 string
		upstreamStatus       int
		upstreamBody         string
		expectedStatus       int
		expectedBody         string
		waitForContext       bool
		cancelRequest        bool
		requestTimeoutSecond int
	}{
		{name: "credential rejection", upstreamStatus: http.StatusUnauthorized, upstreamBody: `{"private":"credential detail"}`, expectedStatus: http.StatusUnprocessableEntity, expectedBody: "provider_key_rejected", requestTimeoutSecond: TestTimeout},
		{name: "model rejection", upstreamStatus: http.StatusNotFound, upstreamBody: `{"private":"model detail"}`, expectedStatus: http.StatusUnprocessableEntity, expectedBody: "provider_key_rejected", requestTimeoutSecond: TestTimeout},
		{name: "rate limit", upstreamStatus: http.StatusTooManyRequests, upstreamBody: `{"private":"rate detail"}`, expectedStatus: http.StatusTooManyRequests, expectedBody: "provider_key_verification_rate_limited", requestTimeoutSecond: TestTimeout},
		{name: "provider timeout status", upstreamStatus: http.StatusGatewayTimeout, upstreamBody: `{"private":"timeout detail"}`, expectedStatus: http.StatusGatewayTimeout, expectedBody: "provider_key_verification_timed_out", requestTimeoutSecond: TestTimeout},
		{name: "provider outage", upstreamStatus: http.StatusServiceUnavailable, upstreamBody: `{"private":"outage detail"}`, expectedStatus: http.StatusServiceUnavailable, expectedBody: "provider_key_verification_unavailable", requestTimeoutSecond: TestTimeout},
		{name: "malformed success", upstreamStatus: http.StatusOK, upstreamBody: `{`, expectedStatus: http.StatusServiceUnavailable, expectedBody: "provider_key_verification_unavailable", requestTimeoutSecond: TestTimeout},
		{name: "timeout", expectedStatus: http.StatusGatewayTimeout, expectedBody: "provider_key_verification_timed_out", waitForContext: true, requestTimeoutSecond: 1},
		{name: "cancellation", expectedStatus: http.StatusGatewayTimeout, expectedBody: "provider_key_verification_timed_out", waitForContext: true, cancelRequest: true, requestTimeoutSecond: TestTimeout},
	}

	for _, transportCase := range transportCases {
		for _, failureCase := range failureCases {
			t.Run(transportCase.provider+"/"+failureCase.name, func(subTest *testing.T) {
				candidateKey := "rejected-" + transportCase.provider + "-" + strings.ReplaceAll(failureCase.name, " ", "-")
				privateUpstreamBody := failureCase.upstreamBody
				requestStarted := make(chan struct{})
				releaseUpstreamHandler := make(chan struct{})
				var upstreamRequests atomic.Int32
				upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
					upstreamRequests.Add(1)
					if providerKeyFromVerificationRequest(request) != candidateKey {
						subTest.Errorf("verification request used an unexpected credential")
					}
					if failureCase.waitForContext {
						close(requestStarted)
						select {
						case <-request.Context().Done():
						case <-releaseUpstreamHandler:
						}
						return
					}
					responseWriter.Header().Set("Content-Type", "application/json")
					responseWriter.WriteHeader(failureCase.upstreamStatus)
					_, _ = responseWriter.Write([]byte(failureCase.upstreamBody))
				}))
				subTest.Cleanup(func() {
					close(releaseUpstreamHandler)
					upstreamServer.Close()
				})

				observedCore, observedLogs := observer.New(zap.DebugLevel)
				router := newOperationalProviderKeyVerificationRouter(
					subTest,
					providerKeyVerificationConfiguration(upstreamServer.URL),
					zap.New(observedCore).Sugar(),
					subTest.TempDir()+"/managed-tenants.db",
					failureCase.requestTimeoutSecond,
				)
				sessionCookie := managementSessionCookie(subTest, "verification-failure-"+transportCase.provider+"-"+strings.ReplaceAll(failureCase.name, " ", "-"))
				tenantID := managementDefaultTenantTestID(subTest, router, sessionCookie)
				requestContext := context.Background()
				cancelRequest := func() {}
				if failureCase.cancelRequest {
					requestContext, cancelRequest = context.WithCancel(requestContext)
					cancelComplete := make(chan struct{})
					go func() {
						<-requestStarted
						cancelRequest()
						close(cancelComplete)
					}()
					subTest.Cleanup(func() {
						cancelRequest()
						<-cancelComplete
					})
				}
				response := putManagementProviderKey(
					subTest,
					router,
					sessionCookie,
					tenantID,
					transportCase.provider,
					candidateKey,
					transportCase.model,
					"private-system-prompt",
					requestContext,
				)
				if response.Code != failureCase.expectedStatus || strings.TrimSpace(response.Body.String()) != failureCase.expectedBody {
					subTest.Fatalf("status=%d body=%q", response.Code, response.Body.String())
				}
				if strings.Contains(response.Body.String(), candidateKey) || (privateUpstreamBody != "" && strings.Contains(response.Body.String(), privateUpstreamBody)) {
					subTest.Fatalf("unsafe verification response body=%q", response.Body.String())
				}
				profile := requestProviderKeyVerificationProfile(subTest, router, sessionCookie, tenantID)
				rejectedProvider := verificationProfileProvider(subTest, profile, transportCase.provider)
				if rejectedProvider.HasKey || profile.Tenant.Defaults.Provider != "" || profile.Tenant.Defaults.Model != "" {
					subTest.Fatalf("rejected profile provider=%+v defaults=%+v", rejectedProvider, profile.Tenant.Defaults)
				}
				if upstreamRequests.Load() != 1 {
					subTest.Fatalf("verification requests=%d want=1", upstreamRequests.Load())
				}
				if usage := requestManagementUsage(subTest, router, sessionCookie, "all"); usage.Totals.Requests != 0 {
					subTest.Fatalf("verification usage requests=%d want=0", usage.Totals.Requests)
				}
				assertProviderKeyVerificationLogsAreSafe(subTest, observedLogs, candidateKey, privateUpstreamBody)
			})
		}
	}
}

func TestManagementProviderKeyVerificationPreservesVerifiedReplacementAndCoversTransportEdges(t *testing.T) {
	t.Run("rejected replacement", func(subTest *testing.T) {
		const (
			verifiedKey  = "verified-existing-key"
			rejectedKey  = "rejected-replacement-key"
			retainedText = "retained provider prompt"
		)
		var upstreamRequests atomic.Int32
		upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			upstreamRequests.Add(1)
			if providerKeyFromVerificationRequest(request) == rejectedKey {
				http.Error(responseWriter, `{"private":"replacement rejected"}`, http.StatusUnauthorized)
				return
			}
			writeProviderKeyVerificationSuccess(responseWriter, verificationTransportChat)
		}))
		subTest.Cleanup(upstreamServer.Close)

		databasePath := subTest.TempDir() + "/managed-tenants.db"
		router := newOperationalProviderKeyVerificationRouter(
			subTest,
			providerKeyVerificationConfiguration(upstreamServer.URL),
			zap.NewNop().Sugar(),
			databasePath,
			TestTimeout,
		)
		sessionCookie := managementSessionCookie(subTest, "verification-replacement")
		tenantID := managementDefaultTenantTestID(subTest, router, sessionCookie)
		initialResponse := putManagementProviderKey(
			subTest,
			router,
			sessionCookie,
			tenantID,
			proxy.ProviderNameDeepSeek,
			verifiedKey,
			proxy.ModelNameDeepSeekV4Flash,
			"initial provider prompt",
			context.Background(),
		)
		if initialResponse.Code != http.StatusOK {
			subTest.Fatalf("initial status=%d body=%q", initialResponse.Code, initialResponse.Body.String())
		}
		retainedSettingsResponse := putManagementProviderKey(
			subTest,
			router,
			sessionCookie,
			tenantID,
			proxy.ProviderNameDeepSeek,
			"",
			proxy.ModelNameDeepSeekV4Flash,
			retainedText,
			context.Background(),
		)
		if retainedSettingsResponse.Code != http.StatusOK || upstreamRequests.Load() != 1 {
			subTest.Fatalf("retained settings status=%d requests=%d", retainedSettingsResponse.Code, upstreamRequests.Load())
		}

		fixtureDatabase := openManagedFixtureDatabase(subTest, databasePath)
		var beforeReplacement managedProviderKeyFixture
		if queryError := fixtureDatabase.Where("tenant_id = ? AND provider_id = ?", tenantID, proxy.ProviderNameDeepSeek).First(&beforeReplacement).Error; queryError != nil {
			subTest.Fatalf("load provider record before replacement: %v", queryError)
		}
		replacementResponse := putManagementProviderKey(
			subTest,
			router,
			sessionCookie,
			tenantID,
			proxy.ProviderNameDeepSeek,
			rejectedKey,
			proxy.ModelNameDeepSeekV4Pro,
			"must not persist",
			context.Background(),
		)
		if replacementResponse.Code != http.StatusUnprocessableEntity || strings.TrimSpace(replacementResponse.Body.String()) != "provider_key_rejected" {
			subTest.Fatalf("replacement status=%d body=%q", replacementResponse.Code, replacementResponse.Body.String())
		}
		var afterReplacement managedProviderKeyFixture
		if queryError := fixtureDatabase.Where("tenant_id = ? AND provider_id = ?", tenantID, proxy.ProviderNameDeepSeek).First(&afterReplacement).Error; queryError != nil {
			subTest.Fatalf("load provider record after replacement: %v", queryError)
		}
		if beforeReplacement.EncryptedAPIKey != afterReplacement.EncryptedAPIKey ||
			afterReplacement.TextModel != proxy.ModelNameDeepSeekV4Flash ||
			afterReplacement.SystemPrompt != retainedText ||
			!beforeReplacement.UpdatedAt.Equal(afterReplacement.UpdatedAt) {
			subTest.Fatalf("provider record changed before=%+v after=%+v", beforeReplacement, afterReplacement)
		}
		profile := requestProviderKeyVerificationProfile(subTest, router, sessionCookie, tenantID)
		retainedProvider := verificationProfileProvider(subTest, profile, proxy.ProviderNameDeepSeek)
		if !retainedProvider.HasKey ||
			retainedProvider.TextModel != proxy.ModelNameDeepSeekV4Flash ||
			retainedProvider.SystemPrompt != retainedText ||
			profile.Tenant.Defaults.Provider != proxy.ProviderNameDeepSeek ||
			profile.Tenant.Defaults.Model != proxy.ModelNameDeepSeekV4Flash {
			subTest.Fatalf("retained provider=%+v defaults=%+v", retainedProvider, profile.Tenant.Defaults)
		}
		revealRequest := authenticatedProviderKeyRevealRequest(
			http.MethodPost,
			managementTenantTestPath(tenantID, "/provider-keys/deepseek/reveal"),
			sessionCookie,
			"http://localhost:8080",
		)
		revealResponse := httptest.NewRecorder()
		router.ServeHTTP(revealResponse, revealRequest)
		if revealResponse.Code != http.StatusOK || !strings.Contains(revealResponse.Body.String(), verifiedKey) || strings.Contains(revealResponse.Body.String(), rejectedKey) {
			subTest.Fatalf("retained reveal status=%d body=%q", revealResponse.Code, revealResponse.Body.String())
		}
		if upstreamRequests.Load() != 2 {
			subTest.Fatalf("verification requests=%d want=2", upstreamRequests.Load())
		}
	})

	edgeCases := []struct {
		name           string
		httpClient     proxy.HTTPDoer
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "transport error",
			expectedStatus: http.StatusServiceUnavailable,
			expectedBody:   "provider_key_verification_unavailable",
			httpClient: coverageHTTPDoer(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("verification transport failed")
			}),
		},
		{
			name:           "response read error",
			expectedStatus: http.StatusServiceUnavailable,
			expectedBody:   "provider_key_verification_unavailable",
			httpClient: coverageHTTPDoer(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       failingVerificationReadCloser{},
				}, nil
			}),
		},
		{
			name:           "oversized response",
			expectedStatus: http.StatusServiceUnavailable,
			expectedBody:   "provider_key_verification_unavailable",
			httpClient: coverageHTTPDoer(func(*http.Request) (*http.Response, error) {
				return coverageHTTPResponse(http.StatusOK, strings.Repeat("x", (1<<20)+1)), nil
			}),
		},
		{
			name:           "rate limit response read error",
			expectedStatus: http.StatusTooManyRequests,
			expectedBody:   "provider_key_verification_rate_limited",
			httpClient: coverageHTTPDoer(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusTooManyRequests,
					Header:     make(http.Header),
					Body:       failingVerificationReadCloser{},
				}, nil
			}),
		},
		{
			name:           "oversized rejected response",
			expectedStatus: http.StatusUnprocessableEntity,
			expectedBody:   "provider_key_rejected",
			httpClient: coverageHTTPDoer(func(*http.Request) (*http.Response, error) {
				return coverageHTTPResponse(http.StatusUnauthorized, strings.Repeat("x", (1<<20)+1)), nil
			}),
		},
	}
	for _, edgeCase := range edgeCases {
		t.Run(edgeCase.name, func(subTest *testing.T) {
			previousHTTPClient := proxy.HTTPClient
			proxy.HTTPClient = edgeCase.httpClient
			router := newOperationalProviderKeyVerificationRouter(
				subTest,
				providerKeyVerificationConfiguration("https://provider.invalid"),
				zap.NewNop().Sugar(),
				subTest.TempDir()+"/managed-tenants.db",
				TestTimeout,
			)
			proxy.HTTPClient = previousHTTPClient
			sessionCookie := managementSessionCookie(subTest, "verification-edge-"+strings.ReplaceAll(edgeCase.name, " ", "-"))
			tenantID := managementDefaultTenantTestID(subTest, router, sessionCookie)
			response := putManagementProviderKey(
				subTest,
				router,
				sessionCookie,
				tenantID,
				proxy.ProviderNameOpenAI,
				"edge-candidate-key",
				proxy.ModelNameGPT41,
				"",
				context.Background(),
			)
			if response.Code != edgeCase.expectedStatus || strings.TrimSpace(response.Body.String()) != edgeCase.expectedBody {
				subTest.Fatalf("status=%d body=%q", response.Code, response.Body.String())
			}
		})
	}

	t.Run("invalid verification URL", func(subTest *testing.T) {
		configuration := providerKeyVerificationConfiguration("https://provider.invalid")
		configuration.GeminiBaseURL = "://"
		router := newOperationalProviderKeyVerificationRouter(
			subTest,
			configuration,
			zap.NewNop().Sugar(),
			subTest.TempDir()+"/managed-tenants.db",
			TestTimeout,
		)
		sessionCookie := managementSessionCookie(subTest, "verification-invalid-url")
		tenantID := managementDefaultTenantTestID(subTest, router, sessionCookie)
		response := putManagementProviderKey(
			subTest,
			router,
			sessionCookie,
			tenantID,
			proxy.ProviderNameGemini,
			"invalid-url-candidate",
			proxy.ModelNameGemini25Flash,
			"",
			context.Background(),
		)
		if response.Code != http.StatusServiceUnavailable || strings.TrimSpace(response.Body.String()) != "provider_key_verification_unavailable" {
			subTest.Fatalf("status=%d body=%q", response.Code, response.Body.String())
		}
	})

	t.Run("missing tenant", func(subTest *testing.T) {
		router := newOperationalProviderKeyVerificationRouter(
			subTest,
			providerKeyVerificationConfiguration("https://provider.invalid"),
			zap.NewNop().Sugar(),
			subTest.TempDir()+"/managed-tenants.db",
			TestTimeout,
		)
		response := putManagementProviderKey(
			subTest,
			router,
			managementSessionCookie(subTest, "verification-missing-tenant"),
			"missing-tenant",
			proxy.ProviderNameOpenAI,
			"must-not-be-verified",
			proxy.ModelNameGPT41,
			"",
			context.Background(),
		)
		if response.Code != http.StatusNotFound {
			subTest.Fatalf("status=%d body=%q", response.Code, response.Body.String())
		}
	})
}

func providerKeyVerificationConfiguration(upstreamURL string) proxy.Configuration {
	return proxy.Configuration{
		OpenAIBaseURL:      upstreamURL,
		DeepSeekBaseURL:    upstreamURL,
		DashScopeBaseURL:   upstreamURL,
		MoonshotBaseURL:    upstreamURL,
		MiniMaxBaseURL:     upstreamURL,
		SiliconFlowBaseURL: upstreamURL,
		ZhipuBaseURL:       upstreamURL,
		GeminiBaseURL:      upstreamURL,
		AnthropicBaseURL:   upstreamURL,
		MetaBaseURL:        upstreamURL,
		XAIBaseURL:         upstreamURL,
	}
}

func newOperationalProviderKeyVerificationRouter(t *testing.T, configuration proxy.Configuration, logger *zap.SugaredLogger, databasePath string, requestTimeoutSeconds int) http.Handler {
	t.Helper()
	configuration = managementConfigurationWithDatabasePath(configuration, databasePath)
	configuration.RequestTimeoutSeconds = requestTimeoutSeconds
	router, buildError := buildRouterWithCatalogs(t, configuration, logger)
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}
	return router
}

func putManagementProviderKey(t *testing.T, router http.Handler, sessionCookie *http.Cookie, tenantID string, provider string, apiKey string, model string, systemPrompt string, requestContext context.Context) *httptest.ResponseRecorder {
	t.Helper()
	baseURL := ""
	if provider == proxy.ProviderNameDashScope {
		baseURL = testDashScopeWorkspaceURL
	}
	return putManagementProviderKeyWithBaseURL(t, router, sessionCookie, tenantID, provider, apiKey, baseURL, model, systemPrompt, requestContext)
}

func putManagementProviderKeyWithBaseURL(t *testing.T, router http.Handler, sessionCookie *http.Cookie, tenantID string, provider string, apiKey string, baseURL string, model string, systemPrompt string, requestContext context.Context) *httptest.ResponseRecorder {
	t.Helper()
	request := authenticatedJSONRequest(
		http.MethodPut,
		managementTenantTestPath(tenantID, "/provider-keys/"+url.PathEscape(provider)),
		managementProviderKeyRequestBodyWithBaseURL(t, apiKey, baseURL, model, systemPrompt),
		sessionCookie,
	).WithContext(requestContext)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func requestProviderKeyVerificationProfile(t *testing.T, router http.Handler, sessionCookie *http.Cookie, tenantID string) providerKeyVerificationProfile {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, managementTenantTestPath(tenantID, ""), nil)
	request.AddCookie(sessionCookie)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("profile status=%d body=%q", response.Code, response.Body.String())
	}
	return decodeProviderKeyVerificationProfile(t, response.Body.Bytes())
}

func decodeProviderKeyVerificationProfile(t *testing.T, responseBytes []byte) providerKeyVerificationProfile {
	t.Helper()
	var profile providerKeyVerificationProfile
	if decodeError := json.Unmarshal(responseBytes, &profile); decodeError != nil {
		t.Fatalf("decode provider verification profile: %v", decodeError)
	}
	return profile
}

func verificationProfileProvider(t *testing.T, profile providerKeyVerificationProfile, provider string) struct {
	ID           string `json:"id"`
	HasKey       bool   `json:"has_key"`
	MaskedKey    string `json:"masked_key"`
	BaseURL      string `json:"base_url"`
	TextModel    string `json:"text_model"`
	SystemPrompt string `json:"system_prompt"`
} {
	t.Helper()
	for _, candidateProvider := range profile.Providers {
		if candidateProvider.ID == provider {
			return candidateProvider
		}
	}
	t.Fatalf("profile missing provider=%s", provider)
	return struct {
		ID           string `json:"id"`
		HasKey       bool   `json:"has_key"`
		MaskedKey    string `json:"masked_key"`
		BaseURL      string `json:"base_url"`
		TextModel    string `json:"text_model"`
		SystemPrompt string `json:"system_prompt"`
	}{}
}

func assertProviderKeyVerificationRequest(t *testing.T, request *http.Request, transportCase providerKeyVerificationTransportCase, candidateKey string) {
	t.Helper()
	requestBody, readError := io.ReadAll(request.Body)
	if readError != nil {
		t.Errorf("read verification request: %v", readError)
		return
	}
	if !bytes.Contains(requestBody, []byte(testProviderKeyVerificationPrompt)) ||
		bytes.Contains(requestBody, []byte(candidateKey)) ||
		bytes.Contains(requestBody, []byte("must-not-enter-verification")) {
		t.Errorf("unsafe verification request body=%s", requestBody)
		return
	}
	var payload map[string]any
	if decodeError := json.Unmarshal(requestBody, &payload); decodeError != nil {
		t.Errorf("decode verification request: %v", decodeError)
		return
	}
	expectedProviderModel := transportCase.providerModel
	if expectedProviderModel == "" {
		expectedProviderModel = transportCase.model
	}
	switch transportCase.transport {
	case verificationTransportOpenAI:
		if request.URL.Path != "/responses" ||
			request.Header.Get("Authorization") != "Bearer "+candidateKey ||
			payload["model"] != expectedProviderModel ||
			payload["background"] != false ||
			payload["store"] != false ||
			payload["max_output_tokens"] != float64(16) {
			t.Errorf("OpenAI verification path=%q headers=%v payload=%v", request.URL.Path, request.Header, payload)
		}
	case verificationTransportChat:
		expectedPath := "/chat/completions"
		if transportCase.provider == proxy.ProviderNameDashScope {
			expectedPath = "/compatible-mode/v1/chat/completions"
		}
		if request.URL.Path != expectedPath ||
			request.Header.Get("Authorization") != "Bearer "+candidateKey ||
			payload["model"] != expectedProviderModel ||
			payload[transportCase.tokenLimitField] != float64(16) {
			t.Errorf("chat verification path=%q headers=%v payload=%v", request.URL.Path, request.Header, payload)
		}
	case verificationTransportGemini:
		generationConfig, generationConfigOK := payload["generation_config"].(map[string]any)
		input, inputOK := payload["input"].([]any)
		if request.URL.Path != testGeminiInteractionsPath ||
			request.Header.Get("x-goog-api-key") != candidateKey ||
			request.Header.Get(testGeminiAPIRevisionHeader) != testGeminiAPIRevisionValue ||
			payload["model"] != expectedProviderModel ||
			payload["background"] != false ||
			payload["store"] != false ||
			!generationConfigOK || generationConfig["max_output_tokens"] != float64(16) ||
			!inputOK || len(input) != 1 || geminiInteractionStepText(t, input[0]) != testProviderKeyVerificationPrompt {
			t.Errorf("Gemini verification path=%q headers=%v payload=%v", request.URL.Path, request.Header, payload)
		}
	case verificationTransportAnthropic:
		if request.URL.Path != "/v1/messages" ||
			request.Header.Get("x-api-key") != candidateKey ||
			request.Header.Get("anthropic-version") != "2023-06-01" ||
			payload["model"] != expectedProviderModel ||
			payload["max_tokens"] != float64(16) {
			t.Errorf("Anthropic verification path=%q headers=%v payload=%v", request.URL.Path, request.Header, payload)
		}
	}
}

func writeProviderKeyVerificationSuccess(responseWriter http.ResponseWriter, transport string) {
	responseWriter.Header().Set("Content-Type", "application/json")
	responseBody := `{"choices":[{}]}`
	switch transport {
	case verificationTransportOpenAI:
		responseBody = `{"id":"verification-response","status":"queued"}`
	case verificationTransportGemini:
		responseBody = `{"status":"incomplete"}`
	case verificationTransportAnthropic:
		responseBody = `{"id":"verification-message","type":"message","role":"assistant"}`
	}
	_, _ = responseWriter.Write([]byte(responseBody))
}

func providerKeyFromVerificationRequest(request *http.Request) string {
	if apiKey := request.Header.Get("x-goog-api-key"); apiKey != "" {
		return apiKey
	}
	if apiKey := request.Header.Get("x-api-key"); apiKey != "" {
		return apiKey
	}
	return strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
}

func assertProviderKeyVerificationLogsAreSafe(t *testing.T, observedLogs *observer.ObservedLogs, unsafeValues ...string) {
	t.Helper()
	for _, logEntry := range observedLogs.All() {
		renderedEntry := logEntry.Message + fmt.Sprint(logEntry.ContextMap())
		for _, unsafeValue := range unsafeValues {
			if unsafeValue != "" && strings.Contains(renderedEntry, unsafeValue) {
				t.Fatalf("verification log leaked unsafe value")
			}
		}
		if strings.Contains(renderedEntry, testProviderKeyVerificationPrompt) {
			t.Fatalf("verification log leaked probe prompt")
		}
	}
}
