package server

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"siptunnel/internal/service"
)

type requestProtector interface {
	Acquire(mappingID, sourceIP string) (func(), error)
}

const (
	autoRestrictionRateLimitThreshold  = 5
	autoRestrictionConcurrentThreshold = 3
	autoRestrictionRateLimitMinutes    = 10
	autoRestrictionConcurrentMinutes   = 5
)

type protectionRejectError struct {
	Scope         string
	Kind          string
	Target        string
	RPS           int
	Burst         int
	MaxConcurrent int
}

func (e *protectionRejectError) Error() string {
	if e == nil {
		return "protection reject"
	}
	switch e.Kind {
	case "rate_limit":
		return fmt.Sprintf("%s rate limit exceeded: rps=%d burst=%d target=%s", e.Scope, e.RPS, e.Burst, e.Target)
	case "max_concurrent":
		return fmt.Sprintf("%s max concurrency exceeded: max_concurrent=%d target=%s", e.Scope, e.MaxConcurrent, e.Target)
	case "temporary_block":
		return fmt.Sprintf("%s temporary block active: target=%s", e.Scope, e.Target)
	default:
		return fmt.Sprintf("%s protection reject: target=%s", e.Scope, e.Target)
	}
}

func classifyProtectionReject(err error) *protectionRejectError {
	if err == nil {
		return nil
	}
	var pre *protectionRejectError
	if errors.As(err, &pre) {
		return pre
	}
	return nil
}

func protectionRejectStatus(err error) int {
	pre := classifyProtectionReject(err)
	if pre != nil && pre.Kind == "rate_limit" {
		return http.StatusTooManyRequests
	}
	return http.StatusServiceUnavailable
}

type protectionTargetStat struct {
	Target string `json:"target"`
	Count  uint64 `json:"count"`
}

type protectionScopeSnapshot struct {
	Scope                string                 `json:"scope"`
	Label                string                 `json:"label"`
	RPS                  int                    `json:"rps"`
	Burst                int                    `json:"burst"`
	MaxConcurrent        int                    `json:"max_concurrent"`
	ActiveRequests       int                    `json:"active_requests"`
	RateLimitHitsTotal   uint64                 `json:"rate_limit_hits_total"`
	ConcurrentRejects    uint64                 `json:"concurrent_rejects_total"`
	AllowedTotal         uint64                 `json:"allowed_total"`
	TopRateLimitTargets  []protectionTargetStat `json:"top_rate_limit_targets,omitempty"`
	TopConcurrentTargets []protectionTargetStat `json:"top_concurrent_targets,omitempty"`
	TopAllowedTargets    []protectionTargetStat `json:"top_allowed_targets,omitempty"`
}

type protectionRuntimeSnapshot struct {
	RPS                  int                             `json:"rps"`
	Burst                int                             `json:"burst"`
	MaxConcurrent        int                             `json:"max_concurrent"`
	ActiveRequests       int                             `json:"active_requests"`
	RateLimitHitsTotal   uint64                          `json:"rate_limit_hits_total"`
	ConcurrentRejects    uint64                          `json:"concurrent_rejects_total"`
	AllowedTotal         uint64                          `json:"allowed_total"`
	LastTriggeredTime    string                          `json:"last_triggered_time,omitempty"`
	LastTriggeredType    string                          `json:"last_triggered_type,omitempty"`
	LastTriggeredTarget  string                          `json:"last_triggered_target,omitempty"`
	TopRateLimitTargets  []protectionTargetStat          `json:"top_rate_limit_targets,omitempty"`
	TopConcurrentTargets []protectionTargetStat          `json:"top_concurrent_targets,omitempty"`
	TopAllowedTargets    []protectionTargetStat          `json:"top_allowed_targets,omitempty"`
	Scopes               []protectionScopeSnapshot       `json:"scopes,omitempty"`
	Restrictions         []protectionRestrictionSnapshot `json:"restrictions,omitempty"`
}

type protectionBucket struct {
	label                     string
	limits                    OpsLimits
	limiter                   *service.RateLimiter
	active                    int
	allowedTotal              uint64
	rateLimitHitsTotal        uint64
	concurrentRejects         uint64
	rateLimitHitsByTarget     map[string]uint64
	concurrentRejectsByTarget map[string]uint64
	allowedByTarget           map[string]uint64
}

type protectionRuntime struct {
	mu sync.Mutex

	limits  OpsLimits
	global  protectionBucket
	mapping protectionBucket
	source  protectionBucket

	lastTriggeredAt     time.Time
	lastTriggeredType   string
	lastTriggeredTarget string
	restrictions        map[string]protectionRestrictionSnapshot
}

func newProtectionRuntime(limits OpsLimits) *protectionRuntime {
	p := &protectionRuntime{}
	p.UpdateLimits(limits)
	return p
}

func (p *protectionRuntime) UpdateLimits(limits OpsLimits) {
	if p == nil {
		return
	}
	limits = normalizeOpsLimits(limits)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.limits = limits
	p.global = ensureProtectionBucket(p.global, "global", deriveScopeLimits("global", limits))
	p.mapping = ensureProtectionBucket(p.mapping, "mapping", deriveScopeLimits("mapping", limits))
	p.source = ensureProtectionBucket(p.source, "source", deriveScopeLimits("source", limits))
}

func ensureProtectionBucket(bucket protectionBucket, label string, limits OpsLimits) protectionBucket {
	bucket.label = label
	bucket.limits = limits
	bucket.limiter = service.NewRateLimiter(maxIntVal(1, limits.RPS), maxIntVal(1, limits.Burst))
	if bucket.rateLimitHitsByTarget == nil {
		bucket.rateLimitHitsByTarget = map[string]uint64{}
	}
	if bucket.concurrentRejectsByTarget == nil {
		bucket.concurrentRejectsByTarget = map[string]uint64{}
	}
	if bucket.allowedByTarget == nil {
		bucket.allowedByTarget = map[string]uint64{}
	}
	if bucket.active > limits.MaxConcurrent && limits.MaxConcurrent > 0 {
		bucket.active = limits.MaxConcurrent
	}
	return bucket
}

func restrictionKey(scope, target string) string {
	scope = strings.TrimSpace(scope)
	target = strings.TrimSpace(target)
	if scope == "" || target == "" {
		return ""
	}
	return scope + "|" + target
}

func (p *protectionRuntime) UpsertRestriction(scope, target string, minutes int, reason string) (protectionRestrictionSnapshot, error) {
	if p == nil {
		return protectionRestrictionSnapshot{}, fmt.Errorf("protection runtime not configured")
	}
	scope = strings.TrimSpace(scope)
	target = strings.TrimSpace(target)
	if scope != "source" && scope != "mapping" {
		return protectionRestrictionSnapshot{}, fmt.Errorf("scope must be source or mapping")
	}
	if target == "" {
		return protectionRestrictionSnapshot{}, fmt.Errorf("target is required")
	}
	if minutes <= 0 {
		return protectionRestrictionSnapshot{}, fmt.Errorf("minutes must be > 0")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.upsertRestrictionLocked(scope, target, minutes, reason, false, "manual"), nil
}

func (p *protectionRuntime) upsertRestrictionLocked(scope, target string, minutes int, reason string, auto bool, trigger string) protectionRestrictionSnapshot {
	now := time.Now().UTC()
	entry := protectionRestrictionSnapshot{
		Scope:       scope,
		Target:      target,
		Reason:      strings.TrimSpace(reason),
		CreatedAt:   formatTimestamp(now),
		ExpiresAt:   formatTimestamp(now.Add(time.Duration(minutes) * time.Minute)),
		Minutes:     minutes,
		Active:      true,
		Auto:        auto,
		Trigger:     strings.TrimSpace(trigger),
		AutoRelease: auto,
	}
	if p.restrictions == nil {
		p.restrictions = map[string]protectionRestrictionSnapshot{}
	}
	p.restrictions[restrictionKey(scope, target)] = entry
	if auto {
		p.markTriggeredLocked(scope+"_temporary_block_auto", target)
	} else {
		p.markTriggeredLocked(scope+"_temporary_block", target)
	}
	return entry
}

func (p *protectionRuntime) maybeAutoRestrictLocked(scope, target, trigger string, count uint64) {
	if p == nil || target == "" {
		return
	}
	if scope != "source" && scope != "mapping" {
		return
	}
	key := restrictionKey(scope, target)
	if key == "" {
		return
	}
	if existing := p.restrictionHitLocked(scope, target); existing != nil && existing.Active {
		return
	}
	minutes := 0
	reason := ""
	switch trigger {
	case "rate_limit":
		if count < autoRestrictionRateLimitThreshold {
			return
		}
		minutes = autoRestrictionRateLimitMinutes
		reason = fmt.Sprintf("自动限制：连续触发限流 %d 次", count)
	case "max_concurrent":
		if count < autoRestrictionConcurrentThreshold {
			return
		}
		minutes = autoRestrictionConcurrentMinutes
		reason = fmt.Sprintf("自动限制：连续触发并发保护 %d 次", count)
	default:
		return
	}
	p.upsertRestrictionLocked(scope, target, minutes, reason, true, scope+"_"+trigger)
}

func (p *protectionRuntime) attributeGlobalRejectLocked(mappingTarget, sourceTarget, trigger string) {
	if p == nil {
		return
	}
	record := func(bucket *protectionBucket, scope, target string) {
		if bucket == nil || strings.TrimSpace(target) == "" {
			return
		}
		switch trigger {
		case "rate_limit":
			bucket.rateLimitHitsTotal++
			bucket.rateLimitHitsByTarget[target]++
			p.markTriggeredLocked(scope+"_rate_limit", target)
			p.maybeAutoRestrictLocked(scope, target, trigger, bucket.rateLimitHitsByTarget[target])
		case "max_concurrent":
			bucket.concurrentRejects++
			bucket.concurrentRejectsByTarget[target]++
			p.markTriggeredLocked(scope+"_max_concurrent", target)
			p.maybeAutoRestrictLocked(scope, target, trigger, bucket.concurrentRejectsByTarget[target])
		}
	}
	record(&p.mapping, "mapping", mappingTarget)
	record(&p.source, "source", sourceTarget)
}

func (p *protectionRuntime) RemoveRestriction(scope, target string) bool {
	if p == nil {
		return false
	}
	key := restrictionKey(scope, target)
	if key == "" {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.restrictions == nil {
		return false
	}
	_, ok := p.restrictions[key]
	if ok {
		delete(p.restrictions, key)
	}
	return ok
}

func (p *protectionRuntime) restrictionHitLocked(scope, target string) *protectionRestrictionSnapshot {
	if p.restrictions == nil {
		return nil
	}
	key := restrictionKey(scope, target)
	entry, ok := p.restrictions[key]
	if !ok {
		return nil
	}
	now := time.Now().UTC()
	expiresAt, _ := time.Parse(time.RFC3339, entry.ExpiresAt)
	if !expiresAt.IsZero() && !expiresAt.After(now) {
		delete(p.restrictions, key)
		return nil
	}
	entry.Active = true
	entry.Minutes = int(expiresAt.Sub(now).Round(time.Minute) / time.Minute)
	if entry.Minutes < 1 {
		entry.Minutes = 1
	}
	p.restrictions[key] = entry
	return &entry
}

func (p *protectionRuntime) restrictionSnapshotsLocked() []protectionRestrictionSnapshot {
	if len(p.restrictions) == 0 {
		return nil
	}
	now := time.Now().UTC()
	out := make([]protectionRestrictionSnapshot, 0, len(p.restrictions))
	for key, item := range p.restrictions {
		expiresAt, _ := time.Parse(time.RFC3339, item.ExpiresAt)
		if !expiresAt.IsZero() && !expiresAt.After(now) {
			delete(p.restrictions, key)
			continue
		}
		minutes := int(expiresAt.Sub(now).Round(time.Minute) / time.Minute)
		if minutes < 1 {
			minutes = 1
		}
		item.Active = true
		item.Minutes = minutes
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Scope == out[j].Scope {
			return out[i].Target < out[j].Target
		}
		return out[i].Scope < out[j].Scope
	})
	return out
}

func deriveScopeLimits(scope string, global OpsLimits) OpsLimits {
	switch scope {
	case "mapping":
		return OpsLimits{
			RPS:           maxIntVal(50, (global.RPS*3)/4),
			Burst:         maxIntVal(100, (global.Burst*3)/4),
			MaxConcurrent: maxIntVal(32, (global.MaxConcurrent*3)/4),
		}
	case "source":
		return OpsLimits{
			RPS:           maxIntVal(50, (global.RPS*2)/3),
			Burst:         maxIntVal(100, (global.Burst*2)/3),
			MaxConcurrent: maxIntVal(32, (global.MaxConcurrent*2)/3),
		}
	default:
		return global
	}
}

func (p *protectionRuntime) Acquire(mappingID, sourceIP string) (func(), error) {
	if p == nil {
		return func() {}, nil
	}
	mappingTarget := strings.TrimSpace(mappingID)
	if mappingTarget == "" {
		mappingTarget = "gateway"
	}
	sourceTarget := strings.TrimSpace(sourceIP)
	if sourceTarget == "" {
		sourceTarget = "unknown"
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry := p.restrictionHitLocked("mapping", mappingTarget); entry != nil {
		p.markTriggeredLocked("mapping_temporary_block", mappingTarget)
		return nil, &protectionRejectError{Scope: "mapping", Kind: "temporary_block", Target: entry.Target}
	}
	if entry := p.restrictionHitLocked("source", sourceTarget); entry != nil {
		p.markTriggeredLocked("source_temporary_block", sourceTarget)
		return nil, &protectionRejectError{Scope: "source", Kind: "temporary_block", Target: entry.Target}
	}
	if err := p.acquireBucketLocked(&p.global, "gateway", "global"); err != nil {
		if pre := classifyProtectionReject(err); pre != nil {
			p.attributeGlobalRejectLocked(mappingTarget, sourceTarget, pre.Kind)
		}
		return nil, err
	}
	if err := p.acquireBucketLocked(&p.mapping, mappingTarget, "mapping"); err != nil {
		return nil, err
	}
	if err := p.acquireBucketLocked(&p.source, sourceTarget, "source"); err != nil {
		return nil, err
	}
	for _, bucket := range []*protectionBucket{&p.global, &p.mapping, &p.source} {
		bucket.active++
		bucket.allowedTotal++
	}
	p.global.allowedByTarget["gateway"]++
	p.mapping.allowedByTarget[mappingTarget]++
	p.source.allowedByTarget[sourceTarget]++
	released := false
	return func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if released {
			return
		}
		released = true
		for _, bucket := range []*protectionBucket{&p.global, &p.mapping, &p.source} {
			if bucket.active > 0 {
				bucket.active--
			}
		}
	}, nil
}

func (p *protectionRuntime) acquireBucketLocked(bucket *protectionBucket, target, scope string) error {
	if bucket == nil {
		return nil
	}
	if bucket.limiter == nil {
		bucket.limiter = service.NewRateLimiter(maxIntVal(1, bucket.limits.RPS), maxIntVal(1, bucket.limits.Burst))
	}
	if !bucket.limiter.Allow() {
		bucket.rateLimitHitsTotal++
		bucket.rateLimitHitsByTarget[target]++
		p.markTriggeredLocked(scope+"_rate_limit", target)
		p.maybeAutoRestrictLocked(scope, target, "rate_limit", bucket.rateLimitHitsByTarget[target])
		return &protectionRejectError{Scope: scope, Kind: "rate_limit", Target: target, RPS: bucket.limits.RPS, Burst: bucket.limits.Burst}
	}
	if bucket.limits.MaxConcurrent > 0 && bucket.active >= bucket.limits.MaxConcurrent {
		bucket.concurrentRejects++
		bucket.concurrentRejectsByTarget[target]++
		p.markTriggeredLocked(scope+"_max_concurrent", target)
		p.maybeAutoRestrictLocked(scope, target, "max_concurrent", bucket.concurrentRejectsByTarget[target])
		return &protectionRejectError{Scope: scope, Kind: "max_concurrent", Target: target, MaxConcurrent: bucket.limits.MaxConcurrent}
	}
	return nil
}

func (p *protectionRuntime) Snapshot() protectionRuntimeSnapshot {
	if p == nil {
		return protectionRuntimeSnapshot{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return protectionRuntimeSnapshot{
		RPS:                  p.global.limits.RPS,
		Burst:                p.global.limits.Burst,
		MaxConcurrent:        p.global.limits.MaxConcurrent,
		ActiveRequests:       p.global.active,
		RateLimitHitsTotal:   p.global.rateLimitHitsTotal + p.mapping.rateLimitHitsTotal + p.source.rateLimitHitsTotal,
		ConcurrentRejects:    p.global.concurrentRejects + p.mapping.concurrentRejects + p.source.concurrentRejects,
		AllowedTotal:         p.global.allowedTotal,
		LastTriggeredTime:    formatTimestamp(p.lastTriggeredAt),
		LastTriggeredType:    p.lastTriggeredType,
		LastTriggeredTarget:  p.lastTriggeredTarget,
		TopRateLimitTargets:  mergeTopProtectionTargets(p.mapping.rateLimitHitsByTarget, p.source.rateLimitHitsByTarget),
		TopConcurrentTargets: mergeTopProtectionTargets(p.mapping.concurrentRejectsByTarget, p.source.concurrentRejectsByTarget),
		TopAllowedTargets:    mergeTopProtectionTargets(p.mapping.allowedByTarget, p.source.allowedByTarget),
		Scopes:               []protectionScopeSnapshot{snapshotBucket("global", "全局入口", p.global), snapshotBucket("mapping", "按映射", p.mapping), snapshotBucket("source", "按来源 IP", p.source)},
		Restrictions:         p.restrictionSnapshotsLocked(),
	}
}

func snapshotBucket(scope, label string, bucket protectionBucket) protectionScopeSnapshot {
	return protectionScopeSnapshot{Scope: scope, Label: label, RPS: bucket.limits.RPS, Burst: bucket.limits.Burst, MaxConcurrent: bucket.limits.MaxConcurrent, ActiveRequests: bucket.active, RateLimitHitsTotal: bucket.rateLimitHitsTotal, ConcurrentRejects: bucket.concurrentRejects, AllowedTotal: bucket.allowedTotal, TopRateLimitTargets: topProtectionTargets(bucket.rateLimitHitsByTarget), TopConcurrentTargets: topProtectionTargets(bucket.concurrentRejectsByTarget), TopAllowedTargets: topProtectionTargets(bucket.allowedByTarget)}
}

func mergeTopProtectionTargets(values ...map[string]uint64) []protectionTargetStat {
	merged := map[string]uint64{}
	for _, item := range values {
		for k, v := range item {
			merged[k] += v
		}
	}
	return topProtectionTargets(merged)
}

func topProtectionTargets(values map[string]uint64) []protectionTargetStat {
	if len(values) == 0 {
		return nil
	}
	out := make([]protectionTargetStat, 0, len(values))
	for target, count := range values {
		if strings.TrimSpace(target) == "" || count == 0 {
			continue
		}
		out = append(out, protectionTargetStat{Target: target, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Target < out[j].Target
		}
		return out[i].Count > out[j].Count
	})
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

func (p *protectionRuntime) markTriggeredLocked(kind, target string) {
	p.lastTriggeredAt = time.Now().UTC()
	p.lastTriggeredType = strings.TrimSpace(kind)
	p.lastTriggeredTarget = strings.TrimSpace(target)
}
