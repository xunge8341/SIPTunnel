package server

import (
	"context"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"siptunnel/internal/repository"
	"siptunnel/internal/selfcheck"
)

const prometheusSnapshotCacheTTL = 2 * time.Second

type prometheusMetric struct {
	Name   string
	Type   string
	Help   string
	Labels map[string]string
	Value  float64
}

func (d *handlerDeps) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(d.prometheusSnapshot(r.Context())))
}

func (d *handlerDeps) prometheusSnapshot(ctx context.Context) string {
	if d == nil {
		return ""
	}
	now := time.Now().UTC()
	d.metricsCacheMu.RLock()
	if now.Before(d.metricsCacheUntil) && d.metricsCacheBody != "" {
		payload := d.metricsCacheBody
		d.metricsCacheMu.RUnlock()
		return payload
	}
	d.metricsCacheMu.RUnlock()

	payload := d.buildPrometheusSnapshot(ctx, now)

	d.metricsCacheMu.Lock()
	d.metricsCacheUntil = now.Add(prometheusSnapshotCacheTTL)
	d.metricsCacheBody = payload
	d.metricsCacheMu.Unlock()
	return payload
}

func (d *handlerDeps) buildPrometheusSnapshot(ctx context.Context, now time.Time) string {
	metrics := make([]prometheusMetric, 0, 48)
	metrics = append(metrics,
		prometheusMetric{Name: "go_goroutines", Type: "gauge", Help: "Number of goroutines.", Value: float64(runtime.NumGoroutine())},
		prometheusMetric{Name: "siptunnel_build_info", Type: "gauge", Help: "Static build metadata for the gateway process.", Labels: map[string]string{"version": "dev"}, Value: 1},
	)

	ready, _ := d.readinessReport(ctx)
	metrics = append(metrics, prometheusMetric{Name: "siptunnel_ready_state", Type: "gauge", Help: "Gateway readiness state, 1 means ready.", Value: boolFloat(ready)})

	if d.networkStatusFunc != nil {
		status := d.networkStatusFunc(ctx)
		metrics = append(metrics,
			prometheusMetric{Name: "siptunnel_sip_tcp_current_connections", Type: "gauge", Help: "Current SIP TCP connections.", Value: float64(status.SIP.CurrentConnections)},
			prometheusMetric{Name: "siptunnel_sip_tcp_connection_errors_total", Type: "counter", Help: "Total SIP TCP connection errors.", Value: float64(status.SIP.ConnectionErrorTotal)},
			prometheusMetric{Name: "siptunnel_sip_tcp_read_timeouts_total", Type: "counter", Help: "Total SIP TCP read timeouts.", Value: float64(status.SIP.ReadTimeoutTotal)},
			prometheusMetric{Name: "siptunnel_sip_tcp_write_timeouts_total", Type: "counter", Help: "Total SIP TCP write timeouts.", Value: float64(status.SIP.WriteTimeoutTotal)},
			prometheusMetric{Name: "siptunnel_rtp_port_alloc_fail_total", Type: "counter", Help: "Total RTP port allocation failures.", Value: float64(status.RTP.PortAllocFailTotal)},
			prometheusMetric{Name: "siptunnel_rtp_port_pool_total", Type: "gauge", Help: "Total RTP ports in pool.", Value: float64(status.RTP.PortPoolTotal)},
			prometheusMetric{Name: "siptunnel_rtp_port_pool_used", Type: "gauge", Help: "Used RTP ports in pool.", Value: float64(status.RTP.PortPoolUsed)},
			prometheusMetric{Name: "siptunnel_rtp_active_transfers", Type: "gauge", Help: "Current active RTP transfers.", Value: float64(status.RTP.ActiveTransfers)},
			prometheusMetric{Name: "siptunnel_rtp_tcp_read_errors_total", Type: "counter", Help: "Total RTP TCP read errors.", Value: float64(status.RTP.TCPReadErrorsTotal)},
			prometheusMetric{Name: "siptunnel_rtp_tcp_write_errors_total", Type: "counter", Help: "Total RTP TCP write errors.", Value: float64(status.RTP.TCPWriteErrorsTotal)},
		)
	}

	if d.accessLogStore != nil {
		summary := d.accessLogStore.Summary()
		metrics = append(metrics,
			prometheusMetric{Name: "siptunnel_requests_total", Type: "counter", Help: "Total HTTP mapping requests recorded by the gateway.", Value: asFloat(summary["total"])},
			prometheusMetric{Name: "siptunnel_requests_failed_total", Type: "counter", Help: "Total failed HTTP mapping requests.", Value: asFloat(summary["failed"])},
			prometheusMetric{Name: "siptunnel_requests_slow_total", Type: "counter", Help: "Total slow HTTP mapping requests (>=500ms).", Value: asFloat(summary["slow"])},
		)
	}

	if d.protectionRuntime != nil {
		protection := d.protectionRuntime.Snapshot()
		metrics = append(metrics,
			prometheusMetric{Name: "siptunnel_active_requests", Type: "gauge", Help: "Current active gateway requests under protection runtime.", Value: float64(protection.ActiveRequests)},
			prometheusMetric{Name: "siptunnel_rate_limit_hits_total", Type: "counter", Help: "Total rate limit hits.", Value: float64(protection.RateLimitHitsTotal)},
			prometheusMetric{Name: "siptunnel_concurrency_rejects_total", Type: "counter", Help: "Total request rejects caused by concurrency protection.", Value: float64(protection.ConcurrentRejects)},
			prometheusMetric{Name: "siptunnel_allowed_requests_total", Type: "counter", Help: "Total requests allowed by protection runtime.", Value: float64(protection.AllowedTotal)},
		)
		for _, scope := range protection.Scopes {
			metrics = append(metrics,
				prometheusMetric{Name: "siptunnel_protection_scope_rps", Type: "gauge", Help: "Configured RPS by protection scope.", Labels: map[string]string{"scope": scope.Scope}, Value: float64(scope.RPS)},
				prometheusMetric{Name: "siptunnel_protection_scope_burst", Type: "gauge", Help: "Configured burst by protection scope.", Labels: map[string]string{"scope": scope.Scope}, Value: float64(scope.Burst)},
				prometheusMetric{Name: "siptunnel_protection_scope_max_concurrent", Type: "gauge", Help: "Configured max concurrency by protection scope.", Labels: map[string]string{"scope": scope.Scope}, Value: float64(scope.MaxConcurrent)},
				prometheusMetric{Name: "siptunnel_protection_scope_active_requests", Type: "gauge", Help: "Current active requests by protection scope.", Labels: map[string]string{"scope": scope.Scope}, Value: float64(scope.ActiveRequests)},
				prometheusMetric{Name: "siptunnel_protection_scope_rate_limit_hits_total", Type: "counter", Help: "Rate limit hits by protection scope.", Labels: map[string]string{"scope": scope.Scope}, Value: float64(scope.RateLimitHitsTotal)},
				prometheusMetric{Name: "siptunnel_protection_scope_concurrent_rejects_total", Type: "counter", Help: "Concurrency rejects by protection scope.", Labels: map[string]string{"scope": scope.Scope}, Value: float64(scope.ConcurrentRejects)},
				prometheusMetric{Name: "siptunnel_protection_scope_allowed_total", Type: "counter", Help: "Allowed requests by protection scope.", Labels: map[string]string{"scope": scope.Scope}, Value: float64(scope.AllowedTotal)},
			)
		}
	}

	circuitSnapshot := defaultUpstreamCircuitGuard.Snapshot(now)
	metrics = append(metrics,
		prometheusMetric{Name: "siptunnel_circuit_open_count", Type: "gauge", Help: "Current upstream circuit open count.", Value: float64(circuitSnapshot.OpenCount)},
		prometheusMetric{Name: "siptunnel_circuit_half_open_count", Type: "gauge", Help: "Current upstream circuit half-open count.", Value: float64(circuitSnapshot.HalfOpenCount)},
		prometheusMetric{Name: "siptunnel_circuit_open_state", Type: "gauge", Help: "Whether any upstream circuit is currently open.", Value: boolFloat(circuitSnapshot.OpenCount > 0)},
		prometheusMetric{Name: "siptunnel_circuit_half_open_state", Type: "gauge", Help: "Whether any upstream circuit is currently half-open.", Value: boolFloat(circuitSnapshot.HalfOpenCount > 0)},
	)

	if d.runtime != nil {
		runtimeSnapshot := d.runtime.Snapshot()
		stateCounts := map[string]int{}
		for _, item := range runtimeSnapshot {
			stateCounts[item.State]++
		}
		for state, count := range stateCounts {
			metrics = append(metrics, prometheusMetric{Name: "siptunnel_mapping_runtime_state", Type: "gauge", Help: "Mapping runtime state counts.", Labels: map[string]string{"state": state}, Value: float64(count)})
		}
		metrics = append(metrics, prometheusMetric{Name: "siptunnel_transport_recovery_failed_total", Type: "counter", Help: "Total mapping transport recovery failures observed by runtime manager.", Value: float64(d.runtime.RecoveryFailureTotal())})
	}

	if d.selfCheckProvider != nil {
		report := d.selfCheckProvider(ctx)
		metrics = append(metrics,
			prometheusMetric{Name: "siptunnel_selfcheck_items", Type: "gauge", Help: "Self-check item count by level.", Labels: map[string]string{"level": string(selfcheck.LevelInfo)}, Value: float64(report.Summary.Info)},
			prometheusMetric{Name: "siptunnel_selfcheck_items", Type: "gauge", Help: "Self-check item count by level.", Labels: map[string]string{"level": string(selfcheck.LevelWarn)}, Value: float64(report.Summary.Warn)},
			prometheusMetric{Name: "siptunnel_selfcheck_items", Type: "gauge", Help: "Self-check item count by level.", Labels: map[string]string{"level": string(selfcheck.LevelError)}, Value: float64(report.Summary.Error)},
			prometheusMetric{Name: "siptunnel_selfcheck_overall_state", Type: "gauge", Help: "Overall self-check status, 1 means report overall matches the given level.", Labels: map[string]string{"level": string(selfcheck.LevelInfo)}, Value: boolFloat(report.Overall == selfcheck.LevelInfo)},
			prometheusMetric{Name: "siptunnel_selfcheck_overall_state", Type: "gauge", Help: "Overall self-check status, 1 means report overall matches the given level.", Labels: map[string]string{"level": string(selfcheck.LevelWarn)}, Value: boolFloat(report.Overall == selfcheck.LevelWarn)},
			prometheusMetric{Name: "siptunnel_selfcheck_overall_state", Type: "gauge", Help: "Overall self-check status, 1 means report overall matches the given level.", Labels: map[string]string{"level": string(selfcheck.LevelError)}, Value: boolFloat(report.Overall == selfcheck.LevelError)},
		)
	}

	tasks, err := d.repo.ListTasks(ctx, repository.TaskFilter{})
	if err == nil {
		counts := map[string]int{"all": len(tasks)}
		for _, task := range tasks {
			counts[string(task.Status)]++
		}
		keys := make([]string, 0, len(counts))
		for status := range counts {
			keys = append(keys, status)
		}
		sort.Strings(keys)
		for _, status := range keys {
			metrics = append(metrics, prometheusMetric{Name: "siptunnel_task_total", Type: "counter", Help: "Total tasks grouped by status.", Labels: map[string]string{"status": status}, Value: float64(counts[status])})
		}
	}

	return formatPrometheusMetrics(metrics)
}

func formatPrometheusMetrics(metrics []prometheusMetric) string {
	var b strings.Builder
	declared := map[string]struct{}{}
	for _, metric := range metrics {
		if metric.Name == "" {
			continue
		}
		if _, ok := declared[metric.Name]; !ok {
			if metric.Help != "" {
				b.WriteString("# HELP ")
				b.WriteString(metric.Name)
				b.WriteByte(' ')
				b.WriteString(metric.Help)
				b.WriteByte('\n')
			}
			if metric.Type != "" {
				b.WriteString("# TYPE ")
				b.WriteString(metric.Name)
				b.WriteByte(' ')
				b.WriteString(metric.Type)
				b.WriteByte('\n')
			}
			declared[metric.Name] = struct{}{}
		}
		b.WriteString(metric.Name)
		if len(metric.Labels) > 0 {
			b.WriteByte('{')
			keys := make([]string, 0, len(metric.Labels))
			for key := range metric.Labels {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for idx, key := range keys {
				if idx > 0 {
					b.WriteByte(',')
				}
				b.WriteString(key)
				b.WriteString("=\"")
				b.WriteString(escapePrometheusLabel(metric.Labels[key]))
				b.WriteString("\"")
			}
			b.WriteByte('}')
		}
		b.WriteByte(' ')
		b.WriteString(strconv.FormatFloat(metric.Value, 'f', -1, 64))
		b.WriteByte('\n')
	}
	return b.String()
}

func escapePrometheusLabel(input string) string {
	replacer := strings.NewReplacer(`\\`, `\\\\`, `"`, `\\"`, "\n", `\\n`)
	return replacer.Replace(input)
}

func asFloat(v any) float64 {
	switch value := v.(type) {
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case float64:
		return value
	case float32:
		return float64(value)
	default:
		return 0
	}
}

func boolFloat(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func (m *mappingRuntimeManager) RecoveryFailureTotal() uint64 {
	if m == nil {
		return 0
	}
	return m.recoveryFailedTotal.Load()
}
