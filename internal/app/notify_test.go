package app

import "testing"

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
