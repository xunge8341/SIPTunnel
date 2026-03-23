package server

import (
	"context"
	"testing"
	"time"
)

func TestOpsObservabilityServiceRecentAccessAnalysisRefreshesOnVersionChange(t *testing.T) {
	store := newAccessLogStore(7, 100, nil)
	now := time.Now().UTC()
	store.Add(AccessLogEntry{ID: "1", OccurredAt: formatTimestamp(now.Add(-10 * time.Minute)), MappingName: "map-a", SourceIP: "10.0.0.1", Method: "GET", StatusCode: 200, DurationMS: 50})
	service := newOpsObservabilityService(store)

	first := service.RecentAccessAnalysis(context.Background())
	if first.Total != 1 || first.Failed != 0 {
		t.Fatalf("unexpected first analysis: %+v", first)
	}

	store.Add(AccessLogEntry{ID: "2", OccurredAt: formatTimestamp(now.Add(-5 * time.Minute)), MappingName: "map-a", SourceIP: "10.0.0.2", Method: "GET", StatusCode: 502, FailureReason: "bad gateway", DurationMS: 600})
	second := service.RecentAccessAnalysis(context.Background())
	if second.Total != 2 {
		t.Fatalf("expected refreshed total=2, got %+v", second)
	}
	if second.Failed != 1 || second.Slow != 1 {
		t.Fatalf("expected failed/slow counts to refresh, got %+v", second)
	}
}

func TestOpsObservabilityServiceTrendSeriesUsesResolvedBuckets(t *testing.T) {
	store := newAccessLogStore(7, 100, nil)
	now := time.Now().UTC()
	store.Add(AccessLogEntry{ID: "1", OccurredAt: formatTimestamp(now.Add(-20 * time.Minute)), MappingName: "map-a", SourceIP: "10.0.0.1", Method: "GET", StatusCode: 200, DurationMS: 50})
	store.Add(AccessLogEntry{ID: "2", OccurredAt: formatTimestamp(now.Add(-10 * time.Minute)), MappingName: "map-a", SourceIP: "10.0.0.1", Method: "GET", StatusCode: 500, FailureReason: "boom", DurationMS: 800})
	service := newOpsObservabilityService(store)

	series := service.DashboardTrendSeries(context.Background(), "1h", "15m")
	if series.Range != "1h" || series.Granularity != "15m" {
		t.Fatalf("unexpected series metadata: %+v", series)
	}
	if len(series.Points) == 0 {
		t.Fatal("expected trend points")
	}
	seenTotal := 0
	seenFailed := 0
	for _, point := range series.Points {
		seenTotal += point.Total
		seenFailed += point.Failed
	}
	if seenTotal != 2 || seenFailed != 1 {
		t.Fatalf("unexpected trend aggregation total=%d failed=%d", seenTotal, seenFailed)
	}
}
