package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"siptunnel/internal/tunnelmapping"
)

func TestChooseLargeResponseDeliveryStrategy_StreamPrimaryForWholeFile(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/video.mp4", nil)
	prepared := &mappingForwardRequest{Method: http.MethodGet, Headers: make(http.Header)}
	resp := &http.Response{StatusCode: http.StatusOK, ContentLength: 335102685, Header: http.Header{"X-Siptunnel-Response-Mode": []string{"RTP"}, "Accept-Ranges": []string{"bytes"}, "Content-Type": []string{"video/mp4"}}}
	if got := chooseLargeResponseDeliveryStrategy(req, prepared, resp); got != deliveryStrategyStreamPrimary {
		t.Fatalf("strategy=%s, want %s", got, deliveryStrategyStreamPrimary)
	}
}

func TestChooseLargeResponseDeliveryStrategy_RangePrimaryForExplicitRange(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/video.mp4", nil)
	req.Header.Set("Range", "bytes=0-4194303")
	prepared := &mappingForwardRequest{Method: http.MethodGet, Headers: make(http.Header)}
	resp := &http.Response{StatusCode: http.StatusPartialContent, ContentLength: 4194304, Header: http.Header{"X-Siptunnel-Response-Mode": []string{"RTP"}, "Content-Range": []string{"bytes 0-4194303/335102685"}, "Content-Type": []string{"video/mp4"}}}
	if got := chooseLargeResponseDeliveryStrategy(req, prepared, resp); got != deliveryStrategyRangePrimary {
		t.Fatalf("strategy=%s, want %s", got, deliveryStrategyRangePrimary)
	}
}

func TestShouldFallbackToFixedWindow(t *testing.T) {
	err := &windowRecoveryError{Class: windowRecoveryFailureThresholdExceeded, Strategy: windowRecoveryStrategyRestartWindow, Err: io.ErrUnexpectedEOF}
	if !shouldFallbackToFixedWindow(err) {
		t.Fatal("expected fallback for threshold/restart error")
	}
	if shouldFallbackToFixedWindow(errors.New("plain error")) {
		t.Fatal("plain error should not trigger fixed window fallback")
	}
}

func TestSendPacketExpandsBufferForBoundaryChunk(t *testing.T) {
	sender := &rtpBodySender{}
	pc, err := netListenPacketLoopback()
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer pc.Close()
	sender.pc = pc
	rtpPacketBufferPool.Put(make([]byte, 0, 972))
	udpAddr := pc.LocalAddr().(*net.UDPAddr)
	payload := []byte(strings.Repeat("a", 1200))
	if err := sender.sendPacket(context.Background(), udpAddr, rtpPacketHeader{PayloadType: gb28181RTPPayloadType, SequenceNumber: 1, Timestamp: 1, SSRC: 1}, payload, nil, nil); err != nil {
		t.Fatalf("sendPacket: %v", err)
	}
}

func netListenPacketLoopback() (*net.UDPConn, error) {
	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		return nil, err
	}
	return pc, nil
}

func TestChooseLargeResponseDeliveryStrategy_StreamPrimaryForOpenEndedRange(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/video.mp4", nil)
	req.Header.Set("Range", "bytes=0-")
	prepared := &mappingForwardRequest{Method: http.MethodGet, Headers: make(http.Header)}
	resp := &http.Response{StatusCode: http.StatusPartialContent, ContentLength: 335102685, Header: http.Header{"X-Siptunnel-Response-Mode": []string{"RTP"}, "Content-Range": []string{"bytes 0-335102684/335102685"}, "Content-Type": []string{"video/mp4"}}}
	if got := chooseLargeResponseDeliveryStrategy(req, prepared, resp); got != deliveryStrategyStreamPrimary {
		t.Fatalf("strategy=%s, want %s", got, deliveryStrategyStreamPrimary)
	}
}

func TestChooseLargeResponseDeliveryStrategy_StreamPrimaryForOpenEndedGenericDownload(t *testing.T) {
	resetAdaptiveStateForTest()
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/archive.zip", nil)
	req.Header.Set("Range", "bytes=0-")
	prepared := &mappingForwardRequest{Method: http.MethodGet, TargetURL: req.URL, Headers: make(http.Header)}
	resp := &http.Response{StatusCode: http.StatusPartialContent, ContentLength: 335102685, Header: http.Header{"X-Siptunnel-Response-Mode": []string{"RTP"}, "Content-Range": []string{"bytes 0-335102684/335102685"}, "Content-Type": []string{"application/octet-stream"}, "Content-Disposition": []string{"attachment; filename=archive.zip"}}}
	if got := chooseLargeResponseDeliveryStrategy(req, prepared, resp); got != deliveryStrategyStreamPrimary {
		t.Fatalf("strategy=%s, want %s", got, deliveryStrategyStreamPrimary)
	}
}

func TestChooseLargeResponseDeliveryStrategy_RangePrimaryAfterRecentProbeAbort(t *testing.T) {
	resetAdaptiveStateForTest()
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/video.mp4", nil)
	req.Header.Set("Range", "bytes=0-")
	prepared := &mappingForwardRequest{Method: http.MethodGet, TargetURL: req.URL, Headers: make(http.Header)}
	resp := &http.Response{StatusCode: http.StatusPartialContent, ContentLength: 335102685, Header: http.Header{"X-Siptunnel-Response-Mode": []string{"RTP"}, "Content-Range": []string{"bytes 0-335102684/335102685"}, "Content-Type": []string{"video/mp4"}}}
	globalAdaptiveDelivery.observeProbeAbort(prepared, resp)
	if got := chooseLargeResponseDeliveryStrategy(req, prepared, resp); got != deliveryStrategyRangePrimary {
		t.Fatalf("strategy=%s, want %s", got, deliveryStrategyRangePrimary)
	}
}

func TestMaybeRewriteOpenEndedRangeForAdaptivePlayback(t *testing.T) {
	resetAdaptiveStateForTest()
	target, _ := url.Parse("http://example.com/video.mp4")
	prepared := &mappingForwardRequest{Method: http.MethodGet, TargetURL: target, Headers: make(http.Header)}
	prepared.Headers.Set("Range", "bytes=0-")
	resp := &http.Response{StatusCode: http.StatusPartialContent, ContentLength: 335102685, Header: http.Header{"Content-Range": []string{"bytes 0-335102684/335102685"}, "Content-Type": []string{"video/mp4"}}}
	globalAdaptiveDelivery.observeProbeAbort(prepared, resp)
	if !maybeRewriteOpenEndedRangeForAdaptivePlayback(prepared) {
		t.Fatal("expected rewrite")
	}
	if got := prepared.Headers.Get("Range"); got != "bytes=0-8388607" {
		t.Fatalf("range=%s, want bytes=0-8388607", got)
	}
}

func TestBuildFixedWindowPlanExtendsOpenEndedRangeToTotal(t *testing.T) {
	resetAdaptiveStateForTest()
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/video.mp4", nil)
	req.Header.Set("Range", "bytes=0-")
	prepared := &mappingForwardRequest{Method: http.MethodGet, Headers: make(http.Header)}
	resp := &http.Response{StatusCode: http.StatusPartialContent, ContentLength: 8388608, Header: http.Header{"X-Siptunnel-Response-Mode": []string{"RTP"}, "Content-Range": []string{"bytes 0-8388607/335102685"}, "Content-Type": []string{"video/mp4"}}}
	plan, ok := buildFixedWindowPlan(req, prepared, resp)
	if !ok {
		t.Fatal("expected fixed window plan")
	}
	if plan.responseStart != 0 || plan.responseEnd != 335102684 {
		t.Fatalf("plan range=%d-%d, want 0-335102684", plan.responseStart, plan.responseEnd)
	}
}

func TestCanReuseInitialFixedWindowSegmentForWholeObjectResponse(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/archive.zip", nil)
	segment := fixedWindowSegment{index: 0, start: 0, end: (8 << 20) - 1, rangeHeader: "bytes=0-8388607"}
	initialResp := &http.Response{StatusCode: http.StatusOK, ContentLength: 128 << 20, Body: io.NopCloser(bytes.NewReader(make([]byte, 9<<20))), Header: http.Header{"Accept-Ranges": []string{"bytes"}}}
	if !canReuseInitialFixedWindowSegment(req, initialResp, segment) {
		t.Fatal("expected whole-object 200 response to reuse first window head")
	}
}

func TestCopyInitialFixedWindowSegmentReusesWholeObjectHeadOnly(t *testing.T) {
	data := bytes.Repeat([]byte("a"), 9<<20)
	initialResp := &http.Response{StatusCode: http.StatusOK, ContentLength: int64(len(data)), Body: io.NopCloser(bytes.NewReader(data))}
	prepared := &mappingForwardRequest{Method: http.MethodGet}
	plan := fixedWindowPlan{responseStart: 0, responseEnd: int64(len(data) - 1), profileName: "generic-rtp", adaptiveProfile: "degraded"}
	segment := fixedWindowSegment{index: 0, start: 0, end: (8 << 20) - 1, rangeHeader: "bytes=0-8388607"}
	var out bytes.Buffer
	written, err := copyInitialFixedWindowSegment(initialResp, &out, make([]byte, 32<<10), "req-1", "trace-1", "map-1", "bulk", prepared, plan, segment)
	if err != nil {
		t.Fatalf("copy initial segment: %v", err)
	}
	if written != 8<<20 {
		t.Fatalf("written=%d, want %d", written, 8<<20)
	}
	if out.Len() != 8<<20 {
		t.Fatalf("buffer len=%d, want %d", out.Len(), 8<<20)
	}
}

func TestSegmentedDownloadProfileUsesExplicitChildHeaderAcrossSelectors(t *testing.T) {
	prepared := &mappingForwardRequest{Method: http.MethodGet, Headers: make(http.Header)}
	prepared.Headers.Set(downloadProfileHeader, "generic-rtp")
	mapping := tunnelmapping.TunnelMapping{ResponseMode: "INLINE"}
	responseProfile := segmentedDownloadProfileForResponse(prepared, nil, nil)
	laneProfile := segmentedDownloadProfileForLane(prepared, mapping)
	if responseProfile.name != "generic-rtp" || laneProfile.name != "generic-rtp" {
		t.Fatalf("expected explicit generic-rtp profile, got response=%s lane=%s", responseProfile.name, laneProfile.name)
	}
}

func TestSegmentChildParallelismTracksLaneProfileConcurrency(t *testing.T) {
	cfg := currentTransportTuning()
	mapping := tunnelmapping.TunnelMapping{ResponseMode: "RTP"}
	expected := maxIntVal(1, udpSegmentParallelismPerDevice())
	if cfg.BoundarySegmentConcurrency*2 > expected {
		expected = cfg.BoundarySegmentConcurrency * 2
	}
	if got := segmentChildParallelism(nil, mapping); got != expected {
		t.Fatalf("parallelism=%d, want %d", got, expected)
	}
}
