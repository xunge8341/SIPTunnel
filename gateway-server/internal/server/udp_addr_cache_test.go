package server

import "testing"

func TestCachedResolveUDPAddrReturnsClone(t *testing.T) {
	first, err := cachedResolveUDPAddr("127.0.0.1:5060")
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	second, err := cachedResolveUDPAddr("127.0.0.1:5060")
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if first == second {
		t.Fatal("expected cloned udp addr instances")
	}
	first.Port = 9999
	if second.Port == 9999 {
		t.Fatal("cached udp addr should not be mutated by caller")
	}
}
