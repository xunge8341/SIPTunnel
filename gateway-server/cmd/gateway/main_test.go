package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"siptunnel/internal/config"
	"siptunnel/internal/selfcheck"
)

func TestReadPort(t *testing.T) {
	t.Setenv("GATEWAY_PORT", "")
	if got := readPort(); got != "18080" {
		t.Fatalf("readPort() default = %s, want 18080", got)
	}

	t.Setenv("GATEWAY_PORT", "19090")
	if got := readPort(); got != "19090" {
		t.Fatalf("readPort() with env = %s, want 19090", got)
	}

	t.Setenv("GATEWAY_PORT", "abc")
	if got := readPort(); got != "18080" {
		t.Fatalf("readPort() with invalid env = %s, want 18080", got)
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

	candidates := configCandidates(cli, env, exeDir, cwd)
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

	candidate, ok := pickExistingConfigCandidate(
		"",
		"",
		func() (string, error) { return exePath, nil },
		func() (string, error) { return cwd, nil },
		func(path string) bool { return existsMap[path] },
	)
	if !ok {
		t.Fatal("pickExistingConfigCandidate() ok=false, want true")
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
	raw, err := defaultConfigYAML()
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
