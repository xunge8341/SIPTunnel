package server

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"siptunnel/internal/config"
	"siptunnel/internal/nodeconfig"
	"siptunnel/internal/protocol/manscdp"
	"siptunnel/internal/protocol/siptext"
	"siptunnel/internal/service/filetransfer"
)

type gb28181Registrar struct {
	nodeStore       nodeConfigStore
	preferredPeerID func() string
	localNode       func() nodeconfig.LocalNodeConfig
	tunnelConfig    func() TunnelConfigPayload
	portPool        filetransfer.RTPPortPool

	mu        sync.Mutex
	challenge sipDigestChallenge
}

func (r *gb28181Registrar) currentConfig() TunnelConfigPayload {
	if r == nil || r.tunnelConfig == nil {
		return TunnelConfigPayload{}
	}
	return normalizeTunnelConfigPayload(r.tunnelConfig(), config.DefaultNetworkMode())
}

func (r *gb28181Registrar) targetPeer() (nodeconfig.PeerNodeConfig, error) {
	if r == nil || r.nodeStore == nil {
		return nodeconfig.PeerNodeConfig{}, fmt.Errorf("node store not configured")
	}
	peers := r.nodeStore.ListPeers()
	if len(peers) == 0 {
		return nodeconfig.PeerNodeConfig{}, fmt.Errorf("peer node not configured")
	}
	preferredID := ""
	if r.preferredPeerID != nil {
		preferredID = strings.TrimSpace(r.preferredPeerID())
	}
	var peer nodeconfig.PeerNodeConfig
	selected := false
	if preferredID != "" {
		for _, item := range peers {
			if item.Enabled && strings.EqualFold(strings.TrimSpace(item.PeerNodeID), preferredID) {
				peer = item
				selected = true
				break
			}
		}
	}
	if !selected {
		enabled := make([]nodeconfig.PeerNodeConfig, 0, len(peers))
		for _, item := range peers {
			if item.Enabled {
				enabled = append(enabled, item)
			}
		}
		if len(enabled) == 0 {
			return nodeconfig.PeerNodeConfig{}, fmt.Errorf("no enabled peer node configured")
		}
		if len(enabled) > 1 {
			ids := make([]string, 0, len(enabled))
			for _, item := range enabled {
				ids = append(ids, item.PeerNodeID)
			}
			return nodeconfig.PeerNodeConfig{}, fmt.Errorf("multiple enabled peer nodes configured (%s); current single-binding mode requires exactly one or an explicit peer binding", strings.Join(ids, ","))
		}
		peer = enabled[0]
	}
	if strings.TrimSpace(peer.PeerSignalingIP) == "" || peer.PeerSignalingPort <= 0 {
		return nodeconfig.PeerNodeConfig{}, fmt.Errorf("peer signaling endpoint not configured")
	}
	return peer, nil
}

func (r *gb28181Registrar) Register(ctx context.Context, authenticated bool) (int, string, error) {
	peer, err := r.targetPeer()
	if err != nil {
		return 0, "", err
	}
	local := localNodeValue(r.localNode)
	cfg := r.currentConfig()
	callID := fmt.Sprintf("register-%d", time.Now().UTC().UnixNano())
	remoteAddr := net.JoinHostPort(strings.TrimSpace(peer.PeerSignalingIP), strconv.Itoa(peer.PeerSignalingPort))
	state := newOutboundDialogState(local, remoteAddr, strings.TrimSpace(peer.PeerNodeID), strings.ToUpper(strings.TrimSpace(local.SIPTransport)), callID)
	req := siptext.NewRequest("REGISTER", state.remoteURI)
	fillOutboundDialogHeaders(req, state, local, 1, "REGISTER")
	expiresSec := cfg.CatalogSubscribeExpiresSec
	if expiresSec <= 0 {
		expiresSec = 3600
	}
	req.SetHeader("Expires", strconv.Itoa(expiresSec))
	if authenticated && cfg.RegisterAuthEnabled && strings.TrimSpace(cfg.RegisterAuthPassword) != "" {
		r.mu.Lock()
		challenge := r.challenge
		r.mu.Unlock()
		if strings.TrimSpace(challenge.Nonce) != "" {
			username := strings.TrimSpace(cfg.RegisterAuthUsername)
			if username == "" {
				username = strings.TrimSpace(local.NodeID)
			}
			req.SetHeader("Authorization", buildSIPDigestAuthorization(req.Method, req.RequestURI, username, strings.TrimSpace(cfg.RegisterAuthPassword), challenge))
		}
	}
	resp, err := r.sendAndParse(ctx, peer, req, callID)
	if err != nil {
		return 0, "", err
	}
	if resp.StatusCode == 401 {
		if ch, ok := parseSIPDigestChallenge(firstNonEmpty(resp.Header("WWW-Authenticate"), resp.Header("Www-Authenticate"))); ok {
			r.mu.Lock()
			r.challenge = ch
			r.mu.Unlock()
		}
	}
	return resp.StatusCode, firstNonEmpty(resp.ReasonPhrase, "registered"), nil
}

func (r *gb28181Registrar) Heartbeat(ctx context.Context) error {
	peer, err := r.targetPeer()
	if err != nil {
		return err
	}
	local := localNodeValue(r.localNode)
	callID := fmt.Sprintf("keepalive-%d", time.Now().UTC().UnixNano())
	remoteAddr := net.JoinHostPort(strings.TrimSpace(peer.PeerSignalingIP), strconv.Itoa(peer.PeerSignalingPort))
	body, err := manscdp.Marshal(manscdp.KeepaliveNotify{CmdType: "Keepalive", SN: 1, DeviceID: strings.TrimSpace(local.NodeID), Status: "OK"})
	if err != nil {
		return err
	}
	state := newOutboundDialogState(local, remoteAddr, strings.TrimSpace(peer.PeerNodeID), strings.ToUpper(strings.TrimSpace(local.SIPTransport)), callID)
	msg := siptext.NewRequest("MESSAGE", state.remoteURI)
	fillOutboundDialogHeaders(msg, state, local, 1, "MESSAGE")
	msg.SetHeader("Content-Type", manscdp.ContentType)
	msg.Body = body
	resp, err := r.sendAndParse(ctx, peer, msg, callID)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("keepalive rejected: %d %s", resp.StatusCode, resp.ReasonPhrase)
	}
	return nil
}

func (r *gb28181Registrar) sendAndParse(ctx context.Context, peer nodeconfig.PeerNodeConfig, msg *siptext.Message, requestID string) (*siptext.Message, error) {
	local := localNodeValue(r.localNode)
	transport := strings.ToUpper(strings.TrimSpace(local.SIPTransport))
	if transport == "" {
		transport = "TCP"
	}
	remoteAddr := net.JoinHostPort(strings.TrimSpace(peer.PeerSignalingIP), strconv.Itoa(peer.PeerSignalingPort))
	raw, err := sendSIPPayload(ctx, transport, remoteAddr, msg.Bytes(), local, r.portPool, requestID)
	if err != nil {
		return nil, err
	}
	resp, err := siptext.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse sip response: %w", err)
	}
	if resp.IsRequest {
		return nil, fmt.Errorf("unexpected sip request while waiting response")
	}
	return resp, nil
}
