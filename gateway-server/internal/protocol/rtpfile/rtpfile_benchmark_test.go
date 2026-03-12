package rtpfile

import (
	"math/rand"
	"testing"
)

func benchmarkChunkOptions() ChunkOptions {
	var transferID [16]byte
	var requestID [16]byte
	var traceID [16]byte
	copy(transferID[:], "transfer-bench-01")
	copy(requestID[:], "request-bench-01x")
	copy(traceID[:], "trace-bench-01xx")
	return ChunkOptions{
		TransferID:    transferID,
		RequestID:     requestID,
		TraceID:       traceID,
		ChunkSize:     32 * 1024,
		SendTimestamp: 1735689600000,
		Extensions: []TLV{
			{Type: TLVTypeFileName, Value: []byte("bench-file.bin")},
			{Type: TLVTypeMimeType, Value: []byte("application/octet-stream")},
		},
	}
}

func benchmarkPayload(b *testing.B, size int) []byte {
	b.Helper()
	payload := make([]byte, size)
	rng := rand.New(rand.NewSource(20260102))
	if _, err := rng.Read(payload); err != nil {
		b.Fatalf("generate benchmark payload: %v", err)
	}
	return payload
}

func BenchmarkFileSplit(b *testing.B) {
	fileData := benchmarkPayload(b, 4*1024*1024)
	opts := benchmarkChunkOptions()

	b.ReportAllocs()
	b.SetBytes(int64(len(fileData)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := SplitFileToChunks(fileData, opts); err != nil {
			b.Fatalf("split file to chunks: %v", err)
		}
	}
}

func BenchmarkFileAssemble(b *testing.B) {
	fileData := benchmarkPayload(b, 4*1024*1024)
	opts := benchmarkChunkOptions()
	packets, err := SplitFileToChunks(fileData, opts)
	if err != nil {
		b.Fatalf("prepare chunks: %v", err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(fileData)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		reassembler := NewReassembler()
		for _, packet := range packets {
			complete, addErr := reassembler.AddChunk(packet)
			if addErr != nil {
				b.Fatalf("add chunk: %v", addErr)
			}
			if !complete && packet.Header.ChunkNo == packet.Header.ChunkTotal {
				b.Fatal("unexpected incomplete state on last chunk")
			}
		}
		assembled, assembleErr := reassembler.Assemble()
		if assembleErr != nil {
			b.Fatalf("assemble file: %v", assembleErr)
		}
		if len(assembled) != len(fileData) {
			b.Fatalf("assembled length mismatch: got=%d want=%d", len(assembled), len(fileData))
		}
	}
}
