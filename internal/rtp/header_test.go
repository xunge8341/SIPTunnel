package rtp

import "testing"

func TestMainHeaderRoundTrip(t *testing.T) {
	h := NewHeader(11, 1, 2, 128, 9)
	decoded, err := DecodeMainHeader(h.Encode())
	if err != nil {
		t.Fatalf("DecodeMainHeader() error=%v", err)
	}
	if decoded.MessageID != 11 || decoded.ChunkTotal != 2 || decoded.PayloadSize != 128 {
		t.Fatalf("decoded header mismatch: %#v", decoded)
	}
}

func TestTLVRoundTrip(t *testing.T) {
	tlv := []TLV{{Type: 1, Value: []byte("trace")}, {Type: 2, Value: []byte{0x01}}}
	decoded, err := DecodeTLVs(EncodeTLVs(tlv))
	if err != nil {
		t.Fatalf("DecodeTLVs() error=%v", err)
	}
	if len(decoded) != 2 || string(decoded[0].Value) != "trace" {
		t.Fatalf("unexpected tlv: %#v", decoded)
	}
}
