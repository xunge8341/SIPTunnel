package netdegrade

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"time"
)

type Link string

const (
	LinkSIPTCP Link = "SIP_TCP"
	LinkRTPUDP Link = "RTP_UDP"
	LinkRTPTCP Link = "RTP_TCP"
)

type Condition struct {
	DelayMS        int     `json:"delay_ms"`
	JitterMS       int     `json:"jitter_ms"`
	LossPercent    float64 `json:"loss_percent"`
	ReorderPercent float64 `json:"reorder_percent"`
	DisconnectMS   int     `json:"disconnect_ms"`
	BandwidthKbps  int     `json:"bandwidth_kbps"`
}

type Sample struct {
	Link             Link      `json:"link"`
	Scenario         string    `json:"scenario"`
	Condition        Condition `json:"condition"`
	Attempts         int       `json:"attempts"`
	Successes        int       `json:"successes"`
	AvgLatencyMS     float64   `json:"avg_latency_ms"`
	Retransmissions  int       `json:"retransmissions"`
	RecoveryTimeMS   float64   `json:"recovery_time_ms"`
	ManualValidation []string  `json:"manual_validation,omitempty"`
}

type Summary struct {
	Link             Link
	Scenario         string
	Condition        Condition
	SuccessRate      float64
	AvgLatencyMS     float64
	RetransmitRate   float64
	RecoveryTimeMS   float64
	ManualValidation []string
}

type ReportData struct {
	Summaries []Summary
}

func Summarize(samples []Sample) []Summary {
	if len(samples) == 0 {
		return nil
	}
	out := make([]Summary, 0, len(samples))
	for _, s := range samples {
		sr := 0.0
		if s.Attempts > 0 {
			sr = float64(s.Successes) / float64(s.Attempts)
		}
		rr := 0.0
		if s.Successes > 0 {
			rr = float64(s.Retransmissions) / float64(s.Successes)
		}
		out = append(out, Summary{
			Link:             s.Link,
			Scenario:         s.Scenario,
			Condition:        s.Condition,
			SuccessRate:      sr,
			AvgLatencyMS:     s.AvgLatencyMS,
			RetransmitRate:   rr,
			RecoveryTimeMS:   s.RecoveryTimeMS,
			ManualValidation: s.ManualValidation,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Link == out[j].Link {
			return out[i].Scenario < out[j].Scenario
		}
		return out[i].Link < out[j].Link
	})
	return out
}

func Render(templateText string, summaries []Summary) (string, error) {
	tpl, err := template.New("report").Funcs(template.FuncMap{
		"percent": func(v float64) string {
			return fmt.Sprintf("%.2f%%", v*100)
		},
		"join": strings.Join,
		"now":  func() string { return time.Now().Format(time.RFC3339) },
	}).Parse(templateText)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, ReportData{Summaries: summaries}); err != nil {
		return "", err
	}
	return buf.String(), nil
}
