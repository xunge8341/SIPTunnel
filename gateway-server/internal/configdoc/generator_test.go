package configdoc

import (
	"strings"
	"testing"
)

func TestRenderMarkdownIncludesRiskAndColumns(t *testing.T) {
	md := RenderMarkdown()
	if !strings.Contains(md, "| 参数名 | 类型 | 默认值 | 热更新 | 风险等级 | 说明 | 可选/校验值 |") {
		t.Fatalf("missing table header")
	}
	if !strings.Contains(md, "⚠️ HIGH-NET") {
		t.Fatalf("missing high network risk mark")
	}
	if !strings.Contains(md, "`network.sip.listen_port`") {
		t.Fatalf("missing parameter row")
	}
	if !strings.Contains(md, "`transport_tuning.udp_control_max_bytes`") {
		t.Fatalf("missing transport tuning row")
	}
}

func TestRenderYAMLProfiles(t *testing.T) {
	dev := RenderYAML(ProfileDev)
	if !strings.Contains(dev, "listen_ip: 127.0.0.1") {
		t.Fatalf("dev profile should bind localhost")
	}
	if !strings.Contains(dev, "transport_tuning:") || !strings.Contains(dev, "# 可选/校验值：推荐 1300；范围 [1024,1400]；低于 1249 会重现现场 oversize") {
		t.Fatalf("generated YAML should include commented transport_tuning guidance")
	}
	prod := RenderYAML(ProfileProd)
	if !strings.Contains(prod, "domain: prod.siptunnel.local") {
		t.Fatalf("prod profile should include production domain")
	}
	if !strings.Contains(prod, "max_inflight_transfers: 128") {
		t.Fatalf("prod profile should have larger inflight setting")
	}
}
