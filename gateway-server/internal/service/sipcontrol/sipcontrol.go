package sipcontrol

import (
	"context"
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

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now().UTC()
}

type Dispatcher struct {
	handlers  map[string]Handler
	verifier  SignatureVerifier
	replay    ReplayGuard
	metrics   Metrics
	logger    *slog.Logger
	clock     Clock
	timeSkew  time.Duration
	transport string
	mutex     sync.RWMutex
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
	var header sip.Header
	if err := json.Unmarshal(body, &header); err != nil {
		return sip.Header{}, RequestContext{}, fmt.Errorf("parse body: %w", err)
	}

	now := d.clock.Now()
	if err := validateTimeWindow(header, now, d.timeSkew); err != nil {
		return sip.Header{}, RequestContext{}, err
	}

	if d.verifier != nil {
		signedPayload, err := payloadWithoutSignature(body)
		if err != nil {
			return sip.Header{}, RequestContext{}, err
		}
		if !d.verifier.Verify(signedPayload, header.Signature) {
			return sip.Header{}, RequestContext{}, fmt.Errorf("signature verification failed")
		}
	}

	if d.replay != nil {
		if err := d.replay.Accept(header.RequestID, header.Nonce, header.ExpireAt, now); err != nil {
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

func validateTimeWindow(header sip.Header, now time.Time, skew time.Duration) error {
	if header.Timestamp.IsZero() || header.ExpireAt.IsZero() {
		return fmt.Errorf("timestamp/expire_at is required")
	}
	if !header.ExpireAt.After(header.Timestamp) {
		return fmt.Errorf("invalid time window")
	}
	if now.After(header.ExpireAt) {
		return fmt.Errorf("message expired")
	}
	if header.Timestamp.After(now.Add(skew)) {
		return fmt.Errorf("timestamp exceeds allowed skew")
	}
	return nil
}

func payloadWithoutSignature(body []byte) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse body for signature payload: %w", err)
	}
	delete(payload, "signature")
	canonical, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload for signature: %w", err)
	}
	return canonical, nil
}

type NoopMetrics struct{}

func (NoopMetrics) IncCounter(_ string, _ map[string]string) {}

func (NoopMetrics) ObserveDuration(_ string, _ time.Duration, _ map[string]string) {}
