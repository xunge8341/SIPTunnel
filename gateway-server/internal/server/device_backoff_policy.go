package server

import (
	"context"
	"log"
	"strings"
	"time"
)

const (
	devicePenaltyWindow           = 10 * time.Minute
	devicePenaltyFailureThreshold = 3
	devicePenaltyExtraWait        = 10 * time.Second
)

type devicePenaltyState struct {
	ConsecutiveFailures int
	LastFailureAt       time.Time
	LastReason          string
}

func normalizeDeviceID(deviceID string) string {
	return strings.ToLower(strings.TrimSpace(deviceID))
}

func (s *GB28181TunnelService) noteDeviceFailure(deviceID, reason string) {
	if s == nil {
		return
	}
	key := normalizeDeviceID(deviceID)
	if key == "" {
		return
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.devicePenalties[key]
	if st == nil || now.Sub(st.LastFailureAt) > devicePenaltyWindow {
		st = &devicePenaltyState{}
		s.devicePenalties[key] = st
	}
	st.ConsecutiveFailures++
	st.LastFailureAt = now
	st.LastReason = strings.TrimSpace(reason)
}

func (s *GB28181TunnelService) noteDeviceSuccess(deviceID string) {
	if s == nil {
		return
	}
	key := normalizeDeviceID(deviceID)
	if key == "" {
		return
	}
	s.mu.Lock()
	delete(s.devicePenalties, key)
	s.mu.Unlock()
}

func devicePenaltyDelayForReason(reason string) time.Duration {
	if normalizeFailureReason(reason) == failureReasonRTPSequenceGap {
		return genericDownloadPenaltyWait()
	}
	return devicePenaltyExtraWait
}

func (s *GB28181TunnelService) currentDevicePenalty(deviceID string) (bool, time.Duration, string) {
	if s == nil {
		return false, 0, ""
	}
	key := normalizeDeviceID(deviceID)
	if key == "" {
		return false, 0, ""
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.devicePenalties[key]
	if st == nil {
		return false, 0, ""
	}
	if now.Sub(st.LastFailureAt) > devicePenaltyWindow {
		delete(s.devicePenalties, key)
		return false, 0, ""
	}
	if st.ConsecutiveFailures < devicePenaltyFailureThreshold {
		return false, 0, ""
	}
	return true, devicePenaltyDelayForReason(st.LastReason), st.LastReason
}

func applyPenaltyDelay(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

func logDevicePenalty(stage, deviceID, reason string, delay time.Duration) {
	if strings.TrimSpace(deviceID) == "" || delay <= 0 {
		return
	}
	log.Printf("gb28181 tolerance stage=%s device_id=%s penalty_wait_ms=%d reason=%s", stage, strings.TrimSpace(deviceID), delay.Milliseconds(), firstNonEmpty(strings.TrimSpace(reason), "consecutive_failures"))
}
