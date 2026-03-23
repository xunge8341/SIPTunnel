package config

const (
	DefaultSIPListenPort                         = 5060
	DefaultRTPPortStart                          = 20000
	DefaultRTPPortEnd                            = 20999
	DefaultMappingPortStart                      = 21000
	DefaultMappingPortEnd                        = 21999
	DefaultUIListenPort                          = 18080
	RecommendedPortHeadroomPC                    = 20
	recommendedInlineSIPStartLineAndHeadersBytes = 180
	recommendedInlineMANSCDPXMLWrapBytes         = 96
)

func RTPPortPoolSize(start, end int) int {
	if start <= 0 || end <= 0 || start > end {
		return 0
	}
	return end - start + 1
}

func RecommendedStableRTPTransfers(start, end int) int {
	size := RTPPortPoolSize(start, end)
	if size == 0 {
		return 0
	}
	return size * (100 - RecommendedPortHeadroomPC) / 100
}

func effectiveInlineHeadroomRatio(c TransportTuningConfig) float64 {
	if c.InlineResponseHeadroomRatio > 0 {
		return c.InlineResponseHeadroomRatio
	}
	if c.InlineResponseHeadroomPercent > 0 {
		return float64(c.InlineResponseHeadroomPercent) / 100
	}
	return 0
}

func RecommendedInlineBodyBudgetBytes(c TransportTuningConfig) int64 {
	if c.InlineResponseUDPBudgetBytes <= 0 {
		return 0
	}
	budget := int64(c.InlineResponseUDPBudgetBytes)
	reserve := int64(c.InlineResponseSafetyReserveBytes)
	overhead := int64(c.InlineResponseEnvelopeOverheadBytes)
	sipHeaders := int64(recommendedInlineSIPStartLineAndHeadersBytes)
	xmlWrap := int64(recommendedInlineMANSCDPXMLWrapBytes)
	headroom := int64(float64(budget) * effectiveInlineHeadroomRatio(c))
	effectiveWireBudget := budget - reserve - overhead - sipHeaders - xmlWrap - headroom
	if effectiveWireBudget < 0 {
		effectiveWireBudget = 0
	}
	return effectiveWireBudget * 3 / 4
}

func RecommendedRTPReorderBufferBytes(c TransportTuningConfig) int64 {
	if c.BoundaryRTPPayloadBytes <= 0 {
		return 0
	}
	packets := c.BoundaryRTPReorderWindowPackets + c.BoundaryRTPLossTolerancePackets
	if packets <= 0 {
		return 0
	}
	return int64(c.BoundaryRTPPayloadBytes) * int64(packets)
}

func RecommendedPlaybackRTPReorderBufferBytes(c TransportTuningConfig) int64 {
	if c.BoundaryRTPPayloadBytes <= 0 {
		return 0
	}
	packets := c.BoundaryPlaybackRTPReorderWindowPackets + c.BoundaryPlaybackRTPLossTolerancePackets
	if packets <= 0 {
		return 0
	}
	return int64(c.BoundaryRTPPayloadBytes) * int64(packets)
}
