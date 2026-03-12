package filetransfer

import (
	"errors"
	"fmt"
	"strings"

	"siptunnel/internal/config"
)

var ErrRTPTransportReserved = errors.New("rtp tcp transport is reserved for future release")

// Transport defines the RTP data-plane transport boundary.
//
// Current production path uses UDP implementation by default.
// TCP implementation is intentionally a placeholder for later extension,
// so boundary-dependent code can evolve without changing upper modules.
type Transport interface {
	Mode() string
	Bootstrap(cfg config.RTPConfig) error
}

func NewTransport(mode string) (Transport, error) {
	switch strings.ToUpper(strings.TrimSpace(mode)) {
	case "", "UDP":
		return UDPTransport{}, nil
	case "TCP":
		return TCPTransport{}, nil
	default:
		return nil, fmt.Errorf("unsupported rtp transport mode %q", mode)
	}
}

// UDPTransport is the current production implementation.
type UDPTransport struct{}

func (UDPTransport) Mode() string { return "UDP" }

func (UDPTransport) Bootstrap(_ config.RTPConfig) error { return nil }

// TCPTransport is a reserved placeholder.
// It exposes interface shape only and returns a reserved error on bootstrap.
type TCPTransport struct{}

func (TCPTransport) Mode() string { return "TCP" }

func (TCPTransport) Bootstrap(_ config.RTPConfig) error { return ErrRTPTransportReserved }
