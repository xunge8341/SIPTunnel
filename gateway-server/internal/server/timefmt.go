package server

import (
	"fmt"
	"strings"
	"time"
)

const unifiedTimestampLayout = "2006-01-02 15:04:05.000"

var supportedTimestampLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	unifiedTimestampLayout,
	"2006-01-02 15:04:05",
	"2006-01-02",
}

func formatTimestamp(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(unifiedTimestampLayout)
}

func parseTimestamp(value string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	for _, layout := range supportedTimestampLayouts {
		if ts, err := time.Parse(layout, trimmed); err == nil {
			return ts, nil
		}
		if layout == unifiedTimestampLayout || layout == "2006-01-02 15:04:05" || layout == "2006-01-02" {
			if ts, err := time.ParseInLocation(layout, trimmed, time.UTC); err == nil {
				return ts, nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp format: %s", trimmed)
}

func parseTimestampOrNow(value string) time.Time {
	if ts, err := parseTimestamp(value); err == nil {
		return ts
	}
	return time.Now()
}
