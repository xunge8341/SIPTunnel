package selfcheck

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"siptunnel/internal/config"
	"siptunnel/internal/service/httpinvoke"
)

type Level string

const (
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

type Item struct {
	Name       string `json:"name"`
	Level      Level  `json:"level"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
	ActionHint string `json:"action_hint"`
	DocLink    string `json:"doc_link,omitempty"`
}

type Summary struct {
	Info  int `json:"info"`
	Warn  int `json:"warn"`
	Error int `json:"error"`
}

type Report struct {
	GeneratedAt time.Time `json:"generated_at"`
	Overall     Level     `json:"overall"`
	Summary     Summary   `json:"summary"`
	Items       []Item    `json:"items"`
}

type Input struct {
	NetworkConfig                      config.NetworkConfig
	StoragePaths                       config.StoragePaths
	DownstreamRoutes                   []httpinvoke.RouteConfig
	DialTimeout                        time.Duration
	RunMode                            string
	SuggestFreePort                    bool
	ExpectSIPPortOwnedByCurrentProcess bool
}

type Runner struct {
	nowFn          func() time.Time
	interfaceIPs   func() (map[string]struct{}, error)
	listenTCP      func(network, address string) (net.Listener, error)
	listenUDP      func(network, address string) (net.PacketConn, error)
	ensureWritable func(path string) error
	dialTCP        func(ctx context.Context, address string) error
	execCommand    func(ctx context.Context, name string, args ...string) ([]byte, error)
	lookPath       func(file string) (string, error)
	findFreePort   func(transport string, ip string) (int, error)
	goos           func() string
}

func NewRunner() *Runner {
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	return &Runner{
		nowFn:          func() time.Time { return time.Now().UTC() },
		interfaceIPs:   hostInterfaceIPs,
		listenTCP:      net.Listen,
		listenUDP:      net.ListenPacket,
		ensureWritable: ensureDirWritable,
		execCommand: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		},
		lookPath:     exec.LookPath,
		findFreePort: findAvailablePort,
		goos:         func() string { return runtime.GOOS },
		dialTCP: func(ctx context.Context, address string) error {
			conn, err := dialer.DialContext(ctx, "tcp", address)
			if err != nil {
				return err
			}
			_ = conn.Close()
			return nil
		},
	}
}

func (r *Runner) Run(ctx context.Context, in Input) Report {
	if r == nil {
		r = NewRunner()
	}
	if in.DialTimeout <= 0 {
		in.DialTimeout = 2 * time.Second
	}
	if !isProdMode(in.RunMode) && !in.SuggestFreePort {
		in.SuggestFreePort = true
	}

	items := []Item{}
	items = append(items, r.checkListenIP("sip.listen_ip", in.NetworkConfig.SIP.Enabled, in.NetworkConfig.SIP.ListenIP)...)
	items = append(items, r.checkListenIP("rtp.listen_ip", in.NetworkConfig.RTP.Enabled, in.NetworkConfig.RTP.ListenIP)...)
	items = append(items, r.checkSIPPortOccupancy(in.NetworkConfig.SIP, in)...)
	items = append(items, r.checkSIPUDPMessageSizeRisk(in.NetworkConfig)...)
	items = append(items, r.checkRTPRange(in.NetworkConfig.RTP)...)
	items = append(items, r.checkRTPTransportPlan(in.NetworkConfig)...)
	items = append(items, r.checkTransportTuningConvergence(in.NetworkConfig)...)
	items = append(items, r.checkSIPRTPConflict(in.NetworkConfig)...)
	items = append(items, r.checkWritableDirs(in.StoragePaths)...)
	items = append(items, r.checkDownstreamReachability(ctx, in.DownstreamRoutes, in.DialTimeout)...)

	report := Report{GeneratedAt: r.nowFn(), Overall: LevelInfo, Items: items}
	for _, item := range items {
		switch item.Level {
		case LevelError:
			report.Summary.Error++
			report.Overall = LevelError
		case LevelWarn:
			report.Summary.Warn++
			if report.Overall != LevelError {
				report.Overall = LevelWarn
			}
		default:
			report.Summary.Info++
		}
	}
	return report
}

func (r Report) ToCLI() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("self-check generated_at=%s overall=%s info=%d warn=%d error=%d\n", r.GeneratedAt.Format(time.RFC3339), r.Overall, r.Summary.Info, r.Summary.Warn, r.Summary.Error))
	for _, item := range r.Items {
		b.WriteString(fmt.Sprintf("- [%s] %s: %s | suggestion: %s | action_hint: %s", strings.ToUpper(string(item.Level)), item.Name, item.Message, item.Suggestion, item.ActionHint))
		if item.DocLink != "" {
			b.WriteString(" | doc: " + item.DocLink)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func item(name string, level Level, message string, suggestion string, actionHint string, docLink string) Item {
	return Item{Name: name, Level: level, Message: message, Suggestion: suggestion, ActionHint: actionHint, DocLink: docLink}
}

func findAvailablePort(transport string, ip string) (int, error) {
	bindIP := strings.TrimSpace(ip)
	if bindIP == "" {
		bindIP = "0.0.0.0"
	}
	addr := net.JoinHostPort(bindIP, "0")
	if strings.EqualFold(transport, "UDP") {
		pc, err := net.ListenPacket("udp", addr)
		if err != nil {
			return 0, err
		}
		defer pc.Close()
		port := pc.LocalAddr().(*net.UDPAddr).Port
		return port, nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	return port, nil
}

func isProdMode(mode string) bool {
	return strings.EqualFold(strings.TrimSpace(mode), "prod")
}
