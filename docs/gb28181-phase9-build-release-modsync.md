# Phase 9：发布构建前自动同步 Go 模块图

## 背景

在 Go 1.23 构建链下，当前仓库在完成多轮协议/界面改造后，可能出现源码 import 已变化、但 `go.mod/go.sum` 还未同步的情况。

典型报错表现为：

- `go: updates to go.mod needed; to update it: go mod tidy`

## 本轮调整

为避免 release 构建因为模块图未同步而中断，`scripts/build.ps1` 与 `scripts/build.sh` 已在真正执行 `go build` 之前自动执行：

```bash
go mod tidy -compat=1.23
```

## 效果

- Windows PowerShell 的 `./scripts/build-release.ps1` 会先自动校正 `gateway-server/go.mod` / `go.sum`。
- Linux/macOS 的 `./scripts/build.sh` 也会执行同样的同步逻辑。
- 后续 UI/后端迭代后，如果只改了源码而忘了 tidy，release 构建仍能自恢复。

## 说明

这一步依赖本机构建环境可以访问或已经缓存所需 Go 模块。
若外网受限，请先准备 GOPROXY、企业代理或本地模块缓存。
