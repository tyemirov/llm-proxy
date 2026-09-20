package proxy

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
)

type voiceBoundaryProvider struct {
	discovery MediaVoiceDiscovery
	authority func() (string, error)
}

func (provider voiceBoundaryProvider) DiscoverMediaVoices(context.Context, string, MediaVoiceQuery) (MediaVoiceDiscovery, error) {
	return provider.discovery, nil
}
func (provider voiceBoundaryProvider) MediaVoiceAuthority(context.Context, string) (string, error) {
	return provider.authority()
}

type voiceBoundaryErrorReader struct{}

func (voiceBoundaryErrorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func voiceBoundaryHTTP(t *testing.T, fixture mediaOperationInternalFixture) func(string, int) []byte {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(contextKeyTenant, fixture.tenant) })
	router.GET(llmproxycontract.MediaVoicesPath, fixture.service.mediaVoiceCollectionHandler())
	router.GET(llmproxycontract.MediaVoicesPath+"/:voice_id", fixture.service.mediaVoiceHandler())
	router.GET(llmproxycontract.MediaVoicesPath+"/:voice_id/previews/:preview", fixture.service.mediaVoicePreviewHandler())
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return func(path string, want int) []byte {
		t.Helper()
		response, err := server.Client().Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != want {
			t.Fatalf("path=%s status=%d want=%d body=%s", path, response.StatusCode, want, body)
		}
		return body
	}
}

func TestMediaVoicePageBoundaryFailures(t *testing.T) {
	for _, scenario := range []string{"authority changed", "native page query", "native page corrupt", "cursor entropy", "cursor expired", "cursor wrong revision", "snapshot offset"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newMediaOperationInternalFixture(t)
			fixture.service.voiceCursorCipher = internalManagedProviderKeyCipher()
			fixture.service.voiceCursorRandom = rand.Reader
			record := MediaVoiceProviderRecord{Provider: "xai", Mode: MediaVoiceModePreset, DisplayName: "Voice", ProviderVoiceReference: "private"}
			calls := 0
			provider := voiceBoundaryProvider{discovery: MediaVoiceDiscovery{NativePage: true, Voices: []MediaVoiceProviderRecord{record}}, authority: func() (string, error) {
				calls++
				if scenario == "authority changed" && calls == 2 {
					return "changed", nil
				}
				return "", nil
			}}
			if scenario == "native page query" {
				failNthMediaGORMOperation(t, fixture.database, "query", "media_voice_records", 2)
			}
			if scenario == "native page corrupt" {
				reads := 0
				if err := fixture.database.Callback().Query().After("gorm:query").Register("corrupt_voice_page", func(tx *gorm.DB) {
					if tx.Statement.Table == "media_voice_records" {
						reads++
						if reads == 2 {
							tx.Statement.Dest.(*mediaVoiceRecord).Metadata = []byte(`{`)
						}
					}
				}); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "cursor entropy" {
				provider.discovery.NextPageToken = "native"
				fixture.service.voiceCursorRandom = voiceBoundaryErrorReader{}
			}
			fixture.service.voiceProviders = map[string]MediaVoiceProvider{"xai": provider}
			read := voiceBoundaryHTTP(t, fixture)
			path := llmproxycontract.MediaVoicesPath + "?provider=xai"
			want := 500
			if scenario == "authority changed" {
				want = 502
			}
			if strings.HasPrefix(scenario, "cursor ") && scenario != "cursor entropy" || scenario == "snapshot offset" {
				cursor := mediaVoiceCursor{Query: llmproxycontract.MediaVoiceQuery{Provider: "xai", PageSize: 1}, Token: "999", Revision: fixture.service.catalog.Revision(), Expires: fixture.now.Add(time.Minute)}
				if scenario == "cursor expired" {
					cursor.Expires = fixture.now
				}
				if scenario == "cursor wrong revision" {
					cursor.Revision = "different"
				}
				if scenario == "snapshot offset" {
					provider.discovery.NativePage = false
					fixture.service.voiceProviders["xai"] = provider
				}
				raw, _ := json.Marshal(cursor)
				sealed, err := fixture.service.voiceCursorCipher.encryptConnection(rand.Reader, fixture.tenant.identifier.string(), "xai", voiceCursorPurpose, string(raw))
				if err != nil {
					t.Fatal(err)
				}
				path += "&cursor=" + url.QueryEscape(sealed)
				want = 400
			}
			read(path, want)
		})
	}
}

func TestMediaVoicePreviewBoundaryFailures(t *testing.T) {
	for _, scenario := range []string{"provider absent", "authority obsolete", "missing preview", "invalid index", "query", "corrupt previews", "untrusted origin", "credential removed", "transport failure", "native status", "native type", "native empty", "native large", "corrupt metadata", "corrupt preview metadata", "stored public preview", "stored language preview", "language preview"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newMediaOperationInternalFixture(t)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if scenario == "native status" {
					w.WriteHeader(401)
					return
				}
				w.Header().Set("Content-Type", "audio/mpeg")
				if scenario == "native type" {
					w.Header().Set("Content-Type", "text/plain")
				}
				if scenario == "native empty" {
					return
				}
				if scenario == "native large" {
					_, _ = io.WriteString(w, strings.Repeat("a", mediaVoicePreviewMaximumBytes+1))
					return
				}
				_, _ = io.WriteString(w, "ID3preview")
			}))
			defer upstream.Close()
			fixture.service.httpClient = upstream.Client()
			provider := fixture.service.providers.definitions[providerID("xai")]
			provider.resources = []ProviderCatalogResource{{Kind: llmproxycontract.ProviderResourceVoices, Transport: "voices"}}
			provider.transports["voices"] = providerTransportDefinition{artifactOrigins: []string{upstream.URL}}
			fixture.service.providers.definitions[providerID("xai")] = provider
			fixture.service.voiceProviders = map[string]MediaVoiceProvider{"xai": voiceBoundaryProvider{authority: func() (string, error) {
				if scenario == "authority obsolete" {
					return "other", nil
				}
				return "", nil
			}}}
			observed := MediaVoiceProviderRecord{Provider: "xai", Mode: MediaVoiceModePreset, DisplayName: "Voice", ProviderVoiceReference: "private", PreviewURLs: []string{upstream.URL + "/audio"}}
			if scenario == "missing preview" {
				observed.PreviewURLs = []string{""}
			}
			if scenario == "untrusted origin" {
				observed.PreviewURLs = []string{"https://private.invalid/audio"}
			}
			if scenario == "language preview" {
				observed.PreviewURLs = append(observed.PreviewURLs, upstream.URL+"/audio")
				observed.Metadata.VerifiedLanguages = []llmproxycontract.MediaVoiceLanguage{{Language: "en", ModelID: "model"}}
			}
			record, err := newMediaVoiceRecord(fixture.tenant.identifier.string(), "xai", observed, fixture.now)
			if err != nil {
				t.Fatal(err)
			}
			if err := fixture.database.Create(&record).Error; err != nil {
				t.Fatal(err)
			}
			path := llmproxycontract.MediaVoicesPath + "/" + record.VoiceID + "/previews/0"
			want := 502
			switch scenario {
			case "provider absent":
				fixture.service.voiceProviders = nil
				want = 404
			case "authority obsolete", "missing preview":
				want = 404
			case "invalid index":
				path = strings.TrimSuffix(path, "0") + "invalid"
				want = 404
			case "query":
				path += "?private=value"
				want = 404
			case "corrupt previews":
				if err := fixture.database.Model(&record).Update("preview_urls", []byte(`{`)).Error; err != nil {
					t.Fatal(err)
				}
				want = 500
			case "stored public preview", "stored language preview":
				metadata := llmproxycontract.MediaVoiceMetadata{Labels: map[string]string{}, HighQualityBaseModelIDs: []string{}, VerifiedLanguages: []llmproxycontract.MediaVoiceLanguage{}}
				leaked := "https://private.invalid/audio"
				if scenario == "stored public preview" {
					metadata.Preview = &leaked
				} else {
					metadata.VerifiedLanguages = []llmproxycontract.MediaVoiceLanguage{{Language: "en", ModelID: "model", Preview: &leaked}}
				}
				encoded, _ := json.Marshal(metadata)
				if err := fixture.database.Model(&record).Updates(map[string]any{"metadata": encoded, "preview_urls": []byte(`[]`)}).Error; err != nil {
					t.Fatal(err)
				}
				path = llmproxycontract.MediaVoicesPath + "/" + record.VoiceID
				want = 500
			case "corrupt preview metadata":
				if err := fixture.database.Model(&record).Update("preview_urls", []byte(`["",""]`)).Error; err != nil {
					t.Fatal(err)
				}
				path = llmproxycontract.MediaVoicesPath + "/" + record.VoiceID
				want = 500
			case "corrupt metadata":
				if err := fixture.database.Model(&record).Update("metadata", []byte(`{`)).Error; err != nil {
					t.Fatal(err)
				}
				path = llmproxycontract.MediaVoicesPath + "/" + record.VoiceID
				want = 500
			case "credential removed":
				if err := fixture.database.Where("tenant_id = ?", fixture.tenant.identifier.string()).Delete(&managedTenantConnectionRecord{}).Error; err != nil {
					t.Fatal(err)
				}
				want = 404
			case "transport failure":
				upstream.Close()
			case "language preview":
				path = llmproxycontract.MediaVoicesPath + "/" + record.VoiceID
				want = 200
			}
			read := voiceBoundaryHTTP(t, fixture)
			read(path, want)
			read(llmproxycontract.MediaVoicesPath+"/voi_00000000000000000000000000000000/previews/0", 404)
		})
	}
}

func TestMediaVoiceNativeCredentialBoundary(t *testing.T) {
	fixture := newMediaOperationInternalFixture(t)
	adapter := elevenLabsVoiceProvider{provider: providerDefinition{identifier: providerID("missing")}, store: fixture.service.store}
	if _, err := adapter.DiscoverMediaVoices(t.Context(), fixture.tenant.identifier.string(), MediaVoiceQuery{}); err == nil {
		t.Fatal("missing native credential accepted")
	}
	adapter.provider.identifier = providerID("xai")
	if _, err := adapter.DiscoverMediaVoices(t.Context(), fixture.tenant.identifier.string(), MediaVoiceQuery{}); err == nil {
		t.Fatal("incomplete native connection accepted")
	}
}
