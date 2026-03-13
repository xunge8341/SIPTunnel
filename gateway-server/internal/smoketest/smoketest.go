package smoketest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type CheckResult struct {
	Name     string
	Passed   bool
	Duration time.Duration
	Detail   string
}

type Options struct {
	BaseURL    string
	ConfigPath string
	Timeout    time.Duration
	Client     *http.Client
	Now        func() time.Time
}

type SuiteResult struct {
	StartedAt time.Time
	Duration  time.Duration
	Results   []CheckResult
}

func (s SuiteResult) Passed() bool {
	for _, item := range s.Results {
		if !item.Passed {
			return false
		}
	}
	return true
}

func Run(ctx context.Context, opts Options) SuiteResult {
	if opts.Timeout <= 0 {
		opts.Timeout = 3 * time.Second
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Client == nil {
		opts.Client = &http.Client{Timeout: opts.Timeout}
	}
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	started := opts.Now()

	results := []CheckResult{
		runNamedCheck("配置加载校验", func() (bool, string) {
			return checkConfig(ctx, opts.ConfigPath)
		}),
		runNamedCheck("自检接口", func() (bool, string) {
			return checkSelfCheck(ctx, opts.Client, baseURL)
		}),
		runNamedCheck("SIP listener", func() (bool, string) {
			return checkSIPListener(ctx, opts.Client, baseURL)
		}),
		runNamedCheck("RTP listener", func() (bool, string) {
			return checkRTPListener(ctx, opts.Client, baseURL)
		}),
		runNamedCheck("UI/API 可访问", func() (bool, string) {
			return checkUIAndAPI(ctx, opts.Client, baseURL)
		}),
		runNamedCheck("首启摘要", func() (bool, string) {
			return checkFirstStartSummary(ctx, opts.Client, baseURL)
		}),
		runNamedCheck("最小 command 链路", func() (bool, string) {
			return checkCommandChain(ctx, opts.Client, baseURL)
		}),
	}

	return SuiteResult{StartedAt: started, Duration: time.Since(started), Results: results}
}

func runNamedCheck(name string, fn func() (bool, string)) CheckResult {
	started := time.Now()
	passed, detail := fn()
	return CheckResult{Name: name, Passed: passed, Detail: detail, Duration: time.Since(started)}
}

func checkConfig(ctx context.Context, configPath string) (bool, string) {
	if strings.TrimSpace(configPath) == "" {
		return false, "缺少 --config"
	}
	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/gateway", "validate-config", "-f", configPath)
	cmd.Dir = "."
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return false, msg
	}
	return true, fmt.Sprintf("config=%s", configPath)
}

func checkSelfCheck(ctx context.Context, client *http.Client, baseURL string) (bool, string) {
	body, code, err := getJSON(ctx, client, baseURL+"/api/selfcheck")
	if err != nil {
		return false, err.Error()
	}
	if code >= 400 {
		return false, fmt.Sprintf("status=%d", code)
	}
	level := jsonPathString(body, "data", "overall")
	if strings.EqualFold(level, "error") {
		return false, "overall=error"
	}
	if level == "" {
		return false, "missing data.overall"
	}
	return true, fmt.Sprintf("overall=%s", level)
}

func checkSIPListener(ctx context.Context, client *http.Client, baseURL string) (bool, string) {
	status, err := fetchNetworkStatus(ctx, client, baseURL)
	if err != nil {
		return false, err.Error()
	}
	transport := strings.ToUpper(status.Data.SIP.Transport)
	if status.Data.SIP.ListenPort <= 0 {
		return false, "sip.listen_port invalid"
	}
	if transport == "TCP" {
		host := status.Data.SIP.ListenIP
		if host == "" || host == "0.0.0.0" {
			host = "127.0.0.1"
		}
		addr := net.JoinHostPort(host, fmt.Sprintf("%d", status.Data.SIP.ListenPort))
		conn, err := (&net.Dialer{Timeout: 800 * time.Millisecond}).DialContext(ctx, "tcp", addr)
		if err != nil {
			return false, fmt.Sprintf("tcp dial failed: %v", err)
		}
		_ = conn.Close()
		return true, fmt.Sprintf("%s %s", transport, addr)
	}
	if transport == "UDP" {
		return true, fmt.Sprintf("UDP listen_port=%d", status.Data.SIP.ListenPort)
	}
	return false, fmt.Sprintf("unsupported transport=%s", status.Data.SIP.Transport)
}

func checkRTPListener(ctx context.Context, client *http.Client, baseURL string) (bool, string) {
	status, err := fetchNetworkStatus(ctx, client, baseURL)
	if err != nil {
		return false, err.Error()
	}
	if status.Data.RTP.PortStart <= 0 || status.Data.RTP.PortEnd <= 0 || status.Data.RTP.PortStart > status.Data.RTP.PortEnd {
		return false, "invalid rtp port range"
	}
	if status.Data.RTP.AvailablePorts <= 0 {
		return false, "rtp port pool exhausted"
	}
	return true, fmt.Sprintf("%s %d-%d available=%d", strings.ToUpper(status.Data.RTP.Transport), status.Data.RTP.PortStart, status.Data.RTP.PortEnd, status.Data.RTP.AvailablePorts)
}

func checkUIAndAPI(ctx context.Context, client *http.Client, baseURL string) (bool, string) {
	_, code, err := getJSON(ctx, client, baseURL+"/healthz")
	if err != nil || code >= 400 {
		if err != nil {
			return false, err.Error()
		}
		return false, fmt.Sprintf("healthz status=%d", code)
	}
	body, code, err := getJSON(ctx, client, baseURL+"/api/startup-summary")
	if err != nil {
		return false, err.Error()
	}
	if code >= 400 {
		return false, fmt.Sprintf("startup-summary status=%d", code)
	}
	uiMode := strings.ToLower(jsonPathString(body, "data", "ui_mode"))
	uiURL := jsonPathString(body, "data", "ui_url")
	if uiMode == "embedded" && strings.TrimSpace(uiURL) != "" {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, uiURL, nil)
		resp, err := client.Do(req)
		if err != nil {
			return false, fmt.Sprintf("ui unreachable: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return false, fmt.Sprintf("ui status=%d", resp.StatusCode)
		}
		return true, fmt.Sprintf("api=ok ui=embedded %s", uiURL)
	}
	return true, fmt.Sprintf("api=ok ui_mode=%s", uiMode)
}

func checkFirstStartSummary(ctx context.Context, client *http.Client, baseURL string) (bool, string) {
	body, code, err := getJSON(ctx, client, baseURL+"/api/startup-summary")
	if err != nil {
		return false, err.Error()
	}
	if code >= 400 {
		return false, fmt.Sprintf("startup-summary status=%d", code)
	}
	mode := strings.TrimSpace(jsonPathString(body, "data", "run_mode"))
	configPath := strings.TrimSpace(jsonPathString(body, "data", "config_path"))
	configSource := strings.TrimSpace(jsonPathString(body, "data", "config_source"))
	if mode == "" || configPath == "" || configSource == "" {
		return false, "missing run_mode/config_path/config_source"
	}
	return true, fmt.Sprintf("mode=%s source=%s", mode, configSource)
}

func checkCommandChain(ctx context.Context, client *http.Client, baseURL string) (bool, string) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/demo/process", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Code", "ops.smoke")
	req.Header.Set("X-Request-ID", "smoke-request")
	req.Header.Set("X-Trace-ID", "smoke-trace")
	req.Header.Set("X-Message-Type", "command")
	req.Header.Set("X-Source-System", "ops-smoke")
	resp, err := client.Do(req)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return false, fmt.Sprintf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	var parsed map[string]any
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return false, fmt.Sprintf("invalid json: %v", err)
	}
	code, _ := parsed["code"].(string)
	if strings.ToUpper(code) != "OK" {
		return false, fmt.Sprintf("unexpected code=%s", code)
	}
	return true, "demo/process 返回 OK"
}

type networkStatusEnvelope struct {
	Data struct {
		SIP struct {
			ListenIP   string `json:"listen_ip"`
			ListenPort int    `json:"listen_port"`
			Transport  string `json:"transport"`
		} `json:"sip"`
		RTP struct {
			PortStart      int    `json:"port_start"`
			PortEnd        int    `json:"port_end"`
			Transport      string `json:"transport"`
			AvailablePorts int    `json:"available_ports"`
		} `json:"rtp"`
	} `json:"data"`
}

func fetchNetworkStatus(ctx context.Context, client *http.Client, baseURL string) (networkStatusEnvelope, error) {
	var out networkStatusEnvelope
	body, code, err := getJSON(ctx, client, baseURL+"/api/node/network-status")
	if err != nil {
		return out, err
	}
	if code >= 400 {
		return out, fmt.Errorf("network-status status=%d", code)
	}
	raw, _ := json.Marshal(body)
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("decode network status: %w", err)
	}
	return out, nil
}

func getJSON(ctx context.Context, client *http.Client, url string) (map[string]any, int, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	var out map[string]any
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("decode %s: %w", url, err)
	}
	return out, resp.StatusCode, nil
}

func jsonPathString(m map[string]any, path ...string) string {
	cur := any(m)
	for _, seg := range path {
		next, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur, ok = next[seg]
		if !ok {
			return ""
		}
	}
	s, _ := cur.(string)
	return s
}

func FormatSummary(result SuiteResult) string {
	var b strings.Builder
	status := "PASS"
	if !result.Passed() {
		status = "FAIL"
	}
	fmt.Fprintf(&b, "\\nSmoke Test Summary [%s] total=%d duration=%s\\n", status, len(result.Results), result.Duration.Round(time.Millisecond))
	for _, item := range result.Results {
		mark := "[PASS]"
		if !item.Passed {
			mark = "[FAIL]"
		}
		fmt.Fprintf(&b, "  %s %-18s %s (%s)\\n", mark, item.Name, item.Detail, item.Duration.Round(time.Millisecond))
	}
	failed := make([]string, 0)
	for _, item := range result.Results {
		if !item.Passed {
			failed = append(failed, item.Name)
		}
	}
	sort.Strings(failed)
	if len(failed) > 0 {
		fmt.Fprintf(&b, "Failed checks: %s\\n", strings.Join(failed, ", "))
	}
	return b.String()
}
