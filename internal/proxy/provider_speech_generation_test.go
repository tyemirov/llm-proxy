package proxy_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gopkg.in/yaml.v3"
)

func TestProviderSpeechGenerationPreservesSourceControlsAndOwnedContext(t *testing.T) {
	contract, err := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range []string{"elevenlabs", "speech-fixture"} {
		t.Run(provider, func(t *testing.T) {
			var posts atomic.Int32
			audio := []byte{1, 2, 3, 4}
			client, _, _ := speechFixture(t, provider, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/pronunciation-dictionaries/add-from-rules" {
					_, _ = io.WriteString(w, dictionaryNativeJSON)
					return
				}
				if r.Method != http.MethodPost || r.Header.Get("xi-api-key") != "speech-secret" || r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("native speech request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(400)
					return
				}
				ordinal := posts.Add(1)
				var body struct {
					Text                  string              `json:"text"`
					Model                 string              `json:"model_id"`
					Language              string              `json:"language_code"`
					VoiceSettings         map[string]any      `json:"voice_settings"`
					Seed                  uint64              `json:"seed"`
					PreviousText          string              `json:"previous_text"`
					NextText              string              `json:"next_text"`
					PreviousRequests      []string            `json:"previous_request_ids"`
					NextRequests          []string            `json:"next_request_ids"`
					Normalization         string              `json:"apply_text_normalization"`
					LanguageNormalization bool                `json:"apply_language_text_normalization"`
					Dictionaries          []map[string]string `json:"pronunciation_dictionary_locators"`
				}
				decoder := json.NewDecoder(r.Body)
				decoder.DisallowUnknownFields()
				if err := decoder.Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if body.Text != "Hello 世界" && body.Text != "[deliberately slowly] Hello 世界" {
					t.Errorf("text=%s", body.Text)
				}
				if ordinal%2 == 0 {
					expectedText := "Hello 世界"
					if body.Model == "eleven_v3" {
						expectedText = "[deliberately slowly] " + expectedText
						if _, exists := body.VoiceSettings["speed"]; exists {
							t.Error("pacing route sent native speed")
						}
					} else if body.VoiceSettings["speed"] != 0.9 {
						t.Errorf("speed=%v", body.VoiceSettings)
					}
					if body.Text != expectedText || body.VoiceSettings["stability"] != 0.5 || body.VoiceSettings["style"] != 0.2 || body.Seed != 4294967295 || body.PreviousText != "Before." || body.NextText != "After." || body.Normalization != "on" || !body.LanguageNormalization {
						t.Errorf("lost generation controls: %+v", body)
					}
					if body.Model == "eleven_v3" {
						if _, exists := body.VoiceSettings["similarity_boost"]; exists {
							t.Error("unsupported similarity sent")
						}
						if _, exists := body.VoiceSettings["use_speaker_boost"]; exists {
							t.Error("unsupported speaker boost sent")
						}
					} else if body.VoiceSettings["similarity_boost"] != 0.6 || body.VoiceSettings["use_speaker_boost"] != false {
						t.Error("supported voice settings lost")
					}
					if body.Model == "eleven_multilingual_v2" {
						if body.Language != "" {
							t.Error("unsupported language control sent")
						}
					} else if body.Language != "ja" {
						t.Error("language control lost")
					}
					expectedRequest := fmt.Sprintf("private-speech-%d", ordinal-1)
					if len(body.PreviousRequests) != 1 || body.PreviousRequests[0] != expectedRequest || len(body.NextRequests) != 1 || body.NextRequests[0] != expectedRequest {
						t.Errorf("native continuity=%v %v", body.PreviousRequests, body.NextRequests)
					}
					if len(body.Dictionaries) != 1 || body.Dictionaries[0]["pronunciation_dictionary_id"] != "native-dictionary" || body.Dictionaries[0]["version_id"] != "native-version" {
						t.Errorf("dictionary=%v", body.Dictionaries)
					}
					if r.URL.Path != "/v1/text-to-speech/native-voice/with-timestamps" {
						t.Error("missing timestamp path")
					}
					characters := []string{}
					starts := []float64{}
					ends := []float64{}
					for index, character := range []rune(body.Text) {
						characters = append(characters, string(character))
						starts = append(starts, float64(index)/10)
						ends = append(ends, float64(index+1)/10)
					}
					alignment := map[string]any{"characters": characters, "character_start_times_seconds": starts, "character_end_times_seconds": ends}
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("request-id", fmt.Sprintf("private-speech-%d", ordinal))
					_ = json.NewEncoder(w).Encode(map[string]any{"audio_base64": base64.StdEncoding.EncodeToString(audio), "alignment": alignment, "normalized_alignment": alignment})
				} else {
					if r.URL.Path != "/v1/text-to-speech/native-voice" || len(body.VoiceSettings) != 0 || len(body.PreviousRequests) != 0 {
						t.Error("absent controls changed")
					}
					w.Header().Set("Content-Type", "audio/mpeg")
					w.Header().Set("request-id", fmt.Sprintf("private-speech-%d", ordinal))
					_, _ = w.Write(audio)
				}
				if r.URL.Query().Get("output_format") != "mp3_44100_128" {
					t.Error("format lost")
				}
			})
			capabilities, err := client.GetMediaCapabilities(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			var discovered []string
			for _, route := range capabilities.Routes {
				if route.Capability != "audio.speech.generate" || route.Provider != provider {
					continue
				}
				discovered = append(discovered, route.Model)
				for _, encoded := range route.Controls {
					var control proxy.CatalogControl
					if err := json.Unmarshal(encoded, &control); err != nil {
						t.Fatal(err)
					}
					if route.Model == "eleven_v3" && (control.ID == "similarity_boost" || control.ID == "use_speaker_boost") {
						t.Fatalf("unsupported v3 control advertised: %s", control.ID)
					}
					if control.ID == "output_format" && !slices.Equal(control.Values, speechSourceFormats) {
						t.Fatalf("model %s formats=%v", route.Model, control.Values)
					}
				}
			}
			slices.Sort(discovered)
			if !slices.Equal(discovered, []string{"eleven_flash_v2_5", "eleven_multilingual_v2", "eleven_turbo_v2_5", "eleven_v3"}) {
				t.Fatalf("speech discovery=%v", discovered)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			voices, err := client.GetMediaVoices(ctx, llmproxyclient.MediaVoiceQuery{Provider: provider})
			if err != nil {
				t.Fatal(err)
			}
			dictionaryInput := llmproxyclient.MediaOperationInput{Capability: "audio.dictionary.create", Provider: provider, Input: json.RawMessage(dictionaryInputJSON), Controls: json.RawMessage(`{}`)}
			dictionaryOperation, err := client.CreateMediaOperation(ctx, "speech-dictionary", dictionaryInput)
			if err != nil {
				t.Fatal(err)
			}
			dictionaryOperation, err = client.WaitMediaOperation(ctx, dictionaryOperation.OperationID, time.Millisecond)
			if err != nil || dictionaryOperation.State != "succeeded" {
				t.Fatalf("dictionary=%+v err=%v", dictionaryOperation, err)
			}
			metadata, err := client.GetAsset(ctx, dictionaryOperation.Outputs[0].AssetID)
			if err != nil {
				t.Fatal(err)
			}
			content, err := client.DownloadAsset(ctx, metadata)
			if err != nil {
				t.Fatal(err)
			}
			var dictionary llmproxycontract.MediaDictionary
			if err = json.Unmarshal(content, &dictionary); err != nil {
				t.Fatal(err)
			}
			for _, model := range []string{"eleven_multilingual_v2", "eleven_flash_v2_5", "eleven_turbo_v2_5", "eleven_v3"} {
				fields := map[string]any{"text": "Hello 世界", "voice_id": voices.Voices[0].VoiceID}
				encoded, _ := json.Marshal(fields)
				input := llmproxyclient.MediaOperationInput{Capability: "audio.speech.generate", Provider: provider, Model: model, Input: encoded, Controls: json.RawMessage(`{"output_format":"mp3_44100_128","timestamps":false}`)}
				first, err := client.CreateMediaOperation(ctx, model+"plain", input)
				if err != nil {
					t.Fatalf("source speech model %s must execute: %v", model, err)
				}
				first, err = client.WaitMediaOperation(ctx, first.OperationID, time.Millisecond)
				if err != nil || first.State != "succeeded" || len(first.Outputs) != 2 {
					t.Fatalf("speech=%+v err=%v", first, err)
				}
				fields["previous_text"] = "Before."
				fields["next_text"] = "After."
				fields["previous_operation_ids"] = []string{first.OperationID}
				fields["next_operation_ids"] = []string{first.OperationID}
				fields["dictionaries"] = []map[string]string{{"dictionary_id": dictionary.DictionaryID, "version_id": dictionary.VersionID}}
				input.Input, _ = json.Marshal(fields)
				controls := map[string]any{"output_format": "mp3_44100_128", "timestamps": true, "stability": 0.5, "similarity_boost": 0.6, "style": 0.2, "use_speaker_boost": false, "speed": 0.9, "seed": 4294967295, "text_normalization": "on", "language_text_normalization": true}
				if model != "eleven_multilingual_v2" {
					controls["language_code"] = "ja"
				}
				if model == "eleven_v3" {
					delete(controls, "similarity_boost")
					delete(controls, "use_speaker_boost")
				}

				input.Controls, _ = json.Marshal(controls)
				document, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				request := httptest.NewRequest(http.MethodPost, "/model/v1/operations", bytes.NewReader(document))
				request.Header.Set("Authorization", "Bearer tenant-secret")
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Idempotency-Key", model+"timestamps")
				if err := contract.ValidateRequest("/model/v1/operations", http.MethodPost, request, document); err != nil {
					t.Fatal(err)
				}

				second, err := client.CreateMediaOperation(ctx, model+"timestamps", input)
				if err != nil {
					t.Fatal(err)
				}
				second, err = client.WaitMediaOperation(ctx, second.OperationID, time.Millisecond)
				if err != nil || second.State != "succeeded" || len(second.Outputs) != 3 {
					t.Fatalf("timed speech=%+v err=%v", second, err)
				}
				metadata, err = client.GetAsset(ctx, second.Outputs[2].AssetID)
				if err != nil {
					t.Fatal(err)
				}
				content, err = client.DownloadAsset(ctx, metadata)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(content), "private-") || strings.Contains(string(content), "deliberately") {
					t.Fatalf("private timing data exposed: %s", content)
				}
				var timings struct {
					Alignment struct {
						Characters []string  `json:"characters"`
						Starts     []float64 `json:"character_start_times_seconds"`
					} `json:"alignment"`
				}
				if err = json.Unmarshal(content, &timings); err != nil {
					t.Fatal(err)
				}
				if strings.Join(timings.Alignment.Characters, "") != "Hello 世界" || (model == "eleven_v3" && timings.Alignment.Starts[0] != float64(len([]rune("[deliberately slowly] ")))/10) {
					t.Fatalf("caller alignment=%s", content)
				}
			}
			if posts.Load() != 8 {
				t.Fatalf("speech native submissions=%d", posts.Load())
			}
		})
	}
}

func TestProviderSpeechGenerationNativeFailuresNeverResubmit(t *testing.T) {
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
			input := speechGenerationInput(t, client)
			if scenario.name == "odd pcm" {
				input.Controls = json.RawMessage(`{"timestamps":false,"output_format":"pcm_16000"}`)
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

func TestProviderSpeechGenerationRetainsUncertainReceiptStorageFailure(t *testing.T) {
	client, database, _ := speechFixture(t, "elevenlabs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte{1, 2})
	})
	input := speechGenerationInput(t, client)
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

func TestProviderSpeechGenerationCancellationKeepsNativeRequestSingle(t *testing.T) {
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
	input := speechGenerationInput(t, client)
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

func speechGenerationInput(t *testing.T, client llmproxyclient.Client) llmproxyclient.MediaOperationInput {
	t.Helper()
	page, err := client.GetMediaVoices(t.Context(), llmproxyclient.MediaVoiceQuery{Provider: "elevenlabs"})
	if err != nil || len(page.Voices) != 1 {
		t.Fatalf("voices=%+v error=%v", page, err)
	}
	return llmproxyclient.MediaOperationInput{Capability: "audio.speech.generate", Provider: "elevenlabs", Model: "eleven_multilingual_v2", Input: json.RawMessage(fmt.Sprintf(`{"text":"Hello","voice_id":%q}`, page.Voices[0].VoiceID)), Controls: json.RawMessage(`{"output_format":"mp3_44100_128","timestamps":false}`)}
}

func TestProviderSpeechGenerationRejectsInvalidInputAndUnownedContext(t *testing.T) {
	var posts atomic.Int32
	client, database, _ := speechFixture(t, "elevenlabs", func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("request-id", "private-continuity")
		_, _ = w.Write([]byte{1, 2})
	})
	base := speechGenerationInput(t, client)
	for index, changes := range []map[string]any{{"timestamps": nil}, {"output_format": "unknown"}, {"speed": 0.69}, {"stability": 1.1}, {"seed": 4294967296}, {"text_normalization": "unknown"}, {"language_code": "en"}, {"remove_background_noise": true}} {
		input := base
		controls := map[string]any{"timestamps": false, "output_format": "mp3_44100_128"}
		for key, value := range changes {
			controls[key] = value
		}
		input.Controls, _ = json.Marshal(controls)
		if _, err := client.CreateMediaOperation(t.Context(), fmt.Sprintf("invalid-generation-control-%d", index), input); err == nil {
			t.Fatalf("accepted controls=%s", input.Controls)
		}
	}
	for index, changes := range []map[string]any{
		{"text": " "}, {"text": strings.Repeat("a", 10001)}, {"previous_text": strings.Repeat("a", 10001)}, {"voice_id": "unowned-voice"},
		{"dictionaries": []map[string]string{{"dictionary_id": "missing", "version_id": "missing"}}},
		{"dictionaries": []map[string]string{{}, {}, {}, {}}},
		{"previous_operation_ids": []string{"missing"}}, {"next_operation_ids": []string{"missing"}},
		{"previous_operation_ids": []string{"one", "two", "three", "four"}},
		{"extra": "unknown"},
	} {
		input := base
		var fields map[string]any
		_ = json.Unmarshal(base.Input, &fields)
		for key, value := range changes {
			fields[key] = value
		}
		input.Input, _ = json.Marshal(fields)
		if _, err := client.CreateMediaOperation(t.Context(), fmt.Sprintf("invalid-generation-input-%d", index), input); err == nil {
			t.Fatalf("accepted input=%s", input.Input)
		}
	}
	for index, field := range []string{"similarity_boost", "use_speaker_boost"} {
		input := base
		input.Model = "eleven_v3"
		controls := map[string]any{"output_format": "mp3_44100_128", "timestamps": false}
		if field == "similarity_boost" {
			controls[field] = 0.5
		} else {
			controls[field] = false
		}
		input.Controls, _ = json.Marshal(controls)
		if _, err := client.CreateMediaOperation(t.Context(), fmt.Sprintf("unsupported-v3-control-%d", index), input); err == nil {
			t.Fatal("unsupported v3 control accepted")
		}
	}
	if posts.Load() != 0 {
		t.Fatalf("invalid requests submitted=%d", posts.Load())
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	first, err := client.CreateMediaOperation(ctx, "continuity-source", base)
	if err != nil {
		t.Fatal(err)
	}
	first, err = client.WaitMediaOperation(ctx, first.OperationID, time.Millisecond)
	if err != nil || first.State != "succeeded" {
		t.Fatalf("source=%+v error=%v", first, err)
	}
	for index, receipt := range []string{"{", `{"request_id":""}`} {
		if err := database.Table("media_operation_records").Where("operation_id = ?", first.OperationID).Update("provider_handle", receipt).Error; err != nil {
			t.Fatal(err)
		}
		input := base
		var fields map[string]any
		_ = json.Unmarshal(base.Input, &fields)
		fields["previous_operation_ids"] = []string{first.OperationID}
		input.Input, _ = json.Marshal(fields)
		if _, err := client.CreateMediaOperation(ctx, fmt.Sprintf("invalid-receipt-%d", index), input); err == nil {
			t.Fatal("invalid continuation receipt accepted")
		}
	}
	if err := database.Table("media_operation_records").Where("operation_id = ?", first.OperationID).Updates(map[string]any{"provider_handle": `{"request_id":"private-continuity"}`, "catalog_revision": "obsolete"}).Error; err != nil {
		t.Fatal(err)
	}
	stale := base
	var staleFields map[string]any
	_ = json.Unmarshal(base.Input, &staleFields)
	staleFields["previous_operation_ids"] = []string{first.OperationID}
	stale.Input, _ = json.Marshal(staleFields)
	if _, err := client.CreateMediaOperation(ctx, "stale-continuity-authority", stale); err == nil {
		t.Fatal("stale continuity authority accepted")
	}
	if posts.Load() != 1 {
		t.Fatal("invalid continuity submitted native speech")
	}
}

func TestProviderSpeechGenerationPreservesFormatsAndPacing(t *testing.T) {
	var expectation atomic.Value
	client, _, _ := speechFixture(t, "elevenlabs", func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Text     string         `json:"text"`
			Settings map[string]any `json:"voice_settings"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		expected := expectation.Load().([2]string)
		if payload.Text != expected[0] || r.URL.Query().Get("output_format") != expected[1] {
			t.Errorf("text=%q format=%s", payload.Text, r.URL.Query().Get("output_format"))
		}
		if _, present := payload.Settings["speed"]; present {
			t.Error("pacing route sent native speed")
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte{1, 2})
	})
	input := speechGenerationInput(t, client)
	input.Model = "eleven_v3"
	for index, sourceFormat := range speechSourceFormats {
		format := sourceFormat
		expectation.Store([2]string{"Hello", format})
		input.Controls = json.RawMessage(fmt.Sprintf(`{"output_format":%q,"timestamps":false,"speed":1}`, format))
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		operation, err := client.CreateMediaOperation(ctx, fmt.Sprintf("generation-format-%d", index), input)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		operation, err = client.WaitMediaOperation(ctx, operation.OperationID, time.Millisecond)
		cancel()
		if err != nil || operation.State != "succeeded" || len(operation.Outputs) != 2 {
			t.Fatalf("format result=%+v error=%v", operation, err)
		}
	}
	for index, scenario := range []struct {
		speed float64
		tag   string
	}{{0.7, "[drawn out and extremely slowly]"}, {0.8, "[extremely slowly]"}, {0.84, "[very slowly]"}, {0.9, "[deliberately slowly]"}, {0.93, "[slowly]"}, {0.99, "[slightly slower]"}, {1.03, "[slightly faster]"}, {1.07, "[quickly]"}, {1.11, "[very quickly]"}, {1.15, "[extremely quickly]"}, {1.2, "[rapid-fire]"}} {
		format := "mp3_44100_128"
		expectation.Store([2]string{scenario.tag + " Hello", format})
		input.Controls = json.RawMessage(fmt.Sprintf(`{"output_format":%q,"timestamps":false,"speed":%v}`, format, scenario.speed))
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		operation, err := client.CreateMediaOperation(ctx, fmt.Sprintf("generation-pacing-%d", index), input)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		operation, err = client.WaitMediaOperation(ctx, operation.OperationID, time.Millisecond)
		cancel()
		if err != nil || operation.State != "succeeded" {
			t.Fatalf("pacing=%+v error=%v", operation, err)
		}
	}
}

func TestProviderSpeechGenerationRejectsChangedDependenciesBeforeDispatch(t *testing.T) {
	for _, scenario := range []string{"binding", "credential", "voice"} {
		t.Run(scenario, func(t *testing.T) {
			var posts atomic.Int32
			client, database, restart := speechFixture(t, "elevenlabs", func(w http.ResponseWriter, r *http.Request) {
				posts.Add(1)
				w.Header().Set("Content-Type", "audio/mpeg")
				_, _ = w.Write([]byte{1, 2})
			})
			input := speechGenerationInput(t, client)
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

func TestProviderSpeechGenerationTimestampBoundary(t *testing.T) {
	for name, body := range map[string]string{
		"invalid json":            "{",
		"invalid audio":           `{"audio_base64":"!"}`,
		"empty audio":             `{"audio_base64":""}`,
		"missing arrays":          `{"audio_base64":"AQI=","alignment":{}}`,
		"array lengths":           `{"audio_base64":"AQI=","alignment":{"characters":["a"],"character_start_times_seconds":[],"character_end_times_seconds":[1]}}`,
		"negative time":           `{"audio_base64":"AQI=","alignment":{"characters":["a"],"character_start_times_seconds":[-1],"character_end_times_seconds":[1]}}`,
		"wrong prefix":            `{"audio_base64":"AQI=","alignment":{"characters":["a"],"character_start_times_seconds":[0],"character_end_times_seconds":[1]}}`,
		"valid absent alignments": `{"audio_base64":"AQI="}`,
	} {
		t.Run(name, func(t *testing.T) {
			client, _, _ := speechFixture(t, "elevenlabs", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			})
			input := speechGenerationInput(t, client)
			input.Model = "eleven_v3"
			input.Controls = json.RawMessage(`{"output_format":"mp3_44100_128","timestamps":true,"speed":0.9}`)
			if name == "missing arrays" {
				input.Controls = json.RawMessage(`{"output_format":"mp3_44100_128","timestamps":true,"speed":1}`)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			operation, err := client.CreateMediaOperation(ctx, "timing-boundary", input)
			if err != nil {
				t.Fatal(err)
			}
			operation, err = client.WaitMediaOperation(ctx, operation.OperationID, time.Millisecond)
			state := "uncertain"
			if name == "valid absent alignments" {
				state = "succeeded"
			}
			if err != nil || operation.State != state {
				t.Fatalf("timing=%+v error=%v", operation, err)
			}
		})
	}
}

func TestProviderSpeechGenerationCatalogRejectsInvalidCompositions(t *testing.T) {
	for name, change := range map[string]func(*proxy.ProviderOffering){
		"wrong codec":            func(o *proxy.ProviderOffering) { o.WireContract = proxy.CatalogProtocolOpenAIImages },
		"missing limits":         func(o *proxy.ProviderOffering) { o.Limits = nil },
		"text behavior":          func(o *proxy.ProviderOffering) { o.WebSearch = true },
		"unknown control":        func(o *proxy.ProviderOffering) { o.Controls[0].ID = "unknown" },
		"unknown format":         func(o *proxy.ProviderOffering) { o.Controls[0].Values = []string{"unknown"} },
		"speed outside protocol": func(o *proxy.ProviderOffering) { value := 1.3; o.Controls[5].Maximum = &value },
		"language format": func(o *proxy.ProviderOffering) {
			o.Controls = append(o.Controls, proxy.CatalogControl{ID: "language_code", Kind: "enum", Values: []string{"EN"}})
		},
		"missing required control": func(o *proxy.ProviderOffering) {
			o.Controls[5] = proxy.CatalogControl{ID: "language_code", Kind: "enum", Values: []string{"en"}}
		},
		"unknown limit":    func(o *proxy.ProviderOffering) { o.Limits[0].ID = "unknown" },
		"too much context": func(o *proxy.ProviderOffering) { value := 4; o.Limits[2].Value = &value },
	} {
		t.Run(name, func(t *testing.T) {
			catalog := testfixtures.ModelCatalog(t)
			for index := range catalog.Offerings {
				if catalog.Offerings[index].Model == "eleven_multilingual_v2" {
					change(&catalog.Offerings[index])
					break
				}
			}
			if _, err := proxy.NewCatalogService(catalog); err == nil {
				t.Fatal("invalid generation catalog accepted")
			}
		})
	}
}

func TestProviderSpeechGenerationCatalogRejectsUndeclaredPacing(t *testing.T) {
	for _, variation := range []string{"", "unknown"} {
		t.Run(variation, func(t *testing.T) {
			schema := testfixtures.ProviderCatalog(t).Schema()
			for index := range schema.Providers {
				if schema.Providers[index].ID != "elevenlabs" {
					continue
				}
				for transport := range schema.Providers[index].Transports {
					if schema.Providers[index].Transports[transport].ID == "speech" {
						schema.Providers[index].Transports[transport].Components.RequestCodec.Variation = variation
					}
				}
			}
			document, err := yaml.Marshal(schema)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := proxy.ParseProviderCatalog(document); err == nil {
				t.Fatal("undeclared pacing accepted")
			}
		})
	}
}
