package server

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"siptunnel/internal/persistence"
)

type accessLogFilter struct {
	Status     int
	Mapping    string
	SourceIP   string
	Method     string
	SlowOnly   bool
	FailedOnly bool
	StartTime  time.Time
	EndTime    time.Time
}

type accessLogStore struct {
	mu                  sync.RWMutex
	items               []AccessLogEntry
	maxAgeDays          int
	maxRecords          int
	sqlite              *persistence.SQLiteStore
	persistCh           chan persistence.AccessLogRecord
	version             atomic.Uint64
	summaryCacheVersion uint64
	summaryCacheExpires time.Time
	summaryCachePayload map[string]any
	persistDrops        atomic.Uint64
	persistDropLogAt    atomic.Int64
	sampledDrops        atomic.Uint64
	sampledDropLogAt    atomic.Int64
}

func newAccessLogStore(maxAgeDays, maxRecords int, sqlite *persistence.SQLiteStore) *accessLogStore {
	store := &accessLogStore{items: make([]AccessLogEntry, 0, 256), maxAgeDays: maxAgeDays, maxRecords: maxRecords, sqlite: sqlite}
	if sqlite != nil {
		store.persistCh = make(chan persistence.AccessLogRecord, 4096)
		go store.persistLoop()
	}
	return store
}

func (s *accessLogStore) Configure(maxAgeDays, maxRecords int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if maxAgeDays > 0 {
		s.maxAgeDays = maxAgeDays
	}
	if maxRecords > 0 {
		s.maxRecords = maxRecords
	}
	if s.sqlite == nil {
		s.trimLocked()
	}
}

func (s *accessLogStore) persistLoop() {
	if s == nil || s.sqlite == nil || s.persistCh == nil {
		return
	}
	const (
		persistBatchSize     = 128
		persistFlushInterval = 100 * time.Millisecond
	)
	batch := make([]persistence.AccessLogRecord, 0, persistBatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := s.sqlite.RecordAccessLogBatch(context.Background(), batch); err != nil {
			log.Printf("access-log persistence batch dropped size=%d err=%v", len(batch), err)
		}
		batch = batch[:0]
	}
	timer := time.NewTimer(persistFlushInterval)
	defer timer.Stop()
	for {
		select {
		case record, ok := <-s.persistCh:
			if !ok {
				flush()
				return
			}
			batch = append(batch, record)
			if len(batch) >= persistBatchSize {
				flush()
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(persistFlushInterval)
			}
		case <-timer.C:
			flush()
			timer.Reset(persistFlushInterval)
		}
	}
}

func (s *accessLogStore) Add(entry AccessLogEntry) {
	if s == nil {
		return
	}
	s.version.Add(1)
	record := persistence.AccessLogRecord{
		ID:            entry.ID,
		OccurredAt:    parseTimestampOrNow(entry.OccurredAt),
		MappingName:   entry.MappingName,
		SourceIP:      entry.SourceIP,
		Method:        entry.Method,
		Path:          entry.Path,
		StatusCode:    entry.StatusCode,
		DurationMS:    entry.DurationMS,
		FailureReason: entry.FailureReason,
		RequestID:     entry.RequestID,
		TraceID:       entry.TraceID,
	}
	if s.sqlite != nil {
		if s.shouldSampleOut(record) {
			s.noteSampledDrop(record)
			return
		}
		select {
		case s.persistCh <- record:
		default:
			s.notePersistDrop(record)
		}
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append([]AccessLogEntry{entry}, s.items...)
	s.trimLocked()
}

func (s *accessLogStore) notePersistDrop(record persistence.AccessLogRecord) {
	if s == nil {
		return
	}
	dropped := s.persistDrops.Add(1)
	nowUnix := time.Now().UTC().Unix()
	last := s.persistDropLogAt.Load()
	if nowUnix == last {
		return
	}
	if !s.persistDropLogAt.CompareAndSwap(last, nowUnix) {
		return
	}
	log.Printf("access-log persistence queue full; dropped_total=%d last_id=%s mapping=%s path=%s", dropped, record.ID, record.MappingName, record.Path)
}

func (s *accessLogStore) shouldSampleOut(record persistence.AccessLogRecord) bool {
	if s == nil || s.persistCh == nil {
		return false
	}
	if record.StatusCode >= 400 || record.DurationMS >= 500 || strings.TrimSpace(record.FailureReason) != "" {
		return false
	}
	queueDepth := len(s.persistCh)
	queueCap := cap(s.persistCh)
	if queueCap <= 0 || queueDepth <= queueCap/2 {
		return false
	}
	rateMask := uint32(0x7)
	if queueDepth >= (queueCap*3)/4 {
		rateMask = 0xF
	}
	key := firstNonEmpty(strings.TrimSpace(record.RequestID), strings.TrimSpace(record.ID), strings.TrimSpace(record.TraceID), strings.TrimSpace(record.Path), strings.TrimSpace(record.MappingName))
	if key == "" {
		return false
	}
	return stableSampleHash32(key)&rateMask != 0
}

func (s *accessLogStore) noteSampledDrop(record persistence.AccessLogRecord) {
	if s == nil {
		return
	}
	dropped := s.sampledDrops.Add(1)
	nowUnix := time.Now().UTC().Unix()
	last := s.sampledDropLogAt.Load()
	if nowUnix == last {
		return
	}
	if !s.sampledDropLogAt.CompareAndSwap(last, nowUnix) {
		return
	}
	log.Printf("access-log success sampling engaged; sampled_total=%d queue_depth=%d queue_capacity=%d last_id=%s mapping=%s path=%s", dropped, len(s.persistCh), cap(s.persistCh), record.ID, record.MappingName, record.Path)
}

func stableSampleHash32(text string) uint32 {
	const offset32 = 2166136261
	const prime32 = 16777619
	h := uint32(offset32)
	for i := 0; i < len(text); i++ {
		h ^= uint32(text[i])
		h *= prime32
	}
	return h
}

func (s *accessLogStore) Version() uint64 {
	if s == nil {
		return 0
	}
	return s.version.Load()
}

func (s *accessLogStore) List(filter accessLogFilter) []AccessLogEntry {
	if s.sqlite != nil {
		records, err := s.sqlite.ListAccessLogs(context.Background(), persistence.AccessLogQuery{
			Status:     filter.Status,
			Mapping:    filter.Mapping,
			SourceIP:   filter.SourceIP,
			Method:     filter.Method,
			SlowOnly:   filter.SlowOnly,
			FailedOnly: filter.FailedOnly,
			StartTime:  filter.StartTime,
			EndTime:    filter.EndTime,
		})
		if err != nil {
			return nil
		}
		out := make([]AccessLogEntry, 0, len(records))
		for _, item := range records {
			out = append(out, AccessLogEntry{
				ID:            item.ID,
				OccurredAt:    formatTimestamp(item.OccurredAt),
				MappingName:   item.MappingName,
				SourceIP:      item.SourceIP,
				Method:        item.Method,
				Path:          item.Path,
				StatusCode:    item.StatusCode,
				DurationMS:    item.DurationMS,
				FailureReason: item.FailureReason,
				RequestID:     item.RequestID,
				TraceID:       item.TraceID,
			})
		}
		return out
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AccessLogEntry, 0, len(s.items))
	for _, item := range s.items {
		if filter.Status > 0 && item.StatusCode != filter.Status {
			continue
		}
		if filter.Mapping != "" && !strings.Contains(strings.ToLower(item.MappingName), strings.ToLower(filter.Mapping)) {
			continue
		}
		if filter.SourceIP != "" && !strings.Contains(strings.ToLower(item.SourceIP), strings.ToLower(filter.SourceIP)) {
			continue
		}
		if filter.Method != "" && !strings.EqualFold(item.Method, filter.Method) {
			continue
		}
		if filter.SlowOnly && item.DurationMS < 500 {
			continue
		}
		if filter.FailedOnly && item.StatusCode < 400 && strings.TrimSpace(item.FailureReason) == "" {
			continue
		}
		if !filter.StartTime.IsZero() {
			if ts, err := parseTimestamp(item.OccurredAt); err == nil && ts.Before(filter.StartTime) {
				continue
			}
		}
		if !filter.EndTime.IsZero() {
			if ts, err := parseTimestamp(item.OccurredAt); err == nil && ts.After(filter.EndTime) {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func (s *accessLogStore) Summary() map[string]any {
	if s == nil {
		return map[string]any{"total": 0, "failed": 0, "slow": 0, "error_types": map[string]int{}}
	}
	now := time.Now().UTC()
	version := s.Version()
	s.mu.RLock()
	if s.summaryCacheVersion == version && now.Before(s.summaryCacheExpires) && s.summaryCachePayload != nil {
		payload := cloneSummaryPayload(s.summaryCachePayload)
		s.mu.RUnlock()
		return payload
	}
	s.mu.RUnlock()

	items := s.List(accessLogFilter{})
	failed := 0
	slow := 0
	errs := map[string]int{}
	for _, item := range items {
		if item.DurationMS >= 500 {
			slow++
		}
		if item.StatusCode >= 400 || strings.TrimSpace(item.FailureReason) != "" {
			failed++
			reason := strings.TrimSpace(item.FailureReason)
			if reason == "" {
				reason = fmt.Sprintf("HTTP %d", item.StatusCode)
			}
			errs[reason]++
		}
	}
	payload := map[string]any{"total": len(items), "failed": failed, "slow": slow, "error_types": errs}
	s.mu.Lock()
	s.summaryCacheVersion = version
	s.summaryCacheExpires = now.Add(opsObservabilityCacheTTL)
	s.summaryCachePayload = cloneSummaryPayload(payload)
	s.mu.Unlock()
	return payload
}

func cloneSummaryPayload(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		if nested, ok := value.(map[string]int); ok {
			clone := make(map[string]int, len(nested))
			for nk, nv := range nested {
				clone[nk] = nv
			}
			out[key] = clone
			continue
		}
		out[key] = value
	}
	return out
}

func (s *accessLogStore) trimLocked() {
	if s.maxAgeDays > 0 {
		cutoff := time.Now().UTC().AddDate(0, 0, -s.maxAgeDays)
		filtered := s.items[:0]
		for _, item := range s.items {
			ts, err := parseTimestamp(item.OccurredAt)
			if err != nil || ts.After(cutoff) {
				filtered = append(filtered, item)
			}
		}
		s.items = filtered
	}
	if s.maxRecords > 0 && len(s.items) > s.maxRecords {
		s.items = s.items[:s.maxRecords]
	}
}

type accessAnalysis struct {
	Total           int
	Failed          int
	Slow            int
	ErrorTypes      map[string]int
	CountByMapping  map[string]int
	CountBySource   map[string]int
	FailedByMapping map[string]int
	FailedBySource  map[string]int
	LatestFailed    *AccessLogEntry
	AvgLatencyByMap map[string]int64
}

func analyzeAccessLogs(items []AccessLogEntry) accessAnalysis {
	result := accessAnalysis{
		ErrorTypes:      map[string]int{},
		CountByMapping:  map[string]int{},
		CountBySource:   map[string]int{},
		FailedByMapping: map[string]int{},
		FailedBySource:  map[string]int{},
		AvgLatencyByMap: map[string]int64{},
	}
	latencyTotals := map[string]int64{}
	for _, item := range items {
		result.Total++
		mapping := strings.TrimSpace(item.MappingName)
		if mapping == "" {
			mapping = "(未命名映射)"
		}
		source := strings.TrimSpace(item.SourceIP)
		if source == "" {
			source = "(unknown)"
		}
		result.CountByMapping[mapping]++
		result.CountBySource[source]++
		latencyTotals[mapping] += item.DurationMS
		if item.DurationMS >= 500 {
			result.Slow++
		}
		if item.StatusCode >= 400 || strings.TrimSpace(item.FailureReason) != "" {
			result.Failed++
			result.FailedByMapping[mapping]++
			result.FailedBySource[source]++
			reason := strings.TrimSpace(item.FailureReason)
			if reason == "" {
				reason = fmt.Sprintf("HTTP %d", item.StatusCode)
			}
			result.ErrorTypes[reason]++
			if result.LatestFailed == nil {
				copyItem := item
				result.LatestFailed = &copyItem
			}
		}
	}
	for mapping, totalLatency := range latencyTotals {
		count := result.CountByMapping[mapping]
		if count > 0 {
			result.AvgLatencyByMap[mapping] = totalLatency / int64(count)
		}
	}
	return result
}
