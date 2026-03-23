package netdiag

import (
	"context"
	"errors"
	"net"
	"strings"
)

func NormalizeErrorText(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func NormalizeError(err error) string {
	if err == nil {
		return ""
	}
	return NormalizeErrorText(err.Error())
}

func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return LooksLikeTimeoutText(NormalizeError(err))
}

func LooksLikeTimeoutText(text string) bool {
	text = NormalizeErrorText(text)
	return strings.Contains(text, "i/o timeout") ||
		strings.Contains(text, "timeout") ||
		strings.Contains(text, "deadline exceeded") ||
		strings.Contains(text, "超时") ||
		strings.Contains(text, "semaphore timeout period has expired")
}

func LooksLikeConnectionRefusedText(text string) bool {
	text = NormalizeErrorText(text)
	return strings.Contains(text, "actively refused") ||
		strings.Contains(text, "connection refused") ||
		strings.Contains(text, "被拒绝连接")
}

func LooksLikeConnectionClosedText(text string) bool {
	text = NormalizeErrorText(text)
	return strings.Contains(text, "forcibly closed") ||
		strings.Contains(text, "connection reset") ||
		strings.Contains(text, "broken pipe") ||
		strings.Contains(text, "eof") ||
		strings.Contains(text, "连接被中断") ||
		strings.Contains(text, "forcibly closed by the remote host") ||
		strings.Contains(text, "existing connection was forcibly closed") ||
		strings.Contains(text, "wsarecv") ||
		strings.Contains(text, "wsasend")
}

func LooksLikeLocalAddrExhaustedText(text string) bool {
	text = NormalizeErrorText(text)
	return strings.Contains(text, "only one usage of each socket address") ||
		strings.Contains(text, "address already in use") ||
		strings.Contains(text, "cannot assign requested address")
}

func LooksLikeDatagramTooLargeText(text string) bool {
	text = NormalizeErrorText(text)
	return strings.Contains(text, "message too long") ||
		strings.Contains(text, "message sent on a datagram socket was larger") ||
		(strings.Contains(text, "datagram socket") && strings.Contains(text, "larger than the internal message buffer"))
}

func LooksLikeDNSFailureText(text string) bool {
	text = NormalizeErrorText(text)
	return strings.Contains(text, "no such host") || strings.Contains(text, "server misbehaving")
}

func LooksLikeNetworkUnreachableText(text string) bool {
	text = NormalizeErrorText(text)
	return strings.Contains(text, "no route to host") ||
		strings.Contains(text, "network is unreachable") ||
		strings.Contains(text, "cannot assign requested address")
}
