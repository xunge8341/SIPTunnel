package config

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

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
	if poolSize := RTPPortPoolSize(c.PortStart, c.PortEnd); poolSize > 0 {
		if poolSize < c.MaxInflightTransfers {
			errs = append(errs, fmt.Errorf("rtp port pool size %d is smaller than max_inflight_transfers %d", poolSize, c.MaxInflightTransfers))
		}
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
