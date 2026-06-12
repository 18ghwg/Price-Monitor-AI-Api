package app

import (
	"errors"
	"strings"
	"testing"
)

func TestCandidateLabelIncludesGroupRatio(t *testing.T) {
	label := candidateLabel(PriceSnapshot{
		SourceType: RuleSourceSub2API,
		SiteName:   "huanapi",
		GroupName:  "Free",
		GroupRatio: ptr(0.000001),
	})

	for _, want := range []string{"sub2api", "huanapi", "Free", "倍率 0.000001"} {
		if !strings.Contains(label, want) {
			t.Fatalf("candidateLabel() = %q, want to include %q", label, want)
		}
	}
}

func TestIsFallbackSyncErrorMatchesUnsupportedGroup(t *testing.T) {
	err := errors.New("您当前的套餐或余额不支持使用所选分组")
	if !isFallbackSyncError(err) {
		t.Fatal("isFallbackSyncError() = false, want true for unsupported group errors")
	}
}

func TestIsSub2APISyncDisabledError(t *testing.T) {
	for _, err := range []error{
		errors.New("sub2api sync is disabled"),
		errors.New("主站 sub2api 同步开关未开启"),
	} {
		if !isSub2APISyncDisabledError(err) {
			t.Fatalf("isSub2APISyncDisabledError(%q) = false, want true", err.Error())
		}
	}
}

func TestIsFallbackSyncErrorMatchesNewAPITokenKeyRoute(t *testing.T) {
	err := errors.New("candidate newapi create NewAPI key: get newapi token key: upstream https://doro.lol/api/token/4173/key returned HTTP 404: Invalid URL")
	if !isFallbackSyncError(err) {
		t.Fatal("isFallbackSyncError() = false, want true for unsupported token key route")
	}
	status := fallbackSyncStatus(err)
	if !strings.HasPrefix(status, "跳过该低价候选：") {
		t.Fatalf("fallbackSyncStatus() = %q, want fallback prefix", status)
	}
	if strings.Contains(status, "Invalid URL") {
		t.Fatalf("fallbackSyncStatus() = %q, want localized error", status)
	}
}

func TestIsFallbackSyncErrorMatchesTemporaryRateLimit(t *testing.T) {
	err := errors.New("candidate newapi / ai.17nas / 账号 ghwg / GPTfree / 倍率 0.01 login NewAPI upstream: upstream https://ai.17nas.com/api/user/login returned HTTP 429:")
	if !isFallbackSyncError(err) {
		t.Fatal("isFallbackSyncError() = false, want true for temporary upstream rate limit")
	}
	status := fallbackSyncStatus(err)
	for _, want := range []string{"跳过该低价候选", "HTTP 429（上游临时限流）"} {
		if !strings.Contains(status, want) {
			t.Fatalf("fallbackSyncStatus() = %q, want %q", status, want)
		}
	}
}

func TestIsFallbackSyncErrorMatchesTemporaryConnectionEOF(t *testing.T) {
	err := errors.New(`candidate newapi / ai.17nas login NewAPI upstream: request https://ai.17nas.com/api/user/login: Post "https://ai.17nas.com/api/user/login": EOF`)
	if !isFallbackSyncError(err) {
		t.Fatal("isFallbackSyncError() = false, want true for temporary upstream EOF")
	}
	status := fallbackSyncStatus(err)
	for _, want := range []string{"跳过该低价候选", "上游连接中断"} {
		if !strings.Contains(status, want) {
			t.Fatalf("fallbackSyncStatus() = %q, want %q", status, want)
		}
	}
}

func TestIsFallbackSyncErrorMatchesTemporaryServiceUnavailable(t *testing.T) {
	err := errors.New(`主站账号连接测试失败：账号 #53，模型 gpt-5.5，原因：API returned 503: {"error":{"message":"Service temporarily unavailable"}}`)
	if !isFallbackSyncError(err) {
		t.Fatal("isFallbackSyncError() = false, want true for temporary service unavailable")
	}
	status := fallbackSyncStatus(err)
	for _, want := range []string{"跳过该低价候选", "接口返回 503", "服务暂时不可用"} {
		if !strings.Contains(status, want) {
			t.Fatalf("fallbackSyncStatus() = %q, want %q", status, want)
		}
	}
}

func TestIsFallbackSyncErrorMatchesTemporaryBadGateway(t *testing.T) {
	err := errors.New(`主站账号连接测试失败：账号 #55，模型 gpt-5.5，原因：API returned 502: error code: 502`)
	if !isFallbackSyncError(err) {
		t.Fatal("isFallbackSyncError() = false, want true for temporary bad gateway")
	}
	status := fallbackSyncStatus(err)
	for _, want := range []string{"跳过该低价候选", "接口返回 502", "错误码 502"} {
		if !strings.Contains(status, want) {
			t.Fatalf("fallbackSyncStatus() = %q, want %q", status, want)
		}
	}
}

func TestIsStaleGroupSyncErrorMatchesNoAvailableChannel(t *testing.T) {
	err := errors.New(`主站账号连接测试失败：账号 #53，模型 gpt-5.5，主站分组 openai-local, VIP，上游低价分组 GPTfree，原因：接口返回 503: {"error":{"code":"model_not_found","message":"No available channel for model gpt-5.5 under group GPTfree (distributor)"}}`)
	if !isStaleGroupSyncError(err) {
		t.Fatal("isStaleGroupSyncError() = false, want true for stale upstream group errors")
	}
	if !isFallbackSyncError(err) {
		t.Fatal("isFallbackSyncError() = false, want true so stale group candidate can be skipped")
	}
	status := fallbackSyncStatus(err)
	for _, want := range []string{"没有可用渠道支持模型", "上游低价分组 GPTfree"} {
		if !strings.Contains(status, want) {
			t.Fatalf("fallbackSyncStatus() = %q, want %q", status, want)
		}
	}
}

func TestLowBalanceNotificationSignatureUsesUpstreamAccount(t *testing.T) {
	tests := []struct {
		name     string
		snapshot PriceSnapshot
		want     string
	}{
		{
			name: "newapi site id",
			snapshot: PriceSnapshot{
				SourceType:  RuleSourceNewAPI,
				SiteID:      42,
				SiteBaseURL: "https://example.com",
			},
			want: "newapi|42",
		},
		{
			name: "sub2api upstream id",
			snapshot: PriceSnapshot{
				SourceType:        RuleSourceSub2API,
				Sub2APIUpstreamID: 88,
				SiteBaseURL:       "https://example.com",
			},
			want: "sub2api|88",
		},
		{
			name: "fallback base url",
			snapshot: PriceSnapshot{
				SourceType:  RuleSourceNewAPI,
				SiteBaseURL: " HTTPS://Example.COM ",
			},
			want: "newapi|https://example.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lowBalanceNotificationSignature(tt.snapshot); got != tt.want {
				t.Fatalf("lowBalanceNotificationSignature() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLowBalanceStatusIncludesSourceGroupAndBalance(t *testing.T) {
	balance := 0.0
	status := lowBalanceStatus(PriceSnapshot{
		SiteName:        "huanapi",
		GroupName:       "Free",
		UpstreamBalance: &balance,
		BalanceUnit:     "usd",
	})

	for _, want := range []string{"跳过余额不足", "huanapi", "Free", "$0.000000"} {
		if !strings.Contains(status, want) {
			t.Fatalf("lowBalanceStatus() = %q, want to include %q", status, want)
		}
	}
}

func TestLowBalanceNotifyWindowKeepsOnlyFirstFive(t *testing.T) {
	var skipped []PriceSnapshot
	for id := int64(1); id <= 7; id++ {
		skipped = append(skipped, PriceSnapshot{ID: id})
	}

	window := lowBalanceNotifyWindow(skipped)
	if len(window) != 5 {
		t.Fatalf("lowBalanceNotifyWindow() length = %d, want 5", len(window))
	}
	if window[0].ID != 1 || window[4].ID != 5 {
		t.Fatalf("lowBalanceNotifyWindow() IDs = %d..%d, want 1..5", window[0].ID, window[4].ID)
	}
}

func TestLowBalanceSkippedBelowCandidateExcludesPricierSkippedRows(t *testing.T) {
	candidate := PriceSnapshot{
		ID:         10,
		InputPrice: ptr(0.2),
		GroupRatio: ptr(0.04),
		GroupName:  "gpt 最后狂欢日",
	}
	skipped := []PriceSnapshot{
		{
			ID:         1,
			InputPrice: ptr(0.4),
			GroupRatio: ptr(0.08),
			GroupName:  "OpenAi",
		},
		{
			ID:         2,
			InputPrice: ptr(0.1),
			GroupRatio: ptr(0.02),
			GroupName:  "Free",
		},
	}

	filtered := lowBalanceSkippedBelowCandidate(skipped, candidate, 0)
	if len(filtered) != 1 {
		t.Fatalf("len(filtered) = %d, want 1", len(filtered))
	}
	if filtered[0].ID != 2 {
		t.Fatalf("filtered[0].ID = %d, want 2", filtered[0].ID)
	}
}

func TestLowerThanPreviousLowestRejectsPriceIncrease(t *testing.T) {
	previous := PriceSnapshot{
		ID:          1,
		ModelName:   "gpt-5.5",
		GroupName:   "default",
		InputPrice:  ptr(0.13),
		OutputPrice: ptr(0.39),
	}
	current := PriceSnapshot{
		ID:          2,
		ModelName:   "gpt-5.5",
		GroupName:   "default",
		InputPrice:  ptr(0.13),
		OutputPrice: ptr(0.78),
	}

	if lowerThanPreviousLowest(previous, current, 0) {
		t.Fatal("lowerThanPreviousLowest() = true, want false when output price increases")
	}
}

func TestLowerThanPreviousLowestAcceptsRealPriceDrop(t *testing.T) {
	previous := PriceSnapshot{
		ID:          1,
		ModelName:   "gpt-5.5",
		GroupName:   "default",
		InputPrice:  ptr(0.13),
		OutputPrice: ptr(0.78),
	}
	current := PriceSnapshot{
		ID:          2,
		ModelName:   "gpt-5.5",
		GroupName:   "default",
		InputPrice:  ptr(0.13),
		OutputPrice: ptr(0.39),
	}

	if !lowerThanPreviousLowest(previous, current, 0) {
		t.Fatal("lowerThanPreviousLowest() = false, want true when output price decreases")
	}
}

func TestLowerThanPreviousLowestUsesExpectedCacheHitRatio(t *testing.T) {
	previous := PriceSnapshot{
		ID:              1,
		ModelName:       "gpt-5.5",
		GroupName:       "default",
		InputPrice:      ptr(1.0),
		OutputPrice:     ptr(2.0),
		CacheReadPrice:  ptr(0.2),
		CacheWritePrice: ptr(1.0),
	}
	current := PriceSnapshot{
		ID:              2,
		ModelName:       "gpt-5.5",
		GroupName:       "default",
		InputPrice:      ptr(0.9),
		OutputPrice:     ptr(2.0),
		CacheReadPrice:  ptr(0.1),
		CacheWritePrice: ptr(1.2),
	}

	if lowerThanPreviousLowest(previous, current, 0.0) {
		t.Fatal("lowerThanPreviousLowest() = true, want false when expected cost is higher at hit ratio 0")
	}
	if !lowerThanPreviousLowest(previous, current, 1.0) {
		t.Fatal("lowerThanPreviousLowest() = false, want true when cache-hit cost is lower at hit ratio 1")
	}
}
