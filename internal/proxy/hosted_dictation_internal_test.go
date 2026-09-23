package proxy

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"gorm.io/gorm/clause"
)

func TestHostedDictationIdentityAndDuration(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	grantHostedDictation(t, database)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Header.Get("Authorization") != "Bearer sk-platform-pinned" {
			t.Error("missing pinned platform credential")
		}
		if err := request.ParseMultipartForm(4096); err != nil {
			t.Error(err)
		}
		defer request.MultipartForm.RemoveAll()
		if request.FormValue("model") != "gpt-transcribe" {
			t.Errorf("upstream model=%q", request.FormValue("model"))
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"text":"private transcript","usage":{"type":"duration","seconds":9007199254740993.125}}`)
	}))
	t.Cleanup(upstream.Close)
	root := t.TempDir()
	server := newHostedDictationServer(t, database, upstream.URL, root)
	hostedDictationHTTP(t, server, dictatePath, "", "private audio", http.StatusBadRequest)
	hostedDictationHTTP(t, server, transcriptionsPath, "", "private audio", http.StatusBadRequest)
	if len(read("")["requests"].([]any)) != 0 || calls.Load() != 0 {
		t.Fatal("missing identity admitted dictation")
	}
	first := hostedDictationHTTP(t, server, dictatePath, "audio-request", "private audio", http.StatusOK)
	second := hostedDictationHTTP(t, server, transcriptionsPath, "audio-request", "private audio", http.StatusOK)
	if first != second || !strings.Contains(first, "private transcript") {
		t.Fatalf("replay changed transcript: %s %s", first, second)
	}
	hostedDictationHTTP(t, server, dictatePath, "audio-request", "changed audio", http.StatusConflict)
	status := hostedIdentityStatusHTTP(t, server, "audio-request", http.StatusOK)
	if !strings.Contains(status, "private transcript") {
		t.Fatalf("status lost transcript: %s", status)
	}
	requests := read("")["requests"].([]any)
	if len(requests) != 1 || requests[0].(map[string]any)["operation"] != ModelOperationDictation {
		t.Fatalf("dictation attribution=%v", requests)
	}
	var observation managedJournalObservationRecord
	if err := database.database.First(&observation).Error; err != nil {
		t.Fatal(err)
	}
	var quantities []journalQuantity
	if err := json.Unmarshal(observation.Quantities, &quantities); err != nil {
		t.Fatal(err)
	}
	if len(quantities) != 1 || quantities[0].Dimension != "audio_seconds" || quantities[0].Unit != "second" || quantities[0].Value != "9007199254740993.125" {
		t.Fatalf("duration changed: %s", observation.Quantities)
	}
	server.Close()
	restarted := newHostedDictationServer(t, openJournalTransactionInstance(t, database), upstream.URL, root)
	hostedDictationHTTP(t, restarted, transcriptionsPath, "audio-request", "private audio", http.StatusOK)
	if calls.Load() != 1 {
		t.Fatalf("dictation dispatched %d times", calls.Load())
	}
}

func TestHostedDictationConcurrencyRevocationAndExpiry(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	grantHostedDictation(t, database)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"text":"one transcript","usage":{"type":"duration","seconds":1.25}}`)
	}))
	t.Cleanup(upstream.Close)
	root := t.TempDir()
	first := newHostedDictationServer(t, database, upstream.URL, root)
	var responses *structuredRequestStore
	second := newHostedDictationServer(t, openJournalTransactionInstance(t, database), upstream.URL, root, func(dependencies *hostedTextRequestDependencies) { responses = dependencies.responses })
	t.Cleanup(unblock)
	result := make(chan string, 1)
	go func() {
		defer close(result)
		result <- hostedDictationHTTP(t, first, dictatePath, "concurrent-audio", "private audio", http.StatusOK)
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("dictation did not start")
	}
	pending := hostedDictationHTTP(t, second, transcriptionsPath, "concurrent-audio", "private audio", http.StatusAccepted)
	if !strings.Contains(pending, `"state":"dispatched"`) {
		t.Fatalf("pending result=%s", pending)
	}
	hostedIdentityStatusHTTP(t, second, "concurrent-audio", http.StatusAccepted)
	if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-journal").Update("state", hostedGrantRevoked).Error; err != nil {
		t.Fatal(err)
	}
	unblock()
	if completed := <-result; !strings.Contains(completed, "one transcript") {
		t.Fatalf("completed result=%s", completed)
	}
	hostedDictationHTTP(t, second, transcriptionsPath, "concurrent-audio", "private audio", http.StatusOK)
	hostedDictationHTTP(t, second, dictatePath, "new-audio", "private audio", http.StatusForbidden)
	expiredAt := time.Now().Add(2 * time.Minute)
	responses.now = func() time.Time { return expiredAt }
	hostedDictationHTTP(t, second, transcriptionsPath, "concurrent-audio", "private audio", http.StatusGone)
	hostedIdentityStatusHTTP(t, second, "concurrent-audio", http.StatusGone)
	if calls.Load() != 1 {
		t.Fatalf("dictation dispatched=%d", calls.Load())
	}
}

func TestHostedDictationUsageEvidence(t *testing.T) {
	for _, scenario := range []struct {
		name, usage, value string
		unknown            journalUnknownReason
		quantityCount      int
	}{
		{"zero", `{"type":"duration","seconds":0}`, "0", "", 1},
		{"absent", `null`, "", journalQuantityNotReported, 1},
		{"missing seconds", `{"type":"duration"}`, "", journalQuantityNotReported, 1},
		{"negative", `{"type":"duration","seconds":-1}`, "", journalQuantityInvalid, 1},
		{"string", `{"type":"duration","seconds":"1.25"}`, "", journalQuantityInvalid, 1},
		{"unknown type", `{"type":"bytes","seconds":1}`, "", journalQuantityUnsupported, 1},
		{"tokens", `{"type":"tokens","total_tokens":9007199254740993,"input_tokens":9007199254740990,"output_tokens":3,"input_token_details":{"audio_tokens":9007199254740989,"text_tokens":1}}`, "9007199254740993", "", 5},
		{"contradictory tokens", `{"type":"tokens","total_tokens":5,"input_tokens":4,"output_tokens":3,"input_token_details":{"audio_tokens":3,"text_tokens":1}}`, "", journalQuantityInvalid, 5},
		{"contradictory modalities", `{"type":"tokens","total_tokens":7,"input_tokens":4,"output_tokens":3,"input_token_details":{"audio_tokens":4,"text_tokens":1}}`, "", journalQuantityInvalid, 5},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, _, read := newJournalTransactionFixture(t)
			grantHostedDictation(t, database)
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(writer, `{"text":"private transcript","usage":%s}`, scenario.usage)
			}))
			t.Cleanup(upstream.Close)
			server := newHostedDictationServer(t, database, upstream.URL, t.TempDir())
			hostedDictationHTTP(t, server, dictatePath, "usage-audio", "private audio", http.StatusOK)
			var observation managedJournalObservationRecord
			if err := database.database.First(&observation).Error; err != nil {
				t.Fatal(err)
			}
			var quantities []journalQuantity
			if err := json.Unmarshal(observation.Quantities, &quantities); err != nil {
				t.Fatal(err)
			}
			dimension := "audio_seconds"
			if scenario.quantityCount == 5 {
				dimension = "total_tokens"
			}
			var measured journalQuantity
			for _, quantity := range quantities {
				if quantity.Dimension == dimension {
					measured = quantity
				}
			}
			if len(quantities) != scenario.quantityCount || measured.Dimension != dimension || measured.Value != scenario.value || measured.UnknownReason != scenario.unknown {
				t.Fatalf("quantities=%s", observation.Quantities)
			}
			if strings.Contains(string(observation.SourceFields), "private") {
				t.Fatal("usage evidence retained content")
			}
			request := read("")["requests"].([]any)[0].(map[string]any)
			cases := read("/" + request["id"].(string) + "/reconciliation-cases")["cases"].([]any)
			if (len(cases) == 0) != (scenario.unknown == "") {
				t.Fatalf("usage uncertainty cases=%v", cases)
			}
		})
	}
}

func grantHostedDictation(t *testing.T, database *gormManagedTenantDatabase) {
	t.Helper()
	if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-journal").Update("offerings", []byte(`[{"model":"gpt-4.1","operations":["text"]},{"model":"gpt-transcribe","operations":["dictation"]}]`)).Error; err != nil {
		t.Fatal(err)
	}
}

func TestHostedDictationFailureDoesNotRepeatPaidWork(t *testing.T) {
	for _, failure := range []string{"response lost", "evidence write"} {
		t.Run(failure, func(t *testing.T) {
			database, _, read := newJournalTransactionFixture(t)
			grantHostedDictation(t, database)
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				if failure == "response lost" {
					connection, _, err := writer.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					if err := connection.Close(); err != nil {
						t.Error(err)
					}
					return
				}
				writer.Header().Set("Content-Type", "application/json")
				fmt.Fprint(writer, `{"text":"private transcript","usage":{"type":"duration","seconds":1}}`)
			}))
			t.Cleanup(upstream.Close)
			root := t.TempDir()
			server := newHostedDictationServer(t, database, upstream.URL, root)
			if failure == "evidence write" {
				if err := database.database.Exec("CREATE TRIGGER reject_dictation_usage BEFORE INSERT ON managed_journal_delivery_records BEGIN SELECT RAISE(ABORT, 'controlled evidence failure'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			body := hostedDictationHTTP(t, server, dictatePath, "failed-audio", "private audio", http.StatusBadGateway)
			if strings.Contains(body, "private transcript") || strings.Contains(body, "controlled evidence") {
				t.Fatalf("private failure=%s", body)
			}
			request := read("")["requests"].([]any)[0].(map[string]any)
			if request["state"] != string(journalRequestUncertain) {
				t.Fatalf("request=%v", request)
			}
			if len(read("/" + request["id"].(string) + "/reconciliation-cases")["cases"].([]any)) == 0 {
				t.Fatal("missing reconciliation")
			}
			server.Close()
			restarted := newHostedDictationServer(t, openJournalTransactionInstance(t, database), upstream.URL, root)
			hostedDictationHTTP(t, restarted, transcriptionsPath, "failed-audio", "private audio", http.StatusConflict)
			hostedIdentityStatusHTTP(t, restarted, "failed-audio", http.StatusConflict)
			if calls.Load() != 1 {
				t.Fatalf("paid dispatches=%d", calls.Load())
			}
		})
	}
}

func newHostedDictationServer(t *testing.T, database *gormManagedTenantDatabase, upstreamURL, root string, configure ...func(*hostedTextRequestDependencies)) *httptest.Server {
	return newHostedDictationProviderServer(t, database, upstreamURL, root, "openai", "gpt-transcribe", configure...)
}

func newHostedDictationProviderServer(t *testing.T, database *gormManagedTenantDatabase, upstreamURL, root, providerName, model string, configure ...func(*hostedTextRequestDependencies)) *httptest.Server {
	t.Helper()
	router, service, upstream := newHostedIdentityHTTPHandler(t, database, upstreamURL, root, configure...)
	providers := service.store.routingDefaults
	if providerName != "openai" {
		definition := providers.definitions[providerID(providerName)]
		for identifier, transport := range definition.transports {
			transport.endpointURLOverride = upstreamURL
			definition.transports[identifier] = transport
		}
		providers.definitions[providerID(providerName)] = definition
		credential, err := service.store.providerKeyCipher.encryptConnection(rand.Reader, platformCredentialReference("platform-dictation", 1), providerName, CatalogCredentialAPIKey, "sk-platform-dictation")
		if err != nil {
			t.Fatal(err)
		}
		fields, err := json.Marshal(map[string]string{CatalogCredentialAPIKey: credential})
		if err != nil {
			t.Fatal(err)
		}
		offerings, err := json.Marshal([]map[string]any{{"model": model, "operations": []string{ModelOperationDictation}}})
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC()
		for _, record := range []any{
			&managedPlatformConnectionRecord{ID: "platform-dictation", Provider: providerName, Name: "Dictation", Version: 1, CreatedAt: now, UpdatedAt: now},
			&managedPlatformCredentialRecord{ConnectionID: "platform-dictation", Version: 1, Fields: fields, QualifiedAt: now, CreatedAt: now},
			&managedHostedGrantRecord{ID: "grant-dictation", BillingAccountID: "billing-journal", TenantID: "managed-first", PlatformConnectionID: "platform-dictation", Provider: providerName, CatalogRevision: "journal-catalog", Offerings: offerings, State: hostedGrantActive, Revision: 1, CreatedAt: now, UpdatedAt: now},
			&managedHostedGrantRevisionRecord{GrantID: "grant-dictation", Revision: 1, State: hostedGrantActive, ActorUserID: "operator", Reason: "Dictation fixture", CreatedAt: now},
			&managedHostedTenantAssignmentRecord{TenantID: "managed-first", ProviderID: providerName, GrantID: "grant-dictation", CreatedAt: now},
			&managedProviderProfileRecord{TenantID: "managed-first", ProviderID: providerName, CreatedAt: now, UpdatedAt: now},
		} {
			if err := database.database.Omit(clause.Associations).Create(record).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	policy, err := newRequestTimeoutPolicy(30, 30)
	if err != nil {
		t.Fatal(err)
	}
	auth := newTenantAuthenticator(service.store)
	logger := zap.NewNop().Sugar()
	const audioLimit = 32_000_000
	router.POST(dictatePath, tenantAuthenticatedHandler(auth, logger, requestTimeoutHandler(policy, logger, dictateHandler(upstream, providers, audioLimit, service.store, logger))))
	router.POST(transcriptionsPath, bearerTenantHandler(auth, requestTimeoutHandler(policy, logger, openAITranscriptionHandler(audioLimit, providers, upstream, service.store, logger))))
	server := httptest.NewServer(router)
	server.Client().Transport = hostedIdentityTransport{next: server.Client().Transport}
	t.Cleanup(server.Close)
	return server
}

func hostedDictationHTTP(t *testing.T, server *httptest.Server, path, key, audio string, want int) string {
	return hostedDictationProviderHTTP(t, server, path, key, audio, "openai", "gpt-transcribe", want)
}

func hostedDictationProviderHTTP(t *testing.T, server *httptest.Server, path, key, audio, provider, model string, want int) string {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	field := formFieldAudio
	if path == transcriptionsPath {
		field = "file"
		if err := form.WriteField("model", provider+"/"+model); err != nil {
			t.Fatal(err)
		}
	} else {
		path += "?provider=" + provider + "&model=" + model
	}
	file, err := form.CreateFormFile(field, "audio.wav")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(file, audio); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+path, &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", form.FormDataContentType())
	if key != "" {
		request.Header.Set(llmproxycontract.HeaderIdempotencyKey, key)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != want {
		t.Fatalf("dictation status=%d want=%d body=%s", response.StatusCode, want, data)
	}
	validateHostedIdentityResponse(t, request, response, data)
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("hosted dictation permits caching")
	}
	return string(data)
}

func TestHostedDictationUsesEveryCatalogProvider(t *testing.T) {
	providers := internalManagementProviderRegistry()
	for name, provider := range providers.definitions {
		if !provider.supportsDictation {
			continue
		}
		for _, selected := range provider.transcriptionModels {
			if !slices.Contains(selected.operations, ModelOperationDictation) {
				continue
			}
			model := selected.identifier.string()
			t.Run(name.string()+"/"+model, func(t *testing.T) {
				database, _, read := newJournalTransactionFixture(t)
				grantHostedDictation(t, database)
				transport := provider.transports[selected.transportIdentifier]
				var calls atomic.Int64
				upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					calls.Add(1)
					key := "sk-platform-dictation"
					if name == "openai" {
						key = "sk-platform-pinned"
					}
					if request.Method != http.MethodPost || request.Header.Get(transport.authentication.Header) != transport.authentication.Prefix+key {
						t.Error("incorrect hosted provider dispatch")
					}
					writer.Header().Set("Content-Type", "application/json")
					switch transport.responseCodec {
					case CatalogProtocolMultipartTranscription:
						fmt.Fprint(writer, `{"text":"catalog transcript","usage":{"type":"duration","seconds":1}}`)
					case CatalogProtocolMetaTranscription:
						fmt.Fprint(writer, `{"transcript":"catalog transcript"}`)
					case CatalogProtocolGeminiInteractions:
						fmt.Fprint(writer, `{"id":"private-audio","status":"completed","steps":[{"type":"model_output","content":[{"type":"text","text":"catalog transcript"}]}],"usage":{"total_input_tokens":10,"total_output_tokens":3,"total_tokens":13,"total_cached_tokens":0,"total_thought_tokens":0}}`)
					case CatalogProtocolVertexGenerateContent:
						fmt.Fprint(writer, `{"candidates":[{"content":{"parts":[{"text":"catalog transcript"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":3,"totalTokenCount":13,"cachedContentTokenCount":0,"thoughtsTokenCount":0}}`)
					default:
						t.Errorf("unqualified dictation protocol %s", transport.responseCodec)
					}
				}))
				t.Cleanup(upstream.Close)
				server := newHostedDictationProviderServer(t, database, upstream.URL, t.TempDir(), name.string(), model)
				audio := []byte("RIFF\x00\x00\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00\x80\x3e\x00\x00\x00\x7d\x00\x00\x02\x00\x10\x00data\x02\x00\x00\x00\x00\x00")
				binary.LittleEndian.PutUint32(audio[4:], uint32(len(audio)-8))
				for _, path := range []string{dictatePath, transcriptionsPath} {
					body := hostedDictationProviderHTTP(t, server, path, "catalog-audio", string(audio), name.string(), model, http.StatusOK)
					if !strings.Contains(body, "catalog transcript") {
						t.Fatalf("transcript=%s", body)
					}
				}
				request := read("")["requests"].([]any)[0].(map[string]any)
				if request["provider"] != name.string() || request["model"] != model || request["state"] != string(journalRequestCompleted) || calls.Load() != 1 {
					t.Fatalf("catalog execution=%v calls=%d", request, calls.Load())
				}
				var observation managedJournalObservationRecord
				if err := database.database.First(&observation).Error; err != nil {
					t.Fatal(err)
				}
				if observation.AdapterRevision != transport.responseCodec+":1" {
					t.Fatalf("adapter=%s", observation.AdapterRevision)
				}
				if transport.responseCodec == CatalogProtocolMetaTranscription && !strings.Contains(string(observation.Quantities), string(journalQuantityUnsupported)) {
					t.Fatalf("invented Meta duration=%s", observation.Quantities)
				}
			})
		}
	}
}
