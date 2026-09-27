package proxy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
)

var speechSourceFormats = []string{
	"alaw_8000", "mp3_22050_32", "mp3_24000_48", "mp3_44100_32", "mp3_44100_64", "mp3_44100_96", "mp3_44100_128", "mp3_44100_192",
	"opus_48000_32", "opus_48000_64", "opus_48000_96", "opus_48000_128", "opus_48000_192",
	"pcm_8000", "pcm_16000", "pcm_22050", "pcm_24000", "pcm_32000", "pcm_44100", "pcm_48000",
	"wav_8000", "wav_16000", "wav_22050", "wav_24000", "wav_32000", "wav_44100", "wav_48000", "ulaw_8000",
}

const conversionVoicePage = `{"voices":[{"voice_id":"native-voice","name":"Narrator"}],"has_more":false,"total_count":1,"next_page_token":null}`

func TestProviderSpeechConversionPreservesAllSourceFormats(t *testing.T) {
	contract, err := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range []string{"elevenlabs", "conversion-fixture"} {
		t.Run(provider, func(t *testing.T) {
			var posts atomic.Int32
			audio := metaTranscriptionWAV(16000, 1)
			client, _, _ := speechFixture(t, provider, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v1/speech-to-speech/native-voice" || r.Header.Get("xi-api-key") != "speech-secret" {
					t.Errorf("unexpected conversion request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(400)
					return
				}
				ordinal := int(posts.Add(1)) - 1
				if r.URL.Query().Get("output_format") != speechSourceFormats[ordinal%len(speechSourceFormats)] || len(r.URL.Query()) != 1 {
					t.Error("conversion output format was not sent exactly")
				}
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				defer r.MultipartForm.RemoveAll()
				file, header, err := r.FormFile("audio")
				if err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				data, _ := io.ReadAll(file)
				_ = file.Close()
				if !bytes.Equal(data, audio) || header.Header.Get("Content-Type") != "audio/wav" || len(r.MultipartForm.Value) != 5 || len(r.MultipartForm.File) != 1 || r.FormValue("file_format") != "other" || r.FormValue("seed") != "4294967295" || r.FormValue("remove_background_noise") != "true" {
					t.Error("conversion lost source bytes or native fields")
				}
				var settings map[string]any
				_ = json.Unmarshal([]byte(r.FormValue("voice_settings")), &settings)
				if len(settings) != 4 || settings["stability"] != 0.3 || settings["similarity_boost"] != 0.6 || settings["style"] != 0.2 || settings["use_speaker_boost"] != false {
					t.Errorf("voice settings=%v", settings)
				}
				model := r.FormValue("model_id")
				if model != "eleven_english_sts_v2" && model != "eleven_multilingual_sts_v2" {
					t.Errorf("native model=%s", model)
				}
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("request-id", "private-request")
				w.Header().Set("history-item-id", "private-history")
				_, _ = w.Write(audio)
			})
			capabilities, err := client.GetMediaCapabilities(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.GetPublicCapabilities(t.Context()); err != nil {
				t.Fatalf("public catalog decoder: %v", err)
			}
			var models []string
			for _, route := range capabilities.Routes {
				if route.Capability != "audio.speech.convert" {
					continue
				}
				if route.Provider != provider || len(route.Controls) != 8 {
					t.Fatalf("route=%+v", route)
				}
				models = append(models, route.Model)
				for _, encoded := range route.Controls {
					var control proxy.CatalogControl
					if err := json.Unmarshal(encoded, &control); err != nil {
						t.Fatal(err)
					}
					if control.ID == "output_format" && !slices.Equal(control.Values, speechSourceFormats) {
						t.Fatalf("formats=%v", control.Values)
					}
				}
			}
			slices.Sort(models)
			if !slices.Equal(models, []string{"eleven_english_sts_v2", "eleven_multilingual_sts_v2"}) {
				t.Fatalf("conversion models=%v", models)
			}
			voice, err := client.GetMediaVoices(t.Context(), llmproxyclient.MediaVoiceQuery{Provider: provider})
			if err != nil || len(voice.Voices) != 1 {
				t.Fatalf("voice=%+v error=%v", voice, err)
			}
			asset, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "audio/wav", Data: audio})
			if err != nil {
				t.Fatal(err)
			}
			for _, model := range []string{"eleven_english_sts_v2", "eleven_multilingual_sts_v2"} {
				for _, format := range speechSourceFormats {
					input := llmproxyclient.MediaOperationInput{Capability: "audio.speech.convert", Provider: provider, Model: model, Input: json.RawMessage(fmt.Sprintf(`{"voice_id":%q,"audio_asset_id":%q}`, voice.Voices[0].VoiceID, asset.AssetID)), Controls: json.RawMessage(fmt.Sprintf(`{"input_format":"other","output_format":%q,"stability":0.3,"similarity_boost":0.6,"style":0.2,"use_speaker_boost":false,"seed":4294967295,"remove_background_noise":true}`, format))}
					encoded, err := json.Marshal(input)
					if err != nil {
						t.Fatal(err)
					}
					request := httptest.NewRequest(http.MethodPost, "/model/v1/operations", bytes.NewReader(encoded))
					request.Header.Set("Authorization", "Bearer tenant-secret")
					request.Header.Set("Content-Type", "application/json")
					request.Header.Set("Idempotency-Key", model+format)
					if err := contract.ValidateRequest("/model/v1/operations", http.MethodPost, request, encoded); err != nil {
						t.Fatal(err)
					}
					ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
					operation, err := client.CreateMediaOperation(ctx, model+format, input)
					if err != nil {
						cancel()
						t.Fatalf("source conversion model %s format %s must execute: %v", model, format, err)
					}
					completed, err := client.WaitMediaOperation(ctx, operation.OperationID, time.Millisecond)
					if err != nil || completed.State != "succeeded" || len(completed.Outputs) != 2 {
						cancel()
						t.Fatalf("conversion=%+v error=%v", completed, err)
					}
					for ordinal, output := range completed.Outputs {
						metadata, err := client.GetAsset(ctx, output.AssetID)
						if err != nil {
							t.Fatal(err)
						}
						data, err := client.DownloadAsset(ctx, metadata)
						if err != nil {
							t.Fatal(err)
						}
						if ordinal == 0 {
							if !bytes.Equal(data, audio) {
								t.Fatal("provider audio bytes changed")
							}
						} else {
							var description llmproxycontract.MediaAudioDescription
							if err := json.Unmarshal(data, &description); err != nil {
								t.Fatal(err)
							}
							parts := strings.Split(format, "_")
							mimeType := map[string]string{"mp3": "audio/mpeg", "opus": "audio/ogg", "wav": "audio/wav", "pcm": "application/octet-stream", "alaw": "application/octet-stream", "ulaw": "application/octet-stream"}[parts[0]]
							if description.OutputFormat != format || description.MIMEType != mimeType || completed.Outputs[0].MIMEType != mimeType || strings.Contains(string(data), "private-") || output.MIMEType != "application/json" {
								t.Fatalf("audio description=%s", data)
							}
							if mimeType == "application/octet-stream" {
								rate, _ := strconv.Atoi(parts[1])
								encoding := map[string]string{"pcm": "s16le", "alaw": "alaw", "ulaw": "mulaw"}[parts[0]]
								if description.RawAudio == nil || description.RawAudio.Encoding != encoding || description.RawAudio.SampleRateHz != rate || description.RawAudio.Channels != 1 {
									t.Fatalf("raw description=%s", data)
								}
							} else if description.RawAudio != nil {
								t.Fatalf("compressed format has raw interpretation: %s", data)
							}
						}
					}
					repeated, err := client.CreateMediaOperation(ctx, model+format, input)
					if err != nil || repeated.OperationID != completed.OperationID {
						t.Fatalf("idempotency result=%+v error=%v", repeated, err)
					}
					cancel()
				}
			}
			if posts.Load() != 56 {
				t.Fatalf("conversion submissions=%d want 56", posts.Load())
			}
		})
	}
}

func speechFixture(t *testing.T, provider string, native http.HandlerFunc, changes ...func(*proxy.Configuration)) (llmproxyclient.Client, *gorm.DB, func() llmproxyclient.Client) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/user/subscription":
			_, _ = io.WriteString(w, elevenQuotaFixture)
		case "/v2/voices":
			_, _ = io.WriteString(w, conversionVoicePage)
		default:
			native(w, r)
		}
	}))
	t.Cleanup(upstream.Close)
	configuration := proxy.Configuration{ProviderCatalog: elevenResourceCatalog(t, provider, upstream.URL), AssetStorePath: t.TempDir(), MaxAssetBytes: 128 << 10, UpstreamCapacity: testfixtures.UpstreamCapacity(4, 100), MediaOperationWorkers: 1, MediaOperationClaimSeconds: 7200, MediaOperationClaimRenewalSeconds: 3600}
	for _, change := range changes {
		change(&configuration)
	}
	databasePath := filepath.Join(t.TempDir(), "speech.sqlite")
	router := newManagementRouterWithDatabasePath(t, configuration, databasePath)
	owner := managementSessionCookie(t, "speech-owner")
	tenantID := managementDefaultTenantTestID(t, router, owner)
	secret := generateManagementTenantSecret(t, router, owner, tenantID)
	connection := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{"name": "Shared speech account", "provider": provider, "fields": map[string]string{"resource_token": "speech-secret"}}, http.StatusCreated)
	accountConnectionExchange(t, router, owner, http.MethodPut, "/tenants/"+tenantID+"/connections/"+provider, map[string]string{"kind": "account_connection", "resource_id": connection["id"].(string)}, http.StatusOK)
	connect := func(handler http.Handler) llmproxyclient.Client {
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: secret})
		if err != nil {
			t.Fatal(err)
		}
		client, err := llmproxyclient.NewClient(config, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	database, err := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return connect(router), database, func() llmproxyclient.Client {
		return connect(newManagementRouterWithDatabasePath(t, configuration, databasePath))
	}
}

func speechConversionInput(t *testing.T, client llmproxyclient.Client) llmproxyclient.MediaOperationInput {
	t.Helper()
	page, err := client.GetMediaVoices(t.Context(), llmproxyclient.MediaVoiceQuery{Provider: "elevenlabs"})
	if err != nil || len(page.Voices) != 1 {
		t.Fatalf("voices=%+v err=%v", page, err)
	}
	asset, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "audio/wav", Data: metaTranscriptionWAV(16000, 1)})
	if err != nil {
		t.Fatal(err)
	}
	return llmproxyclient.MediaOperationInput{Capability: "audio.speech.convert", Provider: "elevenlabs", Model: "eleven_multilingual_sts_v2", Input: json.RawMessage(fmt.Sprintf(`{"voice_id":%q,"audio_asset_id":%q}`, page.Voices[0].VoiceID, asset.AssetID)), Controls: json.RawMessage(`{"input_format":"other","output_format":"mp3_44100_128"}`)}
}

func TestProviderSpeechConversionRawInputAndOptionalSettings(t *testing.T) {
	client, database, _ := speechFixture(t, "elevenlabs", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			return
		}
		defer r.MultipartForm.RemoveAll()
		file, _, err := r.FormFile("audio")
		if err != nil {
			t.Error(err)
			return
		}
		defer file.Close()
		data, _ := io.ReadAll(file)
		if !bytes.Equal(data, []byte{1, 2, 3, 4}) || len(r.MultipartForm.Value) != 2 || r.FormValue("file_format") != "pcm_s16le_16" {
			t.Errorf("raw conversion=%v data=%v", r.MultipartForm.Value, data)
		}
		w.Header().Set("Content-Type", "audio/pcm")
		w.Header().Set("request-id", "private-raw-request")
		_, _ = w.Write(data)
	})
	input := speechConversionInput(t, client)
	asset, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "application/octet-stream", Data: []byte{1, 2, 3, 4}})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]string
	_ = json.Unmarshal(input.Input, &fields)
	fields["audio_asset_id"] = asset.AssetID
	input.Input, _ = json.Marshal(fields)
	input.Controls = json.RawMessage(`{"input_format":"pcm_s16le_16","output_format":"pcm_16000"}`)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	operation, err := client.CreateMediaOperation(ctx, "raw-conversion", input)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.WaitMediaOperation(ctx, operation.OperationID, time.Millisecond)
	if err != nil || result.State != "succeeded" || len(result.Outputs) != 2 || result.Outputs[0].MIMEType != "application/octet-stream" {
		t.Fatalf("raw result=%+v err=%v", result, err)
	}
	metadata, err := client.GetAsset(ctx, result.Outputs[1].AssetID)
	if err != nil {
		t.Fatal(err)
	}
	body, err := client.DownloadAsset(ctx, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"output_format":"pcm_16000","mime_type":"application/octet-stream","raw_audio":{"encoding":"s16le","sample_rate_hz":16000,"channels":1}}` {
		t.Fatalf("raw description=%s", body)
	}
	var retained struct{ ProviderHandle string }
	if err := database.Table("media_operation_records").Where("operation_id = ?", operation.OperationID).First(&retained).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(retained.ProviderHandle, "private-raw-request") {
		t.Fatal("native request evidence lost")
	}
}

func TestProviderSpeechConversionRejectsInvalidInputsBeforeSubmission(t *testing.T) {
	var posts atomic.Int32
	client, database, _ := speechFixture(t, "elevenlabs", func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte{1, 2})
	})
	base := speechConversionInput(t, client)
	for index, controls := range []string{`{}`, `{"input_format":"wrong","output_format":"mp3_44100_128"}`, `{"input_format":"other","output_format":"wrong"}`, `{"input_format":"other","output_format":"mp3_44100_128","stability":-0.01}`, `{"input_format":"other","output_format":"mp3_44100_128","similarity_boost":1.01}`, `{"input_format":"other","output_format":"mp3_44100_128","seed":4294967296}`, `{"input_format":"other","output_format":"mp3_44100_128","seed":-1}`, `{"input_format":"other","output_format":"mp3_44100_128","speed":1}`, `{"input_format":"pcm_s16le_16","output_format":"mp3_44100_128"}`} {
		input := base
		input.Controls = json.RawMessage(controls)
		if _, err := client.CreateMediaOperation(t.Context(), fmt.Sprintf("invalid-conversion-controls-%d", index), input); err == nil {
			t.Fatalf("accepted controls=%s", controls)
		}
	}
	for index, body := range []string{`{}`, `{"voice_id":"native-voice","audio_asset_id":"ast_0123456789abcdef0123456789abcdef"}`, strings.Replace(string(base.Input), `"audio_asset_id":`, `"unknown":`, 1)} {
		input := base
		input.Input = json.RawMessage(body)
		if _, err := client.CreateMediaOperation(t.Context(), fmt.Sprintf("invalid-conversion-input-%d", index), input); err == nil {
			t.Fatalf("accepted input=%s", body)
		}
	}
	for index, mimeType := range []string{"application/json", "application/octet-stream"} {
		asset, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: mimeType, Data: []byte(`123`)})
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]string
		_ = json.Unmarshal(base.Input, &fields)
		fields["audio_asset_id"] = asset.AssetID
		input := base
		input.Input, _ = json.Marshal(fields)
		if index == 1 {
			input.Controls = json.RawMessage(`{"input_format":"pcm_s16le_16","output_format":"pcm_16000"}`)
		}
		if _, err := client.CreateMediaOperation(t.Context(), fmt.Sprintf("invalid-conversion-asset-%d", index), input); err == nil {
			t.Fatal("invalid audio asset accepted")
		}
	}
	var original struct {
		Authority              string
		ProviderVoiceReference string
	}
	if err := database.Table("media_voice_records").First(&original).Error; err != nil {
		t.Fatal(err)
	}
	var missingAsset map[string]string
	_ = json.Unmarshal(base.Input, &missingAsset)
	missingAsset["audio_asset_id"] = "ast_0123456789abcdef0123456789abcdef"
	absent := base
	absent.Input, _ = json.Marshal(missingAsset)
	if _, err := client.CreateMediaOperation(t.Context(), "absent-conversion-asset", absent); err == nil {
		t.Fatal("missing source accepted")
	}
	for index, changes := range []map[string]any{{"authority": "stale"}, {"authority": original.Authority, "provider_voice_reference": "{"}, {"provider_voice_reference": original.ProviderVoiceReference, "provider": "foreign-provider"}} {
		if err := database.Table("media_voice_records").Where("1 = 1").Updates(changes).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := client.CreateMediaOperation(t.Context(), fmt.Sprintf("invalid-conversion-authority-%d", index), base); err == nil {
			t.Fatal("invalid voice authority accepted")
		}
	}
	if posts.Load() != 0 {
		t.Fatalf("invalid input submitted %d requests", posts.Load())
	}
}

func TestProviderSpeechConversionNativeFailuresNeverResubmit(t *testing.T) {
	for _, scenario := range []struct {
		name        string
		status      int
		body        string
		contentType string
		state       string
	}{
		{"rejected", 401, "private provider error", "application/json", "failed"}, {"timeout", 408, "", "audio/mpeg", "uncertain"}, {"server", 503, "", "audio/mpeg", "uncertain"},
		{"empty", 200, "", "audio/mpeg", "uncertain"}, {"wrong content", 200, `{"error":"private"}`, "application/json", "uncertain"},
		{"oversize", 200, strings.Repeat("x", 129<<10), "audio/mpeg", "uncertain"}, {"truncated", -1, "", "audio/mpeg", "uncertain"}, {"disconnected", 0, "", "audio/mpeg", "uncertain"},
		{"odd pcm", 200, "abc", "audio/pcm", "uncertain"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var posts atomic.Int32
			client, database, restart := speechFixture(t, "elevenlabs", func(w http.ResponseWriter, r *http.Request) {
				posts.Add(1)
				w.Header().Set("Content-Type", scenario.contentType)
				if scenario.status == 0 {
					connection, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					_ = connection.Close()
					return
				}
				if scenario.status == -1 {
					w.Header().Set("Content-Length", "100")
					w.WriteHeader(200)
					return
				}
				w.WriteHeader(scenario.status)
				_, _ = io.WriteString(w, scenario.body)
			})
			input := speechConversionInput(t, client)
			if scenario.name == "odd pcm" {
				input.Controls = json.RawMessage(`{"input_format":"other","output_format":"pcm_16000"}`)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			accepted, err := client.CreateMediaOperation(ctx, "native-conversion-failure", input)
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.WaitMediaOperation(ctx, accepted.OperationID, time.Millisecond)
			if err != nil || result.State != scenario.state || len(result.Outputs) != 0 {
				t.Fatalf("failure=%+v error=%v", result, err)
			}
			if err := database.Table("media_operation_records").Where("operation_id = ?", accepted.OperationID).Updates(map[string]any{"public_state": "running", "provider_execution_state": "dispatched", "terminal_at": nil}).Error; err != nil {
				t.Fatal(err)
			}
			result, err = restart().WaitMediaOperation(ctx, accepted.OperationID, time.Millisecond)
			if err != nil || result.State != "uncertain" || posts.Load() != 1 {
				t.Fatalf("recovery=%+v error=%v submissions=%d", result, err, posts.Load())
			}
		})
	}
}

func TestProviderSpeechConversionRetainsUncertainReceiptStorageFailure(t *testing.T) {
	client, database, _ := speechFixture(t, "elevenlabs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte{1, 2})
	})
	input := speechConversionInput(t, client)
	if err := database.Exec(`CREATE TRIGGER reject_speech_receipt BEFORE UPDATE OF provider_handle ON media_operation_records WHEN OLD.provider_handle = '' AND NEW.provider_handle != '' BEGIN SELECT RAISE(FAIL, 'injected receipt failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	operation, err := client.CreateMediaOperation(ctx, "receipt-failure", input)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.WaitMediaOperation(ctx, operation.OperationID, time.Millisecond)
	if err != nil || result.State != "uncertain" || len(result.Outputs) != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestProviderSpeechConversionCatalogRejectsInvalidCompositions(t *testing.T) {
	for name, change := range map[string]func(*proxy.ProviderOffering){
		"wrong codec":      func(o *proxy.ProviderOffering) { o.WireContract = proxy.CatalogProtocolElevenLabsAlignment },
		"missing controls": func(o *proxy.ProviderOffering) { o.Controls = nil },
		"text behavior":    func(o *proxy.ProviderOffering) { o.WebSearch = true },
		"unknown control":  func(o *proxy.ProviderOffering) { o.Controls[0].ID = "unknown" },
		"unknown format":   func(o *proxy.ProviderOffering) { o.Controls[0].Values = []string{"unsupported"} },
		"wrong input kind": func(o *proxy.ProviderOffering) {
			o.Controls[1] = proxy.CatalogControl{ID: "input_format", Kind: "boolean"}
		},
		"unbounded stability": func(o *proxy.ProviderOffering) { value := 2.0; o.Controls[2].Maximum = &value },
		"unbounded seed":      func(o *proxy.ProviderOffering) { value := 4294967296.0; o.Controls[6].Maximum = &value },
		"account noise flag":  func(o *proxy.ProviderOffering) { o.Controls[7].AccountDependent = true },
	} {
		t.Run(name, func(t *testing.T) {
			catalog := testfixtures.ModelCatalog(t)
			for index := range catalog.Offerings {
				if catalog.Offerings[index].Model == "eleven_english_sts_v2" {
					change(&catalog.Offerings[index])
					break
				}
			}
			if _, err := proxy.NewCatalogService(catalog); err == nil {
				t.Fatal("invalid conversion catalog accepted")
			}
		})
	}
}

func TestProviderSpeechConversionCancellationKeepsNativeRequestSingle(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var posts atomic.Int32
	client, _, _ := speechFixture(t, "elevenlabs", func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		close(entered)
		<-release
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte{1, 2})
	})
	input := speechConversionInput(t, client)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	first, err := client.CreateMediaOperation(ctx, "conversion-active", input)
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	t.Cleanup(func() {
		close(release)
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		result, err := client.WaitMediaOperation(cleanup, first.OperationID, time.Millisecond)
		if err != nil || result.State != "succeeded" {
			t.Errorf("released conversion=%+v err=%v", result, err)
		}
	})
	result, err := client.CancelMediaOperation(ctx, first.OperationID)
	if err != nil || result.CancellationState != "unsupported" {
		t.Fatalf("running cancellation=%+v err=%v", result, err)
	}
	second, err := client.CreateMediaOperation(ctx, "conversion-queued", input)
	if err != nil {
		t.Fatal(err)
	}
	result, err = client.CancelMediaOperation(ctx, second.OperationID)
	if err != nil || result.State != "cancelled" || posts.Load() != 1 {
		t.Fatalf("queued cancellation=%+v err=%v posts=%d", result, err, posts.Load())
	}
}

func TestProviderSpeechConversionRejectsChangedDependenciesBeforeDispatch(t *testing.T) {
	for _, scenario := range []string{"binding", "credential", "voice", "metadata", "content"} {
		t.Run(scenario, func(t *testing.T) {
			var posts atomic.Int32
			var root string
			client, database, restart := speechFixture(t, "elevenlabs", func(w http.ResponseWriter, r *http.Request) {
				posts.Add(1)
				w.Header().Set("Content-Type", "audio/mpeg")
				_, _ = w.Write([]byte{1, 2})
			}, func(configuration *proxy.Configuration) { root = configuration.AssetStorePath })
			input := speechConversionInput(t, client)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			operation, err := client.CreateMediaOperation(ctx, "changed-dependency", input)
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.WaitMediaOperation(ctx, operation.OperationID, time.Millisecond)
			if err != nil || result.State != "succeeded" {
				t.Fatalf("initial=%+v error=%v", result, err)
			}
			updates := map[string]any{"public_state": "queued", "provider_execution_state": "not_dispatched", "terminal_at": nil}
			var fields map[string]string
			_ = json.Unmarshal(input.Input, &fields)
			switch scenario {
			case "binding":
				updates["execution_binding"] = "obsolete"
			case "credential":
				updates["credential_reference"] = "obsolete"
			case "voice":
				if err := database.Table("media_voice_records").Where("1 = 1").Update("authority", "obsolete").Error; err != nil {
					t.Fatal(err)
				}
			case "metadata", "content":
				extension := ".json"
				if scenario == "content" {
					extension = ".data"
				}
				if err := os.Remove(filepath.Join(root, fields["audio_asset_id"]+extension)); err != nil {
					t.Fatal(err)
				}
			}
			if err := database.Table("media_operation_records").Where("operation_id = ?", operation.OperationID).Updates(updates).Error; err != nil {
				t.Fatal(err)
			}
			result, err = restart().WaitMediaOperation(ctx, operation.OperationID, time.Millisecond)
			if err != nil || result.State != "failed" || posts.Load() != 1 {
				t.Fatalf("changed dependency=%+v error=%v submissions=%d", result, err, posts.Load())
			}
		})
	}
}
