package loadtest

import (
	"testing"
	"time"
)

func TestPercentile(t *testing.T) {
	vals := []float64{1, 2, 3, 4, 5}
	if got := percentile(vals, 50); got != 3 {
		t.Fatalf("p50 got %.2f", got)
	}
	if got := percentile(vals, 95); got <= 4 {
		t.Fatalf("p95 got %.2f", got)
	}
}

func TestValidateConfig(t *testing.T) {
	cfg := Config{Targets: []string{"http-invoke"}, Concurrency: 1, Duration: time.Second, OutputDir: t.TempDir(), Timeout: time.Second}
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig err=%v", err)
	}
}

func TestClassifyErr(t *testing.T) {
	if got := classifyErr(assertErr("context deadline exceeded")); got != "timeout" {
		t.Fatalf("classify timeout got=%s", got)
	}
	if got := classifyErr(assertErr("connection refused")); got != "connection_refused" {
		t.Fatalf("classify conn got=%s", got)
	}
}

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

func assertErr(s string) error { return fakeErr(s) }
