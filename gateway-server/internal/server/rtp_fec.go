package server

import (
	"encoding/binary"
	"fmt"
)

const rtpFECPayloadType = 127

func deriveRTPFECSSRC(dataSSRC uint32) uint32 {
	fecSSRC := dataSSRC ^ 0x5F3759DF
	if fecSSRC == 0 || fecSSRC == dataSSRC {
		fecSSRC = dataSSRC + 1
		if fecSSRC == 0 || fecSSRC == dataSSRC {
			fecSSRC = 1
		}
	}
	return fecSSRC
}

// FEC 负载格式：
//
//	base_sequence(2) + group_packets(1) + chunk_bytes(2) + packet_lengths(group_packets*2) + parity(chunk_bytes)
//
// 这样接收侧不仅能恢复固定长度 RTP 包，也能恢复尾包/短包这类变长 payload，
// 避免过去“只有满尺寸 RTP 数据包才受保护，最后一跳短包裸奔”的缺口。
func buildRTPFECPayload(baseSequence uint16, packetLengths []int, parity []byte) ([]byte, error) {
	groupPackets := len(packetLengths)
	if groupPackets < 2 || groupPackets > 255 {
		return nil, fmt.Errorf("rtp fec group_packets %d invalid", groupPackets)
	}
	chunkBytes := len(parity)
	if chunkBytes <= 0 || chunkBytes > 0xFFFF {
		return nil, fmt.Errorf("rtp fec chunk_bytes %d invalid", chunkBytes)
	}
	payload := make([]byte, 5+groupPackets*2+len(parity))
	binary.BigEndian.PutUint16(payload[0:2], baseSequence)
	payload[2] = byte(groupPackets)
	binary.BigEndian.PutUint16(payload[3:5], uint16(chunkBytes))
	offset := 5
	for _, packetLen := range packetLengths {
		if packetLen <= 0 || packetLen > chunkBytes {
			return nil, fmt.Errorf("rtp fec packet_len %d invalid chunk_bytes=%d", packetLen, chunkBytes)
		}
		binary.BigEndian.PutUint16(payload[offset:offset+2], uint16(packetLen))
		offset += 2
	}
	copy(payload[offset:], parity)
	return payload, nil
}

func parseRTPFECPayload(payload []byte) (baseSequence uint16, packetLengths []int, chunkBytes int, parity []byte, err error) {
	if len(payload) < 9 {
		return 0, nil, 0, nil, fmt.Errorf("rtp fec payload too short")
	}
	baseSequence = binary.BigEndian.Uint16(payload[0:2])
	groupPackets := int(payload[2])
	chunkBytes = int(binary.BigEndian.Uint16(payload[3:5]))
	if groupPackets < 2 {
		return 0, nil, 0, nil, fmt.Errorf("rtp fec group_packets %d invalid", groupPackets)
	}
	if chunkBytes <= 0 {
		return 0, nil, 0, nil, fmt.Errorf("rtp fec chunk_bytes %d invalid", chunkBytes)
	}
	headerBytes := 5 + groupPackets*2
	if len(payload) < headerBytes+chunkBytes {
		return 0, nil, 0, nil, fmt.Errorf("rtp fec payload size %d mismatch header=%d chunk_bytes=%d", len(payload), headerBytes, chunkBytes)
	}
	packetLengths = make([]int, 0, groupPackets)
	offset := 5
	for i := 0; i < groupPackets; i++ {
		packetLen := int(binary.BigEndian.Uint16(payload[offset : offset+2]))
		if packetLen <= 0 || packetLen > chunkBytes {
			return 0, nil, 0, nil, fmt.Errorf("rtp fec packet_len %d invalid chunk_bytes=%d", packetLen, chunkBytes)
		}
		packetLengths = append(packetLengths, packetLen)
		offset += 2
	}
	parity = payload[offset : offset+chunkBytes]
	return baseSequence, packetLengths, chunkBytes, parity, nil
}

type rtpFECRecoveredPacket struct {
	SequenceNumber uint16
	Payload        []byte
	BaseSequence   uint16
	GroupPackets   int
}

type rtpFECParityGroup struct {
	baseSequence  uint16
	groupPackets  int
	chunkBytes    int
	packetLengths []int
	parity        []byte
	data          map[uint16][]byte
}

func (g *rtpFECParityGroup) contains(seq uint16) bool {
	if g == nil {
		return false
	}
	distance := int(uint16(seq - g.baseSequence))
	if distance >= 0x8000 {
		return false
	}
	return distance >= 0 && distance < g.groupPackets
}

func (g *rtpFECParityGroup) expectedLength(seq uint16) int {
	if g == nil {
		return 0
	}
	idx := int(uint16(seq - g.baseSequence))
	if idx < 0 || idx >= len(g.packetLengths) {
		return 0
	}
	return g.packetLengths[idx]
}

type rtpFECSingleParityTracker struct {
	groups    map[uint64]*rtpFECParityGroup
	recent    map[uint16][]byte
	maxRecent int
}

func newRTPFECSingleParityTracker(maxRecent int) *rtpFECSingleParityTracker {
	if maxRecent < 32 {
		maxRecent = 32
	}
	return &rtpFECSingleParityTracker{
		groups:    make(map[uint64]*rtpFECParityGroup),
		recent:    make(map[uint16][]byte, maxRecent),
		maxRecent: maxRecent,
	}
}

func (t *rtpFECSingleParityTracker) ObserveData(seq uint16, payload []byte, expected uint16) []rtpFECRecoveredPacket {
	if t == nil || len(payload) == 0 {
		return nil
	}
	copyPayload := append([]byte(nil), payload...)
	t.recent[seq] = copyPayload
	var recovered []rtpFECRecoveredPacket
	for key, group := range t.groups {
		if !group.contains(seq) {
			continue
		}
		if want := group.expectedLength(seq); want > 0 && len(copyPayload) <= group.chunkBytes {
			group.data[seq] = copyPayload
		}
		if pkt, ok := t.tryRecover(group); ok {
			recovered = append(recovered, pkt)
			delete(t.groups, key)
			t.recent[pkt.SequenceNumber] = append([]byte(nil), pkt.Payload...)
		}
	}
	t.trim(expected)
	return recovered
}

func (t *rtpFECSingleParityTracker) ObserveFEC(payload []byte, expected uint16) ([]rtpFECRecoveredPacket, error) {
	if t == nil {
		return nil, nil
	}
	baseSequence, packetLengths, chunkBytes, parity, err := parseRTPFECPayload(payload)
	if err != nil {
		return nil, err
	}
	group := &rtpFECParityGroup{
		baseSequence:  baseSequence,
		groupPackets:  len(packetLengths),
		chunkBytes:    chunkBytes,
		packetLengths: append([]int(nil), packetLengths...),
		parity:        append([]byte(nil), parity...),
		data:          make(map[uint16][]byte, len(packetLengths)),
	}
	for i, packetLen := range packetLengths {
		seq := baseSequence + uint16(i)
		if data, ok := t.recent[seq]; ok && len(data) == packetLen {
			group.data[seq] = data
		}
	}
	if pkt, ok := t.tryRecover(group); ok {
		t.recent[pkt.SequenceNumber] = append([]byte(nil), pkt.Payload...)
		t.trim(expected)
		return []rtpFECRecoveredPacket{pkt}, nil
	}
	t.groups[fecGroupKey(baseSequence, packetLengths)] = group
	t.trim(expected)
	return nil, nil
}

func (t *rtpFECSingleParityTracker) tryRecover(group *rtpFECParityGroup) (rtpFECRecoveredPacket, bool) {
	if group == nil || len(group.parity) != group.chunkBytes {
		return rtpFECRecoveredPacket{}, false
	}
	missingCount := 0
	missingSeq := group.baseSequence
	missingLen := 0
	for i := 0; i < group.groupPackets; i++ {
		seq := group.baseSequence + uint16(i)
		packetLen := group.packetLengths[i]
		payload, ok := group.data[seq]
		if !ok {
			missingCount++
			missingSeq = seq
			missingLen = packetLen
			if missingCount > 1 {
				return rtpFECRecoveredPacket{}, false
			}
			continue
		}
		if len(payload) != packetLen || len(payload) > group.chunkBytes {
			return rtpFECRecoveredPacket{}, false
		}
	}
	if missingCount != 1 || missingLen <= 0 || missingLen > group.chunkBytes {
		return rtpFECRecoveredPacket{}, false
	}
	recovered := append([]byte(nil), group.parity...)
	for _, payload := range group.data {
		for i := 0; i < len(payload); i++ {
			recovered[i] ^= payload[i]
		}
	}
	recovered = recovered[:missingLen]
	return rtpFECRecoveredPacket{
		SequenceNumber: missingSeq,
		Payload:        recovered,
		BaseSequence:   group.baseSequence,
		GroupPackets:   group.groupPackets,
	}, true
}

func (t *rtpFECSingleParityTracker) trim(expected uint16) {
	if t == nil {
		return
	}
	for seq := range t.recent {
		if sequenceBehind(seq, expected) > t.maxRecent {
			delete(t.recent, seq)
		}
	}
	for key, group := range t.groups {
		if group == nil {
			delete(t.groups, key)
			continue
		}
		last := group.baseSequence + uint16(group.groupPackets-1)
		if sequenceBehind(last, expected) > t.maxRecent {
			delete(t.groups, key)
		}
	}
}

func fecGroupKey(baseSequence uint16, packetLengths []int) uint64 {
	key := uint64(baseSequence) << 32
	key |= uint64(len(packetLengths)&0xFF) << 24
	for _, packetLen := range packetLengths {
		key ^= uint64(packetLen & 0xFFFF)
		key *= 1099511628211
	}
	return key
}

func sequenceBehind(seq uint16, expected uint16) int {
	distance := int(uint16(expected - seq))
	if distance >= 0x8000 {
		return 0
	}
	return distance
}
