package server

import (
	"net/http"
)

type tunnelCatalogResponse struct {
	Resources []catalogExposureView      `json:"resources"`
	Summary   tunnelCatalogResponseStats `json:"summary"`
}

type tunnelCatalogResponseStats struct {
	ResourceTotal   int `json:"resource_total"`
	ManualExposeNum int `json:"manual_expose_num"`
	UnexposedNum    int `json:"unexposed_num"`
}

func (d *handlerDeps) currentCatalogExposurePlan() catalogExposurePlan {
	var items []TunnelMapping
	if d.mappings != nil {
		items = d.mappings.List()
	}
	if d.catalogRegistry == nil {
		return catalogExposurePlan{EffectiveMappings: cloneMappings(items)}
	}
	return buildCatalogExposurePlan(items, d.catalogRegistry.RemoteSnapshot())
}

func (d *handlerDeps) handleTunnelCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	plan := d.currentCatalogExposurePlan()
	resp := tunnelCatalogResponse{Resources: append([]catalogExposureView(nil), plan.Views...)}
	resp.Summary.ResourceTotal = len(plan.Views)
	for _, item := range plan.Views {
		switch item.ExposureMode {
		case "MANUAL":
			resp.Summary.ManualExposeNum++
		default:
			resp.Summary.UnexposedNum++
		}
	}
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: resp})
}
