package server

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const opsObservabilityCacheTTL = 5 * time.Second

type opsAnalysisCacheEntry struct {
	version   uint64
	expiresAt time.Time
	analysis  accessAnalysis
}

type opsTrendCacheEntry struct {
	version   uint64
	expiresAt time.Time
	series    dashboardTrendSeries
}

type opsObservabilityService struct {
	store *accessLogStore

	mu           sync.RWMutex
	recentWindow opsAnalysisCacheEntry
	trendWindows map[string]opsTrendCacheEntry
}

func newOpsObservabilityService(store *accessLogStore) *opsObservabilityService {
	if store == nil {
		return nil
	}
	return &opsObservabilityService{
		store:        store,
		trendWindows: make(map[string]opsTrendCacheEntry),
	}
}

func (s *opsObservabilityService) RecentAccessAnalysis(ctx context.Context) accessAnalysis {
	_ = ctx
	if s == nil || s.store == nil {
		return accessAnalysis{}
	}
	now := time.Now().UTC()
	version := s.store.Version()

	s.mu.RLock()
	cached := s.recentWindow
	s.mu.RUnlock()
	if cached.version == version && now.Before(cached.expiresAt) {
		return cached.analysis
	}

	start, end, _ := recentAnalysisWindow()
	analysis := analyzeAccessLogs(s.store.List(accessLogFilter{StartTime: start, EndTime: end}))
	entry := opsAnalysisCacheEntry{version: version, expiresAt: now.Add(opsObservabilityCacheTTL), analysis: analysis}

	s.mu.Lock()
	s.recentWindow = entry
	s.mu.Unlock()
	return analysis
}

func (s *opsObservabilityService) DashboardTrendSeries(ctx context.Context, rangeKey, grainKey string) dashboardTrendSeries {
	_ = ctx
	if s == nil || s.store == nil {
		start, now, grain, resolvedRange, resolvedGrain := resolveTrendWindow(rangeKey, grainKey)
		return aggregateTrendSeries(nil, start, now, grain, resolvedRange, resolvedGrain)
	}

	start, end, grain, resolvedRange, resolvedGrain := resolveTrendWindow(rangeKey, grainKey)
	now := time.Now().UTC()
	version := s.store.Version()
	cacheKey := fmt.Sprintf("%s|%s", resolvedRange, resolvedGrain)

	s.mu.RLock()
	cached, ok := s.trendWindows[cacheKey]
	s.mu.RUnlock()
	if ok && cached.version == version && now.Before(cached.expiresAt) {
		return cached.series
	}

	series := aggregateTrendSeries(s.store.List(accessLogFilter{StartTime: start, EndTime: end}), start, end, grain, resolvedRange, resolvedGrain)
	entry := opsTrendCacheEntry{version: version, expiresAt: now.Add(opsObservabilityCacheTTL), series: series}

	s.mu.Lock()
	s.trendWindows[cacheKey] = entry
	s.mu.Unlock()
	return series
}
