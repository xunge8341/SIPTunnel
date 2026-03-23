package smoketest

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCheckCommandChain(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/demo/process" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":"OK","message":"processed"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	passed, detail := checkCommandChain(context.Background(), ts.Client(), ts.URL)
	if !passed {
		t.Fatalf("expected pass, got fail detail=%s", detail)
	}
}

func TestCheckSIPAndRTPListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/node/network-status" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":"OK","data":{"sip":{"listen_ip":"127.0.0.1","listen_port":` + strconv.Itoa(port) + `,"transport":"TCP"},"rtp":{"port_start":20000,"port_end":20100,"transport":"UDP","available_ports":10}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if ok, detail := checkSIPListener(ctx, ts.Client(), ts.URL); !ok {
		t.Fatalf("checkSIPListener fail: %s", detail)
	}
	if ok, detail := checkRTPListener(ctx, ts.Client(), ts.URL); !ok {
		t.Fatalf("checkRTPListener fail: %s", detail)
	}
}

func TestFormatSummaryIncludesFailedChecks(t *testing.T) {
	s := FormatSummary(SuiteResult{Duration: 2 * time.Second, Results: []CheckResult{{Name: "A", Passed: true, Detail: "ok"}, {Name: "B", Passed: false, Detail: "bad"}}})
	if !strings.Contains(s, "[FAIL]") || !strings.Contains(s, "Failed checks: B") {
		t.Fatalf("unexpected summary: %s", s)
	}
}

func TestCheckFirstStartSummary(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/startup-summary" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":"OK","data":{"run_mode":"dev","config_path":"./configs/config.yaml","config_source":"exe_dir"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()
	if ok, detail := checkFirstStartSummary(context.Background(), ts.Client(), ts.URL); !ok {
		t.Fatalf("checkFirstStartSummary fail: %s", detail)
	}
}
