package siptcp

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestServerClientIntegration(t *testing.T) {
	metrics := &ConnectionMetrics{}
	srv := NewServer(Config{
		ListenAddress:        "127.0.0.1:0",
		ReadTimeout:          100 * time.Millisecond,
		WriteTimeout:         100 * time.Millisecond,
		IdleTimeout:          200 * time.Millisecond,
		MaxMessageBytes:      1024,
		MaxConnections:       2,
		TCPKeepAliveEnabled:  true,
		TCPKeepAliveInterval: 100 * time.Millisecond,
	}, MessageHandlerFunc(func(_ context.Context, _ ConnectionMeta, payload []byte) ([]byte, error) {
		return append([]byte("ack:"), payload...), nil
	}), slog.Default(), metrics)

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	})

	client, err := Dial(context.Background(), Config{ListenAddress: srv.Address(), ReadTimeout: time.Second, WriteTimeout: time.Second, MaxMessageBytes: 1024})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = client.Close() }()

	resp, err := client.Send(context.Background(), []byte(`{"message_type":"heartbeat"}`))
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if string(resp) != `ack:{"message_type":"heartbeat"}` {
		t.Fatalf("response=%s", string(resp))
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap := srv.MetricsSnapshot()
		if snap.AcceptedConnectionsTotal >= 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("metrics snapshot not updated: %+v", srv.MetricsSnapshot())
}
