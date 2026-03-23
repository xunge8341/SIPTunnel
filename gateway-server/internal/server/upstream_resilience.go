package server

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"siptunnel/internal/netdiag"
)

type upstreamErrorClass string

const (
	upstreamErrorClassUnknown            upstreamErrorClass = "unknown"
	upstreamErrorClassCircuitOpen        upstreamErrorClass = "circuit_open"
	upstreamErrorClassTimeout            upstreamErrorClass = "timeout"
	upstreamErrorClassConnectionRefused  upstreamErrorClass = "connection_refused"
	upstreamErrorClassDNSFailure         upstreamErrorClass = "dns_failure"
	upstreamErrorClassNetworkUnreachable upstreamErrorClass = "network_unreachable"
	upstreamErrorClassConnectionReset    upstreamErrorClass = "connection_reset"
)

type upstreamErrorInfo struct {
	Class      upstreamErrorClass
	Temporary  bool
	UserReason string
}

type upstreamCircuitOpenError struct {
	Target    string
	Until     time.Time
	LastCause string
}

func (e *upstreamCircuitOpenError) Error() string {
	if e == nil {
		return "upstream target is temporarily suppressed"
	}
	until := e.Until.UTC().Format(time.RFC3339)
	if strings.TrimSpace(e.LastCause) == "" {
		return fmt.Sprintf("上游目标 %s 暂时退避中，预计 %s 后重试", e.Target, until)
	}
	return fmt.Sprintf("上游目标 %s 暂时退避中，预计 %s 后重试；最近失败：%s", e.Target, until, e.LastCause)
}

type upstreamCircuitState struct {
	Consecutive int
	OpenUntil   time.Time
	LastCause   string
	HalfOpen    bool
}

type upstreamCircuitEntrySnapshot struct {
	Key         string `json:"key"`
	State       string `json:"state"`
	OpenUntil   string `json:"open_until,omitempty"`
	LastCause   string `json:"last_cause,omitempty"`
	Consecutive int    `json:"consecutive_failures,omitempty"`
}

type upstreamCircuitSnapshot struct {
	OpenCount      int                            `json:"open_count"`
	HalfOpenCount  int                            `json:"half_open_count"`
	LastOpenUntil  string                         `json:"last_open_until,omitempty"`
	LastOpenReason string                         `json:"last_open_reason,omitempty"`
	Entries        []upstreamCircuitEntrySnapshot `json:"entries,omitempty"`
}

type upstreamCircuitGuard struct {
	mu      sync.Mutex
	entries map[string]upstreamCircuitState
}

func newUpstreamCircuitGuard() *upstreamCircuitGuard {
	return &upstreamCircuitGuard{entries: map[string]upstreamCircuitState{}}
}

func (g *upstreamCircuitGuard) Before(key, target string, now time.Time) error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	state, ok := g.entries[key]
	if !ok {
		return nil
	}
	if !state.OpenUntil.IsZero() && now.Before(state.OpenUntil) {
		return &upstreamCircuitOpenError{Target: target, Until: state.OpenUntil, LastCause: state.LastCause}
	}
	if !state.OpenUntil.IsZero() && !now.Before(state.OpenUntil) {
		state.OpenUntil = time.Time{}
		state.HalfOpen = true
		g.entries[key] = state
	}
	return nil
}

func (g *upstreamCircuitGuard) Reset(target string) int {
	if g == nil {
		return 0
	}
	needle := strings.ToLower(strings.TrimSpace(target))
	g.mu.Lock()
	defer g.mu.Unlock()
	removed := 0
	if needle == "" {
		removed = len(g.entries)
		g.entries = map[string]upstreamCircuitState{}
		return removed
	}
	for key, state := range g.entries {
		if strings.Contains(strings.ToLower(key), needle) || strings.Contains(strings.ToLower(state.LastCause), needle) {
			delete(g.entries, key)
			removed++
		}
	}
	return removed
}

func (g *upstreamCircuitGuard) RecordSuccess(key string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.entries, key)
}

func (g *upstreamCircuitGuard) Snapshot(now time.Time) upstreamCircuitSnapshot {
	if g == nil {
		return upstreamCircuitSnapshot{}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	out := upstreamCircuitSnapshot{}
	for key, state := range g.entries {
		entry := upstreamCircuitEntrySnapshot{Key: key, LastCause: strings.TrimSpace(state.LastCause), Consecutive: state.Consecutive}
		switch {
		case !state.OpenUntil.IsZero() && now.Before(state.OpenUntil):
			out.OpenCount++
			entry.State = "open"
			entry.OpenUntil = state.OpenUntil.UTC().Format(time.RFC3339)
			if out.LastOpenUntil == "" || state.OpenUntil.After(now) {
				out.LastOpenUntil = entry.OpenUntil
				out.LastOpenReason = strings.TrimSpace(state.LastCause)
			}
		case state.HalfOpen:
			out.HalfOpenCount++
			entry.State = "half_open"
		default:
			continue
		}
		out.Entries = append(out.Entries, entry)
	}
	return out
}

func (g *upstreamCircuitGuard) RecordFailure(key string, info upstreamErrorInfo, now time.Time) {
	if g == nil || !info.Temporary {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	state := g.entries[key]
	state.Consecutive++
	if state.Consecutive < 1 {
		state.Consecutive = 1
	}
	backoff := time.Duration(1<<minInt(state.Consecutive-1, 4)) * time.Second
	if backoff > 30*time.Second {
		backoff = 30 * time.Second
	}
	state.OpenUntil = now.Add(backoff)
	state.LastCause = strings.TrimSpace(info.UserReason)
	state.HalfOpen = false
	g.entries[key] = state
}

func classifyUpstreamError(err error, target *url.URL) upstreamErrorInfo {
	targetText := "上游目标"
	if target != nil {
		targetText = target.String()
	}
	if err == nil {
		return upstreamErrorInfo{Class: upstreamErrorClassUnknown, UserReason: fmt.Sprintf("访问 %s 失败", targetText)}
	}
	if _, ok := err.(*upstreamCircuitOpenError); ok {
		return upstreamErrorInfo{Class: upstreamErrorClassCircuitOpen, Temporary: true, UserReason: err.Error()}
	}
	if netdiag.IsTimeoutError(err) {
		return upstreamErrorInfo{Class: upstreamErrorClassTimeout, Temporary: true, UserReason: fmt.Sprintf("访问 %s 超时，可能是目标无响应、IP 已变更或网络受限", targetText)}
	}
	lowered := netdiag.NormalizeError(err)
	switch {
	case netdiag.LooksLikeDNSFailureText(lowered):
		return upstreamErrorInfo{Class: upstreamErrorClassDNSFailure, Temporary: true, UserReason: fmt.Sprintf("解析上游目标 %s 失败，请检查域名、DNS 或目标地址是否已变更", targetText)}
	case netdiag.LooksLikeConnectionRefusedText(lowered):
		return upstreamErrorInfo{Class: upstreamErrorClassConnectionRefused, Temporary: true, UserReason: fmt.Sprintf("访问上游目标 %s 被拒绝连接，请检查服务是否启动、端口是否监听、IP 是否已变更", targetText)}
	case netdiag.LooksLikeNetworkUnreachableText(lowered):
		return upstreamErrorInfo{Class: upstreamErrorClassNetworkUnreachable, Temporary: true, UserReason: fmt.Sprintf("无法到达上游目标 %s，请检查网络路由、防火墙或目标 IP 是否已变更", targetText)}
	case netdiag.LooksLikeTimeoutText(lowered):
		return upstreamErrorInfo{Class: upstreamErrorClassTimeout, Temporary: true, UserReason: fmt.Sprintf("访问上游目标 %s 超时，可能是目标无响应、IP 已变更或网络受限", targetText)}
	case netdiag.LooksLikeConnectionClosedText(lowered):
		return upstreamErrorInfo{Class: upstreamErrorClassConnectionReset, Temporary: true, UserReason: fmt.Sprintf("访问上游目标 %s 时连接被中断，请检查对端稳定性与网络质量", targetText)}
	default:
		return upstreamErrorInfo{Class: upstreamErrorClassUnknown, Temporary: false, UserReason: fmt.Sprintf("访问上游目标 %s 失败", targetText)}
	}
}

func formatPreparedForwardError(prepared *mappingForwardRequest, info upstreamErrorInfo, raw error) error {
	targetText := "<unknown>"
	if prepared != nil && prepared.TargetURL != nil {
		targetText = prepared.TargetURL.String()
	}
	userReason := strings.TrimSpace(info.UserReason)
	if userReason == "" {
		userReason = fmt.Sprintf("访问上游目标 %s 失败", targetText)
	}
	rawText := ""
	if raw != nil {
		rawText = strings.TrimSpace(raw.Error())
	}
	if rawText == "" {
		return errors.New(userReason)
	}
	return fmt.Errorf("%s；原始错误：%s", userReason, rawText)
}

func upstreamGuardKey(prepared *mappingForwardRequest) string {
	if prepared == nil || prepared.TargetURL == nil {
		return ""
	}
	return prepared.MappingID + "|" + strings.ToLower(strings.TrimSpace(prepared.TargetURL.Scheme)) + "://" + strings.ToLower(strings.TrimSpace(prepared.TargetURL.Host))
}

var defaultUpstreamCircuitGuard = newUpstreamCircuitGuard()

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
