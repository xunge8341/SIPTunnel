package control

import (
	"encoding/json"
	"fmt"
)

// SIPBusinessMessage 使用 JSON body 承载完整业务字段。
type SIPBusinessMessage struct {
	TraceID   string                 `json:"trace_id"`
	RequestID string                 `json:"request_id"`
	ApiCode   string                 `json:"api_code"`
	Payload   map[string]any         `json:"payload"`
	Meta      map[string]string      `json:"meta,omitempty"`
	Audit     map[string]interface{} `json:"audit,omitempty"`
}

// SIPEnvelope Header 仅镜像索引字段，避免控制面承载复杂语义。
type SIPEnvelope struct {
	Headers map[string]string
	Body    []byte
}

func EncodeEnvelope(msg SIPBusinessMessage) (SIPEnvelope, error) {
	body, err := json.Marshal(msg)
	if err != nil {
		return SIPEnvelope{}, fmt.Errorf("marshal sip message: %w", err)
	}

	return SIPEnvelope{
		Headers: map[string]string{
			"X-Trace-ID":   msg.TraceID,
			"X-Request-ID": msg.RequestID,
			"X-Api-Code":   msg.ApiCode,
		},
		Body: body,
	}, nil
}

func DecodeEnvelope(env SIPEnvelope) (SIPBusinessMessage, error) {
	var msg SIPBusinessMessage
	if err := json.Unmarshal(env.Body, &msg); err != nil {
		return SIPBusinessMessage{}, fmt.Errorf("unmarshal sip message: %w", err)
	}
	return msg, nil
}
