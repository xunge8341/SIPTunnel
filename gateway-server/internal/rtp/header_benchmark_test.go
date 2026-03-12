package rtp

import "testing"

func BenchmarkRTPHeaderEncode(b *testing.B) {
	header := NewHeader(1001, 7, 64, 1400, 64)
	tlvBytes := EncodeTLVs([]TLV{{Type: 1, Value: []byte("file-001.bin")}, {Type: 2, Value: []byte("application/octet-stream")}})
	header.TLVLength = uint32(len(tlvBytes))

	b.ReportAllocs()
	b.SetBytes(int64(mainHeaderSize + len(tlvBytes)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = header.Encode()
	}
}

func BenchmarkRTPHeaderDecode(b *testing.B) {
	header := NewHeader(1001, 7, 64, 1400, 64)
	tlvBytes := EncodeTLVs([]TLV{{Type: 1, Value: []byte("file-001.bin")}, {Type: 2, Value: []byte("application/octet-stream")}})
	header.TLVLength = uint32(len(tlvBytes))
	encoded := header.Encode()

	b.ReportAllocs()
	b.SetBytes(int64(len(encoded)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := DecodeMainHeader(encoded); err != nil {
			b.Fatalf("decode main header: %v", err)
		}
	}
}
