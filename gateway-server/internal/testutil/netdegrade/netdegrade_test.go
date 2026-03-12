package netdegrade

import (
	"strings"
	"testing"
)

func TestSummarizeMetrics(t *testing.T) {
	summaries := Summarize([]Sample{
		{
			Link:            LinkSIPTCP,
			Scenario:        "delay-jitter",
			Condition:       Condition{DelayMS: 120, JitterMS: 30},
			Attempts:        100,
			Successes:       98,
			AvgLatencyMS:    143.2,
			Retransmissions: 9,
			RecoveryTimeMS:  350,
		},
	})
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	s := summaries[0]
	if s.SuccessRate != 0.98 {
		t.Fatalf("unexpected success rate: %.2f", s.SuccessRate)
	}
	if s.RetransmitRate != 9.0/98.0 {
		t.Fatalf("unexpected retransmit rate: %.4f", s.RetransmitRate)
	}
}

func TestRenderTemplate(t *testing.T) {
	tpl := "{{range .Summaries}}|{{.Link}}|{{percent .SuccessRate}}|{{join .ManualValidation \";\"}}|{{end}}"
	summaries := []Summary{{
		Link:             LinkRTPUDP,
		SuccessRate:      0.91,
		ManualValidation: []string{"断连后抓包确认重建", "验证恢复耗时"},
	}}
	out, err := Render(tpl, summaries)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if !strings.Contains(out, "91.00%") {
		t.Fatalf("missing percent output: %s", out)
	}
	if !strings.Contains(out, "断连后抓包确认重建;验证恢复耗时") {
		t.Fatalf("missing manual step output: %s", out)
	}
}
