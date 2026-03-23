package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"

	"siptunnel/internal/config"
	"siptunnel/internal/tunnelmapping"
)

type localResourceView struct {
	ResourceCode          string   `json:"resource_code"`
	ResourceID            string   `json:"resource_id"`
	DeviceID              string   `json:"device_id"`
	ResourceType          string   `json:"resource_type"`
	Name                  string   `json:"name"`
	Enabled               bool     `json:"enabled"`
	TargetURL             string   `json:"target_url"`
	Methods               []string `json:"methods"`
	ResponseMode          string   `json:"response_mode"`
	MaxInlineResponseBody int64    `json:"max_inline_response_body"`
	MaxRequestBody        int64    `json:"max_request_body"`
	MaxResponseBody       int64    `json:"max_response_body"`
	BodyLimitPolicy       string   `json:"body_limit_policy,omitempty"`
	Description           string   `json:"description"`
	UpdatedAt             string   `json:"updated_at,omitempty"`
}

type localResourceSaveRequest struct {
	ResourceCode string   `json:"resource_code"`
	Name         string   `json:"name"`
	Enabled      bool     `json:"enabled"`
	TargetURL    string   `json:"target_url"`
	Methods      []string `json:"methods"`
	ResponseMode string   `json:"response_mode"`
	Description  string   `json:"description"`
}

type localResourceListResponse struct {
	Items []localResourceView `json:"items"`
}

func localResourceViewFromRecord(item LocalResourceRecord, mode config.NetworkMode) localResourceView {
	profile := tunnelmapping.DeriveBodyLimitProfile(item.ResponseMode, config.DeriveCapability(mode).SupportsLargeRequestBody)
	methods := append([]string(nil), item.Methods...)
	if len(methods) == 0 {
		methods = []string{"GET"}
	}
	return localResourceView{
		ResourceCode:          item.ResourceCode,
		ResourceID:            item.ResourceCode,
		DeviceID:              item.ResourceCode,
		ResourceType:          tunnelmapping.NormalizeResourceType("SERVICE"),
		Name:                  firstNonEmpty(strings.TrimSpace(item.Name), item.ResourceCode),
		Enabled:               item.Enabled,
		TargetURL:             item.TargetURL,
		Methods:               methods,
		ResponseMode:          normalizedResponseMode(item.ResponseMode),
		MaxInlineResponseBody: profile.MaxInlineResponseBody,
		MaxRequestBody:        profile.MaxRequestBodyBytes,
		MaxResponseBody:       profile.MaxResponseBodyBytes,
		BodyLimitPolicy:       profile.PolicyLabel,
		Description:           item.Description,
		UpdatedAt:             item.UpdatedAt,
	}
}

func localResourceRequestToRecord(req localResourceSaveRequest) (LocalResourceRecord, error) {
	item := LocalResourceRecord{
		ResourceCode: strings.TrimSpace(req.ResourceCode),
		Name:         strings.TrimSpace(req.Name),
		Enabled:      req.Enabled,
		TargetURL:    strings.TrimSpace(req.TargetURL),
		Methods:      append([]string(nil), req.Methods...),
		ResponseMode: strings.TrimSpace(req.ResponseMode),
		Description:  req.Description,
	}
	if !tunnelmapping.IsGBCode20(item.ResourceCode) {
		return LocalResourceRecord{}, fmt.Errorf("resource_code must be a 20-digit GB/T 28181 code")
	}
	if _, err := url.ParseRequestURI(item.TargetURL); err != nil {
		return LocalResourceRecord{}, fmt.Errorf("target_url is invalid")
	}
	item = normalizeLocalResource(item)
	return item, nil
}

func (d *handlerDeps) handleLocalResources(w http.ResponseWriter, r *http.Request) {
	if d.localResources == nil {
		writeError(w, http.StatusNotImplemented, "LOCAL_RESOURCE_STORE_NOT_READY", "local resource store not configured")
		return
	}
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/resources/local/"))
	if id != "" && r.URL.Path != "/api/resources/local" && r.URL.Path != "/api/resources/local/" {
		d.handleLocalResourceByID(w, r, id)
		return
	}
	switch r.Method {
	case http.MethodGet:
		items := d.localResources.List()
		sort.Slice(items, func(i, j int) bool { return items[i].ResourceCode < items[j].ResourceCode })
		resp := localResourceListResponse{Items: make([]localResourceView, 0, len(items))}
		for _, item := range items {
			resp.Items = append(resp.Items, localResourceViewFromRecord(item, d.tunnelConfig.NetworkMode))
		}
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: resp})
	case http.MethodPost:
		var req localResourceSaveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
			return
		}
		item, err := localResourceRequestToRecord(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		created, err := d.localResources.Create(item)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		d.onLocalCatalogChanged()
		logLocalResourceAction("create", created)
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: localResourceViewFromRecord(created, d.tunnelConfig.NetworkMode)})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (d *handlerDeps) handleLocalResourceByID(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodPut:
		var req localResourceSaveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
			return
		}
		item, err := localResourceRequestToRecord(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		updated, err := d.localResources.Update(id, item)
		if err != nil {
			if os.IsNotExist(err) || strings.Contains(strings.ToLower(err.Error()), "not exist") {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
				return
			}
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		d.onLocalCatalogChanged()
		logLocalResourceAction("update", updated)
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: localResourceViewFromRecord(updated, d.tunnelConfig.NetworkMode)})
	case http.MethodDelete:
		if err := d.localResources.Delete(id); err != nil {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
			return
		}
		d.onLocalCatalogChanged()
		log.Printf("local-resource action=delete resource_code=%s", strings.TrimSpace(id))
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success"})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}
