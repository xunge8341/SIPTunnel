package startupsummary

import (
	"fmt"
	"strings"
	"time"
)

type Summary struct {
	NodeID           string           `json:"node_id"`
	ConfigPath       string           `json:"config_path"`
	ConfigSource     string           `json:"config_source"`
	UIMode           string           `json:"ui_mode"`
	UIURL            string           `json:"ui_url"`
	APIURL           string           `json:"api_url"`
	SIPListen        ListenEndpoint   `json:"sip_listen"`
	RTPListen        RTPListen        `json:"rtp_listen"`
	StorageDirs      StorageDirs      `json:"storage_dirs"`
	SelfCheckSummary SelfCheckSummary `json:"self_check_summary"`
}

type ListenEndpoint struct {
	IP        string `json:"ip"`
	Port      int    `json:"port"`
	Transport string `json:"transport"`
}

type RTPListen struct {
	IP        string `json:"ip"`
	PortRange string `json:"port_range"`
	Transport string `json:"transport"`
}

type StorageDirs struct {
	TempDir  string `json:"temp_dir"`
	FinalDir string `json:"final_dir"`
	AuditDir string `json:"audit_dir"`
	LogDir   string `json:"log_dir"`
}

type SelfCheckSummary struct {
	GeneratedAt string `json:"generated_at"`
	Overall     string `json:"overall"`
	Info        int    `json:"info"`
	Warn        int    `json:"warn"`
	Error       int    `json:"error"`
}

func (s Summary) ToLogText() string {
	generatedAt := s.SelfCheckSummary.GeneratedAt
	if generatedAt == "" {
		generatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	lines := []string{
		"startup summary:",
		fmt.Sprintf("- node_id: %s", safeValue(s.NodeID)),
		fmt.Sprintf("- config: path=%s source=%s", safeValue(s.ConfigPath), safeValue(s.ConfigSource)),
		fmt.Sprintf("- ui: mode=%s url=%s", safeValue(s.UIMode), safeValue(s.UIURL)),
		fmt.Sprintf("- api_url: %s", safeValue(s.APIURL)),
		fmt.Sprintf("- sip_listen: ip=%s port=%d transport=%s", safeValue(s.SIPListen.IP), s.SIPListen.Port, safeValue(s.SIPListen.Transport)),
		fmt.Sprintf("- rtp_listen: ip=%s port_range=%s transport=%s", safeValue(s.RTPListen.IP), safeValue(s.RTPListen.PortRange), safeValue(s.RTPListen.Transport)),
		fmt.Sprintf("- storage_dirs: temp=%s final=%s audit=%s log=%s", safeValue(s.StorageDirs.TempDir), safeValue(s.StorageDirs.FinalDir), safeValue(s.StorageDirs.AuditDir), safeValue(s.StorageDirs.LogDir)),
		fmt.Sprintf("- self_check_summary: generated_at=%s overall=%s info=%d warn=%d error=%d", generatedAt, safeValue(s.SelfCheckSummary.Overall), s.SelfCheckSummary.Info, s.SelfCheckSummary.Warn, s.SelfCheckSummary.Error),
	}
	return strings.Join(lines, "\n")
}

func safeValue(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	return v
}
