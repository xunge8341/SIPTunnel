# 证据归档

运行 `scripts/agent/run-task.*` 后，实际证据会写入 `artifacts/agent/<task-id>/<timestamp>/`。

本目录仅用于说明结构：

- `latest/`：可选的最新结果软链接或快照入口
- `history/`：可选的历史归档索引

如需将证据长期纳入版本控制，建议只提交摘要和索引，不提交大型日志与长稳输出。
