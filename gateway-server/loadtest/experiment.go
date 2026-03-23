package loadtest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ExperimentManifest struct {
	Kind        string          `json:"kind"`
	GeneratedAt time.Time       `json:"generated_at"`
	Runs        []ExperimentRun `json:"runs"`
}
type ExperimentRun struct {
	Name           string `json:"name"`
	Variant        string `json:"variant"`
	Scenario       string `json:"scenario"`
	Concurrency    int    `json:"concurrency"`
	SummaryFile    string `json:"summary_file"`
	GatewayLogFile string `json:"gateway_log_file,omitempty"`
}
type GatewayLogMetrics struct {
	TransactionCount       int64   `json:"transaction_count"`
	ResponseStartTimeouts  int64   `json:"response_start_timeouts"`
	RTPSeqGapCount         int64   `json:"rtp_seq_gap_count"`
	ResumeCount            int64   `json:"resume_count"`
	ByeCount               int64   `json:"bye_count"`
	GateWaitP95MS          float64 `json:"gate_wait_p95_ms"`
	GateWaitP99MS          float64 `json:"gate_wait_p99_ms"`
	ResponseStartWaitP95MS float64 `json:"response_start_wait_p95_ms"`
	ResponseStartWaitP99MS float64 `json:"response_start_wait_p99_ms"`
}
type ExperimentObservation struct {
	Name        string            `json:"name"`
	Variant     string            `json:"variant"`
	Scenario    string            `json:"scenario"`
	Concurrency int               `json:"concurrency"`
	Summary     Summary           `json:"summary"`
	Gateway     GatewayLogMetrics `json:"gateway"`
}
type KeepAliveABReport struct {
	PreferredVariant string             `json:"preferred_variant"`
	VariantScores    map[string]float64 `json:"variant_scores"`
	Rationale        []string           `json:"rationale"`
}
type CapacityRow struct {
	Scenario              string  `json:"scenario"`
	Variant               string  `json:"variant"`
	Concurrency           int     `json:"concurrency"`
	SuccessRate           float64 `json:"success_rate"`
	FirstByteP95MS        float64 `json:"first_byte_p95_ms"`
	GateWaitP95MS         float64 `json:"gate_wait_p95_ms"`
	ResponseStartTimeouts int64   `json:"response_start_timeouts"`
	RTPSeqGapCount        int64   `json:"rtp_seq_gap_count"`
	ResumeCount           int64   `json:"resume_count"`
	ByeCount              int64   `json:"bye_count"`
	Stable                bool    `json:"stable"`
	DecisionReason        string  `json:"decision_reason"`
}
type CapacityCeiling struct {
	Scenario                 string `json:"scenario"`
	Variant                  string `json:"variant"`
	StableConcurrencyCeiling int    `json:"stable_concurrency_ceiling"`
	DecisionReason           string `json:"decision_reason"`
}
type CapacityBaselineReport struct {
	Rows     []CapacityRow     `json:"rows"`
	Ceilings []CapacityCeiling `json:"ceilings"`
}
type ExperimentAnalysis struct {
	Manifest     ExperimentManifest      `json:"manifest"`
	Observations []ExperimentObservation `json:"observations"`
	KeepAliveAB  *KeepAliveABReport      `json:"keep_alive_ab,omitempty"`
	Capacity     *CapacityBaselineReport `json:"capacity,omitempty"`
}

func AnalyzeExperimentManifest(path string) (ExperimentAnalysis, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ExperimentAnalysis{}, fmt.Errorf("read manifest: %w", err)
	}
	var manifest ExperimentManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return ExperimentAnalysis{}, fmt.Errorf("decode manifest: %w", err)
	}
	obs := make([]ExperimentObservation, 0, len(manifest.Runs))
	for _, run := range manifest.Runs {
		report, err := LoadReportFromSummary(run.SummaryFile)
		if err != nil {
			return ExperimentAnalysis{}, fmt.Errorf("load summary for %s: %w", run.Name, err)
		}
		gatewayMetrics, err := ParseGatewayLogMetrics(run.GatewayLogFile)
		if err != nil {
			return ExperimentAnalysis{}, fmt.Errorf("parse gateway log for %s: %w", run.Name, err)
		}
		obs = append(obs, ExperimentObservation{Name: run.Name, Variant: strings.TrimSpace(run.Variant), Scenario: strings.TrimSpace(run.Scenario), Concurrency: run.Concurrency, Summary: chooseRelevantSummary(report), Gateway: gatewayMetrics})
	}
	analysis := ExperimentAnalysis{Manifest: manifest, Observations: obs}
	if hasKeepAliveScenario(obs) {
		v := analyzeKeepAliveAB(obs)
		analysis.KeepAliveAB = &v
	}
	if hasCapacityScenarios(obs) {
		v := analyzeCapacityBaseline(obs)
		analysis.Capacity = &v
	}
	return analysis, nil
}
func RenderExperimentMarkdown(analysis ExperimentAnalysis) string {
	b := &strings.Builder{}
	b.WriteString("# Task 9 / Task 10 实验分析报告\n\n")
	b.WriteString("## 观测样本\n| Name | Variant | Scenario | 并发 | 成功率 | 首字节P95(ms) | 建连P95(ms) | gate wait P95(ms) | response_start_timeout | RTP seq gap | resume | BYE |\n|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, obs := range analysis.Observations {
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %d | %.2f%% | %.2f | %.2f | %.2f | %d | %d | %d | %d |\n", obs.Name, obs.Variant, obs.Scenario, obs.Concurrency, obs.Summary.SuccessRate*100, obs.Summary.FirstByteP95MS, obs.Summary.ConnectP95MS, obs.Gateway.GateWaitP95MS, obs.Gateway.ResponseStartTimeouts, obs.Gateway.RTPSeqGapCount, obs.Gateway.ResumeCount, obs.Gateway.ByeCount))
	}
	if analysis.KeepAliveAB != nil {
		b.WriteString("\n## Task 9 A/B 结论\n- 推荐变体: `" + analysis.KeepAliveAB.PreferredVariant + "`\n")
		for _, reason := range analysis.KeepAliveAB.Rationale {
			b.WriteString("- " + reason + "\n")
		}
	}
	if analysis.Capacity != nil {
		b.WriteString("\n## Task 10 容量基线\n| Scenario | Variant | 并发 | Stable | Decision |\n|---|---|---:|---|---|\n")
		for _, row := range analysis.Capacity.Rows {
			stable := "no"
			if row.Stable {
				stable = "yes"
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %d | %s | %s |\n", row.Scenario, row.Variant, row.Concurrency, stable, row.DecisionReason))
		}
		b.WriteString("\n### 稳定并发上限\n")
		for _, ceiling := range analysis.Capacity.Ceilings {
			b.WriteString(fmt.Sprintf("- %s / %s: %d (%s)\n", ceiling.Scenario, ceiling.Variant, ceiling.StableConcurrencyCeiling, ceiling.DecisionReason))
		}
	}
	return b.String()
}

var kvPattern = regexp.MustCompile(`([A-Za-z0-9_]+)=([^\s]+)`)

func ParseGatewayLogMetrics(path string) (GatewayLogMetrics, error) {
	if strings.TrimSpace(path) == "" {
		return GatewayLogMetrics{}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return GatewayLogMetrics{}, fmt.Errorf("read gateway log: %w", err)
	}
	var gateWaits, responseStartWaits []float64
	metrics := GatewayLogMetrics{}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := parseKVFields(line)
		if fields["stage"] == "response_start_timeout" {
			metrics.ResponseStartTimeouts++
		}
		if fields["stage"] == "bye_sent" || fields["completion"] == "bye_idle" {
			metrics.ByeCount++
		}
		if fields["stage"] == "transaction_summary" {
			metrics.TransactionCount++
			metrics.RTPSeqGapCount += parseInt64(fields["rtp_seq_gap_count"])
			metrics.ResumeCount += parseInt64(fields["resume_count"])
			if v, ok := parseFloat(fields["gate_wait_ms"]); ok {
				gateWaits = append(gateWaits, v)
			}
			if v, ok := parseFloat(fields["response_start_wait_ms"]); ok {
				responseStartWaits = append(responseStartWaits, v)
			}
		}
	}
	sort.Float64s(gateWaits)
	sort.Float64s(responseStartWaits)
	metrics.GateWaitP95MS = percentile(gateWaits, 95)
	metrics.GateWaitP99MS = percentile(gateWaits, 99)
	metrics.ResponseStartWaitP95MS = percentile(responseStartWaits, 95)
	metrics.ResponseStartWaitP99MS = percentile(responseStartWaits, 99)
	return metrics, nil
}
func analyzeKeepAliveAB(obs []ExperimentObservation) KeepAliveABReport {
	buckets := map[string][]ExperimentObservation{}
	for _, item := range obs {
		if item.Variant != "" {
			buckets[item.Variant] = append(buckets[item.Variant], item)
		}
	}
	scores := map[string]float64{}
	for variant, items := range buckets {
		scores[variant] = keepAliveVariantScore(items)
	}
	return KeepAliveABReport{PreferredVariant: chooseKeepAlivePreferredVariant(scores), VariantScores: scores, Rationale: []string{"success_rate 权重最高，先保证变体不会把现场稳定性打穿。", "在成功率可接受的前提下，再比较 first_byte_p95 与 connect_p95，避免为了规避崩溃而把首包时延拉高。", "response_start_timeout 与 new_conn_count 被视为惩罚项，因为它们通常意味着 keep-alive 关闭后连接建立成本正在吞吐面显性化。"}}
}
func chooseKeepAlivePreferredVariant(scores map[string]float64) string {
	bestVariant := ""
	bestScore := -1e18
	for variant, score := range scores {
		if bestVariant == "" || score > bestScore {
			bestVariant, bestScore = variant, score
		}
	}
	return bestVariant
}

// keepAliveVariantScore deliberately weights success rate first. Task 9 is not
// about chasing a prettier connect latency curve at the cost of more failures;
// it is about proving whether the workaround is a throughput bottleneck after
// keeping the system stable. Once stability is preserved, first-byte latency,
// connect latency, response-start timeouts, and new connection count explain the
// throughput delta and therefore act as secondary penalties.
func keepAliveVariantScore(items []ExperimentObservation) float64 {
	if len(items) == 0 {
		return -1e18
	}
	var successRate, firstByteP95, connectP95, responseStartTimeouts, newConnCount float64
	for _, item := range items {
		successRate += item.Summary.SuccessRate
		firstByteP95 += item.Summary.FirstByteP95MS
		connectP95 += item.Summary.ConnectP95MS
		responseStartTimeouts += float64(item.Gateway.ResponseStartTimeouts)
		newConnCount += float64(item.Summary.NewConnCount)
	}
	n := float64(len(items))
	return successRate/n*1000 - firstByteP95/n - connectP95/n - responseStartTimeouts/n*200 - newConnCount/n*0.1
}
func analyzeCapacityBaseline(obs []ExperimentObservation) CapacityBaselineReport {
	rows := make([]CapacityRow, 0, len(obs))
	for _, item := range obs {
		stable, reason := evaluateCapacityRow(item)
		rows = append(rows, CapacityRow{Scenario: item.Scenario, Variant: item.Variant, Concurrency: item.Concurrency, SuccessRate: item.Summary.SuccessRate, FirstByteP95MS: item.Summary.FirstByteP95MS, GateWaitP95MS: item.Gateway.GateWaitP95MS, ResponseStartTimeouts: item.Gateway.ResponseStartTimeouts, RTPSeqGapCount: item.Gateway.RTPSeqGapCount, ResumeCount: item.Gateway.ResumeCount, ByeCount: item.Gateway.ByeCount, Stable: stable, DecisionReason: reason})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Scenario != rows[j].Scenario {
			return rows[i].Scenario < rows[j].Scenario
		}
		if rows[i].Variant != rows[j].Variant {
			return rows[i].Variant < rows[j].Variant
		}
		return rows[i].Concurrency < rows[j].Concurrency
	})
	return CapacityBaselineReport{Rows: rows, Ceilings: buildCapacityCeilings(rows)}
}

// evaluateCapacityRow uses scenario-specific thresholds because Task 10 compares
// small_page_data, socket.io polling, and bulk_download. These response images
// have different acceptable first-byte and gate-wait budgets. The baseline is
// only considered stable when success_rate stays above 99.5% and
// response_start_timeout remains zero; otherwise the run is not safe to treat as
// deployable capacity, even if average throughput still looks acceptable.
func evaluateCapacityRow(item ExperimentObservation) (bool, string) {
	firstByteLimit, gateWaitLimit := thresholdForScenario(item.Scenario)
	if item.Summary.SuccessRate < 0.995 {
		return false, fmt.Sprintf("success_rate %.4f < 0.995", item.Summary.SuccessRate)
	}
	if item.Gateway.ResponseStartTimeouts > 0 {
		return false, fmt.Sprintf("response_start_timeout=%d", item.Gateway.ResponseStartTimeouts)
	}
	if item.Summary.FirstByteP95MS > firstByteLimit {
		return false, fmt.Sprintf("first_byte_p95 %.2fms > %.2fms", item.Summary.FirstByteP95MS, firstByteLimit)
	}
	if item.Gateway.GateWaitP95MS > gateWaitLimit {
		return false, fmt.Sprintf("gate_wait_p95 %.2fms > %.2fms", item.Gateway.GateWaitP95MS, gateWaitLimit)
	}
	return true, "within success / first-byte / gate-wait budget"
}
func thresholdForScenario(scenario string) (float64, float64) {
	switch strings.TrimSpace(scenario) {
	case "socketio_polling":
		return 800, 250
	case "bulk_download":
		return 1500, 400
	default:
		return 500, 200
	}
}
func buildCapacityCeilings(rows []CapacityRow) []CapacityCeiling {
	maxStable := map[string]CapacityCeiling{}
	for _, row := range rows {
		key := row.Scenario + "|" + row.Variant
		current := maxStable[key]
		if row.Stable && row.Concurrency >= current.StableConcurrencyCeiling {
			maxStable[key] = CapacityCeiling{Scenario: row.Scenario, Variant: row.Variant, StableConcurrencyCeiling: row.Concurrency, DecisionReason: row.DecisionReason}
		} else if current.StableConcurrencyCeiling == 0 {
			maxStable[key] = CapacityCeiling{Scenario: row.Scenario, Variant: row.Variant, StableConcurrencyCeiling: 0, DecisionReason: row.DecisionReason}
		}
	}
	out := make([]CapacityCeiling, 0, len(maxStable))
	for _, item := range maxStable {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Scenario != out[j].Scenario {
			return out[i].Scenario < out[j].Scenario
		}
		return out[i].Variant < out[j].Variant
	})
	return out
}
func chooseRelevantSummary(report Report) Summary {
	for _, key := range []string{"mapping-forward", "http-invoke", "sip-command-create", "rtp-upload", "rtp-udp-upload", "rtp-tcp-upload"} {
		if s, ok := report.Summaries[key]; ok {
			return s
		}
	}
	for _, s := range report.Summaries {
		return s
	}
	return Summary{}
}
func hasKeepAliveScenario(obs []ExperimentObservation) bool {
	for _, item := range obs {
		if strings.Contains(item.Scenario, "keepalive") || strings.HasPrefix(item.Variant, "keepalive_") {
			return true
		}
	}
	return false
}
func hasCapacityScenarios(obs []ExperimentObservation) bool {
	for _, item := range obs {
		switch item.Scenario {
		case "small_page_data", "socketio_polling", "bulk_download":
			return true
		}
	}
	return false
}
func parseKVFields(line string) map[string]string {
	out := map[string]string{}
	for _, match := range kvPattern.FindAllStringSubmatch(line, -1) {
		if len(match) == 3 {
			out[match[1]] = strings.Trim(match[2], "\"`")
		}
	}
	return out
}
func parseInt64(raw string) int64 { v, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); return v }
func parseFloat(raw string) (float64, bool) {
	if strings.TrimSpace(raw) == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	return v, err == nil
}
func WriteExperimentMarkdown(path string, analysis ExperimentAnalysis) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(RenderExperimentMarkdown(analysis)), 0o644)
}
