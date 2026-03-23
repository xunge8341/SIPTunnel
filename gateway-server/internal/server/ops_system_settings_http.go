package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"siptunnel/internal/config"
	"siptunnel/internal/persistence"
)

func defaultSystemSettings(d *handlerDeps, sqlitePath string) SystemSettingsPayload {
	if strings.TrimSpace(sqlitePath) == "" {
		sqlitePath = "./data/final/gateway.db"
	}
	tuning := config.DefaultTransportTuningConfig()
	converged := config.ConvergedGenericDownloadProfile(tuning)
	return SystemSettingsPayload{
		SQLitePath:                        sqlitePath,
		LogCleanupCron:                    "*/30 * * * *",
		MaxTaskAgeDays:                    7,
		MaxTaskRecords:                    20000,
		MaxAccessLogAgeDays:               7,
		MaxAccessLogRecords:               20000,
		MaxAuditAgeDays:                   30,
		MaxAuditRecords:                   50000,
		MaxDiagnosticAgeDays:              15,
		MaxDiagnosticRecords:              2000,
		MaxLoadtestAgeDays:                15,
		MaxLoadtestRecords:                2000,
		AdminAllowCIDR:                    "127.0.0.1/32",
		AdminRequireMFA:                   false,
		GenericDownloadTotalMbps:          bpsToMbps(tuning.GenericDownloadTotalBitrateBps),
		GenericDownloadPerTransferMbps:    bpsToMbps(tuning.GenericDownloadMinPerTransferBitrateBps),
		GenericDownloadWindowMB:           bytesToMB(tuning.GenericDownloadWindowBytes),
		AdaptiveHotCacheMB:                bytesToMB(tuning.AdaptivePlaybackSegmentCacheBytes),
		AdaptiveHotWindowMB:               bytesToMB(tuning.AdaptivePlaybackHotWindowBytes),
		GenericDownloadSegmentConcurrency: tuning.GenericDownloadSegmentConcurrency,
		GenericDownloadRTPReorderWindow:   converged.ReorderWindowPackets,
		GenericDownloadRTPLossTolerance:   converged.LossTolerancePackets,
		GenericDownloadRTPGapTimeoutMS:    converged.GapTimeoutMS,
		GenericDownloadRTPFECEnabled:      converged.FECEnabled,
		GenericDownloadRTPFECGroupPackets: converged.FECGroupPackets,
		CleanerLastResult:                 "未执行",
	}
}

func systemSettingsHasRuntimeProfile(req SystemSettingsPayload) bool {
	return req.GenericDownloadTotalMbps > 0 ||
		req.GenericDownloadPerTransferMbps > 0 ||
		req.GenericDownloadWindowMB > 0 ||
		req.AdaptiveHotCacheMB > 0 ||
		req.AdaptiveHotWindowMB > 0 ||
		req.GenericDownloadSegmentConcurrency > 0 ||
		req.GenericDownloadRTPReorderWindow > 0 ||
		req.GenericDownloadRTPLossTolerance > 0 ||
		req.GenericDownloadRTPGapTimeoutMS > 0 ||
		req.GenericDownloadRTPFECGroupPackets > 0
}

func populateSystemSettingsRuntimeProfile(req *SystemSettingsPayload, base config.TransportTuningConfig) {
	if req == nil {
		return
	}
	converged := config.ConvergedGenericDownloadProfile(base)
	req.GenericDownloadTotalMbps = bpsToMbps(base.GenericDownloadTotalBitrateBps)
	req.GenericDownloadPerTransferMbps = bpsToMbps(base.GenericDownloadMinPerTransferBitrateBps)
	req.GenericDownloadWindowMB = bytesToMB(base.GenericDownloadWindowBytes)
	req.AdaptiveHotCacheMB = bytesToMB(base.AdaptivePlaybackSegmentCacheBytes)
	req.AdaptiveHotWindowMB = bytesToMB(base.AdaptivePlaybackHotWindowBytes)
	req.GenericDownloadSegmentConcurrency = base.GenericDownloadSegmentConcurrency
	req.GenericDownloadRTPReorderWindow = converged.ReorderWindowPackets
	req.GenericDownloadRTPLossTolerance = converged.LossTolerancePackets
	req.GenericDownloadRTPGapTimeoutMS = converged.GapTimeoutMS
	req.GenericDownloadRTPFECEnabled = converged.FECEnabled
	req.GenericDownloadRTPFECGroupPackets = converged.FECGroupPackets
}

func normalizeSystemSettingsRuntimeProfile(req *SystemSettingsPayload, base config.TransportTuningConfig) {
	if req == nil || systemSettingsHasRuntimeProfile(*req) {
		return
	}
	populateSystemSettingsRuntimeProfile(req, base)
}

func applySystemSettingsRuntimeProfile(base config.TransportTuningConfig, req SystemSettingsPayload) config.TransportTuningConfig {
	cfg := base
	if req.GenericDownloadTotalMbps > 0 {
		cfg.GenericDownloadTotalBitrateBps = mbpsToBps(req.GenericDownloadTotalMbps)
	}
	if req.GenericDownloadPerTransferMbps > 0 {
		cfg.GenericDownloadMinPerTransferBitrateBps = mbpsToBps(req.GenericDownloadPerTransferMbps)
	}
	if req.GenericDownloadWindowMB > 0 {
		cfg.GenericDownloadWindowBytes = mbToBytes(req.GenericDownloadWindowMB)
	}
	if req.AdaptiveHotCacheMB > 0 {
		cfg.AdaptivePlaybackSegmentCacheBytes = mbToBytes(req.AdaptiveHotCacheMB)
	}
	if req.AdaptiveHotWindowMB > 0 {
		cfg.AdaptivePlaybackHotWindowBytes = mbToBytes(req.AdaptiveHotWindowMB)
	}
	if req.GenericDownloadSegmentConcurrency > 0 {
		cfg.GenericDownloadSegmentConcurrency = req.GenericDownloadSegmentConcurrency
	}
	if req.GenericDownloadRTPReorderWindow > 0 {
		cfg.GenericDownloadRTPReorderWindowPackets = req.GenericDownloadRTPReorderWindow
	}
	if req.GenericDownloadRTPLossTolerance > 0 {
		cfg.GenericDownloadRTPLossTolerancePackets = req.GenericDownloadRTPLossTolerance
	}
	if req.GenericDownloadRTPGapTimeoutMS > 0 {
		cfg.GenericDownloadRTPGapTimeoutMS = req.GenericDownloadRTPGapTimeoutMS
	}
	if systemSettingsHasRuntimeProfile(req) {
		cfg.GenericDownloadRTPFECEnabled = req.GenericDownloadRTPFECEnabled
	}
	if req.GenericDownloadRTPFECGroupPackets > 0 {
		cfg.GenericDownloadRTPFECGroupPackets = req.GenericDownloadRTPFECGroupPackets
	}
	return cfg
}

func mbpsToBps(v float64) int   { return int(v * 1024 * 1024) }
func mbToBytes(v float64) int64 { return int64(v * 1024 * 1024) }

func validateSystemSettings(req SystemSettingsPayload) error {
	if strings.TrimSpace(req.SQLitePath) == "" {
		return fmt.Errorf("sqlite_path is required")
	}
	if err := validateCleanupSchedule(req.LogCleanupCron); err != nil {
		return err
	}
	checks := []struct {
		name  string
		value int
		min   int
	}{
		{"max_task_age_days", req.MaxTaskAgeDays, 1},
		{"max_task_records", req.MaxTaskRecords, 100},
		{"max_access_log_age_days", req.MaxAccessLogAgeDays, 1},
		{"max_access_log_records", req.MaxAccessLogRecords, 100},
		{"max_audit_age_days", req.MaxAuditAgeDays, 1},
		{"max_audit_records", req.MaxAuditRecords, 100},
		{"max_diagnostic_age_days", req.MaxDiagnosticAgeDays, 1},
		{"max_diagnostic_records", req.MaxDiagnosticRecords, 10},
		{"max_loadtest_age_days", req.MaxLoadtestAgeDays, 1},
		{"max_loadtest_records", req.MaxLoadtestRecords, 10},
		{"generic_download_segment_concurrency", req.GenericDownloadSegmentConcurrency, 1},
		{"generic_download_rtp_reorder_window_packets", req.GenericDownloadRTPReorderWindow, 32},
		{"generic_download_rtp_loss_tolerance_packets", req.GenericDownloadRTPLossTolerance, 1},
		{"generic_download_rtp_gap_timeout_ms", req.GenericDownloadRTPGapTimeoutMS, 100},
	}
	for _, item := range checks {
		if item.value < item.min {
			return fmt.Errorf("%s must be >= %d", item.name, item.min)
		}
	}
	floatChecks := []struct {
		name  string
		value float64
		min   float64
	}{
		{"generic_download_total_mbps", req.GenericDownloadTotalMbps, 1},
		{"generic_download_per_transfer_mbps", req.GenericDownloadPerTransferMbps, 0.5},
		{"generic_download_window_mb", req.GenericDownloadWindowMB, 0.5},
		{"adaptive_hot_cache_mb", req.AdaptiveHotCacheMB, 1},
		{"adaptive_hot_window_mb", req.AdaptiveHotWindowMB, 1},
	}
	for _, item := range floatChecks {
		if item.value < item.min {
			return fmt.Errorf("%s must be >= %.1f", item.name, item.min)
		}
	}
	if req.GenericDownloadRTPFECEnabled && req.GenericDownloadRTPFECGroupPackets < 2 {
		return fmt.Errorf("generic_download_rtp_fec_group_packets must be >= 2 when generic_download_rtp_fec_enabled=true")
	}
	if strings.TrimSpace(req.AdminAllowCIDR) != "" {
		if _, _, err := net.ParseCIDR(strings.TrimSpace(req.AdminAllowCIDR)); err != nil {
			return fmt.Errorf("admin_allow_cidr is invalid")
		}
	}
	return nil
}

func validateSystemSettingsRuntime(req SystemSettingsPayload, runtime managementSecurityRuntime) error {
	if req.AdminRequireMFA && !runtime.MFAConfigured {
		return fmt.Errorf("admin_require_mfa requires configured GATEWAY_ADMIN_MFA_CODE")
	}
	return nil
}

func (d *handlerDeps) handleSystemSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		d.mu.RLock()
		resp := d.systemSettings
		d.mu.RUnlock()
		populateSystemSettingsRuntimeProfile(&resp, currentTransportTuning())
		if d.cleaner != nil {
			runAt, result, removed := d.cleaner.Snapshot()
			resp.CleanerLastRunAt = runAt
			resp.CleanerLastResult = result
			resp.CleanerLastRemovedRecords = removed
		}
		runtimeSec := currentManagementSecurityRuntime(d)
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: map[string]any{
			"sqlite_path":                                 resp.SQLitePath,
			"log_cleanup_cron":                            resp.LogCleanupCron,
			"max_task_age_days":                           resp.MaxTaskAgeDays,
			"max_task_records":                            resp.MaxTaskRecords,
			"max_access_log_age_days":                     resp.MaxAccessLogAgeDays,
			"max_access_log_records":                      resp.MaxAccessLogRecords,
			"max_audit_age_days":                          resp.MaxAuditAgeDays,
			"max_audit_records":                           resp.MaxAuditRecords,
			"max_diagnostic_age_days":                     resp.MaxDiagnosticAgeDays,
			"max_diagnostic_records":                      resp.MaxDiagnosticRecords,
			"max_loadtest_age_days":                       resp.MaxLoadtestAgeDays,
			"max_loadtest_records":                        resp.MaxLoadtestRecords,
			"admin_allow_cidr":                            resp.AdminAllowCIDR,
			"admin_require_mfa":                           resp.AdminRequireMFA,
			"generic_download_total_mbps":                 resp.GenericDownloadTotalMbps,
			"generic_download_per_transfer_mbps":          resp.GenericDownloadPerTransferMbps,
			"generic_download_window_mb":                  resp.GenericDownloadWindowMB,
			"adaptive_hot_cache_mb":                       resp.AdaptiveHotCacheMB,
			"adaptive_hot_window_mb":                      resp.AdaptiveHotWindowMB,
			"generic_download_segment_concurrency":        resp.GenericDownloadSegmentConcurrency,
			"generic_download_rtp_reorder_window_packets": resp.GenericDownloadRTPReorderWindow,
			"generic_download_rtp_loss_tolerance_packets": resp.GenericDownloadRTPLossTolerance,
			"generic_download_rtp_gap_timeout_ms":         resp.GenericDownloadRTPGapTimeoutMS,
			"generic_download_rtp_fec_enabled":            resp.GenericDownloadRTPFECEnabled,
			"generic_download_rtp_fec_group_packets":      resp.GenericDownloadRTPFECGroupPackets,
			"cleaner_last_run_at":                         resp.CleanerLastRunAt,
			"cleaner_last_result":                         resp.CleanerLastResult,
			"cleaner_last_removed_records":                resp.CleanerLastRemovedRecords,
			"admin_token_configured":                      runtimeSec.Enforced,
			"admin_mfa_configured":                        runtimeSec.MFAConfigured,
			"config_encryption_enabled":                   runtimeSec.ConfigKeyEnabled,
			"tunnel_signer_externalized":                  runtimeSec.SignerSecretManaged,
		}})
	case http.MethodPut, http.MethodPost:
		var req SystemSettingsPayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid body")
			return
		}
		req.SQLitePath = strings.TrimSpace(req.SQLitePath)
		if req.SQLitePath == "" {
			d.mu.RLock()
			req.SQLitePath = d.systemSettings.SQLitePath
			d.mu.RUnlock()
		}
		normalizeSystemSettingsRuntimeProfile(&req, currentTransportTuning())
		if err := validateSystemSettings(req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		if err := validateSystemSettingsRuntime(req, currentManagementSecurityRuntime(d)); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		baselineTransport := applySystemSettingsRuntimeProfile(config.DefaultTransportTuningConfig(), req)
		ApplyTransportTuning(baselineTransport)
		d.mu.Lock()
		d.systemSettings = req
		d.baselineTransportTuning = baselineTransport
		d.mu.Unlock()
		if d.accessLogStore != nil {
			d.accessLogStore.Configure(req.MaxAccessLogAgeDays, req.MaxAccessLogRecords)
		}
		if d.cleaner != nil {
			if err := d.cleaner.UpdateSchedule(req.LogCleanupCron); err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
				return
			}
		}
		if d.sqliteStore != nil {
			d.sqliteStore.UpdateRetention(persistence.RetentionPolicy{
				MaxTaskAgeDays:       req.MaxTaskAgeDays,
				MaxTaskRecords:       req.MaxTaskRecords,
				MaxAccessLogAgeDays:  req.MaxAccessLogAgeDays,
				MaxAccessLogRecords:  req.MaxAccessLogRecords,
				MaxAuditAgeDays:      req.MaxAuditAgeDays,
				MaxAuditRecords:      req.MaxAuditRecords,
				MaxDiagnosticAgeDays: req.MaxDiagnosticAgeDays,
				MaxDiagnosticRecords: req.MaxDiagnosticRecords,
			})
		}
		_ = saveJSON(d.systemPath, req)
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: req})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}
