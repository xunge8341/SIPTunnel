package filetransfer

import (
	"errors"
	"sync"
	"testing"
)

func TestMemoryRTPPortPoolAllocateReleaseAndStats(t *testing.T) {
	pool, err := NewMemoryRTPPortPool(30000, 30002)
	if err != nil {
		t.Fatalf("NewMemoryRTPPortPool error: %v", err)
	}

	id1 := [16]byte{1}
	id2 := [16]byte{2}
	id3 := [16]byte{3}
	id4 := [16]byte{4}

	p1, err := pool.Allocate(id1)
	if err != nil {
		t.Fatalf("Allocate id1 error: %v", err)
	}
	p1Repeat, err := pool.Allocate(id1)
	if err != nil {
		t.Fatalf("Allocate id1 repeat error: %v", err)
	}
	if p1 != p1Repeat {
		t.Fatalf("idempotent allocate got=%d want=%d", p1Repeat, p1)
	}
	p2, err := pool.Allocate(id2)
	if err != nil {
		t.Fatalf("Allocate id2 error: %v", err)
	}
	p3, err := pool.Allocate(id3)
	if err != nil {
		t.Fatalf("Allocate id3 error: %v", err)
	}
	if p1 == p2 || p1 == p3 || p2 == p3 {
		t.Fatalf("ports should be unique got=%d,%d,%d", p1, p2, p3)
	}

	if _, err := pool.Allocate(id4); !errors.Is(err, ErrRTPPortExhausted) {
		t.Fatalf("expected exhausted error, got %v", err)
	}

	stats := pool.Stats()
	if stats.Total != 3 || stats.Used != 3 || stats.Available != 0 || stats.AllocFailTotal != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	if !pool.Release(id2) {
		t.Fatalf("release id2 should be true")
	}
	if pool.Release(id2) {
		t.Fatalf("release id2 second time should be false")
	}
	stats = pool.Stats()
	if stats.Used != 2 || stats.Available != 1 {
		t.Fatalf("unexpected stats after release: %+v", stats)
	}
	if _, ok := pool.PortOf(id2); ok {
		t.Fatalf("port mapping for released transfer should not exist")
	}
}

func TestMemoryRTPPortPoolConcurrentAllocationConflictProtection(t *testing.T) {
	pool, err := NewMemoryRTPPortPool(31000, 31099)
	if err != nil {
		t.Fatalf("NewMemoryRTPPortPool error: %v", err)
	}

	const workers = 80
	ports := make(chan int, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			id := [16]byte{byte(i + 1)}
			port, err := pool.Allocate(id)
			if err != nil {
				t.Errorf("Allocate %d failed: %v", i, err)
				return
			}
			ports <- port
		}(i)
	}
	wg.Wait()
	close(ports)

	seen := map[int]struct{}{}
	for p := range ports {
		if _, ok := seen[p]; ok {
			t.Fatalf("duplicate allocated port detected: %d", p)
		}
		seen[p] = struct{}{}
	}
	if len(seen) != workers {
		t.Fatalf("allocated unique ports=%d want=%d", len(seen), workers)
	}

	stats := pool.Stats()
	if stats.Used != workers || stats.Available != stats.Total-workers {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestNewMemoryRTPPortPoolInvalidRange(t *testing.T) {
	if _, err := NewMemoryRTPPortPool(0, 1); err == nil {
		t.Fatalf("expected invalid range error")
	}
	if _, err := NewMemoryRTPPortPool(20000, 19999); err == nil {
		t.Fatalf("expected start>end error")
	}
}
