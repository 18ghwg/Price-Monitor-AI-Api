package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchModelProbeOpenAICompatible(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %s, want /v1/models", r.URL.Path)
		}
		authHeader = r.Header.Get("Authorization")
		writeModelProbeTestJSON(t, w, map[string]any{
			"data": []map[string]any{
				{"id": "gpt-5.5", "object": "model", "owned_by": "openai", "created": 1},
				{"id": "claude-opus-4-8", "object": "model", "owned_by": "anthropic", "created": 2},
			},
		})
	}))
	defer server.Close()

	result, err := FetchModelProbe(context.Background(), ModelProbeInput{
		APIType: "openai_compatible",
		BaseURL: server.URL,
		APIKey:  "sk-test",
	})
	if err != nil {
		t.Fatalf("FetchModelProbe() error = %v", err)
	}
	if authHeader != "Bearer sk-test" {
		t.Fatalf("Authorization = %q, want Bearer sk-test", authHeader)
	}
	if result.Count != 2 || len(result.Models) != 2 {
		t.Fatalf("Count = %d len = %d, want 2", result.Count, len(result.Models))
	}
	if result.Models[0].ID != "claude-opus-4-8" || result.Models[1].ID != "gpt-5.5" {
		t.Fatalf("models sorted = %+v", result.Models)
	}
}

func TestJoinProbeURLAvoidsDoubleV1(t *testing.T) {
	got, err := joinProbeURL("https://api.example.com/v1", "/v1/models")
	if err != nil {
		t.Fatalf("joinProbeURL() error = %v", err)
	}
	if got != "https://api.example.com/v1/models" {
		t.Fatalf("joinProbeURL() = %q", got)
	}
	got, err = joinProbeURL("https://api.example.com/v1", "/v1/models?limit=200")
	if err != nil {
		t.Fatalf("joinProbeURL() query error = %v", err)
	}
	if got != "https://api.example.com/v1/models?limit=200" {
		t.Fatalf("joinProbeURL() query = %q", got)
	}
}

func TestFetchModelProbeAnthropicPagination(t *testing.T) {
	var pages int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "ak-test" {
			t.Fatalf("x-api-key = %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Fatalf("anthropic-version = %q", r.Header.Get("anthropic-version"))
		}
		pages++
		if pages == 1 {
			writeModelProbeTestJSON(t, w, map[string]any{
				"data": []map[string]any{
					{"id": "claude-sonnet-4-6", "display_name": "Sonnet"},
				},
				"has_more": true,
				"last_id":  "claude-sonnet-4-6",
			})
			return
		}
		if r.URL.Query().Get("after_id") != "claude-sonnet-4-6" {
			t.Fatalf("after_id = %q", r.URL.Query().Get("after_id"))
		}
		writeModelProbeTestJSON(t, w, map[string]any{
			"data": []map[string]any{
				{"id": "claude-opus-4-8", "display_name": "Opus"},
			},
			"has_more": false,
		})
	}))
	defer server.Close()

	result, err := FetchModelProbe(context.Background(), ModelProbeInput{
		APIType: "anthropic",
		BaseURL: server.URL,
		APIKey:  "ak-test",
	})
	if err != nil {
		t.Fatalf("FetchModelProbe() error = %v", err)
	}
	if pages != 2 {
		t.Fatalf("pages = %d, want 2", pages)
	}
	if result.Count != 2 {
		t.Fatalf("Count = %d, want 2", result.Count)
	}
	if result.Models[0].ID != "claude-opus-4-8" || result.Models[1].ID != "claude-sonnet-4-6" {
		t.Fatalf("models sorted = %+v", result.Models)
	}
}

func writeModelProbeTestJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
