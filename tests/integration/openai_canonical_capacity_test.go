package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gopkg.in/yaml.v3"
)

// This uses the checked-in origin allocation unchanged. Only the upstream
// protocol transport and managed test account are injected.
func TestIntegrationCanonicalOpenAIUpstreamCapacityStructuredBackground(t *testing.T) {
	document, err := os.ReadFile("../../configs/config.yml")
	if err != nil {
		t.Fatal(err)
	}
	var canonical struct {
		Server struct {
			Capacity proxy.UpstreamCapacityConfiguration `yaml:"upstream_capacity"`
		} `yaml:"server"`
	}
	if err := yaml.Unmarshal(document, &canonical); err != nil {
		t.Fatal(err)
	}
	transport := &canonicalCapacityTransport{createRelease: make(chan struct{}), pollRelease: make(chan struct{}), creates: make(chan struct{}, 32), polls: make(chan struct{}, 32)}
	var createOnce, pollOnce sync.Once
	releaseCreate := func() { createOnce.Do(func() { close(transport.createRelease) }) }
	releasePoll := func() { pollOnce.Do(func() { close(transport.pollRelease) }) }
	release := func() { releaseCreate(); releasePoll() }
	defer release()
	original := proxy.HTTPClient
	proxy.HTTPClient = &http.Client{Transport: transport}
	t.Cleanup(func() { proxy.HTTPClient = original })
	core, logs := observer.New(zap.InfoLevel)
	signals := make(chan struct{}, 4096)
	logger := zap.New(core, zap.Hooks(func(entry zapcore.Entry) error {
		if entry.Message == "upstream HTTP admission" {
			signals <- struct{}{}
		}
		return nil
	}))
	tenant := testfixtures.StandardManagedTenant(serviceSecretValue)
	tenant.ProviderKeys = map[string]string{proxy.ProviderNameOpenAI: openAIKeyValue}
	tenant.ProviderFields = nil
	configuration := integrationConfiguration(t, proxy.Configuration{UpstreamCapacity: canonical.Server.Capacity, AssetStorePath: t.TempDir(), AssetRetentionSeconds: 3600, RequestTimeoutSeconds: 15})
	router, err := testfixtures.BuildManagedRouter(t, configuration, logger.Sugar(), tenant)
	if err != nil {
		t.Fatalf("canonical capacity validation: %v", err)
	}
	logs.TakeAll()
	application := httptest.NewServer(router)
	defer application.Close()
	defer release()
	type result struct {
		status int
		body   string
		err    error
	}
	results := make(chan result, 32)
	submit := func(index int) result {
		payload := `{"model":"gpt-4.1","messages":[{"role":"user","content":"capacity"}],"structured_output":{"schema":{"type":"object","additionalProperties":false,"required":["decision"],"properties":{"decision":{"type":"string","enum":["pass"]}}}}}`
		request, err := http.NewRequest(http.MethodPost, application.URL+"/v2?provider=openai&key="+serviceSecretValue, bytes.NewBufferString(payload))
		if err != nil {
			return result{err: err}
		}
		request.Header.Set("Content-Type", contentTypeJSON)
		request.Header.Set(llmproxycontract.HeaderIdempotencyKey, fmt.Sprintf("canonical-capacity-%d", index))
		response, err := application.Client().Do(request)
		if err != nil {
			return result{err: err}
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		return result{response.StatusCode, string(body), err}
	}
	for index := 0; index < 32; index++ {
		go func(index int) { results <- submit(index) }(index)
	}
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		admitted, rejected := 0, 0
		for _, event := range logs.FilterMessage("upstream HTTP admission").All() {
			switch event.ContextMap()["decision"] {
			case "admitted":
				admitted++
			case "rejected":
				rejected++
			}
		}
		if rejected > 0 {
			t.Fatalf("canonical OpenAI allocation rejected a 32-request cohort: admitted=%d rejected=%d", admitted, rejected)
		}
		if admitted == 32 {
			break
		}
		select {
		case <-signals:
		case result := <-results:
			t.Fatalf("cohort returned before body release: %+v", result)
		case <-deadline.C:
			t.Fatalf("cohort admission stalled: admitted=%d", admitted)
		}
	}
	waitBodies := func(entered <-chan struct{}, phase string) {
		for index := 0; index < 8; index++ {
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatalf("%s did not retain eight active response bodies", phase)
			}
		}
	}
	waitBodies(transport.creates, "background create")
	// All eight bodies remain unread, so the remaining admitted operations are
	// queued rather than dispatching through an early permit release.
	if got := transport.active.Load(); got != 8 {
		t.Fatalf("held create bodies=%d want=8", got)
	}
	excess := submit(32)
	if excess.err != nil || excess.status != http.StatusServiceUnavailable {
		t.Fatalf("excess request: %+v", excess)
	}
	releaseCreate()
	waitBodies(transport.polls, "background poll")
	if got := transport.active.Load(); got != 8 {
		t.Fatalf("held poll bodies=%d want=8", got)
	}
	releasePoll()
	for index := 0; index < 32; index++ {
		select {
		case result := <-results:
			if result.err != nil || result.status != http.StatusOK || result.body != `{"decision":"pass"}` {
				t.Errorf("structured result: %+v", result)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("accepted cohort did not drain")
		}
	}
	if transport.createCount.Load() != 32 || transport.pollCount.Load() != 32 || transport.maximum.Load() != 8 || transport.active.Load() != 0 {
		t.Fatalf("upstream create=%d poll=%d peak=%d active=%d", transport.createCount.Load(), transport.pollCount.Load(), transport.maximum.Load(), transport.active.Load())
	}
	admitted, rejected := 0, 0
	sawSaturatedQueue := false
	for _, event := range logs.FilterMessage("upstream HTTP admission").All() {
		fields := event.ContextMap()
		switch fields["decision"] {
		case "admitted":
			admitted++
		case "rejected":
			rejected++
		}
		if fields["origin_active"] == int64(8) && fields["origin_queued"] == int64(24) {
			sawSaturatedQueue = true
		}
		if fields["origin_active"].(int64) > 8 || fields["origin_queued"].(int64) > 24 {
			t.Fatalf("active/queued capacity exceeded: %v", fields)
		}
	}
	if admitted != 64 || rejected != 1 || !sawSaturatedQueue {
		t.Fatalf("admission creates+polls=%d rejects=%d saturated_queue=%v", admitted, rejected, sawSaturatedQueue)
	}
}

type canonicalCapacityTransport struct {
	createRelease, pollRelease              chan struct{}
	creates, polls                          chan struct{}
	active, maximum, createCount, pollCount atomic.Int64
}

func (transport *canonicalCapacityTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Host != "api.openai.com" || (request.Method != http.MethodPost && request.Method != http.MethodGet) {
		return nil, fmt.Errorf("unexpected upstream route: %s %s", request.Method, request.URL)
	}
	if request.Method == http.MethodPost {
		var payload struct {
			Background bool `json:"background"`
			Text       struct {
				Format struct {
					Type   string `json:"type"`
					Strict bool   `json:"strict"`
				} `json:"format"`
			} `json:"text"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			return nil, err
		}
		if !payload.Background || payload.Text.Format.Type != "json_schema" || !payload.Text.Format.Strict {
			return nil, fmt.Errorf("create lacks background/strict schema: %+v", payload)
		}
	}
	active := transport.active.Add(1)
	for maximum := transport.maximum.Load(); active > maximum; maximum = transport.maximum.Load() {
		if transport.maximum.CompareAndSwap(maximum, active) {
			break
		}
	}
	var gate <-chan struct{}
	var payload string
	if request.Method == http.MethodPost {
		identifier := transport.createCount.Add(1)
		gate = transport.createRelease
		payload = fmt.Sprintf(`{"id":"canonical-%d","status":"queued","background":true}`, identifier)
		transport.creates <- struct{}{}
	} else {
		transport.pollCount.Add(1)
		gate = transport.pollRelease
		payload = `{"id":"canonical","status":"completed","output_text":"{\"decision\":\"pass\"}"}`
		transport.polls <- struct{}{}
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentTypeJSON}}, Body: &canonicalCapacityBody{reader: strings.NewReader(payload), gate: gate, ctx: request.Context(), close: func() { transport.active.Add(-1) }}}, nil
}

type canonicalCapacityBody struct {
	reader *strings.Reader
	gate   <-chan struct{}
	ctx    context.Context
	once   sync.Once
	close  func()
}

func (body *canonicalCapacityBody) Read(buffer []byte) (int, error) {
	select {
	case <-body.gate:
		return body.reader.Read(buffer)
	case <-body.ctx.Done():
		return 0, body.ctx.Err()
	}
}
func (body *canonicalCapacityBody) Close() error { body.once.Do(body.close); return nil }
