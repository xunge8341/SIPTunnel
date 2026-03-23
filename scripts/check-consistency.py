#!/usr/bin/env python3
from __future__ import annotations

import json
import sys
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
REPORT_DIR = ROOT / 'artifacts' / 'consistency'
REPORT_DIR.mkdir(parents=True, exist_ok=True)
ts = datetime.now(timezone.utc).strftime('%Y%m%d-%H%M%S')
json_path = REPORT_DIR / f'consistency-{ts}.json'
md_path = REPORT_DIR / f'consistency-{ts}.md'

checks: list[dict] = []


def text(path: Path) -> str:
    return path.read_text(encoding='utf-8', errors='replace') if path.exists() else ''


def add(name: str, passed: bool, detail: str) -> None:
    checks.append({'name': name, 'passed': passed, 'detail': detail})


# 1. no legacy server.port in active docs/scripts/config runtime descriptions
legacy_server_hits = []
for path in [ROOT / 'README.md', ROOT / 'docs' / 'README.md', ROOT / 'docs' / 'DEPLOYMENT_AND_OPERATIONS.md']:
    if 'server.port' in text(path):
        legacy_server_hits.append(str(path.relative_to(ROOT)))
add(
    'Active sources do not reintroduce server.port as runtime truth',
    not legacy_server_hits,
    'hits=' + (', '.join(legacy_server_hits) if legacy_server_hits else 'none'),
)

# 2. active docs should not use old menu label 节点与隧道
legacy_menu_hits = []
for path in [
    ROOT / 'README.md',
    ROOT / 'docs' / 'README.md',
    ROOT / 'docs' / 'DEPLOYMENT_AND_OPERATIONS.md',
    ROOT / 'gateway-ui' / 'src' / 'stores' / 'app.ts',
    ROOT / 'gateway-ui' / 'src' / 'router' / 'index.ts',
]:
    if '节点与隧道' in text(path):
        legacy_menu_hits.append(str(path.relative_to(ROOT)))
add(
    'Active docs/UI do not use old menu label 节点与隧道',
    not legacy_menu_hits,
    'hits=' + (', '.join(legacy_menu_hits) if legacy_menu_hits else 'none'),
)

# 3. readme/docs menu completeness
menu_needles = ['节点与级联', '本地资源', '隧道映射', '链路监控', '授权管理', '安全事件']
for target in [ROOT / 'README.md', ROOT / 'docs' / 'README.md']:
    target_text = text(target)
    missing = [needle for needle in menu_needles if needle not in target_text]
    add(
        f'{target.relative_to(ROOT)} lists current primary menus',
        not missing,
        'missing=' + (', '.join(missing) if missing else 'none'),
    )

# 4. configuration cleanup doc reflects ui.listen_port as sole UI port source
cfg_doc = text(ROOT / 'docs' / 'CONFIGURATION_CLEANUP.md')
add(
    'Configuration cleanup doc fixes UI port source to ui.listen_port',
    'ui.listen_port' in cfg_doc and 'server.port' in cfg_doc and '已移除' in cfg_doc,
    'Expect cleanup doc to declare ui.listen_port as sole source and mark server.port removed.',
)

# 5. guardrails doc exists
add(
    'Engineering guardrails document exists',
    (ROOT / 'docs' / 'ENGINEERING_GUARDRAILS.md').exists(),
    'Expect fixed-principles doc to exist.',
)

# 6. active docs mention local resources and tunnel mappings split
split_doc_text = text(ROOT / 'docs' / 'ENGINEERING_GUARDRAILS.md') + '\n' + text(ROOT / 'docs' / 'CONFIGURATION_CLEANUP.md')
add(
    'Active docs define LocalResources vs TunnelMappings separation',
    '本地资源' in split_doc_text and '隧道映射' in split_doc_text and '目录发布源' in split_doc_text,
    'Expect separation of responsibilities to be documented.',
)

# 7. UI nav labels match router titles roughly
store = text(ROOT / 'gateway-ui' / 'src' / 'stores' / 'app.ts')
router = text(ROOT / 'gateway-ui' / 'src' / 'router' / 'index.ts')
nav_ok = all(name in store for name in ['节点与级联', '本地资源', '隧道映射']) and all(
    name in router for name in ['节点与级联', '本地资源', '隧道映射']
)
add('UI store and router use current labels', nav_ok, 'Expect store/router to share current menu naming.')

# 8. active docs should pin current delivery strategy families
strategy_doc = text(ROOT / 'docs' / 'design.md') + '\n' + text(ROOT / 'docs' / 'ENGINEERING_GUARDRAILS.md')
strategy_needles = [
    'stream_primary',
    'range_primary',
    'adaptive_segmented_primary',
    'fallback_segmented',
    'MANUAL / UNEXPOSED',
]
strategy_missing = [needle for needle in strategy_needles if needle not in strategy_doc]
add(
    'Active docs pin current delivery strategies and exposure facts',
    not strategy_missing,
    'missing=' + (', '.join(strategy_missing) if strategy_missing else 'none'),
)

# 9. current UI/API should not present AUTO exposure as runtime truth
active_auto_hits = []
exposure_needles = [
    'auto_items',
    'auto_expose_num',
    "mapping_status: 'AUTO'",
    'mapping_status="AUTO"',
    "exposure_mode: 'AUTO'",
    'exposure_mode="AUTO"',
]
for path in [
    ROOT / 'gateway-ui' / 'src' / 'views' / 'TunnelMappingsView.vue',
    ROOT / 'gateway-ui' / 'src' / 'api' / 'gateway.ts',
    ROOT / 'gateway-ui' / 'src' / 'api' / 'mockGateway.ts',
    ROOT / 'gateway-server' / 'internal' / 'server' / 'catalog_http.go',
    ROOT / 'gateway-server' / 'internal' / 'server' / 'tunnel_mapping_overview_http.go',
]:
    body = text(path)
    if any(needle in body for needle in exposure_needles):
        active_auto_hits.append(str(path.relative_to(ROOT)))
add(
    'Active UI/API do not present AUTO exposure as runtime fact',
    not active_auto_hits,
    'hits=' + (', '.join(active_auto_hits) if active_auto_hits else 'none'),
)

# 10. backend chain review doc exists and captures main chains
backend_chain_doc = text(ROOT / 'docs' / 'BACKEND_CHAIN_SEQUENCE_AND_OPTIMIZATION_20260321.md')
backend_needles = ['stream_primary', 'adaptive_segmented_primary', 'generic-rtp', 'boundary-rtp', 'REGISTER / Catalog / INVITE']
backend_missing = [needle for needle in backend_needles if needle not in backend_chain_doc]
add(
    'Backend chain review doc exists and covers current main chains',
    not backend_missing,
    'missing=' + (', '.join(backend_missing) if backend_missing else 'none'),
)

# 11. scratch files and historical generated logs should not stay in source tree
stale_paths = [
    ROOT / 'build-gateway-error.txt',
    ROOT / 'strict-acceptance-full.log',
    ROOT / 'gateway-server' / 'tmp_calc.go',
]
stale_hits = [str(path.relative_to(ROOT)) for path in stale_paths if path.exists()]
add(
    'Source tree does not keep scratch executables or generated error logs as facts',
    not stale_hits,
    'hits=' + (', '.join(stale_hits) if stale_hits else 'none'),
)

# 12. known stale backend helpers should not remain in active source
active_go = '\n'.join(
    text(path)
    for path in [
        ROOT / 'gateway-server' / 'internal' / 'server' / 'build_identity.go',
        ROOT / 'gateway-server' / 'internal' / 'server' / 'response_mode_policy.go',
        ROOT / 'gateway-server' / 'internal' / 'server' / 'runtime_support.go',
        ROOT / 'gateway-server' / 'internal' / 'server' / 'transport_tuning.go',
        ROOT / 'gateway-server' / 'internal' / 'server' / 'gb28181_tunnel.go',
        ROOT / 'gateway-server' / 'internal' / 'server' / 'mapping_forward_transport.go',
    ]
)
stale_helper_needles = [
    'func currentBuildIdentity(',
    'func isResponseLikelyStreaming(',
    'func (s *accessLogStore) Aggregate(',
    'func genericDownloadSegmentRetries(',
    'func inlineResponseHeadroomPercent(',
    'func advertisedSIPCallback(local nodeconfig.LocalNodeConfig)',
    'func cloneTransportForTests(',
    'func dialMappingTransportForTests(',
]
stale_helper_hits = [needle for needle in stale_helper_needles if needle in active_go]
add(
    'Known stale backend helpers have been removed from active source',
    not stale_helper_hits,
    'hits=' + (', '.join(stale_helper_hits) if stale_helper_hits else 'none'),
)

# 13. design/guardrails pin startup active strategy snapshot and shared failure dictionary
strategy_snapshot_text = text(ROOT / 'docs' / 'design.md') + '\n' + text(ROOT / 'docs' / 'ENGINEERING_GUARDRAILS.md')
strategy_snapshot_needles = [
    'active_strategy_snapshot',
    'large_response_delivery_family',
    'segmented_profile_selector',
    '失败原因字典',
]
strategy_snapshot_missing = [needle for needle in strategy_snapshot_needles if needle not in strategy_snapshot_text]
add(
    'Active docs pin startup strategy snapshot and shared failure dictionary',
    not strategy_snapshot_missing,
    'missing=' + (', '.join(strategy_snapshot_missing) if strategy_snapshot_missing else 'none'),
)

# 14. startup/media chains use shared addr-in-use helper from the split entry/media files
shared_addr_hits = []
for path in [
    ROOT / 'gateway-server' / 'cmd' / 'gateway' / 'main_startup.go',
    ROOT / 'gateway-server' / 'internal' / 'server' / 'gb28181_media_sender.go',
    ROOT / 'gateway-server' / 'internal' / 'server' / 'gb28181_media_receiver.go',
]:
    body = text(path)
    if 'IsAddrInUseError(' not in body:
        shared_addr_hits.append(str(path.relative_to(ROOT)))
local_dup_hits = []
for path in [
    ROOT / 'gateway-server' / 'cmd' / 'gateway' / 'main_startup.go',
    ROOT / 'gateway-server' / 'internal' / 'server' / 'gb28181_media_sender.go',
    ROOT / 'gateway-server' / 'internal' / 'server' / 'gb28181_media_receiver.go',
]:
    if 'func isAddrInUseError(' in text(path):
        local_dup_hits.append(str(path.relative_to(ROOT)))
add(
    'Startup/media split files share addr-in-use helper and do not keep local duplicates',
    not shared_addr_hits and not local_dup_hits,
    'shared_missing=' + (', '.join(shared_addr_hits) if shared_addr_hits else 'none') + '; local_duplicates=' + (', '.join(local_dup_hits) if local_dup_hits else 'none'),
)

# 15. active source uses shared transfer failure classifier rather than repeated per-module literals
failure_go = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'failure_reason.go')
active_runtime = (
    text(ROOT / 'gateway-server' / 'internal' / 'server' / 'mapping_runtime_delivery.go')
    + '\n'
    + text(ROOT / 'gateway-server' / 'internal' / 'server' / 'device_backoff_policy.go')
)
add(
    'Active server code uses shared transfer failure classifier',
    'func classifyCommonTransferFailure(' in failure_go
    and 'func normalizeFailureReason(' in failure_go
    and 'classifyCommonTransferFailure(err)' in active_runtime,
    'Expect shared failure_reason.go and active delivery/runtime modules to call it.',
)

# 16. shared network error lexicon is centralized and reused across runtime/loadtest
netdiag_text = text(ROOT / 'gateway-server' / 'internal' / 'netdiag' / 'error_text.go')
loadtest_text = text(ROOT / 'gateway-server' / 'loadtest' / 'loadtest_validate.go')
upstream_text = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'upstream_resilience.go')
add(
    'Shared network error lexicon is centralized and reused',
    'func LooksLikeTimeoutText(' in netdiag_text
    and 'func LooksLikeConnectionClosedText(' in netdiag_text
    and 'netdiag.IsTimeoutError(err)' in loadtest_text
    and 'netdiag.LooksLikeConnectionClosedText(lowered)' in upstream_text,
    'Expect timeout/reset/refused matching to live in internal/netdiag and be reused by runtime/loadtest.',
)

# 16b. strict acceptance must sync go.mod/go.sum before targeted server tests on both shell and PowerShell
acceptance_ps1 = text(ROOT / 'scripts' / 'acceptance' / 'run_phase1_strict_acceptance.ps1')
acceptance_sh = text(ROOT / 'scripts' / 'acceptance' / 'run_phase1_strict_acceptance.sh')
mod_sync_ok = (
    'Sync-GoModuleGraph' in acceptance_ps1
    and 'go mod tidy' in acceptance_ps1
    and "Invoke-Step 'server_targeted'" in acceptance_ps1
    and 'go mod tidy' in acceptance_sh
    and 'run_step server_targeted' in acceptance_sh
)
add(
    'Strict acceptance syncs go.mod/go.sum before targeted server tests in both shell and PowerShell',
    mod_sync_ok,
    'Expect both acceptance entrypoints to run go mod tidy before server_targeted so release builds do not fail on missing go.sum.',
)

verify_ps1 = text(ROOT / 'scripts' / 'acceptance' / 'verify_phase1_strict_source.ps1')
ps1_order_needles = [
    ('$startupTransportSummaryText =', '$startupHelperOk ='),
    ('$systemSettingsBackendText =', '$opsProfileMissing ='),
    ('$systemSettingsUiText =', '$opsProfileMissing ='),
    ('$resourceUsageBackendText =', '$resourceMissing ='),
    ('$resourceUsageUiText =', '$resourceMissing ='),
]
ps1_order_failures = []
for decl, use in ps1_order_needles:
    decl_idx = verify_ps1.find(decl)
    use_idx = verify_ps1.find(use)
    if decl_idx < 0 or use_idx < 0 or decl_idx > use_idx:
        ps1_order_failures.append(f'{decl} -> {use}')
add(
    'PowerShell strict source acceptance declares shared text facts before consuming them',
    not ps1_order_failures,
    'order_failures=' + (', '.join(ps1_order_failures) if ps1_order_failures else 'none'),
)

# 17. oversized backend http.go stays split by responsibilities
http_main = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'http.go')
add(
    'Backend handlers stay split out of http.go by responsibility',
    'func (d *handlerDeps) handleTasks(' not in http_main
    and 'func (d *handlerDeps) handleSelfCheck(' not in http_main
    and 'func (d *handlerDeps) handleNodes(' not in http_main
    and (ROOT / 'gateway-server' / 'internal' / 'server' / 'http_ops_tasks_routes.go').exists()
    and (ROOT / 'gateway-server' / 'internal' / 'server' / 'http_nodes.go').exists()
    and (ROOT / 'gateway-server' / 'internal' / 'server' / 'http_status.go').exists(),
    'Expect task/node/status handlers to live in dedicated files so http.go stays as shared scaffolding.',
)


# 18. oversized backend control files stay decomposed after large-step cleanup
http_split_text = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'http.go')
ops_split_text = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'ops_settings_logs.go')
http_split_ok = all(needle not in http_split_text for needle in [
    'func (d *handlerDeps) handleLinkTest(',
    'func (d *handlerDeps) handleMappings(',
    'func (d *handlerDeps) handleTunnelConfig(',
    'func (d *handlerDeps) handlePeers(',
    'func (d *handlerDeps) handleAudits(',
]) and all((ROOT / 'gateway-server' / 'internal' / 'server' / name).exists() for name in [
    'http_linktest.go',
    'http_mappings.go',
    'http_tunnel_ops.go',
    'http_audits.go',
])
add(
    'Large-step backend HTTP cleanup keeps link/mapping/tunnel/audit handlers out of http.go',
    http_split_ok,
    'Expect http.go to remain scaffold-only and dedicated handler files to exist.',
)

ops_split_ok = all(needle not in ops_split_text for needle in [
    'func (d *handlerDeps) handleSystemSettings(',
    'func (d *handlerDeps) handleDashboardSummary(',
    'func (d *handlerDeps) handleProtectionState(',
    'func (d *handlerDeps) handleNodeTunnelWorkspace(',
    'func (d *handlerDeps) handleLoadtests(',
]) and all((ROOT / 'gateway-server' / 'internal' / 'server' / name).exists() for name in [
    'ops_system_settings_http.go',
    'ops_dashboard_http.go',
    'ops_protection_security_http.go',
    'ops_workspace_loadtest_http.go',
])
add(
    'Large-step ops cleanup keeps settings/dashboard/protection/workspace handlers decomposed',
    ops_split_ok,
    'Expect ops_settings_logs.go to keep models only and dedicated ops handler files to exist.',
)


# 19. mapping runtime delivery logic stays split out of manager/runtime shell
mapping_runtime_main = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'mapping_runtime.go')
mapping_runtime_delivery = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'mapping_runtime_delivery.go')
add(
    'Mapping runtime keeps delivery/resume algorithms out of mapping_runtime.go',
    'func copyForwardResponseWithResume(' not in mapping_runtime_main
    and 'func buildFixedWindowPlan(' not in mapping_runtime_main
    and 'func copyForwardResponseWithResume(' in mapping_runtime_delivery
    and 'func buildFixedWindowPlan(' in mapping_runtime_delivery,
    'Expect mapping_runtime.go to stay focused on runtime shell and dedicated delivery file to own range/resume logic.',
)

# 20. gb28181 tunnel control flow keeps low-level dialog/transport helpers split
gb_main = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'gb28181_tunnel.go')
gb_dialog = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'gb28181_dialog_helpers.go')
gb_transport = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'gb28181_transport_helpers.go')
add(
    'GB28181 tunnel keeps dialog and transport helpers out of the main control-flow file',
    'func buildDeviceURI(' not in gb_main
    and 'func dynamicRelayBodyWait(' not in gb_main
    and 'func buildDeviceURI(' in gb_dialog
    and 'func dynamicRelayBodyWait(' in gb_transport,
    'Expect gb28181_tunnel.go to keep orchestration only while helper files own dialog/header/transport utilities.',
)

# 21. gateway main entry stays split into startup/config/sip-udp responsibilities
main_entry = text(ROOT / 'gateway-server' / 'cmd' / 'gateway' / 'main.go')
main_startup = text(ROOT / 'gateway-server' / 'cmd' / 'gateway' / 'main_startup.go')
main_config = text(ROOT / 'gateway-server' / 'cmd' / 'gateway' / 'main_config.go')
main_sip_udp = text(ROOT / 'gateway-server' / 'cmd' / 'gateway' / 'main_sip_udp.go')
add(
    'Gateway main entry keeps startup/config/SIP-UDP responsibilities decomposed',
    'func runGatewayStartup(' not in main_entry
    and 'func handleConfigCommands(' not in main_entry
    and 'func startSIPUDPServer(' not in main_entry
    and 'func runGatewayStartup(' in main_startup
    and 'func handleConfigCommands(' in main_config
    and 'func startSIPUDPServer(' in main_sip_udp,
    'Expect main.go to remain a thin shell while startup/config/SIP-UDP helpers stay in dedicated files.',
)

# 22. GB28181 media file keeps codec/sender/receiver responsibilities decomposed
gb_media_main = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'gb28181_media.go')
gb_media_ps = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'gb28181_media_ps_codec.go')
gb_media_sender = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'gb28181_media_sender.go')
gb_media_receiver = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'gb28181_media_receiver.go')
add(
    'GB28181 media keeps PS codec, sender, and receiver responsibilities split',
    'func newRTPBodySender(' not in gb_media_main
    and 'func newRTPBodyReceiver(' not in gb_media_main
    and 'func decodeRTPPacket(' not in gb_media_main
    and 'func newRTPBodySender(' in gb_media_sender
    and 'func newRTPBodyReceiver(' in gb_media_receiver
    and 'func decodeRTPPacket(' in gb_media_ps,
    'Expect gb28181_media.go to stay thin and dedicated media helper files to exist.',
)

# 23. loadtest keeps runtime loop, diagnostics, operations, and validation separated
loadtest_main = text(ROOT / 'gateway-server' / 'loadtest' / 'loadtest.go')
loadtest_diag = text(ROOT / 'gateway-server' / 'loadtest' / 'loadtest_diagnostics.go')
loadtest_ops = text(ROOT / 'gateway-server' / 'loadtest' / 'loadtest_ops.go')
loadtest_validate = text(ROOT / 'gateway-server' / 'loadtest' / 'loadtest_validate.go')
add(
    'Loadtest keeps run loop, diagnostics, operations, and validation separated',
    'func newDiagnosticsCollector(' not in loadtest_main
    and 'func buildOperations(' not in loadtest_main
    and 'func validateConfig(' not in loadtest_main
    and 'func newDiagnosticsCollector(' in loadtest_diag
    and 'func buildOperations(' in loadtest_ops
    and 'func validateConfig(' in loadtest_validate,
    'Expect loadtest.go to stay focused on models and Run while helper groups stay in dedicated files.',
)

# 25. shared bind-address helper should stay centralized across config/selfcheck/repository
netbind_text = text(ROOT / 'gateway-server' / 'internal' / 'netbind' / 'bind.go')
local_bind_dup_hits = []
for path in [
    ROOT / 'gateway-server' / 'internal' / 'config' / 'network_validate.go',
    ROOT / 'gateway-server' / 'internal' / 'selfcheck' / 'selfcheck.go',
    ROOT / 'gateway-server' / 'internal' / 'selfcheck' / 'selfcheck_ports.go',
    ROOT / 'gateway-server' / 'internal' / 'selfcheck' / 'selfcheck_checks.go',
    ROOT / 'gateway-server' / 'internal' / 'repository' / 'file' / 'tunnel_mapping_store.go',
]:
    if 'func sameBindAddress(' in text(path):
        local_bind_dup_hits.append(str(path.relative_to(ROOT)))
add(
    'Bind-address overlap logic is centralized in internal/netbind',
    'func SameBindAddress(' in netbind_text
    and 'netbind.SameBindAddress(' in text(ROOT / 'gateway-server' / 'internal' / 'config' / 'network_validate.go')
    and 'netbind.SameBindAddress(' in text(ROOT / 'gateway-server' / 'internal' / 'selfcheck' / 'selfcheck_checks.go')
    and 'netbind.SameBindAddress(' in text(ROOT / 'gateway-server' / 'internal' / 'repository' / 'file' / 'tunnel_mapping_store.go')
    and not local_bind_dup_hits,
    'local_duplicates=' + (', '.join(local_bind_dup_hits) if local_bind_dup_hits else 'none'),
)

# 26. network config remains decomposed into model/defaults/validation files
network_model = text(ROOT / 'gateway-server' / 'internal' / 'config' / 'network.go')
network_defaults = text(ROOT / 'gateway-server' / 'internal' / 'config' / 'network_defaults.go')
network_validate = text(ROOT / 'gateway-server' / 'internal' / 'config' / 'network_validate.go')
network_validate_endpoints = text(ROOT / 'gateway-server' / 'internal' / 'config' / 'network_validate_endpoints.go')
transport_validate = text(ROOT / 'gateway-server' / 'internal' / 'config' / 'transport_tuning_validate.go')
add(
    'Network config keeps models/defaults/validation split',
    'func DefaultTransportTuningConfig(' not in network_model
    and 'func (c NetworkConfig) Validate(' not in network_model
    and 'func ParseNetworkConfigYAML(' in network_defaults
    and 'func (c *NetworkConfig) ApplyDefaults(' in network_defaults
    and 'func validatePortConflict(' in network_validate
    and 'func (c SIPConfig) Validate(' in network_validate_endpoints
    and 'func (c RTPConfig) Validate(' in network_validate_endpoints
    and 'func (c TransportTuningConfig) Validate(' in transport_validate,
    'Expect network.go to keep models only and dedicated defaults/validate files to exist.',
)

# 27. selfcheck remains decomposed into runner/ports/checks files
selfcheck_main = text(ROOT / 'gateway-server' / 'internal' / 'selfcheck' / 'selfcheck.go')
selfcheck_ports = text(ROOT / 'gateway-server' / 'internal' / 'selfcheck' / 'selfcheck_ports.go')
selfcheck_checks = text(ROOT / 'gateway-server' / 'internal' / 'selfcheck' / 'selfcheck_checks.go')
add(
    'Selfcheck keeps runner shell, port diagnostics, and checks split',
    'func (r *Runner) checkListenIP(' not in selfcheck_main
    and 'func (r *Runner) checkSIPPortOccupancy(' not in selfcheck_main
    and 'func (r *Runner) checkListenIP(' in selfcheck_ports
    and 'func (r *Runner) checkDownstreamReachability(' in selfcheck_checks
    and 'func (r *Runner) Run(' in selfcheck_main,
    'Expect selfcheck.go to stay focused on Runner/Report while dedicated files own concrete checks.',
)

# 28. backend HTTP tests stay split by domains instead of a single monolith file
http_test_monolith = ROOT / 'gateway-server' / 'internal' / 'server' / 'http_test.go'
add(
    'Backend HTTP tests stay split by support/mappings/nodes/ops domains',
    not http_test_monolith.exists()
    and (ROOT / 'gateway-server' / 'internal' / 'server' / 'http_test_support_test.go').exists()
    and (ROOT / 'gateway-server' / 'internal' / 'server' / 'http_mappings_test.go').exists()
    and (ROOT / 'gateway-server' / 'internal' / 'server' / 'http_nodes_test.go').exists()
    and (ROOT / 'gateway-server' / 'internal' / 'server' / 'http_ops_test.go').exists(),
    'Expect no single http_test.go monolith and dedicated domain test files to exist.',
)

# 29. runtime support keeps access log/loadtest/security/settings split out of the boundary file
runtime_support_main = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'runtime_support.go')
access_log_store = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'access_log_store.go')
runtime_support_settings = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'runtime_support_settings.go')
runtime_support_loadtest = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'runtime_support_loadtest.go')
runtime_support_security = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'runtime_support_security.go')
add(
    'Runtime support keeps access logs, settings, loadtest jobs, and security events split',
    'type accessLogStore struct' not in runtime_support_main
    and 'type loadtestJob struct' not in runtime_support_main
    and 'type securityEventStore struct' not in runtime_support_main
    and 'type protectionSettings struct' not in runtime_support_main
    and 'type accessLogStore struct' in access_log_store
    and 'type loadtestJob struct' in runtime_support_loadtest
    and 'type securityEventStore struct' in runtime_support_security
    and 'type protectionSettings struct' in runtime_support_settings,
    'Expect runtime_support.go to stay thin while dedicated runtime support files own concrete facts.',
)

# 30. access-log sampling test follows the access-log fact source instead of runtime_support naming
add(
    'Access-log sampling test follows access_log_store naming',
    not (ROOT / 'gateway-server' / 'internal' / 'server' / 'runtime_support_sampling_test.go').exists()
    and (ROOT / 'gateway-server' / 'internal' / 'server' / 'access_log_store_sampling_test.go').exists(),
    'Expect access-log sampling tests to live beside access_log_store instead of runtime_support naming.',
)

# 31. guardrails/review docs record runtime support split as an active engineering rule
runtime_docs_text = text(ROOT / 'docs' / 'ENGINEERING_GUARDRAILS.md') + '\n' + text(ROOT / 'docs' / 'OVERALL_CLEANUP_REVIEW_20260321.md')
runtime_doc_needles = ['运行态支撑域必须按事实源拆分', 'access_log_store.go', 'runtime_support_loadtest.go', 'runtime_support_security.go']
runtime_doc_missing = [needle for needle in runtime_doc_needles if needle not in runtime_docs_text]
add(
    'Docs record runtime-support split and active fact-source files',
    not runtime_doc_missing,
    'missing=' + (', '.join(runtime_doc_missing) if runtime_doc_missing else 'none'),
)

# 32. source/build/publish UI delivery chain stays aligned
ui_main = text(ROOT / 'gateway-ui' / 'src' / 'utils' / 'uiBase.ts')
ui_vite = text(ROOT / 'gateway-ui' / 'vite.config.ts')
ui_dist = text(ROOT / 'gateway-ui' / 'dist' / 'index.html')
ui_embedded = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'embedded-ui' / 'index.html')
ui_meta_exists = (ROOT / 'gateway-server' / 'internal' / 'server' / 'embedded-ui' / '.siptunnel-ui-embed.json').exists()
add(
    'UI source/build/publish chain keeps base-path alignment signals',
    'meta[name="siptunnel-ui-base-path"]' in ui_main
    and "base: './'" in ui_vite
    and 'meta name="siptunnel-ui-base-path"' in ui_embedded
    and './assets/' in ui_embedded
    and (not ui_dist or './assets/' in ui_dist)
    and ui_meta_exists,
    f"dist_present={'yes' if ui_dist else 'no'} metadata={'yes' if ui_meta_exists else 'no'}",
)


# 33. ui delivery guard exists and is wired into build/embed chain
ui_guard = text(ROOT / 'scripts' / 'ui-delivery-guard.mjs')
ui_build_ps1 = text(ROOT / 'scripts' / 'ui-build.ps1')
ui_build_sh = text(ROOT / 'scripts' / 'ui-build.sh')
embed_ui_ps1 = text(ROOT / 'scripts' / 'embed-ui.ps1')
add(
    'UI delivery guard exists and is wired into build/embed scripts',
    'legacyFiles' in ui_guard
    and 'ui-delivery-guard.mjs' in ui_build_ps1
    and 'ui-delivery-guard.mjs' in ui_build_sh
    and 'delivery_guard_status' in embed_ui_ps1,
    'Expect stale legacy UI drift to be checked before build and recorded during embed.',
)

# 34. windows strict acceptance should run targeted server compile/test before full release build
phase1_accept_ps1 = text(ROOT / 'scripts' / 'acceptance' / 'run_phase1_strict_acceptance.ps1')
add(
    'Windows strict acceptance runs server targeted go test before build release',
    "Invoke-Step 'server_targeted'" in phase1_accept_ps1
    and 'go test ./internal/protocol/... ./internal/server ./internal/selfcheck ./internal/config -count=1' in phase1_accept_ps1
    and phase1_accept_ps1.find("Invoke-Step 'server_targeted'") < phase1_accept_ps1.find("Invoke-Step 'build_release'"),
    'Expect PowerShell strict acceptance to fail fast on backend compile/test drift before full release packaging.',
)

# 35. cleared legacy UI source files do not remain in source tree
legacy_ui_paths = [
    ROOT / 'gateway-ui' / 'src' / 'views' / 'ConfigGovernanceView.vue',
    ROOT / 'gateway-ui' / 'src' / 'views' / 'ConfigTransferView.vue',
    ROOT / 'gateway-ui' / 'src' / 'views' / 'NetworkConfigView.vue',
    ROOT / 'gateway-ui' / 'src' / 'views' / 'NodeStatusView.vue',
    ROOT / 'gateway-ui' / 'src' / 'views' / '__tests__' / 'NodeStatusView.spec.ts',
    ROOT / 'gateway-ui' / 'src' / 'api' / '__tests__' / 'gatewayConfigTransfer.spec.ts',
]
legacy_ui_hits = [str(path.relative_to(ROOT)) for path in legacy_ui_paths if path.exists()]
add(
    'Cleared legacy UI files do not remain in source tree',
    not legacy_ui_hits,
    'hits=' + (', '.join(legacy_ui_hits) if legacy_ui_hits else 'none'),
)

# 36. slim source packaging script exists
add(
    'Slim source packaging script exists',
    (ROOT / 'scripts' / 'package-source.sh').exists(),
    'Expect scripts/package-source.sh to create a source-only zip without node_modules/dist/artifacts.',
)

# 37. source root should not keep generated acceptance or delivery reports
root_generated_reports = [
    path.name
    for path in ROOT.glob('*.md')
    if '验收报告' in path.name or '决策落地' in path.name or '复核报告' in path.name
]
add(
    'Source root does not keep generated delivery or acceptance reports',
    not root_generated_reports,
    'hits=' + (', '.join(root_generated_reports) if root_generated_reports else 'none'),
)

# 38. startup transport summary helper centralizes converged generic profile usage
startup_main = text(ROOT / 'gateway-server' / 'cmd' / 'gateway' / 'main_startup.go')
startup_helper = text(ROOT / 'gateway-server' / 'cmd' / 'gateway' / 'startup_transport_summary.go')
add(
    'Startup transport summary centralizes converged generic profile usage',
    'convergedGeneric' not in startup_main
    and 'responseModePolicy :=' not in startup_main
    and 'func effectiveTransportTuningSummary(' in startup_helper
    and 'func logAppliedTransportTuning(' in startup_helper
    and startup_helper.count('convergedGeneric := config.ConvergedGenericDownloadProfile(') >= 2,
    'Expect converged generic profile locals to live only in startup_transport_summary.go so main_startup.go does not drift into compile-only helper references or stale startup-only aliases.',
)

# 39. system settings human-unit ops profile is wired across backend payload, UI API/types, and view
system_settings_backend = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'http.go') + '\n' + text(ROOT / 'gateway-server' / 'internal' / 'server' / 'ops_system_settings_http.go')
system_settings_ui = text(ROOT / 'gateway-ui' / 'src' / 'types' / 'gateway.ts') + '\n' + text(ROOT / 'gateway-ui' / 'src' / 'api' / 'gateway.ts') + '\n' + text(ROOT / 'gateway-ui' / 'src' / 'views' / 'SystemSettingsView.vue')
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
add(
    'System settings expose human-unit ops profile consistently across backend/UI',
    not ops_profile_missing,
    'missing=' + (', '.join(ops_profile_missing) if ops_profile_missing else 'none'),
)

# 40. resource usage endpoint and UI views expose runtime tuning/resource facts with the same human-unit fields
resource_backend = text(ROOT / 'gateway-server' / 'internal' / 'server' / 'ops_resource_usage_http.go') + '\n' + text(ROOT / 'gateway-server' / 'internal' / 'server' / 'ops_settings_logs.go')
resource_ui = text(ROOT / 'gateway-ui' / 'src' / 'types' / 'gateway.ts') + '\n' + text(ROOT / 'gateway-ui' / 'src' / 'views' / 'SystemSettingsView.vue') + '\n' + text(ROOT / 'gateway-ui' / 'src' / 'views' / 'AlertsAndRateLimitView.vue')
resource_needles = [
    'configured_generic_download_mbps',
    'configured_generic_per_transfer_mbps',
    'configured_adaptive_hot_cache_mb',
    'configured_adaptive_hot_window_mb',
    'configured_generic_download_window_mb',
    'configured_generic_segment_concurrency',
    'configured_generic_rtp_reorder_window_packets',
    'configured_generic_rtp_loss_tolerance_packets',
    'configured_generic_rtp_gap_timeout_ms',
    'configured_generic_rtp_fec_enabled',
    'configured_generic_rtp_fec_group_packets',
]
resource_missing = [needle for needle in resource_needles if needle not in resource_backend or needle not in resource_ui]
add(
    'Runtime resource/tuning facts stay aligned across backend endpoint and ops UI',
    not resource_missing,
    'missing=' + (', '.join(resource_missing) if resource_missing else 'none'),
)

overall = all(check['passed'] for check in checks)
report = {
    'generated_at': datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ'),
    'overall': 'PASS' if overall else 'FAIL',
    'summary': {
        'total': len(checks),
        'pass': sum(check['passed'] for check in checks),
        'fail': sum(not check['passed'] for check in checks),
    },
    'checks': checks,
}
json_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
lines = [
    '# 工程一致性检查报告',
    '',
    f"- GeneratedAt(Local): `{report['generated_at']}`",
    f"- Overall: **{report['overall']}**",
    '',
    '| 检查项 | 结果 | 说明 |',
    '|---|---|---|',
]
for check in checks:
    detail = check['detail'].replace('|', '\\|')
    lines.append(f"| {check['name']} | {'PASS' if check['passed'] else 'FAIL'} | {detail} |")
md_path.write_text('\n'.join(lines), encoding='utf-8')
print(md_path)
print(json_path)
if not overall:
    sys.exit(1)
