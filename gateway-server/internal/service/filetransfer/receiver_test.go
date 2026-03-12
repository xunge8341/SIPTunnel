package filetransfer

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"siptunnel/internal/protocol/rtpfile"
)

func TestReceiverCollectComplete(t *testing.T) {
	receiver := NewReceiver(t.TempDir())
	packets, payload := buildPackets(t, 8)

	var result *ReceiveResult
	for _, packet := range packets {
		var err error
		result, err = receiver.AddChunk(packet)
		if err != nil {
			t.Fatalf("AddChunk error: %v", err)
		}
	}
	if result == nil || !result.Completed {
		t.Fatalf("expect completed transfer")
	}
	if result.Status != StatusSUCCESS {
		t.Fatalf("expect SUCCESS got=%s", result.Status)
	}
	assertTempFileContent(t, result.TempFilePath, payload)
}

func TestReceiverOutOfOrderComplete(t *testing.T) {
	receiver := NewReceiver(t.TempDir())
	packets, payload := buildPackets(t, 5)
	reordered := make([]rtpfile.ChunkPacket, 0, len(packets))
	for i := len(packets) - 1; i >= 0; i-- {
		reordered = append(reordered, packets[i])
	}

	var result *ReceiveResult
	for _, packet := range reordered {
		var err error
		result, err = receiver.AddChunk(packet)
		if err != nil {
			t.Fatalf("AddChunk error: %v", err)
		}
	}
	if !result.Completed || result.Status != StatusSUCCESS {
		t.Fatalf("expect out-of-order success status=%s completed=%v", result.Status, result.Completed)
	}
	assertTempFileContent(t, result.TempFilePath, payload)
}

func TestReceiverDuplicateChunkDeduplicate(t *testing.T) {
	receiver := NewReceiver(t.TempDir())
	packets, _ := buildPackets(t, 20)

	res1, err := receiver.AddChunk(packets[0])
	if err != nil {
		t.Fatalf("first AddChunk error: %v", err)
	}
	if res1.Duplicate {
		t.Fatalf("first chunk should not be duplicate")
	}

	dup, err := receiver.AddChunk(packets[0])
	if err != nil {
		t.Fatalf("duplicate AddChunk error: %v", err)
	}
	if !dup.Duplicate {
		t.Fatalf("expect duplicate=true")
	}
	if dup.Status != StatusTRANSFERRING {
		t.Fatalf("expect stay TRANSFERRING got=%s", dup.Status)
	}
}

func TestReceiverDetectMissingAndRetransmit(t *testing.T) {
	receiver := NewReceiver(t.TempDir())
	packets, _ := buildPackets(t, 20)
	transferID := packets[0].Header.TransferID

	if _, err := receiver.AddChunk(packets[0]); err != nil {
		t.Fatalf("AddChunk 1 error: %v", err)
	}

	missingRes, err := receiver.DetectMissing(transferID)
	if err != nil {
		t.Fatalf("DetectMissing error: %v", err)
	}
	wantMissing := []uint32{2}
	if !reflect.DeepEqual(missingRes.Missing, wantMissing) {
		t.Fatalf("missing mismatch got=%v want=%v", missingRes.Missing, wantMissing)
	}
	if missingRes.Status != StatusPARTIALMISSING {
		t.Fatalf("expect PARTIAL_MISSING got=%s", missingRes.Status)
	}
	if missingRes.Retransmit == nil || !reflect.DeepEqual(missingRes.Retransmit.MissingChunks, wantMissing) {
		t.Fatalf("invalid retransmit request: %+v", missingRes.Retransmit)
	}

	if err := receiver.MarkRetrying(transferID); err != nil {
		t.Fatalf("MarkRetrying error: %v", err)
	}
}

func TestReceiverDigestMismatch(t *testing.T) {
	receiver := NewReceiver(t.TempDir())
	packets, _ := buildPackets(t, 20)

	corrupted := packets
	corrupted[1] = packets[1]
	corrupted[1].Header.FileDigest = sha256.Sum256([]byte("unexpected"))

	if _, err := receiver.AddChunk(corrupted[0]); err != nil {
		t.Fatalf("AddChunk 0 error: %v", err)
	}

	_, err := receiver.AddChunk(corrupted[1])
	if err == nil {
		t.Fatalf("expect digest mismatch error")
	}
	if !errors.Is(err, os.ErrInvalid) && !strings.Contains(err.Error(), "file_digest mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func buildPackets(t *testing.T, chunkSize int) ([]rtpfile.ChunkPacket, []byte) {
	t.Helper()
	payload := []byte("SIPTunnel RTP file transfer test payload")
	transferID := [16]byte{1, 2, 3, 4, 5, 6}
	requestID := [16]byte{9, 9, 9}
	traceID := [16]byte{8, 8, 8}
	packets, err := rtpfile.SplitFileToChunks(payload, rtpfile.ChunkOptions{
		TransferID: transferID,
		RequestID:  requestID,
		TraceID:    traceID,
		ChunkSize:  chunkSize,
	})
	if err != nil {
		t.Fatalf("SplitFileToChunks error: %v", err)
	}
	return packets, payload
}

func assertTempFileContent(t *testing.T, path string, want []byte) {
	t.Helper()
	if path == "" {
		t.Fatalf("temp file path should not be empty")
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read temp file error: %v", err)
	}
	if !reflect.DeepEqual(data, want) {
		t.Fatalf("assembled file mismatch got=%q want=%q", string(data), string(want))
	}
}
