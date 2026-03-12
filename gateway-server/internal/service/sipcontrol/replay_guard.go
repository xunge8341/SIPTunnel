package sipcontrol

import (
	"fmt"
	"sync"
	"time"
)

type ReplayGuard interface {
	Accept(requestID string, nonce string, expireAt time.Time, now time.Time) error
}

type replayRecord struct {
	expiresAt time.Time
}

type InMemoryReplayGuard struct {
	mu         sync.Mutex
	ttl        time.Duration
	byRequest  map[string]replayRecord
	byNonceKey map[string]replayRecord
}

func NewInMemoryReplayGuard(ttl time.Duration) *InMemoryReplayGuard {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &InMemoryReplayGuard{
		ttl:        ttl,
		byRequest:  map[string]replayRecord{},
		byNonceKey: map[string]replayRecord{},
	}
}

func (g *InMemoryReplayGuard) Accept(requestID string, nonce string, expireAt time.Time, now time.Time) error {
	if requestID == "" || nonce == "" {
		return fmt.Errorf("request_id and nonce are required")
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	g.gc(now)
	if _, exists := g.byRequest[requestID]; exists {
		return fmt.Errorf("replay detected: request_id already processed")
	}
	if _, exists := g.byNonceKey[nonce]; exists {
		return fmt.Errorf("replay detected: nonce already processed")
	}

	exp := expireAt
	if !exp.After(now) {
		exp = now.Add(g.ttl)
	}
	rec := replayRecord{expiresAt: exp}
	g.byRequest[requestID] = rec
	g.byNonceKey[nonce] = rec
	return nil
}

func (g *InMemoryReplayGuard) gc(now time.Time) {
	for key, record := range g.byRequest {
		if now.After(record.expiresAt) {
			delete(g.byRequest, key)
		}
	}
	for key, record := range g.byNonceKey {
		if now.After(record.expiresAt) {
			delete(g.byNonceKey, key)
		}
	}
}
