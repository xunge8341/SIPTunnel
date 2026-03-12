package longrun

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"siptunnel/internal/config"
	"siptunnel/internal/protocol/rtpfile"
	"siptunnel/internal/protocol/sip"
	"siptunnel/internal/service/filetransfer"
	"siptunnel/internal/service/sipcontrol"
)

type modeConfig struct {
	Name                 string
	Duration             time.Duration
	SampleInterval       time.Duration
	CommandInterval      time.Duration
	FileInterval         time.Duration
	ReconnectInterval    time.Duration
	MaxErrorRate         float64
	MaxGoroutineGrowth   int
	MaxFDGrowth          int
	MaxMemoryGrowthBytes uint64
	MaxBufferGrowthBytes uint64
}

type metricsSample struct {
	Timestamp       time.Time `json:"timestamp"`
	Goroutines      int       `json:"goroutines"`
	FDs             int       `json:"fds"`
	HeapAllocBytes  uint64    `json:"heap_alloc_bytes"`
	HeapInuseBytes  uint64    `json:"heap_inuse_bytes"`
	Connections     int64     `json:"connections"`
	ActiveTasks     int64     `json:"active_tasks"`
	OperationsTotal uint64    `json:"operations_total"`
	ErrorsTotal     uint64    `json:"errors_total"`
	ErrorRate       float64   `json:"error_rate"`
}

type summary struct {
	Mode                 string  `json:"mode"`
	Duration             string  `json:"duration"`
	Samples              int     `json:"samples"`
	OperationsTotal      uint64  `json:"operations_total"`
	ErrorsTotal          uint64  `json:"errors_total"`
	ErrorRate            float64 `json:"error_rate"`
	BaselineGoroutines   int     `json:"baseline_goroutines"`
	FinalGoroutines      int     `json:"final_goroutines"`
	BaselineFDs          int     `json:"baseline_fds"`
	FinalFDs             int     `json:"final_fds"`
	BaselineHeapAlloc    uint64  `json:"baseline_heap_alloc_bytes"`
	FinalHeapAlloc       uint64  `json:"final_heap_alloc_bytes"`
	PeakHeapInuse        uint64  `json:"peak_heap_inuse_bytes"`
	PeakActiveTasks      int64   `json:"peak_active_tasks"`
	PeakConnections      int64   `json:"peak_connections"`
	ConnectionsRecovered bool    `json:"connections_recovered"`
	LeakSuspected        bool    `json:"leak_suspected"`
	BufferGrowthSuspect  bool    `json:"buffer_growth_suspect"`
}

func TestLongRunStability(t *testing.T) {
	if os.Getenv("LONGRUN_ENABLE") != "1" {
		t.Skip("set LONGRUN_ENABLE=1 to run long-run test")
	}

	cfg := loadModeConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration)
	defer cancel()

	dispatcher := sipcontrol.NewDispatcher(nil, nil, nil)
	dispatcher.RegisterHandler(sipcontrol.NewCommandCreateHandler(nil))
	dispatcher.RegisterHandler(sipcontrol.NewFileCreateHandler(nil))

	transport := filetransfer.NewTCPTransport()
	err := transport.Bootstrap(config.RTPConfig{
		Transport:           "TCP",
		MaxTCPSessions:      128,
		MaxPacketBytes:      2048,
		TCPReadTimeoutMS:    500,
		TCPWriteTimeoutMS:   500,
		TCPKeepAliveEnabled: true,
	})
	if err != nil {
		t.Fatalf("bootstrap transport: %v", err)
	}

	var operations atomic.Uint64
	var errorsTotal atomic.Uint64
	var activeTasks atomic.Int64
	var peakActiveTasks atomic.Int64

	incActive := func() {
		cur := activeTasks.Add(1)
		for {
			old := peakActiveTasks.Load()
			if cur <= old || peakActiveTasks.CompareAndSwap(old, cur) {
				break
			}
		}
	}

	decActive := func() {
		activeTasks.Add(-1)
	}

	var workers sync.WaitGroup
	workers.Add(2)

	go func() {
		defer workers.Done()
		ticker := time.NewTicker(cfg.CommandInterval)
		defer ticker.Stop()
		index := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				incActive()
				index++
				err := runCommandOnce(ctx, dispatcher, index)
				operations.Add(1)
				if err != nil {
					errorsTotal.Add(1)
				}
				decActive()
			}
		}
	}()

	go func() {
		defer workers.Done()
		ticker := time.NewTicker(cfg.FileInterval)
		defer ticker.Stop()
		index := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				incActive()
				index++
				err := runFileOnce(ctx, dispatcher, transport, index, cfg.ReconnectInterval)
				operations.Add(1)
				if err != nil {
					errorsTotal.Add(1)
				}
				decActive()
			}
		}
	}()

	baseline := captureSample(transport, &operations, &errorsTotal, &activeTasks)
	samples := []metricsSample{baseline}

	samplerDone := make(chan struct{})
	go func() {
		defer close(samplerDone)
		ticker := time.NewTicker(cfg.SampleInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				samples = append(samples, captureSample(transport, &operations, &errorsTotal, &activeTasks))
			}
		}
	}()

	<-ctx.Done()
	workers.Wait()
	<-samplerDone

	time.Sleep(100 * time.Millisecond)
	samples = append(samples, captureSample(transport, &operations, &errorsTotal, &activeTasks))

	reportDir, reportName := reportLocation(cfg.Name)
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		t.Fatalf("mkdir report dir: %v", err)
	}

	summaryData := buildSummary(cfg, samples, peakActiveTasks.Load())
	if err := writeJSONLines(filepath.Join(reportDir, reportName+".jsonl"), samples); err != nil {
		t.Fatalf("write sample jsonl: %v", err)
	}
	if err := writeSummaryMarkdown(filepath.Join(reportDir, reportName+".md"), summaryData); err != nil {
		t.Fatalf("write markdown summary: %v", err)
	}

	assertThresholds(t, cfg, summaryData)
}

func runCommandOnce(ctx context.Context, dispatcher *sipcontrol.Dispatcher, index int) error {
	now := time.Now().UTC()
	msg := sip.CommandCreate{
		Header:     validHeader(sip.MessageTypeCommandCreate, now, fmt.Sprintf("cmd-req-%d", index), fmt.Sprintf("trace-cmd-%d", index), fmt.Sprintf("nonce-cmd-%d", index)),
		CommandID:  fmt.Sprintf("cmd-%d", index),
		Parameters: map[string]any{"asset_id": "A1001"},
	}
	if index%50 == 0 {
		msg.Header.ExpireAt = now.Add(-time.Second)
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = dispatcher.Route(ctx, sipcontrol.InboundMessage{Body: body})
	return err
}

func runFileOnce(ctx context.Context, dispatcher *sipcontrol.Dispatcher, transport *filetransfer.TCPTransport, index int, reconnectInterval time.Duration) error {
	now := time.Now().UTC()
	msg := sip.FileCreate{
		Header:    validHeader(sip.MessageTypeFileCreate, now, fmt.Sprintf("file-req-%d", index), fmt.Sprintf("trace-file-%d", index), fmt.Sprintf("nonce-file-%d", index)),
		FileID:    fmt.Sprintf("file-%d", index),
		TaskID:    fmt.Sprintf("task-%d", index),
		TotalSize: 256,
		ChunkSize: 128,
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if _, err := dispatcher.Route(ctx, sipcontrol.InboundMessage{Body: body}); err != nil {
		return err
	}

	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()

	writer, err := transport.OpenSession(left)
	if err != nil {
		return err
	}
	defer writer.Close()

	reader, err := transport.OpenSession(right)
	if err != nil {
		return err
	}
	defer reader.Close()

	packets, err := rtpfile.SplitFileToChunks([]byte{1, 2, 3, 4}, rtpfile.ChunkOptions{ChunkSize: 4})
	if err != nil {
		return err
	}
	packet := packets[0]

	errCh := make(chan error, 1)
	go func() {
		_, readErr := reader.ReadPacket()
		errCh <- readErr
	}()
	if err := writer.WritePacket(packet); err != nil {
		return err
	}
	if err := <-errCh; err != nil {
		return err
	}

	if reconnectInterval > 0 && index%10 == 0 {
		time.Sleep(reconnectInterval)
	}
	return nil
}

func validHeader(messageType string, now time.Time, requestID string, traceID string, nonce string) sip.Header {
	return sip.Header{
		ProtocolVersion: sip.ProtocolVersionV1,
		MessageType:     messageType,
		RequestID:       requestID,
		TraceID:         traceID,
		SessionID:       "soak-session",
		ApiCode:         "asset.sync",
		SourceSystem:    "system-a",
		SourceNode:      "node-a",
		Timestamp:       now,
		ExpireAt:        now.Add(time.Minute),
		Nonce:           nonce,
		DigestAlg:       "sha256",
		PayloadDigest:   "digest",
		SignAlg:         "hmac-sha256",
		Signature:       "signature",
	}
}

func captureSample(transport *filetransfer.TCPTransport, operations *atomic.Uint64, errorsTotal *atomic.Uint64, activeTasks *atomic.Int64) metricsSample {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	total := operations.Load()
	errorsCount := errorsTotal.Load()
	errRate := 0.0
	if total > 0 {
		errRate = float64(errorsCount) / float64(total)
	}
	return metricsSample{
		Timestamp:       time.Now().UTC(),
		Goroutines:      runtime.NumGoroutine(),
		FDs:             countFDs(),
		HeapAllocBytes:  mem.Alloc,
		HeapInuseBytes:  mem.HeapInuse,
		Connections:     transport.Snapshot().TCPSessionsCurrent,
		ActiveTasks:     activeTasks.Load(),
		OperationsTotal: total,
		ErrorsTotal:     errorsCount,
		ErrorRate:       errRate,
	}
}

func countFDs() int {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return -1
	}
	return len(entries)
}

func loadModeConfig(t *testing.T) modeConfig {
	t.Helper()
	mode := strings.TrimSpace(os.Getenv("LONGRUN_MODE"))
	if mode == "" {
		mode = "1h"
	}
	cfg, ok := map[string]modeConfig{
		"smoke": {
			Name:                 "smoke",
			Duration:             3 * time.Minute,
			SampleInterval:       10 * time.Second,
			CommandInterval:      60 * time.Millisecond,
			FileInterval:         80 * time.Millisecond,
			ReconnectInterval:    20 * time.Millisecond,
			MaxErrorRate:         0.05,
			MaxGoroutineGrowth:   80,
			MaxFDGrowth:          40,
			MaxMemoryGrowthBytes: 64 * 1024 * 1024,
			MaxBufferGrowthBytes: 96 * 1024 * 1024,
		},
		"1h": {
			Name:                 "1h",
			Duration:             time.Hour,
			SampleInterval:       15 * time.Second,
			CommandInterval:      40 * time.Millisecond,
			FileInterval:         60 * time.Millisecond,
			ReconnectInterval:    10 * time.Millisecond,
			MaxErrorRate:         0.03,
			MaxGoroutineGrowth:   120,
			MaxFDGrowth:          64,
			MaxMemoryGrowthBytes: 256 * 1024 * 1024,
			MaxBufferGrowthBytes: 384 * 1024 * 1024,
		},
		"6h": {
			Name:                 "6h",
			Duration:             6 * time.Hour,
			SampleInterval:       30 * time.Second,
			CommandInterval:      50 * time.Millisecond,
			FileInterval:         80 * time.Millisecond,
			ReconnectInterval:    20 * time.Millisecond,
			MaxErrorRate:         0.03,
			MaxGoroutineGrowth:   160,
			MaxFDGrowth:          80,
			MaxMemoryGrowthBytes: 512 * 1024 * 1024,
			MaxBufferGrowthBytes: 768 * 1024 * 1024,
		},
		"24h": {
			Name:                 "24h",
			Duration:             24 * time.Hour,
			SampleInterval:       time.Minute,
			CommandInterval:      60 * time.Millisecond,
			FileInterval:         100 * time.Millisecond,
			ReconnectInterval:    30 * time.Millisecond,
			MaxErrorRate:         0.02,
			MaxGoroutineGrowth:   240,
			MaxFDGrowth:          120,
			MaxMemoryGrowthBytes: 1024 * 1024 * 1024,
			MaxBufferGrowthBytes: 1536 * 1024 * 1024,
		},
	}[mode]
	if !ok {
		t.Fatalf("unsupported LONGRUN_MODE=%q (supported: smoke/1h/6h/24h)", mode)
	}
	if v := strings.TrimSpace(os.Getenv("LONGRUN_DURATION")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			t.Fatalf("parse LONGRUN_DURATION: %v", err)
		}
		cfg.Duration = d
	}
	if v := strings.TrimSpace(os.Getenv("LONGRUN_SAMPLE_INTERVAL")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			t.Fatalf("parse LONGRUN_SAMPLE_INTERVAL: %v", err)
		}
		cfg.SampleInterval = d
	}
	return cfg
}

func reportLocation(mode string) (dir string, name string) {
	base := os.Getenv("LONGRUN_REPORT_DIR")
	if strings.TrimSpace(base) == "" {
		base = filepath.Join("tests", "longrun", "output")
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	return base, fmt.Sprintf("longrun-%s-%s", mode, stamp)
}

func buildSummary(cfg modeConfig, samples []metricsSample, peakActive int64) summary {
	first := samples[0]
	last := samples[len(samples)-1]
	peakHeap := first.HeapInuseBytes
	peakConn := first.Connections
	for _, s := range samples {
		if s.HeapInuseBytes > peakHeap {
			peakHeap = s.HeapInuseBytes
		}
		if s.Connections > peakConn {
			peakConn = s.Connections
		}
	}
	leakSuspected := (last.Goroutines-first.Goroutines) > cfg.MaxGoroutineGrowth ||
		(last.FDs >= 0 && first.FDs >= 0 && (last.FDs-first.FDs) > cfg.MaxFDGrowth) ||
		(last.HeapAllocBytes-first.HeapAllocBytes) > cfg.MaxMemoryGrowthBytes
	bufferGrowthSuspect := (peakHeap - first.HeapInuseBytes) > cfg.MaxBufferGrowthBytes
	return summary{
		Mode:                 cfg.Name,
		Duration:             cfg.Duration.String(),
		Samples:              len(samples),
		OperationsTotal:      last.OperationsTotal,
		ErrorsTotal:          last.ErrorsTotal,
		ErrorRate:            last.ErrorRate,
		BaselineGoroutines:   first.Goroutines,
		FinalGoroutines:      last.Goroutines,
		BaselineFDs:          first.FDs,
		FinalFDs:             last.FDs,
		BaselineHeapAlloc:    first.HeapAllocBytes,
		FinalHeapAlloc:       last.HeapAllocBytes,
		PeakHeapInuse:        peakHeap,
		PeakActiveTasks:      peakActive,
		PeakConnections:      peakConn,
		ConnectionsRecovered: last.Connections == 0,
		LeakSuspected:        leakSuspected,
		BufferGrowthSuspect:  bufferGrowthSuspect,
	}
}

func writeJSONLines(path string, samples []metricsSample) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, sample := range samples {
		if err := enc.Encode(sample); err != nil {
			return err
		}
	}
	return nil
}

func writeSummaryMarkdown(path string, s summary) error {
	content := fmt.Sprintf(`# LongRun Stability Summary

- Mode: %s
- Duration: %s
- Samples: %d
- Operations: %d
- Errors: %d
- ErrorRate: %.4f

## Leak/Recovery Focus
- Goroutine baseline/final: %d -> %d
- FD baseline/final: %d -> %d
- HeapAlloc baseline/final (bytes): %d -> %d
- Peak HeapInuse (bytes): %d
- Peak connections: %d
- Peak active tasks: %d
- Connections recovered: %t
- Leak suspected: %t
- Buffer growth suspect: %t
`, s.Mode, s.Duration, s.Samples, s.OperationsTotal, s.ErrorsTotal, s.ErrorRate,
		s.BaselineGoroutines, s.FinalGoroutines,
		s.BaselineFDs, s.FinalFDs,
		s.BaselineHeapAlloc, s.FinalHeapAlloc,
		s.PeakHeapInuse, s.PeakConnections, s.PeakActiveTasks,
		s.ConnectionsRecovered, s.LeakSuspected, s.BufferGrowthSuspect)
	return os.WriteFile(path, []byte(content), 0o644)
}

func assertThresholds(t *testing.T, cfg modeConfig, s summary) {
	t.Helper()
	if s.ErrorRate > cfg.MaxErrorRate {
		t.Fatalf("error rate too high: %.4f > %.4f", s.ErrorRate, cfg.MaxErrorRate)
	}
	if s.FinalGoroutines-s.BaselineGoroutines > cfg.MaxGoroutineGrowth {
		t.Fatalf("goroutine growth too high: %d", s.FinalGoroutines-s.BaselineGoroutines)
	}
	if s.BaselineFDs >= 0 && s.FinalFDs >= 0 && s.FinalFDs-s.BaselineFDs > cfg.MaxFDGrowth {
		t.Fatalf("fd growth too high: %d", s.FinalFDs-s.BaselineFDs)
	}
	if s.FinalHeapAlloc-s.BaselineHeapAlloc > cfg.MaxMemoryGrowthBytes {
		t.Fatalf("memory growth too high: %d", s.FinalHeapAlloc-s.BaselineHeapAlloc)
	}
	if !s.ConnectionsRecovered {
		t.Fatalf("tcp connections not fully recovered; current=%d", s.PeakConnections)
	}
}

func TestBuildSummaryFlagsLeakAndBufferGrowth(t *testing.T) {
	cfg := modeConfig{MaxGoroutineGrowth: 2, MaxFDGrowth: 1, MaxMemoryGrowthBytes: 4, MaxBufferGrowthBytes: 8}
	samples := []metricsSample{
		{Goroutines: 10, FDs: 20, HeapAllocBytes: 100, HeapInuseBytes: 120, Connections: 3},
		{Goroutines: 20, FDs: 25, HeapAllocBytes: 200, HeapInuseBytes: 200, Connections: 0, OperationsTotal: 100, ErrorsTotal: 5, ErrorRate: 0.05},
	}
	s := buildSummary(cfg, samples, 9)
	if !s.LeakSuspected {
		t.Fatalf("expected leak suspected")
	}
	if !s.BufferGrowthSuspect {
		t.Fatalf("expected buffer growth suspect")
	}
	if s.PeakActiveTasks != 9 {
		t.Fatalf("unexpected peak active tasks %d", s.PeakActiveTasks)
	}
}

func TestCountFDs(t *testing.T) {
	fd := countFDs()
	if runtime.GOOS == "linux" && fd <= 0 {
		t.Fatalf("expected fd count > 0, got %d", fd)
	}
	if runtime.GOOS != "linux" && fd != -1 {
		t.Fatalf("expected non-linux fallback -1, got %d", fd)
	}
}

func TestLoadModeConfigOverrideDuration(t *testing.T) {
	t.Setenv("LONGRUN_MODE", "smoke")
	t.Setenv("LONGRUN_DURATION", "5s")
	cfg := loadModeConfig(t)
	if cfg.Duration != 5*time.Second {
		t.Fatalf("expected duration override 5s, got %s", cfg.Duration)
	}
}

func TestReportLocationUsesOverride(t *testing.T) {
	t.Setenv("LONGRUN_REPORT_DIR", "tmp/report")
	dir, name := reportLocation("smoke")
	if dir != "tmp/report" {
		t.Fatalf("expected custom report dir, got %s", dir)
	}
	if !strings.Contains(name, "longrun-smoke-") {
		t.Fatalf("unexpected report name %s", name)
	}
}

func TestLoadModeConfigOverrideSampleInterval(t *testing.T) {
	t.Setenv("LONGRUN_MODE", "smoke")
	t.Setenv("LONGRUN_SAMPLE_INTERVAL", "2s")
	cfg := loadModeConfig(t)
	if cfg.SampleInterval != 2*time.Second {
		t.Fatalf("expected sample interval override")
	}
}
