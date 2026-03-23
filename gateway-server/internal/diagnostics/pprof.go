package diagnostics

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	envPprofEnabled              = "GATEWAY_PPROF_ENABLED"
	envPprofListenAddress        = "GATEWAY_PPROF_LISTEN_ADDR"
	envPprofToken                = "GATEWAY_PPROF_AUTH_TOKEN"
	envPprofAllowedCIDRs         = "GATEWAY_PPROF_ALLOWED_CIDRS"
	envPprofBlockProfileRate     = "GATEWAY_PPROF_BLOCK_PROFILE_RATE"
	envPprofMutexProfileFraction = "GATEWAY_PPROF_MUTEX_PROFILE_FRACTION"
)

type PprofConfig struct {
	Enabled              bool
	ListenAddress        string
	AuthToken            string
	AllowedCIDRs         []string
	BlockProfileRate     int
	MutexProfileFraction int
}

func LoadPprofConfigFromEnv() PprofConfig {
	cfg := PprofConfig{
		Enabled:              parseBool(os.Getenv(envPprofEnabled), false),
		ListenAddress:        stringOrDefault(os.Getenv(envPprofListenAddress), "127.0.0.1:6060"),
		AuthToken:            strings.TrimSpace(os.Getenv(envPprofToken)),
		AllowedCIDRs:         splitAndTrim(os.Getenv(envPprofAllowedCIDRs), []string{"127.0.0.1/32", "::1/128"}),
		BlockProfileRate:     parseInt(os.Getenv(envPprofBlockProfileRate), 0),
		MutexProfileFraction: parseInt(os.Getenv(envPprofMutexProfileFraction), 0),
	}
	if cfg.BlockProfileRate < 0 {
		cfg.BlockProfileRate = 0
	}
	if cfg.MutexProfileFraction < 0 {
		cfg.MutexProfileFraction = 0
	}
	return cfg
}

func (c PprofConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if strings.TrimSpace(c.AuthToken) == "" {
		return fmt.Errorf("%s is required when pprof is enabled", envPprofToken)
	}
	for _, cidr := range c.AllowedCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("invalid pprof cidr %q: %w", cidr, err)
		}
	}
	return nil
}

func (c PprofConfig) ApplyRuntimeSampling() {
	runtime.SetBlockProfileRate(c.BlockProfileRate)
	runtime.SetMutexProfileFraction(c.MutexProfileFraction)
}

func StartPprofServer(cfg PprofConfig, logger *slog.Logger) (*http.Server, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg.ApplyRuntimeSampling()

	middleware, err := newPprofGuard(cfg)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/debug/pprof/", middleware(http.HandlerFunc(pprof.Index)))
	mux.Handle("/debug/pprof/cmdline", middleware(http.HandlerFunc(pprof.Cmdline)))
	mux.Handle("/debug/pprof/profile", middleware(http.HandlerFunc(pprof.Profile)))
	mux.Handle("/debug/pprof/symbol", middleware(http.HandlerFunc(pprof.Symbol)))
	mux.Handle("/debug/pprof/trace", middleware(http.HandlerFunc(pprof.Trace)))
	mux.Handle("/debug/pprof/heap", middleware(pprof.Handler("heap")))
	mux.Handle("/debug/pprof/goroutine", middleware(pprof.Handler("goroutine")))
	mux.Handle("/debug/pprof/block", middleware(pprof.Handler("block")))
	mux.Handle("/debug/pprof/mutex", middleware(pprof.Handler("mutex")))
	mux.Handle("/debug/pprof/allocs", middleware(pprof.Handler("allocs")))

	srv := &http.Server{Addr: cfg.ListenAddress, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if logger == nil {
		logger = slog.Default()
	}
	go func() {
		logger.Info("pprof server enabled", "addr", cfg.ListenAddress, "allowed_cidrs", strings.Join(cfg.AllowedCIDRs, ","), "block_profile_rate", cfg.BlockProfileRate, "mutex_profile_fraction", cfg.MutexProfileFraction)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("pprof server failed", "err", err)
		}
	}()
	return srv, nil
}

type guard func(http.Handler) http.Handler

func newPprofGuard(cfg PprofConfig) (guard, error) {
	nets := make([]*net.IPNet, 0, len(cfg.AllowedCIDRs))
	for _, raw := range cfg.AllowedCIDRs {
		_, ipNet, err := net.ParseCIDR(raw)
		if err != nil {
			return nil, err
		}
		nets = append(nets, ipNet)
	}
	token := cfg.AuthToken
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !authorizedToken(r, token) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if !authorizedIP(r.RemoteAddr, nets) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}

func authorizedToken(r *http.Request, expected string) bool {
	v := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if v == "" {
		v = strings.TrimSpace(r.Header.Get("X-Pprof-Token"))
	}
	if len(v) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(v), []byte(expected)) == 1
}

func authorizedIP(remoteAddr string, allowed []*net.IPNet) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, ipNet := range allowed {
		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

func ShutdownServer(ctx context.Context, srv *http.Server) error {
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}

func parseBool(raw string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(raw))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func parseInt(raw string, fallback int) int {
	v := strings.TrimSpace(raw)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func splitAndTrim(raw string, fallback []string) []string {
	if strings.TrimSpace(raw) == "" {
		out := make([]string, len(fallback))
		copy(out, fallback)
		return out
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		out := make([]string, len(fallback))
		copy(out, fallback)
		return out
	}
	return out
}

func stringOrDefault(raw string, fallback string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return fallback
	}
	return v
}
