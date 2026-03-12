package main

import "testing"

func TestReadPort(t *testing.T) {
	t.Setenv("GATEWAY_PORT", "")
	if got := readPort(); got != "18080" {
		t.Fatalf("readPort() default = %s, want 18080", got)
	}

	t.Setenv("GATEWAY_PORT", "19090")
	if got := readPort(); got != "19090" {
		t.Fatalf("readPort() with env = %s, want 19090", got)
	}

	t.Setenv("GATEWAY_PORT", "abc")
	if got := readPort(); got != "18080" {
		t.Fatalf("readPort() with invalid env = %s, want 18080", got)
	}
}
