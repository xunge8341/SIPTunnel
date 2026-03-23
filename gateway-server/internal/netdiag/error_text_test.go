package netdiag

import (
	"errors"
	"testing"
)

func TestLooksLikeConnectionClosedText_CoversWindowsAndUnix(t *testing.T) {
	cases := []string{
		`read tcp 127.0.0.1:80: wsarecv: An existing connection was forcibly closed by the remote host.`,
		`write tcp 127.0.0.1:80: broken pipe`,
		`unexpected EOF`,
	}
	for _, tc := range cases {
		if !LooksLikeConnectionClosedText(tc) {
			t.Fatalf("expected connection-closed match for %q", tc)
		}
	}
}

func TestLooksLikeLocalAddrExhaustedText(t *testing.T) {
	if !LooksLikeLocalAddrExhaustedText(`dial tcp 10.0.0.1:80: bind: address already in use`) {
		t.Fatalf("expected local address exhaustion match")
	}
}

func TestIsTimeoutError_TextFallback(t *testing.T) {
	if !IsTimeoutError(errors.New("下游处理超时")) {
		t.Fatalf("expected timeout classification from text fallback")
	}
}
