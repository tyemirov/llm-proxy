package llmproxyclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestElevenLabsAccountResourcesClientRejectsInvalidResponses(t *testing.T) {
	var body atomic.Value
	body.Store(`{}`)
	var status atomic.Int32
	status.Store(200)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" || r.URL.RawQuery != "" || !strings.HasPrefix(r.URL.Path, "/model/v1/provider-resources/elevenlabs/") {
			t.Error("invalid resource request")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(int(status.Load()))
		_, _ = w.Write([]byte(body.Load().(string)))
	}))
	defer server.Close()
	config, err := NewConfig(ConfigInput{BaseURL: server.URL + "/v2?key=obsolete", Secret: "key"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range []string{"", "ElevenLabs", "elevenlabs/other"} {
		if _, err := client.GetProviderMetadata(context.Background(), provider); !errors.Is(err, ErrInvalidClientRequest) {
			t.Fatalf("provider=%q error=%v", provider, err)
		}
		if _, err := client.GetProviderQuotas(context.Background(), provider); !errors.Is(err, ErrInvalidClientRequest) {
			t.Fatalf("provider=%q error=%v", provider, err)
		}
	}
	for _, value := range []string{`{}`, `null`, `{"provider":"other","models":[]}`, `{"provider":"elevenlabs","models":null}`, `{"provider":"elevenlabs","models":[{"model_id":"","name":"Model"}]}`, `{"provider":"elevenlabs","models":[{"model_id":"model","name":""}]}`} {
		body.Store(value)
		if _, err := client.GetProviderMetadata(context.Background(), "elevenlabs"); !errors.Is(err, ErrClientHTTPFailure) {
			t.Fatalf("metadata body=%s error=%v", value, err)
		}
	}
	for _, value := range []string{`{}`, `null`, `{"provider":"elevenlabs","subscription":{"tier":"creator","status":"active","credit_extension":{"unlimited":false,"value":null}}}`, `{"provider":"elevenlabs","subscription":{"tier":"creator","status":"active","credit_extension":{"unlimited":true,"value":1}}}`} {
		body.Store(value)
		if _, err := client.GetProviderQuotas(context.Background(), "elevenlabs"); !errors.Is(err, ErrClientHTTPFailure) {
			t.Fatalf("quota body=%s error=%v", value, err)
		}
	}

	validModel := `{"model_id":"m","name":"Model","can_do_text_to_speech":null,"can_do_voice_conversion":null,"maximum_text_length_per_request":null,"max_characters_request_free_user":null,"max_characters_request_subscribed_user":null}`
	for _, models := range []string{
		`[{"model_id":"m","name":"Model"}]`,
		`[` + validModel + `,` + validModel + `]`,
		`[` + strings.Replace(validModel, `"maximum_text_length_per_request":null`, `"maximum_text_length_per_request":-1`, 1) + `]`,
	} {
		body.Store(`{"provider":"elevenlabs","models":` + models + `}`)
		if _, err := client.GetProviderMetadata(context.Background(), "elevenlabs"); !errors.Is(err, ErrClientHTTPFailure) {
			t.Fatalf("accepted invalid models %s: %v", models, err)
		}
	}
	validSubscription := `{"tier":"creator","status":"active","character_count":0,"character_limit":0,"credit_extension":{"unlimited":false,"value":0},"can_extend_credit_limit":false,"current_overage":null,"has_open_invoices":false,"currency":null,"next_character_count_reset_unix":null}`
	for _, subscription := range []string{
		strings.Replace(validSubscription, `"character_count":0,`, ``, 1),
		strings.Replace(validSubscription, `"character_count":0`, `"character_count":null`, 1),
		strings.Replace(validSubscription, `"unlimited":false,`, ``, 1),
		strings.Replace(validSubscription, `"unlimited":false`, `"unlimited":true`, 1),
		strings.Replace(validSubscription, `"value":0`, `"value":null`, 1),
		strings.Replace(validSubscription, `"current_overage":null`, `"current_overage":{"amount":"bad","currency":"usd"}`, 1),
		strings.Replace(validSubscription, `"next_character_count_reset_unix":null`, `"next_character_count_reset_unix":-1`, 1),
	} {
		body.Store(`{"provider":"elevenlabs","subscription":` + subscription + `}`)
		if _, err := client.GetProviderQuotas(context.Background(), "elevenlabs"); !errors.Is(err, ErrClientHTTPFailure) {
			t.Fatalf("accepted invalid subscription %s: %v", subscription, err)
		}
	}
	status.Store(502)
	if _, err := client.GetProviderMetadata(context.Background(), "elevenlabs"); err == nil {
		t.Fatal("metadata accepted provider failure")
	}
	if _, err := client.GetProviderQuotas(context.Background(), "elevenlabs"); err == nil {
		t.Fatal("quotas accepted provider failure")
	}
}
