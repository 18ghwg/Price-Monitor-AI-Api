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

func TestIsFallbackSyncErrorMatchesNewAPITokenKeyRoute(t *testing.T) {
	err := errors.New("candidate newapi create NewAPI key: get newapi token key: upstream https://doro.lol/api/token/4173/key returned HTTP 404: Invalid URL")
	if !isFallbackSyncError(err) {
		t.Fatal("isFallbackSyncError() = false, want true for unsupported token key route")
	}
	status := fallbackSyncStatus(err)
	if !strings.HasPrefix(status, "skip fallback candidate: ") {
		t.Fatalf("fallbackSyncStatus() = %q, want fallback prefix", status)
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

	for _, want := range []string{"skip low balance", "huanapi", "Free", "$0.000000"} {
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
