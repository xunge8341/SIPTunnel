package service

import (
	"bytes"
	"fmt"
	"sort"
	"sync"
)

type fileSession struct {
	total  uint32
	chunks map[uint32][]byte
}

type Reassembler struct {
	mu       sync.Mutex
	sessions map[uint64]*fileSession
}

func NewReassembler() *Reassembler {
	return &Reassembler{sessions: map[uint64]*fileSession{}}
}

func (r *Reassembler) AddChunk(messageID uint64, seq, total uint32, payload []byte) (complete bool, merged []byte, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.sessions[messageID]
	if !ok {
		s = &fileSession{total: total, chunks: map[uint32][]byte{}}
		r.sessions[messageID] = s
	}
	if s.total != total {
		return false, nil, fmt.Errorf("inconsistent total chunk for message %d", messageID)
	}
	if _, exists := s.chunks[seq]; exists {
		return false, nil, nil
	}
	s.chunks[seq] = append([]byte{}, payload...)
	if uint32(len(s.chunks)) != s.total {
		return false, nil, nil
	}

	seqs := make([]int, 0, len(s.chunks))
	for k := range s.chunks {
		seqs = append(seqs, int(k))
	}
	sort.Ints(seqs)
	var b bytes.Buffer
	for _, v := range seqs {
		b.Write(s.chunks[uint32(v)])
	}
	delete(r.sessions, messageID)
	return true, b.Bytes(), nil
}
