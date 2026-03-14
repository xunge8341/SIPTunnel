package file

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"siptunnel/internal/tunnelmapping"
)

var (
	ErrMappingNotFound = errors.New("mapping not found")
	ErrMappingExists   = errors.New("mapping already exists")
)

type TunnelMappingStore struct {
	path   string
	mu     sync.RWMutex
	data   map[string]tunnelmapping.TunnelMapping
	cursor int64
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
	m.Normalize()
	if strings.TrimSpace(m.MappingID) == "" {
		m.MappingID = strconv.FormatInt(s.nextID(), 10)
	}
	m.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
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
	m.Normalize()
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
	m.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
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
	items, cursor, migrated, err := decodeMappingPayload(buf)
	if err != nil {
		return fmt.Errorf("decode mapping store: %w", err)
	}
	if cursor > s.cursor {
		s.cursor = cursor
	}
	for _, item := range items {
		item.Normalize()
		if item.UpdatedAt == "" {
			item.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		}
		s.data[item.MappingID] = item
		if idNum, parseErr := strconv.ParseInt(item.MappingID, 10, 64); parseErr == nil && idNum > s.cursor {
			s.cursor = idNum
		}
	}
	if migrated {
		return s.persistLocked()
	}
	return nil
}

func decodeMappingPayload(buf []byte) ([]tunnelmapping.TunnelMapping, int64, bool, error) {
	var latest struct {
		Items  []tunnelmapping.TunnelMapping `json:"items"`
		Cursor int64                         `json:"cursor,omitempty"`
	}
	if err := json.Unmarshal(buf, &latest); err == nil {
		if len(latest.Items) > 0 || latest.Cursor > 0 {
			return latest.Items, latest.Cursor, false, nil
		}
		// 如果是空 payload，继续探测旧模型。
	}

	if hasLegacyRouteConfigShape(buf) {
		legacyRouteConfig := struct {
			Routes []tunnelmapping.LegacyRouteConfig `json:"routes"`
		}{}
		if err := json.Unmarshal(buf, &legacyRouteConfig); err == nil && len(legacyRouteConfig.Routes) > 0 {
			items, migrated, err := convertLegacyRouteConfigs(legacyRouteConfig.Routes)
			return items, 0, migrated, err
		}

		var routeArray []tunnelmapping.LegacyRouteConfig
		if err := json.Unmarshal(buf, &routeArray); err == nil && len(routeArray) > 0 {
			items, migrated, err := convertLegacyRouteConfigs(routeArray)
			return items, 0, migrated, err
		}
	}

	legacyOps := struct {
		Items  []tunnelmapping.LegacyOpsRoute `json:"items"`
		Routes []tunnelmapping.LegacyOpsRoute `json:"routes"`
	}{}
	if err := json.Unmarshal(buf, &legacyOps); err == nil {
		routes := legacyOps.Items
		if len(routes) == 0 {
			routes = legacyOps.Routes
		}
		if len(routes) > 0 {
			items, migrated, err := convertLegacyOpsRoutes(routes)
			return items, 0, migrated, err
		}
	}

	if len(latest.Items) == 0 {
		return []tunnelmapping.TunnelMapping{}, latest.Cursor, false, nil
	}
	return latest.Items, latest.Cursor, false, nil
}

func hasLegacyRouteConfigShape(buf []byte) bool {
	var probe struct {
		Routes []map[string]json.RawMessage `json:"routes"`
	}
	if err := json.Unmarshal(buf, &probe); err == nil && len(probe.Routes) > 0 {
		for _, item := range probe.Routes {
			if _, ok := item["target_host"]; ok {
				return true
			}
			if _, ok := item["target_port"]; ok {
				return true
			}
			if _, ok := item["target_service"]; ok {
				return true
			}
		}
		return false
	}

	var arr []map[string]json.RawMessage
	if err := json.Unmarshal(buf, &arr); err == nil && len(arr) > 0 {
		for _, item := range arr {
			if _, ok := item["target_host"]; ok {
				return true
			}
			if _, ok := item["target_port"]; ok {
				return true
			}
			if _, ok := item["target_service"]; ok {
				return true
			}
		}
	}
	return false
}

func convertLegacyOpsRoutes(routes []tunnelmapping.LegacyOpsRoute) ([]tunnelmapping.TunnelMapping, bool, error) {
	items := make([]tunnelmapping.TunnelMapping, 0, len(routes))
	for _, route := range routes {
		item, err := tunnelmapping.MappingFromLegacyOpsRoute(route)
		if err != nil {
			return nil, false, err
		}
		items = append(items, item)
	}
	return items, true, nil
}

func convertLegacyRouteConfigs(routes []tunnelmapping.LegacyRouteConfig) ([]tunnelmapping.TunnelMapping, bool, error) {
	items := make([]tunnelmapping.TunnelMapping, 0, len(routes))
	for _, route := range routes {
		item, err := tunnelmapping.MappingFromLegacyRouteConfig(route)
		if err != nil {
			return nil, false, err
		}
		items = append(items, item)
	}
	return items, true, nil
}

func (s *TunnelMappingStore) persistLocked() error {
	items := make([]tunnelmapping.TunnelMapping, 0, len(s.data))
	for _, item := range s.data {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].MappingID < items[j].MappingID })
	payload, err := json.MarshalIndent(struct {
		Items  []tunnelmapping.TunnelMapping `json:"items"`
		Cursor int64                         `json:"cursor"`
	}{Items: items, Cursor: s.cursor}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mapping store: %w", err)
	}
	return os.WriteFile(s.path, payload, 0o644)
}

func (s *TunnelMappingStore) nextID() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cursor++
	return s.cursor
}
