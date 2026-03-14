package config

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"gopkg.in/yaml.v3"
)

type NetworkConfig struct {
	Mode NetworkMode `yaml:"mode"`
	SIP  SIPConfig   `yaml:"sip"`
	RTP  RTPConfig   `yaml:"rtp"`
}

type SIPConfig struct {
	Enabled                bool   `yaml:"enabled"`
	ListenIP               string `yaml:"listen_ip"`
	ListenPort             int    `yaml:"listen_port"`
	Transport              string `yaml:"transport"`
	AdvertiseIP            string `yaml:"advertise_ip"`
	Domain                 string `yaml:"domain"`
	MaxMessageBytes        int    `yaml:"max_message_bytes"`
	ReadTimeoutMS          int    `yaml:"read_timeout_ms"`
	WriteTimeoutMS         int    `yaml:"write_timeout_ms"`
	IdleTimeoutMS          int    `yaml:"idle_timeout_ms"`
	TCPKeepAliveEnabled    bool   `yaml:"tcp_keepalive_enabled"`
	TCPKeepAliveIntervalMS int    `yaml:"tcp_keepalive_interval_ms"`
	TCPReadBufferBytes     int    `yaml:"tcp_read_buffer_bytes"`
	TCPWriteBufferBytes    int    `yaml:"tcp_write_buffer_bytes"`
	MaxConnections         int    `yaml:"max_connections"`
}

const SIPUDPRecommendedMaxMessageBytes = 1300

type RTPConfig struct {
	Enabled              bool   `yaml:"enabled"`
	ListenIP             string `yaml:"listen_ip"`
	AdvertiseIP          string `yaml:"advertise_ip"`
	PortStart            int    `yaml:"port_start"`
	PortEnd              int    `yaml:"port_end"`
	Transport            string `yaml:"transport"`
	MaxPacketBytes       int    `yaml:"max_packet_bytes"`
	MaxInflightTransfers int    `yaml:"max_inflight_transfers"`
	ReceiveBufferBytes   int    `yaml:"receive_buffer_bytes"`
	TransferTimeoutMS    int    `yaml:"transfer_timeout_ms"`
	RetransmitMaxRounds  int    `yaml:"retransmit_max_rounds"`
	TCPReadTimeoutMS     int    `yaml:"tcp_read_timeout_ms"`
	TCPWriteTimeoutMS    int    `yaml:"tcp_write_timeout_ms"`
	TCPKeepAliveEnabled  bool   `yaml:"tcp_keepalive_enabled"`
	MaxTCPSessions       int    `yaml:"max_tcp_sessions"`
}

func DefaultNetworkConfig() NetworkConfig {
	return NetworkConfig{
		Mode: DefaultNetworkMode(),
		SIP: SIPConfig{
			Enabled:                true,
			ListenIP:               "0.0.0.0",
			ListenPort:             5060,
			Transport:              "TCP",
			AdvertiseIP:            "",
			Domain:                 "",
			MaxMessageBytes:        65535,
			ReadTimeoutMS:          5000,
			WriteTimeoutMS:         5000,
			IdleTimeoutMS:          60000,
			TCPKeepAliveEnabled:    true,
			TCPKeepAliveIntervalMS: 30000,
			TCPReadBufferBytes:     64 * 1024,
			TCPWriteBufferBytes:    64 * 1024,
			MaxConnections:         2048,
		},
		RTP: RTPConfig{
			Enabled:              true,
			ListenIP:             "0.0.0.0",
			AdvertiseIP:          "",
			PortStart:            20000,
			PortEnd:              20100,
			Transport:            "UDP",
			MaxPacketBytes:       1400,
			MaxInflightTransfers: 64,
			ReceiveBufferBytes:   4 * 1024 * 1024,
			TransferTimeoutMS:    30000,
			RetransmitMaxRounds:  3,
			TCPReadTimeoutMS:     5000,
			TCPWriteTimeoutMS:    5000,
			TCPKeepAliveEnabled:  true,
			MaxTCPSessions:       128,
		},
	}
}

func ParseNetworkConfigYAML(data []byte) (NetworkConfig, error) {
	cfg := DefaultNetworkConfig()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return NetworkConfig{}, fmt.Errorf("unmarshal network config: %w", err)
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return NetworkConfig{}, err
	}
	return cfg, nil
}

func (c *NetworkConfig) ApplyDefaults() {
	defaults := DefaultNetworkConfig()
	if strings.TrimSpace(string(c.Mode)) == "" {
		c.Mode = defaults.Mode
	}
	c.Mode = c.Mode.Normalize()
	c.SIP.applyDefaults(defaults.SIP)
	c.RTP.applyDefaults(defaults.RTP)
}

func (c NetworkConfig) Validate() error {
	var errs []error
	if err := c.SIP.Validate(); err != nil {
		errs = append(errs, err)
	}
	if err := c.Mode.Validate(); err != nil {
		errs = append(errs, err)
	}
	if err := c.RTP.Validate(); err != nil {
		errs = append(errs, err)
	}
	if err := validatePortConflict(c.SIP, c.RTP); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (c SIPConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	var errs []error
	if strings.TrimSpace(c.ListenIP) == "" {
		errs = append(errs, errors.New("sip.listen_ip is required"))
	} else if net.ParseIP(c.ListenIP) == nil {
		errs = append(errs, fmt.Errorf("sip.listen_ip %q is invalid", c.ListenIP))
	}
	if c.ListenPort < 1 || c.ListenPort > 65535 {
		errs = append(errs, fmt.Errorf("sip.listen_port %d out of range [1,65535]", c.ListenPort))
	}
	transport := strings.ToUpper(strings.TrimSpace(c.Transport))
	if transport != "TCP" && transport != "UDP" && transport != "TLS" {
		errs = append(errs, fmt.Errorf("sip.transport %q is unsupported", c.Transport))
	}
	if strings.TrimSpace(c.AdvertiseIP) != "" && net.ParseIP(c.AdvertiseIP) == nil {
		errs = append(errs, fmt.Errorf("sip.advertise_ip %q is invalid", c.AdvertiseIP))
	}
	if c.MaxMessageBytes <= 0 {
		errs = append(errs, fmt.Errorf("sip.max_message_bytes %d must be > 0", c.MaxMessageBytes))
	}
	if c.ReadTimeoutMS <= 0 {
		errs = append(errs, fmt.Errorf("sip.read_timeout_ms %d must be > 0", c.ReadTimeoutMS))
	}
	if c.WriteTimeoutMS <= 0 {
		errs = append(errs, fmt.Errorf("sip.write_timeout_ms %d must be > 0", c.WriteTimeoutMS))
	}
	if c.IdleTimeoutMS <= 0 {
		errs = append(errs, fmt.Errorf("sip.idle_timeout_ms %d must be > 0", c.IdleTimeoutMS))
	}
	if c.TCPKeepAliveIntervalMS <= 0 {
		errs = append(errs, fmt.Errorf("sip.tcp_keepalive_interval_ms %d must be > 0", c.TCPKeepAliveIntervalMS))
	}
	if c.TCPReadBufferBytes <= 0 {
		errs = append(errs, fmt.Errorf("sip.tcp_read_buffer_bytes %d must be > 0", c.TCPReadBufferBytes))
	}
	if c.TCPWriteBufferBytes <= 0 {
		errs = append(errs, fmt.Errorf("sip.tcp_write_buffer_bytes %d must be > 0", c.TCPWriteBufferBytes))
	}
	if c.MaxConnections <= 0 {
		errs = append(errs, fmt.Errorf("sip.max_connections %d must be > 0", c.MaxConnections))
	}
	return errors.Join(errs...)
}

func (c SIPConfig) UDPMessageSizeRisk() bool {
	transport := strings.ToUpper(strings.TrimSpace(c.Transport))
	return c.Enabled && transport == "UDP" && c.MaxMessageBytes > SIPUDPRecommendedMaxMessageBytes
}

func (c RTPConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	var errs []error
	if strings.TrimSpace(c.ListenIP) == "" {
		errs = append(errs, errors.New("rtp.listen_ip is required"))
	} else if net.ParseIP(c.ListenIP) == nil {
		errs = append(errs, fmt.Errorf("rtp.listen_ip %q is invalid", c.ListenIP))
	}
	if strings.TrimSpace(c.AdvertiseIP) != "" && net.ParseIP(c.AdvertiseIP) == nil {
		errs = append(errs, fmt.Errorf("rtp.advertise_ip %q is invalid", c.AdvertiseIP))
	}
	transport := strings.ToUpper(strings.TrimSpace(c.Transport))
	if transport != "UDP" && transport != "TCP" {
		errs = append(errs, fmt.Errorf("rtp.transport %q is unsupported", c.Transport))
	}
	if c.PortStart < 1 || c.PortStart > 65535 {
		errs = append(errs, fmt.Errorf("rtp.port_start %d out of range [1,65535]", c.PortStart))
	}
	if c.PortEnd < 1 || c.PortEnd > 65535 {
		errs = append(errs, fmt.Errorf("rtp.port_end %d out of range [1,65535]", c.PortEnd))
	}
	if c.PortStart > c.PortEnd {
		errs = append(errs, fmt.Errorf("rtp.port_start %d must be <= rtp.port_end %d", c.PortStart, c.PortEnd))
	}
	if c.MaxPacketBytes <= 0 {
		errs = append(errs, fmt.Errorf("rtp.max_packet_bytes %d must be > 0", c.MaxPacketBytes))
	}
	if c.MaxInflightTransfers <= 0 {
		errs = append(errs, fmt.Errorf("rtp.max_inflight_transfers %d must be > 0", c.MaxInflightTransfers))
	}
	if c.ReceiveBufferBytes <= 0 {
		errs = append(errs, fmt.Errorf("rtp.receive_buffer_bytes %d must be > 0", c.ReceiveBufferBytes))
	}
	if c.TransferTimeoutMS <= 0 {
		errs = append(errs, fmt.Errorf("rtp.transfer_timeout_ms %d must be > 0", c.TransferTimeoutMS))
	}
	if c.RetransmitMaxRounds < 0 {
		errs = append(errs, fmt.Errorf("rtp.retransmit_max_rounds %d must be >= 0", c.RetransmitMaxRounds))
	}
	if c.TCPReadTimeoutMS <= 0 {
		errs = append(errs, fmt.Errorf("rtp.tcp_read_timeout_ms %d must be > 0", c.TCPReadTimeoutMS))
	}
	if c.TCPWriteTimeoutMS <= 0 {
		errs = append(errs, fmt.Errorf("rtp.tcp_write_timeout_ms %d must be > 0", c.TCPWriteTimeoutMS))
	}
	if c.MaxTCPSessions <= 0 {
		errs = append(errs, fmt.Errorf("rtp.max_tcp_sessions %d must be > 0", c.MaxTCPSessions))
	}
	return errors.Join(errs...)
}

func validatePortConflict(sip SIPConfig, rtp RTPConfig) error {
	if !sip.Enabled || !rtp.Enabled {
		return nil
	}
	sipTransport := strings.ToUpper(strings.TrimSpace(sip.Transport))
	rtpTransport := strings.ToUpper(strings.TrimSpace(rtp.Transport))
	if sipTransport != rtpTransport {
		return nil
	}
	if !sameBindAddress(sip.ListenIP, rtp.ListenIP) {
		return nil
	}
	if sip.ListenPort >= rtp.PortStart && sip.ListenPort <= rtp.PortEnd {
		return fmt.Errorf("network port conflict: sip.listen_port %d overlaps rtp.port range [%d,%d] with same transport %s", sip.ListenPort, rtp.PortStart, rtp.PortEnd, sipTransport)
	}
	return nil
}

func sameBindAddress(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == b {
		return true
	}
	return a == "0.0.0.0" || b == "0.0.0.0" || a == "::" || b == "::"
}

func (c *SIPConfig) applyDefaults(d SIPConfig) {
	if strings.TrimSpace(c.ListenIP) == "" {
		c.ListenIP = d.ListenIP
	}
	if c.ListenPort == 0 {
		c.ListenPort = d.ListenPort
	}
	if strings.TrimSpace(c.Transport) == "" {
		c.Transport = d.Transport
	} else {
		c.Transport = strings.ToUpper(strings.TrimSpace(c.Transport))
	}
	if c.MaxMessageBytes == 0 {
		c.MaxMessageBytes = d.MaxMessageBytes
	}
	if c.ReadTimeoutMS == 0 {
		c.ReadTimeoutMS = d.ReadTimeoutMS
	}
	if c.WriteTimeoutMS == 0 {
		c.WriteTimeoutMS = d.WriteTimeoutMS
	}
	if c.IdleTimeoutMS == 0 {
		c.IdleTimeoutMS = d.IdleTimeoutMS
	}
	if c.TCPKeepAliveIntervalMS == 0 {
		c.TCPKeepAliveIntervalMS = d.TCPKeepAliveIntervalMS
	}
	if c.TCPReadBufferBytes == 0 {
		c.TCPReadBufferBytes = d.TCPReadBufferBytes
	}
	if c.TCPWriteBufferBytes == 0 {
		c.TCPWriteBufferBytes = d.TCPWriteBufferBytes
	}
	if c.MaxConnections == 0 {
		c.MaxConnections = d.MaxConnections
	}
}

func (c *RTPConfig) applyDefaults(d RTPConfig) {
	if strings.TrimSpace(c.ListenIP) == "" {
		c.ListenIP = d.ListenIP
	}
	if c.PortStart == 0 {
		c.PortStart = d.PortStart
	}
	if c.PortEnd == 0 {
		c.PortEnd = d.PortEnd
	}
	if strings.TrimSpace(c.Transport) == "" {
		c.Transport = d.Transport
	} else {
		c.Transport = strings.ToUpper(strings.TrimSpace(c.Transport))
	}
	if c.MaxPacketBytes == 0 {
		c.MaxPacketBytes = d.MaxPacketBytes
	}
	if c.MaxInflightTransfers == 0 {
		c.MaxInflightTransfers = d.MaxInflightTransfers
	}
	if c.ReceiveBufferBytes == 0 {
		c.ReceiveBufferBytes = d.ReceiveBufferBytes
	}
	if c.TransferTimeoutMS == 0 {
		c.TransferTimeoutMS = d.TransferTimeoutMS
	}
	if c.RetransmitMaxRounds == 0 {
		c.RetransmitMaxRounds = d.RetransmitMaxRounds
	}
	if c.TCPReadTimeoutMS == 0 {
		c.TCPReadTimeoutMS = d.TCPReadTimeoutMS
	}
	if c.TCPWriteTimeoutMS == 0 {
		c.TCPWriteTimeoutMS = d.TCPWriteTimeoutMS
	}
	if c.MaxTCPSessions == 0 {
		c.MaxTCPSessions = d.MaxTCPSessions
	}
}
