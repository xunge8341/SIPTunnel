package security

import "testing"

func TestHMACSigner(t *testing.T) {
	s := NewHMACSigner("secret")
	sig, err := s.Sign([]byte("abc"))
	if err != nil {
		t.Fatalf("Sign() err=%v", err)
	}
	if !s.Verify([]byte("abc"), sig) {
		t.Fatalf("Verify() expected true")
	}
	if s.Verify([]byte("abcd"), sig) {
		t.Fatalf("Verify() expected false")
	}
}
