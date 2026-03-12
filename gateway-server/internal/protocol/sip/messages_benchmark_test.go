package sip

import (
	"encoding/json"
	"testing"
	"time"
)

func benchmarkCommandCreate() CommandCreate {
	now := time.Now().UTC()
	return CommandCreate{
		Header: Header{
			ProtocolVersion: ProtocolVersionV1,
			MessageType:     MessageTypeCommandCreate,
			RequestID:       "req-bench-1",
			TraceID:         "trace-bench-1",
			SessionID:       "session-bench-1",
			ApiCode:         "api.user.create",
			SourceSystem:    "system-b",
			SourceNode:      "node-az1",
			Timestamp:       now,
			ExpireAt:        now.Add(5 * time.Minute),
			Nonce:           "nonce-bench-1",
			DigestAlg:       "sha256",
			PayloadDigest:   "digest-bench-1",
			SignAlg:         "hmac-sha256",
			Signature:       "signature-bench-1",
		},
		CommandID: "cmd-bench-1",
		Parameters: map[string]any{
			"user": map[string]any{
				"id":      "u-10001",
				"name":    "benchmark-user",
				"enabled": true,
			},
			"tags": []string{"alpha", "beta", "gamma"},
		},
	}
}

func BenchmarkSIPJSONDecodeValidate(b *testing.B) {
	msg := benchmarkCommandCreate()
	raw, err := json.Marshal(msg)
	if err != nil {
		b.Fatalf("marshal command create: %v", err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var out CommandCreate
		if err := json.Unmarshal(raw, &out); err != nil {
			b.Fatalf("unmarshal command create: %v", err)
		}
	}
}
