package service

import "testing"

func TestReassemblerMergeAndDuplicate(t *testing.T) {
	r := NewReassembler()
	complete, _, err := r.AddChunk(1, 1, 2, []byte("hello "))
	if err != nil || complete {
		t.Fatalf("unexpected first chunk result complete=%v err=%v", complete, err)
	}
	complete, _, err = r.AddChunk(1, 1, 2, []byte("ignored"))
	if err != nil || complete {
		t.Fatalf("duplicate chunk should be ignored")
	}
	complete, merged, err := r.AddChunk(1, 2, 2, []byte("world"))
	if err != nil || !complete {
		t.Fatalf("expected complete merge, err=%v", err)
	}
	if string(merged) != "hello world" {
		t.Fatalf("unexpected merged %q", string(merged))
	}
}

func TestReassemblerOutOfOrder(t *testing.T) {
	r := NewReassembler()
	complete, _, err := r.AddChunk(2, 2, 3, []byte("world"))
	if err != nil || complete {
		t.Fatalf("unexpected result for first chunk complete=%v err=%v", complete, err)
	}
	complete, _, err = r.AddChunk(2, 1, 3, []byte("hello "))
	if err != nil || complete {
		t.Fatalf("unexpected result for second chunk complete=%v err=%v", complete, err)
	}
	complete, merged, err := r.AddChunk(2, 3, 3, []byte("!"))
	if err != nil || !complete {
		t.Fatalf("expected complete merge for out-of-order chunks, err=%v", err)
	}
	if string(merged) != "hello world!" {
		t.Fatalf("unexpected merged payload %q", string(merged))
	}
}

func TestReassemblerMissingChunk(t *testing.T) {
	r := NewReassembler()
	complete, _, err := r.AddChunk(3, 1, 3, []byte("A"))
	if err != nil || complete {
		t.Fatalf("unexpected result complete=%v err=%v", complete, err)
	}
	complete, _, err = r.AddChunk(3, 3, 3, []byte("C"))
	if err != nil || complete {
		t.Fatalf("should remain incomplete with missing chunk, complete=%v err=%v", complete, err)
	}
}
