package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	dictator "github.com/tyemirov/dictator/sdk/go/dictatorspeechv1"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"google.golang.org/grpc"
)

type hostedDictatorUsageUpstream struct {
	hostedSpeechUpstream
	duration     float64
	failDownload bool
	mode         string
	polls        atomic.Int64
	downloads    atomic.Int64
}

func (upstream *hostedDictatorUsageUpstream) GetSynthesizeSpeechJob(ctx context.Context, request *dictator.GetSynthesizeSpeechJobRequest) (*dictator.GetSynthesizeSpeechJobResponse, error) {
	response, err := upstream.hostedSpeechUpstream.GetSynthesizeSpeechJob(ctx, request)
	response.AudioDurationSeconds = upstream.duration
	if upstream.mode == "failed" {
		response.State = dictator.SynthesisJobState_SYNTHESIS_JOB_STATE_FAILED
	}
	if upstream.mode == "running" && upstream.polls.Add(1) == 1 {
		response.State = dictator.SynthesisJobState_SYNTHESIS_JOB_STATE_RUNNING
		response.AudioDurationSeconds = 100
	}
	return response, err
}
func (upstream *hostedDictatorUsageUpstream) DownloadArtifact(request *dictator.DownloadArtifactRequest, stream grpc.ServerStreamingServer[dictator.DownloadArtifactChunk]) error {
	upstream.downloads.Add(1)
	if upstream.failDownload {
		return fmt.Errorf("controlled artifact loss")
	}
	return upstream.hostedSpeechUpstream.DownloadArtifact(request, stream)
}

func TestHostedDictatorUsagePrecedesArtifactTransfer(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		duration float64
		value    string
		reason   journalUnknownReason
	}{
		{"exact", 1.125, "1.125", ""},
		{"silero", 1.125, "1.125", ""},
		{"running", 1.125, "1.125", ""}, {"failed", 1.125, "1.125", ""}, {"cancelled", 1.125, "1.125", ""}, {"precision", 0.12345678901234567, "0.12345678901234566", ""},
		{"absent", 0, "", journalQuantityNotReported}, {"negative", -1, "", journalQuantityInvalid},
		{"nan", math.NaN(), "", journalQuantityInvalid}, {"infinity", math.Inf(1), "", journalQuantityInvalid},
		{"artifact_loss", 1.125, "1.125", ""}, {"observation_failure", 1.125, "", ""}, {"delivery_failure", 1.125, "", ""},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, _, management, _ := newHostedRatingFixture(t)
			model := ModelNameDictatorQwen3TTS
			if scenario.name == "silero" {
				model = "silero-ru"
			}
			upstream := &hostedDictatorUsageUpstream{mode: scenario.name, duration: scenario.duration, failDownload: scenario.name == "artifact_loss"}
			if scenario.name == "cancelled" {
				upstream.cancelled.Store(true)
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			grpcServer := grpc.NewServer()
			dictator.RegisterVoiceServiceServer(grpcServer, upstream)
			dictator.RegisterArtifactServiceServer(grpcServer, upstream)
			go func() {
				if err := grpcServer.Serve(listener); err != nil {
					t.Error(err)
				}
			}()
			t.Cleanup(grpcServer.Stop)
			server, service, intent := newHostedDictatorUsageFixture(t, database, model, listener.Addr().String())

			hostedSpeechHTTP(t, server, "unfunded-duration", intent, http.StatusPaymentRequired)
			if upstream.submissions.Load() != 0 {
				t.Fatal("unfunded Dictator request dispatched")
			}
			seedHostedFunds(t, database, 500)
			id := hostedSpeechHTTP(t, server, "duration", intent, http.StatusAccepted)["operation_id"].(string)
			assertHostedFundsBalance(t, database, 500, 448)
			failureTable := ""
			switch scenario.name {
			case "observation_failure":
				failureTable = "managed_journal_observation_records"
			case "delivery_failure":
				failureTable = "managed_journal_delivery_records"
			}
			if failureTable != "" {
				if err := database.database.Exec("CREATE TRIGGER reject_duration BEFORE INSERT ON " + failureTable + " BEGIN SELECT RAISE(ABORT, 'controlled duration write failure'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			service.runOperation("duration-worker", id)
			result := hostedMediaWorkerStatus(t, server, id)
			state := MediaOperationStateSucceeded
			if scenario.name == "artifact_loss" || failureTable != "" {
				state = MediaOperationStateUncertain
			}
			if scenario.name == "failed" {
				state = MediaOperationStateFailed
			}
			if scenario.name == "cancelled" {
				state = MediaOperationStateCancelled
			}
			if result["state"] != state {
				t.Fatalf("duration result=%v", result)
			}
			if failureTable != "" {
				if upstream.downloads.Load() != 0 || result["error"].(map[string]any)["code"] != llmproxycontract.ErrorCodeUsageJournalUnavailable {
					t.Fatalf("duration write failure=%v downloads=%d", result, upstream.downloads.Load())
				}
				if err := database.database.Exec("DROP TRIGGER reject_duration").Error; err != nil {
					t.Fatal(err)
				}
			}
			hostedSpeechHTTP(t, server, "duration", intent, http.StatusOK)
			service.runOperation("duplicate-worker", id)
			if upstream.submissions.Load() != 1 {
				t.Fatal("duration replay repeated synthesis")
			}
			pending, err := database.pendingJournalDeliveries(t.Context(), 100)
			if err != nil {
				t.Fatal(err)
			}
			if failureTable != "" {
				if len(pending) != 0 {
					t.Fatal("partial duration evidence")
				}
				assertDictatorFinancialOutcome(t, database, management, scenario.name)
				return
			}
			if len(pending) != 1 {
				t.Fatalf("duration observations=%d", len(pending))
			}
			var quantities []journalQuantity
			if err := json.Unmarshal(pending[0].Quantities, &quantities); err != nil {
				t.Fatal(err)
			}
			if len(quantities) != 1 || quantities[0].Dimension != "output_audio_seconds" || quantities[0].Unit != "second" || quantities[0].Value != scenario.value || quantities[0].UnknownReason != scenario.reason {
				t.Fatalf("duration quantities=%s", pending[0].Quantities)
			}
			if scenario.value != "" && !strings.Contains(string(pending[0].SourceFields), `"path":"audio_duration_seconds","value":"`+scenario.value+`"`) {
				t.Fatalf("duration sources=%s", pending[0].SourceFields)
			}
			usage := journalUsageComplete
			if scenario.reason != "" {
				usage = journalUsageUnknown
			}
			entry := accountConnectionHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/requests", "", http.StatusOK)["requests"].([]any)[0].(map[string]any)
			if entry["usage_state"] != string(usage) {
				t.Fatalf("duration journal=%v", entry)
			}
			assertDictatorFinancialOutcome(t, database, management, scenario.name)
		})
	}
}

func newHostedDictatorUsageFixture(t *testing.T, database *gormManagedTenantDatabase, model, endpoint string) (*httptest.Server, *mediaOperationService, string) {
	t.Helper()
	server, service := newHostedVoiceHTTPFixture(t, database, ProviderNameDictator, model, map[string]string{dictatorAddressField: endpoint, dictatorTokenField: "hosted-duration-secret", dictatorTLSField: "false"})
	settings := hostedDictatorFinancialSettings(t, model)
	service.catalog = settings.catalog
	service.hostedAdmission = settings.mediaAdmission(service.providers)
	if err := database.database.Create(&managedHostedGrantRevisionRecord{GrantID: "grant-voices", Revision: 1, State: hostedGrantActive, ActorUserID: "operator", Reason: "Duration acceptance", CreatedAt: service.store.now()}).Error; err != nil {
		t.Fatal(err)
	}
	imageAdapter := service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageGenerate, "openai", "gpt-image-2")].(*imageGenerationAdapter)
	provider := service.providers.definitions[ProviderNameDictator]
	offering, err := service.catalog.ResolveOffering(ProviderNameDictator, model)
	if err != nil {
		t.Fatal(err)
	}
	service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityAudioSpeechGenerate, ProviderNameDictator, model)] = &accountDictatorAdapter{provider: ProviderNameDictator, model: model, transport: provider.transports[offering.Transport], tenants: imageAdapter.tenants, store: service.store, assets: service.assets}
	engine, _ := dictatorSynthesisEngineForModel(model)
	authority := "platform-voices:" + mediaSHA256Hex([]byte(endpoint+"\x00hosted-duration-secret\x00false"))
	reference, _ := json.Marshal(dictatorVoiceReference{Binding: authority, Engine: engine, Preset: "native-preset"})
	voice, err := persistMediaVoice(database.database, "managed-first", ProviderNameDictator, MediaVoiceProviderRecord{Authority: authority, Provider: ProviderNameDictator, Mode: MediaVoiceModePreset, DisplayName: "Duration", SampleRates: []int{24000}, DefaultSampleRate: 24000, ProviderVoiceReference: string(reference)}, service.store.now())
	if err != nil {
		t.Fatal(err)
	}
	intent := fmt.Sprintf(`{"capability":"audio.speech.generate","provider":"dictator","model":%q,"input":{"text":"A short message.","voice_id":%q},"controls":{"language":"en","text_format":"plain","sample_rate_hz":24000}}`, model, voice.VoiceID)
	return server, service, intent
}

func hostedDictatorFinancialSettings(t *testing.T, model string) *hostedRuntimeSettings {
	t.Helper()
	catalog := internalCanonicalProviderCatalog().ModelCatalog()
	for index := range catalog.Offerings {
		offering := &catalog.Offerings[index]
		if offering.Provider == ProviderNameDictator && offering.Model == model {
			maximum := 10
			offering.Limits = append(offering.Limits, CatalogLimit{ID: "output_audio_seconds", Unit: "seconds", Value: &maximum})
		}
	}
	for index := range catalog.Prices {
		price := &catalog.Prices[index]
		if price.Provider == ProviderNameDictator && price.Model == model && price.Operation == ModelOperationSpeechGeneration {
			*price = CatalogPriceDescriptor{Provider: price.Provider, Model: price.Model, Operation: price.Operation, Available: true, Source: "https://example.com/controlled-duration-prices", LastVerified: "2026-09-23", Rates: []CatalogPriceRate{{Component: "output_audio", Currency: "USD", Rate: "0.04", Unit: "USD/second", Conditions: CatalogPriceConditions{EffectiveFrom: "2026-09-01T00:00:00Z"}}}}
		}
	}
	settings, err := newHostedRuntimeSettings(&HostedConfiguration{Offerings: []HostedOfferingConfiguration{{Provider: ProviderNameDictator, Model: model, Operation: ModelOperationSpeechGeneration, MaximumAttempts: 1}}}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	return settings
}

func assertDictatorFinancialOutcome(t *testing.T, database *gormManagedTenantDatabase, management *httptest.Server, name string) {
	t.Helper()
	for range 2 {
		if err := database.reconcileHostedFunds(t.Context(), time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
	}
	balance := ratingHTTPExchange(t, management, http.MethodGet, fundsBalanceTestPath, "", http.StatusOK)
	charges := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)["charges"].([]any)
	if name == "observation_failure" || name == "delivery_failure" {
		if len(charges) != 0 || balance["pending_cents"] != "52" {
			t.Fatalf("failed evidence financial outcome: charges=%v balance=%v", charges, balance)
		}
		assertHostedFundsBalance(t, database, 500, 448)
		return
	}
	if len(charges) != 1 {
		t.Fatalf("Dictator charges=%v", charges)
	}
	charge := charges[0].(map[string]any)
	summary := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/requests/"+charge["request_id"].(string)+"/charge-summary", "", http.StatusOK)
	var providerCost any = map[string]any{"numerator": "9", "denominator": "200"}
	switch name {
	case "exact", "silero", "running":
		assertHostedFundsBalance(t, database, 495, 495)
		assertFundsCreditRemainder(t, database, "17", "2000")
		if summary["state"] != string(requestChargeRated) || !reflect.DeepEqual(summary["customer_charge"], map[string]any{"numerator": "117", "denominator": "2000"}) {
			t.Fatalf("Dictator settlement=%v", summary)
		}
	case "precision":
		providerCost = map[string]any{"numerator": "6172839450617283", "denominator": "1250000000000000000"}
		assertHostedFundsBalance(t, database, 500, 500)
		assertFundsCreditRemainder(t, database, "80246912858024679", "12500000000000000000")
		if summary["state"] != string(requestChargeRated) || !reflect.DeepEqual(summary["customer_charge"], map[string]any{"numerator": "80246912858024679", "denominator": "12500000000000000000"}) {
			t.Fatalf("Dictator precision lost: %v", summary)
		}
	default:
		assertHostedFundsBalance(t, database, 500, 448)
		assertFundsCreditRemainder(t, database, "0", "1")
		if summary["state"] != string(requestChargeUnresolved) || summary["customer_charge"] != nil || balance["pending_cents"] != "52" {
			t.Fatalf("Dictator unresolved funds lost: summary=%v balance=%v", summary, balance)
		}
		if name == "absent" || name == "negative" || name == "nan" || name == "infinity" {
			providerCost = nil
			if charge["state"] != chargeUsageUnresolved {
				t.Fatalf("unknown duration charge=%v", charge)
			}
		}
	}
	if !reflect.DeepEqual(summary["provider_cost"], providerCost) {
		t.Fatalf("Dictator provider cost=%v", summary)
	}
}
