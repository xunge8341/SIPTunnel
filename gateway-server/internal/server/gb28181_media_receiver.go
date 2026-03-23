package server

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"siptunnel/internal/netutil"
	"siptunnel/internal/nodeconfig"
	"siptunnel/internal/service/filetransfer"
)

type rtpStreamStats struct {
	BufferedCount       int
	RecoveredCount      int
	GapTolerated        int
	GapFastForwardCount int
	LossSkipPackets     int
	LateCount           int
	DuplicateCount      int
	GapTimeouts         int
	SeqGapCount         int
	FECPackets          int
	FECRecovered        int
	PeakPending         int
	PeakGapPackets      int
	MaxGapHoldMS        int64
}

type rtpBodyReceiver struct {
	transferID [16]byte
	portPool   filetransfer.RTPPortPool
	pc         net.PacketConn
	listenIP   string
	port       int
	closeOnce  sync.Once
	startOnce  sync.Once

	mu                sync.RWMutex
	expectedBytes     int64
	policy            rtpTolerancePolicy
	callID            string
	deviceID          string
	contentType       string
	rangePlayback     bool
	onTerminalError   func(error, rtpStreamStats)
	onTerminalSuccess func(rtpStreamStats)
}

func newRTPBodyReceiver(local nodeconfig.LocalNodeConfig, portPool filetransfer.RTPPortPool, requestID string) (*rtpBodyReceiver, error) {
	id := md5.Sum([]byte(strings.TrimSpace(requestID)))
	port := 0
	if portPool != nil {
		var err error
		port, err = portPool.Allocate(id)
		if err != nil {
			return nil, err
		}
	}
	listenIP := advertisedRTPIP(local)
	pc, err := net.ListenPacket("udp", net.JoinHostPort(listenIP, fmt.Sprintf("%d", port)))
	if err != nil {
		if portPool != nil {
			portPool.Release(id)
		}
		if netutil.IsAddrInUseError(err) {
			return nil, fmt.Errorf("receiver rtp listen port conflict listen_ip=%s port=%d: %w", listenIP, port, err)
		}
		return nil, err
	}
	if udp, ok := pc.(*net.UDPConn); ok {
		socketBuffer := maxIntVal(boundaryRTPSocketBufferBytes(), genericDownloadRTPSocketBufferBytes())
		_ = udp.SetReadBuffer(socketBuffer)
		_ = udp.SetWriteBuffer(socketBuffer)
	}
	actualPort := port
	if udp, ok := pc.LocalAddr().(*net.UDPAddr); ok {
		actualPort = udp.Port
	}
	return &rtpBodyReceiver{transferID: id, portPool: portPool, pc: pc, listenIP: listenIP, port: actualPort, expectedBytes: -1, policy: chooseRTPTolerancePolicy(nil, nil, nil)}, nil
}

func (r *rtpBodyReceiver) ListenIP() string { return r.listenIP }
func (r *rtpBodyReceiver) Port() int        { return r.port }

func (r *rtpBodyReceiver) ConfigureTolerancePolicy(policy rtpTolerancePolicy) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if policy.ReorderWindow <= 0 {
		policy.ReorderWindow = boundaryRTPReorderWindowPackets()
	}
	if policy.LossTolerance < 0 {
		policy.LossTolerance = boundaryRTPLossTolerancePackets()
	}
	if policy.GapTimeout <= 0 {
		policy.GapTimeout = boundaryRTPGapTimeout()
	}
	if policy.FECGroupSize <= 0 {
		policy.FECGroupSize = boundaryRTPFECGroupPackets()
	}
	if strings.TrimSpace(policy.ProfileName) == "" {
		policy.ProfileName = string(trafficProfileGeneric)
	}
	r.policy = policy
	r.rangePlayback = policy.RangePlayback
	r.mu.Unlock()
}

func (r *rtpBodyReceiver) SetStreamContext(callID, deviceID, contentType string, rangePlayback bool) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.callID = strings.TrimSpace(callID)
	r.deviceID = strings.TrimSpace(deviceID)
	r.contentType = strings.TrimSpace(contentType)
	r.rangePlayback = rangePlayback
	r.mu.Unlock()
}

func (r *rtpBodyReceiver) SetTerminalCallbacks(onSuccess func(rtpStreamStats), onError func(error, rtpStreamStats)) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.onTerminalSuccess = onSuccess
	r.onTerminalError = onError
	r.mu.Unlock()
}

func (r *rtpBodyReceiver) Stream(ctx context.Context, expectedBytes int64, byeCh <-chan struct{}) io.ReadCloser {
	if expectedBytes >= 0 {
		r.SetExpectedBytes(expectedBytes)
	}
	pr, pw := io.Pipe()
	r.startOnce.Do(func() {
		log.Printf("gb28181 media stage=rtp_receiver_stream_start local_rtp=%s:%d expected_bytes=%d", r.listenIP, r.port, r.currentExpectedBytes())
		go r.streamLoop(ctx, byeCh, pw)
	})
	return pr
}

func (r *rtpBodyReceiver) Wait(ctx context.Context, expectedBytes int64, byeCh <-chan struct{}) ([]byte, error) {
	body, err := io.ReadAll(r.Stream(ctx, expectedBytes, byeCh))
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (r *rtpBodyReceiver) SetExpectedBytes(v int64) {
	if r == nil || v < 0 {
		return
	}
	r.mu.Lock()
	if r.expectedBytes < 0 || v < r.expectedBytes {
		r.expectedBytes = v
	}
	r.mu.Unlock()
	_ = r.pc.SetReadDeadline(time.Now())
}

func (r *rtpBodyReceiver) NotifyBYE() {
	if r == nil {
		return
	}
	_ = r.pc.SetReadDeadline(time.Now())
}

func (r *rtpBodyReceiver) currentExpectedBytes() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.expectedBytes
}

func (r *rtpBodyReceiver) currentPolicy() rtpTolerancePolicy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	policy := r.policy
	if policy.ReorderWindow <= 0 {
		policy.ReorderWindow = boundaryRTPReorderWindowPackets()
	}
	if policy.LossTolerance < 0 {
		policy.LossTolerance = boundaryRTPLossTolerancePackets()
	}
	if policy.GapTimeout <= 0 {
		policy.GapTimeout = boundaryRTPGapTimeout()
	}
	if policy.FECGroupSize <= 0 {
		policy.FECGroupSize = boundaryRTPFECGroupPackets()
	}
	if strings.TrimSpace(policy.ProfileName) == "" {
		policy.ProfileName = string(trafficProfileGeneric)
	}
	if r.rangePlayback {
		policy.RangePlayback = true
	}
	return policy
}

func (r *rtpBodyReceiver) snapshotStreamContext() (string, string, string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.callID, r.deviceID, r.contentType, r.rangePlayback
}

func expectedRTPSendProfileForPolicy(policy rtpTolerancePolicy) rtpSendProfile {
	if policy.ProfileName == "generic_download" {
		return resolveRTPSendProfile("generic-rtp")
	}
	return resolveRTPSendProfile("boundary-rtp")
}

func shouldAllowRTPLossSkip(contentType string, rangePlayback bool) bool {
	if rangePlayback {
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(contentType))
	return strings.HasPrefix(lower, "video/") || strings.HasPrefix(lower, "audio/")
}

func (r *rtpBodyReceiver) Close() error {
	var err error
	r.closeOnce.Do(func() {
		err = r.pc.Close()
		if r.portPool != nil {
			r.portPool.Release(r.transferID)
		}
	})
	return err
}

func (r *rtpBodyReceiver) streamLoop(ctx context.Context, byeCh <-chan struct{}, pw *io.PipeWriter) {
	defer r.Close()
	defer func() {
		_ = pw.Close()
	}()
	packet := make([]byte, 64*1024)
	decoder := &programStreamDecoder{}
	var currentSSRC uint32
	var currentFECSSRC uint32
	var started bool
	var emitted int64
	startedAt := time.Now()
	policy := r.currentPolicy()
	callID, deviceID, contentType, rangePlayback := r.snapshotStreamContext()
	reorder := newRTPSequenceReorderBuffer(policy.ReorderWindow, policy.LossTolerance)
	expectedProfile := expectedRTPSendProfileForPolicy(policy)
	var fecTracker *rtpFECSingleParityTracker
	if policy.FECEnabled && policy.FECGroupSize > 1 {
		fecTracker = newRTPFECSingleParityTracker(maxIntVal(policy.ReorderWindow*4, policy.FECGroupSize*8))
	}
	log.Printf("gb28181 media stage=rtp_ps_policy call_id=%s device_id=%s local_rtp=%s:%d payload_bytes=%d bitrate_bps=%d min_spacing_us=%d reorder_window=%d loss_tolerance=%d gap_timeout_ms=%d profile=%s range_playback=%t content_type=%s fec_enabled=%t fec_group_packets=%d socket_buffer_bytes=%d", callID, deviceID, r.listenIP, r.port, expectedProfile.chunkBytes, expectedProfile.bitrateBps, expectedProfile.minSpacing.Microseconds(), policy.ReorderWindow, policy.LossTolerance, policy.GapTimeout.Milliseconds(), policy.ProfileName, rangePlayback, firstNonEmpty(contentType, "-"), policy.FECEnabled, policy.FECGroupSize, expectedProfile.socketBuffer)
	var byeSeen bool
	var gapSince time.Time
	stats := rtpStreamStats{}
	allowLossSkip := shouldAllowRTPLossSkip(contentType, rangePlayback)
	var processOrderedPackets func([]rtpOrderedPacket) error
	rescueEscalated := false
	escalateRangePlaybackRescue := func(reason string, pending int, gapPackets int, gapHoldMS int64) bool {
		if rescueEscalated || !policy.RangePlayback {
			return false
		}
		nextPolicy, ok := escalateRangePlaybackTolerancePolicy(policy)
		if !ok {
			return false
		}
		if !reorder.ExpandTolerance(nextPolicy.ReorderWindow, nextPolicy.LossTolerance) && nextPolicy.GapTimeout <= policy.GapTimeout {
			return false
		}
		prevProfile := policy.ProfileName
		prevReorder := policy.ReorderWindow
		prevLoss := policy.LossTolerance
		prevGapTimeout := policy.GapTimeout
		policy = nextPolicy
		rescueEscalated = true
		log.Printf("gb28181 media stage=rtp_ps_policy_escalated call_id=%s device_id=%s local_rtp=%s:%d body_bytes=%d reason=%s from_profile=%s to_profile=%s reorder_window=%d->%d loss_tolerance=%d->%d gap_timeout_ms=%d->%d pending=%d gap=%d gap_age_ms=%d range_playback=%t", callID, deviceID, r.listenIP, r.port, emitted, reason, prevProfile, nextPolicy.ProfileName, prevReorder, nextPolicy.ReorderWindow, prevLoss, nextPolicy.LossTolerance, prevGapTimeout.Milliseconds(), nextPolicy.GapTimeout.Milliseconds(), pending, gapPackets, gapHoldMS, rangePlayback)
		return true
	}
	shouldLogGapProgress := func(count int, pending int, gapPackets int) bool {
		if count <= 3 {
			return true
		}
		if count%64 == 0 {
			return true
		}
		if pending >= policy.ReorderWindow || gapPackets >= policy.LossTolerance {
			return true
		}
		return false
	}
	updateGapStats := func() (int, int, int64) {
		pending := reorder.PendingCount()
		gapPackets := reorder.PendingGapPackets()
		if pending > stats.PeakPending {
			stats.PeakPending = pending
		}
		if gapPackets > stats.PeakGapPackets {
			stats.PeakGapPackets = gapPackets
		}
		gapHoldMS := int64(0)
		if !gapSince.IsZero() {
			gapHoldMS = time.Since(gapSince).Milliseconds()
			if gapHoldMS > stats.MaxGapHoldMS {
				stats.MaxGapHoldMS = gapHoldMS
			}
		}
		return pending, gapPackets, gapHoldMS
	}
	tryFastForwardGap := func(reason string) error {
		if !allowLossSkip {
			return nil
		}
		ready, skipped, ok := reorder.FastForwardToNextPending(maxIntVal(policy.LossTolerance, 1))
		if !ok {
			return nil
		}
		stats.GapFastForwardCount++
		stats.LossSkipPackets += skipped
		pending, gapPackets, gapHoldMS := updateGapStats()
		log.Printf("gb28181 media stage=rtp_ps_gap_fast_forward call_id=%s device_id=%s local_rtp=%s:%d body_bytes=%d skipped=%d resumed_seq=%d pending=%d gap=%d gap_age_ms=%d reason=%s profile=%s range_playback=%t", callID, deviceID, r.listenIP, r.port, emitted, skipped, ready[0].SequenceNumber, pending, gapPackets, gapHoldMS, reason, policy.ProfileName, rangePlayback)
		if err := processOrderedPackets(ready); err != nil {
			return err
		}
		if reorder.PendingCount() == 0 {
			gapSince = time.Time{}
		} else {
			gapSince = time.Now()
		}
		return nil
	}

	maybeProactiveFastForward := func(reason string, pending int, gapPackets int, gapHoldMS int64) error {
		if !allowLossSkip || gapPackets <= 0 {
			return nil
		}
		if policy.RangePlayback && rescueEscalated {
			if gapPackets > 2 {
				return nil
			}
			if pending < maxIntVal(policy.ReorderWindow/2, 96) {
				return nil
			}
			if gapHoldMS < 30 {
				return nil
			}
			return tryFastForwardGap(reason)
		}
		if policy.ProfileName == "generic_download" {
			if gapPackets > 4 {
				return nil
			}
			if pending < maxIntVal(policy.ReorderWindow/2, 192) {
				return nil
			}
			if gapHoldMS < 60 {
				return nil
			}
			return tryFastForwardGap("generic_download_small_gap_backlog")
		}
		return nil
	}

	checkActiveGapTimeout := func() error {
		if gapSince.IsZero() || !reorder.HasPending() || policy.GapTimeout <= 0 {
			return nil
		}
		pending, gapPackets, gapHoldMS := updateGapStats()
		if gapPackets <= 0 || gapHoldMS < policy.GapTimeout.Milliseconds() {
			return nil
		}
		if err := tryFastForwardGap("active_gap_timeout"); err != nil {
			return err
		} else if gapSince.IsZero() || reorder.PendingGapPackets() == 0 {
			return nil
		}
		gapErr := fmt.Errorf("rtp pending gap timeout expected=%d pending=%d gap=%d wait_ms=%d while_receiving=true", reorder.ExpectedSequence(), pending, gapPackets, gapHoldMS)
		stats.GapTimeouts++
		log.Printf("gb28181 media stage=rtp_ps_gap_stall call_id=%s device_id=%s local_rtp=%s:%d body_bytes=%d expected=%d pending=%d gap=%d gap_age_ms=%d reorder_window=%d loss_tolerance=%d profile=%s range_playback=%t", callID, deviceID, r.listenIP, r.port, emitted, reorder.ExpectedSequence(), pending, gapPackets, gapHoldMS, policy.ReorderWindow, policy.LossTolerance, policy.ProfileName, rangePlayback)
		return gapErr
	}
	reportSuccess := func(completion string) {
		elapsedMS := time.Since(startedAt).Milliseconds()
		bodyBytesPerSec := int64(0)
		bodyBitrateBPS := int64(0)
		if elapsedMS > 0 {
			bodyBytesPerSec = emitted * 1000 / elapsedMS
			bodyBitrateBPS = bodyBytesPerSec * 8
		}
		log.Printf("gb28181 media stage=rtp_ps_summary call_id=%s device_id=%s local_rtp=%s:%d body_bytes=%d elapsed_ms=%d body_bytes_per_sec=%d body_bitrate_bps=%d completion=%s profile=%s range_playback=%t buffered=%d recovered=%d gap_tolerated=%d gap_fast_forward=%d loss_skip_packets=%d late=%d duplicate=%d gap_timeouts=%d seq_gap_count=%d fec_packets=%d fec_recovered=%d peak_pending=%d peak_gap_packets=%d max_gap_hold_ms=%d", callID, deviceID, r.listenIP, r.port, emitted, elapsedMS, bodyBytesPerSec, bodyBitrateBPS, completion, policy.ProfileName, rangePlayback, stats.BufferedCount, stats.RecoveredCount, stats.GapTolerated, stats.GapFastForwardCount, stats.LossSkipPackets, stats.LateCount, stats.DuplicateCount, stats.GapTimeouts, stats.SeqGapCount, stats.FECPackets, stats.FECRecovered, stats.PeakPending, stats.PeakGapPackets, stats.MaxGapHoldMS)
		r.mu.RLock()
		onSuccess := r.onTerminalSuccess
		r.mu.RUnlock()
		if onSuccess != nil {
			onSuccess(stats)
		}
	}
	reportError := func(err error) {
		elapsedMS := time.Since(startedAt).Milliseconds()
		bodyBytesPerSec := int64(0)
		bodyBitrateBPS := int64(0)
		if elapsedMS > 0 {
			bodyBytesPerSec = emitted * 1000 / elapsedMS
			bodyBitrateBPS = bodyBytesPerSec * 8
		}
		log.Printf("gb28181 media stage=rtp_ps_summary call_id=%s device_id=%s local_rtp=%s:%d body_bytes=%d elapsed_ms=%d body_bytes_per_sec=%d body_bitrate_bps=%d completion=error profile=%s range_playback=%t buffered=%d recovered=%d gap_tolerated=%d gap_fast_forward=%d loss_skip_packets=%d late=%d duplicate=%d gap_timeouts=%d seq_gap_count=%d fec_packets=%d fec_recovered=%d peak_pending=%d peak_gap_packets=%d max_gap_hold_ms=%d err=%v", callID, deviceID, r.listenIP, r.port, emitted, elapsedMS, bodyBytesPerSec, bodyBitrateBPS, policy.ProfileName, rangePlayback, stats.BufferedCount, stats.RecoveredCount, stats.GapTolerated, stats.GapFastForwardCount, stats.LossSkipPackets, stats.LateCount, stats.DuplicateCount, stats.GapTimeouts, stats.SeqGapCount, stats.FECPackets, stats.FECRecovered, stats.PeakPending, stats.PeakGapPackets, stats.MaxGapHoldMS, err)
		r.mu.RLock()
		onError := r.onTerminalError
		r.mu.RUnlock()
		if onError != nil {
			onError(err, stats)
		}
	}
	processOrderedPackets = func(orderedPackets []rtpOrderedPacket) error {
		if reorder.PendingCount() == 0 {
			updateGapStats()
			gapSince = time.Time{}
		}
		if len(orderedPackets) > 1 {
			stats.RecoveredCount++
			log.Printf("gb28181 media stage=rtp_ps_reorder_recovered call_id=%s device_id=%s local_rtp=%s:%d body_bytes=%d packets=%d seq_start=%d seq_end=%d pending=%d profile=%s range_playback=%t", callID, deviceID, r.listenIP, r.port, emitted, len(orderedPackets), orderedPackets[0].SequenceNumber, orderedPackets[len(orderedPackets)-1].SequenceNumber, reorder.PendingCount(), policy.ProfileName, rangePlayback)
		}
		for _, ordered := range orderedPackets {
			chunks, decodeErr := decoder.Write(ordered.Payload)
			if decodeErr != nil {
				return decodeErr
			}
			for _, chunk := range chunks {
				expectedBytes := r.currentExpectedBytes()
				if len(chunk) == 0 {
					continue
				}
				if expectedBytes >= 0 {
					remaining := expectedBytes - emitted
					if remaining <= 0 {
						logGB28181Successf("gb28181 media stage=rtp_ps_received local_rtp=%s:%d body_bytes=%d payload_type=%d ssrc=%d completion=content_length", r.listenIP, r.port, emitted, gb28181RTPPayloadType, currentSSRC)
						reportSuccess("content_length")
						return io.EOF
					}
					if int64(len(chunk)) > remaining {
						chunk = chunk[:remaining]
					}
				}
				if _, writeErr := pw.Write(chunk); writeErr != nil {
					return writeErr
				}
				emitted += int64(len(chunk))
			}
		}
		return nil
	}
	handleSequence := func(seq uint16, payload []byte) error {
		orderedPackets, reorderState, reorderErr := reorder.Push(seq, payload)
		if reorderErr != nil {
			if reorderState == "gap_overflow" {
				stats.SeqGapCount++
			}
			return reorderErr
		}
		switch reorderState {
		case "buffered":
			if gapSince.IsZero() {
				gapSince = time.Now()
			}
			stats.BufferedCount++
			stats.SeqGapCount++
			pending, gapPackets, gapHoldMS := updateGapStats()
			if shouldLogGapProgress(stats.BufferedCount, pending, gapPackets) {
				log.Printf("gb28181 media stage=rtp_ps_reorder_buffered call_id=%s device_id=%s local_rtp=%s:%d body_bytes=%d expected=%d got=%d pending=%d gap=%d gap_age_ms=%d reorder_window=%d loss_tolerance=%d profile=%s range_playback=%t", callID, deviceID, r.listenIP, r.port, emitted, reorder.ExpectedSequence(), seq, pending, gapPackets, gapHoldMS, policy.ReorderWindow, policy.LossTolerance, policy.ProfileName, rangePlayback)
			}
			return nil
		case "gap_tolerated":
			if gapSince.IsZero() {
				gapSince = time.Now()
			}
			stats.GapTolerated++
			stats.SeqGapCount++
			pending, gapPackets, gapHoldMS := updateGapStats()
			if !rescueEscalated && policy.RangePlayback && (pending >= policy.ReorderWindow || (stats.GapTolerated >= 32 && pending >= maxIntVal(policy.ReorderWindow/2, 1))) {
				escalateRangePlaybackRescue("persistent_gap_tolerated", pending, gapPackets, gapHoldMS)
				pending, gapPackets, gapHoldMS = updateGapStats()
			}
			if shouldLogGapProgress(stats.GapTolerated, pending, gapPackets) {
				log.Printf("gb28181 media stage=rtp_ps_gap_tolerated call_id=%s device_id=%s local_rtp=%s:%d body_bytes=%d expected=%d got=%d pending=%d gap=%d gap_age_ms=%d reorder_window=%d loss_tolerance=%d profile=%s range_playback=%t", callID, deviceID, r.listenIP, r.port, emitted, reorder.ExpectedSequence(), seq, pending, gapPackets, gapHoldMS, policy.ReorderWindow, policy.LossTolerance, policy.ProfileName, rangePlayback)
			}
			if err := maybeProactiveFastForward("persistent_small_gap_after_rescue", pending, gapPackets, gapHoldMS); err != nil {
				return err
			}
			return nil
		case "late":
			stats.LateCount++
			return nil
		case "duplicate":
			stats.DuplicateCount++
			return nil
		default:
			return processOrderedPackets(orderedPackets)
		}
	}
	for {
		expectedBytes := r.currentExpectedBytes()
		if expectedBytes >= 0 && emitted >= expectedBytes {
			logGB28181Successf("gb28181 media stage=rtp_ps_received local_rtp=%s:%d body_bytes=%d payload_type=%d ssrc=%d completion=content_length", r.listenIP, r.port, emitted, gb28181RTPPayloadType, currentSSRC)
			reportSuccess("content_length")
			return
		}
		select {
		case <-ctx.Done():
			reportError(ctx.Err())
			_ = pw.CloseWithError(ctx.Err())
			return
		default:
		}
		if !byeSeen {
			select {
			case <-byeCh:
				byeSeen = true
			default:
			}
		}
		deadline := time.Now().Add(rtpReadIdleTimeout)
		if byeSeen {
			deadline = time.Now().Add(rtpReadGraceTimeout)
		}
		if err := r.pc.SetReadDeadline(deadline); err != nil {
			reportError(err)
			_ = pw.CloseWithError(err)
			return
		}
		n, _, err := r.pc.ReadFrom(packet)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				if reorder.HasPending() && !gapSince.IsZero() && time.Since(gapSince) >= policy.GapTimeout {
					if fastForwardErr := tryFastForwardGap("read_timeout"); fastForwardErr != nil {
						log.Printf("gb28181 media stage=rtp_ps_receive_error local_rtp=%s:%d body_bytes=%d err=%v", r.listenIP, r.port, emitted, fastForwardErr)
						reportError(fastForwardErr)
						_ = pw.CloseWithError(fastForwardErr)
						return
					}
					if reorder.HasPending() && !gapSince.IsZero() && time.Since(gapSince) >= policy.GapTimeout {
						gapErr := fmt.Errorf("rtp pending gap timeout expected=%d pending=%d gap=%d wait_ms=%d", reorder.ExpectedSequence(), reorder.PendingCount(), reorder.PendingGapPackets(), policy.GapTimeout.Milliseconds())
						stats.GapTimeouts++
						log.Printf("gb28181 media stage=rtp_ps_gap_timeout call_id=%s device_id=%s local_rtp=%s:%d body_bytes=%d err=%v profile=%s range_playback=%t", callID, deviceID, r.listenIP, r.port, emitted, gapErr, policy.ProfileName, rangePlayback)
						reportError(gapErr)
						_ = pw.CloseWithError(gapErr)
						return
					}
				}
				if byeSeen {
					if reorder.HasPending() {
						if fastForwardErr := tryFastForwardGap("bye_idle"); fastForwardErr != nil {
							log.Printf("gb28181 media stage=rtp_ps_receive_error local_rtp=%s:%d body_bytes=%d err=%v", r.listenIP, r.port, emitted, fastForwardErr)
							reportError(fastForwardErr)
							_ = pw.CloseWithError(fastForwardErr)
							return
						}
					}
					if reorder.HasPending() {
						gapErr := fmt.Errorf("rtp pending gap on bye expected=%d pending=%d gap=%d", reorder.ExpectedSequence(), reorder.PendingCount(), reorder.PendingGapPackets())
						stats.GapTimeouts++
						log.Printf("gb28181 media stage=rtp_ps_gap_timeout call_id=%s device_id=%s local_rtp=%s:%d body_bytes=%d err=%v profile=%s range_playback=%t", callID, deviceID, r.listenIP, r.port, emitted, gapErr, policy.ProfileName, rangePlayback)
						reportError(gapErr)
						_ = pw.CloseWithError(gapErr)
						return
					}
					if expectedBytes >= 0 && emitted < expectedBytes {
						shortErr := fmt.Errorf("rtp bye before content_length emitted=%d expected=%d", emitted, expectedBytes)
						stats.GapTimeouts++
						log.Printf("gb28181 media stage=rtp_ps_gap_timeout call_id=%s device_id=%s local_rtp=%s:%d body_bytes=%d err=%v profile=%s range_playback=%t", callID, deviceID, r.listenIP, r.port, emitted, shortErr, policy.ProfileName, rangePlayback)
						reportError(io.ErrUnexpectedEOF)
						_ = pw.CloseWithError(io.ErrUnexpectedEOF)
						return
					}
					logGB28181Successf("gb28181 media stage=rtp_ps_received local_rtp=%s:%d body_bytes=%d payload_type=%d ssrc=%d completion=bye_idle", r.listenIP, r.port, emitted, gb28181RTPPayloadType, currentSSRC)
					reportSuccess("bye_idle")
					return
				}
				continue
			}
			reportError(err)
			_ = pw.CloseWithError(err)
			return
		}
		hdr, payload, err := decodeRTPPacket(packet[:n])
		if err != nil {
			continue
		}
		if hdr.PayloadType == rtpFECPayloadType {
			if fecTracker == nil {
				continue
			}
			if currentFECSSRC == 0 {
				currentFECSSRC = hdr.SSRC
			}
			if hdr.SSRC != currentFECSSRC {
				continue
			}
			stats.FECPackets++
			recoveredPackets, fecErr := fecTracker.ObserveFEC(payload, reorder.ExpectedSequence())
			if fecErr != nil {
				log.Printf("gb28181 media stage=rtp_ps_fec_invalid call_id=%s device_id=%s local_rtp=%s:%d err=%v profile=%s range_playback=%t", callID, deviceID, r.listenIP, r.port, fecErr, policy.ProfileName, rangePlayback)
				continue
			}
			for _, recovered := range recoveredPackets {
				stats.FECRecovered++
				log.Printf("gb28181 media stage=rtp_ps_fec_recovered call_id=%s device_id=%s local_rtp=%s:%d seq=%d group_base=%d group_packets=%d pending=%d profile=%s range_playback=%t", callID, deviceID, r.listenIP, r.port, recovered.SequenceNumber, recovered.BaseSequence, recovered.GroupPackets, reorder.PendingCount(), policy.ProfileName, rangePlayback)
				if pushErr := handleSequence(recovered.SequenceNumber, recovered.Payload); pushErr != nil {
					if pushErr == io.EOF {
						return
					}
					log.Printf("gb28181 media stage=rtp_ps_receive_error local_rtp=%s:%d body_bytes=%d err=%v", r.listenIP, r.port, emitted, pushErr)
					reportError(pushErr)
					_ = pw.CloseWithError(pushErr)
					return
				}
				if stallErr := checkActiveGapTimeout(); stallErr != nil {
					log.Printf("gb28181 media stage=rtp_ps_receive_error local_rtp=%s:%d body_bytes=%d err=%v", r.listenIP, r.port, emitted, stallErr)
					reportError(stallErr)
					_ = pw.CloseWithError(stallErr)
					return
				}
			}
			continue
		}
		if hdr.PayloadType != gb28181RTPPayloadType {
			continue
		}
		if !started {
			started = true
			currentSSRC = hdr.SSRC
			if currentFECSSRC == 0 {
				currentFECSSRC = deriveRTPFECSSRC(currentSSRC)
			}
		}
		if hdr.SSRC != currentSSRC {
			err := fmt.Errorf("rtp stream switched ssrc from %d to %d", currentSSRC, hdr.SSRC)
			log.Printf("gb28181 media stage=rtp_ps_receive_error local_rtp=%s:%d body_bytes=%d err=%v", r.listenIP, r.port, emitted, err)
			reportError(err)
			_ = pw.CloseWithError(err)
			return
		}
		if fecTracker != nil {
			recoveredPackets := fecTracker.ObserveData(hdr.SequenceNumber, payload, reorder.ExpectedSequence())
			for _, recovered := range recoveredPackets {
				stats.FECRecovered++
				log.Printf("gb28181 media stage=rtp_ps_fec_recovered call_id=%s device_id=%s local_rtp=%s:%d seq=%d group_base=%d group_packets=%d pending=%d profile=%s range_playback=%t", callID, deviceID, r.listenIP, r.port, recovered.SequenceNumber, recovered.BaseSequence, recovered.GroupPackets, reorder.PendingCount(), policy.ProfileName, rangePlayback)
				if pushErr := handleSequence(recovered.SequenceNumber, recovered.Payload); pushErr != nil {
					if pushErr == io.EOF {
						return
					}
					log.Printf("gb28181 media stage=rtp_ps_receive_error local_rtp=%s:%d body_bytes=%d err=%v", r.listenIP, r.port, emitted, pushErr)
					reportError(pushErr)
					_ = pw.CloseWithError(pushErr)
					return
				}
				if stallErr := checkActiveGapTimeout(); stallErr != nil {
					log.Printf("gb28181 media stage=rtp_ps_receive_error local_rtp=%s:%d body_bytes=%d err=%v", r.listenIP, r.port, emitted, stallErr)
					reportError(stallErr)
					_ = pw.CloseWithError(stallErr)
					return
				}
			}
		}
		if pushErr := handleSequence(hdr.SequenceNumber, payload); pushErr != nil {
			if pushErr == io.EOF {
				return
			}
			log.Printf("gb28181 media stage=rtp_ps_receive_error local_rtp=%s:%d body_bytes=%d err=%v", r.listenIP, r.port, emitted, pushErr)
			reportError(pushErr)
			_ = pw.CloseWithError(pushErr)
			return
		}
		if stallErr := checkActiveGapTimeout(); stallErr != nil {
			log.Printf("gb28181 media stage=rtp_ps_receive_error local_rtp=%s:%d body_bytes=%d err=%v", r.listenIP, r.port, emitted, stallErr)
			reportError(stallErr)
			_ = pw.CloseWithError(stallErr)
			return
		}
	}
}
