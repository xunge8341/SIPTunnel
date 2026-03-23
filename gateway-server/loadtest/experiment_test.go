package loadtest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseGatewayLogMetrics(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gateway.log")
	content := strings.Join([]string{"gb28181 relay stage=transaction_summary call_id=a gate_wait_ms=12 response_start_wait_ms=34 rtp_seq_gap_count=1 resume_count=2 final_status=ok", "gb28181 relay stage=response_start_timeout call_id=b", "gb28181 relay stage=bye_sent call_id=c"}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	metrics, err := ParseGatewayLogMetrics(path)
	if err != nil {
		t.Fatalf("ParseGatewayLogMetrics err=%v", err)
	}
	if metrics.TransactionCount != 1 || metrics.ResponseStartTimeouts != 1 || metrics.ByeCount != 1 || metrics.RTPSeqGapCount != 1 || metrics.ResumeCount != 2 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
}
func TestAnalyzeExperimentManifest(t *testing.T) {
	dir := t.TempDir()
	writeReport := func(name string, summary Summary) string {
		report := Report{RunID: name, Generated: time.Now().UTC(), Summaries: map[string]Summary{"mapping-forward": summary}}
		path := filepath.Join(dir, name+"_summary.json")
		raw, _ := json.Marshal(report)
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	writeLog := func(name, content string) string {
		path := filepath.Join(dir, name+".log")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	manifest := ExperimentManifest{Kind: "task9_task10", GeneratedAt: time.Now().UTC(), Runs: []ExperimentRun{{Name: "ka-off-3", Variant: "keepalive_workaround_off", Scenario: "keepalive_ab", Concurrency: 3, SummaryFile: writeReport("kaoff", Summary{SuccessRate: 1, FirstByteP95MS: 120, ConnectP95MS: 12, NewConnCount: 1}), GatewayLogFile: writeLog("kaoff", "gb28181 relay stage=transaction_summary gate_wait_ms=10 response_start_wait_ms=20")}, {Name: "ka-on-3", Variant: "keepalive_workaround_on", Scenario: "keepalive_ab", Concurrency: 3, SummaryFile: writeReport("kaon", Summary{SuccessRate: 0.999, FirstByteP95MS: 200, ConnectP95MS: 35, NewConnCount: 20}), GatewayLogFile: writeLog("kaon", "gb28181 relay stage=transaction_summary gate_wait_ms=20 response_start_wait_ms=50\ngb28181 relay stage=response_start_timeout")}, {Name: "cap-auto-3", Variant: "budget_auto", Scenario: "small_page_data", Concurrency: 3, SummaryFile: writeReport("capauto", Summary{SuccessRate: 1, FirstByteP95MS: 150}), GatewayLogFile: writeLog("capauto", "gb28181 relay stage=transaction_summary gate_wait_ms=40 response_start_wait_ms=60")}, {Name: "cap-rtp-5", Variant: "hardcoded_rtp", Scenario: "small_page_data", Concurrency: 5, SummaryFile: writeReport("caprtp", Summary{SuccessRate: 0.99, FirstByteP95MS: 900}), GatewayLogFile: writeLog("caprtp", "gb28181 relay stage=transaction_summary gate_wait_ms=400 response_start_wait_ms=700\ngb28181 relay stage=response_start_timeout")}}}
	manifestPath := filepath.Join(dir, "manifest.json")
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	analysis, err := AnalyzeExperimentManifest(manifestPath)
	if err != nil {
		t.Fatalf("AnalyzeExperimentManifest err=%v", err)
	}
	if analysis.KeepAliveAB == nil || analysis.KeepAliveAB.PreferredVariant != "keepalive_workaround_off" {
		t.Fatalf("unexpected keepalive analysis: %+v", analysis.KeepAliveAB)
	}
	if analysis.Capacity == nil || len(analysis.Capacity.Rows) == 0 {
		t.Fatalf("missing capacity analysis: %+v", analysis.Capacity)
	}
}
