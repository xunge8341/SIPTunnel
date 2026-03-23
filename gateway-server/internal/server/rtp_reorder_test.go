package server

import "testing"

func TestRTPSequenceReorderBufferRecoversWithinWindow(t *testing.T) {
	buf := newRTPSequenceReorderBuffer(8, 2)
	pkts, state, err := buf.Push(100, []byte("a"))
	if err != nil || state != "in_order" || len(pkts) != 1 {
		t.Fatalf("first push state=%s len=%d err=%v", state, len(pkts), err)
	}
	pkts, state, err = buf.Push(102, []byte("c"))
	if err != nil || state != "buffered" || len(pkts) != 0 {
		t.Fatalf("buffered push state=%s len=%d err=%v", state, len(pkts), err)
	}
	pkts, state, err = buf.Push(101, []byte("b"))
	if err != nil || len(pkts) != 2 {
		t.Fatalf("recovery push state=%s len=%d err=%v", state, len(pkts), err)
	}
	if pkts[0].SequenceNumber != 101 || pkts[1].SequenceNumber != 102 {
		t.Fatalf("recovered order=%d,%d", pkts[0].SequenceNumber, pkts[1].SequenceNumber)
	}
}

func TestRTPSequenceReorderBufferToleratesConfiguredGapBeforeFailing(t *testing.T) {
	buf := newRTPSequenceReorderBuffer(8, 2)
	if _, _, err := buf.Push(100, []byte("a")); err != nil {
		t.Fatalf("seed push err=%v", err)
	}
	if _, state, err := buf.Push(110, []byte("k")); err != nil || state != "gap_tolerated" {
		t.Fatalf("tolerated push state=%s err=%v", state, err)
	}
	if got := buf.PendingGapPackets(); got != 10 {
		t.Fatalf("PendingGapPackets=%d, want 10", got)
	}
	if _, state, err := buf.Push(111, []byte("l")); err != nil || state != "gap_tolerated" {
		t.Fatalf("expected second tolerated push state=gap_tolerated err=%v got state=%s", err, state)
	}
	if _, state, err := buf.Push(112, []byte("m")); err == nil {
		t.Fatalf("expected overflow error, got state=%s", state)
	}
}

func TestRTPSequenceReorderBufferFastForwardToNextPending(t *testing.T) {
	buf := newRTPSequenceReorderBuffer(8, 4)
	if _, _, err := buf.Push(100, []byte("a")); err != nil {
		t.Fatalf("seed push err=%v", err)
	}
	if _, _, err := buf.Push(103, []byte("d")); err != nil {
		t.Fatalf("buffer push err=%v", err)
	}
	if _, _, err := buf.Push(104, []byte("e")); err != nil {
		t.Fatalf("buffer push err=%v", err)
	}
	ready, skipped, ok := buf.FastForwardToNextPending(4)
	if !ok {
		t.Fatal("expected fast-forward to succeed")
	}
	if skipped != 3 {
		t.Fatalf("skipped=%d, want 3", skipped)
	}
	if len(ready) != 2 || ready[0].SequenceNumber != 103 || ready[1].SequenceNumber != 104 {
		t.Fatalf("unexpected ready packets: %+v", ready)
	}
}
