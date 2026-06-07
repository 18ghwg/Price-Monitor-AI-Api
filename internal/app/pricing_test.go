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
