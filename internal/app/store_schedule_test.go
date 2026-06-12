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
