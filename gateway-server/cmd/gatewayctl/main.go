package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"siptunnel/internal/config"
	"siptunnel/internal/repository"
	"siptunnel/internal/selfcheck"
	"siptunnel/internal/server"
)

const defaultBaseURL = "http://127.0.0.1:18080"

const (
	runbookDocPath = "docs/runbook.md"
	oncallDocPath  = "docs/oncall-handbook.md"
)

type cliOptions struct {
	baseURL string
	output  string
	timeout time.Duration
}

type apiEnvelope struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type taskListResponse struct {
	Items []repository.Task `json:"items"`
}

type diagBundle struct {
	GeneratedAt    time.Time                   `json:"generated_at"`
	GatewayBaseURL string                      `json:"gateway_base_url"`
	Health         map[string]any              `json:"health"`
	Node           server.NodeNetworkStatus    `json:"node"`
	SelfCheck      selfcheck.Report            `json:"self_check"`
	Limits         map[string]any              `json:"limits"`
	Routes         map[string]any              `json:"routes"`
	Export         server.DiagnosticExportData `json:"diagnostic_export"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	opts, rest, err := parseGlobalFlags(args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		printRootUsage(stderr)
		return errors.New("missing subcommand")
	}

	switch rest[0] {
	case "config":
		return runConfigCommand(rest[1:], opts, stdout, stderr)
	case "node":
		return runNodeCommand(rest[1:], opts, stdout, stderr)
	case "task":
		return runTaskCommand(rest[1:], opts, stdout, stderr)
	case "diag":
		return runDiagCommand(rest[1:], opts, stdout, stderr)
	case "help", "-h", "--help":
		printRootUsage(stdout)
		return nil
	default:
		printRootUsage(stderr)
		return fmt.Errorf("unknown subcommand %q", rest[0])
	}
}

func parseGlobalFlags(args []string) (cliOptions, []string, error) {
	fs := flag.NewFlagSet("gatewayctl", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	baseURL := fs.String("server", defaultBaseURL, "gateway API base URL")
	output := fs.String("output", "text", "output format: text|json")
	shortOutput := fs.String("o", "text", "output format shorthand: text|json")
	timeout := fs.Duration("timeout", 5*time.Second, "HTTP timeout")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return cliOptions{}, []string{"help"}, nil
		}
		return cliOptions{}, nil, err
	}
	resolvedOutput := strings.ToLower(strings.TrimSpace(*output))
	if fs.Lookup("o").Value.String() != "text" {
		resolvedOutput = strings.ToLower(strings.TrimSpace(*shortOutput))
	}
	if resolvedOutput != "text" && resolvedOutput != "json" {
		return cliOptions{}, nil, fmt.Errorf("unsupported output format %q, choose text or json", resolvedOutput)
	}
	return cliOptions{baseURL: strings.TrimRight(*baseURL, "/"), output: resolvedOutput, timeout: *timeout}, fs.Args(), nil
}

func runConfigCommand(args []string, opts cliOptions, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printConfigUsage(stderr)
		return errors.New("missing config subcommand")
	}
	if args[0] != "validate" {
		printConfigUsage(stderr)
		return fmt.Errorf("unknown config subcommand %q", args[0])
	}
	fs := flag.NewFlagSet("gatewayctl config validate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	file := fs.String("f", "", "config file path")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*file) == "" {
		return errors.New("-f is required, for example: gatewayctl config validate -f config.yaml")
	}
	result, err := validateConfig(*file)
	if err != nil {
		if opts.output == "json" {
			return writeJSON(stdout, map[string]any{"ok": false, "file": *file, "error": err.Error()})
		}
		return err
	}
	if opts.output == "json" {
		return writeJSON(stdout, result)
	}
	fmt.Fprintf(stdout, "配置校验通过: %s\n", result.File)
	fmt.Fprintf(stdout, "SIP=%s %s:%d, RTP=%s %s [%d,%d]\n", result.Network.SIP.Transport, result.Network.SIP.ListenIP, result.Network.SIP.ListenPort, result.Network.RTP.Transport, result.Network.RTP.ListenIP, result.Network.RTP.PortStart, result.Network.RTP.PortEnd)
	return nil
}

type validateConfigResult struct {
	OK      bool                 `json:"ok"`
	File    string               `json:"file"`
	Network config.NetworkConfig `json:"network"`
}

func validateConfig(filePath string) (validateConfigResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return validateConfigResult{}, fmt.Errorf("读取配置文件失败: %w", err)
	}
	var wrapped struct {
		Network config.NetworkConfig `yaml:"network"`
	}
	if err := yaml.Unmarshal(data, &wrapped); err != nil {
		return validateConfigResult{}, fmt.Errorf("解析 YAML 失败: %w", err)
	}
	wrapped.Network.ApplyDefaults()
	if err := wrapped.Network.Validate(); err != nil {
		return validateConfigResult{}, fmt.Errorf("配置校验失败: %w", err)
	}
	return validateConfigResult{OK: true, File: filePath, Network: wrapped.Network}, nil
}

func runNodeCommand(args []string, opts cliOptions, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "inspect" {
		printNodeUsage(stderr)
		if len(args) == 0 {
			return errors.New("missing node subcommand")
		}
		return fmt.Errorf("unknown node subcommand %q", args[0])
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()
	var status server.NodeNetworkStatus
	if err := getAPI(ctx, opts.baseURL, "/api/node/network-status", nil, &status); err != nil {
		return err
	}
	if opts.output == "json" {
		return writeJSON(stdout, status)
	}
	fmt.Fprintf(stdout, "Node Network Status (%s)\n", opts.baseURL)
	fmt.Fprintf(stdout, "SIP: %s %s:%d, sessions=%d, conns=%d\n", status.SIP.Transport, status.SIP.ListenIP, status.SIP.ListenPort, status.SIP.CurrentSessions, status.SIP.CurrentConnections)
	fmt.Fprintf(stdout, "RTP: %s %s [%d,%d], active=%d, used=%d/%d, alloc_fail=%d\n", status.RTP.Transport, status.RTP.ListenIP, status.RTP.PortStart, status.RTP.PortEnd, status.RTP.ActiveTransfers, status.RTP.PortPoolUsed, status.RTP.PortPoolTotal, status.RTP.PortAllocFailTotal)
	printStringList(stdout, "Recent Bind Errors", status.RecentBindErrors)
	printStringList(stdout, "Recent Network Errors", status.RecentNetworkErrors)
	return nil
}

func runTaskCommand(args []string, opts cliOptions, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "query" {
		printTaskUsage(stderr)
		if len(args) == 0 {
			return errors.New("missing task subcommand")
		}
		return fmt.Errorf("unknown task subcommand %q", args[0])
	}
	fs := flag.NewFlagSet("gatewayctl task query", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	requestID := fs.String("request-id", "", "request id")
	traceID := fs.String("trace-id", "", "trace id")
	status := fs.String("status", "", "task status")
	page := fs.Int("page", 1, "page number")
	pageSize := fs.Int("page-size", 20, "page size")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*requestID) == "" && strings.TrimSpace(*traceID) == "" {
		return errors.New("请至少提供 --request-id 或 --trace-id 用于查询")
	}
	params := map[string]string{"page": fmt.Sprintf("%d", *page), "page_size": fmt.Sprintf("%d", *pageSize)}
	if strings.TrimSpace(*requestID) != "" {
		params["request_id"] = *requestID
	}
	if strings.TrimSpace(*traceID) != "" {
		params["trace_id"] = *traceID
	}
	if strings.TrimSpace(*status) != "" {
		params["status"] = *status
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()
	var result taskListResponse
	if err := getAPI(ctx, opts.baseURL, "/api/tasks", params, &result); err != nil {
		return err
	}
	if opts.output == "json" {
		return writeJSON(stdout, result)
	}
	fmt.Fprintf(stdout, "任务查询结果: %d 条\n", len(result.Items))
	for _, task := range result.Items {
		fmt.Fprintf(stdout, "- id=%s request_id=%s status=%s attempts=%d updated_at=%s\n", task.ID, task.RequestID, task.Status, task.Attempt, task.UpdatedAt.Format(time.RFC3339))
	}
	if len(result.Items) == 0 {
		fmt.Fprintln(stdout, "(无匹配任务)")
	}
	return nil
}

func runDiagCommand(args []string, opts cliOptions, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "export" {
		printDiagUsage(stderr)
		if len(args) == 0 {
			return errors.New("missing diag subcommand")
		}
		return fmt.Errorf("unknown diag subcommand %q", args[0])
	}
	fs := flag.NewFlagSet("gatewayctl diag export", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	outFile := fs.String("out", "", "write diagnostic JSON to file")
	requestID := fs.String("request-id", "", "filter diagnostics by request_id")
	traceID := fs.String("trace-id", "", "filter diagnostics by trace_id")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()
	bundle, err := collectDiagnostics(ctx, opts.baseURL, strings.TrimSpace(*requestID), strings.TrimSpace(*traceID))
	if err != nil {
		return err
	}
	payload, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	if strings.TrimSpace(*outFile) != "" {
		if err := os.WriteFile(*outFile, payload, 0o644); err != nil {
			return fmt.Errorf("写入诊断文件失败: %w", err)
		}
	}
	if opts.output == "json" {
		if _, err := stdout.Write(payload); err != nil {
			return err
		}
		fmt.Fprintln(stdout)
		return nil
	}
	fmt.Fprintf(stdout, "诊断导出完成: generated_at=%s server=%s\n", bundle.GeneratedAt.Format(time.RFC3339), bundle.GatewayBaseURL)
	fmt.Fprintf(stdout, "Health: %v\n", bundle.Health)
	fmt.Fprintf(stdout, "Self-check overall=%s (info=%d warn=%d error=%d)\n", bundle.SelfCheck.Overall, bundle.SelfCheck.Summary.Info, bundle.SelfCheck.Summary.Warn, bundle.SelfCheck.Summary.Error)
	fmt.Fprintf(stdout, "Node SIP=%s:%d/%s RTP=[%d,%d]/%s\n", bundle.Node.SIP.ListenIP, bundle.Node.SIP.ListenPort, bundle.Node.SIP.Transport, bundle.Node.RTP.PortStart, bundle.Node.RTP.PortEnd, bundle.Node.RTP.Transport)
	if strings.TrimSpace(*outFile) != "" {
		fmt.Fprintf(stdout, "诊断文件已写入: %s\n", *outFile)
	}
	return nil
}

func collectDiagnostics(ctx context.Context, baseURL, requestID, traceID string) (diagBundle, error) {
	bundle := diagBundle{GeneratedAt: time.Now().UTC(), GatewayBaseURL: baseURL}
	if err := getAPI(ctx, baseURL, "/healthz", nil, &bundle.Health); err != nil {
		return diagBundle{}, fmt.Errorf("获取 healthz 失败: %w", err)
	}
	if err := getAPI(ctx, baseURL, "/api/selfcheck", nil, &bundle.SelfCheck); err != nil {
		return diagBundle{}, fmt.Errorf("获取 selfcheck 失败: %w", err)
	}
	if err := getAPI(ctx, baseURL, "/api/node/network-status", nil, &bundle.Node); err != nil {
		return diagBundle{}, fmt.Errorf("获取 node network status 失败: %w", err)
	}
	if err := getAPI(ctx, baseURL, "/api/limits", nil, &bundle.Limits); err != nil {
		return diagBundle{}, fmt.Errorf("获取 limits 失败: %w", err)
	}
	if err := getAPI(ctx, baseURL, "/api/routes", nil, &bundle.Routes); err != nil {
		return diagBundle{}, fmt.Errorf("获取 routes 失败: %w", err)
	}
	query := map[string]string{}
	if requestID != "" {
		query["request_id"] = requestID
	}
	if traceID != "" {
		query["trace_id"] = traceID
	}
	if err := getAPI(ctx, baseURL, "/api/diagnostics/export", query, &bundle.Export); err != nil {
		return diagBundle{}, fmt.Errorf("获取 diagnostics export 失败: %w", err)
	}
	return bundle, nil
}

func getAPI(ctx context.Context, baseURL, path string, query map[string]string, out any) error {
	u, err := url.Parse(baseURL + path)
	if err != nil {
		return fmt.Errorf("构建请求地址失败: %w", err)
	}
	if len(query) > 0 {
		q := u.Query()
		keys := make([]string, 0, len(query))
		for k := range query {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			q.Set(k, query[k])
		}
		u.RawQuery = q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("构建请求失败: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var env apiEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}
	if strings.ToUpper(env.Code) != "OK" {
		return fmt.Errorf("API 返回错误 code=%s message=%s", env.Code, env.Message)
	}
	if out == nil || bytes.Equal(env.Data, []byte("null")) || len(env.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("解析 data 字段失败: %w", err)
	}
	return nil
}

func writeJSON(w io.Writer, payload any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func printStringList(w io.Writer, title string, values []string) {
	fmt.Fprintf(w, "%s: ", title)
	if len(values) == 0 {
		fmt.Fprintln(w, "none")
		return
	}
	fmt.Fprintln(w)
	for _, item := range values {
		fmt.Fprintf(w, "  - %s\n", item)
	}
}

func printRootUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: gatewayctl [--server URL] [--output text|json] <command>")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  config validate -f <config.yaml>")
	fmt.Fprintln(w, "  node inspect")
	fmt.Fprintln(w, "  task query --request-id <id>")
	fmt.Fprintln(w, "  diag export [--request-id <id>] [--trace-id <id>] [--out diagnostics.json]")
	fmt.Fprintf(w, "Docs: %s (Runbook), %s (On-call)\n", runbookDocPath, oncallDocPath)
}

func printConfigUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: gatewayctl config validate -f <config.yaml>")
}

func printNodeUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: gatewayctl node inspect")
}

func printTaskUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: gatewayctl task query --request-id <id> [--trace-id <id>] [--status <status>]")
}

func printDiagUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: gatewayctl diag export [--request-id <id>] [--trace-id <id>] [--out diagnostics.json]")
}
