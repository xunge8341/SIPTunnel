# GB28181 第一阶段（严格模式）验收脚本与抓包回归清单

日期：2026-03-18
范围：**第一阶段**，仅面向**严格模式**。

本清单用于把“源码整改完成”转化为“可执行验收、可抓包证明、可签核结论”。

---

## 1. 验收目标

阶段一必须证明以下目标均已达成：

1. 控制面只保留 **`MESSAGE + Application/MANSCDP+xml`** 主路径。
2. **不再使用 `INFO(HttpInvoke/HttpResponse*)` 作为线上主路径**。
3. `Catalog SUBSCRIBE` 已完整化，具备 `Event / Accept / Content-Type / body`。
4. `REGISTER 200 OK` 的 `Date` 头符合 GB/T 28181 时间格式，并按网关本地时区输出。
5. `INVITE/SDP` 已收口为国标点播体例：
   - `m=video`
   - `a=rtpmap:96 PS/90000`
   - `a=recvonly / a=sendonly`
6. `Allow`、`Subject`、`Content-Type` 等关键头域完整。
7. secret 已改为只写化，UI 与读接口不回显密码明文。
8. 文档、UI、mock/test 与当前实现保持一致。

---

## 2. 建议执行顺序

### 2.1 源码静态验收

```bash
./scripts/acceptance/verify_phase1_strict_source.sh
```

输出：

- `artifacts/acceptance/phase1-strict-source-*.md`
- `artifacts/acceptance/phase1-strict-source-*.json`

用途：

- 检查阶段一目标是否在源码层面闭合
- 强制阻断遗留私有 `INFO(HttpInvoke/HttpResponse*)` 主路径
- 检查 UI / mock / 后端 / 当前文档是否同步

### 2.2 一键验收编排

```bash
./scripts/acceptance/run_phase1_strict_acceptance.sh
```

该脚本按顺序执行：

1. 阶段一源码静态验收
2. UI build guard
3. 后端定向测试
4. smoke
5. regression smoke

输出：

- `artifacts/acceptance/phase1-strict-acceptance-*.md`
- `artifacts/acceptance/phase1-strict-acceptance-*.json`

可选跳过项：

```bash
ACCEPTANCE_SKIP_UI=true \
ACCEPTANCE_SKIP_SMOKE=true \
./scripts/acceptance/run_phase1_strict_acceptance.sh
```

---

## 3. 抓包回归步骤

### 3.1 开始抓包

```bash
PCAP_IFACE=eth0 \
PCAP_SIP_PORT=5060 \
PCAP_RTP_START=30000 \
PCAP_RTP_END=30101 \
PCAP_PEER_IP=10.20.1.60 \
./scripts/acceptance/pcap_capture.sh start
```

默认抓取：

- SIP 端口（TCP/UDP）
- RTP 端口范围（UDP）

### 3.2 执行业务场景

至少覆盖以下 4 类业务场景：

1. **注册场景**：`REGISTER -> 200 OK`
2. **目录订阅场景**：`SUBSCRIBE -> 200 OK -> NOTIFY -> 200 OK`
3. **小载荷控制场景**：`MESSAGE` 承载请求/响应元数据或 inline 响应
4. **大载荷场景**：`INVITE -> 200 OK -> ACK -> RTP -> BYE`

### 3.3 停止抓包

```bash
./scripts/acceptance/pcap_capture.sh stop
```

### 3.4 校验抓包

```bash
./scripts/acceptance/verify_gb28181_pcap.sh /path/to/phase1-strict-*.pcap
```

输出：

- `artifacts/pcap/<pcap-name>.*.verify.md`
- `artifacts/pcap/<pcap-name>.*.verify.json`

---

## 4. 抓包回归核对点

### 4.1 REGISTER

必须看到：

- `REGISTER`
- `200 OK`
- `Date: yyyy-MM-ddTHH:mm:ss.SSS`（网关本地时区的墙上时间，不使用 UTC 直出）

- UI 与日志展示时间统一按网关主机本地时区的墙上时间展示；对 RFC3339/Z 时间再做本地转换。

不通过判据：

- 无 `REGISTER 200 OK`
- `Date` 仍为 RFC1123 风格

### 4.2 Catalog SUBSCRIBE

必须看到：

- `SUBSCRIBE`
- `Event: Catalog`
- `Accept: Application/MANSCDP+xml`
- `Content-Type: Application/MANSCDP+xml`
- body 中存在 MANSCDP XML
- 若场景包含目录变化，必须看到 `NOTIFY`

不通过判据：

- 仍为空订阅
- 缺 `Content-Type`
- 无 XML body

### 4.3 MESSAGE 控制面

必须看到：

- `MESSAGE`
- `Content-Type: Application/MANSCDP+xml`
- body 为 MANSCDP XML

不通过判据：

- 主路径仍出现 `INFO`
- MESSAGE 没有 MANSCDP Content-Type

### 4.4 INVITE / SDP / RTP

必须看到：

- `INVITE`
- `Allow`
- `Subject`
- `Content-Type: application/sdp`
- SDP 中包含：
  - `m=video`
  - `a=rtpmap:96 PS/90000`
  - `a=recvonly` 或 `a=sendonly`
- `200 OK`
- `ACK`
- RTP/UDP 媒体流
- `BYE`

不通过判据：

- SDP 仍为 `m=application`
- 未使用 `PS/90000`
- 未出现 ACK/BYE
- 媒体流未建立或端口不在预期范围

---

## 5. 人工签核项

### 5.1 源码与配置

- [ ] 当前分支已移除线上 `INFO(HttpInvoke/HttpResponse*)` 主路径
- [ ] UI 不回显 `register_auth_password` 明文
- [ ] mock/test 已同步到严格模式
- [ ] README / 当前设计文档不再把私有 `INFO` 描述为主方案

### 5.2 运行态与接口

- [ ] `/healthz` 正常
- [ ] `/readyz` 正常
- [ ] `/api/selfcheck` 无阶段一阻断项
- [ ] `smoke` 通过
- [ ] `regression smoke` 通过

### 5.3 抓包与互通

- [ ] REGISTER 抓包通过
- [ ] SUBSCRIBE/NOTIFY 抓包通过
- [ ] MESSAGE 控制面抓包通过
- [ ] INVITE/SDP/RTP/BYE 抓包通过
- [ ] 未观察到 INFO 作为线上主路径

---

## 6. 结果归档要求

建议每次验收至少归档以下文件：

- `artifacts/acceptance/*.md`
- `artifacts/acceptance/*.json`
- `artifacts/pcap/*.pcap`
- `artifacts/pcap/*.verify.md`
- `artifacts/pcap/*.verify.json`
- 当次使用的配置文件副本
- 现场网元版本、对端平台版本、抓包接口信息

---

## 7. 结论口径

只有同时满足以下条件，才能判定**阶段一验收通过**：

1. 源码静态验收通过
2. 一键验收编排通过
3. 抓包回归通过
4. 人工签核项全部完成

如果源码验收通过，但抓包未通过，则结论只能是：

> 阶段一整改在源码层面完成，但**尚未完成互通与抓包验收**。


## Windows execution note

Run this once before acceptance to avoid repeated security prompts:

```powershell
Get-ChildItem .\scripts\*.ps1, .\scripts\acceptance\*.ps1 | Unblock-File
```

Recommended order:

```powershell
.\scripts\acceptance\verify_phase1_strict_source.ps1
.\scripts\acceptance\run_phase1_strict_acceptance.ps1 -Mode native -UiPolicy delivery
```

## Windows toolchain and runtime note

- The repository target remains `go 1.23.x` compatibility in `go.mod`.
- On **Windows + Go 1.26.x**, public upstream issues report `net/http.(*connReader)` access violations during socket teardown.
- This repository now enables a conservative mitigation on Windows Go 1.26.x:
  - disable local HTTP keep-alives for mapping runtime and gateway HTTP listeners
  - force `Connection: close` on mapping runtime responses
  - close and clear inbound request bodies after prepare
- For protocol regression and pcap sign-off on Windows, prefer one of:
  - Go 1.25.x stable patch release
  - the project compatibility baseline (Go 1.23.x)


## 运行期补充核对（2026-03-18）

- RTP 媒体包应为 **标准 RTP 头**（Version=2，PT=96），不应再出现自定义 `STP1` 负载前缀。
- `INVITE 200 OK` 的 SDP 中 `m=video` 端口应为实际发送端口，不能为 `0`。
- `NOTIFY(Event: Catalog)` 应与对应 `SUBSCRIBE` 处于同一对话，复用 `Call-ID` 并保持 `From/To` tag 关系一致。
