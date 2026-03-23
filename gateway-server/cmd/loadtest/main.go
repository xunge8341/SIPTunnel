package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"siptunnel/loadtest"
)

func main() {
	var (
		targets           = flag.String("targets", "sip-command-create,sip-status-receipt,rtp-udp-upload,rtp-tcp-upload,http-invoke", "逗号分隔压测对象")
		concurrency       = flag.Int("concurrency", 10, "并发数")
		qps               = flag.Int("qps", 0, "全局QPS限制，0表示不限速")
		fileSize          = flag.Int("file-size", 1024*1024, "文件大小（字节）")
		chunkSize         = flag.Int("chunk-size", 64*1024, "RTP分片大小（字节）")
		transferMode      = flag.String("transfer-mode", "mixed", "传输模式：udp|tcp|mixed")
		rtpTransport      = flag.String("rtp-transport", "", "兼容别名：等同于 -transfer-mode=udp|tcp")
		duration          = flag.Duration("duration", 30*time.Second, "测试时长")
		sipAddress        = flag.String("sip-address", "127.0.0.1:5060", "SIP(TCP)地址")
		rtpAddress        = flag.String("rtp-address", "127.0.0.1:25000", "RTP地址（UDP/TCP复用）")
		httpURL           = flag.String("http-url", "http://127.0.0.1:18080/demo/process", "A网HTTP invoke URL")
		mappingURL        = flag.String("mapping-url", "", "隧道映射入口 URL，用于核心数据面压测")
		mappingMethod     = flag.String("mapping-method", "POST", "隧道映射压测 HTTP 方法")
		mappingBodySize   = flag.Int("mapping-body-size", 1024, "隧道映射压测请求体大小（字节）")
		allowProbePath    = flag.Bool("allow-probe-path", false, "允许把 /healthz /readyz /livez 等探针路径当作 mapping-forward 压测目标")
		strictRealMapping = flag.Bool("strict-real-mapping", false, "严格要求 mapping-forward 目标不能是探针路径；供真实映射链路压测脚本使用")
		outputDir         = flag.String("output-dir", "./loadtest/results", "结果输出目录")
		timeout           = flag.Duration("timeout", 3*time.Second, "单请求超时")
		gatewayBase       = flag.String("gateway-base-url", "", "网关管理面地址，用于采集运维诊断快照；为空时按 http-url 自动推导")
		diagInterval      = flag.Duration("diag-interval", 15*time.Second, "压测期间诊断采样间隔，0 表示仅采集首尾快照")
		analyzeFile       = flag.String("analyze-summary", "", "读取 summary.json 并输出容量评估建议")
		analyzeGBLog      = flag.String("analyze-gb28181-log", "", "读取网关日志并输出 GB28181 事务指标")
		compareGBLogA     = flag.String("compare-gb28181-log-a", "", "A/B 对比：基线网关日志")
		compareGBLogB     = flag.String("compare-gb28181-log-b", "", "A/B 对比：候选网关日志")
		analyzeExperiment = flag.String("analyze-experiment", "", "读取 experiment_manifest.json 并输出 Task 9/10 分析报告")
		experimentOutput  = flag.String("experiment-output", "", "Task 9/10 分析报告输出路径；为空时输出到 stdout")
		currentCmd        = flag.Int("current-command-max-concurrent", 100, "当前命令并发上限")
		currentFile       = flag.Int("current-file-max-concurrent", 60, "当前文件传输并发上限")
		currentRTP        = flag.Int("current-rtp-port-pool", 256, "当前RTP端口池大小")
		currentConn       = flag.Int("current-max-connections", 200, "当前 max_connections")
		currentRPS        = flag.Int("current-rate-limit-rps", 300, "当前限流RPS")
		currentBurst      = flag.Int("current-rate-limit-burst", 450, "当前限流突发值")
	)
	flag.Parse()

	if strings.TrimSpace(*analyzeGBLog) != "" {
		metrics, err := loadtest.AnalyzeGB28181GatewayLog(*analyzeGBLog)
		if err != nil {
			fmt.Fprintf(os.Stderr, "analyze gb28181 log failed: %v\n", err)
			os.Exit(1)
		}
		output, _ := json.MarshalIndent(metrics, "", "  ")
		fmt.Println(string(output))
		return
	}
	if strings.TrimSpace(*compareGBLogA) != "" && strings.TrimSpace(*compareGBLogB) != "" {
		baseline, err := loadtest.AnalyzeGB28181GatewayLog(*compareGBLogA)
		if err != nil {
			fmt.Fprintf(os.Stderr, "analyze baseline gb28181 log failed: %v\n", err)
			os.Exit(1)
		}
		candidate, err := loadtest.AnalyzeGB28181GatewayLog(*compareGBLogB)
		if err != nil {
			fmt.Fprintf(os.Stderr, "analyze candidate gb28181 log failed: %v\n", err)
			os.Exit(1)
		}
		output, _ := json.MarshalIndent(loadtest.CompareGB28181Metrics(baseline, candidate), "", "  ")
		fmt.Println(string(output))
		return
	}

	if strings.TrimSpace(*analyzeExperiment) != "" {
		analysis, err := loadtest.AnalyzeExperimentManifest(*analyzeExperiment)
		if err != nil {
			fmt.Fprintf(os.Stderr, "analyze experiment failed: %v\n", err)
			os.Exit(1)
		}
		markdown := loadtest.RenderExperimentMarkdown(analysis)
		if strings.TrimSpace(*experimentOutput) != "" {
			if err := loadtest.WriteExperimentMarkdown(*experimentOutput, analysis); err != nil {
				fmt.Fprintf(os.Stderr, "write experiment markdown failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(*experimentOutput)
			return
		}
		fmt.Print(markdown)
		return
	}
	if strings.TrimSpace(*analyzeFile) != "" {
		report, err := loadtest.LoadReportFromSummary(*analyzeFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load summary failed: %v\n", err)
			os.Exit(1)
		}
		assessment := loadtest.AssessCapacity(report, loadtest.CapacityCurrentConfig{
			CommandMaxConcurrent:      *currentCmd,
			FileTransferMaxConcurrent: *currentFile,
			RTPPortPoolSize:           *currentRTP,
			MaxConnections:            *currentConn,
			RateLimitRPS:              *currentRPS,
			RateLimitBurst:            *currentBurst,
		})
		output, _ := json.MarshalIndent(assessment, "", "  ")
		fmt.Println(string(output))
		return
	}

	mode := strings.ToLower(strings.TrimSpace(*transferMode))
	if alias := strings.ToLower(strings.TrimSpace(*rtpTransport)); alias != "" {
		mode = alias
	}
	cfg := loadtest.Config{
		Targets:            normalizeTargets(strings.Split(*targets, ","), mode),
		Concurrency:        *concurrency,
		QPS:                *qps,
		Duration:           *duration,
		FileSize:           *fileSize,
		ChunkSize:          *chunkSize,
		TransferMode:       mode,
		SIPAddress:         *sipAddress,
		RTPAddress:         *rtpAddress,
		RTPTransport:       strings.ToUpper(mode),
		HTTPURL:            *httpURL,
		MappingURL:         *mappingURL,
		MappingMethod:      *mappingMethod,
		MappingBodySize:    *mappingBodySize,
		AllowProbePath:     *allowProbePath,
		StrictRealMapping:  *strictRealMapping,
		OutputDir:          *outputDir,
		Timeout:            *timeout,
		GatewayBaseURL:     *gatewayBase,
		DiagnosticInterval: *diagInterval,
	}

	report, err := loadtest.Run(context.Background(), cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "loadtest failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Loadtest done. run_id=%s\n", report.RunID)
	fmt.Printf("Result file: %s\n", report.ResultFile)
	fmt.Printf("Report file: %s\n", report.ReportFile)
	fmt.Printf("Diagnostics snapshots: %d\n", len(report.Diagnostics))
	for _, t := range cfg.Targets {
		s := report.Summaries[t]
		fmt.Printf("[%s] total=%d success=%.2f%% throughput=%.2f/s p50=%.2fms p95=%.2fms p99=%.2fms errors=%v\n",
			t, s.Total, s.SuccessRate*100, s.Throughput, s.P50MS, s.P95MS, s.P99MS, s.ErrorTypes)
		if s.Failed > 0 && len(s.ErrorSamples) > 0 {
			fmt.Printf("[%s] sample_errors=%v\n", t, s.ErrorSamples)
		}
	}
}

func normalizeTargets(raw []string, mode string) []string {
	out := make([]string, 0, len(raw))
	for _, t := range raw {
		v := strings.ToLower(strings.TrimSpace(t))
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		out = []string{"mapping-forward", "sip-command-create", "sip-status-receipt", "rtp-udp-upload", "rtp-tcp-upload", "http-invoke"}
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "udp" {
		return filterTargets(out, func(s string) bool { return s != "rtp-tcp-upload" })
	}
	if mode == "tcp" {
		return filterTargets(out, func(s string) bool { return s != "rtp-udp-upload" })
	}
	return out
}

func filterTargets(items []string, keep func(string) bool) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if keep(item) {
			out = append(out, item)
		}
	}
	return out
}
