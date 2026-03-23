package nodeconfig

import (
	"fmt"
	"strings"

	"siptunnel/internal/config"
)

type LocalNodeConfig struct {
	NodeID           string             `json:"node_id"`
	NodeName         string             `json:"node_name"`
	NodeRole         string             `json:"node_role"`
	NetworkMode      config.NetworkMode `json:"network_mode"`
	SIPListenIP      string             `json:"sip_listen_ip"`
	SIPListenPort    int                `json:"sip_listen_port"`
	SIPTransport     string             `json:"sip_transport"`
	RTPListenIP      string             `json:"rtp_listen_ip"`
	RTPPortStart     int                `json:"rtp_port_start"`
	RTPPortEnd       int                `json:"rtp_port_end"`
	RTPTransport     string             `json:"rtp_transport"`
	MappingPortStart int                `json:"mapping_port_start"`
	MappingPortEnd   int                `json:"mapping_port_end"`
}

type PeerNodeConfig struct {
	PeerNodeID           string             `json:"peer_node_id"`
	PeerName             string             `json:"peer_name"`
	PeerSignalingIP      string             `json:"peer_signaling_ip"`
	PeerSignalingPort    int                `json:"peer_signaling_port"`
	PeerMediaIP          string             `json:"peer_media_ip"`
	PeerMediaPortStart   int                `json:"peer_media_port_start"`
	PeerMediaPortEnd     int                `json:"peer_media_port_end"`
	SupportedNetworkMode config.NetworkMode `json:"supported_network_mode"`
	Enabled              bool               `json:"enabled"`
}

func DefaultLocalNodeConfig() LocalNodeConfig {
	network := config.DefaultNetworkConfig()
	return LocalNodeConfig{
		NodeID:           "gateway-a-01",
		NodeName:         "Gateway A",
		NodeRole:         "gateway",
		NetworkMode:      network.Mode.Normalize(),
		SIPListenIP:      network.SIP.ListenIP,
		SIPListenPort:    network.SIP.ListenPort,
		SIPTransport:     strings.ToUpper(network.SIP.Transport),
		RTPListenIP:      network.RTP.ListenIP,
		RTPPortStart:     network.RTP.PortStart,
		RTPPortEnd:       network.RTP.PortEnd,
		RTPTransport:     strings.ToUpper(network.RTP.Transport),
		MappingPortStart: config.DefaultMappingPortStart,
		MappingPortEnd:   config.DefaultMappingPortEnd,
	}
}

func (c LocalNodeConfig) Validate() error {
	if strings.TrimSpace(c.NodeID) == "" || strings.TrimSpace(c.NodeName) == "" || strings.TrimSpace(c.NodeRole) == "" {
		return fmt.Errorf("node_id/node_name/node_role are required")
	}
	if err := c.NetworkMode.Normalize().Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(c.SIPListenIP) == "" || strings.TrimSpace(c.RTPListenIP) == "" {
		return fmt.Errorf("sip_listen_ip/rtp_listen_ip are required")
	}
	if c.SIPListenPort <= 0 || c.SIPListenPort > 65535 {
		return fmt.Errorf("sip_listen_port must be in (0,65535]")
	}
	if err := validateTransport(c.SIPTransport); err != nil {
		return fmt.Errorf("sip_transport %w", err)
	}
	if c.RTPPortStart <= 0 || c.RTPPortEnd <= 0 || c.RTPPortStart > c.RTPPortEnd || c.RTPPortEnd > 65535 {
		return fmt.Errorf("rtp port range is invalid")
	}
	if err := validateTransport(c.RTPTransport); err != nil {
		return fmt.Errorf("rtp_transport %w", err)
	}
	if c.MappingPortStart <= 0 || c.MappingPortEnd <= 0 || c.MappingPortStart > c.MappingPortEnd || c.MappingPortEnd > 65535 {
		return fmt.Errorf("mapping port range is invalid")
	}
	if c.SIPListenPort >= c.MappingPortStart && c.SIPListenPort <= c.MappingPortEnd {
		return fmt.Errorf("mapping port range [%d,%d] overlaps sip_listen_port %d", c.MappingPortStart, c.MappingPortEnd, c.SIPListenPort)
	}
	if rangesOverlap(c.MappingPortStart, c.MappingPortEnd, c.RTPPortStart, c.RTPPortEnd) {
		return fmt.Errorf("mapping port range [%d,%d] overlaps rtp port range [%d,%d]", c.MappingPortStart, c.MappingPortEnd, c.RTPPortStart, c.RTPPortEnd)
	}
	return nil
}

func (c LocalNodeConfig) Normalized() LocalNodeConfig {
	c.NodeID = strings.TrimSpace(c.NodeID)
	c.NodeName = strings.TrimSpace(c.NodeName)
	c.NodeRole = strings.TrimSpace(c.NodeRole)
	c.NetworkMode = c.NetworkMode.Normalize()
	c.SIPListenIP = strings.TrimSpace(c.SIPListenIP)
	c.SIPTransport = strings.ToUpper(strings.TrimSpace(c.SIPTransport))
	c.RTPListenIP = strings.TrimSpace(c.RTPListenIP)
	c.RTPTransport = strings.ToUpper(strings.TrimSpace(c.RTPTransport))
	if c.MappingPortStart == 0 {
		c.MappingPortStart = config.DefaultMappingPortStart
	}
	if c.MappingPortEnd == 0 {
		c.MappingPortEnd = config.DefaultMappingPortEnd
	}
	return c
}

func (c PeerNodeConfig) Validate() error {
	if strings.TrimSpace(c.PeerNodeID) == "" || strings.TrimSpace(c.PeerName) == "" {
		return fmt.Errorf("peer_node_id/peer_name are required")
	}
	if strings.TrimSpace(c.PeerSignalingIP) == "" || strings.TrimSpace(c.PeerMediaIP) == "" {
		return fmt.Errorf("peer signaling/media ip are required")
	}
	if c.PeerSignalingPort <= 0 || c.PeerSignalingPort > 65535 {
		return fmt.Errorf("peer_signaling_port must be in (0,65535]")
	}
	if c.PeerMediaPortStart <= 0 || c.PeerMediaPortEnd <= 0 || c.PeerMediaPortStart > c.PeerMediaPortEnd || c.PeerMediaPortEnd > 65535 {
		return fmt.Errorf("peer media port range is invalid")
	}
	if err := c.SupportedNetworkMode.Normalize().Validate(); err != nil {
		return fmt.Errorf("supported_network_mode %w", err)
	}
	return nil
}

func (c PeerNodeConfig) Normalized() PeerNodeConfig {
	c.PeerNodeID = strings.TrimSpace(c.PeerNodeID)
	c.PeerName = strings.TrimSpace(c.PeerName)
	c.PeerSignalingIP = strings.TrimSpace(c.PeerSignalingIP)
	c.PeerMediaIP = strings.TrimSpace(c.PeerMediaIP)
	c.SupportedNetworkMode = c.SupportedNetworkMode.Normalize()
	return c
}

func validateTransport(v string) error {
	t := strings.ToUpper(strings.TrimSpace(v))
	switch t {
	case "TCP", "UDP":
		return nil
	default:
		return fmt.Errorf("must be TCP or UDP")
	}
}

func rangesOverlap(aStart, aEnd, bStart, bEnd int) bool {
	if aStart > aEnd || bStart > bEnd {
		return false
	}
	return aStart <= bEnd && bStart <= aEnd
}
