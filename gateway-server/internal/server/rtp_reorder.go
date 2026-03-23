package server

import "fmt"

const (
	defaultRTPReorderWindowPackets = 8
	defaultRTPLossTolerancePackets = 2
)

type rtpOrderedPacket struct {
	SequenceNumber uint16
	Payload        []byte
}

type rtpSequenceReorderBuffer struct {
	expected      uint16
	started       bool
	reorderWindow int
	lossTolerance int
	pending       map[uint16][]byte
}

func newRTPSequenceReorderBuffer(reorderWindow int, lossTolerance int) *rtpSequenceReorderBuffer {
	if reorderWindow <= 0 {
		reorderWindow = defaultRTPReorderWindowPackets
	}
	if lossTolerance < 0 {
		lossTolerance = defaultRTPLossTolerancePackets
	}
	capacity := reorderWindow + lossTolerance
	if capacity <= 0 {
		capacity = reorderWindow
	}
	return &rtpSequenceReorderBuffer{
		reorderWindow: reorderWindow,
		lossTolerance: lossTolerance,
		pending:       make(map[uint16][]byte, capacity),
	}
}

func (b *rtpSequenceReorderBuffer) PendingCount() int {
	if b == nil {
		return 0
	}
	return len(b.pending)
}

func (b *rtpSequenceReorderBuffer) HasPending() bool {
	return b != nil && len(b.pending) > 0
}

func (b *rtpSequenceReorderBuffer) ExpectedSequence() uint16 {
	if b == nil {
		return 0
	}
	return b.expected
}

func (b *rtpSequenceReorderBuffer) PendingGapPackets() int {
	if b == nil || len(b.pending) == 0 {
		return 0
	}
	minDistance := -1
	for seq := range b.pending {
		distance := int(uint16(seq - b.expected))
		if distance >= 0x8000 {
			continue
		}
		if minDistance < 0 || distance < minDistance {
			minDistance = distance
		}
	}
	if minDistance <= 0 {
		return 0
	}
	return minDistance + 1
}

func (b *rtpSequenceReorderBuffer) FastForwardToNextPending(maxSkip int) ([]rtpOrderedPacket, int, bool) {
	if b == nil || len(b.pending) == 0 {
		return nil, 0, false
	}
	minDistance := -1
	var minSeq uint16
	for seq := range b.pending {
		distance := int(uint16(seq - b.expected))
		if distance <= 0 || distance >= 0x8000 {
			continue
		}
		if maxSkip > 0 && distance > maxSkip {
			continue
		}
		if minDistance < 0 || distance < minDistance {
			minDistance = distance
			minSeq = seq
		}
	}
	if minDistance <= 0 {
		return nil, 0, false
	}
	b.expected = minSeq
	ready := b.flushReady(nil)
	if len(ready) == 0 {
		return nil, 0, false
	}
	return ready, minDistance + 1, true
}

func (b *rtpSequenceReorderBuffer) ExpandTolerance(reorderWindow int, lossTolerance int) bool {
	if b == nil {
		return false
	}
	changed := false
	if reorderWindow > b.reorderWindow {
		b.reorderWindow = reorderWindow
		changed = true
	}
	if lossTolerance > b.lossTolerance {
		b.lossTolerance = lossTolerance
		changed = true
	}
	return changed
}

func (b *rtpSequenceReorderBuffer) totalWindow() int {
	if b == nil {
		return defaultRTPReorderWindowPackets + defaultRTPLossTolerancePackets
	}
	total := b.reorderWindow + b.lossTolerance
	if total <= 0 {
		total = b.reorderWindow
	}
	if total <= 0 {
		total = defaultRTPReorderWindowPackets
	}
	return total
}

func (b *rtpSequenceReorderBuffer) Push(seq uint16, payload []byte) ([]rtpOrderedPacket, string, error) {
	if b == nil {
		return []rtpOrderedPacket{{SequenceNumber: seq, Payload: payload}}, "disabled", nil
	}
	if !b.started {
		b.started = true
		b.expected = seq
	}
	if seq == b.expected {
		ready := b.flushReady([]rtpOrderedPacket{{SequenceNumber: seq, Payload: payload}})
		return ready, "in_order", nil
	}
	distance := int(uint16(seq - b.expected))
	if distance < 0x8000 {
		if distance > b.totalWindow() {
			return nil, "gap_overflow", fmt.Errorf("rtp sequence discontinuity beyond tolerance expected=%d got=%d reorder_window=%d loss_tolerance=%d", b.expected, seq, b.reorderWindow, b.lossTolerance)
		}
		if _, exists := b.pending[seq]; exists {
			return nil, "duplicate", nil
		}
		copyPayload := append([]byte(nil), payload...)
		b.pending[seq] = copyPayload
		if distance > b.reorderWindow {
			return nil, "gap_tolerated", nil
		}
		return nil, "buffered", nil
	}
	return nil, "late", nil
}

func (b *rtpSequenceReorderBuffer) flushReady(ready []rtpOrderedPacket) []rtpOrderedPacket {
	if len(ready) > 0 {
		b.expected = ready[len(ready)-1].SequenceNumber + 1
	}
	for {
		seq := b.expected
		payload, ok := b.pending[seq]
		if !ok {
			break
		}
		delete(b.pending, seq)
		ready = append(ready, rtpOrderedPacket{SequenceNumber: seq, Payload: payload})
		b.expected++
	}
	return ready
}
