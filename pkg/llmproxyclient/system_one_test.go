package llmproxyclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSystemOnePreservesWireContract(t *testing.T) {
	for _, scenario := range []struct {
		name, contentType, encoding string
		body                        []byte
		status                      int
		response                    string
	}{
		{"json", "application/json", "", []byte(`{"model":"jev-1","state":"Evaluate","questions":{"one":{"type":"score","instructions":null,"criteria":["Only level"]}},"unknown":null}`), 200, `{"model":"jev-1","answers":{"one":{"type":"score","score":0,"confidence":1,"legend":{"0":"Only level"},"probabilities":{"0":1},"unknown":null}},"usage":{"input_tokens":1,"output_tokens":1,"future":7},"unknown":null}`},
		{"multipart", "multipart/form-data; boundary=image", "", []byte("--image\r\nContent-Disposition: form-data; name=\"image\"; filename=\"a.png\"\r\nContent-Type: image/png\r\n\r\nimage-bytes\r\n--image--\r\n"), 200, `{"answers":{},"unknown":null}`},
		{"gzip", "application/json", "gzip", []byte{31, 139, 8, 0, 1}, 200, `{"answers":{}}`},
		{"zstd", "application/json", "zstd", []byte{40, 181, 47, 253, 1}, 200, `{"answers":{}}`},
		{"provider error", "application/json", "", []byte(`{"model":"jev-1","questions":[]}`), 400, `{"detail":{"message":"provider detail","unknown":null}}`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if r.Method != "POST" || r.URL.Path != "/v1/systemone" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "Bearer tenant-key" || r.Header.Get("Content-Type") != scenario.contentType || r.Header.Get("Content-Encoding") != scenario.encoding || r.Header.Get("TypeSafe-Danger-Zone-Images") != "true" || !bytes.Equal(body, scenario.body) || r.Header.Get("Idempotency-Key") != "" {
					t.Errorf("request=%s %s headers=%v body=%q", r.Method, r.URL, r.Header, body)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Request-ID", "upstream-request")
				w.WriteHeader(scenario.status)
				_, _ = io.WriteString(w, scenario.response)
			}))
			defer server.Close()
			config, err := NewConfig(ConfigInput{BaseURL: server.URL + "/v2?provider=obsolete&key=old", Secret: "tenant-key", Provider: "other"})
			if err != nil {
				t.Fatal(err)
			}
			client, err := NewClient(config, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.PostSystemOne(context.Background(), scenario.body, SystemOneRequestOptions{ContentType: scenario.contentType, ContentEncoding: scenario.encoding, Headers: http.Header{"TypeSafe-Danger-Zone-Images": []string{"true"}}})
			if scenario.status >= 300 {
				if !errors.Is(err, ErrClientHTTPFailure) {
					t.Fatalf("error=%v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if result.StatusCode != scenario.status || string(result.Body) != scenario.response || result.Headers.Get("X-Request-ID") != "upstream-request" {
				t.Fatalf("response=%+v", result)
			}
		})
	}
}

func TestSystemOnePublicCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != PublicCapabilitiesPath || r.Header.Get("Authorization") != "" {
			t.Errorf("request=%s %s", r.Method, r.URL)
		}
		_, _ = io.WriteString(w, `{"offerings":[{"identifier":"typesafe:jev-1.14.0","provider":"typesafe","model":"jev-1.14.0","capabilities":["image_input","typed_decision"],"wire_contract":"system_one","execution_lifecycle":"synchronous_completion","media_limits":[{"id":"inline_request_bytes","media_type":"all","transport":"inline","status":"unknown","value":null,"unit":"bytes","scope":"request_encoded_bytes","source":"https://api.typesafe.ai/openapi.json","last_verified":"2026-11-11"},{"id":"image_count","media_type":"image","transport":"any","status":"unknown","value":null,"unit":"files","scope":"request","source":"https://api.typesafe.ai/openapi.json","last_verified":"2026-11-11"},{"id":"image_inline_bytes","media_type":"image","transport":"inline","status":"unknown","value":null,"unit":"bytes","scope":"attachment_encoded_bytes","source":"https://api.typesafe.ai/openapi.json","last_verified":"2026-11-11"},{"id":"image_file_bytes","media_type":"image","transport":"file","status":"unknown","value":null,"unit":"bytes","scope":"attachment","source":"https://api.typesafe.ai/openapi.json","last_verified":"2026-11-11"}],"media_execution_lifecycle":"synchronous_completion","image_mime_types":["image/png","image/jpeg"]}]}`)
	}))
	defer server.Close()
	config, err := NewConfig(ConfigInput{BaseURL: server.URL, Secret: "tenant-key"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := client.GetPublicCapabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Offerings) != 1 || catalog.Offerings[0].Model != "jev-1.14.0" || catalog.Offerings[0].MediaExecutionLifecycle != "synchronous_completion" || len(catalog.Offerings[0].ImageMIMETypes) != 2 {
		t.Fatalf("catalog=%+v", catalog)
	}
}

func TestSystemOneBoundaryFailures(t *testing.T) {
	config, err := NewConfig(ConfigInput{BaseURL: "http://example.test", Secret: "tenant-key"})
	if err != nil {
		t.Fatal(err)
	}
	transportFailure := errors.New("transport interrupted")
	client, err := NewClient(config, mediaOperationDoer(func(*http.Request) (*http.Response, error) { return nil, transportFailure }))
	if err != nil {
		t.Fatal(err)
	}
	var missingContext context.Context
	if _, err := client.PostSystemOne(missingContext, []byte(`{}`), SystemOneRequestOptions{}); !errors.Is(err, ErrInvalidClientRequest) {
		t.Fatalf("request error=%v", err)
	}
	if _, err := client.PostSystemOne(context.Background(), []byte(`{}`), SystemOneRequestOptions{}); !errors.Is(err, ErrClientHTTPFailure) || !errors.Is(err, transportFailure) {
		t.Fatalf("transport error=%v", err)
	}
	client, err = NewClient(config, mediaOperationDoer(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content type=%q", request.Header.Get("Content-Type"))
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"X-Request-Id": []string{"partial"}}, Body: io.NopCloser(mediaOperationErrorReader{})}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.PostSystemOne(context.Background(), []byte(`{}`), SystemOneRequestOptions{})
	if !errors.Is(err, ErrClientHTTPFailure) || result.StatusCode != 200 || result.Headers.Get("X-Request-ID") != "partial" {
		t.Fatalf("response=%+v error=%v", result, err)
	}
}

func TestSystemOneTenantAuthenticationCannotBeOverridden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if values := r.Header.Values("Authorization"); len(values) != 1 || values[0] != "Bearer tenant-key" {
			t.Errorf("authorization=%v", values)
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	config, err := NewConfig(ConfigInput{BaseURL: server.URL, Secret: "tenant-key"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	headers := http.Header{"authorization": []string{"Bearer override"}}
	if _, err := client.PostSystemOne(context.Background(), []byte(`{}`), SystemOneRequestOptions{Headers: headers}); err != nil {
		t.Fatal(err)
	}
	if map[string][]string(headers)["authorization"][0] != "Bearer override" {
		t.Fatal("caller headers mutated")
	}
}
