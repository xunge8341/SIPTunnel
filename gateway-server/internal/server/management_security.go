package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type adminSessionInfo struct {
	Operator   string
	ClientIP   string
	AuthMethod string
	MFAPassed  bool
}

type adminSessionContextKey struct{}

type managementSecurityRuntime struct {
	Enforced            bool
	Token               string
	TokenFingerprint    string
	AllowCIDR           string
	RequireMFA          bool
	MFACode             string
	MFAConfigured       bool
	ConfigKeyEnabled    bool
	SignerSecretManaged bool
}

func currentManagementSecurityRuntime(d *handlerDeps) managementSecurityRuntime {
	runtime := managementSecurityRuntime{
		Token:               strings.TrimSpace(os.Getenv("GATEWAY_ADMIN_TOKEN")),
		MFACode:             strings.TrimSpace(os.Getenv("GATEWAY_ADMIN_MFA_CODE")),
		ConfigKeyEnabled:    configEncryptionEnabled(),
		SignerSecretManaged: tunnelSignerSecretConfigured(),
	}
	if runtime.Token != "" {
		sum := sha256.Sum256([]byte(runtime.Token))
		runtime.TokenFingerprint = strings.ToUpper(hex.EncodeToString(sum[:6]))
		runtime.Enforced = true
	}
	runtime.MFAConfigured = runtime.MFACode != ""
	d.mu.RLock()
	runtime.AllowCIDR = strings.TrimSpace(d.systemSettings.AdminAllowCIDR)
	runtime.RequireMFA = d.systemSettings.AdminRequireMFA
	d.mu.RUnlock()
	if runtime.RequireMFA {
		runtime.Enforced = true
	}
	return runtime
}

func (d *handlerDeps) withManagementSecurity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requiresManagementSecurity(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		runtime := currentManagementSecurityRuntime(d)
		if !runtime.Enforced {
			next.ServeHTTP(w, r)
			return
		}
		clientIP := requestClientIP(r)
		if runtime.AllowCIDR != "" && clientIP != "" {
			if _, network, err := net.ParseCIDR(runtime.AllowCIDR); err == nil {
				ip := net.ParseIP(clientIP)
				if ip == nil || !network.Contains(ip) {
					d.recordManagementSecurityFailure(r, clientIP, "cidr_block", fmt.Sprintf("client_ip=%s is outside admin_allow_cidr=%s", clientIP, runtime.AllowCIDR))
					writeError(w, http.StatusForbidden, "ADMIN_CIDR_DENIED", "management access denied by CIDR policy")
					return
				}
			}
		}
		if runtime.Token != "" {
			token := firstNonEmpty(strings.TrimSpace(bearerTokenFromRequest(r)), strings.TrimSpace(r.Header.Get("X-Admin-Token")))
			if strings.TrimSpace(token) != runtime.Token {
				d.recordManagementSecurityFailure(r, clientIP, "token_missing_or_invalid", "admin token missing or invalid")
				writeError(w, http.StatusUnauthorized, "ADMIN_TOKEN_REQUIRED", "valid admin token is required")
				return
			}
		}
		mfaPassed := false
		if runtime.RequireMFA {
			if !runtime.MFAConfigured {
				d.recordManagementSecurityFailure(r, clientIP, "mfa_not_configured", "admin MFA is required but GATEWAY_ADMIN_MFA_CODE is empty")
				writeError(w, http.StatusServiceUnavailable, "ADMIN_MFA_NOT_CONFIGURED", "admin MFA is required but not configured")
				return
			}
			if strings.TrimSpace(r.Header.Get("X-Admin-MFA")) != runtime.MFACode {
				d.recordManagementSecurityFailure(r, clientIP, "mfa_invalid", "admin mfa code missing or invalid")
				writeError(w, http.StatusUnauthorized, "ADMIN_MFA_REQUIRED", "valid admin MFA code is required")
				return
			}
			mfaPassed = true
		}
		operator := strings.TrimSpace(firstNonEmpty(r.Header.Get("X-Admin-Operator"), r.Header.Get("X-Initiator"), "admin"))
		session := adminSessionInfo{Operator: operator, ClientIP: clientIP, AuthMethod: "header_token", MFAPassed: mfaPassed}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), adminSessionContextKey{}, session)))
	})
}

func requiresManagementSecurity(path string) bool {
	switch path {
	case "/healthz", "/readyz", "/":
		return false
	}
	if strings.HasPrefix(path, "/assets/") {
		return false
	}
	return strings.HasPrefix(path, "/api/") || path == "/metrics" || path == "/audit/events"
}

func bearerTokenFromRequest(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(header) < 7 || !strings.EqualFold(header[:7], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(header[7:])
}

func adminSessionFromContext(ctx context.Context) (adminSessionInfo, bool) {
	session, ok := ctx.Value(adminSessionContextKey{}).(adminSessionInfo)
	return session, ok
}

func (d *handlerDeps) recordManagementSecurityFailure(r *http.Request, clientIP, category, reason string) {
	if d == nil || d.securityEvents == nil {
		return
	}
	d.securityEvents.Add(securityEventRecord{
		When:      formatTimestamp(time.Now().UTC()),
		Category:  "management_" + strings.TrimSpace(category),
		Transport: "http",
		RequestID: strings.TrimSpace(r.Header.Get("X-Request-ID")),
		TraceID:   strings.TrimSpace(r.Header.Get("X-Trace-ID")),
		SessionID: strings.TrimSpace(r.Header.Get("X-Session-ID")),
		Reason:    strings.TrimSpace(firstNonEmpty(reason, clientIP)),
	})
}
