package app

import "testing"

func TestFilterPricingRowsForRuleRejectsWrongCategoryGroup(t *testing.T) {
	rows := []PricingRow{
		{
			ModelName:  "claude-opus-4-8",
			GroupName:  "Codex Team",
			GroupRatio: 0.01,
			InputPrice: ptr(0.01),
		},
		{
			ModelName:  "claude-opus-4-8",
			GroupName:  "Claude Team",
			GroupDesc:  "Anthropic",
			GroupRatio: 0.05,
			InputPrice: ptr(0.05),
		},
	}

	filtered := filterPricingRowsForRule(Rule{
		Category:     "claud",
		CategoryName: "Claude",
		ModelKeyword: "claude-opus-4-8",
	}, rows)

	if len(filtered) != 1 {
		t.Fatalf("len(filtered) = %d, want 1", len(filtered))
	}
	if filtered[0].GroupName != "Claude Team" {
		t.Fatalf("group = %q, want Claude Team", filtered[0].GroupName)
	}
}

func TestFilterSub2APIPriceRowsForRuleUsesGroupPlatform(t *testing.T) {
	rows := []Sub2APIUserPriceRow{
		{
			ModelName:     "claude-opus-4-8",
			GroupName:     "Codex Team",
			GroupPlatform: sub2PlatformOpenAI,
			EffectiveRate: 0.01,
		},
		{
			ModelName:     "claude-opus-4-8",
			GroupName:     "Claude Team",
			GroupPlatform: sub2PlatformAnthropic,
			EffectiveRate: 0.05,
		},
	}

	filtered := filterSub2APIPriceRowsForRule(Rule{
		Category:     "claud",
		CategoryName: "Claude",
		ModelKeyword: "claude-opus-4-8",
	}, rows)

	if len(filtered) != 1 {
		t.Fatalf("len(filtered) = %d, want 1", len(filtered))
	}
	if filtered[0].GroupPlatform != sub2PlatformAnthropic {
		t.Fatalf("platform = %q, want %q", filtered[0].GroupPlatform, sub2PlatformAnthropic)
	}
}

func TestFilterPricingRowsForRuleRejectsClaudeGroupForCodex(t *testing.T) {
	rows := []PricingRow{
		{
			ModelName:  "gpt-5.5",
			GroupName:  "Claude Team",
			GroupDesc:  "Anthropic",
			GroupRatio: 0.01,
			InputPrice: ptr(0.01),
		},
		{
			ModelName:  "gpt-5.5",
			GroupName:  "Codex Team",
			GroupRatio: 0.05,
			InputPrice: ptr(0.05),
		},
	}

	filtered := filterPricingRowsForRule(Rule{
		Category:     "codex",
		CategoryName: "Codex",
		ModelKeyword: "gpt-5.5",
	}, rows)

	if len(filtered) != 1 {
		t.Fatalf("len(filtered) = %d, want 1", len(filtered))
	}
	if filtered[0].GroupName != "Codex Team" {
		t.Fatalf("group = %q, want Codex Team", filtered[0].GroupName)
	}
}

func TestNewAPICategoryFilterHappensBeforeCheapestSelection(t *testing.T) {
	pricing := map[string]any{
		"group_ratio": map[string]any{
			"Codex Team":  float64(0.01),
			"Claude Team": float64(0.05),
		},
		"usable_group": map[string]any{
			"Codex Team":  map[string]any{"desc": "OpenAI"},
			"Claude Team": map[string]any{"desc": "Anthropic"},
		},
		"data": []any{
			map[string]any{
				"model_name":       "claude-opus-4-8",
				"quota_type":       float64(0),
				"model_ratio":      float64(1),
				"completion_ratio": float64(4),
				"enable_groups":    []any{"Codex Team", "Claude Team"},
			},
		},
	}

	allRows, err := BuildKeywordRows(pricing, "claude-opus-4-8")
	if err != nil {
		t.Fatalf("BuildKeywordRows() error = %v", err)
	}
	filtered := filterPricingRowsForRule(Rule{
		Category:     "claud",
		CategoryName: "Claude",
		ModelKeyword: "claude-opus-4-8",
	}, allRows)
	cheapest := CheapestPricingRows(filtered)

	if len(cheapest) != 1 {
		t.Fatalf("len(cheapest) = %d, want 1", len(cheapest))
	}
	if cheapest[0].GroupName != "Claude Team" {
		t.Fatalf("group = %q, want Claude Team", cheapest[0].GroupName)
	}
}

func TestFilterPricingRowsKeepsMixedCurrentCategoryGroups(t *testing.T) {
	rows := []PricingRow{
		{
			ModelName:  "gpt-5.5",
			GroupName:  "claude-codex",
			GroupDesc:  "claude users can use codex here",
			GroupRatio: 0.05,
			InputPrice: ptr(0.05),
		},
		{
			ModelName:  "gpt-5.5",
			GroupName:  "Claude Team",
			GroupDesc:  "Anthropic",
			GroupRatio: 0.01,
			InputPrice: ptr(0.01),
		},
	}

	filtered := filterPricingRowsForRule(Rule{
		Category:     "codex",
		CategoryName: "Codex",
		ModelKeyword: "gpt-5.5",
	}, rows)

	if len(filtered) != 1 {
		t.Fatalf("len(filtered) = %d, want 1", len(filtered))
	}
	if filtered[0].GroupName != "claude-codex" {
		t.Fatalf("group = %q, want claude-codex", filtered[0].GroupName)
	}
}

func TestBlockedGroupKeywordsAreRemovedBeforeCheapestSelection(t *testing.T) {
	rows := []PricingRow{
		{
			ModelName:  "gpt-5.5",
			GroupName:  "Codex Free",
			GroupRatio: 0.01,
			InputPrice: ptr(0.01),
		},
		{
			ModelName:  "gpt-5.5",
			GroupName:  "Codex Stable",
			GroupRatio: 0.05,
			InputPrice: ptr(0.05),
		},
	}

	filtered := filterPricingRowsByBlockedKeywords(rows, []string{"free"})
	cheapest := CheapestPricingRows(filtered)

	if len(cheapest) != 1 {
		t.Fatalf("len(cheapest) = %d, want 1", len(cheapest))
	}
	if cheapest[0].GroupName != "Codex Stable" {
		t.Fatalf("group = %q, want Codex Stable", cheapest[0].GroupName)
	}
}

func TestBlockedGroupKeywordsFilterSub2APIPriceRows(t *testing.T) {
	rows := []Sub2APIUserPriceRow{
		{
			ModelName:                "claude-opus-4-8",
			GroupName:                "Claude Free",
			GroupPlatform:            sub2PlatformAnthropic,
			EffectiveRate:            0.01,
			FinalInputPerMillion:     ptr(0.01),
			FinalOutputPerMillion:    ptr(0.02),
			FinalCacheReadPerMillion: ptr(0.001),
		},
		{
			ModelName:             "claude-opus-4-8",
			GroupName:             "Claude Stable",
			GroupPlatform:         sub2PlatformAnthropic,
			EffectiveRate:         0.05,
			FinalInputPerMillion:  ptr(0.05),
			FinalOutputPerMillion: ptr(0.10),
		},
	}

	filtered := filterSub2APIPriceRowsByBlockedKeywords(rows, []string{"free"})
	cheapest := cheapestSub2PriceRowsWithExpectedCacheHitRatio(filtered, 1)

	if len(cheapest) != 1 {
		t.Fatalf("len(cheapest) = %d, want 1", len(cheapest))
	}
	if cheapest[0].GroupName != "Claude Stable" {
		t.Fatalf("group = %q, want Claude Stable", cheapest[0].GroupName)
	}
}
