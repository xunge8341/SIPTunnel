package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

type SecuritySettingsPayload struct {
	Signer            string `json:"signer"`
	Encryption        string `json:"encryption"`
	VerifyIntervalMin int    `json:"verify_interval_min"`
}

type LicenseInfoPayload struct {
	Status              string   `json:"status"`
	ProductTypeName     string   `json:"product_type_name"`
	ExpireAt            string   `json:"expire_at"`
	ActiveAt            string   `json:"active_at"`
	MaintenanceExpireAt string   `json:"maintenance_expire_at"`
	LicenseTime         string   `json:"license_time"`
	ProductType         string   `json:"product_type"`
	LicenseType         string   `json:"license_type"`
	LicenseCounter      string   `json:"license_counter"`
	MachineCode         string   `json:"machine_code"`
	ProjectCode         string   `json:"project_code"`
	RegionInfo          string   `json:"region_info"`
	IndustryInfo        string   `json:"industry_info"`
	CustomerInfo        string   `json:"customer_info"`
	UserInfo            string   `json:"user_info"`
	ServerInfo          string   `json:"server_info"`
	Features            []string `json:"features"`
	LastVerifyResult    string   `json:"last_verify_result"`
	Summary1            string   `json:"summary1"`
	Summary2            string   `json:"summary2"`
	RawLicenseContent   string   `json:"raw_license_content,omitempty"`
}

type updateLicenseRequest struct {
	Token   string `json:"token"`
	Content string `json:"content"`
}

func defaultSecuritySettings() SecuritySettingsPayload {
	return SecuritySettingsPayload{Signer: "RSA+MD5", Encryption: "AES", VerifyIntervalMin: 30}
}

func defaultLicenseInfo() LicenseInfoPayload {
	return LicenseInfoPayload{Status: "未授权", ProductTypeName: "SIP隧道网关", ExpireAt: "-", ActiveAt: "-", MaintenanceExpireAt: "-", LicenseTime: "-", ProductType: "-", LicenseType: "-", LicenseCounter: "-", MachineCode: "-", ProjectCode: "-", RegionInfo: "-", IndustryInfo: "-", CustomerInfo: "-", UserInfo: "-", ServerInfo: "-", Features: []string{}, LastVerifyResult: "尚未导入授权"}
}

func loadJSONOrDefault[T any](path string, defaults T) T {
	buf, err := os.ReadFile(path)
	if err != nil {
		return defaults
	}
	var out T
	if err := unmarshalSecureJSON(buf, &out); err != nil {
		return defaults
	}
	return out
}

func saveJSON(path string, payload any) error {
	buf, err := marshalSecureJSON(payload)
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
		req.Signer = "RSA+MD5"
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
		content := strings.TrimSpace(firstNonEmpty(req.Content, req.Token))
		if content == "" {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "授权文件内容为空")
			return
		}
		hw := collectLicenseHardware(d.nodeStore.GetLocalNode().NodeID)
		license, err := verifyLicenseSummary(content, hw.MachineCode)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		d.mu.Lock()
		d.licenseInfo = license
		d.mu.Unlock()
		_ = saveJSON(d.licensePath, license)
		if d.licenseFilePath != "" {
			_ = os.WriteFile(d.licenseFilePath, []byte(content), 0o644)
		}
		d.recordOpsAudit(r, readOperator(r), "UPDATE_LICENSE", map[string]any{"status": license.Status, "machine_code": license.MachineCode, "project_code": license.ProjectCode})
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: license})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

type machineCodePayload struct {
	MachineCode string `json:"machine_code"`
	NodeID      string `json:"node_id"`
	Hostname    string `json:"hostname"`
	CPUID       string `json:"cpu_id"`
	BoardSerial string `json:"board_serial"`
	MACAddress  string `json:"mac_address"`
	RequestFile string `json:"request_file"`
}

func (d *handlerDeps) handleLicenseMachineCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	hw := collectLicenseHardware(d.nodeStore.GetLocalNode().NodeID)
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: machineCodePayload{MachineCode: hw.MachineCode, NodeID: hw.NodeID, Hostname: hw.Hostname, CPUID: hw.CPUID, BoardSerial: hw.BoardSerial, MACAddress: hw.MACAddress, RequestFile: hw.RequestFile}})
}
