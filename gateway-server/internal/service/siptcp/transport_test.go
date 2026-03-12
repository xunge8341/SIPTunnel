package siptcp

import (
	"bytes"
	"errors"
	"testing"
)

func TestFramerHalfAndStickyPackets(t *testing.T) {
	framer := NewFramer(1024)
	msg1 := []byte(`{"message_type":"heartbeat"}`)
	msg2 := []byte(`{"message_type":"command_request"}`)
	encoded := append(Encode(msg1), Encode(msg2)...)

	first, err := framer.Feed(encoded[:20])
	if err != nil {
		t.Fatalf("feed first chunk: %v", err)
	}
	if len(first) != 0 {
		t.Fatalf("frames=%d, want 0", len(first))
	}

	second, err := framer.Feed(encoded[20:])
	if err != nil {
		t.Fatalf("feed second chunk: %v", err)
	}
	if len(second) != 2 {
		t.Fatalf("frames=%d, want 2", len(second))
	}
	if !bytes.Equal(second[0], msg1) || !bytes.Equal(second[1], msg2) {
		t.Fatalf("unexpected payloads")
	}
}

func TestFramerLargeMessageRejected(t *testing.T) {
	framer := NewFramer(8)
	_, err := framer.Feed(Encode([]byte("0123456789")))
	if !errors.Is(err, ErrMessageTooLarge) {
		t.Fatalf("err=%v, want ErrMessageTooLarge", err)
	}
}
