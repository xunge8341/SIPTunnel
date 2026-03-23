package loadtest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func loadtestStopGrace(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return loadtestStopGraceFloor
	}
	grace := timeout / 10
	if grace < loadtestStopGraceFloor {
		grace = loadtestStopGraceFloor
	}
	if grace > loadtestStopGraceCeiling {
		grace = loadtestStopGraceCeiling
	}
	return grace
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
		b.WriteString(fmt.Sprintf("| %s | %.2f | %.2f%% | %.2f | %.2f | %.2f | %.2f | %d | %d | %v |\n", target, s.Throughput, s.SuccessRate*100, s.P50MS, s.P95MS, s.FirstByteP95MS, s.ConnectP95MS, s.NewConnCount, s.ReusedConnCount, s.ErrorTypes))
		if len(s.ErrorSamples) > 0 {
			b.WriteString("  - sample errors: ")
			for i, sample := range s.ErrorSamples {
				if i > 0 {
					b.WriteString(" ; ")
				}
				b.WriteString(sample.Type + ": " + sample.Detail)
			}
			b.WriteString("\n")
		}
		if s.Success == 0 && s.Total > 0 {
			b.WriteString("  - note: all requests failed; this run cannot be used as a performance baseline until the target path returns successful forwards.\n")
		}
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
