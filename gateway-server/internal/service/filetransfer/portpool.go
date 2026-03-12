package filetransfer

import (
	"errors"
	"fmt"
	"sync"
)

// ErrRTPPortExhausted 表示 RTP 端口池已耗尽。
// 运维侧通常需结合 rtp_port_pool_used/total 与并发任务数做容量排查。
var ErrRTPPortExhausted = errors.New("rtp port pool exhausted")

type PortPoolStats struct {
	Total          int `json:"total"`
	Used           int `json:"used"`
	Available      int `json:"available"`
	AllocFailTotal int `json:"alloc_fail_total"`
}

type RTPPortPool interface {
	Allocate(transferID [16]byte) (int, error)
	Release(transferID [16]byte) bool
	PortOf(transferID [16]byte) (int, bool)
	Stats() PortPoolStats
}

type MemoryRTPPortPool struct {
	mu             sync.Mutex
	ports          []int
	usedByTransfer map[[16]byte]int
	usedPorts      map[int][16]byte
	next           int
	allocFailTotal int
}

func NewMemoryRTPPortPool(portStart, portEnd int) (*MemoryRTPPortPool, error) {
	if portStart < 1 || portEnd > 65535 || portStart > portEnd {
		return nil, fmt.Errorf("invalid rtp port range [%d,%d]", portStart, portEnd)
	}
	ports := make([]int, 0, portEnd-portStart+1)
	for p := portStart; p <= portEnd; p++ {
		ports = append(ports, p)
	}
	return &MemoryRTPPortPool{
		ports:          ports,
		usedByTransfer: make(map[[16]byte]int),
		usedPorts:      make(map[int][16]byte),
	}, nil
}

func (p *MemoryRTPPortPool) Allocate(transferID [16]byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if port, ok := p.usedByTransfer[transferID]; ok {
		return port, nil
	}
	if len(p.usedByTransfer) >= len(p.ports) {
		p.allocFailTotal++
		return 0, ErrRTPPortExhausted
	}

	for i := 0; i < len(p.ports); i++ {
		idx := (p.next + i) % len(p.ports)
		port := p.ports[idx]
		if _, occupied := p.usedPorts[port]; occupied {
			continue
		}
		p.usedByTransfer[transferID] = port
		p.usedPorts[port] = transferID
		p.next = (idx + 1) % len(p.ports)
		return port, nil
	}

	p.allocFailTotal++
	return 0, ErrRTPPortExhausted
}

func (p *MemoryRTPPortPool) Release(transferID [16]byte) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	port, ok := p.usedByTransfer[transferID]
	if !ok {
		return false
	}
	delete(p.usedByTransfer, transferID)
	delete(p.usedPorts, port)
	return true
}

func (p *MemoryRTPPortPool) PortOf(transferID [16]byte) (int, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	port, ok := p.usedByTransfer[transferID]
	return port, ok
}

func (p *MemoryRTPPortPool) Stats() PortPoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	total := len(p.ports)
	used := len(p.usedByTransfer)
	available := total - used
	if available < 0 {
		available = 0
	}
	return PortPoolStats{Total: total, Used: used, Available: available, AllocFailTotal: p.allocFailTotal}
}
