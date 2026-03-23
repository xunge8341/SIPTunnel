package server

import (
	"fmt"
	"math"
	"strings"
	"time"

	"siptunnel/internal/config"
)

type autoRuntimeProfileState struct {
	AppliedProfile string `json:"applied_profile,omitempty"`
	AppliedAt      string `json:"applied_at,omitempty"`
	Reason         string `json:"reason,omitempty"`
	Changed        bool   `json:"changed,omitempty"`
}

func (d *handlerDeps) manualBaselineTransportTuning() config.TransportTuningConfig {
	if d == nil {
		return currentTransportTuning()
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.baselineTransportTuning.GenericDownloadTotalBitrateBps > 0 {
		return d.baselineTransportTuning
	}
	base := config.DefaultTransportTuningConfig()
	if systemSettingsHasRuntimeProfile(d.systemSettings) {
		return applySystemSettingsRuntimeProfile(base, d.systemSettings)
	}
	return currentTransportTuning()
}

func (d *handlerDeps) manualBaselineLimits() OpsLimits {
	if d == nil {
		return normalizeOpsLimits(defaultOpsLimits())
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.baselineLimits.MaxConcurrent > 0 {
		return normalizeOpsLimits(d.baselineLimits)
	}
	return normalizeOpsLimits(d.limits)
}

func scaleIntUpper(base int, factor float64, minVal, maxVal int) int {
	if base <= 0 {
		base = minVal
	}
	v := int(math.Round(float64(base) * factor))
	if v < minVal {
		v = minVal
	}
	if maxVal > 0 && v > maxVal {
		v = maxVal
	}
	return v
}

func clampFloat(v, minVal, maxVal float64) float64 {
	if v < minVal {
		return minVal
	}
	if maxVal > 0 && v > maxVal {
		return maxVal
	}
	return v
}

func tuneTransportForProfile(base config.TransportTuningConfig, profile string, usage *systemResourceUsage) config.TransportTuningConfig {
	cfg := base
	switch strings.TrimSpace(profile) {
	case "稳态模式":
		cfg.GenericDownloadTotalBitrateBps = mbpsToBps(clampFloat(bpsToMbps(base.GenericDownloadTotalBitrateBps)*0.5, 8, 16))
		cfg.GenericDownloadMinPerTransferBitrateBps = mbpsToBps(clampFloat(bpsToMbps(base.GenericDownloadMinPerTransferBitrateBps)*0.5, 1, 2))
		cfg.GenericDownloadWindowBytes = mbToBytes(clampFloat(bytesToMB(base.GenericDownloadWindowBytes)*0.5, 1, 2))
		cfg.AdaptivePlaybackSegmentCacheBytes = mbToBytes(clampFloat(bytesToMB(base.AdaptivePlaybackSegmentCacheBytes), 128, 256))
		cfg.AdaptivePlaybackHotWindowBytes = mbToBytes(clampFloat(bytesToMB(base.AdaptivePlaybackHotWindowBytes), 4, 8))
		cfg.GenericDownloadSegmentConcurrency = 1
		cfg.GenericDownloadRTPReorderWindowPackets = 384
		cfg.GenericDownloadRTPLossTolerancePackets = 128
		cfg.GenericDownloadRTPGapTimeoutMS = 900
		cfg.GenericDownloadRTPFECEnabled = true
		cfg.GenericDownloadRTPFECGroupPackets = 8
	case "高吞吐模式":
		cfg.GenericDownloadTotalBitrateBps = mbpsToBps(clampFloat(bpsToMbps(base.GenericDownloadTotalBitrateBps)*1.5, 32, 64))
		cfg.GenericDownloadMinPerTransferBitrateBps = mbpsToBps(clampFloat(bpsToMbps(base.GenericDownloadMinPerTransferBitrateBps)*1.5, 2, 8))
		cfg.GenericDownloadWindowBytes = mbToBytes(clampFloat(bytesToMB(base.GenericDownloadWindowBytes)*2, 4, 16))
		cfg.AdaptivePlaybackSegmentCacheBytes = mbToBytes(clampFloat(bytesToMB(base.AdaptivePlaybackSegmentCacheBytes), 256, 512))
		cfg.AdaptivePlaybackHotWindowBytes = mbToBytes(clampFloat(bytesToMB(base.AdaptivePlaybackHotWindowBytes)*2, 4, 16))
		cfg.GenericDownloadSegmentConcurrency = maxIntVal(base.GenericDownloadSegmentConcurrency+1, 4)
		cfg.GenericDownloadRTPReorderWindowPackets = 768
		cfg.GenericDownloadRTPLossTolerancePackets = 256
		cfg.GenericDownloadRTPGapTimeoutMS = 1200
		cfg.GenericDownloadRTPFECEnabled = true
		cfg.GenericDownloadRTPFECGroupPackets = 8
	default:
		cfg = base
	}
	cfg.ApplyDefaultsForRuntime(config.DefaultTransportTuningConfig())
	if usage != nil {
		cfg.GenericDownloadSegmentConcurrency = maxIntVal(1, usage.RecommendedFileTransferMaxConcurrent)
	}
	return cfg
}

func limitsForProfile(base OpsLimits, usage *systemResourceUsage) OpsLimits {
	limits := normalizeOpsLimits(base)
	if usage == nil {
		return limits
	}
	if usage.RecommendedRateLimitRPS > 0 {
		limits.RPS = usage.RecommendedRateLimitRPS
	}
	if usage.RecommendedRateLimitBurst > 0 {
		limits.Burst = usage.RecommendedRateLimitBurst
	}
	if usage.RecommendedMaxConcurrent > 0 {
		limits.MaxConcurrent = usage.RecommendedMaxConcurrent
	}
	return normalizeOpsLimits(limits)
}

func transportConfigSummary(cfg config.TransportTuningConfig) string {
	conv := config.ConvergedGenericDownloadProfile(cfg)
	return fmt.Sprintf("download=%.1fMbps per=%.1fMbps window=%.1fMB seg=%d reorder=%d loss=%d gap=%dms fec=%t/%d", bpsToMbps(cfg.GenericDownloadTotalBitrateBps), bpsToMbps(cfg.GenericDownloadMinPerTransferBitrateBps), bytesToMB(cfg.GenericDownloadWindowBytes), cfg.GenericDownloadSegmentConcurrency, conv.ReorderWindowPackets, conv.LossTolerancePackets, conv.GapTimeoutMS, conv.FECEnabled, conv.FECGroupPackets)
}

func limitsSummary(l OpsLimits) string {
	l = normalizeOpsLimits(l)
	return fmt.Sprintf("RPS=%d Burst=%d MaxConcurrent=%d", l.RPS, l.Burst, l.MaxConcurrent)
}

func (d *handlerDeps) applyRecommendedRuntimeProfile(usage *systemResourceUsage) autoRuntimeProfileState {
	state := autoRuntimeProfileState{}
	if d == nil || usage == nil || strings.TrimSpace(usage.RecommendedProfile) == "" {
		return state
	}
	profile := strings.TrimSpace(usage.RecommendedProfile)
	baseTransport := d.manualBaselineTransportTuning()
	baseLimits := d.manualBaselineLimits()
	targetTransport := tuneTransportForProfile(baseTransport, profile, usage)
	targetLimits := limitsForProfile(baseLimits, usage)

	currentTransport := currentTransportTuning()
	currentLimits := normalizeOpsLimits(d.limits)
	transportChanged := transportConfigSummary(currentTransport) != transportConfigSummary(targetTransport)
	limitsChanged := currentLimits != normalizeOpsLimits(targetLimits)

	if transportChanged {
		ApplyTransportTuning(targetTransport)
	}
	if limitsChanged {
		d.mu.Lock()
		d.limits = normalizeOpsLimits(targetLimits)
		protector := d.protectionRuntime
		d.runtimeProfileState = autoRuntimeProfileState{AppliedProfile: profile, AppliedAt: formatTimestamp(time.Now().UTC()), Changed: true}
		d.mu.Unlock()
		if protector != nil {
			protector.UpdateLimits(targetLimits)
		}
	} else {
		d.mu.Lock()
		changed := transportChanged
		if d.runtimeProfileState.AppliedProfile != profile || transportChanged {
			changed = true
		}
		d.runtimeProfileState = autoRuntimeProfileState{AppliedProfile: profile, AppliedAt: formatTimestamp(time.Now().UTC()), Changed: changed}
		d.mu.Unlock()
	}
	reasonParts := []string{usage.StatusSummary}
	if len(usage.StatusReasons) > 0 {
		reasonParts = append(reasonParts, usage.StatusReasons...)
	}
	state = autoRuntimeProfileState{AppliedProfile: profile, AppliedAt: formatTimestamp(time.Now().UTC()), Reason: strings.Join(reasonParts, "；"), Changed: transportChanged || limitsChanged}
	state.Reason = strings.TrimSpace(state.Reason)
	d.mu.Lock()
	d.runtimeProfileState = state
	d.mu.Unlock()
	return state
}

func (d *handlerDeps) currentRuntimeProfileState() autoRuntimeProfileState {
	if d == nil {
		return autoRuntimeProfileState{}
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.runtimeProfileState
}
