package sip

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func validHeader(messageType string) Header {
	now := time.Now().UTC()
	return Header{
		ProtocolVersion: ProtocolVersionV1,
		MessageType:     messageType,
		RequestID:       "req-1",
		TraceID:         "trace-1",
		SessionID:       "sess-1",
		ApiCode:         "api.demo",
		SourceSystem:    "system-a",
		SourceNode:      "node-a",
		Timestamp:       now,
		ExpireAt:        now.Add(5 * time.Minute),
		Nonce:           "nonce-1",
		DigestAlg:       "sha256",
		PayloadDigest:   "abc123",
		SignAlg:         "hmac-sha256",
		Signature:       "sig-1",
	}
}

func TestSIPHeaderMirrors(t *testing.T) {
	h := validHeader(MessageTypeTaskStatus)
	mirrors := h.SIPHeaderMirrors()

	if mirrors["X-Request-ID"] != h.RequestID ||
		mirrors["X-Trace-ID"] != h.TraceID ||
		mirrors["X-Session-ID"] != h.SessionID ||
		mirrors["X-Api-Code"] != h.ApiCode ||
		mirrors["X-Message-Type"] != h.MessageType ||
		mirrors["X-Source-System"] != h.SourceSystem {
		t.Fatalf("unexpected mirror headers: %#v", mirrors)
	}
}

func TestHeaderValidateVersionAndTimeWindow(t *testing.T) {
	h := validHeader(MessageTypeCommandCreate)
	h.ProtocolVersion = "2.0"
	if err := h.ValidateForMessageType(MessageTypeCommandCreate, time.Now().UTC()); err == nil {
		t.Fatal("expected version validation error")
	}

	h = validHeader(MessageTypeCommandCreate)
	h.ExpireAt = h.Timestamp
	if err := h.ValidateForMessageType(MessageTypeCommandCreate, time.Now().UTC()); err == nil {
		t.Fatal("expected invalid time window error")
	}

	h = validHeader(MessageTypeCommandCreate)
	h.ExpireAt = time.Now().UTC().Add(-1 * time.Minute)
	if err := h.ValidateForMessageType(MessageTypeCommandCreate, time.Now().UTC()); err == nil {
		t.Fatal("expected expired error")
	}
}

func TestMessageMarshalUnmarshalAndValidate(t *testing.T) {
	cases := []struct {
		name      string
		msg       any
		newMsgPtr func() any
	}{
		{
			name: "command.create",
			msg: CommandCreate{
				Header:     validHeader(MessageTypeCommandCreate),
				CommandID:  "cmd-1",
				Parameters: map[string]any{"k": "v"},
			},
			newMsgPtr: func() any { return &CommandCreate{} },
		},
		{
			name: "command.accepted",
			msg: CommandAccepted{
				Header:     validHeader(MessageTypeCommandAccepted),
				CommandID:  "cmd-1",
				AcceptedAt: time.Now().UTC(),
			},
			newMsgPtr: func() any { return &CommandAccepted{} },
		},
		{
			name: "file.create",
			msg: FileCreate{
				Header:    validHeader(MessageTypeFileCreate),
				FileID:    "file-1",
				TaskID:    "task-1",
				TotalSize: 1024,
				ChunkSize: 256,
			},
			newMsgPtr: func() any { return &FileCreate{} },
		},
		{
			name: "file.accepted",
			msg: FileAccepted{
				Header:     validHeader(MessageTypeFileAccepted),
				FileID:     "file-1",
				AcceptedAt: time.Now().UTC(),
			},
			newMsgPtr: func() any { return &FileAccepted{} },
		},
		{
			name: "task.status",
			msg: TaskStatus{
				Header:   validHeader(MessageTypeTaskStatus),
				TaskID:   "task-1",
				Status:   "running",
				Progress: 50,
			},
			newMsgPtr: func() any { return &TaskStatus{} },
		},
		{
			name: "file.retransmit.request",
			msg: FileRetransmitRequest{
				Header:        validHeader(MessageTypeFileRetransmit),
				FileID:        "file-1",
				MissingChunks: []int{1, 2, 3},
			},
			newMsgPtr: func() any { return &FileRetransmitRequest{} },
		},
		{
			name: "task.result",
			msg: TaskResult{
				Header:  validHeader(MessageTypeTaskResult),
				TaskID:  "task-1",
				Success: true,
				Result:  map[string]any{"output": "ok"},
			},
			newMsgPtr: func() any { return &TaskResult{} },
		},
		{
			name: "task.cancel",
			msg: TaskCancel{
				Header: validHeader(MessageTypeTaskCancel),
				TaskID: "task-1",
				Reason: "user cancelled",
			},
			newMsgPtr: func() any { return &TaskCancel{} },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.msg)
			if err != nil {
				t.Fatalf("marshal error = %v", err)
			}

			ptr := tc.newMsgPtr()
			if err := json.Unmarshal(b, ptr); err != nil {
				t.Fatalf("unmarshal error = %v", err)
			}
		})
	}
}

func TestMessageRequiredFieldValidation(t *testing.T) {
	msg := CommandCreate{
		Header:     validHeader(MessageTypeCommandCreate),
		CommandID:  "",
		Parameters: map[string]any{"k": "v"},
	}
	if err := msg.Validate(); err == nil || !strings.Contains(err.Error(), "command_id") {
		t.Fatalf("expected required field error, got %v", err)
	}

	status := TaskStatus{
		Header:   validHeader(MessageTypeTaskStatus),
		TaskID:   "task-1",
		Status:   "running",
		Progress: 101,
	}
	if err := status.Validate(); err == nil || !strings.Contains(err.Error(), "progress") {
		t.Fatalf("expected progress validation error, got %v", err)
	}

	rt := FileRetransmitRequest{
		Header:        validHeader(MessageTypeFileRetransmit),
		FileID:        "file-1",
		MissingChunks: []int{-1},
	}
	if err := rt.Validate(); err == nil || !strings.Contains(err.Error(), "missing_chunks") {
		t.Fatalf("expected missing_chunks validation error, got %v", err)
	}
}

func TestUnmarshalRejectsWrongMessageType(t *testing.T) {
	raw := map[string]any{
		"protocol_version": ProtocolVersionV1,
		"message_type":     MessageTypeTaskCancel,
		"request_id":       "req-1",
		"trace_id":         "trace-1",
		"session_id":       "sess-1",
		"api_code":         "api.demo",
		"source_system":    "system-a",
		"source_node":      "node-a",
		"timestamp":        time.Now().UTC(),
		"expire_at":        time.Now().UTC().Add(5 * time.Minute),
		"nonce":            "nonce-1",
		"digest_alg":       "sha256",
		"payload_digest":   "abc123",
		"sign_alg":         "hmac-sha256",
		"signature":        "sig-1",
		"command_id":       "cmd-1",
		"parameters":       map[string]any{"k": "v"},
	}

	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var out CommandCreate
	err = json.Unmarshal(b, &out)
	if err == nil || !strings.Contains(err.Error(), "message_type mismatch") {
		t.Fatalf("expected message_type mismatch, got %v", err)
	}
}
