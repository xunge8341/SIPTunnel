package netutil

import (
	"errors"
	"testing"
)

func TestIsAddrInUseError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "unix", err: errors.New("listen tcp :18080: bind: address already in use"), want: true},
		{name: "windows", err: errors.New("bind: Only one usage of each socket address (protocol/network address/port) is normally permitted."), want: true},
		{name: "other", err: errors.New("permission denied"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsAddrInUseError(tc.err); got != tc.want {
				t.Fatalf("IsAddrInUseError()=%v, want %v", got, tc.want)
			}
		})
	}
}
