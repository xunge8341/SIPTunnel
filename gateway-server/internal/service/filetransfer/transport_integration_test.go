package filetransfer

import (
	"net"
	"testing"

	"siptunnel/internal/config"
)

func TestTCPTransportSessionWithReceiverIntegration(t *testing.T) {
	tcpTransport := NewTCPTransport()
	cfg := config.DefaultNetworkConfig().RTP
	cfg.MaxPacketBytes = 4096
	cfg.MaxTCPSessions = 4
	if err := tcpTransport.Bootstrap(cfg); err != nil {
		t.Fatalf("bootstrap error: %v", err)
	}

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	writer, err := tcpTransport.OpenSession(serverConn)
	if err != nil {
		t.Fatalf("open writer session: %v", err)
	}
	defer writer.Close()
	reader, err := tcpTransport.OpenSession(clientConn)
	if err != nil {
		t.Fatalf("open reader session: %v", err)
	}
	defer reader.Close()

	receiver := NewReceiver(t.TempDir())
	packets, payload := buildPackets(t, 9)

	go func() {
		for _, packet := range packets {
			if err := writer.WritePacket(packet); err != nil {
				t.Errorf("WritePacket error: %v", err)
				return
			}
		}
	}()

	var result *ReceiveResult
	for range packets {
		packet, err := reader.ReadPacket()
		if err != nil {
			t.Fatalf("ReadPacket error: %v", err)
		}
		result, err = receiver.AddChunk(packet)
		if err != nil {
			t.Fatalf("receiver AddChunk error: %v", err)
		}
	}
	if result == nil || !result.Completed {
		t.Fatalf("transfer should complete, result=%+v", result)
	}
	assertTempFileContent(t, result.FinalFilePath, payload)
}
