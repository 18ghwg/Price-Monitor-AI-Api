package app

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestNextScheduledRunAtUsesActualRunTime(t *testing.T) {
	runAt := time.Date(2026, 6, 7, 10, 30, 45, 0, time.UTC)

	got := nextScheduledRunAt(runAt, 20)
	want := runAt.Add(20 * time.Minute)
	if !got.Equal(want) {
		t.Fatalf("nextScheduledRunAt() = %s, want %s", got, want)
	}
}

func TestNextScheduledRunAtDefaultsInvalidInterval(t *testing.T) {
	runAt := time.Date(2026, 6, 7, 10, 30, 45, 0, time.UTC)

	got := nextScheduledRunAt(runAt, 0)
	want := runAt.Add(15 * time.Minute)
	if !got.Equal(want) {
		t.Fatalf("nextScheduledRunAt() = %s, want %s", got, want)
	}
}

func TestNormalizeMonitorScheduleSettings(t *testing.T) {
	round, delay := normalizeMonitorScheduleSettings(0, 0)
	if round != 15 || delay != 60 {
		t.Fatalf("normalizeMonitorScheduleSettings(0,0) = %d,%d want 15,60", round, delay)
	}

	round, delay = normalizeMonitorScheduleSettings(2000, 4000)
	if round != 1440 || delay != 3600 {
		t.Fatalf("normalizeMonitorScheduleSettings(max) = %d,%d want 1440,3600", round, delay)
	}
}

func TestNormalizeSub2APISyncKeepCount(t *testing.T) {
	if got := normalizeSub2APISyncKeepCount(0); got != 1 {
		t.Fatalf("normalizeSub2APISyncKeepCount(0) = %d, want 1", got)
	}
	if got := normalizeSub2APISyncKeepCount(2); got != 2 {
		t.Fatalf("normalizeSub2APISyncKeepCount(2) = %d, want 2", got)
	}
	if got := normalizeSub2APISyncKeepCount(99); got != 10 {
		t.Fatalf("normalizeSub2APISyncKeepCount(99) = %d, want 10", got)
	}
}

func TestGroupScheduledRulesBySourceBatchesSameSite(t *testing.T) {
	groups := groupScheduledRulesBySource([]scheduledRuleSource{
		{rule: Rule{ID: 1, SourceType: RuleSourceNewAPI, SiteID: 10}, site: Site{ID: 10, Name: "site-a"}},
		{rule: Rule{ID: 2, SourceType: RuleSourceNewAPI, SiteID: 20}, site: Site{ID: 20, Name: "site-b"}},
		{rule: Rule{ID: 3, SourceType: RuleSourceNewAPI, SiteID: 10}, site: Site{ID: 10, Name: "site-a"}},
		{rule: Rule{ID: 4, SourceType: RuleSourceSub2API, Sub2APIUpstreamID: 7}, upstream: Sub2APIUpstream{ID: 7, Name: "upstream-a"}},
		{rule: Rule{ID: 5, SourceType: RuleSourceSub2API, Sub2APIUpstreamID: 7}, upstream: Sub2APIUpstream{ID: 7, Name: "upstream-a"}},
	})

	if len(groups) != 3 {
		t.Fatalf("groups len = %d, want 3", len(groups))
	}
	assertGroup := func(index int, key string, ruleIDs ...int64) {
		t.Helper()
		if groups[index].key != key {
			t.Fatalf("group %d key = %q, want %q", index, groups[index].key, key)
		}
		if len(groups[index].rules) != len(ruleIDs) {
			t.Fatalf("group %d rules len = %d, want %d", index, len(groups[index].rules), len(ruleIDs))
		}
		for i, wantID := range ruleIDs {
			if got := groups[index].rules[i].rule.ID; got != wantID {
				t.Fatalf("group %d rule %d id = %d, want %d", index, i, got, wantID)
			}
		}
	}
	assertGroup(0, "newapi:10", 1, 3)
	assertGroup(1, "newapi:20", 2)
	assertGroup(2, "sub2api:7", 4, 5)
}

func TestRunEnabledRulesUsesDueCategories(t *testing.T) {
	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	body := string(source)
	if !strings.Contains(body, "s.store.DueScheduledCategories") {
		t.Fatal("runEnabledRules should load only due scheduled categories")
	}
	if strings.Contains(body, "s.store.ScheduledRuleIDs(ctx, 500)") || strings.Contains(body, "s.store.DueRuleIDs(ctx") {
		t.Fatal("runEnabledRules should not drive scheduling directly by rule ids")
	}
}

func TestCategoryBatchResultDeduplicatesAndSortsModels(t *testing.T) {
	result := newCategoryBatchResult("codex", time.Now())
	result.addSnapshots([]PriceSnapshot{
		{ModelName: "gpt-5.5"},
		{ModelName: " gpt-5.5 "},
		{ModelName: "gpt-4.1"},
	})

	models := result.modelNames()
	if len(models) != 2 || models[0] != "gpt-4.1" || models[1] != "gpt-5.5" {
		t.Fatalf("modelNames = %#v, want sorted unique models", models)
	}
}
