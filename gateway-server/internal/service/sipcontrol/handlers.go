package sipcontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"siptunnel/internal/protocol/sip"
)

type CommandCreateHandler struct {
	clock Clock
}

func NewCommandCreateHandler(clock Clock) *CommandCreateHandler {
	if clock == nil {
		clock = realClock{}
	}
	return &CommandCreateHandler{clock: clock}
}

func (h *CommandCreateHandler) MessageType() string {
	return sip.MessageTypeCommandCreate
}

func (h *CommandCreateHandler) Handle(_ context.Context, req RequestContext, body []byte) (OutboundMessage, error) {
	var msg sip.CommandCreate
	if err := json.Unmarshal(body, &msg); err != nil {
		return OutboundMessage{}, fmt.Errorf("parse command.create: %w", err)
	}

	ack := sip.CommandAccepted{
		Header:     acceptedHeader(req.Header, sip.MessageTypeCommandAccepted, h.clock.Now()),
		CommandID:  msg.CommandID,
		AcceptedAt: h.clock.Now(),
	}
	resp, err := json.Marshal(ack)
	if err != nil {
		return OutboundMessage{}, fmt.Errorf("marshal command.accepted: %w", err)
	}
	return OutboundMessage{Body: resp}, nil
}

type FileCreateHandler struct {
	clock Clock
}

func NewFileCreateHandler(clock Clock) *FileCreateHandler {
	if clock == nil {
		clock = realClock{}
	}
	return &FileCreateHandler{clock: clock}
}

func (h *FileCreateHandler) MessageType() string {
	return sip.MessageTypeFileCreate
}

func (h *FileCreateHandler) Handle(_ context.Context, req RequestContext, body []byte) (OutboundMessage, error) {
	var msg sip.FileCreate
	if err := json.Unmarshal(body, &msg); err != nil {
		return OutboundMessage{}, fmt.Errorf("parse file.create: %w", err)
	}
	return newFileAccepted(req.Header, msg.FileID, h.clock.Now())
}

type FileRetransmitRequestHandler struct {
	clock Clock
}

func NewFileRetransmitRequestHandler(clock Clock) *FileRetransmitRequestHandler {
	if clock == nil {
		clock = realClock{}
	}
	return &FileRetransmitRequestHandler{clock: clock}
}

func (h *FileRetransmitRequestHandler) MessageType() string {
	return sip.MessageTypeFileRetransmit
}

func (h *FileRetransmitRequestHandler) Handle(_ context.Context, req RequestContext, body []byte) (OutboundMessage, error) {
	var msg sip.FileRetransmitRequest
	if err := json.Unmarshal(body, &msg); err != nil {
		return OutboundMessage{}, fmt.Errorf("parse file.retransmit.request: %w", err)
	}
	return newFileAccepted(req.Header, msg.FileID, h.clock.Now())
}

type TaskCancelHandler struct {
	clock Clock
}

func NewTaskCancelHandler(clock Clock) *TaskCancelHandler {
	if clock == nil {
		clock = realClock{}
	}
	return &TaskCancelHandler{clock: clock}
}

func (h *TaskCancelHandler) MessageType() string {
	return sip.MessageTypeTaskCancel
}

func (h *TaskCancelHandler) Handle(_ context.Context, req RequestContext, body []byte) (OutboundMessage, error) {
	var msg sip.TaskCancel
	if err := json.Unmarshal(body, &msg); err != nil {
		return OutboundMessage{}, fmt.Errorf("parse task.cancel: %w", err)
	}

	ack := sip.CommandAccepted{
		Header:     acceptedHeader(req.Header, sip.MessageTypeCommandAccepted, h.clock.Now()),
		CommandID:  msg.TaskID,
		AcceptedAt: h.clock.Now(),
	}
	resp, err := json.Marshal(ack)
	if err != nil {
		return OutboundMessage{}, fmt.Errorf("marshal command.accepted: %w", err)
	}
	return OutboundMessage{Body: resp}, nil
}

func newFileAccepted(base sip.Header, fileID string, now time.Time) (OutboundMessage, error) {
	ack := sip.FileAccepted{
		Header:     acceptedHeader(base, sip.MessageTypeFileAccepted, now),
		FileID:     fileID,
		AcceptedAt: now,
	}
	resp, err := json.Marshal(ack)
	if err != nil {
		return OutboundMessage{}, fmt.Errorf("marshal file.accepted: %w", err)
	}
	return OutboundMessage{Body: resp}, nil
}

func acceptedHeader(base sip.Header, messageType string, now time.Time) sip.Header {
	base.MessageType = messageType
	base.Timestamp = now
	base.ExpireAt = now.Add(5 * time.Minute)
	return base
}
