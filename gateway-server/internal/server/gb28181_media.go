package server

import (
	"strings"
	"sync"
	"time"
)

const (
	gb28181RTPPayloadType = 96
	rtpHeaderSize         = 12
	maxPESPayloadBytes    = 60000
	streamReadChunkBytes  = 32 * 1024
	rtpWriteTimeout       = 30 * time.Second
	rtpReadIdleTimeout    = 10 * time.Second
	rtpReadGraceTimeout   = 2 * time.Second
	rtpSocketBufferBytes  = 4 << 20
	rtpTargetBitrateBps   = 3 * 1024 * 1024
	standardRTPMinSpacing = 200 * time.Microsecond
)

var (
	systemHeaderStatic     = []byte{0x00, 0x00, 0x01, 0xBB, 0x00, 0x09, 0x80, 0x04, 0x04, 0xE1, 0x7F, 0xE0, 0xBD, 0xE0, 0x00}
	programStreamMapStatic = []byte{0x00, 0x00, 0x01, 0xBC, 0x00, 0x12, 0xE0, 0xFF, 0x00, 0x00, 0x00, 0x08, 0x90, 0xBD, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	rtpPacketBufferPool    = sync.Pool{New: func() any { return make([]byte, 0, rtpHeaderSize+rtpChunkBytes) }}
	programStreamChunkPool = sync.Pool{New: func() any { return make([]byte, 0, streamReadChunkBytes+128) }}
)

type rtpPacketHeader struct {
	PayloadType    uint8
	Marker         bool
	SequenceNumber uint16
	Timestamp      uint32
	SSRC           uint32
}

type rtpSendProfile struct {
	name            string
	chunkBytes      int
	bitrateBps      int64
	minSpacing      time.Duration
	socketBuffer    int
	fecEnabled      bool
	fecGroupPackets int
}

func resolveRTPSendProfile(name string) rtpSendProfile {
	trimmed := strings.TrimSpace(name)
	if strings.EqualFold(trimmed, "generic-rtp") {
		return rtpSendProfile{
			name:            "generic-rtp",
			chunkBytes:      genericDownloadRTPPayloadBytes(),
			bitrateBps:      genericDownloadRTPBitrate(),
			minSpacing:      genericDownloadRTPMinSpacing(),
			socketBuffer:    maxIntVal(boundaryRTPSocketBufferBytes(), genericDownloadRTPSocketBufferBytes()),
			fecEnabled:      genericDownloadRTPFECEnabled(),
			fecGroupPackets: genericDownloadRTPFECGroupPackets(),
		}
	}
	if strings.EqualFold(trimmed, "boundary-rtp") {
		return rtpSendProfile{
			name:            "boundary-rtp",
			chunkBytes:      boundaryRTPPayloadBytes(),
			bitrateBps:      boundaryRTPBitrate(),
			minSpacing:      boundaryRTPMinSpacing(),
			socketBuffer:    boundaryRTPSocketBufferBytes(),
			fecEnabled:      boundaryRTPFECEnabled(),
			fecGroupPackets: boundaryRTPFECGroupPackets(),
		}
	}
	return rtpSendProfile{name: "standard", chunkBytes: rtpChunkBytes, bitrateBps: rtpTargetBitrateBps, minSpacing: standardRTPMinSpacing, socketBuffer: maxIntVal(boundaryRTPSocketBufferBytes(), genericDownloadRTPSocketBufferBytes())}
}
