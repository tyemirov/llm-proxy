package llmproxyclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

func TestProviderServicesClientVideoDiscovery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tenant" || r.URL.Path != "/model/v1/capabilities" {
			t.Error("invalid tenant capability request")
		}
		_, _ = io.WriteString(w, `{"catalog_revision":"current","routes":[],"resources":[],"services":[{"capability":"video.lipsync","provider":"heygen","controls":[],"limits":[]},{"capability":"video.translate","provider":"heygen","controls":[],"limits":[]}]}`)
	}))
	defer server.Close()
	config, err := NewConfig(ConfigInput{BaseURL: server.URL, Secret: "tenant"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	capabilities, err := client.GetMediaCapabilities(context.Background())
	if err != nil || len(capabilities.Services) != 2 {
		t.Fatalf("video service discovery: %+v, error=%v", capabilities, err)
	}
}

func TestProviderServicesClientTypedVideoRequests(t *testing.T) {
	for _, capability := range []string{"video.lipsync", "video.translate", "avatar.create", "avatar.video.generate"} {
		t.Run(capability, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/model/v1/operations" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer tenant" || r.Header.Get("Idempotency-Key") != "intent" {
					t.Error("invalid typed operation request")
				}
				var request MediaOperationInput
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatal(err)
				}
				if request.Capability != capability || request.Provider != "heygen" || request.Model != "" {
					t.Errorf("operation routing=%+v", request)
				}
				var source map[string]string
				_ = json.Unmarshal(request.Input, &source)
				switch capability {
				case "video.lipsync", "video.translate":
					if source["video_asset_id"] != "video" || source["audio_asset_id"] != "audio" {
						t.Errorf("wrong source=%v", source)
					}
				case "avatar.create":
					if source["image_asset_id"] != "image" || source["name"] != "Character" || string(request.Controls) != "{}" {
						t.Errorf("wrong avatar source=%v", source)
					}
				case "avatar.video.generate":
					if source["avatar_id"] != "avatar" || source["audio_asset_id"] != "audio" {
						t.Errorf("wrong avatar video source=%v", source)
					}
				}
				var response map[string]any
				_ = json.Unmarshal([]byte(validMediaOperationJSON("queued")), &response)
				delete(response, "model")
				response["provider"] = "heygen"
				response["capability"] = capability
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			config, _ := NewConfig(ConfigInput{BaseURL: server.URL, Secret: "tenant"})
			client, _ := NewClient(config, server.Client())
			ctx := context.Background()
			var result MediaOperation
			var err error
			switch capability {
			case "video.lipsync":
				result, err = client.CreateVideoLipSync(ctx, "intent", VideoLipSyncInput{Provider: "heygen", Source: llmproxycontract.VideoLipSyncSource{VideoAssetID: "video", AudioAssetID: "audio"}, Controls: llmproxycontract.VideoLipSyncControls{VideoProcessingControls: llmproxycontract.VideoProcessingControls{Mode: "precision"}}})
			case "video.translate":
				result, err = client.CreateVideoTranslation(ctx, "intent", VideoTranslationInput{Provider: "heygen", Source: llmproxycontract.VideoTranslationSource{VideoAssetID: "video", AudioAssetID: "audio"}, Controls: llmproxycontract.VideoTranslationControls{VideoProcessingControls: llmproxycontract.VideoProcessingControls{Mode: "speed"}, OutputLanguages: []string{"French", "English"}}})
			case "avatar.create":
				result, err = client.CreateAvatar(ctx, "intent", AvatarCreationInput{Provider: "heygen", Source: llmproxycontract.AvatarCreationSource{Name: "Character", ImageAssetID: "image"}})
			case "avatar.video.generate":
				result, err = client.CreateAvatarVideo(ctx, "intent", AvatarVideoInput{Provider: "heygen", Source: llmproxycontract.AvatarVideoSource{AvatarID: "avatar", AudioAssetID: "audio"}, Controls: llmproxycontract.AvatarVideoControls{Engine: "avatar_iv", AspectRatio: "16:9", Resolution: "1080p"}})
			}
			if err != nil || result.Capability != capability {
				t.Fatalf("typed operation=%+v error=%v", result, err)
			}
		})
	}
}

func TestProviderServicesClientRejectsNonFiniteVideoControls(t *testing.T) {
	value := math.NaN()
	client := mediaOperationTestClient(mediaOperationDoer(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid controls caused network dispatch")
		return nil, nil
	}))
	_, err := client.CreateVideoLipSync(context.Background(), "intent", VideoLipSyncInput{Provider: "heygen", Controls: llmproxycontract.VideoLipSyncControls{VideoProcessingControls: llmproxycontract.VideoProcessingControls{Mode: "speed", StartTime: &value}}})
	if !errors.Is(err, ErrInvalidClientRequest) {
		t.Fatalf("non-finite controls error=%v", err)
	}
}
