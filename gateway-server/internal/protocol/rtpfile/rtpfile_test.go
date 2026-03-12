package rtpfile

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"testing"
)

func testID(seed byte) [16]byte {
	var id [16]byte
	for i := range id {
		id[i] = seed + byte(i)
	}
	return id
}

func TestHeaderMarshalUnmarshal(t *testing.T) {
	h := Header{
		Flags:         FlagStart,
		TransferID:    testID(1),
		RequestID:     testID(2),
		TraceID:       testID(3),
		ChunkNo:       1,
		ChunkTotal:    3,
		ChunkOffset:   0,
		ChunkLength:   5,
		FileSize:      12,
		ChunkDigest:   sha256.Sum256([]byte("hello")),
		FileDigest:    sha256.Sum256([]byte("hello world!")),
		SendTimestamp: 1720000000000,
		Extensions: []TLV{
			{Type: TLVTypeFileName, Value: []byte("demo.txt")},
			{Type: TLVTypeMimeType, Value: []byte("text/plain")},
		},
	}
	data, err := h.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error = %v", err)
	}

	var decoded Header
	if err := decoded.UnmarshalBinary(data); err != nil {
		t.Fatalf("UnmarshalBinary() error = %v", err)
	}

	if decoded.ChunkNo != h.ChunkNo || decoded.FileSize != h.FileSize {
		t.Fatalf("decoded header mismatch: %#v", decoded)
	}
	if len(decoded.Extensions) != 2 || string(decoded.Extensions[0].Value) != "demo.txt" {
		t.Fatalf("decoded TLV mismatch: %#v", decoded.Extensions)
	}
}

func TestSplitAndReassembleOutOfOrder(t *testing.T) {
	payload := []byte("abcdefghijklmnopqrstuvwxyz")
	chunks, err := SplitFileToChunks(payload, ChunkOptions{
		TransferID: testID(11),
		RequestID:  testID(22),
		TraceID:    testID(33),
		ChunkSize:  5,
	})
	if err != nil {
		t.Fatalf("SplitFileToChunks() error=%v", err)
	}
	if len(chunks) < 3 {
		t.Fatalf("expected multiple chunks")
	}

	r := NewReassembler()
	order := []int{2, 0, 4, 1, 3, 5}
	for _, idx := range order {
		if idx >= len(chunks) {
			continue
		}
		_, err := r.AddChunk(chunks[idx])
		if err != nil {
			t.Fatalf("AddChunk() error=%v", err)
		}
	}
	merged, err := r.Assemble()
	if err != nil {
		t.Fatalf("Assemble() error=%v", err)
	}
	if !bytes.Equal(merged, payload) {
		t.Fatalf("unexpected payload: %q", merged)
	}
}

func TestReassemblerDuplicateChunk(t *testing.T) {
	chunks, err := SplitFileToChunks([]byte("0123456789"), ChunkOptions{ChunkSize: 4})
	if err != nil {
		t.Fatalf("SplitFileToChunks() error=%v", err)
	}
	r := NewReassembler()
	if _, err := r.AddChunk(chunks[0]); err != nil {
		t.Fatalf("AddChunk() first error=%v", err)
	}
	if _, err := r.AddChunk(chunks[0]); err != nil {
		t.Fatalf("AddChunk() duplicate same payload should not fail: %v", err)
	}

	mutated := chunks[0]
	mutated.Payload = append([]byte(nil), chunks[0].Payload...)
	mutated.Payload[0] ^= 0xFF
	if _, err := r.AddChunk(mutated); err == nil {
		t.Fatalf("expected error for duplicate chunk with different payload")
	}
}

func TestTLVUnknownTypeSkipped(t *testing.T) {
	h := Header{ChunkTotal: 1, ChunkNo: 1, ChunkLength: 1, FileSize: 1}
	data, err := h.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error=%v", err)
	}
	unknown := make([]byte, 4+3)
	binary.BigEndian.PutUint16(unknown[0:2], 999)
	binary.BigEndian.PutUint16(unknown[2:4], 3)
	copy(unknown[4:], []byte("abc"))
	known := make([]byte, 4+2)
	binary.BigEndian.PutUint16(known[0:2], TLVTypeMetaJSON)
	binary.BigEndian.PutUint16(known[2:4], 2)
	copy(known[4:], []byte("{}"))

	withTLV := append(data[:fixedHeaderSize], append(unknown, known...)...)
	binary.BigEndian.PutUint16(withTLV[5:7], uint16(len(withTLV)))

	var decoded Header
	if err := decoded.UnmarshalBinary(withTLV); err != nil {
		t.Fatalf("UnmarshalBinary() error=%v", err)
	}
	if len(decoded.Extensions) != 1 || decoded.Extensions[0].Type != TLVTypeMetaJSON {
		t.Fatalf("unexpected parsed tlv: %#v", decoded.Extensions)
	}
}

func TestHeaderLengthValidation(t *testing.T) {
	h := Header{ChunkTotal: 1, ChunkNo: 1, ChunkLength: 1, FileSize: 1}
	data, err := h.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error=%v", err)
	}
	broken := append([]byte(nil), data...)
	binary.BigEndian.PutUint16(broken[5:7], uint16(fixedHeaderSize-1))
	var decoded Header
	if err := decoded.UnmarshalBinary(broken); err == nil {
		t.Fatalf("expected invalid header_length error")
	}
}
