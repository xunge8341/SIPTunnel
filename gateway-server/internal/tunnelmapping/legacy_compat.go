package tunnelmapping

import (
	"fmt"
	"net"
	"strings"
)

const (
	legacyDefaultPeerNodeID        = "legacy-peer"
	legacyDefaultLocalBindIP       = "127.0.0.1"
	legacyDefaultLocalBindPort     = 18080
	legacyDefaultRemoteTargetIP    = "127.0.0.1"
	legacyDefaultRemoteTargetPort  = 8080
	legacyDefaultConnectTimeoutMS  = 500
	legacyDefaultRequestTimeoutMS  = 3000
	legacyDefaultResponseTimeoutMS = 3000
	legacyDefaultMaxResponseBytes  = 20 * 1024 * 1024
)

// Deprecated: LegacyOpsRoute is the legacy /api/routes persistence payload.
type LegacyOpsRoute struct {
	APICode    string `json:"api_code"`
	HTTPMethod string `json:"http_method"`
	HTTPPath   string `json:"http_path"`
	Enabled    bool   `json:"enabled"`
}

// Deprecated: LegacyRouteConfig is the legacy httpinvoke route template payload.
type LegacyRouteConfig struct {
	APICode       string            `json:"api_code"`
	TargetService string            `json:"target_service"`
	TargetHost    string            `json:"target_host"`
	TargetPort    int               `json:"target_port"`
	HTTPMethod    string            `json:"http_method"`
	HTTPPath      string            `json:"http_path"`
	TimeoutMS     int               `json:"timeout_ms"`
	RetryTimes    int               `json:"retry_times"`
	HeaderMapping map[string]string `json:"header_mapping"`
	BodyMapping   map[string]string `json:"body_mapping"`
}

func MappingFromLegacyOpsRoute(route LegacyOpsRoute) (TunnelMapping, error) {
	if strings.TrimSpace(route.APICode) == "" || strings.TrimSpace(route.HTTPMethod) == "" || strings.TrimSpace(route.HTTPPath) == "" {
		return TunnelMapping{}, fmt.Errorf("legacy ops route requires api_code/http_method/http_path")
	}
	return TunnelMapping{
		MappingID:            route.APICode,
		Name:                 route.APICode,
		Enabled:              route.Enabled,
		PeerNodeID:           legacyDefaultPeerNodeID,
		LocalBindIP:          legacyDefaultLocalBindIP,
		LocalBindPort:        legacyDefaultLocalBindPort,
		LocalBasePath:        route.HTTPPath,
		RemoteTargetIP:       legacyDefaultRemoteTargetIP,
		RemoteTargetPort:     legacyDefaultRemoteTargetPort,
		RemoteBasePath:       route.HTTPPath,
		AllowedMethods:       []string{strings.ToUpper(route.HTTPMethod)},
		ConnectTimeoutMS:     legacyDefaultConnectTimeoutMS,
		RequestTimeoutMS:     legacyDefaultRequestTimeoutMS,
		ResponseTimeoutMS:    legacyDefaultResponseTimeoutMS,
		MaxRequestBodyBytes:  SmallBodyLimitBytes,
		MaxResponseBodyBytes: legacyDefaultMaxResponseBytes,
		Description:          "deprecated legacy ops route migrated to tunnel mapping",
	}, nil
}

func MappingFromLegacyRouteConfig(route LegacyRouteConfig) (TunnelMapping, error) {
	if strings.TrimSpace(route.APICode) == "" || strings.TrimSpace(route.HTTPMethod) == "" || strings.TrimSpace(route.HTTPPath) == "" {
		return TunnelMapping{}, fmt.Errorf("legacy route config requires api_code/http_method/http_path")
	}
	remoteIP := strings.TrimSpace(route.TargetHost)
	if net.ParseIP(remoteIP) == nil {
		remoteIP = legacyDefaultRemoteTargetIP
	}
	remotePort := route.TargetPort
	if remotePort <= 0 {
		remotePort = legacyDefaultRemoteTargetPort
	}
	timeoutMS := route.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = legacyDefaultRequestTimeoutMS
	}
	peerNodeID := strings.TrimSpace(route.TargetService)
	if peerNodeID == "" {
		peerNodeID = legacyDefaultPeerNodeID
	}
	return TunnelMapping{
		MappingID:            route.APICode,
		Name:                 route.APICode,
		Enabled:              true,
		PeerNodeID:           peerNodeID,
		LocalBindIP:          legacyDefaultLocalBindIP,
		LocalBindPort:        legacyDefaultLocalBindPort,
		LocalBasePath:        route.HTTPPath,
		RemoteTargetIP:       remoteIP,
		RemoteTargetPort:     remotePort,
		RemoteBasePath:       route.HTTPPath,
		AllowedMethods:       []string{strings.ToUpper(route.HTTPMethod)},
		ConnectTimeoutMS:     legacyDefaultConnectTimeoutMS,
		RequestTimeoutMS:     timeoutMS,
		ResponseTimeoutMS:    timeoutMS,
		MaxRequestBodyBytes:  SmallBodyLimitBytes,
		MaxResponseBodyBytes: legacyDefaultMaxResponseBytes,
		Description:          "deprecated legacy route config migrated to tunnel mapping",
	}, nil
}
