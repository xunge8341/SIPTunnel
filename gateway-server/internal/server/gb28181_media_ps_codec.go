package server

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

func decodeRTPPacket(packet []byte) (rtpPacketHeader, []byte, error) {
	if len(packet) < rtpHeaderSize {
		return rtpPacketHeader{}, nil, fmt.Errorf("rtp packet too short")
	}
	if packet[0]>>6 != 2 {
		return rtpPacketHeader{}, nil, fmt.Errorf("unsupported rtp version")
	}
	cc := int(packet[0] & 0x0F)
	headerLen := rtpHeaderSize + cc*4
	if packet[0]&0x10 != 0 {
		return rtpPacketHeader{}, nil, fmt.Errorf("rtp extension not supported")
	}
	if len(packet) < headerLen {
		return rtpPacketHeader{}, nil, fmt.Errorf("invalid rtp header length")
	}
	payload := packet[headerLen:]
	if packet[0]&0x20 != 0 {
		if len(payload) == 0 {
			return rtpPacketHeader{}, nil, fmt.Errorf("rtp padding invalid")
		}
		padLen := int(payload[len(payload)-1])
		if padLen <= 0 || padLen > len(payload) {
			return rtpPacketHeader{}, nil, fmt.Errorf("rtp padding invalid")
		}
		payload = payload[:len(payload)-padLen]
	}
	return rtpPacketHeader{
		PayloadType:    packet[1] & 0x7F,
		Marker:         packet[1]&0x80 != 0,
		SequenceNumber: binary.BigEndian.Uint16(packet[2:4]),
		Timestamp:      binary.BigEndian.Uint32(packet[4:8]),
		SSRC:           binary.BigEndian.Uint32(packet[8:12]),
	}, payload, nil
}

func buildProgramStream(body []byte, pts90k uint64) []byte {
	var out bytes.Buffer
	if len(body) == 0 {
		body = []byte{}
	}
	out.Write(buildPackHeader(pts90k))
	out.Write(buildSystemHeader())
	out.Write(buildProgramStreamMap())
	for offset := 0; offset < len(body) || (len(body) == 0 && offset == 0); offset += maxPESPayloadBytes {
		end := offset + maxPESPayloadBytes
		if end > len(body) {
			end = len(body)
		}
		chunk := body[offset:end]
		out.Write(buildPESPrivateStream1(chunk, pts90k))
		if len(body) == 0 {
			break
		}
		pts90k += 3600 // 40ms pacing in 90kHz clock
	}
	return out.Bytes()
}

func buildPackHeader(scr uint64) []byte {
	// Minimal MPEG-2 PS pack header without stuffing.
	b := make([]byte, 14)
	b[0], b[1], b[2], b[3] = 0x00, 0x00, 0x01, 0xBA
	b[4] = 0x44 | byte((scr>>27)&0x38) | byte((scr>>28)&0x03)
	b[5] = byte(scr >> 20)
	b[6] = byte(((scr >> 12) & 0xF8) | 0x04 | ((scr >> 13) & 0x03))
	b[7] = byte(scr >> 5)
	b[8] = byte(((scr & 0x1F) << 3) | 0x04)
	b[9] = 0x01
	muxRate := uint32(50000)
	b[10] = byte(0x80 | ((muxRate >> 15) & 0x7F))
	b[11] = byte(muxRate >> 7)
	b[12] = byte(((muxRate & 0x7F) << 1) | 0x01)
	b[13] = 0xF8 // reserved + stuffing length 0
	return b
}

func buildSystemHeader() []byte {
	return systemHeaderStatic
}

func buildProgramStreamMap() []byte {
	return programStreamMapStatic
}

func buildPESPrivateStream1(payload []byte, pts90k uint64) []byte {
	pts := encodePESPTS(pts90k & ((1 << 33) - 1))
	pesLen := len(payload) + 8 // flags(3) + pts(5)
	if pesLen > 0xFFFF {
		pesLen = 0
	}
	b := make([]byte, 0, 14+len(payload))
	b = append(b, 0x00, 0x00, 0x01, 0xBD)
	b = append(b, byte(pesLen>>8), byte(pesLen))
	b = append(b, 0x80, 0x80, 0x05)
	b = append(b, pts...)
	b = append(b, payload...)
	return b
}

func encodePESPTS(pts uint64) []byte {
	return []byte{
		byte(0x20 | (((pts >> 30) & 0x07) << 1) | 0x01),
		byte(pts >> 22),
		byte((((pts >> 15) & 0x7F) << 1) | 0x01),
		byte(pts >> 7),
		byte(((pts & 0x7F) << 1) | 0x01),
	}
}

type programStreamDecoder struct {
	buf []byte
}

func (d *programStreamDecoder) Write(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	d.buf = append(d.buf, data...)
	var out [][]byte
	for {
		if len(d.buf) < 4 {
			return out, nil
		}
		idx := bytes.Index(d.buf, []byte{0x00, 0x00, 0x01})
		if idx < 0 {
			if len(d.buf) > 3 {
				d.buf = append([]byte(nil), d.buf[len(d.buf)-3:]...)
			}
			return out, nil
		}
		if idx > 0 {
			d.buf = d.buf[idx:]
			if len(d.buf) < 4 {
				return out, nil
			}
		}
		startCode := d.buf[3]
		switch startCode {
		case 0xBA:
			if len(d.buf) < 14 {
				return out, nil
			}
			stuffing := int(d.buf[13] & 0x07)
			total := 14 + stuffing
			if len(d.buf) < total {
				return out, nil
			}
			d.buf = d.buf[total:]
		case 0xBB, 0xBC:
			if len(d.buf) < 6 {
				return out, nil
			}
			sectionLen := int(binary.BigEndian.Uint16(d.buf[4:6]))
			total := 6 + sectionLen
			if len(d.buf) < total {
				return out, nil
			}
			d.buf = d.buf[total:]
		case 0xBD:
			if len(d.buf) < 9 {
				return out, nil
			}
			pesLen := int(binary.BigEndian.Uint16(d.buf[4:6]))
			if pesLen == 0 {
				return out, fmt.Errorf("mpeg-ps pes length zero is not supported")
			}
			total := 6 + pesLen
			if len(d.buf) < total {
				return out, nil
			}
			headerDataLen := int(d.buf[8])
			payloadStart := 9 + headerDataLen
			if payloadStart > total {
				return out, fmt.Errorf("mpeg-ps pes payload invalid")
			}
			payload := append([]byte(nil), d.buf[payloadStart:total]...)
			out = append(out, payload)
			d.buf = d.buf[total:]
		default:
			if len(d.buf) < 6 {
				return out, nil
			}
			sectionLen := int(binary.BigEndian.Uint16(d.buf[4:6]))
			total := 6 + sectionLen
			if len(d.buf) < total {
				return out, nil
			}
			d.buf = d.buf[total:]
		}
	}
}
