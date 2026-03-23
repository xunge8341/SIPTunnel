package loadtest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeGB28181GatewayLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gateway.log")
	content := "2026/03/20 gb28181 relay stage=transaction_summary call_id=a gate_wait_ms=10 response_start_wait_ms=100 rtp_seq_gap_count=2 resume_count=1 final_bytes=1024 final_status=200\n" +
		"2026/03/20 gb28181 relay stage=transaction_summary call_id=b gate_wait_ms=20 response_start_wait_ms=150 rtp_seq_gap_count=0 resume_count=0 final_bytes=2048 final_status=200\n" +
		"2026/03/20 gb28181 relay stage=response_start_timeout call_id=c\n" +
		"2026/03/20 gb28181 relay stage=bye_received call_id=a\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	metrics, err := AnalyzeGB28181GatewayLog(path)
	if err != nil {
		t.Fatalf("AnalyzeGB28181GatewayLog: %v", err)
	}
	if metrics.TransactionCount != 2 || metrics.ResponseStartTimeouts != 1 || metrics.ByeReceivedCount != 1 {
		t.Fatalf("unexpected counts: %+v", metrics)
	}
	if metrics.GateWaitP95MS != 10 || metrics.ResponseStartP95MS != 100 {
		t.Fatalf("unexpected percentiles: %+v", metrics)
	}
}

func TestCompareGB28181Metrics(t *testing.T) {
	base := GB28181LogMetrics{TransactionCount: 10, ResponseStartTimeouts: 2, GateWaitP95MS: 50}
	cand := GB28181LogMetrics{TransactionCount: 12, ResponseStartTimeouts: 1, GateWaitP95MS: 40}
	cmp := CompareGB28181Metrics(base, cand)
	if cmp.Delta.TransactionCount != 2 || cmp.Delta.ResponseStartTimeouts != -1 || cmp.Delta.GateWaitP95MS != -10 {
		t.Fatalf("unexpected delta: %+v", cmp)
	}
}
