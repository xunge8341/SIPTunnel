package server

import (
	"errors"
	"testing"
)

func TestShouldRetrySIPTCPSend(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "broken pipe", err: errors.New("write tcp: broken pipe"), want: true},
		{name: "connection reset", err: errors.New("read tcp: connection reset by peer"), want: true},
		{name: "windows forced close", err: errors.New("wsarecv: An existing connection was forcibly closed by the remote host."), want: true},
		{name: "plain error", err: errors.New("upstream rejected request"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldRetrySIPTCPSend(tc.err); got != tc.want {
				t.Fatalf("shouldRetrySIPTCPSend()=%v want=%v err=%v", got, tc.want, tc.err)
			}
		})
	}
}
