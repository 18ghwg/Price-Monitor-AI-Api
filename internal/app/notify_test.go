package app

import (
	"strings"
	"testing"
)

func TestSnapshotPriceChangesIgnoresUpstreamBalanceOnlyChange(t *testing.T) {
	previousBalance := 10.0
	currentBalance := 8.5
	price := 0.12

	changes := snapshotPriceChanges(PriceSnapshot{
		ModelName:       "gpt-4o",
		GroupName:       "vip",
		GroupRatio:      ptr(0.8),
		InputPrice:      &price,
		UpstreamBalance: &previousBalance,
	}, PriceSnapshot{
		ModelName:       "gpt-4o",
		GroupName:       "vip",
		GroupRatio:      ptr(0.8),
		InputPrice:      &price,
		UpstreamBalance: &currentBalance,
	})

	if len(changes) != 0 {
		t.Fatalf("snapshotPriceChanges() = %#v, want no changes for balance-only update", changes)
	}
}

func TestSnapshotPriceChangesReportsModelGroupPriceChanges(t *testing.T) {
	oldInput := 0.12
	newInput := 0.10
	oldBalance := 10.0
	newBalance := 8.5

	changes := snapshotPriceChanges(PriceSnapshot{
		ModelName:       "gpt-4o",
		GroupName:       "vip",
		GroupRatio:      ptr(0.8),
		InputPrice:      &oldInput,
		UpstreamBalance: &oldBalance,
	}, PriceSnapshot{
		ModelName:       "gpt-4o",
		GroupName:       "premium",
		GroupRatio:      ptr(0.7),
		InputPrice:      &newInput,
		UpstreamBalance: &newBalance,
	})

	labels := make(map[string]bool, len(changes))
	for _, change := range changes {
		labels[change.Label] = true
	}
	for _, label := range []string{"最低价分组", "分组倍率", "输入价格"} {
		if !labels[label] {
			t.Fatalf("snapshotPriceChanges() labels = %#v, want %q", labels, label)
		}
	}
	if labels["上游余额"] {
		t.Fatalf("snapshotPriceChanges() labels = %#v, should not include balance changes", labels)
	}
}

func TestSameLowestSnapshotIgnoresBalanceOnlyChange(t *testing.T) {
	previousBalance := 10.0
	currentBalance := 1.0
	price := 0.12

	previous := PriceSnapshot{
		ID:              1,
		SourceType:      RuleSourceNewAPI,
		SiteID:          10,
		SiteName:        "upstream-a",
		SiteBaseURL:     "https://upstream.example.com/",
		ModelName:       "gpt-4o",
		GroupName:       "vip",
		GroupRatio:      ptr(0.8),
		InputPrice:      &price,
		UpstreamBalance: &previousBalance,
	}
	current := previous
	current.UpstreamBalance = &currentBalance

	if !sameLowestSnapshot(previous, current) {
		t.Fatalf("sameLowestSnapshot() = false, want true for balance-only change")
	}
	if changes := lowestSnapshotChanges(previous, current); len(changes) != 0 {
		t.Fatalf("lowestSnapshotChanges() = %#v, want no changes for balance-only update", changes)
	}
}

func TestLowestSnapshotChangesReportsSourceChange(t *testing.T) {
	price := 0.12
	previous := PriceSnapshot{
		ID:          1,
		SourceType:  RuleSourceNewAPI,
		SiteID:      10,
		SiteName:    "upstream-a",
		SiteBaseURL: "https://a.example.com/",
		ModelName:   "gpt-4o",
		GroupName:   "vip",
		InputPrice:  &price,
	}
	current := previous
	current.ID = 2
	current.SiteID = 11
	current.SiteName = "upstream-b"
	current.SiteBaseURL = "https://b.example.com/"

	changes := lowestSnapshotChanges(previous, current)
	if len(changes) != 1 || changes[0].Label != "最低价渠道" {
		t.Fatalf("lowestSnapshotChanges() = %#v, want source change only", changes)
	}
}

func TestLowestSnapshotChangesReportsInitialLowest(t *testing.T) {
	price := 0.12
	changes := lowestSnapshotChanges(PriceSnapshot{}, PriceSnapshot{
		ID:         1,
		SourceType: RuleSourceSub2API,
		SiteName:   "channel-a",
		ModelName:  "gpt-4o",
		GroupName:  "codex-low",
		InputPrice: &price,
	})

	if len(changes) != 1 || changes[0].Old != "无" {
		t.Fatalf("lowestSnapshotChanges() = %#v, want initial lowest change", changes)
	}
}

func TestFormatSnapshotPriceLinesIncludesAllPriceDimensions(t *testing.T) {
	snapshot := PriceSnapshot{
		InputPrice:      ptr(0.01),
		OutputPrice:     ptr(0.02),
		CacheReadPrice:  ptr(0.003),
		CacheWritePrice: ptr(0.004),
		RequestPrice:    ptr(0.0001),
	}

	body := formatSnapshotPriceLines(snapshot, "  ")
	for _, want := range []string{
		"  输入价格: 0.01",
		"  输出价格: 0.02",
		"  缓存读价格: 0.003",
		"  缓存写价格: 0.004",
		"  请求价格: 0.0001",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("formatSnapshotPriceLines() = %q, want to include %q", body, want)
		}
	}
}
