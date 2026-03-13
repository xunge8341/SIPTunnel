package main

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"siptunnel/internal/config"
	"siptunnel/internal/selfcheck"
)

func TestRunMainSkipsStartupForToolCommands(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`network:
  sip:
    listen_port: 15060
`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "init-config", args: []string{"init-config", "--config", filepath.Join(workspace, "generated.yaml")}},
		{name: "print-default-config", args: []string{"print-default-config"}},
		{name: "validate-config", args: []string{"validate-config", "-f", configPath}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			startupCalled := false
			err := runMain(tc.args, func(_ []string) {
				startupCalled = true
			})
			if err != nil {
				t.Fatalf("runMain() error=%v", err)
			}
			if startupCalled {
				t.Fatalf("runMain() should skip startup for %s", tc.name)
			}
		})
	}
}

func TestRunMainRunsStartupForServerMode(t *testing.T) {
	startupCalled := false
	err := runMain([]string{"--config", "./configs/config.yaml"}, func(args []string) {
		startupCalled = true
		if len(args) != 2 || args[0] != "--config" {
			t.Fatalf("startup args=%v", args)
		}
	})
	if err != nil {
		t.Fatalf("runMain() error=%v", err)
	}
	if !startupCalled {
		t.Fatal("expected startup to be called")
	}
}

func TestReadPort(t *testing.T) {
	t.Setenv("GATEWAY_PORT", "")
	if got := readPort("prod", "linux"); got != "18080" {
		t.Fatalf("readPort() default = %s, want 18080", got)
	}

	t.Setenv("GATEWAY_PORT", "19090")
	if got := readPort("dev", "linux"); got != "19090" {
		t.Fatalf("readPort() with env = %s, want 19090", got)
	}

	t.Setenv("GATEWAY_PORT", "abc")
	if got := readPort("dev", "linux"); got == "" {
		t.Fatal("readPort() with invalid env should fallback to friendly port")
	}
}

func TestShouldBlockStartupOnSelfCheckError(t *testing.T) {
	errorReport := selfcheck.Report{Overall: selfcheck.LevelError}
	warnReport := selfcheck.Report{Overall: selfcheck.LevelWarn}

	if !shouldBlockStartupOnSelfCheckError(errorReport, "prod") {
		t.Fatal("expected prod mode to block startup on self-check error")
	}
	if shouldBlockStartupOnSelfCheckError(errorReport, "dev") {
		t.Fatal("expected dev mode not to block startup on self-check error")
	}
	if shouldBlockStartupOnSelfCheckError(errorReport, "test") {
		t.Fatal("expected test mode not to block startup on self-check error")
	}
	if shouldBlockStartupOnSelfCheckError(warnReport, "prod") {
		t.Fatal("expected non-error report not to block startup")
	}
}

func TestResolveHTTPListenAddr(t *testing.T) {
	addr := resolveHTTPListenAddr("18080", config.UIConfig{Enabled: true, Mode: "embedded", ListenIP: "0.0.0.0", ListenPort: 19090})
	if addr != "0.0.0.0:19090" {
		t.Fatalf("resolveHTTPListenAddr embedded=%q, want 0.0.0.0:19090", addr)
	}
	addr = resolveHTTPListenAddr("18080", config.UIConfig{Enabled: true, Mode: "external", ListenIP: "0.0.0.0", ListenPort: 19090})
	if addr != ":18080" {
		t.Fatalf("resolveHTTPListenAddr external=%q, want :18080", addr)
	}
}

func TestBuildStartupSummary(t *testing.T) {
	summary := buildStartupSummary(
		"gateway-a-01",
		configLoadResult{Path: "./configs/config.yaml", Source: configSourceCLI},
		config.UIConfig{Enabled: true, Mode: "embedded", ListenIP: "0.0.0.0", ListenPort: 18080, BasePath: "/ops"},
		config.NetworkConfig{SIP: config.SIPConfig{Transport: "TCP", ListenIP: "0.0.0.0", ListenPort: 15060}, RTP: config.RTPConfig{ListenIP: "0.0.0.0", PortStart: 16000, PortEnd: 16020}},
		config.StoragePaths{TempDir: "./data/temp", FinalDir: "./data/final", AuditDir: "./data/audit", LogDir: "./data/logs"},
		"18080",
		"udp",
		selfCheckReportForTest(),
		0,
		"dev",
	)
	if summary.NodeID != "gateway-a-01" {
		t.Fatalf("node_id=%q", summary.NodeID)
	}
	if summary.ConfigPath != "./configs/config.yaml" || summary.ConfigSource != string(configSourceCLI) {
		t.Fatalf("config summary mismatch: %+v", summary)
	}
	if summary.UIURL != "http://127.0.0.1:18080/ops" {
		t.Fatalf("ui_url=%q", summary.UIURL)
	}
	if summary.APIURL != "http://127.0.0.1:18080/api" {
		t.Fatalf("api_url=%q", summary.APIURL)
	}
	if summary.SIPListen.Transport != "TCP" || summary.SIPListen.IP != "0.0.0.0" || summary.SIPListen.Port != 15060 {
		t.Fatalf("sip_listen=%+v", summary.SIPListen)
	}
	if summary.RTPListen.Transport != "UDP" || summary.RTPListen.IP != "0.0.0.0" || summary.RTPListen.PortRange != "16000-16020" {
		t.Fatalf("rtp_listen=%+v", summary.RTPListen)
	}
	if summary.StorageDirs.TempDir != "./data/temp" {
		t.Fatalf("storage=%+v", summary.StorageDirs)
	}
	if summary.SelfCheckSummary.Overall != "info" {
		t.Fatalf("self_check_summary=%+v", summary.SelfCheckSummary)
	}
	if summary.BusinessExecution.State != "protocol_only" || summary.BusinessExecution.RouteCount != 0 {
		t.Fatalf("business_execution=%+v", summary.BusinessExecution)
	}
}

func selfCheckReportForTest() selfcheck.Report {
	return selfcheck.Report{GeneratedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Overall: selfcheck.LevelInfo, Summary: selfcheck.Summary{Info: 2, Warn: 0, Error: 0}}
}

func TestConfigCandidatesPriority(t *testing.T) {
	cli := "/tmp/from-cli.yaml"
	env := "/tmp/from-env.yaml"
	exeDir := "/opt/siptunnel"
	cwd := "/workspace/SIPTunnel/gateway-server"

	candidates := configCandidates(cli, env, exeDir, cwd, "linux")
	if len(candidates) != 6 {
		t.Fatalf("configCandidates length=%d, want 6", len(candidates))
	}

	want := []configCandidate{
		{path: cli, source: configSourceCLI},
		{path: env, source: configSourceEnv},
		{path: filepath.Join(exeDir, "configs", "config.yaml"), source: configSourceExeDir},
		{path: filepath.Join(exeDir, "config.yaml"), source: configSourceExeDir},
		{path: filepath.Join(cwd, "configs", "config.yaml"), source: configSourceCWD},
		{path: filepath.Join(cwd, "config.yaml"), source: configSourceCWD},
	}
	if !reflect.DeepEqual(candidates, want) {
		t.Fatalf("configCandidates mismatch\n got=%v\nwant=%v", candidates, want)
	}
}

func TestPickExistingConfigCandidate(t *testing.T) {
	exePath := "/opt/siptunnel/bin/gateway"
	cwd := "/workspace/SIPTunnel/gateway-server"

	existsMap := map[string]bool{
		filepath.Join(cwd, "config.yaml"): true,
	}

	candidate, allCandidates, ok := pickExistingConfigCandidate(
		"",
		"",
		func() (string, error) { return exePath, nil },
		func() (string, error) { return cwd, nil },
		func(path string) bool { return existsMap[path] },
	)
	if !ok {
		t.Fatal("pickExistingConfigCandidate() ok=false, want true")
	}
	if len(allCandidates) == 0 {
		t.Fatal("expected config candidate list")
	}
	if candidate.source != configSourceCWD {
		t.Fatalf("candidate source=%s, want %s", candidate.source, configSourceCWD)
	}
	if candidate.path != filepath.Join(cwd, "config.yaml") {
		t.Fatalf("candidate path=%q, want %q", candidate.path, filepath.Join(cwd, "config.yaml"))
	}
}

func TestExecutableDirSupportsWindowsAndLinuxPaths(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "linux path",
			path: "/opt/siptunnel/bin/gateway",
			want: "/opt/siptunnel/bin",
		},
		{
			name: "windows path",
			path: `C:\\SIPTunnel\\bin\\gateway.exe`,
			want: `C:\SIPTunnel\bin`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := executableDir(tt.path); got != tt.want {
				t.Fatalf("executableDir(%q)=%q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestReadCLIConfigPath(t *testing.T) {
	path, err := readCLIConfigPath([]string{"--config", " ./configs/custom.yaml "})
	if err != nil {
		t.Fatalf("readCLIConfigPath() error=%v", err)
	}
	if path != "./configs/custom.yaml" {
		t.Fatalf("readCLIConfigPath()=%q, want %q", path, "./configs/custom.yaml")
	}
}

func TestDefaultConfigYAMLContainsRequiredSections(t *testing.T) {
	raw, err := defaultConfigYAML("linux")
	if err != nil {
		t.Fatalf("defaultConfigYAML() error=%v", err)
	}
	text := string(raw)
	for _, section := range []string{"server:", "sip:", "rtp:", "storage:", "observability:", "ui:", "ops:"} {
		if !strings.Contains(text, section) {
			t.Fatalf("default config missing section %q", section)
		}
	}
}

func TestDefaultConfigYAMLWindowsUsesFriendlySIPPort(t *testing.T) {
	raw, err := defaultConfigYAML("windows")
	if err != nil {
		t.Fatalf("defaultConfigYAML() error=%v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "listen_port:") {
		t.Fatalf("default config should contain sip.listen_port, got %q", text)
	}
	allowed := []string{"listen_port: 59226", "listen_port: 15060", "listen_port: 25060", "listen_port: 35060", "listen_port: 5060"}
	for _, item := range allowed {
		if strings.Contains(text, item) {
			return
		}
	}
	t.Fatalf("windows default config should use friendly sip port, got %q", text)
}

func TestPickFriendlySIPPortSkipsOccupiedPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:5060")
	if err != nil {
		t.Skipf("unable to occupy 5060 for test: %v", err)
	}
	defer ln.Close()
	if got := pickFriendlySIPPort(); got == 5060 {
		t.Fatalf("pickFriendlySIPPort()=%d, should skip occupied 5060", got)
	}
}

func TestPickFirstAvailablePort(t *testing.T) {
	candidates := []int{5060, 15060, 25060}
	used := map[int]bool{5060: true, 15060: false, 25060: true}
	got := pickFirstAvailablePort(candidates, func(port int) bool {
		return !used[port]
	})
	if got != 15060 {
		t.Fatalf("pickFirstAvailablePort()=%d, want 15060", got)
	}
}

func TestParseRuntimeConfigFromTopLevelYAML(t *testing.T) {
	raw := []byte(`sip:
  listen_port: 16060
rtp:
  port_start: 22000
  port_end: 22020
ui:
  enabled: true
  mode: embedded
  base_path: /ops
`)
	cfg, err := parseRuntimeConfigFromTopLevelYAML(raw)
	if err != nil {
		t.Fatalf("parseRuntimeConfigFromTopLevelYAML() error=%v", err)
	}
	if cfg.Network.SIP.ListenPort != 16060 {
		t.Fatalf("sip.listen_port=%d, want 16060", cfg.Network.SIP.ListenPort)
	}
	if cfg.Network.RTP.PortStart != 22000 || cfg.Network.RTP.PortEnd != 22020 {
		t.Fatalf("rtp range=[%d,%d], want [22000,22020]", cfg.Network.RTP.PortStart, cfg.Network.RTP.PortEnd)
	}
	if cfg.UI.Mode != "embedded" || cfg.UI.BasePath != "/ops" {
		t.Fatalf("ui config mismatch mode=%q base_path=%q", cfg.UI.Mode, cfg.UI.BasePath)
	}
}

func TestResolveConfigOutputPathWindowsUsesExeDir(t *testing.T) {
	cli := ""
	env := ""
	getwd := func() (string, error) { return `D:\work`, nil }
	execFn := func() (string, error) { return `C:\SIPTunnel\gateway.exe`, nil }
	got := resolveConfigOutputPath(cli, env, getwd, execFn, "windows")
	want := filepath.Join(`C:\SIPTunnel`, "configs", "config.yaml")
	if got != want {
		t.Fatalf("resolveConfigOutputPath()=%q, want %q", got, want)
	}
}

func TestConfigCandidatesWindowsPrefersExeRelativeForRelativeCLIAndEnv(t *testing.T) {
	candidates := configCandidates("configs/custom.yaml", "configs/env.yaml", `C:\SIPTunnel`, `D:\work`, "windows")
	if len(candidates) != 8 {
		t.Fatalf("configCandidates length=%d, want 8", len(candidates))
	}
	if candidates[0].path != filepath.Join(`C:\SIPTunnel`, "configs", "custom.yaml") {
		t.Fatalf("first candidate=%q", candidates[0].path)
	}
	if candidates[2].path != filepath.Join(`C:\SIPTunnel`, "configs", "env.yaml") {
		t.Fatalf("third candidate=%q", candidates[2].path)
	}
}

func TestFormatStartupFailureWindowsContainsPortHints(t *testing.T) {
	err := errors.New("listen tcp 127.0.0.1:18080: bind: Only one usage of each socket address")
	msg := formatStartupFailure("start gateway http server", err, "127.0.0.1:18080", "windows")
	for _, part := range []string{"PowerShell", "CMD", "当前监听地址=127.0.0.1:18080"} {
		if !strings.Contains(msg, part) {
			t.Fatalf("formatStartupFailure() missing %q in %q", part, msg)
		}
	}
}

func TestEnsureWindowsDefaultDataDir(t *testing.T) {
	t.Setenv("GATEWAY_DATA_DIR", "")
	t.Setenv("GATEWAY_TEMP_DIR", "")
	t.Setenv("GATEWAY_FINAL_DIR", "")
	t.Setenv("GATEWAY_AUDIT_DIR", "")
	t.Setenv("GATEWAY_LOG_DIR", "")
	ensureWindowsDefaultDataDir(`C:\SIPTunnel`)
	if got := os.Getenv("GATEWAY_DATA_DIR"); got != filepath.Join(`C:\SIPTunnel`, "data") {
		t.Fatalf("GATEWAY_DATA_DIR=%q", got)
	}
}

func TestReadPortPrefersFriendlyFallbackWhenUnset(t *testing.T) {
	t.Setenv("GATEWAY_PORT", "")
	if got := readPort("prod", "linux"); got != "18080" {
		t.Fatalf("prod default port=%s, want 18080", got)
	}
	if got := readPort("dev", "windows"); got == "" {
		t.Fatal("dev windows fallback should not be empty")
	}
}

func TestRunMainSkipsStartupWhenToolCommandAfterFlags(t *testing.T) {
	called := false
	err := runMain([]string{"--config", "./configs/config.yaml", "print-default-config"}, func(_ []string) {
		called = true
	})
	if err != nil {
		t.Fatalf("runMain() error=%v", err)
	}
	if called {
		t.Fatal("startup should be skipped when tool command appears after flags")
	}
}

func TestExtractToolCommandSupportsPastedNewlineEscapes(t *testing.T) {
	cmd, _, ok := extractToolCommand([]string{"init-config\n2026/03/13 16:10:02 log line"})
	if !ok {
		t.Fatal("expected tool command to be detected")
	}
	if cmd != "init-config" {
		t.Fatalf("cmd=%q, want init-config", cmd)
	}
}

func TestHandleMissingConfigFile(t *testing.T) {
	t.Run("dev mode auto generates", func(t *testing.T) {
		t.Setenv("GATEWAY_MODE", "dev")
		root := t.TempDir()
		path, mode, err := handleMissingConfigFile("", filepath.Join(root, "custom.yaml"), func() (string, error) { return root, nil })
		if err != nil {
			t.Fatalf("handleMissingConfigFile() error=%v", err)
		}
		if mode != "dev" {
			t.Fatalf("mode=%q, want dev", mode)
		}
		if path != filepath.Join(root, "custom.yaml") {
			t.Fatalf("path=%q, want %q", path, filepath.Join(root, "custom.yaml"))
		}
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("expected generated file exists: %v", statErr)
		}
	})

	t.Run("prod mode generates template and returns error", func(t *testing.T) {
		t.Setenv("GATEWAY_MODE", "prod")
		root := t.TempDir()
		path, mode, err := handleMissingConfigFile(filepath.Join(root, "prod.yaml"), "", func() (string, error) { return root, nil })
		if mode != "prod" {
			t.Fatalf("mode=%q, want prod", mode)
		}
		if err == nil {
			t.Fatal("expected production mode error, got nil")
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read generated template error=%v", readErr)
		}
		if !strings.Contains(string(raw), "PRODUCTION TEMPLATE") {
			t.Fatalf("expected production template banner, got: %s", string(raw))
		}
	})
}
