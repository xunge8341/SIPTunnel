package config

import (
	"fmt"
	"math"
	"strings"

	"gopkg.in/yaml.v3"
)

func DefaultTransportTuningConfig() TransportTuningConfig {
	return TransportTuningConfig{
		Mode:                                                    "secure_boundary",
		UDPControlMaxBytes:                                      1300,
		UDPCatalogMaxBytes:                                      1300,
		InlineResponseUDPBudgetBytes:                            1200,
		InlineResponseSafetyReserveBytes:                        220,
		InlineResponseEnvelopeOverheadBytes:                     320,
		InlineResponseHeadroomRatio:                             0.15,
		InlineResponseHeadroomPercent:                           15,
		UDPRequestParallelismPerDevice:                          6,
		UDPCallbackParallelismPerPeer:                           6,
		UDPBulkParallelismPerDevice:                             4,
		UDPSmallRequestMaxWaitMS:                                1500,
		UDPSegmentParallelismPerDevice:                          8,
		AdaptivePlaybackHotWindowBytes:                          8 << 20,
		AdaptivePlaybackSegmentCacheBytes:                       512 << 20,
		AdaptivePlaybackSegmentCacheTTLMS:                       45000,
		AdaptivePlaybackPrefetchSegments:                        1,
		AdaptivePrimarySegmentAfterFailures:                     2,
		AdaptiveLoopbackPlaybackSegmentConcurrency:              2,
		AdaptiveOpenEndedRangeInitialWindowBytes:                8 << 20,
		GenericSegmentedPrimaryThresholdBytes:                   8 << 20,
		GenericDownloadWindowBytes:                              2 << 20,
		GenericDownloadOpenEndedWindowBytes:                     8 << 20,
		GenericPrefetchSegments:                                 0,
		GenericDownloadSegmentConcurrency:                       1,
		GenericDownloadSameTransferSplitEnabled:                 false,
		GenericDownloadSourceConstrainedAutoSingleflightEnabled: false,
		GenericDownloadSegmentRetries:                           2,
		GenericDownloadResumeMaxAttempts:                        6,
		GenericDownloadResumePerRangeRetries:                    3,
		GenericDownloadPenaltyWaitMS:                            500,
		GenericDownloadTotalBitrateBps:                          32 * 1024 * 1024,
		GenericDownloadMinPerTransferBitrateBps:                 2 * 1024 * 1024,
		GenericDownloadCircuitFailureThreshold:                  3,
		GenericDownloadCircuitOpenMS:                            30000,
		GenericDownloadRTPBitrateBps:                            8 * 1024 * 1024,
		GenericDownloadRTPMinSpacingUS:                          650,
		GenericDownloadRTPSocketBufferBytes:                     32 << 20,
		GenericDownloadRTPReorderWindowPackets:                  512,
		GenericDownloadRTPLossTolerancePackets:                  192,
		GenericDownloadRTPGapTimeoutMS:                          900,
		GenericDownloadRTPFECEnabled:                            true,
		GenericDownloadRTPFECGroupPackets:                       8,
		BoundaryRTPPayloadBytes:                                 1200,
		BoundaryRTPBitrateBps:                                   16 * 1024 * 1024,
		BoundaryRTPMinSpacingUS:                                 250,
		BoundaryRTPSocketBufferBytes:                            16 << 20,
		BoundaryRTPReorderWindowPackets:                         128,
		BoundaryRTPLossTolerancePackets:                         48,
		BoundaryRTPGapTimeoutMS:                                 300,
		BoundaryRTPFECEnabled:                                   true,
		BoundaryRTPFECGroupPackets:                              8,
		BoundaryPlaybackRTPReorderWindowPackets:                 192,
		BoundaryPlaybackRTPLossTolerancePackets:                 64,
		BoundaryPlaybackRTPGapTimeoutMS:                         450,
		BoundaryPlaybackRTPFECEnabled:                           true,
		BoundaryPlaybackRTPFECGroupPackets:                      8,
		BoundaryFixedWindowBytes:                                4 << 20,
		BoundaryFixedWindowThreshold:                            256 << 20,
		BoundarySegmentConcurrency:                              4,
		BoundarySegmentRetries:                                  1,
		BoundaryResumeMaxAttempts:                               3,
		BoundaryResumePerRangeRetries:                           1,
		BoundaryResponseStartWaitMS:                             12000,
		BoundaryRangeResponseWaitMS:                             45000,
		BoundaryHTTPWindowBytes:                                 4 << 20,
		BoundaryHTTPWindowThreshold:                             256 << 20,
		BoundaryHTTPSegmentConcurrency:                          4,
		BoundaryHTTPSegmentRetries:                              1,
		StandardWindowBytes:                                     16 << 20,
		StandardWindowThreshold:                                 256 << 20,
		StandardSegmentConcurrency:                              4,
		StandardSegmentRetries:                                  1,
	}
}

func (c TransportTuningConfig) IsSecureBoundary() bool {
	mode := strings.ToLower(strings.TrimSpace(c.Mode))
	switch mode {
	case "", "secure_boundary", "boundary", "strict_boundary":
		return true
	default:
		return false
	}
}

func DefaultNetworkConfig() NetworkConfig {
	return NetworkConfig{
		Mode: DefaultNetworkMode(),
		SIP: SIPConfig{
			Enabled:                true,
			ListenIP:               "0.0.0.0",
			ListenPort:             DefaultSIPListenPort,
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
			PortStart:            DefaultRTPPortStart,
			PortEnd:              DefaultRTPPortEnd,
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
		TransportTuning: DefaultTransportTuningConfig(),
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
	c.TransportTuning.applyDefaults(defaults.TransportTuning)
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

func (c *TransportTuningConfig) ApplyDefaultsForRuntime(defaults TransportTuningConfig) {
	c.applyDefaults(defaults)
}

func (c *TransportTuningConfig) applyDefaults(defaults TransportTuningConfig) {
	if strings.TrimSpace(c.Mode) == "" {
		c.Mode = defaults.Mode
	}
	if c.UDPControlMaxBytes <= 0 {
		c.UDPControlMaxBytes = defaults.UDPControlMaxBytes
	}
	if c.UDPCatalogMaxBytes <= 0 {
		c.UDPCatalogMaxBytes = defaults.UDPCatalogMaxBytes
	}
	if c.InlineResponseUDPBudgetBytes <= 0 {
		c.InlineResponseUDPBudgetBytes = defaults.InlineResponseUDPBudgetBytes
	}
	if c.InlineResponseSafetyReserveBytes <= 0 {
		c.InlineResponseSafetyReserveBytes = defaults.InlineResponseSafetyReserveBytes
	}
	if c.InlineResponseEnvelopeOverheadBytes <= 0 {
		c.InlineResponseEnvelopeOverheadBytes = defaults.InlineResponseEnvelopeOverheadBytes
	}
	if c.InlineResponseHeadroomRatio <= 0 && c.InlineResponseHeadroomPercent > 0 {
		c.InlineResponseHeadroomRatio = float64(c.InlineResponseHeadroomPercent) / 100
	}
	if c.InlineResponseHeadroomRatio <= 0 {
		c.InlineResponseHeadroomRatio = defaults.InlineResponseHeadroomRatio
	}
	if c.InlineResponseHeadroomPercent <= 0 && c.InlineResponseHeadroomRatio > 0 {
		c.InlineResponseHeadroomPercent = int(math.Round(c.InlineResponseHeadroomRatio * 100))
	}
	if c.InlineResponseHeadroomPercent <= 0 {
		c.InlineResponseHeadroomPercent = defaults.InlineResponseHeadroomPercent
	}
	if c.UDPSmallRequestMaxWaitMS <= 0 {
		c.UDPSmallRequestMaxWaitMS = defaults.UDPSmallRequestMaxWaitMS
	}
	if c.UDPSegmentParallelismPerDevice <= 0 {
		c.UDPSegmentParallelismPerDevice = defaults.UDPSegmentParallelismPerDevice
	}
	if c.AdaptivePlaybackHotWindowBytes <= 0 {
		c.AdaptivePlaybackHotWindowBytes = defaults.AdaptivePlaybackHotWindowBytes
	}
	if c.AdaptivePlaybackSegmentCacheBytes <= 0 {
		c.AdaptivePlaybackSegmentCacheBytes = defaults.AdaptivePlaybackSegmentCacheBytes
	}
	if c.AdaptivePlaybackSegmentCacheTTLMS <= 0 {
		c.AdaptivePlaybackSegmentCacheTTLMS = defaults.AdaptivePlaybackSegmentCacheTTLMS
	}
	if c.AdaptivePlaybackPrefetchSegments < 0 {
		c.AdaptivePlaybackPrefetchSegments = defaults.AdaptivePlaybackPrefetchSegments
	}
	if c.AdaptivePrimarySegmentAfterFailures <= 0 {
		c.AdaptivePrimarySegmentAfterFailures = defaults.AdaptivePrimarySegmentAfterFailures
	}
	if c.UDPRequestParallelismPerDevice <= 0 {
		c.UDPRequestParallelismPerDevice = defaults.UDPRequestParallelismPerDevice
	}
	if c.UDPCallbackParallelismPerPeer <= 0 {
		c.UDPCallbackParallelismPerPeer = defaults.UDPCallbackParallelismPerPeer
	}
	if c.UDPBulkParallelismPerDevice <= 0 {
		c.UDPBulkParallelismPerDevice = defaults.UDPBulkParallelismPerDevice
	}
	if c.GenericSegmentedPrimaryThresholdBytes <= 0 {
		c.GenericSegmentedPrimaryThresholdBytes = defaults.GenericSegmentedPrimaryThresholdBytes
	}
	if c.GenericDownloadWindowBytes <= 0 {
		c.GenericDownloadWindowBytes = defaults.GenericDownloadWindowBytes
	}
	if c.GenericDownloadOpenEndedWindowBytes <= 0 {
		c.GenericDownloadOpenEndedWindowBytes = defaults.GenericDownloadOpenEndedWindowBytes
	}
	if c.GenericPrefetchSegments < 0 {
		c.GenericPrefetchSegments = defaults.GenericPrefetchSegments
	}
	if c.GenericDownloadSegmentConcurrency <= 0 {
		c.GenericDownloadSegmentConcurrency = defaults.GenericDownloadSegmentConcurrency
	}
	if c.GenericDownloadSegmentRetries < 0 {
		c.GenericDownloadSegmentRetries = defaults.GenericDownloadSegmentRetries
	}
	if c.GenericDownloadResumeMaxAttempts <= 0 {
		c.GenericDownloadResumeMaxAttempts = defaults.GenericDownloadResumeMaxAttempts
	}
	if c.GenericDownloadResumePerRangeRetries <= 0 {
		c.GenericDownloadResumePerRangeRetries = defaults.GenericDownloadResumePerRangeRetries
	}
	if c.GenericDownloadPenaltyWaitMS <= 0 {
		c.GenericDownloadPenaltyWaitMS = defaults.GenericDownloadPenaltyWaitMS
	}
	if c.GenericDownloadTotalBitrateBps <= 0 {
		c.GenericDownloadTotalBitrateBps = defaults.GenericDownloadTotalBitrateBps
	}
	if c.GenericDownloadMinPerTransferBitrateBps <= 0 {
		c.GenericDownloadMinPerTransferBitrateBps = defaults.GenericDownloadMinPerTransferBitrateBps
	}
	if c.GenericDownloadCircuitFailureThreshold <= 0 {
		c.GenericDownloadCircuitFailureThreshold = defaults.GenericDownloadCircuitFailureThreshold
	}
	if c.GenericDownloadCircuitOpenMS <= 0 {
		c.GenericDownloadCircuitOpenMS = defaults.GenericDownloadCircuitOpenMS
	}
	if c.GenericDownloadRTPBitrateBps <= 0 {
		c.GenericDownloadRTPBitrateBps = defaults.GenericDownloadRTPBitrateBps
	}
	if c.GenericDownloadRTPMinSpacingUS <= 0 {
		c.GenericDownloadRTPMinSpacingUS = defaults.GenericDownloadRTPMinSpacingUS
	}
	if c.GenericDownloadRTPSocketBufferBytes <= 0 {
		c.GenericDownloadRTPSocketBufferBytes = defaults.GenericDownloadRTPSocketBufferBytes
	}
	if c.GenericDownloadRTPReorderWindowPackets <= 0 {
		c.GenericDownloadRTPReorderWindowPackets = defaults.GenericDownloadRTPReorderWindowPackets
	}
	if c.GenericDownloadRTPLossTolerancePackets <= 0 {
		c.GenericDownloadRTPLossTolerancePackets = defaults.GenericDownloadRTPLossTolerancePackets
	}
	if c.GenericDownloadRTPGapTimeoutMS <= 0 {
		c.GenericDownloadRTPGapTimeoutMS = defaults.GenericDownloadRTPGapTimeoutMS
	}
	if c.GenericDownloadRTPFECGroupPackets <= 0 {
		c.GenericDownloadRTPFECGroupPackets = defaults.GenericDownloadRTPFECGroupPackets
	}
	if c.BoundaryRTPPayloadBytes <= 0 {
		c.BoundaryRTPPayloadBytes = defaults.BoundaryRTPPayloadBytes
	}
	if c.BoundaryRTPBitrateBps <= 0 {
		c.BoundaryRTPBitrateBps = defaults.BoundaryRTPBitrateBps
	}
	if c.BoundaryRTPMinSpacingUS <= 0 {
		c.BoundaryRTPMinSpacingUS = defaults.BoundaryRTPMinSpacingUS
	}
	if c.BoundaryRTPSocketBufferBytes <= 0 {
		c.BoundaryRTPSocketBufferBytes = defaults.BoundaryRTPSocketBufferBytes
	}
	if c.BoundaryRTPReorderWindowPackets <= 0 {
		c.BoundaryRTPReorderWindowPackets = defaults.BoundaryRTPReorderWindowPackets
	}
	if c.BoundaryRTPLossTolerancePackets <= 0 {
		c.BoundaryRTPLossTolerancePackets = defaults.BoundaryRTPLossTolerancePackets
	}
	if c.BoundaryRTPGapTimeoutMS <= 0 {
		c.BoundaryRTPGapTimeoutMS = defaults.BoundaryRTPGapTimeoutMS
	}
	if c.BoundaryRTPFECGroupPackets <= 0 {
		c.BoundaryRTPFECGroupPackets = defaults.BoundaryRTPFECGroupPackets
	}
	if c.BoundaryPlaybackRTPReorderWindowPackets <= 0 {
		c.BoundaryPlaybackRTPReorderWindowPackets = defaults.BoundaryPlaybackRTPReorderWindowPackets
	}
	if c.BoundaryPlaybackRTPLossTolerancePackets <= 0 {
		c.BoundaryPlaybackRTPLossTolerancePackets = defaults.BoundaryPlaybackRTPLossTolerancePackets
	}
	if c.BoundaryPlaybackRTPGapTimeoutMS <= 0 {
		c.BoundaryPlaybackRTPGapTimeoutMS = defaults.BoundaryPlaybackRTPGapTimeoutMS
	}
	if c.BoundaryPlaybackRTPFECGroupPackets <= 0 {
		c.BoundaryPlaybackRTPFECGroupPackets = defaults.BoundaryPlaybackRTPFECGroupPackets
	}
	if c.BoundaryFixedWindowBytes <= 0 {
		c.BoundaryFixedWindowBytes = defaults.BoundaryFixedWindowBytes
	}
	if c.BoundaryFixedWindowThreshold <= 0 {
		c.BoundaryFixedWindowThreshold = defaults.BoundaryFixedWindowThreshold
	}
	if c.BoundarySegmentConcurrency <= 0 {
		c.BoundarySegmentConcurrency = defaults.BoundarySegmentConcurrency
	}
	if c.BoundarySegmentRetries <= 0 {
		c.BoundarySegmentRetries = defaults.BoundarySegmentRetries
	}
	if c.BoundaryResumeMaxAttempts <= 0 {
		c.BoundaryResumeMaxAttempts = defaults.BoundaryResumeMaxAttempts
	}
	if c.BoundaryResumePerRangeRetries <= 0 {
		c.BoundaryResumePerRangeRetries = defaults.BoundaryResumePerRangeRetries
	}
	if c.BoundaryResponseStartWaitMS <= 0 {
		c.BoundaryResponseStartWaitMS = defaults.BoundaryResponseStartWaitMS
	}
	if c.BoundaryRangeResponseWaitMS <= 0 {
		c.BoundaryRangeResponseWaitMS = defaults.BoundaryRangeResponseWaitMS
	}
	if c.BoundaryHTTPWindowBytes <= 0 {
		c.BoundaryHTTPWindowBytes = defaults.BoundaryHTTPWindowBytes
	}
	if c.BoundaryHTTPWindowThreshold <= 0 {
		c.BoundaryHTTPWindowThreshold = defaults.BoundaryHTTPWindowThreshold
	}
	if c.BoundaryHTTPSegmentConcurrency <= 0 {
		c.BoundaryHTTPSegmentConcurrency = defaults.BoundaryHTTPSegmentConcurrency
	}
	if c.BoundaryHTTPSegmentRetries <= 0 {
		c.BoundaryHTTPSegmentRetries = defaults.BoundaryHTTPSegmentRetries
	}
	if c.StandardWindowBytes <= 0 {
		c.StandardWindowBytes = defaults.StandardWindowBytes
	}
	if c.StandardWindowThreshold <= 0 {
		c.StandardWindowThreshold = defaults.StandardWindowThreshold
	}
	if c.StandardSegmentConcurrency <= 0 {
		c.StandardSegmentConcurrency = defaults.StandardSegmentConcurrency
	}
	if c.StandardSegmentRetries <= 0 {
		c.StandardSegmentRetries = defaults.StandardSegmentRetries
	}
}
