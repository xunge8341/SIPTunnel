package server

import "testing"

func TestRTPFECBuildParseRoundTrip(t *testing.T) {
	packetLengths := []int{4, 3, 2, 4}
	parity := []byte{1, 2, 3, 4}
	payload, err := buildRTPFECPayload(100, packetLengths, parity)
	if err != nil {
		t.Fatalf("buildRTPFECPayload() error = %v", err)
	}
	base, parsedLengths, chunk, parsedParity, err := parseRTPFECPayload(payload)
	if err != nil {
		t.Fatalf("parseRTPFECPayload() error = %v", err)
	}
	if base != 100 || chunk != len(parity) {
		t.Fatalf("parsed header mismatch: base=%d chunk=%d", base, chunk)
	}
	if len(parsedLengths) != len(packetLengths) {
		t.Fatalf("parsed lengths count = %d, want %d", len(parsedLengths), len(packetLengths))
	}
	for i := range packetLengths {
		if parsedLengths[i] != packetLengths[i] {
			t.Fatalf("packetLengths[%d] = %d, want %d", i, parsedLengths[i], packetLengths[i])
		}
	}
	if len(parsedParity) != len(parity) {
		t.Fatalf("parity length mismatch: got %d want %d", len(parsedParity), len(parity))
	}
	for i := range parity {
		if parsedParity[i] != parity[i] {
			t.Fatalf("parity[%d] = %d, want %d", i, parsedParity[i], parity[i])
		}
	}
}

func TestRTPFECRecoverSingleMissingPacket(t *testing.T) {
	tracker := newRTPFECSingleParityTracker(128)
	packetA := []byte{0x10, 0x20, 0x30, 0x40}
	packetB := []byte{0x01, 0x02, 0x03, 0x04}
	packetC := []byte{0x11, 0x22, 0x33, 0x44}
	packetD := []byte{0x55, 0x66, 0x77, 0x88}

	parity := make([]byte, len(packetA))
	for _, payload := range [][]byte{packetA, packetB, packetC, packetD} {
		for i := 0; i < len(payload); i++ {
			parity[i] ^= payload[i]
		}
	}
	fecPayload, err := buildRTPFECPayload(200, []int{len(packetA), len(packetB), len(packetC), len(packetD)}, parity)
	if err != nil {
		t.Fatalf("buildRTPFECPayload() error = %v", err)
	}

	tracker.ObserveData(200, packetA, 200)
	tracker.ObserveData(201, packetB, 200)
	tracker.ObserveData(203, packetD, 200)

	recovered, err := tracker.ObserveFEC(fecPayload, 200)
	if err != nil {
		t.Fatalf("ObserveFEC() error = %v", err)
	}
	if len(recovered) != 1 {
		t.Fatalf("len(recovered) = %d, want 1", len(recovered))
	}
	if recovered[0].SequenceNumber != 202 {
		t.Fatalf("recovered seq = %d, want 202", recovered[0].SequenceNumber)
	}
	if len(recovered[0].Payload) != len(packetC) {
		t.Fatalf("recovered payload length = %d, want %d", len(recovered[0].Payload), len(packetC))
	}
	for i := range packetC {
		if recovered[0].Payload[i] != packetC[i] {
			t.Fatalf("recovered payload[%d] = %d, want %d", i, recovered[0].Payload[i], packetC[i])
		}
	}
}

func TestRTPFECRecoverSingleMissingShortPacket(t *testing.T) {
	tracker := newRTPFECSingleParityTracker(128)
	packetA := []byte{0x10, 0x20, 0x30, 0x40}
	packetB := []byte{0x01, 0x02}
	packetC := []byte{0x11, 0x22, 0x33, 0x44}
	packetD := []byte{0x55, 0x66, 0x77}
	maxLen := 4
	parity := make([]byte, maxLen)
	for _, payload := range [][]byte{packetA, packetB, packetC, packetD} {
		for i := 0; i < len(payload); i++ {
			parity[i] ^= payload[i]
		}
	}
	fecPayload, err := buildRTPFECPayload(300, []int{len(packetA), len(packetB), len(packetC), len(packetD)}, parity)
	if err != nil {
		t.Fatalf("buildRTPFECPayload() error = %v", err)
	}

	tracker.ObserveData(300, packetA, 300)
	tracker.ObserveData(302, packetC, 300)
	tracker.ObserveData(303, packetD, 300)

	recovered, err := tracker.ObserveFEC(fecPayload, 300)
	if err != nil {
		t.Fatalf("ObserveFEC() error = %v", err)
	}
	if len(recovered) != 1 {
		t.Fatalf("len(recovered) = %d, want 1", len(recovered))
	}
	if recovered[0].SequenceNumber != 301 {
		t.Fatalf("recovered seq = %d, want 301", recovered[0].SequenceNumber)
	}
	if len(recovered[0].Payload) != len(packetB) {
		t.Fatalf("recovered payload length = %d, want %d", len(recovered[0].Payload), len(packetB))
	}
	for i := range packetB {
		if recovered[0].Payload[i] != packetB[i] {
			t.Fatalf("recovered payload[%d] = %d, want %d", i, recovered[0].Payload[i], packetB[i])
		}
	}
}

func TestExpectedRTPSendProfileForPolicyUsesBoundaryForPlayback(t *testing.T) {
	profile := expectedRTPSendProfileForPolicy(rtpTolerancePolicy{ProfileName: string(trafficProfileRangePlayback), RangePlayback: true})
	if profile.name != "boundary-rtp" {
		t.Fatalf("profile.name = %q, want boundary-rtp", profile.name)
	}
}
