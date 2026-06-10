package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLocalizedAPIResponseMessageSub2APITokenError(t *testing.T) {
	body := []byte(`{"code":"INVALID_TOKEN","message":"Invalid token"}`)
	got := localizedAPIResponseMessage(http.StatusUnauthorized, body)
	if got != "令牌无效" {
		t.Fatalf("localizedAPIResponseMessage() = %q, want 令牌无效", got)
	}
}

func TestNewAPIHTTPErrorLocalizesOpenAIErrorShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "Invalid URL (POST /api/token/4173/key)",
				"type":    "invalid_request_error",
			},
		})
	}))
	defer server.Close()

	client, err := NewNewAPIClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = client.FetchPricing(context.Background(), 1, "token")
	if err == nil {
		t.Fatal("FetchPricing() error = nil, want localized error")
	}
	text := err.Error()
	for _, want := range []string{"NewAPI 上游", "返回 HTTP 404", "接口地址无效"} {
		if !strings.Contains(text, want) {
			t.Fatalf("error = %q, want %q", text, want)
		}
	}
	if strings.Contains(text, "Invalid URL") {
		t.Fatalf("error = %q, should be localized", text)
	}
}

func TestSub2APIHTTPErrorLocalizesAdminTokenError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    "INVALID_TOKEN",
			"message": "Invalid token",
		})
	}))
	defer server.Close()

	client, err := NewSub2APIAdminClient(server.URL, "bad-admin-key")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.listGroups(context.Background())
	if err == nil {
		t.Fatal("listGroups() error = nil, want localized error")
	}
	text := err.Error()
	for _, want := range []string{"sub2api", "返回 HTTP 401", "令牌无效", "管理员认证失败"} {
		if !strings.Contains(text, want) {
			t.Fatalf("error = %q, want %q", text, want)
		}
	}
	if strings.Contains(text, "Invalid token") {
		t.Fatalf("error = %q, should be localized", text)
	}
}

func TestWriteErrorLocalizesMessage(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeError(recorder, http.StatusUnauthorized, "Invalid token")

	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Error.Message != "令牌无效" {
		t.Fatalf("message = %q, want 令牌无效", payload.Error.Message)
	}
}
