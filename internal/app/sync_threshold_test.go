package app

import (
	"context"
	"math"
	"testing"
)

func TestSyncThresholdRatioForCategory(t *testing.T) {
	codexRatio := 0.05
	settings := IntegrationSettings{
		SyncThresholdRatio: ptr(0.1),
		SyncThresholdRatios: map[string]float64{
			"codex": codexRatio,
			"claud": 0.2,
		},
	}

	if got := syncThresholdRatioForCategory(settings, "codex"); got == nil || *got != codexRatio {
		t.Fatalf("codex ratio = %v, want %v", got, codexRatio)
	}
	if got := syncThresholdRatioForCategory(settings, "other"); got == nil || *got != 0.1 {
		t.Fatalf("fallback ratio = %v, want 0.1", got)
	}
}

func TestOfficialPriceThresholdIncludesClaude(t *testing.T) {
	row, err := officialPriceThreshold(context.Background(), "claude-opus-4-8", 0.05)
	if err != nil {
		t.Fatalf("officialPriceThreshold() error = %v", err)
	}
	if !floatPtrApprox(row.InputPrice, 0.25) {
		t.Fatalf("input threshold = %v, want 0.25", derefFloat(row.InputPrice))
	}
	if !floatPtrApprox(row.OutputPrice, 1.25) {
		t.Fatalf("output threshold = %v, want 1.25", derefFloat(row.OutputPrice))
	}
	if !floatPtrApprox(row.CacheReadPrice, 0.025) {
		t.Fatalf("cache read threshold = %v, want 0.025", derefFloat(row.CacheReadPrice))
	}
	if !floatPtrApprox(row.CacheWritePrice, 0.3125) {
		t.Fatalf("cache write threshold = %v, want 0.3125", derefFloat(row.CacheWritePrice))
	}
}

func floatPtrApprox(value *float64, want float64) bool {
	return value != nil && math.Abs(*value-want) < 1e-12
}

func derefFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}
