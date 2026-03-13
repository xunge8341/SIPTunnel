package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgFile, []byte("network:\n  sip:\n    listen_port: 5060\n  rtp:\n    port_start: 20000\n    port_end: 20100\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	result, err := validateConfig(cfgFile)
	if err != nil {
		t.Fatalf("validateConfig err = %v", err)
	}
	if !result.OK {
		t.Fatalf("validateConfig OK = false")
	}
	if result.Network.SIP.Transport == "" {
		t.Fatalf("expected defaults to be applied")
	}
}

func TestRunTaskQueryJSON(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tasks" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("request_id"); got != "req-1" {
			t.Fatalf("request_id = %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"OK","message":"success","data":{"items":[{"ID":"task-1","RequestID":"req-1","Status":"succeeded","Attempt":1,"updated_at":"2026-03-12T00:00:00Z"}]}}`))
	}))
	defer ts.Close()

	var out bytes.Buffer
	var errOut bytes.Buffer
	err := run([]string{"--server", ts.URL, "--output", "json", "task", "query", "--request-id", "req-1"}, &out, &errOut)
	if err != nil {
		t.Fatalf("run err = %v", err)
	}
	var payload taskListResponse
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if len(payload.Items) != 1 || payload.Items[0].ID != "task-1" {
		t.Fatalf("unexpected task payload: %+v", payload.Items)
	}
}

func TestCollectDiagnostics(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write([]byte(`{"code":"OK","message":"success","data":{"status":"ok"}}`))
		case "/api/selfcheck":
			_, _ = w.Write([]byte(`{"code":"OK","message":"success","data":{"generated_at":"2026-03-12T00:00:00Z","overall":"info","summary":{"info":1,"warn":0,"error":0},"items":[]}}`))
		case "/api/node/network-status":
			_, _ = w.Write([]byte(`{"code":"OK","message":"success","data":{"sip":{"listen_ip":"127.0.0.1","listen_port":5060,"transport":"TCP","current_sessions":0,"current_connections":0},"rtp":{"listen_ip":"127.0.0.1","port_start":20000,"port_end":20100,"transport":"UDP","active_transfers":0,"used_ports":0,"available_ports":101,"rtp_port_pool_total":101,"rtp_port_pool_used":0,"rtp_port_alloc_fail_total":0},"recent_bind_errors":[],"recent_network_errors":[]}}`))
		case "/api/limits":
			_, _ = w.Write([]byte(`{"code":"OK","message":"success","data":{"rps":200}}`))
		case "/api/routes":
			_, _ = w.Write([]byte(`{"code":"OK","message":"success","data":{"items":[]}}`))
		case "/api/diagnostics/export":
			if r.URL.Query().Get("request_id") != "req-1" {
				t.Fatalf("request_id = %s", r.URL.Query().Get("request_id"))
			}
			if r.URL.Query().Get("trace_id") != "trace-1" {
				t.Fatalf("trace_id = %s", r.URL.Query().Get("trace_id"))
			}
			_, _ = w.Write([]byte(`{"code":"OK","message":"success","data":{"generated_at":"2026-03-12T00:00:00Z","node_id":"gateway-a-01","request_id":"req-1","trace_id":"trace-1","file_name":"diag_gateway_a_01_20260312T000000Z_req_req-1_trace_trace-1.zip","output_dir":"diag_gateway_a_01_20260312T000000Z_req_req-1_trace_trace-1","files":[]}}`))
		default:
			t.Fatalf("unexpected endpoint: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	bundle, err := collectDiagnostics(context.Background(), ts.URL, "req-1", "trace-1")
	if err != nil {
		t.Fatalf("collectDiagnostics err = %v", err)
	}
	if bundle.Health["status"] != "ok" {
		t.Fatalf("health status = %v", bundle.Health["status"])
	}
	if bundle.Node.SIP.Transport != "TCP" {
		t.Fatalf("sip transport = %s", bundle.Node.SIP.Transport)
	}
	if bundle.Export.RequestID != "req-1" || bundle.Export.TraceID != "trace-1" {
		t.Fatalf("unexpected export filter: %+v", bundle.Export)
	}
}

func TestRunConfigValidateMissingFlag(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	var errOut bytes.Buffer
	err := run([]string{"config", "validate"}, &out, &errOut)
	if err == nil {
		t.Fatal("expected error for missing -f")
	}
	if !strings.Contains(err.Error(), "-f is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunLinkTestJSON(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/ops/link-test" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"OK","message":"success","data":{"passed":true,"status":"passed","request_id":"req-1","trace_id":"trace-1","duration_ms":12,"checked_at":"2026-03-12T00:00:00Z","mock_target":"http://127.0.0.1:18080/healthz","items":[{"name":"sip_control","passed":true,"status":"passed","detail":"ok","duration_ms":3}]}}`))
	}))
	defer ts.Close()

	var out bytes.Buffer
	var errOut bytes.Buffer
	err := run([]string{"--server", ts.URL, "--output", "json", "link", "test"}, &out, &errOut)
	if err != nil {
		t.Fatalf("run err = %v", err)
	}
	if !strings.Contains(out.String(), `"request_id": "req-1"`) {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestGetAPITimeout(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"code":"OK","message":"success","data":{}}`))
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := getAPI(ctx, ts.URL, "/api/tasks", nil, &map[string]any{})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
