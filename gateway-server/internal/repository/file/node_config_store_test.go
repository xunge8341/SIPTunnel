package file

import (
	"path/filepath"
	"testing"

	"siptunnel/internal/config"
	"siptunnel/internal/nodeconfig"
)

func TestNodeConfigStorePersistAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node_config.json")
	store, err := NewNodeConfigStore(path)
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}

	local := nodeconfig.DefaultLocalNodeConfig()
	local.NodeName = "Gateway A1"
	if _, err := store.UpdateLocalNode(local); err != nil {
		t.Fatalf("update local failed: %v", err)
	}

	peer := nodeconfig.PeerNodeConfig{
		PeerNodeID:           "peer-1",
		PeerName:             "Peer One",
		PeerSignalingIP:      "10.10.1.20",
		PeerSignalingPort:    5060,
		PeerMediaIP:          "10.10.1.20",
		PeerMediaPortStart:   32000,
		PeerMediaPortEnd:     32010,
		SupportedNetworkMode: config.NetworkModeAToBSIPBToARTP,
		Enabled:              true,
	}
	if _, err := store.CreatePeer(peer); err != nil {
		t.Fatalf("create peer failed: %v", err)
	}

	reloaded, err := NewNodeConfigStore(path)
	if err != nil {
		t.Fatalf("reload store failed: %v", err)
	}
	if got := reloaded.GetLocalNode().NodeName; got != "Gateway A1" {
		t.Fatalf("local node not persisted, got %q", got)
	}
	if len(reloaded.ListPeers()) != 1 {
		t.Fatalf("peers not persisted")
	}
}

func TestNodeConfigStorePeerCRUD(t *testing.T) {
	store, err := NewNodeConfigStore(filepath.Join(t.TempDir(), "node_config.json"))
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	peer := nodeconfig.PeerNodeConfig{
		PeerNodeID:           "peer-a",
		PeerName:             "Peer A",
		PeerSignalingIP:      "10.0.0.3",
		PeerSignalingPort:    5060,
		PeerMediaIP:          "10.0.0.3",
		PeerMediaPortStart:   30000,
		PeerMediaPortEnd:     30020,
		SupportedNetworkMode: config.NetworkModeAToBSIPBToARTP,
		Enabled:              true,
	}
	if _, err := store.CreatePeer(peer); err != nil {
		t.Fatalf("create peer failed: %v", err)
	}
	if _, err := store.CreatePeer(peer); err != ErrPeerAlreadyExists {
		t.Fatalf("expected duplicate error, got %v", err)
	}
	peer.PeerName = "Peer A Updated"
	if _, err := store.UpdatePeer(peer); err != nil {
		t.Fatalf("update peer failed: %v", err)
	}
	if err := store.DeletePeer(peer.PeerNodeID); err != nil {
		t.Fatalf("delete peer failed: %v", err)
	}
	if err := store.DeletePeer(peer.PeerNodeID); err != ErrPeerNotFound {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestNodeConfigStoreRejectIncompatiblePeerAndLocalMode(t *testing.T) {
	store, err := NewNodeConfigStore(filepath.Join(t.TempDir(), "node_config.json"))
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	peer := nodeconfig.PeerNodeConfig{
		PeerNodeID:           "peer-incompatible",
		PeerName:             "Peer Incompatible",
		PeerSignalingIP:      "10.0.0.3",
		PeerSignalingPort:    5060,
		PeerMediaIP:          "10.0.0.3",
		PeerMediaPortStart:   30000,
		PeerMediaPortEnd:     30020,
		SupportedNetworkMode: config.NetworkModeABBiDirSIPBiDirRTP,
		Enabled:              true,
	}
	if _, err := store.CreatePeer(peer); err == nil {
		t.Fatalf("expected incompatible peer error")
	}

	okPeer := peer
	okPeer.PeerNodeID = "peer-ok"
	okPeer.SupportedNetworkMode = config.NetworkModeAToBSIPBToARTP
	if _, err := store.CreatePeer(okPeer); err != nil {
		t.Fatalf("create compatible peer failed: %v", err)
	}

	local := nodeconfig.DefaultLocalNodeConfig()
	local.NetworkMode = config.NetworkModeABBiDirSIPBiDirRTP
	if _, err := store.UpdateLocalNode(local); err == nil {
		t.Fatalf("expected local mode incompatibility error")
	}
}
