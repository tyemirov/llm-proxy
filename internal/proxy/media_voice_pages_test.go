package proxy_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"go.uber.org/zap"
)

type snapshotVoiceProvider struct{}

func (snapshotVoiceProvider) MediaVoiceAuthority(context.Context, string) (string, error) {
	return "", nil
}
func (snapshotVoiceProvider) DiscoverMediaVoices(context.Context, string, proxy.MediaVoiceQuery) (proxy.MediaVoiceDiscovery, error) {
	voices := []proxy.MediaVoiceProviderRecord{}
	for index, name := range []string{"Charlie", "Alpha", "Bravo", "Alpha"} {
		voices = append(voices, proxy.MediaVoiceProviderRecord{Provider: "xai", Mode: proxy.MediaVoiceModePreset, DisplayName: name, Language: "en", SampleRates: []int{24000}, DefaultSampleRate: 24000, ProviderVoiceReference: fmt.Sprint(index)})
	}
	return proxy.MediaVoiceDiscovery{Voices: voices}, nil
}

func TestMediaVoiceSnapshotUsesTheSamePageContract(t *testing.T) {
	configuration, err := testfixtures.ProvisionManagedRouter(t, proxy.Configuration{ProviderCatalog: testfixtures.ProviderCatalog(t), AssetStorePath: t.TempDir(), MediaVoiceProviders: map[string]proxy.MediaVoiceProvider{"xai": snapshotVoiceProvider{}}}, zap.NewNop().Sugar(), testfixtures.StandardManagedTenant("snapshot-secret"))
	if err != nil {
		t.Fatal(err)
	}
	router, err := testfixtures.BuildRouter(t, configuration, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	config, _ := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "snapshot-secret"})
	client, _ := llmproxyclient.NewClient(config, server.Client())
	include := true
	page, err := client.GetMediaVoices(t.Context(), llmproxyclient.MediaVoiceQuery{Provider: "xai", PageSize: 1, Sort: "name", SortDirection: "desc", IncludeTotalCount: &include})
	if err != nil || len(page.Voices) != 1 || page.Voices[0].DisplayName != "Charlie" || page.TotalCount == nil || *page.TotalCount != 4 || page.NextCursor == nil {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	for _, name := range []string{"Bravo", "Alpha", "Alpha"} {
		page, err = client.GetMediaVoices(t.Context(), llmproxyclient.MediaVoiceQuery{Provider: "xai", Cursor: *page.NextCursor})
		if err != nil || len(page.Voices) != 1 || page.Voices[0].DisplayName != name {
			t.Fatalf("next=%+v err=%v", page, err)
		}
	}
	if page.HasMore || page.NextCursor != nil {
		t.Fatal("final page retained continuation")
	}
	page, err = client.GetMediaVoices(t.Context(), llmproxyclient.MediaVoiceQuery{Provider: "xai", Search: "brav"})
	if err != nil || len(page.Voices) != 1 || page.Voices[0].DisplayName != "Bravo" || page.TotalCount != nil {
		t.Fatalf("filtered=%+v err=%v", page, err)
	}
	_, err = client.GetMediaVoices(t.Context(), llmproxyclient.MediaVoiceQuery{Provider: "xai", Category: "cloned"})
	if httpFailureStatus(err) != 400 {
		t.Fatalf("unsupported source filter=%v", err)
	}
}
