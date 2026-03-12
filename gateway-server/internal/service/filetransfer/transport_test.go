package filetransfer

import (
	"errors"
	"testing"

	"siptunnel/internal/config"
)

func TestNewTransportDefaultUDP(t *testing.T) {
	tr, err := NewTransport("")
	if err != nil {
		t.Fatalf("NewTransport error: %v", err)
	}
	if tr.Mode() != "UDP" {
		t.Fatalf("mode=%s, want UDP", tr.Mode())
	}
	if err := tr.Bootstrap(config.DefaultNetworkConfig().RTP); err != nil {
		t.Fatalf("udp bootstrap error: %v", err)
	}
}

func TestNewTransportTCPPlaceholder(t *testing.T) {
	tr, err := NewTransport("tcp")
	if err != nil {
		t.Fatalf("NewTransport error: %v", err)
	}
	if tr.Mode() != "TCP" {
		t.Fatalf("mode=%s, want TCP", tr.Mode())
	}
	if err := tr.Bootstrap(config.DefaultNetworkConfig().RTP); !errors.Is(err, ErrRTPTransportReserved) {
		t.Fatalf("expected ErrRTPTransportReserved, got %v", err)
	}
}

func TestNewTransportUnsupported(t *testing.T) {
	if _, err := NewTransport("SCTP"); err == nil {
		t.Fatal("expected error for unsupported mode")
	}
}
