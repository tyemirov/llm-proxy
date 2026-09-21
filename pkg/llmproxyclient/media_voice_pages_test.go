package llmproxyclient_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
)

const observedVoiceJSON = `{"voice_id":"voi_0123456789abcdef0123456789abcdef","provider":"elevenlabs","mode":"preset","language":null,"display_name":"Reader","default":false,"sample_rates":[],"default_sample_rate":null,"description":"Narrator","category":"cloned","labels":{"accent":"american"},"high_quality_base_model_ids":["eleven_v3"],"verified_languages":[{"language":"en","model_id":"eleven_v3","accent":"american","locale":"en-US","preview":"/model/v1/voices/voi_0123456789abcdef0123456789abcdef/previews/1"}],"preview":"/model/v1/voices/voi_0123456789abcdef0123456789abcdef/previews/0"}`

func TestMediaVoiceClientPreservesPagesAndRejectsMalformedObservations(t *testing.T) {
	var body atomic.Value
	page := func(voices string) string {
		return `{"voices":[` + voices + `],"has_more":true,"next_cursor":"opaque-cursor","total_count":2}`
	}
	body.Store(page(observedVoiceJSON))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing tenant authorization")
		}
		if r.URL.Path == "/model/v1/voices" && r.URL.Query().Get("provider") != "elevenlabs" {
			t.Errorf("provider query=%s", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, body.Load().(string))
	}))
	defer server.Close()
	config, _ := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "secret"})
	client, _ := llmproxyclient.NewClient(config, server.Client())
	include := true
	query := llmproxyclient.MediaVoiceQuery{Provider: "elevenlabs", Search: "Reader", PageSize: 1, Sort: "name", SortDirection: "asc", VoiceType: "personal", Category: "cloned", IncludeTotalCount: &include}
	result, err := client.GetMediaVoices(t.Context(), query)
	if err != nil || len(result.Voices) != 1 || result.Voices[0].Language != nil || result.Voices[0].DefaultSampleRate != nil || result.Voices[0].Preview == nil || len(result.Voices[0].VerifiedLanguages) != 1 || result.NextCursor == nil {
		t.Fatalf("page=%+v error=%v", result, err)
	}
	body.Store(observedVoiceJSON)
	if _, err := client.GetMediaVoice(t.Context(), result.Voices[0].VoiceID); err != nil {
		t.Fatal(err)
	}
	variants := []string{
		strings.Replace(observedVoiceJSON, `"language":null`, `"language":""`, 1),
		strings.Replace(observedVoiceJSON, `"sample_rates":[]`, `"sample_rates":null`, 1),
		strings.Replace(observedVoiceJSON, `"default_sample_rate":null`, `"default_sample_rate":24000`, 1),
		strings.Replace(observedVoiceJSON, `"preview":"/model/v1/voices/voi_0123456789abcdef0123456789abcdef/previews/0"`, `"preview":"https://private.invalid/audio"`, 1),
		strings.Replace(observedVoiceJSON, `"high_quality_base_model_ids":["eleven_v3"]`, `"high_quality_base_model_ids":[""]`, 1),
		strings.Replace(observedVoiceJSON, `"language":"en"`, `"language":""`, 1),
		strings.Replace(observedVoiceJSON, `/previews/1`, `/previews/0`, 1),
	}
	for _, rates := range []string{`[24000,24000]`, `[-1,24000]`, `[48000]`} {
		variants = append(variants, strings.Replace(strings.Replace(observedVoiceJSON, `"sample_rates":[]`, `"sample_rates":`+rates, 1), `"default_sample_rate":null`, `"default_sample_rate":24000`, 1))
	}
	for _, invalid := range variants {
		body.Store(invalid)
		if _, err := client.GetMediaVoice(t.Context(), result.Voices[0].VoiceID); err == nil {
			t.Fatalf("invalid voice accepted: %s", invalid)
		}
		body.Store(page(invalid))
		if _, err := client.GetMediaVoices(t.Context(), query); err == nil {
			t.Fatalf("invalid voice in page accepted: %s", invalid)
		}
	}
	for _, invalid := range []string{page(observedVoiceJSON + "," + observedVoiceJSON), page(strings.Replace(observedVoiceJSON, `"provider":"elevenlabs"`, `"provider":"other"`, 1)), `{"voices":[],"has_more":null,"next_cursor":null,"total_count":null}`, `{"voices":[],"has_more":true,"next_cursor":null,"total_count":null}`, `{"voices":[],"has_more":true,"next_cursor":" ","total_count":null}`, `{"voices":[],"has_more":false,"next_cursor":null,"total_count":-1}`} {
		body.Store(invalid)
		if _, err := client.GetMediaVoices(t.Context(), query); err == nil {
			t.Fatalf("invalid page accepted: %s", invalid)
		}
	}
	body.Store(page(observedVoiceJSON))
	encoded, _ := json.Marshal(query)
	if !strings.Contains(string(encoded), `"page_size":1`) {
		t.Fatalf("query=%s", encoded)
	}
}
