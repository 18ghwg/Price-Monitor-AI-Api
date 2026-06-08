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
