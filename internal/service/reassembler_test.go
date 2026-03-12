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
