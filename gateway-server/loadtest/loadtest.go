package loadtest

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	mrand "math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	sipHeaderTerminator          = "\r\n\r\n"
	loadtestSuccessPersistStride = 128
	loadtestResultWriterBuffer   = 1 << 20
	loadtestStopGraceFloor       = 50 * time.Millisecond
	loadtestStopGraceCeiling     = 500 * time.Millisecond
	loadtestSocketBufferBytes    = 1 << 20
	loadtestUDPMaxChunkBytes     = 60 * 1024
)

type Config struct {
	Targets            []string
	Concurrency        int
	QPS                int
	Duration           time.Duration
	FileSize           int
	ChunkSize          int
	TransferMode       string
	SIPAddress         string
	SIPTransport       string
	RTPAddress         string
	RTPTransport       string
	HTTPURL            string
	MappingURL         string
	MappingMethod      string
	MappingBodySize    int
	AllowProbePath     bool
	StrictRealMapping  bool
	OutputDir          string
	Timeout            time.Duration
	GatewayBaseURL     string
	DiagnosticInterval time.Duration
}

type HTTPTraceMetrics struct {
	ConnectLatency    time.Duration `json:"connect_latency_ns,omitempty"`
	FirstByteLatency  time.Duration `json:"first_byte_latency_ns,omitempty"`
	ConnectionReused  bool          `json:"connection_reused,omitempty"`
	ConnectionWasIdle bool          `json:"connection_was_idle,omitempty"`
	Sampled           bool          `json:"sampled,omitempty"`
}

type Result struct {
	Target      string           `json:"target"`
	StartedAt   time.Time        `json:"started_at"`
	Latency     time.Duration    `json:"latency_ns"`
	Success     bool             `json:"success"`
	ErrorType   string           `json:"error_type,omitempty"`
	ErrorDetail string           `json:"error_detail,omitempty"`
	Trace       HTTPTraceMetrics `json:"trace,omitempty"`
}

type ErrorSample struct {
	Type   string `json:"type"`
	Detail string `json:"detail"`
}

type Summary struct {
	Target              string           `json:"target"`
	Total               int64            `json:"total"`
	Success             int64            `json:"success"`
	Failed              int64            `json:"failed"`
	SuccessRate         float64          `json:"success_rate"`
	Throughput          float64          `json:"throughput_qps"`
	P50MS               float64          `json:"p50_ms"`
	P95MS               float64          `json:"p95_ms"`
	P99MS               float64          `json:"p99_ms"`
	ConnectP50MS        float64          `json:"connect_p50_ms,omitempty"`
	ConnectP95MS        float64          `json:"connect_p95_ms,omitempty"`
	FirstByteP50MS      float64          `json:"first_byte_p50_ms,omitempty"`
	FirstByteP95MS      float64          `json:"first_byte_p95_ms,omitempty"`
	TraceSamples        int64            `json:"trace_samples,omitempty"`
	NewConnCount        int64            `json:"new_conn_count,omitempty"`
	ReusedConnCount     int64            `json:"reused_conn_count,omitempty"`
	ReusedIdleConnCount int64            `json:"reused_idle_conn_count,omitempty"`
	ErrorTypes          map[string]int64 `json:"error_types"`
	ErrorSamples        []ErrorSample    `json:"error_samples,omitempty"`
	ElapsedMS           int64            `json:"elapsed_ms"`
	Concurrency         int              `json:"concurrency"`
	ConfiguredQPS       int              `json:"configured_qps"`
}

type Report struct {
	RunID       string               `json:"run_id"`
	Generated   time.Time            `json:"generated_at"`
	Config      Config               `json:"config"`
	Summaries   map[string]Summary   `json:"summaries"`
	ResultFile  string               `json:"result_file"`
	Diagnostics []DiagnosticArtifact `json:"diagnostics,omitempty"`
	ReportFile  string               `json:"report_file,omitempty"`
}

type DiagnosticArtifact struct {
	Phase        string    `json:"phase"`
	CollectedAt  time.Time `json:"collected_at"`
	SnapshotFile string    `json:"snapshot_file"`
	ExportFile   string    `json:"export_file"`
	SummaryFile  string    `json:"summary_file"`
}

type OperationResult struct {
	Err   error
	Trace HTTPTraceMetrics
}

type opFunc func(context.Context) OperationResult

func Run(ctx context.Context, cfg Config) (Report, error) {
	if err := validateConfig(cfg); err != nil {
		return Report{}, err
	}
	runID := time.Now().UTC().Format("20060102-150405")
	outDir := filepath.Join(cfg.OutputDir, runID)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return Report{}, fmt.Errorf("create output dir: %w", err)
	}
	diagnosticsDir := filepath.Join(outDir, "diagnostics")
	if err := os.MkdirAll(diagnosticsDir, 0o755); err != nil {
		return Report{}, fmt.Errorf("create diagnostics dir: %w", err)
	}
	if err := preflightTargets(ctx, cfg); err != nil {
		return Report{}, err
	}

	artifacts := make([]DiagnosticArtifact, 0, 8)
	var artifactsMu sync.Mutex
	appendArtifact := func(a DiagnosticArtifact) {
		artifactsMu.Lock()
		defer artifactsMu.Unlock()
		artifacts = append(artifacts, a)
	}
	collector := newDiagnosticsCollector(cfg, diagnosticsDir)
	if a, err := collector.collect(ctx, "preflight"); err == nil {
		appendArtifact(a)
	}
	resultPath := filepath.Join(outDir, "results.jsonl")
	resultFile, err := os.Create(resultPath)
	if err != nil {
		return Report{}, fmt.Errorf("create result file: %w", err)
	}
	defer resultFile.Close()

	writer := bufio.NewWriterSize(resultFile, loadtestResultWriterBuffer)
	defer writer.Flush()

	type agg struct {
		latencies          []float64
		connectLatencies   []float64
		firstByteLatencies []float64
		errors             map[string]int64
		samples            []ErrorSample
		total              int64
		success            int64
		traceSamples       int64
		newConnCount       int64
		reusedConnCount    int64
		reusedIdleCount    int64
	}
	aggs := map[string]*agg{}
	for _, t := range cfg.Targets {
		aggs[t] = &agg{errors: map[string]int64{}, samples: make([]ErrorSample, 0, 8), latencies: make([]float64, 0, 1024), connectLatencies: make([]float64, 0, 256), firstByteLatencies: make([]float64, 0, 256)}
	}

	results := make(chan Result, cfg.Concurrency*4)
	var writeErr atomic.Value
	go func() {
		enc := json.NewEncoder(writer)
		for r := range results {
			a := aggs[r.Target]
			a.total++
			persist := !r.Success
			if r.Success {
				a.success++
				a.latencies = append(a.latencies, float64(r.Latency.Microseconds())/1000)
				if r.Trace.Sampled {
					a.traceSamples++
					if r.Trace.ConnectLatency > 0 {
						a.connectLatencies = append(a.connectLatencies, float64(r.Trace.ConnectLatency.Microseconds())/1000)
					}
					if r.Trace.FirstByteLatency > 0 {
						a.firstByteLatencies = append(a.firstByteLatencies, float64(r.Trace.FirstByteLatency.Microseconds())/1000)
					}
					if r.Trace.ConnectionReused {
						a.reusedConnCount++
						if r.Trace.ConnectionWasIdle {
							a.reusedIdleCount++
						}
					} else {
						a.newConnCount++
					}
				}
				if loadtestSuccessPersistStride <= 1 || a.success%loadtestSuccessPersistStride == 0 {
					persist = true
				}
			} else {
				a.errors[r.ErrorType]++
				if len(a.samples) < 8 && strings.TrimSpace(r.ErrorDetail) != "" {
					a.samples = append(a.samples, ErrorSample{Type: r.ErrorType, Detail: r.ErrorDetail})
				}
			}
			if persist {
				if err := enc.Encode(r); err != nil {
					writeErr.Store(err)
					continue
				}
			}
		}
	}()

	start := time.Now()
	stopAt := start.Add(cfg.Duration)
	runCtx, cancel := context.WithDeadline(ctx, stopAt)
	defer cancel()
	ops := buildOperations(cfg)

	var diagWG sync.WaitGroup
	if cfg.DiagnosticInterval > 0 {
		diagWG.Add(1)
		go func() {
			defer diagWG.Done()
			ticker := time.NewTicker(cfg.DiagnosticInterval)
			defer ticker.Stop()
			for {
				select {
				case <-runCtx.Done():
					return
				case ts := <-ticker.C:
					phase := "during_" + ts.UTC().Format("150405")
					if a, err := collector.collect(runCtx, phase); err == nil {
						appendArtifact(a)
					}
				}
			}
		}()
	}

	var wg sync.WaitGroup
	var limiter <-chan time.Time
	var limiterTicker *time.Ticker
	if cfg.QPS > 0 {
		limiterTicker = time.NewTicker(time.Second / time.Duration(cfg.QPS))
		defer limiterTicker.Stop()
		limiter = limiterTicker.C
	}
	stopGrace := loadtestStopGrace(cfg.Timeout)

	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			r := mrand.New(mrand.NewSource(time.Now().UnixNano() + seed))
			for {
				if time.Until(stopAt) <= stopGrace {
					return
				}
				select {
				case <-runCtx.Done():
					return
				default:
				}
				if limiter != nil {
					select {
					case <-runCtx.Done():
						return
					case <-limiter:
					}
					if time.Until(stopAt) <= stopGrace {
						return
					}
				}
				target := cfg.Targets[r.Intn(len(cfg.Targets))]
				op := ops[target]
				started := time.Now()
				opCtx, opCancel := context.WithTimeout(ctx, cfg.Timeout)
				opRes := op(opCtx)
				opCancel()
				err := opRes.Err
				res := Result{Target: target, StartedAt: started, Latency: time.Since(started), Success: err == nil, Trace: opRes.Trace}
				if err != nil {
					res.ErrorType = classifyErr(err)
					res.ErrorDetail = err.Error()
				}
				select {
				case results <- res:
				case <-ctx.Done():
					return
				}
			}
		}(int64(i))
	}

	wg.Wait()
	diagWG.Wait()
	close(results)
	elapsed := cfg.Duration
	if elapsed <= 0 {
		elapsed = time.Since(start)
	}
	if v := writeErr.Load(); v != nil {
		return Report{}, fmt.Errorf("write result: %w", v.(error))
	}

	summaries := make(map[string]Summary, len(aggs))
	for target, a := range aggs {
		sort.Float64s(a.latencies)
		sort.Float64s(a.connectLatencies)
		sort.Float64s(a.firstByteLatencies)
		s := Summary{
			Target:              target,
			Total:               a.total,
			Success:             a.success,
			Failed:              a.total - a.success,
			SuccessRate:         safeRate(a.success, a.total),
			Throughput:          float64(a.total) / elapsed.Seconds(),
			P50MS:               percentile(a.latencies, 50),
			P95MS:               percentile(a.latencies, 95),
			P99MS:               percentile(a.latencies, 99),
			ConnectP50MS:        percentile(a.connectLatencies, 50),
			ConnectP95MS:        percentile(a.connectLatencies, 95),
			FirstByteP50MS:      percentile(a.firstByteLatencies, 50),
			FirstByteP95MS:      percentile(a.firstByteLatencies, 95),
			TraceSamples:        a.traceSamples,
			NewConnCount:        a.newConnCount,
			ReusedConnCount:     a.reusedConnCount,
			ReusedIdleConnCount: a.reusedIdleCount,
			ErrorTypes:          a.errors,
			ErrorSamples:        append([]ErrorSample(nil), a.samples...),
			ElapsedMS:           elapsed.Milliseconds(),
			Concurrency:         cfg.Concurrency,
			ConfiguredQPS:       cfg.QPS,
		}
		summaries[target] = s
	}
	if a, err := collector.collect(ctx, "postrun"); err == nil {
		appendArtifact(a)
	}
	report := Report{RunID: runID, Generated: time.Now().UTC(), Config: cfg, Summaries: summaries, ResultFile: resultPath, Diagnostics: artifacts}
	report.ReportFile = filepath.Join(outDir, "report.md")
	reportPath := filepath.Join(outDir, "summary.json")
	if err := writeJSON(reportPath, report); err != nil {
		return Report{}, err
	}
	if err := os.WriteFile(report.ReportFile, []byte(renderMarkdownReport(report)), 0o644); err != nil {
		return Report{}, fmt.Errorf("write report: %w", err)
	}
	return report, nil
}
