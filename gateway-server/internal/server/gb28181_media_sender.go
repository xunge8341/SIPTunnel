package server

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"siptunnel/internal/netutil"
	"siptunnel/internal/nodeconfig"
	"siptunnel/internal/service/filetransfer"
)

type rtpBodySender struct {
	transferID [16]byte
	portPool   filetransfer.RTPPortPool
	pc         net.PacketConn
	listenIP   string
	port       int
	ssrc       uint32
	seq        uint32
	fecSSRC    uint32
	fecSeq     uint32
	closeOnce  sync.Once
}

func newRTPBodySender(local nodeconfig.LocalNodeConfig, portPool filetransfer.RTPPortPool, requestID string) (*rtpBodySender, error) {
	id := md5.Sum([]byte(strings.TrimSpace(requestID)))
	listenIP := advertisedRTPIP(local)
	// Sender sockets must not contend with the receiver-side RTP pool. In same-host
	// 联调或双实例部署中，固定端口 bind 很容易与对端 remote_rtp 冲突，导致 INVITE 随机 500。
	// 发送端统一使用系统分配的临时端口，并把实际端口回填到 200 OK SDP。
	pc, err := net.ListenPacket("udp", net.JoinHostPort(listenIP, "0"))
	if err != nil {
		if netutil.IsAddrInUseError(err) {
			return nil, fmt.Errorf("sender rtp listen port conflict listen_ip=%s port=0: %w", listenIP, err)
		}
		return nil, err
	}
	if udp, ok := pc.(*net.UDPConn); ok {
		_ = udp.SetWriteBuffer(maxIntVal(boundaryRTPSocketBufferBytes(), genericDownloadRTPSocketBufferBytes()))
		_ = udp.SetReadBuffer(maxIntVal(boundaryRTPSocketBufferBytes(), genericDownloadRTPSocketBufferBytes()))
	}
	actualPort := 0
	if udp, ok := pc.LocalAddr().(*net.UDPAddr); ok {
		actualPort = udp.Port
	}
	ssrc := binary.BigEndian.Uint32(id[:4])
	if ssrc == 0 {
		ssrc = 1
	}
	seed := uint32(time.Now().UTC().UnixNano())
	return &rtpBodySender{transferID: id, portPool: nil, pc: pc, listenIP: listenIP, port: actualPort, ssrc: ssrc, seq: seed, fecSSRC: deriveRTPFECSSRC(ssrc), fecSeq: seed ^ 0x00FF00FF}, nil
}

func (s *rtpBodySender) ListenIP() string { return s.listenIP }
func (s *rtpBodySender) Port() int        { return s.port }

func (s *rtpBodySender) Close() error {
	var err error
	s.closeOnce.Do(func() {
		err = s.pc.Close()
		if s.portPool != nil {
			s.portPool.Release(s.transferID)
		}
	})
	return err
}

func (s *rtpBodySender) sendPacket(ctx context.Context, udpAddr *net.UDPAddr, header rtpPacketHeader, payload []byte, pacer *rtpSendPacer, deadline *time.Time) error {
	// 注意：边界口 payload 大小由 runtime transport_tuning 决定，可能大于池子初始化时的默认 chunk。
	// 这里必须先按实际需要扩容，否则在 status=206 / boundary-rtp 场景下会出现 slice bounds out of range。
	need := rtpHeaderSize + len(payload)
	pkt := rtpPacketBufferPool.Get().([]byte)
	if cap(pkt) < need {
		pkt = make([]byte, need)
	} else {
		pkt = pkt[:need]
	}
	pkt[0] = 0x80
	pkt[1] = header.PayloadType & 0x7F
	if header.Marker {
		pkt[1] |= 0x80
	}
	binary.BigEndian.PutUint16(pkt[2:4], header.SequenceNumber)
	binary.BigEndian.PutUint32(pkt[4:8], header.Timestamp)
	binary.BigEndian.PutUint32(pkt[8:12], header.SSRC)
	copy(pkt[12:], payload)
	defer func() {
		if cap(pkt) > rtpHeaderSize+rtpChunkBytes {
			pkt = pkt[:0]
		} else {
			pkt = pkt[:0]
		}
		rtpPacketBufferPool.Put(pkt)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if pacer != nil {
		if err := pacer.Wait(ctx); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if deadline != nil {
		now := time.Now()
		if deadline.IsZero() || now.After(deadline.Add(-5*time.Second)) {
			*deadline = now.Add(rtpWriteTimeout)
			if err := s.pc.SetWriteDeadline(*deadline); err != nil {
				return err
			}
		}
	}
	if _, err := s.pc.WriteTo(pkt, udpAddr); err != nil {
		return err
	}
	if pacer != nil {
		pacer.Advance(len(pkt))
	}
	return nil
}

func buildPackHeaderInto(dst []byte, scr uint64) []byte {
	dst = append(dst, 0x00, 0x00, 0x01, 0xBA)
	dst = append(dst,
		0x44|byte((scr>>27)&0x38)|byte((scr>>28)&0x03),
		byte(scr>>20),
		byte(((scr>>12)&0xF8)|0x04|((scr>>13)&0x03)),
		byte(scr>>5),
		byte(((scr&0x1F)<<3)|0x04),
		0x01,
	)
	muxRate := uint32(50000)
	dst = append(dst,
		byte(0x80|((muxRate>>15)&0x7F)),
		byte(muxRate>>7),
		byte(((muxRate&0x7F)<<1)|0x01),
		0xF8,
	)
	return dst
}

func buildPESPrivateStream1Into(dst []byte, payload []byte, pts90k uint64) []byte {
	pts := encodePESPTS(pts90k & ((1 << 33) - 1))
	pesLen := len(payload) + 8
	if pesLen > 0xFFFF {
		pesLen = 0
	}
	dst = append(dst, 0x00, 0x00, 0x01, 0xBD, byte(pesLen>>8), byte(pesLen), 0x80, 0x80, 0x05)
	dst = append(dst, pts...)
	dst = append(dst, payload...)
	return dst
}

func (s *rtpBodySender) Send(ctx context.Context, ip string, port int, body []byte) error {
	_, _, _, _, err := s.SendStreamWithProfile(ctx, ip, port, bytes.NewReader(body), "")
	return err
}

func (s *rtpBodySender) SendStreamWithProfile(ctx context.Context, ip string, port int, body io.Reader, profileName string) (bodyBytes int64, psBytes int64, pesCount int, rtpPackets int, err error) {
	if s == nil {
		return 0, 0, 0, 0, fmt.Errorf("rtp sender is nil")
	}
	remoteIP := net.ParseIP(strings.TrimSpace(ip))
	if remoteIP == nil || port <= 0 {
		return 0, 0, 0, 0, fmt.Errorf("invalid rtp endpoint %s:%d", ip, port)
	}
	udpAddr := &net.UDPAddr{IP: remoteIP, Port: port}
	profile := resolveRTPSendProfile(profileName)
	var genericLease genericDownloadLease
	if profile.name == "generic-rtp" {
		deviceID, target, transferID, _ := genericDownloadContextInfo(ctx)
		genericLease = globalGenericDownloadController.acquire(deviceID, target, transferID)
		if genericLease.effectiveBPS > 0 && genericLease.effectiveBPS < profile.bitrateBps {
			profile.bitrateBps = genericLease.effectiveBPS
		}
		defer func() {
			globalGenericDownloadController.release(genericLease, err)
		}()
	}
	pts := uint64(time.Now().UTC().UnixNano()/1e6*90) + 1
	buf := make([]byte, streamReadChunkBytes)
	readNext := func() ([]byte, error) {
		n, readErr := body.Read(buf)
		if n > 0 {
			chunk := programStreamChunkPool.Get().([]byte)
			chunk = append(chunk[:0], buf[:n]...)
			return chunk, nil
		}
		if readErr != nil {
			return nil, readErr
		}
		return nil, nil
	}
	current, readErr := readNext()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return 0, 0, 0, 0, readErr
	}
	if current == nil && errors.Is(readErr, io.EOF) {
		current = []byte{}
	}
	startSeq := uint16(s.seq & 0xFFFF)
	startedAt := time.Now()
	pacer := newRTPSendPacer(profile)
	var writeDeadline time.Time
	fecPackets := 0
	for current != nil {
		next, nextErr := readNext()
		final := errors.Is(nextErr, io.EOF)
		if nextErr != nil && !final {
			return bodyBytes, psBytes, pesCount, rtpPackets, nextErr
		}
		fragment := programStreamChunkPool.Get().([]byte)
		fragment = buildPackHeaderInto(fragment[:0], pts)
		fragment = buildPESPrivateStream1Into(fragment, current, pts)
		localPackets, localFECPackets, sendErr := s.sendProgramStreamFragment(ctx, udpAddr, uint32(pts), fragment, final, pacer, &writeDeadline, profile)
		if sendErr != nil {
			programStreamChunkPool.Put(fragment[:0])
			return bodyBytes, psBytes, pesCount, rtpPackets, sendErr
		}
		bodyBytes += int64(len(current))
		psBytes += int64(len(fragment))
		programStreamChunkPool.Put(fragment[:0])
		pesCount++
		rtpPackets += localPackets
		fecPackets += localFECPackets
		pts += 3600
		programStreamChunkPool.Put(current[:0])
		current = next
		if final {
			break
		}
	}
	elapsed := time.Since(startedAt)
	bodyBytesPerSec := int64(0)
	psBytesPerSec := int64(0)
	bodyBitrateBPS := int64(0)
	psBitrateBPS := int64(0)
	rtpPPS := int64(0)
	if elapsed > 0 {
		bodyBytesPerSec = int64(float64(bodyBytes) / elapsed.Seconds())
		psBytesPerSec = int64(float64(psBytes) / elapsed.Seconds())
		bodyBitrateBPS = bodyBytesPerSec * 8
		psBitrateBPS = psBytesPerSec * 8
		rtpPPS = int64(float64(rtpPackets) / elapsed.Seconds())
	}
	if strings.EqualFold(profile.name, "generic-rtp") && strings.TrimSpace(genericLease.target) != "" {
		globalGenericDownloadController.observeSourceRead(genericLease.target, genericLease.transferID, bodyBitrateBPS, profile.bitrateBps, genericLease.activeSegmentsTransfer)
	}
	logGB28181Successf("gb28181 media stage=rtp_ps_sent local_rtp=%s:%d remote_rtp=%s:%d body_bytes=%d ps_bytes=%d pes_packets=%d rtp_packets=%d fec_packets=%d seq_start=%d seq_end=%d payload_type=%d ssrc=%d fec_ssrc=%d profile=%s elapsed_ms=%d body_bytes_per_sec=%d body_bitrate_bps=%d ps_bytes_per_sec=%d ps_bitrate_bps=%d rtp_pps=%d socket_buffer_bytes=%d effective_transfer_bitrate_bps=%d effective_segment_bitrate_bps=%d limiter_active_transfers_global=%d limiter_active_transfers_device=%d limiter_active_segments_global=%d limiter_active_segments_device=%d limiter_active_segments_transfer=%d transfer_id=%s transfer_id_source=%s breaker_open=%t floor_applied=%t source_constrained=%t same_transfer_split_enabled=%t same_transfer_split_applied=%t fec_enabled=%t fec_group_packets=%d", s.listenIP, s.port, udpAddr.IP.String(), udpAddr.Port, bodyBytes, psBytes, pesCount, rtpPackets, fecPackets, startSeq, uint16((s.seq-1)&0xFFFF), gb28181RTPPayloadType, s.ssrc, s.fecSSRC, profile.name, elapsed.Milliseconds(), bodyBytesPerSec, bodyBitrateBPS, psBytesPerSec, psBitrateBPS, rtpPPS, profile.socketBuffer, genericLease.effectiveTransferBPS, profile.bitrateBps, genericLease.activeTransfersGlobal, genericLease.activeTransfersPerDevice, genericLease.activeSegmentsGlobal, genericLease.activeSegmentsPerDevice, genericLease.activeSegmentsTransfer, firstNonEmpty(strings.TrimSpace(genericLease.transferID), "-"), firstNonEmpty(strings.TrimSpace(genericLease.transferIDSource), "-"), genericLease.breakerOpen, genericLease.floorApplied, genericLease.sourceConstrained, genericLease.sameTransferSplitEnabled, genericLease.sameTransferSplitApplied, profile.fecEnabled, profile.fecGroupPackets)
	return bodyBytes, psBytes, pesCount, rtpPackets, nil
}

func (s *rtpBodySender) sendProgramStreamFragment(ctx context.Context, udpAddr *net.UDPAddr, timestamp uint32, ps []byte, final bool, pacer *rtpSendPacer, deadline *time.Time, profile rtpSendProfile) (int, int, error) {
	if len(ps) == 0 {
		ps = buildProgramStream(nil, uint64(timestamp))
	}
	chunkBytes := profile.chunkBytes
	if chunkBytes <= 0 {
		chunkBytes = rtpChunkBytes
	}
	chunks := (len(ps) + chunkBytes - 1) / chunkBytes
	if chunks == 0 {
		chunks = 1
	}
	fecPackets := 0
	groupPackets := 0
	groupStartSeq := uint16(0)
	groupLengths := make([]int, 0, maxIntVal(profile.fecGroupPackets, 2))
	var parity []byte
	if profile.fecEnabled && profile.fecGroupPackets > 1 && chunkBytes > 0 {
		parity = make([]byte, chunkBytes)
	}
	flushFEC := func() (int, error) {
		if len(parity) != chunkBytes || groupPackets < 2 || len(groupLengths) != groupPackets {
			groupPackets = 0
			groupLengths = groupLengths[:0]
			return 0, nil
		}
		fecPayload, fecErr := buildRTPFECPayload(groupStartSeq, append([]int(nil), groupLengths...), parity)
		if fecErr != nil {
			return 0, fecErr
		}
		fecSeq := uint16(s.fecSeq & 0xFFFF)
		s.fecSeq++
		if err := s.sendPacket(ctx, udpAddr, rtpPacketHeader{PayloadType: rtpFECPayloadType, Marker: false, SequenceNumber: fecSeq, Timestamp: timestamp, SSRC: s.fecSSRC}, fecPayload, pacer, deadline); err != nil {
			return 0, err
		}
		fecPackets++
		groupPackets = 0
		groupLengths = groupLengths[:0]
		for j := range parity {
			parity[j] = 0
		}
		return 1, nil
	}
	for i := 0; i < chunks; i++ {
		start := i * chunkBytes
		end := start + chunkBytes
		if end > len(ps) {
			end = len(ps)
		}
		payload := ps[start:end]
		seq := uint16(s.seq & 0xFFFF)
		s.seq++
		if err := s.sendPacket(ctx, udpAddr, rtpPacketHeader{PayloadType: gb28181RTPPayloadType, Marker: final && i == chunks-1, SequenceNumber: seq, Timestamp: timestamp, SSRC: s.ssrc}, payload, pacer, deadline); err != nil {
			return i + fecPackets, fecPackets, err
		}
		if len(parity) == chunkBytes {
			if groupPackets == 0 {
				groupStartSeq = seq
				for j := range parity {
					parity[j] = 0
				}
				groupLengths = groupLengths[:0]
			}
			for j := 0; j < len(payload); j++ {
				parity[j] ^= payload[j]
			}
			groupLengths = append(groupLengths, len(payload))
			groupPackets++
			shouldFlush := groupPackets == profile.fecGroupPackets || (i == chunks-1 && groupPackets >= 2)
			if shouldFlush {
				if _, err := flushFEC(); err != nil {
					return i + 1 + fecPackets, fecPackets, err
				}
			}
		}
	}
	return chunks + fecPackets, fecPackets, nil
}
