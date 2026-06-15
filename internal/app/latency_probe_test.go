package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeModelRequestLatencyUsesChatCompletions(t *testing.T) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("authorization = %q, want Bearer sk-test", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if got := payload["model"]; got != "gpt-5.5" {
			t.Fatalf("model = %v, want gpt-5.5", got)
		}
		if got := payload["stream"]; got != false {
			t.Fatalf("stream = %v, want false", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","choices":[{"index":0,"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()

	latency, err := probeModelRequestLatency(context.Background(), server.URL, "sk-test", "gpt-5.5")
	if err != nil {
		t.Fatalf("probeModelRequestLatency() error = %v", err)
	}
	if latency < 0 {
		t.Fatalf("latency = %v, want non-negative", latency)
	}
}
