package netbind

import "testing"

func TestSameBindAddress(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{name: "exact", a: "127.0.0.1", b: "127.0.0.1", want: true},
		{name: "wildcard ipv4", a: "0.0.0.0", b: "10.0.0.8", want: true},
		{name: "wildcard ipv6", a: "::", b: "fe80::1", want: true},
		{name: "different explicit", a: "127.0.0.1", b: "127.0.0.2", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SameBindAddress(tt.a, tt.b); got != tt.want {
				t.Fatalf("SameBindAddress(%q, %q)=%v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
