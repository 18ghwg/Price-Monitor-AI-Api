package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewAPIClientCreateAPIKeyForGroup(t *testing.T) {
	var createPayload map[string]any
	searchCount := 0
	var sawKey bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer system-token" {
			t.Fatalf("Authorization = %q, want Bearer system-token", r.Header.Get("Authorization"))
		}
		if r.Header.Get("New-Api-User") != "99" {
			t.Fatalf("New-Api-User = %q, want 99", r.Header.Get("New-Api-User"))
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/token":
			if err := json.NewDecoder(r.Body).Decode(&createPayload); err != nil {
				t.Fatalf("decode create payload: %v", err)
			}
			writeNewAPITestJSON(w, nil)
		case r.Method == http.MethodGet && r.URL.Path == "/api/token/search":
			searchCount++
			if got := r.URL.Query().Get("keyword"); got != "pm-token" {
				t.Fatalf("keyword = %q, want pm-token", got)
			}
			items := []map[string]any{}
			if searchCount > 1 {
				items = []map[string]any{{"id": 123, "name": "pm-token", "group": "cheap-group"}}
			}
			writeNewAPITestJSON(w, map[string]any{
				"items": items,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/token/":
			writeNewAPITestJSON(w, map[string]any{
				"items": []map[string]any{},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/token/123/key":
			sawKey = true
			writeNewAPITestJSON(w, map[string]any{"key": "raw-key"})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewNewAPIClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	key, err := client.CreateAPIKeyForGroup(context.Background(), 99, "system-token", "pm-token", "cheap-group")
	if err != nil {
		t.Fatalf("CreateAPIKeyForGroup() error = %v", err)
	}
	if key != "sk-raw-key" {
		t.Fatalf("key = %q, want sk-raw-key", key)
	}
	if createPayload["group"] != "cheap-group" {
		t.Fatalf("group = %v, want cheap-group", createPayload["group"])
	}
	if createPayload["unlimited_quota"] != true {
		t.Fatalf("unlimited_quota = %v, want true", createPayload["unlimited_quota"])
	}
	if searchCount != 2 || !sawKey {
		t.Fatalf("searchCount=%v sawKey=%v, want two searches and key fetch", searchCount, sawKey)
	}
}

func TestNewAPIClientVerifySystemAccessTokenUsesProvidedUserID(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.URL.Path != "/api/user/self" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer system-token" {
			t.Fatalf("Authorization = %q, want Bearer system-token", r.Header.Get("Authorization"))
		}
		if r.Header.Get("New-Api-User") != "88" {
			t.Fatalf("New-Api-User = %q, want 88", r.Header.Get("New-Api-User"))
		}
		writeNewAPITestJSON(w, map[string]any{
			"id":       88,
			"username": "ghwg",
		})
	}))
	defer server.Close()

	client, err := NewNewAPIClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	userID, username, err := client.VerifySystemAccessToken(context.Background(), 88, "system-token")
	if err != nil {
		t.Fatalf("VerifySystemAccessToken() error = %v", err)
	}
	if userID != 88 || username != "ghwg" {
		t.Fatalf("userID=%d username=%q, want 88 ghwg", userID, username)
	}
	if requestCount != 1 {
		t.Fatalf("requestCount = %d, want 1", requestCount)
	}
}

func TestNewAPIClientEnsureAPIKeyForGroupReusesExistingToken(t *testing.T) {
	var sawUpdate bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/token/search":
			writeNewAPITestJSON(w, map[string]any{
				"items": []map[string]any{{"id": 123, "name": "pm-token", "group": "cheap-group"}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/token/123/key":
			writeNewAPITestJSON(w, map[string]any{"key": "existing-key"})
		case r.Method == http.MethodPut && r.URL.Path == "/api/token/":
			sawUpdate = true
			writeNewAPITestJSON(w, nil)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewNewAPIClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	key, action, err := client.EnsureAPIKeyForGroup(context.Background(), 99, "system-token", "pm-token", "cheap-group")
	if err != nil {
		t.Fatalf("EnsureAPIKeyForGroup() error = %v", err)
	}
	if action != "reused" {
		t.Fatalf("action = %q, want reused", action)
	}
	if key != "sk-existing-key" {
		t.Fatalf("key = %q, want sk-existing-key", key)
	}
	if sawUpdate {
		t.Fatalf("saw update for token already in target group")
	}
}

func TestNewAPIClientEnsureAPIKeyForGroupUpdatesExistingTokenGroup(t *testing.T) {
	var updatePayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/token/search":
			writeNewAPITestJSON(w, map[string]any{
				"items": []map[string]any{{"id": 123, "name": "pm-token", "group": "default"}},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/token/":
			if err := json.NewDecoder(r.Body).Decode(&updatePayload); err != nil {
				t.Fatalf("decode update payload: %v", err)
			}
			writeNewAPITestJSON(w, nil)
		case r.Method == http.MethodPost && r.URL.Path == "/api/token/123/key":
			writeNewAPITestJSON(w, map[string]any{"key": "updated-key"})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewNewAPIClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	key, action, err := client.EnsureAPIKeyForGroup(context.Background(), 99, "system-token", "pm-token", "cheap-group")
	if err != nil {
		t.Fatalf("EnsureAPIKeyForGroup() error = %v", err)
	}
	if action != "updated" {
		t.Fatalf("action = %q, want updated", action)
	}
	if key != "sk-updated-key" {
		t.Fatalf("key = %q, want sk-updated-key", key)
	}
	if updatePayload["id"] != float64(123) {
		t.Fatalf("id = %v, want 123", updatePayload["id"])
	}
	if updatePayload["group"] != "cheap-group" {
		t.Fatalf("group = %v, want cheap-group", updatePayload["group"])
	}
	if updatePayload["unlimited_quota"] != true {
		t.Fatalf("unlimited_quota = %v, want true", updatePayload["unlimited_quota"])
	}
}

func TestNewAPIClientEnsureAPIKeyForGroupFallsBackToBatchKey(t *testing.T) {
	var sawBatch bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/token/search":
			writeNewAPITestJSON(w, map[string]any{
				"items": []map[string]any{{"id": 123, "name": "pm-token", "group": "cheap-group"}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/token/123/key":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"Invalid URL (POST /api/token/123/key)"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/token/batch/keys":
			sawBatch = true
			writeNewAPITestJSON(w, map[string]any{"keys": map[string]string{"123": "batch-key"}})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewNewAPIClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	key, action, err := client.EnsureAPIKeyForGroup(context.Background(), 99, "system-token", "pm-token", "cheap-group")
	if err != nil {
		t.Fatalf("EnsureAPIKeyForGroup() error = %v", err)
	}
	if action != "reused" {
		t.Fatalf("action = %q, want reused", action)
	}
	if key != "sk-batch-key" {
		t.Fatalf("key = %q, want sk-batch-key", key)
	}
	if !sawBatch {
		t.Fatalf("batch key fallback was not called")
	}
}

func TestNewAPIClientEnsureAPIKeyForGroupFindsTokenFromList(t *testing.T) {
	var sawList bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/token/search":
			writeNewAPITestJSON(w, map[string]any{"items": []map[string]any{}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/token/" && r.URL.Query().Get("page_size") == "100":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"success":false,"message":"not found"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/token/" && r.URL.Query().Get("size") == "100":
			sawList = true
			writeNewAPITestJSON(w, map[string]any{
				"items": []map[string]any{{
					"id":    123,
					"name":  "pm-token",
					"group": "cheap-group",
					"key":   "listed-key",
				}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/token/123/key":
			writeNewAPITestJSON(w, map[string]any{"key": "existing-key"})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewNewAPIClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	key, action, err := client.EnsureAPIKeyForGroup(context.Background(), 99, "system-token", "pm-token", "cheap-group")
	if err != nil {
		t.Fatalf("EnsureAPIKeyForGroup() error = %v", err)
	}
	if action != "reused" {
		t.Fatalf("action = %q, want reused", action)
	}
	if key != "sk-existing-key" {
		t.Fatalf("key = %q, want sk-existing-key", key)
	}
	if !sawList {
		t.Fatalf("token list fallback was not called")
	}
}

func TestNewAPIClientEnsureAPIKeyForGroupFallsBackToListedKey(t *testing.T) {
	var sawList bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/token/search":
			writeNewAPITestJSON(w, map[string]any{
				"items": []map[string]any{{"id": 4173, "name": "pm-r56-claude-opus-4-8", "group": "Claude 逆向A1"}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/token/4173/key":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"Invalid URL (POST /api/token/4173/key)"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/token/batch/keys":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"success":false,"message":"not found"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/token/" && r.URL.Query().Get("page_size") == "100":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"success":false,"message":"not found"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/token/" && r.URL.Query().Get("size") == "100":
			sawList = true
			writeNewAPITestJSON(w, map[string]any{
				"page":      1,
				"page_size": 10,
				"total":     1,
				"items": []map[string]any{{
					"id":              4173,
					"name":            "pm-r56-claude-opus-4-8",
					"group":           "Claude 逆向A1",
					"key":             "l37GOqyWhbYk2rRyV9fngJm8VMoy6BDget6hXnpWBk7mv7h3",
					"unlimited_quota": true,
				}},
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewNewAPIClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	key, action, err := client.EnsureAPIKeyForGroup(context.Background(), 99, "system-token", "pm-r56-claude-opus-4-8", "Claude 逆向A1")
	if err != nil {
		t.Fatalf("EnsureAPIKeyForGroup() error = %v", err)
	}
	if action != "reused" {
		t.Fatalf("action = %q, want reused", action)
	}
	want := "sk-l37GOqyWhbYk2rRyV9fngJm8VMoy6BDget6hXnpWBk7mv7h3"
	if key != want {
		t.Fatalf("key = %q, want %q", key, want)
	}
	if !sawList {
		t.Fatalf("listed key fallback was not called")
	}
}

func TestNewAPIClientFetchBalance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.Path != "/api/user/self" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer system-token" {
			t.Fatalf("Authorization = %q, want Bearer system-token", r.Header.Get("Authorization"))
		}
		writeNewAPITestJSON(w, map[string]any{"quota": 12345})
	}))
	defer server.Close()

	client, err := NewNewAPIClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	balance, err := client.FetchBalance(context.Background(), 99, "system-token")
	if err != nil {
		t.Fatalf("FetchBalance() error = %v", err)
	}
	want := 12345.0 / newAPIQuotaPerUSD
	if balance.Unit != "usd" || balance.Value == nil || *balance.Value != want {
		t.Fatalf("balance = %+v, want %v usd", balance, want)
	}
}

func TestNewAPIClientFetchRechargeStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer system-token" {
			t.Fatalf("Authorization = %q, want Bearer system-token", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/user/topup/info":
			writeNewAPITestJSON(w, map[string]any{
				"enable_online_topup": true,
				"min_topup":           10,
				"amount_options":      []int{10, 100},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/user/topup/self":
			writeNewAPITestJSON(w, map[string]any{"items": []map[string]any{}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/user/amount":
			var payload map[string]float64
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode amount payload: %v", err)
			}
			writeNewAPITestJSON(w, payload["amount"]/10)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewNewAPIClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	status, err := client.FetchRechargeStatus(context.Background(), 99, "system-token")
	if err != nil {
		t.Fatalf("FetchRechargeStatus() error = %v", err)
	}
	if !status.Enabled {
		t.Fatalf("Enabled = false, want true")
	}
	if status.Multiplier == nil || *status.Multiplier != 10 {
		t.Fatalf("Multiplier = %v, want 10", status.Multiplier)
	}
}

func TestNewAPIClientEnsureDailyCheckinSignsWhenNeeded(t *testing.T) {
	var sawPost bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer system-token" {
			t.Fatalf("Authorization = %q, want Bearer system-token", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/user/checkin":
			if got := r.URL.Query().Get("month"); got != "2026-06" {
				t.Fatalf("month = %q, want 2026-06", got)
			}
			writeNewAPITestJSON(w, map[string]any{
				"enabled": true,
				"stats": map[string]any{
					"checked_in_today": false,
					"records":          []map[string]any{},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/user/checkin":
			sawPost = true
			writeNewAPITestJSON(w, map[string]any{
				"quota_awarded": 250000,
				"checkin_date":  "2026-06-08",
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewNewAPIClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.EnsureDailyCheckin(context.Background(), 99, "system-token", time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("EnsureDailyCheckin() error = %v", err)
	}
	if !sawPost {
		t.Fatalf("expected checkin POST")
	}
	if !result.Enabled || result.Status != "signed" {
		t.Fatalf("result status = %+v, want enabled signed", result)
	}
	wantReward := 250000.0 / newAPIQuotaPerUSD
	if result.Reward == nil || *result.Reward != wantReward || result.Unit != "usd" {
		t.Fatalf("reward = %+v %s, want %v usd", result.Reward, result.Unit, wantReward)
	}
}

func TestNewAPIClientEnsureDailyCheckinDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "签到功能未启用",
		})
	}))
	defer server.Close()

	client, err := NewNewAPIClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.EnsureDailyCheckin(context.Background(), 99, "system-token", time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("EnsureDailyCheckin() error = %v", err)
	}
	if result.Enabled || result.Status != "disabled" {
		t.Fatalf("result = %+v, want disabled", result)
	}
}

func writeNewAPITestJSON(w http.ResponseWriter, data any) {
	raw, _ := json.Marshal(data)
	if data == nil {
		raw = []byte("null")
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "",
		"data":    json.RawMessage(raw),
	})
}
