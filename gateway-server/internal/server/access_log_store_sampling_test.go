package server

import (
	"fmt"
	"testing"

	"siptunnel/internal/persistence"
)

func TestAccessLogStoreShouldSampleOutOnlySuccessfulFastRequestsUnderPressure(t *testing.T) {
	store := &accessLogStore{sqlite: &persistence.SQLiteStore{}, persistCh: make(chan persistence.AccessLogRecord, 8)}
	for i := 0; i < 7; i++ {
		store.persistCh <- persistence.AccessLogRecord{ID: "existing"}
	}
	sampledKey := ""
	for i := 0; i < 1024; i++ {
		candidate := persistence.AccessLogRecord{ID: "rec", RequestID: fmt.Sprintf("req-sample-%d", i), StatusCode: 200, DurationMS: 10, Path: "/healthz", MappingName: "orders"}
		if store.shouldSampleOut(candidate) {
			sampledKey = candidate.RequestID
			break
		}
	}
	if sampledKey == "" {
		t.Fatal("expected at least one sampled key under queue pressure")
	}
	if !store.shouldSampleOut(persistence.AccessLogRecord{RequestID: sampledKey, StatusCode: 200, DurationMS: 10, Path: "/healthz", MappingName: "orders"}) {
		t.Fatal("expected successful fast request to be sampled out under queue pressure")
	}
	if store.shouldSampleOut(persistence.AccessLogRecord{RequestID: sampledKey, StatusCode: 502, DurationMS: 10, FailureReason: "upstream timeout", Path: "/healthz", MappingName: "orders"}) {
		t.Fatal("failed request should never be sampled out")
	}
	if store.shouldSampleOut(persistence.AccessLogRecord{RequestID: sampledKey, StatusCode: 200, DurationMS: 900, Path: "/healthz", MappingName: "orders"}) {
		t.Fatal("slow request should never be sampled out")
	}
}
