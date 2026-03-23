package server

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"siptunnel/internal/config"
	"siptunnel/internal/selfcheck"
)

func clampPercent(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return math.Round(v*10) / 10
}

func percentOf(used, total int) float64 {
	if total <= 0 {
		return 0
	}
	return clampPercent(float64(used) * 100 / float64(total))
}

func percentOfUint64(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return clampPercent(float64(used) * 100 / float64(total))
}

func scaleInt(base int, factor float64, min int) int {
	if base <= 0 {
		return min
	}
	v := int(math.Round(float64(base) * factor))
	if v < min {
		return min
	}
	return v
}

func selfcheckOverallText(level selfcheck.Level) string {
	switch level {
	case selfcheck.LevelError:
		return "red"
	case selfcheck.LevelWarn:
		return "yellow"
	default:
		return "green"
	}
}

func applyTransportTuningToUsage(usage *systemResourceUsage, tuning config.TransportTuningConfig) {
	if usage == nil {
		return
	}
	converged := config.ConvergedGenericDownloadProfile(tuning)
	usage.ConfiguredGenericDownloadMbps = bpsToMbps(tuning.GenericDownloadTotalBitrateBps)
	usage.ConfiguredGenericPerTransferMbps = bpsToMbps(tuning.GenericDownloadMinPerTransferBitrateBps)
	usage.ConfiguredAdaptiveHotCacheMB = bytesToMB(tuning.AdaptivePlaybackSegmentCacheBytes)
	usage.ConfiguredAdaptiveHotWindowMB = bytesToMB(tuning.AdaptivePlaybackHotWindowBytes)
	usage.ConfiguredGenericDownloadWindowMB = bytesToMB(tuning.GenericDownloadWindowBytes)
	usage.ConfiguredGenericSegmentConcurrency = tuning.GenericDownloadSegmentConcurrency
	usage.ConfiguredGenericRTPReorderWindow = converged.ReorderWindowPackets
	usage.ConfiguredGenericRTPLossTolerance = converged.LossTolerancePackets
	usage.ConfiguredGenericRTPGapTimeoutMS = converged.GapTimeoutMS
	usage.ConfiguredGenericRTPFECEnabled = converged.FECEnabled
	usage.ConfiguredGenericRTPFECGroupPackets = converged.FECGroupPackets
}

func applySystemResourceAssessment(d *handlerDeps, usage *systemResourceUsage, status NodeNetworkStatus) {
	if usage == nil {
		return
	}

	usage.RTPPortPoolUsagePercent = percentOf(usage.RTPPortPoolUsed, usage.RTPPortPoolTotal)
	usage.HeapUsagePercent = percentOfUint64(usage.HeapAllocBytes, usage.HeapSysBytes)

	baselineLimits := normalizeOpsLimits(defaultOpsLimits())
	baselineTuning := currentTransportTuning()
	if d != nil {
		baselineLimits = d.manualBaselineLimits()
		baselineTuning = d.manualBaselineTransportTuning()
	}
	baseMaxConcurrent := baselineLimits.MaxConcurrent
	baseRPS := baselineLimits.RPS
	baseBurst := baselineLimits.Burst
	usage.ActiveRequestUsagePercent = percentOf(usage.ActiveRequests, baseMaxConcurrent)

	selfcheckOverall := "green"
	selfcheckSummary := "启动自检正常"
	if d != nil && d.selfCheckProvider != nil {
		report := d.selfCheckProvider(context.Background())
		selfcheckOverall = selfcheckOverallText(report.Overall)
		switch report.Overall {
		case selfcheck.LevelError:
			selfcheckSummary = "启动自检存在错误项"
		case selfcheck.LevelWarn:
			selfcheckSummary = "启动自检存在告警项"
		default:
			selfcheckSummary = "启动自检正常"
		}
	}
	usage.SelfcheckOverall = selfcheckSummary

	quality := snapshotRuntimeQuality(timeNowUTC())
	usage.ObservedJitterLossEvents = quality.RecentJitterLossEvents
	usage.ObservedGapTimeouts = quality.RecentGapTimeouts
	usage.ObservedFECRecovered = quality.RecentFECRecovered
	usage.ObservedPeakPending = quality.RecentPeakPending
	usage.ObservedMaxGapHoldMS = quality.RecentMaxGapHoldMS
	usage.ObservedWriterBlockMS = quality.RecentWriterBlockMS
	usage.ObservedMaxWriterBlockMS = quality.RecentMaxWriterBlockMS
	usage.ObservedContextCanceled = quality.RecentContextCanceled
	usage.ObservedCircuitOpenCount = quality.CircuitOpenCount
	usage.ObservedCircuitHalfOpenCount = quality.CircuitHalfOpenCount

	reasons := []string{}
	statusColor := "green"
	statusSummary := "资源正常"
	factor := 1.0

	applyYellow := func(reason string, newFactor float64) {
		if strings.TrimSpace(reason) != "" {
			reasons = append(reasons, reason)
		}
		if statusColor == "red" {
			return
		}
		statusColor = "yellow"
		statusSummary = "降级运行"
		factor = math.Min(factor, newFactor)
	}
	applyRed := func(reason string, newFactor float64) {
		if strings.TrimSpace(reason) != "" {
			reasons = append(reasons, reason)
		}
		statusColor = "red"
		statusSummary = "不建议继续"
		factor = math.Min(factor, newFactor)
	}

	if selfcheckOverall == "red" {
		applyRed("启动自检存在错误项", 0.60)
	} else if selfcheckOverall == "yellow" {
		applyYellow("启动自检存在告警项", 0.85)
	}
	if usage.RTPPortPoolUsagePercent >= 90 {
		applyRed("RTP 端口池接近耗尽", 0.60)
	} else if usage.RTPPortPoolUsagePercent >= 75 {
		applyYellow("RTP 端口池占用偏高", 0.85)
	}
	if usage.ActiveRequestUsagePercent >= 90 {
		applyRed("活跃请求已逼近并发上限", 0.60)
	} else if usage.ActiveRequestUsagePercent >= 75 {
		applyYellow("活跃请求占用偏高", 0.85)
	}
	if usage.HeapUsagePercent >= 92 {
		applyRed("堆内存占用接近上限", 0.60)
	} else if usage.HeapUsagePercent >= 80 {
		applyYellow("堆内存占用偏高", 0.85)
	}
	if len(status.RecentBindErrors) > 0 {
		applyYellow("最近存在端口/监听绑定异常", 0.85)
	}
	if len(status.RecentNetworkErrors) > 0 {
		applyYellow("最近存在网络侧错误或回源异常", 0.85)
	}
	if status.RTP.PortAllocFailTotal >= 10 {
		applyYellow(fmt.Sprintf("RTP 端口申请失败累计偏高：%d", status.RTP.PortAllocFailTotal), 0.85)
	}
	if quality.CircuitOpenCount > 0 {
		applyRed("上游熔断处于打开状态", 0.60)
	} else if quality.CircuitHalfOpenCount > 0 {
		applyYellow("上游熔断处于半开恢复观察", 0.80)
	}
	if quality.RecentGapTimeouts >= 3 || quality.RecentPeakPending >= 1024 || quality.RecentMaxGapHoldMS >= 3000 {
		applyRed("最近 RTP jitter/loss/pending 压力过高", 0.60)
	} else if quality.RecentJitterLossEvents > 0 || quality.RecentPeakPending >= 256 || quality.RecentMaxGapHoldMS >= 1200 {
		applyYellow("最近 RTP jitter/loss/pending 存在波动", 0.85)
	}
	if quality.RecentMaxWriterBlockMS >= 3000 || quality.RecentWriterBlockMS >= 1500 {
		applyRed("最近 writer block 明显，发送侧排空受阻", 0.60)
	} else if quality.RecentMaxWriterBlockMS >= 1200 || quality.RecentWriterBlockMS >= 600 {
		applyYellow("最近 writer block 偏高", 0.85)
	}
	if quality.RecentContextCanceled >= 10 {
		applyYellow(fmt.Sprintf("最近 context canceled 偏多：%d", quality.RecentContextCanceled), 0.85)
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "CPU、内存、RTP 端口池、并发占用与传输观测均在稳定区间")
	}
	usage.StatusColor = statusColor
	usage.StatusSummary = statusSummary
	usage.StatusReasons = reasons

	recommendedProfile := "平衡模式"
	baseTransportMbps := bpsToMbps(baselineTuning.GenericDownloadTotalBitrateBps)
	if statusColor == "red" {
		recommendedProfile = "稳态模式"
	} else if statusColor == "green" && usage.CPUCores >= 8 && baseTransportMbps >= 24 && usage.RTPPortPoolUsagePercent < 50 && usage.ActiveRequestUsagePercent < 50 && quality.CircuitOpenCount == 0 && quality.RecentMaxWriterBlockMS < 600 && quality.RecentPeakPending < 256 {
		recommendedProfile = "高吞吐模式"
	}
	usage.RecommendedProfile = recommendedProfile

	usage.TheoreticalRTPTransferLimit = maxIntVal(1, int(math.Floor(float64(maxIntVal(1, usage.RTPPortPoolTotal))/2)))
	if usage.RTPPortPoolUsagePercent > 0 {
		sparePorts := maxIntVal(0, usage.RTPPortPoolTotal-usage.RTPPortPoolUsed)
		usage.TheoreticalRTPTransferLimit = maxIntVal(1, int(math.Floor(float64(sparePorts)/2)))
	}

	baseFileConcurrent := baselineTuning.GenericDownloadSegmentConcurrency
	if baseFileConcurrent <= 0 {
		baseFileConcurrent = 4
	}
	if baseMaxConcurrent <= 0 {
		baseMaxConcurrent = maxIntVal(baseFileConcurrent*2, 8)
	}
	if baseRPS <= 0 {
		baseRPS = 60
	}
	if baseBurst <= 0 {
		baseBurst = maxIntVal(10, baseRPS/2)
	}

	usage.RecommendedFileTransferMaxConcurrent = scaleInt(baseFileConcurrent, factor, 1)
	usage.RecommendedMaxConcurrent = scaleInt(baseMaxConcurrent, factor, 2)
	usage.RecommendedRateLimitRPS = scaleInt(baseRPS, factor, 1)
	usage.RecommendedRateLimitBurst = scaleInt(baseBurst, factor, 1)

	suitableScenarios := []string{"视频播放", "大文件下载", "播放+下载混合"}
	if statusColor == "red" {
		suitableScenarios = []string{"单路播放", "单路下载", "低并发排障"}
	} else if statusColor == "yellow" {
		suitableScenarios = []string{"低并发播放", "低并发下载", "受控混合场景"}
	}
	usage.SuitableScenarios = suitableScenarios

	actions := []string{}
	switch statusColor {
	case "red":
		actions = append(actions, "切回稳态模式并立即写回运行策略")
		actions = append(actions, "降低下载与播放并发，优先排查端口池/内存/下游可达性")
	case "yellow":
		actions = append(actions, "保持平衡模式，限制热点来源或热点资源 10~30 分钟")
		actions = append(actions, "重点观察 jitter/loss/pending、writer block 与 circuit 恢复")
	default:
		actions = append(actions, "可按推荐档位继续压测，并放量观察")
		actions = append(actions, "继续观察热点来源、热点资源与回源稳定性")
	}
	usage.SuggestedActions = actions
	usage.RecommendedSummary = fmt.Sprintf("建议按%s运行；建议总并发=%d，建议文件并发=%d，建议限流=%d/%d。", usage.RecommendedProfile, usage.RecommendedMaxConcurrent, usage.RecommendedFileTransferMaxConcurrent, usage.RecommendedRateLimitRPS, usage.RecommendedRateLimitBurst)

	state := autoRuntimeProfileState{}
	if d != nil {
		state = d.applyRecommendedRuntimeProfile(usage)
		applyTransportTuningToUsage(usage, currentTransportTuning())
	}
	usage.RuntimeProfileApplied = firstNonEmpty(state.AppliedProfile, usage.RecommendedProfile)
	usage.RuntimeProfileAppliedAt = state.AppliedAt
	usage.RuntimeProfileReason = firstNonEmpty(state.Reason, usage.RecommendedSummary)
	usage.RuntimeProfileChanged = state.Changed
}

var timeNowUTC = func() time.Time { return time.Now().UTC() }
