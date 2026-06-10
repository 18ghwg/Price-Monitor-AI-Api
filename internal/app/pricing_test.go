package app

import "testing"

func TestBuildCheapestKeywordRows(t *testing.T) {
	pricing := map[string]any{
		"group_ratio": map[string]any{
			"default": float64(1),
			"cheap":   float64(0.25),
			"premium": float64(2),
		},
		"usable_group": map[string]any{
			"default": map[string]any{"desc": "Default"},
			"cheap":   map[string]any{"desc": "Cheap"},
			"premium": map[string]any{"desc": "Premium"},
		},
		"data": []any{
			map[string]any{
				"model_name":       "codex-alpha",
				"quota_type":       float64(0),
				"model_ratio":      float64(1),
				"completion_ratio": float64(4),
				"enable_groups":    []any{"default", "cheap"},
			},
			map[string]any{
				"model_name":    "codex-request",
				"quota_type":    float64(1),
				"model_price":   float64(0.01),
				"enable_groups": []any{"premium", "cheap"},
			},
			map[string]any{
				"model_name":    "claude-hidden",
				"quota_type":    float64(0),
				"model_ratio":   float64(100),
				"enable_groups": []any{"cheap"},
			},
		},
	}

	rows, err := BuildCheapestKeywordRows(pricing, "codex-alpha")
	if err != nil {
		t.Fatalf("BuildCheapestKeywordRows() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	for _, row := range rows {
		if row.GroupName != "cheap" {
			t.Fatalf("row %s group = %s, want cheap", row.ModelName, row.GroupName)
		}
	}
	if rows[0].ModelName != "codex-alpha" {
		t.Fatalf("first row = %s, want exact model codex-alpha", rows[0].ModelName)
	}
	if rows[0].InputPrice == nil || *rows[0].InputPrice != 0.5 {
		t.Fatalf("token input price = %v, want 0.5", rows[0].InputPrice)
	}
	partialRows, err := BuildCheapestKeywordRows(pricing, "codex")
	if err != nil {
		t.Fatalf("BuildCheapestKeywordRows(partial) error = %v", err)
	}
	if len(partialRows) != 0 {
		t.Fatalf("partial model match len = %d, want 0", len(partialRows))
	}
}

func TestApplyNewAPIUserGroupPricingOverridesPricingRatio(t *testing.T) {
	pricing := map[string]any{
		"group_ratio": map[string]any{
			"Codex": float64(1),
		},
		"usable_group": map[string]any{
			"Codex": "Codex",
		},
		"data": []any{
			map[string]any{
				"model_name":       "gpt-5.5",
				"quota_type":       float64(0),
				"model_ratio":      float64(2.5),
				"completion_ratio": float64(6),
				"enable_groups":    []any{"Codex"},
				"cache_ratio":      "0.1",
			},
		},
	}

	ApplyNewAPIUserGroupPricing(pricing, map[string]NewAPIUserGroupPricing{
		"Codex": {
			Desc:  "Codex",
			Ratio: ptr(0.1),
		},
	})

	rows, err := BuildKeywordRows(pricing, "gpt-5.5")
	if err != nil {
		t.Fatalf("BuildKeywordRows() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.GroupRatio != 0.1 {
		t.Fatalf("group ratio = %v, want 0.1", row.GroupRatio)
	}
	if row.InputPrice == nil || *row.InputPrice != 0.5 {
		t.Fatalf("input price = %v, want 0.5", row.InputPrice)
	}
	if row.OutputPrice == nil || *row.OutputPrice != 3 {
		t.Fatalf("output price = %v, want 3", row.OutputPrice)
	}
	if row.CacheReadPrice == nil || *row.CacheReadPrice != 0.05 {
		t.Fatalf("cache read price = %v, want 0.05", row.CacheReadPrice)
	}
}
