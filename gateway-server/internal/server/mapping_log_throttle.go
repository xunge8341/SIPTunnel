package server

import (
	"strings"
	"sync"
	"time"
)

type throttledLogState struct {
	last time.Time
}

type mappingFailureLogThrottler struct {
	mu      sync.Mutex
	entries map[string]throttledLogState
	window  time.Duration
}

func newMappingFailureLogThrottler(window time.Duration) *mappingFailureLogThrottler {
	if window <= 0 {
		window = time.Second
	}
	return &mappingFailureLogThrottler{entries: map[string]throttledLogState{}, window: window}
}

func (t *mappingFailureLogThrottler) Allow(key string, now time.Time) bool {
	if t == nil {
		return true
	}
	key = strings.TrimSpace(strings.ToLower(key))
	if key == "" {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	state := t.entries[key]
	if !state.last.IsZero() && now.Sub(state.last) < t.window {
		return false
	}
	state.last = now
	t.entries[key] = state
	if len(t.entries) > 4096 {
		cutoff := now.Add(-2 * t.window)
		for k, v := range t.entries {
			if v.last.Before(cutoff) {
				delete(t.entries, k)
			}
		}
	}
	return true
}

var defaultMappingFailureLogThrottler = newMappingFailureLogThrottler(time.Second)

func shouldLogMappingFailure(key string, now time.Time) bool {
	return defaultMappingFailureLogThrottler.Allow(key, now)
}
