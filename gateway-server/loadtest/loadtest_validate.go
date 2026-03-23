package loadtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"siptunnel/internal/netdiag"
)

func isProbePathURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(u.Path)) {
	case "/healthz", "/readyz", "/livez", "/health", "/ready", "/live":
		return true
	default:
		return false
	}
}

func validateConfig(cfg Config) error {
	if len(cfg.Targets) == 0 {
		return errors.New("targets is required")
	}
	for _, target := range cfg.Targets {
		switch strings.ToLower(strings.TrimSpace(target)) {
		case "sip-command-create", "sip-status-receipt", "rtp-udp-upload", "rtp-tcp-upload", "rtp-upload", "http-invoke", "mapping-forward":
		default:
			return fmt.Errorf("unsupported target: %s", target)
		}
	}
	if cfg.Concurrency <= 0 {
		return errors.New("concurrency must be > 0")
	}
	if cfg.Duration <= 0 {
		return errors.New("duration must be > 0")
	}
	if cfg.OutputDir == "" {
		return errors.New("output-dir is required")
	}
	if cfg.Timeout <= 0 {
		return errors.New("timeout must be > 0")
	}
	for _, target := range cfg.Targets {
		if strings.EqualFold(strings.TrimSpace(target), "mapping-forward") {
			targetURL := strings.TrimSpace(cfg.MappingURL)
			if targetURL == "" {
				targetURL = strings.TrimSpace(cfg.HTTPURL)
			}
			if targetURL == "" {
				return errors.New("mapping-url or http-url is required for mapping-forward target")
			}
			if cfg.StrictRealMapping && isProbePathURL(targetURL) {
				return fmt.Errorf("mapping-forward target %s is a probe path; use a real mapping URL", targetURL)
			}
		}
	}
	return nil
}

func classifyErr(err error) string {
	if err == nil {
		return "operation_error"
	}
	s := netdiag.NormalizeError(err)
	switch {
	case netdiag.IsTimeoutError(err):
		return "timeout"
	case netdiag.LooksLikeLocalAddrExhaustedText(s):
		return "local_addr_exhausted"
	case netdiag.LooksLikeDatagramTooLargeText(s):
		return "udp_datagram_too_large"
	case netdiag.LooksLikeConnectionRefusedText(s):
		return "connection_refused"
	case netdiag.LooksLikeConnectionClosedText(s):
		return "connection_closed"
	case strings.Contains(s, "status"):
		return "http_status"
	default:
		return "operation_error"
	}
}

func preflightTargets(ctx context.Context, cfg Config) error {
	seen := map[string]struct{}{}
	for _, target := range cfg.Targets {
		switch strings.ToLower(strings.TrimSpace(target)) {
		case "mapping-forward":
			targetURL := strings.TrimSpace(cfg.MappingURL)
			if targetURL == "" {
				targetURL = strings.TrimSpace(cfg.HTTPURL)
			}
			if targetURL == "" {
				continue
			}
			if _, ok := seen[targetURL]; ok {
				continue
			}
			seen[targetURL] = struct{}{}
			if err := preflightHTTPReachability(ctx, targetURL, cfg.Timeout); err != nil {
				return fmt.Errorf("mapping-forward preflight failed for %s: %w", targetURL, err)
			}
		}
	}
	return nil
}

func preflightHTTPReachability(ctx context.Context, targetURL string, timeout time.Duration) error {
	u, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil {
		return fmt.Errorf("parse target url: %w", err)
	}
	if strings.TrimSpace(u.Scheme) == "" || strings.TrimSpace(u.Host) == "" {
		return fmt.Errorf("invalid target url: %s", targetURL)
	}
	hostport := u.Host
	if _, _, err := net.SplitHostPort(hostport); err != nil {
		switch strings.ToLower(strings.TrimSpace(u.Scheme)) {
		case "https":
			hostport = net.JoinHostPort(hostport, "443")
		default:
			hostport = net.JoinHostPort(hostport, "80")
		}
	}
	dialTimeout := timeout
	if dialTimeout <= 0 || dialTimeout > 1500*time.Millisecond {
		dialTimeout = 1500 * time.Millisecond
	}
	dialer := net.Dialer{Timeout: dialTimeout}
	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	conn, err := dialer.DialContext(dialCtx, "tcp", hostport)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

func safeRate(success, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(success) / float64(total)
}

func percentile(sortedValues []float64, p float64) float64 {
	if len(sortedValues) == 0 {
		return 0
	}
	if p <= 0 {
		return sortedValues[0]
	}
	if p >= 100 {
		return sortedValues[len(sortedValues)-1]
	}
	index := (p / 100) * float64(len(sortedValues)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))
	if lower == upper {
		return sortedValues[lower]
	}
	weight := index - float64(lower)
	return sortedValues[lower]*(1-weight) + sortedValues[upper]*weight
}

func writeJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create summary file: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func randomHex(n int) string { return hex.EncodeToString(randomBytes(n)) }

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampInt(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}
