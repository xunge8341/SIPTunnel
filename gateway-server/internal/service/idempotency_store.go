package service

import "sync"

type IdempotencyStore struct {
	mu   sync.RWMutex
	seen map[string]struct{}
}

func NewIdempotencyStore() *IdempotencyStore {
	return &IdempotencyStore{seen: map[string]struct{}{}}
}

// MarkOnce returns true when first seen, false for duplicated request.
func (s *IdempotencyStore) MarkOnce(requestID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.seen[requestID]; ok {
		return false
	}
	s.seen[requestID] = struct{}{}
	return true
}
