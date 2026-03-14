package nodeconfig

import (
	"testing"

	"siptunnel/internal/config"
)

func TestLocalNodeConfigValidate(t *testing.T) {
	cfg := DefaultLocalNodeConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default local node config should be valid: %v", err)
	}

	cfg.SIPListenPort = 70000
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected invalid sip listen port")
	}
}

func TestPeerNodeConfigValidate(t *testing.T) {
	peer := PeerNodeConfig{
		PeerNodeID:           "peer-b",
		PeerName:             "Peer B",
		PeerSignalingIP:      "10.0.0.2",
		PeerSignalingPort:    5060,
		PeerMediaIP:          "10.0.0.2",
		PeerMediaPortStart:   30000,
		PeerMediaPortEnd:     30100,
		SupportedNetworkMode: config.NetworkModeAToBSIPBToARTP,
		Enabled:              true,
	}
	if err := peer.Validate(); err != nil {
		t.Fatalf("peer should be valid: %v", err)
	}

	peer.SupportedNetworkMode = "INVALID"
	if err := peer.Validate(); err == nil {
		t.Fatalf("expected invalid network mode")
	}
}
