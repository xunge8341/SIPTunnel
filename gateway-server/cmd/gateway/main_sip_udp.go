package main

import (
	"context"
	"io"
	"net"
	"path/filepath"
	"strings"

	file "siptunnel/internal/repository/file"
	"siptunnel/internal/server"
	"siptunnel/internal/service/sipcontrol"
)

func (p packetConnCloser) Close() error {
	server.UnregisterSIPUDPTransport(p.PacketConn)
	return p.PacketConn.Close()
}

func startSIPUDPServer(ctx context.Context, addr string, dispatcher *sipcontrol.Dispatcher, dataDir string) (io.Closer, error) {
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		return nil, err
	}
	server.RegisterSIPUDPTransport(pc)
	go func() {
		buf := make([]byte, 256*1024)
		for {
			n, remote, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			payload := append([]byte(nil), buf[:n]...)
			if server.TryHandleSIPUDPResponse(remote.String(), payload) {
				continue
			}
			if !allowConfiguredPeer(remote.String(), dataDir) {
				continue
			}
			respBody, err := server.RouteSignalPacket(ctx, remote.String(), payload, func(ctx context.Context, payload []byte) ([]byte, error) {
				resp, err := dispatcher.Route(ctx, sipcontrol.InboundMessage{Body: payload})
				if err != nil {
					return nil, err
				}
				return resp.Body, nil
			})
			if err == nil && len(respBody) > 0 {
				_, _ = pc.WriteTo(respBody, remote)
			}
		}
	}()
	return packetConnCloser{pc}, nil
}

func allowConfiguredPeer(remoteAddr string, dataDir string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		host = strings.TrimSpace(remoteAddr)
	}
	if host == "127.0.0.1" || host == "::1" || host == "localhost" {
		return true
	}
	store, err := file.NewNodeConfigStore(filepath.Join(dataDir, "node_config.json"))
	if err != nil {
		return true
	}
	for _, peer := range store.ListPeers() {
		if !peer.Enabled {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(peer.PeerSignalingIP), host) {
			return true
		}
	}
	return false
}
