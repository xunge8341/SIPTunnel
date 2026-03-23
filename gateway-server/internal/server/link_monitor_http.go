package server

import (
	"net/http"
	"time"
)

type linkMonitorResponse struct {
	Session          tunnelSessionRuntimeState     `json:"session"`
	Config           TunnelConfigPayload           `json:"config"`
	GB28181          *GB28181Snapshot              `json:"gb28181,omitempty"`
	MappingSummary   *tunnelMappingOverviewSummary `json:"mapping_summary,omitempty"`
	LiveStatus       string                        `json:"live_status"`
	ReadyStatus      string                        `json:"ready_status"`
	ReadinessReasons []string                      `json:"readiness_reasons,omitempty"`
	UpdatedAt        string                        `json:"updated_at"`
}

func (d *handlerDeps) handleLinkMonitor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	ready, reasons := d.readinessReport(r.Context())
	resp := linkMonitorResponse{Config: normalizeTunnelConfigPayload(d.tunnelConfig, ""), LiveStatus: "ok", ReadyStatus: map[bool]string{true: "ready", false: "not_ready"}[ready], ReadinessReasons: reasons, UpdatedAt: formatTimestamp(time.Now().UTC())}
	if d.sessionMgr != nil {
		resp.Session = d.sessionMgr.Snapshot()
	}
	if d.gbService != nil {
		snapshot := d.gbService.Snapshot()
		resp.GB28181 = &snapshot
	}
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: resp})
}
