package configdoc

import (
	"strings"
	"testing"
)

func TestRenderMarkdownIncludesRiskAndColumns(t *testing.T) {
	md := RenderMarkdown()
	if !strings.Contains(md, "| 参数名 | 类型 | 默认值 | 热更新 | 风险等级 | 说明 |") {
		t.Fatalf("missing table header")
	}
	if !strings.Contains(md, "⚠️ HIGH-NET") {
		t.Fatalf("missing high network risk mark")
	}
	if !strings.Contains(md, "`network.sip.listen_port`") {
		t.Fatalf("missing parameter row")
	}
}

func TestRenderYAMLProfiles(t *testing.T) {
	dev := RenderYAML(ProfileDev)
	if !strings.Contains(dev, "listen_ip: 127.0.0.1") {
		t.Fatalf("dev profile should bind localhost")
	}
	prod := RenderYAML(ProfileProd)
	if !strings.Contains(prod, "domain: prod.siptunnel.local") {
		t.Fatalf("prod profile should include production domain")
	}
	if !strings.Contains(prod, "max_inflight_transfers: 128") {
		t.Fatalf("prod profile should have larger inflight setting")
	}
}
