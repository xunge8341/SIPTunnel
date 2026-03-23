package server

import (
	"errors"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"siptunnel/internal/config"
	"siptunnel/internal/tunnelmapping"
)

type LocalResourceRecord struct {
	ResourceCode string   `json:"resource_code"`
	Name         string   `json:"name"`
	Enabled      bool     `json:"enabled"`
	TargetURL    string   `json:"target_url"`
	Methods      []string `json:"methods"`
	ResponseMode string   `json:"response_mode"`
	Description  string   `json:"description"`
	UpdatedAt    string   `json:"updated_at,omitempty"`
}

type localResourceStore interface {
	List() []LocalResourceRecord
	Create(LocalResourceRecord) (LocalResourceRecord, error)
	Update(code string, item LocalResourceRecord) (LocalResourceRecord, error)
	Delete(code string) error
}

type fileLocalResourceStore struct {
	mu    sync.RWMutex
	path  string
	items []LocalResourceRecord
}

func newFileLocalResourceStore(path string) (*fileLocalResourceStore, error) {
	s := &fileLocalResourceStore{path: path, items: []LocalResourceRecord{}}
	if buf, err := os.ReadFile(path); err == nil {
		var items []LocalResourceRecord
		if err := unmarshalSecureJSON(buf, &items); err != nil {
			return nil, err
		}
		s.items = normalizeLocalResourceList(items)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

func normalizeLocalResourceList(items []LocalResourceRecord) []LocalResourceRecord {
	out := make([]LocalResourceRecord, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		item = normalizeLocalResource(item)
		code := strings.TrimSpace(item.ResourceCode)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ResourceCode < out[j].ResourceCode })
	return out
}

func normalizeLocalResource(item LocalResourceRecord) LocalResourceRecord {
	item.ResourceCode = strings.TrimSpace(item.ResourceCode)
	item.Name = strings.TrimSpace(item.Name)
	item.TargetURL = strings.TrimSpace(item.TargetURL)
	item.ResponseMode = tunnelmapping.NormalizeResponseMode(item.ResponseMode)
	item.Methods = normalizeAllowedMethods(item.Methods)
	if item.Name == "" {
		item.Name = item.ResourceCode
	}
	if item.UpdatedAt == "" {
		item.UpdatedAt = formatTimestamp(time.Now())
	}
	return item
}

func validateLocalResource(item LocalResourceRecord) error {
	item = normalizeLocalResource(item)
	if !tunnelmapping.IsGBCode20(item.ResourceCode) {
		return errors.New("resource_code must be a 20-digit GB/T 28181 code")
	}
	if item.TargetURL == "" {
		return errors.New("target_url is required")
	}
	parsed, err := url.Parse(item.TargetURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("target_url is invalid")
	}
	if len(item.Methods) == 0 {
		return errors.New("methods is required")
	}
	return nil
}

func (s *fileLocalResourceStore) saveLocked() error {
	return saveJSON(s.path, s.items)
}

func (s *fileLocalResourceStore) List() []LocalResourceRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]LocalResourceRecord, len(s.items))
	copy(out, s.items)
	return out
}

func (s *fileLocalResourceStore) Create(item LocalResourceRecord) (LocalResourceRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item = normalizeLocalResource(item)
	if err := validateLocalResource(item); err != nil {
		return LocalResourceRecord{}, err
	}
	for _, existing := range s.items {
		if strings.EqualFold(existing.ResourceCode, item.ResourceCode) {
			return LocalResourceRecord{}, errors.New("resource_code already exists")
		}
	}
	item.UpdatedAt = formatTimestamp(time.Now())
	s.items = append(s.items, item)
	s.items = normalizeLocalResourceList(s.items)
	return item, s.saveLocked()
}

func (s *fileLocalResourceStore) Update(code string, item LocalResourceRecord) (LocalResourceRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	code = strings.TrimSpace(code)
	if code == "" {
		code = strings.TrimSpace(item.ResourceCode)
	}
	item = normalizeLocalResource(item)
	if item.ResourceCode == "" {
		item.ResourceCode = code
	}
	if err := validateLocalResource(item); err != nil {
		return LocalResourceRecord{}, err
	}
	idx := -1
	for i, existing := range s.items {
		if strings.EqualFold(existing.ResourceCode, code) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return LocalResourceRecord{}, os.ErrNotExist
	}
	for i, existing := range s.items {
		if i != idx && strings.EqualFold(existing.ResourceCode, item.ResourceCode) {
			return LocalResourceRecord{}, errors.New("resource_code already exists")
		}
	}
	item.UpdatedAt = formatTimestamp(time.Now())
	s.items[idx] = item
	s.items = normalizeLocalResourceList(s.items)
	return item, s.saveLocked()
}

func (s *fileLocalResourceStore) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	code = strings.TrimSpace(code)
	for i, existing := range s.items {
		if strings.EqualFold(existing.ResourceCode, code) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return os.ErrNotExist
	}
	s.items = append(s.items[:idx], s.items[idx+1:]...)
	return s.saveLocked()
}

func localResourceToVirtualResource(item LocalResourceRecord, mode config.NetworkMode) VirtualResource {
	profile := tunnelmapping.DeriveBodyLimitProfile(item.ResponseMode, config.DeriveCapability(mode).SupportsLargeRequestBody)
	return VirtualResource{
		DeviceID:              item.ResourceCode,
		Name:                  firstNonEmpty(strings.TrimSpace(item.Name), item.ResourceCode),
		Status:                boolStatus(item.Enabled),
		MethodList:            allowedMethods(item.Methods),
		ResponseMode:          normalizedResponseMode(item.ResponseMode),
		MaxInlineResponseBody: profile.MaxInlineResponseBody,
		MaxRequestBody:        profile.MaxRequestBodyBytes,
	}
}

func normalizeAllowedMethods(in []string) []string {
	if len(in) == 0 {
		return []string{"GET"}
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, method := range in {
		v := strings.ToUpper(strings.TrimSpace(method))
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	if len(out) == 0 {
		return []string{"GET"}
	}
	return out
}
