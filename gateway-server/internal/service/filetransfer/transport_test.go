package filetransfer

import (
	"errors"
	"io"
	"net"
	"testing"

	"siptunnel/internal/config"
	"siptunnel/internal/protocol/rtpfile"
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
	if got := tr.Snapshot(); got != (TransportStats{}) {
		t.Fatalf("udp snapshot=%+v, want zero", got)
	}
}

func TestNewTransportTCPBootstrapAndSessionLifecycle(t *testing.T) {
	tr, err := NewTransport("tcp")
	if err != nil {
		t.Fatalf("NewTransport error: %v", err)
	}
	tcpTransport, ok := tr.(*TCPTransport)
	if !ok {
		t.Fatalf("type=%T, want *TCPTransport", tr)
	}
	cfg := config.DefaultNetworkConfig().RTP
	cfg.MaxPacketBytes = 2048
	cfg.MaxTCPSessions = 1
	if err := tcpTransport.Bootstrap(cfg); err != nil {
		t.Fatalf("bootstrap error: %v", err)
	}

	c1, c2 := net.Pipe()
	session, err := tcpTransport.OpenSession(c1)
	if err != nil {
		t.Fatalf("OpenSession error: %v", err)
	}
	defer c2.Close()

	if _, err := tcpTransport.OpenSession(c2); !errors.Is(err, ErrRTPTCPSessionLimit) {
		t.Fatalf("OpenSession limit error=%v", err)
	}

	if err := session.Close(); err != nil {
		t.Fatalf("session close error: %v", err)
	}
	snapshot := tcpTransport.Snapshot()
	if snapshot.TCPSessionsCurrent != 0 || snapshot.TCPSessionsTotal != 1 {
		t.Fatalf("snapshot=%+v", snapshot)
	}
}

func TestTCPSessionWriteReadPacket(t *testing.T) {
	tcpTransport := NewTCPTransport()
	cfg := config.DefaultNetworkConfig().RTP
	cfg.MaxPacketBytes = 4096
	if err := tcpTransport.Bootstrap(cfg); err != nil {
		t.Fatalf("bootstrap error: %v", err)
	}

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	writer, err := tcpTransport.OpenSession(serverConn)
	if err != nil {
		t.Fatalf("writer OpenSession error: %v", err)
	}
	defer writer.Close()
	reader, err := tcpTransport.OpenSession(clientConn)
	if err != nil {
		t.Fatalf("reader OpenSession error: %v", err)
	}
	defer reader.Close()

	packets, _ := buildPackets(t, 8)
	go func() {
		if err := writer.WritePacket(packets[0]); err != nil {
			t.Errorf("write packet 0 error: %v", err)
		}
		if err := writer.WritePacket(packets[1]); err != nil {
			t.Errorf("write packet 1 error: %v", err)
		}
	}()

	for i := 0; i < 2; i++ {
		got, err := reader.ReadPacket()
		if err != nil {
			t.Fatalf("ReadPacket(%d) error: %v", i, err)
		}
		if got.Header.ChunkNo != packets[i].Header.ChunkNo {
			t.Fatalf("chunk no got=%d want=%d", got.Header.ChunkNo, packets[i].Header.ChunkNo)
		}
	}
}

func TestTCPSessionReadErrorMetrics(t *testing.T) {
	tcpTransport := NewTCPTransport()
	cfg := config.DefaultNetworkConfig().RTP
	cfg.MaxPacketBytes = 16
	cfg.MaxTCPSessions = 1
	if err := tcpTransport.Bootstrap(cfg); err != nil {
		t.Fatalf("bootstrap error: %v", err)
	}

	server, client := net.Pipe()
	session, err := tcpTransport.OpenSession(server)
	if err != nil {
		t.Fatalf("OpenSession error: %v", err)
	}
	defer session.Close()
	defer client.Close()

	go func() {
		_, _ = client.Write([]byte{0, 0, 0, 20})
		_, _ = client.Write(make([]byte, 20))
		client.Close()
	}()

	if _, err := session.ReadPacket(); err == nil {
		t.Fatal("expected read packet error")
	}
	if got := tcpTransport.Snapshot().TCPReadErrorsTotal; got == 0 {
		t.Fatalf("TCPReadErrorsTotal=%d, want >0", got)
	}
}

func TestMarshalUnmarshalChunkPacketRoundTrip(t *testing.T) {
	packets, _ := buildPackets(t, 12)
	wire, err := marshalChunkPacket(packets[0])
	if err != nil {
		t.Fatalf("marshalChunkPacket error: %v", err)
	}
	decoded, err := unmarshalChunkPacket(wire)
	if err != nil {
		t.Fatalf("unmarshalChunkPacket error: %v", err)
	}
	if decoded.Header.ChunkNo != packets[0].Header.ChunkNo {
		t.Fatalf("chunk_no got=%d want=%d", decoded.Header.ChunkNo, packets[0].Header.ChunkNo)
	}
	if decoded.Header.ChunkLength != uint32(len(decoded.Payload)) {
		t.Fatalf("chunk length mismatch")
	}
}

func TestMarshalChunkPacketValidatesChunkLength(t *testing.T) {
	packet := rtpfile.ChunkPacket{Header: rtpfile.Header{ChunkLength: 2}, Payload: []byte{1}}
	if _, err := marshalChunkPacket(packet); err == nil {
		t.Fatal("expected chunk length mismatch")
	}
}

func TestTCPSessionWriteErrorMetrics(t *testing.T) {
	tcpTransport := NewTCPTransport()
	cfg := config.DefaultNetworkConfig().RTP
	cfg.MaxPacketBytes = 32
	cfg.MaxTCPSessions = 2
	if err := tcpTransport.Bootstrap(cfg); err != nil {
		t.Fatalf("bootstrap error: %v", err)
	}

	server, client := net.Pipe()
	session, err := tcpTransport.OpenSession(server)
	if err != nil {
		t.Fatalf("OpenSession error: %v", err)
	}
	defer session.Close()
	_ = client.Close()

	packets, _ := buildPackets(t, 100)
	err = session.WritePacket(packets[0])
	if err == nil {
		t.Fatal("expected write error")
	}
	if !errors.Is(err, io.ErrClosedPipe) && tcpTransport.Snapshot().TCPWriteErrorsTotal == 0 {
		t.Fatalf("expected write error metric, err=%v snapshot=%+v", err, tcpTransport.Snapshot())
	}
}
