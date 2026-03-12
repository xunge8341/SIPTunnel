package sipcontrol

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"siptunnel/internal/protocol/sip"
)

type fixedClock struct{ now time.Time }

func (f fixedClock) Now() time.Time { return f.now }

type verifierStub struct {
	errSig string
}

func (v verifierStub) Verify(_ []byte, signature string) bool {
	return signature != v.errSig
}

type metricRecord struct {
	name   string
	labels map[string]string
}

type metricsStub struct {
	inc []metricRecord
}

func (m *metricsStub) IncCounter(name string, labels map[string]string) {
	m.inc = append(m.inc, metricRecord{name: name, labels: labels})
}

func (m *metricsStub) ObserveDuration(_ string, _ time.Duration, _ map[string]string) {}

func TestDispatcherRouteCommandCreate(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	metrics := &metricsStub{}
	d := NewDispatcher(verifierStub{}, slog.Default(), metrics)
	d.clock = fixedClock{now: now}
	d.RegisterHandler(NewCommandCreateHandler(fixedClock{now: now}))

	body := mustMarshalCommandCreate(t, now, "sig-ok")
	resp, err := d.Route(context.Background(), InboundMessage{Body: body})
	if err != nil {
		t.Fatalf("Route() err=%v", err)
	}

	var accepted sip.CommandAccepted
	if err := json.Unmarshal(resp.Body, &accepted); err != nil {
		t.Fatalf("unmarshal response err=%v", err)
	}
	if accepted.MessageType != sip.MessageTypeCommandAccepted {
		t.Fatalf("unexpected response type %s", accepted.MessageType)
	}
	if accepted.CommandID != "cmd-1" {
		t.Fatalf("unexpected command id %s", accepted.CommandID)
	}
	if len(metrics.inc) == 0 || metrics.inc[len(metrics.inc)-1].labels["status"] != "success" {
		t.Fatalf("expected success metric, got %+v", metrics.inc)
	}
}

func TestDispatcherRejectsBadSignature(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	d := NewDispatcher(verifierStub{errSig: "bad"}, slog.Default(), &metricsStub{})
	d.clock = fixedClock{now: now}
	d.RegisterHandler(NewCommandCreateHandler(fixedClock{now: now}))

	_, err := d.Route(context.Background(), InboundMessage{Body: mustMarshalCommandCreate(t, now, "bad")})
	if err == nil {
		t.Fatalf("expected signature error")
	}
	if !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDispatcherUnhandledMessageType(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	d := NewDispatcher(verifierStub{}, slog.Default(), &metricsStub{})
	d.clock = fixedClock{now: now}

	_, err := d.Route(context.Background(), InboundMessage{Body: mustMarshalCommandCreate(t, now, "ok")})
	if err == nil {
		t.Fatalf("expected no handler error")
	}
}

func TestDispatcherRejectsReplayRequest(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	d := NewDispatcher(verifierStub{}, slog.Default(), &metricsStub{})
	d.clock = fixedClock{now: now}
	d.RegisterHandler(NewCommandCreateHandler(fixedClock{now: now}))

	body := mustMarshalCommandCreate(t, now, "sig-ok")
	if _, err := d.Route(context.Background(), InboundMessage{Body: body}); err != nil {
		t.Fatalf("first route failed: %v", err)
	}
	if _, err := d.Route(context.Background(), InboundMessage{Body: body}); err == nil {
		t.Fatalf("expected replay error")
	}
}

func mustMarshalCommandCreate(t *testing.T, now time.Time, signature string) []byte {
	t.Helper()
	msg := sip.CommandCreate{
		Header:     newHeader(sip.MessageTypeCommandCreate, now, signature),
		CommandID:  "cmd-1",
		Parameters: map[string]any{"k": "v"},
	}
	body, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal err=%v", err)
	}
	return body
}

func newHeader(msgType string, now time.Time, signature string) sip.Header {
	return sip.Header{
		ProtocolVersion: sip.ProtocolVersionV1,
		MessageType:     msgType,
		RequestID:       "req-1",
		TraceID:         "tr-1",
		SessionID:       "sess-1",
		ApiCode:         "API001",
		SourceSystem:    "sys-a",
		SourceNode:      "node-a",
		Timestamp:       now,
		ExpireAt:        now.Add(2 * time.Minute),
		Nonce:           "nonce",
		DigestAlg:       "SHA256",
		PayloadDigest:   "digest",
		SignAlg:         "HMAC_SHA256",
		Signature:       signature,
	}
}
