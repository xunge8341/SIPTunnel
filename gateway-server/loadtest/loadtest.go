package loadtest

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	mrand "math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"siptunnel/internal/protocol/rtpfile"
)

const sipHeaderTerminator = "\r\n\r\n"

type Config struct {
	Targets            []string
	Concurrency        int
	QPS                int
	Duration           time.Duration
	FileSize           int
	ChunkSize          int
	TransferMode       string
	SIPAddress         string
	RTPAddress         string
	HTTPURL            string
	OutputDir          string
	Timeout            time.Duration
	GatewayBaseURL     string
	DiagnosticInterval time.Duration
}

type Result struct {
	Target      string        `json:"target"`
	StartedAt   time.Time     `json:"started_at"`
	Latency     time.Duration `json:"latency_ns"`
	Success     bool          `json:"success"`
	ErrorType   string        `json:"error_type,omitempty"`
	ErrorDetail string        `json:"error_detail,omitempty"`
}

type Summary struct {
	Target        string           `json:"target"`
	Total         int64            `json:"total"`
	Success       int64            `json:"success"`
	Failed        int64            `json:"failed"`
	SuccessRate   float64          `json:"success_rate"`
	Throughput    float64          `json:"throughput_qps"`
	P50MS         float64          `json:"p50_ms"`
	P95MS         float64          `json:"p95_ms"`
	P99MS         float64          `json:"p99_ms"`
	ErrorTypes    map[string]int64 `json:"error_types"`
	ElapsedMS     int64            `json:"elapsed_ms"`
	Concurrency   int              `json:"concurrency"`
	ConfiguredQPS int              `json:"configured_qps"`
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

type opFunc func(context.Context) error

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

	writer := bufio.NewWriter(resultFile)
	defer writer.Flush()

	type agg struct {
		latencies []float64
		errors    map[string]int64
		total     int64
		success   int64
	}
	aggs := map[string]*agg{}
	for _, t := range cfg.Targets {
		aggs[t] = &agg{errors: map[string]int64{}, latencies: make([]float64, 0, 1024)}
	}

	results := make(chan Result, cfg.Concurrency*4)
	var writeErr atomic.Value
	go func() {
		enc := json.NewEncoder(writer)
		for r := range results {
			if err := enc.Encode(r); err != nil {
				writeErr.Store(err)
				continue
			}
			a := aggs[r.Target]
			a.total++
			if r.Success {
				a.success++
				a.latencies = append(a.latencies, float64(r.Latency.Microseconds())/1000)
			} else {
				a.errors[r.ErrorType]++
			}
		}
	}()

	start := time.Now()
	deadlineCtx, cancel := context.WithTimeout(ctx, cfg.Duration)
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
				case <-deadlineCtx.Done():
					return
				case ts := <-ticker.C:
					phase := "during_" + ts.UTC().Format("150405")
					if a, err := collector.collect(deadlineCtx, phase); err == nil {
						appendArtifact(a)
					}
				}
			}
		}()
	}

	var wg sync.WaitGroup
	var limiter <-chan time.Time
	if cfg.QPS > 0 {
		limiter = time.NewTicker(time.Second / time.Duration(cfg.QPS)).C
	}

	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			r := mrand.New(mrand.NewSource(time.Now().UnixNano() + seed))
			for {
				select {
				case <-deadlineCtx.Done():
					return
				default:
				}
				if limiter != nil {
					select {
					case <-deadlineCtx.Done():
						return
					case <-limiter:
					}
				}
				target := cfg.Targets[r.Intn(len(cfg.Targets))]
				op := ops[target]
				started := time.Now()
				err := op(deadlineCtx)
				res := Result{Target: target, StartedAt: started, Latency: time.Since(started), Success: err == nil}
				if err != nil {
					res.ErrorType = classifyErr(err)
					res.ErrorDetail = err.Error()
				}
				select {
				case results <- res:
				case <-deadlineCtx.Done():
					return
				}
			}
		}(int64(i))
	}

	wg.Wait()
	diagWG.Wait()
	close(results)
	elapsed := time.Since(start)
	if v := writeErr.Load(); v != nil {
		return Report{}, fmt.Errorf("write result: %w", v.(error))
	}

	summaries := make(map[string]Summary, len(aggs))
	for target, a := range aggs {
		sort.Float64s(a.latencies)
		s := Summary{
			Target:        target,
			Total:         a.total,
			Success:       a.success,
			Failed:        a.total - a.success,
			SuccessRate:   safeRate(a.success, a.total),
			Throughput:    float64(a.total) / elapsed.Seconds(),
			P50MS:         percentile(a.latencies, 50),
			P95MS:         percentile(a.latencies, 95),
			P99MS:         percentile(a.latencies, 99),
			ErrorTypes:    a.errors,
			ElapsedMS:     elapsed.Milliseconds(),
			Concurrency:   cfg.Concurrency,
			ConfiguredQPS: cfg.QPS,
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

type diagnosticsCollector struct {
	baseURL string
	outDir  string
	client  *http.Client
}

func newDiagnosticsCollector(cfg Config, outDir string) diagnosticsCollector {
	return diagnosticsCollector{
		baseURL: resolveGatewayBaseURL(cfg.GatewayBaseURL, cfg.HTTPURL),
		outDir:  outDir,
		client:  &http.Client{Timeout: cfg.Timeout},
	}
}

func (c diagnosticsCollector) collect(ctx context.Context, phase string) (DiagnosticArtifact, error) {
	if c.baseURL == "" {
		return DiagnosticArtifact{}, errors.New("empty gateway base url")
	}
	phase = strings.ToLower(strings.TrimSpace(phase))
	phase = strings.ReplaceAll(phase, " ", "_")
	phase = strings.ReplaceAll(phase, ":", "")
	stamp := time.Now().UTC()
	artifact := DiagnosticArtifact{Phase: phase, CollectedAt: stamp}

	snapshotPath := filepath.Join(c.outDir, phase+"_network_status.json")
	if err := c.fetchJSON(ctx, c.baseURL+"/api/node/network/status", snapshotPath); err != nil {
		return DiagnosticArtifact{}, err
	}
	exportPath := filepath.Join(c.outDir, phase+"_diagnostics_export.json")
	if err := c.fetchJSON(ctx, c.baseURL+"/api/diagnostics/export", exportPath); err != nil {
		return DiagnosticArtifact{}, err
	}
	summaryPath := filepath.Join(c.outDir, phase+"_ops_summary.txt")
	summaryText, err := summarizeDiagnostics(exportPath)
	if err != nil {
		return DiagnosticArtifact{}, err
	}
	if err := os.WriteFile(summaryPath, []byte(summaryText), 0o644); err != nil {
		return DiagnosticArtifact{}, err
	}
	artifact.SnapshotFile = snapshotPath
	artifact.ExportFile = exportPath
	artifact.SummaryFile = summaryPath
	return artifact, nil
}

func (c diagnosticsCollector) fetchJSON(ctx context.Context, endpoint, outputPath string) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s status %d", endpoint, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, body, 0o644)
}

func summarizeDiagnostics(exportPath string) (string, error) {
	type diagFile struct {
		Name    string `json:"name"`
		Content any    `json:"content"`
	}
	type diagData struct {
		GeneratedAt string     `json:"generated_at"`
		Files       []diagFile `json:"files"`
	}
	type envelope struct {
		Data diagData `json:"data"`
	}
	raw, err := os.ReadFile(exportPath)
	if err != nil {
		return "", err
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", err
	}
	b := &strings.Builder{}
	b.WriteString("### 运维诊断摘要\n")
	b.WriteString("采集时间: " + env.Data.GeneratedAt + "\n")
	for _, f := range env.Data.Files {
		if f.Name != "02_connection_stats_snapshot.json" && f.Name != "03_port_pool_status.json" && f.Name != "04_transport_error_summary.json" {
			continue
		}
		line, err := json.Marshal(f.Content)
		if err != nil {
			continue
		}
		b.WriteString("- " + f.Name + ": " + string(line) + "\n")
	}
	return b.String(), nil
}

func renderMarkdownReport(report Report) string {
	b := &strings.Builder{}
	b.WriteString("# SIPTunnel 压测报告\n\n")
	b.WriteString("## 基本参数\n")
	b.WriteString("- RunID: " + report.RunID + "\n")
	b.WriteString("- 目标: " + strings.Join(report.Config.Targets, ", ") + "\n")
	b.WriteString("- 并发: " + strconv.Itoa(report.Config.Concurrency) + "\n")
	b.WriteString("- QPS: " + strconv.Itoa(report.Config.QPS) + "\n")
	b.WriteString("- 时长: " + report.Config.Duration.String() + "\n")
	b.WriteString("\n## 结果概览\n")
	b.WriteString("| Target | 吞吐(qps) | 成功率 | P50(ms) | P95(ms) | P99(ms) | 关键错误 |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---|\n")
	for _, target := range report.Config.Targets {
		s := report.Summaries[target]
		b.WriteString(fmt.Sprintf("| %s | %.2f | %.2f%% | %.2f | %.2f | %.2f | %v |\n", target, s.Throughput, s.SuccessRate*100, s.P50MS, s.P95MS, s.P99MS, s.ErrorTypes))
	}
	b.WriteString("\n## 诊断快照\n")
	for _, d := range report.Diagnostics {
		b.WriteString(fmt.Sprintf("- %s: network=`%s` diagnostics=`%s` summary=`%s`\n", d.Phase, d.SnapshotFile, d.ExportFile, d.SummaryFile))
	}
	return b.String()
}

func resolveGatewayBaseURL(explicit, httpURL string) string {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimRight(strings.TrimSpace(explicit), "/")
	}
	u, err := url.Parse(strings.TrimSpace(httpURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return strings.TrimRight(u.Scheme+"://"+u.Host, "/")
}

func buildOperations(cfg Config) map[string]opFunc {
	data := makePayload(cfg.FileSize)
	chunks := buildChunks(data, cfg.ChunkSize)
	httpClient := &http.Client{Timeout: cfg.Timeout}
	return map[string]opFunc{
		"sip-command-create": func(ctx context.Context) error { return sendSIPCommandCreate(ctx, cfg) },
		"sip-status-receipt": func(ctx context.Context) error { return sendSIPStatusChain(ctx, cfg) },
		"rtp-udp-upload":     func(ctx context.Context) error { return sendRTPUDP(ctx, cfg.RTPAddress, chunks, cfg.Timeout) },
		"rtp-tcp-upload":     func(ctx context.Context) error { return sendRTPTCP(ctx, cfg.RTPAddress, chunks, cfg.Timeout) },
		"http-invoke":        func(ctx context.Context) error { return invokeHTTP(ctx, httpClient, cfg.HTTPURL) },
	}
}

func sendSIPCommandCreate(ctx context.Context, cfg Config) error {
	msg := map[string]any{"protocol_version": "1.0", "message_type": "command.create", "request_id": randomHex(16), "trace_id": randomHex(16), "session_id": randomHex(16), "api_code": "asset.sync", "source_system": "loadtest", "source_node": "bench", "timestamp": time.Now().UTC(), "expire_at": time.Now().UTC().Add(5 * time.Minute), "nonce": randomHex(8), "digest_alg": "sha256", "payload_digest": randomHex(16), "sign_alg": "hmac-sha256", "signature": randomHex(32), "command_id": randomHex(8), "parameters": map[string]any{"mode": "loadtest"}}
	payload, _ := json.Marshal(msg)
	resp, err := sendSIPFrame(ctx, cfg.SIPAddress, payload, cfg.Timeout)
	if err != nil {
		return err
	}
	if len(resp) == 0 {
		return errors.New("empty sip response")
	}
	return nil
}

func sendSIPStatusChain(ctx context.Context, cfg Config) error {
	if err := sendSIPCommandCreate(ctx, cfg); err != nil {
		return fmt.Errorf("status-chain command.create: %w", err)
	}
	msg := map[string]any{"protocol_version": "1.0", "message_type": "task.status", "request_id": randomHex(16), "trace_id": randomHex(16), "session_id": randomHex(16), "api_code": "asset.sync", "source_system": "loadtest", "source_node": "bench", "timestamp": time.Now().UTC(), "expire_at": time.Now().UTC().Add(5 * time.Minute), "nonce": randomHex(8), "digest_alg": "sha256", "payload_digest": randomHex(16), "sign_alg": "hmac-sha256", "signature": randomHex(32), "task_id": randomHex(8), "status": "RUNNING", "progress": 66}
	payload, _ := json.Marshal(msg)
	_, err := sendSIPFrame(ctx, cfg.SIPAddress, payload, cfg.Timeout)
	return err
}

func sendSIPFrame(ctx context.Context, addr string, payload []byte, timeout time.Duration) ([]byte, error) {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	frame := []byte(fmt.Sprintf("SIP-TUNNEL/1.0\r\nContent-Length: %d\r\n\r\n", len(payload)))
	frame = append(frame, payload...)
	if _, err := conn.Write(frame); err != nil {
		return nil, err
	}
	buf := make([]byte, 64*1024)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return decodeSIPFrame(buf[:n])
}

func decodeSIPFrame(data []byte) ([]byte, error) {
	raw := string(data)
	idx := strings.Index(raw, sipHeaderTerminator)
	if idx < 0 {
		return nil, errors.New("invalid sip frame")
	}
	body := data[idx+len(sipHeaderTerminator):]
	if len(body) == 0 {
		return nil, errors.New("empty sip frame body")
	}
	return body, nil
}

func invokeHTTP(ctx context.Context, client *http.Client, url string) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(`{"payload":"loadtest"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", randomHex(16))
	req.Header.Set("X-Trace-ID", randomHex(16))
	req.Header.Set("X-Api-Code", "asset.sync")
	req.Header.Set("X-Source-System", "loadtest")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}
	return nil
}

func sendRTPUDP(ctx context.Context, addr string, chunks []rtpfile.ChunkPacket, timeout time.Duration) error {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "udp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	for _, c := range chunks {
		frame, err := marshalChunkPacket(c)
		if err != nil {
			return err
		}
		if _, err := conn.Write(frame); err != nil {
			return err
		}
	}
	return nil
}

func sendRTPTCP(ctx context.Context, addr string, chunks []rtpfile.ChunkPacket, timeout time.Duration) error {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	for _, c := range chunks {
		frame, err := marshalChunkPacket(c)
		if err != nil {
			return err
		}
		lenPrefix := make([]byte, 4)
		binary.BigEndian.PutUint32(lenPrefix, uint32(len(frame)))
		if _, err := conn.Write(append(lenPrefix, frame...)); err != nil {
			return err
		}
	}
	return nil
}

func marshalChunkPacket(packet rtpfile.ChunkPacket) ([]byte, error) {
	hdr, err := packet.Header.MarshalBinary()
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(hdr)+len(packet.Payload))
	copy(out, hdr)
	copy(out[len(hdr):], packet.Payload)
	return out, nil
}

func buildChunks(data []byte, chunkSize int) []rtpfile.ChunkPacket {
	var transferID [16]byte
	var requestID [16]byte
	var traceID [16]byte
	copy(transferID[:], randomBytes(16))
	copy(requestID[:], randomBytes(16))
	copy(traceID[:], randomBytes(16))
	chunks, _ := rtpfile.SplitFileToChunks(data, rtpfile.ChunkOptions{TransferID: transferID, RequestID: requestID, TraceID: traceID, ChunkSize: chunkSize, Extensions: []rtpfile.TLV{{Type: rtpfile.TLVTypeFileName, Value: []byte("loadtest.bin")}}})
	return chunks
}

func makePayload(size int) []byte {
	if size <= 0 {
		size = 1024
	}
	buf := make([]byte, size)
	for i := range buf {
		buf[i] = byte('A' + (i % 26))
	}
	sum := sha256.Sum256(buf)
	copy(buf[:min(16, len(buf))], sum[:16])
	return buf
}

func validateConfig(cfg Config) error {
	if len(cfg.Targets) == 0 {
		return errors.New("targets is required")
	}
	if cfg.Concurrency <= 0 {
		return errors.New("concurrency must be > 0")
	}
	if cfg.Duration <= 0 {
		return errors.New("duration must be > 0")
	}
	if cfg.OutputDir == "" {
		return errors.New("output-dir is required")
	}
	if cfg.Timeout <= 0 {
		return errors.New("timeout must be > 0")
	}
	return nil
}

func classifyErr(err error) string {
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "timeout") || strings.Contains(s, "deadline exceeded"):
		return "timeout"
	case strings.Contains(s, "connection refused"):
		return "connection_refused"
	case strings.Contains(s, "status"):
		return "http_status"
	default:
		return "operation_error"
	}
}

func safeRate(success, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(success) / float64(total)
}

func percentile(sortedValues []float64, p float64) float64 {
	if len(sortedValues) == 0 {
		return 0
	}
	if p <= 0 {
		return sortedValues[0]
	}
	if p >= 100 {
		return sortedValues[len(sortedValues)-1]
	}
	index := (p / 100) * float64(len(sortedValues)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))
	if lower == upper {
		return sortedValues[lower]
	}
	weight := index - float64(lower)
	return sortedValues[lower]*(1-weight) + sortedValues[upper]*weight
}

func writeJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create summary file: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func randomHex(n int) string { return hex.EncodeToString(randomBytes(n)) }

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
