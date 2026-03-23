#!/usr/bin/env bash
set -euo pipefail

PCAP_FILE="${1:-}"
if [[ -z "$PCAP_FILE" ]]; then
  echo "Usage: $0 <pcap-file>" >&2
  exit 2
fi
if [[ ! -f "$PCAP_FILE" ]]; then
  echo "pcap file not found: $PCAP_FILE" >&2
  exit 2
fi
if ! command -v tshark >/dev/null 2>&1; then
  echo "tshark is required" >&2
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPORT_DIR="${PCAP_REPORT_DIR:-$ROOT_DIR/artifacts/pcap}"
mkdir -p "$REPORT_DIR"
TS="$(date +%Y%m%d-%H%M%S)"
BASE="$(basename "$PCAP_FILE")"
REPORT_JSON="$REPORT_DIR/${BASE}.${TS}.verify.json"
REPORT_MD="$REPORT_DIR/${BASE}.${TS}.verify.md"

python3 - "$PCAP_FILE" "$REPORT_JSON" "$REPORT_MD" <<'PY'
from __future__ import annotations
import json, re, subprocess, sys
from pathlib import Path
from datetime import datetime, timezone

pcap = sys.argv[1]
report_json = Path(sys.argv[2])
report_md = Path(sys.argv[3])

def run(*args: str) -> str:
    out = subprocess.check_output(args, text=True, stderr=subprocess.DEVNULL)
    return out

def count(display_filter: str) -> int:
    out = run('tshark', '-r', pcap, '-Y', display_filter, '-T', 'fields', '-e', 'frame.number')
    return sum(1 for line in out.splitlines() if line.strip())

def verbose_first(display_filter: str) -> str:
    return run('tshark', '-r', pcap, '-Y', display_filter, '-V', '-c', '1')

checks = []

def add(name: str, passed: bool, detail: str):
    checks.append({'name': name, 'passed': passed, 'detail': detail})

register_req = count('sip.Method == "REGISTER"')
register_ok = count('sip.Status-Code == 200 && sip.CSeq.method == "REGISTER"')
subscribe_req = count('sip.Method == "SUBSCRIBE"')
message_req = count('sip.Method == "MESSAGE"')
info_req = count('sip.Method == "INFO"')
invite_req = count('sip.Method == "INVITE"')
invite_ok = count('sip.Status-Code == 200 && sip.CSeq.method == "INVITE"')
ack_req = count('sip.Method == "ACK"')
bye_req = count('sip.Method == "BYE"')
notify_req = count('sip.Method == "NOTIFY"')
rtp_count = count('rtp')

add('存在 REGISTER 请求', register_req > 0, f'count={register_req}')
add('存在 REGISTER 200 OK', register_ok > 0, f'count={register_ok}')
add('存在 SUBSCRIBE 请求', subscribe_req > 0, f'count={subscribe_req}')
add('存在 MESSAGE 请求', message_req > 0, f'count={message_req}')
add('不存在 INFO 请求', info_req == 0, f'count={info_req}')
add('存在 INVITE 请求', invite_req > 0, f'count={invite_req}')
add('存在 INVITE 200 OK', invite_ok > 0, f'count={invite_ok}')
add('存在 ACK', ack_req > 0, f'count={ack_req}')
add('存在 BYE', bye_req > 0, f'count={bye_req}')
add('目录链路存在 NOTIFY（如本次场景包含目录同步）', notify_req > 0, f'count={notify_req}')
add('存在 RTP 包（若抓包包含媒体面）', rtp_count > 0, f'count={rtp_count}')

# Header / SDP checks from verbose decode
if register_ok > 0:
    text = verbose_first('sip.Status-Code == 200 && sip.CSeq.method == "REGISTER"')
    add('REGISTER 200 OK 含国标 Date 格式', bool(re.search(r'Date:\s*\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}', text)), '检查 Date: yyyy-MM-ddTHH:mm:ss.SSS')
else:
    add('REGISTER 200 OK 含国标 Date 格式', False, '未抓到 REGISTER 200 OK')

if subscribe_req > 0:
    text = verbose_first('sip.Method == "SUBSCRIBE"')
    add('SUBSCRIBE 含 Event: Catalog', 'Event: Catalog' in text, '检查 Event 头')
    add('SUBSCRIBE 含 Accept: Application/MANSCDP+xml', 'Accept: Application/MANSCDP+xml' in text, '检查 Accept 头')
    add('SUBSCRIBE 含 Content-Type: Application/MANSCDP+xml', 'Content-Type: Application/MANSCDP+xml' in text, '检查 Content-Type 头')
else:
    add('SUBSCRIBE 含 Event: Catalog', False, '未抓到 SUBSCRIBE')
    add('SUBSCRIBE 含 Accept: Application/MANSCDP+xml', False, '未抓到 SUBSCRIBE')
    add('SUBSCRIBE 含 Content-Type: Application/MANSCDP+xml', False, '未抓到 SUBSCRIBE')

if message_req > 0:
    text = verbose_first('sip.Method == "MESSAGE"')
    add('MESSAGE 含 Content-Type: Application/MANSCDP+xml', 'Content-Type: Application/MANSCDP+xml' in text, '检查 MESSAGE Content-Type')
else:
    add('MESSAGE 含 Content-Type: Application/MANSCDP+xml', False, '未抓到 MESSAGE')

if invite_req > 0:
    text = verbose_first('sip.Method == "INVITE"')
    add('INVITE 含 Allow 头', 'Allow:' in text, '检查 Allow 头')
    add('INVITE 含 Subject 头', 'Subject:' in text, '检查 Subject 头')
    add('INVITE 含 Content-Type: application/sdp', 'Content-Type: application/sdp' in text, '检查 INVITE Content-Type')
    add('INVITE SDP 使用 m=video', 'm=video ' in text, '检查 SDP m=video')
    add('INVITE SDP 使用 PS/90000', 'a=rtpmap:96 PS/90000' in text, '检查 SDP rtpmap')
    add('INVITE SDP 具有单向属性', ('a=recvonly' in text) or ('a=sendonly' in text), '检查 a=recvonly/sendonly')
else:
    for name in [
        'INVITE 含 Allow 头', 'INVITE 含 Subject 头', 'INVITE 含 Content-Type: application/sdp',
        'INVITE SDP 使用 m=video', 'INVITE SDP 使用 PS/90000', 'INVITE SDP 具有单向属性'
    ]:
        add(name, False, '未抓到 INVITE')

if invite_ok > 0:
    text = verbose_first('sip.Status-Code == 200 && sip.CSeq.method == "INVITE"')
    add('INVITE 200 OK SDP 具有单向属性', ('a=recvonly' in text) or ('a=sendonly' in text), '检查 200 OK SDP')
else:
    add('INVITE 200 OK SDP 具有单向属性', False, '未抓到 INVITE 200 OK')

overall = all(item['passed'] for item in checks)
report = {
    'generated_at': datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ'),
    'pcap_file': pcap,
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
    '# GB28181 第一阶段抓包验收报告',
    '',
    f"- PCAP: `{pcap}`",
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
    '> 说明：若 RTP 未被 tshark 自动识别为 rtp，仍应结合端口范围与原始 UDP 包进行人工复核。',
]
report_md.write_text('\n'.join(lines), encoding='utf-8')
print(report_md)
print(report_json)
if not overall:
    raise SystemExit(1)
PY
