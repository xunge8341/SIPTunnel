package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (s NodeNetworkStatus) TunnelStatus() string {
	if len(s.RecentBindErrors) > 0 {
		return "degraded"
	}
	return "healthy"
}

func (d *handlerDeps) handleProtectionState(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: d.currentProtectionState()})
	case http.MethodPut, http.MethodPost:
		var req struct {
			AlertRules          []string `json:"alert_rules"`
			CircuitBreakerRules []string `json:"circuit_breaker_rules"`
			FailureThreshold    int      `json:"failure_threshold"`
			RecoveryWindowSec   int      `json:"recovery_window_sec"`
			RPS                 int      `json:"rps"`
			Burst               int      `json:"burst"`
			MaxConcurrent       int      `json:"max_concurrent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid body")
			return
		}
		d.mu.Lock()
		if len(req.AlertRules) > 0 {
			d.protection.AlertRules = req.AlertRules
		}
		if len(req.CircuitBreakerRules) > 0 {
			d.protection.CircuitBreakerRules = req.CircuitBreakerRules
		}
		if req.FailureThreshold > 0 {
			d.protection.FailureThreshold = req.FailureThreshold
		}
		if req.RecoveryWindowSec > 0 {
			d.protection.RecoveryWindowSec = req.RecoveryWindowSec
		}
		if req.RPS > 0 {
			d.limits.RPS = req.RPS
		}
		if req.Burst > 0 {
			d.limits.Burst = req.Burst
		}
		if req.MaxConcurrent > 0 {
			d.limits.MaxConcurrent = req.MaxConcurrent
		}
		d.protection = normalizeProtectionSettings(d.protection)
		d.limits = normalizeOpsLimits(d.limits)
		prot := d.protection
		limits := d.limits
		d.baselineLimits = limits
		protector := d.protectionRuntime
		d.mu.Unlock()
		if protector != nil {
			protector.UpdateLimits(limits)
		}
		_ = saveJSON(d.protectionPath, prot)
		if d.sqliteStore != nil {
			_ = d.sqliteStore.SaveSystemConfig(r.Context(), "ops.limits", limits)
		}
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: d.currentProtectionState()})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (d *handlerDeps) handleProtectionCircuitRecover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	var req struct {
		Target string `json:"target"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	removed := defaultUpstreamCircuitGuard.Reset(strings.TrimSpace(req.Target))
	d.recordOpsAudit(r, readOperator(r), "RECOVER_CIRCUIT_BREAKER", map[string]any{"target": strings.TrimSpace(req.Target), "removed": removed})
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: map[string]any{"removed": removed, "target": strings.TrimSpace(req.Target), "state": d.currentProtectionState()}})
}

func (d *handlerDeps) currentProtectionState() protectionState {
	d.mu.RLock()
	limits := normalizeOpsLimits(d.limits)
	prot := normalizeProtectionSettings(d.protection)
	protector := d.protectionRuntime
	d.mu.RUnlock()
	analysis := d.aggregateAccessStats(context.Background())
	runtimeSnapshot := protectionRuntimeSnapshot{}
	if protector != nil {
		runtimeSnapshot = protector.Snapshot()
	}
	circuitSnapshot := defaultUpstreamCircuitGuard.Snapshot(time.Now().UTC())
	currentTriggered := []string{}
	lastTime := runtimeSnapshot.LastTriggeredTime
	lastTarget := runtimeSnapshot.LastTriggeredTarget
	rateLimitStatus := "已启用"
	circuitStatus := "关闭"
	circuitActiveState := "closed"
	protectionStatus := "未触发保护"
	if runtimeSnapshot.RateLimitHitsTotal > 0 {
		currentTriggered = append(currentTriggered, fmt.Sprintf("rate_limit_hits=%d", runtimeSnapshot.RateLimitHitsTotal))
		rateLimitStatus = "命中限流"
		protectionStatus = "限流保护中"
	}
	if runtimeSnapshot.ConcurrentRejects > 0 {
		currentTriggered = append(currentTriggered, fmt.Sprintf("concurrency_rejects=%d", runtimeSnapshot.ConcurrentRejects))
		if protectionStatus == "未触发保护" {
			protectionStatus = "并发保护中"
		}
	}
	if prot.FailureThreshold > 0 && analysis.Failed >= prot.FailureThreshold {
		currentTriggered = append(currentTriggered, "失败请求阈值告警")
		if protectionStatus == "未触发保护" {
			protectionStatus = "失败保护观察中"
		}
	}
	if len(analysis.FailedByMapping) > 0 {
		currentTriggered = append(currentTriggered, "映射失败保护观察中")
	}
	if circuitSnapshot.OpenCount > 0 {
		circuitStatus = "打开"
		circuitActiveState = "open"
		currentTriggered = append(currentTriggered, fmt.Sprintf("circuit_open=%d", circuitSnapshot.OpenCount))
		if protectionStatus == "未触发保护" {
			protectionStatus = "熔断保护中"
		}
	} else if circuitSnapshot.HalfOpenCount > 0 {
		circuitStatus = "半开恢复观察"
		circuitActiveState = "half_open"
		currentTriggered = append(currentTriggered, fmt.Sprintf("circuit_half_open=%d", circuitSnapshot.HalfOpenCount))
		if protectionStatus == "未触发保护" {
			protectionStatus = "熔断恢复观察中"
		}
	}
	if analysis.LatestFailed != nil {
		if lastTime == "" {
			lastTime = analysis.LatestFailed.OccurredAt
		}
		if lastTarget == "" {
			lastTarget = analysis.LatestFailed.MappingName
		}
	}
	if d.securityEvents != nil {
		recent := d.securityEvents.RecentSince(time.Now().UTC().Add(-1 * time.Hour))
		if len(recent) > 0 {
			counts := map[string]int{}
			for _, item := range recent {
				counts[item.Category]++
			}
			if counts["sip_replay"] > 0 {
				currentTriggered = append(currentTriggered, fmt.Sprintf("SIP 重放拦截=%d", counts["sip_replay"]))
			}
			if counts["sip_digest"] > 0 {
				currentTriggered = append(currentTriggered, fmt.Sprintf("SIP 摘要校验失败=%d", counts["sip_digest"]))
			}
			if counts["sip_signature"] > 0 {
				currentTriggered = append(currentTriggered, fmt.Sprintf("SIP 签名校验失败=%d", counts["sip_signature"]))
			}
			if len(currentTriggered) > 0 && protectionStatus == "未触发保护" {
				protectionStatus = "安全告警观察中"
			}
			if lastTime == "" {
				lastTime = recent[0].When
			}
			if lastTarget == "" {
				lastTarget = recent[0].Category
			}
		}
	}
	return protectionState{
		AlertRules:             prot.AlertRules,
		RateLimitRules:         []string{fmt.Sprintf("RPS=%d", limits.RPS), fmt.Sprintf("Burst=%d", limits.Burst), fmt.Sprintf("MaxConcurrent=%d", limits.MaxConcurrent)},
		CircuitBreakerRules:    prot.CircuitBreakerRules,
		CurrentTriggered:       currentTriggered,
		LastTriggeredTime:      lastTime,
		LastTriggeredTarget:    lastTarget,
		RPS:                    limits.RPS,
		Burst:                  limits.Burst,
		MaxConcurrent:          limits.MaxConcurrent,
		FailureThreshold:       prot.FailureThreshold,
		RecoveryWindowSec:      prot.RecoveryWindowSec,
		RateLimitStatus:        rateLimitStatus,
		CircuitBreakerStatus:   circuitStatus,
		ProtectionStatus:       protectionStatus,
		AnalysisWindow:         "近 1 小时",
		RecentFailureCount:     analysis.Failed,
		RecentSlowRequestCount: analysis.Slow,
		CurrentActiveRequests:  runtimeSnapshot.ActiveRequests,
		RateLimitHitsTotal:     runtimeSnapshot.RateLimitHitsTotal,
		ConcurrentRejectsTotal: runtimeSnapshot.ConcurrentRejects,
		AllowedRequestsTotal:   runtimeSnapshot.AllowedTotal,
		LastTriggeredType:      runtimeSnapshot.LastTriggeredType,
		CircuitOpenCount:       circuitSnapshot.OpenCount,
		CircuitHalfOpenCount:   circuitSnapshot.HalfOpenCount,
		CircuitActiveState:     circuitActiveState,
		CircuitLastOpenUntil:   circuitSnapshot.LastOpenUntil,
		CircuitLastOpenReason:  circuitSnapshot.LastOpenReason,
		CircuitEntries:         circuitSnapshot.Entries,
		TopRateLimitTargets:    runtimeSnapshot.TopRateLimitTargets,
		TopConcurrentTargets:   runtimeSnapshot.TopConcurrentTargets,
		TopAllowedTargets:      runtimeSnapshot.TopAllowedTargets,
		Scopes:                 runtimeSnapshot.Scopes,
		Restrictions:           runtimeSnapshot.Restrictions,
	}
}

func (d *handlerDeps) handleSecurityState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	d.mu.RLock()
	runtimeSec := currentManagementSecurityRuntime(d)
	state := securityCenterState{
		LicenseStatus:         d.licenseInfo.Status,
		ExpiryTime:            d.licenseInfo.ExpireAt,
		ActiveTime:            d.licenseInfo.ActiveAt,
		MaintenanceExpireTime: d.licenseInfo.MaintenanceExpireAt,
		LicenseTime:           d.licenseInfo.LicenseTime,
		ProductType:           d.licenseInfo.ProductType,
		ProductTypeName:       d.licenseInfo.ProductTypeName,
		LicenseType:           d.licenseInfo.LicenseType,
		LicenseCounter:        d.licenseInfo.LicenseCounter,
		MachineCode:           d.licenseInfo.MachineCode,
		ProjectCode:           d.licenseInfo.ProjectCode,
		LicensedFeatures:      d.licenseInfo.Features,
		LastValidation:        d.licenseInfo.LastVerifyResult,
		ManagementSecurity:    d.systemSettings.AdminAllowCIDR,
		SigningAlgorithm:      d.securitySettings.Signer,
		AdminTokenConfigured:  runtimeSec.Enforced,
		AdminMFARequired:      runtimeSec.RequireMFA,
		AdminMFAConfigured:    runtimeSec.MFAConfigured,
		ConfigEncryption:      runtimeSec.ConfigKeyEnabled,
		SignerExternalized:    runtimeSec.SignerSecretManaged,
		AdminTokenFingerprint: runtimeSec.TokenFingerprint,
	}
	d.mu.RUnlock()
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: state})
}

func (d *handlerDeps) handleProtectionRestrictions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		state := d.currentProtectionState()
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: map[string]any{"items": state.Restrictions}})
	case http.MethodPost:
		var req struct {
			Scope   string `json:"scope"`
			Target  string `json:"target"`
			Minutes int    `json:"minutes"`
			Reason  string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid body")
			return
		}
		if d.protectionRuntime == nil {
			writeError(w, http.StatusServiceUnavailable, "PROTECTION_NOT_READY", "protection runtime not configured")
			return
		}
		entry, err := d.protectionRuntime.UpsertRestriction(req.Scope, req.Target, req.Minutes, req.Reason)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		d.recordOpsAudit(r, readOperator(r), "UPSERT_TEMP_RESTRICTION", map[string]any{"scope": entry.Scope, "target": entry.Target, "minutes": entry.Minutes, "reason": entry.Reason})
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: map[string]any{"item": entry, "state": d.currentProtectionState()}})
	case http.MethodDelete:
		var req struct {
			Scope  string `json:"scope"`
			Target string `json:"target"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if d.protectionRuntime == nil {
			writeError(w, http.StatusServiceUnavailable, "PROTECTION_NOT_READY", "protection runtime not configured")
			return
		}
		removed := d.protectionRuntime.RemoveRestriction(req.Scope, req.Target)
		d.recordOpsAudit(r, readOperator(r), "REMOVE_TEMP_RESTRICTION", map[string]any{"scope": strings.TrimSpace(req.Scope), "target": strings.TrimSpace(req.Target), "removed": removed})
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: map[string]any{"removed": removed, "state": d.currentProtectionState()}})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}
