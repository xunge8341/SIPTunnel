package config

import (
	"errors"
	"fmt"
	"strings"

	"siptunnel/internal/netbind"
)

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
	if err := c.TransportTuning.Validate(); err != nil {
		errs = append(errs, err)
	}
	if err := validatePortConflict(c.SIP, c.RTP); err != nil {
		errs = append(errs, err)
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
	if !netbind.SameBindAddress(sip.ListenIP, rtp.ListenIP) {
		return nil
	}
	if sip.ListenPort >= rtp.PortStart && sip.ListenPort <= rtp.PortEnd {
		return fmt.Errorf("network port conflict: sip.listen_port %d overlaps rtp.port range [%d,%d] with same transport %s", sip.ListenPort, rtp.PortStart, rtp.PortEnd, sipTransport)
	}
	return nil
}
