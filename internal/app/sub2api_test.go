package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSub2APIUpsertUpdatesExistingAccountByAPIURLAndGroup(t *testing.T) {
	var updatePayload map[string]any
	var schedulablePayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts":
			if got := r.URL.Query().Get("group"); got != "" {
				t.Fatalf("group query = %q, want empty for apiurl match", got)
			}
			if got := r.URL.Query().Get("search"); got != "" {
				t.Fatalf("search query = %q, want empty because matching uses credentials.base_url", got)
			}
			writeSub2TestJSON(w, map[string]any{
				"items": []map[string]any{
					{
						"id":          41,
						"name":        "wrong category account",
						"platform":    "openai",
						"type":        "apikey",
						"credentials": map[string]any{"base_url": "https://newapi.test/v1"},
						"group_ids":   []int64{9},
						"status":      "active",
					},
					{
						"id":          42,
						"name":        "old",
						"platform":    "openai",
						"type":        "apikey",
						"credentials": map[string]any{"base_url": "https://newapi.test"},
						"group_ids":   []int64{7},
						"status":      "inactive",
					},
				},
				"total": 1,
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/admin/accounts/41":
			writeSub2TestJSON(w, map[string]any{
				"id":          41,
				"platform":    "openai",
				"type":        "apikey",
				"credentials": map[string]any{"base_url": "https://newapi.test/v1"},
				"group_ids":   []int64{9},
				"status":      "inactive",
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/admin/accounts/42":
			if err := json.NewDecoder(r.Body).Decode(&updatePayload); err != nil {
				t.Fatalf("decode update payload: %v", err)
			}
			writeSub2TestJSON(w, map[string]any{
				"id":          42,
				"name":        updatePayload["name"],
				"platform":    "openai",
				"type":        "apikey",
				"credentials": updatePayload["credentials"],
				"group_ids":   []int64{7},
				"status":      "active",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts/42/schedulable":
			if err := json.NewDecoder(r.Body).Decode(&schedulablePayload); err != nil {
				t.Fatalf("decode schedulable payload: %v", err)
			}
			writeSub2TestJSON(w, map[string]any{
				"id":            42,
				"name":          "new name",
				"platform":      "openai",
				"type":          "apikey",
				"credentials":   map[string]any{"base_url": "https://newapi.test"},
				"group_ids":     []int64{7},
				"status":        "active",
				"schedulable":   true,
				"rate_limited":  false,
				"error_message": "",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts/41/schedulable":
			writeSub2TestJSON(w, map[string]any{"id": 41, "schedulable": false})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	_, action, err := client.UpsertOpenAIAPIKeyAccount(context.Background(), "new name", "https://newapi.test", "sk-new", sub2Group{ID: 7, Name: "Codex"})
	if err != nil {
		t.Fatalf("UpsertOpenAIAPIKeyAccount() error = %v", err)
	}
	if action != "updated" {
		t.Fatalf("action = %q, want updated", action)
	}
	credentials, _ := updatePayload["credentials"].(map[string]any)
	if credentials["api_key"] != "sk-new" {
		t.Fatalf("api_key = %v, want sk-new", credentials["api_key"])
	}
	if updatePayload["status"] != "active" {
		t.Fatalf("status = %v, want active", updatePayload["status"])
	}
	if updatePayload["priority"] == nil {
		t.Fatalf("priority missing from update payload")
	}
	if _, ok := updatePayload["concurrency"]; ok {
		t.Fatalf("concurrency should not be overwritten on existing account update")
	}
	if _, ok := updatePayload["load_factor"]; ok {
		t.Fatalf("load_factor should not be overwritten on existing account update")
	}
	if schedulablePayload["schedulable"] != true {
		t.Fatalf("schedulable = %v, want true", schedulablePayload["schedulable"])
	}
}

func TestSub2APIUpsertUpdatesSameURLAccountEvenWhenGroupsDiffer(t *testing.T) {
	var updatePayload map[string]any
	requestCount := map[string]int{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		requestCount[r.Method+" "+r.URL.Path]++
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts":
			if got := r.URL.Query().Get("search"); got != "" {
				t.Fatalf("search query = %q, want empty because matching uses credentials.base_url", got)
			}
			writeSub2TestJSON(w, map[string]any{
				"items": []map[string]any{{
					"id":          41,
					"name":        "claude account",
					"platform":    "openai",
					"type":        "apikey",
					"credentials": map[string]any{"base_url": "https://newapi.test"},
					"group_ids":   []int64{9},
					"status":      "active",
				}},
				"total": 1,
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/admin/accounts/41":
			if err := json.NewDecoder(r.Body).Decode(&updatePayload); err != nil {
				t.Fatalf("decode update payload: %v", err)
			}
			writeSub2TestJSON(w, map[string]any{
				"id":          41,
				"name":        updatePayload["name"],
				"platform":    "openai",
				"type":        "apikey",
				"credentials": updatePayload["credentials"],
				"group_ids":   []int64{7},
				"status":      "active",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts/41/schedulable":
			writeSub2TestJSON(w, map[string]any{"id": 41, "schedulable": true})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	_, action, err := client.UpsertOpenAIAPIKeyAccountGroups(context.Background(), "codex account", "https://newapi.test/v1", "sk-new", []sub2Group{{ID: 7, Name: "Codex"}})
	if err != nil {
		t.Fatalf("UpsertOpenAIAPIKeyAccountGroups() error = %v", err)
	}
	if action != "updated" {
		t.Fatalf("action = %q, want updated", action)
	}
	if requestCount[http.MethodPut+" /api/v1/admin/accounts/41"] != 1 {
		t.Fatalf("update account calls = %d, want 1", requestCount[http.MethodPut+" /api/v1/admin/accounts/41"])
	}
	groupIDs, ok := updatePayload["group_ids"].([]any)
	if !ok {
		t.Fatalf("group_ids type = %T, want []any", updatePayload["group_ids"])
	}
	if len(groupIDs) != 1 || groupIDs[0] != float64(7) {
		t.Fatalf("group_ids = %#v, want [7]", groupIDs)
	}
	if requestCount[http.MethodPost+" /api/v1/admin/accounts"] != 0 {
		t.Fatalf("create account calls = %d, want 0", requestCount[http.MethodPost+" /api/v1/admin/accounts"])
	}
}

func TestSub2APIUpsertReportsAlreadyMatchedGroupsForExactGroupReuse(t *testing.T) {
	requestCount := map[string]int{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		requestCount[r.Method+" "+r.URL.Path]++
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts":
			writeSub2TestJSON(w, map[string]any{
				"items": []map[string]any{{
					"id":          42,
					"name":        "existing",
					"platform":    "openai",
					"type":        "apikey",
					"credentials": map[string]any{"base_url": "https://newapi.test"},
					"group_ids":   []int64{7},
					"status":      "active",
					"schedulable": true,
				}},
				"total": 1,
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/admin/accounts/42":
			writeSub2TestJSON(w, map[string]any{
				"id":          42,
				"name":        "existing",
				"platform":    "openai",
				"type":        "apikey",
				"credentials": map[string]any{"base_url": "https://newapi.test"},
				"group_ids":   []int64{7},
				"status":      "active",
				"schedulable": true,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts/42/schedulable":
			writeSub2TestJSON(w, map[string]any{"id": 42, "schedulable": true})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}

	_, action, alreadyMatched, err := client.UpsertAPIKeyAccountGroupsWithRateAndMode(
		context.Background(),
		"openai",
		"existing",
		"https://newapi.test",
		"sk-new",
		[]sub2Group{{ID: 7, Name: "Codex"}},
		nil,
		sub2APISyncAccountModeSchedulableOnly,
	)
	if err != nil {
		t.Fatalf("UpsertAPIKeyAccountGroupsWithRateAndMode() error = %v", err)
	}
	if action != "updated" {
		t.Fatalf("action = %q, want updated", action)
	}
	if !alreadyMatched {
		t.Fatalf("alreadyMatchedGroups = false, want true")
	}
	if requestCount[http.MethodPut+" /api/v1/admin/accounts/42"] != 1 {
		t.Fatalf("update account calls = %d, want 1", requestCount[http.MethodPut+" /api/v1/admin/accounts/42"])
	}
}

func TestSub2APIUpsertBindsOneAccountToMultipleGroups(t *testing.T) {
	var createPayload map[string]any
	requestCount := map[string]int{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		requestCount[r.Method+" "+r.URL.Path]++
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts":
			if got := r.URL.Query().Get("group"); got != "" {
				t.Fatalf("group query = %q, want empty for multi-group account upsert", got)
			}
			writeSub2TestJSON(w, map[string]any{"items": []map[string]any{}, "total": 0})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts":
			if err := json.NewDecoder(r.Body).Decode(&createPayload); err != nil {
				t.Fatalf("decode create payload: %v", err)
			}
			writeSub2TestJSON(w, map[string]any{
				"id":          88,
				"name":        createPayload["name"],
				"platform":    "openai",
				"type":        "apikey",
				"credentials": createPayload["credentials"],
				"group_ids":   []int64{7, 9},
				"status":      "active",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts/88/schedulable":
			writeSub2TestJSON(w, map[string]any{
				"id":          88,
				"schedulable": true,
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	_, action, err := client.UpsertOpenAIAPIKeyAccountGroups(context.Background(), "multi", "https://newapi.test", "sk-new", []sub2Group{
		{ID: 7, Name: "Codex"},
		{ID: 9, Name: "Claude"},
	})
	if err != nil {
		t.Fatalf("UpsertOpenAIAPIKeyAccountGroups() error = %v", err)
	}
	if action != "created" {
		t.Fatalf("action = %q, want created", action)
	}
	groupIDs, ok := createPayload["group_ids"].([]any)
	if !ok {
		t.Fatalf("group_ids type = %T, want []any", createPayload["group_ids"])
	}
	if len(groupIDs) != 2 || groupIDs[0] != float64(7) || groupIDs[1] != float64(9) {
		t.Fatalf("group_ids = %#v, want [7 9]", groupIDs)
	}
	if createPayload["priority"] != float64(1) {
		t.Fatalf("priority = %v, want 1", createPayload["priority"])
	}
	if createPayload["concurrency"] != float64(10) {
		t.Fatalf("concurrency = %v, want 10", createPayload["concurrency"])
	}
	if createPayload["load_factor"] != float64(10) {
		t.Fatalf("load_factor = %v, want 10", createPayload["load_factor"])
	}
	if createPayload["schedulable"] != true {
		t.Fatalf("schedulable = %v, want true", createPayload["schedulable"])
	}
	if requestCount[http.MethodPost+" /api/v1/admin/accounts"] != 1 {
		t.Fatalf("create account calls = %d, want 1", requestCount[http.MethodPost+" /api/v1/admin/accounts"])
	}
}

func TestSub2APIUpsertSyncsAccountRateMultiplier(t *testing.T) {
	var createPayload map[string]any
	var updatePayload map[string]any
	requestCount := map[string]int{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		requestCount[r.Method+" "+r.URL.Path]++
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts" && requestCount[r.Method+" "+r.URL.Path] == 1:
			writeSub2TestJSON(w, map[string]any{"items": []map[string]any{}, "total": 0})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts":
			writeSub2TestJSON(w, map[string]any{
				"items": []map[string]any{{
					"id":          88,
					"name":        "synced",
					"platform":    "openai",
					"type":        "apikey",
					"credentials": map[string]any{"base_url": "https://newapi.test"},
					"group_ids":   []int64{7},
					"status":      "active",
					"schedulable": true,
				}},
				"total": 1,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts":
			if err := json.NewDecoder(r.Body).Decode(&createPayload); err != nil {
				t.Fatalf("decode create payload: %v", err)
			}
			writeSub2TestJSON(w, map[string]any{
				"id":              88,
				"name":            createPayload["name"],
				"platform":        "openai",
				"type":            "apikey",
				"credentials":     createPayload["credentials"],
				"group_ids":       []int64{7},
				"status":          "active",
				"rate_multiplier": createPayload["rate_multiplier"],
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/admin/accounts/88":
			if err := json.NewDecoder(r.Body).Decode(&updatePayload); err != nil {
				t.Fatalf("decode update payload: %v", err)
			}
			writeSub2TestJSON(w, map[string]any{
				"id":              88,
				"name":            updatePayload["name"],
				"platform":        "openai",
				"type":            "apikey",
				"credentials":     updatePayload["credentials"],
				"group_ids":       []int64{7},
				"status":          "active",
				"rate_multiplier": updatePayload["rate_multiplier"],
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts/88/schedulable":
			writeSub2TestJSON(w, map[string]any{"id": 88, "schedulable": true})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	rate := 0.42
	_, action, err := client.UpsertAPIKeyAccountGroupsWithRate(context.Background(), "openai", "synced", "https://newapi.test", "sk-new", []sub2Group{{ID: 7, Name: "Codex"}}, &rate)
	if err != nil {
		t.Fatalf("create UpsertAPIKeyAccountGroupsWithRate() error = %v", err)
	}
	if action != "created" {
		t.Fatalf("create action = %q, want created", action)
	}
	if createPayload["rate_multiplier"] != rate {
		t.Fatalf("create rate_multiplier = %v, want %v", createPayload["rate_multiplier"], rate)
	}

	rate = 0.27
	_, action, err = client.UpsertAPIKeyAccountGroupsWithRate(context.Background(), "openai", "synced", "https://newapi.test", "sk-newer", []sub2Group{{ID: 7, Name: "Codex"}}, &rate)
	if err != nil {
		t.Fatalf("update UpsertAPIKeyAccountGroupsWithRate() error = %v", err)
	}
	if action != "updated" {
		t.Fatalf("update action = %q, want updated", action)
	}
	if got, ok := updatePayload["rate_multiplier"].(float64); !ok || got != rate {
		t.Fatalf("update rate_multiplier = %#v, want %v", updatePayload["rate_multiplier"], rate)
	}
}

func TestSub2APIEnsureGroupByIDOrNameWithRateUpdatesGroupRate(t *testing.T) {
	var updatePayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/groups/all":
			writeSub2TestJSON(w, []map[string]any{{
				"id":              7,
				"name":            "Codex",
				"platform":        "openai",
				"status":          "active",
				"rate_multiplier": 1,
			}})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/admin/groups/7":
			if err := json.NewDecoder(r.Body).Decode(&updatePayload); err != nil {
				t.Fatalf("decode update payload: %v", err)
			}
			writeSub2TestJSON(w, map[string]any{
				"id":              7,
				"name":            updatePayload["name"],
				"platform":        updatePayload["platform"],
				"status":          updatePayload["status"],
				"rate_multiplier": updatePayload["rate_multiplier"],
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	rate := 0.42
	group, err := client.EnsureGroupByIDOrNameWithRate(context.Background(), 7, "", &rate)
	if err != nil {
		t.Fatalf("EnsureGroupByIDOrNameWithRate() error = %v", err)
	}
	if group.Rate != rate {
		t.Fatalf("group rate = %v, want %v", group.Rate, rate)
	}
	if got, ok := updatePayload["rate_multiplier"].(float64); !ok || got != rate {
		t.Fatalf("group rate_multiplier = %#v, want %v", updatePayload["rate_multiplier"], rate)
	}
}

func TestSub2APIUpsertMatchesSameURLWithinRequestedPlatform(t *testing.T) {
	var anthropicUpdate map[string]any
	requestCount := map[string]int{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		requestCount[r.Method+" "+r.URL.Path]++
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts":
			if got := r.URL.Query().Get("platform"); got != "anthropic" {
				t.Fatalf("platform query = %q, want anthropic", got)
			}
			writeSub2TestJSON(w, map[string]any{
				"items": []map[string]any{
					{
						"id":          50,
						"name":        "claude upstream",
						"platform":    "anthropic",
						"type":        "apikey",
						"credentials": map[string]any{"base_url": "https://newapi.test"},
						"group_ids":   []int64{12},
						"status":      "active",
						"schedulable": true,
					},
				},
				"total": 1,
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/admin/accounts/50":
			if err := json.NewDecoder(r.Body).Decode(&anthropicUpdate); err != nil {
				t.Fatalf("decode update payload: %v", err)
			}
			writeSub2TestJSON(w, map[string]any{
				"id":          50,
				"name":        anthropicUpdate["name"],
				"platform":    "anthropic",
				"type":        "apikey",
				"credentials": anthropicUpdate["credentials"],
				"group_ids":   []int64{12},
				"status":      "active",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts/50/schedulable":
			writeSub2TestJSON(w, map[string]any{"id": 50, "schedulable": true})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	_, action, err := client.UpsertAPIKeyAccountGroups(context.Background(), "anthropic", "claude account", "https://newapi.test/v1", "sk-low", []sub2Group{{ID: 12, Name: "Claude"}})
	if err != nil {
		t.Fatalf("UpsertAPIKeyAccountGroups() error = %v", err)
	}
	if action != "updated" {
		t.Fatalf("action = %q, want updated", action)
	}
	if requestCount[http.MethodPut+" /api/v1/admin/accounts/50"] != 1 {
		t.Fatalf("anthropic update calls = %d, want 1", requestCount[http.MethodPut+" /api/v1/admin/accounts/50"])
	}
	if anthropicUpdate["priority"] != float64(1) {
		t.Fatalf("sync priority = %v, want 1", anthropicUpdate["priority"])
	}
	if _, ok := anthropicUpdate["concurrency"]; ok {
		t.Fatalf("concurrency should not be overwritten on existing account update")
	}
	if _, ok := anthropicUpdate["load_factor"]; ok {
		t.Fatalf("load_factor should not be overwritten on existing account update")
	}
}

func TestSub2APIPrioritizeAccountForGroupsOnlyUpdatesSyncedAccount(t *testing.T) {
	updatePayloads := map[string]map[string]any{}
	schedulableCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/admin/accounts/88":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode priority payload: %v", err)
			}
			updatePayloads[r.URL.Path] = payload
			writeSub2TestJSON(w, map[string]any{"id": pathAccountID(r.URL.Path), "status": payload["status"]})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts/88/schedulable":
			schedulableCalled = true
			writeSub2TestJSON(w, map[string]any{"id": 88, "schedulable": true})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	err = client.PrioritizeOpenAIAPIKeyAccountForGroups(context.Background(), 88, []sub2Group{
		{ID: 7, Name: "Codex"},
		{ID: 9, Name: "Claude"},
	})
	if err != nil {
		t.Fatalf("PrioritizeOpenAIAPIKeyAccountForGroups() error = %v", err)
	}
	if !schedulableCalled {
		t.Fatalf("schedulable endpoint was not called for low-price account")
	}
	assertPriorityPayload(t, updatePayloads["/api/v1/admin/accounts/88"], 1, "active")
	if len(updatePayloads) != 1 {
		t.Fatalf("updated accounts = %v, want only synced account", updatePayloads)
	}
}

func TestSub2APIPrioritizeAccountForGroupsKeepsSyncedRateMultiplier(t *testing.T) {
	updatePayloads := map[string]map[string]any{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/admin/accounts/88":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode priority payload: %v", err)
			}
			updatePayloads[r.URL.Path] = payload
			writeSub2TestJSON(w, map[string]any{"id": pathAccountID(r.URL.Path), "status": payload["status"]})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts/88/schedulable":
			writeSub2TestJSON(w, map[string]any{"id": 88, "schedulable": true})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	rate := 0.33
	err = client.PrioritizeOpenAIAPIKeyAccountForGroupsWithRate(context.Background(), 88, []sub2Group{{ID: 7, Name: "Codex"}}, &rate)
	if err != nil {
		t.Fatalf("PrioritizeOpenAIAPIKeyAccountForGroupsWithRate() error = %v", err)
	}
	if got, ok := updatePayloads["/api/v1/admin/accounts/88"]["rate_multiplier"].(float64); !ok || got != rate {
		t.Fatalf("synced account rate_multiplier = %#v, want %v", updatePayloads["/api/v1/admin/accounts/88"]["rate_multiplier"], rate)
	}
	if len(updatePayloads) != 1 {
		t.Fatalf("updated accounts = %v, want only synced account", updatePayloads)
	}
}

func TestSub2APIListAccountsPaginates(t *testing.T) {
	requestedPages := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/accounts" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		requestedPages = append(requestedPages, r.URL.Query().Get("page"))
		items := make([]map[string]any, 0, 100)
		switch r.URL.Query().Get("page") {
		case "1":
			for i := 0; i < 100; i++ {
				items = append(items, map[string]any{"id": i + 1, "platform": "openai", "type": "apikey"})
			}
		case "2":
			items = append(items, map[string]any{"id": 101, "platform": "openai", "type": "apikey"})
		default:
			t.Fatalf("unexpected page %q", r.URL.Query().Get("page"))
		}
		writeSub2TestJSON(w, map[string]any{
			"items": items,
			"total": 101,
		})
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := client.listAccounts(context.Background(), "", sub2Group{ID: 7})
	if err != nil {
		t.Fatalf("listAccounts() error = %v", err)
	}
	if len(accounts) != 101 {
		t.Fatalf("len(accounts) = %d, want 101", len(accounts))
	}
	if len(requestedPages) != 2 || requestedPages[0] != "1" || requestedPages[1] != "2" {
		t.Fatalf("requested pages = %#v, want [1 2]", requestedPages)
	}
}

func TestSub2APIEnsureGroupPrefersID(t *testing.T) {
	createCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/groups/all":
			writeSub2TestJSON(w, []map[string]any{
				{"id": 7, "name": "Codex", "platform": "openai", "status": "active"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/groups":
			createCalled = true
			writeSub2TestJSON(w, map[string]any{"id": 8, "name": "Created"})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	group, err := client.EnsureGroupByIDOrName(context.Background(), 7, "Different Name")
	if err != nil {
		t.Fatalf("EnsureGroupByIDOrName() error = %v", err)
	}
	if group.ID != 7 || createCalled {
		t.Fatalf("group = %+v createCalled=%v, want existing id 7 without create", group, createCalled)
	}
}

func TestSub2APIEnsureAPIKeyForGroupReusesExistingKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/groups/available":
			writeSub2TestJSON(w, []sub2Group{{ID: 7, Name: "Codex", Rate: 0.15}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/groups/rates":
			writeSub2TestJSON(w, map[string]float64{"7": 0.15})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/keys":
			if got := r.URL.Query().Get("search"); got != "" {
				t.Fatalf("search = %q, want empty because group matching needs full key list", got)
			}
			groupID := int64(7)
			writeSub2TestJSON(w, map[string]any{
				"items": []sub2APIKey{{
					ID:      11,
					Name:    "manual-low-price",
					Key:     "sk-existing",
					GroupID: &groupID,
					Status:  "active",
				}},
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	key, action, err := client.EnsureAPIKeyForGroup(context.Background(), "pm-codex", sub2Group{ID: 7, Name: "Codex", Rate: 0.15})
	if err != nil {
		t.Fatalf("EnsureAPIKeyForGroup() error = %v", err)
	}
	if action != "reused" {
		t.Fatalf("action = %q, want reused", action)
	}
	if key.Key != "sk-existing" {
		t.Fatalf("key = %q, want sk-existing", key.Key)
	}
}

func TestSub2APIEnsureAPIKeyForGroupUpdatesExistingKeyGroup(t *testing.T) {
	var updatePayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/groups/available":
			writeSub2TestJSON(w, []sub2Group{{ID: 7, Name: "Codex", Rate: 0.15}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/groups/rates":
			writeSub2TestJSON(w, map[string]float64{"7": 0.15})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/keys":
			groupID := int64(3)
			writeSub2TestJSON(w, map[string]any{
				"items": []sub2APIKey{{
					ID:      11,
					Name:    "pm-codex",
					Key:     "old-key",
					GroupID: &groupID,
				}},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/keys/11":
			if err := json.NewDecoder(r.Body).Decode(&updatePayload); err != nil {
				t.Fatalf("decode update payload: %v", err)
			}
			groupID := int64(7)
			writeSub2TestJSON(w, sub2APIKey{
				ID:      11,
				Name:    "pm-codex",
				Key:     "new-key",
				GroupID: &groupID,
				Status:  "active",
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	key, action, err := client.EnsureAPIKeyForGroup(context.Background(), "pm-codex", sub2Group{ID: 7, Name: "Codex", Rate: 0.15})
	if err != nil {
		t.Fatalf("EnsureAPIKeyForGroup() error = %v", err)
	}
	if action != "updated" {
		t.Fatalf("action = %q, want updated", action)
	}
	if key.Key != "sk-new-key" {
		t.Fatalf("key = %q, want sk-new-key", key.Key)
	}
	if updatePayload["group_id"] != float64(7) {
		t.Fatalf("group_id = %v, want 7", updatePayload["group_id"])
	}
	if updatePayload["status"] != "active" {
		t.Fatalf("status = %v, want active", updatePayload["status"])
	}
}

func TestSub2APIEnsureAPIKeyForGroupRejectsChangedGroupIdentity(t *testing.T) {
	keyListCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/groups/available":
			writeSub2TestJSON(w, []sub2Group{{ID: 7, Name: "Other", Rate: 0.3}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/keys":
			keyListCalled = true
			writeSub2TestJSON(w, map[string]any{"items": []sub2APIKey{}})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = client.EnsureAPIKeyForGroup(context.Background(), "pm-codex", sub2Group{ID: 7, Name: "Codex", Rate: 0.15})
	if err == nil {
		t.Fatalf("EnsureAPIKeyForGroup() error = nil, want changed group identity error")
	}
	if keyListCalled {
		t.Fatalf("api key list should not be read after group identity mismatch")
	}
}

func TestSub2APIEnsureAPIKeyForGroupRejectsChangedGroupRate(t *testing.T) {
	keyListCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/groups/available":
			writeSub2TestJSON(w, []sub2Group{{ID: 7, Name: "Codex", Rate: 0.15}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/groups/rates":
			writeSub2TestJSON(w, map[string]float64{"7": 0.3})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/keys":
			keyListCalled = true
			writeSub2TestJSON(w, map[string]any{"items": []sub2APIKey{}})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = client.EnsureAPIKeyForGroup(context.Background(), "pm-codex", sub2Group{ID: 7, Name: "Codex", Rate: 0.15})
	if err == nil {
		t.Fatalf("EnsureAPIKeyForGroup() error = nil, want changed group rate error")
	}
	if keyListCalled {
		t.Fatalf("api key list should not be read after group rate mismatch")
	}
}

func TestSub2APIEnsureAPIKeyForGroupPrunesManagedKeysOverLimit(t *testing.T) {
	deletedIDs := []int64{}
	now := time.Now()
	groupID := int64(7)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/groups/available":
			writeSub2TestJSON(w, []sub2Group{{ID: 7, Name: "Codex", Rate: 0.15}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/groups/rates":
			writeSub2TestJSON(w, map[string]float64{"7": 0.15})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/keys":
			keys := make([]sub2APIKey, 0, 12)
			keys = append(keys, sub2APIKey{
				ID:        1,
				Name:      "pm-current",
				Key:       "current-key",
				GroupID:   &groupID,
				Status:    "active",
				CreatedAt: now.Add(-11 * time.Minute),
			})
			for i := int64(2); i <= 12; i++ {
				keys = append(keys, sub2APIKey{
					ID:        i,
					Name:      "pm-extra",
					Key:       "extra-key",
					GroupID:   &groupID,
					Status:    "active",
					CreatedAt: now.Add(-time.Duration(i) * time.Minute),
				})
			}
			writeSub2TestJSON(w, map[string]any{"items": keys, "total": len(keys)})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/keys/"):
			idText := strings.TrimPrefix(r.URL.Path, "/api/v1/keys/")
			id, err := strconv.ParseInt(idText, 10, 64)
			if err != nil {
				t.Fatalf("parse deleted id: %v", err)
			}
			deletedIDs = append(deletedIDs, id)
			writeSub2TestJSON(w, map[string]any{"message": "deleted"})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	key, action, err := client.EnsureAPIKeyForGroup(context.Background(), "pm-current", sub2Group{ID: groupID, Name: "Codex", Rate: 0.15})
	if err != nil {
		t.Fatalf("EnsureAPIKeyForGroup() error = %v", err)
	}
	if action != "reused" {
		t.Fatalf("action = %q, want reused", action)
	}
	if key.ID != 1 {
		t.Fatalf("key id = %d, want 1", key.ID)
	}
	if len(deletedIDs) != 2 {
		t.Fatalf("deleted ids = %#v, want 2 deletes", deletedIDs)
	}
	for _, id := range deletedIDs {
		if id == 1 {
			t.Fatalf("kept key was deleted")
		}
	}
}

func TestSub2APIEnsureAPIKeyForGroupCreatesMissingKey(t *testing.T) {
	var createPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/groups/available":
			writeSub2TestJSON(w, []sub2Group{{ID: 7, Name: "Codex", Rate: 0.15}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/groups/rates":
			writeSub2TestJSON(w, map[string]float64{"7": 0.15})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/keys":
			writeSub2TestJSON(w, map[string]any{"items": []sub2APIKey{}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/keys":
			if err := json.NewDecoder(r.Body).Decode(&createPayload); err != nil {
				t.Fatalf("decode create payload: %v", err)
			}
			groupID := int64(7)
			writeSub2TestJSON(w, sub2APIKey{
				ID:      12,
				Name:    "pm-codex",
				Key:     "created-key",
				GroupID: &groupID,
				Status:  "active",
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	key, action, err := client.EnsureAPIKeyForGroup(context.Background(), "pm-codex", sub2Group{ID: 7, Name: "Codex", Rate: 0.15})
	if err != nil {
		t.Fatalf("EnsureAPIKeyForGroup() error = %v", err)
	}
	if action != "created" {
		t.Fatalf("action = %q, want created", action)
	}
	if key.Key != "sk-created-key" {
		t.Fatalf("key = %q, want sk-created-key", key.Key)
	}
	if createPayload["name"] != "pm-codex" {
		t.Fatalf("name = %v, want pm-codex", createPayload["name"])
	}
	if createPayload["group_id"] != float64(7) {
		t.Fatalf("group_id = %v, want 7", createPayload["group_id"])
	}
}

func TestSub2APISetAccountEnabled(t *testing.T) {
	var statusPayload map[string]any
	var schedulablePayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/admin/accounts/42":
			if err := json.NewDecoder(r.Body).Decode(&statusPayload); err != nil {
				t.Fatalf("decode status payload: %v", err)
			}
			writeSub2TestJSON(w, map[string]any{"id": 42, "status": "inactive"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts/42/schedulable":
			if err := json.NewDecoder(r.Body).Decode(&schedulablePayload); err != nil {
				t.Fatalf("decode schedulable payload: %v", err)
			}
			writeSub2TestJSON(w, map[string]any{"id": 42, "status": "inactive", "schedulable": false})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.SetAccountEnabled(context.Background(), 42, false); err != nil {
		t.Fatalf("SetAccountEnabled() error = %v", err)
	}
	if statusPayload["status"] != "inactive" {
		t.Fatalf("status = %v, want inactive", statusPayload["status"])
	}
	if schedulablePayload["schedulable"] != false {
		t.Fatalf("schedulable = %v, want false", schedulablePayload["schedulable"])
	}
}

func TestSub2APITestAccountConnectionUsesModelID(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/accounts/42/test" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		if got := r.Header.Get("x-api-key"); got != "admin-key" {
			t.Fatalf("x-api-key = %q, want admin-key", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"type":"test_complete","success":true}` + "\n\n"))
	}))
	defer server.Close()

	client, err := NewSub2APIAdminClient(server.URL, "admin-key")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.TestAccountConnection(context.Background(), 42, "gpt-5.5"); err != nil {
		t.Fatalf("TestAccountConnection() error = %v", err)
	}
	if payload["model_id"] != "gpt-5.5" {
		t.Fatalf("model_id = %v, want gpt-5.5", payload["model_id"])
	}
}

func TestSub2APITestAccountConnectionReportsSSEError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"type":"error","error":"model rejected"}` + "\n\n"))
	}))
	defer server.Close()

	client, err := NewSub2APIAdminClient(server.URL, "admin-key")
	if err != nil {
		t.Fatal(err)
	}
	err = client.TestAccountConnection(context.Background(), 42, "gpt-5.5")
	if err == nil || !strings.Contains(err.Error(), "model rejected") {
		t.Fatalf("TestAccountConnection() error = %v, want model rejected", err)
	}
}

func TestSub2APIDisableOtherAPIKeyAccountsForGroupsOnlyClosesSchedulable(t *testing.T) {
	statusUpdated := map[int64]string{}
	schedulable := map[int64]bool{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts":
			if got := r.URL.Query().Get("group"); got != "7" {
				t.Fatalf("group query = %q, want 7", got)
			}
			writeSub2TestJSON(w, map[string]any{
				"items": []map[string]any{
					{"id": 42, "platform": "openai", "type": "apikey", "group_ids": []int64{7}, "status": "active", "schedulable": true},
					{"id": 43, "platform": "openai", "type": "apikey", "group_ids": []int64{7}, "status": "active", "schedulable": true},
					{"id": 44, "platform": "openai", "type": "apikey", "group_ids": []int64{7}, "status": "active", "schedulable": true},
				},
				"total": 3,
			})
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/admin/accounts/"):
			id := pathAccountID(r.URL.Path)
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode disable payload: %v", err)
			}
			if status, ok := payload["status"].(string); ok {
				statusUpdated[id] = status
			}
			writeSub2TestJSON(w, map[string]any{"id": id, "status": payload["status"]})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/schedulable"):
			id := pathAccountID(strings.TrimSuffix(r.URL.Path, "/schedulable"))
			var payload map[string]bool
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode schedulable payload: %v", err)
			}
			schedulable[id] = payload["schedulable"]
			writeSub2TestJSON(w, map[string]any{"id": id, "schedulable": payload["schedulable"]})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.DisableOtherAPIKeyAccountsForGroups(context.Background(), "openai", 42, []sub2Group{{ID: 7, Name: "Codex"}}, "schedulable_only"); err != nil {
		t.Fatalf("DisableOtherAPIKeyAccountsForGroups() error = %v", err)
	}
	if len(statusUpdated) > 0 {
		t.Fatalf("account status/priority endpoint was called: %v; want only schedulable=false", statusUpdated)
	}
	if _, ok := schedulable[42]; ok {
		t.Fatalf("kept account schedulable was changed")
	}
	for _, id := range []int64{43, 44} {
		if statusUpdated[id] == "inactive" {
			t.Fatalf("account %d status was set to inactive; want only schedulable=false", id)
		}
		if schedulable[id] {
			t.Fatalf("account %d schedulable = true, want false", id)
		}
		if _, ok := schedulable[id]; !ok {
			t.Fatalf("account %d schedulable was not changed", id)
		}
	}
}

func TestSub2APIDisableOtherAPIKeyAccountsForGroupsIgnoresDisableStatusMode(t *testing.T) {
	statusUpdated := map[int64]string{}
	schedulable := map[int64]bool{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts":
			writeSub2TestJSON(w, map[string]any{
				"items": []map[string]any{
					{"id": 42, "platform": "openai", "type": "apikey", "group_ids": []int64{7}, "status": "active", "schedulable": true},
					{"id": 43, "platform": "openai", "type": "apikey", "group_ids": []int64{7}, "status": "active", "schedulable": true},
				},
				"total": 2,
			})
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/admin/accounts/"):
			id := pathAccountID(r.URL.Path)
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode status payload: %v", err)
			}
			if status, ok := payload["status"].(string); ok {
				statusUpdated[id] = status
			}
			writeSub2TestJSON(w, map[string]any{"id": id, "status": payload["status"]})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/schedulable"):
			id := pathAccountID(strings.TrimSuffix(r.URL.Path, "/schedulable"))
			var payload map[string]bool
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode schedulable payload: %v", err)
			}
			schedulable[id] = payload["schedulable"]
			writeSub2TestJSON(w, map[string]any{"id": id, "schedulable": payload["schedulable"]})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.DisableOtherAPIKeyAccountsForGroups(context.Background(), "openai", 42, []sub2Group{{ID: 7, Name: "Codex"}}, "disable_status"); err != nil {
		t.Fatalf("DisableOtherAPIKeyAccountsForGroups() error = %v", err)
	}
	if _, ok := statusUpdated[42]; ok {
		t.Fatalf("kept account status was changed")
	}
	if len(statusUpdated) > 0 {
		t.Fatalf("account status endpoint was called: %v; want only schedulable=false", statusUpdated)
	}
	if schedulable[43] {
		t.Fatalf("account 43 schedulable = true, want false")
	}
	if _, ok := schedulable[43]; !ok {
		t.Fatalf("account 43 schedulable was not changed")
	}
}

func TestSub2APIAdminClientUsesAdminKeyHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "admin-key" {
			t.Fatalf("x-api-key = %q, want admin-key", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization = %q, want empty", got)
		}
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/groups/all" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		if got := r.URL.Query().Get("platform"); got != "" {
			t.Fatalf("platform query = %q, want empty so all platforms are returned", got)
		}
		writeSub2TestJSON(w, []map[string]any{
			{"id": 1, "name": "Codex", "platform": "openai", "status": "active"},
			{"id": 2, "name": "Claude", "platform": "anthropic", "status": "active"},
		})
	}))
	defer server.Close()

	client, err := NewSub2APIAdminClient(server.URL, "admin-key")
	if err != nil {
		t.Fatal(err)
	}
	groups, err := client.listGroups(context.Background())
	if err != nil {
		t.Fatalf("listGroups() error = %v", err)
	}
	if len(groups) != 2 || groups[0].ID != 1 || groups[1].Platform != "anthropic" {
		t.Fatalf("groups = %+v, want openai and anthropic groups", groups)
	}
}

func TestSub2APIAdminClientKeepsExplicitBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer jwt-token" {
			t.Fatalf("Authorization = %q, want Bearer jwt-token", got)
		}
		if got := r.Header.Get("x-api-key"); got != "" {
			t.Fatalf("x-api-key = %q, want empty", got)
		}
		writeSub2TestJSON(w, []map[string]any{
			{"id": 1, "name": "Codex", "platform": "openai", "status": "active"},
		})
	}))
	defer server.Close()

	client, err := NewSub2APIAdminClient(server.URL, "Bearer jwt-token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.listGroups(context.Background()); err != nil {
		t.Fatalf("listGroups() error = %v", err)
	}
}

func TestDockerAwareSub2APIBaseURLRewritesLoopbackInDocker(t *testing.T) {
	t.Setenv("PRICE_MONITOR_DOCKER_HOST_REWRITE", "1")
	got := dockerAwareSub2APIBaseURL("http://127.0.0.1:18080")
	want := "http://host.docker.internal:18080/"
	if got != want {
		t.Fatalf("dockerAwareSub2APIBaseURL() = %q, want %q", got, want)
	}
}

func TestDockerAwareSub2APIBaseURLCanDisableRewrite(t *testing.T) {
	t.Setenv("PRICE_MONITOR_DOCKER_HOST_REWRITE", "0")
	got := dockerAwareSub2APIBaseURL("http://127.0.0.1:18080")
	want := "http://127.0.0.1:18080/"
	if got != want {
		t.Fatalf("dockerAwareSub2APIBaseURL() = %q, want %q", got, want)
	}
}

func TestSub2APIFetchBalance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/user/profile" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		writeSub2TestJSON(w, map[string]any{"balance": 12.5})
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	balance, err := client.FetchBalance(context.Background())
	if err != nil {
		t.Fatalf("FetchBalance() error = %v", err)
	}
	if balance.Unit != "usd" || balance.Value == nil || *balance.Value != 12.5 {
		t.Fatalf("balance = %+v, want 12.5 usd", balance)
	}
}

func TestSub2APIFetchRechargeStatusUsesUserPaymentEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("Authorization = %q, want Bearer token", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/payment/config":
			writeSub2TestJSON(w, map[string]any{
				"enabled":                     true,
				"balance_disabled":            false,
				"balance_recharge_multiplier": 5,
				"recharge_fee_rate":           0,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/payment/orders/my":
			if got := r.URL.Query().Get("order_type"); got != "balance" {
				t.Fatalf("order_type = %q, want balance", got)
			}
			writeSub2TestJSON(w, map[string]any{
				"items": []map[string]any{{
					"amount":     100,
					"pay_amount": 10,
					"status":     "COMPLETED",
					"order_type": "balance",
				}},
				"total": 1,
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewSub2APIClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	status, err := client.FetchRechargeStatus(context.Background())
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

func pathAccountID(path string) int64 {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return 0
	}
	id, _ := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	return id
}

func assertPriorityPayload(t *testing.T, payload map[string]any, priority int, status string) {
	t.Helper()
	if payload == nil {
		t.Fatalf("priority payload missing")
	}
	if payload["priority"] != float64(priority) {
		t.Fatalf("priority = %v, want %d", payload["priority"], priority)
	}
	if _, ok := payload["concurrency"]; ok {
		t.Fatalf("concurrency should not be overwritten")
	}
	if _, ok := payload["load_factor"]; ok {
		t.Fatalf("load_factor should not be overwritten")
	}
	if status == "" {
		if _, ok := payload["status"]; ok {
			t.Fatalf("status = %v, want omitted", payload["status"])
		}
		return
	}
	if payload["status"] != status {
		t.Fatalf("status = %v, want %s", payload["status"], status)
	}
}

func writeSub2TestJSON(w http.ResponseWriter, data any) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    0,
		"message": "success",
		"data":    data,
	})
}
