package rtp

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	mainHeaderSize = 32
	magicNumber    = uint32(0x53545031) // STP1
)

// MainHeader 二进制定长主头，便于跨边界高性能处理。
type MainHeader struct {
	Magic       uint32
	Version     uint8
	Flags       uint8
	Reserved    uint16
	MessageID   uint64
	ChunkSeq    uint32
	ChunkTotal  uint32
	PayloadSize uint32
	TLVLength   uint32
}

type TLV struct {
	Type  uint16
	Value []byte
}

func (h MainHeader) Encode() []byte {
	b := make([]byte, mainHeaderSize)
	binary.BigEndian.PutUint32(b[0:4], h.Magic)
	b[4] = h.Version
	b[5] = h.Flags
	binary.BigEndian.PutUint16(b[6:8], h.Reserved)
	binary.BigEndian.PutUint64(b[8:16], h.MessageID)
	binary.BigEndian.PutUint32(b[16:20], h.ChunkSeq)
	binary.BigEndian.PutUint32(b[20:24], h.ChunkTotal)
	binary.BigEndian.PutUint32(b[24:28], h.PayloadSize)
	binary.BigEndian.PutUint32(b[28:32], h.TLVLength)
	return b
}

func DecodeMainHeader(b []byte) (MainHeader, error) {
	if len(b) < mainHeaderSize {
		return MainHeader{}, errors.New("rtp header too short")
	}
	h := MainHeader{
		Magic:       binary.BigEndian.Uint32(b[0:4]),
		Version:     b[4],
		Flags:       b[5],
		Reserved:    binary.BigEndian.Uint16(b[6:8]),
		MessageID:   binary.BigEndian.Uint64(b[8:16]),
		ChunkSeq:    binary.BigEndian.Uint32(b[16:20]),
		ChunkTotal:  binary.BigEndian.Uint32(b[20:24]),
		PayloadSize: binary.BigEndian.Uint32(b[24:28]),
		TLVLength:   binary.BigEndian.Uint32(b[28:32]),
	}
	if h.Magic != magicNumber {
		return MainHeader{}, fmt.Errorf("invalid magic: %x", h.Magic)
	}
	return h, nil
}

func EncodeTLVs(tlvs []TLV) []byte {
	var out []byte
	for _, t := range tlvs {
		segment := make([]byte, 4+len(t.Value))
		binary.BigEndian.PutUint16(segment[0:2], t.Type)
		binary.BigEndian.PutUint16(segment[2:4], uint16(len(t.Value)))
		copy(segment[4:], t.Value)
		out = append(out, segment...)
	}
	return out
}

func DecodeTLVs(b []byte) ([]TLV, error) {
	var tlvs []TLV
	i := 0
	for i < len(b) {
		if len(b[i:]) < 4 {
			return nil, errors.New("invalid tlv header")
		}
		t := binary.BigEndian.Uint16(b[i : i+2])
		l := int(binary.BigEndian.Uint16(b[i+2 : i+4]))
		i += 4
		if len(b[i:]) < l {
			return nil, errors.New("invalid tlv length")
		}
		tlv := TLV{Type: t, Value: append([]byte{}, b[i:i+l]...)}
		tlvs = append(tlvs, tlv)
		i += l
	}
	return tlvs, nil
}

func NewHeader(messageID uint64, chunkSeq, chunkTotal uint32, payloadSize, tlvLength uint32) MainHeader {
	return MainHeader{
		Magic:       magicNumber,
		Version:     1,
		MessageID:   messageID,
		ChunkSeq:    chunkSeq,
		ChunkTotal:  chunkTotal,
		PayloadSize: payloadSize,
		TLVLength:   tlvLength,
	}
}
