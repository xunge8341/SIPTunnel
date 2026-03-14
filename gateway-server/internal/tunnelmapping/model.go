package tunnelmapping

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// TunnelMapping 描述“本端入口 ↔ 对端目标”的 HTTP 隧道映射。
type TunnelMapping struct {
	MappingID            string   `json:"mapping_id"`
	Name                 string   `json:"name"`
	Enabled              bool     `json:"enabled"`
	PeerNodeID           string   `json:"peer_node_id"`
	LocalBindIP          string   `json:"local_bind_ip"`
	LocalBindPort        int      `json:"local_bind_port"`
	LocalBasePath        string   `json:"local_base_path"`
	RemoteTargetIP       string   `json:"remote_target_ip"`
	RemoteTargetPort     int      `json:"remote_target_port"`
	RemoteBasePath       string   `json:"remote_base_path"`
	AllowedMethods       []string `json:"allowed_methods"`
	ConnectTimeoutMS     int      `json:"connect_timeout_ms"`
	RequestTimeoutMS     int      `json:"request_timeout_ms"`
	ResponseTimeoutMS    int      `json:"response_timeout_ms"`
	MaxRequestBodyBytes  int64    `json:"max_request_body_bytes"`
	MaxResponseBodyBytes int64    `json:"max_response_body_bytes"`
	Description          string   `json:"description"`
}

func (m TunnelMapping) Validate() error {
	if strings.TrimSpace(m.MappingID) == "" {
		return errors.New("mapping_id is required")
	}
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(m.PeerNodeID) == "" {
		return errors.New("peer_node_id is required")
	}
	if net.ParseIP(strings.TrimSpace(m.LocalBindIP)) == nil {
		return fmt.Errorf("local_bind_ip is invalid: %s", m.LocalBindIP)
	}
	if m.LocalBindPort <= 0 || m.LocalBindPort > 65535 {
		return errors.New("local_bind_port must be in 1..65535")
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
	if len(m.AllowedMethods) == 0 {
		return errors.New("allowed_methods is required")
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
