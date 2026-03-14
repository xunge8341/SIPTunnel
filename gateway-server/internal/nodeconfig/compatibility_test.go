package nodeconfig

import (
	"testing"

	"siptunnel/internal/config"
)

func TestEvaluateCompatibilitySuccess(t *testing.T) {
	local := DefaultLocalNodeConfig()
	currentMode := config.NetworkModeAToBSIPBToARTP
	local.NetworkMode = currentMode
	peers := []PeerNodeConfig{{
		PeerNodeID:           "peer-1",
		PeerName:             "Peer One",
		PeerSignalingIP:      "10.0.0.2",
		PeerSignalingPort:    5060,
		PeerMediaIP:          "10.0.0.2",
		PeerMediaPortStart:   30000,
		PeerMediaPortEnd:     30100,
		SupportedNetworkMode: currentMode,
		Enabled:              true,
	}}
	status := EvaluateCompatibility(local, peers, currentMode, config.DeriveCapability(currentMode))
	if !status.LocalNodeConfigValid || !status.PeerNodeConfigValid || !status.NetworkCompatibility {
		t.Fatalf("expected full compatibility, got %+v", status)
	}
}

func TestEvaluateCompatibilityPeerMissingField(t *testing.T) {
	local := DefaultLocalNodeConfig()
	currentMode := config.NetworkModeAToBSIPBToARTP
	local.NetworkMode = currentMode
	peers := []PeerNodeConfig{{
		PeerNodeID:           "peer-bad",
		PeerName:             "",
		PeerSignalingIP:      "10.0.0.2",
		PeerSignalingPort:    5060,
		PeerMediaIP:          "10.0.0.2",
		PeerMediaPortStart:   30000,
		PeerMediaPortEnd:     30100,
		SupportedNetworkMode: currentMode,
		Enabled:              true,
	}}
	status := EvaluateCompatibility(local, peers, currentMode, config.DeriveCapability(currentMode))
	if status.PeerNodeConfigValid || status.NetworkCompatibility {
		t.Fatalf("expected peer invalid status, got %+v", status)
	}
	if len(status.IncompatiblePeerNodes) != 1 || status.IncompatiblePeerNodes[0] != "peer-bad" {
		t.Fatalf("expected incompatible peer id, got %+v", status.IncompatiblePeerNodes)
	}
}
