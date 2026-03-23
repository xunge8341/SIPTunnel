package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"siptunnel/internal/config"
)

func resetAdaptiveStateForTest() {
	globalAdaptiveDelivery = newAdaptiveDeliveryController()
	ApplyTransportTuning(config.DefaultTransportTuningConfig())
}

func makeAdaptivePrepared(t *testing.T) (*mappingForwardRequest, *http.Request, *http.Response) {
	t.Helper()
	target, err := url.Parse("http://example.com/video.mp4")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	req, _ := http.NewRequest(http.MethodGet, target.String(), nil)
	prepared := &mappingForwardRequest{Method: http.MethodGet, TargetURL: target, Headers: make(http.Header)}
	resp := &http.Response{StatusCode: http.StatusOK, ContentLength: 335102685, Header: http.Header{"Content-Type": []string{"video/mp4"}, "Accept-Ranges": []string{"bytes"}, "X-Siptunnel-Response-Mode": []string{"RTP"}}}
	return prepared, req, resp
}

func TestAdaptiveStrategyPromotesSegmentedPrimaryAfterFailures(t *testing.T) {
	resetAdaptiveStateForTest()
	prepared, req, resp := makeAdaptivePrepared(t)
	globalAdaptiveDelivery.observeStreamResult(prepared, req, resp, deliveryStrategyStreamPrimary, errors.New("io timeout"))
	globalAdaptiveDelivery.observeStreamResult(prepared, req, resp, deliveryStrategyStreamPrimary, errors.New("io timeout"))
	if got := chooseLargeResponseDeliveryStrategy(req, prepared, resp); got != deliveryStrategyAdaptiveSegmentedPrimary {
		t.Fatalf("strategy=%s, want %s", got, deliveryStrategyAdaptiveSegmentedPrimary)
	}
}

func TestAdaptiveSegmentCacheSharesInflightAndHit(t *testing.T) {
	cache := newAdaptiveSegmentCache()
	cache.setLimit(8 << 20)
	calls := 0
	loader := func() ([]byte, error) {
		calls++
		time.Sleep(10 * time.Millisecond)
		return []byte("segment-data"), nil
	}
	ctx := context.Background()
	ch := make(chan []any, 2)
	go func() {
		data, hit, shared, err := cache.getOrLoad(ctx, "k", time.Second, loader)
		ch <- []any{data, hit, shared, err}
	}()
	go func() {
		data, hit, shared, err := cache.getOrLoad(ctx, "k", time.Second, loader)
		ch <- []any{data, hit, shared, err}
	}()
	for i := 0; i < 2; i++ {
		res := <-ch
		if res[3] != nil {
			t.Fatalf("load err: %v", res[3])
		}
		if string(res[0].([]byte)) != "segment-data" {
			t.Fatalf("unexpected data: %q", string(res[0].([]byte)))
		}
	}
	if calls != 1 {
		t.Fatalf("loader calls=%d, want 1", calls)
	}
	data, hit, shared, err := cache.getOrLoad(ctx, "k", time.Second, loader)
	if err != nil {
		t.Fatalf("cache hit err: %v", err)
	}
	if !hit || shared || string(data) != "segment-data" {
		t.Fatalf("unexpected cache state hit=%t shared=%t data=%q", hit, shared, string(data))
	}
}

func TestAdaptiveSnapshotUsesConservativePlanForOpenEndedGenericDownload(t *testing.T) {
	resetAdaptiveStateForTest()
	tuning := config.DefaultTransportTuningConfig()
	ApplyTransportTuning(tuning)
	target, err := url.Parse("http://example.com/archive.zip")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	req, _ := http.NewRequest(http.MethodGet, target.String(), nil)
	req.Header.Set("Range", "bytes=0-")
	prepared := &mappingForwardRequest{Method: http.MethodGet, TargetURL: target, Headers: make(http.Header)}
	resp := &http.Response{StatusCode: http.StatusPartialContent, ContentLength: 128 << 20, Header: http.Header{"Content-Type": []string{"application/octet-stream"}, "Content-Disposition": []string{"attachment; filename=archive.zip"}, "Content-Range": []string{"bytes 0-4194303/134217728"}}}
	decision := globalAdaptiveDelivery.snapshot(prepared, req, resp, fixedWindowPlan{concurrency: 2, window: genericDownloadWindowBytes()})
	if decision.PreferSegmentedPrimary {
		t.Fatal("expected stream-first recovery plan")
	}
	if decision.SegmentConcurrency != 1 {
		t.Fatalf("segment concurrency=%d, want 1", decision.SegmentConcurrency)
	}
	if decision.PrefetchSegments != 0 {
		t.Fatalf("prefetch=%d, want 0", decision.PrefetchSegments)
	}
	if decision.WindowBytes != genericDownloadConservativeWindowBytes() {
		t.Fatalf("window=%d, want %d", decision.WindowBytes, genericDownloadConservativeWindowBytes())
	}
	if decision.WindowBytes != genericDownloadOpenEndedWindowBytes() {
		t.Fatalf("window=%d, want open-ended bulk window %d", decision.WindowBytes, genericDownloadOpenEndedWindowBytes())
	}
	if decision.StabilityTier != "balanced" {
		t.Fatalf("stability_tier=%s, want balanced", decision.StabilityTier)
	}
	if decision.ConcurrencyReason != "generic_stream_first_bulk_window" {
		t.Fatalf("concurrency reason=%s, want generic_stream_first_bulk_window", decision.ConcurrencyReason)
	}
}

func TestAdaptiveSnapshotUsesBulkWindowForWholeObjectGenericDownload(t *testing.T) {
	resetAdaptiveStateForTest()
	tuning := config.DefaultTransportTuningConfig()
	ApplyTransportTuning(tuning)
	target, err := url.Parse("http://example.com/archive.zip")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	req, _ := http.NewRequest(http.MethodGet, target.String(), nil)
	prepared := &mappingForwardRequest{Method: http.MethodGet, TargetURL: target, Headers: make(http.Header)}
	resp := &http.Response{StatusCode: http.StatusOK, ContentLength: 128 << 20, Header: http.Header{"Content-Type": []string{"application/octet-stream"}, "Content-Disposition": []string{"attachment; filename=archive.zip"}, "Accept-Ranges": []string{"bytes"}}}
	decision := globalAdaptiveDelivery.snapshot(prepared, req, resp, fixedWindowPlan{concurrency: 2, window: genericDownloadWindowBytes()})
	if decision.PreferSegmentedPrimary {
		t.Fatal("expected stream-first recovery plan")
	}
	if decision.WindowBytes != genericDownloadOpenEndedWindowBytes() {
		t.Fatalf("window=%d, want full-object bulk window %d", decision.WindowBytes, genericDownloadOpenEndedWindowBytes())
	}
	if decision.SegmentConcurrency != 1 {
		t.Fatalf("segment concurrency=%d, want 1", decision.SegmentConcurrency)
	}
	if decision.ConcurrencyReason != "generic_stream_first_whole_object_window" {
		t.Fatalf("concurrency reason=%s", decision.ConcurrencyReason)
	}
}

func TestAdaptiveSnapshotLoopbackPlaybackDisablesPrefetchByDefault(t *testing.T) {
	resetAdaptiveStateForTest()
	target, err := url.Parse("http://127.0.0.1/video.mp4")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	req, _ := http.NewRequest(http.MethodGet, target.String(), nil)
	prepared := &mappingForwardRequest{Method: http.MethodGet, TargetURL: target, Headers: make(http.Header)}
	resp := &http.Response{StatusCode: http.StatusOK, ContentLength: 335102685, Header: http.Header{"Content-Type": []string{"video/mp4"}, "Accept-Ranges": []string{"bytes"}, "X-Siptunnel-Response-Mode": []string{"RTP"}}}
	decision := globalAdaptiveDelivery.snapshot(prepared, req, resp, fixedWindowPlan{concurrency: 4, window: 4 << 20})
	if decision.PrefetchSegments != 0 {
		t.Fatalf("prefetch=%d, want 0", decision.PrefetchSegments)
	}
	if decision.SegmentConcurrency != 2 {
		t.Fatalf("segment concurrency=%d, want 2", decision.SegmentConcurrency)
	}
}

func TestAdaptiveSnapshotDownshiftsOpenEndedGenericDownloadWhenSourceConstrained(t *testing.T) {
	resetAdaptiveStateForTest()
	tuning := config.DefaultTransportTuningConfig()
	tuning.GenericDownloadSourceConstrainedAutoSingleflightEnabled = true
	ApplyTransportTuning(tuning)
	target, err := url.Parse("http://example.com/archive.zip")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	req, _ := http.NewRequest(http.MethodGet, target.String(), nil)
	req.Header.Set("Range", "bytes=0-")
	prepared := &mappingForwardRequest{Method: http.MethodGet, TargetURL: target, Headers: make(http.Header)}
	resp := &http.Response{StatusCode: http.StatusPartialContent, ContentLength: 128 << 20, Header: http.Header{"Content-Type": []string{"application/octet-stream"}, "Content-Disposition": []string{"attachment; filename=archive.zip"}, "Content-Range": []string{"bytes 0-4194303/134217728"}}}
	globalGenericDownloadController.observeSourceRead(target.String(), "transfer-a", 3200000, 8388608, 2)
	globalGenericDownloadController.observeSourceRead(target.String(), "transfer-a", 3190000, 8388608, 2)
	decision := globalAdaptiveDelivery.snapshot(prepared, req, resp, fixedWindowPlan{concurrency: 2, window: genericDownloadWindowBytes()})
	if decision.SegmentConcurrency != 1 {
		t.Fatalf("segment concurrency=%d, want 1", decision.SegmentConcurrency)
	}
	if decision.ConcurrencyReason != "generic_stream_first_source_observed" {
		t.Fatalf("concurrency reason=%s", decision.ConcurrencyReason)
	}
}

func TestAdaptiveSnapshotKeepsTwoSegmentsForLoopbackBackedGenericDownload(t *testing.T) {
	resetAdaptiveStateForTest()
	target, err := url.Parse("http://127.0.0.1/archive.zip")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	req, _ := http.NewRequest(http.MethodGet, target.String(), nil)
	req.Header.Set("Range", "bytes=0-")
	prepared := &mappingForwardRequest{Method: http.MethodGet, TargetURL: target, Headers: make(http.Header)}
	resp := &http.Response{StatusCode: http.StatusPartialContent, ContentLength: 128 << 20, Header: http.Header{"Content-Type": []string{"application/octet-stream"}, "Content-Disposition": []string{"attachment; filename=archive.zip"}, "Content-Range": []string{"bytes 0-4194303/134217728"}}}
	decision := globalAdaptiveDelivery.snapshot(prepared, req, resp, fixedWindowPlan{concurrency: 4, window: genericDownloadWindowBytes()})
	if decision.SegmentConcurrency != 1 {
		t.Fatalf("segment concurrency=%d, want 1", decision.SegmentConcurrency)
	}
	if decision.ConcurrencyReason != "generic_stream_first_bulk_window" {
		t.Fatalf("concurrency reason=%s", decision.ConcurrencyReason)
	}
}
