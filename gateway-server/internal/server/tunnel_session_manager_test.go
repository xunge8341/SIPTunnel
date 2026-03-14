package server

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRegistrar struct {
	registerCodes []int
	errOnHB       error
	regCalls      int
	hbCalls       int
}

func (f *fakeRegistrar) Register(context.Context, bool) (int, string, error) {
	idx := f.regCalls
	f.regCalls++
	if idx >= len(f.registerCodes) {
		return 200, "ok", nil
	}
	code := f.registerCodes[idx]
	if code == 401 {
		return 401, "challenge", nil
	}
	return code, "ok", nil
}

func (f *fakeRegistrar) Heartbeat(context.Context) error {
	f.hbCalls++
	return f.errOnHB
}

func TestTunnelSessionManagerRegisterChallengeAndSuccess(t *testing.T) {
	reg := &fakeRegistrar{registerCodes: []int{401, 200}}
	mgr := newTunnelSessionManager(reg, TunnelConfigPayload{HeartbeatIntervalSec: 1, RegisterRetryCount: 1, RegisterRetryIntervalSec: 1})
	mgr.registerOnce()
	mgr.registerOnce()

	s := mgr.Snapshot()
	if s.RegistrationStatus != "registered" {
		t.Fatalf("expected registered got %s", s.RegistrationStatus)
	}
	if s.HeartbeatStatus != "healthy" {
		t.Fatalf("expected healthy got %s", s.HeartbeatStatus)
	}
	if reg.regCalls != 2 {
		t.Fatalf("expected 2 register calls got %d", reg.regCalls)
	}
}

func TestTunnelSessionManagerHeartbeatFailureTriggersRetry(t *testing.T) {
	reg := &fakeRegistrar{registerCodes: []int{200}, errOnHB: errors.New("timeout")}
	mgr := newTunnelSessionManager(reg, TunnelConfigPayload{HeartbeatIntervalSec: 1, RegisterRetryCount: 2, RegisterRetryIntervalSec: 2})
	mgr.registerOnce()
	mgr.heartbeatOnce()

	s := mgr.Snapshot()
	if s.HeartbeatStatus != "timeout" {
		t.Fatalf("expected timeout got %s", s.HeartbeatStatus)
	}
	if s.RegistrationStatus != "failed" {
		t.Fatalf("expected failed got %s", s.RegistrationStatus)
	}
	if s.NextRetryTime == "" {
		t.Fatalf("expected retry time")
	}
}

func TestTunnelSessionManagerTickMarksTimeout(t *testing.T) {
	reg := &fakeRegistrar{registerCodes: []int{200}}
	mgr := newTunnelSessionManager(reg, TunnelConfigPayload{HeartbeatIntervalSec: 1, RegisterRetryCount: 1, RegisterRetryIntervalSec: 1})
	mgr.registerOnce()
	mgr.mu.Lock()
	mgr.lastHeartbeatTime = time.Now().UTC().Add(-5 * time.Second)
	mgr.mu.Unlock()
	mgr.tick()
	s := mgr.Snapshot()
	if s.HeartbeatStatus != "timeout" {
		t.Fatalf("expected timeout status got %s", s.HeartbeatStatus)
	}
}
