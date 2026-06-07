package app

import (
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
