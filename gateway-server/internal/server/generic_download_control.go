package server

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"
)

type genericDownloadState struct {
	DeviceID                 string
	Target                   string
	TransferID               string
	ActiveTransfersGlobal    int
	ActiveTransfersPerDevice int
	ActiveSegmentsGlobal     int
	ActiveSegmentsPerDevice  int
	ActiveSegmentsTransfer   int
	ConsecutiveFailures      int
	BreakerOpenUntil         time.Time
	LastFailureReason        string
	LastTouchedAt            time.Time
	SourceConstrainedHits    int
	SourceConstrainedUntil   time.Time
	LastObservedBodyBitrate  int64
	LastObservedEffectiveBPS int64
	LastObservedSegments     int
}

// genericDownloadController 的公平分享单位是“外层下载事务”，而不是内部 4MiB/16MiB 分段。
// 这样一个下载即使开启了多个 segment child，也只会占用一个全局带宽配额；
// 否则同一下载的多个子请求会把 active_global 错算大，导致限流既不公平，也无法阻止单个下载吃满带宽。
type genericDownloadController struct {
	mu                       sync.Mutex
	activeTransfersGlobal    int
	activeTransfersPerDevice map[string]int
	activeSegmentsGlobal     int
	activeSegmentsPerDevice  map[string]int
	states                   map[string]*genericDownloadState
	lastPruneAt              time.Time
}

const genericDownloadStatePruneInterval = time.Minute

const (
	genericDownloadSourceConstraintHold            = 45 * time.Second
	genericDownloadSourceConstraintOpenRatioPct    = 85
	genericDownloadSourceConstraintRecoverFloorBPS = 4 * 1024 * 1024
)

func genericDownloadTargetStateKey(target string) string {
	target = strings.TrimSpace(target)
	return genericDownloadStateKey("", target, target)
}

func isSevereGenericDownloadFailureReason(reason string) bool {
	return isSevereMediaFailureReason(reason)
}

func isBenignGenericDownloadFailureReason(reason string) bool {
	reason = strings.ToLower(strings.TrimSpace(reason))
	if reason == "" {
		return false
	}
	return strings.Contains(reason, "context canceled") || strings.Contains(reason, " canceled")
}

func (c *genericDownloadController) observeTargetResult(target string, err error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return
	}
	key := genericDownloadTargetStateKey(target)
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.pruneIdleStatesLocked(now)
	state := c.ensureStateLocked(key, "", target, target, now)
	reason := classifyRecoverableRTPReadError(err)
	if reason == "" && err != nil {
		reason = strings.TrimSpace(err.Error())
	}
	if err == nil || isBenignGenericDownloadFailureReason(reason) {
		if state.ConsecutiveFailures > 0 {
			state.ConsecutiveFailures = 0
			state.BreakerOpenUntil = time.Time{}
			state.LastFailureReason = ""
			if err == nil {
				log.Printf("mapping-runtime stage=generic_download_circuit_close target=%s", target)
			} else {
				log.Printf("mapping-runtime stage=generic_download_circuit_ignore target=%s reason=%s", target, firstNonEmpty(reason, "unknown"))
			}
		}
		c.deleteIfIdleAndInactiveLocked(key, state)
		return
	}
	state.ConsecutiveFailures++
	state.LastFailureReason = reason
	threshold := genericDownloadCircuitFailureThreshold()
	if isSevereGenericDownloadFailureReason(reason) {
		// RTP 顺序断裂/长期 pending gap timeout 已经不是“轻微抖动”，
		// 继续按普通失败等 3 次再开闸，会让后续下载继续维持多 segment 并发，把问题放大。
		threshold = 1
	}
	if threshold <= 0 {
		threshold = 1
	}
	if state.ConsecutiveFailures >= threshold {
		state.BreakerOpenUntil = now.Add(genericDownloadCircuitOpen())
		log.Printf("mapping-runtime stage=generic_download_circuit_open target=%s failures=%d open_ms=%d reason=%s", target, state.ConsecutiveFailures, genericDownloadCircuitOpen().Milliseconds(), firstNonEmpty(reason, "unknown"))
	}
}

type genericDownloadLease struct {
	key                      string
	deviceID                 string
	target                   string
	transferID               string
	transferIDSource         string
	activeTransfersGlobal    int
	activeTransfersPerDevice int
	activeSegmentsGlobal     int
	activeSegmentsPerDevice  int
	activeSegmentsTransfer   int
	effectiveTransferBPS     int64
	effectiveBPS             int64
	breakerOpen              bool
	floorApplied             bool
	sourceConstrained        bool
	sameTransferSplitEnabled bool
	sameTransferSplitApplied bool
}

var globalGenericDownloadController = &genericDownloadController{
	activeTransfersPerDevice: make(map[string]int),
	activeSegmentsPerDevice:  make(map[string]int),
	states:                   make(map[string]*genericDownloadState),
}

type genericDownloadCtxKey string

const (
	genericDownloadDeviceKey         genericDownloadCtxKey = "generic_download_device"
	genericDownloadTargetKey         genericDownloadCtxKey = "generic_download_target"
	genericDownloadTransferKey       genericDownloadCtxKey = "generic_download_transfer"
	genericDownloadTransferSourceKey genericDownloadCtxKey = "generic_download_transfer_source"
)

func normalizeGenericDownloadTransferInfo(transferID, target string) (string, string) {
	transferID = strings.TrimSpace(transferID)
	if transferID != "" {
		return transferID, "download_header"
	}
	return strings.TrimSpace(target), "target_fallback"
}

func withGenericDownloadContext(ctx context.Context, deviceID, target, transferID string) context.Context {
	normalizedTransferID, transferSource := normalizeGenericDownloadTransferInfo(transferID, target)
	ctx = context.WithValue(ctx, genericDownloadDeviceKey, strings.TrimSpace(deviceID))
	ctx = context.WithValue(ctx, genericDownloadTargetKey, strings.TrimSpace(target))
	ctx = context.WithValue(ctx, genericDownloadTransferKey, normalizedTransferID)
	ctx = context.WithValue(ctx, genericDownloadTransferSourceKey, transferSource)
	return ctx
}

func genericDownloadContextInfo(ctx context.Context) (string, string, string, string) {
	if ctx == nil {
		return "", "", "", ""
	}
	deviceID, _ := ctx.Value(genericDownloadDeviceKey).(string)
	target, _ := ctx.Value(genericDownloadTargetKey).(string)
	transferID, _ := ctx.Value(genericDownloadTransferKey).(string)
	transferSource, _ := ctx.Value(genericDownloadTransferSourceKey).(string)
	if strings.TrimSpace(transferSource) == "" {
		_, transferSource = normalizeGenericDownloadTransferInfo(transferID, target)
	}
	return strings.TrimSpace(deviceID), strings.TrimSpace(target), strings.TrimSpace(transferID), strings.TrimSpace(transferSource)
}

func normalizeGenericDownloadTransferID(transferID, target string) string {
	normalized, _ := normalizeGenericDownloadTransferInfo(transferID, target)
	return normalized
}

func genericDownloadStateKey(deviceID, target, transferID string) string {
	return strings.ToLower(strings.TrimSpace(deviceID)) + "|" + strings.ToLower(strings.TrimSpace(target)) + "|" + strings.ToLower(normalizeGenericDownloadTransferID(transferID, target))
}

func (c *genericDownloadController) ensureStateLocked(key, deviceID, target, transferID string, now time.Time) *genericDownloadState {
	state := c.states[key]
	if state == nil {
		state = &genericDownloadState{
			DeviceID:   strings.TrimSpace(deviceID),
			Target:     strings.TrimSpace(target),
			TransferID: normalizeGenericDownloadTransferID(transferID, target),
		}
		c.states[key] = state
	}
	state.LastTouchedAt = now
	return state
}

func (c *genericDownloadController) deleteIfIdleAndInactiveLocked(key string, state *genericDownloadState) {
	if state == nil {
		return
	}
	if state.ActiveSegmentsTransfer > 0 {
		return
	}
	if !state.BreakerOpenUntil.IsZero() && time.Now().Before(state.BreakerOpenUntil) {
		return
	}
	if !state.SourceConstrainedUntil.IsZero() && time.Now().Before(state.SourceConstrainedUntil) {
		return
	}
	if state.ConsecutiveFailures > 0 {
		return
	}
	delete(c.states, key)
}

func (c *genericDownloadController) pruneIdleStatesLocked(now time.Time) {
	if !c.lastPruneAt.IsZero() && now.Sub(c.lastPruneAt) < genericDownloadStatePruneInterval {
		return
	}
	for key, state := range c.states {
		if state == nil {
			delete(c.states, key)
			continue
		}
		if state.ActiveSegmentsTransfer > 0 {
			continue
		}
		if !state.BreakerOpenUntil.IsZero() && now.Before(state.BreakerOpenUntil) {
			continue
		}
		if !state.SourceConstrainedUntil.IsZero() && now.Before(state.SourceConstrainedUntil) {
			continue
		}
		delete(c.states, key)
	}
	c.lastPruneAt = now
}

func (c *genericDownloadController) targetBreakerOpenLocked(target string, now time.Time) bool {
	key := genericDownloadTargetStateKey(target)
	state := c.states[key]
	if state == nil {
		return false
	}
	state.LastTouchedAt = now
	if state.BreakerOpenUntil.IsZero() {
		c.deleteIfIdleAndInactiveLocked(key, state)
		return false
	}
	if now.After(state.BreakerOpenUntil) {
		state.BreakerOpenUntil = time.Time{}
		state.ConsecutiveFailures = 0
		state.LastFailureReason = ""
		c.deleteIfIdleAndInactiveLocked(key, state)
		return false
	}
	return true
}

func (c *genericDownloadController) sourceConstrainedLocked(target string, now time.Time) bool {
	key := genericDownloadTargetStateKey(target)
	state := c.states[key]
	if state == nil {
		return false
	}
	state.LastTouchedAt = now
	if state.SourceConstrainedUntil.IsZero() {
		return false
	}
	if now.After(state.SourceConstrainedUntil) {
		state.SourceConstrainedUntil = time.Time{}
		state.SourceConstrainedHits = 0
		return false
	}
	return true
}

func (c *genericDownloadController) observeSourceRead(target, transferID string, bodyBitrateBPS, effectiveSegmentBPS int64, activeSegmentsTransfer int) {
	target = strings.TrimSpace(target)
	if target == "" || bodyBitrateBPS <= 0 {
		return
	}
	if !genericDownloadSourceConstrainedAutoSingleflightEnabled() {
		return
	}
	transferID, _ = normalizeGenericDownloadTransferInfo(transferID, target)
	key := genericDownloadTargetStateKey(target)
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	c.pruneIdleStatesLocked(now)
	state := c.ensureStateLocked(key, "", target, transferID, now)
	state.LastObservedBodyBitrate = bodyBitrateBPS
	state.LastObservedEffectiveBPS = effectiveSegmentBPS
	state.LastObservedSegments = activeSegmentsTransfer
	if activeSegmentsTransfer > 1 && effectiveSegmentBPS > 0 && bodyBitrateBPS*100 <= effectiveSegmentBPS*genericDownloadSourceConstraintOpenRatioPct {
		state.SourceConstrainedHits++
		if state.SourceConstrainedHits >= 2 {
			state.SourceConstrainedUntil = now.Add(genericDownloadSourceConstraintHold)
			log.Printf("mapping-runtime stage=generic_download_source_constraint_open target=%s transfer_id=%s observed_bitrate_bps=%d effective_segment_bitrate_bps=%d active_segments_transfer=%d hold_ms=%d", target, transferID, bodyBitrateBPS, effectiveSegmentBPS, activeSegmentsTransfer, genericDownloadSourceConstraintHold.Milliseconds())
		}
		return
	}
	if activeSegmentsTransfer <= 1 && !state.SourceConstrainedUntil.IsZero() && bodyBitrateBPS >= genericDownloadSourceConstraintRecoverFloorBPS {
		state.SourceConstrainedUntil = time.Time{}
		state.SourceConstrainedHits = 0
		log.Printf("mapping-runtime stage=generic_download_source_constraint_close target=%s transfer_id=%s observed_bitrate_bps=%d recover_floor_bps=%d", target, transferID, bodyBitrateBPS, genericDownloadSourceConstraintRecoverFloorBPS)
		return
	}
	if activeSegmentsTransfer <= 1 && state.SourceConstrainedHits > 0 {
		state.SourceConstrainedHits = 0
	}
}

func (c *genericDownloadController) sourceConstrainedForTarget(target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	c.pruneIdleStatesLocked(now)
	return c.sourceConstrainedLocked(target, now)
}

func (c *genericDownloadController) acquire(deviceID, target, transferID string) genericDownloadLease {
	deviceID = strings.TrimSpace(deviceID)
	target = strings.TrimSpace(target)
	transferID, transferSource := normalizeGenericDownloadTransferInfo(transferID, target)
	key := genericDownloadStateKey(deviceID, target, transferID)
	deviceKey := strings.ToLower(deviceID)

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.pruneIdleStatesLocked(now)
	state := c.ensureStateLocked(key, deviceID, target, transferID, now)

	if state.ActiveSegmentsTransfer == 0 {
		c.activeTransfersGlobal++
		if deviceKey != "" {
			c.activeTransfersPerDevice[deviceKey]++
		}
	}
	c.activeSegmentsGlobal++
	if deviceKey != "" {
		c.activeSegmentsPerDevice[deviceKey]++
	}
	state.ActiveSegmentsTransfer++
	state.ActiveTransfersGlobal = c.activeTransfersGlobal
	state.ActiveTransfersPerDevice = c.activeTransfersPerDevice[deviceKey]
	state.ActiveSegmentsGlobal = c.activeSegmentsGlobal
	state.ActiveSegmentsPerDevice = c.activeSegmentsPerDevice[deviceKey]

	breakerOpen := now.Before(state.BreakerOpenUntil) || c.targetBreakerOpenLocked(target, now)
	sourceConstrained := c.sourceConstrainedLocked(target, now)
	effectiveTransfer := genericDownloadRTPBitrate()
	total := genericDownloadTotalBitrate()
	if total > 0 && c.activeTransfersGlobal > 0 {
		share := total / int64(c.activeTransfersGlobal)
		if share > 0 && share < effectiveTransfer {
			effectiveTransfer = share
		}
	}
	floorApplied := false
	sameTransferSplitEnabled := genericDownloadSameTransferSplitEnabled()
	sameTransferSplitApplied := false
	if minBPS := genericDownloadMinPerTransferBitrate(); minBPS > 0 {
		// 最低保底只在“总预算足以覆盖所有活跃下载”时生效，避免 min floor 把总带宽打穿。
		if total <= 0 || total >= minBPS*int64(c.activeTransfersGlobal) {
			if effectiveTransfer < minBPS {
				effectiveTransfer = minBPS
				floorApplied = true
			}
		}
	}
	if breakerOpen && effectiveTransfer > 0 {
		half := effectiveTransfer / 2
		if half > 0 && half < effectiveTransfer {
			effectiveTransfer = half
		}
	}
	effectiveSegment := effectiveTransfer
	if sameTransferSplitEnabled && state.ActiveSegmentsTransfer > 1 && effectiveTransfer > 0 {
		share := effectiveTransfer / int64(state.ActiveSegmentsTransfer)
		if share > 0 {
			effectiveSegment = share
			sameTransferSplitApplied = true
		}
	}
	if effectiveSegment <= 0 {
		effectiveSegment = effectiveTransfer
	}
	log.Printf("mapping-runtime stage=generic_download_rate_limit device_id=%s target=%s transfer_id=%s transfer_id_source=%s active_transfers_global=%d active_transfers_device=%d active_segments_global=%d active_segments_device=%d active_segments_transfer=%d effective_transfer_bitrate_bps=%d effective_bitrate_bps=%d floor_applied=%t breaker_open=%t source_constrained=%t same_transfer_split_enabled=%t same_transfer_split_applied=%t", deviceID, target, transferID, transferSource, c.activeTransfersGlobal, c.activeTransfersPerDevice[deviceKey], c.activeSegmentsGlobal, c.activeSegmentsPerDevice[deviceKey], state.ActiveSegmentsTransfer, effectiveTransfer, effectiveSegment, floorApplied, breakerOpen, sourceConstrained, sameTransferSplitEnabled, sameTransferSplitApplied)
	return genericDownloadLease{
		key:                      key,
		deviceID:                 deviceID,
		target:                   target,
		transferID:               transferID,
		transferIDSource:         transferSource,
		activeTransfersGlobal:    c.activeTransfersGlobal,
		activeTransfersPerDevice: c.activeTransfersPerDevice[deviceKey],
		activeSegmentsGlobal:     c.activeSegmentsGlobal,
		activeSegmentsPerDevice:  c.activeSegmentsPerDevice[deviceKey],
		activeSegmentsTransfer:   state.ActiveSegmentsTransfer,
		effectiveTransferBPS:     effectiveTransfer,
		effectiveBPS:             effectiveSegment,
		breakerOpen:              breakerOpen,
		floorApplied:             floorApplied,
		sourceConstrained:        sourceConstrained,
		sameTransferSplitEnabled: sameTransferSplitEnabled,
		sameTransferSplitApplied: sameTransferSplitApplied,
	}
}

func (c *genericDownloadController) release(lease genericDownloadLease, err error) {
	deviceKey := strings.ToLower(strings.TrimSpace(lease.deviceID))
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.pruneIdleStatesLocked(now)
	if c.activeSegmentsGlobal > 0 {
		c.activeSegmentsGlobal--
	}
	if deviceKey != "" && c.activeSegmentsPerDevice[deviceKey] > 0 {
		c.activeSegmentsPerDevice[deviceKey]--
	}
	state := c.states[lease.key]
	if state == nil {
		return
	}
	state.LastTouchedAt = now
	if state.ActiveSegmentsTransfer > 0 {
		state.ActiveSegmentsTransfer--
	}
	if state.ActiveSegmentsTransfer == 0 {
		if c.activeTransfersGlobal > 0 {
			c.activeTransfersGlobal--
		}
		if deviceKey != "" && c.activeTransfersPerDevice[deviceKey] > 0 {
			c.activeTransfersPerDevice[deviceKey]--
		}
	}
	state.ActiveTransfersGlobal = c.activeTransfersGlobal
	state.ActiveTransfersPerDevice = c.activeTransfersPerDevice[deviceKey]
	state.ActiveSegmentsGlobal = c.activeSegmentsGlobal
	state.ActiveSegmentsPerDevice = c.activeSegmentsPerDevice[deviceKey]

	reason := classifyRecoverableRTPReadError(err)
	if reason == "" && err != nil {
		reason = strings.TrimSpace(err.Error())
	}
	if err == nil {
		// 仅在一个外层下载事务的最后一个 segment 收尾时清理熔断状态，避免中间成功段提前误清空失败历史。
		if state.ActiveSegmentsTransfer == 0 && state.ConsecutiveFailures > 0 {
			state.ConsecutiveFailures = 0
			state.BreakerOpenUntil = time.Time{}
			state.LastFailureReason = ""
			log.Printf("mapping-runtime stage=generic_download_circuit_close device_id=%s target=%s transfer_id=%s", lease.deviceID, lease.target, lease.transferID)
		}
		c.deleteIfIdleAndInactiveLocked(lease.key, state)
		return
	}
	state.ConsecutiveFailures++
	state.LastFailureReason = reason
	threshold := genericDownloadCircuitFailureThreshold()
	if isSevereGenericDownloadFailureReason(reason) {
		// 发送侧如果收到明确的 RTP 严重拥塞信号，也要立刻进入 breaker open，
		// 让后续事务尽快退到更保守的发送份额。
		threshold = 1
	}
	if threshold <= 0 {
		threshold = 1
	}
	if state.ConsecutiveFailures >= threshold {
		state.BreakerOpenUntil = now.Add(genericDownloadCircuitOpen())
		log.Printf("mapping-runtime stage=generic_download_circuit_open device_id=%s target=%s transfer_id=%s failures=%d open_ms=%d reason=%s", lease.deviceID, lease.target, lease.transferID, state.ConsecutiveFailures, genericDownloadCircuitOpen().Milliseconds(), firstNonEmpty(reason, "unknown"))
		return
	}
	if state.ActiveSegmentsTransfer == 0 {
		delete(c.states, lease.key)
	}
}

func (c *genericDownloadController) breakerOpenForTarget(target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.pruneIdleStatesLocked(now)
	return c.targetBreakerOpenLocked(target, now)
}

func (c *genericDownloadController) breakerOpen(deviceID, target, transferID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.pruneIdleStatesLocked(now)
	st := c.states[genericDownloadStateKey(deviceID, target, transferID)]
	if st == nil {
		return c.targetBreakerOpenLocked(target, now)
	}
	st.LastTouchedAt = now
	if now.After(st.BreakerOpenUntil) {
		st.BreakerOpenUntil = time.Time{}
		st.ConsecutiveFailures = 0
		st.LastFailureReason = ""
		c.deleteIfIdleAndInactiveLocked(genericDownloadStateKey(deviceID, target, transferID), st)
		return c.targetBreakerOpenLocked(target, now)
	}
	return !st.BreakerOpenUntil.IsZero() || c.targetBreakerOpenLocked(target, now)
}
