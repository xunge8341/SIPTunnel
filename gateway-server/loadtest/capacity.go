package loadtest

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
)

type CapacityCurrentConfig struct {
	CommandMaxConcurrent      int `json:"command_max_concurrent"`
	FileTransferMaxConcurrent int `json:"file_transfer_max_concurrent"`
	RTPPortPoolSize           int `json:"rtp_port_pool_size"`
	MaxConnections            int `json:"max_connections"`
	RateLimitRPS              int `json:"rate_limit_rps"`
	RateLimitBurst            int `json:"rate_limit_burst"`
}

type CapacityRecommendation struct {
	RecommendedCommandMaxConcurrent      int      `json:"recommended_command_max_concurrent"`
	RecommendedFileTransferMaxConcurrent int      `json:"recommended_file_transfer_max_concurrent"`
	RecommendedRTPPortPoolSize           int      `json:"recommended_rtp_port_pool_size"`
	RecommendedMaxConnections            int      `json:"recommended_max_connections"`
	RecommendedRateLimitRPS              int      `json:"recommended_rate_limit_rps"`
	RecommendedRateLimitBurst            int      `json:"recommended_rate_limit_burst"`
	Basis                                []string `json:"basis"`
}

type CapacityAssessment struct {
	Current        CapacityCurrentConfig  `json:"current"`
	Recommendation CapacityRecommendation `json:"recommendation"`
}

func LoadReportFromSummary(path string) (Report, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Report{}, fmt.Errorf("read summary file: %w", err)
	}
	var report Report
	if err := json.Unmarshal(raw, &report); err != nil {
		return Report{}, fmt.Errorf("unmarshal summary file: %w", err)
	}
	return report, nil
}

func AssessCapacity(report Report, current CapacityCurrentConfig) CapacityAssessment {
	cmdSummary := bestSummary(report, []string{"sip-command-create", "sip-status-receipt", "http-invoke"})
	fileSummary := bestSummary(report, []string{"rtp-udp-upload", "rtp-tcp-upload"})

	recCmd := recommendConcurrency(current.CommandMaxConcurrent, cmdSummary, 200)
	recFile := recommendConcurrency(current.FileTransferMaxConcurrent, fileSummary, 300)
	if recCmd <= 0 {
		recCmd = max(1, current.CommandMaxConcurrent)
	}
	if recFile <= 0 {
		recFile = max(1, current.FileTransferMaxConcurrent)
	}

	targetRPS := int(math.Floor((safeThroughput(cmdSummary)+safeThroughput(fileSummary))*0.85 + 0.5))
	if targetRPS <= 0 {
		targetRPS = max(50, current.RateLimitRPS)
	}
	burst := int(math.Ceil(float64(targetRPS) * 1.5))
	if burst < targetRPS {
		burst = targetRPS
	}

	rtpPool := int(math.Ceil(float64(recFile)*2.0*1.3 + 0.5))
	if rtpPool < 64 {
		rtpPool = 64
	}
	maxConn := int(math.Ceil(float64(recCmd+recFile) * 1.2))
	if maxConn < 32 {
		maxConn = 32
	}

	basis := []string{
		"启发式规则：仅采纳成功率>=99.5%、P95<=阈值(命令200ms/文件300ms)的压测结果。",
		"并发推荐=当前压测并发 * 质量因子(成功率与P95衰减) * 0.85安全系数。",
		"RTP端口池按每个文件传输2个端口并预留30%缓冲计算，且不低于64。",
		"max_connections 按推荐命令并发+文件并发后再加20%连接余量。",
		"限流阈值 RPS 取命令+文件吞吐总和的85%，burst 取1.5倍RPS。",
	}

	return CapacityAssessment{
		Current: current,
		Recommendation: CapacityRecommendation{
			RecommendedCommandMaxConcurrent:      recCmd,
			RecommendedFileTransferMaxConcurrent: recFile,
			RecommendedRTPPortPoolSize:           rtpPool,
			RecommendedMaxConnections:            maxConn,
			RecommendedRateLimitRPS:              targetRPS,
			RecommendedRateLimitBurst:            burst,
			Basis:                                basis,
		},
	}
}

func recommendConcurrency(current int, summary Summary, p95LimitMS float64) int {
	if summary.Total == 0 {
		return current
	}
	quality := math.Min(1, summary.SuccessRate/0.995)
	if summary.P95MS > 0 {
		quality *= math.Min(1, p95LimitMS/summary.P95MS)
	}
	rec := int(math.Floor(float64(summary.Concurrency) * quality * 0.85))
	if rec <= 0 {
		return max(1, current)
	}
	return rec
}

func safeThroughput(s Summary) float64 {
	if s.SuccessRate < 0.995 {
		return s.Throughput * 0.6
	}
	return s.Throughput
}

func bestSummary(report Report, keys []string) Summary {
	best := Summary{}
	for _, k := range keys {
		s, ok := report.Summaries[k]
		if !ok {
			continue
		}
		if s.Throughput > best.Throughput {
			best = s
		}
	}
	return best
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
