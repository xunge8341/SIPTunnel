package server

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"siptunnel/loadtest"
)

type loadtestJob struct {
	JobID              string                      `json:"job_id"`
	Status             string                      `json:"status"`
	CreatedAt          string                      `json:"created_at"`
	UpdatedAt          string                      `json:"updated_at"`
	Targets            []string                    `json:"targets"`
	HTTPURL            string                      `json:"http_url"`
	SIPAddress         string                      `json:"sip_address"`
	SIPTransport       string                      `json:"sip_transport"`
	RTPAddress         string                      `json:"rtp_address"`
	RTPTransport       string                      `json:"rtp_transport"`
	GatewayBaseURL     string                      `json:"gateway_base_url"`
	Concurrency        int                         `json:"concurrency"`
	QPS                int                         `json:"qps"`
	DurationSec        int                         `json:"duration_sec"`
	OutputDir          string                      `json:"output_dir"`
	SummaryFile        string                      `json:"summary_file,omitempty"`
	ReportFile         string                      `json:"report_file,omitempty"`
	ErrorMessage       string                      `json:"error_message,omitempty"`
	CapacitySuggestion map[string]any              `json:"capacity_suggestion,omitempty"`
	Summaries          map[string]loadtest.Summary `json:"summaries,omitempty"`
}

type loadtestJobStore struct {
	mu   sync.RWMutex
	jobs []loadtestJob
}

func newLoadtestJobStore() *loadtestJobStore {
	return &loadtestJobStore{jobs: make([]loadtestJob, 0, 16)}
}

func (s *loadtestJobStore) upsert(job loadtestJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.jobs {
		if s.jobs[i].JobID == job.JobID {
			s.jobs[i] = job
			return
		}
	}
	s.jobs = append([]loadtestJob{job}, s.jobs...)
}

func (s *loadtestJobStore) list() []loadtestJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]loadtestJob, len(s.jobs))
	copy(out, s.jobs)
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

func (s *loadtestJobStore) get(id string) (loadtestJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, job := range s.jobs {
		if job.JobID == id {
			return job, true
		}
	}
	return loadtestJob{}, false
}

func ensureDir(dir string) string {
	value := strings.TrimSpace(dir)
	if value == "" {
		value = filepath.Join(".", "data", "final", "loadtest")
	}
	_ = os.MkdirAll(value, 0o755)
	return value
}

func startLoadtestJob(ctx context.Context, store *loadtestJobStore, limits OpsLimits, status NodeNetworkStatus, job loadtestJob) {
	if store == nil {
		return
	}
	go func() {
		started := time.Now().UTC()
		job.Status = "running"
		job.UpdatedAt = formatTimestamp(started)
		store.upsert(job)

		cfg := loadtest.Config{
			Targets:        job.Targets,
			HTTPURL:        job.HTTPURL,
			SIPAddress:     job.SIPAddress,
			SIPTransport:   job.SIPTransport,
			RTPAddress:     job.RTPAddress,
			RTPTransport:   job.RTPTransport,
			GatewayBaseURL: job.GatewayBaseURL,
			Concurrency:    job.Concurrency,
			QPS:            job.QPS,
			Duration:       time.Duration(job.DurationSec) * time.Second,
			OutputDir:      ensureDir(job.OutputDir),
		}
		report, err := loadtest.Run(ctx, cfg)
		job.UpdatedAt = formatTimestamp(time.Now().UTC())
		if err != nil {
			job.Status = "failed"
			job.ErrorMessage = err.Error()
			store.upsert(job)
			return
		}
		job.Status = "succeeded"
		job.SummaryFile = report.ResultFile
		job.ReportFile = report.ReportFile
		job.Summaries = report.Summaries
		assessment := loadtest.AssessCapacity(report, loadtest.CapacityCurrentConfig{
			CommandMaxConcurrent:      limits.MaxConcurrent,
			FileTransferMaxConcurrent: maxIntVal(1, status.RTP.ActiveTransfers),
			RTPPortPoolSize:           maxIntVal(64, status.RTP.PortPoolTotal),
			MaxConnections:            maxIntVal(32, status.SIP.MaxConnections),
			RateLimitRPS:              limits.RPS,
			RateLimitBurst:            limits.Burst,
		})
		job.CapacitySuggestion = map[string]any{
			"source":                                   "loadtest-summary",
			"current_network_mode":                     string(status.NetworkMode),
			"current_command_max_concurrent":           assessment.Current.CommandMaxConcurrent,
			"current_file_transfer_max_concurrent":     assessment.Current.FileTransferMaxConcurrent,
			"current_rtp_port_pool_size":               assessment.Current.RTPPortPoolSize,
			"current_max_connections":                  assessment.Current.MaxConnections,
			"current_rate_limit_rps":                   assessment.Current.RateLimitRPS,
			"current_rate_limit_burst":                 assessment.Current.RateLimitBurst,
			"recommended_command_max_concurrent":       assessment.Recommendation.RecommendedCommandMaxConcurrent,
			"recommended_file_transfer_max_concurrent": assessment.Recommendation.RecommendedFileTransferMaxConcurrent,
			"recommended_rtp_port_pool_size":           assessment.Recommendation.RecommendedRTPPortPoolSize,
			"recommended_max_connections":              assessment.Recommendation.RecommendedMaxConnections,
			"recommended_rate_limit_rps":               assessment.Recommendation.RecommendedRateLimitRPS,
			"recommended_rate_limit_burst":             assessment.Recommendation.RecommendedRateLimitBurst,
			"basis":                                    assessment.Recommendation.Basis,
			"note":                                     "建议结合告警与保护页评估失败率、并发阈值与限流策略。",
		}
		store.upsert(job)
	}()
}

func maxIntVal(a, b int) int {
	if a > b {
		return a
	}
	return b
}
