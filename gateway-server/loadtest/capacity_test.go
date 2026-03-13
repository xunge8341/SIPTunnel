package loadtest

import "testing"

func TestAssessCapacity(t *testing.T) {
	report := Report{Summaries: map[string]Summary{
		"sip-command-create": {Target: "sip-command-create", Total: 1000, SuccessRate: 0.999, Throughput: 240, P95MS: 80, Concurrency: 120},
		"rtp-udp-upload":     {Target: "rtp-udp-upload", Total: 1000, SuccessRate: 0.998, Throughput: 90, P95MS: 180, Concurrency: 60},
	}}
	current := CapacityCurrentConfig{CommandMaxConcurrent: 80, FileTransferMaxConcurrent: 40, RTPPortPoolSize: 128, MaxConnections: 160, RateLimitRPS: 250, RateLimitBurst: 400}
	assessment := AssessCapacity(report, current)

	if assessment.Recommendation.RecommendedCommandMaxConcurrent <= 0 {
		t.Fatalf("unexpected command recommendation: %+v", assessment.Recommendation)
	}
	if assessment.Recommendation.RecommendedFileTransferMaxConcurrent <= 0 {
		t.Fatalf("unexpected file recommendation: %+v", assessment.Recommendation)
	}
	if assessment.Recommendation.RecommendedRTPPortPoolSize < 64 {
		t.Fatalf("unexpected rtp pool recommendation: %+v", assessment.Recommendation)
	}
	if len(assessment.Recommendation.Basis) == 0 {
		t.Fatalf("expected basis notes")
	}
}

func TestAssessCapacityFallbackToCurrent(t *testing.T) {
	assessment := AssessCapacity(Report{}, CapacityCurrentConfig{CommandMaxConcurrent: 10, FileTransferMaxConcurrent: 5, RateLimitRPS: 80})
	if assessment.Recommendation.RecommendedCommandMaxConcurrent != 10 {
		t.Fatalf("fallback command not match: %+v", assessment.Recommendation)
	}
	if assessment.Recommendation.RecommendedFileTransferMaxConcurrent != 5 {
		t.Fatalf("fallback file not match: %+v", assessment.Recommendation)
	}
}
