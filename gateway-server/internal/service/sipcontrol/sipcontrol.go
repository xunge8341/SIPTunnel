package sipcontrol

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"siptunnel/internal/protocol/sip"
)

const (
	TransportTCP = "TCP"
	TransportUDP = "UDP"
)

func normalizeTransport(transport string) string {
	v := strings.ToUpper(strings.TrimSpace(transport))
	if v == TransportUDP {
		return TransportUDP
	}
	return TransportTCP
}

// Receiver 抽象 SIP 控制面接收端，便于后续替换真实 SIP 适配器。
type Receiver interface {
	Receive(ctx context.Context) (InboundMessage, error)
	Transport() string
}

// Sender 抽象 SIP 控制面发送端，便于后续替换真实 SIP 适配器。
type Sender interface {
	Send(ctx context.Context, msg OutboundMessage) error
	Transport() string
}

// Router 根据 message_type 将消息分发到不同 handler。
type Router interface {
	Route(ctx context.Context, msg InboundMessage) (OutboundMessage, error)
}

// Handler 定义不同消息类型处理骨架。
type Handler interface {
	MessageType() string
	Handle(ctx context.Context, req RequestContext, body []byte) (OutboundMessage, error)
}

type SignatureVerifier interface {
	Verify(payload []byte, signature string) bool
}

type Metrics interface {
	IncCounter(name string, labels map[string]string)
	ObserveDuration(name string, d time.Duration, labels map[string]string)
}

type InboundMessage struct {
	Body []byte
}

type OutboundMessage struct {
	Body []byte
}

type RequestContext struct {
	RequestID   string
	TraceID     string
	SessionID   string
	MessageType string
	Header      sip.Header
}

type Clock interface {
	Now() time.Time
}

type SecurityEvent struct {
	Category   string
	Reason     string
	RequestID  string
	TraceID    string
	SessionID  string
	OccurredAt time.Time
	Transport  string
}

var globalSecurityEventRecorder func(SecurityEvent)

func SetGlobalSecurityEventRecorder(recorder func(SecurityEvent)) {
	globalSecurityEventRecorder = recorder
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now().UTC()
}

type Dispatcher struct {
	handlers              map[string]Handler
	verifier              SignatureVerifier
	replay                ReplayGuard
	metrics               Metrics
	logger                *slog.Logger
	clock                 Clock
	timeSkew              time.Duration
	transport             string
	securityEventRecorder func(SecurityEvent)
	mutex                 sync.RWMutex
}

func NewDispatcher(verifier SignatureVerifier, logger *slog.Logger, metrics Metrics) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	if metrics == nil {
		metrics = NoopMetrics{}
	}
	return &Dispatcher{
		handlers:  make(map[string]Handler),
		verifier:  verifier,
		replay:    NewInMemoryReplayGuard(15 * time.Minute),
		metrics:   metrics,
		logger:    logger,
		clock:     realClock{},
		timeSkew:  5 * time.Minute,
		transport: TransportTCP,
	}
}

func (d *Dispatcher) RegisterHandler(h Handler) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.handlers[h.MessageType()] = h
}

func (d *Dispatcher) SetTransport(transport string) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.transport = normalizeTransport(transport)
}

func (d *Dispatcher) SetSecurityEventRecorder(recorder func(SecurityEvent)) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.securityEventRecorder = recorder
}

func (d *Dispatcher) recordSecurityEvent(header sip.Header, category, reason string) {
	d.mutex.RLock()
	recorder := d.securityEventRecorder
	transport := d.transport
	d.mutex.RUnlock()
	if recorder == nil {
		recorder = globalSecurityEventRecorder
	}
	if recorder == nil {
		return
	}
	recorder(SecurityEvent{Category: category, Reason: reason, RequestID: header.RequestID, TraceID: header.TraceID, SessionID: header.SessionID, OccurredAt: d.clock.Now(), Transport: transport})
}

func (d *Dispatcher) Transport() string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.transport
}

func (d *Dispatcher) Route(ctx context.Context, msg InboundMessage) (OutboundMessage, error) {
	startedAt := d.clock.Now()
	header, req, err := d.parseAndValidate(msg.Body)
	if err != nil {
		d.metrics.IncCounter("sip_control_route_total", map[string]string{"status": "rejected", "transport": d.transport})
		return OutboundMessage{}, err
	}

	d.logger.Info("sip control message received",
		"message_type", header.MessageType,
		"request_id", header.RequestID,
		"trace_id", header.TraceID,
		"session_id", header.SessionID,
		"transport", d.transport,
	)

	d.mutex.RLock()
	handler, ok := d.handlers[header.MessageType]
	d.mutex.RUnlock()
	if !ok {
		d.metrics.IncCounter("sip_control_route_total", map[string]string{"status": "unhandled", "message_type": header.MessageType, "transport": d.transport})
		return OutboundMessage{}, fmt.Errorf("no handler for message_type=%s", header.MessageType)
	}

	resp, err := handler.Handle(ctx, req, msg.Body)
	status := "success"
	if err != nil {
		status = "failed"
	}
	d.metrics.IncCounter("sip_control_route_total", map[string]string{"status": status, "message_type": header.MessageType, "transport": d.transport})
	d.metrics.ObserveDuration("sip_control_route_duration", d.clock.Now().Sub(startedAt), map[string]string{"message_type": header.MessageType, "transport": d.transport})
	if err != nil {
		d.logger.Error("sip control handler failed", "message_type", header.MessageType, "error", err)
		return OutboundMessage{}, err
	}

	d.logger.Info("sip control message handled",
		"message_type", header.MessageType,
		"request_id", header.RequestID,
		"trace_id", header.TraceID,
		"session_id", header.SessionID,
		"transport", d.transport,
	)
	return resp, nil
}

func (d *Dispatcher) parseAndValidate(body []byte) (sip.Header, RequestContext, error) {
	if len(body) == 0 || len(body) > sip.MaxBodyBytes() {
		d.recordSecurityEvent(sip.Header{}, "sip_payload", "payload size is invalid")
		return sip.Header{}, RequestContext{}, fmt.Errorf("payload size is invalid")
	}
	var header sip.Header
	if err := json.Unmarshal(body, &header); err != nil {
		d.recordSecurityEvent(sip.Header{}, "sip_parse", err.Error())
		return sip.Header{}, RequestContext{}, fmt.Errorf("parse body: %w", err)
	}

	now := d.clock.Now()
	if err := header.ValidateEnvelope(now); err != nil {
		d.recordSecurityEvent(header, "sip_envelope", err.Error())
		return sip.Header{}, RequestContext{}, err
	}
	if err := verifyPayloadDigest(body, header); err != nil {
		d.recordSecurityEvent(header, "sip_digest", err.Error())
		return sip.Header{}, RequestContext{}, err
	}

	if d.verifier != nil {
		signedPayload, err := payloadWithoutSignature(body)
		if err != nil {
			return sip.Header{}, RequestContext{}, err
		}
		if !d.verifier.Verify(signedPayload, header.Signature) {
			d.recordSecurityEvent(header, "sip_signature", "signature verification failed")
			return sip.Header{}, RequestContext{}, fmt.Errorf("signature verification failed")
		}
	}

	if d.replay != nil {
		if err := d.replay.Accept(header.RequestID, header.Nonce, header.ExpireAt, now); err != nil {
			d.recordSecurityEvent(header, "sip_replay", err.Error())
			return sip.Header{}, RequestContext{}, err
		}
	}

	return header, RequestContext{
		RequestID:   header.RequestID,
		TraceID:     header.TraceID,
		SessionID:   header.SessionID,
		MessageType: header.MessageType,
		Header:      header,
	}, nil
}

func verifyPayloadDigest(body []byte, header sip.Header) error {
	digestPayload, err := payloadWithoutFields(body, "signature", "payload_digest")
	if err != nil {
		return err
	}
	expected := ""
	switch strings.ToUpper(strings.TrimSpace(header.DigestAlg)) {
	case "MD5":
		sum := md5.Sum(digestPayload)
		expected = hex.EncodeToString(sum[:])
	case "SHA256":
		sum := sha256.Sum256(digestPayload)
		expected = hex.EncodeToString(sum[:])
	default:
		return fmt.Errorf("unsupported digest algorithm: %s", header.DigestAlg)
	}
	if !strings.EqualFold(strings.TrimSpace(header.PayloadDigest), expected) {
		return fmt.Errorf("payload digest mismatch")
	}
	return nil
}

func payloadWithoutFields(body []byte, remove ...string) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse body for canonical payload: %w", err)
	}
	for _, key := range remove {
		delete(payload, key)
	}
	canonical, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical payload: %w", err)
	}
	return canonical, nil
}

func payloadWithoutSignature(body []byte) ([]byte, error) {
	return payloadWithoutFields(body, "signature")
}

type NoopMetrics struct{}

func (NoopMetrics) IncCounter(_ string, _ map[string]string) {}

func (NoopMetrics) ObserveDuration(_ string, _ time.Duration, _ map[string]string) {}
