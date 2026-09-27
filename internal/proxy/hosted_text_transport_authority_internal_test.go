package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"testing/iotest"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type hostedTextRouteProbe struct {
	textRouteAdapter
	probe func(context.Context, chatRequestParameters)
}

type hostedSearchRequestBody struct {
	io.Reader
	failure error
}

func (body hostedSearchRequestBody) Close() error { return body.failure }

func TestHostedTextSearchBodyFailuresPreserveExecutionAuthority(t *testing.T) {
	database, _, management, _ := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 50)
	var calls, probes atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodDelete {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"id":"search-body-result","status":"completed","output_text":"funded result","output":[],"usage":{"input_tokens":1000,"output_tokens":100,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	t.Cleanup(upstream.Close)
	authorization := hostedSearchRatingAuthorization(t)
	configure := func(dependencies *hostedTextRequestDependencies) {
		authorization(dependencies)
		prices := dependencies.authorize
		dependencies.authorize = func(tx *gorm.DB, record managedJournalRequestRecord, intent hostedCompletionIntent) error {
			return newHostedFundsAdmission(func(tx *gorm.DB, request managedJournalRequestRecord) error {
				return prices(tx, request, intent)
			})(tx, record)
		}
	}
	root := t.TempDir()
	router, routes, _ := newHostedIdentityHTTPHandler(t, database, upstream.URL, root, configure)
	provider := routes.providers.definitions[providerID("openai")]
	for name, model := range provider.textModels {
		model.routeAdapter = hostedTextRouteProbe{textRouteAdapter: model.routeAdapter, probe: func(ctx context.Context, request chatRequestParameters) {
			transport := newProviderTransportHTTPDoer(http.DefaultClient, request.provider, request.provider.credentialFor(endpointKindText))
			failure := errors.New("controlled_search_request_body_failure")
			for _, body := range []io.ReadCloser{io.NopCloser(iotest.ErrReader(failure)), hostedSearchRequestBody{Reader: strings.NewReader(`{}`), failure: failure}} {
				probe, err := http.NewRequestWithContext(contextWithHostedProviderRole(ctx, hostedProviderGeneration), http.MethodPost, upstream.URL, body)
				if err != nil {
					t.Error(err)
					return
				}
				response, err := transport.Do(probe)
				if response != nil || !errors.Is(err, failure) || calls.Load() != 0 {
					t.Errorf("body failure escaped paid boundary: response=%v error=%v calls=%d", response, err, calls.Load())
					return
				}
				probes.Add(1)
			}
		}}
		provider.textModels[name] = model
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	server.Client().Transport = hostedIdentityTransport{next: server.Client().Transport}
	request := func(server *httptest.Server) {
		t.Helper()
		message, err := http.NewRequest(http.MethodPost, server.URL+"/?provider=openai&model=gpt-4.1", strings.NewReader(`{"prompt":"funded prompt","web_search":true}`))
		if err != nil {
			t.Fatal(err)
		}
		message.Header.Set("Content-Type", "application/json")
		message.Header.Set(llmproxycontract.HeaderIdempotencyKey, "search-body-failures")
		response, err := server.Client().Do(message)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != http.StatusOK || string(body) != "funded result" {
			t.Fatalf("search request status=%d body=%s error=%v", response.StatusCode, body, err)
		}
		validateHostedIdentityResponse(t, message, response, body)
	}
	request(server)
	server.Close()
	financial := fundsStartupFixture{database: database, management: management, calls: &calls}
	var before map[string]any
	for iteration := range 2 {
		server = newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, database), upstream.URL, root, configure)
		request(server)
		assertHostedFundsBalance(t, database, 50, 50)
		assertFundsCreditRemainder(t, database, "91", "25000")
		current := financial.state(t)
		if calls.Load() != 1 || probes.Load() != 2 || (iteration > 0 && !reflect.DeepEqual(before, current)) {
			t.Fatal("body failure or replay repeated provider work or financial effects")
		}
		before = current
		server.Close()
	}
}

func (adapter hostedTextRouteProbe) generateText(ctx context.Context, router *providerRouter, request chatRequestParameters, logger *zap.SugaredLogger) (textGenerationResult, error) {
	adapter.probe(ctx, request)
	return adapter.textRouteAdapter.generateText(ctx, router, request, logger)
}

func TestHostedTextTransportRejectsUnpermittedMethodsBeforePaidWork(t *testing.T) {
	database, _, management, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	var calls, probes atomic.Int64
	upstream := fundsUpstream(t, &calls)
	root := t.TempDir()
	router, routes, _ := newHostedIdentityHTTPHandler(t, database, upstream.URL, root, fundsDependencies(prices))
	provider := routes.providers.definitions[providerID("openai")]
	for name, model := range provider.textModels {
		model.routeAdapter = hostedTextRouteProbe{textRouteAdapter: model.routeAdapter, probe: func(ctx context.Context, request chatRequestParameters) {
			transport := newProviderTransportHTTPDoer(http.DefaultClient, request.provider, request.provider.credentialFor(endpointKindText))
			for _, scenario := range []struct {
				role   hostedProviderRole
				method string
			}{
				{hostedProviderGeneration, http.MethodGet},
				{hostedProviderObservation, http.MethodPost},
				{hostedProviderObservation, http.MethodGet},
				{hostedProviderStaging, http.MethodGet},
				{hostedProviderAuxiliary, http.MethodPost},
				{hostedProviderMetadata, http.MethodGet},
				{hostedProviderCancellation, http.MethodDelete},
			} {
				probe, err := http.NewRequestWithContext(contextWithHostedProviderRole(ctx, scenario.role), scenario.method, upstream.URL, nil)
				if err != nil {
					t.Error(err)
					return
				}
				response, err := transport.Do(probe)
				if response != nil || !errors.Is(err, errHostedAuthorityDenied) || calls.Load() != 0 {
					t.Errorf("method escaped paid authority: role=%d method=%s response=%v error=%v calls=%d", scenario.role, scenario.method, response, err, calls.Load())
					return
				}
				probes.Add(1)
			}
		}}
		provider.textModels[name] = model
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	server.Client().Transport = hostedIdentityTransport{next: server.Client().Transport}
	hostedIdentityHTTP(t, server, "transport-methods", "funded prompt", http.StatusOK)
	if probes.Load() != 7 || calls.Load() != 1 {
		t.Fatalf("method rejection consumed execution authority: probes=%d calls=%d", probes.Load(), calls.Load())
	}
	server.Close()
	financial := fundsStartupFixture{database: database, management: management, calls: &calls}
	var before map[string]any
	for iteration := range 2 {
		server = newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, database), upstream.URL, root, fundsDependencies(prices))
		hostedIdentityHTTP(t, server, "transport-methods", "funded prompt", http.StatusOK)
		assertHostedFundsBalance(t, database, 5, 5)
		assertFundsCreditRemainder(t, database, "91", "25000")
		current := financial.state(t)
		if calls.Load() != 1 || probes.Load() != 7 || (iteration > 0 && !reflect.DeepEqual(before, current)) {
			t.Fatal("method rejection or replay repeated financial or provider effects")
		}
		before = current
		server.Close()
	}
}

func TestHostedTextAdapterCannotBorrowPaidAuthority(t *testing.T) {
	fixture := newFundsAdmissionFixture(t)
	fixture.generation.Close()
	handler, service, executor := newHostedIdentityHTTPHandler(t, fixture.database, fixture.upstreamURL, fixture.responseRoot, fundsDependencies(fixture.prices))
	var entered atomic.Bool
	var probes atomic.Int64
	provider := service.providers.definitions[providerID("openai")]
	for name, model := range provider.textModels {
		model.routeAdapter = hostedTextRouteProbe{textRouteAdapter: model.routeAdapter, probe: func(ctx context.Context, request chatRequestParameters) {
			if !entered.CompareAndSwap(false, true) {
				return
			}
			identity := ctx.Value(hostedTextIdentityContextKey{}).(hostedTextIdentity)
			tenantRoutes := service.providers.forTenant(identity.tenant)
			unresolved, selected, err := tenantRoutes.resolveTextRequest("openai", request.model.string(), "", "", false)
			if err != nil || unresolved.credentialFor(endpointKindText) != "" {
				t.Errorf("unresolved grant exposed a credential: %v", err)
				return
			}
			text := request
			text.provider, text.model = unresolved, selected
			if _, err := executor.generateText(ctx, text, zap.NewNop().Sugar()); !errors.Is(err, errHostedAuthorityDenied) {
				t.Errorf("unresolved grant bypassed text admission: %v", err)
				return
			}
			probes.Add(1)
			for _, alternative := range request.provider.textModels {
				if alternative.string() == request.model.string() {
					continue
				}
				resolved, found := request.provider.resolvedTransport(alternative.transportIdentifier)
				if !found {
					t.Error("alternative model has no validated transport")
					return
				}
				text.provider, text.model = resolved, alternative
				if _, err := executor.generateText(ctx, text, zap.NewNop().Sugar()); !errors.Is(err, errHostedAuthorityDenied) {
					t.Errorf("another model borrowed text authority: %v", err)
					return
				}
				probes.Add(1)
				break
			}
			unresolved, audioModel, err := tenantRoutes.resolveDictationRequest("openai", "", "", "")
			if err != nil {
				t.Error(err)
				return
			}
			audio := dictationRequestParameters{provider: unresolved, model: audioModel, fileName: "authority.wav", audioReader: strings.NewReader("RIFF\x26\x00\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00\x80\x3e\x00\x00\x00\x7d\x00\x00\x02\x00\x10\x00data\x02\x00\x00\x00\x00\x00")}
			if _, err := executor.transcribeAudio(ctx, audio, zap.NewNop().Sugar()); !errors.Is(err, errHostedAuthorityDenied) {
				t.Errorf("unresolved grant bypassed dictation admission: %v", err)
				return
			}
			probes.Add(1)
			resolved, found := request.provider.resolvedTransport(request.provider.transcriptionModels[audioModel.string()].transportIdentifier)
			if !found {
				t.Error("dictation model has no validated transport")
				return
			}
			audio.provider = resolved
			if _, err := executor.transcribeAudio(ctx, audio, zap.NewNop().Sugar()); !errors.Is(err, errHostedAuthorityDenied) {
				t.Errorf("dictation borrowed text authority: %v", err)
				return
			}
			probes.Add(1)
			if fixture.calls.Load() != 0 {
				t.Error("authority rejection dispatched provider work")
			}
		}}
		provider.textModels[name] = model
	}
	fixture.generation = httptest.NewServer(handler)
	t.Cleanup(fixture.generation.Close)
	fixture.generation.Client().Transport = hostedIdentityTransport{next: fixture.generation.Client().Transport}
	fixture.recoverAdmission(t)
	if probes.Load() != 4 || fixture.calls.Load() != 1 {
		t.Fatalf("authority probes or replay repeated work: probes=%d calls=%d", probes.Load(), fixture.calls.Load())
	}
}
