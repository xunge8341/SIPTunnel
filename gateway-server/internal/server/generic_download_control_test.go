package server

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"siptunnel/internal/config"
)

func resetGenericDownloadController() {
	globalGenericDownloadController = &genericDownloadController{
		activeTransfersPerDevice: make(map[string]int),
		activeSegmentsPerDevice:  make(map[string]int),
		states:                   make(map[string]*genericDownloadState),
	}
}

func TestGenericDownloadControllerDoesNotSplitWithinSameTransferByDefault(t *testing.T) {
	resetGenericDownloadController()
	lease1 := globalGenericDownloadController.acquire("devA", "http://example.com/a", "transfer-1")
	defer globalGenericDownloadController.release(lease1, nil)
	lease2 := globalGenericDownloadController.acquire("devA", "http://example.com/a", "transfer-1")
	defer globalGenericDownloadController.release(lease2, nil)

	if lease2.activeTransfersGlobal != 1 {
		t.Fatalf("expected same transfer to count as one active transfer, got %d", lease2.activeTransfersGlobal)
	}
	if lease2.activeSegmentsTransfer != 2 {
		t.Fatalf("expected two active segments in one transfer, got %d", lease2.activeSegmentsTransfer)
	}
	if lease2.effectiveTransferBPS <= 0 || lease2.effectiveBPS <= 0 {
		t.Fatalf("expected positive effective bitrate, got transfer=%d segment=%d", lease2.effectiveTransferBPS, lease2.effectiveBPS)
	}
	if lease2.sameTransferSplitEnabled {
		t.Fatal("expected same-transfer split to be disabled by default")
	}
	if lease2.sameTransferSplitApplied {
		t.Fatal("expected same-transfer split not to be applied by default")
	}
	if lease2.effectiveBPS != lease2.effectiveTransferBPS {
		t.Fatalf("expected per-segment bitrate to keep transfer budget when split is disabled, got transfer=%d segment=%d", lease2.effectiveTransferBPS, lease2.effectiveBPS)
	}
}

func TestGenericDownloadControllerCanSplitWithinSameTransferWhenExplicitlyEnabled(t *testing.T) {
	resetGenericDownloadController()
	cfg := config.DefaultTransportTuningConfig()
	cfg.GenericDownloadSameTransferSplitEnabled = true
	ApplyTransportTuning(cfg)
	defer ApplyTransportTuning(config.DefaultTransportTuningConfig())

	lease1 := globalGenericDownloadController.acquire("devA", "http://example.com/a", "transfer-1")
	defer globalGenericDownloadController.release(lease1, nil)
	lease2 := globalGenericDownloadController.acquire("devA", "http://example.com/a", "transfer-1")
	defer globalGenericDownloadController.release(lease2, nil)

	if !lease2.sameTransferSplitEnabled {
		t.Fatal("expected same-transfer split to be enabled")
	}
	if !lease2.sameTransferSplitApplied {
		t.Fatal("expected same-transfer split to be applied")
	}
	if lease2.effectiveBPS >= lease2.effectiveTransferBPS {
		t.Fatalf("expected per-segment bitrate to be lower than transfer bitrate when split is enabled, got transfer=%d segment=%d", lease2.effectiveTransferBPS, lease2.effectiveBPS)
	}
}

func TestGenericDownloadControllerMinFloorDoesNotBreakTotalCap(t *testing.T) {
	resetGenericDownloadController()
	leases := make([]genericDownloadLease, 0, 20)
	for i := 0; i < 20; i++ {
		lease := globalGenericDownloadController.acquire("devA", "http://example.com/a", "transfer-"+string(rune('a'+i)))
		leases = append(leases, lease)
	}
	for _, lease := range leases {
		defer globalGenericDownloadController.release(lease, nil)
	}
	last := leases[len(leases)-1]
	if last.floorApplied {
		t.Fatalf("expected min floor to stop applying once total cap can no longer cover all active transfers")
	}
	if total := genericDownloadTotalBitrate(); total > 0 && int64(last.activeTransfersGlobal) > 0 {
		share := total / int64(last.activeTransfersGlobal)
		if last.effectiveTransferBPS != share {
			t.Fatalf("expected effective transfer bitrate to follow global share once floor is disabled, got %d want %d", last.effectiveTransferBPS, share)
		}
	}
}

func TestGenericDownloadControllerTargetBreakerShapesNewTransfers(t *testing.T) {
	resetGenericDownloadController()
	target := "http://example.com/archive.zip"
	for i := 0; i < genericDownloadCircuitFailureThreshold(); i++ {
		globalGenericDownloadController.observeTargetResult(target, errors.New("boom"))
	}

	lease := globalGenericDownloadController.acquire("devA", target, "transfer-new")
	defer globalGenericDownloadController.release(lease, nil)

	if !lease.breakerOpen {
		t.Fatalf("expected target-level breaker to affect new transfer")
	}
	want := genericDownloadRTPBitrate() / 2
	if lease.effectiveTransferBPS != want {
		t.Fatalf("expected breaker to halve transfer bitrate, got %d want %d", lease.effectiveTransferBPS, want)
	}
}

func TestGenericDownloadControllerBreakerOpenForTargetIgnoresStaleTransferState(t *testing.T) {
	resetGenericDownloadController()
	target := "http://example.com/archive.zip"
	staleKey := genericDownloadStateKey("devA", target, "transfer-stale")
	globalGenericDownloadController.states[staleKey] = &genericDownloadState{
		DeviceID:         "devA",
		Target:           target,
		TransferID:       "transfer-stale",
		BreakerOpenUntil: time.Now().Add(30 * time.Second),
	}

	globalGenericDownloadController.observeTargetResult(target, nil)
	if globalGenericDownloadController.breakerOpenForTarget(target) {
		t.Fatalf("expected target-level breaker to depend on aggregate target state only")
	}
}

func TestChooseRTPTolerancePolicyGeneric(t *testing.T) {
	prepared := &mappingForwardRequest{Method: http.MethodGet, Headers: make(http.Header)}
	resp := &http.Response{Header: make(http.Header), ContentLength: 64 << 20}
	resp.Header.Set("Content-Type", "application/octet-stream")
	policy := chooseRTPTolerancePolicy(prepared, nil, resp)
	if policy.ProfileName != "generic_download" {
		t.Fatalf("expected generic_download profile, got %s", policy.ProfileName)
	}
	if policy.ReorderWindow < genericDownloadRTPReorderWindowPackets() {
		t.Fatalf("expected reorder window >= generic config")
	}
}

func TestGenericDownloadControllerSevereRTPFailureOpensTargetBreakerImmediately(t *testing.T) {
	resetGenericDownloadController()
	target := "http://example.com/archive.zip"
	globalGenericDownloadController.observeTargetResult(target, errors.New("rtp sequence discontinuity beyond tolerance expected=10 got=99 reorder_window=256 loss_tolerance=96"))

	if !globalGenericDownloadController.breakerOpenForTarget(target) {
		t.Fatalf("expected severe RTP sequence gap to open target breaker immediately")
	}
}

func TestGenericDownloadControllerIgnoresContextCanceled(t *testing.T) {
	resetGenericDownloadController()
	target := "http://example.com/archive.zip"
	for i := 0; i < genericDownloadCircuitFailureThreshold(); i++ {
		globalGenericDownloadController.observeTargetResult(target, errors.New("context canceled"))
	}
	if globalGenericDownloadController.breakerOpenForTarget(target) {
		t.Fatal("expected context canceled to be ignored by target breaker")
	}
}

func TestGenericDownloadControllerOpensSourceConstraintForSlowMultiSegmentTarget(t *testing.T) {
	resetGenericDownloadController()
	cfg := config.DefaultTransportTuningConfig()
	cfg.GenericDownloadSourceConstrainedAutoSingleflightEnabled = true
	ApplyTransportTuning(cfg)
	defer ApplyTransportTuning(config.DefaultTransportTuningConfig())
	target := "http://example.com/archive.zip"
	globalGenericDownloadController.observeSourceRead(target, "transfer-a", 3200000, 8388608, 2)
	if globalGenericDownloadController.sourceConstrainedForTarget(target) {
		t.Fatal("expected first slow observation to remain advisory only")
	}
	globalGenericDownloadController.observeSourceRead(target, "transfer-a", 3190000, 8388608, 2)
	if !globalGenericDownloadController.sourceConstrainedForTarget(target) {
		t.Fatal("expected repeated slow multi-segment observations to open source constraint")
	}
	lease := globalGenericDownloadController.acquire("devA", target, "transfer-b")
	defer globalGenericDownloadController.release(lease, nil)
	if !lease.sourceConstrained {
		t.Fatal("expected new lease to inherit source constrained marker")
	}
}

func TestGenericDownloadControllerClosesSourceConstraintAfterHealthySingleSegmentRead(t *testing.T) {
	resetGenericDownloadController()
	cfg := config.DefaultTransportTuningConfig()
	cfg.GenericDownloadSourceConstrainedAutoSingleflightEnabled = true
	ApplyTransportTuning(cfg)
	defer ApplyTransportTuning(config.DefaultTransportTuningConfig())
	target := "http://example.com/archive.zip"
	globalGenericDownloadController.observeSourceRead(target, "transfer-a", 3200000, 8388608, 2)
	globalGenericDownloadController.observeSourceRead(target, "transfer-a", 3190000, 8388608, 2)
	if !globalGenericDownloadController.sourceConstrainedForTarget(target) {
		t.Fatal("expected source constraint to be open")
	}
	globalGenericDownloadController.observeSourceRead(target, "transfer-a", 5000000, 8388608, 1)
	if globalGenericDownloadController.sourceConstrainedForTarget(target) {
		t.Fatal("expected healthy single-segment read to close source constraint")
	}
}
