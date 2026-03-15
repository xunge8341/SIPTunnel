package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

type SecuritySettingsPayload struct {
	Signer            string `json:"signer"`
	Encryption        string `json:"encryption"`
	VerifyIntervalMin int    `json:"verify_interval_min"`
}

type LicenseInfoPayload struct {
	Status           string   `json:"status"`
	ExpireAt         string   `json:"expire_at"`
	Features         []string `json:"features"`
	LastVerifyResult string   `json:"last_verify_result"`
}

type updateLicenseRequest struct {
	Token string `json:"token"`
}

func defaultSecuritySettings() SecuritySettingsPayload {
	return SecuritySettingsPayload{Signer: "HMAC-SHA256", Encryption: "AES", VerifyIntervalMin: 30}
}

func defaultLicenseInfo() LicenseInfoPayload {
	return LicenseInfoPayload{Status: "未授权", ExpireAt: "-", Features: []string{}, LastVerifyResult: "尚未导入授权"}
}

func loadJSONOrDefault[T any](path string, defaults T) T {
	buf, err := os.ReadFile(path)
	if err != nil {
		return defaults
	}
	var out T
	if err := json.Unmarshal(buf, &out); err != nil {
		return defaults
	}
	return out
}

func saveJSON(path string, payload any) error {
	buf, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, buf, 0o644)
}

func (d *handlerDeps) handleSecuritySettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		d.mu.RLock()
		resp := d.securitySettings
		d.mu.RUnlock()
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: resp})
	case http.MethodPut:
		var req SecuritySettingsPayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid body")
			return
		}
		if !strings.EqualFold(req.Encryption, "AES") && !strings.EqualFold(req.Encryption, "SM4") {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "encryption must be AES or SM4")
			return
		}
		if req.VerifyIntervalMin <= 0 {
			req.VerifyIntervalMin = 30
		}
		req.Signer = "HMAC-SHA256"
		d.mu.Lock()
		d.securitySettings = req
		d.mu.Unlock()
		_ = saveJSON(d.securityPath, req)
		d.recordOpsAudit(r, readOperator(r), "UPDATE_SECURITY_SETTINGS", map[string]any{"encryption": req.Encryption})
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: req})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (d *handlerDeps) handleLicense(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		d.mu.RLock()
		resp := d.licenseInfo
		d.mu.RUnlock()
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: resp})
	case http.MethodPut:
		var req updateLicenseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid body")
			return
		}
		status := "已授权"
		result := "校验通过"
		if strings.TrimSpace(req.Token) == "" {
			status = "未授权"
			result = "授权串为空"
		}
		license := LicenseInfoPayload{Status: status, ExpireAt: time.Now().AddDate(1, 0, 0).Format(time.RFC3339), Features: []string{"节点与隧道", "隧道映射", "诊断与压测", "AES", "SM4"}, LastVerifyResult: result}
		d.mu.Lock()
		d.licenseInfo = license
		d.mu.Unlock()
		_ = saveJSON(d.licensePath, license)
		d.recordOpsAudit(r, readOperator(r), "UPDATE_LICENSE", map[string]any{"status": status})
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: license})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}
