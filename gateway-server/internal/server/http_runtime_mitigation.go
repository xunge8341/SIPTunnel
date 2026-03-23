package server

import (
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
)

var runtimeHTTPMitigationLogOnce sync.Once

type runtimeHTTPKeepAliveDecision struct {
	Disable bool
	Source  string
	Reason  string
}

const (
	gatewayHTTPScope          = "gateway-http"
	mappingRuntimeScope       = "mapping-runtime"
	mappingForwardClientScope = "mapping-forward-client"
)

func runtimeHTTPKeepAliveMitigationSuggested() bool {
	return runtime.GOOS == "windows" && strings.HasPrefix(strings.TrimSpace(runtime.Version()), "go1.26")
}

func runtimeHTTPKeepAliveScopeEnv(scope string) string {
	scope = strings.ToUpper(strings.TrimSpace(scope))
	scope = strings.NewReplacer("-", "_", "/", "_", " ", "_").Replace(scope)
	if scope == "" {
		return "GATEWAY_DISABLE_HTTP_KEEPALIVES"
	}
	return "GATEWAY_DISABLE_HTTP_KEEPALIVES_" + scope
}

func runtimeHTTPKeepAlivePolicy(scope string) runtimeHTTPKeepAliveDecision {
	if value := strings.TrimSpace(os.Getenv(runtimeHTTPKeepAliveScopeEnv(scope))); value != "" {
		return runtimeHTTPKeepAliveDecision{Disable: parseBoolEnvValue(value), Source: runtimeHTTPKeepAliveScopeEnv(scope), Reason: "explicit_scope_override"}
	}
	if value := strings.TrimSpace(os.Getenv("GATEWAY_DISABLE_HTTP_KEEPALIVES")); value != "" {
		return runtimeHTTPKeepAliveDecision{Disable: parseBoolEnvValue(value), Source: "GATEWAY_DISABLE_HTTP_KEEPALIVES", Reason: "explicit_global_override"}
	}
	if runtimeHTTPKeepAliveMitigationSuggested() {
		trimmedScope := strings.TrimSpace(scope)
		if strings.EqualFold(trimmedScope, mappingForwardClientScope) || strings.EqualFold(trimmedScope, mappingRuntimeScope) {
			return runtimeHTTPKeepAliveDecision{Disable: false, Source: "auto_exempt", Reason: "preserve_internal_proxy_connection_reuse"}
		}
		return runtimeHTTPKeepAliveDecision{Disable: true, Source: "auto", Reason: "windows_go1.26_connreader_crash_workaround"}
	}
	return runtimeHTTPKeepAliveDecision{Disable: false, Source: "default", Reason: "keep_alive_reuse_preserved"}
}

func shouldDisableHTTPKeepAlivesForRuntime(scope string) bool {
	return runtimeHTTPKeepAlivePolicy(scope).Disable
}

func parseBoolEnvValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}

func ApplyRuntimeHTTPMitigations(scope string, srv *http.Server) {
	if srv == nil {
		return
	}
	decision := runtimeHTTPKeepAlivePolicy(scope)
	if !runtimeHTTPKeepAliveMitigationSuggested() && decision.Source == "default" {
		return
	}
	if decision.Disable {
		srv.SetKeepAlivesEnabled(false)
		log.Printf("http mitigation enabled scope=%s kind=server os=%s go_version=%s keep_alives=false source=%s reason=%s override_hint=%s", scope, runtime.GOOS, runtime.Version(), decision.Source, decision.Reason, runtimeHTTPKeepAliveScopeEnv(scope)+"=false")
		return
	}
	runtimeHTTPMitigationLogOnce.Do(func() {
		log.Printf("http mitigation advisory scope=%s kind=server os=%s go_version=%s keep_alives=true source=%s note=%s hint=%s", scope, runtime.GOOS, runtime.Version(), decision.Source, "runtime workaround available but disabled to preserve connection reuse/performance", runtimeHTTPKeepAliveScopeEnv(scope)+"=true")
	})
}

// ApplyRuntimeHTTPTransportMitigations mirrors the server-side keep-alive policy
// onto outbound http.Transport instances. Task 9 needs this because the A/B
// experiment is invalid if inbound server sockets and outbound mapping-forward
// clients are not switched with the same reuse policy.
func ApplyRuntimeHTTPTransportMitigations(scope string, transport *http.Transport) runtimeHTTPKeepAliveDecision {
	if transport == nil {
		return runtimeHTTPKeepAliveDecision{}
	}
	decision := runtimeHTTPKeepAlivePolicy(scope)
	if decision.Disable {
		transport.DisableKeepAlives = true
		transport.MaxIdleConns = 0
		transport.MaxIdleConnsPerHost = 0
	}
	log.Printf("http mitigation transport scope=%s kind=transport keep_alives=%t source=%s reason=%s override_hint=%s", scope, !decision.Disable, decision.Source, decision.Reason, runtimeHTTPKeepAliveScopeEnv(scope)+"=true|false")
	return decision
}
