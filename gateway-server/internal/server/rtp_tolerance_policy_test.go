package server

import (
	"testing"
	"time"
)

func TestEscalateRangePlaybackTolerancePolicy(t *testing.T) {
	policy := rtpTolerancePolicy{
		ProfileName:   "range_playback",
		RangePlayback: true,
		ReorderWindow: 192,
		LossTolerance: 64,
		GapTimeout:    450 * time.Millisecond,
		FECEnabled:    true,
		FECGroupSize:  8,
	}
	next, changed := escalateRangePlaybackTolerancePolicy(policy)
	if !changed {
		t.Fatal("expected range playback policy to escalate")
	}
	if next.ProfileName != "range_playback_rescued" {
		t.Fatalf("expected rescued profile name, got %s", next.ProfileName)
	}
	if next.ReorderWindow < genericDownloadRTPReorderWindowPackets() {
		t.Fatalf("expected reorder window >= generic download profile, got %d", next.ReorderWindow)
	}
	if next.LossTolerance < genericDownloadRTPLossTolerancePackets() {
		t.Fatalf("expected loss tolerance >= generic download profile, got %d", next.LossTolerance)
	}
	if next.GapTimeout < genericDownloadRTPGapTimeout() {
		t.Fatalf("expected gap timeout >= generic download profile, got %s", next.GapTimeout)
	}
}

func TestRTPSequenceReorderBufferExpandTolerance(t *testing.T) {
	buf := newRTPSequenceReorderBuffer(192, 64)
	if !buf.ExpandTolerance(512, 192) {
		t.Fatal("expected expand tolerance to report change")
	}
	if got := buf.reorderWindow; got != 512 {
		t.Fatalf("reorderWindow=%d, want 512", got)
	}
	if got := buf.lossTolerance; got != 192 {
		t.Fatalf("lossTolerance=%d, want 192", got)
	}
	if buf.ExpandTolerance(128, 32) {
		t.Fatal("expected shrinking tolerance to be ignored")
	}
}
