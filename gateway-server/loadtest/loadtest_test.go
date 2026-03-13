package loadtest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPercentile(t *testing.T) {
	vals := []float64{1, 2, 3, 4, 5}
	if got := percentile(vals, 50); got != 3 {
		t.Fatalf("p50 got %.2f", got)
	}
	if got := percentile(vals, 95); got <= 4 {
		t.Fatalf("p95 got %.2f", got)
	}
}

func TestValidateConfig(t *testing.T) {
	cfg := Config{Targets: []string{"http-invoke"}, Concurrency: 1, Duration: time.Second, OutputDir: t.TempDir(), Timeout: time.Second}
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig err=%v", err)
	}
}

func TestClassifyErr(t *testing.T) {
	if got := classifyErr(assertErr("context deadline exceeded")); got != "timeout" {
		t.Fatalf("classify timeout got=%s", got)
	}
	if got := classifyErr(assertErr("connection refused")); got != "connection_refused" {
		t.Fatalf("classify conn got=%s", got)
	}
}

func TestResolveGatewayBaseURL(t *testing.T) {
	if got := resolveGatewayBaseURL("http://127.0.0.1:18080/", ""); got != "http://127.0.0.1:18080" {
		t.Fatalf("explicit got=%s", got)
	}
	if got := resolveGatewayBaseURL("", "http://127.0.0.1:18080/demo/process"); got != "http://127.0.0.1:18080" {
		t.Fatalf("derived got=%s", got)
	}
}

func TestRunWritesReportAndDiagnostics(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/node/network/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":"OK","message":"success","data":{"sip":{"current_sessions":1},"rtp":{"port_pool_used":2}}}`))
		case "/api/diagnostics/export":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":"OK","message":"success","data":{"generated_at":"2026-01-01T00:00:00Z","files":[{"name":"02_connection_stats_snapshot.json","content":{"sip":{"current_connections":2}}},{"name":"03_port_pool_status.json","content":{"rtp_port_pool_used":3}},{"name":"04_transport_error_summary.json","content":{"recent_network_errors":["timeout"]}}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	cfg := Config{
		Targets:            []string{"http-invoke"},
		Concurrency:        1,
		QPS:                2,
		Duration:           1200 * time.Millisecond,
		FileSize:           1024,
		ChunkSize:          512,
		TransferMode:       "mixed",
		SIPAddress:         "127.0.0.1:1",
		RTPAddress:         "127.0.0.1:2",
		HTTPURL:            ts.URL + "/ok",
		OutputDir:          t.TempDir(),
		Timeout:            time.Second,
		GatewayBaseURL:     ts.URL,
		DiagnosticInterval: 500 * time.Millisecond,
	}
	report, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run err=%v", err)
	}
	if report.ReportFile == "" {
		t.Fatalf("missing report file")
	}
	if len(report.Diagnostics) < 2 {
		t.Fatalf("expected >=2 diagnostics artifacts, got %d", len(report.Diagnostics))
	}
	if _, err := os.Stat(report.ReportFile); err != nil {
		t.Fatalf("stat report: %v", err)
	}
	content, _ := os.ReadFile(report.ReportFile)
	if !strings.Contains(string(content), "诊断快照") {
		t.Fatalf("report content missing diagnostics section")
	}
	summaryPath := filepath.Join(cfg.OutputDir, report.RunID, "summary.json")
	raw, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	var parsed Report
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if parsed.ReportFile == "" || len(parsed.Diagnostics) == 0 {
		t.Fatalf("summary missing report metadata: %+v", parsed)
	}
}

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

func assertErr(s string) error { return fakeErr(s) }
