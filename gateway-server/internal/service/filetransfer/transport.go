package filetransfer

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"siptunnel/internal/config"
	"siptunnel/internal/protocol/rtpfile"
)

const tcpFrameHeaderSize = 4

var (
	ErrRTPTransportNotBootstrapped = errors.New("rtp transport is not bootstrapped")
	ErrRTPTCPSessionLimit          = errors.New("rtp tcp session limit reached")
)

// TransportStats exposes transport-level runtime metrics.
type TransportStats struct {
	TCPSessionsCurrent  int64
	TCPSessionsTotal    uint64
	TCPReadErrorsTotal  uint64
	TCPWriteErrorsTotal uint64
}

// Transport defines the RTP data-plane transport boundary.
type Transport interface {
	Mode() string
	Bootstrap(cfg config.RTPConfig) error
	Snapshot() TransportStats
}

func NewTransport(mode string) (Transport, error) {
	switch strings.ToUpper(strings.TrimSpace(mode)) {
	case "", "UDP":
		return UDPTransport{}, nil
	case "TCP":
		return NewTCPTransport(), nil
	default:
		return nil, fmt.Errorf("unsupported rtp transport mode %q", mode)
	}
}

// UDPTransport is the current production implementation.
type UDPTransport struct{}

func (UDPTransport) Mode() string { return "UDP" }

func (UDPTransport) Bootstrap(_ config.RTPConfig) error { return nil }

func (UDPTransport) Snapshot() TransportStats { return TransportStats{} }

type tcpTransportMetrics struct {
	sessionsCurrent  atomic.Int64
	sessionsTotal    atomic.Uint64
	readErrorsTotal  atomic.Uint64
	writeErrorsTotal atomic.Uint64
}

// TCPTransport provides a controlled-release TCP framing transport for RTP packets.
type TCPTransport struct {
	mu      sync.RWMutex
	cfg     config.RTPConfig
	started bool
	slots   chan struct{}
	metrics tcpTransportMetrics
}

func NewTCPTransport() *TCPTransport {
	return &TCPTransport{}
}

func (t *TCPTransport) Mode() string { return "TCP" }

func (t *TCPTransport) Bootstrap(cfg config.RTPConfig) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if cfg.MaxTCPSessions <= 0 {
		return fmt.Errorf("rtp.max_tcp_sessions %d must be > 0", cfg.MaxTCPSessions)
	}
	t.cfg = cfg
	t.slots = make(chan struct{}, cfg.MaxTCPSessions)
	t.started = true
	return nil
}

func (t *TCPTransport) Snapshot() TransportStats {
	return TransportStats{
		TCPSessionsCurrent:  t.metrics.sessionsCurrent.Load(),
		TCPSessionsTotal:    t.metrics.sessionsTotal.Load(),
		TCPReadErrorsTotal:  t.metrics.readErrorsTotal.Load(),
		TCPWriteErrorsTotal: t.metrics.writeErrorsTotal.Load(),
	}
}

type TCPSession struct {
	conn      net.Conn
	cfg       config.RTPConfig
	transport *TCPTransport
	once      sync.Once
}

func (t *TCPTransport) OpenSession(conn net.Conn) (*TCPSession, error) {
	if conn == nil {
		return nil, errors.New("rtp tcp session conn is nil")
	}
	t.mu.RLock()
	cfg := t.cfg
	started := t.started
	slots := t.slots
	t.mu.RUnlock()
	if !started {
		return nil, ErrRTPTransportNotBootstrapped
	}
	select {
	case slots <- struct{}{}:
	default:
		return nil, ErrRTPTCPSessionLimit
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetKeepAlive(cfg.TCPKeepAliveEnabled)
	}
	t.metrics.sessionsCurrent.Add(1)
	t.metrics.sessionsTotal.Add(1)
	return &TCPSession{conn: conn, cfg: cfg, transport: t}, nil
}

func (s *TCPSession) Close() error {
	var err error
	s.once.Do(func() {
		err = s.conn.Close()
		s.transport.metrics.sessionsCurrent.Add(-1)
		<-s.transport.slots
	})
	return err
}

func (s *TCPSession) WritePacket(packet rtpfile.ChunkPacket) error {
	if s.cfg.TCPWriteTimeoutMS > 0 {
		_ = s.conn.SetWriteDeadline(time.Now().Add(time.Duration(s.cfg.TCPWriteTimeoutMS) * time.Millisecond))
	}
	payload, err := marshalChunkPacket(packet)
	if err != nil {
		s.transport.metrics.writeErrorsTotal.Add(1)
		return err
	}
	if len(payload) > s.cfg.MaxPacketBytes {
		s.transport.metrics.writeErrorsTotal.Add(1)
		return fmt.Errorf("rtp tcp frame too large: %d > %d", len(payload), s.cfg.MaxPacketBytes)
	}
	frame := make([]byte, tcpFrameHeaderSize+len(payload))
	binary.BigEndian.PutUint32(frame[:tcpFrameHeaderSize], uint32(len(payload)))
	copy(frame[tcpFrameHeaderSize:], payload)
	if _, err := s.conn.Write(frame); err != nil {
		s.transport.metrics.writeErrorsTotal.Add(1)
		return err
	}
	return nil
}

func (s *TCPSession) ReadPacket() (rtpfile.ChunkPacket, error) {
	if s.cfg.TCPReadTimeoutMS > 0 {
		_ = s.conn.SetReadDeadline(time.Now().Add(time.Duration(s.cfg.TCPReadTimeoutMS) * time.Millisecond))
	}
	var frameHeader [tcpFrameHeaderSize]byte
	if _, err := io.ReadFull(s.conn, frameHeader[:]); err != nil {
		s.transport.metrics.readErrorsTotal.Add(1)
		return rtpfile.ChunkPacket{}, err
	}
	length := binary.BigEndian.Uint32(frameHeader[:])
	if length == 0 {
		s.transport.metrics.readErrorsTotal.Add(1)
		return rtpfile.ChunkPacket{}, errors.New("rtp tcp frame length must be positive")
	}
	if int(length) > s.cfg.MaxPacketBytes {
		s.transport.metrics.readErrorsTotal.Add(1)
		return rtpfile.ChunkPacket{}, fmt.Errorf("rtp tcp frame too large: %d > %d", length, s.cfg.MaxPacketBytes)
	}
	payload := make([]byte, int(length))
	if _, err := io.ReadFull(s.conn, payload); err != nil {
		s.transport.metrics.readErrorsTotal.Add(1)
		return rtpfile.ChunkPacket{}, err
	}
	packet, err := unmarshalChunkPacket(payload)
	if err != nil {
		s.transport.metrics.readErrorsTotal.Add(1)
		return rtpfile.ChunkPacket{}, err
	}
	return packet, nil
}

func marshalChunkPacket(packet rtpfile.ChunkPacket) ([]byte, error) {
	if uint32(len(packet.Payload)) != packet.Header.ChunkLength {
		return nil, fmt.Errorf("chunk_length mismatch: payload=%d header=%d", len(packet.Payload), packet.Header.ChunkLength)
	}
	hdr, err := packet.Header.MarshalBinary()
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(hdr)+len(packet.Payload))
	copy(out, hdr)
	copy(out[len(hdr):], packet.Payload)
	return out, nil
}

func unmarshalChunkPacket(data []byte) (rtpfile.ChunkPacket, error) {
	var h rtpfile.Header
	if err := h.UnmarshalBinary(data); err != nil {
		return rtpfile.ChunkPacket{}, err
	}
	if int(h.HeaderLength) > len(data) {
		return rtpfile.ChunkPacket{}, fmt.Errorf("header_length overflow: %d > %d", h.HeaderLength, len(data))
	}
	payload := append([]byte(nil), data[int(h.HeaderLength):]...)
	if uint32(len(payload)) != h.ChunkLength {
		return rtpfile.ChunkPacket{}, fmt.Errorf("chunk_length mismatch: payload=%d header=%d", len(payload), h.ChunkLength)
	}
	return rtpfile.ChunkPacket{Header: h, Payload: payload}, nil
}
