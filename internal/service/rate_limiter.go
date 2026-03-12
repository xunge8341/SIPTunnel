package service

import (
	"sync"
	"time"
)

type RateLimiter struct {
	mu       sync.Mutex
	capacity float64
	tokens   float64
	rate     float64
	last     time.Time
}

func NewRateLimiter(rps int, burst int) *RateLimiter {
	now := time.Now()
	return &RateLimiter{capacity: float64(burst), tokens: float64(burst), rate: float64(rps), last: now}
}

func (l *RateLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(l.last).Seconds()
	l.last = now
	l.tokens += elapsed * l.rate
	if l.tokens > l.capacity {
		l.tokens = l.capacity
	}
	if l.tokens < 1 {
		return false
	}
	l.tokens -= 1
	return true
}
