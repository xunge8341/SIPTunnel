package config

import "testing"

func TestUIConfigApplyDefaults(t *testing.T) {
	cfg := UIConfig{}
	cfg.ApplyDefaults(19090)
	if cfg.Mode != "external" {
		t.Fatalf("mode=%q, want external", cfg.Mode)
	}
	if cfg.ListenPort != 19090 {
		t.Fatalf("listen_port=%d, want 19090", cfg.ListenPort)
	}
	if cfg.BasePath != "/" {
		t.Fatalf("base_path=%q, want /", cfg.BasePath)
	}
}

func TestUIConfigValidate(t *testing.T) {
	cfg := UIConfig{Enabled: true, Mode: "embedded", ListenIP: "127.0.0.1", ListenPort: 18080, BasePath: "/ops"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error=%v", err)
	}

	bad := UIConfig{Enabled: true, Mode: "bad", ListenIP: "x", ListenPort: 70000, BasePath: "ops"}
	if err := bad.Validate(); err == nil {
		t.Fatal("Validate() expected error, got nil")
	}
}
