package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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

func TestParseNetworkConfigFromTopLevelYAML(t *testing.T) {
	raw := []byte(`sip:
  listen_port: 16060
rtp:
  port_start: 22000
  port_end: 22020
`)
	cfg, err := parseNetworkConfigFromTopLevelYAML(raw)
	if err != nil {
		t.Fatalf("parseNetworkConfigFromTopLevelYAML() error=%v", err)
	}
	if cfg.SIP.ListenPort != 16060 {
		t.Fatalf("sip.listen_port=%d, want 16060", cfg.SIP.ListenPort)
	}
	if cfg.RTP.PortStart != 22000 || cfg.RTP.PortEnd != 22020 {
		t.Fatalf("rtp range=[%d,%d], want [22000,22020]", cfg.RTP.PortStart, cfg.RTP.PortEnd)
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
