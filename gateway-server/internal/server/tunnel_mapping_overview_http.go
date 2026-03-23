package server

import (
	"net/http"
	"sort"
	"strings"

	"siptunnel/internal/tunnelmapping"
)

type tunnelMappingOverviewItem struct {
	ResourceCode   string   `json:"resource_code"`
	DeviceID       string   `json:"device_id"`
	ResourceType   string   `json:"resource_type"`
	Name           string   `json:"name"`
	SourceNode     string   `json:"source_node,omitempty"`
	Methods        []string `json:"methods"`
	ResponseMode   string   `json:"response_mode"`
	ResourceStatus string   `json:"resource_status"`
	MappingStatus  string   `json:"mapping_status"`
	MappingIDs     []string `json:"mapping_ids,omitempty"`
	MappingID      string   `json:"mapping_id,omitempty"`
	ListenIP       string   `json:"listen_ip,omitempty"`
	ListenPorts    []int    `json:"listen_ports"`
	LocalBindPort  int      `json:"local_bind_port,omitempty"`
	PathPrefix     string   `json:"path_prefix,omitempty"`
	Enabled        bool     `json:"enabled"`
}

type tunnelMappingOverviewSummary struct {
	ResourceTotal int `json:"resource_total"`
	MappedTotal   int `json:"mapped_total"`
	ManualTotal   int `json:"manual_total"`
	UnmappedTotal int `json:"unmapped_total"`
}

type tunnelMappingOverviewResponse struct {
	Items   []tunnelMappingOverviewItem  `json:"items"`
	Summary tunnelMappingOverviewSummary `json:"summary"`
}

func mappingStatusFromExposureMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "MANUAL") {
		return "MANUAL"
	}
	return "UNMAPPED"
}

func firstMappingByID(items []TunnelMapping) map[string]TunnelMapping {
	out := map[string]TunnelMapping{}
	for _, item := range items {
		out[item.MappingID] = item
	}
	return out
}

func currentSourceNodeID(d *handlerDeps) string {
	if d.nodeStore != nil {
		peers := d.nodeStore.ListPeers()
		for _, peer := range peers {
			if peer.Enabled && strings.TrimSpace(peer.PeerNodeID) != "" {
				return strings.TrimSpace(peer.PeerNodeID)
			}
		}
	}
	if strings.TrimSpace(d.tunnelConfig.PeerDeviceID) != "" {
		return strings.TrimSpace(d.tunnelConfig.PeerDeviceID)
	}
	return ""
}

func (d *handlerDeps) handleTunnelMappingOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	plan := d.currentCatalogExposurePlan()
	byID := firstMappingByID(plan.EffectiveMappings)
	sourceNode := currentSourceNodeID(d)
	resp := tunnelMappingOverviewResponse{}
	for _, view := range plan.Views {
		item := tunnelMappingOverviewItem{
			ResourceCode:   view.DeviceID,
			DeviceID:       view.DeviceID,
			ResourceType:   tunnelmapping.DefaultResourceType,
			Name:           firstNonEmpty(strings.TrimSpace(view.Name), strings.TrimSpace(view.DeviceID)),
			SourceNode:     sourceNode,
			Methods:        append([]string(nil), view.MethodList...),
			ResponseMode:   normalizedResponseMode(view.ResponseMode),
			ResourceStatus: firstNonEmpty(strings.TrimSpace(view.Status), "UNKNOWN"),
			MappingStatus:  mappingStatusFromExposureMode(view.ExposureMode),
			MappingIDs:     append([]string(nil), view.MappingIDs...),
			ListenPorts:    append([]int(nil), view.LocalPorts...),
			Enabled:        view.ExposureMode != "UNEXPOSED",
		}
		if len(item.Methods) == 0 {
			item.Methods = []string{"*"}
		}
		if len(view.MappingIDs) > 0 {
			if first, ok := byID[view.MappingIDs[0]]; ok {
				item.MappingID = first.MappingID
				item.ListenIP = strings.TrimSpace(first.LocalBindIP)
				item.LocalBindPort = first.LocalBindPort
				item.PathPrefix = strings.TrimSpace(first.LocalBasePath)
				item.Enabled = first.Enabled
			}
		}
		if item.ListenIP == "" && len(view.LocalPorts) > 0 && d.nodeStore != nil {
			local := d.nodeStore.GetLocalNode()
			item.ListenIP = firstNonEmpty(strings.TrimSpace(local.SIPListenIP), "127.0.0.1")
			item.PathPrefix = "/"
		}
		resp.Items = append(resp.Items, item)
		resp.Summary.ResourceTotal++
		switch item.MappingStatus {
		case "MANUAL":
			resp.Summary.MappedTotal++
			resp.Summary.ManualTotal++
		default:
			resp.Summary.UnmappedTotal++
		}
	}
	sort.Slice(resp.Items, func(i, j int) bool { return resp.Items[i].DeviceID < resp.Items[j].DeviceID })
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: resp})
}
