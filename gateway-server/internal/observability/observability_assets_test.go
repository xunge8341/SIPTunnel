package observability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gopkg.in/yaml.v3"
)

type prometheusRuleFile struct {
	Groups []struct {
		Rules []struct {
			Alert       string            `yaml:"alert"`
			Labels      map[string]string `yaml:"labels"`
			Annotations map[string]string `yaml:"annotations"`
		} `yaml:"rules"`
	} `yaml:"groups"`
}

type grafanaDashboard struct {
	Title  string `json:"title"`
	Templating struct {
		List []struct {
			Name string `json:"name"`
		} `json:"list"`
	} `json:"templating"`
	Panels []struct {
		Title string `json:"title"`
	} `json:"panels"`
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve runtime caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}

func TestPrometheusAlertRulesCoverage(t *testing.T) {
	path := filepath.Join(repoRoot(t), "deploy", "observability", "prometheus", "alerts.yaml")
	buf, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read alert rules: %v", err)
	}

	var rules prometheusRuleFile
	if err := yaml.Unmarshal(buf, &rules); err != nil {
		t.Fatalf("parse alert rules yaml: %v", err)
	}

	requiredAlerts := map[string]struct{}{
		"SIPTunnelConnectionErrorSpike":    {},
		"SIPTunnelTaskFailureRateHigh":     {},
		"SIPTunnelRateLimitHitHigh":        {},
		"SIPTunnelRTPPortAllocFailure":     {},
		"SIPTunnelTransportRecoveryFailed": {},
		"SIPTunnelGoroutineGrowthAnomaly":  {},
		"SIPTunnelDataDiskUsageHigh":       {},
	}

	for _, group := range rules.Groups {
		for _, rule := range group.Rules {
			delete(requiredAlerts, rule.Alert)
			for _, label := range []string{"severity", "team", "service", "component", "category"} {
				if rule.Labels[label] == "" {
					t.Fatalf("alert %s missing label %s", rule.Alert, label)
				}
			}
			for _, annotation := range []string{"summary", "description", "runbook_url"} {
				if rule.Annotations[annotation] == "" {
					t.Fatalf("alert %s missing annotation %s", rule.Alert, annotation)
				}
			}
		}
	}

	if len(requiredAlerts) > 0 {
		t.Fatalf("missing required alerts: %v", requiredAlerts)
	}
}

func TestGrafanaDashboardCoverage(t *testing.T) {
	path := filepath.Join(repoRoot(t), "deploy", "observability", "grafana", "siptunnel-ops-dashboard.json")
	buf, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read dashboard json: %v", err)
	}

	var dashboard grafanaDashboard
	if err := json.Unmarshal(buf, &dashboard); err != nil {
		t.Fatalf("parse dashboard json: %v", err)
	}

	if dashboard.Title == "" {
		t.Fatal("dashboard title must not be empty")
	}

	requiredPanels := map[string]struct{}{
		"SIP TCP 面板":     {},
		"RTP UDP/TCP 面板": {},
		"任务面板":           {},
		"文件传输面板":         {},
		"限流与错误面板":        {},
	}

	for _, panel := range dashboard.Panels {
		delete(requiredPanels, panel.Title)
	}

	hasInstanceVariable := false
	for _, variable := range dashboard.Templating.List {
		if variable.Name == "instance" {
			hasInstanceVariable = true
			break
		}
	}
	if !hasInstanceVariable {
		t.Fatal("dashboard missing instance variable")
	}

	if len(requiredPanels) > 0 {
		t.Fatalf("missing required panels: %v", requiredPanels)
	}
}
