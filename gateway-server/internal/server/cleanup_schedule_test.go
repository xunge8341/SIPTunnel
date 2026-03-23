package server

import (
	"testing"
	"time"
)

func TestNextCleanupDelaySupportsEverySyntax(t *testing.T) {
	now := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
	delay, err := nextCleanupDelay("@every 45m", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if delay != 45*time.Minute {
		t.Fatalf("unexpected delay: %v", delay)
	}
}

func TestNextCleanupDelaySupportsCronMinuteWindow(t *testing.T) {
	now := time.Date(2026, 3, 19, 10, 7, 30, 0, time.UTC)
	delay, err := nextCleanupDelay("*/15 * * * *", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := 7*time.Minute + 30*time.Second
	if delay != expected {
		t.Fatalf("unexpected delay: got=%v want=%v", delay, expected)
	}
}

func TestNextCleanupDelayRejectsUnsupportedCalendarSyntax(t *testing.T) {
	if _, err := nextCleanupDelay("0 0 1 * *", time.Now().UTC()); err == nil {
		t.Fatal("expected validation error for unsupported day-of-month cron")
	}
}
