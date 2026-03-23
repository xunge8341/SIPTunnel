package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type relayTransactionTrackerKey struct{}

// relayTransactionTracker 汇总一次隧道事务的关键观测值，目标是把
// 「SIP 控制 → RTP 建链/乱序 → resume → 最终结束」压缩成一条可检索日志。
//
// 设计约束：
// 1. 该结构只保存轻量指标，不持有大对象，避免影响数据面。
// 2. 指标通过原子变量累加，可被 RTP 接收协程、HTTP copy/resume 逻辑并发更新。
// 3. Finalize 只允许输出一次，避免同一 call_id 在成功/失败路径重复刷屏。
type relayTransactionTracker struct {
	startedAt time.Time

	CallID    string
	MappingID string
	DeviceID  string

	requestedMode atomic.Value
	effectiveMode atomic.Value
	finalMode     atomic.Value
	requestClass  atomic.Value
	finalStatus   atomic.Value

	gateWaitMS          atomic.Int64
	responseStartWaitMS atomic.Int64
	rtpSeqGapCount      atomic.Int64
	rtpGapTolerated     atomic.Int64
	rtpGapTimeouts      atomic.Int64
	rtpFECRecovered     atomic.Int64
	rtpPeakPending      atomic.Int64
	rtpPeakGapPackets   atomic.Int64
	rtpMaxGapHoldMS     atomic.Int64
	resumeCount         atomic.Int64
	finalBytes          atomic.Int64
	firstPayloadAtUnix  atomic.Int64

	finalizeOnce sync.Once
}

func newRelayTransactionTracker(callID, mappingID, deviceID string) *relayTransactionTracker {
	t := &relayTransactionTracker{startedAt: time.Now().UTC(), CallID: strings.TrimSpace(callID), MappingID: strings.TrimSpace(mappingID), DeviceID: strings.TrimSpace(deviceID)}
	t.requestedMode.Store("-")
	t.effectiveMode.Store("-")
	t.finalMode.Store("-")
	t.requestClass.Store("-")
	t.finalStatus.Store("-")
	return t
}

func withRelayTransactionTracker(ctx context.Context, tracker *relayTransactionTracker) context.Context {
	if ctx == nil || tracker == nil {
		return ctx
	}
	return context.WithValue(ctx, relayTransactionTrackerKey{}, tracker)
}

func relayTransactionFromContext(ctx context.Context) *relayTransactionTracker {
	if ctx == nil {
		return nil
	}
	tracker, _ := ctx.Value(relayTransactionTrackerKey{}).(*relayTransactionTracker)
	return tracker
}

func (t *relayTransactionTracker) SetModes(requested, effective, final string) {
	if t == nil {
		return
	}
	if v := strings.TrimSpace(requested); v != "" {
		t.requestedMode.Store(v)
	}
	if v := strings.TrimSpace(effective); v != "" {
		t.effectiveMode.Store(v)
	}
	if v := strings.TrimSpace(final); v != "" {
		t.finalMode.Store(v)
	}
}

func (t *relayTransactionTracker) SetRequestClass(class string) {
	if t == nil {
		return
	}
	if v := strings.TrimSpace(class); v != "" {
		t.requestClass.Store(v)
	}
}

func (t *relayTransactionTracker) SetGateWait(wait time.Duration) {
	if t == nil {
		return
	}
	t.gateWaitMS.Store(wait.Milliseconds())
}

func (t *relayTransactionTracker) SetResponseStartWait(wait time.Duration) {
	if t == nil {
		return
	}
	t.responseStartWaitMS.Store(wait.Milliseconds())
}

func (t *relayTransactionTracker) AddResume() {
	if t == nil {
		return
	}
	t.resumeCount.Add(1)
}

func (t *relayTransactionTracker) AddRTPStats(stats rtpStreamStats) {
	if t == nil {
		return
	}
	t.rtpSeqGapCount.Add(int64(stats.SeqGapCount))
	t.rtpGapTolerated.Add(int64(stats.GapTolerated))
	t.rtpGapTimeouts.Add(int64(stats.GapTimeouts))
	t.rtpFECRecovered.Add(int64(stats.FECRecovered))
	storeMaxAtomic(&t.rtpPeakPending, int64(stats.PeakPending))
	storeMaxAtomic(&t.rtpPeakGapPackets, int64(stats.PeakGapPackets))
	storeMaxAtomic(&t.rtpMaxGapHoldMS, stats.MaxGapHoldMS)
	observeRuntimeRTPStats(stats)
}

func storeMaxAtomic(dst *atomic.Int64, candidate int64) {
	if dst == nil || candidate <= 0 {
		return
	}
	for {
		current := dst.Load()
		if candidate <= current {
			return
		}
		if dst.CompareAndSwap(current, candidate) {
			return
		}
	}
}

func (t *relayTransactionTracker) AddFinalBytes(delta int64) {
	if t == nil || delta <= 0 {
		return
	}
	t.finalBytes.Add(delta)
}

func (t *relayTransactionTracker) MarkFirstPayload() {
	if t == nil {
		return
	}
	t.firstPayloadAtUnix.CompareAndSwap(0, time.Now().UTC().UnixNano())
}

func (t *relayTransactionTracker) SetFinalStatus(status string) {
	if t == nil {
		return
	}
	if v := strings.TrimSpace(status); v != "" {
		t.finalStatus.Store(v)
	}
}

func (t *relayTransactionTracker) Finalize() {
	if t == nil {
		return
	}
	t.finalizeOnce.Do(func() {
		firstPayloadMS := int64(-1)
		bodyActiveMS := int64(0)
		if first := t.firstPayloadAtUnix.Load(); first > 0 {
			firstAt := time.Unix(0, first).UTC()
			firstPayloadMS = firstAt.Sub(t.startedAt).Milliseconds()
			bodyActiveMS = time.Since(firstAt).Milliseconds()
		}
		log.Printf("gb28181 relay stage=transaction_summary call_id=%s mapping_id=%s device_id=%s request_class=%s requested_mode=%s effective_mode=%s final_mode=%s gate_wait_ms=%d response_start_wait_ms=%d first_payload_ms=%d body_active_ms=%d rtp_seq_gap_count=%d rtp_gap_tolerated=%d rtp_gap_timeouts=%d rtp_fec_recovered=%d rtp_peak_pending=%d rtp_peak_gap_packets=%d rtp_max_gap_hold_ms=%d resume_count=%d final_bytes=%d final_status=%s elapsed_ms=%d", firstNonEmpty(strings.TrimSpace(t.CallID), "-"), firstNonEmpty(strings.TrimSpace(t.MappingID), "-"), firstNonEmpty(strings.TrimSpace(t.DeviceID), "-"), firstNonEmpty(t.stringValue(t.requestClass), "-"), firstNonEmpty(t.stringValue(t.requestedMode), "-"), firstNonEmpty(t.stringValue(t.effectiveMode), "-"), firstNonEmpty(t.stringValue(t.finalMode), "-"), t.gateWaitMS.Load(), t.responseStartWaitMS.Load(), firstPayloadMS, bodyActiveMS, t.rtpSeqGapCount.Load(), t.rtpGapTolerated.Load(), t.rtpGapTimeouts.Load(), t.rtpFECRecovered.Load(), t.rtpPeakPending.Load(), t.rtpPeakGapPackets.Load(), t.rtpMaxGapHoldMS.Load(), t.resumeCount.Load(), t.finalBytes.Load(), firstNonEmpty(t.stringValue(t.finalStatus), "-"), time.Since(t.startedAt).Milliseconds())
	})
}

func (t *relayTransactionTracker) stringValue(v atomic.Value) string {
	if raw := v.Load(); raw != nil {
		if s, ok := raw.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// trackingReadCloser 在不侵入调用方逻辑的前提下统计实际下发字节数，
// 并在 Close 时输出事务总览日志。
type trackingReadCloser struct {
	io.ReadCloser
	tracker *relayTransactionTracker
}

func (t *trackingReadCloser) Read(p []byte) (int, error) {
	if t == nil || t.ReadCloser == nil {
		return 0, io.EOF
	}
	n, err := t.ReadCloser.Read(p)
	if t.tracker != nil && n > 0 {
		t.tracker.MarkFirstPayload()
		t.tracker.AddFinalBytes(int64(n))
	}
	if t.tracker != nil && err != nil && err != io.EOF {
		t.tracker.SetFinalStatus(trackerStatusError("body_read", err))
	}
	return n, err
}

func (t *trackingReadCloser) Close() error {
	if t == nil || t.ReadCloser == nil {
		if t != nil && t.tracker != nil {
			t.tracker.Finalize()
		}
		return nil
	}
	err := t.ReadCloser.Close()
	if t.tracker != nil {
		if err != nil && t.tracker.stringValue(t.tracker.finalStatus) == "-" {
			t.tracker.SetFinalStatus("body_close_error")
		}
		t.tracker.Finalize()
	}
	return err
}

func trackerStatusHTTP(statusCode int) string {
	if statusCode <= 0 {
		return "-"
	}
	return strconv.Itoa(statusCode)
}

func trackerStatusError(stage string, err error) string {
	stage = strings.TrimSpace(stage)
	if stage == "" {
		stage = "error"
	}
	if err == nil {
		return stage
	}
	return fmt.Sprintf("%s:%s", stage, strings.TrimSpace(err.Error()))
}
