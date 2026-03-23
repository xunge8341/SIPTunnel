package server

import (
	"strings"

	"siptunnel/internal/netdiag"
)

const (
	failureReasonRTPGapTimeout   = "rtp_gap_timeout"
	failureReasonRTPSequenceGap  = "rtp_sequence_gap"
	failureReasonTimeout         = "timeout"
	failureReasonUnexpectedEOF   = "unexpected_eof"
	failureReasonConnectionReset = "connection_reset"
	failureReasonBrokenPipe      = "broken_pipe"
)

func normalizeFailureReason(reason string) string {
	return netdiag.NormalizeErrorText(reason)
}

func classifyCommonTransferFailure(err error) string {
	if err == nil {
		return ""
	}
	errText := normalizeFailureReason(err.Error())
	switch {
	case strings.Contains(errText, "rtp pending gap timeout"), strings.Contains(errText, "rtp pending gap on bye"):
		return failureReasonRTPGapTimeout
	case strings.Contains(errText, "rtp sequence discontinuity"):
		return failureReasonRTPSequenceGap
	case netdiag.LooksLikeTimeoutText(errText):
		return failureReasonTimeout
	case strings.Contains(errText, "unexpected eof"):
		return failureReasonUnexpectedEOF
	case strings.Contains(errText, "connection reset"):
		return failureReasonConnectionReset
	case strings.Contains(errText, "broken pipe"):
		return failureReasonBrokenPipe
	default:
		return ""
	}
}

func isSevereMediaFailureReason(reason string) bool {
	switch normalizeFailureReason(reason) {
	case failureReasonRTPGapTimeout, failureReasonRTPSequenceGap:
		return true
	default:
		return false
	}
}
