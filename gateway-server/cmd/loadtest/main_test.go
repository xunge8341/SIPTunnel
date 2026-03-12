package main

import "testing"

func TestNormalizeTargetsByMode(t *testing.T) {
	in := []string{"rtp-udp-upload", "rtp-tcp-upload", "http-invoke"}
	udp := normalizeTargets(in, "udp")
	if len(udp) != 2 || udp[0] != "rtp-udp-upload" {
		t.Fatalf("udp targets=%v", udp)
	}
	tcp := normalizeTargets(in, "tcp")
	if len(tcp) != 2 || tcp[0] != "rtp-tcp-upload" {
		t.Fatalf("tcp targets=%v", tcp)
	}
}
