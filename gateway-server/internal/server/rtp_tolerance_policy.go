package server

import (
	"net/http"
	"strings"
	"time"
)

func preparedHeaderValue(prepared *mappingForwardRequest, key string) string {
	if prepared == nil {
		return ""
	}
	return strings.TrimSpace(prepared.Headers.Get(key))
}

type rtpTolerancePolicy struct {
	ProfileName   string
	RangePlayback bool
	ReorderWindow int
	LossTolerance int
	GapTimeout    time.Duration
	FECEnabled    bool
	FECGroupSize  int
}

func chooseRTPTolerancePolicy(prepared *mappingForwardRequest, req *http.Request, resp *http.Response) rtpTolerancePolicy {
	profile := classifyTrafficProfile(prepared, req, resp)
	policy := rtpTolerancePolicy{
		ProfileName:   string(profile),
		RangePlayback: profile == trafficProfileRangePlayback,
		ReorderWindow: boundaryRTPReorderWindowPackets(),
		LossTolerance: boundaryRTPLossTolerancePackets(),
		GapTimeout:    boundaryRTPGapTimeout(),
		FECEnabled:    boundaryRTPFECEnabled(),
		FECGroupSize:  boundaryRTPFECGroupPackets(),
	}
	if isGenericLargeDownloadCandidate(prepared, req, resp) || strings.EqualFold(preparedHeaderValue(prepared, downloadProfileHeader), "generic-rtp") {
		policy.ProfileName = "generic_download"
		policy.ReorderWindow = maxInt(policy.ReorderWindow, genericDownloadRTPReorderWindowPackets())
		policy.LossTolerance = maxInt(policy.LossTolerance, genericDownloadRTPLossTolerancePackets())
		if genericDownloadRTPGapTimeout() > policy.GapTimeout {
			policy.GapTimeout = genericDownloadRTPGapTimeout()
		}
		policy.FECEnabled = genericDownloadRTPFECEnabled()
		policy.FECGroupSize = genericDownloadRTPFECGroupPackets()
		return policy
	}
	if policy.RangePlayback {
		policy.ReorderWindow = maxInt(policy.ReorderWindow, boundaryPlaybackRTPReorderWindowPackets())
		policy.LossTolerance = maxInt(policy.LossTolerance, boundaryPlaybackRTPLossTolerancePackets())
		if boundaryPlaybackRTPGapTimeout() > policy.GapTimeout {
			policy.GapTimeout = boundaryPlaybackRTPGapTimeout()
		}
		policy.FECEnabled = boundaryPlaybackRTPFECEnabled()
		policy.FECGroupSize = boundaryPlaybackRTPFECGroupPackets()
	}
	return policy
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func escalateRangePlaybackTolerancePolicy(policy rtpTolerancePolicy) (rtpTolerancePolicy, bool) {
	if !policy.RangePlayback {
		return policy, false
	}
	next := policy
	changed := false
	if genericDownloadRTPReorderWindowPackets() > next.ReorderWindow {
		next.ReorderWindow = genericDownloadRTPReorderWindowPackets()
		changed = true
	}
	if genericDownloadRTPLossTolerancePackets() > next.LossTolerance {
		next.LossTolerance = genericDownloadRTPLossTolerancePackets()
		changed = true
	}
	if genericDownloadRTPGapTimeout() > next.GapTimeout {
		next.GapTimeout = genericDownloadRTPGapTimeout()
		changed = true
	}
	if genericDownloadRTPFECEnabled() && !next.FECEnabled {
		next.FECEnabled = true
		changed = true
	}
	if genericDownloadRTPFECGroupPackets() > next.FECGroupSize {
		next.FECGroupSize = genericDownloadRTPFECGroupPackets()
		changed = true
	}
	if changed {
		next.ProfileName = "range_playback_rescued"
	}
	return next, changed
}
