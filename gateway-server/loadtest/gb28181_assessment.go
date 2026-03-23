package loadtest

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// GB28181LogMetrics 汇总单份网关日志里与 GB28181 事务稳定性相关的关键指标。
// 它适合快速对单份日志做离线体检；更复杂的 Task 9/10 A/B 结论仍建议走 experiment manifest。
type GB28181LogMetrics struct {
	TransactionCount      int     `json:"transaction_count"`
	ResponseStartTimeouts int     `json:"response_start_timeouts"`
	ByeReceivedCount      int     `json:"bye_received_count"`
	GateWaitP95MS         float64 `json:"gate_wait_p95_ms"`
	GateWaitP99MS         float64 `json:"gate_wait_p99_ms"`
	ResponseStartP95MS    float64 `json:"response_start_p95_ms"`
	ResponseStartP99MS    float64 `json:"response_start_p99_ms"`
}

type GB28181ABComparison struct {
	Baseline  GB28181LogMetrics `json:"baseline"`
	Candidate GB28181LogMetrics `json:"candidate"`
	Delta     GB28181LogMetrics `json:"delta"`
}

func int64Percentile(values []int64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(float64(len(sorted)-1) * p)
	return float64(sorted[idx])
}

// AnalyzeGB28181GatewayLog 从网关日志中提取事务汇总、超时和 BYE 等指标，
// 用于专项压测后对边界口和响应模式策略做离线复核。
func AnalyzeGB28181GatewayLog(path string) (GB28181LogMetrics, error) {
	f, err := os.Open(path)
	if err != nil {
		return GB28181LogMetrics{}, fmt.Errorf("open log: %w", err)
	}
	defer f.Close()

	var m GB28181LogMetrics
	var gateWaits, responseWaits []int64
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		if strings.Contains(line, "stage=transaction_summary") {
			m.TransactionCount++
			for _, kv := range []struct {
				key string
				dst *[]int64
			}{{"gate_wait_ms=", &gateWaits}, {"response_start_wait_ms=", &responseWaits}} {
				if i := strings.Index(line, kv.key); i >= 0 {
					rest := line[i+len(kv.key):]
					end := strings.IndexByte(rest, ' ')
					if end < 0 {
						end = len(rest)
					}
					if v, err := strconv.ParseInt(rest[:end], 10, 64); err == nil {
						*kv.dst = append(*kv.dst, v)
					}
				}
			}
		}
		if strings.Contains(line, "stage=response_start_timeout") {
			m.ResponseStartTimeouts++
		}
		if strings.Contains(line, "stage=bye_received") || strings.Contains(line, "stage=bye_sent") {
			m.ByeReceivedCount++
		}
	}
	if err := s.Err(); err != nil {
		return GB28181LogMetrics{}, fmt.Errorf("scan log: %w", err)
	}
	m.GateWaitP95MS = int64Percentile(gateWaits, 0.95)
	m.GateWaitP99MS = int64Percentile(gateWaits, 0.99)
	m.ResponseStartP95MS = int64Percentile(responseWaits, 0.95)
	m.ResponseStartP99MS = int64Percentile(responseWaits, 0.99)
	return m, nil
}

func CompareGB28181Metrics(baseline, candidate GB28181LogMetrics) GB28181ABComparison {
	return GB28181ABComparison{
		Baseline:  baseline,
		Candidate: candidate,
		Delta: GB28181LogMetrics{
			TransactionCount:      candidate.TransactionCount - baseline.TransactionCount,
			ResponseStartTimeouts: candidate.ResponseStartTimeouts - baseline.ResponseStartTimeouts,
			ByeReceivedCount:      candidate.ByeReceivedCount - baseline.ByeReceivedCount,
			GateWaitP95MS:         candidate.GateWaitP95MS - baseline.GateWaitP95MS,
			GateWaitP99MS:         candidate.GateWaitP99MS - baseline.GateWaitP99MS,
			ResponseStartP95MS:    candidate.ResponseStartP95MS - baseline.ResponseStartP95MS,
			ResponseStartP99MS:    candidate.ResponseStartP99MS - baseline.ResponseStartP99MS,
		},
	}
}
