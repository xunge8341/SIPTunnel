package server

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const defaultCleanupSchedule = "*/30 * * * *"

func validateCleanupSchedule(expr string) error {
	_, err := nextCleanupDelay(expr, time.Now().UTC())
	return err
}

func cleanupScheduleFromInterval(interval time.Duration) string {
	if interval <= 0 {
		return defaultCleanupSchedule
	}
	return "@every " + interval.String()
}

func nextCleanupDelay(expr string, now time.Time) (time.Duration, error) {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" {
		trimmed = defaultCleanupSchedule
	}
	if strings.HasPrefix(trimmed, "@every ") {
		interval, err := time.ParseDuration(strings.TrimSpace(strings.TrimPrefix(trimmed, "@every ")))
		if err != nil {
			return 0, fmt.Errorf("log_cleanup_cron is invalid: %w", err)
		}
		if interval < time.Minute {
			return 0, fmt.Errorf("log_cleanup_cron is invalid: minimum interval is 1m")
		}
		return interval, nil
	}
	fields := strings.Fields(trimmed)
	if len(fields) != 5 {
		return 0, fmt.Errorf("log_cleanup_cron is invalid")
	}
	if fields[2] != "*" || fields[3] != "*" || fields[4] != "*" {
		return 0, fmt.Errorf("log_cleanup_cron is invalid: day/month/weekday filters are not supported")
	}
	start := now.UTC().Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 366*24*60; i++ {
		candidate := start.Add(time.Duration(i) * time.Minute)
		if cronFieldMatches(fields[0], candidate.Minute(), 0, 59) && cronFieldMatches(fields[1], candidate.Hour(), 0, 23) {
			return candidate.Sub(now.UTC()), nil
		}
	}
	return 0, fmt.Errorf("log_cleanup_cron is invalid: no future execution time found")
}

func cronFieldMatches(field string, value, minValue, maxValue int) bool {
	for _, segment := range strings.Split(strings.TrimSpace(field), ",") {
		if cronSegmentMatches(strings.TrimSpace(segment), value, minValue, maxValue) {
			return true
		}
	}
	return false
}

func cronSegmentMatches(segment string, value, minValue, maxValue int) bool {
	if segment == "" {
		return false
	}
	if segment == "*" {
		return true
	}
	if strings.Contains(segment, "/") {
		parts := strings.SplitN(segment, "/", 2)
		if len(parts) != 2 {
			return false
		}
		step, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || step <= 0 {
			return false
		}
		base := strings.TrimSpace(parts[0])
		switch {
		case base == "*":
			return (value-minValue)%step == 0
		case strings.Contains(base, "-"):
			bounds := strings.SplitN(base, "-", 2)
			start, err1 := strconv.Atoi(strings.TrimSpace(bounds[0]))
			end, err2 := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err1 != nil || err2 != nil || start > end || value < start || value > end {
				return false
			}
			return (value-start)%step == 0
		default:
			start, err := strconv.Atoi(base)
			if err != nil || value < start {
				return false
			}
			return (value-start)%step == 0
		}
	}
	if strings.Contains(segment, "-") {
		bounds := strings.SplitN(segment, "-", 2)
		start, err1 := strconv.Atoi(strings.TrimSpace(bounds[0]))
		end, err2 := strconv.Atoi(strings.TrimSpace(bounds[1]))
		return err1 == nil && err2 == nil && start <= value && value <= end
	}
	parsed, err := strconv.Atoi(segment)
	return err == nil && parsed >= minValue && parsed <= maxValue && parsed == value
}
