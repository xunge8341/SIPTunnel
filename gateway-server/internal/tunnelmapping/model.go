package tunnelmapping

import (
	"errors"
	"fmt"
	"hash/crc32"
	"net"
	"strings"
)

// TunnelMapping 描述“本端入口 ↔ 对端目标”的 HTTP 隧道映射。
type TunnelMapping struct {
	MappingID                string   `json:"mapping_id"`
	DeviceID                 string   `json:"device_id,omitempty"`
	ResourceType             string   `json:"resource_type,omitempty"`
	Name                     string   `json:"name"`
	Enabled                  bool     `json:"enabled"`
	PeerNodeID               string   `json:"peer_node_id"`
	LocalBindIP              string   `json:"local_bind_ip"`
	LocalBindPort            int      `json:"local_bind_port"`
	LocalBasePath            string   `json:"local_base_path"`
	RemoteTargetIP           string   `json:"remote_target_ip"`
	RemoteTargetPort         int      `json:"remote_target_port"`
	RemoteBasePath           string   `json:"remote_base_path"`
	AllowedMethods           []string `json:"allowed_methods"`
	ResponseMode             string   `json:"response_mode,omitempty"`
	ConnectTimeoutMS         int      `json:"connect_timeout_ms"`
	RequestTimeoutMS         int      `json:"request_timeout_ms"`
	ResponseTimeoutMS        int      `json:"response_timeout_ms"`
	MaxInlineResponseBody    int64    `json:"max_inline_response_body,omitempty"`
	MaxRequestBodyBytes      int64    `json:"max_request_body_bytes"`
	MaxResponseBodyBytes     int64    `json:"max_response_body_bytes"`
	RequireStreamingResponse bool     `json:"require_streaming_response"`
	Description              string   `json:"description"`
	UpdatedAt                string   `json:"updated_at,omitempty"`
}

func (m TunnelMapping) EffectiveDeviceID() string {
	if strings.TrimSpace(m.DeviceID) != "" {
		return strings.TrimSpace(m.DeviceID)
	}
	return strings.TrimSpace(m.MappingID)
}

func syntheticGBDeviceID(seed string) string {
	trimmed := strings.TrimSpace(seed)
	if IsGBCode20(trimmed) {
		return trimmed
	}
	checksum := uint64(crc32.ChecksumIEEE([]byte(trimmed)))
	return fmt.Sprintf("3402000000%010d", checksum%10000000000)
}

func (m *TunnelMapping) normalizeGB28181Fields() {
	if m == nil {
		return
	}
	if strings.TrimSpace(m.DeviceID) == "" {
		m.DeviceID = syntheticGBDeviceID(m.MappingID)
	}
	m.ResourceType = NormalizeResourceType(m.ResourceType)
	m.ResponseMode = NormalizeResponseMode(m.ResponseMode)
	profile := DeriveBodyLimitProfile(m.ResponseMode, false)
	if m.MaxInlineResponseBody <= 0 {
		m.MaxInlineResponseBody = profile.MaxInlineResponseBody
	}
	if m.MaxRequestBodyBytes <= 0 {
		m.MaxRequestBodyBytes = profile.MaxRequestBodyBytes
	}
	if m.MaxResponseBodyBytes <= 0 {
		m.MaxResponseBodyBytes = profile.MaxResponseBodyBytes
	}
}

var defaultAllowedMethods = []string{"*"}

var browserUnsafePorts = map[int]struct{}{
	1: {}, 7: {}, 9: {}, 11: {}, 13: {}, 15: {}, 17: {}, 19: {}, 20: {}, 21: {}, 22: {}, 23: {},
	25: {}, 37: {}, 42: {}, 43: {}, 53: {}, 69: {}, 77: {}, 79: {}, 87: {}, 95: {}, 101: {}, 102: {},
	103: {}, 104: {}, 109: {}, 110: {}, 111: {}, 113: {}, 115: {}, 117: {}, 119: {}, 123: {}, 135: {},
	137: {}, 139: {}, 143: {}, 161: {}, 179: {}, 389: {}, 427: {}, 465: {}, 512: {}, 513: {}, 514: {},
	515: {}, 526: {}, 530: {}, 531: {}, 532: {}, 540: {}, 548: {}, 554: {}, 556: {}, 563: {}, 587: {},
	601: {}, 636: {}, 989: {}, 990: {}, 993: {}, 995: {}, 1719: {}, 1720: {}, 1723: {}, 2049: {},
	3659: {}, 4045: {}, 5060: {}, 5061: {}, 6000: {}, 6566: {}, 6665: {}, 6666: {}, 6667: {}, 6668: {},
	6669: {}, 6697: {}, 10080: {},
}

func (m *TunnelMapping) Normalize() {
	if m == nil {
		return
	}
	m.normalizeGB28181Fields()
	if len(m.AllowedMethods) == 0 {
		m.AllowedMethods = append([]string{}, defaultAllowedMethods...)
	}
	normalized := make([]string, 0, len(m.AllowedMethods))
	for _, method := range m.AllowedMethods {
		v := strings.ToUpper(strings.TrimSpace(method))
		if v == "" {
			continue
		}
		normalized = append(normalized, v)
	}
	if len(normalized) == 0 {
		normalized = append([]string{}, defaultAllowedMethods...)
	}
	m.AllowedMethods = normalized
}

func (m TunnelMapping) Validate() error {
	m.Normalize()
	if strings.TrimSpace(m.MappingID) == "" {
		return errors.New("mapping_id is required")
	}
	if strings.TrimSpace(m.EffectiveDeviceID()) == "" {
		return errors.New("device_id is required")
	}
	if !IsGBCode20(m.EffectiveDeviceID()) {
		return errors.New("device_id must be a 20-digit GB/T 28181 code")
	}
	if net.ParseIP(strings.TrimSpace(m.LocalBindIP)) == nil {
		return fmt.Errorf("local_bind_ip is invalid: %s", m.LocalBindIP)
	}
	if m.LocalBindPort <= 0 || m.LocalBindPort > 65535 {
		return errors.New("local_bind_port must be in 1..65535")
	}
	if isBrowserUnsafePort(m.LocalBindPort) {
		return fmt.Errorf("local_bind_port %d is blocked by common browsers (ERR_UNSAFE_PORT), please choose another port", m.LocalBindPort)
	}
	if strings.TrimSpace(m.LocalBasePath) == "" || !strings.HasPrefix(strings.TrimSpace(m.LocalBasePath), "/") {
		return errors.New("local_base_path must start with /")
	}
	if net.ParseIP(strings.TrimSpace(m.RemoteTargetIP)) == nil {
		return fmt.Errorf("remote_target_ip is invalid: %s", m.RemoteTargetIP)
	}
	if m.RemoteTargetPort <= 0 || m.RemoteTargetPort > 65535 {
		return errors.New("remote_target_port must be in 1..65535")
	}
	if strings.TrimSpace(m.RemoteBasePath) == "" || !strings.HasPrefix(strings.TrimSpace(m.RemoteBasePath), "/") {
		return errors.New("remote_base_path must start with /")
	}
	for _, method := range m.AllowedMethods {
		if strings.TrimSpace(method) == "" {
			return errors.New("allowed_methods must not contain empty value")
		}
	}
	if m.ConnectTimeoutMS <= 0 || m.RequestTimeoutMS <= 0 || m.ResponseTimeoutMS <= 0 {
		return errors.New("timeout fields must be positive")
	}
	if m.MaxRequestBodyBytes <= 0 || m.MaxResponseBodyBytes <= 0 {
		return errors.New("max_*_body_bytes must be positive")
	}
	return nil
}

func IsBrowserUnsafePort(port int) bool {
	return isBrowserUnsafePort(port)
}

func isBrowserUnsafePort(port int) bool {
	_, found := browserUnsafePorts[port]
	return found
}
