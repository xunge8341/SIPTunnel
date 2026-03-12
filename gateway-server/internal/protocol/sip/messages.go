package sip

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	ProtocolVersionV1 = "1.0"

	MessageTypeCommandCreate                 = "command.create"
	MessageTypeCommandAccepted               = "command.accepted"
	MessageTypeFileCreate                    = "file.create"
	MessageTypeFileAccepted                  = "file.accepted"
	MessageTypeTaskStatus                    = "task.status"
	MessageTypeFileRetransmit                = "file.retransmit.request"
	MessageTypeTaskResult                    = "task.result"
	MessageTypeTaskCancel                    = "task.cancel"
	allowedClockSkew           time.Duration = 5 * time.Minute
)

// Header 定义 SIP 控制面 JSON 的公共字段。
type Header struct {
	ProtocolVersion string    `json:"protocol_version"`
	MessageType     string    `json:"message_type"`
	RequestID       string    `json:"request_id"`
	TraceID         string    `json:"trace_id"`
	SessionID       string    `json:"session_id"`
	ApiCode         string    `json:"api_code"`
	SourceSystem    string    `json:"source_system"`
	SourceNode      string    `json:"source_node"`
	Timestamp       time.Time `json:"timestamp"`
	ExpireAt        time.Time `json:"expire_at"`
	Nonce           string    `json:"nonce"`
	DigestAlg       string    `json:"digest_alg"`
	PayloadDigest   string    `json:"payload_digest"`
	SignAlg         string    `json:"sign_alg"`
	Signature       string    `json:"signature"`
}

func (h Header) ValidateForMessageType(expected string, now time.Time) error {
	required := map[string]string{
		"protocol_version": h.ProtocolVersion,
		"message_type":     h.MessageType,
		"request_id":       h.RequestID,
		"trace_id":         h.TraceID,
		"session_id":       h.SessionID,
		"api_code":         h.ApiCode,
		"source_system":    h.SourceSystem,
		"source_node":      h.SourceNode,
		"nonce":            h.Nonce,
		"digest_alg":       h.DigestAlg,
		"payload_digest":   h.PayloadDigest,
		"sign_alg":         h.SignAlg,
		"signature":        h.Signature,
	}
	for k, v := range required {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("%s is required", k)
		}
	}

	if h.ProtocolVersion != ProtocolVersionV1 {
		return fmt.Errorf("unsupported protocol_version: %s", h.ProtocolVersion)
	}
	if h.MessageType != expected {
		return fmt.Errorf("message_type mismatch: expect=%s got=%s", expected, h.MessageType)
	}
	if h.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}
	if h.ExpireAt.IsZero() {
		return fmt.Errorf("expire_at is required")
	}
	if h.ExpireAt.Before(h.Timestamp) || h.ExpireAt.Equal(h.Timestamp) {
		return fmt.Errorf("invalid time window: expire_at must be after timestamp")
	}
	if now.After(h.ExpireAt) {
		return fmt.Errorf("message expired at %s", h.ExpireAt.Format(time.RFC3339))
	}
	if h.Timestamp.After(now.Add(allowedClockSkew)) {
		return fmt.Errorf("timestamp is in the future beyond allowed skew")
	}

	return nil
}

// SIPHeaderMirrors 生成 SIP Header 镜像索引字段。
func (h Header) SIPHeaderMirrors() map[string]string {
	return map[string]string{
		"X-Request-ID":    h.RequestID,
		"X-Trace-ID":      h.TraceID,
		"X-Session-ID":    h.SessionID,
		"X-Api-Code":      h.ApiCode,
		"X-Message-Type":  h.MessageType,
		"X-Source-System": h.SourceSystem,
	}
}

type CommandCreate struct {
	Header
	CommandID  string         `json:"command_id"`
	Parameters map[string]any `json:"parameters"`
}

func (m CommandCreate) Validate() error {
	if err := m.Header.ValidateForMessageType(MessageTypeCommandCreate, time.Now().UTC()); err != nil {
		return err
	}
	if strings.TrimSpace(m.CommandID) == "" {
		return fmt.Errorf("command_id is required")
	}
	if m.Parameters == nil {
		return fmt.Errorf("parameters is required")
	}
	return nil
}

func (m CommandCreate) MarshalJSON() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	type alias CommandCreate
	return json.Marshal(alias(m))
}

func (m *CommandCreate) UnmarshalJSON(data []byte) error {
	type alias CommandCreate
	var v alias
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*m = CommandCreate(v)
	return m.Validate()
}

type CommandAccepted struct {
	Header
	CommandID  string    `json:"command_id"`
	AcceptedAt time.Time `json:"accepted_at"`
}

func (m CommandAccepted) Validate() error {
	if err := m.Header.ValidateForMessageType(MessageTypeCommandAccepted, time.Now().UTC()); err != nil {
		return err
	}
	if strings.TrimSpace(m.CommandID) == "" {
		return fmt.Errorf("command_id is required")
	}
	if m.AcceptedAt.IsZero() {
		return fmt.Errorf("accepted_at is required")
	}
	return nil
}

func (m CommandAccepted) MarshalJSON() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	type alias CommandAccepted
	return json.Marshal(alias(m))
}

func (m *CommandAccepted) UnmarshalJSON(data []byte) error {
	type alias CommandAccepted
	var v alias
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*m = CommandAccepted(v)
	return m.Validate()
}

type FileCreate struct {
	Header
	FileID    string `json:"file_id"`
	TaskID    string `json:"task_id"`
	TotalSize int64  `json:"total_size"`
	ChunkSize int    `json:"chunk_size"`
}

func (m FileCreate) Validate() error {
	if err := m.Header.ValidateForMessageType(MessageTypeFileCreate, time.Now().UTC()); err != nil {
		return err
	}
	if strings.TrimSpace(m.FileID) == "" {
		return fmt.Errorf("file_id is required")
	}
	if strings.TrimSpace(m.TaskID) == "" {
		return fmt.Errorf("task_id is required")
	}
	if m.TotalSize <= 0 {
		return fmt.Errorf("total_size must be positive")
	}
	if m.ChunkSize <= 0 {
		return fmt.Errorf("chunk_size must be positive")
	}
	return nil
}

func (m FileCreate) MarshalJSON() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	type alias FileCreate
	return json.Marshal(alias(m))
}

func (m *FileCreate) UnmarshalJSON(data []byte) error {
	type alias FileCreate
	var v alias
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*m = FileCreate(v)
	return m.Validate()
}

type FileAccepted struct {
	Header
	FileID     string    `json:"file_id"`
	AcceptedAt time.Time `json:"accepted_at"`
}

func (m FileAccepted) Validate() error {
	if err := m.Header.ValidateForMessageType(MessageTypeFileAccepted, time.Now().UTC()); err != nil {
		return err
	}
	if strings.TrimSpace(m.FileID) == "" {
		return fmt.Errorf("file_id is required")
	}
	if m.AcceptedAt.IsZero() {
		return fmt.Errorf("accepted_at is required")
	}
	return nil
}

func (m FileAccepted) MarshalJSON() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	type alias FileAccepted
	return json.Marshal(alias(m))
}

func (m *FileAccepted) UnmarshalJSON(data []byte) error {
	type alias FileAccepted
	var v alias
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*m = FileAccepted(v)
	return m.Validate()
}

type TaskStatus struct {
	Header
	TaskID   string `json:"task_id"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
}

func (m TaskStatus) Validate() error {
	if err := m.Header.ValidateForMessageType(MessageTypeTaskStatus, time.Now().UTC()); err != nil {
		return err
	}
	if strings.TrimSpace(m.TaskID) == "" {
		return fmt.Errorf("task_id is required")
	}
	if strings.TrimSpace(m.Status) == "" {
		return fmt.Errorf("status is required")
	}
	if m.Progress < 0 || m.Progress > 100 {
		return fmt.Errorf("progress must be in [0,100]")
	}
	return nil
}

func (m TaskStatus) MarshalJSON() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	type alias TaskStatus
	return json.Marshal(alias(m))
}

func (m *TaskStatus) UnmarshalJSON(data []byte) error {
	type alias TaskStatus
	var v alias
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*m = TaskStatus(v)
	return m.Validate()
}

type FileRetransmitRequest struct {
	Header
	FileID        string `json:"file_id"`
	MissingChunks []int  `json:"missing_chunks"`
}

func (m FileRetransmitRequest) Validate() error {
	if err := m.Header.ValidateForMessageType(MessageTypeFileRetransmit, time.Now().UTC()); err != nil {
		return err
	}
	if strings.TrimSpace(m.FileID) == "" {
		return fmt.Errorf("file_id is required")
	}
	if len(m.MissingChunks) == 0 {
		return fmt.Errorf("missing_chunks is required")
	}
	for _, chunk := range m.MissingChunks {
		if chunk < 0 {
			return fmt.Errorf("missing_chunks contains negative index")
		}
	}
	return nil
}

func (m FileRetransmitRequest) MarshalJSON() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	type alias FileRetransmitRequest
	return json.Marshal(alias(m))
}

func (m *FileRetransmitRequest) UnmarshalJSON(data []byte) error {
	type alias FileRetransmitRequest
	var v alias
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*m = FileRetransmitRequest(v)
	return m.Validate()
}

type TaskResult struct {
	Header
	TaskID  string         `json:"task_id"`
	Success bool           `json:"success"`
	Result  map[string]any `json:"result"`
}

func (m TaskResult) Validate() error {
	if err := m.Header.ValidateForMessageType(MessageTypeTaskResult, time.Now().UTC()); err != nil {
		return err
	}
	if strings.TrimSpace(m.TaskID) == "" {
		return fmt.Errorf("task_id is required")
	}
	if m.Result == nil {
		return fmt.Errorf("result is required")
	}
	return nil
}

func (m TaskResult) MarshalJSON() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	type alias TaskResult
	return json.Marshal(alias(m))
}

func (m *TaskResult) UnmarshalJSON(data []byte) error {
	type alias TaskResult
	var v alias
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*m = TaskResult(v)
	return m.Validate()
}

type TaskCancel struct {
	Header
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}

func (m TaskCancel) Validate() error {
	if err := m.Header.ValidateForMessageType(MessageTypeTaskCancel, time.Now().UTC()); err != nil {
		return err
	}
	if strings.TrimSpace(m.TaskID) == "" {
		return fmt.Errorf("task_id is required")
	}
	if strings.TrimSpace(m.Reason) == "" {
		return fmt.Errorf("reason is required")
	}
	return nil
}

func (m TaskCancel) MarshalJSON() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	type alias TaskCancel
	return json.Marshal(alias(m))
}

func (m *TaskCancel) UnmarshalJSON(data []byte) error {
	type alias TaskCancel
	var v alias
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*m = TaskCancel(v)
	return m.Validate()
}
