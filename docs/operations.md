# SIPTunnel 部署与操作手册

本文档面向一线运维与交付人员，提供可重复执行的安装、升级、回滚与排障流程。

## 1. 安装前检查（Precheck）

生产环境建议先执行统一预检查脚本，输出包含：
- `config validate`
- `env inspect`
- `storage check`
- `network check`

### 1.1 一次执行全部检查

```bash
./deploy/scripts/precheck.sh all
```

### 1.2 按检查项单独执行

```bash
./deploy/scripts/precheck.sh config validate
./deploy/scripts/precheck.sh env inspect
./deploy/scripts/precheck.sh storage check
./deploy/scripts/precheck.sh network check
```

### 1.3 检查项说明

- `config validate`：检查配置文件存在性、关键段落和基础字段完整性。
- `env inspect`：打印关键环境变量，方便变更单核对。
- `storage check`：检查安装目录和数据目录可创建、可写。
- `network check`：检查监听端口、媒体端口范围、节点角色、端口占用。

## 2. 安装步骤（Install）

> 适用：首次部署或重建节点。脚本默认幂等，重复执行会尽量复用已有用户、组与配置文件。

### 2.1 准备发布文件

先通过构建流程生成 Linux 可执行文件（示例）：

```bash
./scripts/build.sh
```

默认发布文件可使用 `./dist/gateway-linux-amd64`。

### 2.2 执行安装脚本

```bash
RELEASE_FILE=./dist/gateway-linux-amd64 ./deploy/scripts/install.sh
```

脚本将执行：
1. 预检查（可用 `SKIP_PREFLIGHT=true` 跳过）。
2. 创建系统用户/组（默认 `siptunnel`）。
3. 创建目录并安装二进制到 `/opt/siptunnel/gateway`。
4. 初始化配置文件（若不存在）到 `/opt/siptunnel/config.yaml`。
5. 生成并安装 systemd unit。
6. 启动并设置开机自启。

### 2.3 验证安装结果

```bash
systemctl status --no-pager siptunnel-gateway.service
curl -fsS http://127.0.0.1:18080/healthz
```

## 3. 升级步骤（Upgrade）

> 适用：平滑替换版本。升级脚本会先做备份，失败自动回滚。

### 3.1 执行升级

```bash
RELEASE_FILE=./dist/gateway-linux-amd64 ./deploy/scripts/upgrade.sh
```

升级动作：
1. 备份当前二进制到 `/opt/siptunnel/backups/`。
2. 停止服务并替换二进制。
3. 启动服务并执行健康检查。
4. 若健康检查失败，自动回滚到最近备份。

### 3.2 升级后核查

```bash
systemctl status --no-pager siptunnel-gateway.service
ls -lh /opt/siptunnel/backups
```

## 4. 回滚步骤（Rollback）

### 4.1 回滚到最近版本

```bash
./deploy/scripts/rollback.sh
```

### 4.2 指定备份文件回滚

```bash
TARGET_BACKUP=/opt/siptunnel/backups/gateway-20260101-120000.bak ./deploy/scripts/rollback.sh
```

回滚完成后建议立即检查：

```bash
systemctl status --no-pager siptunnel-gateway.service
curl -fsS http://127.0.0.1:18080/healthz
```

## 5. systemd unit 模板

仓库提供模板：`deploy/systemd/siptunnel-gateway.service`。

安装脚本会自动将模板渲染为：
- `/etc/systemd/system/siptunnel-gateway.service`（默认服务名）

如果手工修改 unit，请执行：

```bash
systemctl daemon-reload
systemctl restart siptunnel-gateway.service
```

## 6. 运行参数基线

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `GATEWAY_PORT` | `18080` | HTTP 控制与运维接口监听端口 |
| `MEDIA_PORT_START` | `20000` | RTP 接收/发送端口范围起始 |
| `MEDIA_PORT_END` | `20100` | RTP 接收/发送端口范围结束 |
| `NODE_ROLE` | `receiver` | 节点职责（`receiver`/`sender`） |
| `GATEWAY_DATA_DIR` | `/var/lib/siptunnel` | 数据根目录 |

## 7. 常见失败场景

### 场景 1：安装时报权限错误（目录不可写）
- 现象：`storage check` 报目录创建失败或不可写。
- 处理：确认脚本以 root 执行，或修正 `INSTALL_DIR/DATA_DIR` 权限后重试。

### 场景 2：服务启动失败，日志提示端口占用
- 现象：`network check` 或 systemd 日志提示 `LISTEN_PORT` 被占用。
- 处理：变更 `GATEWAY_PORT`，或释放冲突端口后 `systemctl restart`。

### 场景 3：升级后健康检查失败
- 现象：`upgrade.sh` 报错并触发自动回滚。
- 处理：
  1. 检查新版本配置兼容性。
  2. 查看 `journalctl -u siptunnel-gateway -n 200 --no-pager`。
  3. 修复后重新执行升级。

### 场景 4：配置校验失败
- 现象：`config validate` 提示缺失字段或加载失败。
- 处理：核对配置路径、YAML 格式与关键字段；可先恢复到上一个有效配置。

## 8. 运维操作建议

1. 所有变更先运行 `precheck.sh all` 并归档输出。
2. 升级前确认备份目录可用并记录变更单号。
3. 升级完成后保留至少一个可回滚版本。
4. 故障排查优先检查：端口占用、配置加载、目录权限、系统服务日志。
