package server

import (
	"context"
	"net"
	"testing"
	"time"

	"siptunnel/internal/nodeconfig"
	"siptunnel/internal/service/filetransfer"
	"siptunnel/internal/service/siptcp"
)

func TestSendSIPPayloadTCPUsesRTPPortPool(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	defer ln.Close()

	pool, err := filetransfer.NewMemoryRTPPortPool(32100, 32110)
	if err != nil {
		t.Fatalf("new port pool: %v", err)
	}

	remotePortCh := make(chan int, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		if tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
			remotePortCh <- tcpAddr.Port
		}
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		frames, err := siptcp.NewFramer(64 * 1024).Feed(buf[:n])
		if err != nil || len(frames) == 0 {
			return
		}
		_, _ = conn.Write(siptcp.Encode([]byte("ok")))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := sendSIPPayload(ctx, "TCP", ln.Addr().String(), []byte(`{"ping":"pong"}`), nodeconfig.LocalNodeConfig{RTPListenIP: "127.0.0.1"}, pool, "req-tcp-1")
	if err != nil {
		t.Fatalf("sendSIPPayload tcp: %v", err)
	}
	if string(resp) != "ok" {
		t.Fatalf("resp=%q, want ok", string(resp))
	}

	select {
	case port := <-remotePortCh:
		if port < 32100 || port > 32110 {
			t.Fatalf("remote observed source port %d outside rtp range", port)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for remote source port")
	}

	if stats := pool.Stats(); stats.Used != 0 {
		t.Fatalf("port pool leak: %+v", stats)
	}
}

func TestSendSIPPayloadUDPUsesRTPPortPool(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer pc.Close()

	pool, err := filetransfer.NewMemoryRTPPortPool(32200, 32210)
	if err != nil {
		t.Fatalf("new port pool: %v", err)
	}

	remotePortCh := make(chan int, 1)
	go func() {
		buf := make([]byte, 4096)
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		if udpAddr, ok := addr.(*net.UDPAddr); ok {
			remotePortCh <- udpAddr.Port
		}
		_, _ = pc.WriteTo(buf[:n], addr)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := sendSIPPayload(ctx, "UDP", pc.LocalAddr().String(), []byte("ok"), nodeconfig.LocalNodeConfig{RTPListenIP: "127.0.0.1"}, pool, "req-udp-1")
	if err != nil {
		t.Fatalf("sendSIPPayload udp: %v", err)
	}
	if string(resp) != "ok" {
		t.Fatalf("resp=%q, want ok", string(resp))
	}

	select {
	case port := <-remotePortCh:
		if port < 32200 || port > 32210 {
			t.Fatalf("remote observed source port %d outside rtp range", port)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for remote source port")
	}

	if stats := pool.Stats(); stats.Used != 0 {
		t.Fatalf("port pool leak: %+v", stats)
	}
}
