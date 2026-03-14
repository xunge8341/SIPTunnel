package file

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"siptunnel/internal/tunnelmapping"
)

var (
	ErrMappingNotFound = errors.New("mapping not found")
	ErrMappingExists   = errors.New("mapping already exists")
)

type TunnelMappingStore struct {
	path string
	mu   sync.RWMutex
	data map[string]tunnelmapping.TunnelMapping
}

func NewTunnelMappingStore(path string) (*TunnelMappingStore, error) {
	s := &TunnelMappingStore{path: path, data: map[string]tunnelmapping.TunnelMapping{}}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *TunnelMappingStore) List() []tunnelmapping.TunnelMapping {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]tunnelmapping.TunnelMapping, 0, len(s.data))
	for _, item := range s.data {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].MappingID < items[j].MappingID })
	return items
}

func (s *TunnelMappingStore) Create(m tunnelmapping.TunnelMapping) (tunnelmapping.TunnelMapping, error) {
	if err := m.Validate(); err != nil {
		return tunnelmapping.TunnelMapping{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[m.MappingID]; exists {
		return tunnelmapping.TunnelMapping{}, ErrMappingExists
	}
	s.data[m.MappingID] = m
	if err := s.persistLocked(); err != nil {
		return tunnelmapping.TunnelMapping{}, err
	}
	return m, nil
}

func (s *TunnelMappingStore) Update(id string, m tunnelmapping.TunnelMapping) (tunnelmapping.TunnelMapping, error) {
	if id != m.MappingID {
		return tunnelmapping.TunnelMapping{}, errors.New("mapping_id mismatch")
	}
	if err := m.Validate(); err != nil {
		return tunnelmapping.TunnelMapping{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[id]; !exists {
		return tunnelmapping.TunnelMapping{}, ErrMappingNotFound
	}
	s.data[id] = m
	if err := s.persistLocked(); err != nil {
		return tunnelmapping.TunnelMapping{}, err
	}
	return m, nil
}

func (s *TunnelMappingStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[id]; !exists {
		return ErrMappingNotFound
	}
	delete(s.data, id)
	return s.persistLocked()
}

func (s *TunnelMappingStore) load() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("mkdir mapping store dir: %w", err)
	}
	buf, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s.persistLocked()
		}
		return fmt.Errorf("read mapping store: %w", err)
	}
	if len(buf) == 0 {
		return nil
	}
	var payload struct {
		Items []tunnelmapping.TunnelMapping `json:"items"`
	}
	if err := json.Unmarshal(buf, &payload); err != nil {
		return fmt.Errorf("decode mapping store: %w", err)
	}
	for _, item := range payload.Items {
		s.data[item.MappingID] = item
	}
	return nil
}

func (s *TunnelMappingStore) persistLocked() error {
	items := make([]tunnelmapping.TunnelMapping, 0, len(s.data))
	for _, item := range s.data {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].MappingID < items[j].MappingID })
	payload, err := json.MarshalIndent(struct {
		Items []tunnelmapping.TunnelMapping `json:"items"`
	}{Items: items}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mapping store: %w", err)
	}
	return os.WriteFile(s.path, payload, 0o644)
}
