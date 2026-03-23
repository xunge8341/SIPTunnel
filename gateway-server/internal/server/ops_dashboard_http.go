package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (d *handlerDeps) handleAccessLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize <= 0 {
		pageSize = 50
	}
	statusCode, _ := strconv.Atoi(strings.TrimSpace(q.Get("status")))
	var startTime, endTime time.Time
	if v := strings.TrimSpace(q.Get("start_time")); v != "" {
		startTime, _ = time.Parse(time.RFC3339, v)
	}
	if v := strings.TrimSpace(q.Get("end_time")); v != "" {
		endTime, _ = time.Parse(time.RFC3339, v)
	}
	items := []AccessLogEntry{}
	if d.accessLogStore != nil {
		items = d.accessLogStore.List(accessLogFilter{
			Status:     statusCode,
			Mapping:    strings.TrimSpace(q.Get("mapping")),
			SourceIP:   strings.TrimSpace(q.Get("source_ip")),
			Method:     strings.ToUpper(strings.TrimSpace(q.Get("method"))),
			SlowOnly:   strings.EqualFold(strings.TrimSpace(q.Get("slow_only")), "true"),
			FailedOnly: strings.EqualFold(strings.TrimSpace(q.Get("failed_only")), "true"),
			StartTime:  startTime,
			EndTime:    endTime,
		})
	}
	total := len(items)
	startIdx := (page - 1) * pageSize
	if startIdx > total {
		startIdx = total
	}
	endIdx := startIdx + pageSize
	if endIdx > total {
		endIdx = total
	}
	analysis := analyzeAccessLogs(items)
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: map[string]any{"items": items[startIdx:endIdx], "pagination": pagination{Page: page, PageSize: pageSize, Total: total}, "summary": accessLogSummary{Total: analysis.Total, Failed: analysis.Failed, Slow: analysis.Slow, ErrorTypes: analysis.ErrorTypes, Window: "当前筛选条件"}}})
}

func (d *handlerDeps) aggregateAccessStats(ctx context.Context) accessAnalysis {
	if d != nil && d.opsView != nil {
		return d.opsView.RecentAccessAnalysis(ctx)
	}
	_ = ctx
	start, end, _ := recentAnalysisWindow()
	return d.buildAnalysis(accessLogFilter{StartTime: start, EndTime: end})
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func asFloatSetting(v any) float64 {
	switch value := v.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case json.Number:
		f, _ := value.Float64()
		return f
	default:
		return 0
	}
}

func asBool(v any) bool {
	if value, ok := v.(bool); ok {
		return value
	}
	return false
}

func (d *handlerDeps) handleDashboardOpsSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	analysis := d.aggregateAccessStats(r.Context())
	state := d.currentProtectionState()
	summary := dashboardOpsSummary{
		TopMappings:         topNWithLatency(analysis.CountByMapping, analysis.AvgLatencyByMap, 5),
		TopSourceIPs:        topN(analysis.CountBySource, 5),
		TopFailedMappings:   topNWithLatency(analysis.FailedByMapping, analysis.AvgLatencyByMap, 5),
		TopFailedSourceIPs:  topN(analysis.FailedBySource, 5),
		RateLimitStatus:     state.RateLimitStatus,
		CircuitBreakerState: state.CircuitBreakerStatus,
		ProtectionStatus:    state.ProtectionStatus,
	}
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: summary})
}

func (d *handlerDeps) handleDashboardSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	analysis := d.aggregateAccessStats(r.Context())
	status := d.networkStatusFunc(r.Context())
	state := d.currentProtectionState()
	summary := dashboardSummary{
		SystemHealth:        map[string]string{"healthy": "健康", "degraded": "降级"}[status.TunnelStatus()],
		ActiveConnections:   status.SIP.CurrentConnections,
		MappingTotal:        len(d.mappings.List()),
		MappingErrorCount:   len(analysis.FailedByMapping),
		RecentFailureCount:  analysis.Failed,
		RateLimitState:      state.RateLimitStatus,
		CircuitBreakerState: state.CircuitBreakerStatus,
	}
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: summary})
}

func (d *handlerDeps) handleDashboardTrends(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	rangeKey := strings.TrimSpace(r.URL.Query().Get("range"))
	grainKey := strings.TrimSpace(r.URL.Query().Get("granularity"))
	if d != nil && d.opsView != nil {
		series := d.opsView.DashboardTrendSeries(r.Context(), rangeKey, grainKey)
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: series})
		return
	}
	start, now, grain, resolvedRange, resolvedGrain := resolveTrendWindow(rangeKey, grainKey)
	items := []AccessLogEntry{}
	if d.accessLogStore != nil {
		items = d.accessLogStore.List(accessLogFilter{StartTime: start, EndTime: now})
	}
	series := aggregateTrendSeries(items, start, now, grain, resolvedRange, resolvedGrain)
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: series})
}

func resolveTrendWindow(rangeKey, grainKey string) (time.Time, time.Time, time.Duration, string, string) {
	now := time.Now().UTC()
	resolvedRange := strings.ToLower(strings.TrimSpace(rangeKey))
	resolvedGrain := strings.ToLower(strings.TrimSpace(grainKey))
	if resolvedRange == "" {
		resolvedRange = "24h"
	}
	start := now.Add(-24 * time.Hour)
	defaultGrain := time.Hour
	switch resolvedRange {
	case "1h":
		start = now.Add(-1 * time.Hour)
		defaultGrain = 5 * time.Minute
	case "6h":
		start = now.Add(-6 * time.Hour)
		defaultGrain = 15 * time.Minute
	case "7d":
		start = now.Add(-7 * 24 * time.Hour)
		defaultGrain = 24 * time.Hour
	default:
		resolvedRange = "24h"
	}
	grain := defaultGrain
	switch resolvedGrain {
	case "5m":
		grain = 5 * time.Minute
	case "15m":
		grain = 15 * time.Minute
	case "1h":
		grain = time.Hour
	case "1d":
		grain = 24 * time.Hour
	default:
		resolvedGrain = map[time.Duration]string{5 * time.Minute: "5m", 15 * time.Minute: "15m", time.Hour: "1h", 24 * time.Hour: "1d"}[defaultGrain]
	}
	if resolvedGrain == "" {
		resolvedGrain = map[time.Duration]string{5 * time.Minute: "5m", 15 * time.Minute: "15m", time.Hour: "1h", 24 * time.Hour: "1d"}[grain]
	}
	return bucketFloor(start, grain), now, grain, resolvedRange, resolvedGrain
}

func bucketFloor(ts time.Time, grain time.Duration) time.Time {
	ts = ts.UTC()
	if grain >= 24*time.Hour {
		return time.Date(ts.Year(), ts.Month(), ts.Day(), 0, 0, 0, 0, time.UTC)
	}
	minutes := int(grain / time.Minute)
	if minutes <= 0 {
		minutes = 1
	}
	minute := (ts.Minute() / minutes) * minutes
	return time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), minute, 0, 0, time.UTC)
}

func aggregateTrendSeries(items []AccessLogEntry, start, end time.Time, grain time.Duration, rangeKey, grainKey string) dashboardTrendSeries {
	type agg struct{ total, failed, slow int }
	buckets := map[time.Time]*agg{}
	for cursor := start; !cursor.After(end); cursor = cursor.Add(grain) {
		buckets[cursor] = &agg{}
	}
	for _, item := range items {
		ts, err := parseTimestamp(item.OccurredAt)
		if err != nil {
			continue
		}
		ts = ts.UTC()
		if ts.Before(start) || ts.After(end) {
			continue
		}
		bucket := bucketFloor(ts, grain)
		entry := buckets[bucket]
		if entry == nil {
			entry = &agg{}
			buckets[bucket] = entry
		}
		entry.total++
		if item.StatusCode >= 400 || strings.TrimSpace(item.FailureReason) != "" {
			entry.failed++
		}
		if item.DurationMS >= 500 {
			entry.slow++
		}
	}
	points := make([]dashboardTrendPoint, 0, len(buckets))
	for cursor := start; !cursor.After(end); cursor = cursor.Add(grain) {
		entry := buckets[cursor]
		label := cursor.Format("15:04")
		if grain >= 24*time.Hour {
			label = cursor.Format("01-02")
		} else if end.Sub(start) > 24*time.Hour {
			label = cursor.Format("01-02 15:04")
		}
		points = append(points, dashboardTrendPoint{Bucket: formatTimestamp(cursor), Label: label, Total: entry.total, Failed: entry.failed, Slow: entry.slow})
	}
	return dashboardTrendSeries{Range: rangeKey, Granularity: grainKey, Points: points}
}

func topN(m map[string]int, n int) []dashboardOpsSummaryItem {
	items := make([]dashboardOpsSummaryItem, 0, len(m))
	for k, v := range m {
		items = append(items, dashboardOpsSummaryItem{Name: k, Count: v})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Count > items[j].Count })
	if len(items) > n {
		items = items[:n]
	}
	return items
}

func recentAnalysisWindow() (time.Time, time.Time, string) {
	now := time.Now().UTC()
	return now.Add(-1 * time.Hour), now, "近 1 小时"
}

func (d *handlerDeps) buildAnalysis(filter accessLogFilter) accessAnalysis {
	items := []AccessLogEntry{}
	if d.accessLogStore != nil {
		items = d.accessLogStore.List(filter)
	}
	return analyzeAccessLogs(items)
}

func topNWithLatency(counts map[string]int, latency map[string]int64, n int) []dashboardOpsSummaryItem {
	items := topN(counts, n)
	for i := range items {
		items[i].AvgLatencyMS = latency[items[i].Name]
	}
	return items
}
