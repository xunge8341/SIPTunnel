#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPORT_DIR="${ACCEPTANCE_REPORT_DIR:-$ROOT_DIR/artifacts/acceptance}"
TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
REPORT_BASENAME="phase1-strict-source-${TIMESTAMP}"
REPORT_JSON="$REPORT_DIR/${REPORT_BASENAME}.json"
REPORT_MD="$REPORT_DIR/${REPORT_BASENAME}.md"

mkdir -p "$REPORT_DIR"

python3 - "$ROOT_DIR" "$REPORT_JSON" "$REPORT_MD" <<'PY'
from __future__ import annotations
import json, re, sys
from pathlib import Path
from datetime import datetime, timezone

root = Path(sys.argv[1])
report_json = Path(sys.argv[2])
report_md = Path(sys.argv[3])

def read(rel: str) -> str:
    path = root / rel
    if not path.exists():
        return ""
    return path.read_text(encoding="utf-8", errors="replace")

checks = []

def add_check(name: str, passed: bool, detail: str):
    checks.append({"name": name, "passed": passed, "detail": detail})

server_text = read("gateway-server/internal/server/gb28181_tunnel.go")
xml_text = read("gateway-server/internal/protocol/manscdp/xml.go")
http_text = read("gateway-server/internal/server/http.go")
ops_text = read("gateway-server/internal/server/ops_settings_logs.go")
ui_types_text = read("gateway-ui/src/types/gateway.ts")
ui_tunnel_text = read("gateway-ui/src/views/TunnelConfigView.vue")
ui_nodes_text = read("gateway-ui/src/views/NodesAndTunnelsView.vue")
mock_text = read("gateway-ui/src/api/mockGateway.ts")
readme_text = read("README.md")
phase_doc_text = read("docs/GB28181_PHASE1_ACCEPTANCE_AND_PCAP_CHECKLIST_20260318.md")
ui_build_ps1 = read("scripts/ui-build.ps1")
embed_ui_ps1 = read("scripts/embed-ui.ps1")
acceptance_ps1 = read("scripts/acceptance/run_phase1_strict_acceptance.ps1")
ui_delivery_guard = read("scripts/ui-delivery-guard.mjs")
startup_transport_summary = read("gateway-server/cmd/gateway/startup_transport_summary.go")
system_settings_backend = read("gateway-server/internal/server/http.go") + "\n" + read("gateway-server/internal/server/ops_system_settings_http.go")
system_settings_ui = read("gateway-ui/src/types/gateway.ts") + "\n" + read("gateway-ui/src/api/gateway.ts") + "\n" + read("gateway-ui/src/views/SystemSettingsView.vue")
resource_usage_backend = read("gateway-server/internal/server/ops_resource_usage_http.go") + "\n" + read("gateway-server/internal/server/ops_settings_logs.go")
resource_usage_ui = read("gateway-ui/src/types/gateway.ts") + "\n" + read("gateway-ui/src/views/SystemSettingsView.vue") + "\n" + read("gateway-ui/src/views/AlertsAndRateLimitView.vue")

# Positive checks
add_check(
    "MESSAGE 作为控制面主入口",
    'case "MESSAGE":' in server_text,
    "期望 gb28181_tunnel.go 存在 MESSAGE 路由分支。",
)
add_check(
    "Catalog SUBSCRIBE 携带 MANSCDP body",
    'SetHeader("Content-Type", manscdp.ContentType)' in server_text and ('Catalog' in server_text and 'SN:' not in server_text or 'BuildCatalog' in server_text or 'BuildQuery' in xml_text),
    "期望 SUBSCRIBE 同时具备 Content-Type: Application/MANSCDP+xml 与 XML body。",
)
add_check(
    "REGISTER 200 OK Date 使用 GB28181 格式",
    '2006-01-02T15:04:05.000' in server_text or 'formatGB28181Date' in server_text,
    "期望存在 yyyy-MM-dd'T'HH:mm:ss.SSS 格式化实现。",
)
add_check(
    "INVITE/SDP 使用 video + PS/90000",
    'm=video ' in xml_text and 'a=rtpmap:96 PS/90000' in xml_text,
    "期望 SDP 为 m=video + a=rtpmap:96 PS/90000。",
)
add_check(
    "Allow 头完整",
    'Allow' in server_text and 'MESSAGE' in server_text and 'SUBSCRIBE' in server_text and 'NOTIFY' in server_text,
    "期望 INVITE/200 OK 路径补齐 Allow 头。",
)
add_check(
    "secret 改为只写化",
    'register_auth_password_configured' in http_text and 'register_auth_password_configured' in ui_types_text,
    "期望后端与 UI 使用 register_auth_password_configured，而不是读回原文密码。",
)
add_check(
    "mock 与 UI 文案同步严格模式",
    'register_auth_password_configured' in mock_text and ('严格模式' in ui_tunnel_text or '严格模式' in ui_nodes_text),
    "期望 mock、UI 与严格模式术语同步。",
)


add_check(
    "Startup transport summary helper承接 converged generic 事实源",
    'convergedGeneric' not in read("gateway-server/cmd/gateway/main_startup.go") and 'responseModePolicy :=' not in read("gateway-server/cmd/gateway/main_startup.go") and 'func effectiveTransportTuningSummary(' in startup_transport_summary and 'func logAppliedTransportTuning(' in startup_transport_summary,
    "期望 converged generic / startup 摘要别名只在 startup_transport_summary.go 中维护，避免 main_startup.go 再次出现未定义或未使用局部变量。",
)

ops_profile_needles = [
    'generic_download_total_mbps',
    'generic_download_per_transfer_mbps',
    'generic_download_window_mb',
    'adaptive_hot_cache_mb',
    'adaptive_hot_window_mb',
    'generic_download_segment_concurrency',
    'generic_download_rtp_reorder_window_packets',
    'generic_download_rtp_loss_tolerance_packets',
    'generic_download_rtp_gap_timeout_ms',
    'generic_download_rtp_fec_enabled',
    'generic_download_rtp_fec_group_packets',
]
ops_profile_missing = [needle for needle in ops_profile_needles if needle not in system_settings_backend or needle not in system_settings_ui]
add_check(
    "系统设置的 MB/Mbps 运维收口参数后端/UI 同步",
    not ops_profile_missing,
    '缺失: ' + (', '.join(ops_profile_missing) if ops_profile_missing else '无'),
)

resource_needles = [
    'configured_generic_download_window_mb',
    'configured_generic_segment_concurrency',
    'configured_generic_rtp_reorder_window_packets',
    'configured_generic_rtp_loss_tolerance_packets',
    'configured_generic_rtp_gap_timeout_ms',
    'configured_generic_rtp_fec_enabled',
    'configured_generic_rtp_fec_group_packets',
]
resource_missing = [needle for needle in resource_needles if needle not in resource_usage_backend or needle not in resource_usage_ui]
add_check(
    "系统资源页与告警页共享运行时资源/收口事实",
    not resource_missing,
    '缺失: ' + (', '.join(resource_missing) if resource_missing else '无'),
)

# Negative checks - source/UI should not retain legacy private control cmd names as current mainline.
legacy_hits = []
for rel in [
    'gateway-server/internal',
    'gateway-ui/src',
    'README.md',
    'docs/GB28181_PHASE1_STRICT_MODE_20260318.md',
]:
    p = root / rel
    if not p.exists():
        continue
    files = [p] if p.is_file() else [f for f in p.rglob('*') if f.is_file()]
    for f in files:
        text = f.read_text(encoding='utf-8', errors='replace')
        if re.search(r'HttpInvoke|HttpResponseStart|HttpResponseInline', text):
            legacy_hits.append(str(f.relative_to(root)))
add_check(
    '现行源码/UI/当前文档不残留私有 HttpInvoke* 主路径名',
    not legacy_hits,
    '残留文件: ' + (', '.join(legacy_hits) if legacy_hits else '无'),
)

# INFO should not be used as live main path.
info_outbound = []
for pattern in [r'NewRequest\("INFO"', r'case\s+"INFO"\s*:']:
    if re.search(pattern, server_text):
        info_outbound.append(pattern)
add_check(
    '源码中不再存在 INFO 主路径发起/处理',
    not info_outbound,
    '命中模式: ' + (', '.join(info_outbound) if info_outbound else '无'),
)

legacy_ui_paths = [
    root / 'gateway-ui' / 'src' / 'views' / 'ConfigGovernanceView.vue',
    root / 'gateway-ui' / 'src' / 'views' / 'ConfigTransferView.vue',
    root / 'gateway-ui' / 'src' / 'views' / 'NetworkConfigView.vue',
    root / 'gateway-ui' / 'src' / 'views' / 'NodeStatusView.vue',
    root / 'gateway-ui' / 'src' / 'views' / '__tests__' / 'NodeStatusView.spec.ts',
    root / 'gateway-ui' / 'src' / 'api' / '__tests__' / 'gatewayConfigTransfer.spec.ts',
]
legacy_ui_hits = [str(path.relative_to(root)) for path in legacy_ui_paths if path.exists()]
add_check(
    '已清退 legacy UI 文件不再残留于源码树',
    not legacy_ui_hits,
    '残留文件: ' + (', '.join(legacy_ui_hits) if legacy_ui_hits else '无'),
)

add_check(
    'UI delivery guard 已接入构建与嵌入脚本',
    'legacyFiles' in ui_delivery_guard and 'ui-delivery-guard.mjs' in ui_build_ps1 and 'delivery_guard_status' in embed_ui_ps1,
    '期望 ui-delivery-guard.mjs 已接入 ui-build/embed 流程。',
)

add_check(
    'Windows 严格验收会先同步 go.mod/go.sum，再跑 server_targeted 和 build_release',
    "Invoke-Step 'server_targeted'" in acceptance_ps1 and 'Sync-GoModuleGraph' in acceptance_ps1 and 'go mod tidy' in acceptance_ps1 and 'go test ./internal/protocol/... ./internal/server ./internal/selfcheck ./internal/config -count=1' in acceptance_ps1 and acceptance_ps1.find("Invoke-Step 'server_targeted'") < acceptance_ps1.find("Invoke-Step 'build_release'"),
    '期望 PowerShell 严格验收先执行 go mod tidy/Sync-GoModuleGraph，再执行 server_targeted，最后执行 build_release。',
)

startup_transport_summary = read(root / 'gateway-server' / 'cmd' / 'gateway' / 'startup_transport_summary.go')
startup_main = read(root / 'gateway-server' / 'cmd' / 'gateway' / 'main_startup.go')
add_check(
    '启动阶段收口摘要统一由 startup_transport_summary.go 输出',
    'convergedGeneric' not in startup_main
    and 'func effectiveTransportTuningSummary(' in startup_transport_summary
    and 'func logAppliedTransportTuning(' in startup_transport_summary
    and startup_transport_summary.count('convergedGeneric := config.ConvergedGenericDownloadProfile(') >= 2,
    '期望收口摘要/日志统一通过 startup_transport_summary.go 生成，避免 main_startup.go 再次漂移。',
)

overall = all(item['passed'] for item in checks)
report = {
    'generated_at': datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ'),
    'overall': 'PASS' if overall else 'FAIL',
    'summary': {
        'total': len(checks),
        'pass': sum(1 for c in checks if c['passed']),
        'fail': sum(1 for c in checks if not c['passed']),
    },
    'checks': checks,
}
report_json.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
lines = [
    '# GB28181 第一阶段严格模式源码验收报告',
    '',
    f"- GeneratedAt(Local): `{report['generated_at']}`",
    f"- Overall: **{report['overall']}**",
    '',
    '| 检查项 | 结果 | 说明 |',
    '|---|---|---|',
]
for c in checks:
    lines.append(f"| {c['name']} | {'PASS' if c['passed'] else 'FAIL'} | {c['detail'].replace('|', '\\|')} |")
lines += [
    '',
    '说明：该脚本面向**阶段一严格模式**，会把私有 `INFO(HttpInvoke/HttpResponse*)` 主路径视为不通过。',
]
report_md.write_text('\n'.join(lines), encoding='utf-8')
print(report_md)
print(report_json)
if not overall:
    raise SystemExit(1)
PY
