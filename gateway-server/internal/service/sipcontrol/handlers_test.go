package sipcontrol

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"siptunnel/internal/protocol/sip"
)

func TestHandlersAcceptedFlow(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	req := RequestContext{Header: newHeader(sip.MessageTypeCommandCreate, now, "sig")}

	tests := []struct {
		name            string
		handler         Handler
		body            []byte
		expectRespType  string
		expectReference string
	}{
		{
			name:            "command.create -> command.accepted",
			handler:         NewCommandCreateHandler(fixedClock{now: now}),
			body:            mustMarshalCommandCreate(t, now, "sig"),
			expectRespType:  sip.MessageTypeCommandAccepted,
			expectReference: "cmd-1",
		},
		{
			name:    "file.create -> file.accepted",
			handler: NewFileCreateHandler(fixedClock{now: now}),
			body: mustMarshal(t, sip.FileCreate{
				Header:    newHeader(sip.MessageTypeFileCreate, now, "sig"),
				FileID:    "file-1",
				TaskID:    "task-1",
				TotalSize: 1024,
				ChunkSize: 256,
			}),
			expectRespType:  sip.MessageTypeFileAccepted,
			expectReference: "file-1",
		},
		{
			name:    "file.retransmit.request -> file.accepted",
			handler: NewFileRetransmitRequestHandler(fixedClock{now: now}),
			body: mustMarshal(t, sip.FileRetransmitRequest{
				Header:        newHeader(sip.MessageTypeFileRetransmit, now, "sig"),
				FileID:        "file-2",
				MissingChunks: []int{1, 2},
			}),
			expectRespType:  sip.MessageTypeFileAccepted,
			expectReference: "file-2",
		},
		{
			name:    "task.cancel -> command.accepted",
			handler: NewTaskCancelHandler(fixedClock{now: now}),
			body: mustMarshal(t, sip.TaskCancel{
				Header: newHeader(sip.MessageTypeTaskCancel, now, "sig"),
				TaskID: "task-9",
				Reason: "manual",
			}),
			expectRespType:  sip.MessageTypeCommandAccepted,
			expectReference: "task-9",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := tc.handler.Handle(context.Background(), req, tc.body)
			if err != nil {
				t.Fatalf("Handle() err=%v", err)
			}

			var raw map[string]any
			if err := json.Unmarshal(resp.Body, &raw); err != nil {
				t.Fatalf("unmarshal response err=%v", err)
			}
			if raw["message_type"] != tc.expectRespType {
				t.Fatalf("unexpected message_type=%v", raw["message_type"])
			}
			if tc.expectRespType == sip.MessageTypeFileAccepted && raw["file_id"] != tc.expectReference {
				t.Fatalf("unexpected file_id=%v", raw["file_id"])
			}
			if tc.expectRespType == sip.MessageTypeCommandAccepted && raw["command_id"] != tc.expectReference {
				t.Fatalf("unexpected command_id=%v", raw["command_id"])
			}
		})
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal err=%v", err)
	}
	return b
}
