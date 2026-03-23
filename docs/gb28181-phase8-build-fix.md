# GB28181 Phase 8 - UI build blocker fix

## 本轮修复

修复了 `gateway-ui/src/api/mockGateway.ts` 中 `TunnelCatalogResource` mock 数据缺少 `methods` 字段的问题。

### 背景

前端类型 `TunnelCatalogResource` 已将 `methods` 作为规范化后的必填字段：

- `methods`: 页面统一消费字段
- `method_list`: 为兼容后端原始返回保留的可选字段

但 mock 数据仍只保留 `method_list`，导致 Windows 下执行：

```powershell
./scripts/build-release.ps1
```

在 `npm run typecheck` 阶段触发 `TS2741`：

- `Property 'methods' is missing ... but required in type 'TunnelCatalogResource'`

## 处理结果

已将 mock Catalog 资源补齐为同时返回：

- `methods`
- `method_list`

这样页面既能直接使用规范化字段，也能继续保留与后端原始响应格式的兼容语义。

另外顺手清理了 `gateway-ui/src/api/gateway.ts` 中 `importConfigJson()` 的重复 `return` 死代码。

## Windows PowerShell 提示

首次运行脚本时如果反复出现安全提示，可先在仓库根目录执行：

```powershell
Get-ChildItem .\scripts\*.ps1 | Unblock-File
```

随后再运行：

```powershell
.\scripts\build-release.ps1
```
