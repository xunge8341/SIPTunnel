package file

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"siptunnel/internal/nodeconfig"
)

var (
	ErrPeerNotFound      = errors.New("peer not found")
	ErrPeerAlreadyExists = errors.New("peer already exists")
)

type NodeConfigStore struct {
	path string
	mu   sync.RWMutex
	data persistedData
}

type persistedData struct {
	LocalNode nodeconfig.LocalNodeConfig  `json:"local_node"`
	Peers     []nodeconfig.PeerNodeConfig `json:"peers"`
}

func NewNodeConfigStore(path string) (*NodeConfigStore, error) {
	store := &NodeConfigStore{path: filepath.Clean(path)}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *NodeConfigStore) GetLocalNode() nodeconfig.LocalNodeConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.LocalNode
}

func (s *NodeConfigStore) UpdateLocalNode(local nodeconfig.LocalNodeConfig) (nodeconfig.LocalNodeConfig, error) {
	local = local.Normalized()
	if err := local.Validate(); err != nil {
		return nodeconfig.LocalNodeConfig{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.LocalNode = local
	if err := s.persistLocked(); err != nil {
		return nodeconfig.LocalNodeConfig{}, err
	}
	return local, nil
}

func (s *NodeConfigStore) ListPeers() []nodeconfig.PeerNodeConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := append([]nodeconfig.PeerNodeConfig(nil), s.data.Peers...)
	sort.Slice(items, func(i, j int) bool { return items[i].PeerNodeID < items[j].PeerNodeID })
	return items
}

func (s *NodeConfigStore) CreatePeer(peer nodeconfig.PeerNodeConfig) (nodeconfig.PeerNodeConfig, error) {
	peer = peer.Normalized()
	if err := peer.Validate(); err != nil {
		return nodeconfig.PeerNodeConfig{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.data.Peers {
		if item.PeerNodeID == peer.PeerNodeID {
			return nodeconfig.PeerNodeConfig{}, ErrPeerAlreadyExists
		}
	}
	s.data.Peers = append(s.data.Peers, peer)
	if err := s.persistLocked(); err != nil {
		return nodeconfig.PeerNodeConfig{}, err
	}
	return peer, nil
}

func (s *NodeConfigStore) UpdatePeer(peer nodeconfig.PeerNodeConfig) (nodeconfig.PeerNodeConfig, error) {
	peer = peer.Normalized()
	if err := peer.Validate(); err != nil {
		return nodeconfig.PeerNodeConfig{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, item := range s.data.Peers {
		if item.PeerNodeID == peer.PeerNodeID {
			s.data.Peers[i] = peer
			if err := s.persistLocked(); err != nil {
				return nodeconfig.PeerNodeConfig{}, err
			}
			return peer, nil
		}
	}
	return nodeconfig.PeerNodeConfig{}, ErrPeerNotFound
}

func (s *NodeConfigStore) DeletePeer(peerNodeID string) error {
	id := strings.TrimSpace(peerNodeID)
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, item := range s.data.Peers {
		if item.PeerNodeID == id {
			s.data.Peers = append(s.data.Peers[:i], s.data.Peers[i+1:]...)
			return s.persistLocked()
		}
	}
	return ErrPeerNotFound
}

func (s *NodeConfigStore) load() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create node config dir: %w", err)
	}
	if _, err := os.Stat(s.path); errors.Is(err, os.ErrNotExist) {
		s.data = persistedData{LocalNode: nodeconfig.DefaultLocalNodeConfig(), Peers: []nodeconfig.PeerNodeConfig{}}
		return s.persistLocked()
	} else if err != nil {
		return fmt.Errorf("stat node config file: %w", err)
	}
	buf, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("read node config file: %w", err)
	}
	if len(buf) == 0 {
		s.data = persistedData{LocalNode: nodeconfig.DefaultLocalNodeConfig(), Peers: []nodeconfig.PeerNodeConfig{}}
		return s.persistLocked()
	}
	if err := json.Unmarshal(buf, &s.data); err != nil {
		return fmt.Errorf("unmarshal node config file: %w", err)
	}
	s.data.LocalNode = s.data.LocalNode.Normalized()
	if err := s.data.LocalNode.Validate(); err != nil {
		return fmt.Errorf("invalid local node in store: %w", err)
	}
	for i := range s.data.Peers {
		s.data.Peers[i] = s.data.Peers[i].Normalized()
		if err := s.data.Peers[i].Validate(); err != nil {
			return fmt.Errorf("invalid peer node in store: %w", err)
		}
	}
	return nil
}

func (s *NodeConfigStore) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create node config dir: %w", err)
	}
	payload, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal node config store: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), "node-config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp node config file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write temp node config file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp node config file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace node config file: %w", err)
	}
	return nil
}
