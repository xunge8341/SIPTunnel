package loadtest

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestLoadtestStopGraceClamp(t *testing.T) {
	if got := loadtestStopGrace(0); got != loadtestStopGraceFloor {
		t.Fatalf("zero timeout grace=%s", got)
	}
	if got := loadtestStopGrace(100 * time.Millisecond); got != loadtestStopGraceFloor {
		t.Fatalf("small timeout grace=%s", got)
	}
	if got := loadtestStopGrace(30 * time.Second); got != loadtestStopGraceCeiling {
		t.Fatalf("large timeout grace=%s", got)
	}
}

func TestPrebuildRTPFrames(t *testing.T) {
	framesUDP, framesTCP := prebuildRTPFrames(buildChunks([]byte("hello world"), 4))
	if len(framesUDP) == 0 || len(framesTCP) == 0 {
		t.Fatalf("expected prebuilt frames udp=%d tcp=%d", len(framesUDP), len(framesTCP))
	}
	if got, want := binary.BigEndian.Uint32(framesTCP[0][:4]), uint32(len(framesUDP[0])); got != want {
		t.Fatalf("tcp length prefix=%d want=%d", got, want)
	}
}
