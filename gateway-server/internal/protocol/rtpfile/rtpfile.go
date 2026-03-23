package rtpfile

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

const (
	Magic           uint32 = 0x52544631 // RTF1
	ProtocolVersion uint8  = 1

	transferIDSize = 16
	requestIDSize  = 16
	traceIDSize    = 16
	digestSize     = sha256.Size

	fixedHeaderSize        = 157
	tlvHeaderSize          = 4
	maxChunkTotal          = 4096
	maxChunkLength         = 8 * 1024 * 1024
	maxFileSize            = 64 * 1024 * 1024
	maxHeaderLength        = 4 * 1024
	maxTLVCount            = 32
	maxTLVValueLen         = 1024
	allowedFlagMask uint16 = FlagStart | FlagEnd | FlagRetransmit | FlagAckRequired
	maxClockSkewMs  uint64 = 10 * 60 * 1000
)

const (
	FlagStart uint16 = 1 << iota
	FlagEnd
	FlagRetransmit
	FlagAckRequired
)

const (
	TLVTypeFileName uint16 = 1
	TLVTypeMimeType uint16 = 2
	TLVTypeMetaJSON uint16 = 3
)

var knownTLVTypes = map[uint16]struct{}{
	TLVTypeFileName: {},
	TLVTypeMimeType: {},
	TLVTypeMetaJSON: {},
}

type TLV struct {
	Type  uint16
	Value []byte
}

type Header struct {
	Magic           uint32
	ProtocolVersion uint8
	HeaderLength    uint16
	Flags           uint16
	TransferID      [transferIDSize]byte
	RequestID       [requestIDSize]byte
	TraceID         [traceIDSize]byte
	ChunkNo         uint32
	ChunkTotal      uint32
	ChunkOffset     uint64
	ChunkLength     uint32
	FileSize        uint64
	ChunkDigest     [digestSize]byte
	FileDigest      [digestSize]byte
	SendTimestamp   uint64
	Extensions      []TLV
}

func (h Header) MarshalBinary() ([]byte, error) {
	tlvBytes := encodeTLVs(h.Extensions)
	headerLen := fixedHeaderSize + len(tlvBytes)
	if headerLen > 0xFFFF {
		return nil, fmt.Errorf("header too large: %d", headerLen)
	}
	if h.HeaderLength != 0 && int(h.HeaderLength) != headerLen {
		return nil, fmt.Errorf("header_length mismatch: got=%d want=%d", h.HeaderLength, headerLen)
	}

	b := make([]byte, headerLen)
	binary.BigEndian.PutUint32(b[0:4], Magic)
	b[4] = ProtocolVersion
	binary.BigEndian.PutUint16(b[5:7], uint16(headerLen))
	binary.BigEndian.PutUint16(b[7:9], h.Flags)
	copy(b[9:25], h.TransferID[:])
	copy(b[25:41], h.RequestID[:])
	copy(b[41:57], h.TraceID[:])
	binary.BigEndian.PutUint32(b[57:61], h.ChunkNo)
	binary.BigEndian.PutUint32(b[61:65], h.ChunkTotal)
	binary.BigEndian.PutUint64(b[65:73], h.ChunkOffset)
	binary.BigEndian.PutUint32(b[73:77], h.ChunkLength)
	binary.BigEndian.PutUint64(b[77:85], h.FileSize)
	copy(b[85:117], h.ChunkDigest[:])
	copy(b[117:149], h.FileDigest[:])
	binary.BigEndian.PutUint64(b[149:157], h.SendTimestamp)
	copy(b[fixedHeaderSize:], tlvBytes)
	return b, nil
}

func (h Header) ValidateEnvelope(now time.Time) error {
	if h.Magic != Magic {
		return fmt.Errorf("invalid magic: %x", h.Magic)
	}
	if h.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("unsupported protocol_version: %d", h.ProtocolVersion)
	}
	if h.HeaderLength < fixedHeaderSize || int(h.HeaderLength) > maxHeaderLength {
		return fmt.Errorf("invalid header_length: %d", h.HeaderLength)
	}
	if h.Flags&^allowedFlagMask != 0 {
		return fmt.Errorf("unsupported flags: %d", h.Flags)
	}
	if h.ChunkTotal == 0 || h.ChunkTotal > maxChunkTotal {
		return fmt.Errorf("invalid chunk_total: %d", h.ChunkTotal)
	}
	if h.ChunkNo == 0 || h.ChunkNo > h.ChunkTotal {
		return fmt.Errorf("invalid chunk_no=%d chunk_total=%d", h.ChunkNo, h.ChunkTotal)
	}
	if h.ChunkLength == 0 || h.ChunkLength > maxChunkLength {
		return fmt.Errorf("invalid chunk_length: %d", h.ChunkLength)
	}
	if h.FileSize == 0 || h.FileSize > maxFileSize {
		return fmt.Errorf("invalid file_size: %d", h.FileSize)
	}
	if h.ChunkOffset+uint64(h.ChunkLength) > h.FileSize {
		return fmt.Errorf("chunk out of file range")
	}
	if bytes.Equal(h.TransferID[:], make([]byte, transferIDSize)) {
		return errors.New("transfer_id is required")
	}
	if bytes.Equal(h.RequestID[:], make([]byte, requestIDSize)) {
		return errors.New("request_id is required")
	}
	if bytes.Equal(h.TraceID[:], make([]byte, traceIDSize)) {
		return errors.New("trace_id is required")
	}
	if h.SendTimestamp == 0 {
		return errors.New("send_timestamp is required")
	}
	nowMs := uint64(now.UnixMilli())
	if h.SendTimestamp > nowMs+maxClockSkewMs || nowMs > h.SendTimestamp+maxClockSkewMs {
		return errors.New("send_timestamp outside allowed skew")
	}
	return nil
}

func (h *Header) UnmarshalBinary(data []byte) error {
	if len(data) < fixedHeaderSize {
		return fmt.Errorf("header too short: %d", len(data))
	}
	if binary.BigEndian.Uint32(data[0:4]) != Magic {
		return fmt.Errorf("invalid magic: %x", binary.BigEndian.Uint32(data[0:4]))
	}
	if data[4] != ProtocolVersion {
		return fmt.Errorf("unsupported protocol_version: %d", data[4])
	}
	headerLen := int(binary.BigEndian.Uint16(data[5:7]))
	if headerLen < fixedHeaderSize {
		return fmt.Errorf("invalid header_length: %d", headerLen)
	}
	if headerLen > len(data) {
		return fmt.Errorf("header_length overflow: %d > %d", headerLen, len(data))
	}

	h.Magic = Magic
	h.ProtocolVersion = data[4]
	h.HeaderLength = uint16(headerLen)
	h.Flags = binary.BigEndian.Uint16(data[7:9])
	copy(h.TransferID[:], data[9:25])
	copy(h.RequestID[:], data[25:41])
	copy(h.TraceID[:], data[41:57])
	h.ChunkNo = binary.BigEndian.Uint32(data[57:61])
	h.ChunkTotal = binary.BigEndian.Uint32(data[61:65])
	h.ChunkOffset = binary.BigEndian.Uint64(data[65:73])
	h.ChunkLength = binary.BigEndian.Uint32(data[73:77])
	h.FileSize = binary.BigEndian.Uint64(data[77:85])
	copy(h.ChunkDigest[:], data[85:117])
	copy(h.FileDigest[:], data[117:149])
	h.SendTimestamp = binary.BigEndian.Uint64(data[149:157])

	exts, err := decodeTLVs(data[fixedHeaderSize:headerLen])
	if err != nil {
		return err
	}
	h.Extensions = exts
	// UnmarshalBinary 只负责结构化解析与基础长度/版本校验。
	// 运行态的时钟偏移、必填 ID、chunk/file 摘要等安全校验统一由 ValidateEnvelope / ValidatePacket 负责，
	// 避免历史报文、离线样本或仅做 TLV 解析时被当前时间窗口误杀。
	return nil
}

func ValidatePacket(packet ChunkPacket, now time.Time) error {
	if err := packet.Header.ValidateEnvelope(now); err != nil {
		return err
	}
	if uint32(len(packet.Payload)) != packet.Header.ChunkLength {
		return fmt.Errorf("chunk_length mismatch: payload=%d header=%d", len(packet.Payload), packet.Header.ChunkLength)
	}
	if packet.Header.ChunkOffset+uint64(packet.Header.ChunkLength) > packet.Header.FileSize {
		return fmt.Errorf("chunk out of file range")
	}
	if sha256.Sum256(packet.Payload) != packet.Header.ChunkDigest {
		return fmt.Errorf("chunk digest mismatch for chunk=%d", packet.Header.ChunkNo)
	}
	return nil
}

type ChunkPacket struct {
	Header  Header
	Payload []byte
}

type ChunkOptions struct {
	TransferID    [transferIDSize]byte
	RequestID     [requestIDSize]byte
	TraceID       [traceIDSize]byte
	ChunkSize     int
	SendTimestamp uint64
	Extensions    []TLV
}

func SplitFileToChunks(fileData []byte, opts ChunkOptions) ([]ChunkPacket, error) {
	if opts.ChunkSize <= 0 {
		return nil, errors.New("chunk_size must be positive")
	}
	if len(fileData) == 0 {
		return nil, errors.New("file data is empty")
	}
	total := (len(fileData) + opts.ChunkSize - 1) / opts.ChunkSize
	if total == 0 {
		return nil, errors.New("invalid chunk_total")
	}
	if opts.SendTimestamp == 0 {
		opts.SendTimestamp = uint64(time.Now().UnixMilli())
	}

	fileDigest := sha256.Sum256(fileData)
	packets := make([]ChunkPacket, 0, total)
	for i := 0; i < total; i++ {
		start := i * opts.ChunkSize
		end := start + opts.ChunkSize
		if end > len(fileData) {
			end = len(fileData)
		}
		chunk := append([]byte(nil), fileData[start:end]...)
		chunkDigest := sha256.Sum256(chunk)

		flags := uint16(0)
		if i == 0 {
			flags |= FlagStart
		}
		if i == total-1 {
			flags |= FlagEnd
		}

		h := Header{
			Magic:           Magic,
			ProtocolVersion: ProtocolVersion,
			Flags:           flags,
			TransferID:      opts.TransferID,
			RequestID:       opts.RequestID,
			TraceID:         opts.TraceID,
			ChunkNo:         uint32(i + 1),
			ChunkTotal:      uint32(total),
			ChunkOffset:     uint64(start),
			ChunkLength:     uint32(len(chunk)),
			FileSize:        uint64(len(fileData)),
			ChunkDigest:     chunkDigest,
			FileDigest:      fileDigest,
			SendTimestamp:   opts.SendTimestamp,
			Extensions:      opts.Extensions,
		}
		packets = append(packets, ChunkPacket{Header: h, Payload: chunk})
	}
	return packets, nil
}

type Reassembler struct {
	total      uint32
	fileSize   uint64
	fileDigest [digestSize]byte
	chunks     map[uint32][]byte
}

func NewReassembler() *Reassembler {
	return &Reassembler{chunks: make(map[uint32][]byte)}
}

func (r *Reassembler) AddChunk(packet ChunkPacket) (bool, error) {
	h := packet.Header
	if h.ChunkTotal == 0 {
		return false, errors.New("chunk_total must be positive")
	}
	if h.ChunkNo == 0 || h.ChunkNo > h.ChunkTotal {
		return false, fmt.Errorf("invalid chunk_no=%d chunk_total=%d", h.ChunkNo, h.ChunkTotal)
	}
	if uint32(len(packet.Payload)) != h.ChunkLength {
		return false, fmt.Errorf("chunk_length mismatch: payload=%d header=%d", len(packet.Payload), h.ChunkLength)
	}
	digest := sha256.Sum256(packet.Payload)
	if digest != h.ChunkDigest {
		return false, fmt.Errorf("chunk digest mismatch for chunk=%d", h.ChunkNo)
	}
	if r.total == 0 {
		r.total = h.ChunkTotal
		r.fileSize = h.FileSize
		r.fileDigest = h.FileDigest
	} else {
		if r.total != h.ChunkTotal || r.fileSize != h.FileSize || r.fileDigest != h.FileDigest {
			return false, errors.New("inconsistent file metadata")
		}
	}
	if existing, ok := r.chunks[h.ChunkNo]; ok {
		if !bytes.Equal(existing, packet.Payload) {
			return false, fmt.Errorf("duplicate chunk with different payload: %d", h.ChunkNo)
		}
		return len(r.chunks) == int(r.total), nil
	}
	r.chunks[h.ChunkNo] = append([]byte(nil), packet.Payload...)
	return len(r.chunks) == int(r.total), nil
}

func (r *Reassembler) Assemble() ([]byte, error) {
	if r.total == 0 || len(r.chunks) != int(r.total) {
		return nil, errors.New("chunks not complete")
	}
	out := make([]byte, 0, r.fileSize)
	for i := uint32(1); i <= r.total; i++ {
		chunk, ok := r.chunks[i]
		if !ok {
			return nil, fmt.Errorf("missing chunk=%d", i)
		}
		out = append(out, chunk...)
	}
	if uint64(len(out)) != r.fileSize {
		return nil, fmt.Errorf("file_size mismatch: got=%d want=%d", len(out), r.fileSize)
	}
	if sha256.Sum256(out) != r.fileDigest {
		return nil, errors.New("file digest mismatch")
	}
	return out, nil
}

func encodeTLVs(tlvs []TLV) []byte {
	out := make([]byte, 0)
	for _, tlv := range tlvs {
		segment := make([]byte, tlvHeaderSize+len(tlv.Value))
		binary.BigEndian.PutUint16(segment[0:2], tlv.Type)
		binary.BigEndian.PutUint16(segment[2:4], uint16(len(tlv.Value)))
		copy(segment[4:], tlv.Value)
		out = append(out, segment...)
	}
	return out
}

func decodeTLVs(data []byte) ([]TLV, error) {
	tlvs := make([]TLV, 0)
	for i := 0; i < len(data); {
		if len(data[i:]) < tlvHeaderSize {
			return nil, errors.New("invalid tlv header")
		}
		t := binary.BigEndian.Uint16(data[i : i+2])
		l := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		i += tlvHeaderSize
		if len(data[i:]) < l {
			return nil, errors.New("invalid tlv length")
		}
		if _, ok := knownTLVTypes[t]; ok {
			tlv := TLV{Type: t, Value: append([]byte(nil), data[i:i+l]...)}
			tlvs = append(tlvs, tlv)
		}
		i += l
	}
	return tlvs, nil
}
