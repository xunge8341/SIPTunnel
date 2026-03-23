package server

import (
	"strings"
	"sync/atomic"
	"time"
)

type runtimeQualitySnapshot struct {
	WindowSeconds          int    `json:"window_seconds"`
	LastJitterLossAt       string `json:"last_jitter_loss_at,omitempty"`
	RecentJitterLossEvents uint64 `json:"recent_jitter_loss_events,omitempty"`
	RecentGapTimeouts      uint64 `json:"recent_gap_timeouts,omitempty"`
	RecentFECRecovered     uint64 `json:"recent_fec_recovered,omitempty"`
	RecentPeakPending      int    `json:"recent_peak_pending,omitempty"`
	RecentMaxGapHoldMS     int64  `json:"recent_max_gap_hold_ms,omitempty"`
	LastWriterBlockAt      string `json:"last_writer_block_at,omitempty"`
	RecentWriterBlockMS    int64  `json:"recent_writer_block_ms,omitempty"`
	RecentMaxWriterBlockMS int64  `json:"recent_max_writer_block_ms,omitempty"`
	LastContextCanceledAt  string `json:"last_context_canceled_at,omitempty"`
	RecentContextCanceled  uint64 `json:"recent_context_canceled,omitempty"`
	CircuitOpenCount       int    `json:"circuit_open_count,omitempty"`
	CircuitHalfOpenCount   int    `json:"circuit_half_open_count,omitempty"`
	CircuitLastOpenReason  string `json:"circuit_last_open_reason,omitempty"`
	CircuitLastOpenUntil   string `json:"circuit_last_open_until,omitempty"`
}

type runtimeQualityObserver struct {
	jitterLossEvents atomic.Uint64
	gapTimeouts      atomic.Uint64
	fecRecovered     atomic.Uint64
	peakPending      atomic.Int64
	maxGapHoldMS     atomic.Int64
	writerBlockMS    atomic.Int64
	maxWriterBlockMS atomic.Int64
	contextCanceled  atomic.Uint64

	lastJitterLossAt      atomic.Int64
	lastWriterBlockAt     atomic.Int64
	lastContextCanceledAt atomic.Int64
}

var globalRuntimeQuality runtimeQualityObserver

const runtimeQualityRecentWindow = 10 * time.Minute

func observeRuntimeRTPStats(stats rtpStreamStats) {
	if stats.SeqGapCount > 0 || stats.GapTolerated > 0 || stats.GapTimeouts > 0 {
		globalRuntimeQuality.jitterLossEvents.Add(uint64(maxIntVal(1, stats.SeqGapCount+stats.GapTolerated+stats.GapTimeouts)))
		globalRuntimeQuality.lastJitterLossAt.Store(time.Now().UTC().UnixNano())
	}
	if stats.GapTimeouts > 0 {
		globalRuntimeQuality.gapTimeouts.Add(uint64(stats.GapTimeouts))
	}
	if stats.FECRecovered > 0 {
		globalRuntimeQuality.fecRecovered.Add(uint64(stats.FECRecovered))
	}
	if stats.PeakPending > 0 {
		storeMaxAtomic(&globalRuntimeQuality.peakPending, int64(stats.PeakPending))
		globalRuntimeQuality.lastJitterLossAt.Store(time.Now().UTC().UnixNano())
	}
	if stats.MaxGapHoldMS > 0 {
		storeMaxAtomic(&globalRuntimeQuality.maxGapHoldMS, stats.MaxGapHoldMS)
		globalRuntimeQuality.lastJitterLossAt.Store(time.Now().UTC().UnixNano())
	}
}

func observeRuntimeWriterBlock(d time.Duration) {
	if d <= 0 {
		return
	}
	ms := d.Milliseconds()
	if ms <= 0 {
		return
	}
	globalRuntimeQuality.writerBlockMS.Store(ms)
	storeMaxAtomic(&globalRuntimeQuality.maxWriterBlockMS, ms)
	globalRuntimeQuality.lastWriterBlockAt.Store(time.Now().UTC().UnixNano())
}

func observeRuntimeCopyError(err error) {
	if err == nil {
		return
	}
	errText := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(errText, "context canceled") || strings.Contains(errText, "canceled") || strings.Contains(errText, "cancelled") {
		globalRuntimeQuality.contextCanceled.Add(1)
		globalRuntimeQuality.lastContextCanceledAt.Store(time.Now().UTC().UnixNano())
	}
}

func snapshotRuntimeQuality(now time.Time) runtimeQualitySnapshot {
	snap := runtimeQualitySnapshot{WindowSeconds: int(runtimeQualityRecentWindow.Seconds())}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if ts := globalRuntimeQuality.lastJitterLossAt.Load(); ts > 0 {
		at := time.Unix(0, ts).UTC()
		if now.Sub(at) <= runtimeQualityRecentWindow {
			snap.LastJitterLossAt = formatTimestamp(at)
			snap.RecentJitterLossEvents = globalRuntimeQuality.jitterLossEvents.Load()
			snap.RecentGapTimeouts = globalRuntimeQuality.gapTimeouts.Load()
			snap.RecentFECRecovered = globalRuntimeQuality.fecRecovered.Load()
			snap.RecentPeakPending = int(globalRuntimeQuality.peakPending.Load())
			snap.RecentMaxGapHoldMS = globalRuntimeQuality.maxGapHoldMS.Load()
		}
	}
	if ts := globalRuntimeQuality.lastWriterBlockAt.Load(); ts > 0 {
		at := time.Unix(0, ts).UTC()
		if now.Sub(at) <= runtimeQualityRecentWindow {
			snap.LastWriterBlockAt = formatTimestamp(at)
			snap.RecentWriterBlockMS = globalRuntimeQuality.writerBlockMS.Load()
			snap.RecentMaxWriterBlockMS = globalRuntimeQuality.maxWriterBlockMS.Load()
		}
	}
	if ts := globalRuntimeQuality.lastContextCanceledAt.Load(); ts > 0 {
		at := time.Unix(0, ts).UTC()
		if now.Sub(at) <= runtimeQualityRecentWindow {
			snap.LastContextCanceledAt = formatTimestamp(at)
			snap.RecentContextCanceled = globalRuntimeQuality.contextCanceled.Load()
		}
	}
	circuit := defaultUpstreamCircuitGuard.Snapshot(now)
	snap.CircuitOpenCount = circuit.OpenCount
	snap.CircuitHalfOpenCount = circuit.HalfOpenCount
	snap.CircuitLastOpenReason = strings.TrimSpace(circuit.LastOpenReason)
	snap.CircuitLastOpenUntil = strings.TrimSpace(circuit.LastOpenUntil)
	return snap
}
