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

func TestSyncUpdateEmailBodyIncludesUpstreamAccountBalanceAndRateDetails(t *testing.T) {
	body := syncUpdateEmailBody(
		Rule{
			ID:                  35,
			ModelKeyword:        "gpt-5.5",
			CategoryName:        "Codex",
			Sub2APIUpstreamName: "",
		},
		Site{Name: "chat.ekti", BaseURL: "https://chat.ekti.cc/"},
		PriceSnapshot{
			SourceAccount:   "ghwg@example.com",
			ModelName:       "gpt-5.5",
			GroupName:       "default",
			GroupRatio:      ptr(0.05),
			InputPrice:      ptr(0.25),
			OutputPrice:     ptr(1.5),
			CacheReadPrice:  ptr(0.025),
			CacheWritePrice: ptr(0.25),
			UpstreamBalance: ptr(12.34),
			BalanceUnit:     "usd",
		},
		"created",
		sub2Account{ID: 54, Name: "chat.ekti openai-local+hermes-openclaw-long+VIP default", Rate: ptr(0.05)},
	)

	for _, want := range []string{
		"上游账号: ghwg@example.com",
		"上游账户余额: $12.340000",
		"分组倍率: 0.05",
		"输入价格: 0.25",
		"输出价格: 1.5",
		"缓存读价格: 0.025",
		"缓存写价格: 0.25",
		"分组倍率变动明细:",
		"- 上游最低价分组: default，倍率 0.05",
		"- 主站账号倍率: 0.05",
		"主站账号: #54 chat.ekti openai-local+hermes-openclaw-long+VIP default",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("syncUpdateEmailBody() = %q, want to include %q", body, want)
		}
	}
	if strings.Contains(body, "上游账号: 未绑定") {
		t.Fatalf("syncUpdateEmailBody() = %q, should not show upstream account as unbound", body)
	}
}

func TestRenderEmailTemplateUsesParsedNotificationVariables(t *testing.T) {
	defaultBody := strings.Join([]string{
		"主站 sub2api 渠道账号已同步。",
		"",
		"站点: chat.ekti",
		"地址: https://chat.ekti.cc/",
		"上游账号: ghwg@example.com",
		"上游账户余额: $12.340000",
		"模型: gpt-5.5",
		"最低价分组: default",
		"分组倍率: 0.05",
		"输入价格: 0.25",
		"动作: created",
	}, "\n")

	subject, body := renderEmailTemplate(IntegrationSettings{
		EmailTemplateEnabled: true,
		EmailTemplateSubject: "【{{notification_type}}】{{site_name}} {{model_name}}",
		EmailTemplateBody:    "站点={{site_name}}\n账号={{upstream_account}}\n余额={{upstream_balance}}\n倍率={{group_ratio}}\n默认:\n{{body}}",
	}, "[主站账号同步] created chat.ekti default", defaultBody)

	if subject != "【主站账号同步】chat.ekti gpt-5.5" {
		t.Fatalf("renderEmailTemplate subject = %q", subject)
	}
	for _, want := range []string{
		"站点=chat.ekti",
		"账号=ghwg@example.com",
		"余额=$12.340000",
		"倍率=0.05",
		"主站 sub2api 渠道账号已同步。",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("renderEmailTemplate body = %q, want %q", body, want)
		}
	}
}
