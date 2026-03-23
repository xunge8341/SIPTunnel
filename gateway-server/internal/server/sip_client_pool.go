package server

import (
	"context"
	"sync"
	"time"

	"siptunnel/internal/service/siptcp"
)

type sipClientPoolKey struct {
	RemoteAddr      string
	LocalBindIP     string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	MaxMessageBytes int
}

type sipClientPoolEntry struct {
	client   *siptcp.TCPClient
	lastUsed time.Time
}

type sipClientPool struct {
	mu            sync.Mutex
	idle          map[sipClientPoolKey][]sipClientPoolEntry
	maxIdlePerKey int
	idleTimeout   time.Duration
}

type sipClientLease struct {
	pool   *sipClientPool
	key    sipClientPoolKey
	client *siptcp.TCPClient
	broken bool
	reused bool
}

var globalSIPClientPool = newSIPClientPool(8, 90*time.Second)

func newSIPClientPool(maxIdlePerKey int, idleTimeout time.Duration) *sipClientPool {
	if maxIdlePerKey <= 0 {
		maxIdlePerKey = 8
	}
	if idleTimeout <= 0 {
		idleTimeout = 90 * time.Second
	}
	return &sipClientPool{idle: make(map[sipClientPoolKey][]sipClientPoolEntry), maxIdlePerKey: maxIdlePerKey, idleTimeout: idleTimeout}
}

func (p *sipClientPool) acquire(ctx context.Context, cfg siptcp.Config) (*sipClientLease, error) {
	key := sipClientPoolKey{
		RemoteAddr:      cfg.ListenAddress,
		LocalBindIP:     cfg.LocalBindIP,
		ReadTimeout:     cfg.ReadTimeout,
		WriteTimeout:    cfg.WriteTimeout,
		MaxMessageBytes: cfg.MaxMessageBytes,
	}
	if client := p.popIdle(key); client != nil {
		return &sipClientLease{pool: p, key: key, client: client, reused: true}, nil
	}
	client, err := siptcp.Dial(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &sipClientLease{pool: p, key: key, client: client}, nil
}

func (p *sipClientPool) popIdle(key sipClientPoolKey) *siptcp.TCPClient {
	p.mu.Lock()
	defer p.mu.Unlock()
	entries := p.idle[key]
	if len(entries) == 0 {
		return nil
	}
	now := time.Now()
	for len(entries) > 0 {
		last := entries[len(entries)-1]
		entries = entries[:len(entries)-1]
		if now.Sub(last.lastUsed) > p.idleTimeout {
			_ = last.client.Close()
			continue
		}
		p.idle[key] = entries
		return last.client
	}
	delete(p.idle, key)
	return nil
}

func (p *sipClientPool) release(key sipClientPoolKey, client *siptcp.TCPClient, broken bool) {
	if client == nil {
		return
	}
	if broken {
		_ = client.Close()
		return
	}
	p.mu.Lock()
	entries := p.idle[key]
	if len(entries) >= p.maxIdlePerKey {
		p.mu.Unlock()
		_ = client.Close()
		return
	}
	p.idle[key] = append(entries, sipClientPoolEntry{client: client, lastUsed: time.Now()})
	p.mu.Unlock()
}

func (l *sipClientLease) MarkBroken() {
	if l != nil {
		l.broken = true
	}
}

func (l *sipClientLease) Close() {
	if l == nil || l.pool == nil || l.client == nil {
		return
	}
	l.pool.release(l.key, l.client, l.broken)
	l.client = nil
}

func (l *sipClientLease) Reused() bool {
	return l != nil && l.reused
}
