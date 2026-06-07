package app

import "testing"

func TestBuildSub2APIUserPriceRowsAppliesUserGroupRate(t *testing.T) {
	prices := map[string]any{
		"gpt-test": map[string]any{
			"litellm_provider":      "openai",
			"mode":                  "chat",
			"input_cost_per_token":  0.000001,
			"output_cost_per_token": 0.000002,
		},
	}
	groups := []sub2Group{{
		ID:       7,
		Name:     "Codex",
		Platform: "openai",
		Rate:     2,
	}}
	rows := buildSub2APIUserPriceRows(prices, groups, map[string]float64{"7": 0.5}, sub2APIUserPriceInput{
		ModelKeyword: "gpt",
	})
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.EffectiveRate != 0.5 {
		t.Fatalf("EffectiveRate = %v, want 0.5", row.EffectiveRate)
	}
	if row.UserGroupRate == nil || *row.UserGroupRate != 0.5 {
		t.Fatalf("UserGroupRate = %v, want 0.5", row.UserGroupRate)
	}
	if row.GroupDefaultRate == nil || *row.GroupDefaultRate != 2 {
		t.Fatalf("GroupDefaultRate = %v, want 2", row.GroupDefaultRate)
	}
	if row.FinalInputPerMillion == nil || *row.FinalInputPerMillion != 0.5 {
		t.Fatalf("FinalInputPerMillion = %v, want 0.5", row.FinalInputPerMillion)
	}
	if row.FinalOutputPerMillion == nil || *row.FinalOutputPerMillion != 1 {
		t.Fatalf("FinalOutputPerMillion = %v, want 1", row.FinalOutputPerMillion)
	}
}

func TestBuildSub2APIUserPriceRowsExactModelsFilter(t *testing.T) {
	prices := map[string]any{
		"gpt-test": map[string]any{
			"litellm_provider":      "openai",
			"mode":                  "chat",
			"input_cost_per_token":  0.000001,
			"output_cost_per_token": 0.000002,
		},
		"gpt-test-plus": map[string]any{
			"litellm_provider":      "openai",
			"mode":                  "chat",
			"input_cost_per_token":  0.0000005,
			"output_cost_per_token": 0.000001,
		},
	}
	groups := []sub2Group{{ID: 7, Name: "Codex", Platform: "openai", Rate: 1}}

	rows := buildSub2APIUserPriceRows(prices, groups, nil, sub2APIUserPriceInput{
		Models: "gpt-test",
		Modes:  "chat",
	})
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].ModelName != "gpt-test" {
		t.Fatalf("model = %q, want exact gpt-test", rows[0].ModelName)
	}
}

func TestCheapestSub2GroupsByPlatformUsesPlatformAndEffectiveRate(t *testing.T) {
	groups := []sub2Group{
		{ID: 1, Name: "GPT Default", Platform: "openai", Rate: 1.2},
		{ID: 2, Name: "GPT Cheap", Platform: "openai", Rate: 0.9},
		{ID: 3, Name: "Claude Default", Platform: "anthropic", Rate: 1.1},
		{ID: 4, Name: "Claude Override", Platform: "anthropic", Rate: 2},
	}

	cheapest := cheapestSub2GroupsByPlatform(groups, map[string]float64{"4": 0.6}, "")
	if len(cheapest) != 2 {
		t.Fatalf("len(cheapest) = %d, want 2", len(cheapest))
	}

	byPlatform := map[string]sub2CheapestGroup{}
	for _, group := range cheapest {
		byPlatform[group.Platform] = group
	}
	if byPlatform["openai"].GroupID != 2 {
		t.Fatalf("openai cheapest group id = %d, want 2", byPlatform["openai"].GroupID)
	}
	if byPlatform["openai"].PlatformLabel != "GPT/OpenAI" {
		t.Fatalf("openai label = %q, want GPT/OpenAI", byPlatform["openai"].PlatformLabel)
	}
	if byPlatform["anthropic"].GroupID != 4 {
		t.Fatalf("anthropic cheapest group id = %d, want 4", byPlatform["anthropic"].GroupID)
	}
	if byPlatform["anthropic"].PlatformLabel != "Claude" {
		t.Fatalf("anthropic label = %q, want Claude", byPlatform["anthropic"].PlatformLabel)
	}
	if byPlatform["anthropic"].EffectiveRate != 0.6 {
		t.Fatalf("anthropic effective rate = %v, want 0.6", byPlatform["anthropic"].EffectiveRate)
	}
}
